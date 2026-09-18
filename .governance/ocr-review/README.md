# OCR 行级评审辅助状态

本目录是 `PROJECT_RULES.md` 5.5 的状态与升级基线，执行入口为
`.codex-workflows/ocr-line-review.workflow.yaml` 与 `scripts/ocr-delegate.mjs`。版本 pin 的唯一
真源是 `dependency-policy.json` 的 `code-review-assistant` 组；本文不复写当前版本号。
单次运行结果不入库，只以候选提交的 `OCR-Review:` trailer 留痕。

## run-0 基线（2026-09-18，接入前评测，原样记录不追溯改写）

- 样本：`d028518a^..cc1e4118`（批量执行快捷命令 + 4 小时韧性），6 文件 +146/-11；
  OCR v1.12.5 委托模式 + 同 diff 自由评审对照臂。
- 约束臂 6 条、自由臂 13 条，重合 46%；唯一 HIGH（批次 header 与移动端 host 选择条重叠锁死
  批量模式，当天由 `dfd3d65a` 修复）只在自由臂出现 → 5.5 规定自由臂必做。
- 主要缺陷在圈选启发式：上游 `default_path` 剔除 `.test.ts`，约束臂误判测试覆盖。
  `.opencodereview/rule.json` 的 `include` 修复后，同一范围可评审文件由 4/6 变为 6/6。
- 已知上游限制：v1.12.5 没有 Vue 专属系统规则，`.vue` 落到通用 `default` 规则；KPanel 不自写
  覆盖规则，升级回放时记录上游是否补齐。

## canary 回放（升级通过条件）

对新版本在 `d028518a^..cc1e4118` 上依次满足：

1. `node scripts/ocr-delegate.mjs preview --from d028518a^ --to cc1e4118 --format json`
   报告 6/6 可评审，`BatchTerminalPanel.test.ts` 与 `TerminalView.batch.test.ts` 不在
   `excluded_files`；
2. `rule` 能为全部 6 个文件解析出规则组，`.vue` 与 `.test.ts` 均有规则；
3. 约束臂重新发现两条已知成立缺陷：`TerminalView.vue` 交互模式 `TerminalQuickCommands`
   缺 `terminalMode === 'batch'` 守卫（F1）；`BatchTerminalPanel.vue` 重试退避 `setTimeout`
   不感知 abort（F2）；
4. 入口测试 `node --test scripts/tests/ocr-delegate.test.mjs` 通过。

条件 1-2、4 是确定性下限；条件 3 取决于执行智能体，未命中时再跑一次并记录两次结果，仍未命中则
保持旧 pin。样本仅作回归夹具，不得为通过 canary 而调整规则文本或只对该样本优化（5.2.3）。

## 回放记录

| 日期 | 候选版本 | 条件 1 / 2 / 3 / 4 | 决定 | 证据位置 |
| --- | --- | --- | --- | --- |
| 2026-09-18 | 1.12.5（接入基线） | PASS / PASS / 未报告（run-0 在接入前以 WSL 原样模式发现 F1/F2）/ PASS | 采用为初始 pin | 本文 run-0 |
