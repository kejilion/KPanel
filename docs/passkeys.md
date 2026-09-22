# KPanel Passkey 登录

## 范围与认证策略

Passkey 是管理员主动绑定的可选登录方式，替代密码输入；已启用 TOTP 的账户仍须提供动态验证码或
一次性恢复码，不会因绑定 Passkey 自动解除两步验证。密码登录继续可用。Passkey 不替代 Session、
CSRF、接口鉴权或 Panel/Agent 隔离，不改变集群、MCP、宿主机与 `kejilion.sh` 权限。

设置中的“通行密钥”支持最多 10 个凭证、名称、最后使用时间和撤销。绑定与撤销要求当前密码，
已启用 TOTP 时还须第二因素；完成后全部现有 Session 失效。设备私钥、生物识别数据不交给 Panel。
支持同步型 Passkey 与有用户验证能力的硬件安全密钥；不承诺所有浏览器、密码管理器和硬件组合。

## 部署入口

- 使用固定域名和浏览器信任的 HTTPS。HTTP、IP 字面地址、公共后缀、通配符和不合法域名不能启用。
- 默认使用 HTTPS 域名形式的 `KEJILION_PANEL_PUBLIC_URL`。直连 IP 后通过可信代理添加域名时，设置页会在
  检测到有效的可信 HTTPS 入口后提供“使用当前入口启用”，管理员确认当前密码及已启用的第二因素后即可持久化
  `passkeyOrigin`，无需手工编辑配置文件或重启 Panel。也可使用
  `KEJILION_PANEL_PASSKEY_ORIGIN=https://panel.example.com`（JSON 字段 `passkeyOrigin`）进行服务器侧固定配置。
  面板入口只接受当前可信 HTTPS Origin，不接受用户任意输入的域名；该字段只指定认证来源，不创建反代、证书或扩大原有 Host 允许范围。
- RP ID 为该配置的精确主机名，不扩大到父域；Origin 包含协议、域名及非默认端口。
  不从每次请求的 Host 自动生成允许列表。只信任既有可信代理 CIDR，外部请求必须实际呈现 HTTPS。
- 登录/注册的 HTTP Origin 与签名内 Origin 均校验；禁止跨源 iframe 和关联域名登录。
- 变更域名后旧凭证不能在新 RP ID 使用，先通过密码及原第二因素登录重新绑定，再撤销旧凭证。
  同域名改变端口仍需更新配置。旧域名凭证继续可在设置中撤销。

## 协议与存储边界

服务端使用 `github.com/go-webauthn/webauthn v0.18.2`，以 W3C WebAuthn Level 3 注册/认证校验为依据。
要求 discoverable credential、`userVerification=required`，注册采用 `attestation=none`；不引入远程
metadata 服务或外部登录供应商。使用稳定随机账户 ID 作为 user handle，不使用用户名作为身份主键。

随机挑战由库生成，服务端只在内存保存，3 分钟失效，最多 64 个。注册绑定发起 Session，登录绑定
Secure/HttpOnly/SameSite=Strict 的随机浏览器 Cookie；挑战还绑定用途、账户和凭证安全版本。
完成尝试原子消费，失败也不可重放；重启后全部失效。登录 begin 不公开账户或凭证是否存在。

登录 begin 按来源限制申请次数；finish 共用密码登录的 IP/账户失败预算，管理另有限速且不能通过
反复提交正确密码重置第二因素失败预算。请求体最多 64 KiB，单凭证持久化 JSON 最多 16 KiB。
凭证验证、计数器/最后使用状态和 Session 创建通过版本检查及原子存储防止撤销竞态。设备绑定凭证
计数器倒退会拒绝，同步型凭证的非单调计数不单独作为克隆证据；零计数器合法。UV、签名、备份标志
一致性、RP ID、Origin 和 user handle 始终由服务端校验。

密码/用户名、TOTP 开关、恢复码轮换、Passkey 管理与恢复推进凭证版本，旧挑战随之失效。
TOTP 待确认绑定同样记录发起时的凭证版本，存储提交时原子校验；安全设置变化后须重新开始绑定。
首次保存 Passkey 将 Panel Store Schema 由 1 升至 2；不含 Passkey 的旧 Store 仍可读取。
删除全部凭证不降低 Schema，旧程序会拒绝打开新版 Store，避免静默丢弃认证数据。

## 恢复、备份与回滚

- 建议至少绑定两个独立设备/安全密钥，妥善保存密码和 TOTP 恢复码。
- 现有 `paneld reset-password` 仅在本机停服并取得独占 Store 后执行，重置密码时同时撤销所有
  Session 和 Passkey；TOTP 仍由现有 `--disable-2fa` 选项控制。不存在网页跳过认证的恢复入口。
- Panel 身份备份不导出 Passkey，恢复会清除目标现有 Passkey，并推进凭证版本；使用密码及现有
  TOTP 登录后重新绑定。这样不会从旧备份重新激活已撤销凭证。完整磁盘快照仍可能恢复旧授权，
  其恢复应按离线账户恢复处理，不等价于面板内置身份备份。
- 回退到不支持 Schema 2 的程序前，必须备份当前数据，再恢复升级前的完整备份及其匹配程序。
  在原 Schema 2 Store 中恢复无 Passkey 身份备份不会降级 Schema。禁止手工改 Schema 数字后运行旧程序。

## 审计

注册 begin、注册完成、撤销记录独立 `auth.passkey.*` 动作与 intent/success/failure；
Passkey 登录只记录 success 与限频 failure，匿名输入不能触发无界 intent 写入。
敏感写入前审计不可用即拒绝；登录成功审计失败则撤销新 Session，不发认证 Cookie。凭证管理在
intent 已持久化后变更，success 写失败仍保留 intent（审计库与身份 Store 不具备跨文件事务）。
审计不包含密码、TOTP、恢复码、挑战、签名、Cookie、凭证公钥或用户输入的凭证名称。
本机密码恢复审计增加 `passkeysRevoked` 数量。匿名失败延续既有审计节流，防止日志耗尽。

## 依据与交付边界

- [W3C WebAuthn Level 3](https://www.w3.org/TR/webauthn-3/)
- [go-webauthn v0.18.2](https://github.com/go-webauthn/webauthn/releases/tag/v0.18.2)
- [FIDO Passkeys](https://fidoalliance.org/passkeys/)
- 竞品只作为恢复完整性复核：[AcePanel CLI](https://acepanel.net/quickstart/cli) 提供通行密钥清除，
  本实现复用已有本机账户恢复边界，不增加远程管理协议。

开发范围为 L2 认证/存储/前后端契约，基线 `a5da3d78c24f1862cd918dd8be91fc2b8deb0d74`。
`scriptLinkageState=not-required`：无需发布脚本（不适用），不改变任何脚本协议或外联配置；内置脚本
仍为 `2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256
`806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
本功能不修改 VERSION、发布工作流或生产环境；自动测试、浏览器模拟认证器与真实硬件兼容证据分开报告。
