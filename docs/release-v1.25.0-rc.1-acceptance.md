# KPanel v1.25.0-rc.1 发布验收记录

日期：2026-10-04（北京时间）；发布级别：L3。

候选提交 / 标签：`4f4996da03523a7d0dd788a28e0b57e9decaf045` / `v1.25.0-rc.1`。上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。

`releaseChannel`：`preview`；`releaseTrain`：`1.25.0`。

候选分支与发布后处置：`release/v1.25.0-candidate` 保留精确发布 SHA，RC 期间不归档。产物已发布，生产未部署。

## 发布画像与范围

- 用户授权：“好了 整理现有其他候选分支 发布新预览版”。先冻结范围/非目标、工具、固定 Runner、权限和回滚；实现与发布全部使用独立 linked worktree，主工作树只管理同步。
- 产品行为沿用 v1.24.0，保留其图库/文件/WebSSH 等累计功能。本轮没有新业务功能、UI、API、数据、权限、端口、Compose 或安装协议变化。`git diff v1.24.0` 对产品目录、部署/打包、依赖策略及 Dockerfile 为零，开发态版本号除外。
- 新增范围：已在 main 的有界并行源码检查、任务预检与复核 SLA 治理，以及重基并重新资格化的业务基座/组件升级验收映射；版本身份与 Changelog。新工具不代表产品运行时提速。
- 精确增量：main 治理 `c91fc78414bf4cec757af431e347a09e68c02f64` / 记录 `df79f1a2cb87aa2e222e18e42a4a666fb8e26f5c`；重放 `9dc3672d1e3a617bfbe610fdedd37b2b3cf0f01c`、`92b04575d9172b58ab3d50094bfc88b04c12f73a`、`336a6c64a3551e987dc5ea1ae33f3d0fc3e21440`；发布身份 `6a5441c74460e88da8e4bf93bd7c4d2ce65d1b4a`；一行复核说明修正 `4f4996da03523a7d0dd788a28e0b57e9decaf045`。
- 风险：发布产物 L3；业务基座静态检查/规范增量 L1；无新的业务载荷。六维规范复核通过，安全/EOL、期限、L2/L3 与权限下限没有降低，采用观察窗口尚无真实样本。

## 现有候选筛选与处置

复用一次盘点 `C:/GitHub/_validation/kpanel-preview-20261004-branch-inventory.json` 的 43 条非 archive local/remote 记录，逐项读取独有提交、路径、containment 与 patch equivalence，没有再次全仓扫描。

| 来源 | 本轮决定与证据边界 |
| --- | --- |
| business-base `dbf58e9fb7b1cf84e10f98d5b7891768f8723e1a` | 三提交重放、六文件增量纳入。原 2026-10-02 gate 232/233 FAIL 与未获推送授权保留；旧日期夹具问题已由当前批准 main 修复。本轮新治理 248/248、L3 与精确 CI 通过，不追认历史 PASS。 |
| governance-status `eead4d24` | 相关源码已由最新 main 重新采纳；旧提案/基线不再整枝合并。 |
| security-audit-run2 `bbc6cf92` | 历史本地账本保留，不覆盖当前同名 run，不公开拼接为新覆盖。 |
| OCR `3b1bc710` / OpenWrt telemetry `21240263` | patch 等价已纳入 main；不重复合并。 |
| light-node-dependencies `40c7ca56` | 暂缓。61b230 是 c981 的后继，但公开 GitHub commits API 两次核读仍 422 No commit found；本地对象不等于已发布。主动消费 pin 需 coupled 先行脚本发布、ROOT/CN 与 smoke/embedded 三 pin 兼容，不能单独改 source.json。本轮不写 sh。 |
| Node26 `fab9ff33` | 暂缓，官方 schedule 的 LTS 日期为 2026-10-28；非默认 LTS 实验不替代 Node24.21.0 发布基线。 |
| TS7 `f124abf49e0003bb2bffd5ea4c930b43a4543d13` / dual `de377f9da4c55be52815dca7329389f235341906` | 原记录为实验、非采纳/发布批准；用户此前已决定“那这个就归档吧”。dual 精确保存 `archive/feature/ts7-dual-benchmark-20261002`，原 active 远端不存在、本地作者 tree 保留。匹配 Alpine build+scene 中位数 23.786→22.173s、依赖 +29.05MiB、安装 +4.7%；不能声称网页/API 或整体发布性能提升。 |
| Windows design `b4e84aac` 及 node-platform `34169bd3`、runtime `98b08039`、RDP `235fb0de`、UI `d2b28dda`、聚合 `8554b904` | 暂缓。最新聚合 39 提交/199 路径；缺签名真实产物、原生 install/update/rollback/uninstall 与 RDP 准入；矩阵 ✓ 是要求，不是结果。没有盲合聚合枝或用交叉构建替代 Windows 实机。 |

以下八个旧 active origin refs 均已在 v1.24.0 交付，执行前原 tip 未变、归档 ref 不存在；双 expected-SHA lease 原子保存 archive 并移除原远端 refs，随后逐项重读通过。原 native 本地分支/upstream/worktree 保留，五个 README 作者 tree clean 且 tip 相同；归档不表示本轮生产上线。

| 原分支 | 精确 tip | 归档 ref | 处置 |
| --- | --- | --- | --- |
| `claude/awesome-clarke-pdzdcz` | `32013251c36c5738b04070897897662878328642` | `archive/claude/awesome-clarke-pdzdcz` | v1.24.0 已交付；原SHA远端核验通过 |
| `claude/files-dual-pane` | `64422127f2fc74fd27858c799a106c1a6081a2f0` | `archive/claude/files-dual-pane` | v1.24.0 已交付；原SHA远端核验通过 |
| `claude/upbeat-curie-x2ei28` | `f3899ad7f39d31fa9d71794544055225c01bc326` | `archive/claude/upbeat-curie-x2ei28` | v1.24.0 已交付；原SHA远端核验通过 |
| `docs/readme-deduplicate-20261002` | `c1e0aa103cb8512460cc232ad3658bec93634be4` | `archive/docs/readme-deduplicate-20261002` | v1.24.0 已交付；原SHA远端核验通过 |
| `docs/readme-desktop-hero-ai-20261002` | `887a44023b9c8949d80d9f3a38fb257d1385f923` | `archive/docs/readme-desktop-hero-ai-20261002` | v1.24.0 已交付；原SHA远端核验通过 |
| `docs/readme-dual-mode-hero-20261002` | `386ae5791f10d565e2174f566553a964c4936fc1` | `archive/docs/readme-dual-mode-hero-20261002` | v1.24.0 已交付；原SHA远端核验通过 |
| `docs/readme-screenshots-20261002` | `642cd059b6d070f1496a07c96040683f2e265974` | `archive/docs/readme-screenshots-20261002` | v1.24.0 已交付；原SHA远端核验通过 |
| `docs/readme-tech-hero-no-logo-20261002` | `dc5c29f18873a848c5e99aa31af09cb126d8bc71` | `archive/docs/readme-tech-hero-no-logo-20261002` | v1.24.0 已交付；原SHA远端核验通过 |

原始 source 分支、延期候选与其它已 contained 的作者树不改不删。未准入项由后续各来源责任任务补实证后重新资格化；不是已验收待发布。RC 列车保留，验收记录 docs 候选同 SHA CI/main CI 后归档，收尾结果附在仓库外本版本 evidence。

## 跨仓库联动与更新通道

- `scriptLinkageState=not-required`：无需发布脚本（不适用）；变更集/脚本候选不适用。本次不升级受管脚本、不写 sh/apps。
- 内置脚本 `c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`。ROOT/CN 公开 raw 字节分别核对，归一化只允许 区域赋值 default→CN 的两行增删差异；正式 L3 静态 pin 与应用生命周期通过，公开镜像实际 `/release/kejilion.sh` 字节也核对。
- apps 打包/本地/公开 `kpanel.conf` 归一化 SHA256 均 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，本地 clean，同契约无需提交，默认 `latest`。
- 稳定来源仍只选择正式 Latest；preview 来源允许规范 RC，版本/通道/digest 校验由现有 Go 测试覆盖。本轮没有再次声称 systemd/OpenRC/Node 原生升级实机；沿用已验证产品契约，不把 Mock 当业务安装证据。
- 加入 preview 只切来源并检查，不自动安装；安装开关与立即安装独立；旧状态默认 stable、退出 preview 不形成降级候选。更新源码相对 v1.24.0 无变化，新 L3 测试通过；OpenRC/轻量 Node 边界不扩大。

## 安全与 OCR

- 覆盖检查（RC 只记录）：`decision=ok`，未审计提交 `0`，精确 target `4f4996da03523a7d0dd788a28e0b57e9decaf045`；沿用完整 run4 和其后 scoped 证据，本轮没有启动或声称完成新的 CF scoped/full 审计。已确认修复按当前源码保留；source-only 风险和未验证部署状态不转为通过。
- OCR 1.12.11：自由臂先落盘，随后 preview/rule；五个适用文件约束臂覆盖 5/5，H0/M0/L0、constrained-only0。Markdown/lock 的排除与自由臂复核明确记录；后续 amend/一行 Markdown 修正不改五个适用 blob，最终 SHA 的逐文件 blob-map 全等。观察结果不证明工具收益或替代 CF。
- 独立接管 owner 与原开发任务分离，实际读取原始失败、规范、六文件差异与组件入口。提供商 codex/codex，替代非交互 CLI 不可用，按规范记录 provider-unavailable；未召回旧作者或新建第三个监督代理。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 新 L3 251 文件/2256 前端测试、Go tests/race/vet、应用生命周期/备份回滚夹具；业务 source 与 v1.24.0 同 blob。无新增原生业务安装旅程。 |
| 网络入侵与供应链 | 已验证 | Go/npm/Trivy source/image 门禁通过；OCI 双架构标签、SBOM/provenance subject/bytes 绑定；不代表新增 CF full 或完整 SBOM 内容人工审计。 |
| 稳定性、失败恢复与兼容 | 已验证 | fail-closed 的 r1/r2 原始失败与取消保留；固定标准入口重试成功，备份/恢复生命周期通过；不代表 WAN/长期 soak/真实路由器。 |
| 性能与资源预算 | 已验证 | 两 lane 有界、输出/超时契约测试；实际源码 wall 和 Release step 见下。采样 899.7MiB 是一次 Runner 样本，不是峰值或产品预算。无新 runtime 性能收益。 |
| UX/可访问性 | 不适用 | 无 UI/交互差异；v1.24.0 的原生/Mock/字号与 12px 图库按钮遗留差距作为历史边界保留，本轮没有新浏览器实机声明。 |
| 数据、配置与迁移 | 不适用 | 无业务 schema/迁移变化；新镜像 E2E 与现有故障恢复夹具通过，不代表生产迁移。 |

## 自动门禁与隔离验收

- 首轮 L3 `6a5441c74460e88da8e4bf93bd7c4d2ce65d1b4a`：02:26:28→02:26:54Z、26s、FAIL；248 tests 通过后 health 拒绝 provider 说明字段。修正一行产生最终 `4f4996da03523a7d0dd788a28e0b57e9decaf045`，重新绑定所有证据。
- 第二轮同 `4f4996da03523a7d0dd788a28e0b57e9decaf045`：02:32:49→02:33:34Z、45s、FAIL；npm ci ECONNRESET16.018s，Go取消、deploy未启动，identity不变；不能拼接为通过。网络只读200后，同固定入口/Runner/参数重跑，无代理或缓存绕行。
- 第三轮：`v1.25.0-rc.1-4f4996da-l3-r3`，`2026-10-04T02:36:36Z`→`2026-10-04T02:51:22Z`，886.0s，exit0/PASS。source wall579419ms，Web360304/Go579364/deploy3891ms；governance248/248。首轮 kit 准备到第三轮结束 02:26:00.629→02:51:22Z 是25分21.371秒，包含返工/准备/协调，不能只报成功轮为全部发布耗时。
- 固定环境 `arena-154` / candidate-validation；Runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Linux amd64、Go1.27.1、Node24.21.0。L3 唯一 entry `scripts/run-release-l3.mjs`，未自造 L3 wrapper；Windows Bash 经 run-repo-bash，未落 WSL。
- bundle `8e14976ce4c8003d42246cf13a40ba1ad51332a669010c5a99ebc5b41505bd91`；plan `13e2cfb14843960c4c3b26cde4c88289136127bca9eae62f54d866dfd3dca07d`；remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；12 个远端 evidence checksum 全核对。task handoff exact candidate/log/environment/tools/argv qualification PASS，不能理解为新增授权。
- 候选 [CI](https://github.com/kejilion/KPanel/actions/runs/37172445305)、[main CI](https://github.com/kejilion/KPanel/actions/runs/37172863757) 均 exact `4f4996da03523a7d0dd788a28e0b57e9decaf045` / push / success，分别先于 main 快进和 Tag；对应 Dependency freshness 均 success。
- [Release workflow](https://github.com/kejilion/KPanel/actions/runs/37173388387) exact `4f4996da03523a7d0dd788a28e0b57e9decaf045` success，安全扫描、双架构构建/发布、附件与供应链产物完成；不是生产部署。
- 新公开镜像仅在隔离 arena-154、loopback18089、固定 `packaging/tests/image-e2e.sh` 执行 `image_e2e=pass`；原始日志/commands/hash/起止保留，owned container/network/tmp均回收。arm64 实机、原生 Node/procd、Windows、WAN/长时 soak 未执行。

## 公开产物与效率实测

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.1) 于 `2026-10-04T03:25:39Z` 公开，draft=false、prerelease=true；GitHub Latest 仍 v1.24.0。
- 版本/preview OCI index `sha256:e3d0b3543f014e84abed356ac590fb36c161d9dad6524eb4d96f7d6b3585f316`；amd64 `sha256:b530cbb4b5ddaca85a2d4738991aae3fd0f0823902f8b60e48668ca23cc28f43`；arm64 `sha256:878a427c5a35e048939cfa13c4e7e02480d72ad7e93c9604709bb2c4b47173a5`。每个 manifest/config 实际 byte digest、版本、revision、script 标签、User65532:65532/Entrypoint 核对。
- Docker latest/1.24.0 均保留 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`，仅 preview 前移；两个架构均有 SBOM/provenance predicate 且 subject 为对应镜像，逐 blob 摘要核对。
- 14 附件、SHA256SUMS 11 产品项逐一与 GitHub digest 一致；实际下载 sums、metadata、amd64 Node 验字节，metadata10个关键文件与 exact candidate git blob 相等。其它二进制仅下载元数据摘要核对，未逐一原生执行。
- 本轮 GitHub Release `Verify source` 417.0s，对照上版同平台449s，单次减少 7.1%；release job 825.0s（上版795s），增加30s / 3.8%。同固定 Runner 的成功 L3 为886s，对照上版1069s，单次减少183s / 17.1%。RC与stable及缓存/网络不同，只报告步骤观测；整体大幅提速未证实，仍须保留本轮两次失败和25分21.371秒的准备/返工总时段。46.2%仅夹具基准，TS7实验收益未采用。L3冷Runner579.419s不与GitHub449s混比。

## 依赖与技术栈变化

- 本轮没有组件/工具链/镜像/Action/扫描器/受管脚本采用；Go1.27.1、Node24.21.0、TypeScript6.0.3、Vue3.5.43、Vite8.3.2及精确lock/Action SHA/Docker digest沿用v1.24.0。source/锁文件的版本身份字段变化不等于依赖升级。
- 本候选与main的Dependency freshness workflow成功，完整命令/step起止见jobs原件；最新每日任务run37110043731（2026-10-03T08:32:22Z、source52cc643601021499fdcd37af63b4151217dc93c7）成功，安全与供应链job原件保存。本轮按Go/Node/npm/镜像门禁再次检测，没有把同pins的历史检测伪装为新run。
- Go/Docker/policy/Action与npm依赖图相对该每日source无变化（除版本身份），当前完整发布扫描通过。没有读取并宣称每一行新鲜度报告或新EOL人工审核均已通过；既有来源完整性、候选分类/启动/决策/处置期限继续由dependency-policy与唯一维护工作流管理。
- Node26、TS7和未公开script候选的具体暂缓证据/退出条件见筛选表；Windows需真实签名/原生矩阵。发布责任人保留原SHA与恢复路径，下次相关维护任务补实证后重新资格化；不另起并行队列。

## 生产部署安全核对与回滚

- 生产目标/授权、备份、部署命令、灰度/正式部署、生产版本/Panel/Agent/重启/日志/公网核对：不适用（预览版禁止生产部署）。生产已执行写操作0；`prod-108`/108全部禁用，本轮未连接、读取、备份、部署、升级或核对。
- 验证只使用登记 arena-154 隔离命名空间，不触及其生产 KPanel 服务；隔离成功不能充当生产结果。实际生产健康/升级/回滚未验证。
- 稳定 tag/commit/digest 为首段回滚点；无需回退公共默认通道，它仍1.24.0。用户退出 preview 不自动降级；将来回退需按授权保存配对数据/配置/脚本备份并核健康，不改历史 tag。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T08:35:32+08:00
- 候选冻结时间：2026-10-04T02:32:48.911+00:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间是重放业务候选保留的原 author date；本轮实际重放时间2026-10-04T10:20:51+08:00。前述 L3/预检失败属于发布流程拦截，均未逃逸到公开失败版本或生产；不能计为产品变更失败，不能把 preview 标签计入生产吞吐。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：18
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 6,
    "impact": "五次错误文件操作数：旧业务文档、两项不存在文档/Windows通配路径、managed-script误用mjs、health checker误名；均无源码写入；另一次猜测verify-release-metadata.sh不存在。",
    "recoveryEvidence": "tool chunks 66332d, a76263(two operands), 33c90f, cca4fe; corrected rg/current paths; ade7cd",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "preflight/branch-inventory/json-top-level-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "盘点JSON误读branches字段，TypeError；修正records后逐项读取",
    "recoveryEvidence": "tool chunk 293f7d; authoritative branch-inventory.json",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-pin-inspection/wrong-repository",
    "position": "before-production-write",
    "count": 2,
    "impact": "先在KPanel、再在无关旧sh克隆读取pin对象，不能构成公开存在证据",
    "recoveryEvidence": "tool chunks 053172, 51123d; corrected C:/GitHub/kejilion-sh and public 422/200 receipts",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-l3-entry/unsupported-help",
    "position": "before-production-write",
    "count": 1,
    "impact": "标准entry不支持--help，usage probe退出；没有执行L3",
    "recoveryEvidence": "tool chunk 8ab7f1; read actual script usage and standard L3 receipts",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-source/direct-network-routing",
    "position": "before-production-write",
    "count": 2,
    "impact": "Node直连nodejs.org和Docker auth分别超时；不能视为版本缺失",
    "recoveryEvidence": "tool chunks 8305d3, 8a1f12; public-preflight-r2.log; Python system-proxy r4 passes",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/external-script-integrity/global-region-replacement",
    "position": "before-production-write",
    "count": 1,
    "impact": "第三次公开预检全局替换误改更多字符串，ROOT/CN归一化比较失败",
    "recoveryEvidence": "public-preflight-r3.log; exact difflib two assignment changes and r4 PASS",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-public-ref/noncanonical-https-read",
    "position": "before-production-write",
    "count": 1,
    "impact": "sh本地HTTPS ls-remote超时；未作脚本写入或公开存在推断",
    "recoveryEvidence": "tool chunks 72110,d5145e; canonical public API 61b422 and c981200",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/l3-harvest/read-before-copy-complete",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机在scp完成前校验r2 checksum文件，文件尚未落盘；无效校验未采纳",
    "recoveryEvidence": "tool chunk a404c7; both failed remote evidence checksum lists verified after completed scp",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/governance-health/missing-provider-note",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1标准治理health拒绝同提供商复核理由缺少明确不可用字段，248tests通过不算完整门禁通过",
    "recoveryEvidence": "l3-r1-status.json:26s; l3-r1-remote full checksums; corrective commit4f4996da",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/source-web/npm-connection-reset",
    "position": "before-production-write",
    "count": 1,
    "impact": "r2 npm ci ECONNRESET16.018s，Go取消/deploy未运行；仅同SHA标准入口重跑",
    "recoveryEvidence": "l3-r2-status.json:45s; l3-r2-remote verified; host npm metadata200 and r3 standard entry",
    "permanentAction": "发布责任人：读取操作数先由rg/git目录清单资格化，实际argv/文件schema来自唯一入口；网络及取证使用已核代理/有界命令和完成后摘要校验。2026-10-11前复核；退出条件为对应真实入口成功且失败原件保留。重复指纹须在下一次L3生产写入前修复唯一入口并补回归，本RC禁止生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/git-ref-inspection/powershell-unquoted-caret",
    "position": "before-production-write",
    "count": 1,
    "impact": "收尾只读git rev-parse HEAD^{tree}未引用，PowerShell改写参数导致Git exit1；与正在运行的docs门禁无关，未推送错误SHA或修改产品。",
    "recoveryEvidence": "tool chunks c7e4f0(error), ece975(quoted ref PASS); original docs2159 tree cfef903fb207c12f8c5b0e4b560db80876b5eb89",
    "permanentAction": "发布责任人：2026-10-11前统一通过Git subprocess argv读取带caret/braces的ref，PowerShell调用必须完整引用；退出条件为真实tree读取成功、异常原件保留及最终docs新SHA重新资格化。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

比对最近五个稳定验收 v1.20–v1.24，原始指纹与本轮根因/实际计数保留，不为降低数值换名。重复 read-path 根因须在下一次 L3 生产写前修复唯一入口并补回归；RC没有生产写。诊断 read失败、r1/r2失败和无效取证不冒充已通过门禁，历史CF异常不加到本轮计数。

## 遗留风险与收尾

- 业务基座试行仍需集成后14天真实依赖维护样本，收益尚未报告；无定时监工或虚构观察。任一准入依据变化时重新资格化。
- 原 v1.24.0 浏览器/图库字号、WAN/长期连接/arm64/router/Windows、历史安全待确认部署事实等边界保留，本轮不扩大声明。
- 本地回收：仅本轮 owned E2E container/network/tmp已实际回收；作者与当前RC worktree、当前/上一稳定原始证据、bundle和故障原件保留。收尾采样C盘空闲121723662336字节（约113GiB）；未执行本地文件删除，实际净释放0字节，不能把预期空间当回收结果。
- 证据根 `C:/GitHub/_release-evidence/v1.25.0-rc.1`，包含task/flow/OCR/coverage、三轮L3完整status/log/checksum、精确CI jobs、tag、公开manifest/config/attestation/assets/metadata/E2E、归档与阶段状态。验收记录为既有标准入口，不另建候选队列。
