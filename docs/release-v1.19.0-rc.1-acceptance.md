# KPanel v1.19.0-rc.1 发布验收记录

日期：2026-09-15

发布级别：L3

候选提交 / 标签：`02b3fb37a02ff5640dacdbec234745c3ebf3ef8b` / `v1.19.0-rc.1`

上一稳定版本 / 回滚点：`v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d` / `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`

`releaseChannel`：`preview`

`releaseTrain`：`1.19.0`

候选分支与发布后处置：`release/v1.19.0-candidate` / 预览版保留，远端仍精确指向发布提交

## 发布画像

- 业务域：KPanel 手动更新、集群主机展示、集群通知、轻量节点批量接入。
- 变更面：展示、只读检查、通知出站、节点接入协议、配套脚本和预览发布。
- 受影响用户旅程：管理员从设置页、应用页或更新提示进入同一更新确认流程；查看轻量节点公网地址；配置 Telegram、飞书、钉钉或企业微信通知；为最多 100 台轻量节点生成共用批量授权并幂等接入。
- 未变化契约：无数据库 schema、端口、Compose、完整节点 Noise/HTTP POST 文件链路或既有轻量节点 WebSocket 链路迁移；应用市场默认入口继续使用稳定版 `latest`。
- 风险等级及理由：高风险。涉及自更新确认、第三方 Webhook、跨仓库脚本和节点接入；最终 SHA 已通过 L3、候选/main/tag 三层门禁、公开双架构 OCI 冷启动 E2E，且预览版未进入生产。

## 发布范围与未纳入内容

- 用户可见更新：统一 KPanel 手动更新入口并展示版本、发布说明、镜像摘要和风险；显示轻量节点公网 IPv4/IPv6；通知新增飞书、钉钉和企业微信；新增轻量节点批量接入授权与进度管理。
- 精确提交清单：`0bbcd620` 统一更新入口；`e3d2ed1f` 更新前发布详情；`b2f72b5f` 轻量节点公网 IP；`9fd8ae87` 多通道通知；`c3c04495` RC 版本准备；`5c7e8312` 批量轻量节点接入；`845816db`、`02b3fb37` 补齐联动发布记录。
- 明确未纳入的分支、文件或后续事项：未改变稳定版 Latest/`latest`，未部署生产，未删除候选分支；真实第三方机器人长期可用性、原生 arm64 运行和长期 soak 留待后续预览反馈。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`。
- 变更集编号（跨仓库时必填；不适用时写“不适用”）：`light-batch-enrollment-20260914`。
- KPanel 实际内置脚本基线 commit / SHA-256：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256（不适用时写“不适用”）：`kejilion/sh@4d61f7ef123fe5ecd7419cdaa1c93483bbfda403`；根脚本 `a0e6cf5193aa9bfbdc572ad91803d4b53d34549ac95259b3288d436766a229dc`，CN 脚本 `132b38bf1300e529ca88e365d075be599ef3466ac1aece48bce6ce7565f344d0`。
- 状态判定依据与兼容性证据：脚本 `main` 与候选分支均精确指向 `4d61f7ef`，短入口和精确 revision 入口已回读批量接入标记；KPanel `cmd/kejilion-node/update_runtime/source.json` 固定该 revision、根脚本摘要和 5 个运行时模板摘要；同步检查、目标烟测、Go 测试、L3 和 Release 全部通过。
- 本版发布决定：配套脚本先行发布，再发布 KPanel 预览版；KPanel 容器内置的通用应用市场脚本仍按既有契约固定 `6ebb945f`。
- 阻断或移除的依赖范围（无则写“不适用”）：不适用；联动脚本已经先行公开并完成哈希核验。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 156 个前端文件、1381 项全量测试，合并后 190 项定向测试，Go 全包测试、节点批量接入、通知通道和更新流程测试通过。 | 未对真实第三方机器人执行长期送达率测试。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck`、npm audit、Trivy 源码/配置/最终镜像均为 0；Webhook 受限出站、OCI revision、脚本与附件摘要已核对。 | 未执行公网攻击注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心包 race、更新失败/中断回滚、systemd/OpenRC 应用生命周期、批量接入幂等与公开镜像冷启动 E2E 通过。 | 未执行真实断电与长期 soak。 |
| 性能与资源预算 | 不适用 | 本次是预览发布且未部署生产；构建、测试和冷启动均未出现超时或 OOM。 | 未采集真实规模 100 节点的长期 P95 和资源曲线。 |
| 用户体验与可访问性 | 已验证 | 更新确认、节点 IP 空状态、批量接入和通知对话框组件测试及中英文案检查通过，2376 条本地化短语通过校验。 | 未另起真实浏览器的 200% 缩放与完整键盘/焦点矩阵。 |
| 数据、配置与迁移 | 已验证 | 通知持久化 schema 保持 v1，批量接入数据进入既有备份集合；无需数据库、端口、Compose、节点身份或配对密钥迁移。 | 未对预览版执行生产数据恢复演练。 |

## 自动门禁

- 定向测试及结果：合并后 9 个前端测试文件共 190 项通过；版本一致性、协作状态、脚本同步和目标烟测通过。
- `make verify-release` 环境和结果：固定 Linux Runner 完整 Go 测试、156 个前端文件/1381 项测试、typecheck、2376 条 i18n、production build、双架构二进制、race、安装安全和应用生命周期全部通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：`v1.19.0-rc.1-02b3fb37-l3-r1`；plan `4b0ec21ff60d0fd04183bf293dd3a28daefab4a6004bceb11a84dade7b3723d3`，remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle `3fec9ebff0e2cd738f83b44178b5a920bb259e4d14755546b90bc90a90154ebe`，Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；2026-09-14T16:59:20Z 至 17:14:02Z，exit 0；证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.1-02b3fb37-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.1-02b3fb37-l3-r1`。
- 候选 CI：[CI 34873553753](https://github.com/kejilion/KPanel/actions/runs/34873553753) 与 [freshness 34873553679](https://github.com/kejilion/KPanel/actions/runs/34873553679) 成功，均绑定最终 SHA。
- 主线 CI：[CI 34873946825](https://github.com/kejilion/KPanel/actions/runs/34873946825) 与 [freshness 34873946811](https://github.com/kejilion/KPanel/actions/runs/34873946811) 成功，均绑定最终 SHA。
- Release workflow：[tag freshness 34874726093](https://github.com/kejilion/KPanel/actions/runs/34874726093) 与 [Release 34874726113](https://github.com/kejilion/KPanel/actions/runs/34874726113) 成功。
- 安全扫描、镜像契约、SBOM/provenance：Go/npm/源码/配置/最终镜像扫描为 0；运行时契约和受限冷启动通过；双架构均带 attestation manifest。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选、main、tag 三层 `Dependency freshness` 于 2026-09-15 全部成功，检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：治理、`govulncheck`、npm audit 与 Trivy 门禁通过，没有阻断项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：本版未新增 Go/npm 依赖或基座升级，不适用。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：继续使用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0、摘要固定的基础镜像/Actions；采用 `kejilion/sh@4d61f7ef` 作为轻量节点更新运行时来源。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：版本文件统一为 `1.19.0-rc.1`；锁文件与 Action 固定检查通过；公共 OCI index 为 `sha256:1c6e9a71a20d250f4563e9b4a0572dd7024145b99d985a7d4f6c2760cf1e7243`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：无依赖候选暂缓项。
- 升级后的兼容、安全、构建、性能资源和回滚结论：预览门禁及公开 OCI 验收通过；未触发产品回滚；生产和公共稳定入口继续保留 v1.18.0。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`，Debian 13、x86_64、Docker 29.6.2；L3 使用固定 `kpanel-release-gate:go1.26.7-node24`。
- 环境策略 ID 与允许用途：`arena-154` / `candidate-validation`；本次未请求 `production-deploy` 或 `production-safety-check`。
- 使用的精确候选或公开产物：`02b3fb37a02ff5640dacdbec234745c3ebf3ef8b` 与 `docker.io/kjlion/kejilion-panel@sha256:1c6e9a71a20d250f4563e9b4a0572dd7024145b99d985a7d4f6c2760cf1e7243`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run `v1.19.0-rc.1-02b3fb37-l3-r1` passed/0；公开 OCI E2E 使用标签内 `packaging/tests/image-e2e.sh`，脚本 SHA-256 `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，证据位于 `C:/GitHub/_release-artifacts/v1.19.0-rc.1-public-oci-e2e-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.19.0-rc.1/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：公开镜像单次冷启动 17 秒；无 soak，本版主要风险由全量测试、race、失败注入、脚本生命周期和公开冷启动覆盖。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：组件测试覆盖更新确认、批量接入、通知通道、IP 空状态及中英文失败态；未另起真实浏览器，因此视口、缩放、主题和完整键盘/焦点专项未验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：仅写入隔离证据目录、Docker 拉取缓存和自动清理的临时容器/网络；L3 覆盖更新失败、中断、安装/卸载和 systemd/OpenRC 生命周期；未写生产 KPanel 数据。
- 未执行场景及原因：生产部署、生产故障注入、真实第三方通知长期送达、原生 arm64、弱网、真实断电、长期 soak 和生产数据恢复未执行；预览版禁止生产部署。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：[KPanel 1.19.0-rc.1](https://github.com/kejilion/KPanel/releases/tag/v1.19.0-rc.1) 于 2026-09-15T01:36:38+08:00 公开，为非 draft、prerelease、非 Latest；annotated tag 对象 `ec111f9d42c728bba264d67530113d17bb9f0baf` peeled 到最终产品 SHA。GitHub Latest 仍为 v1.18.0。
- Docker 版本与通道 OCI index：`1.19.0-rc.1` 与预览通道 `preview` 同为 `sha256:1c6e9a71a20d250f4563e9b4a0572dd7024145b99d985a7d4f6c2760cf1e7243`；稳定通道 `latest` 仍为 `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- `linux/amd64`、`linux/arm64` digest：amd64 `sha256:61972a87c6f76f9244ff140aaf9650947711abb4024e05fe72df9e39478db317`；arm64 `sha256:e6140cc0821eb2a5cfeae3ccb1da5169bd27ffc790cd787ea95724a3f62a78a6`。
- 附件及 `SHA256SUMS`：8 个附件均为 uploaded，包括双架构 Agent、双架构 Node、部署包、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md；公开 `SHA256SUMS` 与 GitHub 资产 digest 一致。
- 公开镜像 `image_e2e=pass`：`arena-154` 从 Docker Hub 按不可变摘要重新拉取，health 返回 1.19.0-rc.1，revision 精确匹配，静态资源、初始化、安全 Cookie、受限网络和清理断言通过；E2E 日志 SHA-256 `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`。
- `kejilion/apps` / `kejilion.sh` 契约结论：KPanel 与 `kejilion/apps@b9be0ca3b56c5f81a463dc38aba891d06a52ab95` 的 `kpanel.conf` Git blob 同为 `95ed487653184e6c056ea9d25958539a453cf9cb`，无需应用市场提交且默认仍为 `latest`；`kejilion/sh` 已先行推进到 `4d61f7ef`。

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

- 源码/tag：稳定回滚点 `v1.18.0` / `f71cd9a8d6c980fa71eca8045493ef72076c263d`；配套脚本回滚点 `kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762`。
- 镜像 digest：`sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 数据/配置备份：不适用（预览版禁止生产部署）；预览发布未修改生产数据或配置。
- 回滚步骤和回滚后复核：已安装 RC 的测试实例需显式选择上述 v1.18.0 摘要并按标准更新事务备份、恢复及复核；退出预览只切换来源，不自动降级。
- 回滚后生产实际版本与健康状态：本轮未部署也未回滚生产；生产版本保持 v1.18.0。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均保持 v1.18.0 / `sha256:cad0f7f035e3f6d63908a82b52a7b4a1fbe52fe0c10a7a55a704b759ad91ceeb`。
- 公共默认更新通道决策：不适用；稳定默认入口保持 v1.18.0，预览用户通过 `preview`/RC 显式加入。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-14T21:36:15+08:00
- 候选冻结时间：2026-09-15T00:58:31+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成回滚、紧急热修复或重复发布。以下流程异常均发生在生产写操作前；本次预览版没有生产写操作。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-freeze/release-l3/late-dependent-candidate",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 L3 已基于早期冻结 SHA 启动后，等待中的 Komari/批量接入任务交付了必须联动的真实候选；旧 L3 证据随即失效并中止，未推送 KPanel 分支或触碰公共通道。",
    "recoveryEvidence": "清理旧远端 Runner 和输入目录后重新冻结 02b3fb37，权威 v1.19.0-rc.1-02b3fb37-l3-r1、候选/main/tag 门禁及公开 OCI E2E 全部通过。",
    "permanentAction": "后续候选冻结清单必须先确认所有显式等待任务已给出终态或明确移除范围；冻结后新增依赖继续强制重建唯一 L3。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-publish/ssh-auth/scoped-deploy-key",
    "position": "before-production-write",
    "count": 1,
    "impact": "kejilion/sh 首次 SSH 推送被当前部署密钥作用域拒绝，脚本候选尚未发布，因而 KPanel 联动候选继续阻断。",
    "recoveryEvidence": "改用已验证的 HTTPS 凭据路径后，sh main 与候选分支均精确指向 4d61f7ef，GitHub API 和 raw 内容完成回读。",
    "permanentAction": "负责人 release operator 于 2026-09-30 前为 sh 配置专用可写凭据；退出条件是 SSH 预检可读取并推送临时受控 ref，未满足前统一使用受凭据管理的 HTTPS 路径。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-publish/https-transport/system-proxy-required",
    "position": "before-production-write",
    "count": 2,
    "impact": "sh 发布和发布后远端 ref 复核各有一次直接 HTTPS 连接 GitHub 443 超时；未产生半写入，但必需的发布或复核步骤需要重试。",
    "recoveryEvidence": "两次均使用系统代理 http://127.0.0.1:10808 与 HTTP/1.1 完成，远端 main/候选 ref 和 GitHub API 都返回 4d61f7ef。",
    "permanentAction": "负责人 release operator 于 2026-09-30 前把 sh HTTPS remote 固定到受管代理配置；退出条件是无命令行覆盖时连续两次 ls-remote 与受控推送成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "version-check/local-shell/ambiguous-windows-bash",
    "position": "before-production-write",
    "count": 1,
    "impact": "版本检查首次调用 Windows PATH 中的 WSL bash 后无输出挂起，需要终止；候选文件未改变。",
    "recoveryEvidence": "改用 C:/Program Files/Git/bin/bash.exe 后固定版本检查通过，最终 L3、CI 与 Release 再次覆盖。",
    "permanentAction": "Windows 上的仓库 Bash 检查统一通过 scripts/run-repo-bash.mjs 或显式 Git Bash，禁止直接解析 PATH 中的 bash。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-ci/api-parser/powershell-pipeline-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI 首次状态汇总因 PowerShell foreach 后空管道语法失败，只影响只读状态采集，没有影响 GitHub 作业。",
    "recoveryEvidence": "先累积对象再格式化后成功读取两条候选门禁，最终记录了精确 run ID、SHA、success 终态和全部步骤。",
    "permanentAction": "PowerShell GitHub 状态采集固定使用数组累积或括号包裹枚举结果，避免在 foreach 语句后直接接管道。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-image/docker-cli/unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 发布端没有 Docker CLI，首次本地 OCI manifest 核验无法执行；镜像和通道已由 Release workflow 正确发布。",
    "recoveryEvidence": "在已登记的 arena-154 candidate-validation 环境使用 Docker 29.6.2 读取版本、preview、latest 清单并完成不可变摘要冷启动 E2E。",
    "permanentAction": "公开 OCI 验证固定在 arena-154 的 candidate-validation 用途中运行，Windows 端只负责保存外层证据。",
    "historicalReleases": ["v1.14.1", "v1.15.0"]
  },
  {
    "fingerprint": "public-verification/remote-command/powershell-quote-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次远端 OCI 循环中的 shell 变量被 PowerShell 提前展开为空，命令未得到摘要证据；未修改镜像或生产。",
    "recoveryEvidence": "改用单引号包裹远端程序后读取三个清单，随后上传无变量插值的证据脚本并完成 public_oci_e2e=pass。",
    "permanentAction": "该指纹在滚动版本内重复；下一次 L3 生产写前由 release operator 将公开 OCI 检查收敛为仓库固定入口，退出条件是回归覆盖 PowerShell 发起、变量保真、摘要和清理。",
    "historicalReleases": ["v1.18.0"]
  },
  {
    "fingerprint": "public-verification/remote-script/windows-crlf-stream",
    "position": "before-production-write",
    "count": 1,
    "impact": "尝试把 PowerShell here-string 直接管入远端 bash 时因 Windows 行结束/流边界导致 unexpected end of file，未得到 OCI 摘要。",
    "recoveryEvidence": "随后使用单引号远端程序完成清单核验，并通过 scp 上传固定 run.sh 执行 E2E；远端 summary、SHA256SUMS 和清理断言通过。",
    "permanentAction": "远端多行验收逻辑使用仓库文件或先落盘再 scp，禁止从 PowerShell here-string 直接流式执行。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收（按 `docs/project-management.md` 13.1）：保留预览候选工作树、远端候选分支、L3/公开 OCI 证据和配套脚本工作树，供同一 `1.19.0` 序列继续追加 RC；公开 E2E 临时容器和网络已清理。未删除其他任务工作树或唯一证据。
- 未验证风险：真实第三方通知长期送达、100 台真实节点并发、原生 arm64、弱网、真实断电、长期 soak、200% 缩放和完整键盘/焦点矩阵。
- 已实现待实机准入：预览用户实际安装、退出预览不降级和跨地域批量接入需要在 RC 反馈期继续观察。
- 不阻断本版的理由：最终 SHA 已通过完整 L3、候选/main/tag 门禁、双架构公开 OCI 与隔离冷启动；稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：下一个 RC 沿用 `release/v1.19.0-candidate`；进入稳定版前补预览反馈复核、真实批量节点与通知通道专项，并解决重复 PowerShell 远端命令指纹。
