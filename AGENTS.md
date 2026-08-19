# KPanel Codex 适配入口

本文件只说明 Codex 如何进入 KPanel 的共享工程规范。产品、质量、任务状态和发布规则不得在这里
平行定义；权威层级如下：

1. `PROJECT_RULES.md`：产品、业务真源、安全边界、质量与发布硬规则；
2. `docs/project-management.md`：多 AI 角色、任务契约、Definition of Ready/Done、worktree、验证、集成和权限；
3. `docs/multi-agent-collaboration.md`：跨 Codex、Claude 和其他智能体的运行手册；
4. `.codex-workflows/`：Codex 可发现的参数化执行适配，不是第二套规范。

不机械地在每个小任务完整重读三份长文。先读本入口和
`docs/product-quality-review-current.md` 定位业务域，再按风险加载：L0 只读受影响文档与相关规则；
L1 读取 `PROJECT_RULES.md` 的工程契约、产品质量和核验分级及领域文档；L2 完整读取
`PROJECT_RULES.md` 与项目管理的任务/验证/集成章节；L3 或规范架构变更完整读取前三项和相关工作流。
已有精确证据足够时禁止重复全仓扫描。

## 启动检查

1. 明确目标、非目标、限制、交付物和验收方式；复杂任务使用简短计划。
2. 记录当前状态和回滚点：

```powershell
git remote get-url origin
git fetch origin --prune
git status --short --branch
git rev-parse --show-toplevel
git rev-parse --short HEAD
git worktree list
```

3. `origin` 必须为 `git@github.com:kejilion/KPanel.git`。GitHub App、Issue、PR、API、插件或 `gh`
   登录不是本地开发前置条件。
4. 只有任务涉及发布、整体质量审计、受控自我改进或专项真机验收时，才读取
   `.codex-workflows/README.md` 并列出相关工作流；普通 L0/L1 不为发现流程加载全部工作流。
5. 按 `docs/project-management.md` 填写标准任务契约。写任务达到 Definition of Ready 后，才能在
   最新批准基线创建专用 branch/worktree；管理工作树只用于同步、盘点和只读比较。

## Codex 专属协调

- `KPanel · 协调中心` 是用户沟通入口；Codex 子任务只用于并行分析、开发、验证或等待，不是全局真源。
- 复用任务前核对项目路径、标题、角色、scope、worktree、分支、基线和当前状态；领域或所有权不一致时
  不复用旧任务。
- 新任务标题使用 `KPanel · Codex · <角色> · <领域>`。任务契约必须包含绝对 worktree 路径，禁止两个
  写任务共享 worktree 或分支。
- 会话 ID、内部计划、模型偏好和临时上下文不得写入仓库。跨工具状态以 SSH 远端分支/提交、CI、
  Release 和验收记录为准。
- 使用等待/消息能力只是本地协调优化；协调中心必须从精确差异、提交和验证证据独立验收，不能只转发
  子任务自述。

## 执行与验证

- 使用 `rg` 定位受影响代码、测试和设计；先复用类型、限额、任务框架和 `kejilion.sh` 协议。
- 修改使用最小、可回滚补丁。新网络入口、宿主机写操作、终端、归档、身份或迁移先覆盖失败边界。
- 日常运行 `make verify-change`；L2 使用 `make verify-l2`；发布使用 `make verify-release`。定向测试用于
  快速反馈，不替代对应等级门禁；完整门禁也不替代实机、浏览器、性能或业务互通证据。
- 证据只对精确提交、环境、工具和参数有效。组合未变化时可复用已有 CI，不机械重复全量测试；变化时
  从受影响层重跑。
- 本地或远程长时间浏览器验收使用 `background-browser-validation` 工作流后台运行；先通过
  `environment-policy.json` 目标检查，再以持久化终态和证据交付，不占用前台会话等待。
- 工作树、分支、`HEAD` 或文件所有权出现非预期变化时立即停止，按项目管理规范保留现场并迁移，
  不执行 `reset --hard`、`clean`、强切分支或覆盖他人改动。

## 交付与权限

达到 Definition of Done 后回传 `docs/project-management.md` 的标准交付包，至少包含：精确 Git 状态、
修改文件、已验证事实、质量维度和证据层级、命令与结果、未验证风险、依赖/冲突、回滚和权限状态。

- 用户要求修改并不自动授权提交、推送、更新 `main`、tag、Release 或部署；按任务明确权限执行。
- 未获提交授权时保留专用 worktree 差异，状态为“待提交授权”，不得进入跨工具交接或集成。
- 获得提交/推送授权后形成聚焦提交并通过 SSH 推送任务分支；只有唯一集成/发布任务在明确授权后
  才能更新 `main`、打 tag、公开 Release、发布镜像或操作生产。
- 最终说明完成内容、修改、验证、风险、回滚点、提交/推送/发布状态，以及是否新增或复用了知识/工作流。
