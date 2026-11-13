package browsercomponent

import (
	"bytes"
	"os"
	"testing"
)

func TestEmbeddedDeploymentAssetsMatchCanonicalPackaging(t *testing.T) {
	for path, embedded := range map[string][]byte{"managed-compose.yml": composeTemplate, "browser-seccomp.json": seccompProfile, "tor-compose.yml": torComposeTemplate, "tor.Dockerfile": torDockerfile, "tor-start.sh": torStart, "tor-browser.tar.xz.asc": torSignature, "tor-signing-key.asc": torSigningKey} {
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
