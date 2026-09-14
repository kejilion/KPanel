package selfupdate

import (
	"os"
	"strings"
	"testing"
)

func TestFixedEnvironmentIsMinimalAndPinsAutomaticTarget(t *testing.T) {
	t.Setenv("BASH_ENV", "/tmp/hostile")
	t.Setenv("KJ_KPANEL_FORCE_ROLLBACK", "1")
	t.Setenv("PATH", "/tmp/hostile")

	environment := fixedEnvironment(map[string]string{
		"KJ_KPANEL_AUTOMATIC":      "1",
		"KJ_KPANEL_TARGET_VERSION": "1.2.3",
	})
	joined := strings.Join(environment, "\n")
	for _, forbidden := range []string{"BASH_ENV=", "KJ_KPANEL_FORCE_ROLLBACK=", "PATH=/tmp/hostile"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe environment leaked: %s", forbidden)
		}
	}
	for _, required := range []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"KJ_KPANEL_AUTOMATIC=1",
		"KJ_KPANEL_TARGET_VERSION=1.2.3",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing fixed environment entry %q in %q", required, joined)
		}
	}
}

func TestTrustedRegularFileRejectsWritableFile(t *testing.T) {
	path := t.TempDir() + string(os.PathSeparator) + "kpanel.conf"
	if err := os.WriteFile(path, []byte("test"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if err := trustedRegularFile(path); err == nil {
		t.Fatal("group/world-writable update file was accepted")
	}
}
