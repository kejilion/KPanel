# Transport v3 scoped audit public receipt (run-7)

The original run completed on 2026-09-23 against clean source
`ce2fffa6f66c81beeb97b903e2701a76462016a5`, tree
`d369bdbb6e7306bcfa1d2320da4899f693144dfd`. Its scope was the 27 paths
and seven boundary commits recorded in `run-metadata.json`.

- Result: scoped partial pass, source-only. No target code or dynamic exploit
  verification was executed during this audit.
- Original dispositions: 0 confirmed, 15 needs_validation, 3 rejected.
- Coverage: 36 units, comprising 19 covered, 16 candidate and 1 out_of_scope.
- The pinned skill validators accepted 18 findings and 36 coverage units.
- These are coverage and structural-validation results, not a claim that the
  product has no vulnerabilities or that outstanding hypotheses are confirmed.

Following `PROJECT_RULES.md` section 5.4, this public history contains only the
run identity, counts, source coverage and this disclosure receipt. Full findings,
validation plans, architecture notes and the coverage ledger remain in the local
evidence directory recorded in metadata and the preserved local-only candidate
`33aa84c85d4989cecfa6e4a6c1a5785f3b00f4ee`. Original artifacts are unchanged;
their SHA-256 values are recorded for traceability. The public metadata omits
per-finding review receipts and critic details rather than changing audit results.

The session-revocation follow-up is published with its regression-tested fix;
see [FIX-TERMINAL-SSE-SESSION-REVOCATION.md](FIX-TERMINAL-SSE-SESSION-REVOCATION.md).
The other 14 original needs_validation leads remain unconfirmed and outside this
release's implementation scope. The historical audit is not rewritten to include
the follow-up fix or the AI model-discovery change; coverage checking tracks those
new commits separately.
