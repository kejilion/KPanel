# 宿主机系统兼容矩阵

KPanel 的 Panel 只维护一个 `linux/amd64`、`linux/arm64` 多架构 Docker
镜像。发行版差异由宿主机 `kejilion-agent` 处理，不为 Debian、Ubuntu、
Rocky 等系统分别制作 Panel 镜像。

## 支持层级

| 层级 | 宿主机 | 当前状态 |
| --- | --- | --- |
| 已实机验证 | Debian 13 `amd64` | 154 测试机持续运行；监控、网站、Docker 和系统管理已验收 |
| 已实现，待实机准入 | Debian 12、Ubuntu 22.04/24.04 | APT/dpkg 与 systemd 路径已实现 |
| 已实现，待实机准入 | Rocky、AlmaLinux、CentOS Stream、RHEL、Oracle Linux、Fedora | DNF/DNF5/YUM 与 systemd 路径已实现 |
| 已实现，待实机准入 | Arch Linux、Manjaro | Pacman 与 systemd 路径已实现 |
| 已实现，待实机准入 | openSUSE Leap/Tumbleweed、SLES | Zypper 与 systemd 路径已实现 |
| 暂不支持正式安装 | Alpine Linux | 宿主机使用 OpenRC，当前 Agent 安装和事务执行依赖 systemd |
| 工具探测适配 | 其他或无法识别的 systemd Linux | 检测本机 APT/dpkg、DNF/DNF5/YUM、APK、Pacman 或 Zypper；没有已实现工具时返回明确的缺失适配器原因 |

“已实现”表示代码路径和固定命令矩阵通过自动化测试，不等于已经完成对应发行版
的真实服务器验收。进入正式支持层级前，必须在干净实例上完成安装、更新、清理、
重启恢复和回滚演练。

## 功能差异

| 功能 | systemd Linux 通用情况 | 发行版限制 |
| --- | --- | --- |
| CPU、内存、负载、磁盘、网络、系统版本 | 读取宿主机 `/proc`、挂载点和系统文件 | 基本不依赖发行版 |
| 网站与 Docker | 读取宿主机 Docker Engine、`/home/web`、`/home/docker` | 依赖 Kejilion 产物布局，不依赖包管理器 |
| 主机名、时区、Swap、IP 优先级、内核优化、BBR | 按命令和内核能力动态开放 | 缺少工具时明确显示依赖未就绪 |
| BBRv3 管理 | x86_64 Debian 12 / Ubuntu 24 及脚本支持的后续版本；可信 `kejilion.sh` 固定协议 | ARM64 外部安装器未固定摘要时只保留 SSH 脚本入口；面板不自动重启 |
| SSH 端口 | 由可信 `kejilion.sh ssh-port` 协议复用脚本现有 `new_ssh_port` 主业务 | 本机脚本必须包含该非交互协议；云安全组仍需在厂商控制面单独放行 |
| SSH 防御 | 由可信 `kejilion.sh f2b manager` 协议读取和管理 Fail2Ban SSH jail、三档策略、封禁与信任地址 | 需要 root、systemd、`fail2ban-client`（未安装时可通过维护任务安装）和 Python 3 地址校验；Alpine/OpenRC 仅保留脚本兼容路径，不属于正式 KPanel 安装目标 |
| DNS 写入 | 可信 `kejilion.sh` 非交互协议；systemd-resolved 原生配置或脚本兼容 `resolv.conf` 事务 | 本机脚本版本过旧或底层 `systemctl`/`chattr` 不可用时禁用 |
| 本地 Hosts | 读取 `/etc/hosts`；可信 `kejilion.sh` 按精确行事务写入 | 文件过大、非普通文件、脚本协议过旧或权限不可信时写入禁用 |
| 定时任务 | 读取并管理 root 用户 Crontab；命令仅作为 Crontab 数据并通过有界 stdin 传输 | 读取需要 root；写入需要 `crontab` 和可信脚本协议；无 Crontab 视为空表，写入与防火墙持久化共享互斥锁 |
| 网卡管理 | 读取 `/sys/class/net` 与 `ip addr`；固定协议即时启停 | 需要 `ip` 与 `CAP_NET_ADMIN`；状态变更不宣称跨重启持久化 |
| 防火墙 | 读取 `iptables-save`；固定端口、地址、PING、DDoS 和全局动作 | 读取需要 `CAP_NET_ADMIN`；写入需要 iptables 工具、可信脚本协议和持久化条件；国家规则未通过来源与事务审计前不从 Web 开放 |
| 软件源读取 | APT、DNF/YUM、APK、Pacman、Zypper | 页面展示实际源主机 |
| 软件源切换 | Debian/Ubuntu APT | 其他系统的换源适配器尚未实现 |
| 系统更新/清理 | APT、DNF/DNF5/YUM、APK、Pacman、Zypper | 固定命令；不接受 Web 传入的包名、命令或 Shell |
| 多主机终端 | systemd Linux `amd64`、`arm64`；KPanel 被控端由 Agent 打开 `/dev/ptmx`，更新后的轻量节点由 root `kejilion-node-terminal.service` 打开固定登录 Shell PTY，并通过 v2 Noise relay 出站 | 本机、新授权 v2 KPanel 和使用新接入命令完成终端公钥注册的轻量节点支持；旧 v1/v2 配对、仅旧遥测配置和非 systemd 环境不支持；其他发行版仍需实机准入 |
| 重装系统 | 非交互适配器未实现 | 需要补齐镜像参数、后台执行和重装后结果回传协议 |

## 部署前置条件

- Linux `amd64` 或 `arm64`。
- systemd，以及 `systemctl`、`systemd-run`；`journalctl` 缺失时只跳过可选日志清理。
- Docker Engine 与 Docker Compose v2。
- 本机 Docker Socket；安装器拒绝远程 `DOCKER_HOST` 或其他 Docker Context。
- 能运行对应架构的无 CGO Agent 二进制。
- 网站功能沿用 Kejilion 标准 `/home/web` 布局，但 `/home/web` 不是安装前置条件。
  全新环境尚未生成 `conf.d`、`html`、`certs` 时，网站列表返回空列表；可信
  `kejilion.sh` 的 WordPress、反向代理和一键建站入口仍可创建站点并初始化所需环境。
  如果只有部分受管目录缺失，则继续按环境异常处理，避免掩盖已有站点损坏。

## 发行版维护命令

| 系列 | 更新 | 缓存清理 | 标准清理 |
| --- | --- | --- | --- |
| Debian/Ubuntu | dpkg 恢复、APT update、full-upgrade | APT clean/autoclean | APT autoremove + 缓存 + journal |
| RHEL/Fedora | DNF/DNF5/YUM update | clean all | autoremove + 缓存重建 + journal |
| Alpine/APK | APK update、upgrade | `apk cache clean` | APK 缓存；标准 Alpine/OpenRC 仍不属于正式安装目标 |
| Arch/Manjaro | `pacman -Syu --noconfirm` | `pacman -Scc --noconfirm` | 校验孤立包名后移除 + 缓存 + journal |
| openSUSE/SLES | Zypper refresh、update | Zypper clean | 缓存刷新 + journal |

所有维护任务均由固定参数的一次性 systemd 服务执行，不自动重启宿主机，不清理
Docker、网站目录、KPanel 备份、`/tmp` 或完整日志目录。
