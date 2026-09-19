# 信任边界安全审计状态

本目录是 `PROJECT_RULES.md` 5.4 定义的审计账本与 findings 入库位置，执行入口为
`.codex-workflows/security-boundary-audit.workflow.yaml`。

- `run-<N>/`：每次审计的 coverage-ledger.json、findings.json 与三份报告；
  run 递增编号并以上一 run 为增量输入。
- 本规范生效后执行的 run（run-2 起）的 metadata 必须记录：精确基线、上游 skill
  来源的固定 commit、profile、实际成本（代理数/token/时长）与验证器结果。
  run-1 为原样迁入的历史基线（2026-09-18 全仓审计，skill 原生 profile 词汇
  standard≈full），不追溯补齐全字段、不改写内容。
- 试用退出窗口（PROJECT_RULES.md 5.4）从 run-1 起算：run-1 计为首个 full run，
  观察序列自 v1.20.0 稳定版列车开始计数。
- 首个增量基线（run-1，2026-09-18，基线 6340e078，45 单元 / 1 confirmed /
  12 加固项）迁入本目录后方可执行 run-2；迁移时保持文件原名不改。

单次运行不构成安全结论；覆盖声明只在账本维度上成立（见 PROJECT_RULES.md 5.4）。

新 run 先在仓库外的独立目录完成，父任务验证并审查披露边界后选择性入库。源码基线与产物提交分开记录；
旧 run 的历史内容保持不变。scoped 必须从当前源码补充新单元，不能仅从 run-1 清单选行。
修复、复核、RC、稳定版与部署状态分别在 [remediation-status.md](remediation-status.md) 追踪。
