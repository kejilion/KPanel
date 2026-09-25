# KPanel v1.22.0-rc.8 发布验收记录

产品发布及公开镜像验收已完成；纯验收提交独立走 CI 与归档，不改变产品标签。

日期：2026-09-25。发布级别：L3。releaseChannel：preview；releaseTrain：1.22.0。

- 候选提交 / 标签：25a4655adb16040e17cf2136f627bad10be23ec7 / v1.22.0-rc.8。
- 发布前 main：6b768c238738058924ec46ef05cba4faee824474（RC7 已推送但 Release 失败，未公开）。
- 上一已发布预览 / 回滚点：v1.22.0-rc.6，245caaa9e85d9e2432b4c7743aa97212d1f5d044；镜像 index sha256:9f69a7752aa108eebfa88e3b3e638758e166eb7856d4f068453bb587ea81702b。
- 上一稳定版：v1.21.0，396fcd62c5635c9812ee97ee509cb61b962724f3；镜像 index sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。
- 本地组装归档：archive/release/v1.22.0-rc.8-assembly，C:/GitHub/_codex-tasks/kpanel-v122-rc8。
- 证据根目录：C:/GitHub/_release-evidence/v1.22.0-rc.8-20260925；最终 L3：C:/GitHub/_release-evidence/v1.22.0-rc.8-l3-r3。最后一次冻结前 main 为 e5ede1e6296caef37d0886265533f168fb7caa4c；此前候选 CI 成功但主线 race 被 submitted 中间态拦截。

## 发布画像与范围

承接 RC7 未公开的壁纸、场景加载与页面标题功能，并修复阻断发布的 MCP 异步测试。RC8 相对 RC7 仅修改一个测试文件、四个版本文件与 CHANGELOG；没有改变生产 MCP 权限实现、Agent 权限、API、业务数据库、端口、Compose 或安装协议。预览版不执行生产部署。

| 来源 | 纳入提交 | 范围 |
| --- | --- | --- |
| feature/desktop-3d-scene-packs-20260924，tip 1a5e52d90dd287260fac9f9d7b2e6568c7b3e17c | 03ba38d8 → 46f35030；1a5e52d9 → 5a8a0bc6 | 场景启动优化、海天日月减重、霓虹都市楼体 |
| fix/desktop-browser-title-20260925，tip 5283eb74b72f89bb33e6bf23c9bc92faf3540787 | 5283eb74 → a62444dc | 桌面模式浏览器标题 |
| feature/classic-wallpaper-through-20260925，tip 4d360a2f96b4e2e21d640ad1663e9c1aa778eb62 | 94745957 → 04266faf；ce178876 → 493ce231；4d360a2f → 1912ba95 | 经典模式关闭/氛围/通透及共享静态/3D 壁纸选择器 |
| RC7 集成 | d750f9b9、bfa1c124、b85a3819、6b768c23；审查 e9e36330、144bcc0c | 真实 Go 场景缓存/gzip、翻译、活动场景更新后刷新、夹具修正 |
| RC8 修复 / 版本 | 549f1173、b58c642d、e5ede1e6（来源 07568ba5） | MCP 异步拒绝、繁忙返回的有界等待及受控慢响应回归；RC8 版本和完整更新说明 |
| RC8 远端状态补全 | 25a4655a（来源 9a5aedac） | 固定经过 submitted 回执，再通过 operation_status 观察远端终态 |

标题与经典壁纸源提交已通过 stable patch-id 核对。启动优化经过适配，保留随机、可撤销 capability，采用 private/no-cache/must-revalidate、ETag/304 与达到至少 10% 收益的 gzip；没有承诺一年 immutable 缓存、Brotli 或第二次零请求。来源映射见 source-mapping.json。

场景始终只有 neon-city 1.1.0、orbital-station 1.0.1、sea-and-sky 1.0.1。没有纳入倒悬月宫、深海遗迹等独立分支。海天日月目录原始文件总量由 10,564,422 降至 5,965,466 字节（约 43.5%），不等同网络传输量。并行加载、着色器准备、1.2 秒进度提示、2.2 秒淡入和按最后进度消息计算的 20 秒超时均承接 RC7；正式包总启动秒数未实测。

## MCP 修复与审查

RC7 Release run 36135146079 的 race 测试在 mcp_cluster_test.go:193 收到 executing 后立即要求 failed。历史 v1.21.0-rc.12 run 35698886207 出现过完全相同失败。生产执行器允许 100 ms 后先返回 executing，该实现与测试在 RC6 到 RC7 未变。

549f1173 在测试中用 channel 阻塞目标 Agent 的垃圾箱查询，确保 operation_execute 先返回 executing；释放后最多等待 3 秒读取持久操作终态。继续严格要求 state=failed 且 Agent 写入调用数仍为 1。同步使用 atomic 和 sync.OnceFunc，取消与退出均能释放等待；没有重试越权动作或放宽生产权限。

定向测试：固定 Go 1.26.7 Runner，test -race -run '^TestMCPClusterHTTPSApprovalLostReceiptRecoveryAndTargetRevocation$' -count=10 -timeout=120s ./internal/panel，10 次通过，13.540 秒，证据 mcp-focused-race-r2.log。首轮回归夹具将延迟放在 HTTP 层，误阻塞同步 capabilities 请求，10 次失败；已移到 Agent 查询层，原失败日志保留，不作为通过证据。

上述 10 次是追加繁忙等待修复前的证据。完整 L3 r1 的整包 race 暴露同一测试另一处时序错误：繁忙目标合法返回 executing 后，测试立即释放目标 workerSlots，再断言 approved。e5ede1e6 在目标名额持续占满时最多等待 3 秒，读错误标记测试失败，释放名额后仍断言 approved 且 Agent 写入调用数为 0。L3 r1 保留 failed/exit 2，最终精确提交重新完整验证。

OCR 1.12.6：6b768c23..45664437，先自由臂再约束臂，6/6 文件复核（4 个工具圈选、2 个手工复核排除项），H0/M0/L0、constrained-only=0；最终仅 amend 修正 trailer，代码树不变。继承 RC7 来源功能审查记录，不声称这次测试修复经过新的外部独立审计。

繁忙等待补丁另行完成 b58c642d..71812ca7 自由臂及约束臂，1/1 文件、H0/M0/L0、constrained-only=0；最终 e5ede1e6 仅追加 trailer，代码树不变。

e5ede1e6 本地 L3 r2 与候选 CI 36139887706 通过，但主线 CI 36140821960 的 race 在 2026-09-25T13:32:00Z 发现控制器 state=submitted，内层 remoteOperation.state=executing。此前修复仅等待控制器 executing 结束，遗漏远端仍执行的中间态。25a4655a 保持目标 Agent 阻塞直到观察到 submitted，随后释放并通过 operation_status 有界刷新远端终态，仍断言 failed 与 Agent 调用数不变；没有修改生产权限或重发执行命令。完整路径 race 连续 10 次通过，13.720 秒（mcp-remote-status-race.log）。OCR e5ede1e6..358054a2，1/1 文件、H0/M0/L0，最终 amend 只追加 trailer。

安全覆盖检查：decision=ok，未审计边界提交 2、文件 1（RC7 Go 场景缓存/压缩），new_boundary_packages=none；继承 full run-4、scoped run-9/7/6。run-3/5 中止不算覆盖；run-9 动态验证缺口保留，没有新 CF 审计。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Go 场景服务回归、共享选择器测试、RC7 Mock 浏览器证据 | 用户实际面板三个场景播放未验证 |
| 网络入侵与供应链安全 | 已实现未实机验证 | MCP 慢响应下仍拒绝越权；安全覆盖检查与发布扫描 | 继承安全审计动态覆盖缺口 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 受控异步 race 回归；下载失败保留场景；完整门禁 | 无真实弱网、GPU 长测或宿主故障注入 |
| 性能与资源预算 | 已实现未实机验证 | 场景减重、ETag/gzip 与并行加载；镜像资源限制契约 | 未测正式包总启动时间、低端 GPU 显存与帧率 |
| 用户体验与可访问性 | 已实现未实机验证 | RC7 三场景、浅深色、窄屏、键盘与三语证据 | RC8 未新增浏览器验收；125%/200% 缩放和系统动态偏好未验收 |
| 数据、配置与迁移 | 已验证 | 无 schema 迁移，安装与脚本契约未改 | 生产迁移/回滚不适用 |

## 自动门禁与公开产物

- 最终 L3 唯一入口 scripts/run-release-l3.mjs，run ID v1.22.0-rc.8-l3-r3；local-wsl-dr / Ubuntu / root Docker；精确候选 25a4655adb16040e17cf2136f627bad10be23ec7，基线 e5ede1e6296caef37d0886265533f168fb7caa4c。2026-09-25T13:36:04Z 至 13:43:23Z，status=passed、exit_code=0，回收证据 12 项摘要全部一致。
- 固定 Runner kpanel-release-gate:go1.26.7-node24，ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d。bundle SHA-256 3121b72783c12a13b42fd68413804faec9b5633d3489b13c8b482b1a700a1d21；plan 4ac3df979a7c45c155f91a5dc79899125ab3f0d3a3d7e823b524d558c3ff623e；manifest 3b7a5045ea0599d3a0bd9d994da3349322ea2ea3117da0b3e3abea12ebe722f9；执行脚本 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979。
- 全量 Go、192 个前端文件 / 1,691 项测试、类型检查、3 包源码/dist 复现、核心 race/vet、语言目录、Linux amd64/arm64 二进制、源码/最终镜像 Trivy、app_conf_lifecycle 全通过；核心 panel race 110.896 秒。日志 SHA-256 29b216edfc586b4aa3c30d7ffb0f4abd007f73421886aca1213b33f84da1dc51。
- 最终本地验证镜像 sha256:c442c4198799083c328dd80868ff36e50eb6aec76a1cd83adcf68612a1c25fa5；这是本地验证摘要，公开镜像另行核对。

以下 r1/r2 为前一候选的历史尝试，不能代替最终提交的 r3：

- L3 唯一入口 scripts/run-release-l3.mjs，run ID v1.22.0-rc.8-l3-r2，local-wsl-dr / Ubuntu / root Docker；2026-09-25T13:07:41Z 至 13:15:40Z，status=passed、exit_code=0。候选 e5ede1e6296caef37d0886265533f168fb7caa4c、75 个稳定标签、v1.21.0 及业务基线核对通过；回收的 evidence.sha256 12 项全部一致。
- Runner kpanel-release-gate:go1.26.7-node24，不可变 ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d。bundle SHA-256 5842e1dfe06426efe8a60b106c94592f5f41bca1328dab9b8ed225eed731f872；plan 44e3dd6c1008ef3451b1c9f55f25c8ed29f4ec18bf5fd27c34b733bf854f104f；执行脚本 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979；manifest cae15d5dc2c85f06d75702076a44e7717d29cea11e716139d5834a6f871d8a15。
- 全量 Go、192 个前端文件 / 1,691 项测试、类型检查、3 包源码/dist 复现、核心 race/vet、语言目录、Linux amd64/arm64 二进制、源码/最终镜像 Trivy、app_conf_lifecycle 全通过。核心 panel race 为 96.936 秒。安装生命周期日志中的中断、错误版本和回滚输出均为既有失败注入用例，最终 app_conf_lifecycle=pass。
- 本地验证镜像 ID sha256:c442c4198799083c328dd80868ff36e50eb6aec76a1cd83adcf68612a1c25fa5，仅是本地构建证据，不作为公开镜像摘要。npm audit 0；源码与镜像扫描未检出门禁范围内问题。既有 glob deprecated 和 Vue 测试 stub warning 保留，未视为新增产品错误。
- r1 在 race 被繁忙目标时序问题拦截，2026-09-25T12:59:49Z 至 13:05:29Z，failed/exit 2；修复后以新提交和新 run ID 完整重跑，没有跳过门禁。

最终产品候选 CI [36142814060](https://github.com/kejilion/KPanel/actions/runs/36142814060) 与 main CI [36143367222](https://github.com/kejilion/KPanel/actions/runs/36143367222) 均 success，精确 SHA 均为 25a4655adb16040e17cf2136f627bad10be23ec7。通过后创建 annotated tag v1.22.0-rc.8，tag object b954de7a7523cee023bd2e89a084b84023919645，未改写 RC7 标签。Release run 36144066119 的终态见公开产物记录。

[Release 36144066119](https://github.com/kejilion/KPanel/actions/runs/36144066119) success；[公开 RC8](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.8) 于 2026-09-25T14:05:13Z 发布，draft=false、prerelease=true。源码、核心 race、安全扫描、安装生命周期、Agent/Node/MCP 二进制、原生镜像扫描和运行时契约全部通过。标签依赖检查 36144066092 success。

公开附件 14 项逐个下载校验 GitHub digest、实际 SHA-256 和大小，SHA256SUMS 的 11 个受校验附件全部一致。精确产品 SHA 和 main 的场景目录与本地一致，3 个包 / 71 个文件 / 11,195,876 字节逐项校验通过。

版本镜像与 preview 均为 index sha256:626122acfa5c467aba66efcac751498201924b7638643ad2c3e5904e8470de85；linux/amd64 为 sha256:697362c563b20fafc55ba49e51e939dd2e857a5f3905adc9c5086662554a9372，linux/arm64 为 sha256:78770a14e379b3882506235503b116bd7f9f087ca474fc6dd0aafe5e87e4ecdb。额外 unknown/unknown 条目为构建证明。Release 说明中的镜像摘要也一致；GitHub Latest 仍为 v1.21.0，Docker latest 仍为 sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。

证据位于 public-verification/github-release-api.json、github-release-assets-verified.json、sha256sums-verified.json、public-scene-verification.json、public-channel-verification.json、release-run.json 和 release-jobs.json；核验脚本 exit 0。

公开镜像本地验收通过：最初 Docker 拉取因网络缓慢取消，官方地址直连和 Windows 两次下载均失败；全部原始日志保留。网络恢复阶段通过 Windows 既有代理读取同一公开 registry 的 index、amd64 manifest、config 与 17 层，校验压缩 SHA-256、大小及解压 rootfs diff_id，生成经过摘要校验的 Docker 导入归档。没有使用本地构建镜像替代公开产物，也没有修改 Docker daemon 或系统代理。

导入后再次普通 docker pull docker.io/kjlion/kejilion-panel:1.22.0-rc.8 成功，公开 index digest 626122acfa5c467aba66efcac751498201924b7638643ad2c3e5904e8470de85；OCI version=1.22.0-rc.8、revision=25a4655adb16040e17cf2136f627bad10be23ec7、User=65532:65532，镜像配置 ID sha256:68b932f771770413fae44c3df796ce1b61b0cf027fdcb6f7469fd64b18c70316，与公开 config digest 一致。

最终使用 L3 r3 精确源码中的既有 packaging/tests/image-e2e.sh，local-wsl-dr / Ubuntu / root Docker，端口 18088，KPANEL_EXPECTED_VERSION=1.22.0-rc.8。exit 0、image_e2e=pass：只读根、cap-drop ALL、no-new-privileges、健康版本、公共资源真实字节、代理来源入口、bootstrap Secure cookie 与 Docker 健康状态通过；临时容器、网络与测试目录由脚本清理。公开 E2E 没有额外施加内存/CPU/PID 限额；256 MiB / 1 CPU / 128 PID 的证据来自 Release 的 Verify runtime image contract，不混为同一测试。

证据：public-image-acquisition.json、public-image-acquisition-resume.log、public-image-load.log、public-pull-final.log、public-image-identity.json、public-image-e2e.log；临时网络取件脚本仅在仓库外保存，没有创建新的 L3 包装脚本。

## 依赖、跨仓库联动与应用市场

- 本版没有升级 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本；锁文件只改产品根版本。
- 依赖检测复用 2026-09-25T02:02:26.128Z 的完整报告 v1.22.0-rc.5-20260925/dependency-report-r2.json，10/10 来源、0 失败、0 emergency-security；SHA-256 c81dbd1a7e23a91ca6341d5191e67845bee75188c1a97f9e6666a77174fe4c47。提示的新版本未在本版采用；本轮安全扫描结果见自动门禁。
- scriptLinkageState：not-required（无需发布脚本（不适用））。实际内置脚本 commit 2b90b2d2ca56bc954c9328a51bb5571e896f713d，SHA-256 806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99。未要求新运行时动作、协议或安装路径；脚本候选、跨仓变更集、阻断依赖均不适用。
- kejilion/apps 工作树 clean，HEAD/远端 main 为 39b498a0dc6b3013fda31b103c138ad0df3cc42c。kpanel.conf 归一化换行并排除既有 app_url 展示链接差异后，安装更新契约一致；不宣称整文件同哈希，无需 apps 提交，默认入口仍使用 latest。

## 浏览器、环境与自更新

RC7 精确 6b768c23 的 mock 预览证据保存在 v1.22.0-rc.7-6b768c23-preview/browser-observations.json：三个场景 running，静态选择移除 iframe，删除当前场景回默认，重新下载可应用，经典/桌面切换标题、浅深主题、390px 窄屏和键盘选择通过，控制台错误 0。RC8 没有 UI 实现差异；这些仅是原 SHA 的历史证据，没有冒充 RC8 精确提交或真实 Go 后端浏览器验收。

本次测试/版本修复没有新增可见旅程，不另开 acceptance 预览。浏览器连接当前不可用，真实后端、正式包性能、GPU soak、125%/200% 缩放和动态 reduced-motion/transparency 未验证。普通旅程没有引入新的长连接资源风险，不机械新增 soak。

local-wsl-dr / Ubuntu / root Docker 仅用于登记的候选验证。自更新契约不变：加入预览只切来源并检查，自动安装与立即安装独立，退出不会自动降级；OpenRC 自动更新和轻量 Node 继续遵守既有边界。本轮系统级更新/备份恢复旅程未实机执行。

生产部署、生产备份、写入和回滚：不适用（预览版禁止生产部署）。prod-108 禁用全部 KPanel 操作，本次未连接、未备份、未部署、未升级、未核对。RC8 全程使用本地验证，没有连接 arena-154。公开预览可用不代表用户机器已经安装。

公共默认更新通道决策：不适用（预览版）；GitHub Latest、Docker latest、应用市场默认保留稳定版，实际摘要核对见公开产物。回滚点为页首 RC6 精确 digest；未执行自动降级或数据恢复。

## 分支与资源处置

- 远端 release/v1.22.0-candidate 保留在产品 SHA 25a4655adb16040e17cf2136f627bad10be23ec7；本轮为 preview，按规则不归档该序列候选。不可变 RC8 tag object b954de7a7523cee023bd2e89a084b84023919645，peeled 为同一产品 SHA。
- 本轮三个本地修复分支已由本任务释放所有权并保存精确 tip：archive/fix/mcp-async-release-20260925 → 549f1173f18bcb6984c2ead6f2c8b4e9060e0b1d；archive/fix/mcp-busy-async-release-20260925 → 07568ba5b6397aa8da4ce9cce71170ddd13feefe；archive/fix/mcp-remote-status-release-20260925 → 9a5aedacb41a1061ec9fed2ac7244719fac46dbc。这些原任务分支未推送远端；首项是 RC8 祖先，后两项分别与集成 e5ede1e6 / 25a4655a 完整树一致。
- 已移除本任务的 kpanel-rc8-mcp-busy 与 kpanel-rc8-mcp-status 两个工作树：路径、所有权、clean/ignored、嵌套仓库、reparse point、进程和恢复 SHA 均复核。逻辑文件分别 40,913,246 / 40,913,663 字节；C 卷空闲从 90,915,946,496 到 91,007,643,648 字节，观察净变化 +91,697,152 字节，可能含并发系统活动，不把逻辑大小当实际释放量。证据 cleanup.json。
- 产品工作树、验收工作树、RC7 用户预览、原始失败日志、L3 bundle、WSL 验收源码和固定 Runner 保留用于验收与恢复。未跨任务清理，没有全局 prune。
- 本验收记录使用独立 docs/release-v1.22.0-rc.8-acceptance，基于产品 25a4655a；它须单独完成精确 SHA 的候选 CI、主线快进和主线 CI，再按 expected-SHA lease 保存至 archive/docs/release-v1.22.0-rc.8-acceptance 并移除远端活跃引用。本记录的提交 SHA、CI 和最终远端归档复核写入本轮证据 completion.json；不追加到已发布 RC8 标签或产品候选。

来源分支按表中精确 tip 证明纳入；原作者未释放或已继续开发的工作树保留，禁止按旧分支名称强删。下一次所有权释放时由协调中心复核归档；其已纳入内容不再算未发布新候选。RC7 失败 tag、全部失败日志与 L3 bundle 保留。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-25T11:52:07+08:00
- 候选冻结时间：2026-09-25T21:34:14+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：是（RC7 发布前被测试拦截，修复后递增 RC8；未覆盖旧版本或回滚生产）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-09-25T12:36:00Z；恢复时间：2026-09-25T14:05:13Z；逃逸门禁：未逃逸：RC7 Release 的 Verify source 在产物公开前拦截
<!-- kpanel-release-metrics:end -->

本版是预览，不计稳定发布或生产部署频率。重复发布源于测试时序缺陷，不是生产业务退化。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：18
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-l3/ssh-transport/connect-timeout",
    "position": "before-production-write",
    "count": 2,
    "impact": "e9e36330 的 arena-154 两次尝试均在上传/执行候选前 SSH 连接超时；不是产品测试失败。",
    "recoveryEvidence": "v1.22.0-rc.7-e9e36330-l3-r1/r2 保留；6b768c23-l3-r2 使用登记的 local-wsl-dr 完整通过。",
    "permanentAction": "发布目标先匹配用户明确的本地发布要求，复用仓库唯一入口和登记的 WSL 灾备路径；网络例外负责人为本发布任务，复核日期 2026-09-26，退出条件为后续需要远端时 SSH 预检成功；不改远端。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/working-directory/nonexistent-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "接续时一次命令批次误将 assembly 分支后缀写入工作树目录，执行器拒绝启动；没有执行候选。",
    "recoveryEvidence": "后续所有候选命令固定使用 C:/GitHub/_codex-tasks/kpanel-v122-rc7，L3 manifest/source-prepare 精确匹配。",
    "permanentAction": "区分分支名和实际工作树路径，沿用 release-profile.json 的固定绝对目录；无产品文件变更。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preview/proxy-host/csp-origin-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "144bcc0c 第一次本地预览 change-origin=true 将 Host 改写为 API 端口，场景 CSP 阻止脚本，取证无效。",
    "recoveryEvidence": "保留 v1.22.0-rc.7-144bcc0c-preview；false 重启的 preview-r2 及最终 6b768c23-preview 三场景运行通过。",
    "permanentAction": "该场景预览固定使用 change-origin=false 保留浏览器来源；不放宽产品 CSP。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/trivy-database/unexpected-eof",
    "position": "before-production-write",
    "count": 1,
    "impact": "6b768c23-l3-r1 的 Trivy 漏洞库下载在 40.85% 处中断，门禁退出，未发布。",
    "recoveryEvidence": "同摘要扫描器及既有 security-scan.sh source 重试通过；6b768c23-l3-r2 完整重跑 status=passed、exit_code=0。",
    "permanentAction": "不绕过扫描，不换扫描器摘要；保留失败 run 后以新 run ID 重试。上游网络例外负责人本发布任务，复核日期 2026-09-26，退出条件为完整漏洞库下载与扫描成功（本轮已满足）。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence-check/local-file-resolution/input-parent-layout",
    "position": "before-production-write",
    "count": 2,
    "impact": "附加摘要核对两次误把 bundle/manifest 等输入文件定位到 wsl-evidence，文件不存在而停止；没有把失败当通过。",
    "recoveryEvidence": "检查真实目录后，4 个输入文件从 run 根目录读取、其余日志从 wsl-evidence 读取，evidence.sha256 的 12 项全部一致。",
    "permanentAction": "复核时先读实际文件布局，按权威 manifest 输入名映射根目录与日志目录；保留原始摘要清单，不复制/改写证据冒充首轮成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-source/mcp-cluster-fixture/async-state-immediate-assertion",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release attempt 1 race 测试在既有 MCP 异步执行合法返回 executing 后立即断言 failed，未等待终态；尚未生成公开 Release/镜像。",
    "recoveryEvidence": "失败原件已保留；最终 25a4655a 受控慢响应 race x10、L3 r3、候选 CI 36142814060、主线 CI 36143367222、Release 36144066119 全部通过；RC8 于 2026-09-25T14:05:13Z 公开。",
    "permanentAction": "549f1173 adds bounded terminal-state wait and deterministic delayed Agent response, preserving failed-state and write-count assertions. Historical recurrence verified in v1.21.0-rc.12 run 35698886207.",
    "historicalReleases": []
  },
  {
    "recoveryEvidence": "RC8 repair uses authorized SSH candidate/main/tag sequence and repository Release automation; RC7 remains immutable.",
    "permanentAction": "Do not depend on Actions write permission for source fixes; use normal SSH release workflow. Existing connector permission remains unchanged.",
    "position": "before-production-write",
    "fingerprint": "release-rerun/github-connector/actions-write-forbidden",
    "historicalReleases": [],
    "count": 1,
    "impact": "GitHub connector rerun returned 403; no second RC7 attempt started."
  },
  {
    "recoveryEvidence": "Retain failed browser observations; RC8 uses existing repository automation via SSH tag.",
    "permanentAction": "Treat browser unavailable as unavailable, verify actual run attempt via API, and publish repaired candidate through existing release workflow. Browser reconnection is outside this release.",
    "position": "before-production-write",
    "fingerprint": "release-rerun/browser-transport/connection-unavailable",
    "historicalReleases": [],
    "count": 1,
    "impact": "Browser recovery attempt batch failed to connect and could not trigger RC7 rerun; no success evidence produced."
  },
  {
    "historicalReleases": [],
    "count": 1,
    "fingerprint": "release-l3/mcp-cluster-fixture/busy-slots-released-early",
    "position": "before-production-write",
    "recoveryEvidence": "失败原件已保留；最终 25a4655a 受控慢响应 race x10、L3 r3、候选 CI 36142814060、主线 CI 36143367222、Release 36144066119 全部通过；RC8 于 2026-09-25T14:05:13Z 公开。",
    "impact": "RC8 L3 r1 race returned executing for busy target; test released target worker slots before checking refusal and required approved immediately.",
    "permanentAction": "e5ede1e6 waits up to 3 seconds for controller state while target slots stay occupied, releases slots before assertion, retains approved-state and zero-write requirements."
  },
  {
    "position": "before-production-write",
    "recoveryEvidence": "失败原件已保留；最终 25a4655a 受控慢响应 race x10、L3 r3、候选 CI 36142814060、主线 CI 36143367222、Release 36144066119 全部通过；RC8 于 2026-09-25T14:05:13Z 公开。",
    "impact": "e5ede1e6 main CI run 36140821960 failed because controller submitted receipt still contained remote executing state; the preceding fix only waited for local executing.",
    "fingerprint": "main-ci/mcp-cluster-fixture/submitted-not-terminal",
    "historicalReleases": [],
    "permanentAction": "25a4655a covers both async layers via public status API and preserves final failed/no-extra-write assertions; initial executing and submitted are both legal. No production code or permission change.",
    "count": 1
  },
  {
    "recoveryEvidence": "公开 registry 的 index/amd64 manifest/config/17 层 SHA-256 与解压 diff_id 全部核对后导入；public-pull-final.log exit0，摘要 626122ac...；public-image-e2e.log image_e2e=pass、exit0。",
    "count": 1,
    "historicalReleases": [],
    "position": "before-production-write",
    "impact": "同一公开镜像下载批次中，r1/r3/r4 出现镜像层下载异常缓慢，r2 共享 r1 连接；这些拉取均取消并保留 exit130，不作为通过证据。",
    "fingerprint": "public-image/docker-pull/slow-registry-layer",
    "permanentAction": "现有镜像加速链路不稳定时先保留原日志，限定到公开镜像的精确摘要核验与续传；最终仍执行 docker pull 和仓库既有 image-e2e.sh。不修改 Docker daemon、系统代理或产品文件。网络例外由本发布任务负责，复核日期 2026-09-26，退出条件为公开摘要拉取与 E2E 成功（已满足）。"
  },
  {
    "recoveryEvidence": "Exact pgrep matches identified task-owned PIDs 1159 and 2430; both interrupted successfully before fresh r3 pull.",
    "count": 1,
    "historicalReleases": [],
    "position": "before-production-write",
    "impact": "Initial process verification shell command had invalid tr quoting, so it did not interrupt the pull. A second pull shared the existing daemon transfer and was not an independent recovery.",
    "fingerprint": "public-image/process-check/shell-quoting",
    "permanentAction": "Use direct WSL argv and exact process pattern, inspect the result before starting dependent recovery; avoid nested shell quoting for process checks."
  },
  {
    "position": "before-production-write",
    "recoveryEvidence": "public-pull-canonical.log 保留；最终公开镜像导入、正常 docker pull 与 E2E 通过。",
    "fingerprint": "public-image/docker-pull/direct-registry-timeout",
    "count": 1,
    "historicalReleases": [],
    "impact": "registry-1.docker.io 精确 amd64 摘要直连在响应头阶段超时，exit1，未取得有效镜像。",
    "permanentAction": "当前主机保留既有网络配置，通过 Windows 已配置代理读取公开字节并校验内容摘要；直连失败不扩大主机权限或重启 Docker。"
  },
  {
    "position": "before-production-write",
    "recoveryEvidence": "public-image-acquisition.log / public-image-acquisition-direct.log 保留；public-image-acquisition-resume.log exit0，17 层内容摘要和 rootfs diff_id 全部通过。",
    "fingerprint": "public-image/registry-download/transport-timeout",
    "count": 2,
    "historicalReleases": [],
    "impact": "Windows 首次代理下载 read timeout；一次仅作用于下载进程的直连尝试 connect timeout。两次均 exit1，没有导入不完整镜像。",
    "permanentAction": "下载使用既有代理、有限重试和 Range 续传，只有完整 SHA-256 一致后才能缓存或导入；公开镜像校验代码位于仓库外证据目录，不成为新 L3 包装入口。"
  },
  {
    "impact": "验收文档初稿给机器读取的 JSON 区块添加 Markdown fence，结构校验拒绝；未提交该无效初稿。",
    "recoveryEvidence": "对照模板去掉 fence，仅保留标记之间的原始 JSON，再次运行既有结构校验。",
    "permanentAction": "机器标记区块按既有模板写原始 JSON，提交前以 report-release-metrics.mjs --validate-acceptance 为准。",
    "count": 1,
    "fingerprint": "acceptance/report-release-metrics/markdown-json-fence",
    "historicalReleases": [],
    "position": "before-production-write"
  }
]
<!-- kpanel-release-process-incidents:end -->

流程异常沿用本次功能发布从 RC7 到 RC8 的实际记录，不把递增版本号视为清零；历史 v1.21.0-rc.12 的同类 MCP 失败已经核对。本次修复该夹具并增加受控慢响应覆盖。

## 遗留风险

正式包启动耗时、真实后端三个场景播放、低端 GPU 资源趋势、浏览器缩放与系统动态偏好仍未实测。自动门禁与公开镜像 E2E 不能代替这些结论；后续稳定版准入需补相应证据。RC7 原开发版测速与第二次零请求不作为线上收益承诺。
