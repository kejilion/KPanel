# KPanel v1.0.0 发布验收记录

日期：2026-09-02

发布级别：L3

候选提交 / 标签：`45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068` / `v1.0.0`

上一稳定版本 / 回滚点：`v0.100.0` / `89a384c7d65c42b14222dcace8843ff23602dc11`，生产回滚镜像为 `sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`

> 本记录对应本次真实 `v1.0.0` 发布和 `arena-154` 部署。所有门禁、Release、OCI、备份和 postdeploy 证据均绑定本次候选 SHA；未复用 `v0.100.0` 的 L3、镜像或生产证据，也未连接 `108`/`prod-108`。

## 发布画像

- 业务域：集群累计流量通知与时区处理、Web 统一视觉契约、对话框全屏视口、三级页面多语言治理，以及发布验收和治理流程。
- 变更面：展示 / 只读 / 部分后台通知配置写入 / 构建与发布治理；不新增数据库迁移，不改变 Panel/Agent 协议版本与端口契约。
- 受影响用户旅程：在集群页面配置和接收累计流量通知；在窄视口桌面模式访问体检窗口；在页面、弹窗、文件分享、集群分享、应用终端和 Apps/Cluster 页面使用统一视觉和英文资源；通过标准应用市场更新 KPanel。
- 未变化契约：API 协议版本 `v1alpha1`、数据库、端口、Compose、Agent 权限边界、受管 `kejilion.sh` 应用入口和应用市场安装契约均未变化；System Center 仅随统一视觉契约调整既有对话框样式，不纳入新的系统维护 API、权限或宿主机动作。
- 风险等级及理由：中风险。包含集群通知业务逻辑、桌面窗口布局、较大范围视觉 token/CSS、i18n 和发布治理门禁；无数据库/端口/Agent 协议迁移，最终候选 CI、主线 CI、L3、Release、OCI、生产备份和 postdeploy 均通过。

## 发布范围与未纳入内容

- 用户可见更新：
  1. 集群通知统一累计接收/传送规则，按主机时区生成通知，并为 scratch 镜像嵌入时区数据。
  2. Web 端统一语义颜色、视觉节奏和对话框视口约束；既有 System Center 对话框只做视觉/布局收敛。
  3. 手机窄视口桌面模式下体检窗口恢复可滚动；全屏对话框在横屏视口内保持可用。
  4. 补齐三级页面、弹窗和门户操作的英文运行时多语言资源及回归检查。
  5. 补充发布验收记录索引、重复流程指纹检测和连续稳定 tag 覆盖治理。
- 精确提交清单（均已进入最终产品 SHA 的主线历史）：
  - `13fa027a87edac1a628f03c695c9c0a23d88b809`：集群累计流量通知。
  - `67c7919db0931d8714c593da1d94e058e122ff41`：通知使用主机时区。
  - `693928d97d34184c2ea27721c09e597640977f5f`：scratch 镜像嵌入时区数据。
  - `89aa6f512d54d8674615516318fdb1965e76d990`：拆分累计流量通知。
  - `8ce715ee181d4052bb2196a0e0d2617de82f6453`：当前主线视觉系统。
  - `10d4a59287d110f6088008198361792319af1a3b`：横屏全屏对话框视口修复。
  - `6332ed4f0e166a408a8ff9faf07b01732abbfb63`：视觉系统契约门禁。
  - `2526e0ea3ebdfbf6a2acb4574566f582aaab0939`：补充 `v0.100.0` 验收记录与覆盖门禁。
  - `27f2e574d29f4b04e7f14bff8ec54b657608891b`：检测重复发布流程指纹。
  - `62991fc58b99ff618716c5567e55c910fa79bf0a`：文档导航索引。
  - `f22ea46688d728d680ac85fdf5b0f342b9d56cce`：满足集群通知视觉字号下限。
  - `45d89afa3461132c3f037cc58d98801c6de1d193`：准备 `v1.0.0` 版本元数据。
  - `0eeefac6bc8df43ae3183549208e471609e45bc6`：移除无用集群分享 import。
  - `cdd6ad2efaf28d5c3a92c1239c0e3d78891866b6`：迁移旧版集群流量规则。
  - `37ce91d72cdc0407e75245499c66f5a11db1daba`：确保版本候选触发 Dependency freshness。
  - `9389a29fc7974ea3edfe5cb9d5a7ae4add53fbd2`：刷新当前产品事实基线至 `v0.100.0`。
  - `414423c863df83b7894d423d7f4528623ad93eb1`：记录刷新后的产品基线。
  - `88bf5193a7a7b4e277dd1aa4589143b3a91edd6f`：将连续验收覆盖 tag 纳入 L3 bundle。
  - `45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`：验收覆盖仅检查连续性基线之后的记录。
- 明确未纳入的分支、文件或后续事项：
  - `codex/light-node-remote-terminal` 的轻量节点 SSH 终端扩展仍有 32 项未提交改动，未进入本版；它没有本版所需的独立提交、审查和 Linux 候选 CI 证据。
  - 旧 `codex/process-heatmap` 工作树的脏差异未进入本版；原有进程热力图已在 `v0.99.4` 主线，脏差异还包含 i18n 回退，未作为新功能重复发布。
  - Claude `feature/visual-refinement-pass` 的旧基线代码未直接 cherry-pick：其中 `c210688` 与当前主线视觉修复 patch-id 等价且旧分支会删除 `v0.100.0` 内容；本版采用当前主线上的等价视觉实现，并纳入 Claude 的治理提交 `27f2e574...` 与文档索引提交 `62991fc...`。
  - 未复用旧 `v0.100.0` 生产证据；未连接、备份、部署、升级或核对 `108`/`prod-108`。
  - 本版未执行真实浏览器人工视觉巡检、长期 soak、生产故障注入和受控回滚演练。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 `v1.0.0-45a2e1d-l3-r5` 的 Go 全量测试、前端 130 文件/1099 测试、集群通知相关回归和 `app_conf_lifecycle=pass` 均通过；生产公开入口 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`version=1.0.0`。 | 集群通知真实外部收件人的长周期投递未在生产触发；由组件、后端和 L3 回归覆盖，列入后续专项。 |
| 网络入侵与供应链安全 | 已验证 | L3 的 `govulncheck`、npm audit、Trivy 源码/依赖/配置和 native 镜像扫描均无阻断性发现；Trivy 的 Go 二进制、package lock、Dockerfile 结果均为 0；Release 完成运行时镜像契约和多架构推送。受管脚本 revision `6e65c0cd7028cb198efb0c88a57726713ee1b23b`，SHA-256 `48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。 | 未执行独立外部渗透测试；新增业务不改变宿主机权限边界。 |
| 稳定性、失败恢复与兼容 | 已验证 | Linux clean checkout 的 Go 全量测试、核心 race、构建、生命周期负例和 L3 远端门禁通过；生产 postdeploy 为 Panel `running/healthy`、`RestartCount=0`、`OOMKilled=false`，Agent `loaded/active/running/enabled`，近 10 分钟日志无 `panic/fatal/OOM`。 | 未执行长期 soak、生产故障注入和真实回滚演练；本版保留完整 v0.100.0 回滚点和本轮备份。 |
| 性能与资源预算 | 已验证 | postdeploy 资源快照：Panel `74.63MiB / 256MiB`、`MemPerc=29.15%`、`CPUPerc=0.03%`、`PIDs=7`、`BlockIO=0B / 32.8kB`；前端构建完成并保持路由分块。 | 未执行长期压力测试；当前未发生 OOM，未引入新的常驻轮询任务。 |
| 用户体验与可访问性 | 已实现未实机验证 | 精确候选的视觉契约、布局、桌面模式、体检窗口滚动和三级页面 i18n 自动回归通过；`390x844` 体检窗口交互验证来自纳入的已验证修复历史。 | 本次未重新执行完整真实浏览器矩阵：浅/深色主题、100%/125%/200% 缩放、最小计算字号、键盘/焦点和人工三语巡检。 |
| 数据、配置与迁移 | 已验证 | 本版无数据库迁移；生产 preflight/backup/postdeploy 的 `protected.sha256` 一致，`protected.diff` 为 0 字节；SQLite 检查为 `ai.db empty`、`panel/ai.db ok`；备份六项文件 SHA256 校验全部通过。 | 不适用 schema/data migration；通知规则旧字段迁移由单元测试和 Go 全量测试覆盖。 |

## 自动门禁

- 定向测试及结果：`npm run i18n:check` 验证 2102 个短语、21 个 lazy catalog；`npm run typecheck` 通过；前端 130 个测试文件、1099 个测试通过；`verify-governance.sh` 为 100/100；L3 中 Go 全量测试、核心 race、`go vet`、双架构构建和应用配置生命周期均通过。
- `make verify-release` 环境和结果：L3 run=`v1.0.0-45a2e1d-l3-r5`，candidate=`45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`，base main=`89a384c7d65c42b14222dcace8843ff23602dc11`，base tag=`v0.100.0`；`verification_preflight=pass platform=Linux level=release tools=docker,go,gofmt,make,npm`；`release_gate_runner=pass`；`release_l3_gate=pass`；`release_l3_remote=pass target=arena-154`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run=`v1.0.0-45a2e1d-l3-r5`，status=`passed`、exit code `0`，`started_at=2026-09-02T10:35:25Z`、`finished_at=2026-09-02T10:47:12Z`；Runner=`kpanel-release-gate:go1.26.7-node24`，不可变 Runner ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle SHA-256=`a10d68ba84ea32b786308d2fcab2ef2e44cb7d5d105d88397de8106a0a182f05`；plan SHA-256=`f2966404a5d804bea75b1cd9f33d756ca3ba71ce51d8880d4b4cd862bd278b99`；远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；远端日志 SHA-256=`b8edef4b44aa0f399348fa49a54c37d0858ea3f3929baa9415cd2527b344bb06`；远端证据目录=`/root/kpanel-release-evidence/v1.0.0-45a2e1d-l3-r5`，本地产物目录=`C:\GitHub\_release-artifacts\v1.0.0-45a2e1d-l3-r5`。
- 候选 CI：GitHub `CI #756` / run `33619809717` 成功；Dependency freshness `#302` / run `33619809916` 成功；均绑定候选精确 SHA `45a2e1d...`。
- 主线 CI：主线从 `89a384c` fast-forward 到 `45a2e1d...` 后，`CI #757` / run `33621346338` 成功；主线 Dependency freshness `#303` / run `33621346359` 成功。
- Release workflow：`Release #195` / run `33621916915` 成功；tag 侧 Dependency freshness `#304` / run `33621917030` 成功；GitHub Release ID=`381179612`，已公开、非 draft、非 prerelease。
- 安全扫描、镜像契约、SBOM/provenance：Release 的源码、Go/Node 依赖、native 镜像扫描、运行时镜像契约和双架构构建均成功；Release 资产包含 agent/node amd64/arm64、部署包、`SHA256SUMS`、许可证和第三方声明；OCI 含双架构镜像和 attestation manifests。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选 `Dependency freshness #302`、主线 `#303`、tag `#304` 均对相应精确 SHA/发布事件成功；检测源完整，报告中的升级建议未直接纳入本版。
- 最近每日安全通告审计、EOL 复核状态及证据：L3 的 `govulncheck`、npm audit、Trivy 均通过；没有阻断性 EOL 或安全行动项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未升级 Go/Node 基座或应用依赖；依赖 freshness 的兼容、minor、major 信号保留为 report-only 后续评估，不以报告存在推断已修复或已采用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go `1.26.7`、Node `24.20.0`、L3 runner `kpanel-release-gate:go1.26.7-node24`、Trivy `0.72.0`、受管 `kejilion.sh` revision `6e65c0cd7028cb198efb0c88a57726713ee1b23b`；本版未引入新的第三方依赖。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：产品 `VERSION=1.0.0`，`internal/version.Version=1.0.0-dev`，web `package.json` 与 lock 根版本均为 `1.0.0`；受管脚本 SHA-256=`48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：未将轻量节点 32 项脏改动、旧进程热力图脏差异、旧基线 Claude 视觉分支直接纳入；退出条件为重新基于当前主线形成干净提交、独立审查、Linux CI/L3 和对应实机/回滚证据后进入下一版本候选。Trivy 版本升级列入后续维护，不阻断本版。
- 升级后的兼容、安全、构建、性能资源和回滚结论：Release、双架构 OCI、生产标准入口、Panel/Agent 健康、配置保护和 SQLite 检查均通过；无数据迁移，保留可验证的 v0.100.0 tag、镜像和本轮备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux/amd64；L3 runner 使用 Go `1.26.7`、Node.js `24`、Docker；生产 Panel 为 Docker 容器，Agent 为 systemd 服务。
- 环境策略 ID 与允许用途：`environment-policy.json` 中 `arena-154`（role=`hybrid`）；本次使用 `candidate-validation`、`production-safety-check`、`production-deploy`。未使用 `prod-108`/`108`。
- 使用的精确候选或公开产物：源码/标签产品 SHA=`45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`，annotated tag=`v1.0.0`；公开 OCI index=`sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3=`v1.0.0-45a2e1d-l3-r5` / passed / exit 0；生产 preflight=`v1.0.0-45a2e1d-prod-preflight-r2` / passed / exit 0，plan SHA-256=`d60aa026783408830755d882210f65108dab80bd1eacc6295498ccdca4893fb0`；backup=`v1.0.0-45a2e1d-prod-backup-r1` / passed / exit 0，plan SHA-256=`2de462a841babdcfb8c69707aa72a2519c5dae8040b1fe2e9fe6f4f17b160210`；postdeploy=`v1.0.0-45a2e1d-prod-postdeploy-r1` / passed / exit 0，plan SHA-256=`044d2f9cd4f03ccf6d743c0e3bcde60547a685f3cc19faa0af785ffc2b2b6405`；三阶段生产远端脚本 SHA-256 均为 `129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`；本地证据目录均位于 `C:\GitHub\_release-artifacts\`，远端证据位于 `/root/kpanel-release-evidence/`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：L3 窗口为 `2026-09-02T10:35:25Z` 至 `10:47:12Z`；生产 preflight 为 `11:03:07Z` 至 `11:03:09Z`，backup 为 `11:03:30Z` 至 `11:03:40Z`，postdeploy 为 `11:04:52Z` 至 `11:04:54Z`；未执行长期 soak，原因是本版无数据库迁移、无新常驻轮询任务，已用 L3 全量门禁与 postdeploy 健康证据覆盖基础风险。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动回归覆盖视觉契约、窄视口体检窗口滚动、对话框布局和三语资源；公网 health endpoint 已从外部返回 HTTP 200。完整真实浏览器的 100%/125%/200% 缩放、最小计算字号、浅/深色主题、键盘/焦点和人工三语矩阵本次未执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 归档旧容器 inspect、旧镜像、应用目录、Agent unit 和 `kpanel.conf`，路径为 `/root/kpanel-backups/pre-v1.0.0-20260902T110330Z`，六项 `SHA256SUMS` 校验通过；标准应用市场更新重建容器并拉取正式 `latest` digest；postdeploy 通过。未执行故障注入和实际回滚。
- 未执行场景及原因：长期 soak、真实浏览器人工巡检、125%/200% 缩放、生产故障注入、受控回滚演练、集群通知真实外部投递、完整 `image-e2e` 独立脚本和系统中心真实宿主机动作未执行；它们不替代已完成的自动门禁和生产安全证据，列入后续专项。

## 发布产物与公开仓库复核

- GitHub Release：[`v1.0.0`](https://github.com/kejilion/KPanel/releases/tag/v1.0.0) 已公开，`draft=false`、`prerelease=false`、`target_commitish=main`，Release ID=`381179612`，published=`2026-09-02T11:01:38Z`；annotated tag `v1.0.0` 剥离后的产品提交为 `45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:1.0.0` 与 `:latest` 均为 `sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:3ffe214e71098cea53a2caf5990a97e51f5880f4d130bea1acc42294b5f5ed8a`；arm64=`sha256:e84eaaa6693235e0e92ebfef32d3c732177502c06595dacbc31cf8091e96ab84`；另含 attestation manifests。
- 附件及 `SHA256SUMS`：Release 含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-1.0.0.tar.gz`、`LICENSE`、`SHA256SUMS`、`THIRD_PARTY_NOTICES.md`。
- 公开镜像 `image_e2e=pass`：L3 native 镜像扫描、运行时镜像契约和生产标准入口实际拉取/运行均通过；本版未单独执行 `packaging/tests/image-e2e.sh`，不把该独立脚本标记为已执行。
- `kejilion/apps` / `kejilion.sh` 契约结论：本版未修改应用市场配置契约；标准入口 `/home/docker/kpanel/bin/kejilion.sh app kpanel` 成功拉取并运行上述 OCI digest，Agent 与 Panel 版本契约通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户已明确授权本次全面梳理、主线集成、`v1.0.0` 发布和生产上线；生产写入仅执行 `arena-154`。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`，用途为 `candidate-validation`、`production-safety-check`、`production-deploy`。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight `v1.0.0-45a2e1d-prod-preflight-r2` 通过，健康版本 `0.100.0`、`status=ok`、`initialized=true`，Agent active/running/enabled，Panel running/healthy、RestartCount=0、OOM=false；backup `v1.0.0-45a2e1d-prod-backup-r1` 通过，备份=`/root/kpanel-backups/pre-v1.0.0-20260902T110330Z`，归档与旧镜像恢复验证全部 OK。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy `v1.0.0-45a2e1d-prod-postdeploy-r1` 通过；health `status=ok`、`version=1.0.0`、`protocolVersion=v1alpha1`；Panel `running/healthy`、`RestartCount=0`、`OOMKilled=false`、容器 RepoDigest=`kjlion/kejilion-panel@sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`；镜像 label revision=`45a2e1d...`、version=`1.0.0`；Agent `LoadState=loaded`、`ActiveState=active`、`SubState=running`、`UnitFileState=enabled`、`NeedDaemonReload=no`；SQLite quick check 通过；`protected.diff=0`；日志无 `panic/fatal/OOM`；公网 health endpoint HTTP 200。
- 生产已执行写操作：生产证据目录和一致性备份写入；标准应用市场通过受管 `kejilion.sh` 拉取 `latest`、重建 `kejilion-panel` 容器并更新 Agent；未执行其他系统中心或轻量节点写操作。
- 仅在隔离真机执行、未在生产执行的场景：L3 构建、race、govulncheck、Trivy、前端全量测试、应用配置生命周期负例和治理测试；生产未执行故障注入、强制回滚和人工浏览器巡检。

## 回滚

- 源码/tag：回滚点 `v0.100.0` / `89a384c7d65c42b14222dcace8843ff23602dc11`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel:0.100.0` 的生产 preflight RepoDigest 为 `kjlion/kejilion-panel@sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`；备份内 `old-image.tar.zst` 已自包含旧镜像并完成 `image-load-verify.txt` 校验。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.0.0-20260902T110330Z`，含旧容器 inspect、旧镜像归档、应用目录归档、Agent unit、`kpanel.conf` 和 `SHA256SUMS`；六项 SHA256 校验均通过。
- 回滚步骤和回滚后复核：先停止当前 Panel/Agent，恢复备份中的 Compose、`.env`、数据、旧镜像和 Agent 文件，固定 Compose 到 `v0.100.0` 公开镜像，按标准应用市场/受管脚本协议恢复，再执行同一 production postdeploy 证据门禁。本次未触发回滚。
- 回滚后生产实际版本与健康状态：未执行回滚，因此不适用；当前 `v1.0.0` postdeploy 已验证健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest 为 `v1.0.0`；Docker `latest` 与 `1.0.0` 同 OCI index=`sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`；标准入口已在生产拉取并运行该 digest。
- 公共默认更新通道决策：保留 `v1.0.0` 为默认稳定通道；未发现需要恢复旧默认版本的产品问题。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-02T10:43:10+08:00
- 候选冻结时间：2026-09-02T18:35:16+08:00
- 生产完成时间：2026-09-02T19:04:54+08:00
- 提交到生产用时：8.36 小时
- 是否回滚、紧急热修复或重复发布：否（生产写入前的产品缺陷和发布证据问题均被门禁拦截并修复，未造成生产变更失败）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：5
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/business-context/stale-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 在业务上下文基线仍指向旧稳定版时被准备门禁拦截，未上传可执行发布包到生产验证流程。",
    "recoveryEvidence": "run v1.0.0-37ce91d-l3-r1 在候选准备阶段失败；随后 9389a29 刷新基线至 89a384c/v0.100.0，最终 r5 manifest 明确 baseTag=v0.100.0、baseMainCommit=89a384c。",
    "permanentAction": "把当前产品事实基线作为候选发布前必过的机器门禁，并在 L3 bundle 中固定 base tag/commit；后续候选必须以精确稳定基线重新生成 bundle。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/coverage-bundle/missing-stable-tags",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r2 的自包含 bundle 未携带连续验收覆盖所需历史稳定 tag，发布验收覆盖门禁 fail-closed。",
    "recoveryEvidence": "run v1.0.0-414423c-l3-r2 的 release acceptance coverage 失败；88bf519 修正 L3 bundle 的 required stable tags，最终 r5 报告 baseline=v0.83.0、in_scope=45、tags=45、in_flight_exempt=none。",
    "permanentAction": "L3 外层入口从 COVERAGE_BASELINE 之后枚举并携带全部候选可达稳定 tag，且用 release-l3-orchestrator 回归锁定 bundle 自包含性。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/local-tags/stale-tag-object",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r3 在候选工作树的本地旧 v0.86.2 tag 与远端对象不一致时被本地准备门禁拦截，未执行远端验证。",
    "recoveryEvidence": "run v1.0.0-88bf519-l3-r3 在 tag 一致性检查失败；随后在独立 clean clone 同步远端 tag，并用该 clone 完成 r4/r5。",
    "permanentAction": "发布 L3 使用独立 clean clone 和远端 tag 同步结果，不依赖共享候选工作树中的陈旧本地 tag；保留 tag/commit 双重校验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/acceptance-coverage/prebaseline-orphans",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r4 把连续性基线之前的历史验收记录误判为当前覆盖孤儿，门禁拒绝发布，未产生生产写入。",
    "recoveryEvidence": "run v1.0.0-88bf519-l3-r4 的 acceptance coverage 失败；45a2e1d 将 orphan 检查限定到 COVERAGE_BASELINE 之后，并新增回归，r5 100/100 治理通过。",
    "permanentAction": "验收覆盖门禁按连续性基线分层检查，不要求回填基线之前的历史记录；新增 before-baseline orphan 回归并由 L3 重跑确认。",
    "historicalReleases": []
  },
  {
    "fingerprint": "production-evidence/preflight/target-version-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮生产 preflight 把待部署版本 1.0.0 当作部署前期望版本，因 arena-154 实际仍为 0.100.0 被 fail-closed 拦截；未创建备份、未更新应用。",
    "recoveryEvidence": "run v1.0.0-45a2e1d-prod-preflight-r1 status=failed/exit_code=1，snapshot health.version=0.100.0；按 preflight 基线语义以 0.100.0 重跑，r2 status=passed/exit_code=0。",
    "permanentAction": "preflight 固定检查当前生产基线版本，backup 使用目标版本命名，postdeploy 才绑定目标 version/revision/digest；三阶段 plan 由 run-production-evidence.mjs 生成并保留原始失败证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：真实浏览器人工验收（浅/深色主题、键盘/焦点、100%/125%/200% 缩放、最小计算字号、三语巡检）、长期 soak、生产故障注入和受控回滚演练未执行；集群通知真实外部投递未执行；`packaging/tests/image-e2e.sh` 未单独执行；系统中心真实宿主机动作不属于本版范围。
- 已实现待实机准入：集群累计通知与时区、桌面模式体检窗口滚动、统一视觉系统、对话框视口、三级页面 i18n 和发布治理已通过自动回归、L3 和生产健康证据；真实浏览器矩阵和通知外部投递应在后续专项补齐。
- 不阻断本版的理由：候选/main/tag freshness、候选/main CI、L3 全量门禁、公开 Release/OCI、arena-154 preflight/backup/postdeploy 均绑定精确 SHA 或 immutable digest 通过；未验证场景不涉及数据库迁移或新增宿主机权限，且回滚 tag、公开旧镜像和本轮备份已保留。
- 后续应进入的自动门禁或专项工作流：把真实浏览器视觉/交互矩阵、长期 soak、受控回滚和独立 `image-e2e` 纳入版本门禁；补集群通知外部投递的隔离验收；继续维护 `arena-154` 磁盘水位和依赖/扫描器 freshness。
