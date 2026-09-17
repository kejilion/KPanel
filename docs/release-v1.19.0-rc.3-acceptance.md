# KPanel v1.19.0-rc.3 发布验收记录

日期：2026-09-17

发布级别：L3

候选提交 / 标签：`60b979ea1d4ae82d0f113670869549910729cf83` / `v1.19.0-rc.3`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`

`releaseChannel`：`preview`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 预览版保留

## 发布画像

- 业务域：终端（快捷命令、批量执行、主机列表）、集群（主机排序真源、公开分享页、能力披露）、桌面（分屏调整）。
- 变更面：展示、只读状态评估、面板持久化数据结构扩展；不新增宿主机写入、协议、数据迁移或生产部署。
- 受影响用户旅程：维护并执行终端快捷命令（窄屏可用）、多主机批量执行、拖动桌面分屏分隔条、在集群/文件/监控/终端间共享主机顺序、查看公开分享页流量明细。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：中等；快捷命令与主机排序扩展了 Panel 持久化数据结构并迁移旧浏览器本地排序，但没有宿主机写入或生产变更。

## 发布范围与未纳入内容

- 用户可见更新：Panel 持久化并备份的终端快捷命令（含排序）；多主机批量执行模式；桌面分屏比例调整与布局持久化；集群主机顺序改为 Panel 真源并在集群/文件/监控/终端共享、跨浏览器保留；终端主机列表语义明确化；集群页披露已授予管理能力；公开分享页流量明细压缩与轮询不干扰布局；快捷命令窄屏覆盖层。
- 精确提交清单：`797928bc`、`e5a952ae`、`ef2f2de3`、`367aec75`、`00f2ab80`、`6e0caa54`、`be51497a`、`0b29c590`、`11e1263a`、`e4234800`、`933a4b5e`、`ad7d8a85`、`b8143517`、`47eb648f`、`19226093`、`06ae49be`、`e9e8d1c9`、`60b979ea`（完整 40 位 SHA 以 `git rev-list v1.19.0-rc.2..v1.19.0-rc.3` 为准）。
- 明确未纳入的分支、文件或后续事项：稳定发布、应用市场配置变更、受管脚本变更和生产部署不在本轮范围。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写"不适用"）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`（与公开镜像 `/release/kejilion.sh` 实测一致）。
- 脚本候选 commit / SHA-256（不适用时写"不适用"）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。
- 状态判定依据与兼容性证据：本轮没有修改 `cmd/kejilion-node/update_runtime/source.json` 或应用安装契约；L3 的受管脚本契约、目标烟测和应用生命周期全部通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写"不适用"）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 r5 全量 Go/前端测试通过；`app_conf_lifecycle=pass`；公开镜像 E2E `image_e2e=pass`。 | 未对真实大规模主机并发执行批量命令压测。 |
| 网络入侵与供应链安全 | 已验证 | L3 固定 Runner 源码/配置/最终镜像扫描为 0；OCI revision `60b979ea`、内嵌脚本 SHA、`SHA256SUMS` 均核对。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 应用安装/更新/中断回滚/卸载故障注入通过；公开镜像冷启动 E2E 含端口/容器/网络残留断言。 | 未执行真实断电与长期 soak。 |
| 性能与资源预算 | 不适用 | 本次为预览发布且未部署生产；构建、测试和冷启动没有超时或 OOM。 | 未采集真实规模节点的长期 P95 与资源曲线。 |
| 用户体验与可访问性 | 已验证 | 验收级浏览器预览（visual-composition）覆盖 7 条受影响旅程、1280×720 与 390×844、浅/深色、键盘焦点和窄屏快捷命令覆盖层；控制台无错误。 | 未单独完成人工三语矩阵与 200% 缩放。 |
| 数据、配置与迁移 | 已验证 | 旧浏览器主机排序在 Panel 未保存排序时安全迁移；无数据库 schema、端口、Compose、节点身份变化。 | 未对预览版执行生产数据恢复演练。 |

## 自动门禁

- 定向测试及结果：最终候选随 L3 r5 完整验证；候选择优重组后以 `release/v1.19.0-candidate` 为唯一序列提交。
- `make verify-release` 环境和结果：固定 Linux Runner `kpanel-release-gate:go1.26.7-node24`（`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`）；`release_gate_runner=pass commit=60b979ea`；`app_conf_lifecycle=pass`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-rc.3-60b979ea-l3-r5`；plan `3fa4e7ecb918829c8010a1540aceae308c9101542f2fe29d55dc7e3789ba5935`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `2ea761b8a9dfa3765a0ec801d55e23641a884a5917b64ab3a4c573dec3b14e0d`，远端日志 `b025a452368f5f6935e2426d814861321f1f99cd1851bac3a11137efba4fa66a`；2026-09-15T22:15:53Z 至 22:29:57Z，status `passed`、exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.3-60b979ea-l3-r5` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.3-60b979ea-l3-r5`。
- 候选 CI：[CI 35031314732](https://github.com/kejilion/KPanel/actions/runs/35031314732) 与 [freshness 35031314771](https://github.com/kejilion/KPanel/actions/runs/35031314771) 成功，均绑定最终 SHA。
- 主线 CI：[CI 35032021635](https://github.com/kejilion/KPanel/actions/runs/35032021635) 与 [freshness 35032021656](https://github.com/kejilion/KPanel/actions/runs/35032021656) 成功，均绑定最终 SHA；[tag freshness 35177786916](https://github.com/kejilion/KPanel/actions/runs/35177786916) 成功。
- Release workflow：[Release 35177786887](https://github.com/kejilion/KPanel/actions/runs/35177786887) 成功（2026-09-17T03:20:16Z 开始，约 11 分钟完成）。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-15/17 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、固定摘要基础镜像和 Actions；`web/package.json` 仅含版本字段同步。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0-rc.3`；公共 OCI index 为 `sha256:d9ec1a24cfc39f6b8dcdba001d8e5616e3aeeda6479f27918bd5330efb500400`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.18.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`60b979ea1d4ae82d0f113670869549910729cf83` 与 `docker.io/kjlion/kejilion-panel@sha256:ceb14a4acaf339fdb6e6a84863f321c3650c5b7ebcb826edaad8ae312ed4a67f`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-rc.3-60b979ea-l3-r5` passed/0；验收级浏览器预览 `v1-19-0-rc-3-r5-1789510503609-8cbf11`（state stopped，7 条旅程，manifest SHA-256 `46539f5fcc44874e9a47d52a0646c5689473276e079bcdbfdbc8887d16bf4287`）位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.3-60b979ea-local-preview-r5`；公开 OCI E2E r1 exit 0，run.sh `22a085f6e5cbe2f225fcb89280ffc92d16bffbdf90c8f69db1b335d315888f0e`，固定 image-e2e `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 `26983d2abd7f974b3b0a04c437b239e31035fa92bfb1fab6cf56412230449562`；证据位于 `C:/GitHub/_release-evidence`（本机无 docker，E2E 按策略在 arena 执行）与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.3/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次验收级浏览器旅程和公开镜像冷启动；无 soak，本版风险由全量测试、race、失败注入和公开冷启动覆盖。
- 受影响用户旅程、视口、缩放、字号、主题、键盘/焦点、语言和失败态：模拟数据预览覆盖 1280×720 与 390×844、100%、浅/深色、中文、键盘焦点、快捷命令窄屏覆盖层和公开分享页窄视口；最小计算字号未发现低于既有契约。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载和生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：生产部署、生产故障注入、原生 arm64、弱网、真实断电、长期 soak、200% 缩放和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0-rc.3](https://github.com/kejilion/KPanel/releases/tag/v1.19.0-rc.3) 于 2026-09-17T03:30:44Z 公开，为非 draft、prerelease、非 Latest；annotated tag 对象 `b7ff80c81e97bc976dafc66015c0666efbb85ce4` peeled 到最终产品 SHA `60b979ea`。GitHub Latest 仍为 v1.18.0。
- Docker 版本与通道 OCI index：`1.19.0-rc.3` 与 `preview` 同为 `sha256:ceb14a4acaf339fdb6e6a84863f321c3650c5b7ebcb826edaad8ae312ed4a67f`（tag 端 index `sha256:d9ec1a24...`）；稳定 `latest` 仍为 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`，未被本次发布改变。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:ceb14a4acaf339fdb6e6a84863f321c3650c5b7ebcb826edaad8ae312ed4a67f`；arm64 `sha256:af5a8d1661bf1c0e4768c4f259926935e2abffc5d6035f25b7e59ef151e638bf`；各架构 manifest 均 带 attestation，`unknown/unknown` 条目为 provenance/SBOM，不判为架构缺失。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded（agent 双架构、node 双架构、deploy tar.gz、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES）。
- 公开镜像 `image_e2e=pass`：`arena-154` 从 Docker Hub 按不可变摘要以 `docker create`/`docker cp` 只读提取元数据（revision=`60b979ea`、VERSION=`1.19.0-rc.3`、脚本 SHA 一致、非 root `65532:65532`）后运行固定 `image-e2e.sh`；输出 `image_e2e=pass` 与 `public_oci_e2e=pass`，端口/容器残留/网络残留断言通过。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca`（`feat: support preview release targets`）的 `kpanel.conf` 归一化内容一致，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` main 保持 `4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：沿用 rc.1/rc.2 已验证实现，本轮 L3 与 r5 预览未改变通道契约。
- 本次发布只提升 Docker `preview` 通道标签，未触碰 `latest`、GitHub Latest 或应用市场默认入口：已验证（Docker digest 复核）。
- 预览版保留候选分支：`release/v1.19.0-candidate` 保留并指向 `60b979ea`。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器；不适用（预览版禁止生产部署）。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未部署、未升级、未核对。
- 生产已执行写操作：本次为 0。

## 回滚

- 源码/tag：稳定回滚点 `v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`。
- 镜像 digest：稳定 `latest` 保持 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择 v1.18.0 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.18.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.18.0 / `sha256:cad0f7f0...`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.18.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-16T05:00:57+08:00
- 候选冻结时间：2026-09-16T06:14:51+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-monitoring/poll-loop/foreground-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release workflow 监控循环在前台 10 分钟预算内未等到完成，命令被超时终止；GitHub 作业本身正常，约 11 分钟后 success。",
    "recoveryEvidence": "切换为后台有界轮询任务，完成时收到终态 completed/success，随后继续步骤 6 核对。",
    "permanentAction": "发布链路等待统一使用后台任务执行，前台只做单次状态读取，不把多阶段构建放进前台超时预算。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-tools/docker/unavailable-on-windows",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 本机无 docker，首次 Docker Hub 摘要核对尝试在本机执行失败；hub.docker.com 直连同样超时。",
    "recoveryEvidence": "按环境策略改用已登记 arena-154 的 docker/buildx 完成版本、通道和双架构摘要核对。",
    "permanentAction": "OCI 相关核对统一路由到已登记 Docker 主机执行，Windows 只做 HTTP 层只读复核；与既有'发布级证据只出自固定 Runner'约定一致。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-oci-e2e/source-clone/cleaned-l3-clone",
    "position": "before-production-write",
    "count": 1,
    "impact": "E2E 首次尝试引用已被 L3 清理的临时克隆路径，入口脚本报 No such file；没有产生容器或网络残留。",
    "recoveryEvidence": "从已校验 r5 bundle（SHA-256 2ea761b8...）按固定提交检出源码后重跑，输出 image_e2e=pass 与 public_oci_e2e=pass。",
    "permanentAction": "公开 OCI E2E 的源码入口固定为'从验证 bundle 检出候选提交'，不引用 L3 临时克隆；该约定已在 rc.2 验收记录中提出收敛为仓库脚本。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：验收预览进程、临时 E2E 容器和网络已停止或清理；保留候选工作树、远端候选分支、L3 r1-r5、公开 OCI r1 及本地预览证据，供同一 `1.19.0` 序列继续追加 RC；未删除其他任务工作树或唯一证据。
- 未验证风险：真实大规模主机批量执行、原生 arm64、弱网、真实断电、长期 soak、200% 缩放、人工三语矩阵和生产数据恢复。
- 已实现待实机准入：快捷命令/主机排序在真实多浏览器长期保留、退出预览不降级需要在 RC 反馈期继续观察。
- 不阻断本版的理由：最终 SHA 已通过验收级浏览器预览、完整 L3、候选/main/tag 门禁、双架构公开 OCI 与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：下一个 RC 沿用 `release/v1.19.0-candidate`（如 `v1.19.0-rc.4`）；进入稳定版前补预览反馈、真实通知与节点专项、批量执行并发专项；公开 OCI 元数据提取与 E2E 源码入口仍待收敛为仓库固定脚本。
