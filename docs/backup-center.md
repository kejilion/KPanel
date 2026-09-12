# 面板数据备份与恢复

入口为“设置 → 备份与恢复”。导出选择 `panel/apps/web/docker`，输入备份密码；导入 `.kpb` 后先检查，再选择类别、确认覆盖。完成的加密包通过登录态下载，不经浏览器内存拼接。任务也在操作记录展示。

## 范围与同源协议

| 类别 | 权威来源与保存内容 | 边界 |
| --- | --- | --- |
| 面板 | 账户、TOTP 加密材料、集群节点身份和现有配对密钥、桌面、通知配置 | 不复制 Session、登录尝试、审计历史、分享授权、临时配对码、运行缓存。目标审计与安全入口保留；通知默认关闭 |
| AI（随面板） | API 服务商、地址、API Key、模型配置 | 不备份会话、消息、附件、运行记录、记忆或流程。恢复替换接入配置，以目标密钥重新加密；目标已有会话保留 |
| 应用 | `kejilion.sh` 应用市场 `/home`、`/home/docker` 的资源目录、Compose 文件、容器配置和本地卷 | Panel 自身持久数据由面板逻辑导出；不复制 Agent Token 或运行二进制 |
| 网站 | LDNMP `/home/web`，包含站点、数据库目录、当前证书及现有环境配置 | 冷停相关容器后归档；不生成替代 Nginx/Compose/TLS 模板 |
| Docker | 原生 inspect 配置、挂载数据、本地命名/匿名卷、原生网络配置 | 与脚本 Docker 备份一样不导出容器可写层或镜像 tar；保留精确镜像 ID/仓库摘要，缺失时按摘要获取；无可获取摘要的本地镜像须先装载 |

`internal/hostbackup` 为 Panel 和脚本共用适配器。脚本提供 `k backup-center`，以及应用市场、LDNMP 和 Docker 备份菜单中的“通用加密备份与恢复”入口。依赖匹配的本机 Agent 服务；不下载或执行备份包中的脚本。根/CN 入口同步，旧 `.tar.gz` 及旧 Docker 目录格式继续由原菜单还原；通用中心仅接受 `.kpb`。

脚本相关三类支持同一格式的双向互通：脚本导出 → KPanel 导入，KPanel 导出 → 脚本导入。脚本可以选择其中部分分类；含面板数据的包在脚本端仅处理主机类别，面板身份仍通过 KPanel 恢复。所有调用共享 Agent 的任务锁和恢复日志。

## 格式、存储与容量

`.kpb` 为格式版本 1：有界 tar（固定 manifest 与模块 payload），scrypt 派生密钥，XChaCha20-Poly1305 分帧认证加密，显式认证结束帧；每个模块另有 SHA-256 和长度校验。必须完整验证密码、认证标签、结尾和摘要后才能进入恢复。密码为 10–256 字节，不进入命令行、环境变量、任务、审计或回执。

Panel 存储于 `<DataDir>/backups/<id>`，Agent 为 `<StateDir>/backup-center/<id>`；目录 `0700`，文件 `0600`。CLI 明文暂存位于 Agent 状态根目录的 `backup-cli`，正常退出即删除，异常残留 24 小时后清理。应将下载副本另存到其他设备。

单包所含模块总计上限 50 GiB，面板模块 32 MiB；主机每类最多 100,000 条归档项、单文件 10 GiB，总计最多 512 个容器、4,096 个根目录。每个管理器最多 50 条任务，任务时限 2 小时。加密导出保留 7 天，导入及临时明文 24 小时，记录 30 天；后台按分钟清理。需要故障恢复的日志及回滚副本保留到处理完成，不能通过删除记录绕过。

库存显示源数据估算，不承诺压缩率。纯面板配置通常远小于业务数据，但实际大小以现场库存为准。导入同时需要上传包、解密内容和解包空间；主机恢复还需要新数据副本与旧数据回滚副本。执行前检查空间，实际写入继续受大小限制。

## 事务与失败边界

导出根据共享挂载、namespace 和 legacy links 建立依赖闭包；依赖缺失时要求同时选择相关类别。按依赖逆序停止容器、正序恢复原运行状态；数据库使用停机文件备份。请勿同时通过 SSH、外部 Compose 或其他进程修改数据。Agent 在备份期间拒绝新的写操作，并检查已有应用、网站、Docker、文件和磁盘任务；开始前须关闭宿主机终端。面板自身的容器、定制数据路径及命名卷均被排除，与业务目录重叠时预检拒绝。

还原先完全解包验证、检查精确镜像、卷映射、目标共享写入者及磁盘空间。写入意图先落盘，再保留旧目录与容器、切换数据、重建并检查服务。健康失败回滚；完成提交后只清理旧副本，重启不能反向回滚已提交结果。回滚失败保留证据、停止后续恢复，入口“处理恢复中断”继续回滚或清理，不重放导入。

面板恢复等待 HTTP 请求、后台服务及 Store 关闭后离线替换，重新初始化账户、AI 和集群并成功绑定监听端口后提交。导入与应用前逐项验证已有配对所引用的密钥，排除待配对和孤儿密钥；未应用前检查失败会取消本次意图，不阻止旧配置启动。崩溃时根据日志完成已提交的清理或回滚未提交的数据。主机类别与面板类别有独立提交点；部分成功时展示 `completedModules`，不能承诺跨 Docker、JSON 与 SQLite 的全局原子事务。

当前格式处理普通文件和目录，保留 Linux UID/GID、权限位。符号链接、特殊文件、包含独立挂载点的数据根、非本地卷驱动/带选项卷、rootless/userns-remap 需要对应适配器，预检拒绝并说明边界。运行中的 AutoRemove 容器须先由用户处理，避免冷停自动删除源容器。SELinux 标签、ACL/xattrs、发行版迁移、跨 CPU 架构与真实 systemd 沙箱仍需发布画像实测，不以单元测试代替。

## 集群迁移

恢复节点身份和配对密钥，且节点对外地址（域名、协议、端口）不变时，现有配对可继续使用。只有域名不够：身份/密钥缺失、被撤销、访问路径变化或对端不可达仍会失败。目标上针对同一节点已有的撤销记录保留。切换 DNS 前停止旧面板，避免两个实例使用同一节点身份。IP 地址改变时需要调整对端连接地址，必要时重新配对；本功能不修改 DNS 或自动改写对端。

## 接口

Panel 写接口统一校验 Session、Origin、CSRF；公开列表及预览不返回密钥或环境变量。

| 路径（`/api/v1/backups`） | 方法 | 行为 |
| --- | --- | --- |
| 根路径 / `inventory` | GET | 有界任务列表 / 模块容量和主机依赖 |
| `export` | POST | `{modules,password,agentRevision}`，返回持久化任务 |
| `import` | POST | multipart，先 `password` 后 `file`，流式上传并检查 |
| `<id>` / `<id>/download` | GET | 单条状态 / 完成的加密包 |
| `<id>/preview` | POST | 刷新目标配置 revision |
| `<id>/restore` | POST | `{modules,revision}`；只允许源包中的模块 |
| `<id>/recover` | POST | 处理对应 Agent 的中断恢复 |
| `<id>` | DELETE | 清理对应 Agent 临时产物及 Panel 记录，活动/待恢复任务不可删除 |

Agent Unix Socket 协议为 `/v1/backups`，格式/协议均为 1。固定动作 `export/import/restore`；固定子操作 `inspect/preview/recover/abort`；`PUT/GET <id>/<apps|web|docker>` 仅传输对应模块。CLI 机器入口 `backup-center file-export` 与 `file-import` 从有界 JSON stdin 读取 `{path,password,modules}`；`file-import` 只检查，恢复须另用 `preview` 和 `restore ID` 明确提交。

## 验证与发布联动

变更集 `backup-center-20260912`，`scriptLinkageState=coupled`。Panel 基线 `8918a5e56483913b9d9e420cb472c6a0f59626e8`；配套脚本已先发布到 `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`，根脚本 SHA-256 为 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048`，CN 脚本 SHA-256 为 `62b01b5b1ba736fafe1a167733d64eaba64606f9c5a4b77fa1cd136adbf843cc`。KPanel 发布候选必须固定这一组合来源并完成 L3。

验收包括加密包损坏/错误密码、选择依赖、原生匿名卷与 namespace/links、数据权限、离线账户事务、重启回滚与提交后清理、既有 AI 会话保留、前端失败预览阻断、根/CN 同步与 shell smoke，以及对应等级门禁和隔离 Docker 往返。测试结果以当前候选运行证据为准；本契约不宣称未运行的平台或发布验收已经完成。

隔离 Docker 回归入口为 `scripts/verify-backup-native.sh`，要求 Linux root、unshare、Docker/Containerd、Go 和已缓存依赖。通过 `KPB_NATIVE_SOURCE`、`KPB_NATIVE_GO`、`KPB_NATIVE_MODCACHE` 指定只读来源；脚本创建独立 mount/network namespace、两套临时 Docker data-root 和离线夹具镜像，不连接现有 Docker daemon。普通 `go test` 默认跳过该实机夹具。
