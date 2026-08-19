# KPanel v0.74.0 上线验收记录

日期：2026-08-15（Asia/Shanghai）

发布工作流：`release-kpanel` v2.2

结论：通过

## 1. 发布范围

- `e0611d2ab5ee4db477fa3bff8a71a36d95e56993`：应用市场支持从官方 `https://app.kejilion.sh/` 动态合并内置与第三方应用目录。
- 动态目录仅接受固定来源、固定 schema、受限数量、唯一 ID/token、合法 selector 与受限 URL；失败时保留内置安全目录，不执行远端命令或安装器。
- 已有应用继续优先使用镜像内固定 `kejilion.sh`；仅当新增内置 selector 不在固定脚本中时，才检查 root-owned、不可组写/全局写且协议兼容的宿主脚本。
- `cf18c94d653c6f692683e4293284d054aff7e6a0`：准备 KPanel 0.74.0、CHANGELOG 与一致版本元数据。
- 数据库、Panel API 路由、端口、Compose、Agent 权限和桌面 UI 没有迁移或破坏性变化。

本轮同时独立发布 `kejilion/sh v4.5.6`：

- `d1e210d`：允许 DeepSeek Harness 官方 npm 包所需的安装脚本。
- `a3820af`：增加受保护的 Harness WebUI 域名访问管理。
- `a70793a344bdb0fc214d4b5ef1e722516d5d36e4`：冻结 4.5.6 版本元数据与更新日志。
- Annotated tag `v4.5.6` 对象为 `43151182f9003cc0705a8ab034a3c78619c327ef`，精确解析到 `a70793a`；GitHub Raw 根/CN、语法、同步与 Harness smoke 均通过。

## 2. 源码、CI 与正式产物

- 起始主线与回滚基线：`565e476623159247ec3ebb6967ab0d6753f165d1` / `v0.73.2`。
- Release commit：`cf18c94d653c6f692683e4293284d054aff7e6a0`。
- Annotated tag：`v0.74.0`，tag object `99303a89a5dc3c89ccdfba7ac7f9012750bcec9c`。
- 候选 CI：`31870957218`，成功；候选依赖治理：`31870957221`，成功。
- main CI：`31871151096`，成功；main 依赖治理：`31871151105`，成功。
- Release workflow：`31871427934`，成功；tag 依赖治理：`31871427928`，成功。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.74.0>，非 draft、非 prerelease，共 8 个附件。
- OCI index（`0.74.0` 与 `latest`）：`sha256:7973094ea48a28191cb2a4360f2c7c67a8fc536e0486082d2ff3e2abb3b33b99`。
- `linux/amd64`：`sha256:d0e24ad5bda273c4a754aa0a5d1012fecacb33c65bd86d417eed0ac5bb8e519b`。
- `linux/arm64`：`sha256:0fc2c226d1b9c8282901f170443e05ff9d8e9092de277e8be06f005d7d27af3b`。
- 两个平台的 OCI version 均为 `0.74.0`，revision 均为 Release commit。
- `kejilion/apps main@c5cc79ce4acd7f7b373573952616dbabd2060b7d` 的 `kpanel.conf` 与候选 Git blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；lifecycle 通过，无需制造 apps 空提交。

## 3. 自动化与 arena-154 隔离验收

- 最终精确 SHA 的完整 L3 通过：Go 全量、核心包 race、vet、Web 96 个测试文件/706 项测试、i18n 2177 条、TypeScript、生产构建、govulncheck、npm audit、Trivy 源码/镜像、Linux amd64/arm64、安装安全、治理、固定脚本契约与应用生命周期全部通过。
- 首次 L3 的源码在 Runner 内挂载为 `/src`，嵌套 Docker 将该路径解释为宿主路径而找不到已跟踪的部署测试；确认不是源码缺失后，改为宿主与 Runner 同一绝对路径，并从同一冻结 SHA 完整重跑通过，未跳过测试。
- L3 日志 SHA-256：`67ddda8296a5fadb631785d0965a86a94c8907546cbe895f81f2e12a6c929ffb`。
- Git bundle：`C:\GitHub\_release-artifacts\v0.74.0\kpanel-v0.74.0-cf18c94.bundle`，SHA-256 `90CEBF26DC8B1EEEF54F71DA5176EC39833B57E8BF0B1E6EADB43A915A0508FD`。
- arena-154 隔离候选完成实时目录读取：153 项、`catalogMode=live`、`builtin-116 / deepseek-harness` 唯一且可安装；固定旧脚本与新版 selector 的兼容边界符合预期。
- 隔离回滚完成 `v0.74.0 -> v0.73.2 -> v0.74.0`，Panel/Agent 版本同步、数据完整、目录恢复。
- 正式公开 OCI 重新拉取后完成 Panel→Agent→实时目录 E2E；healthy、0 重启、无 OOM，约 73 MiB/256 MiB。
- 本轮没有 Web 组件或布局差异；用户旅程由 AppsView 既有渲染回归、真实 Panel/Agent API 与官方目录链路覆盖，没有把浏览器工具故障或 Mock 预览作为发布证据。
- 154 证据：`/root/kpanel-release-v0740-cf18c94`；本地发布证据：`C:\GitHub\_release-artifacts\v0.74.0`。

## 4. arena-154 正式升级

- 升级前：v0.73.2，OCI `sha256:e0e567416b9b31b6541d2363dc0785714b65b0fcf3a0458b65592e641b815464`，Panel healthy、0 重启、无 OOM，Agent active。
- 停写一致性备份：`/root/kpanel-backups/v0.74.0-preupgrade-arena154-20260815T072704Z.tar.gz`。
- 备份 SHA-256：`e30cbb81705d7fcef084512be31588d4b879be449197cc2e522d393867a897fb`。
- v0.73.2 镜像归档：`/root/kpanel-backups/v0.74.0-preupgrade-arena154-20260815T072704Z.image.tar`。
- 镜像归档 SHA-256：`e65f9529b14be00d10762d91b4f83597d2712093df1a3ecbc58f7e4c930f9584`。
- 独立恢复目录已校验文件摘要、权限/属主/符号链接、SQLite/JSON 与镜像归档可读性后删除。
- 使用宿主现有且与正式 apps Git blob 相同的 `/root/apps/kpanel.conf` 执行标准更新，没有覆盖 `/root/apps` 的工作树内容。
- 升级后：v0.74.0，OCI digest/revision 精确匹配；Panel healthy、0 重启、无 OOM；Agent active。
- 生产实时目录为 153 项，包含唯一 `builtin-116 / deepseek-harness`；Compose、SQLite quick check、JSON、错误日志与三次短健康采样均通过。
- 生产证据：`/root/kpanel-release-evidence/v0.74.0/arena154-20260815T072704Z`。

## 5. 环境边界与回滚

- `prod-108`/`108` 已禁用全部 KPanel 操作；本轮未连接、未检查、未备份、未部署、未清理。
- 回滚点：源码/tag `v0.73.2`，OCI `sha256:e0e567416b9b31b6541d2363dc0785714b65b0fcf3a0458b65592e641b815464`。
- 回滚必须停服务并成套恢复本记录中的 v0.73.2 镜像归档、Compose、`.env`、Agent unit/二进制及数据备份，再核对版本、健康、目录和数据完整性。
- 本次没有触发生产回滚。

## 6. 遗留风险与流程说明

- 官方动态目录属于外部只读配置源；若来源不可达、结构越界或内容不合法，KPanel 会保留内置安全目录并显示警告。
- 新增内置 selector 只有在宿主 `kejilion.sh` 已支持时才能执行；旧宿主脚本会明确失败，不会回退到远端 Shell。用户更新到本轮 `kejilion/sh v4.5.6` 后可使用选项 116。
- 本轮未新增重复发布流程，继续复用 `release-kpanel` v2.2、`kejilion-release-rollback` v1.0、项目环境策略与版本治理。
- 发布后按用户要求保持本机唤醒，不执行睡眠。

### 结构化交付节奏数据

以下字段于 2026-08-15 根据本记录、提交元数据和已完成结论补充。原记录没有留存精确生产完成时刻，
因此相关时间与用时保持“未记录”，不使用备份时间、验收提交时间或会话结束时间代替生产完成时间。

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-15T14:04:08+08:00
- 候选冻结时间：2026-08-15T14:37:23+08:00
- 生产完成时间：未记录
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->
