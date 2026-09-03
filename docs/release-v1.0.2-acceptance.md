# KPanel v1.0.2 发布验收记录

日期：2026-09-03

发布级别：L3

候选提交 / 标签：`52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d` / `v1.0.2`

上一稳定版本 / 回滚点：`v1.0.1` / `0e9020d39f585be8a79fb3d5238fe7599bb9dab1`

## 发布画像

- 业务域：集群主机卡片、公开集群分享页和 KPanel 左侧导航的 Web 展示。
- 变更面：展示、i18n 和回归测试；不涉及宿主机写入、协议、数据或部署逻辑。
- 受影响用户旅程：查看集群主机/公开分享流量，以及在深色或浅色主题下滚动左侧导航。
- 未变化契约：API、数据、端口、Compose、Agent 权限、`kejilion.sh`、应用市场和 System Center。
- 风险等级及理由：低；差异仅为 Web 结构、语义标签、样式和版本元数据，L3、CI、镜像契约与生产健康检查均通过。

## 发布范围与未纳入内容

- 用户可见更新：集群流量用箭头与数值呈现，实时/累计流量在主机卡片和公开分享页采用一致结构并降低窄布局高度；侧边导航滚动条使用侧栏主题语义色。
- 精确提交清单：`f238ed65`（源分支 `eafc957`，主机流量展示）、`08695a0`（源分支 `e18b91a`，公开分享页高度）、`d636306`（源分支 `3a07fed`，流量结构统一）、`5d89635`（源分支 `8a99bdc`，侧栏滚动条）、`52f9f4d`（版本元数据）。
- 明确未纳入的分支、文件或后续事项：`361ad1e` + `40e9b9b` 的集群文件页主机入口/远端跳转未纳入；它尚未实现同页跨主机文件互传协议，应在完成权限、传输、回归和 L3 后进入后续功能版本。集群 SSH 登录通知的 KPanel 与 `kejilion.sh` 工作树仍有未提交/未跟踪改动，且涉及 `System Center` 范围，未纳入。Claude 宽泛旧视觉分支、已与主线等价的重复提交和所有 System Center 改动均未纳入。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Web 全量测试、L3、生产标准更新和精确版本健康检查通过；本版无 API 或双端协议变化。 | 真实用户多浏览器长期使用未做 soak。 |
| 网络入侵与供应链安全 | 已验证 | L3 `govulncheck`、`npm audit`、Trivy 源码/镜像扫描和 runtime contract 通过。 | 未新增网络入口；独立 `packaging/tests/image-e2e.sh` 未单独执行。 |
| 稳定性、失败恢复与兼容 | 已验证 | Go/Web 全量测试、核心 race、双架构构建、应用生命周期和生产 backup/postdeploy 通过。 | 未执行受控生产回滚演练。 |
| 性能与资源预算 | 已验证 | 变更无常驻任务、数据迁移或后端轮询；L3 容器契约和生产健康状态通过。 | 未做长期 soak 或完整多标签页压力测试。 |
| 用户体验与可访问性 | 已验证 | 目标布局测试、英文/繁中 i18n 检查、局部浏览器主题/滚动条检查和精确候选构建通过；流量值补充语义标签。 | 真实浏览器 100%/125%/200% 缩放、键盘焦点和完整三语人工矩阵未执行。 |
| 数据、配置与迁移 | 已验证 | 无数据库迁移、配置格式、Compose、Agent 或受管脚本变化；生产备份校验通过。 | 不适用新增迁移。 |

## 自动门禁

- 定向测试及结果：Web 目标回归包含集群流量/分享页和 `AppShell.layout.test.ts`；候选精确 SHA 全量 `npm test` 为 131 个测试文件、1108 个测试通过，`npm run typecheck`、`npm run build`、`npm run i18n:check`（2107 个短语、21 个目录）通过。
- `make verify-release` 环境和结果：固定 Linux L3 `v1.0.2-52f9f4d-l3-r3` 状态 `passed`、exit `0`；Windows 本地完整入口在工具预检处按设计停止（缺少 `go`、`docker`、`gofmt`、`make`），不将其冒充 Linux release 通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run=`v1.0.2-52f9f4d-l3-r3`，candidate=`52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`，base main=`bfd8a5cedbe81fdb0dba6c37f5ed392a1cc253a8`，base tag=`v1.0.1`，business baseline=`v0.100.0` / `89a384c7d65c42b14222dcace8843ff23602dc11`；Runner=`kpanel-release-gate:go1.26.7-node24`，immutable ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle SHA-256=`56186103beaf59614b57988f2d5983886ce6ed8847261e98e8c8ba01625c7592`，plan SHA-256=`1289e8730c78c006856ef2050e5edf486fdf53b286631b51bd0cddcae9ede13f`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；远端证据=`/root/kpanel-release-evidence/v1.0.2-52f9f4d-l3-r3`。
- 候选 CI：GitHub Actions run `33767868868`，精确 `head_sha=52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`，结论 `success`；[`候选 CI`](https://github.com/kejilion/KPanel/actions/runs/33767868868)。
- 主线 CI：run `33770064084` / job `100697743611`，精确 `head_sha=52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`，结论 `success`；[`main CI`](https://github.com/kejilion/KPanel/actions/runs/33770064084)。
- Release workflow：run `33772409661`，状态 `completed/success`；完成公开 Release、双架构 OCI、`latest` promotion 和候选分支清理；[`Release workflow`](https://github.com/kejilion/KPanel/actions/runs/33772409661)。
- 安全扫描、镜像契约、SBOM/provenance：L3 与 Release 均通过 Go/Web 测试、race、vet、govulncheck、npm audit、Trivy 源码/镜像扫描、双架构构建、非 root/healthcheck/资源约束、受管脚本校验和镜像运行契约；正式多架构发布带 provenance/SBOM。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选本地补充报告为 2026-09-03，因 Windows 本机 Go/Docker/安全数据源不可用仅 `4/8`；它不作为通过依据。同 SHA 的 GitHub Dependency freshness workflow `33767868810`（候选）与 `33770064023`（main）均 `completed/success`。
- 最近每日安全通告审计、EOL 复核状态及证据：Release/L3 的 `govulncheck`、`npm audit --audit-level=high` 和 Trivy 扫描通过；未因本版差异新增依赖或 EOL 项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：未引入新的直接依赖；本地 freshness 报告列出 3 个 GitHub Actions 小版本候选及受管 `kejilion.sh` 工具链候选，未在本补丁中擅自升级，后续按依赖维护窗口处理。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go `1.26.7`、Node `24.20.0`、固定 L3 Runner `kpanel-release-gate:go1.26.7-node24`；受管脚本 revision=`6e65c0cd7028cb198efb0c88a57726713ee1b23b`，SHA-256=`48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本和锁文件统一为 `1.0.2`；正式 OCI index=`sha256:d6dec696d803632f3d791af9860e72648e24e2f957b428f63207bbc1d8017b0a`；amd64=`sha256:ca0df96fff12af629355f4bcd316b4b52f958f34b686906122d6a18e69bf1aa5`，arm64=`sha256:0ec7f6890527129a73ad1d55455518aee35aa4755c0aa2720b75072b7125f95c`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：文件管理互传与 SSH 通知候选暂缓；负责人为对应功能任务维护者，下一候选冻结前复核。退出条件分别为：文件互传形成权限明确的协议/实现并完成候选 CI、L3、`arena-154` 验收；SSH 通知形成 KPanel 与 `kejilion.sh` 的同步干净提交，移除或隔离 System Center 范围，并补真实 Telegram/SSH 登录证据。
- 升级后的兼容、安全、构建、性能资源和回滚结论：本版只改变 Web 展示与 i18n，兼容现有 API、数据和应用市场更新入口；构建、扫描、运行契约和生产健康通过；可回滚到 `v1.0.1` 并使用本次备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux，标准 Docker Compose/`kejilion-agent`；固定 L3 Runner 为 `go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `hybrid` / `candidate-validation`（L3）及 `production-safety-check`、`production-deploy`（生产证据）。
- 使用的精确候选或公开产物：源码/tag `52f9f4d` / `v1.0.2`；生产拉取 `docker.io/kjlion/kejilion-panel:latest`，digest=`sha256:d6dec696d803632f3d791af9860e72648e24e2f957b428f63207bbc1d8017b0a`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3=`v1.0.2-52f9f4d-l3-r3` / passed / `0` / 未超时，远端=`/root/kpanel-release-evidence/v1.0.2-52f9f4d-l3-r3`；生产三阶段 run=`v1.0.2-52f9f4d-prod-r1`，preflight=`passed/0`、backup=`passed/0`、postdeploy=`passed/0`，远端=`/root/kpanel-release-evidence/v1.0.2-52f9f4d-prod-r1/production-{preflight,backup,postdeploy}`；三阶段远端脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`；计划 SHA-256 分别为 preflight=`9d7a33f8b48e20942de8250432769fab7e4e219d6277183f2bdaec3497e48c69`、backup=`5ddfa0fe0987c4668a7bdabcf02736151a6c36df7267e8b150d1f8c339362fbf`、postdeploy=`5003bef1e2f2681d1e27e529dfa8015bb35fb9078aeef18951f40362a189db3c`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：L3 全量门禁和单次生产 postdeploy；无长期 soak，因本版无常驻新任务且变更为展示层。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：局部浏览器检查覆盖侧栏滚动、深色/浅色主题和 1280x680 视口；自动 i18n 覆盖 21 个目录。100%/125%/200% 缩放、最小计算字号、完整键盘/焦点和三语人工矩阵未执行；对应失败态由自动测试和 L3 覆盖，未将模拟预览当作生产证据。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 阶段生成并校验停写备份；标准应用市场更新成功；postdeploy 通过版本、revision、digest、健康、Agent、日志和配置一致性检查；未执行正式回滚。
- 未执行场景及原因：真实浏览器完整缩放/语言/键盘矩阵、长期 soak、生产故障注入、受控回滚和独立 `packaging/tests/image-e2e.sh` 未执行；它们不改变本次代码无迁移/无新宿主机协议的事实，列为后续专项。

## 发布产物与公开仓库复核

- GitHub Release：[`v1.0.2`](https://github.com/kejilion/KPanel/releases/tag/v1.0.2)，`draft=false`、`prerelease=false`、`target=main`，Release ID=`382134400`，published=`2026-09-03T15:32:18Z`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:1.0.2` 与 `:latest` 均为 `sha256:d6dec696d803632f3d791af9860e72648e24e2f957b428f63207bbc1d8017b0a`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:ca0df96fff12af629355f4bcd316b4b52f958f34b686906122d6a18e69bf1aa5`；arm64=`sha256:0ec7f6890527129a73ad1d55455518aee35aa4755c0aa2720b75072b7125f95c`。
- 附件及 `SHA256SUMS`：Release workflow 已生成 Agent amd64/arm64、轻量节点 amd64/arm64、部署归档和 `SHA256SUMS` 并上传；下载后以附件校验和为准。
- 公开镜像 `image_e2e=pass`：Release workflow 的 native image scan/runtime contract 通过；生产标准入口实际拉取上述 `latest` digest 并由 postdeploy 复核通过。独立 `packaging/tests/image-e2e.sh` 未单独执行。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场源同步成功，标准入口 `/home/docker/kpanel/bin/kejilion.sh app kpanel` 成功拉取 `latest`、重建并启动 `kejilion-panel`；本版未修改 `kejilion.sh`，受管脚本契约保持一致。

## 生产部署安全核对

- 生产目标和部署授权范围：用户授权本次完整发布；唯一目标为 `arena-154`。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：L3 固定 Runner 的 `arena-154` candidate-validation；未连接其他 KPanel 主机。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight run=`v1.0.2-52f9f4d-prod-r1`，以 `EXPECTED_VERSION=1.0.1` 通过；Panel/Agent/容器健康，随后 backup 生成 `/root/kpanel-backups/pre-v1.0.2-20260903T153421Z`，备份文件 SHA256 校验通过。
- 部署命令/入口：`ssh arena-154 'env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel'`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run=`v1.0.2-52f9f4d-prod-r1` passed；health `status=ok version=1.0.2`，Panel running/healthy，Agent active/running/enabled，restart=`0`，OOM=`false`，revision=`52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`，RepoDigest 含正式 OCI digest；日志与配置一致性检查通过。
- 生产已执行写操作：仅在 `arena-154` 写入本次生产 evidence、停写备份，并通过标准应用市场事务更新 KPanel；未执行 System Center、SSH 通知、文件互传或其他主机写操作。
- 仅在隔离真机执行、未在生产执行的场景：L3 竞态/安全扫描、双架构构建、局部浏览器主题检查和未完成候选功能验证均未作为生产写入。

## 回滚

- 源码/tag：`v1.0.1` / `0e9020d39f585be8a79fb3d5238fe7599bb9dab1`。
- 镜像 digest：上一稳定 OCI index=`sha256:6a19324ffac3bd432ec564dd8e5ae094ab96c9d5dadb188e7acb7e7a058d7eea`；本次备份另含部署前运行镜像归档。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.0.2-20260903T153421Z`，包含应用归档、旧镜像、Compose/配置、Agent unit、inspect 和 SHA256SUMS。
- 回滚步骤和回滚后复核：按应用市场回滚规范停 Panel/Agent，恢复备份的 Compose、`.env`、数据、Agent 文件和旧镜像，恢复旧版受管脚本后使用标准入口启动；再用 production evidence postdeploy 核对 `v1.0.1`、旧 digest、健康、Agent、restart/OOM、数据和日志。禁止只切换浮动 `latest`。
- 回滚后生产实际版本与健康状态：未执行回滚；当前 `v1.0.2` postdeploy 健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest 为 `v1.0.2`；Docker `latest` 与 `1.0.2` 同 OCI index=`sha256:d6dec696d803632f3d791af9860e72648e24e2f957b428f63207bbc1d8017b0a`；标准入口已在 `arena-154` 拉取并运行该 digest。
- 公共默认更新通道决策：保留 `v1.0.2` 为默认稳定通道；未发现需要恢复上一稳定版本的产品问题。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-03T22:29:00+08:00
- 候选冻结时间：2026-09-03T22:34:36+08:00
- 生产完成时间：2026-09-03T23:36:28+08:00
- 提交到生产用时：1.12 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/local-tags/stale-tag-object",
    "position": "before-production-write",
    "count": 1,
    "impact": "共享工作树的本地 v0.86.2 tag 与 origin 对象不一致，L3 prepare-only 按设计 fail-closed，未上传候选 bundle、未执行远端门禁、未产生生产写入。",
    "recoveryEvidence": "v1.0.2-52f9f4d-l3-r2 在本地 clean-clone prepare-only 通过；随后用同步远端 tags 的 clean clone 生成 v1.0.2-52f9f4d-l3-r3，arena-154 status.txt 为 passed/exit_code=0。",
    "permanentAction": "该指纹已在 v1.0.0、v1.0.1 出现；本次使用 clean clone 是受控恢复，不视为永久修复。下一候选冻结前必须把 clean-clone 创建、tag equality preflight 和唯一 L3 入口固化并回归；退出条件是从含陈旧/冲突本地 tag 的工作树启动时自动生成可验证 clean clone，无人工绕行即可完成 tag/commit 双校验。",
    "historicalReleases": ["v1.0.0", "v1.0.1"]
  },
  {
    "fingerprint": "verification/windows/missing-linux-tools",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 本地 release 级 verify-change 在 preflight 因缺少 go、docker、gofmt、make 停止，未生成通过证据且未触及远端或生产。",
    "recoveryEvidence": "固定 Linux Runner 的 v1.0.2-52f9f4d-l3-r3 完整通过，并由候选/main CI 与 Release workflow 对同一 SHA 成功复核。",
    "permanentAction": "发布流程继续将 Windows 结果限制为本地预检，不把它替代 Linux L3；后续维护 Windows 预检提示和固定 Linux Runner 的唯一入口。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：真实浏览器完整缩放/键盘焦点/三语人工矩阵、长期 soak、生产故障注入、受控回滚和独立 `packaging/tests/image-e2e.sh` 未执行；L3 clean-clone stale-tag 指纹已重复，下一版本生产写前必须先完成唯一入口修复。
- 已实现待实机准入：集群文件管理互传的当前提交只提供安全入口和远端 Files 跳转，真正跨主机传输待协议/权限/实机验证；集群 SSH 登录通知待 KPanel 与 `kejilion.sh` 干净同步提交、System Center 范围处理和真实 Telegram/SSH 验收。
- 不阻断本版的理由：本版精确差异为 Web 展示/i18n/测试和版本元数据，无新宿主机协议或数据迁移；候选/main CI、Dependency freshness、固定 L3、Release/OCI、`arena-154` preflight/backup/postdeploy 均绑定精确 SHA 并通过，生产未发生退化、回滚或紧急热修复。
- 后续应进入的自动门禁或专项工作流：修复并回归 L3 clean-clone 唯一入口；完成文件互传和 SSH 通知的 focused commit、独立审查、候选 CI/L3 与 `arena-154` 证据；补真实浏览器缩放/焦点/语言矩阵、长期 soak、受控回滚和独立 `image-e2e`。
