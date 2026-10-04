//go:build windows

package windowsnode

import "errors"

// The expected digest comes from the same fixed official HTTPS release as the
// artifact. hashFile also enforces the protected file's owner, ACL and size.
// This checks integrity, not a certificate publisher or independent provenance.
func VerifyChecksum(path, expected string) error {
	if !validSHA(expected) {
		return errors.New("invalid expected release digest")
	}
	actual, err := hashFile(path)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("release checksum mismatch")
	}
	return nil
}
