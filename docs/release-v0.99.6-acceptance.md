# KPanel v0.99.6 发布验收记录

日期：2026-08-31

发布级别：L3

候选提交 / 标签：`79793bdb531b969a2d45ea1afec4ba0e28c9a2d2` / `v0.99.6`

上一稳定版本 / 回滚点：`v0.99.5` / `e0cc4c896a33f6dc0ed5740733eea42e30a3213c`，生产回滚镜像为 `sha256:820d823c74ef413cbc57d074d9e69af49ae47209846c83eec1fc6e1c26cfb668`

## 发布画像

- 业务域：IP 质量信息展示、应用终端与 Apps/Cluster/Terminal 页面运行时多语言、发布构建工具链。
- 变更面：展示 / 只读 / 前端运行时资源 / 构建与发布门禁；不新增宿主机写操作、数据库迁移或业务协议。
- 受影响用户旅程：查看合并后的 IP 质量风险与属性详情；在应用终端、Apps、Cluster 和 Terminal 页面切换 `en-US`、`zh-CN`、`zh-TW`；通过标准入口安装或更新 KPanel。
- 未变化契约：Panel/Agent API、数据库、端口、Compose、Agent 权限、受管 `kejilion.sh` 和应用市场安装契约均未变化；`packaging/kejilion-app/kpanel.conf` 相对 `v0.99.5` 无差异。
- 风险等级及理由：中风险。包含前端多页面文案和 IP 质量布局调整，并升级构建基座；无数据迁移和协议变化，候选/main/Release/公开镜像及生产 postdeploy 均通过。

## 发布范围与未纳入内容

- 用户可见更新：
  1. 体检页合并 IP 质量风险与属性信息，保留风险等级、分数和单行可读详情。
  2. 应用终端、Apps、Cluster、Terminal 页面补齐运行时多语言，覆盖状态、错误、确认提示、操作按钮和空态，并增加页面级 i18n 回归检查。
  3. 构建与发布基座更新到 Go `1.26.7`、Node.js `24.20.0`，同步固定 CI、依赖检测、Release、真机工作流和 Docker 多架构镜像摘要。
- 精确提交清单：基线 `0e4e75575b61c30ac9b77ab11f7d26c25fcf7254`；纳入 `70809edf183429a0536371c2a976dad3059048d5`（IP quality）、`7ed18033d0f7a426ad40d55820020ea529ac1042`（Go/Node 构建基线）、`03159d489fcdeea723456ab9bc3d6a0590bc4381`（应用终端与页面 i18n）和版本准备 `79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`。候选相对 `v0.99.5` 为 32 个文件、644 行新增、179 行删除。
- 明确未纳入的分支、文件或后续事项：系统中心页面、路由、维护写操作和无关业务未纳入；没有复用旧版本 L3、公开镜像或生产证据；没有连接、备份、部署、升级或核对 `108`/`prod-108`。未在本版新增浏览器人工视觉验收或长期 soak。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 `make verify-release`、页面/i18n 自动回归、公开镜像 `image_e2e=pass`、生产标准更新入口和 postdeploy 均通过。 | 本版不改变持久数据协议；真实外部业务数据写路径不属于本版变更面。 |
| 网络入侵与供应链安全 | 已验证 | Release workflow 的源码、依赖、Go 调用路径、secret/config 和原生镜像扫描均成功；同 SHA candidate/main freshness 均成功。 | 未发现需阻断本版的安全扫描结论。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 全量前端/Go/race/构建、应用配置生命周期、生产 backup 校验、更新后 health/Agent/日志/SQLite postdeploy 均通过。 | 未执行长期 soak；生产根分区仍约 96% 使用率，需持续观察。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 资源快照：Panel `74.06MiB / 256MiB`、7 PIDs、restart `0`、OOM `false`；容器运行时契约通过。 | 未执行长期压力或 soak；本版展示与 i18n 变更没有新增后台常驻任务。 |
| 用户体验与可访问性 | 已实现未实机验证 | 精确 SHA 的页面布局、i18n 和窄视口自动回归在 L3 通过。 | 本轮未执行真实浏览器的浅/深色、键盘/焦点、125%/200% 缩放和人工三语巡检；未把旧版本 preview 证据当作本版证据。 |
| 数据、配置与迁移 | 已验证 | `kpanel.conf` 相对上一稳定版本无语义差异；生产 protected 配置摘要 diff、SQLite quick check、备份归档及恢复校验通过。 | 不适用数据库迁移。 |

## 自动门禁

- 定向测试及结果：L3 中 `i18n:check`、前端 typecheck/build、前端测试、Go 全量测试、核心 race、vet、双架构构建和应用配置生命周期均通过。
- `make verify-release` 环境和结果：目标仅 `arena-154`；L3 run=`v0.99.6-79793bd-l3-r2`，candidate=`79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`，base main=`0e4e75575b61c30ac9b77ab11f7d26c25fcf7254`，`release_gate_runner=pass`。Runner=`kpanel-release-gate:go1.26.7-node24`，不可变 Runner ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；远端证据目录=`/root/kpanel-release-evidence/v0.99.6-79793bd-l3-r2`，本地目录=`C:\GitHub\_release-artifacts\v0.99.6-79793bd-l3-r2`。bundle SHA-256=`e24d2860908e637ea631e8273c8903149ed93d6232ab9bbeff382c5bd955fffa`，manifest SHA-256=`743b22c30da38b3fbaf2c5c66cf394449685048345db3d6001fbed4e69ce2b52`，plan SHA-256=`cebc0a8414b2fbf5a2a0d6c763725b937e11b90e09e95ad2affb9e84edaaf507`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，远端 `l3-verify-release.log` SHA-256=`aab109f3ab7865daab7f5740ded4e508910034fc64b8d88d6e68636bd4141758`，`status.txt` 为 `passed`、exit code `0`。
- 候选 CI：`CI #33368568015` 成功，`Dependency freshness #33368568017` 成功，均为候选 SHA `79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`。
- 主线 CI：`origin/main` 快进到同一产品 SHA 后，`CI #33368953752` 成功，`Dependency freshness #33368953735` 成功。
- Release workflow：`Release #33369420624` 成功；GitHub Release 已公开且非 draft/non-prerelease，候选分支已自动删除。
- 安全扫描、镜像契约、SBOM/provenance：Release 的源代码/依赖/镜像扫描、非 root/healthcheck/label/运行时契约、SBOM/provenance 和双架构构建均成功。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选和主线的 Dependency freshness 均为同 SHA 成功；没有引入业务依赖升级。
- 最近每日安全通告审计、EOL 复核状态及证据：Release workflow 的 Go 调用路径、Node 依赖、源码/配置/secret 和镜像扫描通过；未产生阻断性 EOL 行动项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版基座行动项为 Go `1.26.7`、Node `24.20.0`，已由 Release、CI 和 L3 同步验证。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：固定 Go `1.26.7`、Node `24.20.0`；Runner 基础镜像 digest 为 Go `sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468`、Node `sha256:e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf`，工具包含 gcc、buildx `v0.34.1`、git、make。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：产品版本 `0.99.6`；公开 OCI index=`sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`；`linux/amd64`=`sha256:0d954cf5c789455a6f45a08c85efc2a1e5c674a5a57d432d02b852a915feb656`；`linux/arm64`=`sha256:cc4decba7ca4f4e435e411916ab2162a725744ff5aea61ecab8e2f34a696f0a8`；受管脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`、SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：首轮 `v0.99.6-79793bd-l3-r1` 因 `arena-154` 缺少指定 Runner 被 fail-closed 拦截，未执行候选代码或生产写入；已补齐并预检新 Runner 后，以全新 `l3-r2` 重跑通过。当前暂无剩余阻断性候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：公开 Release、双架构 OCI、公开镜像 E2E、生产备份、标准更新和 postdeploy 均通过；可回滚至 `v0.99.5` 或本次备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：L3 与公开镜像 E2E 使用 `arena-154` Linux/amd64；Runner 为 Go `1.26.7`、Node `v24.20.0`、buildx `v0.34.1`，生产 Panel/Agent 运行于 Docker/systemd。
- 环境策略 ID 与允许用途：`environment-policy.json`；`arena-154` 的 `candidate-validation`、`production-safety-check`、`production-deploy` 均通过；未使用 `prod-108`/`108`。
- 使用的精确候选或公开产物：源码候选 `79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`、tag `v0.99.6`、公开镜像 `docker.io/kjlion/kejilion-panel@sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run=`v0.99.6-79793bd-l3-r2` / pass / exit 0，证据目录如上，入口为 `scripts/run-release-l3.mjs` 与仓库固定远端入口；未创建浏览器后台作业。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：公开镜像 E2E 完整覆盖 health、Host/proxy、bootstrap cookie、容器 healthcheck 和清理，结果 `image_e2e=pass`；无长期 soak，本版未新增常驻后台任务。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动回归覆盖本版 i18n/layout/失败状态；真实浏览器的视口、缩放、主题、键盘/焦点和人工三语巡检本轮未执行，标记为已实现未实机验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 阶段归档旧容器/镜像/应用目录并通过 SHA256 校验，恢复服务后 predeploy snapshot 通过；正式更新后 postdeploy 通过；未做故障注入或实际回滚。
- 未执行场景及原因：长期 soak、真实浏览器人工巡检、125%/200% 缩放、生产故障注入和完整回滚演练未执行；它们不替代已完成的 L3、公开镜像 E2E 与生产安全门禁，列入后续专项。

## 发布产物与公开仓库复核

- GitHub Release：[`v0.99.6`](https://github.com/kejilion/KPanel/releases/tag/v0.99.6) 已公开，annotated tag object=`8cd6ddb7ed6ac25dbb50662f6e94872632a68cda`，peeled product commit=`79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`；附件含 amd64/arm64 Agent 与 node、`kejilion-panel-deploy-0.99.6.tar.gz`、`LICENSE`、`SHA256SUMS`、`THIRD_PARTY_NOTICES.md`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.6` 与 `:latest` 均为 `sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:0d954cf5c789455a6f45a08c85efc2a1e5c674a5a57d432d02b852a915feb656`；arm64=`sha256:cc4decba7ca4f4e435e411916ab2162a725744ff5aea61ecab8e2f34a696f0a8`。
- 附件及 `SHA256SUMS`：Release 已包含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-0.99.6.tar.gz`、`SHA256SUMS`、`LICENSE`、`THIRD_PARTY_NOTICES.md`。
- 公开镜像 `image_e2e=pass`：在 `arena-154` 拉取公开 `0.99.6` 镜像并执行仓库 `packaging/tests/image-e2e.sh`，结果为 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：候选 `packaging/kejilion-app/kpanel.conf` 相对 `v0.99.5` 无差异；与 `C:\GitHub\kejilion\apps\kpanel.conf` 仅有换行表现差异，忽略行尾空白后内容一致，无需制造应用市场提交；生产更新使用受管标准入口并返回 `Update Complete`。

## 生产部署安全核对

- 生产目标和部署授权范围：用户已明确授权本次完整上线；正式写入仅执行 `arena-154`。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`，用途为 candidate-validation、production-safety-check、production-deploy。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight run=`v0.99.6-production` / phase=`preflight` / pass，健康版本 `0.99.5`；backup phase pass，备份=`/root/kpanel-backups/pre-v0.99.6-20260831T075307Z`，归档内容及 `SHA256SUMS` 校验通过。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run=`v0.99.6-production` / phase=`postdeploy` / pass；health `status=ok/version=0.99.6`；Panel `running/healthy`、Agent `active`、restart `0`、OOM `false`；revision=`79793bdb531b969a2d45ea1afec4ba0e28c9a2d2`、image version=`0.99.6`、RepoDigest=`sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`；Panel 资源约 `74.06MiB/256MiB`、7 PIDs；近 10 分钟 fatal/panic/OOM 日志签名为 none；protected diff 为 0，SQLite quick check 通过。
- 生产已执行写操作：通过标准应用市场/`kejilion.sh` 更新；Docker 拉取公开 `latest`、重建 `kejilion-panel`；生产证据和本次备份写入。为恢复空间，另清理了未使用 Docker build cache，未删除活动容器、镜像或卷。
- 仅在隔离真机执行、未在生产执行的场景：公开镜像 E2E 的临时容器/网络；前端人工视觉验收、故障注入和回滚演练未执行。

## 回滚

- 源码/tag：回滚点 `v0.99.5` / `e0cc4c896a33f6dc0ed5740733eea42e30a3213c`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel:0.99.5` / `:latest` 的已知稳定 index=`sha256:820d823c74ef413cbc57d074d9e69af49ae47209846c83eec1fc6e1c26cfb668`；本次备份同时保存旧运行镜像归档。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.99.6-20260831T075307Z`，含旧容器 inspect、旧 image、应用目录、Agent unit、`kpanel.conf` 和 `SHA256SUMS`。
- 回滚步骤和回滚后复核：恢复备份的 Compose、`.env`、数据、旧镜像和 Agent 文件，使用标准入口恢复 `v0.99.5`，再执行同一 `production-evidence` postdeploy；本次未触发回滚。
- 回滚后生产实际版本与健康状态：未执行回滚，因此不适用；当前 `v0.99.6` postdeploy 已验证健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest 为 `v0.99.6`；Docker `latest` 与 `0.99.6` 同 index digest；标准入口已在生产拉取并运行该 digest。
- 公共默认更新通道决策：保留 `v0.99.6` 为默认稳定通道；未发现需要恢复旧默认版本的产品问题。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-31T12:21:30+08:00
- 候选冻结时间：2026-08-31T15:17:06+08:00
- 生产完成时间：2026-08-31T15:54:03+08:00
- 提交到生产用时：3.54 小时
- 是否回滚、紧急热修复或重复发布：否（首轮 L3 Runner 缺口在生产写入前拦截并修复重跑，未造成生产变更失败）
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
    "fingerprint": "l3/run-release-gate/missing-runner",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 l3-r1 在指定 Runner image inspect 阶段被 fail-closed 拦截，未执行候选门禁或生产写入。",
    "recoveryEvidence": "arena-154 的 v0.99.6-79793bd-l3-r1 status.txt 为 failed/exit_code=1；随后补齐 Runner sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3，并由 l3-r2 status.txt 记录 passed/exit_code=0。",
    "permanentAction": "已将 Go 1.26.7、Node 24.20.0 及固定工具包构建为 kpanel-release-gate:go1.26.7-node24，并在 L3 前置预检中记录完整 immutable image ID；后续发布必须先通过同类 Runner 版本和 image ID 校验。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：真实浏览器人工验收（主题、键盘/焦点、125%/200% 缩放和三语巡检）、长期 soak、生产故障注入/实际回滚未执行；`arena-154` 根分区约 96% 使用率，当前可用约 3.8G。
- 已实现待实机准入：本版 IP 质量合并和多页面 i18n 已由自动回归、L3 和生产健康门禁覆盖；真实浏览器视觉/交互证据应在后续专项补齐。
- 不阻断本版的理由：候选、main CI、Release、公开双架构镜像、`image_e2e`、生产 backup/update/postdeploy 均以精确 SHA 和 immutable digest 通过；未验证项不涉及新增数据迁移或后台持久任务。
- 后续应进入的自动门禁或专项工作流：持续监控 `arena-154` 磁盘空间；把新 Runner 的构建和 preflight 纳入发布基础设施；补充真实浏览器视觉/交互矩阵、长期 soak 和受控回滚演练；保持 candidate/main freshness、公开镜像 E2E 与 postdeploy 为必需门禁。
