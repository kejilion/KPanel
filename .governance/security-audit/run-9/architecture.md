# KPanel scoped security audit run-9 — architecture

## Scope and source

This is a standard-profile scoped run for source `4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c` (tree `444f26132591951256e18b471e1473b430f2dbfb`), compared with full baseline `4c0694aa8e02e46145a775707b8d5a0355f7ce10`. The checker scope is the 13 paths recorded in `run-metadata.json`; the current post-run-8 diff adds changes in `internal/panel/scene_packs.go`, `internal/panel/server.go`, `internal/remotedownload/client.go`, and `internal/scenepacks/store.go`. Earlier commits reported covered by runs 6/7 and unchanged since their recorded source are carried as prior evidence, not counted as new review. The new `internal/scenepacks` boundary and its run-8 blockers/findings receive current work.

## Product and trust boundaries

KPanel is a Vue 3/TypeScript SPA served by a non-root Go `paneld`; privileged host work is delegated to a separate Agent over a structured Unix-socket API. Administrators manage host services, federated panels, terminal sessions, monitoring history, AI provider connections, and optional 3D scene packs. The protected resources include administrator sessions, cluster and terminal authority, host state, retained history, provider credentials, and scene-pack filesystem content.

The scoped code crosses these boundaries:

- **Panel HTTP/session boundary:** `internal/panel/server.go` checks Host and security-entry routing before request dispatch. Scene-pack management requires a session and non-GET actions require Origin and CSRF checks (`internal/panel/scene_packs.go`). A syntactically valid scene asset capability path is an explicit public-route exception. Deployment proxy, listener, `DataDir`, and egress policy are not established by source.
- **Scene asset capability and browser sandbox:** an authenticated scene listing/install response contains an opaque random token URL. `Store.File` binds pack ID/token/path to installed manifest state and rechecks file type, length, and digest. The handler returns only scene bytes with path-scoped CSP/CORS headers. Vue loads them in a sandboxed, opaque-origin iframe. Source shows the intended restrictions; browser enforcement of navigation, opaque-origin, and cookie behavior remains external.
- **Remote scene supply chain:** scene catalog/asset requests use fixed official or mirror HTTPS roots; path, body size, catalog shape, and per-file digest are checked before installation. `remotedownload.Client` resolves and rejects non-public addresses before dialing. The new scene-only `RejectRedirects` option rejects redirect hops before a second request; default behavior for other client callers remains unchanged. `Store.download` manages official/mirror preference and retries. A compromised trusted upstream is not cryptographically distinguished from a trusted catalog, so source integrity guarantees are bounded by the configured source trust.
- **Scene request resource boundary:** file and image handlers use a four-request active stream gate plus a bounded waiting queue sized from catalog maxima. Queue admission is released after active admission; the handler owns and releases the active slot around bounded `File`/`Image` reads and response writes. The request is anonymous at the capability route, so the review checks whether invalid tokens, cancellation, and concurrency can cross a meaningful shared-resource boundary. No runtime saturation test is permitted in this environment.
- **Cluster and terminal streams:** Panel peer identities and stream roles are checked in scoped Go dispatch code, while Noise framing/transport helpers and some remote authorization implementations live outside the scoped files. Browser terminal SSE binds a stream to the session/token hash, checks subscription ownership, and periodically revalidates the session before output. The prior run-8 SSE lead remains unresolved because product contract timing and event-order behavior are not source-settled.
- **Monitoring and AI:** monitoring history is persisted in bounded JSONL/rollup storage and decoded through bounded parsing paths. AI provider requests carry configured credentials to provider endpoints and parse bounded responses. Relevant source is unchanged from the prior covered commits/run-8 records; those records remain scoped context rather than a fresh execution claim.

## Execution constraints

Reconnaissance verified no usable OS-enforced sandbox: there is no external-network isolation, empty allowlisted environment, read-only target/toolchain mount, scratch-only process writes, or explicit resource-limit facility. WSL is installed but stopped and was not started. Windows Node lacks `O_NOFOLLOW`; the pinned validators therefore cannot be run natively, and no race-safe trusted evidence promoter was available. Consequently no target-controlled source, tests, builds, services, browsers, fixtures, or network requests are executed. Dynamic browser, filesystem crash-durability, and SSE ordering questions remain blocked; source-only checks use `artifact: null`.

## Prior-run input

Run-8 (`8a9f0ffc2b8a74008ed8fc44379d4f3e82751dba`) supplied current scene-pack coverage. The redirect egress finding is a changed-source revalidation target because this source adds `RejectRedirects`. The one-second terminal SSE revocation lead remains current work. Two prior blocked units are rechecked: browser sandbox navigation/cookie behavior and object-directory crash durability for optional artwork. The previous run-8 directory is read-only input and is not reused as this run's output.

## Companion selections

Selected domain blocks are `WEB-PROTOCOL-AND-AUTH.md`, `CLIENT-SIDE.md`, `SUPPLY-CHAIN-AND-RELEASE.md`, `RESOURCE-EXHAUSTION-AND-AVAILABILITY.md`, `DATA-ISOLATION-AND-LIFECYCLE.md`, `PROTOCOLS-RPC-AND-MESSAGING.md`, `AI-AND-LLM.md`, and `CLOUD-AND-DEPLOYMENT.md`. Their unit-level blocks and exclusions are recorded in the coverage ledger.
