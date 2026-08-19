# KPanel v0.80.1 上线验收记录

日期：2026-08-16（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`9f515017f7a576eb4c134c116e2141b881390eb9` / `v0.80.1`

上一生产版本 / 回滚点：`v0.80.0` / `sha256:206ebd3571432cd91ec15b6cd8285a199fcfa0f5f5d3a18e5edd217e430383ef`

结论：通过

## 发布范围

- 修复桌面文件或目录快捷方式快速拖放时，原生 `drop` 已发生但 `dragend` 尚未回传 `dropEffect`，导致最终位置偶发未持久化的问题。
- 跨 KPanel 复制仍不会被误判为本地移动；拖离本桌面期间隐藏本地预览，返回或结束后完整恢复。
- 版本与 CHANGELOG 更新到 v0.80.1；无数据库、API、配置、应用市场契约或托管脚本迁移。
- 已盘点 KPanel、`kejilion/sh` 与 `kejilion/apps`：本次仅上述 KPanel 聚焦补丁未上线；历史工作树、已上线提交、被替代候选和未形成独立交付的实验均未混入。

## 自动门禁

- Web：98 个测试文件、740 项测试通过；i18n 2254 条文案、20 个按页加载 catalog；typecheck、生产构建通过。
- Go：全包测试、核心包 race、vet、linux/amd64 与 linux/arm64 构建通过。
- 安全与供应链：govulncheck 可达漏洞 0、npm audit 0、Trivy source/image HIGH/CRITICAL/secret/misconfiguration 0；受限容器、安装安全、版本、治理、依赖、业务上下文、托管脚本和应用生命周期门禁通过。
- 最终 bundle：`kpanel-v0.80.1-9f51501.bundle`，SHA-256 `0fb1cda6b396356b32c9521ea5b91c903aec9856c06a3d7fbc521229ddadc878`。
- arena-154 最终 L3：`L3 release verification completed`；日志 SHA-256 `5c709d8f8dadd8563557b95ec01ebb4794b8824c4ebd71a0097c332fd5324abb`。
- 通用 L3 验证镜像缺少精确 revision 元数据，按 fail-closed 规则未用于真机候选；从冻结提交重新构建 `candidate-v0.80.1-9f51501`，核对 version=`0.80.1`、revision=`9f515017f7a576eb4c134c116e2141b881390eb9` 后才继续。

## 隔离真机与浏览器验收

- arena-154 使用正式 Google Chrome 151.0.7922.138、独立临时 Profile 和隔离 Panel/Agent 验证真实原生拖放链路。
- 快速 `dragend` 回退会持久化最终位置且只写入一次；跨面板 copy 不触发本地 move；本地接收蒙层保持隐藏；原生拖离期间源项隐藏，返回及结束后状态完整清除。
- 产品控制台实质错误 0；浏览器结果 SHA-256 `fb2e8b64ca5d98656dbe3c67cd6d8e11dd01a6f46cd946f4a05aa9ff29809e34`；截图 SHA-256 `35067567844cec919b5d7afa113176208e97835c804a2483477fa3e345165e18`。
- 最初合成 `DataTransfer` 方案因 Chrome 不允许脚本可靠改写 `dropEffect` 而被判为证据通道无效；改用真实原生拖动至受控 copy 目标复验，候选代码未因此修改。隔离容器、网络、隧道和临时 Chrome Profile均已清理。

## GitHub、Release 与公开产物

- source tree：`42bf1ad70a7c692e6f52d1ea480ee3a4232d374e`；annotated tag object：`fddf729b752259bbd782413c5bc31175f55d6059`。
- 候选 CI `31931944810`、候选依赖治理 `31931944825`、主线 CI `31932112767`、主线依赖治理 `31932112761`：success，head SHA 精确匹配。
- Release workflow `31932303077`、tag 依赖治理 `31932303152`：success。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.80.1>，非 draft、非 prerelease，8 个附件完成公开校验。
- Docker `0.80.1` 与 `latest` OCI index：`sha256:54f44fce245d8f03991c7880a6c49a065f3662335375b9d4c4a6349d8083700f`。
- `linux/amd64`：`sha256:c86dd2dc8d68b136a30162a2d99d9d4d53b0dbe53df9136f8d185b5d250b94af`；`linux/arm64`：`sha256:ecd8d2ba3ddeab9abcd24cd78aedc84c35b55803f5e19e3b8b19c521298db111`。
- 公开镜像重新拉取受限容器 E2E 通过，version/revision 与正式提交一致；E2E 日志 SHA-256 `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；与 v0.80.1 packaging 契约一致，因此未制造空提交。
- `kejilion/sh main@c5fc243d4a7891fd66f92e36204a17584afb1535`（`v4.5.7`）已包含此前 DeepSeek Harness 等脚本交付，本轮无新的未上线脚本差异。

## 生产部署与健康

- 唯一 KPanel 生产目标：`arena-154`；`108` 按用户长期约束未连接、未测试、未备份、未部署。
- 部署前：v0.80.0，Panel healthy、restart 0、OOM false，Agent active；运行 OCI/revision 为 `sha256:206ebd3571432cd91ec15b6cd8285a199fcfa0f5f5d3a18e5edd217e430383ef` / `88bdb3cf13e125acaba53ba5e2ff80a3af2cece7`。
- 停写备份：`/root/kpanel-backups/v0.80.1-preupgrade-arena154-20260816T065847Z.tar.gz`，SHA-256 `e16191580ea84927dc8910bc2899d39cb730ea661469a52d7672f299f3bf9e3f`。
- 旧镜像归档：同名 `.image.tar`，SHA-256 `5e048764f1781360809824876ffdc8c1b4217f37c8e159bf7193799b6f23d9fa`；manifest SHA-256 `61467bcd7857a44614f3e8013f0fcf5bd591cf3b489276ff3d4c6b0ddc13314f`。备份已独立解包并核对文件清单、`.env` 权限、Compose、Agent unit、两个 SQLite 库和镜像可加载性。
- 标准应用入口完成更新；部署日志 SHA-256 `44b684d791648fd5137e5088a4d30fed1d8e67b0479be2c46b64092cd57e2842`。
- 部署后：Panel/Agent 均为 v0.80.1；运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active。
- Compose、Panel-Agent、两个 SQLite 库、错误日志及五次健康/资源采样通过；Panel 约 74.99 MiB/256 MiB、7 PIDs，CPU 0.01%–0.06%，磁盘剩余约 13 GiB。采样日志 SHA-256 `43383c9a04e16c23216369802822c6b92c5a06560e10a06d7bd65cfce83c3d0f`。
- 已安装托管脚本会保留本机偏好，其 SHA 在更新前后保持 `d73231f146f7398d7b50133695faf2116134fbfe33a7b94068e277cc7b82df55`；初始审计错误地要求它等于镜像内原始脚本摘要，现已按安装契约修正并独立复核通过，未重复部署。

## 回滚

- 源码/tag：`88bdb3cf13e125acaba53ba5e2ff80a3af2cece7` / `v0.80.0`。
- 镜像 digest：`sha256:206ebd3571432cd91ec15b6cd8285a199fcfa0f5f5d3a18e5edd217e430383ef`；旧镜像已形成独立归档。
- 成套材料：上述停写备份、旧镜像归档、manifest、metadata 和文件摘要；恢复材料、SQLite 及镜像加载已独立验证。
- 回滚必须停止 Panel 与 Agent，加载并使用精确 v0.80.0 镜像，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `systemctl daemon-reload`，再启动 Agent 与原 Compose，并核对版本、Panel-Agent、SQLite、restart/OOM 和日志；禁止只切换浮动 `latest`。
- 未触发正式回滚；回滚点保持可执行。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-16T14:14:40+08:00
- 候选冻结时间：2026-08-16T14:22:47+08:00
- 生产完成时间：2026-08-16T15:05:09+08:00
- 提交到生产用时：0.84 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与沉淀

- 跨操作系统窗口的原生拖影仍由浏览器/窗口系统绘制，无法与应用内 pointer 预览像素级一致；本轮只修正业务落点、跨面板语义和状态清理。
- 本补丁为确定性拖放竞态修复，未安排长时间性能观察；完整 L3、真实 Chrome 原生拖放、公开镜像 E2E 和生产五次健康采样均通过。
- 本轮复用 `release-kpanel` v2.4 与既有项目版本治理，没有新增重复工作流或知识文档；验收方法修正与完整回滚材料记录在本文件中。
