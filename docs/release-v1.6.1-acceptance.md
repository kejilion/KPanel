# KPanel v1.6.1 发布验收记录

日期：2026-09-07。发布级别：L3。

候选提交 / 标签：`2f926d6e4d6a2e0643f0f0c60d38f9b2feeefbe2` / `v1.6.1`。

上一稳定版本：`v1.6.0`，产品提交 `6dc6a46c7d2746a263a6ea8cd2f905ecae5f497c`；集成基线 `4297c3639f53857748bc9e60d427abcbcdfb834e`。

## 发布画像、范围与未纳入内容

- 用户授权修复概览/系统工具卡片信息呈现、冷启动顺序和静置刷新抽动，并形成候选上线。属于展示、只读状态合并及标准部署；运行时 API、数据、端口、Compose、Agent 权限、脚本与应用市场契约不变。小型界面回归修复采用 patch；生产流程仍执行 L3。
- 聚焦修复提交 `76b49e3cfff93cf7cf63815c93f557ea083cbd55`，版本提交 `2f926d6e4d6a2e0643f0f0c60d38f9b2feeefbe2`，共10文件。产品代码仅 OverviewView.vue/overviewState.ts，其余为测试、便携浏览器回归及版本/CHANGELOG。
- 恢复卡片标题/状态/详情的信息密度，移除新增脚本描述行和占据布局的观测时间行；详细说明仍在操作弹窗，时间留在 title 元数据。概览基础 runtime 就绪后按原 DOM 顺序整体呈现，不等待管理探针；系统中心静态24项工具仍立即可用，不新增工具、路由或业务写动作。
- 20秒刷新中的 partial 不再将已有服务列表、计数和公网信息临时清空；完整结果仍正常覆盖，包括真实空值或失败。刷新 runtime 失败保留已有页面并明确提示，首次失败仍可重试；取消/陈旧回调保护不变。
- 未纳入：冻结后收到的本机资源提醒下线候选 `dde18fe39517e24b7f7485c0c5b32af3b9d052d4`、其他未交付/未提交内容、依赖升级及额外系统中心扩展。该候选须在下一组合独立复核，旧基线 L2 不是本版或下一版证据。未修改作者工作树或唤醒旧任务。

## 跨仓库联动判定

- `scriptLinkageState=not-required`：无需发布脚本（不适用），不是延期依赖。跨仓库变更集、脚本候选、阻断/移除依赖均不适用。本版不推送 sh/apps。
- 内置脚本 commit `298f6f23751e36726660d73b5c0c83aef1b404f4`，raw SHA256 `80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581`；标准许可/统计替换后安装脚本 SHA256 `e72034d75b24eae1484ddc4a0af437b109ea701e7a0c8679520006dca7c09539`。两种摘要用途不同。
- 精确候选 packaging/kejilion-app/kpanel.conf、本地 apps 文件和公开 apps/main 文件换行归一后相等；无安装/脚本协议变化。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 新状态合并/失败/取消测试、完整 L3；后端及双端协议未改 |
| 网络入侵与供应链安全 | 已验证 | race、vet、govuln 实际可达0、npm audit0、固定 Trivy 源码/镜像、发布附件摘要及受限镜像契约 |
| 稳定性与失败恢复 | 已验证 | 基线确实复现，新浏览器回归通过；真实20秒刷新两周期、未保存表单、独立失败/重试；不是小时级 soak |
| 性能资源 | 已验证 | 新 runtime P95 158.884ms；旧 full summary 使用下述明确有期限例外，不是默认250ms通过 |
| 用户体验与可访问性 | 已验证 | 真实 Chrome+Mock API，经典/桌面、390/768/1280、明暗/三语言/根文字缩放、键盘及几何；不是生产浏览器全功能认证 |
| 数据配置与迁移 | 已验证 | 无迁移；新停写备份、旧镜像可加载、protected 无差异、SQLite ok、标准部署健康 |

## 自动门禁

- 先保留未修复产品的红测试：2文件4失败/11通过。首次 L1 因新增测试夹具把 ServiceStatus.state 写成 status 导致 TS2352，修正后全量148文件/1260测试、typecheck/build、i18n2218短语/21目录通过；定向19测试通过。首次失败没有抹除或冒称首轮全绿。
- 固定 `run-release-l3.mjs` → `run-release-gate.sh` → `make verify-release`，run `v1.6.1-2f926d6-l3-r1`，2026-09-07T01:56:22Z→02:10:46Z，passed/exit0。164治理测试、全量Go、特权 race、vet、前端148/1260、i18n/typecheck/build、govuln/Trivy、安装更新回滚卸载生命周期及双架构二进制通过。负向生命周期夹具错误是预期断言。
- Runner `kpanel-release-gate:go1.26.7-node24`，不可变 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle `bb3d216f82344afb55357e4da82ec3c81c0595d1076217948d7f85735581ad5c`；plan `d0e6a5a1b638dbf0a00f23c82e3ad3912e8065fef282764b4803317380d0540e`；远端脚本 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 证据根 `C:/GitHub/_release-artifacts`，L3子目录 `v1.6.1-2f926d6-l3-r1/remote-evidence`；远端 `/root/kpanel-release-evidence/v1.6.1-2f926d6-l3-r1`。下载后11项 evidence.sha256 独立核对通过。
- 候选 [CI34075592648](https://github.com/kejilion/KPanel/actions/runs/34075592648) / [freshness34075592630](https://github.com/kejilion/KPanel/actions/runs/34075592630)、主线 [CI34075938602](https://github.com/kejilion/KPanel/actions/runs/34075938602) / [freshness34075938578](https://github.com/kejilion/KPanel/actions/runs/34075938578) 均 success，精确2f926d6。主线通过后推送 annotated tag `11df70c35a4905ba8c0848944c8048f1d895aaf3`，peel 为同一产品SHA。
- [Release34076322001](https://github.com/kejilion/KPanel/actions/runs/34076322001) / [tag freshness34076322017](https://github.com/kejilion/KPanel/actions/runs/34076322017) success；镜像扫描、受限运行契约、双架构 SBOM/provenance、latest 提升、公开及候选分支清理通过。未替换旧 tag。

## 依赖与技术栈变化

- 同SHA freshness 报告生成2026-09-07T02:14:07.270Z，8/8源完整，直接/基座行动项 emergency0、patch7、minor14、major3。原始日志 `v161-candidate-freshness.log`；传递信号按直接依赖归属处理，不机械升级。
- 最近每日安全审计34018751198于2026-09-06T07:17:32Z success，是旧351bf2a的历史审计，不代替本次安全检查。当前 govuln 实际可达0，另3项非调用模块提示，不宣称所有依赖没有通告。EOL既定2026-07-28复核、2026-10-28下次，无到期例外。
- 本纯UI补丁不采用依赖/工具链/Action/扫描器/基座或脚本升级候选，锁文件仅版本变更。现有行动项由维护协调按首次完整检测计时，不因新报告清零：安全1/3/3天、patch7/14/30天、minor14/30/60天、major30/90/90天的启动/决策/处置期限。2026-09-07复核，退出条件为逐项证据支持的升级或有期限拒绝，不无限延期；本版没有新增延期例外。

## 隔离真机、浏览器与性能验收

- 唯一 arena-154，现有 candidate-validation/production-safety-check/production-deploy 用途；Linux amd64/Docker29.6.2，固定 Go1.26.7 Runner。Windows Chrome152.0.7977.82、Playwright1.62.1；本地浏览器策略 `local-release-v161`，仅自有 mock 预览，不使用生产账号。
- 跟踪入口 `scripts/browser-tests/overview-presentation.mjs`。基线 r1 捕获冷启动/卡片错误，但桌面未打开及辅助 mock 路由缺失使完整流程无效；修正夹具后的 `v161-baseline-browser-r2` 成功证明 product passed=false。精确最终2f对应 `v161-final-browser-r1`，job `local-release-v161-42240`，01:56:21.609Z→01:57:51.124Z，passed/exit0、9cases、无非预期 console/page error、regressions0、cleanup=true、resourceFailure=false。注入 BBR503 是预期失败测试。
- 命令规格 `C:/GitHub/_release-artifacts/v161-final-spec.json`，SHA256 `2c84d52bce69c267998cbc9041d5eef20fee7402052a042fe33e59e7753f63dc`，270秒外部/240秒内部硬截止；没有无期限前台浏览器等待。
- 经典/桌面各两个真实20秒刷新周期，验证未保存DNS输入、pending状态、完整结果及错误重试。基线经典 height1681.4375→1493.8125/scroll873→740，桌面1737.34375→1533.71875/973→953；修复后经典1552.375/798、桌面1608.28125/908在对应刷新前后不变。首屏无下方管理区抢先闪现，24工具/概览6项信息结构已检查。
- 390/768/1280视口、light/dark、zh-CN/en-US/zh-TW、100/125/200%根文字缩放（不是浏览器菜单缩放）、水平不溢出、详情说明保留和 Escape。状态/帮助基准字号保持14/13px，无新增字号改动；证据不覆盖所有硬件或辅助技术。所有自有预览已正常停止，端点不再可访问。
- 精确 Agent 顺序只读比较在 L3 结束后执行：私有 systemd unit/state/token、标准沙箱、system writes=false、CPUQuota100%/MemoryMax256MiB/TasksMax128，无用户凭据借用。夹具 `v161-readonly-agent-2f926d6.py` SHA256 `ad7e262a6fae576d5b2337827d3edcbf55630207588804aba4626961a5b95cdc`；原始 `v161-readonly-agent-2f926d6-r1-{base,candidate}/result.json`。两轮已停止私有unit、删除私有token，无生产业务写入。

### 旧 full summary 有期限例外复核

1. 数据：安装中的 v1.6.0 Agent vs 精确2f候选，同机順序20个预热后样本。旧 full summary P95 719.319632→797.684109ms（+10.89%）；CPU13974478→14048021usec（+0.53%）；peakRSS29.425781→31.046875MiB（+5.51%），候选idle19.332031MiB，memory max/OOM0，所有只读请求成功。
2. 原因/边界：后端字节源未改，旧完整聚合仍含CPU采样和固定管理探针，不满足默认250ms。本版独立 runtime P95 158.884075ms；config6样本1.483–2.570ms、SSH280.989–382.910ms、BBR183.230–206.334ms只报告范围，不冒称小样本P95。
3. 替代方案：回退状态拆分会重新阻塞首屏，任意延时不能修复 partial 清空，完整结果缓存改变新鲜度；本版仅保留刷新中的旧可选资源，完整结果照实覆盖。未通过修改默认预算掩盖慢路径。
4. 当前发布负责人依据用户授权及新证据复核：仅旧 summary 临时 P95<=1000ms、相对稳定版P95/CPU/peakRSS回退<=20%、idle<=32MiB/readpeak<128MiB、error/OOM0。有效至下一稳定发布复核或2026-09-14较早者，不自动续期；退出条件为完整summary达250ms或新的证据复核。正确性、安全、恢复或资源失败立即取消。不是复用 v1.6.0 数据，也不是默认250ms通过。
5. 回滚用下述完整 v1.6.0 备份与OCI；更慢硬件及小时级soak未验证。纯UI修复采用两个真实轮询周期加完整L3，未执行无关宿主业务写/真实Telegram投递。

## 发布产物与公开仓库复核

- [GitHub v1.6.1](https://github.com/kejilion/KPanel/releases/tag/v1.6.1) 于2026-09-07T02:35:23Z公开，非draft/非prerelease。
- `kjlion/kejilion-panel:1.6.1` 与 `latest` OCI index 均 `sha256:4d64cf52dc093c5f17f8bb9e77e736ecbe0632a9bf2dcc271ebafbd099b9126c`；amd64 `sha256:c66ce435d4cdf993b09822f30782847c6bb663872b3df2a5446c93b4059ef570`，arm64 `sha256:18c5f029821641e3c3c02f495f6bda5b17e9cbbfb8f246f72f486fd05a346985`，另两份unknown平台为attestation。
- 八附件实际下载，与GitHub digest/size及SHA256SUMS核对：Agent amd64 `dd3310116a987a02cd4c3f6a353a4d28b513e7c09106be38e058e57b8f7370e1`、arm64 `393eb0f8f9b00451bed07e9baa0b58e668d3b23ea8019403b4237b4eddd647f6`；node amd64 `40fdb85733e08ef82db336f7e1ac2af9eb7f9b218fe286f8ff2ec634b5e3e848`、arm64 `86a73645227c2201a1932648c661368f54b6fbccc9be3c5cdfcabfa28f9e0a7b`；deploy `0e6db2a2b54edbcb85666c5094e1bd4711acefa2721d493413f6a00a492914e6`。许可证/第三方声明与源码一致。
- 显式拉取公开index，标签version/revision/script pin与非root65532:65532核对，公开 `image_e2e=pass`，02:37:33Z结束。`v1.6.1-public-2f926d6-r1`保留完整证据；校验脚本 `verify-public-v161-2f926d6.sh` SHA256 `cc2d61e5c92bbe25d3c91e1958399b4155cb78e2f4af92f09c6a89296057273a`，仅公开产物验证，不是生产更新wrapper。

## 生产部署安全核对与回滚

- 仅 arena-154，用户已授权本修复上线。108/prod-108禁用全部KPanel操作，本次未连接、未备份、未部署、未升级、未核对。
- 固定 `run-production-evidence.mjs`，run `v1.6.1-2f926d6-production`，脚本SHA256 `129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`。preflight 02:14:36Z→02:14:37Z passed，基线1.6.0健康、Agent active、数据和配置基线已保存；5项顶层evidence摘要独立核验，不把未列入的snapshot称为摘要覆盖。
- 新备份 `/root/kpanel-backups/pre-v1.6.1-20260907T023832Z`，02:38:32Z→02:38:42Z passed；02:38:32Z为首次生产写。停写归档、结构/摘要、旧镜像实际load均通过，六份备份文件已核验。kpanel.tar.zst `53b85db695ca009422bf7d844df43b219e9ce6a8086fada5305b40773bada669`，old-image.tar.zst `a65d39683e9996407d62abf1ff8ccd1cfec478d43d5aa45c59c5d711e1597dca`；unit、应用配置、旧inspect一并保存。
- 标准更新 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel` exit0，实际拉取上文公开latest摘要；无自制生产wrapper。短暂curl reset及TERM/daemonreload过渡提示在固定入口正常收敛，不是独立重试或遗留失败。
- postdeploy 02:40:46Z→02:40:47Z passed/exit0；1.6.1/ok/initialized、revision2f926d6、index4d64cf一致。Panel running/healthy、restart0、OOMfalse；Agent loaded/active/running/enabled、NeedDaemonReload=no，启动日志版本1.6.1。protected.diff为空，panel/ai.db quick_check=ok，顶层ai.db为空如实记录；致命日志检查通过。下载后8项顶层evidence摘要独立核验。
- 本地证据 `v1.6.1-production-{preflight,backup,postdeploy}/remote-evidence`；远端 `/root/kpanel-release-evidence/v1.6.1-2f926d6-production/production-{preflight,backup,postdeploy}`。生产证据覆盖本机API/服务/OCI/配置/数据，不冒称公网域名浏览器端到端实测。
- 生产只执行标准停写备份/恢复、更新及访问策略恢复，无用户业务文件操作、Telegram发送或故障注入。未回滚；当前生产实际1.6.1健康，GitHub Release/Latest、Docker latest及标准更新指向本版，公共默认通道恢复决策不适用。
- 回滚源 v1.6.0/6dc6a46，OCI `sha256:2b8588232a0852a5da6f2e61f1a4786a5eeb94dd3f5901633b5a5121c987d244`。若触发回滚，须停写并成套恢复备份中的旧镜像/Agent/脚本/数据/密钥/配置，核对版本、健康、protected、SQLite及公开默认通道，不只替换镜像；本次未执行该生产回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-07T09:54:43+08:00
- 候选冻结时间：2026-09-07T09:55:13+08:00
- 生产完成时间：2026-09-07T10:40:47+08:00
- 提交到生产用时：0.767778
- 是否回滚、紧急热修复或重复发布：是（修复 v1.6.0 已逃逸的界面回归；本次未回滚）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-09-07T01:50:02.170Z；恢复时间：2026-09-07T02:40:47Z；逃逸门禁：已逃逸：v1.6.0 浏览器只验证指标先于管理请求和卡片数量，未检查冷启动整体顺序与可选资源刷新几何，且误将额外描述作为正确条件
<!-- kpanel-release-metrics:end -->

发现时间来自首次基线冷启动截图原始mtime（2026-09-07T01:50:02.1705617Z），机器字段按既有协议保留毫秒；首个提交采用 committer 时间。既有 v1.6.0 验收记录不追溯改写；本记录明确说明上线后发现的回归及新增拦截。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：5
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "evidence/exec-command/lost-session-handle",
    "position": "before-production-write",
    "count": 1,
    "impact": "最初复制依赖及红测试命令未保留长作业句柄，该次结果不可采信",
    "recoveryEvidence": "另行捕获红测试2文件4失败11通过；后续完整exec返回及session终态均保留",
    "permanentAction": "执行器调用保留完整返回对象，不只取output；发布负责人本轮已按此约束获取L3及生产终态",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/overview-fixture/invalid-service-state",
    "position": "before-production-write",
    "count": 1,
    "impact": "新增测试夹具字段错误触发TS2352，使首次L1失败",
    "recoveryEvidence": "修复后148文件1260测试/typecheck/build通过；精确2f的L3通过",
    "permanentAction": "76b49e3修正ServiceStatus.state夹具并保留刷新状态回归",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/overview-presentation/incomplete-mock-journey",
    "position": "before-production-write",
    "count": 1,
    "impact": "基线r1桌面未打开overview且辅助mock路由不全，完整浏览器流程无效",
    "recoveryEvidence": "baseline-r2成功证明旧产品失败，final-browser-r1精确2f九场景通过",
    "permanentAction": "76b49e3跟踪便携浏览器入口，显式打开桌面应用并补齐本旅程mock路由",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/workflow-parser/missing-python-yaml",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地Python缺少PyYAML，首次工作流解析命令无效",
    "recoveryEvidence": "生产写前预检arena已有PyYAML并对精确候选ci.yml/release.yml解析通过，无安装",
    "permanentAction": "本轮使用预检确认的既有解析运行时；发布负责人2026-09-14前复核本地入口，退出条件为入口显式验证运行时能力，不能把现场绕行视为已完成通用修复",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/release-metrics/timestamp-precision",
    "position": "after-production-write",
    "count": 1,
    "impact": "验收记录首次校验拒绝原始Windows七位小数时间戳，未影响已健康运行的生产",
    "recoveryEvidence": "机器字段改为既有ISO毫秒格式，原始mtime仍保留在区块外；重新执行验收和L0门禁",
    "permanentAction": "按现有report-release-metrics ISO_TIMESTAMP契约限制机器字段至毫秒，不修改校验器或原始证据",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

读取最近五份正式验收记录并比较指纹，本轮上述五项无完全相同历史指纹；相邻的浏览器/工具能力问题保留原因，不以重跑掩盖首次失败。正常红测试、故障注入断言和标准部署短暂readiness提示不计作独立流程异常。

## 遗留风险、资源回收与后续准入

- 旧 full summary 预算例外及运行时解析入口待复核如上；没有更改默认预算或新增治理系统。Mock浏览器不替代生产用户端全功能验证，公开镜像和生产健康使用各自独立证据。
- 本机资源提醒下线交付仅登记下一候选，未纳入本冻结版本，未借用其旧基线L2作为组合证明。
- 本地按13.1盘点，关闭所有本任务预览；保留自有node_modules/dist、准确原始证据和可回滚工作树。此前平台拒绝删除清理，本轮不换工具重试该删除，实际净释放0字节。其他活跃、未知归属或未提交内容未清理；远端私有测试unit/token已由夹具收尾，生产备份保留。
- 复用现有发布、预览、后台浏览器工作流；只增加本旅程回归入口与规范要求的验收记录，未创建新规范、平行发布系统或记忆文档。
