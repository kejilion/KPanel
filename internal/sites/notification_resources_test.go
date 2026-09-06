package sites

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNotificationCertificatePolicyProofAndUnknown(t *testing.T) {
	root := t.TempDir()
	certs := filepath.Join(root, "certs")
	if err := os.Mkdir(certs, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(certs, name), data, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(certs, name), mode); err != nil {
			t.Fatal(err)
		}
	}
	cert := []byte("public certificate fixture")
	key := []byte("private test fixture")
	write("example.com_cert.pem", cert, 0644)
	write("example.com_key.pem", key, 0600)
	check := func(want string) {
		t.Helper()
		got := notificationCertificateMaintenance(root, "example.com", filepath.Join(certs, "example.com_cert.pem"), notificationHash(cert))
		if got != want {
			t.Fatalf("maintenance=%s want=%s", got, want)
		}
	}
	check("unknown")
	info, _ := os.Stat(certs)
	trusted := recipeScriptOwnerTrusted(info)
	write("example.com.custom", []byte("custom-v1\n"), 0600)
	if trusted {
		check("custom")
	} else {
		check("unknown")
	}
	write("example.com.custom", []byte("broken"), 0600)
	check("unknown")
	if err := os.Remove(filepath.Join(certs, "example.com.custom")); err != nil {
		t.Fatal(err)
	}
	proof := []byte(notificationHash(cert) + " " + notificationHash(key) + "\n")
	write("example.com.auto-renewal", proof, 0600)
	if trusted {
		check("automatic")
	} else {
		check("unknown")
	}
	write("example.com_key.pem", []byte("replaced private key"), 0600)
	check("unknown")
	write("example.com_key.pem", key, 0600)
	write("example.com.auto-renewal", proof, 0644)
	check("unknown")
	write("example.com.auto-renewal", proof, 0600)
	if err := os.Remove(filepath.Join(certs, "example.com.custom")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.Symlink("example.com.auto-renewal", filepath.Join(certs, "example.com.custom")); err == nil {
		check("unknown")
	}
	if got := notificationCertificateMaintenance(root, "../example.com", filepath.Join(certs, "example.com_cert.pem"), notificationHash(cert)); got != "unknown" {
		t.Fatal("path traversal accepted")
	}
}

func TestNotificationCertificateDiscoveryIdentityAndReadFailure(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"conf.d", "certs", "html"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	writeTestCertificate(t, filepath.Join(root, "certs"), "site.example.com", now)
	// Use the same newline-oriented parser input as actual Nginx configs.
	config := []byte("server {\nlisten 443 ssl;\nserver_name site.example.com;\nssl_certificate /etc/nginx/certs/site_cert.pem;\nssl_certificate_key /etc/nginx/certs/site_key.pem;\n}\n")
	path := filepath.Join(root, "conf.d", "site.conf")
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	items, err := d.NotificationResources(context.Background())
	if err != nil || len(items) != 1 || !items[0].Known || len(items[0].ID) != 64 || len(items[0].Fingerprint) != 64 {
		t.Fatalf("discovery %+v %v", items, err)
	}
	id := items[0].ID
	if err := os.WriteFile(filepath.Join(root, "conf.d", "alias.conf"), config, 0600); err != nil {
		t.Fatal(err)
	}
	items, err = d.NotificationResources(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != id {
		t.Fatal("shared certificate duplicated")
	}
	if err := os.Remove(filepath.Join(root, "certs", "site_cert.pem")); err != nil {
		t.Fatal(err)
	}
	items, err = d.NotificationResources(context.Background())
	if err != nil || len(items) != 1 || items[0].Known || items[0].ID != id {
		t.Fatalf("read failure lost stable identity %+v %v", items, err)
	}
}

func TestNotificationCertificateDiscoveryLimitAndFailure(t *testing.T) {
	root := t.TempDir()
	d := NewDiscoverer(root)
	items, err := d.NotificationResources(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("uninitialized %v %v", items, err)
	}
	conf := filepath.Join(root, "conf.d")
	if err := os.Mkdir(conf, 0700); err != nil {
		t.Fatal(err)
	}
	for i := range MaxNotificationCertificates + 1 {
		if err := os.WriteFile(filepath.Join(conf, fmt.Sprintf("%d.conf", i)), []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.NotificationResources(context.Background()); !errors.Is(err, ErrNotificationResourceLimit) {
		t.Fatal(err)
	}
}
