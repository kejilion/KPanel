# KPanel 终端与文件传输 v3 设计提案（本机与集群）

- 状态：提案，待评审；未实施，不改变任何现行契约
- 基线：`origin/main@71d50138f9da73999fcaa6046f9b4a860151d2f0`（v1.21.0）
- 范围：本机与远端（完整 KPanel、轻量节点）的交互终端、批量执行、应用/建站/体检/环境任务终端，
  以及文件管理器的浏览、缩略图、上传、下载和跨主机传输
- 风险等级：整体 L3（跨节点协议、信任边界、终端与文件契约、反向代理部署要求均变化）；
  各阶段按第 9 节单独定级
- 影响文档：[多主机终端](multi-host-terminal.md)、[集群监控与联邦协议](cluster-monitoring.md)、
  [架构](architecture.md)、[文件管理器设计](file-manager-design.md)、
  [跨 KPanel 文件传输](cross-kpanel-file-transfer.md)、[部署](deployment.md)、
  [运行时性能基线](runtime-performance-baseline.md)
- 证据性质：第 2 节的延迟、吞吐和 CPU 数字由代码路径推算，**尚未实测**；第 10 节给出实测方案，
  评审通过前必须先补基线数据

## 1. 摘要

终端和文件管理慢，主因不是语言、框架或加密算法，而是传输模型。两段链路各有问题：

- **浏览器到面板**（本机和远端都经过）：终端靠"收到响应后再发下一次"的长轮询，服务端无法推送；
  明文 HTTP/1.1 下同源只有 6 条连接；缩略图不缓存且每次在 Agent 上完整解码重算；上传按文件串行。
- **面板到远端**：每次按键、每次输出轮询、每个 32 KiB 文件块都是一次独立 HTTP 请求，
  并重新执行 Noise IK 握手；没有流控，只能靠请求计数限流和固定小块兜底。

本提案保留 Go、Vue、xterm.js、Noise 静态身份、scope 权限模型、Agent Unix Socket API 和
"不新增监听端口"原则，只做三件事：

1. **统一后端抽象**：面板内部为终端和文件定义流式后端接口，本机、完整 KPanel、轻量节点、
   v2 兼容路径各有一个实现；浏览器接口与主机类型无关；
2. **浏览器侧推送**：一个标签页一条 SSE 流承载全部终端输出（含任务终端），输入沿用现有 POST；
   文件侧补缓存、并发和批量接口；
3. **面板间长会话**：面板之间、中心与轻量节点之间，按需建立一条长期 Noise 会话，承载在现有端口的
   WebSocket 上，按封闭的流类型多路复用；大文件使用独立数据连接；能力协商启用，失败逐主机回退 v2。

**本机先行**：前两项不涉及跨节点协议，先在本机落地并验证浏览器契约，再把远端后端接入同一接口。

可复用的现有基础：轻量节点文件流 `GET /api/v2/federation/files/stream`
（WebSocket + Noise IK + 独立 CipherState 分帧，`internal/cluster/file_stream*.go`）、
浏览器侧 SSE（`internal/panel/ai_handlers.go`）、Agent 文件流式代理（`OpenStream` + 64 KiB `io.CopyBuffer`）。

## 2. 现状与问题

### 2.1 链路

```text
本机：浏览器 ──HTTP/1.1 长轮询 + POST──▶ paneld ──Unix Socket 长轮询──▶ Agent/PTY
远端：浏览器 ──同上──▶ 中心 paneld ──每请求一次 Noise IK──▶ 目标 paneld ──Unix Socket──▶ Agent/PTY
轻量：root broker ──串行 HTTPS 长轮询（v2 Noise）──▶ 中心 paneld
轻量文件：控制 WebSocket + 按请求回拨的数据 WebSocket（已是流式）
任务终端（应用/建站/体检/环境）：浏览器 ──长轮询（wait=1000）+ POST──▶ paneld ──▶ Agent
```

### 2.2 浏览器到面板（本机与远端共同）

| # | 问题 | 代码证据 | 推算影响 |
|---|---|---|---|
| B1 | 终端输出靠长轮询：每次响应后浏览器才发下一次，服务端有字节即返回、不做合并 | `web/src/components/terminal/HostTerminal.vue` `poll()`、`internal/terminal/manager.go` `Output()` | 持续输出按"每个浏览器 RTT 一块（≤64 KiB）"推进；RTT 100 ms 时约 640 KiB/s，且产生大量小响应 |
| B2 | 任务终端采用同一模式，各自一条长轮询 | `web/src/lib/api.ts` 各任务 `terminal()`（`wait: 1000, inputOpen`）、`web/src/components/apps/AppInteractiveTerminal.vue` | 与 B1 相同；同时打开多个任务窗口时占用更多连接 |
| B3 | paneld 明文 HTTP/1.1，浏览器同源最多 6 条连接 | `cmd/paneld/main.go` `server.Serve` | 多个终端 + 批量执行 + 缩略图 + 监控轮询相互排队；批量 4 台即占满大部分连接 |
| B4 | 缩略图返回 `private, no-store`；Agent 每次完整解码源图（≤12 MiB）再做双线性缩放，无服务端缓存，并发 2 | `internal/panel/files.go` 内容代理、`internal/agent/files.go` `makeFileThumbnail`、`internal/agent/server.go` `thumbnailGate` | 每次进入图片目录都重算全部缩略图；1 核主机 100 张照片约数秒到十几秒 CPU，超时后显示失败 |
| B5 | 上传按文件严格串行；目录拖入时逐个创建子目录，文件 2 路并发 | `web/src/views/FilesView.vue` `uploadFiles()`、`web/src/lib/desktopExternalDrop.ts` | 大量小文件时耗时约等于"文件数 × 每文件往返"；远端时每个文件还要走 R1 的多轮往返 |
| B6 | 单个上传是一次性请求（≤512 MiB），不能续传 | `internal/panel/files.go` `handleFileUpload` | 网络抖动时大文件从头重传 |
| B7 | 批量执行以"3 秒无输出"判定完成 | [多主机终端](multi-host-terminal.md) 第 4 节 | 任意命令每台 ≥3 s |

已经做得好的部分：本机下载与上传已是流式直通；JSON 响应 ≥1 KiB 时 gzip；会话校验只查内存；
面板到 Agent 的 Unix Socket 往返可忽略；上传冲突在读取正文前即判定。

### 2.3 面板到远端

| # | 问题 | 代码证据 | 推算影响（中心↔目标 RTT 150 ms） |
|---|---|---|---|
| R1 | 完整 KPanel 远端文件管理每轮 poll 只返回 1 个事件，每轮最多 32 KiB | `internal/cluster/panel_file_relay.go` `collect()`、`readFileRelayBodyChunk()` | 列目录 ≥4 次串行往返约 600 ms；上传/下载约 210 KiB/s，与带宽无关 |
| R2 | 终端、文件流、摘要共用 `MaxConnsPerHost: 2` 的 transport；`http://IP` 来源无 HTTP/2 复用 | `internal/cluster/client.go` `NewRemoteClient` | 同主机文件传输或缩略图加载期间，终端输入可排队至 1 s |
| R3 | 轻量终端 broker 串行轮询，中心无命令时挂起 750 ms；PTY 回显通常晚于 poll 发出 | `internal/cluster/terminal_relay_protocol.go`、`internal/cluster/light_terminal.go`、`cmd/kejilion-node/light_terminal.go` | 回显额外 0–750 ms；持续输出约每 750 ms 一批 |
| R4 | 远端终端输出轮询与目标端 600 次/分钟终端限流冲突 | `internal/cluster/service.go` `terminalRequests` | 持续输出时易触发 429，前端退避 0.5–4 s 并显示"重连中" |

非瓶颈：Noise 握手 CPU（每请求约 4 次 X25519）、每请求读取凭据文件（页缓存命中）、Agent 本地目录读取。

### 2.4 一个关键历史事实

v1.16.0 曾让普通 KPanel v2 也使用 WebSocket 文件流（`fed7c87e`）。v1.16.0 被整体回滚后，
`40d6fa67 fix(cluster): restore legacy Panel file transport` 把普通 Panel 恢复为 POST relay，
只保留轻量节点的流式实现；`docs/cluster-monitoring.md` 由此写入"普通 Panel 不发起文件 WebSocket"。
回滚记录标注为用户主动撤回、非产品故障，但**普通 Panel 放弃流式的具体原因未入库**；
`fed7c87e` 的提交说明写明"Production CDN and reverse-proxy behavior remains unverified"。

结果是：完整 KPanel 之间的远端文件管理，目前反而比轻量节点慢一个数量级。阶段 4 之前必须回答
开放问题 Q1，并把反向代理/CDN 兼容作为一等验收项。

## 3. 目标与非目标

### 3.1 可度量目标（无竞争、无丢包条件下）

| 指标 | 现状推算 | 目标 |
|---|---|---|
| 本机终端回显 | ≈ 1 × RTT(浏览器↔面板)，连接竞争时更高 | ≈ 1 × RTT(浏览器↔面板) + ≤5 ms，竞争下 p95 增量 ≤ 30 ms |
| 远端终端回显 | 约 RTT(浏览器↔中心) + RTT(中心↔目标)，竞争时 +≤1 s | 同左，竞争下 p95 增量 ≤ 30 ms |
| 轻量节点终端回显 | 上式 + 0–750 ms | 与完整 KPanel 相同 |
| 持续输出（本机与远端） | 每 RTT 一块；远端受 10 次/秒限流 | 吞吐受带宽限制；不出现 429 与"重连中" |
| 远端列目录（100 项） | ≥4 × RTT(中心↔目标) | ≤1 × RTT(中心↔目标) + Agent 耗时 |
| 远端上传/下载 | ≈ 32 KiB / RTT | ≥ 链路带宽的 70%，150 ms RTT 下至少为现状 8 倍 |
| 重复进入图片目录 | 全部缩略图重算、重传 | 浏览器零请求；Agent 冷缓存时 1 核 100 张 ≤ 现状 50% CPU |
| 1000 个小文件上传 | ≈ 1000 × 每文件往返 | ≤ 现状 30% 耗时（本机与远端分别测） |
| 同主机"传文件 + 敲终端" | 终端被阻塞 | 终端 p95 增量 ≤ 30 ms |
| 多终端 + 批量 + 文件同时使用 | 受 6 连接限制排队 | 终端输出只占 1 条连接/标签页 |
| 资源（1C1G 中心，100 台主机空闲） | Panel 稳态 RSS 约 15–17 MiB | 新增常驻 ≤ 8 MiB；无活动主机不保持连接 |

### 3.2 非目标

- 不改变 scope、配对、撤权、审计的权限语义；不扩大终端或文件能力；
- 不新增监听端口，不引入 SSH 服务、UDP 或 QUIC，paneld 不内置 TLS；
- 不开放通用 TCP 转发、任意 HTTP 代理或新的任意 Shell API；
- 不让浏览器直连远端，也不跨主机共享登录态；
- **不改 Agent Unix Socket API** 和 PTY Manager 语义（缓冲、闲置回收、最长会话保持不变）；
  本机推送由面板内部把 Agent 长轮询转换为推送实现；
- 不把终端变成后台任务；断线重连仍依赖目标端内存环形缓冲。

## 4. 方案比较

| 方案 | 做法 | 收益上限 | 主要问题 | 结论 |
|---|---|---|---|---|
| A 现状调优 | 批量带回事件、拆连接池、调限流、轮询合并 | 远端列目录降到 1–2 个 RTT；本机收益很小 | 每操作仍 ≥1 RTT + 握手；无推送、无流控 | 只做与 v3 不冲突的部分（阶段 0） |
| B 每请求一条 WebSocket | 恢复 v1.16.0 的普通 Panel 文件流 | 大文件带宽受限 | 短请求需 TCP + TLS + Upgrade + Noise + 请求约 4–5 个 RTT，比现状还慢；终端不适用 | 只用于大文件数据连接 |
| **C 统一后端 + 浏览器推送 + 长会话多路复用** | 本机与远端共用浏览器契约；面板间一条 Noise 会话内含封闭类型的流 | 回显 ≈ 纯 RTT；短请求 1 RTT；推送与流控 | 新协议面、长期会话密钥、代理兼容、兼容旧节点 | **采用**，与 B 组合 |
| D SSH 隧道 | `x/crypto/ssh` + `pkg/sftp` 经现有端口隧道 | 通道、PTY、流控、SFTP 流水线现成 | exec/subsystem 语义需逐项锁死，与 scope 模型错位；新增大依赖面；不解决浏览器侧 | 备选，不采用 |
| E QUIC/WebTransport | quic-go | 无队头阻塞、0-RTT | 需 UDP 端口，常被封；违反不新增端口原则 | 不采用 |
| F gRPC 双向流 | h2/h2c | 流与流控现成 | 依赖重；`http://IP` 需 h2c；与现有 Noise 身份叠加复杂 | 不采用 |
| G 浏览器 WebSocket | 浏览器与面板间 WebSocket 承载输入输出 | 输入也免去逐次 HTTP 开销 | 新增浏览器侧长连接协议与 CSRF 模型变化 | 暂不采用，SSE 不足时单独提案（Q4） |

## 5. 统一后端抽象

面板内部定义与主机类型无关的接口，浏览器接口只依赖它：

```text
TerminalBackend
  Open(rows, cols)            → sessionID, offset
  Attach(sessionID, offset)   → 输出块流（offset 标记、截断标记、退出事件）
  Input / Resize / Close

FileBackend
  Request(受限文件 HTTP 契约) → 流式响应（沿用现有 method、固定路径、query、白名单头、长度）
```

| 实现 | 传输 | 说明 |
|---|---|---|
| 本机 | Agent Unix Socket | Attach 由面板内一个 goroutine 持续调用现有 `/v1/terminals/{id}/output?wait=` 转成推送；文件沿用 `OpenStream`。**Agent 零改动** |
| 完整 KPanel v3 | 第 7 节长会话 | 终端流与短文件请求走会话；大文件走独立数据连接 |
| 完整 KPanel v2 兼容 | 现有 POST + Noise | Attach 在中心内部长轮询远端并转成推送；浏览器侧同样享受 SSE 聚合 |
| 轻量节点 | 现有控制连接 + 回拨数据连接 | 阶段 3 新增终端流类型后，终端也走流式 |
| 任务终端 | Agent 现有任务终端接口 | 实现同一 Attach 语义，并入同一 SSE 流 |

收益：前端只有一条代码路径；本机成为参考实现和首个上线目标；远端后端逐个替换时浏览器无感知。

## 6. 浏览器到面板

### 6.1 终端输出聚合推送（SSE）

- 新增 `GET /api/v1/terminal-stream`，返回 `text/event-stream`。**一个标签页一条流**，承载该管理员在
  本标签页打开的全部终端输出：主机终端、批量执行终端和任务终端。
- 订阅集合通过已认证 POST 维护（`/api/v1/terminal-stream/subscriptions`，带 CSRF/Origin），
  不把会话 ID 列表放进 URL；流本身只携带随机 stream ID。
- 事件：`output`（会话 ID、offset、数据）、`truncated`、`exit`（真实退出码）、`closed`、`heartbeat`（15 秒）。
  输出最多 5 ms 或 32 KiB 合并一次，二者先到先发。
- 背压：每会话在面板侧最多暂存 256 KiB；浏览器消费慢时停止从后端拉取，由后端环形缓冲吸收，
  超出后按现有语义标记截断。
- 断线：浏览器 `EventSource` 重连时携带每会话最后 offset，从后端缓冲续读。
- 鉴权：沿用 Session 与 Origin 校验；注销、Session 过期、会话关闭时服务端主动结束流，并定期重新校验 Session。
- 代理：设置 `Cache-Control: no-store` 与 `X-Accel-Buffering: no`；检测到缓冲（首个心跳超时）时
  回退到现有 `output?wait=` 长轮询。
- 桌面模式：非活动窗口不再停止轮询，改为仍订阅但只缓存，窗口激活时再写入 xterm，避免切回窗口时集中补数据。

### 6.2 输入

- 保持现有 POST 接口与 CSRF/Origin 校验，仍按 12 ms 合并与 UTF-8 边界排队。
- 面板收到后直接写入对应后端：本机进 Agent，远端写入已建立的会话流，不再握手。

### 6.3 文件管理

| 项目 | 做法 | 安全考量 |
|---|---|---|
| 缩略图浏览器缓存 | URL 已绑定 `resourceVersion`，改为 `private, max-age=86400`，只对 `mode=thumbnail` 生效 | 缩略图会留在浏览器磁盘缓存；需评审共享电脑场景，可提供"禁用缓存"开关（Q7） |
| 缩略图服务端缓存 | Agent 按 `(路径, resourceVersion)` 维护有界内存 LRU（建议 16 MiB）；JPEG 优先读取 EXIF 内嵌缩略图 | 缓存只在 Agent 进程内，不落盘；版本变化即失效 |
| 缩略图请求合并 | 前端按可视区域批量请求，限制每主机并发 | — |
| 上传并发 | 普通多选上传改为有界并发（本机 4、远端按后端能力），与目录拖入共用调度器 | 沿用单文件 512 MiB 与现有审计 |
| 目录批量创建 | 新增一次请求创建多级目录清单的文件动作，替代逐个创建 | 沿用路径校验、保护目录和数量上限 |
| 可续传上传 | 大于 64 MiB 的文件按分段上传到隐藏临时文件，完成后 no-replace 原子发布；断点按已确认长度续传 | 临时文件纳入现有配额与清理；单独评审（阶段 5） |

### 6.4 批量执行完成判定

以唯一随机结束标记加退出码取代"3 秒无输出"：命令后追加只输出标记与 `$?` 的固定尾句，
前端在输出中匹配标记即判定完成；子 shell 或交互程序未返回时仍按现有 4 小时上限与关闭流程处理。

## 7. 面板到远端：长会话协议

### 7.1 会话（Channel）

- 端点：`GET /api/v3/federation/channel`，WebSocket 子协议 `kpanel-channel-v3`，复用现有端口和精确路径校验，
  拒绝查询参数与 `RawPath` 变体。
- 方向：完整 KPanel 由中心主动拨号目标，沿用现有 DNS/IP 允许清单、rebinding 检查、TLS 验证和禁止重定向；
  轻量节点由节点主动拨号中心（沿用文件流控制连接模式），节点仍不监听任何端口。
- 认证：101 之后先完成 Noise `IK_25519_ChaChaPoly_SHA256`，使用现有静态密钥（完整 Panel 用 v2 host credential，
  轻量节点用终端 Noise 密钥）。prologue 绑定协议版本、路径、双方 ID、时间戳和随机 channel ID；
  时间戳窗口与现行 v2 一致。认证完成前只允许握手帧。
- 加密：两个独立 CipherState；每帧 AEAD，使用隐式递增 nonce，会话内重放、乱序、截断都会导致解密失败并断开。
  发送量达到 1 GiB 或满 1 小时执行 Noise `Rekey`；会话最长 24 小时后强制重建。
- 按需建立：首次打开终端或文件操作时建立，无活动流 5 分钟后关闭；只做监控的主机不保持连接。
- 心跳：15 秒 ping/pong，45 秒无响应断开（与现有文件流一致，低于常见 CDN 的 100 秒空闲上限）。
- 撤权：删除 Host/Controller 或 scope 变化时立即关闭该身份的全部会话（沿用 `fileStreamHub.closePeer`）。

### 7.2 帧与多路复用

Noise 明文内的帧格式：

```text
stream_id u32 | kind u8 | flags u8 | length u16 | payload[length]
kind: OPEN / ACCEPT / REJECT / DATA / WINDOW / CONTROL / CLOSE / RESET / PING / PONG
```

- 基于额度（credit）的流量控制：每流初始窗口 256 KiB，接收方消费后发 `WINDOW`；每会话总窗口 1 MiB。
  背压直接传到 PTY 读取与文件读写，不在中心无界缓冲。
- 复用库待定（Q2）：`hashicorp/yamux`（MPL-2.0，与 AGPL-3.0 兼容，但属新依赖，需走依赖准入五项证据），
  或自研上述约 300–500 行的最小帧层。倾向自研：帧类型封闭，便于逐字段严格校验和模糊测试。

### 7.3 流类型（封闭枚举）

`OPEN` 负载为严格 JSON，只接受以下类型；未知类型、未知字段、越界参数直接 `REJECT`。
**每次开流都按当前存储重新校验 scope**，不只在会话建立时校验。

| 类型 | 所需 scope | 负载 | 语义 |
|---|---|---|---|
| `terminal.open` | `cluster.terminal.open` | rows、columns | 创建 PTY，返回会话 ID 与起始 offset，随后推送输出 |
| `terminal.attach` | `cluster.terminal.open` | 会话 ID、offset | 断线或回退后从 offset 继续；超出缓冲时标记截断 |
| `file.request` | `cluster.files.read`（写操作沿用现有文件权限判定） | 现有受限文件 HTTP 契约 | 复用 `serveStreamRequest`；只用于元数据、目录、文本、缩略图等短请求 |
| `transfer.export` | linked grant 规则不变 | 与 `/files/open-linked` 相同 | 阶段 4 再评估，可继续走现有 HTTP 流 |

终端流内的 `CONTROL` 子消息：`input`（≤16 KiB）、`resize`、`close`、`exit`。输出合并规则与 6.1 相同。

### 7.4 大文件与队头阻塞

同一 TCP 上多路复用在丢包时仍有队头阻塞。为保证"传文件时终端不卡"：

- 预计超过 1 MiB 的下载、任何上传、目录 TAR/ZIP、跨节点导入导出走**独立数据 WebSocket**
  （恢复并收敛 v1.16.0 的普通 Panel 文件流；轻量节点沿用"控制连接下发请求 ID、节点回拨"）；
- 会话内只跑终端和短文件请求；
- 现有限额继续适用：未认证握手 64、已认证连接 128、每身份 10；文件流大文件与短请求各全局 16、每身份 4。

### 7.5 能力协商与回退

- 目标端在已认证的摘要响应和 v2 能力头中声明 `channel-v3`；中心只在双方都声明时尝试。
- 101 之前收到 404/405/426，或代理返回 400/502/503：该主机标记为兼容模式，10 分钟后再探测，
  主机列表显示"兼容传输"。
- 101 之后认证失败：记为认证错误，**不**回退，与现行规则一致。
- 101 之后断流或超时：终端通过 `terminal.attach` 或 v2 `output` 按 offset 继续
  （目标端 PTY Manager 与 owner 相同，两种传输可读同一会话）；已开始的写操作不自动重放，交给用户重试。
- 轻量节点：中心不支持 v3 时继续使用现有 relay 与文件流；节点回滚后中心自动识别为旧能力。
- 版本回滚：旧版本忽略 `channel-v3` 能力与新浏览器接口，不需要数据迁移。

## 8. 安全、资源与审计

### 8.1 安全分析

| 变化 | 风险 | 控制 |
|---|---|---|
| 浏览器 SSE 长连接 | 登录态失效后流未关闭；会话 ID 泄露到 URL 或日志 | 服务端主动结束流并定期复核 Session；订阅通过带 CSRF 的 POST，URL 只有随机 stream ID |
| 缩略图可缓存 | 浏览器磁盘残留图片缩略图 | 仅 `mode=thumbnail`、`private`、有期限；提供禁用开关；评审后决定默认值（Q7） |
| Agent 缩略图缓存 | 内存占用、跨版本泄露 | 有界 LRU，按 resourceVersion 键控，不落盘 |
| 每请求握手 → 长期会话 | 会话密钥暴露窗口变长 | 定期 Rekey、最长 24 小时、撤权立即断开、开流逐次校验 scope |
| 时间窗 + request ID 防重放 → 会话内隐式 nonce | 跨会话重放 | 每个会话使用新的临时密钥；握手时间窗与 channel ID 唯一性检查保留 |
| 新的长连接端点 | 慢速攻击、连接耗尽 | 握手 8 秒超时，沿用未认证/已认证/每身份连接上限；认证前只收握手帧 |
| 流多路复用 | 借流类型扩权、通用转发 | 流类型封闭枚举；负载严格 JSON；不提供任意目标或端口参数 |
| 批量结束标记 | 输出伪造标记导致误判 | 标记为每次随机 128 位值，只在本次命令后注入 |
| 反向代理/CDN | 缓冲导致延迟，或 Upgrade 被拦截 | SSE 缓冲检测后回退；Upgrade 失败走回退；部署文档补代理配置 |

### 8.2 资源预算

| 项目 | 预算 |
|---|---|
| SSE 流 | 每标签页 1 条；每会话暂存 ≤256 KiB；每用户最多 4 个终端不变 |
| 本机推送转换 | 每个已订阅会话 1 个 goroutine 持有 Agent 长轮询；无订阅即停止 |
| Agent 缩略图缓存 | ≤16 MiB，可配置；低内存主机可关闭 |
| 远端空闲会话 | 每条 ≤64 KiB + 3 个 goroutine；按需建立、5 分钟闲置关闭 |
| 远端终端窗口 | 每流 256 KiB；16 个全局会话上限下最坏 4 MiB |
| 限流 | 远端终端请求计数限流改为每流按字节和帧计的预算；保留握手与开流速率限制 |
| CPU | 远端握手从"每按键/每轮询"降为"每会话"；缩略图重算只在冷缓存时发生 |

### 8.3 审计

记录会话建立/关闭、流打开/拒绝/关闭及结果、订阅变化、目标主机和管理员；
不记录输入、输出、文件内容或密钥。新增信任边界包的阶段按 `PROJECT_RULES.md` 5.4 执行
`security-boundary-audit`（profile=scoped）。

## 9. 分阶段计划

每个阶段都可以独立发布、独立回滚。

| 阶段 | 内容 | 覆盖 | 协议/契约变化 | 预估等级 |
|---|---|---|---|---|
| 0 过渡与基线 | ① 基准测试工具（第 10 节）；② 缩略图浏览器缓存与 Agent LRU；③ 上传有界并发与目录批量创建；④ 前端把 429 与断线分开处理；⑤ 轻量 relay 在请求带输出事件时立即返回（或 ≤20 ms），broker 执行命令后短暂等待回显 | 本机 + 远端 | 无跨节点协议变化；缓存头需安全评审 | L1–L2 |
| 1 本机推送 | 统一后端抽象 + 本机实现；终端 SSE 聚合流覆盖主机终端与批量终端 | 本机 | 新浏览器接口 | L2 |
| 2 任务终端与远端 v2 适配 | 任务终端并入 SSE；v2 远端后端在中心内部转推送；批量执行结束标记 | 本机 + 远端 | 新浏览器接口扩展 | L2 |
| 3 轻量终端上流 | 复用轻量文件流控制连接与回拨机制，新增终端流类型 | 轻量节点 | 轻量协议新增流类型 | L3 |
| 4 完整 Panel 长会话 | `channel-v3`：终端和短文件请求走会话，大文件走独立数据 WebSocket | 完整 KPanel | 新端点与能力 | L3，先关闭 Q1 |
| 5 可选 | 可续传上传；摘要 RPC 走会话 | 本机 + 远端 | 新文件接口 | 单独评审 |

阶段 0 中，"`collect` 批量带回事件、拆分连接池、调整终端限流额度"不单独实施，由阶段 4 整体替代。
阶段 1–2 完成后，本机与 v2 远端在浏览器侧已获得推送与连接聚合收益，远端剩余延迟只来自面板间传输。

## 10. 验证方案

### 10.1 基准测试矩阵

在隔离环境（WSL Docker 或已登记的 arena 目标，禁止 `prod-108`/`108`）用 `tc netem` 注入条件：

- 浏览器↔面板 RTT：5 / 50 / 150 ms；中心↔目标 RTT：20 / 80 / 150 / 300 ms；丢包 0% / 1%；带宽 10 / 100 Mbps；
- 拓扑：本机、中心→完整 KPanel（HTTPS 域名、`http://IP`）、中心←轻量节点；
- 主机规格：1C1G 与 2C4G；
- 反向代理：直连、Nginx、Caddy、Cloudflare 代理（验证 Upgrade、100 秒空闲、SSE 缓冲）。

### 10.2 指标

| 场景 | 指标 |
|---|---|
| 回显 | 本机/远端/轻量单字符往返 p50/p95/p99（浏览器侧打点） |
| 持续输出 | `yes \| head -c 50M` 吞吐、429 次数、"重连中"次数 |
| 连接竞争 | 4 个终端 + 批量 4 台 + 缩略图目录同时打开时的回显 p95 |
| 列目录 | 100 项、500 项目录的首字节与完成时间 |
| 缩略图 | 100 张照片目录首次与再次进入的请求数、Agent CPU 时间、失败数 |
| 上传/下载 | 100 MiB 单文件吞吐；1000 个 4 KiB 小文件耗时；传输期间终端回显 p95 |
| 批量执行 | 4 台、16 台 `uptime` 完成时间 |
| 资源 | Panel/Agent RSS、goroutine、fd、CPU（对照 [运行时性能基线](runtime-performance-baseline.md)） |

改造前后使用同一工具、同一环境对比。证据只对精确提交、环境与参数有效。

### 10.3 回归与兼容

- 浏览器：Chrome/Edge/Safari，桌面与移动端；SSE 被代理缓冲时回退长轮询；标签页休眠与恢复；
- 新旧组合：新前端 × 旧面板不适用（同版本发布）；v3 中心 × v2 目标、v2 中心 × v3 目标、v3 中心 × 旧轻量节点、节点回滚；
- 撤权后会话立即断开、新流被拒；scope 变化后开流逐次校验；注销后 SSE 立即结束；
- 帧篡改、截断、重放、超长帧、未知流类型、窗口溢出的模糊测试；
- 断线后按 offset 恢复、截断标记、会话 8 小时上限与 30 分钟闲置回收不变；
- 任务终端的预输入框、`inputOpen` 状态与失败语义不变。

## 11. 开放问题

| # | 问题 | 为什么重要 |
|---|---|---|
| Q1 | v1.16.0 回滚后，普通 Panel 恢复 POST relay 的具体技术原因是什么？ | 若是代理/CDN 兼容问题，阶段 4 的回退策略必须覆盖该场景 |
| Q2 | 多路复用自研最小帧层，还是引入 `hashicorp/yamux`？ | 依赖准入与长期维护成本 |
| Q3 | [多主机终端](multi-host-terminal.md) 中"不开放新的 TCP、SSH、WebSocket 或 Agent 公网监听端口"是否只约束新端口？ | 决定现有端口上的 WebSocket 会话是否需要修订该契约 |
| Q4 | 浏览器侧是否只允许 SSE，还是也接受 WebSocket？ | 影响输入路径开销和阶段 1 设计 |
| Q5 | v3 支持的最低对端版本和兼容期多长？ | 决定 v2 路径何时可以冻结 |
| Q6 | 用户常见部署是否普遍在 KPanel 前接 CDN 或反向代理？ | 决定代理兼容测试优先级与默认回退行为 |
| Q7 | 缩略图是否允许浏览器缓存，默认开启还是关闭？ | 性能与共享电脑隐私的取舍 |

## 12. 采纳后需要更新的文档

- [多主机终端](multi-host-terminal.md)：链路、浏览器接口、资源上限、批量完成判定；
- [集群监控与联邦协议](cluster-monitoring.md)：v3 端点、能力协商、"普通 Panel 不发起文件 WebSocket"条款；
- [文件管理器设计](file-manager-design.md)：缩略图缓存、上传并发、目录批量创建、可续传上传；
- [架构](architecture.md)：集群监控边界与浏览器推送；
- [部署](deployment.md)：反向代理的 Upgrade、空闲超时和 SSE 缓冲配置；
- [跨 KPanel 文件传输](cross-kpanel-file-transfer.md)：大文件数据连接；
- [运行时性能基线](runtime-performance-baseline.md)：补充终端、文件场景的实测基线。
