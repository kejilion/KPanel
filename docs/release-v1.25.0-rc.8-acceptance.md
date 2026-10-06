# KPanel v1.25.0-rc.8 发布验收记录

日期：2026-10-06

发布级别：L3

候选提交 / 标签：9bd765d2fad423154ada4e3aec3e9e4741a59efe / v1.25.0-rc.8

上一稳定版本 / 回滚点：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；稳定 OCI index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b

releaseChannel：preview

releaseTrain：1.25.0

候选分支与发布后处置：release/v1.25.0-candidate / 预览版保留

- 原分支 / 精确 tip / 处置分类：feature/remove-windows-light-node-20261006 / 9bd765d2fad423154ada4e3aec3e9e4741a59efe / 已快进纳入 main；远端 main 与 release/v1.25.0-candidate 在 RC8 发布时均指向该提交。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：RC8 使用不可变 tag v1.25.0-rc.8；RC2–RC7 的历史 tags 与 Releases 保留。RC7 Windows 节点设计档案为 docs/archive/windows-light-node-design-rc2-rc6.md。旧本地 release/v1.25.0-candidate worktree 仍在 RC6 SHA 9a63e0f57bc257bfb3e8b62f136429da84827583，本次未重置或删除；远端候选分支已推进至 RC8 SHA。
- 本次来源任务分支：feature/remove-windows-light-node-20261006 / 9bd765d2fad423154ada4e3aec3e9e4741a59efe 已纳入。其余非 Windows 候选 claude/compact-dialogs（2dd03bfd）、claude/share-theme-immersive（0be14dcd）、feature/latency-median-band（108162eb）、feature/office-light（b0513673）、feature/panel-login-notification（94d8b1b6）、fix/backup-archive-data-path（96d97075）的产品提交经 git cherry 对比均已在 origin/main 有等价 patch；没有重复重放。RC8 以 RC7 已整合代码线保留 RC2–RC7 的非 Windows 功能，移除 Windows 节点接入及衍生路径。
- 本地分支/upstream/worktree：当前工作树 C:\GitHub\_codex-tasks\kpanel-remove-windows-light-node-20261006 保留在 RC8 源码 SHA；为保存审查、发布和回滚证据，未回收任务 worktree。主仓库本地 main worktree 与本次源码提交不同步，远端 main 已验证为 RC8 源码提交；本记录 docs-only 提交随后快进至远端 main，不移动 RC8 tag 或候选 ref。
- 未完成归档项 / 责任人 / 下次复核触发条件：codex/windows-installer-compat / 2c739c0d81cc6f60aed5cee6ab8ba84d5a94c6ac 属于 Windows 节点候选，按用户要求不纳入 RC8，保留源 worktree 供追溯；其他历史 Windows worktree 与不可变旧版 Release 也未删除。若未来重新引入 Windows 节点，须建立新候选并完整复核安全边界和 Windows 隔离验收。

归档不代表生产上线；RC8 预览产物已发布，生产未部署。

## 发布画像

- 业务域：集群节点管理、轻量节点监控与远程节点访问。
- 变更面：移除 Windows 节点界面、接入、报告、终端/桌面桥接、文件/图库/历史监控、安装更新链路及专属发布资产；保留 Linux 轻量节点、通用 Windows MCP 客户端及 RC2–RC7 的其他功能。
- 受影响用户旅程：Windows 主机不能作为 KPanel 轻量节点加入或管理；Linux 轻量节点、节点监控和其他既有业务继续按原契约运行。历史 Windows 节点数据由当前 API 隔离/拒绝，UI 不再暴露对应业务。
- 未变化契约：没有数据库 schema 迁移、API 新增、端口或 Compose 变化；没有修改 kejilion/sh；稳定更新通道、GitHub Latest 和默认应用市场 stable 来源不变。Windows MCP 客户端是通用 MCP 工具，不是 Windows 节点 Agent/安装器，继续保留。
- 风险等级及理由：L3。版本移除已发布的远程节点能力并清理服务端、界面和交付路径；通过预览通道发布，不覆盖稳定版本或生产环境。

## 发布范围与未纳入内容

- 用户可见更新：移除 Windows 节点接入、批量接入、PowerShell/RDP、文件管理、图库、终端及节点历史监控；移除 Windows 节点 Agent、bootstrap、installer 和对应 release 门禁。保留 Linux 轻节点及 Windows MCP 客户端产物。
- 精确提交清单：从 RC7 tag 6c2aa2c0423e0a6f349af7b4ac2154db997d01de 到 RC8 共纳入 fd932255、a58ecddc、15af2f1d、d31a8106、f445baa3、9bd765d2。最终源码差异 83 files，+694/-2708；Windows 节点相关文件/路径移除，OCR 审核区间以 048f35876019cfc781e77e9f4b288bdf966a42c2 为基线。
- 明确未纳入的分支、文件或后续事项：codex/windows-installer-compat 候选不纳入，避免重新带入 Windows 节点接入/安装逻辑；其余已纳入产品 patch 的非 Windows 候选不重复 cherry-pick。保留 Windows 节点以外的 Windows 通用能力。RC2–RC7 历史 tag/Release、v1.24.0 stable、Docker latest、生产环境及 kejilion/sh 均未改动。真实 Windows 主机/RDP 实机测试按用户安排留待后续。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：Cloudflare security-boundary-audit run 18 基线为 f445baa30031b21c1a892802d3150e73cbbd13f0；任务在平台线程限制下中断，未完成侦察、hunter、coverage critic、独立验证和记录核验阶段。元数据 C:\GitHub\_codex-evidence\kpanel-remove-windows-light-node-20261006\security-audit-run-18\run-metadata.json 标记 incomplete-platform-interruption、scope_complete=false、成功子代理 0、委派失败 2。人工抽查未确认新增问题，但本次审计不构成 scoped/full pass，也不代表 zero findings；既有 filemanager protected-parent symlink/TOCTOU 项仍未独立复核。
- 覆盖检查：node scripts/check-security-audit-coverage.mjs --target HEAD 的 decision=ok；27 个提交/127 个文件尚未被审计，最早 2 天；最近 full audit run 4，距今 15 天；本次无新增 boundary package。此 coverage 结果仅是覆盖状态，不替代 RC8 scoped/full 审计。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本次未报告已确认新 finding。新增 Linux-only 平台拒绝、未知平台边界由 15af2f1d、a58ecddc 与 f445baa3 上的自动测试和候选/L3/Release 门禁验证。旧 TOCTOU 项没有新结论。
- 修复交付状态：RC8 源码、tag、公开 GitHub Release 和 preview 镜像已发布；稳定版和生产未交付。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：区间 048f35876019cfc781e77e9f4b288bdf966a42c2..f445baa30031b21c1a892802d3150e73cbbd13f0；83 文件中 57 个可审查文件完成 57/57（100%），26 个排除项有逐路径理由；0 findings。最终源码无 OCR 后变更；9bd765d2 仅补最终 review trailer。free-form=0，constrained-only=unreported（早期针对已替代区间的预览/rules 不可作为本区间证据）。
- 按 PROJECT_RULES.md 5.4/5.5 记录本稳定周期的观察结果：本次没有新的稳定周期观察数据，不宣称工具有效或应退出。
- 候选分支原始 tip 和归档状态见本节开头的候选处置字段；旧 RC tags 保留不变。

## 跨仓库联动判定

- scriptLinkageState：not-required（无需发布脚本）。
- 变更集编号：不适用；没有跨仓库发布。
- KPanel 实际内置脚本基线 commit / SHA-256：c3a8bd895f8878d9e4ced7592c91a20c974472a5 / d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0。
- 脚本候选 commit / SHA-256：不适用；本版未修改脚本。
- 状态判定依据与兼容性证据：节点 Linux 协议和脚本未改；Windows 节点安装/接入由 KPanel 仓库的 Agent、安装器和服务端路径提供，本版已删除。
- 本版发布决定：脚本不在范围；先前固定脚本基线仍满足现有 Linux 节点链路。
- 阻断或移除的依赖范围：删除 Windows 节点专属 build/install 链路与 IronRDP；其他跨仓依赖不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Linux 节点和远程监控自动测试、候选 CI、L3 及本地预览四条交互旅程通过。Windows 节点 API/UI/专属资产已移除或隔离。 | 未对真实 Windows 主机/RDP 实测；对应节点功能不再属于 RC8。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | CI、L3、Release 的依赖、源码、镜像和密钥扫描通过；Trivy 未报漏洞/配置/密钥问题。 | CF run 18 被平台限制中断；不得宣称 scoped/full audit 通过。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 源码/更新恢复/失败注入、Linux amd64/arm64 构建及运行契约通过。 | 未在真实宿主执行升级/重启/回滚或长期 soak。 |
| 性能与资源预算 | 不适用 | 清理了 Windows 节点代码与远程交互路径，没有新增常驻服务。 | 未做性能 soak。 |
| 用户体验与可访问性 | 已实现未实机验证 | npm build、web 测试和本地 mock 预览通过；旧 Windows 专属提示清理。 | 100%/125%/200% 缩放、完整键盘/屏幕阅读器矩阵未执行。 |
| 数据、配置与迁移 | 已验证 | 无 schema 迁移；旧 Windows 节点数据在当前公开路径被隔离/拒绝，Linux 平台边界有自动测试。 | 用户生产数据的升级行为没有在真实生产实例验证。 |

## 自动门禁

- 定向测试及结果：本地 web monitoring 16/16；npm run build 通过；OCR delegate 测试 4/4；dependency freshness validate-only 11 组通过；governance consistency、version consistency、environment policy arena-154 candidate-validation、collaboration-state candidate 检查均通过。
- make verify-release 环境和结果：固定 L3 外层入口 run v1.25.0-rc.8-9bd765d2-l3-r1 通过，exit 0；source、race/vet/vuln、web、containers、image workflow、更新/恢复失败注入均通过。Linux amd64/arm64 镜像构建通过；Trivy source/dependency/config/secret 与构建镜像均 0 findings。
- L3 计划/脚本/bundle SHA-256、不可变 Runner ID、证据目录：bundle ac026e3acf4567bb7b247332a5bb78748b70ab23712baacc8347c253aeae5b84；plan 7ca42891039f4acd92525591cb62d901de51b9074deee60f1b21e098128d1a5c；remote script 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979；runner kpanel-go127-prep-runner:go1.27.1-node24.21.0，ID sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c；证据 C:\GitHub\_codex-evidence\kpanel-remove-windows-light-node-20261006\l3-v1.25.0-rc.8-9bd765d2-l3-r1。L3 manifest bundle sha256 ac026e3acf4567bb7b247332a5bb78748b70ab23712baacc8347c253aeae5b84；plan sha256 7ca42891039f4acd92525591cb62d901de51b9074deee60f1b21e098128d1a5c；remote script sha256 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979。
- 候选 CI：成功，run 37465597600，精确源码 SHA 9bd765d2fad423154ada4e3aec3e9e4741a59efe；Dependency freshness run 37465597613 成功。
- 主线 CI：成功，run 37467078179，精确源码 SHA 9bd765d2fad423154ada4e3aec3e9e4741a59efe；Dependency freshness run 37467077846 成功。
- Release workflow：成功，run 37468583120，2026-10-06 21:09:21–21:21:55 +08:00；14 个步骤成功，Dependency freshness run 37468583316 成功，security-advisories job skipped；预览版本跳过 stable-only candidate archive。
- 安全扫描、镜像契约、SBOM/provenance：Release 源码/依赖/npm/镜像扫描、runtime image contract、多架构推送及 preview 通道 digest promotion 成功；security advisories job skipped。流程没有提供独立 CF scoped/full audit 结果。

## 依赖与技术栈变化

- dependency freshness：最终源码上的 workflow 11 组检测通过；security advisories job skipped。没有据此推断已完成单独 EOL 专项审查。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本次未生成单独 dependency-report，不对未取得的数据作结论。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go 1.27.1、Node 24.21.0；无新增依赖或 Dockerfile 基础镜像候选。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Release 版本 v1.25.0-rc.8；公开 preview OCI index sha256:61cf716d5700833172a0cd98b97e0861cfaaaee23481e3f7191153cd1480720a；内置脚本基线见跨仓库联动章节。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无新的依赖候选或跨仓脚本候选；安全审计中断及未独立复核项列入遗留风险。
- 升级后的兼容、安全、构建、性能资源和回滚结论：L3 与 Release 构建和扫描通过；未做真实主机性能测试。用户可选择预览来源，不会自动安装。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：arena-154 固定 runner，Go 1.27.1、Node 24.21.0；Linux amd64/arm64 构建。
- 环境策略 ID 与允许用途：arena-154 / candidate-validation；未使用 prod-108。
- 使用的精确候选或公开产物：源码 9bd765d2fad423154ada4e3aec3e9e4741a59efe；预览 Release v1.25.0-rc.8。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 ID v1.25.0-rc.8-9bd765d2-l3-r1，pass，exit 0；证据目录及计划/脚本/bundle SHA 见自动门禁。
- 测试窗口/循环数及风险依据：单次 L3；没有长期 soak。此为预览版本。
- 受影响用户旅程、视口、缩放、主题、键盘/焦点、语言和失败态：本地 mock 预览地址 http://127.0.0.1:4173，最终源码 SHA 已记录；集群列表显示 Linux 节点且无 Windows 节点；远程节点监控加载；磁盘读写和 TCP/UDP 连接切换均通过；切回本地后加载本地历史且不保留远程状态。浏览器 evidence 为 C:\GitHub\_codex-evidence\kpanel-remove-windows-light-node-20261006\feature-preview-rc8-final。
- 宿主机写入、失败注入、重启恢复和回滚结果：失败注入与更新/恢复验证在隔离 L3 runner 通过；本地浏览器使用 mock 数据。没有连接、备份或写入生产主机。
- 未执行场景及原因：真实 Windows/RDP 验收由用户留待后续执行；没有实机长期运行、重启/回滚或全套可访问性矩阵。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：v1.25.0-rc.8 已公开，draft=false、prerelease=true；GitHub Latest 仍是 v1.24.0。
- Docker 版本与通道 OCI index：RC8 Release API 记录 OCI index sha256:61cf716d5700833172a0cd98b97e0861cfaaaee23481e3f7191153cd1480720a；Release workflow 已验证版本标签与 preview 通道 digest 等于本次构建 index digest。构建/推送使用 Linux amd64 与 arm64。执行环境无法独立查询 Docker Hub Registry，因此未声称直接 GET 验证或记录 per-architecture child digest。Docker latest 未被本次 workflow promotion 步骤触及；发布后无法独立读取其远端 digest，故本记录标为未独立复核。
- linux/amd64、linux/arm64 digest：均已构建并推送；child manifest digest 未独立读取/记录。
- 附件及 SHA256SUMS：14 个资产。包含 Linux Agent 与 Linux light-node 的 amd64/arm64 包、Darwin/Linux/Windows MCP 客户端、元数据 tarball、LICENSE、THIRD_PARTY_NOTICES 和 SHA256SUMS；不包含 Windows 节点 Agent/bootstrap/installer。GitHub API 的 SHA256SUMS 资产 digest 为 sha256:3b8e00b11e2a2f97cf2a325fa5688bc6f8c1b73f524f43c996dde68974ca0bf5；本地无法独立下载二进制并逐项重算清单。
- 公开镜像 image_e2e=pass：Release workflow build、native runtime contract 和 channel promotion 成功，均以本次多架构构建 digest 验证。
- kejilion/apps / kejilion/sh 契约结论：无跨仓脚本发布；默认应用市场 stable 通道和脚本基线未变。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：channel contract 检查通过；GitHub Latest 保持 v1.24.0。真实主机更新未执行。
- 加入预览只切换来源并立即检查，没有自动安装：自动契约测试通过；未在生产操作。
- 自动安装开关与一次性立即安装相互独立：既有契约测试通过，本次没有触发安装。
- 旧状态默认迁移到 stable，重启后通道选择保持：既有 release-channel 测试通过；没有实机重启验证。
- 退出预览且稳定版较低时没有产生降级候选：预览通道不自动降级契约未变；未执行真实宿主降级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：L3/自动测试覆盖并通过；没有真实系统更新或回滚。
- OpenRC 与轻量 Node 的当前边界已按 release channels 文档明确呈现：Linux 轻节点保留；Windows 节点不在 RC8 支持范围。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（只授权并执行预览发布；没有生产部署授权）。
- 验证/灰度环境：arena-154，仅 candidate-validation。
- 正式部署环境：不适用（预览版禁止生产部署）。
- prod-108：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用；未进行生产部署。
- 部署命令/入口：不适用；未进行生产部署。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用；未进行生产部署。
- 生产已执行写操作：无。
- 仅在隔离环境执行的场景：L3 构建、镜像验证、失败注入和本地 mock 浏览器交互。

## 回滚

- 源码/tag：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；RC8 tag 不可变。
- 镜像 digest：稳定 OCI index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b。
- 数据/配置备份：没有生产数据或配置写入，无需本次生产回滚备份。
- 回滚步骤和回滚后复核：预览用户可切回 stable 来源；退出预览不会自动降级已安装版本。未执行真实宿主回滚。
- 回滚后生产实际版本与健康状态：不适用；没有生产部署。
- GitHub Latest、Docker latest 与标准更新入口实际指向：GitHub Latest 仍为 v1.24.0；标准更新默认 stable。Docker latest 的发布后 Registry 状态未能独立查询；workflow 未将 preview promotion 指向 latest。
- 公共默认更新通道决策：不适用；预览版没有改变默认 stable 通道。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-06T18:47:38+08:00
- 候选冻结时间：2026-10-06T20:26:53+08:00
- 生产完成时间：未验证
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（RC8 预览 Release 一次成功；无生产回滚或热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/dockerhub-read/registry-query-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "发布前独立 Docker Hub REST/OCI 只读查询超时，无法通过该网络路径预先复核 registry tag；未造成 Release 重试，未发生生产写操作。",
    "recoveryEvidence": "推送前 Git tag 与 GitHub Release 均确认不存在；Release workflow 37468583120 构建、推送并验证版本标签和 preview 通道 digest 等于本次构建 index digest，最终 GitHub Release 发布成功。",
    "permanentAction": "将 Release workflow 的构建后 digest 等同性检查作为权威镜像校验；Docker Hub 独立只读 GET 保留为补充证据。该补充读取不可用时记录限制，不绕过 workflow gate；下次发布复核其网络读取是否恢复。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：未清理工作树或证据目录，净释放 0 bytes。RC8 候选 worktree、L3/审计/OCR 证据和历史候选 worktree 保留以便复核与恢复；未执行删除。
- 未验证风险：CF scoped/full audit 未完成且已有 TOCTOU 事项尚未独立复核；真实 Windows/RDP 试验按用户安排后续执行；Docker Hub latest 与各架构 child digest 未能从当前环境独立读取；真实宿主更新、回滚、长期 soak 和完整可访问性测试未执行。
- 已实现待实机准入：Windows 节点逻辑已从 RC8 清除，Linux 节点自动路径通过 L3；真实主机准入仍由后续使用者验收。
- 不阻断本版的理由：本版是明确的 preview RC；源码/候选/main/Release workflow 门禁通过，GitHub Latest 与生产不变。未完成的 CF 审计被清晰记录，不能视作审计通过。
- 后续应进入的自动门禁或专项工作流：在稳定发布前完成要求的 CF 审计与未结项独立复核；若恢复 Windows 节点能力，重开新候选并补安全审计及真实隔离 Windows/RDP 验收。