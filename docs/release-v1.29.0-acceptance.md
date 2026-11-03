# KPanel v1.29.0 发布验收记录

日期：2026-11-04

发布级别：L3。**产物已发布，公开验收已验证；生产未部署。**

候选提交 / 标签：`174fe03a2dc5db2c69351487936df15c9f07736c` / `v1.29.0`，annotated tag 对象 `74983e1812a419160f448ae6f515a9c86601c442`。

`releaseChannel`：`stable`；`releaseTrain`：`1.29.0`。

上一稳定版本 / 回滚点：`v1.28.0` / `sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`。

## 发布画像与范围

- 本版汇总自 v1.28.0 的变更，含 FastAdmin PHP 7.4 模板、HTTPS/DNS 和域名失败反馈、宿主共享脚本安装与恢复、AI 历史与 SQLite 并发、跨节点凭据与文件中继、双栈规则、挂载过滤和 Windows Node 回滚。
- 最后预览 [RC.7](release-v1.29.0-rc.7-acceptance.md) 产品 `2cf32fead7efe0e484de861374410a60fe044285`，主线准入基线 `213ae78f7fe614af8b36993e1ff21ca36822d042`。稳定版产品代码只移除版本后缀；其余为发布说明、源码身份及验证入口。稳定版重新构建和验证正式产物。
- 源码身份前置检查 `48f5f439`、`e311b1b2`、`ff273931`；原生升级验证 `3c498045`、独立复核 `ce63bbde`；最终冻结 `174fe03a`。无新增端口、Compose 结构或 Agent 权限。文件中继的新签名能力需双端升级；AI 容量统计表与触发器随既有迁移执行。
- `fix/audit-remediation-20261103@931ad9f0` 和 `fix/shared-host-script-maintenance@67518fd4` 均在稳定标签可达。RC.7 的独立复核及后续三项修复已包含。保留有未提交内容的跨主机备份导入工作树，未纳入。范围未扩展到生产部署或上游脚本发布。

## 外部审计与修复交付

从 v1.28.0 到稳定候选运行 `security-review-scope`：13 个包组 recorded，missing=0、stale=0。稳定前补漏没有待审组，未重复做新审计，不将记录存在当成全仓无漏洞。原审计候选 H0/M7/L1、confirmed=4、fixed=3、deferred=1；接管复核三项确认问题已修复并随 RC.7 和稳定版公开。详情、回归及原失败保留见 [RC.7 验收](release-v1.29.0-rc.7-acceptance.md)。本版准入工具独立复核无阻断；同 provider 干净会话 fallback 已记录。未运行 CF。

OCR 区间 v1.28.0..174fe03a：53 个非合并提交的 53 个 trailer，20 条可读计数、16 条 skipped、17 条 constrained-only 未报告。已报告的 constrained-only 均为 0；这是 trailer 观察，含同一差异的重复记录，不是独立适用候选计数，也不用于判定工具成效。准入工具 4/4 可审文件、其余两个文档人工复核，free-form 与 constrained-only 均为 0。稳定版版本元数据不重复 OCR。

## 跨仓库联动判定

`scriptLinkageState=not-required`（无需发布脚本（不适用））。镜像仍固定 `kejilion/sh@d2156444add457973d4a84634653aa10cece712f`，SHA-256 `f60a7023ca0d6dd9ee5f3557669675b84ca7e2c0f3de9fe4b8da22e740478235`。两架构公开镜像标签一致；官方当前脚本仅在可选刷新时校验、暂存、保留偏好，失败回退到内置脚本。运行时可选来源不改变镜像的固定来源。无新脚本协议、未推送脚本仓库；变更集编号、脚本候选和阻断依赖不适用。

## 多维质量结论

| 维度 | 状态 | 已验证证据 | 未验证边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 完整 CI/L3、真实隔离 Panel/Agent、正式镜像升级流程 | 生产宿主业务、真实双栈流量未执行 |
| 网络入侵与供应链安全 | 已验证 | 候选盲审、无待补漏组、源码/依赖/镜像扫描、双架构与公开附件摘要 | 低风险缓存容量待办保留，不宣称零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | RC.7 真实原生 systemd、冷恢复、Windows SCM 和正式镜像/浏览器恢复 | 原生 RC.7 与稳定实现相同，未在生产安装；旧入口首次升级限制保留 |
| 性能与资源预算 | 已验证 | AI/中继配额、并发、有限 spool 与采集回归 | 未测低配长期压力、真实阻塞 FUSE、物理 ARM64；无性能提升声明 |
| 用户体验与可访问性 | 已验证 | 真实浏览器 156 项，含更新弹窗、PTY、HTTPS 恢复和通道选择 | 无新增界面实现；未重做全语言、读屏、实体触控或 200% 专项 |
| 数据、配置与迁移 | 已验证 | AI 统计/私密标记隔离、脚本偏好、测试数据留存、快照恢复 | 无生产数据迁移；未执行完整生产备份恢复 |

## 自动门禁

- 候选 CI：[41662879219](https://github.com/kejilion/KPanel/actions/runs/41662879219)；精确主线 CI：[41666530091](https://github.com/kejilion/KPanel/actions/runs/41666530091)。两者均在 174fe03a 上通过 Linux、核心并发、漏洞/供应链与 Windows Node。
- 准入工具 CI：[41662241766](https://github.com/kejilion/KPanel/actions/runs/41662241766)，ce63bbde；本地 source-identity 用例 13 项和只读预检通过。稳定候选 first-party Go 源码 1262 个文件，SHA-256 `a7d370326b74c192bdb8f7541084e54246ca07e250115ea468bea759948202be`；仅版本 Go 文件变化，六目标闭包与扫描发现/期限不变。
- 稳定 tag push [Release 41669347042 / attempt 1](https://github.com/kejilion/KPanel/actions/runs/41669347042)：GitHub-hosted L3、原生 Windows Node、构建/扫描、公开验收和归档成功。L3 `2026-11-03T15:59:17.586Z` 至 `2026-11-03T16:17:43.728Z`，Runner `ubuntu24 20261026.354.1`，Go/Node/Docker `go version go1.27.3 linux/amd64 / v24.22.1 / 28.0.4`，入口 SHA-256 `181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3`；`make verify-release` 与镜像端到端检查均通过。Web 332 个文件、3362 项通过、6 项既有跳过。
- 草稿与实际发布说明验证通过：正式正文 7 条摘要、3 条升级提示，SHA-256 `01db7735ea54fa593875f409ad0802eae246d7f3b152a740832cfcc29b4e9311`。镜像契约、SBOM/provenance、改动组件清单与公开产物一致。原件：run 的 l3、snapshot、publication 与 public artifacts。

## 依赖与技术栈变化

同一候选的 [Dependency freshness 41662879182](https://github.com/kejilion/KPanel/actions/runs/41662879182) 于 2026-11-03T15:21:34.581Z 完成，9/9 必需源，40 个直接/基座行动项、318 个传递归属信号；它们属于升级评估输入，按依赖策略时限处理，不将候选信号等同于漏洞或本版升级。最近 [每日安全通告 41615769352](https://github.com/kejilion/KPanel/actions/runs/41615769352) 已通过；EOL 状态 current，最近复核 2026-10-29，下次截止 2027-01-29。稳定版相对 RC.7 未调整 Go 模块、工具链、Action、基础镜像或传递依赖；保留 v1.28.0 之后采用的 glob 14.0.2、minimatch 11.0.1 override 及固定内置脚本。没有为版本发布引入其他依赖升级。

## 隔离真机与浏览器验收

- 原生 systemd [41662241696 / 1](https://github.com/kejilion/KPanel/actions/runs/41662241696)：GitHub-hosted Ubuntu、真实 Docker、mocked=false，7 项检查通过，exit=0，清理通过。使用固定 v1.28.0 和 RC.7 公开摘要，验证正式服务单元、更新中断、冷备份/脚本恢复、成功提交、偏好和数据留存。只比较产品实现和验证路径，不把 RC.7 的原生运行身份写成稳定版。
- 正式公开镜像运行：initial 48 项、restart 26 项、published 47 项、upgrade 35 项，合计 156 项，mocked=false、浏览器错误为 0，镜像 `image_e2e=pass`。覆盖真实隔离 Panel/Agent、启动重启、证书失败后的访问恢复、真实终端、通道切换、更新提示与升级旅程。测试 Runner、浏览器、夹具绑定见 public artifact；视口、计算字号与缩放以各报告原件为准。无物理 ARM64、正式生产、全主机恢复或 soak 证据。

## 发布产物与公开仓库复核

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.29.0) ID `432956714`，`2026-11-03T16:40:07Z` 公开，draft=false / prerelease=false / GitHub Latest=v1.29.0；冻结正文、附件摘要和 Release ID 回读一致。
- Docker `1.29.0` 与 `latest` 同为 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`，amd64 `sha256:e26f8e9bd2a5381cf06b0bada2a13df43de997e4cb9824d0544c4a7b73e51e9f`，arm64 `sha256:9e4062781c0cd4486326478a6136336fd080554ba2529d38cc4192de7907f7ef`。`preview` 保持 RC.7 `sha256:1e936f24e2ebf65e827f14d814759e8b500fcbacf4c437bb19c4e1a559eb7696`。
- 19 个公开附件及 15 项 SHA256SUMS 校验通过。应用市场与 kejilion.sh 仍使用公开稳定来源、固定摘要和既有生命周期。面板接口及浏览器更新弹窗核对实际版本、摘要、说明与升级提示。

## 自更新、部署与回滚

稳定通道读取正式 Latest，预览通道识别合规 RC 与稳定版；加入预览只立即检查，自动安装开关与一次性安装独立，重启保持通道，退出预览不自动降级。systemd 更新前备份与失败恢复已在隔离环境验证；OpenRC 与轻量 Node 仍遵守 release-channels 现有边界。

生产部署不适用：仅授权发布产物。没有连接或操作生产目标（包括 prod-108）、没有生产备份/数据迁移/升级或公网健康核对。正式回滚保留 v1.28.0 标签和固定摘要，应通过现有应用市场回滚流程配套升级前数据、配置与脚本备份，复核 Panel/Agent 健康；本次未执行生产回滚。公开默认 GitHub Latest/Docker latest/稳定更新来源指向本版。

## 候选与归档处置

- 远端 `release/v1.29.0-candidate` 已由 stable-completion 原子归档到 `archive/release/v1.29.0-candidate`，回读 tip `174fe03a2dc5db2c69351487936df15c9f07736c`，原活跃引用不存在。稳定 tag 同指该提交。对应本地分支已同名归档并关联远端归档引用，工作树保留。
- `fix/v129-stable-admission` 原 tip `ce63bbde5156494879799191fa7712f1741bda99` 已包含在稳定标签。本地归档为 `archive/fix/v129-stable-admission`，工作树保留；精确远端原分支仅保留本轮准入与原生运行证据，不用于下轮选取。
- 审计整改、共享脚本维护及 RC.7 接管分支沿用 [RC.7 的精确归档](release-v1.29.0-rc.7-acceptance.md)。不处置其他任务的远端引用和有未提交内容的工作树。

保留本轮原始证据、精确提交和工作树；未删除未知或仍可复用内容，不报告净释放空间。不另建知识库，复用既有发布、验收和归档入口。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-31T01:05:16+08:00
- 候选冻结时间：2026-11-03T23:20:23+08:00
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

## 遗留风险与后续准入

- RC.7 已记录低风险缓存容量待办（KPanel 维护者，2026-12-03 复核）仍开放；保留 Windows 开发机 AI 分配量测试在原基线同样失败的对照，未提高阈值，正式 Linux 门禁已执行。
- PHP 7.4 维护风险、旧入口首次自动升级限制、真实双栈流量、阻塞挂载、低配长时和全生产数据恢复仍按既有准入。升级前备份、脚本路径权限与中继双端要求见发布说明。未将隔离证据外推到生产。
