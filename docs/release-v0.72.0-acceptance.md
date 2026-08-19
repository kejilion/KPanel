# KPanel v0.72.0 上线验收记录

日期：2026-08-15（Asia/Shanghai）

## 发布结论

KPanel v0.72.0 已按 `release-kpanel` v2.1 完成候选冻结、完整 L3、154 隔离 Chrome/Firefox 文件桌面旅程、候选 CI、主线 CI、Tag、GitHub Release、双架构公开镜像、应用市场契约、公开镜像 E2E、停写一致性备份以及 154/108 正式升级。

108 未参与候选、功能、浏览器、灰度或持续观察测试；只在不可变产物和 154 门禁全部通过后执行标准升级与最小部署安全核对。

## 上线内容与排除项

- 桌面工作区升级为 schema v2，文件和目录可作为桌面入口；移除桌面入口不会删除真实文件或目录。
- 文件窗口支持相同目录复用、文件预览可见性修复、全屏代码编辑器与批量操作栏布局优化。
- 桌面支持框选、多选批量操作、分组拖动和更明确的选择提示。
- Windows、macOS 与 Linux 的外部文件/目录可拖入桌面，上传到宿主机并创建入口；包含有界枚举、进度、部分失败和同名副本保护。
- 文件窗口之间支持拖放移动与 `Ctrl`/`Option` 复制；移动或重命名成功后，以 workspace CAS 更新完全匹配及后代快捷方式路径。
- 文件 copy/move 校验源 `expectedResourceVersions`，过期源逐项冲突，不误操作；应用、网站和 URL 快捷方式拖入文件窗口保持 no-op。
- 纳入最新主线的概览应用摘要防挤压修复 `a103ca0`，并自动包含 README wordmark 文档提交 `bb74bdf`。
- 未纳入脏的桌面全屏草稿、Web Terminal 草稿、旧图标分支、历史重复补丁或无关依赖升级；`kejilion.sh` 与应用市场契约没有变化。

## 源码、CI 与版本

- 上一稳定版本：`v0.71.0` / `851d3a83940748671bf182c475f80778c9513ffe`
- 最新主线集成基线：`a103ca00a30f91541bd9884f30f4a2691764206a`
- v0.72.0 Release commit：`9d3217459e236bdc08dfa4cd0312cbe76f4b5a2f`
- tree：`80de50f73fbd5c8b9c8ad85a8ea60297c04a2547`
- Tag 对象：`0d765a5a4c0337818aac53a84822f89ae68df26a`，解析到上述 Release commit
- Candidate CI：`31826439185`，通过
- Candidate dependency freshness：`31826439183`，通过
- Main CI：`31827033480`，通过
- Main dependency freshness：`31827033429`，通过
- Tag Release workflow：`31827492963`，通过
- Tag dependency freshness：`31827492916`，通过
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.72.0>，非 draft、非 prerelease，共 8 个附件

主线使用普通快进更新，没有强推或改写既有版本；发布流程已自动清理远端候选分支。

## 自动化与隔离验收

- 最终精确 SHA 的完整 L3 通过：Go 全量、核心包 race、vet、Web 95 个测试文件/700 项测试、i18n 2177 条、TypeScript、生产构建、govulncheck、npm audit、Trivy 源码/镜像、Linux amd64/arm64、受限容器、安装安全、治理、固定脚本契约与应用生命周期全部通过。
- L3 日志 SHA-256：`5fe282c76c9e3bdb729ad1a784d10dbc9adf9520ef6972676d5b545b692e5ff6`
- L3 证据：`/root/kpanel-release-v0720-9d32174`；本地副本：`C:\GitHub\_release-artifacts\v0.72.0`
- 完整 Git bundle：`kpanel-v0.72.0-9d32174.bundle`，SHA-256 `a8743d1168a64734f5a712a602d36965cf17e30ba0d54dfa702ad52efbded18b`
- 隔离验证镜像：`sha256:8465ce8e24da8769ae0da82302ced69c5ca62b2e40405cab1203b27b98663af8`，version/revision 精确匹配。
- Google Chrome `151.0.7922.138` 与 Firefox `153.0` 均使用随机临时 Profile 在后台验收；窗口复用、跨窗 move/copy、过期源冲突、外部文件夹上传、框选批量移除但保留真实文件、Agent/Panel 重启恢复、workspace v2 持久化及无横向溢出全部通过；页面脚本错误 0。
- 浏览器结果文件 SHA-256：`3c75c23c5683fe40e3a985aa61ce67557fdd0856bab7cdac66a9a632536c27c4`；内部汇总摘要：`ac4edb549e78397986dcf355dbb259c084c5ec84782f5e5db57b028b681e9fd9`。
- 隔离回滚严格验证：完整切换到 v0.71.0 后健康，再恢复 v0.72.0；workspace 文件摘要保持不变，两个版本均 healthy、restart 0、OOM false。

## 不可变产物与应用市场

- OCI index（`0.72.0` 与 `latest`）：`sha256:5fce795e767d676d8fc919aff3701269076df4de7adca03f060d50d96a4dcec7`
- Linux amd64 manifest：`sha256:12e9672a79391bfe66825027f4b479c7a1792a762fee163d2a9d01b535471267`
- Linux arm64 manifest：`sha256:6bf6a874fcadee761ab2ffdeafafbfabb67867262ded45fb181f3f420004bed6`
- 两个架构的 OCI version 为 `0.72.0`，revision 为 `9d3217459e236bdc08dfa4cd0312cbe76f4b5a2f`。
- 154 从 Docker Hub 按不可变摘要重新拉取，完成受限容器冷启动、初始化与 Agent 链路 E2E，输出 `public_oci_e2e=pass`。
- `kejilion/apps` commit：`c5cc79ce4acd7f7b373573952616dbabd2060b7d`
- 本轮应用配置无源码差异；apps 与 KPanel 的 Git blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，归一化内容一致，因此未产生空 apps 提交。
- 应用配置 SHA-256 为 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`；生命周期由最终 L3 与 Tag Release workflow 再次验证通过。

## 正式部署

### arena-154

- 端口：`8080`
- 上线后：版本 `0.72.0`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- SQLite `quick_check`、JSON、Compose、错误日志与 3 次最小健康采样通过；最终约 72.16 MiB / 256 MiB、7 PIDs
- 证据：`/root/kpanel-release-evidence/v0.72.0/arena154-20260814T184146Z`
- 备份：`/root/kpanel-backups/v0.72.0-preupgrade-arena154-20260814T184146Z.tar.gz`
- 备份 SHA-256：`50a6eaf59073ebd1a171e3751a8db80df4f1b0bb1f1ca7572a7b1f8f7fb6cfcb`
- v0.71.0 镜像归档：`/root/kpanel-backups/v0.72.0-preupgrade-arena154-20260814T184146Z.image.tar`
- 镜像归档 SHA-256：`32fdc73c698698883f29b6077ad19ef989889ca7789d9bdd04fe7ab05cc63470`

### production-108

- 端口：`5566`
- 上线后：版本 `0.72.0`，OCI 摘要与 revision 精确匹配；Panel healthy、restart 0、OOM false；Agent active
- 仅执行停写备份、标准应用市场更新、版本/健康/Agent/数据/Compose/回滚核对和 3 次最小健康采样；未执行功能、浏览器、灰度、故障注入或长时间观察
- 最终约 76.68 MiB / 256 MiB、7 PIDs
- 证据：`/root/kpanel-release-evidence/v0.72.0/prod108-20260814T184253Z`
- 备份：`/root/kpanel-backups/v0.72.0-preupgrade-prod108-20260814T184253Z.tar.gz`
- 备份 SHA-256：`1a641a26823b49e7fc48427d437dc9215e8bb3b154ce69d03d22f8a8aee2c824`
- v0.71.0 镜像归档：`/root/kpanel-backups/v0.72.0-preupgrade-prod108-20260814T184253Z.image.tar`
- 镜像归档 SHA-256：`eddc84a9d35dec92c73823fb861e73bb79789047e2ca11a68381556706c4fdf8`

两台主机的备份均在 Panel 与 Agent 停写状态生成；归档摘要、完整文件清单、权限/属主/符号链接清单、SQLite/JSON 数据、镜像归档可读性及独立恢复目录校验全部通过。两台原生产环境均没有既有 desktop workspace 文件，因此本轮没有迁移或覆盖历史桌面布局数据。

## 回滚方案

回滚点：`v0.71.0` / `851d3a83940748671bf182c475f80778c9513ffe` / OCI `sha256:4857c207d127facc2228aa205862e51d52e6a8428f9750a1516c8222e085d5b5`。

每台主机回滚必须成套执行：

1. 停止 Panel 与 `kejilion-agent.service`，确认写入停止。
2. 校验该主机 `.tar.gz` 与 `.image.tar` 的 SHA-256。
3. 从归档恢复 `/home/docker/kpanel`、`/root/apps/kpanel.conf` 和 `kejilion-agent.service`。
4. `docker load` 对应 v0.71.0 镜像归档，并按证据中的 `old-image.id` 恢复 `docker.io/kjlion/kejilion-panel:latest`。
5. `systemctl daemon-reload`，启动 Agent，再使用已恢复 `.env` 与 Compose 重建 Panel。
6. 验证 v0.71.0、原端口、Panel healthy、Agent active、数据完整性与 revision。

v0.71.0 会忽略并保留 schema v2 workspace；若生产以后创建文件/目录入口后再回滚，必须成套保留 `workspace.json` 与 `icons/`，不得只切换镜像。本次没有触发生产回滚。

## 遗留风险与流程说明

- 外部目录上传与跨窗口传输的风险集中在大批量文件、部分失败与并发版本冲突；本轮以有界自动化、真实浏览器和源版本 CAS 覆盖，生产仍应结合备份使用。
- 154 首次维护调用在顶层加载应用配置时输出了 `local can only be used in a function` 与缺少 `docker_app_plus` 的上下文告警；随后实际 `docker_app_update`、版本、健康、数据和回滚门禁均成功。108 改用函数内加载配置的标准包装器，过程无该告警。原始日志保留在对应生产证据目录，未覆盖或美化。
- L3 前磁盘保护处理只回收了无容器引用的旧 BuildKit 缓存；未删除生产/回滚镜像、容器、卷或 `/root/kpanel-backups`。后续清理继续要求精确归属，不执行广泛 Docker prune。
- 未新增重复发布流程；继续复用 `release-kpanel` v2.1、环境策略、后台浏览器验收、项目版本控制与应用生命周期门禁。
