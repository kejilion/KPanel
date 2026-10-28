# KPanel v1.28.0-rc.13 发布验收记录

日期：2026-10-28（Asia/Shanghai）。发布级别：L3；releaseChannel=preview，releaseTrain=1.28.0。公开预览产物已发布，生产未部署。

候选提交 / 标签：63688fb7c10f87186b6636bf0bca36abaf13fb65 / v1.28.0-rc.13

上一稳定版本 / 回滚点：v1.27.2 / sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10

候选分支与发布后处置：release/v1.28.0-candidate / 预览候选分支保留

- 原分支 / 精确 tip / 处置分类：codex/site-defense-monitor-20261028 / 8f40bd5ec9c820aaf9dc0c99c6644d3fdf93c2f8 / 纳入 RC13；本地保留。该源分支未推送到远端。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：源分支未归档；origin/main 与 origin/release/v1.28.0-candidate 在发布时均指向 63688fb7c10f87186b6636bf0bca36abaf13fb65；v1.28.0-rc.13 为注释 tag，peeled commit 同为该 SHA。正式发布后本验收记录按既有流程作为独立文档提交追加到 main；tag 与镜像不改写。
- 本次来源任务分支：合入版本；候选通过 merge commit dd6c139dd777e074fc0449558997b50493e1018c 纳入精确 tip 8f40bd5ec9c820aaf9dc0c99c6644d3fdf93c2f8。
- 本地分支/upstream/worktree：源分支在 C:GitHub_codex-taskskejilion-panel-site-defense-monitor，当前为干净本地分支且没有远端同名 ref；发布候选在 C:GitHub_releasekpanel-v128，干净，远端候选 ref 指向已发布提交。
- 未完成归档项 / 责任人 / 下次复核触发条件：源分支与 RC 候选列车均保留，未执行删除或归档；下个预览列车整理时复核分支去留。

归档不代表生产上线；本版生产未部署。

## 发布画像

- 业务域：网站防护状态、Fail2Ban 网站 jail 日志及环境防护操作。
- 变更面：状态展示、只读日志、兼容 API 字段，以及需用户明确提交的主机防护操作；没有自动开启防护。
- 受影响用户旅程：网站监控查看防护状态和近期 Fail2Ban 日志；从监控页进入防护设置；环境管理执行已有防护操作。
- 未变化契约：API 增量字段，无数据库迁移；Compose、端口、Agent 权限和 kejilion/apps 契约未改。Docker 镜像配套 kejilion.sh 固定为本版已验证提交。
- 风险等级及理由：中等。涉及主机防护状态和显式防护操作；日志仅展示兼容来源可读取的数据，保护状态需由用户在设置中配置。

## 发布范围与未纳入内容

- 用户可见更新：网站监控页新增防护状态总览、近期 Fail2Ban 网站日志和防护设置直达入口；修复慢速刷新状态展示；统一环境管理的防护输入与反馈，并加入 cloudflare-off、cloudflare-shield-off 显式操作。WAF 防御事件依赖兼容的 WAF 审计日志，日志不可用时显示空状态；升级不会自动开启网站防护。
- 精确提交清单：
  - 14be4af92a3de9181c083a5eebcc3a08007010f6 feat: align website defense controls and monitoring
  - 3616cb46343e906ecb7e5dc0052f4361ccbcc31f fix: pin site defense script candidate
  - e9789e742341519ca3d43d8b6740c5dc9d1cf869 fix: preserve slow defense status refresh
  - 9abde29d9beae749d4ececd8404d0fb55180b604 fix: open defense settings directly
  - 8f40bd5ec9c820aaf9dc0c99c6644d3fdf93c2f8 review: record OCR coverage
  - 发布组装提交：dd6c139dd777e074fc0449558997b50493e1018c、fcc77452227a22d323b7761a9bd90a94ff8e36d6、fbf2f5bd4907540354b19f1f26c07b6a61a14998、63688fb7c10f87186b6636bf0bca36abaf13fb65。
- 明确未纳入的分支、文件或后续事项：fix/terminal-follow-recovery-20261028 与 feature/browser-vnc-prototype 未混入本版；RC13 不包含 VNC 原型或终端跟随修复。

## 外部审计与修复交付

安全盲审按 PROJECT_RULES.md 5.4 记录；本记录不把自动扫描或 OCR 覆盖度说成独立安全审计。

- 安全盲审补漏（quality-audit review 档）：not-run；本次没有单独执行 quality-audit review。
- node scripts/security-review-scope.mjs --base v1.27.2 输出：security_review=recorded groups=68 invalid_trailer=true；68 个触及组均为 recorded，没有 missing/stale。提交范围内存在旧式 Security-Review trailer 格式，工具标为 invalid_trailer；该历史格式提示未在 RC13 中改写。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本版没有声称新确认的安全 finding；网站防护 UI 候选 commit 8f40bd5 的 OCR-Review 记录 valid H0/M0/L0。Windows 更新回滚首次失败见“流程异常明细”，根因尚未确认，未记为已修复漏洞。
- 修复交付状态：网站防护功能源码、RC13 与公开 prerelease 已交付；生产未部署。kejilion.sh 配套版本已在其公开 main 中，未在本次另行发布脚本。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：来源 commit 的 OCR-Review 为 1.12.11，range=174e9ce35f738f060b3709f07b1f413ac42d8afa..9abde29d9beae749d4ececd8404d0fb55180b604，files=15/17，free-form=0，valid H0/M0/L0，constrained-only=unreported；不据此推断工具有效性或退出。
- 盲审观察记录：未执行，本版不宣称盲审有效或覆盖 Cloudflare security-boundary-audit。

## 跨仓库联动判定

- scriptLinkageState：coupled
- 变更集编号：不适用（该脚本提交已在 KPanel 发布前进入 kejilion/sh 公开 main）。
- KPanel 实际内置脚本基线 commit / SHA-256：594138a9a55ab9d75282948a27fcbcf9835650ec / 27ce9d8c34040ed30f0ffc360d388f7d9617b83a7dc9b63061812f662639aa01。
- 脚本候选 commit / SHA-256：594138a9a55ab9d75282948a27fcbcf9835650ec / 27ce9d8c34040ed30f0ffc360d388f7d9617b83a7dc9b63061812f662639aa01；该 commit 已是公开 kejilion/sh/main 的祖先。
- 状态判定依据与兼容性证据：Dockerfile、镜像 OCI 标签和发布检查绑定同一 revision/hash；managed-script contract、tests/test_cn_script_sync.sh 与 tests/test_unified_cf_smoke.sh 通过。
- 本版发布决定：脚本先进入权威 main，再发布 KPanel 预览版。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 网站监控和环境管理定向测试 15/15 通过；L3 web/Go/deploy lanes 通过 | 真实主机上的 WAF 审计日志与 Fail2Ban jail 未在生产验证 |
| 网络入侵与供应链安全 | 已验证 | GitHub-hosted 安全扫描、Go call-path 扫描、依赖审计、镜像扫描和镜像契约通过；发布 provenance 附件已上传 | 独立 quality-audit review 未执行；security-review-scope 有旧式 trailer 警告 |
| 稳定性、失败恢复与兼容 | 已验证 | 候选和发布 Windows 检查最终通过；隔离容器初始/重启验收通过 | 主线 Windows SCM 回滚首次出现 Access is denied，重跑通过但根因未确认 |
| 性能与资源预算 | 不适用 | 本版无性能目标或压测声称 | 未做独立性能/长时 soak 测试 |
| 用户体验与可访问性 | 已验证 | 已发布版浏览器 45 项、升级对话框 31 项检查通过，页面错误为 0 | 本机手工 CUA 视觉检查未完成；GitHub-hosted 浏览器与截图证据通过 |
| 数据、配置与迁移 | 已验证 | API 增量字段；无数据库迁移；公开更新流程保留 stable 默认、自动安装关闭 | 真实安装环境的 WAF/Fail2Ban 数据源需由主机实际配置提供 |

## 自动门禁

- 定向测试及结果：npm test -- src/views/SiteMonitoringView.test.ts src/views/EnvironmentView.test.ts：2 files / 15 tests passed；npm run build 通过，i18n 3944 phrases / 28 catalogs；node --test scripts/tests/source-checks.test.mjs：21/21 passed；git diff --check 通过。
- make verify-release 环境和结果：本机未执行，Go 不在本机 PATH；GitHub-hosted candidate CI 与 L3 在 Go 1.27.3 环境通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：Publish preview run 40671168205 attempt 1 的 l3 job 122758110970 通过；L3 receipt candidate 精确绑定 63688fb7c10f87186b6636bf0bca36abaf13fb65，runner image ubuntu24 version 20261026.354.1，Go 1.27.3、Node 24.22.1、Docker 28.0.4，entrySha256=43e57f4fd321846204020d6cf27eb75961f0aefa1284a9187e786601704c2d40；source lanes web/Go/deploy 均 pass，identityUnchanged=true。L3 receipt 未提供 plan/bundle SHA-256 或不可变 Runner ID，未补造。证据：C:GitHub_release-evidencekpanel-v128-rc13public-recordsl3-40671168205-1。
- 候选 CI：[最终候选 push CI 40670392875](https://github.com/kejilion/KPanel/actions/runs/40670392875) attempt 1 对精确 SHA 成功；Publish preview 内 candidate-ci 与候选 Windows 检查也通过。
- 主线 CI：[Publish preview 40671168205](https://github.com/kejilion/KPanel/actions/runs/40671168205) attempt 2 成功。attempt 1 的 verify job 通过，Windows SCM rollback 测试因恢复旧 binary 返回 Access is denied 而失败；attempt 2 对同一 candidate SHA 重跑该门禁成功（job 122769014180），未改源码。
- Release workflow：[Publish preview 40671168205](https://github.com/kejilion/KPanel/actions/runs/40671168205) attempt 2 全部成功；attempt 1 在主线 Windows 检查后停止发布。RC tag 与 main promotion 已在 attempt 1 成功，attempt 2 沿用精确 SHA。
- 发布说明门禁：最终正文通过；目标 1.28.0-rc.13，3 条摘要和 1 条升级提示。提示包括 WAF 审计日志要求、加入预览不自动安装、退出预览不自动降级和不自动开启网站防护。
- 安全扫描、镜像契约、SBOM/provenance：Go call-path、npm audit、源码/依赖/机密/配置扫描、原生镜像扫描、runtime image contract 均通过；KPANEL_MODIFIED_COMPONENTS.json provenance 附件已上传。此处不声称已独立验证 SBOM 之外的材料。

早期候选门禁保留如下：fcc7745 的候选 CI run 40668527901 因源码身份与 Dockerfile 派生 hash 过期失败，fbf2f5b 刷新后最终候选 CI 通过；另一独立 dependency-freshness run 40668527971 在早期候选上报告 TypeScript 与 @types/node 的到期例外复核，该维护项未由本 RC 修复。preview dispatch 40669866603 的 inputs 检查发现 scripts/mock-app-market-api.mjs 与 main 的 release-automation 树存在差异并停止；最终 candidate 63688fb 恢复该 mock fixture 文件与 main 一致后，inputs 检查通过。以上均未改写已发布 tag 或 stable 通道。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：未独立运行 dependency-report；release job 使用锁定依赖并通过 npm audit。
- 最近每日安全通告审计、EOL 复核状态及证据：独立 dependency-freshness run 40668527971 在较早候选上失败，提示 TypeScript 6.0.3→7.0.2 与 @types/node 24.19→26.6.5 例外的复核日期已过；RC13 未改动这些依赖策略，需由维护者另行复核。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：上述例外作为待维护事项保留；本次没有变更依赖版本。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go 1.27.3、Node 24.22.1、Docker 28.0.4；Dockerfile 固定 kejilion/sh revision 594138a9a55ab9d75282948a27fcbcf9835650ec。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：package.json/lockfile 仅更新 RC 版本号至 1.28.0-rc.13；镜像版本 sha256:afb71c77a0ed9bb9dc6e01ecd0feba2d829d53c8f163c66cead1ccfad4f310d6；配套脚本 SHA-256 27ce9d8c34040ed30f0ffc360d388f7d9617b83a7dc9b63061812f662639aa01。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：TypeScript 与 @types/node 例外待维护者复核，责任角色为 KPanel base maintenance；不作为本 RC 新依赖升级。
- 升级后的兼容、安全、构建、性能资源和回滚结论：RC13 build、audit 与镜像契约通过；没有单独性能对比。回滚预览镜像使用 RC12 的精确 digest。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Ubuntu 24；Docker 隔离容器；发布版真实浏览器与精确 RC13 镜像。
- 环境策略 ID 与允许用途：release-public-verify 的隔离发布验收；不连接 prod-108，不挂载宿主 Docker/systemd socket。
- 使用的精确候选或公开产物：63688fb7c10f87186b6636bf0bca36abaf13fb65；公开镜像 digest sha256:afb71c77a0ed9bb9dc6e01ecd0feba2d829d53c8f163c66cead1ccfad4f310d6。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：public-acceptance job 122776198059 success；浏览器 published 45 项、upgrade 31 项均 errors=0；隔离容器 initial/restart 48/26 项均 passed、errors=0。完整截图、报告、日志保留于 C:GitHub_release-evidencekpanel-v128-rc13public-recordspublic-40671168205-2。
- 测试窗口/循环数及风险依据：非 soak；执行一次隔离发布验收和一次容器重启恢复验收。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：公开浏览器验证更新提示、stable/preview 通道选择、拒绝自动安装、退出预览不降级、错误处理和页面错误；包含 1280、390、320 宽度场景。没有对网站监控 WAF/Fail2Ban 页面进行已发布主机数据源的实机验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器完成 API、TLS/session、设置持久化、终端历史与应用状态的 initial/restart 检查；systemd 不存在时更新请求按预期 fail closed；证据使用 Docker 库存与测试 CA 夹具，不代表真实生产主机写入。
- 未执行场景及原因：生产主机安装、真实重启、真实 WAF/Fail2Ban 日志源和多主机混合版本未执行；生产部署对 preview 不适用。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：版本 1.28.0-rc.13 与 digest 一致；正文 3 条摘要、1 条升级提示；公开 API 与浏览器返回官方 Release URL，cache=false、stale=false。
- GitHub Release 的 draft / prerelease / Latest 状态：[v1.28.0-rc.13](https://github.com/kejilion/KPanel/releases/tag/v1.28.0-rc.13) 非 draft、prerelease=true；GitHub Latest 保持 v1.27.2。
- Docker 版本与通道 OCI index：docker.io/kjlion/kejilion-panel:1.28.0-rc.13 与 :preview 均绑定 sha256:afb71c77a0ed9bb9dc6e01ecd0feba2d829d53c8f163c66cead1ccfad4f310d6；stable/latest 快照仍为 v1.27.2 / sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10。
- linux/amd64、linux/arm64 digest：amd64 sha256:3028b4ee506827d70b65ddf8324060a68e71e2f4b4c3c6da706d28caf521ecc7；arm64 sha256:260376b147f9660e7bbdf8201cd122edaebfaf59b02872d95c83db41a2fdd153。
- 附件及 SHA256SUMS：GitHub Release 19 个附件；SHA256SUMS 覆盖 15 个文件，公开资产摘要与冻结清单匹配。
- 公开镜像 image_e2e=pass：L3 与 public-acceptance 均通过；公开发布报告 status=passed。
- kejilion/apps / kejilion.sh 契约结论：应用市场契约扫描通过；kejilion.sh revision/hash 已绑定且 smoke tests 通过，script commit 已进入公开 main。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：浏览器检查 stable=v1.27.2，preview=v1.28.0-rc.13，digest 与 Release 一致。
- 加入预览只切换来源并立即检查，没有自动安装：通过。
- 自动安装开关与一次性立即安装相互独立：检查到默认 enabled=false，选择预览未启用自动安装或触发安装。
- 旧状态默认迁移到 stable，重启后通道选择保持：隔离容器 initial/restart 检查通过。
- 退出预览且稳定版较低时没有产生降级候选：公开浏览器与隔离容器检查通过；退出预览不会自动降级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：隔离容器检查与 release L3 通过；无 systemd 时请求拒绝并清理待执行状态。
- OpenRC 与轻量 Node 的当前边界已按 docs/release-channels.md 明确呈现：本 RC 不声称 OpenRC 上可执行 Panel 自更新；轻量 Node 的安装与更新仍按现有边界执行。

## 生产部署安全核对

> preview 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用，预览版禁止生产部署。
- 验证/灰度环境：不适用；本次只用 GitHub-hosted 隔离验证环境。
- 正式部署环境：不适用；未执行生产发布。
- prod-108：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：本文件中所有真实浏览器与容器验收均为隔离环境。

## 回滚

- 源码/tag：恢复 v1.28.0-rc.12；RC12 镜像 digest sha256:773f5dd758cd46b5969fb8748b2dfd96adf2607a5cdf9f406421d20595d61257。
- 镜像 digest：RC13 sha256:afb71c77a0ed9bb9dc6e01ecd0feba2d829d53c8f163c66cead1ccfad4f310d6；回退到上一个 preview digest，不覆盖不可变版本镜像。
- 数据/配置备份：无生产数据改动，因此没有生产备份。
- 回滚步骤和回滚后复核：在 KPanel 更新来源中选择稳定通道并检查版本；退出预览不会自动安装较低版本。手工部署回退时使用 RC12 固定 digest 并核对镜像标签与运行状态。
- 回滚后生产实际版本与健康状态：不适用，未部署生产。
- GitHub Latest、Docker latest 与标准更新入口实际指向：仍为 v1.27.2 / sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10。
- 公共默认更新通道决策：短期保留 preview RC；稳定默认仍为 v1.27.2，退出 preview 不自动降级。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-28T14:45:20+08:00
- 候选冻结时间：2026-10-28T18:13:25+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "main-ci/windows-scm/rollback-access-denied",
    "position": "before-production-write",
    "count": 1,
    "impact": "Publish preview attempt 1 的 main-ci Windows SCM rollback 检查恢复旧 binary 时返回 Access is denied，门禁失败并阻止 publish job；没有生产写入。",
    "recoveryEvidence": "同一 run attempt 2 的 main-ci/windows-node job 122769014180 通过；publish/windows-node job 122769920530 通过；最终 public-acceptance job 122776198059 与 run 40671168205 attempt 2 成功。原失败 job 122764104183 日志保留。",
    "permanentAction": "根因尚未确认。由 KPanel 发布维护者在下一 RC 前比较失败与通过 runner 的 SYSTEM 文件 ACL、旧 binary 句柄和 SCM 日志；若可复现则修复 updater 或测试隔离。退出条件为根因有证据且两次独立 Windows run 通过，不能以一次重跑成功代替根因分析。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

流程异常计数只把这次导致发布重跑的 Windows SCM runner 回滚访问错误计为流程异常；源码 hash 未刷新和 release-automation 树不匹配属于候选内容/输入门禁失败，已在自动门禁段落分别记录，不计入该流程异常数。上述 Windows 根因仍未确认。

### 流程异常明细

Windows SCM 首次回滚访问错误发生于 2026-10-28T18:34:18+08:00；恢复于 2026-10-28T18:45:48+08:00；逃逸门禁：未逃逸：main-ci Windows SCM rollback 检查拦截，attempt 2 与 Release Windows 检查通过后才启动公开 Release。

## 遗留风险与后续准入

- 本地资源回收：保留发布分支、源分支和 L3/public 验收证据；本次未执行磁盘回收。
- 未验证风险：实际服务器的兼容 WAF 审计日志与 Fail2Ban jail 未实机验证；Windows SCM 回滚首失败根因未确认；dependency-freshness 所报到期例外待维护者复核。
- 已实现待实机准入：网站防护状态 API、Fail2Ban 日志和防护设置直达需在有对应 Agent/日志源的隔离真机验证。
- 不阻断本版的理由：目标是预览；最终候选/主线/Release/公开验收均通过，稳定与 latest 通道保持不变。
- 后续应进入的自动门禁或专项工作流：下一 RC 前复核 Windows SCM rollback failure；补真实 WAF/Fail2Ban fixture 覆盖和依赖例外复核。