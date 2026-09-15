# KPanel v1.19.0-rc.2 发布验收记录

日期：2026-09-15

发布级别：L3

候选提交 / 标签：`cfc74247a76d0e0cc099ec6fb4d6512a7bac83f4` / `v1.19.0-rc.2`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`

`releaseChannel`：`preview`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 预览版保留

## 发布画像

- 业务域：集群通知、集群监控交互、设置页备份与恢复、项目协作治理。
- 变更面：展示、只读状态评估、通知触发时序、开发与发布门禁；不新增宿主机写入、协议、数据迁移或生产部署。
- 受影响用户旅程：查看和刷新集群、查看指标并使用键盘焦点、接收节点离线与恢复通知、在设置页导出/导入备份并查看历史记录。
- 未变化契约：API、数据库 schema、端口、Compose、Agent 权限、应用市场稳定配置和公开稳定更新入口不变。
- 风险等级及理由：中等；通知防抖改变告警触发时间，设置与集群页存在用户可见布局变化，但没有数据迁移或生产写入。

## 发布范围与未纳入内容

- 用户可见更新：完整 KPanel 与轻量节点连续 3 次异常后才发送失联通知；集群刷新按钮与指标反馈统一；备份中心按导出、导入和记录重整层级并适配移动端。
- 精确提交清单：`de1faf8947cba7ce46d078b49f4e99c4c92392db`、`cd2ce895c122903f0c0eaebf01a9fbbcd11b3738`、`44a4741ea5c91c542e97525f577d034ba8e9605b`、`8db40516e63ecb58e479d86e449f5e1fe8f0aeeb`、`cfc74247a76d0e0cc099ec6fb4d6512a7bac83f4`。
- 明确未纳入的分支、文件或后续事项：没有发现其他基于验收后主线的真实候选；稳定发布、应用市场配置变更、受管脚本变更和生产部署不在本轮范围。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：不适用；轻量节点更新运行时继续固定 `kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`，根脚本 `a0e6cf5193aa9bfbdc572ad91803d4b53d34549ac95259b3288d436766a229dc`。
- 状态判定依据与兼容性证据：本轮没有修改 `cmd/kejilion-node/update_runtime/source.json` 或应用安装契约；L3 的受管脚本契约、目标烟测和应用生命周期全部通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go 全包、156 个前端文件/1382 项测试、告警防抖与集群/备份定向测试全部通过。 | 未对真实第三方机器人执行长期送达率测试。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck`、npm audit、Trivy 源码/配置/最终镜像为 0；OCI revision、脚本和附件摘要已核对。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心包 race、应用安装/更新/中断回滚/卸载故障注入、公开镜像冷启动 E2E 通过。 | 未执行真实断电与长期 soak。 |
| 性能与资源预算 | 不适用 | 本次为预览发布且未部署生产；构建、测试和冷启动没有超时或 OOM。 | 未采集真实规模节点的长期 P95 与资源曲线。 |
| 用户体验与可访问性 | 已验证 | 验收级浏览器预览覆盖宽屏、390×844、浅/深色、指标键盘焦点和移动端全宽操作；控制台无警告或错误。 | 当前内置浏览器未能切换到 200% 缩放，未单独完成人工三语巡检。 |
| 数据、配置与迁移 | 已验证 | 没有数据库 schema、端口、Compose、节点身份或配对密钥变化；备份中心只调整展示。 | 未对预览版执行生产数据恢复演练。 |

## 自动门禁

- 定向测试及结果：3 个前端文件 47 项通过；治理测试 15 项通过；治理一致性与版本一致性通过；通知 Go 包由 L3 完整验证。
- `make verify-release` 环境和结果：固定 Linux Runner 完整 Go 测试、156 个前端文件/1382 项测试、typecheck、2376 条 i18n、production build、双架构二进制、race、安装安全和应用生命周期全部通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-rc.2-cfc74247-l3-r1`；plan `0e605f36eea70e24e214393681f29d432bc189df39b060793719a6f61dc9f154`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `8ebcbba8c4972a62eac707745af11fa2c3879f3673f0ede7427774f53b9b4f65`，Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；2026-09-15T10:13:10Z 至 10:28:31Z，exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.2-cfc74247-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.2-cfc74247-l3-r1`。
- 候选 CI：[CI 34958171536](https://github.com/kejilion/KPanel/actions/runs/34958171536) 与 [freshness 34958171529](https://github.com/kejilion/KPanel/actions/runs/34958171529) 成功，均绑定最终 SHA。
- 主线 CI：[CI 34958850252](https://github.com/kejilion/KPanel/actions/runs/34958850252) 与 [freshness 34958850279](https://github.com/kejilion/KPanel/actions/runs/34958850279) 成功，均绑定最终 SHA。
- Release workflow：[tag freshness 34959452214](https://github.com/kejilion/KPanel/actions/runs/34959452214) 与 [Release 34959452270](https://github.com/kejilion/KPanel/actions/runs/34959452270) 成功。
- 安全扫描、镜像契约、SBOM/provenance：Go/npm/源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；amd64 与 arm64 均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-15 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0、摘要固定的基础镜像和 Actions。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0-rc.2`；锁文件与 Action 固定检查通过；公共 OCI index 为 `sha256:042dfd5b73e49cbb96b211395cd9d00c43385d53ac36a0eb86e5bc893850f1dd`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.18.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`cfc74247a76d0e0cc099ec6fb4d6512a7bac83f4` 与 `docker.io/kjlion/kejilion-panel@sha256:042dfd5b73e49cbb96b211395cd9d00c43385d53ac36a0eb86e5bc893850f1dd`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-rc.2-cfc74247-l3-r1` passed/0；本地浏览器预览 manifest 位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.2-cfc74247-local-preview`；公开 OCI E2E `r2` exit 0，run.sh `c84a2af28ed25ca0104ed2e061be87bb088535613ca4af5a61bbe4ef6e22fb67`，固定 image-e2e `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 `80e60c2c7af05592a1aa15768ebb719913aa55cbf1bb036fd201a1485a7d675f`；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.2-public-oci-e2e-r2` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.2/public-oci-e2e-r2`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：单次验收级浏览器旅程和公开镜像冷启动；无 soak，本版风险由全量测试、race、失败注入和公开冷启动覆盖。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：模拟数据预览覆盖 1280×720 与 390×844、100%、浅/深色、中文、指标键盘焦点、异常/待恢复记录和窄屏全宽操作；最小计算字号未发现低于既有契约；125% 不适用，未涉及像素取整或历史断点缺陷；200% 因浏览器缩放控制未生效而未验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存及自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载和 systemd/OpenRC 生命周期；没有写入生产 KPanel 数据。
- 未执行场景及原因：生产部署、生产故障注入、真实第三方通知长期送达、原生 arm64、弱网、真实断电、长期 soak、200% 缩放和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0-rc.2](https://github.com/kejilion/KPanel/releases/tag/v1.19.0-rc.2) 于 2026-09-15T18:52:11+08:00 公开，为非 draft、prerelease、非 Latest；annotated tag 对象 `33d44ba485bc2f1c390b1ed01f4f8d79293c9aa0` peeled 到最终产品 SHA。GitHub Latest 仍为 v1.18.0。
- Docker 版本与通道 OCI index：`1.19.0-rc.2` 与 `preview` 同为 `sha256:042dfd5b73e49cbb96b211395cd9d00c43385d53ac36a0eb86e5bc893850f1dd`；稳定 `latest` 仍为 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:7656ec356df853d85978c1172efeed249c558181e87778100d4b111546d3f602`；arm64 `sha256:3e468442ad15824196316976d5fb7121e3aad73a9ea7f7fe68659244f9bc823b`；对应 attestation manifest 为 `sha256:cfe54a76abeb45d98e58696dbb335155b35f128e8287f55f11d4a1c9d9b18177` 与 `sha256:5c8f054c5ec6b19fa288e3ca97425ff764edd51cc935ad368f9e3aba1c05ec08`。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded；`SHA256SUMS` 摘要 `d2c1e5771a727954a3b154c9de110449479427723b9b1a503e77817b50e9f982`，列出的 4 个 Agent/Node 二进制和部署包与 GitHub 资产 digest 一致。
- 公开镜像 `image_e2e=pass`：`arena-154` 从 Docker Hub 按不可变摘要重新拉取，health 返回 1.19.0-rc.2，revision、非 root 用户、静态资源、初始化、安全 Cookie、受限网络和清理断言通过；`public_oci_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca3b56c5f81a463dc38aba891d06a52ab95` 的 `kpanel.conf` Git blob 同为 `95ed487653184e6c056ea9d25958539a453cf9cb`，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` main 保持 `4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：已验证。
- 加入预览只切换来源并立即检查，没有自动安装：已验证。
- 自动安装开关与一次性立即安装相互独立：已验证。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：已验证。
- 退出预览且稳定版较低时没有产生降级候选：已验证。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：已验证。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：已验证；OpenRC 无自动更新，轻量 Node 只消费稳定版本更新。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154` 仅以 `candidate-validation` 用途运行隔离容器；不适用（预览版禁止生产部署）。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：不适用（预览版禁止生产部署）。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对；不适用（预览版禁止生产部署）。
- 部署前版本、健康、备份位置及摘要：不适用（预览版禁止生产部署）。
- 部署命令/入口：不适用（预览版禁止生产部署）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用（预览版禁止生产部署）。
- 生产已执行写操作：不适用（预览版禁止生产部署）；本次为 0。
- 仅在隔离真机执行、未在生产执行的场景：公开镜像拉取、冷启动、初始化和自动清理；不适用（预览版禁止生产部署）。

## 回滚

- 源码/tag：稳定回滚点 `v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`。
- 镜像 digest：`sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择上述 v1.18.0 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.18.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.18.0 / `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.18.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-15T08:01:38+08:00
- 候选冻结时间：2026-09-15T18:09:29+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-inventory/worktree-parser/powershell-pipeline-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次候选工作树映射命令因 PowerShell foreach 后的管道语法失败，只影响候选清单采集。",
    "recoveryEvidence": "改用 git worktree list 与各工作树精确 HEAD/status 回读，确认 4 组独立候选均基于 a7ded9c5。",
    "permanentAction": "候选清单采集统一先累积数组再格式化，禁止在 foreach 语句块后直接接管道。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-verification/go-toolchain/unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 定向 Go 测试因本机没有 go 可执行文件未能运行，候选未因此获得通过结论。",
    "recoveryEvidence": "固定 Linux L3 Runner 完成 Go 全包测试、关键包 race 和发布级构建，最终 exit 0。",
    "permanentAction": "发布级 Go 结论继续只取固定 L3 Runner；Windows 端先执行工具存在性预检，缺失时直接记录并跳转权威入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "version-check/local-entry/nonexistent-script-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次版本一致性检查误调用不存在的 mjs 文件，没有产生版本证据。",
    "recoveryEvidence": "改用仓库固定 scripts/check-version-consistency.sh 并通过，L3 与三个远端层级再次验证 1.19.0-rc.2。",
    "permanentAction": "Windows 本地版本检查固定通过 Git Bash 调用仓库现有 shell 入口，并在运行前用 rg --files 解析实际路径。",
    "historicalReleases": []
  },
  {
    "fingerprint": "ci-observation/gh-cli/unauthenticated-session",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机 gh 未登录，首次 CI 状态查询无法执行；GitHub 作业本身正常运行。",
    "recoveryEvidence": "改用 GitHub 公共只读 API 获取候选、main、tag 的精确 run ID、SHA、步骤和 success 终态。",
    "permanentAction": "公开仓库 CI 观察入口优先使用无需登录的 GitHub REST API；只有需要写操作时才要求受管 gh 凭据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "registry-inspection/powershell-json/response-body-type",
    "position": "before-production-write",
    "count": 1,
    "impact": "PowerShell 首次解析 OCI index 响应体未得到 manifest 子项，架构摘要证据无效。",
    "recoveryEvidence": "在 arena-154 使用 docker buildx imagetools inspect 读取同一 index，确认 amd64、arm64 和两份 attestation manifest。",
    "permanentAction": "OCI 架构清单统一由已登记 Docker 主机的 buildx 固定格式入口采集，Windows 只保留通道顶层摘要复核。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-oci-e2e/metadata-read/distroless-runtime",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开 OCI r1 误假设精简镜像内存在 cat/sha256sum，元数据读取在业务 E2E 前以 127 退出；临时运行未留下容器或网络。",
    "recoveryEvidence": "保留 r1 失败证据后，在独立 r2 目录改用 docker create 与 docker cp 只读提取元数据，固定 image-e2e.sh 输出 image_e2e=pass 和 public_oci_e2e=pass。",
    "permanentAction": "公开 OCI 元数据入口固定使用 docker inspect/create/cp，不在目标镜像内执行诊断工具；后续把该入口提交为仓库脚本并补无 shell 镜像回归。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postrelease-ref/verification/command-time-budget",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收提交推送成功后，同一命令内连续回读 main、候选和 tag 超过 30 秒执行预算，没有返回完整复核结论。",
    "recoveryEvidence": "独立只读命令随后确认 main=a29469a5，候选分支和 tag 均仍为产品 SHA cfc74247。",
    "permanentAction": "发布后多 ref 复核拆成独立有界查询，或为只读批量核验使用明确的更长执行预算。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：验收预览进程、浏览器标签、临时 E2E 容器和网络已停止或清理；保留候选工作树、远端候选分支、L3、公开 OCI r1/r2 及本地预览证据，供同一 `1.19.0` 序列继续追加 RC；未删除其他任务工作树或唯一证据。
- 未验证风险：真实第三方通知长期送达、真实规模节点并发、原生 arm64、弱网、真实断电、长期 soak、200% 缩放、人工三语矩阵和生产数据恢复。
- 已实现待实机准入：预览用户实际安装、退出预览不降级和连续异常告警节奏需要在 RC 反馈期继续观察。
- 不阻断本版的理由：最终 SHA 已通过验收级浏览器预览、完整 L3、候选/main/tag 门禁、双架构公开 OCI 与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：下一个 RC 沿用 `release/v1.19.0-candidate`；进入稳定版前补预览反馈、真实通知与节点专项，并把公开 OCI 元数据提取收敛为仓库固定入口。
