# KPanel v0.99.1 发布验收记录

日期：2026-08-29

发布级别：L3

候选提交 / 标签：`fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837` / `v0.99.1`

上一稳定版本 / 回滚点：`v0.99.0` / `sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`（commit `641a3655c928146fa8daa510c5d3d01747f5d15f`）

## 发布画像

- 业务域：文件管理器预览与代码编辑器主题。
- 变更面：展示与只读交互；不涉及 API、数据、宿主机权限或部署脚本。
- 受影响用户旅程：用户切换 KPanel 全局或自定义主题后，打开文件预览、代码编辑器和相关只读文件内容时，界面跟随当前主题的语义颜色，而不是继续使用固定终端深色调色板。
- 未变化契约：API、数据 schema、端口、Compose、Agent 权限、`kejilion.sh`、应用市场和系统中心均不变。
- 风险等级及理由：低到中；变更限定在文件预览主题 token、消费者和回归测试，未改变布局尺寸、数据流或宿主机写入链路。

## 发布范围与未纳入内容

- 用户可见更新：文件预览和 `CodeEditor` 改用文件预览语义 token，随活动主题正确显示；终端和日志工作区继续保留原有终端调色板。
- 精确提交清单：候选基线为 `origin/main` `c2d11689b29d230abc6e75075771b77733917598`；源修复 `2ec69fe7f2df06b5ef6b37c1382f63ec38b24b59`（父 `71b53cf4f396d36ebffd599c89c959476ec5e7f5`）在候选中形成 cherry-pick `c459ad0c37bd9b23499904e6700fa6a97c7ecd8b`；版本准备提交为 `fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`（父 `c459ad0c37bd9b23499904e6700fa6a97c7ecd8b`）。候选相对基线仅含 `CHANGELOG.md`、`VERSION`、`internal/version/version.go`、`web/package.json`、`web/package-lock.json`、`web/src/components/files/CodeEditor.vue`、`web/src/components/workspaceTheme.test.ts`、`web/src/styles/main.css`、`web/src/styles/themes.css`、`web/src/theme/semanticConsumers.test.ts`、`web/src/views/FilesView.test.ts`、`web/src/views/FilesView.vue`。
- 明确未纳入的分支、文件或后续事项：候选差异不含 System Center 页面、路由、API 或文档；`kejilion/apps` 未产生提交；`kejilion.sh` 未改动；`108`/`prod-108` 未连接。真实浏览器主题矩阵未执行，保留为后续专项验收事项。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 文件主题定向测试 3 个文件/90 项通过，Web 全量 122 个文件/1046 项通过；typecheck、build 和生产 postdeploy 通过 | 未在真实浏览器中逐一切换所有自定义主题并观察文件预览 |
| 网络入侵与供应链安全 | 已验证 | 候选/main CI、固定 L3、Release workflow、govulncheck、npm audit、Trivy、OCI digest 与受管脚本契约均通过 | 本版没有新增网络入口；未另行执行公网渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 生命周期与失败清理、候选/main CI、Release、生产备份和 postdeploy 均通过；无 schema 迁移 | 未做长期 soak；主题异常回退主要由自动化回归覆盖 |
| 性能与资源预算 | 已实现未实机验证 | 变更为 CSS token/消费者与测试，未改数据流或布局尺寸；生产容器资源快照约 71.65 MiB / 256 MiB | 未做浏览器性能采样或长期资源曲线对比 |
| 用户体验与可访问性 | 已实现未实机验证 | `FilesView`、`CodeEditor` 和语义消费者回归通过，build 通过 | 未执行真实浏览器的键盘、焦点、缩放、最小字号和多主题矩阵 |
| 数据、配置与迁移 | 已验证 | 本版无数据/schema/配置迁移；生产 `protected.diff` 为 0，SQLite quick check 通过，备份摘要通过 | 不适用额外迁移验收 |

## 自动门禁

- 定向测试及结果：`npm run test -- src/components/workspaceTheme.test.ts src/theme/semanticConsumers.test.ts src/views/FilesView.test.ts` 通过，3 个文件/90 项；`npm run test --prefix web` 通过，122 个文件/1046 项；`npm run typecheck --prefix web` 和 `npm run build --prefix web` 通过；`npm ci` 安装 282 个包并报告 0 vulnerabilities。Windows 直接执行 `make` 不可用，`node scripts/run-repo-bash.mjs scripts/verify-change.sh` 的 canonical wrapper 通过；本地 L2 因环境缺少 `go`、`gofmt`、`make` fail-closed，未将其误记为产品失败。
- `make verify-release` 环境和结果：固定 Runner `kpanel-release-gate:go1.26.6-node24`，不可变 Runner ID=`sha256:b593c0ffe32e80a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.99.1-fb3a0a1-l3-r1`，`release_l3_gate=pass`、`release_l3_remote=pass`、`app_conf_lifecycle=pass`、`release_gate_runner=pass`；证据目录为 `C:\GitHub\_release-artifacts\v0.99.1-fb3a0a1-l3-r1`，bundle SHA-256=`0742e7360ab8b28153e583aedae44e877ebc1fcab4229e7f9a46483307ae50f3`，manifest SHA-256=`cf494ccb359d42c07634620a897c9b17d2fd54e68d91231509da0a7178bfebb4`，plan SHA-256=`1ea279344ef3112c7945df0bfcc2314bf8cc42d0a8ffd0f051c9d5d0d26eba1a`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：CI run `33235794867`、Dependency freshness run `33235794849`，均绑定 `fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`，completed/success。
- 主线 CI：CI run `33236467662`、Dependency freshness run `33236467708`，均绑定候选 SHA，completed/success；推进前 `origin/main` 为 `c2d11689b29d230abc6e75075771b77733917598`，未漂移后 fast-forward 到候选 SHA。
- Release workflow：run `33236657858`，`v0.99.1^{}` 解引用到候选 SHA，completed/success；GitHub Release 已公开、非 draft、非 prerelease；候选远端分支按 workflow 完成清理。
- 安全扫描、镜像契约、SBOM/provenance：Release 的 Go/Node 双架构构建、native image validation、OCI 多架构推送、`latest` promotion、SBOM/provenance、Trivy、镜像运行契约和受管 `kejilion.sh` revision/SHA 校验均通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本次未单独生成新的完整依赖报告；Dependency freshness runs `33235794849`、`33236467708` 成功，L3 的 govulncheck、npm audit 和 Trivy 通过；不据此声明所有上游依赖均为最新。
- 最近每日安全通告审计、EOL 复核状态及证据：本版沿用固定 L3 的 govulncheck、npm audit、Trivy 和 Release 供应链校验；独立 EOL 复核未单独重做，未作额外“全部当前”结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增运行时依赖或基础镜像；未记录新的完整检测行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：固定 Go `1.26.6`、Node `24`、既有构建镜像和扫描器；受管 `kejilion.sh` revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Panel/Web 版本 `0.99.1`；脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`，clean Git blob SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`；公开 OCI index=`sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：没有新增运行时依赖候选；浏览器主题/可访问性专项留待下一次具备真实浏览器证据的复核，退出条件为补齐主题、视口、缩放、键盘/焦点和失败态记录。
- 升级后的兼容、安全、构建、性能资源和回滚结论：自动门禁、公开 Release/OCI、生产更新和 postdeploy 均通过；无 Panel schema 迁移；回滚材料为 `v0.99.0` OCI、Compose、`.env`、Agent 文件和本次停写备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：固定 L3 使用 Go `1.26.6` / Node `24` runner；正式生产目标为 `arena-154`，容器实际运行 amd64 镜像。
- 环境策略 ID 与允许用途：`environment-policy.json` 的 `arena-154`；candidate-validation、production-safety-check 和 production-deploy 通过；`prod-108` 为 disabled，`108`/`prod-108` 未连接。
- 使用的精确候选或公开产物：候选 `fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`；公开 `kjlion/kejilion-panel:0.99.1` 与 `latest` 均为 `sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`；本轮未另行记录平台子 manifest digest，Release workflow 的多架构推送、扫描和 OCI 校验通过。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v0.99.1-fb3a0a1-l3-r1` passed/0、无超时，证据目录为 `C:\GitHub\_release-artifacts\v0.99.1-fb3a0a1-l3-r1`；生产 `v0.99.1-fb3a0a1-prod-20260829` 的 preflight、backup、postdeploy 均 passed/0，证据目录分别为 `C:\GitHub\_release-artifacts\v0.99.1-fb3a0a1-prod-20260829-preflight`、`C:\GitHub\_release-artifacts\v0.99.1-fb3a0a1-prod-20260829-backup`、`C:\GitHub\_release-artifacts\v0.99.1-fb3a0a1-prod-20260829-postdeploy`；固定生产脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。
- 测试窗口/循环数及风险依据：候选/main CI、Release、固定 L3 和生产 preflight/backup/postdeploy 各一次通过；无独立 soak，因为 `arena-154` 是唯一批准的真实生产目标，不能改装成测试机。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动化覆盖文件预览主题消费者、文件视图和代码编辑器回归；真实浏览器的主题切换、窄视口、缩放、键盘/焦点、语言和失败态矩阵未执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 已验证应用生命周期、失败清理和回滚夹具；生产 backup 按固定流程受控停止/恢复服务，标准应用入口更新并重建 Panel；postdeploy 通过。backup 阶段出现一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时输出，但固定 gate 通过、六项备份摘要均为 `OK`、服务恢复且证据未失效。
- 未执行场景及原因：未执行真实浏览器 UI 验收和长期 soak；原因是本版变更为只读主题语义修复，自动回归已覆盖消费者契约，且当前没有额外获授权的独立浏览器/测试主机；不以生产健康证据冒充浏览器验收。

## 发布产物与公开仓库复核

- GitHub Release：[v0.99.1](https://github.com/kejilion/KPanel/releases/tag/v0.99.1)，Release workflow=`33236657858`，公开、非 draft、非 prerelease；annotated tag object=`d257faa6c52520ba54565af17631e38e3b8612c5`，`v0.99.1^{}`=`fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.1` 与 `latest` 均返回 status 200、OCI index，digest=`sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`。
- `linux/amd64`、`linux/arm64` digest：本轮未单独记录平台子 manifest digest；Release workflow 的多架构构建、推送、native image validation、扫描和 attestation 通过。
- 附件及 `SHA256SUMS`：GitHub Release 附件由 Release workflow 生成并完成 `SHA256SUMS`、native image 和运行契约校验；本版没有改动 Agent 或节点二进制内容。
- 公开镜像 `image_e2e=pass`：Release workflow 的 native image validation、OCI digest 校验和生产标准入口实际回拉均通过；生产容器 image inspect 的 RepoDigest 精确匹配目标 digest。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场无需提交；生产容器受管脚本 revision/SHA 为 `d58079304a92936bf8e3d90467eea484c5b63d6f` / `68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`，clean Git blob、语法、同步和脚本契约门禁通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求复核后走上线流程；仅执行 KPanel `v0.99.1` 标准应用更新、生产证据和备份，目标仅 `arena-154`。
- 验证/灰度环境：固定 `arena-154` L3 runner、公开 OCI 校验和生产安全证据入口，均来自 `environment-policy.json`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight `0.99.0`、health `ok`，Agent `loaded/active/running/enabled`；backup gate 通过，备份目录为 `/root/kpanel-backups/pre-v0.99.0-20260829T054826Z`，六项 `SHA256SUMS` 均记录并校验通过，受保护配置差异为空。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy health `version=0.99.1`、`status=ok`；容器标签 `org.opencontainers.image.revision=fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`、`org.opencontainers.image.version=0.99.1`，镜像 RepoDigest 精确匹配 `sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`；Panel `running/healthy`、restart=0、OOM=false；Agent `active/running/enabled` 且 `NeedDaemonReload=no`；protected diff 0 字节，SQLite quick check 为 `panel/ai.db ok`，最近 10 分钟 Panel/Agent panic/fatal/OOM signature scan=`NONE`；公网入口为 `https://kpanel.154.36.153.9.sslip.io`。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取 `latest` 并重建 Panel，实际 OCI digest 为目标 digest；未执行业务数据写入、schema 迁移或端口变更。
- 仅在隔离真机执行、未在生产执行的场景：L3 负例、脚本失败清理/回滚模拟和自动化主题/布局回归；真实浏览器主题矩阵未执行。

## 回滚

- 源码/tag：`v0.99.0` / commit `641a3655c928146fa8daa510c5d3d01747f5d15f`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.0-20260829T054826Z`，包含旧应用目录、旧镜像、Agent unit、应用配置和校验摘要。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按应用市场原生失败回滚流程成套恢复 `v0.99.0` OCI、Compose、`.env`、数据目录和 Agent，再执行固定 preflight/postdeploy。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release 当前为 `v0.99.1`；Docker `latest` 与 `0.99.1` 同为 `sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`；标准更新入口本次实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.99.1` 为默认更新版本；生产 postdeploy 健康、日志、数据和配置保护均通过，没有产品退化证据，因此不恢复上一稳定版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-29T12:58:51+08:00
- 候选冻结时间：2026-08-29T13:15:48+08:00
- 生产完成时间：2026-08-29T13:51:18+08:00
- 提交到生产用时：0.87 小时
- 是否回滚、紧急热修复或重复发布：否（无产品变更失败、生产回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-integration/cherry-pick/theme-test-conflict",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选基线 cherry-pick 源修复时，web/src/components/workspaceTheme.test.ts 出现集成冲突；按上线规范停止并报告，未推送 main、未创建 tag、未执行生产写入。",
    "recoveryEvidence": "保留 main 既有 terminalThemeSource 与修复引入的 semanticThemeSource，完成冲突后的定向测试、Web 全量测试、typecheck、build、候选 CI、固定 L3、main CI 和 Release workflow；最终候选 SHA 为 fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837。",
    "permanentAction": "候选冻结前继续执行源修复与当前 main 的逐文件冲突审查，并将冲突解决后的测试与完整候选门禁作为唯一放行条件；退出条件为下一次同类 cherry-pick 不再绕过冲突停止点，或保留可审计的人工合并证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

本次另有一次备份阶段 `curl 56` 瞬时连接重置输出，但固定备份 gate 未失败、证据未失效且六项摘要均为 `OK`，因此不计入流程异常指标。

## 遗留风险与后续准入

- 未验证风险：真实浏览器中不同主题、窄视口、100%/125%/200% 缩放、键盘/焦点、语言和失败态的文件预览验收，以及长期性能采样尚未执行。
- 已实现待实机准入：文件预览和代码编辑器的活动主题语义消费已实现并通过自动回归；需在具备真实浏览器证据的专项验收中补齐 UI 矩阵。
- 不阻断本版的理由：本版是低风险只读展示修复，无 API、数据、宿主机权限、端口或部署脚本变化；候选/main CI、固定 L3、公开 Release/OCI、生产备份、标准更新和 postdeploy 均通过，未以生产健康证据冒充浏览器验收。
- 后续应进入的自动门禁或专项工作流：在下一次 UI 专项验收中补充真实浏览器的主题、窄视口、缩放、键盘/焦点、语言和失败态证据，并评估将活动主题下的文件预览截图/交互纳入固定回归。
