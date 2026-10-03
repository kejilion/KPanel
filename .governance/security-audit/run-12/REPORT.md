# Run 12 — scoped review of saved Windows RDP credentials

**Status: complete for this 16-path scoped increment. `scope_complete=false`; the rest of the workflow scope remains unaudited.**

## Result

No reportable vulnerability was established in the source-only review. `findings.json` is empty. The saved-credential coverage umbrella has three named checks: (1) API session, Origin/CSRF, and user/host authorization; (2) vault AEAD, key handling, and persistence failure behavior; and (3) one-use browser handoff, clearing, and current credential-version/enrolled-node identity binding. The coverage critic recommended these as separate durable checks and did not report a new source finding.

## Scope and source identity

This is a scoped incremental review of `C:\GitHub\_codex-tasks\kejilion-panel-codex-rdp-oneclick-cf-source` at `2725d51386f930d54f71ddd8fcae5445f110730a`, tree `65aa80f306c73e57d72e6dba3703224534e2626f`, direct parent `6cac7e9e7211b25c70020658df64bf74ca547d33`. The incremental review base is `9ea1404407dbd89d3d2eeb02934257b870278fb3`; the workflow `comparison_base` is `4c0694aa8e02e46145a775707b8d5a0355f7ce10`.

The selected changed-path set contains 16 paths: 6 overlap the checker’s 188 boundary-file paths, and 10 related changed paths are outside that checker file set. The other 182 checker paths are outside this run. Three unchanged files were read only as source context for shared atomic-write and credential-version behavior; they do not enlarge the selected changed-path scope. `scope_complete=false`.

## Source evidence and limits

The source trace covers all 16 changed paths. It shows authenticated credential routes bound to the current Panel user and selected host; XChaCha20-Poly1305 ciphertext with user/host/version/node-identity AAD; a one-use, expiring session-bound handoff marked `no-store`; and browser password clearing after transfer to the IronRDP/WASM client. The vault’s current Binding equality check prevents retrieval when the current user credential version or enrolled node key identity differs from the stored binding.

The audit did not run target code, tests, builds, a browser, services, or system operations because no suitable OS-enforced sandbox was available. Effective deployed `DataDir` ACLs therefore remain unobserved. Hardening notes cover failed vault-open cleanup during host deletion and post-rename directory-sync error reconciliation; neither is promoted to a vulnerability without an established security-boundary consequence. Parent-provided test results are not audit execution evidence.

## Independent record review and finalization

`recon_entries`, independently of the run12 hunter and coverage critic, verified the frozen source identity, the 16/6/10/182 scope accounting, the 19-path ledger review union, the three named checks, and their cited source lines. Its recorded decision was `replace` because the snapshot it reviewed still marked this report Draft, findings validation pending, and metadata `in_progress`. I applied the requested finalization, preserved that original review record, and then ran both pinned validators successfully. The reviewer did not reread the post-final administrative wording.

## Validation

The pinned `validate-coverage-ledger.cjs` and `validate-findings.cjs` both pass under WSL Ubuntu Node `v24.18.1`; the validator’s `O_NOFOLLOW` requirement is therefore available. Exact commands and outputs are in `validation-results.json`.

Machine conclusions are high-confidence audit leads, not a human security review. This completed incremental run does not establish repository-wide safety.