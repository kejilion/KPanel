# KPanel v1.19.0-rc.4 发布验收记录

日期：2026-09-17

发布级别：L3

候选提交 / 标签：`6a34d7a3dd2f29c62edbb6e033e487b83fdc9cea` / `v1.19.0-rc.4`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`

`releaseChannel`：`preview`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 预览版保留

## 发布画像

- 业务域：终端批量执行、轻量节点接入安全。
- 变更面：展示与交互（批量输出折叠、快捷命令填充）、终端会话存活语义（轮询即存活）、enrollment 密钥比较算法；不新增宿主机写入、协议、数据迁移或生产部署。
- 受影响用户旅程：批量执行多台主机并阅读输出的折叠/展开状态、在批量模式调用快捷命令（填充而非立即执行）、长任务（最长 4 小时）静默执行不被空闲回收、轻量节点批量接入令牌校验。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：中等偏轻；无生产写入，主要风险在批量会话容量与长任务存活语义，由定向测试、L3 全量与公开镜像冷启动覆盖。

## 发布范围与未纳入内容

- 用户可见更新：批量执行默认折叠已完成主机输出（可全部展开/收起）；快捷命令进入批量模式（填充自定义命令框，不立即执行）；单机执行上限 30 分钟提升到 4 小时且静默轮询保持会话存活；轻量节点 enrollment 密钥改为常数时间比较；gosec 覆盖说明刷新与终端空闲保活参数固定。
- 精确提交清单（5 产品提交 + 3 流程提交）：`bacef759`、`44a9e4df`、`d028518a`、`cc1e4118`、`106dd5bd`、`3706089d`（版本准备）、`0f1cbc95`（gofmt）、`6a34d7a3`（业务事实刷新，同时记录为治理提交）。
- 明确未纳入的分支、文件或后续事项：7 条 rc.3 已吸收功能分支仍保留未回收（内容等价性已验证，属可选清理）；稳定发布、应用市场配置变更、受管脚本变更和生产部署不在本轮范围。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`（与公开镜像 `/release/kejilion.sh` 实测一致）。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：两源分支对 `cmd/kejilion-node/update_runtime/source.json` 与 `packaging/kejilion-app/kpanel.conf` 零差异；L3 受管脚本契约、目标烟测和应用生命周期全部通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 r1 全量 Go/前端测试通过；`app_conf_lifecycle=pass`；浏览器验收批量执行 2/2 成功、折叠/展开/快捷命令填充断言通过。 | 未对真实大规模主机并发执行批量命令压测。 |
| 网络入侵与供应链安全 | 已验证 | L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过；OCI revision `6a34d7a3`、内嵌脚本 SHA、`SHA256SUMS` 均核对；gosec 新增规则命中均分类为规范要求或已限界路径。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 应用安装/更新/中断回滚/卸载故障注入通过；静默会话空闲回收回归测试证明轮询保活；公开镜像冷启动 E2E 含端口/容器/网络残留断言。 | 4 小时上限未做全时长真实 soak。 |
| 性能与资源预算 | 不适用 | 预览发布且未部署生产；构建、测试和冷启动无超时或 OOM。 | 未采集真实规模节点长期 P95 与资源曲线。 |
| 用户体验与可访问性 | 已验证 | 验收级浏览器预览（visual-composition）覆盖批量执行折叠态/展开态/快捷命令抽屉（aria-expanded）/长输出阅读；控制台无 JS 错误。 | 未单独完成人工三语矩阵与 200% 缩放。 |
| 数据、配置与迁移 | 已验证 | 无数据库 schema、端口、Compose、节点身份变化；快捷命令数据结构复用 rc.3 已发布形态。 | 不适用。 |

## 自动门禁

- 定向测试及结果：`BatchTerminalPanel.test.ts`、`TerminalView.batch.test.ts`、`manager_test.go`（含新增 35 分钟静默轮询存活测试）随 L3 全量通过；gofmt 修复后 gate 镜像内核验通过。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_runner=pass commit=6a34d7a3`；`app_conf_lifecycle=pass`；`verification_preflight=pass platform=Linux level=release`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-rc.4-6a34d7a3-l3-r1`；plan `1a24b3db2a7a2c8b75533eb205f6eb244a2eab9a11daf57af7ec2457d4b92721`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `230b5cfc15fcb7ec225737e527e54565be8a158db267c55b1e794f96ec746c50`，远端日志 `d7a8205e9d47972fd589a07cf9adc7a7e3889f393263b08b8abf3f08fb38e465`；2026-09-17T07:00:50Z 至 07:17:34Z，status `passed`、exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.4-6a34d7a3-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.4-6a34d7a3-l3-r1`。
- 候选 CI：[CI 35193836261](https://github.com/kejilion/KPanel/actions/runs/35193836261) 与 [freshness 35193836267](https://github.com/kejilion/KPanel/actions/runs/35193836267) 成功，均绑定最终 SHA。
- 主线 CI：[CI 35194470572](https://github.com/kejilion/KPanel/actions/runs/35194470572) 与 [freshness 35194470597](https://github.com/kejilion/KPanel/actions/runs/35194470597) 成功，均绑定最终 SHA；[tag freshness 35195111981](https://github.com/kejilion/KPanel/actions/runs/35195111981) 成功；业务刷新分支 [CI 35192361516](https://github.com/kejilion/KPanel/actions/runs/35192361516) 成功。
- Release workflow：[Release 35195111933](https://github.com/kejilion/KPanel/actions/runs/35195111933) 成功。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-17 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit、Trivy 与本次 gosec v2.28.0 全量复核（278 文件）通过，新增命中均为规范要求（RFC 6238 SHA-1）、常量或已限界路径。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions；`web/package.json` 仅含版本字段同步。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0-rc.4`；公共 OCI index 为 `sha256:f027c6821af123c6f009677bcd39ade0f96d79fed0120ada571239310568e96a`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.18.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`6a34d7a3dd2f29c62edbb6e033e487b83fdc9cea` 与 `docker.io/kjlion/kejilion-panel@sha256:f027c6821af123c6f009677bcd39ade0f96d79fed0120ada571239310568e96a`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-rc.4-6a34d7a3-l3-r1` passed/0；验收级浏览器预览 `v1-19-0-rc-4-r1`（acceptance / visual-composition，3 条旅程，manifest SHA-256 `1201eb077f50254c471c70d6bea7f250f03e6f24551d6645bb6e468148c004f5`）位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.4-3706089d-local-preview-r1`，浏览器断言明细见同目录 `browser-verification.md`（批量 2/2 成功、输出默认折叠 `visible=false`、全部展开后可见、快捷命令填充 `docker ps` 而非立即执行、抽屉自动关闭）；公开 OCI E2E r1 exit 0，run.sh `70cf4e45a6e715c13a93f970d9c7b89e96f878916e20bf05529ab7c1c98ac859`，固定 image-e2e `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 `26983d2abd7f974b3b0a04c437b239e31035fa92bfb1fab6cf56412230449562`；证据位于 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.4/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次验收级浏览器旅程和公开镜像冷启动；无 soak，长任务存活由 35 分钟模拟时钟回归与 4 小时上限内的会话容量测试覆盖。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：模拟数据预览覆盖批量折叠/展开/快捷命令抽屉与执行反馈；控制台无 JS 错误（mock 未实现的 `/api/v1/system/public-network` 404 为既有边界，与本版变更无关）。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：生产部署、生产故障注入、原生 arm64、弱网、真实断电、4 小时全时长 soak、200% 缩放和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0-rc.4](https://github.com/kejilion/KPanel/releases/tag/v1.19.0-rc.4) 于 2026-09-17T07:42:30Z 公开，为非 draft、prerelease、非 Latest；annotated tag 对象 `3909a4e3ea110a31c7d067f0d771d968496048a5` peeled 到最终产品 SHA `6a34d7a3`。GitHub Latest 仍为 v1.18.0。
- Docker 版本与通道 OCI index：`1.19.0-rc.4` 与 `preview` 同为 `sha256:f027c6821af123c6f009677bcd39ade0f96d79fed0120ada571239310568e96a`；稳定 `latest` 仍为 `sha256:9026261e44f4689a6c078690a8608158404d2fd57a5b3d932f197e33082260f9`，未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:f027c6821af123c6f009677bcd39ade0f96d79fed0120ada571239310568e96a`；arm64 `sha256:df442097bedb1fd3fa1e8cf6fd3a3352ac7a0316c1f6a58fb8c8830e31d99ac8`；attestation manifest `sha256:65ad612c...`（amd64）与 `sha256:7a394cfe...`（arm64），`unknown/unknown` 条目为 provenance/SBOM，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、deploy tar.gz、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES）。
- 公开镜像 `image_e2e=pass`：`arena-154` 按不可变摘要拉取，以 `docker create`/`docker cp` 只读提取元数据（revision=`6a34d7a3`、VERSION=`1.19.0-rc.4`、脚本 SHA 一致、非 root `65532:65532`）后运行固定 `image-e2e.sh`；输出 `image_e2e=pass` 与 `public_oci_e2e=pass`，端口/容器残留/网络残留断言通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca` 的 `kpanel.conf` 归一化内容一致，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` main 保持 `4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用 rc.1-rc.3 已验证实现，本轮未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核）。
- 预览版保留候选分支：`release/v1.19.0-candidate` 保留并指向 `6a34d7a3`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未部署、未升级、未核对。
- 生产已执行写操作：本次为 0。

## 回滚

- 源码/tag：稳定回滚点 `v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`。
- 镜像 digest：稳定 `latest` 保持 `sha256:9026261e44f4689a6c078690a8608158404d2fd57a5b3d932f197e33082260f9`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择 v1.18.0 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.18.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.18.0 / `sha256:9026261e...`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.18.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-17T14:44:53+08:00
- 候选冻结时间：2026-09-17T14:59:00+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3-invocation/candidate-sha/manual-typo",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次 L3 启动手写完整候选 SHA 拼错后半段，入口 fail-closed 拒绝执行（第一次无产物目录，第二次留下按规范保留的 r1 目录）。",
    "recoveryEvidence": "改用 `SHA=$(git rev-parse HEAD)` 变量传递后一次通过；两个失败证据目录原样保留。",
    "permanentAction": "L3 与一切门禁的 SHA 参数只允许从 git 程序化读取，禁止手写；已写入执行记忆并在本验收记录固化。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-candidate/gofmt/manager-test-field-alignment",
    "position": "before-production-write",
    "count": 1,
    "impact": "首个完整候选在 L3 gofmt 检查失败（manager_test.go Config 字段冒号未与 Starter 对齐）；Windows 本机无 go 无法预诊。",
    "recoveryEvidence": "经 arena-154 gate 镜像 `gofmt -d` 定位单行修复，gofmt-clean 核验后以新提交重建候选，L3 r1 通过。",
    "permanentAction": "候选在提交前经固定 gate 镜像执行 gofmt -l 预检（主机无裸 gofmt）；上游功能分支任务应在交付门前自查格式。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-candidate/business-context-stale/50-commit-threshold",
    "position": "before-production-write",
    "count": 1,
    "impact": "第二次完整 L3 在业务事实新鲜度门禁被拦：50 提交阈值恰好达到，`product-quality-review-current.md` 基线停在 v1.17.0。",
    "recoveryEvidence": "对照 v1.17.0→v1.19.0-rc.3 区间 50 提交完成增量复核段与基线刷新（commit 6a34d7a3），本地门禁通过后 L3 r1 通过；刷新同时推独立分支过 CI。",
    "permanentAction": "发布任务在准备候选时先本地跑 `check-business-context-freshness.mjs`，接近阈值（≥45 提交）即提前刷新业务事实，不把治理刷新留给 L3 才发现。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：验收预览进程、临时 E2E 容器和网络已停止或清理；保留候选工作树、远端候选分支、L3（3 失败 + 1 通过证据目录）、公开 OCI r1 及本地预览证据，供同一 `1.19.0` 序列继续追加 RC；7 条 rc.3 已吸收功能分支与 worktree 经用户决定暂不回收，列为后续可选清理项。
- 未验证风险：真实大规模主机批量执行并发、4 小时全时长真实任务、原生 arm64、弱网、真实断电、长期 soak、200% 缩放、人工三语矩阵和生产数据恢复。
- 已实现待实机准入：批量执行输出折叠在真实多主机长输出的可读性、快捷命令批量填充的实际使用节奏需要在 RC 反馈期继续观察。
- 不阻断本版的理由：最终 SHA 已通过验收级浏览器预览、完整 L3、候选/main/tag 门禁、双架构公开 OCI 与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：rc.3→rc.4 已沿用 `release/v1.19.0-candidate`；进入稳定版 v1.19.0 前需预览用户实际安装反馈、真实通知与节点专项确认；公开 OCI 元数据提取与 E2E 源码入口收敛为仓库固定脚本的三次重复诉求仍未完成，优先级上调。
