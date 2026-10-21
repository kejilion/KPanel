# KPanel v1.27.0-rc.22 发布验收记录

日期：2026-10-21。发布级别：L3。releaseChannel=preview，releaseTrain=1.27.0。

候选 f0c039d8d3604f4fbafd4745efbc5a3cf4a3df96，tree=ce8eca4f1296bb599d73d4d521256229611d2caf。上一预览及人工回滚点为 v1.27.0-rc.21 / 057284aaf481572c4fd98e027caf0bfa2f93948a / sha256:b45fd6349fdb38c54f7c55015c0317d43e341231707c4b3739cb4e9c33e57d28。稳定版本仍为 v1.26.0。

## 范围与责任

用户明确要求“汇入新优化 systemd 安装器 和其他新候选分支 发布新预览版”，并继续推进；授权本轮候选推送、main、tag、Release、镜像。预览禁止生产部署，不提升 stable。

- systemd 来源 fix/systemd-root-mount-scope@590324e0f7935e0a4cfaf3e87c9e7e1c31be114f，8个来源提交、2文件。替代旧 fix/systemd-late-mount-units@3f05f96037fd832b3f4d73b098b05cfc16301852，不重复纳入。
- 终端来源 fix/terminal-batch-stop-cleanup@e87b803e24f94e3158dd60f3216569db1df8e050，3个来源提交、10文件；包含 fix/terminal-batch-scroll@b513ee520c1e624085509ff7afab3feacbd0167b，不重复纳入。
- 安装器涉及 packaging/tests 发布辅助，先建立独立 fix/v127-rc22-systemd-integration@bc0fc037153c5d7f5796ac3aba4d69069a44de78；来源独立审查与发布者非作者复核、同SHA Linux CI39548328761通过后，从057284aa以expected-SHA lease快进main。候选受信helper tree与批准main一致，仅排除发布身份metadata。
- 管理树只读；唯一 release/v1.27.0-candidate 列车保留。原作者工作树和服务所有权未移交，保留待处置，不擅自修改、清理或召回任务。

## 用户行为与兼容边界

root mount 用户保留现有 systemd unit/wants 链接和inode；路径位于独立挂载卷时，将服务定义复制到 /etc/systemd/system，并使用真实路径 RequiresMountsFor。以 /proc/self/mountinfo 最长组件前缀识别，symlink先规范化，同设备bind/subvolume也可识别。仅安装、更新、自动更新、恢复时判断；普通start/stop不增加常驻轮询。

挂载状态未知时保留已有表示；已有copy保持copy。旧镜像capability negative marker优先，旧镜像fresh安装与失败回退继续使用旧link布局。首行托管标记、root所有权和权限约束阻止覆盖foreign unit；临时文件+原子替换，仅重定向指向自有源的wants。GNU/BusyBox awk转义回归修复包含在来源内。

批量终端执行时收起输入，保留停止按钮；右侧命令与结果整体滚动，快捷/历史栏位置独立。清理默认30秒、单次close10秒；未确认close标为failed/terminal_close_failed，保留session identity、quota和输出；停止不意味着远端进程已经被确认杀死。重试释放保持本轮结束状态，不恢复running或重放命令；m.active阻止并发新任务。

旧结果OS图标取完整hostInventory，不随下一轮勾选变化。提交新任务先清空当前展示与计数/滚动，再过渡显示新结果；提交未确认时恢复旧结果。保留RC20的24小时记录、2MiB metadata quota、仅最近10轮内存输出。两处翻译冲突保留RC21“操作记录已保留，原终端输出已释放。”，增加“重试释放会话”。

普通实体服务器升级、重启、回退完整闭环，以及跨服务器启动性能基准未实测。隔离mount/systemd分析、生命周期与公开容器夹具不能替代此证据；不作普遍启动提速或远程进程终止保证。

## 跨仓库联动与供应链

scriptLinkageState=not-required，**无需发布脚本（不适用）**。内置脚本基线160924a498d9b8d071fed72f165d23c1b9e886de，SHA256=39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1。复核resolve_app_config与kpanel_app_source_config：现有引擎定位可信配置并source隔离snapshot，不解析systemd链接表示；新版配置复用现有生命周期，无新engine动作、callback、协议或字段。跨仓变更集、脚本候选不适用。

保持RC21 Go1.27.3安全基座、依赖图、目标和既有finding/expiry；本轮无生产GoFiles新增，新增cleanup_test仅测试。第一方Go源码1000文件，hash=51a5796bab5f57c59623d8b40015ccebc9dc3a9e05f80a4f08b0d49afea57059；closure=e802837924303d4a34fe1ad937425aa7767843617fca5ed5f70f7d28469788e2。

## 审阅与本地验证

- 发布者独立检查来源diff、生命周期caller、mount匹配、root链接保护、foreign unit拒绝与旧镜像回退；原非作者审发现GNU awk转义问题，修复后GNU/BusyBox实测记录保留。无新增确认finding。
- 自由臂在OCR约束臂前完成；组合OCR15/15 reviewed，3项conf/docs/lock另核；约束新增发现0。conf因扩展名不受支持已手工完整审阅，不伪称OCR自动覆盖。
- 相对RC21及批准main的security-review-scope无敏感包组；本轮CF安全审计及全仓盲审补漏not-run，不作全仓安全通过结论。历史处置不变。
- 来源Linux L2、实际隔离mount、OpenRC、systemd-analyze和安装更新回退全生命周期原件可复核；来源使用Go1.27.2，本轮精确Go1.27.3 CI补证。
- 最终候选push CI39548609154 success；本地Go1.27.3 linux/amd64 terminalbatch race pass，源码精确f0c039d8、clean；本地Vue两文件18项全部通过。
- Mock UI绑定clean f0c039d8：13/14释放卡住后stop使14/14结束，retry不复活、14份输出保留；下一轮13选择不改变14份旧输出/OS图标；提交中旧列表清空且scrollTop0，503后完整恢复；下一次成功只显示13台并完成13/13。
- 结果滚动1421px时命令top=-1261，历史侧栏top/left不变；390px窄屏panel344/content344、page380/viewport380；635px紧凑panel585/content585、page625/viewport625。深浅主题、Ctrl+A选择14台、控制台无error；输出13px、辅助计数最低12px。临时viewport已reset。
- 实际浏览器200%缩放未验证；紧凑窗口不冒充200%缩放。模拟API只证明展示交互，不是实际Panel/宿主终止证据。初次浏览器输入失败原件保留，fresh tab与force语义定位成功后才记录核心旅程通过。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向Go/Vue、生命周期和本轮核心模拟UI；真实远端终止仍受close确认语义限制 |
| 网络入侵与供应链安全 | 已验证 | 同SHA CI与固定Go1.27.3基座，保留既有finding处置；未执行全仓安全盲审 |
| 稳定性、失败恢复与兼容 | 已验证 | 独立cleanup deadline、retry不重放、root inode保护、旧镜像回退；实体服务器升级重启未实测 |
| 性能与资源预算 | 已实现未实机验证 | 有界清理/现有quota、无常驻轮询；无跨服务器启动性能基准 |
| 用户体验与可访问性 | 已验证 | 核心交互、键盘多选、窄屏/紧凑/深浅主题；实际200%缩放未验 |
| 数据、配置与迁移 | 已验证 | metadata历史规则保留；systemd有条件copy和旧版本回退测试，无数据库迁移 |

## 完整发布与公开产物

完整发布root：[39550852186](https://github.com/kejilion/KPanel/actions/runs/39550852186)，controller=bc0fc037153c5d7f5796ac3aba4d69069a44de78。候选CI与L3均success，main/tag已提升到冻结候选；annotated tag对象1727cd584d9b46657e8920756366c9ec4f70eeac。

L3 GitHub ubuntu24 image20261012.334.1，Go1.27.3、Node24.21.0、Docker28.0.4。receipt candidate/tree/identityUnchanged=true匹配；web/go/deploy均exit0，三份原始日志摘要独立回算一致。前端313文件，3082 passed/6 skipped/3088 total。入口摘要a2406ede4695ad76b509bad1389e543b54e6d1e9b1af17f5f6111b6e582ef6f7匹配。artifact12324625474 ZIP=cc70748255dc1ffed657291bfcce81ca4b34f96718008fc73edd9630412cf7f2；qualified-scan-inventory原始流bytes/hash独立核对，source/closure严格匹配冻结身份。

主线CI、Release、公开验收和preview-completion全部success；root结论success。stable专属publish/l3正常skipped，预览完整L3已独立通过。

[公开预览v1.27.0-rc.22](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.22)，公开时间2026-10-21T06:28:15Z，draft=false/prerelease=true；主线、候选与tag peel均f0c039d8d3604f4fbafd4745efbc5a3cf4a3df96。

- OCI版本镜像1.27.0-rc.22与preview均sha256:0dfc446bca4d8e20057d031f927ea8edf54df67c9e68b3643973f2d4b3b3301f；linux/amd64=sha256:ab59ac94ff11552cafd44c1f90de78f10fc52362ee106337098d3a7081e40a5c；linux/arm64=sha256:501aa9bbc3bee40bf6984e6404f25745619437a6644df1cff4680c1334f27c7e。两架构source/version/script标签经公开门禁验证。
- 146项公开验收：published browser45、RC21升级browser31、真实隔离容器initial44/restart26，四份report status=passed/mock=false/errors=[]，共53文件/10PNG。覆盖PTY、桌面恢复、证书故障回退和预览切换；不把容器夹具冒充实体服务器systemd升级重启。
- 全部15附件由正式门禁与发布者独立下载，API size/digest、真实磁盘SHA256逐项一致；SHA256SUMS的11项重新计算通过。说明4项摘要、3项升级提示。public artifact12324969358 ZIP=bfe740369b15b747fe23bf14818003be265935c9341759889cd5d07afec075e5。
- 测试Runner实际Id=sha256:b2d4d0bbc50161220b6cbb369bbfd39e7314cdb79a0724f31a30acedccce32eb；controller=bc0fc037，原始/实际recipe SHA256=d82de7272da2795ef4793a600fb35bcef7b75c948b0d24dd2298581a28f14055，diff为空、productImageUnchanged=true。Runner实际ID不假定与RC21相同。
- 原始snapshot artifact12325316026 ZIP=8bcf12a48927ce9e5898585b28efc65953e2478e18920abf7456b74e2d84452f；stable-before原始hash=da369c54f5f3d68b56de3ab4d26da58d66e459814691cf63d103eccb8abbaf50，candidate/version/controller/run/attempt/phase全部核对。previousPreview明确为RC21/b45fd634；公开验收快照原始字节与该原件一致。
- publication artifact12324809770 ZIP=a2a68799315e2b8a96e21f5e65a3069aa6912459b4a22a7081ee33884c199b84。所有原件保存在C:/GitHub/_release-evidence/v1.27.0-rc.22，独立核对summary一并保留。
- GitHub Latest仍v1.26.0，Docker latest仍sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9，发布前后独立回读一致。

生产部署：不适用，未连接任何生产主机。

## 回滚与收尾

人工回滚到RC21不可变镜像，并使用配套数据备份；退出preview不自动降级。停止清理失败时保留占用session identity，不应误认为远端命令已确认终止。唯一RC列车保留；来源分支已纳入/被替代，不再作为全新候选，但所有权未明确释放时保留工作树和分支待处置。发布者下次在来源释放或实体服务器完整闭环后复核。

自有治理分支已完成同SHA候选CI39548328761、main快进与主线CI39549984402；远端原子归档archive/fix/v127-rc22-systemd-integration@bc0fc037153c5d7f5796ac3aba4d69069a44de78并以expected-SHA lease移除活跃ref，远端读回一致；本地同名归档和clean工作树保留。来源590324/e87b803/b513ee5及被替代3f05f960所有权未明确转交，保留待处置，不作为重复新候选。本验收使用独立docs/release-v127-rc22-acceptance，冻结产品main不追加文档、不创建PR。本任务4177预览与63638 mock均已停止、监听核对无残留；原作者服务未动。结束C盘可用61,344,088,064字节；自有固定Go工具、隔离WSL候选及证据保留供重现，未清理未知目录。

复用现有版本管理、发布、OCR、预览工作流，未新增工作流或知识文档；本验收记录是项目发布强制交付。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-21T10:22:17+08:00
- 候选冻结时间：2026-10-21T13:16:11+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "pre-dispatch/check-release-refs/powershell-array-notmatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "local assertion stopped before dispatch; no remote drift",
    "recoveryEvidence": "join ls-remote output into scalar before regex comparison",
    "permanentAction": "复用已核对的精确路径、参数和候选身份；保留首轮失败，不覆盖既有发布或成功原件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-race/git-clone/windows-worktree-gitdir",
    "position": "before-production-write",
    "count": 1,
    "impact": "First clone fails because Windows worktree gitdir cannot resolve in Linux; original race-go1273.log retained",
    "recoveryEvidence": "Clone read-only management repository into unique own WSL directory, checkout exact candidate; Go1.27.3 terminalbatch race exits0",
    "permanentAction": "复用已核对的精确路径、参数和候选身份；保留首轮失败，不覆盖既有发布或成功原件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-ui/cua/input-no-visible-effect",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial AX/locator actions produced no mode change; original browser-status.json retained",
    "recoveryEvidence": "Fresh candidate tab and force semantic locator click succeeded; six core journeys and narrow/compact layouts verified",
    "permanentAction": "复用已核对的精确路径、参数和候选身份；保留首轮失败，不覆盖既有发布或成功原件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/dockerhub/version-tag-prefix",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次Docker Hub版本回读误加v前缀，HTTP404；未据此认定发布失败或重发。",
    "recoveryEvidence": "读取实际release.yml tags配置后改为1.27.0-rc.22，版本/preview/两架构摘要匹配，latest与GitHubLatest不变；原读脚本保留。",
    "permanentAction": "从冻结发布配置读取镜像tag语法，不从Git tag推断。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/powershell/unquoted-git-revision",
    "position": "before-production-write",
    "count": 1,
    "impact": "tree revision未引用导致PowerShell误解花括号，原读取失败，无成功身份结论。",
    "recoveryEvidence": "使用单引号保护f0c039d8^{tree}，得到ce8eca4f1296bb599d73d4d521256229611d2caf并与L3匹配。",
    "permanentAction": "Shell中的Git revision包含花括号时使用字面量引用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/git/default-working-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次远端ref回读在C:/GitHub执行，找不到origin。",
    "recoveryEvidence": "切到精确候选工作树后只读回读，main/candidate/tag peel均f0c039d8，未进行修复性远端写入。",
    "permanentAction": "Git证据命令显式指定工作目录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/artifact-reader/assumed-file-location",
    "position": "before-production-write",
    "count": 2,
    "impact": "准备独立核对器时，对旧L3目录名及publication文件名的预判读取失败。",
    "recoveryEvidence": "先列实际artifact文件清单，再读取真实receipt/result与publication identity；最终新轮所有zip/日志/附件摘要通过。",
    "permanentAction": "先消费精确文件清单，再读取，不将未找到文件视为成功证据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/report-release-metrics/incident-fingerprint-shape",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收记录首轮指标校验拒绝4个不满足三段slug格式的异常fingerprint；产品发布已成功，不改写发布产物。",
    "recoveryEvidence": "统一为stage/authoritative-entry/root-cause格式后重跑指标校验与L0，保留首轮失败摘要。",
    "permanentAction": "先对照现有指标schema生成记录，再执行文档门禁。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->
