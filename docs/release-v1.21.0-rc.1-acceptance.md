# KPanel v1.21.0-rc.1 发布验收记录

日期：2026-09-20

发布级别：L3

候选提交 / 标签：`679b39489824bf281fd8570d6d95d42d10c54085` / `v1.21.0-rc.1`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览版保留，远端精确指向 `679b39489824bf281fd8570d6d95d42d10c54085`

- 原分支 / 精确 tip / 处置分类：`fix/release-metrics-chronology-20260919` / `5e18853b30d5501ccba2f22fa732133f30a45b23` / 已纳入后归档；`feature/mcp-access-20260919` / `291a646387a61f271f6f3c774a8d7980c55d0813` / 已纳入后归档。
- 归档 ref 与 SHA / 远端复核结果：`archive/fix/release-metrics-chronology-20260919` = `5e18853b30d5501ccba2f22fa732133f30a45b23`；`archive/feature/mcp-access-20260919` = `291a646387a61f271f6f3c774a8d7980c55d0813`；远端逐 ref 复核一致，原活动 metrics ref 已删除，MCP 来源原本无远端活动 ref。
- 本次来源任务分支：两条来源的提交均经 `git cherry 679b394... <source>` 显示 patch-equivalent 已纳入；`release/v1.21.0-candidate` 作为预览列车继续保留。
- 本地分支/upstream/worktree：上述两条来源分支及 `fix/mcp-sdk-boundaries-20260919` 已删除本地 branch 和 worktree 注册；metrics 工作树已删除，MCP 工作树内容已删除但空目录暂被 Windows 进程占用，待句柄释放后回收。未纳入的 WIP 工作树均保留。
- 未完成归档项 / 责任人 / 下次复核触发条件：无远端来源分支未归档项；空目录不含文件和 Git 状态，下一次本机维护时删除。

归档不代表生产上线；本版产物已发布，预览版禁止生产部署。

## 发布画像

- 业务域：按客户端令牌和主机授权范围隔离的 MCP 只读访问；MCP 客户端管理；发布流程异常指标按正式发布时间排序。
- 变更面：新增 Go MCP SDK 与授权存储、只读工具和管理 API；新增设置页交互与中英繁体文案；恢复/回滚清除委派授权；治理指标脚本修复。没有通用 Shell 或 MCP 写操作。
- 受影响用户旅程：管理员创建、配置、测试、刷新和撤销 MCP 客户端；外部客户端读取面板、主机、容器、网站和应用摘要；备份恢复或回滚后的授权失效；发布指标窗口计算。
- 未变化契约：Panel/Agent 端口、Compose、Agent 权限、`kejilion.sh`、应用市场默认 `latest` 入口和生产数据均未改变。
- 风险等级及理由：高；增加外部协议入口、Bearer 凭据、主机授权和依赖，但能力限定为只读，入口预算、输出裁剪、审计、恢复撤权和失败关闭均有测试并通过完整 L3。

## 发布范围与未纳入内容

- 用户可见更新：见 `CHANGELOG.md` `[1.21.0-rc.1]`；MCP 默认关闭，远程使用要求 TLS，令牌只显示一次。
- 精确提交清单：`9e4f6f51` MCP 首版、`19eaeb22` SDK 请求预算、`30a13864` UI token、`021e602a` 恢复/回滚授权回归、`c79abb65` 候选验证记录、`f02a9787` 指标时间顺序、`eefc603c` Git fixture、`679b3948` 版本冻结。
- 明确未纳入的分支、文件或后续事项：`feature/mcp-complete-20260919`、`feature/service-monitor-design-20260919` 为冻结后新工作；`fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 为未完成 WIP；`docs/security-audit-run2-local-20260919` 含受限本地审计材料，均未纳入且未改动。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：Cloudflare `security-audit-skill` pin `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8` 的 scoped run-3 只读目标为 `953c30fb...`；独立验证被平台以 `possible cybersecurity risk` 中止，无 OS 执行沙箱、最终记录与结构校验未完成，因此明确不提供“无漏洞”或审计通过结论。
- finding fingerprint / 修复 commit / 独立复核与回归证据：run-3 没有完成可采信 finding verdict；普通代码复核另行发现 SDK 预算、UI token 和恢复撤权边界，分别由 `19eaeb22`、`30a13864`、`021e602a` 修复，并由定向 race、官方 SDK 双协议、浏览器旅程、L2/L3 和公开镜像 E2E 验证。
- 修复交付状态：源码与 RC 已交付；stable 尚未交付；生产未部署。tag `v1.21.0-rc.1` 包含修复，公开 Release 与 OCI 证据见下文。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：open-code-review 1.12.6 对 `c98727c..6446a9e` 处理 18 个代码文件 / 23 个总文件，free-form 有效发现 0、constrained-only 0；提交 trailer 完整，未将工具输出当作发布门禁。
- 外部能力结论：两项能力均作为辅助证据使用。中止审计保持未完成，OCR 保持非阻断，最终发布判断来自项目本身的 L3、CI、Release、公开产物和隔离 E2E。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：相对 `v1.20.0` 没有 `Dockerfile`、`packaging/kejilion-app/kpanel.conf` 或脚本协议变化；L3 `app_conf_lifecycle=pass`，公开镜像内置脚本摘要保持基线。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 授权存储、管理 API、只读工具、官方 Go SDK 两代协议、L3 全量 Go/Web 与公开镜像健康链路通过。 | 未覆盖具体第三方 MCP 客户端产品。 |
| 网络入侵与供应链安全 | 已验证 | 独立 Bearer、拒绝 Cookie/query token、Origin/TLS 边界、主机授权、输出裁剪、审计失败关闭；govulncheck、npm audit、Trivy 源码/镜像扫描通过。 | scoped run-3 被平台中止，不能据此宣称完整专项安全审计通过。 |
| 稳定性、失败恢复与兼容 | 已验证 | SDK 请求沿用入口预算；刷新/撤销、版本冲突、备份恢复和回滚丢弃委派授权有回归；race、L3 和公共 OCI 冷启动通过。 | 未做长期高并发 MCP soak。 |
| 性能与资源预算 | 已验证 | WSL 短样本记录二进制 +1,925,120 bytes，关闭/启用 MCP 的 RSS 中位数较基线约 +2.5/+3.625 MiB；请求/工具/输出预算有代码和测试约束。 | 样本为空资源、少量回环调用，不能代表真实大集群。 |
| 用户体验与可访问性 | 已验证 | 本地真实回环旅程覆盖创建到撤销、浅/深主题、中英繁体、390/768/1280 视口与 200% 布局模拟，最小计算字号 13px；Web 164 文件 / 1,441 测试通过。 | 标准 acceptance 预览启动器曾被平台审批拒绝，没有 manifest；未将回环预览冒充标准预览。 |
| 数据、配置与迁移 | 已验证 | MCP 授权独立存储、令牌一次显示；恢复/回滚清除授权；旧版本忽略新增目录；无既有数据迁移。 | 未在生产数据目录执行恢复。 |

## 自动门禁

- 定向测试及结果：MCP store/management/tools/SDK/backup 定向测试、race、官方 SDK 双协议和浏览器旅程通过；治理指标单测通过。
- `make verify-release` 环境和结果：`kpanel-release-gate:go1.26.7-node24`，不可变 Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；`app_conf_lifecycle=pass`、`release_gate_runner=pass`、`release_l3_gate=pass`、`release_l3_remote=pass`。
- L3 外层入口：run ID `v1.21.0-rc.1-679b394-l3-r1`，2026-09-20 00:53:31 至 01:08:41 +08:00，exit 0；plan `f65ac47a...`、remote entry `d8bb2cf2...`、bundle `1ea66cd9...`、远端日志 `c4dea4a7...`；证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.1-679b394-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.1-679b394-l3-r1`。
- 候选 CI：[最终 CI 35459393753](https://github.com/kejilion/KPanel/actions/runs/35459393753) 成功；[freshness 35457183656](https://github.com/kejilion/KPanel/actions/runs/35457183656) 成功。前两次 npm 上游维护失败见流程异常。
- 主线 CI：[CI 35459847045](https://github.com/kejilion/KPanel/actions/runs/35459847045) 与 [freshness 35459847052](https://github.com/kejilion/KPanel/actions/runs/35459847052) 成功，均绑定 `679b394...`。
- Release workflow：[Release 35460282207](https://github.com/kejilion/KPanel/actions/runs/35460282207) 与 [tag freshness 35460282279](https://github.com/kejilion/KPanel/actions/runs/35460282279) 成功；2026-09-20T02:18:23+08:00 公开。
- 安全扫描、镜像契约、SBOM/provenance：Release 的 govulncheck、npm audit、Trivy 源码与原生镜像扫描、受限运行时契约全部通过；双架构各带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 freshness 均成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：L3/Release 的 govulncheck、npm audit 与 Trivy 无阻断项；npm 审计端点维护恢复后精确 SHA 重跑成功。
- 直接/基座行动项、传递依赖归属信号及期限：新增 `github.com/modelcontextprotocol/go-sdk v1.8.0` 及其锁定传递依赖，已在本版完成协议、race、扫描和构建验收，无待处置升级候选。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go SDK v1.8.0；继续使用 Go 1.26.7、Node 24、固定摘要基础镜像、Actions 与受管脚本基线。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.21.0-rc.1`；OCI index `sha256:3add580bfd52fe25224de54782ddc52eeb5b87ba34a9af275e51cb131a3a5c21`；脚本摘要 `28cf3934...`。
- 暂缓或拒绝候选：无依赖候选暂缓项；run-3 结果因平台中止不可用，不作为依赖准入结论。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览 L3、双协议互通、失败恢复、双架构公开 OCI 与公开 E2E 通过；生产和稳定默认入口未改变。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.x；固定 L3 Runner 如上。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；策略检查通过，未请求 production 用途。
- 使用的精确候选或公开产物：`679b39489824bf281fd8570d6d95d42d10c54085`；公开 `docker.io/kjlion/kejilion-panel@sha256:3add580bfd52fe25224de54782ddc52eeb5b87ba34a9af275e51cb131a3a5c21`。
- 后台作业 ID、终态、退出码、证据目录、命令规格：L3 r1 passed / 0；公开 OCI E2E r1 `image_e2e=pass`，日志 SHA-256 `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`；固定入口 `packaging/tests/image-e2e.sh` SHA-256 `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`；证据位于 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.1/`。
- 测试窗口/循环数及风险依据：一次完整 L3 和一次公开 OCI 端到端冷启动；本版无流式、重连或长期后台生命周期变化，不机械执行 soak。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：本地真实回环浏览器覆盖创建、一次性配置、测试、刷新、撤销和版本冲突；390/768/1280、200% 布局模拟、浅/深、中英繁体及失败态；L3 重放自动回归。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅隔离容器临时目录；固定 E2E 使用只读根文件系统、drop all capabilities、no-new-privileges，结束后清理容器和网络；恢复/回滚授权清理由自动回归覆盖。
- 未执行场景及原因：第三方 MCP 客户端、真实大 Agent 清单、低配主机、长期并发 soak 和生产部署未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 状态：[KPanel v1.21.0-rc.1](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.1) 为非 draft、prerelease、非 Latest；GitHub Latest 仍为 `v1.20.0`。
- Docker 版本与通道 OCI index：`1.21.0-rc.1` 与 `preview` 均为 `sha256:3add580bfd52fe25224de54782ddc52eeb5b87ba34a9af275e51cb131a3a5c21`；稳定 `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- `linux/amd64`、`linux/arm64` digest：`sha256:35ec71f92b518ecfa9b23a6310f7982f7e2b58c2c135c3ff7be621a904423072` / `sha256:37d4a2125f29692fc1bb856b427f7404c6b19546d762158f874b562a527c3ed1`；另两项 `unknown/unknown` 为 attestation。
- 附件及 `SHA256SUMS`：8 个附件完整，包括 Agent/Node 双架构、部署包、LICENSE、`SHA256SUMS` 和 THIRD_PARTY_NOTICES。
- 公开镜像 `image_e2e=pass`：精确源码 archive SHA-256 `505e12ad...`，按不可变摘要拉取并运行固定入口，版本健康、静态资源、bootstrap、安全 Cookie、网络隔离和容器健康全部通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：安装/更新契约相对 `v1.20.0` 零差异，无 apps 提交；默认仍为 `latest`；脚本基线不变。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源选择规范稳定版或 RC，并校验唯一官方镜像 digest：既有实现与测试继续通过，本版没有改变选择协议。
- 加入预览只切换来源并立即检查，没有自动安装：已由既有通道回归覆盖，本版无契约变化。
- 自动安装开关与一次性立即安装相互独立：已由既有回归覆盖。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：已由既有回归覆盖。
- 退出预览且稳定版较低时没有产生降级候选：已由既有回归覆盖。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：既有回归和 L3 通过，本版未改变。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 呈现；本次只提升 `preview`，候选分支继续保留。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：`arena-154` 仅用于 `candidate-validation` 隔离容器。
- 正式部署环境：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：0。
- 仅在隔离真机执行、未在生产执行的场景：完整 L3、公开 OCI 冷启动与 bootstrap E2E。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`。
- 镜像 digest：稳定 `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`；上一预览 `1.20.0-rc.3` 为 `sha256:76b54033a5f58a614ae121700539b3891f3a8d2554939cd24fa63c501506a2ba`。
- 数据/配置备份：不适用（预览版禁止生产部署）。
- 回滚步骤和回滚后复核：测试实例可显式选择上一预览或稳定摘要；旧版本忽略新增 MCP 授权目录，回滚前关闭 MCP，恢复/回滚会丢弃委派授权。
- 回滚后生产实际版本与健康状态：不适用；本轮未部署或回滚生产。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 `v1.20.0` / `sha256:a991b5d2...` / `latest`。
- 公共默认更新通道决策：不适用；稳定默认入口未改变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-19T18:33:55+08:00
- 候选冻结时间：2026-09-20T00:52:00+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成生产回滚、紧急热修复或重复发布；本次预览版没有生产写操作。两类流程异常都发生在发布前监控或候选 CI，门禁没有逃逸。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/npm-audit/scheduled-maintenance",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI 35457183647 与同 SHA 重试 35457652130 在 npm audit 端点计划维护期间收到 503；同一上游根因批次使候选门禁延迟约 43 分钟，产品测试此前步骤均通过。",
    "recoveryEvidence": "npm 状态窗口结束后直接 audit 返回 0 vulnerabilities；精确 SHA 的 CI 35459393753 成功，随后 main CI、Release 的 npm audit 也成功。",
    "permanentAction": "不可控上游例外由发布维护者负责；后续遇 5xx 先核对 npm 官方状态并保留原 run，不绕过 audit，只在服务恢复后对同一 SHA 重跑；下一候选 CI 复核，连续成功后关闭观察。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-monitor/github-api/anonymous-rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选成功后本地匿名 GitHub API 达到每小时限额，短时无法通过 REST 读取工作流状态；GitHub Actions 本身未失败，发布写入未受影响。",
    "recoveryEvidence": "改用公开 Actions HTML、commit checks 与 workflow badge 只读核验，限额重置后 REST 再次确认候选、main、tag freshness 和 Release 全部成功且绑定 679b394...。",
    "permanentAction": "发布维护者在后续发布前检查匿名 API 剩余额度；无认证上下文时优先使用公开 Actions 页面并降低轮询频率，限额恢复后再用 REST 复核最终结构化状态；下次发布执行时复核。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：已删除 metrics 工作树约 17,910,455 bytes、MCP 工作树内容约 196,335,533 bytes及三个已归档本地分支；MCP 空目录因 Windows 进程句柄暂留但不含文件；保留 L3、公开 E2E 证据和预览候选工作树供 `1.21.0` 后续 RC。
- 未验证风险：第三方 MCP 客户端、真实大规模 Agent 资源、低配主机、长期并发 soak；run-3 未完成专项安全审计。
- 已实现待实机准入：后续写能力仍未开放；如新增写工具，必须接入真实审批记录、版本化能力协商和固定动作，并重新执行专项审计与 L3。
- 不阻断本版的理由：能力默认关闭且限定只读；精确 SHA 已通过完整 L3、候选/main/tag 门禁、Release 安全链、双架构公开 OCI 和公开镜像 E2E；审计中止状态被如实保留，稳定入口及生产未改变。
- 后续应进入的自动门禁或专项工作流：稳定版前复核 RC 反馈、真实第三方 MCP 客户端和大清单预算；新增写能力前完成可执行的 scoped 安全审计；发布监控降低匿名 API 轮询并保留 HTML 回退。
