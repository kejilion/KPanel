# KPanel v1.24.0-rc.3 发布验收记录

日期：2026-10-01。发布级别：L3。`releaseChannel=preview`；`releaseTrain=1.24.0`。

- 冻结产品候选：`086fcbcc8d41cb8634cb556464f06e376bd94e60`；发布前远端 `main` 基线为 `f9749d5327969f7ecd528f327ac9d726993433cc`。候选 CI 成功且主线基线未移动后快进 `main`，主线 CI 成功并三方 SHA 一致后推送注释标签 `v1.24.0-rc.3`，tag object `cf9ff30c0ee206165bf03ce341074e29b0b7ec66`。
- 上一稳定版及回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96`；Docker index `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。上一不可变预览为 `v1.24.0-rc.2`。
- 产品候选 `release/v1.24.0-candidate` 保留供同序列后续 RC 使用；预览版禁止生产部署。

## 发布画像与精确范围

- 业务域：Agent 初始化、备份工作目录、主机备份排除、文件管理保护目录。
- 变更面：宿主状态根目录解析和状态子系统初始化。L3 验证路径及写入边界，不能用容器测试推断真实 NAS 安装、重启和恢复已经通过。
- 核心旅程：`/home/docker` 链接到实际存储卷时 Agent 可以初始化；归档任务、回收站和备份工作目录使用解析后的状态根；文件管理保护与主机备份排除同时覆盖 KPanel 逻辑路径及真实路径；真实状态根内部的链接继续拒绝。
- 未变化契约：数据库与持久文件格式、文件授权 API、Panel/Agent 权限分工、内置 `kejilion.sh`、端口、Compose、安装及应用市场默认入口。旧数据仍在同一真实目录，无数据库迁移。
- 升级注意：加入预览只切换来源并检查，自动安装开关独立；退出预览不会自动降级。本轮未改变这些通道契约，也未在真实 fnOS/NAS 上重测自更新。

| 来源 | 精确提交 | 内容与处置 |
| --- | --- | --- |
| `fix/agent-backup-symlinked-state-root` | `add02cfc6fefbc90e475ca8fb826f12d935b446d` | 状态根路径解析、CLI 备份工作目录及文件管理保护 |
| 同一来源 | `73e935b7abfffdc775476a6cdd1c6f6f017c57fb` | 主机备份同时排除解析后的 Agent/KPanel 根 |
| 同一来源 tip | `8dbcacad9df3aea9eb9d243ef0a36e4132d27431` | 在 Agent 入口统一解析状态目录，覆盖归档、回收站等子系统 |

合并 `7a30834c54cd6ae73a2ca943dc7b1db28cc512e3`，版本及业务事实准备 `086fcbcc`。merge-tree 和实际合并均只有 `CHANGELOG.md` 内容冲突，保留 rc.2 历史并把新修复归入 rc.3；产品代码没有冲突。来源作者 Claude、最终复核与发布者 Codex 不同，独立复核为 PASS WITH FOLLOW-UP，实机边界见下文。

源分支及其工作树所有权未释放，本轮只读取精确 tip 并在自己的候选合并，原本地分支保持原样；不擅自创建其远端引用或归档。源码提交通过候选和主线公开，来源原分支不因此被宣称已同步。上一轮两条 Claude 远端来源、历史 CF 审计文档分支和未知工作树不纳入本次新范围。

## 外部审计与跨仓库联动

- CF 精确产品覆盖检查 `decision=scoped-required`，52 个提交、97 个文件尚未覆盖，新增边界包 `internal/backupremote`；最近 full 为 `run-4`，9 天。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.3-security-coverage.json`。预览只记录，稳定版前补 scoped；本轮未执行 CF 审计，源码扫描和独立复核不等同 CF 通过。
- 来源 OCR 1.12.6 两次检查点：`range=d5c2622..139f373 files=9/9 free-form=2 valid=H0/M1/L2 constrained-only=0`；`range=73e935b..HEAD files=2/2 free-form=1 valid=H0/M0/L1 constrained-only=0`。保留两个来源评审 trailer；集成和版本准备记录 `skipped reason=release-assembly-preserves-source-review-trailers`，不凭本轮样本推断工具长期有效性。
- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集编号和脚本候选不适用。内置脚本 commit `779192048077c130442a64a126d7c0050776d868`，SHA-256 `33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04`。
- 变更仅解析已有配置根的实际路径，没有新的脚本协议、脚本运行时动作、安装/更新路径、宿主产物或外联配置。Dockerfile、安装配置和 release workflow 相对 rc.2 无变化；L3 受管脚本契约通过。
- 公开 `kejilion/apps` 的 `kpanel.conf` 与候选归一化相同，blob `fa4b95374ed3b186d920207537f451a5b6d839f7`；无需应用市场提交。证据 `C:/GitHub/_release-evidence/v1.24.0-rc.3-apps-contract.json`，默认入口继续稳定版。

## 多维质量与证据边界

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Linux Go 回归覆盖真实临时目录链接、缺失后缀、备份 CLI、统一状态目录、解析后保护与排除；真实 fnOS/NAS 安装和备份恢复未执行。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 状态根内链接继续拒绝；govulncheck 可达漏洞 0，源码/镜像 Trivy 和 npm high/critical 门禁通过；CF scoped 待补，DOMPurify 一项 LOW 保留。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 核心特权包 race、安装安全、镜像与更新/回滚/备份生命周期通过；真实 NAS 重启、旧目录及多盘场景未验证。 |
| 性能与资源预算 | 已实现未实机验证 | 保留既有资源预算，自动镜像门禁有界；未执行真实低配、长时间或多卷性能测试。 |
| 用户体验与可访问性 | 不适用 | 纯后端修复，无界面或语言资源变更；停止与旧冻结 SHA 绑定的 rc.2 模拟预览。本轮未启动新 UI 预览或真实浏览器验收。 |
| 数据、配置与迁移 | 已实现未实机验证 | 无数据库迁移和持久文件格式变化；只解析操作员配置的状态根，同一真实目录继续使用。真实旧 NAS 安装升级未测试。 |

## 自动门禁

- `arena-154` 本轮 SSH 再次超时；沿用用户明确选择的 `local-wsl-dr`，只用于候选 L3。没有扩大为生产或 browser-validation 环境。
- L3 `v1.24.0-rc.3-086fcbcc-l3-r1`：UTC 2026-10-01T01:11:20Z 至 01:19:33Z，`status=passed`、`exit_code=0`。本轮没有 L3 重试。
- 完整 Go、前端 219 文件/1978 测试、typecheck、i18n、场景包复现、核心特权包 race、go vet、漏洞扫描、双架构构建、最终镜像及受管脚本契约、安装/更新/回滚和备份生命周期通过。
- 不可变 Runner `kpanel-release-gate:go1.26.7-node24` / `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- bundle SHA-256 `95e8672f9cef3d16ba12ef0b71030be5f25fcd86481d1a5d54ca27767d330802`；plan `bdc40b6686c71d2873d34b48e0048b2fffca8321de534dc61a260ca98b5284c9`；执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- 原件 `C:/GitHub/_validation/v1.24.0-rc.3-086fcbcc-l3-r1`；终态 `wsl-evidence/status.txt`。12 项回收原件 SHA-256 复核通过，补充复核 `C:/GitHub/_release-evidence/v1.24.0-rc.3-l3-checksums.json`。
- 精确产品 SHA 的候选 [CI 36800666630](https://github.com/kejilion/KPanel/actions/runs/36800666630) 与 [Dependency freshness 36800666666](https://github.com/kejilion/KPanel/actions/runs/36800666666)、主线 [CI 36801567823](https://github.com/kejilion/KPanel/actions/runs/36801567823) 与 [Dependency freshness 36801567812](https://github.com/kejilion/KPanel/actions/runs/36801567812) 均成功。
- 标签 [Dependency freshness 36802415768](https://github.com/kejilion/KPanel/actions/runs/36802415768) 成功。
- 纯验收补充 `8c431a97` 的 [CI 36805869079](https://github.com/kejilion/KPanel/actions/runs/36805869079) 在 UTC 02:32:10 因 Hosted Runner 拉取 Alpine 包索引的 TLS 错误失败，OpenRC 用例未开始；此前源码校验、race、漏洞扫描、Bash 生命周期及备份一致性均通过。原始日志 `C:/GitHub/_release-evidence/v1.24.0-rc.3-failed-ci-job-110189949331.log` 保留；最终新记录候选和主线的 CI 在 closeout.json 单独记录，失败 run 不改写为成功。

## 标签与公开产物

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.3) 于 UTC 2026-10-01T01:55:18Z（北京时间 09:55:18）公开，`draft=false`、`prerelease=true`，非 Latest；[Release workflow 36802415760](https://github.com/kejilion/KPanel/actions/runs/36802415760) 成功。GitHub Latest 仍为 `v1.23.0`。
- Docker [版本镜像](https://hub.docker.com/r/kjlion/kejilion-panel/tags?name=1.24.0-rc.3) 与 `preview` 均为 OCI index `sha256:4491b866b5453cb32b9e12104901761067e92cd52e76809220a265d598a6c2a2`；linux/amd64 `sha256:4289eed4e2181dca929aaad54d63238314d1b0a46daa1ecbaa707fdb7441f69b`，linux/arm64 `sha256:6d73f70d22403015ccf18eb213610268057700e3083ffdb03bcd1177889e0bca`。另有两项 unknown/unknown attestation，流水线启用 SBOM/provenance。
- Docker `latest` 与 `1.23.0` 仍为首段稳定回滚 index，与发布前相同。公共稳定默认入口未改变。
- API 列出 14 个附件。下载的 `SHA256SUMS` 自身摘要 `sha256:a8a2722a75935743ec1d2623c181511022c27bfbccb4093e4eae81190cfdb7b6` 与 API 一致，清单内 11 项 hash 与 API 公布的各附件 digest 一致；未逐一下载、运行各平台二进制。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.3-SHA256SUMS`，完整回读和核验 JSON 同目录保留。
- WSL 显式从 Docker Hub 拉取 `kjlion/kejilion-panel:1.24.0-rc.3`，RepoDigest 与上述 index 相同；OCI version/revision 精确匹配产品候选，用户 `65532:65532`。随后以冻结源码 `packaging/tests/image-e2e.sh` 和 `KPANEL_EXPECTED_VERSION=1.24.0-rc.3` 执行，`image_e2e=pass`、退出码 0。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.3-public-pull.log`、`v1.24.0-rc.3-public-image.json` 和 `v1.24.0-rc.3-public-image-e2e.log`。这是公开 amd64 镜像运行证据；arm64 仅验证 descriptor 和构建，未运行容器；没有真实 NAS、Nginx/PWA 或登记主机浏览器证据。

## 依赖与技术栈

本版没有升级依赖、工具链、Action、基础镜像、扫描器或内置脚本，只更新应用版本及锁文件 version。L3 govulncheck 报告 0 个可达漏洞、1 个未调用的模块通告，退出码 0；不据此称所有依赖无漏洞。npm audit 仍报告 DOMPurify 3.4.14 LOW（GHSA-p98j-92pf-mc4p），补丁 3.4.16 超出冻结范围；当前 AiMarkdown 使用返回字符串模式，没有 `IN_PLACE` 或相关 hook。稳定版冻结前由依赖维护/稳定发布任务复核补丁兼容性，LOW 不触发现有 high/critical 阻断线；完整报告保存在本轮 L3 日志。

## 自更新、生产与回滚

- 自更新契约未改，自动门禁覆盖既有更新、写冻结、备份、失败恢复和回滚；本轮不外推为真实 NAS systemd 自更新验收。OpenRC 与轻量 Node 边界沿用 `docs/release-channels.md`。
- 生产部署安全核对：不适用（预览版禁止生产部署）。`prod-108` 禁用全部 KPanel 操作，本次未连接、未备份、未部署、未升级、未核对；WSL 候选和公开镜像测试不构成生产证据。
- 回滚源码/tag 和镜像见首段；数据/配置备份及生产回退不适用。实际手动回退须先备份并复核兼容，不重写公开标签。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-01T07:53:44+08:00
- 候选冻结时间：2026-10-01T09:09:50+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本轮没有产品测试失败、生产退化、回滚、紧急热修复或同版本重复发布。预览不计入正式部署频率；以下均为生产写前的流程或补充证据命令异常。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记环境 SSH 超时，沿用用户已明确选择的 local-wsl-dr；真实隔离主机及 fnOS/NAS 仍未验证。",
    "recoveryEvidence": "本轮 L3 status=passed/exit_code=0，新的精确候选和 12 项回收摘要复核；不冒充登记实机通过。",
    "permanentAction": "延续登记环境上游可达性期限例外；由当前发布任务于 2026-10-03 前复核，退出条件为 arena-154 SSH 恢复且在登记环境完成相同公开镜像旅程，不扩大灾备用途。",
    "historicalReleases": ["v1.22.0", "v1.23.0"]
  },
  {
    "fingerprint": "candidate-checkpoint/collaboration-state/dirty-before-commit",
    "position": "before-production-write",
    "count": 1,
    "impact": "版本文件尚未提交时提前调用 require-candidate，被干净检查拦截。",
    "recoveryEvidence": "提交 086fcbcc 后相同精确基线的 writer require-candidate 通过；冻结后 HEAD 和工作树未变。",
    "permanentAction": "候选完成检查只在提交后执行；提交前保持 diff/version 检查，不放宽既有门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "supplemental-preflight/business-context/undeclared-entry",
    "position": "before-production-write",
    "count": 1,
    "impact": "补充检查使用了不存在的脚本名称，Node 拒绝执行；唯一 L3 入口和测试未受影响。",
    "recoveryEvidence": "从仓库文件清单确认 check-business-context-freshness.mjs；L3 实际调用既有入口并输出 baseline=7a30834 commits=1 releases=0 通过。",
    "permanentAction": "只使用仓库已声明的入口；由 L3 固定入口提供业务事实证据，不现场猜测或新增 wrapper。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence-recovery/security-coverage/output-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "默认文字输出暂存为 .json 扩展名，未用作 JSON 验收证据。",
    "recoveryEvidence": "原文字保留为 .txt，显式 --format=json 重跑并 ConvertFrom-Json 校验，精确 SHA 与覆盖计数一致。",
    "permanentAction": "保存结构化证据时显式指定输出格式并解析真实样本，文字与 JSON 分别保留。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence-inspection/release-artifact/undeclared-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "补充 tail 查询使用了不存在的日志名，拒绝读取；实际 L3 持续正常执行。",
    "recoveryEvidence": "按真实目录清单读取 l3-verify-release.log；固定入口回收原件及摘要复核通过。",
    "permanentAction": "先读权威文件清单，再用已确认的日志路径；不凭查询失败重跑或改写成功 L3。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-metrics/process-history/prerelease-in-stable-array",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收草稿把 rc.2 写入只允许 stable tag 的 historicalReleases 数组，机器校验拒绝；未提交无效记录。",
    "recoveryEvidence": "保留正式版 v1.22.0/v1.23.0 数组，预览序列重复另由报告识别；修正后相同入口验证通过。",
    "permanentAction": "按模板的封闭指标 schema 填写历史数组，RC 不写入 stable 数组；提交验收文档前执行机器校验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-closeout/command-preflight/invalid-invocation",
    "position": "before-production-write",
    "count": 2,
    "impact": "管理角色调用多传 require-clean 被拒绝；随后归档预检的 PowerShell 字符串漏引号导致解析失败。两次均发生在目标操作执行前。",
    "recoveryEvidence": "仅传 --role management 后通过，原件 v1.24.0-rc.3-management-check.log；解析错误保留于本次任务输出，修正记录继续独立 CI；归档最终 ref/SHA 读回见 closeout.json。",
    "permanentAction": "管理角色使用隐含干净检查；先以读取结果核对 CI 与 ref，再调用带精确 lease 的 Git 原生命令，不重复组合临时 PowerShell 写操作包装。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/openrc/alpine-index-tls",
    "position": "before-production-write",
    "count": 1,
    "impact": "Hosted CI 拉取 Alpine v3.24 APKINDEX.tar.gz 出现 TLS 错误，apk 无索引导致 bash 安装失败，OpenRC 用例未执行；不是已执行的产品断言失败。",
    "recoveryEvidence": "CI 36805869079 的原始日志和 jobs/annotations 保留；补充事实记录形成新文档候选，在镜像摘要与测试入口不变的条件下重新验证，最终 CI 见 closeout.json。",
    "permanentAction": "保留固定镜像与原有门禁，当前发布任务按明确网络证据重试；若再次遇到相同错误，停止并复核 Runner/CDN 网络，不现场更换镜像或省略 OpenRC 测试。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与分支收尾

- 真实 fnOS/NAS 安装、重启、旧目录升级及备份/恢复，CF scoped，真实低配/多卷/长时间负载和公开 arm64 运行待验证。已完成门禁支持本次预览发布，不代表实机或生产准入完成；rc.2 的配对/权限及原生浏览器遗留边界继续以其验收记录为准。
- 产品候选本地与远端对齐到 `086fcbcc`，upstream 为 `origin/release/v1.24.0-candidate`，保留供后续 RC。来源本地分支及工作树保留，所有权未释放；由下一次集成任务在释放所有权或来源交付更新时复核。历史分支和未知工作树不清理。
- 纯验收与公开业务事实同步另走 `docs/release-v1.24.0-rc.3-acceptance` 独立候选 CI → 主线 CI → `archive/docs/release-v1.24.0-rc.3-acceptance` 精确 tip 归档。验收提交的 CI、归档 ref/SHA、远端与本地处置另保存在 `C:/GitHub/_release-evidence/v1.24.0-rc.3-closeout.json`；不把产品 CI 当作验收提交 CI，不改公开产品 SHA。
- 旧 rc.2 模拟预览已停止，manifest 保留 stopped 状态；本轮无 UI 改动，不启动新模拟预览。产品候选工作树与现有 node_modules 保留供后续 RC，唯一 bundle、kit、成功 L3 与历史失败证据保留。
- 已回收自己生成的 `/root/kpanel-release-work/v1.24.0-rc.3-086fcbcc-l3-r1`；删除前复核 realpath、冻结 HEAD、无活跃容器引用、公开 E2E 通过及回收摘要。逻辑释放 442972985 字节；未压缩 WSL VHD，不能等同 C 盘物理释放量。原始远端 evidence 与本机 kit 仍保留，清理证据 `C:/GitHub/_release-evidence/v1.24.0-rc.3-wsl-cleanup.json`。
