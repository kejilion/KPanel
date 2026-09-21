package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	totpDigits            = 6
	totpPeriod            = 30 * time.Second
	totpSecretBytes       = 20
	totpKeyBytes          = 32
	recoveryCodeCount     = 10
	recoveryCodeBytes     = 10
	enrollmentTTL         = 10 * time.Minute
	maxPendingEnrollments = 8
)

var (
	totpCodePattern     = regexp.MustCompile(`^[0-9]{6}$`)
	recoveryCodePattern = regexp.MustCompile(`^[A-Z2-7]{5}-[A-Z2-7]{5}-[A-Z2-7]{5}$`)
)

type TOTPEnrollment struct {
	ID         string    `json:"id"`
	Secret     string    `json:"secret"`
	OTPAuthURI string    `json:"otpauthUri"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type TOTPStatus struct {
	Enabled                bool       `json:"enabled"`
	EnabledAt              *time.Time `json:"enabledAt,omitempty"`
	RecoveryCodesRemaining int        `json:"recoveryCodesRemaining"`
}

type pendingTOTPEnrollment struct {
	id                string
	secret            string
	expiresAt         time.Time
	credentialVersion uint64
}

func generateTOTPSecret() (string, error) {
	buffer := make([]byte, totpSecretBytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate TOTP secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer), nil
}

func buildOTPAuthURI(username, secret string) string {
	issuer := "KPanel"
	label := issuer + ":" + username
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", "6")
	query.Set("period", "30")
	return (&url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawQuery: query.Encode()}).String()
}

func matchTOTPStep(secret, code string, now time.Time) (int64, bool) {
	if !totpCodePattern.MatchString(code) {
		return 0, false
	}
	current := now.Unix() / int64(totpPeriod/time.Second)
	for _, offset := range []int64{0, -1, 1} {
		step := current + offset
		generated, err := totpAtStep(secret, step)
		if err == nil && subtle.ConstantTimeCompare([]byte(generated), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func totpAtStep(secret string, step int64) (string, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil || len(decoded) < 16 {
		return "", errors.New("invalid TOTP secret")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, decoded)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000), nil
}

func generateRecoveryCodes() ([]string, []string, error) {
	codes := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		buffer := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(buffer); err != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", err)
		}
		raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buffer)[:15]
		code := raw[:5] + "-" + raw[5:10] + "-" + raw[10:]
		codes = append(codes, code)
		hashes = append(hashes, recoveryCodeHash(code))
	}
	return codes, hashes, nil
}

func normalizeRecoveryCode(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	if !strings.Contains(value, "-") && len(value) == 15 {
		value = value[:5] + "-" + value[5:10] + "-" + value[10:]
	}
	return value
}

func recoveryCodeHash(code string) string {
	digest := sha256.Sum256([]byte("kpanel-totp-recovery\x00" + normalizeRecoveryCode(code)))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func loadOrCreateTOTPKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return nil, errors.New("TOTP key path must be absolute")
	}
	if content, err := os.ReadFile(path); err == nil {
		if len(content) != totpKeyBytes {
			return nil, fmt.Errorf("TOTP key must contain %d bytes", totpKeyBytes)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("protect TOTP key: %w", err)
		}
		return content, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read TOTP key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create TOTP key directory: %w", err)
	}
	key := make([]byte, totpKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate TOTP key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return loadOrCreateTOTPKey(path)
	}
	if err != nil {
		return nil, fmt.Errorf("create TOTP key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write TOTP key: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync TOTP key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close TOTP key: %w", err)
	}
	return key, nil
}

func loadTOTPKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return nil, errors.New("TOTP key path must be absolute")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TOTP key: %w", err)
	}
	if len(content) != totpKeyBytes {
		return nil, fmt.Errorf("TOTP key must contain %d bytes", totpKeyBytes)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("protect TOTP key: %w", err)
	}
	return content, nil
}

func sealTOTPSecret(path, secret string) (string, error) {
	key, err := loadOrCreateTOTPKey(path)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(secret), []byte("kpanel-totp-v1"))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func openTOTPSecret(path, encoded string) (string, error) {
	key, err := loadTOTPKey(path)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("invalid encrypted TOTP secret")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) <= aead.NonceSize() {
		return "", errors.New("invalid encrypted TOTP secret")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("kpanel-totp-v1"))
	if err != nil {
		return "", errors.New("decrypt TOTP secret")
	}
	return string(plain), nil
}
