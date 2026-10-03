//go:build windows

package windowsnode

import (
	"crypto/x509"
	"errors"
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

// Publisher is the exact Windows X.500 subject, not a rotating leaf thumbprint.
// ProfileOID additionally pins an Artifact Signing certificate profile EKU.
type TrustPolicy struct {
	Publisher  string `json:"publisher"`
	ProfileOID string `json:"profileOID,omitempty"`
}

func (p TrustPolicy) Validate() error {
	if p.Publisher == "" || strings.TrimSpace(p.Publisher) != p.Publisher || len(p.Publisher) > 1024 || strings.ContainsAny(p.Publisher, "\r\n\x00") {
		return errors.New("trusted Windows publisher is not configured")
	}
	if len(p.ProfileOID) > 128 || strings.ContainsAny(p.ProfileOID, "\r\n\x00") {
		return errors.New("invalid signing profile OID")
	}
	return nil
}

// Only the signer selected by WinVerifyTrust is examined; independently reading
// an arbitrary certificate from a PKCS#7 bag would permit signer confusion.
func VerifySignature(path string, policy TrustPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	file := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: name}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, RevocationChecks: windows.WTD_REVOKE_WHOLECHAIN, UnionChoice: windows.WTD_CHOICE_FILE, StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_REVOCATION_CHECK_CHAIN_EXCLUDE_ROOT, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&file)}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	}()
	if err != nil {
		return errors.New("Authenticode chain, timestamp or revocation verification failed")
	}
	dll := windows.NewLazySystemDLL("wintrust.dll")
	provider, _, _ := dll.NewProc("WTHelperProvDataFromStateData").Call(uintptr(data.StateData))
	if provider == 0 {
		return errors.New("verified signer unavailable")
	}
	signer, _, _ := dll.NewProc("WTHelperGetProvSignerFromChain").Call(provider, 0, 0, 0)
	if signer == 0 {
		return errors.New("verified signer unavailable")
	}
	type providerCert struct {
		Size uint32
		Cert *windows.CertContext
	}
	type providerSigner struct {
		Size       uint32
		VerifiedAt windows.Filetime
		Count      uint32
		Chain      *providerCert
	}
	// Copy the native structure while WinTrust owns its lifetime. Keeping a
	// syscall-returned address as uintptr and reading it explicitly avoids an
	// unchecked Go uintptr-to-pointer conversion across a GC safe point.
	var selected providerSigner
	var copied uintptr
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), signer, (*byte)(unsafe.Pointer(&selected)), unsafe.Sizeof(selected), &copied); err != nil || copied != unsafe.Sizeof(selected) {
		return errors.New("verified signer structure unavailable")
	}
	if selected.Count == 0 || selected.Count > 32 || selected.Chain == nil || selected.Chain.Cert == nil {
		return errors.New("verified signer chain unavailable")
	}
	cert := selected.Chain.Cert
	if cert.Length == 0 || cert.Length > 65536 {
		return errors.New("invalid signer certificate size")
	}
	parsed, err := x509.ParseCertificate(unsafe.Slice(cert.EncodedCert, int(cert.Length)))
	if err != nil {
		return err
	}
	certName := windows.NewLazySystemDLL("crypt32.dll").NewProc("CertNameToStrW")
	buffer := make([]uint16, 2048)
	length, _, _ := certName.Call(windows.X509_ASN_ENCODING, uintptr(unsafe.Pointer(&cert.CertInfo.Subject)), 3|0x02000000, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 || length >= uintptr(len(buffer)) || windows.UTF16ToString(buffer) != policy.Publisher {
		return errors.New("Authenticode publisher does not match trusted publisher")
	}
	if policy.ProfileOID != "" {
		found := false
		for _, oid := range parsed.UnknownExtKeyUsage {
			if oid.String() == policy.ProfileOID {
				found = true
			}
		}
		if !found {
			return errors.New("Authenticode certificate profile does not match trusted profile")
		}
	}
	return nil
}
