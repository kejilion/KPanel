package sites

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func makeCustomCertificateMaterial(t *testing.T, domain string, notBefore, notAfter time.Time) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func makeCustomPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

func TestScriptSiteInputFlattensCertificateFieldsAlongsideSiteInput(t *testing.T) {
	var input ScriptSiteInput
	if err := json.Unmarshal([]byte(`{"primaryDomain":"custom.example.com","type":"static","certificate":"CERT","privateKey":"KEY"}`), &input); err != nil {
		t.Fatal(err)
	}
	if input.PrimaryDomain != "custom.example.com" || input.Type != "static" ||
		input.Certificate != "CERT" || input.PrivateKey != "KEY" || !input.HasCustomCertificate() {
		t.Fatalf("script site input was not flattened correctly: %#v", input)
	}
}

func TestSiteCommandEnvironmentDropsInheritedCertificatePaths(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"KJ_WEB_CERTIFICATE_FILE=/inherited/certificate.pem",
		"KJ_WEB_PRIVATE_KEY_FILE=/inherited/private-key.pem",
		"KJ_WEB_DOMAIN=example.com",
	}
	extra := []string{"KJ_WEB_CERTIFICATE_FILE=/staged/certificate.pem"}
	environment := sanitizeSiteCommandEnvironment(base, extra)
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "/inherited/") || !strings.Contains(joined, "/staged/certificate.pem") ||
		!strings.Contains(joined, "KJ_WEB_DOMAIN=example.com") {
		t.Fatalf("certificate environment was not sanitized: %s", joined)
	}
}

func TestNormalizeCustomCertificateInputValidatesCertificateAndKey(t *testing.T) {
	now := time.Now()
	certificate, privateKey := makeCustomCertificateMaterial(
		t, "custom.example.com", now.Add(-time.Hour), now.Add(time.Hour),
	)
	valid, err := normalizeCustomCertificateInput(certificate, privateKey, "custom.example.com")
	if err != nil || !valid.present() {
		t.Fatalf("valid certificate rejected: %#v, %v", valid, err)
	}
	if !strings.HasSuffix(valid.certificate, "\n") || !strings.HasSuffix(valid.privateKey, "\n") {
		t.Fatal("normalized certificate material must end with a newline")
	}

	tests := []struct {
		name        string
		certificate string
		privateKey  string
		domain      string
	}{
		{name: "missing private key", certificate: certificate, domain: "custom.example.com"},
		{name: "wrong domain", certificate: certificate, privateKey: privateKey, domain: "other.example.com"},
		{name: "mismatched key", certificate: certificate, privateKey: makeCustomPrivateKey(t), domain: "custom.example.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeCustomCertificateInput(test.certificate, test.privateKey, test.domain); err == nil {
				t.Fatal("invalid certificate material was accepted")
			}
		})
	}

	expiredCertificate, expiredKey := makeCustomCertificateMaterial(
		t, "custom.example.com", now.Add(-2*time.Hour), now.Add(-time.Hour),
	)
	if _, err := normalizeCustomCertificateInput(expiredCertificate, expiredKey, "custom.example.com"); err == nil {
		t.Fatal("expired certificate was accepted")
	}
}

func TestCustomCertificateStagingUsesPrivateFilesAndCleansUp(t *testing.T) {
	now := time.Now()
	certificate, privateKey := makeCustomCertificateMaterial(
		t, "custom.example.com", now.Add(-time.Hour), now.Add(time.Hour),
	)
	normalized, err := normalizeCustomCertificateInput(certificate, privateKey, "custom.example.com")
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	jobID := "0123456789abcdef0123456789abcdef"
	if err := stageCustomCertificateFiles(stateDir, jobID, normalized); err != nil {
		t.Fatal(err)
	}
	paths, err := customCertificateFilePaths(stateDir, jobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.certificate, paths.privateKey} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("unsafe staged certificate file %q: %v", path, err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("unsafe staged certificate file %q: mode %#o", path, info.Mode().Perm())
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != customCertificateMode {
			t.Fatalf("staged certificate file %q is not private: mode %#o", path, info.Mode().Perm())
		}
	}

	invocation, err := withCustomCertificateInvocation(
		templateInvocation("custom.example.com", scriptTemplateDefinitions["static"]),
		stateDir,
		jobID,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append(invocation.environment, invocation.required...), "\n")
	if !strings.Contains(joined, "KJ_WEB_CERTIFICATE_FILE="+paths.certificate) ||
		!strings.Contains(joined, "KJ_WEB_PRIVATE_KEY_FILE="+paths.privateKey) {
		t.Fatalf("custom certificate paths were not passed to the invocation: %s", joined)
	}
	state, err := json.Marshal(RecipeJob{ID: jobID, Domain: "custom.example.com", Recipe: "static-site"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "BEGIN CERTIFICATE") || strings.Contains(string(state), "PRIVATE KEY") {
		t.Fatal("private certificate material was persisted in recipe job state")
	}

	cleanupCustomCertificateFiles(stateDir, jobID)
	for _, path := range []string{paths.certificate, paths.privateKey} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("staged certificate file was not cleaned up: %q, %v", path, err)
		}
	}
	if _, err := withCustomCertificateInvocation(
		templateInvocation("custom.example.com", scriptTemplateDefinitions["static"]),
		stateDir,
		jobID,
		true,
	); err == nil {
		t.Fatal("custom certificate job fell back to automatic mode after its files were removed")
	}
}

func TestCleanupOrphanCustomCertificateFilesPreservesActiveJob(t *testing.T) {
	now := time.Now()
	certificate, privateKey := makeCustomCertificateMaterial(
		t, "custom.example.com", now.Add(-time.Hour), now.Add(time.Hour),
	)
	normalized, err := normalizeCustomCertificateInput(certificate, privateKey, "custom.example.com")
	if err != nil {
		t.Fatal(err)
	}
	stateDir := t.TempDir()
	activeID := "0123456789abcdef0123456789abcdef"
	orphanID := "fedcba9876543210fedcba9876543210"
	for _, id := range []string{activeID, orphanID} {
		if err := stageCustomCertificateFiles(stateDir, id, normalized); err != nil {
			t.Fatal(err)
		}
	}
	registry := newRecipeJobRegistry(stateDir)
	registry.jobs[activeID] = RecipeJob{ID: activeID, Status: "running"}
	cleanupOrphanCustomCertificateFiles(registry)
	activePaths, _ := customCertificateFilePaths(stateDir, activeID)
	orphanPaths, _ := customCertificateFilePaths(stateDir, orphanID)
	if _, err := os.Lstat(activePaths.certificate); err != nil {
		t.Fatalf("active certificate was removed: %v", err)
	}
	if _, err := os.Lstat(orphanPaths.certificate); !os.IsNotExist(err) {
		t.Fatalf("orphan certificate was not removed: %v", err)
	}
	cleanupCustomCertificateFiles(stateDir, activeID)
}
