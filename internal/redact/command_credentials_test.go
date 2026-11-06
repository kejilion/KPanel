package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCommonCommandCredentialsAreHidden(t *testing.T) {
	for _, line := range []string{
		`curl -u alice:OPAQUE_SYNTHETIC https://example.test`,
		`curl -ualice:OPAQUE_SYNTHETIC https://example.test`,
		`curl -su alice:OPAQUE_SYNTHETIC https://example.test`,
		`curl -sSualice:OPAQUE_SYNTHETIC https://example.test`,
		`curl --user=alice:OPAQUE_SYNTHETIC https://example.test`,
		"curl -u 'alice:part\nOPAQUE_SYNTHETIC' https://example.test",
		`docker login -u alice -p OPAQUE_SYNTHETIC example.test`,
		`docker login -pOPAQUE_SYNTHETIC example.test`,
		`docker login --password OPAQUE_SYNTHETIC example.test`,
	} {
		t.Run(strings.Fields(line)[0]+"-"+strings.Fields(line)[1], func(t *testing.T) {
			if strings.Contains(Text(line), "OPAQUE_SYNTHETIC") {
				t.Fatal("credential remained in text")
			}
			raw, _ := json.Marshal(map[string]string{"output": line})
			safe, err := JSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(safe), "OPAQUE_SYNTHETIC") {
				t.Fatal("credential remained in JSON projection")
			}
		})
	}
}

func TestOrdinaryCommandOptionsRemainVisible(t *testing.T) {
	for _, line := range []string{`curl --user-agent test https://example.test`, `docker run -p 8080:80 example`} {
		if got := Text(line); got != line {
			t.Fatalf("ordinary command changed: %q", got)
		}
	}
}
