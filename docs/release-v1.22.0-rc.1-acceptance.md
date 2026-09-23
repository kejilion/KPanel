# KPanel v1.22.0-rc.1 发布验收记录

日期：2026-09-23

发布级别：L3；本机固定 Runner 的候选 L3 已通过。按既往预览版流程继续发布；真机互通、浏览器和资源循环未执行，作为明确未验证的预览版残余风险记录，不表述为通过。

候选提交 / 预览标签：`370877ed1645769ae179bfa5486d56dc12f834a8` / `v1.22.0-rc.1`。注释标签解引用到同一提交；最终提交 SHA 与 L3 manifest 一致。

上一稳定版本 / 回滚点：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3`，镜像 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。

`releaseChannel`：`preview`；`releaseTrain`：`1.22.0`。

候选分支与发布后处置：`release/v1.22.0-candidate`，预览版或失败时保留。

- 发布基线：`origin/main=71d50138f9da73999fcaa6046f9b4a860151d2f0`；推送前核对通过。打 tag 时远端 `main` 与候选分支均精确指向 `370877ed1645769ae179bfa5486d56dc12f834a8`；本次验收追记只会再推进主线文档，不移动产品 Tag 或候选分支。
- 原始候选 `33aa84c85d4989cecfa6e4a6c1a5785f3b00f4ee` 保留于本地 `archive/local-only/v1.22.0-candidate-33aa84c8`；未推送。其历史含未修复问题的详细审计材料，按 PROJECT_RULES.md 5.4 重组公开历史。
- 原始 Transport 分支 `feat/terminal-file-transport-v3@aae2a5674df5d92e4a7eeb1bf1718746892f2710` 保留本地；公开来源为 `fix/transport-v3-public-20260923@9e6a3b0eaf5b19b1b3cebb192b7f14f8dd4424f4`。
- 复核修复分支 `fix/v122-transport-review-20260923@6f0c9ec015807db57e8a0475754f7ad54a8049d6` 已快进纳入候选。来源分支、原始审计和当前 worktree 保留用于追溯，未归档远端或回收活跃工作树。
- 唯一候选 worktree：`C:/GitHub/_codex-tasks/kpanel-v122-rc1-public`。本地证据根：`C:/GitHub/_release-evidence/v1.22.0-rc.1-public-20260923`。

## 发布画像

- 业务域：AI 提供商模型发现；本机/多主机终端、应用交互终端、批量终端；文件列表、缩略图、上传下载。
- 变更面：只读、宿主机文件/PTY 写入、流式协议和浏览器交互；没有安装部署契约或数据库迁移。
- 核心旅程：模型列表与流式聊天；Panel 间、轻量 Node、本机终端的输入输出、断线重连、取消、关闭、注销/过期；批量成功/失败结束；文件并发上传、缩略图、重复列表、下载与取消。
- 未变化契约：既有 API 兼容入口、端口、Compose、Agent 特权模型、`kejilion.sh` 和应用市场安装/更新契约。
- 风险：高。流式连接跨鉴权、会话、远程节点和宿主机写入边界。本预览版发布前未能取得两个 Panel 加轻量 Node 的真实互通、反代重连/撤销、文件持久结果、浏览器生命周期及资源循环证据；这些均保持未验证，不由自动 L3 或 mock UI 代替。预览版按既往发布做法显式披露此缺口，稳定版前须补齐。
- 体验验证采用 interaction：受影响终端/文件旅程，桌面和窄视口、浅/深色、中文/英文、键盘焦点、100%/125%/200% 缩放和错误恢复；没有新增视觉组合布局。
- 性能/恢复验证：隔离真机比较 50/150 ms RTT 的文件/终端行为，执行 20 轮断开/重连/取消并记录连接、FD、内存趋势；不执行无关耗流量/付费业务。
- 生产动作：不适用（预览版禁止生产部署）。只允许提升 `preview`，GitHub Latest 和 Docker `latest` 必须保持稳定版。

## 发布范围与未纳入内容

- AI JSON Accept 修复：`e96ca5c4a0c2b1240bcd281aa513132719ec30d5`，对应原始 `c9ec4774` 产品补丁。
- Transport v3：以 `862ca2a9441e0c2a44fe9284e0d14c2165461b87` 为可回滚合并单元；公开来源保留原始实现至 `ce2fffa6`，补入会话撤销修复 `800a96e33d7127ceb056a737d9fdca4cfdd2adbd`。
- 版本与披露整理：`f412bb7ec2dc692608daa8738b9b9ec6470be66a`；至此相对原始候选的产品代码无差异，仅审计/说明材料不同。
- 发布复核修复：`6f0c9ec015807db57e8a0475754f7ad54a8049d6`，阻止已发送 PTY open 的自动回放，回收取消请求等待项，为批量输出添加容量和生命周期边界。
- 完整清单由 `git log 71d50138..370877ed` 复现。`370877ed` 相对已复核产品代码只更新验收口径；动态壁纸、无关 WIP、尚未动态确认的其他审计线索不纳入。

## 外部审计与修复交付

- run-7 scoped 审计基于 `ce2fffa6f66c81beeb97b903e2701a76462016a5`；公开记录只保留元数据、计数和已有修复的说明。其 source-only 结论不能代替动态/真机验证。
- 覆盖检查：`check-security-audit-coverage.mjs --target 370877ed` decision=`ok`；未审计提交 3 个、文件 4 个、年龄 0 天，无新边界包；last_full=run-4（2 天）。RC 只记录。
- 会话撤销问题对应原始修复 `60154808`，公开重放为 `800a96e3`；修复交付状态为源码已纳入，RC/稳定版/部署均未交付。
- 发布任务由 OpenAI/Codex 对主要实现 Anthropic/Claude 独立复核；本任务形成的纠正补丁经故障复现和定向回归验证，不冒称由其他提供商独立编写或复核。
- OCR：精确代码范围 `71d50138..6f0c9ec0`；后续候选提交只改验收文档。先保存自由臂再执行 1.12.6 preview/rule；51/51 文件 reviewed，11 个文档/锁文件/审计文件按策略排除，测试未被 default_path 排除。自由臂 3 个 MEDIUM 已修复，约束臂新增 0。证据为根目录内 `free-form.json`、`preview.json`、`rules.json`、`coverage.json`。
- 本轮 1 个有效使用，skipped=0、unreported=0；本稳定周期尚未结束，不据此宣称工具长期有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））；变更集编号、脚本候选：不适用。
- 实际内置脚本：`kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 依据：相对发布基线未改变 Dockerfile、packaging、脚本协议、运行时动作枚举、宿主机安装产物、更新路径或外联配置。使用现有 PTY 和文件适配；既有脚本契约由 L3 再验证。
- 本版不发布脚本，不写 `kejilion/apps`。验收时 `kejilion/apps` 的 `main`（`39b498a0dc6b3013fda31b103c138ad0df3cc42c`）与远端同步且工作树干净；`kpanel.conf` 默认仍为 Docker `latest`，版本约束仅接受稳定版，未将用户带入 RC。

## 多维质量结论

| 维度 | 状态 | 当前证据及缺口 |
| --- | --- | --- |
| 业务正确性与双端互通 | 自动/定向测试已验证，未实机验证 | 固定 Runner Go 全量测试与 Web 1601 项通过；双 Panel、轻量 Node 真机闭环未执行 |
| 网络入侵与供应链安全 | L3 扫描已验证，真实反代未实机验证 | L3 govulncheck、npm audit、Trivy 与运行时契约通过；真实反代鉴权未执行 |
| 稳定性、失败恢复与兼容 | 自动门禁已验证，真机恢复未验证 | 最终 L3 race、安装生命周期通过；同 SHA 首次 L3 有一项测试超时，复跑通过；重启/真机故障恢复未执行 |
| 性能与资源预算 | 部分实现已验证，资源趋势未验证 | 有界队列/缓存与原始人工 RTT 测试；最终候选 20 轮真机资源趋势未执行 |
| 用户体验与可访问性 | 自动/Mock 已验证，真实浏览器矩阵未验证 | Web 1601 项及本地 Mock UI 通过；跨真实浏览器/视口体验矩阵未执行 |
| 数据、配置与迁移 | 不适用 | 没有数据格式或配置 schema 迁移；回退保持既有文件和终端入口 |

## 自动门禁

- 定向验证：固定 Runner 中 `go test -race ./internal/cluster -run TestTerminalStream -count=3 -timeout=90s` 通过；前端 terminalOutputReader、terminalStream、batchCompletion、BatchTerminalPanel 共 4 文件/31 测试通过。
- 复现证据：`go-review-before.log` 两项失败与 `go-review-after.log` 修复后成功；前端修复前工具输出 3 失败/2 通过，修复后 `web-review-after.log`。
- `make verify-release`：经唯一外层入口 `node scripts/run-release-l3.mjs` 在 `local-wsl-dr` 执行并通过；第一次 run 的单项超时保留为流程异常，第二次同 SHA 完整通过。
- 冻结 Runner：`kpanel-release-gate:go1.26.7-node24` / `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- L3 最终 run：`v1.22.0-rc.1-370877e-l3-r2`，候选 `370877ed1645769ae179bfa5486d56dc12f834a8`，`local-wsl-dr`，`status=passed`、`exit_code=0`，2026-09-23 07:31:59Z–07:38:39Z。plan SHA `8ab1db3c783ad15b052a622586fcdb72c1a07af4383fcb80c12efada495a2bd8`，script SHA `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，bundle SHA `4635440ce3a483f31321be3d9ea53bd79cb432c7506eb6b7fd59ef07f8ee7f9e`，manifest SHA `3490b493e023dcfb0c73c9050aee0a11a0ae0c3eaed2613c012de647f421e031`，L3 log SHA `9e5b8413d1773bea8ec787c2d62d4a841d3c32914c62dc32ada0d258d80167db`；证据目录 `C:/GitHub/_release-evidence/v1.22.0-rc.1-370877e-l3-r2`。
- 同 SHA 首次 L3 `...-l3-r1` 在 `TestFileSharePublicStreamExpiresAndReleasesCapacity` 超时失败（exit 2）；候选文件未改，同 SHA `...-l3-r2` 全量通过。根因未确认，不将首次失败抹除。
- 候选 CI [#35832962156](https://github.com/kejilion/KPanel/actions/runs/35832962156) 与依赖新鲜度 [#35832962158](https://github.com/kejilion/KPanel/actions/runs/35832962158) 成功；主线 CI [#35833327759](https://github.com/kejilion/KPanel/actions/runs/35833327759) 与依赖新鲜度 [#35833327809](https://github.com/kejilion/KPanel/actions/runs/35833327809) 成功，均绑定精确 SHA `370877ed`。
- Release workflow [#35834234336](https://github.com/kejilion/KPanel/actions/runs/35834234336) 与标签依赖新鲜度 [#35834234282](https://github.com/kejilion/KPanel/actions/runs/35834234282) 成功；源码/供应链扫描、应用生命周期、原生镜像运行时契约和多架构推送均通过。Release workflow 未执行候选归档（预览版按规范保留）。

## 依赖与技术栈变化

- 本版未改变依赖版本、锁定版本、基座、Action、扫描器或内置脚本；package 与 lock 仅同步发行版本。
- 本候选依赖报告：2026-09-23 06:51:28Z 生成，10/10 检测源成功，29 个直接行动候选、159 个传递信号，SHA-256 `830130ba207185908e73eed4ac27c56143e4c80ab3a004fd100dbb4eb599e88a`。本版未升级依赖；候选另行按策略评审。
- 标签 Release 扫描：govulncheck 未发现可达漏洞（另有 1 个模块级记录无可达调用），npm audit 为 0；Trivy 源码/依赖/secret/config 与原生镜像扫描通过。

## 隔离真机与浏览器验收

- 目标策略 ID：`arena-154`；两次 SSH 连接超时，未执行测试。当前端口复查仍不可达；原始 SSH 事实位于 `C:/GitHub/_release-evidence/v1.22.0-rc.1-public-20260923/arena-ssh-recheck.log`。
- 发行版/运行时、后台 job ID/命令规格摘要、候选部署、真机持久结果、浏览器矩阵、失败注入/重启恢复：均未验证。
- `local-wsl-dr` 只允许 candidate-validation，不能替代真机、浏览器、性能、故障注入或生产环境。
- 本次按既往预览版流程使用 `local-wsl-dr` 仅完成候选 L3；真实双 Panel/轻量 Node、反向代理会话撤销、浏览器矩阵、50/150 ms RTT 与 20 轮资源/恢复检查未执行，作为 RC 已知未验证项披露，不宣称已完成或已通过。稳定版候选须重新完成相应真实验证。

## 发布产物与自更新通道

- GitHub Release：[KPanel v1.22.0-rc.1](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.1)，Release ID `394424619`，2026-09-23 08:06:10Z 发布，非 draft、prerelease=true、非 Latest；注释 Tag object `83fa0958f6e3700c3a14f008a9530e8d59862207` 解引用到候选 `370877ed`。
- Release 附件：14 项已上传；`SHA256SUMS` 含 11 项二进制/部署归档，逐项与 GitHub asset digest 比较全部匹配；清单自身 digest `sha256:96f8a6447748c8c4dc51a8a0ef588042e3e68d3fb759bcf6ebba524a450e1253`。
- Docker 版本与通道 OCI index：`1.22.0-rc.1` 与 `preview` 均为 `sha256:7c540253e0091c11c76d2be67561c4e17c1a7d5a42a5c320458bc4d67d86f860`；amd64 `sha256:37aa40476429351c2ce9ec5a322ea35f034d563c6fb14b6d6743aa5786523600`，arm64 `sha256:072e00c499bcb2ff4f5655a9707279de1450c6e90fc937959ee816dc6583f64b`。
- GitHub Latest 仍为 `v1.21.0`；Docker `latest` 仍为稳定版 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`；发布前 `preview` 回退点为 `sha256:bd79e42938a319b325cdfd02d61241525428bef887d2f181636ea2f73d406308`。
- Release workflow 的原生镜像扫描及运行时契约通过；公开镜像 `image_e2e` 未执行（`arena-154` 不可达；`local-wsl-dr` 仅允许 candidate-validation）。发布前 API 证据：`releases-before.json`、`docker-latest-before.json`。
- 自更新实现本版未变。加入预览不自动安装、退出不降级、选择来源持久化和后台更新恢复由既有契约及本候选 L3/公开产物复核，当前不冒用历史结果。
- OpenRC 和轻量 Node 边界遵守 `docs/release-channels.md`。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）；生产写操作 0。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 生产版本、数据备份、正式升级和回滚健康证据不适用；隔离验收不作为生产证据。

## 回滚

- 已发布预览产物；若需撤回预览通道，可将 `preview` 恢复到发布前精确摘要 `sha256:bd79e42938a319b325cdfd02d61241525428bef887d2f181636ea2f73d406308`。`v1.21.0` 与 Docker `latest` 未变；不得用源码 tag 推断镜像存在。
- 若撤销本次源码，先反向撤销纠正补丁 `6f0c9ec0`，再按第一父线反向撤销 Transport 合并 `862ca2a9`；AI 修复可独立保留或反向撤销。实际操作须重新验证版本字段和工作树。
- 预览发布后若需要通道回退，可使用上文记录的旧 preview digest；本次未执行回退。生产和默认稳定通道保持不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-22T21:37:39+08:00
- 候选冻结时间：2026-09-23T15:31:34+08:00
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
    "fingerprint": "preflight/arena-ssh/connection-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一预检批次两次无法连接 arena-154，真机和浏览器验证不能执行；候选 L3 明确选择已登记的灾备环境。",
    "recoveryEvidence": "未恢复；SSH 连接超时工具输出；local-wsl-dr 用途检查只能证明候选 L3 允许执行。",
    "permanentAction": "稳定版候选前再次检查 arena-154 或按规范登记替代验收环境；完成受影响旅程真实验证前不得将其写作已验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependency/collector/network-configuration",
    "position": "before-production-write",
    "count": 1,
    "impact": "依赖报告第一次采集在 go list 阶段运行五分钟后中断；初次结果不完整，不能作为证据。",
    "recoveryEvidence": "使用标准代理配置重跑，10/10 来源成功；报告 SHA-256 记录在依赖章节。",
    "permanentAction": "后续依赖采集继续使用已验证的标准 HTTP(S) 代理出口；检查全部来源成功后才接受报告。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/go-test/file-share-expiry-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一候选 SHA 的首次 L3 中，TestFileSharePublicStreamExpiresAndReleasesCapacity 超过等待时间未结束，L3 exit 2。",
    "recoveryEvidence": "保留 r1 失败日志；未改候选代码，同 SHA 的 L3 r2 全量通过，status=passed、exit_code=0。",
    "permanentAction": "稳定版前复核该 500ms 过期用例的时序敏感性与测试等待边界；不忽略首次失败或把复跑改写成首次通过。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 预览产物已发布，真机、浏览器矩阵、20 轮性能/资源趋势和公开镜像 E2E 仍未执行；不代表专项验收通过。稳定版候选前补齐相应验证，并复核首次 L3 测试超时的原因。
- 新轻量 Node 连接旧中心时可选 stream 被拒绝后按 1 分钟至 30 分钟退避，既有轮询继续；不是完全无请求回退。
- 未修复审计线索保留本地，动态确认和修复单独排期；不公开可重用攻击细节。
- 本地资源回收：未执行；保留当前候选、原始审计、回滚 ref 和验证证据。未处理其他任务的脏工作树。
