# KPanel v1.13.0 发布验收

日期：2026-09-10。发布级别 L3。最终产品提交 `271872628c1882cd444d82829d2bf585fa8ddeb7`，annotated tag `v1.13.0`，tag object `6148ca16016c0d1b27a551c2045e90981e21f07a`。候选基线 main 为 `4d1d0991730709f7ddaf89758b99b7f2684bc41e`；上一稳定版本为 v1.12.0，产品提交 `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`，旧 OCI 为 `sha256:229b81f9d31741f0bb75a1eea470566116951cec791eb70ea7bf2b046a23c34a`。

## 发布画像

- 业务域：light node 文件中继与重新接入、应用市场能力展示、运行中应用任务的关闭确认。
- 变更面：展示、只读、主从协议、宿主机脚本身份切换和部署；没有数据库 schema 迁移。
- 受影响用户旅程：light node 首次连接与断线恢复、文件读取恢复与上传确认、重新接入同一/新令牌、Docker 应用的管理入口、关闭仍在运行的应用任务。
- 未变化契约：Panel 端口、Compose、Agent 权限、生产数据路径、应用市场安装入口和既有 System Center 基线不变。
- 风险等级：高。light node 重新接入涉及宿主机身份文件与连接连续性，文件中继涉及恢复与写操作重放边界；因此采用 paired script、最终 SHA Linux L3、公开镜像 E2E、生产备份和 postdeploy 门禁。

## 发布范围与未纳入内容

本轮从 `4d1d099` 无冲突纳入四组已完成候选，并补齐发布元数据、业务上下文和 Linux 契约测试：

- `d92daa9`：文件中继由服务端持有恢复状态；浏览器只发一个读取请求，旧节点不支持恢复时按 1 秒至 5 分钟退避。写操作不重放，上传等待节点 ACK。
- `1d03057`：light node 安全重新接入，展示精确 enrollment ID 和首报状态；对应 `kejilion/sh` 先发布新的暂存、校验、原子身份切换和同令牌恢复能力。
- `fddfa2f`：`docker_app` / `docker_app_plus` 使用 KPanel 常规入口，不再显示脚本管理；update 与 direct access 仍按实际端口绑定能力决定。
- `843afb3`：关闭仍在运行的应用任务时转入停止确认；后台运行选择仍保留。
- `1c5f628`、`9b227ea`、`39fe280`、`2718726`：同步 v1.13.0 元数据、脚本 pin、CHANGELOG、当前业务基线与 Linux 绑定测试契约。

最终差异为 34 个文件、512 行新增、186 行删除。已逐路径检查，未新增或修改 System Center 页面、路由、API、权限与业务文档；既有 System Center 基线不属于本版内容。AdGuard Home 诊断没有形成候选提交，未纳入本版。

## 跨仓库联动判定

- `scriptLinkageState=coupled`。
- 变更集编号：`v1.13.0-light-node-reenrollment`。
- KPanel 上一内置脚本基线：`9f612efc4f861459c0a525491c7cdf5eda756cf7`，根脚本 SHA256 `9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663`。
- 脚本候选与已发布 main：`7369e1d66d9db304ba9956a9e137930414629ed2`；根脚本 SHA256 `fca0829011778374e067f91539c2d229d15025e5ad665389f1c9d2c3f14bcb59`，CN 脚本 SHA256 `13126260687777e37126001c34179f8dd1e1f66722dac37cadb2a2bbda91a265`。
- 兼容性证据：根/CN `bash -n`、脚本 smoke 和 `tests/test_kpanel_light_node_update.py` 23/23 均在 WSL root 的 `core.autocrlf=false` 精确 bundle clone 中通过；公开 raw 字节与上述摘要一致。KPanel managed-script-contract、cluster/unit tests、最终 L3 和公开镜像提取校验均绑定同一 revision/SHA256。
- 发布决定：脚本兼容版本先纯快进到 `kejilion/sh` main，再冻结 KPanel 候选；KPanel Dockerfile 和来源文档固定该提交与摘要。
- 配对回滚：KPanel 回到 v1.12.0 / OCI `sha256:229b81f9d31741f0bb75a1eea470566116951cec791eb70ea7bf2b046a23c34a`，脚本回到 `9f612efc4f861459c0a525491c7cdf5eda756cf7`。没有阻断后仍保留的依赖范围。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 最终 L3 的 cluster、file relay、appmarket 和前端全量测试通过；paired script 23/23 与 smoke 通过。 | 没有在真实已配对 light node 上执行身份重置或大文件跨节点传输。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck`、`npm audit`、Trivy 源码/配置/最终镜像均为 0；脚本与镜像使用 commit、SHA256 和 OCI digest 固定。 | 未执行长期公网攻击流量或第三方渗透测试。 |
| 稳定性、失败恢复与兼容 | 已验证 | 读取恢复、旧节点退避、写操作不重放、重新接入保留旧连接、原子身份切换和应用关闭失败态均有自动测试；生产有可恢复备份。 | 没有真实跨地域弱网 soak，也未主动在生产触发失败注入。 |
| 性能与资源预算 | 已验证 | L3 核心 race、双架构构建和受限镜像契约通过；生产 256 MiB 容器部署后约 72.92 MiB、CPU 0.02%、PIDs 7。 | 未执行长期 soak、全站 P95 或大规模并发中继基准。 |
| 用户体验与可访问性 | 已验证 | AppsView 41 项、ClusterView 33 项、Files recovery 8 项及全量 1335 项前端测试通过，覆盖隐藏入口、状态文本与关闭确认。 | 最终 SHA 未执行真实浏览器 100%/125%/200%、三语、触屏和读屏器人工矩阵。 |
| 数据、配置与迁移 | 已验证 | 无 schema 迁移；生产 SQLite quick check 通过，保护文件 diff 为空，63 个基线数据文件由备份与部署证据保留。 | 未对真实用户文件执行中继上传或恢复写入。 |

## 自动门禁

定向组合测试为 3 个前端文件、82 项测试，i18n 2228 条、typecheck 和 production build 通过。最终候选 L3 run `v1.13.0-2718726-l3-r4` 于 2026-09-10T04:20:18Z 至 04:34:55Z 完成，exit 0；固定 Runner 为 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，plan SHA256 `e79a86a12d057e6dbafeb9c9e6baee856fbcff9d344802251e37d7eb9dee1949`，remote entry SHA256 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle SHA256 `2ace706007ddc50dbe6a879b7d8884dd3e721f2eab01a3dd2f715abc62852d06`。本地完整证据为 `C:/GitHub/_release-artifacts/v1130-2718726-l3-r4`。

L3 覆盖 Go 全量测试、核心 race、151 个前端文件/1335 项测试、typecheck、生产构建、双架构二进制、安装安全、app-conf 生命周期、版本/脚本/业务上下文/发布覆盖门禁和最终镜像。安全扫描均无命中。

两个中间候选被门禁正确拦截：`9b227ea` 的 Linux 测试仍要求 Docker 应用开放脚本管理；`39fe280` 修正开关后又把回环绑定的既有优先失败原因误写成统一文案。两次都只修改测试契约并生成新 SHA；最终 `2718726` 从头完成 L3，旧证据未拼接，问题未进入候选分支、主线、标签或生产。这属于候选产品/测试缺陷被正常门禁拦截，不计发布流程异常。

候选 dependency freshness [34437759973](https://github.com/kejilion/KPanel/actions/runs/34437759973) 与 CI [34437759975](https://github.com/kejilion/KPanel/actions/runs/34437759975) 成功；main dependency freshness [34438014222](https://github.com/kejilion/KPanel/actions/runs/34438014222) 与 CI [34438014191](https://github.com/kejilion/KPanel/actions/runs/34438014191) 成功；tag dependency freshness [34438531285](https://github.com/kejilion/KPanel/actions/runs/34438531285) 与 Release [34438531279](https://github.com/kejilion/KPanel/actions/runs/34438531279) 成功，均绑定产品 SHA `2718726`。

## 依赖与技术栈变化

候选、main 与 tag 的 dependency freshness 均覆盖 policy 要求的 9 个组。业务上下文基线更新为 v1.12.0 / `0ff1e32`，最终候选距离基线 9 个提交，未放宽 50 提交阈值。最近 EOL 复核仍为 2026-07-28，下次最迟 2026-10-28；本版没有 Go module、npm package、Go 1.26.7、Node 24.20.0、基础镜像、Action 或 Trivy 0.72 升级。

本版采用的跨仓库依赖只有受管脚本 `7369e1d` / `fca082...`，按 `coupled` 完成兼容和回滚证据。没有紧急可达漏洞；未把传递依赖信号自动提升为本版变更。后续依赖采用继续遵守 `dependency-policy.json` 的启动、决策与处置期限。

## 隔离真机与浏览器验收

所有实机只使用 environment policy 中允许 candidate-validation、production-deploy 和 production-safety-check 的 `arena-154`（Linux/amd64、Docker）；没有连接 `108` 或 `prod-108`。最终候选 L3 使用上述固定 Runner；公开 OCI 在 arena-154 的临时端口 18113、只读根、`cap-drop ALL`、`no-new-privileges`、非 root 和独立 Docker 网络中运行固定 `packaging/tests/image-e2e.sh`，输出 `image_e2e=pass`，临时容器、网络、端口与上传脚本均已清理。证据为 `C:/GitHub/_release-artifacts/v1.13.0-public-2718726-r1`。

公开镜像 E2E 覆盖健康、首页、代理头、真实静态资源字节、bootstrap 201、Secure cookie、单网络和最终 container health。没有执行真实 light node 重接入、弱网 soak、生产失败注入、受控回滚演练或最终 SHA 的人工浏览器矩阵；这些风险由自动失败态、paired script、公开镜像与生产恢复证据约束，不写成已实测。

## 发布产物与公开仓库复核

[v1.13.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.13.0) 于 2026-09-10T04:56:10Z 公开，为 Latest、非 draft、非 prerelease。8 个附件齐全：两种架构的 Agent、两种架构的 Node、`kejilion-panel-deploy-1.13.0.tar.gz`、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。5 个受 SHA256SUMS 管理的文件均已下载并逐项匹配，LICENSE 与 notices 的归一内容与标签源码一致；证据为 `C:/GitHub/_release-artifacts/v1130-github-release`。

Docker Hub `1.13.0` 与 `latest` 同指 OCI index `sha256:9dc2ce1cf57bd3f56e001cdcdbca39f40fd3a7a2f6f401258e495d7a440c4043`。linux/amd64 为 `sha256:e2d3a4002cc11c0b39cbc7ce4a2aaae4266681e109f09d5b5ec19efad4d3f934`，linux/arm64 为 `sha256:002d1480d9fe95e228f52d65c162089fa167d2963cbf3ee28034bf5c15afb473`；两个 unknown/unknown 条目为 attestations。公开镜像实际 pull 后核对 version `1.13.0`、revision `2718726`、User `65532:65532` 和脚本 SHA256 `fca082...`，`image_e2e=pass`。

`packaging/kejilion-app/kpanel.conf` 相对 v1.12.0 未变，且与干净的 `kejilion/apps` main 归一全文一致，因此不创建 apps 空提交。发布工作流已自动删除候选远端分支。

## 生产部署安全核对

用户明确授权本次完整上线。验证与正式生产目标均为 `arena-154` / `154.36.153.9`；`prod-108` / `108` 禁用全部 KPanel 操作，本轮未连接、未备份、未部署、未升级、未核对。

生产 preflight run `v1.13.0-production-20260910` 于 04:59:37Z 至 04:59:38Z 确认原生产 v1.12.0、revision `0ff1e32`、旧 OCI `sha256:229b81...` 健康，Panel running/healthy、restart 0、OOM false，Agent active/running/enabled，SQLite quick check 通过。基线为 63 个数据文件，磁盘可用约 24 GiB。

备份阶段于 05:00:17Z 至 05:00:27Z 使用固定入口停写并创建 `/root/kpanel-backups/pre-v1.13.0-20260910T050017Z`，包含数据/配置、旧镜像、service、inspect 和 SHA256SUMS；逐项校验后恢复 v1.12.0 健康，保护文件与 preflight 一致。证据为 `C:/GitHub/_release-artifacts/v1130-production-preflight-r1` 与 `C:/GitHub/_release-artifacts/v1130-production-backup-r1`。

生产仅执行标准业务入口：

```text
env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel
```

命令退出 0，拉取 `latest@sha256:9dc2ce...` 并完成 Panel/Agent/受管脚本事务更新。postdeploy 于 05:01:08Z 至 05:01:10Z 通过：version `1.13.0`、revision `2718726` 和 OCI digest 精确一致；Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite `panel/ai.db=ok`，保护文件 diff 为空，近 10 分钟无 fatal/panic/OOM。部署后资源约 72.92 MiB / 256 MiB、CPU 0.02%、PIDs 7。公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.13.0`。证据为 `C:/GitHub/_release-artifacts/v1130-production-postdeploy-r1`。

生产未执行真实 light node 身份重置、文件上传写入、应用任务强制终止、故障注入或回滚演练。本轮没有生产退化、数据迁移、回滚、紧急热修复或重复正式发布。

## 回滚

- 源码/tag：v1.12.0 / `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`；v1.13.0 tag 保持不可变。
- 镜像：`docker.io/kjlion/kejilion-panel@sha256:229b81f9d31741f0bb75a1eea470566116951cec791eb70ea7bf2b046a23c34a`。
- 脚本：`kejilion/sh@9f612efc4f861459c0a525491c7cdf5eda756cf7`。
- 数据/配置：`/root/kpanel-backups/pre-v1.13.0-20260910T050017Z`，内含旧镜像和已校验 SHA256SUMS。
- 若需回滚，停止 Agent/Panel，恢复备份数据、配置、service 和旧镜像，恢复脚本 pin，再启动并重复 health、Agent、OCI、SQLite、保护文件和日志检查；本轮未实际触发。
- 回滚后生产实际版本与健康状态：不适用，当前生产为健康 v1.13.0。
- GitHub Latest、Docker `latest` 与标准更新入口当前均指向 v1.13.0；公共默认通道无需回退。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-10T10:59:14+08:00
- 候选冻结时间：2026-09-10T12:19:22+08:00
- 生产完成时间：2026-09-10T13:01:10+08:00
- 提交到生产用时：2.03 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终 SHA 的 L3、候选 CI/freshness、main CI/freshness、tag freshness、Release、公开镜像 E2E 和生产三阶段 gate 均为各自首轮产品通过。以下只统计使发布编排、取证或证据无效并需要重试的流程问题；候选业务/测试缺陷另在自动门禁章节记录。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：20
- 其中生产写操作开始后异常次数：3
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/web-dependencies/missing-install",
    "position": "before-production-write",
    "count": 1,
    "impact": "新候选 worktree 首次前端定向测试找不到 vitest，未形成测试结果。",
    "recoveryEvidence": "执行 npm ci 后同一组合 82 项测试、typecheck、build 和最终 L3 全部通过。",
    "permanentAction": "本地前端验证先安装 package-lock 固定依赖，再启动测试。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-source/windows-line-endings",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 生成的脚本源归档被 checkout 为 CRLF，bash 语法失败，不能作为脚本候选证据。",
    "recoveryEvidence": "改用 WSL root、core.autocrlf=false 的精确 Git bundle clone，根/CN 语法与测试通过。",
    "permanentAction": "脚本候选验证固定使用不转换换行的 Linux clone，不复用 Windows 工作文件归档。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-tests/non-root-skips",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次 WSL 默认用户运行使 23 项 root 测试全部跳过，该输出被判为无效证据。",
    "recoveryEvidence": "在 WSL root 精确 clone 中重跑，23/23 通过且无 skip。",
    "permanentAction": "脚本测试入口启动前显式断言 uid=0 和 skip=0。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-git/https-fetch-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "kejilion/sh 的 HTTPS fetch 超时，未取得新的远端状态。",
    "recoveryEvidence": "使用 KPanel 已登记 SSH identity 精确查询并纯快进脚本 main 到 7369e1d。",
    "permanentAction": "跨仓库发布读写统一复用已验证 SSH remote 与 identity。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-bundle/missing-head",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次精确 bundle clone 没有默认 HEAD，普通 clone 无法 checkout，未执行测试。",
    "recoveryEvidence": "以 no-checkout clone 后显式 checkout bundle 中的候选分支，取得精确 SHA 并完成测试。",
    "permanentAction": "bundle 生成时包含可解析 HEAD，或验证入口显式传入并 checkout 唯一 ref。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/orchestrator-cli/unsupported-help",
    "position": "before-production-write",
    "count": 1,
    "impact": "对 L3 编排器传入未支持的 --help，只得到 usage，未生成验证证据。",
    "recoveryEvidence": "读取工作流和入口源码，以受支持参数运行最终 L3 与生产证据。",
    "permanentAction": "该指纹已与 v1.12.0 重复；下一次 L3 生产写前为编排器补受支持的 help 契约测试，或移除所有探测调用并以仓库调用示例为唯一入口。",
    "historicalReleases": ["v1.12.0"]
  },
  {
    "fingerprint": "preflight/business-context/stale-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 L3 在候选代码执行前因业务上下文距旧基线 51 个提交被 freshness 门禁拒绝。",
    "recoveryEvidence": "把当前业务真源刷新到已发布 v1.12.0 / 0ff1e32，不改阈值；freshness 与治理门禁通过。",
    "permanentAction": "每次候选冻结前先运行 business-context freshness，并只依据上一已发布版本刷新真源。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/tool-call/javascript-string-syntax",
    "position": "before-production-write",
    "count": 3,
    "impact": "三个本地复核调用因 JavaScript/PowerShell 字符串转义在执行前被拒绝，未形成 Release、tag peeled ref 或生产预检摘要证据。",
    "recoveryEvidence": "拆成简单命令并使用固定参数后，Release 页面、远端 peeled ref 与生产证据均独立核对通过。",
    "permanentAction": "结构化工具调用不内嵌正则反斜杠或复杂多层引号；先用独立 argv/文件承载。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/tag-verification/powershell-peeled-ref",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 PowerShell 展示 ^{} peeled ref 被错误解析，输出含无效 fatal 行；tag 创建本身成功。",
    "recoveryEvidence": "直接读取 tag object 并用引号固定 v1.13.0^{}，本地和远端均指向 2718726。",
    "permanentAction": "PowerShell 中所有 peeled tag 参数必须作为单独单引号 argv 传给 Git。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/github-api/rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "高频匿名 Actions 查询达到 GitHub API rate limit，后续 API 结果不可用。",
    "recoveryEvidence": "降低查询频率，改读公开 workflow、Release 和 expanded assets 页面；Release 与 freshness 均成功。",
    "permanentAction": "匿名轮询采用更长间隔和页面终态，或使用已授权的短期认证；负责人为发布任务，2026-09-17 复核查询配额。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/github-transport/connection-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release 状态与扩展资产页面曾连续连接超时，不能作为终态证据。",
    "recoveryEvidence": "有界重试后页面返回 Release Success、正式发布时间和 8 个附件。",
    "permanentAction": "上游瞬时故障保留有界重试与终态断言；负责人为发布任务，2026-09-17 复核，退出条件为同页面完整读取一次。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-e2e/ssh-wrapper/powershell-expansion",
    "position": "before-production-write",
    "count": 3,
    "impact": "端口预检、测试后退出码包装和首次清理复核三处跨 Shell 转义被 PowerShell提前解释；产品脚本已输出 pass，但外层状态或只读清理证据无效。",
    "recoveryEvidence": "使用远端单引号命令重新核对端口、容器、网络和临时脚本均已清理，产品 metadata 与日志保留。",
    "permanentAction": "公开 E2E 后续使用仓库脚本或结构化 stdin 文件，禁止在 PowerShell 双引号中承载远端命令替换。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-e2e/evidence-hash/open-log",
    "position": "before-production-write",
    "count": 1,
    "impact": "包装脚本在 tee 日志关闭前计算自身日志哈希，最后一行写入后首份 SHA256SUMS 失效。",
    "recoveryEvidence": "测试进程结束后对稳定日志、metadata 和脚本重算 SHA256SUMS，远端校验通过并复制本地。",
    "permanentAction": "证据封存必须由测试进程外层在日志关闭后执行，日志不得自哈希。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postdeploy/evidence-read/javascript-string-syntax",
    "position": "after-production-write",
    "count": 1,
    "impact": "一次本地 postdeploy JSON 汇总调用因字符串语法错误在执行前失败，生产和固定 gate 未受影响。",
    "recoveryEvidence": "拆分读取固定 evidence 文件，确认 status passed、容器/Agent/OCI/SQLite/保护配置全部符合。",
    "permanentAction": "部署后读取先使用简单字段命令，再做本地结构化汇总。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/github-download/connection-timeout",
    "position": "after-production-write",
    "count": 2,
    "impact": "两个 amd64 Release 附件在本地 curl 用尽重试仍连接超时，未取得可验证文件；其他附件已完成。",
    "recoveryEvidence": "从允许的 arena-154 公网出口下载同一公开 URL，按官方 SHA256SUMS 校验后复制本地；最终五个受管附件全部匹配。",
    "permanentAction": "附件下载保留双出口有界回退和官方摘要断言；负责人为发布任务，2026-09-17 复核，退出条件为本地出口连续完成全部附件。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与清理

远端候选分支已由 Release workflow 删除。公开 E2E 的临时容器、网络、端口与上传脚本已清理；保留 L3、公开附件、公开镜像和生产三阶段证据，以及唯一生产恢复包。没有清理其他任务的功能工作树、脚本候选工作树或历史版本证据。

未验证风险包括真实 light node 重接入/弱网/大文件、最终 SHA 的完整人工浏览器矩阵、长期 soak、生产故障注入和受控回滚。它们不阻断本版，因为最终 SHA 的协议与失败态自动测试、paired script 精确测试、公开 OCI E2E、生产备份和 postdeploy 均已通过，且生产未主动改动对应业务数据。后续专项应在非生产 light node 上覆盖新旧 token、首报、断线续传、写操作不重放和长窗口资源趋势，并补真实浏览器缩放、语言与焦点矩阵。

本文件作为产品发布后的独立验收记录提交；`v1.13.0` 始终固定指向 `271872628c1882cd444d82829d2bf585fa8ddeb7`，后续文档提交不是产品 revision。
