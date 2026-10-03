# KPanel v1.24.0-rc.11 发布验收记录

日期：2026-10-03

发布级别：L3

候选提交 / 标签：`ed3299a9cf8fad494b09124cadf4fa89dc350fe3` / `v1.24.0-rc.11`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96`；Docker `latest` 为 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` / 预览版保留。产物已发布，生产未部署。

- 产品 SHA 固定在不可变 annotated tag；纯验收文档经独立候选/主线 CI 后快进，对齐本地与远端。
- `feature/terminal-job-duplex-input` / `e3f16efca68a163805e59295561331600c7a0e4c` → `refs/heads/archive/feature/terminal-job-duplex-input/v1.24.0-rc.11`，远端精确 SHA 已核对；原作者本地分支、工作树及活跃远端引用保留。
- `fix/terminal-reload-session-leak` / `585cc8cc174ac82ee444e81a54ca4e59a38ebc81` → `refs/heads/archive/fix/terminal-reload-session-leak/v1.24.0-rc.11`，远端精确 SHA 已核对；原作者本地分支、工作树及活跃远端引用保留。
- `fix/terminal-cursor-block` / `f228f5535b8e4c3ee250a33979ff73b3c4495d0d` → `refs/heads/archive/fix/terminal-cursor-block/v1.24.0-rc.11`，远端精确 SHA 已核对；原作者本地分支、工作树及活跃远端引用保留。
- `feature/terminal-multi-session-host` / `d8e19d2b9535a7a37861513a842f61eb81ca0668` → `refs/heads/archive/feature/terminal-multi-session-host/v1.24.0-rc.11`，远端精确 SHA 已核对；原作者本地分支、工作树及活跃远端引用保留。
- `claude/gallery-cover-move` / `c9506fdfe0a4a8607b77f2d31c7cfa41fc45d2ee` → `refs/heads/archive/claude/gallery-cover-move/v1.24.0-rc.11`，远端精确 SHA 已核对；原作者本地分支、工作树及活跃远端引用保留。
- 自有纯文档分支 `docs/release-v1.24.0-rc.11-acceptance` 在同 SHA 主线 CI 成功后归档到 `archive/docs/release-v1.24.0-rc.11-acceptance` 并回收；实际文档 SHA、远端核对、字节回执以 evidence 根的 `acceptance-archive.json`、`acceptance-worktree-cleanup.json`、`final-alignment.json` 为准。
- 未收回任何活跃作者任务；未知工作树、唯一原件、源分支与未成熟候选保留，不因远端归档删除本地。

## 发布画像

- 业务域：WebSSH 本机/完整 Panel/轻量 Node，多终端与页面生命周期，图库经典页/桌面窗口组合。
- 用户旅程：主机旁加号 → 独立标签和序号；关闭 → 服务端释放；刷新 → 旧会话回收、新会话打开；空闲 → 保活后继续输入；图库菜单 → 可点击，筛选随内容滚动，选中操作条保留。
- 未变化契约：既有 API、数据库、配对 scope、端口、Compose、Agent 权限、脚本 PTY/FIFO 与应用市场默认 stable/latest；不增加宿主权限或数据迁移。
- 风险：输入连接和卸载时序按 L2 关注，本次发布执行完整 L3。浏览器、真实字节与公网产物证据分开记录。

## 发布范围与未纳入内容

- 纳入五个成熟 tip：20 秒输入 WebSocket 保活；无排队输入的空闲重连保持安静；页面 pagehide 同源 CSRF keepalive 关闭；统一稳定块状光标；同主机多个标签、序号空位复用和上限反馈；图库筛选条取消悬浮、选择操作条继续 sticky。原 RC9 菜单层级和封面/移动能力保留。
- 崩溃、断网或组件挂载前刷新的会话仍靠原 35 分钟超时回收；bfcache 页面保留会话；连接持续失败仍可能 fail closed。创建请求未完成时切换另一主机的静默 no-op 为既有后续项。
- Windows 聚合 `feature/windows-light-node` 冻结时观察 tip `f82eaf81d2a96c113117b53158ce8c1a42ce8ade`，主线验收期间复核已到 `651a75ef3b874a83aee0c81e46177842653f0465`，仍暂缓：作者仍在收尾，签名身份/配置/真实签名附件未就绪，原生安装、更新、RDP 矩阵未完成。平台、runtime、RDP bridge/UI 和设计源一并保留，不能按 Mock 或编译成功宣称已发布 Windows 能力。
- Node26、TS7 benchmark、治理与业务基线草稿暂缓；OpenWrt telemetry/base OCR 旧源为已发布 patch-equivalent；light-node-dependencies 的旧脚本 pin 已被 c981fb6c 替代，其文档 bootstrap 增量仍不在本终端/图库范围。

## 外部审计与修复交付

- 本轮未新增 CF run。既有 full `run-4` 源码 `4c0694aa8e02e46145a775707b8d5a0355f7ce10` 及 scoped 记录只按其原范围有效。
- 覆盖检查：`decision=scoped-required`，未审计提交 68、文件 124、最老 7 天；上次 full 12 天，新边界 `internal/backupremote` 来自此前版本。RC 非阻断，正式 1.24.0 前需按项目要求补审；执行配置为专用 `gpt-6-luna/max`。不能写成本轮 CF 审计通过。
- OCR 30/30 reviewable files、35 个总文件。发布责任人为 Codex，选定源为 Claude；free-form 先持久化再读 constrained 规则，作者 trailers/已知后续已可见，不能宣称盲审或新增外部独立审计。最终同 tree 候选有效 H0/M0/L0、constrained-only=0，无新确认阻断缺陷；不足三个稳定周期不判断工具有效性。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））。变更集编号、脚本候选与阻断依赖：不适用。
- 实际内置脚本 `c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`；既有输入协议兼容，不新增动作或安装/更新契约。
- `kpanel.conf` 归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，本地 apps、远端 main、产品源及公开镜像 bytes 已核对；apps 本地 clean、`kejilion/apps` 本轮无写入。脚本不在范围，无需发布脚本不是暂缓脚本发布。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 6 个原生浏览器场景，6 个真实主机输入样本，RC10/RC11 双方向 12 场景 | Linux 容器隔离配对，不代表 Windows/RDP 或真实安装业务 |
| 网络入侵与供应链安全 | 已实现未实机验证 | L3 race/vet/govulncheck/npm/Trivy、公开 SBOM/provenance subject 绑定 | CF 覆盖待补审；模块树不可达通告不等于零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | Chrome/Edge 各 6 次刷新旧会话 404、65 秒空闲后真实输出，6 个 PTY 故障，混合版本无 orphan PTY | 有限窗口、受控代理；非 WAN/丢包/长期 soak |
| 性能与资源预算 | 已验证 | 128 KiB 随机粘贴，32 帧/64 KiB flight；local RTT150 粘贴 327.195ms；full-panel RTT150 粘贴 327.067ms；light-node RTT150 粘贴 318.086ms | RTT0/150 模型各一轮；32 次 echo，未比较旧版倍数，点采样非资源峰值 |
| 用户体验与可访问性 | 已验证 | Chrome/Edge 16 Mock + 4 桌面组合 + 6 原生场景，键盘、光标、关闭与零 pageerror；100% 抽测图库12px、终端16px | 字号只确认抽测绝对下限；既有图库小按钮12px与14px操作控件规范仍有差距；CSS zoom 非原生 browser zoom，未覆盖 Safari/OS 剪贴板 |
| 数据、配置与迁移 | 已验证 | 无新增迁移；L3 更新/备份/失败恢复；公开产物及最后清理窗口 18 个业务容器状态未变 | 比较窗口不追溯为之前阶段或生产证据 |

## 自动门禁

- `make verify-release`：固定 Linux Runner Go tests/race/vet、前端 typecheck 与 251 文件/2256 测试、govulncheck、npm audit、Trivy、双架构构建、镜像权限、受管脚本、安装/更新和失败恢复通过。govulncheck 可达漏洞 0，另有 1 条未调用模块通告。
- 唯一 L3 入口 `scripts/run-release-l3.mjs`，run `v1.24.0-rc.11-ed3299a9-l3-r1`，候选 `ed3299a9cf8fad494b09124cadf4fa89dc350fe3`，passed/exit0，12 份原始 evidence checksum 已核对；路径 `C:/GitHub/_validation/kpanel-v124-rc11-l3-r1/remote-evidence`。
- Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0；plan `0486bd1f9198eacd0a7f7f5d304b53a7276d10908b6c844911d854e3421236a6`；remoteScript `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `4b58e30fe5486f8042f7fd4d0eb79fedd47a9061a885b1202034f210720f877b`。
- [candidate CI](https://github.com/kejilion/KPanel/actions/runs/37111756630)；[candidate Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37111756641)；[main CI](https://github.com/kejilion/KPanel/actions/runs/37112361287)；[main Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37112361292)；[Release](https://github.com/kejilion/KPanel/actions/runs/37112953390)，精确产品 SHA 全部成功。文档候选/主线及最终候选 CI 回执独立存放，不替代产品门禁。

## 依赖与技术栈变化

- 未升级依赖、Action、基础镜像或脚本 pin；Go 1.27.1 / Node 24.21.0、锁文件和 Action SHA 沿用现有基线，仅版本元数据一致更新 RC11。
- 实际本轮候选/主线 Dependency freshness 与 Release 依赖任务成功，沿用每日安全/EOL 检测；不虚构额外 `make dependency-report` 或外部审计。Node26/TS7 草稿待作者治理/基线条件满足再复核，不在本轮采用。
- 二进制双方向兼容及公开镜像运行验证通过；可回到不可变 RC10，稳定默认未切换。

## 隔离真机与浏览器验收

- `arena-154` Linux amd64/Docker，固定 Runner 中运行真实 Panel/Agent/Node TLS/Noise/PTY，新独立配对；原生浏览器由本机 Chrome/Edge 驱动。原生 API fixture cap-drop ALL/read-only/no-new-privileges、768 MiB/1.5 CPU/256 PID、有时限，端口仅 loopback。
- 原生浏览器 HTTPS Vite 代理使用隔离自签名证书并改写 Host/Origin，Cookie flags 保留；浏览器忽略 fixture 证书错误。不是公网 TLS、Cloudflare、原生 systemd/procd 或 Windows 证据。
- Chrome/Edge 1366×900，真实后端四会话上限、3 类主机、每浏览器 6 次刷新/旧服务端会话 404、65 秒输入 socket 升级数不变，再执行安全 printf 并在服务端 output bytes 看见完整 marker；pageerror 0。
- 原生作业 `arena-154-23804`，passed/exit0，360 秒硬超时，spec SHA `75051bdaf792da402cf59c3b3abd430b4e11b4d42197bf81a7510c49ca243f43`；`browser-native-result-r3.json`。Fixture 最终 container/network 删除确认。
- Mock 16 场景：多标签/序号复用/上限/刷新/键盘光标；图库浅色100%、深色CSS125%、浅色CSS200%、390窄视口。作业 `arena-154-31892`，passed/exit0，spec SHA `efb2cbc4f6e10b29f1ecd5d27ac9a502219b96b3e648146a1a67d884f8d5e220`。CSS zoom 非原生 browser zoom，不以 Mock 数据证明宿主动作。
- 桌面图库 Chrome/Edge 浅/深色各一场景，共4；菜单点击、静态筛选/选中操作条，fixture 仅5像素滚动，不外推长列表窗口。作业 `arena-154-31572`，passed/exit0。
- 原始失败全部保留：共享 Mock 残留会话、视口外 hit-test、旧 RC9 能力预期、缺少 evidence 目录、HTTP Cookie origin、output 缺 wait。均只修仓库外 harness，冻结产品未变。

## 发布产物与公开仓库复核

- [v1.24.0-rc.11](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.11)：draft=false、prerelease=true，published `2026-10-03T09:37:43Z`；GitHub Latest 仍 `v1.23.0`。
- 版本和 `preview` OCI index `sha256:a1f2cabf9164001e37b6cd8d05dd7a3a2dc80ed07c0eee082cda304a7306e060`；amd64 `sha256:a302f71f5ba2fd4b57b172ad53ba81fb00b22944125983470355a0aa92f4723f`；arm64 `sha256:9d0437bc75cc3659529029515e0dd9fbdc058ac21003c97e590638e657356754`；`latest` `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5` 未变。
- 14 项附件，11 项 SHA256SUMS 与 GitHub asset digest 核对；metadata blob 与源码相同，实际下载 amd64 Node 校验。其余二进制只核对 digest，不当作原生执行。
- OCI 双架构 config version/revision/script labels、65532 非 root runtime 契约、SBOM/provenance predicate/subject 绑定核对；没有对完整 SBOM 内容做安全审计。
- 公开 amd64 不可变镜像 `image_e2e=pass`；VERSION、脚本、appconf、图库 icons bytes 核对。临时容器/network/数据删除；arm64 构建/manifest 不能替代 arm64 真机。

## 自更新通道验收

- 沿用现有通道契约并由本轮 L3 回归：stable 选正式 Latest，preview 选规范 RC、校验唯一官方 digest；加入预览只切换来源并检查，不自动安装。
- 自动安装开关与一次性安装分开，旧状态默认 stable，选择持久化；退出预览不自动降级。
- systemd 备份、失败/崩溃恢复按 L3 原始证据；OpenRC/轻量 Node 边界见 release-channels。生产管理员未加入预览、未安装 RC11。

## 生产部署安全核对

- 生产目标、授权、备份、部署、postdeploy、生产管理员写入：不适用（预览版禁止生产部署）。生产写操作 0。
- 验证环境仅 `arena-154` 和本地 Mock。`prod-108`/`108` 全部 KPanel 操作禁用，本轮未连接、未读取、未备份、未部署或升级。
- 隔离 Runner、PTY、浏览器和公开镜像验证不替代生产部署证据。

## 回滚

- 上一预览 `v1.24.0-rc.10` / `6b3cfa88a47b8da473a443fa0e60190a4bffdae6`；镜像 `sha256:ecf275a256e99ce1d18586fd46d5b86e545153033c99fac53ac2c0aaa714d6db`。
- 无生产数据/配置修改，实际回滚、生产备份不适用。降级须另行授权并绑定精确摘要和对应备份，历史 tag 保留。
- GitHub Latest、Docker latest、应用市场默认仍稳定 1.23.0；公共默认更新通道决策：不适用。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-03T13:26:56+08:00
- 候选冻结时间：2026-10-03T08:17:48.647+00:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：18
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

正常门禁发现的产品问题不混入流程工具计数；首轮原始证据保留。实际比较 v1.19.0—v1.23.0 五份正式验收，按精确指纹填写历史；不同命名的类似根因不能据此排除复发。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/read-only-git-source-inspection/wrong-repository-cwd",
    "position": "before-production-write",
    "count": 1,
    "impact": "script commit inspection was not valid; no mutation",
    "recoveryEvidence": "retain diagnostic, use immutable embedded script baseline and canonical repository evidence; stale source pin excluded",
    "permanentAction": "Release owner: preserve failed evidence, use the corrected frozen outside-repository entry and verify schema/namespace before future runs. Next review before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preflight/external-mock-browser-spec/shared-mock-session-state",
    "position": "before-production-write",
    "count": 2,
    "impact": "two attempts could not assert user-wide cap with another session outstanding",
    "recoveryEvidence": "dedicated preview/API port namespace; all terminal assertions pass; first failed results retained",
    "permanentAction": "Release owner: preserve failed evidence, use the corrected frozen outside-repository entry and verify schema/namespace before future runs. Next review before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-assertion/external-mock-browser-spec/off-viewport-hit-test",
    "position": "before-production-write",
    "count": 1,
    "impact": "CSS 200% menu item was below viewport; hit test returned no element",
    "recoveryEvidence": "scroll item into viewport before hit test; unchanged product candidate passes all 16 cases; original screenshot retained",
    "permanentAction": "Release owner: preserve failed evidence, use the corrected frozen outside-repository entry and verify schema/namespace before future runs. Next review before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 8,
    "impact": "Guessed source, layout, metrics and harvested-log filenames or Windows rg wildcard were unavailable; read-only inspection did not produce valid evidence.",
    "recoveryEvidence": "rg --files discovered actual DesktopView and report-release-metrics entries; terminal behavior read from manager.go. No source mutation.",
    "permanentAction": "Release owner: resolve exact existing filenames with rg --files before reads; use literal directories and rg filters instead of Windows wildcard operands. Recheck before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "host-preflight/external-compatibility-harness/stale-baseline-capability",
    "position": "before-production-write",
    "count": 1,
    "impact": "RC9 harness expected RC10 target capabilities to be absent; native candidate/fault stages passed, mixed-version probe was invalid.",
    "recoveryEvidence": "host-evidence-r1/result.json and compat-newcenter-oldtargets.log preserved; fresh host-r2 tests actual RC10/RC11 protocol on both directions.",
    "permanentAction": "Release owner: bind expected capability assertions to the exact old release version and inspect its source before the next stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture-preflight/native-runtime/missing-evidence-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "Native fixture setup had paired successfully but could not write its nonsecret setup receipt into a missing output directory; owned container/network cleaned.",
    "recoveryEvidence": "native-prepare-r1/setup.log and helper/log retained; native-r2 creates the expected evidence directory before setup.",
    "permanentAction": "Release owner: create and validate harness output directory contract before startup; next review before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preflight/native-browser/secure-cookie-origin-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Browser harness tried to inject __Host- cookies on its HTTP frontend and failed before any journey; owned runtime removed.",
    "recoveryEvidence": "browser-native-job and native-browser-r1 snapshots retained; native browser r2 uses HTTPS Vite with the isolated fixture certificate, secure cookies and browser certificate bypass.",
    "permanentAction": "Release owner: preserve actual cookie flags and HTTPS origin contract in native browser adapters before the next stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-assertion/native-output-query/missing-required-wait",
    "position": "before-production-write",
    "count": 1,
    "impact": "Native browser passed cap/reload assertions and idle socket assertion, but its live output GET omitted required wait=0 and was correctly rejected with HTTP 400.",
    "recoveryEvidence": "browser-native-job-r2 and browser-native-result-r2.json retained; actual terminalOutputQuery contract checked; fresh r3 adds wait=0 to both live and old-session requests.",
    "permanentAction": "Release owner: derive test requests from the actual API query contract before native browser runs; review before stable release. Review deadline: 2026-10-10; exit only after the corrected entry completes on an exact candidate with preserved failure evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/remote-github-file-read/metadata-schema-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "Connector returns raw repository file contents; parsing them as JSON metadata failed.",
    "recoveryEvidence": "Correct raw file response captured and verified in apps-contract.json.",
    "permanentAction": "Release owner: validate raw response schema and preserve exact final newline when serializing content. Review by 2026-10-10; exit after exact byte hash equals all contract sources.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/file-transport/extra-final-newline",
    "position": "before-production-write",
    "count": 1,
    "impact": "apply_patch serialization added one final newline to the read-only evidence copy and correctly triggered hash mismatch.",
    "recoveryEvidence": "Final-newline serialization corrected; remote evidence copy now matches authoritative normalised SHA256 exactly.",
    "permanentAction": "Release owner: validate raw response schema and preserve exact final newline when serializing content. Review by 2026-10-10; exit after exact byte hash equals all contract sources.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- Arena 自有可再生源码/inbox/二进制与临时配对密钥已回收，实际逻辑字节 `1050959514`；磁盘实际变化 `1113456640`。保留原始 evidence、bundle、Runner/cache；18 个既有业务容器在公开产物/最后清理比较窗口状态相同。
- 自有文档工作树同 SHA 主线 CI 后回收，实际字节以收尾 JSON 为准；作者与未知工作树、唯一原件保留。最终候选/main 精确对齐由 final-alignment 验证。
- 最终 Mock `http://127.0.0.1:4175` 只供交互查看，将在文档对齐后由固定 local-feature-preview 入口绑定最终 clean 文档 SHA，停止方法 `node scripts/local-feature-preview.mjs stop --evidence-dir C:/GitHub/_release-evidence/v1.24.0-rc.11/preview-published`。
- 抽测计算字号最小图库12px、终端16px；既有 button--small 在 RC10 已为12px，当前未修改共享 CSS，不能把绝对下限通过解释为操作字号14px规范完全通过。后续 UI 规范适配需解决；本轮没有扩大为全局样式重构。
- 未验证 CF 新增覆盖、真实 WAN/CDN/长期 soak、Safari/移动、原生浏览器/OS 缩放、arm64 原生、Windows 安装/更新/RDP；正式版前补审与相应原生准入，不外推当前 Linux 和 Mock 结果。
