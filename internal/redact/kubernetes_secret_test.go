package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKubernetesSecretDataIsNotProjected(t *testing.T) {
	const sample = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"example"},"data":{"tls.key":"U1lOVEhFVElDX1BSSVZBVEU=","arbitrary":"T1RI RVJfU0VDUkVU"}}`
	for name, project := range map[string]func() (string, error){
		"json": func() (string, error) { v, err := JSON([]byte(sample)); return string(v), err },
		"text": func() (string, error) { return Text(sample), nil },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := project()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "U1lOVEhFVElDX1BSSVZBVEU=") || strings.Contains(got, "T1RI RVJfU0VDUkVU") {
				t.Fatal("synthetic Secret data remained in the projection")
			}
			if !strings.Contains(got, "example") {
				t.Fatal("ordinary metadata was lost")
			}
		})
	}
}

func TestKubernetesSecretsInsideListsAndUnrelatedData(t *testing.T) {
	const sample = `{"kind":"List","items":[{"kind":"Secret","data":{"unusual":"OPAQUE_SYNTHETIC"}},{"kind":"ConfigMap","data":{"caption":"ordinary-visible"}}]}`
	got, err := JSON([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("nested synthetic secret remains")
	}
	if !strings.Contains(string(got), "ordinary-visible") {
		t.Fatal("unrelated ConfigMap data was lost")
	}
}

func TestTLSKeyFieldIsProtectedWithoutKind(t *testing.T) {
	got, err := JSON([]byte(`{"tls.key":"OPAQUE_SYNTHETIC","name":"visible"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("TLS private key remains")
	}
	if !strings.Contains(string(got), "visible") {
		t.Fatal("ordinary field was lost")
	}
}

func TestKubernetesYAMLSecretIsNotProjected(t *testing.T) {
	value := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: example\ndata:\n  arbitrary: OPAQUE_SYNTHETIC\n  tls.key: OTHER_SYNTHETIC\n"
	checks := map[string]string{
		"text":    Text(value),
		"records": strings.Join(Records(strings.Split(value, "\n")), "\n"),
		"lines":   strings.Join(Lines([]byte(value), -1, 4096), "\n"),
	}
	for name, got := range checks {
		if strings.Contains(got, "OPAQUE_SYNTHETIC") || strings.Contains(got, "OTHER_SYNTHETIC") {
			t.Errorf("%s retained YAML secret", name)
		}
	}
}

func TestKubernetesSecretListWithoutItemKind(t *testing.T) {
	value := `{"apiVersion":"v1","kind":"SecretList","items":[{"metadata":{"name":"example"},"data":{"opaque":"OPAQUE_SYNTHETIC"}}]}`
	got, err := JSON([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("SecretList item data remains")
	}
}

func TestKubernetesYAMLInJSONEnvelope(t *testing.T) {
	value := `{"content":"apiVersion: v1\nkind: Secret\ndata:\n  example: OPAQUE_SYNTHETIC\n"}`
	got, err := JSON([]byte(value))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("embedded YAML secret remains")
	}
}

func TestKubernetesYAMLSplitAcrossJSONRecords(t *testing.T) {
	raw := `{"logs":["kind: Secret","data:","  opaque: OPAQUE_SYNTHETIC"],"status":"ordinary-visible"}`
	got, err := JSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("split JSON array exposed secret")
	}
	if !strings.Contains(string(got), "ordinary-visible") {
		t.Fatal("unrelated JSON field was lost")
	}
	lines := []string{`{"line":"kind: Secret"}`, `{"line":"data:"}`, `{"line":"  opaque: OPAQUE_SYNTHETIC"}`}
	if strings.Contains(strings.Join(Records(lines), "\n"), "OPAQUE_SYNTHETIC") {
		t.Fatal("split JSON records exposed secret")
	}
}

func TestKubernetesYAMLTaggedAndBlockKinds(t *testing.T) {
	for _, kind := range []string{"kind: !!str Secret", "kind: &k Secret", "kind: !!str &k Secret", "kind: |\n  Secret", "kind: >-\n  Secret", "kind:\n  Secret", `'kind': Secret`, `kind: "Se\u0063ret"`, "kind: \"Secr\\\n  et\"", `"\u006bind": Secret`, "kind: *kindAlias"} {
		value := "apiVersion: v1\n" + kind + "\ndata:\n  opaque: OPAQUE_SYNTHETIC\n"
		if strings.Contains(Text(value), "OPAQUE_SYNTHETIC") {
			t.Errorf("kind form exposed secret: %s", kind)
		}
	}
}

func TestKubernetesYAMLPreservesUnrelatedDocumentsAndMetadata(t *testing.T) {
	value := "status: startup-visible\n---\ndata:\n  opaque: OPAQUE_SYNTHETIC\nkind: Secret\nmetadata:\n  name: example-visible\n---\nkind: |-\n  ConfigMap\ndata:\n  title: config-visible\n---\nstatus: complete-visible\n"
	for name, got := range map[string]string{
		"text":    Text(value),
		"records": strings.Join(Records(strings.Split(value, "\n")), "\n"),
		"lines":   strings.Join(Lines([]byte(value), -1, 4096), "\n"),
	} {
		if strings.Contains(got, "OPAQUE_SYNTHETIC") {
			t.Errorf("%s retained a secret", name)
		}
		for _, visible := range []string{"startup-visible", "example-visible", "config-visible", "complete-visible"} {
			if !strings.Contains(got, visible) {
				t.Errorf("%s hid %s", name, visible)
			}
		}
	}
}

func TestKubernetesYAMLJSONRecordsPreserveOrdinaryDocuments(t *testing.T) {
	raw := `{"logs":["status: before-visible","---","kind: Secret","data:","  opaque: OPAQUE_SYNTHETIC","---","status: after-visible"],"status":"separate-visible"}`
	got, err := JSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
		t.Fatal("split JSON records retained a secret")
	}
	for _, visible := range []string{"before-visible", "after-visible", "separate-visible"} {
		if !strings.Contains(string(got), visible) {
			t.Errorf("split records hid %s", visible)
		}
	}
	jsonLines := []string{`{"line":"status: before-visible"}`, `{"line":"---"}`, `{"line":"kind: Secret"}`, `{"line":"data:"}`, `{"line":"  opaque: OPAQUE_SYNTHETIC"}`, `{"line":"---"}`, `{"line":"status: after-visible"}`}
	projected := strings.Join(Records(jsonLines), "\n")
	if strings.Contains(projected, "OPAQUE_SYNTHETIC") || !strings.Contains(projected, "before-visible") || !strings.Contains(projected, "after-visible") {
		t.Fatal("JSONL document boundaries did not preserve safe records")
	}
}

func TestKubernetesYAMLListPreservesConfigMap(t *testing.T) {
	value := "kind: List\nitems:\n  - kind: Secret\n    data:\n      opaque: OPAQUE_SYNTHETIC\n  - kind: ConfigMap\n    data:\n      title: config-visible\n"
	got := Text(value)
	if strings.Contains(got, "OPAQUE_SYNTHETIC") || !strings.Contains(got, "config-visible") {
		t.Fatal("mixed YAML list was not projected safely")
	}
}

func TestOrdinaryKindRecordsRemainReadable(t *testing.T) {
	value := "kind: ConfigMap\ndata:\n  title: config-visible\nordinary log with [details]\n"
	got := Text(value)
	if !strings.Contains(got, "config-visible") || !strings.Contains(got, "ordinary log with [details]") {
		t.Fatal("ordinary kind records became unavailable")
	}
}

func TestKubernetesYAMLAmbiguityRemainsFailClosed(t *testing.T) {
	for _, value := range []string{
		"value: &shared\n  opaque: OPAQUE_SYNTHETIC\nkind: Secret\ndata: *shared\n",
		"kind: Secret\ndata: {opaque: OPAQUE_SYNTHETIC}\n",
		"kind: Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\ninvalid indentation\n",
		"content: |\n  kind: Secret\n  data:\n    opaque: OPAQUE_SYNTHETIC\n",
	} {
		if strings.Contains(Text(value), "OPAQUE_SYNTHETIC") {
			t.Fatal("ambiguous YAML exposed a synthetic secret")
		}
	}
}

func TestKubernetesYAMLDeepSequencesFailClosed(t *testing.T) {
	value := "kind: Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\n---\n" + strings.Repeat("- ", 300) + "end\n"
	if strings.Contains(Text(value), "OPAQUE_SYNTHETIC") {
		t.Fatal("deep YAML exposed a synthetic secret")
	}
}

func TestKubernetesYAMLEscapedIdentityAndNestedChannels(t *testing.T) {
	inner := `{"doc":"kind: |-\n  ConfigMap\ndata:\n  opaque: OPAQUE_SYNTHETIC"}`
	quoted, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		`"\u006bind": "\u0053ecret"` + "\n" + `"\u0064ata":` + "\n  opaque: OPAQUE_SYNTHETIC\n",
		"? kind\n: Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\n",
		"? |-\n  kind\n: Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\n",
		"? \"ki\\\n  nd\"\n: Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\n",
		"\"ki\\\n  nd\": Secret\ndata:\n  opaque: OPAQUE_SYNTHETIC\n",
		"kind: Secret\nstringData:\n  payload: " + string(quoted) + "\n",
		"{kind: Secret, data: {opaque: OPAQUE_SYNTHETIC}}",
	} {
		if strings.Contains(Text(value), "OPAQUE_SYNTHETIC") {
			t.Error("text projection exposed a synthetic secret")
		}
		wrapped, err := json.Marshal(map[string]string{"content": value})
		if err != nil {
			t.Fatal(err)
		}
		got, err := JSON(wrapped)
		if err == nil && strings.Contains(string(got), "OPAQUE_SYNTHETIC") {
			t.Errorf("JSON envelope exposed a synthetic secret: %v", err)
		}
	}
}

func TestYAMLEscapedSensitiveKeyDoesNotExposeValue(t *testing.T) {
	value := `- "pass\u0077ord": OPAQUE_SYNTHETIC` + "\n  safe: ordinary-visible\n"
	for name, got := range map[string]string{"text": Text(value), "records": strings.Join(Records(strings.Split(value, "\n")), "\n")} {
		if strings.Contains(got, "OPAQUE_SYNTHETIC") || !strings.Contains(got, "ordinary-visible") {
			t.Errorf("%s failed to redact escaped YAML key", name)
		}
	}
}

func TestYAMLEscapedSensitiveKeysContinueAcrossBlocks(t *testing.T) {
	for _, value := range []string{
		`- "pass\u0077ord": |` + "\n    OPAQUE_SYNTHETIC\n  safe: ordinary-visible\n",
		`- "\u0061uth": OPAQUE_SYNTHETIC` + "\n  safe: ordinary-visible\n",
		`- "\u006bey": >-` + "\n    OPAQUE_SYNTHETIC\n  safe: ordinary-visible\n",
		`- !!str "pass\u0077ord": OPAQUE_SYNTHETIC` + "\n  safe: ordinary-visible\n",
		`- &anchor "pass\u0077ord": OPAQUE_SYNTHETIC` + "\n  safe: ordinary-visible\n",
		`- !!str "pass\u0077ord": |` + "\n    OPAQUE_SYNTHETIC\n  safe: ordinary-visible\n",
	} {
		got := Text(value)
		if strings.Contains(got, "OPAQUE_SYNTHETIC") || !strings.Contains(got, "ordinary-visible") {
			t.Error("escaped YAML key bypassed redaction or hid unrelated data")
		}
	}
}

func TestYAMLEscapedOrdinaryKeyRemainsReadable(t *testing.T) {
	value := `- "some\ key": ordinary-visible` + "\n"
	if !strings.Contains(Text(value), "ordinary-visible") {
		t.Fatal("ordinary YAML escaped key was hidden")
	}
}

func TestInlineYAMLMappingEscapedSensitiveKey(t *testing.T) {
	value := `INFO {"pass\u0077ord": OPAQUE_SYNTHETIC, safe: ordinary-visible}`
	got := Text(value)
	if strings.Contains(got, "OPAQUE_SYNTHETIC") {
		t.Fatalf("inline escaped YAML key was not safely projected: %q", got)
	}
}
