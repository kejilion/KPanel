# KPanel v1.22.0 — CF security-boundary audit (run-10)

## Run and scope

- Profile: scoped. This is a partial pass over the release-coverage scope, not a full-product security review.
- Source: 690f3dc24fe81377f4e17d27c68e4a174761b973. Final baseline tree check is recorded in run-metadata.json.
- Comparison base: 4c0694aa8e02e46145a775707b8d5a0355f7ce10.
- Scope from check-security-audit-coverage.mjs: 9 commits, 10 files, and new boundary package internal/desktopwallpapers.
- No user-set invocation or token budget. There were 19 agent invocations: 16 completed and 3 were interrupted before a conclusion; no partial conclusion from interrupted invocations was adopted.
- Prior run evidence is listed in run-metadata.json. The previous wallpaper run-10 was incomplete and was not treated as coverage. The run-7 light-store lead was re-reviewed against current source and did not carry forward as a security finding.
- Execution was source-only. No target code, test, fixture, Docker, service, or browser ran because a parent-approved OS-enforced sandbox was unavailable.
- No deferred or out-of-scope units were recorded. The coverage ledger contains 24 units and the final-clean critic accepted the completed mapping.

## Security posture

Across the reviewed boundaries, source shows session checks and mutation CSRF/Origin checks on Panel operations, bounded admission and file-size limits for scene streams, capability validation before scene file reads, and typed operations across the Panel-to-Agent boundary. The application update path restores whole data directories; this can overwrite post-snapshot security changes. The possible light-node credential revival remains a needs-validation lead because its final authentication and telemetry effect was not dynamically observed.

Machine results are high-confidence review leads, not a human security review conclusion.

## Confirmed findings

There are 0 confirmed findings. No bounded local effect was observed under the required execution boundary.

| Severity | Title | Boundary | Observed result |
|---|---|---|---|
| — | None | — | No confirmed finding |

## Needs validation

| Title | Source trace | Exact blocker | Bounded next step | Owner-observed deployment check |
|---|---|---|---|---|
| Failed update rollback can restore a revoked light node's report credential | kpanel.conf snapshot → target verification → authenticated host deletion → full-directory restore → light-report HMAC → telemetry write | The target, tests, fixtures, or Docker could not run without an OS-enforced sandbox; source review cannot establish that the restored light store accepts a fresh dummy-node report and persists telemetry. | In an approved offline OS-enforced sandbox, use a temporary data directory and synthetic host/HMAC key; snapshot, delete the dummy host during target verification, force rollback, then submit a fresh signed dummy report and observe authentication and telemetry. Use low resource and time limits and no external network. | Not needed for this source claim; the local dummy fixture can settle it. Do not probe a deployed system. |

No severity is assigned to this lead. Its fingerprint is kpanel:rollback-restores-revoked-light-node and it links to two coverage units.

## Hardening notes

- Add a regression case that revokes a real-format dummy light-host record and secret during target verification, forces rollback, and asserts the node stays revoked and cannot update telemetry.
- Consider freezing security-state mutations during release verification or merging irreversible revocations into restored state.
- The release helper hash is checked against a label from the same image. The Dockerfile pins the fetched script checksum, but no independent signed provenance binding the image, metadata, and promoted host helpers was found in this source scope.
- OpenRC runs the root Agent with Docker-socket access and system writes enabled but lacks systemd's explicit capability, namespace, and syscall restrictions. No OpenRC-specific unauthorized operation was established.
- Add isolated OpenRC install/start/uninstall lifecycle coverage; the existing update-backup fixture uses fake init commands.
- The Agent token is a shared capability for the Panel workload. Consider narrower operation scope if lower-trust processes are introduced into that workload.
- The lifecycle harness's environment variable and /.dockerenv checks are not OS-isolation controls; keep its absolute-path mutations confined to the disposable runner.
- A wallpaper deleted from one browser may remain in another browser's local copy until that browser refreshes. The source did not establish a separate-user protected-data boundary; record this as deletion/lifecycle consistency hardening.
- Scene-file cache headers require revalidation before a cached response, and invalid capability tokens are rejected before file I/O or gzip.
- Scene stream admission is bounded to 400 waiting entries and four active streams; no per-capability fairness control is visible.
- If the target light-store file exists but is corrupt, NewService can fail even when a valid .previous file exists; source review treated this as local recovery/availability behavior, with no low-trust write path established.
- An enrollment response lost after commit has no idempotent retry path for the holder of a valid enrollment token; no unauthorized boundary was demonstrated.
- Automatic update rollback may restore a wallpaper deleted after the snapshot. This requires the host administrator/root lifecycle path; no lower-trust boundary violation was established.- The Windows no-follow implementation is not used by the Linux package target; no lower-trust writer to the private store was established.
- A prior light-store missing-target recovery lead was closed by the current .previous recovery and fail-closed behavior when credential/key files remain.
- Other source-only hunter hardening notes are retained in the coverage ledger.

## Positive source patterns

- Panel write routes use session, Origin, CSRF, typed request checks, and explicit resource/version constraints.
- Scene frame-file access binds the selected pack, capability token, installed state, and manifest; authorization is checked before cached response handling.
- Panel-to-Agent calls use a Unix socket, bearer token, fixed routes, and typed file/system operations; host file operations use a rooted filesystem and path restrictions.
- Compose drops capabilities, uses a read-only root, and gives the Panel only its intended writable data mount.
- The app update archive is checksummed and its restore layout is validated before directory exchange; the unresolved lead is about preserving post-snapshot revocations through rollback.

## Coverage and validation

- Ledger: 24 units; 22 covered, 2 candidate (one fingerprint linked to both), 0 blocked, 0 deferred.
- Wave-2 post-wave critic: no missing units or reassignments; resolved the prior scene-stream queue lead from source bounds.
- Final-clean critic: no missing units or reassignments.
- WSL pinned validators: validate-findings.cjs PASS (1 record); validate-coverage-ledger.cjs PASS (24 units).
- The audit does not claim source-only review proves deployed reachability or the unobserved rollback/report effect.