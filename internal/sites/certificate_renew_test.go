//go:build linux

package sites

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeCertificateRenewer struct {
	root   string
	err    error
	calls  int
	domain string
	// issue writes a fresh pair like the script would after Certbot succeeds.
	issue func(t *testing.T) (cert, key string)
	t     *testing.T
}

func (f *fakeCertificateRenewer) Available() error { return nil }
func (f *fakeCertificateRenewer) Renew(_ context.Context, domain string) error {
	f.calls++
	f.domain = domain
	if f.err != nil {
		return f.err
	}
	if f.issue == nil {
		return nil
	}
	cert, key := f.issue(f.t)
	if err := os.WriteFile(filepath.Join(f.root, "certs", domain+"_cert.pem"), []byte(cert), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.root, "certs", domain+"_key.pem"), []byte(key), 0o600)
}

func certificateRenewFixture(t *testing.T, notAfter time.Duration) (*Manager, *fakeCertificateRenewer, string, RenewCertificateInput) {
	t.Helper()
	m, _, root := newTestManager(t)
	cert, key := makeCustomCertificateMaterial(t, "example.com", time.Now().Add(-time.Hour), time.Now().Add(notAfter))
	config := "server {\nlisten 443 ssl;\nserver_name example.com;\nroot /var/www/html/example.com;\nssl_certificate /etc/nginx/certs/example.com_cert.pem;\nssl_certificate_key /etc/nginx/certs/example.com_key.pem;\n}\n"
	for path, body := range map[string]string{"conf.d/example.com.conf": config, "certs/example.com_cert.pem": cert, "certs/example.com_key.pem": key} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	items, err := m.discoverer.Discover()
	if err != nil || len(items) != 1 {
		t.Fatalf("discover: %v %#v", err, items)
	}
	fake := &fakeCertificateRenewer{root: root, t: t}
	fake.issue = func(t *testing.T) (string, string) {
		return makeCustomCertificateMaterial(t, "example.com", time.Now().Add(-time.Hour), time.Now().Add(80*24*time.Hour))
	}
	m.certificateRenewer = fake
	return m, fake, items[0].ID, RenewCertificateInput{PrimaryDomain: "example.com", ExpectedResourceVersion: items[0].ResourceVersion}
}

func TestRenewCertificateReplacesExpiringCertificateAndReconciles(t *testing.T) {
	for name, notAfter := range map[string]time.Duration{"expiring": 17 * 24 * time.Hour, "expired": -time.Minute} {
		t.Run(name, func(t *testing.T) {
			m, fake, id, input := certificateRenewFixture(t, notAfter)
			before, _ := os.ReadFile(filepath.Join(m.webRoot, "conf.d/example.com.conf"))
			result, err := m.RenewCertificate(context.Background(), id, input)
			if err != nil {
				t.Fatal(err)
			}
			if fake.calls != 1 || fake.domain != "example.com" {
				t.Fatalf("renewer calls=%d domain=%q", fake.calls, fake.domain)
			}
			if result.TLS.Status != "valid" || result.ResourceVersion == input.ExpectedResourceVersion {
				t.Fatalf("result not reconciled: %#v", result.TLS)
			}
			after, _ := os.ReadFile(filepath.Join(m.webRoot, "conf.d/example.com.conf"))
			if string(before) != string(after) {
				t.Fatal("renewal changed site configuration")
			}
		})
	}
}

func TestRenewCertificateRejectsUnsafeRequestsBeforeScript(t *testing.T) {
	for _, mode := range []string{"valid-certificate", "version", "identity", "bad-domain", "custom-binding"} {
		t.Run(mode, func(t *testing.T) {
			notAfter := 17 * 24 * time.Hour
			if mode == "valid-certificate" {
				notAfter = 90 * 24 * time.Hour
			}
			m, fake, id, input := certificateRenewFixture(t, notAfter)
			switch mode {
			case "version":
				input.ExpectedResourceVersion = "sha256:" + strings.Repeat("a", 64)
			case "identity":
				input.PrimaryDomain = "other.example.com"
			case "bad-domain":
				input.PrimaryDomain = "Example.COM;rm"
			case "custom-binding":
				path := filepath.Join(m.webRoot, "conf.d/example.com.conf")
				body, _ := os.ReadFile(path)
				body = []byte(strings.ReplaceAll(string(body), "/etc/nginx/certs/example.com_cert.pem", "/etc/nginx/certs/elsewhere.pem"))
				if err := os.WriteFile(path, body, 0o600); err != nil {
					t.Fatal(err)
				}
				input.ExpectedResourceVersion = ""
			}
			if _, err := m.RenewCertificate(context.Background(), id, input); err == nil || fake.calls != 0 {
				t.Fatalf("unsafe renewal: %v calls=%d", err, fake.calls)
			}
		})
	}
}

func TestRenewCertificateFailureKeepsCertificateAndSurfacesReason(t *testing.T) {
	m, fake, id, input := certificateRenewFixture(t, 17*24*time.Hour)
	fake.err = certificateRenewalResult([]byte("KPANEL_CERTIFICATE failed\n"), false, "example.com")
	before, _ := os.ReadFile(filepath.Join(m.webRoot, "certs/example.com_cert.pem"))
	if _, err := m.RenewCertificate(context.Background(), id, input); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(m.webRoot, "certs/example.com_cert.pem"))
	if string(before) != string(after) {
		t.Fatal("failed renewal changed the certificate")
	}
}

func TestRenewCertificateRejectsReceiptWithoutNewerCertificate(t *testing.T) {
	m, fake, id, input := certificateRenewFixture(t, 17*24*time.Hour)
	fake.issue = nil // script claimed success but left the old certificate
	if _, err := m.RenewCertificate(context.Background(), id, input); !errors.Is(err, ErrNeedsAttention) {
		t.Fatalf("unreconciled success accepted: %v", err)
	}
}

func TestCertificateRenewalResultMapsOnlyFixedReceipts(t *testing.T) {
	cases := []struct {
		output    string
		succeeded bool
		want      error
	}{
		{"KPANEL_CERTIFICATE renewed example.com\n", true, nil},
		{"KPANEL_CERTIFICATE renewed example.com\n", false, ErrUnavailable},
		{"KPANEL_CERTIFICATE renewed other.com\n", true, ErrNeedsAttention},
		{"KPANEL_CERTIFICATE renewed example.com", true, ErrNeedsAttention},
		{"KPANEL_CERTIFICATE needs_attention\n", false, ErrNeedsAttention},
		{"KPANEL_CERTIFICATE busy\n", false, ErrUnavailable},
		{"KPANEL_CERTIFICATE custom\n", false, ErrUnprocessable},
		{"KPANEL_CERTIFICATE not_managed\n", false, ErrUnprocessable},
		{"KPANEL_CERTIFICATE unavailable\n", false, ErrUnprocessable},
		{"KPANEL_CERTIFICATE failed\n", false, ErrUnavailable},
		{"raw certbot noise: secret\n", false, ErrUnavailable},
	}
	for _, c := range cases {
		err := certificateRenewalResult([]byte(c.output), c.succeeded, "example.com")
		if c.want == nil {
			if err != nil {
				t.Fatalf("%q: %v", c.output, err)
			}
			continue
		}
		if !errors.Is(err, c.want) || strings.Contains(err.Error(), "secret") {
			t.Fatalf("%q: %v", c.output, err)
		}
	}
}

type certificateRenewControlRunner struct{ backend string }

func (runner certificateRenewControlRunner) LookPath(name string) (string, error) {
	if (runner.backend == "systemd" && name == "systemd-run") ||
		(runner.backend == "openrc" && (name == "start-stop-daemon" || name == "rc-service")) {
		return "/usr/bin/" + name, nil
	}
	return "", errors.New("not found")
}

func (runner certificateRenewControlRunner) InitRuntimeDirectoryExists(path string) bool {
	return (runner.backend == "systemd" && path == "/run/systemd/system") ||
		(runner.backend == "openrc" && path == "/run/openrc")
}

func (certificateRenewControlRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return nil, errors.New("availability checks must not execute commands")
}

func TestCertificateRenewalRequiresSystemdCleanupBeforeExecution(t *testing.T) {
	for _, backend := range []string{"systemd", "openrc", "unavailable"} {
		t.Run(backend, func(t *testing.T) {
			err := certificateRenewControlAvailable(certificateRenewControlRunner{backend: backend})
			if (err == nil) != (backend == "systemd") {
				t.Fatalf("backend=%s: %v", backend, err)
			}
		})
	}
}
