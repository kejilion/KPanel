# KPanel v1.0.1 发布验收记录

日期：2026-09-03

发布级别：L3

候选提交 / 标签：`0e9020d39f585be8a79fb3d5238fe7599bb9dab1` / `v1.0.1`

上一稳定版本 / 回滚点：`v1.0.0` / `45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`，生产回滚镜像为 `sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`

> 本记录对应 Claude 参与的窄桌面窗口界面优化补丁 `v1.0.1`。候选、主线、标签、Release、OCI、L3、备份和 postdeploy 证据均绑定本次精确产品 SHA；仅使用 `v1.0.0` 作为上一稳定版本和回滚点，未复用其验收或生产证据。本次生产操作只针对 `arena-154`，未连接、未读取、未备份、未部署、未升级或核对 `108`/`prod-108`。

## 发布画像

- 业务域：窄桌面窗口下的 Web 应用市场、集群和 Docker 页面布局密度与可用性。
- 变更面：展示层与前端回归测试，另含版本元数据和发布产物；不涉及宿主机写入、API、协议、数据迁移或权限。
- 受影响用户旅程：在较窄的桌面模式窗口中打开 Apps、Cluster、Docker 页面，查看英雄区统计、操作按钮、资源区和空状态。
- 未变化契约：API、数据、端口、Compose、Agent 权限与协议、`kejilion.sh`、应用市场安装/更新入口均未变化；不新增 System Center API、路由、宿主机动作或权限边界。
- 风险等级及理由：低至中风险。产品变更限定于现有 Web 布局和测试，影响面为窄窗口展示；仍按 L3 完成 Linux 全量门禁、供应链扫描、公开产物、生产备份和 postdeploy。

## 发布范围与未纳入内容

- 用户可见更新：
  1. Apps 和 Cluster 英雄区在窄窗口中改为可换行布局，统计信息保持可读，操作区可换行。
  2. Docker 命令中心使用 `@container desktop-window` 适配窄窗口，压缩资源区标题、按钮和空状态的垂直占用，避免高度弹性把内容推离可视区域。
  3. 四个最小可读字号标签从视觉债务基线移除并提升到 12px；补充 420px 窄窗口布局回归。
- 精确提交清单（均已进入产品标签 `v1.0.1`）：
  - `a6f9ef777980b479f88cd2ba97656685d048e352`：`fix(web): reclaim vertical space in narrow desktop windows`；Claude Opus 5 共同署名的干净 UI 提交，包含 `desktop.css`、Apps/Cluster/Docker 视图与测试，以及 `NarrowWindowHeroLayout.test.ts`。
  - `cd85d3472dee800e92a23fa857b59a531d5ac0d4`：准备 KPanel `1.0.1` 的 `VERSION` 与 `CHANGELOG.md`。
  - `0e9020d39f585be8a79fb3d5238fe7599bb9dab1`：同步 `internal/version`、`web/package.json` 和 lock 文件中的嵌入版本元数据。
- 候选基线为 `origin/main=b84710f85da8ec094c06fc938e4724b150b5f49f`；相对该基线只有上述 3 个提交、13 个文件，差异为 249 insertions / 35 deletions。主线已有的 `v1.0.0` 验收文档是基线内容，不是本补丁重新纳入的业务变更。
- 明确未纳入的分支、文件或后续事项：
  - Claude 旧的 `feature/visual-refinement-pass` 分支基线陈旧且工作树不干净，未直接 cherry-pick；只采用基于当前主线、可审查的 `a6f9ef7` UI 提交。
  - System Center 页面、路由、API、文档和宿主机动作不在候选差异中；未执行任何 System Center 生产操作。
  - 轻量节点 SSH、后端、Agent、数据库、端口、Compose、`kejilion.sh` 和应用市场脚本契约未进入本版。
  - 未复用 `v1.0.0` 的 L3、CI、Release、OCI、备份或生产证据；未连接 `108`/`prod-108`。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 `v1.0.1-0e9020d-l3-r2` 的治理测试 100/100、Go 全量测试、Web 131 个测试文件/1107 个测试、应用配置生命周期负例和构建均通过；生产公开 health 返回 HTTP 200、`status=ok`、`version=1.0.1`。 | 本版没有新增后端业务或双端协议；真实用户数据流不需要迁移验证。 |
| 网络入侵与供应链安全 | 已验证 | L3 的 `govulncheck`、npm audit、Trivy 源码/依赖/配置/镜像扫描均无阻断性发现；npm audit 为 0 vulnerabilities，镜像与源码扫描无漏洞、secret 或 misconfiguration；Release 完成双架构 OCI 与运行时契约。 | 未执行独立外部渗透测试；本版不改变权限边界。 |
| 稳定性、失败恢复与兼容 | 已验证 | Linux clean clone 的 L3 全量门禁通过；生产 postdeploy 为 Panel `running/healthy`、`RestartCount=0`、`OOMKilled=false`，Agent `loaded/active/running/enabled`，近 10 分钟日志无 `panic/fatal/OOM`。 | 未执行长期 soak、生产故障注入和真实回滚演练；上一稳定 tag、旧镜像和本轮备份均保留。 |
| 性能与资源预算 | 已验证 | postdeploy 快照：Panel `74.75MiB / 256MiB`、`MemPerc=29.20%`、`CPUPerc=0.03%`、`PIDs=7`、`BlockIO=0B / 32.8kB`；本版只减少窄窗口布局占用，不新增常驻任务。 | 未执行长期压力测试；当前没有 OOM 或异常重启。 |
| 用户体验与可访问性 | 已验证 | 精确候选的隔离浏览器验收在 1440x900 视口完成；Apps、Cluster、Docker 的窄窗口 client width 为 420px、scroll width 为 420px、`overflowX=hidden`，无横向溢出；浅色 Apps/Cluster、深色 Docker 检查无 console error/warning。 | 未执行完整人工 100%/125%/200% 缩放、最小计算字号、键盘/焦点和人工三语矩阵；本地预览为 `mode=mock`，不替代真实 Panel/Agent/宿主机业务验收。 |
| 数据、配置与迁移 | 已验证 | 本版无数据库迁移；生产 preflight/backup/postdeploy 的受保护文件摘要一致，`protected.diff` 为 0 字节；SQLite 为 `ai.db empty`、`panel/ai.db ok`；备份六项 SHA256 全部通过。 | 不适用 schema/data migration。 |

## 自动门禁

- 定向测试及结果：`npm run i18n:check` 检查 2102 个 localized phrases、21 个 lazy catalogs；`npm run typecheck` 通过；Web 全量 131 个测试文件、1107 个测试通过；治理测试 100/100；L3 的 Go 全量、核心 race、`go vet`、双架构构建、Docker 构建/扫描/运行时契约和 `app_conf_lifecycle=pass` 均通过。
- `make verify-release` 环境和结果：L3 `v1.0.1-0e9020d-l3-r2`，candidate=`0e9020d39f585be8a79fb3d5238fe7599bb9dab1`，base main=`b84710f85da8ec094c06fc938e4724b150b5f49f`，base tag=`v1.0.0`，business baseline=`v0.100.0` / `89a384c7d65c42b14222dcace8843ff23602dc11`；`verification_preflight=pass platform=Linux level=release tools=docker,go,gofmt,make,npm`；`release_gate_runner=pass`、`release_l3_gate=pass`、`release_l3_remote=pass target=arena-154`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run=`v1.0.1-0e9020d-l3-r2`，status=`passed`、exit code `0`，`started_at=2026-09-02T17:41:39Z`、`finished_at=2026-09-02T17:52:34Z`；Runner=`kpanel-release-gate:go1.26.7-node24`，不可变 Runner ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle SHA-256=`6308937ce2d121aca056a56b0280b8057bcb80990c62b1e02230c3417d43c8d4`；plan SHA-256=`432538a8bd01a59342002400394f54e1fc63e36390839a27f974bd325eace875`；远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；远端日志 SHA-256=`627e7a6771853cdb20800551fc2058bda491bbca991821680631e3e45494a01d`；远端证据目录=`/root/kpanel-release-evidence/v1.0.1-0e9020d-l3-r2`，本地 artifact 目录名=`v1.0.1-0e9020d-l3-r2`。
- 候选 CI：[`CI #760`](https://github.com/kejilion/KPanel/actions/runs/33661676606) / run `33661676606` 成功，`Dependency freshness #306` / run `33661676589` 成功；均绑定候选精确 SHA `0e9020d39f585be8a79fb3d5238fe7599bb9dab1`。先前 `CI #759` / run `33661046200` 在嵌入版本元数据一致性门禁失败，未进入生产，修正后 #760 成功。
- 主线 CI：主线从 `b84710f...` fast-forward 到产品 SHA `0e9020d...` 后，[`CI #761`](https://github.com/kejilion/KPanel/actions/runs/33664291584) / run `33664291584` 成功；`Dependency freshness #307` / run `33664291540` 成功，均绑定产品 SHA。
- Release workflow：[`Release #196`](https://github.com/kejilion/KPanel/actions/runs/33664848153) / run `33664848153` 成功；tag 侧 `Dependency freshness #308` / run `33664847771` 成功，均绑定产品 SHA。
- 安全扫描、镜像契约、SBOM/provenance：L3 的源码、Go/Node 依赖、native 镜像扫描、运行时镜像契约、双架构构建和 attestation manifests 均通过；Release 资产包含 agent/node amd64/arm64、部署包、`SHA256SUMS`、许可证和第三方声明。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本版未修改依赖；候选 `Dependency freshness #306`、主线 `#307`、tag `#308` 均对相应精确 SHA 成功，检测源完整。未把报告中的升级建议当作已采用变更。
- 最近每日安全通告审计、EOL 复核状态及证据：L3 的 `govulncheck`、npm audit 和 Trivy 均通过；未发现阻断性 EOL 或安全行动项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版没有 Go/Node 基座、第三方依赖或 Action 升级，不产生新的依赖处置项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go `1.26.7`、Node `24.20.0`、L3 runner `kpanel-release-gate:go1.26.7-node24`、Trivy `0.72.0`；受管脚本 revision=`6e65c0cd7028cb198efb0c88a57726713ee1b23b`，SHA-256=`48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`；本版未引入第三方依赖。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：`VERSION=1.0.1`、`internal/version.Version=1.0.1-dev`、Web package/lock 根版本均为 `1.0.1`；公开 OCI index=`sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：旧 Claude 宽泛视觉分支、未提交轻量节点改动及 System Center 范围均拒绝纳入；退出条件为重新基于当前主线形成独立提交、审查、Linux CI/L3 和对应实机证据。独立 `packaging/tests/image-e2e.sh` 和完整人工视觉矩阵列入后续专项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：版本元数据一致，候选/main/tag freshness、L3、Release、OCI、arena-154 preflight/backup/postdeploy 均绑定精确 SHA 或 immutable digest 通过；无数据迁移，保留 `v1.0.0` 回滚点与本轮生产备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux/amd64；L3 runner 使用 Go `1.26.7`、Node.js `24`、Docker；生产 Panel 为 Docker 容器，Agent 为 systemd 服务。浏览器布局验收使用 Windows 本地精确候选构建预览。
- 环境策略 ID 与允许用途：`environment-policy.json` 中 `arena-154`（role=`hybrid`）；本次仅使用候选验证、`production-safety-check` 和 `production-deploy` 允许用途。
- 使用的精确候选或公开产物：源码/产品 tag SHA=`0e9020d39f585be8a79fb3d5238fe7599bb9dab1`、annotated tag=`v1.0.1`；公开 OCI index=`sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3=`v1.0.1-0e9020d-l3-r2` / passed / exit 0；生产 preflight=`v1.0.1-0e9020d-prod-preflight-r1` / passed / exit 0，plan SHA-256=`0e9d7bdff37f5791a91c2d8d196a033cac3d6c936e9c2f8549ab786488e708da`；backup=`v1.0.1-0e9020d-prod-backup-r1` / passed / exit 0，plan SHA-256=`a6a12c47cebd03532f6da1c331c6c0c63027ea46f7b5dd7d24a3ab8d5059c1e6`；postdeploy=`v1.0.1-0e9020d-prod-postdeploy-r1` / passed / exit 0，plan SHA-256=`7241d4fbf2aee94c8196eadbac69feda9465df349f094ce283515d24a350`；三阶段生产远端脚本 SHA-256 均为 `129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`；本地 artifact 目录名以各 run ID 标识，远端证据位于 `/root/kpanel-release-evidence/`。
- 测试窗口/循环数及风险依据：L3 为 `2026-09-02T17:41:39Z` 至 `17:52:34Z`；生产 preflight 为 `18:13:23Z` 至 `18:13:25Z`，backup 为 `18:14:03Z` 至 `18:14:13Z`，postdeploy 为 `18:18:09Z` 至 `18:18:11Z`；本地精确候选预览 evidence manifest SHA-256=`d9c850a42e6712559d355459614ad557febae82769af881d05a80adea6ead790`，模式为 `mock`、grade=`acceptance`。未执行长期 soak，原因是本版无数据迁移和新常驻任务。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：真实浏览器在 1440x900 页面中打开窄桌面窗口，Apps/Cluster/Docker 的窗口外框约 422px、client width 420px，三页 scroll width 均为 420px，未发生横向溢出；已检查浅/深色主题组合和控制台无 error/warning。局部预览清单名为 `kpanel-v1.0.1-narrow-window-final-preview/manifest.json`，明确为 mock，只证明 UI/交互与失败反馈，不证明真实 Panel/Agent/host/Docker。
- 宿主机写入、失败注入、重启恢复和回滚结果：生产唯一写入为标准应用市场更新和本次证据归档；backup 路径为 `/root/kpanel-backups/pre-v1.0.1-20260902T181403Z`，六项 SHA256 校验通过；未执行故障注入、强制回滚或重启恢复演练。
- 未执行场景及原因：真实浏览器 100%/125%/200% 缩放、最小计算字号、键盘/焦点、人工三语矩阵、长期 soak、生产故障注入、受控回滚演练和独立 `packaging/tests/image-e2e.sh` 未执行；它们不替代已完成的自动门禁、隔离浏览器几何检查和生产安全证据。

## 发布产物与公开仓库复核

- GitHub Release：[`v1.0.1`](https://github.com/kejilion/KPanel/releases/tag/v1.0.1) 已公开，`draft=false`、`prerelease=false`、`target=main`，Release ID=`381473229`，published=`2026-09-02T18:09:44Z`；annotated tag `v1.0.1` 剥离后的产品提交为 `0e9020d39f585be8a79fb3d5238fe7599bb9dab1`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:1.0.1` 与 `:latest` 均为 `sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:ad7ec441d087212ceba052a2ba68e28e3a40ae9d4f0f88fb7c458b334dbc1e33`；arm64=`sha256:de9c65e0d49c838bfdad2a46dff5ea79300eccb7730167f27f8b265b2a984d16`；OCI 另含 attestation manifests。
- 附件及 `SHA256SUMS`：Release 含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-1.0.1.tar.gz`、`LICENSE`、`SHA256SUMS`、`THIRD_PARTY_NOTICES.md`；`SHA256SUMS` 列出的 5 个可执行/部署资产已下载并逐一校验，5/5 exact match。
- 公开镜像 `image_e2e=pass`：L3 native 镜像扫描、运行时镜像契约和生产标准入口实际拉取/运行上述 OCI digest 均通过；未单独执行 `packaging/tests/image-e2e.sh`，不把该独立脚本标记为已执行。
- `kejilion/apps` / `kejilion.sh` 契约结论：本版未修改应用市场配置或脚本契约；标准入口 `/home/docker/kpanel/bin/kejilion.sh app kpanel` 成功拉取 `latest`、重建 `kejilion-panel`，并运行上述 OCI digest，Agent 与 Panel 版本契约通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户已明确授权本次 Claude 界面优化补丁走完整上线流程；生产写入仅执行 `arena-154`。
- 验证/灰度环境：`environment-policy.json` 中允许的 `arena-154`，用途为候选验证、`production-safety-check`、`production-deploy`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight `v1.0.1-0e9020d-prod-preflight-r1` 通过，生产健康 `status=ok`、`version=1.0.0`、`protocolVersion=v1alpha1`，Agent active/running/enabled，Panel running/healthy、RestartCount=0、OOM=false；旧镜像 RepoDigest=`kjlion/kejilion-panel@sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`。backup `v1.0.1-0e9020d-prod-backup-r1` 通过，备份=`/root/kpanel-backups/pre-v1.0.1-20260902T181403Z`，`protected.diff=0`，六项归档摘要通过。
- 部署命令/入口：`ssh arena-154 'env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel'`；标准应用市场更新输出确认拉取 `kjlion/kejilion-panel:latest` 并重建容器。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy `v1.0.1-0e9020d-prod-postdeploy-r1` 通过，`started_at=2026-09-02T18:18:09Z`、`finished_at=2026-09-02T18:18:11Z`；health `status=ok`、`version=1.0.1`、`protocolVersion=v1alpha1`；Panel `running/healthy`、`RestartCount=0`、`OOMKilled=false`，镜像 RepoDigest=`kjlion/kejilion-panel@sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`，label revision=`0e9020d39f585be8a79fb3d5238fe7599bb9dab1`、version=`1.0.1`；Agent `LoadState=loaded`、`ActiveState=active`、`SubState=running`、`UnitFileState=enabled`、`NeedDaemonReload=no`；SQLite quick check 通过，`protected.diff=0`，日志无 `panic/fatal/OOM`。公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 于 `2026-09-02T18:18:34Z` 返回 HTTP 200、`initialized=true`、`status=ok`、`version=1.0.1`。
- 生产已执行写操作：生产证据目录和一致性备份写入；标准应用市场通过受管 `kejilion.sh` 拉取正式 `latest`、重建并启动 `kejilion-panel`；未执行 System Center、轻量节点或其他主机的写操作。
- 仅在隔离环境执行、未在生产执行的场景：L3 构建、race、govulncheck、Trivy、Web 全量测试、治理测试、应用配置生命周期负例和本地浏览器布局交互；生产未执行故障注入、强制回滚和完整人工浏览器矩阵。

## 回滚

- 源码/tag：回滚点 `v1.0.0` / `45a2e1dfeed3f249eb509bf2fe5d3dae0da3d068`。
- 镜像 digest：生产 preflight 的旧镜像 RepoDigest=`kjlion/kejilion-panel@sha256:b7f9413b1576f6211df649ebade39040f33ff9044997b53b3361c8a62c2e44b5`；对应旧镜像已在 backup 的 `old-image.tar.zst` 中归档并通过 `image-load-verify.txt` 校验。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.0.1-20260902T181403Z`，包含旧容器 inspect、旧镜像、应用目录、Agent unit、`kpanel.conf` 和 `SHA256SUMS`；六项 SHA256 校验通过。
- 回滚步骤和回滚后复核：停止当前 Panel/Agent，恢复备份中的 Compose、`.env`、数据、旧镜像和 Agent 文件，固定 Compose 到 `v1.0.0` 公开镜像，再按受管 `kejilion.sh`/应用市场协议恢复并执行同一 postdeploy 证据门禁。本次未触发回滚。
- 回滚后生产实际版本与健康状态：未执行回滚，不适用；当前 `v1.0.1` postdeploy 和公网 health 已验证健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release `v1.0.1` 已公开；Docker `latest` 与 `1.0.1` 同 OCI index=`sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`；标准入口已在生产拉取并运行该 digest。
- 公共默认更新通道决策：保留 `v1.0.1` 为默认稳定通道；没有发现需要恢复旧默认版本的产品问题。下一次 L3 生产写前必须先处理下方重复流程指纹。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-03T01:09:54+08:00
- 候选冻结时间：2026-09-03T01:41:39+08:00
- 生产完成时间：2026-09-03T02:18:11+08:00
- 提交到生产用时：1.14 小时
- 是否回滚、紧急热修复或重复发布：否（候选 CI #759 和 L3 r1 均在生产写入前 fail-closed，未造成生产变更失败）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/local-tags/stale-tag-object",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 在候选工作树的本地 v0.86.2 tag 与 origin 对象不一致时被准备门禁 fail-closed，未上传 bundle、未连接远端 L3、未产生生产写入。",
    "recoveryEvidence": "run v1.0.1-0e9020d-l3-r1 在 local prepare 阶段报告 local tag v0.86.2 与 origin 不一致；随后建立并同步远端 tag 的独立 clean clone，run v1.0.1-0e9020d-l3-r2 status=passed、exit_code=0，完成 Linux release 门禁和 arena-154 验证。",
    "permanentAction": "该指纹已在 v1.0.0 出现，不能用本次现场切换 clean clone 视为永久修复；下一次 L3 生产写前由发布流程维护者把 clean-clone 创建与 tag equality preflight 固化为唯一入口，并补回归。复核日期：下一候选冻结前；退出条件：从含陈旧/冲突本地 tag 的工作树启动时，入口自动生成可验证 clean clone，且不需要人工绕行即可通过 tag/commit 双校验。",
    "historicalReleases": ["v1.0.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：完整人工浏览器 100%/125%/200% 缩放、最小计算字号、键盘/焦点、人工三语巡检、长期 soak、生产故障注入、受控回滚和独立 `packaging/tests/image-e2e.sh` 未执行；它们不影响本版已完成的 L3、公开产物和生产健康证据，但应作为后续专项补齐。
- 已实现待实机准入：窄桌面窗口 Apps/Cluster/Docker 视觉和交互已在精确候选隔离浏览器中验证，并在 `arena-154` 的公开健康与运行时契约上完成生产验收；完整真实用户浏览器矩阵仍待后续准入。
- 不阻断本版的理由：产品差异仅为 Web 布局，候选/main/tag CI 与 freshness、L3、Release、OCI、arena-154 preflight/backup/postdeploy 均绑定精确候选并通过，未发生生产退化、回滚或紧急热修复；但是 `l3/local-tags/stale-tag-object` 已与 `v1.0.0` 重复，属于下一次 L3 生产写前的明确流程阻断项。
- 后续应进入的自动门禁或专项工作流：修复并回归 L3 clean-clone 唯一入口；补真实浏览器缩放/焦点/语言矩阵、长期 soak、受控回滚和独立 `image-e2e`；继续维护 `arena-154` 备份、水位和依赖 freshness。
