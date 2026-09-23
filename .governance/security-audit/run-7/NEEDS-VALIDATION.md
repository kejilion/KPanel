# run-7 — NEEDS-VALIDATION

本文件列出 15 条源级候选线索。每一条均保留 `needs_validation`；没有严重度，不是 confirmed vulnerability。当前审计没有执行目标代码或部署检查。待补齐的结论只允许在批准的 OS 强制隔离环境或通过 owner 对现有非生产配置/记录的只读检查取得。

## An ID-only revoke can leave a colliding V2 controller active

**Fingerprint:** `cluster.file-peer.cross-protocol-controller-delete-shadow`

**Affected boundary:** internal/cluster/file_peer_v2.go#pending/active grant and controller route persistence

**Surface / subsystem:** internal/cluster/file_peer_service_v2.go#mutual file-transfer grant linking / internal/cluster/file-peer-v2

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** A holder of a valid, unexpired V2 pairing code can submit a valid ControllerID matching an existing legacy controller. The separate stores check IDs independently. The Panel presents both protocols through the same Controller shape, and its authenticated delete route passes only the bare ID. Service.DeleteController returns success after deleting a matching legacy record, before revoking the V2 controller or deleting its file-peer routes. The remaining V2 record retains its key and can still pass the active-only V2 authorization path. The UI also removes every displayed row with that ID after one successful response. Deletion still requires the normal Panel session, Origin, CSRF, and audit gates; this is not an unauthenticated delete bypass. File access through a retained route remains conditional on the route and downstream authorization checks.

**源码追踪：**
- `internal/cluster/service_v2.go:867` (entrypoint) — A caller holding a valid V2 pairing code submits an envelope containing its chosen ControllerID.
- `internal/cluster/service_v2.go:912` (propagation) — The handler copies envelope.ControllerID into the V2 controller record.
- `internal/cluster/store_v2.go:636` (propagation) — After checking duplicates within the V2 collection, the store appends the controller there; it does not consult the legacy store.
- `internal/panel/cluster.go:479` (propagation) — The authenticated management route calls DeleteController with the bare ID from the URL.
- `internal/cluster/service.go:764` (propagation) — A successful legacy deletion returns immediately, skipping the subsequent V2 revocation and file-peer cleanup.
- `internal/cluster/service_v2.go:1043` (sink) — A later V2 request looks up the same ID in the V2 store; the active-only summary path can still authenticate the retained record using its stored public key.

**已核对的源码证据：**
- `internal/cluster/protocol_v2.go:518` — V2 envelope validation requires a syntactically valid ControllerID but does not bind it to a globally allocated ID.
- `internal/cluster/service.go:795` — Legacy AcceptPair also accepts a caller-supplied syntactically valid ControllerID.
- `internal/cluster/store.go:336` — Legacy duplicate detection scans only the legacy store's controller collection.
- `internal/cluster/store_v2.go:619` — V2 duplicate detection scans only the V2 store's controller collection.
- `internal/cluster/service.go:737` — Controllers concatenates legacy records and active V2 records into one management list.
- `internal/cluster/types.go:224` — The shared public Controller shape has no protocol discriminator.
- `internal/panel/cluster.go:466` — The delete handler requires the normal cluster mutation session and Origin/CSRF checks before processing the request.
- `internal/panel/cluster.go:470` — The delete route extracts one bare ID from the URL and provides no protocol selector.
- `internal/cluster/service.go:764` — A successful legacy store deletion returns nil before the V2 lookup and revocation code.
- `internal/cluster/service.go:774` — V2 revocation occurs only after the legacy deletion returns ErrNotFound.
- `internal/cluster/service_v2.go:978` — The V2 summary handler admits only active V2 controllers.
- `internal/cluster/service_v2.go:1054` — openControllerV2 checks the request's Noise public key against the V2 record, so the retained V2 credential remains the relevant authenticator.
- `internal/cluster/file_peer_v2.go:454` — File-peer routes for a controller are deleted only when the V2 deletion cleanup is reached.
- `internal/cluster/file_transfer_v2.go:368` — The reverse-file path can consult an active route only when there is no direct Host for that peer node.
- `internal/cluster/file_transfer_v2.go:372` — Before using a reverse route, the code checks the V2 controller's active state, file scope, transaction, and fingerprint.
- `web/src/views/ClusterView.vue:1015` — The UI sends the controller ID without a protocol discriminator.
- `web/src/views/ClusterView.vue:1016` — After success, the UI filters out every controller row with that ID, including a colliding V2 row.

**精确 blocker：**
- The required OS-enforced sandbox and evidence-promotion path are unavailable, so the same-ID two-store lifecycle and subsequent V2 request could not be observed; target code, tests, builds, fixtures, browser actions, and network calls were not run.
- The downstream file-read consequence additionally requires an unexpired matching route, an active file-scoped controller, no direct Host for the peer node, and valid remote linked-file authorization; source confirms these guards but not the resulting call.

**有界本地下一步：**
In an approved offline, scratch-only sandbox, use dummy keys and separate pairing codes to create one legacy controller and one active V2 controller with the same fixed valid 32-hex ID. Add an unexpired file-peer route with matching V2 transaction and fingerprint for a peer node with no direct V2 Host. Call Service.DeleteController once and record only its result, whether the legacy record is gone, whether the V2 record remains active, and whether ActiveRoute remains. Then send one harmless correctly authenticated active-only V2 request using the test V2 key. If checking the file consequence, make one OpenRemoteFileV2 call against a fake remoteV2LinkedFileAPI and record whether the fake is reached. Stop after these observations; use no external network or real credentials. A safe result is cross-store collision rejection or deletion that revokes the V2 record and removes the route.

**安全 owner-observed 部署检查：**
An authorized owner may passively inspect an existing non-production state and deployed build revision for a legacy controller and an active V2 controller with the same ID. If that pre-existing collision and records from an already-completed revoke are present, use read-only diagnostics to observe whether the legacy record is gone while the V2 record remains active and any associated route remains. Do not create pairings, issue revokes, or send audit requests. If no matching pre-existing state and records exist, report this deployment check as inconclusive and use the bounded offline local fixture.

## V2 控制器撤销后，在途监控历史流仍可能继续发送

**Fingerprint:** `cluster.history.inflight-revocation`

**Affected boundary:** internal/cluster/history_transport.go#cancel or invalidate in-flight response

**Surface / subsystem:** Panel v2 federation history request / internal/cluster

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** Panel 在 /api/v2/federation/monitoring/history admission 时验证 V2 请求、控制器状态和密钥、历史权限及查询，再将本地 Agent 响应复制到加密 HTTP 流。该处理使用请求 context 和两分钟超时；V2 撤销会持久化 revoked 状态并关闭 fileStreamHub 跟踪的连接和请求 lease，但历史 handler 未注册到该 hub，也未在复制期间重查控制器状态。因此，源码显示撤销本身不会取消这条已 admission 的历史响应；本轮未观察到并发撤销后实际写出的数据帧，且仓库代码未说明已 admission 请求是否必须立即终止。历史数据包含主机和容器指标、容器名称及镜像名称，响应正文限制为 16 MiB。

**源码追踪：**
- `internal/panel/server.go:228` (entrypoint) — Panel HTTP 请求入口，随后将 API 请求交给 API 分派。
- `internal/panel/server.go:302` (propagation) — 联邦 V2 请求被分派到 handleFederationV2。
- `internal/panel/cluster.go:573` (propagation) — HistoryV2Path 被分派到 handleFederationHistoryV2。
- `internal/cluster/history_transport.go:34` (propagation) — admission 打开 active 控制器，随后检查历史权限和查询，并取得每控制器历史流 limiter。
- `internal/panel/monitoring_federation.go:59` (sink) — 本地 Agent 响应体被复制到加密联邦流；复制期间没有重查控制器状态。

**已核对的源码证据：**
- `internal/cluster/history_transport.go:28` — V2 历史请求在 admission 时先经过按来源的速率限制。
- `internal/cluster/history_transport.go:31` — 请求随后调用 validateV2Request 检查联邦请求 envelope。
- `internal/cluster/service_v2.go:1070` — V2 请求验证限制方法和路径、校验 envelope 及目标节点，并检查请求时间戳。
- `internal/cluster/service_v2.go:1030` — openControllerV2 从存储读取控制器，并在入口只接受调用方允许的状态；历史调用只允许 active 状态。
- `internal/cluster/service_v2.go:1043` — Noise 握手对端公钥与控制器存储的公钥比较；这是 admission 检查。
- `internal/cluster/history_transport.go:35` — 历史授权检查控制器 scope 是否允许历史查询。
- `internal/cluster/history_transport.go:39` — 历史 payload 解码后必须通过 monitoring.Query.Validate。
- `internal/cluster/service_v2.go:1053` — openControllerV2 在入口应用控制器请求速率限制。
- `internal/cluster/service_v2.go:1056` — 请求 ID 在入口经过 replay 检查。
- `internal/cluster/history_transport.go:42` — admission 为控制器取得历史流 limiter；该回调仅在授权对象关闭时释放。
- `internal/cluster/service.go:286` — 历史流 limiter 配置为全局最多两个活动流、每个 peer 最多一个。
- `internal/cluster/file_transfer_v2.go:81` — limiter 依据活动总数和每 peer 活动数作限制，跟踪的是计数。
- `internal/cluster/history_transport.go:46` — HistoryAuthorization 包含请求 envelope、handshake 和 limiter release；没有请求 context 或 revocation generation。
- `internal/panel/monitoring_federation.go:30` — 历史 handler 从 HTTP 请求 context 派生两分钟超时 context。
- `internal/panel/monitoring_federation.go:39` — 请求 context 结束时关闭本地 Agent 响应体；回调没有取消远端 HTTP 响应 writer。
- `internal/cluster/file_transfer_v2.go:207` — FederationFileAuthorization.Close 只调用 limiter release 回调。
- `internal/panel/monitoring_federation.go:51` — 历史 handler 创建绑定请求 context 的空闲响应 writer，设置 30 秒写空闲超时。
- `internal/httpstream/idle.go:72` — 响应 writer 在每次 Write 前检查 context，并为写操作设置 deadline。
- `internal/panel/monitoring_federation.go:59` — handler 直接复制本地响应体并结束加密流；该路径没有控制器状态重查或 fileStreamHub 注册。
- `internal/cluster/history.go:21` — 历史请求总时限为两分钟。
- `internal/monitoring/query.go:16` — 历史响应正文上限定义为 16 MiB。
- `internal/monitoring/response.go:90` — 复制使用受 MaxHistoryResponseBytes 限制的 LimitedReader。
- `internal/cluster/service_v2.go:1007` — V2 revoke 路径调用 RevokeController 持久化撤销状态。
- `internal/cluster/store_v2.go:451` — RevokeController 将控制器状态设为 revoked 并保存撤销时间。
- `internal/cluster/store_v2.go:455` — 撤销状态通过 persistValidatedLocked 持久化；此函数没有取消历史 handler 的回调。
- `internal/cluster/service_v2.go:1014` — V2 revoke 随后调用 fileStreamHub.closePeer。
- `internal/cluster/file_stream_transport.go:56` — closePeer 关闭 hub 中登记且 owner 匹配的连接。
- `internal/cluster/file_stream_transport.go:61` — closePeer 也停止 hub 中登记且 owner 匹配的 request lease；历史 handler 路径没有取得这类 lease。
- `internal/contract/monitoring.go:111` — MonitoringHistory 含主机指标字段；line 112 含容器序列字段。
- `internal/contract/monitoring.go:55` — 容器序列包含 name 和 image 字段。

**精确 blocker：**
- 本轮限定为 source-only，未运行 handler fixture；实际并发撤销后是否有 DATA 或 END 数据帧被写出，尚无本地运行证据。源码显示撤销不取消该 handler，但不能据此声称已观察到具体帧结果。
- 审阅到的仓库代码没有说明已 admission 的历史请求在控制器撤销后允许完成还是必须立即终止；需要确认适用的产品或协议策略。

**有界本地下一步：**
在具备合适隔离环境后，使用一个合成 V2 控制器、一个 synthetic key 和一个仅含单条非敏感指标的 fake Agent body，通过实际 history handler 建立单个请求；让 Agent body 在首个数据块前阻塞，执行本地 V2 revoke，再释放该块。用内存 recording ResponseWriter 检查撤销完成后 handler 是否写出 DATA/END，并确认同一控制器的新历史请求被拒绝、原 limiter slot 最终释放。限制为一个控制器、一个在途请求和一个小数据块，不访问部署或真实数据。

**安全 owner-observed 部署检查：**
请协议/系统 owner 查验当前适用的控制器撤销策略或规范版本，明确已通过 HistoryV2 admission 的响应在撤销后是否可完成，并记录可引用的策略或配置来源；只核对策略，不向部署发送审计流量。

## V2 paired-Panel relay can turn an Agent export failure into clean EOF

**Fingerprint:** `cluster.panel-file-relay.agent-read-error-clean-end`

**Affected boundary:** internal/cluster/panel_file_relay.go#body terminal result and session cleanup

**Surface / subsystem:** V2 paired-Panel file relay and V3 reusable file socket / internal/cluster

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码显示一条具体的错误传播缺口：浏览器请求进入目标 Panel 的 V2 polling file relay 后，`federatedFileTransferExport` 将 `/v1/files/transfer/export` 转发给 Agent。Agent 在输出响应后以 `X-KPanel-Transfer-Result` trailer 标记传输失败；目标 Panel 的 `streamFederatedAgent` 只复制允许的普通响应头，不检查读完 body 后的 `response.Trailer`，也丢弃 `io.CopyBuffer` 错误。于是目标 handler 正常返回，relay finalizer 调用 `finish(nil)` 并发送 `end`；源端据此正常关闭 pipe，浏览器侧只在自己的 body copy 出错时才中止。若实际 Agent 导出在发出部分数据后以 clean body EOF 加 `error` trailer 结束，浏览器响应可能表现为正常完成而内容不完整。此路径要求管理员已登录、目标是 active 且有 files scope 的配对 Panel，并且选择了 V2 relay；源码没有显示认证绕过。当前无法在规定的源码只读边界内观察实际 Agent 错误 trailer 是否经这些层后变成浏览器侧 clean EOF。

**源码追踪：**
- `internal/panel/light_file.go:32` (entrypoint) — 浏览器文件请求进入受 Panel session 保护的远端文件 relay；host 协议选择后可走配对 Panel 的 V2 relay。
- `internal/panel/file_proxy.go:301` (propagation) — 目标 Panel 将 transfer/export GET 转给本机 Agent 的流式响应。
- `internal/agent/files.go:782` (propagation) — Agent 声明 X-KPanel-Transfer-Result 为 response trailer，传输完成后用它表达结果。
- `internal/panel/file_proxy.go:422` (propagation) — 目标 Panel 忽略 Agent body copy 的错误，也未把完成状态转成 relay error。
- `internal/cluster/panel_file_relay.go:373` (propagation) — handler 正常返回时 deferred finalizer 调用 writer.finish(nil)。
- `internal/cluster/panel_file_relay.go:532` (propagation) — nil error 分支排入正常 end 事件。
- `internal/cluster/panel_file_relay.go:756` (propagation) — 源端收到 end 后正常关闭输出 pipe，而不是 CloseWithError。
- `internal/panel/light_file.go:149` (sink) — 浏览器响应层仅在自己的 body copy 出错时中止；clean EOF 会继续记为 copy 完成。

**已核对的源码证据：**
- `internal/panel/light_file.go:61` — 浏览器远端文件请求要求有效 Panel session；非只读方法另行要求 Origin 与 CSRF 校验。
- `internal/cluster/panel_file_relay.go:570` — 源 Panel 只允许 active 配对记录且具有 files scope 的 host 进入远端文件请求。
- `internal/cluster/panel_file_relay.go:586` — 该方法在可用 V3 stream 不存在时调用 V2 remote file relay；本候选限于此回退路径。
- `internal/cluster/service_v2.go:810` — 目标 Panel 的 V2 poll 打开 active controller 并再次要求 files scope。
- `internal/agent/files.go:782` — Agent transfer/export 声明 X-KPanel-Transfer-Result trailer；此处理器没有设置 Content-Length。
- `internal/agent/files.go:814` — file copy、version 检查或 directory export 出错时，Agent 将 trailer 值设为 error；成功才设为 ok。
- `internal/panel/file_proxy.go:422` — io.CopyBuffer 的字节数和错误都赋给空白标识符，且函数返回前没有检查 response.Trailer。
- `internal/cluster/panel_file_relay.go:504` — V2 relay 响应头 allowlist 不含 Trailer，因此该终态标记也不会作为普通响应头被转发。
- `internal/cluster/panel_file_relay.go:532` — finish 收到 nil 且没有 queue error 时发送 end；runFileRelay 对 end 在第 756 行正常 Close，对 error 才在第 767 行 CloseWithError。
- `internal/panel/light_file.go:149` — 浏览器边界在自己的 copyErr 非 nil 时中止；copy 成功后第 154 行设置 copyCompleted。

**精确 blocker：**
- 当前环境没有要求的 OS 强制隔离沙箱，不能执行受控 in-memory Agent/relay fixture；因此没有观察到 Agent transfer trailer 或 body read error 经 V2 relay 后在浏览器边界的实际结果。
- 源码能证明错误状态的生成与转发缺口，但尚未动态确认一个部分输出的 Agent 导出在所支持的 HTTP transport 下表现为 clean body EOF 加 error trailer，也未确认调用方会把该浏览器响应接受为完整文件。

**有界本地下一步：**
在隔离、离线且有 CPU/内存/时间上限的 sandbox 中，用很小的确定性 fixture 经实际 Agent export handler 生成 status 200、部分数据和 X-KPanel-Transfer-Result:error；通过 in-memory Agent transport、目标 federatedFileHandler、panelFileRelaySession、OpenFileRelayV2 和 handleLightFileRelay 全链路记录。断言 Agent response.Trailer 的最终值、V2 终态是 end 还是 error、源端 pipe 的 EOF/error，以及浏览器 handler 是否将响应正常结束并设置 copyCompleted。另用 body 前缀后 sentinel read error 覆盖被忽略的 io.CopyBuffer 错误路径；对照成功 trailer 与显式 Content-Length 情况。仅用小缓冲区和内存 recorder，不访问文件系统、外网或真实主机数据。

**安全 owner-observed 部署检查：**
Ask the owner to inspect only existing non-production paired Panel state or configuration to determine V3 file-stream availability and whether V2 polling fallback is currently selected for this host. Have them inspect the deployed client version or existing logs for whether downloads verify expected size, checksum, or a terminal transfer status. Observe existing configuration, status, or logs only; do not initiate a download, inject a failure, or access file data.

## Abandoned V2 relay entries persist until a later poll

**Fingerprint:** `cluster.panel-file-relay.orphan-session-cardinality`

**Affected boundary:** internal/cluster/panel_file_relay.go#body terminal result and session cleanup

**Surface / subsystem:** V2 paired-Panel file relay and V3 reusable file socket / internal/cluster

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** A valid active V2 files controller can start a relay under a fresh session ID. If the source closes after the target poll has returned but before another poll is submitted, remoteFileRelayBody.Close cancels only the local polling loop and closes its pipe; it sends no target cancel command. The target removes sessions when a poll observes a collection error or collects a terminal end/error event, while expiry cleanup runs only at the start of a later poll. An abandoned session's map entry and session state can therefore remain after its two-hour context expires until another relay poll or service shutdown. The per-controller 1200-requests-per-minute limiter bounds arrival rate but is not an aggregate session cap. Source establishes the retention path, but not its realized resource impact or whether deployment controls impose a lower bound.

**源码追踪：**
- `internal/panel/light_file.go:32` (entrypoint) — The browser file-relay route accepts an allowlisted file path and host selector, then requires a Panel session and CSRF for writes before routing to the stored host.
- `internal/cluster/panel_file_relay.go:590` (propagation) — When the stored paired Panel has an active files scope and no usable V3 stream is selected, the source calls the V2 relay with its stored controller credential.
- `internal/cluster/service_v2.go:810` (propagation) — The target accepts the request only after active-controller and Noise-key validation and a normalized files-scope check.
- `internal/cluster/service_v2.go:822` (propagation) — The validated V2 request is passed to the target-side panelFileRelay poll handler.
- `internal/cluster/panel_file_relay.go:109` (propagation) — For a valid request command using a new session ID, the broker constructs a per-session context and state.
- `internal/cluster/panel_file_relay.go:110` (sink) — The newly allocated session is inserted into the shared map without checking an aggregate or per-controller session limit.

**已核对的源码证据：**
- `internal/panel/light_file.go:61` — The browser route requires an authenticated Panel session before opening a stored-host relay.
- `internal/panel/light_file.go:65` — Non-GET/HEAD requests also pass the Panel CSRF check.
- `internal/cluster/panel_file_relay.go:578` — The source prefers a usable V3 file stream and falls back to the V2 relay only when the stream is unavailable or its initial dial fails.
- `internal/cluster/panel_file_relay.go:569` — The source checks that the stored V2 controller is active and its normalized scope allows files before reading its credential.
- `internal/cluster/service_v2.go:807` — The target authenticates the controller record and Noise request before handling a V2 file-relay poll.
- `internal/cluster/service_v2.go:810` — The target rejects the request unless the active controller's normalized scope allows file operations.
- `internal/cluster/service_v2.go:1062` — V2 file-relay requests use the panelFileRequests limiter keyed by controller ID.
- `internal/cluster/service.go:301` — The panelFileRequests limiter is configured for 1200 requests per minute, which bounds request rate but not concurrent or retained sessions.
- `internal/cluster/panel_file_relay.go:30` — The broker stores sessions in a shared map with no owner count or admission-limit field.
- `internal/cluster/panel_file_relay.go:104` — Each target poll invokes gcLocked before looking up or creating a session.
- `internal/cluster/panel_file_relay.go:110` — A new valid request session is inserted without comparing registry size to a configured bound.
- `internal/cluster/panel_file_relay.go:123` — If the target collect call returns an error while a poll is in flight, that request path removes and aborts the session; this is a real cancellation cleanup path.
- `internal/cluster/panel_file_relay.go:129` — A session is removed when a poll collects an end or error event, so a completed but unpolled terminal event does not itself remove the map entry.
- `internal/cluster/panel_file_relay.go:151` — Expired sessions are swept by gcLocked, which is called from poll rather than from a timer or context-cancellation callback.
- `internal/cluster/panel_file_relay.go:161` — Each session context is rooted in context.Background with a two-hour timeout; its cancellation is not connected to the broker map.
- `internal/cluster/panel_file_relay.go:164` — Each session allocates a 128-slot event channel and its own processed-command map.
- `internal/cluster/panel_file_relay.go:909` — Closing the source response body only cancels the local run and closes the local pipe reader; it does not send a target cancel command.

**精确 blocker：**
- This run is source-only because the required OS-enforced isolated sandbox is unavailable; the exact close-between-polls timing, registry retention, and per-session resource cost have not been observed.
- Cancellation while the target collect call is still in flight reaches an error-removal path, and a later poll can collect terminal events or sweep expired sessions. A bounded reproduction must verify the post-return timing window and whether a meaningful number of abandoned entries persist; source does not establish long-lived handler goroutines.
- The path requires a valid active paired V2 controller credential with files scope, and the target applies a 1200-per-minute per-controller limit. The repository does not establish deployment gateway, process, or service-wide quotas or the resource threshold for shared impact.

**有界本地下一步：**
In an approved offline sandbox, use a deterministic in-memory transport, a harmless blocking file handler, a controllable clock, and at most eight authenticated dummy session IDs. Let a target poll fully return accepted/response, then close the source before it submits another poll; record registry size, allocations, and goroutines. Advance beyond the session TTL without any later poll, then issue exactly one unrelated authenticated poll and record cleanup. Repeat cancellation while collect is in flight to confirm that branch removes the entry. Keep strict process, memory, disk, and wall-clock limits; do not generate load or use network access.

**安全 owner-observed 部署检查：**
Have an authorized owner passively inspect the deployed revision, whether V3 is enabled, and existing metrics or logs for session TTL, cleanup behavior, and any process or gateway concurrency cap. Do not create abandoned sessions, send polls, generate load, or access production file data. If existing telemetry cannot establish cleanup behavior, retain this item as unresolved and use only the bounded local fixture in the isolated sandbox.

## 多个已登记 light node 可耗尽共用 replay 容量

**Fingerprint:** `cluster.replay-guard.shared-capacity-starvation`

**Affected boundary:** internal/cluster/light_service.go#AcceptLightReport health projection and public snapshot

**Surface / subsystem:** cmd/kejilion-node/main.go#collectAndReport light health capability / light-node-health

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码显示，light report 在节点身份与签名验证、每来源和每节点限流之后，将唯一 request ID 写入 Service 共用的 8192 项 replay guard；写入发生在 telemetry 校验之前。按源码限额，10 个已登记节点若各自对应不同的 remoteIP 来源，并在五个一分钟窗口各提交 180 个新 request ID，可提供 9000 个五分钟内未过期的条目。SignedSummary 也使用同一 guard，因此容量耗尽时新请求可能被拒绝。尚未在隔离环境中观察到跨路由拒绝及 TTL 到期后的恢复；节点凭据数量、来源地址映射和部署侧总限流也未确认。

**源码追踪：**
- `internal/panel/server.go:301` (entrypoint) — API 路由将匹配 light-node endpoint 的请求分派到 handleLightNodeFederation。
- `internal/panel/cluster.go:829` (propagation) — handler 选择 light report 分支并读取、解码有大小限制的请求体。
- `internal/panel/cluster.go:846` (propagation) — handler 将 remoteIP、节点及签名请求头和请求体传入 AcceptLightReport。
- `internal/cluster/light_service.go:183` (propagation) — 节点认证和两项限流通过后，服务把 NodeID 与 RequestID 提交给共享 replay guard；telemetry 校验在之后执行。
- `internal/cluster/guard.go:32` (sink) — guard 达到容量时拒绝新请求并返回 ErrRateLimited。

**已核对的源码证据：**
- `internal/cluster/service.go:295` — Service 创建一个容量为 8192、TTL 为五分钟的 replay guard。
- `internal/cluster/guard.go:24` — 所有调用将主体 ID 与 nonce 写入同一个 entries map 的 key 空间。
- `internal/cluster/light_service.go:176` — 写入 replay guard 前，light report 会验证已登记节点对应的认证请求。
- `internal/cluster/light_service.go:258` — 认证逻辑通过节点 ID 查询 light host 记录。
- `internal/cluster/light_service.go:270` — 认证逻辑对请求方法、路径、节点 ID、时间戳、request ID 和请求体计算签名并校验。
- `internal/cluster/service.go:310` — light report 每个 cleaned source subject 每分钟限 240 次，最多跟踪 2048 个 subject。
- `internal/cluster/service.go:311` — light report 每个节点每分钟限 180 次，最多跟踪 MaxHosts 个 subject。
- `internal/cluster/types.go:18` — MaxHosts 为 100，节点限流器的 subject 上限高于填满 guard 所需的 10 个节点。
- `internal/cluster/light_service.go:186` — replay entry 写入后才校验 telemetry；校验失败不会撤销该 entry。
- `internal/cluster/service.go:830` — SignedSummary 也调用同一 Service 的 replay guard。
- `internal/panel/server.go:1834` — remoteIP 来源由请求 peer 或受信代理配置决定，不能仅凭源码确认部署中可用的不同来源数。

**精确 blocker：**
- 未在具备所需隔离条件的环境中复现 guard 达满、独立 SignedSummary 请求被拒绝以及五分钟 TTL 后恢复。
- 攻击前提要求多个已登记 light node 的有效签名密钥，以及足够多不同的 server-derived remoteIP 来源；实际 NAT、代理、入口总限流和 Service 实例拓扑尚未确认。

**有界本地下一步：**
仅在 OS 强制隔离、无网络的沙箱中，以 fake clock 和 dummy stores 创建 10 个测试节点及 10 个不同 source subject；每节点在五个一分钟窗口各提交不超过 180 个唯一、有效签名的 request ID。确认请求写入 replay guard 的数量可达 8192，随后用独立认证的 controller 调用 SignedSummary 并观察是否得到 ErrRateLimited；将时钟推进超过五分钟后用新的 nonce 重试，检查 guard 是否恢复。用 telemetry 无效但可解码的请求体确认 replay entry 的写入顺序。达到预设断言即停止。

**安全 owner-observed 部署检查：**
Have an authorized owner passively inspect the deployed configuration and existing telemetry to determine whether light reports can arrive from multiple registered nodes and distinct remote-IP sources, whether an aggregate limiter exists, and whether federation routes share one replay-guard instance. Do not fill the guard, send test requests, or load-test production or shared staging; if configuration and existing telemetry cannot establish reachability, retain the blocker for the bounded offline local fixture.

## Legacy v1 summary-only controller keys reach write-capable file relay

**Fingerprint:** `cluster.v1-summary-scope-file-relay-expansion`

**Affected boundary:** internal/panel/light_file.go#route allowlist and host selector

**Surface / subsystem:** Panel browser request to light-node file relay / internal/panel

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** The v1 pairing flow creates and persists each controller with `cluster.summary.read`, and the v1 store rejects any other controller scope. In this candidate, `AuthorizeFileRelayV1` accepts that same summary-only scope after validating the controller ID, Ed25519 signature, target, timestamp and nonce, then enforces rate and replay limits. The target reconstructs the embedded request and dispatches it through the fixed file-manager allowlist, which includes `/v1/files/actions`, upload and transfer import. A write action reaches the Panel's authenticated local Agent file API and the file manager configured with root `/`, subject to its existing protected paths, read-only paths and path checks. A process holding an active legacy v1 controller private key can therefore use a summary-read identity for file mutations without a distinct files grant or target-side re-consent. The signature and downstream checks constrain identity, requests and paths, but do not grant file-specific authorization.

**源码追踪：**
- `internal/panel/server.go:305` (entrypoint) — The API router dispatches POST `/api/v1/federation/files/relay` to the federation handler; this route uses controller authentication rather than a target browser session.
- `internal/panel/cluster.go:525` (propagation) — The target invokes `AuthorizeFileRelayV1` before accepting the embedded file request.
- `internal/cluster/file_relay_v1.go:72` (propagation) — The authorization gate accepts exactly `SummaryScope`; subsequent checks verify the enrolled public key, signed target and timestamp, nonce, rate limits and replay state.
- `internal/panel/cluster.go:546` (propagation) — After authorization, the handler reconstructs the inner method, path, query, headers and body, then calls the federated file handler.
- `internal/panel/file_proxy.go:60` (propagation) — The fixed relay dispatch sends `/v1/files/actions` to `federatedFileAction` after the shared allowlist check.
- `internal/panel/file_proxy.go:344` (propagation) — A bounded, decoded and allowlisted action is passed to `doFileAction`.
- `internal/panel/files.go:1199` (propagation) — The action handler sends POST `/v1/files/actions` through the Panel's local Agent client.
- `internal/panel/agent.go:176` (propagation) — The Panel attaches its protected Agent bearer to the local request.
- `internal/agent/server.go:299` (propagation) — The Agent validates the bearer held by the Panel before routing the file request.
- `internal/agent/server.go:474` (propagation) — The authenticated Agent request is routed to `fileAction`.
- `internal/agent/files.go:905` (propagation) — The Agent passes the decoded action to its file manager.
- `internal/filemanager/manager.go:652` (propagation) — For a `mkdir` action, the file manager dispatches to its mutation implementation.
- `internal/filemanager/manager.go:855` (sink) — The validated action creates a directory in the Agent file manager's root filesystem, subject to its mutation policy.

**已核对的源码证据：**
- `internal/cluster/store.go:295` — V1 pairing codes expose `SummaryScope`.
- `internal/cluster/service.go:808` — `AcceptPair` constructs the persisted controller record.
- `internal/cluster/service.go:810` — The accepted v1 controller is stored with `Scope: SummaryScope`.
- `internal/cluster/store.go:569` — Persisted v1 controller records are valid only when their scope is exactly `SummaryScope`.
- `internal/cluster/types.go:14` — `SummaryScope` is `cluster.summary.read`; the files-capable scope is a separate constant.
- `internal/cluster/file_relay_v1.go:72` — The relay's authorization condition rejects when `record.Scope != SummaryScope`, so the summary-only record is admitted.
- `internal/cluster/signature.go:104` — `VerifyRequest` validates the Ed25519 signature over the federation request identity and target.
- `internal/panel/cluster.go:521` — An authenticated v1 summary response advertises `FileRelayV1Capability`.
- `internal/cluster/service.go:1042` — The source Panel sets its v1 file-management availability flag from the advertised capability.
- `internal/cluster/file_relay_v1.go:105` — The source preserves legacy pair records and credentials; the v1 relay path does not require a second pairing.
- `internal/cluster/protocol_v2.go:607` — The shared file route allowlist includes upload.
- `internal/cluster/protocol_v2.go:608` — The allowlist includes transfer import.
- `internal/cluster/protocol_v2.go:609` — The allowlist includes file actions.
- `internal/panel/files.go:1248` — `allowedFileAction` permits write operations including mkdir, rename, copy, move, trash, chmod and trash deletion.
- `internal/cluster/service_v2.go:810` — The v2 relay rejects controllers unless the normalized scope satisfies `ScopeAllowsFiles`.
- `web/src/views/ClusterView.vue:224` — The controller capability display is derived from the stored scope.
- `web/src/views/ClusterView.vue:235` — A record without the files scope is displayed as summary-read only.
- `internal/panel/agent.go:176` — The Panel's Agent client attaches its bearer to the local request.
- `internal/agent/server.go:299` — The Agent requires the Panel's bearer, which the preceding Panel call supplies.
- `internal/agent/server.go:474` — The Agent routes POST `/v1/files/actions` to its file action handler.
- `internal/agent/files.go:905` — The Agent invokes `s.files.Action` with the decoded request.
- `internal/agent/server.go:287` — The configured file manager root is `/`.
- `internal/agent/server.go:288` — The Agent applies protected virtual directories.
- `internal/agent/server.go:289` — `/proc`, `/sys` and `/dev` are configured read-only.
- `internal/filemanager/manager.go:855` — The `mkdir` action reaches a root-filesystem mutation after the file manager's policy check.

**精确 blocker：**
- The audit boundary prohibits running target code or tests and this host lacks the required OS-enforced isolated sandbox; no signed request was executed against a harmless Agent recorder to observe actual dispatch and response.
- Deployment-level endpoint reachability and the running Panel/Agent configuration were not exercised; source review alone cannot establish an affected installation's network exposure.

**有界本地下一步：**
In an approved offline sandbox with temporary state and no real Agent or filesystem writes, seed a v1 store with one generated Ed25519 controller whose scope is SummaryScope, then send a correctly signed POST to /api/v1/federation/files/relay wrapping a harmless POST /v1/files/actions mkdir request. Replace the Panel Agent client with a recorder that never forwards requests and retain only the authorization result and whether the recorder received the call. Confirm whether the active SummaryScope record reaches the recorder; use a bad signature and a deleted controller ID as negative controls. This validates the authorization boundary without invoking a host mutation.

**安全 owner-observed 部署检查：**
Ask the deployment owner to inspect, without sending audit traffic or changing configuration, the effective Panel listener and reverse-proxy/ingress rules for POST /api/v1/federation/files/relay from the already-paired v1 Panel network identity. Have them report whether that route is allowed, whether the legacy controller credential remains active, and the effective Agent file-manager root plus protected and read-only virtual paths. Do not probe the endpoint.

## A stale V2 backup can restore a revoked controller

**Fingerprint:** `cluster.v2-store.stale-backup-revocation-rollback`

**Affected boundary:** cmd/kejilion-node/batch_enrollment_attempt.go#resumable private-key state; internal/cluster/secrets.go and internal/cluster/secrets_v2.go#private-key persistence, record references, deletion and recovery

**Surface / subsystem:** cmd/kejilion-node/main.go#runEnroll batch attempt-file path; internal/cluster/service.go#cluster credential creation, pairing, revocation and startup orphan cleanup / cmd/kejilion-node/batch-enrollment; internal/cluster/credential-store

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** If a V2 controller revocation leaves a stale .previous file and the current state later fails decoding or validation before that backup is removed, openStoreV2 restores the pre-revocation snapshot. A holder of the old Noise private key could then pass the summary endpoint’s active-controller and peer-key checks and receive node telemetry. Source confirms this conditional path, but does not establish that the required storage-failure sequence occurs on supported deployment filesystems or that a remote peer can cause the current file to become invalid. The state directory is protected with mode 0700.

**源码追踪：**
- `internal/panel/server.go:303` (entrypoint) — Routes a recognized federation-v2 POST, including /api/v2/federation/summary, into the federation handler.
- `internal/panel/cluster.go:582` (propagation) — Passes the remote request path and decoded federation envelope to the cluster service.
- `internal/cluster/service_v2.go:736` (propagation) — Dispatches the summary path to handleSummaryV2 after request validation.
- `internal/cluster/service_v2.go:978` (propagation) — Requests authorization through openControllerV2 with only the active controller state allowed.
- `internal/cluster/service_v2.go:1047` (propagation) — Opens the Noise-authenticated request and obtains the peer static key.
- `internal/cluster/service_v2.go:1055` (sink) — Accepts the peer identity when its Noise static key matches the stored controller public key; a restored pre-revocation record can make the old key eligible again.

**已核对的源码证据：**
- `internal/cluster/service.go:774` — DeleteController routes a V2 controller to storeV2.RevokeController after the legacy store returns ErrNotFound.
- `internal/cluster/store_v2.go:450` — RevokeController marks the record revoked before persisting it.
- `internal/cluster/store_v2.go:801` — persistLocked writes the V2 state through atomicWriteFileV2.
- `internal/cluster/store_v2.go:1102` — The new target and containing directory are synced before backup cleanup.
- `internal/cluster/store_v2.go:1111` — Errors removing target.previous and syncing its removal are ignored; the writer then returns success.
- `internal/cluster/store_v2.go:172` — When the current file decodes or validates unsuccessfully, openStoreV2 attempts to restore the previous file, then rereads and validates it.
- `internal/cluster/store_v2.go:107` — persistedStateV2 contains no monotonic generation or revocation epoch to distinguish an older valid snapshot.
- `internal/cluster/store_v2.go:1190` — The store directory is chmodded to mode 0700; source does not establish an unprivileged route to corrupt the target.
- `internal/cluster/service_v2.go:1044` — The summary authorization path rejects controller records that are not in an allowed state.
- `internal/cluster/service_v2.go:1055` — After opening the request, the service checks that the Noise peer key equals the stored controller public key.
- `internal/cluster/service_v2.go:994` — An authorized summary request returns local telemetry in the federation response.
- `internal/panel/server.go:496` — The federation-v2 route accepts POST requests for the summary path.

**精确 blocker：**
- The host does not provide the required OS-enforced sandbox, so no target execution or fault injection was performed.
- The source shows the conditional rollback, but does not establish that a supported deployment filesystem can leave the backup after cleanup failure and later present an invalid current target before backup cleanup succeeds. No remote path to corrupt the target was established; the remaining trigger appears to require a storage fault or local privileged action.

**有界本地下一步：**
In the required no-network sandbox, use a scratch-only dummy store and test controller key. Configure atomicFileOpsV2 to fail only removal of target.previous after installation and the preceding directory sync succeed; verify revocation returns success and the backup remains. Replace only the dummy target with bounded invalid JSON, reopen the store, and check whether the old active record is restored. Send one bounded summary request signed with the dummy pre-revocation Noise key and record whether authorization succeeds, then stop.

**安全 owner-observed 部署检查：**
Use a disposable lab volume matching a supported deployment filesystem and mount configuration, with synthetic state only, to determine whether the ordered cleanup failure and subsequent target-invalidity sequence can occur. Do not use a live store or real credentials.

## Local and legacy light-node directory copies can publish after a source version conflict

**Fingerprint:** `filetransfer.local-light-source-terminal-status-dropped`

**Affected boundary:** internal/filemanager/transfer.go#ExportDirectory and ImportDirectory

**Surface / subsystem:** internal/panel/files.go#handleFileTransfer / internal/filemanager/cross-host-transfer

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** An authenticated Panel administrator can request a directory copy with a source resourceVersion. ExportDirectory writes the TAR and then rechecks the source root and archived entries; if an identity or resourceVersion check fails, it returns ErrConflict after bytes may already have been sent. The Agent reports that late result through X-KPanel-Transfer-Result. The local source adapter returns response.Body and metadata only, so the caller cannot inspect that trailer or convert a non-ok result into a reader error. If the body consequently ends with ordinary EOF, the target importer drains the TAR, publishes its staged directory, and the Panel can report completion from the target response. The legacy light-node poll fallback has the same gap: its response event snapshots headers before streaming and its end event carries no late trailer result. The upgraded light file-stream path explicitly withholds streamEnd for a non-ok export result, so light-node exposure is conditional on the legacy fallback being selected. The affected property is integrity of an administrator-selected copy when a process already authorized to write the source changes it during export; this does not establish an authentication bypass, path escape, or overwrite.

**源码追踪：**
- `internal/panel/files.go:739` (entrypoint) — The Panel file-transfer endpoint requires an authenticated session and CSRF check before it accepts the administrator's source path, resourceVersion, and destination.
- `internal/agent/files.go:782` (propagation) — The source Agent declares X-KPanel-Transfer-Result as a trailer; after the export finishes it sets the trailer to ok or error.
- `internal/filemanager/transfer.go:121` (propagation) — After closing the TAR writer, the exporter rechecks the root and each archived entry and can return ErrConflict after archive bytes have been emitted.
- `internal/panel/files.go:980` (propagation) — The local adapter returns response.Body and metadata only, so its caller does not receive the response trailer.
- `internal/panel/files.go:869` (propagation) — The Panel forwards the source body as the target import request body; its completion path evaluates the target response and does not inspect the source export trailer.
- `internal/agent/files.go:848` (propagation) — For a directory transfer, the target Agent passes the incoming request body directly to ImportDirectory.
- `internal/filemanager/transfer.go:201` (sink) — After extraction and a successful drain, the importer publishes its staged directory with a no-replace rename; ordinary EOF satisfies the drain.

**已核对的源码证据：**
- `internal/panel/files.go:739` — The handler obtains a Panel session and checks CSRF before decoding and processing the file-transfer request.
- `internal/panel/files.go:747` — The source path and target directory must pass validFileDownloadPath, the source cannot be the root, and a nonempty resourceVersion is required.
- `internal/agent/files.go:782` — The export handler declares X-KPanel-Transfer-Result as an HTTP trailer before writing the response body.
- `internal/agent/files.go:814` — After the exporter returns, the Agent sets the trailer to error on any export failure, including a late ErrConflict.
- `internal/filemanager/transfer.go:130` — After the TAR writer is closed, each archived entry is re-Lstatted; an identity or resourceVersion mismatch returns ErrConflict.
- `internal/filemanager/archive.go:406` — While copying a regular file into the TAR, the manager checks the read length and post-read resourceVersion and can return ErrConflict after streaming bytes.
- `internal/filemanager/manager.go:1583` — resourceVersion is based on path, size, modification time, mode, owner, and group rather than file-content bytes; the candidate is limited to mutations that the source checks detect.
- `internal/panel/files.go:980` — openLocalFileTransfer returns response.Body and metadata, dropping caller access to response.Trailer.
- `internal/panel/files.go:904` — The Panel reports success after the target responds successfully with a decodable FileEntry; this path contains no source-trailer check.
- `internal/agent/files.go:848` — The directory import path gives ImportDirectory the request body without a separate source terminal-status value.
- `internal/filemanager/transfer.go:192` — The importer drains beyond the TAR end and aborts when the underlying reader returns an error; ordinary EOF is accepted.
- `internal/filemanager/transfer.go:201` — After a clean drain, the importer publishes the staged directory with renameNoReplaceRoot and then returns its FileEntry.
- `internal/cluster/light_file.go:38` — OpenLightFile selects the upgraded stream when prefersStream is true and otherwise uses the legacy lightFile poll relay.
- `internal/cluster/light_file.go:77` — OpenLightFileTransfer validates the response metadata but returns only response.Body, with no trailer field exposed to the caller.
- `cmd/kejilion-node/light_file.go:345` — The legacy poll writer copies the response headers into its initial response event when WriteHeader is called, before the export finishes.
- `cmd/kejilion-node/light_file.go:374` — The legacy poll writer unconditionally sends an end event and has no branch for a late X-KPanel-Transfer-Result value.
- `internal/cluster/light_file.go:416` — The legacy relay maps an end event to session.finish(nil), losing any exporter failure that was not transmitted in an event.
- `internal/cluster/light_file.go:978` — When that legacy session finishes without an error, its response body returns io.EOF.
- `internal/cluster/file_stream.go:482` — The upgraded light stream refuses to emit streamEnd for a successful-status export whose X-KPanel-Transfer-Result is not ok, providing a source-visible negative control.
- `internal/panel/cluster.go:748` — The full-panel federation producer checks the Agent trailer after copying and turns a non-ok result into copyErr before finishing the encrypted stream.
- `internal/cluster/file_transfer_v2.go:552` — The full-panel consumer maps the authenticated source_transfer_failed record to a read error rather than EOF.

**精确 blocker：**
- No required OS-enforced offline sandbox is available, so no bounded native transfer was run to observe the response trailer, body terminal error, target ImportDirectory result, staging cleanup, or actual destination publication.
- A concurrent mutation must occur after source bytes are emitted and before the final root/member checks, and it must alter the inode or resourceVersion fields that the current checks observe; source alone does not demonstrate that schedule or its end-to-end effect.
- The light-node exposure depends on the runtime prefersStream choice. Source shows the upgraded path's non-ok guard and the legacy fallback's unconditional end, but the active mode for any deployed light node is unknown.

**有界本地下一步：**
In an approved offline Linux sandbox, use one small dummy source directory and destination under scratch, with strict byte, entry, time, and process limits. Through the unmodified local Panel-to-Agent production path, deterministically mutate a permitted source entry after TAR data has been emitted and before ExportDirectory's final Lstat checks. Record the exporter result/trailer, source Body.Read terminal value, ImportDirectory return value, staging cleanup, destination presence/content, and Panel completion response. The candidate result is trailer=error followed by an ordinary source EOF, a successful target response, and a published destination; the secure result is a reader error and no publication. Repeat the same bounded case through the legacy light poll path and upgraded stream path as separate controls, forcing and recording each route selection. Use only non-sensitive dummy files, no external network, and no production endpoints.

**安全 owner-observed 部署检查：**
For any fleet-level conclusion, have an owner inventory which paired light nodes select prefersStream and which use the legacy poll fallback. If legacy nodes remain, reproduce only in a non-production paired lab with dummy files and record the same trailer/body/commit observations; do not infer production impact from the fallback implementation alone.

## Host restore path checks are not inode-bound across writes into a mutable destination parent

**Fingerprint:** `hostbackup.restore.mutable-parent-symlink-race`

**Affected boundary:** internal/agent/server.go#host restore route and destination validation

**Surface / subsystem:** Host backup restore destination path handling / internal/agent

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码确认 restore 仅在路径名上检查目标及 staging 父级，之后仍以路径名创建 .kpanel-next-<id>、复制归档子项并设置元数据。若 Linux/Unix 部署中一个低权限本地进程能在恢复期间重命名所选目标的父目录条目，它可能在检查后将 staging 目录替换为指向目标树外位置的符号链接，使 root Agent 的后续路径操作越出目标树。该操作仍要求管理员显式恢复一个通过归档校验的导入备份，且导入根必须匹配当前 destination inventory；源码不能证明任何实际部署满足父目录权限前提或竞态可命中。

**源码追踪：**
- `internal/agent/server.go:299` (entrypoint) — Agent 备份 API 的恢复操作先经过 Bearer 认证；该授权恢复是触发 root 写入的控制入口，竞态参与者是另一个可能具有父目录写权限的本地低权限进程。
- `internal/agent/server.go:333` (propagation) — 通过认证的备份路径请求被分派给 hostbackup.Service。
- `internal/hostbackup/service.go:120` (propagation) — POST /v1/backups 请求进入创建备份任务的处理路径。
- `internal/hostbackup/service.go:321` (propagation) — restore 只接受状态为 ready 的已导入来源，并要求请求 revision 与导入记录匹配。
- `internal/hostbackup/service.go:372` (propagation) — 校验后的 payload 链接到 restore job，再由 Engine.Restore 执行恢复。
- `internal/hostbackup/restore.go:134` (propagation) — 导入根路径必须与当前 destination inventory 中已有根路径完全相同，拒绝归档任意指定新的主机目标。
- `internal/hostbackup/restore.go:222` (propagation) — 在生成 sibling staging 路径前，恢复对目标路径组件执行 noLinkParents 的逐项 Lstat 检查。
- `internal/hostbackup/restore.go:314` (propagation) — 复制前再次检查 entry.Next 的父路径，但该检查结束后仍继续使用路径字符串。
- `internal/hostbackup/restore.go:320` (propagation) — 归档树以 entry.Next 路径字符串传给 copyTree，没有传入已打开并固定身份的目录句柄。
- `internal/hostbackup/restore.go:668` (sink) — copyTree 按 filepath.Join 得到的目标路径调用 os.OpenFile 创建归档子项；路径中的被替换目录组件没有 no-follow 或目录句柄约束。

**已核对的源码证据：**
- `internal/agent/server.go:299` — ServeHTTP 在路由分派前执行 authorized 检查，未通过 Bearer 校验的请求直接返回 401。
- `internal/hostbackup/service.go:321` — restore 来源必须是 ready import，且输入 revision 必须非空并与来源 TargetRevision 相等。
- `internal/hostbackup/restore.go:127` — Restore 拒绝与受保护 Panel 数据重叠的恢复根。
- `internal/hostbackup/restore.go:134` — 每个导入根都必须匹配当前 destination inventory 中的 existing.Path。
- `internal/hostbackup/restore.go:165` — Restore 检查未选中容器是否挂载了与所选根重叠的路径，避免留下另一写入者。
- `internal/hostbackup/archive.go:289` — 归档条目要求 data/<root-id> 前缀、规范化路径、无反斜杠且不重复。
- `internal/hostbackup/archive.go:297` — 归档只允许普通文件或目录，并限制大小、模式、UID/GID；这排除了直接归档路径穿越或特殊文件攻击，但不固定目标路径 inode。
- `internal/hostbackup/restore.go:222` — 目标链接检查发生在预检阶段，完成后没有保留被检查目录的打开句柄。
- `internal/hostbackup/restore.go:225` — Previous 和 Next 由目标路径字符串拼接生成 sibling 路径。
- `internal/hostbackup/restore.go:230` — Next 只在该时刻以 Lstat 检查为不存在；检查结果未绑定后续创建使用的 inode。
- `internal/hostbackup/restore.go:237` — restore journal 保存的是 Target、Previous、Next 字符串和 HadPrevious 状态。
- `internal/backup/files.go:43` — NoLinkParents 逐层调用 Lstat 并返回，不保留目录描述符或 inode 身份。
- `internal/hostbackup/restore.go:314` — 实际复制前的第二次检查只覆盖 entry.Next 的父路径。
- `internal/hostbackup/restore.go:317` — staging 父目录通过路径名调用 MkdirAll 创建。
- `internal/hostbackup/restore.go:320` — copyTree 接收路径名 entry.Next。
- `internal/hostbackup/restore.go:655` — copyTree 为源目录及归档子目录按 destination 字符串调用 MkdirAll。
- `internal/hostbackup/restore.go:668` — 普通归档子项按 destination 字符串调用 OpenFile 创建。
- `internal/hostbackup/restore.go:680` — 文件所有权随后按同一路径调用 copyOwnership。
- `internal/hostbackup/restore.go:683` — 文件模式随后按同一路径调用 Chmod。
- `internal/hostbackup/restore.go:332` — 完成旧树处理后，staging 到目标的交换仍使用 os.Rename 路径名。
- `internal/hostbackup/owner_unix.go:11` — Unix 构建使用路径名调用 os.Chown，因此若路径解析被重定向，元数据操作也可能作用于被重定向目标。
- `deploy/systemd/kejilion-agent.service:8` — 仓库提供的 systemd Agent 服务以 root 用户运行。
- `deploy/systemd/kejilion-agent.service:25` — systemd 服务包含 CAP_DAC_OVERRIDE 与 CAP_CHOWN 等能力，增加路径被重定向时的潜在影响；目标父目录权限仍须按部署核实。
- `internal/hostbackup/owner_windows.go:7` — Windows 构建的 restoreOwnership 与 copyOwnership 是空操作；所有权影响目前只由 Unix 构建路径支持，跨平台链接重定向行为未在本次 source-only 核验中验证。

**精确 blocker：**
- 本次审核边界为 source-only，禁止运行目标代码、测试、构建、fixture 或脚本，因而没有证实符号链接替换是否能在目标系统上重定向 MkdirAll/OpenFile，也没有观察到任何越界写入。
- 源码与部署模板不能确定具体主机上所选目标父目录的 owner、mode、ACL、sticky bit、挂载语义，以及是否存在能在 restore 窗口内重命名其条目的低权限进程。

**有界本地下一步：**
在获批的离线 Linux root sandbox 中，以 scratch-only Engine.Root、fake Docker transport 和合法且路径安全的最小 imported payload 构造 destination inventory 中已有的目录根。让该根的父目录由 dummy 非特权 UID 可写且无 sticky-bit 限制，在独立 scratch sentinel 中放一个不存在的归档子项；使用隔离副本中的确定性 barrier，将父目录写入者安排在 staging 父级检查之后、copyTree 源根 MkdirAll 之后且首个子项 OpenFile 之前，把仅本次生成的 .kpanel-next-<id> 目录换成指向 sentinel 的符号链接。仅记录 sentinel 子项是否出现、owner/mode、目标路径类型、Restore 返回值和清理结果，第一次观察到重定向效果即停止。sentinel 无变化且操作以 no-follow/路径错误失败则拒绝该路径；发生 sentinel 创建或元数据更改则按实际效果确认并限定影响。所有文件写入限 scratch，禁用外部网络并限制 CPU、内存和时长。

**安全 owner-observed 部署检查：**
对目标部署做只读核查：确认 Agent 的有效 UID/能力，枚举可选 restore roots 及每个 Next sibling 所在父目录的 owner、mode、ACL、sticky bit、文件系统/挂载选项，并确认是否有低权限本地服务或容器能在恢复窗口内创建、删除或重命名条目。若存在该能力，只在相同配置的隔离副本上按上述最小场景验证；不要在生产目录尝试竞态。

## 共享登录尝试上限可能阻断新身份认证

**Fingerprint:** `login-attempt-retention-cap-blocks-authentication`

**Affected boundary:** auth invariant; internal/auth/service.go#reserveLogin

**Surface / subsystem:** auth; internal/auth/passkey.go#BeginLogin / internal/auth

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码显示，未登录的 Passkey login/begin 请求会按 remote IP 限流，并在生成 challenge 前向共享 LoginAttempts 写入一条记录。Store 保留窗口内达到 4096 条后拒绝所有新记录；密码登录也必须先写入同一记录集合，成功记录后才创建 session。因此满额时，Passkey begin 无法取得 challenge，密码登录无法建立 session。源码不能确认目标部署中匿名入口是否可达、攻击者能否在保留窗口内提供足够多的独立来源，或该情况是否会实际发生。

**源码追踪：**
- `internal/panel/passkey.go:357` (entrypoint) — Passkey login/begin 分支将请求的 remote IP 传给 BeginLogin；该登录分支在管理操作的 session 检查之前返回。
- `internal/auth/passkey.go:348` (propagation) — 通过每 IP 预算检查后，BeginLogin 在调用 WebAuthn challenge 生成前写入一条共享 LoginAttempt。
- `internal/store/store.go:738` (sink) — 保留窗口内已有记录加新记录超过 4096 时返回 ErrLimitReached，不驱逐窗口内记录；Passkey begin 因而不能继续生成 challenge。

**已核对的源码证据：**
- `internal/panel/passkey.go:134` — 登录路径进入 handlePasskeyLogin 并提前返回；requireSession 位于之后的管理操作路径。
- `internal/panel/passkey.go:359` — 登录开始处理将 remoteIP、用户名和 binding 传给 BeginLogin。
- `internal/auth/passkey.go:342` — Passkey begin 按 IP 计算预算，预算为 MaxLoginFailures 的两倍。
- `internal/auth/passkey.go:348` — 在 challenge 生成之前写入共享 LoginAttempts。
- `internal/store/store.go:719` — 共享登录尝试上限是 4096 条。
- `internal/store/store.go:735` — Store 明确不驱逐仍在保留窗口内的记录。
- `internal/store/store.go:738` — 满额后新写入返回 ErrLimitReached。
- `internal/auth/service.go:314` — 密码登录必须先成功记录登录尝试。
- `internal/auth/service.go:317` — 密码登录只有记录成功后才创建 session。
- `internal/auth/service.go:797` — 密码登录将 IP 键和账户键两条记录批量写入同一个 Store。
- `internal/panel/server.go:859` — 安全入口启用时，非公开路径要求安全入口 cookie 或有效 session；部署状态会影响匿名登录路由可达性。
- `internal/panel/server.go:1834` — remoteIP 依据连接对端及可信代理配置处理转发 IP。
- `internal/panel/config.go:56` — 默认登录窗口为 15 分钟。
- `internal/panel/config.go:58` — 默认 MaxLoginFailures 为 5，对应 Passkey begin 每 IP 窗口预算 10 次。

**精确 blocker：**
- 当前宿主没有工作流要求的 OS 强制隔离环境；本次未运行目标代码、测试或构建，不能提供规定的动态确认。
- 目标部署的安全入口状态、外部路由及代理和上游限流配置未知；这些决定匿名 Passkey begin 和密码登录是否可达，以及来源 IP 是否能被正确区分。
- 达到 4096 条需要在实际保留窗口内积累足够多的记录。默认 Passkey begin 预算是每 IP 10 次；攻击者是否能提供足够多的独立来源，源码无法确定。

**有界本地下一步：**
在满足工作流 OS 隔离要求的临时环境中使用 dummy 用户和 Store：先写入 4096 条仍在保留窗口内、且不匹配测试 IP 或账户键的记录；调用 Passkey BeginLogin 和带有效 dummy 密码的 Service.Login，观察前者是否返回 ErrLimitReached 且没有 challenge，后者是否返回记录错误且未创建 session。全程不启监听端口、不访问网络、不使用真实凭据。

**安全 owner-observed 部署检查：**
只读核对目标实例的安全入口启用状态、登录路由暴露范围、可信代理 CIDR 与 remote IP 传递方式、KEJILION_PANEL_MAX_LOGIN_FAILURES、loginWindow 及上游限流规则；不向线上入口发送请求或饱和流量。

## MCP backup restore approval does not bind the Agent target revision used at dispatch

**Fingerprint:** `mcp.backup.restore-agent-revision-approval-drift`

**Affected boundary:** internal/panel/server.go#route registration and authenticated backup handler

**Surface / subsystem:** MCP backup restore approval routing / internal/panel

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码支持这一 revision 绑定缺口：host_backup_restore 的 durable Arguments 包含 AgentRevision，且其 digest 覆盖完整 Arguments；执行时会从已批准的 Arguments 重新准备请求，但共享 backupRestore 只校验 input.Revision 与 Panel source.TargetRevision。对于不含 panel 模块的导入源，Panel preview 将 TargetRevision 设为 host-only，同时调用 Agent preview 并把返回值写入 source.AgentRevision。restore 经 ReserveFrom 读取当时的 source，随后把其 AgentRevision 发给 Agent。因此，若预览把 A 更新为 B 后、旧批准操作 dispatch 前先完成更新，实际请求可带 B，而 host-only Panel revision 仍匹配；Agent 也会按其当前导入记录的 B 校验。源码未证明具体 A→B 记录状态及顺序已实际发生。预览本身仍受 Panel 登录和 CSRF 控制，MCP preview 也需要审批；这里是旧 restore 批准与后续 Agent revision 的绑定问题，不是已证明的授权绕过。

**源码追踪：**
- `internal/panel/mcp_management.go:45` (entrypoint) — MCP SDK 将较低信任客户端的 managed tool 参数交给 callManagedMCP。
- `internal/panel/mcp_management.go:152` (propagation) — 非只读 restore 的规范化 Arguments 被持久化为待审批 operation，digest 绑定这些参数。
- `internal/panel/mcp_management.go:331` (propagation) — 执行时从持久化 operation.Arguments 重新准备 dispatch 请求。
- `internal/panel/mcp_backups.go:77` (propagation) — 准备器把已批准的 input.AgentRevision 放入共享 Panel restore 请求体。
- `internal/panel/mcp_backups.go:95` (propagation) — 本地 backups operation 将准备好的请求交给共享 serveBackupOperation handler。
- `internal/backup/jobs.go:175` (propagation) — restore 执行时从 manager 当前 records 映射读取 source，而非从批准 operation 保存 source 快照。
- `internal/panel/backup_handlers.go:602` (propagation) — 取得当前 pinned source 后再次只检查 Panel TargetRevision 与 input.Revision。
- `internal/panel/backup_handlers.go:640` (sink) — 构造发往 root Agent 的 restore 请求时使用当前 source.AgentRevision，而不是批准参数 input.AgentRevision。

**已核对的源码证据：**
- `internal/panel/mcp_backups.go:71` — host_backup_restore 对 AgentRevision 只施加长度上限，不要求其与 source 当前 revision 相等。
- `internal/panel/mcp_backups.go:77` — 准备器将输入的 AgentRevision 序列化到共享 backupRequest 中，表明该值属于调用意图。
- `internal/mcpaccess/operations.go:95` — operationDigest 对 ClientID、HostID、HostIdentity、Tool 和完整 Arguments 计算 digest，故批准时的 AgentRevision 被 digest 固定。
- `internal/panel/backup_handlers.go:172` — preview 对不含 panel 模块的备份把 Panel revision 初始化为固定字符串 host-only。
- `internal/panel/backup_handlers.go:188` — 有 AgentID 时，Panel preview 向 Agent 请求该导入备份的最新预览。
- `internal/panel/backup_handlers.go:194` — preview 将当前 Panel revision 与 Agent preview revision 写回同一 source record 的 TargetRevision 和 AgentRevision。
- `internal/panel/backup_handlers.go:581` — backupRestore 的初始校验只要求 input.Revision 等于 source.TargetRevision；没有校验 input.AgentRevision。
- `internal/backup/jobs.go:175` — ReserveFrom 在 restore 执行时从 manager 当前记录映射读取导入 source。
- `internal/panel/backup_handlers.go:602` — ReserveFrom 后的复核仍只比较 Panel TargetRevision 与批准参数，不比较 AgentRevision。
- `internal/panel/backup_handlers.go:640` — Agent 请求的 Revision 直接来自当前 source.AgentRevision。
- `internal/hostbackup/service.go:321` — Agent 按其当前导入记录的 TargetRevision 校验收到的 restore revision；Agent preview 将该记录更新到 B 后，发送 B 可通过这一检查。
- `internal/panel/mcp_management.go:354` — 若 MCP worker slot 已满，执行在 Claim 前返回 retry 错误，operation 仍可能保持 approved 供稍后重试；这提供了旧批准与后续 preview 之间的源码可见时序窗口。
- `internal/panel/backup_handlers.go:59` — 普通 Panel backup 写操作先要求有效的已认证 session。
- `internal/hostbackup/service.go:204` — Agent preview 通过 Engine.Inventory 更新导入记录 TargetRevision，因此其后续 restore 检查可接受新 revision B。
- `internal/panel/mcp_backups.go:57` — MCP host_backup_preview 是非只读 POST 工具，文档与注册逻辑要求单独审批。
- `internal/panel/mcp_management.go:573` — Panel 管理员批准 operation 后，handleMCPOperations 会立即尝试调用 executeMCPOperation。
- `internal/panel/mcp_management.go:486` — MCP operation_execute 对已获批 operation 再次调用执行路径，允许 worker slot 拒绝后稍后重试。
- `internal/panel/backup_handlers.go:63` — Panel backup 的非 GET 请求还须通过 Origin 与 CSRF 校验后才进入 preview handler。
- `internal/panel/mcp_backups.go:69` — host_backup_restore 注册为非只读工具，描述为 always requires approval。

**精确 blocker：**
- 本轮被限制为 source-only，未运行目标代码、测试、fixture 或 stub Agent；没有实际观察 host-only source 从 AgentRevision=A 经 preview 变为 B，也没有捕获旧 operation 的 dispatch payload。
- 普通 Panel 审批在 Decide 后会立即尝试执行，MCP host_backup_preview 也是单独的审批操作；需要在离线 harness 中确认可重试 approved operation 与受控 preview 的具体先后关系。源码显示 worker slot 满时执行可在 Claim 前返回并保留 approved 状态，但本轮没有实测该完整路径。

**有界本地下一步：**
仅在现有离线 Panel backup harness、临时目录和 recording Agent 中建立带 AgentID 的 host-only imported source（modules 不含 panel，Panel TargetRevision=host-only，AgentRevision=A）。创建并批准 AgentRevision=A 的 host_backup_restore operation；用占满 MCP worker slot 的 dummy operation 使批准后的执行在 Claim 前返回 retry 错误并保持 approved。通过测试中的已认证 Panel preview 路由让 stub Agent 返回 B，确认 Panel TargetRevision 仍为 host-only 且 source.AgentRevision=B；释放 slot 后用原 operationId 和 digest 重试。只记录 Panel 收到的请求，断言当前代码是否发送 Revision=B；recording Agent 不得调用 Engine.Restore 或修改宿主数据。随后可用 revision equality guard 重跑，确认不一致时在 Agent 调用前拒绝。

**安全 owner-observed 部署检查：**
不接触生产。若需要部署侧复核，只在隔离 staging 使用不执行恢复的 stub Agent，记录 preview A→B 后旧 operation 的 restore 请求，验证是否在 dispatch 前拒绝；不连接真实主机数据。

## Reusable public download tickets may let one bearer monopolize authenticated file-read capacity

**Fingerprint:** `panel.download-ticket.reusable-unthrottled-shared-gate-starvation`

**Affected boundary:** per-IP rate limiters on share/ticket/federation routes

**Surface / subsystem:** unauthenticated public endpoints / internal/panel

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** Source confirms that an authenticated operator can create a five-minute, random bearer ticket for a canonical file path; a valid GET ticket redemption is publicly routed, does not require the operator session, and lookup leaves the live ticket reusable. Each ordinary-file redemption streams through the Agent fileContent path into the same Manager.Open used by authenticated file reads. That manager has one four-slot downloadGate per Agent manager instance, and a full gate returns ErrBusy (mapped to HTTP 429); each acquired slot remains attached to the opened file until the stream closes. The ticket handler has no per-ticket active-stream or per-IP admission, while the separate public-share handler has both IP rate limiting and a stream gate. The ticket is an intentional bearer capability, not a forgery: creation requires Origin, session, and CSRF checks. A malicious recipient of a ticket for a sufficiently large regular file could therefore try to occupy all four shared slots and deny ordinary reads. Source does not establish whether slow-client backpressure can keep four streams admitted long enough under the effective Go HTTP and deployment deadlines, or how quickly repeated requests can recycle slots.

**源码追踪：**
- `internal/panel/server.go:279` (entrypoint) — External Panel HTTP requests under /api/ are dispatched to the API router after normal host and security-entrance handling; valid ticket paths are explicitly exempted from the enabled security entrance.
- `internal/panel/server.go:440` (propagation) — A syntactically valid /api/v1/files/download/<token> path is dispatched directly to handleFileDownloadTicket.
- `internal/panel/files.go:323` (propagation) — After method, query, and token-shape checks, each request looks up the same ticket; a successful lookup does not consume it.
- `internal/panel/files.go:342` (propagation) — For an ordinary-file ticket, the Panel calls streamFileDownload with the stored path and does not acquire a ticket-specific stream slot.
- `internal/agent/files.go:483` (propagation) — The Agent ordinary file-content route calls the shared file manager's Open method; the file remains deferred for closing while ServeContent streams the response.
- `internal/filemanager/manager.go:306` (sink) — Manager.Open acquires the process-wide four-slot ordinary downloadGate and returns a gatedFile that retains the slot until Close; a full gate returns ErrBusy.

**已核对的源码证据：**
- `internal/panel/server.go:867` — When the security entrance is enabled, securityEntrancePublicPath explicitly treats valid file-download-ticket paths as public.
- `internal/panel/files.go:32` — Each download ticket expires five minutes after issue; expiration is checked at lookup and does not itself cancel an already-started request.
- `internal/panel/files.go:237` — Ticket creation checks Origin before issuing the capability.
- `internal/panel/files.go:240` — Ticket creation requires a valid authenticated session and CSRF token.
- `internal/panel/files.go:308` — The redemption handler accepts GET and HEAD and performs no session, Origin, CSRF, IP-rate, or active-stream check before ticket lookup.
- `internal/panel/files.go:550` — lookupFileDownloadTicket returns a valid unexpired map record without deleting it or decrementing a use count.
- `internal/panel/files.go:357` — Each ticket stream has a two-hour request context; ticket expiry is not used as the stream deadline.
- `internal/panel/files.go:376` — Ticket output uses a 45-second idle-aware response writer and copies the Agent body until completion or write failure; this bounds idle writes but does not partition stream admission.
- `internal/agent/server.go:299` — The downstream Agent still requires its internal bearer credential, so this candidate is not direct unauthenticated access to the Agent API.
- `internal/agent/files.go:483` — The non-share file-read branch invokes s.files.Open; ticket reads and authenticated ordinary reads use this same branch.
- `internal/agent/files.go:489` — The Agent defers file.Close until the response handler returns, tying the Manager.Open slot lifetime to stream completion.
- `internal/filemanager/manager.go:124` — New initializes one shared ordinary downloadGate with capacity four.
- `internal/filemanager/manager.go:340` — A successful Manager.Open returns a gatedFile associated with that same downloadGate.
- `internal/filemanager/manager.go:1694` — acquireNow uses a non-queuing select for gate admission and immediately returns ErrBusy when the gate is full.
- `internal/filemanager/manager.go:1726` — gatedFile.Close releases the retained downloadGate slot.
- `internal/agent/files.go:995` — The Agent maps filemanager.ErrBusy to HTTP 429 file_transfer_busy.
- `internal/panel/file_shares.go:353` — The distinct public file-share content path has a per-IP request limiter; the file-download-ticket handler does not enter this handler.
- `internal/panel/file_shares.go:364` — The distinct public file-share content path also acquires a Panel active-stream gate; ticket downloads do not use it.
- `internal/httpstream/idle.go:76` — The idle response writer refreshes a per-write deadline, so the interaction between slow reads, socket backpressure, and server timeouts cannot be determined from the handler source alone.
- `cmd/paneld/main.go:121` — The production Panel HTTP server configures a 60-second WriteTimeout in addition to the explicit idle writer.
- `cmd/kejilion-agent/main.go:221` — The production Agent HTTP server configures a two-minute WriteTimeout; its interaction with the stream's refreshed write deadline requires runtime observation.

**精确 blocker：**
- This host lacks the required OS-enforced offline sandbox, empty environment allowlist, read-only target/tool mounts, scratch-only target writes, and process/resource isolation. The source-only assignment prohibits running target code, tests, builds, fixtures, browser actions, or network calls, so the decisive stream behavior was not observed.
- Source proves that repeated valid ticket redemptions reach the same four-slot Manager.Open gate and that a full gate rejects other file readers, but it does not prove that four slow client responses can retain the Agent file descriptors under the effective Panel/Agent HTTP and Unix-socket backpressure deadlines, or that one bearer can recycle all four slots.
- The impact requires an operator to create a ticket for a sufficiently large regular file and give its bearer URL to a recipient who can reach the permitted Panel Host. Ticket creation itself is protected by Origin, session, and CSRF checks, and those controls do not prevent intentional sharing with an untrusted recipient.

**有界本地下一步：**
Only in an approved offline Linux sandbox, use prebuilt Panel and Agent binaries with Panel bound to loopback, Agent reachable only by Unix socket, target/tools read-only, all mutable state and logs in scratch, external networking disabled, and strict CPU/memory/process/disk/time limits. Create one fixed 16 MiB regular file and bootstrap an operator; create one ordinary file ticket through the authenticated Origin+CSRF flow. Run four bounded clients concurrently against the same ticket and stop reading after response headers; while they are connected, issue an authenticated ordinary content GET for a different scratch file and record status, latency, Agent descriptors, and recovery. If idle writers release promptly, allow each client to read one small chunk before the 45-second idle deadline and repeat the authenticated read once, with the experiment capped at two minutes. Stop all clients and verify slots recover. Confirm only if ticket streams actually occupy all four slots and the independent authenticated request receives file_transfer_busy/429 or remains unavailable for the bounded interval; prompt authenticated success or failure to hold the slots refutes the claim. Do not contact any deployment endpoint or shared service.

**安全 owner-observed 部署检查：**
Before attributing the behavior to a deployed service, inspect the effective reverse-proxy/ingress connection, per-IP rate, and timeout limits for the exact permitted Panel Host and ticket path. Do not probe a production endpoint; if deployment-specific behavior is needed, reproduce it in an isolated staging environment.

## 旧便携备份可恢复已删除的 legacy federation controller

**Fingerprint:** `panelbackup.restore.legacy-controller-revocation-resurrection`

**Affected boundary:** internal/ai/backup.go#ExportAccess/RestoreAccess and internal/cluster/backup.go#PruneBackupSecrets/ValidateBackupReferences

**Surface / subsystem:** internal/panel/backup_data.go#AI and cluster credential backup export/import / panel-portable-backup-credential-lifecycle

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 若有有效 Panel 会话的用户导入并恢复一份完整旧便携备份，而其中的 legacy controller 当时仍有效、之后已从同一 Panel 删除，restore 会用旧 cluster-state.json 覆盖当前文件；与 cluster-state-v2.json 不同，它不会合并目标端删除状态。恢复并重启后，仍持有原 Ed25519 私钥的旧 peer 可能通过 v1 summary 接口重新取得 cluster.summary.read。此链条依赖备份导入和恢复操作、相应 Panel 会话，以及旧 peer 仍持有私钥；备份文件本身或任意未配对的远程 peer 不能单独触发恢复。实际恢复、重启和签名请求结果尚未观测。

**源码追踪：**
- `internal/panel/server.go:313` (entrypoint) — GET /api/v1/federation/summary 被路由到 federation handler。
- `internal/panel/cluster.go:515` (propagation) — 将请求交给 cluster.Service.SignedSummary，并在成功时返回 summary。
- `internal/cluster/service.go:823` (propagation) — 在回放检查和读取本机 telemetry 前，先调用 authenticate 验证 legacy controller。
- `internal/cluster/service.go:915` (sink) — legacy 记录通过 scope 检查并以记录中的公钥验签后，其 ID 被作为已认证身份返回；此处没有另行检查删除 tombstone。

**已核对的源码证据：**
- `internal/panel/backup_data.go:33` — 便携备份根目录清单包含 cluster-state.json 和 cluster-state-v2.json。
- `internal/panel/backup_data.go:245` — exportPanelBackup 调用 readPanelBackupFiles 读取便携备份文件。
- `internal/panel/backup_data.go:198` — controller active/revoked 状态筛选仅位于 cluster-state-v2.json 分支；legacy cluster-state.json 不经过该筛选。
- `internal/panel/backup_restore.go:196` — 恢复时的目标端 revocation 合并仅应用于 cluster-state-v2.json。
- `internal/panel/backup_restore.go:215` — 恢复循环将备份中的其他文件（包括 cluster-state.json）原样原子写入目标目录。
- `internal/panel/backup_restore.go:102` — ApplyPanelRestore 仅在 HTTP handlers、stores 和后台 worker 关闭后运行。
- `internal/panel/backup_restore.go:340` — RecoverPanelRestore 在启动前处理恢复日志；pending 状态会调用 ApplyPanelRestore。
- `internal/cluster/store.go:42` — legacy controllerRecord 保存 ID、公钥和 scope，但没有撤销状态或 tombstone 字段。
- `internal/cluster/store.go:373` — legacy DeleteController 从控制器切片中移除记录并持久化，不留下撤销记录。
- `internal/cluster/service.go:764` — legacy 删除成功后立即返回，不进入后续 V2 RevokeController 分支。
- `internal/cluster/service.go:212` — NewService 从 DataDir/cluster-state.json 打开 legacy store，因此重启后恢复的文件会成为认证使用的数据源。
- `internal/panel/server.go:866` — securityEntrancePublicPath 将 /api/v1/federation/ 前缀列为无需浏览器会话的公开路径；该路径仍执行 federation 签名认证。
- `internal/cluster/service.go:898` — 认证从 legacy store 按 controller ID 查记录，并检查其 SummaryScope。
- `internal/cluster/signature.go:73` — v1 请求验签使用传入的 public key，并检查协议、目标 ID、时间戳、nonce 和 Ed25519 签名。
- `internal/cluster/service.go:842` — 通过认证、速率和重放检查并读取 telemetry 后，SignedSummary 返回 FederationSummary。
- `internal/panel/backup_handlers.go:59` — 备份 handler 的操作先要求有效 Panel session。
- `internal/panel/backup_handlers.go:63` — 非 GET 备份操作还要求 Origin 和 CSRF 校验。
- `internal/panel/backup_handlers.go:163` — POST restore 路由到 backupRestore。
- `internal/panel/backup_handlers.go:427` — 备份导入要求 multipart password 字段。
- `internal/panel/backup_handlers.go:581` — 恢复仅接受已导入且 ready 的备份，并要求提交与预览目标修订相符的 revision。
- `internal/panel/backup_handlers.go:663` — Panel 模块恢复会 stagePanelRestore 并请求受控重启。

**精确 blocker：**
- 本轮限定为 source-only，且当前环境没有获批准的 OS-enforced sandbox、外网禁用、空环境白名单和 scratch-only 写入边界；因此未执行目标代码或 fixture。恢复并重启后使用旧 key 发出的单次签名请求是否实际返回 FederationSummary，仍待在隔离本地环境观测。

**有界本地下一步：**
仅在获批准的 OS-enforced sandbox 中验证：使用全新的临时 DataDir、虚构 Panel 用户和 dummy Ed25519 key；仓库源码与工具只读，外网禁用、仅允许 loopback，写入限于该临时目录，并设置固定 CPU、内存、进程、磁盘和时间上限。建立一个有效 legacy controller，导出一次带测试密码的完整 Panel 便携备份；再经有效 dummy Panel session、Origin 和 CSRF 删除该 controller；通过本地备份导入、预览 revision 和 restore 流程只恢复 Panel 模块一次。受控重启并重新打开服务后，仅发送一条带新鲜时间戳和唯一 nonce 的 dummy-key 签名 GET /api/v1/federation/summary，使用确定性本地 telemetry，记录是否返回 FederationSummary 或 ErrAuthentication。记录后停止进程并删除临时 DataDir；不读写主机真实数据、不访问部署环境或非 loopback 地址。

**安全 owner-observed 部署检查：**
本记录未定义部署侧动态检查；不要向部署发送审计流量。

## 会话撤销后既有终端 SSE 流仍可能推送输出

**Fingerprint:** `terminal-sse-output-after-session-revocation`

**Affected boundary:** internal/panel/terminal_stream.go#session revalidation and stream termination

**Surface / subsystem:** GET /api/v1/terminal/stream / internal/panel

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** GET /api/v1/terminal-stream 在建立时验证会话，并把流绑定到 userID 与 token hash。Logout 删除对应 session，但服务端没有按 token 取消已注册的 stream；事件分支可直接写出排队的 output，Authenticate 只在 15 秒 ticker 分支执行。因此源码支持：一个仍打开且已有订阅的 SSE 连接，在同一 token 被注销后、下一次 heartbeat 认证终止该处理器之前，可能继续接收输出。项目 Web 客户端会在当前标签页的 logout 请求完成后 reset 并关闭该标签页的 EventSource；这不直接取消另一标签页或其他已建立客户端的服务端 stream。源码说明了有界心跳检查，但未验证注销后实际送达时间、输出和可见影响。

**源码追踪：**
- `internal/panel/terminal_stream.go:168` (entrypoint) — requireSession 验证初始请求后，:186 按 session.User.ID 和 token hash 注册长连接 stream。请求上下文关闭时才通过 :192 取消该 stream。
- `internal/panel/server.go:964` (propagation) — Origin、当前 session 和 CSRF 校验后，Logout 对本次请求的 token 调用 auth.Logout；此路径清 cookie 并审计，但没有调用 terminalStreams 的逐 token 取消。
- `internal/panel/terminal_stream.go:232` (sink) — 循环接收 stream.events 后在 :233-235 写 output 并 flush；session Authenticate 只在 ticker 分支 :236-245 执行，认证失败才发 auth.expired 并退出。

**已核对的源码证据：**
- `internal/panel/terminal_stream.go:41` — 注释称 heartbeat 限制撤销 session 后停止接收输出的速度，间隔为 15 秒。
- `internal/panel/terminal_stream.go:186` — 已认证的 userID 和当前 token 的 SHA-256 hash 被复制到已注册 stream。
- `internal/panel/terminal_stream.go:192` — 已注册 stream 的取消回调绑定 HTTP 请求上下文；Logout 本身不是该请求，也没有从这里观察 session revoke。
- `internal/panel/terminal_stream.go:232` — 事件分支无 session 检查，直接调用写 SSE output 的函数；重新认证在后续 ticker 分支。
- `internal/panel/server.go:964` — 经过 Origin、requireSession 和 CSRF 校验后，logout handler 只调用 s.auth.Logout(sessionToken)，随后清认证 cookie。
- `internal/auth/service.go:595` — Logout 将 session token hash 交给 store.DeleteSession 删除持久化 session。
- `web/src/stores/session.ts:84` — Web logout 等待 api.auth.logout() 完成后才在 finally 中 resetApiSecurityState。
- `web/src/lib/api.ts:2457` — resetApiSecurityState 调用 terminalStream.reset，作用于当前页面中的共享客户端。
- `web/src/lib/terminalStream.ts:228` — reset 调用 closeSource，关闭本客户端的 EventSource；这不会取消其他已连接客户端的服务器端 stream。

**精确 blocker：**
- 当前环境没有工作流要求的 OS-enforced sandbox；本轮只读源码，未运行目标代码、测试、浏览器或网络请求。
- 尚未观测 store 删除完成后、同 token 的另一条活跃 SSE 连接是否实际收到新的合成 output，以及到 auth.expired/连接关闭的时间；同页 Web logout 在请求完成后会关闭本页 EventSource。
- 需要确认项目对 session logout 的有效性边界是否要求立即终止已建立的输出流，还是接受 heartbeat 检查间隔内的短暂延迟。

**有界本地下一步：**
在满足工作流 OS-enforced sandbox 的本地隔离环境，用 dummy session 建立两条活跃 SSE 客户端并订阅合成终端输出；从一条客户端调用带有效 Origin/CSRF 的 logout，在 session 删除确认点之后继续向合成 terminal backend 写入唯一标记，记录另一条已存在连接是否收到该标记、何时收到 auth.expired 及何时关闭。不得使用真实主机输出或数据。

**安全 owner-observed 部署检查：**
本记录未定义部署侧动态检查；不要向部署发送审计流量。

## 跨源 text/plain 拖拽描述符可借管理员会话发起跨 Panel 文件复制

**Fingerprint:** `web.cross-panel-drag.unbound-text-fallback`

**Affected boundary:** internal/panel/files.go#cross-panel file request and authorization boundary

**Surface / subsystem:** FilesView cross-panel drag fallback / internal/panel

**状态：** `needs_validation`；未评级、未确认。

**源级描述：** 源码确认：目标 Panel 会从通用 text/plain 中读取带固定前缀的跨 Panel 描述符；FilesView 在 drop 后会把其中的 sourceNodeId、路径和 resourceVersion 交给 Panel 页面发起复制。若攻击者能控制一个已配对且获准文件访问的来源节点，并知道其文件的当前路径与版本，伪造描述符可请求目标 Panel 将该文件复制到管理员实际 drop 的目录。服务端仍要求匹配的 Panel origin、有效 Panel session、CSRF、有效目标目录，以及服务端授权的已配对来源节点；V2 来源还须具有 active files scope。这不是未经授权的 API 访问或从目标读取数据，且攻击者不能仅靠描述符指定任意目标 host 或路径。源码无法证明受支持浏览器会把攻击源设置的 text/plain 完整值交给目标 drop handler；候选只覆盖管理员实际 drop 后的效果，不声称攻击者能强制用户拖拽。

**源码追踪：**
- `web/src/lib/desktopFileShortcuts.ts:320` (entrypoint) — 从低信任 DragEvent 的 DataTransfer 读取通用 text/plain，作为跨 Panel fallback 的输入。
- `web/src/views/FilesView.vue:1522` (propagation) — 将解析后的描述符和 drop 目标目录交给 transferCrossPanelFileBatch，并传入 Panel 的 transferFromPanel；没有来源或路径确认步骤。
- `web/src/lib/crossPanelFileTransfer.ts:52` (propagation) — 将描述符内的 sourceNodeId、path、resourceVersion 和 drop 目标目录传入传输函数。
- `web/src/lib/api.ts:784` (sink) — 以 same-origin credentials 向传入的传输路径发起 POST。

**已核对的源码证据：**
- `web/src/lib/desktopFileShortcuts.ts:273` — 注释说明自定义 MIME 在 WebKit 中只对同源页面可见；第 276 行因此将描述符写入 text/plain。注释也指出描述符非秘密，目标 Panel 仍需认证来源节点。
- `web/src/lib/desktopFileShortcuts.ts:295` — hasCrossPanelFileDrag 会把任何不含 Files 但含 text/plain 的拖拽送入跨 Panel drop 分支；实际复制仍须通过后续前缀和 JSON 校验。
- `web/src/lib/desktopFileShortcuts.ts:320` — fallback 读取 text/plain；后续只检查固定前缀、JSON v2、合法 sourceNodeId 和条目字段，并限制大小与数量，没有来源 origin、活动拖拽 token、签名或 expiry 校验。
- `web/src/views/FilesView.vue:1522` — 解析后的 payload 与 drop 目标目录被传给传输批处理函数，没有显示来源或路径确认。
- `web/src/lib/crossPanelFileTransfer.ts:52` — 每个条目的节点 ID、源路径和版本与调用方给定的目标目录一起交给 transferOne。
- `web/src/lib/api.ts:781` — streamFileEntry 从 Panel 页面 csrfToken 设置 X-CSRF-Token header。
- `web/src/lib/api.ts:784` — 传输请求以 same-origin credentials 和 CSRF header 发出。
- `web/src/lib/api.ts:2091` — transferFromPanel 将输入交给 streamFileEntry，并指定 /files/transfers 作为请求路径。
- `internal/panel/files.go:736` — 传输端点检查 Origin，并在第 739–742 行要求有效 Panel session 与 CSRF。
- `internal/panel/files.go:747` — 服务端要求源路径和目标目录符合路径规则，源路径不能是根目录，且 resourceVersion 必须存在。
- `internal/panel/files.go:802` — 服务端查找来源节点并打开 Light 或 V2 文件传输；第 821 行核对返回文件名和 resourceVersion。
- `internal/cluster/file_transfer_v2.go:348` — V2 来源必须处于 active 状态并拥有 files scope，之后使用配对凭据打开来源。
- `internal/panel/files.go:855` — 服务端将来源内容流交给目标文件 Agent 的 transfer/import 接口，目标目录和文件名来自已校验的传输参数。
- `web/src/lib/fileHostContext.ts:24` — fileAPIForHost 将当前文件视图的 hostId 绑定到 transferFromPanel；该 hostId 不来自拖拽描述符，省略时表示当前 Panel。
- `internal/panel/files.go:727` — 请求带目标 hostId 时，服务端重新查找该 host，并要求文件管理可用且类型为 light node 或 Panel。
- `internal/panel/files.go:923` — 本机目标目录必须已存在且确为目录；唯一自动创建的例外是固定桌面上传目录。
- `internal/panel/files.go:1025` — 远程目标目录也必须已存在；唯一自动创建的例外是固定桌面上传目录。

**精确 blocker：**
- 决定性的浏览器事实未观测：受支持的 Chromium/WebKit 是否会将攻击源设置的带前缀 text/plain 暴露给目标页面的 drop 事件，并使 drop handler 获得完整值。源码中的兼容注释不能替代该观察。

**有界本地下一步：**
仅在具备 OS 强制隔离、阻断外网且只允许本地 loopback 服务、只读目标源码和 scratch-only 写入的沙箱中，启动两个 loopback origin 与仅提供虚构文件的已配对 file-scope 来源 fixture。攻击 origin 以一个可见的 draggable 元素设置带 KPanel 前缀的 text/plain 描述符；目标 Panel 使用虚构管理员 session 和临时空目录。测试者显式将该元素 drop 到目标目录，记录 drop 事件可见的 types/getData、解析结果、是否出现确认、是否恰好产生一条本地 /files/transfers POST 及其结果。首次虚构文件落盘即停止并清理；不使用真实凭据或文件，也不访问部署页面或非 loopback 服务。

**安全 owner-observed 部署检查：**
本记录未定义部署侧动态检查；不要向部署发送审计流量。
