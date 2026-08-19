# KPanel Claude 适配入口

本文件只说明 Claude/Claude Code 如何进入共享规范，不维护 Claude 专属的产品规则、质量标准或门禁。
先阅读本入口和 `docs/product-quality-review-current.md` 定位业务域，再按风险加载共享规范：

1. L0：受影响文档与相关规则；
2. L1：`PROJECT_RULES.md` 的工程契约、产品质量和核验分级，以及领域文档、实现和测试；
3. L2：完整 `PROJECT_RULES.md` 与项目管理的任务、验证、集成章节；
4. L3 或规范架构变更：完整读取 `PROJECT_RULES.md`、`docs/project-management.md`、
   `docs/multi-agent-collaboration.md` 和相关工作流。

## 启动与所有权

- 先核对 `origin/main`、目标远端分支、`git status --short --branch`、`git worktree list` 和精确基线；
  不根据旧会话记忆继续写入。
- 按 `docs/project-management.md` 使用统一任务契约和 Definition of Ready/Done。任务必须明确角色、
  scope、允许/禁止路径、共享契约、业务真源、受影响用户旅程、worktree/branch/base、L0-L3、证据和权限。
- 一个写任务独占一个专用 worktree 和短期分支；管理工作树只用于同步、盘点和只读比较。
- 不修改其他智能体拥有的路径，不切换、重置、清理、删除或覆盖其他任务的工作树、分支和未提交内容。
- SSH 远端 `git@github.com:kejilion/KPanel.git`、聚焦提交、CI、Release 和验收记录是跨工具真源；
  会话历史、Todo、模型记忆、GitHub Issue/PR/API 和 `gh` 不是写任务前置条件或长期状态。

## 实施与验证

- 先使用 `rg` 定位直接相关代码、测试和文档，已有精确证据足够时不重复全仓扫描。
- 使用 `PROJECT_RULES.md` 的 L0-L3 和 `make verify-change`、`make verify-l2`、`make verify-release`
  权威入口，不创建 Claude 专属平行命令。
- 定向测试用于反馈，不能替代对应等级门禁；门禁不能替代受影响业务互通、实机、浏览器、性能或回滚证据。
- 本地或远程长时间浏览器验收复用仓库的后台浏览器入口和环境策略；不得把 `prod-108` 用作测试目标，
  也不得依赖前台会话持续打开来维持作业。
- 证据只对精确提交、环境、工具和参数有效；未变化时复用，变化时从受影响层重跑。
- `.codex-workflows/` 是 Codex 执行适配层。Claude 可参考步骤，但共享规则仍以根规范和项目管理文档为准。
- 发现 `HEAD`、分支、文件或所有权非预期变化时立即停止并按冲突恢复流程保留现场，不执行破坏性清理。

## 交付与权限

达到 Definition of Done 后使用项目管理规范的标准交付包，区分已验证事实、分析结论、建议和未验证
风险，并包含精确 Git 状态、质量维度、证据层级、测试、回滚和权限状态。

- 用户要求修改不自动授权提交、推送、更新 `main`、tag、Release 或部署。
- 未获提交授权时保留专用 worktree 差异并标记“待提交授权”，不得交给下一智能体集成。
- 获得提交/推送授权后形成聚焦提交并通过 SSH 推送任务分支；只有唯一集成/发布任务在明确授权后
  才能更新 `main` 或发布。
- 跨智能体交接前原负责人停止写入并释放所有权；接手者重新 fetch、核对提交和差异后再接管。
