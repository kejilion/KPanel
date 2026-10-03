# KPanel Windows 轻量节点与远程桌面设计

- 状态：进入本地候选开发；原提案 `b4e84aac`，实现基线 `52cc6436`，候选分支 `feature/windows-light-node`；未发布
- 范围：`kejilion-node` 的 Windows 平台适配（遥测、健康、安装/更新/卸载、终端与批量执行、文件管理、
  登录事件、服务探测），以及可选的 Windows 远程桌面（第 13 节，P4）
- 非目标：Windows 版 Panel/Agent；结构化 Windows 系统管理（服务、更新、防火墙配置）；自研屏幕采集；
  Windows 家庭版远程桌面；通用 TCP 端口转发
- 业务真源：Windows 宿主机实时状态（Win32 API、服务控制管理器、事件日志、文件系统）。中心只缓存最新快照，
  与 Linux 轻量节点相同
- 关联契约：[集群监控 §3.3](cluster-monitoring.md)、[多主机终端](multi-host-terminal.md)、
  [终端与文件传输 v3](terminal-file-transport-v3.md)、[历史监控](history-monitoring-design.md)、
  [平台支持](platform-support.md)

本文区分三类内容：**已验证事实**（第 2 节，附命令、版本或代码位置）、**设计决定**（第 3–19 节）和
**待决事项/未验证风险**（第 23–24 节）。设计决定在实现前可以修订，但修订必须回到本文同步。

### 本次实施契约

- 目标：原生 Windows 轻量节点接入、生命周期、遥测、PowerShell、文件与可选 RDP；Linux 现有行为回归。
- 用户确认的交互：Windows 与 Linux 同列在终端左侧主机列表，点击 Windows 主机后先选择
  “命令行（PowerShell）”或“远程桌面（RDP）”，随后在右侧打开所选会话。
- 集群添加弹窗在 Linux 接入入口下增加 Windows 入口；单台和批量均生成一行管理员 PowerShell 命令，
  自动下载、校验、安装与接入。执行安装脚本前先完成签名及发布者校验。
- 首批验证 amd64；安装器作为 KPanel Release 资产。`scriptLinkageState=not-required`（无需发布脚本），
  脚本基线仍固定 `c981fb6c8b481981ac7a006e102e111e435f6d30` / SHA-256
  `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`。
- 允许修改节点、相关后端、Web、Windows 安装构建与本设计；不修改版本号、历史发布记录、Linux 脚本。
  仅形成独立本地候选，不推送、不合并 main、不打标签、不发布或部署。
- 生命周期与宿主权限按 L3 风险设计；开发执行变更门禁、平台测试、独立复核。正式 L3、签名发行物、
  Windows Server 干净实例安装/重启验收分别记录，缺少证据不解释为通过。
- 可见交互在精确候选提交后提供 UI Mock acceptance 预览；该证据不能证明真实 RDP 或系统服务生命周期。

## 1. 目标与原则

1. **对齐，不另起一套。** Windows 节点仍是 `light_node` 类型，沿用 `light-v1` 遥测、v2 Noise 中继、
   v3 流式角色、`kpl1`/`kpb1` 接入、`light-health-v1` 健康协商和 100 台共享上限。中心协议不升版本，
   只增加能力声明和可选字段。
2. **轻量。** 不新增第三方 Go 依赖（`golang.org/x/sys` 已覆盖所需 API）；采集不用 WMI，稳态不派生子进程；
   常驻进程与 Linux 一致：4 个服务加 1 个计划任务；远程桌面不新增进程。
3. **安全水位不低于 Linux 轻量节点。** 只出站 HTTPS；低权限遥测与 SYSTEM broker 分离；私钥只有 SYSTEM 可读；
   发布物同时校验 SHA-256 和 Authenticode；不开放任何本地监听端口或命名管道。
4. **稳定。** 服务失败自动恢复；更新失败自动回滚；重启、休眠、Windows 更新后无需人工介入即可恢复上报。
5. **不设操作护栏，但保留攻击面防护。** 遵守 `PROJECT_RULES.md` 第 2、3 节：管理员通过终端仍拥有 SYSTEM
   能力；本文的限制只针对未授权访问、注入、篡改、泄密和资源耗尽。

## 2. 现状核对（已验证事实）

### 2.1 可以直接复用的部分

中心端协议与操作系统无关：

- 遥测：`/api/v3/federation/light/report`，HMAC 绑定方法、路径、节点 ID、时间戳、request ID 与正文摘要；
  `validateTelemetry`（`internal/cluster/service.go`）只校验长度、范围和格式，不假设 Linux。
- 能力协商：响应头 `X-KPanel-Light-Response-Capabilities`（`ssh-login-v1`、`service-checks-v1`、
  `light-health-v1`），节点只在中心声明后才发送可选字段，旧中心返回 400/422 时去掉可选字段重试一次。
- 终端与文件：v2 Noise 中继与 v3 流式角色（`light-terminal-control`/`light-terminal-data`、文件流）。
- 接入：`/api/v3/federation/light/enroll`、`/api/v3/federation/light/batch-enroll`、
  `/api/v3/federation/light/file-capability`。
- Host DTO 已有 `terminalAvailable`、`fileManagementAvailable` 等布尔能力字段（`internal/cluster/types.go`）。

### 2.2 交叉编译

2026-10-03，`kpanel-go127-prep-runner:go1.27.1-node24.21.0` 容器，源码 `644c4c79`（与本文基线
`52cc6436` 之间无 `cmd/`、`internal/`、`web/`、`go.mod` 差异），`CGO_ENABLED=0 -trimpath -ldflags "-s -w"`：

| 目标 | 结果 | 体积 |
| --- | --- | --- |
| `windows/amd64` | 编译通过；`go vet ./cmd/kejilion-node` 无输出 | 10,078,208 B |
| `windows/arm64` | 编译通过 | 9,046,528 B |
| `linux/amd64`（对照） | 编译通过 | 9,953,440 B |

**能编译不等于能用。** 现有非 Linux 分支有多处在出错时直接放行（fail-open），见 2.3。

### 2.3 直接交叉编译后的问题

| 位置 | 现状 | Windows 上的后果 |
| --- | --- | --- |
| `cmd/kejilion-node/terminal_config.go:47`、`main.go:545`、`batch_enrollment_attempt.go:86/133/149/204` | 遇到 `GOOS == "windows"` 就跳过权限检查 | 终端 Noise 私钥、遥测密钥、批量接入暂存都没有访问控制 |
| `cmd/kejilion-node/root_ownership_other.go:16/20` | `rootOwned` 恒 false，`processOwned` 恒 true | 属主校验形同虚设 |
| `internal/terminal/executable_owner_other.go:7` | 可执行文件属主校验恒 true | shell 路径可被劫持 |
| `cmd/kejilion-node/terminal_broker.go:36`、`file_broker.go:42`、`ssh_login_broker.go:37` | 只在 Linux 检查 root | 任何权限都能启动 broker |
| `cmd/kejilion-node/main.go:319` + `internal/systeminfo/collector.go` | 读 `/proc` 失败，但只要取到 hostname 就上报 | 上报全 0 的假遥测，且能通过中心校验 |
| `internal/agent/server.go:268` | 受保护目录是 POSIX 路径文本 | 节点自己的密钥目录不受文件管理保护 |
| `cmd/kejilion-node/procd.go:201` | 状态目录写死 `/var/lib/kejilion-node` | 落到当前盘符的 `\var\lib\...` |
| `internal/hostpty/process_other.go` | 返回 "interactive host terminals require Linux" | 终端不可用（安全地失败） |

### 2.4 前端与发布中的 Linux 假设

- 批量执行包装是 POSIX 写法：`web/src/lib/batchCompletion.ts:18`；退出码只接受 0–255。
- 系统识别表只有 Linux 发行版：`web/src/lib/operatingSystem.ts:12`。
- 文件管理有 chmod 与属主 UI：`web/src/views/FilesView.vue:3249`。
- 接入命令写死 `bash <(curl …)`：`internal/cluster/light_service.go:103`、`light_batch_service.go:90`。
- Release 只构建 `linux/amd64`、`linux/arm64` 的节点：`.github/workflows/release.yml:174`；CI 没有 Windows 任务。
- 发布物只有同一 Release 内的 `SHA256SUMS`，没有签名。

### 2.5 `golang.org/x/sys v0.48.0` 能力核对

同一容器内 `go mod download` 后检查 `windows` 包：

- 已有：`CreatePseudoConsole`、`ResizePseudoConsole`、`ClosePseudoConsole`、`NewProcThreadAttributeList`、
  `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`、`CreateProcess`、`CreateJobObject`、`SetInformationJobObject`、
  `AssignProcessToJobObject`、`GetSecurityInfo`、`GetNamedSecurityInfo`、`SetNamedSecurityInfo`、
  `GetFinalPathNameByHandle`、`WinVerifyTrustEx`、`RtlGetVersion`、`GetIfEntry2Ex`、`GetAdaptersAddresses`、
  `GetDiskFreeSpaceEx`、`NewLazySystemDLL`，以及 `svc`、`svc/mgr`、`svc/eventlog`、`registry` 子包。
- 需要自己包装：`GetSystemTimes`、`GlobalMemoryStatusEx`、`GetTickCount64`（kernel32），
  `IcmpSendEcho2`（iphlpapi），`EvtQuery` 系列（wevtapi）。一律通过 `NewLazySystemDLL` 加载。

## 3. 支持矩阵（设计目标）

| 级别 | 系统 | 能力 |
| --- | --- | --- |
| 首批实机准入 | Windows Server 2022/2025、Windows 11 24H2，`amd64` | 全部 |
| 第二批 | Windows Server 2019、Windows 10 22H2、Windows 11 `arm64` | 全部 |
| 部分支持 | Windows Server 2016 | 遥测、健康、更新、文件管理；没有 ConPTY，不声明终端与批量执行 |
| 不支持 | Windows 7/8.1、Server 2012 R2（Go 运行时最低 Windows 10/Server 2016）；32 位；Nano Server；容器内 Windows | — |

- 远程桌面（P4）只支持带 RDP 被控端的版本：Pro、Enterprise、Education 和 Server。家庭版不支持。
- Server Core 可用：节点不依赖图形界面。
- Windows 10 22H2 已过主流支持，处于 ESU 期；准入记录必须写明具体累积更新版本（build.UBR）。
- 与 Linux 相同，“已实现”不等于“已准入”：必须在干净实例上完成安装、更新、重启恢复、回滚和卸载闭环。

## 4. 总体架构

```text
中心 KPanel（HTTPS 根地址，不变）
   ▲  出站 HTTPS：遥测 HMAC（light-v1）
   ▲  出站 HTTPS：v2 Noise 中继 + v3 Noise 流（终端 / 文件 / 历史 / 远程桌面）
   │
Windows 主机 ─────────────────────────────────────────────────────────────
   KejilionNode            虚拟账户 NT SERVICE\KejilionNode：遥测、健康读取
   KejilionNodeTerminal    LocalSystem：ConPTY 终端；P4 起兼任 RDP 桥
   KejilionNodeFile        LocalSystem：文件管理、历史采样、计划任务状态快照
   KejilionNodeLogin       虚拟账户 NT SERVICE\KejilionNodeLogin：登录事件
   计划任务 \KPanel\KejilionNodeUpdate   SYSTEM：每小时检查稳定版更新
```

不监听任何 TCP/UDP 端口，不创建命名管道，不注册 COM 服务器。进程之间只通过带 ACL 的文件交换窄快照
（与 Linux 的 `/run/kejilion-node-*` 相同）。

### 4.1 进程与账户映射

| Linux | Windows | 权限要点 |
| --- | --- | --- |
| `kejilion-node.service`（`kejilion-node` 用户） | `KejilionNode`（虚拟账户） | 服务 SID 类型 `RESTRICTED`；所需特权只保留 `SeChangeNotifyPrivilege`；只能读遥测配置 |
| `kejilion-node-terminal`（root） | `KejilionNodeTerminal`（LocalSystem） | 只读终端 Noise 私钥；PTY 与 RDP 桥都只服务已认证中心 |
| `kejilion-node-file`（root） | `KejilionNodeFile`（LocalSystem） | 文件管理根为全部固定卷；不默认启用 `SeBackupPrivilege`（见 D8） |
| `kejilion-node-ssh-login`（root，仅 `CAP_DAC_READ_SEARCH`） | `KejilionNodeLogin`（虚拟账户，加入 Event Log Readers） | 只读事件日志，只输出单条窄事件 |
| update timer / periodic / cron | 计划任务（SYSTEM） | 进程级锁；失败回滚 |

四个服务都由同一个 `kejilion-node.exe` 以不同子命令运行，与 Linux 一致。

### 4.2 目录与 ACL

| 路径 | 内容 | DACL（禁用继承，属主 `BUILTIN\Administrators`） |
| --- | --- | --- |
| `%ProgramFiles%\KejilionNode\` | `kejilion-node.exe`、`kejilion-node.exe.old` | SYSTEM、Administrators 完全控制；Users 读取执行 |
| `%ProgramData%\KejilionNode\` | 状态根 | SYSTEM、Administrators 完全控制 |
| `…\node.json` | 遥测配置（含 reporting key） | 额外：`NT SERVICE\KejilionNode` 读 |
| `…\terminal.json` | 终端/文件 Noise 私钥 | 只有 SYSTEM 读写，Administrators 只能接管不能直接读 |
| `…\batch-enrollment-attempt.json` | 批量接入续接状态 | 只有 SYSTEM |
| `…\state\monitoring\` | 历史采样 | 只有 SYSTEM |
| `…\run\` | 健康快照、计划任务快照 | SYSTEM 写；对应虚拟账户读 |
| `…\run\login\` | 登录事件快照 | SYSTEM 与专用 Login service SID 写；遥测账户读。祖先校验仅对真实 KnownFolder 下此目录授予该例外 |
| `…\update-status.json` | 更新结果 | SYSTEM 写；`NT SERVICE\KejilionNode` 读 |

规则：

1. 安装器创建目录前检查路径上每一级：**已存在但属主不是 SYSTEM 或 Administrators、带重解析点、或 DACL
   允许非管理员写入时，拒绝安装并说明原因。** 普通用户可以在 `%ProgramData%` 下抢先建目录，再用 junction
   诱导 SYSTEM 写入任意位置，这是经典本地提权手法。
2. 节点读取任何配置或快照前，用 `GetSecurityInfo` 校验属主与 DACL，取代现有 `runtime.GOOS != "windows"`
   分支；校验失败一律拒绝启动或忽略该快照，不降级为“无校验”。
3. 原子写入：同目录临时文件 → `FlushFileBuffers` → `MoveFileEx(MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)`；
   被杀毒软件短暂占用时有界重试（最多 5 次，总计 ≤ 2 秒）。
4. 二进制只放在 `%ProgramFiles%`，永不放在 `%ProgramData%`。

## 5. 接入、安装与生命周期

### 5.1 接入命令

中心在“集群 → 添加主机”增加“Windows 主机”选项，生成以管理员身份在 PowerShell 中执行的一行命令。
令牌格式、有效期、单次/批量语义、100 台上限与 Linux 相同；生成 API 新增可选 `platform`，缺省仍为 Linux。
界面在原 Linux 接入卡下显示 Windows 卡，批量接入可选目标系统。

一行命令先下载固定 KPanel Release 版本的 `install-windows.ps1` 到受保护的 Program Files 临时目录，
校验 Authenticode、发布者 Subject 和可选 profile EKU OID 后才调用脚本；不得直接执行下载文本。
令牌、名称和信任策略以 UTF-8 Base64 表达式编码，避免 PowerShell Unicode 引号被解释为语法。
中心通过 `KEJILION_PANEL_WINDOWS_NODE_PUBLISHER` 配置信任发布者，缺失时生成接口失败关闭，
可选 `KEJILION_PANEL_WINDOWS_NODE_PROFILE_OID` 进一步绑定签名 profile。

### 5.2 安装器步骤

1. 校验以管理员运行；检测受限语言模式（AppLocker/WDAC 环境）后明确失败，并提示改用离线安装包。
2. 检测版本（`RtlGetVersion` 等价信息）、架构（`IsWow64Process2`）、版本类型；不在支持矩阵内时明确失败。
3. 下载 `SHA256SUMS`（≤ 64 KiB）和 `kejilion-node-windows-<arch>.exe` 到受保护的临时目录；
   与 Linux 相同，`latest` 只解析一次并固定到 `releases/download/v<semver>/`。
4. 校验 SHA-256，再用 Windows 信任 API 校验 Authenticode、发布者 Subject 与可选 profile EKU OID。任一失败即中止。
   不固定会每日轮换的叶证书指纹；发布必须带可信时间戳，轮换发布主体/profile 属显式策略变更。
5. 按 4.2 创建目录与 ACL，安装二进制。
6. 通过受保护、SYSTEM-only 的 bootstrap handoff 完成接入、收据与服务安装，由二进制通过
   `svc/mgr` 注册服务：ImagePath 带引号；自动启动；服务 SID 类型；所需特权；失败恢复动作
   （5 秒、30 秒、60 秒后重启，1 天重置计数，非零退出也视为失败）。
7. 注册更新计划任务（第 8 节），启动服务，输出与 Linux `status` 等价的状态摘要。
8. 接入收据与批量 attempt 持久化在 SYSTEM-only 状态中；失败可用原身份续装，不重复消费已确认接入。
   若响应丢失且无法确定单次令牌是否消费，明确报错，由管理员在中心核对记录后重建授权。

### 5.3 令牌处理

- 令牌不写入服务 ImagePath、服务参数、注册表或环境变量块；只在接入时用于一次 HTTPS 请求。
- 命令行可能被 PowerShell ScriptBlock 日志（事件 4104）和 PSReadLine 历史文件记录。单次令牌 5 分钟失效、
  只能消费一次，风险有限；批量令牌最长 7 天、最多 100 次，风险较高。因此：
  - 安装器支持从环境变量 `KPANEL_NODE_TOKEN` 读取令牌，接入窗口在批量模式下给出该写法；
  - 文档建议 Windows 批量接入使用最短有效期，部署完成后在中心撤销；
  - 安装结束时提示管理员检查并清除 PSReadLine 历史中的令牌行。
- 批量接入续接状态按 4.2 只有 SYSTEM 可读，语义与 Linux 相同（同一 attempt 复用同一身份和 Noise 密钥）。

### 5.4 生命周期命令

| Linux | Windows |
| --- | --- |
| `k kpanel node status` | `kejilion-node.exe status` |
| `k kpanel node update` | `kejilion-node.exe update` |
| `k kpanel node uninstall` | `kejilion-node.exe uninstall` |

安装、续装、更新、卸载共用命名互斥体 `Global\KejilionNodeLifecycle`。
锁由专用固定 OS 线程获得和释放，调用者 Go goroutine 迁移或重复关闭不会遗留锁。并发调用明确返回“稍后重试”。
卸载顺序：停止服务 → 删除服务与计划任务 → 从 Event Log Readers 移除虚拟账户 → 删除目录。
中心删除记录不远程卸载节点（与 Linux 相同）。

### 5.5 中心能力门禁

Windows 接入请求带 `platform=windows`；旧中心严格解析请求，因此在消费令牌前拒绝，不能降级为 Linux 接入。
接入后通过带 HMAC、时间窗和重放保护的 `POST /api/v3/federation/light/capabilities` 探测中心能力。
每次管理 broker 重连都重新校验 `windows-node-v1`，桌面另需 `desktop-v1`，不使用历史响应头作为授权缓存。
报告外层携带 `platform`、`capabilities`、`unavailableMetrics` 与有限枚举的 `desktopUnavailableReason`；
`HostTelemetry` 持久化契约不变。中心缓存能力只存内存，90 秒失鲜或重启后未重新上报时拒绝远程管理。

## 6. 遥测采集

新增 `internal/systeminfo` 的 Windows 实现（`*_windows.go`），保持 `contract.HostTelemetry` 不变：

| 字段 | 来源 | 说明 |
| --- | --- | --- |
| CPU 使用率 | `GetSystemTimes` 两次采样差值，遍历 processor group 后汇总 | 支持超过 64 个逻辑 CPU；采样差值给出空闲/内核/用户比例 |
| CPU 型号/频率 | 注册表 `HARDWARE\DESCRIPTION\System\CentralProcessor\0` | 只在首次读取和每 30 分钟刷新 |
| 核心数 | `GetActiveProcessorCount(ALL_PROCESSOR_GROUPS)` | 覆盖超过 64 核的多处理器组 |
| 内存 | `GlobalMemoryStatusEx` | 总量、可用量 |
| Swap | `GetPerformanceInfo`：提交上限减物理内存 | 作为页面文件近似值 |
| 磁盘 | `GetDiskFreeSpaceEx(%SystemDrive%\)` | 对应 Linux 的 `/` |
| 网络累计量 | `GetAdaptersAddresses` + `GetIfEntry2Ex` | 只计硬件接口；排除回环、过滤驱动、隧道和虚拟接口 |
| TCP/UDP 连接数 | `GetTcpStatisticsEx`、`GetUdpStatisticsEx`（IPv4+IPv6） | 不枚举连接表 |
| 运行时间 | `GetTickCount64` | 开启“快速启动”时不随关机清零，记录在文档中 |
| 系统名称 | 注册表 `ProductName`、`DisplayVersion`、`CurrentBuild`、`UBR` | build ≥ 22000 时把 “Windows 10” 改为 “Windows 11”（注册表历史遗留） |
| 内核 | `10.0.<build>.<UBR>` | |
| `osId` / `osLike` | `windows` / `["windows"]` | 前端据此显示 Windows 图标 |
| 架构 | `IsWow64Process2` 的本机架构 | 区分 arm64 主机上的 x64 仿真 |
| 负载均值 | 不采集 | 见下文 |
| 公网信息 | 沿用现有 `lookupPublicNetwork` | 与操作系统无关 |

规则：

1. **不伪造数据。** CPU、内存、运行时间任一读取失败时，本轮不上报，记录错误；中心按既有阈值把节点标为
   `stale`/`offline`。不允许出现 2.3 中“全 0 也上报”的情况。
2. **负载均值。** Windows 没有对应指标。`contract.LoadSummary` 不是指针，节点发送 0；中心在
   `platform=windows` 时，集群、桌面监控和历史监控隐藏负载项（显示“—”），不把 0 当真实值。
   gopsutil 用处理器队列长度模拟负载，本设计不采用，记入第 22 节取舍。
3. 稳态不使用 WMI、PDH 或子进程。WMI 查询会拉起 `WmiPrvSE` 并带来秒级抖动。

## 7. 健康与服务观测

`light-health-v1` 契约保持不变，Windows 状态映射到现有枚举：

| 服务控制管理器 | `activeState` / `subState` |
| --- | --- |
| `SERVICE_RUNNING` | `active` / `running` |
| `SERVICE_START_PENDING` | `activating` / `start` |
| `SERVICE_STOP_PENDING` | `deactivating` / `stop` |
| `SERVICE_STOPPED`，退出码为 0 | `inactive` / `dead` |
| `SERVICE_STOPPED`，退出码非 0 | `failed` / `failed` |
| 服务不存在 | `loadState=not-found` |

| 启动类型 | `unitFileState` |
| --- | --- |
| 自动 / 延迟自动 | `enabled` |
| 禁用 | `disabled` |
| 手动 | `static`（实现前在 D9 中确认） |

- 遥测进程用 `SERVICE_QUERY_STATUS` 直接查询 4 个服务；如果虚拟账户无权查询，改由 file broker 写快照
  （需实机确认，见第 24 节）。
- 更新计划任务的状态由 `KejilionNodeFile` 每 20 秒读取，写入 `run\task-health.json`；遥测只接受 4 KiB 以内、
  60 秒以内的快照，与 procd 先例相同。
- 前端在 `platform=windows` 时把“定时器”显示为“计划任务”。

## 8. 自动更新

| 项目 | 规则 |
| --- | --- |
| 触发 | 计划任务 `\KPanel\KejilionNodeUpdate`，SYSTEM；开机 15–30 分钟后首次运行，之后每小时；随机延迟 0–15 分钟；不允许多实例并行；单次最长 30 分钟 |
| 通道 | 只跟踪稳定版，与 Linux 相同；中心不推送更新，不支持远程切换版本 |
| 检查 | 先下载 `SHA256SUMS`；本平台摘要未变化则结束 |
| 校验 | SHA-256 + Authenticode 固定发布者；再在暂存目录执行新二进制的 `version`，确认协议与运行时代数不降级 |
| 替换 | 稳定 bootstrap 入口与受保护更新事务记录配合；在停止服务前完成下载、签名和摘要校验，替换故障恢复旧二进制 |
| 重启 | 先重启遥测服务，60 秒内健康则依次重启 3 个 broker |
| 回滚 | 遥测 60 秒内未就绪：恢复 `.old`，重启全部服务，记录 `rolled_back`。broker 失败只记 `degraded` + `optional_service`，不阻断遥测升级（与 Linux 相同） |
| 状态 | `update-status.json` 沿用现有字段与枚举；不保存 stderr、URL 或凭据 |
| 清理 | 下一次成功检查时删除 `.old` |
| 中断 | 下次运行按受保护事务阶段核对文件存在性与摘要，恢复一致状态并报告 `interrupted`；禁止启动状态不明的文件 |

更新逻辑全部写在 Go 二进制里，不维护 PowerShell 版更新脚本。

## 9. 终端与批量执行

### 9.1 进程模型

- 用 `CreatePseudoConsole` 创建 ConPTY，通过 `STARTUPINFOEX` + `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` 和
  `CreateProcess` 启动 shell；新增 `internal/hostpty/process_windows.go`，不引入第三方 ConPTY 库。
- ConPTY 标志为 0，不使用 `PSEUDOCONSOLE_INHERIT_CURSOR`，避免启动时等待终端回应光标位置查询。
- 每个会话一个 Job Object，设置 `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`；shell 先 suspended 创建、加入 Job 后再恢复，
  消除其提前产生子进程的窗口。独立有界读取器排空 ConPTY 输出，避免关闭时被满管道阻塞。关闭会话、broker 退出或中心 epoch
  变化时关闭 Job，整棵进程树被终止，效果对应 Linux 的 transient unit 与 parent-death signal。
- 固定 shell：`<GetSystemDirectory()>\WindowsPowerShell\v1.0\powershell.exe -NoLogo -NoExit -Command
  "[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); $OutputEncoding=[Console]::OutputEncoding"`。
  启动前校验文件属主为 TrustedInstaller、SYSTEM 或 Administrators（实现 `terminalExecutableOwnerTrusted`
  的 Windows 版）。不按 PATH 查找，不接受浏览器提交的 shell、参数、环境变量或工作目录。
- 工作目录：SYSTEM 用户配置目录（`%SystemRoot%\System32\config\systemprofile`），对应 Linux 的 `/root`。
- PowerShell 使用 `-NoProfile`，避免未审计的配置文件影响服务身份 shell 的启动。

### 9.2 传输与限额

完全复用现有轻量终端链路：v2 Noise 中继加 `light-terminal-control`/`light-terminal-data` 流，
`TerminalOpenRequest` 等 payload 不变，`terminal-input-v1` 不变。限额不变：每节点 4 个会话、
单次输入 16 KiB、每会话 1 MiB 缓冲、30 分钟空闲关闭、8 小时上限。

Server 2016 没有 ConPTY：节点不声明终端能力，前端显示“仅监控”。

### 9.3 批量执行

中心 Host DTO 新增 `terminalShell`（`posix` | `powershell`），前端按它选择包装器。PowerShell 包装：

用户命令包在独立 PowerShell 子进程内，经 UTF-16LE `-EncodedCommand` 传入；固定使用当前受信任
`$PSHOME` 下的 PowerShell 可执行文件。子进程处理异常与退出码，父 shell 用 `"<marker>{0}__" -f $LASTEXITCODE`
输出完成标记。用户命令含 `exit`、here-string 或行尾注释也不会提前终止父 shell 的完成标记。

- 标记仍是每次随机 128 位；回显行只含字面 `{0}`，命令输出无法伪造随机值（与 POSIX 版 `%s` 相同性质）。
- 退出码解析放宽为有符号 32 位整数（Windows 进程可能返回 `-1073741819` 这类值）。
- 标记未出现时，沿用“提示符稳定后发送 `exit`”的回退逻辑，PowerShell 同样接受 `exit`。
- 不新增独立的远程执行接口，批量执行继续复用已认证的终端会话（`docs/multi-host-terminal.md` §5）。

## 10. 文件管理

### 10.1 路径模型

- 虚拟根 `/` 列出全部固定卷（`GetLogicalDrives` + `GetDriveTypeW == DRIVE_FIXED`），`/C/Users/...` 映射到
  `C:\Users\...`。每个卷各用一个 `os.Root`，不跨卷解析。
- 可移动盘、网络盘、光驱不列出。

### 10.2 必须拒绝的路径

| 输入 | 原因 |
| --- | --- |
| 盘符之外的 `:`（备用数据流，如 `a.txt:secret`） | 隐藏数据与校验绕过 |
| `\\?\`、`\\.\`、`\??\` 前缀 | 绕过 Win32 路径规范化，访问设备命名空间 |
| UNC（`\\server\share`） | SYSTEM 会用计算机账户向远端发起 SMB 认证 |
| `CON`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9`、`AUX`、`PRN`（含带扩展名形式） | 保留设备名 |
| 以点或空格结尾的名称 | Win32 会静默去掉，导致校验对象和实际对象不一致 |

### 10.3 受保护目录

受保护目录（`%ProgramData%\KejilionNode`、`%ProgramFiles%\KejilionNode`）不做文本前缀比较：
打开句柄后用 `GetFinalPathNameByHandle` 取规范路径，大小写不敏感比较。这样 8.3 短名
（如 `C:\PROGRA~3\KEJILI~1`）、大小写变体和 junction 都无法绕过。

### 10.4 Windows 语义差异

- **重解析点**：junction、符号链接、OneDrive 占位符、AppExecLink 都按符号链接处理，拒绝穿越，
  与 Linux “浏览时拒绝符号链接组件”一致。
- **权限**：SYSTEM 不像 root 那样无视 DACL。拒绝 SYSTEM 的目录（个别用户配置目录、`WindowsApps`、
  EFS 加密文件）返回明确的“权限不足”。是否启用 `SeBackupPrivilege`/`SeRestorePrivilege` 绕过见 D8。
- **chmod 与属主**：中心 Host DTO 新增 `pathStyle`（`posix` | `windows`）；Windows 主机隐藏 chmod 与属主 UI。
  只读属性留作后续，不在本期范围。
- **占用与杀毒**：删除、替换、改名遇到 `ERROR_SHARING_VIOLATION`/`ERROR_LOCK_VIOLATION` 时有界重试
  （≤ 2 秒），仍失败则返回明确的“文件被占用”。
- **回收站**：使用受保护的状态目录 `file-trash`。跨卷复制/恢复必须保留原文件和目录 DACL，
  覆盖保存的临时文件在创建时即带受保护 ACL，不留“先继承宽权限、随后收紧”的句柄窗口。
- **编辑器**：读取和保存保持原始字节，不转换 CRLF；UTF-16 文件按二进制处理，不提供在线编辑。
- **压缩包**：zip 正常支持；tar 中的符号链接和设备文件在 Windows 上拒绝解出。

限额与 Linux 文件链路相同：大文件/短请求各全局 16、每身份 4；上传 2 路并发。

## 11. 登录事件

| 来源 | 事件 | `method` 取值 |
| --- | --- | --- |
| OpenSSH Server | `OpenSSH/Operational` 中的成功认证 | `ssh-password`、`ssh-publickey` |
| 远程桌面 | `Security` 日志 4624，LogonType 10 | `rdp` |

- 由 `KejilionNodeLogin` 通过 `EvtQuery` 读取：每 5 秒检查一次，15 秒缓存，只保留最新一条成功事件；
  写入 `run\login-event.json`，对遥测服务只读开放。不传原始日志、不传凭据。
- 继续使用 `contract.SSHLoginEvent` 与 `ssh-login-v1` 能力；前端在 Windows 主机上把文案显示为“远程登录”。
- 本机控制台登录（LogonType 2）和网络登录（LogonType 3）不上报：后者数量大且噪声高。

## 12. 服务探测与运营商延迟

- `ping` 类型改用 `IcmpSendEcho2`（iphlpapi，无需管理员）；`tcp`、`http` 类型沿用现有实现。
- 运营商延迟（`internal/monitoring/operator_latency.go`）的 ICMP 部分同样替换；TCP/53 与 UDP DNS 不变。
- Docker 容器指标：Windows 节点标记为不可用，不尝试连接 Docker Desktop 的命名管道。

## 13. 远程桌面（P4，可选）

### 13.1 方案选型

| 方案 | 结论 | 理由 |
| --- | --- | --- |
| 自研屏幕采集、编码和键鼠注入 | 否 | 需要在用户会话里运行辅助进程，还要处理 UAC 安全桌面；工作量和风险最高 |
| Guacamole（guacd） | 否 | 中心端多一个 C 守护进程或容器 |
| 公网开放 3389 | 否 | 暴露的 RDP 是最常见的入侵入口之一 |
| **浏览器 IronRDP + 节点 RDCleanPath 桥 + 现有 Noise 流** | **采用** | 复用系统 RDP；节点只转发字节；3389 不对外 |
| 本地 mstsc + 本地端口转发工具 | 备选 | 性能最好，但管理员电脑要装额外工具 |

[IronRDP](https://github.com/Devolutions/IronRDP) 是 Devolutions 维护的 Rust RDP 实现（MIT/Apache-2.0），
有 WASM 网页组件。浏览器不能直接建立 TCP 连接，IronRDP 约定由中间件完成 RDP 握手中的 TLS 升级，
这个约定叫 RDCleanPath。NetBird 的浏览器客户端也是 IronRDP 加自研 Go RDCleanPath 桥，证明 Go 侧可以实现。

### 13.2 链路

```text
浏览器（IronRDP WASM，打开桌面时才加载）
  ⇄ WebSocket  GET /api/v1/desktop-sessions/{id}/stream
     （Session + Origin 校验，复用终端输入流的 WebSocket 模式，子协议固定）
中心 paneld：会话分配、权限、审计、字节转发（不解析 RDP）
  ⇄ Noise 流，新角色 light-desktop-data（节点按 light-terminal-control 的指令回拨）
KejilionNodeTerminal 内的 RDCleanPath 桥（Go）
  ⇄ TCP 127.0.0.1:<RDP 端口>（Windows 自带 TermService）
```

- **目标固定。** 只连本机回环地址加注册表 `Terminal Server\WinStations\RDP-Tcp\PortNumber` 中的端口；
  不接受浏览器或中心指定主机和端口，所以不会变成通用端口转发或内网跳板。
- **不改系统配置。** 节点不开启 RDP、不改防火墙。RDP 未开启（`fDenyTSConnections=1`）时显示“远程桌面未开启”，
  管理员可以通过终端自行开启。远程桌面防火墙规则建议保持关闭，只走回环。
- **能力开关。** 增加 `desktop` 能力：安装时必须显式开启（`-Capabilities ...,desktop`），中心也能按主机关闭；
  已接入的节点不会自动获得。

### 13.3 浏览器 API

```text
POST /api/v1/desktop-sessions                 { hostId }          → { sessionId, nonce, expiresAt }
GET  /api/v1/desktop-sessions/{id}/stream     WebSocket（二进制）
POST /api/v1/desktop-sessions/{id}/close
POST /api/v1/desktop-sessions/policy          { hostId, allowed }
```

写操作校验 Session、Origin、CSRF；随机会话 ID 绑定管理员与本次登录 token 的摘要，同一用户另一登录也不可接管。
会话分配 1 分钟内仅可 claim 一次；32 字节随机 nonce 在 RDCleanPath 内再次校验。GET 流同样强制 Origin，
不允许 query 参数。IronRDP 不发送 WebSocket subprotocol，因此使用上述精确登录绑定而非自创子协议。
中心禁用策略独立持久化，立即关闭该主机的桌面 control/data，不影响命令行和文件；策略损坏时桌面失败关闭。

### 13.4 安全

1. 访问要过三道认证：KPanel 登录、Noise 节点身份、Windows 网络级身份验证（NLA）。
2. **中心能看到解密后的 RDP 流。** TLS 在节点终结，与终端现状一致（中心本来就能看到终端明文）。
   如果要求中心也看不到，需要把桥放进浏览器 WASM（D5）。
3. Windows 凭据每次在浏览器输入，KPanel 不保存。加入域的机器上，RDP 登录会在目标机留下凭据，
   文档建议使用本地运维账户。
4. 默认关闭驱动器、打印机、USB、智能卡和剪贴板重定向；本候选不提供重定向开关。
5. 审计只记录打开、关闭、主机、管理员和时长，不录屏、不记录输入。
6. RDCleanPath 请求按 DER 严格解码，大小有界，未知字段即断开。
7. 必须使用 NLA/CredSSP、TLS 1.2 及以上，拒绝 SSL-only 或无 TLS 降级。本机 listener 证书固定到系统证书存储：
   显式 `SSLCertificateSHA1Hash` 使用 LocalMachine MY；默认使用 WinStations 指定（缺省 Remote Desktop）存储。
   只比较真实 TLS 叶证书 DER，不信任连接对端自行提供的证书列表。

### 13.5 资源与稳定性

| 项目 | 规则 |
| --- | --- |
| 并发 | 每节点 1 个桌面会话；全局 4 个；每管理员 2 个 |
| 缓冲 | 每方向 ≤ 256 KiB；满时停止读取对端（背压），不无限堆积 |
| 连接上限 | 计入每身份 16 条流的既有上限 |
| 时长 | 30 分钟无数据关闭；最长 8 小时 |
| 断线 | Windows 会话保留为“已断开”，重连回到原桌面；broker 或中心重启后同样适用 |
| 带宽 | IronRDP 文档列出 RemoteFX 与 RDP 6.0 位图压缩，未列出 H.264；适合运维和办公画面，不适合视频（需实测） |
| 会话冲突 | 桌面版 Windows 只允许一个交互会话：连入前提示会锁定或挤下本机用户 |

### 13.6 前端

- Windows 主机沿用终端左侧列表；点击主机先选择“命令行（PowerShell） / 远程桌面（RDP）”，
  随后在右侧打开带类型的会话页签；Linux 保留点击直接命令行。RDP 不可用时显示具体原因。
- 两类会话分别管理连接与关闭；批量命令只进入命令行。RDP 断开保留 Windows 登录会话，关闭 Shell 终止 PTY。
- IronRDP 组件仅选择 RDP 后懒加载，不进入首屏。
- 只在桌面端浏览器提供完整体验；手机上显示“建议使用桌面浏览器”，但不禁用。
- 使用官方 npm 精确版本 `@devolutions/iron-remote-desktop-rdp@0.7.0` 与 GUI `@devolutions/iron-remote-desktop@0.11.0`，
  按既有 dependency policy 的 npm 真源管理；RDP npm provenance 对应源码 `e45f68c7e52297ca50d33b44c0ace36c9940fbe6`。
  Vite 从包内提取 WASM 为同源静态资源（约 4.51 MB，gzip 1.62 MB），无需 CDN；CSP 仅增加 `wasm-unsafe-eval`。

### 13.7 立项前 spike

在 Windows 11 Pro 与 Server 2022 虚拟机上，用本地 Noise 流打通 Go RDCleanPath 桥和 IronRDP 组件，测量：

1. WASM 体积与首帧时间；
2. 典型运维操作的带宽与输入延迟；
3. 隧道断开、broker 重启后的重连；
4. 远程桌面防火墙规则关闭时，回环连接是否可用；
5. NLA 开启时能否正常登录。

spike 结论不达标就终止 P4，P1–P3 不受影响。

## 14. 中心端与前端改动

| 位置 | 改动 |
| --- | --- |
| `internal/cluster/signature.go` | 新能力 `windows-node-v1`；P4 增加 `desktop-v1` |
| `internal/cluster/light_service.go`、`light_batch_service.go` | 按平台生成 PowerShell 接入命令；令牌与 API 不变 |
| `internal/cluster/types.go` | Host DTO 增加 `platform`、`terminalShell`、`pathStyle`；P4 增加 `desktopAvailable` |
| `internal/cluster/stream_v3.go` | P4 增加 `light-desktop-data` 角色 |
| `internal/panel` | P4 增加桌面会话 API 与 WebSocket 端点 |
| `.github/workflows/release.yml` | 构建、签名 `kejilion-node-windows-{amd64,arm64}.exe`，写入 `SHA256SUMS` |
| CI | 增加 Windows 构建，以及 `cmd/kejilion-node`、`internal/systeminfo`、`internal/hostpty`、`internal/filemanager` 的 Windows 单元测试 |
| `web/src/lib/operatingSystem.ts` | 增加 Windows 识别 |
| `web/src/lib/batchCompletion.ts` | PowerShell 包装与 int32 退出码 |
| `web/src/views/ClusterView.vue` | “Windows 主机”接入选项；隐藏负载；“计划任务”文案 |
| `web/src/views/FilesView.vue` | `pathStyle=windows` 时隐藏 chmod/属主 |
| `web/src/views/TerminalView.vue` | P4 “桌面”页签 |
| `web/src/i18n` | zh-CN、zh-TW、en-US 同步（`docs/internationalization.md`） |
| 文档 | `platform-support.md` 增加 Windows 层级；`cluster-monitoring.md` §3.3 与 `multi-host-terminal.md` 增加 Windows 说明 |

界面改动遵守 `docs/ui-visual-language.md`，并按 `docs/local-feature-preview-standard.md` 提供绑定候选提交的预览。

## 15. 兼容与回滚

| 组合 | 行为 |
| --- | --- |
| 新中心 × Linux 轻量节点 | 不变 |
| 旧中心 × Windows 节点 | 接入时拒绝 Windows 外层字段，不消费令牌；已接入后回滚中心，Windows 停止管理连接并重试，不伪装 Linux |
| 新中心 × 未开启 `desktop` 的 Windows 节点 | 没有“桌面”页签 |
| 中心回滚到不认识 Windows 的版本 | 节点离线重试，与 Linux 节点遇到旧中心相同；中心状态文件不需要迁移 |
| 节点回滚 | 更新失败时自动用 `.old` 回滚；退回更早的稳定版需要先卸载，再用该版本安装器重新安装（更新器不降级） |
| 彻底移除 | 节点执行 `kejilion-node.exe uninstall`，中心删除主机记录 |

新增 Host DTO 字段都是可选字段，旧前端忽略。新状态（如健康快照）只存在于内存，回滚不需要数据迁移。

## 16. 威胁模型

| 威胁 | 控制 | 验证 |
| --- | --- | --- |
| 预先创建 `%ProgramData%\KejilionNode` 或 junction，诱导 SYSTEM 写入 | 安装前逐级检查属主、DACL、重解析点；显式 DACL；禁用继承 | 以普通用户预建目录和 junction 后安装，必须失败 |
| 服务路径未加引号 | ImagePath 加引号；安装后读回校验 | 单元测试 + 实机 `sc qc` |
| DLL 劫持 | 静态 Go 二进制；全部用 `NewLazySystemDLL`；二进制只放 `%ProgramFiles%` | 代码检查禁止 `NewLazyDLL`；实机用 Procmon 检查加载路径 |
| 密钥文件 ACL 被放宽 | 启动前 `GetSecurityInfo` 校验，失败拒绝启动 | 改坏 ACL 后启动必须失败 |
| 文件管理路径绕过（8.3、ADS、设备路径、UNC、末尾点） | 第 10 节拒绝规则；规范路径比较 | 表驱动测试覆盖每一类 |
| 供应链篡改 | SHA-256 + Authenticode 固定发布者；更新不降级 | 篡改字节、换签名、旧版本三类测试都必须拒绝 |
| 令牌泄露 | 不进注册表和服务参数；环境变量选项；短有效期建议 | 安装后检查注册表与服务配置不含令牌 |
| 伪造遥测或重放 | 沿用 HMAC、时间窗、重放缓存 | 沿用现有测试 |
| 中心被攻破 | 与 Linux 相同：可在节点获得 SYSTEM。远程桌面额外暴露“看屏幕、操作已登录会话”的能力，因此默认不安装 | 文档与接入界面明确说明 |
| 加入域的机器上的横向移动 | 安装时检测入域；建议只装遥测（D3） | 实机检查默认能力 |
| 杀毒软件或 EDR 误报导致节点被隔离 | 签名；VERSIONINFO；不加壳；发布前提交 Defender 误报复核 | 首批实机上 Defender 默认策略不告警 |
| 终端子进程残留 | Job Object `KILL_ON_JOB_CLOSE` | 在终端中启动后台进程后关闭会话，进程必须消失 |
| 远程桌面被用作端口转发 | 目标固定为本机 RDP 端口 | 篡改请求指定其他地址必须失败 |

## 17. 性能与资源预算（建议值，P1 实现时用 Linux 基线校准）

仓库目前没有 Linux 轻量节点的内存与 CPU 基线。P1 第一步先在同等规格 Linux 主机上测出基线，再按下表确定 Windows 上限：

| 项目 | 预算 |
| --- | --- |
| 二进制 | ≤ 12 MB（实测 10.1 MB） |
| 单个服务空闲工作集 | ≤ Linux 基线 × 1.2 |
| 一次遥测采样 CPU | ≤ 5 ms（不含 150 ms CPU 采样等待） |
| 稳态子进程 | 0 |
| 网络 | 与 Linux 相同：遥测每台约 2 请求/分钟；终端空闲时一条长轮询 |
| 历史采样 | 与 Linux 相同：主机每分钟，线路每 5 分钟 |
| 远程桌面（P4） | 节点只转发字节；中心按带宽计费，spike 后确定每会话上限 |

## 18. 稳定性与故障恢复

| 场景 | 行为 |
| --- | --- |
| 服务崩溃 | 服务控制管理器 5/30/60 秒后重启；1 天后重置计数 |
| 开机或 Windows 更新重启 | 服务自动启动；先向服务控制管理器报告 Running，再连接网络 |
| 休眠或睡眠后唤醒 | 立即上报并重建连接；时钟偏差超出时间窗时报告 `clock_skew`，不自动改系统时间 |
| 中心不可达 | 有界指数退避，不在本地堆积报告；历史采样继续在本地保存 |
| 中心重启（epoch 变化） | 关闭本地旧终端和桌面会话 |
| 更新中断 | 第 8 节：锁记录 PID 与启动时间，下次运行恢复 |
| 磁盘满 | 原子写入失败不覆盖旧文件；健康快照标记不可用 |
| 杀毒软件占用文件 | 有界重试后返回明确错误 |
| 代理环境 | Go 只读取环境变量代理，不读 WinHTTP/PAC；安装器提供 `-Proxy`，写入服务环境变量。TLS 解密代理能看到遥测内容（只有 HMAC 完整性保护），终端、文件和桌面仍受 Noise 保护 |
| 日志 | 写入 Windows 事件日志 Application 来源 `KejilionNode`；不含令牌、密钥或命令内容 |

“休眠”与“离线”的区分（避免误发离线告警）需要扩展契约，不在本期范围。

## 19. 编码前质量记录

```text
流量路径：浏览器 → 中心 paneld；Windows 节点四个服务只出站连接中心 HTTPS（遥测 HMAC；终端、文件、历史、桌面走 Noise）；
          节点内部只通过带 ACL 的快照文件交换；RDP 桥只连本机回环 RDP 端口。
不可信输入：令牌、中心响应、Noise 信封、文件路径与名称、压缩包条目、RDCleanPath 请求、事件日志内容、注册表值、
          杀毒软件导致的文件状态变化。
权限与可写范围：遥测虚拟账户只读配置；SYSTEM broker 可写全部固定卷（与 root 等价，受第 10 节路径规则约束）；
          更新任务只写安装目录与状态目录。
最坏输入/输出字节数：沿用 Linux 轻量节点：中心响应 64 KiB、快照 4 KiB、终端输入 16 KiB、每会话 1 MiB；
          桌面每方向 256 KiB 缓冲。
最大并发、CPU 和内存：终端每节点 4；文件大/短请求各全局 16、每身份 4；桌面每节点 1、全局 4；内存见第 17 节。
超时、取消与重试：沿用 Linux 轻量节点；文件占用重试 ≤ 2 秒；更新健康检查 60 秒。
真实状态来源与缓存失效：Win32 API 与服务控制管理器实时读取；CPU 型号 30 分钟刷新；公网信息沿用 30 分钟缓存。
失败、回滚和重启恢复：第 8、15、18 节。
性能预算影响：第 17 节；中心端无额外开销。
网络入侵风险：第 16 节。
受影响业务域与用户旅程：集群接入、集群列表与详情、终端与批量执行、文件管理、历史监控、通知、P4 远程桌面。
桌面/移动端、键盘/焦点、多语言和失败反馈：沿用现有页面；新增文案三语同步；远程桌面以桌面端为主。
证据：自动测试（Windows CI 单元测试与契约测试）、隔离真机（第 21 节矩阵）、公开产物（签名 Release 资产与安装器）、
      生产部署安全核对（不适用于节点本身，适用于中心发布）。
```

## 20. 分期与任务拆分

| 期次 | 内容 | 主要位置 | 等级 |
| --- | --- | --- | --- |
| P0 | Linux 资源基线；签名证书就绪；Windows CI 构建 | CI、`release.yml` | L1 |
| P1 | Windows 采集器；ACL 校验替换全部 fail-open 分支；服务安装与生命周期命令；Go 更新器与计划任务；健康映射；安装器；中心 `windows-node-v1`、PowerShell 接入命令、Windows 图标、隐藏负载、“计划任务”文案 | `cmd/kejilion-node`、`internal/systeminfo`、`internal/cluster`、`web/src` | L3 |
| P2 | ConPTY 终端与 Job Object；PowerShell 批量包装；多卷文件管理与路径规则；历史采样 | `internal/hostpty`、`internal/terminal`、`internal/filemanager`、`internal/agent`、`web/src` | L3 |
| P3 | 登录事件；ICMP 探测与运营商延迟 | `internal/cluster/sshlogin`、`internal/monitoring` | L2 |
| P4 | 远程桌面 spike → 实现 | `cmd/kejilion-node`、`internal/cluster`、`internal/panel`、`web/src` | L3 |

- P1 是对外发布的最小单位：节点不能自我更新就不能交付。
- 每期独立形成候选、独立复核、独立发布；P4 不阻塞 P1–P3。

## 21. 验收与治理

- **等级。** 新安装路径、新信任边界、新发布资产按 L3 处理；P3 不改变安装与发布，按 L2。
- **安全审计。** 每期按 `PROJECT_RULES.md` 5.4 执行 `security-boundary-audit`（`profile=scoped`），
  提交写 `Security-Audit:` trailer。
- **行级评审。** 代码改动按 5.5 执行 OCR 行级评审并写 `OCR-Review:` trailer。
- **独立复核。** Claude 实现的候选交由其他提供商复核（`docs/multi-agent-collaboration.md`）。
- **跨仓库联动。** `scriptLinkageState` 取决于 D2：安装器放在 `kejilion/sh` 则为 `coupled`；
  作为 KPanel Release 资产则为 `not-required`，并记录脚本基线。
- **实机矩阵。** 在登记的隔离环境（不得使用 `prod-108`）完成：

| 系统 | 安装 | 重启恢复 | 自动更新 | 回滚 | 卸载 | 终端 | 文件 | 登录事件 | 桌面（P4） |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Server 2022 amd64 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Server 2025 amd64 | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Windows 11 24H2 amd64（简体中文） | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Server 2016 | ✓ | ✓ | ✓ | ✓ | ✓ | 不声明 | ✓ | ✓ | — |
| Windows 11 arm64（第二批） | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

- **专项检查。** 简体中文系统（代码页 936）下终端中文输入输出；入域机器的默认能力；Defender 默认策略不告警；
  受限语言模式下安装器明确失败；预建目录与 junction 攻击被拒绝；断网、休眠唤醒、Windows 更新重启后的恢复。
- **浏览器。** 集群接入、终端、批量执行、文件管理、远程桌面的桌面与移动端验收，按后台浏览器作业执行。

## 22. 竞品复核

**Komari agent `828afaf`（2026-09-23，GitHub `komari-monitor/komari-agent`，代码阅读，未实机运行）**

| 维度 | Komari | 本设计 |
| --- | --- | --- |
| 传输 | 允许 `http://`（自动转为 `ws://`）；有 `--ignore-unsafe-cert`；长期 token 放在 URL 查询参数 | 只允许 HTTPS；Noise 身份绑定；HMAC 时间窗与防重放 |
| 进程权限 | 整个 agent 是一个 LocalSystem 进程 | 四服务权限拆分 |
| 安装 | 通过 nssm 2.24 注册服务；安装脚本下载 agent 与 nssm 时都不校验 | 原生 `svc/mgr`；SHA-256 + Authenticode |
| 更新 | 9-23 起校验 GitHub API 返回的 digest；服务端可远程切换版本 | 同源 SHA-256 + 签名；中心不推送更新 |
| 密钥 | token 作为参数存进服务注册表 | 带 ACL 的文件 |
| 远程命令 | 独立 `agent.exec`：无超时、输出无上限、命令原文写日志 | 复用有界终端会话；审计不记录输入输出 |
| 终端 | PATH 查找 `powershell.exe`；工作目录为安装目录；无 Job Object | 绝对路径与属主校验；Job Object |
| 文件 | 虚拟根 `/C/...`；未做路径限制 | 同样的虚拟根，加第 10 节路径规则 |
| 远程桌面 | 不支持 | P4 可选，复用系统 RDP |

借鉴并已纳入本设计：多卷虚拟根；build ≥ 22000 改名为 Windows 11；PowerShell 强制 UTF-8 输出；
一个开关控制全部远程控制能力（对应 `-Capabilities`）。

记录取舍：Komari 用 gopsutil 模拟负载均值，本设计显示“—”；Komari 给已登录用户弹“本机已安装远程代理”提示，
本设计暂不采用，共享桌面场景可在后续评估；GPU（DXGI）与进程数不在 Linux 轻量节点能力范围内，不纳入。

**NetBird 浏览器客户端**（[架构文档](https://docs.netbird.io/manage/peers/browser-client/architecture)，2026-10-03 检索）：
IronRDP WASM 加自研 Go RDCleanPath 桥，作为第 13 节方案的先例。

## 23. 决策与发布前置事项

| 编号 | 问题 | 影响 | 建议 |
| --- | --- | --- | --- |
| D1 | 代码签名证书：OV、EV 还是 Azure Artifact Signing | P1 发布前置条件 | 优先 Microsoft 托管签名服务（Azure Trusted Signing，可在 CI 中签名），需确认主体资格与费用 |
| D2 | 安装器放在 `kejilion/sh` 还是 KPanel Release 资产；是否提供国内镜像 | `scriptLinkageState`、接入命令、下载可用性 | Release 资产 + 签名；镜像只作为不可信传输通道，校验不减 |
| D3 | 加入域的机器默认装什么能力 | 默认风险 | 默认只装遥测，显式参数才装终端、文件、桌面 |
| D4 | 首批版本范围；是否包含 arm64 | 实机矩阵规模 | 首批只做 amd64 |
| D5 | 远程桌面：接受中心可见解密 RDP 流，还是要求端到端 | 前端体积与复杂度 | 先接受中心可见（与终端一致），记录取舍 |
| D6 | 远程桌面剪贴板默认开还是关 | 数据外泄面 | 默认关闭，按会话开启 |
| D7 | 确认不支持家庭版远程桌面 | 用户预期 | 确认不支持 |
| D8 | 文件管理是否启用 `SeBackupPrivilege` 越过 DACL | 与 root 能力对齐程度 | P2 不启用，返回明确权限错误；后续按需求评估 |
| D9 | 手动启动类型映射到哪个 `unitFileState` | 健康显示 | 映射到 `static`，不扩展枚举 |

## 24. 未验证风险

- 虚拟账户能否通过 `SERVICE_QUERY_STATUS` 查询另外三个服务（不能则改为 broker 快照）。
- Event Log Readers 成员能否读取 `Security` 通道（不能则登录事件服务改为 SYSTEM 并限制特权）。
- RDP 防火墙规则关闭时，回环连接是否可用。
- PSReadLine 在 ConPTY 中处理多行粘贴时，PowerShell 批量包装是否稳定。
- Defender、主流 EDR 对节点二进制和行为的告警情况。
- IronRDP WASM 的体积、带宽和延迟，以及其 RDCleanPath 细节与 Go 实现的兼容性。
- Windows 轻量节点的实际内存占用（依赖 P0 的 Linux 基线）。

## 参考

- [IronRDP](https://github.com/Devolutions/IronRDP)
- [IronRDP web API（RDCleanPath）](https://www.mintlify.com/Devolutions/IronRDP/api/ironrdp-web)
- [NetBird 浏览器客户端架构](https://docs.netbird.io/manage/peers/browser-client/architecture)
- [Komari agent](https://github.com/komari-monitor/komari-agent)
