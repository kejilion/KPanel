package systemmanage

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type accountProtocolRunner struct {
	statusCalls int
	inputs      [][]byte
	arguments   [][]string
}

func (runner *accountProtocolRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func (runner *accountProtocolRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, nil
}

func (runner *accountProtocolRunner) RunResource(_ context.Context, _ int, input []byte, _ string, arguments ...string) ([]byte, []byte, error) {
	runner.inputs = append(runner.inputs, append([]byte(nil), input...))
	runner.arguments = append(runner.arguments, append([]string(nil), arguments...))
	if len(arguments) > 0 && arguments[len(arguments)-1] == "status" {
		runner.statusCalls++
		version := strings.Repeat("a", 64)
		if runner.statusCalls > 1 {
			version = strings.Repeat("b", 64)
		}
		account := hex.EncodeToString([]byte("root\t0\t0\t/root\t/bin/bash\troot\tenabled\troot\troot\t0"))
		return []byte(strings.Join([]string{
			"KPANEL_ACCOUNT_MANAGEMENT_STATUS=ok",
			"KPANEL_ACCOUNT_MANAGEMENT_VERSION=" + version,
			"KPANEL_ACCOUNT_MANAGEMENT_PASSWORD_AUTH=yes",
			"KPANEL_ACCOUNT_MANAGEMENT_PUBKEY_AUTH=yes",
			"KPANEL_ACCOUNT_MANAGEMENT_ROOT_LOGIN=enabled",
			"KPANEL_ACCOUNT_MANAGEMENT_TOTAL=1",
			"KPANEL_ACCOUNT_MANAGEMENT_TRUNCATED=false",
			"KPANEL_ACCOUNT_MANAGEMENT_ACCOUNT_HEX=" + account,
			"",
		}, "\n")), nil, nil
	}
	return []byte("KPANEL_ACCOUNT_MANAGEMENT_STATUS=applied\nKPANEL_ACCOUNT_MANAGEMENT_VERSION=" + strings.Repeat("b", 64) + "\n"), nil, nil
}

func TestParseAccountManagementSnapshot(t *testing.T) {
	version := strings.Repeat("a", 64)
	keyID := strings.Repeat("b", 64)
	hexValue := func(value string) string { return hex.EncodeToString([]byte(value)) }
	output := strings.Join([]string{
		"KPANEL_ACCOUNT_MANAGEMENT_STATUS=ok",
		"KPANEL_ACCOUNT_MANAGEMENT_VERSION=" + version,
		"KPANEL_ACCOUNT_MANAGEMENT_PASSWORD_AUTH=no",
		"KPANEL_ACCOUNT_MANAGEMENT_PUBKEY_AUTH=yes",
		"KPANEL_ACCOUNT_MANAGEMENT_ROOT_LOGIN=key-only",
		"KPANEL_ACCOUNT_MANAGEMENT_TOTAL=2",
		"KPANEL_ACCOUNT_MANAGEMENT_TRUNCATED=false",
		"KPANEL_ACCOUNT_MANAGEMENT_KEY_HEX=" + hexValue("operator\t"+keyID+"\tssh-ed25519\tSHA256:test\tlaptop"),
		"KPANEL_ACCOUNT_MANAGEMENT_ACCOUNT_HEX=" + hexValue("root\t0\t0\t/root\t/bin/bash\troot\tlocked\troot\troot\t0"),
		"KPANEL_ACCOUNT_MANAGEMENT_ACCOUNT_HEX=" + hexValue("operator\t1000\t1000\t/home/operator\t/bin/bash\thuman\tlocked\tpasswordless-admin\toperator,sudo\t1"),
		"",
	}, "\n")
	snapshot, err := parseAccountManagementSnapshot([]byte(output))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ResourceVersion != version || snapshot.SSHPolicy.PasswordAuthentication || !snapshot.SSHPolicy.PublicKeyAuthentication || snapshot.SSHPolicy.RootLogin != "key-only" {
		t.Fatalf("unexpected policy: %#v", snapshot)
	}
	if len(snapshot.Accounts) != 2 || snapshot.Accounts[1].Username != "operator" || len(snapshot.Accounts[1].SSHKeys) != 1 {
		t.Fatalf("unexpected accounts: %#v", snapshot.Accounts)
	}
	if snapshot.Accounts == nil || snapshot.Accounts[0].Groups == nil || snapshot.Accounts[0].SSHKeys == nil {
		t.Fatalf("empty account collections must serialize as arrays: %#v", snapshot.Accounts[0])
	}
}

func TestAccountManagementInvocationKeepsSecretOutOfArgv(t *testing.T) {
	request := contract.AccountManagementActionRequest{
		Action: "create", ExpectedResourceVersion: strings.Repeat("a", 64),
		Username: "operator", Role: "administrator", Credential: "password", Secret: "do-not-leak-this-password",
	}
	arguments, input := accountManagementInvocation(request)
	if strings.Contains(strings.Join(arguments, " "), request.Secret) || !strings.Contains(strings.Join(arguments, " "), "--secret-stdin") {
		t.Fatalf("secret transport argv=%q", arguments)
	}
	if string(input) != request.Secret+"\n" {
		t.Fatalf("secret stdin=%q", input)
	}
}

func TestTrustedAccountManagementProtocolRequiresExactVersion(t *testing.T) {
	base := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"4\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE\nkpanel_system_resource_dispatch\nKPANEL_SYSTEM_RESOURCE_STATUS\nKPANEL_SYSTEM_RESOURCE_VERSION\n")
	if trustedKejilionAccountManagementContent(base) {
		t.Fatal("system-resource-only script was trusted for account management")
	}
	current := append(base, []byte("KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION=\"1\"\nKJ_ACCOUNT_MANAGEMENT_NONINTERACTIVE\nkpanel_account_dispatch\nKPANEL_ACCOUNT_MANAGEMENT_STATUS\n--secret-stdin\n")...)
	if !trustedKejilionAccountManagementContent(current) {
		t.Fatal("current account-management protocol was rejected")
	}
}

func TestExecuteAccountManagementActionUsesFixedArgvSecretStdinAndReadback(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Skip("trusted root-owned script contract is Linux/root only")
	}
	script := filepath.Join(t.TempDir(), "kejilion.sh")
	content := strings.Repeat("# padding\n", 160) + strings.Join([]string{
		`permission_granted="true"`, `KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="4"`,
		"KJ_SYSTEM_RESOURCE_NONINTERACTIVE", "kpanel_system_resource_dispatch", "KPANEL_SYSTEM_RESOURCE_STATUS", "KPANEL_SYSTEM_RESOURCE_VERSION",
		`KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION="1"`, "KJ_ACCOUNT_MANAGEMENT_NONINTERACTIVE", "kpanel_account_dispatch", "KPANEL_ACCOUNT_MANAGEMENT_STATUS", "--secret-stdin",
	}, "\n")
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &accountProtocolRunner{}
	manager := NewManager(Config{
		Enabled: true, EffectiveUID: func() int { return 0 }, Runner: runner,
		ResourceScript: func() (string, error) { return script, nil },
	})
	secret := "do-not-put-this-password-in-argv"
	result, err := manager.ExecuteAccountManagementAction(context.Background(), contract.AccountManagementActionRequest{
		Action: "set-password", ExpectedResourceVersion: strings.Repeat("a", 64), Username: "root", Secret: secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.ResourceVersion != strings.Repeat("b", 64) || runner.statusCalls != 2 {
		t.Fatalf("result=%#v statusCalls=%d", result, runner.statusCalls)
	}
	foundAction := false
	for index, arguments := range runner.arguments {
		joined := strings.Join(arguments, " ")
		if strings.Contains(joined, secret) {
			t.Fatalf("secret leaked into argv: %q", joined)
		}
		if strings.Contains(joined, "account-management set-password") {
			foundAction = true
			if !strings.HasSuffix(joined, "set-password "+strings.Repeat("a", 64)+" root --secret-stdin") || string(runner.inputs[index]) != secret+"\n" {
				t.Fatalf("action argv=%q stdin=%q", joined, runner.inputs[index])
			}
		}
	}
	if !foundAction {
		t.Fatal("account action invocation was not observed")
	}
}
