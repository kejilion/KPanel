# KPanel v1.24.0-rc.10 发布验收记录

日期：2026-10-03

发布级别：L3

候选提交 / 标签：`6b3cfa88a47b8da473a443fa0e60190a4bffdae6` / `v1.24.0-rc.10`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96`；Docker `latest` 为 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` / 预览版保留。产物已发布，生产未部署。

- 产品候选 tip：`6b3cfa88a47b8da473a443fa0e60190a4bffdae6`，保留在不可变 annotated tag；纯验收文档提交随后按同 SHA CI 快进主线，并对齐本地和远端候选。
- `feature/terminal-duplex-input` / `8440de317a8d18814304da7d9fc5146cc969bfd1` → `refs/heads/archive/feature/terminal-duplex-input/v1.24.0-rc.10`，远端 SHA 已复核；保留原作者本地分支和工作树，未操作未知内容。
- `feature/terminal-job-duplex-input` / `d0437534f54552b16448fd93f6dcf1a6e0e6598f` → `refs/heads/archive/feature/terminal-job-duplex-input/v1.24.0-rc.10`，远端 SHA 已复核；保留原作者本地分支和工作树，未操作未知内容。
- `fix/rc10-environment-receipt-race` / `4f0c3368574efb89f85dfa4e265b236bb469c16f` → `refs/heads/archive/fix/rc10-environment-receipt-race/v1.24.0-rc.10`，远端 SHA 已复核；本次自有 clean 修复工作树已回收。
- 纯验收分支：`docs/release-v1.24.0-rc.10-acceptance`；在同 SHA 主线 CI 成功后归档到 `archive/docs/release-v1.24.0-rc.10-acceptance` 并回收自有工作树；实际 SHA 与收尾回执位于 `C:\GitHub\_release-evidence\v1.24.0-rc.10/acceptance-archive.json`、`final-alignment.json`。
- 旧候选 `87b856f7a5799f3f9e7f9f35487f5054df8d6907` 的证据保存在 `revision-r1`，因发现完成凭据时序缺陷被替代，没有发布该候选或改写历史 tag。
- 未知作者的图库、Docker 和其他工作树保留；本次未重置、清理、提交其内容。

## 发布画像

- 业务域：WebSSH 本机/完整 Panel/轻量节点终端，应用/建站/体检/环境任务终端。
- 变更面：输入协议、UI 输入状态、内存队列、错误恢复、环境任务完成状态；不新增业务权限或迁移。
- 用户旅程：连续输入和 UTF-8 粘贴 → 有序 ACK；刷新 → 新页面接管；代理拒绝 WS → HTTP 批次回退；Agent 重启 → 旧帧拒绝、新 epoch；任务完成 → 输入关闭、拒绝后续 claim。
- 未变化契约：Panel 非特权，宿主写入经既有 Agent；已有配对 scope、监听端口、Compose、脚本 PTY/FIFO、应用市场默认 stable/latest 保持。
- 风险等级：协议/任务生命周期为 L2；本次发布执行 L3。性能模型、真实字节和用户界面证据分别记录。

## 发布范围与未纳入内容

- 用户更新：32 帧、每帧 2048 字节的确认流水输入，合计队列最多 1 MiB；首次协商复用、页面接管和结果不确定提示。旧目标能力不足时保持原 POST。
- 主机候选：`8440de317a8d18814304da7d9fc5146cc969bfd1`；任务候选：`d0437534f54552b16448fd93f6dcf1a6e0e6598f`；以 no-ff merge 保留原提交。
- 本次验收发现并修复：`4f0c3368574efb89f85dfa4e265b236bb469c16f`。第一次读不到凭据、检查 unit 时任务已经退出，会把成功环境任务记为 `needs_attention/receipt_missing`；退出后再读凭据恢复可信完成状态，仍无凭据时继续明确人工复核。
- 保留 RC9 的图库菜单层级、手动相册封面/移动及 Docker 监控；不重新宣称本轮覆盖其全部媒体/缩放矩阵。
- 未纳入：Node 26、TS7、治理草稿、其他未知候选；不执行生产部署、原生安装器业务动作或实际 WAN 长期 soak。

## 外部审计与修复交付

- 安全审计：本轮没有新增 CF run；已完成 full `run-4` 源码 `4c0694aa8e02e46145a775707b8d5a0355f7ce10` 和既有 scoped 记录仍按原范围有效。
- 覆盖检查：`decision=scoped-required`，未审计提交 67、文件 124、最老 6 天；新边界 `internal/backupremote` 来自此前版本序列。RC 只记录，不能写成 CF 审计通过；正式 1.24.0 前必须按项目规定补审。审计执行配置为 `gpt-6-luna/max`。
- 修复证据：原生 R2 状态、成功 receipt 和 systemd journal 保存于 `revision-r1`；定向 race 回归覆盖凭据在退出检查中出现，以及真正缺凭据；修复后全 L3、R3 16 个原生任务均成功。
- OCR：最终 58/58 reviewable files，52 个 Git blob 与已交付源相同、复用原独立审查；六个集成/修复文件由接手发布责任人复核。没有额外独立审查或第二 Provider 结论。
- 修复增量的 free-form 先于新 OCR 规则；旧规则已经在前轮可见，不能宣称全候选重新盲审。最终有效 H0/M0/L0、constrained-only=0；首轮发现的生命周期缺陷另行保留，未隐去。
- 本稳定周期尚不足三个完整稳定观察周期，不提前判断 OCR 有效性或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））。
- 变更集编号：不适用。
- 实际内置脚本：`c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`。
- 脚本候选：不适用；使用既有 PTY/FIFO，不增加脚本动作、receipt 格式或安装/更新契约。
- `kpanel.conf` 归一化 SHA-256：`f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；公开镜像实际 bytes 已验证。`kejilion/apps` 无写入。
- 发布决定：脚本不在范围；无需发布脚本不是暂缓脚本发布。阻断或移除依赖：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 20 Chrome/Edge 输入场景、16 原生 systemd/FIFO/PTY、6 主机和 12 混合版本场景 | 控制菜单输入链路，不代表原生安装/建站等业务成功 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 全 L3 的 npm、govulncheck、Trivy、权限/协议测试及公开 SBOM/provenance | 未进行本轮 CF 入侵审计；模块树存在不可达通告不能写成零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | 6 真实 PTY ACK/背压场景、双方向 RC9 混合版本、Agent epoch 重启、完成凭据 race | 人为不消费 ACK 不是真实网络丢包；未做长期 soak |
| 性能与资源预算 | 已验证 | 128 KiB 随机粘贴、32 帧/64 KiB flight；local RTT150 粘贴 325.018ms；full-panel RTT150 粘贴 327.282ms；light-node RTT150 粘贴 317.921ms | 单轮 RTT0/150 模型，没有本轮旧版对比倍数结论 |
| 用户体验与可访问性 | 已验证 | Chrome/Edge 实际输入、粘贴、刷新、关闭及零 pageerror | 外置 harness 挂载真实 Vue 组件；未覆盖移动/Safari/多主题/缩放和 OS 剪贴板 |
| 数据、配置与迁移 | 已验证 | 无新增迁移；L3 生命周期/备份/更新回归；18 个既有容器前后状态相同 | 隔离现场与生产严格区分 |

## 自动门禁

- 定向回归：环境完成 race 两种状态；`go test -race ./internal/webenv` 通过；格式及 diff 检查通过。
- `make verify-release`：固定 Linux Runner 全测试、typecheck、race、vet、govulncheck、npm audit、Trivy、双架构构建、受管脚本、安装/更新/失败恢复通过；`l3-checksums.json` 的 12 份原始证据 SHA-256 已核对。
- L3 外层入口：`scripts/run-release-l3.mjs`；run `v1.24.0-rc.10-6b3cfa88-l3-r2`；终态 passed/exit0。
- Runner：`sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0。
- plan `d1a90586f80ba6cc654841279e373f5ad729fbba0d45dbdae0fa2d15b1b8cfc8`；remoteScript `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `43d7e3bd1deb29dc5a38a3701a423ce113c970dd5c5bef83920e9865430000ab`。
- 原始证据：`C:\GitHub\_validation\kpanel-v124-rc10-l3-r2\remote-evidence`；首轮验证未覆盖修复候选，不能替代本次。
- 候选 CI：[CI](https://github.com/kejilion/KPanel/actions/runs/37086748515)、[Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37086748438)；主线：[CI](https://github.com/kejilion/KPanel/actions/runs/37087022166)、[Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37087022189)；Release：[Release](https://github.com/kejilion/KPanel/actions/runs/37087577737)，均同一精确产品 SHA 成功。
- 纯验收文档需独立候选/主线 CI，不改变产品 tag 和镜像；最终回执存于本次 evidence 根。

## 依赖与技术栈变化

- 本版未升级第三方依赖、Action 或基础镜像，仅一致更新 RC10 版本元数据；候选 freshness 与 Release freshness 按固定来源完成。
- 保留 Go 1.27.1 / Node 24.21.0、当前锁文件、Action SHA、受管脚本及基座摘要。未将 Node26 或 TS7 草稿混入。
- 每日安全/EOL 检测沿用仓库现有状态；本轮不虚构新的一次完整外部审计。模块级不可达通告和 CF 覆盖待补审均保留。
- 回滚到不可变 RC9 镜像；无人值守节点仍跟 stable/latest。

## 隔离真机与浏览器验收

- 环境策略：`arena-154`，原生 Linux amd64/systemd/Docker；禁止 `prod-108`/`108`，本次未连接。
- 原生 Panel 容器非 root/read-only/cap-drop ALL/no-new-privileges，Panel/Agent 分别 256 MiB；systemd 控制菜单使用真实 FIFO/PTY，限定可写目录，未运行真实安装器。
- Chrome/Edge：chrome 154.0.8037.93, edge 154.0.4258.48；1366×900，zh-CN，默认缩放。DOM ClipboardEvent 模拟粘贴，不能写成 OS 剪贴板。
- 作业 `arena-154-34688`，终态 passed / exit 0，600 秒硬超时；规格 `terminal-browser-spec-r3.json` / `ab508871cf7cbef955257d96e19bb708e04aaa285613eaf78d3074810f1ed6de`，脚本 `6bcc134af398692aea1f7cab2bd6dcb4740ea1e51044d9be05bb15949b1480dd`。
- 20 场景：16 四类任务 + 4 本机主机终端；WS 允许和 HTTP-only 拒绝升级两种真实代理；顺序字节、重复 ACK 去重、变更 replay/gap 拒绝、刷新接管、Agent 重启旧帧拒绝、完成后输入关闭及成功状态。
- Native R3 16 tasks 均 succeeded、unit inactive、FIFO removed；Panel 容器移除、Agent 停止，18 个既有容器的 ID/StartedAt/status/health 未变。主机 harness 的容器和网络也已回收。
- 主机矩阵复用已有 harness 源码，重新执行本次 Panel/Agent/Node 二进制和 TLS/Noise 配对；同一配对跨兼容交换保留。不是复用旧候选成功结论。
- 不执行长期 soak、WAN/CDN/TLS、Safari/移动、实际安装/建站/跑分/环境升级；输入协议变化已有针对证据，不以模拟业务成功填补这些边界。

## 发布产物与公开仓库复核

- [v1.24.0-rc.10](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.10)：draft=false，prerelease=true；published `2026-10-03T02:03:54Z`；GitHub Latest 仍 `v1.23.0`。
- Docker 版本和 `preview` OCI index：`sha256:ecf275a256e99ce1d18586fd46d5b86e545153033c99fac53ac2c0aaa714d6db`；amd64 `sha256:2ad7c4f093b3a0b3024e0849ebffc4aa82b6d18e6e1571fb11b25b8de4edf182`，arm64 `sha256:cbe5407462dd396098e79819be6602b04fac423bba4936bde44a1959a19defb0`。
- `latest` 未变：`sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`；原 RC9 镜像与 tag 保留。
- 附件共 14 项，其中 11 项 `SHA256SUMS` 与 GitHub asset digest 核对；元数据内 VERSION 与精确源码相同。公开 amd64 Node 实际下载校验，其余二进制只核对 GitHub digest，不代表本轮原生执行；SBOM/provenance 的 predicate 与镜像 subject 绑定已核对，未审计其完整内容。
- 公开 amd64 镜像 `image_e2e=pass`，不可变 digest，127.0.0.1:18089；临时容器/网络/数据清理，内置 VERSION/脚本/appconf/图库图标 bytes 核对通过。arm64 构建/manifest 验证不代表原生 arm64 真机。

## 自更新通道验收

- 沿用 RC9 通道契约并在本次 L3 回归：稳定来源正式 Latest，预览来源规范 RC，校验官方镜像摘要；加入预览只切换来源并检查，不自动安装。
- 自动安装开关与一次性安装分离；旧状态默认 stable、选择持久；退出预览不自动降级。
- systemd 备份、失败恢复和崩溃恢复按 L3 证据；OpenRC/轻量 Node 边界按 release-channels，未将其写成当前原生路由器升级验证。
- 生产管理员未实际加入预览或安装 RC10，不能写成生产自更新成功。

## 生产部署安全核对

- 生产目标、正式部署环境、授权、备份、部署命令、postdeploy 和正式管理员写入：不适用（预览版禁止生产部署）。
- 验证环境：`arena-154` 的独立资源；本次生产写操作 0。
- `prod-108`：禁用全部 KPanel 操作，本次未连接、未备份、未部署、未升级、未核对。
- 隔离输入/服务重启和公开镜像 E2E 均不作为生产证据。

## 回滚

- 上一预览 tag：`v1.24.0-rc.9` / `2dd0c6398b9bc16292c7db44e526915c2a147111`；镜像 `sha256:11ee0a1c6691d36cf2523d30304ae8fedbd9a9ec657f21a1e5d8bcac372d384f`。
- 无生产数据/配置修改；实际回滚和生产备份不适用。需要降级时单独授权、绑定精确 digest 和对应备份，不改历史 tag。
- GitHub Latest/Docker latest/应用市场默认仍稳定 1.23.0；公共默认更新通道决策：不适用，稳定来源未变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T23:51:38+08:00
- 候选冻结时间：2026-10-03T01:13:33.437+00:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：12
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

原始执行器/夹具/收尾失败独立计数，正常门禁发现的产品凭据缺陷不混入流程异常。复发历史只填实际核对的正式版本，不用本轮重试冒充多个版本；本版是 RC，不宣称正式发布/部署频率。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/git-refs/powershell-tag-peel",
    "position": "before-production-write",
    "count": 1,
    "impact": "Unquoted ^{} tag peel was parsed as a PowerShell ScriptBlock; mandatory ref read failed before any write.",
    "recoveryEvidence": "remote-preflight-r1-failed.log; quoted-ref remote preflight passed at exact D9/P9 identities",
    "permanentAction": "Release owner: use Python argv and quoted tag peels in RC10 SSH publication helpers; recheck before next L3 production write by 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/runner-discovery/legacy-alias",
    "position": "before-production-write",
    "count": 1,
    "impact": "An old Runner alias was absent during discovery; no gate used that alias.",
    "recoveryEvidence": "Frozen exact Runner ID resolves to kpanel-go127-prep-runner:go1.27.1-node24.21.0; 12 L3 evidence checksums and runner.txt verified",
    "permanentAction": "Release owner: resolve the required exact image ID to tags before Runner selection; next review 2026-10-10 before a production run.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/component-harness/wrong-auth-method",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1 browser harness called nonexistent api.auth.session; no browser journey completed; product was not published.",
    "recoveryEvidence": "terminal-browser-job/browser-test.log and state.json; browser-r1 source/result snapshots; fresh r2 using actual api.auth.status",
    "permanentAction": "Corrected only the outside-repository harness; preserve r1 and require exact-candidate r2 passed state and 20 executed journeys before publication.",
    "historicalReleases": []
  },
  {
    "fingerprint": "host/harness-evidence/busybox-find-option",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1 pairing setup completed but BusyBox find rejects GNU -printf; host matrix was aborted and isolated resources cleaned.",
    "recoveryEvidence": "host-harness-execution.log; host-evidence/setup.log and result.json cleaned=true; fresh host-r2 uses host Python paths",
    "permanentAction": "Removed the unnecessary in-container find command; exact known evidence files are read through the owned host bind directory; require r2 complete and clean before publication.",
    "historicalReleases": []
  },
  {
    "fingerprint": "collector/native-lifecycle/unchecked-python-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "First native collector had a missing closing bracket and stopped before mutation.",
    "recoveryEvidence": "finish-terminal-validation-r1.py; terminal-harvest-cleanup-r1.log",
    "permanentAction": "Release owner: Final r3 collector parsed before execution; original source retained. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "collector/native-lifecycle/job-schema-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "Collector assumed every job record has inputOpen; environment Job has this only in TerminalChunk. A separate real completion race was also exposed and repaired.",
    "recoveryEvidence": "finish-terminal-validation-r2.py; terminal-harvest-cleanup-r2.log; revision-r1 raw native evidence; receipt repair commit",
    "permanentAction": "Release owner: Final collector checks executed browser input-closed assertion, rejected claim, removed FIFO and inactive unit rather than a nonexistent Job field. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/transient-units/unloaded-unit-stop",
    "position": "before-production-write",
    "count": 1,
    "impact": "After removing the isolated Panel, stopping an already unloaded transient unit returned exit 5 and interrupted receipt creation. No business service changed.",
    "recoveryEvidence": "revision-r1/terminal-harvest-cleanup.log; recover-r2-cleanup.py; revision-r1/terminal-raw-r2/task-lifecycle.json",
    "permanentAction": "Release owner: Current collector saves pre-cleanup proof and only stops units still active; current and superseded business comparisons retained. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "repair/targeted-go/empty-offline-module-cache",
    "position": "before-production-write",
    "count": 1,
    "impact": "Focused repair tests initially disabled network while the mounted Go cache lacked x/sys, so dependency resolution failed before executing tests.",
    "recoveryEvidence": "receipt-repair-race-r1.log; receipt-repair-race.log",
    "permanentAction": "Release owner: Exact Runner rerun with normal dependency-download network and explicit owned source/cache bindings; tests passed. Verify module cache before offline use. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "freeze/evidence-parser/go-output-whitespace",
    "position": "before-production-write",
    "count": 1,
    "impact": "Refreeze preflight incorrectly matched Go ok output whitespace; valid test output was rejected before mutation.",
    "recoveryEvidence": "refreeze-preflight-failures.log; refreeze-repaired-candidate.py",
    "permanentAction": "Release owner: Compare parsed tokens rather than an exact whitespace prefix. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "freeze/ocr-coverage/hardcoded-file-count",
    "position": "before-production-write",
    "count": 1,
    "impact": "Refreeze preflight assumed 59 reviewable files; actual OCR preview reports 58, so it stopped before commit.",
    "recoveryEvidence": "refreeze-preflight-failures.log; ocr-preview.json; ocr-coverage.json",
    "permanentAction": "Release owner: Derive coverage and reused blob counts from actual preview and Git blob equivalence; final 58/58, reused 52. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-progress/github-read/unsupported-job-endpoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "Optional release job metadata inspection used an unsupported GitHub Fetch endpoint; absent content was then rejected by JSON.parse. No mandatory receipt or publication changed.",
    "recoveryEvidence": "release-progress-read-failure.json; purpose-built github_fetch_workflow_job_steps and the allowed run endpoint provide actual status.",
    "permanentAction": "Release owner: use purpose-built job steps; check isError and response schema before parsing GitHub Fetch. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/release-metrics/unsupported-field-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "Acceptance writer supplied six fractional timestamp digits and annotated not-applicable production value; canonical metrics validator rejected those field formats before commit. Product tag and publication were unchanged.",
    "recoveryEvidence": "acceptance-schema-r1 preserves both docs, helper source and failed checks; acceptance-checks-r2.log contains the corrected canonical-field validation.",
    "permanentAction": "Release owner: format structured metrics timestamps with millisecond precision and use bare canonical not-applicable/yes-no values; explanatory prose belongs outside the six-field block. Review before next production L3, no later than 2026-10-10.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 资源回收：本次 clean 修复工作树已归档回收，字节数见 `branch-disposition.json`；Arena 自有可再生目录在公开镜像验收后已回收，18 个既有容器状态未变，实际回执见 `arena-cleanup.json`。作者工作树、唯一原件、证据与 Runner 缓存保留。纯验收工作树在最终同 SHA 主线 CI 后按精确 owned 路径回收，实际回执见 evidence 根。
- 当前 Mock 地址 `http://127.0.0.1:4174` 仅供交互查看；最终文档对齐后由固定 local-feature-preview 入口重新绑定最终 clean 文档提交，并保留停止命令。
- 未验证：CF 增量审计、WAN/真实丢包/长期压力、移动/Safari、多主题/缩放、arm64 原生、实际安装业务。正式版前补审；性能结果不外推为所有网络环境或比较倍数。
