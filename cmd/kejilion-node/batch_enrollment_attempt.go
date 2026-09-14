package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/curve25519"
)

const maxBatchEnrollmentAttemptBytes = int64(4096)

type batchEnrollmentAttempt struct {
	SchemaVersion int    `json:"schemaVersion"`
	TokenHash     string `json:"tokenHash"`
	AttemptID     string `json:"attemptId"`
	Name          string `json:"name,omitempty"`
	PrivateKey    string `json:"privateKey"`
	PublicKey     string `json:"publicKey"`
}

func prepareBatchEnrollmentAttempt(
	path string,
	token string,
	name string,
	privateKey []byte,
	publicKey []byte,
) (batchEnrollmentAttempt, []byte, []byte, error) {
	if !filepath.IsAbs(path) || !validEnrollmentName(name) {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	tokenHash := sha256.Sum256([]byte(token))
	expectedHash := hex.EncodeToString(tokenHash[:])
	if _, err := os.Lstat(path); err == nil {
		attempt, storedPrivateKey, storedPublicKey, err := readBatchEnrollmentAttempt(path)
		if err != nil {
			return batchEnrollmentAttempt{}, nil, nil, err
		}
		if attempt.TokenHash != expectedHash || attempt.Name != name {
			return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state belongs to a different command or name")
		}
		return attempt, storedPrivateKey, storedPublicKey, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return batchEnrollmentAttempt{}, nil, nil, err
	}
	if len(privateKey) != 32 || len(publicKey) != 32 {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	attemptID, err := randomHex(16)
	if err != nil {
		return batchEnrollmentAttempt{}, nil, nil, err
	}
	attempt := batchEnrollmentAttempt{
		SchemaVersion: 1,
		TokenHash:     expectedHash,
		AttemptID:     attemptID,
		Name:          name,
		PrivateKey:    base64.RawURLEncoding.EncodeToString(privateKey),
		PublicKey:     base64.RawURLEncoding.EncodeToString(publicKey),
	}
	if err := writeBatchEnrollmentAttemptAtomic(path, attempt); err != nil {
		return batchEnrollmentAttempt{}, nil, nil, err
	}
	return attempt, append([]byte(nil), privateKey...), append([]byte(nil), publicKey...), nil
}

func readBatchEnrollmentAttempt(path string) (batchEnrollmentAttempt, []byte, []byte, error) {
	if !filepath.IsAbs(path) {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state path must be absolute")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is unavailable")
	}
	if runtime.GOOS != "windows" && (before.Mode().Perm() != 0o600 || !processOwned(before)) {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state permissions are unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return batchEnrollmentAttempt{}, nil, nil, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() > maxBatchEnrollmentAttemptBytes {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBatchEnrollmentAttemptBytes+1))
	if err != nil || int64(len(content)) > maxBatchEnrollmentAttemptBytes {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	var attempt batchEnrollmentAttempt
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attempt); err != nil {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	privateKey, privateErr := base64.RawURLEncoding.DecodeString(attempt.PrivateKey)
	publicKey, publicErr := base64.RawURLEncoding.DecodeString(attempt.PublicKey)
	derivedPublicKey, deriveErr := curve25519.X25519(privateKey, curve25519.Basepoint)
	if attempt.SchemaVersion != 1 || len(attempt.TokenHash) != sha256.Size*2 ||
		!validHexID(attempt.AttemptID) || !validEnrollmentName(attempt.Name) ||
		privateErr != nil || publicErr != nil || deriveErr != nil || len(privateKey) != 32 ||
		len(publicKey) != 32 || !bytes.Equal(derivedPublicKey, publicKey) {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	if _, err := hex.DecodeString(attempt.TokenHash); err != nil {
		return batchEnrollmentAttempt{}, nil, nil, errors.New("batch enrollment state is invalid")
	}
	return attempt, privateKey, publicKey, nil
}

func writeBatchEnrollmentAttemptAtomic(path string, attempt batchEnrollmentAttempt) error {
	if !filepath.IsAbs(path) {
		return errors.New("batch enrollment state path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			(runtime.GOOS != "windows" && (info.Mode().Perm() != 0o600 || !processOwned(info))) {
			return errors.New("batch enrollment state target is unsafe")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content, err := json.Marshal(attempt)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	directoryInfo, err := os.Lstat(directory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 ||
		(runtime.GOOS != "windows" && (!processOwned(directoryInfo) || directoryInfo.Mode().Perm()&0o022 != 0)) {
		return errors.New("batch enrollment state directory is unsafe")
	}
	temporary, err := os.OpenFile(
		filepath.Join(directory, ".batch-enrollment-attempt.tmp-"+strconv.FormatInt(time.Now().UnixNano(), 10)),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		0o600,
	)
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		directoryHandle, err := os.Open(directory)
		if err != nil {
			return err
		}
		defer directoryHandle.Close()
		if err := directoryHandle.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func removeBatchEnrollmentAttempt(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("batch enrollment state path must be absolute")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		(runtime.GOOS != "windows" && (info.Mode().Perm() != 0o600 || !processOwned(info))) {
		return errors.New("batch enrollment state target is unsafe")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		defer directory.Close()
		return directory.Sync()
	}
	return nil
}

func validEnrollmentName(value string) bool {
	if len([]rune(value)) > 80 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}
