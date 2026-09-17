# KPanel v1.19.0-rc.5 发布验收记录

日期：2026-09-17

发布级别：L3

候选提交 / 标签：`e0c4739b9a28a42a19ab9cb645abc8d3d7d095b2` / `v1.19.0-rc.5`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`

`releaseChannel`：`preview`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 预览版保留

## 发布画像

- 业务域：终端批量执行、桌面分屏。
- 变更面：展示与交互（停止按钮、展开滚动、滚动条/布局）、批量执行完成判定与 exit 发送时序（前端逻辑）、单侧吸附分隔条显示条件；不新增后端协议、宿主机写入、数据迁移或生产部署。
- 受影响用户旅程：批量执行运行中手动终止、菜单类命令（`k 更新` 等）或子 shell 后正确判完成、普通输出 `$` 结尾不再误判完成、展开结果跟读最新输出、单侧吸附窗口拖动分隔条调宽并持久化。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：中等；完成判定收严改变批量主机的状态时序（mock 环境无真实提示符行时主机保持"命令执行中"为预期新语义），由组件测试、浏览器验收与公开镜像 E2E 覆盖。

## 发布范围与未纳入内容

- 用户可见更新：批量执行运行中"执行命令"替换为"终止执行"（终止待执行主机、关闭全部会话、未完成标记"已手动终止"）；完成判定只匹配 `user@host:path$/#` 真实提示符；`exit` 改在提示符稳定后单独发送，菜单吞尾随 exit 也能完成；展开主机结果或全部展开时输出块滚动到底部；批量面板滚动条与终端色调对齐；批量 stage 移动端竖屏与桌面布局修复；桌面单侧吸附窗口显示分隔条可调宽（对侧稍后吸附沿用已存比例）。
- 精确提交清单（8 产品提交 + 1 版本提交）：`dfd3d65a`、`d503798d`、`95c3e827`、`4aa9e716`、`f7123331`、`371fd941`、`7ca7a60b`、`90421eda`、`e0c4739b`。
- 明确未纳入的分支、文件或后续事项：`fix/light-secret-constant-time` 的 `5d5256fb` 与 rc.4 已发布 `bacef759` 为同一补丁（patch-id 相同）未重复纳入；rc.3/rc.4 已吸收历史分支按用户决定继续保留；稳定发布、应用市场配置变更、受管脚本变更和生产部署不在本轮范围。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`（与公开镜像 `/release/kejilion.sh` 实测一致）。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：本轮全部提交为前端与测试改动，对 `cmd/kejilion-node/update_runtime/source.json` 与 `packaging/kejilion-app/kpanel.conf` 零差异；L3 受管脚本契约、目标烟测和应用生命周期全部通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 组件测试 BatchTerminalPanel 7/7（含菜单 exit、`$` 结尾防误判）、DesktopView.layout 21/21；浏览器实证终止/展开/单侧分屏/持久化；L3 全量 + `app_conf_lifecycle=pass`。 | 真实菜单类命令（如 `k 更新`）在真机 shell 的完成行为未实测。 |
| 网络入侵与供应链安全 | 已验证 | L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过；OCI revision `e0c4739b`、内嵌脚本 SHA、`SHA256SUMS` 均核对。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 应用安装/更新/中断回滚/卸载故障注入通过；公开镜像冷启动 E2E 含端口/容器/网络残留断言。 | 未做 4 小时全时长真实 soak（rc.4 上限语义未变）。 |
| 性能与资源预算 | 不适用 | 预览发布且未部署生产；构建、测试和冷启动无超时或 OOM。 | 未采集真实规模节点长期 P95 与资源曲线。 |
| 用户体验与可访问性 | 已验证 | 验收级浏览器预览（visual-composition）覆盖终止按钮语义/输出展开跟读/单侧分隔条（role=separator、aria-label）与拖动反馈；控制台无 JS 错误。 | 未单独完成人工三语矩阵与 200% 缩放。 |
| 数据、配置与迁移 | 已验证 | 无数据库 schema、端口、Compose、节点身份变化；分屏比例继续沿用 `kpanel:desktop-side-split:v1`。 | 不适用。 |

## 自动门禁

- 定向测试及结果：`BatchTerminalPanel.test.ts` 7/7、`DesktopView.layout.test.ts` 21/21 本地通过并随 L3 全量复验；本轮候选无产品 Go 改动（仅 `internal/version/version.go` 版本字段）。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_runner=pass commit=e0c4739b`；`app_conf_lifecycle=pass`；`release_l3_gate=pass`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-rc.5-e0c4739b-l3-r1`；plan `2ecbd415f169e515f1f2f89ead32466ad1547e0a20133b76e7101fc15477054e`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `af46b7e8c7b9e6dd45cffedce901ee310fd13103ce19d74fdc0b24d4762e7220`，远端日志 `03aa656ac92749f9eab051e4e0a942ff160f9dd99eb750a58ef6889c0e69d5e3`；2026-09-17T09:30:42Z 至 09:47:07Z，status `passed`、exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.5-e0c4739b-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.5-e0c4739b-l3-r1`。
- 候选 CI：[CI 35207215433](https://github.com/kejilion/KPanel/actions/runs/35207215433) 与 [freshness 35207215442](https://github.com/kejilion/KPanel/actions/runs/35207215442) 成功，均绑定最终 SHA。
- 主线 CI：[CI 35208040060](https://github.com/kejilion/KPanel/actions/runs/35208040060) 与 [freshness 35208040048](https://github.com/kejilion/KPanel/actions/runs/35208040048) 成功，均绑定最终 SHA；[tag freshness 35208753621](https://github.com/kejilion/KPanel/actions/runs/35208753621) 成功。
- Release workflow：[Release 35208753892](https://github.com/kejilion/KPanel/actions/runs/35208753892) 成功（2026-09-17T10:16:16Z 公开）。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-17 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions；`web/package.json` 仅含版本字段同步。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0-rc.5`；公共 OCI index 为 `sha256:066c9a980e15f75e367845854a4e22084022c17b4b7eb3dfaf5e2d8690522296`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.18.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`e0c4739b9a28a42a19ab9cb645abc8d3d7d095b2` 与 `docker.io/kjlion/kejilion-panel@sha256:066c9a980e15f75e367845854a4e22084022c17b4b7eb3dfaf5e2d8690522296`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-rc.5-e0c4739b-l3-r1` passed/0（09:30:42Z 至 09:47:07Z）；验收级浏览器预览 `v1-19-0-rc-5-r1`（acceptance / visual-composition，4 条旅程，manifest SHA-256 `05c871adb71305fc0e3d54931a542a39636eeea3660abfb599cb6585128b1104`）位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.5-e0c4739b-local-preview-r1`，断言明细见同目录 `browser-verification.md`（终止按钮替换与即时"已手动终止"、mock 无真实提示符行时主机停留"命令执行中"为收严后正确语义、展开详情 `.batch-result__detail` 与 `scrollbarColor rgb(53,75,68) rgb(11,20,18)`、单侧吸附 `winSnapped=true` 且分隔条 `role=separator/aria-label=调整吸附窗口宽度` 可见、拖动后窗口宽度跟随并持久化 `0.6133`）；公开 OCI E2E r1 exit 0，run.sh `d8ba283fdf99b2a9fb303e935539c719003489c2e8c35e4527390bef0ba41556`，固定 image-e2e `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 `26983d2abd7f974b3b0a04c437b239e31035fa92bfb1fab6cf56412230449562`；证据位于 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.5/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次验收级浏览器旅程和公开镜像冷启动；无 soak，完成判定时序由组件测试与浏览器交互覆盖。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：模拟数据预览覆盖批量终止/展开/分屏拖动与焦点反馈；进入桌面模式被浏览器抑制的 native confirm 拦截时按预期拒绝（有确认语义），覆盖 `window.confirm` 后通过。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：真实 shell 菜单命令完成行为、真机批量并发、原生 arm64、弱网、真实断电、长期 soak、200% 缩放和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0-rc.5](https://github.com/kejilion/KPanel/releases/tag/v1.19.0-rc.5) 于 2026-09-17T10:16:16Z 公开，为非 draft、prerelease、非 Latest；annotated tag 对象 `eaf8b4d54557f6e355a0cdea622508f505170692` peeled 到最终产品 SHA `e0c4739b`。GitHub Latest 仍为 v1.18.0。
- Docker 版本与通道 OCI index：`1.19.0-rc.5` 与 `preview` 同为 `sha256:066c9a980e15f75e367845854a4e22084022c17b4b7eb3dfaf5e2d8690522296`；稳定 `latest` 仍为 `sha256:9026261e44f4689a6c078690a8608158404d2fd57a5b3d932f197e33082260f9`，未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:066c9a980e15f75e367845854a4e22084022c17b4b7eb3dfaf5e2d8690522296`；arm64 `sha256:86eb8cad18c70d9d990083a08618fdf224fc5a16dfb5405cf0a2969af6b20920`；attestation manifest `sha256:77644a0e...`（amd64）与 `sha256:82213b58...`（arm64），`unknown/unknown` 条目为 provenance/SBOM，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、deploy tar.gz、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES）。
- 公开镜像 `image_e2e=pass`：`arena-154` 从已验证 L3 bundle 检出源码（bundle SHA `af46b7e8` 核对一致）、按不可变摘要拉取，以 `docker create`/`docker cp` 只读提取元数据（revision=`e0c4739b`、VERSION=`1.19.0-rc.5`、脚本 SHA 一致、非 root `65532:65532`）后运行固定 `image-e2e.sh`；输出 `image_e2e=pass` 与 `public_oci_e2e=pass`，端口/容器残留/网络残留断言通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca` 的 `kpanel.conf` 归一化内容一致，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` main 保持 `4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用 rc.1-rc.4 已验证实现，本轮未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核）。
- 预览版保留候选分支：`release/v1.19.0-candidate` 保留并指向 `e0c4739b`。

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
- 首个纳入提交时间：2026-09-17T17:22:39+08:00
- 候选冻结时间：2026-09-17T17:30:00+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-review/stale-inventory/mid-preparation-upstream-growth",
    "position": "before-production-write",
    "count": 1,
    "impact": "rc.5 首轮选点时上游批量执行分支仍在新增提交，只到 +2 就开始组装候选；用户叫停后确认分支已增至 +7，首轮 wip 组装过时（含未提交版本文件改动，未推送）。",
    "recoveryEvidence": "作废整个 wip worktree 重建，重新基于 +7 全量与单侧分屏 +1 组装 9 提交候选；已 cherry-pick 的两提交经树级比对与最终候选一致，无内容丢失。",
    "permanentAction": "组装发布候选前必须确认每条源分支处于静止状态（worktree clean 且最近一次提交时间已停止推进），分支仍在生长时等待或向用户确认截点；不基于活跃分支的中间态组候选。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：验收预览进程、临时 E2E 容器和网络已停止或清理；rc.5 一次成稿，无失败 L3 目录；保留候选工作树、远端候选分支、L3 r1、公开 OCI r1 及本地预览证据，供同一 `1.19.0` 序列继续追加 RC。
- 未验证风险：真实 shell 菜单命令（`k 更新`）完成时序、真实大规模主机批量并发、4 小时全时长任务、原生 arm64、弱网、真实断电、长期 soak、200% 缩放、人工三语矩阵和生产数据恢复。
- 已实现待实机准入：批量手动终止与收严判定的真实使用反馈、单侧分屏比例在真实多窗口场景的复用需要在 RC 反馈期观察。
- 不阻断本版的理由：最终 SHA 已通过验收级浏览器预览、完整 L3（一次通过）、候选/main/tag 门禁、双架构公开 OCI 与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：`1.19.0` 已连续 5 个 RC，建议下一轮评估转稳定版 v1.19.0（需预览用户实际安装反馈与真机菜单命令完成专项确认）；发布准备期的"分支是否仍在生长"检查应在组装候选前显式确认（本版教训）；公开 OCI 元数据提取与 E2E 源码入口收敛为仓库固定脚本已连续 4 个版本诉求，优先级最高。
