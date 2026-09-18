# KPanel Security Audit Report — run-1

**Run profile**: standard (two hunting waves + coverage critics) · **Scope**: full repository · **Budget**: none set · **Source ref**: `6340e078` (main), worktree dirty: 19 files, +40/−640 (batch-terminal refactor removals) · **Execution policy**: sandboxed-source-and-local-only — all checks source-only; Windows host provides no OS-enforced sandbox, so no target-controlled execution was authorized. Dynamic confirmations are described as safe local/deployment plans. · **Prior runs**: none (first audit of this repository). · **Coverage**: 45 ledger units (40 seeded + 5 added by the post-wave critic, closed in wave 2); final-clean critic returned stop=true with no missing units. · **Agents spent**: 4 recon + 10 hunters (wave 1: 8, wave 2: 2) + 2 critics + 1 Phase-3 verifier + 1 Phase-5 verifier = 18 agents.

## Security posture summary

KPanel's security architecture is unusually disciplined for this product category: paneld (non-root, behind operator TLS proxy) + root agent over a permission-locked Unix socket; constant-time comparisons everywhere tokens are checked; Argon2id with dummy-hash parity; triple-bound CSRF (header == cookie == session-hash); strict positive validation on virtually every command-execution surface (argv-only construction, trusted-script file checks, hex-encoded free text); full SSRF filter chains with DNS pinning on all outbound fetchers; Noise/ed25519 mutual auth with replay guards on every federation hop; node-side re-validation independent of the center. Of 45 audited trust-boundary units, 44 held against source-level attack. The one confirmed finding is a narrow but high-impact validation asymmetry in the backup-restore chain. Run again before releases: repeated runs find what single runs miss.

## Confirmed findings

| Severity | Title | Boundary | Observed result (source-settled) |
|---|---|---|---|
| **high** | hostbackup restore applies imported payload Root.Path values outside the /home data model with no positive allowlist | import/restore → root host filesystem | A crafted .kpb (attacker-known password) declaring module apps with root path /root (or /etc/ssh, /etc/cron.d, /usr/local/bin, /var/lib/docker) and zero containers passes the only scope check (negative exclusion list), proceeds through preview (which never displays root paths), and Engine.Restore as host root swaps attacker-authored uid/gid-0 mode-≤07777 content into the target directory and deletes the original. |

See [FINDINGS-DETAIL.md](FINDINGS-DETAIL.md) for the complete source path and target-neutral reproduction.

## Finding detail — summary

**Affected boundary**: authenticated admin session (import+restore UX) → agent root filesystem write/delete.

**Lower-trust principal**: a third party who authors a .kpb and supplies its password (migration-helper / shared-backup / recovery scenario). The admin performs the import and restore; every screen shows only module names and sizes — root paths never appear in web, CLI, or API previews, so the attack surface is indistinguishable from a routine restore.

**Root cause**: validatePayload ([internal/hostbackup/archive.go:393](C:/GitHub/kejilion-panel/internal/hostbackup/archive.go)) checks each payload Root.Path only for canonical-absolute form and `!excluded()` — a negative list of /proc,/sys,/dev,/run,/var/run and panel-owned dirs. The exporter's positive data model (owner(): /home/web→web, /home→apps, container binds→docker) is never enforced on import, nor are payload roots compared against the destination inventory at restore time. Bidirectional `within()` excludes literal parents (/etc, /var/lib) but siblings and subdirectories pass.

**Smallest fix**: in validatePayload, require each root's path to lie inside the declared module's data model (web → /home/web; apps → /home with owner()==apps; docker → bind source or volume mountpoint of a payload-declared container), require Root.ID == hex(sha256(path)[:16]) per the exporter convention, re-verify against destination Inventory before the first rename in Engine.Restore, and surface root paths in the preview response and both confirmation UIs. Regression case: zero-container apps payload with root /root must be rejected at import.

## Needs validation

None. (Wave-1 and wave-2 hunters settled every other unit at the source level; no needs_validation leads survived the candidate gates. The two conditions on the confirmed finding that are host-state dependent — no nested mount under the target, no bind-mount overlap — are characterized in the finding and typically absent on stock hosts.)

## Hardening notes

Recorded during hunting (non-findings; see agent hardening logs in run-1/hunter-hardening.json for full text). Highest-value subset:

1. **Trusted-script Stat→ReadFile TOCTOU** (internal/sites/recipe_jobs.go, internal/systemmanage/resources.go, internal/diagnostics/service.go): the same file-trust chain that node configs, sshlogin, and the notification store close with SameFile. Exploitation requires root today.
2. **AI auto-mode file denylist omits cron dirs** (internal/panel/ai_tools.go): in auto-approval mode a prompt-injected model can read then overwrite an existing /etc/cron.d file; manual mode still gates this. Add /etc/cron.* and /var/spool/at to aiFileMutable/aiFileReadable.
3. **Notification credential plaintext at 0600** (internal/notification/store.go) while TOTP secrets are AES-GCM sealed — reuse the totp-encryption.key sealing scheme.
4. **App-market manual update path pulls :latest** (packaging/kejilion-app/kpanel.conf:36,1600,1682,1847): mitigated post-pull by version-label re-validation, but the automatic path is digest-pinned — resolve the digest for manual flows too.
5. **govulncheck invoked by version tag not digest** in ci.yml/release.yml (trivy is digest-pinned); pin the module by version+sum.
6. **Forwarded-header trust contract** (internal/panel/server.go:1745): remoteIP is correct for the standard chain; document that the operator's loopback proxy must append the client address or per-IP rate limits weaken.
7. **self-update release-body digest anchor** (internal/selfupdate/source.go): the 24h hold cannot distinguish an edited release body from a new release; cross-check release assets or an attested digest to reduce reliance on body text.
8. **checkOrigin/checkHost with empty PublicURL**: legacy engines without Origin/Sec-Fetch fall back to CSRF+SameSite only, and Host pinning no-ops — document "set publicUrl for name-based deployments".
9. **Fixed-window limiter boundary doubling** (internal/cluster/guard.go, file_shares.go): ~2x nominal burst across window boundaries; token bucket + LRU eviction for full tables.
10. **Public file-share stream gate is global (2)** rather than per-share — two slow readers starve all anonymous downloads for up to 2h.
11. **health endpoint discloses version + init state anonymously** — intentional liveness design; trim if undesired.
12. **Download tickets reusable within 5-min TTL** — intended for Range/resume (test-confirmed); document as accepted design.

Positive patterns worth preserving: hex-encoding of free-text into argv (disk jobs); three-layer contract validation (panel/agent/systemmanage) on system actions; owner-derivation convention Root.ID=sha256(path) already present in the exporter (the fix hooks into it); SameFile anti-TOCTOU on node config/secret reads; scope-disjoint key namespaces in federation.

## Coverage summary

- 45 units: 44 covered, 1 candidate-linked-unit (the backup unit, now carrying the confirmed finding). 0 blocked, 0 deferred, 0 out-of-scope.
- Wave 1: 40 seeded units, 8 hunters. Post-wave critic found 5 genuine gaps. Wave 2: 5 units, 2 hunters. Final-clean critic: stop=true, no missing units, all 4 cmd/ binaries and all internal/ external-facing packages evidenced.
- Excluded companion domains with reasons: MEMORY-SAFETY (pure Go, no native parsers), AI-AND-LLM (AI client with host tools covered under Access control; no untrusted-context prompt assembly), CLOUD-AND-DEPLOYMENT (deploy manifests trivially small: non-root, cap_drop ALL, internal networks — reviewed under supply-chain light), DESKTOP-MOBILE-LOCAL-IPC (Unix socket covered under panel→agent), PROTOCOLS-RPC (federation covered by dedicated units).
- Both validators PASS: findings.json (1 confirmed record), coverage-ledger.json (45 units).

## Execution statement

All checks were source review. No target code was built, executed, or network-contacted; no deployed endpoints probed. The confirmed finding's dynamic reproduction is specified as a bounded local harness (extend internal/hostbackup native integration tests with a crafted payload) and a disposable-VM deployment check — both described in FINDINGS-DETAIL.md and not performed in this run.
