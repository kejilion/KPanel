# 质量改进提案：open-code-review 行级评审辅助轻量接入

- 提案状态：待复核
- 提案日期：2026-09-18
- 负责人：Claude（治理写任务 `docs/ocr-line-review-assistant`）
- 适用业务域：开发评审流程（不涉及产品运行时）
- 基线提交 / 标签：`2cea520f`（v1.20.0-rc.1 验收记录之后）
- 关联缺陷、验收记录、CI 或事件：run-0 评测（`.governance/ocr-review/README.md`）；批量终端 HIGH 缺陷
  当天由 `dfd3d65a` 人工修复、未经任何门禁拦截；全量审计 2026-09-18 结论"缺口集中在无机器入口条款"
- 提交、推送、发布和生产权限：仅本地候选提交；推送、主线与发布未授权

## 观察证据

- 可重复观察：`d028518a^..cc1e4118` 上 OCR v1.12.5 委托模式约束臂 6 条、自由臂 13 条，重合 46%；
  唯一 HIGH 只在自由臂。上游默认圈选剔除 `.test.ts`：同一范围 4/6 可评审，接入 `.opencodereview/rule.json`
  后 6/6（本任务在 Windows 原生 Node 上复测，命令见 README canary 条件 1）。
- 已确认事实：委托模式不需 LLM 端点；OCR npm 启动器默认后台检查更新，需 `OCR_NO_UPDATE=1` 固定版本；
  v1.12.5 无 Vue 专属系统规则。
- 未确认信息与数据缺口：约束臂相对自由臂的独有有效发现率只有 1 个样本（run-0 为 0 条独有 HIGH），
  需试行积累；约束臂执行成本（run-0 约 97k tokens / 7 分钟）未在其他样本复测。
- 为什么不是偶发：圈选剔除测试文件是上游确定性规则，可在任意含测试的差异上复现。

## 原因假设

- 待验证：逐文件强制覆盖清单能减少自由评审漏看文件，且与 KPanel 质量标准引用结合后产生自由臂之外的有效发现。
- 替代解释：自由评审本身已覆盖同等范围，约束臂只增加成本（run-0 数据支持此解释）。
- 证伪结果：3 个稳定版周期内 `constrained-only` 累计为 0 → 按 5.5 退出条款移除。
- 不涉及 `kejilion.sh`、真实系统资源、Panel/Agent 权限边界或兼容契约。

## 产品原则与核心思想对齐

- 不改变业务真源、双端互通和产品运行时；工具安装在仓库外，不进入镜像与依赖图。
- 不形成第二真源：pin 只在 `dependency-policy.json`，圈选只在 `.opencodereview/rule.json`，KPanel 尺度只引用
  `docs/development-quality-standard.md`，上游规则文本不复制。
- 安全：入口只放行 LLM-free 子命令，源码不发往 OCR 端点。

## 基线、目标与观察窗口

| 指标 | 当前基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| 主指标：`constrained-only` 有效发现 | 未报告（run-0 自由臂把约束臂 6 条作为已知项，未独立测量） | 3 个稳定版周期累计 > 0 | 候选提交 `OCR-Review:` trailer | v1.20.0 起 3 个稳定版 |
| 防回归：圈选失真 | 上游默认 4/6 | canary 6/6，无 `default_path` 剔除测试 | canary 回放 | 每次升级 |
| 防回归：门禁与流程 | 无新增门禁 | CI、DoD、L0-L3 不变 | `verify-change` / 本提案范围 | 持续 |

## 范围与非目标

- 允许修改：`PROJECT_RULES.md` 5.5、`.codex-workflows/ocr-line-review.workflow.yaml` 与 README 索引、
  `docs/multi-agent-collaboration.md` 一条、`.opencodereview/rule.json`、`.governance/ocr-review/README.md`、
  `scripts/ocr-delegate.mjs` 及测试、`dependency-policy.json` 新组与报告器采集、治理一致性与 verify 脚本登记；
  自动参与挂点：`session-collaboration` 第 4 步与验证清单、`quality-audit-kpanel` 第 2 步、`AGENTS.md`/`CLAUDE.md`
  各一条、`check-collaboration-state.mjs --require-candidate` 的非阻塞 `ocr_line_review` 提醒及其测试。
- 明确不修改：CI、Definition of Done、L0-L3、发布工作流、验收模板、既有验收记录、产品代码。
- 风险等级：L0 规范 + 治理脚本（按 5.3 验收）。

### 规范验收合同

- 精确规范基线：`2cea520f`。
- 冻结的允许范围：同上；不新增门禁。
- 明确非目标：不把 OCR 结果作为合并/发布条件；不启用 OCR 默认（LLM 端点）模式。
- 权威入口和正常授权路径：`scripts/ocr-delegate.mjs` + 工作流；升级经依赖报告候选信号 → canary → 独立治理提交。
- 固定验收矩阵：正确性 / 一致性 / 完整性 / 可执行性 / 效率与比例性 / 可演进性（`PROJECT_RULES.md` 5.3）。
- 预先确定的回归集与证据：`node --test scripts/tests/ocr-delegate.test.mjs scripts/tests/report-dependency-freshness.test.mjs`、
  `check-governance-consistency`、`report-dependency-freshness --validate-only`、`workflow.py validate ocr-line-review`、
  canary 条件 1/2/4 实跑、`verify-change`。
- 本轮停止条件：上述回归全绿且独立复核无 HIGH。
- 范围外发现：`security-audit-skill` 组声明 `github-commits` 检测器但报告器无对应采集器，另立事项。

## 备选方案与取舍

| 方案 | 收益 | 风险 / 成本 | 是否采用及理由 |
| --- | --- | --- | --- |
| A 维持不引入 | 零成本 | 圈选/覆盖清单能力缺失 | 否：圈选缺陷已有可验证修复，值得有退出条款的试行 |
| B 默认模式接入 CI 门禁 | 自动化 | 外配 LLM 端点、源码外发、run-0 召回结构受限，门禁化会误导 | 否 |
| C 委托模式默认自动参与（挂在候选交付与质量审计流程，非阻塞提醒）+ 自由臂必做 + 试用退出 | 无需人工引导、零新门禁、可退出 | 每个代码候选增加一次评审成本 | 采用 |

## 最小改动方案

- 唯一入口：`scripts/ocr-delegate.mjs`（pin 读取、仓库外安装、禁更新、子命令白名单、禁 `--rule/--repo`）。
- 新增自动门禁：无；只新增入口单测与一致性文本校验（机器下限）。
- 迁移与恢复：删除本提案文件列表即回滚，无数据迁移。
- 防指标投机：canary 样本只作回归夹具，不得为通过而调整规则或只对样本优化；trailer 缺字段按"未报告"。

## 验证与证据层级

- 定向测试：见回归集；canary 条件 1/2/4 在 Windows 原生 Node 实跑通过（条件 3 依赖执行智能体，接入时未重跑）。
- `make verify-change`：以候选提交结果为准（见交付包）。
- 隔离真机、公开产物、生产部署：不适用（不涉及运行时）。
- 未验证项：约束臂成本在其他样本上的复测；主指标需试行积累；Linux 上首次安装路径未实跑。

## 独立复核

- 复核人 / 智能体：待指派（须未编写本方案）
- 是否独立读取原始证据：待定
- 假设与方案评审结论：待定
- 门禁是否被削弱、绕过或只对样例优化：待定
- 复核状态：待复核

## 回滚

- 源码 / 规范回滚点：`2cea520f`。
- 触发回滚的客观条件：5.5 退出条款成立；或入口被发现能把源码发往外部端点。
- 回滚步骤：还原本提案范围文件，保留 `.governance/ocr-review/README.md` 与提交历史；重跑治理核验。

## 采纳决策与结果

- 决策：待独立复核后决定进入试行
- 决策依据和日期：—
- 观察窗口结果：未报告
- 实际收益、回归和意外影响：未报告
- 是否更新永久规范、测试、工作流或验收模板：是（见范围），验收模板不改
- 规范验收结论（仅规范变更）：待复核
- 规范验收停止依据：—
- 后续事项：独立复核；`security-audit-skill` 检测器缺口另立事项。
