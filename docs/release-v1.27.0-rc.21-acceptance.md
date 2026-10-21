# KPanel v1.27.0-rc.21 发布验收记录

日期：2026-10-21。发布级别：L3。releaseChannel=preview，releaseTrain=1.27.0。

候选：057284aaf481572c4fd98e027caf0bfa2f93948a；tree=023b1dee7f60f274a9b8ed332798d910655cc6b7。上一预览及回滚点：v1.27.0-rc.20 / 11a9ca456a64a9439e06b07d3f6358a171c9257f / sha256:866acb4fd993255d3ae53b7363b48ad7552259d69424e58e27e8dccf5249bf35。稳定版仍为v1.26.0。

## 发布范围与责任

用户明确授权本次候选推送、主线、标签、Release和镜像发布；不包含生产部署或稳定提升。管理树只读；唯一列车release/v1.27.0-candidate保留。

- 纳入candidate/gallery-experience-optimization，精确tip bb62b2d818713a5e3a2f0f5a776d6bbf2fbeb1df，4个来源提交、17个文件，基线RC20；源码clean。
- 手机点缩略图直接看大图，选择按钮启用选择；两屏且最多1600px屏外预加载；快滑暂停新任务入场，减速分批补充；上滑退出大图；近期节点与查看器URL缓存复用。
- 原作者17条零散图库分支此前已归档，不重复计入本轮新候选。来源工作树和作者4175预览未移交，保留为本地待处置，不擅自删除或停止。
- 明确排除fix/systemd-late-mount-units@3f05f96037fd832b3f4d73b098b05cfc16301852。用户再次确认暂不汇入；普通服务器旧版升级、重启、回退闭环仍待实机验证，且发布辅助测试须独立治理。kpanel.conf及app-conf-lifecycle.sh与RC20逐字相同。
- 必需工具链安全修复在独立fix/v127-rc21-go-security：9d85c91d源码、dc492664a3b04f4cb481d66979b717eb1969da53最终候选；非作者只读复核和同SHA Linux CI通过后，以expected-SHA lease从11a9ca45快进主线。057284aa随后组合图库与已批准基座。

## 业务、资源、兼容与迁移

复用现有文件API、resourceVersion、权限和路径边界。无新后端接口、端口、常驻服务、宿主配置或数据库迁移。页面与查看器各自拥有滚动策略生命周期；硬后台/资源变化/卸载取消任务，快滑保留在途原生请求。图片分批4个、节点分批16个，近期节点基准240；必要视口范围可超过该基准，不是全页硬内存上限。查看器screen缓存3项、6MiB压缩/24MiB估算解码、60秒；原图缓存和既有配额不扩大。

快滑超出缓冲可短暂留白；浏览器控制解码与OS回收，不承诺永不重新解码。缩放、双指保护和横向翻页仍在；上滑只在未放大触摸手势生效。

scriptLinkageState=not-required，**无需发布脚本（不适用）**。内置脚本基线commit=160924a498d9b8d071fed72f165d23c1b9e886de，SHA256=39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1，本地Git blob独立回算一致。跨仓变更集及脚本候选不适用；脚本协议、安装路径、外部配置和镜像内置脚本均未变化。

## 安全修复与审阅

首次候选2827b275的CI39538067972由正常Go漏洞门禁拦截GO-2026-6809；原日志保留，不把实际载荷缺陷记作基础设施故障。[官方漏洞记录](https://vuln.go.dev/ID/GO-2026-6809.json)要求crypto/x509证书拒信补丁Go1.27.3，可达路径为Passkey证书验证。采用官方稳定补丁，无提速声明。

Go1.27.2→1.27.3；官方golang alpine index=f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd、bookworm=5c3e9b43cfd3bb95c7937462e29d87ada57b481a96fa23ad0eb61778d7a00110，实时Docker Hub与固定来源核对，两架构可用。工具链/runner/CI/fixtures与检查器同步，无门禁放宽。

- 图库自由臂在OCR圈选前保存；约束臂21/21 reviewed、2项文档/lock另核；无confirmed，constrained-only=0。来源代码17项blob精确复用。Go补丁另行自由臂及OCR15/15，无confirmed；合并仅扫描身份metadata冲突，保留新closure/build hash和RC21版本源码hash。
- 首次review trailer使用非标准provider、第二次分段导致Git未识别，后续2827b275修正为连续标准trailer；不改写已推送SHA。两次只改变提交消息，源码tree不变，原记录保留。
- 其他提供商CLI不可用。用户明确授权只读独立代理；非作者审dc492664全部16文件，独立访问官方数据，重建12组Go list原始流，核实全部源/manifest/build hash，结论PASS WITH FOLLOW-UP、无确认缺陷。跟进项为精确CI/L3/公开验收，由发布者完成；不把作者自查冒充独立复核。
- 相对RC20及批准工具链主线的security-review-scope均无敏感包组；安全盲审补漏not-run，不声称全仓安全通过，不执行CF审计。历史缺口与37项既有finding处置不变。
- Go补丁50项定向测试通过，规范一致性及canonical六平台actual/original inventory通过；仅标准库版本标签变化，恢复go1.27.2标签可精确恢复原closure，生产GoFiles和110模块集合不变。finding/observedAt/2026-10-25 expiry未变。
- 最终999个第一方源hash=ffbd8b1ae106298d6b3a38ae87c1a16a8161754fac9cecdb7af0b63a05f582df；Go1.27.3 closure=e802837924303d4a34fe1ad937425aa7767843617fca5ed5f70f7d28469788e2。图版本变化只增加Go版本字符串，L3必须重新核验。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 图库定向回归、完整源码与现有文件接口契约；触摸模拟和当前浏览器预览，实体手机未验 |
| 网络入侵与供应链安全 | 已验证 | 固定官方补丁/摘要、CI/L3扫描、保留37项既有处置；新盲审补漏not-run，不作全仓安全保证 |
| 稳定性、失败恢复与兼容 | 已验证 | source/version/后台/卸载取消与资源复用测试；宿主重启、真实systemd升级与混合旧节点未验 |
| 性能与资源预算 | 已实现未实机验证 | 有界调度/缓存/节点回归，未实测GPU帧率与峰值内存，无量化提速结论 |
| 用户体验与可访问性 | 已验证 | 显式选择、触摸上滑保护、键盘退出与焦点、深浅主题；200%缩放/Safari未实测 |
| 数据、配置与迁移 | 不适用 | 无数据库或安装器迁移；人工回滚仍使用配套备份，退出预览不自动降级 |

## 验证与证据层级

- 补丁push CI39539122804、最终候选push CI39539223376均success，head精确对应。完整源码、Go/vet、前端构建、race、Go原始/实际漏洞扫描、Node审计、Trivy及应用生命周期均通过。
- 最终前端原日志：313文件，3077 passed、6 skipped，合计3083；纠正来源交付把3083总数表述为全部通过的口径。
- 原图库Chromium触摸模拟验证快滑/减速、近期94节点保留、在途请求不重启、后台取消、失败图片入口、上滑关闭与焦点恢复。源码blob一致，因此复用；不是实体手机/生产Panel证据。
- 最终057284aa的acceptance mock预览绑定clean源；当前浏览器补验1280宽、深浅主题、点图、前后翻页、Left/Escape焦点恢复、显式选择。截图与manifest在仓外，所有本任务预览已停止；原作者服务未动。200%缩放和实体手机/Safari、真实大图库/弱网、GPU帧率与峰值内存未实测。
- 完整发布root：[39540735509](https://github.com/kejilion/KPanel/actions/runs/39540735509)，controller=dc492664；L3 passed，receipt candidate/tree/identityUnchanged=true匹配；GitHub ubuntu24 image20261018.339.1，Go1.27.3、Node24.21.0、Docker28.0.4。web/go/deploy分组均exit0，三个日志sha256独立回算一致。入口run-release-gate.sh摘要a2406ede4695ad76b509bad1389e543b54e6d1e9b1af17f5f6111b6e582ef6f7匹配；artifact12319619594 ZIP=ead4ccdb297454e8ce5a293efda7e82b202b9b9e30029967afdd5390ad18b57e独立核对。canonical原始inventory的source/graph/closure均匹配冻结范围。
- 公开验收146项全部通过：真实容器initial44/restart26、公开浏览器45、RC20更新说明浏览器31；四份报告mocked=false/errors=[]，覆盖PTY输入输出/刷新恢复/历史关闭、桌面恢复、证书故障回退、预览加入/退出及较低稳定版不自动降级。53个验收文件、10张PNG；artifact12320733710 ZIP=5ede4d2fc72e010afecedc9baad602e762f019383e9bcf2e1602f9faca8eea79，下载后摘要核对。固定测试Runner实际Id=sha256:fabd43717524cb6b85efcdf2ac56632131840619b852c1051877a8f7fb5b30e1；recipe/controller匹配，recipe原始/实际均d82de7272da2795ef4793a600fb35bcef7b75c948b0d24dd2298581a28f14055，diff为空、productImageUnchanged=true。原发布root各必需阶段全部success；publish/l3为stable专属路径正常skipped，预览的完整L3已在root单独执行。

## 发布产物与通道回读

[公开预览版v1.27.0-rc.21](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.21)，2026-10-21T04:42:22Z公开，draft=false/prerelease=true。annotated tag对象9eadd68b97b7b46812287cd46741c5534e353ec6，peel/main/candidate均057284aaf481572c4fd98e027caf0bfa2f93948a。

- OCI index及preview均sha256:b45fd6349fdb38c54f7c55015c0317d43e341231707c4b3739cb4e9c33e57d28。
- linux/amd64=sha256:c1ccb6bb558d60bd3ee8ece640be54fb2d471b4229b2103139fcf4a450ae5a58；linux/arm64=sha256:0d3646d98640cbee37d139f8452d8866192e4f66288cdd8a06eeb5ee6291b347；源码/version/script标签一致。
- 15附件经正式门禁下载/API摘要/磁盘校验通过；发布者另行从公开URL下载全部15附件，逐项size/SHA256与API及报告核对，并独立重算SHA256SUMS的11项。说明4项摘要、3项升级提示；发布信息接口和更新弹窗实际显示匹配。
- GitHub Latest仍v1.26.0；Docker latest仍sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9，独立前后回读一致。
- 原始snapshot artifact12321202136 ZIP=984fae7e14ac80ba4af62522fa2a75840c50811ba906211932fc81e9bd0dfb9b；stable-before.json原始hash=cf789ae33dcce40c425bac6d4cc597535daee2e5ffc1413c42efd40060a8bfd7，publication-identity严格绑定candidate/version/controller/run/attempt。previousPreview为RC20及866acb4f镜像，公开验收中的快照原始字节相同。
- publication artifact12321666344 ZIP=cedd3875ac212df073a23c9f2903ab2c27e1fe9ebb03ca65177b1fe9ecfd10f6。根目录C:/GitHub/_release-evidence/v1.27.0-rc.21保存首次失败、成功CI、全部artifact与摘要、公开附件、registry/API原件和独立复核。

生产部署：不适用（preview禁止生产部署），未连接任何生产主机。没有真实宿主systemd更新/重启/恢复证据；公开隔离容器夹具不能替代该验收。

## 回滚与候选收尾

人工回滚使用RC20不可变镜像及配套数据备份。退出preview不自动降级，本轮无数据迁移；RC20的批量记录备份/旧版读取限制继续适用。唯一release/v1.27.0-candidate列车与源码worktree保留。自有fix/v127-rc21-go-security@dc492664已完成独立非作者复核、同SHA候选与主线CI，在远端原子保存archive/fix/v127-rc21-go-security并用expected-SHA lease移除原引用，归档精确回读一致；本地同名归档、clean工作树保留。图库来源bb62b2d8已纳入RC21，不再计入下一轮待发布，所有权未明确移交故保留本地待处置；systemd3f05f960明确未纳入，继续待验收。责任为当前发布者，下次来源明确释放或实机闭环完成时复核。验收文档使用独立docs/release-v127-rc21-acceptance，不并入冻结main、不创建PR；本任务所有临时预览已停止，其他任务服务未动。C盘新建前约58GB可用，结束数值记录于publication-state.json；固定Go工具和证据保留供重现，不清理未知缓存或目录。复用既有发布/版本管理/OCR/预览流程，仅更新既有工具链pin，未新增工作流。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-21T09:41:43+08:00
- 候选冻结时间：2026-10-21T11:33:03+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：6
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-review/git-trailers/nonconforming-block",
    "position": "before-production-write",
    "count": 2,
    "impact": "先后使用不合规provider及空行分隔trailer，独立review advisory未通过。",
    "recoveryEvidence": "保留9bb2d388、d18dc1dc；2827b275连续标准trailer检查recorded，三者产品tree相同。",
    "permanentAction": "本轮后续使用精确消息文件和连续trailer块，提交后读取Git实际trailer，不以消息肉眼存在判定成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/git-show/default-output-buffer",
    "position": "before-production-write",
    "count": 1,
    "impact": "脚本blob超过Node默认1MiB输出限制，首轮读取失败。",
    "recoveryEvidence": "明确16MiB上限重新只读读取，SHA256与内置pin完全一致。",
    "permanentAction": "大体积公开blob读取设置有限maxBuffer，不把截断结果用于hash或兼容结论。",
    "historicalReleases": []
  },
  {
    "fingerprint": "toolchain-qualification/focused-tests/identity-not-final",
    "position": "before-production-write",
    "count": 1,
    "impact": "闭包观察尚未结束时提前执行定向检查，旧build hash使1项失败。",
    "recoveryEvidence": "保留before-identity日志；等待实际身份落盘后最终50/50通过，canonical原件通过。",
    "permanentAction": "身份生成与依赖该身份的测试顺序执行，后续CI基于完整提交，不覆盖首次失败日志。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/artifact-reader/assumed-binary-inclusion",
    "position": "before-production-write",
    "count": 1,
    "impact": "独立复核初稿误将仅含报告与摘要的验收包当成含全部二进制，读取不存在文件失败；未产生成功结论。",
    "recoveryEvidence": "从公开URL另行下载全部15附件；size/API SHA256/磁盘SHA256及11项SHA256SUMS独立通过，public-independent-summary.json保存原件核对。",
    "permanentAction": "先核对artifact文件清单，再按实际清单读取；报告包与公开二进制下载分开验证，不以不存在文件或摘要文本冒充本地文件核验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/powershell/stale-native-exit-code",
    "position": "before-production-write",
    "count": 1,
    "impact": "附件并行下载和逐项摘要已经成功，但末尾错误使用LASTEXITCODE检查PowerShell cmdlet，误报下载失败。",
    "recoveryEvidence": "15项下载摘要记录存在；独立Node重新读取全部真实文件/API并核对，最终146项报告、15附件和11项校验和通过。",
    "permanentAction": "PowerShell cmdlet以ErrorAction Stop及真实结果断言为准；不把遗留原生命令退出码用于cmdlet成功判定，不重发或覆盖已公开版本。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->
