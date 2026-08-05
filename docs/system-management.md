# 系统监控与管理

KPanel 的系统管理业务以 `kejilion.sh` 为功能基准，但不把整个 Shell
脚本暴露给 Web。Agent 只提供固定输入、固定输出的类型化接口；Panel
负责登录、CSRF、Origin 校验和审计，不能传入任意命令。

## 状态一致性

- 首页状态直接读取宿主机当前配置，不以 KPanel 数据库代替系统事实。
- `kejilion.sh` 或人工修改产生的 SSH、DNS、时区、Swap、镜像源、
  `gai.conf`、内核优化和 BBR 状态，刷新后应被 Agent 重新识别。
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
| SSH 端口 | `sshd_config` 与片段 | 调用本机可信 `kejilion.sh ssh-port` 非交互适配入口；脚本复用原有 `new_ssh_port` 主业务，Agent 负责结构化校验、执行前备份和结果回读 |
| SSH 防御 | `k f2b status|enable|disable` | 读取真实 Fail2Ban SSH jail；开启由后台任务安装并验证，关闭仅停用服务与自启、保留配置 |
| DNS | `resolv.conf`、systemd-resolved 与脚本 DNS 协议 | 调用本机可信 `kejilion.sh` 固定非交互入口；systemd-resolved 使用原生配置，其他管理器沿用脚本的 `resolv.conf` 写入与锁定语义 |
| 时区 | `/etc/timezone` 或 `localtime` | 有效 IANA 时区名称、回读 |
| 虚拟内存 | `/proc/meminfo`、`/proc/swaps`、`/swapfile` | 与 `kejilion.sh` 共用 `/swapfile`；合并旧版 KPanel Swap，不清除现有分区或第三方 swapfile |
| 系统镜像源 | APT/RPM/APK/Pacman/Zypper 源地址 | Debian/Ubuntu 对齐脚本四种区域模式；第三方源不修改，隔离 `apt-get update` 失败回滚 |
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
BBR、BBRv3，以及普通确认的服务器重启。重装系统、其他发行版换源和通用
sysctl 编辑目前缺少适配器；这不是永久产品限制。

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
