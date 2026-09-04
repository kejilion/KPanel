# KPanel v1.3.1 发布验收记录

日期：2026-09-05

发布级别：L3

候选提交 / 标签：`650f4db432d09f2251f8a03b6ac554d84e31d23b` / `v1.3.1`

标签对象：annotated tag object `fa8262807fffb9d0b4a5a333f31927feb54c12e6`；剥离后的产品提交精确为 `650f4db432d09f2251f8a03b6ac554d84e31d23b`。

候选基线：`origin/main@a3a82cd9115fdac270ffe3c1ad6cae355f43de3a`，基线标签 `v1.3.0` 的产品提交为 `8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`。

上一稳定版本 / 回滚点：`v1.3.0` / `8374ff2d4a20d6cd949dc37a93f412eed64a1ec9` / OCI index `sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`。

## 发布画像

- 业务域：集群轻量节点重新配对后的连接确认。
- 变更面：展示、只读轮询、集群配对交互和发布部署；未新增宿主机危险写入、数据库迁移或 Agent 协议。
- 受影响用户旅程：在集群页生成轻量节点接入命令后，窗口等待新节点实际出现在主机列表中，再允许完成配对，避免命令执行但节点尚未可用时误报成功；窗口关闭、过期或组件卸载时停止轮询。
- 未变化契约：无数据库迁移、无 Compose 端口变化、无 Agent/API 协议变化、无 `System Center` 页面/路由/API/数据/文档/写入变化；应用市场标准更新入口不变。KPanel 本版受管 `kejilion.sh` 镜像标记仍为 `2ee9856c9916b7ede8bbc19edc97e22872e86203`；独立 `kejilion/sh` 主线已另行发布 `4388c183681951d3df7456b6e301f206d7ea40c1`，不属于本 KPanel tag 的内置脚本变更。
- 风险等级及理由：中等兼容性风险。变更集中在前端配对状态确认和轮询生命周期，未改变后端协议或数据结构；已通过固定 L3、候选/main CI、公开镜像 E2E、Release/OCI 和生产三阶段证据。

## 发布范围与未纳入内容

- 用户可见更新：轻量节点配对窗口在检测到新节点实际注册前保持等待；匹配到新节点后才允许完成并给出成功反馈；轮询在超时、关闭和卸载时清理。
- 精确提交清单：`2c5c1aabb2c0818c05f99ef37298b9fc99ec42d0`（轻量节点配对连接确认及中英文资源/测试）；`650f4db432d09f2251f8a03b6ac554d84e31d23b`（v1.3.1 版本与发布准备）。候选相对 `origin/main@a3a82cd9115fdac270ffe3c1ad6cae355f43de3a` 的变更仅涉及 CHANGELOG、版本元数据、ClusterView 及其测试/i18n。
- 明确未纳入的分支、文件或后续事项：所有 `System Center` 相关内容、未形成干净候选的其他工作树、轻量节点 SSH 终端后续改动、浏览器人工矩阵、云存储及与本版无关的界面候选均未纳入；`kejilion.sh` 的独立候选已在其仓库单独发布，不将其改动伪装进 KPanel tag。未连接或操作 `108` / `prod-108`。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | ClusterView 定向测试 27 项通过；固定 L3、公开镜像 E2E 和生产 postdeploy 均通过 | 未做长期多节点并发 soak |
| 网络入侵与供应链安全 | 已验证 | 固定 Runner 的 govulncheck、npm audit、Trivy 源码/依赖/secret/config/镜像扫描通过；Release/OCI 契约通过 | 未进行独立第三方渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | Go/前端全量门禁、应用生命周期、备份、公开镜像 E2E 和生产 postdeploy 通过；回滚点明确 | 未执行受控生产回滚演练 |
| 性能与资源预算 | 已验证 | L3 runtime contract 和生产容器健康/资源证据通过；轮询为窗口生命周期内的 2 秒间隔请求 | 未做长时间压力或多节点吞吐基线 |
| 用户体验与可访问性 | 已实现未实机验证 | 定向/全量前端测试、typecheck、build 和中英文资源检查通过 | 未执行完整浏览器视口、缩放、主题、键盘和人工可访问性矩阵 |
| 数据、配置与迁移 | 已验证 | 无 schema migration；生产 protected config、SQLite quick_check、旧镜像和数据备份校验通过 | 未执行实际回滚，因此回滚结果为预案/备份证据而非演练证据 |

## 自动门禁

- 定向测试及结果：ClusterView 定向测试 `27/27` 通过；全量前端 `133/133` 测试文件、`1133/1133` 测试通过；typecheck、`i18n:check`（2133 个本地化短语、21 个 lazy catalogs）和 build 通过。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24` 内通过；Go 全量测试、前端测试/typecheck/build、应用生命周期、运行时契约、依赖和安全检查均通过。Windows 本地缺少 docker/go/gofmt/make，仅作为环境预检，不替代固定 Linux 结果。
- L3 外层入口：run=`v1.3.1-650f4db-l3-r2`，candidate=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，base main=`a3a82cd9115fdac270ffe3c1ad6cae355f43de3a`，base tag=`v1.3.0`，状态 `passed`、exit 0；远端目标仅 `arena-154`，证据目录 `/root/kpanel-release-evidence/v1.3.1-650f4db-l3-r2`，本地证据目录 `C:\GitHub\_release-artifacts\v1.3.1-650f4db-l3-r2`。
- L3 证据包：manifest SHA-256=`a5c1cc97c05e778afa9d293cf794255991acf957ef4c6b9f8fddefcc908a7484`；bundle SHA-256=`5792bfa9554d047ec544728ea0c36fdb279e29986cb3f96f3579f5c48e1baa64`；plan SHA-256=`116bbd0239e06658ba323ac8f69f0b2a7890385e482c2e834c282944971d276a`；远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；不可变 Runner ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`。
- 候选 CI：run `33902248663`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，分支 `release/v1.3.1-candidate`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33902248663。
- 候选 Dependency freshness：run `33902248552`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33902248552。
- 主线 CI：run `33902854817`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，分支 `main`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33902854817。
- 主线 Dependency freshness：run `33902854649`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，分支 `main`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33902854649。
- Tag Dependency freshness：run `33903352448`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，tag `v1.3.1`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33903352448。
- Release workflow：run `33903353404`，精确 SHA=`650f4db432d09f2251f8a03b6ac554d84e31d23b`，源码校验、Go/Node/security、原生构建、多架构推送、`latest` 晋级、公开 Release 和候选分支清理全部 completed successfully；https://github.com/kejilion/KPanel/actions/runs/33903353404。
- 安全扫描、镜像契约、SBOM/provenance：L3 的 govulncheck、npm audit、Trivy 源码/配置/secret/镜像扫描、CGO-free amd64/arm64 构建和应用契约通过；OCI 保留 provenance/SBOM attestation，`unknown/unknown` 条目不影响两个目标架构。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、主线和 tag 上 Dependency freshness 均以最终产品 SHA 通过；依赖检测源和锁文件完整性通过。
- 最近每日安全通告审计、EOL 复核状态及证据：本版未新增独立人工安全通告/EOL 审计；不把自动扫描结果冒充人工审计，发布仍由固定 L3、CI 和 Release 安全门禁覆盖。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版无新增直接依赖、基座镜像或 Action；未产生待处置行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：沿用仓库锁定依赖与既有发布链；L3 Runner 使用 Go 1.26.7、Node 24；KPanel 镜像内受管脚本仍为 `2ee9856c9916b7ede8bbc19edc97e22872e86203`。独立 `kejilion/sh` 主线发布 `4388c183681951d3df7456b6e301f206d7ea40c1`，不涉及 KPanel 版本号。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：KPanel 产品版本与前端 package metadata 为 `1.3.1`；OCI index=`sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`；linux/amd64=`sha256:53f659d9927af00bd4d7639306c45f7f75552e55b32645e13b9aa1e50f0159db`；linux/arm64=`sha256:8917a7a111703be752aebe4f270d1b426182c0d78d8851cb853105d9f24e3804`；独立脚本提交为 `4388c183681951d3df7456b6e301f206d7ea40c1`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：System Center 范围、未形成干净候选的界面/终端/专项工作树继续暂缓；待形成干净提交、独立复核和适用 L3 后再评估。
- 升级后的兼容、安全、构建、性能资源和回滚结论：固定 L3、候选/main/tag CI、Dependency freshness、Release/OCI、公开镜像 E2E 以及 `arena-154` 三阶段证据通过；版本可按 `v1.3.0` 备份和 OCI 精确回滚。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux/Debian 13，Docker 29.6.2，x86_64；L3 使用固定 `kpanel-release-gate:go1.26.7-node24` Runner。
- 环境策略 ID 与允许用途：`environment-policy.json` schemaVersion 1；`arena-154` 允许 production-deploy、production-safety-check 及候选/浏览器/性能验证；`prod-108` disabled 且无允许用途。
- 使用的精确候选或公开产物：KPanel 候选完整 SHA `650f4db432d09f2251f8a03b6ac554d84e31d23b`、tag `v1.3.1`、公开 OCI index `sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v1.3.1-650f4db-l3-r2` passed/exit 0，证据目录 `/root/kpanel-release-evidence/v1.3.1-650f4db-l3-r2`，L3 bundle/plan/remote script 摘要见“自动门禁”；公开镜像脚本在 `arena-154` 临时端口 18131 执行并输出 `image_e2e=pass`，临时容器、网络和文件已清理。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：L3、公开镜像 E2E 和生产 postdeploy 为单次固定门禁与短时健康核对；长期多节点 soak 不适用本次配对状态兼容性范围，作为后续观察项。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动前端测试覆盖配对等待、匹配、超时/关闭/卸载清理及中英文资源；完整浏览器视口、缩放、主题、键盘人工矩阵未执行，状态为已实现未实机验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：公开镜像 E2E 仅使用临时容器；生产 backup、标准更新、postdeploy 通过；未执行生产失败注入和实际回滚，保留 `v1.3.0` 作为回滚点。
- 未执行场景及原因：长期 soak、第三方渗透、完整浏览器人工矩阵、真实多节点并发配对和受控生产回滚演练未执行；不把这些场景的缺失写成已验证。

## 发布产物与公开仓库复核

- GitHub Release：公开、非 draft、非 prerelease，`v1.3.1`；https://github.com/kejilion/KPanel/releases/tag/v1.3.1。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:1.3.1` 与 `:latest` 均为 `sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:53f659d9927af00bd4d7639306c45f7f75552e55b32645e13b9aa1e50f0159db`；arm64=`sha256:8917a7a111703be752aebe4f270d1b426182c0d78d8851cb853105d9f24e3804`。
- 附件及 `SHA256SUMS`：公开 Release 附件包含 `kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-1.3.1.tar.gz`、`SHA256SUMS`、`LICENSE` 和 `THIRD_PARTY_NOTICES.md`。
- 公开镜像 `image_e2e=pass`：在 `arena-154` 显式拉取 `docker.io/kjlion/kejilion-panel:1.3.1`，拉取 digest 为 `sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`，运行仓库脚本 `packaging/tests/image-e2e.sh` 输出 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel `packaging/kejilion-app/kpanel.conf` 与 `kejilion/apps` 当前远端/工作文件 blob 均为 `abf0efd22876f34aa3731f5b6d8ba04e373b965e`，本版未变，无需应用市场提交；`kejilion/sh` 主线已精确到 `4388c183681951d3df7456b6e301f206d7ea40c1`，无版本号变更。`sh` 仓库公开 workflow 无 push CI，因此以本地 Bash 语法/同步/轻量节点 smoke 通过和远端 main 精确引用为证，不虚构 CI run。

## 生产部署安全核对

- 生产目标和部署授权范围：本次唯一生产目标为 `arena-154`，用户明确授权发布、标准应用市场更新及完成后让电脑睡眠；仅执行 KPanel 标准更新入口。
- 验证/灰度环境：`environment-policy.json` 中仅使用允许的 `arena-154`；未使用 `prod-108`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight run=`v1.3.1-production-20260905`，expected version=`1.3.0`，passed/exit 0，证据目录 `C:\GitHub\_release-artifacts\v1.3.1-production-20260905-preflight`，远端 `/root/kpanel-release-evidence/v1.3.1-production-20260905/production-preflight`；生产 Panel/Agent healthy，旧 revision=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，旧 OCI digest=`sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；应用市场更新成功并拉取公开 `latest`，digest 与本版 OCI index 一致。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run=`v1.3.1-production-20260905`，expected version=`1.3.1`、revision=`650f4db432d09f2251f8a03b6ac554d84e31d23b`、image digest=`sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`，passed/exit 0，证据目录 `C:\GitHub\_release-artifacts\v1.3.1-production-20260905-postdeploy`，远端 `/root/kpanel-release-evidence/v1.3.1-production-20260905/production-postdeploy`；Panel `running/healthy`，health `status=ok`、version=`1.3.1`，Agent `active/running/enabled`，restart=0，OOM=false，近 10 分钟无 fatal/panic/OOM signature，SQLite quick_check 和 protected config 校验通过；公网入口沿用既有域名/端口契约。
- 生产已执行写操作：应用市场更新、公开 OCI 拉取、容器重建/启动和标准 postdeploy 取证；未执行 System Center、危险系统资源写入、失败注入或回滚。
- 仅在隔离真机执行、未在生产执行的场景：长期压力、完整浏览器人工矩阵、第三方渗透、真实多节点并发专项和实际回滚演练。

## 回滚

- 源码/tag：`v1.3.0` / `8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`。
- 镜像 digest：上一稳定 OCI index `sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.3.1-20260904T180840Z`，含旧镜像、Agent unit、`kpanel.conf`、Panel 数据归档、旧镜像归档、inspect 和 `SHA256SUMS`，备份门禁校验全部 OK。
- 回滚步骤和回滚后复核：按应用市场规范恢复 `v1.3.0` 对应 OCI、Compose、`.env`、Panel 数据、Agent unit 和受管脚本，再以 production evidence 核对 health、Agent、digest、restart/OOM、数据和日志；禁止只切换浮动 `latest`。本次未执行实际回滚。
- 回滚后生产实际版本与健康状态：未执行，当前生产保持 v1.3.1 healthy。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release `v1.3.1` 已公开；Docker `latest` 与 `1.3.1` OCI index digest 一致；标准 `k app kpanel` 更新入口已拉取并运行本版。
- 公共默认更新通道决策：保留 v1.3.1 作为当前稳定默认版本；若出现退化按精确 v1.3.0 digest 与本次备份回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-05T01:24:14+08:00
- 候选冻结时间：2026-09-05T01:28:45+08:00
- 生产完成时间：2026-09-05T02:09:41+08:00
- 提交到生产用时：0.76 小时
- 是否回滚、紧急热修复或重复发布：否
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
    "fingerprint": "l3/run-release-l3/local-tag-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次 L3 尝试在共享候选工作树发现本地 v0.86.2 tag 与 origin 不一致，按规范 fail-closed 停止；没有上传候选、运行候选门禁或生产写入。",
    "recoveryEvidence": "改用全新 no-tags release clone，校验 required tags 与 origin 一致后，以同一最终候选 SHA 执行 v1.3.1-650f4db-l3-r2 并通过；证据目录为 /root/kpanel-release-evidence/v1.3.1-650f4db-l3-r2。",
    "permanentAction": "本次及后续 L3 固定使用全新 release clone，先精确抓取并校验 required tags、origin/main、候选 SHA 和基线，再运行唯一 L3 入口；禁止在共享管理树覆盖历史 tag。",
    "historicalReleases": ["v1.3.0", "v1.2.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

该流程异常发生在首次生产写操作前，被本地 L3 前置校验拦截；恢复后的固定 Linux L3、候选 CI、主线 CI、Release 和生产证据均以最终 SHA 通过，不计为产品变更失败。Windows 本地缺少发布工具的预检未替代固定 Runner，也未作为生产证据。

## 遗留风险与后续准入

- 未验证风险：完整浏览器人工矩阵、真实多节点并发配对、长期 soak、独立第三方渗透和受控生产回滚演练未执行。
- 已实现待实机准入：不同视口/缩放/主题/键盘条件下的配对等待与失败态、两台真实 KPanel 间的连续配对操作，以及 KPanel 镜像内置 `kejilion.sh` revision 从 `2ee9856` 对齐到独立主线 `4388c183` 的后续候选评估。
- 不阻断本版的理由：本版 UI 变更边界清晰，无 System Center 范围；固定 L3、候选/main/tag CI、Dependency freshness、Release/OCI、公开镜像 E2E、备份和 `arena-154` 三阶段生产证据均以最终 SHA 通过，当前 Panel/Agent healthy。
- 后续应进入的自动门禁或专项工作流：后台浏览器验证、真实多节点配对并发/失败恢复专项、发布回滚演练；若要求 KPanel 镜像同步 `kejilion/sh` 的 `4388c183`，另行形成独立 KPanel 候选并重新执行完整发布门禁。

生产结论：`v1.3.1` 已在唯一授权目标 `arena-154` 完成上线；`System Center` 保持排除，`108`/`prod-108` 未连接。独立 `kejilion.sh` 主线已发布到 `4388c183681951d3df7456b6e301f206d7ea40c1`，无版本号变更。
