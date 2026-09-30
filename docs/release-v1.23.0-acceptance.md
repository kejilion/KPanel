# KPanel v1.23.0 发布验收记录

日期：2026-09-30

发布级别：L3

候选提交 / 标签：b73d62628ed77e12ce03715fafc1aa490b884f96 / v1.23.0

上一稳定版本 / 回滚点：v1.22.0 / 源码 cea6261f9ae1de064216a1c423d3cd3f2d87d3fa；Docker OCI index sha256:46b854bdf6233e81742986a3bee9e65c6b4a39123c393a51a4ada63783eed7ac

releaseChannel：stable

releaseTrain：1.23.0

候选分支与发布后处置：release/v1.23.0-candidate / 稳定版归档

- 原分支 / 精确 tip / 处置分类：release/v1.23.0-candidate，b73d62628ed77e12ce03715fafc1aa490b884f96；正式 Release 成功后归档。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：archive/release/v1.23.0-candidate 指向 b73d62628ed77e12ce03715fafc1aa490b884f96；远端活动候选 ref 已移除；main 与 v1.23.0 标签 peeled commit 均指向同一 SHA。
- 本次来源任务分支：纳入版本/替代依据、归档或保留理由：RC.1–RC.8 的已成型内容按各自验收记录纳入；RC.8 来源 fix/classic-refresh-background-20260930 tip 13e26899bbc17880c25fddb0e4f64561f4c4208b 和 feature/site-branding-20260930 tip 7bb41f12bda659020a2f0f8bcf06face641ac985 已分别归档，远端 SHA 与原 tip 一致。其余来源处置见 docs/release-v1.23.0-rc.1-acceptance.md 至 rc.8 记录。
- 本地分支/upstream/worktree：已回收 / 保留及原因（不能以远端归档代替本地核对）：本地稳定候选 worktree C:/GitHub/_codex-tasks/kpanel-v123-preview-release 与同名分支已回收；C: 实际增加可用空间 315,437,056 字节。L3 和公开产物证据保留在下方路径。独立审计、用户预览及归属未确认的工作树未删除。
- 未完成归档项 / 责任人 / 下次复核触发条件：其他任务/用户预览及审计工作树不属于本稳定候选清理；由发布负责人在对应任务结束、确认所有者和精确 refs 后复核。不得据此宣称所有本地旧目录已清理。

归档不代表生产上线；本次正式产物已发布，生产主机未部署。

## 发布画像

- 业务域：备份与恢复；集群主机资料、流量统计、分享和提醒；桌面交互、通知、AI 助手、经典/桌面/登录外观同步；站点品牌；安装更新与内置脚本兼容。
- 变更面：展示、只读分享、面板配置持久化、远程备份协议、通知、桌面交互、镜像内脚本及其更新联动。没有新增数据库迁移；没有新增 Agent 宿主机权限。
- 受影响用户旅程：远程/定时备份及取回；集群用量、价格、到期提醒和公开主题；经典模式/桌面/登录外观加载与同步；站点图标设置；AI 会话和终端交互；稳定/预览更新选择。
- 未变化契约：既有面板数据目录与备份加密包/恢复确认；Agent 权限模型；安装和更新继续使用官方唯一镜像 digest 校验。kejilion.sh 有耦合升级，见跨仓库章节；应用市场安装配置无需改动。
- 风险等级及理由：L3。版本、Docker 镜像和更新通道变化；含远程存储配置、持久化与稳定版默认通道提升，需全量构建、生命周期、镜像与发布门禁。

## 发布范围与未纳入内容

- 用户可见更新：见 CHANGELOG.md [1.23.0]。RC.1–RC.8 汇总：S3 兼容存储/WebDAV 与定时备份；集群资料、月度流量和剩余价值、公开主题及滚轮缩放；到期/服务状态提醒和通知历史；外观跨设备同步、登录和经典模式壁纸修复及站点品牌；桌面快捷键、终端尺寸同步、MCP 版本查询和 AI 会话修复。
- 精确提交清单：v1.22.0..v1.23.0 精确祖先区间，共 185 个提交；差异 306 个文件（20,341 additions / 1,519 deletions）。提交和文件清单可用 git log v1.22.0..v1.23.0 与 git diff --stat v1.22.0..v1.23.0 重建。稳定准备提交为 b73d6262，在 RC.8 验收与两项治理延期提交后设置正式版本号并更新变更记录。
- 明确未纳入的分支、文件或后续事项：真实生产主机升级；浏览器 UI 实机验收；真实 S3/WebDAV 服务和 NAS 联调；本记录的独立文档提交不属于 v1.23.0 稳定标签内容。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：CF security-boundary run-11 的源码基线为 9f933b682255f536fc1bc61670dc7238195740e0、tree 3d659bcb9ac5f39542d6d05b97e486acd1017d45；目标范围 40 commits / 75 paths。run-11 和 run-12 均为 incomplete / scope_complete=false；run-12 没有源码复核。证据位于 C:/GitHub/_validation/kpanel-v123-stable-security-run-11。
- 覆盖检查（check-security-audit-coverage.mjs 的 decision、未审计提交数；RC 只记录。稳定版带 --require，非 ok 时写补审 run，或用户原话、理由与不超过 14 天的补审截止日）：在精确稳定候选上 decision=scoped-required，40 个提交 / 75 个路径未由 CF 账本完整覆盖，新增边界为 internal/backupremote。严格准入不是 ok。本次按用户原话“请完成稳定版上线流程”记为用户豁免并继续发布产物；原因是平台连续拒绝创建 fresh coverage critic 与独立审查代理。此豁免不等于审计通过，补审责任人为本次发布负责人，截止 2026-10-10；退出条件为按仓库要求完成 fresh gpt-6-luna/max critic 和独立验证、双 pinned validators 通过且 --require 为 ok。
- finding fingerprint / 修复 commit / 独立复核与回归证据：无已确认 finding；run-11 一项仍为 needs_validation 且无 severity，另有五项 fingerprint 和两项范围归属问题未终审。不得据此推断不存在安全风险。未把未审 findings 标成修复或通过。
- 修复交付状态：源码 / RC / 稳定版 / 部署分别记录；stable tag 包含修复与公开 Release 的证据：稳定标签和公开 Release 对应 b73d62628ed77e12ce03715fafc1aa490b884f96；本次没有生产部署。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：从 v1.22.0 至稳定候选前的 72 条相关 trailer 记录中，38 条 numeric 有效、34 条 unreported、抽查成立的新增 constrained-only 为 0。72 是带 trailer 的记录数，可能含跟进/重述；适用候选分母未完全分类，不能据此计算代码覆盖率。
- 按 PROJECT_RULES.md 5.4/5.5 记录本稳定周期的观察结果；不足三个周期不提前宣称工具有效或应退出：本周期保留上述统计和口径限制，不据单次观察推断工具有效性。严格治理检查通过；两项提案延期至 2026-10-07，无追溯验收。延期提交 79c4acae85a864934044fc9826915fdf5b8cc43d。
- 候选分支的原始 tip、归档位置和远端核验统一填写本模板既有候选处置字段，遵守 docs/release-channels.md：见本文开头。

## 跨仓库联动判定

- scriptLinkageState：coupled
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：ai-cli-20260929
- KPanel 实际内置脚本基线 commit / SHA-256：镜像内置脚本 commit 779192048077c130442a64a126d7c0050776d868；SHA-256 33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：kejilion/sh main commit 779192048077c130442a64a126d7c0050776d868；根脚本 SHA-256 33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04；中文脚本 SHA-256 b40355fddfb6c781f6b52cb421f0a30686dce38637e04b5e5ace959b6e1293c1。
- 状态判定依据与兼容性证据：兼容 Claude Code / Codex 的脚本先行发布；公开 Release 镜像脚本标签与摘要已核对，L3 受管脚本契约检查通过。
- 本版发布决定：脚本先行后发布 KPanel；AI CLI 更新随 KPanel 稳定版提供。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用。
- packaging/kejilion-app/kpanel.conf 与 kejilion/apps main blob 均为 fa4b95374ed3b186d920207537f451a5b6d839f7（后者 main commit 4fc985e964dbd1742143521d0a28ab652b885773）；完全一致，无需应用市场仓库写入。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 候选/主线 CI、L3 全套自动测试和公开 amd64 镜像 API/E2E 通过。 | 未完成真实双设备、S3/WebDAV 服务联调和浏览器 UI E2E。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | govulncheck 无可达漏洞，npm audit 0，Trivy 源码/配置与镜像扫描通过，Release provenance/SBOM 门禁成功。 | CF 审计不完整，见外部审计与修复交付；扫描结果不替代人工边界审计。 |
| 稳定性、失败恢复与兼容 | 已验证 | 完整 Go tests、竞态测试、应用生命周期/备份一致性测试、公开镜像 E2E 通过。 | 本地 WSL AMD64 与自动环境结果不代表所有生产主机组合。 |
| 性能与资源预算 | 已实现未实机验证 | CI/L3 的 build、race 和资源边界检查通过。 | 本轮没有登记硬件上的低配性能/长时 soak 数据。 |
| 用户体验与可访问性 | 已实现未实机验证 | RC8 精确候选 8 项 mock-ui 组合检查、键盘/窄屏/i18n/CSS zoom 检查通过。 | stable 公共镜像未运行浏览器 UI E2E；CSS zoom 不等于原生浏览器缩放，PWA 未验收。 |
| 数据、配置与迁移 | 已实现未实机验证 | 自动备份/恢复及生命周期测试通过；无新增数据库迁移；kpanel.conf 跨仓库 blob 一致。 | 真实远程存储和现存生产实例升级未实测。 |

## 自动门禁

- 定向测试及结果：候选 CI 与 L3 覆盖完整 Go、前端 typecheck、215 个前端文件/1940 项测试、i18n、构建、竞态、漏洞扫描、场景包可复现性、amd64/arm64 构建及应用配置生命周期测试，均通过。公开 amd64 镜像标准 packaging/tests/image-e2e.sh 通过。
- make verify-release 环境和结果：固定 Runner 在登记的 local-wsl-dr 以 candidate-validation 用途执行，run v1.23.0-b73d6262-l3-r1 通过，exit 0。Runner kpanel-release-gate:go1.26.7-node24 image ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d。证据：C:/GitHub/_validation/v1.23.0-b73d6262-l3-r1。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run v1.23.0-b73d6262-l3-r1；UTC 2026-09-30T14:28:53Z 至 2026-09-30T14:37:48Z；runner ID 如上；plan 867ee092f49b4e8b5e8ed78b1c0975c7224a2e99702731a37a8b576b18ced516；远程脚本 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979；bundle 84adf5a14cdac7c4f043c5ed1f6e67e6271baaaf9d5e7e67538339b63fd6311c；status passed。
- 候选 CI：36730758251 成功；Dependency freshness 36730758321 成功；均对应候选 b73d6262。
- 主线 CI：36731880366 成功；Dependency freshness 36731880341 成功；均对应主线 b73d6262。
- Release workflow：36732758886 成功；标签 Dependency freshness 36732759086 成功。
- 安全扫描、镜像契约、SBOM/provenance：L3 与 Release workflow 中的 Trivy、镜像契约、Buildx provenance/SBOM 成功；这不是 CF scoped 审计的替代证据。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：本机 Windows 报告只成功 5/10 个源，Go 不在该环境；固定 Linux 上游 Go 查询超过 15 分钟未完成，因此没有将其报告称为完整。冻结候选、主线与 Release 三项 Dependency freshness 门禁均通过，范围以这些工作流实际检查为限。
- 最近每日安全通告审计、EOL 复核状态及证据：依赖新鲜度 CI 成功；本地完整日常公告/EOL 全源报告未取得，不声称已完成额外独立全源复核。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：两个治理提案按延期提交 79c4acae85a864934044fc9826915fdf5b8cc43d 保留待审，期限 2026-10-07；没有将未完成的上游报告信号记为无风险。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：github.com/minio/minio-go/v7 v7.3.0、golang.org/x/sync v0.23.0；前端 markdown-it 15.0.2、锁定 brace-expansion 2.1.7 / undici 8.11.2；Go 1.26.7、Node 24.20.0；受管脚本见跨仓库章节。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：依赖锁文件与 Docker/Action pin 随提交固定；脚本精确 commit/摘要见跨仓库章节；镜像摘要见公开产物章节。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：上游全源 dependency report 未完成，不把缺失结果视为拒绝或通过；由发布负责人在下一次完整来源可用时复核，沿用 2026-10-07 治理提案期限。
- 升级后的兼容、安全、构建、性能资源和回滚结论：Go/前端与双架构镜像构建通过；govulncheck/npm audit/Trivy 通过。未做真实生产升级，实际生产兼容性仍待部署窗口验证。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：本地 WSL2 Ubuntu，AMD64，Docker 29.6.2；固定 L3 runner kpanel-release-gate:go1.26.7-node24。
- 环境策略 ID 与允许用途：local-wsl-dr，仅 candidate-validation；arena-154 SSH 超时。未将本地环境声明为 browser-validation、host-failure 或 production-safety 环境。
- 使用的精确候选或公开产物：公开 docker.io/kjlion/kejilion-panel:1.23.0；candidate L3 SHA b73d62628ed77e12ce03715fafc1aa490b884f96。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：公开 AMD64 镜像 pull 与标准镜像 E2E 通过，image_e2e=pass、exit 0；证据 C:/GitHub/_validation/v1.23.0-stable-release-verification；L3 作业信息见自动门禁。E2E 临时容器、网络和数据已清理。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：无长时 soak；本次稳定产物流程执行单次完整 L3 和公开镜像生命周期 E2E。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：RC8 有 8 项 mock-ui 组合证据及 CSS zoom/i18n/键盘检查；该证据不是稳定公开镜像浏览器实测。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 的应用配置/备份一致性生命周期测试通过。没有对生产主机写入。
- 未执行场景及原因：真实浏览器 UI E2E、Nginx/PWA、原生浏览器 200% 缩放及登记主机故障恢复未执行；local-wsl-dr 不允许 browser-validation，arena-154 不可达。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：v1.23.0 Release 于 2026-09-30T15:06:57Z 发布；draft=false、prerelease=false、GitHub Latest=v1.23.0。注释标签对象 a7fd94ece39875b767181495c7d84504e7a4cd9b，peeled commit 为 b73d62628ed77e12ce03715fafc1aa490b884f96。
- Docker 版本与通道 OCI index：1.23.0 与稳定通道 latest 同为 sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5；preview 保持 RC.8 的 sha256:7ce0d347d5f1672a95c25f2caa969e7562ee94685ff4f5e2c51410699d3a5ef8。
- linux/amd64、linux/arm64 digest：amd64 sha256:b23db0b1bc757514da80f0959818cd622b906241de108073388baf90cf0df832；arm64 sha256:8efcc427b6085c27c99f79853bb66c5fcea0fefe4afa710e2ba18e5277828965。OCI index 还含绑定两架构的 provenance attestation manifests。
- 附件及 SHA256SUMS：Release API 共 14 个附件；SHA256SUMS 11 条逐一匹配实际下载文件及 API digest。文件 SHA-256 8e8de7fba4ccfb8d3bba149978d210dd1fe20a42b3190e631ef70e6811c922b4。未将 11 条表述为全部 14 个附件均有此清单校验。
- 公开镜像 image_e2e=pass：Docker Hub AMD64 公开镜像通过标准 packaging/tests/image-e2e.sh；公开 OCI API/容器生命周期断言通过。
- kejilion/apps / kejilion.sh 契约结论：应用市场配置 blob 与 KPanel 内置版本相同，无仓库写入。脚本联动已先行发布并核对镜像标签和脚本摘要；详情见跨仓库章节。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：既有自动化契约测试通过；正式 Latest 与 stable/latest digest 已核对。
- 加入预览只切换来源并立即检查，没有自动安装：既有契约测试通过。
- 自动安装开关与一次性立即安装相互独立：既有契约测试通过。
- 旧状态默认迁移到 stable，重启后通道选择保持：既有契约测试通过。
- 退出预览且稳定版较低时没有产生降级候选：既有契约测试通过。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：由现有单元/镜像测试覆盖；未在真实宿主机重演。
- OpenRC 与轻量 Node 的当前边界已按 docs/release-channels.md 明确呈现：文档与自动化契约通过。

## 生产部署安全核对

- 生产目标和部署授权范围：本次授权完成稳定版正式产物发布；没有生产服务器部署授权。
- 验证/灰度环境（必须来自 environment-policy.json，不得包含 prod-108）：登记 local-wsl-dr 仅用于 candidate-validation，不是灰度环境。
- 正式部署环境（默认 arena-154；不得包含 prod-108）：arena-154 SSH 超时，本轮未连接部署。
- prod-108：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对：全部未操作。
- 部署前版本、健康、备份位置及摘要：不适用（无生产部署）。
- 部署命令/入口：不适用（无生产部署）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：未执行生产核对。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3 candidate-validation 与公开 AMD64 容器 E2E。

## 回滚

- 源码/tag：v1.22.0，源码 cea6261f9ae1de064216a1c423d3cd3f2d87d3fa。
- 镜像 digest：Docker latest 当前为 v1.23.0 index；上一稳定镜像回滚点 sha256:46b854bdf6233e81742986a3bee9e65c6b4a39123c393a51a4ada63783eed7ac。
- 数据/配置备份：无生产写入，未创建生产备份。
- 回滚步骤和回滚后复核：若需撤回公开 stable 发布，由发布负责人按项目 Release/通道流程将默认标签与 GitHub Latest 恢复至已验证的 v1.22.0 digest 后复核；实例降级前须单独备份并核对兼容性。
- 回滚后生产实际版本与健康状态：不适用，本次未部署/回滚生产。
- GitHub Latest、Docker latest 与标准更新入口实际指向：本次发布后指向 v1.23.0；Docker preview 仍为 rc.8。
- 公共默认更新通道决策：不适用（无已知需撤回的问题）；正式稳定通道现指向 v1.23.0。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-27T10:35:52+08:00
- 候选冻结时间：2026-09-30T22:28:53+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：24
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记隔离环境 SSH 超时。用户已明确选择本地通道；公开镜像浏览器只能作为补充证据，实机项目仍未验证。",
    "recoveryEvidence": "本轮 arena-154 有界 SSH 输出；后续 local-wsl-dr L3 与正式公开镜像补充验收分别记录，不把它们改称登记实机通过。",
    "permanentAction": "属于上游环境可达性例外。v1.23.0 发布负责人于 2026-10-03 前复核 SSH 与登记 browser-validation；退出条件是登记隔离环境完成同一公开镜像旅程。此前不得扩大本地通道权限。",
    "historicalReleases": [
      "v1.22.0"
    ]
  },
  {
    "fingerprint": "governance-health/strict-preflight/expired-proposal",
    "position": "before-production-write",
    "count": 1,
    "impact": "严格检查发现两个旧提案超过复核 SLA；冻结前未通过。没有将未归集的原始实现证据补写为通过。",
    "recoveryEvidence": "79c4acae85a864934044fc9826915fdf5b8cc43d 按原规则只补真实延期状态；governance-health.txt 显示 overdue=0、deferred_valid=2。",
    "permanentAction": "两个提案保持待复核，负责人为 v1.23.0 发布负责人；2026-10-07 前归集原实现同 SHA CI 和非作者复核，形成通过或拒绝结论，逾期继续由 strict 门禁拦截。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-linkage/remote-read/noncanonical-https",
    "position": "before-production-write",
    "count": 1,
    "impact": "脚本仓库原 HTTPS 远端读取超时，不能证明公开分支状态。",
    "recoveryEvidence": "随后使用 SSH 精确核对 kejilion/sh main=779192048077c130442a64a126d7c0050776d868；没有修改脚本用户工作树。",
    "permanentAction": "冻结执行方案明确只读远端使用 SSH git@github.com:kejilion/sh.git，不再以本地既有 HTTPS transport 作为发布事实来源。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-linkage/ssh-remote/missing-identity-adapter",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 SSH 读取未带本仓库既有身份适配，返回 publickey 拒绝；未写远端。",
    "recoveryEvidence": "复用 KPanel core.sshCommand、通过 git -c 显式传递后 ls-remote 取得上述精确公开 tip；不打印 SSH 凭据或改用户仓库配置。",
    "permanentAction": "成对脚本核对的冻结命令固定使用已预检的 KPanel SSH 身份适配，兼容版本和 SHA-256 均另外由精确提交 blob 复核。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-metrics/report-release-metrics/unsupported-flag",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次质量指标读取使用不支持的 --json 参数，被入口拒绝，没有产生有效报告。",
    "recoveryEvidence": "修正为权威 --format json --ref 9f933b6，release-metrics.json 保留真实生成时间和 source_ref。",
    "permanentAction": "本轮执行方案和验收记录只使用仓库 parseArguments 支持的 --format 与 --validate-acceptance；不以失败调用宣称指标完成。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-linkage/run-repo-bash/unsupported-flag",
    "position": "before-production-write",
    "count": 1,
    "impact": "Bash launcher 调用误带 --repo，在脚本启动前被拒绝；整份宿主机管理脚本未执行。",
    "recoveryEvidence": "本轮拒绝输出；后续 script-smoke.log 只执行 bash -n 与隔离的 AI CLI/dispatch 模拟夹具，script_paired_smoke=pass。",
    "permanentAction": "脚本核对固定为精确 git blob 的语法检查与网络禁用夹具，不再以整份管理脚本的 --help 或 launcher 未支持参数进行验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-smoke/isolated-fixture/noexec-scratch",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 mock CLI 位于默认 noexec tmpfs，夹具返回 Permission denied；这不代表发布脚本逻辑失败。",
    "recoveryEvidence": "首轮 script-smoke-r1.log 保留；显式允许该临时模拟 CLI 目录执行，保留网络禁用、只读根目录和资源限额，script-smoke.log 通过。",
    "permanentAction": "已预检的配对脚本烟测命令显式声明 exec scratch，继续使用固定 Runner ID 和 network none；生产路径从未写入。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependency-report/windows/unqualified-local-runtime",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 报告退出 2，只完成 5/10 源；缺 Go、网络检测失败，且本地安装图落后于候选锁文件，不能作为完整新鲜度证据。",
    "recoveryEvidence": "dependency-freshness.json 与 summary 保留原始缺口，未设置 allow-partial 改成通过；候选 CI 必须用 Linux/新锁图完成全部检测。",
    "permanentAction": "冻结方案把完整检测限定为固定 Linux 工具链和候选锁图，Windows 缺口只作失败证据；每日漏洞任务与新鲜度报告分开记录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependency-report/fixed-runner/unbounded-upstream-query",
    "position": "before-production-write",
    "count": 1,
    "impact": "固定 Runner 已按精确源码新装锁图，但 go list -m -u 长时间无终态，超过 15 分钟后停止自己的临时容器。具体外联故障未确认，未生成完整报告。",
    "recoveryEvidence": "dependency-report-runner.log、dependency-report-runner-status.json 和 exit 137；停止前复核容器脚本参数、不可变镜像及唯一 evidence mount。没有使用此结果放行。",
    "permanentAction": "本地入口已加 timeout 900s；完整新鲜度仍要求候选 CI 的同 SHA report job 成功，其 workflow 外层 timeout-minutes=20。v1.23.0 发布负责人在 2026-10-03 前复核本地查询渠道，退出条件为完整 10/10 检测源报告；无完整报告时保持失败状态。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/cluster-hunter/wrong-comparison-base",
    "position": "before-production-write",
    "count": 1,
    "impact": "cluster hunter 中间分析误用 HEAD~40 作为比较基线；该结论未计入覆盖或发布准入，没有改变候选源码。",
    "recoveryEvidence": "原件保存在 kpanel-v123-stable-security-run-11/agents/hunter_cluster_auth/artifacts/wrong-base-comparison-observation.json；精确 comparison_base 的 structured hunter 结果已单独保存，初始错误结果未纳入覆盖。候选发现的独立验证仍待完成。",
    "permanentAction": "后续 assignment 固定核对 source/tree 与 checker comparison_base=4c0694aa8e02e46145a775707b8d5a0355f7ce10，逐单元完成精确差异追踪后才关闭。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/collaboration/agent-thread-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一平台限制批次：run-11的audit critic与root独立尝试、run-12的fresh critic两次尝试均被平台以agent thread limit reached拒绝。完整审计不能完成。仅在记录期限例外后继续产物流程，不将incomplete审计写成通过。",
    "recoveryEvidence": "run-11 agents/ledger_ingest/coverage-critic-wave2-spawn-failure.txt 及本会话root spawn拒绝；run-12 run-metadata.json与REPORT.md记录两次新critic在启动前失败。代理从未作为validator执行；没有复用recon、hunter或旧critic。按PROJECT_RULES.md 5.4期限例外推进剩余发布门禁。",
    "permanentAction": "发布负责人于2026-10-10前复核平台并完成scoped run：fresh critic确认余下scope paths，候选指纹经独立Phase3/Phase5和final critic，scope_complete=true、双pinned validators通过且check-security-audit-coverage --require为ok。未恢复则保持未补审并到期复核；不得沿用本次例外。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/unowned-reviewed-paths",
    "position": "before-production-write",
    "count": 1,
    "impact": "集群指标结果的4个top-level reviewed_paths没有对应check owner，pinned ledger validator 首次拒绝；不作为覆盖通过证据。",
    "recoveryEvidence": "原始 hunter JSON 与 metadata anomaly 已保留；fresh hunter_telemetry_wave2 在 agents/hunter_telemetry_wave2/artifacts/structured-result.json 为原4路径补齐逐单元 check owner，Ubuntu/root 两项 pinned validators PASS 33 units/1 finding。集群偏好单元 covered，轻量节点单元因历史身份/回滚线索另处 blocked，仍待精确范围复审；不把结构通过当完整审计。",
    "permanentAction": "后续hunter交付时逐单元核对reviewed_paths与checks-owned path一致；critic核对原4路径，范围内缺证据的路径补检查或单元后才可scope_complete。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/validator-launcher/unavailable-wsl-distro",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 pinned validator 调用使用未安装 Ubuntu-24.04，在 validator 执行前被 WSL 拒绝；未形成通过证据。",
    "recoveryEvidence": "agents/ledger_ingest/artifacts/validator-distro-first-failure.txt 保留首次输出；修正 Ubuntu/root 后 validator-wave2-preflight.txt 显示 ledger PASS 33 units、findings PASS 1 finding。",
    "permanentAction": "固定使用已预检的 Ubuntu/root 与 pinned validator 绝对路径；不安装新发行版，不把结构验证成功等同于完整审计覆盖。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/malformed-notification-check-paths",
    "position": "before-production-write",
    "count": 1,
    "impact": "通知 hunter 的 outbox check 使用点分隔组合路径且 check union 与单元 reviewed_paths 不一致；该单元和原始候选未纳入覆盖或发布发现。",
    "recoveryEvidence": "wave2-notifications-raw-result.json 是当时的转录副本（含父任务漏抄）；reconstructed-from-original-tool-result.json 仅补回原工具返回已有的 service_checks.go，并保留 provenance manifest。原owner的结构补交只改8处路径标签，14条unit/check路径并集一致；两项 pinned validators PASS coverage33/findings2。候选仍待独立Phase3/5，没有确认漏洞。",
    "permanentAction": "逐单元核对每个 check 的路径数组与真实源路径、check union 和 reviewed_paths；不能用修剪未归属路径冒充完整检查，缺证据时重新分配同固定配置 hunter。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/invalid-same-owner-reassignment",
    "position": "before-production-write",
    "count": 1,
    "impact": "父任务把原owner的结构纠正误记为重新分配，pinned coverage validator 拒绝多余attempt；未采用失败验证作为覆盖证据。",
    "recoveryEvidence": "agents/ledger_ingest/artifacts/validator-notification-first-attempt-failure.txt 保留首次错误；移除误登记attempt后 validator-notification-structural-correction.txt 为PASS，coverage33/findings2。原件、重建件和结构补交件分别保留。",
    "permanentAction": "原owner的格式纠正与跨owner reassignment 分别记录；共享ledger更新后用固定pinned validator核验身份、attempt与证据归属，禁止用人工修剪替代未完成源码检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/validator-launcher/duplicate-script-input-argument",
    "position": "before-production-write",
    "count": 1,
    "impact": "pinned ledger validator 命令误把 validator 本身路径重复为输入，导致读入自身源码并报 malformed JSON；不是对ledger的有效验证。",
    "recoveryEvidence": "agents/ledger_ingest/artifacts/validator-extra-argument-error.txt 是调用错误的纠正说明；使用单一coverage-ledger.json输入重跑 PASS33。原始工具拒绝不作ledger失败或通过证据，没有执行target代码。",
    "permanentAction": "固定Ubuntu/root，validator程序路径与唯一JSON输入分别传递；校验实际argv，保留初次拒绝与正确调用结果，不现场猜测CLI参数。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/in-progress-unit-retained-evidence",
    "position": "before-production-write",
    "count": 1,
    "impact": "轻节点单元追加检查改为in_progress时仍带旧reviewed_paths/local_checks/unresolved，违反active单元应为空的pinned契约，被validator拒绝。",
    "recoveryEvidence": "validator-lightnode-inprogress-first-failure.txt 保留初次拒绝。active行已清空旧数组，旧source原件单独保存；上下文补交后17条路径/10项checks正式入账，Ubuntu/root coverage validator PASS33。当前unit因V2历史状态条件仍blocked，源证据完整性与范围归属由critic判断。",
    "permanentAction": "旧结果单独归档，active assignment保留空证据数组；完成后逐项合并实际checks与source provenance，再运行pinned validator。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/findings/invalid-multientry-trace-and-order",
    "position": "before-production-write",
    "count": 1,
    "impact": "cluster Phase3导入时trace中间项被标为第二个entrypoint，findings数组也未按fingerprint排序；pinned findings validator拒绝该版本。",
    "recoveryEvidence": "原Phase3保留在 agents/verify_cluster_delete_shadow_v123/artifacts/phase3-result.json；初次拒绝保留 validator-cluster-phase3-first-failure.txt。独立verifier的结构重交只修改trace[5].kind，父任务按fingerprint排序；validator-cluster-structure-correction.txt 为 findings PASS3。结论仍needs_validation，无confirmed或源码修改。",
    "permanentAction": "保持跨请求步骤的真实条件描述，trace仅首项entrypoint、中间propagation、末项sink；协调器稳定排序findings，先验证再纳入正式记录，结构重交与原件分开。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/ledger-ingest/powershell-array-parser-error",
    "position": "before-production-write",
    "count": 1,
    "impact": "scene ledger ingest 的手写PowerShell数组在解析阶段失败，未修改ledger、metadata或候选源码。",
    "recoveryEvidence": "agents/ledger_ingest/scene-ingest-first-parser-failure.txt 保留拒绝；原scene-wave2-result.json随后作为JSON数据解析，三个covered单元正式入账。validator-scene-wave2-ingest.txt PASS33，validator-scene-wave2-findings.txt PASS5。",
    "permanentAction": "长结构化返回按JSON数据保存与解析，避免嵌成长PowerShell代码数组；共享账本写回后只调用固定Ubuntu/root pinned validators，保留写前解析拒绝。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/supplement-overwrote-owner-checks",
    "position": "before-production-write",
    "count": 1,
    "impact": "同owner上下文supplement替换了原checks，丢失已真实检查的monitoring.go归属；逐路径差集发现该缺口，run仍未完成，未用于发布准入。",
    "recoveryEvidence": "未改动 agents/hunter_telemetry_wave2/artifacts/structured-result.json 的首项check包含monitoring.go。该check已原样提取、追加并重算并集，独立restoration provenance保留；pinned coverage PASS33/findings PASS5。75路径剩余owner缺口为未交付release四路径及另两个待范围复审路径。",
    "permanentAction": "同SHA补充检查采用可追踪合并，不以新结果直接覆盖旧checks；原结果独立保存，完成后对全部scope路径与check-owner union做差集核对，缺口必须补查或有源证据的N/A。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/terminal-metadata/missing-object-property",
    "position": "before-production-write",
    "count": 1,
    "impact": "审计终态元数据首次写入使用不存在的PSCustomObject incomplete_reason属性而中断；findings筛选已完成、run_status已写incomplete，但reason与terminal event该次未写完。没有改动候选源码或发布远端。",
    "recoveryEvidence": "首次拒绝转录保存在 kpanel-v123-stable-security-run-11/agents/ledger_ingest/incomplete-reason-property-first-write-failure.txt；使用Add-Member NoteProperty补齐属性后完成incomplete元数据与报告。发布root已实际核对reason、scope_complete=false以及最终 validator-final-coverage.txt PASS33、validator-final-findings.txt PASS1。结构通过不算完整覆盖通过；候选源码与远端未写入。",
    "permanentAction": "终态元数据写入前使用显式字段存在检查或Add-Member -Force构造声明字段；更新后读回metadata并运行两项固定validators。原始字段失败保留，不把结构修正改成源码或完整审计通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/validator-launcher/windows-unsupported-input-protection",
    "position": "before-production-write",
    "count": 1,
    "impact": "run-12 在 Windows native 调用 pinned validators 时，因当前 OS 不支持 no-follow 与 nonblocking 输入保护，在读取前拒绝；未形成有效验证。",
    "recoveryEvidence": "validators/coverage-ledger-windows-rejected.txt 与 findings-windows-rejected.txt 保留拒绝；固定 WSL Ubuntu、身份 root 验证分别 PASS: 0 coverage units、PASS: 0 findings。空数组通过只证明结构格式，不证明任何覆盖。",
    "permanentAction": "严格使用已验证的 WSL Ubuntu/root pinned validator 入口；发布预检先确认 Distro 和有效身份，并核对输入 ledger 非空且与真实范围相符。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-release-verification/powershell/missing-launcher",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次公开 Release 校验尝试引用不存在的 PowerShell 7 可执行路径，校验脚本未启动，后续公开验证尚未开始；未改动远端或镜像。",
    "recoveryEvidence": "从当前 PowerShell 直接执行固定 verify-stable-release.ps1 后通过，14 个 Release 附件中 11 个 SHA256SUMS 条目实际下载校验一致，GitHub Latest 为 v1.23.0；结果写入 C:/GitHub/_validation/v1.23.0-stable-release-verification/release-verification.json。",
    "permanentAction": "复用当前已验证的 PowerShell 执行入口直接调用冻结脚本；执行前检查脚本路径，不假设未安装的 PowerShell 7 固定路径。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-release-verification/powershell/stale-last-exit-code",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release 校验脚本已通过并写出报告，但包装器错误读取旧的 LASTEXITCODE，将成功误报失败并中止同一命令中的 Docker digest 步骤；没有重跑、覆盖或改变公开产物。",
    "recoveryEvidence": "核对已生成的 release-verification.json 为 11/11 校验通过后，独立运行固定 verify-stable-docker.sh；stable/latest index 相同、preview digest 保持、双架构标签及 OCI revision/脚本摘要核验通过。",
    "permanentAction": "PowerShell 脚本/函数使用其结果文件和成功状态核验；仅在紧邻的原生进程调用后读取 LASTEXITCODE，避免复用旧值；将多个必需验证命令分开执行。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 docs/project-management.md 13.1）：稳定候选 worktree/分支回收，C: 增加 315,437,056 可用字节；L3、公开 Release 校验及审计失败原件保留于 C:/GitHub/_validation。用户预览、未知归属和独立审计 worktree 未清理。
- 未验证风险：CF scoped 审计未完成；真实生产主机和浏览器 UI 未验收；真实 S3/WebDAV/NAS 未联调；稳定版在真实用户主机的升级/回滚未实测；本地 dependency report 与上游 Go 查询不完整。
- 已实现待实机准入：跨设备外观、公开登录页、远程备份、通知和集群分享等功能需真实环境旅程验收。
- 不阻断本版的理由：安全审计缺口按用户明确要求和 14 天期限例外披露，例外不代表门禁通过；本次仅发布正式产物，没有执行生产写操作。
- 后续应进入的自动门禁或专项工作流：最迟 2026-10-10 完成 CF 安全审计补审并使 check-security-audit-coverage --require 为 ok；按登记环境恢复后完成 browser-validation 与真实生产部署安全核对；补齐依赖上游完整报告。责任人：本次发布负责人。
