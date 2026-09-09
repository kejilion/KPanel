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

type fakeCertificateReplacer struct {
	root    string
	err     error
	calls   int
	staging string
}

func (f *fakeCertificateReplacer) Available() error { return nil }
func (f *fakeCertificateReplacer) Replace(_ context.Context, request certificateReplacement) error {
	f.calls++
	f.staging = filepath.Dir(request.keyPath)
	for _, path := range []string{request.certificatePath, request.keyPath} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			return errors.New("unsafe staged material")
		}
	}
	if f.err != nil {
		return f.err
	}
	for _, pair := range [][2]string{{request.certificatePath, "_cert.pem"}, {request.keyPath, "_key.pem"}} {
		body, err := os.ReadFile(pair[0])
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(f.root, "certs", request.domain+pair[1]), body, 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(f.root, "certs", request.domain+".custom"), []byte("custom-v1\n"), 0o600); err != nil {
		return err
	}
	return nil
}

func certificateReplacementFixture(t *testing.T) (*Manager, *fakeCertificateReplacer, string, ScriptSiteInput) {
	t.Helper()
	m, _, root := newTestManager(t)
	m.recipeJobs = newRecipeJobRegistry(t.TempDir())
	cert, key := makeCustomCertificateMaterial(t, "example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
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
	fake := &fakeCertificateReplacer{root: root}
	m.certificateReplacer = fake
	newCert, newKey := makeCustomCertificateMaterial(t, "example.com", time.Now().Add(-time.Hour), time.Now().Add(48*time.Hour))
	return m, fake, items[0].ID, ScriptSiteInput{SiteInput: SiteInput{PrimaryDomain: "example.com", Type: "static", ExpectedResourceVersion: items[0].ResourceVersion}, Certificate: newCert, PrivateKey: newKey}
}

func TestReplaceCertificateReconcilesAndCleansPrivateStaging(t *testing.T) {
	m, f, id, input := certificateReplacementFixture(t)
	before, _ := os.ReadFile(filepath.Join(m.webRoot, "conf.d/example.com.conf"))
	result, err := m.ReplaceCertificate(context.Background(), id, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.ResourceVersion == input.ExpectedResourceVersion || f.calls != 1 {
		t.Fatal("replacement not reconciled")
	}
	after, _ := os.ReadFile(filepath.Join(m.webRoot, "conf.d/example.com.conf"))
	if string(before) != string(after) {
		t.Fatal("replacement changed site configuration")
	}
	if _, err := os.Stat(f.staging); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("private staging retained")
	}
}

func TestReplaceCertificateRejectsConflictAndCombinedEdits(t *testing.T) {
	for _, mode := range []string{"version", "identity", "combined", "wrong-domain", "missing-key", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			m, f, id, input := certificateReplacementFixture(t)
			switch mode {
			case "version":
				input.ExpectedResourceVersion = "sha256:" + strings.Repeat("a", 64)
			case "identity":
				input.PrimaryDomain = "other.example.com"
			case "combined":
				input.Upstream = "http://127.0.0.1:80"
			case "wrong-domain":
				input.Certificate, input.PrivateKey = makeCustomCertificateMaterial(t, "other.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			case "missing-key":
				input.PrivateKey = ""
			case "oversize":
				input.Certificate = strings.Repeat("a", 16385)
			}
			if _, err := m.ReplaceCertificate(context.Background(), id, input); err == nil || f.calls != 0 {
				t.Fatalf("unsafe replacement: %v calls=%d", err, f.calls)
			}
		})
	}
}

func TestReplaceCertificateFailureCleansPrivateStaging(t *testing.T) {
	m, f, id, input := certificateReplacementFixture(t)
	f.err = ErrUnavailable
	if _, err := m.ReplaceCertificate(context.Background(), id, input); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.staging); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed replacement leaked staging")
	}
}

func TestCertificateReceiptIsBounded(t *testing.T) {
	var receipt certificateReceipt
	data := []byte(strings.Repeat("x", 8192))
	if n, err := receipt.Write(data); n != len(data) || err != nil || len(receipt.value) != 4096 {
		t.Fatal("unbounded output")
	}
}

func TestCertificateReplacementErrorReceipts(t *testing.T) {
	for _, test := range []struct {
		name, receipt string
		want          error
	}{
		{"renewal", "KPANEL_CERTIFICATE renewal_adapter_unavailable\n", ErrCertificateRenewalUnavailable},
		{"crlf", "KPANEL_CERTIFICATE renewal_adapter_unavailable\r\n", ErrCertificateRenewalUnavailable},
		{"conflict", "KPANEL_CERTIFICATE conflict\n", ErrConflict},
		{"recovery wins", "KPANEL_CERTIFICATE renewal_adapter_unavailable\nKPANEL_CERTIFICATE needs_attention\n", ErrNeedsAttention},
		{"unknown", "private-key=SECRET\n", ErrUnavailable},
		{"embedded", "log: KPANEL_CERTIFICATE renewal_adapter_unavailable\n", ErrUnavailable},
		{"truncated", "KPANEL_CERTIFICATE renewal_adapter_unavailable", ErrUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := certificateReplacementError([]byte(test.receipt))
			if !errors.Is(err, test.want) || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unsafe or incorrect error: %v", err)
			}
			if test.want != ErrCertificateRenewalUnavailable && errors.Is(err, ErrCertificateRenewalUnavailable) {
				t.Fatal("non-protocol output was treated as a renewal receipt")
			}
		})
	}
}

func TestReplaceCertificateCoversAllServedNames(t *testing.T) {
	m, f, id, input := certificateReplacementFixture(t)
	path := filepath.Join(m.webRoot, "conf.d/example.com.conf")
	config, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(config), "server_name example.com;", "server_name example.com www.example.com;", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	current, err := m.findActionableByID(id, "delete")
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedResourceVersion = current.ResourceVersion
	if _, err := m.ReplaceCertificate(context.Background(), id, input); err == nil || f.calls != 0 {
		t.Fatalf("certificate missing alias was accepted: %v", err)
	}
}
