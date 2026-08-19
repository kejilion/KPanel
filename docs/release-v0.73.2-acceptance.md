# KPanel v0.73.2 上线验收记录

日期：2026-08-15
发布工作流：`release-kpanel` v2.2
结论：通过

## 1. 发布范围

- `6a430afbb87e91caebe58fcc9d4602277218ba33`：经典模式侧栏在 `/ai/s/:sessionId` 等嵌套工作区路由保持分区选中态。
- `da54824c00aeddc10c57f30cf1e93624e69e51e5`：桌面 Agent 更新提示改善浅色主题对比度，并保持深色主题原有视觉层级。
- `3508326431fae539a95fa8da9cf5a8b2647bafbc`：准备 `0.73.2` 版本、CHANGELOG 与一致版本元数据。
- 变更仅涉及 Web 展示和导航状态；数据库、API、Agent、Compose、端口、应用市场安装契约及 KPanel 固定 `kejilion.sh` 协议均未改变。

本轮同时独立发布 `kejilion/sh`：

- `73c9090862778050be43c95df01ac1097d4b7296`：新增 DeepSeek Hermes 基础轻量管理器与选项 116。
- `601594897e8c74377331577c0e219a93af1dbfbc`：修复 Gateway 已停止时的状态误报；该提交已快进到 `kejilion/sh main`。
- GitHub 公网 Raw 已重新下载验证：Linux 语法、根/CN 同步、专属 smoke、选项 116 唯一入口均通过。

## 2. 源码、CI 与正式产物

- 发布基线：`origin/main@0eb999c8abaf0d126a330525153fe70066c76f32`
- Release commit：`3508326431fae539a95fa8da9cf5a8b2647bafbc`
- Annotated tag：`v0.73.2`，tag object `09518906a0dbb3e660fcc59275628da793890151`
- 候选 CI：`31862420325`，成功；候选依赖治理：`31862420244`，成功。
- main CI：`31862602117`，成功；main 依赖治理：`31862602116`，成功。
- Release workflow：`31862770310`，成功。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.73.2>，非 draft、非 prerelease，共 8 个附件。
- OCI index（`0.73.2` 与 `latest`）：`sha256:e0e567416b9b31b6541d2363dc0785714b65b0fcf3a0458b65592e641b815464`
- `linux/amd64`：`sha256:c7e48ae51e9b9616f586e6d552fdb1acd0865ac6bdbabcdfc6ac6bf6eae37fc8`
- `linux/arm64`：`sha256:d445cc930f48b4dc064defd985eb2e15b7d9b1bf529697ab9e8a59175e52bb72`
- 两个平台的 OCI version 为 `0.73.2`，revision 为 Release commit。
- `kejilion/apps main@c5cc79ce4acd7f7b373573952616dbabd2060b7d` 的 `kpanel.conf` Git blob 与候选完全一致；固定容器内 lifecycle 通过，无需制造 apps 空提交。
- 公网 OCI E2E：Panel/Agent 真链路正常，容器 healthy、0 重启、无 OOM。

## 3. 自动化与隔离真机证据

- 本地 Web：96 个测试文件、706 项测试通过；i18n 2177 条、TypeScript、production build 通过。
- 154 L3：Go 全量、核心包 race、vet、govulncheck、npm audit、Trivy 源码/镜像、双架构构建、安装安全和应用配置 lifecycle 全部通过。
- L3 日志 SHA-256：`363f880d1663a90e133307c6f42f211d2df7c40ce7b0a029c7ab5abe30feea69`。
- 隔离候选镜像：`sha256:df2958252c0f29853377f4f2729b5a04fe43bfb8229feca0d5da831e8bf1cc33`，version/revision 精确匹配。
- 正式 Google Chrome `151.0.7922.138`：嵌套 AI 路由选中态、桌面更新提示浅/深主题均通过；页面错误 0；候选 healthy、0 重启、无 OOM。
- 浏览器证据摘要：`4ff09c627a25860acf8d6cd1d25b600fe0400470891ecd677fe88ba724cd1f15`。
- 隔离回滚：候选切换到 v0.73.1 后健康，再恢复 v0.73.2；排除 SQLite WAL/SHM 后持久文件摘要一致。
- 本地证据目录：`C:\GitHub\_release-artifacts\v0.73.2`；154 L3：`/root/kpanel-release-v0732-3508326`。

## 4. arena-154 正式升级

- 部署前：v0.73.1，Panel healthy、0 重启、无 OOM，Agent active。
- 停写一致性备份：`/root/kpanel-backups/v0.73.2-preupgrade-arena154-20260815T035951Z.tar.gz`
- 备份 SHA-256：`b1858895b2f31311fb6ef86c924f81c051f1a1099d92d1322bfd168936e0a3e1`
- v0.73.1 镜像归档：`/root/kpanel-backups/v0.73.2-preupgrade-arena154-20260815T035951Z.image.tar`
- 镜像归档 SHA-256：`812b08c33b7f9c7df10ea377c8b9ad027fd0d74dc039c1a00f52fae0546e3d64`
- 独立恢复目录已校验文件摘要、权限/属主/链接清单和镜像归档可读性后删除。
- 标准应用市场更新入口拉取正式 `latest`；升级后 v0.73.2，OCI digest/revision 精确匹配。
- Panel healthy、0 重启、无 OOM；Agent active；Compose 可解析；数据库 quick check 与 JSON 数据校验通过；fatal 日志为空；三次短健康采样全部通过。
- 生产证据：`/root/kpanel-release-evidence/v0.73.2/arena154-20260815T035951Z`；本地脱敏副本位于发布证据目录。

## 5. 环境边界与回滚

- `prod-108`/`108` 已禁用全部 KPanel 操作；本轮未连接、未检查、未备份、未部署、未清理。
- 回滚点：源码/tag `v0.73.1`，公开 OCI `sha256:d0e025b13e75de329e9a8e950459b7d8940e687603471ce0f92fb731ec23cddc`。
- 生产回滚必须停服务并成套恢复本记录中的 v0.73.1 镜像归档、Compose、`.env`、Agent unit/二进制及数据备份，随后核对版本、健康和数据完整性。

## 6. 发布节奏与遗留风险

- 候选冻结：2026-08-15 11:26:43 +08:00；L3 完成：11:38:06；正式健康确认：12:00:39。
- 候选到正式确认约 34 分钟；未发生生产回滚、紧急修复或重复发布。
- 两项 KPanel 变更均为低风险展示修复，生产不需要长时间 soak；真实浏览器和三次短健康采样覆盖了受影响路径。
- Hermes 真机安装/更新/卸载等写路径证据来自独立开发验收；本发布任务仅重新验证源码与公网 Raw，不在 KPanel 生产环境重复安装，也未移除开发验收遗留的通用系统依赖。
- 发布后按用户要求保持本机唤醒，不执行睡眠。

### 结构化交付节奏数据

以下字段于 2026-08-15 根据本记录已有时间、提交元数据和生产结论补充，只改善机器读取，不新增或改变生产事实。

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-15T11:21:11+08:00
- 候选冻结时间：2026-08-15T11:26:43+08:00
- 生产完成时间：2026-08-15T12:00:39+08:00
- 提交到生产用时：0.66 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->
