# KPanel 多主机传输 v3 设计提案

- 状态：提案，待评审；未实施，不改变任何现行契约
- 基线：`origin/main@71d50138f9da73999fcaa6046f9b4a860151d2f0`（v1.21.0）
- 风险等级：L3（跨节点协议、信任边界、终端与文件契约、反向代理部署要求均变化）
- 影响文档：[多主机终端](multi-host-terminal.md)、[集群监控与联邦协议](cluster-monitoring.md)、
  [架构](architecture.md)、[跨 KPanel 文件传输](cross-kpanel-file-transfer.md)、[部署](deployment.md)
- 证据性质：第 2 节的延迟与吞吐数字由代码路径推算，**尚未实测**；第 9 节给出实测方案，评审通过前必须先补基线数据

## 1. 摘要

多主机终端、远端文件管理体验慢的主因不是语言、框架或加密算法，而是传输模型：
远端的每次按键、每次输出轮询、每个 32 KiB 文件块都是一次独立 HTTP 请求并重新执行 Noise IK 握手，
服务端无法主动推送，也没有流控，只能用请求计数限流和固定小块兜底。

本提案保留 Go、Vue、xterm.js、Noise 静态身份、scope 权限模型和"不新增监听端口"原则，
只替换传输层：

1. 面板之间、中心与轻量节点之间，按需建立一条**长期 Noise 会话**，承载在现有端口的 WebSocket 上；
2. 会话内按**封闭的流类型**多路复用：终端流、文件短请求流；大文件继续使用独立数据连接；
3. 浏览器到中心的终端输出改为 **SSE 推送**，输入仍用现有 POST；
4. 双方通过能力协商启用；升级失败逐主机回退到现行 v2 协议，旧节点不受影响。

仓库已有可复用的基础：轻量节点文件流 `GET /api/v2/federation/files/stream`
（WebSocket + Noise IK + 独立 CipherState 分帧，见 `internal/cluster/file_stream*.go`），
以及浏览器侧的 SSE 先例（`internal/panel/ai_handlers.go`）。

## 2. 现状与问题

### 2.1 链路

```text
浏览器 ──HTTP/1.1 长轮询 + POST──▶ 中心 paneld ──每请求一次 Noise IK──▶ 目标 paneld ──Unix Socket──▶ Agent/PTY
轻量节点：root broker ──串行 HTTPS 长轮询（v2 Noise）──▶ 中心 paneld
轻量节点文件：控制 WebSocket + 按请求回拨的数据 WebSocket（已是流式）
```

### 2.2 已定位的问题

| # | 问题 | 代码证据 | 推算影响（中心↔目标 RTT 150 ms） |
|---|---|---|---|
| P1 | 完整 KPanel 远端文件管理每轮 poll 只返回 1 个事件，每轮最多 32 KiB | `internal/cluster/panel_file_relay.go` `collect()`、`readFileRelayBodyChunk()` | 列目录 ≥4 次串行往返约 600 ms；上传/下载约 210 KiB/s，与带宽无关 |
| P2 | 终端、文件流、摘要共用 `MaxConnsPerHost: 2` 的 transport；`http://IP` 来源无 HTTP/2 复用 | `internal/cluster/client.go` `NewRemoteClient` | 同主机文件传输或缩略图加载期间，终端输入可排队至 1 s |
| P3 | 轻量终端 broker 串行轮询，中心在无命令时挂起 750 ms；PTY 回显通常晚于 poll 发出 | `internal/cluster/terminal_relay_protocol.go`、`internal/cluster/light_terminal.go`、`cmd/kejilion-node/light_terminal.go` | 回显额外 0–750 ms；持续输出约每 750 ms 一批 |
| P4 | 前端输出轮询零间隔、服务端有字节即返回，与目标端 600 次/分钟终端限流冲突 | `web/src/components/terminal/HostTerminal.vue` `poll()`、`internal/terminal/manager.go` `Output()`、`internal/cluster/service.go` | 持续输出时易触发 429，前端退避 0.5–4 s 并显示"重连中" |
| P5 | 带 version 的缩略图返回 `private, no-store` | `internal/panel/files.go` 文件内容代理 | 回到同一目录重复拉取；远端每张图都要走完整 P1 链路 |
| P6 | paneld 明文 HTTP/1.1，浏览器同源最多 6 连接 | `cmd/paneld/main.go` | 批量执行 4 台时 4 条长轮询占满大部分连接 |
| P7 | 批量执行以"3 秒无输出"判定完成 | [多主机终端](multi-host-terminal.md) 第 4 节 | 任意命令每台 ≥3 s |

非瓶颈：Noise 握手 CPU（每请求约 4 次 X25519）、每请求读取凭据文件（页缓存命中）、Agent 本地目录读取。

### 2.3 一个关键历史事实

v1.16.0 曾让普通 KPanel v2 也使用上述 WebSocket 文件流（`fed7c87e`）。v1.16.0 被整体回滚后，
`40d6fa67 fix(cluster): restore legacy Panel file transport` 把普通 Panel 恢复为 POST relay，
只保留轻量节点的流式实现；`docs/cluster-monitoring.md` 由此写入"普通 Panel 不发起文件 WebSocket"。
回滚记录标注为用户主动撤回、非产品故障，但**普通 Panel 放弃流式的具体原因未入库**；
`fed7c87e` 的提交说明写明"Production CDN and reverse-proxy behavior remains unverified"。

结果是：完整 KPanel 之间的远端文件管理，目前反而比轻量节点慢一个数量级。本提案必须先回答
开放问题 Q1，并把反向代理/CDN 兼容作为一等验收项，而不是事后补测。

## 3. 目标与非目标

### 3.1 可度量目标（无竞争、无丢包条件下）

| 指标 | 现状推算 | 目标 |
|---|---|---|
| 完整 KPanel 终端回显 | 约 RTT(浏览器↔中心) + RTT(中心↔目标)，竞争时 +≤1 s | 同左，且竞争下 p95 增量 ≤ 30 ms |
| 轻量节点终端回显 | 上式 + 0–750 ms | 与完整 KPanel 相同 |
| 持续输出 | 受 10 次/秒限流，出现"重连中" | 不出现 429；吞吐受带宽限制 |
| 远端列目录（100 项） | ≥4 × RTT(中心↔目标) | ≤1 × RTT(中心↔目标) + Agent 耗时 |
| 远端上传/下载 | ≈ 32 KiB / RTT | ≥ 链路带宽的 70%，150 ms RTT 下至少为现状 8 倍 |
| 同主机"传文件 + 敲终端" | 终端被阻塞 | 终端延迟不受文件传输影响（p95 增量 ≤ 30 ms） |
| 资源：1C1G 中心，100 台主机空闲 | — | 空闲会话常驻内存合计 ≤ 16 MiB；无活动主机不保持连接 |

### 3.2 非目标

- 不改变 scope、配对、撤权、审计的权限语义；不扩大终端或文件能力；
- 不新增监听端口，不引入 SSH 服务、UDP 或 QUIC；
- 不开放通用 TCP 转发、任意 HTTP 代理或新的任意 Shell API；
- 不让浏览器直连远端，也不跨主机共享登录态；
- 不改 Agent Unix Socket API 和 PTY Manager 语义（缓冲、闲置回收、最长会话保持不变）；
- 不把终端变成后台任务；断线重连仍依赖目标端内存环形缓冲。

## 4. 方案比较

| 方案 | 做法 | 收益上限 | 主要问题 | 结论 |
|---|---|---|---|---|
| A 现状调优 | 批量带回事件、拆连接池、调限流、轮询合并 | 列目录降到 1–2 个 RTT；吞吐仍受 64 KiB 负载上限约束 | 每操作仍 ≥1 RTT + 握手；无推送、无流控 | 只做与 v3 不冲突的部分（第 8 节阶段 0） |
| B 每请求一条 WebSocket | 恢复 v1.16.0 的普通 Panel 文件流 | 大文件带宽受限 | 短请求需 TCP + TLS + Upgrade + Noise + 请求，约 4–5 个 RTT，比现状还慢；终端不适用 | 只用于大文件数据连接 |
| **C 长会话 + 多路复用** | 每对节点一条 Noise 会话，内含封闭类型的流 | 回显 ≈ 纯 RTT；短请求 1 RTT；推送与流控 | 新协议面、长期会话密钥、代理兼容、兼容旧节点 | **采用**，与 B 组合 |
| D SSH 隧道 | `x/crypto/ssh` + `pkg/sftp` 经现有端口隧道 | 通道、PTY、流控、SFTP 流水线现成 | exec/subsystem 语义需逐项锁死，与 scope 模型错位；新增大依赖面 | 备选，不采用 |
| E QUIC/WebTransport | quic-go | 无队头阻塞、0-RTT | 需 UDP 端口，常被封；违反不新增端口原则 | 不采用 |
| F gRPC 双向流 | h2/h2c | 流与流控现成 | 依赖重；`http://IP` 需 h2c；与现有 Noise 身份叠加复杂 | 不采用 |

## 5. 协议设计

### 5.1 会话（Channel）

- 端点：`GET /api/v3/federation/channel`，WebSocket 子协议 `kpanel-channel-v3`，复用现有端口和精确路径校验，拒绝查询参数与 `RawPath` 变体。
- 方向：
  - 完整 KPanel：中心主动拨号目标，沿用现有 DNS/IP 允许清单、rebinding 检查、TLS 验证和禁止重定向；
  - 轻量节点：节点主动拨号中心（沿用文件流控制连接模式），节点仍不监听任何端口。
- 认证：101 之后先完成 Noise `IK_25519_ChaChaPoly_SHA256`，使用现有静态密钥
  （完整 Panel 用 v2 host credential，轻量节点用终端 Noise 密钥）。prologue 绑定协议版本、路径、双方 ID、
  时间戳和随机 channel ID；时间戳窗口与现行 v2 一致。认证完成前只允许握手帧。
- 加密：握手产生两个独立 CipherState；每帧 AEAD，使用隐式递增 nonce，因此会话内重放、乱序、截断都会导致解密失败并断开。
  发送量达到 1 GiB 或会话满 1 小时执行 Noise `Rekey`；会话最长 24 小时后强制重建。
- 按需建立：首次打开终端或文件操作时建立，无活动流 5 分钟后关闭。只做监控的主机不保持连接；集群摘要在阶段 4 前继续走 v2。
- 心跳：15 秒 ping/pong，45 秒无响应断开（与现有文件流一致，低于常见 CDN 的 100 秒空闲上限）。
- 撤权：删除 Host/Controller 或 scope 变化时立即关闭该身份的全部会话（沿用 `fileStreamHub.closePeer`）。

### 5.2 帧与多路复用

Noise 明文内的帧格式：

```text
stream_id u32 | kind u8 | flags u8 | length u16 | payload[length]
kind: OPEN / ACCEPT / REJECT / DATA / WINDOW / CONTROL / CLOSE / RESET / PING / PONG
```

- 流量控制采用基于额度（credit）的窗口：每流初始 256 KiB，接收方消费后发 `WINDOW`；每会话总窗口 1 MiB。
  背压直接传到 PTY 读取与文件读写，不在中心无界缓冲。
- 复用库待定（Q2）：`hashicorp/yamux`（MPL-2.0，与 AGPL-3.0 兼容，但属新依赖，需走依赖准入五项证据）
  或自研上述约 300–500 行的最小帧层。倾向自研：帧类型封闭、便于逐字段做严格校验和模糊测试，与项目"严格 JSON、拒绝未知字段"的风格一致。

### 5.3 流类型（封闭枚举）

`OPEN` 负载为严格 JSON，只接受以下类型；未知类型、未知字段、越界参数直接 `REJECT`。**每次开流都按当前存储重新校验 scope**，不只在会话建立时校验。

| 类型 | 所需 scope | 负载 | 语义 |
|---|---|---|---|
| `terminal.open` | `cluster.terminal.open` | rows、columns | 创建 PTY，返回会话 ID 与起始 offset，随后推送输出 |
| `terminal.attach` | `cluster.terminal.open` | 会话 ID、offset | 断线或回退后从 offset 继续；超出缓冲时标记截断，与现行语义一致 |
| `file.request` | `cluster.files.read`（写操作沿用现有文件权限判定） | 现有受限文件 HTTP 契约（method、固定路径、query、白名单头、长度） | 复用 `serveStreamRequest`；只适用于元数据、目录、文本、缩略图等短请求 |
| `transfer.export` | linked grant 规则不变 | 与 `/files/open-linked` 相同 | 阶段 2 再评估，可继续走现有 HTTP 流 |

终端流内的 `CONTROL` 子消息：`input`（≤16 KiB）、`resize`、`close`、`exit`（真实退出码）。
输出推送采用最多 5 ms 或 32 KiB 的合并，二者先到先发，每块带 offset。

### 5.4 大文件与队头阻塞

同一 TCP 上多路复用在丢包时仍有队头阻塞。为保证"传文件时终端不卡"：

- 预计超过 1 MiB 的下载、任何上传、目录 TAR/ZIP、跨节点导入导出，走**独立数据 WebSocket**（恢复并收敛 v1.16.0 的普通 Panel 文件流；
  轻量节点继续使用现有"控制连接下发请求 ID、节点回拨"的模式）；
- 会话内只跑终端和短文件请求；
- 现有限额继续适用：未认证握手 64、已认证连接 128、每身份 10；文件流大文件与短请求各全局 16、每身份 4。

### 5.5 浏览器到中心

- 新增 `GET /api/v1/terminal-stream?sessions=<id,...>&offsets=<n,...>`，返回 `text/event-stream`，
  **一个标签页一条流**承载该用户的全部终端输出，避免多终端占满 HTTP/1.1 的 6 个连接；
  鉴权沿用 Session，并对流做 Origin 校验；会话 ID 仍是绑定管理员的 256 位 Panel 公共 ID。
- 输入、resize、close 保持现有 POST 接口与 CSRF/Origin 校验；中心收到后直接写入已建立的终端流，不再握手。
- SSE 断开后浏览器按各自 offset 重连；不支持 SSE 或被代理缓冲时，回退到现行 `output?wait=` 长轮询。
- 不在本提案引入浏览器 WebSocket；如果评审后仍认为 POST 输入开销显著，再单独提案。

### 5.6 能力协商与回退

- 目标端在已认证的摘要响应和 v2 能力头中声明 `channel-v3`；中心只在双方都声明时尝试。
- 回退规则：
  - 101 之前收到 404/405/426，或代理返回 400/502/503：该主机标记为兼容模式，10 分钟后再探测，主机列表显示"兼容传输"；
  - 101 之后认证失败：记为认证错误，**不**回退，与现行规则一致；
  - 101 之后断流或超时：终端通过 `terminal.attach` 或 v2 `output` 按 offset 继续（目标端 PTY Manager 与 owner 相同，两种传输可以读同一会话）；
    已开始的写操作不自动重放，交给用户重试。
- 轻量节点：中心不支持 v3 时继续使用现有 relay 与文件流；节点回滚后中心自动识别为旧能力。
- 版本回滚：旧版本忽略 `channel-v3` 能力，不需要数据迁移。

## 6. 安全分析

| 变化 | 风险 | 控制 |
|---|---|---|
| 每请求独立握手 → 长期会话 | 会话密钥暴露窗口变长 | 定期 Rekey、最长 24 小时、撤权立即断开、开流逐次校验 scope |
| 时间窗 + request ID 防重放 → 会话内隐式 nonce | 跨会话重放 | 每个会话使用新的临时密钥；握手时间窗与 channel ID 唯一性检查保留 |
| 新的长连接端点 | 慢速攻击、连接耗尽 | 握手 8 秒超时，沿用未认证/已认证/每身份连接上限；认证前只收握手帧 |
| 流多路复用 | 借流类型扩权、通用转发 | 流类型封闭枚举；负载严格 JSON；不提供任意目标或端口参数 |
| SSE 长连接 | 登录态失效后流未关闭 | 注销、Session 过期、会话关闭时服务端主动结束流；定期重新校验 Session |
| 反向代理/CDN | 缓冲导致延迟，或 Upgrade 被拦截 | SSE 设置 `X-Accel-Buffering: no`；Upgrade 失败走回退；部署文档补代理配置 |

审计：记录会话建立/关闭、流打开/拒绝/关闭及结果、目标主机和管理员；不记录输入、输出、文件内容或密钥。
实施时新增信任边界包，按 `PROJECT_RULES.md` 5.4 执行 `security-boundary-audit`（profile=scoped）。

## 7. 资源预算

| 项目 | 预算 |
|---|---|
| 空闲会话 | 每条 ≤ 64 KiB（WebSocket 缓冲、CipherState、帧缓冲）+ 3 个 goroutine |
| 100 台主机全部有活动会话 | ≤ 6.4 MiB 常驻；实际按需建立、5 分钟闲置关闭 |
| 终端窗口 | 每流 256 KiB；16 个全局会话上限下最坏 4 MiB |
| 限流 | 终端请求计数限流改为每流按字节和帧计的预算；保留握手与开流速率限制 |
| CPU | 握手次数从"每按键/每轮询"降为"每会话"；ChaCha20-Poly1305 在低配 ARM 上仍可达数十 MiB/s |

## 8. 分阶段计划

每个阶段都可以独立发布、独立回滚，依靠能力协商共存。

| 阶段 | 内容 | 协议变化 | 验收重点 |
|---|---|---|---|
| 0 过渡 | ① 轻量 relay 在请求带有输出事件时立即返回（或最多等 20 ms），broker 执行命令后短暂等待回显；② 前端把 429 与断线分开处理；③ 带 version 的缩略图改用有期限的私有缓存（需安全评审）；④ 建立第 9 节的基准测试工具 | 无（行为调整） | 基线数据；轻量回显改善 |
| 1 轻量终端上流 | 复用已有文件流控制连接和回拨机制，新增终端流类型 | 轻量节点新增流类型 | 750 ms 延迟消失；节点回滚兼容 |
| 2 完整 Panel 会话 | `channel-v3`：终端和短文件请求走会话，大文件走独立数据 WebSocket | 新端点与能力 | 先关闭 Q1；代理/CDN 矩阵；与 v2 回退互通 |
| 3 浏览器推送 | 终端 SSE 聚合流 | 新浏览器接口 | 6 连接压力、代理缓冲、Session 失效 |
| 4 可选 | 摘要 RPC 走会话；批量执行改用唯一结束标记加退出码判定完成 | 流类型扩展 | 批量命令秒级完成 |

阶段 0 中，"`collect` 批量带回事件、拆分连接池、调整终端限流额度"不单独实施，由阶段 2 整体替代。

## 9. 验证方案

### 9.1 基准测试矩阵

在隔离环境（WSL Docker 或已登记的 arena 目标，禁止 `prod-108`/`108`）用 `tc netem` 注入条件：

- RTT：20 / 80 / 150 / 300 ms；丢包：0% / 1%；带宽：10 / 100 Mbps；
- 拓扑：中心→完整 KPanel（HTTPS 域名、`http://IP`）、中心←轻量节点；
- 反向代理：直连、Nginx、Caddy、Cloudflare 代理（验证 Upgrade、100 秒空闲、SSE 缓冲）。

### 9.2 指标

| 场景 | 指标 |
|---|---|
| 回显 | 单字符往返 p50/p95/p99（浏览器侧打点） |
| 持续输出 | `yes \| head -c 50M` 吞吐、429 次数、"重连中"次数 |
| 列目录 | 100 项、500 项目录的首字节与完成时间 |
| 文件 | 100 MiB 上传/下载吞吐；传输期间终端回显 p95 |
| 批量执行 | 4 台、16 台 `uptime` 完成时间 |
| 资源 | 1C1G 中心在 100 台主机、16 个终端下的 RSS、goroutine、fd、CPU |

v2 与 v3 使用同一工具、同一环境对比。证据只对精确提交、环境与参数有效。

### 9.3 回归与兼容

- 新旧组合矩阵：v3 中心 × v2 目标、v2 中心 × v3 目标、v3 中心 × 旧轻量节点、节点回滚；
- 撤权后会话立即断开、新流被拒；scope 变化后开流逐次校验；
- 帧篡改、截断、重放、超长帧、未知流类型、窗口溢出的模糊测试；
- 断线后按 offset 恢复、截断标记、会话 8 小时上限与 30 分钟闲置回收不变。

## 10. 开放问题

| # | 问题 | 为什么重要 |
|---|---|---|
| Q1 | v1.16.0 回滚后，普通 Panel 恢复 POST relay 的具体技术原因是什么？ | 若是代理/CDN 兼容问题，阶段 2 的回退策略必须覆盖该场景 |
| Q2 | 多路复用自研最小帧层，还是引入 `hashicorp/yamux`？ | 依赖准入与长期维护成本 |
| Q3 | [多主机终端](multi-host-terminal.md) 中"不开放新的 TCP、SSH、WebSocket 或 Agent 公网监听端口"是否只约束新端口？ | 决定现有端口上的 WebSocket 会话是否需要修订该契约 |
| Q4 | 浏览器侧是否只允许 SSE，还是也接受 WebSocket？ | 影响阶段 3 设计与输入路径开销 |
| Q5 | v3 支持的最低对端版本和兼容期多长？ | 决定 v2 路径何时可以冻结 |
| Q6 | 用户常见部署是否普遍在 KPanel 前接 CDN 或反向代理？ | 决定代理兼容测试的优先级与默认回退行为 |

## 11. 采纳后需要更新的文档

- [多主机终端](multi-host-terminal.md)：链路、固定 API、资源上限、浏览器推送；
- [集群监控与联邦协议](cluster-monitoring.md)：v3 端点、能力协商、"普通 Panel 不发起文件 WebSocket"条款；
- [架构](architecture.md)：集群监控边界；
- [部署](deployment.md)：反向代理的 Upgrade、空闲超时和 SSE 缓冲配置；
- [跨 KPanel 文件传输](cross-kpanel-file-transfer.md)：大文件数据连接。
