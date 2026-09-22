# KPanel v1.21.0-rc.13 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`e122b189f9e9606635d04f43eedea2f4d80d8b07` / `v1.21.0-rc.13`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` 保留，远端候选分支、`main` 和预览标签均精确指向 `e122b189f9e9606635d04f43eedea2f4d80d8b07`。`e122b189` 是仅用于重跑候选门禁的空提交，不改变 rc.13 的产品树；验收记录提交只会使 `main` 继续前进，不改变产品标签。预览版不执行候选分支归档，Release workflow 的归档步骤按预期跳过。

本版只发布 GitHub Pre-release 与 Docker `1.21.0-rc.13`/`preview`，不部署生产、不更新 GitHub Latest、不更新 Docker `latest`。

## 发布画像

- 业务域：Passkey 二次因子按需展示、Passkey 禁用与重新绑定流程；承接前一候选已有的 trusted-origin、桌面异步 inventory/布局和 MCP/系统管理能力。
- 本次补齐的遗漏：`0ce224fb6878bd3756a50c3cbd00c44a358f77a5`（按需展示 Passkey 二次因子）和 `04e1459e67d4ad7981eb7faa52368ee9df43963f`（Passkey 禁用与重新绑定）。候选 `0290e354` 对应的 trusted-origin 变更已在前一预览版以等价提交纳入；其余候选分支差异已核对为已合入或已归档的旧视觉改版，未再带入。
- 变更面：认证二次因子展示与凭据生命周期、审计/多语言文案；未修改 `kejilion.sh` 或其调用契约。
- 风险等级及理由：L3。涉及认证信任边界和凭据管理，完成固定 Runner L3、候选/main/tag 门禁、公开预览 Release 与双架构镜像校验。

## 覆盖检查与安全审计

- `node scripts/check-security-audit-coverage.mjs --target e122b189f9e9606635d04f43eedea2f4d80d8b07` 的最终 decision 为 `scoped-required`，不是 full coverage。原因是新增边界包：`cmd/kpanel-mcp`、`internal/mcpaccess`、`internal/mcpbridge`、`internal/tlsfallback`。
- 历史 `run-1` 的 full-run 证据 `source_dirty=true`，不作为完全可复现的 full coverage 结论；`run-2`、`run-6` 已完成并计入覆盖。当前仍有 21 个未覆盖提交、71 个文件，包含本版 Passkey 禁用/重新绑定提交 `dd3d46e5a538`；中断的 `run-3`、`run-5` 不计为覆盖。
- 这一区分仅描述覆盖状态：L3 的 Go 全量测试、核心 race、govulncheck、npm audit、Trivy 源码/依赖/镜像扫描均通过，但不把它们表述为 full security coverage 或“没有漏洞”。稳定版前仍须完成 required security coverage。

## 自动门禁与隔离验证

- 候选 Dependency freshness：[#542](https://github.com/kejilion/KPanel/actions/runs/35700034339) 成功，绑定 `dc7fdde5403bce327539a77c8019d6e86657d821`；重跑后的候选 CI [#1036](https://github.com/kejilion/KPanel/actions/runs/35700914788) 成功，绑定 `e122b189f9e9606635d04f43eedea2f4d80d8b07`。
- 主线 CI [#1037](https://github.com/kejilion/KPanel/actions/runs/35701534437) 和主线 Dependency freshness [#543](https://github.com/kejilion/KPanel/actions/runs/35701534455) 均成功并绑定同一产品 SHA；标签 Dependency freshness [#545](https://github.com/kejilion/KPanel/actions/runs/35702662516) 成功并绑定 `v1.21.0-rc.13`。
- L3 回退环境 `local-wsl-dr`：run ID `v1.21.0-rc.13-e122b18-l3-r1`，候选 `e122b189f9e9606635d04f43eedea2f4d80d8b07`，`status=passed`、`exit_code=0`；开始 `2026-09-22T07:42:57Z`，完成 `2026-09-22T07:48:49Z`。Runner image `kpanel-release-gate:go1.26.7-node24`，Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`，base tag `v1.20.0`。
- L3 证据目录：`C:/GitHub/_release-evidence/v1.21.0-rc.13-e122b18-l3-r1`。bundle SHA-256 `4b98870592a7d6134768d0048eff721407e82dfa6387a5453990656260dc1f3a`，plan SHA-256 `f5d8a054af26becca2e4bcfa7532c4dc1d87877474ddf2d2f48c2134b67db2a7`，远端脚本 SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，L3 日志 SHA-256 `9983420ba684c5ceda5cf07524943936f7d0ba58f546f4d321cf5f86c78b1f51`，manifest SHA-256 `bef5b5fdd11777ee433b7e0afd18906b1ca66ef04acf7ed63c45e2cb4b766734`。
- L3 通过项目：Go 全量测试、`internal/panel`/`internal/auth`/`internal/dockerx` race、Web 测试、typecheck/build、govulncheck、npm audit、Trivy、双架构构建、managed script contract、镜像运行时限制和 app config lifecycle。

## 发布产物与通道状态

- GitHub Release workflow [#245](https://github.com/kejilion/KPanel/actions/runs/35702662539) 成功；公开页面 [KPanel v1.21.0-rc.13](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.13) 为非 draft、Pre-release、非 Latest，API 返回 14 个已上传资产（GitHub 自动 source archive 另计）；annotated tag object 为 `9466c602ab42d7844816e40ab178f98f9e683170`。
- Docker `kjlion/kejilion-panel:1.21.0-rc.13` 与 `:preview` OCI index 均为 `sha256:fe9307f4191b439aaea0083b56ae37b5a4b23c790c43ef7748725c932c6339ea`；`linux/amd64` 为 `sha256:4d815e1c4b9e24f9dce2c026d5789207dba0913d4710dd1cb309493796c5b40d`，`linux/arm64` 为 `sha256:056a41d8454964fc1d92bd0e679958747a313e9f996a9ac3cfe20bb74b1360d5`，另有 provenance/attestation manifests。
- GitHub Latest 仍为 `v1.20.0`；Docker `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`，稳定入口与生产入口未改动。

## 生产部署安全核对

- 正式部署环境：不适用；本轮预览发布禁止生产写入。
- `prod-108`：未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：0。
- 回滚：未执行；若预览异常，将 `preview` 来源固定回上一已验证预览镜像，稳定入口和 Docker `latest` 保持不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T12:19:54+08:00
- 候选冻结时间：2026-09-22T15:41:51+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：是（前一 rc.12 发布门禁失败后重新发布 rc.13；无生产回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间: 2026-09-22T15:38:30+08:00; 恢复时间: 2026-09-22T16:13:56+08:00; 逃逸门禁: 未逃逸: 候选 CI #1036、主线 CI #1037、标签 Release #245 均成功，未进入生产。
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "ci/candidate/race-flake",
    "position": "before-production-write",
    "count": 1,
    "impact": "rc.13 首次候选 CI #1035（run 35700034439）仅在 Detect races in privileged core packages 步骤失败，产品树未进入发布阶段。",
    "recoveryEvidence": "使用新空提交 e122b189 重跑；候选 CI #1036（run 35700914788）、主线 CI #1037 和后续 L3 均成功，未改写产品提交或标签。",
    "permanentAction": "候选门禁失败时保留原始 run，使用同一产品树的新重试提交和新 run ID 验证，不跳过 race 门禁、不移动失败标签。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/verify-source/transient-failure",
    "position": "before-production-write",
    "count": 1,
    "impact": "前一 rc.12 Release #244（run 35698886207）在 Verify source 步骤失败，未生成公开 Release 或镜像 promotion。",
    "recoveryEvidence": "保留 rc.12 原始失败标签，改以精确产品 SHA e122b189 发布 rc.13；Release #245（run 35702662539）全流程成功，GitHub Release、Docker rc.13/preview 均已核验。",
    "permanentAction": "发布工作流失败后不重用失败标签；重新冻结并通过 candidate/main/tag 门禁后使用新不可变预览标签，逐项核对 Release 和镜像 digest。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 真实目标服务/硬件认证器矩阵、长期 soak、arena-154 真实目标服务和生产部署均未执行，属于预览版未覆盖范围。
- 稳定版前必须以新的稳定候选重新执行 required security coverage、稳定 L3、生产灰度和回滚演练；本 RC 的 `scoped-required` 不能替代稳定版覆盖门禁。
- 候选分支、L3 证据和 Release 证据保留用于后续复核；不删除或强制清理历史证据。
