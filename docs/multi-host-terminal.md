# KPanel 多主机终端设计与安全契约

- 状态：已实现并通过前后端、竞态与双架构自动化验收；完整部署契约及 L3 实机验收待发布前执行
- 页面：`/terminal`
- 支持范围：本机 KPanel、使用新 v2 权限重新配对的远端 KPanel
- 暂不支持：轻量监控节点、旧 v1 配对和未授予终端权限的旧 v2 配对

## 1. 产品交互

用户从左侧“终端”进入后，先看到连接列表：

- 本机显示“本机终端”；
- 已授予终端权限的远端 KPanel 显示“加密直连”；
- 轻量节点显示“轻量监控节点”，旧配对显示“需要重新配对”，两者不可开启终端；
- 每台主机只打开一个页签，可以在多个主机页签间切换；关闭页签会同时终止对应 PTY；
- 主机终端始终在底部显示预输入框；应用、建站、体检和环境任务在脚本等待输入时显示同一套
  预输入框。浏览器本地完成编辑，按 Enter 后整行发送；xterm.js 原生键盘输入仍可用于方向键、
  密码和 TUI，并以 24 ms 短批次合并，降低高延迟网络中的逐键请求开销；
- 每个终端保留 5000 行浏览器回滚缓冲并显示主题化滚动条；用户位于底部时新输出自动跟随，
  向上查看历史时不强制跳回，标题栏按钮可随时回到最新输出；
- 多主机连接列表使用独立滚动区域，不参与右侧终端的高度计算；主机数量增加时不得挤压终端
  输出区或底部预输入框；
- 输出中的普通文本 `http://`、`https://` 地址可在新页面打开；不接受其他协议；终端输出嵌入的
  OSC 8 任意链接和 OSC 52 剪贴板指令均被拦截。

浏览器刷新、退出登录或 Panel/Agent 关闭不会把终端转为后台任务。连接丢失时页面重连读取
内存环形缓冲；超出容量的旧输出明确标记为截断。需要长期执行的安装、更新和备份仍必须使用
已有后台任务体系，不能依赖终端保持浏览器连接。

## 2. 链路与信任边界

```text
本机：浏览器 Session + CSRF/Origin → paneld → Agent Unix Socket → 固定登录 Shell PTY

远端：浏览器 Session + CSRF/Origin → 中心 paneld
      → Noise v2 已认证加密通道 → 目标 paneld
      → 目标 Agent Unix Socket → 固定登录 Shell PTY
```

- 不开放新的 TCP、SSH、WebSocket 或 Agent 公网监听端口；
- Panel 继续无特权运行，宿主机 PTY 仅由 root Agent 创建；
- Agent 主服务因文件管理根目录为 `/` 而保持宿主机文件系统可写，Panel 状态目录继续通过
  `ReadOnlyPaths` 独立保护。主机终端仍由 Agent 通过固定参数创建独立 transient systemd PTY，
  以独立生命周期和审计边界提供 `apt`、`dnf` 等系统维护能力；其他 Agent API 仍只能调用各自的
  固定适配器；
- 远端终端只允许已激活且 scope 包含 `cluster.terminal.open` 的 v2 控制端；当前合法 scope 为
  `cluster.summary.read cluster.terminal.open` 或新增文件读取权限后的
  `cluster.summary.read cluster.terminal.open cluster.files.read`；
- 现有 v1 和旧 v2 授权不自动扩权，管理员必须撤销后重新配对；
- 轻量节点以无登录低权限账户运行，首版只做监控，不能通过升级静默获得 root Shell。

## 3. 固定 API

浏览器接口：

```text
POST /api/v1/terminal-sessions
GET  /api/v1/terminal-sessions/{id}/output?offset={n}&wait={0..1000}
POST /api/v1/terminal-sessions/{id}/input
POST /api/v1/terminal-sessions/{id}/resize
POST /api/v1/terminal-sessions/{id}/close
```

Panel 间接口：

```text
POST /api/v2/federation/terminal/open
POST /api/v2/federation/terminal/output
POST /api/v2/federation/terminal/input
POST /api/v2/federation/terminal/resize
POST /api/v2/federation/terminal/close
```

Agent 接口仅位于权限受限的 Unix Socket：

```text
POST /v1/terminals
GET  /v1/terminals/{id}/output
POST /v1/terminals/{id}/input
POST /v1/terminals/{id}/resize
POST /v1/terminals/{id}/close
```

接口不接受浏览器提交的 Shell 路径、启动参数、用户、环境变量或工作目录。Agent 只启动
`/bin/bash -l`，不存在时回退 `/bin/sh -l`，并固定 `TERM=xterm-256color`。systemd 部署环境使用
固定 transient unit 参数，最长运行 8 小时；关闭会话时同时停止 unit，Agent 停止或重启时也会结束会话。

## 4. 资源、生命周期与审计

| 项目 | 上限或规则 |
| --- | --- |
| 全局活动会话 | 16 |
| 单一 Panel 用户 / 联邦控制端 | 4 |
| 单次输入 | 16 KiB |
| 单次输出 | 本机最多 64 KiB；远端加密封装最多 32 KiB，可按偏移连续读取 |
| 每会话输出缓冲 | 1 MiB，仅内存 |
| 闲置关闭 | 30 分钟 |
| 最长会话 | 8 小时 |
| 浏览器长轮询 | 最长 1 秒 |
| Panel 失联会话索引 | 35 分钟后回收 |

Panel 公共会话 ID 使用独立 256 位随机值并绑定当前管理员 ID，不向浏览器暴露 Agent 或远端
真实会话 ID。本机会话在 Agent 或 Panel 退出时关闭；远端关闭请求因网络不可达而无法送达时，
由目标 Agent 在最长 30 分钟闲置期后回收。审计只保存打开/关闭、目标主机、结果和管理员，
不保存输入、输出、命令历史、环境变量或终端回放。

## 5. 安全与失败规则

- 浏览器所有写操作验证 Session、Origin 和 CSRF；跨用户会话查询返回 404；
- Panel 间正文使用现有 Noise IK 身份认证、时间窗和 request ID 防重放，HTTP 链路上不出现
  命令或输出明文；HTTPS 仍执行证书验证，HTTP 字面量 IP 仅承载 Noise 密文；
- 维度、偏移、等待时间、请求体、输入、输出、并发和内存全部有界；
- 终端能力本质上等价于 root SSH。它不是任意 Shell API 的替代品，其他页面仍只能调用固定
  结构化动作，不能复用终端接口拼接后台业务命令；
- 网络断开只改变前端连接状态，不把“暂时没读到输出”误判为进程失败；PTY 真实退出才显示结束；
- 远端撤销授权后不能新建终端；既有会话由目标 Agent 的会话上限、闲置时间和进程生命周期回收。

## 6. 验收与回滚

发布前至少验证：

- 本机 open/output/input/resize/close；用户隔离、CSRF/Origin 和会话上限；
- v2 远端完整生命周期、权限不足拒绝、重放拒绝以及命令明文不出现在传输中；
- UTF-8 输入、整行预输入、控制键、24 ms 合并、窗口缩放、5000 行滚动、智能跟随、手动回到底部、
  输出截断、断线重连和 URL 安全跳转；
- 以超过单屏高度的主机和体检命令列表复核独立滚动，确认终端输出和预输入框始终可见；
- 从测试 Agent 启动主机终端，验证 transient PTY 拥有独立 systemd 单元、可写系统目录并能运行
  系统维护命令；同时验证应用、建站、体检和环境任务仍只能调用各自的固定适配器；
- 本机 Panel/Agent 退出关闭 PTY；远端关闭成功或链路中断后的闲置回收、过期索引回收均符合约定；
- `amd64`、`arm64`，Chrome/Edge 桌面与移动端布局；
- CPU、内存、连接数和 16 会话上限下的资源预算。

回滚到上一版本不会修改集群状态、网站、Docker 或宿主机配置。新 v2 配对记录中的 scope 会被
旧版本忽略；终端会话仅在内存中，回滚或重启会全部关闭，不需要数据迁移。
