# Docker 五分区管理与互通边界

> 历史版本说明：本文件记录 v0.18 初始设计。危险配置、外部目录、KPanel 自身或资源来源
> 导致的只读/保护策略已由 [`PROJECT_RULES.md`](../PROJECT_RULES.md) 废止。

KPanel 0.18 把原“Docker 工具箱”拆为环境、容器、镜像、网络和存储卷五个
常驻子标签。每次读取和写入均以 Docker Engine、`/home/docker` 和
`/etc/docker/daemon.json` 为事实来源，不把面板数据库作为 Docker 资源真相。

## 与 kejilion.sh 的业务映射

| 分区 | `kejilion.sh` 业务 | KPanel 0.18 |
| --- | --- | --- |
| 环境 | 安装/更新、状态、清理、换源、IPv6 | KPanel 已运行时 Docker 必然已安装；更新复用发行版原生后台维护，清理为固定 Docker API，镜像源使用脚本同一列表并保留其他 daemon 键 |
| 环境 | 备份/还原/迁移 | 后台归档 `/home/docker` 到 `/home/docker/.kpanel-backups`；还原拒绝同名项目覆盖，迁移只用既有 SSH 密钥与 known_hosts |
| 环境 | 卸载 | 显示明确入口和影响，但交回 `k docker`/SSH；网页不能在销毁自身运行环境后继续核验或回滚 |
| 容器 | 创建、启动、停止、重启、删除 | 结构化创建端口、卷、环境变量和启动参数；脚本和 Web 都从 Docker Engine 立即发现相同容器 |
| 容器 | 进入、日志、占用 | 归属明确且安全配置通过的容器开放有界日志和单次控制台；性能详情使用 Docker 双周期 CPU stats，批量历史采样使用 one-shot 累计计数并在相邻轮次间计算 |
| 容器 | 允许/阻止 IP+端口 | 使用脚本相同的 `DOCKER-USER` TCP/UDP ACCEPT/DROP 规则形态和 `/etc/iptables/rules.v4` |
| 镜像 | 获取、更新、删除 | 拉取同一 tag 即更新；单个删除和未使用镜像清理均为后台任务 |
| 网络 | 创建、加入、退出、删除 | 直接写 Docker Engine；系统网络和 KPanel 网络受保护 |
| 存储卷 | 创建、删除、清理 | 只创建 local 卷；使用中或 KPanel 卷不可删除 |

## 双向同步

- `k docker`、应用市场或人工 Compose 创建/修改资源后，KPanel 下一次请求重新读取
  Docker Engine，不依赖旧缓存。
- KPanel 创建的容器写入 `io.kejilion.panel.managed=true`，脚本的
  `docker ps`、`docker image ls`、网络和卷命令无需适配即可直接操作。
- 脚本 Compose 工作目录在 `/home/web` 或 `/home/docker` 且身份唯一时，KPanel
  可继续执行生命周期、日志、性能、控制台和访问控制；危险配置或外部目录保持只读。
- 脚本以 `docker run` 安装的单容器应用，通过同名 `<容器名>_port.conf` 业务标记
  建立归属，仍须通过 privileged、host namespace、设备、capability 和挂载边界复核。
- 备份还原后的项目文件、数值属主和 `appno.txt` 会回到脚本原布局；`appno.txt` 去重合并，
  KPanel 自身目录、备份目录和端口标记不会被覆盖。

## 写入与回滚

- 所有长任务持久化到 Agent 状态目录，页面刷新或离开不会中断。
- 当前全局变更记录直接聚合 Docker、应用和 Web 环境各自的任务记录，使用来源加任务 ID
  区分同名记录；HTTP 202 的审计只表示接受，并关联任务身份，不能作为执行成功。
  旧的异步 `success` 审计只保留在操作历史中，不据此生成成功任务。只有提交意图而没有结果时
  显示结果未确认。集合查询保留 `items`，并以 `sources` 和 `partial` 明确来源完整性：一个来源
  不可达或返回无效任务页时，保留其余来源本轮确认的记录；三个 owner 全部失败时返回 503，
  不沿用陈旧成功。这是原先任一来源失败即整页失败契约的兼容演进。
- Docker 的 queued 必须先保存才能接受提交；running 保存失败时不执行主机动作，记录“尚未执行”。
  执行结束但终态未保存时，内存显示 `failed/persistence_pending` 和结束时间，明确结果待保存，
  不显示虚假成功或继续运行。未保存的输入会清除，实际终态在同一有界 registry 中等待重试。
- 读取任务或提交新任务时，每秒最多触发一轮状态保存重试；不重跑 Docker、脚本或主机动作。
  存储待恢复的新提交返回 `503/docker_job_storage_unavailable`，真实任务仍执行时保持
  `409/docker_task_conflict`。保存成功后解除排他，新任务可正常提交。
  Agent 重启后只看到旧 queued/running 时，仍按现有机制保存 failed/interrupted，要求核对资源，
  不能猜测成功或重放动作；中断状态保存失败也走同一恢复路径。
- 全局详情通过 `GET /api/v1/jobs/{owner}/{id}` 从已有 owner 详情恢复；owner 只允许
  `docker/app/webenv`，ID 只允许 32 位小写十六进制，并绑定响应身份。选择保存在页面 `job`
  查询参数中，超过最近 50 条仍可刷新或重开。owner 404、不可达、无效响应时清除旧详情，明确
  结果未确认并提供返回业务页面；无可靠 owner 的旧审计仍只按有界查询窗口展示。
  单次详情最多 1 MiB/5 秒，集合每来源最多 8 MiB/5 秒、同时固定 3 个只读请求，各保留最新
  100 条后合并为最多 100 条；审计最多读取 200 条。页面等待上一轮结束后每 4 秒刷新，关闭
  详情/页面或桌面窗口失活即停止对应请求。查询只使用 GET，不重放宿主动作、不新增任务数据库；
  owner 自身保留期仍是恢复边界。自动回归覆盖该契约，隔离真机与生产行为本轮未验证。
- 镜像、容器、网络、卷写入在执行前重新核验 `resourceVersion`。
- `daemon.json` 只修改负责的键，Docker 重启失败恢复原文件并再次启动。
- 新建容器启动失败时删除本次刚创建的容器；不删除镜像或卷。
- 防火墙变更或规则文件持久化失败时，反向恢复本次插入/删除的规则。
- 还原先完整校验归档和目标冲突，再创建新顶层路径；失败只清理本次新建路径。

## 安全边界

- 不接受任意 `docker run`、Compose、daemon.json、iptables 或宿主机命令文本。
- 控制台命令只在安全识别的容器内执行，无 TTY，最长 20 秒，输出最多
  64 KiB/1000 行；命令本身不进入审计变化或后台任务文件。
- 还原拒绝符号链接、硬链接、设备文件、路径穿越、单文件超过 10 GiB、
  总解包超过 50 GiB 或超过 100000 个条目。
- SSH 迁移不接收密码，不使用 `StrictHostKeyChecking=no`，目标目录固定为 `/tmp`。
