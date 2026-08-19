# KPanel AI 工作台

## 范围与架构

AI 工作台位于 `/ai` 与 `/ai/s/{sessionId}`，由 `paneld` 内的原生 Go Runtime 驱动。它不安装 Hermes、Eino、Sidecar、本地模型或向量数据库，也不提供宿主机通用 Shell、通用 HTTP/Web、语音和多 Agent。

数据流为：

```text
Vue 三栏工作台 ── REST/SSE ── paneld AgentRuntime
                                  ├─ OpenAI-compatible / Anthropic / Gemini
                                  ├─ /var/lib/kejilion-panel/ai.db
                                  └─ 固定 KPanel Tool Registry ── Unix Socket ── kejilion-agent
```

身份、Origin、CSRF 与现有 KPanel API 完全一致。AI 数据独立保存到 `ai.db`；宿主机资源仍由 Agent 实时读取，不写入 AI 数据库。

## Provider 与网络

- 固定协议：`openai_compatible`、`anthropic`、`gemini`。
- `openai_compatible` 必须明确选择 `apiMode`：`responses` 使用 `POST /v1/responses` 语义事件，`chat_completions` 使用 `POST /v1/chat/completions` 数据分片。历史 Provider 自动迁移为 `chat_completions`，避免升级后改变行为；OpenAI 官方预设默认使用 `responses`，其他兼容预设默认保守使用 `chat_completions`。
- Responses 模式使用 KPanel 持久化的完整会话上下文，并设置 `store=false`。工具调用使用 `function_call` / `function_call_output`；推理模型返回的加密 reasoning item 只保存在内部工具上下文、仅向同一 Provider 回放，不通过 REST、SSE、日志或审计暴露。
- Responses 首次请求保持标准 OpenAI 载荷；若兼容 Provider 在尚未输出任何内容时明确返回 `messages[n]: missing field id`，Runtime 只重试一次带稳定消息 ID 的兼容载荷，并在当前进程内记住该端点能力。已产生文字或工具调用后禁止回放，其他 4xx 不触发方言猜测。
- Chat Completions 会捕获 Provider 实际返回的 `reasoning_content`、`reasoning_text` 或 `reasoning`，仅随对应工具调用保存在内部上下文；同一模型响应产生的多个工具调用共享这份上下文。标准后续请求不主动猜测方言；只有端点在未输出内容时明确要求补充某一 reasoning 字段，Runtime 才以已保存的原文和指定字段重试并记住当前端点/模型方言。隐藏推理不进入 REST、SSE、日志或审计。
- Responses 的 HTTP SSE 与 Realtime WebSocket 是不同接口；v1 不接入 `/v1/realtime`、音频或语音会话。
- 公网 Provider 必须使用 HTTPS；拒绝 URL userinfo、query、fragment，以及每次 DNS 解析得到的回环、私网、链路本地、组播、未指定和保留地址。
- 内网/本地 Provider 必须显式选择 `private`；只有此模式允许 HTTP。
- 重定向最多三次；跨源重定向移除 `Authorization`、`X-Api-Key` 与 `X-Goog-Api-Key`。
- Compose 同时连接固定 `panel-internal` 与普通 `panel-egress`。容器仍为 UID 65532、只读根文件系统、`cap_drop: ALL`、`no-new-privileges`、256 MiB。
- Provider 只能访问模型 API；模型自身只能调用固定 KPanel 工具，没有通用联网工具。
- Ollama/LM Studio 预设通过 `host.docker.internal` 访问宿主机；本地服务需监听 Docker 网关可达地址，且保存前仍需显式确认内网模式。

## 用户交互

- 首次进入先展示三步引导：选择 API、验证连接、启用模型。新增 API 默认执行“保存 → 测试 → 同步模型”；后两步失败时保留已加密保存的连接，允许修正后单独重试。
- 点击“新建会话”会直接使用管理员设置的默认模型；没有显式默认值时使用第一个可用模型。会话标题由第一条用户消息自动生成，仍可手工重命名。
- 模型选择器位于输入框右下角，实时包含所有已启用 Provider 的已启用模型。运行中允许切换，但当前 Run 使用启动时快照，新选择明确标记为“下一轮”并只影响后续 Run。
- 思考强度位于模型选择器旁，提供 `low`、`medium`、`high`。Run 固定启动时快照；模型声明原生 reasoning 能力时映射为 OpenAI `reasoning.effort`/`reasoning_effort`、Anthropic `output_config.effort` 或 Gemini `thinkingConfig.thinkingLevel`，否则只使用通用规划强度提示。界面不暴露隐藏思维链，只展示可审计工具过程。
- 会话列表提供搜索、置顶、重命名、归档、恢复和删除；当前与已归档会话有独立视图，移动端通过抽屉管理。
- 对话在首个模型分片到达前显示规划/重连状态；助手可见说明与紧凑工具行按真实发生时间穿插在同一个助手回合中，工具参数和结果按需展开。最终回答底部依次展示复制、模型和输出时间，代码块提供独立复制按钮。
- 流式文字使用浏览器帧内轻量缓冲，避免大分片突跳；只有用户停留在底部附近时自动跟随。用户上滚后停止抢夺滚动位置，并显示“回到最新”。
- 输入框随内容自动增高，运行中仍可追加要求。支持最多 4 个附件：PNG/JPEG/WebP/GIF 图片使用模型原生多模态输入，UTF-8 文本、日志、代码和配置文件按“不可信附件”进入上下文。单图 4 MiB、单文本 512 KiB、总计 8 MiB；拒绝 SVG、可执行文件和其他二进制格式。
- OpenAI-compatible Chat 端点若在尚未输出内容时明确拒绝 `image_url` 且只接受文本，Runtime 只把旧回合图片降为带文件名的不可用占位后重试并记住当前端点/模型能力；当前 Run 上传的图片绝不静默丢弃，而是明确要求切换支持图像输入的模型或兼容中转。接受多模态的中转和模型仍使用原生图片载荷。

## 密钥与数据

- SQLite 使用 WAL、外键、5 秒 `busy_timeout` 和事务迁移。
- API Key 由 XChaCha20-Poly1305 加密，主密钥为 `/var/lib/kejilion-panel/ai-secrets.key`，Linux 权限 `0600`。
- 数据库存在密文但主密钥丢失时，AI 模块失败关闭；KPanel 其他功能继续提供服务。不得自动生成新密钥覆盖。
- API 只返回 `apiKeySet` 和末四位；工具参数、结果、Provider 错误与审计进入统一限长和脱敏链路。
- 上下文估算超过模型窗口 70% 时，将旧消息压缩为最多 8 KiB 的持久摘要并保留最近消息。
- 附件正文随消息保存在权限为 `0600` 的 `ai.db`，REST/SSE 历史响应只返回名称、类型、分类和大小，不回传原始附件数据；删除会话会一并删除附件。图片与文本体积纳入上下文压缩估算。

## 运行与审批

- 单条消息 16 KiB、工具结果 64 KiB、12 个模型步骤、20 次工具调用。
- 单会话一个 Run，全局两个并行 Run；模型请求超时 180 秒，SSE 心跳 15 秒。
- 401 不重试；429/502/503/504 仅在尚未输出内容时重试两次。流式输出开始后不重放。
- 每个会话可选择 `manual`（手动审批）或 `auto`（安全自动审批），默认 `manual`。Run 启动时固定模式快照，执行中切换只影响下一轮。
- `manual` 下所有非只读工具逐次确认；`auto` 只放行固定 Schema 分类的常规应用、网站、容器、文件覆盖/回收站、Nginx 安全重载和缓存清理。
- 删除/卸载、标准级系统清理、系统核心设置、Docker 维护与备份迁移、容器 exec、交互式任务输入以及无法识别或无法解析的动作始终进入 `pending_approval`；分类失败时默认要求确认。
- 工具名与 Agent 路径是固定映射。Provider 侧使用不含组合关键字的扁平对象 Schema，让模型根据动作、真实状态和用户意图自主选择一般参数；Panel 只保留鉴权、审批、固定动作路由、严格结构化输入和审计边界，不重复限制每个动作的业务参数。Agent 根据 Docker 的真实状态校验底层技术前置条件、`resourceVersion` 和命令安全，并把失败作为可纠正结果返回模型。不存在任意路径或任意宿主机命令入口。
- 删除或修改现有容器、镜像、网络、卷前，必须先调用对应只读工具并使用同一资源最新的 id/name 与 `resourceVersion`。备份恢复/迁移先读 `host_docker_backups`，daemon 镜像源/IPv6 变更先读 `host_docker_environment`；`image_prune` 只处理悬空镜像，不能替代精确 `image_remove`。
- Agent 返回 `docker_task_invalid` 时按可纠正参数错误反馈给模型；资源不存在按真实状态冲突处理并要求重新读取。只有不属于参数或状态变化的 4xx 业务拒绝才作为操作边界处理。
- Agent 返回带安全问题码的可预期 4xx 业务拒绝时，Runtime 将“操作未完成”和问题码作为工具结果交给模型重新规划；401、请求超时、限流、无结构问题码的协议错误、5xx 与网络故障仍终止 Run。Agent `detail` 永不进入模型上下文。
- 相同工具和规范化参数在同一 Run 内失败两次后，第三次调用在到达 Agent 前熔断；单个 Run 最多容许 6 次可恢复工具失败，超过后以明确的重规划上限结束，避免消耗完通用步骤上限。
- 无业务参数的只读工具仍要求 `{}`；为兼容部分 OpenAI-compatible 模型生成的 `{"_":true/false}`，Panel 仅对这组只读工具丢弃布尔占位字段。写工具、有业务参数的读工具、非布尔 `_` 和其他未知字段继续严格拒绝。
- 工具结果以“不可信数据”标签返回模型，不能修改核心提示词、Schema、鉴权、审计或确认策略。
- Panel 重启会把模型请求中的 Run 标记为 `interrupted`；未执行审批保留，Agent 已提交的后台任务按原有任务恢复机制继续。

## 运维闭环与边界

AI 使用同一条轻量工作链处理常见实质任务，而不是按“修 Nginx”“清磁盘”等语句复制业务逻辑：

```text
读取真实状态 → 定位原因 → 选择最小且可回滚的注册操作 → 按会话策略审批 → 执行 → 再读真实状态验收
```

- Nginx：尾读 `/home/web/log/nginx` 日志，读取相关配置；已登记站点优先复用站点变更服务。任何直接配置修改后必须执行固定 `nginx -t`，只有通过才允许固定 `nginx -s reload`。
- CPU：结合系统概览、宿主机进程 CPU/内存双排行和 Docker 容器指标定位来源；不读取进程命令行，也不提供任意 PID kill。
- 磁盘：从系统概览逐级进入固定根目录热点分析，再选择缓存清理、需确认的标准清理/Docker prune，或带 `resourceVersion` 的可恢复回收站操作，最后复查空闲空间。
- 迁移：先盘点应用与容器并创建备份；只有用户提供明确目标且已有可信 SSH 配置时，才调用现有 `backup_migrate` 后台任务。迁移始终需要审批，不接收或推断目标凭据。

磁盘分析只允许 `/`、`/home`、`/home/docker`、`/home/web`、`/opt`、`/root`、`/srv`、`/tmp`、`/usr`、`/var`、`/var/lib/docker`、`/var/log`，不跨文件系统、不跟随符号链接，最多四个扫描线程、20 万条目录项、8 秒且同一时刻只运行一个。进程读取最多扫描 8192 个 PID，并仅返回 PID、PPID、名称、状态、UID、CPU、RSS 和启动标识。

AI 文件内容读取额外禁止 `.env`、SSH 凭据、私钥、证书私钥、Token、进程环境、Docker Swarm 与 KPanel 数据；文件改写/回收额外禁止 KPanel、Docker 内部目录、systemd、SSH、PAM、sudo、cron、系统账号和系统程序目录。Agent 仍会再次执行 File Manager 的路径规范化、符号链接和保护目录检查。

## 后台学习规则

用户明确表达稳定偏好、任务成功完成一个常规写操作，或完成两个及以上工具调用时，当前模型会在 Run 完成后异步判断是否值得跨会话复用；单次普通读取不会额外触发学习请求：

- `memory`：事实或偏好，不超过 500 字。
- `procedure`：适用条件和 1–10 个步骤，只能引用注册工具。

判断结果必须达到 0.82 置信度；瞬时指标、一次性状态、故障输出、密钥、令牌、IP、资源版本、任务 ID、不确定结论以及重复或冲突内容直接跳过。流程只能引用本次真正成功的工具，并再次通过 Tool Registry、确认策略和 dry-run；涉及删除、系统核心、Docker 维护、exec 或交互输入的流程不得自动沉淀。

通过校验的产物自动启用并在同一管理员的后续会话中生效，不在正常聊天中弹窗或要求点击。学习记录仍保存在本机，可在“AI 设置”的后台记忆/后台流程中停用、退休或回滚；系统提示词最多加载最近 12 条记忆和 8 条流程，避免自动学习导致上下文无限增长。任何学习产物都不能改变工具 Schema、鉴权、审计和受保护操作确认策略。

## 新增依赖与许可

| 依赖 | 用途 | 许可 |
| --- | --- | --- |
| `modernc.org/sqlite v1.55.0` | CGO-free SQLite | BSD-3-Clause |
| `markdown-it 15.0.0` | 禁用原始 HTML 的 Markdown 解析 | MIT |
| `DOMPurify 3.4.13` | 渲染后 DOM 二次净化 | MPL-2.0 OR Apache-2.0 |

构建仍使用 `CGO_ENABLED=0`。发布流水线生成的 SBOM 会从锁定后的 `go.mod/go.sum` 与 `package-lock.json` 收录这些依赖。

## 回滚

运行数据回滚前先停止 `paneld`，备份 `ai.db*` 与 `ai-secrets.key`；两者必须成对保留。应用市场更新会保留数据目录并自动恢复旧镜像、Agent、Compose 与 `.env`。移除 AI 功能不需要迁移或改写 `panel-state.json`，也不影响 Agent 宿主机状态。

## 轻量验收

0.42.0 在 154 Debian 13 真机完成发布级 L3、双架构 CGO-free 构建、公开镜像 E2E、隔离 AI 实聊和生产升级。相对发布前主线 0.40.3，同参数 stripped `paneld` 增加 4.06 MiB，两次空闲 RSS 增量为 3.53 MiB 和 3.59 MiB；均低于 30 MiB/25 MiB 目标。两个并行 Mock Run 使用 139.8 MiB/256 MiB，未发生 OOM。

Compose 的 `256M` 内存限制保持不变。完整数据、CI/Release 链接、镜像摘要和回滚证据见 [v0.42.0 发布验收](release-v0.42.0-acceptance.md)。
