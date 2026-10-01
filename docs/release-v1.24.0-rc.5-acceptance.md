# KPanel v1.24.0-rc.5 发布验收记录

日期：2026-10-01

发布级别：L3

候选提交 / 标签：`35498116b8b325dded78ee006b1dac08c6744c37` / `v1.24.0-rc.5`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / OCI `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`

`releaseChannel`：`preview`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` / 预览版保留；唯一产品工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly`。

## 发布画像

- 业务域：桌面导航及搜索、依赖和构建基座、内置脚本固定版本。
- 变更面：展示和只读入口、构建供应链；本轮不新增宿主写入协议、API、数据库或工作区 schema。菜单操作复用既有关闭守卫、权限和动作入口。
- 用户旅程：K 按钮/快捷键打开菜单，三语搜索并启动系统应用/已安装应用/站点/快捷方式，隐藏条目可打开且不改变隐藏偏好；方向键、IME、Escape、焦点恢复；主题/语言/经典模式/退出登录；三个 3D 场景渲染及切回默认壁纸。
- 风险等级：L3 发布。工具链及受管脚本字节更新需要完整构建、扫描、契约和公开产物核验。新增 UI 不构成授权入口。
- 安装契约、Agent 权限、端口、Compose、存储格式及稳定默认通道不变。预览禁止生产部署，测试只接触自建隔离资源。

## 发布范围与未纳入内容

- 基线：`9c5be9c293f98ad9bce260a4562e28743951b105`，发布产品 SHA 为上述候选。
- `feature/base-202610-all`：`e5fd20062c37ec9a7d12292674b9a469166de828`，12 个提交；包含 Vitest 5.0.3、DOMPurify 3.4.16、Vue/Vite/CodeMirror/图标/Three.js、Go 模块、Node 24.21.0、构建 Actions、Trivy 0.74.0、OCR 1.12.11、三个重建场景及脚本 pin。
- `claude/desktop-start-menu`：`a98a04aed9ae5acb90a566aff333f855653662bb`，3 个提交；包含开始菜单、快捷键/输入法/焦点与滚动修正，以及测试隔离和搜索模型简化。
- 来源范围以 `contract.json`、Git 精确差异与祖先关系为真源。Vitest 单独 tip `a83103346825413f43dafaeedaffc75a394dadbc` 已是整合分支祖先；OCR tip `3b1bc710324d508c634c3098b5564c4384ad2afc` 与整合 cherry-pick `0a78d8a3` 的稳定 patch-id 均为 `dfc0a4cd77f1c29159593f27b2e688637fdda937`，不重复合并。
- `7921ba3d` 集成两来源；`35498116` 仅版本、发布说明、当前业务事实和评审 trailer。评审 head `759c008b` 与产品 SHA 的 tree 相同。
- 未纳入：冻结后的新功能、脚本上游后续提交、无关历史分支、生产部署、CF 审计执行。来源所有权未交接，不推其原名、不归档/删除其工作树或 4176 预览。

## 外部审计与修复交付

- 覆盖检查：`decision=scoped-required`，未审计提交 54、最老 5 天，新增边界包 `internal/backupremote`；last full run-4 source `4c0694aa8e02`，10 天；已有 scoped run-10/9/7/6。原件 `security-coverage.json`。
- 本轮为 RC，只记录覆盖检查，不将未完成审计写成通过；稳定发车需要按既有规则补 scoped。没有本轮 CF run，也未编排 CF 代理。
- 独立复核：菜单实现 Claude，发布复核 Codex。基座复用源码任务已完成的独立 clean-session 复核，明确同提供商 fallback（替代提供商 executable 不可用）；25 文件整合复核与 Vite 两文件增量复核均无新增阻断。完整原件在 `C:/GitHub/_validation/kpanel-base-202610-all/independent-review*.json`。
- OCR 1.12.11：精确范围 `9c5be9c..759c008b`，40 文件中 reviewable 26/26，excluded 14 均解释，无测试文件被默认排除；锁文件、go.sum、文档及生成 dist 分别由锁审查、原件和可重现场景检查覆盖。自由臂 0、本轮有效 H0/M0/L0；已知来源 OCR 披露使 `blind=false`、`constrained-only=unreported`，不计为盲测有效周期。
- `free-form.json` 原件的手工时间有误，保留原件及 `free-form-errata.json`，以文件落盘时间为自由臂完成时间；约束臂在后。评审覆盖不能代替构建、运行或真机证据。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集 `base-202610-all`。
- 内置脚本基线：`779192048077c130442a64a126d7c0050776d868` / SHA-256 `33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04`。
- 脚本候选：`8bebc2d80614e96b844c2c5f88acb0a81d4abd10` / SHA-256 `578b9e4328ba231ad08783c3f2007034f332621d48db07cffe3429da807940b4`；两版本均从公开固定 URL 回读并校验，不使用移动 main。
- 脚本先行已公开；来源固定 clone HEAD 为 8bebc2d8、root/CN 同步及语法、隔离 AI CLI 和 KPanel noninteractive fixtures 通过（原件 `script-smoke-r2.log`）。本次无需再写/发布脚本，不把已有公开兼容版本写成“尚未发布”。
- 七个 KPanel 应用协议函数逐字相同，`script-protocol-diff.json` 保存各函数摘要。真机生命周期核对只提取公开脚本中原样的 service_name/verified_service，只读核验自建 Docker 测试容器 running/exited/paused/restart 状态和错误 ID 拒绝；控制动作只对自建容器执行，结束后删除，现存业务容器 ID/状态保持。原件 `paired-script-lifecycle.log` 和 before/after。
- 兼容矩阵：脚本协议未变，固定候选 Panel 契约/L3、不可变镜像实际内置字节校验与上述原生 Docker 身份核对共同支持兼容；不把身份检查称为脚本原生 install/update/uninstall 或全量 Panel↔脚本双端实机测试。这些未变宿主操作依赖既有 rootfs/协议回归，本轮不在真实业务应用执行。
- 发布决定：脚本兼容版本先可用，再发布 KPanel 预览。成对回滚：rc.4 产品 `0cbee81494991166f9b32b495494a4616463a73d` / OCI `sha256:80578043ae505d6b2f34d3a3cd1b168dab78d89824c68c648846458c5ad991f0` 与基线脚本 779192/33010；稳定回滚点独立列于首部。无阻断或移除的依赖范围。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向 34 测试、菜单和三场景浏览器、完整 L3、公开 image_e2e 与内置字节 pin 通过；互通协议字节相同及实际 Docker 身份核对 | 没有脚本原生宿主安装/更新全旅程的新实机证据 |
| 网络入侵与供应链安全 | 已验证 | 固定 pin、OCR、本次 L3 源码/最终镜像 HIGH/CRITICAL 门禁、Release 安全扫描及公开供应链声明核对 | CF scoped 尚需稳定发车补审；不宣称全域零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | IME、Escape、焦点、失败/空态、关闭守卫、真实 Docker 身份失败拒绝、L3 更新中断恢复及备份一致性、公开 E2E | 无生产重启/恢复，无 fnOS/NAS 新现场 |
| 性能与资源预算 | 已验证 | 浏览器磁盘/内存 watchdog 无越线；本次 Release 的 1CPU/256MiB/128PIDs、非 root、只读根及 cap-drop ALL 运行契约通过；源任务单次 smoke 仅补充 | 不以单样本宣称性能提升；无低配/多卷长测 |
| 用户体验与可访问性 | 已验证 | Chrome 154.0.8037.59 / Playwright 1.55.0；三语、浅深、宽窄、键盘/焦点/IME；最小 computed 字号 12px | 125%/200% 采用等效 CSS 视口压力，不等同原生浏览器/OS 缩放；WebGL 为软件渲染 |
| 数据、配置与迁移 | 不适用 | API、数据、工作区、安装契约无变更；apps 三方规范化内容相同 | 无数据库迁移，未修改正式数据 |

## 自动门禁

- 本地定向：开始菜单三个文件 34/34；npm ci audit 0；版本一致性、diff --check、writer clean/candidate、OCR trailer 通过。
- 冻结执行：唯一入口 `scripts/run-release-l3.mjs`，SSH `arena-154` 使用既有仓库身份；Go 1.26.7 / Node 24.21.0 / npm 11.19.0；Runner 来源 Dockerfile 双基座均固定 digest。归档 SHA `38fb18ef1256ff4b3390ace5cbee681e2009596b2479176875715407be30ede7`。
- 远端实际 Runner ID `sha256:7d8908c158d0b90f7152ed81df5d6eccbe4c811d5f2cb83aecf65f95a9a5b50b`；WSL 来源 ID ddac358c 导入后不同，r1 被机器拦截。原归档 rootfs diff IDs 与远端逐项相同，Go/Node/npm 与双基座标签复核通过；新执行方案重新冻结，未改产品 SHA。
- candidate CI `36871685912`、Dependency freshness `36871685513` 均在精确产品 SHA 成功。L3 r4 227 文件/2054 前端测试、Go/race/govuln、多语言、场景重建、typecheck/build 通过，但镜像阶段因容量停止，终态 failed/137；不能称 L3 pass。用户明确选择“arena-154 清理陈旧内容 重新跑”。核验终态、clean Git、精确 HEAD、容器无挂载和完整原件摘要后，回收 82 份历史 L3 work 副本 26,084,917,248 字节；原始证据和 bundle 全部保留。可用空间 2,979,168,256 → 29,064,077,312 字节，现存容器 ID/镜像/状态相同；未清共享缓存、镜像、volume、inbox 或未知内容。原件 `arena-stale-cleanup.json`；r5 在同一产品 SHA 和远端 Runner 上于 2026-10-01T14:25:19Z–14:43:01Z 完整执行，status=passed/exit_code=0；12 项原始证据回收后逐项 SHA-256 相同，`l3-checksums.json`。Go/vet、三特权包 race、227 文件/2054 前端测试、govuln、Trivy 源码和最终镜像 HIGH/CRITICAL 门禁、3414 phrases/22 catalogs、多语言、三个场景可重现、typecheck/build、双架构二进制、原生镜像及全部 app-conf 生命周期/中断恢复/备份一致性均通过。WSL 备用 kit 未执行。main CI `36878743897` 和主线新鲜度 `36878743966` 在精确产品 SHA 成功。Release、公开 E2E 和纯验收 CI 后续分层记录。
- 本地证据主目录：`C:/GitHub/_release-evidence/v1.24.0-rc.5`；成功/失败 L3 的独立 kit 在 `C:/GitHub/_validation/kpanel-v124-rc5-l3-rN`。失败原件、status、hash 和截图/trace 保留。

## 依赖与技术栈变化

- 采用：Vitest 5.0.3、DOMPurify 3.4.16、Vue 3.5.43/router 5.3.1、Vite 8.3.2、vueplugin 6.0.9/testutils 2.5.1/jsdom 30.1.1、CodeMirror/Lezer、Lucide 1.49/simple-icons 16.33/Three 0.186.1；Go x/image 0.46、x/net 0.59、sqlite 1.60.1；Node 24.21.0、Dockerfile frontend 1.27.1、Buildx action 4.4.1/build-push 7.4.0、Trivy 0.74.0、OCR 1.12.11。精确版本、SHA、digest、tarball/integrity 以 policy/manifests/lock 为真源。
- Source 新鲜度单次 8/10（Actions/CF upstream 403）；此前各源成功并集只作为决策辅助，不伪造一次 10/10 报告。本轮公开 CI 检测完整性单独核对。
- TypeScript 7.0.2 实测 ERR_PACKAGE_PATH_NOT_EXPORTED，与 vue-tsc 3.3.11 不兼容，保留 6.0.3；@types/node 24.19.0 匹配 Node 24 LTS，不引入 Node 26 声明。两项 exception 有 owner、impact、mitigation、exit 和 rollback，reviewDate=2026-10-15。
- 来源 govuln 无 called/imported 漏洞，另有 module-only OpenPGP 未导入通告；Trivy source/image HIGH/CRITICAL 门禁通过仅作为补充，本轮扫描与 CI 另记。传递依赖为 parent-owned 信号，未强制逐项跨大版本。
- 三场景源码未变，版本为 neon-city 1.1.2、orbital-station 1.0.3、sea-and-sky 1.0.3；generated dist/catalog 的重建、hash 与渲染需共同核对。来源首次生成不一致已修正，原失败仍保留。

## 隔离真机与浏览器验收

- `arena-154` 登记允许 candidate-validation/browser-validation/failure-injection；Docker 原生测试容器仅本轮新建，无端口、无网络、read-only、32MiB/16PIDs/.25CPU、无 capability，结束清理。
- 本地 mock acceptance/visual-composition 预览绑定精确产品 SHA，`http://127.0.0.1:4175`；旧 rc.4 自有预览经固定 stop 入口收尾；来源 4176 不触碰。
- 菜单 r2 作业 `arena-154-38836` passed/exit0；三场景 r2 作业 `arena-154-28908` passed/exit0。作业规范与 SHA、起止、超时、log 均在独立 state.json，环境字段为登记策略 ID；浏览器实际运行 Windows 本地 mock，不能称为 arena Panel 实机浏览器验证。
- 宽 1760/1600、窄 390，以及 1280/800 等效布局压力，zh-CN/en-US/zh-TW、light/dark；14 系统入口，搜索文件窗口、IME Enter 不启动、Escape 清空后关闭并恢复焦点、End 选项、Ctrl+K 均断言。空/失败、图标回退、原生快捷键避让、隐藏条目和关闭守卫另由 34 项集成单测覆盖。
- 三场景的 file routes 在浏览器上下文隔离，仅从精确候选 dist 供给，安装目录不写；真实 sandbox=allow-scripts iframe、ready 消息、非零 canvas、选择持久化和切回 classic 全部断言，pageerror=0；不会改变其他预览共享的 mock 安装目录。截图人工查看已完成。
- 无 soak：变更集中于确定性导航及工具链兼容，采用明确完成断言和 watchdog；不宣称 GPU 性能、生产稳定性或 arm64 实机运行。

## 发布产物与公开仓库复核

- GitHub Release：`v1.24.0-rc.5` 于 `2026-10-01T15:02:15Z` 公开，`draft=false`、`prerelease=true`；Release run `36879877690` 和标签新鲜度 `36879877817` 成功，精确 head 均为产品 SHA。公开页 https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.5 。annotated tag 对象 `4ad512fb16b8ee0bee3a73b489aaceeb98ec9ccc`，peeled commit 为产品 SHA。
- 版本镜像及 `preview` 同为 OCI index `sha256:6f9155d3025fcfc25d48bdacda77e8bcde5e828428f82e228dcf98db2e7b1956`；`linux/amd64` 为 `sha256:5de3ed636d080acb1aa4ef1e4ed8f95e4a54889ef4870148a3567610a564a5a2`，`linux/arm64` 为 `sha256:f9c3b764449887832947d0a291f2ab487b991182f176cead1e753ce1f7a5e986`。
- 直接从公开 registry 回读 index 及两份 attestation manifest，字节 SHA-256 与引用相同；每平台均绑定对应镜像 subject，声明 SPDX SBOM 与 SLSA provenance/v1 predicate。原件 `oci-index.json`、`oci-attestation-*.json`、`oci-attestations.json`。不将声明核对称为完整 SBOM 人工审计。
- 14 个附件齐全；`SHA256SUMS` 的 11 个二进制/metadata 条目均与 GitHub API digest 相同。实际下载校验 SHA256SUMS 和 metadata 包，metadata 的 VERSION、许可证、第三方声明、安装/预检/初始化、Compose、systemd/OpenRC 十个关键文件与产品 Git blob 逐字相同。`release-assets.json`、`public-metadata.json`；不声称所有独立二进制均在本机运行。
- `arena-154` 显式拉取上述不可变公开 index，仓库入口 `packaging/tests/image-e2e.sh`、期望版本 `1.24.0-rc.5`、loopback 端口 18085，输出 `image_e2e=pass`；自建容器、网络和临时目录由入口清理。仅 `linux/amd64` 实机执行，不冒充 arm64 实机证据。
- 公开镜像 OCI version/revision 与产品 SHA 相同；停止态自有 bytecheck 容器仅复制 `/release/VERSION`、`kejilion.sh`、`kpanel.conf`，实际脚本 SHA 为 578b9e43…940b4、契约 SHA 为 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，版本 rc.5；容器已删除。首轮辅助 cat 的 stdin CR 异常及重读均保留，不影响此前 E2E。
- GitHub Latest 仍为 `v1.23.0`，Docker `latest` 仍为首部稳定 index；`channel-after.json` 保存实际回读，不改变应用市场默认入口。
- 纯验收记录使用独立 docs 分支和产品 SHA 基线，执行 metrics/L0、同一文档 SHA 的候选/主线 CI 后保存到 `archive/docs/release-v1.24.0-rc.5-acceptance`。最终精确文档 SHA、CI 和引用以对应 Actions、归档引用及本地 `closeout.json` 为真源，不重打产品标签。

- `kejilion/apps`：安装契约未变；候选、公网上游和本地 apps 的规范化文本相同（本地 CRLF 字节差异），无需 apps 写入，不制造空提交。

## 自更新通道验收

stable/preview 通道算法、保持选择、退出预览不降级、后台安装/备份/恢复、失败版本隔离均未改；本轮全量门禁执行现有测试。不把选择来源写成安装授权。OpenRC 与轻量 Node 边界仍按 `release-channels.md`。

## 生产部署安全核对

- 所有生产动作：不适用（预览版禁止生产部署）；产物发布与生产部署分别记录。
- `prod-108` / `108` 禁用全部 KPanel 操作；本轮未连接、未备份、未部署、未升级、未核对。
- `arena-154` 的现存业务容器仅做脱敏只读 inventory，未重启、替换或修改。隔离测试不能替代生产证据。

## 回滚

稳定入口仍为 GitHub Latest v1.23.0 / Docker latest 首部 digest。若后续发现预览缺陷，停止采纳本 RC，以 rc.4 的固定 index 恢复预览来源；产品标签与公开版本不能覆盖。已公开脚本无需回退移动 main；镜像中脚本随对应旧不可变镜像回滚。无生产写入、无需生产数据回滚；不得强制降级已安装实例。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-01T17:21:10+08:00
- 候选冻结时间：2026-10-01T21:00:01.984+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：15
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "runner-preflight/wsl-cli/argument-carrier",
    "position": "before-production-write",
    "count": 1,
    "impact": "WSL Runner inspect 的直接命令形式收到异常 encodedCommand 参数，预检失败，未生成有效 Runner 证据。",
    "recoveryEvidence": "改用 wsl.exe --exec /bin/sh -c 的明确载体，导出归档、source config、SHA 及远端 smoke 均保存于 runner-provenance.json。",
    "permanentAction": "发布负责人于 2026-10-15 前复核 WSL 参数形式；后续沿用固定 --exec 载体并在导出前核验 image ID。",
    "historicalReleases": []
  },
  {
    "fingerprint": "review-evidence/manual-timestamp/wrong-time",
    "position": "before-production-write",
    "count": 1,
    "impact": "自由臂 JSON 的手工时间错误，被证据复核拦截；未用其计算发布或实验指标。",
    "recoveryEvidence": "free-form-errata.json 以原文件落盘时间修正元数据，原件未覆盖；constrained-only=unreported。",
    "permanentAction": "发布负责人于 2026-10-15 前复核元数据生成方式；直接使用运行时 ISO 时间或原件权威时间，不手填。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preview/css-zoom/fixture-layout",
    "position": "before-production-write",
    "count": 1,
    "impact": "测试对 document 根设置 CSS zoom 导致旧桌面固定栏移出视口，不能代表原生浏览器缩放；r1 未完成。",
    "recoveryEvidence": "r1 原件保留；r2 用明示等效视口压力完成 5 组，menu-job-r2 passed/exit0；不宣称原生缩放通过。",
    "permanentAction": "固定 r2 spec 与 zoomMethod；原生缩放保留未验证，发布负责人于 2026-10-15 复核是否需要专用 Chrome zoom 入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-preflight/runner-import/image-id",
    "position": "before-production-write",
    "count": 1,
    "impact": "WSL 导出后 arena Docker 报告 ID 改变，r1 精确 Runner 门禁拦截，未开始测试。",
    "recoveryEvidence": "归档 hash、rootfs diff IDs 一致、基座 labels 及 Go/Node/npm smoke 确认；用远端实际 7d8908c1 ID 重冻方案；r1 status failed/exit1 和全部 evidence hashes 保留。",
    "permanentAction": "传输前先预检目标 Docker 导入的实际 ID；发布负责人于 2026-10-15 复核导出/导入兼容，不能以源端 ID 冒充目标 ID。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preview/sandbox-frame/init-storage",
    "position": "before-production-write",
    "count": 1,
    "impact": "测试 initScript 也注入 opaque-origin iframe，访问 localStorage 产生 3 个错误；场景渲染和切回虽完成，r1 仍判失败。",
    "recoveryEvidence": "r2 init 只在 top 执行；scene-job-r2 passed/exit0、3 场景 running 与切回 classic、pageerror=0，保留 r1。",
    "permanentAction": "浏览器 fixture 初始化明确 frame 范围；固定 scene-preview-r2.cjs/spec SHA，发布负责人于 2026-10-15 复核复用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-gate/arena-capacity/staging-space",
    "position": "before-production-write",
    "count": 2,
    "impact": "r2 hostbackup staging 容量检查失败；回收后 r4 源码检查通过，但镜像构建瞬时容量预算仍不足，可用磁盘一度归零，按自有 mount/image 核验后停止 Runner，终态 failed/137。没有放宽门禁或修改产品。",
    "recoveryEvidence": "r1/r2/r4 完整 evidence hashes 校验后仅回收自有 work/inbox；r4 12 项摘要通过，空间恢复 2.8GiB、现存 Panel healthy。arena-recovery.json 和 arena-capacity-stop.json 保留；不以 partial gate 当 L3 pass。WSL839GiB kit已准备，待明确环境选择。",
    "permanentAction": "发布负责人于 2026-10-15 复核容量预算和日志回收方案；每次 Runner/kit 导入前检查瞬时编译与 staging 余量，不清未知缓存。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-prepare/github-ssh/fetch-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "r3 isolated-source fetch 超时，未生成有效 L3 plan、未连接执行 gate；固定入口已清理临时 clone。",
    "recoveryEvidence": "source-prepare.json 与 l3-orchestration-r3.log 保留；原仓库身份 ls-remote 恢复精确 main/candidate，独立 r4 重跑。",
    "permanentAction": "GitHub 上游瞬时网络例外；发布负责人于 2026-10-15 复核，恢复条件为既有 SSH 身份连通且同精确 refs，不绕过唯一入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "transport-preflight/ssh-env/identity-override",
    "position": "before-production-write",
    "count": 1,
    "impact": "辅助连接重试设置 GIT_SSH_COMMAND 时未保留仓库 core.sshCommand 身份，fetch/ls-remote 拒绝；没有远端写入。",
    "recoveryEvidence": "取消临时覆盖，以原仓库 core.sshCommand 的登记 key 成功复核 refs；固定 L3 transportEnvironment 继续继承原配置。",
    "permanentAction": "不覆盖已有 SSH 身份；发布负责人于 2026-10-15 复核辅助网络预检，若加选项必须保留原配置。",
    "historicalReleases": []
  },
  {
    "position": "before-production-write",
    "fingerprint": "acceptance-metrics/schema/value-format",
    "permanentAction": "指标按 report-release-metrics.mjs 的封闭格式生成；发布负责人于 2026-10-15 复核模板复用。",
    "impact": "验收草稿的 7 位时间小数和带解释的不适用值未符合机器指标 schema，校验拒绝；尚未提交。",
    "recoveryEvidence": "原稿 acceptance-draft-r1.md 保留；使用三位毫秒和精确不适用标记，重新校验。",
    "historicalReleases": [],
    "count": 1
  },
  {
    "impact": "Runner mount 读取采用带空格 json 模板，SSH 拆参导致 template parsing error，ownership 前置失败，未执行停止。",
    "fingerprint": "supplemental-runner-query/ssh/format-argument",
    "count": 1,
    "position": "before-production-write",
    "historicalReleases": [],
    "permanentAction": "辅助读取禁用跨 SSH 的带空格 format 参数，采用整体 JSON 再在本地结构化选择；发布负责人于2026-10-15复核重复参数错误。",
    "recoveryEvidence": "随后捕获 inspect JSON 到内存，明确 mount 路径和 image ID 一致才停止唯一自有 ef767b7a9851；没有输出 Env 或凭据。"
  },
  {
    "fingerprint": "workflow-static/yaml-parser/missing-module",
    "historicalReleases": [],
    "recoveryEvidence": "在仓库外安装固定 yaml@2.8.1，三份 GitHub workflow 由 YAML1.2 parser 成功解析，workflow-yaml.json 与外部 lockfile 保存，产品依赖未改。",
    "position": "before-production-write",
    "impact": "本地 web Node 和 bundled Python 均无 YAML 解析包，两次解析预检拒绝；未将工作流语法计为已通过。",
    "permanentAction": "发布负责人于2026-10-15复核静态检查的能力清单，先确认已安装模块或使用固定外部工具，再执行解析；不成为生产工具依赖。",
    "count": 2
  },
  {
    "historicalReleases": [],
    "fingerprint": "ci-evidence-query/powershell/syntax",
    "impact": "辅助 CI 状态格式化命令含多余闭合括号，PowerShell 解析失败；查询未执行，没有据此判定通过。",
    "recoveryEvidence": "改为独立赋值、直接选择 steps 字段的多行命令；main-jobs.json 保存实际 API 返回，主线仍按精确 SHA 和 completed/success 判定。",
    "permanentAction": "发布负责人于 2026-10-15 复核结构化读取；辅助查询复用已成功的多行参数形式，避免内联嵌套。",
    "position": "before-production-write",
    "count": 1
  },
  {
    "position": "before-production-write",
    "count": 1,
    "recoveryEvidence": "保留 public-image-bytes-r1.log；直接 SSH 参数回读三个固定文件，脚本、契约与 VERSION 一致，trap 已删除自有 bytecheck 容器，复核无残留。",
    "historicalReleases": [],
    "fingerprint": "public-artifact-read/powershell-stdin/final-cr",
    "impact": "辅助字节回读的 PowerShell native stdin 在末行附加 CR，cat VERSION 失败；之前三项 docker cp 与两个 SHA 回读已完成，E2E 不受影响。",
    "permanentAction": "发布负责人于 2026-10-15 复核 stdin 承载形式；固定单行只读参数或按字节送入 stdin，避免末行 CR。"
  }
]
<!-- kpanel-release-process-incidents:end -->

## 分支与资源收尾、未完成项

- 产品候选本地/远端保持产品 SHA；验收只经独立 `docs/release-v1.24.0-rc.5-acceptance` 分支的同 SHA candidate/main CI，再 exact-tip 归档为 `archive/docs/release-v1.24.0-rc.5-acceptance`。最终 D、CI、refs、management sync 和 cleanup 以 `closeout.json` 为权威。
- 未交接来源分支和历史未知资源保留；Vitest/OCR 单独来源已纳入/等价替代，不再作为未发布功能队列。产品工作树、node_modules 与 ready 用户预览保留，发布任务负责 stop；纯验收工作树和自建验证副本在证据恢复并核验无进程引用后回收。
- 未验证风险：CF scoped；原生脚本宿主安装/更新全链路；fnOS/NAS 新现场；浏览器原生缩放；公开 arm64 运行；GPU 和低配/多卷长测。由稳定发车或专项验证触发，不把预览验收扩大为生产准入。
