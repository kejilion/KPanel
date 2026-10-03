# Run 11 — Windows CF scoped security audit

**Status: incomplete.** Frozen-source review and candidate verification artifacts exist, but the required final coverage critic and independent Phase 5 surviving-record review were not completed. Attempts to create or reactivate dedicated gpt-6-luna/max agents were rejected with agent thread limit reached; no model or reasoning fallback was used. The seven candidate records remain in findings.json with precise blockers, and this run is not a completed coverage result.

## Frozen source and scope

Target: C:\GitHub\_codex-tasks\kejilion-panel-codex-windows-cf-source  
HEAD 431345f6097d08ac96aedec18f5605ce8a191aeb, tree 1211c022438ef4dc6ef51da121d0237edf3ad75f, direct parent c18bb50c552ba98c4a75e4a1fa968b9226dd2f1a; Windows Git recorded a clean worktree. Workflow comparison base is 4c0694aa8e02e46145a775707b8d5a0355f7ce10.

Coverage checker output contains 186 changed paths, 79 commits, and 3 new packages. This Windows/shared-boundary run inspected 81 selected paths and explicitly left 105 changed paths out of scope. Its coverage ledger has 16 units: 13 Windows/shared-boundary units (6 covered, 7 candidate) and 3 explicit out-of-scope units (Prior run-1 host backup, Prior run-10 generic rollback, unrelated changes). scope_complete=false; the run does not claim the broader change set or repository is fully audited.

## Findings state

findings.json contains seven needs_validation candidates and no confirmed or rejected finding. The candidates concern RDP output around logout, desktop-role re-admission after policy revocation, release-signing deployment configuration, effective Windows SCM ACLs, conditional PSReadLine history exposure of a one-time enrollment token, replay of a complete valid telemetry request across service reconstruction, and terminal SSE output around logout. Every conclusion is bounded by the source evidence and exact blockers in NEEDS-VALIDATION.md; no severity is assigned.

The independent source verification for U2 confirms only post-revoke desktop-role re-registration, not a usable RDP session or bytes. The U15 verifier confirms its replay guard and report freshness path predate this Windows increment; the delta is Windows applicability only. U12 deployment/tag/environment controls remain owner-validation items, not established permissive settings. U13 does not assume the Windows SCM default DACL is weak. U14 does not infer an actual token leak. U1 and U16 do not claim client-visible post-logout delivery or an unrequested atomic logout/write contract.

## Remediation follow-up

The frozen source remains 431. Later source-only review found U2’s desktopAllowed check in f82eaf81d2a96c113117b53158ce8c1a42ce8ade in the same authorization/registration lock section. For the f82eaf81..651a75ef3b874a83aee0c81e46177842653f0465 RDP output patch, OCR review covered 3/3 Go files and the unsupported Markdown file was read manually; per-write authentication matches the documented subsequent-frame recheck contract. These are separate source remediation reviews, not Windows runtime proof. Parent-provided race/CI logs were not executed by this audit agent and are not audit evidence.

## Method and remaining work

No target code, tests, builds, installer, browser, service, or system operation was run: the host lacked the required OS-enforced no-network, empty-environment, read-only-source/toolchain, scratch-only-write sandbox. Dynamic questions remain explicitly needs_validation.

The run remains incomplete because the required independent final coverage critic and fresh Phase 5 surviving-record review were not completed. New-agent and existing-agent follow-up requests for dedicated gpt-6-luna/max reviewers were rejected at the platform thread limit; no model fallback was used. Both pinned structural validators passed after ledger schema corrections; exact results are in validation-results.json. A future continuation must perform only the missing final critic and Phase 5 record verification against this frozen source and persisted ledger, then update run_status only if protocol requirements are satisfied. A whole-scope rescan is not required.

Pinned structural validators run under WSL Ubuntu Node v24.18.1 with O_NOFOLLOW; final results are recorded in validation-results.json. Machine conclusions are high-confidence leads, not a human security review.