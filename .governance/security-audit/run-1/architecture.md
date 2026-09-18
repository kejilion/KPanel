# KPanel Architecture Summary — security-audit run-1

**Target**: kejilion/KPanel @ 6340e078 (main, worktree dirty: 19 files, batch-terminal refactor removals). Product: single-admin Linux server management panel (Go backend, Vue 3 SPA), AGPL-3.0.

## Principals and authority

1. **Anonymous visitor** — reaches SPA assets, `/api/v1/health`, public cluster-share page/API (`/s/`, 64-hex token), public file-share metadata/content (`/f/`, 32-byte token, rate-limited), file download tickets (`/api/v1/files/download/{43-char}`, 5-min TTL), and all federation endpoints (own scheme auth). Security-entrance gate (`internal/panel/server.go:786-817`) hides everything else behind an unguessable path.
2. **Panel user** — exactly one admin account by construction (`Role: "admin"`, `internal/auth/service.go:246`); no user-creation endpoint. Session cookie (HttpOnly/Secure/SameSite=Strict, 32-byte token stored hashed), CSRF double-submit + Origin + Host checks on writes.
3. **Local agent (`kejilion-agent`)** — root daemon; sole host-mutation authority. Reached only via Unix socket `0660` root:`kejilion-panel` + shared bearer token (constant-time SHA-256 compare, `internal/agent/server.go:495-506`). Exposes terminals (root PTY), full filesystem manager rooted at `/` with protected/read-only/symlink-rejecting policy, docker control incl. exec, sites/nginx, app market, backups, self-update, system writes.
4. **kpctl** — local CLI reusing the agent token; read-only commands.
5. **Federation peers** — v1: ed25519-signed requests (±60s window, nonce replay guard, scope `summary` only). v2: Noise (X25519-ChaChaPoly) envelopes (±120s, per-controller replay), scope strings gate terminal/files (`internal/cluster/types.go:114-121`). A paired controller can open root terminals on this host if scope allows.
6. **Light nodes (`kejilion-node`)** — enroll via one-time `kpl1.` (5 min) / batch `kpb1.` (≤7d, bounded uses) HTTPS-bound tokens → per-node 32-byte reportingKey (HMAC-SHA256 over method/path/nodeID/timestamp/requestID/body) + optional Noise terminal/file keys. Report telemetry; can host root terminal/file brokers reached via outbound long-poll relay.

## Comparable baseline

Source-grounded: 1Panel / 宝塔 (docs cite both; same single-admin Chinese server-panel category), Nezha for cluster monitoring. KPanel's differentiator: non-root containerized panel + root Unix-socket agent split ("no shadow truth"). Trade-off both 1Panel and Baota share: full host file/shell exposure to the authenticated admin is the product's core function — not a finding by itself.

## Entry surfaces → sinks

- **Browser → panel**: `serveAPI` dispatch (`internal/panel/server.go:256-441`) — ~60 routes; JSON capped (default 1 MiB, max 16 MiB, DisallowUnknownFields), binary upload 512 MiB streamed to agent. No browser websockets (terminal/monitoring are JSON polling). WebSocket only on `/api/v2/federation/files/stream` (Noise handshake per socket).
- **Panel → agent**: `allowedAgentPath` GET allowlist + structured `HostOperationService.Do` for writes (`internal/panel/server.go:1113-1269`). Agent 64 KiB JSON cap.
- **Federation/light inbound**: `/api/v1/federation/*` (ed25519), `/api/v2/federation/*` (Noise), `/api/v3/federation/light/*` (enrollment + HMAC).
- **Root-command paths (all trusted-script or fixed-argv, no raw `sh -c` of user strings found in recon)**: terminal PTY, recipe/site jobs, webenv (argv + env, `storedArgumentsAllowed` enum), app-market (`k app {selector}`), diagnostics, self-update lifecycle script `/bin/bash`, docker exec `/bin/sh -lc` (in-container, 2048-byte single-line).
- **Outbound URL fetchers**: remote download (DNS-pin + public-address-only + redirect re-validation, `internal/remotedownload/client.go`), site icons (dial pinned to 127.0.0.1:80/443), app catalog (fixed https origin), Telegram (fixed https origin), AI providers (admin-configured, public scope requires HTTPS; private requires explicit confirmation), self-update (GitHub API + `docker.io/kjlion/kejilion-panel@sha256:` digest from release body).

## Trust boundaries and strongest source-visible controls

| Boundary | Control |
|---|---|
| Anonymous → panel | security-entrance path + token-gated public routes + per-IP rate limits |
| Browser mutation → panel | requireSession + checkCSRF (double-submit, constant-time) + checkOrigin + checkHost vs PublicURL |
| Panel → root host | Unix socket (FS perms) + bearer token + panel-side route allowlists + filemanager protected/read-only/symlink policy (`resolveExisting` walks every component, rejects symlinks) |
| Peer panel/light → panel | ed25519 signature / Noise envelope + replay guard + scope strings + per-node key verification |
| Panel → external network | publicAddress filter, reserved-prefix list (incl. 168.63.129.16), redirect policy, HTTPS binding on enrollment |

## Deployment-dependent (needs_validation if decisive)

TLS termination and HSTS live on the operator's reverse proxy (panel serves plain HTTP :8080); trusted-proxy CIDR correctness; Docker network isolation; systemd sandbox enforcement; `kejilion.sh` external script integrity beyond build-time sha256 pin.

## Offline execution limits

All checks in this run are **source-only**: Windows host has no OS-enforced sandbox (no network namespace/rlimits), so target-controlled execution (incl. `go test`) is not authorized; dynamic confirmations become needs_validation with a safe local plan. Go toolchain + module cache exist for an owner-run bounded harness.

## Companion selection (RECONNAISSANCE.md §8)

Selected: **WEB-PROTOCOL-AND-AUTH.md** (HTTP auth, session, CSRF, MFA surfaces — browser boundary), **CLIENT-SIDE.md** (Vue DOM sinks, v-html, window.open, localStorage), **SUPPLY-CHAIN-AND-RELEASE.md** (self-update digest chain, trusted-script trust, app market, Dockerfile CI), **RESOURCE-EXHAUSTION-AND-AVAILABILITY.md** (unauthenticated surfaces, pre-auth work, terminal/session limits), **DATA-ISOLATION-AND-LIFECYCLE.md** (backup import/restore authority, share/token lifecycle). Excluded: MEMORY-SAFETY-AND-BINARY.md (pure Go, no native parsers — GC'd), AI-AND-LLM.md (target embeds an AI *client* with host tools, but no model-controlled prompt assembly/agent loop on untrusted context; AI tool misuse is covered under Access control), PROTOCOLS-RPC-AND-MESSAGING.md (federation covers covered by v1/v2 units), CLOUD-AND-DEPLOYMENT.md (deploy manifests reviewed under supply-chain light: compose runs panel non-root, cap_drop ALL, internal networks — no IAM/IaC attack surface), DESKTOP-MOBILE-AND-LOCAL-IPC.md (Unix socket covered under panel→agent boundary).

## Prior runs

None — first audit of this repo. No prior ledger exists; all units seeded `none`.

## Starting paths

`internal/panel/server.go` (router), `internal/agent/server.go` (agent routes), `internal/cluster/` (federation), `internal/filemanager/manager.go`, `internal/remotedownload/client.go`, `internal/sites/`, `internal/webenv/service.go`, `internal/selfupdate/`, `internal/ai/`, `internal/auth/service.go`, `web/src/` (frontend), `internal/panel/ai_tools.go`.
