# 安全入口与全盘文件管理设计

## 目标

- 提供可选的登录安全入口，降低公网扫描器直接发现登录页的概率。
- 文件管理覆盖宿主机 `/`，满足 `/home`、`/root`、`/etc`、`/var` 等日常运维。
- 保持 Panel 进程非特权；所有宿主机文件操作仍由 root Agent 的固定类型 API 执行。

## 安全入口

- 升级安装默认关闭，避免已有入口在升级后失效。
- 设置保存在现有 `panel-state.json`，不增加端口、环境变量或部署参数。
- 管理员可在设置中启用、复制、自定义或重新生成入口（6–48 位小写字母、数字和连字符）。
- 访问正确入口后写入 12 小时、`HttpOnly`、`SameSite=Strict` 的短期 Cookie，再跳转登录页。
- 未认证且没有有效入口 Cookie 时，登录接口与 SPA 页面统一返回 404；健康检查、集群协议和静态资源不受影响。
- 已登录会话始终可正常访问；更换入口会立即使旧入口 Cookie 失效。
- 安全入口仅用于减少扫描噪声，不替代密码、会话、CSRF、Origin、限速和审计。
- 已配对 KPanel 可在控制端明确声明兼容能力后，通过既有 HTTPS/Noise 集群摘要读取当前入口路径，
  仅用于“打开面板”跳转；旧版本或未声明能力的控制端不会收到该字段，升级不要求重新配对。

## 全盘文件管理

- 文件根目录改为宿主机 `/`，前端默认仍定位 `/home`，并提供 `/`、`/home`、`/root`、`/etc`、`/var` 快捷入口。
- KPanel 凭据、状态和运行目录不对文件 API 开放：
  - `/var/lib/kejilion-panel`
  - `/run/kejilion-panel`
  - `/etc/kejilion-panel`
  - `/home/docker/kpanel/secrets`
  - `/home/docker/kpanel/data/panel`
  - `/home/docker/kpanel/data/agent`
  - `/home/docker/kpanel/run`
- `/proc`、`/sys`、`/dev` 可查看目录，但禁止通过通用文件 API 写入；这些是内核虚拟文件系统，不属于普通文件管理。
- 路径逐级 `Lstat`，禁止通过符号链接越界；拒绝 `..`、反斜杠、NUL 和超长路径。
- 写入继续使用资源版本校验、同目录临时文件、`fsync` 与原子替换，并保留原权限、所有者和 Linux 扩展属性（含 SELinux 标签）。
- 上传、下载、批量、复制大小和并发维持现有限额；全部写操作继续记录审计。
- 浏览器下载前通过已认证的 `POST /api/v1/files/download-tickets` 获取 5 分钟内有效的内存 ticket；下载地址仅携带 256-bit 随机凭证，使手机系统下载器无需共享浏览器 Cookie。ticket 不持久化，业务日志和审计不得记录其明文，服务重启后自动失效，全局最多保留 128 个。
- ticket 下载仅接受 `GET`、`HEAD` 和既有 `Range` 条件请求；实际文件读取仍经过 Agent 的保护路径、符号链接、权限、并发和流式传输边界。安全入口只放行格式有效的 ticket 路由，伪造或过期凭证统一返回 404。
- Panel 容器不挂载 `/`，仅 Agent systemd 服务获得固定文件 API 所需的宿主机写权限。

## 验收基线

- 安全入口关闭时升级前后访问行为一致。
- 开启后错误路径、直接 `/login` 和登录 API 不暴露登录面；正确入口和已有会话可用。
- `/root`、`/etc` 的普通文件可创建、编辑、复制、移动和删除。
- KPanel 密钥/状态路径、符号链接逃逸、路径穿越和内核虚拟目录写入均被拒绝。
- 文件列表、流式上传下载和历史业务的资源限制不回退。
- 开启安全入口后，无 Cookie 的有效 ticket 可下载并支持 `HEAD`、`Range` 和重试；伪造、过期 ticket 不调用 Agent。
- 执行 Go 全量测试、前端测试与构建、安装安全测试和 L2 验证。

## 恢复与回滚

- 忘记入口时，可从已登录浏览器进入设置复制；所有会话都失效时，由 root 在停止 Panel 后备份并修正 `panel-state.json` 的 `securityEntrance` 设置，再启动 Panel。
- 回滚旧版 Panel 会忽略新增的可选设置字段；用户文件和回收站不迁移、不删除。
- 回滚旧版 Agent 后文件管理范围恢复为 `/home`，不会修改 `/root`、`/etc` 或其他宿主机文件。

## 2026-07-31 本地验收记录

- Go：`internal/store`、`internal/filemanager`、`internal/agent` 全量通过；Panel 认证、安全入口与密码回归通过；相关包 `go vet` 通过。
- 前端：26 个测试文件、174 个用例通过；TypeScript 检查与生产构建通过。
- 构建：Linux `amd64`、`arm64` 的 Panel/Agent 交叉构建通过；Linux 扩展属性测试已完成编译校验。
- 部署脚本：Shell 语法检查通过；完整 `install-safety.sh` 需要具备 `groupadd`、systemd 等能力的 Linux 验收机执行。
- 当前 Windows 环境的全仓 Go 测试仍包含项目既有的 Linux 专属/路径语义失败，未将其误报为本功能通过；合并前须由 Linux CI 完成 L2。
