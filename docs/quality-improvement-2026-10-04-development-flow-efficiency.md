# 开发与发布执行治理：并行核验、输入资格和交付闭环

- 提案状态：待复核（实现完成；用户已授权候选推送及汇入主线，试行前完成非作者复核和精确候选 CI）
- 提案日期：2026-10-04
- 负责人：治理候选实现者与唯一集成负责人
- 适用业务域：开发、验证、交接、发布执行效率和治理稳定性
- 基线提交 / 标签：`edf43af7947b779b5517a2e55bc02b9b9439ad70` / `v1.24.0`
- 关联证据：RC1–RC11、v1.24.0 验收记录；Release run 37158469376；CF run-16 元数据
- 提交、推送、发布和生产权限：用户明确授权形成高质量候选并汇入主线；允许本地提交和 SSH 候选/主线推送。产品版本、Tag、Release、镜像和生产部署不在本次范围。

## 观察证据

RC1–RC11 的结构化异常合计 144 次；v1.24.0 另记录 86 次，其中 security-audit 指纹 64 次。
两种猜路径指纹合计 34 次。run-16 从 2026-10-03T10:46:38Z 至 21:35:14Z，稳定候选 21:38:16Z 冻结。
实际审查与恢复缺少分段计时，不能将全部审计用时视作浪费。

GitHub Release job 历时 795 秒，Verify source 为 449 秒；其中 Web、Go 与部署检查串行。
RC11 的 L3 为 1129 秒，stable L3 为 1069 秒。现有 make test、race、vet 和前端构建可以按独立资源并行，
同一源码的 Web 构建不应在同一核验入口中重复。近期 64 条独立复核 trailer 中 7 条格式无效。
19 份提案中 2 份草案未进入待复核 SLA，2 份待复核已有延期，生命周期仍需要闭环。

## 原因假设

固定检查串行、执行前缺少成组资格检查、跨 Shell 临时拼接，以及结构化交付未与 Git/证据身份对齐，
导致有价值验证被工具返工和重复步骤拉长。替代解释是必要安全审查、上游网络和复杂范围本身耗时；
不以 RC 数或异常总数直接判断产品质量。不改变业务真源、Panel/Agent 边界或脚本契约。

## 产品原则与核心思想对齐

改造只涉及开发工具与治理。所有现有测试、race、vet、安全扫描、部署/镜像契约、双端互通、L3、
候选/主线 CI 和公开产物核验保留。并行只限独立检查，限制并发、输出和时长；任何子项失败、取消、
超时、候选变化均不能成功。没有常驻服务、自动发布器或会话台账。

## 基线、目标与观察窗口

| 指标 | 基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| 源码检查串行等待 | Release Verify source 449 秒（单次样本） | 同一命令覆盖的独立阶段并行；记录真实阶段用时，不承诺单样本外推收益 | source-check 终态、CI 原始日志 | 接下来 2 个稳定列车 |
| 执行资格错误 | 两种猜路径指纹 34 次 | 可资格化的文件/字段/候选身份在长任务前一次拒绝 | task-preflight 与异常记录 | 同上 |
| 质量防回归 | 原有门禁与明确未知边界 | 不减少命令，不因取消/超时/损坏证据错误成功 | 固定回归集、非作者复核、Linux CI | 每次变更 |
| 规范状态闭环 | 草案不计 SLA；7 条无效 trailer | 草案和待复核共用原日期；新交付提前发现无效记录，历史不改写 | governance-health / collaboration-state | 同上 |

## 范围与非目标

- 允许修改：`PROJECT_RULES.md`、`AGENTS.md`、`CLAUDE.md`、`Makefile`、`.github/workflows/release.yml`；
  `docs/project-management.md`、`docs/multi-agent-collaboration.md`、`docs/development-quality-standard.md`、
  `docs/quality-improvement-proposal-template.md`、本提案、9 月 19 日分支归档和 9 月 20 日 RC 指标提案的生命周期行；
  `.codex-workflows/{README.md,session-collaboration.workflow.yaml,release-kpanel.workflow.yaml,evolve-kpanel.workflow.yaml}`；
  `scripts/{run-source-checks.mjs,verify-source-lane.sh,task-preflight.mjs,verify-change.sh,verify-governance.sh,run-release-gate.sh,check-governance-consistency.mjs,check-collaboration-state.mjs,report-governance-health.mjs}` 及对应测试。
- 非目标：产品代码、版本、依赖 pin、CF hunting/validator、生产、历史测试/验收内容重写；不自动补审、审批、发布或清理他人工作树。
- 风险等级：L2 治理候选；本地治理回归 + 独立规范/实现复核 + 精确 SHA Linux 候选 CI + 主线 CI。

### 规范验收合同

- 精确规范基线：`edf43af7947b779b5517a2e55bc02b9b9439ad70`。
- 冻结范围：上述文件；权限和非目标如上。复核只能在 PROJECT_RULES 5.3 的六维矩阵内判断。
- 权威入口：既有 verify-change / verify-governance；run-source-checks 承担唯一源码发布检查；task-preflight 只资格化任务输入和证据，不授予权限、不替代业务验收。
- 正常执行路径：管理树读取精确基线 → 专用 linked worktree 完成 ready 预检 → 范围内实施/本地回归并提交非空候选
  → handoff 校验 clean、候选、范围与原始证据 → 非作者六维复核 → 同 SHA 候选 Linux CI 成功
  → SSH 快进 main → 同 SHA 主线 CI 成功 → 归档本任务分支。任何资格/证据/复核/CI 失败均停止对应后续写入。
  Linux L3 和 Release 均运行 `node scripts/run-source-checks.mjs`，成功后继续原安全、构建与发布步骤；
  Windows 通过 `node scripts/run-repo-bash.mjs scripts/verify-change.sh <base>` 使用 Git Bash；没有 Make 时，
  本地治理回归仍可运行，产品 L2/L3 必须转固定 Linux runner，不将局部成功标为完整产品验收。
- 固定矩阵：正确性（失败关闭、身份准确）；一致性（文档/入口/工作流同源）；完整性（范围、证据、终态、权限）；
  可执行性（正常 Linux/Windows 路径与异常回归）；效率与比例性（并发有界、同命令覆盖、小任务无新增重步骤）；
  可演进性（观察真实效果、可回滚、历史不改写）。
- 预定回归：新增源码并行/预检用例；collaboration-state、governance-health、release-gate-runner、release-channel-contract、治理全回归；工作流结构校验；候选和主线 Linux CI。
- 停止条件：矩阵完成，阻断 0，其他事项有分级与跟进；合入后观察 2 个稳定列车，尚未运行的新 Release 不冒充实测提速。
- 范围外发现：记录后续项，不扩大本轮产品改动。

## 备选方案与取舍

直接降低门禁、跨身份复用成功和放松审计均拒绝。采用固定命令有界并行、资格检查和机器交付；
沿用已有 10 月 2 日草案时限候选中相关实现，在当前基线重新验证，不接管原工作树。

## 最小改动方案

1. 固定 Web/Go/部署检查分组，统一源检查入口、并发上限 2、过程树取消、日志与终态；L3 和 Release 共用，去除同入口重复 Web 构建。
2. 只读任务预检以 JSON 承载精确基线、范围、工具/环境/参数和证据摘要；ready/handoff 两阶段检查，不执行传入命令。
3. 草案进入 SLA、标准化 fallback 原因、候选提醒无效复核；旧提案如实更新状态与未完成证据，不追认采纳。
4. 共享文档与工作流接入，审计在单功能候选阶段前移，发布接管、证据复用和 RC 聚合规则统一。

## 验证与证据层级

- 当前状态：验收合同与正常路径已冻结，实现完成；非作者复核及精确候选/主线 CI 待完成。
- 本地中间实现的治理全回归 247 项通过；新增任务预检与源码入口定向 12 项通过。
  最终候选仍从同一入口完整复跑并以精确 SHA 原始日志为准，不跨修订复用成功。
- 受控调度基准：Windows / Node v24.17.0，同一 Web/Go 各 1000 ms、部署 100 ms 的进程夹具，
  串行和并行各 3 轮，中位数 2320 / 1248 ms，等待减少 46.2%。这只验证调度机制，不是实际发布收益。
  原始每轮日志、JSON 及脚本保存在仓库外；最终实现增补了日志摘要核验，定向回归通过。
- 纯开发基础设施，无产品页面，acceptance 预览不适用；无产品产物发布或生产验证。
- 收益分为：本地可复现并行机制、Linux 实际执行、后续端到端发布效果，不相互替代。

## 独立复核

- 提供商可用性：PATH 中无 claude/gemini/qwen 可调用入口，用户目录可执行工具无另一提供商；使用独立干净复核上下文，记录 provider-unavailable，不能自我复核。
- 复核状态：待复核。
- 复核提供商 / 实现提供商：codex / codex（另一提供商不可用，独立干净上下文）
- 复核证据和六维结论：待完成。

## 回滚

回滚点为上述基线。若正常任务误阻断、检查减少、残留进程、日志丢失或流程/质量指标恶化，
revert 本治理提交，复跑固定回归和 Linux CI；保留原件，不改写共享历史。

## 采纳决策与结果

- 决策：待独立复核及精确候选 CI 后决定试行。
- 观察窗口结果：未开始；未宣称实际发布已提速。
- 规范验收结论：待完成。
- 后续事项：真实编码/等待/返工分段计时、跨版本风险同口径比较在两列车观察时补齐。
