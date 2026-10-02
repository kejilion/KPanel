# KPanel v1.24.0-rc.7 发布验收记录

日期：2026-10-02

发布级别：L3

候选提交 / 标签：`3015c1fb2ba6918d445322d234f6543ddb0ac978` / `v1.24.0-rc.7`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` 保留，唯一产品工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly`，本地/远端精确 tip 为 `3015c1fb2ba6918d445322d234f6543ddb0ac978`。

## 发布画像

- 业务域：桌面资源状态；轻量 Node 原生 procd 生命周期、健康、持久状态和登录事件。
- 变更面：只读摘要和特权 Node 服务监管/安装模板。Panel/Agent API、端口、Compose、身份协议和数据库格式没有变更；保持单管理员及业务原生真源。
- 旅程：已对齐网站尚未获得健康探测、真实漂移和接口失败；procd 固定四服务、cron、失败恢复、SSH 登录与 overlay 容量。风险 L3，按 `profile.json` 完成自动门禁、浏览器及隔离 fixture 验证。
- 新 Node 特性进入 RC 附件；无人值守下载仍只选择稳定 Release，不以切换 Panel preview 隐式升级路由器。

## 发布范围与未纳入内容

- 批准主线基线 `bd6fe05efba5bc0aac7f2999375b7fec37115e9f`。桌面来源 `fix/desktop-site-status` / `10ba3d76d2fa242b6454e04d60767160b2a69323`；Node 来源 `feature/light-node-procd` / `7d084570bda3a9f7af81841ea15485f3ee3e59c3`；配套脚本 `feature/light-node-procd` / `c981fb6c8b481981ac7a006e102e111e435f6d30`。
- 集成 `a27e57e9`、`9eec6bc3`；测试夹具修复 `e36e3872`；版本/脚本来源 `f41ed00d`；最终 `3015c1fb2ba6918d445322d234f6543ddb0ac978` 只记录复核 trailer，tree 与 reviewedHead `f41ed00d991a1c9587b8c0fde8ade2eeab5a1fc7` 相同。完整精确提交清单和 delta 在 `review-diff.patch`、`freeze.json`。
- 未纳入：相册工作树含未提交改动；TS7 基准已按用户原话归档；Node 26 Current、治理草稿与旧治理 fallback 保留。没有接管、重置或清理这些来源，也没有生产部署。

## 外部审计与修复交付

- CF 覆盖 `decision=scoped-required`；59 个未审计提交、110 个文件、最老 6 天，last full run-4 source `4c0694aa8e02` 距离 11 天；新增边界包 `internal/backupremote`。RC 只记录，稳定发布前补审，本轮未执行或宣称 scoped/full 审计通过。
- 来源独立复核 PASS，原 P2 cron 移除状态问题由 `9a0abb10` 关闭；发布 owner 与来源主要实现不同，原始独立复核和失败证据保留于 `C:/GitHub/_validation/light-node-procd-20261002/review`。Claude CLI 未登录，来源披露采用独立干净 Codex 会话 fallback；不称跨 provider 复核。
- OCR 1.12.11：精确 `bd6fe05efba5bc0aac7f2999375b7fec37115e9f..f41ed00d991a1c9587b8c0fde8ade2eeab5a1fc7`，25/25 文件 reviewed；自由臂先落盘，无新阻断。6 个排除文件也人工覆盖，procd shell 模板另外验证字节与原生语法/运行。来源结果已知，`blind=false`、`constrained-only=unreported`，不计有效盲测周期；没有修改圈选规则。
- 日期敏感审计测试原失败保留；只用既有 fixture helper 固定 merge day=2，16/16 回归及最终 L3 通过，生产审计阈值不变。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集 `light-node-procd-20261002`。
- 旧 KPanel 内置脚本 `8bebc2d80614e96b844c2c5f88acb0a81d4abd10` / SHA-256 `578b9e4328ba231ad08783c3f2007034f332621d48db07cffe3429da807940b4`。脚本集成基线 `cc5d2fe54377bc8b7fe1b0806f3e2efe37031219` 的根脚本字节与它相同，两者不是同一 commit。
- 脚本候选 `c981fb6c8b481981ac7a006e102e111e435f6d30` / 根 SHA-256 `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`；CN SHA-256 `5be9c4462664ed12833a9dbeba64c4623f7f00deb3ddd5adb8e1539615ff617a`。
- 已先发布脚本 main，公开根/CN 实际字节与候选 Git blob 相同，再合入 KPanel；Dockerfile labels/ADD 和 Node 七个权威更新模板固定同一来源。阻断或移除的依赖范围：不适用。
- 发布 owner 重新执行 31 项更新器回归和 installer smoke，全部通过，旧 systemd/OpenRC 路径保留。源 OpenWrt native fixture 与最终受影响目录逐字相同，新候选 Node 二进制另行重跑；旧稳定二进制 version/health 和现有 generation-5 updater 保留验证通过。
- `kejilion/apps` 本地/公开/打包 `kpanel.conf` 一致，归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；apps 无写入，默认 stable/latest。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 全量 Go、前端、脚本回归、原生 fixture 与公开镜像 E2E | 未执行路由器联网接入与真实中心全旅程 |
| 网络入侵与供应链安全 | 已验证 | L3 安全扫描、固定脚本 byte contract、双架构 OCI/attestation | CF 补审待完成，不称全依赖无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | 更新器失败/回滚、cron 保留、原生监管 fixture、旧稳定命令/模板保留 | 四服务 source lifecycle 使用模拟 Node，无全设备重启 |
| 性能与资源预算 | 已验证 | 命令时间/输出上限、expiring relay、固定隔离容器 CPU/内存/PID | 无生产性能或长时间 soak 数据 |
| 用户体验与可访问性 | 已验证 | 精确候选 1440x1000 浏览器三状态、Tab 焦点和 Enter 网站入口 | 无布局/字号/主题/断点变化，缩放/多视口/语言矩阵不适用 |
| 数据、配置与迁移 | 已验证 | protected persistent state、relay、cron 与锁/恢复夹具 | 未执行用户设备数据迁移 |

## 自动门禁

- 固定入口 `scripts/run-release-l3.mjs`，run `v1.24.0-rc.7-3015c1fb-l3-r1`；runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，准确候选 `3015c1fb2ba6918d445322d234f6543ddb0ac978`，终态 passed/exit_code=0，12 项原件 SHA-256 全部核验。kit manifest、脚本、plan、bundle 摘要见 `C:/GitHub/_validation/kpanel-v124-rc7-l3-r1/manifest.json`。
- `make verify-release`：治理 233/233、完整 Go/test/vet/race、前端类型/227 个文件共 2057 项测试/构建、三场景源码复现、deploy/安装安全、十个 Linux 发行二进制、源码和镜像安全门禁、镜像/脚本/生命周期检查均通过，原始完整日志保留。
- 候选 CI `36983771379`；产品主线 CI `36985299318`；Release workflow `36986288811` 均为精确候选 completed/success。新鲜度 CI 结果见同 SHA Actions 记录，不把单次历史报告冒充本轮报告。

## 依赖与技术栈变化

- Go 1.27.1、Node 24.21.0 LTS、TypeScript 6.0.3、Action、基础镜像和扫描器 pin 保持 rc.6；只更新受管脚本 c981 来源与发行版本。package-lock 仅根版本两处变化。
- 原 TypeScript/@types-node 有期限例外由 KPanel base maintenance 在 2026-10-15 复核；Trivy 0.75 qualification 保持首次检测 `2026-10-01T15:47:46.958Z`，最晚 10-15 启动、10-31 决策、11-30 处置，不重置日期、不搭车变更。本版没有新采用/拒绝/暂缓决策。
- 最近每日安全/EOL 审计复用既有证据，本轮未重新执行每日全审计。L3/CI 的当前扫描单独记录，不等于全部软件支持周期或 CF 审计完成。

## 隔离真机与浏览器验收

- 环境 `arena-154`，purpose candidate-validation，固定 Linux Runner；现存 Panel 仅健康读取，无生产升级。OpenWrt 24.10.3 x86_64 rootfs SHA-256 `7567e55ec5b6b834b2a0f27215bd6679c86fd3505c8a48398216626b586e3164`，image `sha256:4315b6b8f94b116ae4fef99f2171354f6f8e6696de201c5523e30e834564bb3e`。
- 原生 `procd-native-rc7-3015c1fb-r1` passed；新 Node SHA-256 `5a744529b511baea2c4660fdf593c2806c512928e0061ed528a4630557e5948b`；network none、无宿主 bind、cap-drop ALL，仅 CHOWN/KILL/SETUID/SETGID，192MiB/0.5CPU/96PID。版本、健康命令、真实 SSH broker/logread 事件和停止成功；旧稳定 Node 官方附件摘要验证、命令及 updater 保留成功，旧版本健康输出全为 unknown，未把它写成支持新 procd 健康检查。自有容器已清理。
- 公开 Node 附件 SHA-256 `9f830a89c07fdd4aa551135bd40e89c423606e2276819b3d37c57fbae6c613f5` 与打标签前 L3 构建不同，首轮比对失败完整保留。两者 Go 1.27.1、依赖、构建选项、VCS 提交/时间一致，模块版本与 vcs.modified 标记不同；Release 会在源码树生成未跟踪 release/ 文件。固定 Runner 从同一 P7 重建标签和生成文件状态后逐字复现公开附件，见 node-public-reproduction.json；公开原件又在 `procd-native-rc7-public-3015c1fb-r1` 独立重跑并通过上述有限原生验证。未改写标签、版本或产品代码，不把打标签前的摘要冒充公开原件摘要。
- 模拟数据 acceptance 预览与浏览器绑定精确候选，1440x1000/dark、three status cases、keyboard focus 与网站窗口路径；pageErrors 与 unexpectedConsoleErrors 均空。已知 mock 404 与故意注入 503 分开记录。没有把 mock 当真实 API/主机验收。
- 未执行：iStoreOS/ImmortalWrt、arm64 原生设备、网络下载/真实 enrollment、真实用户 SSH 登录、全路由启动/路由/重启和长期 soak。公开 E2E 为 linux/amd64，回环 18087，固定仓库脚本，有限隔离生命周期，不替代这些边界。

## 发布产物与公开仓库复核

- GitHub prerelease `https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.7` 于 `2026-10-02T09:02:55Z` 公开，draft=false/prerelease=true；GitHub Latest 仍 v1.23.0。
- 版本/preview index `sha256:e8fd978f1286826950b062324523037a728021f580eafc05c880690a7e4cfa74`；linux/amd64 `sha256:552327740cfc7b599718678cc5d2cc6cbf85a47f2b62200d635ec3772f17709f`，linux/arm64 `sha256:a750381a9eb5f523a7a8e529aa0d548917471163e3c905975e61ffc0f884f803`；latest 仍 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。
- 14 附件；SHA256SUMS、metadata archive 与 amd64 Node 实际 bytes 校验，11 条 SUM 等于 GitHub asset digest，10 metadata 文件等于精确 Git blob；公开 amd64 Node 与本轮原生测试二进制逐字相同。其余独立二进制未全部执行。
- 双架构实际 manifest/config byte SHA、版本/revision、USER65532/entrypoint、script labels 和 SPDX/SLSA subject 绑定通过；不是完整 SBOM 内容审计。
- 公开 immutable 镜像真实 `image_e2e=pass`，启动/版本/资源/bootstrap/Host-Origin/Secure Cookie/健康/清理通过；镜像内脚本、kpanel.conf、VERSION 实际字节通过，E2E 与从未启动的 bytecheck 容器/临时数据均删除。

## 自更新通道验收

- stable/preview 选择、合法版本、唯一官方 digest、切换清候选、退出不自动降级、开关/立即安装互不越权，由完整现有回归验证。
- 本版只提升 preview；标准 apps 安装仍 latest/stable。OpenRC 完整 KPanel 自动更新边界保留，轻量 Node 无人值守只跟 stable，新 procd 能力需要对应 Node 版本，不宣称设备自动收到 RC。

## 生产部署安全核对

- 不适用（预览版禁止生产部署）。产物已发布，生产未部署；本轮生产写入 0，没有生产备份、升级、重启、回滚或数据修改。隔离容器内安装路径不等于业务宿主安装。
- prod-108/108 禁用全部 KPanel 操作，本轮未连接、备份、部署、升级或核对。

## 回滚

- 前一 preview v1.24.0-rc.6 / 0a16722239495e5e37399decf31c6579d998bff7 / sha256:4deadfed891d113d8ec3886a6a0b4ae15a191a226b152c840dd10d40c009ac62 保留。
- 成对源码回滚脚本基线 cc5d2fe54377bc8b7fe1b0806f3e2efe37031219，KPanel 上一 RC。共享 main 只做聚焦 revert 并重验，不改写历史；稳定默认未改变，无生产回滚需求。未来设备回滚必须核对 /etc 持久状态、旧 /var 状态和备份，不能以通道开关代替。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-02T10:55:41+08:00
- 候选冻结时间：2026-10-02T16:01:45.059+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：19
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/audit-fixture/wall-clock-merge-date",
    "position": "before-production-write",
    "count": 1,
    "impact": "Existing date-sensitive fixture blocks the current release date; original failure preserved.",
    "recoveryEvidence": "date-fixture-before.log, date-fixture-after.log: 16/16 PASS",
    "permanentAction": "e36e38720eebca85149423f8734409314d86ef75 uses existing deterministic fixture commit helper; production age thresholds unchanged.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-remote/https-connect-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "HTTPS script ref read failed before mutation.",
    "recoveryEvidence": "Explicit SSH URL read verifies cc5d2fe baseline; script-push.log and script-publication.json bind c981fb6.",
    "permanentAction": "Use explicit SSH URL and the configured repository key for script source reads and writes.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-remote/wrong-repository-cwd",
    "position": "before-production-write",
    "count": 1,
    "impact": "Temporary remote override from Panel checkout returned Panel main; result rejected as script evidence.",
    "recoveryEvidence": "Correct sh worktree and explicit repository URL returned cc5d2fe; public root/CN byte verification passed.",
    "permanentAction": "Bind worktree and explicit SSH repository URL in the publication helper; do not infer repository identity from a ref name.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/source-hash/child-output-buffer",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial hash helper default child output limit rejected the large script blob.",
    "recoveryEvidence": "prepare.mjs uses 8 MB output limit; all three actual Git blob hashes recorded in contract.json.",
    "permanentAction": "Bound subprocess output according to the inspected script size and check status before hashing.",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication/script-bytes/node-fetch-reset",
    "position": "before-production-write",
    "count": 1,
    "impact": "Node direct fetch reset while reading published script bytes; assembly stopped before the dependent merge.",
    "recoveryEvidence": "assemble.mjs uses bounded Python urllib; script-publication.json proves both public files equal exact candidate blobs.",
    "permanentAction": "Use the existing working proxy-aware download transport with fixed URL, timeout, status and byte verification.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/ocr-rule/invalid-helper-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial inline JavaScript syntax prevented rule extraction; no coverage result accepted.",
    "recoveryEvidence": "constrained.mjs and ocr-rules.json; coverage 25/25 bound to f41ed00d.",
    "permanentAction": "Run saved small helper with checked subprocess status; inspect rule output before writing coverage.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/artifact-read/guessed-evidence-path",
    "position": "before-production-write",
    "count": 4,
    "impact": "Convenience reads guessed absent helper/manifest/business-facts paths; no absent output used as release evidence.",
    "recoveryEvidence": "File inventory and exact baseline diff locate actual helpers, completed kit manifest and product-quality-review-current.md. A later guessed .codex workflow directory was absent; rg --files locates the actual .codex-workflows/release-kpanel.workflow.yaml. Native runtime templates were located by rg --files at cmd/kejilion-node/update_runtime after a guessed internal directory was absent.",
    "permanentAction": "Locate existing paths with rg/files or exact Git delta; wait for preparation completion before reading its manifest.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/thread-inventory/response-format-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Expanded thread-list parse received a non-JSON response; response was not used to classify source ownership.",
    "recoveryEvidence": "Exact worktrees, source commits, source delivery and dirty gallery state independently establish selected scope in contract.json.",
    "permanentAction": "Check structured response/error before parsing; candidate identity and delivery remain grounded in Git and saved source evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/l3-evidence/missing-ssh-collection",
    "position": "before-production-write",
    "count": 1,
    "impact": "Local evidence verifier rejected absent SSH evidence before candidate push; the fixed SSH producer had already passed remotely.",
    "recoveryEvidence": "Complete immutable remote evidence copied to kpanel-v124-rc7-l3-r1/remote-evidence; harvest-l3.py verifies status, candidate, Runner and all 12 hashes.",
    "permanentAction": "Explicitly collect SSH producer evidence after terminal completion, then verify checksums before candidate publication; keep fixed L3 entry unchanged.",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication/ci-receipt/windows-text-encoding",
    "position": "before-production-write",
    "count": 1,
    "impact": "Main promotion helper rejected reading the authenticated UTF-8 CI receipt with the Windows default GBK codec, before any main push.",
    "recoveryEvidence": "promote-main.py explicitly reads UTF-8 and all Python release invocations use -X utf8; product-main-push.log binds the expected fast-forward.",
    "permanentAction": "Specify UTF-8 for structured tool receipts and the Python execution profile rather than relying on locale defaults.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-helper/repeated-generator",
    "position": "before-production-write",
    "count": 1,
    "impact": "Re-running the one-time helper generator reverted the newly added public Node byte check and the fresh native receipt link; no generated result was accepted or product changed.",
    "recoveryEvidence": "The byte check was restored and hashes refreshed; freshBinaryRerun restored from the retained passed native-candidate.json with exact candidate binding.",
    "permanentAction": "One-time generator now refuses existing helpers; refresh-public-manifest.py refreshes only hashes and checks the retained native receipt.",
    "historicalReleases": []
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "publication/node-bytecheck/git-build-metadata",
    "count": 2,
    "impact": "Initial public byte equality check and subsequent clean-VCS assumption rejected the official Node before acceptance; the public asset is stamped with the release tag and dirty generated-artifact state, unlike pre-tag L3.",
    "recoveryEvidence": "public-validation.log; public-node-bytes-first-mismatch.json; node-build-info-*.txt; official attachment OpenWrt rerun passed in native-public-candidate.json. Exact source reproduction is recorded separately before closeout.",
    "permanentAction": "Compare and retain build metadata, execute the actual downloaded public attachment, and reproduce it from an independent same-SHA checkout with the correct tag/generated-artifact state."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "diagnostic/runtime/go-not-on-path",
    "count": 1,
    "impact": "Windows Go build-info convenience command was unavailable; no result accepted.",
    "recoveryEvidence": "Fixed Linux Go 1.27.1 Runner successfully read both actual binaries; node-build-info-l3.txt and node-build-info-public.txt.",
    "permanentAction": "Use the frozen Linux Runner for Go artifact introspection rather than assuming local PATH has Go."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "diagnostic/node-repro/read-only-module-cache",
    "count": 1,
    "impact": "First additional reproduction container could not create the default module cache on its read-only root; no source reproduction result accepted.",
    "recoveryEvidence": "Retained /root/kpanel-release-evidence/v1.24.0-rc.7-node-public-repro/build.log. Subsequent diagnostic has task-owned writable checksum-verified Go caches.",
    "permanentAction": "Give standalone build diagnostics explicit task-owned writable caches while preserving a read-only container root."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "diagnostic/node-repro/shared-git-objects",
    "count": 1,
    "impact": "Second reproduction attempt used a shared clone whose alternate object path was outside its container mount; Go could not obtain VCS status.",
    "recoveryEvidence": "Retained node-public-repro-r2/build.log and Git alternate-object failure. Final reproduction uses --no-hardlinks independent Git objects.",
    "permanentAction": "Use an independent clone for container diagnostics, preserving commit metadata and making the entire Git object database visible without mounting the original worktree."
  }
]
<!-- kpanel-release-process-incidents:end -->

## 候选、来源与资源收尾

- 产品候选保留；两项 Panel 来源 tip 与脚本来源已保存精确 archive 引用，Git 可恢复；原 clean 来源工作树与作者 mock 预览/历史证据保留，不接管原预览、不视为待集成候选，原始 ref 和归档证明见 source-archive.json。

| 仓库 / 来源 | 精确 tip | 归档 ref / SHA | 处置与责任 |
| --- | --- | --- | --- |
| KPanel / fix/desktop-site-status | `10ba3d76d2fa242b6454e04d60767160b2a69323` | `archive/fix/desktop-site-status` / 同 tip | 已纳入 rc.7；直接保存归档，远端活跃 ref 原本不存在；本轮发布 owner 核验，来源工作树保留 |
| KPanel / feature/light-node-procd | `7d084570bda3a9f7af81841ea15485f3ee3e59c3` | `archive/feature/light-node-procd` / 同 tip | 已纳入 rc.7；直接保存归档，远端活跃 ref 原本不存在；本轮发布 owner 核验，来源工作树保留 |
| sh / feature/light-node-procd | `c981fb6c8b481981ac7a006e102e111e435f6d30` | `archive/feature/light-node-procd` / 同 tip | 脚本 main 已发布；原子保存归档并移除同 SHA 活跃 ref；本轮发布 owner 核验，来源工作树保留 |

- 自有 L3 work/inbox、当前 native/script 可再生源目录已按准确路径、证据摘要、Git clean、无活动挂载核验后回收；逻辑字节 1051614341，磁盘实际空闲 20923613184 → 22077681664。保留原件、kit/bundle、Runner 和 native image，不做全局 Docker prune。
- 发布后的纯验收文档会在专用 docs 分支经精确 Linux CI，再快进 main、等 main CI，随后保存 archive/docs/release-v1.24.0-rc.7-acceptance 并回收自有 docs 工作树。精确文档 SHA、CI、refs 和 local/remote 对齐结果以本轮外部 closeout.json 为准，不为文档重发版本。
- 证据目录 `C:/GitHub/_release-evidence/v1.24.0-rc.7`；L3 完整原件 `C:/GitHub/_validation/kpanel-v124-rc7-l3-r1`。复用发布/版本管理/OCR 流程，没有新工作流、PR、会话、代理或自动任务。
