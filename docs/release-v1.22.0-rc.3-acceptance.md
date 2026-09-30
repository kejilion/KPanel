# KPanel v1.22.0-rc.3 发布验收记录

日期：2026-09-24

发布级别：L3。预览产物已公开；隔离主机不可达导致公开镜像冷启动 E2E 未验证，生产未部署。

候选提交 / 标签：`5796a0c9a379d17316a921a7ef3bf3cc9ccdcaa3` / `v1.22.0-rc.3`。Annotated tag object `e8cc1643095c113b5e5d98b423d897ace237be5b` 解引用到该提交。

上一稳定版本 / 回滚点：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3`；Docker `latest` 为 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。

`releaseChannel`：`preview`；`releaseTrain`：`1.22.0`。

候选分支与发布后处置：远端 `release/v1.22.0-candidate@5796a0c9` 保留，供同序列后续 RC 使用；发布时远端 `main` 同为 `5796a0c9`。候选 worktree `C:/GitHub/_codex-tasks/kpanel-v122-rc1-public` 保留且干净。

- 本次基线：`origin/main=31565f3ebaf902d3242483c1491c9d3ba8fe607f`；候选增加 7 个提交，未将其他 worktree 的 dirty 内容纳入。
- 已纳入的本地来源分支已在原 worktree 中归档，并核对旧活跃 ref 消失、归档 ref 精确 tip 保留、无对应远端活跃 ref：`archive/codex/settings-compact@32cab0ac` 对应候选 `c3817c67`；`archive/codex/monitoring-status-grid-wide@b1e806cc` 对应 `a7815634`；`archive/fix/overview-monitoring-processes-navigation@8716041a` 对应 `2e6775cd`，候选另有范围修正 `1c891ec8`。三对原始补丁的 stable patch-id 分别相等。
- 三个来源 worktree 保留且干净；归档仅移动本地分支引用，未删除 worktree 或唯一证据。用户已归档的动态场景实现 `archive/feature/desktop-scenes-gpu-finish@05ebce59` 未动；dirty 的 `feature/desktop-dynamic-scenes` 保留并排除。
- 编辑器工作区、Terminal file transport v3 等独有后续工作未纳入。文件主机选择、AI JSON Accept 和监控性能补丁已在发布基线 `origin/main` 中，不重复 cherry-pick。
- 本次没有未完成的已授权来源分支归档；其他任务分支归属和后续版本范围不在本次清理授权内。

## 发布画像

- 业务域：Overview 内导航、历史监控状态矩阵、设置搜索结果反馈；`v1.21.0` 后主线已集成的功能随 RC3 产物一并发布。
- 变更面：可见界面和只读导航。RC3 增量未改变 API、数据 schema、端口、Compose、Agent 权限、安装或更新契约。
- 受影响旅程：Overview 打开历史监控与进程管理并返回；服务检测状态矩阵按宽度排 1/2/3 列；设置搜索在缩短页面后保持辅助技术播报。
- 风险：中。桌面窗口导航可能误开新窗口，宽屏状态格可能溢出，设置摘要可能从辅助技术树消失。真实 Panel/Agent 与公开镜像冷启动仍需隔离验收。

## 发布范围与未纳入内容

- RC3 增量提交：`c3817c67`、`a7815634`、`110d58e6`、`2e6775cd`、`ec6bcff8`、`1c891ec8`、`5796a0c9`；完整差集由 `git log 31565f3e..5796a0c9` 复现。
- `1c891ec8` 将 Overview 导航修复限制在 `/overview` 来源，保留其他桌面应用进入监控/进程的独立窗口行为；定向回归 10/10 通过。
- 桌面动态场景、编辑器工作区、Terminal file transport v3 的本轮未批准增量均未纳入。

## 外部审计与修复交付

- `check-security-audit-coverage.mjs --target 5796a0c9`：`decision=ok`，最近 full `run-4`、scoped `run-6/run-7`；未审计提交 4 个、文件 7 个、最老 0 天，无新边界包。预览版只记录，不把覆盖检查解释为新 CF 审计。
- OCR 1.12.6 观察区间 `31565f3e..1c891ec8`，7/7 个代码文件 reviewed，`CHANGELOG.md` 与锁文件另经人工检查；自由臂有效 `H0/M0/L1`，低级范围问题已由 `1c891ec8` 修复。`blind=false`、`constrained-only=unreported`，不声称约束臂有效性。证据位于 `C:/GitHub/_codex-tasks/kpanel-v122-rc3-ocr-1c891ec8`，最终 trailer 为 `5796a0c9`。
- 本稳定周期 OCR 观察仍不足三个周期，不提前得出工具长期有效或退出结论。没有将未动态核验的审计线索写为已修复。

## 跨仓库联动判定

- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集编号、脚本候选和依赖阻断范围均不适用。
- 镜像内受管脚本基线：`kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`；L3 受管脚本契约通过。本次不发布脚本。
- `packaging/kejilion-app/kpanel.conf` 相对 `v1.21.0` 没有差异；`kejilion/apps@39b498a0` 工作树干净，默认更新仍为 `latest`，无需 apps 提交。两个仓库的 `app_url` 有先前形成的官方地址差异，本次未修改安装/更新通道契约，未用空提交掩盖它。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Overview、监控、设置四条 Mock 浏览器旅程通过；Go/Web 全量门禁通过 | 真实双 Panel、轻量 Node 和公开镜像冷启动未做 |
| 网络入侵与供应链安全 | 已验证 | L3 与 Release 的 govulncheck、npm audit、Trivy、受管脚本和运行时契约通过 | 未审计的 4 个提交按 RC 覆盖策略记录；真实反代未做 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 候选/main CI、race 和应用生命周期通过；搜索空结果可恢复 | 隔离真机重启、故障注入及公开镜像 E2E 未做 |
| 性能与资源预算 | 已实现未实机验证 | L3 镜像构建通过；监控矩阵 9 行在三种宽度无页面横向溢出 | 真实节点长期资源与 P95 未采集 |
| 用户体验与可访问性 | 已验证 | 本地 Mock 真浏览器核对 1/2/3 列、同页导航、返回历史、深浅主题、搜索播报和空状态；控制台 warn/error 为 0 | 200% 浏览器缩放与本次汇总预览的英文界面未验证 |
| 数据、配置与迁移 | 不适用 | 本轮无数据格式、配置 schema 或安装迁移 | 不以 Mock UI 证明真实数据恢复 |

## 自动门禁

- 定向桌面导航回归 10/10；L3 中 Go 全量测试、Web 类型检查和 183 个 Vitest 文件 / 1605 项测试通过。
- L3 唯一外层入口 `node scripts/run-release-l3.mjs`：run `v1.22.0-rc.3-5796a0c9-l3-r1`，目标 `local-wsl-dr`（仅 candidate-validation），外层与远端均 pass、退出码 0；证据目录 `C:/GitHub/_codex-tasks/kpanel-v122-rc3-l3-r1`。不可变 Runner `kpanel-release-gate:go1.26.7-node24@sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。plan SHA-256 `6655fad9de71f66aa984f7644bf51af8686c7598db6ac32130893061cd59ff75`，remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，bundle `0e512f68eb91475fc18f3e2d810a10567c01ebaded7a2977e23ad726c9225c6e`，manifest `15ee7b47adc8a3f37ee84b73c1538cdd06f193fb43b5507e7f22c08a74edc98a`。
- 候选 [CI 1061](https://github.com/kejilion/KPanel/actions/runs/35882974886) 与依赖新鲜度 [559](https://github.com/kejilion/KPanel/actions/runs/35882974829) 成功；主线 [CI 1062](https://github.com/kejilion/KPanel/actions/runs/35884076536) 与依赖新鲜度 [560](https://github.com/kejilion/KPanel/actions/runs/35884076587) 成功，均对应 `5796a0c9`。
- [Release workflow 250](https://github.com/kejilion/KPanel/actions/runs/35886574385) 成功；源码扫描、双架构 Agent/Node 二进制、原生镜像扫描与运行时契约、Docker 多架构推送和 `preview` 提升均成功。候选归档步骤因 preview 通道按设计跳过。
- L3 govulncheck 未发现可达漏洞，另报告 1 项无可达调用的模块级记录；npm audit 0；Trivy 最终镜像漏洞、secret 和 misconfig 均为 0。

## 依赖与技术栈变化

- 本版未采用依赖、工具链、基础镜像、Action、扫描器或受管脚本升级；`web` package/lock 只同步版本号。候选、主线依赖新鲜度自动检查均成功。
- 本地在线报告在 2026-09-24 00:17 左右生成但退出 2，仅 6/10 个检测源成功：缺本机 Go，Docker 基础镜像、安全工具和 Dockerfile frontend 上游读取失败。报告 `C:/GitHub/_codex-tasks/kpanel-v122-rc3-dependency-report.json`（SHA-256 `507f439a7e9db9e8067a5a61b60111253c17520390967cb84ff86369dc78fb6f`）只作部分信号，不写成完整检测或升级决策。
- 部分报告显示 24 个直接行动候选和 142 个传递信号，不能推断缺失来源为无更新。基座维护任务在可用 Go/网络环境补完整报告后按策略分级；本次不临时扩大版本范围。

## 隔离真机与浏览器验收

- 本地预览 `rc3-desktop-monitoring-settings-1790179100742-4b0652`：clean 候选 `5796a0c9`，Mock、acceptance、visual 画像；地址 `http://127.0.0.1:4173`，manifest `C:/GitHub/_codex-tasks/kpanel-v122-rc3-preview-r2/manifest.json`，浏览器记录 `browser-acceptance.json`（SHA-256 `d0af33e7ac22175f047e1dc14c90673586bf51184a3c6797d6d242bd1d841514`）。原预览 r1 在接续任务时 `liveState=not-ready`，r2 同 SHA 重启后完成验收。
- Codex 内置真实浏览器：Overview 历史监控与进程管理均复用当前标签，后退返回 Overview；状态矩阵容器宽 478/908/1378px 时分别为 1/2/3 列，9 行，无页面横向溢出。状态标签与图例计算字号为 12px；深/浅主题目视正常。
- 设置搜索“主题”时 `role=status`、`aria-live=polite` 播报“找到 1 项设置”，视觉裁剪至 1×1px；无匹配查询显示可恢复空状态，清除后恢复 14 项设置。焦点在搜索框可见，控制台 warn/error 记录为空。
- 内置浏览器的 `ctrl+plus`/`ctrl+equal` 未改变缩放比例，200% 缩放未验证；本次汇总预览未重新核对英文和完整键盘路径。Mock 不证明真实 Panel/Agent、Docker、宿主机写入或数据恢复。
- 登记隔离环境 `arena-154` 在候选冻结及 Release 发布后均 SSH 端口超时；本次未在该主机运行公开镜像 E2E、真机故障注入、重启或 soak。`local-wsl-dr` 仅用于候选 L3，未把它写成隔离真机或公开镜像门禁通过。

## 发布产物与公开仓库复核

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.3) ID `394843301`，2026-09-24 00:21:04+08:00 公开，`draft=false`、`prerelease=true`；GitHub Latest 仍为 `v1.21.0`。
- Docker `1.22.0-rc.3` 与 `preview` OCI index 同为 `sha256:d723f4117d5972c6194f478a2f413b67b9c222982d9bb9d7f996490f5ad22e4e`。`linux/amd64` 为 `sha256:41ea0f5e52759a9f9f29ff2427e3148598f7d2c44e7134afb89fa6f8daeaf3be`；`linux/arm64` 为 `sha256:b4e7fe8182138030f874a8166d732d3d6dd713dd2c22f9807d83a0239180f22d`。两个 `unknown/unknown` 条目属于附加证明，不替代双架构。
- Docker `latest` 仍为 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`；发布前 `preview` 为 `sha256:7c540253e0091c11c76d2be67561c4e17c1a7d5a42a5c320458bc4d67d86f860`。
- Release 14 个附件包含 amd64/arm64 Agent、部署归档和 `SHA256SUMS`；清单 11 项逐一匹配 GitHub asset digest。`SHA256SUMS` 自身 digest `sha256:44dfe65ab0ba75174183732afdc1ec1dce7ab9739a191f570326d63567f540a0`。
- 公开镜像 `image_e2e`：未验证，原因是 `arena-154` 持续不可达；Release 的原生镜像运行时契约通过，但不能代替公开镜像拉取和冷启动。应用市场无需本次提交，默认稳定入口保持 `latest`。

## 自更新通道验收

- Release/镜像公开事实证明 RC 只进入显式 `preview`，Latest/`latest` 未变。来源选择、自动安装开关、退出预览不自动降级和失败恢复实现本次未改变，L3 既有通道契约检查通过；真实已安装测试实例的加入、退出与降级保护本轮未重测。
- systemd 后台更新、OpenRC 与轻量 Node 的边界沿用 `docs/release-channels.md`；不将自动测试表述为生产更新完成。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）；本次生产写操作 0。没有生产目标、备份、部署命令、升级后健康或回滚后的现场状态。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- `arena-154` 仅作为登记隔离验证目标尝试连接，连接超时；没有在该主机执行灰度或生产动作。

## 回滚

- 已发布的 Tag、Release 和不可变版本镜像保持可追溯。若需撤销源码变化，基于 `v1.21.0` 建新修复提交并重走候选门禁；不移动既有 `v1.22.0-rc.3` 标签。
- 若需退出本次预览通道，将 Docker `preview` 恢复到发布前精确摘要 `sha256:7c540253e0091c11c76d2be67561c4e17c1a7d5a42a5c320458bc4d67d86f860` 并复核公开摘要；本次未执行回滚。安装 RC 的测试实例如需降级须显式指定旧摘要并按更新事务备份/恢复，退出预览来源不会自动降级。
- 数据/配置备份和回滚后生产健康：不适用。GitHub Latest=`v1.21.0`、Docker `latest`=`sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`、应用市场默认入口=`latest`，本次均未提升。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-23T18:38:08+08:00
- 候选冻结时间：2026-09-23T23:17:56+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

RC2 的 Release workflow #249 曾失败，因此本次使用新的 rc.3 不可变标签；没有覆盖 rc.2 标签、Release 或镜像，也未发生生产回滚。下列计数只覆盖本次 RC3 的流程与证据异常。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：5
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser/cua-service/fetch-failure",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮浏览器控制服务读取失败，不能把 HTTP 200 或 Mock 预览 ready 冒充真实浏览器验收。",
    "recoveryEvidence": "用户要求重试后服务恢复；同一候选 SHA 的 r2 预览由 Codex 内置浏览器完成四条旅程，browser-acceptance.json 记录结果。",
    "permanentAction": "发布任务负责人在下一 RC 浏览器验收前检查控制服务可用性；若再次失败，保留错误并停止该证据通道，至服务恢复且有真实浏览器记录才退出例外。复核日期 2026-09-30。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview/local-feature-preview/process-stopped",
    "position": "before-production-write",
    "count": 1,
    "impact": "接续任务时 r1 manifest 仍记 ready，但两个预览 PID 已退出，旧 URL 拒绝连接。",
    "recoveryEvidence": "权威 status 给出 liveState=not-ready；同一 5796a0c9 在独立 r2 目录重启并完成浏览器验收，未覆写 r1 证据。",
    "permanentAction": "发布任务负责人在跨回合继续本地预览前固定执行 status 并确认 PID/HTTP；下一 RC 核查进程持久性，复核日期 2026-09-30。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/arena-ssh/connection-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记隔离主机多次 SSH 超时，公开镜像冷启动 E2E 和隔离真机专项不能执行。",
    "recoveryEvidence": "未恢复；候选冻结时与公开 Release 后的 arena-154 连接检查均超时。L3 灾备 local-wsl-dr 只证明候选门禁通过。",
    "permanentAction": "发布负责人在稳定版冻结前恢复 arena-154 连通性或先按环境策略登记等价隔离主机，并以公开镜像拉取/冷启动 E2E 通过为退出条件；本指纹在 v1.22.0-rc.1 已出现。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependency/collector/partial-sources",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机在线依赖报告退出 2，仅 6/10 来源成功，不能作为完整新鲜度证据。",
    "recoveryEvidence": "部分报告与 SHA 保留；候选/main 的自动依赖新鲜度检查通过，本版不采用升级，未把失败来源推断为无候选。",
    "permanentAction": "基座维护负责人在 2026-09-30 前使用具备 Go 与可达上游的固定环境重跑 10/10 报告，核对直接行动项后才退出部分报告例外。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-visual/zoom/unsupported",
    "position": "before-production-write",
    "count": 1,
    "impact": "内置浏览器的缩放快捷键未生效，visual 画像中的 200% 浏览器缩放证据缺失。",
    "recoveryEvidence": "未恢复；记录 devicePixelRatio 与 CSS 视口未变，同时以实际 550/1280/1750px 视口核对布局和无横向溢出，不冒称等同 200% 缩放。",
    "permanentAction": "发布任务负责人在稳定版验收前用可控 200% 缩放的浏览器补测受影响旅程并存证；完成前保持未验证。复核日期 2026-09-30。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 公开预览产物已发布，隔离真机和公开镜像 `image_e2e` 未执行；200% 缩放、真实多主机互通、长期资源与更新退出行为未验证。稳定版前补齐，RC 反馈期不将这些场景称为通过。
- 本地资源回收：已归档三个 clean 来源分支引用，未删除 worktree。保留 RC 候选工作树、浏览器预览 r2、L3/OCR/部分依赖报告和失败 r1 状态供复核；未触碰 dirty 动态场景、其他活跃任务或唯一证据。预览结束后由本发布任务用 manifest 给出的停止命令回收进程；当前尚未停止，净释放空间未计算。
