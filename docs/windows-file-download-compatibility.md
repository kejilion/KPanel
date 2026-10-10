# Windows Chromium 文件下载与拖出兼容性验证

> 最后核验：2026-08-20（Asia/Shanghai）
> 性质：可复现的现场证据、竞品对照和回归清单；不是历史 release acceptance 的替代品。

2026-10-10 实现调整：显式下载恢复为浏览器携带登录 Cookie 的直接请求，单文件使用
`/api/v1/files/content`，目录和多选使用 `/api/v1/files/archive`；不再签发临时凭证。
ZIP 的选择和资源版本恢复使用 URL 参数，受既有请求行长度限制；不共享浏览器 Cookie 的
外部下载器不再支持。下文保留 2026-08-20 的历史现场证据与当时候选方案，不作为恢复后实现的兼容保证。

## 1. 目标、基线与边界

本记录回答三个问题：

1. KPanel 文件拖到 Windows Explorer 后出现 `FILE_BLOCKED` 的现场事实是什么；
2. 同机可成功的 KodBox 使用了什么实现，哪些差异已确认、哪些仍不能推断；
3. KPanel 如何把 `DownloadURL` 保留为渐进增强，同时为被客户端拦截的环境提供可靠显式下载兜底。

核验基线：

| 项目 | 已验证值 |
| --- | --- |
| Windows | Windows 11 专业版，10.0.26200，64 位 |
| Chrome | 151.0.7922.169 |
| KPanel 代码基线 | `91583a311da9e31346c5e9ca4f628c35ce98494e`，`v0.88.1-1-g91583a3`；候选分支已 fast-forward 到核验时的 `origin/main` |
| KPanel 候选分支 | `fix/windows-drag-download`，未提交、未发布 |
| KPanel 实服 | `https://kpanel.kejilion.eu.org/files?path=/home` |
| KodBox 官方源码 | `1.69.01`，`83ac1fd643b6440403681b0dd24d59005697108e` |

本记录区分以下概念：

- **显式下载**：用户点击“下载”或“下载 ZIP”，页面启动常规浏览器下载；
- **操作系统拖出**：网页写入 Chromium 私有 `DownloadURL`，浏览器尝试向 Explorer 提供 promised file；
- **KPanel 内部拖拽**：页面内移动、跨窗口和跨 KPanel 自定义载荷，不依赖 `DownloadURL`；
- **最终落盘**：文件通过浏览器和 Windows 安全处理后真实存在，网页没有可靠回调可确认该结果。

## 2. Windows 实机 A/B 结果

Chrome History 中失败任务共同表现为：

- `state=4`（interrupted）；
- `danger_type=4`（maybe dangerous content）；
- `interrupt_reason=11`（`DOWNLOAD_INTERRUPT_REASON_FILE_BLOCKED`）；
- `received_bytes=0`；
- Windows 目标位于 Chrome 的 `chrome_drag...` 临时路径，未形成可用 Explorer 文件。

现场文案“贵组织屏蔽了此文件，因为它不符合安全政策”只证明 Chromium 最终返回
`FILE_BLOCKED`，不能单独证明存在企业 DLP。Safe Browsing、Windows Attachment Services、浏览器扩展、
防病毒/EDR 或本地/云策略都可能归并到该结果。

### 2.1 结果矩阵

#### 2.1.1 原始故障与同机对照

| 编号 | 发起页 / `DownloadURL` 目标 | 描述文件名 | 结果 | 能排除或证明的因素 |
| --- | --- | --- | --- | --- |
| K1 | KPanel 页面 → `/api/v1/files/content?...` | `nginx.conf` | `FILE_BLOCKED`，0 B | 复现原问题 |
| K2 | KPanel 页面 → `/api/v1/files/archive?...` | `.zip` | `FILE_BLOCKED`，0 B | 问题不只发生于单文件 |
| K3 | 本地受控页 → KPanel 短时 ticket URL | `.conf` | `FILE_BLOCKED`，0 B | 免 Cookie ticket 不能单独修复 |
| K4 | K3 的同一 ticket，仅更换描述名 | `.txt` / `.zip` | 均 `FILE_BLOCKED`，0 B | 扩展名不是共同根因 |
| K5 | 本地受控页 → KPanel 公共 `/manifest.webmanifest` | `public-probe.txt` | `FILE_BLOCKED`，0 B | 不依赖 Session、Agent 文件 API、ticket 或 ZIP |
| K6 | KPanel ticket，由用户正常点击 | `nginx.conf` | 成功，内容可用 | 服务端文件读取和普通下载链路可用 |
| C1 | 同机本地无鉴权测试页 → 本地资源 | `.zip` / `.txt` | Explorer 拖出成功 | Chrome/Explorer 没有全局禁用 `DownloadURL` |
| C2 | 同机 `https://demo.kodcloud.com` → 同源 KodBox ZIP | `.zip` | Explorer 拖出成功 | 同机同浏览器存在成功竞品对照 |

说明：K3～K5 的本地受控页只更换目标 URL、描述名或资源类型；票据明文和会话信息没有写入本记录。
Chrome History 的 `mime_type` / `original_mime_type` 记录响应或嗅探结果，不能反推出 `DownloadURL`
描述符前缀。K1～K6 没有记录实服构建 ID，不能据此证明实服运行的就是当前本地候选。C2 也没有保留
受测 KodBox 实例的版本或 bundle hash，所以它只作为同机成功对照；第 3 节对 `1.69.01` 的源码核验是
独立证据，不能反向证明 C2 实例使用该版本。

#### 2.1.2 同目标受控时序复测

后续 harness 对同一个远端目标做了更严格的复测。A/B 使用同一个 `127.0.0.1` 发起源、同一目标、同一
文件名、`effectAllowed=copy`，并都先写 `DownloadURL`；唯一变量是描述 MIME：

| 阶段 | 发起环境与载荷 | 结果 | 能确认的边界 |
| --- | --- | --- | --- |
| A | `127.0.0.1`；`text/plain` | Explorer 拖出成功 | 该时刻此 MIME 可用 |
| B | 与 A 相同，仅改为 `application/octet-stream` | Explorer 拖出成功 | 两种 MIME 在同一环境均可用 |
| C～F，第二轮 | 同一 `127.0.0.1` 起源；四种写入顺序/载荷组合 | 四项均 `FILE_BLOCKED` | 结果随后在相同目标上翻转；第二轮使用了不同文件名 |
| C～F，全新标签页 | 新建 `127.0.0.1` 标签页，恢复首轮成功的原文件名和 MIME | 四项均 `FILE_BLOCKED` | 排除第二轮文件名差异和旧标签页本身是充分原因 |
| G | `127.0.0.1` 发起页，`no-referrer` | `FILE_BLOCKED` | 删除 referrer 不能单独修复 |
| C～F，换发起源 | 同一回环服务改由 `localhost` 发起，使用首轮成功文件名 | 四项均成功 | 下游结果会随 initiator / 页面环境变化；目标载荷不是固定结论 |

C～F 分别为：C=`copy` + `DownloadURL` first 的最小控制；D=`all` + `DownloadURL` first；
E=`all` + `text/plain` first + `DownloadURL` last；F=`all` + KPanel 三种 custom type first +
`DownloadURL` last。四项在 `127.0.0.1` 全部失败、换成 `localhost` 后又全部成功，因此
`effectAllowed`、写入顺序、`text/plain` 或 KPanel custom type 都不能单独解释结果。`127.0.0.1` 与
`localhost` 是两个不同 origin；这组时序只证明分类与 initiator、tab、referrer、navigation 等页面环境
相关，不证明 hostname 是唯一因果变量。

### 2.2 已验证结论

- KPanel 的共同失败不能由 Cookie、ticket、`.conf`、`.zip`、`Content-Disposition`、归档流或 KPanel
  文件 API 中任意单项解释；公共静态资源对照已经绕开这些链路。严格 A/B 还确认 `text/plain` 与
  `application/octet-stream` 能在同一状态下同时成功，随后又能在另一页面状态下同时失败，所以 MIME、
  文件名、`effectAllowed`、数据类型写入顺序及 KPanel custom type 都不是固定根因。
- 同一机器的本地测试页和 KodBox 能成功，故不符合 `DownloadRestrictions=3` 一类“全部拖出下载禁用”的表现。
- 最新时序更符合 Chromium/Windows 下游安全分类会结合 initiator、tab、referrer、navigation 或其派生
  状态，而不是只按目标 URL 或载荷静态分类。当前证据只能确认结果随页面环境翻转，不能唯一确定参与
  判定的是 Safe Browsing download protection、`IAttachmentExecute`、扩展、AV/EDR、管理策略还是其他状态。
- Chromium 官方源码中，拖出路径创建的 `DownloadUrlParameters` 保持默认
  `content_initiated=false`；`DownloadManagerImpl` 只对 `content_initiated=true` 的页面下载进入 automatic
  download limiter。该 limiter 拒绝是在下载项创建前回调失败，也不会生成本次观察到的 History
  `FILE_BLOCKED` 中断项，因此可排除它是这批失败记录的机制；这不排除后续安全检查。
- 2026-08-20 读取 Google Transparency Safe Browsing 公开 status endpoint 时，
  `kpanel.kejilion.eu.org` 当前未报告威胁，返回记录中的 threat flags 均为 `false`。这是当前公开站点状态，
  不等于 Chromium download protection 对某次拖出下载的 verdict；未公开 schema 不做进一步字段解码。
- 注册表未发现 Chrome/Edge 常见 `DownloadRestrictions` policy；发现 Windows Attachment Manager
  `ScanWithAntiVirus=3`。该值只说明附件会交给已注册防病毒程序扫描，不足以单独归因。
- `DownloadURL` 在同机同浏览器已多次成功，适合保留为尽力而为的渐进增强，但网页没有最终结果回调，
  不能把单次成功扩展成兼容承诺。显式单文件 ticket 下载与目录/批量 ZIP 是可观察启动错误、可由用户
  重试的可靠兜底；仍不能把浏览器或 Windows 最终落盘误报为成功。
- 修改 MIME、伪装扩展名、删除 CSP/`nosniff` 或把长期鉴权放进 URL 都不是安全修复。`localhost` 变化的是
  发起 origin，不是远端下载域；不能据此宣称更换 KPanel 下载域会修复，也不得用换域绕过客户端策略。

## 3. KodBox 1.69.01 源码核验

### 3.1 是否最新

本机唯一相关 Git 仓库为：

`C:\GitHub\kejilion-panel\.codex-tmp\kodbox-main`

核验结果：

| 检查 | 结果 |
| --- | --- |
| Remote | `https://github.com/kalcaddle/kodbox.git` |
| 分支 / upstream | `main` / `origin/main` |
| 本机 HEAD | `83ac1fd643b6440403681b0dd24d59005697108e` |
| Tag | `1.69.01` |
| Ahead / behind | `0 / 0` |
| 工作树 | clean |
| 远端默认分支 HEAD | 同一 SHA |
| GitHub latest release | `1.69.01`，2026-08-17 发布，非 draft/prerelease，同一 SHA |

已执行只读 `git fetch --prune origin`，fetch 前后 SHA 和 ahead/behind 不变。该 checkout 是 shallow
partial clone（`blob:none`），所以历史标签不能只靠本地列表判断；最新版结论同时使用远端 HEAD 和
GitHub Releases API 验证。

KPanel 的站点安装代码没有内置 KodBox 源码。`internal/sites/recipe_jobs.go` 实际通过
`/bin/bash <trusted-script> kodbox <domain>` 调用受信脚本；当前镜像固定的
`kejilion/sh@6fa7bcc7c2d15fe09d829cb9664941ff40bf4aaf` 会查询 `kalcaddle/kodbox/releases/latest` 后下载
该标签。宿主机 fallback 脚本只经过属主、权限、大小、许可标记和所需协议片段检查，不能仅凭这些检查保证
其 KodBox 下载逻辑与镜像固定脚本完全相同。因此“当前镜像安装时取 latest”与“已安装实例以后自动升级”
是两件事，已部署实例版本仍需在实例后台或文件中单独检查。

### 3.2 KodBox 拖出实现

官方发布源码只提供压缩后的 Webpack bundle；相关实现位于：

- `static/app/dist/main.js:2`，Webpack module 117，`DownloadURL` 字符位置约 782204；
- `static/app/dist/api.js:2` 存在同一份实现；
- 固定提交链接：<https://github.com/kalcaddle/kodbox/blob/83ac1fd643b6440403681b0dd24d59005697108e/static/app/dist/main.js#L2>。

已验证行为：

1. 仅 Chrome、`dragDownload` 未被设为 `0` 且 `G.disableDragOut` 未启用时绑定；目录/多选还要求
   `dragDownloadZip=1`，并在启用 `dragDownloadLimit` 时受大小上限和未知目录大小限制；
2. `dragMouseDown` 同时检查复制和下载权限；
3. `dragDataSet` 同步写入：
   - `DownloadURL = application/octet-stream:<name>:<url>`；
   - `text/kod-list`；
   - `text/uri-list`；
   - 内部 `text/kod-copy` 和窗口 marker；
4. 它没有使用 `DataTransfer.items.add(File)`、`Blob` 或 `new File`，本质仍是 Chromium 私有
   `DownloadURL`；
5. 单文件使用 `explorer/index/fileDownload`；目录/多选使用带 `disableCache=1`、`dataArr` 和
   `accessToken` 的 `explorer/index/zipDownload`；
6. `dropTips` 仅根据鼠标离开窗口和 `dropEffect != none` 显示“成功/准备下载”，不能确认随后是否被
   Chromium 或 Windows 阻止。

后端相关位置：

- `app/controller/explorer/index.class.php:737-763`：解析选择、生成/复用临时 ZIP，`disableCache=1`
  时直接输出；
- `app/controller/explorer/index.class.php:872-900,950-952`：单文件下载权限检查后交给 `IO::fileOut`；
- `app/controller/explorer/auth.class.php:23-26,66`：`fileDownload` / `zipDownload` 属于下载权限；
- 固定提交链接：<https://github.com/kalcaddle/kodbox/blob/83ac1fd643b6440403681b0dd24d59005697108e/app/controller/explorer/index.class.php#L737-L763>。

公共仓库没有包含最终 `IO` / `IOArchive` 实现，因此不能从该快照确认最终 Range、对象存储重定向和
全部响应头。`app/kod/ZipMake.class.php` 的头部代码没有找到此下载路径的调用点，不能替代缺失证据。

### 3.3 KPanel 与 KodBox 差异

| 维度 | KodBox 1.69.01 | KPanel 原实现 | KPanel 修正后候选方向 |
| --- | --- | --- | --- |
| OS 拖出协议 | 私有 `DownloadURL` | 私有 `DownloadURL` | 恢复为尽力而为的渐进增强，不承诺最终落盘 |
| 描述 MIME | 固定 `application/octet-stream` | 文件 MIME / ZIP MIME | MIME 不作为安全修复假设；受控 A/B 中两类均曾成功并随后失败 |
| URL fallback | `text/uri-list` + KodBox 自定义类型 | KPanel 自定义类型 + 文本描述符 | `DownloadURL` 与 KPanel 内部/跨 Panel 类型及文本描述符并存 |
| OS 拖出单文件 URL | 直接 `fileDownload` URL | `/files/content` | 保留受控内容 URL；失败时引导使用显式 ticket 下载 |
| 显式单文件下载 | 直接 `fileDownload` URL | Files 已经在点击后创建短时 ticket；Desktop 无该入口 | 复用同一 ticket，并新增 Desktop 入口 |
| 目录/多选 | 临时 ZIP 后输出，URL 带选择和 access token | OS 拖出使用 Agent 流式 ZIP；显式批量会逐文件建 ticket 且跳过目录 | 用户点击后 POST 创建短时 archive ticket，再用不含路径的短 URL 下载一个流式 ZIP |
| 拖出最终状态 | 用 `dropEffect` 推测并提示 | 页面同样无法观测 | 仅作为尽力增强，不把它当可靠成功路径 |
| 内部/跨站拖拽 | `text/kod-copy` | KPanel v1/v2 + 内存 token | 原载荷保留，未与 OS 下载耦合 |

KodBox 成功只证明受测 ZIP URL 在该次 Chrome 环境中通过了安全判定，不证明该站点的所有下载或通用网页都能稳定依赖
`DownloadURL`。新的严格 A/B 已排除固定 `application/octet-stream` 是充分修复：它与 `text/plain` 在
同一状态下一起成功，又在后续页面状态下一起失败。`text/uri-list` 和 access token 也没有证据能覆盖
KPanel 的公共资源失败，照搬 token URL 还会扩大 URL 泄露风险。某次拖出成功只能支持“渐进增强可用”，
网页仍收不到后续 `FILE_BLOCKED` 的最终回调，不能据此承诺所有 Windows 或策略环境稳定可用。

## 4. KPanel 候选方向

最新证据要求候选采用“渐进增强 + 明确兜底”，而不是彻底删除 `DownloadURL`，也不是宣称已经修复
`kpanel.kejilion.eu.org` 上观察到的客户端拦截。浏览器端方向如下：

- 在 Chromium 支持的鼠标拖出场景继续写入 `DownloadURL`，单文件使用受控内容 URL，目录/多选使用
  受限流式 ZIP URL；它只是尽力而为的 Explorer 拖出增强，不能进入可靠完成状态或发布兼容承诺；
- 内存 token、`application/x-kpanel-desktop-file-shortcut`、跨 KPanel v1/v2 载荷和文本回退与
  `DownloadURL` 同时保留。操作系统下载失败不能取消、降级或改写 KPanel 内部/跨 Panel 复制语义；
- 文件管理和桌面快捷方式同时提供明确的可靠兜底入口：
  - 单个普通文件：复用 Files 已有的短时 ticket 链路，并为 Desktop 提供相同入口；
  - 单个目录或任意多选：由同源、Session、CSRF 保护的 POST 创建 5 分钟 archive ticket，封存精确来源、
    `resourceVersion` 和 ZIP 名；无 Cookie GET 只暴露随机短 URL，不把完整选择写入浏览器地址或代理 URL；
    Panel 到 Agent 使用 JSON body 的内部 POST，完整选择也不会重新进入受 16 KiB 限制的内部请求行；
- 页面只可提示“已提供给系统；若被阻止请点击下载”，不得根据 `dragend`、`dropEffect` 或是否发出请求
  宣称 Explorer 已落盘，也不在未知最终状态下自动重试；
- 桌面混合选择包含应用、站点或缺失元数据时不显示显式下载入口，避免静默只下载部分选择；
- 集群身份暂不可用时仍允许桌面内文件快捷方式移动；只是不写跨 KPanel 描述符；
- 不修改 CSP、`nosniff`、鉴权期限或其他安全响应边界，也不将独立下载域作为规避客户端分类的方案。

后端完成以下收口：`walkArchive` 递归进入子项前过滤 protected descendant 和 `.kpanel-*` 内部临时组件；
`ExportZIP` 在写出任何字节前预扫描 symlink、特殊文件和资源预算；不同路径的同名顶层来源按 Windows
大小写不敏感规则确定性改名。Panel 新增 archive ticket 创建路由，复用现有 32 字节随机 token、只存
SHA-256、5 分钟 TTL 和 128 张全局容量；Agent 归档路由保留旧 GET，并新增只接受 `name` query 与
最多 256 KiB JSON body 的内部 POST。

明确不受影响的路径：Windows → KPanel 上传、同一 KPanel 页面内移动/复制、桌面图标移动、跨窗口和
跨 KPanel 文件复制。`DownloadURL` 只是与这些路径并列的私有附加载荷，成功与失败都不得改变它们。

## 5. 自动化证据与剩余边界

以下结果均在恢复 `DownloadURL` 渐进增强、加入显式下载兜底并 fast-forward 到上述代码基线后重新取得；
不挪用此前“删除 `DownloadURL`”候选的测试结果。

| 检查 | 最终结果 | 说明 |
| --- | --- | --- |
| 受影响 Web 测试 | 5 files，139 tests，全部通过 | 覆盖 API、helper、Files、Desktop 与显式下载模块 |
| 全量 Web Vitest | 106 files，807 tests，全部通过 | jsdom 输出两条既有 `scrollTo()` 未实现提示，不影响结果 |
| `npm run typecheck` | 通过 | `vue-tsc --noEmit` 无错误 |
| `npm run i18n:check` | 通过 | 20 个 lazy catalogs、2423 条 phrase |
| `npm run build` | 通过 | 2588 modules transformed，生产构建与预压缩完成 |
| Go FileManager / Agent | `go test ./internal/filemanager ./internal/agent -count=1` 通过 | 含递归过滤、预扫描和确定性同名处理 |
| Go ExportZIP 定向测试 | 相关定向测试通过 | 覆盖保护后代、内部组件、同名来源、symlink 与字节超限零输出 |
| Go Panel 文件套件 | `go test ./internal/panel -run '^TestFile' -count=1` 通过 | 含单文件/归档 ticket、无 Cookie GET/HEAD、Range、CSRF、严格输入及内部 POST 转发校验 |
| Go Panel 包全量测试 | 未通过：5 项既有 Windows 环境失败 | 2 项缺少静态构建产物导致 404；3 项 Windows `dataDir` 被配置校验判为非绝对路径；与本次归档过滤无调用关系，仍需 Linux release 门禁 |

最终 Web 测试已确认：

- 受支持 Chromium 拖动会同时保留 `DownloadURL`、内部 token、跨 KPanel v1/v2 和文本回退；
- 单文件显式入口只创建一个 ticket；单目录、两项及文件/目录混选只创建一张 archive ticket 并启动一个 ZIP；
- ZIP 名称处理 `.zip` 后缀、Windows 保留名、非法字符和 Unicode；
- archive ticket 短 URL 不泄露来源路径，GET/HEAD 无 Cookie，过期、容量和非法输入边界均受测；
- 超过 16 KiB 的合法选择集通过 Agent JSON body 到达业务校验，不再受内部 `MaxHeaderBytes` 限制；
- 桌面右键提供单文件 ticket 或目录/多选 ZIP，文件快捷方式 + 应用混选不进行部分下载；
- 集群身份不可用时桌面内移动继续工作，操作系统拖出成败不改变内部/跨 Panel 载荷；
- 页面不根据 `dragend` / `dropEffect` 宣称落盘成功，也不因无法观测的 `FILE_BLOCKED` 自动重试。

后端测试确认 protected descendant、`.kpanel-*` 文件/目录不会进入 ZIP，正常文件仍可读取；同名顶层来源
稳定改名，预扫描发现 symlink 或字节预算超限时输出保持 0 B。ticket、免 Cookie GET/HEAD、Range 与 archive
ticket 的严格校验均已通过。代码仍以 `maxCopyEntries`、`maxCopyBytes` 和 `downloadGate` 限制流式归档；
预扫描后若源发生竞态变化或后续真实读取失败，已开始的 HTTP 流仍可能是不完整 ZIP，页面无法接收结构化
错误。发布前仍必须在 Linux release 环境补跑项目门禁，并补充并发饱和及真实读取失败专项验证。

## 6. 后续 Windows 实机回归

候选构建发布前至少执行：

| 场景 | 预期 |
| --- | --- |
| Files 单文件 `.conf` / `.txt` / `.zip` | 各一个普通下载；名称、大小、SHA-256 正确 |
| Files 单目录 / 两文件 / 文件+目录 | 各一个 ZIP；顶层结构、空目录、内容 hash 正确 |
| Desktop 单文件 / 单目录 / 同类多选 | 菜单标签和实际 API 一致，只启动一次下载 |
| Explorer 拖出 | 受支持 Chromium 场景写入 `DownloadURL`，同时保留内部载荷；成功或 `FILE_BLOCKED` 均不宣称页面已确认落盘，并显示显式下载兜底 |
| 实服 `kpanel.kejilion.eu.org` | 记录精确 initiator、tab/navigation、referrer、载荷顺序和 History 结果；不得把候选描述为已解除现有客户端拦截 |
| 受控 `127.0.0.1` / `localhost` | 复跑 A～G 时序，确认页面只记录尽力增强，不把任一 origin 的单次成功固化为 MIME、hostname 或站点信誉结论 |
| 页面内文件拖拽 | 原移动/复制行为和修饰键语义不变 |
| 两个 KPanel | v1/v2 描述符、鉴权、复制、失败提示不变 |
| Windows → KPanel | 单项、批量、文件夹上传不变 |
| Chrome / Edge 受管 profile | 记录 policy 差异，不把客户端拦截报告为 KPanel 成功 |

网页不能自动验证最终 Explorer 落盘，`DownloadURL` 因而只能是渐进增强；明确点击的单文件 ticket 与
目录/多选 ZIP 始终保留为兜底。如果产品以后必须提供真正稳定的任意文件/目录 Shell 拖出，需要
桌面客户端、本地 helper 或受控文件系统挂载；File System Access 只能实现用户选择目标后的“保存到”，
不是拖到任意 Explorer 目录。

## 7. 可复现核验命令

```powershell
$kpanel = 'C:\GitHub\kejilion-panel-codex-windows-drag-download'
$kodbox = 'C:\GitHub\kejilion-panel\.codex-tmp\kodbox-main'
$go = 'C:\GitHub\kejilion-panel\.codex-tmp\go1.26.5\go\bin\go.exe'

git -C $kpanel rev-parse HEAD
git -C $kpanel status --short
git -C $kodbox fetch --prune origin
git -C $kodbox status --short --branch
git -C $kodbox rev-parse HEAD
git -C $kodbox describe --tags --always --dirty
git -C $kodbox rev-list --left-right --count 'HEAD...origin/main'
git -C $kodbox ls-remote --symref origin HEAD

Set-Location "$kpanel\web"
npm test -- --run src/lib/desktopFileShortcuts.test.ts src/lib/fileDownloads.test.ts `
  src/lib/api.test.ts src/views/FilesView.test.ts src/components/desktop/DesktopView.entries.test.ts
npm test -- --run
npm run i18n:check
npm run typecheck
npm run build

Set-Location $kpanel
& $go test ./internal/filemanager ./internal/agent
& $go test ./internal/panel -run '^TestFile' -count=1
git -C $kpanel diff --check
```

官方核验入口：

- KodBox Releases：<https://github.com/kalcaddle/kodbox/releases>
- Chromium drag download：<https://chromium.googlesource.com/chromium/src/+/refs/heads/main/content/browser/download/drag_download_file.cc>
- Chromium `DownloadUrlParameters` 默认值：<https://chromium.googlesource.com/chromium/src/+/refs/heads/main/components/download/public/common/download_url_parameters.cc>
- Chromium download manager 限流入口：<https://chromium.googlesource.com/chromium/src/+/refs/heads/main/content/browser/download/download_manager_impl.cc>
- Chromium automatic download limiter：<https://chromium.googlesource.com/chromium/src/+/refs/heads/main/chrome/browser/download/download_request_limiter.cc>
- Chromium Windows quarantine：<https://chromium.googlesource.com/chromium/src/+/lkgr/components/services/quarantine/quarantine_win.cc>
- Google Transparency 当前公开站点状态：<https://transparencyreport.google.com/transparencyreport/api/v3/safebrowsing/status?site=kpanel.kejilion.eu.org>

不得在证据中保存 ticket 明文、Session、CSRF、账号信息或含敏感绝对路径的完整 URL。
