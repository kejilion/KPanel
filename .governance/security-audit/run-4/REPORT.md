# v1.21.0 候选 full 审计入库记录（run-4）

状态：**complete（full）**。本文件只登记运行身份、覆盖计数与证据位置，不复述任何线索内容。

- 审计源码：`4c0694aa8e02e46145a775707b8d5a0355f7ce10`（tree `de988b2c9939aa9f8d2ae123c4170b481e04e87d`，clean），
  已是 `main` 祖先；完成时间 2026-09-21T16:05:43Z。
- 上游 skill：`cloudflare/security-audit-skill` pinned `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`，profile `standard`，
  模式 full（元数据沿用 skill 原生字段 `project_mode`，按 5.4 视为 `scope_mode` 同义）。
- 覆盖：127 个单元（95 covered，32 candidate）；findings 0 confirmed、34 needs_validation、1 rejected。
  Phase 3 全部候选经独立验证，保留记录均经 Phase 5 独立记录核验。
- 结构验证：2026-09-22 在 WSL 下以 pinned skill 的 `validate-findings.cjs`（35 findings）与
  `validate-coverage-ledger.cjs`（127 units）重跑，均 PASS。
- 执行边界：source-only，未执行目标代码、测试、构建或接触任何部署；needs_validation 均为带精确阻塞项的假设，
  没有 severity，不等同于已确认漏洞，也不是“无漏洞”结论。

## 为什么只入库元数据与本记录

34 条 needs_validation 描述的是尚未修复或尚未动态确认的边界假设。按 `PROJECT_RULES.md` 5.4“审计产物披露边界”，
在对应修复进入公开主线前不推送其细节；与 run-2 的做法一致，完整 `findings.json`、`coverage-ledger.json`、
`FINDINGS-DETAIL.md`、`NEEDS-VALIDATION.md` 与代理产物保留在仓库外证据目录
`C:/GitHub/_codex-evidence/kpanel-security-audit-run4-v1.21.0`。后续修复按原 fingerprint 在
[remediation-status.md](../remediation-status.md) 追踪，修复公开后再按需补入细节。

本次入库补齐 2026-09-21 完成后遗漏的登记；run 编号沿用原运行时的 run-4，不改写 run-5/run-6。
