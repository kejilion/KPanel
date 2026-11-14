package browsercomponent

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedDeploymentAssetsMatchCanonicalPackaging(t *testing.T) {
	for path, embedded := range map[string][]byte{"managed-compose.yml": composeTemplate, "browser-seccomp.json": seccompProfile, "tor-compose.yml": torComposeTemplate, "tor.Dockerfile": torDockerfile, "tor-start.sh": torStart, "tor-browser.tar.xz.asc": torSignature, "tor-signing-key.asc": torSigningKey, "tor-helper-locks.tar": torHelperLocks, "tor-helpers-build.sh": torHelpersBuild} {
		data, err := os.ReadFile("../../packaging/browser/" + path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(embedded, []byte("\r\n"), []byte("\n"))) {
			t.Fatal("embedded component asset differs from canonical packaging", path)
		}
	}
	if !bytes.Contains(composeTemplate, []byte(componentImage)) {
		t.Fatal("component image differs from deployment pin")
	}
	if !bytes.Contains(torComposeTemplate, []byte(componentTorImage)) {
		t.Fatal("Tor image differs from deployment pin")
	}
}

func TestTorHelperLockArchiveMatchesReviewedCanonicalInputs(t *testing.T) {
	names := []string{"conjure/go.mod", "conjure/go.sum", "lyrebird/go.mod", "lyrebird/go.sum", "manifest.json"}
	expected := make(map[string][]byte)
	var sums strings.Builder
	for _, name := range names {
		data, err := os.ReadFile("../../packaging/browser/tor-helpers/" + name)
		if err != nil {
			t.Fatal(err)
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		expected[name] = data
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(data), name)
	}
	expected["SHA256SUMS"] = []byte(sums.String())
	reader := tar.NewReader(bytes.NewReader(torHelperLocks))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		want, ok := expected[header.Name]
		if !ok || header.Typeflag != tar.TypeReg || header.Size != int64(len(want)) {
			t.Fatalf("unexpected helper archive entry: %s", header.Name)
		}
		data, err := io.ReadAll(reader)
		if err != nil || !bytes.Equal(data, want) {
			t.Fatalf("helper archive differs from canonical lock: %s", header.Name)
		}
		delete(expected, header.Name)
	}
	if len(expected) != 0 {
		t.Fatal("helper lock archive is incomplete")
	}
}
