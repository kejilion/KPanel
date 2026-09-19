# 管理员密码恢复

KPanel 不保存明文密码，也不通过邮件、短信或公开 HTTP 接口恢复管理员凭据。忘记密码时，必须先通过 SSH 获得服务器 root 权限，再在本机使用 `/paneld reset-password` 离线重置。

## 应用市场安装

在服务器上复制并执行以下单条命令：

```sh
cd /home/docker/kpanel && sh -c 'set -e; cleanup() { status=$?; trap - EXIT; docker compose --env-file .env up -d panel; exit "$status"; }; trap cleanup EXIT; docker compose --env-file .env stop panel; docker compose --env-file .env run --rm --no-deps panel reset-password'
```

命令会停止正在运行的 Panel，以交互式终端隐藏输入两遍新密码，并在成功、失败或中断退出时尝试重新启动 Panel。密码不能通过命令参数或环境变量传入。

如果面板存在多个管理员，可在 `reset-password` 后添加：

```text
--username 管理员用户名
```

当前密码恢复默认保留 TOTP。只有在身份验证器和恢复码也已丢失时，才显式添加：

```text
--disable-2fa
```

该选项会同时删除现有 TOTP 配置并使全部恢复码失效，之后需要登录设置页重新启用两步验证。

## 自定义 Compose 部署

在 Compose 项目目录依次执行，第二条命令完成交互后再执行第三条：

```sh
docker compose stop panel
docker compose run --rm --no-deps panel reset-password
docker compose up -d panel
```

自定义配置文件可在 `reset-password` 后使用 `--config /absolute/path/config.json`。如果 Panel 尚未停止，状态文件锁会拒绝恢复；不要删除锁文件或在线修改 `panel-state.json`。

## 数据与安全影响

恢复操作会原子完成以下凭据变更，并在变更前写入审计：

- 使用现有 Argon2id 参数保存新密码哈希；
- 撤销该管理员的全部 Session；
- 清除旧的登录限速记录，使新密码可以立即登录；
- 写入 `auth.password.recover` 本地审计事件；
- 默认保留用户名、角色、审计历史、安全入口和 TOTP。

网站、容器、镜像、数据卷、数据库、证书、项目文件、Agent Token 和 AI 数据不在密码恢复的写入范围内。持久化失败时，密码、Session、限速和 TOTP 会一起回滚，不会留下部分成功状态。审计保存在独立的审计库（见 [audit-storage.md](audit-storage.md)），因此恢复事件先于凭据变更写入；凭据提交失败时会追加一条失败结果的审计记录，不会出现凭据已改而没有审计的情况。

恢复结束后建议验证：

```sh
docker compose ps panel
docker compose exec -T panel /paneld healthcheck
```

随后确认旧密码无法登录、新密码可以登录；若保留了 TOTP，登录仍需有效动态验证码或未使用的恢复码。
