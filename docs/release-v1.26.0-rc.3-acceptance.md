# KPanel v1.26.0-rc.3 预览版发布验收

日期：2026-10-10

发布级别：L3

候选提交 / 标签：`3cf0589d109f7beded30aaf2417a0432e3751e18` / `v1.26.0-rc.3`

上一稳定版本 / 回滚点：`v1.25.1` / `8affed894e4dd0a2a2f0af3c1f1d12318a60361a` / `sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`

`releaseChannel`：`preview`

`releaseTrain`：`1.26.0`

产物已发布（2026-10-10T12:23:24Z），生产未部署。

## 候选分支与发布后处置

- 唯一预览序列候选 `release/v1.26.0-candidate` 保留在 `3cf0589d109f7beded30aaf2417a0432e3751e18`，本次 RC 不归档。不可变 tag 为恢复依据。
- 新来源：优惠栏目 `feat/offers-posters@4884b5afa78a311b76dc243986292a58ddba3ff8`（同时含远端 claude/offers-column）、集群/分享布局 `claude/cluster-layout-polish@8b6edbf9286bb215a0606c5c38170dddf2e06fbd`、快捷方式图标 `fix/file-shortcut-artwork@c341ced0f1019024462bac5f49b8be4144ad6d12`。优惠桌面图标 `e3261e8af5952add5e2ff4c9ecc7512fcf43c22c` 的精确 blob 已在优惠来源中，不重复应用。
- 来源祖先全部保留。原作者工作树与分支所有权未明确释放，物理保留并列为已发布、本地待处置，下一轮不再重复算待发布功能；责任人为来源维护者，所有权释放或 tip 变化时再核验。旧下载/Docker/桌面/Office/续签已随 RC1/RC2 纳入；其原作者资源继续保护。
- 管理 main 基线 `d11b1aca7989e50a0ed49ad5bda6d40262d358ff`；旧 RC2 已在历史中，向前恢复其产品/支持文件后再合并新来源，没有重写共享历史。12 个恢复支持文件字节与 RC2 相同，mock API 另含两行优惠入口。
- 本任务集成修复 `080cb99f6d1907b1f92c62f187e4436616631d9b` 已纳入产品 tag；验收候选在同 SHA 候选/main CI 后精确归档。自有工作树与临时资源在 closeout 核对后回收，实际结果写外部回执；本记录不提前宣称已清理。

## 发布画像与范围

- 业务域：优惠展示、集群和公开分享可读性、桌面快捷方式；继承 RC 的文件下载、Docker、Office与续签。
- 新增优惠栏目：固定公开 manifest、3 张主推轮播及卡片墙；图片摘要/格式/尺寸验证后以同源缓存服务。鉴权 GET，来源不可由用户指定；失败保留旧刊，过期/无内容/重试有明确反馈；推广链接标明广告和目标主机。
- 集群与默认公开分享增加系统/地区悬浮信息，区分网络速率与累计量，收紧列表/卡片宽度和窄屏布局。公开字段白名单不扩展，已有自定义 iframe 不变。
- 文件快捷方式采用纯色类型底板，网站图标缺失时使用统一 fallback；已有加载成功的 favicon 保留，开始菜单和确认框一致。
- 集成修复：失败刷新也清理未被采用的图片对象，新增有/无旧缓存、连续不同失败发布的回归；动效复用现有 fast/fade/standard token。没有提高动效或性能预算。
- 完整恢复 RC2 的 HTTP 智能下载/本机 BT、新 Docker 批处理，以及 RC1 桌面材质、Office 和手动证书续签。Docker 新批处理已按用户后续明确授权纳入；旧持久批队列设计仍不在范围。
- 新栏目外联与私有缓存属于新信任边界；公开版本、镜像和更新通道变更按 L3。Agent 权限、既有文件/更新 API、端口、Compose与应用市场契约沿用；优惠桌面隐藏项 schema 5 需按下文回滚。

## 外部审计与修复交付

- 精确产品覆盖检查：decision=scoped-required，69 个未审计提交、最早 6 天、完整审计年龄 19 天；新包 internal/bittorrent、internal/offers。RC 只记录，不能用于稳定版豁免。
- CF run-33：源码 `4884b5afa78a311b76dc243986292a58ddba3ff8` / tree `5bf89b98c73e1b0e2edfec2ca184a2c64326ae9b`，固定 Cloudflare skill `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`，专用父/子代理 gpt-6-luna/max。source-only、7 单元、9 唯一代理/10 次调用；2100 秒预算到期，run_status=incomplete、scope_complete=false，不计完成覆盖。
- 实际开始时间不可重构，保留 null；可靠 checkpoint 为 10:41:44Z、预算截止 11:16:44Z。浏览器/桌面 hunter 未完成，critic、独立 verifier、Phase-5、最终 findings/coverage 验证及修复源码比较未执行。分配 ledger 的受限 Runner 结构 PASS 不等于审计完成。
- offers.cache.orphan-growth-on-failed-refresh 仅为 needs_validation 假设，无独立确认或评级；固定站点发布权限、实际 DataDir 配额与影响未验证。发布任务的功能清理修复和 L3 回归不代替该安全验证，不关闭候选。
- run-31/32 及既有 terminal-sse-output-after-session-revocation、kpanel:rollback-restores-revoked-light-node、Bittorrent.PrivateMagnetDHTBeforeMetadata 等范围外 needs_validation 保持原状态。安全维护者 2026-10-15 或稳定提升前完成独立补审；原有更早截止继续有效。
- 仅入库脱敏 incomplete 元数据，原始审计记录与缺失原件说明保留外部 cf-offers-run；没有导出的 hunter 逐字原件不伪称已保留。
- OCR 1.12.11：bcc80ec..080cb99，91 文件中 65 个可评审文件 65/65，26 个文档/二进制/锁文件按规则排除；先自由后约束，free-form=3，成立 H0/M2/L1，constrained-only=0。3cf0589d 与复核头 tree 相同，以 trailer 绑定；不足三个周期不推断工具有效性。

## 跨仓库联动判定

- scriptLinkageState=coupled；变更集 `kpanel-v1.26.0-rc.1-manual-certificate-renewal`。
- 内置脚本 `1800d955f216aebd2776674368a8e469a671c489` / SHA-256 `5778fdc9637c8614f5246f9eb13c1e549de51522fb91b3805067cbc0475b614c` 已随 RC1/RC2 公开，重用已有兼容发布，不新增 sh/apps 写入。相对当前稳定版切换回续签脚本配对，不能标为 not-required。
- 完整 L3 managed script、锁兼容、应用生命周期和备份回滚证明本轮组合契约；真实 CA 签发未验证。回滚需恢复匹配的 Panel/脚本配对，不能只回退其中之一。

## 多维质量结论

| 维度 | 状态 | 证据 | 边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 完整 L3、公开 Panel/Agent E2E、实际发布信息与优惠 API | 不外推为真实 Docker 多项操作、真实 CA 或 WAN BT 准入 |
| 网络入侵与供应链安全 | 已验证 | L3 配置范围内扫描、公开 OCI/script/附件身份 | CF scoped 未完成，既有风险不关闭；不是无漏洞保证 |
| 稳定性与失败恢复 | 已验证 | 全量/race、缓存回归、生命周期故障/备份门禁、UI 失败态 | 隔离夹具不替代生产故障注入或长期 soak |
| 性能与资源预算 | 已实现未实机验证 | 本轮资源看护通过；界面布局与动效回归 | GPU 经典拖动 +25.485% 超 20% 预算仍试行；最终依赖图吞吐未重测 |
| 用户体验与可访问性 | 已验证 | 4 组功能 UI /26 PNG +8 项公开信息 /8 PNG，键盘、真实 200% | 未覆盖 125%、读屏、实体手机、Safari/Firefox、原生 ARM64 |
| 数据、配置与迁移 | 已验证 | workspace/隐藏状态、公开分享白名单、升级与备份门禁 | 回旧版前先恢复隐藏优惠入口；无生产数据操作 |

## 自动门禁

- Canonical L3 run `v1.26.0-rc.3-3cf0589d-l3-r1`；候选 `3cf0589d109f7beded30aaf2417a0432e3751e18`，基线 v1.25.1。唯一外层 native exit 0、远端 passed/0、原件全部 hash 核对，watchdog passed。证据 `C:/GitHub/_release-evidence/v1.26.0-rc.3-3cf0589d-l3-r1/remote-evidence`。
- 固定 Runner `sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`；manifest `18b8b488246dc9886b5920fe381f4a1ff0bf359b280902e54174e5218ed18f77`；bundle `00829c628cf4847a23fd1c7cf85b53d366e29aa05894a7003c14d66b7c378fb7`；plan `1fda4dd049617c6e7a2c55a82bf4dc0620882d4b1d9c44bdf3dd4a7e594c5c9a`；remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- 源码检查：source_lane=web status=passed duration_ms=636682; Test Files  271 passed (271); Tests  2529 passed | 6 skipped (2535); source_lane=go status=passed duration_ms=945272; source_lane=deploy status=passed duration_ms=3326; source_checks=pass candidate=3cf0589d109f7beded30aaf2417a0432e3751e18 duration_ms=945316 identity_unchanged=true。Go 全量、特权核心 race、vet、前端 typecheck/test/build、治理和部署门禁通过；新缓存回归属于该精确候选全量测试。
- govulncheck 可达与已导入漏洞均为 0，仍有 1 个未调用的依赖模块信号；npm audit 为 0，Trivy 源码与镜像在配置扫描范围内为 0，不把未扫描项当作 0。双架构构建、公开镜像 image E2E、app_conf_lock_compat/lifecycle/update_backup_parity 通过。预期故障注入拒绝/Killed 保留原日志，不记为额外失败。
- 候选 CI [38048876751](https://github.com/kejilion/KPanel/actions/runs/38048876751)；Dependency freshness [38048876852](https://github.com/kejilion/KPanel/actions/runs/38048876852)。主线 CI [38049714516](https://github.com/kejilion/KPanel/actions/runs/38049714516)；Dependency freshness [38049714527](https://github.com/kejilion/KPanel/actions/runs/38049714527)。
- Release [38050435163](https://github.com/kejilion/KPanel/actions/runs/38050435163/attempts/2)；Tag Dependency freshness [38050435159](https://github.com/kejilion/KPanel/actions/runs/38050435159)。均为精确产品 SHA 的成功终态。
- Release 首轮 attempt 1 在 TestViewHonoursPublishingWindow 的 TempDir 清理失败（后台刷新仍可能写 objects），业务断言未失败，所有产物步骤跳过；原始 312233 字节日志和失败 API 保留。完整 attempt 2 成功后放行，没有跳过失败步骤或改写 tag。夹具 Close 未 join 与 fake fetch 忽略取消的退出竞争，需优惠/测试维护者在 2026-10-15 或稳定提升前修复并补确定性回归；本次恢复不伪称永久修复。
- 草稿基于 frozen Git archive 经规范 Go parser 通过；公开正文实际 8 条摘要/3 条升级提示，由实时 Panel API和弹窗逐项比对。公开 OCI attestation entries=2；workflow 配置 SBOM/provenance，未另称独立密码学签名验证。

## 依赖与技术栈变化

- 继承 RC2 已修复的 Go1.27.2、x/net v0.60.0、OpenTelemetry 1.47.0 与 RC2 BT 依赖；前端锁图只变根版本号，没有临时扩大冻结依赖范围。
- 候选/main/tag 的 Dependency freshness 完整检测成功，以原 API receipt 的实际运行时间为证；没有另行下载报告正文，不编造候选数量或 generatedAt。每日安全旧失败记录和旧源码基线不改写。
- EOL 最近政策复核 2026-07-28、最长 92 天，freshness 门禁通过。直接/基座与传递依赖继续按既有分类期限处置；未把未知版本写成最新。
- 固定 Node24.21.0 与扫描器沿用；Trivy0.74.0 提示新版的后续评估责任归依赖维护者，2026-10-17 前复核兼容/资源/回滚，不在冻结后换扫描器。最终图的 WAN/大任务/长期性能未获新准入。

## 隔离真机与浏览器验收

- arena-154，Debian13/x86_64，候选与 browser-validation 用途；浏览器全部远端后台 headless，未调用用户本机前台浏览器或 GPU。
- 功能 UI r5 绑定本提交：1280 深色中文 100%、1280 浅色中文真实 native zoom 200%、390 深色英文、390 浅色繁体，CSS zoom=1。4 组 passed/0，26 PNG 实际查看并核对 SHA，4 trace 保留；无 page/console/resource error、无横向溢出。
- 覆盖优惠轮播键盘/暂停/减少动效、广告链接、刷新失败/恢复/无内容、隐藏入口后开始菜单可打开并恢复、集群与默认公开分享列表/卡片和悬浮信息、纯色快捷方式确认/取消/准确键盘目标。mock 页脚版本不作公开版本证据。
- 公开镜像：`docker.io/kjlion/kejilion-panel@sha256:5be7461cdbe50dc2a933f796441c02e20e37cdf11ee4b27226ff80f30113f5a3`，canonical image_e2e=pass；专属 fixture 容器/网络 before/after 相同。
- 公开后台 job `arena-154-53124`，passed/0，8 项 actual API/弹窗，8 PNG 实际查看通过、2 trace 保留；Chromium 140.0.7339.16 / Playwright 1.55.0 / Node v24.18.0，固定 Browser Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`。
- 实际 RC3、稳定 v1.25.1、上一 RC2 Panel/Agent；使用真实上游发布正文/摘要。稳定与 RC2 加入预览立即查 RC3，三种 stable 视口/主题及 RC2 窄屏弹窗，当前 RC3 stable 通道拒绝降级。自动安装关闭，安装请求 0。
- 实际 RC3 Panel 从固定公开优惠源获取刊物，活动条目与实时 manifest 相同，每个同源图片实际响应的 SHA/字节与 digest URL 一致；具体数量和媒体哈希见 actual-offers-media.json。没有用 mock 代替该公开镜像取图证明。
- 资源失败/OOM/残留私有 fixture 均 0。Docker 代理只允许真实健康和强制专属空应用命名空间 GET，宿主 Docker socket 未挂入 Agent；不是 Docker 批处理完整真实引擎验收。
- 普通确定性交互无新增 soak；实体手机、Safari/Firefox、低端 GPU、ARM64 实机、完整回滚、CA 签发与公网 BT 长稳未验证。桌面负责人 2026-10-16 或稳定提升前复核 GPU 试行预算。

## 发布产物与公开仓库复核

- [KPanel v1.26.0-rc.3](https://github.com/kejilion/KPanel/releases/tag/v1.26.0-rc.3)：draft=false、prerelease=true；GitHub Latest=v1.25.1。
- 版本与 preview OCI index `sha256:5be7461cdbe50dc2a933f796441c02e20e37cdf11ee4b27226ff80f30113f5a3`；amd64 `sha256:411d36eb3f701492bb7b841dc4e12943bb4634b61007e2e17d0d202e578308c5`；arm64 `sha256:bcced194688840560ffb9afb95780c0ab20073f2e8b23fc0bfe12a6fc5ec94cb`。两平台 version/revision/script labels 与冻结源码一致。
- stable latest 保持 `sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`；本次只更新 preview。上一 preview 回滚点 v1.26.0-rc.2 / bcc80ec5 / `sha256:46ee78d7e322adf15f7a9664d225d6134311ecc4f2090191da3a25f8c02f1f84`。
- 附件14项、SHA256SUMS11项与 API digest 一致；meta VERSION、四项 BT LICENSE、LICENSE/THIRD_PARTY_NOTICES 字节与 Git 对比。未下载所有二进制字节，不把 API digest 说成独立全量下载验证。
- 官方 Release 正文、真实 Panel API、更新弹窗版本/摘要/digest/链接一致。apps/sh 本轮无写入，使用已有脚本配对与 digest 固定安装协议。

## 自更新通道验收

- stable 只选择正式 GitHub Latest，preview 选择规范发布并核对唯一官方 digest；实时两通道 API 均通过。
- 加入预览只切换来源并立即检查；自动安装开关保持关闭，未执行一次性安装。退出预览时当前 RC3 高于稳定，不产生降级候选。
- 旧状态默认 stable、重启持久化、systemd执行/更新前备份/失败隔离沿用源码与完整 L3 生命周期证明；本轮没有额外真实安装或生产恢复证明。
- OpenRC 不启用需要 systemd 超时控制的手动续签；轻量 Node 不获得更新/BT等 Panel 能力，边界沿用 release-channels。

## 生产部署安全核对与回滚

- 不适用（预览版禁止生产部署）；生产未部署，无生产写入、备份、升级、回滚或生产健康证据。
- prod-108/108/108.165.140.213：禁用全部 KPanel 操作，本次未连接、未部署、未核对。
- 隔离自动门禁、公开 image E2E 与浏览器不替代生产管理员写入证明。
- 回上一 RC2 前恢复被隐藏的优惠桌面项，再恢复对应 Panel/脚本配对和必要配置备份；向稳定 v1.25.1 回退须先处理 RC 数据与功能差异，不能以切换通道触发降级。未执行完整现场回滚。
- 公共默认入口仍为 v1.25.1；没有改写旧 tag、Release 或不可变版本镜像。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-10T12:33:06+08:00
- 候选冻结时间：2026-10-10T11:09:41.526Z
- 生产完成时间：未验证
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：14
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

本轮只新增一个预览发布，未新增稳定标签或生产部署。流程计数按实际事件，含非阻断 CF 取证中的失败尝试；与产品门禁失败、生产变更失败分开。没有产品重复发布。逐项读过批准 main 最近五个正式验收 v1.25.1/v1.25.0/v1.24.0/v1.23.0/v1.22.0；Go PATH 原因复发沿用同指纹，其他相关大类不伪称同根因。所有事件发生在生产写前；没有生产逃逸事实。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/create-worktree/projectless-cwd",
    "position": "before-production-write",
    "count": 1,
    "impact": "Managed worktree creation failed with Not a git repository because the chat directory was C:/GitHub. No shared files were overwritten.",
    "recoveryEvidence": "Original tool response retained in the task transcript; no separate raw file is invented. The owned integration-fix worktree was created through Git against the verified KPanel repository and passed writer qualification before edits.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Qualify the repository path before choosing the worktree API. Current dedicated Git fallback is recovery, not a shared tool repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "inputs/task-preflight/unsupported-option-order",
    "position": "before-production-write",
    "count": 1,
    "impact": "First ready invocation used --phase ready instead of the required positional ready subcommand; it returned invalid option before qualification.",
    "recoveryEvidence": "Original tool chunk b64d51 retains the failure. task-integration-ready.log and task-final-ready.log retain the corrected positional canonical invocation.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Reuse the prepared positional command array and keep CLI qualification before dependent work; no shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-notes/go-not-on-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "The Windows draft command rendered text but its Go parser could not run because Go was absent from PATH. That output was not accepted as parser evidence.",
    "recoveryEvidence": "Original tool chunk b64d51 and draft-render.log retained. The exact frozen Git archive passed the canonical Go publication parser in fixed Linux Runner r2; draft-parser-qualified.json records source, tool and raw log hashes.",
    "permanentAction": "Release tooling maintainer; review 2026-10-12 and before the next L3 production write. This repeats the v1.25.0 canonical parser/PATH cause. Use the prequalified Linux parser controller; complete a shared preflight and regression repair before any later production write. Current external recovery is not claimed as that shared repair.",
    "historicalReleases": [
      "v1.25.0"
    ]
  },
  {
    "fingerprint": "browser-validation/offers-preview/skeleton-before-items",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r1 asserted 14 tiles after the skeleton wall became visible, before asynchronous data tiles were present. Candidate source was unchanged.",
    "recoveryEvidence": "ui-browser-job-r1 terminal and controller log retained. The r5 harness waits for actual non-placeholder tiles and all four cases passed with 26 reviewed PNGs.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17. Retain the corrected readiness condition with this fixture and qualify real data readiness in the next canonical fixture revision.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/offers-preview/invalid-fixture-state",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r2 supplied fresh, which is outside the declared live/stale/unavailable API enum. The product correctly rejected the malformed fixture.",
    "recoveryEvidence": "ui-browser-job-r2 terminal and raw controller log retained. A separate r5 attempt uses live and covers empty/unavailable/retry states without changing source validation.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17. Keep fixture values bound to the actual API type; do not relax product normalization.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/shortcut-preview/ambiguous-keyboard-target",
    "position": "before-production-write",
    "count": 2,
    "impact": "UI r3 and r4 used a search term matching both the system monitor and a URL shortcut. Enter initially activated the system item; the first harness correction missed the second activation site.",
    "recoveryEvidence": "Both failed job terminals, failure observations/screenshots/traces and diagnosis records retained. The r5 harness selects the exact aria-activedescendant at both sites, checks cancel/confirmation/open URL and passes all four cases. These remain two events in one root-cause batch.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17. Centralize exact-key keyboard selection in the maintained fixture and cover every activation site before running the full matrix.",
    "historicalReleases": []
  },
  {
    "fingerprint": "intake/candidate-freeze/archive-inventory-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first freeze helper compared 84 total refs against a prior active-only inventory and mistook archived refs for newly changed candidates. No actual active candidate changed.",
    "recoveryEvidence": "Original chunk 6c8a2d and intake-freeze.json retained. pre-freeze-r2.cjs applies the archive prefix consistently; intake-freeze-r2.json reports changed=[] with all 84 refs shown transparently.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17. Bind each inventory to its explicit active/archive classification and compare only matching populations; this external guard repair does not alter release source.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-notes/fixed-runner/noexec-scratch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Draft parser r1 compiled but could not execute its Go test binary from the default noexec /tmp tmpfs. It exited 1 without establishing a product parser failure.",
    "recoveryEvidence": "draft-parser-r1-container-receipt.json and stdout/stderr retained. Separate r2 uses an explicitly declared executable private /tmp with the same source, Runner and command; canonical parser exits 0 and the owned container is removed.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before next production write. Include executable scratch in the frozen parser mount contract. Related historical script-smoke noexec incidents use a different authoritative entry and are not silently merged into this event.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/draft-parser-recovery/output-whitespace-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first recovery qualifier rejected a successful raw Go parser output because it expected ok followed directly by a tab; actual output had spaces before the tab.",
    "recoveryEvidence": "Original chunk 9d3c62 and unchanged r2 raw parser logs retained. qualify-draft-parser.cjs accepts the observed anchored whitespace grammar and produced draft-parser-qualified.json without rerunning or changing parser evidence.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17. Qualify the parser output grammar against preserved original bytes; canonical exit and source identity remain required.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/validator-launcher/input-dir-untraversable",
    "position": "before-production-write",
    "count": 1,
    "impact": "The initial optional CF structural validator could not read tools/input through a root-only parent directory as uid 65534. This did not execute target code or establish audit coverage.",
    "recoveryEvidence": "CF recovery note and original task return retained; the first raw runner output was not captured and is not reconstructed. Later unique staged directories with traversable parents ran the fixed validator in the same restricted Runner.",
    "permanentAction": "Security audit tooling maintainer; review 2026-10-15. Preflight every parent directory and read-only mount as the actual low-privilege uid before validator execution; keep the missing-original limitation.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/noncanonical-unit-sort",
    "position": "before-production-write",
    "count": 1,
    "impact": "A CF assignment-ledger validator retry exited 1 on the initial unsorted coverage-unit representation. The attempt did not validate hunter outcomes.",
    "recoveryEvidence": "coverage-validation-retry-20261010T110003Z.log records exit 1 and the original unsorted ledger is retained; later sorted/assignment-map structural logs pass. Terminal audit remains incomplete with critic/verifier/Phase-5 absent.",
    "permanentAction": "Security audit tooling maintainer; review 2026-10-15. Generate canonical sorted coverage IDs before frozen low-privilege validation and preserve stderr as well as stdout. Structural PASS must never be promoted to completed audit coverage.",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/owned-parser-cache/optional-cache-dir-assumed",
    "position": "before-production-write",
    "count": 1,
    "impact": "First owned parser cleanup guard required GOMODCACHE directories because they were declared in the Runner environment. The parser used only standard packages and those optional directories never existed. The guard exited 1 before any deletion.",
    "recoveryEvidence": "parser-cache-cleanup-r1/execution.log and execution-receipt.json retain the failed attempt (native chunk 4678e1); read-only inventory confirmed both absent directories. Separate r2 checked only the four actual owned directories and the exact uploaded archive, retained all qualified source/log hashes, and passed (native chunk c323f9), with observed free-space delta 348987392 bytes. No shared Docker cache was deleted.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17. Inventory optional cache directories before freezing deletion targets, retain absent-path observations, and keep complete pre-delete ownership/hash/process guards. This external helper correction is not claimed as a shared cleanup script repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/source-checks/offers-fixture-tempdir-refresh-race",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release run 38050435163 attempt 1 failed in TestViewHonoursPublishingWindow during testing.TempDir RemoveAll: objects directory was not empty. No business assertion failure is present; all artifact build/publication steps were skipped. Source reading identifies Close cancellation without joining in-flight refresh, while the fake origin ignores context and the test advances time enough to start a background refresh.",
    "recoveryEvidence": "Original full 312233-byte gh-run-tag-attempt1.log, its receipt, jobs-tag-r2.json and actions-tag-watch-r3.json retained. public-before-r2 proves no partial public Release/image and unchanged stable/previous-preview defaults. Same immutable tag received one full workflow retry, not a failed-step skip; final success must be separately qualified as Release attempt 2 in tag-ci-result.json before acceptance. No separate reproduction or permanent fixture repair is claimed.",
    "permanentAction": "KPanel offers/test fixture maintainer; review 2026-10-15 or before stable promotion. Ensure the fixture joins bounded refresh shutdown before TempDir cleanup and make fake-origin cancellation explicit, with deterministic regression. Preserve this original failure; a repeated failure must be repaired before any additional retry. Current bounded rerun is recovery, not the shared fixture repair.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与资源回收

- 安全审计与桌面 GPU/下载最终图性能边界保持开放，不能因本次 RC 通过自动门禁获得稳定准入。
- BT 仅本机 v1 种子/URL与公开 btih；未知 magnet 在获知私有标志前可能参与 DHT，私有磁力隐私不作保证。并发1、任务10GiB不是实际满额证明；需要两至三份暂存空间，跨重启中断，不做种，不支持 v2/hybrid/远端节点或跨重启续传。
- Docker 使用既有逐项 API/CSRF/resourceVersion，离开页面停止新派发；没有持久后台批队列，删卷不可恢复。真实多项 Engine 全旅程未新增证明。
- 自有预览已通过规范 stop 停止；公开浏览器 fixture 私有数据、容器和网络已回收。其余可再生自有检出、上传和 parser 缓存按精确路径与 ownership 在最终 closeout 回收，保留本地 kit、原始 hash 证据及恢复点。审计 staging 尚无完整精确归属及保留文件清单，继续保存；未知作者与共享缓存保留。实际净字节与跳过原因见本轮外部 closeout 回执。
- 只创建规范强制的本版验收与当前事实更新、最小审计元数据；未新建工作流。本文档候选/main CI、精确归档及最终公共 closeout 在提交后执行，外部回执记录终态，不提前写成已通过。
