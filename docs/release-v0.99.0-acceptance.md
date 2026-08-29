# KPanel v0.99.0 发布验收记录

日期：2026-08-29

发布级别：L3

候选提交 / 标签：`641a3655c928146fa8daa510c5d3d01747f5d15f` / `v0.99.0`

上一稳定版本 / 回滚点：`v0.98.4` / `sha256:7e0128bee4b5b190ed1dcfb93b50161c2a11d2ab15c3f0358bbc7daa06f8f703`（commit `7e1ffa8416fcfa6fb0960a87e00f53b53220735d`）

## 发布画像

- 业务域：集群监控、轻量节点接入与多主机终端。
- 变更面：协议与状态、宿主机 root PTY 服务、部署脚本、终端页面和发布产物。
- 受影响用户旅程：从 KPanel 生成轻量节点接入命令，节点安装或更新后通过出站 HTTPS 注册终端能力，在集群页面打开终端并完成输入、输出、调整大小和关闭。
- 未变化契约：既有 `light-v1` 遥测兼容；不增加 SSH、WebSocket 或其他入站监听；既有 KPanel 数据库 schema、应用市场入口和 Panel 无特权容器边界不变。
- 风险等级及理由：中到高；新增 root PTY broker 和端到端加密终端 relay，但终端私钥 root-only、能力需认证后公布、请求有 scope/大小/重放保护，节点仍只主动出站。

## 发布范围与未纳入内容

- 用户可见更新：轻量节点新增与多主机终端一致的安全远程终端；节点更新/卸载链路管理独立的 `kejilion-node-terminal.service`；旧中心不支持 relay 时继续遥测并退避。
- 精确提交清单：KPanel `641a3655c928146fa8daa510c5d3d01747f5d15f`（父提交/候选基线 `9fe3ed9281722117c0e893f4b91032daa4268eb4`）；`kejilion-sh` `d58079304a92936bf8e3d90467eea484c5b63d6f`。
- 明确未纳入的分支、文件或后续事项：相对候选基线的最终文件差异不含 System Center 页面、路由、API 或文档；`kejilion/apps` 未产生提交；`108`/`prod-108` 未连接。独立 Linux 轻量节点的实机安装、更新、断网恢复、摘要拒绝、broker 重启、回滚、卸载和真实浏览器终端旅程未在本轮执行。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Go 全包、集群/终端协议测试、Panel API 测试、Web 全量测试、脚本 smoke 和 Release 构建均通过 | 未在独立 Linux 节点完成真实接入并从浏览器打开 PTY |
| 网络入侵与供应链安全 | 已验证 | 固定 L3 的 govulncheck、npm audit、Trivy source/config/image、OCI 多架构、脚本摘要和 Release 附件校验均通过 | 未执行独立节点公网安装的真实网络观测 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | relay 重投递/重启对账、session 清理、旧中心兼容、脚本更新回滚和失败清理自动测试通过；生产 Panel postdeploy 通过 | 节点 systemd 服务重启、断网恢复和真实回滚未执行 |
| 性能与资源预算 | 已实现未实机验证 | 命令/输出/会话边界和并发/竞态自动测试通过，节点不增加中心主动轮询 | 未执行独立节点长期 soak 或真实资源采样 |
| 用户体验与可访问性 | 已实现未实机验证 | 既有终端页面复用，TerminalView 布局测试、Web 全量测试和 build 通过 | 未在真实轻量节点连接上执行浏览器输入、输出、resize、断线重连和移动视口矩阵 |
| 数据、配置与迁移 | 已验证 | 轻量节点状态与终端身份分离、凭据原子写入和 root-only 配置自动测试通过；生产备份、SQLite 和 protected 配置检查通过 | 未在真实节点执行安装后文件清理和卸载残留检查 |

## 自动门禁

- 定向测试及结果：Panel Web `npm test` 为 122 个文件/1044 项通过，`npm run typecheck`、`npm run build` 通过；Go `go test ./...`、核心 race、`go vet ./...` 通过；新增轻量节点 terminal-broker、Noise relay、会话/重投递/重放、PTY 配置和 Panel 接入测试通过；`kejilion.sh` 四语言语法、同步和 `test_kpanel_light_node_smoke.sh` 通过。
- `make verify-release` 环境和结果：固定 runner `kpanel-release-gate:go1.26.6-node24`，不可变 Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.99.0-641a365-l3-r1`，`release_l3_gate=pass`、`release_l3_remote=pass`；证据目录为 `C:\GitHub\_release-artifacts\v0.99.0-641a365-l3-r1`，bundle SHA-256=`4736e1e820b1c2b0031bef8eaf639210d86e78b2ce38a228755464dbe9aeff9e`，manifest SHA-256=`81bb85b861d4e4b0250a7c78c4e4f00362fbeae6b9d83b369ea44888a2251c81`，plan SHA-256=`0700020e3470931c30dff474215ef447430ad9fa1c942ad4f919fc8ebd8fad15`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：CI run `33221547896`、Dependency freshness run `33221547463`，均绑定 `641a3655c928146fa8daa510c5d3d01747f5d15f`，completed/success。
- 主线 CI：CI run `33222192961`、Dependency freshness run `33222192963`，均绑定同一 SHA，completed/success；推进前重新确认 `origin/main=9fe3ed9281722117c0e893f4b91032daa4268eb4` 未漂移。
- Release workflow：run `33222476652`，tag `v0.99.0` 解引用到候选 SHA，completed/success；GitHub Release 已公开，非 draft、非 prerelease。
- 安全扫描、镜像契约、SBOM/provenance：Release 的 Go/Node 双架构构建、native image validation、OCI 多架构推送、`latest` promotion、SBOM/provenance、Trivy 和受管脚本契约均通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本次未单独生成新的完整依赖报告；候选 Dependency freshness run `33221547463` 成功，L3 的 govulncheck、npm audit 和 Trivy 均通过；不据此声明所有上游依赖均为最新。
- 最近每日安全通告审计、EOL 复核状态及证据：本次以 L3 的 govulncheck、npm audit、Trivy 和 Release 供应链校验为准；独立 EOL 复核未单独重做，未作额外“全部当前”结论。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增运行时依赖或基础镜像；未记录新的完整检测行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：固定 Go `1.26.6`、Node `24`、既有构建镜像和扫描器；受管 `kejilion.sh` 更新为指定提交。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Panel/Web 版本 `0.99.0`；脚本 revision=`d58079304a92936bf8e3d90467eea484c5b63d6f`，clean Git blob SHA-256=`68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2`；生产 OCI index=`sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：未改变依赖图；其他工具链或上游升级候选不纳入本版，退出条件为下一次具备完整检测源的独立兼容评估。
- 升级后的兼容、安全、构建、性能资源和回滚结论：自动门禁、公开 Release/OCI、生产更新和 Panel postdeploy 均通过；无 Panel schema 迁移，回滚材料为 `v0.98.4` OCI、Compose、`.env`、Agent 文件和本次停写备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：固定 L3、公开 OCI 校验和正式生产目标均为 `arena-154`；L3 使用固定 Go `1.26.6` / Node `24` runner。
- 环境策略 ID 与允许用途：`environment-policy.json` 的 `arena-154`；candidate-validation、production-safety-check 和 production-deploy 通过；`prod-108` 为 disabled，`108`/`prod-108` 未连接。
- 使用的精确候选或公开产物：候选 `641a3655c928146fa8daa510c5d3d01747f5d15f`；公开 `kjlion/kejilion-panel:0.99.0` 与 `latest` 均为 `sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`；amd64=`sha256:6f9b4b7cb37ab4570e56f27e66de6a46ca607c54d944bc1d1599677dd39df7b7`，arm64=`sha256:46565b5a4ab55977a2cd6d4fe86ed7f2b5aef68aaa5f5f6aad571c396e2c83d1`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v0.99.0-641a365-l3-r1` passed/0、无超时，证据目录为 `C:\GitHub\_release-artifacts\v0.99.0-641a365-l3-r1`；生产 `v0.99.0-641a365-prod-20260829` 的 preflight/backup/postdeploy 均 passed/0，证据目录为 `C:\GitHub\_release-artifacts\v0.99.0-641a365-prod-20260829-{preflight,backup,postdeploy}`，固定生产脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。
- 测试窗口/循环数及风险依据：L3、候选/main CI、Release 和生产 preflight/backup/postdeploy 各一次通过；无独立节点 soak，因为当前唯一批准的真实目标 `arena-154` 是生产机，不能将其改装成测试节点。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：TerminalView 自动化布局和组件回归通过；真实轻量节点浏览器终端的连接、键盘输入、输出、resize、断线重连、主题/缩放/视口矩阵未执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 已验证安装脚本安全、摘要校验、服务契约、回滚/失败清理和应用生命周期；生产仅按固定入口完成停写备份、标准更新和 postdeploy。备份输出曾出现一次 `curl: (56) Recv failure: Connection reset by peer`，固定 gate 仍通过，备份六项摘要均为 `OK`、服务恢复且未造成无效证据。
- 未执行场景及原因：没有第二台获授权的独立 Linux 节点或测试 enrollment token；因此未执行节点真实安装、断网恢复、自动更新、摘要拒绝、broker 重启、回滚、卸载，以及基于真实节点的浏览器终端验收。不能用 Panel 生产健康证据替代这些场景。

## 发布产物与公开仓库复核

- GitHub Release：[v0.99.0](https://github.com/kejilion/KPanel/releases/tag/v0.99.0)，Release workflow=`33222476652`，公开、非 draft、非 prerelease；annotated tag 解引用到 `641a3655c928146fa8daa510c5d3d01747f5d15f`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.99.0` 与 `latest` 均为 `sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:6f9b4b7cb37ab4570e56f27e66de6a46ca607c54d944bc1d1599677dd39df7b7`；arm64=`sha256:46565b5a4ab55977a2cd6d4fe86ed7f2b5aef68aaa5f5f6aad571c396e2c83d1`；公开 OCI 还包含 provenance/SBOM attestation。
- 附件及 `SHA256SUMS`：GitHub Release 含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、部署归档、`LICENSE`、`SHA256SUMS` 和 `THIRD_PARTY_NOTICES`；Release workflow 完成附件摘要和 native image 校验。
- 公开镜像 `image_e2e=pass`：L3/Release 的 native image validation、OCI digest 校验和生产公开镜像回拉均通过；生产容器最终 image 与 repo digest 精确匹配。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场配置无需空提交；`kpanel.conf` 的受管脚本 revision/SHA 与 `d58079304a92936bf8e3d90467eea484c5b63d6f` / `68a9451582034444f5178f893e8a00d6c4d5e24d109401b6bdf091f948251cf2` 一致，clean Git blob 的 `bash -n`、同步和轻量节点 smoke 通过。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求复核后走上线流程；仅执行 KPanel `v0.99.0` 标准应用更新、生产证据和备份，目标仅 `arena-154`。
- 验证/灰度环境：固定 `arena-154` L3 runner、公开 OCI 校验和生产安全证据入口，均来自 `environment-policy.json`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight `0.98.4`，Panel health `ok`，Agent `loaded/active/running/enabled`；backup 通过，备份目录为 `/root/kpanel-backups/pre-v0.98.4-20260829T001313Z`，六项备份摘要均为 `OK`，protected diff 为空。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy health version=`0.99.0`、status=`ok`、revision=`641a3655c928146fa8daa510c5d3d01747f5d15f`、OCI digest 精确匹配；Panel `running/healthy`、restart=0、OOM=false；Agent `active/running/enabled`；protected diff 为空，SQLite quick check 通过，fatal signature scan=`NONE`。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取 `latest` 并重建 Panel，实际拉取 OCI digest `sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`；未执行业务数据写入、schema 迁移或端口变更。
- 仅在隔离真机执行、未在生产执行的场景：L3 负例、脚本失败清理/回滚模拟和自动化协议/布局回归；轻量节点实机闭环因缺少独立授权主机未执行。

## 回滚

- 源码/tag：`v0.98.4` / commit `7e1ffa8416fcfa6fb0960a87e00f53b53220735d`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:7e0128bee4b5b190ed1dcfb93b50161c2a11d2ab15c3f0358bbc7daa06f8f703`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.98.4-20260829T001313Z`，包含旧应用目录、旧镜像、Agent unit、应用配置和校验摘要。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按应用市场原生失败回滚流程成套恢复 `v0.98.4` OCI、Compose、`.env`、数据目录和 Agent，再执行固定 preflight/postdeploy；目标机终端 unit 由节点自身卸载流程管理，不通过 Panel 回滚删除。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release 当前为 `v0.99.0`；Docker `latest` 与 `0.99.0` 同为 `sha256:c148aaf6bb48188027bc9f61fa41a3086ccfff2cdbfaffa904043d40ac537586`；标准更新入口本次实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.99.0` 为默认更新版本；无产品退化证据，因此不恢复上一稳定版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-29T07:29:04+08:00
- 候选冻结时间：2026-08-29T07:46:29+08:00
- 生产完成时间：2026-08-29T08:14:25+08:00
- 提交到生产用时：0.76 小时
- 是否回滚、紧急热修复或重复发布：否（无产品变更失败、生产回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/managed-script-contract/stale-doc-hash",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 SHA 5c6828ce 的 change-aware verification 因 Dockerfile 已切换到 d580793，而 managed-script 文档契约仍保留旧摘要，CI run 33221224214 被门禁拦截；未推送 main、未创建 v0.99.0 tag、未生产写。",
    "recoveryEvidence": "更新 docs/external-config-sources.md 的受管脚本 revision/SHA 并收敛为最终候选 SHA 641a3655；候选 CI 33221547896、依赖新鲜度 33221547463、固定 L3、main CI 和 Release workflow 均 success。",
    "permanentAction": "候选冻结前固定运行 check-managed-script-contract.sh，并将 Dockerfile、受管脚本 clean blob 和文档契约作为同一候选摘要复核；退出条件为候选 CI 不再因 revision/SHA 漂移拦截。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/tag-preflight/powershell-regex-escape",
    "position": "before-production-write",
    "count": 1,
    "impact": "推送验收记录前的 main/tag 守门命令因 PowerShell 正则转义过度，误将未变化的远端 main 与 v0.99.0 tag 判为不匹配并停止；未产生远端写入。",
    "recoveryEvidence": "改用 git ls-remote 结果的 tab 字段精确比较，确认 main=641a3655、v0.99.0 annotated tag object=797691d 后继续执行安全推送。",
    "permanentAction": "发布前远端 ref 守门固定使用字段解析和完整 SHA 等值比较，不在 PowerShell 中拼接多层正则；退出条件为检查误报时不触发写入且重试可复现通过。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

本次另有一次备份阶段 `curl 56` 瞬时连接重置输出，但固定备份 gate 未失败、证据未失效且六项摘要均为 `OK`，因此不计入“流程异常或无效证据拦截”指标。

## 遗留风险与后续准入

- 未验证风险：独立 Linux 轻量节点真实安装、离线恢复、自动更新、摘要拒绝、broker 重启、回滚、卸载，以及真实浏览器终端连接/输入/输出/resize/断线重连尚未执行。
- 已实现待实机准入：轻量节点 terminal-broker、v2 Noise relay、节点脚本生命周期和 KPanel 终端入口已实现并通过自动门禁；需在获授权的独立 systemd Linux 节点补齐上述闭环。
- 不阻断本版的理由：v0.99.0 已完成候选/main CI、固定 L3、公开 Release/OCI、生产备份、标准更新和 postdeploy；当前没有轻量节点实机失败证据，不以 Panel 生产健康冒充节点验收，也不自动回滚已健康的 Panel 发布。
- 后续应进入的自动门禁或专项工作流：在下一次轻量节点专项验收中使用独立 Linux 测试节点和一次性 enrollment token，按 `docs/cluster-monitoring.md` §8 完成安装、断网、更新、摘要拒绝、broker 重启、回滚、卸载及浏览器终端证据；完成前该功能维持“已实现未实机验证”状态。
