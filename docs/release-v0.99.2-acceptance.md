# KPanel v0.99.2 发布验收记录

日期：2026-08-29

发布级别：L3

候选提交 / 标签：`8efb92d2529711500d42832f6a39591a4188e62c` / `v0.99.2`

上一稳定版本 / 回滚点：`v0.99.1` / `sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`（commit `fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`）

## 发布画像

- 业务域：KPanel 品牌资产与应用市场展示资源。
- 变更面：展示与静态资源；不涉及 API、数据、宿主机写入、Agent 权限或部署协议。
- 受影响用户旅程：用户打开 KPanel、浏览器标签页、PWA/桌面快捷方式、应用市场和公开 README/截图时，看到统一的新 Logo、favicon、Apple touch icon、manifest 图标、Safari mask 和配套展示素材。
- 未变化契约：API、数据 schema、端口、Compose、Agent 权限、`kejilion.sh` 和应用市场功能契约均不变；应用市场仅更新 KPanel 静态图标及对应 catalog 资源哈希。
- 风险等级及理由：低；变更集中在品牌位图/SVG/WebP、静态截图、HTML 图标引用和资源校验测试，未改变业务逻辑、布局结构、数据流或宿主机写入链路。

## 发布范围与未纳入内容

- 用户可见更新：Panel Logo、favicon、Apple touch icon、PWA/manifest 图标、Safari mask、应用市场 KPanel 图标，以及 README/公开截图中的品牌素材；`web/index.html` 的 favicon 引用使用 `?v=20260829` cache bust。
- 精确提交清单：候选基线为最新 `origin/main` `34832d33e25e7cb7138b7dd82ab81e2135f7589f`；源修复 `0aeb18dcf5ce5a21b5cf0f93ec65b2532ba29b3b`（父 `9fe3ed9281722117c0e893f4b91032daa4268eb4`）在候选中形成 cherry-pick `c9e4ddbdae8f94f3f29f1ef1a7f454f58089e0fb`；版本准备提交为 `8efb92d2529711500d42832f6a39591a4188e62c`（父 `c9e4ddbdae8f94f3f29f1ef1a7f454f58089e0fb`）。相对候选基线的 24 个文件仅包括 19 个品牌变更文件与 `CHANGELOG.md`、`VERSION`、`internal/version/version.go`、`web/package.json`、`web/package-lock.json` 五个版本元数据文件。
- 明确未纳入的分支、文件或后续事项：候选差异不含 System Center 页面、路由、API 或文档；`system-settings.webp` 只是公开静态截图中的品牌素材，不是 System Center 功能变更。`kejilion/apps` 未产生提交，`kejilion.sh` 未改动，`108`/`prod-108` 未连接。真实生产浏览器 Logo 视觉矩阵未在本轮重新执行，保留为后续专项验收事项。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Web 全量 122 个文件/1046 项通过；品牌资源回归、应用市场 catalog 测试、typecheck、build 通过；生产 postdeploy 版本与镜像 revision 精确匹配 | 本版无 API、节点协议或双端数据契约变化 |
| 网络入侵与供应链安全 | 已验证 | 候选/main CI、固定 L3、Release workflow、Go/Node 安全扫描、npm audit、Trivy、OCI digest、SBOM/provenance 与受管脚本契约均通过 | 本版没有新增网络入口；未另行执行公网渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 生命周期与失败清理、候选/main CI、Release、生产备份、标准更新入口和 postdeploy 均通过；无 schema 迁移 | 未做长期 soak；品牌资源回退依赖浏览器和既有静态资源加载行为 |
| 性能与资源预算 | 已实现未实机验证 | 变更为静态资源/HTML 引用和展示素材，未改数据流或运行时布局；生产容器资源快照约 73.34 MiB / 256 MiB | 未做浏览器缓存与首次加载的长期性能曲线对比 |
| 用户体验与可访问性 | 已实现未实机验证 | 资源尺寸、格式、路径、旧 Logo 特征清理、cache bust、自动回归和构建通过；源修复已有本地预览/缓存验证 | 未执行真实生产浏览器的主题、窄视口、100%/125%/200% 缩放、键盘/焦点、最小字号和语言矩阵 |
| 数据、配置与迁移 | 已验证 | 本版无数据/schema/配置迁移；生产 `protected.diff` 为 0，SQLite quick check 为 `panel/ai.db ok`，备份六项摘要校验通过 | 不适用额外迁移验收 |

## 自动门禁

- 定向测试及结果：`npm ci --prefix web` 成功，安装 282 个包并报告 0 vulnerabilities；`npm run test --prefix web` 通过，122 个文件/1046 项；`npm run typecheck --prefix web` 通过；`npm run build --prefix web` 通过，i18n 检查为 2582 phrases / 21 lazy catalogs。Windows 本地 `node scripts/run-repo-bash.mjs scripts/verify-change.sh` 按环境策略对缺少 `go`、`gofmt` fail-closed，这是执行环境限制，不是候选产品失败；固定 Linux L3 已完成完整放行。
- `make verify-release` 环境和结果：固定 Runner `kpanel-release-gate:go1.26.6-node24`，不可变 Runner ID=`sha256:b593c0ffe32e80a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.99.2-8efb92d-l3-r1`，`release_l3_gate=pass`、`release_l3_remote=pass`、`app_conf_lifecycle=pass`、`release_gate_runner=pass`；证据目录为 `C:\GitHub\_release-artifacts\v0.99.2-8efb92d-l3-r1`，bundle SHA-256=`41fa5d87cd2bfbc4ce31a8987f2b7314e3f3b54e0fc2c5461084898988329ab1`，manifest SHA-256=`B024C314C781D1FDF7B15D0859A1F2A49F403F81B085F574140A0D93B02EA933`，plan SHA-256=`83585a145080fd8f58aafab36e1529ea1cfbd08c527d1d668d3cdf4778012b87`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：CI run `33238880179`、Dependency freshness run `33238880181`，均绑定 `8efb92d2529711500d42832f6a39591a4188e62c`，completed/success。
- 主线 CI：主线在推进前为 `34832d33e25e7cb7138b7dd82ab81e2135f7589f`，经远端 SHA guard 后 fast-forward 到产品候选 SHA；CI run `33239577985`、Dependency freshness run `33239578007` 均绑定产品 SHA，completed/success。验收记录提交后将再次以文档提交 SHA 等待主线 CI，不改变 `v0.99.2` tag 指向。
- Release workflow：run `33239742728`，`v0.99.2^{}` 精确解引用到 `8efb92d2529711500d42832f6a39591a4188e62c`，completed/success；全部构建、校验、扫描、发布、latest promotion 和候选分支清理步骤均 success。GitHub Release 已公开、非 draft、非 prerelease。
- 安全扫描、镜像契约、SBOM/provenance：Release workflow 的 Go/Node 双架构构建、native image validation、OCI 多架构推送、`latest` promotion、SBOM/provenance、Trivy、镜像运行契约和受管 `kejilion.sh` revision/SHA 校验均通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本版未新增运行时依赖或基础镜像；Dependency freshness runs `33238880181`、`33239578007` 成功，L3 的 govulncheck、npm audit 和 Trivy 通过；不据此声明所有上游依赖均为最新。
- 最近每日安全通告审计、EOL 复核状态及证据：本版沿用固定 L3 的 govulncheck、npm audit、Trivy 和 Release 供应链校验；独立 EOL 复核未单独重做，未作额外“全部当前”结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增运行时依赖或基础镜像，未记录新的完整检测行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：固定 Go `1.26.6`、Node `24`、既有构建镜像和扫描器；受管 `kejilion.sh` revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Panel/Web 版本 `0.99.2`；生产脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`，clean Git blob SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`；公开 OCI index=`sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：没有新增运行时依赖候选；浏览器 Logo/可访问性专项留待后续具备真实浏览器证据的复核，退出条件为补齐品牌资源、视口、缩放、键盘/焦点、语言和失败态记录。
- 升级后的兼容、安全、构建、性能资源和回滚结论：自动门禁、公开 Release/OCI、生产更新和 postdeploy 均通过；无 Panel schema 迁移；回滚材料为 `v0.99.1` OCI、生产前备份和标准应用更新入口。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：固定 L3 使用 Go `1.26.6` / Node `24` runner；正式生产目标为 `arena-154`，容器实际运行 amd64 镜像。
- 环境策略 ID 与允许用途：`environment-policy.json` 的 `arena-154`；candidate-validation、production-safety-check 和 production-deploy 通过；`prod-108` 为 disabled，`108`/`prod-108` 未连接。
- 使用的精确候选或公开产物：候选 `8efb92d2529711500d42832f6a39591a4188e62c`；公开 `kjlion/kejilion-panel:0.99.2` 与 `latest` 均为 OCI index `sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v0.99.2-8efb92d-l3-r1` passed/0、无超时，证据目录为 `C:\GitHub\_release-artifacts\v0.99.2-8efb92d-l3-r1`；生产 `v0.99.2-8efb92d-prod-20260829` 的 preflight、backup、postdeploy 均 passed/0，证据目录分别为 `C:\GitHub\_release-artifacts\v0.99.2-8efb92d-prod-20260829-preflight`、`C:\GitHub\_release-artifacts\v0.99.2-8efb92d-prod-20260829-backup`、`C:\GitHub\_release-artifacts\v0.99.2-8efb92d-prod-20260829-postdeploy`；固定生产脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。
- 测试窗口/循环数及风险依据：候选/main CI、Release、固定 L3 和生产 preflight/backup/postdeploy 各一次通过；无独立 soak，因为 `arena-154` 是唯一批准的真实生产目标，不能改装成测试机。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动化覆盖品牌资产路径、尺寸/格式、cache bust、旧 Logo 特征清理、应用市场资源校验和构建；源任务已有本地预览/缓存验证；真实生产浏览器的视口、缩放、键盘/焦点、语言和失败态矩阵未执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 已验证应用生命周期、失败清理和回滚夹具；生产 backup 按固定流程受控停止/恢复服务，标准应用入口更新并重建 Panel；postdeploy 通过。backup 阶段出现一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时输出，但固定 gate 通过、六项备份摘要均为 `OK`、服务恢复且证据未失效。
- 未执行场景及原因：未执行真实生产浏览器 UI 验收和长期 soak；原因是本版为低风险静态品牌展示变更，自动回归、资源哈希、构建和生产健康证据已覆盖主要回归面，且当前没有额外获授权的独立浏览器/测试主机；不以生产健康证据冒充浏览器验收。

## 发布产物与公开仓库复核

- GitHub Release：[v0.99.2](https://github.com/kejilion/KPanel/releases/tag/v0.99.2)，Release workflow=`33239742728`，公开、非 draft、非 prerelease；annotated tag object=`73b3810e02f95d6193e11f256d1476e8a3847982`，`v0.99.2^{}`=`8efb92d2529711500d42832f6a39591a4188e62c`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.2` 与 `latest` 均返回 status 200、OCI index，digest=`sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:534e9f428a02e39df3a6f43a4555d58bc970f3c258dbd38051f71ddd1a33a169`；arm64=`sha256:53e598fc4c5425dd14dec518dc5cf21ccc7df68c756514c40f9fff2a695162f9`；OCI index 另含 provenance/attestation 子清单，Release workflow 校验通过。
- 附件及 `SHA256SUMS`：GitHub Release 附件由 Release workflow 生成并完成 `SHA256SUMS`、native image 和运行契约校验；本版没有改动 Agent 或节点二进制内容。
- 公开镜像 `image_e2e=pass`：Release workflow 的 native image validation、OCI digest 校验和生产标准入口实际回拉均通过；生产容器 image inspect 的 RepoDigest 精确匹配目标 digest。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场无新增应用提交；生产容器受管脚本 revision/SHA 为 `d58079304a92936bf8e3d90467eea484c5b63d6f` / `68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`，clean Git blob、语法、同步和脚本契约门禁通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求复核后走上线流程；仅执行 KPanel `v0.99.2` 标准应用更新、生产证据和备份，目标仅 `arena-154`。
- 验证/灰度环境：固定 `arena-154` L3 runner、公开 OCI 校验和生产安全证据入口，均来自 `environment-policy.json`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight `0.99.1`、health `ok`，Agent `loaded/active/running/enabled`；backup gate 通过，备份目录为 `/root/kpanel-backups/pre-v0.99.1-20260829T070650Z`，`image-load-verify.txt`、`kejilion-agent.service`、`kpanel.conf`、`kpanel.tar.zst`、`old-image.tar.zst`、`panel-inspect.json` 六项摘要均为 `OK`，受保护配置差异为空。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy health `status=ok`、`version=0.99.2`；容器标签 `org.opencontainers.image.revision=8efb92d2529711500d42832f6a39591a4188e62c`、`org.opencontainers.image.version=0.99.2`，镜像 RepoDigest 精确匹配 `sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`；Panel `running/healthy`、restart=0、OOM=false；Agent `loaded/active/running/enabled` 且 `NeedDaemonReload=no`；`protected.diff` 0 字节，SQLite quick check 为 `ai.db empty`、`panel/ai.db ok`，最近 10 分钟 Panel/Agent panic/fatal/OOM signature scan=`NONE`。本次未单独执行生产浏览器公网入口矩阵。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取目标 `latest` digest 并重建 Panel，实际 OCI digest 为目标 digest；未执行业务数据写入、schema 迁移或端口变更。
- 仅在隔离真机执行、未在生产执行的场景：L3 负例、脚本失败清理/回滚模拟和自动化品牌资源回归；真实浏览器 Logo 矩阵未执行。

## 回滚

- 源码/tag：`v0.99.1` / commit `fb3a0a1ad4f0b680ad3cb3a06c17c047bf68b837`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:89703cde4a01a510d12970c1cae4bbf8574f5e5e1657e25f59dc048e89dd46e9`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.1-20260829T070650Z`，包含旧应用目录、旧镜像、Agent unit、应用配置和校验摘要。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按应用市场原生失败回滚流程成套恢复 `v0.99.1` OCI、Compose、`.env`、数据目录和 Agent，再执行固定 preflight/postdeploy。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release 当前为 `v0.99.2`；Docker `latest` 与 `0.99.2` 同为 `sha256:bf518818feed806e6f2748be09f388b95183951e1d736cb420e0f0fef8ba3e85`；标准更新入口本次实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.99.2` 为默认更新版本；生产 postdeploy 健康、日志、数据和配置保护均通过，没有产品退化证据，因此不恢复上一稳定版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-29T14:24:42+08:00
- 候选冻结时间：2026-08-29T14:36:15+08:00
- 生产完成时间：2026-08-29T15:08:13+08:00
- 提交到生产用时：0.73 小时
- 是否回滚、紧急热修复或重复发布：否（无产品变更失败、生产回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

本次 backup 阶段另有一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时输出，但固定备份 gate 未失败、服务已恢复、六项摘要均为 `OK`、证据未失效，因此不计入流程异常指标。

## 遗留风险与后续准入

- 未验证风险：真实生产浏览器中 Logo、favicon/PWA 图标刷新、不同视口、100%/125%/200% 缩放、键盘/焦点、最小字号、语言和失败态矩阵，以及长期缓存/性能采样尚未执行。
- 已实现待实机准入：品牌资源已替换并通过路径、格式、尺寸、哈希、旧特征清理、自动回归和生产产物校验；需在具备真实浏览器证据的专项验收中补齐 UI 矩阵。
- 不阻断本版的理由：本版是低风险只读品牌展示变更，无 API、数据、宿主机权限、端口或部署脚本变化；候选/main CI、固定 L3、公开 Release/OCI、生产备份、标准更新和 postdeploy 均通过，未以生产健康证据冒充浏览器验收。
- 后续应进入的自动门禁或专项工作流：在下一次 UI 专项验收中补充生产浏览器的 Logo/cache-bust、PWA 图标、视口、缩放、键盘/焦点、语言和失败态证据，并评估将品牌资源路径与图标清晰度纳入固定回归。
