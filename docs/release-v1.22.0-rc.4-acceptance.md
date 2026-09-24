# KPanel v1.22.0-rc.4 发布验收记录

日期：2026-09-24

发布级别：L3

候选提交 / 标签：`866d2aa8bd34e568fbd686d6c0aefa70f484e8f9` / `v1.22.0-rc.4`

上一稳定版本 / 回滚点：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3`

`releaseChannel`：`preview`

`releaseTrain`：`1.22.0`

## 候选分支与发布后处置

- 同序列远端 `release/v1.22.0-candidate` 保留，产品提交固定为上述 SHA；发布验收记录另作治理提交，不改已发布 tag。
- 本地组装分支 `release/v1.22.0-rc.4-assembly` 和源码 worktree 保留供后续追溯；管理 checkout 的 main 未被本任务切换或重置。
- 下列来源 tip 均为产品 tag 的祖先；已纳入 RC4，不再作为未发布功能重复选入。源工作树属于原任务，本轮未逐项取得所有权释放证据，保留本地分支、upstream 和 worktree；未删除或改名其他任务的引用。
- 后续协调者在下一次候选盘点且原任务释放所有权后，按项目管理 10.2 保存精确 tip 到 archive 引用，再处理活跃引用。本记录和不可变 tag 是恢复依据；不得按分支名或“已合并”批量删除。

| 来源 | 精确 tip | 处置 |
| --- | --- | --- |
| fix/desktop-touch-menu-20260924 | e7f917e00cfef0a05a908b35b907ed1b8e9c9031 | 纳入编辑器、图标、拖拽、手机搜索、长按完整链 |
| fix/cluster-toolbar-single-line-20260924 | 31bb25607cb5aebc13435b87073f3a556c14d89d | 纳入集群列表、头部操作及工具栏修复链 |
| fix/appmarket-responsive-columns-20260924 | 54272638c47867e19bafbe2b5556b8f39dc60583 | 纳入应用市场内容列宽修复 |
| codex/settings-desktop-grid | 380a0b7220e80b39db38a8644ae16c27db8a2f9b | 纳入桌面设置窗口按内容高度排列的 CSS 修复 |

## 发布画像与范围

- 业务域：文件管理、桌面交互、集群、应用市场、设置。
- 变更面：前端展示与交互、现有文件保存 API 的客户端状态；没有新增宿主机动作、API、数据结构、权限、端口或安装契约。
- 受影响旅程：目录侧栏、多文件标签草稿/撤销、正常与冲突保存、未保存关闭保护、手机主机搜索、桌面菜单与拖拽、容器宽度响应式布局。
- 风险：L3 发布；主要风险为客户端草稿状态、异步响应归属和移动端布局。编辑器限制最多 12 个标签、目录最多 500 项，仅挂载一个 CodeMirror。
- 准确增量为 `v1.22.0-rc.3..866d2aa8`，四个无冲突 merge 保留源 ancestry。产品增量 39 个文件；版本、CHANGELOG、审查记录与业务现状文档刷新均在冻结前完成。
- 明确排除：desktop-dynamic-scenes、desktop-scenes-gpu-finish、desktop-live-wallpaper、desktop-3d-scene-packs、blender-abyssal-ruins、celestial-palace-scene。相关工作树及未提交内容原样保留，不纳入本版。
- 早已公开的 AI、transport、monitoring 和 host-picker 候选不重复当作新功能；独立 files-solid-icons 的效果由合入的 748c9f08 链覆盖。

## 外部审计与修复交付

- 本次没有新 CF scoped/full 审计。覆盖检查 `target=866d2aa8bd34 decision=ok`；最近 full run-4（4c0694aa8e02，3 天），scoped run-7/run-6 覆盖后续边界；未审计 4 个边界提交/7 文件，最老 1 天，无新增边界包。中断的 run-3/run-5 不计覆盖。
- RC 仅记录覆盖结论，不把未审计增量写成已审计；本轮无新安全修复 fingerprint。
- OCR 1.12.6：先自由审查落证，再约束审查；范围 5796a0c9..c87f00b3，35/35 可审文件，排除文档和锁文件 3 项，H0/M0/L0，constrained-only=0。后续 866d2aa8 只刷新既有业务现状文档。
- 实现来自独立开发任务；本发布任务审查异步归属、资源限制、草稿/保存竞态、生命周期和回归。Claude CLI 未安装，使用独立 Codex 发布任务 fallback，提交 trailer 如实记录；不把同一实现任务自审冒充跨模型复核。
- 本 RC 不是稳定周期工具有效性结论，不据单次零发现评价 OCR。

## 跨仓库联动判定

- `scriptLinkageState=not-required`：无需发布脚本（不适用）。
- 变更集编号、脚本候选、阻断依赖：不适用。
- 实际内置 `kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`，Dockerfile 固定且 managed-script 契约门禁通过。
- 与 v1.21.0 相比 `packaging/kejilion-app/kpanel.conf` 无变化。apps 仓库 `39b498a0dc6b3013fda31b103c138ad0df3cc42c` clean；归一化仅既有 app_url（GitHub 与 Docker Hub 展示链接）差异，安装/更新逻辑一致，默认仍 latest。本轮无需应用市场提交。
- 使用既有文件保存 resourceVersion 契约；无脚本协议、运行时动作、宿主机产物或外联配置变化，不创建脚本或 apps 空提交。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go、1652 前端用例、mock 保存后重新打开回读、409 冲突草稿保留 | mock 不证明真实宿主机文件写入；服务端/脚本契约未变 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、固定 Trivy 源码/镜像扫描、CI、既有审计覆盖检查 | 不等同于新增渗透测试 |
| 稳定性、失败恢复与兼容 | 已验证 | 特权包 race、编辑器切换/生命周期回归、安装失败/回滚/卸载夹具 | 真实 systemd 更新与故障恢复本轮未重做 |
| 性能与资源预算 | 已验证 | 12 标签/500 项边界、单编辑器挂载、发布镜像资源约束启动 | 未跑真实宿主机 soak；确定性 UI 增量无新后台循环 |
| 用户体验与可访问性 | 已验证 | 1280×720、1440×900、390×844，浅/深色，中英文，键盘切换与菜单焦点 | 真实触摸长按、125%/200% 缩放、原生关闭确认人工交互未完成 |
| 数据、配置与迁移 | 不适用 | 无数据格式及安装配置变化，已有保存版本检查仍保留 | 不需要迁移或生产数据写入 |

## 自动门禁

- `make verify-release`：固定 Ubuntu WSL / Linux Runner 中完整通过；前端 185 files / 1652 tests，全量 Go、特权核心 race、go vet、govulncheck、npm audit、Trivy source/image、Linux amd64/arm64 构建、内置脚本契约、app_conf_lifecycle 均通过。
- L3 r1 在离线预检被 business context 63>=50 提交阈值拦截，未执行产品测试；更新原有业务现状文档后重新冻结。
- 有效 run：`v1.22.0-rc.4-local-l3-r2`；2026-09-24T10:37:39Z 至 10:45:10Z，status=passed，exit_code=0。
- Runner `kpanel-release-gate:go1.26.7-node24`，ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；Go 1.26.7 / Node 24.20.0。
- bundle SHA-256 `c1a6ba94479f6c4672c3c84c260893113409af0f8c66fffdca430f8ae879ddb3`。
- plan SHA-256 `9e5b7e5fb59f3a80fb793cbf0ac77a75299b33f27af16b1b1586f8d5e668f852`。
- 执行脚本 SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- 完整证据：`C:/GitHub/_release-evidence/v1.22.0-rc.4-local-l3-r2`，其余证据 `C:/GitHub/_release-evidence/v1.22.0-rc.4-20260924`。失败尝试均保留。
- 候选 CI：[35989100310](https://github.com/kejilion/KPanel/actions/runs/35989100310)，同一 SHA、success。
- 主线 CI：[35989864400](https://github.com/kejilion/KPanel/actions/runs/35989864400)，同一 SHA、success。
- Release workflow：[35990618473](https://github.com/kejilion/KPanel/actions/runs/35990618473)，同一 SHA、success；源码、漏洞扫描、安装生命周期、双架构附件和最终镜像均通过。
- 镜像运行时核对非 root 65532:65532、只读根目录、cap-drop ALL、256 MiB / 1 CPU / 128 PID、健康检查、版本/源码/许可标签及内置脚本摘要。多架构构建启用 SBOM/provenance。

## 发布产物与公开仓库复核

- [v1.22.0-rc.4 Release](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.4)：2026-09-24T11:10:18Z 公开，draft=false、prerelease=true；GitHub Latest 仍为 v1.21.0。
- Docker `1.22.0-rc.4` 与 `preview` OCI index 相同：`sha256:b7401060adee40926ae690950db1d67f2d6b4462526d1ee1ed6d2b844abda4ca`，与 Release 正文唯一“预览镜像”一致。
- linux/amd64：`sha256:1e33d4290c3abc88ab8dd64f1c88fd03fa4a7948d66f8cff53e571e54ddba895`。
- linux/arm64：`sha256:e36bdf199d57d60f4a6d69758aaf511af5ac508bbb6e481398edc8fbebd49855`。
- OCI index 另含两个 unknown/unknown 证明条目；不把它们计为运行平台。Docker `latest` 仍为 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。
- Release 14 个附件：Agent 双架构、Node 双架构、MCP 六平台、部署归档、LICENSE、THIRD_PARTY_NOTICES.md、SHA256SUMS。下载的 SHA256SUMS 摘要 `075c7ca0b1a62dc4b349ae6f21a2e44bc588b8cfc78017c63a8a65ee0c14c4e0` 与 GitHub 上传摘要一致，11 个产物条目逐一匹配对应资产 digest；未将未下载的二进制写成本机逐字节校验。
- 公开镜像显式 `docker pull` 成功；本地 inspect 的 RepoDigest、version=1.22.0-rc.4、revision=866d2aa8 均与官方发布一致。随后在 local-wsl-dr 的独立临时容器，使用精确源码中的 `packaging/tests/image-e2e.sh`、端口 18944，得到 `image_e2e=pass`、exit 0：版本健康、真实静态资源字节、引导账户、Secure Cookie、隔离网络和容器健康检查通过。测试入口负责清理临时容器、网络及数据；这不是宿主机部署或生产验收。
- apps / kejilion.sh 无需提交，详见跨仓库判定。预览候选远端保留。

## 依赖与技术栈变化

- 本版未升级 Go/npm、工具链、Action、扫描器、基础镜像或受管脚本。锁文件只变更发行版本字段。
- 最新版本检测最终报告 `dependency-report-r3.json`，2026-09-24T10:52:22.905Z，10/10 来源成功；前两次不完整报告保留，不当作完整证据。
- 30 个直接/基座候选（patch 11、minor 15、major/base 4）、160 个传递信号；emergency-security=0。安全通告依据同日 L3 与 CI govulncheck/npm/Trivy，不能仅凭“有新版本”认定旧版不安全。
- 没有本轮采纳或正式拒绝的升级；冻结后的发现进入下轮依赖评估，由项目维护者按首次完整检测与既有更早报告核对期限，不能用本报告重置已有期限。若以本次首次发现计：patch 最晚 10-01 启动、10-08 决策、10-24 处置；minor 10-08/10-24/11-23；major/base 10-24/12-23/12-23。安全期限更严格时优先。
- EOL review current，最近 2026-07-28，下次 2026-10-28；无到期例外。检测不是采纳批准，不在本版临时升级。

## 隔离真机与浏览器验收

- 本地 `local-wsl-dr` 只用于 candidate-validation；没有远程真机或生产部署。Windows 本地 mock UI 按唯一 local-feature-preview 入口启动，证据级别 mock-ui，不能替代宿主机业务结果。
- 有效 preview ID：`rc4-editor-desktop-cluster-1790246253197-d5bf77`，source=866d2aa8，clean，visual-composition；本地 4284/8184，结束后两个进程已停止。
- 核心事实：多标签草稿保留；正常保存后关闭重开读到 rc4-browser-draft；conflict.conf 返回外部修改错误且草稿保留；读取失败有重试且保存禁用；手机主机菜单持续展开、不自动聚焦、搜索能过滤；集群窄屏与应用卡片列数正确；桌面设置窗口不拉伸；右键空白菜单首项获焦。
- 英文手机编辑器 active tab 可见，ArrowLeft 切换到前一文件，“更多操作”聚焦查找；编辑内容 computed font=14px，仅一个 contenteditable，1280 下无页面横向溢出。布局/英文编辑器 tab warn/error=0。
- 原生 confirm 使内置浏览器 tab 2 控制超时，其他旅程通过新标签完成；不宣称关闭确认人工验收通过。自动回归覆盖未保存关闭。真实触摸长按无所选 API 支持，以长按回归和鼠标菜单检查作有限证据。
- ctrl+plus 没改变 viewport/DPR，不宣称 125%/200% 缩放通过。截图保留在任务工具记录，`preview-r2/browser-acceptance.json` 保存结构化结果。
- 无新后台生命周期或重连实现，不机械进行 soak。真实宿主机写入、失败注入、systemd PID 1 更新回滚未执行；不以 mock fixture 的版本号充当发布版本。

## 自更新通道验收

- 本轮沿用已有契约；release-channel、self-update、Panel/Agent 与应用配置测试通过，覆盖 stable/RC 选择、唯一官方 digest、resourceVersion、显式检查/安装分离与退出预览不降级。
- 加入预览只切换来源且检查，不隐式开启自动安装；旧状态默认 stable，持久选择、失败隔离与备份/回滚由现有自动测试覆盖。真实 systemd 更新本轮未实机重演。
- OpenRC 完整自动更新不支持，轻量 Node 仍跟踪稳定版；未放宽边界。

## 生产部署安全核对

不适用（预览版禁止生产部署）。产物公开不代表生产上线。本次没有连接、备份、部署、升级或核对 prod-108/108，也没有连接远程生产。GitHub Latest、Docker latest 和 apps 默认入口保持上一稳定版。

## 回滚

- 上一预览 `v1.22.0-rc.3` / 源码 `5796a0c9a379d17316a921a7ef3bf3cc9ccdcaa3`，镜像 `sha256:d723f4117d5972c6194f478a2f413b67b9c222982d9bb9d7f996490f5ad22e4e`。
- 上一稳定 `v1.21.0` 镜像 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。
- 本次无生产数据/配置改动，无生产回滚。用户若需人工回滚，另行明确目标 digest、备份数据、使用标准更新事务并验证版本/健康；关闭预览计划不自动降级。
- 公共默认通道仍 v1.21.0；本次不涉及恢复默认通道的决策。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-23T21:59:23+08:00
- 候选冻结时间：2026-09-24T18:36:47+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/run-release-l3/stale-business-context",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1 离线预检因 63>=50 提交阈值停止，未执行产品测试。",
    "recoveryEvidence": "l3-r1.log；866d2aa8 刷新已有业务现状文档；r2 终态 passed。",
    "permanentAction": "既有唯一 freshness 入口正常拦截；本次通过 866d2aa8 完成要求的文档刷新，门禁未放宽。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview/local-feature-preview/restricted-auto-port",
    "position": "before-production-write",
    "count": 1,
    "impact": "自动分配 4190，Node fetch 因受限端口拒绝就绪，初始浏览器落入连接错误页。",
    "recoveryEvidence": "preview/manifest.json 与进程日志；preview-r2 使用显式 4284/8184，ready 后完成浏览器验收。",
    "permanentAction": "本次使用入口已有 web-port/api-port 参数恢复；自动端口排除规则仍由项目维护者于 2026-10-01 复核，以预检排除受限端口并有回归为退出条件，未宣称已修复入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview/local-feature-preview/unsupported-port-argument",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次重试使用不支持的 --port，被参数校验拒绝，未创建预览进程。",
    "recoveryEvidence": "任务工具记录；改用脚本支持的 --web-port 后 preview-r2 ready。",
    "permanentAction": "依据唯一入口实际参数执行 --web-port 4284 --api-port 8184；不修改参数门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/cua-native-dialog/interaction-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "触发未保存关闭操作后 tab 2 控制超时，无法验证确认框接受/取消。",
    "recoveryEvidence": "任务 CUA 记录及 preview-r2/browser-acceptance.json；新标签完成其余旅程，关闭保护已有自动回归通过。",
    "permanentAction": "记录浏览器控制局限；项目维护者于 2026-10-01 复核可用的原生对话框交互路径，退出条件为实际验证确认/取消，不把自动测试冒充人工操作证据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/report-dependency-freshness/missing-local-runtime",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 首轮依赖报告缺 Go，部分网络源失败，5/10 来源，未作为完整报告。",
    "recoveryEvidence": "dependency-report.json/log；固定 Linux Runner r3 报告 10/10。",
    "permanentAction": "后续同类报告复用固定 Runner，并按 L3 的现有标准代理变量转发方式预检；本次已使用该路径成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/report-dependency-freshness/ambient-proxy-not-forwarded",
    "position": "before-production-write",
    "count": 1,
    "impact": "独立 Runner 报告未转发现有代理，Go/HTTP 源不可达，仅 1/10 来源。",
    "recoveryEvidence": "dependency-report-r2.json/log；r3 使用 host 网络和按变量名转发既有代理，10/10 来源成功。",
    "permanentAction": "复用 run-release-gate 的变量名转发模式和 Node 标准环境代理支持；代理值未写入命令、证据或镜像。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/powershell/approval-policy-rejection",
    "position": "before-production-write",
    "count": 1,
    "impact": "清理本任务 node_modules/dist 的命令被自动审批拒绝，未执行文件删除。",
    "recoveryEvidence": "resource-cleanup.json；工具返回 blocked by policy；目录保留且发布不受影响。",
    "permanentAction": "保留资源，不以其他机制绕过拒绝；项目维护者于下一次已授权清理时复核，退出条件为明确允许的精确目录清理及复核证据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "image/docker-pull/public-layer-transfer-retry",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开镜像最后一层传输缓慢并触发一次 Docker 自动重试，延迟本地 E2E。",
    "recoveryEvidence": "public-image-pull.log 最终 exit 0、公共 index digest 一致；public-image-e2e.log 为 image_e2e=pass。",
    "permanentAction": "保留原始传输和摘要证据，不改 registry 来源或使用本地构建冒充公开产物；项目维护者在下轮发布前复核既有网络传输，退出条件为公开下载及 E2E 完成。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/verify-change/trailing-blank-line",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收文档候选 e5b81932 的 CI 被末尾多余空行拦截，主线未快进到该文档提交。",
    "recoveryEvidence": "CI 35993136014 / job 107611715008；acceptance-ci-r1-failure.log；修正后重新验证文档候选。",
    "permanentAction": "本次删除末尾空行，新增文件暂存后执行 git diff --cached --check，再执行 metrics 校验；不再用忽略 untracked 文件的空 diff 当作新增文档格式通过。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 动态壁纸/3D 场景全部留在后续候选，没有发布或丢弃其代码。
- UI 未实测项目如上；本版没有对应已确认产品失败，自动回归及实际可执行旅程通过，因此作为 RC 公开观察。不得据此宣称完整真实触摸、跨浏览器或缩放认证。
- 来源分支本地归档待所有权释放；已发布身份由精确 tip、main 和不可变 RC tag 证明。
- 本地资源：本任务 preview-r2 的 web/mock API 进程已停止。删除本任务 `web/node_modules` / `web/dist` 的 PowerShell 命令被自动审批以 `blocked by policy` 拒绝，未执行文件删除，净释放字节按 0 记录；目录继续保留。L3 源码、Runner/缓存、原始日志/manifest/bundle 和全部他人 worktree 保留。当前 C 盘仍有约 88 GB 可用空间，未执行共享 Docker prune。
