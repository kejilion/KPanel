# KPanel v1.21.0-rc.9 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`62c9871034da7ffcc5489927ee5ecc198bdfab82` / `v1.21.0-rc.9`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `62c9871034da7ffcc5489927ee5ecc198bdfab82`

- 原分支 / 精确 tip / 处置分类：`feature/passkey-20260921` / `b8ba15f484c64d43241eaa3f2d4060d702b75fbd`、`fix/desktop-group-initial-render` / `cf5338261dccbd4741896e7b4e920bff8aa23ceb`、`docs/security-audit-trigger-20260921` / `aea09c4aca9e045362ffb6081e00fe40d7b991bd`；均已纳入 rc.9，来源分支保留用于复核，预览候选分支继续保留。
- 归档 ref 与 SHA：本次没有删除来源分支；候选分支和 tag 已由远端复核精确指向 `62c9871034da7ffcc5489927ee5ecc198bdfab82`，tag object 为 `ac40c4500adc48b4303f8885a8b0f1788b666ea6`。
- 本次来源任务分支：Passkey 后端、前端和测试以 `b8ba15f4` 为纳入依据；桌面首次渲染与紧凑重排以 `cf533826` 为纳入依据；治理审计触发以 `aea09c4` 为纳入依据。`feature/passkey-store-20260921`、`feature/passkey-ui-20260921` 已被 Passkey 主分支完整包含，未重复合并。
- 本地分支/upstream/worktree：发布 worktree `C:/GitHub/_codex-tasks/kpanel-v1.21.0-rc.9-release` 保留用于验收复核且工作树干净；浏览器、mock、L3 证据目录保留，不删除可再生缓存。
- 未完成归档项 / 责任人 / 下次复核触发条件：无；若组装 `v1.21.0` 稳定候选，需以新的稳定候选重新执行 L3、候选/main/tag 门禁和生产流程。

归档不代表生产上线；本版只发布预览产物，生产未部署。

## 发布画像

- 业务域：Passkey 登录与凭据管理、桌面首次渲染与窗口重排、发布治理审计。
- 变更面：前端展示与认证协议、只读/凭据管理 API、测试和发布治理；没有生产部署写入。
- 受影响用户旅程：用户可在支持 WebAuthn 的浏览器中注册、登录、撤销 Passkey；打开桌面时分组、壁纸和遮罩首帧稳定，紧凑视口重排不播放位置动画。
- 未变化契约：TOTP、恢复码、会话撤销、API 端口、Compose、Agent 权限、应用市场稳定入口和 `kejilion.sh` 内容未改变。
- 风险等级及理由：L3。认证协议与宿主机部署边界敏感，已执行 scoped 安全审计、固定 Runner L3、候选/main CI、发布扫描和真实本地 WebAuthn 回归。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.9]`：Passkey 注册/登录/撤销、TOTP 恢复因子策略、桌面首帧与紧凑重排修复、治理校验增强。
- 精确提交清单：`aea09c4a`（治理触发）、`b8ba15f4`（Passkey）、`cf533826`（桌面）、`b77be4ac`（版本与变更日志）、`62c98710`（Passkey scoped audit 证据）。
- 明确未纳入：生产部署、稳定 `latest`、应用市场默认入口、Passkey 真实硬件/目标服务矩阵、长期浏览器 soak，以及 rc8 已包含的 MCP 批量选择、空目录安装和集群临时排序候选的重复合并。

## 外部审计与修复交付

- 安全审计：scoped run-6，精确源码基线 `b8ba15f484c64d43241eaa3f2d4060d702b75fbd`；8 个受影响单元完成审计，5 个明确 out-of-scope，confirmed=0、needs_validation=0，final critic clean。证据位于 `C:/GitHub/_codex-evidence/kpanel-passkey-20260921/security-audit-run-6`，候选中归档于 `.governance/security-audit/run-6`。
- 覆盖检查：`node scripts/check-security-audit-coverage.mjs --validate` 通过；对当前 HEAD 的 decision 为 `scoped-required`，原因是历史边界包仍要求补审且 run-3/run-5 曾中断，预览规则记录该决定但不阻断本 RC；未审计当前新增提交数按该脚本决定记录为 0 个未覆盖 Passkey 单元。
- finding fingerprint / 修复 commit / 独立复核与回归证据：run-6 findings=[]；旧 run-5 的 TOTP 候选被 credential-version CAS 与回归验证 refute；真实浏览器报告 10/10 通过，未发现待修复 fingerprint。
- 修复交付状态：源码、RC、公开 GitHub Release 和 Docker preview 均包含修复；生产未部署。公开 Release workflow #241 成功，Tag `v1.21.0-rc.9` 指向产品提交。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：本 RC 使用来源候选已有 review 证据，release integration commit 记录 `OCR-Review: skipped reason=release integration preserves constituent candidate review evidence`；本次未把 skipped 伪造为通过，其他指标按历史周期规则延续。
- 按 `PROJECT_RULES.md` 5.4/5.5：观察周期不足三期，不提前宣称工具有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（本轮未修改 `kejilion.sh` 或 `kpanel.conf`）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：L3 `managed_script_contract=pass`，脚本 revision/hash 与镜像契约一致；候选 diff 未触及受管脚本。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Linux L3 完整 Go、Web 180 文件/1575 断言通过；最终候选真实浏览器 WebAuthn 10/10 通过。 | 未覆盖真实目标服务/硬件认证器和部署矩阵。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、Trivy 源码/依赖/镜像扫描均无阻断漏洞；SBOM/provenance 已生成。 | 依赖通告中不可达项按现有策略继续跟踪。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 Go/Web、核心 race、部署与应用配置生命周期、候选/main CI 和 Release workflow 均通过。 | 未做长期浏览器 soak；Windows L2 受平台工具和 Linux-only 测试限制。 |
| 性能与资源预算 | 已验证 | 本版没有新增轮询或持久后台资源；L3 镜像资源契约通过。 | 未做独立长期压力测试。 |
| 用户体验与可访问性 | 已验证 | mock UI 预览覆盖桌面、主题、紧凑视口和设置 Passkey 区域；真实浏览器覆盖 390/1280、浅深色、中英文、100%/200% CSS 布局缩放、键盘焦点和无水平溢出。 | mock API 的 Passkey 请求受模拟限制；未做真实硬件矩阵。 |
| 数据、配置与迁移 | 已验证 | Passkey 凭据版本 CAS、CSRF、撤销清理、TOTP 恢复因子和会话撤销均在真实本地回归通过；无 schema 迁移。 | 未在生产数据上执行写操作。 |

## 自动门禁

- 定向测试及结果：`npm run build`（3308 localized phrases、typecheck、Vite build）通过；Web Vitest 180 files / 1575 tests 通过；Passkey browser report 10/10，`cleanup=true`，errors=[]。
- L3 外层入口：run ID `v1.21.0-rc.9-62c9871-l3-r3`，2026-09-22 01:14:15 至 01:21:19 +08:00，exit 0；bundle SHA-256 `2025db0e9d9fcea6314fc948cfeb567b5c779730c10bf79e08a77508d2159152`，plan `17f936c3cb558c0458739a196525e41e09ca417921025bf3bed4f3cf5aa3fd0a`，remote entry `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，manifest `56411fc41e88f050bd7d39566927d2c868706a427c1fa2a0ddd9c737b38dfc36`；不可变 Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.9-62c9871-l3-r3`。
- 候选 CI：[CI 35631521437](https://github.com/kejilion/KPanel/actions/runs/35631521437) 与 [freshness 35631521244](https://github.com/kejilion/KPanel/actions/runs/35631521244) 成功；主线 [CI 35632676510](https://github.com/kejilion/KPanel/actions/runs/35632676510) 与 [freshness 35632676522](https://github.com/kejilion/KPanel/actions/runs/35632676522) 成功。
- annotated tag object `ac40c4500adc48b4303f8885a8b0f1788b666ea6` 指向候选提交；[Release workflow #241](https://github.com/kejilion/KPanel/actions/runs/35633673169) 成功。
- 安全扫描、镜像契约、SBOM/provenance：L3 与 Release workflow 均通过；Release workflow 生成双架构镜像和 provenance/attestation manifests。

## 依赖与技术栈变化

- 候选/main/tag dependency freshness 均成功；本版没有升级产品依赖、工具链、Action、基础镜像或受管脚本。
- 沿用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0；web lockfile 同步到 `1.21.0-rc.9`。
- Release workflow 的镜像基线和 Action SHA 固定在仓库 workflow；版本镜像与 preview manifest digest 均已公开复核。
- 未发现需暂缓的直接依赖；不可达依赖通告继续由 dependency freshness 跟踪。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：本机 Windows/amd64；本地 WSL2 runner 为 Linux/amd64、Docker，Chromium 153.0.8010.52，Go 1.26.7。
- 环境策略 ID 与允许用途：`local-wsl-dr`，仅 `candidate-validation`；`arena-154` 首次 L3 因 SSH 超时未执行候选代码。
- 使用的精确候选或公开产物：源码候选 `62c9871034da7ffcc5489927ee5ecc198bdfab82`；真实浏览器使用该候选编译的 `paneld.exe` SHA-256 `010be9908c5376b27fd9aa408616c428c8192419723d5606126d9e3e33e383fa`；mock 预览 manifest 同一 SHA。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 r3 终态 passed/exit 0，证据目录见上；浏览器报告 `C:/GitHub/_release-evidence/v1.21.0-rc.9-browser/browser-report.json`；mock 目录 `C:/GitHub/_release-evidence/v1.21.0-rc.9-preview-mock-retry`，已通过 stop 入口关闭 web/mock API。
- 测试窗口/循环数及风险依据：一次完整真实浏览器回归 10/10；未执行长期 soak，原因是预览版认证器与本地运行时验证已覆盖主要失败态，长期风险保留。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：真实报告覆盖 390/1280px、100%/200% CSS 布局缩放、浅深色、中英文、键盘焦点、>=14px 控件和 >=13px 辅助文字、撤销后拒绝登录、TOTP 缺少恢复因子拒绝、HTTP fallback；125% 浏览器 UI 缩放未单独执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：真实回归验证凭据撤销、CSRF、TOTP 清理；L3 应用配置生命周期与失败恢复通过；未进行生产宿主机写入。
- 未执行场景及原因：真实硬件认证器、目标代理/服务、arena-154 真实目标服务、长期 soak 和生产部署均未执行，预览策略禁止生产写入。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.9](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.9) 为非 draft、Pre-release、非 Latest；Release workflow #241 成功，GitHub Latest 仍为 `v1.20.0`。
- 16 个附件均已上传且非空，其中 `SHA256SUMS` 本地下载摘要为 `87757c510c8c11be747235afa85d22b8f53fa9d0a040d0cc6227a03dca4e921c`，包含 11 个二进制/部署包条目。
- Docker `1.21.0-rc.9` 与 `preview` OCI index 均为 `sha256:ebfc1e3a804032c38651b132fdb5adbf44dbc450714842221dae4bdf3686f676`；`linux/amd64` 为 `sha256:36196098ca51daeb3eb9447f355a7522bac5fdf78a170ea0b26e9f69e8a949a4`，`linux/arm64` 为 `sha256:67b996a7c8401d006f935703fced2c0c4f6957a0470cce5490ca99b6fc5fcec9`，另有两个 provenance/attestation manifests。
- 稳定 Docker `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`；GitHub Latest 仍为 `v1.20.0`。
- `image_e2e=pass`：Release workflow 的运行镜像契约、L3 部署/应用生命周期和公开镜像构建均通过；`kejilion/apps` 与生产入口未改动。

## 自更新通道验收

- 稳定来源继续只选择 GitHub Latest，预览来源选择 `v1.21.0-rc.9` 和 Docker `preview`；版本镜像与通道 digest 一致。
- 加入预览只切换来源并立即检查，不自动安装；自动安装与一次性立即安装仍独立。
- 旧状态迁移、退出预览降级保护、systemd/OpenRC 与轻量 Node 边界沿用既有契约；本版未修改自更新入口。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：仅 `local-wsl-dr` candidate-validation；`arena-154` 仅记录 SSH 超时，未写入。
- 正式部署环境：不适用；本轮没有生产写操作。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：0。
- 仅在隔离环境执行、未在生产执行的场景：完整 L3、mock UI 和真实本地 WebAuthn 回归。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`；上一预览 `v1.21.0-rc.8` / `8ba06cf85ff4d445db77b11cb28b72fdbb3ab44d`。
- 镜像 digest：稳定 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`；上一预览 digest 以 rc.8 验收记录为准；本版 `preview` 为 `sha256:ebfc1e3a804032c38651b132fdb5adbf44dbc450714842221dae4bdf3686f676`。
- 数据/配置备份：不适用，未部署生产。
- 回滚步骤和回滚后复核：预览异常时将预览来源固定回上一已验证 RC digest，重跑公开镜像 E2E；不改动 GitHub Latest 或 Docker latest。
- 回滚后生产实际版本与健康状态：不适用。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：保持 `v1.20.0` / `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- 公共默认更新通道决策：本次只更新显式 preview，不改变稳定默认通道。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-21T15:35:05+08:00
- 候选冻结时间：2026-09-22T01:14:15+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 未能连接 arena-154，候选代码未执行，证据无效。",
    "recoveryEvidence": "C:/GitHub/_release-evidence/v1.21.0-rc.9-62c9871-l3-r1 的 manifest 与 SSH timeout 输出；随后 local-wsl-dr r3 passed。",
    "permanentAction": "保留 local-wsl-dr 候选验证回退路径，并在下一次 L3 前复核 arena-154 SSH 可达性。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/local-wsl-dr/runner-image-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r2 在 runner ID 与实际镜像不一致时 fail-closed，未执行候选代码。",
    "recoveryEvidence": "r2 manifest 记录期望旧 ID；r3 使用实际 immutable ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d 并 passed。",
    "permanentAction": "下一次发布前同步 runner manifest 的 expectedRunnerId 与固定镜像 ID，并保留 fail-closed 校验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/preview-port/port-race",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 mock acceptance 的自动端口分配发生 4190 端口竞争，证据未采用。",
    "recoveryEvidence": "首次目录 C:/GitHub/_release-evidence/v1.21.0-rc.9-preview-mock 标记 failed；固定 5199/8199 的 retry manifest stopped 且 source commit 精确为 62c98710。",
    "permanentAction": "验收脚本使用显式未占用端口并在启动前做端口探测，保留失败证据而不覆盖。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：mock web/API 已通过 stop 入口关闭，真实浏览器 runtime 已停止；L3、浏览器和发布证据目录保留用于审计，未强删可再生缓存，净释放字节未记录。
- 未验证风险：真实硬件/目标代理矩阵、125% 浏览器 UI 缩放、长期浏览器 soak、生产部署健康和大规模节点压力。
- 已实现待实机准入：稳定版前以新的稳定候选重新执行 required security coverage、稳定 L3、生产灰度和回滚演练。
- 不阻断本版的理由：本版只进入 GitHub prerelease 与 Docker preview；固定 Runner L3、候选/main CI、Release 安全链、双架构 OCI 和真实本地 WebAuthn 回归均通过，稳定入口与生产未改变。
- 后续应进入的自动门禁或专项工作流：稳定版前补真实目标服务/硬件认证器矩阵、最终 SHA 的浏览器 UI 缩放矩阵和长期 soak；同步 runner expected ID 后再执行下一次 L3。
