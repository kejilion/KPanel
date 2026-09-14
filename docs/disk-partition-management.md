# 磁盘与分区管理

KPanel 的磁盘管理以宿主机真实块设备状态为准，不在数据库中维护第二份拓扑。
页面借鉴 `kejilion.sh` 现有的挂载、卸载、格式化和文件系统检查能力，但不复用其交互菜单，
也不允许浏览器传入 Shell、设备路径或任意命令。

## 首版范围

已接入的固定动作：

- 展示磁盘、分区、loop、device-mapper、LVM、加密映射和 Linux Software RAID 拓扑；
- 挂载已有文件系统，可选择是否写入 `defaults,nofail` 开机挂载配置；
- 普通卸载，可选择是否精确移除对应开机挂载配置；
- 格式化叶子块设备为 ext4、XFS、NTFS 或 FAT32；
- 对未挂载的 ext4、XFS、NTFS 或 FAT32 执行只读检查或自动修复。

首版不创建、删除或调整分区表，不调整分区大小，也不编排 LVM、RAID 或 LUKS。
这些设备仍会完整展示，但只有满足安全前置条件的叶子文件系统开放动作。

## 信任边界

```text
Browser
  -> Panel: session + Origin + CSRF + typed JSON + audit
  -> Agent: fixed Unix-socket routes
  -> init-managed worker: systemd transient unit 或 OpenRC 独立进程组
  -> trusted kejilion.sh protocol
  -> util-linux / filesystem-specific tools
  -> fresh inventory readback
```

常驻 Agent 继续使用 `PrivateDevices=yes`，其 system call filter 不增加 `@mount`。
只有固定的 inspect/action transient unit 能看到宿主机设备：

- inspect worker 没有 Linux capabilities，只读取 `/dev`、`/sys`、`/proc`、`fstab` 和挂载状态；
- action worker 使用 `DevicePolicy=closed`，只放行提交前已绑定的目标块设备；
- action worker 只保留挂载和受控文件写入所需能力，不开放网络地址族或通用命令入口；
- worker 重新读取并比对 opaque device ID、`MAJ:MIN`、设备路径、保护状态、操作前置条件和整表资源版本。

`deviceId` 和 `resourceVersion` 都是 64 位小写十六进制摘要。浏览器只提交 opaque ID；
真实 `/dev/...` 路径和 `MAJ:MIN` 只在 Agent 的 root-only 状态中绑定，不能由请求替换。

## HTTP 契约

| 层 | 方法与路径 | 能力 |
| --- | --- | --- |
| Panel | `GET /api/v1/system/disk-partitions` | `system.disk-partitions.read` |
| Panel | `POST /api/v1/system/disk-partition-actions` | `system.disk-partitions.write` |
| Agent | `GET /v1/system/disk-partitions` | 固定本机读取 |
| Agent | `POST /v1/system/disk-partition-actions` | 固定后台动作 |

写入只接受以下字段：

- 通用：`action`、`deviceId`、`expectedResourceVersion`；
- mount：`mountPoint`、可选 `persist`；
- unmount：`mountPoint`、可选 `removePersistence`；
- format：`filesystem`；
- check/repair：无其他字段。

未知字段、重复字段、尾随 JSON、不适用于当前动作的字段、非规范绝对路径和不支持的枚举均被拒绝。
资源版本或设备身份变化返回冲突，客户端必须刷新后重新确认，不能自动重放特权写入。

## 保护与前置条件

以下状态默认保护设备及其上层设备：

- 承载 `/`、`/boot`、`/boot/efi`、`/home`、`/home/docker` 或 KPanel 状态目录；
- 当前作为 Swap 使用；
- 只读设备、存在 holder、包含子设备或处于受保护设备的上层；
- 受保护路径下的挂载点；
- 无法可靠识别拓扑、挂载状态或设备身份。

格式化和文件系统检查只允许未挂载、无 holder、无子设备的可写叶子设备。
卸载只使用普通 `umount`；不提供 lazy/force 选项。挂载目标必须满足：

- UTF-8 编码后不超过 4096 字节，是规范 Linux 绝对路径且不与系统保护路径重叠；
- 全路径组件不存在 symlink；
- 已存在目标为空目录，或由 KPanel 在 root-owned、group/world 不可写的父目录下创建；
- KPanel 创建的目录固定为 `root:root 0700`，只在精确 marker 匹配且卸载后为空时清理；
- 目标没有被其他设备占用，同一设备没有挂载在其他目标。

这些规则同时在 Web、Agent 和 Shell 协议中校验。Web 校验用于即时反馈，不能替代 worker 的最终复核。

## `fstab` 事务

持久化优先使用 UUID，其次使用 PARTUUID，不写瞬时 `/dev/sdX` 名称。
写入项固定为：

```text
UUID=<id> <escaped-target> <fstype> defaults,nofail 0 <pass>
```

脚本在 root-only 跨进程锁中执行以下步骤：

1. 读取并限制 `/etc/fstab` 大小和行数，拒绝 symlink；
2. 检查同源、同目标和冲突项；
3. 在同目录生成临时文件并保留原 mode/owner；
4. 使用 `findmnt --verify --tab-file` 校验；
5. 建立 root-only 恢复快照，`fsync` 后原子替换；
6. 再次校验并回读真实挂载状态；
7. 失败时恢复原文件和实时挂载，回滚不完整则标记 `needs_attention` 并保留恢复路径。

恢复快照严格限制在 KPanel 状态目录，最多保留 16 份。普通审计只记录动作、opaque ID/版本前缀、
布尔选项和挂载点摘要，不记录原始 mountpoint、设备路径、serial、label 或恢复路径。

## 后台任务

所有写动作都使用同一个持久化磁盘任务槽，状态为：

- `queued`：请求和目标身份已经持久化，worker 正在启动；
- `running`：正在重新验证、执行或回读；
- `succeeded`：脚本回执和真实状态一致；
- `failed`：动作没有成功，且没有需要人工恢复的部分状态；
- `needs_attention`：完成凭据缺失、回读不一致或回滚不完整，需要人工检查。

浏览器断开不会取消已接受的任务。重新打开弹窗会读取持久化状态并继续轮询。
init manager 的退出状态不能代替业务完成凭据；没有原子回执时不得推断成功。

## 文件系统工具与兼容性

| 能力 | 固定工具 | 缺失时行为 |
| --- | --- | --- |
| 拓扑/挂载 | `lsblk`、`findmnt`、`blkid`、`mount`、`umount` | 对应 capability 或动作禁用 |
| ext4 | `mkfs.ext4`、`e2fsck` | 只禁用缺失的格式化/检查动作 |
| XFS | `mkfs.xfs`、`xfs_repair` | 同上 |
| NTFS | `mkfs.ntfs`（兼容 `mkntfs`）、`ntfsfix` | 同上 |
| FAT32 | `mkfs.vfat`（兼容 `mkfs.fat`）、`fsck.vfat`（兼容 `fsck.fat`） | 同上 |

- 标准目标是使用 systemd 或 OpenRC 的 root Linux 主机；发行版由实际命令能力决定，不按名称猜测支持。
- WSL 2 可以读取真实拓扑，并允许操作独立挂入且未被占用的磁盘；系统盘和 WSL Swap 继续保护。
- WSL 1、容器、非 root Agent、缺少可用 systemd/OpenRC 后台执行器或可信脚本协议时只读降级或直接关闭 capability。
- util-linux 版本差异通过 `MOUNTPOINTS`/`MOUNTPOINT` 两种 `lsblk` 输出适配；未知字段和畸形输出失败关闭。

## 界面原则

入口位于“系统中心 → 基础配置”，不增加概览首页快捷卡片。弹窗先展示容量、挂载数和嵌套拓扑，
再针对当前设备展示一组短动作卡；保护原因和禁用原因始终可见。实时挂载与开机挂载分开表达，
格式化和修复使用独立二次确认，不要求输入固定确认词。

桌面宽屏使用拓扑与检查器双栏；平板改为单栏；手机使用全屏弹窗和底部式确认。
简体中文、繁体中文和英文使用同一组件内响应式短语适配，避免 Teleport 内容脱离页面观察器后退回简体中文。

## 验收与发布顺序

自动化至少覆盖：严格 JSON、路由与认证、Origin/CSRF、审计脱敏、拓扑解析、相同 serial/WWN 去碰撞、
保护传播、工具缺失、未知容量、资源冲突、单任务互斥、状态损坏、无可信回执、回滚失败和
systemd/OpenRC 后台执行契约。

真实 Linux/WSL 验收只使用本轮创建的稀疏 loop image，并在格式化前复核 loop 设备、backing file 和
`MAJ:MIN`。测试必须串行，结束后精确卸载、detach 并对比前后设备/挂载/Swap 快照。

本轮已在 Ubuntu 24.04 WSL2/systemd 255 上用 384 MiB 独立 loop 分区完成 ext4 格式化、只读检查、
修复、挂载、卸载、任务持久化、Agent 重启回读和并发互斥验收。结束后 `/etc/fstab` 哈希不变，
loop、挂载、Swap 和 transient unit 前后快照一致。物理 NVMe/VirtIO、RAID/LVM/dm-crypt、
SELinux/AppArmor、异常断电及 XFS/NTFS/VFAT 真写仍属于发布前扩展测试边界。

发布必须按以下顺序：

1. 先合入并发布含 `KPANEL_DISK_MANAGEMENT_PROTOCOL_VERSION="1"` 的 `kejilion.sh`；
2. 将 KPanel 的可信脚本固定到 `kejilion/sh@9fec61b50cc6ef798dfac1edf11c2ec60ca6b0d1`，LF SHA-256 为 `54ceb0e72c4c342382500fc35da636fa436c484a12c4766fb9c7f806a23ae8fa`；
3. 再构建 KPanel 候选并执行 Linux/WSL、浏览器和回滚验收；
4. 最后才允许部署。

在脚本 pin 更新前，新 KPanel 只能展示磁盘状态，写 capability 必须保持关闭。回滚 KPanel 不会修改
磁盘或 `fstab`；如动作已被接受，应先等待任务进入可信终态，再按任务提供的恢复路径处理未完成事务。
