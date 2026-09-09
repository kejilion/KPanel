# KPanel v1.9.0 发布验收

日期：2026-09-09。发布级别：L3。候选提交9028fab55d950cedc6e59325a7a28ba6ce21bd1c，标签v1.9.0。基线main 942ede6a24d6d0f97bbea01d21d84188b7e5abf8；上一稳定v1.8.1/c2d47e7c76e26332c9bbd31cb0e28986f50b3f2e，OCI sha256:a2c48db4c63d7bbb0fecc5c9e212f21b678288b37030b26929f8039d1cccbd34。

## 发布画像与范围

纳入d3b73a5文件编辑器、ec91d66重定向传参、49d27da证书替换兼容；重放f08f5e2/f418ae5/26a24d9，版本与脚本固定969e191。首次真实FilesView浏览器验收发现初始布局未测量时点击行号多选上一行，9028fab改用gutter对应逻辑行作为起点，拖动先完成公共几何测量，再计算文档坐标；7项单元回归和可拒绝原候选的真实浏览器脚本随提交保留。候选没有合入冲突，不召回作者或新增监工任务。

用户可见变化为14px代码、主题语法配色、连续及不连续整行选择；重定向填写目标域名并作为独立argv传给可信脚本；换证兼容两份官方历史续签器，自动迁移且不泄露内部输出。系统中心页面、路由、API、权限及业务文档无新增，既有基线不删除。数据schema、配对、应用市场安装配置和权限边界不变。

业务风险为宿主机证书写入/脚本协议和文件编辑交互。文件原文与CodeMirror选区、Nginx配置、证书PEM与systemd是真源；保持Agent固定动作、资源版本、脚本事务和材料清理。没有长期soak或真实用户业务数据写入声明。

## 跨仓库联动

scriptLinkageState=coupled。变更集redirect-script-inputs、certificate-replacement-compat-20260909。原脚本298f6f23751e36726660d73b5c0c83aef1b404f4/SHA256 80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581；脚本源提交740724c/a5bd96a组合为aac8bc8710bb559ec7ae1d6988770f64564954ca，SHA256 583aff02c2f75510edc5135addad31163a7dfeeb9ac99fcf190b064fcbadedee。先SSH快进发布脚本主线并下载公开原始字节确认摘要，再固定Dockerfile与节点source.json；节点更新模板字节不变。

根/CN同步、重定向合法/非法参数、旧单域名交互兼容、原生重试/导入、证书缺失停止、事务恢复、官方续签器迁移与未知修改保护四项smoke通过。新Panel与旧脚本的新增参数协议不匹配时明确拒绝；原有脚本调用保持兼容。apps契约diff为空，无apps提交。脚本配对回滚为298f6f2；KPanel回滚v1.8.1。

## 多维质量

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选网站契约；真实Agent PATCH经systemd和脚本更换正在提供TLS的证书，官方重定向配置由Agent发现 |
| 网络与供应链安全 | 已验证 | 固定脚本公开字节、参数边界、错误输出和材料保护；完整L3及公开OCI结果在完成段登记 |
| 稳定与恢复 | 已验证 | Nginx reload故障恢复旧证书、过期资源版本拒绝、续签忙碌失败后重试、Agent重启重新发现；无长期soak |
| 性能与资源 | 已验证 | 选行交互完成、无新增轮询；隔离容器768MiB/1CPU/pids512和宿主资源watchdog、OOM/restart检查；未重新测试无变化full-summary P95，不续期旧例外 |
| 用户体验与可访问性 | 已验证 | 实际FilesView三视口主题选区、14px代码、18种主题/视口组合采样对比度最低5.9285；网站两视口失败与重试；浏览器原生200%缩放、触屏与读屏器未实测，记录的是100/125/200%根字号布局变化 |
| 数据、配置与迁移 | 已验证 | 官方b3d0d35/014f450续签器迁移字节一致，自定义续签器不覆盖；无数据库schema变化；生产备份/SQLite/保护文件结果在完成段登记 |

## 隔离实机与浏览器

arena-154仅使用本轮隔离容器kpanel-v190-real-sites-9028-r1，fixture image sha256:9b05d7b3648fc2e72f557b0c4fe798246903c9d45d90fd9882555647fcd07e29。无宿主挂载、无外网、非privileged，systemd所需能力显式列出；真实Nginx以独立rootfs运行，固定Docker名称适配器仅转发nginx -t/reload，不冒称Docker Engine全覆盖。候选Agent从9028fab独立Linux clone用Go1.26.7编译，脚本为公开固定字节。10项真实TLS/迁移/错误/回滚/重启/HTTPS301检查通过，容器无OOM/restart，清理断言通过。证据C:/GitHub/_release-artifacts/v190-real-sites-9028-r1，输入摘要、commands、资源、结果和cleanup齐全。

实际续签入口验证自有证书不会触发ACME。官方rewrite.conf摘要f777e3472934732469cfe39cb1eaf44981e49889c117c37965f2983702726fe3，真实HTTPS 301保持/path/to/file?x=1&y=two。网络隔离下没有公网ACME签发、DNS变更或生产cron调度；这些原有链路未修改，不用本次离线证据替代它们。

后台浏览器环境local-release-v190通过本轮仓库外策略登记，仅允许browser-validation；本机Chrome/Playwright与canonical mock preview，页面代码均为9028fab。编辑器作业10452、网站作业17752终态passed/exit0；证据v190-editor-9028-r1、v190-sites-9028-r1。1280/768/390px、浅深主题、根字号1/1.25/2，首次点击、Shift/Ctrl扩选、拖动、删除撤销通过；网站1280/390px同域名拒绝、目标https规范化、失败本地化、重试保留、关闭清空通过。另外桌面实际FilesView的浅深两场景作业24552通过，复用已修正的三个辅助mock响应，证据v190-editor-desktop-9028-r1；未改候选代码。截图已查看；英文/繁中消息另由自动测试核对。mock不等于生产账户全功能浏览器验收。

## 自动门禁与发布状态

969e191候选因产品交互缺陷被替换，L3 r1在源码更新后主动停止，exit137属于精确旧Runner停止结果，不是生产OOM；保留原证据，不沿用。9028fab L3 r2的应用目录冷读测试测得50.474346ms，超过原50ms门槛；代码相对基线无变化，阻塞网络fetch由独立goroutine执行。同固定Runner定向连续30次通过，总0.341s。暂判断为墙钟计时受调度影响，未证明所有负载下均达标，不放宽门槛；完整r3于2026-09-09T04:32:20Z至04:47:28Z通过，exit0；三个L3原始目录各11项摘要均已核对，r1/r2失败证据保留。发布负责人2026-09-16前复核计时异常，原50ms阈值保留。

执行唯一run-release-l3.mjs，固定Runner sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3。完整L3覆盖Go全量、核心race、149文件1290前端测试、2208短语21目录、typecheck/build、双架构构建、源码与镜像扫描、rootfs生命周期；govulncheck可达漏洞0，模块未调用通告3，npm audit0。以下公开门禁均绑定产品SHA 9028fab55d950cedc6e59325a7a28ba6ce21bd1c并成功：

| 门禁 | GitHub Actions run |
| --- | --- |
| 候选CI / freshness | [34312422441](https://github.com/kejilion/KPanel/actions/runs/34312422441) / [34312422356](https://github.com/kejilion/KPanel/actions/runs/34312422356) |
| main CI / freshness | [34312737558](https://github.com/kejilion/KPanel/actions/runs/34312737558) / [34312737494](https://github.com/kejilion/KPanel/actions/runs/34312737494) |
| Release / tag freshness | [34313201443](https://github.com/kejilion/KPanel/actions/runs/34313201443) / [34313201438](https://github.com/kejilion/KPanel/actions/runs/34313201438) |

freshness报告生成2026-09-09T04:49:08.683Z，完整8/8源；直接/基座25项、传递归属125信号，紧急安全0，沿用既有采用期限。EOL最近2026-07-28，下次2026-10-28；当时最近每日security-advisories运行34199191688于2026-09-08通过。

[v1.9.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.9.0)于2026-09-09T05:09:24Z公开，非draft/prerelease。标签annotated object dcaa065850c46be53eb658ab42ba3ae6f3caaba7，指向产品SHA。公开8附件在远端及本地全部核对API摘要/大小；公开immutable OCI实际pull、标签、脚本来源及User 65532:65532核对，image-e2e于05:10:37Z通过。证据目录C:/GitHub/_release-artifacts/v1.9.0-public-9028fab-r1。

- 1.9.0/latest index：sha256:b6afec34cb271659d0ad937f06fa1d0e1593a606ccb0b91bdbd6da8fa3fa9db6。
- linux/amd64：sha256:ba64ce5fd2e89b2f77fa1b64a1ed0340a8b1feb17f46803d726ef21c4c6f6d6f。
- linux/arm64：sha256:5cc00368ca6a323c56aa54953606074a4370b4a93fc179b804b6297ade570e4d。
- SBOM/provenance开启，两个attestation manifest保留；附件完整摘要见公开SHA256SUMS和本轮metadata。

本次未升级Go/Node/npm依赖或基础镜像，仅受管脚本pin与产品version变化。

## 生产执行与回滚

用户明确授权上线流程。唯一生产arena-154/154.36.153.9，108/prod-108禁用全部操作，本次未连接。固定run-production-evidence.mjs preflight于04:13:27Z至04:13:29Z通过，旧1.8.1健康。backup及postdeploy实际argv先prepare-only，旧版、新版、baseline、候选SHA分别固定；占位digest只用于明确argv-only目录，从未执行生产，正式postdeploy已使用上述公开真实摘要，正式backup/postdeploy分别使用独立backup-r1/postdeploy-r1目录。

公开验收脚本本地/远端bash -n及上传SHA256 ba1a7902a0793522f4065751b6055822df9d50019286873e8fb5002906dd5bb0一致。闭环L0 argv/env、时间秒/毫秒与精度拒绝预检通过；验收文档生成单换行并以临时index预检staged diff。生产更新唯一标准入口为KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel。停写备份于2026-09-09T05:11:37Z至05:11:47Z通过，恢复包/root/kpanel-backups/pre-v1.9.0-20260909T051137Z，6个备份文件SHA校验、旧镜像实际load与旧服务恢复成功。标准更新exit0，Panel日志05:12:25Z启动1.9.0，Agent 05:12:23Z启动1.9.0。postdeploy于05:16:44Z至05:16:46Z通过；版本、精确revision和OCI匹配，容器running/healthy、RestartCount=0、OOMKilled=false，Agent loaded/active/running/enabled、NeedDaemonReload=no。protected.diff为空，panel/ai.db quick_check=ok，顶层ai.db为空文件如实保留；日志无fatal/panic/OOM。

preflight、backup、postdeploy证据分别在C:/GitHub/_release-artifacts/v190-production-preflight、v190-production-backup-r1、v190-production-postdeploy-r1；顶层证据摘要分别5/5、8/8、8/8通过。[arena公网HTTPS健康](https://kpanel.154.36.153.9.sslip.io/api/v1/health)于2026-09-09T05:17:35Z返回HTTP 200、version 1.9.0、status ok、initialized true。公开apps/main/kpanel.conf与候选归一全文相同，SHA256 7b5b52af0ff20cff4bebf114e747ddf1c82996500f2767ba8d3733217e83121c，标准更新提示apps Already up to date。

未执行回滚、紧急热修复或重复正式发布。保留v1.8.1标签、旧OCI摘要、配套旧脚本来源和上述停写备份作为恢复点。

## 发布节奏与首轮情况

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-09T11:36:19+08:00
- 候选冻结时间：2026-09-09T04:23:45Z
- 生产完成时间：2026-09-09T05:16:46Z
- 提交到生产用时：1.674167 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：10
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/git-archive/crlf-conversion",
    "position": "before-production-write",
    "count": 2,
    "impact": "Windows archive换行转换分别使smoke入口和提取的脚本函数无法在Linux执行。",
    "recoveryEvidence": "git -c core.autocrlf=false archive后的根/CN字节与Git blob一致，四项smoke通过。",
    "permanentAction": "本轮验证载体固定关闭archive autocrlf并在执行前校验原始脚本摘要；不改发布脚本逻辑或放宽摘要。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/linux-fixture/ephemeral-tmp",
    "position": "before-production-write",
    "count": 1,
    "impact": "跨WSL调用后/tmp验证目录消失，未运行有效smoke。",
    "recoveryEvidence": "改为专属/root/kpanel-validation持久路径，同步完整输入后通过。",
    "permanentAction": "本轮唯一脚本固定持久路径并在同一执行器完成提取和验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/editor-browser/unsupported-api",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮浏览器脚本调用不存在的EditorView.find，交互断言未开始。",
    "recoveryEvidence": "对照已安装源码改用findFromDOM，最终9028fab浏览器passed。",
    "permanentAction": "正式浏览器回归脚本随9028fab进入仓库，保持实际FilesView消费链。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/catalog-cold-read/wall-clock-jitter",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r2未改动冷读测试50.474346ms超过50ms，完整门禁失败。",
    "recoveryEvidence": "同固定Runner连续30次定向通过，完整r3 passed/exit0，候选与main CI成功；保留r2原始日志。",
    "permanentAction": "发布负责人2026-09-16前复核；当前属于计时调度疑似瞬时异常，未宣称永久修复。退出条件为原门槛完整L3通过且无复现；再次复现先修复可重复测量或实际性能，禁止仅提高阈值。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-diagnostics/shell-arguments",
    "position": "before-production-write",
    "count": 2,
    "impact": "探索性SSH Docker format/CLI help参数批次被拒绝，另一次桌面浏览器规格生成的PowerShell嵌套引号解析失败；均未执行业务写入。",
    "recoveryEvidence": "正式调用使用明确字符串参数、精确SHA和已知脚本schema；公开脚本语法/hash及闭环L0实际通过。",
    "permanentAction": "本轮唯一生产与L3参数固定为仓库入口；只读Docker format整体引用；桌面规格改用独立Node文件生成JSON并通过完整浏览器回归，复核v1.8.1/v1.7.0同类根因，不扩展参数白名单。",
    "historicalReleases": [
      "v1.8.1",
      "v1.7.0"
    ]
  },
  {
    "fingerprint": "preflight/git-ssh/identity-selection",
    "position": "before-production-write",
    "count": 1,
    "impact": "脚本仓库SSH初次未指定既有发布身份而被拒绝，未执行推送。",
    "recoveryEvidence": "复用KPanel已配置core.sshCommand，一次性-c参数成功验证main后快进脚本。",
    "permanentAction": "后续本轮脚本SSH沿用已验证发布身份，不更改全局凭据或读取私钥。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/production-evidence/artifact-directory-reuse",
    "position": "before-production-write",
    "count": 1,
    "impact": "正式backup首次复用prepare-only目录，被本地入口拒绝，未写生产。",
    "recoveryEvidence": "改用独立backup-r1目录，停写备份passed/exit0；正式postdeploy同样使用独立postdeploy-r1目录并通过。",
    "permanentAction": "本轮固定预检与真实执行独立证据路径，保留两份证据，不删除或覆盖预检目录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/public-artifacts/implicit-text-encoding",
    "position": "after-production-write",
    "count": 1,
    "impact": "backup开始后，本机二次校验release.json时Python默认GBK读取UTF-8中文失败；远端8附件与镜像验收此前已通过，未影响生产文件或升级。",
    "recoveryEvidence": "显式encoding=utf-8后本机8/8附件hash和size全部通过。",
    "permanentAction": "本轮公开JSON和文本解析显式指定UTF-8，Node使用fs utf8，Python指定encoding；未扩展为仓库通用重构。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

产品交互缺陷独立记录：发布前真实FilesView拦截，作者组件预览和原单元模型漏掉首次布局变化；9028fab永久修复并新增能拒绝旧候选的回归。未逃逸生产。r2失败和r1主动失效均不能冒称首轮全链通过。最近5版验收指纹已比较；已知参数、EOF、时间精度和本地回收限制在生产写前预检或保持边界。

本地磁盘启动约118GB可用；新工作树及原始证据保留。预览与临时测试容器按所有权停止，保留本轮node_modules、bundle和恢复资料，不清理他人活跃/未知内容；实际净回收0。不新增工作流、长期规范或记忆。
