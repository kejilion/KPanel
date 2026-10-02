# KPanel v1.24.0-rc.9 发布验收记录

日期：2026-10-03；发布级别：L3。

候选提交 / 标签：`2dd0c6398b9bc16292c7db44e526915c2a147111` / `v1.24.0-rc.9`。

`releaseChannel`：`preview`；`releaseTrain`：`1.24.0`。

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。

## 发布画像与范围

基线为 `b7d018f523d82bf9c8e7477959c347aca8695412`。保留来源历史，纳入图库菜单层级修复 `7f718c33781986e018b3da54aa8634eee9303cda`、相册封面与移动 `3b9e31ed56e5c0bc8d2d9d12258896b8daffa09a`、Docker 监控 `fac0f71c7552bb9c791aa7ea5e8fc15f2fb2c387`。

- 图库更多菜单打开时覆盖筛选工具栏，关闭后恢复原层级；菜单项命中、键盘和窄屏交互通过。
- 从照片设置图库/相册封面，使用普通目录隐藏文件 `.kpanel-cover.json`，可恢复自动封面；不存在、非法或视频封面标记按现有自动策略回退。
- 选择或拖动媒体到相册，复用现有文件移动与资源版本 API；同名不覆盖，部分失败保留原文件并明确提示。
- Docker 管理/监控切换，复用真实 Docker stats，采样并发上限 3、容器上限 32；关闭、切主机和离开页面停止旧请求。清单刷新与采样分开。

发布 owner 修复 marker 读取超时实际取消及 owner abort、关闭后旧新建相册请求不得影响重开窗口、非法视频 marker 拒绝。修复提交 `2b5a93f98c58b5aabf9f43b47a7cd9fd37119739`、`3ebdf9128764c704b36f6204e87f08e64277b409`；红/绿回归、85 项定向回归及类型检查保留。

未纳入两条终端输入候选：`8440de317a8d18814304da7d9fc5146cc969bfd1` 的最终 L2 尚未完成，`d0437534f54552b16448fd93f6dcf1a6e0e6598f` 的四类真实 systemd 任务、浏览器/代理等预发布矩阵仍待验收。Node 26、TS7、治理草稿未搭车；没有新增后台 API、数据库、监听端口或 Agent/脚本权限。

## 独立复核、安全与脚本联动

Claude 来源由 Codex 发布 owner 复核。OCR 工具 1.12.11，最终范围 `b7d018f..3ebdf91`，36/36 文件检查；原自由臂先于圈选，3 个问题，后续增量检查另发现非法视频 marker。有效问题合计 H0/M2/L2，已修复；constrained-only 未报告，不推断为零。发布 owner 自己的修复没有第二个 provider 复核，不把自查称为独立盲测。

CF 覆盖 `decision=scoped-required`，59 个未审计提交、110 文件、最老 6 天；last full run-4 距离 11 天，边界包变化来自此前候选的 `internal/backupremote`。本 RC 记录该非阻塞缺口，稳定版前补审；本轮未执行 CF scoped/full，不宣称 CF 审计通过。

`scriptLinkageState`：`not-required`。沿用脚本 commit `c981fb6c8b481981ac7a006e102e111e435f6d30`，SHA-256 `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`；`kpanel.conf` 归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`。受管脚本及 app-conf lifecycle 通过，公开镜像实际脚本/配置/VERSION/图库图标 bytes 核验通过；没有脚本或应用市场仓库写入，应用市场仍 stable/latest。

## 门禁与多维验收

固定入口 `scripts/run-release-l3.mjs`，run `v1.24.0-rc.9-2dd0c639-l3-r1`，准确候选 `2dd0c6398b9bc16292c7db44e526915c2a147111`，passed/exit_code=0；12 项原始证据摘要已逐项核验。Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0；全量 Go、前端 244 文件 2198 测试、typecheck、race、vet、govulncheck、npm audit、Trivy source/image、双架构 Linux 构建及安装/升级/失败恢复契约通过。govulncheck 的可达调用结果为 0，不等于完整依赖/供应链无风险。

候选 CI `37074171735`、Dependency freshness `37074171756`；产品主线 CI `37074861689`、Dependency freshness `37074861764`；Release `37075457624`，均同一准确产品 SHA completed/success。没有新增依赖，package-lock 只更新根版本；既有 TypeScript/@types-node/Trivy qualification 期限不重置。

| 维度 | 已验证证据 | 限制 |
| --- | --- | --- |
| 业务正确性/互通 | 真 Panel/Agent API 21 项，封面 bytes、移动部分成功/冲突/stale、普通 Shell 读取一致；真实浏览器 6 项 | 使用隔离目录，没有生产数据或多远端主机全旅程 |
| 安全/供应链 | 精确 L3、固定脚本、公开双架构 OCI 元数据及 attestation subject binding | CF 稳定前补审，未完整审计 SBOM 内容 |
| 稳定/恢复/兼容 | marker abort/超时、重开窗口防竞态、Agent 停止/重启后恢复、监控退出停止采样 | 没有长期故障/soak，未验证所有设备和媒体编码 |
| 性能/资源 | 采样 3 并发/32 上限回归、有限真实采样，受限浏览器与 256MiB Panel/Agent 夹具 | 没有大图库或生产长期性能数据 |
| UX/可访问性 | 菜单 10 个 Mock 组合、功能 8 个 Mock 场景、真实浏览器与截图；390/768/1280/1700/1920、浅深色、键盘/语言 | CSS 缩放是模拟，原生缩放/全部设备未验证 |
| 数据/配置 | 普通文件、现有资源版本；封面存储/恢复与同名不覆盖验证 | 新隐藏 marker 是可选偏好，无排他索引或数据库迁移 |

Mock 使用 Chrome 154.0.8037.93 / Playwright 1.62.1；pageErrors/unexpectedErrors 为 0，已知 Mock 404 与故意注入 503 独立记录。真实浏览器使用 Chromium 140.0.7339.16 / Playwright 1.55.0，背景权威作业 `arena-154-35520` passed/exitCode=0，规格摘要 `c97874d7d4b5bc17f5d5d1196705e0df1837f05a40fdcc8e8f6830b2b9bdfcc2`；真实页面错误及未预期控制台错误为 0；移动旧版本缩略图 409 和四个既有业务容器私有镜像查询的 Docker API 403（Panel 502）按准确路径、容器 ID、错误码与响应单列。最终新缩略图返回 200，有效 PNG 在移动后实际解码；本轮不声称 H.264 播放或私有仓库鉴权通过。

真实 API 使用最终 L3 不可变本地 index `sha256:6ba51c57d7301a6c05a4c99ab79775ecc54c05ddb93a0caf3f5a6a1d4a06f25b`。首轮 20 项通过后，隔离 PID namespace 导致 Docker daemon guard 拒绝 stats；保留原失败，r2 以 host PID 可见性和只读 socket/PID 文件验证已运行 dockerd，activation guard 保持默认 false，system mutations 禁用；21 项通过。仅任务自有 Panel 容器被采样，业务容器清单只读。

`arena-154` 18 个业务容器的 ID、StartedAt、状态和健康与本轮基线逐项一致。任务独立容器/网络、L3 临时源码/inbox、成功及失败夹具和最终 verify 镜像已精确清理并核验不存在；保留原始证据、固定 Runner、复用浏览器工具及公开镜像缓存，没有全局 prune。证据根 `C:/GitHub/_release-evidence/v1.24.0-rc.9`，L3 原件 `C:/GitHub/_validation/kpanel-v124-rc9-l3-r1`。

## 公开产物与通道

[GitHub prerelease](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.9) 于 `2026-10-02T23:13:10Z` 公开，draft=false/prerelease=true，14 附件。SHA256SUMS、metadata archive 和 amd64 Node 实际下载 bytes 已核验；11 条 SUM 与 GitHub asset digest 一致，10 metadata 文件与准确 Git blob 一致；其他二进制未全部独立执行，本轮未重复原生 procd 测试。

版本与 preview index `sha256:11ee0a1c6691d36cf2523d30304ae8fedbd9a9ec657f21a1e5d8bcac372d384f`；amd64 `sha256:425ba0cac894f98dede438e22cc7c9418036477edd5d686a03384a39b2fe3f7c`；arm64 `sha256:95641839c5347c331b8aad2a99818264fcde12805f3fadda31ee529fa0978236`。双架构 manifest/config 原始 byte 摘要、version/revision/script labels、65532:65532、entrypoint 与 SPDX/SLSA subject binding 通过。公开 immutable amd64 镜像 `packaging/tests/image-e2e.sh` 和独立内置 bytes 核验通过；临时 E2E 容器/网络/数据已移除。

GitHub Latest 仍 `v1.23.0`，Docker latest 仍 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。只提升 preview；加入预览不会自动安装，退出不会自动降级，轻量 Node 无人值守仍只跟 stable/latest。生产部署不适用，生产写入 0，`prod-108`/108 本轮未连接或执行任何 KPanel 操作。

## 回滚与分支处置

上一 preview `v1.24.0-rc.8` / `e0684ae924bcc093397af0773297c52dc8713ea7` / `sha256:b6a88574f55143e18412ffa0586d26e0ebfbe44ad9d96290063376a233001c78` 保留；稳定 `v1.23.0` 保留。普通目录文件仍由旧文件管理和 Shell 读取，新隐藏 marker 可忽略。共享 main 使用聚焦 revert 并重验，不改写历史；设备回滚需先备份，通道开关不代替安装回滚。

候选 `release/v1.24.0-candidate` 保留，唯一候选工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly`；本记录合并后 main/候选本地与远端以同一最终文档 tip 对齐，并重新提供该 tip 的 acceptance Mock 预览，实际终态见仓库外 `final-alignment.json`/`preview-published/manifest.json`。

纳入来源均已被产品 tag 包含。来源工作树 `claude/gallery-cover-move`、`claude/docker-monitor-mode` 及旧 `claude/media-gallery` 未确认释放所有权，保留其本地树/分支/原预览；不得因归档快照删除。发布 owner 自有菜单/生命周期修复分支按准确 SHA 归档后回收干净工作树；本验收分支在候选/main 同 SHA CI 成功后归档 `archive/docs/release-v1.24.0-rc.9-acceptance` 并回收。实际归档、远端身份及清理收据保留于 `branch-disposition.json`、`acceptance-archive.json`，不制造第二个产品版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T22:09:45+08:00
- 候选冻结时间：2026-10-03T06:25:55.704+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：15
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/tag-probe/python-inline-newline",
    "count": 1,
    "impact": "Inline Python newline quoting rejected the first mandatory public preflight; no external write.",
    "recoveryEvidence": "public-preflight.py and public-preflight.json passed",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "regression/gallery-marker/vitest-cwd",
    "count": 1,
    "impact": "Vitest ran from the wrong cwd, failed the import alias and executed zero tests.",
    "recoveryEvidence": "marker-video-regression-r2.log: 12 passed from web cwd",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/menu-harness/english-selector",
    "count": 1,
    "impact": "Chinese-only refresh selector invalidated the English case of the first menu browser run.",
    "recoveryEvidence": "menu-browser-r2/result.json: 10 passed using locale-aware refresh selector; original run retained",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/feature-harness/state-fixture-selection",
    "count": 5,
    "impact": "Five feature harness attempts failed on nonexistent selectors, selection state, toast timing or invalid image fixture; none is used as a passing product result.",
    "recoveryEvidence": "feature-browser-r1/result.json; feature-browser-r2/result.json; feature-browser-r3/result.json; feature-browser-r4/result.json; feature-browser-r5/result.json; feature-browser-r6/result.json: 8 passed with valid PNG, explicit selection completion and decoded-media wait",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-validator/python-inline-quoting",
    "count": 1,
    "impact": "PowerShell quoting made the first mandatory public validator preparation invalid; no publication occurred.",
    "recoveryEvidence": "prepare-public-validator.py syntax checked; publication-helper-hashes.json refreshed; file-based helper used",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/command/guessed-resource-path",
    "count": 1,
    "impact": "The first mandatory incident consolidation referenced a nonexistent feature-browser result path and stopped; no metrics were published.",
    "recoveryEvidence": "rg --files located feature-browser-r1/result.json; consolidated ledger checks every actual result and total",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "real-api/fixture/docker-daemon-visibility",
    "count": 1,
    "impact": "First real API fixture passed 20 checks but failed Docker stats because its isolated PID namespace could not verify the live dockerd PID; existing activation guard refused connection.",
    "recoveryEvidence": "real-api-raw/result.json failed retained; real-api-raw-r2/result.json: 21 passed with read-only daemon PID/socket and host process visibility; system mutations disabled, default activation guard unchanged",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/real-harness/response-error-classification",
    "count": 2,
    "impact": "Two real browser runs finished all six functional cases but were rejected by the final console-error gate: transient stale-thumbnail conflict plus existing business private-registry authorization failures had not been classified with exact response evidence.",
    "recoveryEvidence": "real-browser-r1/result.json and real-browser-r2/result.json retained; final real-browser/result.json records fresh thumbnail 200 and decoded image, exact known business IDs/403 registry-auth response bodies; all other errors still fail",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "docs/metadata/merged-stderr-warning",
    "count": 1,
    "impact": "All first acceptance validators passed, but the exact-path guard consumed a Git CRLF warning merged into stdout and refused the commit. Only the two allowed docs were changed.",
    "recoveryEvidence": "acceptance-checks.log validators passed; native git status confirmed the exact two paths; acceptance-checks-r2.log separates stderr from metadata stdout and writes repository docs with LF",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "docs/record-writer/non-idempotent-version-anchor",
    "count": 1,
    "impact": "Second acceptance preparation stopped because the current-context writer searched only for the old RC8 paragraph after the first attempt had already written RC9; no commit/push occurred.",
    "recoveryEvidence": "acceptance-checks-r2.log failed retained; write-acceptance.py now finds the preview paragraph independently of RC number; acceptance-checks-r3.log validates the resumed exact two-file scope",
    "permanentAction": "负责人 KPanel 发布维护者；2026-10-04 复核；下一次正式生产写入前，把已验证的文件式脚本、真实媒体夹具和页面状态定位纳入稳定入口并补回归。本次保留失败原件及核对后的重试，不宣称仓库永久入口已修复。",
    "position": "before-production-write",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

以上按模板统计导致必需步骤无效/失败/重试的事件；普通只读诊断搜索不计，正常产品红回归另存质量证据。失败原件均保留，重试只使用实际通过的结果；本轮没有生产操作，流程异常不冒充产品生产变更失败。
