# KPanel v1.24.0-rc.6 发布验收记录

日期：2026-10-02

发布级别：L3

候选提交 / 标签：`0a16722239495e5e37399decf31c6579d998bff7` / `v1.24.0-rc.6`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / OCI `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` 保留，同序列继续使用；唯一产品工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly`，本地与远端产品 tip 一致。

## 发布画像

- 业务域：Go 编译器、构建基座、发布 Runner 和依赖检测。
- 变更面：构建供应链与测试/检测工具。Go 运行时代码仅发行版本元数据改变；没有业务逻辑、UI、API、数据库、端口、Compose、Agent 权限或宿主机脚本协议变更。
- 受影响旅程：AI 附件 JSON、网络与认证、定时器、应用安装/更新、中断恢复及备份一致性；编译器变化需要完整发布验证，不能仅用源码不变推断行为一致。
- Node 24.21.0 LTS、TypeScript 6.0.3、Go 模块和内置脚本保持原基线。风险等级 L3；预览版禁止生产部署。

## 发布范围与未纳入内容

- 批准基线 `dc5c29f18873a848c5e99aa31af09cb126d8bc71`。Go 来源 `feature/base-202610-go127` / `ba5ecf1fbd9e91e74544871b9d79ce3145f89ff3`，7 个提交：`80dd5c42`、`ecc3c84b`、`de753c33`、`62f591bf`、`af60a62e`、`47f4cc4b`、`ba5ecf1f`。
- `d79fb672` 集成来源；`64abafe4` 同步版本和发布说明；`0a167222` 仅记录复核 trailer，tree 与 `64abafe4` 相同。来源 15 文件与最终候选逐字相同，原件 `source-integration-equivalence.json`。
- 用户可见更新：Go 构建工具链 1.27.1；Go 新版本检测不再漏识别官方 `goX.Y.Z`；JSON 旧实现对照测试适配 1.27，生产深度限制、无部分成功、输入不变和分配限额保留。
- 未纳入 Node 26 `fab9ff33`（Current 实验）、TypeScript 7 `f124abf4`（双编译器试验）、治理 `eead4d24`、生产部署、冻结后的新功能和 CF 审计执行。以上独立候选保留。
- 三个 Claude 远端 tip `32013251`、`64422127`、`f3899ad7` 已是批准主线祖先，不重复合入，也不凭名称/时间删除未知所有权分支。

## 外部审计与修复交付

- 安全审计覆盖：`decision=scoped-required`，未审计提交 55、最老 5 天、未审计文件 98；新增边界包 `internal/backupremote`，last full run-4 source `4c0694aa8e02`、10 天。RC 只记录；稳定版前补审，未声明 CF 已通过。
- 来源独立复核链已关闭原 P2“Runner 回归未接入治理门禁”；Go 检测和 JSON 对照测试增量均 PASS。原件 `C:/GitHub/_validation/kpanel-base-202610-go127/independent-review*.json`；最终来源 L2、竞态、扫描和日志摘要已核验。
- 复核限制：来源与发布任务均 Codex，复用已披露的替代 provider CLI 不可用、独立干净会话 fallback；发布任务与主要实现任务不同，没有召回旧 writer 或新增监工。
- OCR 1.12.11：精确范围 `dc5c29f1..64abafe4`，17/17 reviewable 文件完成；3 个排除为文档/Go manifest/锁文件且人工覆盖。自由臂先落盘，无新增阻断；已知来源规则/结果，`blind=false`、`constrained-only=unreported`，不计有效盲测周期。未修改圈选规则。
- L3 中缺少 `origin/main` 时既有治理脚本跳过安全审计记录的 push-ordering 机器下限；本范围没有新增审计记录/详细 findings，已单独核对，未把该分支当成完整 CF 覆盖证据。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`，无需发布脚本（不适用）；变更集编号：不适用（Go 来源追踪 `base-202610-go127`）。
- 实际内置脚本基线：`8bebc2d80614e96b844c2c5f88acb0a81d4abd10` / SHA-256 `578b9e4328ba231ad08783c3f2007034f332621d48db07cffe3429da807940b4`；新脚本候选：不适用。
- 差异不新增或改变脚本协议、运行时动作、宿主产物、安装/更新路径、外联配置或内置脚本字节；源码、最终镜像契约及实际公开镜像字节均通过。
- `kpanel.conf` 三方归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；本地、公开 apps 仓库和实际镜像相同。无需 apps 或 sh 提交，不修改默认更新入口。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 全量 Go、2054 前端测试、脚本契约、隔离应用生命周期和公开镜像 E2E | 未新增完整宿主原生双端安装/更新/卸载旅程 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、Trivy 源码/镜像 HIGH/CRITICAL 门禁、双架构 OCI 与 SBOM/provenance 绑定 | CF 补审仍待完成；不称整个依赖图无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | 三核心包 race、隔离中断恢复及备份一致性、镜像运行时契约 | 无长期 soak、生产部署或 arm64 实机运行 |
| 性能与资源预算 | 已验证 | 来源 5 次短样本与 Runner 资源保护；Release 原生镜像 256MiB/1CPU/128PID 约束健康检查 | 单机短样本，非显著性/生产性能结论；二进制总大小 +2.69%，通知分配 +7.47% |
| 用户体验与可访问性 | 不适用 | 本轮无 UI 源码变化，自动前端/类型/i18n/场景重现通过 | 不复用旧浏览器结果宣称新候选实机浏览器验收；未新建 acceptance 预览 |
| 数据、配置与迁移 | 已验证 | 深度拒绝、nil 结果、原输入不变和内存分配防线保留，隔离备份 parity 通过 | 无 schema/协议迁移；无生产数据修改 |

## 自动门禁

- 31 项工具链/Go 检测聚焦回归、3 份 GitHub workflow YAML 唯一键解析、版本一致性、clean candidate 检查通过。
- 固定 L3 外层入口 `scripts/run-release-l3.mjs`，run `v1.24.0-rc.6-0a167222-l3-r1`，`2026-10-01T17:32:44Z` 至 `2026-10-01T17:50:30Z`，`status=passed/exit_code=0`。
- Runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1、Node 24.21.0、npm 11.19.0。WSL 导出和 arena 导入 ID 不同，但归档摘要、rootfs diff IDs、基座 labels 和工具版本一致；先确认目标 ID 再冻结，本次未因此重跑 L3。
- bundle `17752202ff6156a2cae63ee1dcd69cb9736f25ed1c13219ac65c6d475f1a378a`，plan `fdb740ae23b7354389b012631f811c03114112cbe67def4a081b9b34c26d3664`，执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，manifest `455d242dd364e8c2cb3216353a7c8f0b1758dc1616f2578dd085d5475a8fd87f`；12 项远端原始证据回收后逐项 hash 相同。
- 全量 Go/vet、227 文件/2054 前端测试、三特权核心包 race、3414 phrases/22 catalogs、3 场景重现、类型检查、生产构建、10 个双架构二进制、源码/镜像扫描、源码/镜像脚本契约、app-conf lock/lifecycle/中断恢复/备份 parity 通过。
- 来源 CI `36899225905`；候选 CI `36902692382`；产品主线 CI `36904058866`；来源/候选/主线依赖检测 `36899225974` / `36902692269` / `36904059234` 均精确 SHA success。
- Release workflow `36905723563` success；原生镜像非 root、只读、network none、cap-drop ALL、256MiB/1CPU/128PID 约束及健康检查通过。
- 原件主目录 `C:/GitHub/_release-evidence/v1.24.0-rc.6`；L3 kit/完整原件 `C:/GitHub/_validation/kpanel-v124-rc6-l3-r1`，远端 `/root/kpanel-release-evidence/v1.24.0-rc.6-0a167222-l3-r1`。

## 依赖与技术栈变化

- Go 1.26.7 → 1.27.1；Docker/Runner/CI/现有真机工作流一致固定。先前来源 1.26.8 是局部兼容检查点，不冒充完整 L2；失败原件保留。
- `golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414`；Bookworm 工作流固定 `sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195`。Runner Dockerfile 与来源 SHA-256 `3685e83dd07595ea5d5d560d24fd5bd03d0047516ae3948533cc2e3c9a280839` 相同。
- 来源新鲜度报告 `2026-10-01T16:43:02.713Z`，10/10 检测源、152 候选（3 行动项、149 传递信号）；来源和最终产品 SHA 的远端新鲜度 CI 成功，未虚构新的单次候选统计。
- TypeScript/@types-node 两项现有有期限例外继续由 KPanel base maintenance 于 2026-10-15 复核。Node 26 与 TS7 本地实验不等于本版采用。
- Trivy 0.75.0 是后续独立 qualification 行动项，首个完整检测 `2026-10-01T15:47:46.958Z`；负责人 KPanel base maintenance，最晚 2026-10-15 启动、2026-10-31 决策、2026-11-30 完成处置，不重置首次日期。尚未作采用/拒绝/暂缓决定，若后来暂缓须建立 policy 正式例外；本版保留 0.74.0 和原门槛。
- 来源完整性 `go mod verify` / `go mod tidy -diff` 通过，go.sum/模块版本不变。模块级 `GO-2026-5932` 涉及未导入的 `x/crypto/openpgp`，来源详报与默认可达扫描区分，未加忽略；本次 L3/CI 默认可达扫描零受影响代码漏洞。
- 复用源码短基准，Go 及对应 Alpine 补丁同时变化；没有将局部耗时改善归因到生产性能。长期观察未执行；未建立自动监控。

## 隔离真机与浏览器验收

- 登记环境 `arena-154`，候选验证与隔离故障注入用途；主机 `Linux 6.12.107+deb13-amd64 x86_64 GNU/Linux`，Docker `29.6.2`，固定 Runner rootfs Alpine 3.24.2。
- 原 Panel 仅健康读取，保持 healthy；没有执行生产升级、重启、备份或部署。L3 生命周期日志中的 `/home/docker/kpanel` 是隔离测试容器 rootfs，不是现存业务数据。
- 实际公开镜像在 linux/amd64 上运行固定 `packaging/tests/image-e2e.sh`，仅回环 18086、自有容器/网络/临时数据，`image_e2e=pass`；验证启动、版本、真实静态资源 bytes、bootstrap、反向代理 Host/Origin 与 Secure Cookie、健康及清理。
- 本轮没有 UI 变化，浏览器多视口、缩放、字号、键盘和语言矩阵不适用；不能把 rc.5 mock/GPU 结果当成新 Go runtime 实机证据。没有 arm64 真实执行、原生宿主全生命周期或长时间 soak。

## 发布产物与公开仓库复核

- GitHub `v1.24.0-rc.6` 于 `2026-10-01T18:30:08Z` 公开，draft=false、prerelease=true、非 Latest；Latest 仍 `v1.23.0`。
- 版本及 `preview` index `sha256:4deadfed891d113d8ec3886a6a0b4ae15a191a226b152c840dd10d40c009ac62`；`latest` 仍 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。
- linux/amd64 `sha256:7e1abc01ee0fc7f2cf7650b469e6ee2c335b71ef6abc991a05bf755a97bcb636`；linux/arm64 `sha256:dd667018ca74ec0702cbf49d04c560bca27312e882da3a4623013feb6aa1aabe`。两个实际 registry manifest/config 字节 hash、版本/revision、USER/entrypoint 和脚本 labels 均验证。
- 14 附件完整；下载 SHA256SUMS 与 metadata archive 校验实际字节，11 条 SUM 与 GitHub asset digest 相同；10 个 metadata 文件与精确 Git blob 逐字相同。其余独立二进制未全部下载/执行。
- 两个架构均有 SPDX 与 SLSA provenance/v1，实际 blob 摘要、predicate 和各自 subject 绑定验证；不等于完整 SBOM 内容安全审计。
- 实际公开镜像 `/release/kejilion.sh`、`kpanel.conf`、VERSION 字节验证，临时从未启动的 bytecheck 容器及临时文件已删除；公开 E2E passed。apps/sh 无写入。

## 自更新通道验收

- 现有 stable/preview 选择、语义排序、非法版本 fail-closed、通道切换清旧候选、退出 preview 不自动降级、开关与立即安装互不越权的自动回归通过。
- apps 默认入口仍 stable/latest；公开 Latest/latest 实际指向 1.23.0，本版只提升 preview。退出 preview 不承诺自动降级。
- systemd 更新备份/失败恢复及版本隔离由隔离 app-conf 生命周期验证；未在现存宿主安装执行。OpenRC 完整 KPanel 自动更新仍不支持，轻量 Node 无人值守更新仍只跟踪 stable。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）。本轮没有生产写入，生产数据/备份/部署后版本核对均不适用，隔离验证不替代生产证据。
- 验证环境 arena-154；prod-108/108 禁用全部 KPanel 操作，本次未连接、备份、部署、升级或核对。

## 回滚

- 上一个预览 `v1.24.0-rc.5` / `35498116b8b325dded78ee006b1dac08c6744c37` / OCI `sha256:6f9155d3025fcfc25d48bdacda77e8bcde5e828428f82e228dcf98db2e7b1956` 保留。
- 公共稳定默认仍为 v1.23.0 / 上述 latest digest；无需生产回滚或备份恢复。本轮无 schema/协议变更；未来人工回滚须显式目标、备份和独立验证，不用通道开关代替。
- 共享 main 不改写历史；Go 来源完整提交集、不可变 RC 标签和已核验 L3 bundle 可恢复。若以后撤回 Go 升级，用聚焦 revert 并重验，不 reset main。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T00:12:03+08:00
- 候选冻结时间：2026-10-02T01:31:30.460+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

RC 不计入正式稳定标签发布频率；4 次预检参数/取证失败均发生在生产写入前，已纠正，没有放宽测试或改动冻结产品。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "impact": "Initial convenience health read used nonexistent name kpanel; no service mutation",
    "permanentAction": "Release owner uses inventory/exact identity for subsequent checks",
    "position": "before-production-write",
    "fingerprint": "preflight/ssh-health/guessed-container-name",
    "recoveryEvidence": "Read exact retained container7655193b...: healthy",
    "count": 1,
    "historicalReleases": []
  },
  {
    "impact": "PowerShell interpreted docker format and Git tree suffix as script blocks; reads failed, export and product source unaffected",
    "permanentAction": "Freeze plain inspect JSON and quote Git brace expressions; subsequent L3 uses repository process-argument arrays",
    "position": "before-production-write",
    "fingerprint": "preflight/powershell-native/unquoted-brace-arguments",
    "recoveryEvidence": "runner-wsl-inspect.json and quoted tree comparison both873f4e89",
    "count": 2,
    "historicalReleases": []
  },
  {
    "impact": "Public E2E environment preflight used unsupported --target; checker rejected before SSH or service writes",
    "permanentAction": "Read CLI parser and use --environment arena-154 --purpose candidate-validation",
    "position": "before-production-write",
    "fingerprint": "preflight/environment-policy/unknown-cli-argument",
    "recoveryEvidence": "environment-policy-public.log: environment_policy=pass role=hybrid purpose=candidate-validation",
    "count": 1,
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 候选、来源与资源收尾

产品候选精确 tip `0a16722239495e5e37399decf31c6579d998bff7` 保留，本地/远端一致；同序列 RC 不归档候选。
Go 来源原 tip `ba5ecf1fbd9e91e74544871b9d79ce3145f89ff3` 已核对纳入本版、作者释放所有权，并以两端 expected-SHA lease 原子归档为 `archive/feature/base-202610-go127` 同 SHA；远端 active ref 已不存在。原 clean 来源工作树和历史证据保留，local source 无 upstream，不算待发布候选。
自有 L3 工作目录、inbox 与临时上传 Runner tar 已在 12 项原件摘要、公开 E2E、Git clean 和挂载排除核验后删除并确认不存在；逻辑删除量 `1402986496` bytes，磁盘空闲实际从 `24864280576` 到 `26267271168` bytes。Runner image、原始远端证据、本地 tar/bundle 和历史失败原件保留，无全局 Docker prune。
原 18 个业务容器身份、镜像及运行/退出状态与开始快照一致，原 Panel 仍 healthy；既有 `arena-brawl-bots` unhealthy 状态保留，不宣称所有容器健康。
纯验收记录候选 `docs/release-v1.24.0-rc.6-acceptance` 将按同 SHA Linux CI、主线快进、主线 CI 顺序完成后归档为 `archive/docs/release-v1.24.0-rc.6-acceptance`，不为文档再发版本；精确文档提交/CI/归档结果保存于 `C:/GitHub/_release-evidence/v1.24.0-rc.6/closeout.json`，公开标签与产品候选保持产品 SHA。
未知所有权分支、本地未集成 Node/TS/治理实验、历史原始证据和可恢复 bundle 保留。
复用发布 3.4、版本管理和 OCR 现有入口，未新增仓库工作流、PR、会话或自动任务；仅按现有规范保存本版本验收记录。
