# 轻量文件分享设计与验收

## 任务记录

- 任务 scope / 角色 / AI：`file-sharing` / development / Codex。
- 风险等级：L3 发布阻断修复。原候选在 Linux overlayfs 暴露 ctime 粒度缺口，必须保持同元数据改写和替换后旧分享失效。
- worktree / branch / parent：`C:\GitHub\kejilion-panel-codex-file-sharing` / `feature/file-sharing` / `d9ad791a01bd3c804f0c1b7f6392597b56eb9bf4`。
- 授权边界：只允许在该功能分支形成并推送一个线性修复子提交；唯一发布任务负责重建候选、main、Tag、Release 和生产，旧候选 `dc20a34` 不得复用。
- 协议结论：复用现有 ETag 和 `filemanager` 安全边界；新增仅供 Panel 使用的 Agent 分享身份查询、分享内容流和内部前置条件 header，不改变联邦协议或外部脚本。

## 产品原则

文件分享是文件管理的一项轻量能力，不建设独立“图床”产品：

- 文件管理的单个普通文件菜单增加“分享”；目录、符号链接、特殊文件和批量选择不支持分享。
- 一个文件同一时间只有一个有效分享。创建新链接会原子替换旧链接。
- 每个分享同时提供分享页面和文件直链。图片直链可直接用于外部网站的 `<img src>`，无需图片专用记录、入口或存储目录。
- 默认有效期为 7 天，可选 30 天或永久。
- 首版不提供密码、访问统计、目录浏览、相册、自定义域名、上传 API 或批量分享。

这保持了 KPanel 的既有设计：真实文件系统仍是事实源，Panel 只保存少量授权元数据，所有宿主机文件读取继续由 Agent 和 `filemanager` 完成。

## 状态与链接生命周期

`panel-state.json` 的 `fileShares` 为有界列表，每条记录只保存：

- 随机管理 ID；
- 32 字节随机 bearer token 的 SHA-256 摘要，不保存原始 token；
- 规范化绝对路径、创建时的大小、`resourceVersion` 和分享专用 `shareVersion`；
- 创建时间和可选过期时间。

完整链接只在创建或重新生成成功的响应中返回一次。关闭对话框后，管理员仍可看到分享状态和到期时间；再次取得链接需要“重新生成链接”，旧链接随即失效。该取舍避免 bearer token 进入持久化状态、审计、错误和日志。

文件管理页提供一个轻量“分享管理”对话框，列出仍有效的分享路径和有效期并允许停止分享。因此文件即使已经移动或删除，永久记录也不会成为无法撤销、持续占用上限的孤儿状态；该入口只是文件管理内的管理动作，不构成独立图床或一级应用。

以下任一条件都会使公开文件内容访问统一返回 404：

- 分享已停止或过期；
- 链接已重新生成；
- 当前 `filemanager` 资源版本发生变化；
- 目标不再是同一个普通文件。

Panel 通过内部 `GET /v1/files/share-entry` 取得分享专用版本，并仅通过新的 `GET|HEAD /v1/files/share-content` 读取公开流，同时固定注入 `If-Match: "<resourceVersion>"` 和必填的 `X-KPanel-File-Share-Version: <shareVersion>`；客户端不能覆盖后者。普通 `/v1/files/content` 明确拒绝该分享 header。Agent 在 `filemanager` 安全打开文件并对打开的 fd 执行 `fstat` 后，先用 `If-Match` O(1) 拒绝普通元数据变化，再在写出任何文件响应头或正文前完成内容强校验。

`resourceVersion` 继续保持 KPanel 既有并发语义，不影响现有文件编辑、预览和下载。分享专用的 `shareVersion v2` 从安全打开并完成 `fstat` / `SameFile` 复核的同一个 fd 全量计算 SHA-256，同时绑定 `resourceVersion` 及 Linux device、inode、ctime；普通 `Stat`、目录列表、编辑和登录态下载不计算内容摘要。ctime 是时间戳而不是单调变更计数器，因此不能单独证明文件未变；内容摘要保证同长度改写并恢复 mtime 时旧分享仍失效，Linux 对象身份保证内容相同的 inode 替换也失效。Agent 在公开流发送响应头前重新完成全量校验并将 fd rewind。公开 JSON 元数据只做 O(1) 的普通资源版本检查，因此同元数据改写后页面可能仍显示原文件描述，但 `/f/{token}` 在发送任何文件字节前必定 404。校验完成后，已打开 fd 若被宿主机其他特权进程并发原地改写，操作系统仍可能返回变化或混合的字节；KPanel 自身的原子替换会让后续请求失效，但已打开旧 fd 的在途请求仍可能完成，属于文件系统无法完全消除的并发边界。

## API 契约

认证管理接口：

- `GET /api/v1/files/shares`：列出仍有效的分享（含管理 ID、内部路径和有效期，不返回 token 或链接），用于统一停止失效、移动或已删除文件的分享。
- `GET /api/v1/files/shares?path=...&resourceVersion=...`：读取当前文件的有效分享状态；不会返回旧 token。
- `POST /api/v1/files/shares`：同源、Session、CSRF 和审计保护下创建或重新生成链接。请求为 `path`、`expectedResourceVersion`、`expectedShareID`、`expiresIn`，其中 `expiresIn` 仅允许 `7d`、`30d`、`never`；`expectedShareID` 用于防止并发重新生成返回已失效链接。
- `DELETE /api/v1/files/shares/{id}`：同等保护下停止分享并取消该分享正在传输的文件流。

匿名接口：

- `GET /api/v1/public/file-shares/{token}`：仅返回 `name`、`mime`、`sizeBytes`、`expiresAt`、`directPath` 和 `downloadPath`。
- `GET /share/file/{token}`：公开文件页面。
- `GET|HEAD /f/{token}`：内联文件直链；唯一可选查询为 `download=1`，用于附件下载。

公开响应不包含内部路径、管理 ID、文件属主、权限、`resourceVersion`、错误原因或 Agent 响应正文。链接路径均为相对路径，不依据请求 `Host` 拼接 URL。

## 安全、缓存与资源边界

- 三类公开入口使用 32 字节 Base64URL token 的精确形状校验后才进入安全入口白名单；缺段、非法字符、额外路径段和多余查询均拒绝。
- 保留 Panel 的 Host 校验；匿名元数据和原始文件入口按受信代理规则解析客户端 IP，并共用固定窗口限流：每 IP 每分钟 300 次、最多 2,048 个有界键。
- 公开元数据查询仅在 token 摘要命中后占用 Panel 的 16 路 O(1) Agent `Stat` 查询闸门，最长 8 秒；查询完成后再次确认 token 仍有效，避免停止或轮换过程中返回陈旧 200。
- Store 最多保留 256 条有效分享；创建时清理过期记录。该上限同时约束最坏路径 JSON 转义后的状态体积，保持在当前 JSON → SQLite 评审阈值内。
- 单个可分享文件最大 512 MiB，与 KPanel 上传上限一致；创建和管理员状态复核的强指纹最长 30 秒。匿名文件流最多并发 2 路，为登录后的文件管理保留 Agent 下载容量。每次文件流打开以固定 64 KiB 缓冲全量读取内容，随后从同一 fd 发送，内存占用有界但强校验 I/O 成本为 O(文件大小)。
- 强校验按路径和大小计费：2 MiB 为一个单位，每条分享和全局每分钟均最多 512 单位（1 GiB），并继续叠加每 IP 请求限流；小于等于 2 MiB 的图片最多 512 次/分钟，512 MiB 文件最多两次。未取得文件流槽位或在二次撤销检查前已失效的请求不扣预算。HEAD、Range 和 304 同样先强校验，多段下载器不属于首版性能目标。单次文件流最长 2 小时、空闲超时 45 秒。
- 停止或重新生成分享会取消已注册的在途流；过期时间同时是流 context 的最晚截止时间。
- 原始直链返回 `Cache-Control: public, max-age=0, must-revalidate` 和现有 ETag，允许缓存复用但每次使用前必须验证撤销、过期和文件版本。
- `/f/{token}` 将全局 `Cross-Origin-Resource-Policy` 覆盖为 `cross-origin`，因此外站可以嵌入图片；不返回 CORS 授权，任意来源 JavaScript / Canvas 读取不属于首版契约。
- 只有 JPEG、PNG、GIF、WebP、AVIF 五类安全位图可以 `inline`；HTML、SVG、XML、JavaScript、CSS、PDF、`text/*` 及其他类型统一改为 `application/octet-stream` 并强制 `attachment`。
- Panel 不信任 Agent 的内容 CSP，会在所有原始分享响应上覆盖 `default-src 'none'; sandbox; base-uri 'none'; form-action 'none'; frame-ancestors 'none'`，并保留 `nosniff`，防止主动内容借 KPanel 同源执行。

## 交互与视觉约束

- 单文件分享入口仅位于文件的右键 / `···` 菜单；文件管理工具区另有一个克制的“分享管理”入口用于统一撤销。不增加侧栏、桌面应用、图片托管标签或批量分享按钮。
- 对话框复用 `ModalDialog size="small"`，包含加载、未分享、已分享、创建中、复制结果、确认停止和错误恢复状态。
- 公开页只包含 KPanel 品牌、主题切换、文件信息、安全位图预览和一个清晰下载按钮；SVG 和主动内容不内嵌。超过 12 MiB 的位图不自动拉取预览，避免页面打开即占用公开流。
- 正文和控件不小于 14px，辅助文字不小于 13px，仅紧凑标签与页脚可使用 12px；覆盖浅色、深色、390 / 768 / 1280px、100% / 125% / 200% 缩放、长文件名和长 URL。
- 新文案纳入 `zh-CN`、`en-US`、`zh-TW`；对话框关闭后恢复触发元素焦点，复制结果通过可见文本和 live region 提示。

## 兼容、回滚与恢复

- 旧状态文件可直接加载；未包含 `sizeBytes` 的未发布候选记录按零值读取并在内容匹配时 fail-closed，管理员仍可从分享管理中停止。不改变既有文件管理、下载票据或集群分享。新 Panel 若连接未升级、缺少分享内部端点的 Agent，分享管理操作和公开访问会安全返回不可用或 404，且绝不重试普通文件内容端点，因此不会退回弱校验或公开文件。
- 回滚到不含本功能的旧二进制后，公开路由不存在，链接立即不可访问。旧版本只会在未发生状态写入时暂时保留未知的 `fileShares` 字段；一旦账户、Session、审计或其他 Panel 状态被持久化，旧 Store 会重写 JSON 并永久丢弃该字段。
- 回滚前应先停止全部分享并备份 `panel-state.json`；如果要求旧链接永久不可恢复，可在 Panel 停止后从备份副本确认状态，再离线删除 `fileShares` 字段。不得在线直接编辑状态文件。
- 泄露处理：对相应文件执行“重新生成链接”或“停止分享”；无需移动或复制源文件。

## 验收状态

2026-08-22 在 Windows + WSL2 Linux 开发工作区完成以下修复验收：

- 原失败回归：WSL2/v9fs 上 `TestShareVersionDetectsSameMetadataRewriteAndReplacement` 连续 100 次通过；覆盖同长度原地改写并恢复 mtime，以及同内容、同元数据 inode 替换。
- Linux 后端：`go test ./...`、`go vet ./...` 全仓通过；Store、Filemanager、Agent、Panel 四个受影响包全量 `-race` 通过。强校验覆盖 stale `If-Match` 快速拒绝、内容 SHA-256、前后 `fstat`、两层闸门释放、512 MiB 上限、Range / HEAD / 304、限流原子计费和撤销竞态。
- 前端：i18n 检查通过（2,443 条短语、21 个延迟目录）、typecheck 通过、110 个 Vitest 文件共 870 项通过、生产构建通过；新增 512 MiB 边界、前置阻止、后端 413 竞态、历史分享仍可停止、创建/轮换响应中断后的状态恢复和完整英文回归。
- 浏览器：项目标准 draft/mock 预览下复核普通及 513 MiB 超限对话框；桌面布局和 390×844 移动端无横向溢出，提示、隐藏有效期、禁用创建状态清楚；临时 mock 差异已还原，预览均按 manifest 停止。
- 独立复核：后端安全/并发和前端交互均无 P0/P1/P2；公开强校验额外读盘被限制为每路径及全局各 1 GiB/分钟，最多两路并发。Agent 忙时已扣预算不退款属于保守、最长一分钟的可用性取舍，不影响 fail-closed 安全。

本机没有可用 Docker daemon，不能替代发布任务在固定 `kpanel-release-gate:go1.26.6-node24` Runner 与 arena-154 overlayfs 上的权威 L3、镜像和真实 Panel + Agent 验收。新提交交付后必须从头重建候选；旧候选及其结论不得复用。
