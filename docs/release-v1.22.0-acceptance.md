# KPanel v1.22.0 发布验收记录

日期：2026-09-26。发布级别：L3。`releaseChannel=stable`；`releaseTrain=1.22.0`。

- 正式提交：`cea6261f9ae1de064216a1c423d3cd3f2d87d3fa`；注释标签：`v1.22.0`，tag object `47bb8b75c18125419402ba16ec1a930380b2e47f`。
- 发布前 `main`：`690f3dc24fe81377f4e17d27c68e4a174761b973`（RC10 验收）；上一稳定版：`v1.21.0`，源码 `396fcd62c5635c9812ee97ee509cb61b962724f3`，OCI index `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。
- 本版正式产物已发布；生产未部署。`prod-108` 禁用全部 KPanel 操作，本轮未连接、未核对、未备份、未部署。
- 证据根目录：`C:/GitHub/_release-evidence/v1.22.0-stable-20260926`；候选 L3：`C:/GitHub/_release-evidence/v1.22.0-cea6261f-l3-r1`。

## 发布画像与范围

- 业务域：桌面与登录外观、3D 场景、集群监控、手动/自动更新、轻量节点恢复、文件与终端交互。
- 变更面：前端展示、浏览器本地私有图片副本、Panel 图片和场景 API、宿主机更新事务及数据快照、目标镜像摘要绑定；没有数据库迁移。
- 核心旅程：上传自定义图、调整焦点并选择配色；经典模式“通透”和登录页使用所选图；应用 3D 场景并显示登录静态海报；放大集群指标时间范围后保持定位；更新前备份、失败恢复与轻量节点重启。
- 未变化契约：既有端口、Agent 权限、受管 `kejilion.sh` 内容。应用市场 `kpanel.conf` 安装/更新契约随本版同步。
- 风险等级：高。壁纸信任边界、场景文件服务与宿主机更新回滚均需 L3、审计覆盖、两级 CI 和公开镜像验收。
- 用户可见内容以 `CHANGELOG.md` 的 `[1.22.0]` 为准：三款官方 3D 场景、Worker/OffscreenCanvas 渲染与启动/缓存优化、自定义壁纸到经典模式/登录页、集群监控放大定位修复，以及文件/终端体验改进。
- 未纳入：生产部署；新到的 BusyBox `flock` 修复只进入 `kejilion/apps` 的公共入口，未进入已经冻结的 `v1.22.0` 镜像内置脚本，需后续补丁版同步。上传图片和已下载场景包不包含在设置备份，迁移后需重新上传/下载；登录页图片副本只保存在当前浏览器。

## 候选来源与分支处置

- `release/v1.22.0-candidate` 从 `690f3dc2` 快进至 `cea6261f`；稳定版纳入 9 个提交，依次处理写入冻结、版本准备、回滚重试、Upgrade、run-10 审计记录、目标能力标签和不可变镜像摘要。完整清单可用 `git log 690f3dc2..v1.22.0` 复核。
- Release workflow 自动把远端候选精确 tip `cea6261f` 保存在 `archive/release/v1.22.0-candidate` 并移除活跃远端分支；`git ls-remote` 已确认归档 ref 同 SHA，活跃 ref 不存在。远端旧 `feature/login-wallpaper-20260925` 与 `fix/monitoring-cluster-zoom-delivery-20260926` 活跃 ref 也不存在。
- 本地发布工作树和来源功能工作树尚保留作证据/回滚核对；不以远端归档冒充本地清理。未清理未知归属或未提交工作树。

## 外部审计与修复交付

- 已完成的 scoped `run-10` 覆盖 RC10 基线，记录一个 `needs_validation` 线索 `kpanel:rollback-restores-revoked-light-node`。本版写入冻结和回滚阶段修复位于 `b845606b`、`81f5d05b`、`dfad70a8`、`db4c4109`、`cea6261f`；源码、候选、主线和稳定标签均包含这些提交，生产未部署。该旧线索没有被独立动态复核成 confirmed 或 closed。
- 覆盖检查：`node scripts/check-security-audit-coverage.mjs --target HEAD --require` 返回 `decision=ok`，退出码 0；未审计边界提交 5 个、文件 6 个、最早 0 天、无新增边界包，符合 14 天窗口。此结论只说明覆盖检查准入，不声明新增提交已通过 CF 审计。
- 额外 scoped `run-15` 固定源码 `cea6261f` / tree `6ee3d7ef373de59bdef8ab887eca5ce67e29a1fb`；四组侦察完成，但三次有界 hunter 分派未给结构化结果，最终 `run_status=incomplete`、`scope_complete=false`。19 单元账本与 0 条 findings 的结构校验通过；16 个范围内单元均 deferred，post-wave critic、Phase 3、Phase 5 未运行。证据在上述根目录的 `security-audit-run-15/`，不得作为覆盖完成或无漏洞证明，也未导入 `.governance/security-audit/`。
- OCR：本稳定候选是 RC10 已审内容加更新恢复修复；源码差异审查、定向回归、L3 和 CI 均已执行。本轮没有新的有效自由臂/约束臂成对 OCR 记录，计数 `unreported`；不得把 CI 或 run-15 侦察当成 OCR/独立验证。试用退出条款仍按后续完整周期评估。

## 跨仓库联动判定

- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集编号、脚本候选和脚本先行发布均不适用。
- 受管 `kejilion.sh` 内置基线 `2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`；本版没有新增或修改该脚本的协议/内容，L3 的 managed script contract 通过。
- `kejilion/apps` 的 `main` 从独立到达的 BusyBox 兼容提交 `b956b84` 快进至 `5323b7edf473ecd6d6453c34a84ffc1e7ead4c4b`。本版同步配置相对镜像内置 `kpanel.conf` 只多应用市场 `app_url` 和先到的 BusyBox `flock -n` 兼容修复。该差异、后续镜像补齐需求已明示，不能声称两份脚本完全同哈希。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 固定 SHA 的 L3、候选/主线 CI、公开镜像 `image_e2e=pass`；本地公开镜像浏览器上传/经典/登录/场景流程 | 登记隔离环境真实 Agent 与生产实例未验收 |
| 网络入侵与供应链安全 | 已实现未实机验证 | run-10、强制覆盖 `decision=ok`、Trivy/govulncheck/npm audit、Release 运行时契约和 digest 核对 | 额外 run-15 未完成；本版新边界提交尚未形成完整 CF 审计覆盖 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | Go race、备份恢复与应用生命周期夹具、公开镜像容器 E2E | 真实宿主机崩溃恢复、BusyBox 镜像内置脚本兼容未验收 |
| 性能与资源预算 | 已实现未实机验证 | L3 最终镜像资源契约、三场景资源压缩/缓存实现 | 真实低端 GPU、长时 soak 未测 |
| 用户体验与可访问性 | 已实现未实机验证 | Chrome 1280×800 上传、经典/登录与场景静态海报截图，页面错误 0 | 125%/200% 缩放、手机和 Safari 矩阵未完整验收 |
| 数据、配置与迁移 | 已实现未实机验证 | 应用配置备份恢复夹具，版本/配置一致性检查；无 schema 迁移 | 图片和场景包迁移需重新上传/下载，生产备份未执行 |

## 自动门禁

- 本地定向：版本一致性、业务事实新鲜度、治理一致性、工作树状态、`git diff --check`、Bash 语法及应用配置备份恢复夹具通过。
- L3：唯一入口 `scripts/run-release-l3.mjs`，`v1.22.0-cea6261f-l3-r1`，目标 `local-wsl-dr`，2026-09-26T11:48:43Z 至 11:55:48Z，`status=passed`、`exit_code=0`，固定 Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。bundle SHA-256 `7dbe3ea549063d8647fe563825e647a8a02b46e9436ccc24a6f29da23b73e02b`；plan `d0f2388c49cdbb020bee90e5126a6f6da507d3f4fa758a0c08a05677cef8982f`；远端脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- L3 包含 Go 全套与 race、前端 1712 项、Trivy 源码及最终镜像扫描、govulncheck、最终镜像构建与应用更新备份生命周期；`release_l3_gate=pass`、`release_l3_remote=pass`。
- 候选 CI [36240407580](https://github.com/kejilion/KPanel/actions/runs/36240407580) success、Dependency freshness 36240407616 success；主线 CI [36245467281](https://github.com/kejilion/KPanel/actions/runs/36245467281) success、Dependency freshness 36245467228 success；精确源码 SHA 均为 `cea6261f`。
- Release workflow [36246049132](https://github.com/kejilion/KPanel/actions/runs/36246049132) success、标签 Dependency freshness 36246049133 success；源码验证、运行时契约、原生镜像扫描、双架构构建/推送、通道提升与候选归档均通过。正式镜像带 SBOM/provenance attestation。

## 依赖与技术栈

- 没有新的 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本候选；`web` 锁文件的版本元数据升至 `1.22.0`。候选、主线和标签的 Dependency freshness 均成功。
- 本轮未单独生成新的 `make dependency-report` 报告；沿用发布门禁中实际执行的漏洞与依赖检查，不把“未运行”写成报告完成。下一次依赖治理按每日审计与 EOL 复核执行。

## 隔离真机与浏览器验收

- `arena-154` SSH 超时，未取得登记隔离环境的公开镜像浏览器/宿主机验收。用户明确选择本地通道；`local-wsl-dr` 仅登记为 candidate-validation，本地公开镜像和浏览器结果作为补充证据，不冒充登记的 browser-validation 或 production-safety-check。
- WSL Ubuntu/root Docker 显式拉取公开 `docker.io/kjlion/kejilion-panel:1.22.0`，摘要 `sha256:46b854bdf6233e81742986a3bee9e65c6b4a39123c393a51a4ada63783eed7ac`；`KPANEL_EXPECTED_VERSION=1.22.0 sh packaging/tests/image-e2e.sh` 输出 `image_e2e=pass`。
- Chrome 1280×800、本地隔离公开镜像容器：上传 2560×1440 自定义 WebP，预览自然尺寸非零、保存 1 项；经典模式“通透”引用该图片，退出登录后登录页保持所选图与本地高细节副本，页面错误 0。证据为 `public-stable-browser/upload-ready.png`、`classic.png`、`login.png` 与 `journey.cjs` 输出。
- 官方 `orbital-station` 场景包逐文件按 catalog SHA256 校验后播种到该隔离容器，因为外部包下载不可达；应用后经典模式与登录页均显示静态海报，本地副本长度 45764 字符，页面错误 0。证据为 `scene-classic.png`、`scene-login.png` 和 `seed-scene.py`。这验证公开 Panel 镜像的已安装场景旅程，不验证线上场景包下载链路或实时 3D GPU 性能。
- 未做生产写入、生产备份、真实宿主机故障注入、125%/200% 全视口/语言矩阵和长时 soak；上述环境边界与未执行项不冒充通过。

## 发布产物与公开仓库复核

- [GitHub Release v1.22.0](https://github.com/kejilion/KPanel/releases/tag/v1.22.0)：draft=false、prerelease=false、GitHub Latest=v1.22.0；公开时间 2026-09-26T13:54:05Z，14 个附件。
- Docker `1.22.0` 与 `latest` 的 OCI index 同为 `sha256:46b854bdf6233e81742986a3bee9e65c6b4a39123c393a51a4ada63783eed7ac`；`linux/amd64` 为 `sha256:6ebcaca87baa63f8818dc4e162d91674f877ba7681f75d9e309a428dffd8540b`，`linux/arm64` 为 `sha256:571637d37289e56c158bd5d29341b94602ed2099eeddc14a185fa0cca55c403d`；另有 SBOM/provenance 的 `unknown/unknown` attestation 条目。
- Docker `preview` 仍为 RC10 index `sha256:0a00853548e05671ea404b9ae8e9e3bdb660ea8a1aa1a4451044cb2ac1742c8d`，未被稳定版提升覆盖。
- `SHA256SUMS` 资产本身 SHA-256 为 `651ea64718bf8cd3d18d9894a7c4bf44862111ed4bd376905f070c2aacb2c409`；文件内 11 项与 GitHub 附件 digest 全部匹配。公开镜像 `image_e2e=pass`；应用市场 `main=5323b7e`。

## 自更新通道验收

- 稳定来源使用已发布 GitHub Latest，预览来源只接受规范稳定版/RC 与唯一官方镜像 digest；候选选择与 digest 校验由源码、L3 和 CI 覆盖，公开通道摘要另已核对。
- 加入预览仅切来源并触发检查；自动安装开关与一次性立即安装独立；旧状态默认 `stable`、重启后保留通道；退出预览不会自动降级。以上是既有回归和源码契约，本轮未在生产或登记隔离真实 Agent 复演。
- systemd 更新前备份、失败恢复、失败版本隔离和轻量 Node 恢复由 L3 夹具覆盖；OpenRC/BusyBox 的镜像内置生命周期脚本仍有前述兼容缺口，不能标为实机通过。

## 生产部署安全核对与回滚

- 用户授权正式产物发布，未授权生产部署；本轮生产目标 `arena-154` 未连接。`prod-108` 禁用全部 KPanel 操作，未连接、未备份、未部署、未升级、未核对。生产写操作、部署前后版本/健康/备份证据均不适用。
- 源码与版本回滚点为 `v1.21.0`；上一稳定公开镜像 index `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。没有生产实例变更或生产数据备份可回滚；本轮也没有实际生产回滚。
- 公共默认更新入口当前为 GitHub Latest `v1.22.0`、Docker `latest` `sha256:46b854…7ac`、应用市场稳定入口 `latest`。生产未部署，不把这些公共产物状态写成用户实例已升级。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-26T15:37:41+08:00
- 候选冻结时间：2026-09-26T19:48:43+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（本稳定标签首次发布；RC10 预览修复另见其验收）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记隔离环境 SSH 超时；候选 L3 改走登记灾备，公开镜像浏览器验收只能用用户选定的本地通道作为补充。",
    "recoveryEvidence": "local-wsl-dr 固定 SHA L3 status=passed；公开镜像本地 image_e2e=pass、浏览器旅程通过，但登记隔离环境项目仍明确未验证。",
    "permanentAction": "负责人为 1.22.x 发布任务；2026-10-03 前复核 arena-154 SSH 与 browser-validation，退出条件为在登记隔离环境完成同一公开镜像旅程。",
    "historicalReleases": []
  },
  {
    "fingerprint": "appmarket-fixture/staging/crlf-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次组合夹具使用 Windows 工作文件副本，CRLF 导致容器内 Bash 在第 13 行拒绝执行；该次测试无效。",
    "recoveryEvidence": "仅在仓库外的只读挂载测试副本规范为 LF，随后相同组合配置的生命周期夹具 exit=0；仓库提交和远端文件未被改写。",
    "permanentAction": "负责人为 1.22.x 发布任务；2026-10-03 前让组合夹具从 Git blob 导出 LF 文本并预检换行，退出条件为一次运行无 CRLF 拒绝。",
    "historicalReleases": []
  },
  {
    "fingerprint": "appmarket-fixture/container/capability-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "自建组合夹具容器先误加 cap-drop ALL，夹具需模拟 chown/chmod 与受限目录写入，前两次因容器能力不足无效。",
    "recoveryEvidence": "恢复一次性容器的默认 capability、仍保持无网络和只读源码挂载后，应用生命周期与备份恢复夹具 exit=0。",
    "permanentAction": "负责人为 1.22.x 发布任务；2026-10-03 前把夹具所需容器 capability 写入固定入口并预检，退出条件为同一组合夹具首轮完成。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/local-wsl/container-exit",
    "position": "before-production-write",
    "count": 1,
    "impact": "自定义壁纸旅程通过后，隔离测试容器正常码提前退出，首次场景测试连接被拒。",
    "recoveryEvidence": "保持 WSL 会话存活后重启同一已拉取公开镜像容器；场景经典模式/登录页旅程通过，随后测试容器、网络和临时数据清理完成。",
    "permanentAction": "负责人为 1.22.x 发布任务；2026-10-03 前在浏览器验收入口加入 WSL 存活检查及容器持续运行预检，退出条件为两条旅程连续运行不发生容器退出。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

run-15 三次 hunter 中断属于额外审计的未完成尝试，不计入使必需发布步骤失效的流程异常；其原始证据和未覆盖状态仍保留。Docker Hub 拉取时单层瞬时自动重试后命令成功，不计作失败发布步骤。

## 遗留风险与后续准入

- `arena-154` 恢复后补登记环境公开镜像真实浏览器/宿主机验收；本地通道结果不能替代它。生产部署需另获明确授权，完成正式备份、部署和安全核对。
- 对本版新增的更新边界完成后续独立 CF scoped 审计；run-15 incomplete 不算覆盖，run-10 轻量节点撤销线索需安全隔离环境的动态验证。
- 镜像内置 `kpanel.conf` 的 BusyBox flock 兼容修复应在后续补丁版同步，并跑 Alpine/OpenRC 真实环境；应用市场已先保留该修复。
- 本地工作树与仓库外证据暂保留作回滚与审计复核；结束前按所有权和可恢复性盘点，不清理其他会话的工作树。
