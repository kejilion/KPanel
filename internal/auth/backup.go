package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

// ValidateBackupIdentity checks credentials without executing a password KDF
// or touching the destination key file.
func ValidateBackupIdentity(user store.User, key []byte) error {
	if !usernamePattern.MatchString(user.Username) {
		return errors.New("invalid backup username")
	}
	if _, _, _, err := parseArgon2id(user.PasswordHash); err != nil {
		return err
	}
	if user.TOTPLastUsedStep < 0 || user.TOTPLastUsedStep > time.Now().Unix()/30+1 {
		return errors.New("invalid backup TOTP step")
	}
	if len(key) != 0 && len(key) != 32 {
		return errors.New("invalid backup TOTP key")
	}
	if user.TOTPSecret == "" {
		if user.TOTPEnabledAt != nil {
			return errors.New("missing backup TOTP secret")
		}
		return nil
	}
	sealed, err := base64.RawURLEncoding.DecodeString(user.TOTPSecret)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return errors.New("invalid backup TOTP secret")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("kpanel-totp-v1"))
	if err != nil {
		return err
	}
	defer clear(plain)
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(string(plain))
	if err != nil || len(decoded) != 20 {
		return errors.New("invalid backup TOTP secret")
	}
	return nil
}
