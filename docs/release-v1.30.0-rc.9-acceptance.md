# KPanel v1.30.0-rc.9 发布验收记录

日期：2026-11-08

发布级别：L3

候选提交 / 标签：6249888f59d7ddf5cf67f9d5724b6e6c2766f714 / v1.30.0-rc.9

上一稳定版本 / 回滚点：v1.29.0 / Docker digest sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6

releaseChannel：preview

releaseTrain：1.30.0

候选分支与发布后处置：release/v1.30.0-candidate / 预览版保留

- 原分支 / 精确 tip / 处置分类：release/v1.30.0-candidate，6249888f59d7ddf5cf67f9d5724b6e6c2766f714；RC9 预览分支。
- 归档 ref 与 SHA / 远端复核结果：未归档；发布后 origin/main 与 origin/release/v1.30.0-candidate 均指向 6249888f59d7ddf5cf67f9d5724b6e6c2766f714，RC9 tag 指向同一提交。
- 本次来源任务分支：RC9 无新增产品代码。站点访问密码、监控检查单页、媒体整合、音乐当前曲目匹配及移动终端交互已在 RC8 基线；本记录下方列出本次复核的候选与处置。
- 本地分支 / upstream / worktree：保留于 C:\GitHub\_release-candidates\kpanel-v130；没有删除来源分支或工作树。
- 未完成归档项 / 责任人 / 下次复核触发条件：候选按 docs/project-management.md 10.2 保留；终止 1.30.0 预览列车时再复核是否归档。

预览产物已发布，生产未部署。

## 发布画像

- 业务域：Panel、Agent、站点管理、监控、音乐/影视媒体、终端、AI 文件读取。
- 变更面：RC8 到 RC9 为版本/发布说明/扫描身份元数据；没有新增产品代码、API 或数据迁移。
- 受影响用户旅程：同 RC8；RC9 本身不改变功能行为。
- 未变化契约：稳定更新通道、现有 API、Docker Compose、Agent 权限、kejilion.sh 配套版本和生产部署状态。
- 风险等级及理由：中；本版是预览版重建，仍需记录此前集成功能边界、双架构身份、公开升级和恢复证据。

## 发布范围与未纳入内容

- 用户可见更新：RC9 不新增功能；RC8 期间已审查功能继续可选预览，包括站点访问密码、历史监控单页/检查分组、音乐当前曲目在线匹配与可选歌词/封面旁车、电影资料与进度、已有终端交互和 AI 文件安全读取。
- 精确提交清单：RC8 基线 11c3c8a401809424e26377bc638d7290708fd9a0 到 RC9 提交 6249888f59d7ddf5cf67f9d5724b6e6c2766f714；9f81da31ead2525cf6ec5daa7619481771e1bdb3 准备版本，6249888f59d7ddf5cf67f9d5724b6e6c2766f714 只更新 torrent 扫描范围中的 first-party source identity。
- 明确未纳入的分支、文件或后续事项：
  - candidate/media-integrated-preview f7728b8d4de33a4ff9d9430b98453ae805062408：媒体整合已在 RC8。
  - fix/monitor-checks-single-page 264525fa5570ace657dac5e38491944b5bfe1fdf：监控单页已在 RC8。
  - candidate/music-online-match-latest 39de71c94a3b2bb0d8acd634294ec7553f0d6ddb：当前曲目在线匹配已在 RC8。
  - feature/site-access-password e52717b5b030d8c8d1d361d5b3420a0cb7c9e0b7：访问密码及配套脚本已在 RC8。
  - candidate/mobile-terminal-gestures 3d935d48ffa1e9ce80580dc803a28fee8157ee8a：已有交互已在 RC8；RC9 没有新增手机终端优化，未做实体手机验收。
  - feature/site-settings-main-20261106 bf95693698ceb29c68e8772c7eb54ecd89758bb0：旧实现会移除当前站点识别、证书未完成提示及折叠表单卸载等主线行为，未纳入。
  - feature/music-sidecar-checkpoints-20261107 1b03c3a41d9da6f675155e7c9208ed9b90d191e7：旧实现会回退队列能力，未纳入。
  - feature/music-sidecar-save 63213d7512f34b287745f1a64969020f9287a98f：L2 的 TestStableDockerBindPreventsUnprivilegedRedirection 失败，未纳入。
  - 音乐整库自动刮削/扫描仍未实现；本 RC 只有当前曲目匹配及可选旁车保存。

## 外部审计与修复交付

- 安全盲审补漏：RC9 没有产品代码差异，未对 RC8 已审源码重复盲审；这不代表 RC8 的独立审查范围扩大。
- node scripts/security-review-scope.mjs --base v1.29.0：RC9 L3 复核报告记录发布验收覆盖通过；未在本记录中重新声称历史 trailer 全部符合新格式。
- finding fingerprint / 修复 commit / 独立复核与回归证据：RC9 无新增产品 finding。首次候选 CI 的 source-scan baseline 失败见交付指标与流程异常；修复只更新本候选 source identity 绑定，没有改变扫描范围或依赖闭包。
- 修复交付状态：RC9 预览已公开；stable 未提升；生产未部署。
- OCR 观察：RC9 无产品代码差异；本次未重算 OCR 样本，不据此判断工具有效性。
- 本稳定周期观察：不适用；RC9 是预览版本。
- 候选分支的原始 tip、归档位置和远端核验：见上方候选处置；所有未合入分支及工作树均保留。

## 跨仓库联动判定

- scriptLinkageState：coupled
- 变更集编号：不适用（沿用 RC8 已发布的配套脚本）。
- KPanel 实际内置脚本基线 commit / SHA-256：1faee13f3faef164efbfa899e244fc19ad58aaff / 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。
- 脚本候选 commit / SHA-256：同上；RC9 amd64 与 arm64 镜像标签及公开快照一致。
- 状态判定依据与兼容性证据：L3 与公开产物身份均校验脚本 revision 和摘要；访问密码功能依赖此配套脚本。
- 本版发布决定：不发布新脚本；RC9 沿用 RC8 已发布版本。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | RC9 精确候选 CI、L3、公开真实镜像浏览器和隔离验收通过；RC9 无业务代码差异 | 外部音乐服务匹配质量未验证 |
| 网络入侵与供应链安全 | 已验证 | 精确候选 L3 扫描、双架构镜像身份、脚本摘要、SBOM/provenance 和发布附件校验通过 | RC9 未单独执行产品代码盲审；没有新增产品代码 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 公开初始/重启及 upgrade/published 四份验收通过；首次 Windows ConPTY 候选测试超时后，同一候选后续通过 | ConPTY 首次超时根因未确认；无实体生产主机或长期 soak |
| 性能与资源预算 | 已实现未实机验证 | L3 和受限隔离容器验收通过 | 本 RC 未执行独立长时性能/容量测试 |
| 用户体验与可访问性 | 已实现未实机验证 | published 45 项、upgrade 31 项通过；升级弹窗 320/390/1280px 无横向溢出 | 未测试实体手机，也未覆盖手机终端完整交互矩阵 |
| 数据、配置与迁移 | 已验证 | 隔离初始 48 项、重启 26 项通过；偏好/会话/终端历史重启后可读 | RC9 无生产迁移；没有生产数据操作 |

## 自动门禁

- 定向测试及结果：正式 preview run 42395500461 中候选 CI run 42394719103 成功；候选 Windows job 128471929430 与 verify job 128471929432 均通过。
- make verify-release 环境和结果：GitHub-hosted Linux，Go 1.27.3、Node 24.22.1、Docker 28.0.4；精确候选 L3 通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：preview run 42395500461 / job 128471929390 成功；runner ID 1000005187，ubuntu24 image version 20261101.360.1；receipt entry SHA-256 181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3。receipt 未单列 plan/script/bundle 三个摘要。证据：C:\GitHub\_release-evidence\kpanel-v130-rc9-release-42395500461\l3。
- 候选 CI：最终精确候选 run 42394719103 通过；正式 preview 流程重复验证同一候选。
- 主线 CI：preview run 的 main-ci/verify job 128476142276 与 main-ci/windows-node job 128476142311 成功。
- Release workflow：run 42395500461，Release job 128480061842 成功，公开验收 job 128485796470 成功；preview completion job 128486276378 成功。
- 发布说明门禁：最终公开正文解析通过，2 条版本摘要、1 条升级提示。
- 安全扫描、镜像契约、SBOM/provenance：精确 RC9 L3 与 Release 门禁通过；双架构镜像标签 revision、脚本摘要一致。

## 依赖与技术栈变化

- make dependency-report：RC8 到 RC9 未改依赖版本、Go module 清单或锁文件；正式 dependency freshness workflow 42392671746 通过。RC9 L3 对最终 SHA 再次核验。
- 最近每日安全通告审计、EOL 复核状态及证据：随精确候选 L3/Release 安全与供应链扫描通过；未在本记录复制每日审计全文。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的期限：本 RC 未升级依赖；依赖治理结果以 L3 artifact 为准。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无依赖版本升级；构建工具由固定 release workflow 执行。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：镜像及脚本身份见公开产物章节；依赖锁文件未改变。
- 暂缓或拒绝候选：无新增依赖候选；产品候选范围见上方。
- 升级后的兼容、安全、构建、性能资源和回滚结论：双架构构建、扫描和镜像运行通过；稳定回滚点未改变。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted ubuntu24 Runner；RC9 镜像提供 linux/amd64 与 linux/arm64；隔离应用验收在 Linux 容器执行。
- 环境策略 ID 与允许用途：仓库 environment-policy.json 的 GitHub-hosted candidate/browser validation；未连接生产主机。
- 使用的精确候选或公开产物：6249888f59d7ddf5cf67f9d5724b6e6c2766f714；RC9 OCI index sha256:0f789b90291d8bd1eb35298eb4351e325ee1d0c2a9b479e3c46c0722a9ae5c93。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：published job 128485796470 成功；published 45 项、upgrade 31 项、isolated initial 48 项、restart 26 项，合计 150 项；错误与 assetFailures 均为空。证据位于 C:\GitHub\_release-evidence\kpanel-v130-rc9-release-42395500461\public。
- 测试窗口/循环数及风险依据：一次完整公开发布和隔离重启验收；未做 soak。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：真实发布浏览器验证已列旅程；升级对话框在 320/390/1280px 视口无溢出并覆盖失败态；不声称完成手机终端/多缩放矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器覆盖 TLS 失败关闭、离线恢复、初始/重启和更新失败路径；生产宿主机未写入。
- 未执行场景及原因：实体手机、真实音乐外部上游、生产升级和长时间 soak 未执行；此版本为 opt-in preview。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：version 1.30.0-rc.9、候选 SHA 和 OCI index digest 一致；2 条摘要、1 条升级提示；published 与 upgrade 浏览器流程均通过。
- GitHub Release：isDraft=false、isPrerelease=true，publishedAt 2026-11-07T18:12:05Z；Latest 仍为 v1.29.0。https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.9
- Docker 版本与通道 OCI index：kjlion/kejilion-panel:1.30.0-rc.9 与 :preview 均为 sha256:0f789b90291d8bd1eb35298eb4351e325ee1d0c2a9b479e3c46c0722a9ae5c93；stable :latest 为 sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6。
- linux/amd64、linux/arm64 digest：amd64 sha256:7421094d16fbcc95434756b9fd5c5ca44e0bfdf73f83baaedae0f47c22f28efb；arm64 sha256:99a386eab1fefbeefe68dc44d290cb76d057880d326355ce8b38560db136d484。
- 附件及 SHA256SUMS：19 个附件，SHA256SUMS 包含 15 项；SHA256SUMS 自身 sha256:34e4bc2ec8aefa6c5ab4ce78de6148c4ef66eb6c0b13ee78fa8ec6c6d0a1af1c。
- 公开镜像 image_e2e=pass：四份浏览器/隔离验收报告通过，共 150 项，页面错误与资源失败均为 0。
- kejilion/apps / kejilion.sh 契约结论：RC9 未变更应用市场或脚本；受管脚本 revision/hash 与双架构镜像标签一致。

## 自更新通道验收

- stable 来源只选择正式 GitHub Latest，preview 来源只接受规范稳定版或 RC，并校验唯一官方镜像 digest：公开 published 与 upgrade 两套报告通过。
- 加入预览只切换来源并立即检查，没有自动安装：published 流程确认 opted-in 不触发安装。
- 自动安装开关与一次性立即安装相互独立：upgrade 31 项通过。
- 旧状态默认迁移到 stable，重启后通道选择保持：initial/restart 报告通过。
- 退出预览且稳定版较低时没有产生降级候选：published 报告通过。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：公开浏览器覆盖无 systemd 的失败关闭和状态清理；不等同于生产宿主机验收。
- OpenRC 与轻量 Node 的当前边界已按 docs/release-channels.md 明确呈现：是；未以本次发布扩大边界。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：GitHub-hosted 隔离环境；未使用 prod-108。
- 正式部署环境：不适用；未连接 arena-154。
- prod-108：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、备份、部署命令、部署后健康及公网入口：不适用，未进行生产操作。
- 生产已执行写操作：无。
- 仅在隔离环境执行的场景：L3、公开浏览器、双架构镜像、初始/重启/失败恢复验收。

## 回滚

- 源码/tag：稳定回滚点 v1.29.0；RC9 tag 保留，不改写。
- 镜像 digest：稳定回滚 sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6；RC9 preview sha256:0f789b90291d8bd1eb35298eb4351e325ee1d0c2a9b479e3c46c0722a9ae5c93。
- 数据/配置备份：没有生产数据变更，因此没有生产备份。
- 回滚步骤和回滚后复核：退出 preview 切回 stable；预览选择不会自动降级，需按更新流程处理已安装版本。
- 回滚后生产实际版本与健康状态：不适用，生产未部署。
- GitHub Latest、Docker latest 与标准更新入口：仍指向 v1.29.0 stable。
- 公共默认更新通道决策：保留 opt-in preview，不改变 stable 默认。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-08T00:41:58+08:00
- 候选冻结时间：2026-11-08T01:16:40+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（RC9 对外发布一次；候选 CI 首轮失败不属于产品变更失败）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

流程异常：首轮候选扫描基线在 RC9 version-literal 更新后仍绑定旧 first-party source digest，导致必需候选 CI 重试；没有进入 main 提升或公开步骤。该指纹也见于 RC5 的预览记录；RC 复发由预览指标观察，historicalReleases 字段只接受 stable 标签，因此不填 RC tag。RC9 的修正只为精确候选更新摘要，不视为通用预防修复。稳定版下一次 L3 生产写入前，Release Maintainer 必须补齐自动 source-identity 预检，并用旧摘要夹具证明推送前失败关闭。

Windows ConPTY 测试 TestWindowsConPTYUnicodeResizeAndNaturalExit 首轮等待 EOF 超时，后续精确候选 CI 通过；原因尚未查明，保留为未关闭的偶发测试风险，不宣称已修复。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/scan-scope/stale-source-digest",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI run 42392671765 的必需扫描因 RC9 version-literal 变更后的 first-party source digest 未同步而失败；main 未提升，公开产物未生成。",
    "recoveryEvidence": "候选提交 6249888f59d7ddf5cf67f9d5724b6e6c2766f714 在 scripts/torrent-backport-scan-scope.json 更新 source identity；候选 CI run 42394719103、正式 L3 job 128471929390、Release 及公开验收随后成功。source identity receipt 位于 C:/GitHub/_release-evidence/kpanel-v130-rc9-source-identity-rebind-20261108/identity-rebind.json。",
    "permanentAction": "负责人：Release Maintainer。下一次稳定版 L3 生产写入前，在唯一候选预检入口自动从精确候选计算并核对 first-party source identity；增加旧摘要夹具，验收条件为推送前失败关闭且不再需要人工回绑。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：未删除候选分支、工作树或证据；未执行磁盘清理。
- 未验证风险：首次 Windows ConPTY EOF timeout 原因未知；真实外部音乐服务、实体手机/移动终端、生产安装和长期 soak 未验证。
- 已实现待实机准入：AI 文件投影在可证明稳定读取时返回；活动写入或不支持的文件系统可能 fail-closed，AI 助手其他功能不因此整体关闭。
- 不阻断本版的理由：RC9 没有新增产品代码；候选 CI、主线 CI、精确 L3、双架构 Release、公开验收均通过；稳定默认与生产未变化。
- 后续应进入的自动门禁或专项工作流：稳定发布前先完成 source-identity 自动预检和旧摘要故障夹具；复核 ConPTY 超时根因后决定是否调整测试。



