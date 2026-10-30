# KPanel v1.28.1-rc.1 预览版发布验收记录

日期：2026-10-31（Asia/Shanghai）；发布级别：L3。

releaseChannel=preview；releaseTrain=1.28.1。

候选提交 / 标签：`5e6b5b49edc4f879e424a24762764ab7d9d2fcec` / `v1.28.1-rc.1`；产品 tree：`4ca74f28f96951aaf04702dc6d09c7e36b95c2bc`；annotated tag object：`f72890e81de668e4fb0adc5281af6639fcedbf13`。

**预览产物已公开，公开验收经只读恢复通过；生产环境未部署。** [Release](https://github.com/kejilion/KPanel/releases/tag/v1.28.1-rc.1) ID=429348392，于2026-10-30T18:18:38Z公开，draft=false、prerelease=true。

原[发布任务41099129500](https://github.com/kejilion/KPanel/actions/runs/41099129500) attempt 1完成候选CI、L3、main/tag、主线CI、Release及镜像发布，但公开浏览器的版本信息请求返回502，故原任务整体为failure，preview-completion与stable-completion均skipped。随后[只读恢复41106673910](https://github.com/kejilion/KPanel/actions/runs/41106673910) attempt 1对原产品通过公开完整性和全部浏览器旅程；没有重建、覆盖镜像或重发版本，不把原流水线改记为全绿。

## 发布画像与范围

- 基线：`dae2d25f5e9b278f3b4534c878608d0a49146df1`；业务域：网站环境防护、后台任务提示；变更面：受管脚本固定来源、前端展示与交互。
- 用户更新：修复WAF首次启用下载configure.py的404；防护已安装时隐藏重复安装/修复按钮并保留现有开关、卸载；任务提示可关闭，关闭不取消后台任务，新任务仍显示。
- 集成映射：`6acde819 -> 86825000`，`f1b42e45 -> a8fa4860`，`4ef4baf8 -> b521e9e5`；另有聚焦DOM回归、版本准备和派生扫描身份资格化。最终11个差异文件，135行增加、19行删除。
- 不新增API、权限、数据schema、端口、Compose或宿主机业务状态；不声明性能提升。应用市场默认入口不变。
- FastAdmin `codex/fastadmin-php74@b7395dbac86b38d43d03fc3ca81788a8d30fb582` 未纳入：所需脚本入口未公开，继续保留本地实验候选；本轮未发布sh/nginx的新提交。

## 候选与来源处置

| 原分支 / 原tip | 纳入与处置 | 恢复引用 |
| --- | --- | --- |
| release/v1.28.1-candidate / 5e6b5b49edc4f879e424a24762764ab7d9d2fcec | RC列车保留，不按稳定版归档 | 同名远端候选及不可变v1.28.1-rc.1 tag |
| fix/waf-managed-script-revision / 92fefd39a74dd8fdc76a27eed30f665457e21b69 | 功能经86825000纳入；原tip保存为归档，Dockerfile与冻结产品逐字相同 | archive/fix/waf-managed-script-revision / 同一原tip |
| fix/hide-protection-repair / 4ef4baf8d4a33a483930f0c37b7e1fcd95d592da | 两个功能提交经a8fa4860、b521e9e5纳入；EnvironmentView.vue与冻结产品逐字相同 | archive/fix/hide-protection-repair / 同一原tip |

归档通过仅创建新引用的原子SSH推送和空目标lease保护，并已回读精确SHA；原来源分支此前没有活跃远端引用。本地来源工作树与分支保持原样、clean，保留用于追溯，不再作为待发布功能；未删除未知归属内容。FastAdmin保留待后续。产品候选工作树为C:/GitHub/_release/kpanel-v1281，无upstream；本文在独立docs候选集成，本文自身的CI和归档结果由最终交付与远端引用核对，不改变产品tag。

## 跨仓库联动与审查

- scriptLinkageState=coupled；变更集waf-pin-correction-20261031。脚本已先公开：kejilion/sh@6f6d8445b3e0884dc96035879b8d76ff97a175b3，根/CN各1181864字节，SHA-256均为fe9625222ac3ffa300de131d4f8a9da212a3ca125054ae4a0d658f69c70508b5。
- GitHub主线557f571d相对该脚本提交ahead=10、behind=0；本轮独立读取Git blob并核对根/CN原始字节。共享WAF包42698292499f6e6ba4ebe4f833e164805e6b5641及七文件摘要不变，只修正旧脚本中的错误提交号。复核原来源七文件下载、12项bootstrap和隔离安装安全证据；真实主机启用/迁移未验证。
- Dockerfile派生身份6e56611f3f5a93d38efffa78f91313891bb766c4602bfff5067b8baecaf09c87；1255个Go源文件的firstPartySourceSha256为ffac3999fa0be71b4349a25d3ca3d357352693a9336442907ae7237ccab27ad8。基线库存通过仅替换版本常量的旧hash重现；依赖图、实际生产GoFiles、闭包、扫描发现及原到期日2026-11-08不变。
- security-review-scope输出not-applicable，groups=0；没有敏感包产品逻辑差异，未执行安全盲审或CF审计，不声称全仓逻辑安全。独立接手复核采用Codex干净任务；Claude/Gemini非交互CLI不可用，fallback=provider-unavailable，原提交保留trailer。
- OCR 1.12.11覆盖7/7可审文件，初始自由臂有效发现0；最终派生身份和测试夹具修正晚于首次圈选，故constrained-only=unreported，不计工具新增收益。测试、文档、锁文件处置及原始阶段证据保留，不改写已推送SHA。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 固定来源、契约与组合CI/L3通过；真实宿主WAF启用、迁移、Cloudflare联动未实演。 |
| 网络入侵与供应链安全 | 已验证 | 精确候选扫描、来源摘要、双架构OCI和19附件校验通过；不等于全仓安全认证。 |
| 稳定性、失败恢复与兼容 | 已验证 | 真实容器重启、证书故障恢复和公开升级旅程通过；首轮版本信息502根因未完全定位并保留跟进。 |
| 性能与资源预算 | 已实现未实机验证 | 原有预算未扩大，隔离作业有限额和清理；未做低配真实主机峰值或长期soak。 |
| 用户体验与可访问性 | 已验证 | DOM回归、390px布局、主题切换、Tab/Shift-Tab和Enter关闭、3px可见焦点及公开更新弹窗通过；IAB截图失败、原生125%/200%缩放、读屏与实体触控未验证。 |
| 数据、配置与迁移 | 已验证 | 本轮无schema迁移；关闭提示是页面内状态，刷新恢复；升级及人工回退仍需配套数据备份。 |

## 自动核验与依赖

- 本地：治理366项通过；类型检查通过；环境页10项定向回归通过。新测试早期的路由mock和ApiList.total错误已修正，原始失败不当作产品成功证据。
- 候选push CI：[41097002026](https://github.com/kejilion/KPanel/actions/runs/41097002026)，同SHA成功；发布内candidate-ci/verify=124212762128、windows-node=124212761967成功；主线verify=124218761340、windows-node=124218761401成功。发布windows-node=124224550141成功，release=124225995656成功。
- L3：run41099129500/attempt1，job124212762072。回执status=passed，精确candidate/tree及三组日志摘要匹配；GitHub-hosted Ubuntu24 image20261026.354.1，Go1.27.3、Node24.22.1、Docker28.0.4。权威入口run-release-gate.sh SHA-256为43e57f4fd321846204020d6cf27eb75961f0aefa1284a9187e786601704c2d40。
- L3源码web/go/deploy耗时分别348031/474354/1936ms，并行总474382ms；Web 332文件、3348测试通过，6项按原规则skipped，生产构建成功。L3含镜像契约、扫描、场景与生命周期、image_e2e=pass；hosted L3不使用旧SSH/WSL执行方案或旧Runner ID。
- 依赖报告run41097002020，生成于2026-10-30T17:15:04.816Z，必需源9/9；347个版本通道信号，其中36个直接/基座行动项、311个传递归属信号，不代表全部应升级。CF可选源not-requested不计缺失。
- 本轮仅采用已公开受管脚本修订，其他版本/锁依赖不升级。TypeScript7及@types/node26沿用已有例外，负责人KPanel base maintenance，复核日2026-11-28；EOL最近复核2026-10-29、下次2027-01-29。其余候选按既有维护时限另行评估，检测不能重置期限。该push报告的security-advisories job按事件条件skipped，安全结论使用本次精确CI/L3扫描，不冒充每日作业已运行。

## 公开产物与真实浏览器

- 版本镜像及Docker preview：`sha256:dfb162a28988165a2a7854e01705a95b39723fb1ec0cfc9d3939447524fee54b`；linux/amd64=`sha256:5958ada91bc2fe5348d7e290eb6f69376d388dddaff91c510506a09de531bdd5`，linux/arm64=`sha256:bc26aa866d84ecd2b6eb2752d252ebdae1885b2d8106cb356f1804af91aecdea`。两架构source/version和内置脚本标签匹配；GitHub Latest和Docker latest仍为v1.28.0 / `sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`。
- 19附件、15项SHA256SUMS、GitHub API digest、下载/磁盘回读及modified-component provenance通过；公开正文与冻结原文SHA-256均为1743863f6e45329c017a7a96f6421c908f219615f211fe96b20084736ea3ba64。公开摘要4条、升级提示3条，面板接口cached=false、stale=false，更新弹窗实际显示一致。
- 原snapshot-41099129500-1保存发布前stable/previous-preview、source/version/run/attempt、19附件清单与正文身份。stable-before SHA-256=2c6a2f5994cd532b08b151aa079f94ec85ab17862601d17e9ec74adfe732d959；清单SHA-256=1a40f6ac2287cf516bb0ee2ebe794bb075bcd12b150ad9f271b609e5b36a25df。
- 原控制面dae2d25f；恢复控制面5e6b5b49。恢复严格读取原snapshot，产品SHA/tag/digest未变；Runner、夹具、浏览器脚本原/当前摘要各自相同，productImageUnchanged=true，没有测试修复或降低断言。恢复测试Runner实际ID=sha256:4348a0d5e65bc61ab014d2d8f999bfaccb31e911fb95b89de59ef9825b997485；原失败Runner ID=sha256:bf438986bf8cdabce5803c9bf704b943853832f36665c25242f6898700299a10，不要求不同构建的ID相等。
- 恢复run41106673910/attempt1/job124238212376成功：initial48、restart26、published45、upgrade31项，errors=0、assetFailures=0。真实Panel/Agent容器与浏览器使用明确Docker库存和测试CA夹具；不挂宿主Docker/systemd socket，不等于真实主机安装或生产验收。三份cleanup回执确认只移除自身容器/网络。
- 环境策略github-hosted；受硬超时、CPU/内存/PID限制约束。确定性交互与重启/升级旅程按完成断言验收，未做长时soak；OpenRC/systemd真实更新、物理ARM64、全发行版WAF矩阵仍未执行。
- 预览通道与自动安装继续分离，退出不会自动降级；本次公开API、弹窗及既有通道回归通过，不外推为真实宿主机自动安装、备份或回滚已完成。

## 生产与回滚

生产动作不适用（预览版禁止生产部署）。本次未连接、读取、备份、部署或升级prod-108，也未连接其他生产主机。GitHub/Docker公开产物不代表用户实例已升级，生产实际版本未核对。

回滚点v1.28.0及`sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`保留；上一preview为v1.28.0-rc.15 / `sha256:2db52a45d988c8376efe286ad157b266b567b6d3193f4bd5f89f296e34f2184d`。没有执行实际回退或创建生产备份；用户人工回退需使用对应镜像和升级前数据备份，退出预览不能代替回退。公共稳定默认保持不变。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-30T13:46:13+08:00
- 候选冻结时间：2026-10-31T01:13:49+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本轮只有一次实际版本发布，后续为只读验收恢复；原失败与恢复分别保留。主线作业曾等待GitHub runner，排队精确耗时未单独记录，不反推节省时间。以下包含本地准备、无效取证和监控异常，不能等同于生产变更失败；本版没有生产写操作。公开浏览器首败已在上文与明细中记录，根因尚未完全定位。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：20
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preparation/workflow/python-alias",
    "count": 1,
    "impact": "Windows python alias could not execute the workflow CLI.",
    "recoveryEvidence": "Workflow list/render succeeded with the verified Python313 absolute path; rendered-workflow-frozen.md retained.",
    "permanentAction": "Use the confirmed installed interpreter for this task; keep workflow discovery checks. No repository mechanism change claimed.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/task-preflight/wrong-working-directory",
    "count": 1,
    "impact": "Preflight was first invoked from web/ with a repository-root relative script path.",
    "recoveryEvidence": "Original tool error retained in the task; root-level ready and final handoff passed for 5e6b5b49.",
    "permanentAction": "Run the existing preflight from its declared repository root; no alternate gate or permission path.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/task-preflight/unsupported-tool-label",
    "count": 1,
    "impact": "The contract listed gh outside the preflight tool-name allowlist.",
    "recoveryEvidence": "Removed unsupported label without changing actual release permissions; ready and handoff passed.",
    "permanentAction": "Keep the canonical allowlist; gh authentication and dispatch remain separately verified.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preparation/source-download/connection-reset",
    "count": 1,
    "impact": "Direct Node fetch of immutable raw script bytes failed with ECONNRESET.",
    "recoveryEvidence": "Verified both immutable Git blobs through GitHub API; byte counts and expected SHA-256 matched in source-qualification-final.json.",
    "permanentAction": "Use the existing authenticated GitHub byte source for this task; preserve checksum rejection. No network reliability claim.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "qualification/github-contents/large-file-empty-content",
    "count": 1,
    "impact": "The contents API returned encoding=none for a file above 1 MiB; the preparation script rejected empty decoded bytes.",
    "recoveryEvidence": "Read the exact git blob identity instead; both 1181864-byte scripts matched fe962522... and current public FastAdmin absence was rechecked from nonempty bytes.",
    "permanentAction": "Require explicit nonempty base64 bytes and exact checksum in the task preparation helper; do not treat empty content as absence.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "qualification/scan-inventory/incomplete-version-binding",
    "count": 1,
    "impact": "The first local qualification note covered Docker changes but omitted the Go default-version literal; it was superseded before CI.",
    "recoveryEvidence": "qualify-version.mjs reproduced the baseline 1255-file inventory by substituting only the prior version-file hash; final source and Docker identities passed L3.",
    "permanentAction": "Bind both actual first-party inventory and build target hashes; retain all findings, original observation dates and expiry.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "qualification/review-trailers/non-terminal-trailer-block",
    "count": 1,
    "impact": "Separated commit-message paragraphs hid the new OCR trailer from Git trailer parsing, producing a stale advisory.",
    "recoveryEvidence": "Added one terminal trailer block; final candidate-checkpoint.log reports ocr_line_review=recorded. Prior commits retained.",
    "permanentAction": "Use a file containing a single terminal trailer block for this task; keep canonical metadata parsing.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/ui-test/missing-router-mock",
    "count": 2,
    "impact": "New DOM tests first failed while loading missing router mocks, then while resolving an unstubbed RouterLink.",
    "recoveryEvidence": "The focused fixture stubs were corrected; interaction-tests.log has 10 passed tests and typecheck-r2.log passed.",
    "permanentAction": "Keep the new focused DOM regressions and explicit component/router stubs; no production code or assertion weakened.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/ui-test/missing-list-total",
    "count": 1,
    "impact": "Type checking rejected new API-list fixtures without the required total field.",
    "recoveryEvidence": "5e6b5b49 adds the actual ApiList contract fields; final type checking, 10 focused tests and candidate CI passed.",
    "permanentAction": "Retain typed API mocks and the standard type checker; no cast or skip introduced.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/iab-screenshot/capture-timeout",
    "count": 2,
    "impact": "IAB screenshot capture timed out; no screenshot evidence was accepted as success.",
    "recoveryEvidence": "preview-observations.json records DOM interaction and 390px geometry only; screenshot and native zoom limitations remain explicit.",
    "permanentAction": "Release maintainer to review the screenshot channel by 2026-11-07; repeated capture failure triggers channel repair. Public browser artifacts remain separately required.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preparation/release-metrics/powershell-parser-error",
    "count": 1,
    "impact": "A local read-only history-extraction command had an invalid foreach pipeline.",
    "recoveryEvidence": "The corrected expression read the preceding five stable acceptance records without modifying them.",
    "permanentAction": "Use a concrete result variable before formatting in this task; no historical record rewrite.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "monitor/github-api/http-502",
    "count": 4,
    "impact": "Two gh watch processes and two aggregated/attempt-specific jobs reads encountered HTTP 502; publication was not restarted.",
    "recoveryEvidence": "publication-watch.log and publication-watch-r2.log retain failures; light run/artifact APIs, exact-attempt reads, immutable L3 receipts and SSH main/tag readback restored progress evidence. Smaller standard pagination and direct known-job endpoints recovered current main/publication stages; main CI passed.",
    "permanentAction": "Stop repeated failing jobs aggregation; use bounded reads of the same run/attempt and independently verify Git/artifacts. Release maintainer reviews by 2026-11-01; no extra publication retry or relaxed gate.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "monitor/github-connector/read-timeout",
    "count": 1,
    "impact": "One independent GitHub connector jobs read timed out during the same monitoring interruption.",
    "recoveryEvidence": "Original connector result retained in the task; the exact same run/attempt continued and produced the validated L3 artifact.",
    "permanentAction": "Do not infer workflow failure from connector timeout. Keep bounded canonical API/Git/artifact checks; maintainer reviews by 2026-11-01.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/release-info/non-200-response",
    "position": "before-production-write",
    "count": 1,
    "impact": "原发布 run 41099129500 的公开完整性检查已通过，但重启旅程在第18项后请求预览版本信息得到 Agent 层 HTTP 502，原流水线结论为 failure；未部署生产。",
    "recoveryEvidence": "public-acceptance-failed.log、原 public-41099129500-1 工件和 public-failure-classification.json 保留。当前匿名官方源返回200且身份有效后，只读恢复41106673910/1对原SHA/tag/digest通过；未重建或重推镜像。",
    "permanentAction": "具体上游错误正文未记录，根因仍未完全定位，不以恢复成功关闭。发布维护者于2026-11-07前复核；若同指纹再现，停止重复验收并先补有界响应诊断或修复入口，不降低200及版本/摘要断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/business-context/preview-as-stable-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收草稿将当前预览版写入仅允许稳定版的基线版本字段，业务背景检查拒绝该草稿。",
    "recoveryEvidence": "原工具返回missing a semantic 基线版本；保留产品基线SHA，字段改为最近稳定版v1.28.0，并重验权威入口。",
    "permanentAction": "沿用既有稳定版本元数据语义和检查器，不放宽正则或覆盖历史记录；预览版本事实写入对应发布小节。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

最近五个稳定验收记录已按指纹复核，未以缺失数据推断为零；本轮未触发需要在生产写前处置的已确认重复项，且无生产写入。未新增流程、工作流或治理提案；复用现有release-kpanel和只读恢复入口。

## 证据与剩余事项

证据根C:/GitHub/_release-evidence/kpanel-v1281-rc1，含原L3/源码日志、候选/主线日志、source-qualification-final.json、OCR双臂记录、类型/DOM测试、模拟预览manifest、原snapshot/publication/public失败工件、匿名源探测与public-recovery工件。身份核验文件为public-identity-verification.json，恢复结果另有不可覆盖的Actions工件。

本地模拟预览仅证明界面：390px scrollWidth380px，查看终端字号14px、关闭控件40x40px；Tab/Shift-Tab可达、3px焦点、Enter关闭、控制台无错误。原生缩放和截图风险如表所列。全部本人预览进程已停止；候选、来源及唯一证据为追溯/下一RC保留，未执行磁盘删除或宣称释放空间，未知归属工作树保持原样。

待跟进：版本信息502的原始上游原因；真实WAF/CF启用与迁移、低配长期资源及未覆盖可访问性。恢复成功不关闭这些风险。FastAdmin只有配套脚本与模板完成发布、固定摘要及双端验收后才能进入后续候选。
