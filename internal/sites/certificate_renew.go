package sites

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

// Manual renewal calls `k ssl <domain>` with the KPanel-only force switch. The
// script keeps the automatic renewal job untouched; this is an extra trigger.
var certificateRenewRequirements = []string{
	`KPANEL_WEB_CERTIFICATE_FORCE_RENEW_PROTOCOL_VERSION="1"`,
	"kpanel_web_force_renew_certificate()",
}

type siteCertificateRenewer interface {
	Available() error
	Renew(context.Context, string) error
}

type scriptCertificateRenewer struct{}

type RenewCertificateInput struct {
	PrimaryDomain           string `json:"primaryDomain"`
	ExpectedResourceVersion string `json:"expectedResourceVersion,omitempty"`
}

func (scriptCertificateRenewer) Available() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("certificate renewal requires Linux")
	}
	if _, err := findTrustedKejilionScript(certificateRenewRequirements...); err != nil {
		return err
	}
	return jobcontrol.Available(systemRecipeJobRunner{})
}

func (m *Manager) CertificateRenewWritable() error {
	if m.certificateRenewer == nil {
		return fmt.Errorf("certificate renewal adapter unavailable")
	}
	return m.certificateRenewer.Available()
}

// RenewCertificate requests a new certificate for a script-managed site whose
// certificate is expiring or expired. It accepts no configuration edits and
// reconciles the result from the certificate file instead of trusting output.
func (m *Manager) RenewCertificate(
	ctx context.Context,
	id string,
	input RenewCertificateInput,
) (contract.SiteSummary, error) {
	normalized, err := normalizeScriptDomain(input.PrimaryDomain)
	if err != nil || normalized != input.PrimaryDomain {
		return contract.SiteSummary{}, fmt.Errorf("%w: primaryDomain must be a valid normalized domain", ErrInvalidInput)
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	current, err := m.findActionableByID(id, "delete")
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if current.PrimaryDomain != normalized {
		return contract.SiteSummary{}, fmt.Errorf("%w: site identity and primaryDomain do not match", ErrConflict)
	}
	if input.ExpectedResourceVersion != "" && current.ResourceVersion != input.ExpectedResourceVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: site changed since the certificate status was shown", ErrConflict)
	}
	if current.TLS.Status != "expiring" && current.TLS.Status != "expired" {
		return contract.SiteSummary{}, fmt.Errorf("%w: the certificate is not expiring or expired", ErrUnprocessable)
	}
	if err := m.CertificateRenewWritable(); err != nil {
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
	if err := m.certificateRenewer.Renew(ctx, current.PrimaryDomain); err != nil {
		return contract.SiteSummary{}, err
	}
	renewed, err := m.findActionableByID(id, "delete")
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: renewed site could not be rediscovered", ErrNeedsAttention)
	}
	if renewed.TLS.Status != "valid" || renewed.TLS.ExpiresAt == nil ||
		(current.TLS.ExpiresAt != nil && !renewed.TLS.ExpiresAt.After(*current.TLS.ExpiresAt)) {
		return contract.SiteSummary{}, fmt.Errorf("%w: certificate result could not be reconciled", ErrNeedsAttention)
	}
	return renewed, nil
}

func (scriptCertificateRenewer) Renew(ctx context.Context, domain string) error {
	script, err := findTrustedKejilionScript(certificateRenewRequirements...)
	if err != nil {
		return fmt.Errorf("%w: certificate renewal protocol unavailable", ErrUnavailable)
	}
	controlRunner := systemRecipeJobRunner{}
	if err := jobcontrol.Available(controlRunner); err != nil {
		return fmt.Errorf("%w: certificate worker unavailable", ErrUnavailable)
	}
	// The script owns Nginx downtime and rollback; let it finish even if the
	// browser disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Minute)
	defer cancel()
	spec := jobcontrol.Spec{
		Unit:        "kpanel-certificate-renew-" + stableID(domain, time.Now().UTC().String())[:24],
		Executable:  "/bin/bash",
		Arguments:   []string{script, "ssl", domain},
		Environment: []string{"LC_ALL=C.UTF-8", "LANG=C.UTF-8", "KJ_WEB_FORCE_RENEW=1", "KJ_WEB_NONINTERACTIVE=1"},
		StateDir:    "/var/lib/kejilion-panel/wordpress-jobs",
		SystemdProperties: []string{
			"Type=exec", "RuntimeMaxSec=450s", "TimeoutStopSec=30s",
			"User=root", "UMask=0077",
		},
		UMask: "0077",
	}
	name, args, _, err := jobcontrol.ForegroundInvocation(controlRunner, spec)
	if err != nil {
		return fmt.Errorf("%w: certificate worker unavailable", ErrUnavailable)
	}
	command := exec.CommandContext(ctx, name, args...)
	command.Env = siteCommandEnvironment(spec.Environment)
	var receipt certificateReceipt
	command.Stdout = &receipt
	// Certbot output is discarded by the script; only fixed receipts are read.
	runErr := command.Run()
	return certificateRenewalResult(receipt.value, runErr == nil, domain)
}

func certificateRenewalResult(receipt []byte, succeeded bool, domain string) error {
	lines := strings.Split(string(receipt), "\n")
	hasReceipt := func(value string) bool {
		for _, line := range lines[:len(lines)-1] {
			if strings.TrimSuffix(line, "\r") == "KPANEL_CERTIFICATE "+value {
				return true
			}
		}
		return false
	}
	if succeeded && hasReceipt("renewed "+domain) {
		return nil
	}
	switch {
	case hasReceipt("needs_attention"):
		return fmt.Errorf("%w: 证书续签后恢复失败，请检查站点证书与 Nginx 状态", ErrNeedsAttention)
	case hasReceipt("busy"):
		return fmt.Errorf("%w: 另一项证书操作正在进行，请稍后重试", ErrUnavailable)
	case hasReceipt("custom"):
		return fmt.Errorf("%w: 该站点使用自定义证书，请更换证书而不是自动申请", ErrUnprocessable)
	case hasReceipt("not_managed"):
		return fmt.Errorf("%w: 当前证书不是 Let's Encrypt 签发，已拒绝自动替换", ErrUnprocessable)
	case hasReceipt("invalid"), hasReceipt("unavailable"):
		return fmt.Errorf("%w: 站点证书文件或 Nginx 容器不满足申请条件", ErrUnprocessable)
	case hasReceipt("failed"):
		return fmt.Errorf("%w: 证书申请失败，原证书保持不变；请检查 DNS 解析、80 端口和签发限额", ErrUnavailable)
	}
	if succeeded {
		return fmt.Errorf("%w: certificate renewal receipt missing", ErrNeedsAttention)
	}
	return fmt.Errorf("%w: 证书申请失败，请检查站点证书状态", ErrUnavailable)
}
