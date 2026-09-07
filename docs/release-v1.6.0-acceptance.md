# KPanel v1.6.0 发布验收记录

日期：2026-09-07

发布级别：L3

候选提交 / 标签：`6dc6a46c7d2746a263a6ea8cd2f905ecae5f497c` / `v1.6.0`。

上一稳定版本 / 回滚点：`v1.5.0` / `7ba46d1ad7d3e1d55dc6451b194e75a6c78df8b2`。

## 发布画像与范围

- 本次 minor 包含按需 Docker 镜像更新检查、本机证书/容器 Telegram 提醒、用户主动生成的本地问题报告，以及 Docker 备份恢复、远程文件流、AI 附件历史解析、资源读取和概览/系统中心加载优化。涉及展示、只读聚合、既有恢复写链、通知可选数据和安装兼容，执行 L3。
- 精确增量 `b85be6aaeed163d43b7871e710d24eec9d7c7fef..6dc6a46c7d2746a263a6ea8cd2f905ecae5f497c`：2159f53、bf36f03、545f040、f80a24d、777e7df、aec3411、d119c62、d97a6e5、a6a02ec、94676b7、4c54590、5063695、df86c3c、6dc6a46。
- 用户追加授权现有系统中心全部工具加载优化：24 项静态名称/描述先显示，基础指标不等管理探针；配置、SSH 防御、BBRv3 分组独立读取、显示观测时间，失败可重试。刷新保留已有观测及未保存表单；未知状态不会冒充关闭或开放依赖状态的写操作。
- 新增固定认证只读 `/v1/system/runtime` 和 `/v1/system/management/{config,ssh-defense,bbrv3}` 及对应 Panel 代理白名单，复用现有源头；不新增 System Center 页面、工具、权限、业务写动作或脚本协议。保留旧完整 summary，旧 Agent 仅404/501触发兼容回退；不缓存完成结果，不将管理子进程混入 CPU 观测窗口。
- 未纳入：未提交改动、冻结后新功能、其他 System Center 扩展、依赖/工具链升级。原作者工作树未改动、未唤醒旧任务；关闭的预览均为本任务所有。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`，无需发布脚本（不适用），不是待发布依赖。跨仓库变更集/脚本候选/移除依赖：不适用。
- 内置 `kejilion.sh` commit `298f6f23751e36726660d73b5c0c83aef1b404f4`，raw SHA256 `80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581`。实际安装脚本 SHA256 `e72034d75b24eae1484ddc4a0af437b109ea701e7a0c8679520006dca7c09539`，为标准许可/统计替换后的字节，不拿它与 raw 摘要误判漂移。
- GNU/BusyBox sha256sum 兼容修改在 Panel 安装器；现有受管脚本协议和应用市场配置不改，无 sh/apps 推送。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确6dc全量门禁、真实 Agent 恢复/EBUSY/重启、真实 Docker 标签移动；未测所有用户节点 |
| 网络入侵与供应链安全 | 已验证 | race、govulncheck实际可达0、npm audit0、固定Trivy源码/镜像、认证白名单；3项模块非调用提示不等于全部依赖无通告 |
| 稳定性与失败恢复 | 已验证 | 文件取消/中断/panic后不伪造完整响应，失败恢复保留旧文件，状态独立失败/重试/取消及旧Agent兼容 |
| 性能资源 | 已验证 | 新runtime P95约159ms；旧完整summary使用下述有期限例外，不是250ms默认门禁通过 |
| 用户体验与可访问性 | 已验证 | 真浏览器+Mock API，24工具先显示、分组失败、键盘/刷新表单保留、多语言/视口矩阵；不是生产浏览器全功能认证 |
| 数据配置与回滚 | 已验证 | 真实备份恢复字节/UID/GID/权限、EBUSY回滚、Agent重启不重放；新通知字段回滚须恢复兼容数据，不是只换镜像 |

## 自动门禁

- 固定 `run-release-l3.mjs` → `run-release-gate.sh` → `make verify-release`，run `v1.6.0-6dc6a46-l3-r1`，2026-09-06T23:28:08Z→23:42:12Z，passed/exit0。
- Runner `kpanel-release-gate:go1.26.7-node24` / `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，Go1.26.7；bundle `65fe69a18717542feda0740fd9778b8a3fa203b08c0a9e41311bedb927be0ff5`，plan `c641397abfa1a9a23ab3c370f86727e50c9278949a29ec3e4052e4dbc8cfeab8`，远端脚本 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。11份证据下载后逐个SHA256核对通过。
- 全量Go、特权race、vet、148前端文件/1255测试、typecheck、i18n2218短语/21目录、amd64/arm64构建、runtime镜像、安装/更新/回滚/卸载生命周期全部通过。负向安装夹具的错误日志为预期断言，不是生产失败。
- 候选 [CI34067693956](https://github.com/kejilion/KPanel/actions/runs/34067693956) / [freshness34067693888](https://github.com/kejilion/KPanel/actions/runs/34067693888) success；主线 [CI34068043478](https://github.com/kejilion/KPanel/actions/runs/34068043478) / [freshness34068043520](https://github.com/kejilion/KPanel/actions/runs/34068043520) success，均完整6dc。main先快进，CI通过后创建annotated tag `bb8e3e1adfd8076e28dc6f528f618965d0161214`，peel为6dc；Release34068468621与tag freshness34068468622待最终核对。
- 本地证据根 `C:/GitHub/_release-artifacts`，L3子目录 `v1.6.0-6dc6a46-l3-r1/remote-evidence`；远端 `/root/kpanel-release-evidence/v1.6.0-6dc6a46-l3-r1`。早期4c/506结果保留历史，不替代最终SHA。

## 依赖与技术栈变化

- 同SHA报告生成2026-09-06T23:44:03.463Z，8/8源完整，24直接/基座行动项：emergency0、patch7、minor14、major3，另122传递信号。完整报告 `v160-dependency-freshness-6dc.md`；它们不是验证完毕的实现候选，本版不夹带升级。
- 最近每日安全审计34018751198/job101447305053（2026-09-06T07:17:32Z，先前main351bf2a）success；它不是当前SHA替身，当前自己的安全门禁独立通过。EOL无到期例外，下次既定复核2026-10-28。
- 由项目维护协调沿首次完整检测计时，不因重报重置：安全1/3/3天，patch7/14/30天，minor14/30/60天，major30/90/90天；启动/决策/处置依现有政策。负责人2026-09-07复核，退出条件为逐项兼容/安全/资源/回滚证据或有期限拒绝，不无限延期。

## 隔离真机与浏览器验收

- 唯一 arena-154，环境策略登记 candidate-validation/production-safety-check/production-deploy。Docker Engine29.6.2；测试只使用有界自有容器/namespace/unit，未降低生产服务权限。
- `v160-profile-6dc-r1`：8受影响race包、3AI benchmark（114.69–138.83ms/op，与L3并行负载，不作为独立性能比较）、3root mount/net隔离证书测试通过。宿主测试二进制CGO_ENABLED=0并先验证静态链接。
- `v160-real-docker-6dc-r1`：真实Engine current→标签移动→available，运行container/Image身份不变；仅清理自有fixture，cleanup通过。
- `v160-backend-6dc-r1`：精确Agent二进制、私有mount/net、无Docker socket/system writes=false；备份→变更→恢复bytes/1234:2345/604与751，第二project真实EBUSY后两边旧bytes恢复，Agent停止/重启保留失败任务且不重放，无暂存/rollback残留。夹具SHA256 `09546a10e2e735a985dd5f1c0cc30b61fea278a8415e96264f571e8462284667`。
- `v160-browser-6dc-r1`：2026-09-06T23:27:53Z→23:28:59Z，passed/exit0，五套组合测试（Docker、问题报告、资源通知、三项共存、overview）；spec `v160-browser-6dc-spec.json` / `ed77dd113e5523d507c81a57b983d5d5af89725c8acbbfcc9e0292995cc3c3e3`。
- Overview覆盖390/768/1280、zhCN/en/zhTW、明暗色、100/125/200%根文字缩放（不是浏览器菜单缩放），状态14px/帮助13px、长说明换行、键盘/Escape。24项目录在runtime未响应时已显示，metrics先于被阻塞管理请求，SSH待定/BBR503不妨碍DNS、未知操作受保护、失败重试无页面错误；截图/geometry结果保留。
- `v160-overview-refresh-6dc-r1`：23:31:49Z→23:32:10Z，passed/exit0；实际20秒timer未加速，第二config请求held时DNS编辑器仍挂载、未保存8.8.8.8保留，旧观测标记Refreshing，响应完成后输入不丢。spec `a58c38b4cef8901c97c836642b29e975ba9bfe6e642c7ae13d5cd4119263f9c4`。各作业硬超时和资源按原spec/state保留，结果复制至外部证据目录，所有自有预览正常停止。
- 真实浏览器使用Mock业务API，不冒充生产；未向真实Telegram接收者发消息，投递由固定transport/持久化夹具验证；无用户凭据借用。没有小时级soak，不承诺所有低端硬件或全部用户环境。

## 性能与有期限例外

1. 环境/数据：精确6dc vs生产v1.5.0 Agent，arena同机实际243进程，正式安全profile/固定安装脚本，独立unit与token，CPUQuota100%/MemoryMax256MiB/TasksMax128。完整summary24次去前4次预热，旧P95837.479ms→759.147ms，CPU13.806021s→13.692741s，peakRSS27.078125→26.246094MiB；初始RSS19.308594MiB，memory max/oom/oom_kill均0。原始JSON `v160-readonly-agent-6dc-r1-{base,candidate}`；是有界顺序比较，不是置信区间/全硬件认证。
2. 原因：旧full summary保留150ms CPU采样后，还需固定F2B/BBRv3脚本现查，稳定版已不达250ms。本版新runtime20次有效样本P95158.827ms；普通config6次1.331–1.535ms，SSH302–371ms、BBRv3172–212ms分组独立，不挡基础指标。6样本报告范围，不把floor小样本分位冒称nearest-rank P95。
3. 替代比较：丢掉优化会退回更慢旧链；与CPU采样重叠会污染观测；省略字段改变兼容；完整结果缓存引入陈旧。采用固定分组只读和并发进行中共享，保持权威数据/取消/超时，未改脚本协议。
4. 用户明确让发布负责人据改善评估继续。仅旧 `/system/summary` 临时预算P95<=1000ms且相对稳定P95/CPU/peakRSS回退<=20%，idle<=32MiB、readpeak<128MiB、error0/OOM0仍强制；最终6dc满足。默认250ms没有整体修改，新runtime满足。例外于下一稳定发布复核或2026-09-14较早者到期，不自动续期，不创建后台任务。退出条件为完整summary达标或新一轮证据评估；更慢硬件未验证。
5. 回滚：保留v1.5.0 tag和OCI `sha256:20c3648c1fb3d64a8730a625a3ab3afc38d60643ac1f29c7d0b77c4239ee7134`，正式更新前新备份；通知新字段必须同时恢复兼容数据。任何正确性/安全/恢复或上述预算失败均取消此例外。

## 发布产物与公开仓库复核

- [Release34068468621](https://github.com/kejilion/KPanel/actions/runs/34068468621) 与tag freshness34068468622均success，完整6dc；源码、安全、镜像契约、双架构SBOM/provenance、latest提升、公开及候选分支清理全部通过。远端候选已不存在；历史tag未改写。
- [GitHub v1.6.0](https://github.com/kejilion/KPanel/releases/tag/v1.6.0) 于2026-09-07T00:10:03Z公开，draft=false/prerelease=false。
- [Docker Hub](https://hub.docker.com/r/kjlion/kejilion-panel/tags) 1.6.0/latest OCI index均 `sha256:2b8588232a0852a5da6f2e61f1a4786a5eeb94dd3f5901633b5a5121c987d244`；amd64 `sha256:3fedfd3859d0ea0228aae0a3d0acb844e0fbdddab078fbc888abc367a2ac6cb8`，arm64 `sha256:f7e457aa746a9b09d3f3bd3bd717746475b35f5c7a089772a37a71647e133095`；另两份unknown平台为attestation。
- 八附件实际下载，SHA256SUMS五项全部OK，每个附件与GitHub digest/size一致。SHA256SUMS `8c229be3301ac29e43ff3a7e4337fc0a93aa0eef9b77e7b47bc04aa1245c7644`；Agent amd64 `2644956fb946e0fe51cd69798b8537e95085b34b346dafe71240b6899ba8a009`、arm64 `f0298d2e4b958ff56899d4fe08579327126ffceb077d2823f72785d83ccc43f5`；node amd64 `1cab8847da258bdd7d454e10fd04eb92ae7b5acb6768aab6c5384b111c60724c`、arm64 `ca58b976e6646c7bf1c21cb469082873e62bdf5ff0f171c9e72cf683f40d3507`；deploy `cab1fa02424e00cc9de2147e4587c4ce26feddd091a6fbeab1978bf4fae249fe`。LICENSE/NOTICES与精确源码相同。
- 显式拉取公开index，version1.6.0/revision6dc/script298+803/user65532:65532一致；公开 `image_e2e=pass`，00:11:19Z完成。`v1.6.0-public-6dc-r1`证据已下载；校验fixture `951f59b98f2830daeeea152b71aaea40d837a4ad2b364c3d17020ca46809b6e1`，不作为生产更新wrapper。
- apps当前公开main2d8044adec98e3eb16f47cdbb297f6be9632a66f；raw、本地apps HEADf07bb5112b776cc9c2384dc1d417e0ba2b825892的kpanel.conf、候选 `packaging/kejilion-app/kpanel.conf` 换行归一字节相同。没有把本地apps未推送的其他差异带入发布。

## 生产部署安全核对与回滚

- 仅arena-154获授权。108/prod-108禁用全部KPanel操作，本次未连接、未备份、未部署、未核对。
- 固定入口 `run-production-evidence.mjs`；final preflight `v1.6.0-final6dc-production-r1` 23:35:41Z→23:35:43Z已通过，旧1.5.0 running/healthy、Agentactive、protected/SQLite基线已保存。
- 新备份 `/root/kpanel-backups/pre-v1.6.0-20260907T001148Z`，00:11:48Z→00:11:58Z，passed/exit0。停写归档、tar/zstd结构、旧镜像实际load、六项SHA256验证通过，备份后旧1.5.0健康恢复。kpanel.tar.zst `191ee87971119f4ad0479b86264ca3f5eafc3ddfdafedd123a54351ac7a4b91d`，old-image.tar.zst `29015a4e7950ae98abbd31238cc6fd1885d716ff6b1487aadc268fa572a9a2e5`；unit、apps和旧inspect一并保留。
- 标准应用市场update exit0，实际latest digest与上文相同，安装器恢复访问策略；日志 `v160-production-standard-update.log` SHA256 `660f3204622168fbba3096ba180de0df817f3eaf3e96cedae71721801e96c9f8`。
- postdeploy同run，00:13:05Z→00:13:07Z，passed/exit0；线上1.6.0/ok/initialized，revision6dc/index2b8588一致。Panel running/healthy、restart0、OOMfalse，Agent loaded/active/running/enabled、NeedDaemonReload=no；protected.diff空，panel/ai.db quick_check=ok，顶层ai.db为空如实记录，近10分钟致命日志扫描通过。证据 `v1.6.0-final6dc-production-{preflight,backup,postdeploy}-r1/remote-evidence`。
- 生产只执行标准停写备份/恢复、标准升级及访问策略恢复；文件恢复失败注入、证书变更、模拟断流、Agent隔离重启属于测试环境，不在生产业务上演练。生产证据为本机API/服务/OCI/数据安全检查，不冒称外部域名浏览器全功能实测。
- 未发生生产回滚或紧急热修复；生产实际1.6.0健康，GitHub Latest/版本Release、Docker latest和标准更新入口指向本版。公共默认通道恢复决策：不适用。备份恢复时一次短暂curl reset、安装中TERM/daemon-reload过渡提示均在既有入口正常收敛，不是新增绕过或独立重试。
- 标准更新入口 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；禁止临时生产wrapper。恢复应停写后成套恢复旧镜像、Agent、脚本、数据、密钥和配置，再核对版本/OCI/健康/protected/SQLite；不执行生产失败注入或用户文件恢复测试。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-06T21:48:21+08:00
- 候选冻结时间：2026-09-07T07:26:38+08:00
- 生产完成时间：2026-09-07T08:13:07+08:00
- 提交到生产用时：10.412778
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与资源回收

- 本版普通配对和数据库schema不变；通知新可选字段的旧版严格读取限制、Mock浏览器/固定通知transport和有限性能窗口边界均如上。
- 本地只读盘点本任务web/node_modules（13819文件、逻辑195332905字节）和web/dist（1072文件、6587817字节）；未发现使用它们的进程或子reparse point。带完整路径/祖先边界复核的同一PowerShell清理命令在执行前被平台策略拒绝，未换shell/工具绕过。没有删除，净释放0；可由精确源码/lockfile重建但本次保留。全部原作者工作树、未提交修改、原始失败、成功证据和备份保留。

## 首轮失败与流程异常

- 旧4c完整门禁通过后用户追加加载优化，因此以新6dc重建全部受影响证据，不把4c通过沿用。长描述截断、刷新时表单会消失在候选冻结前修复，属于正常产品审查拦截，不是生产事故。
- 记录8次使必需验证无效/失败或重试的流程事件：7次生产写前，1次生产写后本地清理被执行策略拒绝；没有生产服务失败。普通查找路径笔误没有运行验证或产生假pass，不计为有效性事件。先前完整summary预算失败为真实性能结果，采用明确例外而非删除失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：8
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/agent-fixture/readonly-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "性能r1使用原unit模板默认ReadOnlyPaths，实际应用市场路径不同，Agent未启动，无延迟样本。",
    "recoveryEvidence": "r3-r5及最终6dc base/candidate均使用正式unit现查路径并通过，原失败保留。",
    "permanentAction": "唯一v160-readonly-agent-6dc.py启动前断言路径存在、systemctl实际ReadOnlyPaths相符；仅更换测试私有路径，不放宽生产sandbox。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/agent-fixture/token-permissions",
    "position": "before-production-write",
    "count": 1,
    "impact": "性能r2 token属组/权限准备不满足正式Agent读取规则，未产生有效样本。",
    "recoveryEvidence": "固定root:kejilion-panel/0640后r3-r5以及最终两个真实unit认证请求成功，unit/token均清理。",
    "permanentAction": "唯一性能fixture在启动前核对UID/GID/0640，错误则assert退出；不复用生产token或禁用认证。已对比最近5版，识别v1.5.0复发，生产写前固定入口并以实际双binary运行验证，不声称仓库通用治理已经改写。",
    "historicalReleases": ["v1.5.0"]
  },
  {
    "fingerprint": "preflight/profile-fixture/elf-interpreter",
    "position": "before-production-write",
    "count": 1,
    "impact": "旧4c补充证书测试ELF依赖musl解释器，宿主无法执行。",
    "recoveryEvidence": "最终v160-profile-6dc-r1静态binary在root mount/net namespace真实执行3测试通过。",
    "permanentAction": "唯一profile编译显式CGO_ENABLED=0并检查可执行文件、file输出；保留旧失败，不修改生产。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/i18n-check/wrong-script-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "误用根scripts/check-page-i18n.mjs导致检查没有运行。",
    "recoveryEvidence": "回到web既有npm run i18n:check，最终L3独立2218/21通过。",
    "permanentAction": "发布责任人2026-09-07核实package.json已登记唯一入口；退出条件为实际入口和最终L3均通过，已满足，无新平行检查脚本。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/release-l3/missing-local-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3尚未复制远端证据时尝试本地SHA核对，缺文件，未计作通过。",
    "recoveryEvidence": "显式scp到最终remote-evidence后，11文件全部逐项hash匹配。",
    "permanentAction": "同一取证步骤先检查目录和条数、Stop模式验证全部摘要；无缺失默认成功，记录外层不会自动下载而非复用旧目录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/overview-fixture/missing-test-import",
    "position": "before-production-write",
    "count": 1,
    "impact": "新增定向Go测试缺httptest import，r1编译失败。",
    "recoveryEvidence": "修复后r2三个相关包race及最终6dc全量L3成功。",
    "permanentAction": "候选506/6dc跟踪测试导入修正并进入常规Go编译；不省略失败测试。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/overview-fixture/missing-request-host",
    "position": "before-production-write",
    "count": 1,
    "impact": "506 L3的新未登录fixture未带合法Host，收到421而非认证断言的401。",
    "recoveryEvidence": "6dc request.Host=panel.test 后完整L3和CI通过，旧506failed保留。",
    "permanentAction": "仅修正测试Host使其真正到达Session检查，生产Host防护和认证不放松。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/local-artifacts/execution-policy",
    "position": "after-production-write",
    "count": 1,
    "impact": "生产通过后，本任务node_modules/dist回收命令执行前被平台拒绝；未删除、净释放0，线上不受影响。",
    "recoveryEvidence": "保留原始拒绝和全部目录，不换shell或工具重试；后续文档收尾独立继续。",
    "permanentAction": "已识别v1.5.0同类平台拒绝；发布负责人2026-09-08复核平台执行限制，退出条件为原已核对范围命令得到平台明确允许。无自动重试、无扩大删除授权；跳过回收不阻断健康上线，不宣称已永久修复平台。",
    "historicalReleases": ["v1.5.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

- 已比对v1.5.0、v1.4.1、v1.4.0、v1.3.1、v1.3.0原始异常指纹；token权限复发明确记录，不称所有首轮通过。
- 生产写前追加8项确定性正反例，直接执行唯一性能fixture AST里的真实前置assert：正确token/path通过，错误UID/GID/0644/0600/缺路径/错误unit路径全部拒绝。`v160-fixture-preflight-test.py` SHA256 `7b5d9127f51f80d928705e6d5586e3b8c1ec7bf92e784172ab708357920e91e8`，8/8成功。只是测试夹具预检回归，不修改冻结候选或生产入口。
