# KPanel v1.29.0-rc.7 发布验收记录

日期：2026-11-03

发布级别：L3。**产物已发布，公开验收已验证；生产未部署。**

候选提交 / 标签：`2cf32fead7efe0e484de861374410a60fe044285` / `v1.29.0-rc.7`。

`releaseChannel`：`preview`；`releaseTrain`：`1.29.0`。

上一稳定版本 / 回滚点：`v1.28.0` / `sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`。

唯一列车 `release/v1.29.0-candidate` 保留。本记录提交属于发布后的验收元数据，不改写产品标签或镜像。

## 发布画像

- 业务域：宿主脚本安装更新、AI 历史、跨节点监控/文件中继、Docker 访问控制和系统采集；包含诊断与验证工具。
- 变更面：宿主写入、协议、SQLite 配额迁移、资源边界；无新页面、端口、Compose 结构或 Agent 权限扩张。
- 用户旅程：安装/更新时可选刷新官方脚本，失败使用已验证内置脚本并保留偏好；中断恢复；AI 历史容量限制；跨节点文件授权与监控脱敏；双栈访问规则。
- 文件中继能力标识升级，控制端与目标端须一起更新；旧端不支持正文签名时停止不兼容写入。AI 历史达到容量上限后需清理旧会话。

## 发布范围与未纳入内容

纳入 `fix/shared-host-script-maintenance@67518fd4` 和 `fix/audit-remediation-20261103@931ad9f0`；`fix/audit-remediation-20261103-l2` 为同 SHA 验证别名，非独立候选。备份跨主机导入工作树有未提交内容，未纳入、未覆盖。

发布接管另修复三项独立复核发现：合法第三项符号链接快照误拒绝、AI 普通迁移占用私密清理标记、慢速中继签名时间窗。`4f40f68e` 与 `2bdb1828` 完成修复；最终普通迁移使用未占用的 13，保留 11/12 的清理完成语义。原 FAIL 复核和首轮修复遗漏均保留。

验收工具先在主线资格化：`421756e9`、`31c18bf7`、`a7ead032`。通知重启测试复用同一假时钟，避免真实日期跨越 30 天留存期造成误报；未更改产品通知逻辑。发布前再次比较全部可信自动化路径，最终仅允许随源码派生的 scan-scope JSON 不同。

## 外部审计与修复交付

- 原候选安全盲审：raw H0/M7/L1，confirmed=4、fixed=3、deferred=1；剩余共享缓存容量低风险已登记负责人 KPanel 维护者和 2026-12-03 复核期限，不公开未修复细节、不宣称风险关闭。
- 发布接管独立复核：两名与实现分离的只读复核者，三项确认中级发现全部修复；最终两份结论 PASS WITH FOLLOW-UP，源码阻断 0。其他 provider CLI 不可用，采用记录明确的同 provider 干净会话 fallback。
- 最终候选 collaboration check：clean、nonempty candidate，security_review=recorded groups=10，independent_review=recorded。原实现者与本次发布者分离。
- OCR：原宿主脚本候选有 1/1 覆盖；审计候选因默认路径排除 Python 测试按政策停止，未冒充完整 OCR。接管修复分别完成 5/5 与 2/2 双臂覆盖，constrained-only=0；独立发现不记作 OCR 增量。未运行 CF 或稳定版补漏档，不改写历史审计。
- 证据：仓库外 `C:/GitHub/_release-evidence/v129-rc7` 的 audit-independent-review.json（原 FAIL）、audit-independent-review-final.json、platform-independent-review.json、repair-ocr-* 与原候选证据。

## 跨仓库联动判定

`scriptLinkageState=not-required`（无需发布脚本（不适用））：不要求新的脚本协议或脚本仓库发布，复用既有公开契约；没有修改镜像内固定脚本来源。

- 实际内置脚本：`kejilion/sh@d2156444add457973d4a84634653aa10cece712f`，SHA-256 `f60a7023ca0d6dd9ee5f3557669675b84ca7e2c0f3de9fe4b8da22e740478235`；两架构公开镜像标签均读回一致。
- 安装阶段可选官方当前来源另行记录：`9110407856ff3704e0519704ec8c977cc8405418`，SHA-256 `2f4ede044d50927fd983b897436dd2ab4c888923987ad9bde2a2818b030431c4`。真实下载后在隔离 rootfs 验证暂存、偏好、同步与回滚；未执行该脚本的全部宿主业务。
- 脚本候选发布、变更集编号和依赖移除：不适用；本轮未推送脚本仓库。

## 多维质量结论

| 维度 | 状态 | 已验证证据 | 未验证边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向回归、完整 L2/L3、真实隔离 Panel/Agent 和公开更新流程 | 完整生产宿主业务、实际双栈流量与所有脚本业务未实机覆盖 |
| 网络入侵与供应链安全 | 已验证 | 候选盲审/独立复核、Go/Node/源码与镜像扫描、19 附件和双架构身份 | 有期低风险待办保留；不宣称全仓零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | 真实文件/锁 rootfs、许可拒绝/EOF、可选下载失败、中断更新、撤权和慢 body 回归 | 断电持久性、长期运行和旧入口首次升级限制保留 |
| 性能与资源预算 | 已验证 | AI 总量、有限磁盘 spool/并发和重放配额回归；采集排除挂载类型测试 | 无性能提升声明、无 soak；真实阻塞挂载和低配长期压力未覆盖 |
| 用户体验与可访问性 | 已验证 | 真实浏览器 150 检查，版本/摘要/提示与真实 PTY/通道行为 | 无新增前端组件；全主题/语言、读屏、实体触控和 200% 专项未重做 |
| 数据、配置与迁移 | 已验证 | SQLite 标记隔离/配额、共享脚本偏好、快照与回滚恢复 | 无生产数据迁移；完整生产备份恢复未执行 |

纯后端、测试/构建与发布元数据范围，不另建本地页面 acceptance 预览；公开真实浏览器验收不等于生产主机验收。

## 自动门禁

- 最终 push CI：[41640795152](https://github.com/kejilion/KPanel/actions/runs/41640795152)，精确产品 SHA 2cf32fea，Linux 与 Windows 成功。
- [Publish preview 41643574495 attempt 1](https://github.com/kejilion/KPanel/actions/runs/41643574495)：inputs、候选 CI、完整 GitHub-hosted L3、main/tag 原子快进、最终主线 CI、Release、public-acceptance、preview-completion 全部成功；stable-completion 按通道跳过。
- L3 入口 `node scripts/run-release-l3.mjs --github-hosted`，2026-11-03T13:22:11.392Z 至 13:40:34.141Z；Ubuntu24 image 20261026.354.1，Go 1.27.3、Node 24.22.1、Docker 28.0.4；入口 SHA-256 `181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3`。
- source lanes web/go/deploy passed，identityUnchanged=true；Web 332 文件、3362 passed / 6 既有 skipped。镜像与更新备份恢复 image_e2e=pass、release_gate_runner=pass。
- 本地源码检查 21 项、AI 历史/私密与学习清理、中继定向回归、精确产品 rootfs 迁移通过；浏览器诊断 12 项和 benchmark 5 项通过。
- Windows 全 AI 包的附件内存分配用例失败，在原主线 33b13119 复现相同约 2.2 MB 分配；未提高阈值、未记为通过。产品 Linux 全包和 L3 已通过；Windows 产品节点验证另有真实成功回执。
- 原审计 L2 的陈旧源码指纹和本机 Git 配置失败未被改写；本轮六目标 actual/original 闭包重新资格化，模块/构建输入与扫描发现和期限未变，精确组合 hosted CI/L3 重新通过。

## 依赖与技术栈变化

Go 模块、工具链、基础镜像和 Action pin 未变。Web 根版本同步 RC7；开发传递依赖 override 固定 glob 14.0.2、minimatch 11.0.1，干净安装、构建、测试和依赖审计通过。本轮未重做完整每日公告与 EOL 独立周期报告，不把既有报告当作新执行结果。

## 隔离真机与浏览器验收

- 环境策略：github-hosted 的 candidate/browser-validation；本地 local-wsl-dr 的一次性无网络 rootfs。没有连接生产服务器。
- 固定产品 digest：`sha256:1e936f24e2ebf65e827f14d814759e8b500fcbacf4c437bb19c4e1a559eb7696`。公开 initial/restart/published/upgrade 分别 48/26/45/31，合计 150；四份报告 status=passed、mocked=false、errors=0，真实容器两份 assetFailures=0。
- Docker inventory、测试 CA 属于明确 fixture；真实 Panel/Agent、浏览器和 PTY 运行在隔离容器。不是物理 ARM64、生产账号或宿主完整防火墙实测。
- 无 soak（本轮只发布 preview，未授予生产暴露或持续压力测试任务）。容器 rootfs 清理完成，生产动作不适用。

## 发布产物与公开仓库复核

- [公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.29.0-rc.7)，ID 432800548，2026-11-03T14:28:28Z；draft=false、prerelease=true、非 Latest。
- 19 个附件已公开；本次 canonical verifier 实际校验 **15 项 SHA256SUMS**，不沿用旧摘要的 11 项数字；各附件名称、大小、API digest、下载和磁盘回读通过。
- OCI index / Docker preview：`sha256:1e936f24e2ebf65e827f14d814759e8b500fcbacf4c437bb19c4e1a559eb7696`。
- linux/amd64：`sha256:6b8f6d4062e29a2e069892770af3e00952c1323a486ec3247b5cddbe55546044`。
- linux/arm64：`sha256:414e4c897db46173efa6675cdd9e12c53a4b5c8c5606f65e2d862ba358eb78b9`。
- 公开发布说明为 4 条摘要、3 条升级提示；冻结 notes SHA-256 与公开正文均为 `8625714e6ae7dffc292bca6b5aab33493d92c6ba3007f63fbe4c2ea97644b47a`。
- snapshot artifact 将 candidate/version/run/attempt、原稳定快照、19 附件冻结清单与正文绑定；本地独立校验通过。发布回执、SBOM/provenance、修改组件来源与公开镜像契约成功。
- 发布后独立读回 GitHub Latest=v1.28.0、Docker latest=`sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`；preview 为本版 digest。

## 自更新通道验收

加入预览仅切换来源，不自动安装；自动安装默认关闭；退出不自动降级；版本、摘要、4 条说明及 3 条升级提示与公开来源一致。真实 published/upgrade 流程通过，含失败恢复、通道保持及 PTY 生命周期。旧版首次自动升级仍沿用旧入口，OpenRC/无 systemd 边界保留；生产宿主更新未执行。

## 分支归档与本地资源

| 原分支 | 原 tip | 本地归档 ref | 回读与保留 |
| --- | --- | --- | --- |
| fix/shared-host-script-maintenance | 67518fd45e2b699f8cbb5d239c56c3977f759188 | archive/fix/shared-host-script-maintenance | 本地回读 SHA 一致；工作树保留 |
| fix/audit-remediation-20261103 | 931ad9f0541c47b0efb296cfc6bc3fdd79799a4a | archive/fix/audit-remediation-20261103 | 本地回读 SHA 一致；工作树保留 |
| fix/rc7-incomplete-snapshot-symlink | 2bdb18289d1f820157c46785fdfd7f704179455b | archive/fix/rc7-incomplete-snapshot-symlink | 本地回读 SHA 一致；工作树保留 |
| fix/rc7-validation-tools | a7ead032db69eb44df365398ed05dc4929b02e6b | archive/fix/rc7-validation-tools | 本地回读 SHA 一致；工作树保留 |

四项 tip 均为正式产品标签祖先，原作者已交付或为本任务自有；工作树 clean 后改名，已有 upstream 已解除。未删除远端分支；远端验证分支保留，不以本地归档冒充远端处置。RC 列车保留；重复 L2 验证别名及失效 linked worktree 记录保留待处置，非新产品候选。未提交备份工作树、未知/旧工作树与原失败材料均保留。

没有删除工作树、大型缓存或唯一证据；不报告磁盘净释放。必需发布/验收证据继续在本轮证据目录保存；未新增知识或工作流，复用既有 release-kpanel 流程。

## 生产部署安全核对

不适用（预览版禁止生产部署）。prod-108、kpanel-eu 和 arena-154 本轮均未连接、备份、部署、升级或核对；没有生产写操作。公开镜像不等于生产部署。

## 回滚

保留本版不可变 tag/digest、上一预览 v1.29.0-rc.6 / `sha256:bcb3fb8470324ab4514e768b2bd3a15039310509fd53a5c2f3c9452d295b8d90` 和稳定版 v1.28.0 / 上述 latest digest。未执行生产回滚或生产数据备份；退出预览不自动降级，默认更新来源继续为稳定版。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-03T11:37:26+08:00
- 候选冻结时间：2026-11-03T21:21:26+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：11
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/notification-fixture/real-clock-retention",
    "count": 2,
    "impact": "The existing restart notification test used a fixed October 4 clock for the first service and real November 3 time for the reopened service, so the 30-day retention correctly removed the event and the test misreported a duplicate.",
    "recoveryEvidence": "tools-ci-first-failure.log and tools-ci-second-failure.log; 31c18bf7 passes ten targeted repetitions and is included in subsequent CI.",
    "permanentAction": "31c18bf7 injects the same fixture clock on reopen; no production notification change.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "migration-rootfs/powershell-flags/dotted-native-arguments",
    "count": 1,
    "impact": "The first disposable test launch parsed unquoted dotted Go test arguments incorrectly; the binary refused execution.",
    "recoveryEvidence": "Observed tool output flag provided but not defined: -test; no separate original file log. Quoted argument invocation succeeded; exact final migration-final-rootfs.log passed.",
    "permanentAction": "Pass quoted -test.run=... and -test.v=true as individual native arguments.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "official-source/windows-curl/tls-connection-reset",
    "count": 1,
    "impact": "The local verification download failed before any host operation or repository change.",
    "recoveryEvidence": "Observed curl exit35/connection reset; no separate original file log. PowerShell HTTPS download of the exact public commit succeeded and official-current-source-test.log passed.",
    "permanentAction": "Use the qualified PowerShell download for this local evidence path; product fallback behavior is independently tested.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "candidate-preflight/trusted-automation/lifecycle-test-not-on-main",
    "count": 1,
    "impact": "Pre-dispatch tree comparison showed the shared-script lifecycle test was not yet trusted on main; dispatch was stopped and no tag or registry write occurred.",
    "recoveryEvidence": "Candidate-to-main diff included packaging/tests/app-conf-lifecycle.sh; a7ead032 admits the corresponding previously reviewed implementation and test together. 2cf32fea has only the permitted source-scope JSON difference in trusted automation paths.",
    "permanentAction": "Compare every candidate automation path against the complete intended main admission before final freezing; do not derive that list from only one source branch.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "local-integration/git-merge/unsupported-trailer-option",
    "count": 1,
    "impact": "A local merge command rejected unsupported --trailer before changing the worktree.",
    "recoveryEvidence": "Observed Git usage error; no separate original file log. Follow-up uses merge --no-commit then git commit --trailer and a7ead032 records the result.",
    "permanentAction": "Attach trailers with git commit, not git merge.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "review-metadata/security-counts/unverified-severity-label",
    "count": 1,
    "impact": "The first review trailer overstated one finding severity; all three independent findings are medium.",
    "recoveryEvidence": "deb8168a original retained; immutable follow-up394677b2 records H0/M3/L0 from both review JSON reports, and final2cf32fea retains the corrected counts.",
    "permanentAction": "Read severity fields from the independent result JSON before writing a summary trailer.",
    "historicalReleases": [],
    "position": "before-production-write"
  },
  {
    "fingerprint": "local-tests/python-entry/windows-app-alias",
    "count": 1,
    "position": "before-production-write",
    "impact": "The first unqualified Windows python entry returned without a unittest report; it was not treated as evidence of five passing tests.",
    "recoveryEvidence": "Observed initial tool output; no separate original file log. Explicit bundled Python 3.12 and independent Python 3.13 executions each reported five passing tests; platform-independent-review.json preserves the independent result.",
    "permanentAction": "Use a qualified Python interpreter path and require the actual unittest summary; use -B for repository probes.",
    "historicalReleases": []
  },
  {
    "fingerprint": "postflight/checksum-count/legacy-document-assumption",
    "count": 1,
    "position": "before-production-write",
    "impact": "A local post-publication summary checker expected the legacy 11 checksum lines and rejected the actual 15; publication and canonical public verification had already succeeded.",
    "recoveryEvidence": "Observed assertion 15 !== 11; original canonical public-release.json records checksumCount=15 and status=passed. The corrected summary retains 15 and the nineteen-asset identity.",
    "permanentAction": "Derive checksum counts from current canonical public verification and actual asset inventory, not an older acceptance summary.",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/metrics/process-only-recovery-field",
    "count": 1,
    "position": "before-production-write",
    "impact": "The first acceptance draft placed process-recovery details in the field reserved for a failed production change; the validator rejected it before commit.",
    "recoveryEvidence": "report-release-metrics rejected recovery details without an explicit failed change state. The field now says not applicable, while all process incidents retain their own recovery evidence below.",
    "permanentAction": "Keep production-change failure metrics separate from preparation and postflight process incidents; validate the record before commit.",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/business-context/prerelease-baseline-field",
    "count": 1,
    "position": "before-production-write",
    "impact": "The first documentation candidate used an RC in the business baseline version field, whose schema requires the nearest stable tag. The local documentation candidate had not been pushed.",
    "recoveryEvidence": "check-business-context-freshness reported missing semantic baseline; stable baseline restored to v1.28.0 while exact reviewed commit stays 2cf32fea. Original local commit bc2fc886 and tool output are preserved in the local reflog/trace.",
    "permanentAction": "Use the nearest stable tag in the baseline version field, record preview identity in its section, and check each validator exit before commit/push.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

有期低风险共享缓存容量待办、Windows AI 内存测试平台基线、实际双栈宿主流量、真实阻塞挂载、物理 ARM64 和生产全生命周期仍按上文边界保留。RC6 中稳定版 source-identity 自动前置入口要求不因本轮手工资格化而自动关闭；稳定版准入仍需对应独立验收。未把准备失败、公开成功与生产部署混为同一事实。
