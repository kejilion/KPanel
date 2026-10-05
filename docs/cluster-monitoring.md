# KPanel 集群监控与联邦协议

## 可选服务器资料

主机“管理”窗口通过底部“保存”统一提交显示名称、到期日期（`YYYY-MM-DD`）、价格（最多 40 个字符，可包含币种和计费周期）及
每月流量重置日（1–31，0/留空表示未设置）。设置重置日后，列表/卡片标题显示蓝色“月流量”，使用中心端持久化的周期统计；
未设置时继续展示原始遥测计数，不修改主机时间、计费服务或系统网卡计数。短月份的重置日按月末处理。信息按“到期、价格”显示于地址下方；流量重置日仅在管理设置中显示，
空字段隐藏，卡片与分享页复用同一展示组件，身份指纹和轻量节点能力在管理详情查看。
资料使用紫色到期日期、绿色价格的图标与值胶囊，悬停显示完整说明，并提供可访问名称。
集群和分享页均支持到期日期、价格临时排序及方向切换，不覆盖保存的主机顺序；空值始终置后。
价格识别单一金额、常见币种及月/季/半年/年周期，同币种按折算月价比较；不同币种、未注明周期的金额
分别分组，不使用汇率。无法识别的价格（如优惠描述、价格区间）置后；金额和周期相同保持原顺序。

至少一台主机有可估算的价格和到期日期时，集群与公开分享页顶部显示“剩余价值（估算）”；全部未配置或资料不足时隐藏整个入口和占位，已配置但估值为 0 时仍显示。点击查看按币种汇总和逐台明细。估算复用以上价格解析及到期日期，
不新增配置或网络请求，不受列表搜索筛选影响；不同币种分别计算。剩余价值为“周期价格 × 剩余天数 / 周期天数”，
最高不超过一个周期价格，到期当天及已过期为 0；每整年按 365 天，其余每月按 30 天，按浏览器本地日期计算日差，
不是实际退款余额。折算月费为价格除以周期月数。缺少日期、币种、周期或不能明确解析的价格显示未计入及原因；
支持最多 100 年周期和不超过一万亿元的单周期金额，超出估算范围保留原资料并标记未计入。
页面每分钟更新日期、打开明细时立即更新；页面卸载清除计时器。服务器资料保存、清空后即时重新估算，
登录后明细的“管理”复用原编辑窗口。匿名分享仅根据公开价格和到期日期计算，只读明细不包含管理入口或编辑指引。

“集群 → 通知 → 事件通知”中的“服务器到期提醒”统一控制所有填写了有效到期日期的主机，默认关闭；主机管理只填写日期，不再逐台勾选。开启后，通知中心按当前
KPanel 宿主时区，在提前 7、3、1 天及到期当天各生成一次 `server-expiry` 本地通知，独立于节点在线状态。
开启既有外部推送且渠道就绪时同步投递；本地记录及去重标记先原子持久化，失败不会提前消耗提醒档位。
同一日期的四档去重标记随通知历史状态保存，重启或重复勾选不重发；续费修改日期后重新计算。
停机错过的日期不补发过时提醒。关闭全局开关、清空或修改日期后，旧日期尚未成功投递的通知停止重试，
原记录保留；已进入发送中的请求无法撤回。提醒开关仅属于当前中心端，不进入匿名分享或 Agent 协议。
通知历史内部新增到期去重/重试字段及规则，通知配置新增可选 `hostExpiryEnabled`。首次加载时，任一旧主机存在有效日期且开启提醒则迁移为全局开启，否则关闭；显式全局关闭在重启后不会被旧标记重新开启。旧主机标记只作兼容保留，不再参与提醒判断，旧通知客户端省略新字段时保留已保存选择。回退到不认识这些字段的旧候选前应恢复对应版本备份，
不能把旧版本无法读取新通知历史当成空记录覆盖。

`GET /api/v1/cluster/hosts` 返回每台主机的 `hostDetails` 与独立资源版本；
`PUT /api/v1/cluster/hosts/{id}/details` 在 Session、Origin、CSRF 和审计保护下保存，
`expectedResourceVersion` 防止其他页面覆盖。统一保存仅提交已修改项，先保存资料再保存名称；名称失败时明确提示
资料已保存并保留输入，重试跳过已保存资料。两者仍沿用各自接口及资源版本。旧客户端不提交资料时不影响资料。
资料保存在现有 `panel-state.json`，最多 101 项，每次保存按现有主机清理已移除条目；
复用原子写入、失败回滚和面板备份。旧配置缺省为空；旧二进制可忽略新增字段，但其后续写入可能丢弃资料，
回滚前需保存面板备份。此功能不修改 Cluster v1/v2/light-v1 或 `kejilion.sh` 契约。

### 周期流量统计

管理窗口可设置月流量额度 `trafficMonthlyQuotaGiB`（整数 GiB，1–1,048,576，0/留空不显示百分比）和
统计方式 `trafficCalculation`（`total` 收发合计、`sent` 仅发送、`received` 仅接收、`max` 取较大值；缺省为合计）。
同时存在有效周期统计与额度时，标题显示“月流量 · 29%”；百分比向下取整，达到 80% 为警告色、95% 为危险色，
超过 100% 继续显示实际比例。标题仍保持品牌蓝色语义，收发数值和行数不变；额度、方式与不完整/估算提示
通过标题悬停及辅助阅读文本提供，不展示周期起止日期。无重置日保留“累计流量”，统计不可用时不伪造 0%。
额度只用于显示，不影响周期计数、通知阈值或宿主网络；修改额度/方式不会清零已有统计。
开启分享后，额度与统计方式进入公开白名单，分享页复用相同的“月流量 · 百分比”标题与颜色；通知阈值仍不公开。新字段沿用资料的资源版本、
原子保存与备份恢复；旧状态缺省无额度，回退旧二进制前保留备份，避免后续写入丢失新增资料。

主机配置支持独立的累计接收/发送通知阈值（整数 GiB，1–1,048,576）；每个方向填写后启用该方向提醒并覆盖全局规则，
留空（API 的 0）恢复全局阈值与启用状态。外部推送仍须开启全局通知渠道，未开启时只记本地通知。
阈值按下述当前累计口径判断，不限速、不停机；不公开到分享页，不改变周期计数。
修改有效阈值会重新评估该方向并取消旧阈值待重试通知；修改被覆盖的全局阈值不重复提醒。
配置、备份及资源版本沿用主机资料，通知去重状态包含有效阈值；回滚旧二进制需恢复对应版本的通知备份。
升级前未记录逐条阈值的待发送/失败累计流量消息会取消重试并保留历史，避免误套最新阈值补发；已有告警去重状态保留。

设置重置日即启用，无额外开关；首次设置、更改日期或中心时区后以有效采样建立基线，从零累计，
清空日期删除派生记录并恢复原始计数。名称、价格与到期日期变更不清除流量。周期按中心宿主机时区
在指定日 00:00 切换，31 日遇短月份使用当月最后一天；新周期不沿用旧累计量。
首次无法读取宿主时区时等待，不猜测周期；已知时区在 Agent 暂时不可用时保持，避免容器 UTC 回退误清零。

中心后台每 30 秒采样，独立于浏览器和推送渠道。每台主机只保留当前周期及一份原始计数游标，
最多 101 条，复用 `panel-state.json` 原子持久化、进程锁及备份恢复；重复采样不重复累计，正常面板重启可续算。
写失败不消耗增量且对外标记不可用，恢复后重试；坏记录不静默覆盖。删除节点后清理其记录，不保留月度历史。
还原备份只能恢复备份时的状态；回退旧二进制前应保留新版本备份，避免旧版本后续写入丢弃派生状态。

主机重启后保留已知累计量，能够判断属于本周期的重启后计数接续加入；网卡计数回退则重建该方向基线。
首次启用或丢失计数段标记 `partial`。跨重置点的采样间隔不超过 90 秒时按时间比例划分并标记 `estimated`；
长时间离线跨周期无法恢复精确归属，只从恢复采样起算并标记不完整，不把旧周期使用量记入新周期。
这不是运营商账单同步，采样丢失可能导致误差；网卡集合变化见下节。

集群列表/卡片、分享页和地球详情通过可选 `trafficPeriod` 展示同一份累计数据，悬停累计标题查看周期及完整性；
流量排序和累计量告警使用相同口径。实时速率及历史监控仍来自原始遥测。周期切换重新允许累计阈值提醒，
旧周期失败的推送取消重试，历史记录保留；已发送的通知不撤回。新告警去重字段回退旧版本时需恢复对应版本备份。

### 统计网卡与计数连续

遥测流量是被统计网卡的接收/发送计数之和。默认统计有默认路由（IPv4 或 IPv6）的网卡，避免回环、容器与
VPN 叠加层重复计数；没有默认路由时统计全部非虚拟网卡。WARP、故障切换线路等改变默认路由，或手动改变
选择时，统计集合随之变化。旧版本直接报告新集合的计数和，切到计数更大的集合会把新网卡此前的全部计数
一次计入本周期；采集端现在保证计数连续：

- 集合变化时记下偏移，使报告值从上次报告值继续，之后只增加新集合的真实增量；同一网卡被重建导致计数
  回退时照旧由中心端标记 `partial`，报告值下限为 0；
- 偏移以 `/proc/sys/kernel/random/boot_id` 区分开机；主机重启后清零，由中心端按 uptime 回退识别重启；
- 本机/完整 KPanel 的 Agent 把偏移写入 `<Agent 状态目录>/traffic-continuity.json`（0600，集合变化或计数回退
  时立即写，其余至多每 5 分钟一次，写失败只影响重启后的接续），Agent 重启后同一次开机内继续接续；
  停机期间集合变化时从最后保存值接续，最多损失一个采样间隔的增量，不会多计；
- 轻量节点遥测以非 root 运行且没有可写目录，偏移只保存在进程内存。同一次开机内集合变化后遥测进程
  重启（例如自动更新）时，计数回到真实值，可能出现一次跳变或一次 `partial`；需要稳定统计的轻量节点
  用下述命令固定网卡，路由变化即不再改变统计集合。
- 不新增任何协议字段，中心端、联邦 v1/v2 与 `light-v1` 报文不变；实时速率与历史监控随之连续。
  系统中心的“流量关机”由 `kejilion.sh` 自行计数，不受统计网卡选择影响，两者数值可能不同。

统计网卡由每台主机自行设置，选择存在主机本地，不进中心端状态或备份：

- 本机：集群“管理”本机的“统计网卡”在自动与“指定网卡”间切换，随“保存”作为独立阶段写入
  （资料 → 统计网卡 → 名称，失败提示已保存到哪一步）；接口为 `GET/PUT /api/v1/system/traffic-interfaces`
  （Session、Origin、CSRF、审计 `system.traffic_interfaces.update`，记录网卡名），未计入且未选择的容器、网桥、
  隧道类网卡（与自动规则同一前缀表）收在可展开的分组里，避免 Docker 主机的 veth 淹没上行网卡；Agent 端
  `/v1/system/traffic-interfaces` 以 `resourceVersion`（文件内容摘要）防并发覆盖。选择存为
  `<Agent 状态目录>/traffic-interfaces.json`，宿主机备份排除该目录，恢复到另一台机器不会带入网卡名；
- 其他完整 KPanel：在该主机自己的面板设置；中心端管理弹窗只给出指引；
- 轻量节点：在节点上以 root 执行 `/usr/local/lib/kejilion-node/kejilion-node interfaces` 查看每块网卡的
  计数、是否计入及原因；`interfaces include eth0 …` 指定、`interfaces exclude NAME …` 在自动选择中排除、
  `interfaces auto` 恢复自动。选择写入 `/etc/kejilion-node/traffic-interfaces.json`（0640 root:kejilion-node），
  遥测与文件服务按修改时间重读，下次上报生效，无需重启；卸载节点时随配置目录删除。仅 Linux 节点支持：
  计数来自 `/proc/net/dev`，其他平台的节点命令直接报错，遥测照常按各自系统接口统计。

选择文件为 `{schemaVersion:1, include, exclude}`：严格解码、不超过 4 KiB、只接受普通文件；网卡名按 Linux
规则校验并限于可打印 ASCII（≤15 字节，无 `/`、`:`、空白和控制字符，不能是 `lo`；其他名称仍可被自动规则统计），每个列表至多 16 个且不能交叉。
`include` 非空时取代自动选择；所列网卡当前都不存在（改名、PPP 断开）时回退自动选择而不是统计为 0，
界面与命令输出标出“当前不存在”。文件无法读取时同样回退自动选择并在界面提示，重新保存即可覆盖。

- 状态：集群监控已发布；多主机终端与轻量节点主动反向终端已按 v2 Noise 模型实现，自动化验证进行中；跨面板文件复制开发完成，待 L3 实机验收与发布
- 协议：KPanel 新配对默认 `v2`、兼容既有 `v1`；轻量节点遥测仍为 `light-v1`，终端反向传输复用 v2 Noise 信封与终端 payload
- 范围：主机概要监控、独立面板跳转、接入授权与撤销、多主机终端、轻量节点安全反向终端、跨面板文件复制、非面板 Linux 主机只读采集及经认证的远程终端/文件管理

## 1. 产品边界

每台 KPanel 都是对等节点，可同时作为：

- **中心端**：保存远端节点授权，后台轮询远端概要并向当前浏览器提供缓存列表。
- **被控端**：签发一次性授权码，向已授权中心端返回本机只读概要。

没有安装 KPanel 的 Linux 主机可以安装独立的 `kejilion-node`。它不是被控面板，不提供 Web 面板、
Agent、网站或 Docker 管理能力；默认低权限遥测进程只通过出站 HTTPS 主动上报主机概要，
另由同一安装包的 root `terminal-broker` 在完成中心认证后提供固定登录 Shell PTY，root `file-broker`
提供包含写入与删除的文件管理链路，并由独立的 root `ssh-login-broker` 读取 SSH 登录记录后只发布一条窄事件文件。Telegram 凭据始终只保留在中心端，
不下发到轻量节点。中心端可修改其备注、排序或移除记录，但不会显示“打开面板”。

新 v2 配对固定授权当前中心使用多主机终端和远程文件管理；新轻量节点在相应 broker 通过认证并在线后
显示终端或文件管理能力。既有 v1 与旧 v2 配对不会因升级自动获得新增权限。
v1 配对只授权只读摘要；被控端管理员可在“已授权控制端”中对单个 v1 控制端显式“允许文件管理”
（含写入与删除），随时停用。授权绑定该控制端当前密钥，保存在独立的 `cluster-v1-file-relay.json`，
撤销或以新密钥重新配对后失效，也不随面板备份迁移；旧版本回滚时忽略该文件。被控端只向已授权的
控制端声明 `file-relay-v1-signed`，且要求控制端对内层方法、路径和查询额外签名；未升级的控制端
不会再对新版被控端显示文件管理，旧版被控端继续按原协议工作。
终端使用独立 Panel Session 和 Noise v2 请求，不共享目标面板登录态；详细契约见
[`multi-host-terminal.md`](multi-host-terminal.md)。跨面板文件复制契约见
[`cross-kpanel-file-transfer.md`](cross-kpanel-file-transfer.md)。集群公开分享契约见
[`cluster-public-share.md`](cluster-public-share.md)。当前不提供批量写操作或免登录打开目标面板。
点击“打开面板”默认使用配对时保存的根地址；支持安全入口的目标面板会通过现有集群摘要同步可选入口路径，
跳转时先进入该路径再由目标面板转到登录页，目标面板仍需独立登录。入口路径只在控制端声明兼容能力后返回，
旧控制端、旧目标端或未启用安全入口时继续使用根地址，不需要重新配对。集群链路支持 HTTPS，或在没有域名时使用
端到端加密的 `http://公网IP:非80端口`；后者只保护 KPanel 间的集群数据，浏览器登录目标
面板仍是普通 HTTP，页面会在跳转前明确警告。

主机列表自动包含当前 KPanel，标记为“本机”，未设置自定义顺序时默认排在第一位。本机摘要直接经现有 Unix
Socket 读取本地 Agent，不配对、不生成密钥、不写入远端主机存储，也不占 100 台远端配额。
本机不能重复添加或移除；本机 Agent 暂时不可用时只把本机卡片标记为异常，不隐藏远端数据。

## 2. 数据与页面

左侧“概览”后增加“集群”，路由为 `/cluster`。页面一次读取中心端缓存并展示：

- CPU、内存、磁盘使用率；
- 网络累计量换算的收发速率；
- 系统、内核、架构、运行时间；
- 公网 IP、国家/地区、城市和 ISP；
- Panel/Agent 版本、轮询延迟、最后成功时间和错误状态。

浏览器每 15 秒读取一次中心端列表，请求不重叠；页面隐藏时暂停，恢复可见时立即刷新。
刷新失败保留上一次成功数据。KPanel 与轻量节点合计最多 100 台远端主机，桌面三列、平板
两列、手机一列。

登录后的主机顺序是 Panel 自身偏好，保存在 `panel-state.json` 的独立 `clusterHostOrder`
字段中；它与匿名公开页的 `clusterShare.hostOrder` 相互独立。顺序最多包含本机加 100 台远端
主机，单个内部 ID 最多 128 字节，重复、空白、控制字符和超限值会被拒绝。写入复用 Panel Store
的 `0600`、同步临时文件、原子替换、内存回滚和 `resourceVersion` 比较交换，并随 Panel 身份
备份恢复。集群页、文件管理、终端和历史监控统一消费服务端顺序；不存在的旧 ID 在展示时忽略，
新主机稳定追加在末尾，下一次用户调整时再以当前完整列表收敛。

`kpanel:cluster-host-order` 只作为浏览器缓存与旧版本一次性迁移来源：服务端已经配置时始终以
服务端为准；服务端尚未配置且浏览器存在有效旧顺序时，集群页用当前
`resourceVersion` 尝试迁移。迁移或正常保存失败不会伪装为成功；显式调整会回滚当前视图并提示，
并发冲突会重新读取服务端快照。浏览器存储不可用不影响服务端持久化。

联邦摘要使用独立的 `HostTelemetry`，不返回网站、应用、Docker、SSH 端口、DNS、系统配置、
凭据或宿主机管理能力。

## 3. 配对、身份与兼容

### 3.1 v2（新配对默认）

被控端在“集群 → 接入授权”生成：

```text
kp2.<base64url-json>
```

规则：

- 授权码包含目标节点 ID、X25519 公钥、短期 code ID、32 字节随机 secret 和到期时间；
- 5 分钟过期，只能成功消费一次；
- 连续 5 次错误后失效；
- secret 只用于本地派生配对 PSK，不出现在联邦 HTTP 请求、状态、审计或日志中；
- 新授权权限固定为 `cluster.summary.read cluster.terminal.open cluster.files.read`；其中
  `cluster.files.read` 是兼容保留的历史 scope 名称，当前授权实际包含文件写入与删除等管理操作；
  旧授权保留 `cluster.summary.read`，不会因升级自动扩权。
- 新版中心执行“添加主机”时，在主配对成功后另行协商一个文件专用的双向 link；它复用既有
  Noise 静态身份但不建立反向 Host、不授予反向终端或概要权限。旧 v2 配对可由管理员显式
  启用该 link，无需重新配对；不支持 link 的旧版本继续保持原单向能力。

中心端为每台远端主机生成独立 X25519 身份。初次配对使用
`Noise_IKpsk0_25519_ChaChaPoly_SHA256`，日常采集和撤销使用
`Noise_IK_25519_ChaChaPoly_SHA256`。请求方法、固定路径、控制端 ID、目标节点 ID、
code ID、时间戳和 request ID 都进入认证 prologue；业务正文始终位于 Noise 加密消息内。

配对采用 `Pair → Commit` 两阶段落盘。一次性授权码仍在 5 分钟后失效；目标端已认证并绑定的
事务可在 24 小时内完成 Commit，使中心端重启或短时断网后能够继续收敛。未 Commit 的控制端
不会出现在有效授权列表中。

节点私钥和每主机凭据只存放在 `cluster-secrets-v2/`，目录权限 `0700`、文件权限 `0600`；
`cluster-state-v2.json` 只保存公钥、指纹、状态和引用名。密钥写入与状态引用使用同一临界区，
状态采用同步、原子替换和恢复副本，启动及 checkpoint 会清理无引用凭据。

被控端验证目标身份、固定路径、协议版本、±2 分钟时间窗和有界 request ID 重放缓存。节点
ID 或静态公钥变化时停止信任，不自动接受新身份。

### 3.2 v1 兼容

既有 v1 主机、`cluster-state.json`、`cluster-secrets/*.ed25519` 和
`POST /api/v1/cluster/pairing-codes` 保持可读可用。v1 仍要求公网 HTTPS，并使用原有
Ed25519 签名。新前端调用 `/api/v1/cluster/pairing-codes/v2`，不会把旧授权码接口改成
另一种格式。v1 与 v2 主机可同时存在，撤销和凭据互不影响。

### 3.3 轻量节点（遥测 `light-v1`，终端复用 v2 Noise）

管理员在“集群 → 添加主机 → 非面板 Linux 主机”生成一次性命令：

```bash
bash <(curl -fsSL https://kejilion.sh) kpanel node join '<kpl1-token>'
```

同一窗口可切换到“批量接入”，生成一条 `kpb1` 命令并分别在多台目标机执行。批量授权默认与
最多均为 100 次，默认有效期 24 小时、可选 1 小时或 7 天；远程 KPanel 与轻量节点仍共享
100 台总上限，已有远程主机会占用名额。可选名称前缀仅用于显示，节点仍沿用既有
`light_node` 类型、遥测、终端、文件和自动更新能力，不增加第二套运行时。

批量命令复制成功后，页面主按钮变为“完成”；关闭窗口不会撤销授权。有效授权会显示已用/总数
和到期时间，可显式撤销未来接入，已接入节点不受影响。原始命令只在创建响应中返回，列表不会
再次返回 secret。

规则：

- 命令中的 `kpl1` 授权包含中心端当前已验证的 HTTPS 根地址、随机 ID、32 字节随机 secret 和到期时间；
  5 分钟过期且只能成功消费一次，非法名称或无效请求不会提前烧毁有效授权；
- `kpb1` 授权按策略保存 secret 哈希、有效期、最多使用次数和已分配身份。每台目标机先在
  root-only 暂存文件中生成独立 `attemptId` 与 Noise 密钥；网络响应丢失时复用同一身份重试，
  中心返回同一节点 ID 与 reporting key，不重复占用名额。成功落盘后删除暂存；换 token、换名称
  或密钥不一致时拒绝续接；
- 通过可信 `k fd` 反向代理访问时，中心直接使用当前浏览器正在访问的 HTTPS 根地址，不要求
  用户修改安装时保存的 IP + 端口地址，也不额外填写或回传凭据；
- 目标机要求 Linux、root、正在运行的 systemd、OpenRC 或原生 procd、`bash`、`curl`、`sha256sum`、`install`、
  `mktemp`、`flock`、`stat`、`readlink`、`awk`、`grep`、`sed`、`cmp`、`od`、`tr` 和系统账户创建工具；
  Alpine 可使用 BusyBox `adduser`，不要求 Docker、Go、
  Node.js 或编译环境，支持 `amd64`、`arm64`；
- procd 按 PID 1、可信 `/etc/rc.common`、`/lib/functions/procd.sh`、`ubus`、`jsonfilter` 与原生
  `/etc/init.d/cron` 能力识别，适用于满足这些条件的 OpenWrt 及其衍生系统，不依赖 iStoreOS 等品牌名称。
  缺少工具时明确报告，不替换系统服务管理器；32 位 ARM/MIPS 没有本项目发布产物。
  精简 OpenWrt 固件可能需要从同版本、同架构软件源补齐 `bash`、`curl`、HTTPS CA 证书、
  `coreutils-install`、`coreutils-stat`、`coreutils-od`、`flock` 和 `shadow-useradd`；实际缺项以预检及 HTTPS 下载错误为准。
  安装器检查依赖，不自动安装这些系统包；仅有 BusyBox `ash` 或 `/etc/init.d` 目录不足以满足安装条件。
  四个 `/etc/init.d/kejilion-node*` 服务由 procd 监督并自动重启，保持遥测低权限和 broker 权限分离，
  stdout/stderr 交给系统日志；支持时启用 `no_new_privs`，不宣称与 systemd 沙箱等价。
  procd 节点将有界监控历史和文件管理状态保存到 `0700 root:root /etc/kejilion-node/state`，
  避免 OpenWrt 常见的易失 `/var`；其余平台保留 `/var/lib/kejilion-node`。
  overlay 根磁盘使用可写上层容量并排除重复 backing mount；SSH 登录支持有界 `logread` 和 Dropbear
  完整成功事件，多因素尚未全部通过的日志不计成功。平台适配不能替代具体固件、架构与设备的实机验收；
- `kejilion.sh` 只负责固定安装协议，下载 Release 中对应架构的静态 `kejilion-node` 和
  `SHA256SUMS`，校验摘要及二进制 `version` 后再原子安装；
- 服务使用无登录、无 home 的 `kejilion-node` 系统用户运行，配置目录 `0750`，遥测凭据文件
  `0640 root:kejilion-node`；终端 Noise 私钥另存为 `0600 root:root`，低权限遥测进程不可读取；
  遥测 systemd unit 继续启用 `NoNewPrivileges`、只读系统、隐藏 home、空 capability 及地址族限制；
  OpenRC service 使用低权限用户、`no_new_privs`、严格 umask、`supervise-daemon` 和整组停止，
  但不宣称具备 systemd namespace/mount/capability 沙箱的同等隔离。root PTY broker 仅保留执行
  系统维护命令所需的宿主机访问能力；SSH 登录由独立 service 以 root 运行，systemd unit 只保留
  `CAP_DAC_READ_SEARCH` 并限制 `AF_UNIX`，OpenRC service 则使用 `root:kejilion-node`、
  `no_new_privs` 和严格 umask；两者读取 journal 或固定的 `/var/log/secure`、`/var/log/auth.log`、
  `/var/log/messages`，通过 `0750 root:kejilion-node`
  运行目录向遥测进程提供 `0640` 的单条事件，不传原始日志、凭据或网络请求；OpenRC 下各服务的
  stdout/stderr 交给 `logger` 写入系统 syslog，不维护无轮转的私有日志文件；
- 终端由独立的 root `kejilion-node-terminal` service（systemd 名称带 `.service`）运行
  `kejilion-node terminal-broker`；它不监听
  TCP、SSH、HTTP 或可被低权限节点进程调用的 Unix Socket，只通过 v2 Noise relay 主动轮询中心。
  中心返回的固定 `open`、`input`、`resize`、`close` 命令 payload 直接使用既有 v2 终端请求结构，
  由接入时绑定的节点终端静态公钥完成认证后才进入 root PTY Manager；节点重启时会在轮询会话
  ID 对账中回收旧终端。新版 broker 另外保持一条 `light-terminal-control` 流式控制连接：连接在线时
  中心为新会话请求节点回拨 `light-terminal-data` 连接，由节点推送带偏移的输出，去掉轮询中继最多
  750 ms 的挂起；会话 2 分钟内没有中心重新附着即关闭 PTY。旧中心在认证阶段拒绝该角色时，broker
  按 1 分钟起、最长 30 分钟退避重试，轮询中继始终可用；
- 节点默认每 30 秒采集一次，仅向授权中的 HTTPS 中心地址出站上报；遥测请求继续使用独立
  32 字节 reporting key 做 HMAC-SHA256，绑定方法、固定路径、节点 ID、时间戳、request ID
  和正文摘要；终端请求不复用 reporting key，而使用 root-only Noise 私钥和中心公钥，中心校验
  v2 信封、±2 分钟时间窗与有界重放缓存；
- 节点以客户端实际 HTTPS 上报请求耗时作为连接延迟：首次上报没有历史样本，成功后在下一次
  上报通过可选 `X-KPanel-Light-Report-Latency-Milliseconds` 传递上一轮 RTT；中心只接受
  `1–15,000 ms` 的有界值，旧节点或无有效样本显示为未知，不把 `0 ms` 当作真实延迟；
- 自动更新在 systemd 使用 timer（启动约 15–30 分钟后检查，随后每次结束后 1 小时再检查），
  在 Alpine/OpenRC 使用 `/etc/periodic/hourly` 与 `crond`，procd 在 `/etc/crontabs/root` 中维护唯一
  KPanel 行，每小时第 17 分钟调用固定 `update-cron.sh`；保留其他任务，卸载不停止共享 cron。
  三种后端都加入 0–15 分钟随机延迟，
  中心端不推送更新。先下载有界 `SHA256SUMS`，仅在摘要变化时下载
  同一 Release 的二进制，HTTPS 下载带有界重试；内核 `flock` 在进程退出后自动释放。
  更新检查同时核对 `/proc/MainPID/exe`，恢复磁盘已替换但进程仍旧的中断；遥测重启失败回滚，
  可选终端、SSH 采集、文件服务失败单独报告，不阻断正常遥测升级；
- 配置能力升级原子保留 `node.json` 的属主、组和权限。更新器只对既有安全安装目录中的固定
  遥测配置恢复 `root:kejilion-node 0640`，不扩读终端私钥或自定义路径；
- 旧安装通过已校验临时 Release 二进制的 `version` 兼容桥安装同源更新器与 timer，无需重新配对。
  当前旧更新脚本仍会执行完原逻辑，下一轮才使用新流程；PID 与启动时间保护这段交接并发。
  迁移只适用于旧更新器仍能完成下载/校验且存在 `flock` 的机器。旧目录锁已卡死、timer 被禁用或
  GitHub 长期不可达时，发布中心端不能远程恢复，须读取该节点服务/更新日志定位并恢复更新入口；
- `k kpanel node status|update|uninstall` 分别用于状态、手动更新和本机卸载；`status` 同时显示
  遥测服务和 SSH 登录采集服务。中心删除记录不远程执行卸载；节点被移除后上报凭据立即失效，
  目标机由用户自行卸载或重新接入。

中心将轻量节点状态和凭据分别保存在 `cluster-light-state.json` 与
`cluster-light-secrets/`，批量授权策略独立保存在 `cluster-light-batch-state.json`。策略文件只含
secret 哈希，不含原始命令，并限制为 2 MiB、最多 16 条有效策略、每条最多 100 个分配记录；
三者均纳入 Panel 备份。接入、改名和删除等配置变更立即使用 `0600`、同步和原子替换持久化；
30 秒遥测只更新内存快照，由既有 5 分钟 checkpoint 或正常退出合并落盘，避免高频磁盘写入和
无意义的 `resourceVersion` 冲突。reporting key 不进入状态、审计、
浏览器响应或日志。单次接入的消费、凭据写入和主机落盘属于同一事务，落盘失败会恢复授权并清理
孤立凭据；批量接入先持久化 attempt 分配，再原子写入节点凭据与主机状态，中途失败时同一 attempt 可续接且
不重复占用名额。

## 4. 网络与 SSRF

联邦只访问用户登记的规范化 origin：

- v1 只允许 `https://host[:port]`；
- v2 允许 `https://host[:port]`，或 `http://字面量IP:非80端口`；
- 两种形式都禁止 userinfo、路径、查询、fragment 和重定向；
- TLS 最低 1.2，验证系统 CA、主机名和证书有效期；
- 不继承 `HTTP_PROXY`/`HTTPS_PROXY`；
- 每次拨号重新解析全部地址，先校验再直接拨校验后的 IP，TLS SNI 保留原主机名；
- 默认拒绝 loopback、link-local、multicast、unspecified、RFC1918、ULA、CGNAT、
  文档保留地址、NAT64/6to4/Teredo 转换前缀和云元数据链路；
- 私网只能通过部署端 `KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS` 精确放行；
- 混合返回公网与受限地址时整体拒绝，防止 DNS rebinding。

轻量节点方向相反：中心不主动访问目标机，也不接收其 URL 或开放端口；节点只连接授权中
经过严格解析的 HTTPS 根地址，拒绝 userinfo、路径、查询、fragment、降级 HTTP 和重定向，
TLS 最低 1.2。因而普通 NAT、动态公网 IP 或仅可出站的主机也能加入，但中心端必须具备被
目标机访问的有效 HTTPS 地址。中心端正常操作仅为生成并复制命令；目标机需以 root 执行，
具备 `curl` 且能够出站访问该 HTTPS 地址。

HTTP v2 的集群正文使用 Noise 端到端加密且绑定目标静态身份，但 HTTP 本身不能保护浏览器
访问目标管理页。Agent 仍只监听本机权限受限的 Unix Socket。联邦入口位于 Panel，不开放
Agent TCP，也不复用 Agent Token、Bootstrap Token、管理员密码或 Session Cookie。

## 5. API

浏览器接口需要 Panel Session；所有写入同时验证 Origin、CSRF 并写审计：

```text
GET    /api/v1/cluster/hosts
POST   /api/v1/cluster/hosts
GET    /api/v1/cluster/host-order
PUT    /api/v1/cluster/host-order
GET    /api/v1/cluster/hosts/{id}
PATCH  /api/v1/cluster/hosts/{id}
DELETE /api/v1/cluster/hosts/{id}
POST   /api/v1/cluster/hosts/{id}/refresh
POST   /api/v1/cluster/pairing-codes
POST   /api/v1/cluster/pairing-codes/v2
POST   /api/v1/cluster/light-enrollments
GET    /api/v1/cluster/light-batch-enrollments
POST   /api/v1/cluster/light-batch-enrollments
DELETE /api/v1/cluster/light-batch-enrollments/{id}
GET    /api/v1/cluster/controllers
DELETE /api/v1/cluster/controllers/{id}
```

Panel 间固定接口：

```text
POST   /api/v1/federation/pair
GET    /api/v1/federation/summary
DELETE /api/v1/federation/revoke

POST   /api/v2/federation/pair
POST   /api/v2/federation/commit
POST   /api/v2/federation/summary
POST   /api/v2/federation/revoke
POST   /api/v2/federation/terminal/open
POST   /api/v2/federation/terminal/output
POST   /api/v2/federation/terminal/input
POST   /api/v2/federation/terminal/resize
POST   /api/v2/federation/terminal/close
POST   /api/v2/federation/files/open
GET    /api/v2/federation/files/stream

POST   /api/v3/federation/light/enroll
POST   /api/v3/federation/light/batch-enroll
POST   /api/v3/federation/light/report
POST   /api/v2/federation/terminal/relay
```

v2 只接受固定 POST 路径和文件流的精确 GET Upgrade 路径；light-v1 遥测只接受以上两个精确 POST 路径，终端 relay
使用 v2 固定路径。两者都拒绝查询
参数或 `RawPath` 变体。Noise 外层请求上限 96 KiB，解密后业务负载上限 64 KiB；轻量节点
接入和上报使用更小的固定请求上限。所有接口使用严格 JSON，拒绝未知字段和多值 JSON。

### 完整 Panel 流式传输 v3 与兼容链路

目标 Panel 在 v2 摘要的 HTTP 响应头 `X-KPanel-Response-Capabilities` 中声明 `panel-stream-v3`；
中心只对声明过的主机使用流式传输，从不探测。该头未经认证，只作为优化提示：伪造会让 Noise 握手
失败并回退，删除只会变慢。流式传输复用 `GET /api/v2/federation/files/stream` 与既有静态密钥，
新增两个仅限已激活控制端的角色，每次开连接都重新校验控制端状态和 scope：

- `panel-file`（需 `cluster.files.read`）：同一条已认证连接可顺序承载多个受限文件请求，每个请求前
  再次核对控制端授权；中心每主机最多保留 2 条空闲连接（空闲 20 秒、最长 10 分钟），提前拒绝或
  上传未完成的连接绝不复用；复用连接在收到响应前失败时，只有无正文的 GET/HEAD 会在新连接上重试一次。
- `panel-terminal`（需 `cluster.terminal.open`）：每个终端会话一条连接，目标推送带偏移的输出
  （3 ms 合并），输入、调整尺寸和关闭均需确认。PTY owner 与 v2 相同（`federation:<控制端>`），
  中心断线后按偏移重新附着；6 秒内无法恢复时同一会话继续走 v2 POST。

任何在发出请求记录之前的失败（升级被拒、代理拦截、握手失败）都回退 v2，并在 2 分钟内不再尝试；
已发出的写操作不自动重放。v2 文件中继使用独立连接池（每主机 4 条），不再与终端和摘要共享两条连接。
未声明能力的目标继续使用 v1.14.1 的固定 POST 文件链路：文件管理通过
`/api/v2/federation/files/relay` 轮询，每次请求执行独立的 Noise 认证交换；跨节点导出继续通过
`/api/v2/federation/files/open` 和只读的 linked grant 传输，`panel` 与 `linked` 旧角色仍被拒绝。
既有 DNS/IP 拨号检查、TLS 验证、禁止重定向、权限范围和请求限流保持不变。经被控端授权的 v1 Panel 继续使用原有
HTTPS 流式接口。设计与实测对比见 [终端与文件传输 v3](terminal-file-transport-v3.md)。

轻量节点保留 v1.15.0 的 `GET /api/v2/federation/files/stream` 实现，WebSocket 子协议为
`kpanel-file-stream-v1`。节点保持一条主动连向中心的控制连接，收到请求 ID 后再主动建立该请求的
数据连接；回接必须匹配节点身份、当前控制连接 generation、未过期且未消费的请求 ID。握手通过
既有静态密钥的 Noise IK 认证并绑定 GET、路径、双方 ID、时间戳与随机 request ID；数据帧最多
60 KiB，不压缩，也不将文件内容转换为 JSON/base64。

控制连接每 15 秒心跳，45 秒无响应断开，按 1/2/4/5 秒退避重连；轻量 broker 最多同时处理
8 个数据任务。中心限制 64 个未认证握手、128 个已认证连接和每身份 16 个连接（含 v3 终端与文件连接）。文件流分别为
大文件与目录/元数据操作保留全局 16、每身份 4 个并发，握手 8 秒超时，数据空闲 45 秒超时，
单文件最长 2 小时。只有加密结束帧的累计长度匹配才产生 EOF；导出还必须通过 Agent 的最终
`X-KPanel-Transfer-Result: ok` 校验。取消、断流、删除轻量节点和 Service 关闭都会取消对应连接。
Panel 为认证失败及每个轻量数据连接记录完成/失败审计，控制心跳不逐条写审计。

轻量 broker 只有在 GET 升级前明确收到 404/405/426 时才切换到兼容轮询；选中兼容模式后保持到
进程重启，避免探测升级时中断旧传输。101 后的认证失败、断流和超时不回退，也不重放已经开始的
文件动作。节点回滚旧版本后，新到达的认证文件 poll 可在流式控制断开时恢复兼容模式。兼容文件
broker 继续复用既有 Noise 身份、固定文件路由和 64 KiB 业务负载，不增加节点入站端口。
中心在处理数据后立即交付浏览器，每轮成功文件 poll 的总时长至少 125 ms（包含命令等待和背压），
顺序 broker 因而低于每节点 600 次/分钟的现有限流；旧节点也适用。空闲仍长轮询，活动空轮询为 125 ms。
新版节点按会话轮转装入批次，数据块为 23 KiB，每会话通道和待发队列分别最多 16 个事件；
负载空间不足时保留事件顺序。确认丢失后，中心只接受最近一批内 offset、长度与 SHA-256 全部相同的
完整块重发；broker 的单次 HTTP 取消不再丢弃仍在读取的浏览器流，浏览器取消、节点重启和超时仍清理。
来源限流仍为文件/历史共用的 1200 次/分钟，多节点共用出口时仍受此预算约束。

## 6. 轮询与状态

中心端默认每 30 秒轮询，加入 ±20% 抖动；全局并发上限 8、单主机连接上限 2、总超时
6 秒、响应头超时 3 秒。失败按指数退避，最大 5 分钟，不自动重复配对或写操作。

状态规则：

- `pairing`：两阶段安全配对尚在后台收敛；
- `revoking`：进程中断后恢复到待撤销状态；
- `online`：最近成功且没有连续失败；
- `degraded`：有最近快照，但当前出现少量失败；
- `stale`：最近成功超过 90 秒；
- `offline`：连续 3 次失败；
- `auth_failed`：签名或授权失效；
- `tls_error`：证书或 TLS 校验失败；
- `incompatible`：协议或远端节点身份变化。

轻量节点不由中心轮询：最近上报不超过 90 秒为 `online`，不超过 5 分钟为 `stale`，之后为
`offline`。终端 broker 使用同一出站 HTTPS 根地址独立长轮询；最近 2 分钟没有经过认证的
终端轮询时，中心暂不把终端能力视为可用，但不改变遥测在线状态。中心只保存最新快照；节点
断网时继续以有界指数退避尝试，不在目标机堆积历史或任务。轻量节点与 KPanel 节点共享列表、
搜索、排序和 100 台上限，但不占中心普通摘要轮询并发。

轻量终端轮询首次完成 v2 Noise 身份认证后才开放终端能力。每台节点最多 4 个终端会话；命令
payload 直接复用既有 v2 的 `TerminalOpenRequest`、`TerminalInputRequest`、
`TerminalResizeRequest`、`TerminalCloseRequest`，不再使用独立终端 HMAC。中心保留待确认命令，
节点丢失 HTTP 响应时会安全重投递；节点通过当前会话 ID 列表对账，重启或消失的会话会在中心
回收，不把旧 PTY 当作仍然可用。中心 relay epoch 变化（例如中心进程重启）时，节点 broker
也会关闭本地旧 PTY，避免中心索引丢失后留下不可控会话。

只保存最新快照，不保存高频历史。v1 的 `cluster-state.json` 和 v2 的
`cluster-state-v2.json` 均采用同目录临时文件、`0600`、同步和原子替换；每 5 分钟及正常
退出时 checkpoint。轮询进程重启后从最新快照和未完成事务恢复，真实状态仍由下一次远端
摘要刷新。

删除主机时先尽力撤销远端授权，再让本地状态和凭据收敛。远端不可达不会把本地条目永久卡
在“撤销中”；API 返回 `remoteRevoked=false`，目标端残留授权可在其“接入授权”页面手动
撤销。若凭据清理暂时失败，API 会明确返回清理状态；下次服务初始化会删除无引用凭据。

标准 Compose 为 Panel 增加独立出站网络，仅用于联邦 HTTPS 或 Noise 加密 HTTP；容器仍
保持非 root、只读根、`cap_drop: ALL` 和 `no-new-privileges`。应用层每次拨号都执行上述
SSRF 与 TLS 校验。对
网络隔离要求更高的部署，仍建议在宿主机 `DOCKER-USER`/专用出口网关增加出站 ACL；首版
尚未把联邦轮询拆成独立 sidecar。

## 7. 编码前质量记录

| 项目 | 决策 |
| --- | --- |
| 流量路径 | 浏览器 → 当前 Panel；当前 Panel → 远端 Panel HTTPS 或 Noise 加密 HTTP → 远端 Agent Unix Socket；轻量节点 telemetry、root terminal-broker 与 root file-broker → 中心 Panel（遥测 HMAC，终端/文件 v2 Noise），file-broker 在节点内直连 filemanager |
| 不可信输入 | 主机名称/批量名称前缀、origin、单次或批量授权码、batch attempt ID、DNS 结果、远端证书、远端 JSON、轻量节点时间戳/request ID/HMAC/遥测、终端 Noise 信封 |
| 权限与可写范围 | Panel 只写自身 v1/v2/light 集群状态、凭据目录与 `panel-state.json` 中的私有主机排序；不写宿主机业务目录 |
| 最坏输入/输出 | 远端主机合计 100；单条批量授权最多 100 次、同时最多 16 条、批量状态 2 MiB；v1 配对 16 KiB；v2 外层 96 KiB、解密负载 64 KiB；摘要 64 KiB；轻量请求有界；Store 4 MiB；控制端 256；各类有效授权码均有界 |
| 最大并发 | 普通摘要轮询 8、单主机连接 2（v2 文件中继另有每主机 4 条独立连接）；v3 每主机最多 2 条空闲文件连接，终端每会话 1 条；每个轻量节点最多 4 个终端会话；轻量文件流按大文件/短请求分别全局 16、每身份 4 个，升级连接单独限额；请求与 nonce/rate-limit 缓存均有界 |
| 超时与重试 | 普通联邦连接 2 秒、响应头 3 秒、总计 6 秒；批量接入以本机 root-only attempt 状态幂等续接；轻量终端长轮询最长 25 秒，节点请求总超时 40 秒；轻量文件命令接单最多 12 秒；轻量节点首次收到旧中心 404/405/426 时从 1 秒指数退避至 5 分钟，其他文件连接失败按 1/2/4/5 秒退避；Relay 丢失确认时可重发同一命令和完全相同的数据批次，浏览器不重复请求，写入不自动重试 |
| 真实状态与缓存 | 远端 Agent 实时摘要是事实；中心只缓存最近快照；本机摘要缓存 5 秒；Panel Store 是登录后主机顺序真源，浏览器 localStorage 仅迁移和缓存 |
| 失败与恢复 | 保留最近成功快照；排序写入失败回滚内存、并发版本冲突重新读取；批量接入响应丢失时同 attempt 返回同一身份，撤销/到期只阻止新节点；认证/TLS/身份错误单独标识；先尽力撤销远端授权，再删除状态，最后清理凭据；孤立凭据启动时回收 |
| 性能预算 | 浏览器单请求，无 N+1；100 台 KPanel 按 30 秒轮询约 3.3 请求/秒、最多 8 并发；轻量 telemetry 每台约 2 请求/分钟；轻量 SSH broker 每 5 秒轮询但使用 15 秒读取缓存，约 4 次/分钟本地日志采样，不产生额外网络请求；终端空闲时由长轮询维持，不触发中心出站 |
| 网络入侵风险 | SSRF、DNS rebinding、TLS 劫持、单次/批量授权码泄露或猜测、批量配额耗尽、签名重放、伪造遥测、恶意大响应和轮询/上报 DoS；批量入口按来源与策略各 240/分钟限速，允许 100 台并发接入及有界重试 |

## 8. 验收

自动测试至少覆盖：

- 普通 Panel 的 v1.14.1 文件管理、跨节点复制与反向只读导出；轻量节点真实 TLS WebSocket 上传、下载和流间复制；轻量流提前拒绝、取消、帧篡改、缺失结束帧/最终 trailer、撤权及旧端点回退；

- HTTPS 与字面量 IP origin 规范化、loopback/私网/元数据/IPv4-mapped IPv6、混合 DNS 与 rebinding；
- 授权码过期、错误次数、并发单次消费及明文不落盘；
- 批量授权默认/最大 100、有效期与撤销、不同 attempt 的独立身份、同 attempt 的幂等续接、
  名称/密钥不一致拒绝、总主机上限，以及 token/reporting key 不进入策略文件、列表或审计；
- v1 签名及 v2 Noise 篡改、错误 PSK/身份/路径、过期/未来时间和 request ID 重放；
- 两阶段配对重启恢复、密钥/状态原子性、慢节点不阻塞其他轮询与管理操作；
- 本机始终只出现一次、不落盘、不占远端配额、不可删除；
- Browser Session、Origin、CSRF、未知字段、请求体和响应体上限；
- 私有主机顺序的未配置/已配置空值、上限与非法 ID、持久化重启、写失败回滚、备份恢复、
  并发冲突、浏览器旧值迁移、服务端优先级，以及集群/文件/终端/历史监控的一致排序；
- 100 台并发上限、退避、取消、重启恢复和 `go test -race`；
- 前端无 N+1、轮询不重叠、失败保留旧数据、外链安全与移动端布局；
- 轻量授权 HTTPS 约束、过期/单次消费、HMAC 篡改、未来/过期时间、重放、状态阈值、
  凭据原子性、错误请求限速和密钥不进入审计；轻量终端 v2 Noise 身份/密文/重放、重投递、
  会话 ID 对账、固定 root PTY、命令/输出上限、旧中心 404/405/426 兼容和 broker 故障不影响遥测；
- `kejilion-node` 严格配置、拒绝重定向、固定动作、静态跨架构构建；安装器无 Docker 依赖、
  Release 摘要验证、systemd/OpenRC/procd 服务权限、自动更新回滚与失败安装清理；
- 标准 Compose 和应用市场部署都能出站验证 HTTPS，Panel 仍无 Docker Socket 和宿主权限。

发布前执行 L2 验证；正式版本与镜像发布仍按 L3 流程执行。

轻量节点变更涉及新的脚本协议、安装/更新路径或内置脚本内容时，按 `PROJECT_RULES.md` 1.5 判为
`coupled` 并记录变更集和成对证据；仅消费已有兼容契约的面板修改按实际差异判定，不能仅因属于轻量节点就要求脚本发布。
需要双端联动时，发布顺序固定为：先发布兼容旧 KPanel 的 `kejilion.sh` 安装入口，再发布
包含 `kejilion-node`、`SHA256SUMS`、`light-v1` 上报、SSH 登录采集和 v2 Noise relay 的 KPanel Release。
既有轻量节点不需要逐台手动执行 `k kpanel node update`，也不需要重新配对。节点现有的 root 自动更新
任务下载并校验新版 Agent 时，会由已校验的临时二进制自动补写并启用独立的
SSH 登录采集 service（systemd 为 `kejilion-node-ssh-login.service`，OpenRC 为
`kejilion-node-ssh-login`）；报告密钥和节点身份保持不变。其他已存在的节点辅助单元继续按原有兼容流程管理。
新节点接入会一次写入全部单元。脚本先
发布期间下载资产返回 404 只会让新安装明确失败，不影响既有 KPanel 节点；KPanel Release 发布后
必须在独立 Linux 主机完成安装、断网恢复、自动更新、摘要拒绝、终端 broker 重启、回滚与卸载
闭环，才可结束 L3。

安装、续装、手动/自动更新及卸载共享固定的生命周期锁；锁 inode 位于 root 管理的 `/run`，不随
安装目录一起删除。兼容旧的 flock 和 mkdir 更新器时，必须先验证旧进程身份，只能回收已确认
无主的空目录。并发调用明确返回稍后重试，不清理另一事务的身份或运行时。
新更新器在 `kejilion-node-release.*` 暂存目录校验资产，避免旧发行二进制的 `version` 命令
触发旧运行时回写。新的迁移桥兼容新旧暂存目录，并拒绝降低已安装的
`KPANEL_NODE_RUNTIME_GENERATION`；修改脚本更新器/共用锁/update 单元模板必须递增这个内部代数，
再通过 `scripts/sync-light-node-runtime.mjs` 同步来源。代数不是脚本版本号。

### 轻节点更新与服务观测

节点详情将遥测在线与更新/辅助服务观测分开。支持 `light-health-v1` 的中心通过现有响应头协商；
新节点首次使用旧格式，获得能力后才在签名请求中附带 `health`。中心降级导致 400/422 时，节点
仅重试一次去掉可选健康与 SSH 登录字段的请求，并重新生成请求 ID 和签名；能力开关不落盘。
旧节点缺报或非法健康摘要不会阻断基础遥测，也不会刷新旧健康。匿名分享不包含健康、更新错误码或运行版本。

更新器在既有生命周期锁内原子替换 `/etc/kejilion-node/update-status.json`（root:kejilion-node 0640），
固定暂存文件最多一份，包含最近检查/完成时间、有限结果与错误码，不保存 stderr、URL、凭据或历史日志。
退出码保持原更新语义；核心失败回滚、辅助服务降级、被中断与成功分开。已有安装失败/卸载清理配置目录的流程
覆盖这些产物。遥测以 1 KiB 上限和无符号链接的安全读取获取状态，再以总计 2 秒、4 KiB 输出上限的固定
后端查询 timer、telemetry、terminal、file、SSH login 五项：systemd 使用固定的 `systemctl show`，
OpenRC 以活动中的 `/run/openrc` 为优先信号，只读取固定 service 的 `rc-service status`、init 脚本、
default runlevel 链接和 hourly periodic 文件；查询失败为未知，`not-found` 才为未安装。
procd 由现有 root 文件 broker 每 20 秒在同一 2 秒预算内读取固定服务的 `ubus` 状态、开机链接和
cron 成员关系，写入 `0640 root:kejilion-node /run/kejilion-node-monitoring/procd-health.json`。
遥测仅通过既有安全文件读取规则消费 4 KiB 内、60 秒以内的快照；缺失、过期、无权限或非法内容均为未知，
不为查询状态放宽 `ubus` ACL，也不增加常驻进程。状态依据 `running` 与有效 PID，不以脚本存在代替运行。
单元运行不代表终端/文件权限或 relay 连接已经可用，原权限判断保持独立。

中心健康仅存在内存快照，不改变旧中心严格解码的持久状态；中心重启后等待重新上报。页面按观测时间 90 秒
判过期，检查记录超过 3 小时判过期，`running` 超过 30 分钟判中断；刷新失败时旧观测也会按时间失效。
已实现并通过本地协议/权限/故障回归、加速 systemd timer 及 OpenRC service/periodic 契约；
下载为夹具、调度使用加速触发，真实 Alpine OpenRC PID 1、真实发行资产、原调度周期、旧节点自动迁移
与公开产物 L3 尚未验证。证书/续签契约继续沿用成对父基线。

## 9. 回滚

该功能不迁移网站、Docker 或系统业务状态。回滚 Panel 到上一稳定镜像后：

- v1 的 `cluster-state.json`/`cluster-secrets` 与 v2 的
  `cluster-state-v2.json`/`cluster-secrets-v2` 保留，不影响其他面板功能；
- `cluster-light-state.json`/`cluster-light-secrets` 同样是独立可忽略状态；批量策略另存于
  `cluster-light-batch-state.json`，旧 Panel 会忽略它，回滚后尚未执行的 `kpb1` 命令不可接入；
  旧 Panel 不读取轻量节点状态与凭据，
  目标机上的 `kejilion-node` 只会变为离线重试，不影响宿主机业务；
- 旧版本继续读取 v1 文件并忽略 v2 文件，因此原有 v1 主机仍可回滚使用；
- 旧 Panel 会忽略 `panel-state.json` 中新增的 `clusterHostOrder` 可选字段；若旧版本随后重写
  `panel-state.json`，该显示偏好可能丢失，但不会改变主机、凭据或公开分享状态。旧版本恢复工具
  可能拒绝包含该新字段的新版身份备份，跨版本恢复应保留并使用升级前备份；
- 如需彻底撤销，先在各节点“接入授权”中撤销控制端，再在停机维护窗口备份并删除集群文件；
- 不得通过回滚删除 `/home/web`、Docker 容器或 Agent Token。
- 统计网卡：旧版本忽略 `traffic-interfaces.json` 与 `traffic-continuity.json`，恢复自动选择和原始计数和；
  存在偏移时回滚瞬间报告值回到原始值，中心端按计数回退标记一次 `partial` 或计入一次跳变。轻量节点回退旧
  二进制后选择文件同样被忽略；如需彻底清除，执行 `interfaces auto` 后再回退。
