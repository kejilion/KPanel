package sites

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MaxNotificationCertificates = 128

var ErrNotificationResourceLimit = errors.New("notification certificate limit reached")

type NotificationCertificate struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Maintenance string     `json:"maintenance"`
	Known       bool       `json:"known"`
}

// Reuse the actual site parser and TLS discovery, with a hard directory bound.
// An over-limit directory is unknown as a whole, never a truncated empty list.
func (d *Discoverer) NotificationResources(ctx context.Context) ([]NotificationCertificate, error) {
	webRoot := d.WebRoot
	if webRoot == "" {
		webRoot = "/home/web"
	}
	confRoot := filepath.Join(webRoot, "conf.d")
	info, err := os.Lstat(confRoot)
	if errors.Is(err, os.ErrNotExist) {
		return []NotificationCertificate{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("notification config directory unavailable")
	}
	dir, err := os.Open(confRoot)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(MaxNotificationCertificates + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > MaxNotificationCertificates {
		return nil, ErrNotificationResourceLimit
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	result := []NotificationCertificate{}
	seen := map[string]bool{}
	now := time.Now().UTC()
	if d.Now != nil {
		now = d.Now().UTC()
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".conf" || name == "default.conf" || name == "map.conf" {
			continue
		}
		site, err := d.fromConfig(filepath.Join(confRoot, name), now)
		if err != nil {
			// Its former certificate identity cannot be reconstructed. Preserve
			// previous alert states by reporting an incomplete collection.
			return nil, errors.New("notification site discovery unavailable")
		}
		if !site.TLS.Enabled {
			continue
		}
		item := NotificationCertificate{Name: site.PrimaryDomain, ExpiresAt: site.TLS.ExpiresAt, Maintenance: "unknown"}
		var certificatePath string
		for _, artifact := range site.Artifacts {
			if artifact.Kind == "certificate" {
				certificatePath = artifact.Path
				item.Fingerprint = strings.TrimPrefix(artifact.Hash, "sha256:")
				break
			}
		}
		identity := certificatePath
		if identity == "" {
			identity = filepath.Join(confRoot, name)
		}
		item.ID = notificationHash([]byte(identity))
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		item.Known = item.ExpiresAt != nil && len(item.Fingerprint) == 64
		if item.Known {
			item.Maintenance = notificationCertificateMaintenance(webRoot, site.PrimaryDomain, certificatePath, item.Fingerprint)
		}
		result = append(result, item)
	}
	return result, nil
}

// Marker validation is a projection of the pinned script's existing policy.
// Automatic eligibility proves only a matching PEM pair, not cron health or
// successful renewal. Neither private key paths nor proof hashes leave Agent.
func notificationCertificateMaintenance(webRoot, domain, certificatePath, fingerprint string) string {
	if domain == "" || !domainPattern.MatchString(domain) || strings.ContainsAny(domain, "*/\\") {
		return "unknown"
	}
	rootPath := filepath.Join(webRoot, "certs")
	if filepath.Clean(certificatePath) != filepath.Join(rootPath, domain+"_cert.pem") {
		return "unknown"
	}
	info, err := os.Lstat(rootPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 || !recipeScriptOwnerTrusted(info) {
		return "unknown"
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "unknown"
	}
	defer root.Close()
	cert, err := readNotificationPolicyFile(root, domain+"_cert.pem", 1<<20, false)
	if err != nil || notificationHash(cert) != fingerprint {
		return "unknown"
	}
	marker, markerErr := readNotificationPolicyFile(root, domain+".custom", 10, true)
	if markerErr == nil {
		if string(marker) == "custom-v1\n" {
			return "custom"
		}
		return "unknown"
	}
	if !errors.Is(markerErr, os.ErrNotExist) {
		return "unknown"
	}
	proof, err := readNotificationPolicyFile(root, domain+".auto-renewal", 130, true)
	if err != nil || len(proof) != 130 {
		return "unknown"
	}
	fields := strings.Split(string(proof), " ")
	if len(fields) != 2 || len(fields[0]) != 64 || len(fields[1]) != 65 || fields[1][64] != '\n' || fields[0] != fingerprint {
		return "unknown"
	}
	key, err := readNotificationPolicyFile(root, domain+"_key.pem", 8192, true)
	if err != nil {
		return "unknown"
	}
	hash := sha256.Sum256(key)
	if fields[1][:64] != hex.EncodeToString(hash[:]) {
		return "unknown"
	}
	// Recheck public material after the pair read to reject replacement races.
	again, err := readNotificationPolicyFile(root, domain+"_cert.pem", 1<<20, false)
	if err != nil || notificationHash(again) != fingerprint {
		return "unknown"
	}
	return "automatic"
}

func readNotificationPolicyFile(root *os.Root, name string, limit int64, private bool) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit || !recipeScriptOwnerTrusted(info) || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm() != 0600) {
		return nil, errors.New("notification policy unavailable")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errors.New("notification policy changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("notification policy unavailable")
	}
	return data, nil
}

func notificationHash(data []byte) string { return strings.TrimPrefix(hashBytes(data), "sha256:") }
