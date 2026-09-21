# Passkey scoped audit — needs validation

本轮没有 source-grounded `needs_validation` finding。以下为环境层限制，不能作为漏洞或严重度结论：

- 真实反向代理必须确认只向受信 Panel 转发规范化 HTTPS scheme/host，且不让公网客户端伪造 `X-Forwarded-*`。
- 登记隔离验收环境需确认真实 HTTPS 域名、浏览器平台、硬件认证器和同步提供商的兼容性。
- 本轮未执行目标代码；任何动态行为结论需在符合审计技能要求的 OS 隔离沙箱中重新验证。
