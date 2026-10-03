package desktopcredentials

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T) (*Store, Binding, Credentials) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	return s, Binding{UserID: "panel-user", HostID: strings.Repeat("a", 32), UserVersion: 3, HostIdentity: strings.Repeat("b", 64)},
		Credentials{Username: "Windows测试", Domain: "DOMAIN", Password: "synthetic-secret-not-a-real-password"}
}

func TestEncryptedRoundTripBindingAndReplacement(t *testing.T) {
	s, binding, credentials := fixture(t)
	if err := s.Put(binding, credentials); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.root, "credentials.json")
	first, _ := os.ReadFile(file)
	for _, value := range []string{credentials.Username, credentials.Domain, credentials.Password} {
		if bytes.Contains(first, []byte(value)) {
			t.Fatal("plaintext persisted")
		}
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{file, filepath.Join(s.root, "key")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("credential file permissions", err)
			}
		}
	}
	loaded, err := Open(s.root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := loaded.Get(binding); err != nil || got != credentials {
		t.Fatal("restart failed", err)
	}
	for _, alter := range []func(*Binding){
		func(b *Binding) { b.UserID = "other" }, func(b *Binding) { b.HostID = strings.Repeat("c", 32) },
		func(b *Binding) { b.UserVersion++ }, func(b *Binding) { b.HostIdentity = strings.Repeat("d", 64) },
	} {
		other := binding
		alter(&other)
		if _, err := loaded.Get(other); !errors.Is(err, ErrMissing) {
			t.Fatal("binding bypass", err)
		}
	}
	if err := loaded.Put(binding, credentials); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(file)
	if bytes.Equal(first, second) {
		t.Fatal("nonce reused")
	}
	binding.UserVersion++
	if err := loaded.Put(binding, credentials); err != nil || len(loaded.state.Records) != 1 {
		t.Fatal("replacement accumulated stale revisions", err)
	}
}

func TestTamperingMissingKeyAndMalformedStateFailClosed(t *testing.T) {
	for _, mode := range []string{"ciphertext", "binding", "missing-key", "wrong-key", "duplicate", "version"} {
		t.Run(mode, func(t *testing.T) {
			s, binding, credentials := fixture(t)
			if err := s.Put(binding, credentials); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "ciphertext":
				s.state.Records[0].Sealed[30] ^= 1
			case "binding":
				s.state.Records[0].Binding.UserID = "attacker"
			case "missing-key":
				if err := os.Remove(filepath.Join(s.root, "key")); err != nil {
					t.Fatal(err)
				}
			case "wrong-key":
				if err := os.WriteFile(filepath.Join(s.root, "key"), bytes.Repeat([]byte{9}, 32), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate":
				s.state.Records = append(s.state.Records, s.state.Records[0])
			case "version":
				s.state.Version++
			}
			data, _ := json.Marshal(s.state)
			if err := os.WriteFile(filepath.Join(s.root, "credentials.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(s.root); err == nil {
				t.Fatal("corrupt store accepted")
			}
			if mode == "missing-key" {
				if _, err := os.Stat(filepath.Join(s.root, "key")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing encryption key silently regenerated")
				}
			}
		})
	}
}

func TestDeleteIsolationAndFailedPersistence(t *testing.T) {
	s, binding, credentials := fixture(t)
	otherUser, otherHost := binding, binding
	otherUser.UserID = "another-user"
	otherHost.HostID = strings.Repeat("e", 32)
	for _, b := range []Binding{binding, otherUser, otherHost} {
		if err := s.Put(b, credentials); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete(binding.UserID, binding.HostID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(binding); !errors.Is(err, ErrMissing) {
		t.Fatal(err)
	}
	if _, err := s.Get(otherUser); err != nil {
		t.Fatal("cleared another user's account", err)
	}
	if err := s.DeleteHost(binding.HostID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(otherUser); !errors.Is(err, ErrMissing) {
		t.Fatal("host deletion retained credentials", err)
	}
	if _, err := s.Get(otherHost); err != nil {
		t.Fatal("cleared another host", err)
	}
	file := filepath.Join(s.root, "credentials.json")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	credentials.Password = "changed-value"
	if err := s.Put(otherHost, credentials); err == nil {
		t.Fatal("failed write accepted")
	}
	got, err := s.Get(otherHost)
	if err != nil || got.Password == credentials.Password {
		t.Fatal("failed write changed active credential")
	}
	if err := s.Delete(otherHost.UserID, otherHost.HostID); err == nil {
		t.Fatal("failed delete accepted")
	}
	if _, err := s.Get(otherHost); err != nil {
		t.Fatal("failed delete changed state", err)
	}
}

func TestCredentialValidationLimits(t *testing.T) {
	s, binding, valid := fixture(t)
	for _, invalid := range []Credentials{
		{Username: "", Password: "p"}, {Username: " user", Password: "p"}, {Username: "a\n", Password: "p"},
		{Username: strings.Repeat("x", 257), Password: "p"}, {Username: "user", Domain: "x\x00", Password: "p"},
		{Username: "user", Password: ""}, {Username: "user", Password: "p\x00"}, {Username: "user", Password: strings.Repeat("x", 1025)},
	} {
		if err := s.Put(binding, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid credential accepted", err)
		}
	}
	valid.Password = "  spaces are significant  "
	if err := s.Put(binding, valid); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(binding); err != nil || got.Password != valid.Password {
		t.Fatal("password trimmed", err)
	}
}
