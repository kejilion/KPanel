package desktopbridge

import (
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const terminalServerKey = `SYSTEM\CurrentControlSet\Control\Terminal Server`

func localEndpoint() (endpoint, error) {
	server, err := registry.OpenKey(registry.LOCAL_MACHINE, terminalServerKey, registry.QUERY_VALUE)
	if err != nil {
		return endpoint{}, ErrConfiguration
	}
	defer server.Close()
	deny, valueType, err := server.GetIntegerValue("fDenyTSConnections")
	if err != nil || valueType != registry.DWORD {
		return endpoint{}, ErrConfiguration
	}
	if deny != 0 {
		return endpoint{}, ErrDisabled
	}
	// Query-only handles work in the telemetry identity as well as SYSTEM. Never
	// use mgr.Connect/OpenService defaults, which request service write access.
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return endpoint{}, ErrConfiguration
	}
	defer windows.CloseServiceHandle(scm)
	name, _ := windows.UTF16PtrFromString("TermService")
	service, err := windows.OpenService(scm, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return endpoint{}, ErrServiceStopped
	}
	defer windows.CloseServiceHandle(service)
	var state windows.SERVICE_STATUS_PROCESS
	var needed uint32
	if err := windows.QueryServiceStatusEx(service, windows.SC_STATUS_PROCESS_INFO, (*byte)(unsafe.Pointer(&state)), uint32(unsafe.Sizeof(state)), &needed); err != nil {
		return endpoint{}, ErrConfiguration
	}
	if state.CurrentState != windows.SERVICE_RUNNING || state.ProcessId == 0 {
		return endpoint{}, ErrServiceStopped
	}
	listener, err := registry.OpenKey(registry.LOCAL_MACHINE, terminalServerKey+`\WinStations\RDP-Tcp`, registry.QUERY_VALUE)
	if err != nil {
		return endpoint{}, ErrConfiguration
	}
	defer listener.Close()
	port, valueType, err := listener.GetIntegerValue("PortNumber")
	if err != nil || valueType != registry.DWORD || port == 0 || port > 65535 {
		return endpoint{}, ErrConfiguration
	}
	hash, _, err := listener.GetBinaryValue("SSLCertificateSHA1Hash")
	storeName := "MY"
	if errors.Is(err, registry.ErrNotExist) {
		stations, e := registry.OpenKey(registry.LOCAL_MACHINE, terminalServerKey+`\WinStations`, registry.QUERY_VALUE)
		if e != nil {
			return endpoint{}, ErrConfiguration
		}
		storeName, _, e = stations.GetStringValue("SelfSignedCertStore")
		stations.Close()
		if errors.Is(e, registry.ErrNotExist) {
			storeName = "Remote Desktop"
		} else if e != nil {
			return endpoint{}, ErrConfiguration
		}
		hash = nil
	} else if err != nil || len(hash) != sha1.Size {
		return endpoint{}, ErrCertificate
	}
	if len(storeName) == 0 || len(storeName) > 128 || strings.ContainsAny(storeName, "\\/\x00") {
		return endpoint{}, ErrCertificate
	}
	pins, err := readCertificates(storeName, hash)
	if err != nil {
		return endpoint{}, err
	}
	usable := pins[:0]
	for _, pin := range pins {
		cert, err := x509.ParseCertificate(pin)
		if err == nil && verifyCertificate(cert, [][]byte{pin}, time.Now()) == nil {
			usable = append(usable, pin)
		}
	}
	if len(usable) == 0 {
		return endpoint{}, ErrCertificate
	}
	return endpoint{uint16(port), usable}, nil
}

func readCertificates(name string, hash []byte) ([][]byte, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, ErrCertificate
	}
	store, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_W, 0, 0,
		windows.CERT_SYSTEM_STORE_LOCAL_MACHINE|windows.CERT_STORE_READONLY_FLAG|windows.CERT_STORE_OPEN_EXISTING_FLAG, uintptr(unsafe.Pointer(wide)))
	runtime.KeepAlive(wide)
	if err != nil {
		return nil, ErrCertificate
	}
	defer windows.CertCloseStore(store, 0)
	if len(hash) > 0 {
		blob := windows.CryptDataBlob{Size: uint32(len(hash)), Data: &hash[0]}
		cert, err := windows.CertFindCertificateInStore(store, windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, 0, windows.CERT_FIND_HASH, unsafe.Pointer(&blob), nil)
		runtime.KeepAlive(hash)
		if err != nil {
			return nil, ErrCertificate
		}
		defer windows.CertFreeCertificateContext(cert)
		der, err := copyCertificate(cert)
		if err != nil {
			return nil, err
		}
		return [][]byte{der}, nil
	}
	var cert *windows.CertContext
	var pins [][]byte
	total := 0
	for {
		// CertEnumCertificatesInStore frees its previous context even on error.
		cert, err = windows.CertEnumCertificatesInStore(store, cert)
		if err != nil {
			if errors.Is(err, syscall.Errno(windows.CRYPT_E_NOT_FOUND)) {
				break
			}
			return nil, ErrCertificate
		}
		der, copyErr := copyCertificate(cert)
		if copyErr != nil || len(pins) >= 64 || total+len(der) > 256<<10 {
			windows.CertFreeCertificateContext(cert)
			return nil, ErrCertificate
		}
		pins = append(pins, der)
		total += len(der)
	}
	if len(pins) == 0 {
		return nil, ErrCertificate
	}
	return pins, nil
}

func copyCertificate(cert *windows.CertContext) ([]byte, error) {
	if cert == nil || cert.EncodedCert == nil || cert.Length == 0 || cert.Length > 16<<10 {
		return nil, ErrCertificate
	}
	return append([]byte(nil), unsafe.Slice(cert.EncodedCert, cert.Length)...), nil
}
