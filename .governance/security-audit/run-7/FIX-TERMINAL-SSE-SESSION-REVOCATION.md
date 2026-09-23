# Terminal SSE session revocation follow-up

- Date: 2026-09-23
- Fingerprint: `terminal-sse-output-after-session-revocation`
- Original audited source: `ce2fffa6f66c81beeb97b903e2701a76462016a5`
- Handoff baseline: `9af65132a4b8bc4ba2ce8d36a0b4597339bee4e3`
- Fix commit: `601548083ecebd49787402d8cafa51b697a9b2fc`
- Status: source fixed and regression tested locally; independent review pending.
- RC / stable / deployed: none for this fix; no push, merge, tag, release or deployment.

This is a normal fix follow-up, not a new security audit run. The historical findings,
ledger, source identity and coverage claim remain unchanged. The only formatting
correction to the original report removes its extra trailing blank line, which blocked
the first L2 attempt at `git diff --check` before any tests ran.

## Reproduction and fix

New tests on the handoff implementation reproduced continued subscribed terminal output
after logout, password change and Passkey disable with the production 15-second heartbeat.
Each revocation case still delivered output beyond the 1.5-second assertion window.
An idle stream with a shortened session expiry also failed to close before the test's
two-second deadline. Passkey disable exercises the store-wide revocation path; a complete
browser authenticator rebind ceremony was not exercised.

The fix caches a successful session check for at most one second before output writes.
Failed revalidation emits `auth.expired`, skips the dequeued output and returns through
stream cancellation/removal. An expiry timer also closes idle expired sessions. Heartbeats
stay at 15 seconds. Idle revoked sessions can remain until the next heartbeat; bytes already
written into the transport cannot be recalled, and slow-client writes retain the existing
20-second timeout. This is not a promise of instantaneous revocation or network delivery.

## Verification

- Linux gate image: `kpanel-release-gate:go1.26.7-node24`, WSL `Ubuntu`, root Docker access.
- `go test ./internal/panel/ -run TerminalStream -count=1 -timeout=90s`: PASS (6.603s).
- `go test -race ./internal/panel/ -run TerminalStream -count=3 -timeout=120s`: PASS (20.875s).
- OCR 1.12.6: free-form first, then constrained review; 2/2 files, no exclusions,
  no valid new findings. Reviewed `9af65132..7af8fc07`; `60154808` has the same tree and
  only adds review/audit trailers. This author review does not replace independent review.
- L2: first attempt on `60154808` blocked by the pre-existing report EOF blank line;
  the corrected candidate must rerun the full L2 gate against `71d50138`.
- Coverage at `60154808`: `decision=ok`, one unaudited boundary commit / one file,
  age 0 days, no new boundary package. The fix carries an explicit deferred trailer for
  the next scoped audit; it is not falsely included in run-7 coverage.
- Writer completion check at `60154808`: PASS, clean, one commit above the handoff baseline.
- Evidence: `C:/GitHub/_review-evidence/terminal-file-v3-ocr/session-revocation-20260923/`.

`scriptLinkageState=not-required` (无需发布脚本（不适用）): this change only alters Panel
session checks and does not modify script protocols, actions, installation or host artifacts.
The Dockerfile still pins script commit `2b90b2d2ca56bc954c9328a51bb5571e896f713d` and SHA-256
`806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`.
Local feature preview is not applicable: backend session enforcement and regression tests
only, with no frontend change. Existing full-feature previews do not validate this fix.

## Remaining boundaries

Independent review of the candidate, two real Panels plus a light node, proxy/CDN buffering
and upgrades, Safari/mobile, and the old-center callback compatibility decision remain open.
The other 14 run-7 leads are outside this task. Production 108 was not accessed.
Rollback the fix and its documentation with a revert of the task commits; the preserved
pre-task checkpoint is `9af65132`.
