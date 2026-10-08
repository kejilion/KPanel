# KPanel v1.25.0-rc.15 发布验收记录

日期：2026-10-08

发布级别：L3。**预览产物已发布，生产未部署；生产实例版本未验证。**

候选提交 / 标签：`5eeecd4bac16ae4fccec5039863e476ad2054004` / `v1.25.0-rc.15`；tree `02faf8fcad8d79172bfe24bc25fa4761cad295f1`；公开时间 `2026-10-08T15:27:33Z`。
releaseChannel=preview；releaseTrain=1.25.0。唯一候选 release/v1.25.0-candidate 保留在产品SHA。
上一稳定 / 回滚点：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3 / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。上一预览RC14产品289c5d51c60fe55535829bb4862c95b05c3b469d / `sha256:835f66c11d4bfd46676d2096587718320bdd9d8d3b6d1409d32ba7ca425b79dd`。

## 发布画像与范围

业务域为Linux集群轻节点和文件管理；变更仅涉及发行版身份识别与现有成功操作的展示定位。原认证、签名/重放、权限、存储格式、API动作、端口、Compose、Agent权限、kejilion.sh、应用市场契约不变；L3精确组合验证。
- 修复 OpenWrt、LEDE、ImmortalWrt、iStoreOS 轻量节点在面板重启或容器重建后从列表消失、无法继续上报的问题。保留原记录和密钥，符合现有 Linux 身份规则的节点可继续重试上报，无需重新配对。
- 文件和文件夹上传、远程下载、重命名、压缩解压、粘贴及拖拽操作完成后，选中并定位成功产生的项目；后台归档同样反馈结果。复用有界分页查找，使用服务端实际路径，切换主机或目录后忽略迟到结果。

升级提示：

- 软路由恢复需要节点进程仍在运行、网络可达、原记录和凭据保留。本次没有改动轻量节点协议、密钥或存储格式；未验证用户的两台实机，也未确定现场容器重建原因。
- 文件结果只定位当前主机和目录，最多继续读取 20 页；超出预算的项目不会触发无界加载。只定位成功结果，不自动打开或执行文件。
- 本版仅用于预览，Docker 批处理未纳入；继承的 CF 审计和 RC13 接收性能限制仍保留。生产未部署，退出预览不会自动降级。

用户明确“Docker 批处理 不汇入预览版”；`refs/heads/archive/feature/docker-container-batch-actions` / `dca6305b9d631aeea5b12eebffa6b6752b731d5a`未变且非产品祖先。旧已合并/patch-equivalent来源、备份残余旧测试和进行中的登录CF记录未纳入。

## 来源、修正与候选处置

批准main `b23d7992948018aec18b1e2aaebc538c1b5a2291`。两个原source tip保留祖先，no-ff组合无冲突，integration-map.json保存映射。1542de8修复多结果分页、异步原主机归属、实际上传路径并加强现有断言；528f3cfb补现有测试绑定optional nextOffset；5eeecd4b对scrollIntoView作optional方法调用。无新依赖/无关重构，路由测试文件来自源作者。

| 原分支 | 原始精确tip | 归档ref/同SHA | 远端与本地 |
| --- | --- | --- | --- |
| `fix/light-node-openwrt-recovery` | `ff3d6876fec25748b0a8e59aad6494b873bbc829` | `refs/heads/archive/fix/light-node-openwrt-recovery` | absent; original source was local only；clean detached，目录物理保留 |
| `claude/files-select-after-op` | `56dac61e4770c8da2a47225d23c4c686656da202` | `refs/heads/archive/claude/files-select-after-op` | absent; original source was local only；clean detached，目录物理保留 |

归档前复核作者结束、clean tip、所有权和远端引用。Claude end_turn 2026-10-08T13:30:01.552Z，Codex源completed/idle；只接管已提交快照，未召回作者。expected-SHA本地事务，local-only来源不制造远端archive，两个作者工作树clean detached物理保留。无reset/stash/强制切换。
独立docs/release-v1.25.0-rc.15-acceptance仅两个docs路径；同SHA候选/main CI成功后精确归档，管理main普通快进；结果另存docs-closeout-result.json/management-sync-result.json。Dependency freshness按实际路径和filters判定。产品tag/镜像/正文不重发。其他作者目录保留，无新增归档待办。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 17签名/重载场景，完整L3，11真实文件旅程 | 用户路由器、实体OpenWrt/iStoreOS未验证 |
| 网络入侵与供应链安全 | 已实现未实机验证 | Trivy/govulncheck/npm audit、固定供应链、同SHA CI | CF scoped-required，不代表完整审计 |
| 稳定性、失败恢复与兼容 | 已验证 | 旧索引字节保留、原密钥重载回归、迟到上传不改导航、L3安装更新契约 | 生产回滚/断电/soak未验证 |
| 性能与资源预算 | 已验证 | watchdog/OOM；Panel/Agent1CPU/256MiB/128PID，browser2CPU/1GiB/256PID | 无新perf结论；RC13退步/临时空间继承 |
| 用户体验与可访问性 | 已验证 | 分页定位、窄屏浅色/桌面深色/CSS200%、Escape、4项公开notes | 原生缩放/Safari/手机键盘/完整语言矩阵未验证 |
| 数据、配置与迁移 | 已验证 | 实际upload/rename/paste/归档/下载字节与旧索引hash | 无新schema；生产存量/NAS未验证 |

## 自动门禁、审计与供应商复核

唯一L3入口scripts/run-release-l3.mjs；arena-154 candidate-validation，production=false。固定Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go1.27.1/Node24.21.0/npm11.19.0/Linux amd64。
- `v1.25.0-rc.15-c3e39f28-l3-r1`：failed/2，`2026-10-08T13:49:20Z` 至 `2026-10-08T13:51:59Z`；`C:/GitHub/_release-evidence/v1.25.0-rc.15-c3e39f28-l3-r1/remote-evidence`
- `v1.25.0-rc.15-528f3cfb-l3-r2`：failed/2，`2026-10-08T13:56:44Z` 至 `2026-10-08T14:05:43Z`；`C:/GitHub/_release-evidence/v1.25.0-rc.15-528f3cfb-l3-r2/remote-evidence`
- `v1.25.0-rc.15-5eeecd4b-l3-r3`：passed/0，`2026-10-08T14:09:37Z` 至 `2026-10-08T14:25:19Z`；`C:/GitHub/_release-evidence/v1.25.0-rc.15-5eeecd4b-l3-r3/remote-evidence`
R1在TS2353测试类型失败，R2既有多窗口测试DOM缺滚动方法；失败原件各12文件hash回收，失败头未推送、未标通过，第三轮完整重跑。
` Test Files  264 passed (264)`；`      Tests  2428 passed | 6 skipped (2434)`。typecheck/i18n/生产build、全部Go单测/race/vet、安装安全/锁兼容/生命周期/更新备份契约通过；三lane web=587492ms / go=542854ms / deploy=1831ms。定向两个既有FilesView回归99项通过。
日志 `C:/GitHub/_release-evidence/v1.25.0-rc.15-5eeecd4b-l3-r3/remote-evidence/l3-verify-release.log` / SHA256 `e5798475f1b250edca99baa424cff43cc182c54ca1c99bc9bf13607580780e0c`；12权威文件。输入plan `4fb60e65af7436e5a143898c637a37ee8d98efe78ddf851e478e93145e6304cb` / script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979` / bundle `533e7b9dfb607601d07cc0822e2419f6d6f15ee2b7ab722f2dae562f717caadb`，本地远端一致。
DRAFT_NOTES-r2.md与公开正文canonical renderer一致；实际API2摘要/3升级提示。Trivy源码/最终镜像、SBOM/provenance按实际workflow通过；原始扫描输出保留，不扩大为完整漏洞审计。

- CI / `release/v1.25.0-candidate` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37796673216](https://github.com/kejilion/KPanel/actions/runs/37796673216)，success
- Dependency freshness / `release/v1.25.0-candidate` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37796673366](https://github.com/kejilion/KPanel/actions/runs/37796673366)，success
- CI / `main` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37798178881](https://github.com/kejilion/KPanel/actions/runs/37798178881)，success
- Dependency freshness / `main` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37798178842](https://github.com/kejilion/KPanel/actions/runs/37798178842)，success
- Release / `v1.25.0-rc.15` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37799344203](https://github.com/kejilion/KPanel/actions/runs/37799344203)，success
- Dependency freshness / `v1.25.0-rc.15` / `5eeecd4bac16ae4fccec5039863e476ad2054004`：[37799344187](https://github.com/kejilion/KPanel/actions/runs/37799344187)，success

OCR9/9 reviewable，H0/M2/L1已修正；blind=false、constrained-only=unreported。相同source blob继承覆盖，版本字面值/测试typing/scroll修正自审。Claude来源由Codex不同供应商复核；Codex来源为不同实现会话同供应商回退，claude/gemini CLI不可调用；owner局部修正不冒充独立复核。
CF精确目标 `5eeecd4bac16ae4fccec5039863e476ad2054004` / `scoped-required`，待审54提交/169文件；internal/atomicfile、internal/netpolicy新边界。最近full run4/4c0694aa8e02e46145a775707b8d5a0355f7ce10，incomplete/partial不计覆盖。本轮未执行scoped/full，PROJECT_RULES5.4预览非阻断记录；稳定版另需准入，gpt-6-luna/max规则保留。

## 跨仓库联动与依赖变化

scriptLinkageState=not-required；变更集/脚本候选不适用，脚本不在范围。内置script `c3a8bd895f8878d9e4ced7592c91a20c974472a5` / SHA256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`；apps `4fc985e964dbd1742143521d0a28ab652b885773` / `feat/lanqin-email` clean，kpanel.conf规范化相等SHA256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`。
本版无依赖/锁依赖项/Action/基础镜像/扫描器/受管脚本升级，lock只变版本字段。精确L3/Dependency freshness采用固定工具链通过；make dependency-report及本轮每日公告/EOL未单独执行，继承RC14既有决策，不以构建成功替代最新完整检测。Node26/TS7暂缓项未采用，按既有期限复核。
L3的Trivy0.74.0日志提示0.75.0可用，仅记检测信号，未确认候选维护/架构/兼容资格，也未在冻结头引入工具升级。负责人KPanel base maintenance，2026-10-14复核；保留0.74.0固定扫描与原始日志，退出条件为独立L2/L3完成兼容/安全/资源/回滚验证或有证据拒绝。正式完整检测起算后遵守minor启动14日/决策30日/处置60日上限，安全可达漏洞服从更严24/72小时；扫描器回滚点为本产品既有固定0.74.0。

## 隔离真机与浏览器验收

Mock acceptance/visual-composition local-preview-r4绑定`5eeecd4bac16ae4fccec5039863e476ad2054004`，仅布局/旅程约束。
`native-feature-browser-r8`：外层passed/0，420秒硬超时；内部receipt passed，240秒browser超时，原件全hash恢复。使用完整L3导出immutable image/index ID，version=RC15；本地镜像缺公开revision标签不冒充公开image证据。
11项：多文件跨页upload、rename、真实后台压缩/解压、copy/paste独立目标目录、公共HTTPS下载、390×844浅色、桌面CSS200%、Escape、迟到上传导航保持、字号。Chromium 140.0.7339.16 / Playwright 1.55.0 / Node v24.18.0，最小字号12px。
持久化核对upload/rename/copy字节、tar单成员、解压文件、迟到upload与不可变RC14 VERSION下载；两个旧索引hash原样。私有隔离目录，不自动执行结果。
`public-notes-browser-r1`：实际公开RC15 Panel/Agent API和实际RC14设置页/更新弹窗，1280×900深色、390×844浅/深色；2摘要/3提示逐条匹配、末条可滚动、Escape、字号≥13px。加入预览、自动安装disabled、安装请求0。
两个成功轮次page/console/HTTP错误0、OOM/resource failures0、own容器/网络/私有夹具清理通过。browser Runner sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29，Node/Playwright文件hash固定。只读Docker代理真实GET ping/info和强制唯一空应用label；Agent不挂宿主socket，写方法/其他路径上游前拒绝。
此前native r1失败为125夹具上传目标未实际触发分页，r3为README不在初始页，r5为browser1GiB OOM，r6多上传跨页通过但随后菜单不可见，r7四项真实操作通过后验收脚本误替换目录双击步骤而失败；各原件及cleanup/resource结果保留。r2/r4仅prepare后在启动前被协议/真实枚举顺序复核替代，未运行，不冒充执行结果。成功轮次采用150夹具与实际扫描顺序选取跨页目标；实际UI默认100，Agent最大500，早期误写已在当前结论更正；操作使用已有行菜单按钮和稳定几何、同1GiB预算，trace保留screenshots且snapshots=false。浏览器trace开销是原因分析，不由该OOM推定Panel/Agent或生产浏览器内存资格。
既有文件控制的最小字号12px，低于通用14px建议；本版没有改变字样样式，只记录现状，不能声称全字号规范达标。后续基座UX复核；不扩大当前源码范围。
五张真实viewport截图已逐张核对，native-visual-review.json保存结论；上传选中与数量、归档/下载完成、导航保持可见。窄屏与CSS200%沿用部分shell/toolbar裁切、结果可在viewport外，选中由实际DOM断言验证；未宣称完整布局或可访问性资格。
物理路由器/用户两节点、手机/Safari/原生125%/200%缩放、WAN/10GiB/10000项、arm64原生、完整systemd升级回滚、断电/soak未执行。

## 公开产物和自更新

[GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.15) draft=false/prerelease=true，Latest仍`v1.24.0`。版本/preview OCI index `sha256:90713c54f2eda99b1cb47601846f51dc08b5c022ee4675e1f5f61beaa9ca99ae`；amd64 `sha256:6145e9ed2b97b1a21be4b4fa13a7f058528b4769bbfd1aa0014e116614420946`；arm64 `sha256:0f8ada6aa69e68b805b45b19baef009fffab25b4128a9a1f44d3517cdb66472b`。两架构revision=`5eeecd4bac16ae4fccec5039863e476ad2054004`、version=`1.25.0-rc.15`、script pin一致。
14附件/11 sums/2 attestation entries；metadata/license实际字节核对通过，未下载所有binary字节。公开immutable `docker.io/kjlion/kejilion-panel@sha256:90713c54f2eda99b1cb47601846f51dc08b5c022ee4675e1f5f61beaa9ca99ae` / image_e2e=pass，own fixture前后inventory一致。
Release canonical正文hash一致，实际notes API/RC14弹窗匹配公开digest，2/3内容非空。稳定Latest/latest `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`不变；加入预览只切来源立即检查，canInstall=true，自动安装disabled，未点安装。退出较高RC不降级、旧状态stable等由既有L3源码回归覆盖，完整生产升级/回滚未执行；OpenRC/轻Node沿用release-channels。

## 生产部署安全核对与回滚

生产目标/授权/部署前后/备份/命令/写操作全部不适用（预览版禁止生产部署）。仅arena-154隔离candidate/browser验证；prod-108/108未连接、备份、部署、升级或核对。生产实例版本健康未验证。
源码/tag/镜像回滚点为上述v1.24.0及RC14；未采集生产备份、未执行生产回滚。未来回滚需数据/配置备份和实际资源健康复核。GitHub Latest、Docker latest、标准默认入口仍v1.24.0；公共默认决策不适用，不提升稳定。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-08T20:59:13+08:00
- 候选冻结时间：2026-10-08T14:08:02.434Z
- 生产完成时间：未验证
- 提交到生产用时：未验证
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

候选验证失败记录：未发布候选的两轮完整L3失败；发现时间2026-10-08T13:51:59Z，恢复时间2026-10-08T14:25:19Z。未逃逸：完整L3在首次推送/tag/生产写入前拦截，失败原件保留，精确新头第三轮完整重跑通过。生产回滚、紧急热修复或重复发布未发生；生产恢复指标不适用。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：28
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 9,
    "impact": "rg included nonexistent .codex; recovered with literal .codex-workflows discovery; no mutation or validation invalidated Additional event: Read-only externalFileDrop.ts guess rejected; rg --files located desktopExternalDrop.ts, actual root-entry contract then read Additional event: Read-only light_auth.go/light_wire.go and L3 plan.json guesses rejected; actual domain files and plan.env discovered. No source mutation. Additional event: Read-only guessed zh-CN.ts path did not exist; actual flat messages source inspected. Additional event: Read-only wrong security coverage script filename rejected; canonical check-security-audit-coverage.mjs discovered and exact-SHA assessment passed. Additional event: Attempted to read archive helper after failed preparation; file absent and no archive operation executed. Saved preparation script produces file before inspection. Additional event: Read-only rg positional Windows directory wildcard rejected as invalid path. Recovery used literal internal root with rg --files glob filters; no mutation. Additional event: R8 inspection used guessed run-native-browser.py although directory inventory named run-browser.py. No execution or source change; corrected literal path read before start. Original command failure retained.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "preflight/management-role/redundant-require-clean",
    "position": "before-production-write",
    "count": 1,
    "impact": "Management checker rejected writer-only redundant flag; correct canonical management invocation subsequently passed",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-evidence/remote-parent-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "First public preflight upload rejected before execution because own remote evidence parent was missing. Created exact parent; new public-before-r2 passed, original tool error retained",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/ocr/unsupported-plural-subcommand",
    "position": "before-production-write",
    "count": 1,
    "impact": "rules command rejected by canonical wrapper. Failed output retained; read workflow and used rule with exact preview paths in fresh rules-r2.json",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "commit-metadata/independent-review/invalid-provider-trailer",
    "position": "before-production-write",
    "count": 1,
    "impact": "Writer advisory rejected free-text provider trailer qualification. Before L3/native acceptance appended conforming provider trailers in new commit with identical tree; original bound notes/commit preserved, no amend.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/local-preview/incorrect-cli-options",
    "position": "before-production-write",
    "count": 1,
    "impact": "First Mock preview used unsupported --repo/--acceptance-profile options. No process started; canonical workflow read and --project-dir/--profile new local-preview-r2 started successfully, later stopped on SHA change",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/l3/test-fixture-pagination-type",
    "position": "before-production-write",
    "count": 1,
    "impact": "Exact L3 r1 stopped web lane at TS2353, Go lane cancelled and deploy not-run. All 12 original files hash-recovered; one-line test type corrected, new candidate full L3 r2 required.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ssh-runner-identity/powershell-remote-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "Read-only docker ps format containing spaces lost quoting through Windows SSH; no mutation or identity evidence used.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "preflight/evidence-read/remote-path-not-ready",
    "position": "before-production-write",
    "count": 1,
    "impact": "Read-only L3 r2 tail before remote log existed; retried only after actual orchestration created the path.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/l3/optional-dom-scroll-method",
    "position": "before-production-write",
    "count": 1,
    "impact": "Full L3 r2 found missing scrollIntoView in existing multi-window test DOM. One optional call corrected; existing regressions passed; full L3 r3 required.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/patch-transfer/multiline-hunk-escaping",
    "position": "before-production-write",
    "count": 1,
    "impact": "First outside-repository cleanup helper patch rejected for missing hunk line prefixes; no file or deletion happened. Raw string patch corrected, two exact failed runs cleanup completed.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/evidence-writer/powershell-inline-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "Archive helper preparation inline node text rejected by PowerShell parser before execution; moved exact code into saved script.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/evidence-writer/js-template-backticks",
    "position": "before-production-write",
    "count": 1,
    "impact": "First outside-repository acceptance writer tool script failed parsing nested template backticks before any file write. Saved array-based writer prepared successfully; no docs commit or publication executed.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/insufficient-pagination-fixture",
    "position": "before-production-write",
    "count": 1,
    "impact": "Native r1 uploaded and selected both real results with zero page/HTTP errors, but 125 fixtures did not exceed actual 500-entry default page; pagination assertion stopped. All14 evidence files hash-recovered, own cleanup passed. Fixture now505; real transfer session commit used for delayed navigation, viewport screenshots bounded; product source unchanged.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/unloaded-readiness-anchor",
    "position": "before-production-write",
    "count": 1,
    "impact": "R3 readiness waited for README beyond real500-entry first page after larger fixture. API/page errors0 and cleanup passed,14 files hash-recovered. Readiness nowexisting directory sorted before files; real pagination still exercised by upload destinations. Product unchanged.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/browser-trace-memory-budget",
    "position": "before-production-write",
    "count": 1,
    "impact": "R5 browser-only container OOM at1GiB while1500-entry full DOM snapshot trace ran; Panel/Agent noOOM. All12 raw files recovered and own cleanup passed. Reduce600 fixtures still exceed500-entry default, keep original1GiB budget, retain screenshot trace/actual API/DOM assertions/durable bytes, omit expensive trace DOM snapshots. Cause attribution to snapshot overhead is analysis, no product heap claim.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/context-menu-after-scroll",
    "position": "before-production-write",
    "count": 1,
    "impact": "R6 real cross-page multi-upload passed; subsequent right-click rename menu absent during long-directory interaction; cause from scroll closing is analysis. Zero API/page/console errors, cleanup/resource passed,16 files recovered. Use existing row operation button after observed stable geometry; no synthetic handlers/product changes.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/service-cap-vs-ui-default",
    "position": "before-production-write",
    "count": 1,
    "impact": "Earlier fixture metadata/reports confused Agent max500 with actual web API default100. Canonical api.ts list and recorded offsets100,200... prove100. Raw prior incorrect metadata retained; new150 fixture uses authoritative100; final acceptance explicitly corrects distinction.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/overbroad-helper-replacement",
    "position": "before-production-write",
    "count": 1,
    "impact": "R7 real uploads/rename/archive/extract4 cases passed; helper regex wrongly transformed directory dblclick to menu promise. Zero API/page errors,16 files recovered, cleanup/resource passed. Literal multiline copy journey corrected, preserve all failures, full native rerun required; product unchanged.",
    "recoveryEvidence": "Original commands, failed L3 kits and replacement receipts retained under C:\\GitHub\\_release-evidence\\v1.25.0-rc.15. Failed evidence never relabeled passed.",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. Localized recovery used for this preview. Shared entry/fixture regression repair is not claimed complete; recurrent preventable fingerprints require implemented repair and regression evidence before production.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/acceptance-metrics/failure-recovery-state-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Document draft said no production rollback/hotfix/republication while filling production recovery details for two unpublished L3 candidate failures. Canonical metrics rejected it before docs commit/push. Preserve original draft/log and 27-event normalization; keep production recovery not applicable, retain candidate detection/recovery/gate narrative separately, add this event and rerun exact docs validation.",
    "recoveryEvidence": "C:\\GitHub\\_release-evidence\\v1.25.0-rc.15/docs-draft-failed-r1.md; docs-metrics-before-commit.log; docs-metrics-before-commit-r2.log",
    "permanentAction": "Owner: release task; review 2026-10-14 and before next L3 production write. This helper corrects the preview document; shared generator and regression repair is not claimed complete.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

最近5正式记录v1.24.0…v1.20.0逐一读取，路径/quoting语义归一不减次数，局部绕行不称永久修复；负责人release task，复核2026-10-14且下次L3生产写前，复现根因需共享入口/fixture修正及回归完成。

## 遗留风险与资源回收

失败L3临时clone/重复bundle已回收915039829bytes；净空闲变化单列failed-runs-cleanup-results.json，保留原日志、plan/status/manifest及本地self-contained bundles。
最终own Mock、local node_modules/dist与成功L3临时clone/重复远端bundle按project-management13.1精确收尾，结果cleanup-closeout-result.json；此文档提交时最终清理待收尾，不提前声称已回收。禁止shared Docker prune/cache和作者工作树物理删除。
实体router/全平台/WAN/soak/CF完整审计与RC13性能仍未验证；继承64MiB约1.75x、小文件约8.13x耗时与目录约两倍临时空间，无本轮新perf测量。只公开preview，mandatory范围内门禁和Linux真实旅程通过，无生产部署或完整CF/实体准入结论。
