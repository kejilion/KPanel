package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceBlocksKeepProjectionAndFailClosedOnAmbiguity(t *testing.T) {
	for _, content := range []string{
		"server {\n password=CONFIG_SECRET;\n listen 443;\n}\n",
		"server {\n key: CONFIG_SECRET;\n}\n",
		"server {\n auth: CONFIG_SECRET;\n}\n",
		"server {\n _auth=CONFIG_SECRET;\n}\n",
		"server {\n # _auth=CONFIG_SECRET\n listen 443;\n}\n",
		"server {\n location / { proxy_set_header Authorization \"Basic dTpw\"; }\n}\n",
		"server {\n location / { proxy_set_header X-Api-Key CONFIG_SECRET; }\n}\n",
		"server auth=CONFIG_SECRET {\n listen 443;\n}\n",
		"auth CONFIG_SECRET {\n listen 443;\n}\n",
		"server {\n pass\\u0077ord=CONFIG_SECRET;\n}\n",
		"server {\n location \\u0061uth {\n  value CONFIG_SECRET;\n }\n}\n",
		"server {\n location / { password=CONFIG_SECRET; }\n}\n",
		"server {\n value = \"{\\\"passw\\\\u006frd\\\":\\\"CONFIG_SECRET\\\"}\";\n}\n",
		"server {\n false\n \"CONFIG_SECRET\"\n}\n",
		"server {\n \"passw\\u006frd\": \"CONFIG_SECRET\"\n}\n",
		"server {\n password=CONFIG_SECRET;\n",
	} {
		raw, err := json.Marshal(map[string]string{"content": content})
		if err != nil {
			t.Fatal(err)
		}
		projected, err := JSON(raw)
		if err == nil && (strings.Contains(string(projected), "CONFIG_SECRET") || strings.Contains(string(projected), "dTpw")) {
			t.Fatalf("secret leaked from %q: %s", content, projected)
		}
		if safe := Text(string(raw)); strings.Contains(safe, "CONFIG_SECRET") || strings.Contains(safe, "dTpw") {
			t.Fatalf("secret leaked by text projection: %s", safe)
		}
	}
}

func TestLogSeverityPrefixesDoNotHideStructuredFailures(t *testing.T) {
	for _, level := range []string{"notice", "TRACE", "debug", "info", "warning", "error", "critical", "alert", "emerg"} {
		content := "2026/11/05 [" + level + "] service started\npassword=CONFIG_SECRET\n"
		raw, _ := json.Marshal(map[string]string{"content": content})
		safe, err := JSON(raw)
		if err != nil || !strings.Contains(string(safe), "service started") || strings.Contains(string(safe), "CONFIG_SECRET") {
			t.Fatalf("level %s: projection=%s err=%v", level, safe, err)
		}
	}
	for _, content := range []string{`prefix [null"CONFIG_SECRET"`, `prefix ["CONFIG_SECRET"`} {
		raw, _ := json.Marshal(map[string]string{"content": content})
		if safe, err := JSON(raw); err == nil && strings.Contains(string(safe), "CONFIG_SECRET") {
			t.Fatalf("malformed array leaked: %s", safe)
		}
	}
}
