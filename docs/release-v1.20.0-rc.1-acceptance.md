# KPanel v1.20.0-rc.1 发布验收记录

日期：2026-09-18

发布级别：L3

候选提交 / 标签：`4a39d48b7c1c61484c1e212373e3618e4119fc4c` / `v1.20.0-rc.1`

上一稳定版本 / 回滚点：`v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`

`releaseChannel`：`preview`

`releaseTrain`：`1.20.0`

候选分支与发布后处置：`release/v1.20.0-candidate` / 预览版保留

## 发布画像

- 业务域：轻量节点存储恢复、主机备份导入安全、发布治理（安全审计标准）。
- 变更面：Go 后端逻辑修复（`internal/cluster` 轻量节点 store 恢复、`internal/hostbackup`+`internal/backup` 导入根路径限定，含单元/预览测试）；治理文档与工作流定义（新增 `.codex-workflows/security-boundary-audit.workflow.yaml`、`PROJECT_RULES.md` 5.4、`dependency-policy.json` 安全审计条款、`.governance/security-audit/run-1/` 证据）；不新增前端交互、脚本协议、宿主机写入路径或数据迁移。
- 受影响用户旅程：轻量节点宿主机异常断电/进程崩溃后残留原子备份文件时，Panel 重启能恢复有效状态而非丢弃；备份中心导入导出文件时拒绝越界根路径。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限、`kejilion.sh` 脚本契约、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：低到中等；两个修复均为防御性收紧（恢复残留、拒绝越界），有单元测试与既有备份导入测试覆盖，L3 全量门禁 + 公开镜像 E2E 通过；治理改动不影响运行时。

## 发布范围与未纳入内容

- 用户可见更新：轻量节点 store 从残留原子备份恢复（不再丢弃有效状态）；主机备份导入 payload 根路径限定在导出数据模型内。
- 治理新增：信任边界安全审计标准（guidance/scoped/full 三档）、首轮 run-1 证据、`PROJECT_RULES.md` 5.4、`dependency-policy.json` 安全审计联动条款。
- 精确提交清单（4 产品提交 + 2 发布提交）：`5cd10b8a`（light store 恢复）、`e99e6b3d`、`34811da8`（备份导入根路径限定）、`be72d5b0`、`34c3afec`（安全审计标准）、`6206d9b4`（版本准备）、`4a39d48b`（业务事实基线刷新，冻结提交）。
- 明确未纳入的分支、文件或后续事项：约 96 条 1.19.0 之前的历史候选分支经盘点确认内容已全部进入 `origin/main`（`git cherry` 零独立 patch 或已被更新 SHA 在 main 重新落地），不纳入本候选；其中 `fix/file-host-switch-context-20260913` 与 `feature/visual-refinement-pass` 工作树仍有未提交 WIP，保留不归档；`fix/light-secret-constant-time` 与 rc.4 已发布补丁 patch-id 相同，未重复纳入；发布后按用户决定归档到 `archive/` 前缀。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`（与公开镜像 `/release/kejilion.sh` 实测一致）。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：候选对 `cmd/kejilion-node/update_runtime/source.json` 与 `packaging/kejilion-app/kpanel.conf` 零差异（`git diff v1.19.0..HEAD -- packaging/kejilion-app/kpanel.conf` 为空）；L3 受管脚本契约、目标烟测和应用生命周期全部通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | `light_store_recovery_test.go`（新增 54 行断言：残留备份恢复、有效状态不丢弃）；`backup_test.go` 等导入根路径测试；L3 全量 + `app_conf_lifecycle=pass`。 | 未在真实断电轻量节点上复现残留场景（单元测试以文件系统状态模拟）。 |
| 网络入侵与供应链安全 | 已验证 | L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过；OCI revision `4a39d48b`、内嵌脚本 SHA `28cf3934...`、`SHA256SUMS` 均核对；导入根路径限定本身即安全收紧。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 轻量 store 恢复即失败恢复路径修复；L3 应用安装/更新/中断回滚/卸载故障注入通过；公开镜像冷启动 E2E 含端口/容器/网络残留断言。 | 未做轻量节点长期运行 soak。 |
| 性能与资源预算 | 不适用 | 预览发布且未部署生产；修复不触及热路径（仅在启动恢复与导入校验分支）。 | 未采集生产规模资源曲线。 |
| 用户体验与可访问性 | 不适用 | 无前端交互变化；仅备份中心错误文案新增一个 i18n key。 | 不适用。 |
| 数据、配置与迁移 | 已验证 | 无数据库 schema、端口、Compose、节点身份变化；残留备份恢复兼容既有 store 格式。 | 不适用。 |

## 自动门禁

- 定向测试及结果：`light_store_recovery_test.go`、`backup_test.go`、`hostbackup` 套件随 L3 全量在本候选上通过；组件级预览测试 `test(preview): provide root paths in backup mock fixture`（`34811da8`）覆盖。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_preflight=pass commit=4a39d48b`；`app_conf_lifecycle=pass`；`release_gate_runner=pass commit=4a39d48b`；`make verify-release` 覆盖核心特权包 race、固定摘要 Trivy 源码/镜像扫描与最终镜像构建全部通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.20.0-rc.1-4a39d48b-l3-r1`；plan.env `a86cf0c206ed35588894cf38a257d58edfdb20e59057d0a6c07505d7be3319a2`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `e85d05c8b83710d5304576ab68149f372a110831fa54581426953c7ea561baa0`，manifest `69d744b5a356e4b53b22b22f7987337aed35f122b2712e974d0a95775cea4f1b`，远端日志 `d3a288cb3909d61e01b275170d78d5d06bd1351fc9fbc2edb56a4e1b3423252d`；2026-09-18T14:44 启动至 15:10 结束（本机 +0800），status `passed`、exit 0；证据位于 `C:/GitHub/_release-evidence/v1.20.0-rc.1-4a39d48b-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.1-4a39d48b-l3-r1`。
- 候选 CI：[CI 35318301432](https://github.com/kejilion/KPanel/actions/runs/35318301432) 与 [freshness 35318301672](https://github.com/kejilion/KPanel/actions/runs/35318301672) 成功，均绑定 `4a39d48b`。
- 主线 CI：[CI 35318944625](https://github.com/kejilion/KPanel/actions/runs/35318944625) 与 [freshness 35318944613](https://github.com/kejilion/KPanel/actions/runs/35318944613) 成功，均绑定 `4a39d48b`；[tag freshness 35319761101](https://github.com/kejilion/KPanel/actions/runs/35319761101) 成功。
- Release workflow：[Release 35319761099](https://github.com/kejilion/KPanel/actions/runs/35319761099) 成功（2026-09-18T07:39:30Z 公开）。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness`（35318301672、35318944613、35319761101）于 2026-09-18 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions；`web/package.json` 仅含版本字段同步。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.20.0-rc.1`；公共 OCI index 为 `sha256:ec1293152df6fdbd75843c51bf74490ec01b042ede31d75477829e2832b41e1d`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.19.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.x；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`4a39d48b7c1c61484c1e212373e3618e4119fc4c` 与 `docker.io/kjlion/kejilion-panel@sha256:ec1293152df6fdbd75843c51bf74490ec01b042ede31d75477829e2832b41e1d`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.20.0-rc.1-4a39d48b-l3-r1` passed/0（14:44 至 15:10 本机时间）；无浏览器验收作业（本轮无前端交互变化，按发布画像标记不适用，未创建空作业）；公开 OCI E2E r8 exit 0（`image-e2e-r8.log` SHA-256 `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`），固定 image-e2e 脚本入口 `1378218f9d4ac0fd...`；证据位于 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.1/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次公开镜像冷启动 + L3 全量；无 soak，恢复路径由单元测试与 L3 故障注入覆盖。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：不适用（无前端交互变化；备份导入越界根路径的用户可见失败态由既有测试与新增 mock fixture 覆盖）。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；E2E 容器以 `--read-only --cap-drop ALL --no-new-privileges` 运行；L3 覆盖更新失败、中断、安装/卸载生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：真实断电轻量节点残留恢复、真实越界备份文件导入、原生 arm64、弱网、长期 soak 和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.20.0-rc.1](https://github.com/kejilion/KPanel/releases/tag/v1.20.0-rc.1) 于 2026-09-18T07:39:30Z 公开，为非 draft、prerelease、非 Latest；GitHub Latest 仍为 v1.19.0。
- Docker 版本与通道 OCI index：`1.20.0-rc.1` 与 `preview` 同为 `sha256:ec1293152df6fdbd75843c51bf74490ec01b042ede31d75477829e2832b41e1d`；稳定 `latest` 仍为 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`（v1.19.0 生产 pin），未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：双平台均在 index 中；`unknown/unknown` 条目为 provenance/SBOM attestation，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、deploy tar.gz、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES）。
- 公开镜像 `image_e2e=pass`：`arena-154` 从已验证 L3 工作目录检出候选源码（HEAD `4a39d48b` 核对一致）、按不可变摘要拉取、`docker create`/`docker cp` 只读提取元数据（VERSION=`1.20.0-rc.1`、脚本 SHA `28cf3934...` 一致）后运行固定 `image-e2e.sh`（显式 `KPANEL_EXPECTED_VERSION=1.20.0-rc.1`）；输出 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 相对 `v1.19.0` 零差异，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` 内嵌脚本 SHA 保持 `28cf3934...` 基线。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用已验证实现，本轮未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核，`latest` = `sha256:351a236f...` 未变）。
- 预览版保留候选分支：`release/v1.20.0-candidate` 保留并指向 `4a39d48b`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未部署、未升级、未核对。
- 生产已执行写操作：本次为 0。

## 回滚

- 源码/tag：稳定回滚点 `v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`。
- 镜像 digest：稳定 `latest` 保持 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择 v1.19.0 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.19.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.19.0 / `sha256:351a236f...`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.19.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-18T13:07:48+08:00
- 候选冻结时间：2026-09-18T14:43:02+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-l3/artifact-dir/pre-created-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 首次启动被入口 fail-closed 拒绝（exit 1）：证据目录在调用前已预创建为空目录，入口要求目录不存在，重试需新 run-id 与新路径。",
    "recoveryEvidence": "删除空目录后以同一 run-id 与路径重启 L3，preflight 通过并完整执行至 passed；首轮失败未产生任何远端副作用。",
    "permanentAction": "调用 run-release-l3.mjs 前不再预创建 artifact-dir，由入口自行创建；把该前置行为写入发布执行方案核对项。",
    "historicalReleases": []
  },
  {
    "fingerprint": "arena-154/port-collision/ephemeral-source-port-18080",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开镜像 E2E 两次 attempt 失败（docker run exit 125）：xray 出站连接把 18080 选作临时源端口，与 E2E 容器 -p 127.0.0.1:18080 绑定冲突；默认 ip_local_port_range (1024-49151) 未保留该端口。",
    "recoveryEvidence": "sysctl net.ipv4.ip_local_reserved_ports=18080 后重跑 E2E 正常拉起容器；r8 以显式 KPANEL_EXPECTED_VERSION 输出 image_e2e=pass。",
    "permanentAction": "arena-154 已将 18080 写入 ip_local_reserved_ports（运行时立即生效）；后续可考虑把该设置固化进主机配置，防止重启后回退。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-evidence/oci-e2e-log-capture/ssh-quoting-empty-logs",
    "position": "before-production-write",
    "count": 1,
    "impact": "E2E 通过 SSH 嵌套重定向落盘时 6 次 attempt 输出空日志且 exit 1（r3-r7），与直接执行结果不一致，延误一次可成功运行的证据采集；根因是跨 SSH 的引号/重定向语义差异导致环境变量与管道在远端 shell 中的解析与预期不符。",
    "recoveryEvidence": "改用 scp 上传脚本后以 bash <script> 在远端原子执行（r6/r7/r8），获得带显式 KPANEL_EXPECTED_VERSION 的完整日志 image-e2e-r8.log 及 SHA-256；r1/r2 失败与 r4 无显式版本的环境回退均已保留原样。",
    "permanentAction": "跨 SSH 执行需要环境变量与重定向组合时，统一先 scp 脚本再远端执行，不再在 ssh 命令行内嵌套转义；与遗留待收敛的'公开 OCI 元数据提取与 E2E 源码入口'仓库脚本化诉求合并处理。",
    "historicalReleases": ["v1.19.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：E2E 临时容器、网络与 `/tmp/kpanel-image-e2e.*` 目录已由脚本 cleanup 清理（docker ps/network 复核无残留）；保留候选工作树、远端候选分支、L3 r1 证据、公开 OCI E2E r8 证据，供同一 `1.20.0` 序列继续追加 RC；arena-154 上 8 个早期失败/空 E2E 日志原样保留作流程异常证据。
- 未验证风险：真实断电轻量节点的残留恢复、真实越界备份文件的导入拒绝、原生 arm64、弱网、长期 soak 和生产数据恢复。
- 已实现待实机准入：残留恢复与导入根路径限定在真实集群节点上的反馈需要在 RC 反馈期观察。
- 不阻断本版的理由：最终 SHA 已通过完整 L3（一次通过）、候选/main/tag 三层门禁、双架构公开 OCI、不可变摘要元数据核对与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：公开 OCI 元数据提取与 E2E 执行入口沉淀为仓库固定脚本的诉求本版再次出现（连续第 5 个版本），优先级应升为下一列车前处理；约 96 条历史分支的 `archive/` 归档在本次发布后执行；`arena-154` 的 18080 端口保留需固化进主机配置避免重启回退。
