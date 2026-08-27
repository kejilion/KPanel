# KPanel v0.98.2 发布验收记录

日期：2026-08-27

发布级别：L3

产品候选提交 / 标签：`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7` / `v0.98.2`

上一稳定版本 / 回滚点：`v0.98.1` / `sha256:be6ffc64c97a37f733c703424858c6f04153e732fd2448a0b19e9ea79fa3e102`

## 发布画像

- 业务域：体检（Diagnostics）报告布局与响应式可读性。
- 变更面：展示层、版本元数据与验收记录；无后端、Agent、宿主机或协议写入。
- 受影响用户旅程：打开体检报告，查看性能/网络分区、资源数值、身份信息与线路数据；在宽桌面、中等宽度和 390px 手机视口阅读并滚动。
- 未变化契约：API / 数据 / 端口 / Compose / Agent 权限 / `kejilion.sh` / 应用市场。
- 风险等级及理由：低；仅改 Vue/CSS/布局回归与三语文案，未改业务数据或网络边界。

## 发布范围与未纳入内容

- `fdda78bc74df09891f836c6e9439041def86630a`：重排体检性能/网络报告，统一数值字号与网格，改善中等宽度和手机滚动，移除重复 per-card 分数芯片与“出口线路”显示。
- `ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`：版本 0.98.2、CHANGELOG 与发布元数据。
- 只纳入上述 2 个线性提交；旧诊断分支、共享脏 `main`、未提交草稿、浏览器/系统中心等其他工作树均未纳入。
- 不涉及 `kejilion/sh`、`kejilion/apps` 或应用安装契约。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 固定 Runner L3；Go 全量、Web 122 文件/1044 测试；Diagnostics 布局回归 32 项 | 本版无新后端协议或 Agent 路径 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 无可达漏洞、npm audit 0、Trivy source/image/config 0、双架构 OCI revision/version 校验 | 本版未增加网络入口、权限或依赖 |
| 稳定性、失败恢复与兼容 | 已验证 | L3、候选/main CI、Release、停写备份恢复、标准更新、postdeploy 全通过 | 无数据迁移；生命周期负例均按设计 fail-closed |
| 性能与资源预算 | 已验证 | L3 受限容器构建/健康检查通过；生产 postdeploy 容器 healthy、restart=0、OOM=false | 低风险 CSS/布局变更，无常驻任务 |
| 用户体验与可访问性 | 已验证 | 标准 acceptance/visual 预览；宽/中等/390px 视口布局测试、数值可读性与滚动断言通过 | 当前本地浏览器会话未形成独立生产登录态，未把 mock 数据冒充真实业务数据 |
| 数据、配置与迁移 | 已验证 | 154 备份归档、旧镜像加载、SHA256SUMS、SQLite quick_check、Compose/.env/Agent 摘要前后无差异 | 无 schema 或配置迁移 |

## 自动门禁

- 本地候选验证：`npm test` 122 files / 1044 tests、`npm run build`（i18n 2582、typecheck、Vite/precompress）通过；Windows `verify-change` 因缺少 `go/gofmt` 按设计 fail-closed，未冒充全量通过。
- `make verify-release` / L3：run `diagnostics-v0982-20260827`，固定 Runner `sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`，exit 0；bundle `C:\GitHub\_release-artifacts\kpanel-v0982\kpanel-diagnostics-v0982-20260827.bundle` SHA-256=`903ba93967933db8ed059961acbec0b06ffeed50e4a0439f439ef1ce54c38eed`；manifest SHA-256=`99e67186855ffeb5d53f1c9319fae2a7058a6ef5aad2c3d16fe6d98e9f1c4c`；plan SHA-256=`a25f06bcdd88cd3e9fd6412be51e7aec3755365aead331a7289fa7f006fdf6e1`；remote script SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 候选 CI：`33075030018` CI、`33075029982` Dependency freshness；均绑定 `ef843a08e0e8a8fb3199c1dd6f471211943ff3b7` 且 completed/success。
- 主线 CI：`33076431460` CI、`33076431464` Dependency freshness；均绑定同一 SHA 且 completed/success。
- Release workflow：`33076882566` completed/success；Tag Dependency freshness `33076882543` completed/success；Release 页面为非 draft，候选分支已自动清理。
- 安全扫描、镜像契约、SBOM/provenance：L3 govuln/Trivy/npm audit 全通过；官方 workflow 完成 native image contract 与多架构推送。

## 依赖与技术栈变化

- 依赖新鲜度：候选与 Tag 依赖检查均成功；`npm ci` 审计 0 vulnerabilities（仅既有 `glob@10.5.0` deprecation warning）。
- 本版未新增依赖、工具链、基础镜像、Action 或受管脚本候选；版本仅更新到 0.98.2。
- OCI revision/version、受管脚本 revision/hash 与 v0.98.1 一致性由 Release workflow、公开 registry 与生产 postdeploy 核对。
- 直接/传递依赖无本版行动项；glob deprecation 为既有提示，未构成发布阻断。

## 隔离真机与浏览器验收

- L3/隔离运行目标：`arena-154` 固定 Runner；Ubuntu/Linux 构建、Go 全量/race/vet、双架构二进制与受限容器检查通过。
- 本地浏览器预览：标准 `local-feature-preview` acceptance/visual，mock 数据模式，commit=`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`；证据目录 `C:\GitHub\_release-artifacts\kpanel-v0982-preview`，预览进程已停止且端口不可访问。
- 受影响旅程：体检报告分区、性能/网络卡片数值和身份列可读；宽桌面/中等宽度/390px 的布局与无横向溢出由 122 文件测试及预览核对覆盖。未把 mock API、截图或本地预览表述为生产业务数据验收。
- 未执行：本版无新增宿主机写操作；未连接 108；没有为纯布局变更在生产执行业务写入或长时间 soak。

## 发布产物与公开仓库复核

- GitHub Release：[v0.98.2](https://github.com/kejilion/KPanel/releases/tag/v0.98.2)，Release workflow `33076882566` completed/success；annotated Tag peel=`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`。
- Docker `0.98.2` 与 `latest` OCI index：`sha256:f894543eee6c8999824d43357a6dfa12421912c93d3fc9a66628a5673b3abeb6`。
- `linux/amd64`=`sha256:c306cb234b9463cc4c270e1c81a3775708b091a080e152990c17a550a912e7a8`；`linux/arm64`=`sha256:c53c6e4657d86cacb7d892adb09ec710fe9c5e38aac6994e3180ed7b17940e9e`；unknown/unknown 为 provenance/SBOM attestation。
- 公开 OCI labels：version=`0.98.2`、revision=`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`、source=`https://github.com/kejilion/KPanel`；manifest HEAD 与 `latest` digest 一致。
- Release 附件由 workflow 生成并含四个 amd64/arm64 Agent/Node 二进制、部署归档、`SHA256SUMS`、许可证及第三方声明。
- `packaging/kejilion-app/kpanel.conf`、`kejilion/sh` 与应用市场契约相对 v0.98.1 无差异，未产生 apps 提交。

## 生产部署安全核对

- 唯一生产目标：`arena-154`；`108`/`prod-108` 未连接、未检查、未测试、未备份、未部署、未升级。
- 部署前：run `diagnostics-v0982-prod-pre-20260827`，health `0.98.1/status=ok`，Agent active/enabled，Panel healthy，restart=0，OOM=false。
- 停写一致性备份：`/root/kpanel-backups/pre-v0.98.2-20260827T133833Z`；远端备份 `SHA256SUMS` 本地证据 SHA-256=`7a46b40292f357004dee7e1f95a7e5cab2265adf56a0826537659cc9f21003bd`；归档、旧镜像加载、校验和及恢复健康均通过。
- 标准入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，拉取官方 latest digest、输出达到 `KPANEL_PROGRESS 100` 与 `KPanel 更新完成`，exit 0。
- 部署后：run `diagnostics-v0982-prod-post-20260827`；health `0.98.2/status=ok`，OCI revision=`ef843a08e0e8a8fb3199c1dd6f471211943ff3b7`、digest=`sha256:f894543eee6c8999824d43357a6dfa12421912c93d3fc9a66628a5673b3abeb6`；Panel healthy、Agent active、restart=0、OOM=false；配置保护摘要无差异，SQLite quick_check=ok，近 10 分钟日志无 panic/fatal/OOM。
- 生产写操作仅包括备份阶段受控 stop/start 与标准应用市场更新；未执行体检业务写操作。

## 回滚

- 源码/tag：`v0.98.1`（产品 commit `a4848d5a468f258a1e7f2193cadfe0ce8e98043b`）。
- 镜像 digest：`sha256:be6ffc64c97a37f733c703424858c6f04153e732fd2448a0b19e9ea79fa3e102`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.98.2-20260827T133833Z`，包含旧镜像、Compose、`.env`、Agent unit、apps 配置（若存在）和数据归档，且已恢复校验。
- 回滚步骤：停写后按备份成套恢复匹配的旧镜像、Compose、`.env`、Agent unit、apps 配置和数据，再用标准入口核对 health、Agent active、restart/OOM、SQLite 与保护摘要；不得只换镜像或只改版本字符串。
- 未执行生产回滚；当前正式版本保持 `0.98.2` 健康。GitHub Release、Docker `latest` 与标准更新入口均指向 0.98.2。
- 公共默认更新通道决策：短期保留 0.98.2；本版无已知产品阻断，若后续发现体检视觉回归，按上述备份与 v0.98.1 OCI 成套回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-27T21:03:11+08:00
- 候选冻结时间：2026-08-27T21:04:10+08:00
- 生产完成时间：2026-08-27T21:39:53+08:00
- 提交到生产用时：0.61 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-operator/git-fetch/local-conflicting-tag",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 fetch origin main --tags 被本地历史冲突标签 v0.86.2 拒绝；未改产品或远端，随后仅 fetch main 成功。",
    "recoveryEvidence": "origin/main 仍精确为 4e5e1b8，候选与 L3 使用该 SHA，未覆盖本地标签。",
    "permanentAction": "主线核对使用不改写本地标签的 refs/heads/main；标签冲突作为只读预检事项记录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/browser-preview/session-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地 acceptance 预览浏览器会话在切换经典模式时超时；未触及远端、产品代码或生产，预览进程随后按 ownership marker 停止。",
    "recoveryEvidence": "预览 status=stopped、进程存活=false、端口不可访问；L3/CI/生产门禁独立通过。",
    "permanentAction": "本版仅保留 mock visual 预览证据，不把超时会话当作生产浏览器结论；后续 UI 变更复用独立临时浏览器 profile。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/verification/windows-missing-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows change-aware 入口因缺少 go/gofmt fail-closed；没有生成部分通过结论或写入远端。",
    "recoveryEvidence": "固定 Linux Runner 的 L3 从同一 SHA 完整通过，候选/main CI 与 Release workflow 均成功。",
    "permanentAction": "Windows 仅作前端/版本预检；Go 全量、竞态、镜像和部署门禁固定由 Linux Runner 权威执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-operator/production-verification/ssh-template-quoting",
    "position": "after-production-write",
    "count": 1,
    "impact": "一次补充只读 docker inspect 因 PowerShell/SSH 模板转义失败返回错误；未修改生产状态。",
    "recoveryEvidence": "随后使用简化模板成功核对容器 running/healthy、restart=0、OOM=false 与官方镜像 digest。",
    "permanentAction": "生产取证优先使用固定脚本导出的 JSON/状态文件，补充 SSH 查询避免复杂嵌套模板。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 当前无已知产品 P0/P1/P2；本版体检布局未改 API、数据或宿主机行为。
- 本地视觉证据为 mock 数据模式；若后续调整体检真实数据密度、长文本或特殊语言，应重新执行真实浏览器 `visual` 旅程。
- Windows/Chrome 企业策略、真实生产登录态不属于本版布局门禁；未宣称已验证不存在此类环境差异。
- 后续涉及体检数据语义、宿主机指标或 API 变更时，必须重新选择相应 L2/L3 真机门禁，不得复用本版纯 UI 结论。
