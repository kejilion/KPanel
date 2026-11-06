# KPanel v1.30.0-rc.5 发布验收

日期：2026-11-06；发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.30.0`。

候选提交 / 标签：`1ce188d610d1415e463e9ece44843ee413bb25da` / `v1.30.0-rc.5`。标签对象 `2a17218a829f28da43174e11a51780cb744a1f46`，剥离后指向候选提交。

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。上一预览为 `v1.30.0-rc.4` / `sha256:965197fd6f8d311df3de4997bda8906e6b0908bf79003c2f10ead47bc2bf3063`。

`releaseChannel`：`preview`。

`releaseTrain`：`1.30.0`。

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览版保留。

- 原分支 / 精确 tip / 处置分类：`release/v1.30.0-candidate` / `1ce188d610d1415e463e9ece44843ee413bb25da` / 已发布 RC5，按预览规则保留。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：不适用（单次预览候选不归档）。发布时 `origin/main` 与候选分支均为 `1ce188d610d1415e463e9ece44843ee413bb25da`；远端 `v1.30.0-rc.5` 标签对象为 `2a17218a829f28da43174e11a51780cb744a1f46`。
- 本次来源任务分支：`fix/upload-directory-permissions-20261106` tip `2125235dcf117adf675c052e62b92e4936ca8313` 的功能被重放为 RC5 提交 `c58281d2bf3f433614ad8ffddbf92dcc6ba61066`、`7757fb9581c70ccedf24b879b2ad299031d9991e`；原任务分支保留。其余来源：`feature/site-access-password` tip `e52717b5b030d8c8d1d361d5b3420a0cb7c9e0b7` 与 `fix/monitor-checks-single-page` tip `264525fa5570ace657dac5e38491944b5bfe1fdf` 未纳入，因候选范围未包含其后续改动而保留为本地候选。
- 本地分支/upstream/worktree：RC5 候选工作树 `C:\GitHub\_release-candidates\kpanel-v130` 保留；站点密码工作树 `C:\GitHub\_codex-tasks\kpanel-site-access-password` 与监控工作树 `C:\GitHub\_codex-tasks\kejilion-panel-codex-monitor-checks-single-page` 均保留，未删分支或工作树。验收记录另在 main 后续提交。
- 未完成归档项 / 责任人 / 下次复核触发条件：无 RC5 归档项。两个未纳入功能分支由当前发布维护者保留；下次预览冻结前重新核对是否纳入，并基于当时 main 重放、验证。

## 发布画像

- 业务域：文件管理器中的新建/上传目录权限和批量权限编辑。
- 变更面：文件系统目录创建、权限发布顺序和权限对话框默认值；无数据库迁移或公开 API 变化。
- 受影响用户旅程：受限 Agent umask 下创建或上传目录；在批量选择后查看共同权限，避免将混合权限误填为默认值。
- 未变化契约：端口、Compose、数据库 schema、Agent API 权限、应用市场和配套 `kejilion.sh` 脚本协议不变；RC4 已提供的网站访问密码基础能力继续由绑定脚本支持。
- 风险等级及理由：L3 预览门禁。改动影响 Agent 对用户文件系统的写入权限，涉及不可信可写父目录下的暂存与原子发布。

## 发布范围与未纳入内容

- 用户可见更新：新建/上传目录在受限 Linux umask 下仍可被 Web 服务读取；不可信可写父目录中的新目录先在受保护位置设好权限，再无覆盖地发布；文件权限对话框仅在所选项目权限一致且可识别时预填共同权限。RC5 继承 RC4 功能，但未新增手机终端、AI/MCP 或访问密码交互改动。
- 精确提交清单：`c58281d2bf3f433614ad8ffddbf92dcc6ba61066`、`7757fb9581c70ccedf24b879b2ad299031d9991e`（文件权限）；`6b321c4789a3b6927a52090bee209715fb3bd794`（RC5 版本与说明）；`f0ecd5994612ca42e338f557a59723850cf11749`、`1ce188d610d1415e463e9ece44843ee413bb25da`（精确候选扫描范围身份修正）。完整范围为 `2552735267b858c10c2bf66d3772478022442084..1ce188d610d1415e463e9ece44843ee413bb25da`。
- 明确未纳入的分支、文件或后续事项：站点访问密码核心能力早已在 RC4 基线中；但设置窗口迁移与交互统一的后续提交 `1428e55356c2e3fb62017989e66f4769581a848b..e52717b5b030d8c8d1d361d5b3420a0cb7c9e0b7` 不在 RC5。历史监控服务检测取消分页、分为 Ping/TCP/HTTP 三组的提交 `a16aadfaddf7b456d586a8d7b296f3add5c444b7..264525fa5570ace657dac5e38491944b5bfe1fdf` 不在 RC5；RC5 页面仍保留旧分页。RC5 已公开，不能原地替换这些内容；纳入需进入新的预览候选和门禁。生产部署未授权且未执行。

## 外部审计与修复交付

- 安全盲审补漏（quality-audit review 档）：未找到本候选独立盲审记录；不把自动扫描或非盲代码复核表述为盲审通过。
- `node scripts/security-review-scope.mjs --base v1.29.0` 输出（recorded / missing / stale 包组）：候选流程记录为 `recorded`；L3 来源范围扫描通过。独立范围身份复核文件 `C:\GitHub\_release-evidence\v1.30.0-rc.5\scan-scope-identity-review-r1.json`，SHA-256 `808a55e73bc0f416e8a344dde0e45f50b800f8decfd1884f7af283dda32f6541`。
- finding fingerprint / 修复 commit / 独立复核与回归证据：扫描范围身份未发现产品漏洞。候选 CI 首两次因源码摘要、闭包摘要绑定不一致而拦截；以正式 `productionClosure` 计算方式修正于 `1ce188d6`，同 SHA 候选 CI 与 L3 随后通过。该流程异常见下方明细。
- 修复交付状态：目录权限修复源码与 RC5 已公开；未进入稳定 tag，也未部署生产。RC5 Release 与公开产物地址见下方。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：工具 `1.12.11`；基线 `2552735267b858c10c2bf66d3772478022442084` 到 `3567f4fcd8c82af2a3071a84475e446fc3eedef3`，14 个可审文件全部覆盖，0 findings，H0/M0/L0；审查为非盲，`constrained-only=unreported`。最终 RC5 仅另改 `scripts/torrent-backport-scan-scope.json` 以绑定身份，复核证据另见上项。
- 按 PROJECT_RULES.md 5.5 记录本稳定周期的 OCR 观察结果；不足三个周期不提前宣称工具有效或应退出：本次不作工具有效性结论。
- 候选分支的原始 tip、归档位置和远端核验统一填写本模板既有候选处置字段，遵守 `docs/release-channels.md`：预览分支按规则保留；两条未纳入本地候选也保留。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`（RC4 已发布的网站访问密码依赖配套脚本；RC5 镜像继续内置同一受管脚本）。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：沿用 RC4 验收记录标识 `kpanel-v1.30.0-rc.4-website-access-password`；RC5 未变更脚本。
- KPanel 实际内置脚本基线 commit / SHA-256：`1faee13f3faef164efbfa899e244fc19ad58aaff` / `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：同上；RC5 无新脚本候选。
- 状态判定依据与兼容性证据：L3 与 Release 日志 `managed_script_contract=pass`，并核对公开镜像中的 revision/hash；完整 Panel→Agent→真实 Nginx 主机端到端链路未在本次公开验收执行。
- 本版发布决定：复用已公开绑定脚本；RC5 本身不发布新脚本变更。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 108 项定向 Web 测试；L3 Go/Web；Windows 与候选 CI 均通过 | 用户实际 Agent 主机与挂载文件系统未实测 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 候选安全扫描、脚本契约、镜像 SBOM/provenance 和源码范围校验通过 | 未记录独立盲审；真实主机权限与并发写入环境未覆盖 |
| 稳定性、失败恢复与兼容 | 已验证 | 目录暂存/清理/不覆盖测试；公开镜像初始、重启和升级浏览器门禁通过 | 无长时间 soak 或生产回滚 |
| 性能与资源预算 | 已实现未实机验证 | 暂存仅用于不可信可写父目录，未引入新服务或常驻任务 | 未单独测量大目录上传时的额外 I/O |
| 用户体验与可访问性 | 已验证 | 文件权限默认值及混合权限阻断测试、翻译检查通过 | 本次未做物理手机和 100/125/200% 全视口专项检查 |
| 数据、配置与迁移 | 不适用 | 无数据库 schema 或现有文件权限批量迁移；setgid 与 Windows 继承 ACL 保留 | 实际宿主文件系统差异未实测 |

## 自动门禁

- 定向测试及结果：本机 RC5 日志中 `src/lib/filePermissions.test.ts` 与 `src/views/FilesView.test.ts` 共 108 项通过；4081 条本地化文案完整性检查通过；版本一致性通过；Web typecheck 退出码 0。
- `make verify-release` 环境和结果：本地未直接执行该目标；GitHub-hosted L3 以 `node scripts/run-repo-bash.mjs --env VERIFY_LEVEL=release -- scripts/verify-change.sh` 对精确候选执行并通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run `42237526046` attempt 1，job `127959071568`，成功；GitHub Actions runner ID `1000005086`，Ubuntu 24.04.5 / image `20261101.360.1`。回执 `entrySha256=181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3`；回执未单列 plan/script/bundle 摘要。L3 artifact `13515076162`；证据目录 `C:\GitHub\_release-evidence\v1.30.0-rc.5\l3-artifact-42237526046`，`l3.log` SHA-256 `5da6e714853e288ec6297cd7f984e8c7049895ce3367e0361889094928899b04`。web、go、deploy lanes 均 exit 0；`source/result.json` SHA-256 `6ead40e681295279c0f6b05bed84f4cfe15da820e603906fb273e5b1a94f9e9d`。
- 候选 CI：最终候选 `1ce188d610d1415e463e9ece44843ee413bb25da` 的 run `42236117396` 成功，verify 与 windows-node 均通过。早先 run `42227932703` 与 `42233642208` 的必需扫描均失败，保留在流程异常记录中；未绕过门禁。
- 主线 CI：预览工作流提升 main 后，run `42237526046` 的 main verify 与 windows-node 均通过。
- Release workflow：run `42237526046` 成功；L3、候选 CI、main CI、Release 和公开验收按顺序完成。
- 发布说明门禁：草稿及最终正文版本 `1.30.0-rc.5`；公开验收日志校验 3 条摘要和 1 条升级提示，浏览器显示通过。
- 安全扫描、镜像契约、SBOM/provenance：候选 CI 扫描步骤全部通过；L3 production source scan 扫描 113 个包、exit 0、无保留 findings；受管脚本 revision/hash 匹配；Release 生成并发布双架构镜像及 provenance。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：未单独记录；本版未修改 `go.mod` / `go.sum`，`web/package-lock.json` 仅同步版本字段。
- 最近每日安全通告审计、EOL 复核状态及证据：本次验收证据未单列，记为未验证。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版无依赖变更；相关审计未单列。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无新依赖；L3 使用 Go 1.27.3、Node v24.22.1、Docker 28.0.4，GitHub Actions 采用工作流固定 SHA；镜像含上述受管脚本。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：RC5 锁文件只更新 `1.30.0-rc.5` 元数据；预览 OCI index `sha256:df8cb94c7575b4c2ba9061481cd3f735de7d5308121bd1b96f5f7add91996544`；脚本 commit/hash 见跨仓库联动判定。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选；每日通告与 EOL 证据未记录，发布维护者应在稳定版准入时补核。
- 升级后的兼容、安全、构建、性能资源和回滚结论：构建、候选与镜像验收通过；新建目录保留 setgid 组继承，Windows ACL 继续继承；不修改既有权限。大目录 I/O 影响未量化；稳定回滚点仍为 `v1.29.0`。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Ubuntu 24.04.5，L3 runner image `20261101.360.1`，amd64；公开 OCI index 核对了 linux/amd64 与 linux/arm64 两个平台，容器 E2E 在本次 amd64 runner 执行，未做 ARM64 原生运行。Go 1.27.3、Node v24.22.1、Docker 28.0.4。
- 环境策略 ID 与允许用途：未使用登记的 SSH 真机；GitHub-hosted runner 仅用于仓库 L3 与公开产物隔离验收。
- 使用的精确候选或公开产物：`1ce188d610d1415e463e9ece44843ee413bb25da` 及公开 OCI index `sha256:df8cb94c7575b4c2ba9061481cd3f735de7d5308121bd1b96f5f7add91996544`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `42237526046` / job `127959071568`；公开验收同一 run / job `127988334597`；均成功。L3 source timeout `2700000 ms`，lanes exit 0。命令 `scripts/verify-change.sh`；artifact 与日志摘要见自动门禁。公开测试输出保存在 run `42237526046` artifact `public-42237526046-1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次 release L3 与公开镜像初始/重启/升级旅程；无长周期 soak，因本版只改变目录创建和权限预填逻辑。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：定向测试覆盖权限预填、一致/混合/未知模式和禁止空值提交；公开发布/升级浏览器门禁通过。RC5 未单独记录文件管理器的完整视口、缩放、主题和键盘矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：Linux 测试覆盖私有 umask、不可信父目录暂存、无覆盖发布、清理和 setgid；公开容器初始与 restart 阶段通过。未对真实用户主机写入或执行生产回滚。
- 未执行场景及原因：未连接 `arena-154`；真实挂载卷/自定义 ACL、长时间并发上传、物理手机与生产数据迁移未在本次预览发布验证。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：`1.30.0-rc.5` 与公开 digest 匹配，摘要非空；公开验收 browser gate 检查 3 条摘要、1 条升级提示并通过。Release：[v1.30.0-rc.5](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.5)。

- GitHub Release 的 draft / prerelease / Latest 状态：`isDraft=false`、`isPrerelease=true`；GitHub Latest 仍为 `v1.29.0`。
- Docker 版本与通道 OCI index：RC5 和 `preview` 为 `sha256:df8cb94c7575b4c2ba9061481cd3f735de7d5308121bd1b96f5f7add91996544`；稳定 `latest` 仍为 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:4d39d21095a1337b7a8b7c52c1226f3b79437536fcc9ecd98c1c31deb276cced`；arm64 `sha256:adbf74db64eac8b37aaa689607ba6d3d4f27629870d1f049c4b6e136195aba0d`。
- 附件及 `SHA256SUMS`：19 个附件；`SHA256SUMS` 有 15 个校验条目，文件摘要 `sha256:c92321d36cc37095f6f9f9c187e66992270f5a416ebcb68ccb1e26ead2aba440`；公开附件、校验和与来源标签检查通过。
- 公开镜像 `image_e2e=pass`：初始阶段 48 项、重启阶段 26 项；published browser 45 项、upgrade browser 31 项，均通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：apps 默认稳定入口未变；镜像受管脚本 revision/hash 与 `1faee13f3faef164efbfa899e244fc19ad58aaff` 匹配。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：公开接口确认 stable=`v1.29.0`，preview=`v1.30.0-rc.5`，版本与 digest 对应。
- 加入预览只切换来源并立即检查，没有自动安装：公开浏览器流程通过；本次无生产主机安装。
- 自动安装开关与一次性立即安装相互独立：公开升级浏览器 gate 通过；生产环境未执行。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：公开容器 restart 与浏览器状态检查通过；真实宿主跨重启未验证。
- 退出预览且稳定版较低时没有产生降级候选：升级浏览器 gate 通过；未在真实宿主执行。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：生产 systemd 主机未验证。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：公开 Release 的兼容说明沿用既有边界。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：GitHub-hosted L3 与公开产物隔离验收；未连接登记的生产主机。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：不适用；未连接 `arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：未运行。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：未验证（无生产部署）。
- 生产已执行写操作：否。
- 仅在隔离真机执行、未在生产执行的场景：GitHub-hosted L3、双架构 OCI 清单核对、amd64 容器和公开浏览器验收。

## 回滚

- 源码/tag：RC5 保留 `v1.30.0-rc.5` → `1ce188d610d1415e463e9ece44843ee413bb25da`；上一预览 `v1.30.0-rc.4`；稳定回退点 `v1.29.0`。
- 镜像 digest：RC5 `sha256:df8cb94c7575b4c2ba9061481cd3f735de7d5308121bd1b96f5f7add91996544`；稳定 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- 数据/配置备份：未触及真实生产数据；用户安装前仍应备份 Panel、Agent 数据和自定义宿主脚本。
- 回滚步骤和回滚后复核：退出预览不会自动降级；如需恢复稳定版，按 `docs/release-channels.md` 手动切回并检查数据。未在生产执行。
- 回滚后生产实际版本与健康状态：不适用。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：仍为 `v1.29.0` / stable。
- 公共默认更新通道决策：不适用（显式加入预览，稳定默认未变）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-06T18:03:20+08:00
- 候选冻结时间：2026-11-06T22:40:36+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（RC5 产品产物仅公开一次，未回滚或紧急热修复；两次发布前候选 CI 失败单列于流程记录）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/scan-scope/stale-source-digest",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI run 42227932703 在必需的源码/依赖/机密/配置扫描步骤因 first-party source 摘要仍绑定旧源码而失败；未提升 main、未创建公开产物。",
    "recoveryEvidence": "run 42227932703 失败；随后补绑 source 摘要。run 42233642208 仍因独立的闭包摘要错误失败；最终精确候选 1ce188d610d1415e463e9ece44843ee413bb25da 的 run 42236117396 两个作业均通过。",
    "permanentAction": "本候选保留 source manifest 与 source 摘要的独立复核证据；当前通用的提交前摘要预检尚未覆盖此类漂移。Release Maintainer 在下一候选扫描范围准备前增加 source-manifest 预检，并用旧摘要夹具验证必需 CI 会在发布前拦截。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/scan-scope/incorrect-closure-digest",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI run 42233642208 因手工计算的 production closure 摘要不匹配官方生产闭包函数而失败；未提升 main、未创建公开产物。",
    "recoveryEvidence": "run 42233642208 的必需扫描步骤失败；提交 1ce188d610d1415e463e9ece44843ee413bb25da 按官方 productionClosure 函数核对后，run 42236117396 的候选 CI 通过，后续 L3、主线 CI 与公开验收均成功。",
    "permanentAction": "不再手工计算 closure 摘要；下一次候选预检由 Release Maintainer 调用官方 productionClosure 并验证 source/closure 两种错误夹具，完成条件为预检可在 push 前稳定拒绝错误摘要。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：未删除候选分支、工作树或证据；保留 RC5 发布证据和未纳入的两个功能分支。未做磁盘清理，无净释放量。
- 未验证风险：真实挂载卷和自定义 ACL；不可信父目录中持续并发写入的主机级行为；长时间负载；真实 systemd 升级回滚。
- 已实现待实机准入：目录权限与无覆盖发布的主机级跨平台验收；站点访问密码完整 Panel→Agent→脚本→Nginx 真实主机链路仍沿用 RC4 边界。
- 不阻断本版的理由：本版是显式 opt-in 预览；双架构镜像、CI、L3 和公开验收通过；稳定默认未变，且生产未部署。
- 后续应进入的自动门禁或专项工作流：下一预览冻结前核对来源候选清单，明确每条本地分支纳入或排除；若纳入站点密码交互和监控去分页改动，重放至当前 main、重新验证并发布新的 RC，不修改 RC5。
