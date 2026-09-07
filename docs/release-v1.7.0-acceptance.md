# KPanel v1.7.0 发布验收记录

v1.7.0 已公开并通过 arena-154 标准升级及部署后验收。最终产品提交与正式标签保持不变；本记录是独立收尾提交。

## 范围、角色与版本

- 用户授权“v1.5.1 后的候选全部进入上线流程”。实查无v1.5.1标签，以v1.5.0起扩大盘点，剔除已进入当前稳定v1.6.1的祖先/等价提交；唯一发布负责人使用独立工作树 `C:/GitHub/_release-candidates/kpanel-v1.7.0-20260907`、`release/v1.7.0-candidate`，没有召回作者或创建复核任务。
- 基线 `758ec9ca93a4de45f8b84ce58e20cb39f5ebd1f6`；最终产品候选 `d0f248904edc0d63b1d00add1ca5c7e0113fcfb0`。自动异步检测扩展原手动行为，使用功能次版本1.7.0。版本四处一致，未改变依赖版本。
- 来源 f093fe1/9b6c986/57d92e4 → d014101/45e6094/aa42435：Docker后台静默检查、与应用市场共享有界只读检测、仅可更新时显示被动图标。
- 来源 dde18fe →898b6e6：撤下本机证书/容器资源提醒入口和主动采集/发送；保留历史配置/状态及普通集群CPU/内存/磁盘/流量/掉线/SSH/Telegram行为。
- b9255c9更新触及50提交阈值的业务事实文档，以当前main758ec9c/v1.6.1为基线；不修改治理阈值。4dcd9ad准备版本；d0f2489修正本轮发现的历史12px阈值输入为14px并补回归。
- 42文件差异；不新增System Center业务功能/页面/路由/API/权限，不纳入维护分支未提交 `.github/workflows/release.yml`。10个旧分支的patch-equivalent内容已上线，不重复cherry-pick。所有其他工作树、未提交内容保持原样；共享管理主工作树clean/cd6d554仅检查，不更新或重置。

## 跨仓库联动

- `scriptLinkageState=not-required`：无需发布脚本（不适用），不是延期。无新宿主产物/安装更新协议/外联配置/内置脚本内容变化，本次不推送sh/apps。
- 内置脚本commit `298f6f23751e36726660d73b5c0c83aef1b404f4`；raw SHA256 `80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581`；标准安装替换后SHA `e72034d75b24eae1484ddc4a0af437b109ea701e7a0c8679520006dca7c09539`。用途不同，不混用。
- packaging/kejilion-app/kpanel.conf与v1.6.1无差异，候选/本地apps/公开apps/main换行归一一致，无应用市场提交。

## 多维质量与边界

- 只读Docker检查20秒总截止/两共享槽位、无排队、冲突仅重试一次；不放宽mutation的resourceVersion。可信Engine本地index descriptor避免读取已经删除的旧远端index，缺摘要/不可比较不报current，错误分类不泄漏registry内容。
- UI至多200容器、1请求/秒、并发2、普通结果30分钟TTL、busy30秒；隐藏/窗口不活跃取消暂停，身份变化失效，刷新同身份复用。只显示18px绝对定位可更新图标，不自动pull/rebuild/update、不新增凭据存储或弹窗toast。
- 本机资源规则在读写API、定时tick、服务重建和暂停到期后均保持休眠；不采集、不重试、不发送恢复消息、不清空历史配置。普通通知继续工作，真实Telegram收件人未投递。回滚旧版可能恢复原已启用规则和待发通知，必须提示并成套恢复备份。
- 概览/系统工具既有异步拆分未改，保留加载顺序/partial刷新防闪动/未保存表单回归。本版无数据库迁移、用户文件写入或新增特权动作。

## 首轮与最终验证

- 初始4dcd9ad的L3通过但已被后续修正替代，不作为最终候选通过证据。通知browser-r1按14px操作控件规则正确拦截继承12px样式；这是发布前产品问题，不是生产变更失败。最小修正后重新冻结d0f2489并重新执行完整L3与全部浏览器绑定。
- 新字体回归首次因为jsdom下import.meta URL不是file协议而失败；改为Vite rawimport后5/5通过，首次错误保留。初期PowerShell inventory/tag-date/SSH参数诊断错误已修正，不用于任何成功判断。
- 最终L3 `v1.7.0-d0f2489-l3-r2`：2026-09-07T07:39:04Z→07:54:27Z passed/exit0；164治理测试、全量Go、特权race、vet、148文件1267前端测试、2208短语/21目录i18n、typecheck/build、govuln/npm/固定Trivy源码与最终镜像扫描、双架构二进制和安装更新回滚卸载生命周期全部通过；14项evidence摘要独立核对（标准11项加事先上传的3份功能夹具），负向生命周期日志是预期断言。唯一入口run-release-l3.mjs→run-release-gate.sh→make verify-release。Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`（Go1.26.7）；bundle `fe9aff7e2ffd5964af94595efd8ea7252471afa406be9c6d922cb66540273b04`、plan `16b27c0a9299adcbe22195e853ccc3a462324066e08ddbdc45b4bc410f567ad3`、remoteScript `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- exact候选ci.yml/release.yml经arena已有PyYAML6.0.2解析通过，无本地临时安装。既有固定Actions、双架构、draft→OCI成功后公开、稳定latest提升顺序未改。验收解析器14测试通过，额外时间预检验证秒/毫秒接受、7位拒绝；所有生产机器字段使用Git ISO/实际UTC秒或Date.toISOString，原始纳秒时间保留区块外。

## 浏览器与隔离真机

- 本地canonical mock acceptance/visual-composition，环境local-release-v170；真实Chrome152.0.7977.82、Playwright1.62.1。均有硬超时、资源watchdog及cleanup；不借生产账号，不冒称生产全功能浏览器验收。
- Docker最终job23500，v170-docker-browser-r2，07:36:45.396Z→07:37:29.062Z passed：1280light zhCN/768dark en/390dark zhTW/1280桌面，根文字100/125/200%（不是浏览器菜单zoom），四状态只出现一个被动badge，列表高度不变、点击无操作、刷新无重检；Apps中英文current/available/fixed/unavailable一次请求且无重复清单刷新。pageerrors0，预期失败网络响应不代表全部浏览器网络日志为零。
- 概览最终job49456，v170-overview-browser-r2，07:36:45.460Z→07:38:16.507Z passed：九场景、经典/桌面各两个真实20秒刷新周期、未保存输入、冷启动顺序、错误重试、静置几何稳定，regressions0/resourceFailurefalse/cleanuptrue。
- 通知最终job39312，v170-notifications-browser-r3，07:41:16.652Z→07:41:20.716Z passed：三宽度、明暗、键盘Escape/Enter、CPU85保存后重开读回、保留普通通知、无本机提醒/旧资源暴露、PUT不传resourceAlerts、计算字号14px、无水平溢出、滚动后实拍；console/pageerrors0，cleanuptrue。此前r2已成功，r3仅补输入可见截图与console检查，不是修复第二次产品失败。
- 真Docker29.6.2 fixture `v170-real-docker-d0f2489-r1` passed：独立loopback registry/零层测试镜像/自有容器，current→本地tag移动→available；原容器ID与镜像不变，测试容器/新旧镜像全部清理。镜像固定/不可比较/共享HTTP错误契约由自动测试覆盖，不虚称本fixture覆盖这些路径。脚本SHA `14f4e9cd1f098bfe1bc9cb1e56cefda1a5650acfecce47e307b58a19bfcd511d`。
- 通知持久配置/状态重建、正常/异常资源、25小时暂停到期、多次tick、历史retry/recovery冻结及普通CPU/Telegram假传输保留由真实Linux Go测试验证；NewService重建不是OS整机重启/真实Telegram投递。

## 性能与例外

已在最终L3结束后顺序运行v1.6.1与d0f2489私有Agent样本，固定fixture-r2 SHA `d5f91159583419830282fd520f915493b9c8ff62fa0001f7604b270819ed6c94`。单CPU/256MiB/128PID、独立状态/token、标准sandbox、system writes=false、公网lookup=false。每轮24次summary、前4次预热后20样本：P95 784.074650→743.136950ms(-5.221%)，CPU14089435→13961579us(-0.907%)，peakRSS27.550781→31.152344MiB(+13.072%)，候选idle20.707031MiB，OOM/max0，全部读取成功且私有unit/token已清理。候选L3 Agent SHA bff3f5689adbd91733c299e4f818c784b3d7cf2b05a6fd54e9ab1fd62bcb83e7；旧Agent来自实际OCI而非GitHub附件。

旧fullsummary250ms默认未达标且不修改默认；本版未改变该后端聚合，实际含CPU窗口/固定管理探针，任意延时或回退异步拆分不是合适修复。独立runtime20个预热后样本P95159.165284ms达标；config6样本1.407–1.760ms、SSH296.193–381.161ms、BBR183.605–212.819ms只报告范围，不冒称小样本P95。本轮发布负责人依据新证据复核，仅旧fullsummary<=1000ms、P95/CPU/peakRSS相对<=20%、idle<=32MiB/readpeak<128MiB、error/OOM0的有限例外成立；有效至下一稳定版复核或2026-09-14较早者，不自动续期。退出条件为完整聚合达到250ms或重新具备证据的评估；任何正确性/安全/恢复/资源失败取消。小时级soak和更慢主机未验证。

## 依赖复核

- 同SHA candidate freshness34098182962成功，07:59:22.147Z，8/8源完整；行动项emergency0/patch7/minor14/major3，123项传递归属信号。只表示稳定通道候选，不自动采用；本轮不夹带依赖/工具链/Action升级，不重置原首次检测的启动/决策/处置期限（安全1/3/3天、patch7/14/30天、minor14/30/60天、major30/90/90天）。当前负责人2026-09-07复核，无新增延期例外。
- 新每日security-advisories34095910017在基线758ec9c成功（07:31:47Z），实际govuln可达0/npm0/源码扫描通过；不代替当前d0f2489的L3/CI扫描。Go另3项未调用模块通告，不能宣称所有依赖零通告。EOL复核current，下次2026-10-28。

## CI、正式产物与生产

- 候选 [CI34098183071](https://github.com/kejilion/KPanel/actions/runs/34098183071) / [freshness34098182962](https://github.com/kejilion/KPanel/actions/runs/34098182962)、主线 [CI34098598021](https://github.com/kejilion/KPanel/actions/runs/34098598021) / [freshness34098598014](https://github.com/kejilion/KPanel/actions/runs/34098598014) 全部success，均精确d0f2489。每次推送前重读main基线，758ec9c安全快进至d0f2489，无冲突和其他会话提交混入。
- annotated tag对象 `aafade988e26f02643908f754760635ea6f3c9dd`，peel为d0f248904edc0d63b1d00add1ca5c7e0113fcfb0。[Release34099239725](https://github.com/kejilion/KPanel/actions/runs/34099239725) / [tag freshness34099239744](https://github.com/kejilion/KPanel/actions/runs/34099239744) success，单次正式发布、无重发/移动旧tag；候选远端分支由发布工作流确认包含后删除。
- [GitHub v1.7.0](https://github.com/kejilion/KPanel/releases/tag/v1.7.0) 于2026-09-07T08:20:24Z公开，非draft/非prerelease，8附件完整。Release原生镜像扫描及256MiB/1CPU/128PID/非root/只读根/cap-drop ALL运行契约通过，双架构SBOM/provenance完整。
- `kjlion/kejilion-panel:1.7.0` 与 `latest` index均 `sha256:913bcac5f1344930f6fb87084d32453dea32b4932ef2c400c09a8ea3a96a266f`；linux/amd64 `sha256:a205eaf1d8c65df8c404bc206032cc8ed944c23f044066a9d63311d82cba1205`，linux/arm64 `sha256:f7de95f9e82d1dd37186cb2191dd890c091c3d2ea884b2c2ea7af16a2397297c`，两份unknown平台为attestation，不是缺架构。
- 八个公开附件实际下载，与GitHub digest/size及SHA256SUMS核对。Agent amd64 `70c0b19d48ce73c08dceae8192b0e3ce14a6e61a6bbe6a5c62ae15ff0073f6e5`、arm64 `2a6ce0fc2fdfc50f368e4270068f88c607a9e1bc0cc0ba765fcef2f670c960a1`；node amd64 `62099f2d96bcde0cab3ecf84de3610393f4a0e080579e485f580b0db074dce5e`、arm64 `037ef0aa2119c8b308cb8472c8e185b3acc34cffa1d604762d6e69b9242353db`；deploy `dd903474972c1a40786c7f812cd87ac2e1408a70e33b27896e17dc63eafad879`。LICENSE/THIRD_PARTY_NOTICES与候选源码一致。
- 从公开index显式拉取并核对revision/version/script pin/非root65532:65532，`image_e2e=pass`，08:21:44Z完成。证据 `v1.7.0-public-d0f2489-r1` 本地及远端同名根保留；不是本地L3镜像复用。
- 正式部署仅arena-154。108/prod-108禁用全部KPanel操作，本次未连接、未核对、未备份、未部署。

- 固定production run `v1.7.0-production` preflight07:26:14Z→07:26:16Z passed，线上1.6.1/ok/initialized、Panelrunning/healthy/restart0/OOMfalse、Agentactive/running/enabled/NeedDaemonReloadno、panel/ai.db quick_checkok；5项顶层证据hash通过。没有把未列入的snapshot称作顶层hash覆盖。
- 生产脚本固定SHA `129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`；正式备份与postdeploy使用同runID和独立目录，标准应用市场update唯一入口。公开验证脚本SHA `adcef86aba977e3960b380eadb29919286ff4b2bff662b54ac314af8beb19bd1`已完成语法/hash预检。
- 回滚基线v1.6.1/2f926d6e4d6a2e0643f0f0c60d38f9b2feeefbe2；OCI `sha256:4d64cf52dc093c5f17f8bb9e77e736ecbe0632a9bf2dcc271ebafbd099b9126c`。本轮新停写备份已完成，不复用上版备份。回滚恢复镜像/Agent/脚本/数据/密钥/配置并复核公共默认分发，不仅切换镜像。


- 首次生产写为2026-09-07T08:22:09Z。新备份 `/root/kpanel-backups/pre-v1.7.0-20260907T082209Z`，backup08:22:09Z→08:22:19Z passed；停写归档、6文件摘要/结构、旧镜像实际load和旧服务恢复通过，8项顶层证据摘要独立核验。kpanel.tar.zst `5ebf7d5218878a6b91be7bfd358eea0774888aa75f447bbad42008a48b51ba9e`，old-image.tar.zst `6ad5c3f4479b451567dcc6c38d283b71e4de65762f584a1816ec287d649c8427`。
- 标准入口 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel` exit0，实际拉取公开latest913bcac5摘要；无自制生产wrapper。备份恢复/更新中的短暂curl reset、TERM和daemon-reload过渡提示在固定入口正常收敛，无单独重试或遗留故障。
- postdeploy08:23:35Z→08:23:36Z passed/exit0，1.7.0/ok/initialized，OCI revision d0f2489与index913bcac5一致。Panelrunning/healthy、restart0、OOMfalse；Agentloaded/active/running/enabled、NeedDaemonReload=no。protected.diff为空，panel/ai.db quick_checkok，顶层ai.db空如实记录；近10分钟致命日志检查通过，8项顶层证据摘要独立核验。原始health.checkedAt纳秒值08:23:35.037179178Z保留原文件，不写入机器时间块。
- 本地证据 `C:/GitHub/_release-artifacts/v1.7.0-production-{preflight,backup,postdeploy}/remote-evidence`；远端 `/root/kpanel-release-evidence/v1.7.0-production/production-{preflight,backup,postdeploy}`。生产只执行标准备份/更新及访问策略恢复，没有用户文件操作、Telegram发送或生产故障注入；未回滚。GitHub Latest、Docker latest及标准更新当前指向健康v1.7.0，公共默认通道回滚决策不适用。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-07T15:23:56+08:00
- 候选冻结时间：2026-09-07T15:35:59+08:00
- 生产完成时间：2026-09-07T16:23:36+08:00
- 提交到生产用时：0.994444 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {"fingerprint":"acceptance/run-repo-bash/unsupported-env-argument","position":"after-production-write","count":1,"impact":"生产已健康后，收尾L0首次错误地通过launcher的--env传VERIFY_BASE_REF；入口白名单只允许VERIFY_LEVEL，在启动任何Bash前拒绝，未影响生产","recoveryEvidence":"读取实际parseRunArguments后，VERIFY_BASE_REF通过进程环境传递，CLI仅--env VERIFY_LEVEL=0；同一仓库入口L0通过，验收及58tag覆盖校验通过；原始拒绝保留","permanentAction":"已纠正本轮调用且复核既有参数白名单回归，不为调用错误扩大白名单。识别v1.4.1同指纹复发：当前发布负责人必须在下一次L3生产写前预检完整收尾argv及环境（不仅时间格式）；退出条件为同一入口完整调用和正负参数回归提前通过。此项未冒称通用预检已经固化，本版不重发产品或改线上。","historicalReleases":["v1.4.1"]},
  {"fingerprint":"preflight/agent-fixture/artifact-provenance","position":"before-production-write","count":1,"impact":"性能基线首次预检错误地以GitHub附件dd3310摘要匹配应用市场从OCI提取的Agent，assert在创建任何私有unit之前停止","recoveryEvidence":"安装Agent与运行中immutablev1.6.1镜像/release/kejilion-agent经cmp一致，均fb45c9542030f112c2daa0d00a96d42e18752108fb517f20956824b67b61c5e4；镜像revision2f926d6/version1.6.1一致，Go metadata证实OCI(devel)与GitHubv1.6.1+dirty/VCS字段不同；修正来源绑定后顺序重新采样","permanentAction":"唯一外部性能fixture-r2同时断言生产imageID、OCIrevision、从镜像提取的原件与安装二进制摘要，不使用GitHub附件摘要替代OCI产物；新脚本d5f91159583419830282fd520f915493b9c8ff62fa0001f7604b270819ed6c94已语法/hash预检，旧失败保留","historicalReleases":[]},
  {"fingerprint":"preflight/release-diagnostics/shell-arguments","position":"before-production-write","count":1,"impact":"早期inventory foreach-pipeline、annotated tag日期和SSH参数承载诊断同类批次无效；不使用错误输出判定候选或运行时","recoveryEvidence":"显式收集对象后序列化、tag peel提交日期和正确单一远端参数后，完成main/候选盘点及PyYAML6.0.2精确工作流解析","permanentAction":"本轮发布负责人固定无拼接L3/生产入口；所有实际发布参数已预检，后续沿用同一参数形式，不以现场修正冒称通用仓库治理已新增","historicalReleases":[]},
  {"fingerprint":"testing/notification-fixture/jsdom-file-url","position":"before-production-write","count":1,"impact":"新增字体回归用fs读取jsdom的非file import.meta URL，首次定向测试失败","recoveryEvidence":"修正为Vite rawimport后5/5通过，最终候选d0f2489重新全量验证","permanentAction":"d0f2489将原始Vue样式回归读取固定为既有Vite rawimport契约，保留14px断言及浏览器计算字号验收","historicalReleases":[]}
]
<!-- kpanel-release-process-incidents:end -->

已比较v1.6.1/v1.6.0/v1.5.0/v1.4.1/v1.4.0；前三类指纹无精确历史重复，收尾unsupported-env-argument与v1.4.1复发，明确保留下一次L3前置完整调用预检要求。v1.6.1生产后timestamp精度失败已在本轮生产写前做正负预检，不更改校验器。12px产品问题为正常门禁拦截，不自动计作流程异常或生产事故。所有首次失败、替代候选和旧证明保留，不伪称全链首轮成功。

## 清理与未覆盖

当前保留全部原始证据和恢复路径、其他会话dirty内容；旧r1自有preview已停止，最终preview也已正常停止，4174端点无法访问。此前本机删除被平台拒绝，本轮不换工具重试删除；实际净释放0。远端临时fixture自有资源按断言清理，生产备份保留。没有小时级soak、真实Telegram收件人投递、所有平台辅助技术或生产用户业务写的测试声明。


复用现有发布、预览、后台浏览器工作流；未创建新的规范、平行发布系统或记忆文档。正式产物和生产均只有本次一个版本；最终记录按仓库验收格式和时间预检约束生成。
