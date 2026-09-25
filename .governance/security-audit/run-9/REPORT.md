# KPanel v1.22.0-rc.6 CF scoped security audit (run-9)

## Executive summary

This audit is frozen to source commit `4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c` (tree `444f26132591951256e18b471e1473b430f2dbfb`) in `C:/GitHub/_codex-tasks/kpanel-rc5-scene-playback`. The worktree was clean at both the start and end. This is a standard-profile, scoped, partial pass over the 13 paths and 9 commits produced by the coverage script relative to `4c0694aa8e02e46145a775707b8d5a0355f7ce10`; it does not claim a whole-repository audit.

No confirmed vulnerability remains. One terminal SSE logout-timing candidate is independently retained as `needs_validation`: source permits a possible same-session output `Write` during the remaining sub-second reauthentication cache, but the documents conflict over the required logout deadline and no runtime ordering was observed. The prior redirect finding is rejected for this source because the production scene-pack client rejects redirects before a follow-up request. Two other units are statically covered but runtime-blocked: browser enforcement of the opaque iframe/navigation boundary and filesystem crash durability of optional public artwork.

## Scope and source identity

- Repository/worktree: `C:/GitHub/_codex-tasks/kpanel-rc5-scene-playback`
- Frozen HEAD/tree: `4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c` / `444f26132591951256e18b471e1473b430f2dbfb`
- Comparison base: `4c0694aa8e02e46145a775707b8d5a0355f7ce10`
- Scope: `internal/ai/client.go`, `internal/cluster/stream_v3.go`, `internal/cluster/terminal_stream.go`, `internal/monitoring/hourly.go`, `internal/monitoring/record_decode.go`, `internal/monitoring/service.go`, `internal/panel/cluster.go`, `internal/panel/scene_packs.go`, `internal/panel/server.go`, `internal/panel/terminal_stream.go`, `internal/remotedownload/client.go`, `internal/scenepacks/catalog.go`, `internal/scenepacks/store.go`
- The 9 scoped commits are recorded in `run-metadata.json` and `coverage-source.json`.
- Pinned workflow: Cloudflare `security-boundary-audit`, skill commit `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`; local pin verification is linked from run metadata.
- Prior run evidence: runs 1-7 and frozen run-8 at `C:/GitHub/_release-evidence/v1.22.0-rc.5-20260925/security-run-8`, plus the prior transport-v3 audit listed in run metadata.
- Scene scope is the three in-scope packs; this review does not add other scene packs or unrelated repository paths.

## Findings

### Confirmed findings

None.

### Needs validation

| Fingerprint | Boundary | Exact unresolved point |
|---|---|---|
| `terminal-sse-output-after-session-revocation` | Authenticated terminal SSE output after logout | Source permits a possible application-level `ResponseWriter.Write` for the same existing subscription before the one-second auth recheck. The required logout timing is unclear: the overview says “immediate,” while the detailed protocol allows periodic active checks and idle heartbeat cleanup. Browser receipt and contract violation were not established. |

See [NEEDS-VALIDATION.md](NEEDS-VALIDATION.md) for the source trace and bounded resolution plans. Phase 5 independently verified the record. A product owner should define the timing contract before deciding whether this is an RC gate. If the intended guarantee is zero new output writes after session deletion or before the logout response, resolve and test that contract before release; if a bounded recheck delay is intended, align the documents and verify that bound in an isolated local fixture.

### Revalidated prior claim

The prior run-8 fingerprint `scenepacks.redirect.public-egress-before-origin-check` is rejected for this frozen source: the scene store enables `RejectRedirects`, and the redirect callback returns an error before sending a follow-up request. This is a resolved prior coverage lead, not a current finding. The audit does not claim private-network SSRF or credential disclosure.

## Coverage and review

The ledger contains 16 scoped coverage units: 13 `covered`, 1 `candidate`, 2 `blocked`, and 0 `deferred`. All 13 scope paths are mapped. The 9 same-source units carry prior reviewed evidence; changed-source scene-pack units were rechecked. The post-wave and distinct final-clean critics both returned no missing units or reassignments and resolved the prior redirect lead.

All 15 child invocations were explicitly configured as `gpt-6-luna` / `max`: 4 reconnaissance, 6 hunter assignments, 2 coverage critics, 2 Phase 3 verifiers, and 1 Phase 5 record verifier. The coordinator was also configured `gpt-6-luna` / `max`. Runtime token counts are not exposed and were not estimated.

## Execution and limits

The audit used source review only for target behavior. It did not execute target code, tests, builds, servers, browsers, fixtures, network requests, or fault injection because the environment did not provide the required OS-enforced isolation and evidence promoter. WSL was used only for trusted pinned structural validators. Both final checks passed: 16 coverage units valid and 2 findings valid.

The source worktree remained frozen and clean. Exact start/end HEAD and tree evidence is in `source-baseline-start.json` and `source-baseline-end.json`.

## Positive source patterns and hardening

- Scene pack paths and catalog metadata are bounded and validated; file access is tied to installed-pack capability, catalog entry, size, and digest.
- Scene Store uses fixed HTTPS roots, rejects redirects before follow-up requests, checks public dial addresses, and bounds downloaded content.
- Scene installation retires the old token at the committed state transition; quota checks and store operations are serialized.
- Optional artwork durability could be strengthened by syncing newly created object-directory entries before committing the state index and validating referenced object files during recovery. This is a durability improvement, not a confirmed security boundary failure.
- Align the terminal logout timing language across `docs/multi-host-terminal.md` and `docs/terminal-file-transport-v3.md`.

## Final status

The scoped static audit and independent coverage reviews are complete. Every ledger candidate has a Phase 3 disposition; the retained `needs_validation` record passed Phase 5. Linux/WSL structural validators passed. This is a complete run of the declared scope and remains a partial pass for the repository as a whole.
