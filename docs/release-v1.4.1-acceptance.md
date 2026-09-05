# KPanel v1.4.1 发布验收记录

日期：2026-09-06

发布级别：L3

候选提交 / 标签：`7cb68c0303946cf9cffeb211af5d2651e52236ac` / `v1.4.1`。公开Release及arena-154生产部署均已验证。

上一稳定版本 / 回滚点：`v1.4.0` / `bfbd1bf0ff2bc0f6854fb4a522236911a6166615`。

## 发布画像

- 业务域：任务真源与保存恢复、文件窗口主机上下文、Compose外部编辑保护、AI有界附件与Retry、既有系统资源协议兼容、静态资源权限、轻量节点安装及自动更新恢复。
- 变更面：展示、宿主机写入、协议和数据、部署；L3。风险在异步操作错误成功、跨窗口错主机、配置被覆盖、附件内存、schema往返和节点更新事务。
- 未变化契约：不增加Panel/Agent权限、端口或配对授权；应用市场安装事务不改；普通已配对节点保留身份。`kejilion.sh` 为coupled，先发布兼容脚本再固定来源构建产品。

## 发布范围与未纳入内容

- 用户可见更新详见CHANGELOG 1.4.1；补丁修复，不增加独立业务功能。
- 精确产品提交范围：`0826d158e3f147d6f183b26d6eb6ca8dbfac48d5..7cb68c0303946cf9cffeb211af5d2651e52236ac`，共18提交；0826仅为上一版本验收记录，不算本版产品功能。
- 提交依次为9bd6a1b、48a7040、392cffd、eb5dfa9、4037ee6、c74aa5c、c6b98b8、a2e3998、7ea3098、8ecd472、75de928、d6f8a26、f862648、2c14a83、c4ad0b6、a9eb566、8c8ab76、7cb68c0。
- 未纳入：新System Center页面/routes/APIs/权限、P2审批/diagnostics、未提交改动及依赖升级。系统资源仅用户明确授权的既有五文件v3/v4兼容修复，不以此放行其他System Center内容。治理18ce47e独立队列，不混入产品tag。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集：`v1.4.1-light-node-lifecycle`。
- KPanel原内置脚本基线a74495cc3ea4bac1b0b42bf572f0f122ba9e2cf1 / SHA256 d9e2a75d97e5914c2fb3d163122e025dee8e1c757ae8515cc34ba6991b319974；本版实际内置3759e10692f61fe375c0424389842ddc70a73ed7 / SHA256 8c0edc5b97d9f7ec67edbfa6ed8dbbf473f9f651333334e2ac3c4cfb27bfe1dd。
- 脚本候选已发布为同一3759/SHA256，脚本版本号不改；source d028及发布修复3805/3759全部包含。
- 判定依据：实际改变轻量节点安装/自动更新/卸载协议与嵌入运行时；成对源码、旧公开资产及新正式资产真实systemd矩阵通过。
- 发布决定：脚本兼容修复先行，再发布KPanel固定脚本pin与节点附件；阻断或移除依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | A真实双Panel/双Agent正式配对、文件传输/撤权；B真实任务owner和Compose；轻量节点真实systemd矩阵 | 文件场景辅助Apps/Docker inventory有明确GET替身；轻量首次enrollment为合成HTTPS peer，不等于用户节点实测 |
| 网络入侵与供应链安全 | 已验证 | 精确SHA L3、race、govulncheck、npm audit、Trivy源码和镜像、固定脚本来源/摘要 | govulncheck依赖模块存在3项不可达提示，实际调用路径0；不是所有模块完全无通告 |
| 稳定性、失败恢复与兼容 | 已验证 | Docker保存失败不重放动作；Compose外部bytes保留；真实timer、SIGKILL并发、下载/校验失败和旧公开node往返 | 不承诺和任意未协调编辑器有原子CAS；旧残留锁节点仍可能需要本机恢复入口 |
| 性能与资源预算 | 已验证 | AI13例，工作集代理峰184.57421875MiB低于224MiB，256MiB/swap0/1CPU/64PID，无OOM/timeout | total峰约256MiB且max/PSI非零，不等于零回收、真实32MiB余量或长期SLA |
| 用户体验与可访问性 | 已验证 | 文件pending/ready/lifecycle、窄屏390/Escape、历史/迟到回调；Jobs同一页面详情自动轮询 | 仅受影响矩阵，不扩展为所有页面、所有缩放或所有无障碍认证 |
| 数据、配置与迁移 | 已验证 | 完整旧镜像→候选→旧→候选，8次SQL快照与7次Provider请求；原消息/附件/密钥/所有权保留 | schema10仅新增runs.retry_of；未知来源NULL保留，含附件未知Retry拒绝；极深手工JSON明确失败但不改原数据 |

## 自动门禁

- 最终L3：`v1.4.1-7cb68c0-l3-r1`，2026-09-05T17:09:07Z→17:24:08Z，passed/exit0。固定`run-release-l3.mjs`→固定Runner→`make verify-release`，未采用外部L3 wrapper。
- Runner：`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle `65ce39270c0b9291b9c56267c8a8936165a23490190cdb9fb28bae70dee84b29`；plan `16d6eb1e03d584468dc08b61f63083eee36f874995ec8429d67f05cb644e60fc`；remote script `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- L3日志SHA256 `8019105c29c27b68b7cbaab74a8a9d1fdb93dcbd4a50f01ff7edd8196b56bf66`，manifest `e30e3a64f7db532eab0ccfe43c675dc41cf88bf59b97cc5cd8b5a004eaa88225`。远端`/root/kpanel-release-evidence/v1.4.1-7cb68c0-l3-r1`，本地发布证据同run/remote-evidence；所有evidence.sha256逐项通过。
- 全量Go、前端138文件1191测试、typecheck、2191多语言短语、双架构编译、race、vet、安装安全、rootfs应用配置生命周期、managed-script和最终镜像Trivy通过。生命周期日志中故意的版本/健康/回滚拒绝是负向断言，终态app_conf_lifecycle=pass。
- 候选CI [33979998305](https://github.com/kejilion/KPanel/actions/runs/33979998305) SUCCESS；候选Dependency freshness [33979998335](https://github.com/kejilion/KPanel/actions/runs/33979998335) SUCCESS，均7cb完整SHA。
- 主线CI [33980875925](https://github.com/kejilion/KPanel/actions/runs/33980875925) 与主线freshness [33980876026](https://github.com/kejilion/KPanel/actions/runs/33980876026) SUCCESS；主线已普通快进到7cb。annotated v1.4.1已创建推送且commit为7cb。
- Release [33981212839](https://github.com/kejilion/KPanel/actions/runs/33981212839) SUCCESS，含实际源码验证、扫描、非root运行契约、多架构发布、SBOM/provenance、latest提升、公开Release及远端候选分支删除；公开镜像与生产终态均通过。

## 依赖与技术栈变化

- 候选dependency-report生成2026-09-05T17:08:27.795Z，检测源8/8，直接/基座行动项23（compatible-patch6、minor14、major3），传递归属信号121；emergency-security0、需处理例外0。EOL current，截止2026-10-28T23:59:59.999Z。
- push事件security-advisories跳过；最近每日通告审计[33951609449](https://github.com/kejilion/KPanel/actions/runs/33951609449)于2026-09-05T07:05:53Z创建，security-advisories job101267347933实际SUCCESS，report跳过。其源码732eac2与本候选的go.mod/go.sum/策略/安全脚本/通告工作流无差异，保留历史SHA、不冒称7cb同SHA每日审计；7cb实际调用路径govulncheck/npm/Trivy另有通过证据。
- 本版不升级依赖/Action/基座；报告发现候选不代表已接受，常规候选按现有依赖策略时限独立评估，不机械逐项升级传递依赖。
- 唯一受管脚本采用`kejilion/sh` main `3759e10692f61fe375c0424389842ddc70a73ed7`；root SHA256 `8c0edc5b97d9f7ec67edbfa6ed8dbbf473f9f651333334e2ac3c4cfb27bfe1dd`，node updater `b75a6504ea51603f23080ff2401d1c8a43b1a15e274c3953a75e0b9efad91484`。Dockerfile/source.json/外部来源文档一致，根/CN同步及20项进程回归通过。

## 隔离真机与浏览器验收

- 仅arena-154；原始失败证据不覆盖。下列组件证据按实际源码SHA保留，不将c4ad/2c14重新标记成7cb实测：c4ad..7cb对internal、cmd/paneld、cmd/kejilion-agent、web与image-e2e无差异；7cb最终完整L3独立新跑。
- A：`v141-c4ad-file-r5` / job `arena-154-1541444`，passed/exit0，16:12:18.413Z→16:13:09.184Z。10顶层检查，pending3/ready3/lifecycle8，真实双向正式配对、3传输、撤权和无本机fallback；4容器OOMfalse/restarts0、cleanup=true。fixture包`fa101de56a3397b5113f57d750d933a69ec12d8c15e567a190328de82bcec58b`，spec `6e072ff333cf3545278011e3097528260c9a172fb43007b2b2623357906df64e`。r1至r4失败保留，403/409后的同host精确一次GET仅负向步骤允许，无全4xx白名单。
- B：`v141-c4ad-b-ui-r1` / job `arena-154-1555761`，16:33:46.293Z→16:36:39.228Z passed。真实Panel/Agent/Docker，同page/dialog、同docker owner从persistence_pending恢复为failed，fresh资源任务503，upCalls=1且外部Composebytes保留；无动作重放；精确清理。原2c14真实API13例/owner4组是未变源码的补充证据，非最终UI替代。
- C：`v141-c4ad-image-r1`，4完整镜像启动v1.4.0→c4ad→v1.4.0→c4ad，7Provider请求、8SQL快照、schema9→10、legacy NULL Retry拒绝，所有容器exit0/OOMfalse/restarts0/swap0，峰149757952B、cleanup通过。AI正式压力`v141-2c14-ai-r1`13例通过，两个Retry各保留8361704B附件，每请求11172547B；前7ea/75de超224MiB失败及诊断原件保留。
- 既有系统资源：`v141-protocol-c4ad-native-r3`，25离线测试、ENVIRONMENT_PROBE_PASS/REAL_AGENT_PROTOCOL_PASS/CLEANUP_PASS，实际Agent19请求、未授权401、6能力、Hosts增读旧版本409删及原bytes/owner/mode恢复、12调优ID读取、端口扫描、防火墙resourceVersion与过期409、3国家操作503确因v3要求v4。普通防火墙写=false、无新System Center UI覆盖；没有宿主模块/包安装。私有namespace、512MiB/swap0/1CPU/128PID/180秒，实际5秒67MiB。r2空默认表基线失败保留，r3先记录cold/primed并严格恢复。
- 轻量节点：最终`/root/kpanel-light-systemd-7cb-r2/lifecycle-r2`，真实systemd PID1、节点二进制、锁、权限、进程inode和timer，8项通过；768MiB/swap0/1CPU/512PID，无网络/无host mounts/非privileged/私有cgroup，OOMfalse/restart0、最终owned容器清理通过。基础镜像不可变`sha256:be6bd104fb79562b2ea6d563047e94f5d3a04bddb4c0c258ad70d13439b2239c`；沿已批准fixture的SYS_ADMIN/DAC_READ_SEARCH/SYS_PTRACE与安全配置，仅隔离guest，不增加生产权限。
- 轻量节点八场景：无主锁续装、旧公开v1.4.0资产不覆盖新updater、实际timer升级、并发join/update/uninstall及SIGKILL恢复、断网/摘要失败保持身份与二进制、卸载保持锁inode、全新安装/单次身份续装及自动更新、最终卸载。下载为控制的离线Release传输，fresh enrollment是严格HTTPS合成peer，不是实际PanelUI/用户节点链路。
- 轻量旧资产SHA256 `6c91acadf01396268734faa8f0f8be5c4d6989aa8d0d75dcd5df36e2b1cba4f6`；7cb原生候选`46123e86d4f9029dd416f272813f26b19442c890f7f28056133a3e966daed662`。正式新公开资产`fb39dc753a23753bd03ed3f0729360f5093b05e8926d12c0a72860ca0d3d7c21`另行实际下载，不能冒称与原生候选字节相同；`/root/kpanel-light-systemd-v141-public-r1/lifecycle-public-r1`按相同八场景重新通过。unit kpanel-light-systemd-v141-public-r1成功，38.964秒；guest768MiB/swap0/1CPU/512PID、OOMfalse/restart0，cleanup passed、remaining为空。原始公开资产/脚本/fixture摘要与全部命令、8项结果、guest状态均保留，边界仍为离线传输+合成HTTPS peer。
- 原systemd r1发现resume仅start导致旧PID仍运行：已修为3759的resume update/restart并以最终r2通过；不是首轮通过、不是生产回滚。原8c8 normal/permissions/rollback三个真实场景通过作为未变源码辅助，不扩展其旧guest未固定swap的资源声明。

## 发布产物与公开仓库复核

- [GitHub Release v1.4.1](https://github.com/kejilion/KPanel/releases/tag/v1.4.1) 于2026-09-05T17:40:26Z公开，非draft/非prerelease；annotated tag对象b04d0418b2195096b210caadb1392ad48146ada9，peel为7cb完整提交。
- Docker版本与latest OCI index：`sha256:ae7d997f0e3dd565ffbbcd8ed63c552cb8c64e2462ec436665fb9ceebab3e9d3`；amd64 `sha256:fcacd03e1a2d22bbc428cb83ac453b8399e490aab92b5e1ba5925fd8f11488ac`；arm64 `sha256:e6f4920052e4f3cd84b73950704e275f9fcdd80374a58076342083f779eabfd7`。两个unknown/unknown为关联平台attestation，不是缺少架构。
- 公开八附件已实际下载；SHA256SUMS的五项全部OK，校验单自身SHA256 `cc766dca9d93684df0420ff69a862b8930bccf7ebf37066b9f165e3987939a3a`与Release元数据一致；LICENSE/THIRD_PARTY_NOTICES与源码逐字节相同。
- Agent amd64 `0a7be829f5dbec01dc5a087939491bfb74c7798dcabd974e255517835b9dab48`、arm64 `a8cc104708093f20ac4fefa7554c224152f25c028bf79a7907db5cf4d935af89`；node amd64 `fb39dc753a23753bd03ed3f0729360f5093b05e8926d12c0a72860ca0d3d7c21`、arm64 `3568cee6ca90e5ad078df6cbe940f7f1d026cbb2ee593e1dbf585138c9d1d468`；deploy archive `0168d559889bc79fbacc5f51003ad91b23449186e734fbfca627317ef79e0f56`。
- 公开镜像显式pull后OCI revision7cb、version1.4.1、script3759/8c0、user65532:65532全部一致，固定`packaging/tests/image-e2e.sh`输出image_e2e=pass（含实际图片读取/字节、bootstrap安全cookie及health），完成时间2026-09-05T17:42:02Z。证据`/root/kpanel-release-evidence/v1.4.1-public-7cb-r1`已完整复制本地同名证据目录。未将L3本地镜像当公开产物。
- apps无契约变更：规范文件/本地文件blob `abf0efd22876f34aa3731f5b6d8ba04e373b965e`，本地clean，远端main `2d8044adec98e3eb16f47cdbb297f6be9632a66f`，公开raw HTTP200换行归一字节一致，无需apps提交。
- `kejilion.sh` coupled已先发布main3759，不改脚本版本号；固定raw校验和短入口curl UA交付/CN代理内容核对通过。浏览器UA返回说明网页属既有内容协商。没有在生产执行下载脚本作测试。

## 生产部署安全核对

- 生产/验证唯一arena-154；prod-108及108禁用全部KPanel操作，本次未连接、未备份、未部署、未核对。
- 新preflight `v1.4.1-production-7cb-r2` PASS，2026-09-05T17:16:47.059994451Z线上1.4.0/ok，revision bfbd1bf、RepoDigest/ID `sha256:9f70d341db62e16376f281b301c4031fc1bfb92e94b289b2c20facd9a764137b`。旧pre-v1.4.0备份不作为本次回滚备份。
- 本次backup passed，停写归档并验证旧镜像可load，备份`/root/kpanel-backups/pre-v1.4.1-20260905T174318Z`；kpanel.tar.zst SHA256 `4985da6e3c9f323e974277d1248849cea17b7781ad5c6b531f231ac0acdfc6e6`，old-image.tar.zst `616dc833c665d53447fcff992eb10af9be5aad3cfc44aac80ffb165ea79069de`，其余服务/安装标记/旧inspect及SHA256SUMS一并保留。备份后服务恢复1.4.0且受保护文件与preflight相同。
- 标准应用市场更新exit0，原生update完成且访问策略恢复。deploy log SHA256 `e3fddf870039d3e75a65be77beca438f7866addf61db0d48616dddafdff1420f`；没有临时改生产wrapper/直接替换容器。
- postdeploy `v1.4.1-production-7cb-r2`，17:44:38Z→17:44:40Z，passed/exit0；version1.4.1/revision7cb/公开ae7d digest一致；Panel running/healthy、restart0/OOMfalse，Agent active/running/enabled/NeedDaemonReload=no；protected.diff为空，panel/ai.db quick_check=ok，顶层空ai.db标记empty；近10分钟日志无致命签名。公网health 1.4.1/ok。原始快照保留于对应production-postdeploy证据。
- 生产写操作仅备份时停写/恢复、标准更新、安装入口自身的访问策略恢复；并发杀进程、校验失败、用户身份卸载和旧镜像降级均仅隔离环境执行。
- 固定生产入口`run-production-evidence.mjs`，标准更新`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，不手动替换生产容器。

## 回滚

- 保留v1.4.0/tag和上列9f70镜像；真正回滚须配套本次停写备份的数据库、密钥、配置、Agent、镜像后启动并验health/OCI/受保护文件/SQLite。
- 未发生生产回滚，生产实际1.4.1健康；GitHub Latest、Docker latest及标准入口指向1.4.1，不存在失败版本继续默认分发的情况。历史tag/镜像/证据未移动或覆盖。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-05T13:26:19+08:00
- 候选冻结时间：2026-09-06T01:08:47.496+08:00
- 生产完成时间：2026-09-06T01:44:40+08:00
- 提交到生产用时：12.305833
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：26
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "testing/ai-pressure/build-directory-ownership",
    "position": "before-production-write",
    "count": 1,
    "impact": "C首轮源码预先chown后cap-drop编译不能写，业务未执行。",
    "recoveryEvidence": "v141-7ea-ai-r1-remote；r2编译通过",
    "permanentAction": "夹具取消编译前chown，测量阶段仍65532/原预算。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/image-compat/exact-absence-parser",
    "position": "before-production-write",
    "count": 2,
    "impact": "完整镜像r1/r2前置不存在判定不完整。",
    "recoveryEvidence": "v141-2c14-image-r1/r2；r4完整往返通过",
    "permanentAction": "精确name/返回码/诊断匹配与18正负例，未用广泛忽略。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/image-compat/internal-network-endpoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "内部Docker网络不提供原host发布端口，r3连接拒绝。",
    "recoveryEvidence": "v141-2c14-image-r3；r4及c4ad-r1通过",
    "permanentAction": "改为owned私网IP直连，Host/Origin保持PUBLIC_URL。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/file-context/agent-readiness-profile",
    "position": "before-production-write",
    "count": 1,
    "impact": "无Docker socket的文件Agent被完整健康检查误判不可用。",
    "recoveryEvidence": "v141-2c14-file-r1；最终c4ad-r5通过",
    "permanentAction": "专用readiness只接受唯一docker_unavailable，仍要求真实文件能力/字节。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/file-context/diagnostic-evidence-gap",
    "position": "before-production-write",
    "count": 1,
    "impact": "2c14文件r2只记录错误hash且诊断可能覆盖主错误。",
    "recoveryEvidence": "v141-2c14-file-r2；后续r3原始诊断与最终r5",
    "permanentAction": "有界脱敏path/status和primary/secondary区分，保持不吞console策略。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/file-context/jobs-activation-fixture",
    "position": "before-production-write",
    "count": 1,
    "impact": "Jobs未实际激活，业务场景缺GET/jobs；辅助库存与origin边界不完整。",
    "recoveryEvidence": "v141-2c14-file-r3；最终c4ad-r5",
    "permanentAction": "实际点击激活Jobs，辅助GET替身逐项声明，loopback安全origin；不mock静态图或文件权限。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/file-context/federation-public-origin",
    "position": "before-production-write",
    "count": 1,
    "impact": "c4ad-r1 loopback PUBLIC_URL导致联邦poll覆写private callback。",
    "recoveryEvidence": "v141-c4ad-file-r1；r2/r5实际配对通过",
    "permanentAction": "固定private peer PUBLIC_URL，独立loopback纯TCP转发与原安全断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/file-context/negative-followup-contract",
    "position": "before-production-write",
    "count": 2,
    "impact": "c4ad-r2的mock403与r4的真实409之后一次GET回读未纳入负向诊断。",
    "recoveryEvidence": "v141-c4ad-file-r2/r4；r5完整passed",
    "permanentAction": "精确POST后同host/同status/2秒内一次GET，反例回归；不全量豁免4xx。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/file-context/stale-evidence-binding",
    "position": "before-production-write",
    "count": 1,
    "impact": "c4ad-r3 allocation仍指向旧result，前置拒绝，checks0。",
    "recoveryEvidence": "v141-c4ad-file-r3/state.json/browser-test.log；r4准备回归",
    "permanentAction": "root/evidence/unit/command绑定回归，旧文件未覆盖，r4/r5使用新run。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/system-protocol/python-site-customization",
    "position": "before-production-write",
    "count": 1,
    "impact": "native-r1宿主sitecustomize指向/etc，来源边界拒绝。",
    "recoveryEvidence": "v141-protocol-2c14-native-r1-evidence；c4ad-native-r3通过",
    "permanentAction": "只排除可选宿主定制模块，保留stdlib来源限制；精确owned目录清理。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/system-protocol/empty-nft-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "c4ad-native-r2冷空规则与首次初始化ACCEPT链基线不等。",
    "recoveryEvidence": "v141-protocol-c4ad-native-r2；r3 cold/primed/changed/restored",
    "permanentAction": "先记录并精确断言默认空表，增加离线回归和原始前后规则，不抹除r2失败。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/browser-tools/os-release-symlink",
    "position": "before-production-write",
    "count": 1,
    "impact": "native Chrome工具首轮复制os-release链接无法形成完整工具根。",
    "recoveryEvidence": "v1.4.1-browser-tools-2c14-native-r2-evidence及保留r1",
    "permanentAction": "同一精确CID按已核链接目标复制，独立r2工具hash及真输入烟测。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/native-go/archive-executable-mode",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows归档丢失自有Go工具执行位，version Permission denied。",
    "recoveryEvidence": "kpanel-native-go-2c14；后续B/C实际编译与版本通过",
    "permanentAction": "仅修复自有bin和8个tool文件执行位；今后供应先version预检，不改系统权限。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/verify-change/wsl-docker-permission",
    "position": "before-production-write",
    "count": 1,
    "impact": "AI L2首轮WSL无Docker权限，Go/Web通过不能代替完整L2。",
    "recoveryEvidence": "d6 L2-r1保留；自有root Linux clone L2-r2通过",
    "permanentAction": "使用固定授权Linux环境，不修改宿主socket权限。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/verify-change/windows-linux-toolchain",
    "position": "before-production-write",
    "count": 2,
    "impact": "静态资源修复及轻量修复的Windows验证不能执行Linux/install或缺Go/gofmt/make。",
    "recoveryEvidence": "c4ad及8c8本地日志；最终7cb固定Linux L3 passed",
    "permanentAction": "Windows仅源门禁，完整验证走固定Linux Runner；不是Windows L2完成。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/light-node-tests/module-cache-selection",
    "position": "before-production-write",
    "count": 2,
    "impact": "namespace Go r1无GOPATH/cache，r2选中缺依赖cache且GOPROXY=off。",
    "recoveryEvidence": "kpanel-light-lifecycle-draft-r1/go-test.log及go-test-r2.log；r3/r4通过",
    "permanentAction": "显式固定既有Go1.26.7、完整modcache和自有buildcache，不现场安装宿主工具。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/production-evidence/phase-specific-fields",
    "position": "before-production-write",
    "count": 1,
    "impact": "preflight误传postdeploy revision/digest，远端参数拒绝exit2，无生产检查或服务写。",
    "recoveryEvidence": "v1.4.1-production-7cb68c0-20260906-preflight；7cb-r2 passed",
    "permanentAction": "按固定工作流phase字段重新调用；发布负责人2026-09-07复核JS前置字段一致性，退出条件为拒绝错误phase字段回归通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/background-browser/execution-host-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "B c4ad Linux规格误在Windows启动，2ms ENOENT，产品未运行。",
    "recoveryEvidence": "保留本地错误状态；v141-c4ad-b-ui-r1 arena passed",
    "permanentAction": "environment标签不等于SSH，按固定远程命令入口执行并核实际runtime；不新建包装器。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/protocol-l2/base-image-download-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "系统协议来源候选L2固定Bash镜像下载超时。",
    "recoveryEvidence": "system-protocol-evidence-20260905首轮与同digest补缓存后L2",
    "permanentAction": "上游有期限例外：发布负责人2026-09-07复核，仅同digest成功且无持续失败退出；不换源或放开摘要。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/arena-ssh/transient-connect-timeout",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次SSH连接超时延后协议失败证据取回/owned目录清理，公网服务正常。",
    "recoveryEvidence": "执行计划14:08Z失败与14:09Z恢复，native-r1 cleanup PASS",
    "permanentAction": "上游有期限例外：发布负责人2026-09-07复核，SSH稳定且精确取证/清理完成退出；不改代理或生产网络。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/run-repo-bash/unsupported-env-argument",
    "position": "after-production-write",
    "count": 1,
    "impact": "本地收尾验证误将VERIFY_BASE_REF交给只允许VERIFY_LEVEL的--env，入口拒绝，未启动Bash或操作生产。",
    "recoveryEvidence": "改用已支持的位置参数 scripts/verify-change.sh 7cb68c0完整SHA，123项治理测试及change-aware门禁通过。",
    "permanentAction": "未改或绕过入口；既有parseRunArguments测试确认--env限制与SCRIPT [ARG...]传递。发布负责人2026-09-07复核下一次L3调用规格预检，退出条件为精确argv的无执行解析回归通过，下一L3生产写前必须完成。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->


- 首个纳入提交按作者时间计，候选冻结取最终L3 manifest时间；本版正式产品仅一个稳定tag/一次生产发布。发布前连续修复未成为重复正式版本。
- 已知产品前置缺陷：AI工作集超预算、严格checkout静态文件0600、轻量resume旧PID；均在生产前修复，未以重跑洗成首轮通过。
- 流程异常按已记录的21类26次统计，不声称覆盖未记录的普通诊断命令；生产写后仅1次本地收尾调用参数拦截，没有生产影响。已逐项比较最近五版验收中的原指纹：tag基线/role/freshness/CRLF/scale-fixture/template-rewrite/验收机器路径等，无本批同指纹跨两版复发；本版同类负向回读、缺工具/缓存等重复仍在明细保留计数，不能降成一次。夹具修复已有回归；不可控上游问题保留负责人、复核日和退出条件。

## 遗留风险与后续准入

- 本地资源回收：本次未执行本地递归删除，净释放字节未测量；保留专用候选、验收原件与恢复资料用于回滚/追溯，不清理其他任务或未知目录。隔离guest/网络/进程按每组ownership和cleanup记录已释放；不把容器清理当本地磁盘净释放。
- 旧自动更新器仍被残留锁挡住且没有获得恢复版时，需要本机恢复入口；不承诺中心免接触修好所有离线节点，不要求清除身份重新配对。
- 真实用户节点、ARM64原生设备以及任意第三方超深附件数据不在本轮物理覆盖；ARM64编译和正式镜像架构核验与amd64实机分开记录。
- 常规依赖候选和治理候选独立处理；不增加System Center上线范围。
