# KPanel v0.83.0 上线验收记录

日期：2026-08-18（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.6

候选提交 / 标签：`202a7f912d8462948657fd19fec69a89d0620338` / `v0.83.0`

上一稳定版本 / 回滚点：`v0.82.0` / `sha256:76acd065a99b296c64712d8db611e337b38727018869f073940dabb82c749c61`

结论：通过，`arena-154` 已部署，未回滚；`prod-108` 未连接、未测试、未部署。

## 发布画像

- 业务域：Docker 容器部署、Compose 项目管理与容器列表交互。
- 变更面：展示、宿主机 Compose 配置写入和部署；新增项目 `.env` 管理，不改变 Panel/Agent 网络协议、端口或权限模型。
- 受影响用户旅程：Docker Run/Compose 粘贴诊断、精确错误定位、Compose/独立容器编组、项目收起展开、项目变量编辑、生命周期、重新部署、失败恢复和零容器项目恢复。
- 未变化契约：认证、端口、Agent 权限、`kejilion.sh`、KPanel 应用市场安装/更新入口和 `packaging/kejilion-app/kpanel.conf` 未变化。
- 风险等级及理由：中高；涉及宿主机 Compose 与 `.env` 原子写入、Docker 生命周期和失败回滚，因此按 L3、真实 Docker 和生产隔离项目验证。

## 发布范围与未纳入内容

- 用户可见更新：语法错误行自动短暂呼吸并保留浅红标记；中等宽度操作列不再裁切；Compose/独立容器组可收起，Compose 组使用稳定低饱和强调色和 120ms reduced-motion 兼容动画；项目变量可安全保存到标准 `.env`；无容器项目仍可管理和恢复。
- 精确提交清单：`51abb412`、`192033e8`、`b3c0d0f4`、`d56b5316`、`909f01d9`、`68d4db1d`、`aee8f4ea`、`104fb060`、`e803e731`、`ad0398fd`、版本提交 `202a7f91`。
- 最终净差异：26 个文件，新增 1044 行、删除 104 行；最终提交相对 `v0.82.0` 线性可重放，工作树干净。
- 明确未纳入：未提交草稿、其他活动分支、后续无关功能、依赖升级、`kejilion.sh` 改动和任何 `prod-108` 操作。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Web 100 文件/759 测试；真实 Docker 集成；154 Panel→Agent→Docker 生产隔离项目完整闭环 | 未对现有业务 Compose 执行写测试，避免破坏生产数据 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck` 可达漏洞 0、npm audit 0、Trivy source/image 漏洞/secret/misconfiguration 0；公开 OCI revision 精确匹配 | 本版无新外部网络入口 |
| 稳定性、失败恢复与兼容 | 已验证 | Compose 与 `.env` 失败后共同回滚；零容器项目保留/重建；stale resourceVersion 拒绝；生产 5 次健康采样通过 | v0.82.0 回退后新 `.env` 仍留在项目目录但旧版本忽略，不构成数据丢失 |
| 性能与资源预算 | 已验证 | 动画仅 120ms opacity/translate3d，reduced-motion 禁用；生产快照 0.03% CPU、71.11 MiB/256 MiB、7 PIDs、0 restart、无 OOM | 未做长 soak；本版无新常驻任务，确定性状态与资源采样足够 |
| 用户体验与可访问性 | 已验证 | 正式 Chrome 151，桌面/390px、亮暗主题、键盘收起、错误行精确选中、reduced-motion 和无横向溢出共 31 项通过 | 真实浏览器在隔离候选完成，生产复用同 revision 正式 OCI并做后端闭环 |
| 数据、配置与迁移 | 已验证 | `.env` 0600、24 KiB/无 NUL 边界、同目录原子写、资源版本、失败回滚；停写备份及 SQLite 原始/恢复/上线后检查通过 | 无数据库 Schema 迁移；管理员应继续妥善管理项目变量中的秘密 |

## 自动门禁

- 定向测试及结果：Docker/Compose 单元和 Web 布局测试、真实 Docker `TestComposeLifecycleAgainstDocker` 均通过；真实 Docker日志 SHA256 `4f7ee8c46b7e6243cfab72fad7904780ea8ed26bef6502793f1da0722e3ec54c`。
- `make verify-release`：`arena-154` Debian 13 / linux-amd64，Go 1.26.6、Docker 29.6.2、Compose 5.3.1；Runner image ID `sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`，输出 `L3 release verification completed` 和 `release_gate_runner=pass`。
- L3 日志 SHA256：`e361024a7b2e5791d5b0cf42c2e34062ab0805ca9715349dca32414cfb79ab43`；bundle SHA256：`b07a24712a83159c450fcf01ce5c172f1579d2a833bed1edbebacf4d2572c33a`。
- 候选 CI：[`32048959886`](https://github.com/kejilion/KPanel/actions/runs/32048959886) success；候选依赖治理 [`32048959985`](https://github.com/kejilion/KPanel/actions/runs/32048959985) success。
- 主线 CI：[`32049286868`](https://github.com/kejilion/KPanel/actions/runs/32049286868) success；主线依赖治理 [`32049286784`](https://github.com/kejilion/KPanel/actions/runs/32049286784) success。
- Release workflow：[`32049787622`](https://github.com/kejilion/KPanel/actions/runs/32049787622) 的构建、扫描、8 个资产与 OCI 发布均成功，最终将 draft 转公开时 GitHub API 返回 HTTP 503；受限恢复工作流 [`32051288334`](https://github.com/kejilion/KPanel/actions/runs/32051288334) 对精确 Tag、版本、draft 和 8 个资产 fail-closed 复核后成功发布，不修改 Tag 或产物。
- 安全扫描、镜像契约、SBOM/provenance：Go full/race/vet、linux/amd64 与 linux/arm64 构建、govuln、npm audit、Trivy、受管脚本契约和 app lifecycle 全部通过；双架构 OCI 均带 attestation。

## 依赖与技术栈变化

- `make dependency-report`、依赖策略 9 组和候选/主线/Tag 三层 dependency freshness 均在 2026-08-18 发布窗口通过，检测源完整。
- 最近每日安全通告审计和 EOL 检查由 v2.6 L3 门禁执行；`govulncheck` 可达漏洞 0，npm audit 0，Trivy source/image 0。
- 本版没有依赖、工具链、基础镜像、Action、扫描器或受管脚本升级；`web/package*.json` 仅同步项目版本 0.83.0。
- 构建工具链：Go 1.26.6；固定 release-gate runner `sha256:b593c0f...`；Trivy 0.72.0。Trivy 提示上游 0.74.0，仅作为独立依赖候选，未夹带升级。
- 升级后的构建、安全、兼容和回滚门禁均通过；无暂缓或拒绝的本版依赖候选。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时：`arena-154`，Debian 13、x86_64、kernel 6.12.96、Docker 29.6.2、Compose 5.3.1、Go 1.26.6。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`、`browser-validation`、`production-deploy`、`production-safety-check`；两项部署策略检查均 pass。
- 使用精确候选 `202a7f912d8462948657fd19fec69a89d0620338`；隔离镜像 `sha256:a0a91986c697ead157ca3bb79c0bd7d9f36f590cf1b0be287d1273bbb786766f` 的 version/revision 精确匹配。
- 后台作业终态：L3、真实 Docker、候选镜像和浏览器脚本均 exit 0；证据目录 `/root/kpanel-release-evidence/v0.83.0` 与 `C:\GitHub\_release-artifacts\v0.83.0`。
- 浏览器：Google Chrome 151.0.7922.138，31 项通过；证据 `browser-docker-compose-r3/result.json` SHA256 `fe05614e819212b5f6ece0027a1170d5975c82e271c0beea384aabb7cb5c2e26`。
- 受影响旅程：编组默认展开、鼠标/键盘收起、稳定强调色、操作列边界、`.env` 隐藏/显示/更新、失败回滚、零容器重建、错误行动画/持久状态/reduced-motion、390×844 无溢出；页面错误 0。
- 宿主机写入、失败注入和回滚：仅隔离项目；无效镜像触发失败并恢复 Compose、`.env` 和运行服务；临时容器、网络和目录均清理。
- 无 soak：本版没有新增常驻任务或协议；以全量测试、真实 Docker 事务、资源采样和确定性失败恢复作为准入证据。

## 发布产物与公开仓库复核

- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.83.0>，非 draft、非 prerelease，8 个附件公开；annotated tag object `d0e5a0f9788b661dc90940efc143e4ac315bbb49`，peeled commit 为精确候选。
- Docker `0.83.0` 与 `latest` OCI index：`sha256:bfa0c98aca0d22b4fd8e6ac14507572b491c5628caf5b3924ebd230cfea4afd2`。
- `linux/amd64`：`sha256:2ef3addd2c86716bf533ae8ecedb9f43b413915999fcef40fe61590ad90ff4f6`；`linux/arm64`：`sha256:69ebb3426ccf180b4824ae38608a8ac7ecf90587e008a163edaf9b328b764e71`；另含对应 attestation manifests。
- 附件：双架构 Agent、双架构 lightweight node、部署归档、LICENSE、`SHA256SUMS` 和 `THIRD_PARTY_NOTICES`。
- 公开镜像在 154 重新拉取并运行 `packaging/tests/image-e2e.sh`，输出 `image_e2e=pass`；日志 SHA256 `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`。
- `packaging/kejilion-app/kpanel.conf` 相对 v0.82.0 无差异；KPanel 与 `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，无需空提交；`kejilion.sh` 契约未变化。

## 生产部署安全核对

- 生产目标和部署授权范围：仅 `arena-154`；用户明确授权合并、发布与上线。
- 验证/灰度环境：`arena-154`；环境策略允许候选、浏览器和生产安全检查。
- 正式部署环境：仅 `arena-154`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前：v0.82.0，revision `4b4469128feeed6ee54d48ff74c024dd7a5acb89`，运行镜像 `sha256:76acd065a99b296c64712d8db611e337b38727018869f073940dabb82c749c61`；Panel healthy、restart 0、OOM false，Agent active。
- 停写一致性备份：`/root/kpanel-backups/pre-v0.83.0-20260817T175359Z`；应用归档 SHA256 `713b145e4a6222fbe5de216db9351ad16c3683a21b0f6e3c3cc757ca9bfb5cb0`，旧镜像归档 `4760823a61a73b520fdcc7e294f102505f78677f343405fd22d77521cf82c27b`，`kpanel.conf` `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`。
- 备份已独立解包逐文件比对，原始/恢复 SQLite 均 `quick_check=ok`；服务恢复到 v0.82.0 healthy 后才执行升级。
- 部署入口：服务器现有 `/root/apps/kpanel.conf` 的标准 `docker_app_update` 事务；未直接改 Compose 或数据，更新日志 SHA256 `7d4385212585d7a7986e7c71573d737c991452cd07d3ff20827b2f51c5574b13`。
- 部署后：v0.83.0、revision `202a7f91`、正式 OCI digest 精确一致；Panel healthy、Agent active、restart 0、OOM false；5 次健康采样和 SQLite `quick_check=ok`。
- 生产专项写验收：仅临时 `kpanel-v0830-prodcheck`，完成 `.env`、生命周期、重部署、失败回滚和零容器恢复后已清理；摘要 SHA256 `33ca71db75885e0ef6f67ac6e5df95bae4ca959259d41931df9a13a2e7527f5a`。
- 服务器既有 `/root/apps/kpanel.conf` 上线前后 SHA256 完全一致，确认未覆盖本地定制；未对任何现有业务 Compose 写入。

## 回滚

- 源码/tag：`4b4469128feeed6ee54d48ff74c024dd7a5acb89` / `v0.82.0`。
- 镜像 digest：运行时旧镜像 `sha256:76acd065a99b296c64712d8db611e337b38727018869f073940dabb82c749c61`，已导出到备份。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.83.0-20260817T175359Z`，包含完整 `/home/docker/kpanel`、旧 OCI、Compose、`.env`、Agent unit/binary/script、容器 inspect 和独立 `kpanel.conf`。
- 回滚步骤：停止 Panel/Agent，加载 `previous-image.tar`，成套恢复 `kpanel.tar.gz` 与 `kpanel.conf`，核对摘要和 unit target，`systemctl daemon-reload` 后启动，再复核 v0.82.0、SQLite、health、restart/OOM 和日志；禁止只切换浮动 `latest`。
- 本次未触发正式回滚；生产实际版本为 v0.83.0，健康正常。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向 v0.83.0 / `sha256:bfa0c98aca0d22b4fd8e6ac14507572b491c5628caf5b3924ebd230cfea4afd2`。
- 公共默认更新通道决策：不适用；生产验证通过，无需恢复上一稳定版。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-17T22:17:20+08:00
- 候选冻结时间：2026-08-18T00:23:56+08:00
- 生产完成时间：2026-08-18T01:58:17+08:00
- 提交到生产用时：3.68 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：11
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

生产写操作前 10 次流程异常分别来自 Windows linked-worktree 验证通道、两次 Runner PATH 预检、Compose
Buildx 插件预检、候选 OCI revision 预检、Agent 夹具组权限、浏览器夹具路径、reduced-motion 取证阈值、
GitHub Release draft 发布 HTTP 503，以及不可用的直接重跑/浏览器恢复通道；均在对应门禁处停止并以精确
候选重验或受限恢复工作流闭环。生产写操作后 1 次为健康采样后 SQLite 只读汇总命令的 here-doc 引号错误；
5 次健康采样已经成功，随后改用经 `bash -n` 的独立只读脚本补齐并通过。以上均未造成产品退化、数据写入
失败、回滚或逃逸门禁。

## 遗留风险与后续准入

- 未验证风险：未对生产现有业务 Compose 执行写操作；未做长 soak；这两项是保护业务数据和本版无常驻任务的有意边界。
- 已实现待实机准入：无；本版受影响事务已在隔离真实 Docker和生产独立项目验证。
- 不阻断本版的理由：全量 L3、安全扫描、双架构产物、真实 Docker、正式 Chrome、公开 OCI E2E、停写备份和生产隔离项目均绑定同一源码 revision 并通过。
- 后续应进入的自动门禁：持续保留 `.env` 原子写/权限/共同回滚、零容器项目和 DockerView 多视口回归；流程通道异常继续由发布流程指标单独追踪。
- 本次复用 `release-kpanel` v2.6、`project-version-control` 和项目既有真实 Docker/浏览器验收流程；没有新增重复工作流。
