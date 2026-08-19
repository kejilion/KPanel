# KPanel v0.77.0 上线验收记录

日期：2026-08-16（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`dc0082a3493e257267f896d58860613aa75571bb` / `v0.77.0`

上一稳定版本 / 回滚点：`v0.76.0` / `sha256:7cfc37123f870b66e46d20b06387e3f5dc7401a1da7d23ba7f12618fbd9f352d`

结论：通过

## 发布画像

- 业务域：前端国际化、应用市场目录元数据。
- 变更面：展示、浏览器语言检测与偏好持久化；不改变宿主机写入、协议、端口或 Compose。
- 受影响用户旅程：首次访问语言识别、语言切换、刷新恢复、全部核心页面和应用市场繁体中文显示、手机窄屏。
- 未变化契约：Panel/Agent API、数据、端口、Compose、Agent 权限、`kejilion.sh` 固定摘要和 KPanel 应用市场安装契约。
- 风险等级及理由：中；覆盖全站文案与目录展示，但无数据迁移或宿主机写操作，并以全量自动化、隔离真机和真实 Chrome 验收约束。

## 发布范围与未纳入内容

- 新增 `zh-TW` 繁体中文语言包，共 2241 条文案、20 个按页加载 catalog；支持 `zh-TW`、`zh-HK`、`zh-MO` 和 `Hant` 浏览器语言识别、手动切换及持久化。
- 应用市场 7 个分类和 147 个内置应用新增繁体中文名称/说明；同步流程保留繁中字段。
- 精确业务提交：`4fa55485511fe2b226494dda1362e82308a4ee74`、`16e2100`、`cee5642`、`c8e0866`；版本冻结提交：`dc0082a3493e257267f896d58860613aa75571bb`。
- 未纳入旧工作树、未提交草稿、重复补丁或实验内容；未改变应用市场安装契约，因此没有制造 apps 空提交。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go 全包、Web 98 文件/725 项、应用目录 7/147、公开镜像 E2E | 第三方动态应用可不提供繁中元数据，按既有回退显示 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/镜像高危严重 0 | 无新增外部运行依赖 |
| 稳定性、失败恢复与兼容 | 已验证 | race、双架构、隔离启动、刷新持久化、生产连续健康、旧镜像可加载 | 不涉及数据库迁移 |
| 性能与资源预算 | 已验证 | 生产约 72.63 MiB/256 MiB、7 PIDs，restart 0、OOM false | 语言包继续按页加载 |
| 用户体验与可访问性 | 已验证 | Chrome 151、桌面与 390px、无横向溢出、页面错误 0 | 未逐条人工朗读全部 2241 条文案 |
| 数据、配置与迁移 | 已验证 | 停写备份解包、数据树、Compose、SQLite、`.env` 权限和回滚镜像核对 | 无结构迁移 |

## 自动门禁

- 定向测试及结果：Web 98 个文件/725 项测试，i18n 2241 条/20 个 catalog、typecheck、生产构建全部通过。
- `make verify-release`：arena-154 固定 Linux Runner 从 bundle 和冻结 SHA 全量执行成功；L3 日志 SHA-256 `d3c2782040bf293d8fe5749b980b3147969378d57e1c50d5b4af0048195bb432`，bundle SHA-256 `ee6854f73677fda0d6a49437b99bdb83ef03c2053287fb34294587a8e272f149`。
- 候选 CI `31898595291` 与候选依赖治理 `31898595253`：success，head SHA 精确匹配。
- 主线 CI `31898807152` 与主线依赖治理 `31898807115`：success，head SHA 精确匹配。
- Release workflow `31899037263` 与 tag 依赖治理 `31899037384`：success。
- 安全扫描、镜像契约、SBOM/provenance：govulncheck、npm audit、Trivy source/image、受限容器、版本/revision、双架构和 attestation 均通过。

## 依赖与技术栈变化

- 本版没有新增或升级运行依赖、Go/Node 工具链、基础镜像、Action、扫描器或受管脚本。
- 依赖报告及检测源沿用冻结主线，Release 的依赖新鲜度、govulncheck、npm audit 和 Trivy 已重新执行并通过。
- 固定 Go 1.26.6 Alpine 基础镜像 digest 与 `kejilion.sh` 提交/摘要未变化；无需暂缓候选。
- 兼容、安全、构建、资源和回滚结论均已在本版精确 SHA 与正式 OCI 上复核。

## 隔离真机与浏览器验收

- 主机：arena-154，Debian 13 / linux/amd64；环境策略 `arena-154`，用途 release verification 与 production deploy。
- 精确候选：`dc0082a3493e257267f896d58860613aa75571bb`；隔离镜像 ID `sha256:821a8c8b37461f06ad2d9d87788b953ba19e98c4c17951ecddb566c4147392d`，version/revision 标签精确匹配。
- 应用市场真实 API 返回 7 个带 `zh_tw` 的分类、147 个带完整繁中名称/说明的内置应用。
- 正式 Google Chrome `151.0.7922.138` 使用独立临时 Profile 验证繁中切换、刷新持久化、概览、设置、应用市场、可见繁中应用说明及 390px 布局；页面错误 0、应用控制台错误 0、横向溢出 0。
- 浏览器结果 SHA-256：`bed14f75d0703acfb0c28f1800904bba9c4304b120287f6edb2339ff23e6dfd3`；证据位于 `/root/kpanel-release-evidence/v0.77.0-dc0082a/browser-i18n` 和本地 `C:\GitHub\_release-artifacts\v0.77.0`。
- 本版不涉及宿主机写功能或新失败注入；隔离容器、Agent、隧道和临时浏览器 Profile 已清理，生产 v0.76.0 在门禁阶段保持健康。

## 发布产物与公开仓库复核

- annotated tag object：`787558ed3ace33058ebbea9103dc27ccd83fc61d`；GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.77.0>，非 draft、非 prerelease。
- Docker `0.77.0` 与 `latest` OCI index：`sha256:072ede148231f4233d9fe849c0882340b1fcbed4bd327a13da830cacc058f378`。
- `linux/amd64`：`sha256:64a558ee80fac576fc467af28d37e7967b6f45dee8d3a7a07afeb27ee03e8d83`；`linux/arm64`：`sha256:58344e6f531a1035e7de4c2dbe0bfdfc02a7fa9258e98a4eed22bb3454ca549b`。
- 部署包 SHA-256：`f1ed31a2ca091096d40aef01967bb78c96be1370d27d5206a516acba87a08ebd`；`SHA256SUMS` 附件 SHA-256：`4d67d634d3d56cdc9341b60f3be4a216fc009ff63436257edc1a23c61989e4e5`。
- 公开镜像在 arena-154 重新拉取，`image_e2e=pass`；version/revision 与正式提交一致。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；安装契约无业务差异，无 apps 提交。

## 生产部署安全核对

- 生产目标和授权范围：唯一目标 `arena-154`；已授权正式升级、健康与回滚核对。
- 验证/灰度环境：`arena-154` 隔离容器。
- 正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前 v0.76.0：Panel healthy、restart 0、OOM false，Agent active；运行 OCI/revision 为 `sha256:7cfc37123f870b66e46d20b06387e3f5dc7401a1da7d23ba7f12618fbd9f352d` / `985774771a36037e638e7a87d3be1824223bda52`。
- 停写备份：`/root/kpanel-backups/v0.77.0-preupgrade-arena154-20260815T175825Z.tar.gz`，SHA-256 `c5512a2eb2b2a55c28d41410addded183cd314f11d8d65feeabccd0f2c53732f`；旧镜像归档同名 `.image.tar`，SHA-256 `f1e45af8ef4e412e0f7681d795813509417f55eb1ae7db8c2c6a55d4deb4c9d3`。
- 部署入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`；更新日志 SHA-256 `d8799153706725346e4af26c45a230ce0bd40ed4c5f377f290ad38a9c7700114`。
- 部署后 Panel/Agent 均为 v0.77.0；运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active，systemd 无待 reload；Compose、Panel-Agent、SQLite、错误日志和五次健康采样通过。
- 生产已执行写操作：标准应用市场更新及配套 Panel/Agent 替换；未写入测试业务数据。
- 仅在隔离真机执行、未在生产执行：新账户初始化、繁中偏好写入和浏览器 UI 旅程。

## 回滚

- 源码/tag：`e99eccaf471be1a892ac85f37a3a058a2f4a71a5` / `v0.76.0`。
- 镜像 digest：`sha256:7cfc37123f870b66e46d20b06387e3f5dc7401a1da7d23ba7f12618fbd9f352d`。
- 数据/配置备份：上述停写备份已在独立目录实际解包，数据树、`.env` 权限、Compose 与 SQLite 完整性通过；旧镜像归档已实际 `docker load`。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.76.0 镜像归档，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `systemctl daemon-reload`，启动 Agent 与 Compose，再核对版本、Panel-Agent、SQLite、restart/OOM 和日志。
- 回滚后生产实际版本与健康状态：未触发回滚；回滚材料和恢复核验已验证。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：v0.77.0 / OCI `sha256:072ede148231f4233d9fe849c0882340b1fcbed4bd327a13da830cacc058f378`。
- 公共默认更新通道决策：不适用；本版全部门禁通过并保持为稳定默认版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-16T01:03:55+08:00
- 候选冻结时间：2026-08-16T01:05:09+08:00
- 生产完成时间：2026-08-16T02:00:23+08:00
- 提交到生产用时：0.94
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与后续准入

- 第三方动态应用若未提供 `zh_tw` 字段，将沿用既有回退文案；147 个内置应用已完整覆盖。
- 未逐条人工朗读全部 2241 条文案；键完整性、按页加载、核心页面和真实浏览器旅程已覆盖，因此不阻断本版。
- 本轮复用 `release-kpanel` v2.4，不新增重复工作流；验收事实沉淀于本文件。
