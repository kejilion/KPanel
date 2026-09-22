# KPanel v1.21.0 发布验收记录

日期：2026-09-22

发布级别：L3

候选提交 / 标签：`396fcd62c5635c9812ee97ee509cb61b962724f3` / `v1.21.0`

上一稳定版本 / 回滚点：`v1.20.0` / `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`

`releaseChannel`：`stable`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 稳定版归档

- 原分支 / 精确 tip / 处置分类：`release/v1.21.0-candidate` / `396fcd62c5635c9812ee97ee509cb61b962724f3` / absorbed。
- 归档 ref 与 SHA / 远端复核结果：Release workflow 的 Archive published candidate branch 步骤成功；`archive/release/v1.21.0-candidate` 指向 `396fcd62c5635c9812ee97ee509cb61b962724f3`，与 stable tag 提交一致，活动候选远端已不存在。
- 本次来源任务分支：rc.1 至 rc.14 的全部候选已随各 RC 进入 `main`；稳定候选相对 rc.14 只新增 run-4 审计登记 `05094472` 与版本准备 `396fcd62`，不含产品代码变化。
- 本地分支/upstream/worktree：发布工作树 `C:/GitHub/_codex-tasks/kpanel-v1.21.0-rc.14`（分支 `codex/release-v1.21.0-rc.14-assembly`、`codex/release-v1.21.0-stable`）在本验收记录提交后回收；L3、OCR 证据保留在 `C:/GitHub/_release-evidence/`。
- 未完成归档项 / 责任人 / 下次复核触发条件：已合入 main 的来源分支（`feature/monitoring-history-categories`、`feature/passkey-20260921`、`feature/passkey-store-20260921`、`feature/passkey-ui-20260921`、`fix/desktop-group-initial-render`、`docs/security-audit-trigger-20260921`）由本发布任务按 10.2 归档；陈旧 WIP 分支 `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 含未提交改动，保留待负责人处置。

归档不代表生产上线：本版**正式产物已发布，生产未部署**（见“生产部署安全核对”）。

## 发布画像

- 业务域：MCP 访问与结构化管理、Passkey 认证、系统中心病毒查杀与软件包管理、服务检测与历史监控、桌面分组与首帧稳定、集群 TLS 兼容、安装/卸载安全清理、发布治理。
- 变更面：展示、只读、宿主机写入、协议或数据、部署。
- 受影响用户旅程：MCP 客户端创建/授权/审批、Passkey 注册/登录/关闭重绑、病毒扫描与软件包安装卸载、服务检测配置与历史监控、桌面分组、设置搜索、集群排序、重新安装与残缺卸载。
- 未变化契约：既有端口、Compose 入口与 Agent 权限模型；MCP 与 Passkey 默认关闭；`kejilion.sh` 契约相对 v1.20.0 为 `coupled`（rc.4 病毒扫描协议，脚本先行发布）。
- 风险等级及理由：高。新增认证方式、MCP 远程管理边界和宿主机写入能力，由 14 个 RC 渐进验收、full run-4 与 scoped run-6 审计、稳定候选 L3 覆盖；本次未能执行公开 OCI E2E 和生产部署，风险在遗留项中明确。

## 发布范围与未纳入内容

- 用户可见更新：见 `CHANGELOG.md` 的 `[1.21.0]` 汇总条目（MCP、Passkey、病毒查杀、软件包管理、服务检测、历史监控分类、桌面分组、设置搜索、集群排序、TLS 兼容回退、安装清理）。
- 精确提交清单：以 `git log v1.20.0..396fcd62c5635c9812ee97ee509cb61b962724f3`（172 个提交）为完整可复现清单；各 RC 的纳入范围记录在 `docs/release-v1.21.0-rc.*-acceptance.md`。
- 明确未纳入的分支、文件或后续事项：`fix/file-host-switch-context-20260913` 与 `feature/visual-refinement-pass` 两个陈旧 WIP；本地保留的 run-2 与 run-4 审计细节（披露边界，未推送）。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：full run-4 基于 `4c0694aa8e02e46145a775707b8d5a0355f7ce10`（127 单元：95 covered、32 candidate；0 confirmed、34 needs_validation、1 rejected），本次登记入库（`05094472`，仅元数据与计数简报）；scoped run-6 基于 `b8ba15f484c64d43241eaa3f2d4060d702b75fbd` 覆盖 Passkey 边界。run-3、run-5 为平台中止，不计覆盖。
- 覆盖检查：`node scripts/check-security-audit-coverage.mjs --target 396fcd62 --require` decision=`ok`，exit 0；last_full=run-4（1 天），未审计提交 2 个（`dd3d46e5` Passkey 关闭/重绑、`632c988b` Passkey trusted origin，均 0 天，在 14 天窗口内），无新边界包。补审 run 为已记为 complete 的 run-4。
- finding fingerprint / 修复 commit / 独立复核与回归证据：run-4 无 confirmed；34 条 needs_validation 的细节按 5.4 披露边界保留在 `C:/GitHub/_codex-evidence/kpanel-security-audit-run4-v1.21.0`，尚无对应修复提交。
- 修复交付状态：不适用（本周期无 confirmed finding）；needs_validation 待具备 OS 沙箱或隔离真机后动态确认。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：`v1.20.0..v1.21.0` 共 93 条 `OCR-Review:` trailer；数值型 constrained-only（有效）39 条、unreported 27 条、skipped 26 条、deferred 1 条；constrained-only>0 的 2 条均来自 0.3.4 早期版本（`adfa0c23`、`9dfedb7d`），1.12.6 下累计 0。`Independent-Review:` trailer 9 条，跨提供商 0。
- 本稳定周期观察结果：OCR 在 1.12.6 下未产出约束臂独有的 ≥MEDIUM 发现，自由臂仍是主要发现来源；security-audit-skill 本周期 full 产出 0 confirmed / 34 needs_validation。两者均未满三个完整观察周期，不提前宣称有效或应退出。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`。
- 变更集编号：v1.21.0-rc.4 病毒扫描协议（KPanel `22519950` pin 脚本）。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`（`Dockerfile` 标签 `io.kejilion.script.sha256`）。
- 脚本候选 commit / SHA-256：同上；rc.4 之后无新增脚本改动。
- 状态判定依据与兼容性证据：病毒扫描路径校验在浏览器、KPanel API 与脚本统一；稳定 L3 `managed_script_contract` 与 `app_conf_lifecycle=pass`，Release 的 Verify kejilion.sh application lifecycle 成功。
- 本版发布决定：脚本先行后发布 KPanel（脚本已于 rc.4 前发布到权威主线）。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 稳定 L3 Go 全量与 Web 测试、应用生命周期、managed script contract 通过；各 RC 的定向与浏览器验收。 | 公开 OCI E2E 与 arena-154 真机/生产本次未执行（arena-154 不可达）。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、Trivy 源码/依赖/密钥/配置/镜像均通过；full run-4 + scoped run-6 覆盖，覆盖检查 ok；Release 附件校验和与 OCI attestations 齐全。 | run-4 的 34 条 needs_validation 未动态确认；2 个 Passkey 提交在窗口内未审计。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | privileged core race（L3 与 CI #1048/#1049）通过；安装/更新/中断恢复/卸载生命周期测试通过。 | 未做生产 soak、真实断电；主线 race 在 rc.13/rc.14 出现抖动，根因未定位。 |
| 性能与资源预算 | 已实现未实机验证 | 镜像运行时契约在 256 MiB/1 CPU/128 PID 限制下启动通过；L3 构建无超时/OOM。 | 未采集真实节点资源曲线。 |
| 用户体验与可访问性 | 已验证 | Web 测试、类型检查与 i18n 覆盖；各 RC 浏览器验收；rc.14 OCR 记录分类 tablist 的 LOW 可访问性项。 | 稳定候选相对 rc.14 无 UI 变化；未重复全部真机视口。 |
| 数据、配置与迁移 | 已实现未实机验证 | Passkey 存储、MCP 授权存储、桌面分组 workspace 资源版本与备份恢复回归测试通过。 | 未执行生产 SQLite quick_check 与升级回滚演练（生产未部署）。 |

## 自动门禁

- 定向测试及结果：稳定候选相对 rc.14 只有治理元数据与版本文件；`scripts/verify-governance.sh` 通过，覆盖检查 `--validate`（6 runs）与 `--require` 通过，版本一致性通过。
- `make verify-release` 环境和结果：`local-wsl-dr`（`arena-154` SSH 超时，按 `docs/project-management.md` 选择灾备，仅用于候选 L3），`kpanel-release-gate:go1.26.7-node24`，完整 L3 通过。
- L3 外层入口：run `v1.21.0-396fcd6-l3-r1`，开始 `2026-09-22T11:22:28Z`、完成 `2026-09-22T11:29:10Z`，status=passed/exit 0；plan SHA `2d1773007999011fedd97e05856927f354c246c883299fc8d3d889df1a9e3a82`，remote script SHA `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`，bundle SHA `4d7500b004f9887a50e09f5da8a6d30141729055f51adc4510d6561d8d16c66a`，manifest SHA `0f9ada066200f905dff65c6f764f36abf346cb8da6e9e4770cda84af43ae8af9`，日志 SHA `18ec7bc7c6a532b3c88f15eb85883de3d71223e1360ac407b7d50d49037403ec`，Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；证据目录 `C:/GitHub/_release-evidence/v1.21.0-396fcd6-l3-r1`。
- 候选 CI：[#1048](https://github.com/kejilion/KPanel/actions/runs/35721747264) success；Dependency freshness [#549](https://github.com/kejilion/KPanel/actions/runs/35721747278) success。
- 主线 CI：[#1049](https://github.com/kejilion/KPanel/actions/runs/35722451119) success；Dependency freshness [#550](https://github.com/kejilion/KPanel/actions/runs/35722451102) success。
- Release workflow：[#247](https://github.com/kejilion/KPanel/actions/runs/35723165515) 全部步骤 success（含 Promote image to its release channel 与 Archive published candidate branch）。
- 安全扫描、镜像契约、SBOM/provenance：Release 内 govulncheck、npm audit、Trivy 源码与原生镜像扫描、运行时镜像契约均成功；OCI index 含 amd64/arm64 与两个 attestation manifests。

## 依赖与技术栈变化

- `make dependency-report` 与检测源完整性：候选、主线 Dependency freshness 工作流均成功；`report-dependency-freshness.mjs --validate-only` 11 个依赖组策略通过。
- 最近每日安全通告审计、EOL 复核状态：`report-governance-health.mjs --strict --since=v1.20.0` 报 overdue=2（`quality-improvement-2026-09-05-release-source.md`、`quality-improvement-2026-09-05-standards-alignment.md` 待复核 17 天，SLA 14），unclassified=0；该项无机器门禁，不阻断本版，列入遗留。
- 直接/基座行动项：新增直接依赖 `github.com/modelcontextprotocol/go-sdk v1.8.0`（MCP）、`github.com/go-webauthn/webauthn v0.18.2` 与 `github.com/fxamacker/cbor/v2 v2.9.4`（Passkey）、`github.com/google/jsonschema-go v0.4.3`；`golang.org/x/crypto` 0.55→0.57、`x/net` 0.57→0.58、`x/sys` 0.47→0.48、`x/term` 0.45→0.46，及随之引入的间接依赖。
- 本版采用的工具链：Go 1.26.7、Node 24.20.0、固定 digest 基础镜像与 Actions SHA、open-code-review 1.12.6、security-audit-skill `c1c8a8c1`。
- 暂缓或拒绝候选：无阻断本版的依赖升级。
- 升级结论：锁文件、构建、漏洞扫描、双架构镜像与应用生命周期均通过；生产回滚入口本次未演练（生产未部署）。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`local-wsl-dr`（WSL2 Ubuntu，amd64），固定 Runner Go 1.26.7 / Node 24.20.0。
- 环境策略 ID 与允许用途：`local-wsl-dr` / validation，仅 `candidate-validation`；`arena-154` 本次 SSH 22 端口超时，未使用。
- 使用的精确候选或公开产物：`396fcd62c5635c9812ee97ee509cb61b962724f3`。
- 后台作业和证据：L3 run `v1.21.0-396fcd6-l3-r1` exit 0，见“自动门禁”。
- 测试窗口/循环数及风险依据：一次稳定候选完整 L3；长期 soak 不适用，风险由 14 个 RC 与 race 门禁覆盖。
- 用户旅程与视觉：沿用各 RC 已记录的浏览器验收；稳定候选相对 rc.14 无 UI 变化。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 应用生命周期覆盖安装、更新、中断恢复与卸载；生产未执行。
- 未执行场景及原因：公开 OCI E2E、arena-154 真机验收、生产部署与回滚演练——`arena-154` 不可达，`local-wsl-dr` 不允许用于其他验收；`prod-108` 禁用，未执行任何动作。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：`v1.21.0` draft=false、prerelease=false、GitHub Latest；发布时间 2026-09-22T11:55:49Z；annotated tag object `aeaf12180203ad281bef3610819576312c8a20d3`。
- Docker 版本与通道 OCI index：`1.21.0` 与 `latest` 均为 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`（与 Release notes 一致）；`preview` 保持 rc.14 的 `sha256:bd79e42938a319b325cdfd02d61241525428bef887d2f181636ea2f73d406308`。
- `linux/amd64`、`linux/arm64` digest：`sha256:9d181268e009a799a30a1e7272dd883b1ede407be10539a3596dc70b70a8ff54` / `sha256:837b9ac521831febbd6be360ca57fc345c175a13c84c5a75141a7d5ebec7d887`。
- 附件及 `SHA256SUMS`：Agent amd64/arm64、Node amd64/arm64、`kpanel-mcp` 六个平台、部署包 `kejilion-panel-deploy-1.21.0.tar.gz`、LICENSE、THIRD_PARTY_NOTICES.md、SHA256SUMS，共 14 个。
- 公开镜像 `image_e2e=pass`：未执行（arena-154 不可达）；恢复后补做。
- `kejilion/apps` / `kejilion.sh` 契约结论：`C:\GitHub\kejilion\apps` clean 且与 `origin/main` 同步；忽略仓库专属 `app_url` 后 `kpanel.conf` 与发布配置一致（已由 `69f1a071` 提前同步），无需提交；受管脚本 SHA-256 与 pin 一致。

## 自更新通道验收

- 稳定来源选择正式 GitHub Latest，预览来源选择规范稳定版或 RC，并校验唯一官方镜像 digest：已实现未实机验证（本列车未改变通道选择逻辑，L3 单元测试覆盖）。
- 加入预览只切换来源并立即检查，没有自动安装：已实现未实机验证，契约未变化。
- 自动安装开关与一次性立即安装相互独立：已实现未实机验证，契约未变化。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：已实现未实机验证，契约未变化。
- 退出预览且稳定版较低时没有产生降级候选：已实现未实机验证；v1.21.0 高于全部 1.21.0 RC，预览用户将获得向前升级。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：已实现未实机验证，本次无真机。
- OpenRC 与轻量 Node 的当前边界按 `docs/release-channels.md` 呈现：已实现未实机验证，边界未变化。

## 生产部署安全核对

- 生产目标和部署授权范围：用户决定“先发产物，暂不部署”；本次正式产物已发布，生产未部署。
- 验证/灰度环境：`local-wsl-dr`（仅候选 L3）。
- 正式部署环境：`arena-154`，本次 SSH 不可达，未连接。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：未执行。
- 部署命令/入口：未执行；恢复后使用 `scripts/run-production-evidence.mjs` preflight/backup → 标准应用市场更新 → postdeploy。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：未执行。
- 生产已执行写操作：0。
- 仅在隔离真机执行、未在生产执行的场景：稳定候选 L3。

## 回滚

- 源码/tag：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`。
- 镜像 digest：`sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- 数据/配置备份：不适用（生产未部署）。
- 回滚步骤和回滚后复核：若公开默认更新需要撤回，将 GitHub Latest 与 Docker `latest` 恢复到 v1.20.0 digest，保留不可变 `1.21.0` 版本镜像与 tag，并在 Release 注明原因。
- 回滚后生产实际版本与健康状态：不适用，未触发回滚。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest `v1.21.0`；Docker `latest` `sha256:e2c5d392…`；应用市场默认入口 `latest`。
- 公共默认更新通道决策：不适用；v1.21.0 作为稳定默认发布。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-19T19:38:56+08:00
- 候选冻结时间：2026-09-22T19:21:38+08:00
- 生产完成时间：未验证
- 提交到生产用时：未验证
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：发布工作树在本记录提交后回收；L3/OCR 证据保留于 `C:/GitHub/_release-evidence/`，未删除历史证据。

- 未验证风险：生产未部署；公开 OCI E2E、arena-154 真机与回滚演练未执行；run-4 的 34 条 needs_validation 未动态确认；主线 race 门禁在 rc.13/rc.14 抖动，根因未定位。
- 已实现待实机准入：`arena-154` 恢复后依次执行公开 OCI E2E（`image-e2e.sh`，`KPANEL_EXPECTED_VERSION=1.21.0`）与生产 preflight/backup/部署/postdeploy，并补记本文件“生产部署安全核对”与交付节奏。
- 不阻断本版的理由：用户明确选择“先发产物，暂不部署”；覆盖检查 ok；所有候选/主线/Release 门禁通过；项目管理规范允许停留在“产物已发布，生产未部署”。
- 后续应进入的自动门禁或专项工作流：治理 `--strict` 两份 09-05 提案超期复核；race 抖动定位；Passkey 两个窗口内提交在 14 天内（2026-10-06 前）进入下一次 scoped 审计。
