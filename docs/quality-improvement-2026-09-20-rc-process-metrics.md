# RC 流程异常纳入指标视图

- 提案状态：草案（本地验证完成；待非作者独立复核与同一 SHA 的候选 Linux CI）
- 提案日期：2026-09-20
- 负责人：本治理候选作者；最终验证与集成由独立于作者的唯一集成任务负责
- 适用业务域：发布交付节奏与治理指标
- 基线提交 / 标签：`1ae9fcbbb8d9bb26ade95a8304fa3a5210c65aae` / `v1.21.0-rc.4`
- 关联缺陷、验收记录、CI 或事件：`docs/release-v1.19.0-rc.1..5-acceptance.md`、
  `docs/release-v1.20.0-rc.1..3-acceptance.md`、`docs/release-v1.21.0-rc.1..4-acceptance.md`
- 提交、推送、发布和生产权限：用户已授权本地候选提交；本任务不推送、不写 `main`、不打 tag、不发布、不部署

## 观察证据

- 可重复观察（基线 `1ae9fcbb`，2026-09-20）：`node scripts/report-release-metrics.mjs` 的全部指标只统计稳定版标签。
  `scripts/report-release-metrics.mjs` 的 `collectReleases` 以 `isStableReleaseTag` 过滤标签，
  `readAcceptanceHistory` 的 `ACCEPTANCE_FILENAME` 为 `release-(v\d+\.\d+\.\d+)-acceptance\.md`，
  两处都把 `-rc.N` 记录排除在外。
- 精确环境、版本、时间范围和样本量：本机 Windows 11 + Node 24；仓库内 12 份 RC 验收记录
  （v1.19.0 的 5 份、v1.20.0 的 3 份、v1.21.0 的 4 份），对应 12 个已合入 `main` 的 RC 标签，
  时间范围 2026-09-14T17:25Z 至 2026-09-20T07:27Z。
- 原始命令、日志、截图或验收记录：
  - `node -e "...validateAcceptanceMetrics(rc.4)"` → 0 errors；`readAcceptanceHistory(rc.4)` → 长度 0。
  - 逐份读取 RC 验收记录的 `已记录发布流程异常或无效证据拦截次数` 字段：
    v1.19.0 列车 9/7/3/4/1 合计 24；v1.20.0 列车 3/1/1 合计 5；v1.21.0 列车 2/4/6/10 合计 22；总计 51。
  - 同期稳定版视图记录的对应数字：v1.19.0 = 2，v1.20.0 = 1。
- 已确认事实：
  1. RC 验收记录已按 `PROJECT_RULES.md` 的既有要求携带 `release-process-metrics` 与
     `release-process-incidents` 证据块，并通过 `--validate-acceptance` 的结构校验。
     证据已经存在且合规，缺的只是聚合视图。
  2. RC 记录不进入 `readAcceptanceHistory`，因此 RC 之间的重复流程异常指纹从未被比较过。
  3. 51 次 RC 流程异常中，`其中生产写操作开始后异常次数` 全部为 0，与"RC 属预览通道、无生产写操作"一致。
- 未确认信息与数据缺口：RC 异常的根因分布未经人工核对；本提案不判断根因，也不推断永久处置是否真实成立。
- 为什么不是一次偶发环境故障：这是数据流结构决定的恒定行为，与运行环境无关，12/12 份 RC 记录一致复现。

## 原因假设

- 待验证根因：指标体系建立时以"正式发布"为唯一统计单元，RC 被视为过程而非可统计对象；
  随着单个稳定版的 RC 数量从 3 增至 5，发布摩擦的主要发生地转移到了统计口径之外。
- 可能的替代解释：(a) RC 异常本就不值得统计；(b) 稳定版异常下降是真实改善，与 RC 无关。
  对 (a)：RC 异常消耗的是同一批人和同一条发布链路，且已按规范记录，排除它等于自愿丢弃已付成本的证据。
  对 (b)：本提案不修改任何稳定版数字，两种解释可以并存——新视图只增加一个此前不可见的维度。
- 能证伪本假设的结果：若后续两个发布列车的 RC 异常合计显著低于同期稳定版异常，
  则"摩擦转移到 RC"不成立，本节应改写。
- 是否涉及 `kejilion.sh`、真实系统资源、Panel/Agent 权限边界或兼容契约：否。
  改动只读取已存在的 Git 标签与仓库内验收记录，不触碰产品代码、权限或宿主机。

## 产品原则与核心思想对齐

- 对业务真源和双端互通的影响：无。验收记录仍是唯一真源，本改动只增加读取路径，不新增数据来源。
- 对轻量、低资源、单管理员控制面的影响：无。改动仅限治理报告脚本，不进产品二进制。
- 对安全、稳定、性能、体验、数据/迁移的影响：无。只读、无迁移、无新依赖；
  报告耗时增加一次 `for-each-ref` 的复用结果与 12 次 `git show`，与既有稳定版路径同量级。
- 不应引入的复杂度或第二真源：不新建 RC 专用的异常格式、阈值或校验器。
  重复指纹沿用 `detectRepeatedProcessIncidents` 与同一个 `PROCESS_INCIDENT_REPEAT_WINDOW` 常量，
  标签采集沿用同一个 `collectTaggedReleases`。

## 基线、目标与观察窗口

| 指标 | 当前基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| 主指标：发布摩擦可见比例（进入指标视图的流程异常数 ÷ 已记录流程异常总数） | 稳定版 182、RC 0 可见，RC 51 不可见（可见比例 78.1%） | 100%：每份已存在的 RC 验收记录都出现在报告中 | `report-release-metrics.mjs` 的预览通道小节 | v1.21.0 起 2 个稳定版列车 |
| 防回归指标：稳定版指标不变 | 稳定版小节当前输出 | 同一 `--now` 下与基线逐字节一致（仅允许新增分节空行） | 基线与候选的报告输出 diff | 每次改动 |
| 防回归指标：治理回归集 | 基线 197 项通过 | 保持通过，新增用例只增不改（候选为 203 项） | `bash scripts/verify-governance.sh` | 每次改动 |

数据不足时写"先建立基线"，不得把缺失值当作零缺陷或成功。本提案的 RC 重复指纹基线为 1，
不代表 RC 只有 1 次重复根因——见"未验证项"。

## 范围与非目标

- 允许修改：`scripts/report-release-metrics.mjs`、`scripts/tests/report-release-metrics.test.mjs`、
  本提案文档、`CHANGELOG.md`。
- 明确不修改：`PROJECT_RULES.md`、任何门禁的通过/失败条件、`--validate-acceptance` 的行为、
  稳定版指标的口径与数值、`historicalReleases` 的校验规则、验收记录模板、已存在的验收记录。
- 受影响用户旅程：无终端用户旅程；使用者是发车前质量审计与治理复盘。
- 风险等级与所需验收层级：L2。只读报告扩展，不改门禁语义，但文件位于治理路径，
  按 `PROJECT_RULES.md` 5.2.6 需候选分支与同一 SHA 的 Linux CI。

### 规范验收合同

按 `PROJECT_RULES.md` 5.3 的"规范验收契约 v1.0"填写：

- 精确规范基线：`1ae9fcbbb8d9bb26ade95a8304fa3a5210c65aae`。
- 冻结的允许范围：上列"允许修改"四个文件。
- 明确非目标：不把 RC 纳入发布频率、前置时间、变更失败率、部署频率或稳定版重复指纹；
  不为 RC 重复指纹设立门禁；不修改 `PROJECT_RULES.md`。
- 权威入口和正常授权路径：`scripts/report-release-metrics.mjs`（报告与 `--validate-acceptance` 同一入口）；
  发车前由 `quality-audit-kpanel` 消费报告。
- 固定验收矩阵：
  - 正确性：RC 聚合值与逐份验收记录字段独立核对一致（51 / 22-5-24 / 重复 1）。
  - 一致性：不新建第二套异常定义、窗口常量或采集路径；`check-governance-consistency` 通过。
  - 完整性：新小节自带口径说明、数据完整性列与已知限制。
  - 可执行性：唯一入口即现有脚本，无新命令、无新参数、无新配置。
  - 效率与比例性：只读扩展，不引入门禁，不要求任何人补写历史记录。
  - 可演进性：RC 重复指纹先只观察，取得两个列车的数据后再决定是否设门禁。
- 预先确定的回归集与证据：
  `node --test scripts/tests/report-release-metrics.test.mjs`、`bash scripts/verify-governance.sh`、
  基线与候选的报告输出 diff（固定 `--now`）。
- 本轮停止条件：上述矩阵全部完成、阻断为零、稳定版输出逐字节未变。
- 范围外发现如何进入后续事项：写入本文件"后续事项"，不在本轮扩大改动。

## 备选方案与取舍

| 方案 | 收益 | 风险 / 成本 | 是否采用及理由 |
| --- | --- | --- | --- |
| A：RC 并入现有稳定版指标 | 单一视图，无需新小节 | 改写发布频率、前置时间、变更失败率的既有含义；RC 无生产完成时间，会污染中位数与失败率分母；违反"不改写历史统计口径" | 否 |
| B：RC 独立小节，只读，复用既有异常定义与窗口常量（本方案） | 摩擦全部可见；稳定版数字逐字节不变；无新真源 | 报告变长；RC 重复暂不可门禁 | 是 |
| C：同时为 RC 重复指纹设门禁 | 强制处置 RC 重复 | `historicalReleases` 当前只接受稳定版标签，RC 重复无法声明，门禁将无法被满足；且无基线即设阈值属于无证据收紧 | 否，列入后续事项 |
| D：新建独立的 RC 指标脚本 | 与稳定版完全隔离 | 产生第二个入口和第二套定义，违反唯一入口原则 | 否 |

## 最小改动方案

- 预计修改文件或唯一入口：`scripts/report-release-metrics.mjs`（唯一入口不变）。
  - 新增 `isPreviewReleaseTag` / `previewReleaseTrain` 标签分类；
  - 把 `collectReleases` 的函数体提取为 `collectTaggedReleases(repo, commit, limit, accepts)`，
    `collectReleases` 与新增的 `collectPreviewReleases` 各传一个过滤器，采集逻辑保持单一实现；
  - 新增 `summarizePreviewMetrics`，复用 `detectRepeatedProcessIncidents` 与
    `PROCESS_INCIDENT_REPEAT_WINDOW`，按发布列车分组并标记在途列车；
  - `renderMarkdown` 在稳定版小节与其口径说明之后追加"预览通道（RC）流程异常"小节；
    `report.preview` 缺失时不输出该小节。
- 新增或调整的自动门禁：无。本改动不增加任何失败条件。
- 迁移、兼容和失败恢复：无数据迁移。JSON 输出只增加 `preview` 键，既有键不变；
  `renderMarkdown` 对旧 report 对象保持向后兼容（已有用例覆盖）。
- 防止指标投机或规则自我弱化的约束：
  - 未填报 `release-process-metrics` 的 RC 不计入分母，缺失不推断为 0；
  - RC 重复指纹不进入任何门禁，不得据此宣称"RC 无重复"；
  - 小节口径说明写明指纹按精确字符串比较，同一根因写成不同指纹时不会被识别为重复；
  - 稳定版指标以逐字节 diff 守护，防止本改动顺手调整既有数字。

## 验证与证据层级

- 定向测试：`node --test scripts/tests/report-release-metrics.test.mjs` → 23/23 通过（基线 17/17，新增 6 项：
  标签分类、按列车聚合、缺失不推断为零、markdown 分区与向后兼容、跨列车重复归属、stableTags 必传）。
- `make verify-change` / `make verify-l2` / `make verify-release`：
  `bash scripts/verify-governance.sh` → 203/203 通过，
  `check-governance-consistency`、`report-governance-health --validate`（16 份提案）、
  `report-dependency-freshness --validate-only`（11 组）、`check-release-acceptance-coverage` 均通过。
- 交叉核对：对 12 份 RC 验收记录用一次性独立脚本重算合计与重复指纹，
  与脚本输出一致（51 / v1.21.0=22、v1.20.0=5、v1.19.0=24 / 重复 1 条
  `release-monitor/github-api/anonymous-rate-limit`，v1.21.0-rc.2 ← rc.1）。
- 稳定版不变证据：同一 `--now 2026-09-20T13:00:00Z` 下基线与候选的报告输出 diff，
  稳定版部分逐字节一致，唯一差异是新小节前的一个分隔空行。
- 隔离真机或浏览器证据：不适用（无用户可见变更，无本地可复现页面）。
- 公开产物证据：不适用（不进产品二进制与发布产物）。
- 生产部署安全核对：不适用；本任务不触及生产。
- 未验证项：
  1. 已完成：候选分支 `docs/release-metrics-rc-view-20260920` 于 2026-09-20 推送，
     `6c5f8e01` 的 Linux `CI` 成功（run 35515208788，`event=push`，head_sha 相符）。
     本次更正提交后需在新的精确 SHA 上重新取得候选 CI 成功，才满足 5.2.6。
  2. 已完成：非作者独立复核，结论 `PASS WITH FOLLOW-UP`（见「独立复核」一节）。
     复核者未执行的项：候选 Linux CI（无推送权限）、自由臂是否真的先于约束臂落盘
     （仓库外证据无版本控制，该条款本质依赖执行者自陈）、基线检出的 `verify-governance.sh`
     全量实测（该脚本会创建临时 clone/worktree，与只读边界冲突，基线 197 由候选 203 减新增 6 推得）。
  3. RC 重复指纹真实数量——当前实现按指纹精确字符串比较，
     基线数据中至少存在 4 条同根因（Windows PowerShell 向远端 shell 传变量）但指纹不同的记录
     （`remote-registry-inspect/ssh-loop/powershell-variable-expansion`、
     `release-shell/powershell-interpolation/variable-colon-adjacency`、
     `script-validation/remote-shell/local-variable-expansion`、
     `registry-inspection/ssh-loop/powershell-expansion`），本轮不识别为重复，也不改写它们。

## 独立复核

- 复核人 / 智能体：Claude（干净会话，未参与本提案编写或实现），2026-09-20，复核对象 `6c5f8e01`
- 复核提供商 / 实现提供商：claude / claude（本机无 Codex 非交互 CLI，跨提供商不可用，按
  `docs/multi-agent-collaboration.md` 降级为与实现分离的干净会话）
- 是否独立读取原始证据：是。复核者不 import 候选模块，自写解析器重算 12 份 RC 验收记录，
  自写比较逻辑重算重复指纹，并在基线与候选两个检出各跑一次固定 `--now` 的报告做逐字节对拍。
- 假设与方案评审结论：
  - 独立重算与候选输出逐项一致：RC 合计 51、分列车 22 / 5 / 24、`post-production` 0、重复指纹 1 条。
  - 口径判断独立成立：正确口径是声明的次数字段（51）而非 incidents 条目数（46），
    依据是既有校验要求 `Σ incident.count` 等于声明字段。
  - 稳定版未被改写：Markdown 前 87 行 `cmp` 零字节差异；JSON 剥离 `preview` 键后严格相等。
  - 缺失不推断为零：复核者另构造 6 组候选测试未覆盖的形态（含「未记录」字段、全未填报、
    指纹在两个更早 RC 重复、列车乱序、窗口边界距离 4/5、post-production>0），行为均符合声明口径。
  - 「`historicalReleases` 只接受稳定版标签」的前提经代码核实属实，方案 C 的排除有据。
- 门禁是否被削弱、绕过或只对样例优化：否。对仓库全部 204 份验收记录用基线与候选两版脚本
  分别跑 `--validate-acceptance`，逐份比对退出码与 stderr，**0 处差异**；预览路径在
  `--validate-acceptance` 的 return 之后，不在校验路径上，且不含任何 `throw`/`exitCode`。
- 复核状态：通过（带后续事项）
- 规范验收结论：`PASS WITH FOLLOW-UP`
- 复核提出的重要事项及处置：提案「基线、目标与观察窗口」与「验证与证据层级」记录的回归集数字
  取自补充测试之前的运行，与被复核提交不符（实际 23/23、新增 6 项、203/203、16 份提案，
  基线 197）。已在本次提交更正；偏差方向均为「实际通过数更多」，未隐藏失败，未削弱门禁。

## 回滚

- 源码 / 规范回滚点：`1ae9fcbbb8d9bb26ade95a8304fa3a5210c65aae`。
- 数据或配置备份：不适用，无持久化状态与配置变更。
- 触发回滚的客观条件：
  1. 稳定版指标输出相对基线出现任何非空行差异；
  2. 治理回归集出现失败；
  3. 新小节被用作任何门禁的判据。
- 回滚步骤和回滚后复核：`git revert` 候选提交（单一提交、无迁移），
  随后重跑 `bash scripts/verify-governance.sh` 与固定 `--now` 的报告 diff。

## 采纳决策与结果

- 决策：待定（草案）
- 决策依据和日期：待独立复核与候选 CI 后填写
- 观察窗口结果：未开始
- 实际收益、回归和意外影响：未报告
- 是否更新永久规范、测试、工作流或验收模板：本轮只更新脚本、测试与 `CHANGELOG.md`，不改永久规范
- 规范验收结论（仅规范变更）：待复核后填写
- 规范验收停止依据：待复核后填写
- 后续事项：
  1. 指纹根因聚类：现行重复检测按精确字符串比较，同一根因的不同指纹不被识别。
     取得两个发布列车的 RC 数据后评估是否引入聚类键，届时按 5.3 单独验收。
  2. RC 重复是否设门禁：需先放宽 `historicalReleases` 使其可声明 RC 标签，属独立治理变更，
     不在本轮范围。该前提已由独立复核在代码中核实属实。
  4. `OCR-Review:` trailer 的 `range` 终点写的是 amend 前的提交对象，从已推送分支不可达。
     独立复核确认这是项目既有惯例（近 12 个带 trailer 的提交同样写法），非本候选引入；
     建议后续统一改为最终候选 SHA，或在仓库外证据中记录两者 tree 差异。属项目级后续事项。
  5. 双臂分级计数口径：trailer 的 `valid=H/M/L` 目前只统计自由臂发现，约束臂新增的 LOW 未计入。
     5.5 未明确该字段是否含约束臂，且不影响退出条款（只看 `constrained-only`）；
     建议后续统一为「双臂全部成立发现的分级计数」。
  3. 发布列车总成本视图：把同一列车的 RC 异常与稳定版异常合并呈现，
     需先确认不改写稳定版既有口径，列入后续评估。
