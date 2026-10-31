# KPanel v1.29.0-rc.1 预览版发布验收记录

日期：2026-10-31（Asia/Shanghai）；发布级别：L3；releaseChannel=preview；releaseTrain=1.29.0。

候选提交 / 标签：`52015445b78315e7567e48703c9dfe2d9bfe2f00` / `v1.29.0-rc.1`；产品 tree：`3ff740cc152b3cd71a26100bded20bd61e8bc63a`；annotated tag object：`b6250b889d5e5be1d723934fd6c93dca62dc2ea9`。

**预览产物已公开，完整发布与公开验收通过；生产未部署。** [Release](https://github.com/kejilion/KPanel/releases/tag/v1.29.0-rc.1)，ID=429633431，公开时间2026-10-31T01:11:37Z，draft=false、prerelease=true。[发布任务41149393403](https://github.com/kejilion/KPanel/actions/runs/41149393403) attempt2结论success；无重发、无镜像覆盖。

## 发布画像与范围

- 批准基线 `6e575b5c27ab846769d24678a26ac1b947869bdf`；新增 FastAdmin 网站模板、固定 recipe 允许列表、菜单10及原生 `k fastadmin <域名>` 命令；沿用前一预览的防护修复。
- 原候选 `b7395dbac86b38d43d03fc3ca81788a8d30fb582` 的代码 `01188545` 经 `b597e194` 重放；后续 `6f351dcd` 准备版本/来源、`b8a38d5e` 明确失败恢复边界、`b15b9432` 记录复核；Windows修复经e46287cb加入、77f49294重新资格化、52015445冻结最终候选。
- 用户旅程：网站→新建→展开更多模板→FastAdmin→提交；任务成功只表示文件/配置就绪，仍需访问安装页初始化数据库/管理员并保存随机后台入口。
- API路径、数据schema、端口、Compose、Agent权限和应用市场契约不变；固定允许值和脚本能力扩展。未调整依赖版本/资源预算，不宣称性能提升。
- 追加发布门禁发现的Windows Node回滚拒绝访问修复（a4517e97经e46287cb纳入）；不纳入其他本地候选或脚本主线无关更新；未执行生产安装、升级或部署。

原发布run41145928580/1的候选b15b9432已通过Linux CI/L3，但Windows SCM回滚替换旧二进制时Access denied，因此promote/main-ci/publish均skipped，未推进main、未创建tag/Release。保留首败Windows日志及原L3回执，没有盲重跑原SHA。独立工作树复现真实运行EXE的同类拒绝，修复只把该错误纳入现有5次/累计1.5s重试；永久只读拒绝仍保留原错误、两端内容与目标只读属性，未放宽ACL或完整性校验。红测失败、绿测全包通过，精确源文件/命令/日志摘要见windows-regression-receipt.json。具体原runner持锁者未捕获，不能宣称底层原因完全定位；新候选的GitHub真实SCM apply/rollback闭环已通过，后续完整CI/L3仍按新SHA重跑。

本轮run41149393403/attempt1在产物Windows检查出现另一类15秒首次启动超时：ConPTY最小环境无命令标记，普通PowerShell最小环境对照也超时；同机system-folders对照12.28秒通过、runner-env0.20秒通过、CMD/生产终端交互及SCM更新回滚通过。相关hostpty/terminal/验证入口均未改动；同候选前三台Windows runner及本机对照通过。推断为宿主相关初始化延迟，具体原因未确认，保留原始日志与风险，不计为已修复。仅更换新GitHub-hosted Windows VM作一次带诊断日志的有界复核，15秒断言未放宽；若同指纹再现即停止复核转诊断。attempt2继承同一SHA的原成功CI/L3/main/tag回执，只重跑失败Windows作业及其下游；L3原回执仍是attempt1。首次产物构建发布实际发生在attempt2，之前release/public均skipped，无版本镜像覆盖。详细结果与未关闭事项见windows-shell-failure-classification.json，Windows维护负责人于2026-11-07前复核。
## 候选与来源处置

- `release/v1.29.0-candidate@52015445b78315e7567e48703c9dfe2d9bfe2f00` 按预览列车保留，tag/digest不可变；`release/v1.28.1-candidate` 保留此前预览恢复点。
- KPanel原来源 `codex/fastadmin-php74@b7395dbac86b38d43d03fc3ca81788a8d30fb582` 已经映射纳入本版；具体归档引用及回读见下方收尾附记。
- sh/nginx原任务工作树和本地分支保持原样；原有其他工作树、未公开提交与 dirty 修改未改动。新建的发布clone只在本地合入main后显式推 `refs/heads/main:refs/heads/main`，未推功能分支、未创建PR。
- 本任务产品与验收工作树保留恢复身份；只清理已确认测试运行资源。没有将远端归档当作本地删除授权。

## 跨仓库联动判定

scriptLinkageState=coupled；变更集 `fastadmin-php74-20261031`，脚本/模板先公开后发布KPanel。

- 旧内置脚本 `6f6d8445b3e0884dc96035879b8d76ff97a175b3` / SHA-256 `fe9625222ac3ffa300de131d4f8a9da212a3ca125054ae4a0d658f69c70508b5`。
- 新内置兼容脚本 `5b8327fa9cfed52cfb3a96b92d15280f25499bd4`，经公开main合入 `046614235e5ef96cd4a80d39218b68fee75501f7` 可达；公开main之前为 `557f571dd6ee3b0f8dee269353beb16f11fe970a`。兼容提交仅在旧镜像脚本基线上增加FastAdmin，主线保留其既有其他更新。
- CN另恢复规范区域入口default→CN；KPanel内置根脚本区域值未变。根SHA-256 `2ba489e89be5320537135a1ca0bdd3c1b69f5ff927c548593ec6ee9ae1ed51e4`；CN `e7806a617c2a4fe9247754ee465d0c8a0ab8a6f709fa7b154172177b831711e3`，除区域入口外字节一致。
- 模板 `kejilion/nginx@c830bf34ae6da8e183b4892860fbce30ea6dc651/fastadmin.com.conf`，SHA-256 `986b57db53211332331c1c0022e7894546c8e8d3a1e4baeef5f2c95f01c23f46`；此前main `1e185a050b7c6d5670a82dd70a8e85bbd49de3f6`。脚本在发布站点前检查模板摘要。
- 官方完整包1.6.8.20261019固定SHA-256 `457bbe56d5a37d69a2e1371bea09bef8ce88e2dd11516da930ea442d70ae906f`。只修正固定包两处INI读取，保留共享PHP设置。
- 匿名GitHub contents/git-blobs API回读以上三份完整字节，摘要全部一致；本机raw域名TLS被重置的原失败保留。GitHub镜像构建以实际raw URL与Docker ADD checksum成功，未放宽验证。
- 配套回滚：KPanel可恢复前一tag/digest和数据备份；独立脚本可读取原main/旧内置兼容commit，模板恢复此前main。本轮未执行真实回退或自动删除业务数据库。

## 多维质量结论

| 维度 | 状态 | 证据 | 风险与边界 |
| --- | --- | --- | --- |
| 业务与双端互通 | 已验证 | 固定argv/selector回归、39项SitesView测试、CLI初始化与后台登录页路由 | 完整宿主机原生安装未执行 |
| 网络与供应链 | 已验证 | 固定包/模板/脚本摘要、CI/L3扫描、双架构公开身份、19附件回读 | 非全范围CF审计；PHP74应用维护风险保留 |
| 稳定与失败恢复 | 已验证 | 下载/摘要/缺文件/现有域名/Nginx失败回归、真实重启 | 建库/移动源码后部分失败保留产物，需人工检查；旧Nginx恢复是best effort |
| 性能与资源 | 不适用 | 无预算或性能逻辑变更，既有门禁通过 | 未做低配压力或长时soak，不宣称性能收益 |
| 体验与可访问性 | 已验证 | 候选mock交互、390px、14/13px、Tab/Enter与3px焦点 | 原生125%/200%缩放、实体触控、读屏与本机截图未验证 |
| 数据/配置迁移 | 不适用 | 无新增schema或迁移；复用脚本布局 | 不等同完整原生TLS/建库及真实生产验证 |

## 复核、安全与自动门禁

- 独立复核PASS WITH FOLLOW-UP：原实现与最终发布任务分离；另一提供商CLI不可用，按规范使用干净Codex复核，fallback=provider-unavailable。确认固定argv、PHP/Nginx契约和失败保留边界；原13px标题已局部修正14px，说明13px。
- 自动安全盲审范围：精确基线 `6e575b5c` 输出not-applicable groups=0；无CF审计，无稳定版补漏审计；不将其记为全范围安全通过。
- OCR1.12.11双臂完成，最终代码范围16/16覆盖；末轮为77f49294范围，后续仅记录元数据。自由臂原件保留，最终回放已见规则，blind=false、constrained-only=unreported，无新增confirmed发现，不归功于OCR的独立发现。
- 本地类型检查、SitesView39项、Go固定选择器/argv、发布说明、业务事实、治理与任务ready/handoff通过。首次无效准备检查保留于流程异常。
- 候选push CI [41148108889](https://github.com/kejilion/KPanel/actions/runs/41148108889) 同SHA success：Web332文件3349passed、6项原有skip；治理366项，Go、race、漏洞/Node审计、源码/配置/密钥扫描、应用生命周期及Windows Node通过。
- 完整L3唯一入口 `node scripts/run-release-l3.mjs --github-hosted`，run41149393403/1，入口SHA-256 `43e57f4fd321846204020d6cf27eb75961f0aefa1284a9187e786601704c2d40`，runner ubuntu24/20261026.354.1，Go go version go1.27.3 linux/amd64，Node v24.22.1，Docker 28.0.4。状态passed，2026-10-31T00:18:40.308Z 至 2026-10-31T00:30:47.463Z；源码三分组、安全、构建、镜像与生命周期门禁均以原回执/日志为证。
- dispatch候选CI、main/tag快进、主线CI、Release、双架构OCI及公开浏览器全部success，准确job ID见workflow-success.json与原日志。
- 扫描身份资格化：1255个第一方Go文件，`ffac3999fa0be71b4349a25d3ca3d357352693a9336442907ae7237ccab27ad8` → `07d19f40ed6537754c2e1929109de8f93e16b3a1617e122b8ddd1ba1f73bc52d`；七个变更文件的旧哈希能还原精确基线清单。无新增imports/GoFiles/依赖闭包；Docker仅固定脚本commit/摘要。保留原过期日2026-11-08、findings和disposition，没有重置观察期限或隐藏发现。

## 依赖与技术栈

Dependency freshness [41148108901](https://github.com/kejilion/KPanel/actions/runs/41148108901) success，生成2026-10-31T00:05:17.543Z，必需检测源9/9：348信号、37直接/基座行动项、311传递信号；需处理例外0，EOL current，下次2027-01-29。push事件的security-advisories按既有条件skipped，不冒充每日安全通告完成；CI/L3漏洞门禁另有实际成功证据。

本轮仅采用配套受管脚本/模板，不升级其他依赖。TS7与Node26类型例外沿用负责人KPanel base maintenance和2026-11-28复核期限，采用须通过原有完整兼容门禁；其他候选不在本版范围，首次完整检测的期限不重置。CF可选工具未请求，不查询、不计成功。最终镜像scratch，不将构建Go/Node环境等同宿主运行时。

## 隔离Linux与界面验收

- 环境策略local-wsl-dr/candidate-validation：真实Docker中的PHP7.4.33、MySQL26.10.0、Nginx1.31.8，实际镜像ID固定，未使用生产主机。
- 应用测试从精确脚本提取FastAdmin和HTTP适配函数；两函数在compat与公开main字节相同，SHA记录在linux-source-identity.json。缓存包和公开模板摘要一致。真实PHP/MySQL执行CLI初始化，随机后台登录页PATH_INFO路由、根`/.env`及`/uploads/probe.php`404，PHP/Nginx重启恢复通过。
- 下载被摘要一致缓存替换；证书和建库外层适配为夹具，登录验证只覆盖GET页面渲染，未提交真实账号认证；未实测在线完整包下载、原生TLS/环境安装或Web初始化表单。不能外推为整个原生脚本已完成全环境验收。
- 候选mock acceptance绑定 `52015445b78315e7567e48703c9dfe2d9bfe2f00`：选择/模拟提交通过。标题14px、说明及命令13px；390px页面scrollWidth390、卡片352px，说明高度38px无裁切；Tab焦点3px、Enter选择通过。mock列表返回通用proxy表示，未创建真实站点；本机截图CDP超时，无截图证据，不外推全页面视觉/历史字号债务已解决。
- 公开github-hosted真实Panel/Agent与浏览器：initial48、restart26、published45、upgrade31项全部passed、errors=0、assetFailures=0。使用固定浏览器/测试Runner与库存/测试CA夹具，不挂宿主Docker/systemd socket；不等于真实宿主安装/生产/systemd验证。测试recipe摘要与controller原件一致，productImageUnchanged=true；Runner实际ID `sha256:5b822d370c5efc8d26c7b212064a365ccddf163aabb5f9153e2e3d5bfe34fc26`。三份cleanup回执确认移除各自容器/网络。
- 无长时soak；物理ARM64、完整原生安装、跨发行版和真实主机自动更新回滚未覆盖。既有通道语义与公开接口/弹窗通过：加入预览只检查、自动安装与立即安装独立、退出不自动降级；不把隔离测试记为宿主机systemd/OpenRC操作。

## 公开产物与通道

- 版本/preview OCI index `sha256:655e84cc25d5a6343ca932f548e462fa6984d3bd1b0a2a2e6e08b06ccea45f12`；amd64 `sha256:c163ef2b4865997e6048bebdc6e073e26b1105ab59cc4b885a6eec865056ccd3`；arm64 `sha256:6019733f2e1d2ca450a61ec3f9d53097c5b36c16a057c64f184788788fc699c5`。
- 公开19附件名称/大小/digest与冻结清单、磁盘下载回读一致；15个checksum条目，modified-component provenance通过。公开正文SHA-256 `cdf8c81f04103576564fb869fa0b6073386ec504c483b081b0c5342c79db1c78`，摘要5条、接口升级提示3条（正文4条；共享运行时maxUpgradeNotes=3），面板接口及真实更新弹窗版本/digest/正文一致。
- snapshot稳定前快照SHA-256 `f2b7d9441a16371cc54074b9a5970d24431deb4108dd3d2d8ea9989a85a6e2e7`，附件清单SHA-256 `84bd06158ef4e70900c538e02feb9d9575790f91bd5a5a68f311796f506d2358`；controller `6e575b5c27ab846769d24678a26ac1b947869bdf`，准确产品/source/run/attempt身份均独立回读。
- GitHub Latest仍v1.28.0；Docker latest仍 `sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4`。前一preview v1.28.1-rc.1 / `sha256:dfb162a28988165a2a7854e01705a95b39723fb1ec0cfc9d3939447524fee54b` 保留恢复身份。
- 本轮没有重现上一预览的版本信息502；上一记录中的根因跟进仍保留，成功不能关闭未定位问题，若同指纹再现先诊断不盲重试。

## 生产与回滚

全部生产动作不适用（预览版禁止生产部署）；prod-108本轮未连接、读取、备份、核对、部署或升级，其他生产主机亦未操作。没有生产备份或实际回退证据，不声称用户实例已升级。

保留v1.28.0及前一预览不可变tag/digest，人工回退需配套数据备份；退出预览不等同降级。若安装中途保留源码/数据库，先人工核对，不自动删除业务数据，也不承诺所有失败可直接重试或旧Nginx恢复必然成功。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-30T18:32:16+08:00
- 候选冻结时间：2026-10-31T08:04:08+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：14
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preparation/git-auth/repository-ssh-config-missing",
    "count": 2,
    "impact": "Fresh publication clones initially lacked the repository SSH key selection; both failed before any clone or external write.",
    "recoveryEvidence": "Applied existing KPanel core.sshCommand to each own clone, then authenticated clone/push succeeded.",
    "permanentAction": "Reuse the configured repository SSH identity; do not alter global credentials.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preparation/git-identity/local-identity-missing",
    "count": 1,
    "impact": "Cherry-pick staged the intended changes but could not create a commit without local identity.",
    "recoveryEvidence": "Configured this own clone with the established task identity; cherry-pick --continue preserved original author and succeeded.",
    "permanentAction": "Set task-local identity before future own-clone commits.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "qualification/task-preflight/dirty-checkpoint",
    "count": 1,
    "impact": "Ready qualification ran while preparation edits were still dirty and correctly rejected that checkpoint.",
    "recoveryEvidence": "Committed the complete candidate and reran ready/handoff successfully; no invalid receipt was accepted.",
    "permanentAction": "Run clean qualification at a committed checkpoint; retain exact writer check already completed before first write.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preparation/apply-patch/mismatched-context",
    "count": 1,
    "impact": "A local UI patch referenced a nonmatching selector and made no changes.",
    "recoveryEvidence": "Read exact template context and applied the intended FastAdmin-only typography patch.",
    "permanentAction": "Use observed exact source context.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/npm-prefix/nested-working-directory",
    "count": 1,
    "impact": "Type checking used --prefix web from the web directory and failed before running.",
    "recoveryEvidence": "Corrected working directory; typecheck-corrected.log passed; SitesView 39 tests passed.",
    "permanentAction": "Use the package directory consistently.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "qualification/task-preflight/missing-go-path",
    "count": 1,
    "impact": "Governance tool probe lacked the installed Go toolchain PATH.",
    "recoveryEvidence": "Selected the existing pinned Go 1.27.3 path and passed canonical preflight; no replacement gate or downloaded toolchain.",
    "permanentAction": "Record the exact toolchain path in this task invocation; this fingerprint also appears in v1.27.2 and needs entry-point follow-up before future production write.",
    "position": "before-production-write",
    "historicalReleases": [
      "v1.27.2"
    ]
  },
  {
    "fingerprint": "evidence/public-source/raw-host-tls-reset",
    "count": 2,
    "impact": "Node fetch and curl to raw.githubusercontent.com were reset locally; source identity was not inferred from the failed downloads.",
    "recoveryEvidence": "Anonymous GitHub contents/git-blobs API independently returned all exact bytes and hashes; GitHub-hosted image build still must prove raw script consumption.",
    "permanentAction": "Bounded alternative read of the same public immutable source; do not keep retrying the raw host or relax checksum.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/local-preview/screenshot-cdp-timeout",
    "count": 1,
    "impact": "Local Page.captureScreenshot timed out; no screenshot evidence was produced.",
    "recoveryEvidence": "DOM/AX interactions and computed font, keyboard and narrow viewport measurements passed; screenshot remains explicitly unverified locally.",
    "permanentAction": "Preserve timeout and rely on the required GitHub browser artifacts for their covered journeys; do not claim local visual screenshot coverage.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/volume-inspection/wsl-argument-expansion",
    "count": 1,
    "impact": "Inline WSL inspection had broken argument expansion; no deletion was attempted.",
    "recoveryEvidence": "Stored the exact read-only inspection in a file, identified the five unused fastadmin_test-only volumes, then removed only their explicit names after rechecking ownership.",
    "permanentAction": "Use reviewed file-based scripts across Windows/WSL quoting boundaries; preserve the two unidentified volumes.",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "monitor/release-fanout/missing-windows-job",
    "count": 1,
    "position": "before-production-write",
    "impact": "Initial publication monitoring followed Linux and L3 but omitted the independent Windows job; its 23:43:16Z failure was identified at the 23:56:10Z aggregate failure check, delaying diagnosis. Automatic gates prevented main/tag/publication advancement.",
    "recoveryEvidence": "Original Windows job124370963626 and dispatch-windows-failed.log preserved. Fixed candidate52015445 has exact-SHA Windows SCM success in push CI, dispatch and main CI; subsequent monitoring explicitly checks all mandatory fan-out jobs.",
    "permanentAction": "Track every required job and terminal dependency using bounded small-page APIs/direct job endpoints; never infer complete pipeline health from a partial green subset. Reuse existing monitoring APIs; no new workflow or retries-to-green.",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication/windows-shell/first-start-latency",
    "count": 1,
    "position": "before-production-write",
    "impact": "Publication Windows host timed out in both ConPTY and direct minimal-environment inbox PowerShell startup; same-host CMD/production-terminal and SYSTEM updater passed, so image/Release stage was skipped. No production deploy or version-image write occurred.",
    "recoveryEvidence": "Original publish-windows-failed.log, direct controls and local control retained. One bounded fresh-host recheck on job124392658662/attempt2 passed full native verification with unchanged source/15s assertions; publish-windows-recheck.log retained. No second recheck; cause remains open.",
    "permanentAction": "Root cause remains open. Windows node maintenance reviews by 2026-11-07; if same fingerprint recurs stop rechecks, diagnose initialization and use the existing native entry point without weakening gates.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/release-notes/runtime-upgrade-limit",
    "count": 1,
    "position": "before-production-write",
    "impact": "Local post-publication identity helper incorrectly expected four runtime upgrade summaries because CHANGELOG has four bullets; canonical public verification and browser correctly reported three. Initial source lookup also guessed a parser filename before using the exact rg result.",
    "recoveryEvidence": "Shared immutable internal/selfupdate/source.go:24 defines maxUpgradeNotes=3, lines251-253 apply the cap; live body retains four bullets while public JSON/UI show the first three. Corrected only the outside-repository helper to the established contract; full 150-check artifact identity verification passed. No product/parser/gate/body/image changed.",
    "permanentAction": "Read exact runtime parser limits and paths returned by rg before writing expected counts; distinguish body content from bounded API summaries. Keep frozen body hash checks and original assertion result.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

已读取最近五个稳定验收记录的原指纹，未把缺失事实推断为零。Windows产品断言失败由正常门禁拦截，已在质量章节保留，不机械计为执行器异常或生产变更失败。Go PATH准备指纹在v1.27.2出现过，本次使用既有工具链路径通过；未来生产写前须处理入口问题。本轮没有生产写入，不新增治理流程或工作流，复用现有release-kpanel。

## 本地资源、证据与剩余事项

原FastAdmin及本轮mock进程已停止，命名测试容器/网络/临时目录均0。补查匿名卷，依据创建窗口、无容器引用及仅有fastadmin_test业务库，逐项验证后移除5个确认测试数据库卷；另2个归属未确认卷保留。精确卷名、检查与回读见volume-cleanup.log/cleanup-receipt.json。共享镜像、唯一日志、安装包、来源工作树及恢复引用保留，未prune或压缩VHD。

C盘任务开始可用109709336576字节；测试卷清理前109817503744、后109582098432字节，净变化-235405312字节。删除WSL数据不等于宿主VHD缩小；同期候选依赖和其他写入使宿主净变化不能归因于删除，未宣称释放1GiB。结束读数见收尾附记。

证据根 `C:/GitHub/_release-evidence/kpanel-v129-rc1`：候选CI/Windows、依赖完整报告、源码资格化、匿名公开源、双臂/独立复核、本地测试与预览、Linux运行/清理、L3、snapshot/publication/public原件和独立身份核验。公开Actions工件是精确run/attempt原件。

剩余风险：PHP74及应用维护，部分安装失败后人工恢复、Nginx恢复健康需回读，完整原生TLS/宿主安装/真实更新和未覆盖可访问性；上一预览502上游根因仍未定位。无生产部署，不外推未测能力。

## 来源及本地收尾附记

- 已回读 `archive/codex/fastadmin-php74` = `b7395dbac86b38d43d03fc3ca81788a8d30fb582`、`archive/fix/windows-rollback-image-lock` = `ce61caa65d9aa6eea6cc866db4b224f9a4c57d68`，原tip保存。源功能8个路径与最终产品逐字节一致；SitesView仅另有已复核字号修正。Windows两文件也与最终产品逐字节一致。归档不等同原始tip原样上线，采用映射见上文。
- 原FastAdmin/sh/nginx工作树与原本地分支均clean保留，不作为待发布候选；归属未变的原工作树未删除。本人Windows修复工作树改为archive/fix/windows-rollback-image-lock；独立sh/nginx发布clone的功能分支仅在本地归档，远端仍只推main。
- 当前RC候选与此前预览列车保留。纯验收docs/v129-rc1-acceptance在同SHA候选Linux CI、主线快进及主线CI成功后移到archive/docs/v129-rc1-acceptance；最终精确SHA/回读由closeout.json记录，不改写已发布产品。
- 收尾空间采样：C盘可用109335916544字节，相对任务开始净变化-373420032字节；含依赖/证据及其他并行写入，不归因为测试卷删除，未做VHD压缩。
- Win32文件替换与权限语义参考[Microsoft MoveFileExW](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-movefileexw)及[DeleteFileW](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-deletefilew)；本轮确认依据仍是独立原生红绿回归与GitHub SCM实测，不以文档推断原runner持锁者。
