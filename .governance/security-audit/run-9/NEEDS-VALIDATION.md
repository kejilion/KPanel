# Needs validation and blocked boundaries

## Retained candidate: terminal SSE logout timing

No severity is assigned. Source-only evidence does not establish a violation of the intended contract or browser receipt.

### Affected boundary and source trace

An authenticated `GET /api/v1/terminal-stream` reaches `internal/panel/server.go:383` and `internal/panel/terminal_stream.go:169`. The handler registers a stream associated with the session user and token hash (`terminal_stream.go:187`). Logout in `server.go:976` calls `auth.Logout`, which deletes the token record through `internal/auth/service.go:599` and `internal/store/store.go:696`; that path does not cancel the already-open stream.

Before writing a dequeued output event, the stream invokes `checkSession` (`terminal_stream.go:255`). If less than one second has elapsed since the last successful authentication, `checkSession` returns true without consulting the session store (`terminal_stream.go:234`). The event may then reach `ResponseWriter.Write` (`terminal_stream.go:218`). The source establishes a possible application-level write for the same existing subscription; it does not establish that a browser received the bytes. The event queue capacity is 32 (`terminal_stream.go:34,125`). The existing test only fails if output arrives after the 1.5-second bound (`terminal_stream_test.go:288`) and was not run in this audit.

### Exact blockers

1. The timing contract is ambiguous. `docs/multi-host-terminal.md:147` says logout ends the stream immediately; `docs/terminal-file-transport-v3.md:103-105` describes per-second active rechecks, idle heartbeat cleanup, and bytes already written to the transport that cannot be withdrawn. The product owner must define whether the guarantee is zero new output writes after session deletion, termination before the logout response, or a bounded recheck delay; also define idle-stream timing and how already-written bytes count.
2. The deterministic ordering and browser receipt remain unobserved. This run was source-only because no compliant OS-enforced isolation and evidence promoter were available.

### Resolution plans

- **Local:** First resolve the product timing contract. Then, in an approved OS-enforced sandbox, use a read-only frozen source copy, synthetic session store, controlled clock, barriers, and in-memory `ResponseWriter`. Disable external networking; use an empty allowlisted environment; keep source/toolchain read-only; permit scratch-only writes; apply CPU, memory, process-count, file-size, disk, and wall-clock limits; and promote only allowlisted evidence through trusted no-follow code. Set `lastAuth` less than one second before `DeleteSession`, release a queued synthetic event, and record deletion, `Write`, and `auth.expired` ordering. Cover a full interval and idle heartbeat separately. Use synthetic output only.
- **Deployment:** Have the owner confirm one timing rule for logout, password change, and Passkey revocation, and align both documents. If browser behavior is still material, use a non-production dummy account with two tabs: keep one subscribed to SSE and log out in the other, recording session deletion, logout response, application SSE frames, and test-browser receipt. Do not use production accounts or traffic.

## Blocked coverage units (not findings)

- **Opaque scene iframe navigation/referrer behavior:** Static source review found the capability path bound to listed public pack files, the expected iframe sandbox declarations, and restricted scene CSP. Browser-enforced opaque-origin, self/top navigation, Referer, cookie/storage, and post-navigation messaging behavior could not be verified. No browser or fixture was run. Follow up only in an isolated browser fixture with no external network and dummy credentials.
- **Scene-pack object-directory durability:** Static review found file data sync and state-file sync, but not a sync of newly created object-directory entries before state commit; recovery does not verify every referenced file. The effect is limited to optional public artwork. No filesystem crash/fault-injection was run. Validate on supported filesystems only inside a compliant OS-enforced sandbox. No protected Panel-state corruption is established.
