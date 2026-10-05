# 质量改进提案：草案纳入复核时限，降级复核理由改为固定取值

- 提案状态：已采纳（行为已等价进入 RC3 主线；2026-10-05 补历史提案与独立复核回执，观察收益尚未证明）
- 提案日期：2026-10-02
- 负责人：Claude（候选实现）；集成由唯一集成/发布任务负责
- 适用业务域：治理（`PROJECT_RULES.md` 5.2.4、5.2.7；`docs/multi-agent-collaboration.md`）
- 基线提交 / 标签：`642cd059b6d070f1496a07c96040683f2e265974`（`origin/main`，v1.24.0-rc.5 之后）
- 关联缺陷、验收记录、CI 或事件：2026-10-02 规范执行复查；`docs/quality-improvement-2026-09-19-governance-execution-health.md`
- 提交、推送、发布和生产权限：原候选仅获本地实施授权；本轮用户授权整合候选并发布新预览版。此文件只补历史提案回执，不自行授予生产权限。

正文的基线计数、原门禁和原权限描述均保留为 2026-10-02 的历史事实，不作为 RC4 当前门禁结论。
2026-10-05 独立复核确认有效行为已等价进入 `7621e52d87b5b7f8879cfe4bff87946dde0fb2f1`；
本轮不重放旧代码。当前源码、CI、发布与观察状态以末尾回执和各精确提交的验收原件为准。

## 观察证据

- 可重复观察：
  1. 两份提案的实现已进入 `main`，状态行仍为"草案"，而 `--strict` 只对"待复核"计时，因此超期一直为 0：
     `quality-improvement-2026-09-19-branch-archive-lifecycle.md`（实现提交 `2774212b` 自 `v1.20.0` 起在主线）和
     `quality-improvement-2026-09-20-rc-process-metrics.md`（实现提交 `29c180a2` 自 `v1.21.0-rc.5` 起在主线）。
  2. `Independent-Review:` trailer 的 `fallback=` 是自由文本。`v1.22.0..642cd059` 共 34 条 trailer，跨提供商 1 条，
     格式无效 4 条；带 `fallback` 的 30 条使用了 12 种写法（如 `claude-cli-unavailable`、
     `claude-and-gemini-cli-unavailable`、`no-installed-cross-provider-reviewer`），其中 3 条
     （`same-provider` ×1、`same-provider-clean-session` ×2）没有说明任何不可用原因，现有脚本仍把它们计为"已说明"。
- 精确环境、版本、时间范围和样本量：Windows 本机，Node 24.17.0；提案 19 份；trailer 样本为 `v1.22.0..642cd059`。
- 原始命令、日志、截图或验收记录：
  `node scripts/report-governance-health.mjs --strict --since=v1.23.0`（基线输出 `pending=2 overdue=0 ... draft=2`）；
  `git log v1.22.0..HEAD --format='%(trailers:key=Independent-Review,valueonly,unfold,separator=%x1e)%x1e'`；
  `git tag --contains 2774212b`、`git tag --contains 29c180a2`。
- 已确认事实：上述计数和主线归属可由命令复现；两份草案提案的正文均写"待候选 CI / 主线采纳"，与主线事实不符。
- 未确认信息与数据缺口：自由文本 fallback 中有多少属于"调用失败"而非"没有入口"无法判断，这正是本提案要建立的数据。
- 为什么不是一次偶发环境故障：两个问题都是规则/计数口径问题，跨多个发布列车持续存在，与执行环境无关。

## 原因假设

- 待验证根因：5.2.7 只给"待复核"设时限，"实现进入主线"这一生命周期节点没有对应的状态要求，草案于是成为不计时的停留态；
  fallback 只要求"写原因"，没有取值约束，也没有候选层提醒，各会话各写各的。
- 可能的替代解释：草案停留是因为集成任务不知道要更新提案状态（流程缺口）而非时限缺失——本提案同时补"集成时更新状态"的要求，
  由时限兜底；fallback 写法分散也可能是因为跨提供商入口本身缺失，固定取值不能解决入口问题，只能让它可度量。
- 能证伪本假设的结果：采纳后到 v1.25.0 发车前，仍出现实现已进主线但状态为草案、且未被 `--strict` 报告的提案；
  或采纳后的新 trailer 仍大量使用取值外的写法而候选提醒未被注意。
- 是否涉及 `kejilion.sh`、真实系统资源、Panel/Agent 权限边界或兼容契约：否。

## 产品原则与核心思想对齐

- 对业务真源和双端互通的影响：无，只涉及治理文档和治理脚本。
- 对轻量、低资源、单管理员控制面的影响：无运行时影响。
- 对安全、稳定、性能、体验、数据/迁移的影响：无产品代码变化；不放宽任何门禁。
- 不应引入的复杂度或第二真源：不新增提案↔提交映射表、不新增独立校验脚本；fallback 取值只在
  `scripts/report-governance-health.mjs` 定义一次，候选检查与一致性检查都从这里导入。

## 基线、目标与观察窗口

| 指标 | 当前基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| 主指标 1：实现已进 `main` 但状态为草案且未被时限报告的提案数 | 2 | 0（超过 14 天的草案必须出现在 `--strict` 的 overdue 中） | `report-governance-health --strict` + `git tag --contains` | 采纳至 v1.25.0 稳定版发车前质量审计 |
| 主指标 2：采纳后新写的同提供商 trailer 中取值外或缺少 fallback 的条数 | 不适用（取值尚不存在；`v1.22.0` 以来 30 条全部为自由文本） | 0 | `report-governance-health --since=<采纳提交>` 的 `fallback_unrecognized` 与 `same_provider_without_fallback` | 同上 |
| 防回归指标 1：草案超期的处置方式 | 先建立基线 | 以"复核延期"处置的超期草案不超过一半；超过则复审本规则是否只制造文书 | 提案状态行与"复核延期至"行 | 同上 |
| 防回归指标 2：候选检查退出码 | `--require-candidate` 不因 trailer 失败 | 保持：`independent_review=` 只提醒，不改变退出码 | `scripts/tests/collaboration-state.test.mjs` | 每次门禁 |
| 防回归指标 3：治理回归集 | 基线 `642cd059` 227/227、提案 19 份（候选 230/230、提案 20 份，新增 3 项测试） | 全部通过，无删减测试 | `scripts/verify-governance.sh` | 每次门禁 |

数据不足时写"先建立基线"，不得把缺失值当作零缺陷或成功。

## 范围与非目标

- 允许修改：`PROJECT_RULES.md` 5.2.7；`docs/multi-agent-collaboration.md` 独立复核段；
  `docs/quality-improvement-proposal-template.md` 延期行；`scripts/report-governance-health.mjs`；
  `scripts/check-collaboration-state.mjs`；`scripts/check-governance-consistency.mjs`；对应测试；本提案。
- 明确不修改：14 天时限与延期上限；跨提供商复核的默认要求；历史提案与已推送 trailer；
  任何发布、CI 或 release-gate 门禁；两份现存草案提案的状态（由其负责人或集成任务按新规则处置）。
- 受影响用户旅程：无产品用户旅程；影响治理提案作者、独立复核者和集成任务。
- 风险等级与所需验收层级：L1（治理文档与脚本，不涉及产品代码）。

### 规范验收合同（仅永久规范、工作流、策略或治理门禁变更填写）

- 精确规范基线：`642cd059b6d070f1496a07c96040683f2e265974`。
- 冻结的允许范围：同"允许修改"。
- 明确非目标：同"明确不修改"；不把 trailer 合规做成阻断门禁；不强制跨提供商复核。
- 权威入口和正常授权路径：`scripts/report-governance-health.mjs`（`--validate` 日常、`--strict` 稳定版发车前质量审计）；
  `scripts/check-collaboration-state.mjs --role writer --require-candidate`（候选层提醒）；`scripts/verify-governance.sh`。
- 固定验收矩阵：正确性 / 一致性 / 完整性 / 可执行性 / 效率与比例性 / 可演进性。
- 预先确定的回归集与证据：`scripts/tests/report-governance-health.test.mjs`、`scripts/tests/collaboration-state.test.mjs`、
  `scripts/verify-governance.sh` 全量；`--strict --today=2026-10-05` 应把两份现存草案报告为超期并退出 3。
- 本轮停止条件：矩阵六项完成、回归集通过、阻断项为零。
- 范围外发现如何进入后续事项：记入本提案"后续事项"，不扩大本轮范围。

## 备选方案与取舍

| 方案 | 收益 | 风险 / 成本 | 是否采用及理由 |
| --- | --- | --- | --- |
| A：草案与待复核共用 14 天时限，集成时须更新状态 | 复用现有计时与延期机制，一处改动 | 早期草案也会计时，需要写延期理由 | 采用；延期机制已存在，成本是一行理由 |
| B：机器识别"实现已进主线"（新增提案↔提交映射 trailer） | 判定精确 | 新增第二真源和映射维护成本，历史提交无映射 | 不采用 |
| C：只在文档要求集成时更新状态，不计时 | 零代码 | 无机器入口；09-20 复查已证实无提醒的条款不被执行 | 不采用 |
| D：fallback 固定两值 + 可选 `detail=` | 可统计"缺入口"与"调用失败"的比例，具体情况不丢失 | 两值可能过粗 | 采用；观察窗口内按数据决定是否细分 |
| E：取值外或缺少 fallback 时阻断 | 强制合规 | trailer 常在复核后补写、历史不可改，阻断会卡发布 | 不采用；只计数和提醒 |

## 最小改动方案

- 预计修改文件或唯一入口：
  - `report-governance-health.mjs`：时限判定覆盖 `draft` 与 `pending-review`；导出 `FALLBACK_REASONS`；
    trailer 统计新增 `fallback_unrecognized`；超期行显示类别。
  - `check-collaboration-state.mjs`：`--require-candidate` 时检查候选范围内最新一个带 `Independent-Review:` 的提交上的
    全部 trailer，输出 `independent_review=recorded|nonconforming`，只提醒，无 trailer 时不输出。
  - `check-governance-consistency.mjs`：固定 5.2.7 新措辞；从脚本导入取值，要求协作文档列出全部取值。
- 新增或调整的自动门禁：无新增阻断门禁。`--strict` 的超期集合扩大到草案，退出码语义不变。
- 迁移、兼容和失败恢复：历史提案与 trailer 不改写。旧的自由文本 fallback 在 `--since` 跨越采纳点时计入
  `fallback_unrecognized`，属于历史债务展示；以采纳后的标签为起点统计时自然消失。
- 防止指标投机或规则自我弱化的约束：草案改为待复核不重新计时；延期仍受单次 14 天上限约束；
  `detail=` 不改变取值，取值外写法不能通过补充说明变为合规。

## 验证与证据层级

- 定向测试：`node --test scripts/tests/report-governance-health.test.mjs scripts/tests/collaboration-state.test.mjs`
  21/21（基线 18，新增草案计时、fallback 取值、候选提醒 3 项）；`--strict --since=v1.23.0` 当日退出 0，
  `--today=2026-10-05` 将两份现存草案报告为 `overdue ... draft 16d/15d` 并退出 3；
  `--since=v1.22.0` 输出 `total=34 cross_provider=1 invalid=4 same_provider_without_fallback=0 fallback_unrecognized=29`。
- `make verify-change` / `make verify-l2` / `make verify-release`：Windows 经 `node scripts/run-repo-bash.mjs`
  运行 `verify-governance.sh` 230/230、`verify-change.sh`（level=auto，治理路径，tools=none）通过；L2/Release 不适用。
- OCR 行级评审（5.5，candidate 档）：自由臂先盲跑 2 条 LOW，约束臂 5/5 文件、新增 3 条 LOW、`constrained-only=0`；
  空格、多 trailer 漏检和变量命名已在同一写任务修复，证据在仓库外 `_codex-evidence/kpanel-ocr-draft-sla-fallback-20261002`。
- 隔离真机或浏览器证据：不适用（无产品运行时变化）。
- 公开产物证据：不适用。
- 生产部署安全核对（仅在明确授权后；不作为质量验证证据）：不适用。
- 未验证项（原候选交付时）：同一 SHA 的候选 Linux CI和其他提供商独立复核当时尚未完成；
  本轮已完成下述非作者复核，RC4 回执文件的同 SHA 候选 CI / main CI / 归档由唯一发布任务另记。

## 独立复核

- 复核人 / 智能体：RC4 独立验证 / 发布任务 Codex，未参与原方案与实现；2026-10-05。
- 复核提供商 / 实现提供商：Codex / Claude
- 是否独立读取原始证据：是；读取原候选精确差异、历史原件与当前主线 blob / Git 祖先关系，核对范围及测试责任归属。
- 假设与方案评审结论：PASS；固定六项矩阵见末尾。代码行为已等价采纳，不把原候选待复核字段当当前任务队列。
- 门禁是否被削弱、绕过或只对样例优化：否；固定 fallback 是计数与提醒，草案计时复用原 14 天与延期边界，无新增产品门禁。
- 复核状态：通过（仅规范行为与历史回执；不等于观察收益或 RC4 发布已通过）。

## 回滚

- 源码 / 规范回滚点：基线 `642cd059`；回退本候选提交即可。
- 数据或配置备份：无数据或配置变化。
- 触发回滚的客观条件：观察窗口内防回归指标 1 超阈值且复审认定规则只制造延期文书；或候选提醒导致
  `--require-candidate` 退出码变化。
- 回滚步骤和回滚后复核：`git revert` 本候选提交，运行 `scripts/verify-governance.sh` 与 `--strict`，确认恢复基线输出。

## 采纳决策与结果

- 决策：补录等价已采纳行为的历史提案；不重复合并原候选代码。
- 决策依据和日期：2026-10-05，原有效行为已在 RC3 主线；本轮非作者独立复核完成。
- 观察窗口结果：截至本回执没有完整同口径窗口复测；仍以采纳后至 v1.25.0 稳定版发车前的质量审计为触发条件。
- 实际收益、回归和意外影响：未证明；规则正确实施与窗口效果分别判断，不以主线已有实现代替收益证据。
- 是否更新永久规范、测试、工作流或验收模板：是（见范围）
- 规范验收结论（仅规范变更）：PASS
- 规范验收停止依据：固定六项矩阵完成，原有效行为与当前主线等价；无阻断或重要事项。历史观察不足保留为未验证项，不扩大本轮范围。
- 后续事项：
  1. 两份现存草案提案在本规则采纳后由其负责人或集成任务处置（完成复核、拒绝或写明延期）；本候选不代为改写。
  2. 观察窗口结束时按 `provider-unavailable` / `provider-failed` 比例判断跨提供商复核是否为环境缺口
     （如安装另一提供商的非交互 CLI），再决定是否调整 5.2.4。
  3. 原候选登记的 `--strict` 与 `check-security-audit-coverage --require` 接线问题未纳入该历史范围；
     当前执行入口以现行规范及脚本为准，不将本条当作 RC4 门禁缺失结论。

## 2026-10-05 等价采纳回执

- 原候选：`20ec6fd3` 实现与 `eead4d24b472a8aa518dcde65d010ee13050994c` 交付；原规范基线 `642cd059b6d070f1496a07c96040683f2e265974`。
- 当前已接受主线：`7621e52d87b5b7f8879cfe4bff87946dde0fb2f1`，RC3 验收提交。
- `scripts/report-governance-health.mjs`、`scripts/check-collaboration-state.mjs`、提案模板与两份相关测试在原候选和当前已接受主线为相同 blob。
  该范围最近主线变更为 `c91fc78414bf4cec757af431e347a09e68c02f64`，是 RC3 主线祖先；这是内容与拓扑证明，不伪造原分支已合并的祖先关系。
- 当前共享规范已经包含草案 / 待复核 14 天 SLA、集成时更新状态、固定 `provider-unavailable` / `provider-failed` 以及只计数提醒的行为。
  `check-governance-consistency.mjs` 的旧候选其余差异涉及后续主线规则；本轮不覆盖新规则，不重放空白 / 注释差异。
- 原 OCR 原件：仓库外 `C:/GitHub/_codex-evidence/kpanel-ocr-draft-sla-fallback-20261002`，5/5 文件有逐文件处置，
  自由臂问题与约束臂问题的修复归属保持原提交；本回执不将历史 OCR 算作 RC4 新一轮。
- 当前行为验证复用 RC3 精确源码及 CI / 验收原件，RC4 完整治理门禁将校验这份回执的状态结构。
  原 230/230 属原候选，不能改名为 RC4 结果；RC4 同 SHA CI 和归档尚未完成，不在此预填。

| 固定维度 | 复核证据与结论 |
| --- | --- |
| 正确性 | 草案计时、固定 fallback、历史不改写和只提醒符合实际治理需求；行为已等价采纳，状态与当前 Git 事实一致。 |
| 一致性 | 5.2 / 跨智能体手册引用当前唯一脚本；固定取值由原唯一导出维护，不重放旧规范覆盖后续主线。 |
| 完整性 | 原目标、范围、角色、权限、证据、退出码和回滚完整；本回执补采纳映射与原候选 / RC4 结果归属。 |
| 可执行性 | 时限与 trailer 分类由既有机器入口执行；两份测试与当前主线相同，候选提醒不改变退出码。 |
| 效率与比例性 | 只补未入库历史提案和复核回执，复用已接受的等价代码证据；不重跑旧候选全量门禁或召回旧任务。 |
| 可演进性 | 原窗口、可证伪指标和回滚条件保持；有效性尚未证明，未来按稳定版质量审计同口径复测。 |

六项矩阵结论 `PASS`，未发现阻断、重要或新增一般事项。本次回执仅关闭原规范候选的独立复核记录缺口，
不宣称实际 fallback 入口改善、超期率下降、RC4 CI / 发布完成或已执行稳定版观察审计。
