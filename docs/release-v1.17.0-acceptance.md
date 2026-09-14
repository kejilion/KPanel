# KPanel v1.17.0 发布验收

日期：2026-09-14

发布级别：L3

候选提交 / 标签：`22c3e17c209779e3d5fe36b30ac70bd8598deec6` / `v1.17.0`

上一稳定版本 / 回滚点：`v1.15.1` / `36ee5bc638b7c981d035c0542f3c587d6dbfb7d4` / `sha256:daee9c987b7804d152050c6aa141b8b2c60c9c0e8bfad27a621dc302d62e77f6`

## 发布画像

- 业务域：Alpine/OpenRC 主机和轻量节点、KPanel 稳定版自动更新、集群通知可读性。
- 变更面：Agent/Node 安装与服务管理、应用市场生命周期、自动更新事务与设置页、Telegram 集群消息排版。
- 受影响用户旅程：Alpine 主机安装、更新和卸载 KPanel；systemd 主机自动检查并事务更新；管理员阅读集群告警、恢复、SSH 登录和通道测试消息。
- 保持兼容的文件链路：完整 KPanel 节点继续使用 v1.15.1 的 Noise/HTTP POST 文件链路；轻量节点继续使用 v1.15.0 的 `light-control` / `light-data` WebSocket 实现。
- 风险等级及理由：高风险。跨三个仓库并新增 root 级更新事务和跨 init system 生命周期；最终 SHA 的 L3、三层 CI、公开 OCI E2E、停写备份与生产 postdeploy 已通过。

## 发布范围与未纳入内容

- OpenRC 候选：`kejilion/sh@6ebb945f6d5cb69fdb41e3761de23566acbaf762`、`kejilion/apps@fe8c1ae5fd8dd7814df3d59772fac88795333e30`、KPanel `4637a7344a41ae1cb07795174dd575e3326cbefe`。
- 自动更新候选：`kejilion/apps@b5f24594d0044f348ccd33acdc0387a50d22c66b`、KPanel `f915305dfba7181719c25880d9415856f40536ff`。
- 通知候选：KPanel `615a588ae4535e2a4a7d0983aa39590f3b15e2c4`。
- 发布准备 `e38341b00b5fc845d6213c4408b8877cffa40039` 固定版本、Changelog 与跨仓 pin；`22c3e17c209779e3d5fe36b30ac70bd8598deec6` 只让 root 所有权测试在非 root GitHub Runner 上显式跳过，在 root Linux 上仍全部执行。
- 不包含 OpenRC 自动更新定时器；OpenRC 仅提供安装、手动更新、失败恢复和卸载。无数据库 schema、端口、Compose、节点身份或配对密钥迁移。

## 跨仓库联动判定

- `scriptLinkageState=coupled`。
- `kejilion/sh` 主线为 `6ebb945f6d5cb69fdb41e3761de23566acbaf762`；根脚本 SHA-256 `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`，CN 脚本 SHA-256 `90b5831551c609ebe9d7087b83b0e9f9fb6859e8f5ac204830a63d6139829bcf`。
- `kejilion/apps` 主线为 `b5f24594d0044f348ccd33acdc0387a50d22c66b`；`kpanel.conf` SHA-256 `35f589f37cbdba578a457be9365190dac7d5cb8a1163bff7ca5f20314f2e6fe3`。
- KPanel 镜像 label、Dockerfile pin、发布包和生产安装结果均绑定上述 revision；生产允许按既有偏好把镜像内脚本的 `permission_granted="false"` 继承为 `true`，逐行比较确认没有其他变化。
- 三个已合并候选分支和 KPanel 旧重试分支已删除；三仓主线、`v1.17.0` annotated tag 和公开 Release 保留。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Alpine/OpenRC 安装与应用生命周期、systemd 自动更新事务、通知格式和完整 Go/前端测试通过；生产 systemd timer 已启用。 | 未在真实 Alpine 生产主机执行长期运行。 |
| 网络入侵与供应链安全 | 已验证 | Noise/POST 与轻量 WebSocket 边界保持；`govulncheck`、npm audit、Trivy 源码/配置/镜像通过；OCI revision、脚本 SHA 和应用配置精确核对。 | 未执行生产攻击或故障注入。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、自动更新互斥/冷备份/失败回退/中断恢复、应用安装更新回滚卸载和生产停写备份通过。 | 未执行长期 soak、真实断电或原生 arm64 运行时。 |
| 性能与资源预算 | 已验证 | 生产最终约 CPU 0.02%、9.234 MiB/256 MiB、7 PIDs，restart 0、OOM false。 | 单点采样不代表长期 P95。 |
| 用户体验与可访问性 | 已验证 | 通知三语标题、状态图标与分段结构由单元测试覆盖；设置页自动更新状态与被动可用提示由前端测试覆盖。 | 未向真实 Telegram 通道投递，也未重复完整浏览器缩放矩阵。 |
| 数据、配置与迁移 | 已验证 | 64 行数据清单前后相同，仅当天监控 JSONL 增长；SQLite quick check 通过；`agent.env` 只新增自动更新状态目录变量。 | 未执行真实数据恢复演练。 |

## 自动门禁

- 最终 L3 run `v1.17.0-22c3e17-l3-r2` 于 2026-09-14T06:12:22Z 至 06:25:20Z 完成，exit 0。固定 Runner `kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`。
- L3 bundle `9b3c4ac4f1943dd0eff067d0e8c28ed2292a8abdaf44c332d85a2d1b3a51eaaa`、plan `93eccc7d7713bae2a41aa4b9ac6f1ac87bd76a9ef9c498713fef1c983481b767`、remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`；证据为 `C:/GitHub/_release-artifacts/v1.17.0-22c3e17-l3-r2`。
- L3 覆盖 164 项治理/编排测试、完整 Go、核心 race、155 个前端文件/1362 项测试、typecheck、生产构建、双架构二进制、OpenRC 与 systemd 应用生命周期、受管脚本契约、govulncheck、npm audit、Trivy 和最终镜像。
- 候选 CI [34813428361](https://github.com/kejilion/KPanel/actions/runs/34813428361) 与 freshness [34813480240](https://github.com/kejilion/KPanel/actions/runs/34813480240) 成功；main CI [34813759387](https://github.com/kejilion/KPanel/actions/runs/34813759387) 与 freshness [34813759371](https://github.com/kejilion/KPanel/actions/runs/34813759371) 成功；tag freshness [34814370972](https://github.com/kejilion/KPanel/actions/runs/34814370972) 与 Release [34814370868](https://github.com/kejilion/KPanel/actions/runs/34814370868) 成功。六个 run 均绑定最终 SHA。

## 依赖与技术栈变化

- 业务上下文基线为 `v1.12.0` / `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`；候选、main 和 tag 三层 dependency freshness 均通过。
- 本版没有新增 Go/npm 依赖；继续使用 Go 1.26.7、Node 24.20.0、Trivy 0.72.0 和摘要固定的基础镜像/Actions。
- 新增 Alpine/OpenRC 运行适配、job control 和自动更新模块；自动更新只消费稳定版官方镜像和已签入的应用生命周期，使用 root 所有、0700 的状态目录和事务恢复。
- GitHub CI 非 root Runner 对 7 个必须验证 root 所有权的测试显式 skip；同一最终 SHA 在 root Linux L3 中完整执行并通过。

## 隔离真机与浏览器验收

- 所有实机验证和生产操作仅使用 environment policy 允许的 `arena-154`；没有连接或操作已禁用的 `prod-108` / `108`。
- 公开镜像为 `docker.io/kjlion/kejilion-panel@sha256:0fff5dbd9850e085e03cf2d0b74d8f7634ec3776810ef9e246b027922f35ae8a`。
- 固定 `packaging/tests/image-e2e.sh` 在临时端口 18118 验证 version、revision、User `65532:65532`、health、首页、代理头、真实静态资源、bootstrap、Secure Cookie、单网络和最终 container health，输出 `image_e2e=pass`。
- 公开验收证据为 `C:/GitHub/_release-artifacts/v1170-public-22c3e17-r2`；临时容器和网络已清理。未执行真实 Telegram 外部投递、完整浏览器缩放矩阵、长期 soak 或生产故障注入。

## 发布产物与公开仓库复核

- [v1.17.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.17.0) 于 2026-09-14T06:49:17Z 公开，为 Latest、非 draft、非 prerelease；annotated tag 对象 `68e4db5487e0d3fd3c8db9b632fa78956ef4580d` peeled ref 为最终产品 SHA。
- Docker Hub `1.17.0` 与 `latest` 同指 OCI index `sha256:0fff5dbd9850e085e03cf2d0b74d8f7634ec3776810ef9e246b027922f35ae8a`；linux/amd64 manifest `sha256:ba1876b4ca4ea2c7e1172df39756f9798205ed14310a734ad143398408f045d9`，linux/arm64 manifest `sha256:a197208c0e213a86df5811b7b55221e7ef4319bcdcb24e8c97a8d614dc1a00be`，并各带一份 attestation manifest。
- 8 个附件齐全：两种架构 Agent、两种架构 Node、部署归档、LICENSE、SHA256SUMS、THIRD_PARTY_NOTICES.md。公开 API 摘要、大小、状态及六个 Actions run 的 SHA/终态保存在 `C:/GitHub/_release-artifacts/v1170-github-publication-r1`。
- 公开镜像 version `1.17.0`、revision、脚本 revision/SHA、User、healthcheck 和 RepoDigest 全部精确匹配。

## 生产部署安全核对

- 用户明确授权三组候选全部上线。唯一生产目标为 `arena-154` / `154.36.153.9`；本轮未连接、核对、备份或部署 `prod-108` / `108`。
- preflight run `v1170-production-preflight-r1` 于 06:55:43Z 至 06:55:44Z 通过：原生产 `1.15.1`、Panel running/healthy、restart 0、OOM false、Agent active/running/enabled、SQLite 和 64 行数据清单正常。
- backup run `v1170-production-backup-r1` 于 06:56:05Z 至 06:56:14Z 通过，创建 `/root/kpanel-backups/pre-v1.15.1-20260914T065605Z`。数据归档 SHA-256 `c0aecc2a9e3de0cb76d7d1786825116dbe2d2b495d193416bc59e647c8ba7cba`，旧镜像归档 `af8e9d73ef49ee65ee6b3564b652b3e38e0e66b7e4828267eec8d9fa1feffeba`；6 个备份文件、旧镜像加载、保护文件和恢复健康通过。
- 唯一更新入口 `env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel` 于 06:56:31Z 至 06:57:02Z 退出 0，拉取目标 digest、同步 apps 主线并完成 Panel/Agent/脚本事务更新。
- 首次 postdeploy r1 正确识别到 `agent.env` 变化并 fail closed。备份内旧文件与当前文件逐行比对证明唯一变化为新增 `KEJILION_AGENT_SELF_UPDATE_STATE_DIR=/home/docker/kpanel/update-state`；数据清单仅当天监控文件增长。部署后稳定 preflight 和 postdeploy r2 于 06:59:37Z 通过，确认配置不再漂移。
- 最终公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.17.0`。Panel running/healthy、restart 0、OOM false；Agent active/running/enabled；systemd update timer active/waiting/enabled，状态目录 root:root 0700，`agent.env` root:root 0600。
- 生产证据为 `C:/GitHub/_release-artifacts/v1170-production-preflight-r1`、`v1170-production-backup-r1`、`v1170-production-deploy-r1`、`v1170-production-postdeploy-r1`、`v1170-production-stabilized-preflight-r2`、`v1170-production-postdeploy-r2` 和 `v1170-production-final-verification-r2`。

## 回滚

- 源码/tag：`v1.15.1` / `36ee5bc638b7c981d035c0542f3c587d6dbfb7d4`；`v1.17.0` tag 保持不可变。
- 镜像 digest：`sha256:daee9c987b7804d152050c6aa141b8b2c60c9c0e8bfad27a621dc302d62e77f6`。
- 脚本与应用配置回滚点：`kejilion/sh@5c4972229bd9c98669d99c81354c9b79915a02f3`、`kejilion/apps@2d8044adec98e3eb16f47cdbb297f6be9632a66f`。
- 数据/配置恢复包：`/root/kpanel-backups/pre-v1.15.1-20260914T065605Z`，包含数据、配置、service、inspect、旧镜像和 SHA256SUMS。
- 若需回滚，先恢复 GitHub Latest 与 Docker `latest`，再用同一应用市场入口恢复旧镜像和脚本；必要时使用恢复包，并重复 health、Agent、OCI、SQLite、保护文件和日志检查。本轮未触发回滚，默认通道与生产均保持 `1.17.0`。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-14T02:25:42-03:00
- 候选冻结时间：2026-09-14T03:25:20-03:00
- 生产完成时间：2026-09-14T03:59:37-03:00
- 提交到生产用时：1.57 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终产品 SHA 的 L3、候选/main/tag 门禁、Release、公开附件、公开 OCI E2E、停写备份、生产稳定 postdeploy 和公网健康均通过。以下记录流程异常和无效证据拦截，不把它们写成产品故障。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：2
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/nonroot-state-tests/root-owned-tempdir",
    "position": "before-production-write",
    "count": 2,
    "impact": "候选 SHA e38341b 的 CI run 34811624460 与隔离重试 34811979852 在 7 个 root 所有权状态目录测试处失败；L3 root Linux 同一产品逻辑已通过，尚未进入 main、tag 或生产。",
    "recoveryEvidence": "最终提交 22c3e17 让非 root Linux 对这 7 项显式 skip，root Linux 仍全部执行；最终 L3、候选 CI 34813428361、main CI 34813759387 和 Release 34814370868 全部成功。",
    "permanentAction": "root 所有权测试继续在固定 root Linux L3 强制执行，GitHub 非 root Runner 只允许带明确原因的逐项 skip。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-freshness/path-filter/manual-dispatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "最终提交只改测试文件，候选 dependency freshness 未由路径过滤器自动触发，无法直接形成最终 SHA 的 freshness 结论。",
    "recoveryEvidence": "对规范候选分支手动 dispatch，run 34813480240 绑定 22c3e17 并成功；main 和 tag freshness 随后也成功。",
    "permanentAction": "候选冻结后的任何测试或门禁提交都显式检查 freshness 是否绑定最终 SHA，未自动触发时使用登记的 workflow_dispatch。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-verification/windows-upload/crlf-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次通过 PowerShell 文本管道创建公开验收目录时，末行携带 CR，wallpaper 上传在运行 image-e2e 前停止；未运行容器或生产命令。",
    "recoveryEvidence": "改用固定单行目录命令并逐项核对四个输入 SHA-256，随后 r2 公开镜像 E2E 通过且临时资源清理。",
    "permanentAction": "Windows 到 Linux 的多行验收脚本固定写为 LF 文件后 scp 执行；目录创建使用无换行的参数化命令。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-verification/shell-runner/dollar-prefixed-assignment",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开验收 r1 把 Bash 变量赋值误写成美元符号开头且缺少 fail-fast，空镜像引用被拒绝，固定 image-e2e 未运行。",
    "recoveryEvidence": "新建不覆盖的 r2 证据目录，修正赋值并启用 set -Eeuo pipefail；公开不可变镜像输出 image_e2e=pass。",
    "permanentAction": "一次性 Linux 验收脚本在上传前执行 bash -n，并把 set -Eeuo pipefail 固定为首行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "production-postdeploy/protected-config/expected-agent-env-extension",
    "position": "after-production-write",
    "count": 1,
    "impact": "标准更新成功后，postdeploy r1 因 agent.env 哈希变化 fail closed；Panel、Agent、OCI、SQLite 和数据均健康。",
    "recoveryEvidence": "从停写备份提取旧 agent.env 并逐行比较，唯一变化为本版要求的 KEJILION_AGENT_SELF_UPDATE_STATE_DIR；稳定 preflight 与同一固定 postdeploy r2 随后通过。",
    "permanentAction": "下一版在生产写前让固定 postdeploy 入口支持声明并精确验证受控配置迁移，同时继续拒绝未声明的保护文件变化。",
    "historicalReleases": []
  },
  {
    "fingerprint": "production-final-check/managed-script/preference-inheritance-hash",
    "position": "after-production-write",
    "count": 1,
    "impact": "补充核验 r1 错把安装后脚本整体哈希要求为镜像原始哈希，忽略安装器按设计继承许可偏好，因此在公网和运行时契约已通过后停止。",
    "recoveryEvidence": "从不可变镜像重新提取原始脚本并逐行比较，唯一差异为 permission_granted false 到 true；r2 继续核对镜像 label SHA、允许的偏好集合、应用配置、timer 和权限后通过。",
    "permanentAction": "安装后脚本核验固定检查镜像原始 SHA，并对安装副本执行只允许 permission_granted、ENABLE_STATS 和 canshu 偏好行变化的逐行比较。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 远端三个规范候选分支和 KPanel 隔离重试分支已删除；公开 E2E 临时容器与网络已清理。L3、发布、生产证据和唯一恢复包保留。
- OpenRC 自动更新定时器本版未启用；进入该范围前必须完成真实 OpenRC PID 1、重启持久化、并发更新和失败恢复验收。
- 未验证边界包括真实 Alpine 生产长期运行、真实 Telegram 外部投递、完整浏览器缩放矩阵、跨地域弱网、原生 arm64 运行时、长期 soak、生产故障注入和实际数据恢复。
- 当前准入结论：KPanel `1.17.0`、`kejilion/sh@6ebb945f` 和 `kejilion/apps@b5f24594` 已上线；默认更新通道和 `arena-154` 生产均为健康 `1.17.0`，上述未验证边界不阻断本版。
