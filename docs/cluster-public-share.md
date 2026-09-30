# KPanel 集群公开分享

## 任务契约

- 任务 scope / 角色 / AI：`cluster-public-share` / development / Codex。
- 目标与用户价值：管理员可生成一个匿名只读链接，集中展示本机和已接入集群机器的在线状态、地区、系统与资源使用情况，用于公开展示。
- 允许路径：`internal/panel`、`internal/store`、`web/src`、集群相关文档与针对性测试。禁止路径：Agent/联邦协议扩权、安装升级脚本、版本号、正式发布与生产配置。
- 业务真源与用户旅程：[`cluster-monitoring.md`](cluster-monitoring.md)、[`storage-strategy.md`](storage-strategy.md)；集群页打开“公开分享” -> 显式启用并保存 -> 复制随机链接 -> 游客匿名查看 -> 管理员关闭或重置链接。
- worktree / branch / base / rollback：`C:\GitHub\kejilion-panel-codex-cluster-public-share` / `feature/cluster-public-share` / `origin/main@f982f2e8c4e08dfda190ae63938c8114acff4006` / 删除本任务差异即可回到基线，未修改现有集群状态或联邦凭据格式。
- 依赖与冲突面：复用现有 `cluster.Service.Hosts` 缓存列表与 `panel-state.json` 原子存储；不改变 Cluster v1/v2/light-v1 协议。
- 风险等级：L2。新增匿名 HTTP 页面和 API，必须覆盖默认关闭、不可猜链接、严格输出白名单、限流、缓存上限、Host/安全入口规则、失效与回滚。
- 验收：Store/Panel Go 定向和全量测试、Web i18n/typecheck/Vitest/build、L2 门禁、Linux 构建与匿名页面浏览器复核。
- 权限：仅授权本地实现和验证；未授权提交、推送、更新 `main`、发布或部署。
- 交付物：实现差异、设计与隐私契约、自动化验证证据、未验证风险和回滚说明。

## 产品规则

1. 分享默认关闭。首次开启时生成 32 字节随机值并以 64 位十六进制形式进入 URL；链接可随时关闭或重置。
2. 获取链接的人无需登录。分享路由不请求 Session，不跳转登录页；面板启用安全入口时，只有格式正确的分享页和对应公开 API 获得豁免。
3. 首版固定展示所有集群机器，不提供单机选择。机器名称本身属于公开字段，管理员启用前需确认名称适合公开。
4. 关闭后公开 API 返回 404；重置后旧链接返回 404。不存在、关闭和错误 Token 使用相同结果，避免泄露配置状态。
5. 匿名页只展示当前快照，不提供历史趋势、终端、文件、面板跳转或任何管理操作。
6. 匿名页默认使用紧凑列表，可切换为卡片排列；系统图标和国家旗帜复用 KPanel 现有组件，深浅模式沿用全局主题偏好。

## 公开字段契约

公开响应必须从 `cluster.HostList` 重新构造，禁止直接序列化 `cluster.Host` 或 `HostTelemetry`。

| 公开 | 不公开 |
| --- | --- |
| 展示名称、归一化状态 | Panel origin、IP、真实节点 ID |
| 管理员填写的到期日期、价格、每月流量重置日、月流量额度与统计方式（未设置则省略） | 主机资料的 resourceVersion、到期提醒开关、流量通知阈值 |
| 国家/地区/城市、ISP | Peer fingerprint、联邦协议与 scope |
| OS、架构、运行时间、负载 | Panel/Agent 版本、内核、遥测来源 |
| CPU 核数/使用率、累计或本周期收发流量 | CPU 型号、连接数 |
| 内存/磁盘容量与使用率 | resourceVersion、轮询计划、创建时间 |
| 实时上下行速率、采集时间 | 错误码、错误文本、失败次数、管理能力 |

公开机器 ID 使用分享 Token 对内部 Host ID 做 HMAC-SHA256，并截取为稳定页面键。Token 重置后，公开 ID 也随之变化。

内部状态归一化为 `online`、`degraded`、`offline`、`pending`，不向游客区分认证失败、TLS 错误或协议不兼容等内部原因。

有月度统计时，列表与卡片复用管理页的“月流量 · 百分比”及 80%/95% 颜色提示；无额度不显示百分比，
统计不可用时显示等待提示而不是 0%。未配置重置日时保持累计流量。百分比只用于展示，不改变计数与通知。
至少一台主机有有效价格和到期日期时显示顶部“剩余价值（估算）”，否则连同占位隐藏；已到期的有效配置仍显示 0。汇总根据已公开的价格、到期日期按币种分别汇总，口径与管理页一致，筛选不改变汇总。
点击可查看只读费用明细和未计入原因，不提供管理按钮或编辑指引。分享失效后撤下整个快照及已打开的费用明细。

## API 与持久化

- `GET /api/v1/cluster/share`：登录后读取设置。
- `PUT /api/v1/cluster/share`：同源、Session、CSRF 和审计保护下保存启用状态、标题与介绍；使用 `expectedResourceVersion` 防止覆盖并发修改。`hostOrder` 可选，缺省时保留匿名页现有顺序；管理界面仅在管理员勾选“按当前面板顺序排列公开页”时携带。
- `POST /api/v1/cluster/share/token`：同等保护下重置链接；审计只记录 `tokenRotated: true`，不记录 Token 或完整 URL。
- `GET /api/v1/public/cluster-share/{token}`：匿名读取严格白名单快照。
- `GET /share/{token}`：匿名 Vue 展示页。

配置存入现有 `panel-state.json` 的 `clusterShare` 字段，继续复用容量上限、`0600` 权限、临时文件同步、原子替换、Windows 恢复替换和写失败内存回滚。旧状态缺少该字段时按关闭处理，不迁移业务数据。
其中 `clusterShare.hostOrder` 只控制匿名页的展示顺序；登录后集群、文件、终端和历史监控使用独立的
`clusterHostOrder` 面板偏好，调整任一侧都不会隐式改写另一侧。

服务端公开快照缓存 10 秒并由互斥锁合并并发刷新；匿名来源每分钟最多 120 次请求，最多追踪 2048 个来源。HTTP 响应保持 `Cache-Control: no-store`，确保关闭或重置后下一次请求重新验证链接，不由浏览器/CDN继续展示旧内容。

## 竞品核对（2026-08-15）

- 哪吒官方文档将用户前台与管理前台分开，游客可访问部分只读 API；支持按服务器“对游客隐藏”，也可通过 `force_auth` 禁止游客访问。游客历史范围比登录用户更小。参考：[Servers](https://nezha.wiki/en_US/guide/servers.html)、[API Interface](https://nezha.wiki/en_US/guide/api)、[Settings](https://nezha.wiki/en_US/guide/settings.html)。
- Komari 官方文档说明主题可通过公开 API 获取站点设置和节点状态，并提供私有站点及临时访问链接语义。参考：[Public API](https://komari-document.pages.dev/en/dev/api)、[Theme Development](https://komari-document.pages.dev/en/dev/theme)。

KPanel 首版不把现有集群 API整体改成游客可读，也不默认公开首页；使用独立、可撤销的随机链接和新 DTO，减少现有管理面扩大匿名攻击面的风险。

## 回滚与恢复

主机可选资料保存在 `panel-state.json` 的 `clusterHostDetails`，本机、完整节点、轻量节点共用当前中心端资料。
列表和卡片只展示“到期日期、价格”，流量重置日仅在管理设置中显示；公开页从该资料逐字段加入白名单，不返回管理版本信息。
资料保存成功立即失效公开快照缓存。关闭或重置分享链接仍使用原有权限与失效规则。

- 功能回滚：移除分享路由、前端入口和 `clusterShare` 读写；JSON 中多余字段会被旧版本忽略，不影响登录、审计或集群凭据。
- 运维止血：管理员关闭分享；若页面不可用，可直接回到集群管理页重新保存或重置链接。
- 泄露处理：先重置链接使旧 Token 失效，再检查机器展示名称和公开介绍；无需重新配对主机。

## 开发验收记录（2026-08-15）

- Go：Store、Cluster、分享功能与相关认证/安全入口测试通过；`go vet ./...` 通过；Linux amd64/arm64 的 `paneld`、Agent、Node 和 CLI 交叉构建通过。
- Web：i18n 校验、TypeScript 类型检查、生产构建通过；Vitest 全量 98 个测试文件、720 个测试通过。
- 页面：本地匿名公开页完成桌面与 390×844 移动视口复核；移动端无横向溢出，浏览器控制台无 warning/error。
- L2 汇总脚本在 Windows 全量 Go 测试处未通过；失败集中在仓库既有的 Docker 临时路径、Windows 权限/数据目录和 Linux systemd 专用用例，分享功能相关测试均通过。尚未在两台真实 KPanel 或 Linux 运行态进行端到端联调。
- 本次没有提交、推送、发布或部署。

## 分享主题（`kpanel-share-theme@2`）

管理员可在分享设置中下载并应用社区/官方主题，改变匿名页的整体版式与风格。主题包是完整的独立网页，运行在
`sandbox="allow-scripts"` 的不透明来源 iframe 中，只通过 postMessage 接收字段白名单后的快照；样式、
版式与交互不依赖 KPanel 前端框架。协议 2 在保持隐私白名单不变的前提下额外提供原始数值（`ratio`、
字节、速率，未知为 `null`）、本地化词表与状态名；未声明 `protocol: 2` 的旧主题继续收到 schema 1 快照。
协议、字段与作者约束以 [`share-themes/README.md`](../share-themes/README.md) 为准。
