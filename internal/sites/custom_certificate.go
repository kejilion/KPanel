package sites

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// The Agent request body is capped at 64 KiB. PEM newlines are escaped in
	// the Panel-to-Agent JSON hop, so keep the two fields comfortably below
	// that transport limit while allowing normal certificate chains and RSA
	// keys.
	maxSiteCertificateBytes = 16 << 10
	maxSitePrivateKeyBytes  = 8 << 10
	customCertificateMode   = 0o600
)

var customCertificateProtocolRequirements = []string{
	`KPANEL_WEB_CERTIFICATE_PROTOCOL_VERSION="1"`,
	"kpanel_web_prepare_custom_certificate()",
}

var customCertificateFilePattern = regexp.MustCompile(`^([a-f0-9]{32})\.(?:certificate|private-key)\.pem$`)

func siteCommandEnvironment(extra []string) []string {
	return sanitizeSiteCommandEnvironment(os.Environ(), extra)
}

func sanitizeSiteCommandEnvironment(base, extra []string) []string {
	environment := make([]string, 0, len(base)+len(extra))
	for _, value := range base {
		name, _, found := strings.Cut(value, "=")
		if found && (name == "KJ_WEB_CERTIFICATE_FILE" || name == "KJ_WEB_PRIVATE_KEY_FILE") {
			continue
		}
		environment = append(environment, value)
	}
	return append(environment, extra...)
}

type normalizedCustomCertificate struct {
	certificate string
	privateKey  string
}

// ScriptSiteInput carries the fields accepted by the kejilion.sh-backed
// website creation entry points. Keeping certificate material out of SiteInput
// prevents the legacy managed-template path from accepting and ignoring it.
type ScriptSiteInput struct {
	SiteInput
	Certificate string `json:"certificate,omitempty"`
	PrivateKey  string `json:"privateKey,omitempty"`
}

func (certificate normalizedCustomCertificate) present() bool {
	return certificate.certificate != "" || certificate.privateKey != ""
}

func (input ScriptSiteInput) HasCustomCertificate() bool {
	return strings.TrimSpace(input.Certificate) != "" || strings.TrimSpace(input.PrivateKey) != ""
}

func normalizeCustomCertificateInput(
	certificateValue string,
	privateKeyValue string,
	domain string,
) (normalizedCustomCertificate, error) {
	certificateValue = normalizePEMText(certificateValue)
	privateKeyValue = normalizePEMText(privateKeyValue)
	if certificateValue == "" && privateKeyValue == "" {
		return normalizedCustomCertificate{}, nil
	}
	if certificateValue == "" || privateKeyValue == "" {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate and privateKey must be supplied together",
			ErrUnprocessable,
		)
	}
	if len([]byte(certificateValue)) > maxSiteCertificateBytes {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate is too large",
			ErrUnprocessable,
		)
	}
	if len([]byte(privateKeyValue)) > maxSitePrivateKeyBytes {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: privateKey is too large",
			ErrUnprocessable,
		)
	}
	if hasUnsupportedPEMControlCharacter(certificateValue) ||
		hasUnsupportedPEMControlCharacter(privateKeyValue) {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate material contains unsupported control characters",
			ErrUnprocessable,
		)
	}

	certificates, canonicalCertificate, err := parseCertificateBundle(certificateValue)
	if err != nil {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate PEM is invalid",
			ErrUnprocessable,
		)
	}
	signer, canonicalPrivateKey, err := parsePrivateKey(privateKeyValue)
	if err != nil {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: privateKey PEM is invalid or encrypted",
			ErrUnprocessable,
		)
	}
	leaf := certificates[0]
	if err := leaf.VerifyHostname(domain); err != nil {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate does not cover the primary domain",
			ErrUnprocessable,
		)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate is not currently valid",
			ErrUnprocessable,
		)
	}
	certificatePublicKey, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate public key is unsupported",
			ErrUnprocessable,
		)
	}
	privatePublicKey, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil || !bytes.Equal(certificatePublicKey, privatePublicKey) {
		return normalizedCustomCertificate{}, fmt.Errorf(
			"%w: certificate and privateKey do not match",
			ErrUnprocessable,
		)
	}
	return normalizedCustomCertificate{
		certificate: canonicalCertificate,
		privateKey:  canonicalPrivateKey,
	}, nil
}

func normalizePEMText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func hasUnsupportedPEMControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 && character != '\n' && character != '\t' {
			return true
		}
		if character == 0x7f {
			return true
		}
	}
	return false
}

func parseCertificateBundle(value string) ([]*x509.Certificate, string, error) {
	remaining := []byte(value)
	certificates := make([]*x509.Certificate, 0, 2)
	var canonical bytes.Buffer
	for len(bytes.TrimSpace(remaining)) > 0 {
		remaining = bytes.TrimSpace(remaining)
		block, rest := pem.Decode(remaining)
		if block == nil || len(rest) >= len(remaining) || block.Type != "CERTIFICATE" {
			return nil, "", errors.New("invalid certificate bundle")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, "", err
		}
		certificates = append(certificates, certificate)
		canonical.Write(pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: block.Bytes}))
		remaining = rest
	}
	if len(certificates) == 0 {
		return nil, "", errors.New("certificate bundle is empty")
	}
	return certificates, canonical.String(), nil
}

func parsePrivateKey(value string) (crypto.Signer, string, error) {
	block, remaining := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil || len(bytes.TrimSpace(remaining)) != 0 || x509.IsEncryptedPEMBlock(block) {
		return nil, "", errors.New("private key PEM is invalid")
	}

	var (
		key any
		err error
	)
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, "", err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, "", errors.New("private key is not a signer")
	}
	return signer, string(pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: block.Bytes})), nil
}

type customCertificateFiles struct {
	certificate string
	privateKey  string
}

func customCertificateFilePaths(stateDir, id string) (customCertificateFiles, error) {
	if !recipeJobIDPattern.MatchString(id) {
		return customCertificateFiles{}, errors.New("invalid website job identity")
	}
	stateDir = filepath.Clean(stateDir)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) {
		return customCertificateFiles{}, errors.New("custom certificate state directory is invalid")
	}
	return customCertificateFiles{
		certificate: filepath.Join(stateDir, id+".certificate.pem"),
		privateKey:  filepath.Join(stateDir, id+".private-key.pem"),
	}, nil
}

func stageCustomCertificateFiles(
	stateDir string,
	id string,
	certificate normalizedCustomCertificate,
) error {
	if !certificate.present() {
		return nil
	}
	paths, err := customCertificateFilePaths(stateDir, id)
	if err != nil {
		return err
	}
	if err := writePrivateCertificateFile(paths.certificate, certificate.certificate); err != nil {
		return err
	}
	if err := writePrivateCertificateFile(paths.privateKey, certificate.privateKey); err != nil {
		_ = os.Remove(paths.certificate)
		return err
	}
	return nil
}

func writePrivateCertificateFile(path, value string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, customCertificateMode)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(customCertificateMode); err != nil {
		return err
	}
	if _, err := io.WriteString(file, value); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

func withCustomCertificateInvocation(
	invocation scriptSiteInvocation,
	stateDir string,
	id string,
	required bool,
) (scriptSiteInvocation, error) {
	paths, err := customCertificateFilePaths(stateDir, id)
	if err != nil {
		return scriptSiteInvocation{}, err
	}
	certificateInfo, certificateErr := os.Lstat(paths.certificate)
	privateKeyInfo, privateKeyErr := os.Lstat(paths.privateKey)
	certificateExists := certificateErr == nil
	privateKeyExists := privateKeyErr == nil
	if !certificateExists && !privateKeyExists {
		if !errors.Is(certificateErr, os.ErrNotExist) || !errors.Is(privateKeyErr, os.ErrNotExist) {
			return scriptSiteInvocation{}, errors.New("custom certificate files are unavailable")
		}
		if required {
			return scriptSiteInvocation{}, errors.New("custom certificate files are unavailable")
		}
		return invocation, nil
	}
	if certificateErr != nil || privateKeyErr != nil || !certificateExists || !privateKeyExists ||
		!certificateInfo.Mode().IsRegular() || certificateInfo.Mode()&os.ModeSymlink != 0 ||
		!privateKeyInfo.Mode().IsRegular() || privateKeyInfo.Mode()&os.ModeSymlink != 0 ||
		certificateInfo.Size() <= 0 || certificateInfo.Size() > maxSiteCertificateBytes ||
		privateKeyInfo.Size() <= 0 || privateKeyInfo.Size() > maxSitePrivateKeyBytes {
		return scriptSiteInvocation{}, errors.New("custom certificate files are unavailable")
	}
	invocation.environment = append(append([]string(nil), invocation.environment...),
		"KJ_WEB_CERTIFICATE_FILE="+paths.certificate,
		"KJ_WEB_PRIVATE_KEY_FILE="+paths.privateKey,
	)
	invocation.required = append(append([]string(nil), invocation.required...), customCertificateProtocolRequirements...)
	return invocation, nil
}

func cleanupCustomCertificateFiles(stateDir, id string) {
	paths, err := customCertificateFilePaths(stateDir, id)
	if err != nil {
		return
	}
	_ = os.Remove(paths.certificate)
	_ = os.Remove(paths.privateKey)
}

func cleanupOrphanCustomCertificateFiles(registry *recipeJobRegistry) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	active := make(map[string]bool, len(registry.jobs))
	for id, job := range registry.jobs {
		active[id] = job.Status == "queued" || job.Status == "running"
	}
	registry.mu.Unlock()
	entries, err := os.ReadDir(registry.stateDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		matches := customCertificateFilePattern.FindStringSubmatch(entry.Name())
		if len(matches) != 2 || active[matches[1]] {
			continue
		}
		path := filepath.Join(registry.stateDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		_ = os.Remove(path)
	}
}
