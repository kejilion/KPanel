# KPanel v1.8.1 发布验收

日期：2026-09-08。发布级别：L3。

候选：c2d47e7c76e26332c9bbd31cb0e28986f50b3f2e；上一稳定版 v1.8.0，源码435ae91f25de24e997aabb3414bf60f1e7eb1e10，OCI sha256:86112e0e2549be443b50b80193068be3603f6bcdf5004d421099db77f3098a58。

## 范围与跨仓库

基线main为3a490f5f5286d4af6eaa5e28bdcb492233293c35。仅纳入c1fea7063dec6ac2d040597c5b97746400166751，重放27308e8；版本准备c2d47e7。git cherry确认该功能分支之前四个提交均等价上线，不重复移植。候选16文件增量包含版本与Changelog；与旧tag比较额外含已有主线v1.8.0验收文档。

减少Docker/应用市场合法镜像检查的磁盘写入和Docker Engine查询成本，定位应用与实际检查共用两槽并发和20秒截止。Docker Engine继续作为真实镜像来源，只读列表快照不用于写操作授权；启停、重启、升级审计和资源版本保护不变。没有系统中心页面、路由、API、权限或业务文档变更；未纳入其它候选或未提交工作树。

scriptLinkageState=not-required（无需发布脚本（不适用）），跨仓库变更集与脚本候选不适用。实际Dockerfile脚本commit298f6f23751e36726660d73b5c0c83aef1b404f4，SHA256 80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581。本次不更改脚本协议、运行时动作、宿主产物、安装路径或内置脚本；L3脚本契约及生命周期作为兼容证据。不提交sh/apps；公开apps/main/kpanel.conf与候选换行归一全文一致。

## 发布画像与质量边界

- 业务域：Docker、应用市场、Panel只读代理；变更面为只读和版本发布。测试更新状态、真实运行镜像、请求次数、取消、繁忙/传输失败和写操作审计。
- 安全：保留Session、CSRF、Origin、固定Agent路径、输入验证、两槽并发和总超时，不新增出站来源或特权动作。固定Runner扫描、公开镜像和资源契约另行取证。
- 性能：精确候选模拟Engine 200容器/20次应用检查验证100次请求（旧路径4100），不能换算为生产CPU降幅。合法检查成功/失败/繁忙时账户审计文件字节、inode和mtime不变，真实操作仍写intent/result。
- 稳定性：取消释放两个共享名额，嵌套查询不重置截止，真实Docker移动本地tag后仍比较容器实际镜像。无长期soak声明。
- 用户体验：前端业务、布局、语言、焦点未改，无新增视觉矩阵或本地预览要求；不将旧mock浏览器证据冒充本候选生产体验。
- 数据/配置/迁移：无Schema迁移、无需重新配对。备份与受保护文件、SQLite校验走固定生产证据入口。
- 原full-summary路径未改，本次不重新测试整机聚合P95，不续期或扩大旧性能例外；全平台辅助技术和用户真实业务写操作未验证。

## 自动与隔离验证

本地独立Linux clone绑定c2d47e7，WSL Docker29.6.2、Go1.26.7；定量成本、鉴权、预算和审计回归通过。真实回环零层镜像fixture验证current→移动tag→available，容器身份不变、清理断言通过，未使用现有用户容器。证据C:/GitHub/_release-artifacts/v181-targeted-c2d47e7，命令规格v181-targeted-validation.sh。该层不代替L3或生产证据。

固定L3入口run-release-l3.mjs，run v1.8.1-c2d47e7-l3-r1，不可变Runner sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3。L3首轮passed/exit0，08:23:40Z至08:39:48Z；全量Go、核心race/vet、148文件1280前端测试、2208短语21目录、typecheck/build、双架构、源码/最终镜像扫描与rootfs生命周期通过。11项evidence.sha256独立核对通过；L3日志SHA256 79df6cdd2eca11be90e7ca07bf3494f1f89604c67383365f759262acbaa92bf6。bundle4a6266b3794e3e8535078e3d0bcaa289cfd6278f1ddac79e642882f4f55becc8、plan c7d93a57f30d9d8f6fe9ba5e70b58ec8286b5d31f482467b4077e6a1413afc8f、remoteScript d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c。另有appmarket race3.277s通过。候选CI34205778627/freshness34205778650、主线CI34206391148/freshness34206389912全部success，均绑定c2d47e7。annotated tag对象fc9e32d868f788b8815ff474d395ec38c592d9e4。Release34207150769及公开产物均通过，详见完成证据。未修改依赖、Action、工具链或脚本pin；版本锁文件仅改产品version。freshness生成08:40:53.153Z，8/8源完整，直接/基座21项与传递归属124信号；无紧急安全项，不因新报告重置既有采用期限，不把本轮产品优化扩成依赖升级。每日security-advisories34199191688在main3a490f5于07:24:45Z至07:25:34Z成功；EOL最近2026-07-28，92天周期内。govulncheck当前可达0、未调用模块通告3、npm0，不声称全部依赖无通告。

## 执行方案与生产

唯一正式目标arena-154，环境策略允许候选、生产部署及安全核对；108/prod-108禁用全部操作，本次未连接、未备份、未部署、未核对。用户已明确授权上线流程。

preflight run v1.8.1-production 于08:22:05Z→08:22:06Z passed，旧1.8.0健康，Agent loaded/active/running/enabled、NeedDaemonReload=no。备份与postdeploy均通过，详见完成证据。生产入口仅run-production-evidence.mjs各阶段与标准KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel。

生产写前复用完整argv/env、秒/毫秒时间和拒绝非法精度预检，59项生产入口与指标回归通过，backup prepare-only通过。公开产物脚本本地/远端bash -n通过，上传hash024b3bd1952fad04bf601225bae03dccd58504ae3b32ed87f5f2fa9a8b442440一致；正式发布后必须实际下载8附件、核对size/digest/SHA256SUMS、公开双架构index/latest和image_e2e。文档生成使用trimEnd+单换行，并以独立临时index预检真正staged diff。

回滚保留旧tag/OCI及本次新备份；恢复完整镜像、Agent、数据、密钥和配置后重新postdeploy并判断GitHub Latest、Docker latest与标准更新入口公共指向。备份路径、公开产物和生产结果见完成证据。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-07T20:51:56+08:00
- 候选冻结时间：2026-09-08T08:23:20Z
- 生产完成时间：2026-09-08T09:07:11Z
- 提交到生产用时：20.254167 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {"fingerprint":"preflight/release-diagnostics/shell-arguments","position":"before-production-write","count":1,"impact":"公开验收脚本语法预检误将-n传给只接受script路径的仓库适配器，在执行前被拒绝；未运行生产命令或更改候选。","recoveryEvidence":"按已确认Git for Windows路径直接运行bash -n，同一公开验证脚本本机和arena语法通过，上传SHA256一致。","permanentAction":"本轮冻结语法预检为明确Git Bash可执行文件的-n入口，生产门禁继续走原run-production-evidence；保留原错误，不扩大run-repo-bash参数白名单。识别v1.7.0同类调用复发，完整闭环L0的argv/env正负回归另外已通过；修复本轮唯一预检调用，不声称新增通用治理。","historicalReleases":["v1.7.0"]}
]
<!-- kpanel-release-process-incidents:end -->

已比较最近v1.8.0/v1.7.0/v1.6.1/v1.6.0/v1.5.0原始验收指纹；本轮未用已知缺路由mock，无参数白名单扩张、时间精度放宽或失败证据覆盖。本机gh未登录后使用无凭据公开API元数据；最初错误路径/PowerShell只读诊断不用于门禁结论。独立L3入口自行处理共享历史tag差异，原始source-prepare保留。

本地资源回收：候选未安装node_modules、未开preview；保留当版bundle、WSL复核clone、原始证据与上一稳定恢复资料，不处理其它任务或已被平台限制的历史清理目录。当版无可回收前端产物，实际净释放0；其余边界见完成证据。

## 正式产物与生产完成证据

[Release v1.8.1](https://github.com/kejilion/KPanel/releases/tag/v1.8.1) 于2026-09-08T09:04:31Z公开，非draft/prerelease，8附件。[Release workflow34207150769](https://github.com/kejilion/KPanel/actions/runs/34207150769)及tag freshness34207150672全部success。公开说明包含具体优化、兼容升级、校验和回滚。原生镜像256MiB、只读根、非root及能力收紧运行契约通过；最终双架构SBOM/provenance启用，两个attestation manifest存在。

- 1.8.1/latest OCI index均为sha256:a2c48db4c63d7bbb0fecc5c9e212f21b678288b37030b26929f8039d1cccbd34。
- linux/amd64为sha256:db6d90123367acf808e945c8532c961ce44785815e6ffb76d877e61be684393c，linux/arm64为sha256:2e2cdce953c6899524acb79bdf1d293334f9820b54d1f87659e09308e2a33bed。
- 公开下载8附件并核对API size/digest及SHA256SUMS，本机副本再次独立hash核对通过。Agent amd64 501370a39ecd4e0a9bfa0196b4c7cfa7165d8cf24a9fca0842947121fe15c0f5，arm64 34728373de3b2efb349c05ff96d559258368db90ee646dd93c112fb1bf603e6b；node amd64 fe47d91606523c89f3f9bb531c46b23eeacb66c6f8a2de8004678fce75be173f，arm64 6448640defdd178da0ac10f4c3c5d373d002a9b91e4c70caddbd3fd7f7ae8fbb；deploy归档fad4609588768649d3bc0198f6dd4ef45a781b1824f002fb4ff774df306439a9。
- 公开immutable镜像重新pull，revision c2d47e7、version1.8.1、脚本双pin及User65532:65532一致；image_e2e=pass，09:05:31Z完成。C:/GitHub/_release-artifacts/v1.8.1-public-c2d47e7-r1及远端同名证据保留，没有用私有L3镜像充当公开验收。
- 首次生产写09:06:11Z。backup09:06:11Z至09:06:21Z passed；停写归档、6文件校验、旧镜像实际load和旧服务恢复通过。新备份/root/kpanel-backups/pre-v1.8.1-20260908T090611Z；数据归档SHA256 022e4d4de68aa83b89531932c66c837d4e92a756a40573b19821feb4e471a62c，旧镜像cb5f869eed3e071d8aea0caedb0ab1f8ef3f3b74a023574f4a6b68494386d33a。
- 标准更新入口exit0，拉取latest a2c48db4，恢复原访问策略；apps两次Already up to date。生产/root/apps clean，HEAD2d8044adec98e3eb16f47cdbb297f6be9632a66f，kpanel.conf SHA256 7b5b52af0ff20cff4bebf114e747ddf1c82996500f2767ba8d3733217e83121c。停写恢复的短暂连接reset、TERM与daemon-reload过渡提示由标准入口正常收敛，没有额外部署重试。
- postdeploy09:07:09Z至09:07:11Z passed/exit0，1.8.1/ok/initialized，revision c2d47e7、公开OCI a2c48db4一致；Panel running/healthy、restart0、OOMfalse；Agent loaded/active/running/enabled、NeedDaemonReload=no。protected.diff为空，panel/ai.db quick_check=ok；顶层ai.db为空如实记录；近10分钟无fatal/panic/OOM日志。单点CPU0.02%、RSS74.86MiB/256MiB、PIDs7，不作为长期资源趋势。
- preflight/backup/postdeploy顶层清单5/8/8项均独立核对通过。本地C:/GitHub/_release-artifacts/v181-production-{preflight,backup,postdeploy}/remote-evidence，远端/root/kpanel-release-evidence/v1.8.1-production/production-{preflight,backup,postdeploy}。原始health时间纳秒保留，机器字段使用UTC秒。
- [arena-154公网入口](https://kpanel.154.36.153.9.sslip.io) 的DNS为154.36.153.9，与SSH环境一致；现有Nginx反代127.0.0.1:8080，09:09:24Z实际HTTPS health为1.8.1/ok。旧会话地址kpanel.kejilion.eu.org的DNS为158.179.20.115，09:08:19Z只读响应1.8.0，属于另一个实例；未对它执行SSH、配置或升级，也未将其作为arena验收。没有连接108。
- 本次未回滚。GitHub Latest、Docker latest、标准更新入口及arena正式实例指向健康1.8.1，公共默认通道回滚决策不适用。Release自动删除已包含于tag的远端候选分支，未强删或改写历史tag。

源码没有后续修复，L3、候选CI、主线CI及Release均首轮通过。流程预检调用异常1次，已在生产写前修正；生产写后必需步骤失败0次。不把非终态Actions日志404、初期只读路径/PowerShell诊断当成产品失败或有效门禁证据。没有生产故障注入、Telegram发送、真实用户数据写操作或小时级soak声明。

当版候选未启动preview、未安装node_modules，没有需要停止的本地服务。保留源码、当前/上一稳定bundle、原始证据、新备份及WSL复核clone；本轮未触碰其它任务资源，实际净释放0。未新增永久规范、工作流或记忆；验收记录为仓库发布规范要求的独立文档提交。
