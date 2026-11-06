package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONPrivateKeyAcrossRecords(t *testing.T) {
	for _, raw := range []string{
		`["before","-----BEGIN PRIVATE KEY-----","PRIVATE_BODY","-----END PRIVATE KEY-----","after"]`,
		`[{"line":"before"},{"line":"-----BEGIN PRIVATE KEY-----"},{"line":"PRIVATE_BODY"},{"line":"-----END PRIVATE KEY-----"},{"line":"after"}]`,
		`{"logs":["before","-----BEGIN RSA PRIVATE KEY-----","PRIVATE_BODY","-----END RSA PRIVATE KEY-----","after"]}`,
		`["PRIVATE_BODY","-----END PRIVATE KEY-----","after"]`,
		`["-----BEGIN PRIVA\u001b[0mTE KEY-----","PRIVATE_BODY","-----END PRIVA\u001b[0mTE KEY-----","after"]`,
	} {
		for _, text := range []bool{false, true} {
			var result string
			if text {
				result = Text(raw)
			} else {
				safe, err := JSON([]byte(raw))
				if err != nil {
					t.Fatal(err)
				}
				result = string(safe)
			}
			if strings.Contains(result, "PRIVATE_BODY") || !strings.Contains(result, "after") || !json.Valid([]byte(result)) {
				t.Fatalf("cross-record private key leaked or broke JSON: %s", result)
			}
		}
	}
}

func TestTextConsumesWholeSensitiveShellWord(t *testing.T) {
	for _, input := range []string{
		`run password=''actual-secret ; echo safe`,
		`run --token ''actual-secret ; echo safe`,
		`run password='actual-'"secret" ; echo safe`,
		`run --token actual\-secret ; echo safe`,
	} {
		safe := Text(input)
		if strings.Contains(safe, "actual") || strings.Contains(safe, "secret") || !strings.Contains(safe, "echo safe") {
			t.Fatalf("sensitive shell word was only partially hidden: %q", safe)
		}
	}
}

func TestJSONSensitiveNamesAndConfigurationLineValues(t *testing.T) {
	for _, raw := range []string{
		`{"MYSQL_PWD":"actual-secret","db_pwd":"actual-secret","pass\u001b[0mword":"actual-secret","safe":"visible"}`,
		`{"content":"password: correct horse battery staple\nPASSWORD=correct horse battery staple\nexport API_TOKEN=correct horse battery staple\nsafe: visible"}`,
	} {
		for _, project := range []func(string) string{Text, func(s string) string {
			value, err := JSON([]byte(s))
			if err != nil {
				t.Fatal(err)
			}
			return string(value)
		}} {
			safe := project(raw)
			for _, secret := range []string{"actual-secret", "correct", "horse", "battery", "staple"} {
				if strings.Contains(safe, secret) {
					t.Fatalf("configuration credential leaked: %s", safe)
				}
			}
			if !strings.Contains(safe, "visible") || !json.Valid([]byte(safe)) {
				t.Fatalf("configuration projection lost structure: %s", safe)
			}
		}
	}
}

func TestJSONProjectionHandlesEncodedNamesAndNestedContent(t *testing.T) {
	input := `{"passw\u006frd":"secret-one","auths":{"registry":{"auth":"base64-secret"}},"content":"{\"api_key\":\"secret-two\",\"items\":[{\"name\":\"NORMAL\",\"value\":\"secret-three\"}],\"safe\":\"original\"}","count":9007199254740993}`
	result, err := JSON([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(result) {
		t.Fatal("invalid projected JSON")
	}
	for _, secret := range []string{"secret-one", "base64-secret", "secret-two", "secret-three"} {
		if strings.Contains(string(result), secret) {
			t.Fatalf("secret leaked: %s", result)
		}
	}
	if !strings.Contains(string(result), "original") || !strings.Contains(string(result), "9007199254740993") {
		t.Fatalf("safe fields changed: %s", result)
	}
	if got := Text(input); strings.Contains(got, "secret-one") || strings.Contains(got, "secret-two") {
		t.Fatalf("text entry point bypassed decoded JSON: %s", got)
	}
}

func TestJSONProjectionRecognizesUnicodeSimpleFoldAliases(t *testing.T) {
	input := `{"ſecret":"OPAQUE_ONE","paſſword":"OPAQUE_TWO","apiKey":"OPAQUE_THREE","author":"visible"}`
	result, err := JSON([]byte(input))
	if err != nil || strings.Contains(string(result), "OPAQUE_") || !strings.Contains(string(result), `"author":"visible"`) {
		t.Fatalf("unicode credential projection result=%s err=%v", result, err)
	}
}

func TestJSONProjectionKeepsOrdinaryLogsAndRejectsMalformedData(t *testing.T) {
	input := `{"content":"2026-10-24 nginx: syntax is ok\npassword=synthetic-secret\nrequest finished","sizeBytes":123,"resourceVersion":"sha256:test"}`
	result, err := JSON([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "synthetic-secret") || !strings.Contains(string(result), "syntax is ok") || !strings.Contains(string(result), "sha256:test") {
		t.Fatalf("unexpected log projection: %s", result)
	}
	for _, invalid := range []string{"{", `{} {}`, `{"ok":true} garbage`} {
		if _, err := JSON([]byte(invalid)); err == nil {
			t.Fatalf("invalid JSON accepted: %s", invalid)
		}
	}
}

func TestStructuredJSONLinesAndRepeatedEncodingUseSameProjection(t *testing.T) {
	log := "{\"auth\":\"jsonl-secret\"}\n{\"note\":\"ordinary entry\"}"
	nested, _ := json.Marshal(`{"auths":{"registry":{"auth":"encoded-secret"}},"safe":"original"}`)
	input, _ := json.Marshal(map[string]string{"content": log, "nested": string(nested)})
	result, err := JSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "jsonl-secret") || strings.Contains(string(result), "encoded-secret") || !strings.Contains(string(result), "ordinary entry") || !strings.Contains(string(result), "original") {
		t.Fatalf("structured log projection failed: %s", result)
	}
}
