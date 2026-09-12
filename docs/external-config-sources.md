# KPanel 外联配置来源登记

本文件是 [`PROJECT_RULES.md`](../PROJECT_RULES.md)“外联配置直接复用硬规则”的强制登记表。
“目录相同”“脚本可发现”或“配置语义相似”不等于合规。状态只能使用：

- **已合规**：直接调用脚本入口、消费脚本同一权威模板，或双方调用同一共享生成器，并有双端证据；
- **不合规/冻结**：KPanel 仍维护自编模板或独立流程，禁止继续扩展，必须先迁移；
- **待审计**：尚未完成脚本来源与实际写入链路核对，不得据此宣称完全对齐。

## 当前登记

| ID | 业务与 KPanel 入口 | `kejilion.sh` 权威来源 | 当前方式 | 状态与发布要求 |
| --- | --- | --- | --- | --- |
| `website-http-port` | 新建站点输入 `域名:端口` 或 `IPv4:端口` | `k http-site <端口> <原建站命令> <主机> ...`；`KPANEL_WEB_HTTP_PROTOCOL_VERSION="1"` | Agent 分离主机与端口并保留后台任务恢复；脚本在独立子进程复用原菜单、模板与环境锁，跳过证书，将本站模板转换为指定 HTTP 监听；复用 `open_port` 放行。外部 `.conf` 只读取各 server 块监听并生成访问地址，不自动改写 | **已合规（代码与隔离测试，完整双端实机待验收）**：`scriptLinkageState=coupled`，变更集 `site-http-port-20260912`。配套组合脚本已先发布到 `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`，根脚本 SHA-256 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048`，CN 脚本 SHA-256 `62b01b5b1ba736fafe1a167733d64eaba64606f9c5a4b77fa1cd136adbf843cc`；KPanel 候选固定该来源。旧脚本拒绝新协议，普通域名沿用原入口。目录/删除仍使用不带端口的主机标识，同主机保留原有重复检查。原流程执行前检查端口占用，重载后确认实际监听；host 网络需要 `ss`，桥接容器需先具备相同端口映射。失败移除本次新配置并尝试恢复原运行中的 Nginx，源码/数据库保留供恢复。 |
| `website-nginx-create` | 静态站、PHP、域名反代、负载均衡、跳转的新建入口 | `k static-site`、`k php-site`、`k domain-proxy`、`k loadbalance-site`、`k redirect-site`；分别固定映射 `k web` 的 30、20、24、28、22 | 面板提交首个域名；重定向额外将目标域名作为独立 argv 传给 `k redirect-site <原域名> <目标域名>`，要求脚本 `KPANEL_WEB_REDIRECT_PROTOCOL_VERSION="1"`，沿用官方 `rewrite.conf` 的 301 HTTPS 跳转及路径/查询参数；其他模板细节仍由脚本原生交互询问。自定义证书继续通过一次性文件协议提供 | **已合规（代码链路，重定向传参待本轮隔离发布验收）**：`scriptLinkageState=coupled`，变更集 `redirect-script-inputs`；当前内置脚本基线仍为下文固定提交，发布前必须换成已发布且通过同步/smoke 的配套脚本提交和摘要。旧版域名单参数交互调用继续兼容；新传参协议缺失时明确失败。证书复用、自动签发和自定义材料仍由脚本处理，失败不继续生成/加载配置，关闭窗口不终止后台任务；不以本地测试代替真实 ACME、Nginx 和双端管理验收 |
| `website-delete` | 网站管理删除站点、应用市场解绑域名 | `k web del <域名>`；`web_del()` 与 `KPANEL_DELETE_SITE` / `KPANEL_DELETE_DATABASE` 机器回执 | 两个入口只提交实际站点 ID 与规范化主域名；Agent 核对身份后通过受限 systemd 单元固定调用可信脚本，并在返回后复核站点目录、Nginx 配置和证书均已移除 | **已合规（代码链路，待隔离 L2）**：不保留仅删 Nginx 的第二路径；数据库失败按脚本回执报告部分成功，正式发布前须在隔离主机验证静态站、PHP 站和应用反代域名闭环 |
| `website-nginx-edit` | 已有静态站、PHP、域名反代、负载均衡、跳转的结构化编辑；`internal/sites/managed_template.go` | `k web` 与脚本官方模板 | KPanel 历史 `renderManagedConfig()` 自行拼接，仅保留旧站兼容维护 | **不合规/冻结**：本次不扩展；后续须迁移到脚本同源编辑协议 |
| `reverse-proxy-ip-port` | IP+端口反向代理；网站页热门入口 | `k fd <domain> <host> <port>`；`ldnmp_Proxy` 与 `reverse-proxy-backend.conf` | Go 后台 PTY 任务直接执行本机可信脚本命令，域名、固定上游参数和可选自定义证书由面板传入，其余提示可交互输入；完成后发现 `/home/web` 产物 | **已合规（代码链路）**：发布前仍需目标机实测创建、脚本管理、面板管理与删除 |
| `wordpress-flow` | WordPress；网站页热门入口 | `k wp <domain>`；`ldnmp_wp`、LDNMP、证书、数据库、`wordpress.com.conf` 和脚本源码地址 | Go 后台 PTY 任务直接执行本机可信脚本命令，域名和可选自定义证书由面板传入；KPanel 不再先进入 `k web` 菜单，也不维护第二套 WordPress 安装器 | **已合规（代码链路）**：发布前仍需目标机实测创建、脚本管理、面板管理与删除 |
| `website-recipes` | Discuz、KodBox、MacCMS、独角数卡、Flarum、Typecho、LinkStack、AI Prompt、Bitwarden、Halo | `k discuz <domain>`、`k bitwarden-site <domain>`、`k halo-site <domain>` 等固定直达命令 | 后台 PTY 直接执行脚本命令并读取 `KPANEL_PROGRESS`；域名和可选自定义证书由面板传入，窗口关闭后任务继续，页面可恢复终端 | **已合规（代码链路）**：发布前仍需按目标脚本版本做实机闭环 |
| `application-market` | 应用安装、更新、卸载、域名与访问控制 | `/root/apps/*.conf`、`https://app.kejilion.sh/` 动态应用目录及脚本非交互协议 | 内置与第三方复用同一目录和脚本任务链路；新应用图标只从目录声明的同源 `icons/<slug>.webp` 有界抓取并缓存，不引入安装命令或第二套业务配置 | **待审计**：图标来源、格式、大小、尺寸、并发、缓存和失败回退已受限；仍须逐应用登记安装入口与来源后才能宣称完全对齐 |
| `dockerhub-update-check` | 左侧 KPanel 更新提醒、应用市场镜像更新检测 | Docker Engine `/distribution/{image}/json`；CN/HK 官方 Docker Hub 查询失败时，依次尝试 `docker.1ms.run/<image>` 与 `gh.kejilion.pro/<image>` | 只对严格解析后的 Docker Hub 公共镜像执行失败回退；官方成功、已知非 CN/HK、私有 Registry 和 digest 固定镜像不走代理；远端结果必须是合法 `sha256` 摘要 | **已合规（代码链路，待 L3）**：不增加后台轮询或第二套更新状态；实际拉取、健康检查和回滚仍使用现有应用任务；发布前须实测 KPanel、双段仓库及 `library/*` 镜像摘要一致性、超时切换和海外不回退 |
| `system-dns` | 概览页 DNS 设置 | `set_dns` 与 `kpanel_set_dns_noninteractive` | Go 仅校验结构化 IP 并调用本机可信 `kejilion.sh dns`；最终配置、后端选择和回滚由脚本负责 | **已合规（代码链路）**：发布前仍需在 systemd-resolved、静态文件和受网络管理器接管的主机完成双端实机闭环 |
| `system-ssh-port` | 概览页 SSH 端口 | `new_ssh_port` 与 `kpanel_ssh_port_noninteractive` | Agent 仅校验结构化端口、备份当前 SSH 配置并调用本机可信 `kejilion.sh ssh-port`；适配层直接复用脚本原有主业务并返回固定结果标记 | **待审计**：代码链路已迁移；发布前必须固定包含该协议的脚本提交与摘要，并完成目标发行版及云安全组实机闭环 |
| `diagnostic-scripts` | 体检页第三方增值测试：IP、线路、性能与报告型综合测试 | `linux_test`、`kpanel_test_catalog`、`kpanel_run_remote_bash` 与 `kpanel_run_test_noninteractive` | KPanel 核心体检由 Agent 原生探针提供；若存在可信脚本，Agent 追加读取固定目录，以 PTY 执行 `KJ_TEST_NONINTERACTIVE=1 k test run <selector>`，固定来源先下载再执行，保留 stdin 与 ANSI 颜色 | **已合规（代码链路）**：原生目录不依赖脚本；脚本目录、拒绝未知 selector、终端偏移、输入保护、后台日志和失败状态已自动验证；各第三方来源的完整实机跑分需在目标服务器按需验收 |
| `managed-script-runtime` | SSH 端口、SSH 防御、DNS、BBRv3、应用、建站、体检、LDNMP 环境管理、系统资源、一条龙系统调优、磁盘与分区管理及轻量节点安装共同使用的宿主机脚本入口 | `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`；SHA-256 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048` | 镜像构建时按提交和摘要下载到 `/release/kejilion.sh`；安装/更新后以 root:root、0700 保存到 `/home/docker/kpanel/bin/kejilion.sh`，只继承既有可信脚本已明确接受的许可及区域、统计设置。应用市场仍优先使用受管脚本；仅当它不包含动态新增的内置 selector 时，才使用通过 root 所有权、写权限、协议和 selector 校验的宿主机 `/usr/local/bin/k`。并行原生脚本任务通过脚本并发协议保护共享的软件包、防火墙、应用目录和安装标记；磁盘写入仅在脚本包含 `KPANEL_DISK_MANAGEMENT_PROTOCOL_VERSION="1"` 时启用；第三方应用继续由受管脚本刷新动态配置；其他固定机器协议不变 | **已合规（代码链路）**：安装、旧版升级、摘要拒绝和回滚由生命周期测试覆盖；并发协议的锁释放、嵌套调用、失败传播、共享写入互斥和路径拒绝由脚本测试覆盖；磁盘协议只接收固定动作、可信 `MAJ:MIN` 与有界参数，事务、回读和恢复凭据由脚本负责；一条龙调优分离原生日志与机器回执，Swap 按 `/swapfile` 文件、激活状态和 `fstab` 回读，自动 DNS 直接复用既有固定 DNS 协议，新机 SSH 项只在一条龙中补齐 OpenSSH Server 和 systemd socket/service 前置条件；一条龙联网下载及其外部脚本执行显式清除代理环境并保持直连，避免宿主机代理变量污染特权调优；轻量节点新增 SSH 登录通知和文件代理的 systemd 生命周期、兼容回执与卸载清理；重新接入用 token 指纹和 root-only 暂存区原子切换身份，失败时保留旧连接，成功后按新 token 与指定名称完成上报；本组合新增 HTTP 站点端口协议和备份中心协议；发布时必须先发布脚本提交，再构建 KPanel，并复核 OCI 标签与镜像内摘要 |
| `ldnmp-environment` | 网站 → 环境管理 | `kejilion.sh` 的 `k web env` 固定协议；安装、Fail2Ban/WAF/Cloudflare/DDoS、优化模板、镜像更新、`/home/web_*.tar.gz` 备份与卸载语义 | Agent 只调用固定动作并读取真实产物；所有远程模板继续使用脚本内 `gh_proxy` 与原上游地址，KPanel 不增加替代下载源 | **已合规（代码链路）**：Shell/Go/前端测试覆盖协议、枚举、任务凭据、资源版本、备份路径与敏感输入；发布前必须完成 LDNMP 实机矩阵 |
| `system-bbrv3` | 概览 → 网络工具 → BBRv3 管理 | `k bbrv3` 内的 XanMod 仓库、PSABI 包选择、安装/更新/卸载和 `bbr_on`；KPanel 固定入口为 `KJ_BBRV3_NONINTERACTIVE=1 k bbrv3 <action>` | Agent 仅接受 `status/install/update/uninstall` 固定动作并复用系统维护队列；面板不拼接包名、URL 或 Shell，不自动重启 | **已合规（代码链路）**：脚本 smoke、严格 JSON 解析、固定参数任务、并发锁和前端状态已覆盖；发布前仍须在 Debian 12 / Ubuntu 24 的 x86_64 主机完成安装、重启、更新与卸载闭环 |
| `system-resource-adapters` | 概览 → 基础系统设置/网络工具 → 本地 Hosts、定时任务、网卡、防火墙 | `KJ_SYSTEM_RESOURCE_NONINTERACTIVE=1 k kpanel system-resource <resource> <action>` 与 `KPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION="3"` / `"4"`；Hosts、root Crontab、`/sys/class/net`、iptables/ipset 与脚本既有持久化语义 | Agent 有界读取真实状态，只把固定结构化动作交给可信脚本；Cron 命令正文通过有界 stdin 帧传输；防火墙版本忽略动态生成时间与计数器，v3 仅计算 iptables，v4 在存在 ipset 命令时纳入有界 `ipset save`；国家/地区写入要求 v4，仅支持入站阻止、允许和清除，代码和网段由脚本固定 HTTPS 数据源解析；共享锁位于经过 root owner、权限与 symlink 校验的独立目录 `/run/kejilion-system-resource`；脚本负责锁内资源版本复核、写入、持久化、回读和失败回滚 | **待隔离 L2 验收**：国家/地区动作先完成脚本与 Agent/Panel 双端协议测试，再在 `arena-154` 验证 iptables-nft、ipset、持久化重启和失败回滚；未发布、未合入主线、未改变生产 |
| `network-operations-adapters` | 概览 → 网络工具 → 端口占用查看、限流自动关机 | `KJ_NETWORK_OPERATIONS_NONINTERACTIVE=1 k kpanel network-operations <port-usage|traffic-shutdown> <action>` 与 `KPANEL_NETWORK_OPERATIONS_PROTOCOL_VERSION="1"`；端口事实来自固定 `ss -H -lntup`，累计流量沿用脚本对 `/proc/net/dev` 的 eth/ens/enp/eno 统计目的 | Agent 只解析脚本的有界机器回执；限流脚本由 `kejilion.sh` 内置生成，写入与 root crontab 共用 `/run/kejilion-system-resource` 安全锁、资源版本和事务回滚；cron 只维护带标记的自身区块，不删除无法归属的其他 `reboot` 项 | **已合规（隔离 L2）**：固定脚本提交/摘要已发布；2026-08-11 在隔离 Ubuntu 24.04 root Linux 完成真实 `ss`、启用/更新/停用、保留无关 crontab、可审计关机/重启替身、版本/锁冲突、失败回滚与原状恢复的 Shell→Agent→Panel 闭环；生产只做只读状态验收 |
| `account-management-adapter` | 概览 → 基础系统设置 → 账户管理 | `KJ_ACCOUNT_MANAGEMENT_NONINTERACTIVE=1 k kpanel account-management <action>` 与 `KPANEL_ACCOUNT_MANAGEMENT_PROTOCOL_VERSION="1"`；账户事实来自 `/etc/passwd`、`/etc/group`、`/etc/shadow`、用户 `authorized_keys`、sudo/wheel 与 `sshd -T` 有效配置 | Agent 只提交创建账户、修改密码、公钥增删、角色、SSH 策略、禁用 Root、Root 安全迁移和删除账户的固定动作；密码和公钥正文使用有界单行 stdin，不进入 argv、回执或审计；脚本共用 `/run/kejilion-system-resource` 写锁、资源版本、sshd 语法校验和失败回滚 | **已合规（隔离 L2）**：固定脚本提交/摘要已发布；2026-08-11 在隔离 Ubuntu 24.04 root Linux 完成密码/密钥账户、Root 密码、三种角色、公钥增删、SSH 策略、Root 安全迁移、删除、版本/锁冲突、失败回滚、`rollback-failed`、`needs-attention` 与原状恢复的 Shell→Agent→Panel 闭环；生产不执行危险写验收 |
| `ssh-defense-manager-adapter` | 概览 → 基础系统设置 → SSH 防御 | `KJ_F2B_NONINTERACTIVE=1 k f2b manager <action>` 与 `KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"`；状态来自 Fail2Ban 服务、SSH jail、有效策略、封禁列表、信任地址和 Fail2Ban 日志 | Agent 只提交温和/标准/严格策略、信任地址增删、单个/全部解封及启停/卸载固定动作；脚本负责安全锁、配置备份、`fail2ban-client -t`、reload、回读和失败回滚；启停/卸载继续走持久化维护任务 | **已合规（154 隔离 L2）**：2026-08-11 在 Ubuntu 24.04 root 容器内使用真实 Fail2Ban 完成 Shell→Agent→Panel 信任地址增删、真实 ban/unban、严格↔标准策略往返、审计和失败回滚；浏览器验证桌面/390px、加载不消失和 0 页面错误；测试容器、目录和隧道均已清理，生产 Panel/Agent 状态未变 |
| `system-tuning-adapter` | 系统中心 → 性能优化 → 一条龙系统调优（概览保留快捷入口） | `KJ_SYSTEM_TUNING_NONINTERACTIVE=1 k kpanel system-tuning <status|apply-item>` 与 `KPANEL_SYSTEM_TUNING_PROTOCOL_VERSION="1"`；固定 12 个项目沿用脚本菜单 66 的原始业务顺序 | KPanel 只提交 12 个固定项目 ID；Agent 以持久化维护任务逐项调用脚本、回读收据并在首个失败项停止。LinuxMirrors 固定 `SuperManito/LinuxMirrors@649e948763042e485e411be540d21c32cface1c1` 与 SHA-256 `2e3b78a460f10ef291f30e3cbf3d3b28a9521d6615364f11b36e4a70ec97d18d`；网络参数脚本固定 `kejilion/sh@e9c3078eb516b05f9df6d2a9294cf3b226ca02bd` 与 SHA-256 `94f86598805b7a8155f444f35a446df4657985ef81b25f96f7799aa465033bbb` | **已合规（154 隔离 L2）**：2026-08-11 在 Ubuntu 24.04 systemd 容器完成固定 12 项 Shell 状态、Panel typed 选择、Agent 后台进度恢复、时区安全项成功、首项故障即停止、后续项未执行、409 版本冲突和审计闭环；Swap、SSH 与全开放防火墙等危险项未在生产宿主执行。证据位于 `/root/kpanel-release-evidence/system-tuning-6dcfcf7` |
| `system-network` | 系统更新源、V4/V6、内核、BBR | `kejilion.sh` 系统工具对应函数和远程配置 | 多个 Go 适配器独立执行 | **待审计**：凡脚本已有外联模板/远程来源的项目必须迁移为同源；更新源仍不得以当前 Go 自编流程宣称完全对齐 |
| `backup-center-shared-adapter` | 设置 → 备份与恢复；脚本三个备份菜单及 `k backup-center` | `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04` 的应用 `/home`、LDNMP `/home/web`、Docker inspect/Compose/挂载数据机制；新协议 `backup-center` 1 | 双端调用同一 `internal/hostbackup` Agent 适配器和 `.kpb` 认证加密格式；恢复归档中的实际配置，不生成外联模板；旧格式保留原脚本入口 | **已合规（组合候选待 L3）**：`scriptLinkageState=coupled`，变更集 `backup-center-20260912`；配套组合脚本已先发布，根/CN 同步、Linux 语法、HTTP 端口兼容、备份可信入口及加密包双向互通已通过，镜像固定上述提交与摘要。跨发行版 LDNMP/数据库迁移仍属于发布画像中的未验证边界，范围见 [备份契约](backup-center.md) |
| `docker-environment` | Docker 安装、换源、维护、迁移、备份与还原 | `kejilion.sh` Docker 工具函数及其远程来源 | KPanel 固定动作适配器 | **待审计**：逐动作核对，不得新增自编外联配置 |
| `cluster-light-node-runtime` | 集群 → 添加主机 → 非面板 Linux 主机 | `bash <(curl -fsSL https://kejilion.sh) kpanel node join <授权>`；官方短入口按区域加载 `kejilion/sh` 的根目录或 `cn/kejilion.sh`，二者保持同一节点协议；二进制与 `SHA256SUMS` 来自 `https://github.com/kejilion/KPanel/releases/latest/download/` | `kejilion.sh` 固定安装协议按架构下载静态 `kejilion-node`，严格校验 Release 摘要与 `version` 后原子安装；systemd timer 使用同一更新器自动更新并在健康失败时回滚 | **已合规（待 L3）**：不要求 Docker/Go；脚本入口仅使用 HTTPS 官方域名，Release 资产校验不变；正式发布须验证短入口、Release 资产、摘要、安装、断网重试、更新回滚与卸载 |
| `cluster-light-node-update-migration` | 既有轻量节点自动更新恢复 | `kejilion/sh@9f612efc4f861459c0a525491c7cdf5eda756cf7`（配套脚本主线已发布）；完整脚本 SHA-256 `9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663`；模板摘要见 `cmd/kejilion-node/update_runtime/source.json` | `scripts/sync-light-node-runtime.mjs` 原样组合安装器共用生命周期锁与 updater，并提取 service/timer 并嵌入节点；普通 Go 测试核对固定摘要，配对验收执行该脚本 `PATH_TO_KEJILION_SH --check`。仅已校验 root 临时 Release 的兼容入口可原子更新既有安装，保留 reporting key；官方历史文件 unit 精确匹配后同源迁移，自定义 unit 与 drop-in 保留 | **已实现（本版生命周期与防回退回归通过，最终真实 systemd / L3 待验）**：配置权限、锁/交接、下载失败、回滚、进程版本和命名空间迁移有回归；隔离 Ubuntu 24.04 的真实 systemd 已验证旧更新器首轮配置权限、两轮更新、timer、历史文件 unit 的网络族及失败回滚，下载使用受控夹具，不能代替公开 Release 下载与最终 SHA L3。最终发布按本版验收记录核对配套脚本、节点二进制与受管脚本，不得只更新中心端 |
| `monitoring-operator-latency` | 历史监控 → 三网延迟 | KPanel 原生只读监控；`kejilion.sh` 无同类历史业务。运营商网段归属离线复核自 `gaoyifan/china-operator-ip@4593b6c4d577b61e3c2189bcd06f1e4c24750b7d`，固定目标清单见 `docs/history-monitoring-design.md` | Agent 每 5 分钟对代码内固定九个运营商 DNS 地址执行有界 UDP/53 往返探测；运行时不下载地址列表，不接受 API 自定义目标 | **已合规（代码链路，待 L3）**：固定九目标、3 并发、1.5 秒超时、缺测不记 0、无新增 capability 已自动验证；发布前须在境内外真实 Linux 主机复核目标可达率与历史曲线 |

<!-- external-config-debt:website-nginx:blocked -->

## KPanel 与 kejilion.sh 发布关系

当前应用任务退出码变更集 `kpanel-v1100-native-app-status`：`scriptLinkageState=coupled`。
配对脚本 `9f612efc4f861459c0a525491c7cdf5eda756cf7` 已发布到脚本主线，SHA-256
`9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663`。根/CN 同步、完整应用分发器
失败码与成功码、普通 SSH 菜单兼容回归通过，旧脚本被新增负例拒绝。镜像与节点固定该来源，节点更新
模板字节不变；精确候选的真实 Agent/systemd/Docker 更新失败与恢复验收仍按发布门禁执行。
成对回滚点为 KPanel v1.9.0 与脚本 `aac8bc8710bb559ec7ae1d6988770f64564954ca`。

以下为此前已发布的兼容能力与历史来源；上述当前脚本完整保留它们。

重定向传参及证书兼容变更集 `redirect-script-inputs` / `certificate-replacement-compat-20260909`：`scriptLinkageState=coupled`。
组合脚本 `aac8bc8710bb559ec7ae1d6988770f64564954ca` 已发布到 `kejilion/sh` 主线，原始 Git 字节 SHA-256
`583aff02c2f75510edc5135addad31163a7dfeeb9ac99fcf190b064fcbadedee`；该来源已随 v1.9.0 发布。根/中文同步、重定向传参、原生重试/导入、
证书失败恢复及官方历史续签器迁移 smoke 已通过；完整候选 CI、公开 OCI 与本轮实机验收仍按发布门禁执行。
脚本回滚点为 `298f6f23751e36726660d73b5c0c83aef1b404f4`。

网站证书生命周期固定来源：`kejilion/sh@aac8bc8710bb559ec7ae1d6988770f64564954ca`，
`k web certificate-replace` 与创建证书协议共用 `/home/web/certs`；不生成 Nginx 模板。
同源 `auto_cert_renewal.sh` SHA-256 为
`3a63b9e0c1557fae9e18983a8eee284476a946a811c11b3076535aab1432e0d0`。
当前固定脚本仅将精确官方旧续签器 `ffc714440b503d5f8ee082006f31cc0b59b1c3fd8a1d015257a6cf5944153e83`
升级为该字节级一致的受保护版本，保留未知本地改动；来源标记只决定自动续签策略。
兼容修复 `certificate-replacement-compat-20260909`（`coupled`，配套脚本主线已发布）增加对官方
`b3d0d35` 和 `014f450` 续签器的识别，迁移结果仍须等于上述当前摘要；不改写未知自定义脚本。
Agent 将固定回执 `renewal_adapter_unavailable` 映射为 `site_certificate_renewal_unavailable`，
兼容迁移在内部自动完成，不新增用户步骤或迁移提示；前端仅在操作失败时提示稍后重试，不展示续签适配细节或要求用户检查脚本。
当前受管脚本保留上述公开组合提交的证书逻辑，完整验收见对应版本发布记录。
证书生命周期已随历史版本发布；当前修复继续采用 `coupled` 和同源脚本协议。
本地 PEM、事务、旧续签器迁移和失败恢复夹具已验证；真实 TLS、cron/systemd、双端互通和公开产物 L3 未验证。

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
