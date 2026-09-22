# KPanel v1.21.0-rc.11 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`f1cfab37f8c6d7dd1e84a5233029683fe789133c` / `v1.21.0-rc.11`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` 保留，发布后不删除或改写；远端候选分支与预览标签均精确指向 `f1cfab37f8c6d7dd1e84a5233029683fe789133c`。主线验收记录提交会使 `main` 继续前进，但不改变产品标签。

本版只发布 GitHub Pre-release 与 Docker `preview`，不部署生产、不更新 GitHub Latest、不更新 Docker `latest`，也不执行候选分支归档。

## 发布画像

- 业务域：Passkey trusted-origin 配置与桌面异步 inventory/布局首帧稳定性。
- 本次补齐的遗漏：`0290e354a34de6103bcac65bb657a8af4b354f6e`（Passkey trusted-origin）；`73b36fe4cbd561aa2860c987a316ad56c504c2c8`、`00f5781e8be34d98a07096d125e68e008b876cdb`、`15bd4f67a1f0a53df1bb2c14250149bc2de3a60e`（桌面异步清单、布局和 workspace readiness）。
- 变更面：认证 origin 识别、当前管理因子确认、审计/多语言文案、桌面并行加载和图标回退；`scriptLinkageState=not-required`，未修改 `kejilion.sh` 或其调用契约。
- 风险等级及理由：L3。涉及认证信任边界和桌面首帧状态，完成固定 Runner L3、候选/main/tag 门禁、公开 Release 和双架构镜像校验。

## 覆盖检查与安全审计

- `node scripts/check-security-audit-coverage.mjs --target HEAD` 的最终 decision 为 `scoped-required`；当前历史有 20 个未覆盖提交、71 个文件和 4 个新增边界包（`cmd/kpanel-mcp`、`internal/mcpaccess`、`internal/mcpbridge`、`internal/tlsfallback`）。`run-2`、`run-6` 已完成并计入覆盖；`run-3`、`run-5` 中断，不计为覆盖；`run-1` 的 full-run 证据标记 `source_dirty=true`，不作完全可复现的 full coverage 结论。
- 本记录明确区分覆盖状态与扫描结果：L3 的 Go 全量测试、核心 race、govulncheck、npm audit、Trivy 源码/依赖/镜像扫描均无阻断项，但这不等于 full security coverage 或“没有漏洞”；稳定版前仍须完成 required security coverage。

## 自动门禁与隔离验证

- 候选 CI：[#1030](https://github.com/kejilion/KPanel/actions/runs/35682041539) 成功并绑定 `f1cfab37f8c6d7dd1e84a5233029683fe789133c`。
- 主线 CI：[#1031](https://github.com/kejilion/KPanel/actions/runs/35682667486) 成功并绑定同一 SHA；主线 Dependency freshness [#537](https://github.com/kejilion/KPanel/actions/runs/35682667473) 成功。
- 候选 Dependency freshness：[#536](https://github.com/kejilion/KPanel/actions/runs/35681872425) 在版本元数据提交 `11afd6302b67db83c7ebfe63045e99f7a7eee850` 上成功；其后仅有主线同步和业务上下文文档刷新，依赖图未变化，最终主线和 tag freshness 均对精确产品 SHA 成功。
- 标签 Dependency freshness：[#538](https://github.com/kejilion/KPanel/actions/runs/35683230399) 成功并绑定标签提交。
- L3 默认环境 `arena-154`：run ID `v1.21.0-rc.11-f1cfab3-l3-r2`，在上传候选代码前因 SSH `154.36.153.9:22` 超时失败；证据保留在 `C:/GitHub/_release-evidence/v1.21.0-rc.11-f1cfab3-l3-r2`，不作为产品测试失败。
- 注册回退环境 `local-wsl-dr`：run ID `local-wsl-dr-v1.21.0-rc.11-f1cfab3-l3-r3`，`status=passed`、`exit_code=0`；证据目录 `C:/GitHub/_release-evidence/local-wsl-dr-v1.21.0-rc.11-f1cfab3-l3-r3`。bundle SHA-256 `efa0dd4baf27aef75c1c796e0ec8828d7d67037bfbf9472a061a36cab5f4b372`，plan SHA-256 `2782ea3b253d709c524fb4243980ec56f1ad5ee721cd6db0cb5cb7aad935e189`，远端脚本 SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，L3 日志 SHA-256 `7f2ee5f333b5a42b5ed79c6eaf06492688b88c6428e4e60ee98b51473f8e7b67`，manifest SHA-256 `5ec121fbe3869815e9ea68d7734f0a03510a9462906b3186208e25bb906390f1`。
- L3 通过项目：Go 全量测试、`internal/panel`/`internal/auth`/`internal/dockerx` race、Web 180 files/1578 tests、typecheck/build、govulncheck、npm audit、Trivy、双架构构建、managed script contract、镜像运行时限制和 app config lifecycle。

## 发布产物与通道状态

- GitHub Release workflow：[#243](https://github.com/kejilion/KPanel/actions/runs/35683230345) 成功；公开页面 [KPanel v1.21.0-rc.11](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.11) 为非 draft、Pre-release、非 Latest。API 返回 14 个上传资产，连同 GitHub 自动生成的 source zip/tar.gz 共 16 个附件；annotated tag object 为 `cc8d0a532751a073e08d40a6c72e0cfa9d8b1264`。
- Docker 公开镜像：`1.21.0-rc.11` 与 `preview` OCI index 均为 `sha256:3343d8421ad8fd6baeedf95e43b276a24afcefcafdf0bce7ab7d1ca696ddf90c`；`linux/amd64` 为 `sha256:9d07ffd39f12a3cb90cdd4bdc1f9757b34e63f7e050bbd00c00317702544d7b5`，`linux/arm64` 为 `sha256:e1b831668ec0e43775ea35fd9609a8dd83caa2786ecb8f8d362e5dc409868088`，另有两个 provenance/attestation manifests。Release notes 已记录同一 immutable digest。
- Release `SHA256SUMS` 资产摘要为 `sha256:c6d9129a39e6337b21b1f59328ed633abf32233baf785d1656b20d30556a170d`。GitHub Latest 仍为 `v1.20.0`；Docker `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`，应用市场稳定默认入口与生产入口未改动。

## 生产部署安全核对

- 正式部署环境：不适用；本轮预览发布禁止生产写入。
- `prod-108`：未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：0。
- 回滚：预览异常时将 `preview` 来源固定回 `v1.21.0-rc.10`，其已验证 OCI digest 为 `sha256:ac5b86cb69d4e8e2485e41913040bf4b17d5a208e163b7cb6eabe2fc66a16dc1`；稳定入口和 Docker `latest` 不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T08:01:08+08:00
- 候选冻结时间：2026-09-22T11:08:05+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/business-context/stale-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 L3 预检发现业务上下文距基线 52 个提交，候选代码未执行。",
    "recoveryEvidence": "刷新 docs/product-quality-review-current.md 至基线 6e9d2d82，随后 business-context freshness 通过。",
    "permanentAction": "每次候选冻结前先运行 business-context freshness，并以最近已发布稳定版本维护 canonical review。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/orchestrator/candidate-argument-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次手工 L3 调用传入了与 HEAD 不同的候选 SHA，入口 fail-closed。",
    "recoveryEvidence": "run-release-l3.mjs 返回 candidate does not match HEAD，未启动远端执行；随后改用精确 f1cfab37 重试。",
    "permanentAction": "发布命令执行前固定从 git rev-parse HEAD 复制候选 SHA，并在新 run ID 下重试。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/orchestrator/artifact-retry-collision",
    "position": "before-production-write",
    "count": 1,
    "impact": "参数失败留下的受管证据目录阻止同 run ID 重用，避免覆盖无效证据。",
    "recoveryEvidence": "入口返回 artifact directory already exists，随后使用新 run ID/path `...-r2` 和 `...-r3`，证据保持不可变。",
    "permanentAction": "每次 L3 重试都生成新 run ID 和新 artifact 目录，不复用失败目录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "默认 arena-154 L3 在 SSH 连接阶段超时，候选代码未执行，原证据不计入产品结论。",
    "recoveryEvidence": "C:/GitHub/_release-evidence/v1.21.0-rc.11-f1cfab3-l3-r2 的 manifest 与 SSH timeout 输出；随后 local-wsl-dr r3 passed。",
    "permanentAction": "保留 local-wsl-dr 候选验证回退路径，并在下一次 L3 前复核 arena-154 SSH 可达性。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 真实目标服务/硬件认证器矩阵、长期 soak、arena-154 真实目标服务和生产部署均未执行，属于预览版未覆盖范围。
- 稳定版前必须以新的稳定候选重新执行 required security coverage、稳定 L3、生产灰度和回滚演练；本 RC 的 `scoped-required` 不能替代稳定版覆盖门禁。
- 候选分支、L3 证据和 Release 证据保留用于后续复核；不删除或强制清理历史证据。
