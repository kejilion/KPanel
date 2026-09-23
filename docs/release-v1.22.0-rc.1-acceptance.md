# KPanel v1.22.0-rc.1 发布验收记录

日期：2026-09-23

发布级别：L3；当前为发布准备记录，尚未取得发布准入。

候选产品提交：`6f0c9ec015807db57e8a0475754f7ad54a8049d6`。本记录和评审 trailer 提交后冻结，最终候选 SHA 以仓库外 L3 manifest 为准；标签 `v1.22.0-rc.1` 尚未创建。

上一稳定版本 / 回滚点：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3`，镜像 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。

`releaseChannel`：`preview`；`releaseTrain`：`1.22.0`。

候选分支与发布后处置：`release/v1.22.0-candidate`，预览版或失败时保留。

- 发布基线：`origin/main=71d50138f9da73999fcaa6046f9b4a860151d2f0`，推送前须再次核对。
- 原始候选 `33aa84c85d4989cecfa6e4a6c1a5785f3b00f4ee` 保留于本地 `archive/local-only/v1.22.0-candidate-33aa84c8`；未推送。其历史含未修复问题的详细审计材料，按 PROJECT_RULES.md 5.4 重组公开历史。
- 原始 Transport 分支 `feat/terminal-file-transport-v3@aae2a5674df5d92e4a7eeb1bf1718746892f2710` 保留本地；公开来源为 `fix/transport-v3-public-20260923@9e6a3b0eaf5b19b1b3cebb192b7f14f8dd4424f4`。
- 复核修复分支 `fix/v122-transport-review-20260923@6f0c9ec015807db57e8a0475754f7ad54a8049d6` 已快进纳入候选。来源分支、原始审计和当前 worktree 保留用于追溯，未归档远端或回收活跃工作树。
- 唯一候选 worktree：`C:/GitHub/_codex-tasks/kpanel-v122-rc1-public`。本地证据根：`C:/GitHub/_release-evidence/v1.22.0-rc.1-public-20260923`。

## 发布画像

- 业务域：AI 提供商模型发现；本机/多主机终端、应用交互终端、批量终端；文件列表、缩略图、上传下载。
- 变更面：只读、宿主机文件/PTY 写入、流式协议和浏览器交互；没有安装部署契约或数据库迁移。
- 核心旅程：模型列表与流式聊天；Panel 间、轻量 Node、本机终端的输入输出、断线重连、取消、关闭、注销/过期；批量成功/失败结束；文件并发上传、缩略图、重复列表、下载与取消。
- 未变化契约：既有 API 兼容入口、端口、Compose、Agent 特权模型、`kejilion.sh` 和应用市场安装/更新契约。
- 风险：高。流式连接跨鉴权、会话、远程节点和宿主机写入边界。自动门禁之外需要两个 Panel 加轻量 Node 的真实互通、反代重连/撤销、文件持久结果和浏览器生命周期证据。
- 体验验证采用 interaction：受影响终端/文件旅程，桌面和窄视口、浅/深色、中文/英文、键盘焦点、100%/125%/200% 缩放和错误恢复；没有新增视觉组合布局。
- 性能/恢复验证：隔离真机比较 50/150 ms RTT 的文件/终端行为，执行 20 轮断开/重连/取消并记录连接、FD、内存趋势；不执行无关耗流量/付费业务。
- 生产动作：不适用（预览版禁止生产部署）。只允许提升 `preview`，GitHub Latest 和 Docker `latest` 必须保持稳定版。

## 发布范围与未纳入内容

- AI JSON Accept 修复：`e96ca5c4a0c2b1240bcd281aa513132719ec30d5`，对应原始 `c9ec4774` 产品补丁。
- Transport v3：以 `862ca2a9441e0c2a44fe9284e0d14c2165461b87` 为可回滚合并单元；公开来源保留原始实现至 `ce2fffa6`，补入会话撤销修复 `800a96e33d7127ceb056a737d9fdca4cfdd2adbd`。
- 版本与披露整理：`f412bb7ec2dc692608daa8738b9b9ec6470be66a`；至此相对原始候选的产品代码无差异，仅审计/说明材料不同。
- 发布复核修复：`6f0c9ec015807db57e8a0475754f7ad54a8049d6`，阻止已发送 PTY open 的自动回放，回收取消请求等待项，为批量输出添加容量和生命周期边界。
- 完整清单由 `git log 71d50138..6f0c9ec0` 复现。动态壁纸、无关 WIP、尚未动态确认的其他审计线索不纳入。

## 外部审计与修复交付

- run-7 scoped 审计基于 `ce2fffa6f66c81beeb97b903e2701a76462016a5`；公开记录只保留元数据、计数和已有修复的说明。其 source-only 结论不能代替动态/真机验证。
- 覆盖检查：`check-security-audit-coverage.mjs --target 6f0c9ec0` decision=`ok`；未审计提交 3 个、文件 4 个、年龄 0 天，无新边界包；last_full=run-4（2 天）。RC 只记录。
- 会话撤销问题对应原始修复 `60154808`，公开重放为 `800a96e3`；修复交付状态为源码已纳入，RC/稳定版/部署均未交付。
- 发布任务由 OpenAI/Codex 对主要实现 Anthropic/Claude 独立复核；本任务形成的纠正补丁经故障复现和定向回归验证，不冒称由其他提供商独立编写或复核。
- OCR：精确范围 `71d50138..6f0c9ec0`；先保存自由臂再执行 1.12.6 preview/rule；51/51 文件 reviewed，11 个文档/锁文件/审计文件按策略排除，测试未被 default_path 排除。自由臂 3 个 MEDIUM 已修复，约束臂新增 0。证据为根目录内 `free-form.json`、`preview.json`、`rules.json`、`coverage.json`。
- 本轮 1 个有效使用，skipped=0、unreported=0；本稳定周期尚未结束，不据此宣称工具长期有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））；变更集编号、脚本候选：不适用。
- 实际内置脚本：`kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 依据：相对发布基线未改变 Dockerfile、packaging、脚本协议、运行时动作枚举、宿主机安装产物、更新路径或外联配置。使用现有 PTY 和文件适配；既有脚本契约由 L3 再验证。
- 本版不发布脚本，不写 `kejilion/apps`；最终须核对应用市场配置仍指向稳定来源。

## 多维质量结论

| 维度 | 状态 | 当前证据及缺口 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 源码/定向测试已验证；双 Panel、轻量 Node 真机闭环未执行 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 会话撤销/资源边界复核已完成；本候选 L3 扫描和真实反代鉴权未完成 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 取消、含糊 open、流式生命周期回归已通过；重启/真机故障恢复未执行 |
| 性能与资源预算 | 已实现未实机验证 | 有界队列/缓存及原始人工 RTT 测试；最终候选真机资源趋势未执行 |
| 用户体验与可访问性 | 已实现未实机验证 | 31 项相关前端测试通过；浏览器体验矩阵未执行 |
| 数据、配置与迁移 | 不适用 | 没有数据格式或配置 schema 迁移；回退保持既有文件和终端入口 |

## 自动门禁

- 定向验证：固定 Runner 中 `go test -race ./internal/cluster -run TestTerminalStream -count=3 -timeout=90s` 通过；前端 terminalOutputReader、terminalStream、batchCompletion、BatchTerminalPanel 共 4 文件/31 测试通过。
- 复现证据：`go-review-before.log` 两项失败与 `go-review-after.log` 修复后成功；前端修复前工具输出 3 失败/2 通过，修复后 `web-review-after.log`。
- `make verify-release`：未验证；仅通过 `node scripts/run-release-l3.mjs` 执行。
- 冻结 Runner：`kpanel-release-gate:go1.26.7-node24` / `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- L3 选择 `local-wsl-dr`（Ubuntu/root/Docker），上一稳定基线 `v1.21.0`，一次一目录；候选、run ID、plan/script/bundle 摘要及终态在 manifest/原始日志中记录后更新。
- 候选 CI、主线 CI、Release workflow、安全扫描、最终镜像运行时限制及 SBOM/provenance：未执行，尚无可引用的成功 run。

## 依赖与技术栈变化

- 本版未改变依赖版本、锁定版本、基座、Action、扫描器或内置脚本；package 与 lock 仅同步发行版本。
- 新候选完整依赖报告、安全通告审计与扫描：未验证；不把上一版本报告当成本候选结果。
- 无本版新增依赖候选或暂缓决策；现有兼容/供应链检查由 L3 及后续 CI 验证。

## 隔离真机与浏览器验收

- 目标策略 ID：`arena-154`；两次 SSH 连接超时，未执行测试。原始事实保留在本任务工具输出，复查证据随后保存至证据目录。
- 发行版/运行时、后台 job ID/命令规格摘要、候选部署、真机持久结果、浏览器矩阵、失败注入/重启恢复：均未验证。
- `local-wsl-dr` 只允许 candidate-validation，不能替代真机、浏览器、性能、故障注入或生产环境。
- 受影响旅程及循环数按发布画像执行；缺少这些结果时不宣称取得发布准入。

## 发布产物与自更新通道

- GitHub Release、Docker 版本/preview OCI index、amd64/arm64 digest、附件 SHA256SUMS、公开镜像 `image_e2e`：均未发布/未验证。
- 发布前 API 核对：GitHub Latest 为 `v1.21.0`，Docker latest 为本记录的上一稳定 digest；目标 RC tag/镜像尚未占用。证据 `releases-before.json`、`docker-latest-before.json`。
- 自更新实现本版未变。加入预览不自动安装、退出不降级、选择来源持久化和后台更新恢复由既有契约及本候选 L3/公开产物复核，当前不冒用历史结果。
- OpenRC 和轻量 Node 边界遵守 `docs/release-channels.md`。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）；生产写操作 0。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 生产版本、数据备份、正式升级和回滚健康证据不适用；隔离验收不作为生产证据。

## 回滚

- 尚无公开变更，无需执行回滚。保留 `v1.21.0` 源码与精确稳定镜像 digest；不得用源码 tag 推断镜像存在。
- 若撤销本次源码，先反向撤销纠正补丁 `6f0c9ec0`，再按第一父线反向撤销 Transport 合并 `862ca2a9`；AI 修复可独立保留或反向撤销。实际操作须重新验证版本字段和工作树。
- 预览发布后若需要通道回退，使用事先确认的 preview digest；当前未记录旧 preview digest，因此不执行或承诺其回退。生产和默认稳定通道保持不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T21:37:39+08:00
- 候选冻结时间：未记录
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/arena-ssh/connection-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一预检批次两次无法连接 arena-154，真机和浏览器验证不能执行；候选 L3 明确选择已登记的灾备环境。",
    "recoveryEvidence": "未恢复；SSH 连接超时工具输出；local-wsl-dr 用途检查只能证明候选 L3 允许执行。",
    "permanentAction": "发布负责人在 2026-09-23 恢复检查 arena-154 或按规范登记替代验收环境；退出条件是完成受影响旅程的真实验证，不以本地单测代替。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 真机、浏览器、性能资源趋势、最终 L3 与公开产物门禁尚未完成，当前阻断发布。
- 新轻量 Node 连接旧中心时可选 stream 被拒绝后按 1 分钟至 30 分钟退避，既有轮询继续；不是完全无请求回退。
- 未修复审计线索保留本地，动态确认和修复单独排期；不公开可重用攻击细节。
- 本地资源回收：未执行；保留当前候选、原始审计、回滚 ref 和验证证据。未处理其他任务的脏工作树。
