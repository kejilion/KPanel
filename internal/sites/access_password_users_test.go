package sites

import (
	"reflect"
	"strings"
	"testing"
)

func TestAccessPasswordUsernamesNeverReturnHashes(t *testing.T) {
	got, err := accessPasswordUsernames([]byte("# comment\r\nalice:$apr1$private\r\nbob:{SHA}secret\n"))
	if err != nil || !reflect.DeepEqual(got, []string{"alice", "bob"}) {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestAccessPasswordUsernamesRejectAmbiguousFiles(t *testing.T) {
	for _, content := range []string{"missing-colon", "alice:\n", "alice:hash\nalice:other", strings.Repeat("a", maxAccessPasswordFileBytes+1)} {
		if _, err := accessPasswordUsernames([]byte(content)); err == nil {
			t.Fatal("expected invalid or ambiguous file to fail")
		}
	}
}
