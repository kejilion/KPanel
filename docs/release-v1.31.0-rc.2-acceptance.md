# KPanel v1.31.0-rc.2 发布验收记录

日期：2026-11-19

发布级别：L3；releaseChannel=preview；releaseTrain=1.31.0。

候选 / annotated tag：`60b40b65a523a3acea255f712639ef79ca611dc1` / `v1.31.0-rc.2`。可信控制面与批准基线：`49aa43e53a0802ac8dec7f517ed7cd5455f73b44`。

上一公开预览：v1.30.1-rc.3@f656291688401a7e5232f97659810d98ff0c95c8 / `sha256:4e7cfce82b4421acb11a9e574504aa430f5dbebe6b1a655f1887df0c8d4e9d93`。稳定回滚点：v1.30.0@cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa / `sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`。

## 范围与用户行为

- 可选 Tor 桌面改为 UID/GID65532、零能力、只读根文件系统，使用 Tini/TigerVNC/Openbox 与私有控制网络转发，无宿主发布端口或数据挂载；删除两精确 Dockerfile 的 root-init 扫描例外。新运行时标签为kpanel-browser-tor:16.0.1-kpanel.2，Tor官方包与两个既有重建助手的来源/锁不变。
- 已安装旧 Tor 组件如实显示旧镜像并提示修复。只有显式确认按钮才提交；取消或重复点击修复不隐式执行。修复沿用资源版本绑定后台任务，会重建容器并结束共享临时会话，Panel代码回退不会恢复旧容器。默认Chromium和安装后类型锁定保持原有行为，Tor仍仅支持Linux amd64。
- 构建、CI、镜像与固定发布Runner统一Node26.13.1 LTS及匹配@types/node；Go仍1.27.4、TypeScript仍6.0.3。Node属于构建工具链，最终产品仍是Go/scratch，不要求用户宿主机安装Node。Node暂缓例外在实际采用后移除；TypeScript7仍阻断Vue类型检查，保留真实失败及有期限评估。
- 新列车承接原1.30.1 RC的剪贴板上传、统一文件冲突、Tor与音乐列表/FLAC修复，不覆盖历史RC。文件覆盖/移动/目录合并改变实际数据，代码回退不能撤销；重要文件先备份。旧节点保留原能力，Panel/Agent/节点宜同步更新。
- scriptLinkageState=not-required，无需脚本发布。沿用kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff / SHA-256 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。

## 来源、独立性与真实证据

初始批准基线为已公开RC3 `f656291688401a7e5232f97659810d98ff0c95c8`。本轮四个输入分别是Tor `0ba996e52151a7d5ffcabb7047d8dbe44f754c3a`、Node `e0dd4e00a43e7a457a49a00abfe9ae62f30cca17`、兼容性评估 `578c00df8ca005ebb010208d088d2b4cb23a9163`和RC3验收记录 `16c1cef4db5cd87e761122aa3f000205284ac3a7`。初次准备阶段37个实现/配置Git blob与源候选逐一相同；仅组合共享的扫描身份与依赖策略，TS7评估保留而Node暂缓被实际采用替代，冲突原件与解析证明保留。

两来源的5份关键日志及TS7实际类型检查日志摘要已独立核对。Tor与Node的完整隔离L2分别绑定0ba996e5和e0dd4e00；源码完整性、模块图、12份actual/original生产闭包、双架构产物与工具链运行验证按原报告的范围保留，不借用源码L2冒充最终RC L3。

Tor实际Agent任务覆盖旧容器修复、停止/启动、重启、Openbox故障恢复、卸载/重装与人工旧镜像回退烟测；受认证Panel原生画面1280×720及未认证401/Origin403/CSRF拒绝通过。实际安装镜像`sha256:5ace22ad0dc123c8ea896d970a40ae0deeff02b856761879276a30a595259d94`的全进程非root、能力0、Seccomp2、只读、无宿主数据挂载/发布端口与出口网络VNC不可达已记录；固定Trivy0.74源配置/秘密与实际Tor镜像HIGH/CRITICAL检查通过，无Tor规则抑制。WSL验证使用了仅构建网络/代理适配，运行网络来自发布Compose，不声称已验证所有真实主机默认构建网络。

来源独立复核PASS WITH FOLLOW-UP，提供商fallback=provider-unavailable，主要实现者与当前发布任务分离。Node审查树3b90b8bf与e0dd4e00相同；Tor最终审查树85c2409f与0ba996e5只差第一方源摘要。原始OCR Node16/16、Tor18/18和后续非盲记录均保留，不将unreported补成0。最终版本元数据的自由臂与约束臂见本任务ocr原件及Git trailer，不冒充独立复核。

合同包安全盲审保留H0/M2/L0历史线索：控制桥主体缺少独立组件端认证为确认旧边界，MCP cron投影为待验证旧问题；保留既定责任人与2026-12-03复核期限。当前监听范围缩窄，未放宽专用控制网络成员或Panel Session/Origin/CSRF。本轮没有CF scoped/full审计，不声称全仓无漏洞、完整Firefox内容沙箱或匿名性保证。

## RC1失败与来源证明修复

RC1 `2dcfabf842961966e8b68eb7dcd81c0a5c7ed62e` 的run44499379870已通过候选/主线CI、hosted L3、原生镜像扫描与运行契约，并推送不可变双架构镜像sha256:ee6b974aa746467a9032c4ae356948f97aca699595bac93b95db3dc110f498fe。随后来源证明检查以Created container image identity mismatch停止；Release451197455保持draft，未提升preview。远端原件、完整日志和稳定/preview未变化回执保留在RC1交付目录，不使用RC1成功记录代替RC2精确L3。

独立修复任务交付 `49aa43e53a0802ac8dec7f517ed7cd5455f73b44`，两文件差异已由发布者独立审查；原作者已移交所有权。经典Docker的container.Image使用config摘要，containerd使用child manifest摘要，实际双存储/双架构提取已复现并修复。新检查把本地image.Id、不可变RepoDigest、平台与两个containerd描述符关联；保留registry config、nonce、未启动及Go二进制来源检查，不以宽泛OR或跳过校验恢复发布。42项测试与L1门禁通过，OCR双臂2/2、0发现；安全scope不适用，无CF审计。34份交付原件与26个registry对象的摘要由发布者独立核对，四组合只create/inspect/cp、未执行目标镜像，同架构二进制摘要完全一致。实际Docker验证使用RC1不可变镜像，联网依赖本机既有代理；临时daemon与自有容器清理回执已核对。

这项修复独立作者与发布者分离，同提供商fallback=provider-unavailable。其5类辅助环境/入口问题有8次失败调用，另有1次非阻断Go可用性探测和1次预期旧代码回归失败，分开保留、没有补写为0。RC2重新走精确候选CI、主线CI、hosted L3及公开验收。

## 多维质量与局限

RC2公开后的第一轮浏览器/容器验收尚未启动旅程即停止：Ubuntu26的sudo不支持保留整个环境，sudo -E被忽略导致RUNNER_TEMP缺失。原run44516987123整体conclusion=failure，发布前全部必需阶段和产物发布成功，公开元数据/双架构/19附件已通过，preview-completion因该失败skipped；不能称原流水线全绿。

只读测试环境修复`5bd45a24fe58d9ab9027b44f1ba7e32a9ad25271`由独立实现任务交付，发布者核对完整差异与回归原件后，精确[候选CI](https://github.com/kejilion/KPanel/actions/runs/44530502235)和[主线CI](https://github.com/kejilion/KPanel/actions/runs/44532028586)通过。仅测试调用以sudo env -i+固定PATH显式传递13项输入，秘密/BASH_ENV不传，缺项在依赖与Docker前拒绝；控制面白名单精确增加run.sh，与原三项测试资源一起保存四份全文/hash/diff回执，产品HEAD、VERSION和镜像不变。既有测试新增三次调用、11项外部required缺失、字节保留和越界拒绝回归，没有跳过原断言。

[只读公开验收恢复](https://github.com/kejilion/KPanel/actions/runs/44533596229)读取原run44516987123/attempt1的真实snapshot，不手工重造发布前状态。控制面为`5bd45a24fe58d9ab9027b44f1ba7e32a9ad25271`，被测产品仍`60b40b65a523a3acea255f712639ef79ca611dc1`和`sha256:a379ff245e3c101836ce3f0b950bef228a9fa862e4420c5a44b93c3f2a670758`。四份overlay回执的原/新全文、摘要逐一与这两个Git提交核对；原产品源树确有四项具名测试资源覆盖，不声称冻结源树clean。恢复整体success且四份实际浏览器/容器报告passed，未重建、覆盖或重复发布产物，没有制造RC3。本验收文档以最新批准控制面`5bd45a24fe58d9ab9027b44f1ba7e32a9ad25271`为本地文档基线，产品身份仍为tag绑定的RC2。

| 维度 | 已验证 | 未验证/限制 |
| --- | --- | --- |
| 业务与互通 | 源实际Panel/Agent生命周期、两来源L2、组合候选/主线CI及公开容器/浏览器 | 无双物理服务器新验收；Tor公网出口未重新验证 |
| 安全与供应链 | 非root/cap0、监听边界、无例外配置扫描、Node锁与受控镜像pin、实际发布扫描 | 专用控制桥仍须可信；无全仓无漏洞或匿名性承诺 |
| 稳定与恢复 | 重启/故障恢复、取消/版本冲突、旧容器修复与标准升级验收 | 切换后的健康失败仍需管理员恢复，不自动退回特权旧镜像 |
| 性能与资源 | 既有CPU/内存/PID/tmpfs/日志上限保留，构建/启动成功 | 低配冷构建峰值、多用户及长期吞吐未量化 |
| 体验与可访问性 | 浏览器设置10项测试、1440/390px Mock、键盘确认/取消、14px及页面错误0 | 成功分支在390px执行；无实体手机、原生缩放或完整读屏 |
| 数据与兼容 | 无新持久化schema，旧客户端可忽略可选字段，原文件冲突机制保留 | 临时Tor会话会重置；完整Windows动态文件语义及ARM64原生执行未重新验收 |

## 门禁、准备与版本冻结

- [准备候选CI](https://github.com/kejilion/KPanel/actions/runs/44510453985)与[准备主线CI](https://github.com/kejilion/KPanel/actions/runs/44511829104)均成功；本轮主线从2dcfabf8以普通快进到来源证明修复49aa43e53a0802ac8dec7f517ed7cd5455f73b44并SSH回读，没有强制改写。此前Node26/Tor准备4430ab9e保留；两次准备都先完成可信发布自动化，再构造对应RC，未在dispatch中执行候选独有工作流。
- 本地源码/工具预检、38项检测器/Runner测试、12组依赖策略与10项浏览器设置测试通过。本次辅助工具/环境失败及RC1发布失败的原件、次数与恢复见流程指标，没有省略首轮记录。初次准备与RC1冻结候选的Mock保留；RC2 UI实现内容及依赖只变化版本元数据，按精确blob映射复用既有1440/390px证据，不冒称它在RC2 SHA上重跑，也不代替最终RC2真实Docker运行。
- RC相对准备基线只改版本/Changelog/业务事实与扫描源身份，Go差异仅版本字面量。第一方1344文件摘要`0f04f2f29c613c9541a7749d1c8b8e0eadc14cc92fbb23bb4376aa958e824813`，闭包`9018045d7dda52d2fe280bd2ab58b4809466807724913b5aaa9841b506367a35`；构建输入继承已验证Node26版本，主模块图、38项finding处置、2026-11-06观察时间与2026-11-20到期时间不延长。
- [RC push CI](https://github.com/kejilion/KPanel/actions/runs/44513555588)成功；唯一[canonical发布](https://github.com/kejilion/KPanel/actions/runs/44516987123)的发布前必需阶段和产物发布success，随后公开fixture环境失败，整体failure；公开验收通过上述只读恢复完成。GitHub-hosted L3从2026-11-19T11:41:53.718Z至2026-11-19T11:56:13.832Z，go version go1.27.4 linux/amd64、Nodev26.13.1、Docker29.4.2；入口run-release-gate.sh、精确源码树及web/go/deploy三lane日志摘要已逐一核验。
- 主线/tag提升、主线Linux/Windows、版本镜像安全/运行契约、双架构成功；公开验收由恢复run成功，原preview-completion保持skipped。publish内部L3按既有流程复用已成功canonical L3，stable-completion因preview不适用而skipped，未称作新增测试通过。RC1与RC2各只有一次发布dispatch，另一次只读验收dispatch不重发或覆盖任何不可变版本。

## 实际公开产物

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.31.0-rc.2) ID451338887，公开于2026-11-19T12:52:03Z，draft=false/prerelease=true；annotated tag对象`9d10ff195978c2b71a3d6d5c81bc6c6c1a8aa0d1`指向`60b40b65a523a3acea255f712639ef79ca611dc1`。19附件名称、大小、digest均与冻结清单一致；实际公开正文与冻结原文逐字相同，SHA-256 `1a8f1a144a81aa12f83c404bb88d12aa166836e5b5e79a17a800645ff76ce222`。
- 版本镜像与preview同为`sha256:a379ff245e3c101836ce3f0b950bef228a9fa862e4420c5a44b93c3f2a670758`；linux/amd64、linux/arm64平台摘要及revision/version标签与候选一致，unknown条目为构建证明。GitHub Latest仍v1.30.0，Docker latest仍`sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`；独立实时HTTPS注册表查询确认，TLS未绕过。
- 匿名附件/OCI核对与真实隔离容器initial/restart、浏览器published/upgrade四份报告通过，共150项检查、0未预期错误，candidate/digest匹配且mocked=false。容器Docker库存和CA trust是明确夹具，不外推生产Engine/CA或真实宿主systemd安装。
- 旧RC3发现新RC1，更新弹窗版本/摘要/说明正确；no-systemd安装派发的受控503与清理是预期失败，不冒称生产升级。加入预览不自动安装，退出不自动降级。
- 原始artifact下载均与GitHub API digest核对，Docker构建记录按opaque格式保留；ZIP展开、API metadata、冻结文件、截图/JSON和完整原发布原始日志位于`C:/GitHub/_release-artifacts/v1.31.0-rc.2/canonical-44516987123-1/`；恢复原件与实际旅程在`C:/GitHub/_release-artifacts/v1.31.0-rc.2/canonical-44533596229-1/`。原始日志ZIP仅有本地SHA记录，API未提供digest，不虚构远端摘要验证。

## 分支与生产收尾

| 原ref | 原tip | 处置 | 恢复/保留依据 |
| --- | --- | --- | --- |
| fix/tor-rootless-runtime | `0ba996e52151a7d5ffcabb7047d8dbe44f754c3a` | 精确提交已纳入RC2，原任务所有权未声明释放，保留分支/工作树 | fix/tor-rootless-runtime@0ba996e52151a7d5ffcabb7047d8dbe44f754c3a；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| fix/node26-toolchain | `e0dd4e00a43e7a457a49a00abfe9ae62f30cca17` | 精确提交已纳入RC2，原任务所有权未声明释放，保留分支/工作树 | fix/node26-toolchain@e0dd4e00a43e7a457a49a00abfe9ae62f30cca17；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| fix/toolchain-compatibility-review-202611 | `578c00df8ca005ebb010208d088d2b4cb23a9163` | 精确评估已纳入RC2，原任务所有权未声明释放，保留分支/工作树 | fix/toolchain-compatibility-review-202611@578c00df8ca005ebb010208d088d2b4cb23a9163；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| docs/v1301rc3-acceptance | `16c1cef4db5cd87e761122aa3f000205284ac3a7` | 历史验收已纳入RC2，原工作树所有权未声明释放，保留 | docs/v1301rc3-acceptance@16c1cef4db5cd87e761122aa3f000205284ac3a7；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| review/tor-root-init-20261119 | `d97a7667934b9103be9cf3870377722337de54a2` | 未纳入；续期提案被实际非root修复替代，未知所有权原样保留 | review/tor-root-init-20261119@d97a7667934b9103be9cf3870377722337de54a2；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| release/v1.30.1-candidate | `f656291688401a7e5232f97659810d98ff0c95c8` | 旧RC序列历史保留，未授权废弃或改写 | release/v1.30.1-candidate@f656291688401a7e5232f97659810d98ff0c95c8；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| release/v1.31.0-candidate | `60b40b65a523a3acea255f712639ef79ca611dc1` | RC2已纳入；保留同序列唯一候选，RC1由其不可变tag/image及未公开草稿恢复 | release/v1.31.0-candidate@60b40b65a523a3acea255f712639ef79ca611dc1；纳入提交亦可从v1.31.0-rc.2祖先恢复，未知新增提交保留后续处理 |
| fix/v131-release-preparation-20261119 | `4430ab9ee0b37e89a35cad990c4148cc7e3afced` | 已原子归档并远端回读；本地重命名、解除旧upstream（如有），保留工作树与原件 | archive/fix/v131-release-preparation-20261119@4430ab9ee0b37e89a35cad990c4148cc7e3afced；不可变v1.31.0-rc.2亦可达 |
| fix/provenance-docker29-20261119 | `49aa43e53a0802ac8dec7f517ed7cd5455f73b44` | 已原子归档并远端回读；本地重命名、解除旧upstream（如有），保留工作树与原件 | archive/fix/provenance-docker29-20261119@49aa43e53a0802ac8dec7f517ed7cd5455f73b44；不可变v1.31.0-rc.2亦可达 |
| fix/public-verification-sudo-env-20261119 | `5bd45a24fe58d9ab9027b44f1ba7e32a9ad25271` | 已原子归档并远端回读；本地重命名、解除旧upstream（如有），保留工作树与原件；纯治理已纳入main及只读恢复，不在RC2产品镜像内 | archive/fix/public-verification-sudo-env-20261119@5bd45a24fe58d9ab9027b44f1ba7e32a9ad25271；当前批准main祖先、精确候选/主线CI已通过 |

旧Tor例外续期d97a7667未纳入，被实际非root修复替代。C:/KPanel是官网/演示站的独立本地仓库，最新提交仍在11月12日，本轮没有其新面板候选。未知/在用的源工作树和作者4173预览保留，仅停止本任务预览。RC列车与当前候选资源保留用于下一RC，不执行磁盘删除或宣称净回收。

生产部署、连接、备份、清理和prod-108操作全部不适用；本次只有GitHub SSH源码交付，无生产运维SSH。固定上一RC3或稳定版可作为人工代码回退点，但不会恢复已重建Tor会话或文件操作。验收记录在实际公开通过后形成独立本地文档候选，不为记录制造新RC、推送记录分支或新增工作流/知识库。

## 交付节奏数据

最早纳入时间包含11月14日的RC3验收补记，不把该历史间隔冒充本次执行或审计耗时。

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-14T19:03:48+08:00
- 候选冻结时间：2026-11-19T19:16:34+08:00
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
    "fingerprint": "preparation-tools/dependency-policy/incorrect-entrypoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "组合源码身份与38项定向测试已通过，但辅助依赖策略命令选错不存在入口，安全中断；尚未推送或发布。",
    "recoveryEvidence": "preparation-policy-first-wrong-entrypoint.json与preparation-targeted-node.log保留；按Makefile唯一入口report-dependency-freshness.mjs --validate-only重新检查。",
    "permanentAction": "后续工具入口先从当前Makefile或工作流读取，再执行并显式核对退出码；未修改产品或门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preparation-tools/browser-test/incorrect-working-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "辅助定向测试通过npm exec从仓库根目录启动，未加载web的Vite别名配置，0测试收集；原日志保留，未推送或发布。",
    "recoveryEvidence": "preparation-browser-settings-test.log保留；改用项目web package的npm run test入口，使用web工作目录与原Vite配置。",
    "permanentAction": "浏览器定向验证继续使用已有package script，不从仓库根目录绕过配置启动Vitest；不修改测试、别名或产品代码。",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication/provenance/docker-image-identity",
    "position": "before-production-write",
    "count": 1,
    "impact": "RC1 canonical run44499379870 completed source/Windows/L3/native image gates and immutable multiarch push, then modified-component provenance failed Created container image identity mismatch. Release451197455 remains draft, preview/latest unchanged; no production write.",
    "recoveryEvidence": "canonical-44499379870-1/workflow-logs/publish _ release/22_Verify and upload modified-component provenance.txt plus artifact API digest receipts and failed-public-state.json. Root cause qualification and minimal correction assigned to separate implementation task; RC1 tag/image/draft preserved, next version RC2 required.",
    "permanentAction": "Confirm actual Docker identity semantics and maintain immutable manifest/config/platform and owned-container verification; add bounded regression for supported stores. No retry/overwrite or weakened provenance.",
    "historicalReleases": []
  },
  {
    "fingerprint": "provenance-fix/wsl-default-user/docker-socket-permission-denied",
    "position": "before-production-write",
    "count": 2,
    "impact": "Initial docker info/version used default WSL user; permission denied at unix:///var/run/docker.sock",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-provenance-docker29-20261119/failure-ledger.json; All subsequent Docker probes use wsl -d Ubuntu -u root; four combinations pass",
    "permanentAction": "Use exact qualified environment/entry point and retain initial failure; no product checks weakened.",
    "historicalReleases": []
  },
  {
    "fingerprint": "provenance-fix/wsl-sh-nohup/daemon-not-created",
    "position": "before-production-write",
    "count": 3,
    "impact": "Shell background launch returned 0 but no daemon.log/socket; subsequent tail, docker info and docker create failed",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-provenance-docker29-20261119/failure-ledger.json; Foreground exec session with 1800s timeout and explicit namespace/sockets/paths; stopped cleanly",
    "permanentAction": "Use exact qualified environment/entry point and retain initial failure; no product checks weakened.",
    "historicalReleases": []
  },
  {
    "fingerprint": "provenance-fix/native-node-direct/registry-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "First fixed collectImage failed Registry request failed or timed out before Docker work",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-provenance-docker29-20261119/containerd-fixed-direct-network-failure.log; Existing local proxy at 127.0.0.1:10808 with Node --use-env-proxy; both live registry collections passed",
    "permanentAction": "Use exact qualified environment/entry point and retain initial failure; no product checks weakened.",
    "historicalReleases": []
  },
  {
    "fingerprint": "provenance-fix/handoff-evidence/timestamp-precision",
    "position": "before-production-write",
    "count": 1,
    "impact": "PowerShell ISO o emitted 7 fractional digits; preflight accepts <=3",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-provenance-docker29-20261119/handoff-qualification-timestamp-failure.json; Actual timestamps serialized to milliseconds; handoff-qualification.json passed",
    "permanentAction": "Use exact qualified environment/entry point and retain initial failure; no product checks weakened.",
    "historicalReleases": []
  },
  {
    "fingerprint": "provenance-fix/handoff-tools/go-path-not-inherited",
    "position": "before-production-write",
    "count": 1,
    "impact": "New exec invocation omitted temporary Go PATH",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-provenance-docker29-20261119/handoff-qualification-go-path-failure.json; Explicit pinned GOROOT/PATH in invocation; handoff-qualification.json passed",
    "permanentAction": "Use exact qualified environment/entry point and retain initial failure; no product checks weakened.",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/ubuntu-sudo-rs/environment-not-preserved",
    "position": "before-production-write",
    "count": 1,
    "impact": "RC2 publication succeeded but hosted public browser/container fixtures stopped before journey startup: sudo -E ignored and RUNNER_TEMP unset. Public assets/platform/provenance verified; full acceptance not complete.",
    "recoveryEvidence": "canonical-44516987123-1/workflow-logs/publish _ public-acceptance _ verify/10_Verify public release in real browsers and isolated containers.txt; public-release.json passed. Existing release-public-verify.yml read-only recovery must reuse original run44516987123/attempt1 snapshot and exact immutable product.",
    "permanentAction": "Minimal explicit environment allowlist for sudo and trusted controller invocation fixture binding; no assertions or artifact changes. Separate implementation/independent review and exact candidate/main CI before read-only recovery.",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-env-fix/python-stdin/unqualified-runtime",
    "position": "before-production-write",
    "count": 1,
    "impact": "Auxiliary editing entry failed before any repository mutation: Python stdin editing attempt exit1",
    "recoveryEvidence": "C:/GitHub/_task-evidence/kpanel-public-verification-env-20261119/auxiliary-failures.json; Stopped using Python without retry; performed same bounded edits with qualified Node26.13.1 successfully",
    "permanentAction": "Use the prequalified native Node entry; retain first failure and do not retry an unqualified runtime.",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-metrics/report-release-metrics/fingerprint-segment-count",
    "position": "before-production-write",
    "count": 1,
    "impact": "Final local acceptance record validator rejected a four-segment fingerprint introduced by the release evidence adapter; no public product/artifact changed and no document committed at failure.",
    "recoveryEvidence": "acceptance-metrics-first-schema-failure.json and acceptance-first-metrics-failure.md preserve initial document/error; source auxiliary record retained unchanged, canonical fingerprint normalized to three segments before revalidation.",
    "permanentAction": "Normalize the source failure identifier to the authoritative three-segment release metrics schema before generating the local acceptance record; keep initial failure and final validator receipt.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

来源作者已有失败/修复原件保留在各交付目录，不被覆盖或改记为本任务从未发生问题；缺少统一原始时序的工具尝试不推断为0。
