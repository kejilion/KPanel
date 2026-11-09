# KPanel v1.30.0-rc.10 发布验收记录

日期：2026-11-10

发布级别：L3

候选提交 / 标签：`5b1d1331907b5833274cfcfca2a91db382a1f804` / `v1.30.0-rc.10`

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：预览候选 `release/v1.30.0-candidate`，tip `5b1d1331907b5833274cfcfca2a91db382a1f804`，纳入 RC10 并保留；来源分支 `desktop-open-chooser`，tip `0cdd313dc83d05d4f04f2dd2f43ac9c511cf1fbb`，经 `70cd3e12` 纳入；`desktop-icon-preview`，tip `43fa6186881a9a57fbb062b60a2819d0ad1717f2`，因安全审查的 P2 未认证出站抓取风险而排除，候选内由 `71f70e9` 撤销。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：预览候选不归档。Release workflow 提升时，`main` 与 `release/v1.30.0-candidate` 均为 `5b1d1331907b5833274cfcfca2a91db382a1f804`，不可变 tag `v1.30.0-rc.10` 指向同一提交；验收记录以独立 docs-only follow-up 合入 main 和预览候选，不改变 RC tag。来源分支 `desktop-open-chooser` 与被排除的 `desktop-icon-preview` 仍保留在远端，未做删除。
- 本次来源任务分支：`desktop-open-chooser@0cdd313` 已纳入；`desktop-icon-preview@43fa618` 未纳入，理由见上。具体差异见 [RC9 到 RC10 比较](https://github.com/kejilion/KPanel/compare/v1.30.0-rc.9...v1.30.0-rc.10)。
- 本地分支/upstream/worktree：本机 `C:\GitHub\_release-candidates\kpanel-v130` 的 `release/v1.30.0-candidate` 保留；验收记录作为 docs-only follow-up 后，tip 与 main/远端候选同步；未配置 upstream；工作树与发布证据保留。
- 未完成归档项 / 责任人 / 下次复核触发条件：两个来源分支远端引用未删除；本次授权仅覆盖 RC 与预览候选发布。由 Release Maintainer 在 v1.30.0 稳定版收尾时复核纳入关系和所有权，再按授权归档。候选工作树和验收证据未清理。

预览产物已发布，生产未部署。

## 发布画像

- 业务域：桌面网站快捷方式、音乐库与手机音乐播放栏。
- 变更面：展示、只读元数据缓存；无宿主机写入、协议、数据迁移或部署变更。
- 受影响用户旅程：从桌面打开网站时选择内置或系统浏览器；浏览音乐时搜索本地标签和文件名，读取版本缓存资料并播放；手机端展开较长歌曲与歌手信息。
- 未变化契约：Panel/Agent API 认证边界、Agent 权限、用户数据目录、端口、Compose、`kejilion.sh` 与应用市场协议。
- 风险等级及理由：中低；新增本地歌曲资料缓存和桌面打开选项，远端 favicon 自动抓取候选因 P2 风险排除。当前为 opt-in preview，未部署生产。

## 发布范围与未纳入内容

- 用户可见更新：桌面网站菜单可选择内置浏览器或系统浏览器；音乐页按文件版本缓存资料并按文件名和标签搜索；手机音乐播放栏可展开完整标题和歌手信息。
- 精确提交清单：公开 tag 的精确目标为 `5b1d1331907b5833274cfcfca2a91db382a1f804`；产品范围包含音乐缓存提交 `f5426288`、`2f4e2cf2`、`b7fb11a9`、`72407a3d`，桌面选择器提交 `0cdd313d` / `70cd3e12`，以及候选版本、验收和发布记录提交；完整祖先差异见上方 GitHub compare。
- 明确未纳入的分支、文件或后续事项：`desktop-icon-preview@43fa618` 未发布；本版不提供全曲库自动刮削或影视刮削，不新增电影/剧集匹配流程。手机改动仅涉及音乐播放栏，不代表通用手机终端优化。RC10 相对 RC9 未修改 AI 助手代码；助手既有功能沿用 RC9 基线。

## 外部审计与修复交付

- 安全盲审补漏：音乐相关包组的候选审查记录为 H0/M0/L0，confirmed=0、fixed=0、deferred=0。桌面图标候选另有 P2 未认证逐请求出站抓取问题，已从本版排除并撤销；不将此发现计入 RC10 已发布代码。
- `node scripts/security-review-scope.mjs --base v1.29.0`：`recorded groups=17 invalid_trailer=true`，没有 pending group；stable-range 聚合含历史格式不合规 trailer。精确 RC9→RC10 范围另核验为 `overall=recorded`、`internal/panel/server.go=recorded`、`invalidTrailer=false`。
- finding fingerprint / 修复 commit / 独立复核与回归证据：被排除图标候选的 P2 未进入公开 tag；音乐元数据缓存的版本绑定、文件关闭和恢复边界已在候选提交中修复，独立复核记录 PASS，来源审查为 H0/M0/L0。部分复核因其他 provider 不可用而采用 fallback，记录 `cross_provider=false`，不声称跨 provider 复核。
- 修复交付状态：音乐缓存和桌面打开选择器进入 RC10 源码、tag、Release 与 preview 镜像；没有生产部署。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：提交 trailers 记录了候选音乐变更的多轮覆盖（19/19、18/18、12/12 文件及后续 1/1 fixture 范围），constrained-only=0；最后一次局部记录为 H0/M0/L0。初始范围记录 free-form=2、H0/M1/L1；本记录不把该初始提示解释为安全 finding。skipped 与 unreported 计数未单列，故不推定为零；未宣称 OCR 工具有效性或应退出。
- 候选来源、原始 tip、处置、远端引用与责任人已在上方候选处置字段记录。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（RC10 未修改或发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：沿用 RC9 已发布脚本 `1faee13f3faef164efbfa899e244fc19ad58aaff` / `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选 commit / SHA-256：无新候选，沿用上述已发布基线；网站访问密码功能依赖的配套脚本未在 RC10 改动。
- 状态判定依据与兼容性证据：精确 L3 记录 `managed_script_contract=pass`，revision 与摘要匹配；公开验收使用同一冻结产物。
- 本版发布决定：脚本不在范围，面板 RC10 单独发布。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 CI、L3 和真实 Panel/Agent 公共浏览器验收通过 | 未连接真实第三方音乐服务做端到端刮削 |
| 网络入侵与供应链安全 | 已验证 | RC10 候选安全范围完整记录；L3 扫描、镜像契约和供应链门禁成功；P2 图标候选被排除 | stable-range 聚合包含历史无效 trailer；L3 原始库存仍保留已审查的上游扫描 finding，不代表零 finding |
| 稳定性、失败恢复与兼容 | 已验证 | 公共验收 150 项通过；包括升级、离线恢复、首次启动与重启 | 未执行长时 soak 或生产回滚 |
| 性能与资源预算 | 已实现未实机验证 | 缓存限制和淘汰有定向测试；双架构镜像运行通过 | 未做持续高并发或长时容量测试 |
| 用户体验与可访问性 | 已验证 | 精确候选本地预览搜索与播放通过；音乐播放栏窄屏布局覆盖 390×844 和 580×442；公共浏览器验收通过 | 未用实体手机验证；未完成全键盘、缩放与多语言矩阵 |
| 数据、配置与迁移 | 已验证 | 公开 upgrade 31 项、initial 48 项、restart 26 项通过 | 无生产数据迁移；未在生产环境执行升级 |

## 自动门禁

- 定向测试及结果：候选前音乐缓存相关定向验证 6 个文件 / 62 项通过；正式候选 CI 对精确 SHA 通过。
- `make verify-release` 环境和结果：GitHub-hosted Ubuntu 24，Go 1.27.3、Node 24.22.1、Docker 28.0.4；L3 两条并行验证 lanes 及部署检查均通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run `42637975925` / attempt 1；receipt 记录候选 SHA、GitHub-hosted、ubuntu24 image `20261101.360.1`、起止时间 `2026-11-09T14:47:21Z`–`15:06:13Z`、`passed` 和 entry SHA-256 `181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3`。receipt 未单列 plan/script/bundle 三个摘要及不可变 Runner ID。证据：`C:\GitHub\_codex-evidence\rc10-l3-42637975925`。
- 候选 CI：run `42636502087`，精确候选 SHA `5b1d1331907b5833274cfcfca2a91db382a1f804` 成功；Linux 与 Windows jobs 均通过。
- 主线 CI：正式预览 run `42637975925` 的 main-ci verify 与 Windows-node jobs 成功。
- Release workflow：正式预览 run `42637975925`，inputs、候选校验、L3、main CI、双架构发布、Release、公开验收和 preview completion 全部成功。
- 发布说明门禁：正式 Release 正文目标版本为 `1.30.0-rc.10`，2 条版本摘要、1 条升级提示。
- 安全扫描、镜像契约、SBOM/provenance：正式 L3 与 Release 门禁成功；公开验收校验两架构来源、版本、revision 与附件清单。L3 原始库存报告保留上游扫描结果，按既有 reviewed disposition 处理。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：由精确候选 L3 执行；报告在 L3 artifact 中。
- 最近每日安全通告审计、EOL 复核状态及证据：未在 RC10 独立验收记录中列出每日公告快照；不推断为另一次已完成的人工复核。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：无新增 module 版本；上游库存 finding 按 L3 artifact 中既有 disposition 保留。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无 Go 或 npm 依赖升级；固定工具链、Actions 与扫描器来自正式工作流。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Go `go.mod` / `go.sum` 未改变；npm lockfile 仅更新版本号到 RC10；OCI index `sha256:617167d14a868d4d98fcdfbe37e72421568f54441f1c493bf52669921b4873f5`；受管脚本沿用上一节基线。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：桌面 favicon 自动填充候选被排除，见候选处置字段；需修复未认证出站请求边界并通过独立安全复核后再纳入后续候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：双架构构建和运行契约通过；默认通道与稳定回滚点未改变；未作生产升级。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Ubuntu 24，Linux amd64；公开镜像另验证 linux/amd64 与 linux/arm64。
- 环境策略 ID 与允许用途：仓库 `environment-policy.json` 中 GitHub-hosted candidate/browser validation；仅隔离构建和验收。
- 使用的精确候选或公开产物：候选 `5b1d1331907b5833274cfcfca2a91db382a1f804`；公开镜像 OCI index `sha256:617167d14a868d4d98fcdfbe37e72421568f54441f1c493bf52669921b4873f5`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：preview run `42637975925` 成功；published 45、upgrade 31、isolated initial 48、restart 26 项均通过，errors 与 assetFailures 为 0。artifact 及截图由 run 保存；L3 本地证据目录见自动门禁。
- 测试窗口/循环数及风险依据：一次精确候选发布与公开验收，不做 soak；RC 为显式 opt-in preview。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：公共真实浏览器覆盖发布与升级旅程；本地精确候选预览验证音乐搜索、播放及桌面打开选项。移动窄屏证据为 Web 代码相同版本的 390×844 与 580×442 检查；未覆盖完整缩放、键盘/焦点、多语言矩阵或实体手机。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器运行 TLS 失败关闭、离线恢复、初始及重启验收；未写入生产主机。
- 未执行场景及原因：生产安装、真实外部音乐刮削、全曲库自动刮削、影视刮削、实体手机和长时 soak 均未执行；RC10 只包含本地资料缓存和播放器 UI。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：公开验收确认显示 RC10、对应 OCI digest、非空摘要和升级提示；published/upgrade 报告分别通过 45/31 项检查。
- GitHub Release 的 draft / prerelease / Latest 状态：`isDraft=false`、`isPrerelease=true`，发布时间 `2026-11-09T15:49:50Z`；`/releases/latest` 仍为 `v1.29.0`。https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.10
- Docker 版本与通道 OCI index：`kjlion/kejilion-panel:1.30.0-rc.10` 与 `:preview` 为 `sha256:617167d14a868d4d98fcdfbe37e72421568f54441f1c493bf52669921b4873f5`；`:latest` 仍为 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:a378c8a5fe12ae2844c8cbc77c347d40b91cd1cc4fd285de0ed632709d54b609`；arm64 `sha256:898e789ef8a9b2cd4a154d23ff5165e97df88e7cadf8a24d56469d80f235aa0c`。
- 附件及 `SHA256SUMS`：19 个 Release 附件；`SHA256SUMS` 有 15 项，本机复算 SHA-256 为 `a067a8e836aad09da537ab626fd71cb25dbece6bf4e49e085846df4c7dc04b09`，与 GitHub asset digest 一致。
- 公开镜像 `image_e2e=pass`：真实 Panel/Agent 隔离容器与浏览器验收通过；四份报告共 150 项，errors=0、assetFailures=0。
- `kejilion/apps` / `kejilion.sh` 契约结论：RC10 未改应用市场或脚本；面板内置脚本的 revision/hash 与公开快照和镜像标签契约一致。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：公开验收通过。
- 加入预览只切换来源并立即检查，没有自动安装：published 报告通过。
- 自动安装开关与一次性立即安装相互独立：upgrade 31 项通过。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：initial/restart 共 74 项通过。
- 退出预览且稳定版较低时没有产生降级候选：published 报告通过。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：公开升级流程通过；真实生产 systemd 服务未验证。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：是；本版未扩大支持边界。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：GitHub-hosted 隔离环境；未使用 `prod-108`。
- 正式部署环境：不适用；未连接 `arena-154`。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用，未执行生产操作。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用，未做生产部署。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3、双架构构建、公开浏览器与容器 E2E、初始/重启/恢复验收。

## 回滚

- 源码/tag：稳定回滚点 `v1.29.0`（提交 `174fe03a2dc5db2c69351487936df15c9f07736c`）；RC10 tag 不可变，target `5b1d1331907b5833274cfcfca2a91db382a1f804`。
- 镜像 digest：稳定回滚 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`；RC10 `preview` 为 `sha256:617167d14a868d4d98fcdfbe37e72421568f54441f1c493bf52669921b4873f5`。
- 数据/配置备份：无生产数据变更，因此无生产备份。
- 回滚步骤和回滚后复核：在加入预览的隔离实例退出 preview 并按标准更新流程处理已安装版本；退出通道不会自动降级。生产未部署。
- 回滚后生产实际版本与健康状态：不适用。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：`v1.29.0` stable，镜像 `sha256:587b51...`；preview 保持 opt-in。
- 公共默认更新通道决策：恢复上一稳定版本（保持 `v1.29.0` 为 Latest / `latest` 默认，RC10 仅 preview）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-08T08:43:48+08:00
- 候选冻结时间：2026-11-09T22:46:34+08:00
- 生产完成时间：不适用
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（无产品变更失败、回滚、紧急热修复或重复发布；首次预检失败单独计入流程异常）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preview-preflight/trusted-main-check/test-fixture-diff",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 preview run 42633638699 因 scripts/mock-music.mjs 的候选差异不符合 main 上受信脚本基线而被 inputs preflight 拦截；没有公开或生产写入。",
    "recoveryEvidence": "提交 6b1e24e 将仅用于本地 mock 的 fixture 恢复到 main 基线；最终精确候选 CI 42636502087、L3 与公开 preview run 42637975925 全部成功，远端 main/candidate/tag 最终目标均核对为 5b1d1331907b5833274cfcfca2a91db382a1f804。",
    "permanentAction": "保持预检 fail-closed，不放宽 trusted-main 检查；Release Maintainer 在 RC11 冻结前核对 scripts/ 差异，需调整 mock fixture 时先独立复核并集成到 main。退出条件：预检对候选 fixture 与受信发布自动化差异保持清晰区分且仍拒绝未集成自动化。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：未执行删除或回收；保留当前 RC10 证据、截图、公开验收产物与 clean candidate worktree，路径见上方。未清理活跃或恢复所需内容。
- 未验证风险：没有真实外部音乐服务刮削、全库音乐刮削、影视刮削、实体手机、长时性能 soak、生产升级或部署验证；公开预览不等于生产安全验证。stable-range security 汇总提示历史 invalid trailer，精确 RC10 delta 无效 trailer=false。
- 已实现待实机准入：文件版本缓存和窄屏播放栏可在 preview 中试用；需真实手机与实际音乐文件/标签场景继续验证。AI 助手没有 RC10 差异改动，未在本轮另做完整 AI 旅程矩阵。
- 不阻断本版的理由：RC10 是显式 opt-in preview；候选 CI、精确 L3、主线 CI、双架构镜像发布和真实 Panel/Agent 公共验收均成功；稳定通道、Latest、Docker `latest` 与生产均未改变。
- 后续应进入的自动门禁或专项工作流：修复桌面图标候选的未认证出站抓取边界后再独立复核；RC11 前保持并复核 scripts trust preflight；未来稳定版发布前复核来源分支所有权与归档授权。
