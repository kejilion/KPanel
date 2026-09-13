# KPanel 跨面板文件复制

## 任务契约

- 任务 scope / 角色 / AI：`cross-kpanel-file-transfer` / development / Codex。
- 目标与用户价值：允许用户把 KPanel A 文件窗口或桌面上的一个或多个普通文件/目录拖到 KPanel B 桌面或文件管理窗口；B 将内容复制到指定目录，桌面目标在复制成功后创建入口。
- 允许路径：`internal/cluster`、`internal/filemanager`、`internal/agent`、`internal/panel`、`internal/contract`、`web/src`、本文件及针对性测试。禁止路径：安装/升级/发布脚本、生产配置、版本号、数据库迁移。
- 共享契约：Cluster v2 Noise 身份、Panel-Agent Unix socket、文件路径/resourceVersion、桌面 workspace。
- 业务真源与用户旅程：`docs/cluster-monitoring.md`、`docs/desktop-icon-workspace.md`、`docs/storage-strategy.md`；A 文件行或桌面文件/目录快捷方式拖拽 -> B 桌面/文件窗口识别 -> B 后端向已配对 A 拉取 -> B Agent 原子落盘 -> 桌面目标创建入口。
- worktree / branch / base / rollback：`C:\GitHub\kejilion-panel-cross-kpanel-file-transfer` / `feat/cross-kpanel-file-transfer` / `origin/main@431b9ca5ac21d5a3d1629e46e209ce718367569f` / 源实现回滚到 `431b9ca5ac21d5a3d1629e46e209ce718367569f`；正式版本继续记录上一稳定 Tag、镜像与生产备份。
- 依赖、相邻任务与冲突面：依赖现有 Cluster v2 配对；相邻 worktree 均未修改上述实现路径，治理 worktree 的脏文件不重叠。
- 风险等级：L2。新增跨前后端网络认证、流式传输与 Agent 文件写入；不发布、不改安装链。
- 验收：Go 定向测试与全量测试、Web 单测/typecheck/build、两个本地 KPanel 浏览器跨窗口拖放；长时间浏览器测试按 registered arena 工作流执行。
- 权限：用户已明确授权将文件互传及当前可上线内容提交并进入上线流程；由唯一发布任务负责候选推送、更新 main、tag、Release 与已授权生产部署。
- 交付物：可审阅差异、设计文档、测试与浏览器证据、已知限制。

桌面来源扩展在 `C:\GitHub\kejilion-panel-desktop-cross-panel-transfer`、分支
`feat/desktop-cross-panel-transfer` 上基于 `origin/main@f982f2e8c4e08dfda190ae63938c8114acff4006`
实现；该任务只形成可重放提交，不推送、不更新 main、版本、Tag、Release 或生产环境，由唯一 release writer 集成。

## 产品规则

1. 跨面板操作永远是复制，不提供移动语义；源文件不发生修改。
2. 拖到 B 桌面时，默认目标为 B 的桌面文件目录（初始值 `/home/KPanel Desktop`）。文件复制完成后才创建桌面入口。
3. 同名目标采用 `name (1)`、`name (2)` 的可预期规则，不覆盖现有内容。
4. 传输中显示来源、目标、当前字节数并允许取消。失败不留下可见的半成品；快捷方式保存失败与文件复制失败分开报告。
5. 只有已通过 Cluster v2 配对且明确带有 `cluster.files.read` scope 的来源节点可用。新版新增主机时会在同一次管理操作中建立双向文件授权；旧配对不会静默扩权，可在主机管理中显式“启用双向文件互传”，无需重新配对。
6. 一次最多接收 64 个顶层文件或目录。批量传输按选择顺序逐项执行，每项独立原子提交；单项失败不阻断后续项目，并在结束时展示成功/失败明细。
7. 拖到 B 桌面时复制到桌面文件目录并批量创建入口；拖到 B 文件管理空白区时复制到当前目录，拖到目录项时复制到该目录，不创建桌面入口。
8. 跨面板拖放始终是复制。中心添加 A 后，原 Host 授权中心读取 A，独立的互传 grant 授权 A 读取中心；仅对文件能力双向，不反向授予概要、终端或主机管理权限。
9. 桌面只有 `file` / `directory` 快捷方式可以跨面板传输；应用、网站、URL 和固定系统入口只参与本地布局。混合框选拖动时跨面板描述符只包含已确认可读的文件/目录并提示跳过数量；同一桌面内落下仍移动整组选中图标。
10. 桌面来源在后台通过单次有界 `POST /api/v1/files/entries` 获取最多 64 项当前元数据与 `resourceVersion`。缺失、无权限或类型变化的入口不可开始跨面板拖拽，避免拖到目标后才暴露陈旧引用。

## 协议与安全

浏览器兼容读取单项 MIME `application/x-kpanel-cross-panel-file-v1`；批量拖拽使用
`application/x-kpanel-cross-panel-files-v2`，只包含来源 `nodeId` 和最多 64 个项目的文件名、规范化绝对路径、类型及 `resourceVersion`。描述符不是授权凭据；B 后端必须为每项使用本地保存的配对密钥找到 A 并重新鉴权。旧目标仍可接收新来源的单项 v1 描述符，不会把批量误当成单项。

桌面文件/目录快捷方式在宽屏鼠标环境使用浏览器原生 HTML5 drag，以便跨标签页、窗口和域名传递上述
自定义 MIME；WebKit 跨域会隐藏自定义 MIME，因此同时提供不含凭据的带标记 `text/plain` 描述符，
目标仍执行相同严格解析与后端重新鉴权。同一桌面的 drop 只更新 workspace 位置。触摸、手写笔和紧凑布局继续使用既有
Pointer Events 布局手势。来源与目标 `nodeId` 相同时拒绝跨面板路径，避免把本机布局拖动误解为远程复制。

B Panel 通过 Cluster v2 Noise IK 向 A 的固定端点请求文件。握手响应携带经认证的文件元数据，随后复用握手产生的 Noise transport cipher，以长度帧传输加密正文和经认证的结束记录。此方式同时覆盖 HTTPS 与受允许私网中的 `http://IP:port`，且不放宽 SSRF、重放、时钟偏差、并发和空闲超时规则。

一次新版配对复用现有两把静态密钥建立反向文件专用通道，不创建第二条 Host，也不复制私钥：中心通过 `/api/v2/federation/files/link` 下发随机 `linkId`、中心节点 ID 和经浏览器 Origin/CSRF 验证的回连地址；A 将其保存为 route。A 读取中心时只调用独立的 `/api/v2/federation/files/open-linked`，Noise 角色反向使用，且中心必须同时验证未过期的本地 grant、原 active Host、事务、节点、指纹及 Host credential。正向 `/files/open` 与反向 `/files/open-linked` 的 prologue 相互隔离，认证失败不做 fallback。

route/grant 只写入独立 `cluster-file-peers-v2.json`，不含任何密钥；每 10 分钟认证续租、30 分钟过期。删除 Host 或 Controller 会立即使父关系失效并清理互传记录，后续新传输立即拒绝；已经完成授权并进行中的单项流按既有取消或超时规则结束。回连地址支持 HTTPS 域名或 IP，也支持 `http://字面量IP:非80端口`，并继续执行 DNS、私网 allowlist 与 rebinding 校验；不强制使用域名。新版与不支持 link 端点的旧版配对仍保持原单向能力，协商失败不会回滚主配对。

目录使用受限 TAR 流传输：拒绝 symlink、special file、路径穿越和超出既有 entry/byte budget 的内容。B Agent 解包到目标目录内的隐藏临时目录，完整结束并同步后以 no-replace rename 原子发布。

审计只记录来源节点、类型、字节数、目标目录和结果；不记录 Noise 明文、密钥或拖拽描述全文。

## 状态机

单项：`idle -> connecting -> transferring -> committing -> shortcut? -> complete`

批量：`idle -> item[n].connecting -> item[n].transferring -> item[n].committing -> next|failed -> shortcut? -> complete|partial`

取消或错误进入 `cancelled` / `error`。在 `committing` 前取消会清理临时文件；文件已提交而桌面入口失败时进入 `partial`，保留真实文件并允许用户手动重试“添加到桌面”。

## 与操作系统下载和拖出的边界

- 本协议支持的是 KPanel 页面内、跨标签页、跨窗口和跨 KPanel 的文件复制。目标 KPanel 会重新鉴权、
  拉取并原子提交文件；该成功状态不依赖 Windows Explorer 或 macOS Finder。
- 用户把内容保存到本机时，可靠兜底是显式下载：单个普通文件使用短时 download ticket，单个目录或
  批量选择先用已认证 POST 封存选择和版本，再通过不含路径的短时 ticket 下载受限 ZIP。内部及跨
  KPanel 拖拽在这些入口之外继续保留。
- Chromium 私有 `DownloadURL` 作为尽力而为的渐进增强与 KPanel 自定义载荷并存。Windows 实机中，
  同一远端目标、同一 `127.0.0.1` 发起源的 `text/plain` / `application/octet-stream` A/B 曾同时成功，
  随后全部载荷失败；全新标签页恢复原文件名和 `no-referrer` 均未改变失败，改用 `localhost` 发起后
  四种载荷又全部成功。结果因此属于会随 initiator、tab、referrer、navigation 环境翻转的下游安全分类，
  不是固定 MIME、文件名、custom type 或载荷顺序问题。
- Chromium 源码已排除 automatic download limiter 是这批 `FILE_BLOCKED` History 记录的机制；当前
  Google Transparency 公开站点状态也未报告威胁，但它不等于 download protection 对具体下载的 verdict。
- Web 页面没有 Chromium 下载器或 Explorer 最终落盘结果的回调；HTML `dragend` 不能证明文件已保存，
  因此操作系统拖出不得进入跨 KPanel 复制的成功状态、失败重试或可靠性承诺；被客户端阻止时只引导
  使用显式 ticket / ZIP，不影响内部或跨 Panel 拖拽。
- 候选不应写成已修复 `kpanel.kejilion.eu.org` 的客户端拦截，也不通过伪装文件类型、删除安全响应头、
  延长 URL 凭据、改用其他下载域或其他方式规避浏览器、操作系统或组织策略。`127.0.0.1` / `localhost`
  对照改变的是发起 origin，不证明切换下载域有效。
- 完整现场证据与 KodBox 对照见
  [`windows-file-download-compatibility.md`](windows-file-download-compatibility.md)。

## 明确非目标

- 把 KPanel 到 Windows/macOS 的原生拖出作为跨面板复制完成条件或兼容保证。
- KPanel A 到 B 的移动、双向同步、断点续传、后台任务跨刷新恢复。
- 未配对面板、Cluster v1 或 light node 的文件传输。

## 开发验收记录

- Go：`internal/agent` 全量测试、新增批量元数据 Agent/Panel 定向测试及 Panel 文件路径定向测试通过；变更范围 `go vet` 通过。Windows 下 Panel 全量测试仍受既有绝对路径与静态资源 fixture 基线限制，不作为本功能失败。
- Web：Vitest 97 个测试文件、719 项测试通过；i18n 校验、`vue-tsc --noEmit`、Vite 生产构建与预压缩通过。
- 浏览器：本地生产构建在应用内真实 Chromium 中完成经典页、桌面壳和桌面文件窗口烟测，页面结构正常且控制台无 warning/error。预览 workspace 没有文件快捷方式，且不具备双 Agent 和 Cluster v2 配对，因此没有把该结果记录为桌面跨面板传输 E2E；跨窗口 HTML5 drag、批量描述符、WebKit 文本回退和本地组选中移动由组件/单元测试覆盖，不代表 Windows/macOS 原生拖出已通过。
- 待 release writer：在两台已配对 Linux KPanel 上验证文件窗口及桌面快捷方式来源的单项、多选、混合框选分别拖到桌面、文件管理当前目录与目录项；同时验证 Chrome/Safari、双方向授权、同名副本、部分失败、取消、来源变更、旧配对拒绝、断链清理及快捷方式失败保留真实文件。
