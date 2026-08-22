# 轻量文件分享设计与验收

## 任务记录

- 任务 scope / 角色 / AI：`file-sharing` / development / Codex。
- 风险等级：L2。新增匿名公开页面、公开元数据 API 和文件流入口，并扩展 Panel Store。
- worktree / branch / base：`C:\GitHub\kejilion-panel-codex-file-sharing` / `feature/file-sharing` / `origin/main@7a00d5314c355f799cd84e2409f647bac77cccd5`。
- 授权边界：允许在独立分支修改和验证；未授权提交、推送、合并、打标签、发布或部署。
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
- 规范化绝对路径、创建时的 `resourceVersion` 和分享专用 `shareVersion`；
- 创建时间和可选过期时间。

完整链接只在创建或重新生成成功的响应中返回一次。关闭对话框后，管理员仍可看到分享状态和到期时间；再次取得链接需要“重新生成链接”，旧链接随即失效。该取舍避免 bearer token 进入持久化状态、审计、错误和日志。

文件管理页提供一个轻量“分享管理”对话框，列出仍有效的分享路径和有效期并允许停止分享。因此文件即使已经移动或删除，永久记录也不会成为无法撤销、持续占用上限的孤儿状态；该入口只是文件管理内的管理动作，不构成独立图床或一级应用。

以下任一条件都会使公开访问统一返回 404：

- 分享已停止或过期；
- 链接已重新生成；
- 当前 `filemanager` 资源版本发生变化；
- 目标不再是同一个普通文件。

Panel 通过内部 `GET /v1/files/share-entry` 取得分享专用版本，并仅通过新的 `GET|HEAD /v1/files/share-content` 读取公开流，同时固定注入 `If-Match: "<resourceVersion>"` 和必填的 `X-KPanel-File-Share-Version: <shareVersion>`；客户端不能覆盖后者。普通 `/v1/files/content` 明确拒绝该分享 header。Agent 在 `filemanager` 安全打开文件并对打开的 fd 执行 `fstat` 后、写出任何文件响应头或正文前同时校验这两个前置条件。

`resourceVersion` 继续保持 KPanel 既有并发语义，不影响现有文件编辑、预览和下载。Linux 上的 `shareVersion` 额外绑定 device、inode 和 ctime，因此即便文件被同长度改写并恢复 mtime，或替换为大小、mtime、权限、属主均相同的另一文件，旧分享仍会失效。它防止公开请求打开前已经完成的替换；已打开 fd 在传输期间被宿主机其他进程原地改写时，操作系统仍可能返回变化或混合的字节，KPanel 自身的写入使用原子替换，不存在这一窗口。

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
- 公开元数据查询仅在 token 摘要命中后占用独立的 16 路 Agent 查询闸门，最长 8 秒；查询完成后再次确认 token 仍有效，避免停止或轮换过程中返回陈旧 200。
- Store 最多保留 256 条有效分享；创建时清理过期记录。该上限同时约束最坏路径 JSON 转义后的状态体积，保持在当前 JSON → SQLite 评审阈值内。
- 匿名文件流最多并发 2 路，为登录后的文件管理保留 Agent 下载容量；单次最长 2 小时、空闲超时 45 秒、复制缓冲区 64 KiB。
- 停止或重新生成分享会取消已注册的在途流；过期时间同时是流 context 的最晚截止时间。
- 原始直链返回 `Cache-Control: public, max-age=0, must-revalidate` 和现有 ETag，允许缓存复用但每次使用前必须验证撤销、过期和文件版本。
- `/f/{token}` 将全局 `Cross-Origin-Resource-Policy` 覆盖为 `cross-origin`，因此外站可以嵌入图片；不返回 CORS 授权，任意来源 JavaScript / Canvas 读取不属于首版契约。
- HTML、SVG 等主动内容继续由 Agent 降级为 `text/plain`；响应保留 `nosniff` 和文件内容 CSP。

## 交互与视觉约束

- 单文件分享入口仅位于文件的右键 / `···` 菜单；文件管理工具区另有一个克制的“分享管理”入口用于统一撤销。不增加侧栏、桌面应用、图片托管标签或批量分享按钮。
- 对话框复用 `ModalDialog size="small"`，包含加载、未分享、已分享、创建中、复制结果、确认停止和错误恢复状态。
- 公开页只包含 KPanel 品牌、主题切换、文件信息、安全位图预览和一个清晰下载按钮；SVG 和主动内容不内嵌。
- 正文和控件不小于 14px，辅助文字不小于 13px，仅紧凑标签与页脚可使用 12px；覆盖浅色、深色、390 / 768 / 1280px、100% / 125% / 200% 缩放、长文件名和长 URL。
- 新文案纳入 `zh-CN`、`en-US`、`zh-TW`；对话框关闭后恢复触发元素焦点，复制结果通过可见文本和 live region 提示。

## 兼容、回滚与恢复

- 新字段使用 `omitempty`，旧状态文件可直接加载；不改变既有文件管理、下载票据或集群分享。新 Panel 若连接未升级、缺少分享内部端点的 Agent，分享管理操作和公开访问会安全返回不可用或 404，且绝不重试普通文件内容端点，因此不会退回弱校验或公开文件。
- 回滚到不含本功能的旧二进制后，公开路由不存在，链接立即不可访问；旧版本会忽略 `fileShares` 字段。
- 若未来再次升级且要求旧链接永久不可恢复，回滚前应先停止分享，或在停止 Panel 后备份并从 `panel-state.json` 删除 `fileShares` 字段。不得在线直接编辑状态文件。
- 泄露处理：对相应文件执行“重新生成链接”或“停止分享”；无需移动或复制源文件。

## 验收状态

2026-08-22 在 Windows 开发工作区完成以下验收：

- 分享目标测试：`go test ./internal/store ./internal/filemanager ./internal/agent ./internal/panel -run 'TestFileShare|TestShareVersion|TestFileEndpointsListWrite' -count=1` 通过；覆盖 Store 恢复与 CAS、Linux 强身份算法、Agent 专用端点、Panel 创建/轮换/停止、真实公开流限流与释放、删除/轮换/过期/服务关闭取消、打开前二次撤销检查、Range/HEAD/缓存和安全入口。
- 受影响包回归：Store、Filemanager、Agent 全量测试通过；`go vet ./...` 通过。Panel 的分享目标测试通过。
- 前端：i18n 检查通过（2,442 条三语短语）、typecheck 通过、110 个 Vitest 文件共 860 项通过、生产构建通过；新增 Teleport 对话框的英文回归，避免运行时切换语言后仍显示中文。
- 预览脚本：模拟 API 语法检查和本地预览脚本 6 项测试通过；两个 draft/mock 预览均按 manifest 停止，无残留进程。
- 浏览器人工复核：经典模式和桌面模式 1280px、移动端 390×844、浅色/深色、简中/英文/繁中均通过；菜单键盘方向键、Escape 与焦点恢复通过；目录和批量栏没有分享入口；创建、一次性链接、重开不泄露旧 token、分享管理和无效链接恢复态符合设计，页面无横向溢出。
- 图片引用：公开页的 `<img src="/f/{token}">` 实际加载 2,560×1,440 WebP；移动端图片宽度自适应且最小辅助文字 13px。直链 `HEAD` 返回 `image/webp`、`Content-Disposition: inline`、`Cross-Origin-Resource-Policy: cross-origin` 和 revalidate 缓存策略；`?download=1` 改为 `attachment`。
- 仓库级 `go test ./...` 和 L2 权威脚本已执行，但当前 Windows 主机仍命中与本功能无关的既有平台失败：`internal/dockerx` 的 Windows 路径/权限 fixture、`internal/panel` 的静态资源/绝对路径/SPA fallback 5 项、`internal/systemmanage` 的 Linux/systemd-only 测试；其余包及本功能测试通过。

当前环境没有可执行的 Linux、WSL 或 Docker，因此 Linux 专属的同元数据替换测试只完成 `GOOS=linux GOARCH=amd64` 编译验证，未在 Linux 内实际运行，也未完成 Linux `-race`。浏览器验收使用项目标准 mock preview，证明 UI、交互和直链语义，不替代真实 Linux Panel + Agent 的发布前集成验收。
