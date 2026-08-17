# 系统监控与管理

KPanel 的系统管理业务以 `kejilion.sh` 为功能基准，但不把整个 Shell
脚本暴露给 Web。Agent 只提供固定输入、固定输出的类型化接口；Panel
负责登录、CSRF、Origin 校验和审计，不能传入任意命令。

## 状态一致性

- 首页状态直接读取宿主机当前配置，不以 KPanel 数据库代替系统事实。
- `kejilion.sh` 或人工修改产生的 SSH、DNS、时区、Swap、更新源、Hosts、
  root Crontab、网卡、iptables、`gai.conf`、内核优化和 BBR 状态，刷新后应被
  Agent 重新识别。
- Web 后续执行变更后必须回读同一状态接口；回读不一致时任务不得标记成功。
- 新字段保持向后兼容。旧 Agent 未返回时，前端显示“待 Agent 升级”，
  不构造虚假的默认状态。

## v0.8 `k info` 信息对齐

系统首页按“主机信息、网络与位置、实时指标、资源一致性”展示
`kejilion.sh` 的 `linux_info` 信息：

| `k info` 信息 | KPanel 展示 |
| --- | --- |
| 主机名、系统版本、内核、架构 | 主机信息 |
| CPU 型号、核心数、频率、使用率 | 主机信息与实时指标 |
| 1/5/15 分钟负载 | 主机信息 |
| TCP/UDP 连接数 | 网络与位置 |
| 物理内存、Swap、根磁盘 | 实时指标与资源一致性 |
| 累计接收/发送流量 | 主机信息 |
| 拥塞算法、qdisc | 网络与位置 |
| DNS、时区、宿主机时间、运行时长 | 网络与位置、主机信息及管理工具 |
| 公网 IPv4/IPv6、ISP、国家/地区/城市 | 网络与位置 |

本地信息直接读取 `/proc`、`/sys` 和宿主机配置。公网信息使用固定的
IPinfo IPv4/IPv6 HTTPS 端点，成功结果缓存 30 分钟；外部查询超时或不可用时，
本地监控照常返回，页面只把公网字段标记为不可用。可设置
`KEJILION_PUBLIC_NETWORK_LOOKUP_ENABLED=false` 完全关闭该查询。

## 功能与完整性边界

| 业务 | 脚本产物识别 | Web 技术前置条件 |
| --- | --- | --- |
| 主机名 | kernel hostname | hostname 规则校验、原子更新、回读 |
| SSH 端口 | `sshd_config` 与片段 | 调用本机可信 `kejilion.sh ssh-port` 非交互适配入口；脚本复用原有 `new_ssh_port` 主业务，并强制验证配置语法与新端口监听；Agent 负责结构化校验、执行前备份和结果回读 |
| SSH 防御 | `k f2b manager status|enable|disable|set-profile|unban|unban-all|add-trusted|remove-trusted|uninstall` | 读取真实 Fail2Ban SSH jail；提供三档策略、封禁 IP 解封、信任地址和最近事件；启停/卸载由后台任务执行 |
| DNS | `resolv.conf`、systemd-resolved 与脚本 DNS 协议 | 调用本机可信 `kejilion.sh` 固定非交互入口；systemd-resolved 使用原生配置，其他管理器沿用脚本的 `resolv.conf` 写入与锁定语义 |
| 时区 | `/etc/timezone` 或 `localtime` | 有效 IANA 时区名称、回读 |
| 虚拟内存 | `/proc/meminfo`、`/proc/swaps`、`/swapfile` | 与 `kejilion.sh` 共用 `/swapfile`；合并旧版 KPanel Swap，不清除现有分区或第三方 swapfile |
| 系统更新源 | APT/RPM/APK/Pacman/Zypper 源地址 | Debian/Ubuntu 对齐脚本四种区域模式；第三方源不修改，隔离 `apt-get update` 失败回滚 |
| 本地 Hosts | `/etc/hosts` 原始内容 | 按精确行管理；写入前后校验整文件资源版本，保留注释、权限和属主，失败回滚 |
| 定时任务 | root 用户实际 Crontab | 命令只作为 Crontab 数据写入，并通过有界 stdin 帧传给可信脚本，不出现在特权进程 argv；按精确行新增、修改或删除，保留注释、环境变量和未知行 |
| 网卡 | `/sys/class/net`、`ip addr` | 展示全部接口；只执行即时启停，不宣称持久化，失败恢复原管理状态 |
| 防火墙 | `iptables-save`、`INPUT`/`DOCKER-USER` 与脚本持久化产物 | 固定动作、整表资源版本、互斥锁、写后回读和失败回滚；资源版本忽略生成时间与实时计数器，仅对策略和规则变化冲突；不接受任意规则或 Shell；“全部开放/关闭”沿用脚本清空 filter 规则和自定义链后重建基础规则的高影响语义，页面必须明确提示 |
| V4/V6 优先 | `gai.conf` | 维护 `kejilion.sh` 同一 precedence 规则并保留其他用户配置 |
| 内核优化 | Kejilion sysctl 产物 | 五种固定预设、内存自适应、逐项应用和版本化回滚；合法脚本产物可接管 |
| BBR | 当前/可用拥塞算法与 qdisc | 内核能力检查、独立 sysctl 文件、回读 |
| BBRv3 | `k bbrv3 status|install|update|uninstall` | 可信 `kejilion.sh` 固定协议；XanMod 状态回读、systemd 后台任务、完成后提示重启 |
| 系统更新 | APT/DNF/YUM/APK/Pacman/Zypper 源与后台任务状态 | 已实现对应包管理器和 systemd 后台执行器 |
| 系统清理 | 软件包管理器与 journal 后台任务状态 | 缓存或标准策略；动作差异必须在生态对齐矩阵中明确 |
| 重启服务器 | systemd 能力 | 普通确认后固定延迟约 15 秒执行；维护任务不构成禁止条件 |
| 重装系统 | 不适用 | 非交互后台执行与重装后结果回传适配器尚未实现 |

## v0.3 写入范围

Agent 根据宿主机命令、配置管理器和 root/sandbox 条件动态返回 capability。
满足技术条件时，登录管理员可在页面填写结构化字段并普通确认；不满足条件时展示
真实状态和缺少的命令、服务或适配器。

已开放：主机名、`kejilion.sh` 同源 SSH 单端口切换、同源 SSH 防御、DNS、时区、脚本兼容
`/swapfile`、Debian/Ubuntu APT 四种区域镜像预设、地址优先级、KPanel 内核调优预设和
BBR、BBRv3、本地 Hosts、root 定时任务、网卡即时启停、固定防火墙动作，以及普通确认的服务器重启。
重装系统、其他发行版换源和通用
sysctl 编辑目前缺少适配器；这不是永久产品限制。

## 概览快捷入口与系统中心分区

概览只保留 3×2 的六个高频入口：虚拟内存、SSH 端口、DNS 优化、V4/V6 优先、BBR 管理和一条龙优化；
“更多设置”进入完整系统中心。系统中心按操作目的分成六个顺序稳定的区块：

- 日常维护：系统更新、系统清理和服务器重启；
- 基础配置：虚拟内存、主机名、时区、系统更新源和定时任务；
- 登录与安全：SSH 端口、SSH 防御、账户管理和防火墙；
- 网络与流量：DNS、端口占用、网卡、V4/V6 优先、本地 Hosts 和限流自动关机；
- 性能优化：一条龙优化、BBR 管理、内核调优和 BBRv3；
- 危险操作：系统重装。

新增系统日志等能力时应放入对应目的分区；只有形成独立、持续增长的管理域时才增加新区块，避免打乱现有顺序。

Hosts、定时任务、网卡和防火墙使用独立类型化资源接口。概览首屏只读取已有系统摘要，
用户打开相应管理器时才读取有界列表；旧 Agent 没有对应 capability 时显示升级或适配器原因，
不会构造空数据冒充真实状态。每次写入携带当前 `resourceVersion`，脚本在互斥锁内再次比对，
成功后 Web 重新读取同一资源。

四类写操作统一调用可信 `kejilion.sh` 的
`KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1 k kpanel system-resource` 固定协议。Agent 仅信任包含
`KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="3"` 的脚本版本；该版本明确包含 Cron stdin 契约、稳定的
防火墙规则版本及持久恢复备份。动作、资源版本和结构化字段以
固定 argv 传递；Cron 命令正文通过最大 2048 字节、单行终止的 stdin 帧传递，避免泄露到进程列表。
脚本适配器在请求期间只把命令保存为 Crontab 数据，不立即执行；之后由系统 Cron 按计划运行。
防火墙端口动作与脚本一致，同时处理 TCP 和 UDP。脚本现有国家规则依赖
未固定摘要的远程地址列表，且默认策略与解除规则尚未满足事务要求，因此本轮不向 Web 暴露。

root Crontab 读取要求 Agent 实际以 root 运行；防火墙读取要求 `CAP_NET_ADMIN`。回滚失败或脚本报告需要
人工检查时，接口保留 `503` 状态表达当前不可用，但明确返回 `retryable: false`，不得自动重复特权写入。
回滚失败的快照只接受 `/var/lib/kejilion-panel/system/recovery/system-resource/` 下的 root-only 持久路径；
Agent 的私有 `/tmp` 路径不会作为可恢复备份回传。
进程内写锁最多等待 2 秒，脚本跨进程锁最多等待 5 秒；取得写锁后的事务使用独立 45 秒期限，浏览器断开
不会中止正在进行的系统写入和回滚。

### SSH 防御业务模型

SSH 防御以“状态、强度、封禁、信任”四个区域呈现，不暴露 Fail2Ban 的复杂 jail 配置：

- 防御开关：未安装时一键安装并启用；停用保留配置；卸载放在页面底部并单独确认；
- 防御强度：温和、标准、严格三档，默认推荐标准；页面直接说明失败次数和封禁时长；
- 已封禁 IP：支持 IP 搜索、单个解封和全部解封；最多返回 256 条并明确截断状态；
- 信任地址与最近事件：支持精确 IP/CIDR 增删，展示最近 20 条登录失败、封禁和解封事件。

页面状态每次来自 Fail2Ban 服务、实际 SSH jail、有效 `bantime/findtime/maxretry`、`ignoreip` 和日志，
Panel 不保存第二份规则。策略与信任地址写入独立受管配置片段，脚本在共享 root-only 锁内复核
`resourceVersion`，备份后原子替换，执行 `fail2ban-client -t`、reload 和回读；失败时恢复原配置。
动态封禁和日志不参与资源版本，避免正常攻击流量造成无意义的写冲突。启停和卸载继续使用现有
systemd 维护队列，关闭浏览器不会中断；停用不删除配置，卸载才移除 Fail2Ban 及其配置。

该功能使用 `KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"`，固定到
`kejilion/sh@28f89c1b34df4b25e6ef9b144c328fdea75dbac9` 与原始 SHA-256
`0583f7cd5be1f0bb6ec48d92e2cf224bfabfafada5788658bda4414ba9561229`。
2026-08-11 已在 154 的隔离 Ubuntu 24.04 root 容器内以真实 Fail2Ban 完成信任地址、封禁解封、
三档策略、Agent/Panel 审计和浏览器桌面/窄屏闭环；生产宿主未安装或修改 Fail2Ban，测试资源已清理。

### 账户管理业务模型

账户管理不复制 `kejilion.sh` 的交互菜单，而是把同一业务目的整理为四个简单区域：

- 账户：默认展示 Root 与可登录用户，可按需查看系统账户；支持创建、角色调整和删除；
- 登录凭据：为任意现有账户修改密码、添加或按精确 ID 删除 SSH 公钥；不显示 shadow 哈希；
- SSH 登录策略：密码兼容模式表示密码与公钥同时可用，密钥模式表示关闭全局密码认证并保留公钥认证；
  Root 独立选择密码/密钥、仅密钥或禁止登录；
- 禁用 Root 向导：同一事务先创建替代管理员，再锁定 Root 密码并禁止 Root SSH 登录；密码凭据创建
  需要密码的 sudo 管理员，密钥凭据创建 `NOPASSWD` 管理员并关闭全局密码认证。

系统账户、密码锁定状态、sudo/wheel、每个用户的 `authorized_keys` 以及 `sshd -T` 有效结果是唯一状态真源；
Panel 不保存账户副本。用户名、角色和动作使用固定字段，密码或公钥只通过最大 256/4096 字节、单行 LF
终止的 stdin 帧传给可信脚本。审计只记录用户名、角色、登录方式和“已提供秘密”，不会保存秘密正文。

SSH 策略由脚本维护独立配置片段，并确保主配置包含该目录；应用前执行 `sshd -t`，再 reload SSH。
账户数据库、托管 sudo 文件和 SSH 配置在事务前建立 root-only 有界快照；失败时回滚，回滚失败才把快照
迁移到 `/var/lib/kejilion-panel/system/recovery/system-resource/`。删除账户可按明确确认删除其主目录；由于
底层 `userdel -r` 的文件删除不可逆，界面必须直接说明这一影响，不能宣称能够恢复已删除的主目录。

该功能使用 `KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION="1"`，固定到
`kejilion/sh@28f89c1b34df4b25e6ef9b144c328fdea75dbac9` 与原始 SHA-256
`0583f7cd5be1f0bb6ec48d92e2cf224bfabfafada5788658bda4414ba9561229`。2026-08-11 已在隔离的
Ubuntu 24.04 root Linux 环境完成密码/密钥账户、Root 密码与安全迁移、三种角色、公钥增删、SSH
策略 reload、删除、版本/锁冲突、失败回滚、`rollback-failed`、`needs-attention` 及原状恢复的
Shell→Agent→Panel L2 闭环。

## v0.29 BBRv3 管理

- 页面与普通 BBR 并排展示，状态始终来自 `kejilion.sh` 的
  `KJ_BBRV3_NONINTERACTIVE=1 k bbrv3 status`，不以 Panel 数据库推断安装结果。
- Web 只接受 `install`、`update`、`uninstall` 三个枚举动作；Agent 通过可信脚本校验后，
  使用现有 systemd 维护队列执行，同一时间不与系统更新、清理或 SSH 防御并发。
- 安装、更新和卸载继续复用脚本的 XanMod 软件源、CPU PSABI 匹配、磁盘/Swap 检查、
  BBR + fq 配置和包管理语义；KPanel 不维护第二份内核安装命令。
- 任务完成后只标记“需要重启”，不会自动重启。用户可使用同页受控重启入口，并在重连后
  由状态接口复核运行内核包含 XanMod、拥塞算法为 `bbr` 且 qdisc 为 `fq`。
- 当前 KPanel 受控安装支持 x86_64 的 Debian 12 / Ubuntu 24 及脚本列出的后续受支持版本。
  原 ARM64 菜单依赖未固定摘要的第三方安装器，因此只保留 SSH 脚本入口，不从 Web 自动执行。
  已安装但发行版已不在支持列表时仍允许卸载。

## v0.5 后台系统维护

“系统更新”和“系统清理”参考当前 `kejilion.sh` 的业务顺序，但由固定参数的
systemd transient service 执行。Web 请求只能选择 `update/full`、
`cleanup/cache` 或 `cleanup/standard`，不能传入命令、包名或文件路径。

- 更新：APT 执行 dpkg 恢复、刷新索引和 `full-upgrade`；RHEL 系执行
  DNF/DNF5/YUM 缓存刷新与升级；Arch/Manjaro 执行 `pacman -Syu`；
  openSUSE/SLES 执行 Zypper 刷新与升级。
- 缓存清理：只调用对应软件包管理器的固定缓存清理参数。
- 标准清理：APT、DNF/YUM 在自身支持时额外执行 `autoremove`；Pacman 对原生
  孤立包输出做包名语法校验后执行删除。所有 systemd 系统轮转 journal，保留最近 7 天并
  限制到 500 MiB。
- 后台状态持久化在
  `/var/lib/kejilion-panel/system/maintenance-state.json`；同一时间只允许
  一个维护任务。
- 只有 worker 原子写入的 `succeeded` 状态才是完成凭据；systemd 单元已退出、被
  `--collect` 回收或返回默认的 `Result=success`，均不能替代业务完成状态。未取得
  完成凭据时必须显示未验证失败，不能从启动进度直接推断成功。
- systemd 对账属于并发观察者；写入任何推断终态前必须重新读取原子状态文件。若 worker
  已推进阶段或写入终态，对账只能返回该新状态，不能用先前读取的启动快照覆盖。
- 更新和清理属于不可逆的软件包事务，KPanel 不宣称自动回滚；失败时保留
  阶段和错误摘要供人工检查。任务不会自动重启宿主机。

系统备份保存在 `/var/lib/kejilion-panel/system/backups`。Panel 数据、
`kejilion.sh` 文件、现有网站、其他容器和其他 Swap 不进入系统操作事务。

## v0.12 系统更新源切换

- 页面入口与 `kejilion.sh` 系统工具第 19 项一致：中国大陆【默认】、中国大陆【教育网】、
  海外地区、智能切换更新源。
- 为适应无交互 Web，前三个区域固定使用 LinuxMirrors 当前对应列表的首选线路：
  阿里云、北京大学和 xTom 香港。智能模式按脚本逻辑执行：
  `CN → mirrors.huaweicloud.com`，Debian/Ubuntu 海外主机回到发行版官方源。
- 智能地区识别仅访问固定的 IPinfo HTTPS 国家端点，4 秒超时；查询失败时不猜测中国线路，
  明确回退官方源。该查询只在管理员确认执行智能换源时发起。
- Agent 识别 LinuxMirrors 当前默认、教育网和海外列表中的主机名，并从 URL 中定位
  `debian`、`debian-security`、`ubuntu` 或 `ubuntu-ports` 仓库路径。因此脚本先换源、
  Web 后换源，或 Web 先换源、脚本后换源，首页都以宿主机实际源地址为准。
- Web 不接受镜像 URL，不下载或执行远程 `main.sh`。修改前备份所有实际变化的 APT 文件，
  然后在 `/var/lib/kejilion-panel/system/apt-validation` 独立 lists/cache 中执行短超时
  `apt-get update`；任何文件写入或索引验证失败都会恢复原文件。
- 换源动作与脚本的 `upgrade_software=false`、`clean_cache=false` 一致：不升级软件包、
  不清理缓存。Docker、NodeSource 等第三方源保持不变。
- KPanel 的系统维护读取范围已覆盖 RPM、Pacman 和 Zypper，但本版换源写入仍只开放
  Debian/Ubuntu。其他发行版继续显示真实源和缺少换源适配器的原因，不伪装为已支持。

## v0.11 服务器重启

- 页面使用一次普通确认；Panel 继续执行登录、Origin、CSRF 和审计校验。
- Agent 只接受固定的 `reboot` 动作，不要求固定确认词，也不接受 Shell、命令参数、自定义延迟或计划时间。
- 通过 `systemd-run` 创建一次性 transient timer，固定延迟约 15 秒调用系统
  `systemctl --no-wall reboot`，让 Panel 有时间落盘成功审计并向浏览器返回结果。
- 软件包更新或清理任务运行时仍允许管理员重启；缺少 systemd 工具、Agent 写入开关关闭、非 Linux 或
  Agent 非 root 时，页面显示真实依赖原因。
- 重启会短暂中断 KPanel、网站和 SSH。KPanel 不声称业务已安全停机，因此管理员仍需先确认
  数据库迁移、备份、长连接和外部任务状态。

## v0.5.1 虚拟内存事务

- 状态读取同时区分 `/swapfile`、旧版 KPanel Swap 和其他活动 Swap，页面
  的总量仍以 `/proc/meminfo` 为准。
- 设置入口提供 `kejilion.sh` 相同的 1/2/4 GiB 常用值，并允许
  任意正整数 MiB 自定义值或 0 停用；脚本默认值为 1 GiB。
- 创建或调整时在同一文件系统分配临时 swapfile，不以当前内存余量阻止管理员提交。
- 事务只会 `swapoff` `/swapfile` 和旧版 KPanel 路径；不会执行 `wipefs`，
  不会停用 Swap 分区或第三方 swapfile。
- 新文件、`fstab` 和 `swapon` 任一步失败时，恢复原文件、原 `fstab` 和原
  活动状态。成功后启动项使用脚本同款
  `/swapfile swap swap defaults 0 0`，脚本与 Web 可双向识别。
- 文件系统写入由固定参数的 root systemd transient service 完成。常驻
  Agent 仍受原 systemd 沙箱限制，Web 不能传入路径或任意命令。浏览器请求
  中断不会杀死已经启动的事务，事务仍会完成或执行自身回滚。

## 一条龙系统调优

- 入口位于“系统中心 → 性能优化”，概览也保留高频快捷卡片；它与“内核调优”保持独立：前者是 kejilion.sh 原有的一次性系统准备流程，后者用于长期切换内核参数预设。
- 页面固定展示原脚本的 12 个项目并默认全选；管理员可以取消任意项目，但不能提交路径、命令、软件包名或其他 Shell 参数。
- Agent 将所选项目拆成持久化 systemd 后台步骤，逐项调用 `KJ_SYSTEM_TUNING_NONINTERACTIVE=1 k kpanel system-tuning apply-item <id>`。页面关闭或浏览器断开不会取消任务，重新打开会继续显示真实阶段和进度。
- 每个项目由脚本共享锁串行执行并输出结构化回执；项目失败时任务立即停止，后续项目不会被标记为成功。系统更新、清理、换源和安装软件包属于不可整体回滚操作，界面不宣称整套事务可以自动撤销。
- 后台执行分别采集原生操作日志与结构化 stdout 回执，软件包管理器、操作返回码和完成态回读任一失败都会停止；交互菜单 66 使用同一失败停止与回读规则，不再在失败后继续打印 `[OK]`。
- 1 GiB Swap 按 `/swapfile` 的 1 GiB 文件大小、`/proc/swaps` 激活项和 `fstab` 启动项共同判断，不再使用会因 `mkswap` 元数据取整或其他 Swap 而误判的总量；自动 DNS 直接复用 KPanel 已有的固定 DNS 协议，不修改单项 DNS 功能。
- 原脚本第 6 项此前只显示“开放所有端口”而未执行动作；现在交互菜单与 KPanel 都调用同一个事务化 firewall `open-all` 真源实现，并在持久化或回读失败时恢复快照。
- LinuxMirrors 固定到 `649e948763042e485e411be540d21c32cface1c1`，网络参数脚本固定到 `e9c3078eb516b05f9df6d2a9294cf3b226ca02bd`；两者下载后都先校验已登记 SHA-256，再交给 Bash 执行。
- 2026-08-11 已在 154 的隔离 Ubuntu 24.04 systemd 容器完成固定 12 项状态、Panel typed 选择、Agent 后台运行态与恢复、时区安全项成功、首项失败即停止、后续项不执行、资源版本冲突和审计闭环；生产 KPanel、Agent、Swap、SSH 和防火墙均未改动。
- 发布顺序门禁仍保持：必须先发布固定的 kejilion.sh 提交，再构建 KPanel 镜像并复核镜像内脚本摘要；未发布脚本提交时 Docker `ADD` 不可用，不能绕过摘要改用分支或浮动地址。

## v0.6 内核优化

- Web 提供与当前 `kejilion.sh` 一致的高性能、均衡、网站、直播和游戏服五种
  本地预设，以及还原默认设置；API 只接受这些枚举值。
- 预设参数与脚本的 `_kernel_optimize_core` 保持一致，并按 `/proc/meminfo`
  的 `MemTotal` 使用 `<1 GiB`、`1–4 GiB`、`4–16 GiB`、`≥16 GiB` 四档
  自适应规则。
- 产物写入脚本相同的
  `/etc/sysctl.d/99-kejilion-optimize.conf`，保留 `# 模式:` 与 `# 场景:`
  标识。脚本执行后 Web 可读取；Web 切换后脚本菜单也可识别当前模式。
- 已识别的脚本手动预设允许由 Web 切换；`99-network-optimize.conf` 自动调优
  结果可读取并在用户选择其他模式或还原时清理。未知结构的同名文件仍返回冲突。
- 参数使用固定列表逐项 `sysctl -w`，兼容内核缺少可选参数的情况；若全部参数
  都无法应用，则恢复 sysctl、limits、modules 和 BBR 冲突文件并重新加载。
- BBR 可用时与脚本一样使用 `bbr + fq`，否则使用 `cubic + fq_codel`；同时
  管理透明大页、nofile 限制和 `tcp_bbr` 模块持久化。
- 脚本中的“自动调优”需要实时测速并在线获取 `network-optimize.sh`。首版 Web
  不执行远程脚本；已有自动调优产物仍会被状态接口正确识别。

## 网络工具：端口占用与限流自动关机

- 两项能力使用独立的类型化接口，不进入旧 `/system/actions`：端口占用只读，限流关机仅接受
  `enable`、`disable`、双向 GiB 阈值、每月重置日和资源版本。
- 端口记录由可信 `kejilion.sh` 固定执行 `ss -H -lntup`；脚本限制原始结果为 4 MiB/4096 行，
  最多向 Agent 返回 512 条，超出时明确标记截断。页面只在打开弹窗后读取，并支持本地筛选。
- Agent 的 systemd 单元保留 `CAP_SYS_PTRACE`，当前用途仅是让可信 `ss` 适配器取得 socket 的程序名
  和 PID；Web 与 Agent 不提供通用 ptrace、进程内存或任意命令入口。内核仍未返回归属时，页面降级为
  “未知程序”分组并保留原始技术详情，不伪造进程身份。
- 限流累计值继续只统计 `/proc/net/dev` 中 eth/ens/enp/eno 接口，从本次开机开始累计；接收或发送
  任一值达到阈值时执行 `shutdown -h now`。每月重置日会在 01:00 重启主机，以重置开机累计值。
- 脚本内置生成 `/root/Limiting_Shut_down.sh`，不再下载浮动远程模板；root crontab 使用
  `# kejilion traffic shutdown start/end` 托管区块。启用、更新和停用只修改该脚本、该标记区块及
  可精确识别的旧限流调用，不删除其他 `reboot` 定时任务。
- 写入与 Hosts、Cron、防火墙共用安全锁；脚本在锁内复核资源版本、备份脚本和完整 root crontab，
  写后回读，失败时恢复，回滚失败时把 root-only 快照保存在宿主可见恢复目录。
- 页面在启用前明确提示“达到任一阈值会关机”和“重置日会重启”，停用前说明不会删除其他
  `reboot` 项。不设置自定义确认词，也不增加默认路由、面板连接或阈值大小的业务阻止条件。
- 2026-08-11 已在隔离的 Ubuntu 24.04 root Linux 环境完成真实 `ss`、启用/更新/停用、保留无关
  crontab、可审计关机/重启替身、版本/锁冲突、失败回滚及原状恢复的 Shell→Agent→Panel L2 闭环；
  端口归属另外使用与生产完全一致的候选 systemd 单元复核，禁止用脱离单元的 root 进程替代。

## 多发行版维护

- 优先读取 `/etc/os-release` 的 `ID` 和 `ID_LIKE` 匹配 Debian/Ubuntu、
  RHEL/Fedora、Alpine、Arch/Manjaro 和 openSUSE/SLES；未知发行版在本机
  存在已实现原生工具时按工具能力运行。
- 已实现执行器为 APT/dpkg、DNF/DNF5/YUM、APK、Pacman 和 Zypper；Web 只能提交
  `full`、`cache` 或 `standard` 枚举，不能指定命令、包名、仓库或参数。
- 启动后台任务前确认当前 Agent 绝对路径、软件包管理器、固定步骤命令和
  `systemd-run`；软件源可位于发行版自定义目录，由原生包管理器进行最终校验。
- `journalctl` 是标准清理的可选步骤；缺失时继续完成无用依赖和软件包缓存清理，
  不会让整个清理能力失效。
- RHEL 系识别 `/etc/yum.repos.d/*.repo`，Arch 识别
  `/etc/pacman.d/mirrorlist`，openSUSE/SLES 识别
  `/etc/zypp/repos.d/*.repo`。
- 软件源切换当前只实现 Debian/Ubuntu。RPM、Pacman、Zypper 的换源适配器仍需补齐。
- APK 更新与缓存清理已有固定命令实现，但标准 Alpine/OpenRC 尚不能运行当前
  systemd Agent 安装方式，因此不属于正式部署目标。
