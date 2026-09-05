package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestUpdateRuntimeMatchesPinnedSourceDigests(t *testing.T) {
	content, err := os.ReadFile("update_runtime/source.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Repository   string
		Revision     string
		ScriptSHA256 string
		Templates    map[string]string
	}
	if err := json.Unmarshal(content, &source); err != nil {
		t.Fatal(err)
	}
	if source.Repository != "https://github.com/kejilion/sh" || len(source.Revision) != 40 || len(source.ScriptSHA256) != 64 {
		t.Fatal("runtime source is not pinned")
	}
	for name, template := range map[string][]byte{"update.sh": lightNodeUpdater, "update.service": lightNodeUpdateService, "update.timer": lightNodeUpdateTimer} {
		sum := sha256.Sum256(template)
		if hex.EncodeToString(sum[:]) != source.Templates[name] {
			t.Fatalf("%s differs from pinned script; regenerate from kejilion.sh", name)
		}
	}
}

func TestProcessStartTimeHandlesSpacesAndParentheses(t *testing.T) {
	fields := append([]string{"S"}, strings.Fields(strings.Repeat("0 ", 18))...)
	fields = append(fields, "12345", "0")
	start, ok := processStartTime([]byte("123 (update (legacy)) " + strings.Join(fields, " ")))
	if !ok || start != "12345" {
		t.Fatalf("process identity = %q, %v", start, ok)
	}
	for _, invalid := range []string{"", "123 (bash) S 0", "123 no comm"} {
		if _, ok := processStartTime([]byte(invalid)); ok {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

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
