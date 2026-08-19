# KPanel v0.76.0 上线验收记录

日期：2026-08-16（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`985774771a36037e638e7a87d3be1824223bda52` / `v0.76.0`

上一稳定版本 / 回滚点：`v0.75.0` / `sha256:4a19a1e1624d454d4817b60973ed9ae37b1e7d45164a08a4d9b868a174575724`

结论：通过

## 发布范围

- 桌面真实文件和目录快捷方式可拖到另一个已配对 KPanel；目标桌面在复制成功后创建入口，目标文件窗口复制到当前或指定目录，支持多选、同名不覆盖、部分跳过和持久化恢复。
- 集群页面新增默认关闭的公开分享页；匿名响应只包含固定白名单字段，分享链接可关闭或重置，旧链接立即失效。
- AI 思考模式在一次模型响应包含多批工具调用时保留真实推理上下文，避免兼容接口拒绝后续工具历史，同时继续排除猜测性推理内容。
- 精确业务提交：`d1978517eefe8a9885a84a0511125bf47983cfa6`、`5c42f2c`、`92c9553`；版本冻结提交：`985774771a36037e638e7a87d3be1824223bda52`。
- 未纳入其他旧工作树、未提交草稿、重复补丁或实验内容。`packaging/kejilion-app/kpanel.conf` 与 `kejilion/apps` 正式内容一致，因此没有制造 apps 空提交。

## 自动门禁与隔离验收

- 固定 Linux Runner 从冻结 SHA 完成 `make verify-release`：Go 全包、核心包 race、vet、govulncheck 可达漏洞 0；Web 98 个文件/724 项测试、i18n 2234 条/20 个按页 catalog、typecheck、生产构建；npm audit 0；Trivy 源码和最终镜像高危/严重问题 0；Linux amd64/arm64 构建、安装安全、受管脚本和应用配置生命周期全部通过。
- L3 日志 SHA-256：`d0603e1cf4bc62c1d5a1e30a0e126721b941a5fab0f8ba78cd5639a884f63e1d`；候选 bundle SHA-256：`0d3edc6da1954ae72982cc768ad1e4769632fc592b7ede149afdf61f917727c6`。
- 候选 CI `31895066896`、候选依赖治理 `31895066855`、主线 CI `31895270190`、主线依赖治理 `31895270196` 均成功并绑定精确提交。
- Release workflow `31895627585` 与 tag 依赖治理 `31895627601` 成功；输入、发布说明、供应链、运行镜像契约和双架构发布步骤均通过。
- arena-154 隔离双 Panel/Agent 真链路完成 B→A 文件/目录、A→B 文件、多选元数据、同名不覆盖、stale version、symlink 拒绝、取消清理、重启恢复、桌面入口、公开分享启停/重置及浏览器公开页。
- 正式 Google Chrome `151.0.7922.138` 使用独立临时 Profile 验证真实 `dragstart` 描述符、目标 DOM `dragover/drop`、复制结果、持久化桌面入口与匿名分享页；控制台错误 0。浏览器结果 SHA-256：`0ff639c657ce09dc11fbe835217a4dd9e2f3ac1d42025b14574d20a2f0b0b29e`。
- 隔离汇总 SHA-256：`fe68c6798a8e5470eadd5720fa38de08933c748dfd21393205088c7fc3fb943d`；临时容器、网络、测试数据、隧道与浏览器 Profile 已清理。

## 发布产物

- annotated tag object：`8ad9a7d417c6ce3b10e7f3f069b49c0baabe1ac2`；GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.76.0>，非 draft、非 prerelease。
- OCI index（`0.76.0` 与 `latest`）：`sha256:7cfc37123f870b66e46d20b06387e3f5dc7401a1da7d23ba7f12618fbd9f352d`。
- `linux/amd64`：`sha256:570eadc1a8dde60753f9f7a7d4642877d5f40831de70d73295ecb16e04734c69`；`linux/arm64`：`sha256:f928c9633f63f0f2ecea28bd5cb186099d5d202da31c6debea031845f14deb45`。
- 部署包 SHA-256：`eb3a96ed84ef581d4af5266339efac29a10729848324c0a954d797ff90fc7b30`；`SHA256SUMS` 附件 SHA-256：`72a2a64f7fcae706b150f77bf13c32188f6b56e0d0be6b204d1876bc536d1919`。
- 公开 OCI 在 arena-154 拉取后核对 version `0.76.0`、revision `985774771a36037e638e7a87d3be1824223bda52`，以受限临时容器完成健康 E2E 并清理。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，与冻结源码一致；完整 lifecycle 已通过，无 apps 提交。

## 生产部署与回滚

- 唯一 KPanel 生产目标为 `arena-154`；`prod-108` 未连接、未检查、未备份、未部署、未升级、未核对。
- 升级前 v0.75.0：Panel healthy、restart 0、OOM false，Agent active；运行 OCI/revision 为 `sha256:4a19a1e1624d454d4817b60973ed9ae37b1e7d45164a08a4d9b868a174575724` / `de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`。
- 停写备份：`/root/kpanel-backups/v0.76.0-preupgrade-arena154-20260815T163831Z.tar.gz`，SHA-256 `185ce15fe01627e1c429fc5e6fdb46564e82f7d9364423e1b884f51e8a96a0eb`；旧镜像归档同名 `.image.tar`，SHA-256 `bb5b8d37079ec0f29fbceb782059612d059dc5c1161e96dec2f932dd50e0607c`。
- 备份在独立目录实际解包，数据树、`.env` 权限、Compose 与 SQLite 完整性通过；旧镜像归档已实际 `docker load`，并核对 v0.75.0/revision，回滚材料可执行。
- 部署入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`；更新日志 SHA-256 `a46ea1c7c19010ed27d2e32742e07ec2d266d9c4a02b6acbe11e6a4303c6296d`。
- 升级后 Panel/Agent 均为 v0.76.0；运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active；Compose、Panel-Agent、SQLite、错误日志和五次短健康采样通过，约 72 MiB/256 MiB、7 PIDs。
- 生产未开启公开分享，未执行跨主机传输或写入测试业务文件；危险和写旅程仅在隔离实例执行。
- 生产证据：`/root/kpanel-release-evidence/v0.76.0/arena154-20260815T163831Z`，其 `SHA256SUMS` 文件摘要为 `399e27d61ed2a6956f6ee7a97994217d90d0de2e57d96e2e5e1efc8812c5c891`；L3/隔离证据位于 `/root/kpanel-release-evidence/v0.76.0-9857747` 和本地 `C:\GitHub\_release-artifacts\v0.76.0`。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.75.0 镜像归档，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `systemctl daemon-reload`，启动 Agent 与 Compose，再核对版本、Panel-Agent、SQLite、restart/OOM 和日志。本次未触发生产回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-15T23:25:40+08:00
- 候选冻结时间：2026-08-15T23:49:09+08:00
- 生产完成时间：2026-08-16T00:39:45+08:00
- 提交到生产用时：1.24
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留边界与后续

- 公开分享默认关闭；管理员开启后仍需自行确认公开内容适合其环境。旧配对不会静默获得 `cluster.files.read`，跨面板文件复制需要按最小权限重新配对。
- 没有在生产业务目录执行跨面板文件写验收；安全、失败恢复和浏览器旅程已在 arena-154 隔离实例覆盖。
- 本轮复用既有 `release-kpanel` v2.4，不新增重复工作流；验收事实沉淀于本文件。
