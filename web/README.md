# KPanel Web

Vue 3 + TypeScript + Vite 实现的 KPanel 前端。所有数据均来自同源 `/api/v1`，不包含 mock 数据，也不会直接执行 Shell 或访问 Docker Socket。

## 本地开发

```bash
npm install
npm run dev
```

跨会话功能预览不要直接约定固定端口，统一执行项目根目录的
`scripts/local-feature-preview.mjs` 和 `.codex-workflows/local-feature-preview.workflow.yaml`；完整分层、
预览卡和停止规则见 `docs/local-feature-preview-standard.md`。`npm run dev` 保留用于单会话手动调试。

开发服务器默认将 `/api` 代理到 `http://127.0.0.1:8080`。如需覆盖：

```bash
VITE_DEV_API_TARGET=http://127.0.0.1:9000 npm run dev
```

WSL 中的 KPanel 使用独立端口时，可在 PowerShell 中启用目标 Host 改写：

```powershell
$env:VITE_DEV_API_TARGET='http://127.0.0.1:8866'
$env:VITE_DEV_API_CHANGE_ORIGIN='true'
npm run dev
```

生产构建：

```bash
npm run typecheck
npm test
npm run build
```

静态产物输出到 `dist/`，可由 `paneld` 嵌入或直接提供。

## 攻击面约定

- 认证使用服务端 Session 与 `HttpOnly` Cookie。
- CSRF Token 仅保存在运行内存，并随所有写请求通过 `X-CSRF-Token` 发送。
- API 请求固定使用 `credentials: same-origin`，不把 Token 写入 `localStorage`。
- Agent 离线或协议不兼容时，写入口因技术依赖不可用而禁用，并显示真实原因。
- `capabilities` 表示宿主机适配器是否存在；资源 `allowedActions` 只反映实时状态，
  不得根据来源、label、marker 或危险配置缩减动作。
- 日志以纯文本插值呈现，不使用 `v-html`。

## 页面

- `/setup`：一次性凭据初始化。
- `/login`：安全登录。
- `/overview`：系统资源与 Agent 状态。
- `/sites`：网站产物、一致性与证书。
- `/docker`：Docker 环境、全部容器、镜像、网络和卷管理。
- `/jobs`：由持久化审计意图与结果聚合出的同步变更记录。
- `/audit`：脱敏审计记录。
- `/settings`：账户、主题和 Agent 能力信息。
