package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsExactAuthKeysAndNestedAuths(t *testing.T) {
	for _, input := range []string{
		"auth: OPAQUE_BODY\n",
		"'auth': OPAQUE_BODY\n",
		`"\u0061uth": OPAQUE_BODY` + "\n",
		"auths:\n  registry.example:\n    auth: OPAQUE_BODY\n",
		"auth: |\n  OPAQUE_BODY\n",
		"AUTH=OPAQUE_BODY\n",
		"request auth=OPAQUE_BODY completed\n",
		"https://example.test/?auth=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?auths=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?%61uth=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?%61uths=OPAQUE_BODY&safe=visible\n",
	} {
		if output := Text(input); strings.Contains(output, "OPAQUE_BODY") {
			t.Fatalf("auth credential exposed in %q: %q", input, output)
		}
	}
	input := "ordinary authentication message\nauthority: visible\nhttps://example.test/?author=visible\n"
	if output := Text(input); output != input {
		t.Fatalf("ordinary text changed: %q", output)
	}
}

func TestTextRedactsFormatCharactersInSensitiveKeysOnly(t *testing.T) {
	for _, input := range []string{
		"pass\u200bword=OPAQUE_BODY\n",
		"pass\u200bword: OPAQUE_BODY\n",
		"pass\u200bword: |\n  OPAQUE_BODY\n",
		"--pass\u200bword OPAQUE_BODY\n",
		"Authori\u200bzation: Bearer OPAQUE_BODY\n",
		"B\u200bearer OPAQUE_BODY\n",
		"Ba\u200bsic dXNlcjpQQVNT\n",
		"-----BEGIN PRI\u200bVATE KEY-----\nOPAQUE_BODY\n-----END PRI\u200bVATE KEY-----\n",
		"https://example.test/?pass\u200bword=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?pass%E2%80%8Bword=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?signa%E2%80%8Bture=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?signature%E2%80%8B=OPAQUE_BODY&safe=visible\n",
		"https://example.test/?s%E2%80%8Big=OPAQUE_BODY&safe=visible\n",
	} {
		if got := Text(input); strings.Contains(got, "OPAQUE_BODY") || strings.Contains(got, "dXNlcjpQQVNT") {
			t.Fatalf("format-key leaked input=%q result=%q", input, got)
		}
	}
	ordinary := "author\u200b=visible metadata=present body=a\u200bb\nhttps://example.test/?design=visible&signal=visible&safe=visible\n"
	if got := Text(ordinary); got != ordinary {
		t.Fatalf("ordinary content changed: %q", got)
	}
}

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
		`https://example.test/path?design=dark`,
		`https://example.test/path?authtoken=auto-secret&clientsecret=client-secret&password1=number-secret&key=key-secret&sig=sig-secret&myKey=camel-key-secret&mySig=camel-sig-secret&api=visible`,
		`https://token-only@example.test/path`,
		`https://example.test:8443/path`,
	}, "\n")
	output := Text(input)
	for _, secret := range []string{
		"refresh-secret", "aws-secret", "pwd-secret", "p@ss#2026", "abc&def",
		"YmFzaWMtc2VjcmV0", "ApiKey", "arbitrary-auth-secret", "Custom", "another-auth-secret",
		"sk-example-dummy-secret", "ZHVtbXk6ZHVtbXk=",
		"cookie-secret", "csrf-secret", "url-user", "url-pass", "query-secret", "token-only",
		"auto-secret", "client-secret", "number-secret", "key-secret", "sig-secret", "camel-key-secret", "camel-sig-secret",
	} {
		if strings.Contains(output, secret) {
			t.Fatalf("secret %q was not redacted: %s", secret, output)
		}
	}
	if !strings.Contains(output, "safe=visible") {
		t.Fatalf("safe URL query was removed: %s", output)
	}
	if !strings.Contains(output, "design=dark") {
		t.Fatalf("ordinary URL query was redacted: %s", output)
	}
	if !strings.Contains(output, "api=visible") {
		t.Fatalf("ordinary api query was redacted: %s", output)
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

func TestTextRedactsMySQLShortPasswordOptionWithoutMaskingPort(t *testing.T) {
	output := Text(strings.Join([]string{
		`mysql -p history-secret`,
		`mysqldump -pattached-secret database`,
		`mysql -P 3306 -u panel`,
	}, "\n"))
	for _, leaked := range []string{"history-secret", "attached-secret"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("MySQL short password option leaked %q: %s", leaked, output)
		}
	}
	if !strings.Contains(output, `mysql -P 3306 -u panel`) {
		t.Fatalf("MySQL port option was redacted: %s", output)
	}
}

func TestTextRedactsQuotedAttachedMySQLShortPasswordOption(t *testing.T) {
	output := Text(strings.Join([]string{
		`mysql -p"alpha beta" database`,
		`mariadb -p'gamma delta' database`,
		`mysqldump -p"alpha \"quoted\" beta" database`,
		`mysql -p'alpha'\'' beta' database`,
		`mysql -p 'theta'\'' iota' database`,
		`mysql -p'omega\' database`,
		`mysql -p "kappa"-suffix database`,
	}, "\n"))
	for _, leaked := range []string{"alpha", "beta", "gamma", "delta", "quoted", "theta", "iota", "omega", "kappa", "suffix"} {
		if strings.Contains(output, leaked) {
			t.Fatalf("quoted MySQL password leaked %q: %s", leaked, output)
		}
	}
	if strings.Count(output, "[REDACTED]") != 7 || strings.Count(output, "database") != 7 {
		t.Fatalf("quoted MySQL arguments were not fully redacted: %s", output)
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

func TestRecordsHidePGPPrivateKeyBlock(t *testing.T) {
	output := Text(strings.Join([]string{
		"before",
		"-----BEGIN PGP PRIVATE KEY BLOCK-----",
		"",
		"lQVYBGVwZ3Atc2VjcmV0LWtleS1tYXRlcmlhbA==",
		"=AbCd",
		"-----END PGP PRIVATE KEY BLOCK-----",
		"after",
	}, "\n"))
	if strings.Contains(output, "lQVYBGVwZ3Atc2VjcmV0LWtleS1tYXRlcmlhbA") || strings.Contains(output, "=AbCd") ||
		!strings.Contains(output, privateKeyReplacement) || !strings.Contains(output, "before") || !strings.Contains(output, "after") {
		t.Fatalf("PGP private key redaction failed: %s", output)
	}
	if public := Text("-----BEGIN PGP PUBLIC KEY BLOCK-----\nmQENBGVwZ3AtcHVibGlj\n-----END PGP PUBLIC KEY BLOCK-----"); strings.Contains(public, privateKeyReplacement) {
		t.Fatalf("PGP public key was treated as private: %s", public)
	}
}
