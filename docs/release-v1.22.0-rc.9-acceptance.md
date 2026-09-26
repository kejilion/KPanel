# KPanel v1.22.0-rc.9 发布验收记录

预览产物已发布；公开镜像的本地 E2E 通过，但登记隔离环境验收尚未完成。纯验收提交独立走 CI 与归档，不改变产品标签。

日期：2026-09-26。发布级别：L3。releaseChannel：preview；releaseTrain：1.22.0。

- 候选提交 / 标签：f727155f9e9e33a140886a6fdec32800f0f7ff5c / v1.22.0-rc.9（tag object 9cd991ce68b61b5f117875e51ab5b3297a420b24）。
- 发布前 main：3d7c28d13b039b4e86a551894078c39b7e334178（RC8 验收记录）。
- 上一已发布预览 / 回滚点：v1.22.0-rc.8，25a4655adb16040e17cf2136f627bad10be23ec7；镜像 index sha256:626122acfa5c467aba66efcac751498201924b7638643ad2c3e5904e8470de85。
- 上一稳定版：v1.21.0，396fcd62c5635c9812ee97ee509cb61b962724f3；镜像 index sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。
- 本地组装时分支：release/v1.22.0-rc.9-assembly；当前验收分支：docs/release-v1.22.0-rc.9-acceptance。工作树：C:/GitHub/_codex-tasks/kpanel-v1.22.0-rc.9。
- 证据根目录：C:/GitHub/_release-evidence/v1.22.0-rc.9-20260926；L3：C:/GitHub/_release-evidence/v1.22.0-rc.9-f727155f-l3-r1；OCR：C:/GitHub/_review-evidence/ocr-rc9-745ac92。

候选分支与发布后处置：release/v1.22.0-candidate / 预览版保留（project-management.md 10.2）。

- 原分支 / 精确 tip / 处置分类：远端 release/v1.22.0-candidate 由 25a4655a 快进至 f727155f，预览期保留。
- 归档 ref 与 SHA / 远端复核结果：本轮预览不归档序列候选；来源分支处置见“分支与资源处置”。
- 本次来源任务分支：见范围表，均以精确 tip cherry-pick（-x）纳入。
- 本地分支/upstream/worktree：来源工作树单独核对；`feature/custom-wallpapers-20260925` 有 1 个未跟踪文件，内容和所有权未确认，原样保留。其他来源工作树当前 clean；不以此推定所有者已释放。
- 未完成归档项 / 责任人 / 下次复核触发条件：来源分支由发布任务在所有权、精确重放映射和远端状态核实后按 10.2 处置；未确认项保留。验收分支在同 SHA 候选 CI、主线快进及主线 CI 成功后归档。

产物已发布，生产未部署（预览版禁止生产部署）。

## 发布画像

- 业务域：桌面外观（壁纸上传、登录页壁纸、3D 场景渲染）、应用市场安装/更新事务、集群轻量节点状态恢复。
- 变更面：展示；协议或数据（新增 /api/v1/desktop/wallpapers 与 `<DataDir>/desktop-wallpapers` 存储）；宿主机写入（kpanel.conf 更新事务与回退快照、轻量节点状态文件恢复）；部署不变。
- 受影响用户旅程：管理员上传/选择/删除自定义壁纸；登录与首次设置页显示所选壁纸；打开 3D 场景；通过应用市场脚本手动/自动更新 KPanel 及中断恢复；轻量节点崩溃后重启。
- 未变化契约：端口、Compose、Agent 权限、受管 kejilion.sh、应用市场默认入口（latest）。kpanel.conf 源文件有变化，但本轮不同步到 kejilion/apps（见跨仓库章节）。
- 风险等级及理由：中。新增一个需会话的上传写接口和一个新信任边界包；更新事务变更只在仓库脚本内生效，未推到用户安装入口。

## 发布范围与未纳入内容

用户可见更新见 CHANGELOG 1.22.0-rc.9 节。

| 来源 | 纳入提交 | 范围 |
| --- | --- | --- |
| fix/update-backup-parity-20260925，tip a52553bf | 5219b8da → eb2ff2d1；9a154b4b → eb08665e；a52553bf → 206511dc（空提交，独立复核 trailer） | 手动/自动更新共用快照与恢复事务 |
| fix/light-node-crash-recovery-20260925，tip f3bdd584 | f3bdd584 → 611cc293 | 轻量节点中断后恢复，不删凭据 |
| feature/custom-wallpapers-20260925，tip 426369ff | f73bce3d → 30e67f14；220df500 → a5ccfa5a；426369ff → 0a565f08 | 自定义壁纸上传与验收修复 |
| feature/login-wallpaper-20260925，tip 2949c1b5 | 07cc5be2 → c6e9b412；2949c1b5 → 4c77e7b6 | 登录页壁纸、私有壁纸本地副本 |
| feature/desktop-3d-scene-packs-20260924，tip 885adae4 | 885adae4 → ffac7f97（补 OCR trailer） | 场景 Worker/OffscreenCanvas 渲染 |
| RC9 集成 | 9e2f6c98；f727155f | 场景包版本升级；RC9 版本与更新说明 |

- 该场景分支前两个提交 03ba38d8/1a5e52d9 已以 46f35030/5a8a0bc6 随 RC8 发布（range-diff 仅 catalog 尺寸与 cherry-pick 注记），未重复纳入。
- 集成冲突：CHANGELOG 按“Unreleased 在前”合并；scene-packs/README.md 两侧保留（Worker 建议 + ETag 说明）；catalog.json 由 `npm --prefix web run scene-packs:build` 重新生成，`node scripts/check-scene-packs.mjs` ok，dist 与源码复现一致。
- 集成发现：Worker 提交改变三个官方场景内容但未升 version，已安装用户不会收到更新提示（违反 scene-packs/README.md 第 3 节）。9e2f6c98 升级为 neon-city 1.1.1、orbital-station 1.0.2、sea-and-sky 1.0.2。
- 明确未纳入：feat/terminal-file-transport-v3（待独立复核、真机/反代验证与 SSE 撤销窗口修复）；feature/desktop-dynamic-scenes、feature/desktop-live-wallpaper-20260924、design/files-solid-icons-20260924（陈旧或已被替代）。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：本轮未新增 CF 安全审计 run；继承 full run-4（源 4c0694aa）与 scoped run-9/7/6。wallpaper 功能来源记录为 Security-Audit deferred。
- 覆盖检查：decision=scoped-required，reason=new boundary packages: internal/desktopwallpapers；未审计提交 7、文件 10，最早 0 天（上限 14）。预览版只记录不阻断；稳定版 1.22.0 预检前须以 next_scoped comparison_base=4c0694aa 补 scoped 审计或按规则记录用户决定。run-3/5 中止不算覆盖。
- finding fingerprint / 修复 commit / 独立复核与回归证据：不适用（本轮无安全审计 finding）。
- 修复交付状态：不适用。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：本轮适用候选 5 组。backup parity 3d7c28d..9a154b4（2/2，H0/M0/L0，constrained-only=unreported）；light node 3d7c28d..863c33c（2/2，H0/M0/L0，0）；custom wallpapers 6b768c2..f73bce3（25/25，free-form=3，H0/M2/L4，0）及 220df50..72370cf（4/4，0）；login wallpaper 426369f..4c0478f（4/4，M1/L1，0）与 07cc5be..bed0869（7/7，M1，0）；Worker 渲染 4c77e7b..745ac92（24/24，先自由臂盲跑，H0/M0/L0，constrained-only=0，dist/catalog 以构建复现核验）。skipped 0。场景版本遗漏由集成阶段发现，不归功于 OCR。
- 5.4/5.5 观察结果：本轮仍无 constrained-only 增量；不提前宣称工具有效或应退出。

## 跨仓库联动判定

- scriptLinkageState：not-required（无需发布脚本（不适用））。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：2b90b2d2ca56bc954c9328a51bb5571e896f713d / 806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99（RC8 以来内置脚本文件无差异）。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：差异不涉及受管 kejilion.sh 协议、运行时动作或镜像内脚本内容。kpanel.conf 是 kejilion/apps 应用市场配置，不是 kejilion.sh；L3 的 app_conf_lifecycle 与新增 app_conf_update_backup_parity 均 pass。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Go desktopwallpapers/panel/cluster 测试、193 个前端文件 / 1,710 项测试、场景包复现校验 | 真实面板上传/登录页壁纸/三场景 Worker 播放未浏览器实测 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 上传接口会话+Origin+CSRF+审计、完整解码与元数据剥离测试；Trivy 源码/镜像、npm audit | 新边界包未经 CF scoped 审计（稳定版前补） |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | app_conf_update_backup_parity（中断恢复、快照校验）、light_store_recovery_test、Worker 三路退回页面 | 无真机更新中断注入；Worker 未在 Safari/低端设备实测 |
| 性能与资源预算 | 已实现未实机验证 | Worker 渲染移出主线程（来源提交实测首次加载卡顿 0.7–0.8 s）；镜像 256 MiB/1 CPU/128 PID 契约 | 正式包启动耗时、GPU 帧率与显存未测 |
| 用户体验与可访问性 | 已实现未实机验证 | 壁纸选择器/焦点/上传对话框单测，来源分支验收修复 | RC9 精确提交未做浏览器验收；缩放与动态偏好未验 |
| 数据、配置与迁移 | 已实现未实机验证 | 无 schema 迁移；新目录 desktop-wallpapers 不进 .kpb 备份；回退快照保留两份 | 升级/降级往返未在隔离机演练 |

## 自动门禁

- 定向测试及结果：本机 vue-tsc 通过；vitest 193 文件 / 1,710 项通过；桌面组件 211 项在场景版本升级后复跑通过；check-version-consistency、business-context-freshness（baseline b7a7735，43 提交）、governance-consistency、collaboration-state writer 均通过。
- make verify-release 环境和结果：由 L3 入口在固定 Runner 内执行，全部通过；核心 panel race 106.787 s；本地验证镜像 sha256:243bcce730ce6c21088855e05269c1934ba454595e2f3749abefe8f8252f3ab3（仅本地证据）。l3-verify-release.log SHA-256 2b747c82790c095c4a675760933dcdb58a2d21b0722befc40ddce39068a14d89。
- L3 外层入口：scripts/run-release-l3.mjs，run ID v1.22.0-rc.9-f727155f-l3-r1，target local-wsl-dr（arena-154 SSH 连接超时，未上传候选），base tag v1.21.0，基线 main 3d7c28d1；2026-09-25T23:26:56Z 至 23:34:33Z，status=passed、exit_code=0，evidence.sha256 12 项。Runner kpanel-release-gate:go1.26.7-node24，ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d。bundle b82ae2245de37de03c6b68b452a16de842449ea692cd6f87ee0e4fe6b8ff8950；plan bc483aa88b95f290a39554c4ee951955e7cd9932df41d222c6f83e53df496d8d；manifest ef84d2a486cf6c7907dbfe5c91071f012a7a34375fda9346a77123a060045a89；执行脚本 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979。
- 候选 CI：[36204636769](https://github.com/kejilion/KPanel/actions/runs/36204636769) success；Dependency freshness 36204636758 success。
- 主线 CI：[36205228645](https://github.com/kejilion/KPanel/actions/runs/36205228645) success；Dependency freshness 36205228640 success。精确 SHA 均为 f727155f。
- Release workflow：[36205892749](https://github.com/kejilion/KPanel/actions/runs/36205892749) success；标签依赖检查 36205892699 success。
- 安全扫描、镜像契约、SBOM/provenance：L3 与 Release 的 Trivy、运行时契约通过；index 含 2 个 unknown/unknown 构建证明条目。

## 依赖与技术栈变化

- 本版未升级 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本；锁文件只改产品根版本。
- 依赖检测复用 2026-09-25 完整报告 v1.22.0-rc.5-20260925/dependency-report-r2.json（10/10 来源、0 失败、0 emergency-security）；候选/主线 Dependency freshness 均 success。
- 暂缓或拒绝候选：沿用 RC8 记录，无新增。
- 升级后的兼容结论：不适用（无依赖升级）。

## 隔离真机与浏览器验收

- 主机/环境：`local-wsl-dr`（Ubuntu / root Docker）登记用途为 candidate-validation，仅允许候选 L3 灾备验证。公开镜像 E2E 也曾在这里运行，但不计入规范要求的隔离环境验收。`arena-154` 于 2026-09-26 再次 SSH 连接超时，未在其上运行测试。
- 使用的产物：L3 为精确候选 f727155f；E2E 为公开 docker.io/kjlion/kejilion-panel:1.22.0-rc.9。
- 后台浏览器作业：未执行。RC9 精确提交没有 acceptance 预览；壁纸与登录页的浏览器证据来自来源分支，不冒充 RC9 精确提交。
- 测试窗口/soak：不适用（没有新增长连接；Worker 生命周期风险由单测与退回路径覆盖，需要在稳定版前补浏览器实测）。
- 宿主机写入/失败注入：只在 L3 容器内的 app_conf 生命周期测试中执行（中断更新→恢复原版本与数据、快照 SHA-256 校验）；未在真机执行。
- 未执行场景及原因：真实浏览器三场景 Worker 播放、Safari 退回路径、125%/200% 缩放、真机更新中断——本轮只发预览产物，列入稳定版准入。

## 发布产物与公开仓库复核

- GitHub Release：2026-09-26T00:55:47Z 公开，draft=false、prerelease=true；GitHub Latest 仍为 v1.21.0。
- Docker 版本与通道：1.22.0-rc.9 与 preview 均为 index sha256:02d750537540888b05299e7bcdc93d22d0cabb9dde9fcae317eb5eefff063bc2；latest 仍为 sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。
- linux/amd64 sha256:f9fa86beae3e847038efd7b25d5d217ac7eb0c7d8fa2d1878a53fe6f355d7690；linux/arm64 sha256:e6c00bcda5faeb4f1c7609508bf867797841ee51f9f5c979d8962b3015102219。
- 附件 14 项（Agent/Node/MCP 各平台二进制、kejilion-panel-deploy-1.22.0-rc.9.tar.gz、LICENSE、THIRD_PARTY_NOTICES.md、SHA256SUMS）；SHA256SUMS 11 项与 GitHub 资产 digest 全部一致。证据 public-verification/github-release-api.json、SHA256SUMS、channel-digests.txt、index.json。
- 公开镜像本地检查：普通 `docker pull` 成功（`pull_exit=0`）；镜像 OCI version=1.22.0-rc.9、revision=f727155f9e9e33a140886a6fdec32800f0f7ff5c、User=65532:65532；`image-e2e.sh` 输出 `image_e2e=pass`、`e2e_exit=0`。执行位置为 `local-wsl-dr`，超出该环境限定的候选 L3 用途，因此只作本地诊断证据，不写作正式隔离验收通过。证据见 `public-pull.log`、`public-image-identity.txt`、`public-image-e2e.log`。
- kejilion/apps / kejilion.sh 契约结论：kejilion/apps main 39b498a0dc6b3013fda31b103c138ad0df3cc42c，工作树 clean。本版 packaging/kejilion-app/kpanel.conf 有安装/更新契约变化（更新事务与回退快照）。用户决定本轮不同步，延至 1.22.0 稳定版；apps 保持 RC8 契约，默认入口仍为 latest，无 apps 提交。

## 自更新通道验收

- 通道选择、加入/退出预览、自动安装开关与降级保护：代码未变，沿用 RC8 结论。
- systemd 后台执行、更新前备份、失败恢复：仓库 kpanel.conf 已统一为事务化快照与恢复，L3 app_conf_update_backup_parity=pass；未进入应用市场入口，用户实际更新路径本轮不变。
- OpenRC 与轻量 Node 边界：OpenRC 手动更新共用事务，自动调度范围不变；轻量节点状态恢复见 611cc293。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：local-wsl-dr（仅候选验证与公开镜像 E2E）。
- 正式部署环境：不适用。
- prod-108：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。arena-154 SSH 不可达，本轮未连接。
- 部署前后状态、生产写操作：不适用。
- 在登记用途内执行的场景：候选 L3。公开镜像 E2E 是上述本地诊断检查，登记隔离环境验收未执行。

## 回滚

- 源码/tag：v1.22.0-rc.8（25a4655a）；稳定 v1.21.0（396fcd62）。
- 镜像 digest：RC8 sha256:626122ac…de85；稳定 sha256:e2c5d392…3f61。
- 数据/配置备份：不适用（无生产写入）。
- 回滚步骤：预览通道用户可显式更新到 RC8 镜像；新增 desktop-wallpapers 目录对旧版无影响。
- GitHub Latest、Docker latest 与标准更新入口：均仍指向 v1.21.0，应用市场默认 latest 未变。
- 公共默认更新通道决策：不适用（预览版）。

## 分支与资源处置

- 远端 release/v1.22.0-candidate 保留在 f727155f；本验收记录使用独立分支 docs/release-v1.22.0-rc.9-acceptance，基于 f727155f，完成候选 CI、主线快进和主线 CI 后归档，不追加到已发布标签。
- 来源分支（update-backup-parity、light-node-crash-recovery、custom-wallpapers、login-wallpaper、desktop-3d-scene-packs）对应内容已随 RC9 发布；其中 custom-wallpapers 工作树有 1 个未跟踪文件，所有权未确认。分支归档须逐项核对精确提交、所有权与远端状态，不在此次验收中声称已归档。
- L3 bundle、WSL 验收源码、公开镜像及固定 Runner 保留用于验收与恢复；未执行全局 prune。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-25T21:09:45+08:00
- 候选冻结时间：2026-09-26T07:25:13+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本版是预览，不计稳定发布或生产部署频率。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "public-image/wsl-invocation/msys-path-conversion",
    "position": "before-production-write",
    "count": 1,
    "impact": "从 Git Bash 调用 wsl bash /mnt/c/... 时路径被 MSYS 改写为 C:/Program Files/Git/mnt/c/...，E2E 脚本未启动，未拉取镜像，无结果。",
    "recoveryEvidence": "设置 MSYS_NO_PATHCONV=1 后同一脚本重跑；public-pull.log、public-image-identity.txt、public-image-e2e.log 保留。",
    "permanentAction": "Git Bash 下调用 wsl 传 Linux 路径时固定加 MSYS_NO_PATHCONV=1；与 RC8 跨 Shell 参数承载问题同根，公开镜像 E2E 入口脚本化仍为遗留诉求。负责人本发布任务，复核日期 2026-10-03，退出条件为仓库提供固定公开 E2E 入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-image/environment-policy/disallowed-wsl-e2e",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开镜像 E2E 在仅允许候选 L3 灾备验证的 local-wsl-dr 执行，虽输出 image_e2e=pass，但不能作为登记隔离环境验收证据。",
    "recoveryEvidence": "保留原始 pull、镜像身份和 E2E 日志并降级为本地诊断证据；arena-154 于 2026-09-26 SSH 连接超时，合规隔离验收待补。",
    "permanentAction": "公开镜像 E2E 前执行 environment-policy 用途预检，仅在已登记且允许验收的环境运行；负责人为后续 1.22.0 稳定版发布任务，复核日期 2026-10-03，退出条件为登记隔离环境中同一公开镜像的 E2E 通过。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：本轮未执行；组装工作树、L3 证据与 WSL 验收目录保留供验收。
- 未验证风险：RC9 精确提交的浏览器验收（上传壁纸、登录页本地副本、三场景 Worker 播放与退回路径）；真机更新中断恢复；Safari/低端 GPU。
- 已实现待实机准入：kpanel.conf 更新事务需在稳定版前完成隔离真机演练后再同步 kejilion/apps。
- 当前状态：预览产物已公开，自动门禁和公开镜像本地 E2E 通过，应用市场入口与 latest 未变；合规隔离环境 E2E 未完成，不把本地结果提升为正式验收。稳定版前须在允许该用途的登记环境补测。
- 后续：1.22.0 稳定版前补 internal/desktopwallpapers scoped 安全审计、浏览器验收与 kpanel.conf 隔离演练。
