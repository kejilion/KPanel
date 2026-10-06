# KPanel v1.25.0-rc.7 发布验收记录

日期：2026-10-06

发布级别：L3

候选提交 / 标签：`6c2aa2c0423e0a6f349af7b4ac2154db997d01de` / `v1.25.0-rc.7`

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；稳定 OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`

`releaseChannel`：`preview`

`releaseTrain`：`1.25.0`

候选分支与发布后处置：`release/v1.25.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：`feature/remove-windows-light-node-20261006` / `6c2aa2c0423e0a6f349af7b4ac2154db997d01de` / 已快进纳入 RC7；远端 `main` 与 `release/v1.25.0-candidate` 在发布时都指向该提交。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：Windows 轻节点设计归档为 `docs/archive/windows-light-node-design-rc2-rc6.md`；历史 RC2–RC6 tags 与 Releases 保持不变。RC7 tag 和远端候选分支均指向 `6c2aa2c0423e0a6f349af7b4ac2154db997d01de`。
- 本次来源任务分支：只新增本轮 Windows 轻节点移除候选；RC6 已整合来源及处置沿用 [`release-v1.25.0-rc.6-acceptance.md`](release-v1.25.0-rc.6-acceptance.md)，本轮没有重放或删除其他候选。
- 本地分支/upstream/worktree：当前任务 worktree `C:\GitHub\_codex-tasks\kpanel-remove-windows-light-node-20261006` 保留；发布候选远端 ref 保持在 RC7 SHA。原 RC6 worktree `C:\GitHub\_codex-tasks\kpanel-v125-rc5` 仍固定于 `9a63e0f57bc257bfb3e8b62f136429da84827583`，未重置或回收。验收记录以独立 docs-only 提交汇入 `main`，不移动 RC7 tag 或候选 ref。
- 未完成归档项 / 责任人 / 下次复核触发条件：无代码归档待办；历史 Windows 节点代码与发布资产由不可变 RC2–RC6 保留。若未来重新引入 Windows 节点，重新建立候选并完成其发布门禁。

归档不代表生产上线；RC7 预览产物已发布，生产未部署。

## 发布画像

- 业务域：集群轻量节点、远程终端与节点文件管理。
- 变更面：移除 Windows 节点业务界面、接入/报告处理、RDP 与终端桌面桥接、Windows 节点文件/图库/集群分支及其安装、更新和发布产物；保留通用 Windows 主机能力和 Linux 轻量节点。
- 受影响用户旅程：Windows 节点不再可接入或管理；已有记录保留但显示离线。Linux 节点仍可接入；补上平台字段持久化，允许已知新 Linux 节点在首份报告前启动中继。
- 未变化契约：不新增 API、数据库迁移、端口或 Compose 配置；Linux 轻节点协议与 `kejilion.sh` 固定版本不变；稳定更新通道、GitHub Latest、Docker `latest`、默认应用市场通道不变。Windows 节点 enrollment/report 会明确拒绝 Windows 平台。
- 风险等级及理由：L3。此版本撤下已发布的节点接入与宿主交互能力，并改变旧 Windows 节点的可管理状态；通过预览发布，保留历史 tag 和稳定回滚点。

## 发布范围与未纳入内容

- 用户可见更新：撤下 Windows 单台/批量接入、PowerShell 会话、RDP、Windows 节点终端/文件/图库/历史监控及专属安装/更新入口；删除 Windows 节点 Agent、安装器与 Windows Runner 专属发布门禁。Linux 轻节点仍受支持；回归修复将 `Platform` 持久化，省略平台的新接入按 Linux 处理，未知旧记录不会因此获准，Windows 记录强制离线且不可管理。历史设计文档移入归档目录，清理 8 处过时本地化提示。
- 精确提交清单（RC6 tag `9a63e0f57bc257bfb3e8b62f136429da84827583` 至 RC7）：`ce872fe95da69b34dd3089d5374bd99707880c04`（RC6 验收记录）、`da829e348da4516f72f4ca5e6e4a4fd809a549e2`（移除 Windows 轻节点）、`6c2aa2c0423e0a6f349af7b4ac2154db997d01de`（保留 Linux 节点启动修复）。
- 明确未纳入的分支、文件或后续事项：不改 RC6 及更早不可变 tag/Release，不改稳定发布或生产环境，不改 `kejilion/sh`。通用 `kpanel-mcp-windows-*.exe` 是独立 MCP 工具产物，不属于 Windows 节点 Agent/安装器；因此继续保留在资产中。未执行 Windows 节点真机/RDP 验收。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：未另行运行 Cloudflare `security-boundary-audit`。`node scripts/check-security-audit-coverage.mjs --target HEAD` 对最终提交的 decision 为 `ok`，原因是本次没有新增边界包；这不等同于一次 scoped/full 审计。源代码、安全依赖、密钥与配置扫描由候选 CI、主线 CI、L3 和 Release workflow 完成。L3 `govulncheck` 显示 0 个可达漏洞；另有 1 个依赖模块公告，但扫描未发现代码调用受影响路径。
- 覆盖检查（`check-security-audit-coverage.mjs` 的 decision、未审计提交数；RC 只记录。稳定版带 `--require`，非 ok 时写补审 run，或用户原话、理由与不超过 14 天的补审截止日）：decision=`ok`；未审计提交数未单独记录；新边界包数为 0。稳定发布仍按届时 coverage 结果执行强制门禁。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本版扫描没有确认新的安全 finding。Linux 节点启动回归由 `6c2aa2c0423e0a6f349af7b4ac2154db997d01de` 修复；持久化及首报告前中继行为由 `internal/cluster` 回归测试覆盖，并由独立固定 L3 runner 验证。
- 修复交付状态：源码、RC7 tag、公开预览 Release 与 `preview` 镜像均为 `6c2aa2c0423e0a6f349af7b4ac2154db997d01de`；稳定版和生产均未交付、未部署。Release 明确说明旧 Windows 节点在本版离线且不可管理。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：未记录；本次没有 OCR 审查报告，未推断工具有效或无效。
- 按 PROJECT_RULES.md 5.4/5.5 记录本稳定周期的观察结果；不足三个周期不提前宣称工具有效或应退出：本次没有新的稳定周期观察数据，未作有效性或退出结论。
- 候选分支的原始 tip、归档位置和远端核验统一填写本模板既有候选处置字段，遵守 `docs/release-channels.md`。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：`remove-windows-node-rc7-20261006`（仅供本仓库追踪；没有跨仓库发布）。
- KPanel 实际内置脚本基线 commit / SHA-256：`c3a8bd895f8878d9e4ced7592c91a20c974472a5` / `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`。
- 脚本候选 commit / SHA-256：不适用；候选与基线相同。
- 状态判定依据与兼容性证据：Windows 节点接入由 KPanel 自身 Release 附件提供，不依赖 `kejilion/sh`；Linux 轻节点脚本协议未变，Dockerfile 与 Compose 未改。
- 本版发布决定：脚本不在范围；沿用已发布固定基线后发布 KPanel RC7。
- 阻断或移除的依赖范围（无则写“不适用”）：移除 Windows 节点构建/安装链路依赖与 IronRDP；其他范围不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Windows enrol/report 拒绝、旧记录离线、Linux 平台持久化与首报告前中继回归测试；候选 CI、L3 全部通过。 | 未对真实 Windows 主机进行节点/RDP 实测；本版已移除该功能。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 未发现可达漏洞；npm audit 为 0 vulnerabilities；Trivy source/dependency/secret/config 与 Release image scans 无发现；runtime contract、SBOM 与 provenance 步骤通过。 | govulncheck 同时提示 1 个 required module advisory，但没有发现代码调用受影响路径；未另行执行 CF scoped/full audit，coverage decision=`ok` 且无新增边界包。 |
| 稳定性、失败恢复与兼容 | 已验证 | 全部 web tests、Go 变更验证及 Linux 启动回归测试通过；L3 通过。 | 未执行真实宿主升级/重启/回滚或长时 soak。 |
| 性能与资源预算 | 不适用 | 本版撤除 Windows 远程功能并修复 Linux 启动，不新增常驻服务或性能路径。 | 未进行性能 soak。 |
| 用户体验与可访问性 | 已验证 | web 254 个测试文件通过，`npm run build` 通过；删除 Windows 专属旧提示。 | 未完成 100%/125%/200% 缩放、完整键盘/焦点和手工屏幕阅读器矩阵。 |
| 数据、配置与迁移 | 已验证 | 节点记录 `Platform` JSON 持久化、重开 store 和 legacy unknown 状态回归测试通过；无数据库 schema 迁移。 | 真实存量 Windows 记录在用户宿主上的升级状态未手工验证；预期由本版显示离线且不可管理。 |

## 自动门禁

- 定向测试及结果：`go test ./internal/cluster -count=1` 通过；Windows 节点禁用 enrollment 定向测试通过；web 全集 254 个文件、2312 passed、6 skipped（共 2318）；`npm run build`、`npm audit --audit-level=high`（0 vulnerabilities）、版本一致性、release channel contract 6/6、`git diff --check` 均通过。
- `make verify-release` 环境和结果：固定 L3 runner 的 source/web/go/deploy lanes 与 release gate 全部通过；Linux amd64/arm64 构建、Docker 多架构镜像、运行契约、打包与恢复测试通过。完整执行日志 SHA-256：`ac48dbf86d0bc21b6018df602ac3cc4aae8766c0e26fb1afe9f6989a2f561653`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run `v1.25.0-rc.7-6c2aa2c0-l3-r2`；plan `012d4ac53c868e3792b8b32ae730549f4819a136118d754fb2ca190b1846a6fc`；remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `35d3668fa64f0f0d911fc2a0c6b0150411e90bfec35f0ca9ae0972b9af021840`；runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0`, ID `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`；终态 passed，exit 0，2026-10-06 11:45:16–12:00:06 +08:00；证据目录 `C:\GitHub\_codex-evidence\kpanel-windows-node-removal-20261006\l3-r2`（远端 `/root/kpanel-release-evidence/v1.25.0-rc.7-6c2aa2c0-l3-r2`）。
- 候选 CI：成功，run [37409538359](https://github.com/kejilion/KPanel/actions/runs/37409538359)，精确 SHA `6c2aa2c0423e0a6f349af7b4ac2154db997d01de`；覆盖 change-aware、race、govulncheck、npm audit、Trivy 与脚本生命周期检查。
- 主线 CI：成功，run [37411777522](https://github.com/kejilion/KPanel/actions/runs/37411777522)，精确 SHA 同上。Dependency Freshness [37411777561](https://github.com/kejilion/KPanel/actions/runs/37411777561) 成功；security-advisories job skipped。
- Release workflow：成功，run [37412718747](https://github.com/kejilion/KPanel/actions/runs/37412718747)，2026-10-06 12:13:55–12:27:26 +08:00。GitHub release publish、multi-arch image、channel promotion 均成功；仅 stable 专用 candidate-archive step skipped。
- 安全扫描、镜像契约、SBOM/provenance：候选/主线/L3/Release scans 全部通过；Release `Scan native release image`、`Verify runtime image contract`、`Build and push multi-architecture image` 与 `Promote image to its release channel` 成功，provenance mode=max、SBOM enabled。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：未记录；Dependency Freshness workflow 在最终 SHA 上成功，安装锁定依赖通过，security-advisories job skipped。
- 最近每日安全通告审计、EOL 复核状态及证据：本记录未取得每日审计和 EOL 专项结果，记为未记录；不据此推断没有待办。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：未记录；本次唯一锁文件安全修复见下一项。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：`web/package-lock.json` 将 `source-map-js` 从 `1.2.1` 更新到 `1.2.2`，以通过 `npm audit --audit-level=high`；`package.json` 依赖集合未改。Go 1.27.1、Node 24.21.0；无新 Dockerfile/基础镜像候选。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本与 Web lockfile 为 `1.25.0-rc.7`；Release workflow 使用固定 Action SHA；OCI index `sha256:4c06a4a4c8e1a4654e5bc1771952ffe440c71d05152ab362c3bb821d0f10211d`；脚本 revision 与 SHA 见跨仓库联动字段。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：没有其他新依赖候选；无新增延期依赖行动项可据现有 evidence 确认。每日审计/EOL 待办在稳定版前由发布负责人依最新报告复核。
- 升级后的兼容、安全、构建、性能资源和回滚结论：锁文件审计为 0 vulnerabilities；Go/npm 多架构构建及 Release gate 通过。未测真实宿主性能；退出预览可切回 stable 来源，不自动安装较低版本。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：固定镜像 `kpanel-go127-prep-runner:go1.27.1-node24.21.0`；Runner ID `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`。底层宿主发行版未记录；L3 同时构建 Linux amd64 与 arm64。
- 环境策略 ID 与允许用途：`arena-154`，本次限 `candidate-validation`；没有使用生产环境。
- 使用的精确候选或公开产物：`6c2aa2c0423e0a6f349af7b4ac2154db997d01de`；发布后为 `v1.25.0-rc.7` 及 OCI digest `sha256:4c06a4a4c8e1a4654e5bc1771952ffe440c71d05152ab362c3bb821d0f10211d`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 ID `v1.25.0-rc.7-6c2aa2c0-l3-r2`；passed，exit 0；超时配置/命令规格未单列；计划与脚本 SHA、证据位置见“自动门禁”。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次固定 L3 运行 14 分 50 秒；无 soak。预览版本由用户主动选择，未改变稳定默认源。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动 web tests/build 覆盖 Windows 节点 UI 删除后的断言、Linux 节点路径及本地化文案；完整手工缩放/键盘/焦点矩阵未执行，未记录屏幕阅读器结果。
- 宿主机写入、失败注入、重启恢复和回滚结果：只在隔离 runner 执行候选容器运行契约、构建与恢复测试；没有连接或写入生产主机，没有真实宿主更新/重启/回滚。
- 未执行场景及原因：Windows 节点与 RDP 在 RC7 中被移除，未运行 Windows 节点真机验收；用户明确表示真机测试后续自行执行。长期负载 soak 和完整人工可访问性矩阵未执行。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[`v1.25.0-rc.7`](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.7) 已发布，draft=false、prerelease=true、非 Latest；GitHub Latest 仍为 `v1.24.0`。
- Docker 版本与通道 OCI index：`docker.io/kjlion/kejilion-panel:1.25.0-rc.7` 与 `:preview` 均为 `sha256:4c06a4a4c8e1a4654e5bc1771952ffe440c71d05152ab362c3bb821d0f10211d`；`:latest` 仍为稳定版 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:89d50f76ce3b52186d205efcaf3fab07ae4f413959895ece26e4fb2a8530641c`；arm64 `sha256:a05327d06badae69a0010ffc4224a34d3c7ef57fec9deee0c216553a7fedc3eb`。
- 附件及 `SHA256SUMS`：14 个资产。包括 Linux Agent 与 Linux Node 各 amd64/arm64、MCP 各平台二进制、面板元数据归档、LICENSE、THIRD_PARTY_NOTICES.md、SHA256SUMS；无 Windows Node Agent、bootstrap 或 installer。公开 `SHA256SUMS` 为 1023 bytes，SHA-256 `67771943c4dc5738a19115c8902d9d0ffd515822799d0cba3401d992acff0da0`，校验表含 11 个 payload 摘要。
- 公开镜像 `image_e2e=pass`：Release runtime image contract 与 digest promotion 均通过；实时 Registry 检查确认版本 tag 与 `preview` OCI index 相同，`latest` 未变。
- `kejilion/apps` / `kejilion.sh` 契约结论：`not-required`；没有改应用市场默认 stable 来源或 `kejilion/sh`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：release channel contract 6/6 通过；GitHub Latest 仍为 `v1.24.0`，RC7 使用官方预览 OCI digest。真实宿主更新未执行。
- 加入预览只切换来源并立即检查，没有自动安装：现有契约测试通过；本次没有操作生产节点。
- 自动安装开关与一次性立即安装相互独立：现有契约测试通过；本次未触发安装。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：既有 release-channel tests 通过；没有真实主机重启验证。
- 退出预览且稳定版较低时没有产生降级候选：既有契约保持“退出预览只切换稳定来源，不自动降级”；未执行真实宿主降级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：既有自动测试继续通过；本次没有真实系统更新或回滚。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：是；Linux 轻节点支持保留，Windows 节点不再纳入本版。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（仅授权并执行预览发布；未授权生产部署）。
- 验证/灰度环境：`arena-154`，仅 `candidate-validation`。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用；未进行生产部署。
- 部署命令/入口：不适用；未进行生产部署。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用；未进行生产部署。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：候选 L3 固定 runner、临时容器运行契约、Linux 双架构构建和发布镜像校验。

## 回滚

- 源码/tag：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；RC6/RC7 tags 均不可变。
- 镜像 digest：稳定 `latest` OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。
- 数据/配置备份：无生产数据或配置写入，无需本次生产回滚备份。
- 回滚步骤和回滚后复核：预览用户可退出预览并切换回 stable 来源；退出预览不会自动降级已安装版本。真实主机回滚未执行。
- 回滚后生产实际版本与健康状态：不适用；本次没有生产部署或生产回滚。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest=`v1.24.0`；Docker `latest`=`sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；默认更新入口仍为 stable。
- 公共默认更新通道决策：不适用（预览版不改变默认 stable 通道）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-06T11:09:50+08:00
- 候选冻结时间：2026-10-06T11:43:37+08:00
- 生产完成时间：未验证
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（RC7 Release 一次成功；无生产回滚或热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/powershell-guard/empty-status-null",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次本地标签预检将干净 git status 的空输出直接调用 Trim，命令异常退出；标签未创建、远端未发生写入。",
    "recoveryEvidence": "修正为空安全数组接收并检查 Count 后，重新核对 HEAD、工作树、main/candidate 远端 SHA 与 RC7 tag 缺失，再成功推送 v1.25.0-rc.7；Release run https://github.com/kejilion/KPanel/actions/runs/37412718747 成功。",
    "permanentAction": "后续 PowerShell Git 发布守卫统一将输出包装为数组并按 Count 判空；标签推送前强制核验干净工作树和远端 ref。仓库 Release workflow 无需修改。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：没有删除候选 worktree 或 L3 本地证据；活动候选 worktree 和 `C:\GitHub\_codex-evidence\kpanel-windows-node-removal-20261006\l3-r2` 证据目录保留用于审阅与恢复，净释放 0 bytes；旧 RC6 worktree 保留在 RC6 tag。
- 未验证风险：无 Windows 节点/RDP 真机验证；无长期 soak、全人工可访问性矩阵或真实宿主更新/回滚；CF scoped/full audit 未执行且不得宣称通过。
- 已实现待实机准入：无 Windows 轻节点新增实现待准入；既有 Windows 节点记录在 RC7 中离线且不可管理，用户升级前应知悉。
- 不阻断本版的理由：这是主动加入的 RC 预览；Windows 节点功能本身已移除，Linux 路径与更新契约通过自动门禁；GitHub Latest、Docker `latest` 与生产均未改变。
- 后续应进入的自动门禁或专项工作流：若未来重建 Windows 节点功能，应重新评估安全边界、按项目规定触发 CF scoped audit，补独立验证与 Windows 隔离真机流程；本记录不将此设为 RC7 发布后的隐藏承诺。
