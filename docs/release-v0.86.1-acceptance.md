# KPanel v0.86.1 发布验收记录

日期：2026-08-19

发布级别：L3（补丁）

候选提交 / 标签：`67f19e7e6db6ad2fc13f5a66aee56838819e4b39` / `v0.86.1`

上一稳定版本 / 回滚点：`v0.86.0` / `sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8`

## 发布画像

- 业务域：桌面文件/目录拖拽下载兼容性。
- 变更面：仅前端拖拽 `DataTransfer` 协议兼容；新增标准 `text/uri-list` 回退，并复用同源 HTTP(S)、无凭据 URL 校验。
- 受影响用户旅程：桌面文件单项、ZIP（文件夹/多选）拖拽到浏览器或文件管理器；右键下载、内部桌面拖拽保持原有行为。
- 未变化契约：后端/API、Agent 权限、端口、Compose 拓扑、apps 配置、`kejilion.sh` 与数据模型均未改变。
- 风险等级及理由：中等；Windows Explorer/企业浏览器策略属于客户端边界，已由用户现场验证并发现组织策略拦截，不能由前端代码绕过。

## 发布范围与未纳入内容

- `d1e5dab6f3f5525f6b2c445334365f91c9c91968`：候选补丁源提交。
- `279adb8e84206ff3b364404b3a00ac66d4999055`：在最新主线重放后的等价提交 `fix(files): preserve native drag URI fallback`。
- `67f19e7e6db6ad2fc13f5a66aee56838819e4b39`：版本、CHANGELOG 与发布元数据冻结提交。
- 明确未纳入：旧分支 `9f70044`/`11e5825` 的陈旧归档差异、未提交 MonitoringView 用户改动、后端/API/版本之外的草稿、108 环境。

## 变更语义

- 同源、HTTP(S)、不含 username/password 的 URL 同时写入既有 `DownloadURL` 与标准 `text/uri-list`（CRLF）。
- 跨域、非 HTTP(S) 或含凭据 URL 两种格式均拒绝写入；不新增长期 Token，不放宽会话/企业下载策略。
- 单文件、文件夹/多选 ZIP、内部桌面拖拽和 MIME/Windows 文件名清理保持主线实现。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与兼容 | 已验证（自动化） | 定向 62 tests；前端全量 100 files / 772 tests；同源 URI、ZIP URI、跨域/凭据拒绝回归通过 | Windows Explorer 实机拖拽由用户现场验证 |
| 网络与供应链安全 | 已验证 | 候选/主线/Release workflow 的 govuln、Trivy、npm audit、镜像契约、SBOM/provenance 全通过 | 无 |
| 稳定性与失败恢复 | 已验证 | Go/Web/部署门禁、双架构构建与公开镜像 runtime contract 通过；154 升级后 3 次 health 采样成功 | 未做长时 soak；补丁无常驻后端改动 |
| 用户体验与可访问性 | 部分验证 | 桌面前端测试与构建通过；不改变确认/右键路径 | 用户现场 Chrome→Windows 拖拽被组织安全策略拦截，非 KPanel 主链路故障 |
| 数据、配置与迁移 | 已验证 | 生产停写备份归档可读、SHA256SUMS 全部 OK；升级前后 Compose、`.env`、apps 哈希一致 | 未执行业务数据写入 |

## 自动门禁与发布产物

- 候选 CI：run `32206798650` success。
- 候选 dependency freshness：run `32206798628` success。
- 主线 CI：run `32207067653` success；主线 dependency freshness `32207067744` success。
- Tag dependency freshness：run `32207230748` success。
- Release workflow：run `32207230954` success；源码、安全、依赖、Agent/Node 构建、双架构 OCI、latest promotion、Release 发布和候选分支清理均成功。
- `origin/main`：`67f19e7e6db6ad2fc13f5a66aee56838819e4b39`；`v0.86.1^{}` 同 SHA；远端候选分支已按 workflow 清理。
- GitHub Release：[v0.86.1](https://github.com/kejilion/KPanel/releases/tag/v0.86.1)。
- 公开 OCI：`docker.io/kjlion/kejilion-panel:0.86.1` 与 `latest` 均为 `sha256:232045ade043d571d3f51d2c07fcf82c3d4ebab324e9ec56c96ff357c0a1bf11`，含 linux/amd64 与 linux/arm64。
- 公开镜像标签核对：`org.opencontainers.image.version=0.86.1`、`org.opencontainers.image.revision=67f19e7e6db6ad2fc13f5a66aee56838819e4b39`。
- 受管脚本契约沿用 v0.86.0，未变更 revision/hash。

## 154 真机与生产部署

- 目标：仅 `arena-154`；`prod-108` 明确禁用，本轮未连接、未测试、未备份、未部署、未核对。
- 部署前：Panel v0.86.0 healthy，Agent active，restart=0、OOM=false；旧镜像 `sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8`。
- 新停写一致性备份：`/root/kpanel-backups/pre-v0.86.1-20260819T021524Z`；包含镜像、Compose、`.env`、`agent.env`、Agent unit、面板数据与 `/root/apps/kpanel.conf`；`sha256sum -c SHA256SUMS` 通过，数据/镜像归档可读取。
- 标准更新入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后：Panel `0.86.1`，Agent 日志报告 `version=0.86.1` 且 systemd active；`/api/v1/health` 连续 3 次 `status=ok`；容器 healthy、restart=0、OOM=false；近 3 分钟无 panic/fatal/OOM。
- 升级前后哈希保持：Compose `4f47f9ffdd63b8a5082447dee80b7b574a086ab3f2daac3b15395bf9f2a4184d`、`.env` `0ba468d8031cd5f67c5a7ffb0333c13dabd1c72c5223f7004df0084712a096f4`、`/root/apps/kpanel.conf` `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`。
- 本次生产写入仅限标准应用更新产生的新镜像/容器、Agent 二进制和 systemd 运行文件；未执行业务数据、SSH、防火墙、账户或 Docker 项目写操作。

## Windows 拖拽现场验证

- 用户现场已验证：从 Chrome 拖入 Windows 的单文件与多文件 ZIP 均显示“贵组织屏蔽了此文件，因为它不符合安全政策”。
- v0.86.1 已同时写入 `DownloadURL` 与 `text/uri-list`；两者最终都进入 Chrome/Windows 原生下载边界，现象不是缺少 KPanel 拖拽协议。
- 归类：`browser-policy / organization-DLP`，不是 `product-hard-failure`；KPanel 不降低安全边界、不伪造下载、不绕过 Chrome 企业策略。
- 可行后续：由管理员对 Chrome/组织 DLP 放行相应下载类型，或另行评估受管扩展/Windows 客户端；不能通过普通 KPanel 补丁安全解决。
- 右键下载、系统文件管理入口仍保留；本轮不新增代码、不改版本、不重发 Tag/Release。

## 回滚

- 成套回滚点：`v0.86.0`、OCI `sha256:d27e5fb6ec0d56cc6a862754bc2d21a4fbfd5608e79b2561304b670fb0f24ed8` 与备份 `/root/kpanel-backups/pre-v0.86.1-20260819T021524Z`。
- 回滚步骤：停止 Agent 与 Compose，恢复备份中的镜像/Compose/`.env`/Agent 文件和数据，执行 `systemctl daemon-reload`，再启动 Agent/Compose；核对 `/api/v1/health`、healthy、restart/OOM 和三项配置哈希。
- 本轮未执行回滚；当前 154 保持 v0.86.1 healthy。不得只换镜像或只改配置。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-19T09:54:40+08:00
- 候选冻结时间：2026-08-19T09:57:11+08:00
- 生产完成时间：2026-08-19T10:16:54+08:00
- 提交到生产用时：0.37 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

两次流程工具异常均未影响发布：本地 Windows linked-worktree 检查器输出 fatal 但 exit 0；GitHub REST 轮询曾遇限流，最终以 GitHub Actions job 结果复核。

## 遗留风险与后续准入

- 已知边界：用户现场 Chrome→Windows 拖拽受组织安全策略拦截；未做长时 soak。该限制需组织策略或独立受管客户端方案处理。
- 108 永久不纳入 KPanel 测试、灰度或部署。
- 若后续组织策略放行后仍出现产品错误，再收集脱敏行为证据并基于最新 main 制作新的最小补丁；不得覆盖本 Tag/Release。
