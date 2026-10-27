//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const acceptanceService = "KPanelRC8Acceptance"
const acceptanceHelper = "rc8-scm-acceptance.test.exe"

// Local runs never install services. The center is an explicit TLS protocol
// fixture: these assertions prove SCM lifecycle, not real multi-host sessions.
func TestWindowsHostedSCMLifecycle(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || os.Getenv("RUNNER_OS") != "Windows" || os.Getenv("RUNNER_TEMP") == "" {
		t.Skip("isolated GitHub-hosted Windows acceptance only")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Fatal("hosted runner must be elevated")
	}
	for _, root := range []string{windowsnode.InstallDir(), windowsnode.DataDir()} {
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("refusing pre-existing node installation")
		}
	}
	manager, err := mgr.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Disconnect()
	for _, name := range []string{acceptanceService, windowsnode.BootstrapService, windowsnode.TelemetryService, windowsnode.TerminalService, windowsnode.FileService} {
		s, err := manager.OpenService(name)
		if err == nil {
			s.Close()
			t.Fatal("refusing pre-existing SCM service")
		}
		if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	var reports atomic.Int64
	secret := bytes.Repeat([]byte{7}, 32)
	peer, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	center := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(cluster.LightResponseCapabilitiesHeader, "windows-command-v1")
		switch r.URL.Path {
		case lightEnrollPath:
			var request enrollRequest
			if json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&request) != nil || request.Platform != "windows" || request.TerminalPublicKey == "" {
				http.Error(w, "invalid enrollment", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(enrollResponse{NodeID: strings.Repeat("a", 32), ReportingKey: base64.RawURLEncoding.EncodeToString(secret), ReportInterval: 20, TerminalPeerPublicKey: base64.RawURLEncoding.EncodeToString(peer.Public), TargetNodeID: strings.Repeat("b", 32)})
		case lightReportPath:
			body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			expected := cluster.LightRequestSignature(secret, r.Method, r.URL.Path, r.Header.Get("X-KPanel-Light-Node-ID"), r.Header.Get("X-KPanel-Timestamp"), r.Header.Get("X-KPanel-Request-ID"), body)
			var report reportRequest
			if expected != r.Header.Get("X-KPanel-Signature") || json.Unmarshal(body, &report) != nil || report.Platform != "windows" {
				http.Error(w, "invalid signed telemetry", 401)
				return
			}
			reports.Add(1)
			_, _ = io.WriteString(w, `{}`)
		case "/api/v3/federation/light/capabilities":
			_, _ = io.WriteString(w, `{"capabilities":["windows-command-v1"]}`)
		default:
			http.Error(w, "no relay session in SCM fixture", http.StatusServiceUnavailable)
		}
	}))
	defer center.Close()
	for _, item := range []struct {
		path   string
		access windowsnode.Access
	}{{windowsnode.InstallDir(), windowsnode.ProgramRead}, {windowsnode.DataDir(), windowsnode.StateDirectory}} {
		if err := windowsnode.EnsureDirectory(item.path, item.access); err != nil {
			t.Fatal(err)
		}
	}
	live := filepath.Join(windowsnode.InstallDir(), "kejilion-node.exe")
	build := func(path, version string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, "go", "build", "-ldflags=-X github.com/kejilion/kejilion-panel/internal/version.Version="+version, "-o", path, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build acceptance executable: %v: %s", err, output)
		}
	}
	build(live, "1.28.0-rc.8-scm-original")
	content, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.InstallDir(), "kejilion-node-bootstrap.exe"), content, windowsnode.ProgramRead); err != nil {
		t.Fatal(err)
	}
	if err := windowsnode.SecureProgramFile(live); err != nil {
		t.Fatal(err)
	}
	// Refusal by the actual uninstaller remains a failure; never replace its
	// checks with recursive deletion of a computed directory.
	t.Cleanup(func() {
		if live == "" {
			return
		}
		cleanupCtx, stop := context.WithTimeout(context.Background(), 90*time.Second)
		defer stop()
		if output, err := exec.CommandContext(cleanupCtx, live, "uninstall").CombinedOutput(); err != nil {
			t.Errorf("SCM cleanup/uninstall failed: %v: %.1500s", err, output)
		}
	})
	// Identify protected-file versus SCM failures without changing the installer.
	releaseLifecycle, err := windowsnode.AcquireLifecycle()
	if err != nil {
		t.Fatalf("lifecycle preflight: %v", err)
	}
	if err := releaseLifecycle(); err != nil {
		t.Fatalf("lifecycle release preflight: %v", err)
	}
	if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.DataDir(), "acceptance-sealed-preflight.json"), []byte("fixture-only"), windowsnode.SystemOnly); err != nil {
		t.Fatalf("sealed atomic handoff preflight: %v", err)
	}
	pin := sha256.Sum256(center.Certificate().RawSubjectPublicKeyInfo)
	wire, _ := json.Marshal(tokenWire{Version: 1, Origin: center.URL, ID: strings.Repeat("c", 32), Secret: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)), ExpiresAt: time.Now().Add(5 * time.Minute).Unix(), TLSPublicKeyPin: base64.RawURLEncoding.EncodeToString(pin[:])})
	request, _ := json.Marshal(windowsInstallRequest{Token: lightTokenPrefix + base64.RawURLEncoding.EncodeToString(wire), Name: "isolated-hosted-scm", Capabilities: "monitoring,terminal,files", SHA256: hex.EncodeToString(digest[:])})
	install := exec.CommandContext(ctx, live, "install", "--stdin")
	install.Stdin = bytes.NewReader(request)
	if output, err := install.CombinedOutput(); err != nil {
		message := strings.ReplaceAll(string(output), lightTokenPrefix+base64.RawURLEncoding.EncodeToString(wire), "<redacted-enrollment>")
		message = strings.ReplaceAll(message, base64.RawURLEncoding.EncodeToString(secret), "<redacted-reporting-key>")
		diagnostic := diagnoseHostedBootstrap(ctx, manager, request)
		t.Fatalf("real SYSTEM bootstrap failed: %v: %.1500s; diagnostic: %.1500s", err, message, diagnostic)
	}
	clear(request)
	assertRunning := func() {
		t.Helper()
		for _, spec := range windowsnode.Services {
			s, err := manager.OpenService(spec.Name)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := s.Config()
			if err != nil {
				s.Close()
				t.Fatal(err)
			}
			status, err := s.Query()
			s.Close()
			if err != nil || status.State != svc.Running {
				t.Fatalf("service %s not running: %v state=%v", spec.Name, err, status.State)
			}
			if strings.Contains(cfg.BinaryPathName, "kpl1.") || strings.Contains(cfg.BinaryPathName, "kpb1.") {
				t.Fatal("enrollment secret in SCM command")
			}
			if spec.Privileged {
				if !strings.EqualFold(cfg.ServiceStartName, "LocalSystem") {
					t.Fatal("broker must run as LocalSystem")
				}
			} else if !strings.EqualFold(cfg.ServiceStartName, `NT SERVICE\`+spec.Name) || cfg.SidType != windows.SERVICE_SID_TYPE_RESTRICTED {
				t.Fatal("telemetry lost restricted virtual service identity")
			}
		}
	}
	waitReport := func(previous int64) {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for reports.Load() <= previous && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
		if reports.Load() <= previous {
			t.Fatal("restricted telemetry service did not produce a verified report")
		}
		assertRunning()
	}
	waitReport(0)
	t.Log("SCM install, SYSTEM bootstrap, account separation and signed telemetry passed")
	previous := reports.Load()
	if err := windowsnode.StopServices(); err != nil {
		t.Fatal(err)
	}
	if err := windowsnode.StartServices(); err != nil {
		t.Fatal(err)
	}
	waitReport(previous)
	t.Log("real SCM stop/start and telemetry recovery passed; OS reboot is not claimed")
	// SYSTEM executes the unchanged production updater against real protected
	// files and services. This is not a claim about public download/install entry points.
	helperBytes, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	helperPath := filepath.Join(windowsnode.InstallDir(), acceptanceHelper)
	if err := windowsnode.WriteAtomic(helperPath, helperBytes, windowsnode.ProgramRead); err != nil {
		t.Fatal(err)
	}
	helper, err := manager.CreateService(acceptanceService, helperPath, mgr.Config{StartType: mgr.StartManual}, "-test.run=^TestWindowsHostedSCMUpdaterHelper$")
	if err != nil {
		t.Fatal(err)
	}
	helperClosed := false
	defer func() {
		if !helperClosed {
			helper.Control(svc.Stop)
			helper.Delete()
			helper.Close()
		}
	}()
	next := filepath.Join(windowsnode.InstallDir(), "staging", "kejilion-node.exe.next")
	for _, mode := range []string{"apply", "rollback", "apply", "rollback", "apply", "rollback", "apply", "rollback", "apply", "rollback"} {
		build(next, "1.28.0-rc.8-scm-"+mode)
		if err := windowsnode.SecureProgramFile(next); err != nil {
			t.Fatal(err)
		}
		nextBytes, err := os.ReadFile(next)
		if err != nil {
			t.Fatal(err)
		}
		nextHash := sha256.Sum256(nextBytes)
		plan, _ := json.Marshal(map[string]string{"mode": mode, "digest": hex.EncodeToString(nextHash[:])})
		if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.DataDir(), "acceptance-plan.json"), plan, windowsnode.SystemOnly); err != nil {
			t.Fatal(err)
		}
		resultPath := filepath.Join(windowsnode.InstallDir(), "acceptance-result.json")
		if err := os.Remove(resultPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if err := helper.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Minute)
		var result []byte
		var resultErr error
		for time.Now().Before(deadline) {
			result, resultErr = os.ReadFile(resultPath)
			if resultErr == nil {
				break
			}
			// WriteAtomic publishes the result before its no-sharing handle closes.
			// Retry that brief interval without suppressing other access failures.
			if !errors.Is(resultErr, os.ErrNotExist) && !errors.Is(resultErr, windows.ERROR_SHARING_VIOLATION) {
				t.Fatalf("SYSTEM updater result read: %v", resultErr)
			}
			time.Sleep(200 * time.Millisecond)
		}
		if resultErr != nil {
			t.Fatalf("SYSTEM updater acceptance did not finish: %v", resultErr)
		}
		if string(result) != "pass" {
			t.Fatalf("SYSTEM updater %s acceptance: %s", mode, result)
		}
		for time.Now().Before(deadline) {
			s, err := helper.Query()
			if err != nil {
				t.Fatal(err)
			}
			if s.State == svc.Stopped {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		assertRunning()
		t.Log("real-disk production updater " + mode + " and service health passed")
	}
	if err := helper.Delete(); err != nil {
		t.Fatal(err)
	}
	if err := helper.Close(); err != nil {
		t.Fatal(err)
	}
	helperClosed = true
	if err := exec.CommandContext(ctx, live, "uninstall").Run(); err != nil {
		t.Fatal("real SYSTEM uninstaller failed")
	}
	for _, spec := range windowsnode.Services {
		s, err := manager.OpenService(spec.Name)
		if err == nil {
			s.Close()
			t.Fatal("service survived uninstall")
		}
		if !errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			t.Fatal(err)
		}
	}
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(systemDirectory, "Tasks", "KPanel", "KejilionNodeUpdate")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("scheduled update definition survived uninstall or could not be inspected")
	}
	live = "" // Checked uninstall completed; cleanup has nothing further to do.
	t.Log("real SYSTEM uninstall removed product services; reboot-pending files remain allowed")
}

// The normal product install must still pass. On failure, a protected test-only
// SYSTEM service calls the same bootstrap once to expose its returned error.
// It never converts that failed installation into an acceptance success.
func diagnoseHostedBootstrap(ctx context.Context, manager *mgr.Mgr, request []byte) string {
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		return "cannot read diagnostic helper"
	}
	path := filepath.Join(windowsnode.InstallDir(), acceptanceHelper)
	if err := windowsnode.WriteAtomic(path, data, windowsnode.ProgramRead); err != nil {
		return "cannot stage diagnostic helper: " + err.Error()
	}
	s, err := manager.CreateService(acceptanceService, path, mgr.Config{StartType: mgr.StartManual}, "-test.run=^TestWindowsHostedSCMUpdaterHelper$")
	if err != nil {
		return "cannot register diagnostic helper: " + err.Error()
	}
	defer s.Close()
	defer s.Delete()
	defer s.Control(svc.Stop)
	if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.DataDir(), "enrollment-request.json"), request, windowsnode.SystemOnly); err != nil {
		return "cannot restore diagnostic handoff: " + err.Error()
	}
	if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.DataDir(), "acceptance-plan.json"), []byte(`{"mode":"bootstrap-diagnostic"}`), windowsnode.SystemOnly); err != nil {
		return "cannot write diagnostic plan: " + err.Error()
	}
	if err := s.Start(); err != nil {
		return "cannot start diagnostic helper: " + err.Error()
	}
	result := filepath.Join(windowsnode.InstallDir(), "acceptance-result.json")
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(result); err == nil {
			return string(data)
		}
		select {
		case <-ctx.Done():
			return "diagnostic cancelled"
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "diagnostic timed out"
}

// Only the protected test executable registered above can run this SYSTEM helper.
func TestWindowsHostedSCMUpdaterHelper(t *testing.T) {
	if !windowsnode.IsSystem() {
		t.Skip("SYSTEM helper only")
	}
	exe, err := os.Executable()
	if err != nil || !strings.EqualFold(exe, filepath.Join(windowsnode.InstallDir(), acceptanceHelper)) {
		t.Fatal("untrusted helper location")
	}
	err = windowsnode.RunService(acceptanceService, func(ctx context.Context) error {
		result := func() error {
			data, err := windowsnode.ReadFile(filepath.Join(windowsnode.DataDir(), "acceptance-plan.json"), 1024, windowsnode.SystemOnly)
			if err != nil {
				return err
			}
			var plan struct {
				Mode   string `json:"mode"`
				Digest string `json:"digest"`
			}
			if json.Unmarshal(data, &plan) != nil || (plan.Mode != "apply" && plan.Mode != "rollback" && plan.Mode != "bootstrap-diagnostic") {
				return errors.New("invalid acceptance plan")
			}
			// Consume this private fixture as SYSTEM. The administrator cannot
			// replace an existing SYSTEM-only plan for the next acceptance step.
			if err := os.Remove(filepath.Join(windowsnode.DataDir(), "acceptance-plan.json")); err != nil {
				return err
			}
			if plan.Mode == "bootstrap-diagnostic" {
				data, err := windowsnode.ReadFile(filepath.Join(windowsnode.DataDir(), "enrollment-request.json"), 8192, windowsnode.SystemOnly)
				if err != nil {
					return errors.New("diagnostic handoff unavailable")
				}
				var request windowsInstallRequest
				if json.Unmarshal(data, &request) != nil {
					return errors.New("diagnostic handoff invalid")
				}
				err = bootstrapWindowsNode()
				message := "bootstrap returned success on diagnostic replay; original install remains failed"
				if err != nil {
					message = err.Error()
				}
				message = strings.ReplaceAll(message, request.Token, "<redacted-enrollment>")
				message = strings.ReplaceAll(message, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)), "<redacted-reporting-key>")
				if len(message) > 1500 {
					message = message[:1500]
				}
				return errors.New(message)
			}
			live := filepath.Join(windowsnode.InstallDir(), "kejilion-node.exe")
			before, err := os.ReadFile(live)
			if err != nil {
				return err
			}
			oldHash := sha256.Sum256(before)
			calls := 0
			u := windowsnode.NewUpdater(func(ctx context.Context) error {
				calls++
				if plan.Mode == "rollback" && calls == 1 {
					return errors.New("injected new-binary health failure")
				}
				return windowsServicesHealthy(ctx)
			})
			ctx, cancel := context.WithTimeout(ctx, 100*time.Second)
			defer cancel()
			err = u.Apply(ctx, plan.Digest)
			if plan.Mode == "apply" && err != nil {
				return err
			}
			if plan.Mode == "rollback" && err == nil {
				return errors.New("expected verified rollback but update succeeded")
			}
			if plan.Mode == "rollback" && !strings.Contains(err.Error(), "rolled back") {
				return fmt.Errorf("expected verified rollback: %w", err)
			}
			after, err := os.ReadFile(live)
			if err != nil {
				return err
			}
			hash := sha256.Sum256(after)
			if plan.Mode == "apply" && hex.EncodeToString(hash[:]) != plan.Digest {
				return errors.New("new binary not installed")
			}
			if plan.Mode == "rollback" && hash != oldHash {
				return errors.New("rollback bytes not restored")
			}
			if _, err := os.Stat(u.Journal); !errors.Is(err, os.ErrNotExist) {
				return errors.New("update journal not cleared")
			}
			return nil
		}()
		message := "pass"
		if result != nil {
			message = result.Error()
		}
		return errors.Join(result, windowsnode.WriteAtomic(filepath.Join(windowsnode.InstallDir(), "acceptance-result.json"), []byte(message), windowsnode.ProgramRead))
	})
	if err != nil {
		t.Fatal(err)
	}
}
