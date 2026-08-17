# KPanel 外联配置来源登记

本文件是 [`PROJECT_RULES.md`](../PROJECT_RULES.md)“外联配置直接复用硬规则”的强制登记表。
“目录相同”“脚本可发现”或“配置语义相似”不等于合规。状态只能使用：

- **已合规**：直接调用脚本入口、消费脚本同一权威模板，或双方调用同一共享生成器，并有双端证据；
- **不合规/冻结**：KPanel 仍维护自编模板或独立流程，禁止继续扩展，必须先迁移；
- **待审计**：尚未完成脚本来源与实际写入链路核对，不得据此宣称完全对齐。

## 当前登记

| ID | 业务与 KPanel 入口 | `kejilion.sh` 权威来源 | 当前方式 | 状态与发布要求 |
| --- | --- | --- | --- | --- |
| `website-nginx-create` | 静态站、PHP、域名反代、负载均衡、跳转的新建入口 | `k static-site`、`k php-site`、`k domain-proxy`、`k loadbalance-site`、`k redirect-site`；分别固定映射 `k web` 的 30、20、24、28、22 | 面板只提交首个域名；后台 PTY 直接执行可信脚本，源码、入口路径、上游和跳转目标继续由脚本原生交互询问 | **已合规（代码链路）**：关闭窗口不终止任务；完成状态以脚本凭据、Nginx 配置和真实产物对账共同确认，发布前仍需目标机逐模板实测 |
| `website-nginx-edit` | 已有静态站、PHP、域名反代、负载均衡、跳转的结构化编辑；`internal/sites/managed_template.go` | `k web` 与脚本官方模板 | KPanel 历史 `renderManagedConfig()` 自行拼接，仅保留旧站兼容维护 | **不合规/冻结**：本次不扩展；后续须迁移到脚本同源编辑协议 |
| `reverse-proxy-ip-port` | IP+端口反向代理；网站页热门入口 | `k fd <domain> <host> <port>`；`ldnmp_Proxy` 与 `reverse-proxy-backend.conf` | Go 后台 PTY 任务直接执行本机可信脚本命令，域名和固定上游参数由面板传入，其余提示可交互输入；完成后发现 `/home/web` 产物 | **已合规（代码链路）**：发布前仍需目标机实测创建、脚本管理、面板管理与删除 |
| `wordpress-flow` | WordPress；网站页热门入口 | `k wp <domain>`；`ldnmp_wp`、LDNMP、证书、数据库、`wordpress.com.conf` 和脚本源码地址 | Go 后台 PTY 任务直接执行本机可信脚本命令，KPanel 不再先进入 `k web` 菜单，也不维护第二套 WordPress 安装器 | **已合规（代码链路）**：发布前仍需目标机实测创建、脚本管理、面板管理与删除 |
| `website-recipes` | Discuz、KodBox、MacCMS、独角数卡、Flarum、Typecho、LinkStack、AI Prompt、Bitwarden、Halo | `k discuz <domain>`、`k bitwarden-site <domain>`、`k halo-site <domain>` 等固定直达命令 | 后台 PTY 直接执行脚本命令并读取 `KPANEL_PROGRESS`；窗口关闭后任务继续，页面可恢复终端 | **已合规（代码链路）**：发布前仍需按目标脚本版本做实机闭环 |
| `application-market` | 应用安装、更新、卸载、域名与访问控制 | `/root/apps/*.conf`、`https://app.kejilion.sh/` 动态应用目录及脚本非交互协议 | 内置与第三方复用同一目录和脚本任务链路；新应用图标只从目录声明的同源 `icons/<slug>.webp` 有界抓取并缓存，不引入安装命令或第二套业务配置 | **待审计**：图标来源、格式、大小、尺寸、并发、缓存和失败回退已受限；仍须逐应用登记安装入口与来源后才能宣称完全对齐 |
| `dockerhub-update-check` | 左侧 KPanel 更新提醒、应用市场镜像更新检测 | Docker Engine `/distribution/{image}/json`；CN/HK 官方 Docker Hub 查询失败时，依次尝试 `docker.1ms.run/<image>` 与 `gh.kejilion.pro/<image>` | 只对严格解析后的 Docker Hub 公共镜像执行失败回退；官方成功、已知非 CN/HK、私有 Registry 和 digest 固定镜像不走代理；远端结果必须是合法 `sha256` 摘要 | **已合规（代码链路，待 L3）**：不增加后台轮询或第二套更新状态；实际拉取、健康检查和回滚仍使用现有应用任务；发布前须实测 KPanel、双段仓库及 `library/*` 镜像摘要一致性、超时切换和海外不回退 |
| `system-dns` | 概览页 DNS 设置 | `set_dns` 与 `kpanel_set_dns_noninteractive` | Go 仅校验结构化 IP 并调用本机可信 `kejilion.sh dns`；最终配置、后端选择和回滚由脚本负责 | **已合规（代码链路）**：发布前仍需在 systemd-resolved、静态文件和受网络管理器接管的主机完成双端实机闭环 |
| `system-ssh-port` | 概览页 SSH 端口 | `new_ssh_port` 与 `kpanel_ssh_port_noninteractive` | Agent 仅校验结构化端口、备份当前 SSH 配置并调用本机可信 `kejilion.sh ssh-port`；适配层直接复用脚本原有主业务并返回固定结果标记 | **待审计**：代码链路已迁移；发布前必须固定包含该协议的脚本提交与摘要，并完成目标发行版及云安全组实机闭环 |
| `diagnostic-scripts` | 体检页 IP、线路、性能与综合测试 | `linux_test`、`kpanel_test_catalog`、`kpanel_run_remote_bash` 与 `kpanel_run_test_noninteractive` | Agent 从可信脚本读取固定目录，以 PTY 执行 `KJ_TEST_NONINTERACTIVE=1 k test run <selector>`；固定来源先下载再执行，保留 stdin 与 ANSI 颜色 | **已合规（代码链路）**：目录、拒绝未知 selector、终端偏移、输入保护、后台日志和失败状态已自动验证；各第三方来源的完整实机跑分需在目标服务器按需验收 |
| `managed-script-runtime` | SSH 端口、SSH 防御、DNS、BBRv3、应用、建站、体检、LDNMP 环境管理、系统资源、一条龙系统调优与轻量节点安装共同使用的宿主机脚本入口 | `kejilion/sh@f34e6d0b9c40c7927e1ccba69cf8b1d6d8bba74c`；SHA-256 `91203ab6ea86769961427a0791e20e9b5827cb498939101cdd618413daff5288` | 镜像构建时按提交和摘要下载到 `/release/kejilion.sh`；安装/更新后以 root:root、0700 保存到 `/home/docker/kpanel/bin/kejilion.sh`，只继承既有可信脚本已明确接受的许可及区域、统计设置。应用市场仍优先使用受管脚本；仅当它不包含动态新增的内置 selector 时，才使用通过 root 所有权、写权限、协议和 selector 校验的宿主机 `/usr/local/bin/k`。第三方应用继续由受管脚本刷新动态配置；其他固定机器协议不变 | **已合规（代码链路）**：安装、旧版升级、摘要拒绝和回滚由生命周期测试覆盖；一条龙调优分离原生日志与机器回执，Swap 按 `/swapfile` 文件、激活状态和 `fstab` 回读，自动 DNS 直接复用既有固定 DNS 协议；发布时必须先发布脚本提交，再构建 KPanel，并复核 OCI 标签与镜像内摘要 |
| `ldnmp-environment` | 网站 → 环境管理 | `kejilion.sh` 的 `k web env` 固定协议；安装、Fail2Ban/WAF/Cloudflare/DDoS、优化模板、镜像更新、`/home/web_*.tar.gz` 备份与卸载语义 | Agent 只调用固定动作并读取真实产物；所有远程模板继续使用脚本内 `gh_proxy` 与原上游地址，KPanel 不增加替代下载源 | **已合规（代码链路）**：Shell/Go/前端测试覆盖协议、枚举、任务凭据、资源版本、备份路径与敏感输入；发布前必须完成 LDNMP 实机矩阵 |
| `system-bbrv3` | 概览 → 网络工具 → BBRv3 管理 | `k bbrv3` 内的 XanMod 仓库、PSABI 包选择、安装/更新/卸载和 `bbr_on`；KPanel 固定入口为 `KJ_BBRV3_NONINTERACTIVE=1 k bbrv3 <action>` | Agent 仅接受 `status/install/update/uninstall` 固定动作并复用系统维护队列；面板不拼接包名、URL 或 Shell，不自动重启 | **已合规（代码链路）**：脚本 smoke、严格 JSON 解析、固定参数任务、并发锁和前端状态已覆盖；发布前仍须在 Debian 12 / Ubuntu 24 的 x86_64 主机完成安装、重启、更新与卸载闭环 |
| `system-resource-adapters` | 概览 → 基础系统设置/网络工具 → 本地 Hosts、定时任务、网卡、防火墙 | `KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1 k kpanel system-resource <resource> <action>` 与 `KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="3"`；Hosts、root Crontab、`/sys/class/net`、iptables 与脚本既有持久化语义 | Agent 有界读取真实状态，只把固定结构化动作交给可信脚本；Cron 命令正文通过有界 stdin 帧传输；防火墙版本忽略动态生成时间与计数器；共享锁位于经过 root owner、权限与 symlink 校验的独立目录 `/run/kejilion-system-resource`；脚本负责锁内资源版本复核、写入、持久化、回读和失败回滚 | **已合规（154 L2）**：2026-08-10 在隔离的 `0.59.0-l2` Panel/Agent 实例完成 Hosts、root Crontab、dummy 网卡和保留地址防火墙规则的 Shell↔Agent↔Panel 写入、回读与恢复；生产 `0.58.0` 未替换，测试后脚本、配置和临时资源均恢复 |
| `network-operations-adapters` | 概览 → 网络工具 → 端口占用查看、限流自动关机 | `KJ_NETWORK_OPERATIONS_NONINTERACTIVE=1 k kpanel network-operations <port-usage|traffic-shutdown> <action>` 与 `KPANEL_NETWORK_OPERATIONS_PROTOCOL_VERSION="1"`；端口事实来自固定 `ss -H -lntup`，累计流量沿用脚本对 `/proc/net/dev` 的 eth/ens/enp/eno 统计目的 | Agent 只解析脚本的有界机器回执；限流脚本由 `kejilion.sh` 内置生成，写入与 root crontab 共用 `/run/kejilion-system-resource` 安全锁、资源版本和事务回滚；cron 只维护带标记的自身区块，不删除无法归属的其他 `reboot` 项 | **已合规（隔离 L2）**：固定脚本提交/摘要已发布；2026-08-11 在隔离 Ubuntu 24.04 root Linux 完成真实 `ss`、启用/更新/停用、保留无关 crontab、可审计关机/重启替身、版本/锁冲突、失败回滚与原状恢复的 Shell→Agent→Panel 闭环；生产只做只读状态验收 |
| `account-management-adapter` | 概览 → 基础系统设置 → 账户管理 | `KJ_ACCOUNT_MANAGEMENT_NONINTERACTIVE=1 k kpanel account-management <action>` 与 `KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION="1"`；账户事实来自 `/etc/passwd`、`/etc/group`、`/etc/shadow`、用户 `authorized_keys`、sudo/wheel 与 `sshd -T` 有效配置 | Agent 只提交创建账户、修改密码、公钥增删、角色、SSH 策略、禁用 Root、Root 安全迁移和删除账户的固定动作；密码和公钥正文使用有界单行 stdin，不进入 argv、回执或审计；脚本共用 `/run/kejilion-system-resource` 写锁、资源版本、sshd 语法校验和失败回滚 | **已合规（隔离 L2）**：固定脚本提交/摘要已发布；2026-08-11 在隔离 Ubuntu 24.04 root Linux 完成密码/密钥账户、Root 密码、三种角色、公钥增删、SSH 策略、Root 安全迁移、删除、版本/锁冲突、失败回滚、`rollback-failed`、`needs-attention` 与原状恢复的 Shell→Agent→Panel 闭环；生产不执行危险写验收 |
| `ssh-defense-manager-adapter` | 概览 → 基础系统设置 → SSH 防御 | `KJ_F2B_NONINTERACTIVE=1 k f2b manager <action>` 与 `KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"`；状态来自 Fail2Ban 服务、SSH jail、有效策略、封禁列表、信任地址和 Fail2Ban 日志 | Agent 只提交温和/标准/严格策略、信任地址增删、单个/全部解封及启停/卸载固定动作；脚本负责安全锁、配置备份、`fail2ban-client -t`、reload、回读和失败回滚；启停/卸载继续走持久化维护任务 | **已合规（154 隔离 L2）**：2026-08-11 在 Ubuntu 24.04 root 容器内使用真实 Fail2Ban 完成 Shell→Agent→Panel 信任地址增删、真实 ban/unban、严格↔标准策略往返、审计和失败回滚；浏览器验证桌面/390px、加载不消失和 0 页面错误；测试容器、目录和隧道均已清理，生产 Panel/Agent 状态未变 |
| `system-tuning-adapter` | 系统中心 → 性能优化 → 一条龙系统调优（概览保留快捷入口） | `KJ_SYSTEM_TUNING_NONINTERACTIVE=1 k kpanel system-tuning <status|apply-item>` 与 `KPANEL_SYSTEM_TUNING_PROTOCOL_VERSION="1"`；固定 12 个项目沿用脚本菜单 66 的原始业务顺序 | KPanel 只提交 12 个固定项目 ID；Agent 以持久化维护任务逐项调用脚本、回读收据并在首个失败项停止。LinuxMirrors 固定 `SuperManito/LinuxMirrors@649e948763042e485e411be540d21c32cface1c1` 与 SHA-256 `2e3b78a460f10ef291f30e3cbf3d3b28a9521d6615364f11b36e4a70ec97d18d`；网络参数脚本固定 `kejilion/sh@e9c3078eb516b05f9df6d2a9294cf3b226ca02bd` 与 SHA-256 `94f86598805b7a8155f444f35a446df4657985ef81b25f96f7799aa465033bbb` | **已合规（154 隔离 L2）**：2026-08-11 在 Ubuntu 24.04 systemd 容器完成固定 12 项 Shell 状态、Panel typed 选择、Agent 后台进度恢复、时区安全项成功、首项故障即停止、后续项未执行、409 版本冲突和审计闭环；Swap、SSH 与全开放防火墙等危险项未在生产宿主执行。证据位于 `/root/kpanel-release-evidence/system-tuning-6dcfcf7` |
| `system-network` | 系统更新源、V4/V6、内核、BBR | `kejilion.sh` 系统工具对应函数和远程配置 | 多个 Go 适配器独立执行 | **待审计**：凡脚本已有外联模板/远程来源的项目必须迁移为同源；更新源仍不得以当前 Go 自编流程宣称完全对齐 |
| `docker-environment` | Docker 安装、换源、维护、迁移、备份与还原 | `kejilion.sh` Docker 工具函数及其远程来源 | KPanel 固定动作适配器 | **待审计**：逐动作核对，不得新增自编外联配置 |
| `cluster-light-node-runtime` | 集群 → 添加主机 → 非面板 Linux 主机 | `bash <(curl -fsSL https://kejilion.sh) kpanel node join <授权>`；官方短入口按区域加载 `kejilion/sh` 的根目录或 `cn/kejilion.sh`，二者保持同一节点协议；二进制与 `SHA256SUMS` 来自 `https://github.com/kejilion/KPanel/releases/latest/download/` | `kejilion.sh` 固定安装协议按架构下载静态 `kejilion-node`，严格校验 Release 摘要与 `version` 后原子安装；systemd timer 使用同一更新器自动更新并在健康失败时回滚 | **已合规（待 L3）**：不要求 Docker/Go；脚本入口仅使用 HTTPS 官方域名，Release 资产校验不变；正式发布须验证短入口、Release 资产、摘要、安装、断网重试、更新回滚与卸载 |
| `monitoring-operator-latency` | 历史监控 → 三网延迟 | KPanel 原生只读监控；`kejilion.sh` 无同类历史业务。运营商网段归属离线复核自 `gaoyifan/china-operator-ip@4593b6c4d577b61e3c2189bcd06f1e4c24750b7d`，固定目标清单见 `docs/history-monitoring-design.md` | Agent 每 5 分钟对代码内固定九个运营商 DNS 地址执行有界 UDP/53 往返探测；运行时不下载地址列表，不接受 API 自定义目标 | **已合规（代码链路，待 L3）**：固定九目标、3 并发、1.5 秒超时、缺测不记 0、无新增 capability 已自动验证；发布前须在境内外真实 Linux 主机复核目标可达率与历史曲线 |

<!-- external-config-debt:website-nginx:blocked -->

## KPanel 与 kejilion.sh 发布关系

- KPanel 与 `kejilion.sh` 按协议兼容，不要求版本号同步。
- KPanel 仅修改前端、Go 服务或不涉及脚本协议的功能时，继续固定上一份已验证脚本；
  不提交或发布新的 `kejilion.sh`。
- KPanel 新增或修改脚本协议时，先发布 `kejilion.sh`，再在 KPanel 固定脚本提交和
  SHA-256，并完成双端协议测试。
- `kejilion/apps/kpanel.conf` 不跟随普通 KPanel 或脚本版本发布；它从镜像 OCI 标签、
  `/release/VERSION` 和产物本身读取并交叉验证版本、源码提交、脚本提交及脚本摘要。
- 只有应用安装协议、镜像产物路径或 OCI 契约发生变化时，才修改应用市场配置。

### KPanel 应用市场自更新约束

- 用户入口固定为“应用市场 → KPanel → 更新”；底层增强不得另建第二套更新流程或改变原有确认、
  后台运行和任务日志体验。
- 更新必须把 Panel、Agent 与 KPanel 专用 `kejilion.sh` 作为同一事务校验和切换；宿主机不需要
  Go、Node.js 或其他编译环境，也不得覆盖系统的 `k` 命令。
- 更新前必须从正在运行的容器读取并校验当前镜像 ID 和宿主机端口；更新后继续保留原端口、
  `.env`、Panel 数据、应用、站点、域名和 `/home/web`。
- 新版本健康检查失败时，必须将 `latest` 本地标签恢复到更新前的精确镜像 ID，并同步恢复旧
  Agent、脚本、Compose、systemd unit 与 Agent 环境配置后重新验收。
- Panel 或 Agent 切换造成的短暂 API 不可用属于可重试状态；前端不得因此清除后台任务 ID。
  只有任务明确结束或服务确认返回任务不存在时，才结束任务跟踪。
- 发布门禁必须包含自定义端口保持、元数据/版本/脚本摘要拒绝、更新失败镜像回滚、任务重连和
  原安装/卸载体验不变的自动测试；正式发布仍需在真实 Docker 主机执行升级与回滚闭环。

## 每项迁移完成的证据

1. 脚本菜单/函数、模板 URL 或共享生成器的准确版本与 SHA-256；
2. KPanel 调用链证明没有内置第二份业务模板；
3. 相同输入下去除时间戳、随机凭证等易变字段后的有效配置对比；
4. 脚本创建 → KPanel 管理，以及 KPanel 创建 → 脚本管理的实机记录；
5. 更新、删除、失败回滚和脚本来源升级后的兼容测试。
