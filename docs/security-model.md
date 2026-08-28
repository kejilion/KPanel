# KPanel 攻击面与操作边界

本文件服从仓库根目录的 [`PROJECT_RULES.md`](../PROJECT_RULES.md)。KPanel 将“防攻击”与
“限制管理员操作”严格分开：前者必须保留，后者不得作为产品能力门槛。
性能、稳定性、资源预算和网络入侵防护的强制开发与发布标准见
[`development-quality-standard.md`](development-quality-standard.md)。

## 1. 授权模型

- 浏览器只连接无特权的 `paneld`；`kejilion-agent` 是唯一宿主机特权入口，仅监听 Unix Socket。
- 登录、Session、CSRF/Origin、Agent Token 和 Unix Socket 文件权限决定“谁可以操作”。
- 通过鉴权的管理员可以调用已实现的业务动作。资源来源、KPanel label、Compose 工作目录、
  `appno.txt` marker、人工漂移、`privileged`、host network、设备挂载、宿主机目录挂载以及
  “这是 KPanel 自身”都不能决定“是否允许操作”。
- 危险动作可以显示影响并要求一次普通确认，但后端不得要求固定确认词，也不得把确认字段当成
  capability 开关。
- 资源状态、依赖和底层工具真实不支持某动作时可以拒绝。例如不存在目标、资源版本已变化、
  容器状态不接受该 Docker 动作、系统缺少命令或当前发行版尚无适配器。错误必须说明真实原因。

## 2. 必须保留的防攻击控制

| 边界 | 保留措施 | 防范问题 |
| --- | --- | --- |
| 身份 | Argon2id、一次性 Bootstrap Token、服务端 Session、登录限速、可选 TOTP 与一次性恢复码 | 未授权访问、撞库、密码泄露后的账户接管 |
| Web 请求 | CSRF Token、严格 Origin、可信代理 CIDR、Secure/HttpOnly Cookie | 跨站请求伪造、代理头伪造 |
| Agent | Unix Socket、独立 Bearer Token、最小 systemd 权限 | 绕过 Panel 直接提权 |
| 输入 | 类型化 API、枚举或语法校验、禁止把 Web 字段拼成宿主机 Shell | 命令注入 |
| 文件 | 固定业务根、路径穿越检查、符号链接检查、原子替换 | 任意文件覆盖、竞态篡改 |
| 并发 | `resourceVersion`、任务互斥、提交前回读 | 旧页面覆盖新状态、并发破坏 |
| 事务 | 配置语法检查、备份、失败回滚、需人工处理状态 | 半成品配置、静默失败 |
| 数据 | Token、密码、私钥和环境变量脱敏；初始应用信息只在管理员任务日志展示 | 凭据泄露 |
| 资源 | 响应、日志、任务时长、并发数和备份体积上限 | 内存、磁盘和连接耗尽 |
| 出站下载 | 无特权 Panel 固定 GET、公开地址策略、DNS 固定直拨、逐跳重定向复核、URL 脱敏 | SSRF、云元数据读取、签名 URL 泄漏 |
| 终端 | 独立 Session、CSRF/Origin、Agent Unix Socket、Noise v2 scope、轻量终端专属 Noise 出站 relay、遥测 reporting key 隔离、独立 root broker、固定 PTY 动作、会话/输入/输出上限、无回放 | 未授权 root Shell、入站 SSH 暴露、重放、明文泄漏和资源耗尽 |
| 供应链 | 固定来源、内容/结构校验、本地回退目录 | 远程目录获得执行权限 |

这些控制约束请求和执行过程，不按资源归属缩减管理员业务功能。

TOTP 的注册、密钥保护、恢复和防绕过要求见
[`two-factor-authentication.md`](two-factor-authentication.md)。两步验证只增强正常认证链路，
不能替代每个 API 的 Session 鉴权，也不能防御未认证 RCE、路径穿越或 Session 劫持。

## 3. 真实状态与双端互通

- Docker Engine、Nginx 配置、systemd、系统文件和 `kejilion.sh` 兼容产物是共享事实。
- KPanel 每次读取真实产物并计算 `resourceVersion`；Panel 持久化只允许保存账户、Session、
  任务索引、审计和必要缓存，高权限任务状态由 Agent 独立保存。
- 脚本、Compose、CLI 或 Web 创建的容器都可按实时状态执行生命周期、日志、性能、控制台和访问控制。
- 网站更新尽量在当前配置结构上补丁修改；无法解析的配置仍可按资源 ID 删除，不要求 KPanel marker。
- 网站图标接口不是任意 URL 代理：Agent 只接受已发现站点 ID，网络连接固定到本机
  `127.0.0.1:80/443`，并限制域名、重定向、字节数、并发、超时和位图格式；Panel 再做一次
  媒体校验。缓存属于可删除的派生数据，不进入网站事实和资源版本。
- 文件远程下载同样不是通用 URL 代理：只有已认证管理员可提交公开 HTTP/HTTPS 内容地址，
  无特权 Panel 在禁止代理和凭据的专用客户端中完成 DNS 全量校验、固定 IP 拨号及逐跳重定向
  复核，再把有界字节流通过 Unix Socket 交给既有 Agent 上传动作。root Agent 不接收 URL、
  不获得公网出站能力；完整 URL、查询签名和响应正文不进入审计、持久任务状态或用户错误。后台
  下载只在 Panel 进程内保留 URL，页面关闭后可继续；Panel 重启时 active 索引转为 `interrupted`
  且不自动重放，避免持久化凭据或在原子提交不确定窗口重复下载。
- 应用 marker 与 label 只用于发现和展示。脚本管理协议是否存在、容器是否存在、端口是否可识别属于
  动作所需的技术前置条件，不是资源归属授权。
- 写入后必须继续生成 `kejilion.sh` 可识别的目录、配置和 marker；脚本侧变更后 Web 刷新即可对账。

## 4. 高风险动作的处理方式

停止、删除、Prune、覆盖恢复、重启、卸载 KPanel 自身和未来的系统重装都属于正式业务能力：

1. 页面说明实际影响并使用普通确认；
2. 后端校验身份、请求结构、目标 ID 和资源版本；
3. 直接调用对应底层动作，不检查“是否由 KPanel 创建”；
4. 记录审计与后台任务进度；
5. 可回滚的动作失败时回滚；不可回滚的动作如实报告已完成步骤和剩余状态。

操作风险不是隐藏按钮、禁用 API 或返回 `403` 的理由。

## 5. 当前适配缺口

以下能力尚未完成时，应显示“缺少适配器/依赖”，不能显示“为了安全受保护”：

- 系统重装的非交互任务协议及重装后结果回传；
- 尚未升级到固定 DNS 非交互协议的旧版 `kejilion.sh`；
- 尚未包含 `system-resource` 固定协议的旧版 `kejilion.sh`；此时 Hosts、定时任务、网卡和防火墙
  保持真实状态只读并明确显示脚本适配器缺失；
- 非 Debian/Ubuntu 的系统更新源切换适配器；
- 交互式容器 TTY、Compose 与 `daemon.json` 通用结构化编辑器；宿主机多主机终端和更新后
  light-v1 节点终端已按 [`multi-host-terminal.md`](multi-host-terminal.md) 实现；
- 部分应用的专属交互安装参数，以及没有主容器时的应用级生命周期动作；
- 无法解析的复杂 Nginx 结构化更新适配器。

这些条目必须进入生态对齐矩阵逐项实现；不得把“只读查看”算作完成。

## 6. 代理部署

`k fd` 可以把域名反代到 KPanel 直连端口。只有立即来源位于配置的可信代理 CIDR，且
`X-Forwarded-Proto` 为单一 `https` 时，Panel 才接受代理传入的 HTTPS Host/Origin、设置 Secure
Cookie 并解析真实客户端地址。公网客户端伪造转发头不会改变信任边界。
