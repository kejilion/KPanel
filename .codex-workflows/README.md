# Project Workflows

本目录保存 KPanel 可跨 Codex 会话复用的执行适配工作流。跨 Codex、Claude 和其他智能体的共享
规则与状态真源位于 `PROJECT_RULES.md`、`docs/project-management.md`、
`docs/multi-agent-collaboration.md`、SSH 远端分支/提交和 CI；本目录不得形成第二套规范。

- `session-collaboration.workflow.yaml`：复用或创建任务、等待、复核并统一交付。
- `background-browser-validation.workflow.yaml`：在登记的隔离环境后台执行真实浏览器 E2E/稳定性验收，
  以持久化 job 状态和证据跨会话恢复；测试强度由风险画像决定，不机械套固定时长。
- `local-feature-preview.workflow.yaml`：为可见功能交付统一的 mock/本地集成预览，自动分配回环端口，
  固定候选身份、体验步骤、证据目录和停止责任；不替代真机、L0-L3 或发布验收。
- `release-kpanel.workflow.yaml`：版本准备、标准 Linux Runner、CI、Release、Docker Hub、应用市场、
  隔离验收和生产部署安全核对。
- `quality-audit-kpanel.workflow.yaml`：快速迭代后的业务正确性、体验、性能、稳定、安全、交付节奏和
  发布门禁健康审计。
- `evolve-kpanel.workflow.yaml`：从可复核证据形成改进假设，经独立复核、最小试行、指标对比和观察窗口
  决定采纳、拒绝或回滚；规范复核执行 `PROJECT_RULES.md` 5.3 的固定矩阵、严重度与停止条件，不自动
  放宽门禁、无限扩展反例或扩大提交、发布权限。
- `maintain-kpanel-dependencies.workflow.yaml`：读取全技术栈新鲜度报告，按安全、兼容、维护和资源风险
  决定采用、暂缓或拒绝，在独立 worktree 完成升级与分级验收；检测不自动等于合入或发布。
- 当前产品性质、业务规模和流程效率入口见
  [`docs/product-quality-review-current.md`](../docs/product-quality-review-current.md)；历史性能与安全原始
  基线继续见 `docs/quality-audit-2026-08-02.md`，后续审计按用途引用，不把旧产品形态当作当前事实。
- `kpanel-real-machine-app-lifecycle.workflow.yaml`：用隔离候选实例和真实 Chrome 验证应用
  在运行、停止、重启、暂停状态下均可打开详情，并恢复真机现场。
- `kpanel-site-icon-cache-validation.workflow.yaml`：用隔离 Agent/Panel、真实回环入口和真实 Chrome
  验证网站图标缓存、安全边界、性能预算及系统发行版图标映射。
- `normalize-kpanel-app-icons.workflow.yaml`：以现有应用图标为身份参考，生成统一风格候选，完成透明背景、
  视觉居中、真实桌面尺寸复核和单问题定向返工；默认只输出预览，上游采用与 KPanel 同步分开授权。
- 新版本发布后使用 `docs/release-acceptance-template.md` 记录多维质量状态、证据层级、生产部署安全核对、
  交付节奏和回滚点；产品变更失败与发布流程异常/无效证据独立统计，缺失数据保持“未报告”。除有明确
  追溯依据的单版本补录外，不批量改写历史验收记录。
- 重复缺陷、指标恶化或门禁缺口使用 `docs/quality-improvement-proposal-template.md` 建立提案，并通过
  `make release-metrics` 和受控自我改进工作流验证；数据不足时保持“未报告”。
- 依赖、工具链和底层组件使用 `make dependency-policy-check` 离线验证清单，使用
  `make dependency-report` 联网生成版本通道稳定候选；检测源失败保持“未报告”，不得推断为全部最新。
  GitHub Actions 每周生成候选、每日复核当前依赖图安全通告，并在例外或 EOL 复核到期时失败告警；采用
  资格仍由风险分级任务补齐。
- 主机必须登记在 `environment-policy.json`。`prod-108`/`108` 已禁用全部 KPanel 操作，测试、只读检查、
  备份、发布、部署和安全核对均由机器门禁拒绝；默认隔离验收与唯一正式部署环境均为 `arena-154`。

工作流由 `codex-workflows` 技能管理。使用前运行 `workflow.py list`，执行前使用
`workflow.py run <name> --param key=value` 渲染参数，修改后必须运行
`workflow.py validate <name>`。

## Portability and repository hygiene

- Resolve the workflow CLI from `CODEX_WORKFLOWS_CLI` when set; otherwise use the current user's `$HOME` skill directory. Never commit a workstation username or temporary attachment path.
- Keep session delegation envelopes, clipboard attachments, generated output, and local evidence outside tracked source files.
- `node scripts/check-governance-consistency.mjs` enforces these rules on every tracked file.
