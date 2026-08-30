# KPanel v0.99.5 发布验收记录

日期：2026-08-31

发布级别：L3

候选提交 / 标签：`e0cc4c896a33f6dc0ed5740733eea42e30a3213c` / `v0.99.5`

上一稳定版本 / 回滚点：`v0.99.4` / `a4c163ffd432ba012b61a4d57b95d82532554445`

## 发布画像

- 业务域：AI 助手与应用脚本多语言运行时、本地 Web 服务选择器、系统中心端口占用只读展示。
- 变更面：展示 / 只读 / 可选 API 响应字段；不新增宿主机写操作、数据库迁移或权限边界。
- 受影响用户旅程：切换三语页面、扫描本地 Web 服务并选择上游、查看监听端口对应的 Docker 容器归属。
- 未变化契约：数据库、端口协议、Compose、Agent 权限、受管 `kejilion.sh` 和应用市场安装契约均未变化；`packaging/kejilion-app/kpanel.conf` 相对 `v0.99.4` 无差异。
- 风险等级及理由：中低风险。跨 Web/Agent/contract 的只读增强；Docker 查询失败、host network 或归属不明确时保留原监听结果，新增字段均为可选。

## 发布范围与未纳入内容

- 用户可见更新：
  1. AI 助手与应用脚本三级页面补齐 `en-US`、`zh-CN`、`zh-TW` 运行时文案，状态、错误、确认提示、设置操作以及数字/日期格式跟随当前 locale。
  2. 本地 Web 服务选择器优先推荐 Docker 映射端口，显示容器信息，并标记已有本地反代端口；仍可手动修改上游地址。
  3. 端口占用查看为 Docker 发布端口补充容器名、镜像、容器端口及 Compose 项目/服务归属。
- 精确提交清单：基线 `73b46561ed9db9e6673d590f1f049d9ab5997c32`；纳入 `32dc760447f97aabfe5fcbbe7afc2f57873cc4e1`（来源 `f8fa1ebcfe27121cb45ccf745aa1fcdfb3ee874e`）、`f784daf`（重新应用来源 `6a382d7dd63a42273ea7538759397a2760e4b00d`）、`ccbb656`（来源 `11e2f367fec1998cb19db7c0321d5f7cb3f31740`）、`4fabbf5`（来源 `22451695fe415cc4490f73646f2547fd77d5e27b`）、版本准备 `5f4287b01cf88893ca8a0b935e919c9266ab39e8` 和测试稳定性修复 `e0cc4c896a33f6dc0ed5740733eea42e30a3213c`。候选相对基线为 38 个文件、981 行新增、99 行删除。
- 明确未纳入的分支、文件或后续事项：没有复用旧 `v0.99.3`/`v0.99.4` 的成功证据；没有连接、备份、部署或核对 `108`/`prod-108`。默认“系统中心不发布”范围经本次用户明确授权解除，仅纳入端口占用 Docker 归属所需的 `PortUsageDialog`、network contract/Agent 只读查询及对应测试/fixture；没有纳入其他系统中心页面、路由、维护写操作或无关业务。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 在 `arena-154` 固定 Runner 通过；Agent/Panel contract、Docker 归属回归测试及公开镜像 `image_e2e=pass`。 | 无新增持久数据协议；Docker 容器归属在并发/歧义场景按失败关闭处理。 |
| 网络入侵与供应链安全 | 已验证 | L3 的 `govulncheck`、`npm audit`、Trivy 源码/依赖/镜像/二进制/配置扫描均通过，无高危漏洞和 secrets findings。 | 依赖 freshness 同 SHA 通过。 |
| 稳定性、失败恢复与兼容 | 已验证 | Go 全量测试、核心 race、前端 126/126 文件与 1061/1061 测试、应用配置生命周期、备份归档校验和 postdeploy 通过。 | 生产磁盘可用空间约 1.8G（99% 使用率），本次门禁通过但应持续关注。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 资源快照：Panel 约 72.87MiB/256MiB、7 PIDs、重启 0、OOM false；Docker enrichment 有超时和保留原结果的边界。 | 未执行长期 soak；本版为只读查询增强。 |
| 用户体验与可访问性 | 已验证 | mock UI acceptance preview 覆盖三语切换、端口 Docker 归属、选择器排序/已反代标记及 390x844 无横向溢出；console error/warn 为空。 | Preview 运行在 `5f4287b`，后续 `e0cc4c8` 仅修改 ICU 稳定性测试断言，未改变 UI 代码；模拟 API 不证明真实宿主机行为。 |
| 数据、配置与迁移 | 已验证 | `kpanel.conf` 无差异；生产 preflight/backup/postdeploy 的 protected 配置摘要 diff 均为 0，SQLite quick check 通过。 | 不适用数据库迁移。 |

## 自动门禁

- 定向测试及结果：`npm run i18n:check` 通过（2547 phrases / 21 catalogs）；Web 定向测试通过；完整前端 126/126 文件、1061/1061 测试通过；typecheck、build、Go 全量、race、vet、双架构构建通过。
- `make verify-release` 环境和结果：`arena-154` 固定 Runner `kpanel-release-gate:go1.26.6-node24`，不可变 Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.99.5-e0cc4c8-l3-r2`，`release_gate_runner=pass`、`release_l3_gate=pass`、`release_l3_remote=pass`，目标仅 `arena-154`。本地证据目录为 `C:\GitHub\_release-artifacts\v0.99.5-e0cc4c8-l3-r2`；bundle SHA-256=`dc0d583b8c59090b0ad6a1725e71798d465ab630313f204a81a9ee47eab72eb7`，manifest SHA-256=`2e8f57247713890c54493824305de7ef53e022ac2538ba5a02cc2b4289a27876`，plan SHA-256=`3c3ea7e49c3180dfc9230accfff63e984f6198b83ad2af1e03537a2f5f60457e`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，远端 `l3-verify-release.log` SHA-256=`95149e3d8c8f327570c25c00c218127285f4afa90606c88a05dc8ff0b5b91cf9`。
- 候选 CI：`CI #33322717176` 成功，`Dependency freshness #33322717169` 成功，均为候选 SHA `e0cc4c8`。
- 主线 CI：产品 SHA `e0cc4c8` 快进到 `origin/main` 后，`CI #33322913794` 成功，`Dependency freshness #33322913783` 成功。
- Release workflow：`Release #33323199646` 成功；GitHub Release 已公开，候选分支已按流程删除。
- 安全扫描、镜像契约、SBOM/provenance：Release 源码/依赖/镜像扫描、非 root/healthcheck/label/脚本摘要契约、SBOM/provenance 及双架构构建均成功。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选 CI 与主线 CI 的 Dependency freshness 均为同 SHA 成功；未引入依赖版本升级。
- 最近每日安全通告审计、EOL 复核状态及证据：Release 的 `govulncheck`、`npm audit --audit-level=high`、Trivy 扫描通过；无新增 EOL 行动项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版无新增依赖候选，freshness 通过。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：沿用仓库固定 Go 1.26.6、Node 24、Buildx/Trivy 与受管 `kejilion.sh`；无候选升级。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本 `0.99.5`；OCI index=`sha256:820d823c74ef413cbc57d074d9e69af49ae47209846c83eec1fc6e1c26cfb668`；`linux/amd64`=`sha256:177910084e73e88962f7212e47a04f5fc84fa61ec8c7a7bb5b9af911b1c85509`；`linux/arm64`=`sha256:f4a771f324f99c0326bbefa6170ede3b6df0c99b77f924545b133273d83b5e6b`；受管脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`、SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无；首次 L3 的 ICU 日期格式断言被门禁拦截，已在生产写入前由 `e0cc4c8` 修复并通过完整重跑。
- 升级后的兼容、安全、构建、性能资源和回滚结论：公开 Release、OCI 双架构、image E2E、生产备份及 postdeploy 均通过；可回滚至 `v0.99.4` 和本次备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：L3 使用 `arena-154` 固定 Linux Runner；生产为 `arena-154`，Panel/Agent 运行于 Docker/systemd，公开镜像 E2E 使用真实 Docker。
- 环境策略 ID 与允许用途：`environment-policy.json`；`arena-154` / `production-safety-check`、`production-deploy` 均通过；未使用 `prod-108`。
- 使用的精确候选或公开产物：源码候选 `e0cc4c896a33f6dc0ed5740733eea42e30a3213c`、tag `v0.99.5`、公开镜像 `docker.io/kjlion/kejilion-panel@sha256:820d823c74ef413cbc57d074d9e69af49ae47209846c83eec1fc6e1c26cfb668`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v0.99.5-e0cc4c8-l3-r2` / pass / exit 0，证据目录 `C:\GitHub\_release-artifacts\v0.99.5-e0cc4c8-l3-r2`，脚本为 `scripts/run-release-l3.mjs` 及固定远端入口；生产证据 run=`v0.99.5-production-20260831-01`，preflight、backup、postdeploy 均 pass。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：公开镜像 E2E 完整 bootstrap/health/host/proxy cookie/healthcheck 流程通过；无长期 soak，本版只读功能且生产 postdeploy 已完成健康和日志检查。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：mock UI acceptance 覆盖 `390x844` 和宽视口、三语切换、Docker 归属、选择器排序/已反代、无横向溢出和空 console error/warn；缩放 125%/200% 与真实生产宿主机交互未执行，记为未验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：本版功能无宿主机写入；备份阶段归档及旧版本恢复校验通过，更新后 postdeploy 通过；未做故障注入。
- 未执行场景及原因：长期 soak、125%/200% 缩放和真实三语完整页面逐项人工巡检未执行；L3/前端回归、mock UI 与生产健康门禁已覆盖本版风险边界。

## 发布产物与公开仓库复核

- GitHub Release：[`v0.99.5`](https://github.com/kejilion/KPanel/releases/tag/v0.99.5) 已公开，tag annotated object=`af87d3adc7901747b1983c2b1415f20e4ad962e2`，peeled product commit=`e0cc4c896a33f6dc0ed5740733eea42e30a3213c`；附件含 amd64/arm64 Agent 与 node、deploy tar、`LICENSE`、`SHA256SUMS`、`THIRD_PARTY_NOTICES.md`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.5` 与 `:latest` 均为 `sha256:820d823c74ef413cbc57d074d9e69af49ae47209846c83eec1fc6e1c26cfb668`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:177910084e73e88962f7212e47a04f5fc84fa61ec8c7a7bb5b9af911b1c85509`；arm64=`sha256:f4a771f324f99c0326bbefa6170ede3b6df0c99b77f924545b133273d83b5e6b`。
- 附件及 `SHA256SUMS`：Release 已包含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-0.99.5.tar.gz`、`SHA256SUMS`、`LICENSE`、`THIRD_PARTY_NOTICES.md`。
- 公开镜像 `image_e2e=pass`：在 `arena-154` 以 immutable digest 执行 `packaging/tests/image-e2e.sh`，结果 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：`kpanel.conf` 相对上一稳定版本无差异；Release 受管脚本契约通过；生产标准入口为 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，返回 `Update Complete`。

## 生产部署安全核对

- 生产目标和部署授权范围：本次明确授权仅更新 `arena-154`；没有连接 `108` 或 `prod-108`。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`，用途为 candidate-validation、production-safety-check、production-deploy。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight run=`v0.99.5-production-20260831-01` phase=`preflight` pass，健康版本 `0.99.4`；backup phase pass；备份为 `/root/kpanel-backups/pre-v0.99.5-20260830T165259Z`，归档与 SHA256 校验通过。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy pass；health `status=ok/version=0.99.5`；Panel `running/healthy`、Agent `active/running`、restart `0`、OOM `false`；revision=`e0cc4c8`、image version=`0.99.5`、digest=`sha256:820d823c...`；fatal/panic/OOM 日志签名为 none；protected diff 为 0。
- 生产已执行写操作：标准应用市场/`kejilion.sh` 更新；Docker 拉取新 OCI、重建 `kejilion-panel`；生产证据 inbox/evidence 与本次备份写入。
- 仅在隔离真机执行、未在生产执行的场景：公开镜像 E2E 的临时容器/网络；mock UI 浏览器预览。

## 回滚

- 源码/tag：回滚点 `v0.99.4` / `a4c163ffd432ba012b61a4d57b95d82532554445`。
- 镜像 digest：按 v0.99.4 的稳定 Release/OCI digest 恢复；本次生产备份同时保存旧运行镜像归档。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.5-20260830T165259Z`，含旧容器 inspect、旧 image、应用目录、Agent unit、`kpanel.conf` 和 `SHA256SUMS`。
- 回滚步骤和回滚后复核：按现有应用市场回滚入口恢复 v0.99.4，恢复 Compose、`.env`、数据和 Agent 文件后执行同一 `production-evidence` postdeploy 验收；本次未触发回滚。
- 回滚后生产实际版本与健康状态：未执行回滚，因此不适用；当前 v0.99.5 postdeploy 已验证健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release v0.99.5 已公开；Docker `latest` 与 0.99.5 同 digest；标准入口已在生产拉取并运行该 digest。
- 公共默认更新通道决策：保留 v0.99.5 为默认稳定通道；无已知阻断问题。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-30T19:45:53+08:00
- 候选冻结时间：2026-08-31T00:19:05+08:00
- 生产完成时间：2026-08-31T00:54:01+08:00
- 提交到生产用时：5.14 小时
- 是否回滚、紧急热修复或重复发布：否（无生产回滚、紧急热修复或重复发布；首次 L3 失败在生产写入前修复并重跑）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

本次首次 L3 run=`v0.99.5-5f4287b-l3-r1` 在生产写入前被前端 ICU 日期格式断言拦截（125/126 文件、1060/1061 测试）；修复为仅断言各 locale 与自身 `Intl.DateTimeFormat` 一致后，`v0.99.5-e0cc4c8-l3-r2` 完整通过。该事件未逃逸到生产，不计为发布流程异常或生产变更失败。

## 遗留风险与后续准入

- 未验证风险：生产根分区使用率约 99%（可用约 1.8G）；长期 soak、125%/200% 缩放与完整人工三语巡检未执行。
- 已实现待实机准入：本版公开镜像、标准更新入口和生产 postdeploy 已完成；未验证场景不影响本版只读功能上线，但应作为后续专项验收候选。
- 不阻断本版的理由：发布与生产门禁均以精确 `e0cc4c8`、`v0.99.5`、OCI immutable digest 和本次备份通过；Docker 失败有原监听结果回退。
- 后续应进入的自动门禁或专项工作流：持续监控 `arena-154` 磁盘空间；补充真实浏览器 125%/200% 缩放和三语人工巡检；保持 `image_e2e`、candidate/main freshness 与 postdeploy 证据为必需门禁。
