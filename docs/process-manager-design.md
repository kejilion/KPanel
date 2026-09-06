# KPanel 轻量进程管理器设计

- 状态：实现完成并进入 `v0.52.0` 发布候选；正式状态以候选 L3、CI 和真机验收为准
- 路由：`/processes`
- 入口：概览实时监控标题栏，与“查看历史”并列；桌面模式可从任务栏右键菜单直接打开；不增加左侧主导航
- 真源：Linux `/proc`；不保存进程快照，不建立缓存或数据库影子事实
- 发布集成基线/回滚点：`9494f09` / `v0.51.0`

## 1. 产品目标

提供接近 `top`、`htop`、`btop` 和 Windows 任务管理器的核心排障闭环：实时查看宿主机进程，
按名称、PID 或用户搜索，按 CPU、内存、PID、名称、用户、状态和线程数排序，查看选中进程详情，
并向确认过身份的进程发送 `SIGTERM` 或 `SIGKILL`。

页面保持 KPanel 原生风格，不嵌入终端、不启动常驻 `top/htop/btop`，也不新增前端依赖。默认只在
页面可见且窗口激活时约每 2 秒读取一次；暂停、失焦、切换页面或卸载都会停止计时器并取消请求。

## 2. 现有能力与复用

现有 `internal/systeminfo` 已为 AI 诊断提供 `/v1/system/processes`，最多扫描 8192 个 PID，进行一次
300 ms CPU 差分采样，只返回 CPU 与内存各 32 条排行。新页面复用同一采样器并保持无参数响应兼容，
增加有界查询结果，不读取第二套进程来源。

进程结束复用既有 `/api/v1/system/actions` → Agent `/v1/system/actions` 链路，因此继续具备 Session、
Origin、CSRF、Agent Unix Socket、Bearer Token、严格 JSON、固定动作枚举和 intent/outcome 审计。

## 3. 当前产品参考复核

复核日期：2026-08-08。

- Windows Task Manager 以列表列展示 CPU、内存等资源，支持点击列头排序、查看 PID 和结束任务；
  KPanel 采用相同的低学习成本表格与选中进程操作区。
- `htop` 支持搜索、过滤、交互排序和不手输 PID 的进程动作；KPanel 保留搜索、列排序和行选择，
  不复制终端快捷键和命令行显示。
- `btop` 支持快速过滤、排序、进程详情、暂停刷新和发送信号；KPanel 采用暂停、详情及固定
  `TERM/KILL`，不开放任意信号。

参考：

- <https://learn.microsoft.com/en-us/troubleshoot/windows-server/support-tools/support-tools-task-manager>
- <https://github.com/htop-dev/htop>
- <https://github.com/aristocratos/btop>

## 4. 数据与 API

### 4.1 只读查询

```text
GET /api/v1/system/processes?q=<text>&sort=cpu|memory|pid|name|user|state|threads&order=asc|desc&limit=1..256
```

Panel 只做已认证 GET 转发；Agent 严格拒绝未知、重复、过长或越界参数。无参数调用继续返回既有
`topCpu/topMemory`，保证 AI 工具兼容。页面查询额外返回：

- 有界 `items` 和过滤前/后数量；
- PID、PPID、进程名、状态、UID、用户名、CPU、RSS、线程数、nice 值和启动标识；
- 当前 CPU、内存、进程总数及运行/睡眠/停止/僵尸数量摘要；
- 扫描量、截断状态、采样耗时和采集时间。

搜索只匹配受限进程名、用户名和十进制 PID，不读取 `cmdline`、`environ`、工作目录、文件描述符、
端口、日志或 cgroup 内容。用户名只从宿主机 `/etc/passwd` 的首字段与 UID 映射读取。

### 4.2 结束动作

```json
{
  "action": "process-signal",
  "pid": 1234,
  "startTimeTicks": 987654,
  "signal": "term"
}
```

`pid` 与 `/proc/<pid>/stat` 的 `startTimeTicks` 必须同时匹配，以阻止页面旧数据在 PID 复用后误伤
新进程。`signal` 只允许 `term` 或 `kill`，由 Agent 直接执行固定系统调用，不调用 Shell。返回成功
仅表示信号已送达，不宣称进程已经退出；页面随后立即重新采样真源。

## 5. 固定资源边界

| 项目 | 边界 |
| --- | ---: |
| 单次扫描 PID | 最多 8192 |
| 单次返回进程 | 最多 256，页面默认 200 |
| 搜索词 | UTF-8 文本，最多 128 字节 |
| CPU 差分窗口 | 默认 300 ms |
| Agent 查询超时 | 3 秒 |
| Agent 并发采样 | 1；相同规范化查询最多 8 个请求共享进行中的采样，其余返回 `429 process_metrics_busy` |
| 页面刷新间隔 | 2 秒；请求完成后再计时，不重叠 |
| 页面搜索防抖 | 250 ms |
| 响应敏感字段 | 0 个命令行/环境/凭据字段 |
| 写动作 | 单 PID、单固定信号、无重试 |

最多两轮读取每个 PID 的小型 `stat` 文件；第二轮只读取一次 `status`，不读取昂贵的 `smaps`，
不逐进程调用外部命令。排序和过滤在同一采样内完成，返回后前端不复制完整列表。

共享仅限尚未完成的同一次观测，不缓存已完成结果；无参数排行与带参数列表不混用。单个请求取消
不影响其他等待者，最后一个等待者离开时取消采样，采样真正退出后才释放名额。进程信号操作前后
关闭当前观测的加入入口，避免操作完成后的刷新加入操作前的采样；PID 与启动时钟复核保持实时执行。

## 6. 安全与失败边界

流量路径：浏览器 → Panel Session/Origin/CSRF → Agent Unix Socket/Token → `/proc` 与 Linux signal。

- 不可信输入：查询参数、PID、启动标识、信号枚举；全部有类型、长度和枚举校验。
- 权限：沿用 root Agent 和现有 system write 开关，不新增 TCP、capability、挂载或可写目录。
- 竞态：信号发送前立即回读启动标识；不存在或身份变化返回 `409`，不发送信号。
- 失败：权限不足或系统调用失败返回明确失败，不自动升级 `TERM` 为 `KILL`，不自动重试写动作。
- 生命周期：页面取消只读请求；信号一旦送达不能回滚，审计记录请求信号与目标身份。
- 合法管理员：不按进程名称、UID、服务归属或是否属于 KPanel 隐藏动作；底层内核拒绝才失败。

## 7. 界面与无障碍

- 顶部四张紧凑摘要卡展示 CPU、内存、进程数与运行状态；
- 工具栏提供搜索、暂停/继续、刷新状态和结果数量；
- 桌面为高密度可横向滚动表格，列头按钮支持升降序并展示当前方向；
- 选中行展示进程详情和结束操作，危险操作使用普通确认对话框；
- 移动端隐藏次要列并保留 PID、CPU、内存和操作入口；
- 数值使用等宽数字、状态同时使用文本与颜色，键盘可聚焦行和列头，遵守减少动画偏好。

## 8. 验收

- Go：查询参数、扫描/输出上限、搜索排序、取消、PID 复用、信号枚举、错误映射和审计；
- Web：懒加载路由、API 类型、搜索防抖、非重叠轮询、失焦停止、排序、确认动作和错误保留；
- 性能：采样基准、最大 256 条响应体、独立路由 gzip `< 120 KiB`，不影响主入口预算；
- L2：`make verify-l2`，包含全量 Go/Web、Linux 构建和部署契约验证；
- 真机待发布验收：低配 Linux 上持续打开 30 分钟，确认 Agent/Panel RSS、CPU、FD 和请求 P95
  不持续增长，并验证普通进程、僵尸消失、PID 复用冲突及 `TERM/KILL`。

### 8.1 开发分支验收记录（2026-08-08）

- Linux Go `go test ./...` 与 `go vet ./...`：通过；
- Web：`69` 个文件、`451` 项测试通过，typecheck、i18n（`1712` 条）和生产构建通过；
- 路由预算：`ProcessManagerView` JS gzip `5.21 KiB`、CSS gzip `1.85 KiB`；
- 8192 条过滤排序基准：`3.30 ms/op`、`917688 B/op`、`4 allocs/op`（WSL2，i5-12600KF）；
- Linux `amd64/arm64` 的 `paneld`、`kejilion-agent`、`kejilion-node`、`kpctl` 构建通过；
- 本地视觉：桌面端、390 px 移动端、搜索、详情、确认动作及中英文动态文案通过，控制台无错误；
- 开发分支运行 `scripts/verify-deploy.sh` 时，WSL Ubuntu 的 GNU `sha256sum` 不支持测试替身转译出的
  `-s`，在未进入本次功能路径前失败；该历史结果不作为发布通过证据，发布候选必须重新通过 L3、CI
  和正式镜像验收。
