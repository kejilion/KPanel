package panel

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

func TestMCPRejectsRawTerminalWindowWithoutContext(t *testing.T) {
	// A permitted caller can request an offset immediately after password=.
	// Sanitizing this suffix alone cannot recover its credential classification.
	body, _ := json.Marshal(map[string]any{"dataBase64": base64.StdEncoding.EncodeToString([]byte("synthetic-secret")), "nextOffset": 100})
	if output, err := mcpManagedOutput(body, 0, 20); err == nil {
		t.Fatalf("context-free raw terminal was exposed: %#v", output)
	}
}

func TestMCPFileProjectionCannotQualifyTerminalData(t *testing.T) {
	for _, body := range []string{
		`{"projection":"full-context-v1","content":"safe","dataBase64":"U0VDUkVU"}`,
		`{"projection":"full-context-v1","content":"safe","text":"SECRET"}`,
		`{"projection":"full-context-v1","content":"safe","nextOffset":4}`,
		`{"projection":"full-context-v1","content":123}`,
		`{"projection":"full-context-v1","content":"safe","Content":"OPAQUE_SYNTHETIC"}`,
		`{"projection":"full-context-v1","content":"safe","content":"OPAQUE_SYNTHETIC"}`,
	} {
		if output, err := mcpManagedOutputFor("host_file_tail", []byte(body), 0, 20); err == nil {
			t.Fatalf("invalid file projection accepted: %#v", output)
		}
	}
	valid := qualifiedTestTail("safe")
	duplicate := append(append([]byte{}, valid[:len(valid)-1]...), []byte(`,"Content":"OPAQUE_SYNTHETIC"}`)...)
	if _, err := mcpManagedOutputFor("host_file_tail", duplicate, 0, 20); err == nil {
		t.Fatal("case-variant field accepted")
	}
	if _, err := mcpManagedOutput(valid, 0, 20); err == nil {
		t.Fatal("file projection accepted for unrelated tool")
	}
	if _, err := mcpManagedOutputFor("host_file_tail", valid, 0, 20); err != nil {
		t.Fatal(err)
	}
}

func TestMCPJobOutputVersionedDisplayContract(t *testing.T) {
	for source, owner := range managedJobSources {
		if !owner.Output {
			continue
		}
		op := managedCatalog()["host_"+source+"_job"]
		old := op
		old.Schema = nil
		currentSchema, _ := json.Marshal(op.Schema)
		_ = json.Unmarshal(currentSchema, &old.Schema)
		delete(old.Schema, "description")
		if old.signature() == op.signature() {
			t.Fatal("old output contract still shares delegated signature")
		}
		policy := &mcpaccess.Policy{Domains: []string{op.Domain}, Operations: []string{op.Name}, OperationVersions: map[string]string{op.Name: old.signature()}}
		if op.allowed(policy) {
			t.Fatal("old output grant remained valid")
		}
		policy.OperationVersions[op.Name] = op.signature()
		raw := json.RawMessage(`{"jobId":"` + strings.Repeat("a", 32) + `","output":true,"offset":9}`)
		request, err := op.prepare(raw, policy)
		if err != nil {
			t.Fatal(err)
		}
		query, _ := url.ParseQuery(request.Query)
		if query.Get("projection") != "model-v1" || query.Get("offset") != "9" {
			t.Fatalf("%s: %#v", source, request)
		}
	}
	for _, name := range []string{"host_app_jobs", "host_web_jobs"} {
		op := managedCatalog()[name]
		policy := &mcpaccess.Policy{Domains: []string{op.Domain}, Operations: []string{name}, OperationVersions: map[string]string{name: op.signature()}}
		request, err := op.prepare(json.RawMessage(`{}`), policy)
		if err != nil || request.Query != "projection=model-v1" {
			t.Fatalf("list %s: %#v %v", name, request, err)
		}
		if err := mcpTaskDisplayBody(name, []byte(`{"items":[],"redacted":true}`)); err == nil {
			t.Fatal("old owner list accepted")
		}
		if err := mcpTaskDisplayBody(name, []byte(`{"data":{"items":[],"redacted":true}}`)); err == nil {
			t.Fatal("old peer list accepted")
		}
		if err := mcpTaskDisplayBody(name, []byte(`{"data":{"items":[],"projection":"model-v1"}}`)); err != nil {
			t.Fatal(err)
		}
	}
}
