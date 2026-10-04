package panel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadConfigRetiredWindowsTrustKeys(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Panel configuration targets Linux; covered by Linux source checks")
	}
	config := validTestConfig()
	content, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		t.Fatal(err)
	}
	fields["windowsNodePublisher"] = json.RawMessage(`"retired test publisher"`)
	fields["windowsNodeProfileOid"] = json.RawMessage(`"1.2.3.4"`)
	path := filepath.Join(t.TempDir(), "config.json")
	write := func() {
		t.Helper()
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if loaded, err := LoadConfig(path); err != nil || loaded.Listen != config.Listen {
		t.Fatalf("migration: %v", err)
	}
	fields["unrelatedUnknown"] = json.RawMessage(`true`)
	write()
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("migration weakened strict decoding of unrelated fields")
	}
}

func TestLoadConfigAllowIPHostsEnvironment(t *testing.T) {
	t.Setenv("KEJILION_PANEL_CONFIG", "")
	t.Setenv("KEJILION_PANEL_ALLOW_IP_HOSTS", "true")
	config, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if !config.AllowIPHosts {
		t.Fatal("KEJILION_PANEL_ALLOW_IP_HOSTS did not enable literal IP hosts")
	}

	t.Setenv("KEJILION_PANEL_ALLOW_IP_HOSTS", "invalid")
	if _, err := LoadConfig(""); err == nil {
		t.Fatal("invalid KEJILION_PANEL_ALLOW_IP_HOSTS was accepted")
	}
}

func TestConfigRejectsNonOriginPublicURLs(t *testing.T) {
	t.Parallel()
	for _, publicURL := range []string{
		"https://user@panel.example.com",
		"https://panel.example.com/path",
		"https://panel.example.com?query=1",
		"https://panel.example.com#fragment",
		"https://panel.example.com:65536",
	} {
		config := validTestConfig()
		config.PublicURL = publicURL
		if err := config.Validate(); err == nil {
			t.Errorf("Validate accepted %q", publicURL)
		}
	}
}

func TestConfigAcceptsHTTPSOrigin(t *testing.T) {
	t.Parallel()
	config := validTestConfig()
	config.PublicURL = "https://panel.example.com:8443"
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate rejected HTTPS origin: %v", err)
	}
}

func TestConfigRejectsWebRootSecretOverlap(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*Config){
		"relative": func(config *Config) {
			config.WebRoot = "relative/web"
		},
		"data child": func(config *Config) {
			config.WebRoot = "/var/lib/kejilion-panel/web"
		},
		"data parent": func(config *Config) {
			config.WebRoot = "/var/lib"
		},
		"agent token parent": func(config *Config) {
			config.WebRoot = "/run"
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := validTestConfig()
			mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("Validate accepted an unsafe webRoot")
			}
		})
	}
}

func TestConfigRejectsUnsafeTOTPKeyPaths(t *testing.T) {
	t.Parallel()
	for name, keyPath := range map[string]string{
		"relative":        "totp-encryption.key",
		"store collision": "/var/lib/kejilion-panel/panel-state.json",
		"web root child":  "/app/web/totp-encryption.key",
	} {
		t.Run(name, func(t *testing.T) {
			config := validTestConfig()
			config.TOTPKeyPath = keyPath
			if err := config.Validate(); err == nil {
				t.Fatal("Validate accepted an unsafe TOTP encryption key path")
			}
		})
	}
}

func TestConfigRejectsUnsafeCookieNames(t *testing.T) {
	t.Parallel()
	for _, cookieName := range []string{
		"__Host-invalid cookie",
		"__Host-kejilion_csrf",
		"__Host-invalid;cookie",
	} {
		config := validTestConfig()
		config.CookieName = cookieName
		if err := config.Validate(); err == nil {
			t.Errorf("Validate accepted cookie name %q", cookieName)
		}
	}
}

func TestConfigValidatesTrustedProxyCIDRs(t *testing.T) {
	t.Parallel()
	config := validTestConfig()
	config.TrustedProxyCIDRs = []string{"127.0.0.0/8", "172.17.0.0/16", "::1/128"}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate rejected trusted proxy CIDRs: %v", err)
	}
	config.TrustedProxyCIDRs = []string{"not-a-cidr"}
	if err := config.Validate(); err == nil {
		t.Fatal("Validate accepted an invalid trusted proxy CIDR")
	}
}

func validTestConfig() Config {
	config := DefaultConfig()
	config.StorePath = "/var/lib/kejilion-panel/panel-state.json"
	config.BootstrapTokenPath = "/var/lib/kejilion-panel/bootstrap.token"
	config.TOTPKeyPath = "/var/lib/kejilion-panel/totp-encryption.key"
	config.CookieName = "__Host-kejilion_session"
	return config
}
