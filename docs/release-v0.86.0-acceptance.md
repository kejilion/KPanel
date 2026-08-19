# KPanel v0.86.0 发布验收记录

日期：2026-08-19

发布级别：L3

候选提交 / 标签：`132a06a84f359ccdbd863ad167f5740aacb78049` / `v0.86.0`

上一稳定版本 / 回滚点：`v0.85.0` / `sha256:3894a1f4dad31fa853b4bd93d561e4eaafe6e2c14cbcac49031a65602f57bf40`

## 发布画像

- 业务域：AI 选择菜单、文件媒体预览、Docker 编组、监控历史提示。
- 变更面：展示 / 只读 / 文件读取与媒体流响应；不新增宿主机写入、协议或数据迁移。
- 受影响用户旅程：移动端 AI 选项选择、文件视频预览与拖动/范围读取、Docker 编组按创建时间排序、监控空闲态浏览。
- 未变化契约：API 认证与权限、Agent 权限、`kejilion.sh` 内容、应用市场配置契约、生产端口和 Compose 拓扑均未改变。
- 风险等级及理由：中等；涉及文件流响应与 Docker 展示排序，已完成全量测试、隔离真机和发布级安全门禁。

## 发布范围与未纳入内容

- 用户可见更新：AI 移动端选择菜单居中；媒体预览支持有界流式读取；Docker 编组可按容器创建时间排序；监控无活动时隐藏历史缩放提示。
- 精确提交清单：
  - `80a7f25c513d83a6da78854b34f036778c169942` fix(ai): center mobile choice menus
  - `51f3256bfc57e3d1913806885940cb36542c5634` fix(files): make media preview streamable
  - `d552d01e7cbb0a84427b1a0d67209d37595475fc` feat(docker): sort compose groups with containers
  - `7b9febac7185888512a9eab54a1fc38a056e0b97` fix(monitoring): hide idle history zoom hint
  - `132a06a84f359ccdbd863ad167f5740aacb78049` chore: prepare KPanel 0.86.0
- 明确未纳入的分支、文件或后续事项：旧文件拖拽草稿 `11e5825…` 未纳入；已在主线等价存在的归档/桌面传输功能未重复 cherry-pick；未纳入视频内核、108 环境及任何未审查工作树改动。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Web 100 files / 772 tests；媒体、Docker、AI、监控定向回归；154 隔离真机文件视频 Range/HEAD、Docker 生命周期与真实容器排序通过 | 生产未执行写入型业务操作，避免影响现有数据 |
| 网络入侵与供应链安全 | 已验证 | 候选/主线/Release workflow 安全扫描、Trivy、govuln、npm audit、镜像契约和 SBOM/provenance 全绿；公开 OCI digest 固定 | 无 |
| 稳定性、失败恢复与兼容 | 已验证 | Go 全量、race、vet、amd64/arm64 构建；Docker 失败重部署回滚与资源版本测试通过；生产 3 次健康采样 | 未进行长时 soak，本版无长时内核改动 |
| 性能与资源预算 | 已验证 | 发布级构建与隔离运行门禁通过；生产 Panel healthy、restart=0、OOM=false | 本次生产仅做短采样，未做额外长时资源曲线 |
| 用户体验与可访问性 | 已验证 | 390px/桌面视口、媒体播放、Docker 编组和 AI 菜单浏览器复核通过；监控空闲提示隐藏回归通过 | 无 |
| 数据、配置与迁移 | 已验证 | 停写备份保存 Compose、`.env`、Agent/脚本、面板数据和 v0.85.0 镜像；升级前后 `/root/apps/kpanel.conf` 哈希一致 | 未执行生产数据库写入迁移；SQLite 深度完整性检查未单独运行 |

## 自动门禁

- 定向测试及结果：媒体、Docker、监控、AI 相关回归通过；前端全量 100 files / 772 tests、i18n 2295 phrases、typecheck、production build 全通过。
- `make verify-release` 环境和结果：Linux WSL Go 1.26.6 全量 test/vet/race、amd64/arm64 CGO-free 构建通过；Release workflow L3 完整通过。
- 候选 CI：run [32157568861](https://github.com/kejilion/KPanel/actions/runs/32157568861) success。
- 主线 CI：run [32158473628](https://github.com/kejilion/KPanel/actions/runs/32158473628) success；依赖 freshness run `32158473589` success。
- Release workflow：run [32158969994](https://github.com/kejilion/KPanel/actions/runs/32158969994) success，Tag/Release/双架构 OCI、latest promotion、应用生命周期和安全扫描均通过。
- 安全扫描、镜像契约、SBOM/provenance：Trivy source/image、govuln、npm audit 通过；公开镜像 revision=`132a06a84f359ccdbd863ad167f5740aacb78049`、version=`0.86.0`，受管脚本 revision/hash 固定并匹配。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：2026-08-18 发布 workflow；依赖 freshness CI `32158473589` success。
- 最近每日安全通告审计、EOL 复核状态及证据：Release workflow 的 dependency policy、govuln、Trivy 和 npm audit 全通过。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go 1.26.6；其余依赖与受管脚本沿用已发布基线，无新增第三方运行时。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：VERSION/package 锁文件为 0.86.0；OCI index=`sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8`；脚本 revision=`fdb0ac0e1f2b98d27339937e7f8eb0c9299c56a9`、sha256=`d8c06ad40c2845a2ee3f1f4c9f0780b7e30d65a58bca91a80cdca5c390222408`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：旧拖拽草稿因夹带历史差异拒绝；无待发布阻断候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：兼容现有 Compose/Agent；生产升级可回滚到 v0.85.0 备份与镜像。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：arena-154 隔离 Debian 13、Docker 29.6.2、x86_64；发布 Runner 使用 Linux amd64/arm64。
- 环境策略 ID 与允许用途：`arena-154`，验证与正式 KPanel 部署；未使用 108。
- 使用的精确候选或公开产物：公开 `docker.io/kjlion/kejilion-panel@sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：Release run `32158969994` success；WSL/隔离证据保存在本次受限 release artifacts，完整 bundle SHA-256=`0A361FBC8AFC24BC448FFFB7E8306A0CD14EA8CAE2B844DC2F4261335EC4C651`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：媒体播放、Range/HEAD、Docker 生命周期与回滚按受影响画像完成；长时 soak 不适用，本版无长时驻留内核变更。
- 受影响用户旅程、视口、主题、键盘/焦点、语言和失败态：桌面/390px、暗色/亮色、中英文与媒体播放、Docker 编组排序、AI 菜单键盘/焦点回归通过；错误与回滚路径通过。
- 宿主机写入、失败注入、重启恢复和回滚结果：隔离 Docker 项目完成失败重部署自动恢复、资源版本冲突和容器生命周期；生产仅执行标准应用更新入口。
- 未执行场景及原因：108 全部禁用；生产不执行文件写入、Docker 项目重部署或系统调优写操作，避免影响正式业务。

## 发布产物与公开仓库复核

- GitHub Release：[v0.86.0](https://github.com/kejilion/KPanel/releases/tag/v0.86.0)，已公开；资产和 `SHA256SUMS` 齐全。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.86.0` 与 `latest` 均指向 `sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8`。
- `linux/amd64`、`linux/arm64` digest：Release workflow 双架构构建与推送通过，index 内含两平台清单。
- 附件及 `SHA256SUMS`：Release 页面已核对。
- 公开镜像 `image_e2e=pass`：154 回拉不可变 digest，隔离容器 healthcheck 与 HTTP 200 通过，测试容器和临时目录已清理。
- `kejilion/apps` / `kejilion.sh` 契约结论：apps main clean、`kpanel.conf` 无本次变更；标准 lifecycle 更新成功；镜像内受管脚本 revision/hash 与契约一致。

## 生产部署安全核对

- 生产目标和部署授权范围：仅 `arena-154`；用户已授权 KPanel v0.86.0 正式升级。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 隔离容器及本机回环端口。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：v0.85.0，Panel healthy、Agent active、restart=0、OOM=false；备份 `/root/kpanel-backups/pre-v0.86.0-20260818T162729Z`，旧镜像 tar SHA-256=`3f527dff95b92dcb367bc58392d7f72d568b531c8eff0483521ff733647401dd`。
- 部署命令/入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：Panel `0.86.0`、`/api/v1/health` 连续 3 次 `status=ok`、Agent `0.86.0 v1alpha1` active、restart=0、OOM=false、近 3 分钟无 panic/fatal/OOM；根 HTTP 200；配置哈希与升级前一致。
- 生产已执行写操作：应用市场标准更新写入新 Panel 镜像/容器、Agent 二进制及 systemd 运行文件；未执行业务数据、SSH、防火墙、Docker 项目或用户账户写操作。
- 仅在隔离真机执行、未在生产执行的场景：媒体播放/Range、Docker Compose 失败回滚与容器排序、文件归档与资源限制、AI/监控交互回归。

## 回滚

- 源码/tag：回滚到 `v0.85.0`（tag `0643785978ebec27292f34706e32a907a2555955`）或备份对应的旧镜像。
- 镜像 digest：旧生产镜像 `sha256:3894a1f4dad31fa853b4bd93d561e4eaafe6e2c14cbcac49031a65602f57bf40`；本次保留 rollback tag `kpanel-rollback:v0.85.0-20260818T162729Z`。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.86.0-20260818T162729Z`，含 Compose、`.env`、Agent/脚本、面板数据、apps 配置副本和旧镜像 tar。
- 回滚步骤和回滚后复核：停止 Agent/Panel，恢复备份中的镜像/Compose/`.env`/Agent 文件，`systemctl daemon-reload` 后启动 Agent 与 Compose，核对 `/api/v1/health`、healthy、restart/OOM 和配置哈希。
- 回滚后生产实际版本与健康状态：未执行回滚；当前 v0.86.0 healthy。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：Release workflow 已将 `latest` promotion 到 v0.86.0；标准 `app kpanel update` 已实际回拉该 digest。
- 公共默认更新通道决策：短期保留 v0.86.0；已知未验证风险为生产 SQLite 深度完整性未单独运行，若发现业务数据异常立即按上述成套回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-18T23:21:46+08:00
- 候选冻结时间：2026-08-18T23:54:07+08:00
- 生产完成时间：2026-08-19T00:28:56+08:00
- 提交到生产用时：1.12
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

## 遗留风险与后续准入

- 未验证风险：生产 SQLite 深度完整性未单独执行；未做长时 soak；108 永久不纳入 KPanel 测试或部署。
- 已实现待实机准入：无；受影响功能已在 arena-154 隔离真机完成对应画像验收。
- 不阻断本版的理由：生产健康、版本、配置和回滚备份均已核对；未验证项不涉及本版新增迁移或危险宿主机写操作。
- 后续应进入的自动门禁或专项工作流：将 SQLite integrity 与短时资源采样纳入后续只读发布后核对；继续保留 v0.85.0 成套回滚资料。
