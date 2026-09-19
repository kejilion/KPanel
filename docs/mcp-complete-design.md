# MCP 完整版本设计与验收范围

状态：候选开发收尾。实现范围和证据层级见文末；尚未通过的验收不得视为完成。
原 `291a6463` 候选只有 6 个巡检工具，不能作为本范围的完整交付。
产品基于 v1.20.0 `c98727c8`；一次发布覆盖完整目标，允许工程内部聚焦提交，不将主体功能排到后续版本。

## 竞品核对

复核日期 2026-09-19。证据是公开源码/官方文档，不等于部署压测或所有客户端实际互通。

| 对象 | 已核对范围 | 采用的产品经验 |
| --- | --- | --- |
| [1Panel dev-v2](https://github.com/1Panel-dev/mcp-1panel) | 文档列出 11 工具，系统、网站、证书、应用、数据库；stdio / Streamable HTTP；readonly/readwrite/full。开发分支不等同已发布版 | 明确访问等级、工具发现与执行同时过滤 |
| [宝塔](https://docs.bt.cn/ai-ops/mcp/tools-reference) | 文档列出 98 工具，文件、主机、网站、数据库、Docker、后台任务等；包含需单独授权的 Shell | 学习业务闭环、任务结果、接入教程；不引入任意宿主机 Shell |
| [宝塔客户端接入](https://docs.bt.cn/ai-ops/mcp/installation) | HTTP/Bearer；Codex、Claude Code、WorkBuddy、TRAE、DeepSeek Harness、Hermes/OpenClaw 教程；独立 Python 服务/8765 端口 | 客户端专用配置、连接诊断；KPanel 保持同端口内置 |
| [Portainer 2.45.1](https://github.com/portainer/portainer-mcp/tree/2.45.1) | OpenAPI 转换；Docker、Kubernetes、GitOps 与可选 Edge/Admin 分组；HTTP/stdio/mcpb；每用户实际权限；OAuth 可经外部代理 | 分组降低工具噪声、客户端身份隔离、桌面接入；不照搬通用 API 代理或引入 KPanel 没有的 Kubernetes 业务 |

## 客户端与传输

| 接入类型 | 本次目标 | 完成证据 |
| --- | --- | --- |
| HTTP + Bearer | Codex、Claude Code、Cursor、VS Code/Copilot、Windsurf/Cascade 的专用配置；其他客户端提供匹配其文档的通用配置/教程 | 配置结构测试，代表客户端实际 tools/list / tools/call；没有安装的产品不能标记“实测通过” |
| stdio | 自带轻量桥接子命令，复用同一 HTTP 授权与工具；适配本地桌面/stdio 客户端 | 真实子进程与 SDK 客户端互通，退出/超时/撤销/标准输出无日志污染 |
| OAuth | 同源发现、授权码 + PKCE S256、受众绑定、单次 code、可撤销授权；供 Claude 远程连接器及需要 OAuth 的客户端接入 | 独立协议客户端完整授权、拒绝、到期、撤销与重放测试；具体产品账号条件单列 |
| 接入体验 | 选择客户端 → 指定主机/权限 → 复制正确格式或浏览器授权 → 测试 → 查看最近失败原因/撤销 | 浏览器完整旅程，凭据仅显示一次；不把 token 写进一键安装 URL 或教程提示词 |

采用 Streamable HTTP 与必要的本地 stdio 桥接。旧 SSE-only 客户端不列为主流兼容承诺；工具不能据连接器品牌绕过授权。
官方文档锚点：[Codex](https://learn.chatgpt.com/docs/extend/mcp)、[Claude Code](https://code.claude.com/docs/en/mcp)、
[Cursor](https://prod.cursor.com/help/customization/mcp)、[VS Code](https://code.visualstudio.com/docs/agents/reference/mcp-configuration)、
[Windsurf/Cascade](https://docs.devin.ai/desktop/cascade/mcp)、[Claude 远程连接器](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp)。

## 业务完整性

范围是 KPanel 已有真实能力的 MCP 闭环；不为工具数量另造数据库、Kubernetes、Shell 或业务真源。

| 领域 | 本次必须覆盖的闭环 |
| --- | --- |
| 主机与集群 | 主机发现/身份/在线状态/指标/能力，逐主机查询与固定管理动作，部分成功和过期数据明确呈现 |
| Docker / Compose | 容器/镜像/网络/卷/项目查询，已有启停/创建/更新/删除/备份维护，任务进度与最终结果 |
| 网站 / Web 环境 | 网站/配置/证书状态、既有创建更新删除、nginx 校验重载、环境/安装任务与失败结果 |
| 应用 | 目录/安装状态、安装启停更新卸载、已存在交互任务、结果与资源版本 |
| 系统与诊断 | 已有系统配置、进程/存储/网络/防火墙/日志等查询，已注册结构化动作，诊断任务和输出 |
| 文件 | 独立选择允许目录；有界列举/文本/日志读取，版本约束的写入与回收站等现有动作；敏感系统文件仍受原保护 |
| 备份与恢复 | 现有备份清单/发起/进度/恢复前检查/恢复结果；恢复和破坏性操作须消费真实审批 |
| 操作任务 | 请求标识、审批状态、执行状态、成功/失败/中断/结果未知、适用的取消/输入；断线重试不重复写入 |

## 权限与执行

- 客户端按主机与业务域授权；预设巡检、日常运维和完整管理，预设只简化选择，不取代执行时的授权判断。
- 旧凭据升级后仍为原巡检权限；新增主机/新能力不自动加入旧授权。
- 写操作经过服务端保存的操作记录：固定工具、规范化参数摘要、客户端、主机身份、资源版本、时限和唯一请求标识。
- 默认人工批准写操作；可显式授权日常非破坏动作自动执行，删除/恢复/敏感系统变更仍单独确认。
- 审批只从已登录面板、Origin/CSRF 保护的 UI 提交；模型传入 confirmed/approved 不能替代批准。
- 审批/执行分开；凭据撤销、主机身份变化、到期、参数变化或资源版本冲突均使旧批准失效。
- 使用原 Agent 契约与审计；输出经过裁剪、分页、脱敏、大小/时间预算。写入结果超时应标记待核对，不自动重发。
- MCP 不拥有独立的 Docker、站点、文件或系统状态；新增持久化仅保存必要授权和有界操作记录。

## 集群边界

原 HostOperationService 只连接本机 Agent；原有集群配对仅覆盖摘要、受限终端与文件，不能直接授权新的远程业务执行。
新增结构化操作需要显式的版本化能力和目标侧授权。复用现有加密身份/防重放传输，不接受调用者提供原始 URL 或 Agent 路径。
完整 Panel 节点可在升级和授权后提供相应管理能力；轻节点按其实际安装的能力返回工具范围，缺少 Agent 的节点不能伪装完整面板。
旧节点保持兼容并明确提示需要升级/授权。远端拒绝、撤销、断连、响应丢失和任务结果必须有两端集成测试。

## 验收与发布定义

每个上述领域都必须记录：实现入口、schema/权限、成功、失败恢复、任务最终状态与真实测试证据。
协议互通、配置生成、某客户端实测三者分别记录。完整门禁不得仅用工具数量或“SDK 通过”代替。
保留轻量目标：服务端不新增运行时/监听端口/轮询服务；桥接仅在客户端需要时运行；操作记录、队列与 OAuth 状态均有容量和清理上限。
验收包含对应 L2、独立正确性复核、OCR、两端集成、浏览器、构建和资源样本；发布另需 L3。
此前标准预览与安全专项审计的平台阻断仍如实保留，不重试规避，不据普通测试宣称专项审计通过。
完整矩阵未关闭前不宣称“完整版本已验收”。

## 当前实现与证据层级

MCP 定位为可选 AI 接口，既有 Panel/Agent 继续负责管理；不宣称替代 SSH 或天然更安全。
完整授权发现 81 个工具，实际发现结果仍按域、主机和目标侧授权缩减。工具数量不作为成熟度结论。

| 范围 | 实现入口 | 当前证据 / 限制 |
| --- | --- | --- |
| 客户端 | `web/src/lib/mcp-clients.ts`、`internal/mcpbridge`、`cmd/kpanel-mcp` | 专用配置断言、真实 stdio 子进程发现/调用/撤权/退出；第三方产品账号未逐一实测 |
| OAuth | `internal/mcpaccess/oauth.go`、`internal/panel/mcp_oauth.go` | HTTP 发现/注册/登录态审批/换令牌/撤销，PKCE、受众、过期、重放和刷新复用测试；授权 UI 确认及失败恢复测试 |
| 本机业务 | `mcp_catalog*.go`、`mcp_backups.go`、`mcp_files.go` | 固定类型/路由复用现有业务，备份实际 owner 测试；目录跨页、回收站权限、部分失败回归；真实 Linux 发行版场景另列 |
| 集群管理 | `cluster/managed_operations.go`、`panel/mcp_cluster.go` | 本地双 Panel HTTPS + Noise；未授权拒绝、授权、审批、容量繁忙重试、回执丢失恢复、结果查询、撤销。Agent 使用模拟数据，非生产真机 |
| 操作闭环 | `mcpaccess/operations.go`、`panel/mcp_management.go`、`mcp_jobs.go` | 加密存储、重复请求、原子 Claim、崩溃 unknown、容量预留、预检期间撤权、真实备份终态与维护 ID |
| 界面与文案 | 设置下 MCP、操作审批、OAuth、集群授权组件 | 组件测试、类型检查、英文/繁体完整词条与构建；标准浏览器 acceptance 仍受既有平台阻断 |
| 总体门禁 | 仓库 L2、独立审查、OCR、构建与资源测量 | 只对最终精确提交计入，结果由候选交付记录报告。Cloudflare 专项审计未完成 |

文件文本修改有界；二进制备份包和文件上传下载、完整编辑器及任意宿主机交互 Shell 留在已有 UI/SSH，
不通过 MCP 模型上下文复刻。这是接口边界，不声称 KPanel 所有按钮都已转换为工具。
