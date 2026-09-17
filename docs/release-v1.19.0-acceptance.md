# KPanel v1.19.0 发布验收记录

日期：2026-09-17

发布级别：L3

候选提交 / 标签：`031e7b2616c383f006a583e44426f7948a53c8c7` / `v1.19.0`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`

`releaseChannel`：`stable`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 稳定版已在 Tag 包含全部候选提交后自动删除

## 发布画像

- 业务域：本版汇总 `1.19.0` 列车 rc.1-rc.5 全部变更：发布双通道与自动更新、轻量节点批量接入与安全修复、终端快捷命令与批量执行（含手动终止与完成判定收严）、集群通知与主机排序真源、桌面分屏（双侧与单侧）、备份中心视觉重排、协作门禁强化。
- 变更面：展示、交互、Panel 持久化数据结构、轻量节点 enrollment 密钥比较算法、更新通道选择；除标准应用市场更新事务外无宿主机写新增，无生产数据迁移。
- 受影响用户旅程：稳定版用户经应用市场标准入口从 v1.18.0 升级到 v1.19.0（本次已在 arena-154 生产完成）；预览用户继续经 `preview`/RC 通道。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限边界；应用市场默认入口仍指向稳定版。
- 风险等级及理由：中等偏上（稳定版直接面对生产与公共默认更新通道）；风险由 5 个 RC 的渐进验证（每个 RC 独立 L3 + 浏览器验收 + 公开镜像 E2E）、稳定版完整 L3 复验与生产三阶段证据链覆盖。

## 发布范围与未纳入内容

- 用户可见更新：完整清单见 `CHANGELOG.md` `[1.19.0]` 段（rc.1-rc.5 全量汇总）。
- 精确提交清单：`031e7b26`（版本准备），产品载荷为 `v1.18.0..v1.19.0` 全区间（rc.1-rc.5 验收记录已逐提交列出，此处不重复）。
- 明确未纳入的分支、文件或后续事项：`1.19.0` 之后的新功能将进入下一列车（`v1.20.0-rc.1` 起）；rc.3/rc.4 已吸收历史分支按用户决定继续保留为可选清理项。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`（变更集 `light-batch-enrollment-20260914`）。
- 变更集编号：`light-batch-enrollment-20260914`。
- KPanel 实际内置脚本基线 commit / SHA-256：应用市场脚本 `kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`（与公开镜像 `/release/kejilion.sh` 实测一致）。
- 脚本候选 commit / SHA-256：`kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`（轻量节点更新运行时来源，已随 rc.1 于 2026-09-14 先行发布在 `origin/main`, 根脚本 `a0e6cf5193aa9bfbdc572ad91803d4b53d34549ac95259b3288d436766a229dc`，rc.2-rc.5 与本版未再变化）。
- 状态判定依据与兼容性证据：`cmd/kejilion-node/update_runtime/source.json` 自 rc.1 起固定 `4d61f7ef`；本次生产更新经标准应用市场入口执行, 内嵌脚本一致性校验通过。
- 本版发布决定：脚本已于 rc.1 时先行发布, 本版直接发布 KPanel 稳定版。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | rc.1-rc.5 每 RC 独立 L3 + 验收；稳定版完整 L3 复验（全量 Go/前端测试 + `app_conf_lifecycle=pass`）；生产 postdeploy 健康 ok、SQLite quick_check ok。 | 真实 shell 菜单命令完成时序已随 rc.5 组件测试覆盖但未在真机菜单实测。 |
| 网络入侵与供应链安全 | 已验证 | L3 固定 Runner govet/govulncheck/npm audit/Trivy 全过；gosec v2.28.0 全量复核随 rc.4 进入；OCI revision/digest、内嵌脚本、`SHA256SUMS` 均按 index 摘要口径核对。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 应用安装/更新/中断回滚/卸载故障注入通过；生产更新事务含停写备份与失败回退能力（本次未触发回退）；公开镜像冷启动 E2E 通过。 | 未做 4 小时全时长生产 soak 与真实断电。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 采集 `df.txt`/`resources.json`，容器在低配资源约束下健康运行；构建测试无超时或 OOM。 | 真实规模节点长期 P95 与资源曲线未采集。 |
| 用户体验与可访问性 | 已验证 | rc.1-rc.5 累计验收级浏览器预览覆盖全部新旅程（快捷命令、批量执行、分屏、备份中心、集群页），稳定版变更仅版本字段。 | 200% 缩放与人工三语矩阵仍未单独执行。 |
| 数据、配置与迁移 | 已验证 | 生产更新前后受保护配置哈希 diff=0；SQLite quick_check ok；旧浏览器主机排序安全迁移随 rc.3 验证。 | 生产数据恢复演练未执行（备份产物已落盘可恢复）。 |

## 自动门禁

- 定向测试及结果：本版候选相对 rc.5 仅版本文件变化，产品载荷测试结论随 rc.5（组件测试全部通过）与稳定版完整 L3 复验。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_runner=pass commit=031e7b26`；`app_conf_lifecycle=pass`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-031e7b26-l3-r1`；plan `0b3bb9219bd2d4070dac446290090d9add51528c10e1a0ccb2b9465cc4efa8a7`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `c055fd7a3c84e094edd3eea697cca651d186acda0a8beb6ae440fcda4862838c`，远端日志 `c03dc2e16ef22c7636d8f122c494e0abc69084757c47bb903b2290806f30ef65`；2026-09-17T10:46:44Z 至 11:02:25Z，status `passed`、exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-031e7b26-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-031e7b26-l3-r1`。
- 候选 CI：[CI 35213701254](https://github.com/kejilion/KPanel/actions/runs/35213701254) 与 [freshness 35213701177](https://github.com/kejilion/KPanel/actions/runs/35213701177) 成功，均绑定最终 SHA。
- 主线 CI：[CI 35214820323](https://github.com/kejilion/KPanel/actions/runs/35214820323) 与 [freshness 35214820360](https://github.com/kejilion/KPanel/actions/runs/35214820360) 成功，均绑定最终 SHA；[tag freshness 35215503265](https://github.com/kejilion/KPanel/actions/runs/35215503265) 成功。
- Release workflow：[Release 35215503290](https://github.com/kejilion/KPanel/actions/runs/35215503290) 成功（2026-09-17T11:34:44Z 推送 tag 后完成）。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/最终镜像扫描为 0；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-17 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit、Trivy 与 gosec v2.28.0 全量复核（278 文件，随 rc.4）通过，无阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：`1.19.0` 列车未新增 Go/npm 依赖或基座升级。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions；受管运行时 `kejilion/sh@4d61f7ef`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0`；公共 OCI index（tag 权威口径）为 `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无。
- 升级后的兼容、安全、构建、性能资源和回滚结论：稳定门禁、公开 OCI 与生产三阶段全部通过；未触发回滚；公共默认更新通道已指向 v1.19.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation` + `production-deploy` + `production-safety-check`（后者为稳定版生产核对申请并通过）。
- 使用的精确候选或公开产物：`031e7b2616c383f006a583e44426f7948a53c8c7` 与 `docker.io/kjlion/kejilion-panel@sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`（多架构 index；amd64 子 manifest `sha256:023859e9...`，arm64 子 manifest `sha256:0fdd6b4d...`）。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-031e7b26-l3-r1` passed/0；公开 OCI E2E r1 exit 0，run.sh `b4a15005705a986baebdce3c8986de91ed732f849b9f6b892b0bbd6fc406611d`，固定 image-e2e `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 `26983d2abd7f974b3b0a04c437b239e31035fa92bfb1fab6cf56412230449562`，证据位于 `arena-154:/root/kpanel-release-evidence/v1.19.0/public-oci-e2e-r1`；浏览器验收结论随 rc.1-rc.5 验收记录（本版候选相对 rc.5 仅版本字段，无需重新预览）。
- 测试窗口/循环数及风险依据：单次公开镜像冷启动 + 生产单次更新事务；无 soak，长期风险由 5 个 RC 的渐进反馈与后续监控承担。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：全部随 rc 系列验收级预览覆盖；stable 候选无 UI 变化。
- 宿主机写入、失败注入、重启恢复和回滚结果：生产写仅标准应用市场更新事务（备份→更新→核对）；L3 覆盖更新失败/中断回滚；生产实际更新一次成功、未触发回退。
- 未执行场景及原因：真实断电、弱网、长期 soak、生产数据恢复演练、人工三语矩阵未执行。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0](https://github.com/kejilion/KPanel/releases/tag/v1.19.0) 为非 draft、非 prerelease、**GitHub Latest**；annotated tag 对象 `9b34879bb7f89b7bbc9c0adc2dde1e3a818e8e13` peeled 到最终产品 SHA `031e7b26`。
- Docker 版本与通道 OCI index（buildx 权威口径）：`1.19.0` = `latest` = `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`（本次发布提升 `latest`）；`preview` 保持 `sha256:647e5bad1a9ab5646bd7a036a05613d8e892bf09f07e0e6dd0f9b42bc36d4163`（rc.5 多架构 index），未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:023859e939861955eb11ebe25dec7f2475b2dd9a8bfbdf45505c8a7b232f8162`；arm64 `sha256:0fdd6b4d3372dd44864736e02a8a5be95cd62e7b853b0fee4ad76a4c3c1cf0c6`；attestation `sha256:3d2b0758...`/`sha256:2fdf6bc4...`，`unknown/unknown` 为 provenance/SBOM。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、deploy tar.gz、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES）。
- 公开镜像 `image_e2e=pass`：`arena-154` 从已验证 L3 bundle 检出源码（bundle SHA `c055fd7a` 核对一致）、以 amd64 子 manifest 摘要 `sha256:023859e9...` 拉取, `docker create`/`docker cp` 只读提取元数据（revision=`031e7b26`、VERSION=`1.19.0`、非 root `65532:65532`）后运行固定 `image-e2e.sh`；输出 `image_e2e=pass` 与 `public_oci_e2e=pass`，端口/容器/网络残留断言通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca` 的 `kpanel.conf` 归一化一致，无需应用市场提交；默认入口指向 `latest`（本次发布后即 v1.19.0）；`kejilion/sh` main 保持包含 `4d61f7ef`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest（本次发布后 v1.19.0），校验唯一官方镜像 digest：已随 rc.1 实现，生产更新事务实际执行验证。
- 本次发布提升 GitHub Latest 与 Docker `latest`，`preview` 通道未被改变：已验证（digest 复核）。
- 候选分支处置：`release/v1.19.0-candidate` 已由 Release workflow 在确认 Tag 包含全部候选提交后自动删除（稳定版规则）。

## 生产部署安全核对

- 生产目标和部署授权范围：`arena-154` 生产 KPanel 实例（用户全链路授权，含生产部署）。
- 验证环境（`environment-policy.json`）：`arena-154` / `production-deploy` + `production-safety-check` 用途门禁均 pass；`prod-108` 未连接。
- 正式部署环境：`arena-154`（默认，非 108）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight pass（2026-09-17T11:37Z，版本 1.18.0 健康核对一致）；backup pass（11:38:32Z-11:38:43Z，plan SHA `99738018...`→backup plan `18a85697...`），备份位于 `/root/kpanel-backups/pre-v1.19.0-20260917T113832Z`（kpanel.tar.zst、old-image.tar.zst、agent 服务、kpanel.conf、image-load-verify 全部 OK，SHA256SUMS `c57f5cfa2b79d3a33c72c32cd244914f0db4f4af8e362cc38edcb5fbd02936e3`）。
- 部署命令/入口：`env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`（KPanel 标准应用市场入口，非直接改数据）；执行输出 `KPanel 更新完成`、`KPANEL_PROGRESS 100 应用更新完成`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy `v1.19.0-production-r2` pass（plan SHA `d34a2715...`，11:44:11Z-11:44:12Z），核对 version=1.19.0、revision=`031e7b26`、镜像版本标签 1.19.0、RepoDigests 含 `@sha256:351a236f`（tag 权威 index）、Agent systemd active/enabled、容器无 restart/OOM、近 10 分钟无 panic/fatal 日志、受保护配置哈希与基线 diff=0、SQLite quick_check ok、`df`/`findmnt`/数据清单快照落盘；公网入口 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 `status=ok`、`version=1.19.0`（11:44:37Z 复核）。
- 生产已执行写操作：1 次（标准应用市场更新事务，含前置备份与后置核对）。
- 仅在隔离真机执行、未在生产执行的场景：公开镜像 E2E 与 L3（隔离容器/Runner）。

## 回滚

- 源码/tag：稳定回滚点 `v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`（历史 tag 与镜像不可变，保留可回退）。
- 镜像 digest：`v1.18.0` index `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.19.0-20260917T113832Z`（完整镜像+数据卷+服务配置，SHA256SUMS 可校验恢复）。
- 回滚步骤和回滚后复核：应用市场入口执行指向 v1.18.0 的更新事务（备份目录含 old-image.tar.zst 与恢复脚本），恢复后按 postdeploy 同口径核对。
- 回滚后生产实际版本与健康状态：本次未回滚，无需执行；当前生产 v1.19.0 健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均为 v1.19.0 / `sha256:351a236f...`（公共默认更新通道本次已随稳定版发布切换）。
- 公共默认更新通道决策：v1.19.0 已通过全部稳定版门禁与生产验证，作为公共默认版本；如后续发现逃逸缺陷按"已知问题公告+修复期限或恢复上一稳定默认版本"处置。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-15T00:52:21+08:00
- 候选冻结时间：2026-09-17T19:02:00+08:00
- 生产完成时间：2026-09-17T19:44:12+08:00
- 提交到生产用时：66.86 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

补充说明：提交到生产用时包含 rc.1-rc.5 五个预览版的渐进验证与预览反馈期（2026-09-15 至 2026-09-17）。

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前或为门禁正常拦截；生产写操作（更新事务）一次成功。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "production-postdeploy/parameter-validation/missing-revision",
    "position": "before-production-write",
    "count": 1,
    "impact": "postdeploy 首次调用缺少 --expected-revision/--expected-image-digest 被 L3 外层入口 fail-closed 拒绝, 未产生远端副作用。",
    "recoveryEvidence": "补齐参数后以同一 run-id 重新执行; 入口要求新 run ID, 以 v1.19.0-production-r2 完成全部核对。",
    "permanentAction": "postdeploy 调用参数清单固定为 version+revision+image-digest 三件套, 缺一即拒; 已写入发布执行记忆。",
    "historicalReleases": []
  },
  {
    "fingerprint": "oci-digest-semantics/legacy-api/child-manifest-vs-index",
    "position": "before-production-write",
    "count": 1,
    "impact": "postdeploy 第二次以 amd64 子 manifest 摘要(023859e9)作为 --expected-image-digest 失败: 旧 docker manifest inspect 返回的是单架构子 manifest 摘要, 而 tag 权威指向与生产更新器 pin 的是多架构 index 摘要(351a236f)。生产容器本身 revision/版本/健康全部正确, 纯参数口径错误。",
    "recoveryEvidence": "用 buildx imagetools 取得 tag 权威 index 摘要 351a236f, 确认其 amd64 子项正是 E2E 拉取的 023859e9、与生产 RepoDigests 匹配, 以 r2 run 完成全部七项核对通过。",
    "permanentAction": "OCI 通道/版本 digest 统一以 buildx imagetools 的 index 摘要为权威口径; docker manifest inspect 仅用于单架构子 manifest 视图。后续应将该口径差异沉淀为仓库脚本, 避免下个列车再次混淆。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：验收预览、临时 E2E 容器和网络已停止或清理；候选工作树与 L3/生产证据保留；rc 系列临时验收目录与历史已吸收分支仍为用户保留的可选清理项。
- 未验证风险：4 小时全时长生产 soak、真实断电、弱网、长期大规模节点、200% 缩放、人工三语矩阵、生产数据恢复演练（备份已落盘）。
- 已实现待观察：生产 v1.19.0 的实际负载表现与轻量节点批量接入的真实使用反馈。
- 不阻断本版的理由：完整 L3、候选/main/tag 三层 CI、双架构公开 OCI、公开镜像 E2E、生产三阶段证据链全部一次通过；生产公开入口健康返回 1.19.0。
- 后续应进入的自动门禁或专项工作流：`1.19.0` 列车期间三次归类的"公开 OCI 元数据提取与 E2E 源码入口收敛为仓库固定脚本"与本次新增的"OCI index digest 口径统一"应合并为下一列车的发布基建专项；下一预览列车 `v1.20.0-rc.1` 沿用双通道规则从 main 新建候选分支。
