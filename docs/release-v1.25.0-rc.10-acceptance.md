# KPanel v1.25.0-rc.10 发布验收记录

日期：2026-10-07

发布级别：L3

候选提交 / 标签：`da4c597accb0f60c09a8b7463b8192fd46d30bce` / `v1.25.0-rc.10`

上一稳定版本 / 回滚点：`v1.24.0` / tag target `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；Docker `latest` OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`

`releaseChannel`：`preview`

`releaseTrain`：`1.25.0`

候选分支与发布后处置：`release/v1.25.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：`release/v1.25.0-candidate`，`da4c597accb0f60c09a8b7463b8192fd46d30bce`；预览序列唯一候选，远端保留；`main` 与候选分支已读回同一 SHA。RC10 标签的 annotated tag object 为 `0537550d1d84fadc23e725ec3dc6782cadade1eb`，peeled target 与候选一致。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：候选集成分支本地归档 `archive/feature/rc10-desktop-mobile-script-window`=`da4c597accb0f60c09a8b7463b8192fd46d30bce`；预览候选按规范不归档。RC10 tag 和远端 main 同 SHA。L3 bundle `C:\GitHub\_release-artifacts\kpanel-v125-rc10-da4c597-l3-r1\kpanel-v1.25.0-rc.10-da4c597-l3-r1.bundle`，SHA-256 `3eae20cfa2f3ce44c6f33a08588b69e1a584f1dd9c27d7e404aa764f80adcaf3`。
- 本次来源任务分支：`claude/desktop-mobile-icon-pager` tip `3d0548ee91512bd3dd1580c9a7bc083f5fdcfdf0` 的三项提交经 `git range-diff` 分别对应候选 `633738b73eabdeab9340c328691ffdf90515f8f3`、`107c0df61ba238ccadf26f2622e9d28bf3ec0201`、`3c82a04ece1208d958e91a736b8bc34a7451f230`；`claude/compact-dialogs` tip `af8d02d45cd2fd58b7dbd3fe0cfa8bab01630195` 对应 `9800f5dae9753e7f7c559ea03168d251f8905eeb`。发布后活动本地引用已删除，精确 SHA 保存在同名 `archive/` 本地引用。远端已有 `origin/archive/claude/compact-dialogs` 指向较早的 `4ed8eda42d1edf4a2b9d998c74298338fa6bec35`，未覆盖该不可变归档。
- Windows 候选处置：活动本地分支 `codex/windows-installer-compat`、`feature/remove-windows-light-node-20261006`、`feature/prune-windows-light-node-state-20261006` 已按预期 SHA 删除；恢复点分别为 `archive/codex/windows-installer-compat`=`2c739c0d81cc6f60aed5cee6ab8ba84d5a94c6ac`、`archive/feature/remove-windows-light-node-20261006`=`e6f3ca26e5a45ee29e7f0cd1748dc26f36096ce4`、`archive/feature/prune-windows-light-node-state-20261006`=`0e7972a60847fcb84aa6085e777fbe5b96959fcf`。10 个 Windows 相关工作树均已从 Git worktree 列表移除；远端没有活动 Windows 分支，既有 `origin/archive/*windows*` 历史引用保持不变。
- 本地分支/upstream/worktree：RC10 集成工作树及候选分支已删除，精确候选仍可从 RC10 tag、远端 main/候选分支及本地 archive 恢复。本地规范候选工作树已快进到 `da4c597accb0f60c09a8b7463b8192fd46d30bce`。两个已发布来源工作树的 Git 注册、活动分支均已移除，但 Windows 报告目录删除权限拒绝；留下的两个空目录无 `.git` 标记、无 Git worktree 注册，暂留待 OS 文件句柄释放后复核。忽略的 `web/dist` / `web/node_modules` 已随成功删除工作树回收；净释放字节未测，不作估算。
- 未完成归档项 / 责任人 / 下次复核触发条件：仅剩上述两个空目录的物理回收；责任人为当前发布维护者，在系统不再报告目录“被另一进程使用”后复核并删除。活动分支、Git worktree 和发布产物不受影响。唯一 Windows 安装器候选只保存在本机 archive，未推送远端。

归档不代表生产上线；本次产物已发布，生产未部署。

## 发布画像

- 业务域：桌面启动器布局与应用脚本任务窗口。
- 变更面：展示与交互；无宿主机写入、协议或数据迁移。
- 受影响用户旅程：窄屏桌面可横向分页并使用分组文件夹；应用脚本运行进度窗口将“后台运行”收进标题栏最小化操作，底部操作栏移除，结束任务仍由关闭确认流程处理。
- 未变化契约：API、持久数据、端口、Compose、Agent 权限、`kejilion.sh` 和应用市场契约不变；RC9 已移除的 Windows 节点功能没有在 RC10 中恢复。Release 保留通用 Windows MCP 客户端附件，它不是 Windows 节点接入功能。
- 风险等级及理由：产品改动为局部 UI 交互，未涉及授权、数据和宿主机写入；版本按 L3 发布门禁验证。已接受一项 LOW 可访问性限制：文件夹图标 DOM Tab 顺序可能与视觉分页顺序不同，焦点处理会滚动到当前聚焦项。

## 发布范围与未纳入内容

- 用户可见更新：手机桌面横向分页、分组文件夹、打开文件夹时的手势边界，以及应用脚本任务窗标题栏最小化和底部操作栏移除。
- 精确提交清单：基线 `40a5b9299b4382eaa130c0b1044e1d67999b0076`；产品提交 `633738b73eabdeab9340c328691ffdf90515f8f3`、`107c0df61ba238ccadf26f2622e9d28bf3ec0201`、`3c82a04ece1208d958e91a736b8bc34a7451f230`、`9800f5dae9753e7f7c559ea03168d251f8905eeb`；版本准备 `c3311e1e4f831338a9f457039eab19a76af51bac`；line-review 记录 `da4c597accb0f60c09a8b7463b8192fd46d30bce`。基线至候选 20 个文件，`+972/-80`。
- 明确未纳入的分支、文件或后续事项：不新增 Windows 节点能力；不修改 `kejilion/sh`、`kejilion/apps`、稳定更新源、Docker `latest`、GitHub Latest 或生产。100%/125%/200% 原生缩放、真实 KPanel/Agent/主机和长稳 soak 未执行，按用户计划留待后续真机测试。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：本版未执行 CF scoped/full 安全审计；本次界面差异未触及 CF 安全边界。自动依赖与源码扫描及 L3 隔离验证已执行，不能替代 scoped/full 审计。
- 覆盖检查（`check-security-audit-coverage.mjs` 的 decision、未审计提交数；RC 只记录。稳定版带 `--require`，非 ok 时写补审 run，或用户原话、理由与不超过 14 天的补审截止日）：候选精确 target 的 coverage checker `decision=ok`；最近 full audit run 距今 15 天；未审计积压 `29 commits / 127 files`，最旧 2 天，最大 14 天。此结果不代表本版 CF scoped/full 审计完成。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本版没有新增安全 finding 或修复提交；line review 接受上述一项 LOW 键盘焦点顺序限制。安全扫描发现 1 项位于必需依赖、但未见被调用路径触达的 govulncheck advisory，留待依赖治理复核。
- 修复交付状态：源码、RC、公开 Release 均为同一精确 SHA `da4c597accb0f60c09a8b7463b8192fd46d30bce`；无安全修复交付，不适用稳定版和生产部署。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：line-review trailer `OCR-Review: 1.12.11 range=40a5b92..c3311e1 files=17/17 free-form=0 valid=H0/M0/L1 constrained-only=unreported`；精确范围内 17/17 个可审查文件覆盖，0 free-form，1 项 LOW；constrained-only 未报告。
- 本稳定周期的观察结果：RC10 为预览版，不据此宣布工具稳定性或退出结论；稳定周期仍需按 `PROJECT_RULES.md` 5.4/5.5 继续观察。
- 候选分支原始 tip、归档位置与远端核验见本记录顶部的处置清单；无活动 Windows 候选分支或 worktree。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh` commit `c3a8bd895f8878d9e4ced7592c91a20c974472a5`；Dockerfile 固定脚本 SHA-256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：本版无 shell 脚本、应用市场或跨仓库变更；Docker 构建仍校验原固定脚本摘要。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向 3 个测试文件共 91 项通过；候选、main CI 和 L3 source checks 通过 | 用户真实主机环境待后续真机测试 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、Trivy source/config/image、SBOM/provenance 与 image contract 均执行 | govulncheck 有 1 项未见调用路径的必需依赖 advisory；CF scoped/full audit 未执行 |
| 稳定性、失败恢复与兼容 | 已验证 | Go full tests/race、web 测试与构建、更新备份 parity、原生镜像运行契约通过 | 实机长稳和生产回滚未执行 |
| 性能与资源预算 | 已实现未实机验证 | 390/768/1280 CSS px 页面宽度与视口一致；任务窗口在 195 CSS px 下可容纳 | 全页受既有 320 px 最小宽度约束；没有实际 200% 浏览器缩放或性能 soak |
| 用户体验与可访问性 | 已验证 | Playwright 验证分页、文件夹打开/关闭、Escape 焦点恢复、滚轮边界、窗口最小化/恢复和关闭流程 | DOM Tab 顺序和视觉文件夹顺序有一项已接受 LOW 限制 |
| 数据、配置与迁移 | 不适用 | 仅 UI 与交互改动，没有持久状态或迁移 | 不适用 |

## 自动门禁

- 定向测试及结果：3 个目标测试文件、91 项通过；`npm --prefix web run build` 通过。Headless UI smoke 证据位于 `C:\GitHub\_codex-evidence\kpanel-v125-rc10-ui-da4c597\`。
- `make verify-release` 环境和结果：Windows 工作站缺少 `go` / `gofmt`，本地工具链 preflight 未运行 Go 检查；同一冻结 SHA 的 L3 Linux Runner 完成完整 Go/web 测试、race、构建、应用更新备份 parity 和镜像运行契约，终态 pass。`govulncheck` 未发现可达调用路径漏洞，另有 1 个未见调用的依赖 advisory。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.25.0-rc.10-da4c597-l3-r1`；plan `b3d3be5c61f4289b97b0aad0117153f475f7f5cdd7db8fe9108e50bd2904a737`；remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `3eae20cfa2f3ce44c6f33a08588b69e1a584f1dd9c27d7e404aa764f80adcaf3`；Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`；`arena-154` 终态 pass；证据目录 `C:\GitHub\_release-artifacts\kpanel-v125-rc10-da4c597-l3-r1`。
- 候选 CI：run `37506265199` success，精确 SHA `da4c597accb0f60c09a8b7463b8192fd46d30bce`；Dependency freshness `37506265078` success。
- 主线 CI：run `37507157806` success；Dependency freshness `37507157658` success；主线精确 SHA 与 RC10 标签 target 一致。
- Release workflow：run `37508074687` success；Release job 12m46s（整个 run 12m50s）；公开时间 `2026-10-07T02:14:02+08:00`。
- 安全扫描、镜像契约、SBOM/provenance：Go 调用路径检查、npm audit、Trivy source/config/native image 均通过；原生运行镜像契约通过；多架构镜像生成 SBOM/provenance 并完成摘要核对。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：未单独生成。本版没有依赖版本升级；`web/package.json` 与 lockfile 仅更新 RC 版本号，依赖项版本保持不变。
- 最近每日安全通告审计、EOL 复核状态及证据：未单独执行每日 advisory/EOL 复核；发布工作流中的 govulncheck、npm audit、Trivy 扫描已通过，未达成 CF scoped/full audit 结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未增加直接依赖或基础镜像；govulncheck 的 1 项未见调用依赖 advisory 交依赖治理复核，稳定版准入前给出负责人和处置结论。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无版本升级；沿用固定 Go `1.27.1`、Node `24.21.0`、既有容器基座和 SHA 固定 Actions。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：锁文件只改 `1.25.0-rc.9` 到 `1.25.0-rc.10`；内置 `kejilion.sh` 固定 commit/hash 见跨仓库联动章节；公开镜像 digest 见发布产物章节。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：未新增依赖候选；未见调用路径的依赖 advisory 在稳定版准入前由依赖维护者复核。
- 升级后的兼容、安全、构建、性能资源和回滚结论：没有依赖升级；双架构构建和运行契约通过；UI 真机性能未验证；回滚点仍为 `v1.24.0`。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：本地 Windows 工作站上的 system Chrome，由 Playwright headless 驱动；主机具体 Windows build 未记录。发布 L3 使用冻结 Linux Runner `arena-154`。
- 环境策略 ID 与允许用途：`arena-154 candidate-validation`，仅候选验证；未连接 `prod-108`。
- 使用的精确候选或公开产物：`da4c597accb0f60c09a8b7463b8192fd46d30bce`；本地页面使用 mock API，不是实际 Panel/Agent。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.25.0-rc.10-da4c597-l3-r1` pass；release workflow run `37508074687` success。headless UI evidence 目录为 `C:\GitHub\_codex-evidence\kpanel-v125-rc10-ui-da4c597\`；独立远端作业命令规格未建立，不适用。
- 测试窗口/循环数及风险依据：短时交互 smoke，无长稳循环；改动仅限 UI，真实主机和长稳由用户后续真机测试。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：在 390、768、1280 CSS px 检查页面宽度；195 CSS px 检查对话框边界，不等于浏览器实际 200% 缩放。验证分页、文件夹触摸/滚轮边界、Escape 焦点恢复、中英文标题、最小化/恢复和关闭确认；初始 mock 缺旧协议 `input-transport` 路由的失败提示已通过 transport interception 重跑排除。未跑真实 100%/125%/200% 缩放。
- 宿主机写入、失败注入、重启恢复和回滚结果：本地 mock 不执行宿主机写入；UI-only 不涉及数据迁移或重启恢复。L3 更新备份 parity 通过。
- 未执行场景及原因：真实 Panel/Agent/集群、触摸真机、100%/125%/200% 原生缩放、实际 Docker 拉取运行和长稳 soak 未执行；本机无 Docker CLI，用户计划稍后实机测试。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：`v1.25.0-rc.10` 已发布、非 draft、`Pre-release`；GitHub Latest 仍为 `v1.24.0`。Release 页面显示 16 项（14 个附件及 GitHub 自动源码归档）。附件包含 `SHA256SUMS`、Linux Agent/Node、MCP client 和 metadata；其中 Windows MCP client 是通用 MCP 客户端，不是 Windows 节点程序。
- Docker 版本与通道 OCI index：`kjlion/kejilion-panel:1.25.0-rc.10` 与 `:preview` 均为 `sha256:8f92695039d4842356d24a967c3414cca501af1dea57426b67be0f178e73d472`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:35b7ab7bbbd08b895f3a97c396a8aa331d269e3415b98428691bc5269fdb4dbd`；arm64 `sha256:a5ea1d36373941682d0034ee60b46832f0cf031a34510ad5b02deaaecf1fc42f`。
- 附件及 `SHA256SUMS`：公开 expanded-assets 页面列出 14 个附件，含 `SHA256SUMS`；另有 GitHub 自动源码 ZIP/TAR。Release workflow 上传步骤成功；没有逐件下载附件复算。
- 公开镜像 `image_e2e=pass`：本机无 Docker CLI，未单独从 Docker Hub 拉取运行；Release workflow 的原生镜像运行契约、双架构发布及版本/preview digest 一致性检查均通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：没有改配套仓库或脚本；`kejilion.sh` 使用既有固定摘要，应用市场契约不变。

## 自更新通道验收

- 稳定来源与预览来源：Release 页面 Latest=`v1.24.0`；preview 是规范 RC，Docker 官方镜像摘要一致；L3 与 main CI 全套通道契约测试通过。
- 加入预览只切换来源并立即检查、没有自动安装：本版未改变该流程；未在真实实例单独操作。
- 自动安装开关与一次性立即安装：本版未改变相关代码；由 CI 合同测试覆盖。
- 旧状态迁移到 `stable`、重启后通道选择保持：本版未改变相关代码；由 L3 完整 source checks 覆盖。
- 退出预览且稳定版较低时没有产生降级候选：本版未改变相关代码；未在真实实例运行升级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：L3 app-conf update backup parity 通过；真实主机重启恢复未验证。
- OpenRC 与轻量 Node 的当前边界：本版不改通道边界，沿用 `docs/release-channels.md`。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：`arena-154` 仅用于隔离候选验证；没有部署。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：否。
- 仅在隔离真机执行、未在生产执行的场景：L3 源码测试、扫描、双架构镜像构建及原生运行契约验证。

## 回滚

- 源码/tag：回滚至 `v1.24.0`，tag target `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`。
- 镜像 digest：Docker `latest` `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；该摘要发布后未变化。
- 数据/配置备份：UI-only 预览未迁移数据；生产未操作，无新增备份。
- 回滚步骤和回滚后复核：预览用户退出 `preview` 并按现有稳定来源/应用市场回滚流程回到 `v1.24.0`；真实回滚未执行。
- 回滚后生产实际版本与健康状态：不适用（未部署生产）。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest=`v1.24.0`；Docker `latest` 仍为原 OCI index；标准更新默认仍为 stable。
- 公共默认更新通道决策：维持 stable 默认；preview 只供主动加入预览的用户选择，不自动安装，不触碰 `latest`。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-07T00:48:32+08:00
- 候选冻结时间：2026-10-07T01:30:15+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（预览 Release 一次发布；未做生产写操作）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "validation/windows-l2/missing-go-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 工作站缺少 go 和 gofmt，导致本地 verify-change preflight 未运行 Go 检查。",
    "recoveryEvidence": "同一冻结 SHA 在 L3 固定 Linux Runner 完成 Go/web 全套验证、race、构建和更新备份 parity；候选 CI 与 main CI 均 success。",
    "permanentAction": "本地继续 fail-fast；启动前检查 Go 工具链，缺失时直接使用 manifest 已固定的 L3 Runner image 与 immutable ID。",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/powershell/bash-script-runner-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次版本一致性检查错误地通过 node 调用 bash 脚本，命令失败后需要改用仓库 Bash wrapper 重跑。",
    "recoveryEvidence": "使用 node scripts/run-repo-bash.mjs scripts/check-version-consistency.sh 重跑并通过；候选版本所有位置均为 1.25.0-rc.10。",
    "permanentAction": "PowerShell 下统一通过 scripts/run-repo-bash.mjs 调用 bash 检查脚本，并在执行前确认入口类型。",
    "historicalReleases": []
  },
  {
    "fingerprint": "ui/playwright/browser-harness-setup-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 UI smoke 的路由与 mock transport 配置不匹配，产生无效应用路径/输入失败提示和过渡态截图，未采用为验收证据。",
    "recoveryEvidence": "改为从桌面图标进入应用并拦截 legacy input-transport，重跑 Playwright；最终三视口及脚本窗口交互断言通过，保留 r2 截图和 JSON。",
    "permanentAction": "复用已验证的 preview entry 与 mock route fixture；失败态截图标记为无效，不进入验收附件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/github-rest/anonymous-rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "发布后状态轮询触及 GitHub REST 匿名速率限制，未能使用该 API 响应作为完成证据。",
    "recoveryEvidence": "使用 GitHub Release 页面和 Actions run 页面确认 prerelease、Latest 与 workflow success，并以 Docker Hub 公共 tag API 复核 OCI index 与平台摘要。",
    "permanentAction": "发布验收保留 GitHub HTML/Actions 页面和 Docker Hub tag API 作为可复核的只读 fallback，减少无效匿名 REST 轮询。",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/github-web/release-page-404",
    "position": "before-production-write",
    "count": 1,
    "impact": "发布 tag 后首次外部页面快照对 RC10 release URL 返回 404，不能作为未发布的结论。",
    "recoveryEvidence": "稍后直接 HTTP 请求返回 200；重读的公开 GitHub Release 页面显示 Pre-release 和发布说明，Actions run success，Docker Hub version/preview 摘要一致。",
    "permanentAction": "对刚完成的 Release 采用可重试的 direct HTTP/公开页面核验，并等待传播后再判定不存在；禁止基于单次快照撤回或重发。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/worktree/os-directory-in-use",
    "position": "before-production-write",
    "count": 2,
    "impact": "两个已发布来源工作树执行普通 git worktree remove 时遇到 Windows Permission denied，Git 注册已移除但空目录仍被系统报告为使用中。",
    "recoveryEvidence": "精确提交已保存在 archive/claude/desktop-mobile-icon-pager 与 archive/claude/compact-dialogs；两个路径无 .git 标记且不在 git worktree list，活动分支引用已按 SHA 删除；未使用 --force。",
    "permanentAction": "保留空目录并在 OS 文件句柄释放后复核；禁止强删，确认路径无内容且不再被占用时再做普通目录回收。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

所有流程异常均发生在生产写操作之前。候选 CI、main CI、L3、Release workflow 最终均在精确 SHA `da4c597accb0f60c09a8b7463b8192fd46d30bce` 成功；本版无生产操作。

### 流程异常明细

L3 入口使用固定 Runner 完整通过；GitHub 公开 Release 于 `2026-10-07T02:14:02+08:00` 发布，预览 OCI index 一致，`latest` 和 GitHub Latest 未改变。工作树回收的两个空目录待 OS 文件句柄释放后复核，当前不再是 Git worktree 或活动候选分支。

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：10 个 Windows 相关 worktree、3 条活动 Windows 分支、RC10 集成工作树与本地来源活动分支已移除；RC10 发布来源的 `node_modules` / 构建缓存随成功删除的工作树回收。两个来源目录当前为空但系统报告占用，保留到句柄释放；C 盘前后空闲量未记录，因此净释放字节未验证。精确提交恢复位置见顶部本地 archive ref 与公开 RC10 tag。
- 未验证风险：真实机器/实际 KPanel 与 Agent、100%/125%/200% 缩放、长稳 soak、公开镜像本机运行、CF scoped/full audit 和 1 项未见调用路径的依赖 advisory 处置。
- 已实现待实机准入：手机桌面横向分页/文件夹、脚本任务窗口标题栏最小化；用户计划后续在真机验证。
- 不阻断本版的理由：RC 预览由用户明确要求；候选与 main CI、L3、Release workflow 均通过，公开 prerelease 和多架构 `preview` digest 已复核；没有生产或 stable/latest 写入。
- 后续应进入的自动门禁或专项工作流：稳定版准入前完成 CF scoped/full audit 和 finding 独立验证；复核 govulncheck advisory；补真实主机/缩放/性能验收；系统不再占用两个空目录后完成物理回收。
