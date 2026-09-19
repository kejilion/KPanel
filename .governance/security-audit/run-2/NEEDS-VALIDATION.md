# NEEDS VALIDATION

Source-only leads; no severity or runtime confirmation. Execution requires the full offline OS sandbox described by the pinned skill.

## AI redactor replacement drops bare credential filtering before model-context export

Fingerprint: `ai/runtime/redactAndLimit-bare-credential-regression`

The source diff removes the previous bare sk- and standalone Basic credential filters from AI tool results. During an authenticated administrator's AI run, lower-trust model output can select host_file_read for an otherwise allowed ordinary text file. If that file contains an unrelated service credential in either bare form, the current content filter does not match it, and source tracing reaches persistent tool context and the next request to the configured model provider. The affected boundary is host credential material versus an external provider, not administrator access to the file or another administrator session. This lead requires the credential to be outside other recognized secret assignments, headers, URLs or private-key blocks, within the retained result, and not intentionally shared with that provider. The filtering regression and propagation are source-supported; no runtime disclosure has been observed in this source-only pass.

Claimed root cause: internal/ai/runtime.go:953 delegates to redact.Text after the old marker loop containing sk- and Basic was removed. internal/redact/redact.go has no equivalent bare-token rules. Allowed file content is returned inside a content JSON value, successful Agent bodies pass through unchanged, and tool-message storage and provider serialization do not supply a second equivalent filter. Existing authentication, path restrictions, read approval policy and output limits do not filter these credential forms inside an allowed small text file.

### Source trace

1. `internal/panel/ai_tools.go:231` (panelAITools.Execute host_file_read, entrypoint): Model-selected arguments reach the fixed file-read route after strict argument validation. aiFileReadable at line 241 rejects sensitive paths and filenames but permits ordinary files such as /opt/demo/config.txt; line 268 calls the Agent /v1/files/text endpoint with that path.
2. `internal/agent/files.go:337` (Server.fileText, propagation): File Manager safely opens the allowed regular file. The handler enforces editable text, a 64 KiB maximum, valid UTF-8 and no NUL, then returns unfiltered Content in fileTextResult at lines 356-358.
3. `internal/panel/ai_tools.go:517` (panelAITools.finish, propagation): After audit and error-status handling, the successful Agent response body is returned verbatim to the runtime.
4. `internal/ai/runtime.go:497` (NativeRuntime.executeTool, propagation): redactAndLimit applies the central redactor and the 64 KiB result cap. The result becomes ResultPreview and is saved as RoleTool content at line 502, wrapped with an untrusted-data label that does not remove secrets.
5. `internal/ai/runtime.go:256` (NativeRuntime.Run and completionMessages, propagation): The next loop reads same-session context and constructs CompletionRequest. completionMessages line 433 copies toolMessage.Content to a tool ChatMessage with its tool-call ID; streamWithRetry invokes the model client at line 467.
6. `internal/ai/client.go:228` (HTTPModelClient.streamOpenAIResponsesAttempt, sink): The Responses path serializes message.Content into function_call_output, calls do at line 257, and do marshals the payload and submits it to the configured provider at line 703 without an additional credential-content filter.

### Verified source evidence

- `internal/ai/runtime.go:952`: Current redactAndLimit uses redact.Text. Independently read diff from 6340e078 to exact source 23cbb9979ce0df84801b9fe614bd1b00d7ad8a99 removes a case-insensitive marker loop containing sk- and Basic; this is a specific regression, not an expectation that every arbitrary secret can be recognized.
- `internal/redact/redact.go:13`: Read the full pattern set and redactLine at line 155: recognized cases include labeled JSON/text/flag secrets, Authorization and Cookie headers, Bearer tokens, URL credentials and sensitive queries, plus private-key blocks. Neither bare sk- nor standalone Basic has a rule. A non-secret-named outer JSON content field adds no matching label.
- `internal/panel/ai_tools.go:702`: The strongest AI input control normalizes absolute paths and denies sensitive roots/names including .env, private keys, credentials files and KPanel paths. It does not reject ordinary .txt configuration files merely because their contents hold a credential.
- `internal/panel/ai_tools.go:66`: RequiresApproval returns false for recognized ReadOnly tools at lines 71-74; host_file_read is ReadOnly at line 172. Runtime manual approval applies only to non-read-only tools at runtime.go:373. Read-tool approval is therefore not a compensating secret-export check.
- `internal/panel/ai_handlers.go:35`: The AI API requires an authenticated session and Origin/CSRF for mutations. The lead assumes an administrator has initiated a legitimate AI run; it does not claim unauthenticated access to AI or host files.
- `internal/agent/server.go:279`: File Manager uses host root with explicit protected directories; ServeHTTP line 292 requires Agent authorization. This is Panel-delegated host access, not a bypass of the Agent token or exposure of protected KPanel storage.
- `internal/filemanager/manager.go:1443`: resolveExisting rejects protected paths, path escape, internal temporary components and symlink components. Open at line 305 also requires a regular file and matching opened identity. viewerSupport at line 1657 allows small .txt files. These controls can all hold while an allowed file contains a bare credential.
- `internal/ai/store.go:684`: AddMessage persists message.Content without a second redaction pass. context.go:154 reads content using the same session ID, and runtime.go:433 forwards tool content. The claim concerns external-provider disclosure, not cross-session storage access.
- `docs/ai-workspace.md:52`: The documented data policy places tool arguments and results in a common bounded redaction pipeline. The administrator's ordinary SSH/File Manager ability to inspect the original file does not itself authorize revealing an unrelated credential to a model provider.

### Decisive blockers

- A bounded native runtime demonstration of the dummy credential surviving both stored tool content and the captured next provider payload has not been performed. This audit forbids target execution because OS-enforced no-network isolation, read-only target/tool mounts, scratch-only writes and explicit resource/wall-clock limits are not established. Source already resolves the missing-pattern and propagation questions; a real provider's model behavior is not required to test them.

### local resolution plan

After establishing the required offline OS sandbox with an isolated network namespace, no external interfaces or routes, read-only source and tools, scratch-only writes and explicit resource and wall-clock limits, use a scratch File Manager root containing /opt/demo/config.txt, a regular UTF-8 file below 1 KiB with bare dummy values sk-example-dummy-secret and Basic ZHVtbXk6ZHVtbXk= on separate lines. Use an in-process Agent handler/recorder and panel host-operation adapter, a fresh same-user session, and a deterministic fake ModelClient response selecting host_file_read with {"path":"/opt/demo/config.txt"}; the user request should ask for configuration diagnosis without authorizing credential disclosure. Permit only that read and one follow-up completion. Capture stored RoleTool content and the next CompletionRequest. To observe native Responses serialization, pass that captured request to NewHTTPModelClient().Stream with ProtocolOpenAICompatible, APIMode OpenAIResponses, EndpointScope EndpointPrivate, a dummy API key and a httptest server bound only to loopback inside the same isolated namespace. The mock handler should capture one bounded request body and return a minimal response.completed SSE event; it must make no outgoing connection. This uses the existing native HTTP path because HTTPModelClient constructs its own transport and has no injectable RoundTripper. Check exact dummy-value presence in storage, the next CompletionRequest and the captured function_call_output, then stop. Include password=labelled-dummy and Authorization: Basic labelled controls and a rejected .env path, verifying that applicable protections still act. Use a fresh short context, valid tool-call IDs, ordinary manual approval mode and a step budget allowing the follow-up. No live provider, host secret, shell tool, external network or production state is needed. If isolated loopback is not available, omit the serialization execution and keep that part unobserved rather than relaxing isolation.

## Replayed audit migration may make the next event expire retained history prematurely

Fingerprint: `audit-migration-autoincrement-gap-retention-loss`

The store explicitly promises that restarting an interrupted audit migration does not lose records. Its database import commits separately from removal of the legacy JSON array. Replaying the migration uses INSERT OR IGNORE in an AUTOINCREMENT table, whereas later retention measures sequence distance rather than the number of retained events. If ignored inserts advance the persisted sequence in the pinned driver, a subsequent ordinary event can delete history that should remain within the 10000-event cap; after one replay at capacity the candidate result is loss of all 10000 previous events. An unauthenticated requester able to reach the login handler and pass its existing request controls can supply that subsequent event through an ordinarily permitted failed login. The attacker is not shown to cause or detect the interrupted migration. This is a conditional audit-integrity candidate, not a demonstrated remote erasure vulnerability or an observed database result.

Claimed root cause: Migration deduplicates rows by ID without reconciling possible AUTOINCREMENT gaps, while retention assumes max(seq) minus maxEntries identifies precisely the rows beyond the retention count.

### Source trace

1. `internal/panel/server.go:660` (Server.handleLogin, entrypoint): After a replayed migration has completed, a lower-trust login requester whose request passes Host, optional security entrance and Origin checks can cause an invalid-credential audit at lines 681-683; rate-limited logins take a separate branch. This path does not give the requester control over migration or the database.
2. `internal/panel/server.go:889` (Server.auditAuthFailure, propagation): A failed authentication that passes the five-second global and one-minute per action/source audit throttles invokes the common audit writer.
3. `internal/panel/server.go:1526` (Server.audit, propagation): The server creates a fresh audit event and passes MaxAuditEntries to Store.AppendAudit.
4. `internal/store/store.go:666` (Store.AppendAudit, propagation): The common writer invokes auditLog.append with skipKnownIDs=false; the retained database sequence is reused after the earlier migration.
5. `internal/store/audit_log.go:183` (auditLog.append, propagation): The ordinary insertion omits seq, so SQLite allocates it according to the AUTOINCREMENT table state.
6. `internal/store/audit_log.go:188` (auditLog.append, sink): The same transaction deletes rows with seq <= max(seq)-maxEntries; if migration left a sufficiently large allocation gap, this can delete still-retainable audit events.

### Verified source evidence

- `internal/store/store.go:244`: The migration contract explicitly promises repeat without duplicate or lost records; Open invokes it at line 235.
- `internal/store/store.go:252`: The database import completes before the JSON audit array is cleared and persisted at lines 255-257. An interruption between these durable operations leaves a legitimate replay state.
- `internal/store/audit_log.go:88`: audit_events uses seq INTEGER PRIMARY KEY AUTOINCREMENT and a separate UNIQUE event ID.
- `internal/store/audit_log.go:173`: Migration uses INSERT OR IGNORE, whereas normal writes explicitly reject already-known IDs before insertion at lines 175-180.
- `internal/store/audit_log.go:188`: Retention uses max(seq)-maxEntries, not the count or ordered offset of existing rows; append and deletion commit together at line 192.
- `internal/store/audit_log.go:140`: The stated writer contract trims the oldest events beyond maxEntries, supporting a count-based retention invariant rather than expiry by skipped sequence allocations.
- `internal/store/audit_log_test.go:79`: The interrupted-migration test creates three legacy records and checks the immediate duplicate-free listing at lines 101-103; it never appends a fresh event after replay or exercises replay at the retention cap. The separate retention test at line 107 has no migration replay. No tests were executed in this verification.
- `internal/panel/server.go:868`: Anonymous failure auditing retains global and source-specific throttles; the conditional claim requires one permitted append, not bypass of either throttle.
- `internal/panel/server.go:233`: The server applies security entrance before API routing. The entrance guard at lines 789-823 can prevent an unknown anonymous client reaching login when enabled; the candidate does not assume this guard can be bypassed.
- `internal/panel/server.go:1303`: checkOrigin rejects mismatched request origins, and checkHost at line 1339 applies the configured host policy. A reachable login requester must satisfy these controls.
- `internal/store/audit_log.go:53`: The database is protected with 0600 permissions, one SQL connection, WAL/FULL synchronization and file-identity checks. Store.Open additionally holds a process lock. These controls constrain direct tampering and concurrent owners but do not change the sequence-based pruning expression.
- `go.mod:13`: The target pins modernc.org/sqlite v1.55.0. This review did not execute the driver or collect a runtime sequence/retention observation.

### Decisive blockers

- No bounded target-native observation establishes the pinned driver's persisted AUTOINCREMENT state after duplicate migration or the exact retained IDs after the next event. The required OS-enforced offline sandbox, read-only target/tools, scratch-only writes, resource limits and trusted artifact promotion are not established, so target and fixture execution was prohibited.
- Source establishes a possible interruption window, not that any deployed store is in the replayed state or that an attacker can create it. Anonymous triggering is conditional on reaching login through the existing entrance and request controls; no deployed exposure or actual audit-history loss was observed.

### local resolution plan

In an approved offline, resource-bounded sandbox with dummy data and trusted artifact promotion, use the pinned Go SQLite driver and auditLog.append with cap 3: append IDs a,b,c with skipKnownIDs=true, close/reopen the log, replay a,b,c, then append fresh d with skipKnownIDs=false. Record sqlite_sequence, row sequences and retained IDs after each stage; the count-based expectation after d is d,c,b. Stop at this bounded difference. If it demonstrates the gap, verify the actual Store.Open migration path using one disposable legacy JSON fixture of at most MaxAuditEntries dummy events, simulating the already-committed database plus uncleared JSON state without crashing a live process, then append one dummy event. Verify IDs and count. Do not contact a deployment or use real audit records.

### deployment resolution plan

For applicability only, the owner may inspect existing migration/startup error records and a read-only snapshot for coexistence of migrated database IDs with the legacy JSON audit array, and inspect whether the optional security entrance restricts anonymous access to login. Do not interrupt startup, provoke authentication failures, mutate audit files, or send audit traffic. This observation cannot replace the bounded local sequence/retention check.

## Unrelated login failures can evict an account's unexpired rate-limit history

Fingerprint: `auth-login-global-history-evicts-active-lockouts`

Source shows that the new global 4096-record cap discards the oldest login history even while its configured failure window remains active. An unauthenticated requester who can reach the login endpoint, satisfy any enabled security-entrance gate, and supply sufficient distinct real source addresses could use rejected logins for unrelated usernames to remove a limited account's recorded failures. Subsequent password verification for that account would then be admitted within the original window. At defaults the intended account budget is 50 failures per 15 minutes; 2048 completed unrelated failures append 4096 records, provided the requester stays below the per-IP and per-account admission thresholds. This is a source-grounded verification-budget reset lead, not an observed credential compromise or an MFA bypass. No runtime reproduction or deployment applicability has been established.

Claimed root cause: RecordLoginAttempts bounds all keys using a shared FIFO without preserving unexpired per-key failure counts. FailedLoginCount and reserveLogin reconstruct the account budget solely from retained records plus transient in-flight reservations; neither holds separate lockout state after history eviction.

### Source trace

1. `internal/panel/server.go:673` (handleLogin, entrypoint): After the Host/security-entrance gates, Origin validation and JSON decoding, the public login route passes supplied credentials and the source-derived remote IP to Service.Login.
2. `internal/auth/service.go:294` (Login, propagation): A nonexistent username or invalid password records a failed attempt after per-IP/account reservation and bounded hash admission.
3. `internal/auth/service.go:778` (recordLoginAttempt, propagation): Every completed failed login appends both an IP-key record and an account-key record to the same store history, including failures for nonexistent usernames.
4. `internal/store/store.go:695` (RecordLoginAttempts, propagation): When the appended history exceeds 4096 records, oldest records are discarded without preserving active per-key windows; a persistence failure rolls the mutation back.
5. `internal/store/store.go:706` (FailedLoginCount, propagation): The failure count uses only retained records newer than the window start or latest retained success; removed account failures no longer contribute.
6. `internal/auth/service.go:532` (reserveLogin, propagation): A later attempt passes account admission once retained failures plus pending reservations fall below the account threshold, provided its source IP also passes.
7. `internal/auth/service.go:293` (Login, sink): Password verification can be reached again inside the original failure window. Valid credentials and any configured second factor remain required for a session.

### Verified source evidence

- `internal/auth/service.go:122`: Defaults are a 15-minute login window and five failures per IP; accountFailureLimitMultiplier at line 44 is ten.
- `internal/store/store.go:675`: maxLoginAttempts is 4096. RecordLoginAttempts appends records, keeps the newest 4096, and rolls back on persistence failure at lines 681-703.
- `internal/auth/service.go:535`: Both rate limits derive from FailedLoginCount plus pending reservations. releaseLogin at lines 547-556 removes pending counts after each call; they are not persistent lockout state.
- `internal/auth/service.go:273`: Hash slots bound simultaneous verification; rejected slot admissions append no history. The default is one slot (128-131), so record turnover requires completed verifications, not arbitrary parallel requests.
- `internal/auth/service.go:821`: loginKeys trims and lowercases identity keys and limits their lengths. The lead requires distinct normalized usernames and source addresses rather than casing variants.
- `internal/panel/server.go:1752`: remoteIP trusts forwarding headers only for configured trusted proxy peers; otherwise it uses the socket address. No untrusted forwarding-header bypass is asserted.
- `internal/panel/server.go:233`: ServeHTTP applies handleSecurityEntrance before routing. At lines 789-823 an enabled entrance hides login unless the caller has its cookie or an existing valid session; a reachable login interface is therefore a prerequisite.
- `internal/panel/server.go:1303`: checkOrigin validates the expected origin; this is not an attacker authentication check for a direct client that already reaches the known login endpoint.
- `internal/auth/service_test.go:135`: TestLoginAppliesHigherDistributedAccountLimit expresses the intended account limit across source IPs. It does not exercise global history eviction; tests were read, not run.
- `internal/store/store_test.go:1435`: TestLoginAttemptsKeepOnlyTheNewestEntries explicitly expects the oldest key's retained count to disappear, but does not test preservation of an actively limited account. It was read, not run.

### Decisive blockers

- The bounded native Service.Login reproduction has not run: an OS-enforced no-network, read-only-target/tools, scratch-only, resource- and wall-clock-limited execution sandbox and trusted artifact promotion are not established. Source establishes the eviction mechanism but no observed verification-budget reset is claimed.
- Deployment applicability remains unobserved: the caller must reach login despite any security-entrance or ingress restriction, provide sufficient distinct legitimate source IPs, and complete enough successful failure-record writes before the configured LoginWindow expires. The one-slot default hasher and any ingress controls constrain that timing.

### local resolution plan

In an approved offline bounded sandbox, use the native auth Service with one dummy administrator, a fixed clock, a cheap deterministic PasswordHasher with a Verify-call counter, temporary store paths, and default failure thresholds. Complete 50 wrong-password calls for that administrator across permitted dummy IPs, then assert that the next call from a fresh dummy IP returns ErrRateLimited without invoking Verify. Without advancing the clock, complete no more than 2048 sequential wrong-password calls for distinct nonexistent dummy usernames and distinct dummy IP strings, checking that each is ErrInvalidCredentials rather than a slot rejection. Make one additional wrong-password call for the limited administrator from a fresh dummy IP and check whether Verify is invoked and ErrInvalidCredentials replaces ErrRateLimited. Cap the harness at 2100 total Login calls, stop at this result, and use no real credentials, real hasher load, sockets or deployment traffic. This isolates the native accounting invariant; it does not establish realistic network capacity or timing.

### deployment resolution plan

The owner can inspect, without sending audit traffic, the configured login window and failure thresholds, security-entrance state, trusted-proxy/client-IP handling and existing ingress rate controls. Use already available operational timing and capacity observations, if any, to assess whether completed failure writes from sufficiently diverse real source addresses can turn over the history inside one window. Keep applicability unresolved if that evidence is unavailable.

## Imported root path lengths bypass the persistent backup record read budget

Fingerprint: `backup.jobs.record-root-budget-ignores-path-bytes`

A lower-trust archive author controls root metadata inside an otherwise valid password-protected backup. An administrator must import the archive; this is not an unauthenticated request or a claim about intended administrator shell power. Source independently shows that automatic host inspection aggregates roots from separately bounded module manifests and persists them using a count-only budget. Long canonical apps/docker paths can therefore exceed the persistent record reader limit without exceeding either module manifest limit. If destination Inventory succeeds, the write succeeds, and the record remains present until a later default Agent initialization, source predicts initialization failure at the 4 MiB record read limit. No archive fixture, persisted byte length, manager reopen or Agent failure has been observed in execution.

Claimed root cause: RecordRootsExceedReadBudget estimates 512 + 96*rootCount instead of serialized RootRef or full-record bytes. Root.Path has no byte-length bound; apps and docker payload manifests each have a separate 4 MiB allowance. Combining their roots can exceed the shared 4 MiB record limit, while Manager.Update/saveLocked and WriteJSON impose no serialized-byte cap. OpenManager rejects any oversized retained record before starting maintenance, and default Agent backup initialization propagates this error.

### Source trace

1. `internal/panel/backup_handlers.go:408` (authenticated backup import, entrypoint): An administrator supplies an external .kpb and its password. Session, origin and CSRF checks protect invocation but do not authenticate the archive author's root metadata.
2. `internal/panel/backup_handlers.go:547` (backupImport host inspection, propagation): After uploading host module payloads, the import worker invokes Agent inspect automatically; no restore confirmation is needed.
3. `internal/hostbackup/archive.go:247` (Engine.ReadPayload, propagation): Each module receives a separate 4 MiB manifest budget. validatePayload checks canonical paths, derived IDs and module/mount bindings, but does not cap path length; extracted filenames use data/<ID>.
4. `internal/hostbackup/service.go:228` (Service inspect worker, propagation): Roots from all selected module payloads are combined into one []RootRef.
5. `internal/backup/jobs.go:58` (RecordRootsExceedReadBudget, propagation): The strongest added budget gate computes 512 + 96*len(roots), ignoring actual Path bytes; two roots yield an estimate of 704 regardless of string lengths.
6. `internal/hostbackup/service.go:242` (inspect persistent update, propagation): The accepted root list is written into the record through Jobs.Update and saveLocked without checking serialized record size.
7. `internal/backup/jobs.go:94` (OpenManager restart read, propagation): A retained record over maxRecordBytes=4 MiB returns ErrInvalid rather than opening the manager.
8. `internal/agent/server.go:218` (Agent server initialization, sink): When Config.Backups is nil and StateDir is nonempty, hostbackup.NewService propagates an OpenManager read failure and Agent construction returns initialize backup jobs error. This source-visible failure path affects Agent initialization beyond the individual import; no live failure is claimed.

### Verified source evidence

- `internal/hostbackup/archive.go:247`: Per-module manifest cap is 4 MiB; roots in different payloads have independent budgets.
- `internal/hostbackup/archive.go:393`: Root checks enforce canonical absolute form, module, exclusion and derived ID, but no string length. :305 extracts to the short ID path, so virtual Path need not exist during inspect.
- `internal/hostbackup/engine.go:185`: Positive scope control allows an arbitrarily long canonical /home/... apps root; docker roots require a matching declared bind source. This control is included in the proposed local fixture, not ignored.
- `internal/hostbackup/native.go:178`: Missing HostConfig mounts are reconstructed from c.Mounts; a valid declared docker bind does not require adding a third long-path copy to the stored manifest.
- `internal/hostbackup/service.go:235`: Only combined budget gate is RecordRootsExceedReadBudget, followed by independent destination Inventory and storage of Roots at :242-245. Destination root membership is checked only on Restore, not inspect.
- `internal/backup/jobs.go:58`: Fixed 96-byte multiplier cannot bound serialized variable-length paths; even maximum allowed root counts cannot trip it.
- `internal/backup/jobs.go:165`: saveLocked writes a Record without comparing marshaled length to maxRecordBytes.
- `internal/backup/files.go:136`: WriteJSON marshals then AtomicFile writes all bytes; no read-budget enforcement.
- `internal/backup/jobs.go:94`: OpenManager rejects an oversized record before starting its maintenance/sweep worker; cleanup cannot repair it during startup.
- `internal/hostbackup/service.go:42`: NewService returns the OpenManager error, propagated by Agent initialization at server.go:218-220.
- `internal/backup/archive.go:177`: Outer backup framing checks a small manifest, selected modules, part sizes, SHA256, authenticated encryption frames and trailing data. The author can create a self-consistent archive and password; these integrity checks do not cap inner root path bytes.
- `internal/hostbackup/engine.go:102`: Engine.excluded uses string/path normalization and overlap with protected/state paths, without statting the imported virtual root; a long nonoverlapping canonical virtual root is not rejected by filesystem NAME_MAX here.
- `internal/hostbackup/restore.go:134`: Exact destination root membership is enforced in Restore, after inspect has already written the record; it does not protect inspect persistence.
- `internal/hostbackup/protection.go:13`: Runtime protection rejects imported Panel/Agent identity, protected mounts and identity-bearing configuration. A neutral dummy docker container with /srv/... bind source and /data destination avoids these controls without claiming they are absent.
- `internal/backup/jobs.go:221`: Abort updates status and stage but retains Roots. Sweep can eventually delete old records, but OpenManager validates all record sizes before its maintenance worker starts. Impact requires the record still to be present at reopen.
- `internal/backup/archive_test.go:120`: The legacy 32 KiB regression uses 700 ordinary short roots and checks reopen; it does not exercise independently bounded manifests whose combined variable-length paths exceed 4 MiB. This test was read, not run.
- `internal/backup/space_unix.go:15`: RequireSpace reserves an additional 64 MiB beyond estimated writes. The bounded fixture therefore needs a scratch filesystem whose reported free capacity clears that gate; a 32 MiB tmpfs would reject upload or inspect before the candidate path.

### Decisive blockers

- Decisive execution is prohibited in this run because OS-enforced no-network, read-only target/tools, scratch-only writes, an empty allowlisted environment and resource/wall-clock limits are not jointly established. Source settles the budget mismatch and error propagation, but acceptance of the complete bounded fixture, actual persisted record length and isolated OpenManager reopen outcome remain unobserved. No target-produced artifact was created or promoted.

### local resolution plan

After the required sandbox and trusted parent-side bounded artifact promotion are established, use dummy state under scratch, Engine.Root mapped to an empty scratch host tree, and an in-process mock Docker transport returning healthy empty /info and /containers/json?all=1 responses. Do not open a real Docker socket. Within 256 MiB memory, 128 MiB scratch disk and a 60-second timeout, first verify that the isolated scratch filesystem actually reports at least the proposed write size plus 64 MiB of free space, as required by backup.RequireSpace; do not bypass that check. Then construct two gzip/tar payloads: one apps root /home/ followed by 3 MiB of ASCII a; one docker root /srv/ followed by 1.5 MiB of ASCII b. For each root use the first 16 SHA256 bytes as lowercase hex ID, Directory=true, nonnegative bounded byte/entry values and one empty data/<ID> directory with current dummy ownership. The docker payload must declare one neutral container named audit-dummy, module docker, a valid sha256 image ID, config Image set to a nonempty dummy reference, and exactly one bind Mount with Source equal to its root and Destination=/data; leave HostConfig mounts absent so nativeMountConfig reconstructs them. Set Version=1 and the proper module in each payload. Assert each serialized manifest remains below 4 MiB. Use the normal backup container writer with a dummy password and then ReadContext to check outer framing, then drive a scratch-only Service import reservation/upload/inspect through an in-process handler. Poll its Manager directly for the terminal record, avoiding response-size assumptions. Capture only booleans, numeric lengths and error identity, never full paths. Confirm both payloads were accepted, record.json exceeds 4 MiB, and, after closing the isolated service, reopening its Manager returns ErrInvalid. Stop there: no Restore, production restart, network, real data or deployment traffic. Failure of any prerequisite must refute or narrow the candidate rather than be bypassed.
