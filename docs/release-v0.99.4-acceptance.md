# KPanel v0.99.4 发布验收记录

日期：2026-08-30

发布级别：L3

候选提交 / 标签：`a4c163ffd432ba012b61a4d57b95d82532554445` / `v0.99.4`

上一稳定版本 / 回滚点：`v0.99.3` / commit `0cf8a57259297e844a2f2077eeb9cdc102c79e87`；OCI index `sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`

## 发布画像

- 业务域：KPanel 网站反向代理创建表单、进程管理列表和全局键盘聚焦视觉反馈。
- 变更面：展示、只读宿主信息选择；不新增宿主机写入、数据迁移、协议、权限或应用市场契约。
- 受影响用户旅程：在网站页创建/编辑 IP / 端口反代时扫描并选择本机 TCP 监听服务；在进程管理中按 CPU/内存峰值快速识别高占用进程；使用键盘操作表单时获得紧凑一致且不重复的聚焦框。
- 未变化契约：复用现有只读端口占用 API；不改变 API schema、数据库、端口协议、Compose、Agent 权限、`kejilion.sh` 或 `kejilion/apps` 安装更新契约。
- 风险等级及理由：低到中；候选包含前端交互和可视化，选择器仍由既有 API 提供事实，生产写入只通过标准应用市场入口完成；真实生产浏览器完整矩阵未执行。

## 发布范围与未纳入内容

- 用户可见更新：
  - 统一网站、Docker、文件、终端、集群、应用和日志相关表单的键盘聚焦框为 2px 紧凑样式，避免重复描边。
  - 网站反向代理创建/编辑增加本机 TCP 监听端口扫描与候选选择，默认填入 HTTP 上游地址，仍可手动修正。
  - 进程管理 CPU 和内存单元格按本次列表峰值归一化显示热力背景，保留数值、选中态、主题和 reduced-motion 兼容性。
- 精确提交清单：候选基线为 `origin/main` `654dccde28c0d4e0ffa16c247e2bfdf79d913c50`；候选相对基线依次为 `17fb09fb1e58790ce526ca24cb4003083e635c6e`（focus rings）、`d7abbb95a443c2e159ef7bd0aa951a1a68cd6bff`（local web service picker）、`f4edd219211eee6c2d88f27c71ba2d29ecbf3fe8`（Docker owner enhancement 暂纳入）、`b3d6242dced87772ce84d6dd2857eaf68691a167`（process heatmap）、`fc698fe1ec1ca70f7ed5f9ea97163e6c9a6cdb11`（完整撤销 Docker owner enhancement）和 `a4c163ffd432ba012b61a4d57b95d82532554445`（版本准备）。最终相对基线为 28 个文件、939 行新增、25 行删除；`f4edd219` 与 `fc698fe1` 的净差异为零。
- 明确未纳入的分支、文件或后续事项：System Center 页面、路由、生产 API、文档和 Docker 端口归属增强均排除；`internal/`、`web/src/types/api.ts`、`web/src/lib/api.ts` 等系统中心实现文件不在最终差异中。`kejilion/apps` 无提交，`kejilion.sh` 未改动。`108`/`prod-108` 未连接、未核对。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 定向测试、前端全量 126 个文件/1054 项、typecheck、build、固定 L3、候选/main CI 通过；隔离 mock 浏览器实测扫描候选、填入上游和进程热力背景 | 选择器使用 mock 端口数据，未在生产浏览器直接执行真实创建/更新流程；本版无 Agent/节点协议变化 |
| 网络入侵与供应链安全 | 已验证 | 固定 L3 的 govulncheck、npm audit、Trivy、源码/依赖/密钥配置扫描、镜像运行契约和 SBOM/provenance 均通过；无新增生产网络入口 | 未另行执行公网渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | 前端回归、L3 生命周期/失败清理、Release、公开镜像 E2E、生产 preflight/backup/postdeploy 均通过；无 schema 迁移 | 未做长期 soak；浏览器组合仍有未覆盖矩阵 |
| 性能与资源预算 | 已实现未实机验证 | 热力图只增加单次列表计算和 CSS 背景，不增加轮询或后端负载；生产容器 healthy、restart=0、OOM=false | 未做浏览器长期性能曲线或不同设备资源采样 |
| 用户体验与可访问性 | 已实现未实机验证 | `390x844` 隔离浏览器完成网站选择器、进程热力图组合和键盘 Tab 聚焦检查；聚焦框实测为单一 2px box-shadow；保留回归测试 | 未执行生产浏览器的 768/1280、缩放、明暗主题、最小字号、多语言、触控和完整失败态矩阵 |
| 数据、配置与迁移 | 已验证 | 本版无数据/schema/配置迁移；生产 `protected.diff` 为 0 字节，SQLite quick check 为 `ai.db empty`、`panel/ai.db ok`，备份校验通过 | 不适用额外迁移验收 |

## 自动门禁

- 定向测试及结果：`LocalWebServicePicker`、`localWebServices`、`ProcessManagerView`、form focus 和相关页面回归通过；前端全量 `126` 个测试文件、`1054` 项通过；`npm run typecheck`、`npm run build` 和版本一致性通过；build 报告 `2594` localized phrases / `21` lazy catalogs。Windows `verify-change` 在本机因缺少 `docker`、`go`、`gofmt`、`make` 按门禁 fail-closed，这是执行环境限制，不是候选产品失败；固定 Linux L3 已完成权威门禁。
- `make verify-release` 环境和结果：固定 Runner `kpanel-release-gate:go1.26.6-node24`；L3 run `v0.99.4-a4c163-l3-r1`，`release_l3_gate=pass`、`release_l3_remote=pass`，目标仅 `arena-154`。不可变 Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；证据目录为 `C:\GitHub\_release-artifacts\v0.99.4-a4c163-l3-r1`；bundle SHA-256=`0fbfe408e39014b150d4fb4a81975a734a137c89d9d0524e2ca39e891000aeb9`，manifest SHA-256=`232679b179e9cec6031bf91d42b62b1d2534d56a5d16ef2261ac222f4195adb`，plan SHA-256=`4b5434308766759526a3fdc6e4c03d648b9ecf6a97bf5d6cdea67ac71a6e8946`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。Go 全包测试、amd64/arm64 构建、镜像构建与契约、应用配置生命周期均通过。
- 候选 CI：CI run [33311597917](https://github.com/kejilion/KPanel/actions/runs/33311597917) 和 Dependency freshness run [33311597918](https://github.com/kejilion/KPanel/actions/runs/33311597918)，均绑定 `release/v0.99.4-candidate` 与精确 SHA `a4c163ffd432ba012b61a4d57b95d82532554445`，completed/success。
- 主线 CI：主线在 SHA guard 确认 `origin/main` 仍为 `654dccde28c0d4e0ffa16c247e2bfdf79d913c50` 后 fast-forward 到产品 SHA；CI run [33312007280](https://github.com/kejilion/KPanel/actions/runs/33312007280) 和 Dependency freshness run [33312007276](https://github.com/kejilion/KPanel/actions/runs/33312007276) 均绑定产品 SHA，completed/success。
- Release workflow：run [33312224014](https://github.com/kejilion/KPanel/actions/runs/33312224014)，`v0.99.4^{}` 精确解引用到 `a4c163ffd432ba012b61a4d57b95d82532554445`，completed/success；源码验证、漏洞扫描、双架构构建、native image、draft Release、镜像推送、latest promotion、Release 公开和候选分支清理全部 success。
- 安全扫描、镜像契约、SBOM/provenance：固定 L3/Release 的 Go 漏洞调用路径扫描、npm audit、Trivy（go.mod、package-lock、Dockerfile、paneld、Agent）、运行时资源/权限契约及 provenance 均通过；受管 `kejilion.sh` revision/SHA 契约通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本版未新增运行时依赖、基础镜像或 Action；候选和主线 Dependency freshness 均成功；固定 L3 的依赖、漏洞和镜像扫描通过。
- 最近每日安全通告审计、EOL 复核状态及证据：本版沿用固定 L3、候选 CI、主线 CI 和 Release 的扫描链路；未单独作“所有上游依赖当前”的额外结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：未新增依赖行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go `1.26.6`、Node `24`、`kpanel-release-gate:go1.26.6-node24`、既有构建/扫描工具链；未改变受管脚本。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：KPanel/Web `0.99.4`；生产脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`，clean Git blob SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`；公开 OCI index=`sha256:ff04275f9966cf2fca940df07a68dd6cc4a86284450a26eced1986279402767a`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：Docker owner port-picker enhancement 因 System Center 排除规则撤回，候选 `f4edd219` 由 `fc698fe1` 完整 revert；后续如解除范围，需独立候选、审查和 L3。
- 升级后的兼容、安全、构建、性能资源和回滚结论：自动门禁、公开 Release/OCI、公开镜像 E2E、生产标准更新和 postdeploy 均通过；无 Panel schema 迁移；可用 `v0.99.3`、旧 OCI 和本次生产备份回滚。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：本地隔离预览为 Windows + Codex In-app Browser，固定 `390x844` 移动视口；公开镜像 E2E 在 `arena-154` 的 Linux Docker 环境执行；生产容器为 `linux/amd64`。
- 环境策略 ID 与允许用途：`arena-154` 的 `candidate-validation`、`production-safety-check`、`production-deploy` 均通过 `environment-policy.mjs`；`prod-108`/`108` 永久禁用，本次未连接。
- 使用的精确候选或公开产物：候选 `a4c163ffd432ba012b61a4d57b95d82532554445`；浏览器预览 manifest=`C:\GitHub\_release-artifacts\v0.99.4-a4c163-ui-preview-20260830\manifest.json`，preview ID=`release-ui-bundle-1788093018266-4f548a`，`mock-ui`；公开镜像为 `docker.io/kjlion/kejilion-panel:0.99.4`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：固定 L3 `v0.99.4-a4c163-l3-r1` passed/0；生产 run `v0.99.4-production-20260830-01` 的 preflight、backup、postdeploy 均 passed/0，证据目录分别为 `C:\GitHub\_release-artifacts\v0.99.4-production-preflight-20260830-01`、`C:\GitHub\_release-artifacts\v0.99.4-production-backup-20260830-01`、`C:\GitHub\_release-artifacts\v0.99.4-production-postdeploy-20260830-01`；固定生产远端脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。
- 测试窗口/循环数及风险依据：候选/main CI、Release、L3、公开 image E2E 和生产三个固定证据阶段各一次通过；无独立 soak，因为 `arena-154` 是唯一批准的真实环境。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：`390x844` mock UI 组合验收覆盖网站创建表单、候选端口扫描、选择 3000 填入 `http://127.0.0.1:3000`、进程表 CPU/内存渐变和选中态、键盘 Tab；无重复聚焦框。未执行 768/1280、100%/125%/200% 缩放、明暗主题、多语言、最小字号、触控、长文本和完整加载/空/失败/部分状态矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：公开 `image-e2e.sh` 在 `arena-154` 的临时容器上 `image_e2e=pass` 并清理；生产 backup 通过固定入口受控停止/恢复服务，标准应用市场更新入口成功，postdeploy 通过。backup 输出曾出现一次 `curl: (56) Recv failure: Connection reset by peer` 瞬时信息，但 gate 未失败、未重试、服务和摘要校验均通过，不计产品失败或流程异常。
- 未执行场景及原因：未执行生产浏览器公网 UI 矩阵、生产进程信号操作、长时 soak 和回滚实操；原因是本版不改变后端/数据/权限，破坏性操作不应为形式验收主动改写生产业务数据；不以生产健康证据替代 UI 矩阵。

## 发布产物与公开仓库复核

- GitHub Release：[v0.99.4](https://github.com/kejilion/KPanel/releases/tag/v0.99.4)，Release workflow=`33312224014`，公开、非 draft、非 prerelease；annotated tag object=`5dbff83f637eefd707913497c5c7e9bdc8a24443`，peeled commit=`a4c163ffd432ba012b61a4d57b95d82532554445`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.4` 和 `latest` 均为 OCI index，digest=`sha256:ff04275f9966cf2fca940df07a68dd6cc4a86284450a26eced1986279402767a`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:8014871ba0d734272676c2ef8d8eb9b8a8ee380fdb4c3c2b992190ff0d58daea`；arm64=`sha256:9e6cbe4c5410d5b17862d4e72f55cd6ee53b31aadff37d9b0256fc9af575ac38`；另有 2 个 provenance/attestation unknown 子清单，不影响双架构。
- 附件及 `SHA256SUMS`：Release 包含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-0.99.4.tar.gz`、`SHA256SUMS`、LICENSE 和 `THIRD_PARTY_NOTICES.md`；Release workflow 附件摘要、native image 和运行时契约校验通过。
- 公开镜像 `image_e2e=pass`：在 `arena-154` 显式 pull 公开 `docker.io/kjlion/kejilion-panel:0.99.4`，按仓库 `packaging/tests/image-e2e.sh` 在只读根、`cap-drop ALL`、无特权和动态 Docker 网络条件执行，结果 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 与 `kejilion/apps` `origin/main:kpanel.conf` Git blob 均为 `abf0efd22876f34aa3731f5b6d8ba04e373b965e`，无需 apps 提交且 apps 工作树干净；受管脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f` / SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求将“聚焦框优化、本地 Web 服务选择器、进程管理热力图”走上线流程；正式写入仅执行本次 `v0.99.4` 标准应用更新和固定证据入口，目标仅 `arena-154`。
- 验证/灰度环境：固定 Linux L3、公开镜像 E2E 和生产安全证据均使用 `arena-154`，用途策略通过。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight `status=passed`，Panel `0.99.3`、health `ok`，Agent `loaded/active/running/enabled`、容器 healthy、restart=0、OOM=false；backup `status=passed`，备份目录为 `/root/kpanel-backups/pre-v0.99.4-20260830T125458Z`，备份校验摘要在 `C:\GitHub\_release-artifacts\v0.99.4-production-backup-20260830-01\remote-evidence\backup-SHA256SUMS`。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；实际 exit 0，返回 `KPanel 更新完成 / Update Complete`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy `status=passed`；health `status=ok`、version=`0.99.4`；容器 image=`sha256:ff04275f9966cf2fca940df07a68dd6cc4a86284450a26eced1986279402767a`，OCI revision=`a4c163ffd432ba012b61a4d57b95d82532554445`，`running/healthy`、restart=0、OOM=false；Agent `loaded/active/running/enabled`、`NeedDaemonReload=no`，日志显示 version=`0.99.4`；`protected.diff` 0 字节，SQLite quick check 为 `ai.db empty`、`panel/ai.db ok`，固定 postdeploy 的近端致命日志检查通过。preflight/backup 后磁盘使用率约 `98%`、可用约 `2.6G`，这是当前遗留风险。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取 `latest` 的目标 digest 并重建 Panel；未执行业务数据写入、schema 迁移、端口变更或进程终止操作。
- 仅在隔离真机执行、未在生产执行的场景：L3 失败清理/生命周期夹具、公开镜像临时容器 E2E、mock 浏览器交互和前端自动回归；生产浏览器 UI 完整矩阵、进程 signal 和长期 soak 未执行。

## 回滚

- 源码/tag：`v0.99.3` / commit `0cf8a57259297e844a2f2077eeb9cdc102c79e87`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:d76ce9d40b2017d80e9723509704c5dfcb25e13b6a19f6830b8a0a373020bafb`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.4-20260830T125458Z`，包含旧镜像、KPanel 应用目录、Agent unit、应用配置、panel inspect 和 SHA256 校验。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按 `kejilion.sh` 应用更新的原生回滚能力成套恢复 `v0.99.3` OCI、Compose、`.env`、数据和 Agent，再运行固定 preflight/postdeploy。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：当前 GitHub Release 为 `v0.99.4`；Docker `0.99.4` 与 `latest` 同为 `sha256:ff04275f9966cf2fca940df07a68dd6cc4a86284450a26eced1986279402767a`；标准更新入口本次已实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.99.4` 为默认更新版本；生产 postdeploy、日志、数据、配置保护均通过，没有产品退化证据，因此未回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-30T20:08:03+08:00
- 候选冻结时间：2026-08-30T20:16:00+08:00
- 生产完成时间：2026-08-30T20:56:15+08:00
- 提交到生产用时：0.80 小时
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

backup 阶段的 `curl: (56) Recv failure: Connection reset by peer` 是受控停止/恢复期间的瞬时输出，固定 gate 未失败、未重试，服务和校验摘要均恢复通过，因此不计入使必需步骤失效的流程异常。Windows 本地工具缺失也未使发布步骤失效，已由固定 Linux L3 完成权威验证。

## 遗留风险与后续准入

- 未验证风险：生产浏览器中网站页创建/编辑真实业务提交、390/768/1280 全视口、100%/125%/200% 缩放、明暗主题、最小字号、键盘/焦点/触控、多语言、长文本、加载/空/失败/部分状态和长期 soak 尚未执行；`arena-154` 根分区约 98% 使用率。
- 已实现待实机准入：本地 Web 服务选择器和进程热力图的真实生产浏览器旅程仍待专项验收；本版只记录了隔离 mock UI 的组合证据，不把 mock 数据写成真实主机事实。
- 不阻断本版的理由：三项改动聚合为一个 patch 版本；最终差异无 System Center 生产功能、无 API/数据/权限/端口/应用市场契约变化；候选/main CI、固定 L3、公开 Release/双架构 OCI、公开 image E2E、生产备份、标准更新和 postdeploy 均通过。
- 后续应进入的自动门禁或专项工作流：将真实端口占用 schema 与选择器提交路径、进程表长列表/主题/缩放/键盘矩阵纳入登记的浏览器验收；在生产写入前处理磁盘空间风险并重新通过 preflight。
