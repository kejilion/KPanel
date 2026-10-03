package desktopbridge

import (
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestMissingCertificateStoreIsNotCreated(t *testing.T) {
	const name = "KPanel-DesktopBridge-ReadOnly-Test-9F81E0B6"
	path := `SOFTWARE\Microsoft\SystemCertificates\` + name
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ); err == nil {
		key.Close()
		t.Skip("test store name already exists; do not inspect or change it")
	}
	if _, err := readCertificates(name, nil); err != ErrCertificate {
		t.Fatalf("missing store returned %v", err)
	}
	if key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ); err == nil {
		key.Close()
		t.Fatal("query created a machine certificate store")
	}
}

func TestWindowsLocalReadOnlyProbe(t *testing.T) {
	if os.Getenv("KPANEL_TEST_LOCAL_RDP_PROBE") != "1" {
		t.Skip("explicit read-only local probe")
	}
	local, err := localEndpoint()
	if err != nil {
		t.Logf("local RDP readiness: %s", Code(err))
		return
	}
	if local.port == 0 || len(local.certificates) == 0 {
		t.Fatal("ready endpoint lacks listener or trusted certificate")
	}
	t.Logf("local RDP ready: loopback port=%d trusted_certificates=%d", local.port, len(local.certificates))
}
