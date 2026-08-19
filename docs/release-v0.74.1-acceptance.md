# KPanel v0.74.1 上线验收记录

日期：2026-08-15（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.3

候选提交 / 标签：`94a1109b8b991928ce2e3fcd5b74e4ffac85c9c2` / `v0.74.1`

上一稳定版本 / 回滚点：`v0.74.0` / `sha256:7973094ea48a28191cb2a4360f2c7c67a8fc536e0486082d2ff3e2abb3b33b99`

结论：通过

## 发布画像

- 业务域：AI 对话、应用市场动态图标、诊断终端、发布治理。
- 变更面：只读目录与图标缓存、AI 上下文编码、诊断文本输出、治理与部署。
- 受影响用户旅程：DeepSeek 等兼容接口的连续对话；应用市场在线目录新增应用的图标显示与失败回退；诊断终端标题显示。
- 未变化契约：数据库、端口、Compose、Agent 权限、应用安装动作和 `kejilion.sh` 协议均未改变。
- 风险等级及理由：L3；动态图标增加 Agent 外部只读下载和本地缓存，AI 历史消息编码影响跨供应商兼容，正式镜像和生产更新必须完整验证。

## 发布范围与未纳入内容

- 用户可见更新：动态新增应用可显示经安全缓存的官方图标；诊断标题不再产生平台相关多余空行；AI 连续对话不再回放不兼容或推测性推理字段。
- 精确提交清单：治理加固 `5824d84213b40c333136edc1ee8a4206fa9636c7..ec46356b4769592f56b858332c389ef574cd2864`；诊断修复 `e3145b7fd3622b9c13cc5b5b0d9bf927448b98cf`；动态图标 `9f00de6f64945b2de792101965597fed4a136086`；AI 兼容 `15e6bfb1724241a4d634469ed5233d09a760c683`、`70ed7e81d420ddb9ffe7f81ba54ea409dc86a834`；版本冻结 `94a1109b8b991928ce2e3fcd5b74e4ffac85c9c2`。
- 明确未纳入：`feat/cross-kpanel-file-transfer` 工作树中的未提交跨面板文件传输开发；主工作树中已由 `v0.74.0` 等价包含的旧 UI 提交；其他临时预览和验证文件。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go 全包与回归测试；arena-154 真实 Panel→Agent→官方目录→图标缓存链路 | 未使用付费 AI Key 调用真实模型，跨供应商载荷由回归测试覆盖 |
| 网络入侵与供应链安全 | 已验证 | 固定官方 Origin、HTTPS、重定向/大小/类型/像素/并发边界；govulncheck、npm audit、Trivy 源码与镜像均通过 | 外部目录不可达时按设计回退内置目录 |
| 稳定性、失败恢复与兼容 | 已验证 | 96 个 Web 测试文件/706 项、核心包 race、公开镜像 E2E、容器 healthy/restart 0/OOM false、旧镜像归档可加载 | 无长时间 soak；变更为有界只读缓存与编码修复，采用短健康采样 |
| 性能与资源预算 | 已验证 | 图标下载最大 128 KiB、4 worker、4 秒超时、本地复用；生产约 74 MiB/256 MiB、7 PIDs | 不适用长期性能基准，未改变主页面渲染或高频采样路径 |
| 用户体验与可访问性 | 已验证 | 动态 `deepseek-harness` WebP 实际返回；失败保留回退图标；终端换行和 AI 上下文有回归 | 本版无布局、主题、焦点或多语言文案变化，不重复浏览器视觉验收 |
| 数据、配置与迁移 | 已验证 | 无 schema/配置迁移；停写备份、归档校验、Compose 解析、SQLite 完整性检查 | 图标缓存可删除重建，不属于业务数据 |

## 自动门禁

- 定向与全量结果：Go 全包通过；`internal/panel`、`internal/auth`、`internal/dockerx` race 通过；Web 96 文件/706 项通过；i18n 2177、typecheck、生产构建通过。
- `make verify-release`：arena-154 固定 Runner（Go 1.26.6、Node 24、Buildx）最终完整通过；日志 SHA-256 `a510143320ee869f4d38157d812b714754a0d9e70a9bde8c9ccdb0f5ba99463a`。
- 首两次 L3 在源码测试前分别因归档缺少 `.git` 元数据、bundle 未导入稳定 tag refs 被新版治理门禁拒绝；候选 SHA 未变化，补齐完整 Git 引用后从头重跑，未跳过门禁。
- 候选 CI `31882714623`、候选依赖治理 `31882714613`：成功。
- 主线 CI `31883014159`、主线依赖治理 `31883014088`：成功。
- Release workflow `31883231329`、tag 依赖治理 `31883231289`：成功。
- 安全扫描与产物：govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/镜像高危与严重问题 0；正式 OCI revision/version 精确匹配。

## 依赖与技术栈变化

- 本版没有新增或升级 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本。
- 三层 `Dependency freshness` run 均成功，现有固定依赖与例外治理继续有效。
- 受管脚本仍固定 `kejilion/sh@28f89c1b34df4b25e6ef9b144c328fdea75dbac9`，原始 SHA-256 `0583f7cd5be1f0bb6ec48d92e2cf224bfabfafada5788658bda4414ba9561229`。
- 宿主脚本只继承已授权的 `permission_granted`、统计开关和区域参数；除三项外与镜像固定脚本规范化一致。

## 隔离真机与浏览器验收

- 主机/环境：`arena-154`，Linux amd64、Docker；环境策略允许 candidate-validation 与 production-deploy。
- 精确候选：`94a1109b8b991928ce2e3fcd5b74e4ffac85c9c2`；随后用正式公开 OCI 重跑相同链路。
- 真实 Panel/Agent：在线目录 `catalogMode=live`、153 项、6 个动态图标；`deepseek-harness` 经 Agent 下载、验证、以目录 `0700`/文件 `0600` 缓存，并由 Panel 认证同源入口返回 `image/webp`。
- 候选与公开镜像 E2E 均为 `image_e2e=pass`；隔离容器 healthy、0 重启、无 OOM，执行后已清理。
- 本版没有 Web 布局、焦点、主题或响应式变化，真实浏览器视觉测试不适用；没有执行付费 AI 请求。
- 证据：`/root/kpanel-release-evidence/v0.74.1/arena154-20260815T120419Z`，目录 `SHA256SUMS` 已生成；本地证据在 `C:\GitHub\_release-artifacts\v0.74.1`。

## 发布产物与公开仓库复核

- Release commit：`94a1109b8b991928ce2e3fcd5b74e4ffac85c9c2`；annotated tag object：`7ee81c9b2e08c56ebc9b4c277fed93701a051461`。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.74.1>，非 draft、非 prerelease，共 8 个附件。
- OCI index（`0.74.1` 与 `latest`）：`sha256:068c69a4c70c15656535cb78f7c51f8199d66f35391a546030f5157edead8279`。
- `linux/amd64`：`sha256:fe4047c4b0fa63b5f1fdccf37cdfea646c9568ced7e43e4da70c93ef3d65cc84`；`linux/arm64`：`sha256:b78a3d720b64e25da1718ac3afc64e902d56c21afe555553279126a1f5a50651`。
- 附件包含 Agent/Node 双架构二进制、部署包、许可证与 `SHA256SUMS`；公开镜像 `image_e2e=pass`。
- `packaging/kejilion-app/kpanel.conf` 相对 `v0.74.0` 无变化，并与 `kejilion/apps main@c5cc79ce4acd7f7b373573952616dbabd2060b7d` 规范化一致；lifecycle 通过，无 apps 空提交。

## 生产部署安全核对

- 唯一目标与授权：`arena-154`；使用与正式 apps Git blob 相同的 `/root/apps/kpanel.conf` 标准更新入口，没有覆盖 `/root/apps` 本地内容。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未检查、未备份、未部署、未升级、未核对。
- 升级前：v0.74.0、OCI `sha256:7973094ea48a28191cb2a4360f2c7c67a8fc536e0486082d2ff3e2abb3b33b99`，Panel healthy、0 重启、无 OOM，Agent active。
- 停写备份：`/root/kpanel-backups/v0.74.1-preupgrade-arena154-20260815T120419Z.tar.gz`，SHA-256 `284a8938defcf2ed6517b936d937dbc7b9eb815926042f09a635bc0e31fee041`。
- v0.74.0 镜像归档：同名 `.image.tar`，SHA-256 `de6532eaf42347b7aaa99ebc614e6ab2cdc6c344fce459e44334c361a2a753fd`；已实际 `docker load` 并核对 version/revision。
- 升级后：v0.74.1，运行镜像/revision 与公开 OCI 精确匹配；Panel healthy、restart 0、OOM false；Agent active 且版本 `0.74.1 v1alpha1`；Compose、Panel-Agent 健康、固定脚本、SQLite、错误日志和 5 次短采样通过。
- 生产仅执行停写备份、标准 KPanel 更新及部署安全核对；动态图标真实缓存写入只在隔离实例验收，未主动改变生产业务数据。

## 回滚

- 源码/tag：`v0.74.0` / `cf18c94d653c6f692683e4293284d054aff7e6a0`。
- 镜像 digest：`sha256:7973094ea48a28191cb2a4360f2c7c67a8fc536e0486082d2ff3e2abb3b33b99`。
- 数据/配置：上述停写备份；镜像归档已验证可加载。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.74.0 镜像归档，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `daemon-reload`，启动 Agent 与 Compose，再核对版本、Panel-Agent 健康、数据库完整性、restart/OOM 和日志。
- 本次未触发生产回滚；当前公共 GitHub Latest、Docker `latest` 与标准更新入口均指向已通过生产核对的 v0.74.1，公共默认通道无需恢复。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-15T15:47:56+08:00
- 候选冻结时间：2026-08-15T19:26:06+08:00
- 生产完成时间：2026-08-15T20:08:24+08:00
- 提交到生产用时：4.34
- 是否回滚、紧急热修复或重复发布：否（验证包 Git 引用补齐后重跑 L3，不属于版本回滚、热修复或重复发布）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与后续准入

- 外部官方目录或图标不可达、格式越界时会使用内置目录或回退图标；这是已验证的安全降级，不阻断本版。
- 没有可公开使用的付费 AI Key，本版未对真实 DeepSeek 计费接口发请求；兼容消息序列、空助手消息和推理字段不回放由 Go 回归测试覆盖。
- 本轮复用并更新既有 `release-kpanel` v2.3 流程，不新增重复工作流；发布后按用户要求不执行电脑睡眠。
