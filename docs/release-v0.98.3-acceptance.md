# KPanel v0.98.3 发布验收记录

日期：2026-08-28

发布级别：L3

候选提交 / 标签：`47debf1b2a98b418906a81c911280db3db05dbd7` / `v0.98.3`

上一稳定版本 / 回滚点：`v0.98.2` / `sha256:f894543eee6c8999824d43357a6dfa12421912c93d3fc9a66628a5673b3abeb6`

## 发布画像

- 业务域：KPanel 依赖、工具链、发布治理与兼容性维护。
- 变更面：构建与依赖、CI/CD 及版本元数据；正式部署仅执行受管应用更新，不新增宿主机功能写入、API 或数据协议。
- 受影响用户旅程：通过标准应用市场更新拉取 `latest`，启动 KPanel Panel/Agent，并使用现有配置和数据继续运行。
- 未变化契约：API / 数据 / 端口 / Compose / Agent 权限 / `kejilion.sh` / 应用市场安装契约。
- 风险等级及理由：中；包含 TypeScript 6、Vue Router 5、Node 24 类型和 GitHub Actions v7 兼容升级，但固定 Linux L3、公开镜像 E2E、备份恢复和生产 postdeploy 均通过。

## 发布范围与未纳入内容

- 用户可见更新：依赖、工具链和发布治理维护；无新的业务功能或 API 变更。
- 精确提交清单（相对冻结基线 `e3b7cf7f780021334bfcda35a1bc9be0c3c90d65`）：`8955a6395a8847caa044a7722ef583e8c9b340e5`、`b1947aeda67708250f75556ca040e1bea96554f8`、`b0339719ce85086e3380477debcb86bbb423c31a`、`85d43f230308c88726458e66334c663ed57aff3e`、`f81c9fb080ab1ccb024ac5d3aeccc67de65f42c6`、`b22e6ffc208b85af94009b00412e5bb2650b5e86`、`47debf1b2a98b418906a81c911280db3db05dbd7`。
- 明确未纳入的分支、文件或后续事项：系统中心（System Center）文件、路由、API 和文档未纳入；共享脏 `main` 工作树未触碰；`kejilion/apps` 未产生提交；`kejilion.sh` 未改；`prod-108`/`108` 未连接；TypeScript 7、Node 26 类型及其他未完成兼容评估的候选未纳入。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 固定 Linux L3 的 Go 全量测试、Web 122 个测试文件/1044 项、typecheck/build、公开镜像 bootstrap E2E、生产 Panel/Agent 健康；本版无新后端协议 | 未新增业务 API 或 Agent 互通路径 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit 0、Trivy 源码/镜像扫描 0；双架构 OCI revision/version、Release 附件 `SHA256SUMS` 和 provenance/SBOM attestation 复核通过 | Trivy 输出提示有新版本可用，但不影响本次扫描结果 |
| 稳定性、失败恢复与兼容 | 已验证 | 候选/main CI、L3、Release、公开镜像 E2E、生产停写备份、标准更新和 postdeploy 全通过；容器 healthy、OOM=false | 无长时间 soak；依赖升级仍需后续真实使用观察 |
| 性能与资源预算 | 已验证 | L3 受限容器运行契约和生产资源/健康检查通过；未新增常驻运行时任务 | 未执行独立压测或长时间容量基准 |
| 用户体验与可访问性 | 不适用 | 本版无 UI 业务交互或视觉设计变更，L3 覆盖构建和运行契约 | 未执行真实生产登录态、浏览器视口/缩放/主题/键盘专项验收，不将其冒充为已验证 |
| 数据、配置与迁移 | 已验证 | 生产备份归档和校验通过，`protected.diff=0`，SQLite 检查通过；无 schema 或配置迁移 | 未执行回滚；回滚材料已保留 |

## 自动门禁

- 定向测试及结果：L3 首次运行通过；Web 122 个测试文件/1044 项、i18n 2582 条/21 个 catalog、typecheck、Vite build、Go 全量、核心 race、govulncheck、npm audit、Trivy、安装安全和应用配置生命周期均通过。Windows 本地 `verify-release` 后置补验按设计 fail-closed，缺少 `docker/go/gofmt/make`，未冒充全量通过。
- `make verify-release` 环境和结果：`arena-154` Debian 13 / Linux `6.12.96+deb13-amd64`，固定 Runner image `kpanel-release-gate:go1.26.6-node24`，Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.98.3-47debf1-l3-r1`，exit 0，远端证据 `/root/kpanel-release-evidence/v0.98.3-47debf1-l3-r1`，本地归档 `C:\GitHub\_release-artifacts\v0.98.3-47debf1-l3-r1`。bundle SHA-256=`5b7394e7477ab7850d5aa6c60345366ce4350ba920d1b37d2100dde4ae7ca9f7`；manifest=`7eeebf2a049e54e93bb8c5e8ec21fc1472ff15309a20d0e387f4834a9b1e82df`；plan=`ab30e3841aa0a113ee6ed227d7f8c5a259cd2935c217535a4726a3ac5b625ca0`；remote script=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；L3 log=`4224fc56fff5cf0e7254423b39a04ed2aec21744ee1cdd04b7c6d7e950e1323c`。
- 候选 CI：CI run `33096172820`、Dependency freshness `33096173485`，均绑定 `47debf1b2a98b418906a81c911280db3db05dbd7`，completed/success。
- 主线 CI：CI run `33096644609`、Dependency freshness `33096644570`，均绑定同一 SHA，completed/success；发布前 `main` 未漂移。
- Release workflow：`33098309617` completed/success；Tag Dependency freshness `33098309548` completed/success；Release 已公开，候选分支 API 返回 404，确认已自动清理。
- 安全扫描、镜像契约、SBOM/provenance：L3 和 Release workflow 的扫描、native image contract、双架构推送、latest promotion、SBOM/provenance 全部成功；L3 终态为 `status=passed`、`release_gate_runner=pass commit=47debf1b2a98b418906a81c911280db3db05dbd7`、`app_conf_lifecycle=pass`。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：实时报告 `2026-08-28T01:43:20.345+08:00`，检测源完整性 `4/8`，报告因本地缺少 `go` 和若干网络源失败而 exit 1；该结果作为数据缺口记录，不作“全部依赖最新”结论。L3 的 dependency policy validation 仍通过，候选与 Tag freshness workflow 均成功。
- 最近每日安全通告审计、EOL 复核状态及证据：L3 govulncheck/npm audit/Trivy 均通过；EOL 状态为 `current`，最近复核记录为 `2026-07-28`，下一截止为 `2026-10-28T23:59:59.999Z`，来源为 `docs/security-performance-hardening-2026-07-28.md`。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：报告记录 12 个直接/基座行动项、95 个传递依赖归属信号、0 个依赖例外；直接/传递依赖按拥有者和兼容范围处理，没有机械升级传递依赖。major-toolchain-base 最低验收为 L2-or-L3，兼容 patch 最低验收按影响范围执行。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：兼容前端补丁、TypeScript `6.0.3`、Vue Router `5.3.0`、Node types `24.13.3`、Vue/Vitest/vue-tsc/DOMPurify/CodeMirror 更新；GitHub Actions checkout/setup-go/setup-node 升级到 v7；未改变基础镜像或受管 `kejilion.sh`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：`VERSION`/`internal/version`/`web/package.json`/lockfile 为 `0.98.3`；checkout=`3d3c42e5aac5ba805825da76410c181273ba90b1`、setup-go=`b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`、setup-node=`820762786026740c76f36085b0efc47a31fe5020`、buildx=`d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5`、build-push=`f9f3042f7e2789586610d6e8b85c8f03e5195baf`、login=`abd2ef45e78c5afb21d64d4ca52ee8550d9572c7`；公开 OCI index=`sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`；受管脚本 revision=`9fec61b50cc6ef798dfac1edf11c2ec60ca6b0d1`、sha256=`54ceb0e72c4c342382500fc35da636fa436c484a12c4766fb9c7f806a23ae8fa`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：TypeScript 7、Node types 26 及报告中超出本版兼容边界的候选暂缓；证据为本版兼容范围、L3 和当前报告的数据缺口，负责人/复核日期未记录，退出条件为下一次独立 L2/L3 兼容性评估完成。
- 升级后的兼容、安全、构建、性能资源和回滚结论：固定 Runner、双架构构建/扫描、公开镜像 E2E、生产更新和 postdeploy 均通过；无 API、数据或权限迁移，回滚使用 `v0.98.2` OCI、Compose、`.env`、Agent 和本次备份成套恢复。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，hostname=`kejilion`，Debian 13，Linux `6.12.96+deb13-amd64`，x86_64，Docker `29.6.2`；L3 Runner 为 Go `1.26.6` / Node `24` 固定镜像。
- 环境策略 ID 与允许用途：`environment-policy.json` 的 `arena-154`，role=`hybrid`；用途通过 `production-safety-check` 与 `production-deploy`，未使用 `prod-108`。
- 使用的精确候选或公开产物：候选 SHA=`47debf1b2a98b418906a81c911280db3db05dbd7`；生产标准入口拉取 `docker.io/kjlion/kejilion-panel:latest`，其 digest 与 `0.98.3` 相同。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run=`v0.98.3-47debf1-l3-r1`，passed/0，无超时；生产 run=`v0.98.3-production-20260828`，preflight/backup/postdeploy 均 passed/0；生产证据本地目录为 `C:\GitHub\_release-artifacts\v0.98.3-production-20260828-{preflight,backup,postdeploy}`，远端固定入口脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。公开镜像 `image-e2e.sh` 为一次性 SSH 执行，无后台作业 ID，结果 `image_e2e=pass`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：公开镜像 E2E 一次完整运行；生产 preflight、backup、postdeploy 各一次；无 soak，原因是本版为依赖/工具链维护且无新增常驻任务。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：未执行浏览器专项验收；本版无 UI 业务旅程变更，不能把 L3 构建或 mock/截图证据解释为真实生产 UX。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 阶段受控 stop/start 并创建校验备份；标准更新入口 exit 0；postdeploy 确认 Panel/Agent healthy/active、restart=0、OOM=false、SQLite 通过；未执行生产回滚，失败注入只在 L3 应用生命周期负例中完成并按设计 fail-closed。
- 未执行场景及原因：未连接 `108`/`prod-108`；未执行真实生产登录、浏览器缩放矩阵和长期 soak；本版无需要生产业务写入的功能变更。

## 发布产物与公开仓库复核

- GitHub Release：[v0.98.3](https://github.com/kejilion/KPanel/releases/tag/v0.98.3)，Release ID=`378005184`，非 draft、非 prerelease，Release workflow=`33098309617`；annotated tag object=`6bd4ea33aea4f74d16b1f0e71ea648268b314c74`，peel=`47debf1b2a98b418906a81c911280db3db05dbd7`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.98.3` 与 `latest` 均为 `sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:feca87924b86445dede16d96dcdea0f689418f5d9c1671f30f4d73298cf97d77`；arm64=`sha256:d7e6612852848bfd02518e856b90ef5613a7e9b8f05e9960268d6ab2946f0c82`；两个 `unknown/unknown` manifest 为 provenance/SBOM attestation。
- 附件及 `SHA256SUMS`：Release 附件包含四个 amd64/arm64 Agent/Node 二进制、部署归档、`LICENSE`、`SHA256SUMS`、`THIRD_PARTY_NOTICES.md`；下载到 `C:\GitHub\_release-artifacts\kpanel-v0983-public-assets-20260828` 后，5 个带校验和对象逐一匹配：Agent amd64=`03671a60b8d3ea0580cad232fa5adec975bb7cd28960578b9c1ff05e07429672`、Agent arm64=`38120a5183b8833defa336586da5ff99451c536202dc9e9b7fe112b48d858c56`、Node amd64=`86d2e21765292a2cb78a6301c2880c8cb5b62f9685b81495010f25018d80301e`、Node arm64=`ff56b030cd4a58305c566ec9935f9158ff68ee8e291caa23548964bcf3d20af0`、deploy archive=`022504d2ac6af88aa7c49ff850be26f8abdaa890ea64657ed706dbc101c20aa7`。
- 公开镜像 `image_e2e=pass`：`arena-154` 显式拉取 `0.98.3` 后调用仓库固定 `packaging/tests/image-e2e.sh`，健康、Host/Forwarded-Proto、bootstrap、Secure cookie、单网络、受限容器和最终 healthcheck 全部通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 相对 `v0.98.2` 未变化；候选 blob=`abf0efd22876f34aa3731f5b6d8ba04e373b965e` 与 `kejilion/apps` `origin/main` 相同；标准更新期间 apps main 已是最新，未产生 apps 或 `kejilion.sh` 提交。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确授权本次复核通过后上线；仅操作 `arena-154`，范围为 KPanel v0.98.3 标准应用更新、生产证据和备份。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 的固定 L3 Runner、公开镜像 E2E 和生产安全证据入口。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight run=`v0.98.3-production-20260828`，版本 `0.98.2`、health `status=ok`；backup 同 run passed，备份 `/root/kpanel-backups/pre-v0.98.3-20260827T173808Z`，`kpanel.tar.zst`、旧镜像、`kpanel.conf`、Agent unit、inspect 文件及各自 `SHA256SUMS` 均校验 `OK`，`protected.diff=0`。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；exit 0，输出 `KPanel 更新完成 / Update Complete`、`KPANEL_PROGRESS 100`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run=`v0.98.3-production-20260828` passed；health=`0.98.3/status=ok`，Panel container=`running/healthy`，Agent=`active/running/enabled`，restart=0，OOM=false，SQLite=`ok`，`protected.diff=0`；Panel image label revision=`47debf1b2a98b418906a81c911280db3db05dbd7`、version=`0.98.3`、digest=`sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`；配置中的公网入口为 `https://kpanel.154.36.153.9.sslip.io`，固定 postdeploy 探针通过，未执行独立生产登录浏览器验收。
- 生产已执行写操作：backup 阶段受控 stop/start、创建 `/root/kpanel-backups/pre-v0.98.3-20260827T173808Z`；标准 `kejilion.sh app kpanel` 更新并重建 Panel/Agent；未执行业务数据写入。
- 仅在隔离真机执行、未在生产执行的场景：L3 完整测试、应用生命周期失败注入、公开镜像 bootstrap E2E 和容器安全约束在固定 Runner/隔离目录执行；生产只执行备份、标准更新和固定证据采集。

## 回滚

- 源码/tag：`v0.98.2`，产品 commit=`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`。
- 镜像 digest：`sha256:f894543eee6c8999824d43357a6dfa12421912c93d3fc9a66628a5673b3abeb6`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.98.3-20260827T173808Z`，包含旧镜像、Compose/`.env`、Agent unit、应用配置和数据归档；备份归档与校验和已通过。
- 回滚步骤和回滚后复核：停写后成套恢复 `v0.98.2` OCI、Compose、`.env`、Agent unit、apps 配置和数据，再按固定证据入口复核 health、Agent active、restart/OOM、SQLite、保护摘要和公网入口；不得只换镜像或只改版本字符串。
- 回滚后生产实际版本与健康状态：未执行生产回滚；当前 `0.98.3/status=ok`，Panel healthy，Agent active。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release v0.98.3 已公开；Docker `latest` 与 `0.98.3` 均指向 `sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`；标准入口实际拉取该 digest。
- 公共默认更新通道决策：不适用（本版正式发布成功，`latest` 保持指向 `0.98.3`，无已知 P0/P1/P2）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-27T22:28:23+08:00
- 候选冻结时间：2026-08-28T01:00:05+08:00
- 生产完成时间：2026-08-28T01:39:39+08:00
- 提交到生产用时：3.19 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：10
- 其中生产写操作开始后异常次数：2
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-operator/verification/wrong-working-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次版本一致性检查从 C:\\GitHub 执行，仓库包装入口找不到相对脚本；未改变代码、远端或生产状态。",
    "recoveryEvidence": "随后从候选根目录重跑，输出 Version metadata is consistent: 0.98.3，工作树干净。",
    "permanentAction": "所有仓库门禁调用在执行前固定校验候选根目录；下一次发布前退出条件为工作目录和脚本路径预检通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/scope-audit/broad-regex",
    "position": "before-production-write",
    "count": 1,
    "impact": "范围审计正则将 CHANGELOG.md 的文件名片段 log 误判为系统相关关键词，产生无效证据；未改变产品或远端。",
    "recoveryEvidence": "改为精确路径和路径段审计后确认无系统中心文件、路由、API 或文档进入候选差异。",
    "permanentAction": "范围审计使用显式路径集合和路径段匹配，并对 CHANGELOG 等文件名做固定回归；退出条件为误报不再阻断冻结。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/verification/runner-wrapper-argument-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "误以为仓库 Bash 适配入口支持 -c，命令被入口拒绝；未执行发布门禁或远程写入。",
    "recoveryEvidence": "按入口契约改用脚本参数，并以 Git for Windows Bash 对固定远端脚本完成语法检查，exit 0。",
    "permanentAction": "只使用 Makefile/AGENTS.md 规定的 wrapper 参数契约；发布前先执行固定脚本参数单测。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/orchestration/template-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次组合式安全/路径预检因嵌套 PowerShell/JavaScript 模板转义产生解析错误，未执行检查或远程写入。",
    "recoveryEvidence": "去除嵌套模板后，environment policy、run 目录碰撞和受管入口检查均通过。",
    "permanentAction": "复用固定脚本或静态参数，避免在发布编排中嵌套未预检模板；退出条件为预检命令先通过语法检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/production-evidence/missing-entrypoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "误调用候选版本不存在的 production-safety-check.mjs，命令 fail-closed；未上传计划或修改生产。",
    "recoveryEvidence": "按仓库实际脚本清单改用 check-environment-policy.mjs --purpose production-safety-check，随后 preflight/backup/postdeploy 均通过。",
    "permanentAction": "生产证据入口只从工作流、Makefile 和 scripts 清单读取，不猜测脚本名；发布前执行入口存在性检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/l3-evidence/copy-handoff",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 外层终端先交还控制，远端仍在执行；本地证据目录未及时出现，未重复启动 L3、未改变候选或生产。",
    "recoveryEvidence": "同一固定 run ID 的远端 status.txt 为 passed/0，随后从同一固定证据目录补取文件，本地/远端 bundle、plan、script、manifest 校验一致。",
    "permanentAction": "L3 外层入口需等待远端终态和完整证据复制后再返回；在修正前不得把手工补取当作流程已修复。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/public-audit/ssh-auth-denied",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选分支删除复核的本机 git ls-remote 因 SSH publickey 被拒，未改变 GitHub 或候选分支。",
    "recoveryEvidence": "改用 GitHub API 复核，release/v0.98.3-candidate 返回 404；Release workflow 终态 success。",
    "permanentAction": "公开仓库 ref 复核以 GitHub API 为权威并预检认证；SSH 只作为已验证的备用通道。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/public-audit/template-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次 GitHub/Docker 公开产物组合查询在本地 JavaScript 模板解析阶段失败，未发出查询或写入。",
    "recoveryEvidence": "随后以无嵌套模板的同一查询核验 Release、候选分支、Docker digest/platform，结果一致。",
    "permanentAction": "公开产物复核脚本固定为可独立执行的静态命令，并在发布前先做解析/语法检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/dependency-report/incomplete-sources",
    "position": "after-production-write",
    "count": 1,
    "impact": "生产 postdeploy 后的实时 dependency report 因本地 go 缺失及若干网络检测源失败，完整性为 4/8 并 exit 1；未将不完整报告当作全量结论，也未改变生产。",
    "recoveryEvidence": "报告保留 4/8 数据缺口；L3 dependency policy validation、候选/Tag freshness CI 均 completed/success，发布验收明确披露缺口。",
    "permanentAction": "下一次 L3 前将 dependency report 放入具备完整工具链和网络源的固定 Runner，并把 8/8 完整性作为报告可发布条件；负责人和复核日期未记录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/verification/windows-missing-toolchain",
    "position": "after-production-write",
    "count": 1,
    "impact": "生产 postdeploy 后补跑 Windows release gate 因缺少 docker/go/gofmt/make fail-closed；未生成部分通过结论，未改变生产。",
    "recoveryEvidence": "固定 Linux Runner 的同一 SHA L3 exit 0；候选/main CI、Release workflow、公开镜像 E2E 和生产 postdeploy 均成功。",
    "permanentAction": "下一版 L3 前把 Windows 工具链清单前置并避免生产后补跑本地全量门禁；该指纹已在 v0.98.2 出现，负责人和复核日期未记录，退出条件为唯一入口前置预检并通过。",
    "historicalReleases": ["v0.98.2"]
  }
]
<!-- kpanel-release-process-incidents:end -->

上述 10 项均未逃逸为产品变更失败、生产退化、回滚、紧急热修复或重复发布；生产写入后的两项只是补充验证通道被环境/数据源拦截，生产 postdeploy 仍为首次通过。

## 遗留风险与后续准入

- 未验证风险：依赖实时报告本次完整性为 4/8；未执行真实生产登录态、浏览器缩放矩阵和长期 soak；TypeScript 7、Node 26 类型及报告中的其他传递候选未完成兼容评估。
- 已实现待实机准入：无新增 UI 旅程；若后续依赖升级引发实际渲染、登录或长时间资源问题，应重新执行真实浏览器和 soak/性能门禁。
- 不阻断本版的理由：本版产品候选的固定 Linux L3、CI、Release、公开镜像 E2E、备份、标准生产更新和 postdeploy 全部通过；未变化 API、数据、端口、Compose、Agent 权限和应用市场契约。
- 后续应进入的自动门禁或专项工作流：下一版发布前修正 L3 证据回收、公开审计模板、Windows 工具链前置和 dependency report 8/8 数据源；重复的 Windows toolchain 指纹须在下一次 L3 生产写前完成唯一入口治理。
