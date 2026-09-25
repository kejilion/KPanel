# Findings detail

## Confirmed

There are no confirmed findings in this scoped run.

## Rejected prior claim

The run-8 fingerprint `scenepacks.redirect.public-egress-before-origin-check` described a cross-host follow-up GET before the fixed-source URL comparison. In the current production path, `internal/panel/server.go:204` creates the store with its built-in fetch; `internal/scenepacks/store.go:57` sets `RejectRedirects: true`; and `internal/remotedownload/client.go:112-115` rejects the redirect callback before the HTTP client sends a second request. The final URL comparison remains defense in depth. The prior fingerprint is resolved as rejected for this source.

## Retained needs-validation record

The only retained candidate is `terminal-sse-output-after-session-revocation`. Its final schema-shaped record, verified source trace, exact blockers, and resolution plans are in [findings.json](findings.json) and [NEEDS-VALIDATION.md](NEEDS-VALIDATION.md). It is not a confirmed vulnerability.
