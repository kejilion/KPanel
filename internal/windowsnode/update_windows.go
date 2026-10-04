//go:build windows

package windowsnode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

var stableTag = regexp.MustCompile(`^v(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})\.(0|[1-9][0-9]{0,5})$`)

const maximumBinaryBytes = int64(64 << 20)

type UpdateTransaction struct {
	Phase          string `json:"phase"`
	PreviousSHA256 string `json:"previousSHA256"`
	NextSHA256     string `json:"nextSHA256"`
}
type Updater struct {
	Directory string
	Journal   string
	Stop      func() error
	Start     func() error
	Healthy   func(context.Context) error
	Store     UpdateStore
}

func NewUpdater(health func(context.Context) error) *Updater {
	return &Updater{Directory: InstallDir(), Journal: filepath.Join(DataDir(), "update-transaction.json"), Stop: StopServices, Start: StartServices, Healthy: health}
}
func (u *Updater) paths() (string, string, string) {
	return filepath.Join(u.Directory, "kejilion-node.exe"), filepath.Join(u.Directory, "kejilion-node.exe.old"), filepath.Join(u.Directory, "staging", "kejilion-node.exe.next")
}

type UpdateStore interface {
	ReadJournal(string) ([]byte, error)
	WriteJournal(string, []byte) error
	Hash(string) (string, error)
	Move(string, string) error
	Remove(string) error
}
type diskUpdateStore struct{}

func (diskUpdateStore) ReadJournal(path string) ([]byte, error) {
	return ReadFile(path, 2048, SystemOnly)
}
func (diskUpdateStore) WriteJournal(path string, data []byte) error {
	return WriteAtomic(path, data, SystemOnly)
}
func (diskUpdateStore) Hash(path string) (string, error) { return hashFile(path) }
func (diskUpdateStore) Move(from, to string) error       { return durableMove(from, to) }
func (diskUpdateStore) Remove(path string) error         { return os.Remove(path) }
func (u *Updater) store() UpdateStore {
	if u.Store != nil {
		return u.Store
	}
	return diskUpdateStore{}
}
func (u *Updater) transaction(tx UpdateTransaction) error {
	content, err := json.Marshal(tx)
	if err != nil {
		return err
	}
	return u.store().WriteJournal(u.Journal, content)
}
func hashFile(path string) (string, error) {
	// Open the object itself, rejecting reparse points before any digest read.
	handle, err := openChecked(path, ProgramRead, false, windows.GENERIC_READ)
	if err != nil {
		return "", err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	if err := ValidateFile(file, ProgramRead); err != nil {
		return "", err
	}
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(file, maximumBinaryBytes+1))
	if err != nil {
		return "", err
	}
	if n == 0 || n > maximumBinaryBytes {
		return "", errors.New("binary exceeds update limit")
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
func durableMove(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	for i := 0; i < 5; i++ {
		err = windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

// Recover is called by the separate bootstrap executable before every update.
// Neither the SCM services nor the scheduler depend on the replaceable live EXE
// being present while a transaction is interrupted between its two renames.
func (u *Updater) Recover(ctx context.Context) error {
	content, err := u.store().ReadJournal(u.Journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var tx UpdateTransaction
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&tx) != nil || !validSHA(tx.PreviousSHA256) || !validSHA(tx.NextSHA256) {
		return errors.New("invalid update recovery journal")
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return errors.New("invalid update recovery journal")
	}
	if tx.Phase != "prepared" && tx.Phase != "stopped" && tx.Phase != "swapped" && tx.Phase != "committed" {
		return errors.New("invalid update phase")
	}
	live, old, next := u.paths()
	liveHash, _ := u.store().Hash(live)
	if tx.Phase == "committed" && liveHash == tx.NextSHA256 {
		if err := u.Start(); err != nil {
			return err
		}
		if err := u.Healthy(ctx); err != nil {
			return err
		}
		return u.store().Remove(u.Journal)
	}
	if err := u.Stop(); err != nil {
		return err
	}
	if liveHash != tx.PreviousSHA256 {
		oldHash, err := u.store().Hash(old)
		if err != nil || oldHash != tx.PreviousSHA256 {
			return errors.New("verified rollback binary unavailable")
		}
		if err := u.store().Move(old, live); err != nil {
			return err
		}
	}
	if err := u.Start(); err != nil {
		return err
	}
	if err := u.Healthy(ctx); err != nil {
		return err
	}
	if err := u.store().Remove(next); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return u.store().Remove(u.Journal)
}
func (u *Updater) Apply(ctx context.Context, expected string) error {
	if !validSHA(expected) {
		return errors.New("invalid expected release digest")
	}
	live, old, next := u.paths()
	nextHash, err := u.store().Hash(next)
	if err != nil {
		return err
	}
	if nextHash != expected {
		return errors.New("release checksum mismatch")
	}
	previous, err := u.store().Hash(live)
	if err != nil {
		return err
	}
	tx := UpdateTransaction{Phase: "prepared", PreviousSHA256: previous, NextSHA256: nextHash}
	if err := u.transaction(tx); err != nil {
		return err
	}
	fail := func(cause error) error {
		if err := u.Recover(ctx); err != nil {
			return fmt.Errorf("update failed (%v), recovery pending: %w", cause, err)
		}
		return fmt.Errorf("update failed and rolled back: %w", cause)
	}
	if err := u.Stop(); err != nil {
		return fail(err)
	}
	tx.Phase = "stopped"
	if err := u.transaction(tx); err != nil {
		return fail(err)
	}
	if err := u.store().Move(live, old); err != nil {
		return fail(err)
	}
	if err := u.store().Move(next, live); err != nil {
		return fail(err)
	}
	tx.Phase = "swapped"
	if err := u.transaction(tx); err != nil {
		return fail(err)
	}
	if err := u.Start(); err != nil {
		return fail(err)
	}
	if err := u.Healthy(ctx); err != nil {
		return fail(err)
	}
	tx.Phase = "committed"
	if err := u.transaction(tx); err != nil {
		return fail(err)
	}
	return u.store().Remove(u.Journal)
}
func validSHA(value string) bool {
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 32 && value == strings.ToLower(value)
}

func ReleaseClient() *http.Client {
	return &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 5 || !releaseURL(request.URL) {
			return errors.New("release redirect outside trusted HTTPS hosts")
		}
		return nil
	}}
}
func releaseURL(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") && (u.Hostname() == "github.com" || u.Hostname() == "release-assets.githubusercontent.com" || u.Hostname() == "objects.githubusercontent.com")
}
func fetch(ctx context.Context, client *http.Client, endpoint string, limit int64) ([]byte, *url.URL, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, nil, err
	}
	if !releaseURL(request.URL) {
		return nil, nil, errors.New("untrusted release URL")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return nil, nil, errors.New("release response unavailable or too large")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, nil, errors.New("release response exceeds limit")
	}
	return data, response.Request.URL, nil
}
func DownloadUpdate(ctx context.Context, current string) (string, string, error) {
	client := ReleaseClient()
	// GitHub's signed asset redirect loses the version. Resolve the release tag
	// separately exactly once, then fetch BOTH manifest and binary at that tag.
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://github.com/kejilion/KPanel/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	response.Body.Close()
	prefix := "/kejilion/KPanel/releases/tag/"
	if response.StatusCode != 200 || response.Request.URL.Hostname() != "github.com" || !strings.HasPrefix(response.Request.URL.Path, prefix) {
		return "", "", errors.New("cannot resolve stable release")
	}
	tag := strings.TrimPrefix(response.Request.URL.Path, prefix)
	if !stableTag.MatchString(tag) {
		return "", "", errors.New("latest release is not a stable version")
	}
	if !newerVersion(tag, current) {
		return "", tag, nil
	}
	base := "https://github.com/kejilion/KPanel/releases/download/" + tag + "/"
	manifest, _, err := fetch(ctx, client, base+"SHA256SUMS", 64<<10)
	if err != nil {
		return "", "", err
	}
	asset := "kejilion-node-windows-" + runtime.GOARCH + ".exe"
	expected := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		if expected != "" || !validSHA(fields[0]) {
			return "", "", errors.New("invalid or duplicate release checksum")
		}
		expected = fields[0]
	}
	if expected == "" {
		return "", "", errors.New("Windows artifact is missing from stable release")
	}
	data, _, err := fetch(ctx, client, base+asset, maximumBinaryBytes)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != expected {
		return "", "", errors.New("download checksum mismatch")
	}
	path := filepath.Join(InstallDir(), "staging", "kejilion-node.exe.next")
	return expected, tag, WriteAtomic(path, data, ProgramRead)
}
func newerVersion(next, current string) bool {
	parse := func(value string) [3]int64 {
		value = strings.TrimPrefix(value, "v")
		value = strings.Split(value, "-")[0]
		parts := strings.Split(value, ".")
		var result [3]int64
		if len(parts) != 3 {
			return result
		}
		for i, p := range parts {
			result[i], _ = strconv.ParseInt(p, 10, 64)
		}
		return result
	}
	a, b := parse(next), parse(current)
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return strings.Contains(current, "-")
}
