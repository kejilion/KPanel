# KPanel v1.21.0-rc.10 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`1bf45c51b6611a0f90485080aa455f86ca7c4ede` / `v1.21.0-rc.10`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览候选分支保留，发布后不得删除或改写；发布前远端精确指向 `1bf45c51b6611a0f90485080aa455f86ca7c4ede`。

本版只发布 GitHub Pre-release 与 Docker `preview`，不部署生产、不更新 GitHub Latest、不更新 Docker `latest`、不归档预览候选分支。

## 发布画像

- 业务域：发布治理覆盖账本与候选发布流程修正；产品运行时功能未新增。
- 变更面：治理脚本、审计覆盖记录、版本与变更日志；`scriptLinkageState=not-required`，未修改 `kejilion.sh` 或 `kpanel.conf`。
- 风险等级及理由：L3。候选基线包含治理边界与发布门禁变更，完成固定 Runner L3、候选/main CI、公开 Release 安全链和双架构镜像验证。

## 覆盖检查与安全审计

- 覆盖检查：`node scripts/check-security-audit-coverage.mjs --target HEAD` 的 decision 为 `scoped-required`；当前历史仍有 19 个未覆盖提交、67 个文件和 4 个新增边界包（`cmd/kpanel-mcp`、`internal/mcpaccess`、`internal/mcpbridge`、`internal/tlsfallback`）。已完成的 `run-2`、`run-6`计入覆盖；`run-3`、`run-5` 曾中断，不计为完成覆盖。本结论按预览版规则记录，不阻断本 RC，不得表述为 full coverage 或全量无漏洞。
- 本次没有把动态沙箱不可用的线索降级为 confirmed；L3 的 govulncheck、npm audit、Trivy 源码/依赖/镜像扫描均未发现阻断项，但覆盖账本仍是后续稳定版准入条件。

## 自动门禁与隔离验证

- 候选 CI：[#1025](https://github.com/kejilion/KPanel/actions/runs/35677900575) 成功；Dependency freshness [#533](https://github.com/kejilion/KPanel/actions/runs/35677900566) 成功。
- 主线 CI：[#1026](https://github.com/kejilion/KPanel/actions/runs/35678749034) 成功；主线 Dependency freshness #534 成功。
- L3 默认环境 `arena-154`：run ID `v1.21.0-rc.10-1bf45c5-l3-r1`，因 SSH 连接 `154.36.153.9:22` 超时，在候选代码执行前失败；证据保留在 `C:/GitHub/_release-evidence/v1.21.0-rc.10-1bf45c5-l3-r1`，该次不作为产品失败。
- 注册回退环境 `local-wsl-dr`：run ID `local-wsl-dr-v1.21.0-rc.10-1bf45c5-l3-r2`，`status=passed`、`exit_code=0`；证据目录 `C:/GitHub/_release-evidence/local-wsl-dr-v1.21.0-rc.10-1bf45c5-l3-r2`。bundle SHA-256 `089eeb8ce68e3ec07ddea3791e1ad13b77fdc653574873963bee819b7aab41e1`，plan SHA-256 `5fe9927e4b3aba8349d9d2f922422e8b7a41588b095c613f08dc64f79a4180d9`，远端脚本 SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，L3 日志 SHA-256 `598c964a67dd4dc01c1d8c74438c8753d79a4c046e2b1b123658bacd046ca278`。
- L3 通过项目：Go 全量测试、核心 race、Web 180 files/1575 tests、typecheck/build、govulncheck、npm audit、Trivy、双架构构建、managed script contract、镜像运行时限制和 app config lifecycle。

## 发布产物与通道状态

- GitHub Release workflow：[#242](https://github.com/kejilion/KPanel/actions/runs/35679236805) 成功；公开页面 [KPanel v1.21.0-rc.10](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.10) 已为非 draft、Pre-release、非 Latest，资产区显示 16 个附件。annotated tag object 为 `31a832cd2f9dcd5cf564bd1c90f93fdef5463985`。
- Docker 公开镜像：`1.21.0-rc.10` 与 `preview` OCI index 均为 `sha256:ac5b86cb69d4e8e2485e41913040bf4b17d5a208e163b7cb6eabe2fc66a16dc1`；`linux/amd64` 为 `sha256:ea03c5910d87fddd0a791c016beaaf44e9245bf1c534d5ecaea7e58dd60974c2`，`linux/arm64` 为 `sha256:cc144b23588dc74655f8f32a68a7baa3f360d4bec8636cc73830191f3d90af93`，另有两个 provenance/attestation manifests。`latest` 保持上一稳定摘要 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- Release 资产共 16 个（14 个发布文件加 source zip/tar.gz），`SHA256SUMS` 资产摘要为 `sha256:0d15d4a2fd8d627e72eff9371eb8ad89ccf6895499bdc4c9bb97a512fad8fd1d`；GitHub Latest 仍为 `v1.20.0`，应用市场默认入口与生产入口未改动。

## 生产部署安全核对

- 正式部署环境：不适用；本轮预览发布禁止生产写入。
- `prod-108`：未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：0。
- 回滚：预览异常时将显式 preview 来源固定回 `v1.21.0-rc.9` 已验证 digest，稳定入口与 Docker `latest` 不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T01:59:28+08:00
- 候选冻结时间：2026-09-22T10:07:22+08:00
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
    "fingerprint": "l3/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "默认 arena-154 L3 在 SSH 连接阶段超时，候选代码未执行，原证据不计入产品结论。",
    "recoveryEvidence": "C:/GitHub/_release-evidence/v1.21.0-rc.10-1bf45c5-l3-r1 的 manifest 与 SSH timeout 输出；随后 local-wsl-dr r2 passed。",
    "permanentAction": "保留 local-wsl-dr 候选验证回退路径，并在下一次 L3 前复核 arena-154 SSH 可达性。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 真实目标服务/硬件认证器矩阵、长期 soak、arena-154 真实目标服务和生产部署均未执行，属于预览版未覆盖范围。
- 稳定版前必须以新的稳定候选重新执行 required security coverage、稳定 L3、生产灰度和回滚演练；本 RC 的 `scoped-required` 不能替代稳定版覆盖门禁。
- 候选分支、L3 证据和 Release 证据保留用于后续复核；不删除或强制清理历史证据。
