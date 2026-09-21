# Passkey 候选修复追踪

两项修复均只存在于本地候选；未发布 RC/stable，未部署，发布状态为 `pending-stable`。
正式审计中断原因与证据边界见 [REPORT.md](REPORT.md)。

| 项目 | 发现来源与旧代码 | 修复 | 证据与当前状态 |
| --- | --- | --- | --- |
| 匿名 Passkey 登录 intent 审计写入未限频 | 负责人 OCR 自由臂，`790fa396`；不是独立 hunter 新发现 | `1659b096`：删除未验证匿名 intent，保留限频 failure 与成功审计失败关闭会话；同时使用 `__Host-` 挑战 Cookie | 独立源码复核与无效请求日志限频回归已完成；候选修复，未发布 |
| 待确认 TOTP 绑定未随凭证版本变化失效 | 生命周期补查，`1659b096`；fingerprint `auth/totp-enrollment-stale-authority-after-credential-revocation` | `54650283`：绑定创建时记录版本，`EnableUserTOTP` 在存储锁内核对版本后才提交，冲突返回绑定已过期 | 普通开发 HTTP 回归在旧代码失败、修复后通过；覆盖密码、用户名、Passkey 管理、恢复与本机密码恢复的版本变化。正式独立验证与 final-clean 尚未完成 |

测试路径：`internal/panel/totp_lifecycle_regression_test.go`、`internal/auth/passkey_test.go`、
`internal/panel/passkey_test.go`、`internal/store/passkeys_test.go`。
旧代码回归失败证据：仓库外 `totp-stale-before-fix.log`；修复定向测试：`totp-fix-targeted.log`。
这些是本地开发测试，不等同于正式审计沙箱验证。

另外修正了 Schema 2 回滚说明：身份备份恢复不会降低原 Store 的 Schema；退回旧程序需升级前完整备份。
