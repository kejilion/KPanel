# KPanel v1.21.0-rc.14 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`7273ac4e30cfcb032d5d21a403c30d434ad4411d` / `v1.21.0-rc.14`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` 保留，远端候选分支、`main` 和预览标签精确指向 `7273ac4e30cfcb032d5d21a403c30d434ad4411d`。`7273ac4e` 是仅用于重跑主线门禁的空提交，产品树与 L3 冻结候选 `754fef020c46cdb7a449c702a1ab2b3281e93c75` 完全相同。预览版不执行候选分支归档，Release workflow 的归档步骤按预期跳过。

本版只发布 GitHub Pre-release 与 Docker `1.21.0-rc.14`/`preview`，不部署生产、不更新 GitHub Latest、不更新 Docker `latest`。

## 发布画像

- 业务域：历史监控页面分类切换；Passkey 设置操作区与登录页提示的界面收尾。承接 rc.13 已发布的 Passkey 与 MCP/系统管理能力，不改变服务端契约。
- 纳入提交：`7b8674bc`、`b49a1659`、`af6d0ad4`（`feature/monitoring-history-categories`，快进纳入）；`712cc7ef`、`ac9909b9`、`d8b18f7c`、`f2041416`、`8c499517`（由 `feature/passkey-20260921` 的 `2e8dacd3..c7ef901e` 无冲突 cherry-pick，界面文件与源分支逐字节一致）；版本准备 `754fef02`。
- 未纳入：`fix/file-host-switch-context-20260913`（落后 main 312 提交且有未提交改动）与 `feature/visual-refinement-pass`（落后 528 提交且有未提交改动）为陈旧 WIP，保留原样；`docs/security-audit-run2-local-20260919` 按设计仅保留本地。其余活跃分支经 cherry/merge-tree 判定内容已在 main。
- 变更面：仅前端展示与本地偏好（`localStorage` 键 `kpanel:monitoring:category`）；无 API、存储、宿主机写入或部署变化。`scriptLinkageState=not-required`：未修改 `kejilion.sh` 或其调用契约。
- 风险等级及理由：L3（发布通道）。产品改动为 L1 前端展示；OCR 行级评审 `OCR-Review: 1.12.6 range=9815fa4..b9167c7 files=7/7 free-form=2 valid=H0/M0/L2 constrained-only=0`，两条 LOW 为分类 tablist 缺少 tabpanel/方向键导航与分类跨主机沿用（设计如此），不阻断。

## 覆盖检查与安全审计

- `node scripts/check-security-audit-coverage.mjs --target 754fef020c46cdb7a449c702a1ab2b3281e93c75`：decision 为 `scoped-required`，未审计 21 个提交、71 个文件，新边界包 `cmd/kpanel-mcp`、`internal/mcpaccess`、`internal/mcpbridge`、`internal/tlsfallback`。预览版只记录不阻断。
- 发布期间确认 2026-09-21 已完成的 full run-4（源码 `4c0694aa`，127 单元，0 confirmed / 34 needs_validation / 1 rejected，两个结构验证器 2026-09-22 重跑 PASS）尚未入库；登记后覆盖检查模拟结果为 `ok`。该登记随稳定版候选单独提交，不改变本 RC 的结论。
- L3 的 Go 全量测试、核心 race、govulncheck、npm audit、Trivy 源码/依赖/镜像扫描均通过，不表述为 full security coverage 或“没有漏洞”。

## 自动门禁与隔离验证

- L3 回退环境 `local-wsl-dr`（`arena-154` SSH 22 端口超时，本次明确选择灾备）：run ID `v1.21.0-rc.14-754fef0-l3-r1`，候选 `754fef020c46cdb7a449c702a1ab2b3281e93c75`，`status=passed`、`exit_code=0`；开始 `2026-09-22T09:12:11Z`，完成 `2026-09-22T09:18:33Z`。Runner image `kpanel-release-gate:go1.26.7-node24`，Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`，base tag `v1.20.0`，业务基线 `6e9d2d82`。
- L3 证据目录：`C:/GitHub/_release-evidence/v1.21.0-rc.14-754fef0-l3-r1`。bundle SHA-256 `8bcdce123be5c574887a4d8825f56792a0d73d7ce07af1396be73d1d4f8f93b8`，plan SHA-256 `b03bb177a0cf3dabbbf419cbe898c26196c2c8f94654c67488f8f4c7a12b46b2`，远端脚本 SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，L3 日志 SHA-256 `af2e7d7091c6fa8e67d0bea75719b5fe8cb85eb41a076229cb90a6db8c181375`，manifest SHA-256 `b65f348e2f127924b0adc24aa8e644cc15f39eb5a47836789a71871750cf6173`。
- L3 通过项目：Go 全量测试、`internal/panel`/`internal/auth`/`internal/dockerx` race、Web 测试、typecheck/build、govulncheck、npm audit、Trivy、双架构构建、managed script contract、install safety、镜像运行时限制和 app config lifecycle。
- 候选 CI [#1042](https://github.com/kejilion/KPanel/actions/runs/35709672191) 与候选 Dependency freshness [#546](https://github.com/kejilion/KPanel/actions/runs/35709672215) 成功，绑定 `754fef02`；主线 Dependency freshness [#547](https://github.com/kejilion/KPanel/actions/runs/35710394162) 成功。
- 主线 CI [#1043](https://github.com/kejilion/KPanel/actions/runs/35710394149) 仅在 Detect races in privileged core packages 步骤失败（见流程异常）；同产品树的重试提交 `7273ac4e` 候选 CI [#1044](https://github.com/kejilion/KPanel/actions/runs/35711055419) 与主线 CI [#1045](https://github.com/kejilion/KPanel/actions/runs/35711055430) 均成功。空提交不触发 Dependency freshness 路径过滤；标签 Dependency freshness [#548](https://github.com/kejilion/KPanel/actions/runs/35719581769) 随 tag 运行。
- 公开镜像 E2E、隔离真机与浏览器验收：未执行。`arena-154` 不可达，`local-wsl-dr` 按 `docs/project-management.md` 只允许候选 L3，不作为其他验收环境。组件与视图行为由 L3 Web 测试覆盖（分类切换、记忆、深链切回、手动选择保持；Passkey 主/次操作与禁用状态）。

## 发布产物与通道状态

- GitHub Release workflow [#246](https://github.com/kejilion/KPanel/actions/runs/35719581766) 全部步骤成功；公开页面 [KPanel v1.21.0-rc.14](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.14) 为非 draft、Pre-release、非 Latest，API 返回 14 个资产（含 `kejilion-agent-linux-amd64/arm64`、`kejilion-panel-deploy-1.21.0-rc.14.tar.gz`、`SHA256SUMS`）；annotated tag object 为 `10627f405b43d0d632f80fad93a62c0398e2aa5c`。
- Docker `kjlion/kejilion-panel:1.21.0-rc.14` 与 `:preview` OCI index 均为 `sha256:bd79e42938a319b325cdfd02d61241525428bef887d2f181636ea2f73d406308`（与 Release notes 记录一致）；`linux/amd64` 为 `sha256:e6348b68b03cc038e12d9fa98eec44111f6d4b616147c393a1262b06bb999b00`，`linux/arm64` 为 `sha256:e25cf1da7c0d361b5d7a7a88ffbb34bdc20586620f1be970c086627d3d0f2751`，另有两个 provenance/attestation manifests。
- GitHub Latest 仍为 `v1.20.0`；Docker `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- 应用市场：`packaging/kejilion-app/kpanel.conf` 自 `v1.21.0-rc.13` 无差异，无需 `kejilion/apps` 提交，默认入口仍为稳定版。

## 生产部署安全核对

- 正式部署环境：不适用；本轮预览发布禁止生产写入。
- `prod-108`：禁用全部 KPanel 操作，本次未连接、未部署、未核对。
- 生产已执行写操作：0。
- 回滚：未执行；若预览异常，将 `preview` 来源固定回 rc.13 镜像 `sha256:fe9307f4191b439aaea0083b56ae37b5a4b23c790c43ef7748725c932c6339ea`，稳定入口和 Docker `latest` 保持不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T16:44:59+08:00
- 候选冻结时间：2026-09-22T17:11:19+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "ci/main/race-flake",
    "position": "before-production-write",
    "count": 1,
    "impact": "主线 CI #1043（run 35710394149）仅在 Detect races in privileged core packages 步骤失败；同一产品树的候选 CI #1042 与 L3 race 均通过。未登录 GitHub，无法下载失败日志确认具体用例。",
    "recoveryEvidence": "创建不改产品树的空重试提交 7273ac4e，候选 CI #1044（run 35711055419）与主线 CI #1045（run 35711055430）均成功；标签打在 7273ac4e，Release #246 全流程成功。",
    "permanentAction": "rc.13 与 rc.14 连续出现同一 race 步骤抖动，应在有日志访问权限时定位具体测试并修复其不确定性，而不是继续依赖重试提交；在此之前保留原始失败 run，不跳过 race 门禁、不移动标签。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 公开镜像 E2E、真实目标服务/浏览器矩阵、长期 soak 和生产部署均未执行，属于本预览版未覆盖范围；`arena-154` 恢复后应补做公开镜像 E2E。
- 主线 race 步骤连续两个 RC 出现抖动，需要在可读取 CI 日志时定位根因。
- 稳定版前需以新的稳定候选重新执行稳定 L3，并以入库的 full run-4 满足覆盖检查；本 RC 的 `scoped-required` 记录不能替代稳定版覆盖门禁。
- 候选分支、L3 证据和 Release 证据保留用于后续复核。
