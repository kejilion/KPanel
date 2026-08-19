# KPanel v0.81.1 上线验收记录

日期：2026-08-17（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`e2416ae393e0e23323c0d3088b996bc8d89273d5` / `v0.81.1`

上一稳定版本 / 回滚点：`v0.81.0` / `sha256:541f9b2d3b4b6e17c925e6c663da162e1bc42db855f7a253e95c8b5eba82100d`

结论：通过，生产已部署，未回滚。

## 发布画像

- 业务域：应用市场全量列表排序与四个多人游戏图标。
- 变更面：展示；无宿主机写入、协议、数据、权限或部署契约变化。
- 受影响用户旅程：打开应用市场全量列表时先看到所有已安装应用；已安装/未安装分组内继续看到 60 个 UTC 自然日新品倒序；四个游戏显示各自的本地图标。
- 未变化契约：API、数据格式、端口、Compose、Agent 权限、`kejilion.sh`、应用安装/更新入口和 `packaging/kejilion-app/kpanel.conf` 均未变化。
- 风险等级及理由：发布、镜像和生产部署属于 L3；功能差异本身是前端排序和静态资源，不新增输入、网络或主机权限面。

## 发布范围与未纳入内容

- 用户可见更新：全量列表已安装应用始终优先；两个安装状态组内继续按 60 天新品规则排序；更新 Arena Brawl、Bomb Party、Ice Climber Arena、Neon Arena 四个 128×128 WebP 图标。
- 精确提交清单：开发候选 `624fcad97cc67ab13969edd6eef0cb8e6d494b4d`；集成重放 `52dc2866f9d9fd3dcf976e764887f1548babda04`；版本提交 `e2416ae393e0e23323c0d3088b996bc8d89273d5`。
- 明确未纳入：其他远端分支、依赖升级、应用安装契约、数据库/配置迁移和任何 `prod-108` 操作。
- 候选相对基线 `86552a5931760bfcb13e4a2d7fed19dae75916a5` 仅含上述两个提交；开发候选恰好比基线多一个提交，merge-base 与基线一致，无冲突或夹带。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 生产 Agent 真实库存 153 项、已安装 5 项；生产数据排序断言确认已安装项形成完整前缀，两个状态组内 60 天新品倒序；四图标线上摘要与 catalog 一致 | 本版不改变安装、更新、卸载或 `kejilion.sh` selector，双端写入闭环不适用 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck` 可达漏洞 0、npm audit 0；Trivy source/image HIGH/CRITICAL、secret、misconfiguration 均为 0；镜像、Action、基础层和托管脚本继续固定摘要 | Trivy 0.72.0 提示上游已有 0.74.0；扫描器升级属于独立依赖任务，不改变本次扫描结论 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心包 race、全量 Go/Web、安装安全、应用生命周期、标准更新自动回滚路径、停写备份恢复校验、部署后 5 次健康采样通过 | 无配置或 Schema 兼容窗口变化 |
| 性能与资源预算 | 已验证 | 生产 5 次采样 CPU 0.01%–0.05%、内存 11.21 MiB / 256 MiB、8 PIDs；排序仍为单次内存排序，无新增轮询或常驻任务 | 无性能回归信号 |
| 用户体验与可访问性 | 已实现未实机验证 | 前端 98 文件/741 测试、AppsView 30 项、typecheck、i18n 2255 条、生产 build 通过；回环登录页真实浏览器控制台 error/warn 为 0 | 浏览器没有生产管理员登录态，未对已认证 Apps 页面做视觉截图；生产真实库存、排序算法与 HTTP 图标资源已独立验证 |
| 数据、配置与迁移 | 不适用 | `kpanel.conf` 与 v0.81.0 blob 无差异；两份生产 SQLite 部署前备份及部署后 `integrity_check=ok` | 本版无数据、配置或迁移变更 |

## 自动门禁

- 定向复核：`git diff --check`、版本一致性、AppsView 60/61 天边界、未来/非法日期、四图标 128×128 与 SHA256 全部通过。
- Linux L3：在 `arena-154` 从完整 Git bundle detached 到 `e2416ae393e0e23323c0d3088b996bc8d89273d5`，Runner `kpanel-release-gate:go1.26.6-node24` 固定 image ID `sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；输出 `L3 release verification completed` 和 `release_gate_runner=pass`。
- L3 日志 SHA256：`244bd999ee8240b57d25ddbb1916f52c0f942854e26b7d8eb4d10d15ff38c52a`；bundle SHA256：`aef4f9053b57a9a743b9097fcd35ca63364548c86dede8e5732c74238b2b3199`。
- 全量结果：Go `go test ./...`、核心 `panel/auth/dockerx` race、`go vet ./...`、linux/amd64 与 linux/arm64 的 paneld/Agent/node/kpctl、前端 98 文件/741 测试、typecheck、i18n、生产 build、部署与治理检查全部通过。
- 候选 CI：[`31989333612`](https://github.com/kejilion/KPanel/actions/runs/31989333612) success；候选依赖治理 [`31989333609`](https://github.com/kejilion/KPanel/actions/runs/31989333609) success。
- 主线 CI：[`31989578059`](https://github.com/kejilion/KPanel/actions/runs/31989578059) success；主线依赖治理 [`31989578070`](https://github.com/kejilion/KPanel/actions/runs/31989578070) success。
- Release workflow：[`31989838677`](https://github.com/kejilion/KPanel/actions/runs/31989838677) success；tag 依赖治理 [`31989838702`](https://github.com/kejilion/KPanel/actions/runs/31989838702) success。
- 安全扫描、镜像契约、SBOM/provenance：Release 原生镜像运行 contract、Trivy 最终镜像扫描、双架构 OCI index 与两个 attestation manifest 均通过。

## 依赖与技术栈变化

- 本版没有依赖、工具链、基础镜像、Action、扫描器或受管脚本版本变化。
- 候选、主线和 tag 三层 `Dependency freshness` 均成功；当前锁文件和固定 Action SHA 可重建。
- `govulncheck` 报告代码可达漏洞 0；npm audit 报告 0；Trivy 漏洞数据库在 L3 时重新下载并完成扫描。
- Trivy 当前 0.72.0、上游提示 0.74.0；是否升级继续由依赖维护流程独立评估，本次不夹带工具链变化。

## 隔离真机与浏览器验收

- 主机：`arena-154`，Linux amd64，Docker Engine 29.6.2；环境策略允许 `candidate-validation`、`browser-validation`、`production-deploy` 与 `production-safety-check`。
- 精确候选：`e2416ae393e0e23323c0d3088b996bc8d89273d5`；公开产物：`docker.io/kjlion/kejilion-panel:0.81.1`。
- L3 使用标准 Runner 前台完成，无长时间浏览器/soak 风险，因此未创建后台浏览器 job；普通排序交互由确定性单测、生产库存断言和资源 HTTP 校验覆盖。
- 真实浏览器通过本机回环 SSH 隧道加载生产登录页，控制台 error/warn 为 0；因没有管理员登录态，未提交登录表单、未修改账户或生产业务数据，隧道与临时标签页已关闭。
- 生产 Agent Unix Socket 只读库存证据：`total=153 installed=5 running=5`；已安装前缀 `arena-brawl,bomb-party,cloudreve,n8n,kpanel`；已安装新品顺序 `arena-brawl,bomb-party`；未安装新品顺序 `deepseek-harness,airadio,ice-climber-arena,neon-arena-fps`。
- 生产应用市场证据日志 SHA256：`6d97d8689e317b8b27e0c41ae801570b13974d307194980138673dbddf226c92`；库存 JSON SHA256：`087c445b9e68c290946a2c2139c42012b117da8b65fe2bbc357ce70e76e43079`。

## 发布产物与公开仓库复核

- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.81.1>，非 draft、非 prerelease，8 个附件公开；Release 正文包含更新、升级、迁移、完整性、测试和回滚提示。
- source commit：`e2416ae393e0e23323c0d3088b996bc8d89273d5`；annotated tag object：`39777dfcf33b68b1b6d281f47bb42fa4aa9522f9`。
- Docker `0.81.1` 与 `latest` OCI index：`sha256:83372a7c772e67ddf3682c7b28d2ca1763ed48284fe50e187d1541c81b3f1ddd`。
- `linux/amd64`：`sha256:604a0e84f60a91de3935980aeb0a7fd409db446cdbb30a2783cdf9d7255b5e11`；`linux/arm64`：`sha256:5afcb5ca46043dbc5b32f49c6b59777486b4bfce0d38bfe4e33201c9665a7573`。
- Release 附件包括双架构 Agent、双架构 lightweight node、`kejilion-panel-deploy-0.81.1.tar.gz`、`SHA256SUMS`、LICENSE 和 notices。
- 公开镜像显式重新拉取并运行 `packaging/tests/image-e2e.sh`，输出 `image_e2e=pass`。
- `packaging/kejilion-app/kpanel.conf` 相对 v0.81.0 无差异；KPanel blob 与 `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 同为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，无需制造空提交。
- 四个生产 HTTP 图标均为 128×128 WebP：Arena Brawl `ccba8f5c…`、Bomb Party `248767ad…`、Ice Climber Arena `aac98d7d…`、Neon Arena `31350f89…`，均与内置 catalog SHA256 完全一致。

## 生产部署安全核对

- 生产目标和授权：仅 `arena-154`；用户明确授权发布、生产部署与验收。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前：v0.81.0，OCI `sha256:541f9b2d3b4b6e17c925e6c663da162e1bc42db855f7a253e95c8b5eba82100d`，revision `21261e35dd2aa8bc3fea4bed004c8b5ba896c975`；Panel healthy、restart 0、OOM false，Agent active。
- 停写备份：`/root/kpanel-backups/pre-v0.81.1-20260817T031140Z`；完整应用目录归档 SHA256 `ef877803492753564fb476a8dab9a2c1966d022671de85b979f3f4d50dfb5ef1`，旧镜像归档 SHA256 `b95495cffce4ec046f180b338e9ae8c4b3d81af5aaf03626966e7d5b4237a0be`。
- 备份已解包并逐文件对比；原始/恢复副本的 `data/ai.db` 与 `data/panel/ai.db` 均 `integrity_check=ok`；备份日志 SHA256 `8e95c55657367f0e91e3f9c76beaea02042b6bb067eef56ed5303c6b71c1acf2`。
- 部署入口：加载发布提交内、与 apps 真源同 blob 的 `packaging/kejilion-app/kpanel.conf`，调用标准 `docker_app_update` 事务；版本/`latest` digest 在事务前固定为正式摘要。
- 首次执行器在加载契约时因 Bash `local` 作用域保护 fail-closed，发生在镜像拉取、停服务或文件修改之前；生产保持 v0.81.0 healthy。把 source 放入函数作用域后，同一标准事务一次升级成功；这不是产品失败或生产回滚。
- 部署后：v0.81.1，revision `e2416ae393e0e23323c0d3088b996bc8d89273d5`，运行 OCI 与正式摘要一致；Panel healthy、restart 0、OOM false，Agent active，`/v1/health status=ok version=0.81.1`。
- 部署日志 SHA256：`c9a251f8d1004e132f4ad2df2869dc9368b1b159dbe8a8156a00da822ad88154`；托管脚本升级前后 SHA256 均为 `d73231f146f7398d7b50133695faf2116134fbfe33a7b94068e277cc7b82df55`。
- 生产已执行写操作：停写备份、标准 KPanel 更新事务和服务重建；没有安装/卸载其他应用，没有修改账户、目录条目或业务配置。

## 回滚

- 源码/tag：`21261e35dd2aa8bc3fea4bed004c8b5ba896c975` / `v0.81.0`。
- 镜像 digest：`sha256:541f9b2d3b4b6e17c925e6c663da162e1bc42db855f7a253e95c8b5eba82100d`；旧镜像已独立归档。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.81.1-20260817T031140Z`，包含完整 `/home/docker/kpanel`、有效 Compose/Agent unit 记录、容器 inspect、SQLite 数据和旧镜像归档。
- 回滚步骤：停止 Panel 与 Agent，加载 `previous-image.tar`，成套恢复 `kpanel.tar`，恢复 v0.81.0 的 `latest` 标签与 Agent unit，`systemctl daemon-reload` 后启动 Agent/Compose，再核对版本、SQLite、健康、restart/OOM、托管脚本与日志；禁止只切换浮动 `latest`。
- 本次未触发正式回滚；生产实际版本为 v0.81.1，健康状态正常。
- GitHub Latest、Docker `latest` 与标准更新入口实际均指向 v0.81.1 / `sha256:83372a7c772e67ddf3682c7b28d2ca1763ed48284fe50e187d1541c81b3f1ddd`。
- 公共默认更新通道决策：不适用；生产部署安全核对成功，无需恢复上一稳定默认版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-17T10:37:16+08:00
- 候选冻结时间：2026-08-17T10:42:22+08:00
- 生产完成时间：2026-08-17T11:19:00+08:00
- 提交到生产用时：0.70 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与后续准入

- 未验证风险：缺少已认证生产 Apps 页面的视觉截图；真实生产库存、排序断言、图标 HTTP 资源、控制台登录页、前端自动测试和精确运行镜像均已验证。
- 不阻断本版的理由：本版差异仅为排序比较器的两行顺序调整和四个固定摘要静态资源；生产使用同一正式 OCI digest，真实库存排序和资源摘要已直接通过。
- 本次复用 `release-kpanel` v2.4、`project-version-control` 与既有应用市场目录/图标摘要门禁；没有发现需要新增第二套工作流或修改永久规范的长期缺口。
