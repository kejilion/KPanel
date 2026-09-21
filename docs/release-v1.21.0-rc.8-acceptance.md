# KPanel v1.21.0-rc.8 发布验收记录

日期：2026-09-21

发布级别：L3

候选提交 / 标签：`8ba06cf85ff4d445db77b11cb28b72fdbb3ab44d` / `v1.21.0-rc.8`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端与本地均精确指向 `8ba06cf85ff4d445db77b11cb28b72fdbb3ab44d`

- 原分支 / 精确 tip / 处置分类：`feature/cluster-temporary-sort-20260921` / `9330031ea179420bacbe3d71603c5718e3a0cd40` 已纳入；`fix/kpanel-empty-install-dir` / `98360967854148fc886ff8e64cb00bfa180b447b` 已纳入。两者均无远端活动引用，本地活动分支已归档。
- 归档 ref 与 SHA：`archive/feature/cluster-temporary-sort-20260921` = `9330031ea179420bacbe3d71603c5718e3a0cd40`；`archive/fix/kpanel-empty-install-dir-20260921` = `98360967854148fc886ff8e64cb00bfa180b447b`。
- 本次来源任务分支：集群临时指标排序对应发布提交 `b000a917`、`1711ee2f`；空目录安装和不完整安装卸载清理对应 `704948e5`、`8d2a14a5`。
- 本地分支/upstream/worktree：两个来源 worktree 保持干净并切换到上述 archive ref；集群 UI 预览已通过标准 stop 入口关闭。为保留浏览器证据和可再生缓存，本轮不强删 worktree；候选与验收 worktree按发布证据需要保留。
- 未完成归档项 / 责任人 / 下次复核触发条件：无；候选分支按预览列车规则保留到稳定版成功。

归档不代表生产上线；本版仅发布预览产物，生产未部署。

## 发布画像

- 业务域：集群节点指标浏览、KPanel 应用安装与卸载恢复。
- 变更面：前端展示排序，以及应用市场宿主机安装/卸载脚本的受控文件和 Docker 网络清理；没有 API、数据库、端口、Compose、Agent 权限或受管脚本协议变化。
- 受影响用户旅程：管理员可按 CPU、内存、磁盘和流量临时排序集群节点；空安装目录可重新安装；由应用市场拥有但 Compose 文件不完整的安装可在安全条件下清理。
- 未变化契约：排序不持久化且不修改后端数据；只删除归属明确且空闲的 KPanel 网络；公共应用市场默认镜像仍为 `latest`。
- 风险等级及理由：L3。排序是局部前端变化，但安装/卸载脚本涉及宿主机文件、服务和 Docker 网络，必须完整验证失败保护与公开镜像。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.8]`：集群节点临时指标排序；不完整安装的安全重装与卸载清理。
- 精确提交清单：`b000a917`、`1711ee2f`、`704948e5`、`8d2a14a5`、版本提交 `1697d4fd`、候选复核 `8ba06cf8`。
- 明确未纳入：生产部署、持久化排序、未知归属目录/服务/网络的强制删除，以及本轮之外的脏工作树和安全审计分支。

## 外部审计与修复交付

- 本 RC 没有新增独立安全审计 run；安全结论来自 govulncheck、npm audit、Trivy 源码/配置/最终镜像扫描和人工候选复核。
- OCR 观察区间 `6ca4ae7..1697d4f`，12/12 个受支持文件覆盖；`OCR-Review: 1.12.6`，有效 H0/M0/L0，constrained-only=0。`CHANGELOG.md`、`kpanel.conf` 和 lockfile 由自由审查补充，未发现问题。
- 独立复核记录在 `8ba06cf8`；其他 provider CLI 不可用，按项目回退规则使用干净 Codex 复核并明确记录 fallback，没有把不可用外部能力改写为通过。
- 本稳定周期继续积累观察数据，不依据单个 RC 的零发现提前判断工具长期有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：本版没有修改 `kejilion.sh` 内容或调用协议；L3 managed-script-contract 与应用生命周期通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 完整 Go 与 Web 176 文件 / 1541 项通过；应用配置生命周期通过。 | 没有新增 API 或双端协议。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/最终镜像 0 阻断项。 | 3 个依赖模块通告不可达，继续跟踪。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、空目录重装、不完整安装卸载、闲置网络归属检查、候选/main/tag 门禁和公开 OCI E2E 通过。 | 未做长期 soak。 |
| 性能与资源预算 | 已验证 | 排序仅对当前节点数组本地计算；没有后端轮询、存储或资源协议变化。 | 大规模节点长期交互未压测。 |
| 用户体验与可访问性 | 已验证 | 精确来源提交 `9330031e` 的本地浏览器验收通过，排序状态和方向可见。 | 未另做最终 SHA 的 125%/200% 与多浏览器矩阵。 |
| 数据、配置与迁移 | 已验证 | 无 schema/持久数据迁移；删除路径需 ownership marker，网络需 Compose 归属且容器数为 0。 | 未对未知第三方 Docker 网络执行破坏性注入。 |

## 自动门禁

- 定向测试及结果：排序组件定向测试与来源提交浏览器验收通过；`packaging/tests/app-conf-lifecycle.sh` 在固定 Runner 中通过新增空目录与部分卸载场景。
- `make verify-release` 环境和结果：固定 Runner 中完整 Go、Web 176 文件 / 1541 项、typecheck、3304 个本地化短语、生产构建、核心 race、双架构构建和镜像契约通过。
- L3 外层入口：run ID `local-wsl-dr-v1.21.0-rc.8-8ba06cf-l3-r1`，2026-09-21 13:24:26 至 13:30:40 +08:00，exit 0；bundle `d87ad405...`、plan `446ea554...`、remote entry `21c08b11...`；不可变 Runner ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。证据位于 `C:/GitHub/_release-evidence/local-wsl-dr-v1.21.0-rc.8-8ba06cf-l3-r1`。
- 候选 CI：[CI 35564819650](https://github.com/kejilion/KPanel/actions/runs/35564819650) 与 [freshness 35564819698](https://github.com/kejilion/KPanel/actions/runs/35564819698) 成功。
- 主线 CI：[CI 35565362341](https://github.com/kejilion/KPanel/actions/runs/35565362341) 与 [freshness 35565362343](https://github.com/kejilion/KPanel/actions/runs/35565362343) 成功。
- annotated tag object `e910fba0a02b7b414ef359142075983e94221b5c` 指向候选提交；[Release 35565861243](https://github.com/kejilion/KPanel/actions/runs/35565861243) 与 [tag freshness 35565861177](https://github.com/kejilion/KPanel/actions/runs/35565861177) 成功。
- 安全扫描、镜像契约、SBOM/provenance：源码/配置/原生镜像扫描通过，运行镜像 revision/version/non-root 契约通过；多架构镜像含两个 provenance/attestation manifest。

## 依赖与技术栈变化

- dependency policy validate-only、候选/main/tag freshness 均通过；本版没有产品依赖、Action、基础镜像或受管脚本升级。
- 沿用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0 与固定发布 Runner。
- 本版没有直接依赖行动项；版本文件和 lockfile 只同步到 `1.21.0-rc.8`。

## 隔离真机与浏览器验收

- 主机：本机 `Ubuntu` WSL2、Linux/amd64、Docker；环境策略 `local-wsl-dr` 只允许 `candidate-validation`。
- 公开 OCI 使用不可变摘要 `sha256:47c2dbb65e05e23b503ee7cb6bd8a39ed9b61f61222f72445cc688ad9dc57d2e`，镜像 revision=`8ba06cf8...`、version=`1.21.0-rc.8`、User=`65532:65532`。
- 在临时端口 `18188` 执行固定 `packaging/tests/image-e2e.sh`；`image_e2e=pass`、`public_oci_e2e=pass`，临时容器、网络和 WSL 临时目录已清理。
- 证据：源码 archive `ddea1931...`、执行脚本 `6d7d0231...`、日志 `7a3beffe...`，位于 `C:/GitHub/_release-evidence/v1.21.0-rc.8-public-image-e2e-local-wsl-r1`。
- 排序 UI 在精确来源提交 `9330031e` 的本地 Chromium 默认视口和 100% 缩放验收通过；最终 RC 未另做 125%/200%、暗色、多浏览器或长期 soak，因此列为遗留风险而非伪造通过。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.8](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.8) 为非 draft、prerelease、非 Latest；发布时间 2026-09-21 13:58:08 +08:00，GitHub Latest 仍为 `v1.20.0`。
- 14 个附件均为 uploaded 且非空；`SHA256SUMS` API/本地摘要均为 `sha256:bbbc6b4feabb7c99d16a765c80752b3290eabce0962e7922550be792a709693f`，包含 11 个二进制/部署包条目。
- Docker `1.21.0-rc.8` 与 `preview` OCI index 均为 `sha256:47c2dbb65e05e23b503ee7cb6bd8a39ed9b61f61222f72445cc688ad9dc57d2e`。
- `linux/amd64` / `linux/arm64` 分别为 `sha256:623fc155c0122335effe6e48dcb35488dfe9dd4e3f06fb2858cb3b31d89e8f3b` / `sha256:9f1e2323c2a2b016c5a686d41812e87bf7027b766fcc83f8be89d83a5b931eea`；另外两项为 attestations。
- 稳定 `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- `kejilion/apps` 的安装/卸载契约已由 `69f1a071ede6ad8aece5910616c8a66a78744032` 提前同步；忽略仓库专属 `app_url` 后与发布配置一致，LF 归一化 Bash syntax 通过，工作树干净，无需空提交。应用市场默认镜像仍为 `latest`。

## 自更新通道验收

- 稳定来源继续只接受正式 GitHub Latest；预览来源接受同列车 RC，并可在没有更新 RC 时接受更高稳定版本。
- 加入预览只切换来源并立即检查，不自动安装；自动安装和一次性立即安装相互独立。
- 旧状态默认迁移、退出预览降级保护、systemd/OpenRC 边界沿用 rc.7 已验证契约；本版未修改自更新代码。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：仅 `local-wsl-dr` 候选验证与公开 OCI E2E。
- 正式部署环境：不适用；`arena-154` 本轮未执行生产写入。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：0。
- 仅在隔离真机执行、未在生产执行的场景：完整 L3 与公开镜像 E2E。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c...`；上一预览 `v1.21.0-rc.7` / `4ab8fdf6...`。
- 镜像 digest：稳定 `sha256:a991b5d2...`；上一预览 `sha256:feaa30df...`。
- 数据/配置备份：不适用，未部署生产。
- 回滚步骤和回滚后复核：预览回退时重新固定 rc.7 digest 并重跑公开镜像 E2E；不改动稳定入口。
- 回滚后生产实际版本与健康状态：不适用。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 `v1.20.0`。
- 公共默认更新通道决策：不适用；本次只更新显式预览通道。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-21T10:22:31+08:00
- 候选冻结时间：2026-09-21T13:30:40+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：集群 UI 预览的 mock API 与 web 进程已停止；两个来源分支已归档。L3、浏览器和公开 OCI 证据以及来源 worktree 暂留用于复核，未强删可再生缓存，因此本轮净释放字节未记录。
- 未验证风险：最终 SHA 的 125%/200% 缩放、多浏览器矩阵、长期排序交互 soak 和大规模节点性能。
- 已实现待实机准入：以同一发布提交继续组装 `v1.21.0` 稳定候选时，重新执行稳定 SHA 的 L3、候选/main/tag 门禁、公开 OCI 与 `arena-154` 生产流程。
- 不阻断本版的理由：本版只进入预览渠道；固定 Runner L3、候选/main/tag 门禁、14 个附件、公开双架构 OCI 与本地 WSL 公开镜像 E2E 均通过，稳定入口和生产未改变。
- 后续应进入的自动门禁或专项工作流：稳定版前补最终 SHA 浏览器缩放矩阵，并继续验证部分安装卸载失败保护。
