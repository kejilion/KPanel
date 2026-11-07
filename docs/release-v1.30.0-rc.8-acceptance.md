# KPanel v1.30.0-rc.8 发布验收记录

日期：2026-11-08

发布级别：L3

候选提交 / 标签：`11c3c8a401809424e26377bc638d7290708fd9a0` / `v1.30.0-rc.8`

上一稳定版本 / 回滚点：`v1.29.0` / Docker digest `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：`release/v1.30.0-candidate` `11c3c8a4`，RC8 来源；按预览列车保留。
- 归档 ref 与 SHA / 远端复核结果：未归档；远端候选分支、`main` 与 RC8 标签均指向 `11c3c8a401809424e26377bc638d7290708fd9a0`。
- 本次来源任务分支：站点访问密码、历史监控单页、音乐在线匹配/旁车、电影资料与媒体整合、移动终端及 AI 文件投影相关可发布改动已在 RC8 源码中；源码范围及替代理由见下方。
- 本地分支 / upstream / worktree：候选分支与工作树保留于 `C:\GitHub\_release-candidates\kpanel-v130`；本次未删除工作树或分支。
- 未完成归档项 / 责任人 / 下次复核触发条件：预览候选按 `docs/project-management.md` 10.2 保留；需终止预览列车时再复核。

预览版未部署生产。

## 发布画像

- 业务域：Panel、Agent、音乐与影视媒体、监控、站点管理、终端及 AI 文件读取。
- 变更面：展示、只读、偏好数据及预览版本元数据；站点访问密码功能依赖配套脚本，脚本版本随 RC8 身份固定。
- 受影响用户旅程：音乐匹配与可选旁车保存、影片资料查看与播放进度、监控检查、站点证书/访问密码、移动终端输入、AI 文件安全读取。
- 未变化契约：稳定更新通道、Docker Compose、Agent 权限边界、既有稳定 API 及生产部署状态。
- 风险等级及理由：中；涉及媒体偏好、AI 文件读取与宿主机文件旁车写入，发布流程使用隔离 L3 和公开真实浏览器验收。

## 发布范围与未纳入内容

- 用户可见更新：音乐队列、当前曲目与进度同步；当前曲目在线匹配；按需歌词/封面旁车保存；影片资料和播放进度；Ping/TCP/HTTP 双栈与域名检测及历史单页分组；网站证书和访问密码统一入口；移动终端辅助输入；AI 文件读取在无法证明稳定时拒绝返回。
- 精确提交清单：`v1.30.0-rc.7..11c3c8a401809424e26377bc638d7290708fd9a0`；最终预览源码 SHA 为 `11c3c8a401809424e26377bc638d7290708fd9a0`。
- 明确未纳入的分支、文件或后续事项：`feature/site-settings-main-20261106` 的旧提交会移除主线已有的未完成证书提示、当前站点标识及折叠时卸载敏感表单的行为，保留待刷新；`feature/music-sidecar-checkpoints-20261107` 会回退队列能力；`feature/music-sidecar-save` 的精确 L2 结果在 `TestStableDockerBindPreventsUnprivilegedRedirection` 失败，均未纳入。音乐整库自动刮削/扫描未实现。RC8 首次发布浏览器阶段失败，恢复验证通过但根因未确认。

## 外部审计与修复交付

- 安全盲审补漏：未找到独立的 RC8 盲审档；发布 L3 安全扫描通过，不能据此宣称完成独立盲审。
- `node scripts/security-review-scope.mjs --base v1.29.0`：`security_review=recorded groups=17 invalid_trailer=true`；17 组均有记录，但存在旧 `Security-Review` trailer 格式不能机器验证，未宣称完整。
- finding fingerprint / 修复 commit / 独立复核与回归证据：RC8 增量发布无新增已确认 finding；各来源任务复核随候选保留，未用此验收记录替代原始评审。
- 修复交付状态：RC8 已公开为 preview；stable 与生产未变更。
- OCR 观察：来源提交中的 OCR 范围见各提交 trailer；本验收未重新汇总整个 RC8 的 OCR 样本口径。
- 本周期观察：不据此声明 OCR 工具已通过或应退出。
- 候选分支原始 tip、归档位置和远端核验：预览候选按项目规范保留，源任务分支未删除或批量归档。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`
- 变更集编号：不适用（沿用已发布并审定的站点访问密码配套脚本）。
- KPanel 实际内置脚本基线 commit / SHA-256：`1faee13f3faef164efbfa899e244fc19ad58aaff` / `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选 commit / SHA-256：同上；RC8 双架构镜像 label 与公开验收快照一致。
- 状态判定依据与兼容性证据：RC8 的 amd64/arm64 镜像均绑定该脚本 revision 与摘要；访问密码功能要求配套脚本版本。
- 本版发布决定：RC8 已发布配套脚本；本验收不发新脚本。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 候选 CI、L3、发布后浏览器及隔离容器验收通过 | 真实网易云/QQ 上游匹配质量未验证 |
| 网络入侵与供应链安全 | 已验证 | L3 安全扫描、双架构镜像身份和配套脚本摘要核验通过 | 独立 RC8 盲审未执行；review scope 存在旧 trailer 格式 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 初次公开验收失败保留；同一冻结快照只读恢复的初始、重启、published 和 upgrade 四份报告均通过 | 初次重启后资源失败根因未确认；未做生产实机或长期 soak |
| 性能与资源预算 | 已验证 | L3 与受限容器验收通过 | 未单独执行长时间性能压测 |
| 用户体验与可访问性 | 已实现未实机验证 | 公开浏览器验收通过；公开升级弹窗视口检查通过 | 无实体手机；完整多应用缩放/键盘矩阵未执行 |
| 数据、配置与迁移 | 已验证 | RC8 偏好同步、旁车写入及双架构更新身份检查通过 | 音乐旁车写入后会新增文件；无生产数据迁移验证 |

## 自动门禁

- 定向测试及结果：候选 CI `42379563592` 成功；覆盖本提交源码的定向测试由 L3 运行。
- `make verify-release` 环境和结果：GitHub-hosted Linux；L3 外层入口通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：Preview workflow `42380336219` 的 L3 job `128426469296` 成功；原始证据由 Actions artifact 保存。验收恢复 run `42389672305` 使用冻结 RC8 快照。
- 候选 CI：`42379563592` 成功；preview workflow 中候选 `windows-node` `128426469302` 与 `verify` `128426469353` 成功。
- 主线 CI：preview workflow 的 `main-ci/windows-node` `128431340248` 与 `main-ci/verify` `128431340360` 成功。
- Release workflow：release job `128436182193` 成功并公开 RC8；公开验收 job `128441905013` 失败，其他发布前门禁通过。
- 发布说明门禁：公开快照记录 8 条摘要、3 条升级提示，说明解析与公开身份检查通过。
- 安全扫描、镜像契约、SBOM/provenance：L3/Release 成功；公开 snapshot 记录 19 个附件、15 项 SHA256SUMS 校验。

## 依赖与技术栈变化

- RC7 到 RC8 未修改 `go.mod`、`go.sum`、`Dockerfile` 或 Go/Node 依赖版本；Web 包版本随 RC 更新。
- 直接/传递依赖报告：本验收未重新生成完整来源报告；以 GitHub-hosted L3 和候选 Dependency freshness 的真实回执为准。
- 暂缓或拒绝候选：未随 RC8 引入额外依赖更新。
- 升级后的兼容、安全、构建、性能资源和回滚结论：L3、镜像构建与发布身份核验通过；稳定回滚点未改。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Linux Runner；公开 RC8 镜像含 `linux/amd64` 和 `linux/arm64`。
- 环境策略 ID 与允许用途：仓库 `environment-policy.json` 中 GitHub-hosted candidate/browser validation；只在隔离容器与公开浏览器执行。
- 使用的精确候选或公开产物：`11c3c8a401809424e26377bc638d7290708fd9a0`，OCI index `sha256:1d9a8eeb5ec36d25e80f3d43fb2e61c34636f2f315462284d7b17838a884c7c2`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：初次验收 run `42380336219` 的公开验收 job `128441905013` 失败；原冻结快照恢复 run `42389672305` / job `128454410738` 成功。恢复报告：published 45、upgrade 31、initial 48、restart 26 项，均 `passed`，无报告错误。证据位于 `C:\GitHub\_release-evidence\kpanel-v130-rc8-public-recovery-20261107\public-42389672305-1`。
- 初次失败事实：重启阶段页面空白，32 个 `/assets/*.js` 请求失败；Panel 日志出现 TLS `unknown certificate`。根因未确认；没有重建镜像或重新发布，恢复使用同一公开快照。
- 测试窗口/循环数及风险依据：一轮公开快照恢复；未执行 soak。
- 受影响用户旅程、视口、主题、键盘/焦点、语言和失败态：published/upgrade 公开浏览器流程通过；升级对话框 320/390/1280px 视口断言通过；没有真实手机实机。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器重启报告通过；未连接生产主机。
- 未执行场景及原因：生产系统升级、实体手机、真实外部音乐上游、长期 soak 未执行；本版仅预览。

## 发布产物与公开仓库复核

- 公开 Release、面板发布信息接口与更新弹窗：恢复快照确认版本 `1.30.0-rc.8`、candidate SHA 和 digest 匹配；8 条摘要、3 条升级提示。
- GitHub Release 状态：`isDraft=false`、`isPrerelease=true`；公开于 `2026-11-07T15:15:21Z`；GitHub Latest 保持 `v1.29.0`。Release：<https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.8>。
- Docker 版本与通道 OCI index：RC8 和 `preview` 为 `sha256:1d9a8eeb5ec36d25e80f3d43fb2e61c34636f2f315462284d7b17838a884c7c2`；stable `latest` 为 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:03fd4db537434f56499bd773c3aabdfb4972d8c438fa3deca9f71c5129d5296b`；arm64 `sha256:7346dde94a98c4c29eb5f7c61bba294386f7976d61f8c7dd3d2d65c76f70680d`。
- 附件及 `SHA256SUMS`：19 个附件，15 项校验；SHA256SUMS digest `sha256:f20bf09c2105d5a0042b3ecfac48dbeb775a09467ef5868a1bb556b0a830319d`。
- 公开镜像 `image_e2e=pass`：恢复 run 的四份报告均通过，合计 150 项检查。
- `kejilion/apps` / `kejilion.sh` 契约结论：脚本 revision/hash 与 RC8 镜像标签一致；无应用市场或脚本范围变更。

## 自更新通道验收

- stable 来源保持 `v1.29.0`，preview 指向 RC8 唯一公开 digest；恢复验收通过。
- 加入预览不触发自动安装；退出预览不触发降级；稳定通道未改变。
- 自动安装开关与一次性安装行为：公开 upgrade 流程 31 项通过。
- 旧状态迁移与重启后通道选择：公开 initial/restart 报告均通过。
- systemd、OpenRC 与轻量 Node 边界：未执行宿主机更新；边界沿用 `docs/release-channels.md`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：GitHub-hosted 隔离容器；未使用 `prod-108`。
- 正式部署环境：不适用；未连接 `arena-154`。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前状态、命令、部署后健康与回滚核验：不适用（未进行生产操作）。
- 生产已执行写操作：无。
- 仅在隔离环境执行的场景：L3、公开浏览器、公开 RC8 镜像、容器初始/重启验收。

## 回滚

- 源码/tag：稳定回滚点 `v1.29.0`；RC8 annotated tag 保留。
- 镜像 digest：稳定回滚 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`；RC8 预览 `sha256:1d9a8eeb5ec36d25e80f3d43fb2e61c34636f2f315462284d7b17838a884c7c2`。
- 数据/配置备份：无生产备份或修改；预览更新由使用者自行备份。
- 回滚步骤：退出预览切回 stable；不会自动降级。
- 回滚后生产实际版本与健康状态：不适用（无生产部署）。
- GitHub Latest、Docker `latest` 与标准更新入口：均继续指向 `v1.29.0` stable。
- 公共默认更新通道决策：保留 opt-in preview。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-07T09:24:15+08:00
- 候选冻结时间：2026-11-07T22:00:15+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

首次公开验收失败是有效的门禁结果，恢复复验针对同一冻结快照并已通过；目前没有证据把它归类为执行器异常或无效证据，因此不计入流程异常。失败事实及未确认根因已保留在上文。

## 遗留风险与后续准入

- 未验证风险：初次公开重启时 32 个 JS 资源请求失败及 TLS unknown certificate 的根因未确认；真实外部音乐服务、实体手机、生产安装及长期 soak 未验证。
- AI 文件读取边界：仅在 Linux 支持稳定读取 lease 且文件可证明稳定时返回；活动写入日志或不支持的文件系统可能返回拒绝，需暂停写入方或使用稳定副本。AI 其他功能未因此整体关闭。
- 音乐功能边界：在线匹配仅针对当前曲目；旁车写入为可选；整库自动刮削/扫描未实现。
- 不阻断 RC8 已公开状态的理由：L3、candidate CI、main CI、Release 以及原快照公开恢复验收均有对应证据；首次公开失败仍作为显式未解释风险保留。
- 后续准入：若公开重启资源失败再次发生，保留新截图、assetFailures 和 TLS 日志后再决定代码修复；不得覆盖 RC8 镜像或首次失败证据。
