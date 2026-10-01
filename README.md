<h1 align="center">
  <a href="https://kpanel.kejilion.sh/">
    <img src=".github/assets/kpanel-logo.png" alt="KPanel" width="520">
  </a>
</h1>

<p align="center">
  <strong>把服务器，变成你的运维工作台。</strong>
</p>

<p align="center">
  与 <code>kejilion.sh</code> 双向互通的开源 Linux 运维面板，支持双模式工作台、多主机管理与 AI 助手。
</p>

<p align="center">
  An open-source Linux server management panel with two-way <code>kejilion.sh</code> integration,
  desktop and classic workspaces, multi-host management, and an AI assistant.
</p>

<p align="center">
  <a href="https://github.com/kejilion/KPanel/releases/latest"><img src="https://img.shields.io/github/v/release/kejilion/KPanel?display_name=tag" alt="Latest release"></a>
  <a href="https://github.com/kejilion/KPanel/actions/workflows/ci.yml"><img src="https://github.com/kejilion/KPanel/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/github/go-mod/go-version/kejilion/KPanel" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0--only-blue.svg" alt="AGPL-3.0-only"></a>
</p>

<p align="center">
  <a href="https://kpanel.kejilion.sh/"><strong>产品官网</strong></a> ·
  <a href="https://panel.kejilion.pro/">在线体验</a> ·
  <a href="https://kpanel.kejilion.sh/en/">English website</a> ·
  <a href="#快速开始">开始部署</a> ·
  <a href="https://blog.kejilion.pro/kpanel-kejilion-web-server-panel/">图文教程</a> ·
  <a href="https://github.com/kejilion/KPanel/releases">版本发布</a>
</p>

<p align="center">
  <a href=".github/assets/readme/kpanel-desktop-hero-ai.webp">
    <img src=".github/assets/readme/kpanel-desktop-hero-ai.webp" alt="基于最新截图经 AI 合成的 KPanel 工作台主视觉，以大桌面搭配经典模式应用市场；实际界面见下方截图" width="100%">
  </a>
</p>

KPanel 面向单管理员 Linux 服务器场景：从一台主机开始，在需要时连接更多节点。它不把资源锁进面板私有数据库；
`kejilion.sh`、SSH、Compose 与 KPanel 始终面向同一台主机的真实状态。

在线体验在浏览器内模拟数据与操作，不连接真实主机。下方截图包含稳定版与预览版本；
具体功能以安装版本、节点能力和授权为准，版本区别见[稳定版与预览版](docs/release-channels.md)。

<table>
  <tr>
    <td width="50%">
      <strong>一套能力，两种工作方式</strong><br>
      桌面模式适合多窗口协作，经典模式适合快速巡检；切换的是界面，不是系统。
    </td>
    <td width="50%">
      <strong>真实主机，不造第二套事实</strong><br>
      系统、Nginx、Docker 与文件本身就是事实来源，既有资源无需导入即可继续管理。
    </td>
  </tr>
  <tr>
    <td width="50%">
      <strong>AI 是工作窗口</strong><br>
      多 Provider、多模型与结构化工具直接使用 KPanel 已掌握的状态，默认人工确认，高风险操作始终确认。
    </td>
    <td width="50%">
      <strong>从单机到多节点</strong><br>
      连接已有 KPanel 或无需 Docker 的轻量 Linux 节点，统一监控，并按节点能力与授权使用远程文件和终端。
    </td>
  </tr>
</table>

## 一套能力，两种工作方式

桌面模式保留拖拽、最小化、最大化、任务栏与多窗口；经典模式提供熟悉的侧栏、结构化表单和高密度信息。
两者共享同一账户、同一 Agent 与同一主机状态，无需迁移任何资源。

<table>
  <tr>
    <th width="50%">桌面模式 · 把复杂任务摊开来做</th>
    <th width="50%">经典模式 · 保持面板应有的直接</th>
  </tr>
  <tr>
    <td>监控、终端、集群和 AI 助手可以并行展开，适合排障与跨模块操作。</td>
    <td>清晰侧栏、连续配置和快速巡检，保留传统服务器面板的效率。</td>
  </tr>
  <tr>
    <td><a href=".github/assets/readme/kpanel-desktop-workspace.webp"><img src=".github/assets/readme/kpanel-desktop-workspace.webp" alt="KPanel 桌面模式中的文件管理器、OpenCode 终端与 AI 助手"></a></td>
    <td><a href=".github/assets/screenshots/overview.webp"><img src=".github/assets/screenshots/overview.webp" alt="KPanel 经典模式服务器概览"></a></td>
  </tr>
</table>

## AI 是工作窗口，不是第二套控制面

KPanel 的轻量 AI 助手直接理解面板已经掌握的主机与容器状态，通过固定、结构化的 KPanel 工具完成查询、分析和建议，
不提供任意宿主机 Shell 或通用联网工具。

- 支持 OpenAI-compatible、Anthropic 与 Gemini Provider，以及多模型、多会话。
- 会话默认使用人工审批；可选安全自动审批，高风险动作始终需要确认。
- 工具过程、关键变更与结果可审计；主机资源仍由 KPanel Agent 实时读取和统一管理。
- AI 数据独立保存，不会成为 Docker、Nginx、系统配置或文件的第二份事实来源。

## 集群管理：分散的主机，在一处掌握

从一台主机到多台服务器，将节点接入、状态与历史监控、文件管理和终端操作汇集在同一个工作台。
通过消息通知掌握关键变化，也可以生成公开只读页面，分享主机运行状态。

<p align="center">
  <a href=".github/assets/readme/kpanel-cluster-globe-history.webp">
    <img src=".github/assets/readme/kpanel-cluster-globe-history.webp" alt="KPanel 地球集群视图与历史监控并排显示，呈现节点分布、线路延迟和可用性" width="100%">
  </a>
</p>

| 从接入到日常管理 | 可以做什么 |
| --- | --- |
| **接入与总览** | 使用授权码配对已有 KPanel，或为普通 Linux 主机安装轻量节点；支持轻量节点单台和批量接入。 |
| **全局与历史监控** | 在列表、卡片和地球视图间切换，查看在线状态与资源使用；选择目标主机，回看历史指标、线路延迟和 Ping / TCP / HTTP 服务检测。 |
| **文件与终端管理** | 选择目标主机浏览、编辑、上传和下载文件；管理多主机会话、快捷命令和批量执行。已授权的 KPanel 间还可复制文件与目录。 |
| **消息通知** | 接收资源阈值、流量、主机失联与恢复、SSH 登录等通知；支持 Telegram、飞书、钉钉和企业微信，同时启用一个通知渠道。 |
| **公开状态分享** | 管理员主动开启公开只读页面，展示当前运行状态；不公开 IP 或管理入口，可随时关闭或重置链接。 |

终端与文件操作按节点版本、能力和授权开放；旧配对不会因界面升级自动获得新权限。
打开远端面板时，目标面板仍独立完成登录。

<details>
<summary>查看多主机文件与终端工作区</summary>

<p align="center">
  <a href=".github/assets/readme/kpanel-cluster-files-terminal.webp">
    <img src=".github/assets/readme/kpanel-cluster-files-terminal.webp" alt="KPanel 文件管理器的主机选择列表，与多主机终端和快捷命令并排显示" width="100%">
  </a>
</p>

</details>

更多界面见[官网集群介绍](https://kpanel.kejilion.sh/#cluster)；接入与权限见[集群文档](docs/cluster-monitoring.md)。

## 核心能力

| 场景 | KPanel 提供什么 |
| --- | --- |
| **主机与历史监控** | 查看 CPU、内存、磁盘、负载、网络、连接与容器历史；查看实时进程，并管理主机名、SSH、DNS、时区、Swap、软件源、内核预设、系统更新与清理。 |
| **网站与 Nginx** | 发现已有站点、证书与真实 Nginx 配置；管理静态站、PHP 站、反向代理、负载均衡、域名重定向及 LDNMP 环境。 |
| **Docker 与应用** | 管理容器、镜像、网络、卷、日志、性能、更新、备份和迁移；支持 Compose 项目的配置发现、启停与受控编辑重部署。应用市场动态对齐 `app.kejilion.sh`，展示真实安装状态与任务进度。 |
| **文件、终端与体检** | 按节点能力与授权切换本机和远程文件、终端工作区；本机另提供进程管理、系统设置及网络、硬件和综合体检。 |
| **集群与轻量节点** | 统一接入、监控和管理多台主机，按节点能力开放远程文件与终端；提供消息通知和公开只读状态分享。 |
| **AI、任务与审计** | 多 Provider、多模型、多会话的 AI 工作区；固定工具、审批边界、后台任务、资源版本冲突检测、审计与失败恢复。 |

## 快速开始

准备一台 Linux 服务器，使用 `root` 用户执行：

```bash
bash <(curl -fsSL https://kejilion.sh) app kpanel
```

安装脚本会检查运行环境、准备所需组件并部署 KPanel。完成后，根据终端提示打开面板并初始化管理员账户。

| 项目 | 当前范围 |
| --- | --- |
| 已实机验证 | Debian 13 · AMD64 |
| 发布架构 | AMD64 · ARM64 |
| 运行基础 | systemd 或 OpenRC · rootful Docker Engine · Docker Compose v2 |

面板本体目前仅支持 Docker 部署；Release 中的 `kejilion-panel-meta-<version>.tar.gz`
只提供部署脚本、文档和许可等元数据，不是可构建源码包。Agent 与轻量节点可原生运行。

Debian 12、Ubuntu 22.04/24.04、Rocky/AlmaLinux/CentOS Stream/RHEL/Oracle Linux/Fedora、
Arch/Manjaro 与 openSUSE/SLES 路径已经实现，仍按支持矩阵逐步完成实机准入；
Alpine Linux 3.24 `sys` mode 的 OpenRC 路径也已实现并通过自动化契约测试，但尚未完成
干净 VPS/VM 的 PID 1 实机准入。准确状态请以[宿主机系统兼容矩阵](docs/platform-support.md)为准。

> [!TIP]
> 初次使用可阅读 [KPanel 官方图文教程](https://blog.kejilion.pro/kpanel-kejilion-web-server-panel/)；
> 需要审查构建产物、固定镜像 digest 或进行开发者部署时，请使用[完整部署文档](docs/deployment.md)。

> [!IMPORTANT]
> KPanel 具备宿主机管理能力。请只在可信服务器上使用官方部署入口；生产环境请先备份现有配置，
> 并为管理入口配置 HTTPS 与访问控制。

## 管理真实主机，边界也必须真实

<p align="center">
  <code>kejilion.sh</code> · <code>SSH</code> · <code>Compose</code> · <code>KPanel</code><br>
  <strong>共同管理同一台 Linux 主机的真实状态</strong>
</p>

- **不接管既有环境**：安装器不会修改 `kejilion.sh`、`/home/web`、Nginx、防火墙或现有站点。
- **来源不限制管理**：脚本、KPanel、Compose 或人工创建的资源，都可以按实际状态继续管理。
- **权限分层**：Web/API 以非特权身份运行；宿主机操作由 Unix Socket 上的结构化 Agent 执行，root PTY 使用独立授权和有界生命周期。
- **入口保护**：登录限速、服务端 Session、CSRF、可选 TOTP / Passkey、路径约束与供应链校验共同保护管理入口。
- **变更可恢复**：关键操作保留审计记录；配置写入前执行校验，失败时回滚并报告未完成的清理项。

完整原则见 [PROJECT_RULES.md](PROJECT_RULES.md)、[架构与事实来源](docs/architecture.md)、
[生态互通基线](docs/ecosystem-parity.md)与[操作边界审计](docs/operational-boundary-audit.md)。

## 运维，也可以有自己的色彩

选择内置壁纸或上传自己的图片，让桌面、经典模式与登录页延续喜欢的背景和配色。
桌面模式还支持按需下载、随时删除的 3D 场景包。

<table>
  <tr>
    <th width="50%">霓虹都市</th>
    <th width="50%">星港轨道</th>
  </tr>
  <tr>
    <td><a href=".github/assets/readme/kpanel-theme-neon.webp"><img src=".github/assets/readme/kpanel-theme-neon.webp" alt="KPanel 霓虹都市 3D 桌面，展示夕阳城市、文件分组和服务器监控"></a></td>
    <td><a href=".github/assets/readme/kpanel-theme-orbit.webp"><img src=".github/assets/readme/kpanel-theme-orbit.webp" alt="KPanel 星港轨道 3D 桌面，展示地球、空间站和服务器监控"></a></td>
  </tr>
</table>

浏览[官网多彩主题](https://kpanel.kejilion.sh/#themes)，或阅读[场景包开发规范](scene-packs/README.md)。

## 当前边界

已支持的 Compose 项目编辑重部署有明确的项目与配置范围。
`daemon.json` 通用编辑器、系统重装非交互适配器，以及部分发行版的 DNS / 换源适配器仍在规划中。
这些是待实现能力；后续实现仍需经过鉴权、结构化输入、路径约束、并发控制、审计和失败恢复。

## 文档

| 主题 | 入口 |
| --- | --- |
| 开始使用 | [产品官网](https://kpanel.kejilion.sh/) · [在线体验](https://panel.kejilion.pro/) · [图文教程](https://blog.kejilion.pro/kpanel-kejilion-web-server-panel/) · [部署文档](docs/deployment.md) · [平台支持](docs/platform-support.md) |
| 产品架构 | [架构与事实来源](docs/architecture.md) · [安全模型](docs/security-model.md) · [存储策略](docs/storage-strategy.md) |
| 核心能力 | [AI 工作区](docs/ai-workspace.md) · [集群监控](docs/cluster-monitoring.md) · [Docker 管理](docs/docker-management-v0.18.md) · [应用市场](docs/application-market.md) |
| 多主机管理 | [多主机终端](docs/multi-host-terminal.md) · [跨面板文件传输](docs/cross-kpanel-file-transfer.md) · [集群通知](docs/cluster-notifications.md) · [公开状态分享](docs/cluster-public-share.md) |
| 生态与质量 | [兼容基线](docs/compatibility.md) · [开发质量标准](docs/development-quality-standard.md) · [项目协作](docs/session-collaboration.md) |
| 版本信息 | [稳定版与预览版](docs/release-channels.md) · [更新记录](CHANGELOG.md) · [最新稳定版](https://github.com/kejilion/KPanel/releases/latest) |

<details>
<summary>更多安全、设计与工程文档</summary>

- [两步验证安全契约](docs/two-factor-authentication.md)
- [管理员密码恢复](docs/password-recovery.md)
- [多语言架构与本地化契约](docs/internationalization.md)
- [体检与第三方测试协议](docs/diagnostics.md)
- [网站业务分析](docs/legacy-site-contract.md)

</details>

## 开源许可

KPanel 源代码采用 [GNU Affero General Public License v3.0 only](LICENSE)（SPDX：`AGPL-3.0-only`）。
通过网络向用户提供修改版 KPanel 服务时，应按该协议向这些用户提供对应源码。

Copyright © 2026 kejilion and KPanel contributors.

随 KPanel 分发的 `kejilion.sh` 和其他第三方组件继续使用各自的原始许可，详见[第三方许可声明](THIRD_PARTY_NOTICES.md)。
KPanel 名称和 Logo 的使用边界见[品牌说明](TRADEMARKS.md)。
