# KPanel v0.99.3 发布验收记录

日期：2026-08-29

发布级别：L3

候选提交 / 标签：`0cf8a57259297e844a2f2077eeb9cdc102c79e87` / `v0.99.3`

上一稳定版本 / 回滚点：`v0.99.2` / `sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`（commit `8efb92d2529711500d42832f6a39591a4188e62c`）

## 发布画像

- 业务域：KPanel 桌面模式下的网站页布局展示。
- 变更面：展示与前端布局；不涉及 API、数据、宿主机写入、Agent 权限或部署协议。
- 受影响用户旅程：用户在桌面模式打开“网站”页时，页面内容按实际内容高度布局，体检/表格等页面区域不再被异常撑高或产生多余空白。
- 未变化契约：API、数据 schema、端口、Compose、Agent 权限、`kejilion.sh` 和应用市场功能契约均不变。
- 风险等级及理由：低；变更只增加页面作用域 class、桌面布局对齐规则和回归测试，不改变业务逻辑、接口、数据流或宿主机操作。

## 发布范围与未纳入内容

- 用户可见更新：`SitesView` 在桌面窗口中使用 `sites-page` 作用域，并令桌面窗口内容采用 `align-content: start`，避免网站页内容区域按异常高度拉伸。
- 精确提交清单：候选基线为最新 `origin/main` `cd6d55428991907a51516564c70321beab58b093`；源修复 `401ab239ec920e3aa0519194230bd09e59ebd4b4`（父 `a33fbec2a4a54d5ae9589ffd69fadaa5e777f860`）在候选中形成 cherry-pick `c4132ac218c23cf150af35fabc81597987a70fef`；版本准备提交为 `0cf8a57259297e844a2f2077eeb9cdc102c79e87`。相对候选基线的产品差异仅为 `CHANGELOG.md`、`VERSION`、`internal/version/version.go`、`web/package.json`、`web/package-lock.json`、`web/src/styles/desktop.css`、`web/src/views/SitesView.layout.test.ts`、`web/src/views/SitesView.vue` 八个文件。
- 明确未纳入的分支、文件或后续事项：候选差异不含 System Center 页面、路由、API 或文档；本版明确排除系统中心上线。`kejilion/apps` 未产生提交，`kejilion.sh` 未改动，`108`/`prod-108` 未连接。正式生产浏览器的完整 UI 矩阵未在本轮重新执行。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向布局回归测试、L3 前端全量测试（123 个文件/1047 项）、typecheck、build、候选/main CI 和生产 postdeploy 均通过；源任务对同一三文件修复在真实浏览器验证了桌面网站页内容高度变化 | 本版无 API、节点协议或双端数据契约变化 |
| 网络入侵与供应链安全 | 已验证 | 候选/main CI、固定 L3、Release workflow、Go 漏洞扫描、npm audit、Trivy、OCI digest、SBOM/provenance 与受管脚本契约均通过 | 本版没有新增网络入口；未另行执行公网渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 生命周期与失败清理、候选/main CI、Release、生产 preflight/backup、标准应用更新入口和 postdeploy 均通过；无 schema 迁移 | 未做长期 soak；布局兼容性仍保留浏览器组合差异风险 |
| 性能与资源预算 | 已实现未实机验证 | 变更为页面布局规则和回归测试，未改数据流、轮询或资源加载；生产容器健康、restart=0、OOM=false | 未做浏览器长期性能曲线或不同设备资源采样 |
| 用户体验与可访问性 | 已实现未实机验证 | 源任务在 `1896x823` 浏览器窗口对同一修复观察到基线 `align-content: normal`、工具栏约 `89px`/表格约 `197px`，修复后为 `align-content: start`、工具栏约 `64px`/表格约 `172px`；无相关控制台错误；布局回归测试和构建通过 | 本次未重新执行正式发布 SHA 的 390/768/1280 视口、100%/125%/200% 缩放、明暗主题、键盘/焦点/触控、最小计算字号、长文本/多语言及加载/空/失败/部分状态矩阵 |
| 数据、配置与迁移 | 已验证 | 本版无数据/schema/配置迁移；生产 `protected.diff` 为 0 字节，SQLite quick check 为 `ai.db empty`、`panel/ai.db ok`，备份和恢复快照通过 | 不适用额外迁移验收 |

## 自动门禁

- 定向测试及结果：`web/src/views/SitesView.layout.test.ts` 定向回归通过；固定 L3 前端全量测试通过（123 个文件/1047 项）、typecheck、build 通过，i18n 检查为 2582 localized phrases / 21 lazy catalogs。Windows 本地 `node scripts/run-repo-bash.mjs --env VERIFY_LEVEL=release -- scripts/verify-change.sh` 因环境缺少 `docker`、`go`、`gofmt`、`make` 在最终 preflight fail-closed，这是执行环境限制，不是候选产品失败；固定 Linux L3 完成完整放行。
- `make verify-release` 环境和结果：固定 Runner `kpanel-release-gate:go1.26.6-node24`，不可变 Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.99.3-0cf8a57-l3-r1`，`release_l3_gate=pass`、`release_l3_remote=pass`、`release_gate_preflight=pass`；证据目录为 `C:\GitHub\_release-artifacts\v0.99.3-0cf8a57-l3-r1`，bundle SHA-256=`5e1d6cc7c2f10e63c3073d2587e9a553bbf673ecd05831444eac1ead91b69781`，manifest SHA-256=`1876ed7134b752abd60b1c8bfbb1b0fee379fa215712b063d973054734d0c9b2`，plan SHA-256=`3a2834161d91758834588f0de18062b5b9cc94b1e8c050139d8f0620f80cf00c`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：CI run `33247927067`、Dependency freshness run `33247927022`，均绑定 `release/v0.99.3-candidate` 和精确 SHA `0cf8a57259297e844a2f2077eeb9cdc102c79e87`，completed/success。
- 主线 CI：主线推进前经 SHA guard 确认为 `cd6d55428991907a51516564c70321beab58b093`，随后 fast-forward 到产品候选 SHA；CI run `33248172017`、Dependency freshness run `33248172050` 均绑定精确产品 SHA，completed/success。验收记录提交后主线会再前进一条文档提交，`v0.99.3` 标签仍只指向产品 SHA。
- Release workflow：run `33248349618`，`v0.99.3^{}` 精确解引用到 `0cf8a57259297e844a2f2077eeb9cdc102c79e87`，completed/success；构建、校验、扫描、发布、latest promotion 和候选分支清理步骤均 success。GitHub Release 已公开、非 draft、非 prerelease。
- 安全扫描、镜像契约、SBOM/provenance：Release workflow 的 Go/Node 双架构构建、native image validation、OCI 多架构推送、latest promotion、SBOM/provenance、Trivy、镜像运行契约和受管 `kejilion.sh` revision/SHA 校验均通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本版未新增运行时依赖、基础镜像或 Action；Dependency freshness runs `33247927022`、`33248172050` 成功，固定 L3 的 govulncheck、npm audit 和 Trivy 通过；不据此声明所有上游依赖均为最新。
- 最近每日安全通告审计、EOL 复核状态及证据：本版沿用固定 L3 的 govulncheck、npm audit、Trivy 和 Release 供应链校验；独立 EOL 复核未单独重做，未作额外“全部当前”结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增运行时依赖或基础镜像，未记录新的完整检测行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：固定 Go `1.26.6`、Node `24`、既有构建镜像和扫描器；受管 `kejilion.sh` revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Panel/Web 版本 `0.99.3`；生产脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`，clean Git blob SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`；公开 OCI index=`sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无新增运行时依赖候选；浏览器完整 UI 矩阵留待后续专项验收，退出条件为补齐桌面/移动窄视口、缩放、主题、键盘/焦点、语言和失败态证据。
- 升级后的兼容、安全、构建、性能资源和回滚结论：自动门禁、公开 Release/OCI、生产更新和 postdeploy 均通过；无 Panel schema 迁移；回滚材料为 `v0.99.2` OCI、生产前备份和标准应用更新入口。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：固定 L3 使用 Go `1.26.6` / Node `24` runner；正式生产目标为 `arena-154`，容器实际运行 `linux/amd64` 镜像。
- 环境策略 ID 与允许用途：`environment-policy.json` 中 `arena-154` 的 candidate-validation、production-safety-check 和 production-deploy 均通过；`prod-108` 为 disabled，`108`/`prod-108` 未连接。
- 使用的精确候选或公开产物：候选 `0cf8a57259297e844a2f2077eeb9cdc102c79e87`；公开 `kjlion/kejilion-panel:0.99.3` 与 `latest` 均为 OCI index `sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v0.99.3-0cf8a57-l3-r1` passed/0、无超时，证据目录为 `C:\GitHub\_release-artifacts\v0.99.3-0cf8a57-l3-r1`；生产 `v0.99.3-production` 的 preflight、backup、postdeploy 均 passed/0，证据目录分别为 `C:\GitHub\_release-artifacts\v0.99.3-production-preflight`、`C:\GitHub\_release-artifacts\v0.99.3-production-backup`、`C:\GitHub\_release-artifacts\v0.99.3-production-postdeploy`；固定生产远端脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。
- 测试窗口/循环数及风险依据：候选/main CI、Release、固定 L3 和生产 preflight/backup/postdeploy 各一次通过；无独立 soak，因为 `arena-154` 是唯一批准的真实生产目标，不能改装成测试机。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：源任务同一修复已在 `1896x823` 浏览器窗口完成布局几何和控制台检查；本轮未重新执行正式发布 SHA 的 390/768/1280 视口、100%/125%/200% 缩放、明暗主题、最小计算字号、键盘/焦点/触控、长文本/多语言及加载/空/失败/部分状态矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 已验证应用生命周期、失败清理和回滚夹具；生产 backup 按固定流程受控停止/恢复服务，标准应用市场入口更新并重建 Panel；postdeploy 通过。backup 阶段曾出现一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时输出，但固定 gate 未失败、服务已恢复、校验摘要和证据均通过，未发生产品退化或回滚。
- 未执行场景及原因：未执行正式生产浏览器 UI 全矩阵和长期 soak；原因是本版为低风险前端布局变更，源任务已有针对性几何证据，自动回归、构建、固定 L3、公开产物和生产健康证据覆盖主要回归面；不以生产健康证据冒充浏览器矩阵验收。

## 发布产物与公开仓库复核

- GitHub Release：[v0.99.3](https://github.com/kejilion/KPanel/releases/tag/v0.99.3)，Release workflow=`33248349618`，公开、非 draft、非 prerelease；annotated tag 的 `v0.99.3^{}`=`0cf8a57259297e844a2f2077eeb9cdc102c79e87`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.3` 与 `latest` 均为 OCI index，digest=`sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:426d5ffa574b2abc553e0253c371c7fcb47d7ba4e06c3d559a1069b589c91c54`；arm64=`sha256:c4d90b21b0b2ed4e58ab1d2bd7dff91bfd528641766fa2d6f1673d59779626d9`；OCI index 另含 provenance/attestation 子清单，Release workflow 校验通过。
- 附件及 `SHA256SUMS`：GitHub Release 附件包含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-0.99.3.tar.gz`、`SHA256SUMS`、LICENSE 和 `THIRD_PARTY_NOTICES.md`；Release workflow 完成附件摘要、native image 和运行契约校验。
- 公开镜像 `image_e2e=pass`：在 `arena-154` 使用公开 `docker.io/kjlion/kejilion-panel:0.99.3` 执行隔离 image E2E，`KPANEL_EXPECTED_VERSION=0.99.3`，结果 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场 `packaging/kejilion-app/kpanel.conf` 与 `kejilion/apps` 的 `origin/main:kpanel.conf` Git blob 一致，未产生 apps 提交；生产容器受管脚本 revision/SHA 为 `d58079304a92936bf8e3d90467eea484c5b63d6f` / `68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`，脚本契约门禁通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求复核后走上线流程；仅执行 KPanel `v0.99.3` 标准应用更新、生产证据和备份，目标仅 `arena-154`。
- 验证/灰度环境：固定 `arena-154` L3 runner、公开 OCI 校验和生产安全证据入口，均来自 `environment-policy.json`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight `0.99.2`、health `ok`，Agent `loaded/active/running/enabled`；backup gate 通过，备份目录为 `/root/kpanel-backups/pre-v0.99.2-20260829T105139Z`，服务恢复快照通过，`protected.diff` 为 0 字节。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，实际返回 exit 0、`KPanel 更新完成 / Update Complete`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：固定 postdeploy `status=passed`，health `status=ok`、`version=0.99.3`；容器标签 `org.opencontainers.image.revision=0cf8a57259297e844a2f2077eeb9cdc102c79e87`、`org.opencontainers.image.version=0.99.3`，镜像 RepoDigest 精确匹配 `sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`；Panel `running/healthy`、restart=0、OOM=false；Agent `loaded/active/running/enabled` 且 `NeedDaemonReload=no`；`protected.diff` 0 字节，SQLite quick check 为 `ai.db empty`、`panel/ai.db ok`，最近 10 分钟 Panel/Agent fatal、panic、OOM、error signature scan 无匹配。本次未单独执行生产浏览器公网入口 UI 矩阵。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取目标 `latest` digest 并重建 Panel；未执行业务数据写入、schema 迁移或端口变更。
- 仅在隔离真机执行、未在生产执行的场景：L3 负例、脚本失败清理/回滚模拟和自动化布局回归；正式生产浏览器 UI 矩阵和长期 soak 未执行。

## 回滚

- 源码/tag：`v0.99.2` / commit `8efb92d2529711500d42832f6a39591a4188e62c`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.2-20260829T105139Z`，包含旧应用目录、旧镜像、Agent unit、应用配置和校验摘要。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按应用市场原生失败回滚流程成套恢复 `v0.99.2` OCI、Compose、`.env`、数据目录和 Agent，再执行固定 preflight/postdeploy。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release 当前为 `v0.99.3`；Docker `latest` 与 `0.99.3` 同为 `sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`；标准更新入口本次实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.99.3` 为默认更新版本；生产 postdeploy 健康、日志、数据和配置保护均通过，没有产品退化证据，因此不恢复上一稳定版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-29T18:14:40+08:00
- 候选冻结时间：2026-08-29T18:17:48+08:00
- 生产完成时间：2026-08-29T18:54:34+08:00
- 提交到生产用时：0.67 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

backup 阶段曾出现一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时输出，但固定备份 gate 未失败、服务已恢复、校验摘要和证据均通过，因此按规范不计入使必需步骤失效或重试的流程异常指标。

## 遗留风险与后续准入

- 未验证风险：正式生产浏览器中网站页的 390/768/1280 视口、100%/125%/200% 缩放、明暗主题、最小字号、键盘/焦点/触控、长文本/多语言、加载/空/失败/部分状态矩阵，以及长期 soak 尚未执行。
- 已实现待实机准入：桌面模式网站页高度修复已通过针对性浏览器几何证据、布局回归测试、全量前端门禁、固定 L3、公开产物和生产 postdeploy；完整浏览器矩阵仍待后续专项验收。
- 不阻断本版的理由：本版是低风险前端布局变更，无 API、数据、宿主机权限、端口、Agent 或应用市场契约变化；候选/main CI、固定 L3、公开 Release/OCI、生产备份、标准更新和 postdeploy 均通过，且没有系统中心变更混入。
- 后续应进入的自动门禁或专项工作流：在下一次 UI 专项验收中补充窄视口、桌面缩放、主题、键盘/焦点、触控、语言和失败态证据，并评估将网站页桌面容器几何约束纳入固定浏览器回归。
