# KPanel v1.31.0-rc.5 发布验收记录

日期：2026-11-28

发布级别：L3

候选提交 / 标签：6f9417d4183400cda6f5bd73224430e8f570beb9 / v1.31.0-rc.5

上一稳定版本 / 回滚点：v1.30.0 / docker.io/kjlion/kejilion-panel@sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e

releaseChannel：preview

releaseTrain：1.31.0

候选分支与发布后处置：release/v1.31.0-candidate / 预览版保留

- 原分支 / 精确 tip / 处置分类：
  - release/v1.31.0-candidate：6f9417d4183400cda6f5bd73224430e8f570beb9；预览候选，按规则保留。
  - fix/docker-compose-json：813585f30f74b328008f0327a9047c77d16c4aa5；已纳入，来源任务分支暂时保留。
  - feat/compose-project-upgrade：295baf9e2b67f77eb27220710821d348a77de7af；已纳入，来源任务分支暂时保留。
  - fix/app-job-list-payload：001403944c4e4fb959a78ec046f9ed282cf83457；已纳入，来源任务分支暂时保留。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：本次不归档来源分支。已复核 origin/release/v1.31.0-candidate 指向候选 SHA；三条来源分支无同名 origin ref，远端 origin/main 在候选发布时指向该 SHA。
- 本次来源任务分支：三条来源分支纳入本版本；作者尚未释放所有权，保留其本地 refs 与工作树，不以候选分支替代或删除来源分支。
- 本地分支/upstream/worktree：候选分支保留在 C:\GitHub\_release-candidates\kpanel-v131，远端跟踪 origin/release/v1.31.0-candidate；来源分支及工作树分别保留在 C:\GitHub\_codex-tasks\kejilion-panel-compose-json、C:\GitHub\_codex-tasks\kpanel-compose-upgrade-42、C:\GitHub\_codex-tasks\kejilion-panel-issue-43。来源作者未确认释放前不回收。
- 未完成归档项 / 责任人 / 下次复核触发条件：来源分支的任务作者/所有者待确认；由下一轮 KPanel 候选盘点人在作者明确释放所有权后复核。此次不清理其他工作树。

归档不代表生产上线；本版产物已发布，生产未部署。

## 发布画像

- 业务域：Docker 管理、自更新任务记录与应用市场任务隔离。
- 变更面：展示、面板/Agent 应用接口、用户显式触发的宿主机 Docker 更新写入；无数据库迁移、端口变化或 kejilion.sh 协议/运行时动作变化。
- 受影响用户旅程：识别 Compose JSON 部署、从 Docker 页面菜单更新自部署 Compose 项目或独立容器、查看应用市场后台任务列表与终端日志。
- 未变化契约：未改变 Agent 协议版本 v1alpha1、外部端口、持久化数据结构、Compose 部署格式、kejilion.sh 内置脚本内容；预览通道仍需用户主动选择，稳定版仍是默认来源。
- 风险等级及理由：L3。候选包含宿主机 Docker 更新路径和应用市场任务边界处理，需按精确候选执行 L3 与公开产物验收。

## 发布范围与未纳入内容

- 用户可见更新：识别 Compose JSON；支持从 Docker 容器菜单更新自部署 Compose 项目和独立容器；Compose 更新定位所选服务并遵循 pull policy；应用任务列表不再依赖保留日志，并通过任务详情读取日志；修复后台任务历史信息脱敏和第三方应用选择器边界。
- 精确提交清单：候选范围为 d4899e7123e160cd9417f412145ab94ebbef4622..6f9417d4183400cda6f5bd73224430e8f570beb9，28 个文件，1197 insertions、71 deletions。核心来源提交：813585f、0014039、2510b33、ee9c140、8eb3a57、df60605、295baf9；集成与候选提交：3ea2529、204cedf、34c4230、3d4e30b；安全边界、pull policy、交互说明、日志脱敏和来源绑定提交：82c1912、e5c1d03、8def328、a401de8、48791ff、078d8a7、feb9bd5、eec8018、def8ca4；候选回归断言修复：b30245d、002ab8f、6f9417d。
- 明确未纳入的分支、文件或后续事项：其他未通过本次范围核对的分支未纳入；未进行稳定版发布、生产部署或来源任务工作树清理。scripts/mock-app-market-api.mjs 是验收预览夹具变更，已纳入 contract.allowedPaths。

## 外部审计与修复交付

- 安全盲审补漏（quality-audit review 档）：本次审查范围记录为 internal/appmarket；基线 v1.30.0；提交 trailer 记录 H0/M3/L1、confirmed=2、fixed=2、deferred=2。Deferred 项仍是公开预览的跟进风险，不代表安全问题已清零。
- node scripts/security-review-scope.mjs --base v1.30.0 输出：未单独保存该命令的终端输出；本次使用候选提交审查 trailer、来源清单与候选/发布 CI 扫描记录核对。最终候选的 GitHub-hosted CI 和公开镜像验收均成功。
- finding fingerprint / 修复 commit / 独立复核与回归证据：完整 finding 文本位于外部审查快照 C:\GitHub\_release-evidence\kpanel-v131-rc5-20261128\security-appmarket.tar；聚合审查记录在候选提交 def8ca481049b5732ace9d7f6b6f173a64538ca2。报告计 2 项已确认修复、2 项 deferred；当前验收记录未将 deferred 项表述为已解决。独立复核 trailer 为 PASS WITH FOLLOW-UP，记录的 provider fallback 原因为 Claude CLI 未登录。
- 修复交付状态：应用市场任务边界修复进入候选 SHA、预览 RC、公开 Release 与不可变镜像；没有稳定 tag 或生产部署。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：候选审查 trailer 记录 d4899e7..eec8018，23/23 文件、free-form=0、valid H0/M0/L0、constrained-only=unreported；其后两个测试调整分别记录 2/2 文件，末次编译修复也记录 2/2。此数据不用于宣称 OCR 工具有效或应退出。
- 按 PROJECT_RULES.md 5.5 记录本稳定周期的 OCR 观察结果：本版为预览版，不据此提前评估工具效果或退出。
- 候选分支的原始 tip、归档位置和远端核验：见本记录开头候选分支处置字段；候选预览分支保留，来源任务 refs 与工作树未删除。

## 跨仓库联动判定

- scriptLinkageState：not-required
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：1faee13f3faef164efbfa899e244fc19ad58aaff / 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：本次未改 kejilion.sh 的协议、运行时动作、宿主机产物或内嵌脚本；contract.json 记录上述基线身份。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 候选 CI、L3 的 Go/web/deploy lanes 和公开面板升级验收均通过；精确候选 SHA 与公开镜像身份匹配。 | 未用真实混合版本 Agent/Panel 主机验证 Docker 更新。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 候选及主线 CI、安全扫描、发布镜像身份与 checksum/provenance 核对通过；应用市场审查记录 2 项修复、2 项 deferred。 | deferred 项仍需跟进；CI 之外的真实生产网络环境未验证。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 单元/集成 CI 与浏览器验收通过，失败断言在候选 CI 中修正后最终候选通过。 | 未在真实宿主机执行 Docker 更新、失败回滚、systemd 重启恢复或卷恢复。 |
| 性能与资源预算 | 不适用 | 本次未新增性能目标或性能门槛；L3 运行资源见运行记录。 | 没有 soak 或生产负载数据，不能据此推断宿主机容量。 |
| 用户体验与可访问性 | 已实现未实机验证 | 精确公开镜像浏览器升级验收 31 checks 通过；视口 1280/390/320 无横向溢出、page errors=0；本地 mock 预览覆盖 5 个升级旅程。 | 未完成 200% 缩放、全键盘/焦点、主题组合的人工实机验收。 |
| 数据、配置与迁移 | 已实现未实机验证 | 无数据库 schema migration；Compose 服务选择与 pull policy 有自动化测试。 | 未在真实主机验证卷内容、宿主机 Docker 状态与升级后真实恢复。 |

## 自动门禁

- 定向测试及结果：候选 CI 和 L3 覆盖 Go、web、部署脚本及发布工作流检查；最终精确候选全部通过。此处不将早期断言/编译失败计作发布流程异常，详见后文。
- make verify-release 环境和结果：未在本机单独运行；精确候选的 GitHub-hosted L3 和 Preview Release 工作流通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run 46144844721 attempt 1，终态 passed、exit 0；receipt entrySha256=8252bd7faa9da4a455bb83d3c856aa8190adc079a08ca3cb4978532022d74138；l3.log SHA-256=72304cdede8ca90af8ab74c32f1e6328acc1ade32e587d122307cc7a031409ec。Runner 为 GitHub-hosted ubuntu26 image version 20261122.9.1；receipt 未分别记录 plan/bundle SHA 或不可变 Runner ID。证据目录 C:\GitHub\_release-evidence\kpanel-v131-rc5-20261128\l3\l3-46144844721-1；receipt 时间 2026-11-28T12:29:32Z 至 12:41:41Z。
- 候选 CI：run 46143671156 success，精确 SHA 6f9417d4183400cda6f5bd73224430e8f570beb9；候选 Linux/Windows CI 通过。
- 主线 CI：Preview Release run 46144844721 内 promotion 与 main Linux/Windows CI 均通过；origin/main 在发布提升后指向候选 SHA。
- 主线后续记录：验收记录作为单独文档提交追加到 main；不会移动预览 tag、镜像或 release/v1.31.0-candidate 的候选 SHA。
- Release workflow：run 46144844721 success；版本 tag、Windows 发布资产、GitHub Release 与预览 OCI 镜像发布成功；public acceptance 和 preview completion 成功。独立 L3 已通过，因此 publish/l3 跳过；稳定版 completion 跳过符合 preview 流程。
- 发布说明门禁：最终正文通过；版本 v1.31.0-rc.5，摘要 5 条、升级提示 3 条。
- 安全扫描、镜像契约、SBOM/provenance：候选/主线 CI 扫描及公开镜像契约通过；公开验收核对了 exact OCI digest、平台子 manifest、修改组件清单与 SHA256SUMS。modified-components 清单 SHA-256=43742040f7d10994e4f680fe348cfbc28aa52ecac6df34453c7b77e68e6d3bae；单独 SBOM 文件未在本记录中复核。
- 候选 CI 失败与恢复：run 46139312349（2026-11-28 19:19:12+08）发现应用任务脱敏断言预期过时，b30245d（19:30:04+08）更新后进入下一轮；run 46140953578（19:40:06+08）发现任务 marker 断言过窄，002ab8f（19:48:16+08）修正后进入下一轮；run 46142326276（19:57:58+08）发现 Go 测试重复短变量声明，6f9417d（20:06:24+08）修复，run 46143671156 于 20:27:56+08 通过。三次均在预览发布前由候选 CI 拦截，未逃逸门禁。
- 交付 handoff：首次检查因 contract.allowedPaths 漏列验收 mock 文件 scripts/mock-app-market-api.mjs 而失败；发布与公开验收已成功后，将这一精确文件加入 contract.allowedPaths（未扩大 forbiddenPaths、未改候选），同一候选的 handoff 随后通过。计入一次发布流程证据拦截，详见流程异常明细。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：未单独生成报告。
- 最近每日安全通告审计、EOL 复核状态及证据：未在本次预览流程中单独记录每日通告/EOL 复核；候选 CI 安全扫描通过。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：未单独复核。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：应用代码与版本号从 rc.4 升至 rc.5；无新增 npm 依赖。L3 使用 Go 1.27.4、Node 26.13.1、Docker 29.4.2 与 ubuntu26 image version 20261122.9.1。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：VERSION、Go 与 web package version 更新为 1.31.0-rc.5；web package-lock 只更新版本字段。基础镜像/Action 没有随本次候选变更。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：未记录新的依赖候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：构建与公开镜像身份通过；没有实机性能或宿主机兼容性结论；回滚锚点为 v1.30.0 稳定镜像。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted ubuntu26 和隔离容器；不是 arena-154 或生产主机。
- 环境策略 ID 与允许用途：GitHub-hosted L3 与 workflow public acceptance；没有连接 arena-154。
- 使用的精确候选或公开产物：6f9417d4183400cda6f5bd73224430e8f570beb9；公开镜像 docker.io/kjlion/kejilion-panel@sha256:ffb57b615fec667f101e0af5aa77c08a4b6f99995f3a7e78c2b8af2ff870cbeb。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run 46144844721 attempt 1 passed、exit 0；超时预算 2700000 ms；receipt 与各 lane 结果在 C:\GitHub\_release-evidence\kpanel-v131-rc5-20261128\l3\l3-46144844721-1；外层 entry SHA-256 见“自动门禁”。公开验收 artifacts 由同一 workflow run 关联。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：没有 soak；本次预览验收为定向升级与公开浏览器检查，不作为生产负载认证。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：公开浏览器覆盖升级弹窗/通道状态，视口 1280、390、320，31 checks 通过，无横向溢出和页面错误；本地 mock 覆盖 5 条升级旅程。200% 缩放、完整键盘/焦点与主题矩阵未验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离公开验收没有连接宿主机 systemd 或 Docker socket；公开面板测试使用 dockerInventoryFixture=true 和 testCATrustFixture=true，故不能代表真实宿主机升级/恢复。mock 失败流程有自动化检查。
- 未执行场景及原因：arena-154 服务端尚未安装共享 SSH 公钥，本次未连接；无真实 Docker 主机写入、系统服务重启、卷恢复或长时间 soak。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：公开验收使用精确发布产物；版本、digest 匹配，摘要与升级提示非空并显示；31 checks、mocked=false、status=passed、errors 为空。隔离容器 browser/upgrade 报告不是生产宿主机证据。
- GitHub Release 的 draft / prerelease / Latest 状态：v1.31.0-rc.5 已发布，draft=false、prerelease=true；GitHub Latest 仍为 v1.30.0。Release：https://github.com/kejilion/KPanel/releases/tag/v1.31.0-rc.5
- Docker 版本与通道 OCI index：stable latest 仍是 v1.30.0；preview 为 v1.31.0-rc.5。
- linux/amd64、linux/arm64 digest：preview OCI index sha256:ffb57b615fec667f101e0af5aa77c08a4b6f99995f3a7e78c2b8af2ff870cbeb；amd64 manifest sha256:f4b8ebc4f419ccb5bf4e312179fac55d2ab6ff21610d4cbbc987c3105bdfc9f9；arm64 manifest sha256:fde6fc278e1e2ee40087561d0384992c439bf05a6acc291373f8f956f7d7a511。
- 附件及 SHA256SUMS：GitHub Release 附件和 SHA256SUMS 已发布，清单 15 项；public acceptance 验证下载产物与公开 OCI identity。
- 公开镜像 image_e2e=pass：公开验收通过，使用上述不可变 index digest。
- kejilion/apps / kejilion.sh 契约结论：本次不修改应用市场仓库或 kejilion.sh；脚本联动判定为 not-required。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：公开验收验证稳定 Latest 与 RC 预览来源身份，以及精确镜像 digest。
- 加入预览只切换来源并立即检查，没有自动安装：浏览器检查确认预览可选择、不会自动安装。
- 自动安装开关与一次性立即安装相互独立：默认自动安装关闭；公开浏览器验收检查为未自动执行安装。
- 旧状态默认迁移到 stable，重启后通道选择保持：本次公开验收未覆盖所有本地状态迁移/重启组合，标记未验证。
- 退出预览且稳定版较低时没有产生降级候选：代码/自动化有相应保护；本次没有在真实主机升级状态中验证。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：自动化/代码检查通过；没有在本次隔离容器外执行真实 systemd 或宿主机备份恢复。
- OpenRC 与轻量 Node 的当前边界已按 docs/release-channels.md 明确呈现：发布说明沿用该边界；本次没有额外实机验证。

## 生产部署安全核对

> preview 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（预览版禁止生产部署；本次 production=false）。
- 验证/灰度环境：不适用；未连接 arena-154。
- 正式部署环境：不适用；未执行正式部署。
- prod-108：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用（无生产操作）。
- 部署命令/入口：不适用（无生产操作）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用（无生产操作）。
- 生产已执行写操作：否。
- 仅在隔离真机执行、未在生产执行的场景：GitHub-hosted L3、隔离容器公开产物测试与浏览器升级检查。

## 回滚

- 源码/tag：v1.31.0-rc.5 -> 6f9417d4183400cda6f5bd73224430e8f570beb9；上一稳定源码/tag 为 v1.30.0。
- 镜像 digest：本预览 sha256:ffb57b615fec667f101e0af5aa77c08a4b6f99995f3a7e78c2b8af2ff870cbeb；稳定回滚点 sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e；上一预览 v1.31.0-rc.4 digest sha256:3454abf049e0daeb9acade439322e6db979bb902ce85b19e5c93754f5d58f94c。
- 数据/配置备份：无生产迁移或生产写操作，因此未生成生产备份。
- 回滚步骤和回滚后复核：预览消费方停止选择 RC 并保持 stable 通道；生产版本无需回滚。公开 stable latest 与 Docker latest 未改变。
- 回滚后生产实际版本与健康状态：不适用（未部署）。
- GitHub Latest、Docker latest 与标准更新入口实际指向：Latest 与 Docker latest 保持 v1.30.0 稳定版本。
- 公共默认更新通道决策：不适用（保持 stable 默认；预览只对主动加入者开放）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-26T17:57:03+08:00
- 候选冻结时间：2026-11-28T20:28:44+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

候选 CI 中的三次失败均由候选门禁在预览发布前拦截并修正；交付 handoff 的一次证据拦截发生在公开预览完成后，但生产写操作从未开始。两者分别保留在自动门禁与发布流程异常记录中，不计为生产变更失败或生产回滚。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "handoff/contract/allowed-path-omission",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开预览及公开验收成功后，交付 handoff 首轮因 contract.allowedPaths 漏列合法验收夹具而失败；未修改或重新发布 tag/image，也未执行生产写入。",
    "recoveryEvidence": "contract.json 已补入精确路径 scripts/mock-app-market-api.mjs；同一 SHA 6f9417d4183400cda6f5bd73224430e8f570beb9 的 handoff 随后通过，输出保存于 C:\\\\GitHub\\\\_release-evidence\\\\kpanel-v131-rc5-20261128\\\\handoff.json。",
    "permanentAction": "本次合同范围遗漏已通过加入精确路径修正，既有 handoff gate 保持启用；下一次任务提交 contract 前由当次发布负责人复核完整候选 diff 与 allowedPaths。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 docs/project-management.md 13.1）：未回收。预览候选工作树、三条来源任务工作树及外部验收证据保留；来源作者释放所有权后再复核，未计算磁盘净变化。
- 未验证风险：真实宿主机 Docker 更新/失败恢复、systemd 与卷恢复、混合版本 Agent/Panel、长时间性能负载、200% 缩放及完整键盘焦点覆盖；安全审查中 2 项 deferred 需要按原审查记录继续处理。
- 已实现待实机准入：Compose 项目/容器升级及应用市场任务隔离已通过自动化、L3 和公开镜像浏览器验收；需要在获批隔离主机和真实 Docker/systemd 环境验证后才能形成实机结论。
- 不阻断本版的理由：该版本仅发布为显式 opt-in 的 preview；稳定 Latest、Docker latest 与默认更新通道仍保持 v1.30.0；未部署生产。
- 后续应进入的自动门禁或专项工作流：下一轮候选盘点时确认来源任务分支作者是否释放所有权；对 deferred 安全审查项按原 finding 记录复核；待 arena-154 密钥配置可用后再评估隔离真机准入。
