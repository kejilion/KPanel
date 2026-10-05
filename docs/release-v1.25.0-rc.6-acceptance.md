# KPanel v1.25.0-rc.6 发布验收记录

日期：2026-10-06

发布级别：L3

候选提交 / 标签：9a63e0f57bc257bfb3e8b62f136429da84827583 / v1.25.0-rc.6

上一稳定版本 / 回滚点：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；稳定 OCI index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b

releaseChannel：preview

releaseTrain：1.25.0

候选分支与发布后处置：release/v1.25.0-candidate / 预览版保留；main 继续为主线

- 原分支 / 精确 tip / 处置分类：RC5 后盘点 11 个活动候选来源，逐项记录于 C:/GitHub/_release-evidence/v1.25.0-rc.6/candidate-sources.json。4 个来源贡献本次选定变更：claude/compact-dialogs tip 2dd03bfd647051e001b04d3099f77bd00069a757；codex/windows-installer-compat tip 2c739c0d81cc6f60aed5cee6ab8ba84d5a94c6ac；fix/backup-archive-data-path tip 96d97075a449f7681c383f400094b2b1643ac930；claude/share-theme-immersive tip 0be14dcd184868239e7ec5a783e736f7eeb1f13b。其余 7 个活动候选按此前已整合处置保留，没有重放或丢弃来源。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：来源原 tip、原 disposition、归档 tip 和后续变化记录见 candidate-sources.json。发布时远端活动分支复核为 main 与 release/v1.25.0-candidate，两者均指向 9a63e0f57bc257bfb3e8b62f136429da84827583；候选来源历史 ref 保留。
- 本次来源任务分支：claude/compact-dialogs 纳入应用脚本任务后台运行及终端入口修正；codex/windows-installer-compat 纳入 ProgramData 标准 Users 元数据权限兼容和短 bootstrap 接入；fix/backup-archive-data-path 纳入备份根内安全符号链接；claude/share-theme-immersive 纳入协议 2 分享主题整页接管、刷新限速和本版说明。其余 7 个活动候选沿用 inventory 中的已整合判定。
- 本地分支/upstream/worktree：发布候选 worktree C:/GitHub/_codex-tasks/kpanel-v125-rc5 保留并固定于发布 SHA。4 个来源 worktree 在集成检查时均为 clean 并保留；其他任务 worktree 未因本次发布删除或重置。
- 未完成归档项 / 责任人 / 下次复核触发条件：本次来源处置没有遗失项；release/v1.25.0-candidate 按预览规则保留。其他任务 worktree 的负责人及回收时间不属于本次盘点范围，待所属任务结束后由其负责人复核。

预览产物已公开；生产未部署。RC6 不改变稳定更新入口。

## 发布画像

- 业务域：应用脚本任务、公共集群分享主题、Windows 轻量节点安装、主机备份恢复。
- 变更面：展示与交互、Windows 安装链路、备份归档格式内的符号链接处理；未更改对外 API、数据库 schema、端口、Compose、Agent 权限或生产部署。
- 受影响用户旅程：应用脚本任务可转为后台运行；公共分享页协议 2 主题可接管整页；Windows 节点使用较短且分层校验的 bootstrap 安装流程；主机备份可保存并恢复备份根目录内可解析的符号链接。
- 未变化契约：稳定更新通道、GitHub Latest、Docker latest、kejilion.sh、kejilion/apps 默认通道及既有端口与部署配置均未改变。
- 风险等级及理由：L3。版本包含 Windows 宿主交互、备份归档解析和跨站消息边界；自动测试和预览包验证已完成，但 CF scoped 审计与 Windows 真机安装、更新、重启、回滚及 RDP 实测尚未完成。

## 发布范围与未纳入内容

- 用户可见更新：RC5 后纳入应用交互任务后台运行；协议 2 公共分享主题可在整页模式显示受限的刷新、浅深色切换和恢复默认样式控件；Windows 节点 bootstrap 校验嵌套安装器与 EXE，安装器兼容 ProgramData 标准 Users 元数据权限但仍拒绝不受信任写入和重解析点；备份归档支持根目录内安全符号链接并在恢复时重建相对链接。
- 精确提交清单（RC6 基线 fd80529f60fe0f6ea6de712ab94b246fbe8e29de 至发布 SHA）：b9355d94e98ef30214f7040bd23b1a84d4410848、34e461841aefc4e2b5adc1bc7f7f4d88dde13113、6a37a138b09b662f6ab5f8a071bfc09ee6fd31f9、9d3d2689ef3bf7382fd0aa7d2bf69ce1336026a0、2209c4a5f0afdbf077bc176718f523ecc8ac3904、072aac5efd33548656bf04829e58412643ebb8fc、c5d8353c7ad782c38294a8f8e56b66e7e42b7af3、1db18c6cb23808588d13728d8a5d2a7ed7c7844b、d66364b2a1bcda25e02c19968c4b5a967c84fc5b、9a63e0f57bc257bfb3e8b62f136429da84827583。
- 明确未纳入的分支、文件或后续事项：候选清单中未选的 Windows/RDP UI 和其他旧候选改动未进入版本；未把落后候选整支重放；本版不做生产部署，不宣称 Windows 真机/RDP、长时间 soak 或 CF scoped audit 已通过。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：RC6 最终 SHA 为 9a63e0f57bc257bfb3e8b62f136429da84827583。security-coverage-final.json 的 decision 为 scoped-required；待审边界包括 internal/desktopbridge、internal/desktopcredentials、internal/windowsnode。
- 覆盖检查（check-security-audit-coverage.mjs 的 decision、未审计提交数）：decision=scoped-required；未审计提交数未单独记录。RC 记录此状态，不代表 CF 审计通过；稳定版必须先完成精确最终 SHA 的 scoped 审计和独立验证。
- finding fingerprint / 修复 commit / 独立复核与回归证据：最终范围内没有已完成 CF 审计确认的 finding；不得将未完成审计中的线索写作已验证发现。OCR 最终复核报告未列 finding；新增的最终提交只包含 Go 格式要求的空行。
- 修复交付状态：源码、候选分支与预览 Release 均为最终 SHA 9a63e0f57bc257bfb3e8b62f136429da84827583；稳定版和生产未交付、未部署。Windows 附件未签名；官方 HTTPS 与 SHA256SUMS 提供来源传输和文件完整性校验，不提供证书发布者身份保证。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：最终复核 r4 覆盖 40/40 个 reviewable 文件；报告记录 16 个 excluded/skipped 文件并分别给出 unsupported_ext、default_path 或 user_exclude 原因，且标记为人工检查或生成物核对。constrained-only 为 unreported；本次没有足够跨稳定周期样本作工具有效性结论。
- 按 PROJECT_RULES.md 5.4/5.5 记录本稳定周期的观察结果：本次只记录当前候选复核；稳定周期观察结论未记录，不据单个候选样本判断工具有效或退出。
- 候选分支的原始 tip、归档位置和远端核验：见本记录候选处置字段和 C:/GitHub/_release-evidence/v1.25.0-rc.6/candidate-sources.json。

## 跨仓库联动判定

- scriptLinkageState：not-required。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：c3a8bd895f8878d9e4ced7592c91a20c974472a5 / d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0。
- 脚本候选 commit / SHA-256：不适用；脚本未变更。
- 状态判定依据与兼容性证据：本轮脚本联动契约记录为 not-required；package/版本与 Release 变更均未更新 kejilion.sh 或 kejilion/apps。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Apps、备份、分享主题定向回归；L3 全量 Web 与 Go 检查通过。 | 未对真实 Windows 主机和真实 Panel/Agent 做端到端交互。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | Release workflow 完成漏洞扫描、附件摘要与双架构镜像一致性门禁；最终覆盖检查为 scoped-required。 | CF scoped boundary audit 尚未完成；Windows 文件未签名。稳定准入前必须完成 scoped audit。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | Go 单测、race、Linux amd64/arm64 构建、容器契约、备份恢复测试通过。 | 没有长时间 soak；Windows 安装、服务重启、更新和回滚留待真机验证。 |
| 性能与资源预算 | 已实现未实机验证 | L3 运行时门禁通过；分享刷新设有每秒一次速率限制。 | 本版没有生产负载或长时间资源预算测量。 |
| 用户体验与可访问性 | 已验证 | browser-preview-r3 mock UI 覆盖主题接管、主题切换、刷新、恢复原生样式、后台脚本任务及空搜索状态。 | 模拟数据只验证界面和反馈；没有完成真实服务交互、缩放矩阵和 Windows RDP 操作。 |
| 数据、配置与迁移 | 已验证 | 备份测试覆盖根内相对/绝对链接往返以及外部、无效链接拒绝；无数据库 schema 迁移。 | 未在用户真实备份集上运行恢复。 |

## 自动门禁

- 定向测试及结果：Web 定向测试 73 项通过；Windows 四附件边界测试通过；OCR 工具测试 4/4 通过；依赖政策 11 组、版本一致性、主题包 build check、gofmt -d 和 git diff --check 通过。L3 最终轮还通过 Web 全套、Go 单测与 race、Linux amd64/arm64 构建、容器契约和漏洞扫描。
- make verify-release 环境和结果：未单独调用该快捷入口；使用规范 L3 外层入口 scripts/run-release-l3.mjs 执行冻结候选验收，结果通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run ID v1.25.0-rc.6-9a63e0f5-l3-r2；RUNNER_IMAGE kpanel-go127-prep-runner:go1.27.1-node24.21.0；不可变 Runner ID sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c；plan.env SHA-256 3e5df66a9ba586693cf1fa32c342317541e49a8bb382952e4a41829fa72faf22；远程脚本 SHA-256 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979；bundle SHA-256 a2bde7a5f7f805756710c030657041b6bd95f8d3d335c9d00f60d6b662300020；最终 status=pass、进程退出码 0。原始计划、脚本、bundle、source-prepare 和结果摘要保存在 C:/GitHub/_release-evidence/v1.25.0-rc.6/l3-r2；运行时长和完成时间未记录。arena-154 在 environment-policy.json 中允许 candidate-validation；该次没有部署 KPanel。
- 候选 CI：run 37340791717 成功；依赖 freshness run 37340791876 成功。
- 主线 CI：run 37341747249 成功；依赖 freshness run 37341747306 成功。
- Release workflow：run 37343002701 成功；GitHub Release 于 2026-10-05 16:57 UTC 显示发布，标记为 Pre-release。
- 安全扫描、镜像契约、SBOM/provenance：Release workflow 的测试、漏洞扫描、双架构构建、镜像运行契约和摘要一致性检查通过；单独公开复核 SBOM/provenance 的证据未记录。RC6 tag dependency freshness run 37343002368 成功，advisory 子任务 skipped，不据此声称无漏洞。

第一轮 L3 在 Go 1.27 格式检查处失败，定位为 internal/cluster/light_windows_install.go 的 go:embed 前缺少 gofmt 要求空行；9a63e0f 为仅格式修正，最终 gofmt 检查及第二轮 L3 均通过。该情况是代码质量门禁拦截，不是公开版本失败或生产故障。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：未记录；候选、主线和 tag 的 dependency freshness workflow 均通过。
- 最近每日安全通告审计、EOL 复核状态及证据：未记录；RC6 tag 的 advisory 子任务为 skipped，不将其解释为无风险。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：未记录。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：最终 L3 runner 使用 Go 1.27.1 与 Node 24.21.0；本版相对 RC5 的 web/package.json 与 package-lock.json 只更新包版本字符串，没有依赖集合变化。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Web 包和锁文件版本为 1.25.0-rc.6；Docker 预览 OCI index 为 sha256:91d1725c2633d68af3842f828970cfb0cab9ae11fdc81d7c8cb96755677a565d；脚本提交与摘要见跨仓库联动字段。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：没有引入新依赖候选；未选的来源代码依据候选 inventory 的既有 disposition 保留。
- 升级后的兼容、安全、构建、性能资源和回滚结论：版本元数据与锁文件对齐；未新增依赖版本；双架构构建通过。性能 soak 未做；预览退出按现有稳定通道流程手动操作。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：L3 固定 runner image kpanel-go127-prep-runner:go1.27.1-node24.21.0、Runner ID sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c；arena-154 实际宿主发行版和架构未记录。没有 Windows 真机/RDP 验收。
- 环境策略 ID 与允许用途：arena-154；environment-policy.json 允许 candidate-validation、browser-validation、performance-validation、failure-injection、staging-deploy 和生产用途；本次仅用于 candidate-validation。prod-108 为 disabled 且无允许用途。
- 使用的精确候选或公开产物：9a63e0f57bc257bfb3e8b62f136429da84827583；本地浏览器 mock preview manifest id rc6-merged-candidates-1791216351008-45cf5c，source clean=true 且 SHA 与发布候选一致。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 ID v1.25.0-rc.6-9a63e0f5-l3-r2，终态 pass，退出码 0；后台任务 ID、超时、耗时未记录。L3 证据目录 C:/GitHub/_release-evidence/v1.25.0-rc.6/l3-r2；browser preview 地址 http://127.0.0.1:4175，mock API http://127.0.0.1:8083。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：没有进行 soak；预览 UI 交互检查与自动回归不替代长期负载验证。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：mock UI 旅程包含公共分享页官方主题整页显示、刷新限速、浅深色切换、恢复原生页、关闭脚本终端后后台运行并重新进入脚本管理、搜索不存在应用的空状态。mock UI 使用模拟数据，仅验证页面交互和错误反馈；缩放比例、最小字号和完整键盘矩阵未记录。
- 宿主机写入、失败注入、重启恢复和回滚结果：未对生产 KPanel 主机执行部署/升级写操作；自动化涵盖备份链接安全、恢复、容器运行和失败边界。未在 Windows 真机注入服务失败或实测 RDP。
- 未执行场景及原因：Windows 真机安装、服务重启、更新、回滚、RDP 由用户后续进行；生产部署不属于预览授权；长时间 soak 和真实用户备份恢复未执行。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：公开 Release v1.25.0-rc.6，Pre-release，20 项 assets；不是稳定版 Latest。
- Docker 版本与通道 OCI index：docker.io/kjlion/kejilion-panel:1.25.0-rc.6 与 :preview 均为 sha256:91d1725c2633d68af3842f828970cfb0cab9ae11fdc81d7c8cb96755677a565d；:latest 仍为稳定版 sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b。
- linux/amd64、linux/arm64 digest：RC6 amd64 sha256:48de51e781458300b5bb6bb0c92c54628286cbbd6b42f8bbe64caf9555453f10；arm64 sha256:479007fdd20e4d7700040096c245c58878c6d2b3392b0c2de9762e182ba45891。
- 附件及 SHA256SUMS：Release 共 20 项。Windows 四件为 bootstrap-windows.ps1（sha256:c10b3cd7e5a77c68cbc3645016a2426b49bc217a7fcd25adea533037ee919e9d）、install-windows.ps1（sha256:f8f32908de2cab6b40a5ccbe1bf6cacdead6be04c0122f5348c04c2d92aef509）、kejilion-node-windows-amd64.exe（sha256:7885ba359c682cbad23ad3c7bc236de0490ec8ec9aa030166a34bbe7f0e05793）、kejilion-node-windows-arm64.exe（sha256:fda5b0597198ddf247812965366d8a05b0155a721865cf5172846aa7b2e4cded）。SHA256SUMS 附件 SHA-256 为 c8cb918831bcf5ea281b047b615cb4f6164a24deb44941681aa65a121acaf153；Windows 四件最终字节摘要由 Release gate 校验。附件公开列表见 GitHub expanded assets。
- 公开镜像 image_e2e=pass：Release gate 执行双架构镜像运行契约与摘要一致性检查并通过；Docker Hub 实时 manifest 复核显示版本 tag 和 preview index 相同。
- kejilion/apps / kejilion.sh 契约结论：not-required；不改应用市场默认 stable 来源，也不改脚本仓库内容。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：契约测试通过；RC6 公开镜像 digest 固定如上。真实宿主升级未执行。
- 加入预览只切换来源并立即检查，没有自动安装：契约测试通过；真实生产节点未操作。
- 自动安装开关与一次性立即安装相互独立：契约/回归覆盖通过；本次没有生产更新。
- 旧状态默认迁移到 stable，重启后通道选择保持：已有发布通道契约保持；本次未在真实主机重启验证。
- 退出预览且稳定版较低时没有产生降级候选：Release 说明与契约要求保持“退出预览只切换稳定来源，不自动降级”。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：已有通道行为与自动检查保持；本次未执行生产系统更新。
- OpenRC 与轻量 Node 的当前边界已按 docs/release-channels.md 明确呈现：是；本版没有扩大 OpenRC 或轻节点自动更新支持承诺。

## 生产部署安全核对

> preview 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（本次仅授权并执行预览版发布；未授权生产部署）。
- 验证/灰度环境：arena-154 仅用于 candidate-validation；没有以生产部署替代验收。
- 正式部署环境：不适用（预览版禁止生产部署）。
- prod-108：environment-policy.json 标记 disabled 且 allowedPurposes 为空；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用，未进行生产部署。
- 部署命令/入口：不适用，未进行生产部署。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用，未进行生产部署。
- 生产已执行写操作：无。
- 仅在隔离环境执行、未在生产执行的场景：固定 runner 的 L3 候选质量门禁、临时容器运行契约与本地 mock browser preview。

## 回滚

- 源码/tag：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；RC6 tag 不可变且不覆盖。
- 镜像 digest：stable latest OCI index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b。
- 数据/配置备份：没有生产配置或数据写入，不需要本次生产回滚备份。
- 回滚步骤和回滚后复核：预览用户可按通道流程退出预览并切回 stable；退出预览不会自动安装较低版本。
- 回滚后生产实际版本与健康状态：不适用；本次没有生产部署或生产回滚。
- GitHub Latest、Docker latest 与标准更新入口实际指向：继续为 v1.24.0 / 原稳定 digest；本次未修改。
- 公共默认更新通道决策：不适用；RC6 只供主动加入预览的用户，stable 默认仍为 v1.24.0。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-05T16:00:37+08:00
- 候选冻结时间：2026-10-06T00:43:17+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（公开 Release 一次完成；无生产回滚或热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：未记录
- 其中生产写操作开始后异常次数：未记录
<!-- kpanel-release-process-metrics:end -->

发布过程异常历史记录不完整，不能可靠复算总数；依模板明确记为“未记录”，不推断为零。incident JSON 留空数组。

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：arena-154 L3 临时源目录清理状态为 removed；release runner 和测试容器按运行结果清理。RC6 产品 worktree、候选分支、候选来源分支及浏览器 mock preview 保留，供用户复核；未删除其他任务的 worktree 或缓存。
- 未验证风险：CF scoped audit；Windows 真机安装、更新、服务重启、回滚与 RDP；公开镜像 amd64/arm64 实机容器运行；完整缩放/键盘矩阵；长期负载。
- 已实现待实机准入：Windows 节点和安装器四附件已公开但未签名；Windows 主机安装、更新、回滚、重启和 RDP 需要真实设备验收。
- 不阻断本版的理由：RC6 标记 Pre-release；stable GitHub Latest、Docker latest 和默认更新源未改变；用户已说明 Windows 真机稍后自行测试。CF scoped audit 保留为 stable 前置门禁。
- 后续应进入的自动门禁或专项工作流：以最终 SHA 9a63e0f57bc257bfb3e8b62f136429da84827583 完成 CF security-boundary-audit 与独立验证；完成 Windows 真机及 RDP 记录；补录 dependency-report/每日审计证据及发布流程异常计数；稳定发布前重跑所需门禁。
