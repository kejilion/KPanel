# KPanel v1.20.0-rc.2 发布验收记录

日期：2026-09-19

发布级别：L3

候选提交 / 标签：`7404fe29753e0f7ad44136b369eb07b298babb58` / `v1.20.0-rc.2`

上一稳定版本 / 回滚点：`v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`

`releaseChannel`：`preview`

`releaseTrain`：`1.20.0`

候选分支与发布后处置：`release/v1.20.0-candidate` / 预览版保留

## 发布画像

- 业务域：全量规范审计修复（脱敏收口、备份恢复、终端、Docker/运维、文件/站点/应用、Web 契约、i18n、低风险加固）、批量终端体验修复、发布治理（OCR 行级评审辅助试行、审计产物披露边界、审计 skill 新鲜度检测）。
- 变更面：Go 后端逻辑修复（`internal/panel`、`internal/agent`、`internal/dockerx`、`internal/appmarket`、`internal/sites`、`internal/notification`、`internal/auth`、`internal/webenv` 等）；前端交互与文案（批量终端快捷命令/全屏、集群公开分享排序勾选、远程下载说明、多页 i18n）；治理文档、工作流与脚本（`PROJECT_RULES.md` 5.4/5.5、`scripts/ocr-delegate.mjs`、`scripts/report-dependency-freshness.mjs`、`dependency-policy.json`）。不新增数据迁移、脚本协议或宿主机安装路径。
- 受影响用户旅程：批量终端填入快捷命令与铺满网页；集群公开分享保存；远程下载提交前说明；覆盖上传、多配置站点更新、应用安装端口预检；容器控制台并发；初始化/改密/恢复密码规则；备份恢复大记录；AI 附件上传。
- 未变化契约：数据库 schema、端口、Compose、Agent 权限模型、`kejilion.sh` 脚本契约、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：中等；改动面广（116+ 文件）但每组均为审计发现的收紧或纠偏，均有单元/结构测试，源分支各自通过 L2，本候选通过完整 L3 与 mock 交互验收；后端强制"字母+数字"密码规则只影响新设/修改密码。

## 发布范围与未纳入内容

- 用户可见更新：见 `CHANGELOG.md` `[1.20.0-rc.2]`。
- 治理新增：`PROJECT_RULES.md` 5.4 审计产物披露边界、5.5 行级评审辅助（open-code-review 试行）、依赖新鲜度 `security-audit-skill` 采集。
- 精确提交清单（22 产品提交 + 5 合并提交 + 1 发布提交）：
  - `fix/global-audit-20260918`（合并 `0baffcf1`）：`33e002c8`、`0c886ac1`、`2b036654`、`8507f91e`、`fe72fb09`、`be2b5c58`、`f375f94b`、`80f3a835`、`ba302b32`。
  - `fix/terminal-batch-findings-20260918`（合并 `16499887`）：`8bb3702b`、`1c9fe4e9`、`45dcb6e2`、`1b4a80dd`、`061e03be`、`9d27e86d`。
  - `docs/ocr-line-review-assistant`（合并 `1098ba46`）：`fbda83d6`、`5740d0ab`、`04a99e52`、`350a58e1`、`16f497ee`。
  - `docs/audit-disclosure-boundary-20260918`（合并 `45fff830`，解决 `PROJECT_RULES.md` 5.4/5.5 相邻追加冲突）：`9df8c1ae`。
  - `chore/security-audit-skill-freshness-20260918`（合并 `3ba08e71`，解决 `check-governance-consistency.mjs` 与 `report-dependency-freshness.mjs` 纯追加冲突）：`e7119cb8`。
  - `7404fe29`（版本准备，冻结提交）。
- 明确未纳入的分支、文件或后续事项：`fix/hostbackup-restore-scope-20260918`、`fix/light-store-backup-discard-20260918`、`docs/security-boundary-audit-standard-20260918` 经 `git cherry` + `merge-tree` 判定内容已在 rc.1 落地；`fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 与 main 冲突且含未完成 WIP，保留不纳入；`ops/restore-v1100-channel`（09-09 应急通道恢复）已过时不纳入；审计存储性能（10k 条追加 p95≈149ms，越过 §5.2 复核触发）另起任务。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：候选对 `packaging/kejilion-app/kpanel.conf` 与 `Dockerfile` 相对 `v1.20.0-rc.1` 零差异；差异不涉及脚本协议、运行时动作、宿主机产物或安装/更新路径；L3 受管脚本契约与应用生命周期（`app_conf_lifecycle=pass`）通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 各修复随附单元/结构测试（脱敏、备份 record 大小、文件传输失败审计结构测试、UDP 端口状态、站点歧义、分享顺序等）；L3 全量 Go 测试与 web 162 文件 / 1436 用例通过；mock 交互验收 4/4。 | 真实多配置站点、真实 UDP 客户端套接字场景未在真机复现。 |
| 网络入侵与供应链安全 | 已验证 | 客户端可见敏感值统一经 `internal/redact`；AI 附件超限 413；诊断重定向限制；备份下载 SameFile 复核；L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 终端 spawn 锁/超时/尺寸漂移修复与测试；Docker 列表并发上限；容器控制台并发 4；L3 应用安装/更新/中断回滚/卸载故障注入通过；公开镜像冷启动 E2E。 | 未做长时间 soak；终端重连长期行为以单元测试覆盖。 |
| 性能与资源预算 | 已验证 | 并发扇出/分片上传/登录记录均为收紧上限，不引入热路径开销；Release 受限冷启动（256 MiB/1 CPU/128 PID）通过。 | 审计存储 p95 越过 §5.2 触发，已另起任务，本版未改变该路径。 |
| 用户体验与可访问性 | 已验证 | `interaction` 画像 mock 预览（模拟数据）在 `7404fe29` 上验证批量终端填入/全屏与 Esc 退出、分享排序勾选的请求体、远程下载说明；证据 `C:/GitHub/_preview-evidence/v1.20.0-rc.2-7404fe29/journeys-result.md`；i18n 由 `check-page-i18n` 门禁覆盖。 | 仅 1280x800 深色中文界面；窄视口、浅色和英文界面未逐页人工复核；控制台有 3 条 mock 资源 404，无 JS 异常。 |
| 数据、配置与迁移 | 已验证 | 无 schema/端口/Compose 变化；通知模块移除死代码但保留已存规则、告警状态与快照字段兼容；登录记录上限只裁剪最旧条目。 | 不适用。 |

## 自动门禁

- 定向测试及结果：本机 `node --test` 依赖新鲜度与 OCR 入口 28/28 通过、`check-governance-consistency` 通过、`check-version-consistency` 通过；源分支 `fix/global-audit-20260918` 在 arena-154 gate 镜像通过 L2。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_preflight=pass commit=7404fe29`；`app_conf_lifecycle=pass`；`release_gate_runner=pass`；`release_l3_gate=pass`；`release_l3_remote=pass`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.20.0-rc.2-7404fe29-l3-r1`；plan.env `53872c9a19eab96a8d8a836403b25240223385cecd447f5bd58b333c269270ba`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `79b9f57876ba9dfbaab58888ceaaa41ec5fc804e809abee93bb95ceddba2c67e`，manifest `276d45348e2b49665ea040e2239712683132a4cbb79959e0d32e613b15c9de8b`，远端日志 `4d0cbd67a8858d4343076ad11c83758b63f009c0e881be7d07aa46b083a725c9`；2026-09-19T07:31 启动至 07:46 结束（+0800），一次通过、exit 0；证据位于 `C:/GitHub/_release-evidence/v1.20.0-rc.2-7404fe29-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.2-7404fe29-l3-r1`。
- 候选 CI：[CI 35407005959](https://github.com/kejilion/KPanel/actions/runs/35407005959) 与 [freshness 35407005877](https://github.com/kejilion/KPanel/actions/runs/35407005877) 成功，均绑定 `7404fe29`。
- 主线 CI：[CI 35407459605](https://github.com/kejilion/KPanel/actions/runs/35407459605) 与 [freshness 35407459607](https://github.com/kejilion/KPanel/actions/runs/35407459607) 成功，均绑定 `7404fe29`；[tag freshness 35407850849](https://github.com/kejilion/KPanel/actions/runs/35407850849) 成功。
- Release workflow：[Release 35407850871](https://github.com/kejilion/KPanel/actions/runs/35407850871) 成功（2026-09-19T00:10:03Z 公开）。
- 安全扫描、镜像契约、SBOM/provenance：Release 工作流源码/镜像扫描、原生镜像运行时契约与受限冷启动均通过；amd64 与 arm64 均带 attestation manifest（`unknown/unknown` 条目）。
- OCR 行级评审（`PROJECT_RULES.md` 5.5）：`check-collaboration-state` 对组装候选提示 `ocr_line_review=stale`（非阻塞）；5.5 规定发布操作不适用，各源分支按各自任务完成评审（OCR 分支自身已跑首轮 candidate 档并修复发现），本发布不补跑。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main 两层 `Dependency freshness`（35407005877、35407459607）于 2026-09-19 成功，检测源完整；新增 `security-audit-skill` 与 `code-review-assistant` 两个检测源。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 运行时依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：`dependency-policy.json` 新增 `code-review-assistant`（open-code-review pin，仅本地评审辅助、不进镜像/CI）；继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.20.0-rc.2`；公共 OCI index 为 `sha256:e320e392efbfe1442bbb72ad3794092570bba090564eac8bb12c2355324dc867`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；生产和公共稳定入口继续保留 v1.19.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.x；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`7404fe29753e0f7ad44136b369eb07b298babb58` 与 `docker.io/kjlion/kejilion-panel@sha256:e320e392efbfe1442bbb72ad3794092570bba090564eac8bb12c2355324dc867`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.20.0-rc.2-7404fe29-l3-r1` passed/0；浏览器验收为本机 mock `local-feature-preview`（acceptance/interaction，`127.0.0.1:4174`，manifest `C:/GitHub/_preview-evidence/v1.20.0-rc.2-7404fe29/manifest.json`），未使用远程后台浏览器作业；公开 OCI E2E r2 exit 0（`image-e2e-r2.log` SHA-256 `5262555a238fd8463cd28d788d0c8a2bc29c053ca30815b482067febfd62c5fb`），固定入口 `packaging/tests/image-e2e.sh` SHA-256 `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`；r1 因只上传脚本缺源码树 exit 2（见流程异常）；证据位于 `arena-154:/root/kpanel-release-evidence/v1.20.0-rc.2/public-oci-e2e-r{1,2}`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次公开镜像冷启动 + L3 全量；无 soak，生命周期风险由 L3 故障注入与单元测试覆盖。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：1280x800、深色、中文、键盘 Esc 退出全屏；批量终端、公开分享、远程下载四条旅程通过；其余视口/主题/语言未逐页复核。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：真实多主机批量终端、真实多配置站点更新拒绝、原生 arm64、弱网、长期 soak 和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.20.0-rc.2](https://github.com/kejilion/KPanel/releases/tag/v1.20.0-rc.2) 于 2026-09-19T00:10:03Z 公开，为非 draft、prerelease、非 Latest；GitHub Latest 仍为 v1.19.0。
- Docker 版本与通道 OCI index：`1.20.0-rc.2` 与 `preview` 同为 `sha256:e320e392efbfe1442bbb72ad3794092570bba090564eac8bb12c2355324dc867`；稳定 `latest` 仍为 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`（v1.19.0），未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：`linux/amd64` `sha256:318fbb0f47fedaaa35b9ac88aa4e174e432d176bf9c2fac0bf8a70d4ed368305`、`linux/arm64` `sha256:031bd8a20a1ef4ea3f3cc420d1df433b59d4ad8e0ab69f29b719dbc40b11f58e`；两个 `unknown/unknown` 条目为 provenance/SBOM attestation，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、`kejilion-panel-deploy-1.20.0-rc.2.tar.gz`、LICENSE、`SHA256SUMS`、THIRD_PARTY_NOTICES.md）。
- 公开镜像 `image_e2e=pass`：`arena-154` 上传 `git archive 7404fe29` 精确候选源码、按不可变摘要拉取、`docker create`/`docker cp` 只读提取元数据（VERSION=`1.20.0-rc.2`、OCI revision `7404fe29`、脚本 SHA `28cf3934...` 一致）后运行固定 `image-e2e.sh`（显式 `KPANEL_EXPECTED_VERSION=1.20.0-rc.2`）；输出 `image_e2e=pass`；无本次残留容器/网络（`kpanel-e2e-batch-repro` 网络与一个 `/tmp/kpanel-image-e2e.*` 目录为 2026-09-17 其他任务遗留，未动）。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 相对 `v1.20.0-rc.1` 零差异，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` 内嵌脚本 SHA 保持 `28cf3934...` 基线。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用已验证实现，本轮未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核，`latest` = `sha256:351a236f...` 未变；GitHub Latest 仍为 v1.19.0；应用市场配置零差异）。
- 预览版保留候选分支：`release/v1.20.0-candidate` 保留并指向 `7404fe29`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未部署、未升级、未核对。
- 生产已执行写操作：本次为 0。

## 回滚

- 源码/tag：稳定回滚点 `v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`；上一预览 `v1.20.0-rc.1` / `2cea520f`。
- 镜像 digest：稳定 `latest` 保持 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`；上一预览 `1.20.0-rc.1` 为 `sha256:ec1293152df6fdbd75843c51bf74490ec01b042ede31d75477829e2832b41e1d`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择 v1.19.0 或 rc.1 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.19.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.19.0 / `sha256:351a236f...`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.19.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-18T15:26:46+08:00
- 候选冻结时间：2026-09-19T07:30:48+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-evidence/oci-e2e/script-only-upload-missing-source-tree",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开镜像 E2E r1 只上传 image-e2e.sh 与 VERSION，脚本对比镜像内桌面图标需读取候选源码 web/public 资源，cmp 找不到文件而 exit 2；产品未被测到，属无效证据。",
    "recoveryEvidence": "r2 以 git archive 7404fe29 完整候选源码上传并解包后执行同一固定脚本（SHA-256 1378218f...），输出 image_e2e=pass、e2e_exit=0；r1 日志 616cac4a... 原样保留。",
    "permanentAction": "image-e2e.sh 依赖完整候选源码树，远端执行必须先落地精确候选源码（git archive 或 L3 工作目录检出）；与'公开 OCI 元数据提取与 E2E 执行入口仓库脚本化'遗留诉求合并处理。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：mock 预览进程已按固定入口停止；保留候选工作树、远端候选分支、L3 r1 证据与公开 OCI E2E 证据，供同一 `1.20.0` 序列继续追加 RC。
- 未验证风险：真实多主机批量终端、真实多配置站点与 UDP 客户端场景、窄视口/浅色/英文界面逐页复核、原生 arm64、弱网、长期 soak 和生产数据恢复。
- 已实现待实机准入：后端密码规则强制与登录记录上限需要在 RC 反馈期观察用户反馈。
- 不阻断本版的理由：最终 SHA 已通过完整 L3（一次通过）、候选/main/tag 三层门禁、双架构公开 OCI 与隔离冷启动、mock 交互验收；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：审计存储性能复核（§5.2）另起任务；公开 OCI 元数据提取与 E2E 执行入口仓库脚本化诉求仍待处理；多分支组装时 `PROJECT_RULES.md` 与治理脚本清单的相邻追加冲突反复出现，可考虑清单排序约定降低冲突。
