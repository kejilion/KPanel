# KPanel v0.73.0 上线验收记录

日期：2026-08-15（Asia/Shanghai）

## 发布结论

KPanel v0.73.0 已按 `release-kpanel` v2.1 完成净差异审计、候选冻结、完整 L3、154 隔离 Chrome/Firefox 旅程、候选 CI、主线 CI、Tag、GitHub Release、双架构公开镜像、应用市场契约、公开镜像 E2E、停写一致性备份以及 154/108 正式升级。

108 未参与候选、功能、浏览器、灰度或持续观察测试；只在不可变产物和 154 门禁全部通过后执行标准升级与一次最小部署安全核对。

## 上线内容与排除项

- 桌面右键菜单在主题切换下方新增浏览器全屏开关；进入、退出、浏览器拒绝和不支持时均保持桌面可用。
- 网站页面等待 Agent capabilities 实际加载完成后才显示写入不可用提示，避免初次加载时短暂误报。
- 两项改动分别从旧开发工作树提取为聚焦提交，再迁入最新主线；没有把旧分支历史整体带入候选。
- Web Terminal 的既有功能提交经 patch-id 核对已在主线，本轮只纳入上述加载期修复。
- 未纳入已上线重复项、旧实验、临时预览文件、无关依赖升级或其他脏工作树；`kejilion.sh` 与应用市场契约没有变化。

## 源码、CI 与版本

- 上一稳定版本：`v0.72.0` / `9d3217459e236bdc08dfa4cd0312cbe76f4b5a2f`
- 本轮起始主线：`5921708d3705e1528aa2671b3dc0142073155228`
- 功能提交：`b6c118e`、`b9e40bd`
- v0.73.0 Release commit：`900c74e8f98085c77a14c807bba1d1d240e0272c`
- tree：`20971cbb864d75beb7b2aa45971ac26404ce78ba`
- Tag 对象：`1d425173e18d069852027630f510d738188efe8e`，解析到上述 Release commit
- Candidate CI：`31836255458`，通过
- Candidate dependency freshness：`31836255431`，通过
- Main CI：`31836620454`，通过
- Main dependency freshness：`31836620474`，通过
- Tag Release workflow：`31837091509`，通过
- Tag dependency freshness：`31837091473`，通过
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.73.0>，非 draft、非 prerelease，共 10 个附件

主线使用普通快进更新，没有强推或改写既有版本；发布流程已自动清理远端候选分支。

## 自动化与隔离验收

- 最终精确 SHA 的完整 L3 通过：Go 全量、核心包 race、vet、Web 96 个测试文件/705 项测试、i18n 2177 条、TypeScript、生产构建、govulncheck、npm audit、Trivy 源码/镜像、Linux amd64/arm64、受限容器、安装安全、治理、固定脚本契约与应用生命周期全部通过。
- L3 日志 SHA-256：`8b6d026911dccd3663df08013ac04ef3f7277c0a879e5d87d60c3f5fb85a0ff3`
- L3 证据：`/root/kpanel-release-v0730-900c74e`；本地副本：`C:\GitHub\_release-artifacts\v0.73.0`
- 完整 Git bundle：`kpanel-v0.73.0-900c74e.bundle`，SHA-256 `19f3c1258bb35fca128262af66050de817088aed7df9969ea307954433ce11af`
- 隔离验证镜像：`sha256:8d7df54f590aae47037388367a0a829f5054d1a322a0809385aa702020aef9b5`，version/revision 精确匹配。
- Google Chrome `151.0.7922.138` 与 Firefox `153.0` 使用随机临时 Profile 在后台验收；桌面右键菜单位置、进入/退出全屏、请求被拒绝后的可恢复行为，以及 capabilities 延迟期间不误报均通过；页面脚本错误 0。
- 浏览器结果文件 SHA-256：`7344dab14becfb0e2508770a245821c53fa2ea3de129578f2b6de4b3d7ba4ec5`；内部证据摘要：`4324c60a52cf854a1a7a81701e16e36a477ba48c444d8cd3c8269e3065771fc5`。
- 隔离回滚验证：完整切换到 v0.72.0 后健康，再恢复 v0.73.0；workspace 文件 SHA-256 始终为 `2917b3d201704405b9ab1c60e5538aa71c8167727606ff6780246028f5c4274e`。

## 不可变产物与应用市场

- OCI index（`0.73.0` 与 `latest`）：`sha256:1a08d2a6a9963e05332ab85b5c6816332378a9ab4abbb7339c26274b3d316e42`
- Linux amd64 manifest：`sha256:a964b3b65de5a70f8381d3cb41e2016e6b28f7543b60defe8840f6a5176b4a5b`
- Linux arm64 manifest：`sha256:5608701c0d1100f938492e6787192b9f6a7933231bac8cdbf127906243efdbdd`
- 两个架构的 OCI version 为 `0.73.0`，revision 为 `900c74e8f98085c77a14c807bba1d1d240e0272c`。
- 154 从 Docker Hub 按不可变摘要重新拉取，完成受限容器冷启动、初始化与 Agent 链路 E2E，输出 `public_oci_e2e=pass`。
- `kejilion/apps` commit：`c5cc79ce4acd7f7b373573952616dbabd2060b7d`
- 本轮应用配置无源码差异；apps 与 KPanel 的 Git blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，归一化内容一致，因此未产生空 apps 提交。
- 应用配置 SHA-256 为 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`；生命周期由最终 L3 与 Tag Release workflow 再次验证通过。

## 正式部署

### arena-154

- 端口：`8080`
- 上线后：版本 `0.73.0`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- SQLite `quick_check`、JSON、Compose、错误日志与 3 次最小健康采样通过
- 证据：`/root/kpanel-release-evidence/v0.73.0/arena154-20260814T203818Z`
- 备份：`/root/kpanel-backups/v0.73.0-preupgrade-arena154-20260814T203818Z.tar.gz`
- 备份 SHA-256：`18e2e6bc2906fe0217c0f50afd95d67cf760355f78c0f4ef9b8dee6c3700c24a`
- v0.72.0 镜像归档：`/root/kpanel-backups/v0.73.0-preupgrade-arena154-20260814T203818Z.image.tar`
- 镜像归档 SHA-256：`86f66b04ce0a601a73b7e54296f665a4e6d8999ffe7401bd62b3fe5dd9d077fd`

### production-108

- 端口：`5566`
- 上线后：版本 `0.73.0`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- 仅执行停写备份、标准应用市场更新和一次版本/健康/Agent/数据/Compose/回滚核对；未执行功能、浏览器、灰度、故障注入或持续观察
- 证据：`/root/kpanel-release-evidence/v0.73.0/prod108-20260814T203920Z`
- 备份：`/root/kpanel-backups/v0.73.0-preupgrade-prod108-20260814T203920Z.tar.gz`
- 备份 SHA-256：`c9487dbb7ce575f9372779a8f9994a8fbaf7bae5110defa965de7f0e3c5936f3`
- v0.72.0 镜像归档：`/root/kpanel-backups/v0.73.0-preupgrade-prod108-20260814T203920Z.image.tar`
- 镜像归档 SHA-256：`8b303ae630e091dbc0a8d8a4b3d5f7b8f9ff1681455a8987988bc1136064662f`

两台主机的备份均在 Panel 与 Agent 停写状态生成；归档摘要、完整文件清单、权限/属主/符号链接清单、SQLite/JSON 数据、镜像归档可读性及独立恢复目录校验全部通过。

## 回滚方案

回滚点：`v0.72.0` / `9d3217459e236bdc08dfa4cd0312cbe76f4b5a2f` / OCI `sha256:5fce795e767d676d8fc919aff3701269076df4de7adca03f060d50d96a4dcec7`。

每台主机回滚必须成套执行：

1. 停止 Panel 与 `kejilion-agent.service`，确认写入停止。
2. 校验该主机 `.tar.gz` 与 `.image.tar` 的 SHA-256。
3. 从归档恢复 `/home/docker/kpanel`、`/root/apps/kpanel.conf` 和 `kejilion-agent.service`。
4. `docker load` 对应 v0.72.0 镜像归档，并按证据中的 `old-image.id` 恢复镜像引用。
5. `systemctl daemon-reload`，启动 Agent，再使用已恢复 `.env` 与 Compose 重建 Panel。
6. 验证 v0.72.0、原端口、Panel healthy、Agent active、数据完整性与 revision。

本次没有触发生产回滚。

## 遗留风险与流程说明

- 全屏 API 最终受浏览器权限、用户手势与嵌入环境限制；产品在拒绝或不支持时保留原桌面视口并给出可理解反馈。
- 网站 capabilities 仍依赖 Agent 真源；本轮修复只消除加载窗口期误报，不改变 Agent 不可用时的真实告警。
- 108 按项目环境政策只做稳定部署和一次最小安全核对，未被包装成功能验收环境。
- 未新增重复发布流程；继续复用 `release-kpanel` v2.1、环境策略、后台浏览器验收、项目版本控制与应用生命周期门禁。
