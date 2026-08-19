# KPanel v0.73.1 上线验收记录

日期：2026-08-15（Asia/Shanghai）

## 发布结论

KPanel v0.73.1 已按 `release-kpanel` v2.1 完成未上线内容审计、候选冻结、完整 L3、154 隔离 Chrome/Firefox 旅程、候选 CI、主线 CI、Tag、GitHub Release、双架构公开镜像、应用市场契约核对、公开镜像 E2E、停写一致性备份以及 154/108 正式升级。

108 未参与候选、功能、浏览器、灰度或持续观察测试；只在不可变产物和 154 门禁全部通过后执行标准升级与一次最小部署安全核对。上线后未执行电脑睡眠。

## 上线内容与排除项

- 桌面框选区域改用稳定品牌色，在明暗主题下保持清晰可见。
- 系统浏览器外跳确认窗统一标题垂直对齐和头部、正文间距。
- 应用市场已安装卡片提高边框辨识度，统一使用品牌成功色。
- 应用卡片标题、状态徽标和正文采用明确的 flex 收缩约束，窄视口不重叠。
- 仅纳入四个范围明确、已提交且可重放的修复；未纳入旧草稿、重复补丁、临时预览、无关依赖升级或其他工作树内容。
- `kejilion.sh`、后端 API、数据结构和应用市场契约没有变化。

## 源码、CI 与版本

- 上一稳定版本：`v0.73.0` / `900c74e8f98085c77a14c807bba1d1d240e0272c`
- 本轮起始主线：`46c521aea2ae9e2513961fcf3efc8e5bb83361bd`
- 候选修复提交：`e29df3a`、`ce65e7b`、`e31aa04`、`20d74e5`
- v0.73.1 Release commit：`992da5d5d3a47198aa058fd117ee7fa523861e9c`
- tree：`237c0d933193ffcca5c20e8d911f8637b68a3719`
- Tag 对象：`cea3d2b18621f55943461e1b1817bdd7c3e4dd72`，解析到上述 Release commit
- Candidate CI：`31854949613`，通过
- Candidate dependency freshness：`31854949633`，通过
- Main CI：`31855160543`，通过
- Main dependency freshness：`31855160556`，通过
- Tag Release workflow：`31855382145`，通过
- Tag dependency freshness：`31855382170`，通过
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.73.1>，非 draft、非 prerelease，共 8 个附件

主线使用普通快进更新，没有强推或改写既有版本。

## 自动化与隔离验收

- 最终精确 SHA 的完整 L3 通过：Go 全量、核心包 race、vet、Web 96 个测试文件/705 项测试、i18n 2177 条、TypeScript、生产构建、govulncheck、npm audit、Trivy 源码/镜像、Linux amd64/arm64、受限容器、安装安全、治理、固定脚本契约与应用生命周期全部通过。
- L3 日志 SHA-256：`133e352b02f970a117557d9312344bf62cc9d7ce367881322725b68048f53f49`
- L3 证据：`/root/kpanel-release-v0731-992da5d`；本地副本：`C:\GitHub\_release-artifacts\v0.73.1`
- 完整 Git bundle：`kpanel-v0.73.1-992da5d.bundle`，SHA-256 `f9df0ad3b005c8ff3da66aaaccc3063021a14b6dc4b20b5e1df7f6f68c93a619`
- 隔离验证镜像：`sha256:d5446f09b94c1025dc0f61b34402ca0154e9711d5055565ccbc4569dd6e14a49`，version/revision 精确匹配。
- Google Chrome `151.0.7922.138` 与 Firefox `153.0` 使用随机临时 Profile 在后台验收；明暗主题框选、外跳确认与取消、精确 URL、`noopener`、已安装卡片高亮、标题/状态布局、390px 无横向溢出均通过；页面脚本错误 0。
- 浏览器结果文件 SHA-256：`0ff2771a577f7a815e6882fcf5ff64a643db239e4f91dd902c48387448010fff`；后台状态文件 SHA-256：`2ace4544397e1232122a2b8c33007f487f60957366f0e37af32a6df394a8d518`。
- 隔离回滚验证：完整切换到 v0.73.0 后健康，再恢复 v0.73.1；排除 SQLite WAL/SHM 运行时文件后，持久文件摘要一致。

## 不可变产物与应用市场

- OCI index（`0.73.1` 与 `latest`）：`sha256:d0e025b13e75de329e9a8e950459b7d8940e687603471ce0f92fb731ec23cddc`
- Linux amd64 manifest：`sha256:ec5332b33914bbd0a52ac777822d5e5a96c815995ffb0d72e495ffecb1e7c188`
- Linux arm64 manifest：`sha256:fab00fd47e5c949de156dbd1fac389ac87595d57d82618b5cf1859f8581f1ef9`
- 两个架构的 OCI version 为 `0.73.1`，revision 为 `992da5d5d3a47198aa058fd117ee7fa523861e9c`。
- 154 从 Docker Hub 按不可变摘要重新拉取，完成受限容器冷启动、初始化与 Agent 链路 E2E，输出 `public_oci_e2e=pass`。
- `kejilion/apps` commit：`c5cc79ce4acd7f7b373573952616dbabd2060b7d`
- 本轮应用配置无源码差异；apps 与 KPanel 的 Git blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，因此未产生空 apps 提交。
- 应用配置归一化 SHA-256 为 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`；生命周期由最终 L3 与 Tag Release workflow 再次验证通过。

## 正式部署

### arena-154

- 端口：`8080`
- 上线后：版本 `0.73.1`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- SQLite `quick_check`、JSON、Compose、错误日志与 3 次最小健康采样通过
- 证据：`/root/kpanel-release-evidence/v0.73.1/arena154-20260815T011030Z`
- 备份：`/root/kpanel-backups/v0.73.1-preupgrade-arena154-20260815T011030Z.tar.gz`
- 备份 SHA-256：`118fbd6996992d43665574217ddf77721f21b8f1b4cf8ac2e4d10b5783109da5`
- v0.73.0 镜像归档：`/root/kpanel-backups/v0.73.1-preupgrade-arena154-20260815T011030Z.image.tar`
- 镜像归档 SHA-256：`01c50c1121747cc699afc16bed1db7edcc57d8a65306972e741a7b5df6766595`

### production-108

- 端口：`5566`
- 上线后：版本 `0.73.1`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- 仅执行停写备份、标准应用市场更新和一次版本/健康/Agent/数据/Compose/回滚核对；未执行功能、浏览器、灰度、故障注入或持续观察
- 证据：`/root/kpanel-release-evidence/v0.73.1/prod108-20260815T011050Z`
- 备份：`/root/kpanel-backups/v0.73.1-preupgrade-prod108-20260815T011050Z.tar.gz`
- 备份 SHA-256：`9914744abb08a7c7d20210acaa3be0e9a90a2e4d620a570e0fa89a0419e62838`
- v0.73.0 镜像归档：`/root/kpanel-backups/v0.73.1-preupgrade-prod108-20260815T011050Z.image.tar`
- 镜像归档 SHA-256：`6387a5b9cce21198bf680b27cc430d33ea64a4509af815f36a6b87df79603af8`

两台主机的备份均在 Panel 与 Agent 停写状态生成；归档摘要、完整文件清单、权限/属主/符号链接清单、SQLite/JSON 数据、镜像归档可读性及独立恢复目录校验全部通过。

## 回滚方案

回滚点：`v0.73.0` / `900c74e8f98085c77a14c807bba1d1d240e0272c` / OCI `sha256:1a08d2a6a9963e05332ab85b5c6816332378a9ab4abbb7339c26274b3d316e42`。

每台主机回滚必须成套执行：

1. 停止 Panel 与 `kejilion-agent.service`，确认写入停止。
2. 校验该主机 `.tar.gz` 与 `.image.tar` 的 SHA-256。
3. 从归档恢复 `/home/docker/kpanel`、`/root/apps/kpanel.conf` 和 `kejilion-agent.service`。
4. `docker load` 对应 v0.73.0 镜像归档，并按证据中的 `old-image.id` 恢复镜像引用。
5. `systemctl daemon-reload`，启动 Agent，再使用已恢复 `.env` 与 Compose 重建 Panel。
6. 验证 v0.73.0、原端口、Panel healthy、Agent active、数据完整性与 revision。

本次没有触发生产回滚。

## 遗留风险与流程说明

- 本轮均为视觉和布局修复，没有数据/API 契约变化；主要剩余风险是未来浏览器 CSS 实现差异，已通过 Chrome、Firefox、明暗主题和窄视口覆盖。
- 应用卡片浏览器验收使用真实应用清单并将首项运行状态作为受控布局样本，不执行安装、更新或其他写操作。
- 108 按项目环境政策只做稳定部署和一次最小安全核对，未被包装成功能验收环境。
- 未新增重复发布流程；继续复用 `release-kpanel` v2.1、环境策略、后台浏览器验收、项目版本控制与应用生命周期门禁。
