# 质量改进提案：审计覆盖按并集计算，并补齐兼容、适配入口与验收校验

- 提案状态：待复核
- 提案日期：2026-09-22
- 负责人：Claude（治理写任务 `docs/security-audit-coverage-union-20260922`）
- 适用业务域：规范治理流程（`PROJECT_RULES.md` 5.4）；不改产品运行时
- 基线提交 / 标签：`c5aadd8a`（v1.21.0-rc.9 验收记录）
- 关联缺陷、验收记录、CI 或事件：`docs/quality-improvement-2026-09-21-security-audit-trigger.md`（前序提案，试行中）；
  `docs/release-v1.21.0-rc.9-acceptance.md` 第 42 行；`.governance/security-audit/run-6/run-metadata.json`；
  仓库外 run-4 证据 `C:/GitHub/_codex-evidence/kpanel-security-audit-run4-v1.21.0/run-metadata.json`
- 提交、推送、发布和生产权限：用户 2026-09-22 授权推进本候选（推送候选分支与后续流程）；主线由发布任务集成；不涉及 tag、Release 与生产

## 观察证据

前序规则随 rc.9 进入主线后首次实际运行，在临时 worktree 中复现（基线 `c5aadd8a`）：

- **run-4 原样入库会让 CI 失败。** run-4（v1.21.0 候选 `4c0694aa` 的 full 审计，2026-09-21 14:45 开始，早于规则合入）
  的元数据只有 skill 自带的 `project_mode`，没有 `scope_mode`。复制进仓库后 `--validate` 输出
  `run-4: scope_mode must be full or scoped`，`verify-governance.sh` 会因此失败，阻断 v1.21.0 稳定版。
- **覆盖链是线性的，而实际开发是并行的。** full 跑在发布分支（run-4 → `4c0694aa`），scoped 跑在功能分支
  （run-6 → `b8ba15f4`），两者 merge-base 为 `a5da3d78`，互不为祖先。补上 `scope_mode` 后模拟：结论为 `ok`，
  但 run-6 显示 `not_chained`，它审过的 4 个 Passkey 提交被算作未审；最早的 `7799be4a`（09-21 23:22）会从
  2026-10-06 23:22 起让稳定版预检判为 `scoped-required`，要求重审已审过的代码。
- **现有输出已被误读。** rc.9 验收把脚本的"23 个未审提交"写成"0 个未覆盖 Passkey 单元"。
- **适配入口未同步。** `AGENTS.md`、`CLAUDE.md` 都写了 `OCR-Review`（各 1 处），`Security-Audit` 为 0 处，违反 0.7。
- **稳定版覆盖检查字段没有机器校验。** 5.4 要求记录，但 `report-release-metrics.mjs` 不检查，漏记不会被发现。
- 为什么不是偶发：并行分支是本项目的常态（发布候选与功能候选同时进行），且前序规则的候选层本就要求在功能分支上
  做 scoped，所以线性链几乎永远无法推进；run-4 的字段问题会在它入库的第一刻发生。

## 原因假设

- 待验证根因：覆盖模型假设审计首尾相接；字段校验没有兼容规则生效前已开始的 run；新条款只写进规范和工作流，
  没有进入各智能体首先阅读的适配入口，也没有进入验收校验。
- 替代解释：并行 scoped 可以通过让审计者手动把 comparison_base 设成上一次 full 来规避。反驳：run-6 审计时
  run-4 尚未完成，审计者无法预知应接在哪个 full 之后；且这会把已审的其他分支改动重新纳入范围。
- 证伪结果：并集模型在真实拓扑上仍把已审提交判为未审，或把未审提交判为已审。

## 产品原则与核心思想对齐

- 业务真源：覆盖仍只从 run 元数据和 Git 历史计算，不新增状态文件。
- 轻量：普通任务无新增步骤；只有稳定版验收记录多一项已存在于模板的字段校验。
- 安全：并集只让"确实被某个已完成 run 审过的提交"计入覆盖，中止、部分和历史外 run 仍不计。
- 不引入：入口注册表解析、L3 编排器改动、新的 JSON 区块。

## 基线、目标与观察窗口

| 指标 | 当前基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| 主指标：已完成 scoped run 的覆盖被识别 | run-6 的 4 个提交全部未被识别 | 全部识别 | 覆盖检查 `covered` 字段 | 合入后 3 个稳定版或 30 天 |
| 主指标：run-4 原样入库时治理门禁 | 失败 | 通过 | `verify-governance.sh` | 同上 |
| 防回归：未审提交被误判为已审 | 0 | 0 | 定向测试 + 真实历史回放 | 同上 |
| 防回归：v1.21.0 起稳定版记录缺覆盖检查结论 | 无机器检查 | 0（`--validate-acceptance` 失败关闭） | 验收记录 | 同上 |

## 范围与非目标

- 允许修改：`scripts/check-security-audit-coverage.mjs` 及测试、`scripts/report-release-metrics.mjs` 及测试、
  `PROJECT_RULES.md` 5.4、`security-boundary-audit` 与 `release-kpanel` 工作流、`.governance/security-audit/README.md`、
  `AGENTS.md`、`CLAUDE.md`。
- 明确不修改：产品代码、历史 run 与历史验收记录、边界策略阈值、候选层提醒逻辑、L3 编排器。
- 风险等级与所需验收层级：L1（治理脚本）；按 5.3 规范验收。

### 规范验收合同

- 精确规范基线：`c5aadd8a`。
- 冻结的允许范围：同上。
- 明确非目标：全量间隔的数值调整（待 run-4 线索验证后另行评估）、`web/` 纳入边界、既有包内新能力的候选层识别。
- 权威入口：`check-security-audit-coverage.mjs`（覆盖判定）、`report-release-metrics.mjs --validate-acceptance`（验收字段）。
- 固定验收矩阵：5.3.2 六维。
- 预先确定的回归集：`node --test scripts/tests/check-security-audit-coverage.test.mjs scripts/tests/report-release-metrics.test.mjs
  scripts/tests/collaboration-state.test.mjs`；`bash scripts/verify-governance.sh`；在 `c5aadd8a` 上把 run-4 元数据原样复制进
  仓库后 `--validate` 通过、结论为 `ok`、`covered_by_scoped run-6=4`；不复制时仍列出 19 个真正未审的提交。
- 本轮停止条件：矩阵完成、回归集通过、无阻断项。

## 备选方案与取舍

| 方案 | 收益 | 风险 / 成本 | 是否采用及理由 |
| --- | --- | --- | --- |
| A 维持线性链，要求 scoped 的 comparison_base 接在最近 full 之后 | 代码不变 | 并行时审计者无法预知基点；重复审计其他分支 | 否 |
| B 覆盖按已完成 run 的区间并集 | 与实际分支拓扑一致，逐提交可追溯 | 需要改一个函数和测试 | 采用 |
| C run-4 字段问题只靠通知 Codex 手工补字段 | 不改代码 | 下一次 skill 默认输出仍会触发同一问题 | 否，改为兼容同义字段 |
| D 验收字段只写在模板里 | 无改动 | 与 5.4 旧问题同型：有条款无入口 | 否，加机器校验 |

## 最小改动方案

- 覆盖：`classifyRuns` 把已完成 run 分为 full、scoped（`scope_complete: true` 且有 `comparison_base`）、partial、
  interrupted、outside。待审提交 = 目标历史中不被任何已完成 full 包含的边界提交；其中落在某个 scoped 的
  `comparison_base..source_ref` 区间内的计入 `covered`，其余为未审。新边界包按未审提交逐个判定。full 间隔取源码
  提交时间最新的已完成 full，不再依赖 run 编号。输出去掉 `through`，改为 `covered_by_scoped run-N=计数` 和
  `nextScoped.comparison_base`（最近 full 的源码）。
- 兼容：`project_mode` 视为 `scope_mode` 的同义字段；两者同时存在且不一致时校验失败。
- 验收：v1.21.0 起的稳定版记录（按文件名识别，RC 不适用）必须有"覆盖检查"行并写明 decision；非 `ok` 时必须写明
  `run-<N>`，或"豁免"加 YYYY-MM-DD 截止日。
- 适配入口：`AGENTS.md`、`CLAUDE.md` 在 OCR 条目后各补一条安全审计提醒。
- 防止规则自我弱化：并集不放宽任何"什么算已审"的条件，只修正"已审的提交是否被识别"。

## 验证与证据层级

- 定向测试：覆盖检查 14 个用例（新增真实拓扑的并行分支用例、并集区间与缺口、按时间取最近 full、`project_mode`
  同义与冲突）；验收字段 1 个用例（缺失、空值、无 decision、pending 未说明、run-N、豁免加日期、RC 与 v1.20.0 不适用）。
- 真实数据：在 `c5aadd8a` 上，不含 run-4 时 `covered_by_scoped run-6=4 run-2=11`、未审 19；含原样 run-4 时
  `--validate` 通过、`decision=ok`、未审 0。rc.9 验收记录的 `--validate-acceptance` 仍通过。
- `verify-governance.sh`（Windows Git Bash）：224/224 通过（基线 221，新增 3：覆盖检查 2、验收字段 1）；提案 19 份。
- `verify-change.sh`：本机无 Go，预检失败关闭；完整门禁由候选分支 Linux CI 执行。
- 未验证项：真实稳定版上的验收字段执行情况（需观察窗口）。

## 独立复核

- 复核人 / 智能体：待定
- 复核提供商 / 实现提供商：待定 / claude
- 是否独立读取原始证据：待定
- 假设与方案评审结论：待定
- 门禁是否被削弱、绕过或只对样例优化：待定（重点：并集是否会把未审提交误判为已审）
- 复核状态：待复核

## 回滚

- 源码 / 规范回滚点：`c5aadd8a`。
- 触发回滚的客观条件：并集把任何未审提交判为已审；或验收字段校验误伤合规记录。
- 回滚步骤：还原本提案提交，重跑 `verify-governance.sh`。

## 采纳决策与结果

- 决策：待复核后决定
- 后续事项：run-4 线索验证完成后按"成立的发现位于改动代码还是未改代码"评估全量间隔；`Security-Audit` trailer
  与豁免次数的统计入口（前序提案主指标）留待下一轮。
