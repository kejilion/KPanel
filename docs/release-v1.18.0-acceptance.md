# KPanel v1.18.0 发布验收记录

日期：2026-09-14

发布级别：L3

候选提交 / 标签：`f71cd9a8d6c980fa71eca8045493ef72076c263d` / `v1.18.0`

上一稳定版本 / 回滚点：`v1.17.0` / `22c3e17c209779e3d5fe36b30ac70bd8598deec6` / `sha256:0fff5dbd9850e085e03cf2d0b74d8f7634ec3776810ef9e246b027922f35ae8a`

`releaseChannel`：`stable`

`releaseTrain`：`1.18.0`

候选分支与发布后处置：`release/v1.18.0-candidate` / 稳定版 Release 成功后已自动删除，GitHub API 返回 404

## 发布画像

- 业务域：KPanel 稳定/预览自更新通道、应用市场更新目标、设置页信息顺序。
- 变更面：展示、只读检查、root 级更新事务、应用市场契约和发布部署。
- 受影响用户旅程：管理员选择 stable/preview、手动安装已检查版本、设置自动安装；管理员按“外观与配色 → 备份与恢复 → 自动更新”阅读设置页；systemd 主机按固定 digest 更新。
- 未变化契约：无数据库 schema、端口、Compose 或节点身份迁移；完整 KPanel 节点继续使用既有 Noise/HTTP POST 文件链路，轻量节点继续使用 `1.15.0` 的 `light-control` / `light-data` WebSocket 实现；OpenRC 不启用自动更新，轻量节点只消费稳定版。
- 风险等级及理由：高风险。涉及 root 级更新、跨仓应用配置和公共通道提升；最终 SHA 的 L3、候选/main/tag 门禁、公开 OCI E2E、停写备份和生产 postdeploy 均通过。

## 发布范围与未纳入内容

- 用户可见更新：增加 stable/preview 通道、精确版本/digest 手动安装、预览加入与自动安装分离、退出预览不降级；设置页把“备份与恢复”移到“外观与配色”之后、“自动更新”之前。
- 精确提交清单：`6e0df0b2` stable/preview 通道；`c2388112` 设置页顺序；`38b5a423` 1.18.0 发布准备；`ea30db96` 当前产品上下文；`f71cd9a8` 生命周期测试桩固定版本。
- 明确未纳入的分支、文件或后续事项：未增加 OpenRC 自动更新 timer；未改变完整节点和轻量节点文件传输协议；未发布 RC。后续候选默认优先走 preview，只有明确要求稳定版时才提升 `latest` 和部署生产。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：`kpanel-v1.18.0-release-channels`。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：不适用；本版未改 `kejilion/sh`。
- 状态判定依据与兼容性证据：Docker label、发布包和 L3 受管脚本契约均固定上述 revision/SHA；`kejilion/apps@b9be0ca3b56c5f81a463dc38aba891d06a52ab95` 的 `kpanel.conf` SHA-256 为 `81e61b2f9b2a47234505068e8856bc34c345ae9ea603192adffc16a21137c7ae`，与 KPanel 标签内文件一致。
- 本版发布决定：脚本不在范围；正式 Release 和镜像验证后同步应用市场配置。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | stable/preview 选择、手动安装、退出预览不降级、应用生命周期和设置顺序测试通过；生产稳定通道更新成功。 | 未发布真实 RC 供长期使用。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck`、npm audit、Trivy 源码/配置/最终镜像均为 0；OCI revision/digest、脚本和应用配置精确核对。 | 未执行生产攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、更新互斥/备份/失败回滚/中断恢复、systemd/OpenRC 生命周期、公开镜像 E2E 和生产停写备份通过。 | 未执行真实断电和长期 soak。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 单点采样 CPU 0.03%、74.23 MiB/256 MiB、7 PIDs，Panel/Agent 重启 0、OOM false。 | 单点采样不代表长期 P95。 |
| 用户体验与可访问性 | 已验证 | 设置页 22 项测试通过；引用候选完成 1280/390、浅/深色浏览器检查，无溢出和控制台错误。 | 未覆盖 200% 缩放和完整键盘/焦点矩阵。 |
| 数据、配置与迁移 | 已验证 | 保护配置前后 diff 为 0；数据文件前后均 64 个；`panel/ai.db` quick check 为 ok。 | 未实际执行数据恢复演练。 |

## 自动门禁

- 定向测试及结果：设置页 22/22、发布通道契约 3/3、治理一致性、应用配置 Bash 语法和生命周期均通过。
- `make verify-release` 环境和结果：固定 root Linux L3 完整 Go、155 个前端文件/1365 项测试、typecheck、2349 条 i18n、生产构建、双架构二进制、安装安全和应用生命周期全部通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.18.0-f71cd9a-l3-r3`；plan `f6e08a162e323ea6f6857633a4995b55dd67da19634e28841041a3ca9d740b1b`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `8e9141bb1be71c2873382fa194ff7d278caf09484d781a570f483d8ff716a76d`，Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；2026-09-14T10:51:07Z 至 11:04:07Z，exit 0；证据 `C:/GitHub/_release-artifacts/v1.18.0-f71cd9a-l3-r3` 与 `arena-154:/root/kpanel-release-evidence/v1.18.0-f71cd9a-l3-r3`。
- 候选 CI：[CI 34836333515](https://github.com/kejilion/KPanel/actions/runs/34836333515) 与 [freshness 34836333500](https://github.com/kejilion/KPanel/actions/runs/34836333500) 成功，均绑定最终 SHA。
- 主线 CI：[CI 34836685078](https://github.com/kejilion/KPanel/actions/runs/34836685078) 与 [freshness 34836685084](https://github.com/kejilion/KPanel/actions/runs/34836685084) 成功，均绑定最终 SHA。
- Release workflow：[tag freshness 34837448145](https://github.com/kejilion/KPanel/actions/runs/34837448145) 与 [Release 34837448201](https://github.com/kejilion/KPanel/actions/runs/34837448201) 成功。
- 安全扫描、镜像契约、SBOM/provenance：Go/npm/源码/配置/最终镜像扫描为 0；runtime contract 通过；双架构各带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-14 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理与 freshness 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版无新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0、摘要固定的基础镜像/Actions 和 `kejilion/sh@6ebb945f`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 1.18.0；锁文件校验通过；公共 OCI index 为 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：门禁与生产证据通过，未触发回滚，保留 v1.17.0 恢复点。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Linux amd64，Docker；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / candidate-validation、production-safety-check、production-deploy；`prod-108` 禁用。
- 使用的精确候选或公开产物：`f71cd9a8d6c980fa71eca8045493ef72076c263d` 与 `docker.io/kjlion/kejilion-panel:1.18.0@sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.18.0-f71cd9a-l3-r3` passed/0；公开镜像使用固定 `packaging/tests/image-e2e.sh`，输出 `image_e2e=pass`；未另起浏览器后台作业。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：无 soak；本版以更新事务、失败注入、重启恢复、公开拉取和生产 postdeploy 覆盖主要风险。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：设置顺序候选覆盖 1280/390、浅/深色；自动测试覆盖中英文案和失败态；200% 缩放、完整键盘/焦点未验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 覆盖 systemd/OpenRC 安装、更新、失败回滚、中断恢复和卸载；生产更新一次成功，未触发回滚。
- 未执行场景及原因：真实 RC 长期运行、原生 arm64 运行时、弱网、真实断电、长期 soak 和生产故障注入未执行，避免为形式验证扰动正式数据。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.18.0](https://github.com/kejilion/KPanel/releases/tag/v1.18.0) 于 2026-09-14T11:27:26Z 公开，为非 draft、非 prerelease、GitHub Latest；annotated tag 对象 `dc6afdf49dabed1004e91cb7e42595ffd9a5d495` peeled 到最终产品 SHA。
- Docker 版本与通道 OCI index：`1.18.0` 与稳定通道 `latest` 同为 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`；预览通道 `preview` 发布前后均不存在（Docker Hub 404）。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:9026261e44f4689a6c078690a8608158404d2fd57a5b3d932f197e33082260f9`；arm64 `sha256:59c8420c1ca1ac7297ddd338030018ac23585a4037376ce16419ab5beb020e71`。
- 附件及 `SHA256SUMS`：8 个附件齐全，包括双架构 Agent、双架构 Node、部署包、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。
- 公开镜像 `image_e2e=pass`：`arena-154` 从 Docker Hub 新拉取 1.18.0，实际 digest 匹配并输出 `image_e2e=pass`。
- `kejilion/apps` / `kejilion.sh` 契约结论：apps `main` 已快进到 `b9be0ca3`；sh 无提交。生产安装脚本与镜像原始脚本唯一差异为既有许可偏好 `permission_granted="false"` 继承为 `true`。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只选择规范稳定版或 RC，并校验唯一官方镜像 digest：已验证。
- 加入预览只切换来源并立即检查，没有自动安装：已验证。
- 自动安装开关与一次性立即安装相互独立：已验证。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：已验证。
- 退出预览且稳定版较低时没有产生降级候选：已验证。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：已验证。
- OpenRC 与轻量 Node 的当前边界已按 `docs/release-channels.md` 明确呈现：已验证；OpenRC 无自动更新，轻量 Node 仅 stable。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确要求本次走稳定版上线流程，授权正式发布与生产部署。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154` / `154.36.153.9`。
- `prod-108`：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight 2026-09-14T11:30:00Z 至 11:30:01Z 通过，版本 1.17.0、Panel healthy、Agent active。backup 11:30:20Z 至 11:30:29Z 通过；恢复包 `/root/kpanel-backups/pre-v1.18.0-20260914T113020Z`，数据归档 `e5ebf5d0139fec0785fc5181eea4ba261ead4749e46a71d9a0b2bb36cc19f3ef`，旧镜像归档 `eb40ef5d2375cc45bfcc11271117f1660dcb60a6ab4117436bc5c7bb32da2575`。
- 部署命令/入口：`env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，2026-09-14T11:30:45Z 至 11:31:15Z，exit 0。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy 11:31:32Z 至 11:31:34Z 一次通过；公网 health HTTP 200、status ok、initialized true、version 1.18.0；Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NRestarts 0；timer active/waiting/enabled；近 10 分钟无 panic/fatal/OOM；保护配置 diff 为 0，64 个数据文件保持，SQLite 正常。
- 生产已执行写操作：apps 工作树快进、停写备份、标准 KPanel 更新和服务重建；未直接编辑正式数据库。
- 仅在隔离真机执行、未在生产执行的场景：失败注入、更新中断、安装/卸载、OpenRC 生命周期和公开镜像临时容器 E2E。

## 回滚

- 源码/tag：`v1.17.0` / `22c3e17c209779e3d5fe36b30ac70bd8598deec6`。
- 镜像 digest：`sha256:0fff5dbd9850e085e03cf2d0b74d8f7634ec3776810ef9e246b027922f35ae8a`。
- 数据/配置备份：`arena-154:/root/kpanel-backups/pre-v1.18.0-20260914T113020Z`，包含数据、配置、service、inspect、旧镜像和 SHA256SUMS。
- 回滚步骤和回滚后复核：恢复 GitHub Latest/Docker `latest` 与应用市场目标后，使用标准应用入口恢复旧镜像；必要时恢复上述归档，再复核 health、Agent、OCI、SQLite、保护配置和日志。
- 回滚后生产实际版本与健康状态：本轮未回滚；生产实际为健康 1.18.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均指向 1.18.0 / `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 公共默认更新通道决策：不适用；本轮稳定版生产验证通过。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-14T18:10:28+08:00
- 候选冻结时间：2026-09-14T18:48:04+08:00
- 生产完成时间：2026-09-14T19:31:34+08:00
- 提交到生产用时：1.35 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品变更未造成生产退化、回滚、紧急热修或重复发布。以下流程异常都发生在首次生产写操作前，由门禁或验证入口拦截，未进入公共默认通道或生产。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-l3/business-context/stale-review",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 在 bundle verification 因当前产品评审落后 51 个提交而失败；没有上传远端 Runner、推送分支或触碰生产。",
    "recoveryEvidence": "ea30db96 刷新 canonical product review；freshness 通过，最终 L3 r3、候选/main/tag freshness 全部成功。",
    "permanentAction": "保留 check-business-context-freshness.mjs 的 50 提交硬门禁；达到阈值时必须先刷新 canonical review 再冻结候选。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/app-conf-fixture/runtime-env-version",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r2 的产品测试、race、构建与镜像扫描均通过，但 app-conf 生命周期测试桩在更新函数返回后丢失临时版本环境，最终断言错误读取 1.18.0，导致门禁失败；产品事务已正确完成。",
    "recoveryEvidence": "f71cd9a8 在创建 fake Agent 时固化版本；权威定向生命周期、完整 L3 r3、候选 CI、main CI 和 Release workflow 均通过。",
    "permanentAction": "生命周期夹具现在模拟真实二进制，将创建时版本写入 fake Agent；该行为由同一固定 app-conf-lifecycle.sh 在 L3/CI/Release 持续回归。",
    "historicalReleases": []
  },
  {
    "fingerprint": "targeted-validation/upload/missing-inbox-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "测试桩修复后的首次补充上传因远端候选输入目录不存在而在 scp 阶段停止，未运行候选测试或生产命令。",
    "recoveryEvidence": "创建并核对唯一候选输入目录后，固定 Alpine 生命周期命令输出 app_conf_lifecycle=pass；最终准入仍只采用完整 run-release-l3.mjs r3。",
    "permanentAction": "补充定向验证不再替代准入；正式准入只使用会预建并校验 inbox 的 run-release-l3.mjs，手工上传前固定执行目录存在性预检。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-verification/remote-command/powershell-quote-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开镜像 E2E 首次远端提交校验被 PowerShell 引号展开为多参数 test，在 docker pull 和 image-e2e 前停止；未创建容器或改变生产。",
    "recoveryEvidence": "改用 git rev-parse HEAD 管道到 grep -qx 的无变量校验，随后从 Docker Hub 拉取精确 digest 并输出 image_e2e=pass。",
    "permanentAction": "Windows 发起公开 E2E 时使用无远端变量插值的精确 SHA 管道校验，并继续由仓库固定 packaging/tests/image-e2e.sh 执行业务断言和清理。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：尚未在本记录提交前删除证据或恢复包；保留 `C:/GitHub/_release-artifacts/v1.18.0-*`、`arena-154:/root/kpanel-release-evidence/v1.18.0-*` 与唯一生产恢复包。隔离 E2E 临时容器/网络由固定脚本清理；补充脚本比较停止容器和临时文件已清理。候选工作树在验收提交后再按安全边界评估回收。
- 未验证风险：200% 浏览器缩放、完整键盘/焦点矩阵、真实 RC 长期运行、原生 arm64 运行时、真实断电、跨地域弱网、长期 soak、生产故障注入和实际数据恢复。
- 已实现待实机准入：未来 preview 候选需在非生产隔离环境验证 RC 获取、切换不安装、自动安装和退出预览不降级；预览版禁止生产部署。
- 不阻断本版的理由：稳定路径的最终 SHA 已完成 L3、三层 GitHub 门禁、公开 OCI E2E、生产备份与 postdeploy，所有生产健康和数据保护断言通过。
- 后续应进入的自动门禁或专项工作流：下一次上线默认优先走 preview；设置页若继续调整，补 200% 缩放与完整键盘/焦点专项。
