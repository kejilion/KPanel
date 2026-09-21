# KPanel v1.21.0-rc.7 发布验收记录

日期：2026-09-21

发布级别：L3

候选提交 / 标签：`4ab8fdf6439bbb21a50044661ab14a43be1ecb30` / `v1.21.0-rc.7`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端与本地均精确指向 `4ab8fdf6439bbb21a50044661ab14a43be1ecb30`

- 原分支 / 精确 tip / 处置分类：`feature/mcp-host-bulk-select` / `76e754a1919ef748ba37150a6ebef4d66be47ae7` 已纳入并归档；`feature/local-wsl-dr-release-channel` / `2f6fb8b8fb093007e9750c853dfd164f3c690de4` 已进入主线、归档并删除远端活动引用。
- 归档 ref 与 SHA：`archive/feature/mcp-host-bulk-select-20260921` = `76e754a1...`；`archive/feature/local-wsl-dr-release-channel` = `2f6fb8b8...`；`archive/assemble/v1.21.0-rc.7` = `4ab8fdf6...`。
- 首轮缺少最终灾备治理的组装提交单独保留为 `archive/assemble/v1.21.0-rc.7-pre-wsl-20260921` = `e59443de...`，明确标记为未发布，不与最终 tag 混用。
- 三个来源 Git worktree 注册和本地活动分支已回收为 archive 分支；MCP 来源路径因 Windows 忽略缓存未完成物理目录删除，但不再是 Git worktree。验收记录工作树在提交前保留。
- `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 及本轮之外的新任务工作树保持原样，未纳入、未删除；`docs/security-audit-run2-local-20260919` 继续保持未发布。
- 未完成归档项：无。候选分支按预览列车规则保留，等待稳定版成功后由唯一归档入口处置。

归档不代表生产上线；本版仅发布预览产物，生产未部署。

## 发布画像

- 业务域：MCP 客户端授权主机选择、发布验证与责任移交。
- 变更面：MCP 管理界面的主机全选/反选；环境策略、固定 Linux Runner、WSL 传输、独立 handoff kit 与证据回收。没有新增 MCP 权限、API、数据迁移、受管脚本协议或生产部署目标。
- 受影响用户旅程：管理员创建 MCP 客户端时批量选择授权主机；发布负责人在 `arena-154` 不可达时显式选择本地 WSL 完成同一 L3，或把冻结 kit 与独立 manifest 摘要移交给登记接收端。
- 未变化契约：选择只在创建客户端时按现有权限边界一次性保存；默认 L3 与唯一正式部署目标仍为 `arena-154`，`local-wsl-dr` 只允许 `candidate-validation`，`108`/`prod-108` 禁止全部 KPanel 操作。
- 风险等级及理由：L3。用户功能很小，但发布治理涉及执行身份、Docker socket、代理、Runner 归档和跨控制端证据链，必须以完整 L3、候选/main/tag 门禁、公开 OCI 和真实移交演练收敛。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.7]`：MCP 客户端授权主机支持一键全选和反选，空列表禁用批量操作。
- 精确产品提交为 `4c6f84ff`；版本提交 `61bdcc82`、候选复核 `7f650178`、业务事实与移交证据刷新 `4ab8fdf6`。治理基线为主线中的 `2f6fb8b8`。
- 明确未纳入：生产部署、自动回退到 WSL、WSL 生产用途、MCP 新权限或写入语义、长期浏览器 soak，以及本轮之外的本地工作树。

## 外部审计与修复交付

- 本 RC 没有新增可引用的 Cloudflare `security-audit-skill` 最终结构化结论；Alibaba open-code-review 对产品候选记录 `OCR-Review: 0.3.4 ... valid=H0/M0/L0`，治理证据文档按无代码规则记录 skipped。
- 两项外部能力继续作为辅助证据，不替代项目测试、独立治理复核、固定 L3、安全扫描、候选/main/tag CI 和公开镜像验收；中止、skipped 或 constrained-only 均不改写为通过。
- WSL 通道的修复闭环覆盖 Runner ID、归档摘要、manifest/plan 身份、路径规范化、证据摘要、临时代理与嵌套 Trivy；治理 SHA 和最终 rc.7 SHA 分别通过真实 L3。
- 稳定周期观察仍按 `PROJECT_RULES.md` 5.4/5.5 延续，不根据单个 RC 的零发现提前判断工具长期有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：产品只改变设置前端；治理只改变发布入口和验证环境，不修改 `kejilion.sh` 内容或调用契约。L3 managed-script-contract 与应用生命周期通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | MCP 定向测试 6/6；完整 Go 与 Web 174 文件 / 1530 项通过；主机选择变化不提前提交。 | 没有后端协议变化。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/产品镜像 0 阻断项；Runner 归档和 manifest 绑定 SHA-256。 | 3 个模块通告不可达，继续跟踪。 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、应用生命周期、固定 WSL L3、真实 handoff、候选/main/tag 门禁与公开 OCI E2E 通过。 | 未做长期浏览器 soak。 |
| 性能与资源预算 | 已验证 | 产品只增加本地数组选择操作；发布通道复用同一门禁且代理不持久化。 | WSL 长期并发发布容量未压测。 |
| 用户体验与可访问性 | 已验证 | 全选、反选、空列表禁用及已选计数由组件测试覆盖，沿用现有设置布局与键盘语义。 | 未另做最终 SHA 多浏览器缩放人工矩阵。 |
| 数据、配置与迁移 | 不适用 | 没有 schema、配置格式或持久数据变化；发布 kit 不含凭据。 | 不适用。 |

## 自动门禁

- 本地最终候选：业务新鲜度 baseline `7f650178`、治理 208/208、17 个提案、11 个依赖组；完整 Go、Web 174 文件 / 1530 项、typecheck、3296 个短语、生产构建和安装安全检查通过。
- L3 Runner：`kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；完整 Go/Web、核心 race、govulncheck、npm audit、Trivy、双架构、镜像和 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：`local-wsl-dr-v1.21.0-rc.7-4ab8fdf-r2`，2026-09-21 11:21:06 至 11:28:07 +08:00，exit 0；bundle `04573fe2...`、plan `851535c8...`、remote entry `21c08b11...`、L3 日志 `c669fbac...`。证据位于 `C:/GitHub/_release-evidence/local-wsl-dr-v1.21.0-rc.7-4ab8fdf-r2`。
- 候选 CI：[CI 35557633803](https://github.com/kejilion/KPanel/actions/runs/35557633803) 与 [freshness 35557633745](https://github.com/kejilion/KPanel/actions/runs/35557633745) 成功。
- 主线 CI：[CI 35558003047](https://github.com/kejilion/KPanel/actions/runs/35558003047) 与 [freshness 35558003098](https://github.com/kejilion/KPanel/actions/runs/35558003098) 成功。
- annotated tag object `e3b588f888a7deb3fc0689e592fd216e33462492` 指向产品提交；[Release 35557938084](https://github.com/kejilion/KPanel/actions/runs/35557938084) 与 [tag freshness 35557938075](https://github.com/kejilion/KPanel/actions/runs/35557938075) 成功。

## 依赖与技术栈变化

- dependency policy validate-only、候选/main/tag freshness 均通过；本版没有产品依赖、Action、基础镜像或受管脚本升级。
- Go 1.26.7、Node 24.20.0、Trivy 0.72.0 沿用冻结工具链；新增固定 Runner 构建源与受信归档是发布基础设施，不进入产品镜像。
- `packaging/release-runner/Dockerfile` 的 Docker socket root 等价例外只限该文件，至 2026-12-21 到期并有退出条件；产品 Dockerfile 扫描保持 0 误配置。

## 隔离真机与浏览器验收

- 主机：本机 `Ubuntu` WSL2、Linux/amd64、Docker；环境策略只允许 `local-wsl-dr` 执行 `candidate-validation`。
- 公开 OCI 使用不可变摘要 `sha256:feaa30df0b999a8e9ace7294f936b3c1ecedfa04de12fb531affe2b90c259562`，镜像 revision=`4ab8fdf6...`、version=`1.21.0-rc.7`、User=`65532:65532`。
- 在临时端口 `18187` 执行固定 `packaging/tests/image-e2e.sh`；`image_e2e=pass`、`public_oci_e2e=pass`，临时容器、网络和 WSL 临时目录已清理。
- 证据：源码 archive `9533e2fd...`、执行脚本 `89609164...`、日志 `cd9fc9df...`，位于 `C:/GitHub/_release-evidence/v1.21.0-rc.7-public-image-e2e-local-wsl-r1`。
- 首轮独立移交演练另以治理 SHA `2f6fb8b8...`、manifest SHA-256 `3a6f4ecc...` 输出 `release_l3_handoff=pass`；证明接收方无需原生成目录即可核对并执行 kit。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.7](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.7) 为非 draft、prerelease、非 Latest；发布时间 2026-09-21 11:45:13 +08:00，GitHub Latest 仍为 `v1.20.0`。
- 14 个附件均为 uploaded 且非空；`SHA256SUMS` 自身摘要 `908ddb6f...` 与 GitHub API digest 一致，包含 11 个二进制/部署包条目。
- Docker `1.21.0-rc.7` 与 `preview` OCI index 均为 `sha256:feaa30df0b999a8e9ace7294f936b3c1ecedfa04de12fb531affe2b90c259562`。
- `linux/amd64` / `linux/arm64` 分别为 `sha256:1fe004dacefc75722a311c1f75bcf4bc72417436b98c7eab6d3d6d25b513fe7b` / `sha256:d0faa1cf0e6b4b35dcb8bfd9865c7501200ac53a911799dcb4f7c706b5db31e6`；另外两项为 attestations。
- 稳定 `latest` 与 `1.20.0` 均保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。

## 自更新通道验收

- 稳定来源继续只接受正式 GitHub Latest；预览来源接受同列车 RC，并可在没有更新 RC 时接受更高稳定版本。
- 加入预览只切换来源并立即检查，不自动安装；自动安装和一次性立即安装相互独立。
- rc.7 发布后 `preview` 可继续接收后续 rc.8 或更高稳定版；退出预览不会产生降级候选。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证环境：`local-wsl-dr` 的完整 L3、handoff 演练和公开 OCI E2E。
- 正式部署环境：不适用；`arena-154` 本轮未执行生产写入。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：0。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c...`；上一预览 `v1.21.0-rc.6` / `d73e9c78...`。
- 镜像 digest：稳定 `sha256:a991b5d2...`；上一预览 `sha256:0d4cee03...`。
- 数据/配置备份及生产回滚：不适用，未部署生产。
- 预览回退时重新固定 rc.6 digest；GitHub Latest、Docker `latest` 和稳定更新入口继续指向 `v1.20.0`。
- 发布治理回滚点：`bae0e533...`；如 WSL 通道的 Runner 身份、用途或证据绑定失败，回退治理提交并只使用 `arena-154`。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-21T11:13:16+08:00
- 候选冻结时间：2026-09-21T11:28:07+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：5
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-validation/arena-154/unreachable-before-upload",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮 rc.7 L3 在上传候选前因 arena-154 SSH、ICMP 和服务端口持续超时而停止；候选代码未在目标执行。",
    "recoveryEvidence": "保留不可达观察并实现策略化 local-wsl-dr；治理 SHA 与最终 4ab8fdf6 SHA 均在固定 Runner 上完成 L3。",
    "permanentAction": "默认仍使用 arena-154；需要灾备时显式选择登记的 WSL，禁止自动静默回退。",
    "historicalReleases": []
  },
  {
    "fingerprint": "wsl-dr/bootstrap/transient-proxy-gaps",
    "position": "before-production-write",
    "count": 2,
    "impact": "灾备通道首轮治理演练分别暴露 Go 下载和嵌套 Trivy 没有继承主机代理；两次均在最终候选执行前停止，失败证据独立保留。",
    "recoveryEvidence": "代理只以运行时变量、host network 和 BuildKit secret 转发，不写入 kit、日志或镜像；治理 handoff r2 与 rc.7 L3 r2 完整通过。",
    "permanentAction": "代理传递由脚本回归覆盖；任何缺失、泄漏或扫描失败继续关闭失败，run ID 不复用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-preflight/business-context/stale-threshold",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮最终候选 L3 在准备阶段因业务事实基线达到 51 个提交而关闭失败；候选门禁未执行。",
    "recoveryEvidence": "刷新到 7f650178 精确基线，区分 rc.5/rc.6 已发布事实、rc.7 候选和 WSL 验证能力；freshness 以 commits=1 通过后使用新 run ID 完成 L3。",
    "permanentAction": "继续保留 50 提交阈值；发布准备发现 stale 时先刷新规范，不以参数绕过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "main-ci/governance-sha/race-step-failure",
    "position": "before-production-write",
    "count": 1,
    "impact": "治理 SHA 2f6fb8b8 的 main CI 在 race 步骤失败；同 SHA 候选 CI 已成功，具体失败未被改写为产品通过。",
    "recoveryEvidence": "最终候选包含完整治理后，本地 L3、候选 CI 和 main CI 35558003047 的 race 均成功，再继续标签发布。",
    "permanentAction": "保留候选与 main 双层 race；治理中间 SHA 失败不能由候选结果替代，最终发布必须具备同 SHA 成功证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：三个来源 Git worktree 注册和活动分支已回收；MCP 来源路径只剩可再生成的忽略缓存。L3、handoff 与公开 OCI 证据保留在 `C:/GitHub/_release-evidence/`。
- 未验证风险：最终人工多浏览器缩放矩阵、长期设置页交互 soak、WSL 长期并发容量，以及把生产发布责任移交给另一台已登记控制主机的完整演练。
- 已实现待稳定版准入：以 rc.7 源码组装 `v1.21.0`，重新执行精确稳定 SHA 的 L3、候选/main/tag 门禁、公开 OCI、`arena-154` preflight/backup/标准更新/postdeploy，再归档活动候选分支。
- 不阻断本版的理由：本版只进入预览渠道；同一源码已通过固定 WSL L3、候选/main/tag 门禁、14 个附件、公开双架构 OCI 与本地 WSL 公开镜像 E2E，稳定入口和生产均未改变。
- 后续应进入的自动门禁或专项工作流：定期演练独立 manifest 交接、到期复核 Runner Dockerfile 例外，并在稳定版验收记录中确认候选分支已归档。
