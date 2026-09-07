# KPanel v1.8.0 发布验收记录

## 发布画像与范围

v1.8.0 已公开，arena-154 已完成标准升级并通过部署后验收。本记录是独立收尾提交，不修改正式标签或产品产物。

用户授权“v1.7.0 后的候选全部进入上线流程”。发布负责人自行盘点所有 refs/工作树及候选证据，未召回作者、未创建新任务。治理先独立复核与合入；产品采用兼容性功能次版本1.8.0。

- 最终产品候选 `435ae91f25de24e997aabb3414bf60f1e7eb1e10`，冻结2026-09-07T18:40:32+08:00，独立工作树 `C:/GitHub/_release-candidates/kpanel-v1.8.0-20260907`，分支 `release/v1.8.0-candidate`。产品基线 `1461f288a71cdf934d12f9665491cf5cdb671c56`。
- 治理来源39ac7f8→1461f28：release.yml四处Docker Action固定SHA升级。candidate CI34112083012/freshness34112082911、main CI34112442900/freshness34112442790均成功后已快进main。
- 产品来源c56952a→c6438dc：浮动桌面窗口允许部分移出左右/下方视口，至少保留200px可找回标题区域和42px标题栏，避开72px任务栏；初始层叠、吸附、最大化、窄屏行为保留。
- 产品来源dd1aa80→11c6a41：已安装应用后台静默检查镜像更新，与Docker复用共享有界缓存；仅available显示18px被动图标。手动检查复用结果及分类错误，静默失败不toast、不自动pull/rebuild/update、不刷新整张清单。
- dcd5dec跟踪可重用桌面浏览器入口，435ae91更新四处版本与CHANGELOG。
- v1.7.0至候选21文件差异中包括上版收尾记录cc3d15；它是既有main基线，不重复发布业务。f093fe1/9b6c986/57d92e4等旧提交经git cherry确认为等价已上线，未重复移植。冻结后作者新活动不默默纳入。
- 本次没有新增System Center页面/路由/API/权限/业务文档；共享管理主树及15个其他dirty工作树保留，不reset/clean/stash/强推。未提交内容及旧等价候选不属于本次有效交付候选。

## 跨仓库与契约

`scriptLinkageState=not-required`：无需发布脚本（不适用），不是延期。跨仓库变更集/脚本候选不适用；不推送kejilion.sh/apps。

实际内置脚本revision `298f6f23751e36726660d73b5c0c83aef1b404f4`，raw SHA256 `80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581`。候选与v1.7.0的internal/agent、internal/panel、internal/dockerx、internal/appmarket、packaging、Dockerfile、go.mod/go.sum无差异；API/数据格式/端口/Compose/Agent权限/安装生命周期未改。

2026-09-07T10:53:45Z公开apps/main/kpanel.conf与候选换行归一全文相同，SHA256 `7b5b52af0ff20cff4bebf114e747ddf1c82996500f2767ba8d3733217e83121c`。仍用latest与OCI标签动态版本校验，无需应用市场提交。

## 质量与验证边界

业务正确性：两组前端旅程通过自动/真实Chrome模拟API验证；真实生产浏览器用户数据写操作未执行。安全：不增加特权动作、凭据存储或网络协议，错误信息继续分类不泄漏registry内容。稳定性：隐藏/不活跃取消，身份变化失效，手动检查缓存同步，窗口持久恢复与吸附保留。

性能/资源：共享检查至多200条、并发2、启动间隔1秒、正常TTL30分钟/busy30秒、AbortSignal25秒；Agent原有共享两槽/20秒截止不变。浏览器断言刷新同身份不重检、应用清单额外刷新0、状态变化前后卡片高度不变。没有小时级soak或慢主机全面性能声称。后端fullsummary未改，本次画像不重新测整机聚合P95，不将上版例外自动续期或旧样本冒称本版通过；新公开镜像资源契约仍为必测。

数据/迁移不适用：无数据库/配置协议迁移或用户文件写入。桌面已存位置可回滚到旧clamp行为；无新增数据格式。可访问性包含主题、语言、键盘/焦点、被动图标role/aria/title和窄屏；未覆盖所有辅助技术。现有桌面标题12px未改，不声称全站最小字号达到14px。

## 浏览器与首轮情况

唯一canonical mock acceptance/visual-composition preview绑定435ae91，环境local-release-v180，loopback4176。Chrome152.0.7977.82，独立后台作业180秒硬超时、RAM512MiB/磁盘1GiB watchdog及cleanup。页面模拟版本v0.16.0来自mock API，不是部署版本；通过Git HEAD与preview manifest确认实际候选代码。

- Docker/Apps首轮v180-docker-browser-r1，job51996，10:42:19.344Z→10:43:36.359Z passed，8场景，pageerrors0，cleanuptrue。1280light zhCN/768dark en/390dark zhTW/1280desktop；根字号缩放100/125/200%，并非浏览器菜单zoom。四状态仅available被动图标、点击无写操作、行/卡片高度不变、缓存复用、自动请求4次和手动分类检查、无额外清单刷新。
- 桌面r1六组几何均通过，但缺少mock公网信息和网站appearance/icon三个GET接口产生404，console断言正确拦截。外部窄范围preload复用overview-presentation已定义的相同响应；不改候选、不屏蔽意外错误。r2 job51112，10:49:38.363Z→10:50:12.430Z passed/exit0，spec SHA `45a9a773798b1131d01b28b7f366afdf397c7b6df0de590f18c7d0ebd9dd127a`。1920/1280/768及1024有效125%场景验证移出左右下方、拖回、刷新恢复、左右吸附/最大化、Tab焦点；390/640有效200%窄屏拖动保持原状；console/pageerrors0，cleanuptrue。
- Docker补充console审计r2原错误分类只接受预设502，误判mock原有444…容器503超时，8组功能断言仍通过但作业exit1。修正外部审计为仅允许明确URL的两类预设失败响应，r3已通过，详细结果见发布前补充证明。没有删改首轮日志，不声称全链首次成功。

## 依赖与供应链

setup-buildx-action4.3.0 `37fe631027851001ddb9b187196cc803df7f5f0e`；build-push-action7.3.0 `53b7df96c91f9c12dcc8a07bcb9ccacbed38856a`（两处）；login-action4.6.0 `dbcb813823bdd20940b903addbd779551569679f`。独立SSH读取官方tag与raw action.yml，node24及post实现匹配；输入/权限/密钥作用域、Buildx0.34.1与BuildKit0.30固定镜像未改。产品依赖及扫描器未改。

164项治理与9组依赖策略检查通过。产品同SHA freshness、每日security-advisories/EOL及公开Release证明见下文；既有行动项期限不因本次候选冻结而重置。Go scanner发现3项未调用模块通告，不声称所有依赖零通告；当前可达漏洞0/npm0/源码扫描通过。

## 固定门禁与生产安全预检

L3唯一入口run-release-l3.mjs→run-release-gate.sh→make verify-release，run `v1.8.0-435ae91-l3-r1`。不可变Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，Go1.26.7/Node24。bundle SHA `c13c57cf8914d79522df2fbb879c45cdf79299a4d8933c0f94a383d925d00b97`，plan `c10fd424ec9d66f129f45e21e1e4eea8cd5fc1e37958a66c4bb443af8935fb8a`，remoteScript `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。全量Go/race/vet、148文件1280前端测试、2208短语/21目录、typecheck/build已通过，L3最终passed，终态和摘要见下文。

生产目标仅arena-154，environment-policy允许candidate/browser/performance/production用途；108/prod-108全部KPanel操作禁用，本次未连接/核对/备份/部署。production run固定 `v1.8.0-production`，preflight10:39:50Z→10:39:51Z passed，实际1.7.0 healthy。随后新备份、标准update与postdeploy全部完成，精确结果见下文。

已在首次生产写前执行完整收尾argv/env预检：继承VERIFY_BASE_REF=HEAD，CLI仅`--env VERIFY_LEVEL=0 -- scripts/verify-change.sh`；错误`--env VERIFY_BASE_REF`负向断言拒绝，正确参数与环境通过；真实L0在435ae91通过。秒/毫秒时间正例、7位小数负例通过；验收CLI与58tag coverage实跑通过；backup prepare-only通过。此处修复v1.7.0/v1.4.1反复调用错误，不放宽仓库白名单、不冒称新通用治理已采用。

公开产物验证脚本已本机/arena bash-n通过，上传前后SHA `f3ac21d0f9427e3504a07a206056f7954b719a550baddd766dbbda61bbf597eb`一致；必须在正式Release后下载真正公开附件/OCI重新验收。GitHub附件与OCI提取的Agent来自不同构建，不互作摘要断言。

## 回滚

### 发布前补充证明

产品候选[CI34114123024](https://github.com/kejilion/KPanel/actions/runs/34114123024)/[freshness34114123032](https://github.com/kejilion/KPanel/actions/runs/34114123032)和主线[CI34114566153](https://github.com/kejilion/KPanel/actions/runs/34114566153)/[freshness34114566163](https://github.com/kejilion/KPanel/actions/runs/34114566163)均success且精确435ae91。主线从1461f28安全快进435ae91，推送前重新读取远端基线，无冲突与额外载荷。

annotated tag `v1.8.0` 对象 `022f0dceb377245d549b1b9c902ff8b6414e8820`，peel435ae91；主线CI于11:07:55Z成功后核对main/候选同SHA再创建并推送，未移动旧tag。[Release34115049161](https://github.com/kejilion/KPanel/actions/runs/34115049161)及tag freshness34115049075全部success；发布工作流确认候选包含于tag后自动删除远端候选分支，主线仍435ae91。

L3终态passed/exit0，2026-09-07T10:41:58Z→10:56:19Z，全部双架构、固定Trivy最终镜像扫描与rootfs安装/更新/回滚/卸载生命周期通过。11项evidence.sha256已下载逐一核对；生命周期错误日志属于预设负向断言。最终L3日志SHA `3f3563f9f288b5a50f7a7b2da22eccc4166ba206d09a0dbce39088ec7983ba98`。本地/远端目录分别 `C:/GitHub/_release-artifacts/v1.8.0-435ae91-l3-r1/remote-evidence`、`/root/kpanel-release-evidence/v1.8.0-435ae91-l3-r1`。

Docker补充console-r3 job30092，10:54:22.916Z→10:55:39.024Z passed/exit0；8场景、pageerrors0，12条均为明确预设失败响应（4条指定444容器503、8条Apps502），unexpected0/cleanuptrue。spec SHA `6b208dc398c79430d93ea1d36f8c9bc2f01e1d9f564ca3ed884dd1e3931b31f6`。preload SHA `591bf0a2d800be1b032b5bd0d51488d066415245bb39557402dc837bdd563d83`；共享三路由mock preload SHA `c4b20a3da1adab7f2e9ff14bd4f6d1586a1858ff84b135d875b5dfc751ded87a`。前述r2分类错误已经同候选完整复跑修复，不隐藏预设网络错误。

候选freshness34114123032已成功，报告生成10:57:56.813Z，8/8源完整；direct/foundation行动项emergency0/patch7/minor11/major3，传递归属信号123。三项Action候选已消化，其他上游版本信号不是本地已交付候选，不自动扩入本次范围，也不重置首次发现期限。最近每日security-advisories34095910017在758ec9c上07:31:47Z→07:32:55Z成功，24小时内；不代替当前435的L3扫描。EOL最近2026-07-28，在92天周期内，下次不晚于2026-10-28。

所有浏览器完成后，本次自有preview release-v180-1788777665390-26a656通过canonical stop停止mockApi/web；其他任务预览未触碰。该预览停止步骤在首次生产写入之前完成。

源码/tag `v1.7.0` / `d0f248904edc0d63b1d00add1ca5c7e0113fcfb0`；OCI `sha256:913bcac5f1344930f6fb87084d32453dea32b4932ef2c400c09a8ea3a96a266f`。本版新备份 `/root/kpanel-backups/pre-v1.8.0-20260907T111942Z`；未复用旧备份作为本版证据。需要回滚时恢复完整镜像/Agent/内置脚本/数据/密钥/配置并重新验收，判断GitHub Latest/Docker latest/标准更新入口公共默认通道，不只切镜像。

## 正式产物与生产闭环

- [GitHub v1.8.0](https://github.com/kejilion/KPanel/releases/tag/v1.8.0) 于2026-09-07T11:16:43Z公开，id384031547，非draft/非prerelease，8附件。Release原生镜像扫描、256MiB/1CPU/128PID/非root/只读根/cap-drop ALL运行契约通过；双架构SBOM/provenance完整。
- [Docker Hub](https://hub.docker.com/r/kjlion/kejilion-panel/tags) 的1.8.0和latest index均 `sha256:86112e0e2549be443b50b80193068be3603f6bcdf5004d421099db77f3098a58`。linux/amd64 `sha256:0b8855b12e899feb1e5335ebe56d5ace7d921b96c17ed47e7c59d3eab03251ea`，linux/arm64 `sha256:a8c57dd2a2b45d4cdf171a87eafdb2917dea2f0d4b0cb45d9b05a66ca2a273af`；额外两份unknown为attestation。
- 8公开附件实际下载并核对GitHub digest/size与SHA256SUMS；本机副本再次独立hash核对通过。Agent amd64 `57e638a163ce80986333d5c3b2698e1ea701b8fbb2f3dcf5f89959be27b69a7f`、arm64 `ab0fd2b7fd6b40d3c78d4b8d9bcee538d5bfb7dacb70e4dd43c916c0503f767d`；node amd64 `4ca835280b00a779afa50a27bcad55f28740c6144841f53bf2c71996bb9a7656`、arm64 `32583d166ed28d72c23e3bde2b91a98d89674439452b775302f991f4f7b2b7f1`；deploy `569db3040ce4f9e1d2fe3a363ec25c15e8728b93c9e2b567f1125ca96492453a`。LICENSE/THIRD_PARTY_NOTICES与精确源码一致。
- 从公开immutable index重新拉取镜像，revision435ae91/version1.8.0/script pins/User65532:65532一致，`image_e2e=pass`，11:18:33Z完成。证据 `C:/GitHub/_release-artifacts/v1.8.0-public-435ae91-r1` 与远端同名根，未复用私有L3镜像。
- 生产apps工作树 `/root/apps` clean，HEAD2d8044adec98e3eb16f47cdbb297f6be9632a66f；kpanel.conf与候选及公开main全文hash7b5b52af一致。标准更新检查apps两次均Already up to date；没有apps空提交或配置覆盖。
- 生产入口脚本SHA `129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`；完整postdeploy参数及真实公开digest已在首次生产写前prepare-only通过。
- 首次生产写为2026-09-07T11:19:42Z。backup11:19:42Z→11:19:51Z passed：停写归档、6文件摘要/结构、旧镜像实际load、旧服务恢复通过；新备份 `/root/kpanel-backups/pre-v1.8.0-20260907T111942Z`。kpanel.tar.zst SHA `5670296fc70214753fa0f6720f5dec7a7be43405cd312bada49baf49f019b471`，old-image.tar.zst `c140d7a61fb4d27fc57c3e12b6d0183337c34067727593804937febe241b9d4b`。
- 唯一正式升级入口 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel` exit0，实际拉取latest86112e0e，恢复原访问策略；没有自制生产wrapper。停写恢复短暂curl reset、TERM和daemon-reload过渡提示由标准入口正常收敛，没有额外重试。
- postdeploy11:20:53Z→11:20:55Z passed/exit0：1.8.0/ok/initialized、revision435ae91/OCI86112e0e；Panelrunning/healthy、restart0、OOMfalse；Agentloaded/active/running/enabled、NeedDaemonReloadno。protected.diff为空；panel/ai.db quick_checkok，顶层ai.db为空如实记录；近10分钟无fatal/panic/OOM日志。资源单点CPU0.02%、RSS72.64MiB/256MiB、PIDs7，不冒称长期趋势或P95。
- preflight/backup/postdeploy顶层证据hash分别5/8/8项独立通过，不把未列入的snapshot称作顶层清单hash覆盖。原始health.checkedAt纳秒11:20:53.685336953Z只保留原件，机器字段采用UTC秒。
- 本地生产证明 `C:/GitHub/_release-artifacts/v180-production-{preflight,backup,postdeploy}/remote-evidence`；远端 `/root/kpanel-release-evidence/v1.8.0-production/production-{preflight,backup,postdeploy}`。生产仅标准备份/更新及访问策略恢复，未回滚。GitHub Latest、Docker latest和标准更新入口指向健康v1.8.0，公共默认通道回滚决策不适用。
- 多维状态：业务/安全/稳定/受影响前端资源预算/体验已验证，真实浏览器层是隔离mock API而非生产账号业务操作；数据迁移不适用。未验证的整机聚合性能、小时级soak及全平台辅助技术不被此次UI变更扩大，也未使用旧证据宣称本版通过。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-07T16:08:20+08:00
- 候选冻结时间：2026-09-07T18:40:32+08:00
- 生产完成时间：2026-09-07T19:20:55+08:00
- 提交到生产用时：3.209722 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：2
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {"fingerprint":"testing/overview-presentation/incomplete-mock-journey","position":"before-production-write","count":1,"impact":"新增桌面窗口旅程复用overview时缺少三个辅助mock GET接口，几何通过但console404导致r1失败","recoveryEvidence":"v180-desktop-browser-r2同435ae91六场景通过，console/pageerrors0；preload明确只补public-network及模拟c32站点appearance/icon","permanentAction":"本轮唯一规格固定Node --require v180-desktop-mock-preload.cjs再调用原仓库桌面入口，保留严格console断言；这是交付验收夹具修复，不冒称通用mock服务器已修改。相关v1.6.1曾补过overview入口；本版已复用其路由响应并在生产写前完成全旅程回归，后续调用须携带同规格。","historicalReleases":["v1.6.1"]},
  {"fingerprint":"testing/docker-console/expected-failure-classification","position":"before-production-write","count":1,"impact":"补充console审计仅分类502，误判模拟444容器预设503超时导致r2退出1","recoveryEvidence":"保留r2原始12条console及状态；r3按实际mock定义仅匹配指定check_update路径和502/503失败，r3于10:55:39.024Z passed，8场景通过、unexpected0/cleanuptrue","permanentAction":"外部r3审计分别固定Apps 502和Docker444容器503，其他所有console error仍失败；未放宽产品错误处理或通用网络错误。最终同候选全旅程回归已在生产写前通过。","historicalReleases":[]},
  {"fingerprint":"cleanup/local-artifacts/execution-policy","position":"after-production-write","count":1,"impact":"生产已健康后，仅针对本次自有web/node_modules与web/dist的回收命令被平台执行策略在启动前拒绝；没有删除文件，不影响生产","recoveryEvidence":"保留平台拒绝原始结果，未换工具/命令绕过或重试删除。只读盘点合计201915630逻辑字节，实际净释放0；原件与全部证明仍可用","permanentAction":"本轮发布负责人保留受限目录，不把失败写成回收成功；平台执行策略属于外部限制，期限例外复核2026-09-08，退出条件为策略允许且重新核对所有权/无使用者后通过原生PowerShell精确回收。下轮生产写前先记录此约束，不将非必要本地清理放入生产关键链，禁止绕过策略。","historicalReleases":["v1.5.0","v1.6.0"]},
  {"fingerprint":"acceptance/record-generator/trailing-blank-line","position":"after-production-write","count":1,"impact":"新验收记录生成器保留了额外EOF空行，首次暂存后git diff --cached --check阻断提交，未产生提交或生产变更","recoveryEvidence":"去掉EOF多余空行后重新暂存，差异检查、指标校验和L0均通过；原始拒绝保留","permanentAction":"本轮输出按trimEnd后恰好一个末尾换行生成，提交前同时检查已暂存差异；下轮须把此检查纳入生产写前的同一文档生成预检，不把未暂存文件的git diff空输出当作通过证明。","historicalReleases":[]}
]
<!-- kpanel-release-process-incidents:end -->

已比较最近v1.7.0/v1.6.1/v1.6.0/v1.5.0/v1.4.1，按相同根因保留mock旅程历史关联，不更换指纹来掩盖重复；本次初期路径/SSH格式诊断无效输出均不用于门禁结论，也未触发重跑必需门禁。

## 本地资源与未覆盖

其他所有dirty内容与原始失败证据保留。本版自有preview已正常停止；远端生命周期测试负责清理自身容器/目录，备份与原始证明保留。无生产故障注入、用户文件写、真实Telegram发送、小时级soak或全平台辅助技术验收声明。

自有回收目标已逐一规范化核对：web/node_modules13819文件195332883逻辑字节、web/dist1075文件6582747字节，均Git ignored且reparsePoints0，preview stopped；总201915630字节。平台策略拒绝实际删除，净释放0，没有换工具重试。源码、原始证明和新备份全部保留；下次清理前需重新确认没有消费者。此限制不是生产故障。
