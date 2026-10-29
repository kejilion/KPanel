# KPanel v1.28.0-rc.14 发布验收记录

日期：2026-10-29（Asia/Shanghai）。发布级别：L3；releaseChannel=preview，releaseTrain=1.28.0。公开预览版已发布，生产未部署。

候选提交 / 标签：`c928683951d07923d81ab206d9c5d65d058ade33` / `v1.28.0-rc.14`

上一稳定版本 / 回滚点：`v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`

候选分支与发布后处置：`release/v1.28.0-candidate` / 预览候选分支保留

- 原分支 / 精确 tip / 处置分类：`release/v1.28.0-candidate` / `c928683951d07923d81ab206d9c5d65d058ade33` / 保留；发布时 `origin/main`、候选分支和注释 tag 均绑定该 SHA。
- 归档 ref 与 SHA（或历史 tag/bundle 恢复证据）/ 远端复核结果：RC14 为注释 tag，tag object `4ffd509a0d7b809d9bd156d58da538b3b35df9b2` 指向精确候选 SHA；`origin/release/v1.28.0-candidate` 保留同一 tip。验收文档随后作为独立提交追加到 main，不改写候选、tag 或镜像。
- 本次来源任务分支：`fix/terminal-follow-recovery-20261028` tip `5761f6df`、`feature/terminal-keyword-colors` tip `91936840`、`perf/browser-pipeline` tip `0f297d51`、`codex/waf-migration-compat-20261028` tip `cc2eb7e4` 均包含于候选历史。`feature/browser-vnc-prototype` tip `29c160bb` 未作为分支祖先合入；其隐藏远端光标的 `-nocursor` 行为已由 `perf/browser-pipeline` 的 `2975eee` 进入候选源码。网站防护基础视图来自上一预览已发布的 `codex/site-defense-monitor-20261028` tip `8f40bd5ec9c820aaf9dc0c99c6644d3fdf93c2f8`。
- 本地分支/upstream/worktree：候选 worktree `C:\GitHub\_release\kpanel-v128`，分支干净，upstream 为 `origin/release/v1.28.0-candidate`，发布 tip 与远端一致。来源 worktree 未删除或归档。
- 未完成归档项 / 责任人 / 下次复核触发条件：候选和来源工作树均保留；由 KPanel 维护者在下一个候选列车整理时复核去留。本次未执行本地资源回收。

归档不代表生产上线；本版生产未部署。

## 发布画像

- 业务域：远程浏览器/VNC 交互、终端输出着色与跟随恢复、网站防护迁移状态与 CF 凭据兼容。
- 变更面：展示、交互、增量 API 状态字段及既有显式主机防护操作；没有自动开启防护或自动安装更新。
- 受影响用户旅程：远程浏览器切换与滚动、首个画面就绪后输入、终端输出与历史、网站安全状态与旧 WAF/CF 迁移。
- 未变化契约：没有数据库 schema 迁移；Docker Compose、端口、Agent 权限边界和 `kejilion/apps` 契约未变。镜像内 `kejilion.sh` 固定到本版已验证提交及摘要。
- 风险等级及理由：中等。改动涉及远程交互、迁移兼容和主机防护状态；真实主机 VNC、WAF/Fail2Ban 数据源仍需在受控主机环境另行验证。

## 发布范围与未纳入内容

- 用户可见更新：终端普通输出突出状态词、IP、端口与路径并保留 URL 颜色；浏览器保持显示连接、按当前画面控制输入、保留滚动残量、重试背压中的滚轮输入并隐藏远端光标；网站防护页显示迁移状态、Jail 计数和近期日志；缺失封禁 owner 状态时 CF/WAF 清理和卸载 fail closed。
- 精确提交清单：候选精确范围为 `606b25c311484512acec434766c831550c94a9ac..c928683951d07923d81ab206d9c5d65d058ade33`，共 44 个提交、35 个净变更文件（1066 行新增、112 行删除）。核心来源提交：`2975eee` 浏览器交互路径、`26dae426` 画面就绪与滚动残量、`2643be9f` 背压下重试滚轮输入、`0dd574c1` 终端颜色集成、`d6608042` WAF 迁移兼容合并、`27808b82` WAF 清理边界、`bebfd66d` 浏览器输入限速，以及发布组装提交 `c9286839`。
- 明确未纳入的分支、文件或后续事项：`perf/browser-interaction` 的旧截图流水线未合入，避免回退新画面就绪机制。`feature/browser-vnc-prototype` 原分支提交未按祖先合并，但 `-nocursor` 行为在最终 `internal/webbrowser/display.go` 中；该原分支不应被误报为完整历史合入。真实主机 VNC 流未由公开升级验收覆盖。

## 外部审计与修复交付

安全盲审按 `PROJECT_RULES.md` 5.4 记录；自动扫描与 OCR 结果不替代独立安全审计。

- 安全盲审补漏（quality-audit review 档）：not-run；本次未执行独立 quality-audit review 或 CF `security-boundary-audit`。
- `node scripts/security-review-scope.mjs --base v1.27.2` 输出：`security_review=recorded groups=68 invalid_trailer=true`；没有 missing/stale 组。工具提示既有记录中含旧式 trailer；不把它解释为独立审计通过。
- finding fingerprint / 修复 commit / 独立复核与回归证据：候选 OCR 发现并修复两个 LOW 问题：滚轮背压重试 `2643be9f`、连续浏览器输入限速 `bebfd66d`。最终候选输入校验发现本地模拟 API 文件改动会违反 main 的可信发布工具树约束；将 `scripts/mock-app-market-api.mjs` 恢复到 main 基线并以 `c9286839` 冻结，产品功能代码未因此删改；新的候选输入检查通过。
- 修复交付状态：终端、浏览器及 WAF 兼容修复已进入 RC14 与公开 prerelease；生产未部署。配套 `kejilion.sh` 已先发布到其权威 main。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：OCR-Review `1.12.11` 精确记录为 `range=606b25c..77fbac4 files=32/32 free-form=2 valid=H0/M0/L2 constrained-only=unreported`。该评审检查了最终产品代码及更丰富的本地 mock 数据；`c9286839` 随后仅恢复该 mock 文件到可信 main 基线，最终净产品变更为 35 个文件。约束臂差异未报告，不作工具有效性结论。
- 盲审观察记录：未执行；本版不宣称 CF 边界盲审覆盖。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`。
- 变更集编号：`waf-migration-compat-20261028`。
- KPanel 实际内置脚本基线 commit / SHA-256：RC13 `594138a9a55ab9d75282948a27fcbcf9835650ec` / `27ce9d8c34040ed30f0ffc360d388f7d9617b83a7dc9b63061812f662639aa01`。
- 脚本候选 commit / SHA-256：`kejilion/sh` `b408d60d7ed1c9a935bb1ff0d6e41b73469898ff` / 根脚本及 CN 同步脚本 `2f814354d0d0de113e44c788a2b79484eca0762293a168da1baebec8fdac312d`。
- 状态判定依据与兼容性证据：脚本和共享包已先进入公开 main；KPanel Dockerfile、OCI revision/hash labels 绑定脚本提交与字节摘要。RC14 公共发布验收回读到镜像内脚本标记；候选/L3 和 `external-config-sources.md` 记录了兼容依据。
- 本版发布决定：脚本先进入权威 main 并通过兼容门禁，再发布 KPanel 预览版。
- 阻断或移除的依赖范围：不适用；本版没有移除该配套范围。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 CI、L3、公开发布验收通过；终端/VNC/WAF 定向测试通过 | 真正宿主机 VNC 会话及实际 WAF/Fail2Ban 日志源未在生产验证 |
| 网络入侵与供应链安全 | 已验证 | Go 漏洞扫描、Node audit、源码/依赖/秘密/配置扫描、镜像扫描和运行契约通过；provenance 附件和公开摘要校验通过 | 独立 quality-audit/CF 边界审计未执行；旧 security-review trailer 提示保留 |
| 稳定性、失败恢复与兼容 | 已验证 | 候选与主线 CI、L3、Windows node 边界、公开隔离容器首次/重启验收通过 | 真实 Windows SCM 环境的长期恢复未做 soak；RC13 SCM 首次失败的根因仍未确认 |
| 性能与资源预算 | 已实现未实机验证 | 浏览器输入限速、显示连接保留及滚轮背压恢复通过代码测试和 CI | 未做真实 VNC 网络下的性能基准或长时间压力测试 |
| 用户体验与可访问性 | 已验证 | 公开真实浏览器发布旅程 45 项、升级旅程 31 项通过，页面错误为 0；两组报告均 `mocked=false` | 浏览器验收聚焦安装/升级/通道/终端流程，不等同真实远程 VNC 体验验收 |
| 数据、配置与迁移 | 已实现未实机验证 | API 增量字段测试、发布容器初始/重启检查通过；无数据库迁移 | 旧 WAF/Jail/CF 凭据迁移须在真实配置主机验证；旧 CF 凭据需用户重录 |

## 自动门禁

- 定向测试及结果：变更代码的前端 59 tests / 6 files 通过、`npm run typecheck` 通过；Go `go test ./internal/webbrowser ./internal/webenv` 通过。最终 `c9286839` 的 push CI、Publish preview 候选 CI和主线 CI均成功。
- `make verify-release` 环境和结果：本机缺少可用的 `make`，本地未能运行完整 L2；GitHub-hosted L3 对最终 SHA 运行权威 `make verify-release` 入口并通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：Publish preview run `40793608653` attempt 1，L3 job `123184642571` success；receipt candidate=`c928683951d07923d81ab206d9c5d65d058ade33`，runner `ubuntu24` / `20261026.354.1`，Go 1.27.3、Node 24.22.1、Docker 28.0.4，`entrySha256=43e57f4fd321846204020d6cf27eb75961f0aefa1284a9187e786601704c2d40`。receipt 未提供 plan/bundle SHA-256 或不可变 Runner ID，不补造。证据目录：`C:\GitHub\_release-evidence\kpanel-v128-rc14-run-40793608653`。
- 候选 CI：最终 push CI [40793020315](https://github.com/kejilion/KPanel/actions/runs/40793020315) 精确绑定候选 SHA 并成功；Publish preview run 内 candidate-ci verify/Windows node 检查也成功。
- 主线 CI：[Publish preview 40793608653](https://github.com/kejilion/KPanel/actions/runs/40793608653) 内 main-ci verify 与 Windows node 检查均成功；随后 `promote` 原子快进 main 并创建注释 tag。
- Release workflow：[Publish preview 40793608653](https://github.com/kejilion/KPanel/actions/runs/40793608653) attempt 1 成功；包括发布输入与说明、源码和安全扫描、双架构构建、provenance、冻结 19 个附件、提升 preview 通道、公开 GitHub prerelease。
- 发布说明门禁：最终正文通过；目标 `1.28.0-rc.14`，5 条摘要、1 条升级提示。提示旧 CF 凭据需重新录入、加入预览不自动安装、退出预览不自动降级，稳定版为 `1.27.2`。
- 安全扫描、镜像契约、SBOM/provenance：Go call-path、npm audit、Trivy 源码及镜像扫描、runtime image contract 通过；镜像生成 SBOM/provenance，`KPANEL_MODIFIED_COMPONENTS.json` 上传且公开校验通过。

非阻断报告：Dependency freshness run `40792104204` 对前一候选 SHA `bebfd66d` 失败，指出 TypeScript `6.0.3` 与 `@types/node 24.19.0` 的升级决策复核日期已过（2026-10-23），并提示 EOL review 逾期。RC14 未变更这些依赖版本；该独立报告不属于 Publish preview 阻断门，仍需 KPanel base maintenance 后续复核。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：未单独运行；release workflow 的依赖审计通过。
- 最近每日安全通告审计、EOL 复核状态及证据：独立 Dependency freshness run `40792104204` 在 mock-fixture 回退前的 `bebfd66d` 报告两项逾期决策；最终候选只改变本地 mock 文件，依赖树相同。本版未解除该维护事项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：TypeScript 和 `@types/node` 的例外由 KPanel base maintenance 维护者复核；本候选未升级。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go 1.27.3、Node 24.22.1、Docker 28.0.4；KPanel 镜像固定 `kejilion/sh@b408d60d7ed1c9a935bb1ff0d6e41b73469898ff`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：应用版本字段升级到 RC14；所有 19 个 Release 资产有冻结摘要，`SHA256SUMS` 覆盖 15 个文件。镜像和脚本摘要详见下方公开复核。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：未采纳自动依赖升级；待 KPanel base maintenance 复核上述两个逾期决策。
- 升级后的兼容、安全、构建、性能资源和回滚结论：发布构建、审计及镜像契约通过；无独立性能基准。必要时回退到 RC13 固定镜像摘要。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted Ubuntu 24、Docker 28.0.4；精确 RC14 公共多架构镜像。
- 环境策略 ID 与允许用途：`release-public-verify` 隔离发布验收；无生产主机访问，不挂载宿主 Docker/systemd socket。
- 使用的精确候选或公开产物：`c928683951d07923d81ab206d9c5d65d058ade33`；镜像 `sha256:5e6fe52fd2382579879bfde1f3efaa6330ea2de2aa1a3da8eda95ca732d9ded2`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：public-acceptance job `123198867662` success；published browser 45 项、upgrade 31 项，errors=0；隔离容器 initial/restart 48/26 项，errors=0；所有报告均 `status=passed`、`mocked=false`。证据目录见自动门禁段落，GitHub artifact `public-40793608653-1`。
- 测试窗口/循环数及风险依据：一次公开浏览器验收和一次隔离容器初始/重启验收，不是 soak 测试。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：公开验收覆盖真实浏览器登录、终端/PTTY 延续、更新提示、stable/preview 通道选择、拒绝自动安装和退出预览；页面错误为 0。该工作流没有声称覆盖真实 VNC 屏幕流的响应性能。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离容器验收通过；使用项目定义的 Docker 库存与测试 CA 夹具，不等同真实生产主机的 systemd/证书/宿主写入验证。
- 未执行场景及原因：真实 VNC 目标、WAF/Fail2Ban 主机日志源、CF 凭据重录及生产安装/重启未执行；本版为预览，不连接生产。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：`1.28.0-rc.14` 与 digest 匹配；正文 5 条更新摘要、1 条升级提示；公开接口和真实浏览器读取官方 Release URL，`cached=false`、`stale=false`。
- GitHub Release 的 draft / prerelease / Latest 状态：[v1.28.0-rc.14](https://github.com/kejilion/KPanel/releases/tag/v1.28.0-rc.14) 为非 draft、`prerelease=true`；GitHub Latest 仍为 `v1.27.2`。
- Docker 版本与通道 OCI index：`docker.io/kjlion/kejilion-panel:1.28.0-rc.14` 和 `:preview` 均为 `sha256:5e6fe52fd2382579879bfde1f3efaa6330ea2de2aa1a3da8eda95ca732d9ded2`；`:latest` 保持 `v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:c759f75372d0da8ff92f12bac4c9d986a8294e4197b9d0b2eb6080c1076df76e`；arm64 `sha256:68efa5a09049eb5bd169f24b054ee50e7ddac0dd295967b98635a1a29bc08ebe`。
- 附件及 `SHA256SUMS`：Release 共 19 个附件；`SHA256SUMS` 覆盖 15 个文件；19 个资产的公开 digest 与发布冻结清单一致。
- 公开镜像 `image_e2e=pass`：公开 Release 身份、多架构 digest、镜像标签/版本/source revision、附件下载与摘要、真实浏览器和隔离容器回读均通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：应用市场契约和受管脚本检查通过；镜像 revision/hash 为 `b408d60d7ed1c9a935bb1ff0d6e41b73469898ff` / `2f814354d0d0de113e44c788a2b79484eca0762293a168da1baebec8fdac312d`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：报告验证 stable=`v1.27.2`、preview=`v1.28.0-rc.14`，公开 digest 匹配。
- 加入预览只切换来源并立即检查，没有自动安装：公开浏览器验收通过。
- 自动安装开关与一次性立即安装相互独立：默认 auto-install 关闭；加入预览没有启用自动安装或触发安装。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：隔离容器 initial/restart 检查通过。
- 退出预览且稳定版较低时没有产生降级候选：公开浏览器和隔离容器检查通过；退出 preview 不自动降级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：隔离验收及 L3 通过；未将容器夹具解释为真实生产 systemd 主机验证。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：本 RC 不扩展 OpenRC 自更新承诺；轻量 Node 继续按现有安装/更新边界使用。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用，预览版禁止生产部署。
- 验证/灰度环境：不适用；只使用 GitHub-hosted 隔离环境。
- 正式部署环境：不适用；未执行生产发布。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：本文件记录的浏览器与容器验收均在隔离环境执行。

## 回滚

- 源码/tag：回退到 `v1.28.0-rc.13`；其 preview digest 为 `sha256:afb71c77a0ed9bb9dc6e01ecd0feba2d829d53c8f163c66cead1ccfad4f310d6`。
- 镜像 digest：RC14 `sha256:5e6fe52fd2382579879bfde1f3efaa6330ea2de2aa1a3da8eda95ca732d9ded2`；回滚时固定使用 RC13 摘要，不覆盖不可变镜像。
- 数据/配置备份：没有生产数据或配置写入，因此无生产备份。
- 回滚步骤和回滚后复核：在更新来源中退出 preview 并检查稳定来源；退出预览不会自动安装较低版本。如需手动恢复，使用 RC13 固定 digest 并核对镜像标签与健康状态。
- 回滚后生产实际版本与健康状态：不适用，未部署生产。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：保持 `v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`。
- 公共默认更新通道决策：稳定默认保持 `v1.27.2`；preview 提供 RC14，用户需主动加入，不自动安装或降级。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-28T16:46:09+08:00
- 候选冻结时间：2026-10-29T08:16:33+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

### 流程异常明细

RC14 Publish preview attempt 1 的输入、候选 CI、L3、主线 CI、Release、镜像提升和 public-acceptance 全部成功；没有必需发布步骤失败或重试。独立 Dependency freshness 报告的逾期提醒不属于发布阻断门，按依赖维护项登记，不计为流程异常。发布前发现的本地 mock 文件差异在冻结新候选 SHA 前已恢复到可信 main 基线，正式 dispatch 的 inputs 检查通过。

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：未执行；保留候选和来源工作树以及发布证据目录 `C:\GitHub\_release-evidence\kpanel-v128-rc14-run-40793608653`。
- 未验证风险：真实主机 VNC 会话与滚动性能、实际 WAF/Fail2Ban 日志和 CF 迁移未验证；独立 CF 边界审计未执行；TypeScript / `@types/node` 例外复核逾期；RC13 Windows SCM rollback 首次失败根因仍未确认。
- 已实现待实机准入：browser/VNC 优化、WAF/Jail 迁移状态展示及 CF 凭据重录行为需在非生产且具备真实 Agent/日志源的隔离主机验证。
- 不阻断本版的理由：RC14 定位为预览版；精确候选 CI、GitHub-hosted L3、主线 CI、Release、公开资产与多架构镜像、真实浏览器/隔离容器验收均通过；stable/latest 未变，生产未部署。
- 后续应进入的自动门禁或专项工作流：验证真实 VNC 流和 WAF/Fail2Ban 数据源；维护者复核依赖例外；在后续 Windows RC 流程确认 SCM rollback 错误根因。
