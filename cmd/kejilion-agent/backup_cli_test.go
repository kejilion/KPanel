package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBackupCLIProtocolAndInputBoundary(t *testing.T) {
	var out bytes.Buffer
	if err := runBackupCLI([]string{"protocol"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"protocol":1,"format":1}`+"\n" {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{{"status", "../../etc"}, {"download", strings.Repeat("a", 32), "../../secret"}, {"exec", "id"}, {"protocol", "extra"}} {
		if err := runBackupCLI(args, strings.NewReader(""), &out); err == nil {
			t.Fatal("unrecognized command accepted", args)
		}
	}
}
