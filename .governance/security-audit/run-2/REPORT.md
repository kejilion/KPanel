# KPanel run-2 scoped security audit

Profile: standard; scoped partial source-only pass. Source `23cbb9979ce0df84801b9fe614bd1b00d7ad8a99`; tree `dc4aaf80391407317119fa0a4ff01b05508cccd1`; clean at start and end. Comparison base `6340e0783d3e57873fd2c93a964372aefb9e1a81`.

The independent audit workflow is complete; dynamic validation is not. Machine conclusions are high-confidence leads, not a human security-review conclusion. This is not a release approval or a full-system safety claim.

Confirmed: 0; needs_validation: 4; rejected: 0. No severity is assigned to unresolved leads.

## Scope and execution

Reviewed 97 tracked paths across 16 current scope units. The 45 historical units and one pre-existing upload-parent lead remain explicitly out_of_scope. No old covered status is counted as current coverage. Previous run-1 had an unavailable dirty patch, so its complete source snapshot cannot be reconstructed.

Pinned skill: cloudflare/security-audit-skill c1c8a8c1471069fb0e188eeaff69b8e8db6564a8. Execution policy is sandboxed-source-and-local-only; only source checks were performed. Full OS network, filesystem and resource isolation was not established. No product tests, builds, fixtures, live traffic, target execution or target-generated artifact promotion occurred. Existing release test records are not new observations.

Budget: unset. Initially planned 4 recon, 5 hunters, 2 critics plus per-candidate validation. Actual: 4 recon, 7 hunters, 5 critics, 4 candidate verifiers and 5 final reviewers = 25 agent calls. Two additional units came from critics. Tokens: unreported (API unavailable). Wall time: 1861 seconds.

## Confirmed findings

None in this source-only run.

## NEEDS VALIDATION

| Lead | Source trace | Decisive blocker / next step |
| --- | --- | --- |
| AI redactor replacement drops bare credential filtering before model-context export | internal/panel/ai_tools.go:231 → internal/ai/client.go:228 | Bounded offline target-native reproduction remains unobserved; exact source-supported conditions and resolution plan are in NEEDS-VALIDATION.md. |
| Replayed audit migration may make the next event expire retained history prematurely | internal/panel/server.go:660 → internal/store/audit_log.go:188 | Bounded offline target-native reproduction remains unobserved; exact source-supported conditions and resolution plan are in NEEDS-VALIDATION.md. |
| Unrelated login failures can evict an account's unexpired rate-limit history | internal/panel/server.go:673 → internal/auth/service.go:293 | Bounded offline target-native reproduction remains unobserved; exact source-supported conditions and resolution plan are in NEEDS-VALIDATION.md. |
| Imported root path lengths bypass the persistent backup record read budget | internal/panel/backup_handlers.go:408 → internal/agent/server.go:218 | Bounded offline target-native reproduction remains unobserved; exact source-supported conditions and resolution plan are in NEEDS-VALIDATION.md. |

Owner-observed deployment checks are requested only where present in the individual record. No deployment traffic is authorized by this report.

## Previous finding and delivery

`hostbackup.restore.payload-root-unconfined-to-data-model` is source-resolved by commit e99e6b3d225425aa0b9afd14c55c8a33d75722f9: module/hash binding and exact destination-inventory membership precede restore writes. Hunters and independent coverage critics re-read the current controls. This does not rewrite run-1 or claim a new dynamic regression result. The regression tests exist but were not run here.

RC.1, RC.2 and RC.3 contain that fix. As checked 2026-09-19, stable Latest v1.19.0 does not. State: pending-stable; deployment unverified. Keep release/v1.20.0-candidate until successful stable release and the authoritative archival procedure.

## Hardening notes

These are separate from findings; no independent runtime proof or severity is claimed.
1. The changed light-store cleanup is safe for the validated-target-plus-residue case. An inherited reliability gap remains outside that hunk: light_store.go:108 initializes empty state when target is missing instead of using recoverAtomicTargetV2 as light_batch_store.go:67 does; cleanupOrphanCredentials then removes unreferenced key files. A crash between the two renames in store_v2.go:1092-1096 can reach that state. No lower-trust crash capability or newly introduced authority violation was established, so this is not a security finding.
2. The comment at internal/auth/service.go:643-646 still describes recovery audit/state as atomic, although store.go:376-404 intentionally uses separate database/state commits and compensating failure records. Update that comment when convenient; no unaudited credential change is claimed.
3. internal/panel/ai_tools.go:36 defines native Docker environment entries as {name,value}; safeArgumentSummary at :677 treats their strings independently, preserving an entry such as {"name":"MYSQL_ROOT_PASSWORD","value":"dummy-password"}. The new test at internal/panel/ai_tools_test.go:87 instead supplies environment strings. A source-grounded plaintext audit-copy gap remains (AppendAudit at ai_tools.go:281, raw JSON serialization at internal/store/audit_log.go:128). No distinct unauthorized audit reader was established in this single-administrator, 0600-database source model, so this is recorded as hardening rather than a cross-principal finding. Redact value based on sibling name and test the native object shape.
4. internal/sites/discover.go:95 says deletion is bound to the current configuration resource ID, whereas script_delete.go:161 and the explicit SiteDeleteDialog.vue:33-41 confirmation implement full domain-level deletion. Align this warning with the actual destructive scope; the authenticated administrator already confirms the domain-wide operation, so this is not reported as an authorization finding.
5. internal/terminal/manager.go:312 clears spawning before registration at 342, so the standalone Busy guarantee in its comment is stronger than the implementation. Keep spawning true through registration or cleanup for defensive consistency. Agent ServeHTTP's enclosing backupMutationMu (internal/agent/server.go:301-311) prevents the current HTTP backup admission path from observing this window.
6. internal/diagnostics/native.go:62-69 permits cross-host HTTPS redirects and does not pin DNS results to public addresses. Consider restricting redirect destinations to intended probe hosts or using public-address-validated dialing if provider responses are intended to remain outside the host-network trust boundary. Fixed endpoints and ordinary TLS verification prevent concluding that a lower-trust request can supply such a redirect from repository evidence alone.
7. internal/webenv/service.go:776 performs an ordinary blocking open before :785 validates descriptor identity. A no-follow/nonblocking open followed by regular-file verification would also avoid blocking on a FIFO substituted after Lstat. No lower-trust principal with write access to the fixed /home directory was established, so this is not a vulnerability candidate.
8. internal/webenv/service.go:300-307 sets CommandContext but no WaitDelay/process-group cancellation. Avoid interpreting the comment's 20-second deadline as proof that inherited subprocess pipes always close within that time; a bounded regression test could verify cancellation behavior. The trusted script implementation and an attacker-influenceable hanging descendant were not present as evidence, so no availability finding is claimed.
9. The federated wrapper at internal/panel/file_proxy.go:288 does not inspect panelUploadBody.exceeded as the local handler does at internal/panel/files.go:684-692. An oversized chunked request can therefore receive a generic 503 from streamFederatedAgent (:400-402) rather than a consistent 413. The read limit still fails closed and the Agent publication path rejects body-read failure; this is error-classification hardening, not a demonstrated security boundary failure.
10. cmd/kejilion-agent/backup_bundle.go:339 should render imported root paths with visible quoting/escaping of C0/C1, newline, carriage return, escape and bidi formatting characters, instead of emitting path bytes directly to the operator's terminal. BackupCenter.vue:157 could similarly expose invisible/formatting characters. This is a confirmation-display hardening note, not a demonstrated unauthorized restore or terminal code-execution finding: later Restore exact inventory membership prevents an archive-only fabricated control root from replacing data, and the additional pre-existing malicious-name/terminal conditions were not established. A future bounded regression should check literal escaped display and unchanged backend exact-path identity. No runtime result is claimed.

## Positive source patterns

Exact restore inventory membership, protected-root checks, authenticated Agent access, bounded JSON/body readers, source-IP trust controls, read-only AI path filtering, explicit destructive confirmation and fail-closed upload publication constrain the inspected paths. They do not prove unrelated subsystems safe.

## Coverage and final critic

Ledger status counts: {'candidate': 5, 'covered': 11, 'out_of_scope': 46}. Current scope = 16; outside scope = 46. Candidate units can share a fingerprint; unique record count is four. Blocked/deferred units = 0/0.

Final critic: no missing units, no reassignments; final clean result retained in critic-final2.json. All four unique candidates have separate Phase3 and Phase5 source review. The ledger retains candidates for unresolved dynamic validation.

CI/release execution, unchanged full subsystems, live proxy/TLS, deployment mounts/inventory and operating environment remain outside this pass. Source manifest lists the exact reviewed blobs. Upstream validators prove structure only.

Disclosure: retain this run locally until unresolved details have been triaged and approved for disclosure; the public governance candidate contains aggregate status only.
