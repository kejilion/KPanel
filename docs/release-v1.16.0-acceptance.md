# KPanel v1.16.0 发布验收

日期：2026-09-13

发布级别：L3

候选提交 / 标签：`2483538fc4da399cec18763752c93699fd6e2f86` / `v1.16.0`

上一稳定版本 / 回滚点：`v1.15.0` / `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92` / `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9`

最终状态：用户于 2026-09-13 明确要求完整回滚；生产、GitHub Latest、Docker `latest`、标准更新入口和源码主线均恢复到 `1.15.0`。`v1.16.0` 标签、Release、附件和版本镜像保留用于审计与按版本取用，Release 标记为 prerelease/withdrawn。

## 发布画像

- 业务域：集群公开分享、设置页、集群文件流和文件管理跨主机复制。
- 变更面：展示、只读状态、文件写入、集群流协议、持久任务元数据和部署。
- 受影响用户旅程：打开公开集群分享页；查看设置与备份入口；下载、ZIP 和跨主机复制文件；查看、取消、清理和重试持久复制任务。
- 未变化契约：无数据库 schema、Compose、端口、Agent 权限、节点身份、配对密钥或 `kejilion.sh` 应用市场动作迁移。
- 风险等级及理由：中高风险。文件流和跨主机写入路径变化，新增有界任务索引；但不迁移现有数据，最终候选、公开 OCI、停写备份和生产 postdeploy 均通过。

## 发布范围与未纳入内容

- `ec795226`：修复公开集群分享页窄视口布局与刷新稳定性。
- `4a988a12`：把设置页备份与恢复区域移到外观设置之后。
- `fed7c87e`：为集群文件流增加连接、静默心跳、归档和总操作预算，并要求来源结束记录与目标完整响应同时成功。
- `eafc9588`：新增持久化跨主机复制任务、有界队列、真实落盘进度、安全取消、清理和显式重试。
- `2483538f`：固定 1.16.0 版本、Changelog 和发布状态。
- `fix/file-host-switch-context-20260913` 与文件流修复内容重复，没有二次纳入；无关历史分支和 v1.15.0 已发布内容没有重放。

最终差异为 49 个文件、2965 行新增、198 行删除。新增 `${DataDir}/file-transfers/jobs.json` 只保存有界任务元数据，不保存文件内容、密钥、Session 或 Cookie；旧版本可以忽略。

## 跨仓库联动判定

- `scriptLinkageState=not-required`（无需发布脚本（不适用））。
- 变更集编号：不适用。
- KPanel 实际内置脚本：`kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`，根脚本 SHA-256 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048`。
- 脚本候选：不适用；本版未修改脚本动作、安装/更新/卸载路径、宿主机产物或镜像内脚本内容。
- 兼容性证据：候选和公开镜像的受管脚本 revision/SHA、应用生命周期与生产 postdeploy 均精确匹配既有脚本契约。
- 本版决定：脚本不在范围；没有阻断或移除的脚本依赖范围。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 定向传输、完成确认、持久任务和界面测试；L3 全量测试；公开镜像 E2E；生产健康。 | 未在两台物理主机间执行长时间大目录复制。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck` 无可达漏洞，npm audit、Trivy 源码/配置/镜像均无命中；附件、OCI、脚本 SHA 精确核对。 | 未执行生产故障注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 连接/心跳/归档/总操作预算、取消与失败状态回归；核心 race；应用安装、更新、回滚、卸载生命周期；生产备份恢复和 postdeploy。 | 未执行长期 soak、原生 arm64 运行时和真实跨地域弱网测试。 |
| 性能与资源预算 | 已验证 | 有界任务队列、响应和元数据上限进入测试；生产单点约 CPU 0.03%、74.4 MiB/256 MiB、7 PIDs。 | 单点数据不代表 P95 或长期趋势。 |
| 用户体验与可访问性 | 已实现未实机验证 | 公开分享窄视口和刷新、文件窗口主机上下文、任务状态与失败文案均有组件回归，21 个语言目录 2310 条短语通过完整性检查。 | 未做真实浏览器的 125%/200% 缩放、键盘焦点和三主题跨视口验收。 |
| 数据、配置与迁移 | 已验证 | 无 schema/Compose/端口迁移；停写备份与旧镜像加载通过；postdeploy SQLite quick check、保护文件 diff 和数据清单通过。 | 回滚不会删除已复制文件或新任务索引，旧版不展示该索引。 |

## 自动门禁

- 候选 L2 在固定 Linux Runner 完成 Go/vet、157 个前端文件/1373 项测试、i18n、生产构建及 amd64/arm64 构建，证据 `C:/GitHub/_candidate-artifacts/post-v1150-eafc958-l2-r2`。
- 最终 L3 run `v1.16.0-2483538-l3-r1` 于 2026-09-13T14:45:06+08:00 至 14:59:49+08:00 完成，exit 0。固定 Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；plan `cc222fecce871990e6b5b7139de44ffba3b738c22e544161d86a495d88624c58`、remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`、bundle `81c80d99dae8e4ed69110e3e96357a109e337befc9c3319635a296710a311c46`。本地输入与 11 个逐项匹配的远端证据为 `C:/GitHub/_release-artifacts/v1.16.0-2483538-l3-r1` 和 `C:/GitHub/_release-artifacts/v1.16.0-2483538-l3-r1-remote`。
- L3 覆盖 164 项治理/编排测试、Go 全量与核心 race、157 个前端文件/1373 项测试、2310 条 i18n、typecheck、生产构建、双架构二进制、最终镜像、受管脚本契约和 app-conf 生命周期。
- 候选 dependency freshness [34743192055](https://github.com/kejilion/KPanel/actions/runs/34743192055) 与 CI [34743192068](https://github.com/kejilion/KPanel/actions/runs/34743192068) 成功；main dependency freshness [34744137579](https://github.com/kejilion/KPanel/actions/runs/34744137579) 与 CI [34744137566](https://github.com/kejilion/KPanel/actions/runs/34744137566) 成功；tag dependency freshness [34744485165](https://github.com/kejilion/KPanel/actions/runs/34744485165) 与 Release [34744485158](https://github.com/kejilion/KPanel/actions/runs/34744485158) 成功，全部绑定产品 SHA。
- Release 提供 SBOM/provenance attestations；`v1.16.0` annotated tag peeled ref 精确为产品 SHA。

## 依赖与技术栈变化

dependency freshness 在候选、main 和 tag 三层均通过。业务上下文基线为 v1.12.0 / `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`，未超过策略阈值。本版没有新增或升级 Go/npm 依赖、Go 1.26.7、Node 24.20.0、基础镜像、Action、Trivy 0.72 或受管脚本；版本锁文件只同步 1.16.0。没有暂缓候选或新增例外。

## 隔离真机与浏览器验收

- 主机：`arena-154`，linux/amd64、Docker Buildx；环境策略允许 candidate-validation、production-safety-check 和 production-deploy。
- 公开镜像固定为 `docker.io/kjlion/kejilion-panel@sha256:7d1806ac669c942724a8f6dc60fb2df33b67f3f64f9c36b380ce12817ff5d245`。
- `packaging/tests/image-e2e.sh` 在临时端口 18116 验证版本、revision、非 root 用户、受管脚本、健康、首页、代理头、静态资源、bootstrap、Secure Cookie 和单网络，输出 `image_e2e=pass`。证据为 `C:/GitHub/_release-artifacts/v1160-public-image-e2e-r1`。
- 临时容器、网络、源码目录和端口均已清理；没有执行长期 soak 或真实跨物理主机复制，依据是本次生产不主动制造文件写入与故障。

## 发布产物与公开仓库复核

- [v1.16.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.16.0) 于 2026-09-13T15:17:52+08:00 公开；回滚后保留为非 draft、prerelease，标题为 `KPanel 1.16.0 (withdrawn)`。GitHub Latest 已恢复为 [v1.15.0](https://github.com/kejilion/KPanel/releases/tag/v1.15.0)。
- `1.16.0` 版本标签仍指 OCI index `sha256:7d1806ac669c942724a8f6dc60fb2df33b67f3f64f9c36b380ce12817ff5d245`；Docker `latest` 已恢复为 `1.15.0` 的 `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9`。
- 8 个 v1.16.0 附件继续保留：两种架构 Agent、两种架构 Node、部署归档、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES.md。5 个受 `SHA256SUMS` 管理的文件下载后逐项匹配，且全部 8 个文件的本地摘要与 GitHub asset digest 一致；证据 `C:/GitHub/_release-artifacts/v1160-public-assets-r1`。
- 公开默认通道恢复工作流 [34746414210](https://github.com/kejilion/KPanel/actions/runs/34746414210) 成功，精确核对 GitHub Latest、withdrawn 标记和 Docker `latest`；`kejilion/apps` 与 sh 仓库无需变更。

## 生产部署安全核对

- 用户明确授权完整上线。验证和正式生产目标均为 `arena-154` / `154.36.153.9`；`prod-108` / `108` 禁用全部 KPanel 操作，本轮未连接、核对、备份、部署或升级。
- preflight run `v1.16.0-production-20260913` 于 15:11:32+08:00 至 15:11:33+08:00 通过：原生产 `1.15.0` / `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92` / OCI `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9` 健康，Panel running/healthy、restart 0、OOM false，Agent active/running/enabled，SQLite 和 63 行数据清单正常。
- 首次生产写为 15:20:53+08:00。backup 至 15:21:03+08:00 通过，创建 `/root/kpanel-backups/pre-v1.15.0-20260913T072053Z`；数据归档 SHA-256 `17b55aa1411d41565900b353ea53a9b6911648b1d4d0c1c590983a982e90dcd4`，旧镜像归档 `1501853c8997c8611dacf77938da998a1d1ea5aa56c1bb58d2f64a8d9b04017f`。6 个备份文件、旧镜像实际加载、保护文件和旧版恢复均通过。
- 唯一更新入口为 `env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，退出 0并拉取精确新 OCI。
- postdeploy 于 15:21:55+08:00 至 15:21:56+08:00 通过：`1.16.0`、产品 revision、OCI 和受管脚本精确匹配；Panel running/healthy、restart 0、OOM false；Agent loaded/active/running/enabled、NeedDaemonReload=no；`panel/ai.db` quick check ok、顶层 `ai.db` 为空、保护文件 diff 为空。64 行数据清单比基线新增本版空任务索引，运行监控文件继续正常增长。
- 公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.16.0`。三阶段证据分别为 `C:/GitHub/_release-artifacts/v1160-production-preflight-r1`、`v1160-production-backup-r1` 和 `v1160-production-postdeploy-r1`。
- 生产未执行真实跨主机复制、故障注入、数据恢复、DNS/端口/身份切换或回滚演练。
- 用户随后明确要求完整回滚。回滚 preflight 于 15:46:58+08:00 确认生产仍为健康 `1.16.0`；停写备份于 15:47:17+08:00 通过并创建 `/root/kpanel-backups/pre-v1.16.0-20260913T074727Z`。6 个备份文件与校验清单齐全，数据归档 SHA-256 `0148761242c42ab37fba1785e5d6a3ca383031fc3c427b753ba17cfd6c7cb886`，旧镜像归档 `1675408de98b37fac04a0169217fda410e757fc58137326fd55f2730107cf0d0`。
- 标准应用更新入口拉取恢复后的 Docker `latest`，退出 0。回滚 postdeploy 于 15:58:32+08:00 至 15:58:34+08:00 通过：`1.15.0`、revision `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`、OCI `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9` 精确匹配；Panel healthy、restart 0、OOM false，Agent active/running/enabled、NeedDaemonReload=no，SQLite 与保护文件检查通过。公网 health 于 15:58:50+08:00 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.15.0`。
- 回滚三阶段证据为 `C:/GitHub/_release-artifacts/v1160-rollback-preflight-r1`、`v1160-rollback-backup-r1` 和 `v1160-rollback-postdeploy-r1`。本轮只连接和操作登记目标 `arena-154`，未连接 `prod-108` / `108`。

## 回滚

- 回滚已实际完成。源码以 revert 提交恢复 `1.15.0` 产品树，不改写历史；`v1.15.0` / `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92` 是最终产品基线，回滚候选 CI [34746198987](https://github.com/kejilion/KPanel/actions/runs/34746198987) 与 dependency freshness [34746199020](https://github.com/kejilion/KPanel/actions/runs/34746199020) 成功。
- `v1.16.0` annotated tag、Release、附件和 `docker.io/kjlion/kejilion-panel:1.16.0` 保持不可变；Release 标记为 prerelease/withdrawn，避免进入默认升级路径。
- GitHub Latest、Docker `latest` 与标准更新入口均指向 `1.15.0`；生产实际为健康 `1.15.0`，受管脚本保持 `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`。
- 回滚前的 `1.16.0` 数据、配置、service、inspect 和镜像保存在 `/root/kpanel-backups/pre-v1.16.0-20260913T074727Z`；原 `1.15.0` 上线前备份 `/root/kpanel-backups/pre-v1.15.0-20260913T072053Z` 继续保留。
- 回滚保留当前数据，没有恢复旧数据快照；因此已复制文件和 `file-transfers/jobs.json` 不被删除，`1.15.0` 会忽略该任务索引。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-13T08:32:56+08:00
- 候选冻结时间：2026-09-13T14:59:49+08:00
- 生产完成时间：2026-09-13T15:21:56+08:00
- 提交到生产用时：6.82 小时
- 是否回滚、紧急热修复或重复发布：是（用户明确要求完整回滚到 1.15.0；非产品故障）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-09-13T15:46:58+08:00；恢复时间：2026-09-13T15:58:50+08:00；逃逸门禁：未逃逸：本次为用户主动撤回决策，1.16.0 上线与回滚前均保持健康，公开默认通道、生产和源码主线按回滚门禁恢复到 1.15.0
<!-- kpanel-release-metrics:end -->

最终 SHA 的 L2/L3、候选/main/tag 门禁、公开附件、公开 OCI E2E、停写备份与生产 postdeploy 均通过。以下单独记录发布执行、基础设施和证据通道异常，不把它们写成产品失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：23
- 其中生产写操作开始后异常次数：12
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-l2/windows-preflight/missing-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 本机缺少 Go、gofmt 和 Make，L2 本地预检在测试前停止。",
    "recoveryEvidence": "改用登记的 arena-154 固定 Runner，完整 Go/vet、前端、i18n、构建与双架构检查通过。",
    "permanentAction": "候选 L2 先运行工具链能力预检；缺少完整工具链时直接选择登记的固定 Linux Runner。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-l2/bundle/missing-business-tags",
    "position": "before-production-write",
    "count": 1,
    "impact": "首个远端 L2 bundle 未包含业务基线标签，业务上下文门禁在测试前拒绝。",
    "recoveryEvidence": "重建包含 v1.12.0 等必需标签的 bundle 后，业务上下文与完整 L2 通过；最终 L3 由权威入口自动封装 68 个标签。",
    "permanentAction": "L2/L3 bundle 统一从治理入口计算并包含 requiredTags，不手工裁剪业务标签。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-l2/runner-login-shell/path-reset",
    "position": "before-production-write",
    "count": 1,
    "impact": "固定 Runner 首次经 login shell 启动时覆盖预置 PATH，未找到 Go 并在测试前停止。",
    "recoveryEvidence": "使用 Runner 原始环境直接执行同一 Make 入口后完整 L2 通过，最终 L3 Runner preflight 也通过。",
    "permanentAction": "固定 Runner 入口不再额外套 login shell；工具链路径由镜像及 preflight 单一确定。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/api-parser/powershell-pipeline-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI 状态读取脚本把 foreach 块直接接管道，PowerShell 在查询前报语法错误。",
    "recoveryEvidence": "先累积对象再序列化，精确确认候选 CI 与 dependency freshness 成功。",
    "permanentAction": "Windows API 状态解析固定使用对象数组模板，不在 foreach 语句块尾部直接拼接管道。",
    "historicalReleases": ["v1.15.0"]
  },
  {
    "fingerprint": "main-ci/gh-cli/missing-authentication",
    "position": "before-production-write",
    "count": 1,
    "impact": "主线 CI 首次使用未登录的 gh CLI 查询，入口在读取前拒绝。",
    "recoveryEvidence": "改用公开 GitHub API，并从 arena-154 独立复核同一 run ID、SHA 与 success 结论。",
    "permanentAction": "公开仓库 CI 读取先做 gh auth status；无认证时直接使用带速率检查的公开 API。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-ci/github-api/rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机匿名 GitHub API 轮询耗尽 core 配额，单次状态读取返回 403。",
    "recoveryEvidence": "GitHub 公开页面和 arena-154 独立出口确认 Release success，随后 API 记录 run 与 Release 精确信息。",
    "permanentAction": "发布执行负责人在 2026-09-20 前固定 ETag/退避与剩余额度预检；额度不足时使用登记的只读出口，连续三次稳定后关闭临时例外。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-image/registry/connect-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机访问 Docker Registry token 服务连接超时，未形成镜像结论。",
    "recoveryEvidence": "arena-154 的 Docker Buildx 精确确认版本与 latest 同 index，并列出双架构和 attestations；公开镜像 E2E 通过。",
    "permanentAction": "发布执行负责人在 2026-09-20 复核本机 Registry 路由；公开 OCI 验收固定优先在登记的 Docker Runner 执行，路由连续三次稳定后退出例外。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-release/github-read/connect-timeout",
    "position": "before-production-write",
    "count": 2,
    "impact": "本机两次 GitHub 公开读取命令连接超时，附件发布状态和 badge 未在该出口形成结论。",
    "recoveryEvidence": "arena-154 公开下载 8 个附件并校验，GitHub API 与工作流页面确认 Release success/Latest。",
    "permanentAction": "发布执行负责人在 2026-09-20 复核本机 GitHub 路由；超时后只做幂等读取并切换登记出口，连续三次稳定后退出例外。",
    "historicalReleases": ["v1.15.0"]
  },
  {
    "fingerprint": "release-cleanup/candidate-branch/noncanonical-name",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release workflow 只清理 release/v1.16.0-candidate，实际聚合分支使用非规范名称，自动清理未覆盖。",
    "recoveryEvidence": "产品 tag 与 main 精确绑定同一 SHA；发布收尾对该精确分支执行可恢复的定向删除。",
    "permanentAction": "下一候选统一命名 release/vX.Y.Z-candidate，使 Release workflow 的精确包含关系检查和自动清理生效。",
    "historicalReleases": []
  },
  {
    "fingerprint": "backup-health/canonical-gate/connection-reset",
    "position": "after-production-write",
    "count": 1,
    "impact": "停写备份恢复后的首次健康探测遇到连接重置，产品更新尚未开始。",
    "recoveryEvidence": "同一 backup gate 继续有界探测并确认 1.15.0、Panel、Agent、SQLite 和保护文件健康，随后才执行标准更新。",
    "permanentAction": "下一次 L3 前在 run-production-evidence-remote.sh 将恢复窗口的预期连接重置收敛为静默有界重试并补回归，最终健康断言继续 fail-closed。",
    "historicalReleases": ["v1.15.0", "v1.14.1"]
  },
  {
    "fingerprint": "postrelease-ci/powershell-ssh/variable-expansion",
    "position": "after-production-write",
    "count": 1,
    "impact": "批量读取 CI 记录时 PowerShell 提前展开远端循环变量，curl 收到畸形 URL，没有形成证据。",
    "recoveryEvidence": "改用六个显式 run URL，逐项确认候选、main、tag 和 Release 均绑定产品 SHA 且 success。",
    "permanentAction": "Windows 到远端的证据查询使用参数文件或显式 URL 数组，不在双引号 SSH 命令中传递远端 shell 变量。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postdeploy-health/web-opener/url-policy",
    "position": "after-production-write",
    "count": 1,
    "impact": "Web opener 因 sslip.io 域名含 IP 而拒绝打开公网健康 URL，没有发出请求。",
    "recoveryEvidence": "本机 curl 直连同一 HTTPS URL 返回 HTTP 200、ok、initialized 和 1.16.0。",
    "permanentAction": "公网健康证据固定使用仓库允许的 curl HTTPS 入口保存状态码和响应体，不再把该域名交给 Web opener。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postrelease-ci/jq-filter/quote-stripping",
    "position": "after-production-write",
    "count": 2,
    "impact": "查询最终文档 CI 的步骤进度时，跨 PowerShell/SSH 的 jq 字符串常量引号被剥离，过滤器编译失败，没有形成步骤结论。",
    "recoveryEvidence": "最终 CI 继续使用已验证的 run 状态 API 精确核对，不依赖该可选步骤过滤器。",
    "permanentAction": "跨 Shell 的 JSON 处理保存原始响应后在本地 Node 解析，远端 jq 只使用无字符串常量的固定表达式。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-research/rg/windows-glob",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 路径通配符直接传给 rg，首次验收记录检索被路径解析拒绝，没有形成结论。",
    "recoveryEvidence": "改用仓库内固定文件路径读取 v1.15.0 之后的发布记录和当前验收文档。",
    "permanentAction": "Windows 上先用 rg --files 生成候选文件清单，再对明确路径执行内容查询。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-backup/canonical-gate/connection-reset",
    "position": "after-production-write",
    "count": 1,
    "impact": "回滚前停写备份恢复 1.16.0 后的首次健康探测遇到连接重置，生产降级尚未开始。",
    "recoveryEvidence": "同一权威 backup gate 的有界重试确认 1.16.0、Panel、Agent、SQLite、保护文件和旧镜像加载全部健康，随后才恢复公开通道和生产版本。",
    "permanentAction": "在 run-production-evidence-remote.sh 将受控恢复窗口的预期连接重置收敛为静默有界重试，最终健康断言继续 fail-closed。",
    "historicalReleases": ["v1.16.0", "v1.15.0", "v1.14.1"]
  },
  {
    "fingerprint": "rollback-preflight/yaml-parser/missing-dependency",
    "position": "after-production-write",
    "count": 2,
    "impact": "两次尝试用本地 Node 解析一次性 Actions YAML 时缺少 yaml 模块，解析器在读取前停止。",
    "recoveryEvidence": "使用 git diff --check、固定 Action SHA 和 GitHub Actions 实际执行验证工作流语法及行为；最终通道恢复 run 成功。",
    "permanentAction": "临时工作流只使用仓库已有验证入口；未声明的本地 Node 模块不再作为 YAML 前置检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-preflight/action-pin-checker/wrong-path",
    "position": "after-production-write",
    "count": 1,
    "impact": "一次性工作流的 Action 固定检查引用了仓库中不存在的脚本路径，没有形成固定性结论。",
    "recoveryEvidence": "逐项复用仓库现有 workflow 的完整 Action commit SHA，GitHub Actions setup 与登录步骤成功。",
    "permanentAction": "新增工作流前从仓库现有 workflow 读取 Action 固定值，并先用 rg --files 确认可用检查入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-channel/github-release/latest-prerelease-order",
    "position": "after-production-write",
    "count": 1,
    "impact": "首次通道恢复 run 已恢复 Docker latest，但尝试先把当前 GitHub Latest 标记为 prerelease，GitHub 拒绝该顺序，Release 默认通道未改变。",
    "recoveryEvidence": "后续 run 先把 v1.15.0 设为 Latest，再把 v1.16.0 标记为 prerelease/withdrawn；公开 API 精确确认最终状态。",
    "permanentAction": "Release 回滚固定按目标稳定版 Latest、撤回版 prerelease、双重读取校验的顺序执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-channel/github-release/gh-json-field",
    "position": "after-production-write",
    "count": 1,
    "impact": "第二次通道恢复 run 已完成所需 Release 修改，但尾部 gh release view 使用不兼容 JSON 字段导致工作流误报失败。",
    "recoveryEvidence": "改用 GitHub REST 的 prerelease 字段校验；最终工作流 run 34746414210 成功，公开状态与 Docker 摘要同时匹配。",
    "permanentAction": "Release 状态修改与校验统一使用同一 REST API 字段，避免 CLI 展示字段差异。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-ci/github-api/rate-limit",
    "position": "after-production-write",
    "count": 1,
    "impact": "回滚文档候选 CI 轮询耗尽 arena-154 匿名 GitHub API 配额，末次只读查询返回 403，没有改变 CI 或生产状态。",
    "recoveryEvidence": "停止轮询并改用公开 Actions 页面核对同一 run ID、分支和精确提交，待页面给出最终状态后再推进主线。",
    "permanentAction": "CI 状态读取复用现有 run URL 并采用至少 60 秒退避；匿名 API 配额不足时只使用公开 Actions 页面，不切换到新的高频出口。",
    "historicalReleases": ["v1.16.0"]
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- v1.16.0 聚合发布分支在 tag/main/Release/生产和完整回滚均完成后定向清理；保留 L2、L3、公开附件、版本镜像、原上线与回滚三阶段证据及两套生产恢复包。
- 未验证风险：真实跨物理主机的长时间大目录复制、跨地域弱网、原生 arm64、长期 soak、真实浏览器 125%/200% 缩放与键盘焦点、生产故障注入。
- 已实现待实机准入：跨主机复制队列、取消和重试已自动验证，真实跨机长流量在后续专项工作流验证。
- 当前准入结论：v1.16.0 已退出默认通道；其新增状态有界且不自动重放不确定写入，无破坏性迁移。恢复后的 v1.15.0 已通过候选 CI、公开默认通道核对、停写备份和生产 postdeploy。
- 后续门禁：在两台登记的非生产主机补跨物理主机长文件/目录、断线恢复与资源趋势专项；下一次 L3 前闭环重复的备份恢复连接重置指纹。
