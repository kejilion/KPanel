# KPanel v0.98.4 发布验收记录

日期：2026-08-28

发布级别：L3

候选提交 / 标签：`7e1ffa8416fcfa6fb0960a87e00f53b53220735d` / `v0.98.4`

上一稳定版本 / 回滚点：`v0.98.3` / `sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`（commit `47debf1b2a98b418906a81c911280db3db05dbd7`）

## 发布画像

- 业务域：体检页面的响应式桌面窗口滚动。
- 变更面：用户界面与回归测试；不涉及 API、数据、宿主机权限或部署协议。
- 受影响用户旅程：手机窄视口切换桌面模式后，在体检窗口报告区域通过触摸上下滑动查看完整报告。
- 未变化契约：API / 数据 / 端口 / Compose / Agent 权限 / `kejilion.sh` / 应用市场安装契约。
- 风险等级及理由：低到中；变更仅修正 scoped CSS 的全局选择器匹配范围，但属于用户可见触摸交互，仍保留真实手机浏览器验收缺口。

## 发布范围与未纳入内容

- 用户可见更新：修复手机窄视口进入桌面模式后，体检窗口报告区域无法通过触摸上下滑动的问题；经典模式页面滚动与桌面窗口内滚动边界保持分离。
- 精确提交清单（相对候选基线 `23338a70671339ea8ab66d5ee5ebe100fef71337`）：`b5c22d2615a70d3c5ebba9a84c53a012e52b910d`、`7e1ffa8416fcfa6fb0960a87e00f53b53220735d`。
- 明确未纳入的分支、文件或后续事项：System Center 文件、路由、API 和文档未纳入；`kejilion/apps` 未产生提交；`kejilion.sh` 未改；`108`/`prod-108` 未连接；真实手机浏览器触摸、缩放矩阵和长期 soak 未纳入本轮。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 体检布局定向测试 1 文件/32 项通过；固定 L3 Web 122 个测试文件/1044 项、typecheck、build 通过；公开镜像 bootstrap E2E 与生产 Panel/Agent 健康通过 | 未新增后端 API 或 Agent 互通路径；真实手机触摸仍未执行 |
| 网络入侵与供应链安全 | 已验证 | L3/Release 的 govulncheck、npm audit、Trivy、镜像契约、SBOM/provenance、双架构 OCI 与附件校验均通过 | 依赖实时报告有 5/8 检测源暂时失败，已单独披露 |
| 稳定性、失败恢复与兼容 | 已验证 | 候选/main CI、L3、Release、公开镜像 E2E、生产停写备份、标准更新和 postdeploy 全通过；容器 healthy、重启 0、OOM false | 未执行生产回滚和长期 soak；回滚材料已保留 |
| 性能与资源预算 | 已验证 | L3 受限容器运行契约、生产资源快照和健康检查通过；未新增常驻任务 | 未执行独立压测或容量基准；本次无后端性能路径变化 |
| 用户体验与可访问性 | 已实现未实机验证 | CSS 选择器与布局回归断言已修正并通过，生产已部署该版本 | 未执行真实手机触摸、100%/125%/200% 缩放、主题、键盘/焦点和多语言专项验收 |
| 数据、配置与迁移 | 已验证 | 生产备份归档与校验通过，`protected.diff` 为空，SQLite quick check 通过；无 schema 或配置迁移 | 未执行生产回滚；旧镜像、Compose、`.env`、Agent 和数据备份可用 |

## 自动门禁

- 定向测试及结果：`npm test -- src/views/DiagnosticsView.layout.test.ts` 为 1 file/32 tests passed；L3 另通过 Go 全量、核心 race、Web typecheck、122 个测试文件/1044 项、i18n 2582 条/21 个 catalog、Vite build、govulncheck、npm audit、Trivy、安装安全和应用配置生命周期测试。
- `make verify-release` 环境和结果：固定 `arena-154` Runner image `kpanel-release-gate:go1.26.6-node24`，Runner ID=`sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；L3 run=`v0.98.4-7e1ffa8-l3-r1`，终态 `pass`/exit 0；本地归档为 `C:\GitHub\_release-artifacts\v0.98.4-7e1ffa8-l3-r1`，bundle SHA-256=`798099cd2d330ece3ded5955eb861bcd920724346ffc4063b38ea026c385557a`，manifest SHA-256=`48d2995de8be23ca5df50e24b5ec88e09d5b65ced6469cee4c16205fbbe7effe`，plan SHA-256=`ccb546d04466a91c290011a5aaf1c8d0d061bbe7e64dd3ed9a4a0a45c2628910`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：CI run `33137001909`、Dependency freshness run `33137001913`，均绑定 `7e1ffa8416fcfa6fb0960a87e00f53b53220735d`，completed/success。
- 主线 CI：CI run `33137289611`、Dependency freshness run `33137289631`，均绑定同一 SHA，completed/success；推进前 `origin/main` 未漂移。
- Release workflow：Release run `33137607599`、tag push 后 Dependency freshness run `33137607600`，均 completed/success；Release 已公开，候选分支已自动删除。
- 安全扫描、镜像契约、SBOM/provenance：Release 的 native image contract、双架构推送、`latest` promotion、SBOM/provenance 和公开 OCI digest 一致性检查均成功。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：实时报告 `2026-08-28T03:16:23.713Z`，本地归档 `C:\GitHub\_release-artifacts\v0.98.4-dependency-report-20260828-r1.json`，SHA-256=`be7f58cb744d47ef001411b210f1ade32c4501f3b3c29a4edf86f7a98b203fd7`；8 个检测源中 3 个成功、5 个失败（`go-modules`、`toolchains-and-base-images`、`docker-base-images`、`security-tools`、`dockerfile-frontend`），报告使用 `--allow-partial`，不作“全部依赖最新”结论；候选 108 项、可行动 12 项、传递信号 96 项。
- 最近每日安全通告审计、EOL 复核状态及证据：L3 govulncheck、npm audit 和 Trivy 均通过；EOL 状态 `current`，最近复核 `2026-07-28`，下一截止 `2026-10-28T23:59:59.999Z`，证据为 `docs/security-performance-hardening-2026-07-28.md`。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：实时报告记录 12 个可行动候选和 96 个传递信号；依赖仍按所属直接依赖、兼容范围和既有 L2/L3 门槛处理，未因报告部分失败而机械升级。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：本版未改变运行时依赖图、基础镜像、Action 或受管脚本；使用既有固定 Go `1.26.6`、Node `24.18.0`、Buildx 和受管 `kejilion.sh`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本元数据与 Web 根版本为 `0.98.4`；受管脚本 revision=`9fec61b50cc6ef798dfac1edf11c2ec60ca6b0d1`、SHA-256=`54ceb0e72c4c342382500fc35da636fa436c484a12c4766fb9c7f806a23ae8fa`；生产 OCI index=`sha256:7e0128bee4b5b190ed1dcfb93b50161c2a11d2ab15c3f0358bbc7daa06f8f703`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：报告中的 TypeScript 7、Node 26、Docker/工具链和其他 major/minor 候选暂缓；退出条件为具备完整检测源的下一次独立 L2/L3 兼容评估，负责人和复核日期未记录。
- 升级后的兼容、安全、构建、性能资源和回滚结论：固定 Runner、双架构构建/扫描、公开镜像 E2E、生产更新和 postdeploy 均通过；无 API、数据或权限迁移，回滚使用 `v0.98.3` OCI、Compose、`.env`、Agent 和本次备份成套恢复。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，hostname=`kejilion`，Debian 13，Linux `6.12.96+deb13-amd64`，x86_64，Docker `29.6.2`；L3 Runner 为固定 Go `1.26.6` / Node `24` 镜像。
- 环境策略 ID 与允许用途：`environment-policy.json` 的 `arena-154`，role=`hybrid`；`candidate-validation`、`production-safety-check` 和 `production-deploy` 均通过策略检查。
- 使用的精确候选或公开产物：公开 `docker.io/kjlion/kejilion-panel:0.98.4` 拉取后确认 digest=`sha256:7e0128bee4b5b190ed1dcfb93b50161c2a11d2ab15c3f0358bbc7daa06f8f703`，固定脚本以同一 digest 执行；生产标准入口实际拉取 `latest`，得到同一 digest。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run=`v0.98.4-7e1ffa8-l3-r1`，passed/0，无超时；生产 run=`v0.98.4-production-20260828`，preflight/backup/postdeploy 均 passed/0；本地证据目录为 `C:\GitHub\_release-artifacts\v0.98.4-production-20260828-{preflight,backup,postdeploy}`，固定远端生产脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。公开镜像 E2E 无后台作业 ID，最终 `image_e2e=pass`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：公开镜像固定 E2E 最终一次完整运行；生产 preflight、backup、postdeploy 各一次；无 soak，因为本版没有新增常驻任务，但真实移动端 UX 仍需补验。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：体检桌面窗口的报告滚动选择器与定向断言已覆盖；未执行真实手机浏览器视口、触摸拖动、缩放、主题、键盘/焦点和多语言矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 阶段按固定脚本受控 stop/start 并创建校验备份；标准更新入口 exit 0、输出 `KPanel 更新完成 / Update Complete` 与 `KPANEL_PROGRESS 100`；postdeploy 确认 Panel/Agent healthy/active、restart=0、OOM=false、SQLite 通过；未执行生产回滚，L3 应用生命周期负例按设计 fail-closed。
- 未执行场景及原因：未连接 `108`/`prod-108`；未执行真实手机/浏览器触摸和缩放验收、生产登录态专项和长期 soak；本版无需要生产业务写入的功能变更。

## 发布产物与公开仓库复核

- GitHub Release：[v0.98.4](https://github.com/kejilion/KPanel/releases/tag/v0.98.4)，Release ID=`378238521`，非 draft、非 prerelease，Release workflow=`33137607599`；annotated tag object=`3e1ed83ce1b2cbc7700cb6407b089a630c25e108`，peel=`7e1ffa8416fcfa6fb0960a87e00f53b53220735d`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.98.4` 与 `latest` 均为 `sha256:7e0128bee4b5b190ed1dcfb93b50161c2a11d2ab15c3f0358bbc7daa06f8f703`。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:e8e755407799e18db4396e33437316a7710bd560045b0911c9c8dd6d9c47a6f0`；arm64=`sha256:97aec8abeab3517c46f24f1d14fb61b8712f26ce88b5651d0f69013925722799`；两个 `unknown/unknown` manifest 为 provenance/SBOM attestation。
- 附件及 `SHA256SUMS`：Release 8 个附件齐全；公开下载到 `C:\GitHub\_release-artifacts\kpanel-v0984-public-assets-20260828` 后，Agent amd64=`4dfb50c8c23999a802dcc5f15c7fd6b7967b022a78bcd443b05e6b6364900503`、Agent arm64=`917d029e5ec6d24dc7f791f07d80c6cb5097918ea0e6cce2a0024c2871c84feb`、Node amd64=`10958d1ff73a73d066985b63f5c2ea6cf7b5fcb31ee91efa81c2f07756eb5504`、Node arm64=`1f3b6fcfc8525ff4b58c32e573db3809c3270d80837d19047a9a6c1979c22fa5`、部署归档=`f3c98054fb685b83c818143cf4b12117574fc9774e26ab2217adb8f991d6324d` 均与 `SHA256SUMS` 匹配；`SHA256SUMS` 文件 SHA-256=`4aa1e3892965bb1fe7f17b8228878320db33fdbaa9eaf91b9ad5ba08e5596d14`。
- 公开镜像 `image_e2e=pass`：`arena-154` 从 Docker Hub 回拉 `0.98.4`，确认公开 RepoDigest 后使用仓库固定 `packaging/tests/image-e2e.sh`、不可变 digest 和临时端口 `18084` 执行，健康、Host/Forwarded-Proto、bootstrap、Secure cookie、单网络、受限容器和最终 healthcheck 全部通过，临时脚本、容器和网络已清理。
- `kejilion/apps` / `kejilion.sh` 契约结论：`packaging/kejilion-app/kpanel.conf` 相对 `v0.98.3` 无变化，生产 `/root/apps/kpanel.conf` SHA-256=`7b5b52af0ff20cff4bebf114e747ddf1c82996500f2767ba8d3733217e83121c` 与本候选打包配置一致；未创建 `kejilion/apps` 提交；`kejilion.sh` 仍使用固定受管脚本 revision/摘要。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求在本次核对后走上线流程；仅执行 KPanel `v0.98.4` 标准应用更新、生产证据和备份。
- 验证/灰度环境：固定 `arena-154` L3 Runner、`arena-154` 公开镜像 E2E 和生产安全证据入口，均来自 `environment-policy.json`。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对；`108` 同样未连接。
- 部署前版本、健康、备份位置及摘要：preflight 记录 `0.98.3`、Panel `running/healthy`、Agent `active`、重启 0、OOM false；backup 证据通过，备份目录为 `/root/kpanel-backups/pre-v0.98.4-20260828T031136Z`，备份文件 SHA256 校验通过。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy 记录 health version=`0.98.4`、revision=`7e1ffa8416fcfa6fb0960a87e00f53b53220735d`、OCI digest 精确匹配；Panel `running/healthy`、Agent `active`、重启 0、OOM false；Agent/Panel 最近日志无 panic/fatal/OOM signature；protected 配置摘要无差异，SQLite quick check 通过，健康接口正常。
- 生产已执行写操作：固定 backup 阶段受控停止/恢复服务并创建旧版本备份；标准应用市场入口拉取 `latest` 并重建 Panel、更新 Agent 到 `0.98.4`；未执行业务数据写入、数据库迁移或端口/反向代理变更。
- 仅在隔离真机执行、未在生产执行的场景：公开镜像 bootstrap/受限容器 E2E、L3 负例和自动化浏览器/布局回归；真实手机触摸场景尚未执行。

## 回滚

- 源码/tag：`v0.98.3` / commit `47debf1b2a98b418906a81c911280db3db05dbd7`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel@sha256:05e2b7c069827f0bd4d004b1076d1f970e1fed88eb9a9fb1bd5524ecb08d33e9`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.98.4-20260828T031136Z`，包含旧应用目录归档、旧镜像归档、Agent unit、应用配置（如存在）和 `SHA256SUMS`。
- 回滚步骤和回滚后复核：本次未执行回滚；如需回滚，按应用市场原生失败回滚流程成套恢复 `v0.98.3` OCI、Compose、`.env`、数据目录和 Agent，并重新执行固定 preflight/postdeploy 复核。
- 回滚后生产实际版本与健康状态：未执行，不作已回滚声明。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Release 当前为 `v0.98.4`；Docker `latest` 与 `0.98.4` 同为 `sha256:7e0128...`；标准更新入口本次实际拉取该 digest。
- 公共默认更新通道决策：保留 `v0.98.4` 为默认更新版本；没有触发回滚或短期保留旧版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-28T10:31:26+08:00
- 候选冻结时间：2026-08-28T10:32:22+08:00
- 生产完成时间：2026-08-28T11:12:54+08:00
- 提交到生产用时：0.69 小时
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
    "fingerprint": "release-operator/tag-preflight/empty-output-check",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次 v0.98.4 tag 前置检查因空的本地 tag 输出调用 Trim 失败；未创建 tag、未写远端。",
    "recoveryEvidence": "该命令 exit 1 后未产生 tag；改用空值安全检查重新核对 main SHA，随后 annotated tag 推送成功。",
    "permanentAction": "发布编排的空输出解析改为数组和显式空值处理；后续固定命令先做 null-safe 语法检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/public-e2e/repo-digest-name-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次公开镜像校验因 Docker RepoDigests 使用短仓库名而包装器要求 docker.io 前缀提前退出；固定 image-e2e 尚未运行，生产未写。",
    "recoveryEvidence": "公开 pull 已返回目标 OCI digest；改用 Docker 实际返回的 kjlion/仓库名后，固定 packaging/tests/image-e2e.sh 输出 image_e2e=pass，临时资源已清理。",
    "permanentAction": "公开产物校验接受 Docker Engine 的 canonical RepoDigests 表示，校验命令先独立验证解析结果再调用固定 E2E。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

上述 2 项均发生在首次生产写操作前，未逃逸为产品变更失败、生产退化、回滚、紧急热修复或重复发布；生产 postdeploy 首次通过。

## 遗留风险与后续准入

- 未验证风险：真实手机触摸滑动、桌面模式的 100%/125%/200% 缩放、主题、键盘/焦点和长期使用仍未实机验证；依赖实时报告完整性为 3/8 成功源，5 个源因 fetch failed 未纳入完整性结论。
- 已实现待实机准入：体检窗口 scoped scroll selector 修复已进入 `v0.98.4`，定向测试、L3、公开镜像和生产 postdeploy 均通过；后续若要关闭 UX 风险，应在真实手机浏览器完成桌面模式触摸滑动验收。
- 不阻断本版的理由：本版代码范围最小且无 API/数据/部署契约变化；固定 L3、候选/main CI、Release、公开 OCI、image E2E、备份、标准更新和 postdeploy 全部通过。
- 后续应进入的自动门禁或专项工作流：补充真实移动端桌面模式的浏览器触摸/滚动几何验收；下一版 L3 前修正 dependency report 5/8 fetch failure 的完整检测环境，并将公共 digest 校验和 tag 前置检查改为固定 null-safe 脚本。
