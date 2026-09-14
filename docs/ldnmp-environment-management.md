# LDNMP 环境管理契约

## 范围与事实来源

KPanel `v0.22.0` 在“网站 → 环境管理”开放 `/home/web` 对应的 LDNMP 生命周期。
`kejilion.sh` 仍是安装、防护、优化、更新、备份、还原和卸载的唯一业务真源；
Agent 只负责固定参数校验、后台任务、安全边界、审计和真实产物复核。

KPanel 不维护第二份 Compose、Nginx、Fail2Ban、优化模板或版本目录。页面状态每次从
Docker、`/home/web`、Nginx、MySQL、Fail2Ban、iptables、cron 与脚本协议重新采集。

## 脚本协议

脚本新增独立命令空间，原 `k web` 菜单编号、文案和默认入口保持不变：

```text
k web env
k web env status
k web env catalog
k web env install full|nginx
k web env protect <fixed-action>
k web env optimize <fixed-action>
k web env update <component> <version> <backup-before>
k web env backup [create|delete <archive>]
k web env restore <archive>
k web env uninstall <backup-before>
```

普通终端运行 `k web env` 进入环境菜单；Agent 设置
`KJ_LDNMP_NONINTERACTIVE=1` 与 `KJ_LDNMP_PROTOCOL=1`。任务只接受固定枚举，
不接受自定义 Shell。状态输出是单一 JSON；任务输出使用：

```text
KPANEL_LDNMP_PROTOCOL 1
KPANEL_LDNMP_EVENT {...}
KPANEL_LDNMP_RESULT {...}
```

环境任务、KPanel 一键建站和关键网站写操作共同使用
`/run/lock/kejilion-web-environment.lock`。任务成功必须同时满足：

1. systemd/OpenRC worker 已退出；
2. 脚本以原子替换写入 root 专属完成凭据；
3. Agent 能重新读取最终环境产物。

缺少完成凭据时，即使 init backend 报告进程已成功退出，任务仍标记为“需要人工处理”。

## API 与任务

Agent 与 Panel 代理开放：

```text
GET  /v1/web-environment
GET  /v1/web-environment/catalog
GET  /v1/web-environment/backups
GET  /v1/web-environment/jobs
POST /v1/web-environment/jobs
GET  /v1/web-environment/jobs/{id}
GET  /v1/web-environment/jobs/{id}/terminal
POST /v1/web-environment/jobs/{id}/input
GET  /v1/web-environment/backups/{id}
```

任务动作固定为：

- `install`
- `protection.configure`
- `optimization.apply`
- `update.component`
- `update.all`
- `backup.create`
- `backup.delete`
- `restore`
- `uninstall`

每个写入请求必须携带当前 `expectedResourceVersion`。Agent 由独立 systemd transient unit
或 OpenRC `start-stop-daemon` worker
启动 PTY，终端按偏移量读取并保留 ANSI 颜色，单次输入限制 16 KiB 且拒绝 NUL。
浏览器关闭、刷新或 Agent 重启不改变 worker 的生命周期；页面可重新打开终端并全屏
显示。环境任务也会映射到统一“活动记录”任务列表。

当前不会向环境任务发送强制终止信号。关闭终端只是关闭显示；在脚本尚未提供可验证的
事务安全停止点前，KPanel 不暴露可能中断数据库、归档或目录切换的“停止任务”按钮。

## 敏感信息

Cloudflare 账号、API Key/Token 与 Zone ID：

- 不写入 Panel 数据库、任务 JSON、终端和审计 change；
- 仅写入任务专属 `0600` 文件；
- 后台执行器只接收文件路径，不接收凭据值；
- 脚本读取后立即删除输入文件；
- 任务完成、启动失败或无凭据退出时 Agent 再次清理。

脚本自身需要长期使用的 Cloudflare action/自动五秒盾文件使用 `0600`/`0700`。

## 备份格式

KPanel 冷备仍生成原脚本可发现的：

```text
/home/web_YYYYMMDDHHMMSS.tar.gz
/home/web_YYYYMMDDHHMMSS.tar.gz.kpanel.json
```

冷备记录原本运行的 Compose 服务，只短暂停止并恢复这些服务。归档和 sidecar 使用
`0600`；sidecar 保存格式版本、文件名、SHA-256、脚本版本和创建时间。列表中的
“已校验”表示 Agent 已重新计算完整归档 SHA-256，而不是只发现 sidecar 文件。

传统 `/home/web_*.tar.gz` 仍可发现，但显示“传统备份”。还原前执行 gzip、条目数量、
展开大小、剩余磁盘、顶层 `web/`、路径穿越、链接和设备文件检查。

## 还原与回滚

还原使用 `/home` 同文件系统暂存目录：

1. 校验并解压到暂存目录；
2. 验证 Compose；
3. 记录旧环境运行服务；
4. 原子移动旧 `/home/web` 为回滚副本；
5. 切换新环境并验证 Compose、容器和 `nginx -t`；
6. 失败时恢复旧目录及旧运行服务。

当前实现拒绝所有归档链接（比“仅允许目录内相对符号链接”更严格），避免链接逃逸。

## 发布与回滚

- KPanel 回滚点：`v0.21.0` / `4b41740`。
- 计划发布：`v0.22.0`。
- 固定脚本：`f031d1206224de3743845d2fc81c4801ecda32f4`；
  SHA-256 `278526cee183cdc826c25e113a399fcac72484f8f2af2fd17a8f75a1cd6a40c1`。
- 发布顺序：先推送脚本协议，再固定脚本提交与 SHA-256，最后构建 KPanel 镜像。
- 回滚 KPanel/Agent 不删除或覆盖 `/home/web` 与 `/home/web_*.tar.gz`。
- 正式发布前执行 `verify-l2`、完整 `verify-release`、镜像内脚本摘要核验和独立主机实测。
