package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/flynn/noise"
)

const defaultTerminalConfigPath = "/etc/kejilion-node/terminal.json"

type terminalConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	PrivateKey    string `json:"privateKey"`
	PublicKey     string `json:"publicKey"`
	PeerPublicKey string `json:"peerPublicKey"`
}

type terminalIdentity struct {
	Key  noise.DHKey
	Peer []byte
}

func readTerminalConfig(path string) (terminalConfig, terminalIdentity, error) {
	if !filepath.IsAbs(path) {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration path must be absolute")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is unavailable")
	}
	if runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration permissions are unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return terminalConfig{}, terminalIdentity{}, err
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() > 8192 {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is invalid")
	}
	content, err := io.ReadAll(io.LimitReader(file, 8193))
	if err != nil || len(content) > 8192 {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is invalid")
	}
	var config terminalConfig
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is invalid")
	}
	privateKey, privateErr := decodeTerminalKey(config.PrivateKey)
	publicKey, publicErr := decodeTerminalKey(config.PublicKey)
	peerKey, peerErr := decodeTerminalKey(config.PeerPublicKey)
	if config.SchemaVersion != 1 || privateErr != nil || publicErr != nil || peerErr != nil {
		return terminalConfig{}, terminalIdentity{}, errors.New("terminal configuration file is invalid")
	}
	return config, terminalIdentity{
		Key: noise.DHKey{Private: privateKey, Public: publicKey}, Peer: peerKey,
	}, nil
}

func decodeTerminalKey(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("terminal key is invalid")
	}
	return decoded, nil
}

func writeTerminalConfigAtomic(path string, config terminalConfig) error {
	if !filepath.IsAbs(path) {
		return errors.New("terminal configuration path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("terminal configuration target is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content, err := json.Marshal(config)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	temporary, err := os.OpenFile(
		filepath.Join(directory, ".terminal.json.tmp-"+strconv.FormatInt(time.Now().UnixNano(), 10)),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600,
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

func removeTerminalConfig(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("terminal configuration path must be absolute")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("terminal configuration target is not a regular file")
	}
	return os.Remove(path)
}
