package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsCredentialFamiliesAndKeepsSafeURLFields(t *testing.T) {
	input := strings.Join([]string{
		`refresh_token=refresh-secret`,
		`AWS_SECRET_ACCESS_KEY=aws-secret`,
		`pwd=pwd-secret`,
		`password=p@ss#2026`,
		`token=abc&def`,
		`Authorization: Basic YmFzaWMtc2VjcmV0`,
		`Authorization: ApiKey arbitrary-auth-secret`,
		`Proxy-Authorization=Custom another-auth-secret`,
		`sk-example-dummy-secret`,
		`Basic ZHVtbXk6ZHVtbXk=`,
		`Cookie: session=cookie-secret; csrf=csrf-secret`,
		`https://url-user:url-pass@example.test/path?access_token=query-secret&safe=visible`,
		`https://token-only@example.test/path`,
		`https://example.test:8443/path`,
	}, "\n")
	output := Text(input)
	for _, secret := range []string{
		"refresh-secret", "aws-secret", "pwd-secret", "p@ss#2026", "abc&def",
		"YmFzaWMtc2VjcmV0", "ApiKey", "arbitrary-auth-secret", "Custom", "another-auth-secret",
		"sk-example-dummy-secret", "ZHVtbXk6ZHVtbXk=",
		"cookie-secret", "csrf-secret", "url-user", "url-pass", "query-secret", "token-only",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("secret %q was not redacted: %s", secret, output)
		}
	}
	if !strings.Contains(output, "safe=visible") {
		t.Fatalf("safe URL query was removed: %s", output)
	}
	if !strings.Contains(output, "https://example.test:8443/path") {
		t.Fatalf("ordinary URL host port was redacted: %s", output)
	}
}

func TestTextConsumesEntireOrdinaryAssignmentButPreservesSafeURLQuery(t *testing.T) {
	output := Text(strings.Join([]string{
		`password=p@ss#2026`,
		`token=abc&def`,
		`password=a,b;c`,
		`--token d,e;f`,
		`https://example.test/path?access_token=query-secret&safe=visible`,
	}, "\n"))
	for _, leaked := range []string{
		"p@ss", "#2026", "abc", "&def", "a,b", ";c", "d,e", ";f", "query-secret",
	} {
		if strings.Contains(output, leaked) {
			t.Fatalf("assignment/query redaction left %q: %s", leaked, output)
		}
	}
	if !strings.Contains(output, "&safe=visible") {
		t.Fatalf("safe URL query parameter was removed: %s", output)
	}
}

func TestTextRedactsArbitraryAuthorizationSchemeAndValue(t *testing.T) {
	output := Text(strings.Join([]string{
		`Authorization: ApiKey arbitrary-auth-secret`,
		`Proxy-Authorization=Custom another-auth-secret`,
		`--authorization Digest flag-auth-secret`,
	}, "\n"))
	for _, leaked := range []string{
		"ApiKey", "arbitrary-auth-secret", "Custom", "another-auth-secret", "Digest", "flag-auth-secret",
	} {
		if strings.Contains(output, leaked) {
			t.Fatalf("authorization redaction left %q: %s", leaked, output)
		}
	}
}

func TestRecordsHidePrivateKeyAcrossRecords(t *testing.T) {
	values := []string{
		"before",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"cHJpdmF0ZS1rZXktbWF0ZXJpYWw=",
		"-----END OPENSSH PRIVATE KEY-----",
		"after",
	}
	output := Records(values)
	joined := strings.Join(output, "\n")
	if strings.Contains(joined, "cHJpdmF0ZS1rZXktbWF0ZXJpYWw") || !strings.Contains(joined, privateKeyReplacement) ||
		!strings.Contains(joined, "before") || !strings.Contains(joined, "after") {
		t.Fatalf("private key redaction failed: %s", joined)
	}
}

func TestRecordsHandleOrphanEndAndLaterBlockWithinOneMultilineRecord(t *testing.T) {
	output := Records([]string{strings.Join([]string{
		"window-private-body",
		"-----END RSA PRIVATE KEY-----",
		"safe-after-orphan",
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"later-private-body",
		"-----END OPENSSH PRIVATE KEY-----",
		"safe-after-block",
	}, "\n")})
	if len(output) != 1 {
		t.Fatalf("records=%#v", output)
	}
	for _, secret := range []string{"window-private-body", "later-private-body"} {
		if strings.Contains(output[0], secret) {
			t.Fatalf("multiline private-key window leaked %q: %s", secret, output[0])
		}
	}
	if !strings.Contains(output[0], "safe-after-orphan") || !strings.Contains(output[0], "safe-after-block") ||
		strings.Count(output[0], privateKeyReplacement) != 2 {
		t.Fatalf("safe suffixes or redaction markers missing: %s", output[0])
	}
}

func TestTextHidesUnclosedPrivateKeyThroughValueEnd(t *testing.T) {
	output := Text(strings.Join([]string{
		"before",
		"-----BEGIN PRIVATE KEY-----",
		"dW5jbG9zZWQtcHJpdmF0ZS1rZXk=",
		"still-private",
	}, "\n"))
	if strings.Contains(output, "dW5jbG9zZWQtcHJpdmF0ZS1rZXk") ||
		strings.Contains(output, "still-private") ||
		!strings.Contains(output, privateKeyReplacement) ||
		!strings.Contains(output, "before") {
		t.Fatalf("unclosed private key redaction failed: %s", output)
	}
}

func TestLinesTracksPrivateKeyBeforeReturnedTail(t *testing.T) {
	output := Lines([]byte(strings.Join([]string{
		"before",
		"-----BEGIN RSA PRIVATE KEY-----",
		"cHJpdmF0ZS1ib2R5",
		"-----END RSA PRIVATE KEY-----",
		"after",
	}, "\n")), 2, 16<<10)
	joined := strings.Join(output, "\n")
	if strings.Contains(joined, "cHJpdmF0ZS1ib2R5") || !strings.Contains(joined, "after") {
		t.Fatalf("bounded line redaction leaked a private key: %s", joined)
	}
}

func TestLinesHidesWindowPrefixBeforeOrphanPrivateKeyEnd(t *testing.T) {
	output := Lines([]byte(strings.Join([]string{
		"cHJpdmF0ZS1ib2R5",
		"-----END RSA PRIVATE KEY-----",
		"after",
	}, "\n")), 2, 16<<10)
	joined := strings.Join(output, "\n")
	if strings.Contains(joined, "cHJpdmF0ZS1ib2R5") ||
		!strings.Contains(joined, privateKeyReplacement) ||
		!strings.Contains(joined, "after") {
		t.Fatalf("orphan private key end did not protect the bounded prefix: %s", joined)
	}
}
