# KPanel v1.20.0-rc.3 发布验收记录

日期：2026-09-19

发布级别：L3

候选提交 / 标签：`184a1c7677af0aaa8716497d74c3484c7610414b` / `v1.20.0-rc.3`

上一稳定版本 / 回滚点：`v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`

`releaseChannel`：`preview`

`releaseTrain`：`1.20.0`

候选分支与发布后处置：`release/v1.20.0-candidate` / 预览版保留

## 发布画像

- 业务域：审计存储（审计历史从状态文件迁入独立 SQLite 审计库）、本地密码恢复审计顺序、发布治理（规范执行健康度试行、open-code-review pin 升级）。
- 变更面：Go 后端存储与调用方（`internal/store/audit_log.go`、`internal/store/store.go`、`internal/panel/server.go`、`internal/panel/jobs.go`、`internal/auth/service.go` 等）；首次启动的数据迁移（`panel-state.json` 的 `audit` 迁入同目录 `panel-state-audit.db`）；治理文档、工作流与脚本（`PROJECT_RULES.md` 5.2.4/5.2.7、`scripts/report-governance-health.mjs`、`dependency-policy.json`）。无前端文件改动，无新增 Go/npm 依赖（`modernc.org/sqlite` 已在用，纯 Go、`CGO_ENABLED=0`）。
- 受影响用户旅程：升级后首次启动的自动迁移；审计页与任务页读取审计；登录/会话等状态写入延迟；降级到旧版本及再次升级；本地密码恢复。
- 未变化契约：审计 API 形状、端口、Compose、Agent 权限模型、`kejilion.sh` 脚本契约、应用市场稳定配置和公开稳定更新入口不变；KPanel 备份仍不导出审计，主机级备份仍排除 `/var/lib/kejilion-panel`。
- 风险等级及理由：中等；涉及数据迁移，但迁移按事件 ID 去重导入、中断可重做，源分支 L2 通过，本候选通过完整 L3 与隔离真机升级/降级/再升级演练（约 1 万条记录）。

## 发布范围与未纳入内容

- 用户可见更新：见 `CHANGELOG.md` `[1.20.0-rc.3]`。
- 精确提交清单（10 产品提交 + 3 合并提交 + 2 发布提交）：
  - `docs/governance-execution-health`（合并 `9b1df924`）：`c35b8fa3`、`6cdcd1cb`、`fa28a484`、`fc054664`、`5b7bfcc3`、`9436fdb9`、`eb910b48`。
  - `docs/ocr-upgrade-1.12.6`（合并 `7579e2cc`）：`1042970a`。
  - `perf/audit-storage-20260919`（合并 `dd2c27ec`）：`a5d049e0`、`d70b9d50`。
  - `4861bb90`（版本准备）、`184a1c76`（业务事实基线刷新至 v1.20.0-rc.2 验收提交，冻结提交）。
  - 三条合并均无冲突。
- 明确未纳入的分支、文件或后续事项：`fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 与 main 冲突且含未完成 WIP；`ops/restore-v1100-channel` 已过时；rc.2 已纳入的 5 条源分支仍在远端，待归档决定。规范执行健康度"3 份超期 + 2 份状态不可归类提案"须在下一稳定版发车前的质量审计中处置，本预览版不处理。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：候选对 `packaging/`、`Dockerfile`、`.github/`、`go.mod`/`go.sum` 与 `web/package*.json`（版本字段除外）相对 `v1.20.0-rc.2` 零差异；差异不涉及脚本协议、运行时动作、宿主机产物或安装/更新路径；L3 `app_conf_lifecycle=pass`。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | `internal/store` 审计库、迁移、分页、重复 ID 与关闭后写入测试；L3 全量 Go 测试与 web 162 个测试文件通过；隔离演练中升级前后审计 API 分页计数一致（9907/9907）。 | 未在真实生产规模数据目录上迁移。 |
| 网络入侵与供应链安全 | 已验证 | 审计库被删除或替换时写入失败即拒绝；审计不可用时 API 返回 503；无新增依赖；L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 隔离演练：升级、重启、降级到 rc.2、再次升级均健康，无重复、无丢失（`downgrade_records_missing_after_reupgrade=0`），容器日志 panic/fatal 为 0；迁移中断可重做由单元测试覆盖。 | 未在迁移过程中注入断电；arm64 未实机演练。 |
| 性能与资源预算 | 已验证 | `docs/audit-storage.md` 候选实测：满 1 万条追加 P95 约 1.2–1.6 ms（原约 160 ms）、会话写 P95 2.6–4.2 ms、峰值 RSS 约 18 MB（原约 68 MB）；隔离演练迁移 9907 条后健康检查 856 ms 内就绪，状态文件由 3,135,031 字节降至 4,052 字节。 | 演练未单独计时迁移本身；未采集生产规模资源曲线。 |
| 用户体验与可访问性 | 不适用 | 无前端文件改动；审计不可用时任务页标记来源不可用由后端 `jobs.go` 与测试覆盖。 | 不适用。 |
| 数据、配置与迁移 | 已验证 | 首次启动自动迁移并从状态文件移除 `audit`；降级后旧版本只显示降级期间新记录（演练 1 条），再次升级按事件 ID 合并（9911 条、唯一）；备份导出范围不变。 | 手工拷贝运行中的数据目录可能得到不一致 SQLite 副本，已写入升级说明。 |

## 自动门禁

- 定向测试及结果：本机 `node --test scripts/tests/*.test.mjs` 190/190 通过；`check-governance-consistency`、`report-governance-health --validate`（13 份提案）、`check-version-consistency`、`check-business-context-freshness`（baseline `9d584e8`，15 提交）通过；源分支 `perf/audit-storage-20260919` L2 通过并已写 OCR trailer。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_preflight=pass`；`app_conf_lifecycle=pass`；`release_gate_runner=pass commit=184a1c76`；`release_l3_gate=pass`；`release_l3_remote=pass`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：r1 `v1.20.0-rc.3-4861bb90-l3-r1` 在入口业务事实新鲜度检查 fail-closed（候选代码未执行，见流程异常）；r2 `v1.20.0-rc.3-184a1c76-l3-r2`：plan.env `00a1e042cfb5f6e4b81f56d2507630d9af4bba05c05d03fbd4db53f9a1f6d84b`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `f082d9a985cb374c97a9c71d4a21a062580517abd5b681f989e5f0c021c2495b`，manifest `9f3413437381d22cbfd47c1171a8a306bc856c2deec1197604742fc0c84c8b30`，远端日志 `e3acffb670cc701557aa83c80239f70061eae35534d8e62758a14ad8ce42b2fa`；2026-09-19T13:54 启动至 14:17 结束（+0800），passed、exit 0；证据位于 `C:/GitHub/_release-evidence/v1.20.0-rc.3-*-l3-r{1,2}` 与 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.3-184a1c76-l3-r2`。
- 候选 CI：[CI 35426244265](https://github.com/kejilion/KPanel/actions/runs/35426244265) 与 [freshness 35426244236](https://github.com/kejilion/KPanel/actions/runs/35426244236) 成功，均绑定 `184a1c76`。
- 主线 CI：[CI 35426607774](https://github.com/kejilion/KPanel/actions/runs/35426607774) 与 [freshness 35426607756](https://github.com/kejilion/KPanel/actions/runs/35426607756) 成功，均绑定 `184a1c76`；[tag freshness 35426983320](https://github.com/kejilion/KPanel/actions/runs/35426983320) 成功。
- Release workflow：[Release 35426983330](https://github.com/kejilion/KPanel/actions/runs/35426983330) 成功（2026-09-19T06:43:50Z 公开）。
- 安全扫描、镜像契约、SBOM/provenance：Release 工作流源码/镜像扫描、原生镜像运行时契约与受限冷启动均通过；amd64 与 arm64 均带 attestation manifest（`unknown/unknown` 条目）。
- OCR 行级评审（`PROJECT_RULES.md` 5.5）：审计存储源分支已写 `OCR-Review:` trailer；治理与 pin 升级为文档/配置改动不适用；5.5 规定发布操作不适用，本发布不补跑。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness`（35426244236、35426607756、35426983320）于 2026-09-19 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 运行时依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：`dependency-policy.json` 中 `code-review-assistant` 的 open-code-review pin 升到 1.12.6（经 canary 回放，仅本地评审辅助、不进镜像/CI）；继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.20.0-rc.3`；公共 OCI index 为 `sha256:76b54033a5f58a614ae121700539b3891f3a8d2554939cd24fa63c501506a2ba`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁、隔离迁移演练及公开 OCI 验收通过；生产和公共稳定入口继续保留 v1.19.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.x；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`184a1c7677af0aaa8716497d74c3484c7610414b`；迁移演练使用 L3 r2 最终镜像 `kejilion-panel:verify`（image ID `sha256:d581c32a74609c3161bf7379ce1a886836134e5de2fc9b3936767c948fcc19bf`，版本 `1.20.0-rc.3`）与公开 rc.2 `docker.io/kjlion/kejilion-panel@sha256:e320e392efbfe1442bbb72ad3794092570bba090564eac8bb12c2355324dc867`；公开验收使用 `docker.io/kjlion/kejilion-panel@sha256:76b54033a5f58a614ae121700539b3891f3a8d2554939cd24fa63c501506a2ba`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：迁移演练 `audit-migration-drill-r1` exit 0、`audit_migration_drill=pass`（日志 `4f0b662776a9032f923dc84f9068bb8100b23c2afeefe63f5500b5ee23b22db0`，脚本 `b26661cc604f9611901631cab3c3df2a2976a99945250513d4d461b40cac53e4`）；同脚本以 rc.2→rc.2 自测通过（`audit-migration-drill-selftest`）；公开 OCI E2E r1 exit 0（`image-e2e-r1.log` SHA-256 `717c5990f40dcf9cd3c79022222e4704c9f4df4fac2b0ee3a7b6002ec36f18de`，固定入口 `packaging/tests/image-e2e.sh` SHA-256 `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`）； 证据位于 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.3/`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：迁移演练一轮四阶段（rc.2 生成 6 条真实审计并注入 9900 条旧格式记录 → 升级 rc.3 → 重启 → 降级 rc.2 → 再升级 rc.3）；无 soak，迁移为一次性启动行为。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：不适用（无前端改动）；审计 API 通过真实会话分页读取验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器以 `--read-only --cap-drop ALL --no-new-privileges` 运行，数据目录为临时目录并已清理；升级后既有会话仍有效（`session_after_upgrade=200`）；降级与再升级均健康。
- 未执行场景及原因：迁移中途断电、原生 arm64、生产规模数据目录迁移未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.20.0-rc.3](https://github.com/kejilion/KPanel/releases/tag/v1.20.0-rc.3) 于 2026-09-19T06:43:50Z 公开，为非 draft、prerelease、非 Latest；GitHub Latest 仍为 v1.19.0。
- Docker 版本与通道 OCI index：`1.20.0-rc.3` 与 `preview` 同为 `sha256:76b54033a5f58a614ae121700539b3891f3a8d2554939cd24fa63c501506a2ba`；稳定 `latest` 仍为 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`（v1.19.0），未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：`linux/amd64` `sha256:27f07f56ff3748c34e9ee1102de9ebb68b0572ed882bc270513823ca5a7acd5a`、`linux/arm64` `sha256:67e293939d271f633e1672a0df96f11da0d6e29af8516677e2d8904c593d7c7b`；两个 `unknown/unknown` 条目为 provenance/SBOM attestation，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、`kejilion-panel-deploy-1.20.0-rc.3.tar.gz`、LICENSE、`SHA256SUMS`、THIRD_PARTY_NOTICES.md）。
- 公开镜像 `image_e2e=pass`：`arena-154` 上传 `git archive 184a1c76` 精确候选源码、按不可变摘要拉取、`docker create`/`docker cp` 只读提取元数据（VERSION=`1.20.0-rc.3`、OCI revision `184a1c76`、脚本 SHA `28cf3934...` 一致）后运行固定 `image-e2e.sh`（显式 `KPANEL_EXPECTED_VERSION=1.20.0-rc.3`）；输出 `image_e2e=pass`，无残留容器。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 相对 `v1.20.0-rc.2` 零差异，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` 内嵌脚本 SHA 保持 `28cf3934...` 基线。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用已验证实现，本轮未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核，`latest` = `sha256:351a236f...` 未变；GitHub Latest 仍为 v1.19.0；应用市场配置零差异）。
- 预览版保留候选分支：`release/v1.20.0-candidate` 保留并指向 `184a1c76`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未部署、未升级、未核对。
- 生产已执行写操作：本次为 0。

## 回滚

- 源码/tag：稳定回滚点 `v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`；上一预览 `v1.20.0-rc.2` / `7404fe29`。
- 镜像 digest：稳定 `latest` 保持 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`；上一预览 `1.20.0-rc.2` 为 `sha256:e320e392efbfe1442bbb72ad3794092570bba090564eac8bb12c2355324dc867`。
- 数据/配置备份：不适用（预览版禁止生产部署）；降级行为已在隔离演练中验证（旧版本只显示降级后新记录，历史保留在审计库，再次升级合并）。
- 回滚步骤和回滚后复核：已安装 rc.3 的测试实例可显式选择 rc.2 或 v1.19.0 摘要，按标准更新事务备份、恢复及复核；审计历史不随降级丢失。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.19.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.19.0 / `sha256:351a236f...`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.19.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-19T08:06:40+08:00
- 候选冻结时间：2026-09-19T13:53:26+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-l3/business-context-freshness/stale-before-freeze",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 在入口离线检查 check-business-context-freshness 处 fail-closed（基线后 54 个提交 ≥ 50）：候选冻结前未刷新业务事实基线，候选代码未执行，需补刷新提交并以新 run-id 重跑，约多耗 25 分钟。",
    "recoveryEvidence": "新增 184a1c76 把 docs/product-quality-review-current.md 基线刷新到 v1.20.0-rc.2 验收提交 9d584e83（v1.19.0 稳定基线版本不变，补 v1.19.0 至 v1.20.0-rc.2 增量复核），本机 freshness 通过（15 提交）后 r2 一次通过；r1 证据原样保留。",
    "permanentAction": "组装候选后、冻结前先在本机运行 check-business-context-freshness.mjs；距阈值 5 个提交内即提前刷新，把该检查列入发布执行方案预检项。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：迁移演练容器、网络、临时数据目录与临时镜像标签已清理；保留候选工作树、远端候选分支、L3 r1/r2 证据、迁移演练与公开 OCI E2E 证据，供同一 `1.20.0` 序列继续追加 RC。
- 未验证风险：迁移中途断电、原生 arm64、生产规模数据目录迁移、长期 soak。
- 已实现待实机准入：审计库在真实长期运行节点上的磁盘与 WAL 行为需要在 RC 反馈期观察。
- 不阻断本版的理由：最终 SHA 已通过完整 L3、候选/main/tag 三层门禁、隔离升级/降级/再升级演练与双架构公开 OCI 冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：迁移演练脚本可沉淀为仓库固定入口（审计以外的存储迁移同类旅程第二次出现时）；公开 OCI 元数据提取与 E2E 执行入口仓库脚本化诉求仍待处理；v1.20.0 稳定版发车前须按规范执行健康度 `--strict` 处置超期与不可归类提案。
