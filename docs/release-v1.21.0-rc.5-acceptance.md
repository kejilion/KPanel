# KPanel v1.21.0-rc.5 发布验收记录

日期：2026-09-21

发布级别：L3

候选提交 / 标签：`573b17dace1f37b114ee6013701e236b5fea538d` / `v1.21.0-rc.5`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `573b17dace1f37b114ee6013701e236b5fea538d`

- 原分支 / 精确 tip / 处置分类：桌面分组链从 `feature/desktop-icon-groups` 的 `cb8aea2e...` 至 `fix/group-drop-hint` 的 `1455e947...`，共 13 条来源分支，均已纳入并归档；`feature/desktop-monitoring-processes` / `7e7b1c77...`、`fix/version-link-navigation` / `448f2fd8...`、`feat/settings-navigation` / `cf57ad2e...`、`fix/site-connection-recovery` / `b57af5ef...`、`docs/release-metrics-rc-view-20260920` / `f3d45ce...` 均已纳入并归档。
- 归档 ref 与 SHA：上述每条来源分支均存在对应的 `archive/<原分支>-20260920`；组装分支归档为 `archive/assemble/v1.21.0-rc.5` = `573b17da...`。远端逐 ref 复核一致，活动非归档 ref 仅保留 `main` 与 `release/v1.21.0-candidate`。
- 本次来源任务分支均已映射到冻结候选；18 条本地来源活动分支及 Git worktree 注册已回收。16 个路径因 Windows 下 `.vite`、`node_modules` 等忽略缓存未完成物理目录删除，但不再是 Git worktree，内容可由归档提交重建。
- `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 含未提交内容，按规范原样保留且未纳入。`docs/security-audit-run2-local-20260919` 含受限本地材料，保持未发布、未改动。
- 未完成归档项：无。候选分支按预览列车规则保留；上述可再生成的本地缓存目录留待独立缓存维护清理。

归档不代表生产上线；本版仅发布预览产物，生产未部署。

## 发布画像

- 业务域：桌面图标分组、桌面系统入口、设置导航、网站 CDN 任务恢复、发布治理指标。
- 变更面：桌面与设置展示和交互、现有 workspace 数据写入、任务状态恢复、发布报告；没有新增宿主机权限或生产部署。
- 受影响用户旅程：管理员整理桌面图标组、移动和重命名分组、打开监控与进程页面、从当前版本进入版本设置、搜索设置项、在 CDN 连接中断后恢复任务状态。
- 未变化契约：Panel/Agent 权限边界、端口、Compose、受管 `kejilion.sh`、稳定更新入口和应用市场稳定默认入口均未改变。
- 风险等级及理由：L3。桌面布局持久化与拖放状态组合复杂，设置导航影响长页面定位，CDN 恢复涉及异步任务真值；采用完整 Go/Web、核心 race、治理、双架构构建、镜像扫描、生命周期与公开 OCI E2E 收敛风险。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.5]`：可折叠桌面图标组、监控与进程入口、设置搜索和分类、可点击当前版本、导航粘滞、CDN 任务恢复与 RC 流程指标。
- 精确提交清单：`bf11a291` 至 `181fe87d` 的 20 个桌面分组提交、`c282a9a6`、`cdeb84ca`、`16b0bda5`、`713ccd49`、`17185c20`、`73f507a4`、`29c180a2`、`1f1eaf64`、`b92a97f9`、`af10d6e1`、`573b17da`。
- 明确未纳入：三个保留的旧本地工作项、生产部署、长期桌面交互 soak、最终人工多浏览器缩放矩阵，以及与本次功能无关的依赖升级。

## 外部审计与修复交付

- 本 RC 没有新增可引用的 Cloudflare `security-audit-skill` 最终结构化结论，也没有把 Alibaba open-code-review 的行级观察改写为发布门禁通过。
- 外部审计能力继续按 `PROJECT_RULES.md` 5.4/5.5 作为辅助证据：建议必须映射到精确源码、由项目测试和门禁独立复核，不能替代 L3、漏洞扫描或公开镜像验收。
- 本轮实际修复交付由来源提交、351 项定向 Web 测试、1527 项完整 Web 测试、完整 Go/race、治理 203/203、Trivy、govulncheck、npm audit、生命周期与 OCI E2E 证明。
- 当前稳定周期的候选数量仍不足以单凭外部工具命中率作退出结论，继续观察有效、skipped、unreported 和 constrained-only 状态。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`，沿用上一候选。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：本轮没有修改 Dockerfile 中的受管脚本 revision/SHA、脚本协议或调用契约；L3 应用生命周期和公开镜像 E2E 通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 桌面分组、布局持久化、监控/进程入口、设置定位、CDN 恢复的定向测试和完整 Go/Web 测试通过。 | 未做多日桌面交互 soak。 |
| 网络入侵与供应链安全 | 已验证 | 无新增权限；govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/镜像 0 阻断项，Release 生成 SBOM/provenance。 | 3 个模块通告不可达，继续由每日审计跟踪。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、CDN 重连真值、资源版本、应用安装/更新/回滚/卸载生命周期和公开镜像启动通过。 | 未执行长时间弱网反复断连 soak。 |
| 性能与资源预算 | 已验证 | 桌面位置预算 512、组内网格和过渡有界；没有新增后台轮询；完整构建与测试通过。 | 未做大量图标的独立帧率基准。 |
| 用户体验与可访问性 | 已验证 | 当前版本可点击并定位版本区，设置搜索、分类和粘滞导航有组件回归；reduced-motion、键盘和多语言契约保留。 | 未另做真实浏览器 100%/125%/200% 人工矩阵。 |
| 数据、配置与迁移 | 已验证 | 分组复用现有 workspace 资源和版本保护；旧工作区无分组时保持原布局；无 Panel schema 迁移。 | 跨多个历史 workspace 的人工迁移抽查未执行。 |

## 自动门禁

- 定向测试：15 个前端测试文件 / 351 项通过；完整前端 174 个文件 / 1527 项通过；治理一致性 203/203、16 个提案、11 个依赖组通过。
- L3 Runner：`kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；Go 全量、核心 race、Web、typecheck、翻译、构建、govulncheck、npm audit、Trivy、双架构二进制、镜像与 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：`v1.21.0-rc.5-573b17d-l3-r1`，2026-09-20 23:23:34 至 23:42:04 +08:00，exit 0；bundle `27c8b9d4...`、plan `1e87d3c1...`、remote entry `d8bb2cf2...`、远端日志 `15d38883...`。证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.5-573b17d-l3-r1` 和 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.5-573b17d-l3-r1`。
- 候选 CI：[CI 35520455814](https://github.com/kejilion/KPanel/actions/runs/35520455814) 与 [freshness 35520455846](https://github.com/kejilion/KPanel/actions/runs/35520455846) 成功。
- 主线 CI：[CI 35520877668](https://github.com/kejilion/KPanel/actions/runs/35520877668) 与 [freshness 35520877679](https://github.com/kejilion/KPanel/actions/runs/35520877679) 成功。
- annotated tag object `7ba58c904b034b31bd53154b0b7a24776acb71c1` 指向产品提交；[Release 35521289302](https://github.com/kejilion/KPanel/actions/runs/35521289302) 与 [tag freshness 35521289311](https://github.com/kejilion/KPanel/actions/runs/35521289311) 成功。
- Release 生成 SBOM/provenance，原生镜像 Trivy、运行时契约和双架构发布均通过。

## 依赖与技术栈变化

- dependency policy validate-only、候选/main/tag freshness 均通过；本版没有依赖或基础镜像升级。
- Go 1.26.7、Node 24.20.0、Trivy 0.72.0、固定 Action SHA 与受管脚本基线沿用项目冻结工具链。
- 3 个 govulncheck 模块通告不可达，npm audit 和 Trivy 无阻断项；继续由每日安全审计跟踪，不在本 RC 混入无关升级。
- 版本、锁文件、Release 资产摘要和公开 OCI digest 已核对；回滚点保持 `v1.20.0`。

## 隔离真机与浏览器验收

- 主机：`arena-154`，Linux/amd64、Docker；环境策略允许 candidate-validation，禁止 `prod-108`。
- 固定 Linux L3 使用精确源码 `573b17da...`；公开 OCI 使用不可变摘要 `sha256:5ee9bb3c24f58b3ad28cfb5594831ec2b4037ee4dae26b73fb71cbc2f9283740`，镜像 version/revision 精确匹配。
- 公开镜像在临时端口 `18185` 执行项目标准 `packaging/tests/image-e2e.sh`；输出 `image_e2e=pass`、`public_oci_e2e=pass`，临时容器、网络和远端目录已清理。
- 公开镜像证据：源码 archive `0812be22...`、执行脚本 `59b09c8d...`、日志 `64d9626e...`，位于 `C:/GitHub/_release-evidence/v1.21.0-rc.5-public-image-e2e-r1`。
- 设置与桌面用户旅程由组件、布局、键盘、reduced-motion 和多语言自动测试覆盖；未执行最终 SHA 的人工多浏览器缩放矩阵、长期桌面 soak 和弱网反复断连 soak。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.5](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.5) 为非 draft、prerelease、非 Latest；发布时间 2026-09-21 00:09:15 +08:00，GitHub Latest 仍为 `v1.20.0`。
- 14 个附件均为 uploaded 且非空；`SHA256SUMS` 自身摘要 `8e747d79...` 与 GitHub API digest 一致，11 个二进制/部署包条目与 API 摘要逐项一致。
- Docker `1.21.0-rc.5` 与 `preview` OCI index 均为 `sha256:5ee9bb3c24f58b3ad28cfb5594831ec2b4037ee4dae26b73fb71cbc2f9283740`。
- `linux/amd64` / `linux/arm64` 分别为 `sha256:d5e801e8b0fc49c70da2ecd4afeda876fff6346ce5f9e70b2f119354f6ba6c45` / `sha256:41bad7519ffcb726e7e5f598aaac094a1a60522c06bb85fbc0e194280db8e4cf`；另外两项为 attestations。
- 稳定 `latest` 与 `1.20.0` 均保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。

## 自更新通道验收

- 稳定来源只接受正式 GitHub Latest；预览来源接受同列车 RC，也会在没有更高 RC 时接受更高稳定版本。现有更新选择测试通过。
- 加入预览只切换来源并立即检查，不自动安装；自动安装和一次性立即安装相互独立。
- 旧状态默认迁移到 `stable`，重启后通道保持；退出预览不会把较低稳定版作为降级候选。
- systemd 更新执行、更新前备份、失败恢复和失败版本隔离由 L3 生命周期覆盖；OpenRC 与轻量 Node 边界按 `docs/release-channels.md` 保持。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证环境：仅 `arena-154` 的隔离 L3 和公开 OCI E2E。
- 正式部署环境：不适用。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前后版本、备份、入口和生产数据：不适用。
- 生产已执行写操作：0。
- 仅在隔离真机执行：L3、公开镜像拉取和 E2E。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c...`；上一预览 `v1.21.0-rc.4` / `0c566418...`。
- 镜像 digest：稳定 `sha256:a991b5d2...`；上一预览 `sha256:07b67e3b...`。
- 数据/配置备份及生产回滚：不适用，未部署生产。
- 需要回退预览通道时重新固定上一 RC digest；GitHub Latest、Docker `latest` 和稳定更新入口继续指向 `v1.20.0`。
- 公共默认更新通道决策：不适用，稳定通道未改变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-20T18:04:38+08:00
- 候选冻结时间：2026-09-20T23:43:05+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：20
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-assembly/cherry-pick/missing-parent",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次治理提交序列遗漏父提交，产生 modify/delete 冲突；组装在提交前中止，没有改变候选或远端。",
    "recoveryEvidence": "安全 abort 后按完整父子顺序重放，冻结候选拓扑、治理 203/203、候选和 main CI 均通过。",
    "permanentAction": "候选组装前固定用 merge-base 与 reverse log 生成来源提交序列，并在首个 cherry-pick 前核对父提交可达性。",
    "historicalReleases": []
  },
  {
    "fingerprint": "governance-validation/freshness/stale-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选累计 62 个提交后当前业务事实超过新鲜度阈值，治理门禁按设计阻断。",
    "recoveryEvidence": "将业务事实基线刷新到 rc.4 验收提交后，治理一致性、候选/main/tag freshness 全部通过。",
    "permanentAction": "候选组装在提交数接近阈值时先运行 freshness，并把业务基线刷新作为冻结前固定检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "web-validation/test-invocation/wrong-workdir-prefix",
    "position": "before-production-write",
    "count": 1,
    "impact": "从 web 目录运行测试时仍带 web/src 前缀，Vitest 返回 No test files found，没有形成错误通过结论。",
    "recoveryEvidence": "改用 src/... 路径后 15 个文件 / 351 项定向测试及 174 个文件 / 1527 项完整测试通过。",
    "permanentAction": "前端定向测试命令从当前工作目录解析路径，并在记录结果前要求测试文件数和用例数均大于零。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-gate/local-preflight/missing-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 本机缺少 go、gofmt 和 make，L2 预检明确失败且未产生部分通过声明。",
    "recoveryEvidence": "转入固定 Linux Runner 完成 L3，run v1.21.0-rc.5-573b17d-l3-r1 exit 0，完整日志和摘要已保留。",
    "permanentAction": "发布门禁固定优先使用具备冻结工具链的 Linux Runner，本机只做可用性预检和定向 Web 验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "worktree-cleanup/windows/ignored-cache-residual",
    "position": "before-production-write",
    "count": 16,
    "impact": "16 个 clean 来源 worktree 因 .vite、node_modules 等忽略缓存导致 Windows 物理目录删除未完全完成；Git worktree 注册和本地活动分支均已安全释放。",
    "recoveryEvidence": "18 条来源分支的远端 archive ref 已先复核；git worktree list 不再包含来源路径，本地来源活动分支列表为空。",
    "permanentAction": "后续 worktree 回收在解除注册前先使用项目缓存清理入口删除忽略缓存，并将物理目录清理作为独立可重试步骤。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：18 条来源 Git worktree 注册和活动分支已回收；16 个路径只剩可再生成缓存，因 Windows 删除限制跳过物理清理。L3 和公开 OCI 证据保留在 `C:/GitHub/_release-evidence/` 与 `arena-154:/root/kpanel-release-evidence/`。
- 未验证风险：大量图标下的长期帧率、最终人工浏览器多缩放矩阵、跨历史 workspace 人工迁移、弱网反复断连 soak。
- 已实现待实机准入：桌面分组与设置导航应在稳定版前补最终候选的多浏览器缩放和长时间交互抽查。
- 不阻断本版的理由：本版只进入预览渠道；固定源码已通过 L3、候选/main/tag freshness、Release 安全链、14 个公开附件和不可变公开 OCI E2E，稳定入口与生产均未改变。
- 后续应进入的自动门禁或专项工作流：为设置导航补浏览器滚动/锚点矩阵，为桌面分组补大量图标性能基准，并继续观察外部审计有效性与重复流程指纹。
