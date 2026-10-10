# KPanel v1.25.1 稳定安全补丁发布验收

日期：2026-10-10

发布级别：L3

候选提交 / 标签：`8affed894e4dd0a2a2f0af3c1f1d12318a60361a` / `v1.25.1`

上一稳定版本 / 回滚点：`v1.25.0` / `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83` / `sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`

`releaseChannel`：`stable`

`releaseTrain`：`1.25.1`

产物已发布（2026-10-10T08:57:44Z），生产未部署。

## 候选分支与发布后处置

- 原分支 / 精确 tip / 分类：`release/v1.25.1-candidate` / `8affed894e4dd0a2a2f0af3c1f1d12318a60361a` / 稳定补丁已发布。
- 归档 ref / SHA：`refs/heads/archive/release/v1.25.1-candidate` / `8affed894e4dd0a2a2f0af3c1f1d12318a60361a`。Release workflow 已原子归档并移除活跃引用；本任务再次通过唯一入口只读核验为 `already-archived`，保留不可变标签恢复点。
- 来源：安全修复 `35dcc3df9952e301d3f7704f68c68ed2c5898ab4` 的 Go/x/net 构建修复；稳定运行代码来源 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`。批准 main 为 `1c930e34c1d1e04155fd6b1f1185ce4f03e09da9`，通过向前提交恢复稳定运行树，未重写共享历史。
- 预览分支 `release/v1.26.0-candidate` 保留 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`；`preview` digest 仍为 `sha256:46ee78d7e322adf15f7a9664d225d6134311ecc4f2090191da3a25f8c02f1f84`。其他功能作者分支不属于本补丁，未归档或删除。
- 后续 1.26 候选必须基于届时批准 main，向前恢复 RC2 运行树或重放所需差异。RC2 提交已在历史中，直接 merge 旧候选不会自动恢复被稳定快照替换的功能；须重新冻结和完整 L3。
- 本地产品与本文档工作树在本记录主线 CI、公开复核通过前保留；之后仅回收本任务 clean、可由精确 tag/归档及已验证 bundle 恢复的工作树，并保留本地归档分支、解除旧 upstream。执行结果另存本轮外部 closeout 原件，未提前记为已清理。

## 发布画像

- 业务域：稳定通道的网络解析、HTTP 服务及构建供应链安全。
- 变更面：Go 标准库/x/net 和构建环境；发布版本身份及稳定运行树恢复。
- 受影响旅程：现有稳定用户安装或升级至安全补丁，Panel/Agent 重启后继续使用原业务。
- API、数据格式、端口、Compose、Agent 权限、内置脚本和应用市场契约沿用 v1.25.0；无数据迁移。
- L3：公开不可变产物、双架构镜像及公共默认更新通道发生变更。

## 发布范围与未纳入内容

- Go 1.27.1 → 1.27.2；`golang.org/x/net` v0.59.0 → v0.60.0，修复上游网络与标准库安全问题。
- HTTP Range CPU 耗尽（GO-2026-6609/CVE-2026-78667）涉及稳定版 ServeFile 路径；multipart 内存限制绕过（GO-2026-6608/CVE-2026-94440）涉及上传解析，现有请求总长度限制仍保留。未执行攻击性漏洞利用测试。
- 上游依据：[Go 1.27.2](https://go.dev/doc/devel/release#go1.27.2)、[Range 通告](https://pkg.go.dev/vuln/GO-2026-6609)、[multipart 通告](https://pkg.go.dev/vuln/GO-2026-6608)。
- 两个本轮提交：`1a23472317a7f85d359a9cb4ff321707dad22efa`（稳定树与安全修复）、`8affed894e4dd0a2a2f0af3c1f1d12318a60361a`（独立复核指出的业务基线文档修正）。
- 对比 v1.25.0：1495 个 internal/cmd/web/src 运行源文件字节相同，只有 internal/version 身份变化；npm 锁定依赖图保持稳定版，仅根版本号更新。go.mod/go.sum 仅含本补丁的 Go/x/net 修复。
- 1.26 的下载加速/BT、新 Docker 批处理、手动续签及桌面/Office 新能力继续属于 RC；稳定依赖图没有 Otel/BT/pion/gorilla，不引入 RC 的 Otel 修复或 MPL 依赖。

## 外部审计与修复交付

- 覆盖检查：`decision=ok`；未审计提交数 65、最早 6 天、完整审计年龄 19 天、`newPackages=[]`。精确目标 `8affed894e4dd0a2a2f0af3c1f1d12318a60361a`，稳定版 `--require` 返回 0；冻结及推送前原始 JSON 均保留，不将 65 个提交称为全部已审计。
- 未新增信任边界包，本轮没有新增 CF 审计 run。已有 run-31/32 仍为 incomplete、零完成覆盖；预览稳定化的专项补审与性能准入要求继续保留。
- 既有 `auth-state-dir-fsync-error-revives-revoked-session`、`terminal-sse-output-after-session-revocation`、`backupremote.credentials.allow-http` 仍为 needs_validation，沿用账本责任人与 10/12 或 10/15 截止，不在本补丁宣布关闭或风险接受。
- 独立 Codex 发布复核先 FAIL（业务机器基线过期），修正后 PASS WITH FOLLOW-UP。Claude CLI 未安装，按 provider-unavailable 使用不同 Codex 代理复核；不是不同提供商证明。
- OCR 1.12.11，区间 `1c930e34c1d1e04155fd6b1f1185ce4f03e09da9..1a234723`，60/60 代码文件完整；free-form 0，成立 H0/M0/L0，constrained-only 0。稳定 blob 恢复和构建/版本差异逐项复核，后续仅文档修正；原时间和到最终提交的资格化证据保留，不外推工具有效性结论。
- 修复已到源码、稳定 tag 和公开 Release；生产部署未执行。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本，不适用）；变更集编号：不适用。
- 实际内置脚本：`c3a8bd895f8878d9e4ced7592c91a20c974472a5` / SHA-256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`，与 v1.25.0 相同；未使用 RC 续签脚本 pin。
- 脚本候选：不适用；本版决定：脚本不在范围，`kejilion/sh` 和 `kejilion/apps` 无写入。
- 依据：稳定业务源字节比对、managed_script_contract=pass、应用配置生命周期/锁兼容/备份回滚门禁全部通过。阻断或移除的跨仓库依赖：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 实际边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 完整源码检查、公开 Panel/Agent image E2E、三版本真实 API | 未新增远端实体平台验收 |
| 网络入侵与供应链安全 | 已验证 | govulncheck/npm audit/Trivy、精确 OCI/script/附件摘要 | 既有 CF needs_validation 保持开放；不是无漏洞保证 |
| 稳定性、失败恢复与兼容 | 已验证 | Go race、应用安装/升级事务与备份负例、公开镜像运行 | 生命周期故障为隔离夹具，未执行生产故障注入 |
| 性能与资源预算 | 已验证 | L3 资源看护和受限镜像运行；最低磁盘 9263693824 B、MemAvailable 2484948992 B | 沿用稳定性能基线，未新增吞吐/延迟/长期 soak 测量 |
| 用户体验与可访问性 | 已验证 | 8 项实际公开更新信息与三主题/视口弹窗，6 PNG 人工复核 | 100%/dpr1；未新增原生 125%/200% 缩放、读屏验收 |
| 数据、配置与迁移 | 已验证 | 稳定源和脚本 pin、备份/失败恢复门禁 | 无数据格式迁移，未操作生产数据 |

## 自动门禁

- 完整 L3：`node scripts/run-release-l3.mjs`，run ID `v1.25.1-8affed89-l3-r2`，2026-10-10T07:59:26Z → 2026-10-10T08:15:41Z，终态 passed/0。
- kit manifest SHA-256：`4f784dacb14032835f2dfcb91b1fc3a1e1157fdb0e980d782306499e9d881d80`；bundle：`c77934f791dcc91c31340b2dfde243572c383f4e3bd8da62cbb1546b3af53cd6`；plan：`63e0613229fa69629160e7dbc1fa2200fbf9f10f8c90abf04a7f8c2ed2a2425f`；执行脚本：`21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- Runner：`kpanel-go127-security-runner:go1.27.2-node24.21.0` / `sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`。原始 12 项证据已回收逐项 hash 核验：`C:/GitHub/_release-evidence/v1.25.1-8affed89-l3-r2/remote-evidence`。
- source_checks=pass，606413 ms，identity_unchanged=true；前端 264 文件/2428 passed/6 skipped；Go 全量、特权核心 race、typecheck/build、部署和治理门禁通过。
- govulncheck：可达漏洞 0、导入包漏洞 0，required module 另有 1 个未调用漏洞信号；npm audit 0。Trivy 源码与最终镜像在配置门禁范围内均 0 findings，不把未扫描项称为 0。
- app_conf_lock_compat、app_conf_lifecycle、app_conf_update_backup_parity 均 pass，保留故障注入的预期 Killed/拒绝/恢复原始日志。
- 候选 CI：[38037572174](https://github.com/kejilion/KPanel/actions/runs/38037572174)；Dependency freshness：[38037572262](https://github.com/kejilion/KPanel/actions/runs/38037572262)，精确 SHA 相同。
- 主线 CI：[38038323346](https://github.com/kejilion/KPanel/actions/runs/38038323346)；Dependency freshness：[38038323344](https://github.com/kejilion/KPanel/actions/runs/38038323344)，精确 SHA 相同。
- Release workflow：[38038955129](https://github.com/kejilion/KPanel/actions/runs/38038955129)；Tag Dependency freshness：[38038955196](https://github.com/kejilion/KPanel/actions/runs/38038955196)。
- 发布说明：草稿经规范解析；正式正文实际解析为 3 条摘要/2 条升级提示，API 与弹窗逐项一致。
- 双架构构建、镜像契约、扫描和受限运行门禁通过；公开 OCI index 的 attestation entries=2，workflow 配置 SBOM/provenance；未额外声明独立密码学签名核验。

## 依赖与技术栈变化

- 依赖报告由本提交候选/main/tag 的 Dependency freshness workflow 产生，运行时间以原始 API receipt 为准。默认完整模式遇检测源错误返回 2、EOL/例外到期返回 3，本提交 report 成功；据此资格化完整检测通过。未另行下载报告正文，不编造候选条目数或精确 generatedAt。
- 最近每日审计观察（2026-10-10T08:25Z）：run 37912308544（2026-10-09）在 Audit the current dependency graph 失败，旧源码为 29f74518；原始日志未另行取回，不推断具体原因。本补丁当日完整 L3 govulncheck/npm audit 和候选/main 安全门禁通过，不改写旧每日状态。
- EOL 上次复核 2026-07-28，政策最长 92 天；当前 freshness 门禁通过，没有把未知依赖称为最新。直接/基座升级按既有 1/3/3、7/14/30、14/30/60、30/90/90 天分类期限处置，传递信号归属直接依赖。
- 采用 Go 1.27.2 / x/net v0.60.0；构建 Go Alpine digest `sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673`，Runner Go Bookworm digest `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`。
- Node 24.21.0、固定 Actions、稳定 npm 图及脚本 pin 沿用。Trivy 0.74.0 提示 0.75.0 可用，保留固定扫描器；负责人为后续依赖维护任务，2026-10-17 前评估，退出条件为兼容/资源/回滚验收完成，不在冻结后临时换扫描器。
- Otel 1.47.0 只适用于 RC2 依赖图，本稳定图不存在该系列。新功能和性能专项不因本稳定补丁获得准入。

## 隔离真机与浏览器验收

- 环境：`arena-154`，Debian 13/trixie、x86_64，Docker 版本见 browser-preflight-output.json；仅 candidate-validation/browser-validation 用途。所有浏览器在远端后台 headless 执行，未调用本地有头浏览器。
- 公开镜像 E2E：`docker.io/kjlion/kejilion-panel@sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`，`image_e2e=pass`，独立夹具容器/网络 before/after 相同。
- 后台 job：`arena-154-43980`，passed/0，超时 420 秒；spec SHA-256 `f473a30b730d89cf8f3be9edd2cee24d54d9732183b4da792e324f5c7bb4b5b5`；`C:\GitHub\_release-evidence\v1.25.1/public-notes-browser-r1/remote-evidence`。
- 浏览器 Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`；Playwright 1.55.0、Chromium 140.0.7339.16、Node v24.18.0；固定 Node/Playwright 文件 SHA 和实际资源原件保留。
- 8 项：当前稳定、上一稳定和 RC2 各自读取实时 stable/preview Release API；当前稳定拒绝重装，RC2 拒绝降级；v1.25.0 → v1.25.1 弹窗 desktop-dark 1280×900、narrow-light/narrow-dark 390×844。
- 实际 3/2 正文、版本、digest、官方链接逐项相同；最小字号 ≥13px、无横向溢出、末条升级提示可见、Escape 关闭；6 张 PNG 人工复核通过，trace 已回收。
- console/page/resource errors=0，安装请求=0，自动安装始终关闭，夹具与专属网络已回收。真实 Panel/Agent；Docker 只读代理强制专属空应用命名空间，拒绝所有写入，宿主 Docker socket 未挂入 Agent。
- 未执行原生缩放、ARM64 浏览器、真实安装/生产回滚、跨平台实体客户端及长时间 soak；纯工具链安全补丁不冒充新增性能或平台资格。

## 发布产物与公开仓库复核

- [KPanel v1.25.1](https://github.com/kejilion/KPanel/releases/tag/v1.25.1)：draft=false、prerelease=false，GitHub Latest=v1.25.1。
- Docker 版本/latest OCI index：`sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`；amd64：`sha256:7a1417e0fdeb68d07e593b6a81457be8d1adc7378f5eb06fc426b2d1f3e232d8`；arm64：`sha256:0a023d66aa3959e4e6016efe7d2c55b59b79aba5133fe6d27b479d644b432ef6`。
- preview 保持 `sha256:46ee78d7e322adf15f7a9664d225d6134311ecc4f2090191da3a25f8c02f1f84`，两通道独立。两个平台 OCI labels 的 version/revision/script pin 全部吻合。
- 附件 14 项、SHA256SUMS 11 项与 API asset digest 一致；meta VERSION、LICENSE/NOTICES 和 LICENSES 字节核对。未下载所有二进制字节，不以 API digest 代替独立全量二进制下载证明。
- 公开正文、真实面板发布信息 API 与更新弹窗通过，具体原件见 public-after-r1、public-image-e2e.json、public-browser-result.json。
- 公开 amd64 镜像的 Panel/Agent 实际二进制均经固定 Go 工具读取 build info：go1.27.2、x/net v0.60.0；提取容器未启动，临时容器和副本已回收。原始输出及二进制 SHA-256 见 public-buildinfo-result.json/public-buildinfo-raw.log；未外推为 ARM64 实机运行证明。
- 脚本/应用市场契约不变，无关联仓库写入；默认入口使用 latest，最终安装目标保持 digest 固定。

## 自更新通道验收

- 三个真实版本的 stable/preview API 选择正确；当前稳定无重装候选、较高 RC 没有稳定降级候选，上一稳定可以看到精确 v1.25.1 digest。
- 加入预览与自动安装/立即安装的分离、旧状态 stable 迁移及持久化、systemd 备份/失败隔离由完整源码和应用配置门禁覆盖；本轮浏览器没有发出安装请求，也未重演加入预览确认旅程。
- OpenRC 不支持完整自动更新，轻量 Node 无人值守仍只跟踪稳定；沿用既有明确边界。

## 生产部署安全核对

- 授权：仅正式产物发布和隔离验收，生产未部署。正式生产环境、前后版本/备份/公网状态：未验证；生产写操作：0。
- `prod-108` 全部禁用：本次未连接、未备份、未部署、未升级、未核对。
- 验证环境仅 arena-154 的本轮隔离 fixture；不将隔离安装负例或公开镜像 E2E 写成生产证据。

## 回滚

- 源码/tag：v1.25.0 / `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`；不可变镜像：`sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`。未执行生产回滚，没有生产备份或健康状态可报告。
- 如后续生产出现问题，须另行授权、固定旧 digest、先备份并按正式更新事务恢复，随后核对 Panel/Agent、版本、日志和数据；公共默认通道恢复需要单独核验，历史 tag 不改写。
- 当前 GitHub Latest、Docker latest 与标准默认更新入口均指向 v1.25.1；公共默认更新通道回滚决策：不适用（本轮没有生产失败）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-10T15:40:28+08:00
- 候选冻结时间：2026-10-10T07:58:41.426Z
- 生产完成时间：未验证（仅正式产物已发布，生产未部署）
- 提交到生产用时：不适用（生产未部署）
- 是否回滚、紧急热修复或重复发布：否（未部署生产，仅发布正式产物）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "inputs/task-preflight/validation-argv-omitted",
    "impact": "First task ready qualification failed because the required validations array was empty. No product writes were made before the corrected ready qualification passed.",
    "recoveryEvidence": "task-ready.log retains failure; task-ready-r2.log and corrected task-contract.json retain the successful ready qualification.",
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Retain this exact external preflight/recovery implementation and diagnose recurrence before any production write; current recovery does not claim a permanent shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-artifact-verifier/stale-registry-target",
    "impact": "The first public preflight helper still queried an existing RC2 registry target and incorrectly reported occupation. It did not verify target 1.25.1.",
    "recoveryEvidence": "public-preflight.py and public-preflight.log retain the original target error; public-preflight-r2.py and public-preflight-r2.json verify exact target 1.25.1 absent.",
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Retain this exact external preflight/recovery implementation and diagnose recurrence before any production write; current recovery does not claim a permanent shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-validation/run-release-l3/untracked-release-notes",
    "impact": "The canonical r1 prepare call rejected the untracked draft notes file before product validation or remote execution. The raw diagnostic was Release L3 orchestration failed: candidate worktree must be clean.",
    "recoveryEvidence": "Original tool result chunk 8c7261 retains exit 1; no separate raw file was captured and none is fabricated. The exact draft was moved to the external evidence root; canonical r2 prepare passed and manifest.json binds the final clean SHA.",
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Retain this exact external preflight/recovery implementation and diagnose recurrence before any production write; current recovery does not claim a permanent shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/owned-source/generated-binary-allowlist",
    "impact": "The first deep cleanup guard preserved every target because ignored dual architecture dist binaries were not included in the safe generated-file allowlist. It failed before deleting any target.",
    "recoveryEvidence": "deep-cleanup-execute-r1.log and receipt preserve the guard failure; deep-ignored-diagnostic-output.json identifies exactly ten canonical binaries in two completed source copies; r2 permits only those exact binary paths and verifies cleanup.",
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Retain this exact external preflight/recovery implementation and diagnose recurrence before any production write; current recovery does not claim a permanent shared entry repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/external-helper/powershell-string-escaping",
    "impact": "One preparation command for the second cleanup helper failed PowerShell parsing; the dependent helper invocation then failed because no file had been generated. No remote mutation occurred in that invalid attempt.",
    "recoveryEvidence": "Original tool chunks d8056a and 59fc4e retain both failures; no separate original raw file was captured. prepare-deep-cleanup-r2.cjs carries the exact source transform in a saved Node file, then r2 cleanup completed.",
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Retain this exact external preflight/recovery implementation and diagnose recurrence before any production write; current recovery does not claim a permanent shared entry repair.",
    "historicalReleases": []
  },
  {
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Qualify external helper syntax and full raw output capture before use; this corrected release-specific helper is recovery, not a permanent shared entry repair.",
    "historicalReleases": [],
    "fingerprint": "release-controller/publish-ref/invalid-computed-key",
    "impact": "The first candidate push helper failed JavaScript parsing before running any Git command; no remote mutation occurred in that attempt.",
    "recoveryEvidence": "Tool chunk d78616 retains SyntaxError: Unexpected token +. Computed property syntax was corrected, node --check passed (8f2841), and candidate-push.json/log record the exact successful push."
  },
  {
    "position": "before-production-write",
    "count": 1,
    "permanentAction": "Owner: KPanel release tooling task; review by 2026-10-11 or before the next L3 production write. Qualify external helper syntax and full raw output capture before use; this corrected release-specific helper is recovery, not a permanent shared entry repair.",
    "historicalReleases": [],
    "fingerprint": "evidence/tool-output/truncated-json-capture",
    "impact": "A successful coverage command output was truncated by the tool output budget while being copied to a JSON artifact. The incomplete copy was not accepted as machine evidence.",
    "recoveryEvidence": "security-coverage-prepush-tool-output.log retains the truncated copy; security-coverage-prepush-r2.json is full original process stdout captured before display, with exact target and decision=ok."
  }
]
<!-- kpanel-release-process-incidents:end -->

逐项读取最近 5 个正式验收（v1.25.0 到 v1.21.0），精确指纹均无匹配；保留原始失败及修复资格化，不把现场恢复写成永久共享工具修复。独立业务基线 FAIL 单列为正常门禁产品文档发现，不混算流程异常。无生产写入。

## 遗留风险与后续准入

- 深度清理已净释放约 7.71 GB（7712256000 B），回收 131 个已结束且精确可恢复的源码/重复 inbox 目录，另含用户授权的 25 个可回收构建缓存。清理前后容器/镜像/卷清单相同。
- 原始日志、截图、trace、当前/上一稳定恢复资料、dirty v1.17 源码、未知所有权资源和共享 Go/npm/Trivy 缓存保留；并发 L3 写入会影响各时点可用空间，数值为各次测量净差之和。
- 本轮 L3 work/inbox 与远端重复 bundle 待本文档同 SHA 主线 CI 和最终公开复核通过后由 owned-closeout.py 精确回收；本地完整 kit/bundle 与全部原始证据保留。此处没有提前宣称执行成功，最终结果在外部 owned-closeout-result.json/closeout-result.json。
- 既有 CF needs_validation、RC1/RC2 性能与平台限制继续开放。新功能只在新候选重新冻结/审计/完整 L3 后进入稳定通道；本补丁不提供该准入。
