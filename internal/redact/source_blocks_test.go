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
		"server {\n pass\\u0077ord=CONFIG_SECRET;\n}\n",
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
		if err == nil && strings.Contains(string(projected), "CONFIG_SECRET") {
			t.Fatalf("secret leaked from %q: %s", content, projected)
		}
		if safe := Text(string(raw)); strings.Contains(safe, "CONFIG_SECRET") {
			t.Fatalf("secret leaked by text projection: %s", safe)
		}
	}
}
