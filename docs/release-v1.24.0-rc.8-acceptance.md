# KPanel v1.24.0-rc.8 发布验收记录

日期：2026-10-02

发布级别：L3

候选提交 / 标签：`e0684ae924bcc093397af0773297c52dc8713ea7` / `v1.24.0-rc.8`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` 保留，唯一产品工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly`，本地/远端 tip 均为 `e0684ae924bcc093397af0773297c52dc8713ea7`。

## 发布画像

- 业务域：图库、文件管理入口、统一主机选择器和历史监控入口。
- 变更面：前端展示、媒体读取和已有上传/目录/改名/回收站动作；没有新增后台 API、数据库、端口、Compose、Agent 权限或宿主脚本协议。
- 用户旅程：从桌面/开始菜单或文件管理打开目录，按月份和相册浏览照片/视频，上传重名媒体、删除/恢复，切换主机和关闭窗口。新 UI 组合及既有宿主写入入口按 L3 验证。
- 文件仍是普通目录文件，偏好按浏览器保存；未增加排他图库索引、后台扫描服务或服务端转码。

## 发布范围与未纳入内容

- 批准主线基线 `55a807d55795bf2f77a94035e83bf514ac17163e`；来源 `claude/media-gallery` / `85982e5bca36a3b41df4ec881e583370bcc3ebd0`，16 个来源提交、54 文件。发布结果相对基线 59 文件、7071 插入、692 删除；完整提交及差异见 `contract.json` 和 `review-final.patch`。
- 图库入口、相册/月份、筛选、照片缩放、视频播放、悬浮查看器、上传重名处理、回收站、图库/文件互相刷新，以及 Files/Gallery/Monitoring 共用的主机选择器。
- 发布 owner 修复 `d336ed453e78c4210dfebd958e809f2b23f9d5f1`：视频海报按来源主机隔离、同路径切换清除旧快照、关闭后不启动排队上传、保留旧列表时仍显示刷新错误与重试。`374d965137fb47771a75185ba033cff3fa5a94fd` 只修复屏幕外时间线占位导致窄布局撑宽。
- 未纳入：已按用户决定归档的 TS7 基准、Node 26 Current、治理草稿与历史 fallback；未合并新外部安全审计或生产变更。Gallery 默认 `/home/gallery`，时间线按修改时间，不宣称 EXIF 拍摄时间归档。

## 外部审计与修复交付

- CF 覆盖 `decision=scoped-required`；59 个未审计提交、110 文件、最老 6 天；last full run-4 距离 11 天，新增边界包为早期候选中的 `internal/backupremote`。RC 仅记录，稳定版前补审；本轮没有执行 scoped/full 或宣称 CF 审计通过。
- 原实现者 Claude 与发布复核者 Codex 不同；自由臂确认 4 个 MEDIUM 并保留原失败回归，修复后 22 定向测试及类型检查通过。真实 Chromium 又复现占位宽度缺陷并修复；原 browser-r1 和诊断原件保留。
- OCR 1.12.11：`55a807d55795bf2f77a94035e83bf514ac17163e..59e87f09acb398ecad4a9c1fe53a9dfa80634996` 自由臂先于圈选，51/51 文件覆盖，8 排除文件也人工检查，free-form=4、constrained-only=0。增量 `83bf6cf..d033add` 1/1 覆盖，未新增约束臂问题；合并观察不能当作第二个独立盲测周期。
- Codex 发布修复没有另一个模型提供商复核，不把根任务的自查冒充跨 provider 复核。后端边界未变化，真实失败回归、最终完整 L3 和真实 API/浏览器提供修复证据；稳定补审仍保留。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`；无需发布脚本（不适用）。变更集编号和脚本候选：不适用。
- 实际内置脚本：`c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`，与上一 RC 相同。发行安装模板、生命周期及受管脚本 byte contract 通过；本版没有脚本仓库写入。
- `kpanel.conf` 归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；应用市场仍 stable/latest。公开镜像实际脚本、配置、VERSION 和两个图库图标逐字摘要核验通过。
- 实际隔离目录上传、改名、回收站恢复后可由普通 Shell 读取相同媒体 bytes；该证据只证明同一普通文件真源，不声称执行了图库专属脚本命令。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 真 Panel/Agent API 14 项、真实浏览器 4 项、普通目录 bytes | 非用户生产目录，未完成真实多远程节点全旅程 |
| 网络入侵与供应链安全 | 已验证 | L3、固定 script bytes、双架构 OCI/attestation | CF 稳定前补审；不宣称全依赖无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | 主机隔离/关闭队列回归、冲突和 stale 拒绝、Agent 停止/重启/回收站恢复 | 无长期网络故障或生产数据恢复演练 |
| 性能与资源预算 | 已验证 | 扫描/并发/缩略图/海报/查看器有界，受限浏览器和 256MiB Panel 夹具 | 无大图库或长期生产性能数据 |
| 用户体验与可访问性 | 已验证 | 17 Mock 组合、真实解码帧/上传/回收站、焦点和键盘、多语言 | 缩放为 CSS 模拟，未验证浏览器原生缩放或全部设备 |
| 数据、配置与迁移 | 已验证 | 普通目录、版本化动作、重启后恢复，既有全量回归 | 没有新格式迁移，未修改正式数据 |

## 自动门禁

- 固定入口 `scripts/run-release-l3.mjs`，最终 run `v1.24.0-rc.8-e0684ae9-l3-r3`，精确候选 `e0684ae924bcc093397af0773297c52dc8713ea7`；终态 passed/exit_code=0，12 项原始证据 SHA-256 全部核验。Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0。
- bundle `b6335c67cb747222fdb34c012dabf132e0776db174714db4373a5c05d4489d91`；plan `0c042f734946fe345b8e5ca722c9846dfb48372fa3b9659dcb7d7437cb1fad2d`；remoteScript `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；原件 `C:/GitHub/_validation/kpanel-v124-rc8-l3-r3`。
- 全量 Go、类型/前端 236 文件 2110 测试、race、vet、govulncheck、npm audit、Trivy source/image、Linux 二进制、受管脚本和 app-conf lifecycle 全部通过。govulncheck 可达调用及导入包结果为 0，另记录 1 项未调用的 require 模块观察；不改写为全依赖无漏洞。
- 首轮源码准备 fetch 超时在载荷前退出并清理；旧候选 r2 已通过但被布局修复替代，不用于最终候选结论。
- 候选 CI `37011285538`、新鲜度 `37011285563`；产品主线 CI `37012121143`、新鲜度 `37012121168`；Release `37012839662`：均准确 SHA completed/success。

## 依赖与技术栈变化

- Go 1.27.1、Node 24.21.0 LTS、TypeScript 6.0.3、Action/镜像/扫描器 pin 沿用 RC7；package-lock 仅根发行版本变化，无新依赖。
- 精确候选与主线的 Dependency freshness push 作业执行官方候选检测并成功；本轮未独立下载其 step summary，不能据此称每日安全通告/EOL 全量复核已完成。L3/CI 当前安全扫描独立成立。
- 原 TypeScript/@types-node 例外和 Trivy 0.75 qualification 由 KPanel base maintenance 在 2026-10-15 复核；既有首次检测及采用时限不重置。Node 26 不搭车采用。

## 隔离真机与浏览器验收

- `arena-154`，purpose candidate-validation；现有 18 个业务容器身份、运行/健康状态和 StartedAt 与本轮新基线逐项一致，未用 RC7 旧状态冒充本轮基线。
- Mock 作业 `arena-154-10364`：准确候选、passed/exitCode=0、650 秒外层限制，规格 `browser-spec-r5.json` SHA-256 `7202c936509b961629fc6cbd243f076cc23afd1ce740f8d1bb035fc56ed6b9ed`。实际 Chromium 140.0.7339.16 / Playwright 1.55.0 在 arena 执行，通过任务私有 SSH 回环 relay 访问本地候选 Mock。
- 390/768/1280、浅/深色、CSS 100%/200%，1700 的经典/桌面 CSS125%；最小计算字体 12px。相册/筛选/查看器、方向键/i/Escape、主机选择器焦点恢复、英语/繁中、上传重名/相册/回收站/503重试同场组合通过。pageErrors 与 unexpectedErrors 为 0；4 已知 Mock 404 和故意注入 503 分列，不称控制台完全没有任何错误。
- 真实 API 使用 final L3 构建的不可变本地 image ID `sha256:04108f53f729d63a184c5f5d9233209f16250f4fd8d595af64cfe7fa998b15e7`，14 项业务/普通文件/失败恢复结果；真实浏览器对同一隔离 Panel 验证 VP8 WebM 的实际解码帧、上传 bytes 与回收站持久结果。该 Chromium 实测无法解码夹具 H.264（错误码 4），单独验证了提示及原文件下载 bytes；不声称 H.264 播放通过。pageErrors 和未预期控制台错误为 0；夹具中的应用市场 `/api/v1/apps` 503 独立记录，规格/后台终态保留于 `real-browser-job`。
- 硬资源限制、资源采样、精确容器/网络/隧道及临时源清理终态见 `browser-resources-r5.json`、`real-browser-resources.json`、`arena-cleanup.json`。无生产媒体写入，没有长期 soak；媒体旅程为有限确定性回归。
- 未执行：原生浏览器缩放、所有 HEIC/TIFF/视频编码、真实远程主机断连/大图库、arm64 原生浏览器、长期媒体 soak 或真实设备接入。本版不包含 Node 后端变化，新公开 Node bytes 未重复执行 RC7 原生 procd 测试。

## 发布产物与公开仓库复核

- [GitHub prerelease](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.8) 于 `2026-10-02T13:38:39Z` 公开，draft=false/prerelease=true；GitHub Latest 仍 `v1.23.0`。
- 版本/preview index `sha256:b6a88574f55143e18412ffa0586d26e0ebfbe44ad9d96290063376a233001c78`；amd64 `sha256:f7ff36f85472d423767f050a207a3b018d2cc25b9e2b6c3f7851510467fe3ad3`；arm64 `sha256:b682b35f77246c0433d6408750c8400adc141f0b74806832f3128868551e3a38`；latest 仍 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。
- 14 附件；SHA256SUMS、metadata archive 和 amd64 Node 实际 bytes 验证，11 条 SUM 与 GitHub digest 相符，10 metadata 文件与精确 Git blob 相同。其他独立二进制未全部执行，不声称公开 Node 与打标签前 L3 bytes 相同。
- 双架构 manifest/config 原始 byte digest、version/revision/非 root/entrypoint/script labels、SPDX/SLSA subject binding 通过；不是完整 SBOM 内容审计。
- 公开 immutable amd64 镜像由仓库 `packaging/tests/image-e2e.sh` 验证 `image_e2e=pass`，独立 image ID bytecheck 通过；公开测试容器/网络/临时数据已移除。

## 自更新通道验收

- 既有完整回归覆盖 stable/preview 合法来源、唯一 digest、开关/一次安装、重启保存和退出不降级。本轮只提升 preview，默认安装与无人值守轻量 Node 继续 stable/latest；加入预览不会自动安装，退出预览不会自动降级。
- systemd 备份/失败隔离、OpenRC/轻量 Node 边界保持上一 RC；本轮不对生产设备实际安装 RC，也不宣称用户设备自动获得图库。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）。产物已发布，生产未部署；生产写入 0，没有生产备份、升级、重启或回滚。
- `prod-108`/108 禁用全部 KPanel 操作；本轮未连接、备份、部署、升级或核对。

## 回滚

- 上一 preview `v1.24.0-rc.7` / `3015c1fb2ba6918d445322d234f6543ddb0ac978` / `sha256:e8fd978f1286826950b062324523037a728021f580eafc05c880690a7e4cfa74` 保留；稳定 `v1.23.0` 保留。
- 本版不迁移图库数据，普通目录文件仍可由旧文件管理/Shell 管理。共享 main 只做聚焦 revert 并重验，不改写历史；实际设备回滚必须先备份数据，通道开关不代替安装回滚。
- 稳定默认未变化；生产回滚、生产备份和回滚后生产版本：不适用。

## 候选与来源分支处置

- 发布候选保留，main 在其上另加验收文档。来源 `claude/media-gallery` 精确 G 已被 tag 包含；原作者未明确释放工作树所有权，本地树及其既有 4173 预览原样保留，不删除、不重置、不移除未知分支。
- 本发布 owner 修复分支 `fix/rc8-gallery-context-20261002` tip `374d965137fb47771a75185ba033cff3fa5a94fd` 已保存到 `refs/heads/archive/fix/rc8-gallery-context-20261002`，远端 SHA/祖先恢复验证及 clean worktree 回收见 `branch-disposition.json`、`local-cleanup.json`。
- 验收文档候选 `docs/release-v1.24.0-rc.8-acceptance` 在同 SHA 候选与主线 CI 完成后，按 10.2 保存到只读 `archive/docs/release-v1.24.0-rc.8-acceptance` 并移除活跃引用/本地树；精确最终 tip、实际归档及清理结果在仓库外 `acceptance-archive.json`。记录本身不制造第二个产品版本。
- 待处置：Gallery 来源所有权确认，由原开发负责人在下一次完成交付/释放时复核；TS7、Node 26 和治理草稿不因本轮清理而删除。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T12:22:24+08:00
- 候选冻结时间：2026-10-02T20:26:45.158+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：38
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/command/guessed-resource-path",
    "count": 16,
    "impact": "8 earlier invalid resource operations plus 8 continuation operations: one theme-file search, two nonexistent mock entry searches, agentd and agent guessed entry searches, premature browser-r2 log read, nonexistent packaging Dockerfile and literal Go glob. No business writes.",
    "recoveryEvidence": "Repository path inventories; final exact frozen candidate and validated helper inventory.",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-03 复核，下一次生产写入前把发现与预检纳入唯一已审查入口并补回归。本次保留失败原件并采用已核对路径/规格，不宣称永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ocr-cli/unsupported-help",
    "count": 1,
    "impact": "Unsupported OCR --help call rejected; source workflow and CLI parser used instead.",
    "recoveryEvidence": "sizing-ocr-preview.json, sizing-ocr-rules.json, sizing-ocr-coverage.json",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-03 复核，下一次生产写入前把发现与预检纳入唯一已审查入口并补回归。本次保留失败原件并采用已核对路径/规格，不宣称永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/git-trailer/separate-paragraph",
    "count": 1,
    "impact": "Writer check reported stale OCR because trailer paragraphs were separated. Only unbound local review commit was corrected before final preview/L3.",
    "recoveryEvidence": "sizing-review-message.txt; final writer check ocr_line_review=recorded",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-03 复核，下一次生产写入前把发现与预检纳入唯一已审查入口并补回归。本次保留失败原件并采用已核对路径/规格，不宣称永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/source-prepare/git-fetch-timeout",
    "count": 1,
    "impact": "r1 failed during isolated clone fetch before candidate payload; temporary source removed.",
    "recoveryEvidence": "l3-orchestration.log, validation r1/source-prepare.json; r2 passed for old candidate, r3 runs final candidate",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-03 复核 SSH upstream reliability. Same authoritative entry and fixed Runner retry; close when exact final candidate has passed and temporary source absence is verified.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/harness/shared-theme-fixture",
    "count": 1,
    "impact": "Mock shared appearance persistence could override requested per-context theme; those r1 theme labels are not accepted as verified coverage.",
    "recoveryEvidence": "r2/r5 explicit per-context appearance response and dataset.theme assertion; r5 17 cases passed",
    "permanentAction": "browser-r5.cjs fixes per-context appearance and asserts resolved theme; retain original r1 evidence. Unique task-only harness, not claimed as a permanent repository entry.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/harness/newline-byte-hash",
    "count": 1,
    "impact": "r2 rejected at script hash before browser launch: Windows text write translated LF to CRLF.",
    "recoveryEvidence": "browser-job-r2 terminal; r3+plan actual read_bytes SHA; r5 passed",
    "permanentAction": "Hash actual saved bytes before remote copy, as browser-plan-r5.json; retain rejected plan and original script.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/harness/unmounted-module-target",
    "count": 1,
    "impact": "r3 browser container could not resolve a symlink pointing outside its mounted tool directory; no product scenario executed.",
    "recoveryEvidence": "browser-remote-r3.log; prepare-browser-r4.py module preflight; r5 passed",
    "permanentAction": "Own tool directory contains dependency copy; network-none bounded require preflight validates 1.55.0 before background launch.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/harness/transient-overlay",
    "count": 1,
    "impact": "r4 mouse action retried beneath transient album toast until viewer controls auto-hid; persisted album/upload results were verified and retained.",
    "recoveryEvidence": "browser-r4/result.json; r5 waits transient overlay removal, reveals/holds controls and verifies final filesystem-like mock results",
    "permanentAction": "browser-r5.cjs uses normal pointer movement/hover after transient toast, without forced clicks or dropping durable assertions.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "permanentAction": "Bind actual Docker identity to final L3 exported OCI index, retaining distinct config and platform manifest digests.",
    "impact": "First real API preflight compared an OCI index identity with config digest and stopped before fixture writes.",
    "recoveryEvidence": "run-real-api-r1.py; final L3 config/manifest/index exports and Docker Descriptor show exact index identity.",
    "position": "before-production-write",
    "fingerprint": "preflight/docker-identity/index-config-assumption",
    "historicalReleases": [],
    "count": 1
  },
  {
    "recoveryEvidence": "real-api-raw-r1/result.json; real-api-execution-r1.log; bounded readiness retry in revised fixture",
    "impact": "Connection reset during first startup health probe escaped readiness retry; no API business cases executed. Own containers/network removed by fixture finally.",
    "position": "before-production-write",
    "historicalReleases": [],
    "count": 1,
    "permanentAction": "Readiness only handles normal OSError startup failures within the existing bounded attempt window; business assertions remain strict.",
    "fingerprint": "real-api/harness/startup-reset-unhandled"
  },
  {
    "historicalReleases": [],
    "count": 1,
    "position": "before-production-write",
    "recoveryEvidence": "real-api-raw-r2/result.json; agent parseFileContentQuery allows only path/disposition/mode/version",
    "permanentAction": "Revised fixture uses exact existing Gallery thumbnail URL contract without dimension knobs; source endpoint and assertions remain unchanged.",
    "impact": "Fixture added width/height to fixed existing thumbnail contract; strict parser rejected the request after five passed business cases. Own fixture cleaned.",
    "fingerprint": "real-api/harness/unsupported-thumbnail-query"
  },
  {
    "fingerprint": "real-api/harness/port-reuse-bind",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Fourth attempt failed during Python exclusive bind preflight, before container creation. No listener or task containers were present on subsequent ss/docker inspection; TIME_WAIT was not independently captured.",
    "recoveryEvidence": "real-api-execution-r3.log; empty real-api-raw-r3; ss no listener on 18089/18090; unique r4 fixture uses freshly checked loopback 18090",
    "permanentAction": "Use an explicitly checked fresh loopback port for retained browser fixture; preserve failed attempt and avoid removing unknown port owners."
  },
  {
    "fingerprint": "diagnostic/path-guess/frontend-src",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Read-only rg used an unverified frontend/src directory; corrected by rg --files inventory to web/src.",
    "recoveryEvidence": "Exact file inventory and web/src/views/FilesView.vue / GalleryView.vue actual rename calls",
    "permanentAction": "Use established web/src paths and inventory before uncertain paths."
  },
  {
    "fingerprint": "real-api/harness/rename-target-contract",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Rename fixture sent name instead of required destination target; stopped after eight passed cases; fixture finally removed its containers/network.",
    "recoveryEvidence": "real-api-raw-r4/result.json; real-api-execution-r4.log; actual web/src rename target contract",
    "permanentAction": "Use exact UI rename destination target and retain HTTP status/body in failed action assertions."
  },
  {
    "fingerprint": "real-api/harness/multi-status-refusal",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Stale-version refusal returned documented 207 with empty succeeded and one failed, but generic fixture helper incorrectly accepted only 200; own fixture cleaned after nine passed cases.",
    "recoveryEvidence": "real-api-execution-r5.log; real-api-raw-r5/result.json; internal/agent/files.go StatusMultiStatus",
    "permanentAction": "Accept the endpoint documented 200/207 result envelope; retain strict per-case succeeded/failed and physical-file assertions."
  },
  {
    "fingerprint": "diagnostic/path-guess/docker-entrypoint",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Read-only rg used an unverified packaging/docker-entrypoint.sh path; no accepted evidence came from it.",
    "recoveryEvidence": "No source mutation; existing Gallery source and fixture HTTP responses used instead",
    "permanentAction": "Inventory packaging paths before uncertain file reads."
  },
  {
    "fingerprint": "browser/harness/search-render-race",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "First real browser timed out waiting for H264 video. A search/render race was initially hypothesized but not established; exact-name retry reproduced codec failure.",
    "recoveryEvidence": "real-browser-r1/result.json; real-browser-r2/failure.png and trace show unsupported codec UI; retry adds actual H264 decode probe and VP8 WebM positive playback",
    "permanentAction": "Use exact accessible target, capture failures, probe actual decoder before codec-specific assertions; expected isolated app 503 must match actual HTTP response."
  },
  {
    "fingerprint": "diagnostic/path-guess/workflow-root",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Read-only rg used .codex/workflows instead of existing .codex-workflows; actual path was then obtained from rg --files.",
    "recoveryEvidence": ".codex-workflows/release-kpanel.workflow.yaml inventory; earlier verified release workflow remains authoritative",
    "permanentAction": "Use known .codex-workflows store and inventory before uncertain resource access."
  },
  {
    "fingerprint": "browser/harness/missing-codec-preflight",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Exact-name second browser repeated H264 failure before positive playback; screenshot shows expected unsupported-codec fallback. Actual runtime capability had not been probed.",
    "recoveryEvidence": "real-browser-r2/result.json and failure.png; bounded real decoder probe and positive generated VP8 WebM fixture",
    "permanentAction": "Record actual decoder result and assert original-byte download fallback; require separate positive decoded frame case rather than treating unsupported media as playback success."
  },
  {
    "fingerprint": "browser/harness/download-scope-strictness",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Download fallback assertion matched both toolbar and notice buttons; Playwright strict mode stopped before positive playback.",
    "recoveryEvidence": "real-browser-r3/result.json and failure.png; notice-scoped accessible selector",
    "permanentAction": "Scope fallback control to the visible unsupported-media notice, retaining exact role/name and byte assertions."
  },
  {
    "fingerprint": "evidence/github-connector/wrapper-shape",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Read-only workflow jobs tool returned structured jobs, but formatter incorrectly parsed nonexistent content as JSON; CI runs and gate files were unchanged.",
    "recoveryEvidence": "Actual structuredContent.jobs inspection; release gate continues using raw github_fetch run JSON",
    "permanentAction": "Read each connector returned shape before formatting; do not reuse github_fetch text-envelope assumptions for specialized wrappers."
  },
  {
    "fingerprint": "acceptance/log-parser/ansi-count-format",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Acceptance generator expected plain Vitest count text, but immutable L3 orchestration log contains ANSI styling; generator stopped before repository writes.",
    "recoveryEvidence": "Original log lines 1579/1580 repr show 236/2110 passed with ANSI; l3 terminal exact candidate passed and 12 raw checksums verified",
    "permanentAction": "Strip ANSI only in the reader and retain original logs unchanged; assert exact test totals and L3 terminal marker."
  },
  {
    "fingerprint": "acceptance/release-metrics/invalid-fingerprint-format",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "impact": "Local L0 gate rejected an incident fingerprint with two segments; acceptance candidate was not committed or pushed.",
    "recoveryEvidence": "acceptance-before-fingerprint-fix.md item 21; authoritative report-release-metrics three-segment fingerprint regex",
    "permanentAction": "Validate all incident fingerprints against the authoritative slug schema before committing; maintain exact aggregate count."
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：root 持有的修复 worktree 已回收，精确路径/实际空闲增量/恢复引用见 `local-cleanup.json`；候选与来源活跃树保留。任务 Mock 预览在验收结束后停止。远端仅回收本次私有容器、网络、隧道与源码/inbox，可追溯唯一证据和固定 Runner 保留；没有全局 prune。
- CF scoped 补审、大图库/长期媒体和真实远端主机旅程仍待后续准入，原生浏览器缩放及全部编码/设备未覆盖。不阻断有限 RC 产物发布，不能据此批准稳定版或生产安装。
- 本轮复用既有 release/local-preview/background-browser/OCR 工作流，未新增知识库或治理工作流；一次性验证规格在仓库外留证，同类旅程再次出现时按现有工作流要求沉淀仓库入口。
