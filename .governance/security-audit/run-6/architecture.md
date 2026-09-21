# Passkey scoped architecture

本轮是 Cloudflare `security-audit-skill` 的 scoped source-only 审计，只覆盖 KPanel 新增 Passkey 功能及直接影响的认证边界，不是全仓安全审计。审计基线为 `b8ba15f484c64d43241eaa3f2d4060d702b75fbd`，源码树为 `c158c8cf4c7590221a38973737839c856ef5c328`，技能 pin 为 `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`。

KPanel 是单管理员主机控制面板。浏览器匿名方可请求 Passkey 登录挑战并提交 WebAuthn 断言；已认证管理员只有在密码和已启用 TOTP/恢复码再次验证后才能注册或删除 Passkey。保护资源是管理员身份、Passkey 公钥材料、TOTP 因子、会话和审计状态。离线密码恢复属于本机特权流程，身份备份是未信任输入。

Passkey 服务将固定 HTTPS origin、RP ID、UV、discoverable credential、3 分钟单次 ceremony、用途/账户/会话绑定和 credential version 交给 WebAuthn 库与本地 CAS。管理变更撤销会话；登录同时原子更新认证器计数器和新会话。TOTP enrollment 在提交时用 credential-version CAS 拒绝旧授权。备份不携带 Passkey 或会话，恢复推进版本并清除 Passkey。HTTP 层检查 Origin、Session、CSRF、JSON 大小和审计失败关闭；登录挑战使用 `__Host-`、Secure、HttpOnly、Strict cookie。

本轮审阅入口为 `internal/auth/passkey.go`、`internal/auth/service.go`、`internal/auth/totp.go`、`internal/store/passkeys.go`、`internal/store/store.go`、`internal/store/backup.go`、`internal/panel/passkey.go`、`internal/panel/totp.go`、`internal/panel/server.go`、`cmd/paneld/main.go` 及直接前端序列化/会话文件。集群、Agent、文件、终端、MCP、发布和其他无关产品面明确为 out-of-scope。

审计只读取冻结源码，没有执行目标代码、测试、服务、浏览器或网络请求；当前环境不能证明完整 OS 沙箱、目标只读挂载、外网隔离和父侧制品安全提升。故动态运行时、真实反向代理头净化、真实 HTTPS、硬件/浏览器矩阵仍属 needs-validation 限制，不据此构造漏洞。
