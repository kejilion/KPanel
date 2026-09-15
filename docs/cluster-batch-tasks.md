# KPanel 集群批量任务

- 状态：开发完成，待发布
- 页面：`/cluster/tasks`
- 定位：在当前 KPanel 上创建持久化任务，对本机和已显式授权的远端 KPanel 执行固定系统维护动作
- 安全边界：不接受 Shell、脚本、路径或自定义参数；轻量节点不执行系统维护

## 1. 动作目录

页面和 API 都只接受服务端目录返回的动作 ID。维护动作复用目标 KPanel 已有的
`POST /v1/system/actions`，状态跟踪复用 `GET /v1/system/summary`，不会增加通用命令执行入口。

| 批量动作 | Agent 动作 | 固定策略 | 风险 |
| --- | --- | --- | --- |
| `refresh` | 不调用系统动作；重新采集集群摘要 | 无 | 只读 |
| `system-update` | `update` | `full` | 系统写入 |
| `cleanup-cache` | `cleanup` | `cache` | 系统写入 |
| `cleanup-standard` | `cleanup` | `standard` | 系统写入 |
| `logs-retain-7d` | `log-cleanup` | `retain-7d` | 系统写入 |
| `logs-retain-3d` | `log-cleanup` | `retain-3d` | 系统写入 |
| `logs-max-500m` | `log-cleanup` | `max-500m` | 系统写入 |
| `reboot` | `reboot` | 无；目标端使用既有延迟重启 | 中断服务 |

重启必须在创建请求中显式提交 `confirmDisruptive=true`。目标 Agent 接受延迟重启后，中心将该目标
标记为“已安排”，不会等待主机离线再上线。系统更新、清理与日志策略必须返回可跟踪的精确任务 ID；
状态中的任务 ID、动作和策略任一不匹配都进入“需要人工核对”。

## 2. 授权模型

批量系统维护使用独立 scope：

```text
cluster.system.maintenance
```

新版本目标 KPanel 只有在已装载维护适配器时，才会在新生成的 v2 授权码中声明完整 scope：

```text
cluster.summary.read cluster.terminal.open cluster.files.read cluster.system.maintenance
```

中心通过 Noise 认证的 `POST /api/v2/federation/pair-maintenance` 请求该权限。旧目标端明确返回
404 时，中心才回退到原 v2 配对路径，此时最多获得概要、终端和文件权限。既有 v1、旧 v2 连接
不会在升级后自动扩权；管理员必须在目标 KPanel 重新生成授权并重新配对。客户端自报 header、旧配对
路径或仅修改本地状态都不能获得维护权限。

为保持代码回滚兼容，既有 `cluster-state-v2.json` 中的主机和控制端记录仍保存旧版认识的
`cluster.summary.read cluster.terminal.open cluster.files.read`；维护授权只以主机/控制端 ID sidecar
写入新增的批量任务状态文件。运行时必须同时满足旧 scope、sidecar grant、活动连接状态和目标端适配器
四项条件才可执行。旧版会忽略 sidecar，而不会因遇到未知 scope 无法加载原联邦状态。

远端动作通过 `POST /api/v2/federation/batch-task` 传输。方法、路径、双方节点身份、时间戳和随机
request ID 都进入 Noise prologue，动作正文位于加密信封内。目标端再次检查控制端记录的精确 scope、
动作目录、operation ID 和本机维护适配器。每次提交及状态查询均使用独立、有界、可防重放的请求 ID。

本机不经过联邦授权，但只有 Panel 成功装载同一维护适配器时才显示可执行。轻量节点即使具备 root
terminal-broker 或 file-broker，也不具备此 scope，不作为任何批量任务目标；轻量节点仍可在集群页
使用原有的单机刷新能力。

## 3. 任务与目标状态

任务状态：

```text
queued -> running -> succeeded | partial | failed | needs_attention
               \-> cancelling -> cancelled | partial | needs_attention
```

逐台目标状态：

```text
queued -> submitting -> running -> succeeded | failed | needs_attention
   \-> cancelled                 \-> needs_attention
   \-> unsupported
```

- `submitting` 在网络请求前持久化，表示中心准备提交动作。
- `running` 只在目标返回合法 execution ID 后持久化。
- `needs_attention` 表示动作可能已被接受，但中心不能安全证明最终结果；它不是普通失败，也不自动重试。
- `unsupported` 在创建时按主机类型、协议版本和 scope 逐台计算；全部目标都不支持时拒绝创建。
- 一个任务可以包含不支持的目标，但必须至少有一台可执行目标，最终状态会如实反映部分结果。

同一主机在整条活动任务结束前保持占用，即使它在该任务中已经先完成，也不能加入另一条活动任务，
避免两个系统维护动作交叠。全局最多 4 条活动任务；每条最多 50 台目标；任务内并发和全局维护并发
均不超过 4。单目标跟踪时限为 60–3600 秒，默认 3600 秒。

## 4. 取消、重试与恢复

取消只保证停止尚未提交的目标：

- `queued` 目标直接变为 `cancelled`，不会进入目标 Agent；
- 已提交的维护动作没有远程撤销契约，中心取消跟踪后标记 `needs_attention`，并提示到目标主机核对；
- `refresh` 是只读采集，可在上下文取消后标记已取消；
- API 和页面不会把“停止中心跟踪”描述成“撤销目标动作”。

重试不会修改原记录，而是为 `failed`、`cancelled`、`unsupported`、`needs_attention` 目标创建一条
带 `parentTaskId` 的新任务。新任务生成全新的 operation ID。对 `needs_attention` 重试前，页面会要求
用户先核对目标主机，避免重复执行结果不明的动作。重试 `reboot` 也必须重新提交
`confirmDisruptive=true`，不能复用原任务的中断服务确认。

中心重启或一次落盘失败后的恢复规则：

- `queued` 目标可以继续调度；
- 带合法 execution ID 的 `running` 目标只恢复状态跟踪，不重新提交；
- 孤立的 `submitting` 目标一律变为 `needs_attention`，绝不猜测或重放；
- 已请求取消的排队目标收敛为 `cancelled`；已提交目标收敛为 `needs_attention`；
- 服务运行期间会定期协调没有活动 runner 的未完成任务，使暂时性落盘故障恢复后无需重启 Panel；
- 原子落盘失败会回滚内存修改，API 不会把未持久化的取消或创建报告为成功。

## 5. 持久化与保留

任务及维护授权 sidecar 保存在 Panel 数据目录的：

```text
cluster-batch-task-state.json
```

文件使用严格 JSON、`0600`、同步写入和同目录原子替换；写入中断时使用短期
`cluster-batch-task-state.json.previous` 恢复，并在成功校验后清理。读取拒绝符号链接、未知字段、
重复 ID、派生计数不一致、活动任务主机重叠及非法时间或状态。文件上限 4 MiB，最多保留 100 条任务；
达到上限时只裁剪最旧的已结束任务，绝不删除活动任务。

该文件是执行历史、恢复游标及新增维护授权 sidecar，刻意不进入 Panel 业务备份：恢复到另一时间点或
另一节点时，不应重新激活旧维护动作或复制控制关系。回滚旧版 Panel 会忽略它，并继续读取保持兼容的
v2 主记录；维护授权随之失效，概要、终端与文件能力不受影响。需要迁移审计记录时使用既有审计/任务
查询能力，不复制活动任务文件。

## 6. 浏览器 API

读取接口要求 Panel Session；创建、取消、重试和删除还要求同源 Origin、CSRF，并写入 intent 与
结果审计。请求使用严格 JSON，拒绝未知字段和多值 JSON。

```text
GET    /api/v1/cluster/batch-actions
GET    /api/v1/cluster/batch-tasks
POST   /api/v1/cluster/batch-tasks
GET    /api/v1/cluster/batch-tasks/{id}
DELETE /api/v1/cluster/batch-tasks/{id}
POST   /api/v1/cluster/batch-tasks/{id}/cancel
POST   /api/v1/cluster/batch-tasks/{id}/retry
```

列表只返回任务摘要；详情才返回逐台目标，避免任务历史页面放大响应。任务同时以
`cluster-batch:{taskId}` 投影到统一任务中心；从任务中心可回到 `/cluster/tasks?task={taskId}` 的
精确详情。批量任务来源独立于 Agent 的 Docker、应用和网站环境任务来源，一个来源不可用不会伪造
其他来源的成功结果。

## 7. 页面行为

`/cluster/tasks` 是集群下的独立工作区，而不是主机卡片上的一次性弹窗。页面包含：

- 服务端动作目录、风险等级和固定策略说明；
- 主机搜索、全选可用主机、逐台不可用原因和 50 台上限；
- 并发、超时、离线主机提示和重启二次确认；
- 持久化任务历史、总体进度、逐台状态、错误码、开始/结束时间；
- 取消、重试、删除和统一任务中心深链。

活动任务每 3 秒刷新；请求不重叠。页面隐藏或桌面窗口失焦时暂停，恢复时立即刷新；任务全部结束且
当前详情也已结束后停止轮询。请求失败保留最近成功列表并显示错误，不清空历史结果。

## 8. 审计与失败语义

中心记录 `cluster.batch.create|cancel|retry|delete` 的用户 intent 与结果。目标 KPanel 记录
`cluster.batch.target.{action}`，request ID 使用中心生成的 operation ID，actor 是已认证控制端节点。
审计 change 只含动作、目标数、并发、时限及合法 execution ID，不记录凭据、Noise 正文或任意命令。
创建与重试还会记录 `confirmDisruptive` 的布尔值，用来证明中断服务动作是否经过本次明确确认。

目标 Agent 明确拒绝且返回合法 Problem 时记为 `failed`。以下情形必须记为 `needs_attention`：

- 提交维护动作时发生传输错误；
- Agent 返回 2xx 但响应无效、动作不匹配或已接受却缺少任务 ID；
- 当前维护任务 ID、动作或策略与批量任务不一致；
- 跟踪超时、服务重启打断提交、目标状态未知；
- 用户取消时动作已经提交。

这一区分保证系统不会因为把“结果未知”误判为“可重试失败”而执行两次系统更新、清理或重启。

## 9. 回滚

回滚 Panel 代码只会停止新的调度与跟踪，不会撤销已经提交到目标 Agent 的维护动作。回滚前应停止创建
新任务，等待活动任务结束，并逐台核对所有 `needs_attention` 目标。旧版会忽略
`cluster-batch-task-state.json` 中的维护授权 sidecar；既有 v2 主记录不包含未知 scope，因此概要、终端、
文件授权仍按旧版本能力工作，批量维护权限则安全失效。

如需重新启用，恢复支持该协议的 Panel 版本即可读取原状态并按第 4 节规则安全收敛。不要手工把
`submitting` 改为 `queued`，也不要复制任务文件到另一中心端。
