package sites

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var certificateReplaceRequirements = []string{
	`KPANEL_WEB_CERTIFICATE_REPLACE_PROTOCOL_VERSION="1"`,
	"kpanel_web_replace_certificate()",
}

var ErrCertificateRenewalUnavailable = fmt.Errorf("%w: certificate renewal adapter is incompatible or busy; existing certificates were not changed", ErrUnavailable)

type certificateReplacement struct {
	domain, configHash, certificateHash, keyHash string
	certificatePath, keyPath                     string
}

type siteCertificateReplacer interface {
	Available() error
	Replace(context.Context, certificateReplacement) error
}

type scriptCertificateReplacer struct{}

func (scriptCertificateReplacer) Available() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("certificate replacement requires Linux")
	}
	if _, err := findTrustedKejilionScript(certificateReplaceRequirements...); err != nil {
		return err
	}
	_, err := findSystemdRun()
	return err
}

func (m *Manager) CertificateReplaceWritable() error {
	if m.certificateReplacer == nil {
		return fmt.Errorf("certificate replacement adapter unavailable")
	}
	return m.certificateReplacer.Available()
}

// ReplaceCertificate deliberately accepts no simultaneous configuration edits.
// The script owns the existing certificate paths, transaction and Nginx reload.
func (m *Manager) ReplaceCertificate(ctx context.Context, id string, input ScriptSiteInput) (contract.SiteSummary, error) {
	if input.ExpectedResourceVersion == "" || !input.HasCustomCertificate() {
		return contract.SiteSummary{}, fmt.Errorf("%w: certificate and resource version required", ErrInvalidInput)
	}
	if len(input.Aliases) != 0 || input.Recipe != "" || input.Upstream != "" || len(input.Upstreams) != 0 || input.RedirectTarget != "" || input.RedirectCode != 0 || input.PHPVersion != "" || input.Enabled != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: replace certificates separately from site settings", ErrInvalidInput)
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	current, err := m.findActionableByID(id, "delete")
	if err != nil {
		return contract.SiteSummary{}, err
	}
	kind := map[string]contract.SiteKind{"static": contract.SiteStatic, "php": contract.SitePHP, "wordpress": contract.SiteWordPress, "proxy": contract.SiteReverseProxy, "proxy_domain": contract.SiteDomainProxy, "load_balance": contract.SiteLoadBalance, "redirect": contract.SiteRedirect}[input.Type]
	if current.PrimaryDomain != input.PrimaryDomain || current.Kind != kind || current.ResourceVersion != input.ExpectedResourceVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: site identity or resource version changed", ErrConflict)
	}
	material, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, current.PrimaryDomain)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	// Cover every served name, not only the primary domain.
	for _, domain := range current.Domains {
		if _, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, domain); err != nil {
			return contract.SiteSummary{}, err
		}
	}
	if err := m.CertificateReplaceWritable(); err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	configPath, _, config, err := m.verifiedDeleteConfig(current)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if configPath != filepath.Join(m.webRoot, "conf.d", current.PrimaryDomain+".conf") {
		return contract.SiteSummary{}, fmt.Errorf("%w: certificate target is not a script domain site", ErrForbidden)
	}
	directives := parseDirectives(stripComments(string(config)))
	for name, suffix := range map[string]string{"ssl_certificate": "_cert.pem", "ssl_certificate_key": "_key.pem"} {
		values := directives[name]
		if len(values) == 0 {
			return contract.SiteSummary{}, fmt.Errorf("%w: existing site has no script TLS binding", ErrUnprocessable)
		}
		for _, value := range values {
			if strings.TrimSpace(value) != "/etc/nginx/certs/"+current.PrimaryDomain+suffix {
				return contract.SiteSummary{}, fmt.Errorf("%w: existing TLS binding is not a script certificate path", ErrUnprocessable)
			}
		}
	}
	cert, err := readCertificateReplacementFile(filepath.Join(m.webRoot, "certs", current.PrimaryDomain+"_cert.pem"), maxSiteCertificateBytes)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	key, err := readCertificateReplacementFile(filepath.Join(m.webRoot, "certs", current.PrimaryDomain+"_key.pem"), maxSitePrivateKeyBytes)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	latest, err := m.findActionableByID(id, "delete")
	if err != nil || latest.ResourceVersion != input.ExpectedResourceVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: site changed before certificate replacement", ErrConflict)
	}
	if m.recipeJobs == nil || m.recipeJobs.stateDir == "" {
		return contract.SiteSummary{}, fmt.Errorf("%w: private certificate staging unavailable", ErrUnavailable)
	}
	dir, err := os.MkdirTemp(m.recipeJobs.stateDir, "certificate-replace-")
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: create private certificate staging", ErrUnavailable)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return contract.SiteSummary{}, err
	}
	request := certificateReplacement{domain: current.PrimaryDomain, configHash: hashBytes(config), certificateHash: hashBytes(cert), keyHash: hashBytes(key), certificatePath: filepath.Join(dir, "certificate.pem"), keyPath: filepath.Join(dir, "private-key.pem")}
	if err := writePrivateCertificateFile(request.certificatePath, material.certificate); err != nil {
		return contract.SiteSummary{}, err
	}
	if err := writePrivateCertificateFile(request.keyPath, material.privateKey); err != nil {
		return contract.SiteSummary{}, err
	}
	if err := m.certificateReplacer.Replace(ctx, request); err != nil {
		return contract.SiteSummary{}, err
	}
	actual, err := readCertificateReplacementFile(filepath.Join(m.webRoot, "certs", current.PrimaryDomain+"_cert.pem"), maxSiteCertificateBytes)
	if err != nil || strings.TrimSpace(string(actual)) != strings.TrimSpace(material.certificate) {
		return contract.SiteSummary{}, fmt.Errorf("%w: certificate result could not be reconciled", ErrNeedsAttention)
	}
	actualKey, err := readCertificateReplacementFile(filepath.Join(m.webRoot, "certs", current.PrimaryDomain+"_key.pem"), maxSitePrivateKeyBytes)
	if err != nil || strings.TrimSpace(string(actualKey)) != strings.TrimSpace(material.privateKey) {
		return contract.SiteSummary{}, fmt.Errorf("%w: private key result could not be reconciled", ErrNeedsAttention)
	}
	actualConfig, err := readRegularFile(configPath, maxConfigBytes)
	if err != nil || hashBytes(actualConfig) != request.configHash {
		return contract.SiteSummary{}, fmt.Errorf("%w: site configuration changed during certificate replacement", ErrNeedsAttention)
	}
	policy, err := readRegularFile(filepath.Join(m.webRoot, "certs", current.PrimaryDomain+".custom"), 32)
	if err != nil || string(policy) != "custom-v1\n" {
		return contract.SiteSummary{}, fmt.Errorf("%w: certificate renewal policy could not be reconciled", ErrNeedsAttention)
	}
	return m.findActionableByID(id, "delete")
}

func readCertificateReplacementFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("%w: certificate artifact unavailable or unsafe", ErrUnprocessable)
	}
	data, err := readRegularFile(path, maximum)
	if err != nil || int64(len(data)) > maximum {
		return nil, fmt.Errorf("%w: certificate artifact unavailable", ErrUnprocessable)
	}
	return data, nil
}

type certificateReceipt struct{ value []byte }

func (b *certificateReceipt) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.value) < 4096 {
		b.value = append(b.value, p[:min(len(p), 4096-len(b.value))]...)
	}
	return n, nil
}

func (scriptCertificateReplacer) Replace(ctx context.Context, input certificateReplacement) error {
	script, err := findTrustedKejilionScript(certificateReplaceRequirements...)
	if err != nil {
		return fmt.Errorf("%w: certificate protocol unavailable", ErrUnavailable)
	}
	runner, err := findSystemdRun()
	if err != nil {
		return fmt.Errorf("%w: certificate worker unavailable", ErrUnavailable)
	}
	// Let the bounded transaction finish recovery even if the browser disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Second)
	defer cancel()
	args := []string{"--wait", "--pipe", "--collect", "--quiet", "--property=Type=exec", "--property=RuntimeMaxSec=75s", "--property=TimeoutStopSec=15s", "--property=User=root", "--property=UMask=0077", "--setenv=KJ_WEB_NONINTERACTIVE=1", "--setenv=KJ_WEB_CERTIFICATE_FILE=" + input.certificatePath, "--setenv=KJ_WEB_PRIVATE_KEY_FILE=" + input.keyPath, "--", "/bin/bash", script, "web", "certificate-replace", input.domain, strings.TrimPrefix(input.configHash, "sha256:"), strings.TrimPrefix(input.certificateHash, "sha256:"), strings.TrimPrefix(input.keyHash, "sha256:")}
	args = append([]string{"--setenv=KJ_WEB_CERTIFICATE_EPHEMERAL=1"}, args...)
	command := exec.CommandContext(ctx, runner, args...)
	command.Env = siteCommandEnvironment(nil)
	var receipt certificateReceipt
	command.Stdout = &receipt
	// Raw script output must never be returned to audit/task/log consumers.
	if err := command.Run(); err != nil {
		return certificateReplacementError(receipt.value)
	}
	if !strings.Contains(string(receipt.value), "KPANEL_CERTIFICATE replaced "+input.domain+"\n") {
		return fmt.Errorf("%w: certificate replacement receipt missing", ErrNeedsAttention)
	}
	return nil
}

func certificateReplacementError(receipt []byte) error {
	// Interpret only complete fixed protocol lines, never return raw script output.
	lines := strings.Split(string(receipt), "\n")
	hasReceipt := func(value string) bool {
		for _, line := range lines[:len(lines)-1] {
			if strings.TrimSuffix(line, "\r") == "KPANEL_CERTIFICATE "+value {
				return true
			}
		}
		return false
	}
	code := ErrUnavailable
	if hasReceipt("needs_attention") {
		code = ErrNeedsAttention
	} else if hasReceipt("conflict") {
		code = ErrConflict
	} else if hasReceipt("renewal_adapter_unavailable") {
		return ErrCertificateRenewalUnavailable
	}
	return fmt.Errorf("%w: certificate replacement failed; inspect site certificate state", code)
}
