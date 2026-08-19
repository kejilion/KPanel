# KPanel v0.75.0 上线验收记录

日期：2026-08-15（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.3

候选提交 / 标签：`de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad` / `v0.75.0`

上一稳定版本 / 回滚点：`v0.74.1` / `sha256:068c69a4c70c15656535cb78f7c51f8199d66f35391a546030f5157edead8279`

结论：通过

## 发布画像

- 业务域：集群配对、文件管理器、桌面文件工作区、应用市场集成。
- 变更面：跨 KPanel 文件读取协议、Agent/Panel 流式传输、文件与桌面拖放、权限与失败恢复。
- 受影响用户旅程：已配对 KPanel 之间复制文件或目录；从远端文件窗口拖入本地目录或桌面；同名、冲突、取消、部分失败及重启恢复；应用市场在线目录中的 DeepSeek Harness 展示。
- 未变化契约：数据库 schema、端口、Compose、Agent 特权、受管 `kejilion.sh` 和应用市场打包格式均未改变；跨主机操作只复制，不移动或删除源文件。
- 风险等级及理由：L3；新增双端文件流和宿主机目标写入，必须验证权限最小化、归档边界、原子提交、失败清理、双向互通与旧配对兼容。

## 发布范围与未纳入内容

- 用户可见更新：Cluster v2 新配对可授权跨主机文件复制；文件管理器和桌面支持远端文件/目录拖放、逐项进度、取消、同名副本、部分成功及明确错误反馈。
- 精确提交清单：`92ac91808b3acb143a5e51e20c9ff541a073092a`（与源提交 `fd82a70fe18094aa74113f0363879245af6d42aa` patch-id 一致）及版本冻结 `de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`。
- 应用市场纳入方式：在线动态目录、同源图标和 `builtin-116` / `deepseek-harness` 作为本版集成旅程；`packaging/kejilion-app/kpanel.conf` 与 `kejilion/apps` 正式 blob 相同，因此没有制造空提交。
- 明确未纳入：其他旧工作树、未提交草稿、重复补丁、预览临时文件及未通过冻结审计的实验。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | arena-154 两套真实 Panel/Agent 完成 B→A 文件/目录、A→B 文件、同名副本、桌面提交后建入口；Chrome/Firefox 真实交互通过 | 未在生产业务目录执行互传写验收，避免制造业务数据 |
| 网络入侵与供应链安全 | 已验证 | 新 scope `cluster.files.read`、旧配对拒绝、路径/资源版本/归档条目与字节预算、symlink/特殊文件/穿越拒绝；govulncheck、npm audit、Trivy 源码与镜像通过 | 可信配对仍由管理员确认双方身份 |
| 稳定性、失败恢复与兼容 | 已验证 | stale version、取消清理、重启恢复、部分失败、旧配对不扩权；公开镜像 E2E；生产 healthy/restart 0/OOM false | 没有多小时 soak；文件传输是有界按需操作，采用完整失败注入和短健康采样 |
| 性能与资源预算 | 已验证 | 条目/单项/总字节和并发均有硬限制；隔离两套候选约 74–88 MiB/256 MiB；生产约 11 MiB/256 MiB、8 PIDs | 未用生产业务大文件做吞吐基准 |
| 用户体验与可访问性 | 已验证 | Chrome 151、Firefox 153；390×844 无横向溢出；进度/成功/错误态、桌面拖放和无凭据描述符通过 | Firefox 极快传输的瞬时成功提示可能在自动化读取前消失，目标行和内容为权威结果；反馈单元测试已覆盖 |
| 数据、配置与迁移 | 已验证 | 无 schema/配置迁移；停写备份、独立解包、文件树摘要、Compose、SQLite、镜像加载均通过 | 旧配对需重新配对才能获得新 scope，这是明确的最小权限设计 |

## 自动门禁

- 定向与全量结果：Go 全包、核心包 race、vet 通过；Web 97 个文件/715 项测试、i18n 2189、typecheck、生产构建通过；Linux amd64/arm64 Panel、Agent、Node、kpctl 构建通过。
- `make verify-release`：固定 Linux Runner 从冻结 SHA 完整通过；日志 SHA-256 `5dca4fc88a49dc12b0bb08e9c5a6f53d60f213689365d20737797060691ef126`。
- 首次 L3 Runner 由登录 shell 重置镜像内 PATH，preflight 在任何源码门禁执行前以 exit 127 停止；改用固定 entrypoint/PATH 后从同一冻结 SHA 从头重跑并通过，未跳过测试或修改候选。
- 候选 CI `31888422053`、候选依赖治理 `31888422051`：成功。
- 主线 CI `31888661340`、主线依赖治理 `31888661342`：成功。
- Release workflow `31889021788`、tag 依赖治理 `31889021843`：成功，均绑定 `de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`。
- 安全扫描与产物：govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/镜像高危与严重问题 0；正式 OCI revision/version 与 tag 精确匹配。

## 依赖与技术栈变化

- `make dependency-report` 在本版 L3 中生成；候选、主线和 tag 三层 `Dependency freshness` 均成功，检测源与锁定元数据完整。
- 本版没有新增或升级 Go/npm 依赖、工具链、基础镜像、Action、扫描器或受管脚本。
- 现有 Go 1.26.6、固定基础镜像与供应链治理继续生效；受管脚本仍固定 `kejilion/sh@28f89c1b34df4b25e6ef9b144c328fdea75dbac9`，原始 SHA-256 `0583f7cd5be1f0bb6ec48d92e2cf224bfabfafada5788658bda4414ba9561229`。
- 没有暂缓或拒绝的依赖候选；本版回滚不需要变更锁文件或脚本真源。

## 隔离真机与浏览器验收

- 主机/环境：`arena-154`，Linux amd64、Docker；环境策略允许 candidate-validation、production-deploy 和 production-safety-check。
- 使用的候选：源码 `de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`；隔离镜像 `sha256:12b4375128f9a806402273b0fef8d06cfe9d4494ab29c15667fe4b4a9fd37e8f`，version/revision 标签精确匹配；公开 OCI 随后另行执行 `image_e2e=pass`。
- 双端真链路：新配对 scope 精确为 `cluster.summary.read cluster.terminal.open cluster.files.read`；B→A 文件/目录、A→B 文件、同名不覆盖、stale version、symlink 拒绝、取消清理和重启恢复全部通过；两套 Panel healthy、0 重启、无 OOM。
- 旧配对兼容：v0.74.1 旧 scope 保持不变，`fileTransferAvailable=false`，未授权传输被拒绝且候选目标未变化。
- 真实浏览器：Chrome `151.0.7922.138`、Firefox `153.0`；13 项检查通过，跨面板拖放描述符不含 token，复制内容正确，390×844 无横向溢出，桌面只在真实提交后创建入口，页面错误 0。
- 应用市场：在线目录 `catalogMode=live`、153 项；DeepSeek Harness 为 `builtin-116` / `deepseek-harness`，图标使用同源动态入口。
- 无 soak 依据：本版没有常驻新服务或高频后台任务；流式操作有硬预算、取消和失败清理，已用双向、冲突、异常归档及重启旅程覆盖主要风险。
- 证据：本地 `C:\GitHub\_release-artifacts\v0.75.0`；跨面板汇总 SHA-256 `42e979419aa29af07e440afe79c511d681838a3cddc10d02aff53aedc0fc1372c`，旧配对汇总 `1734c8a4d2a512eaca9426faaa1262e1b55d0c95269da558aa0d4093b5f4a618`，浏览器结果 `553ec1335945240de939c7d8721cb4386c5d1ad5d55f30e15a28adc641945cce`。

## 发布产物与公开仓库复核

- Release commit：`de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`；annotated tag object：`74096e3996e498d35721067e09f320b4f389e698`。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.75.0>，Latest、非 draft、非 prerelease；八个显式附件及 GitHub 自动源码归档可用。
- OCI index（`0.75.0` 与 `latest`）：`sha256:4a19a1e1624d454d4817b60973ed9ae37b1e7d45164a08a4d9b868a174575724`。
- `linux/amd64`：`sha256:79034dfbd975968c0a516a78a33601aab1f6fcabcdc1abb42419162f3b07564f`；`linux/arm64`：`sha256:7c2212a46ec8a12fe1d3a89e6999262d0a81520b04e62111cd8655f7935bbf85`。
- `SHA256SUMS` SHA-256 `60b4bff76fd5663e2d89c25893c91cf374af6a9255a0482abc1bfce6b94e3997`；公开下载的部署包校验为 `aded8e299df2f0555011b3870253c443182929aaa3d1783d2b41cfc7fcb4fb42`，与清单一致。
- 公开镜像按不可变 index 在 arena-154 执行 `image_e2e=pass`，运行标签为 version `0.75.0`、revision `de5a9b7af17ba9d69eeaa11a346fb3102f8b33ad`，临时容器与网络已清理。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob `34316059d4e42f527819bc7d56e0ff14ec434c96` 与 KPanel 冻结源码完全一致；完整 lifecycle 已通过，无 apps 空提交。

## 生产部署安全核对

- 生产目标与授权：唯一目标 `arena-154`；只执行停写备份、标准应用市场更新和最小生产安全核对。
- 验证/灰度环境：`arena-154`；正式部署环境：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未检查、未备份、未部署、未升级、未核对。
- 升级前：v0.74.1、OCI `sha256:068c69a4c70c15656535cb78f7c51f8199d66f35391a546030f5157edead8279`，Panel healthy、restart 0、OOM false，Agent active。
- 停写备份：`/root/kpanel-backups/v0.75.0-preupgrade-arena154-20260815T142528Z.tar.gz`，SHA-256 `cf785268cdf62e884d771eb50d0f60722b2863df5d24cb35af2ecf2a3d3f9508`；旧镜像归档同名 `.image.tar`，SHA-256 `a0736f6a9887f58b1598b9d153c17114609acd1042348252a735cf96a3f34f28`。
- 备份恢复核对：在独立目录实际解包，文件树摘要、`.env` 权限、Compose、SQLite 完整性通过；旧镜像归档已实际 `docker load` 并核对 v0.74.1/revision。
- 第一次标准更新在进入镜像更新事务前被 `/root/apps/kpanel.conf` 本地修改安全拦截；该工作文件 blob 与最新 apps main 和冻结契约三方一致。先备份为 `/root/kpanel-backups/v0.75.0-preupgrade-arena154-20260815T142528Z.apps-kpanel.conf.local`（SHA-256 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`），仅恢复该文件索引并将 apps 仓库 `--ff-only` 到 `6d86eee…`，内容未丢失。
- 部署入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`；第二次执行成功，更新日志 SHA-256 `58e7f607cbcff92ff97d4368a958bf8cc9507fe753db83495580ac43d99be88e`，未触发应用回滚。
- 升级后：v0.75.0，运行 OCI/revision 与公开产物一致；Panel healthy、restart 0、OOM false；Agent active 且 `0.75.0 v1alpha1`；Compose、Panel-Agent、固定脚本、SQLite、错误日志和两轮 5 次短采样通过，约 11 MiB/256 MiB、8 PIDs。
- 生产已执行写操作：停写备份、apps 仓库安全快进、标准 KPanel 更新；未主动修改生产业务文件或配对。
- 仅在隔离真机执行：跨主机复制、冲突、取消、恶意归档、旧配对拒绝、重启恢复及真实浏览器拖放。
- 生产证据：`/root/kpanel-release-evidence/v0.75.0/arena154-20260815T142528Z`，`SHA256SUMS` 文件摘要 `a917a5541db838b3db6f1ee3e8f935595bcd5a535f9dc9163fbc9d85f30a2f7a`。

## 回滚

- 源码/tag：`v0.74.1` / `94a1109b8b991928ce2e3fcd5b74e4ffac85c9c2`。
- 镜像 digest：`sha256:068c69a4c70c15656535cb78f7c51f8199d66f35391a546030f5157edead8279`。
- 数据/配置备份：上述停写备份；镜像归档已验证可加载，独立恢复目录校验通过。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.74.1 镜像归档，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `daemon-reload`，启动 Agent 与 Compose，再核对版本、Panel-Agent、SQLite、restart/OOM 和日志。
- 回滚后生产实际版本与健康状态：不适用，本次未触发生产回滚；当前 v0.75.0 healthy。
- GitHub Latest、Docker `latest` 与标准更新入口均指向已通过生产核对的 v0.75.0，公共默认更新通道无需恢复。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-15T20:41:53+08:00
- 候选冻结时间：2026-08-15T21:10:28+08:00
- 生产完成时间：2026-08-15T22:39:22+08:00
- 提交到生产用时：1.96
- 是否回滚、紧急热修复或重复发布：否（首次标准更新由本地同内容应用配置安全拦截，未进入镜像更新事务；安全快进后同一正式版本一次部署成功）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与后续准入

- 升级前旧配对不会静默获得 `cluster.files.read`；需要文件互传时必须重新配对，这是已验证的最小权限边界。
- 没有在生产业务目录进行大文件或目录互传；双向复制、异常归档、取消、重启和浏览器旅程已在 arena-154 隔离实例完成，生产只做安全核对。
- 应用市场在线目录不可达时按既有契约回退内置目录；本次 live 目录和 DeepSeek Harness 已在隔离真链路验证。
- 本轮复用既有 `release-kpanel` v2.3，不新增重复工作流；验收事实沉淀于本文件，上线后按用户要求不执行电脑睡眠。
