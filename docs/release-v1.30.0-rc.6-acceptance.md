# KPanel v1.30.0-rc.6 发布验收记录

日期：2026-11-07

发布级别：L3

候选提交 / 标签：`1ccf979714681273992c127618cfda495f737af0` / `v1.30.0-rc.6`。Annotated tag 对象：`58e295142b09b915574f6bb781dbf55f655e7124`，剥离后指向候选提交。

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览版保留。

- 原分支 / 精确 tip / 处置分类：`release/v1.30.0-candidate` / `1ccf979714681273992c127618cfda495f737af0` / RC6 已发布，按预览规则保留；`feature/site-settings-main-20261106` / `bf95693698ceb29c68e8772c7eb54ecd89758bb0` 与 `feature/monitoring-checks-mainline` / `580e563320ff59f2937ee6055f857006338b6d63` 的最终改动已纳入 RC6，源分支暂保留待任务所有权确认。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：本次未创建 `archive/` 引用；发布时已核验 `origin/main` 与 `origin/release/v1.30.0-candidate` 均指向 `1ccf979714681273992c127618cfda495f737af0`；`git ls-remote` 未发现上述来源任务分支的远端 ref。RC5 标签及对象保持不变。
- 本次来源任务分支：访问密码设置入口与交互改进来自 `feature/site-settings-main-20261106`；监控分组来自 `feature/monitoring-checks-mainline`，其原任务分支 `fix/monitor-checks-single-page` / `264525fa5570ace657dac5e38491944b5bfe1fdf` 被保留用于追溯。较早的 `feature/site-access-password` / `e52717b5b030d8c8d1d361d5b3420a0cb7c9e0b7` 未作为 RC6 新代码重放：访问密码基础功能已在此前主线中，RC6 纳入的是设置入口和交互完善。依据为 RC6 精确差异、两条引用任务记录及候选验收。
- 本地分支/upstream/worktree：`release/v1.30.0-candidate` 工作树干净，路径 `C:\GitHub\_release-candidates\kpanel-v130`；来源工作树均干净并保留，`fix/monitor-checks-single-page` 跟踪 `origin/main`（ahead 5 / behind 106），其他上述来源分支无 upstream。未删除工作树或分支。
- 未完成归档项 / 责任人 / 下次复核触发条件：KPanel Release Maintainer 在任务所有者确认释放后、下一 RC 收集或稳定版候选关闭前，核对来源分支是否仍需恢复，再保存精确 tip 并归档；在此之前保留原分支和工作树。

归档不代表生产上线；本版产物已公开，生产未部署。

## 发布画像

- 业务域：网站安全设置入口与历史监控检测展示。
- 变更面：展示与交互；沿用已有证书、访问密码和监控接口，没有新增宿主机写入能力。
- 受影响用户旅程：新建网站与编辑网站时折叠展开 HTTPS 证书、访问密码设置并分别保存；历史监控同页查看 Ping、TCP、HTTP 分组，并独立控制每组曲线与状态格。
- 未变化契约：API 数据结构、数据目录、端口、Compose、Agent 权限、`kejilion.sh` 与应用市场默认更新入口均未变化。
- 风险等级及理由：低至中；主要是设置入口、保存反馈和监控页面重排，涉及已有安全设置的可发现性，已用定向测试、浏览器和公开镜像验收覆盖。没有 AI 助手或手机终端代码变更。

## 发布范围与未纳入内容

- 用户可见更新：网站创建/设置中的 HTTPS 证书和访问密码共用折叠区块、独立保存；历史监控取消协议分页，Ping/TCP/HTTP 同页分组并各自控制曲线与状态格。RC5 已有访问密码基础能力；RC6 纳入的是设置入口与交互完善。监控同页分组未进入 RC5。
- 精确提交清单：RC5 之后、RC6 标签之前的提交依序为 `8226ea86cabfbe66cea666efa4aba3e8196653b9`、`6c95feb5e0b99796cd5d335a4c9a2a53939c6e7b`、`dc327ad8ee1f8cfb57d1305b6aa434befadf2719`、`9df6272deb748aae49c6a1e348be3c5786360ca4`、`a226393b3435b429b636ce7a90cb37714c43e845`、`127704c846ca82bdf85e9beb017f6c9e17a75901`、`0bf05e11c55288b0cd48b7bab2f0b78e7f333f32`、`61a40cf782848df7ecbc99a57eb8408190f7501d`、`02c62f71a2717ed59d264b0ff6cf482f2517a653`、`9fe4b3a0833b4aecb0f855a14fad13f8ce6e5879`、`1407f77f0fec8ff08c5aa44cb601ba2dff4d015f`、`3864d4b2dd66d5a6eb914e2947c4b04a8327da2a`、`69d90ddafb9abeb38f420039909846f058258d45`、`1ccf979714681273992c127618cfda495f737af0`。网站设置相关为前三个产品交互提交及 `1ccf9797` 提示修订；监控分组和测试为 `0bf05e11`、`61a40cf7`、`02c62f71`、`9fe4b3a0`、`1407f77f`。
- 明确未纳入的分支、文件或后续事项：未纳入来源分支中不属于最终候选的中间提交；未改 AI 助手、手机终端、服务端认证实现或配套脚本。跨 Panel→Agent→`kejilion.sh`→真实 Nginx 的完整主机旅程未在 RC6 公开验收中重新执行。

## 外部审计与修复交付

- 安全盲审补漏（quality-audit review 档）：本次未执行独立盲审；没有 CF 安全边界改动，因此未触发 Cloudflare `security-boundary-audit`。候选 OCR 和独立代码复核不是盲审，不能替代盲审结论。
- `node scripts/security-review-scope.mjs --base v1.29.0` 输出：`recorded groups=16 invalid_trailer=true`，16 个敏感源码组均显示 `recorded`，未报告 `missing` 或 `stale`。`invalid_trailer=true` 来自若干无 `scope=` 的 `Security-Review: skipped reason=...` 元数据（包含 RC6 的前端分组/版本身份提交）；本命令退出码为 0，但 trailer 格式问题仍是治理遗留。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本次 RC6 差异没有确认的安全漏洞 finding。OCR 1.12.11 对 `6c95feb5e0b99796cd5d335a4c9a2a53939c6e7b..b347d859aca56de78668cdc0e97e871dd1715350` 覆盖 18/18 可审文件，H0/M0/L0；3 个排除项均记录原因。非盲自由审查 finding=0，`constrained-only=unreported`；独立清洁会话代码复核通过。最终 `1ccf9797` 相比该复核树仅追加说明性提示及记录尾注，候选 CI 和 L3 通过。
- 修复交付状态：RC6 本次 UI 改动已发布；没有新增服务端安全修复，稳定版尚未包含本 RC6 差异，生产未部署。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：工具 `1.12.11`，RC6 上述候选差异，18 个可审文件已覆盖、3 个排除项、0 findings；非盲，`constrained-only=unreported`。本记录只陈述单次覆盖，不推断工具有效性。
- 按 PROJECT_RULES.md 5.5 记录本稳定周期的 OCR 观察结果：本条是一个 RC 候选记录，不能单独用于判断工具长期有效或退出。
- 候选分支的原始 tip、归档位置和远端核验：见本模板前部候选处置字段；来源分支暂不归档，预览候选保留。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`。
- 变更集编号：不适用（本版未变更配套脚本）。
- KPanel 实际内置脚本基线 commit / SHA-256：`1faee13f3faef164efbfa899e244fc19ad58aaff` / `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：RC6 Release/L3 的 `managed_script_contract=pass`，公开镜像中的脚本 revision/hash 与登记值匹配；RC6 未改脚本。
- 本版发布决定：脚本不在范围；不重新发布脚本。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 网站设置、独立保存与监控分组定向测试；候选 CI、主线 CI；公开镜像初始/重启验收通过。 | RC6 没有变更双端 API；完整真实 Nginx 主机链路未重跑。 |
| 网络入侵与供应链安全 | 已验证（限本次差异与发布产物） | 候选/Release 扫描、镜像契约、SBOM/provenance 与脚本哈希检查通过；公开产物复验通过。 | 本版未执行盲审；`security-review-scope` 有 scope-less skipped trailer 元数据遗留，详见审计章节。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3、候选 CI、主线 CI、Windows Node 和同一公开镜像恢复验收通过；无数据库迁移。 | 公开浏览器第一次导航有网络变化故障，原工作流仍失败；同一产物恢复验收通过，详见流程异常。 |
| 性能与资源预算 | 不适用 | 本 RC6 增量为前端设置和监控布局，没有后端采样或资源预算改动。 | 未执行长时间 soak。 |
| 用户体验与可访问性 | 已验证（限定范围） | 本地 375×812 模拟预览无横向溢出；公开更新弹窗在 320/390/1280 宽度通过；设置交互及分组显示测试通过。 | 无实体手机、全应用 100%/125%/200% 缩放及完整键盘/焦点矩阵。 |
| 数据、配置与迁移 | 不适用 | 没有 schema、持久化格式、API 协议、端口或 Compose 变化。 | 不适用。 |

## 自动门禁

- 定向测试及结果：RC6 最终修订后 `SitesView.settings.test.ts` 定向 2 项通过；typecheck、i18n（4082 strings / 30 catalogs）和生产构建通过。最终小修前完整 Web 测试为 348 个文件 / 3531 项通过；候选 CI 对冻结候选复验通过。
- `make verify-release` 环境和结果：GitHub-hosted Ubuntu 24 / amd64，L3 run `42263516340` 通过；候选工作流完整 L3 门禁通过。工作区完整 L2 运行记录见该 run 的证据工件。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run `42263516340` / attempt 1；入口脚本 SHA-256 `181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3`；Runner image `ubuntu24 20261101.360.1`，不可变 Runner ID `1000005103`；Go `1.27.3`、Node `24.22.1`、Docker `28.0.4`；2026-11-06T17:39:28Z 至 17:59:40Z，状态 passed；证据目录 `C:\GitHub\_task-evidence\kpanel-rc6-release-20261107\l3-publication\l3-42263516340-1`。计划/脚本/bundle 单独 SHA 未记录；L3 source timeout 未触发，配置限值未记录。
- 候选 CI：run `42261420660` success；依赖 freshness run `42261420377` success。
- 主线 CI：`42263516340` 的 main-ci verify 与 Windows Node 均 success；promotion 将 `main` 与候选提升到精确候选 SHA。
- Release workflow：`Publish preview` run `42263516340`。inputs、L3、candidate CI、promote、main CI、Windows Node 与 Release job success；`public-acceptance/verify` 在 `terminal-first-navigation` 遇到多次 `net::ERR_NETWORK_CHANGED` 后失败，`preview-completion` skipped，因此原外层 run 终态为 failure。后续只读 `Verify published release` run `42273795677` success，复核同一候选、同一不可变镜像摘要与原发布产物；未重建或重复发布镜像。
- 发布说明门禁：最终正文版本 `1.30.0-rc.6`；摘要 2 条，升级注意事项 1 条，另有升级说明；公开更新弹窗验收通过。
- 安全扫描、镜像契约、SBOM/provenance：Release/候选 CI 扫描通过；双架构 OCI index、镜像运行契约、SBOM/provenance 与版本标签核对通过；发布镜像内受管脚本 revision/hash 匹配。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：本次没有依赖图变更；RC6 依赖 freshness run `42261420377` success。独立 `make dependency-report` 的生成时间未记录。
- 最近每日安全通告审计、EOL 复核状态及证据：RC6 未单独重跑每日/EOL 全量审计；无直接依赖或锁图变化，沿用 RC5 的依赖审计记录，当前新鲜度门禁通过。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：RC6 无新增依赖行动项；RC5 已记录的行动项沿用，未在本 RC6 差异中扩展。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：未引入新的依赖、基础镜像、Action 或脚本候选；L3 工具链如上。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：`web/package.json` 与 `web/package-lock.json` 只有项目版本元数据更新；依赖图不变。产品 index digest 为 `sha256:34a6c473854441f6c54d8cf55e9c597ec2a2fdd28d657cd0f4718cadf7aca67f`；脚本 commit/hash 见跨仓库章节。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无新增依赖候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：候选 CI、L3、主线 CI 与发布镜像验证通过；没有依赖升级或性能资源变化，可回到 `v1.29.0` stable digest。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Linux `ubuntu24` / amd64；容器镜像同时验证 linux/amd64、linux/arm64。
- 环境策略 ID 与允许用途：`environment-policy.json` 中的 `github-hosted`；本次使用 `candidate-validation` 与 `browser-validation`，仅用于隔离容器、更新流程与公开网页验收，不是生产主机。
- 使用的精确候选或公开产物：候选 `1ccf979714681273992c127618cfda495f737af0`；公开 RC6 OCI index `sha256:34a6c473854441f6c54d8cf55e9c597ec2a2fdd28d657cd0f4718cadf7aca67f`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `42263516340`，job `128049445126` passed；公开恢复 run `42273795677`，job `128084821121` success，退出码 0；浏览器报告 45 项、升级报告 31 项、真实隔离容器初始/重启分别 48/26 项通过，0 errors。初次失败与恢复证据分别在 `C:\GitHub\_task-evidence\kpanel-rc6-release-20261107\public-acceptance-failure` 和 `...\public-acceptance-success`；L3 命令 `scripts/verify-change.sh`；入口 SHA 如上；超时未触发。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单轮 L3 与公开镜像初始、重启、发布页和升级旅程；无 soak，因 RC6 无后端/Agent/数据面变更。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：RC6 设置与监控界面在本地模拟数据预览核对；本地 375×812 无横向溢出。公开更新弹窗检查 320、390、1280px，稳定通道保持 `v1.29.0`，预览显示 RC6 和 digest；浏览器 published/upgrade 测试通过。完整应用缩放、实体手机、全局键盘/焦点矩阵未执行。
- 宿主机写入、失败注入、重启恢复和回滚结果：没有连接或写入宿主机；公开隔离容器初始/重启检查通过；本次无生产回滚。
- 未执行场景及原因：完整 Panel→Agent→脚本→真实 Nginx 主机联动、实体手机和全应用缩放矩阵未重复执行；RC6 是前端交互与布局差异，脚本及后端协议未变。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：`1.30.0-rc.6`、摘要和 digest 一致；正文有 2 条变更摘要、1 条升级注意事项；恢复 run 的 published/upgrade 浏览器分别 45/31 项通过，更新弹窗在 320/390/1280px 无横向溢出。
- GitHub Release 的 draft / prerelease / Latest 状态：`isDraft=false`、`isPrerelease=true`，2026-11-06T18:40:05Z 发布；GitHub Latest 仍是 `v1.29.0`。[RC6 Release](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.6)
- Docker 版本与通道 OCI index：`1.30.0-rc.6` 与 `preview` 均为 `sha256:34a6c473854441f6c54d8cf55e9c597ec2a2fdd28d657cd0f4718cadf7aca67f`；Docker `latest` 未变，仍为 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:5efc909b910b6503561c35d0410e2ea8d992c0f4502185f0944b4e51221c24f1`；arm64 `sha256:a1d12c3f189fae9cb4bdf0b7528b41c76289af84e69ba460f09276cd002dac4c`。
- 附件及 `SHA256SUMS`：GitHub Release 共 19 个附件；`SHA256SUMS` asset digest `sha256:fda9bb49ed2fc8389cf99ba173d2f31c5e6aa196975ffa8511fea0a56a396e5c`；恢复验收按原 Release 附件集合和摘要复核。
- 公开镜像 `image_e2e=pass`：同一镜像 digest 的隔离初始/重启检查通过，48+26 项，0 errors。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场稳定默认入口不变；镜像内 `kejilion.sh` revision/hash 与登记值一致。本版没有脚本发布。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：公开恢复验收通过；stable 显示 `v1.29.0`，preview 显示 `v1.30.0-rc.6` 与固定 digest。
- 加入预览只切换来源并立即检查，没有自动安装：published/upgrade 浏览器门禁通过。
- 自动安装开关与一次性立即安装相互独立：published/upgrade 浏览器门禁通过。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：隔离容器初始/重启和升级门禁通过。
- 退出预览且稳定版较低时没有产生降级候选：升级浏览器门禁通过。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：公开真实隔离容器与升级流程通过。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：公开更新界面与发布说明验收通过；未改变平台策略。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：GitHub-hosted 隔离验收；未连接生产或灰度主机。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：不适用；未连接 `arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用（预览版禁止生产部署）。
- 部署命令/入口：不适用（预览版禁止生产部署）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用（预览版禁止生产部署）。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3、镜像初始/重启、更新/回滚失败态与公开网页验收。

## 回滚

- 源码/tag：回滚参考 `v1.29.0`；RC6 使用不可变 tag `v1.30.0-rc.6`，不覆盖历史 tag。
- 镜像 digest：stable 回滚镜像 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- 数据/配置备份：本次未部署生产，无生产备份操作。
- 回滚步骤和回滚后复核：若隔离验收发现问题，按更新流程恢复先前镜像和数据；生产未执行回滚。
- 回滚后生产实际版本与健康状态：不适用（无生产部署）。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest 为 `v1.29.0`；Docker `latest` 保持 `v1.29.0` stable digest；应用市场默认稳定入口未变。
- 公共默认更新通道决策：不适用（RC6 仅 opt-in preview，不提升默认通道）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-07T00:43:18+08:00
- 候选冻结时间：2026-11-07T01:24:56+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（产品产物未回滚、重建或重复发布；只恢复公开验收）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "public-acceptance/browser/network-changed",
    "position": "before-production-write",
    "count": 1,
    "impact": "RC6 Release 与预览镜像已公开；首次公开浏览器验收在 terminal-first-navigation 阶段多次遇到 net::ERR_NETWORK_CHANGED，外层 Publish preview 终态为 failure 且 preview-completion skipped。没有证据表明产品本身失败，也没有生产写操作。",
    "recoveryEvidence": "原始 run 42263516340 的失败报告保存在 C:\\GitHub\\_task-evidence\\kpanel-rc6-release-20261107\\public-acceptance-failure；只读恢复 run 42273795677 / job 128084821121 对同一候选 1ccf979714681273992c127618cfda495f737af0 和 digest sha256:34a6c473854441f6c54d8cf55e9c597ec2a2fdd28d657cd0f4718cadf7aca67f 通过，真实容器 48+26、浏览器 45+31 项通过，errors=0；未重建或重发产物。",
    "permanentAction": "负责人 KPanel Release Maintainer；2026-11-14复核下一次公开浏览器门禁是否再次发生 ERR_NETWORK_CHANGED。退出条件：随后两次公开门禁无此错误；若复发，修复验收浏览器入口的有限传输重试并保留首次失败证据。该自动重试目前尚未实施。对照此前最近五个公开验收记录 v1.30.0-rc.5、v1.30.0-rc.4、v1.30.0-rc.3、v1.30.0-rc.1、v1.29.0，未见同一指纹；v1.30.0-rc.2 未公开。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：未执行；保留候选和任务工作树、公开验收失败/恢复证据及 OCR 记录，以便追溯和下一 RC 恢复。
- 未验证风险：全应用 100%/125%/200% 缩放和实体手机；RC6 未重跑完整 Panel→Agent→脚本→真实 Nginx 主机旅程；AI 助手未因本 RC6 差异专项回归。`security-review-scope` 的 scope-less skipped trailer 格式问题仍待治理。
- 已实现待实机准入：新设置入口在真实网站管理数据上的交互仍应在后续隔离主机体验验收中复核；不改变已上线访问密码基础能力。
- 不阻断本版的理由：RC6 是 opt-in 预览；产品差异为前端交互/布局，候选、L3、主线 CI、公开同 digest 恢复验收通过，稳定默认与生产未变；首次公开验收失败已单独保留并透明记录。
- 后续应进入的自动门禁或专项工作流：修正 `Security-Review: skipped` trailer 统一要求 `scope=`；在下一轮公开验收跟踪网络变更是否复发，若复发再实现有界重试；下一 RC 复核来源分支归档和真实网站设置旅程。
