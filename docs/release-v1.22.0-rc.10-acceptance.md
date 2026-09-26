# KPanel v1.22.0-rc.10 发布验收记录

日期：2026-09-26。发布级别：L3。releaseChannel：preview；releaseTrain：1.22.0。

- 候选提交 / 标签：081c983bccb617cb70e5a39f77507eeefb521f30 / v1.22.0-rc.10（tag object c0f10a6c9961dab496da91d3092d883267216fe8）。
- 发布前 main：f92b1223d46f81a314514aadaa0c0e4cf3be317b（RC9 验收提交）。
- 上一预览 / 回滚点：v1.22.0-rc.9，f727155f9e9e33a140886a6fdec32800f0f7ff5c；镜像 index sha256:02d750537540888b05299e7bcdc93d22d0cabb9dde9fcae317eb5eefff063bc2。
- 上一稳定版：v1.21.0，396fcd62c5635c9812ee97ee509cb61b962724f3；镜像 index sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。
- 发布候选分支：release/v1.22.0-candidate；预览版按 `docs/project-management.md` 10.2 保留。
- 发布工作树：C:/GitHub/_codex-tasks/kpanel-v122-rc10-release；验收工作树：C:/GitHub/_codex-tasks/kpanel-v122-rc10-acceptance。
- 证据根目录：C:/GitHub/_release-evidence/v1.22.0-rc.10-20260926；L3：C:/GitHub/_release-evidence/v1.22.0-rc.10-081c983b-l3-r1。

预览产物已公开，生产未部署（预览版禁止生产部署）。公开镜像的登记隔离环境 E2E 尚未完成。

## 发布画像与范围

- 业务域：桌面外观、登录页、集群监控。
- 变更面：前端展示与浏览器本地副本；Panel 静态资源缓存及 CSP 图片来源；未增加 API、数据库迁移或宿主机写入。
- 核心用户旅程：上传自定义图并预览、经典模式通透显示所选图、登录页显示自定义图或 3D 场景静态海报、从集群指标深链放大时间范围后保持浏览位置。
- 未变化契约：Agent、端口、Compose、`kejilion.sh`、`kpanel.conf` 安装/更新契约、应用市场稳定默认入口。
- 风险等级：中。改动触及 CSP 和登录页私有图片本地副本；CSP 只为 `img-src` 增加 `blob:`，`script-src` 保持 `'self'`。

| 来源 | 纳入候选 | 内容 |
| --- | --- | --- |
| fix/wallpaper-classic-login-20260926，tip 3f75b35a | 8a9392e0 → 75d939db；cb44b47b → 34a25a4a；3f75b35a → a04dc765 | 上传预览 CSP、经典模式启动脚本缓存、登录页图片缩小重试 |
| fix/monitoring-cluster-zoom-delivery-20260926，tip e61bed80 | e61bed80 → 765de39b | 指标时间范围放大后的滚动定位 |
| RC10 集成 | 081c983b | 版本元数据与 CHANGELOG |

- 未纳入其他本地功能分支。RC9 既有功能及验收提交是基线，不作为 RC10 新增功能重复统计。
- 合并冲突：无。两个来源提交集均在 RC9 验收基线之上，逐提交 cherry-pick 后形成发布提交。

## 外部审计与修复交付

- 安全审计：未执行新的 CF run。覆盖检查 `decision=scoped-required`，原因是 RC9 新增 `internal/desktopwallpapers` 信任边界包；截至本候选未审计 9 个提交 / 10 个文件，最早 0 天。预览版记录而不阻断；1.22.0 稳定版预检前按 `PROJECT_RULES.md` 5.4 补 scoped 审计或记录符合规范的用户决定。既有 full run-4、scoped run-9/7/6 不覆盖本次差异。
- OCR：发布候选 `f92b1223..081c983b`，11/11 个可审文件覆盖，自由臂无发现、约束臂无新增成立发现；由于来源提交曾有 OCR 记录，本轮非盲测，`constrained-only=unreported`。证据位于 `ocr/`，最终提交仅追加 trailer，源码 tree 与受审提交一致。
- 独立复核：发布任务复查了精确差异、失败边界和门禁；本轮未取得另一模型提供商的独立复核记录，不把 OCR 或 CI 冒充为独立复核。
- 修复交付：源码、候选分支、主线和公开 RC10 标签同为 081c983b；Release 与双架构镜像已经公开。生产部署不适用。

## 跨仓库联动判定

- `scriptLinkageState`：not-required（无需发布脚本（不适用））。
- 变更集编号 / 脚本候选：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：2b90b2d2ca56bc954c9328a51bb5571e896f713d / 806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99；L3 `managed_script_contract=pass`。
- 判定依据：`v1.22.0-rc.9..081c983b` 不包含受管脚本、`packaging/kejilion-app/kpanel.conf` 或安装/更新契约变更。本版不创建 `kejilion/sh` 或 `kejilion/apps` 提交。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 前端 1712 项测试、Panel 测试、本地模拟浏览器上传/经典/登录/场景流程 | 登记隔离环境的真实 API 与公开镜像 E2E 未执行 |
| 网络入侵与供应链安全 | 已实现未实机验证 | CSP 测试、L3 Trivy / govulncheck / npm audit | `internal/desktopwallpapers` scoped 审计待稳定版前补齐 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | L3 Go race、应用更新生命周期；图片副本尺寸重试单测 | 真实浏览器存储配额与跨浏览器差异未完整验收 |
| 性能与资源预算 | 已实现未实机验证 | 副本最多 400 kB；L3 镜像资源与运行时契约 | 真机 GPU 与大图性能未测 |
| 用户体验与可访问性 | 已实现未实机验证 | 绑定候选提交的 1280 px 本地浏览器流程、经典与登录背景、无页面错误；监控定位回归单测 | 窄屏 200% 缩放仍有已知横向溢出；全视口/语言矩阵未覆盖 |
| 数据、配置与迁移 | 不适用 | 无 schema 与持久化配置变更；私有图片副本仅在当前浏览器 | 跨浏览器登录页不会共享该副本，属于既有设计边界 |

## 自动门禁

- 本地：版本一致性、业务上下文新鲜度、治理一致性、候选状态和工作流 YAML 解析通过。
- L3：`scripts/run-release-l3.mjs`，run ID `v1.22.0-rc.10-081c983b-l3-r1`，target `local-wsl-dr`，2026-09-26T02:24:28Z 至 02:31:56Z，`status=passed`、`exit_code=0`。`arena-154` SSH 连接超时，按登记的候选灾备路径执行。
- 固定 Runner：`kpanel-release-gate:go1.26.7-node24`，image ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- L3 bundle SHA-256：`7d67444a448ea8663b072cb7604007ea60c3e98d2b3d9601aa181bf62c61ffa6`；plan：`a6ee9c80933f146cac8977ad238ebcf844a5a7238f87eba4c6f67aa4cfb2a554`；执行脚本：`21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- L3 覆盖 Go 全套/race、前端 1712 测试、Trivy 源码与镜像扫描、最终镜像构建、应用配置备份恢复生命周期。
- 候选 CI：[36212021954](https://github.com/kejilion/KPanel/actions/runs/36212021954) success；Dependency freshness 36212021968 success。
- 主线 CI：[36212507268](https://github.com/kejilion/KPanel/actions/runs/36212507268) success；Dependency freshness 36212507207 success。两者精确 SHA 均为 081c983b。
- Release workflow：[36213042084](https://github.com/kejilion/KPanel/actions/runs/36213042084) success；标签上的 Dependency freshness 36213042045 success，精确 SHA 同为 081c983b。Release 的源码验证、运行时契约、镜像构建及通道提升均通过。

## 依赖与技术栈

- 未升级 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本；锁文件只改变产品根版本。
- 沿用 RC9 依赖检测状态；本轮候选及主线 Dependency freshness 均通过。没有新增可采用依赖候选。

## 隔离真机与浏览器验收

- `local-wsl-dr` 仅登记为 candidate-validation，实际只承担候选 L3。`arena-154` 登记允许 browser-validation，但 2026-09-26 SSH 连接超时，未在其上执行公开镜像或真实浏览器验收。
- 本地 mock acceptance 预览：`http://127.0.0.1:4177/settings`，`visual-composition`，绑定干净提交 081c983b，证据 `preview-acceptance/manifest.json`。它验证界面和交互，不证明真实 Panel/Agent、宿主机或公开镜像行为。
- 本地 Chrome 1280×800：在模拟 API 和与生产一致的 CSP 下，上传弹窗图像自然宽度大于 0，自定义壁纸进入经典模式与登录页，场景 `pack:neon-city` 在登录页呈现 WebP 静态海报，页面错误 0；结果见 `wallpaper-journey.jsonl`。
- 监控：`MonitoringView.test.ts` 覆盖从集群指标深链选择放大时间范围，数据重载后 `scrollIntoView` 仍只调用一次。
- 未执行公开镜像 E2E、真实 API/宿主机验证、125%/200% 全组合、Safari 与低端 GPU；原因是登记隔离环境不可达，不能把 WSL 灾备环境扩展为浏览器用途。

## 发布产物与公开仓库复核

- GitHub Release：[v1.22.0-rc.10](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.10) 于 2026-09-26T03:02:45Z 公开，draft=false、prerelease=true；GitHub Latest 仍是 v1.21.0。
- Docker `1.22.0-rc.10` 与 `preview` OCI index 均为 `sha256:0a00853548e05671ea404b9ae8e9e3bdb660ea8a1aa1a4451044cb2ac1742c8d`；`latest` 仍是 v1.21.0 的 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。发布前 `preview` 为 RC9 `sha256:02d750537540888b05299e7bcdc93d22d0cabb9dde9fcae317eb5eefff063bc2`。
- `linux/amd64`：`sha256:7f09ff1e7f0937edd30afb746b8fc8467a654b4e915dd78f028d53cd5cd6eb42`；`linux/arm64`：`sha256:f0454c7f988d27f2bdd39ad2928c35acce8b10fd454f95cf61d189625ffaef0a`。两平台在版本与 `preview` 标签中一致。
- 附件 14 项，含 Agent/Node/MCP 二进制、部署包、许可证和 `SHA256SUMS`。校验文件 11 项逐一与 GitHub 资产 `digest` 一致；证据为 `github-release-api.json`、`SHA256SUMS` 和 `docker-tags.json`。
- 公开镜像 E2E：未验证；登记隔离环境不可达。
- `kejilion/apps`：本轮相对 RC9 的 `kpanel.conf` 无差异，不产生应用市场提交；默认 `latest` 不变。应用市场工作树为 clean 的 `39b498a0`，其 `kpanel.conf` 与本仓库版本不同，这是 RC9 已记录的延至稳定版同步项。

## 自更新与生产适用性

- 自更新通道、加入/退出预览、自动安装与一次性安装代码均未变化，沿用 RC9 回归结论；本版只提升 `preview`，不触动稳定来源。
- 生产目标与部署授权范围：不适用（预览版禁止生产部署）。本次未连接、未备份、未部署、未升级或核对 `prod-108`；该环境禁用全部 KPanel 操作。
- 正式部署环境、生产写操作及部署前后安全核对：不适用。`arena-154` 本轮未建立 SSH 连接。
- 本版产物发布不等于用户面板已经升级；预览通道需用户显式选择并确认精确版本与 digest。

## 回滚

- 不可变回滚点：v1.22.0-rc.9 / `sha256:02d75053…63bc2`；稳定默认仍为 v1.21.0 / `sha256:e2c5d392…3f61`。
- 未执行生产写入，数据/配置备份与生产回滚不适用。预览用户需经独立明确操作选择旧版 digest；退出预览不会自动降级。
- GitHub Latest、Docker `latest` 与应用市场标准默认入口保持 v1.21.0；本版不改变公共稳定默认通道。

## 分支与资源处置

- `release/v1.22.0-candidate` 从 f727155f 快进至 081c983b，预览期间保留；本轮不归档同一发布序列候选。
- 来源修复分支仅在本地，精确 tip 分别为 3f75b35a 与 e61bed80；内容已 cherry-pick，保留原工作树与用户预览，不以发布操作清理其他会话资源。
- 本验收记录在独立 `docs/release-v1.22.0-rc.10-acceptance` 分支提交，通过同 SHA 候选与主线 CI 后按 `docs/project-management.md` 10.2 归档。发布 worktree 的 acceptance 预览与 L3 证据仍用于复核。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-26T09:27:43+08:00
- 候选冻结时间：2026-09-26T10:23:59+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：是（RC9 壁纸缺陷导致 RC10 重复预览发布）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-09-26T09:29:53+08:00；恢复时间：2026-09-26T11:02:45+08:00；逃逸门禁：已逃逸：RC9 本地 Vite 验收未覆盖生产 CSP、旧启动脚本缓存和大图登录副本失败，且未完成公开镜像浏览器验收
<!-- kpanel-release-metrics:end -->

本版是预览，不计稳定发布或生产部署频率。故障发现时间取本会话用户首次报告（2026-09-26T01:29:53Z）；“恢复时间”指修复产物公开可用，不代表用户实例已升级或恢复。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记隔离环境 SSH 超时；候选 L3 走登记灾备，公开镜像和真实浏览器 E2E 未取得合规终态。",
    "recoveryEvidence": "local-wsl-dr 的候选 L3 status=passed；浏览器和公开镜像 E2E 明确记为未验证，没有冒用本地模拟结果。",
    "permanentAction": "负责人为 1.22.0 稳定版发布任务；2026-10-03 前复核 arena-154 SSH 和 browser-validation，退出条件为在登记隔离环境对同一公开镜像完成真实浏览器 E2E。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 公开镜像与登录/壁纸真实浏览器 E2E 待登记隔离环境恢复后补测；窄屏 200% 缩放存在已观察的横向溢出，未纳入本次修复范围。
- `internal/desktopwallpapers` scoped 安全审计与 `kpanel.conf` 隔离真机事务演练，仍是 1.22.0 稳定版前准入。
- 本轮没有生产部署或应用市场同步；本地预览和证据暂保留供验收，发布结束时再盘点可回收资源。
