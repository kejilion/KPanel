package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestStagedChecksumRequiresExactReleaseAsset(t *testing.T) {
	name := "kejilion-node-linux-amd64"
	sum := sha256.Sum256([]byte("verified binary"))
	content := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	parsed, ok := stagedChecksum(content, name)
	if !ok || string(parsed) != string(sum[:]) {
		t.Fatalf("stagedChecksum(valid) = %x, %v", parsed, ok)
	}
	for _, invalid := range [][]byte{
		[]byte("not-a-checksum  " + name + "\n"),
		[]byte(hex.EncodeToString(sum[:]) + "  another-name\n"),
		[]byte(hex.EncodeToString(sum[:]) + "  " + name + " extra\n"),
	} {
		if _, ok := stagedChecksum(invalid, name); ok {
			t.Fatalf("stagedChecksum(%q) accepted an invalid release manifest", invalid)
		}
	}
}
