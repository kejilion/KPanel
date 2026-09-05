# KPanel 轻量文件管理设计

- 状态：基础能力已随 `v0.30.0` 发布，根目录边界已随 `v0.34.0` 发布，回收站生命周期随 `v0.34.4` 发布；压缩/解压已随 `v0.34.7` 发布；远程下载已随 `v0.92.0` 发布，后台离线任务为当前候选实现
- 基线：`v0.29.0` / `2abef3d`
- 核验等级：L2（跨 Panel、Agent 与宿主机文件写入）

## 1. 业务定位

文件管理是 KPanel 对宿主机文件系统的日常文件工作台，入口位于左侧“体检”上方。
它服务于网站文件、应用数据、备份和普通用户目录，不替代 SSH，也不接受任意 Shell。

产品原则：

- 已登录管理员可以管理 `/` 下的普通文件，不以“是否由 KPanel 创建”限制操作。
- KPanel 自身运行目录、凭据和任务状态始终隐藏且不可读写。
- 默认使用专业列表视图；目录快捷入口、批量选择、右键菜单和快速预览保持一屏可用。
- 大文件传输采用流式代理，不把完整内容载入 Panel 或 Agent 内存。
- 写入后重新读取真实文件状态，不维护第二套文件事实。
- 显式下载是可靠兜底：单个普通文件使用短时 download ticket；单个目录或批量选择由用户明确触发，
  先封存精确选择和版本到短时 archive ticket，再下载受限 ZIP。Chromium 私有 `DownloadURL` 只作为
  尽力而为的操作系统拖出渐进增强。

## 2. 首版范围

### 文件操作

- 浏览目录、搜索当前目录、排序。
- 新建文件夹、上传、下载。
- 从公开 HTTP/HTTPS 地址流式下载单个文件到当前目录；可指定名称，同名自动追加序号且不覆盖。
- 重命名、复制、移动和移入回收站。
- 批量复制、移动、下载和移入回收站。
- 回收站查看、恢复、选择性彻底删除与清空；恢复不覆盖原路径中的同名项目。
- 批量压缩为 TAR.GZ、ZIP 或 TAR；默认推荐 TAR.GZ。
- 将 TAR.GZ、TGZ、ZIP 或 TAR 解压到新文件夹；不覆盖已有目录或文件。
- 查看并修改 POSIX 权限。
- 显示名称、类型、大小、权限、所有者、修改时间和资源版本。

### 查看与编辑

- 文本与代码：纯文本、Shell、Go、JavaScript/TypeScript、Vue、HTML/CSS、JSON、YAML、
  TOML、INI、Nginx、日志、Markdown。
- 图片：PNG、JPEG、GIF、WebP。
- 文档：PDF。
- 媒体：浏览器支持的音频和视频。
- 压缩包：支持从右键菜单解压，也可继续下载；其他二进制显示元数据并提供下载。
- 编辑器使用懒加载的轻量 textarea + 行号、Tab 缩进、快捷保存和冲突检测，不引入大型 IDE 依赖。

基础首版不包含跨主机传输、Git 客户端、终端或数据库文件解析；后续跨 KPanel 复制和公开 URL
远程下载均以固定协议独立加入。Git 客户端和数据库文件解析仍不在当前范围，也不得通过任意 Shell
临时补位。远程下载不支持 BT/magnet、批量 URL、自定义 Header/Cookie/代理、私网来源、自动解压、
断点续传或自动重试。后台任务允许关闭或刷新文件页面后继续，但不把完整 URL 持久化，因此 Panel
进程重启后不会自动续传或重试。

## 3. 信任边界与质量记录

```text
浏览器
  -> Panel Session + Origin + CSRF
  -> Panel 流式代理 / 公开 URL 受限 GET
  -> Unix Socket + Agent Token
  -> Agent 固定文件动作
  -> /（排除保护目录与内核虚拟写入口）
```

| 项目 | 设计 |
| --- | --- |
| 流量路径 | 浏览器仅访问 Panel；Panel 通过现有 Unix Socket 调用 Agent |
| 不可信输入 | 虚拟路径、文件名、批量来源、目标目录、权限、上传内容、文本内容、Range、远程 URL、重定向和响应头 |
| 权限与可写范围 | `os.Root` 固定根 `/`；保护 KPanel 凭据/状态目录及回收站，`/proc`、`/sys`、`/dev` 只读；回收站固定在 Agent 实际状态目录内 |
| systemd 沙箱 | 文件管理根目录保持可写并对 Panel 状态单独设置 `ReadOnlyPaths`；写入仅授予 root Agent，Panel 容器不挂载宿主机根目录 |
| 最坏输入/输出 | JSON 64 KiB；远程 URL 4 KiB、响应头 64 KiB；文本编辑 2 MiB；单次上传或远程下载 512 MiB；列表单页 500 项、最多扫描 20,000 项；回收站最多 10,000 项、单次显示 500 项；批量 100 项 |
| 最大并发 | 上传 2、远程下载 2、下载 4，超限立即返回 `429`；远程下载最终还受 Agent 上传总闸门约束；复制、移动、压缩、解压及回收站操作串行执行；复制和归档预算按整批累计 10,000 项/10 GiB |
| 超时、取消与重试 | 普通 API 保留短超时；文件流使用 45 秒上游正文空闲超时和 2 小时硬上限；压缩/解压及远程下载可由页面主动取消；远程下载取消立即停止读取并清理尚未提交的临时产物，若与原子提交重叠则刷新目录确认；写操作和远程 GET 不自动重试 |
| 真实状态 | `Lstat`、打开后 `Stat`、目录内容和文件元数据 |
| 失败与回滚 | 上传/编辑使用同目录临时文件、`fsync` 和原子替换，并保留权限、所有者与 Linux 扩展属性；复制失败清理目标临时产物；回收站元数据原子写入，恢复与彻底删除逐项报告结果 |
| 性能预算 | 文件页面懒加载；不增加首屏包；流式传输使用固定 64 KiB 缓冲；目录搜索在 Agent 有界执行并按 500 项分页 |
| 网络入侵风险 | 路径穿越、符号链接逃逸、CSRF、超大请求、恶意 MIME、SSRF、DNS rebinding、重定向绕过、签名 URL 泄漏、资源耗尽 |

### 3.1 Windows Chromium 下载兼容边界

- Windows 实机先复现了 KPanel HTTPS 目标 `FILE_BLOCKED` / 0 B；后续同目标严格 A/B 中，来自
  `127.0.0.1` 的 `text/plain` 与 `application/octet-stream` 曾同时成功，随后同一发起 origin 的全部
  载荷又失败。新标签页恢复原文件名仍失败，`no-referrer` 也失败；仅把发起 origin 改为 `localhost`
  后四种载荷又全部成功。故这不是固定 MIME、文件名、载荷顺序或目标内容问题，而是下游安全分类会随
  initiator、tab、referrer、navigation 等页面环境变化；现有证据不能唯一归因具体安全组件。
- Chromium 源码确认拖出下载保持 `content_initiated=false`，不进入 automatic download limiter；该
  limiter 的拒绝也不会生成已观察到的 History `FILE_BLOCKED` 项。Google Transparency 当前公开状态未
  报告 `kpanel.kejilion.eu.org` 威胁，但该站点状态不等于某次 download protection verdict。
- 页面只能得知 HTML 拖拽生命周期，无法取得 Chromium 下载器或 Windows Explorer 的最终落盘结果；
  `dragend`、`dropEffect` 和是否发出网络请求都不能作为文件已保存的成功回调，也不能据此自动重试。
- 候选保留 `DownloadURL` 作为渐进增强：受支持 Chromium 场景可尝试把文件拖到 Explorer，但产品不
  承诺落盘，也不得写成已修复 `kpanel.kejilion.eu.org` 的客户端拦截。失败时始终引导用户使用显式入口。
- 可靠兜底是用户显式点击下载：单个普通文件先由已认证请求创建短时 download ticket；单个目录或
  批量选择由同源、Session、CSRF 保护的 POST 创建 archive ticket，服务端封存精确来源、版本和 ZIP 名，
  浏览器只取得不含路径和选择集的 5 分钟短 URL。页面只报告 ticket/URL 构造和下载启动阶段错误，不把
  浏览器或操作系统最终落盘状态误报为成功。
- ticket 消费时 Panel 通过内部 `POST /v1/files/archive?name=...` 把选择 JSON 放在 body 中；旧 GET query
  仅为同步 `DownloadURL` 兼容保留，避免显式批量下载再次撞上 Agent 的 16 KiB 请求头上限。
- 页面内、跨窗口和跨 KPanel 的拖拽载荷及后端重新鉴权不依赖操作系统原生拖出；浏览器拒绝
  `DownloadURL` 时，这些路径继续可用。
- 不得把伪装扩展名或 MIME、删除 CSP/`nosniff` 等安全响应头、延长 URL 凭据或切换下载域名描述为
  修复。`127.0.0.1` / `localhost` 对照改变的是发起 origin，不证明更换下载域有效；组织管理设备继续
  遵循既有安全策略，KPanel 不提供绕过或弱化方案。
- Windows 现场证据、KodBox 对照和复现命令统一记录在
  [`windows-file-download-compatibility.md`](windows-file-download-compatibility.md)。

### 3.2 归档安全与兼容契约

- Agent 直接使用 Go 标准库流式处理，不调用宿主机 Shell、`tar`、`zip` 或第三方常驻服务。
- 压缩和解压均使用目标目录内的隐藏暂存路径、`fsync` 和不覆盖原子改名；失败或取消立即清理。
- 单次最多选择 100 项，累计最多 10,000 个条目、10 GiB 原始/解压数据；固定 64 KiB 缓冲，
  不把完整压缩包或文件载入内存。
- 单个目录或批量选择的显式 ZIP 下载复用相同遍历、版本、并发和容量预算；写出首字节前完整预扫描
  symlink、特殊文件、条目数和字节预算，并再次复核顶层版本。不同目录的同名顶层来源按 Windows
  大小写不敏感规则稳定追加 ` (2)` 等后缀；重复同一路径仍拒绝。
- 请求取消或预扫描后的并发源变化会停止流；失败归档不写入服务器文件系统，也不生成完整 ZIP 中央目录。
- `walkArchive` 在递归进入每个子项前重新过滤 protected descendant 与 `.kpanel-*` 内部临时组件；
  定向测试确认允许祖先目录中的正常文件保留，保护后代及内部文件/目录不会进入 ZIP。
- 解压拒绝绝对路径、`..`、反斜杠路径、重复条目、符号链接、硬链接、设备文件和特殊文件，
  不能通过压缩包逃逸目标目录或覆盖现有内容。
- ZIP 在构造中央目录对象前先读取尾部目录记录并拒绝超过条目上限的归档，避免恶意中央目录先行
  消耗大量内存；实际解压字节仍按流量预算再次检查。
- 处理前后复核源文件资源版本及 inode；保留普通文件和显式目录的权限与修改时间。
- 单目录压缩时归档其内容，解压到默认同名文件夹后不会产生 `目录/目录` 双层嵌套；多选时保留
  每个来源的顶层名称。

### 3.3 竞品复核记录

2026-08-01 以公开官方资料复核：

- [1Panel 文件管理文档](https://1panel.cn/docs/v1/user_manual/hosts/file/)提供多种压缩格式并推荐
  TAR.GZ；其[版本记录](https://1panel.cn/docs/v2/changelog/)体现了停止压缩/解压任务、格式兼容和
  修改时间保留等持续修复。
- [宝塔批量解压接口](https://docs.bt.cn/api/files/mutil_unzip)支持批量归档、指定目标目录以及
  逐项成功/失败结果。

2026-08-22 再次复核远程下载交互：

- [1Panel v2 文件管理文档](https://1panel.cn/docs/v2/user_manual/hosts/file/)把“在线下载”作为文件
  工具栏中的直接入口，目标是把其他服务器文件保存到当前服务器；这验证了入口应可见而不是藏在
  更多菜单中。
- [1Panel v2 版本记录](https://1panel.cn/docs/v2/changelog/)持续修复远程下载取消、URL 校验、错误
  提示和响应文件名解析，并在后续版本加入代理配置。KPanel 首个版本吸收取消、逐跳 URL 校验和安全
  文件名解析；代理会改变 SSRF 信任边界，因此明确不纳入当前版本。

KPanel 方案覆盖上述核心场景，并额外固定无覆盖原子落盘、资源版本冲突、归档路径逃逸、链接与
特殊文件拒绝、累计解压预算，以及取消时清理未提交暂存文件并对提交窗口做结果确认。压缩与解压
仍使用现有请求生命周期；远程下载只增加 Panel 进程内 worker 和一个有界任务索引，不引入消息队列、
通用调度平台或 Agent 公网抓取能力。

### 3.4 远程下载安全与生命周期契约

```text
Browser POST JSON（URL 只在 body）
  -> 非 root Panel 专用下载客户端
     -> 公开 HTTP/HTTPS 固定 GET
     -> DNS 全部地址 fail-closed 校验并直拨 IP
     -> 最多 5 次重定向，每跳复核，拒绝 HTTPS -> HTTP
  -> Agent /v1/files/upload（只看到字节流、目录和名称）
  -> filemanager.Upload 同目录暂存、fsync、no-replace 原子发布
```

- URL 最多 4096 字节，只接受绝对 HTTP/HTTPS；拒绝 userinfo、fragment、控制字符、反斜杠、
  IPv6 zone、模糊数字主机、单标签主机和非法端口。
- DNS 返回的每个地址都必须是公开 global-unicast；任一结果为 loopback、RFC1918/ULA、link-local、
  multicast、unspecified、CGNAT、文档/基准保留段、IPv4-mapped、NAT64、6to4 或 Teredo 时整次拒绝。
  实际连接只拨已校验 IP，保留原 Host 与 TLS SNI，避免第二次系统解析造成 rebinding。
- 客户端不读取环境代理，不接受自定义方法、Header、Cookie、Authorization、Host、请求体、私网
  allowlist 或跳过 TLS。固定 `Accept-Encoding: identity`，拒绝压缩响应；跨域重定向清除 Referer，
  避免源 URL 的查询签名泄漏。
- 只接受完整的 `200 OK`，拒绝 `Content-Range`、其他 `2xx` 和压缩响应；上游错误正文不落盘、
  不回显。Content-Length 已知超限时提前拒绝，未知长度仍由 Panel 与 Agent 读取到 `512 MiB + 1`
  后中止。DNS 与最多 8 个候选 IP 共用一次连接阶段时限，TLS、响应头、正文空闲和总时长也均有界。
- 保存名称优先级为管理员明确名称、有效 Content-Disposition、`download`；不从 URL 路径推断名称，
  避免路径型签名令牌进入文件名、状态或审计。每个候选只可作为 255 字节以内 basename，不得决定
  目录。已有同名文件时在下载前生成 ` (1)` 等后缀，
  最终提交仍使用 no-replace 处理竞态，绝不静默覆盖。
- 默认 UI 创建后台任务，状态为 `queued -> connecting -> transferring -> confirming -> complete|error`；
  用户停止后为 `cancelled`。已知总量显示真实进度，未知总量只显示已接收字节，不伪造百分比。
  `confirming` 表示 Agent 已结束原子上传、Panel 正在核对返回结果，不把它误写成“正在提交”。关闭、
  刷新或离开文件页面只停止浏览器轮询，Panel worker 继续；旧的请求作用域 NDJSON POST 保留兼容。
- Panel 任务索引只持久化脱敏 origin、目标目录、安全名称、字节数、状态、结果与时间。完整 URL 只在
  worker 内存中存活，不进入索引，所以 Panel 重启会把遗留 active 状态保守标记为 `interrupted`，不自动
  重放 GET。用户必须先刷新目标目录确认原子提交结果，再重新输入 URL；这避免在提交后、状态落盘前崩溃
  的窗口重复保存文件。
- 下载并发仍与同步兼容接口共享 2 个槽位；后台 running + queued 最多 10 个，任务历史最多 100 条，
  terminal 状态保留 7 天并允许用户清理。索引使用私有目录、`0600` 文件、大小上限和原子替换；损坏时
  后台任务 fail-closed 为不可用，旧同步接口不依赖该索引。
- 停止会取消上游读取并清理尚未提交的隐藏暂存文件；若停止或进程退出恰逢原子提交窗口，界面必须
  提示刷新真实目录确认。目录不会出现半文件，但任务状态不能替代目录事实。
- 审计仅记录 `scheme://host[:port]`、目标目录、安全文件名、字节数和结果码；完整 URL、路径、查询串、
  重定向链、响应正文及底层 `url.Error` 不进入审计、状态、Toast 或服务端错误。

## 4. 路径和隔离

API 使用以 `/` 开头的虚拟路径，与宿主机绝对路径一一对应；页面默认定位 `/home`。

每次操作执行：

1. 拒绝 NUL、反斜杠、过长路径、空组件、`.` 和 `..`。
2. 所有读取、写入、重命名、复制和回收操作都通过 Go `os.Root` 根句柄执行，
   即使路径组件在检查后被并发替换，也不能通过符号链接跳转。
3. 对已有路径的每级组件执行 `Lstat`，拒绝跟随符号链接。
4. 检查最终路径不位于保护目录；会移动、改名或改变权限的操作同时拒绝保护目录的祖先，
   避免通过父目录移动、改权或删除间接影响 KPanel 凭据和状态目录。
5. 打开后再次 `Stat`，避免把链接替换竞态解释为普通文件；不覆盖式重命名在 Linux
   使用 `renameat2(RENAME_NOREPLACE)` 原子完成。

符号链接可以作为目录条目显示，但不可进入、预览、下载、编辑、复制或移动到根外。

## 5. API

除远程 URL 抓取只存在于无特权 Panel 外，Agent 与 Panel 使用相同资源路径，Panel 增加 `/api` 前缀：

```text
GET  /v1/files?path=/docker&limit=200&offset=0&search=log
GET  /v1/files/trash
GET  /v1/files/content?path=/docker/app/file&disposition=inline
GET  /v1/files/content?path=/docker/app/file&disposition=inline&mode=text
PUT  /v1/files/content?path=/docker/app/file
POST /v1/files/upload?path=/docker/app&name=file
POST /v1/files/actions
POST /api/v1/files/remote-downloads
GET  /api/v1/files/remote-downloads
GET  /api/v1/files/remote-downloads/{id}
POST /api/v1/files/remote-downloads/{id}/cancel
DELETE /api/v1/files/remote-downloads/{id}
```

远程下载请求固定为 JSON：

```json
{
  "url": "https://downloads.example.com/archive.tar.zst?signature=...",
  "targetDirectory": "/home/downloads",
  "name": "archive.tar.zst",
  "background": true
}
```

`name` 可省略，接口不接受 `overwrite`。`background: true` 成功返回 `202` 和脱敏任务；省略该字段时
保留流式 `application/x-ndjson` 兼容响应。URL 不进入 path、query、Agent 请求、任务索引或 Jobs API。
创建、停止和清理属于写操作，必须通过 Origin、Session、CSRF 与审计；读取任务要求 Session。预流式
校验失败使用 RFC 7807，连接后的业务失败使用稳定 `code`，前端不得显示底层含 URL 的错误字符串。

`POST /files/actions` 仅接受固定动作：

```text
mkdir
rename
copy
move
compress
extract
trash
chmod
trash_restore
trash_delete
trash_empty
```

批量操作最多 100 个来源。编辑、回收、恢复和彻底删除请求携带资源版本；资源变化返回 `409`。
复制、跨文件系统移动和回收站操作共享整批累计预算，不能通过一次请求携带多个来源绕过容量限制。
批量操作逐项返回成功与失败结果；部分成功使用 `207 Multi-Status`，页面始终重新读取目录，
避免把已完成的文件变更误报为整体失败。
编辑器读取必须显式使用 `mode=text`，Agent 会复核当前大小、UTF-8 编码和 NUL 字节；
图片、PDF 和音视频仍使用 Range 流式读取。

目录列表单页稳定排序，前端合并分页后重新排序；单页最多 500 项，`offset` 用于加载后续页，`search` 在 Agent
侧最多扫描 20,000 个可见条目。达到扫描边界时 API 返回 `scanTruncated`，前端明确提示，
不把未扫描内容误报为“没有匹配项”。

常驻 Agent 暂不直接设置 `MemoryMax` 或 `CPUQuota`：应用、建站和诊断任务可能由 Agent
派生到独立 systemd 单元，给常驻服务增加硬配额会改变既有任务行为。当前先在文件 API
入口实施体积、并发、目录扫描和累计复制预算；若后续增加常驻服务资源配额，必须先完成
子任务 cgroup 完全隔离和多发行版实机验收。

## 6. 前端交互

- 左侧导航：文件位于 Docker 与体检之间。
- 默认列表：目录优先，列出大小、权限、所有者和修改时间。
- 双击目录进入，双击文件打开查看器。
- 成功进入目录后才把 Agent 确认的规范路径写入浏览器历史；前进、后退和刷新按 URL 中的
  `path` 恢复目录，失败或被后续导航取消的读取不新增历史记录，也不能覆盖较新的目录状态。
- 右键菜单与顶部/批量工具栏调用同一动作定义，避免能力不一致。
- 顶部命令栏直接显示“远程下载”。弹窗只包含下载地址、可选保存名称和当前目录快照；提交后立即
  清空 URL 输入并关闭弹窗。轻量任务列表显示脱敏来源、安全名称、目标目录、阶段、真实字节进度，
  active 任务可停止，terminal 任务可清理。页面进入时重新读取任务，存在 active 状态时才轮询；页面
  卸载只停止轮询，不取消服务端任务。完成当前可见目录的任务后刷新目录。
- 界面明确说明关闭页面后继续、Panel 重启时任务会标记中断且不会自动重试、完整 URL 不保存。停止、
  中断或错误与原子提交重叠时结果可能已保存，需要先刷新目标目录确认。
- 批量工具栏提供“压缩”；受支持的压缩包右键提供“解压到文件夹”。格式默认 TAR.GZ，用户可选
  ZIP 或 TAR；执行中明确显示运行状态并可停止，不把关闭页面误报为成功。
- 代码查看器使用等宽字体、行号和语法类型提示；编辑保存时执行资源版本冲突检查。
- 图片、PDF、音视频通过同源受控 URL 预览；视频只有在浏览器实际解码出非零尺寸画面后才标记为可播放，
  避免把“音轨可播、视频轨不兼容”误报为成功，并提供原文件下载及 H.264 + AAC MP4 转换提示；
  HTML、SVG 等主动内容只按纯文本显示或下载。
- 单个普通文件可创建轻量公开分享，同时获得分享页和文件直链；图片直链可供外站嵌入，但不建设独立图床入口。
  分享授权、公开字段、资源限制和回滚边界见 [`file-sharing.md`](file-sharing.md)。
- 普通文件列表中的删除固定进入 KPanel 回收站；彻底删除和清空只在回收站管理界面中提供并要求二次确认。
- 回收站显示原路径和删除时间，可批量恢复、彻底删除或清空；旧版无元数据项目仅允许彻底删除。

## 7. 验收

### 多窗口主机上下文

- 文件请求只读取显式 `hostId`，省略或空值固定表示当前 Panel 本机；不得以模块全局选择状态路由请求。
- 每个文件窗口持有自己的主机上下文，并在窗口路由保存 `hostId + path`。本机桌面快捷方式不能复用同路径的远端窗口。
- 上传、目录递归上传和跨主机批量复制在开始时捕获目标主机与路径；其他窗口切换或关闭不得改变后续请求目标。
- 剪贴板与目录变更通知绑定主机，只有同主机窗口共享复制/移动与路径重映射。同页面跨主机拖拽复用现有跨主机复制协议，不能按本机路径执行移动。
- 主机从清单消失、离线或撤权时保留原目标，由现有 API 返回失败；不回落本机、不重放写操作。旧主机的延迟预览响应不能更新新主机窗口。
- 自动组合回归使用真实 `api.ts` 与两个 `FilesView` 实例，mock 仅覆盖网络/上传传输。真实双节点、断线及撤权仍由隔离环境验收，不能由该测试推定通过。

- 单元测试：路径穿越、保护目录及其祖先、归档递归保护后代/内部临时组件、符号链接、慢速传输、
  上传/下载并发、目录分页与服务端搜索、超限上传/编辑、资源冲突、目标不覆盖、整批累计复制预算，以及三种归档格式、
  解压穿越/链接/特殊文件、归档膨胀、取消清理和修改时间保留。
- 远程下载网络测试：URL 语法、公开 IPv4/IPv6、全部受限地址、混合 DNS、固定 IP 拨号、重定向
  上限/私网跳转/HTTPS 降级、Referer 清除、环境代理无效、TLS、响应编码、非 2xx、已知与未知
  长度超限、正文空闲与取消；完整 URL 和上游正文不得出现在响应或审计中。
- Panel 测试：未登录、Origin、CSRF、未知路由、同步流式兼容、后台请求脱离、列表/读取、停止/清理、
  Panel 关闭、索引损坏与 Agent 不可用；持久化状态、API 和审计均不得泄漏 URL 路径或查询签名。
- 前端测试：导航顺序、目录前进/后退/刷新、失败与并发导航、列表、批量选择、右键菜单、
  查看器分类和保存冲突；远程下载目录快照、已知/未知进度、关页继续、重新进入恢复展示、停止、清理、
  中断说明、错误、成功刷新和 URL 不持久化。
- 构建：Go 全量测试、前端测试与生产构建、生态规则检查、Linux Agent 构建。
- Linux 真机：重启 Agent 后必须通过 `/proc/<MainPID>/mountinfo` 或 `nsenter` 核对进程的实际
  根挂载为 `rw`，不能只读取 `systemctl show`；随后通过 Agent Unix Socket 文件 API 分别在
  `/etc` 和 `/var/local` 创建并清理唯一测试目录，同时确认 Panel 状态目录的 API 返回保护错误且
  进程挂载仍为 `ro`，最后比对 Panel 健康状态和全部业务容器 ID/状态基线。

## 8. 代码编辑器性能记录

代码高亮使用 CodeMirror 6，并按“编辑器组件 + 当前文件语言”动态加载，不进入文件列表首屏。
超过 1 MiB 的文本不加载语法解析器，仍可使用带行号的纯文本编辑。

| 项目 | 原文本框 | CodeMirror 方案 |
| --- | ---: | ---: |
| 文件管理路由 JS + CSS（raw） | 35.3 KiB | 35.6 KiB |
| 首次打开 JavaScript 文件所需编辑器资源（gzip） | 0 | 约 156 KiB，按需加载 |
| 618.9 KiB / 6001 行文件暖启动平均耗时 | 932 ms | 636 ms |
| 同一文件末尾输入一行 | 39 ms | 42 ms |

按需编辑器资源高于常规单路由 120 KiB 预算，但不影响文件列表首屏；相比继续扩展自制高亮器，
CodeMirror 提供成熟的增量解析、选择模型和销毁生命周期，降低卡顿及 HTML 注入风险。该功能可独立
回滚到 `5f938dc`，不会改变 Agent 文件 API 或磁盘格式。
- 人工验收：浅色/深色、桌面/窄屏、上传下载、文本编辑、图片/PDF/媒体预览和失败提示。

## 9. 发布和回滚

- 功能提交保持独立，不修改 `kejilion.sh`。
- 回收站接口属于兼容新增，按补丁版本发布；旧版本已进入回收目录的文件仍可识别和彻底删除。
- 发布前执行 L3 门槛；未经明确授权不推送、不构建生产镜像、不部署。
- 回滚恢复上一稳定镜像和 Agent；用户文件不迁移、不删除。
