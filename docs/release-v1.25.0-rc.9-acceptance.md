# KPanel v1.25.0-rc.9 发布验收记录

日期：2026-10-07

发布级别：L3

候选提交 / 标签：`0e7972a60847fcb84aa6085e777fbe5b96959fcf` / `v1.25.0-rc.9`

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`，Docker Hub `latest` OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`

`releaseChannel`：`preview`

`releaseTrain`：`1.25.0`

候选分支与发布后处置：`release/v1.25.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：`release/v1.25.0-candidate`，`0e7972a60847fcb84aa6085e777fbe5b96959fcf`；预览版候选保留。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：Release workflow 的 “Archive published candidate branch” 为 `skipped`；没有新建归档 ref。远端 main、候选分支和 annotated tag 的 peeled target 在发布时都指向 `0e7972a60847fcb84aa6085e777fbe5b96959fcf`；tag object 为 `3598923fc47c5c3a920bec3778530977ccd62764`。L3 bundle 位于 `C:\GitHub\_release-artifacts\v1.25.0-rc.9-0e7972a-l3-r2\kpanel-v1.25.0-rc.9-0e7972a-l3-r2.bundle`，SHA-256 `74b3d9833b575a45d86b39ae58760a7afe5ce81561d463c265c3c9daf1b4e92f`；远端 L3 status 为 passed/exit 0。远端核对只发现本版这一条未归档候选分支，旧 archive refs 未改动。
- 本次来源任务分支：`feature/prune-windows-light-node-state-20261006` 的两个产品提交纳入 RC9；review trailer 提交只记录证据，不改变源码树。候选盘点遗漏了当时已存在的 `claude/desktop-mobile-icon-pager`（`548c45c0`、`110bc954`、`3d0548ee`），因此手机桌面横向分页未进入 RC9；该功能应使用新的不可变 RC 标签发布，不得改写 RC9。
- 本地分支/upstream/worktree：本地功能分支及 `C:\GitHub\_codex-tasks\kpanel-v125-rc9-residual-state` 保留，tip 为候选 SHA；`origin/main` 在发布时与候选一致。工作树无未提交改动；L3、OCR、安全审计和失败重试证据均保留。
- 未完成归档项 / 责任人 / 下次复核触发条件：无本版归档操作待办；旧候选归档和历史恢复材料保持原样。下次稳定版准入前重做 CF scoped/full audit 并关闭 `NEEDS-VALIDATION.md` 中的恢复路径验证。

归档不代表生产上线；生产状态在对应章节独立填写。无生产授权时明确“产物已发布，生产未部署”。

## 发布画像

- 业务域：集群轻量 Linux 节点上报与服务端持久状态。
- 变更面：协议或数据；没有界面、端口、Compose 或应用市场变更。
- 受影响用户旅程：KPanel 升级后打开轻节点存储时，移除可确认为 Windows 的旧轻节点运行态、报告凭据和终端公钥；Linux 节点继续上报；未知节点报告使用独立限额。
- 未变化契约：HTTP API 形状、端口、Compose、Agent 权限、Linux 轻节点协议、`kejilion.sh` 与应用市场契约不变。Windows 节点 Agent/bootstrap/installer 继续不提供；通用 Windows MCP 客户端不是 Windows 节点功能，仍保留。
- 风险等级及理由：L3。启动迁移会删除限定记录和关联密钥，并调整未知报告的限额路径；涉及持久数据、凭据和可用性边界。

## 发布范围与未纳入内容

- 用户可见更新：RC8 之后补清残留的 Windows 轻节点记录/凭据/公钥；未知节点报告重试不再消耗 Linux 节点共享公网来源额度。Linux 记录、平台信息不足的记录及已存在的历史备份保留。
- 精确提交清单：基线 `e6f3ca26e5a45ee29e7f0cd1748dc26f36096ce4`；产品提交 `53a7ca40bae792f12290502cdfc16fa8e9b074bb`、`ef8387e2ad574d45bb0ded508972555cdb4628b0`；最终源码/tag target `0e7972a60847fcb84aa6085e777fbe5b96959fcf`。基线至产品提交共 12 个文件，`+384/-12`。
- 明确未纳入的分支、文件或后续事项：手机桌面横向分页候选 `claude/desktop-mobile-icon-pager` 未纳入，原因为候选冻结盘点遗漏；包含 Windows MCP 客户端的通用功能仍保留。没有改 `kejilion/sh`、稳定更新源、Docker Hub `latest`、GitHub Latest、生产数据或旧备份；平台不明且无 Windows 遥测佐证的记录不会被此次迁移删除。已有 archive refs 未操作。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：`security-audit-run-19`，source `ef8387e2ad574d45bb0ded508972555cdb4628b0`，tree `351b39f6e5628b9dd03ea29b326400ede849a92b`，仅计划覆盖两个本版产品提交。状态 `incomplete-platform-interruption`，不能称为 scoped/full 通过或零发现。CF 专用审计子代理因平台 thread limit 未能启动；缺 OS sandbox，故采用 source-only。必要侦察、hunters、独立 finding validator、Phase-5 记录核验及固定 findings/coverage-ledger validators 未完成。报告：`C:\GitHub\_codex-evidence\kpanel-v125-rc9-residual-state\security-audit-run-19\REPORT.md`、`NEEDS-VALIDATION.md`。
- 覆盖检查（`check-security-audit-coverage.mjs` 的 decision、未审计提交数；RC 只记录。稳定版带 `--require`，非 ok 时写补审 run，或用户原话、理由与不超过 14 天的补审截止日）：精确 target checker 退出 0、`decision=ok`；本批两提交与 27 个范围外 pending commits 区分记录；最近 full audit run 4，距今 15 天。此批次/时间策略结果不等于本版安全审计完成。
- finding fingerprint / 修复 commit / 独立复核与回归证据：未生成本 run 的完整 findings verdict，不声称无 finding。独立增量复核只检查 `ef8387e2` 的 membership index 与 report-admission 直接路径，未见明显同步缺陷；不覆盖备份恢复、运行时并发及端到端验证。独立复核为 Codex/Codex `PASS WITH FOLLOW-UP`，`fallback=provider-unavailable`（没有其他模型提供商入口）。全量待核线索见 `NEEDS-VALIDATION.md`。
- 修复交付状态：源码、RC tag、GitHub prerelease、Docker Hub `preview` 均已交付；stable tag/Latest 和生产未交付。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：`e6f3ca2..ef8387e`，OCR 1.12.11 覆盖全部 10 个支持复核文件；有效发现 H0/M0/L0，free-form=0，`constrained-only=unreported`。`CHANGELOG.md` 和 `web/package-lock.json` 分别因扩展名与 user-exclude 不支持 OCR，已人工复核；不将未报告解释为 0。
- 按 PROJECT_RULES.md 5.4/5.5 记录本稳定周期的观察结果；不足三个周期不提前宣称工具有效或应退出。
- 候选分支的原始 tip、归档位置和远端核验统一填写本模板既有候选处置字段，遵守 `docs/release-channels.md`。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`c3a8bd895f8878d9e4ced7592c91a20c974472a5` / `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：不适用；与已发布基线相同。
- 状态判定依据与兼容性证据：RC9 只改 KPanel 轻节点持久状态；容器中受管脚本 revision 与 checksum 未变，managed-script contract 检查通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go 全量测试、候选 CI、L3 和 Release workflow 均通过；覆盖 Windows 记录分类清理、Linux/歧义记录保留及上报限额路径。 | 没有对生产集群写数据；不做已删除 Windows 节点功能的实机验收。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | CI/L3/Release 源码、依赖、秘密、配置、原生镜像扫描成功；govulncheck 未发现可达漏洞，npm audit 为 0。 | CF scoped audit 未完成；govulncheck 另报告 1 个所需模块漏洞但当前代码未见调用；不得表述为全依赖无漏洞。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 全量、核心 race、安装/更新/回滚失败注入、备份 parity 和 app-conf lifecycle 通过。 | 未在真实宿主做长时间 soak 或真实升级/回滚。 |
| 性能与资源预算 | 已实现未实机验证 | `HasHost` membership index 将已登记节点 admission 查找改为 O(1)；未知来源限额独立且有界。 | 没有压力/性能基准；arena-154 L3 执行期间系统盘曾到 99% 使用率，最低约 1.5 GB 可用，但本轮门禁成功完成，远端未清理。 |
| 用户体验与可访问性 | 不适用 | 本 RC 无 UI 改动。 | 没有执行浏览器、键盘、缩放或屏幕阅读器矩阵。 |
| 数据、配置与迁移 | 已实现未实机验证 | 启动迁移仅删除明确 Windows，或缺失 Platform 且遥测确认 Windows 的运行态；持久写入失败和模拟中断恢复测试通过；历史备份不改写。 | 旧备份恢复及跨更新完整恢复链未由本次中断的 CF audit 独立验证。迁移删除项如需恢复，必须使用升级前备份。 |

## 自动门禁

- 定向测试及结果：版本一致性、治理一致性、`scripts/ocr-delegate` 单测 4/4、dependency-freshness validate-only 11 组、环境策略和 collaboration-state 检查通过。工作站没有 Go/gofmt/make，故本地 L2 preflight 未运行代码测试；已由完整 L3 验证取代。
- `make verify-release` 环境和结果：`arena-154` candidate-validation；Go 1.27.1 / Node 24.21.0；L3 `v1.25.0-rc.9-0e7972a-l3-r2` passed/exit 0。全量 Go 与 source lanes 通过；`go test -race ./internal/panel ./internal/auth ./internal/dockerx` 通过；Web lane、typecheck、npm test、deploy lane、scene-pack tests 9/9 通过；双架构构建通过；`app_conf_update_backup_parity=pass`。日志记录 `govulncheck` 0 可达漏洞、1 个未见调用的 required-module 漏洞；npm audit 0；L3 Trivy 源码及最终 Agent 二进制扫描无报告漏洞。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：唯一成功 run `v1.25.0-rc.9-0e7972a-l3-r2`，2026-10-06T15:27:18Z 至 15:42:19Z；Runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`；bundle `74b3d9833b575a45d86b39ae58760a7afe5ce81561d463c265c3c9daf1b4e92f`；plan `41c49f56e32c930226de933ac1df04be5f6a3a69ce63f9e29c419aca3a5ef4f6`；remote script SHA-256 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- 候选 CI：CI run [37486504716](https://github.com/kejilion/KPanel/actions/runs/37486504716) success；Dependency freshness run [37486504973](https://github.com/kejilion/KPanel/actions/runs/37486504973) success；均绑定候选 SHA。
- 主线 CI：[37490577889](https://github.com/kejilion/KPanel/actions/runs/37490577889) success；Dependency freshness [37490577987](https://github.com/kejilion/KPanel/actions/runs/37490577987) success；均绑定候选 SHA。
- Release workflow：[37491972862](https://github.com/kejilion/KPanel/actions/runs/37491972862) success，所有步骤完成；tag Dependency freshness [37491972733](https://github.com/kejilion/KPanel/actions/runs/37491972733) success。
- 安全扫描、镜像契约、SBOM/provenance：Release workflow 的漏洞、Node、源码/配置/秘密扫描、native image scan 与 `Verify runtime image contract` 均成功；多架构镜像构建和通道 promotion 成功。没有单独运行生产 E2E。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：未单独生成新的依赖建议报告；validate-only 11 组和候选/main/tag 三次 Dependency freshness workflow 均成功。
- 最近每日安全通告审计、EOL 复核状态及证据：本 RC 未单独重跑每日审计/EOL 全量复核，记为未验证；govulncheck 输出见 L3 日志。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本 RC 未升级依赖；CI/Release 检查通过不重置已有行动项期限。govulncheck 的 1 个未见调用模块通告仍需按依赖治理流程跟踪。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无依赖或 Action 版本升级；发布 Runner 使用冻结的 Go 1.27.1 / Node 24.21.0；Trivy 0.74.0；应用构建使用仓库现有固定基础镜像摘要；受管脚本保持基线。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：产品版本 `1.25.0-rc.9`；Go module 与依赖版本未变；`web/package.json`/lockfile 仅更新版本；`kejilion.sh` commit/SHA 同跨仓库章节。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无新依赖候选；未将 govulncheck 未见调用的模块漏洞解释为可忽略，待维护者在后续依赖评估中复核。
- 升级后的兼容、安全、构建、性能资源和回滚结论：L3 和 Release 完整构建通过；真实主机性能、恢复链和 soak 未验证；恢复已删除节点数据需升级前备份。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154` Linux amd64；冻结 Runner Go 1.27.1、Node 24.21.0。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未使用 `prod-108`。
- 使用的精确候选或公开产物：源码/tag target `0e7972a60847fcb84aa6085e777fbe5b96959fcf`，预览 Release `v1.25.0-rc.9`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.25.0-rc.9-0e7972a-l3-r2` passed/exit 0；证据目录 `C:\GitHub\_release-artifacts\v1.25.0-rc.9-0e7972a-l3-r2\remote-evidence`，`status.txt` 完成时间 `2026-10-06T15:42:19Z`，`evidence.sha256` 12 项本地复核全匹配。无浏览器后台作业。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次 L3；未做长期 soak。RC 为预览版。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：不适用；本版没有 UI 变化。
- 宿主机写入、失败注入、重启恢复和回滚结果：失败注入、启动迁移、L3 隔离应用生命周期恢复和备份 parity 自动测试通过；未向用户真实/生产主机写入数据。
- 未执行场景及原因：真实 Windows/RDP 节点测试不适用，RC9 已移除 KPanel Windows 节点功能；真实 Linux 集群升级、重启、回滚、长时间运行和浏览器交互由后续使用者验收。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：`KPanel 1.25.0-rc.9`，draft=false、prerelease=true，2026-10-07T00:10:40+08:00 发布；GitHub Latest 仍为稳定版 `v1.24.0`。
- Docker 版本与通道 OCI index：`docker.io/kjlion/kejilion-panel:1.25.0-rc.9` 与 `:preview` 均为 `sha256:a1749acafd945fed1a728bf4f8a2a4c0da35f25edb6cbbe30c42029c745eadf7`；稳定 `:latest` 仍为 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`（发布前后相同）。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:27679c70e8e2aa8e6a428ace2b601d7a4e916a9df808406d2acd283bbaa251e8`；arm64 `sha256:7b8750939472574bea301b5c3ba159fd6c6f2ea263700c3cba7aa2f1d70a0237`。
- 附件及 `SHA256SUMS`：GitHub Release 共 14 项，包含 Linux Agent/轻节点 amd64/arm64、Darwin/Linux/Windows MCP 客户端、部署元数据 tarball、LICENSE、THIRD_PARTY_NOTICES 和 `SHA256SUMS`。校验文件 digest `sha256:3191d294dd7317ceb279115367e14ef7516c499bde158eb0b921bd8d04c7003d`；其 11 个程序/元数据 payload hash 均与 GitHub API asset digest 匹配。LICENSE 和 notices 不在清单范围。没有 Windows 节点 Agent/bootstrap/installer；Windows MCP 程序是通用客户端。
- 公开镜像 `image_e2e=pass`：Release workflow native runtime image contract 成功；推送后的版本 tag 与 `preview` OCI index 相同且同时包含 amd64/arm64。没有单独的真实主机 image E2E。
- `kejilion/apps` / `kejilion.sh` 契约结论：未改应用市场清单和脚本；managed-script contract revision 与 checksum 校验通过。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：实现约定未变；公开 Release 与 Docker Hub digest 已独立核对。
- 加入预览只切换来源并立即检查，没有自动安装：本版未改变通道交互，也未做真实浏览器操作。
- 自动安装开关与一次性立即安装相互独立：契约未改；由 L3 安装/更新路径自动测试覆盖。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：本版没有改此代码；未做真实宿主重启。
- 退出预览且稳定版较低时没有产生降级候选：默认/稳定通道未改变；没有部署操作。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：L3 lifecycle 与 backup parity 测试通过；无真实宿主更新。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：Linux 轻节点保留；Windows 节点不在 RC9 支持范围。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`，仅 candidate-validation。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：不适用（未部署）。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用（未执行生产部署）。
- 部署命令/入口：不适用（未执行生产部署）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：未验证（未执行生产部署）。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3 构建、扫描、安装/恢复失败注入和容器生命周期测试。

## 回滚

- 源码/tag：回滚代码基线 `v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；RC9 源码 tag `v1.25.0-rc.9` 固定指向 `0e7972a60847fcb84aa6085e777fbe5b96959fcf`。
- 镜像 digest：稳定回滚镜像 `docker.io/kjlion/kejilion-panel@sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；RC9 preview index `sha256:a1749acafd945fed1a728bf4f8a2a4c0da35f25edb6cbbe30c42029c745eadf7`。
- 数据/配置备份：迁移前的 panel 数据备份是恢复被清除轻节点状态/凭据的依据；不要只切回旧镜像并假定被清除数据自动复原。原有历史备份未改写。
- 回滚步骤和回滚后复核：按应用市场支持的备份恢复流程恢复升级前数据，再选择稳定来源；回滚时校验稳定镜像 digest、服务健康、Linux 节点报告与凭据。旧备份导入/更新中断完整链路未通过本轮独立 CF audit。
- 回滚后生产实际版本与健康状态：不适用（未进行生产操作）。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest=`v1.24.0`；Docker `latest` 保持原 OCI index；RC9 仅进入 preview 通道。
- 公共默认更新通道决策：短期保留既有 stable 默认通道；preview 仅供主动加入预览的用户选择，不自动安装、不触碰 stable/latest。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-06T22:44:32+08:00
- 候选冻结时间：2026-10-06T23:15:12+08:00
- 生产完成时间：未验证
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（v1.25.0-rc.9 GitHub prerelease 和 preview 镜像各发布一次；没有生产操作）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "validation/windows-l2/missing-go-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "工作站上的 L2 preflight 因缺少 go、gofmt、make 被阻断，未在工作站执行产品测试。",
    "recoveryEvidence": "后续 frozen Runner 的 L3 `source_lane=go`、完整 release suite 和候选/main CI 均通过；task-contract handoff receipt 指向精确候选及 L3 日志。",
    "permanentAction": "本机工具缺失时保留 fail-fast；L3 使用版本与摘要已登记的 frozen Runner，不绕过工具链前置条件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "ocr/powershell/process-substitution-unsupported",
    "position": "before-production-write",
    "count": 1,
    "impact": "OCR workflow 的首次 PowerShell 调用无法解析 bash process substitution，未生成 OCR 结果。",
    "recoveryEvidence": "以显式 UTF-8 文件输入重跑；外部 OCR evidence 的 preview/rules/coverage/free-form 文件齐全，10/10 reviewable files 已覆盖。",
    "permanentAction": "在 Windows PowerShell 调用 OCR 时使用已落盘输入文件，不使用进程替换语法。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/orchestrator/runner-reference-mismatch",
    "position": "before-production-write",
    "count": 2,
    "impact": "一次 L3 invocation 因 SHA256 runner-id 参数格式错误在本地 fail-closed；随后 r1 使用错误/不可用的 Runner image tag，在 arena-154 的 runner inspect 阶段失败。两次均未运行候选测试。",
    "recoveryEvidence": "r1 远端 `status.txt` 记录 failed/exit 1 及 `No such image`；核对 RC8 原始 manifest 后，以 Runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c` 重新运行 r2，status passed/exit 0，12 项远端 evidence hashes 全部通过。",
    "permanentAction": "冻结 L3 前从最近成功验收 manifest 同时复制 image name 与 immutable image ID，并在传输前校验 ID 的完整 64 位格式。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

“流程异常或无效证据”与产品缺陷、生产回滚分别计数。以上所有异常都发生在生产写操作前；candidate CI、main CI、L3 和 Release workflow 最终均以精确 SHA `0e7972a60847fcb84aa6085e777fbe5b96959fcf` 成功。

### 流程异常明细

L3 r1 的远端状态为 failed/exit 1，原因是候选控制端提供了不存在的 Runner tag；应用/Go/web 测试尚未启动。RC8 acceptance 原始 manifest 确认正确 Runner 后，L3 r2 通过完整门禁。该失败没有逃逸到 tag、GitHub Release、Docker Hub 或生产。

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：未删除候选 worktree、远端候选分支、L3 r1/r2 证据或安全审计/OCR 证据；净释放 0 bytes。L3 source-preparation 临时目录由入口清理（`source-prepare.json`=`cleanup: removed`，释放字节未测）；`arena-154` 执行时根盘最低约 1.5 GB 可用，本轮未清理远端共享文件。
- 未验证风险：CF scoped audit 未完成，未形成 findings verdict；备份导入/回滚恢复链、生产升级、真实主机长稳和性能 soak 未验证；govulncheck 的 1 个未见调用模块漏洞待依赖治理复核。
- 已实现待实机准入：启动清除旧 Windows 轻节点持久态、凭据与终端公钥；Linux/歧义记录保留；未知节点上报额度与已知 Linux 来源额度隔离。真实数据备份和恢复须按 RC9 Release 指引操作。
- 不阻断本版的理由：这是用户主动选择的 preview RC；候选 CI、main CI、L3、Release workflow 和公开产物 digest 检查均对同一源 SHA 成功；稳定 Latest、Docker latest 和生产均未变。CF audit 中断已显式标记，不能作为稳定版审计通过依据。
- 后续应进入的自动门禁或专项工作流：稳定版准入前以符合 AGENTS 要求的 `gpt-6-luna/max` 专用审计代理完成 scoped/full CF audit；完成独立验证与固定 validators；重点验证旧备份导入/恢复时记录、凭据和 membership index 的一致性。
