# KPanel v1.3.0 发布验收记录

日期：2026-09-05

发布级别：L3

候选提交 / 标签：`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9` / `v1.3.0`

标签对象：annotated tag object `4eb4ba436895c0260cf9f8490e265aa083db6d98`；剥离后的产品提交精确为 `8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`。

候选基线：`origin/main@f5d37d6c7e5d8ef8def7e760ab09b30a39c5bfa5`，基线标签 `v1.2.0`。

上一稳定版本 / 回滚点：`v1.2.0` / `a163cff124a94d7f5efa07486f576d3998cf0ba3` / OCI index `sha256:ebadf5361a24b8d580da873190ae43f493ab2fefdf1ba9f41f4ffdab1f3e5f5e`。

## 发布画像

- 业务域：集群主机排序、当前面板文件切换、跨面板文件受限中继、公开分享入口、轻量节点显示名和文件拖入提示。
- 变更面：展示、只读、集群文件协议/中继和发布部署；未新增宿主机危险写入。
- 受影响用户旅程：在集群页设置并保持主机顺序；在文件管理、公开分享和终端主机列表中使用相同顺序；在当前 KPanel 内切换已配对主机并进行受限文件访问/互传；轻量节点重新接入后保留显示名；浅色主题下从其他 KPanel 拖入文件时清晰显示提示。
- 未变化契约：无数据库迁移、无 Compose 端口变化、无 `System Center` 页面/路由/API/数据/文档/写入变化；保留既有文件 API 白名单、权限、路径校验、`ResourceVersion`、流式传输限制和 Panel → Agent 边界；无 `kejilion.sh` 脚本源码变更，应用市场标准更新入口不变。
- 风险等级及理由：中等兼容性风险。变更集中在集群文件访问、排序持久化和前端交互，涉及 Panel/Agent 协同但没有 schema 或端口迁移；已通过固定 L3、CI、公开镜像复核和生产三阶段证据。

## 发布范围与未纳入内容

- 用户可见更新：
  - 集群文件管理、终端主机列表和公开分享页统一使用用户排序，排序持久化后在相关入口保持一致。
  - 已配对主机支持当前页面文件切换与受限的跨面板文件访问/互传。
  - 轻量节点重新接入保留用户设置的显示名。
  - 浅色模式的跨 KPanel 文件拖入提示提高文字与背景对比度。
- 精确提交清单：`62c668e`（轻量节点显示名，候选 cherry-pick `a984581`）、`7cb2a39`（主机排序，候选 `2cd9921`，冲突已按共享排序 helper 最小化解决）、`32a7200`（当前面板集群主机文件切换，候选 `0b80188`）、`d1ae676`（浅色主题拖入对比度，候选 `b8e46fa`）、`b211190`（公开分享主机排序，候选 `cd55c3c`）、`afeefc2`（视觉 radius token 修正）、`a7ddfd5`（文件代理签名参数修正）、`adb0bba`（排序结构测试安全比较）；版本与发布准备提交为 `4b0c7f8`，治理基线刷新为 `2215c3d`，最终发布记录修正提交为 `8374ff2`。最终相对候选基线差异为 42 个文件、1433 行新增、153 行删除。
- 明确未纳入的分支、文件或后续事项：Claude 界面美化工作树、轻量节点 SSH 终端未提交改动、进程管理热力图、云存储、原生图标、浏览器拖出专项及其他未形成干净可验收提交的工作树；所有 `System Center` 相关内容均排除。本次未连接 `108` 或 `prod-108`。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go 全量测试、前端全量测试、L3 集群/文件协议与 runtime contract、生产 postdeploy 版本/健康/Agent/digest 精确匹配 | 未做长期多节点并发 soak |
| 网络入侵与供应链安全 | 已验证 | `govulncheck`、`npm audit`、Trivy 源码/依赖/secret/config/镜像扫描、脚本契约和固定 Runner | 未进行独立第三方渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 Go/前端/应用生命周期、重启恢复、备份及生产前后证据通过；旧版本回滚点明确 | 未执行受控生产回滚演练 |
| 性能与资源预算 | 已验证 | L3 runtime contract 与生产容器资源证据通过；postdeploy 内存约 72.64 MiB / 256 MiB | 未做长时间压力或多节点吞吐基线 |
| 用户体验与可访问性 | 已实现未实机验证 | 相关前端定向测试、全量测试、typecheck、build 通过；保留 `:focus-visible` 等既有语义 | 未执行完整浏览器视口、缩放、主题和键盘人工矩阵；跨两台真实 KPanel 的拖入仍需专项验收 |
| 数据、配置与迁移 | 已验证 | 无 schema migration；保护配置 diff 为空；backup 包含旧镜像、Agent unit、`kpanel.conf`、数据归档和校验清单 | 未执行实际回滚，因此回滚结果为预案/备份证据而非演练证据 |

## 自动门禁

- 定向测试及结果：修正视觉 token 与轻量节点文件代理签名参数、排序结构测试后，前端 `133/133` 测试文件、`1132/1132` 测试通过；typecheck、`i18n:check`（2129 个本地化短语、21 个 lazy catalogs）和 build 通过。
- `make verify-release` 环境和结果：固定 Linux L3 Runner 内通过；Go 全量测试、前端测试/typecheck/build、应用生命周期、运行时契约、依赖和安全检查均通过。
- L3 外层入口：run=`v1.3.0-8374ff2-l3-r2`，candidate=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，base main=`f5d37d6c7e5d8ef8def7e760ab09b30a39c5bfa5`，base tag=`v1.2.0`，状态 `passed`、exit 0；远端目标仅 `arena-154`，证据目录 `/root/kpanel-release-evidence/v1.3.0-8374ff2-l3-r2`，本地证据目录 `C:\GitHub\_release-artifacts\v1.3.0-8374ff2-l3-r2`。
- L3 证据包：bundle SHA-256=`484723d7761440000f2d87487d21f667c0fc0ebbce26ac9aef9700a0fe6bbd30`；plan SHA-256=`f555409c7e6751fc063405ee781816fea20c52e62b790d0ed6c89debb0afa2ca`；远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；Runner=`kpanel-release-gate:go1.26.7-node24`，不可变 digest=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`。
- 候选 CI：最终 run `33890460224`，精确 SHA=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33890460224。更早的 `2215c3d` 与 `a7ddfd5` 候选 CI 分别拦截了源代码编译和测试比较缺陷，修正后没有进入发布链。
- 候选 Dependency freshness：run `33890460184`，精确 SHA=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33890460184。
- 主线 CI：run `33892389132`，精确产品 SHA=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，分支 `main`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33892389132。
- 主线 Dependency freshness：run `33892389173`，精确产品 SHA=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，分支 `main`，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33892389173。此后仅追加本验收文档，不改变产品依赖图。
- Release workflow：run `33892875758`，精确 SHA=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`，源码校验、Go/Node/security、原生构建、多架构推送、`latest` 晋级、公开 Release 和候选分支清理全部 completed successfully；https://github.com/kejilion/KPanel/actions/runs/33892875758。
- 安全扫描、镜像契约、SBOM/provenance：L3 的 govulncheck、npm audit、Trivy 源码/config/secret/镜像扫描、CGO-free amd64/arm64 构建和应用契约通过；公开镜像保留 OCI provenance attestation，Release workflow 的镜像标签、license 和受管脚本契约通过。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选与主线 Dependency freshness 均以最终产品 SHA 通过；依赖检测源和锁文件完整性通过。
- 最近每日安全通告审计、EOL 复核状态及证据：本版未新增独立人工安全通告/EOL 审计；不把自动扫描结果冒充人工审计，发布仍由固定 L3、CI 和 Release 安全门禁覆盖。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版无新增直接依赖、基座镜像或 Action；未产生待处置行动项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：沿用仓库锁定依赖与既有发布链；L3 Runner 使用 Go 1.26.7、Node 24；受管 `kejilion.sh` 继续使用既有发布脚本。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：产品版本文件和前端 package metadata 为 `1.3.0`；OCI index=`sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`；平台 manifest 为 amd64=`sha256:854b9d65fd3372e4eca29ca91a96747566bbef439c1ac4c14bea4cb0b307b10d`、arm64=`sha256:67f3fe42bc8ebffdabd1ea8350cf6ac660a64c88cdc48aaba13638ccafb3049e`；受管脚本 revision=`2ee9856c9916b7ede8bbc19edc97e22872e86203`，SHA-256=`77258027f934ffe6a583300f8350249978eace0ddc838e2d35f26cc5c21ae35c`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：Claude 界面美化、轻量节点 SSH 终端、热力图、云存储及系统中心范围继续暂缓；没有对应本版生产证据，待各自形成干净候选、独立复核和适用 L3 后再评估。
- 升级后的兼容、安全、构建、性能资源和回滚结论：固定 L3、候选/main CI、Dependency freshness、Release/OCI 以及 `arena-154` 三阶段证据通过；版本可按 `v1.2.0` 备份和 OCI 精确回滚。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux/Debian 13，Docker；L3 使用固定 `kpanel-release-gate:go1.26.7-node24` Runner。
- 环境策略 ID 与允许用途：`environment-policy.json` schemaVersion 1；`arena-154` 允许 candidate-validation、browser-validation、performance-validation、failure-injection、staging-deploy、production-deploy、production-safety-check；`prod-108` disabled 且无允许用途。
- 使用的精确候选或公开产物：候选完整 SHA `8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`、tag `v1.3.0`、OCI index `sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 `v1.3.0-8374ff2-l3-r2`，passed/exit 0，证据目录 `/root/kpanel-release-evidence/v1.3.0-8374ff2-l3-r2`，本地 bundle/plan/remote script 摘要见“自动门禁”；生产 run=`v1.3.0-production-20260905`，preflight/backup/postdeploy 均 passed/exit 0，本地目录分别为 `C:\GitHub\_release-artifacts\v1.3.0-production-20260905-preflight`、`...-backup`、`...-postdeploy`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：L3 和生产 postdeploy 为单次固定门禁与短时健康核对；长期多节点 soak 不适用本次补丁兼容性范围，作为后续观察项。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动前端测试覆盖相关排序、文件切换、拖入提示、焦点语义和中英文资源；完整浏览器视口/缩放/主题/键盘人工矩阵未执行，状态为已实现未实机验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 应用生命周期和 runtime contract、生产 backup、update、postdeploy 通过；未执行生产失败注入和实际回滚，保留 `v1.2.0` 作为回滚点。
- 未执行场景及原因：长期 soak、第三方渗透、完整浏览器人工矩阵、真实双面板拖入专项和受控生产回滚演练未执行；不把这些场景的缺失写成已验证。

## 发布产物与公开仓库复核

- GitHub Release：公开、非 draft、非 prerelease，`v1.3.0`；https://github.com/kejilion/KPanel/releases/tag/v1.3.0。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:1.3.0` 与 `:latest` 均为 `sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:854b9d65fd3372e4eca29ca91a96747566bbef439c1ac4c14bea4cb0b307b10d`；arm64=`sha256:67f3fe42bc8ebffdabd1ea8350cf6ac660a64c88cdc48aaba13638ccafb3049e`。
- 附件及 `SHA256SUMS`：8 个公开附件（Agent amd64/arm64、轻量节点 amd64/arm64、部署归档、`SHA256SUMS`、`LICENSE`、`THIRD_PARTY_NOTICES.md`）均可通过 HTTP 200；`SHA256SUMS` 长度 471 字节。
- 公开镜像 `image_e2e=pass`：本次未单独运行 `packaging/tests/image-e2e.sh`；L3 的原生构建/runtime contract、Release 镜像契约和生产从公开 OCI 拉取后的 postdeploy 均通过，因此不把独立脚本结果写成已执行。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场标准 `k app kpanel` 更新入口成功拉取并重建本版；受管脚本 revision 保持 `2ee9856c9916b7ede8bbc19edc97e22872e86203`，无脚本接口变更。

## 生产部署安全核对

- 生产目标和部署授权范围：本次唯一生产目标为 `arena-154`，仅执行 KPanel 标准应用市场更新。
- 验证/灰度环境：`environment-policy.json` 中仅使用允许的 `arena-154`；未使用 `prod-108`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight run=`v1.3.0-production-20260905`，expected version=`1.2.0`，passed/exit 0，开始 `2026-09-04T16:11:31Z`、结束 `2026-09-04T16:11:33Z`；backup 同 run expected version=`1.3.0`，passed/exit 0，结束 `2026-09-04T16:12:07Z`；备份为 `/root/kpanel-backups/pre-v1.3.0-20260904T161158Z`。本地 manifest SHA-256：preflight=`d6e1c4ca3ab40e4ed5bcbffebd242510fdcbb9bb1afc446649862b80a7ed33b2`、backup=`c5a60eabcb1d0be8d8fc6ef037aa47672352816269f3dd6abd9de4ba04cff412`。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；应用市场更新成功，拉取 digest 与本版 OCI index 一致。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run 同上，expected version=`1.3.0`、revision=`8374ff2d4a20d6cd949dc37a93f412eed64a1ec9`、image digest=`sha256:d84d8b748c516f095a6eb34c1c4f698bcc4a77c9fc7bf08fcc67efcb02e830b4`，passed/exit 0，结束 `2026-09-04T16:13:13Z`；Panel running/healthy，health `status=ok`、version=`1.3.0`，Agent active/running/enabled，restart=0，OOM=false；panel/agent 日志无 fatal、panic、OOM signature，SQLite/data check 通过，protected config diff 为空。公网入口沿用生产现有入口，未改域名/端口契约。本地 postdeploy manifest SHA-256=`48491405ba2325b5cc9ae7d82006662fd2db2717b8a9a08115cf9a9c7cc1efd6`。
- 生产已执行写操作：应用市场更新、拉取公开 OCI、容器重建/启动和标准 postdeploy 取证；未执行 System Center、危险系统资源写入或失败注入。
- 仅在隔离真机执行、未在生产执行的场景：长期压力、完整浏览器人工矩阵、第三方渗透和实际回滚演练。

## 回滚

- 源码/tag：`v1.2.0` / `a163cff124a94d7f5efa07486f576d3998cf0ba3`。
- 镜像 digest：上一稳定 OCI index `sha256:ebadf5361a24b8d580da873190ae43f493ab2fefdf1ba9f41f4ffdab1f3e5f5e`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.3.0-20260904T161158Z`，含旧镜像、Agent unit、`kpanel.conf`、Panel 数据归档、旧镜像归档、inspect 和 `SHA256SUMS`。
- 回滚步骤和回滚后复核：按应用市场规范恢复 `v1.2.0` 对应 OCI、Compose、`.env`、Panel 数据、Agent unit 和受管脚本，再以 production evidence 核对 health、Agent、digest、restart/OOM、数据和日志；禁止只切换浮动 `latest`。本次未执行实际回滚。
- 回滚后生产实际版本与健康状态：未执行，当前生产保持 v1.3.0 healthy。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release `v1.3.0` 已公开；Docker `latest` 与 `1.3.0` OCI index digest 一致；标准 `k app kpanel` 更新入口已拉取本版。
- 公共默认更新通道决策：恢复上一稳定版本；当前不需要短期保留旧版，若出现退化按上述精确 digest/备份回滚。

## 流程异常与证据修正

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-04T23:13:38+08:00
- 候选冻结时间：2026-09-04T23:44:28+08:00
- 生产完成时间：2026-09-05T00:13:13+08:00
- 提交到生产用时：0.99 小时
- 是否回滚、紧急热修复或重复发布：是（候选阶段发生两次产品门禁失败及一次 L3 本地 tag 前置拦截；未回滚、未紧急热修复、未重复发布产品）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间: 2026-09-04T23:23:06+08:00; 恢复时间: 2026-09-04T23:44:28+08:00; 逃逸门禁: 未逃逸: 候选 CI 与 L3 preflight 均在生产写入前拦截，最终候选 CI、L3 r2 和生产证据通过
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
    "impact": "第一次 L3 在候选共享工作树发现本地 v0.86.2 tag 与 origin 不一致，按规范 fail-closed 停止；没有上传候选、运行候选门禁或生产写入。",
    "recoveryEvidence": "改用全新干净 release clone，校验历史 tag 与 origin 一致后，以同一最终候选 SHA 执行 v1.3.0-8374ff2-l3-r2 并通过；证据目录为 /root/kpanel-release-evidence/v1.3.0-8374ff2-l3-r2。",
    "permanentAction": "L3 固定使用干净 release clone，执行 required tags 与候选/基线精确校验，禁止覆盖共享管理树的历史 tag。",
    "historicalReleases": ["v1.2.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

候选 CI 的两次失败属于产品/测试缺陷，分别在生产前被精确 SHA 门禁发现并修正，不计入发布流程异常：第一次为轻量节点文件代理签名参数缺失，第二次为含 `[]string` 的结构体直接比较；最终 `8374ff2` 候选 CI、Dependency freshness、L3、主线 CI、Release 均通过。首轮本地 `verify-change` 的 business context freshness 过期也在冻结前刷新 canonical review 后通过，不进入生产。

## 遗留风险与后续准入

- 未验证风险：完整浏览器人工矩阵、真实双面板拖入、长期多节点并发 soak、独立第三方渗透和受控生产回滚演练未执行。
- 已实现待实机准入：跨集群文件互传的真实多节点并发、轻量节点重新配对后的连续文件操作、不同视口/缩放/主题下的排序和拖入提示。
- 不阻断本版的理由：变更边界清晰，无 System Center 范围；固定 L3、候选/main CI、Dependency freshness、Release/OCI、备份和 `arena-154` 三阶段生产证据均以最终 SHA 通过，当前 Panel/Agent healthy。
- 后续应进入的自动门禁或专项工作流：后台浏览器验证、跨节点文件中继并发/失败恢复专项、发布回滚演练；Claude 与其他未冻结工作树单独形成候选后重新走完整门禁。

生产结论：`v1.3.0` 已在唯一授权目标 `arena-154` 完成上线；`System Center` 保持排除，`108`/`prod-108` 未连接。
