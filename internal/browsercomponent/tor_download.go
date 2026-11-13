package browsercomponent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const torArchiveURL = "https://dist.torproject.org/torbrowser/16.0.1/tor-browser-linux-x86_64-16.0.1.tar.xz"
const torArchiveSHA256 = "5b36ad610c20c81faa72caf27d1cd3a11f14a327c320b4cec581400016ba78b0"

func downloadTor(ctx context.Context, directory string) error {
	client := &http.Client{Timeout: 8 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return downloadTorArchive(ctx, client, torArchiveURL, filepath.Join(directory, "tor-browser.tar.xz"), torArchiveSHA256, 256<<20)
}

// Stream to one private temporary file with a hard byte bound. A failed or
// cancelled download never replaces the last complete, checksum-verified file.
func downloadTorArchive(ctx context.Context, client *http.Client, source, target, expected string, limit int64) error {
	// The worker holds worker.lock. Reuse one fixed partial name so abrupt
	// worker termination cannot accumulate another archive on every retry.
	partial := filepath.Join(filepath.Dir(target), ".tor-download.part")
	if info, err := os.Lstat(partial); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !rootOwned(info) {
			return ErrInvalid
		}
		if err := os.Remove(partial); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !rootOwned(info) || info.Size() > limit {
			return ErrInvalid
		}
		file, err := os.Open(target)
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			file.Close()
			return ErrConflict
		}
		hash := sha256.New()
		_, err = io.Copy(hash, io.LimitReader(file, limit+1))
		file.Close()
		if err == nil && hex.EncodeToString(hash.Sum(nil)) == expected {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return errors.New("browser_tor_download_failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > limit {
		return errors.New("browser_tor_download_failed")
	}
	file, err := os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, limit+1))
	if err != nil || size > limit || hex.EncodeToString(hash.Sum(nil)) != expected {
		return errors.New("browser_tor_download_integrity_failed")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), target)
}
