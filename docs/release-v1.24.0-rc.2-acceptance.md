# KPanel v1.24.0-rc.2 发布验收记录

日期：2026-10-01。发布级别：L3。`releaseChannel=preview`；`releaseTrain=1.24.0`。

- 冻结产品候选和标签提交：`5c97ea1f540bee29c6013dd5477070d9e67d0cd6`；发布前远端 `main` 基线为 `d5c2622a6e5869edd1d7ab2aa4b01cb53bc67486`。候选 CI 通过且主线基线未移动后快进 `main`，主线 CI 通过并三方核对一致后推送注释标签 `v1.24.0-rc.2`，tag object `746011d7933a56de1623026aca3c0d5f4ec57ec4`。
- 上一稳定版及回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96`；Docker index `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。上一不可变预览为 `v1.24.0-rc.1`。
- 产品候选 `release/v1.24.0-candidate` 保留供同序列后续 RC 使用；本轮预览版禁止生产部署。

## 发布画像与精确范围

- 业务域：文件编辑器、跨节点文件代理、AI 文件审批和读取、账户复核、Passkey 登录、宿主文件权限修改。
- 变更面：展示、文件识别、授权协议、配置持久化及宿主机写入边界；无数据库迁移。风险较高的权限变化按 L3 验证，自动门禁不能替代真实双方升级和宿主操作。
- 核心旅程：宽屏目录与标签并列、窄屏目录覆盖；切换多种语法及无扩展名配置；保存冲突保留草稿；v1 配对默认只读摘要，目标管理员允许后才开放文件管理；AI 避开凭据及备份副本；密码和用户名修改复用复核预算；chmod 拒绝路径对象替换。
- 升级注意：既有 v1 配对双方升级后，须在被控端“集群 → 接入 → 已授权控制端”重新允许文件管理；授权绑定当前控制端密钥，保存在 `cluster-v1-file-relay.json`，不随面板备份迁移。加入预览不会自动安装，退出预览不会自动降级。

| 来源分支 | 精确 tip | 纳入与处置依据 |
| --- | --- | --- |
| `claude/awesome-clarke-pdzdcz` | `32013251c36c5738b04070897897662878328642` | 四个提交；编辑器、文本识别、高亮、AI 凭据读取边界；保留原名 |
| `claude/upbeat-curie-x2ei28` | `f3899ad7f39d31fa9d71794544055225c01bc326` | 安全加固；保留原名 |

合并提交 `6dfc3d80`、`9af50e6d`，发布准备 `90bae028`，业务事实刷新 `5c97ea1f`。merge-tree 预检及实际合并无冲突。两条来源 tip 纳入本产品候选，未擅自归档仍可能继续推送的来源会话分支；来源所有权释放后由下一次集成任务复核。历史分支和未知工作树原样保留。

## 外部审计与跨仓库联动

- CF 覆盖以精确产品候选的 `check-security-audit-coverage.mjs` JSON 为准，`decision=scoped-required`，49 个提交、92 个文件未覆盖，新增边界包 `internal/backupremote`，最近 full 为 `run-4`（9 天）。原始 JSON `C:/GitHub/_release-evidence/v1.24.0-rc.2-security-coverage.json`。预览只记录；稳定版前补 scoped。本轮未执行 CF 审计，不把扫描或来源评审写成 CF 通过。
- 来源 OCR 1.12.6：安全线 `range=d5c2622..e6cff87 files=29/29 free-form=0 valid=H0/M0/L0 constrained-only=0`；编辑器线 `range=d5c2622..fb98023 files=7/7 free-form=1 valid=H1/M1/L0 constrained-only=1`，最终 `32013251` 进一步修复连续备份后缀读取边界。发布准备和事实文档提交分别记录 skipped 理由；发布复核者 Codex 与来源作者 Claude 不同。
- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集及脚本候选不适用。内置脚本 commit `779192048077c130442a64a126d7c0050776d868`，SHA-256 `33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04`，L3 受管契约通过。
- `Dockerfile`、安装配置及 release workflow 相对 rc.1 无变化。线上 `kejilion/apps` 的 `kpanel.conf` 与候选归一化相同，blob `fa4b95374ed3b186d920207537f451a5b6d839f7`；无需应用市场提交，公共默认入口继续稳定版。

## 多维质量与证据边界

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 文件、授权、AI、认证有 Go/前端回归；真实双方 v1 升级与授权未测试。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | L3 govulncheck、npm high/critical 门禁、源码和镜像 Trivy 通过；CF scoped 待补，DOMPurify 一项 LOW 保留。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 全量 race、镜像生命周期、更新回滚与备份恢复通过；模拟保存冲突保留草稿、读取失败可重试；真实重启授权持久化待补。 |
| 性能与资源预算 | 已实现未实机验证 | 大文件纯文本降级和授权 sidecar 256 KiB/256 主机上限有代码与测试；真实低配、并发及长时间负载未测试。 |
| 用户体验与可访问性 | 已实现未实机验证 | IAB 模拟 UI 验证 1280×900 与 390px、浅深色、目录/标签及失败态，最小可见字号 13px；原生 125%/200% 缩放、完整键盘/PWA 未验证。 |
| 数据、配置与迁移 | 已实现未实机验证 | 无数据库迁移；v1 授权文件有有界存储、原子写与失败恢复测试，真实旧配置升级及备份迁移未单独测试。 |

## 自动门禁与本地验收

- `arena-154` SSH 超时，经用户明确选择使用登记的 `local-wsl-dr`；WSL Ubuntu/root Docker 只用于候选验证，未扩大为生产或 browser-validation 环境。
- 首轮 `v1.24.0-rc.2-90bae028-l3-r1` 在 source preparation 的业务事实过期门禁被拦截（56 commits ≥ 50），未执行候选测试。原失败记录保留；按真实发布事实更新 canonical review 到 `90bae028`，治理 227 项通过，形成 `5c97ea1f`，使用新 run ID 重试。
- L3 run `v1.24.0-rc.2-5c97ea1f-l3-r2`：UTC 2026-09-30T23:47:34Z 至 23:55:29Z，`status=passed`、`exit_code=0`。完整 Go、前端 219 文件/1978 测试、typecheck、i18n、构建、race、漏洞扫描、场景包复现、双架构构建、镜像契约、安装/更新/回滚和备份生命周期通过。
- 不可变 Runner `kpanel-release-gate:go1.26.7-node24` / `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- bundle SHA-256 `eb7ecea2e2cbb1f1e3b0bd5244143f240b22753ac9303580fd52878855195c39`；plan `8ca5443a32653f8f9a0dfc897adde662ff963e0381a2c92942ee3756c3950f53`；执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- 原件 `C:/GitHub/_validation/v1.24.0-rc.2-5c97ea1f-l3-r2`；终态 `wsl-evidence/status.txt`，回收原件 12 项 SHA-256 复核通过。首轮失败在 `C:/GitHub/_validation/v1.24.0-rc.2-90bae028-l3-r1`。
- 本机定向 3 文件/109 测试与 typecheck 通过。模拟预览 `http://127.0.0.1:4173/files`，仅替换接口数据、使用完整 FilesView；证据 `C:/GitHub/_preview-evidence/kpanel-v124-rc2-5c97ea1f/manifest.json`、`ui-result.json` 和三张截图，控制台错误为空。原生 Chrome 连接不可用，使用 IAB；本机模拟 UI 不是 WSL browser-validation 或真实 Panel/Agent 验收。
- 精确产品 SHA 的候选 [CI 36793788615](https://github.com/kejilion/KPanel/actions/runs/36793788615) 与 [Dependency freshness 36793788630](https://github.com/kejilion/KPanel/actions/runs/36793788630)、主线 [CI 36794570237](https://github.com/kejilion/KPanel/actions/runs/36794570237) 与 [Dependency freshness 36794570270](https://github.com/kejilion/KPanel/actions/runs/36794570270) 均成功；标签 [Dependency freshness 36795488401](https://github.com/kejilion/KPanel/actions/runs/36795488401) 成功。

## 依赖与技术栈

- 本版未升级工具链、Action、直接依赖、基础镜像或内置脚本，只更新应用版本与锁文件 version 字段。依赖 freshness 的最终 run 在产物章节记录；未重写既有依赖政策或历史安全通告。
- npm audit 报 DOMPurify 3.4.14 一项 LOW（GHSA-p98j-92pf-mc4p），补丁 3.4.16 超出当前冻结精确版本。当前 `AiMarkdown.vue` 使用返回字符串模式，没有 `IN_PLACE` 或相关 hook；本次未发现通告触发模式，但不据此称依赖无风险。原始 JSON `C:/GitHub/_release-evidence/v1.24.0-rc.2-npm-audit.json`。由下一次依赖维护/稳定发布任务在稳定版冻结前复核补丁与兼容性；LOW 不触发当前 high/critical 阻断线。

## 标签与公开产物

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.2) 于 UTC 2026-10-01T00:30:59Z（北京时间 08:30:59）公开，`draft=false`、`prerelease=true`，非 Latest；[Release workflow 36795488388](https://github.com/kejilion/KPanel/actions/runs/36795488388) 成功。GitHub Latest 仍为 `v1.23.0`。
- Docker [版本镜像](https://hub.docker.com/r/kjlion/kejilion-panel/tags?name=1.24.0-rc.2) 与 `preview` 均为 OCI index `sha256:613b63476f6e5942cd2e367500c0140861d42e3336c78ce6f32b6e1bda1e2644`；linux/amd64 `sha256:855adc189c27c33351ec6af2e1a82b869d88098612b15ca99f49f4c8858a66e4`，linux/arm64 `sha256:7fdfe48d6c445d7b81950db920643ff24519e220b2cd4c3ce2af4d2a5828672d`。另有两项 unknown/unknown attestation，流水线使用 SBOM/provenance。
- Docker `latest` 与 `1.23.0` 仍为首段的 `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`，与发布前相同。本次未改变公共稳定默认入口。
- API 列出 14 个附件。下载的 `SHA256SUMS` 自身摘要 `sha256:641368bac293beb43492019028cee42decf44e4ba2b2c69d790362b8b3fb1f16` 与 GitHub API 一致，清单内 11 项 hash 与 API 公布的各附件 digest 一致；未逐一下载、运行各平台二进制。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.2-SHA256SUMS`，Release、workflow、Docker Hub 回读 JSON 同目录保留。
- WSL 从 Docker Hub 显式拉取 `kjlion/kejilion-panel:1.24.0-rc.2`，RepoDigest 与上述 index 相同，OCI version/revision 精确匹配产品候选；用户 `65532:65532`、healthcheck 与受管脚本标签一致。随后用冻结源码的 `packaging/tests/image-e2e.sh`、`KPANEL_EXPECTED_VERSION=1.24.0-rc.2` 执行，输出 `image_e2e=pass`，退出码 0。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.2-public-pull.log`、`v1.24.0-rc.2-public-image-e2e.log`、`v1.24.0-rc.2-public-image.json`。这是公开 amd64 镜像运行证据；arm64 仅检查发布 descriptor 和构建，未运行容器；没有真实 Nginx/PWA 或登记主机浏览器证据。

## 自更新、生产与回滚

- 自更新通道契约未改，本轮未在真实宿主重测；自动门禁覆盖既有更新、冻结、备份和失败恢复。加入预览仅改来源并检查，自动安装开关独立；退出预览不自动降级，OpenRC/轻量 Node 边界沿用 `docs/release-channels.md`。
- 生产部署安全核对：不适用（预览版禁止生产部署）。`prod-108` 禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。候选/公开镜像容器均非生产证据。
- 回滚源码/tag 和镜像见首段；数据/配置备份及实际生产回退不适用。手动回退须先备份并检查兼容，不重写公开标签；稳定默认更新入口维持 v1.23.0。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-01T01:20:51+08:00
- 候选冻结时间：2026-10-01T07:47:07+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成生产退化、回滚、热修复或重复发布；预览不计入正式发布/部署频率。以下异常均发生在生产写前，首轮 L3 预检失败与证据复核命令错误单独计流程异常。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：5
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/arena-154/ssh-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "登记环境 SSH 超时，L3 改用用户明确选择的 local-wsl-dr；真实隔离主机及 browser-validation 仍未验证。",
    "recoveryEvidence": "用户选择灾备的回复及 r2 status=passed/exit_code=0；模拟 IAB UI 单独记录，不冒充登记实机通过。",
    "permanentAction": "延续登记环境上游可达性期限例外；由当前发布任务于 2026-10-03 前复核，退出条件为 arena-154 SSH 恢复且在登记环境完成相同公开镜像旅程。不得扩大灾备环境用途。",
    "historicalReleases": ["v1.22.0", "v1.23.0"]
  },
  {
    "fingerprint": "source-preparation/run-release-l3/stale-business-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1 业务事实过期门禁拒绝执行，需刷新 canonical review 并重冻候选。",
    "recoveryEvidence": "r1 source-prepare.json，5c97ea1f 事实刷新、227 项治理检查与 r2 passed 原件。",
    "permanentAction": "现有唯一入口正确拦截，未放宽门禁；已以 5c97ea1f 修正业务事实。下一次发布在冻结前先复核 canonical review，不通过则更新事实后生成新 kit；本轮无同指纹最近五个稳定版记录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence-recovery/powershell/linux-path-mapping",
    "position": "before-production-write",
    "count": 2,
    "impact": "补充 SHA-256 复核先错误拼接 Linux 绝对路径，再错误假设 bundle 与日志同目录，两次命令拒绝；L3 原始证据本身未变。",
    "recoveryEvidence": "按文件 basename 分别映射 artifact 根目录与 wsl-evidence 后，12 项校验全部通过。",
    "permanentAction": "复核使用 evidence.sha256 原摘要，显式区分 kit 与日志目录；不把路径错误当摘要错误，不改写原件或重新跑成功 L3。后续由发布复核者沿用该映射，唯一入口的原件回收校验继续保留。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-cleanup/wsl/bash-argument-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "可再生工作目录清理的 Bash 参数在 Windows 调用中解析失败，未执行删除；不影响已通过 L3 或原件。",
    "recoveryEvidence": "改用固定绝对路径的 WSL 原生命令，删除前重新核对 realpath、运行容器及目录用量；成功清理结果见资源收尾。",
    "permanentAction": "本机资源处理用固定路径的原生命令参数，不串接带命令替换的跨 Shell 清理脚本；唯一原件和活跃预览不在清理目标。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与分支收尾

- CF scoped、真实双方配对升级与权限撤销/重启、宿主 chmod/Passkey、原生浏览器 125%/200% 缩放、键盘/PWA 和长时间负载留待稳定版前。已完成自动门禁及模拟旅程足以支持本次预览产物发布，不代表实机或生产准入完成。
- 产品候选与两条来源分支保留；来源本地 tracking 分支与远端精确 tip 对齐，产品候选 upstream 为 `origin/release/v1.24.0-candidate`。纯验收与公开业务事实同步另走独立 `docs/release-v1.24.0-rc.2-acceptance` 候选 CI → 主线 CI → `archive/docs/release-v1.24.0-rc.2-acceptance` 精确 tip 归档，避免改动公开产品 SHA 和预览身份。验收提交的 CI、归档 ref/SHA、远端与本地处置读回另保存在 `C:/GitHub/_release-evidence/v1.24.0-rc.2-closeout.json`；不把产品 CI 当作验收提交 CI。
- 本机候选工作树与 node_modules 供 ready 模拟预览保留；历史/未知工作树、唯一 bundle 和失败/成功原始证据保留。已回收自己生成的 `/root/kpanel-release-work/v1.24.0-rc.2-5c97ea1f-l3-r2`，删除前核对 realpath、无活跃 Runner、成功终态及本机回收摘要，逻辑释放 442922036 字节；未压缩 WSL VHD，不能等同 C 盘物理释放量。原始远端 evidence、离线 kit 和本机证据仍可恢复精确候选。
