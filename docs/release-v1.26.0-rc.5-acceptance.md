# KPanel v1.26.0-rc.5 预览版发布验收

日期：2026-10-11

发布级别：L3

候选提交 / 标签：`5cd7bcc747eefc952247312a7d672cd453609251` / `v1.26.0-rc.5`

上一稳定版本 / 回滚点：`v1.25.1` / `8affed894e4dd0a2a2f0af3c1f1d12318a60361a` / `sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`

`releaseChannel`：`preview`；`releaseTrain`：`1.26.0`

产物已发布（2026-10-10T18:39:45Z），生产未部署。

## 候选分支与发布后处置

- 唯一预览候选 `release/v1.26.0-candidate` 保留在 `5cd7bcc747eefc952247312a7d672cd453609251`；本轮不归档。不可变 tag 与本地完整 L3 bundle 可恢复产品。
- 十组来源精确 tip 已纳入产品历史。作者工作树、分支及 4179/4180/4181 预览保留为已发布、本地待处置；责任人为各来源维护者，所有权释放或 tip 改变时复核。发布任务不删除作者资源，也不重复计为待发布功能。
- 白色描边试验 fix/file-shortcut-artwork@3637c6eb9094d77545c0d3697aa3fab5dc5909e7 被 45% 白色类型底板替代；旧 fix/files-diagnostics-background@4789a74 的返工稿由 ebd748fe 成型候选替代。原分支和失败记录保留。
- RC4 候选 c9d6fe46 在候选 CI 失败后撤回，未推 main、tag、Release 或镜像。其完整 L3、失败 CI 原件与诊断保留；没有将该验证冒充本版精确源码门禁。
- 本轮文档分支 `docs/v126-rc5-acceptance` 在 L0、候选/main CI 后归档为 `archive/docs/v126-rc5-acceptance`；精确远端读回及本地分支改名见外部 docs-archive-result.json。随后同步管理 main 并清理本轮自有文档工作树。
- 形成来源的截止时间 `2026-10-10T16:55:54.017Z`；冻结后变化另记 intake-freeze-final-r3-r2.json，留待后续预览，不追改本版不可变来源。

## 发布画像与范围

业务域为下载、广告与桌面、终端、容器管理和体检报告；变更面包含展示、既有会话边界、宿主机容器 exec/精确进程清理以及有界 BT 重拨，风险等级 L3。

- 文件及 ZIP 恢复有效登录 Cookie 下载，移除临时凭证签发/缓存/免 Cookie 路由。保留路径/权限、Agent 路由、Range/HEAD 和流式时限。外部下载器必须携带会话；大 ZIP 仍受请求行限制，退出拒绝新请求，不保证在途流即时撤销。
- 广告栏目精简重复页头，状态和刷新移到更多厂商标题行，保留 AFF 页脚；历史监控/进程管理采用 512px 图标，文件快捷方式采用 55% 类型色/45% 白色底板。
- 主机、应用与诊断输出共享当前壁纸和裁切，84% 遮罩、亮色标记 94%，输入栏实色；高对比/减少透明度/forced-colors 使用实色回退。任务终端 13px、诊断日志 12.5px 字号保留。终端订阅取消释放阻塞读取并过滤已取消订阅排队事件；后台任务继续，不等于会话撤权审计关闭。
- Docker 容器控制台采用持续交互会话，保留 cd、Tab、Ctrl+C 和终端尺寸；完整容器 ID 与 resourceVersion、会话归属和数量限制一起校验。Linux pidfd/nonce/boot/TTY 二次身份核对用于精确关闭；清理未确定时保留记录并返回待清理，Agent 下次启动先恢复。Agent unit 新增 CAP_KILL，必须随 Panel 同步升级 Agent；需要 Linux 对应进程/TTY 能力。已脱离会话的后台任务可能继续；Agent 停止期间没有独立清理守护，重启后恢复。
- 体检报告调整评分、数字单位、网络卡片和窄屏布局，保留范围/负号，进度条限制 0–100，等待态依据原字段。未开始任务不显示终端栏，clear 文件行静态透明、选中/悬停/拖放保留反馈。TcpQuality 复用既有脚本 selector，仅调整入口文案和 Mock 身份，没有真实第三方探测。
- BT 在元数据就绪、有活跃/已知公开 peer 且持续 30 秒零原始载荷时，最多恢复两次；等待被关闭的旧连接退出后重加已知 peer，收到任意载荷后永久停止此恢复。未扩大发现/隐私/拨号预算，整体无进度截止不重置；不处理已经传输后的停滞。
- 继承 RC3 的 Docker 批处理、手动证书续签、Office 基础编辑与 HTTP/BT 下载；Compose 删除已随 v1.25.0 发布。无新端口、应用市场发布、下载持久化格式或受管脚本协议变化。

精确来源：

- downloads：`6d3f401bee99f7699c96ac1762f5af3444d43741`
- offers：`db81b33c7cddf90f61c991f6a275587fc48b4674`
- icons：`ef2a4a8d9865b64acc319a136d91fcdb3b6cadaa`
- lighter：`14a1ecfcf3fbfa31553662bf7ec97a81a0103ae7`
- terminal：`59a28b3ea5f7c1948ef4c5cdc2cef2284d75d0ce`
- tcpquality：`cfeaf2775ba1eed712d79b062e7106a4593b6399`
- cancel：`47a8b81569917e81de393dec78184fc25da37c66`
- dockerTerminal：`3ecb23750ad0ef555f1110eb76c8952eed97eac7`
- diagnosticsPolish：`ebd748fe8ff4ab7f09754fe8ed7c4e3f06a0d896`
- btRecovery：`50d60ff5933dc8edc8a561514fba0551328e21b5`

批准 main `7bcd6bd5892310e5e39ac5e0596a4199de671293`，上一公开预览 `3cf0589d109f7beded30aaf2417a0432e3751e18`。尚未形成并交接的分支及截止后新工作不纳入。

## 外部审计与修复交付

- 自由复核和 OCR 1.12.11 约束复核覆盖 65/65 代码文件，H0/M0/L0；精确 blob 继承、增量集成和排除的元数据/资产人工复核见 ocr-final-r5 与 source-blob-witness-final-r5.json。blind=false、constrained-only=unreported，不能提前声称增量工具有效。
- 来源外部复核和原始修正记录保留。BT 最终 50d60ff5 由不同代理独立复核 PASS；其他供应商非交互入口不可用，使用不同实现会话的同供应商回退，不冒充异供应商盲审。
- CF 精确目标 `5cd7bcc747eefc952247312a7d672cd453609251`，decision=`scoped-required`，待审 75 提交/194 文件，最早 6 天，距 full 19 天。本轮 scoped/full 未执行，预览如实延期；新增容器控制台权限和 BT 边界未计为 CF 已完成覆盖。
- run31/32/33 incomplete，critic/verifier/Phase-5 和原始 hunter 缺口保留。SSE 撤权输出、撤销轻节点回滚、私有 Magnet 元数据前 DHT、RC3 cache-cleanup 独立关闭仍需验证；终端取消与本次开发门禁不替代这些结论。
- 稳定提升前完成补审/独立验证，或按项目规则取得明确有限期决定；本轮没有新的稳定豁免。若执行后续 KPanel CF scoped/full，按用户规定使用 gpt-6-luna/max 专用审计代理与不同发现/验证代理。

## 跨仓库联动判定

- `scriptLinkageState=not-required`：无需发布脚本（不适用）；跨仓库变更集、脚本候选和依赖移除均不适用。
- 内置 revision `1800d955f216aebd2776674368a8e469a671c489` / SHA-256 `5778fdc9637c8614f5246f9eb13c1e549de51522fb91b3805067cbc0475b614c`，公开双架构标签/字节与契约一致。新增 CAP_KILL 是 KPanel Agent unit 调整，不是 kejilion.sh 协议变化。
- 无 sh/apps 外部写入。应用市场默认 latest 和正式更新入口继续选择 v1.25.1，历史回退使用对应历史脚本配对。

## 多维质量结论

| 维度 | 状态 | 证据与未验证边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确完整 L3、公开 Panel/Agent 镜像 E2E 与真实小文件/ZIP；Docker 核心真机证据来自精确来源，集成 UI 使用 Mock；没有新增真实 CA 或完整 Engine 多项批处理验收 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 强制扫描与独立复核通过；CF scoped/full 未完成，不能作为完整边界安全结论 |
| 稳定性、失败恢复与兼容 | 已验证 | BT 同夹具未修复失败/修复 10 轮 race、完整源码/race/双架构构建与包装生命周期；无 WAN 长稳或下载重启续传准入 |
| 性能与资源预算 | 已实现未实机验证 | 固定 Runner、冻结预检和 watchdog；GPU 拖动 +25.485% 超 20% 试行预算仍开放，本轮无吞吐/GPU 结论 |
| 用户体验与可访问性 | 已验证 | 4 组/56 PNG、原生 200%、390px 英/繁体与媒体回退；公开 8 项/8 PNG。没有实体手机、原生 ARM、Safari/Firefox 或屏幕阅读器结论 |
| 数据、配置与迁移 | 已验证 | Agent 新增私有终端清理记录并先恢复再开放会话，来源真机 crash/restart 证据；没有生产迁移或独立停止 Agent 守护验证 |

## 自动门禁与失败恢复

- 唯一完整 L3 入口 scripts/run-release-l3.mjs --execute-kit；runId `v1.26.0-rc.5-5cd7bcc7-l3-r1`，manifest SHA-256 `c10ca50c7b6779373bc79fa89c99957d80dd53d19b8267080a302b3662d848b5`，固定 Runner `sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`（Go1.27.2/Node24.21.0）。完整 bundle、80 个必需标签、终态 passed/exit0、逐项哈希恢复和 watchdog 见外部原件。源码摘要：source_lane=web status=passed duration_ms=618504; Test Files  273 passed (273); Tests  2543 passed | 6 skipped (2549); source_lane=go status=passed duration_ms=941730; source_lane=deploy status=passed duration_ms=3366; source_checks=pass candidate=5cd7bcc747eefc952247312a7d672cd453609251 duration_ms=941781 identity_unchanged=true。
- 候选 CI [38073849844](https://github.com/kejilion/KPanel/actions/runs/38073849844) / Dependency freshness [38073849830](https://github.com/kejilion/KPanel/actions/runs/38073849830)；main CI [38074787542](https://github.com/kejilion/KPanel/actions/runs/38074787542) / Dependency freshness [38074787554](https://github.com/kejilion/KPanel/actions/runs/38074787554)；Release [38075708465](https://github.com/kejilion/KPanel/actions/runs/38075708465) / Dependency freshness [38075708511](https://github.com/kejilion/KPanel/actions/runs/38075708511)，均精确本版 SHA，attempt 按各原始记录。
- 草稿在固定 Linux Runner 调用真实 Go publication parser；公开正文也经规范脚本及实际 Go 解析门禁；正文摘要 12 条、升级提示 4 条，面板保持既有 8 条摘要/3 条提示/每条280字展示限额。没有以手写预期替代实际正文解析。
- govulncheck、npm audit、Trivy、镜像/脚本契约、SBOM/provenance 按完整 L3/Release 原始结果记录，不放宽扫描严重度。本版本地 L3 govulncheck 可达漏洞0、imported package漏洞0，另有1条所需模块提示但未判定代码调用受影响；npm audit为0，Trivy所扫描的源码/锁文件和Dockerfile目标为0漏洞/secret/错误配置。这些扫描不替代未完成的 CF 边界审计。
- 撤回的 RC4 CI [38066797030](https://github.com/kejilion/KPanel/actions/runs/38066797030) attempt1 在既有 12MiB/120 秒 BT 夹具出现活跃 peer/零字节，未报告 Go data race。原始同源码 20 轮诊断均通过，唯一根因未确认，未盲目重跑或推 main/tag。
- 本版合成零载荷 peer 后重拨真实 Seeder；同最终测试夹具未修复基线正常编译后 40 秒截止失败，修复版纯状态/完整字节/两次拨号/单次 announce/1 秒旧连接关闭/空缓存与原 Panel 夹具 10 轮 race 全通过。保留每次失败和独立复核。
- 新测试的 Seeder 与客户端仅按锁定上游 TestingConfig 使用 1ms KeepAliveTimeout，隔离后续夹具 writer 停滞；生产仍为 1 分钟。原始 CI 唯一根因及诊断中后续部分传输停滞唯一原因均未确认，不用该配置宣称生产问题全部关闭。

## 依赖与技术栈变化

- 本轮未升级依赖版本、npm 锁图、Action、基础镜像、扫描器或受管脚本；工具链和依赖政策沿用稳定补丁/RC3。版本字段随 RC5 更新。
- 精确候选/main/tag Dependency freshness 通过；dependency-report 的生成时间、检测源完整性和直接/基座/传递行动项保留在各自动报告/日志。未额外声称新的每日安全通告或 EOL 人工复核，也不默认所有升级候选已采用。
- pinned anacrolix/torrent v1.61.0 保留，采用本地有界恢复；没有直接升级未发布上游或修改模块缓存。性能/资源未新增基准准入，回滚点见下。

## 隔离真机与浏览器验收

- 仅 arena-154，环境策略允许 candidate/browser validation；Debian13.7/Linux6.12.107+deb13-amd64/x86_64/Docker29.6.2。无用户本地/headed 浏览器操作和 GPU 测试。
- 冻结可用磁盘 8000000000 字节以上、内存 3500000000 字节以上；实际快照 resource-freeze-r2.json，完整运行低水位见 watchdog。测试资源不代替生产资源结论。
- 集成后台 job arena-154-38856 / ui-browser-job-r3，exact source `5cd7bcc747eefc952247312a7d672cd453609251`，exit0，600 秒上限，2026-10-10T17:21:54.212Z 至 17:27:42.433Z。命令 spec SHA-256 0d23d0c70e4c2f00ace67d9b168d04361bf6fc0b6e6c72ad843668686914ebca，Chrome140.0.7339.16/Playwright1.55.0、固定浏览器 Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`。
- 1440 深色中文100%、1440 浅色中文原生200%、390 深色英文100%、390 浅色繁体100%，使用原生 Chrome zoom。最小计算字号、横向布局、keyboard/focus、正常/陈旧/错误/不可用/空广告、重试及图标 decode、文件菜单、主机/应用/容器终端、TcpQuality 确认/取消与体检报告 Mock 均通过。
- 56 PNG：42 张不相同原图由 root 亲自检查，14 张与此前亲自检查原图 SHA-256 完全一致，逐图见 ui-visual-review-final-r3.json / ui-image-identity-r3.json。200% 需普通纵向滚动，toast 可暂遮顶部总分；既有文件浮动操作条可能裁切或需滚动；英文确认主按钮保留中文，不宣称完整本地化。
- Docker 来源真机 TestContainerTerminalDockerIntegration 对精确核心代码、实际 Agent systemd capabilities，root/uid1000 的 cd、Tab、Ctrl+C、42×99 resize、关闭不杀 PID1、断线/自然退出/Agent crash 后启动恢复通过，原件 final-docker-integration.log/binary 摘要保留。集成界面是 Mock，不宣称新增真实公开 Panel-Agent Docker 全旅程。
- 公开产物 job `arena-154-16228`，spec SHA-256 `d75f915fb85635ca1a606c487a37b9b3490cbdb65d771afaeb116e97c270c5c2`，exit0，140.0.7339.16/Playwright 1.55.0/Node v24.18.0；8 项真实更新信息、8 PNG 均亲自检查。私有夹具、7 个容器和网络清理通过。
- 公开 RC5 Panel/Agent 使用只读自有 /home/rc5-download-fixture，实际 GET/HEAD/Range206、混合 ZIP 字节/安全路径、FilesView 单文件和目录 ZIP、匿名/退出后新请求401、已删除凭证路由404均通过。仅小型本机夹具，不证明 WAN、吞吐、大查询或在途撤销。
- 无新 soak 准入：长 WAN、真实 CA、完整 Engine 多项写入、原生 ARM/实体手机/GPU 未执行；容器核心定向/生命周期、BT 有界10轮与完整 L3 用于本版预览风险判断。

## 发布产物与自更新通道

- [公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.26.0-rc.5) draft=false、prerelease=true、非 Latest；版本 OCI index 与 preview 均为 `sha256:1cab7f607c8ddc6fd84489210caf544fbceb5888a2d836efecaa43f02bfbf242`。
- linux/amd64 `sha256:a08572dd9f56a3cd3747f39ffd04c553676569ad86050a05483a7f2c225d7ea3`，linux/arm64 `sha256:4dc620ad40f6ee4fb6f40a4ecc23bf252a9cd9e8d6446e7e994c447fed2673bb`；OCI version/revision/script 配对一致。ARM64 验证公开身份和构建，未原生运行。
- 14 个附件、11 条 SHA256SUMS，API 摘要/清单一致并实际下载 META/LICENSE；allBinaryBytesDownloaded=false，不宣称全部二进制字节已下载。2 条 attestation index 存在，不代表额外密码学签名验证。
- 精确公开 digest 的 image_e2e=pass。GitHub 正文含12条摘要和4条升级提示；真实发布信息 API/更新弹窗逐条匹配既有限额下的8条摘要和3条升级提示，第四条 Agent 升级提示在完整 Release 页面保留；正式版加入预览只改来源并立即检查，上一 RC3 可向前发现 RC5，当前 RC5 在 stable=1.25.1 时无降级候选，安装请求0。
- 自动安装与一次安装独立；旧状态默认 stable、重启通道保持、systemd 更新备份/失败恢复/版本隔离及 OpenRC/轻节点边界沿用既有验收，未在本轮重复实测。
- GitHub Latest、Docker latest、正式更新入口仍 v1.25.1 / `sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`。

## 生产部署安全核对与回滚

- 全部生产动作：不适用（预览版禁止生产部署）。prod-108 全程未连接、未备份、未部署、未升级、未核对；生产写操作0。隔离验收不能代替生产健康/备份/数据证据。
- 上一预览恢复点 v1.26.0-rc.3 / `3cf0589d109f7beded30aaf2417a0432e3751e18` / `sha256:5be7461cdbe50dc2a933f796441c02e20e37cdf11ee4b27226ff80f30113f5a3`；内置配对 1800d955/5778fdc9。稳定恢复点 v1.25.1 及历史原配脚本。本轮没有生产备份或生产回滚。
- 需要回退时选择对应不可变版本/digest、使用其原配脚本并按相应已授权环境流程复核；本记录只提供恢复点。预览不会将较低稳定版变成自动降级候选，公共默认稳定通道无需恢复。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-10T19:47:48+08:00
- 候选冻结时间：2026-10-10T17:33:49.545Z
- 生产完成时间：未验证
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本版 preview 未部署生产，生产时间未验证、生产用时及生产变更失败恢复不适用。候选产品质量事件另记：RC4 候选 CI 发现时间 2026-10-10T16:27:53Z；修复后 RC5 候选 CI 恢复时间 2026-10-10T18:09:25Z；未逃逸候选 CI，RC4 未推 main/tag/公开产物。此事件不计生产回滚、紧急热修复或重复发布。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：38
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "source-edit/version-file/patch-context",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial atomic version patch expected a const declaration, while the version is a var with -dev suffix. No partial modification occurred.",
    "recoveryEvidence": "version-patch-attempt1.json; actual file read and corrected version commit 8a35c781; final canonical version gate required.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/manual-review/future-timestamp",
    "position": "before-production-write",
    "count": 1,
    "impact": "Manual free-form metadata used a future literal timestamp; source and finding dispositions were unaffected.",
    "recoveryEvidence": "Original free-form retained; free-form-timestamp-correction.json records file birth time before rule output. Final four-file free-form uses actual runtime timestamp.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/syntax-extra-parenthesis",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r1 external harness failed syntax checking before journeys.",
    "recoveryEvidence": "ui-r1-diagnosis.json and job-r1 raw terminal retained; separate r5 matrix passed.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/narrow-folder-wrapper",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r2 selected a data-key wrapper not present inside the narrow folder sheet.",
    "recoveryEvidence": "ui-r2-diagnosis.json and job-r2 raw trace; corrected actual folder icon selector.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/cold-vite-readiness",
    "position": "before-production-write",
    "count": 3,
    "impact": "UI r3 timed out at 12 seconds while the cold lazy Vue graph was still loading. Additional event: UI r14 cold application route still had pending Vue modules when the 12-second manage response listener timed out; blank app capture and original trace retained. Additional event: UI r16 navigation inherited the 12-second default during offers reload, before its Vite/auth/icon graph completed.",
    "recoveryEvidence": "ui-r3-diagnosis.json, successful resource responses and pending modules retained; actual desktop readiness uses bounded 45-second wait.; ui-r14-diagnosis.json; r15 bounds app manage startup response at 45 seconds, without weakening status or terminal output assertions.; ui-r16-diagnosis.json and original trace; r17 sets bounded navigation readiness 45 seconds with unchanged UI assertions.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/obscured-underlying-icon",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r4 matched the desktop icon under an open folder modal; the modal correctly intercepted input.",
    "recoveryEvidence": "ui-r4-diagnosis.json and trace retained; scope visible button inside the actual folder. No force click.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/draft-parser/recursive-cache-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial recursive SCP recovery included parser source and caches instead of only four required evidence files.",
    "recoveryEvidence": "Exact scp PID 17600 was checked and stopped (tool chunks 12e292/a00e72); partial local copy retained. draft-parser-qualified-raw and draft-parser-final-raw separately hold four exact hash-verified files.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/parser-cache/optional-directory-guard",
    "position": "before-production-write",
    "count": 1,
    "impact": "First parser cleanup guard expected a module cache that a standard-library-only parser never created; stopped before removal.",
    "recoveryEvidence": "parser-cleanup-r1-diagnosis.json; r2 and final cleanup retain all source/archive/hash/mount checks and include modcache only if present.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/terminal-wallpaper/css-url-predicate",
    "position": "before-production-write",
    "count": 2,
    "impact": "Final UI r6/r7 compared cached data URLs or relative shared CSS URLs directly with filename/absolute computed URLs. Both failures remain separate counted events in one root-cause batch.",
    "recoveryEvidence": "ui-r6-diagnosis.json, ui-r7-diagnosis.json and both raw attempts retained. Failure screenshot personally shows orbit background and connected terminal; r8 resolves shared URL against document location, keeping selected key, image change and session assertions.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/process-history/prior-controller-root",
    "position": "before-production-write",
    "count": 1,
    "impact": "Invoked the prior release history helper directly; its __dirname targeted the old immutable evidence file and wx refused EEXIST before modifying anything.",
    "recoveryEvidence": "Tool chunk a35d60 retains the refusal; process-history-verified.json validates all five current stable acceptance byte hashes before reusing prior history.",
    "permanentAction": "Release tooling maintainer; review 2026-10-17 or before the next L3 production write. Retain exact command/source qualification and fix the maintained entry with focused regression before claiming a permanent repair. Current external controller recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/terminal-wallpaper/auto-open-host-drawer",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r8 redundantly clicked the local host already opened automatically; the host drawer was hidden at native 200%.",
    "recoveryEvidence": "ui-r8-diagnosis.json; use actual TerminalView automatic local connection, without forced click.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Preserve original attempts and exact candidate identity. Current external harness correction is recovery, not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/image-visible-before-decode",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r9 checked icon dimensions immediately after visibility while image decoding was pending.",
    "recoveryEvidence": "ui-r9-diagnosis.json; both failure images later loaded; await image.decode and retain 512x512 identity assertions.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Preserve original attempts and exact candidate identity. Current external harness correction is recovery, not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/terminal-wallpaper/vertical-scroll-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r10 expected the existing 560px terminal workspace to fit a 450 CSS-pixel viewport at native 200%.",
    "recoveryEvidence": "ui-r10-diagnosis.json; exercise ordinary scrollIntoViewIfNeeded, require complete input reachability and disclose scrolling.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Preserve original attempts and exact candidate identity. Current external harness correction is recovery, not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/assembled-ui/readable-content-readiness",
    "position": "before-production-write",
    "count": 1,
    "impact": "Personal review of all 26 UI r11 originals found undecoded narrow offer images and terminal captures before the mock WebSocket prompt; original programmatic pass did not establish complete visual evidence.",
    "recoveryEvidence": "ui-r11-visual-review.json and ui-r11-diagnosis.json; require live/refresh-stale fixture isolation, all image decode, connected prompt and actual uname response before final captures.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before the next L3 production write. Preserve original attempts and exact candidate identity. Current external harness correction is recovery, not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/preview-launch/journey-count-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "Preview launcher rejected an eight-journey specification before starting resources; canonical visual-composition maximum is seven.",
    "recoveryEvidence": "preview-start-r4.json rejected; preview-start-r4-r2.json passed after combining the related terminal/output journey.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before next L3 production write. Current external specification correction is recovery.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/external-controller/patch-newline-escaping",
    "position": "before-production-write",
    "count": 1,
    "impact": "First apply_patch attempt to create prepare-ui-r13.cjs was rejected by invalid embedded newline escaping; no helper or dependent operation was executed.",
    "recoveryEvidence": "Original tool rejection invalid hunk at line 13 retained in conversation; corrected patch and syntax qualification completed in tool chunk 33372e.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Preserve tool rejection and syntax-qualified controller.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/diagnostics/offscreen-drawer-visibility",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r13 final mock execution clicked a CSS-visible run control in the closed mobile drawer; three complete cases passed, fourth could not finish.",
    "recoveryEvidence": "ui-r13-diagnosis.json; r14 explicitly opens the mobile selector before rerunning confirmation. Original result and failure screenshot retained.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before next L3 production write. External harness recovery is not a shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-notes/source-heading/rendered-source-confusion",
    "position": "before-production-write",
    "count": 1,
    "impact": "Final cancel integration merged the candidate, then stopped before changing notes because its source-heading guard expected rendered Chinese headings instead of canonical English CHANGELOG headings.",
    "recoveryEvidence": "Original native failure chunk bd543c; merge is clean and retained. resume-final-cancel-r3.cjs validates both merge parents and canonical current-version Security heading before completing only the remaining steps.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Preserve the original failed controller and require actual source-format markers.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/transport/preview-health-readiness",
    "position": "before-production-write",
    "count": 1,
    "impact": "UI r17 remote preview-health ssh invocation timed out before any browser container was created; tunnel cleanup passed.",
    "recoveryEvidence": "ui-r17-diagnosis.json and native HTTP200 chunk f850eb; r18 bounds connection setup and remote health readiness explicitly. Root cause remains unconfirmed.",
    "permanentAction": "Release/browser acceptance maintainer; review 2026-10-17 or before next L3 production write. Preserve original transport failure and do not claim a permanent repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-freeze/evidence-reference/missing-final-cutoff",
    "position": "before-production-write",
    "count": 1,
    "impact": "Freeze r18 stopped before freeze.json, task final ready or L3 execution because an adapted late-intake-r3.json filename did not exist.",
    "recoveryEvidence": "Native failure bf29c6; existing final-source-cutoff-r3.json verified, r19 corrects that reference and writes separate fresh preflight snapshots; original partial r18 records retained.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Check all mandatory evidence paths before writing any freeze outputs; this local controller correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/bt-controller/env-key-placeholder-collision",
    "count": 1,
    "impact": "BT r1 global CACHE substitution corrupted GOCACHE/GOMODCACHE environment keys; outer baseline timeout occurred before the test ran.",
    "recoveryEvidence": "bt-diagnostic-controller-incident.json; bt-fix-r1/result.json; r2+ use exact pathlib.Path(CACHE) replacement.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/intake-record/superseded-field-shape",
    "count": 1,
    "impact": "finish-review serialized an object superseded field as an iterable after metadata/review completed; intake completion failed.",
    "recoveryEvidence": "review-r3-invalidated.json; finish-review-r4/r5 normalize object/array and retain old records.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/bt-recovery/tcp-reset-closure-assertion",
    "count": 1,
    "impact": "BT r2 retrieved all data in five rounds but the new test rejected valid ECONNRESET termination at the closure assertion.",
    "recoveryEvidence": "bt-fix-r2/fixed-race.log; ca7374a8; independent review r3; EOF or ECONNRESET only, deadline and one-second closure bound retained.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/bt-recovery/fixture-writer-backstop",
    "count": 2,
    "impact": "BT r3 and instrumented r4 each had one timeout. r4 proves two dials/one announce/3112960 verified bytes; startup recovery occurred, later transfer stopped. Specific later stall cause unconfirmed.",
    "recoveryEvidence": "bt-fix-r3/fixed-race.log; bt-diagnostic-r4/run.log; 50d60ff5 isolates only the new fixture using pinned TestingConfig keepalive; independent r5 review and same-fixture baseline/fixed-r5 raw gates.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/bt-diagnostic/input-upload-path",
    "count": 1,
    "impact": "Diagnostic r4 preparation wrote overlay inputs at its root but attempted uploading from input/; no test started in that failed preparation.",
    "recoveryEvidence": "native e18d5d; recovered exact existing files to private input/ before the one diagnostic run b189f0/415db0.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/files-surface/clipped-toolbar-click",
    "count": 1,
    "impact": "UI r1 attempted a setup click at a clipped toolbar corner; pointer interception correctly failed the required job.",
    "recoveryEvidence": "ui-r1-diagnosis.json; actual resting-style setup uses blur and pointer movement, unchanged selection assertions.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/docker-terminal/locale-accessible-name",
    "count": 1,
    "impact": "UI r2 passed Chinese 100%/200%, then used a Chinese accessible name in English and failed before opening the container console.",
    "recoveryEvidence": "ui-r2-diagnosis.json; final job uses exact existing en-US/zh-TW/zh-CN catalog names, no forced clicks.",
    "position": "before-production-write",
    "permanentAction": "Release/browser/BT test maintainer; review 2026-10-17 or before next L3 production write. Keep original failures and exact source identity; external harness recovery is not a shared-entry permanent fix. Test delta 50d60ff5 retains complete assertions; underlying original CI cause remains unconfirmed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/status-telemetry/nested-string-escape",
    "position": "before-production-write",
    "count": 1,
    "impact": "Optional compact L3 status command was rejected by Node before SSH due to a nested string escape. Canonical L3 kept running and its gate/evidence were unaffected.",
    "recoveryEvidence": "Native command failure 7ad8bf; status-compact.cjs uses a saved syntax-qualified controller and SSH stdin. No canonical gate rerun.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Keep telemetry in saved controllers with structured arguments; this local correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-notes/runtime-projection/full-body-expectation",
    "position": "before-production-write",
    "count": 1,
    "impact": "Required public browser r1 compared all 12 summary/4 upgrade body items with the unchanged 8 summary/3 upgrade runtime display and failed at current preview API before user journeys. Public Release and image E2E passed; no production writes.",
    "recoveryEvidence": "public-browser-r1-diagnosis.json preserves and hashes all original raw files and cleanup. r2 expectations derive the existing source limits, retain complete body counts, and require exact API/dialog text, all existing scenarios and actual downloads. Actual result is qualified separately after execution.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Derive display expectations from the authoritative bounded parser contract and retain source/body provenance in the shared entry; this external harness correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/download-observer/unhandled-event-wait",
    "position": "before-production-write",
    "count": 1,
    "impact": "Public journey r2 passed bounded release APIs, download API checks and UI single-file save, then an unhandled directory download wait rejected before the click failure could be recorded. Underlying UI/event timeout cause was not identifiable from that attempt.",
    "recoveryEvidence": "public-browser-r2-diagnosis.json hashes all original files, API responses, actual mixed ZIP and saved hello bytes; cleanup/resources passed. r3 coordinates click/event using Promise.all and records exact menu/response/page/trace evidence. Required directory ZIP remains mandatory.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before next L3 production write. Attach handlers immediately and preserve failed UI observation before process exit in the shared entry. This external instrumentation is recovery only; no unproven product fix claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/download-menu/locale-accessible-name",
    "position": "before-production-write",
    "count": 1,
    "impact": "Required public journey r3 exposed that the default current browser fixture was en-US while the directory menu locator expected Chinese 下载 ZIP. The actual menu contained Download ZIP, so no directory download click was made. This also identifies the previously masked r2 timeout cause.",
    "recoveryEvidence": "public-browser-r3-diagnosis.json preserves raw menu/body/trace, root-inspected original PNG, exact API and cleanup/resource evidence. r4 explicitly selects zh-CN and asserts html language before the unchanged Chinese menu/download/ZIP checks.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before next L3 production write. Every fixture must declare locale and match authoritative accessible names. This task harness correction does not claim a shared-entry permanent fix.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/archive-content/single-directory-root-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "Public browser r4 completed all 8 scenarios and actual UI downloads, then Python post-validation expected logs/nested.txt in a single-directory archive. Existing ExportZIP deliberately strips that selected directory root; actual nested.txt bytes were correct.",
    "recoveryEvidence": "public-browser-r4-diagnosis.json hashes original raw files, failed terminal, passed browser sub-result and actual ZIP. public-browser-r4-zip-inspection.json verifies exact nested.txt entry/23 bytes. r5 follows transfer.go and retains exact bytes/safe path and mixed-selection checks.",
    "permanentAction": "Browser acceptance maintainer; review 2026-10-17 or before next L3 production write. Derive fixture path expectations from ExportZIP single/multiple selection contract in the shared entry. External correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/browser-recovery/read-before-scp-completion",
    "position": "before-production-write",
    "count": 1,
    "impact": "r4 failure inspection attempted to read failure.json while its SCP native session 55443 was still running, so the required local read returned missing file. The SCP itself completed successfully; no raw evidence was modified or lost.",
    "recoveryEvidence": "Native partial read 07e8ab; SCP completion 08ab74 exit0; authoritative completed read 061e0c. Full raw hash verification is recorded in public-browser-r4-diagnosis.json; subsequent dependent reads wait for transport completion.",
    "permanentAction": "Release evidence maintainer; review 2026-10-17 or before next L3 production write. Require successful terminal transport result before dependent local reads in the shared recovery entry; this execution correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "documentation/release-metrics/preview-field-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "Required docs L0 r1 rejected custom 不适用（preview） timestamp/duration values and candidate-CI recovery details in a production change-failure field while the change-failure flag was 否. Product publication and public acceptance were unaffected; no production writes.",
    "recoveryEvidence": "docs-l0-result.json and docs-l0.log retain exact original docs commit and all three errors. docs metrics r2 uses existing accepted preview markers 未验证/不适用, keeps candidate quality event times separately outside production metrics, and retains original failed docs commit as ancestor.",
    "permanentAction": "Release acceptance maintainer; review 2026-10-17 or before next L3 production write. Generate metrics using the authoritative allowed marker set and separate candidate quality events from production change-failure metrics; task correction is recovery only.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

文档 L0 r1 的指标格式拒绝及其修正作为一次流程异常保留，原始 docs commit/log/receipt 不覆盖；生产指标采用既有模板允许的精确标记，候选质量事件单独记录。原产品 CI 失败单独保留为质量证据，不自动计为流程异常。公开浏览器 r1 因全文12/4与既有8/3展示限额混淆失败，原始哈希/清理保留；r2修正投影后在目录下载观察器出现未捕获等待异常；r3改为同时等待点击/事件并保存菜单、响应和trace，确认实际英文Download ZIP与中文locator不符，尚未发起目录下载；r4显式zh-CN后8项浏览器/界面下载全部通过，末尾单目录ZIP路径预期错误；r5按既有ExportZIP去掉所选目录外层语义，严格断言nested.txt唯一条目与完整字节，混合选择仍校验logs/nested.txt。恢复取证曾先于SCP完成读取一次，完成后重读并核验全部原始哈希。见public-browser-r1/r2/r3/r4-diagnosis.json与最终公开旅程证据。原流程账本及其一个两段指纹的三段格式映射均保留，计数/事实不变。最近五个正式版本逐文件哈希核对，重复根因与历史关联见 process-history-verified.json；现场恢复不冒充唯一入口永久修复。流程维护者在 2026-10-17 或下次 L3 生产写入前复核。

## 遗留风险、资源回收与后续准入

- CF 补审/needs_validation、GPU +25.485%/20% 和 offers 后台刷新/TempDir 清理竞态仍开放；offers 维护者在 2026-10-15 或稳定前修复/复核，GPU 在 2026-10-16 或稳定前复核。RC3 完整重试不等于永久修复。
- BT 仅本机 v1 btih/种子，私有 Magnet 元数据前 DHT 缺口、无 PEX/v2/hybrid/做种/跨重启恢复/远端 BT/10GiB 全量验收；可能需要2–3份磁盘副本。有界零载荷恢复不处理载荷开始后的停滞。
- 新容器终端 CF 仍待审；同步升级 Agent 与 Linux capabilities，脱离会话后台任务可能继续，停止 Agent 后清理等待下次启动。没有独立守护或公开 Panel-Agent Docker 全旅程结论。
- 这些限制不阻断规则允许的 preview；稳定准入不能沿用本次延期作为豁免。
- 发布前精确回收已释放的 RC4 L3 工作/inbox、BT 诊断独占缓存和本轮旧解包源码，观察可用空间净增 3002867712 字节，见 owned-pre-l3-cleanup-result.json；草稿 parser 缓存另回收见 parser-final-cleanup-result.json。共享 Docker 缓存/镜像/卷未清理。
- 最终 L3 源码工作/inbox/远端重复 bundle 在公开和文档门禁结束后，按精确路径/哈希/进程/挂载保护回收，实际释放见外部 owned-closeout-result.json；仅停止本轮自有发布预览，作者预览保留。原始日志、所有失败、截图、审计、源码归档和本地完整 L3 bundle 留存。

## 外部原始证据

`C:\GitHub\_release-evidence\v1.26.0-rc.5`；L3 `C:/GitHub/_release-evidence/v1.26.0-rc.5-5cd7bcc7-l3-r1`。冻结清单、原始 CI/BT/UI 失败、源码/OCR blob 证明、同夹具基线/10轮 race、最终56原图、公开 API/OCI/附件响应、实际 ZIP 字节、公开浏览器终态/清理和各 CI run 按原件哈希保留。仓库仅提交本验收与当前业务事实，不提交令牌、私有夹具或大原件。
