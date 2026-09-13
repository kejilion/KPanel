# KPanel v1.15.1 发布验收

日期：2026-09-13

发布级别：L3

候选提交 / 标签：`36ee5bc638b7c981d035c0542f3c587d6dbfb7d4` / `v1.15.1`

上一稳定版本 / 回滚点：`v1.15.0` / `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92` / `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9`

## 发布画像

- 业务域：完整 KPanel 节点间文件浏览、上传、下载和复制；轻量节点文件中继。
- 变更面：集群文件协议、完整节点文件读写链路、传输回归测试、发布与生产证据入口。
- 受影响用户旅程：完整 KPanel 节点通过 Noise 加密连接和 HTTP POST 中继操作远端文件；轻量节点继续通过 `light-control` / `light-data` WebSocket 操作文件。
- 未变化契约：无数据库 schema、配置、端口、Compose、Agent 权限、节点身份、配对密钥、业务数据或 `kejilion.sh` 应用市场动作迁移。
- 风险等级及理由：中高风险。完整节点文件读写协议回退到旧链路，但轻量节点保留 v1.15.0 实现；最终候选、公开 OCI、停写备份与生产 postdeploy 均通过。

## 发布范围与未纳入内容

- `40d6fa67fc16124d97d37084395be3c5b3440230`：完整 KPanel 文件链路恢复到 v1.14.1 的 Noise/HTTP POST 实现，轻量节点文件链路保持 v1.15.0 WebSocket 实现。
- `36ee5bc638b7c981d035c0542f3c587d6dbfb7d4`：固定 1.15.1 版本、Changelog、升级与回滚说明，并收敛停写备份恢复窗口的健康探测噪声。
- 最终产品树相对 `v1.15.0` 为 19 个文件、679 行新增、663 行删除；其中包含 v1.15.0/v1.16.0 已发布与回滚验收记录，产品范围仅为上述两个提交。
- 未纳入 v1.16.0 的公开分享、设置页布局、持久复制任务和新版完整节点 WebSocket 文件流；`v1.16.0` 标签、Release 与版本镜像继续保留为 withdrawn/prerelease 审计产物。

## 跨仓库联动判定

- `scriptLinkageState=not-required`（无需发布脚本（不适用））。
- 变更集编号：不适用。
- KPanel 实际内置脚本：`kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`，根脚本 SHA-256 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：应用市场动作、镜像内受管脚本 revision/SHA、安装更新回滚卸载生命周期均未变化并通过 L3、Release 与公开镜像核对。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Panel 文件路径与 v1.14.1 精确比对；轻量节点路径与 v1.15.0 精确比对；集群/Panel 定向测试和 L3 Go 全量通过。 | 未在两台生产物理主机间主动执行大文件写入。 |
| 网络入侵与供应链安全 | 已验证 | Noise/POST 链路回归；`govulncheck` 无可达漏洞，npm audit、Trivy 源码/配置/镜像无命中；附件与 OCI 摘要一致。 | 未做生产故障注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、完整 Go、应用生命周期、公开镜像 E2E、生产备份恢复和 postdeploy 通过。 | 未执行长期 soak、跨地域弱网或原生 arm64 运行时。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 约 CPU 0.03%、73.41 MiB/256 MiB、7 PIDs，restart 0、OOM false。 | 单点数据不代表长期 P95。 |
| 用户体验与可访问性 | 不适用 | 本版无界面、文案或交互变更。 | 未重复执行浏览器缩放和键盘专项验收。 |
| 数据、配置与迁移 | 已验证 | 无 schema/配置/端口/Compose/身份迁移；64 行数据清单前后相同，SQLite quick check 与保护文件 diff 通过。 | 生产未执行数据恢复演练。 |

## 自动门禁

- 聚合后定向测试通过：完整 Go 编译、`internal/cluster`、`cmd/kejilion-node` 与 Panel 文件路由；L3 再覆盖完整测试集。
- 最终 L3 run `v1.15.1-36ee5bc-l3-r2` 于 2026-09-13T16:38:27+08:00 至 16:52:52+08:00 完成，exit 0。固定 Runner `kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`。
- L3 plan `4ef223577e176ced6a2c0bb9fc20f3c1632665b84b69d29e84a55000a3746292`、remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`、bundle `ad96c3c65eb1298879044c4351cf41c086fcb9110f9d92e8e8d580ab10459e30`；本地输入与 11 个逐项匹配的远端证据为 `C:/GitHub/_release-artifacts/v1.15.1-36ee5bc-l3-r2` 和 `C:/GitHub/_release-artifacts/v1.15.1-36ee5bc-l3-r2-remote`。
- L3 覆盖 164 项治理/编排测试、完整 Go 与核心 race、155 个前端文件/1359 项测试、2294 条 i18n、typecheck、生产构建、双架构二进制、应用生命周期、受管脚本契约及最终镜像。
- 候选 dependency freshness [34748838621](https://github.com/kejilion/KPanel/actions/runs/34748838621) 与精确 SHA CI [34749163917](https://github.com/kejilion/KPanel/actions/runs/34749163917) 成功；后者使用隔离重试分支，产品 SHA 与规范候选一致。
- main dependency freshness [34749432221](https://github.com/kejilion/KPanel/actions/runs/34749432221) 与 CI [34749432133](https://github.com/kejilion/KPanel/actions/runs/34749432133) 成功；tag dependency freshness [34749766625](https://github.com/kejilion/KPanel/actions/runs/34749766625) 与 Release [34749766683](https://github.com/kejilion/KPanel/actions/runs/34749766683) 成功，全部绑定产品 SHA。
- Release 提供双架构 SBOM/provenance attestations；annotated tag 对象 `0be62719142c9762d7ce724956f1cf32dee8230d` peeled ref 精确为产品 SHA。

## 依赖与技术栈变化

- 候选、main 和 tag 三层 dependency freshness 均通过；业务上下文基线为 `v1.12.0` / `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`。
- 本版没有新增或升级 Go/npm 依赖、Go 1.26.7、Node 24.20.0、基础镜像、Action、Trivy 0.72 或受管脚本；版本与锁文件只同步到 1.15.1。
- `govulncheck` reachable 0；modules-not-called 3，未发现进入调用路径的已知漏洞；npm audit 和 Trivy 无高危命中。
- 无暂缓候选、新增例外或 EOL 行动项。
- 升级不迁移数据或配置；完整节点应成对升级。回滚到 1.15.0 会让完整节点回到 WebSocket 文件链路，但不会删除现有文件、配置或节点数据。

## 隔离真机与浏览器验收

- 主机：`arena-154`，linux/amd64、Docker Buildx；环境策略允许 candidate-validation、production-safety-check 和 production-deploy。
- 公开镜像固定为 `docker.io/kjlion/kejilion-panel@sha256:daee9c987b7804d152050c6aa141b8b2c60c9c0e8bfad27a621dc302d62e77f6`。
- `packaging/tests/image-e2e.sh` 在临时端口 18117 验证版本、非 root 用户、健康、首页、代理头、静态资源、bootstrap、Secure Cookie、单网络和最终 container health，输出 `image_e2e=pass`；镜像 label revision 精确为产品 SHA。
- 公开验收证据为 `C:/GitHub/_release-artifacts/v1151-public-verification-r2`；临时容器、网络、源码包和端口均已清理。
- 本版无 UI 变更，因此不重复执行主题、视口、缩放、键盘和多语言浏览器验收；未执行长期 soak、真实跨物理主机文件写入或故障注入。

## 发布产物与公开仓库复核

- [v1.15.1 Release](https://github.com/kejilion/KPanel/releases/tag/v1.15.1) 于 2026-09-13T17:46:08+08:00 公开，为 Latest、非 draft、非 prerelease。
- Docker Hub `1.15.1` 与 `latest` 同指 OCI index `sha256:daee9c987b7804d152050c6aa141b8b2c60c9c0e8bfad27a621dc302d62e77f6`。
- linux/amd64 manifest 为 `sha256:3e1bf5466e36a78266dca9d1e5be9e76fdd5223253c11de321f50ccc72e9e082`，linux/arm64 为 `sha256:51e30654e9f4c1931611e6916257661475e6d965ca1863f235fadae700f20759`；另有两个 unknown/unknown attestations。
- 8 个附件齐全：两种架构 Agent、两种架构 Node、部署归档、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES.md。5 个 `SHA256SUMS` 管理文件逐项匹配，全部 8 个附件的下载摘要与 GitHub asset digest 一致。
- 公开镜像 `image_e2e=pass`，User `65532:65532`、version `1.15.1`、revision 与受管脚本 revision/SHA 精确匹配。
- `kejilion/apps` 与 `kejilion/sh` 无需变更；标准应用市场入口继续使用已发布的脚本组合。

## 生产部署安全核对

- 用户明确授权完整上线。验证和正式生产目标均为 `arena-154` / `154.36.153.9`。
- `prod-108` / `108` 禁用全部 KPanel 操作；本轮未连接、未核对、未备份、未部署或升级。
- preflight run `v1.15.1-production-20260913` 于 17:52:00+08:00 至 17:52:01+08:00 通过：原生产 `1.15.0` / `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9` 健康，Panel running/healthy、restart 0、OOM false，Agent active/running/enabled、NeedDaemonReload=no，SQLite 和 64 行数据清单正常。
- 停写备份于 17:52:18+08:00 至 17:52:28+08:00 通过，创建 `/root/kpanel-backups/pre-v1.15.0-20260913T095218Z`。数据归档 SHA-256 `d900cd306093c1dc7a45f23c977713f8c45c003440f039e9d95b9e34e305dcf6`，旧镜像归档 `1501853c8997c8611dacf77938da998a1d1ea5aa56c1bb58d2f64a8d9b04017f`；6 个备份文件、旧镜像加载、保护文件和恢复健康均通过。
- 唯一更新入口为 `env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，退出 0并拉取精确新 OCI。
- postdeploy 于 17:53:09+08:00 至 17:53:11+08:00 通过：`1.15.1`、产品 revision、OCI、受管脚本均精确匹配；Panel running/healthy、restart 0、OOM false；Agent loaded/active/running/enabled、NeedDaemonReload=no；SQLite 正常、保护文件 diff 为空，64 行数据清单保持一致。
- 公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 于 17:53:23+08:00 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.15.1`。
- 生产三阶段证据为 `C:/GitHub/_release-artifacts/v1151-production-preflight-r1`、`v1151-production-backup-r1` 和 `v1151-production-postdeploy-r1`。
- 生产已执行写操作：停写备份和标准 KPanel 更新；未执行真实跨主机文件写入、故障注入、数据恢复、DNS/端口/身份切换或回滚演练。

## 回滚

- 源码/tag：`v1.15.0` / `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`；`v1.15.1` tag 保持不可变。
- 镜像 digest：`sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.15.0-20260913T095218Z`，包含数据、配置、service、inspect、旧镜像和校验清单。
- 若需回滚，先恢复 GitHub Latest 与 Docker `latest` 到 1.15.0，再用同一标准应用市场更新入口恢复旧镜像；必要时使用该备份恢复数据/配置，并重复 health、Agent、OCI、SQLite、保护文件和日志检查。
- 本轮未触发回滚；生产实际为健康 `1.15.1`。
- GitHub Latest、Docker `latest` 与标准更新入口当前均指向 `1.15.1`。
- 公共默认更新通道决策：不适用；本版验收通过并保持为默认稳定版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-13T16:07:00+08:00
- 候选冻结时间：2026-09-13T16:52:52+08:00
- 生产完成时间：2026-09-13T17:53:11+08:00
- 提交到生产用时：1.77 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终产品 SHA 的 L3、候选/main/tag 门禁、Release、公开附件、公开 OCI E2E、停写备份和生产 postdeploy 均通过。以下单独记录执行器、基础设施和证据通道异常，不把它们写成产品失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：16
- 其中生产写操作开始后异常次数：3
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-preflight/worktree-path/nonexistent",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次终态核对使用不存在的 C:/GitHub/KPanel 路径，在任何标签或远端写入前停止。",
    "recoveryEvidence": "定位规范候选工作树后确认 HEAD、origin/main 与候选分支均为产品 SHA，目标标签不存在。",
    "permanentAction": "发布终态命令从 git worktree list 选择已登记工作树，不再推断仓库目录名。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/local-candidate-sha/argument-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 传入了错误完整 SHA，本地候选校验立即拒绝，未上传 bundle 或运行远端测试。",
    "recoveryEvidence": "用 git rev-parse 获取产品完整 SHA 后，L3 r2 在不可变 Runner 完整通过并回收 11 个匹配证据。",
    "permanentAction": "L3 参数只从当前候选 git rev-parse HEAD 程序化注入，禁止手工补全短 SHA。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/github-hosted-runner/acquisition",
    "position": "before-production-write",
    "count": 2,
    "impact": "候选 freshness run 34748636144 在 5 次获取 runner 后失败，隔离 CI run 34748726798 为 startup_failure；均未执行产品步骤。",
    "recoveryEvidence": "同一产品 SHA 的 freshness 34748838621 和隔离 CI 34749163917 成功，随后 main 与 tag 层门禁也成功。",
    "permanentAction": "上游 GitHub runner 获取失败时仅重试同一不可变 SHA；发布负责人于 2026-09-20 复核，连续三次候选首轮可获取后关闭临时隔离分支策略。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/ref-retrigger/concurrency-cancellation",
    "position": "before-production-write",
    "count": 3,
    "impact": "为恢复 runner 获取而重触发规范候选时，两个 CI run 被并发策略取消，另一个精确 SHA run 34748838521 长时间 queued，未形成权威候选 CI 结论。",
    "recoveryEvidence": "隔离分支只指向同一产品 SHA，CI 34749163917 成功；main CI 34749432133 与 Release 34749766683 再次成功。",
    "permanentAction": "runner 故障恢复固定使用单个隔离重试 ref，并等待其终态后再触发后续层，避免翻转规范候选 ref。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-ci/github-api/rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机匿名 GitHub API 轮询再次耗尽 core 配额，后续状态读取无法从该出口完成。",
    "recoveryEvidence": "GitHub 公开页面与登记的 arena-154 只读出口确认候选、main、tag 和 Release 的 run ID、SHA 与 success 结论。",
    "permanentAction": "发布负责人在 2026-09-20 前固定 ETag、退避与剩余额度预检；额度不足时使用登记的只读出口，连续三次稳定后退出例外。",
    "historicalReleases": ["v1.16.0"]
  },
  {
    "fingerprint": "release-ci/api-parser/powershell-quoting",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次把带字符串比较的 jq 表达式嵌入 PowerShell/SSH 时引号被改写，Release job 状态读取在解析前失败。",
    "recoveryEvidence": "保存 arena-154 返回的原始 JSON 后在本地 PowerShell 解析，精确取得所有 Release 步骤与最终 success。",
    "permanentAction": "跨 Shell API 状态读取固定保存原始 JSON并在本地解析，远端不再拼接含字符串常量的 jq 表达式。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-evidence/scp/windows-crlf-filename",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次从 arena-154 回收公开验收目录时，管道脚本末行产生带 CR 的 status.txt 文件名，scp 对该文件拒绝；公开附件与镜像检查本身已通过。",
    "recoveryEvidence": "第二次回收已完整取得附件、Release JSON、OCI manifest、image config 和 E2E 日志，本地逐项复核后生成无 BOM 的 pass 状态；远端临时目录已清理。",
    "permanentAction": "Windows 向 bash 传递多行验收脚本固定先写 LF 临时文件再 scp 执行，证据目录禁止通过 PowerShell 文本管道创建。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-evidence/runner-source/missing-upload",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开验收 r2 首次启动时，前次清理已删除远端 source tar，入口在解包前停止。",
    "recoveryEvidence": "重新上传同一 v1.15.1 git archive 后，8 个附件、OCI、运行时标签与 image E2E 全部通过。",
    "permanentAction": "公开验收入口在执行前统一上传并校验 source tar，清理只在本地证据回收成功后执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "rollback-research/rg/windows-glob",
    "position": "after-production-write",
    "count": 1,
    "impact": "验收记录历史指纹查询再次把 Windows 通配路径直接传给 rg，路径解析拒绝，未形成历史重复结论。",
    "recoveryEvidence": "改用 --glob 配合固定 docs 目录后完成最近发布记录检索，并声明重复指纹历史。",
    "permanentAction": "下一次 L3 前把验收历史检索固定为 tracked Node 入口并补 Windows 回归；完成前发布负责人只使用 rg --glob 加明确目录。",
    "historicalReleases": ["v1.16.0"]
  },
  {
    "fingerprint": "acceptance-verify/git-worktree/windows-gitdir-path",
    "position": "after-production-write",
    "count": 1,
    "impact": "验收文档提交后的本地 verify-change 在 Windows 链接 worktree 中无法解析 .git 内的 Windows 绝对 gitdir，协作状态预检在测试前停止。",
    "recoveryEvidence": "文档结构、指标和治理一致性已由原生 Node 入口通过；同一提交随后由 GitHub Linux CI 完成权威 verify-change。",
    "permanentAction": "Windows 本地 Bash 验收固定使用独立 clone 或由 run-repo-bash.mjs 规范化 gitdir，不在链接 worktree 中直接启动 Git Bash。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-main/ancestry-check/powershell-empty-output",
    "position": "after-production-write",
    "count": 1,
    "impact": "主线快进前用 PowerShell 布尔判断 git merge-base --is-ancestor 的空标准输出，把退出 0 误判为失败；push 尚未执行。",
    "recoveryEvidence": "远端 main 仍保持产品 SHA；改为读取 LASTEXITCODE 后重新执行同一祖先关系与精确 lease 检查。",
    "permanentAction": "PowerShell 调用以退出码表达结果的 Git 命令时固定检查 LASTEXITCODE，不再判断标准输出。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 远端规范候选分支已由 Release workflow 删除；公开 E2E 临时容器、网络、源码包和端口已清理。保留 L3、公开制品和生产三阶段证据，以及唯一生产恢复包。
- GitHub runner 异常恢复期间遗留的候选 CI `34748838521` 在验收时仍为 queued；它不改变产品 SHA 或已成功的候选/main/tag/Release 结论，后续由 GitHub 终止或执行完毕。
- 未验证边界包括真实跨两台生产物理主机的大文件/目录传输、跨地域弱网、原生 arm64 运行时、长期 soak、生产故障注入和实际数据恢复。
- 这些边界不阻断本版，因为完整与轻量节点代码边界已精确比对，定向测试、最终 L3、三层 CI、公开 OCI E2E、停写备份和生产 postdeploy 均通过，且无数据或配置迁移。
