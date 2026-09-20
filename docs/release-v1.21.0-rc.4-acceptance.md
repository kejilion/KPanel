# KPanel v1.21.0-rc.4 发布验收记录

日期：2026-09-20

发布级别：L3

候选提交 / 标签：`0c56641853471ce4e333382bec52f580c4a517af` / `v1.21.0-rc.4`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `0c56641853471ce4e333382bec52f580c4a517af`

- 原分支 / 精确 tip / 处置分类：`feature/virus-scan-system-center` / `3319d3beec7712a357c545d4e0b0c2a0445b29be` / 已纳入并归档；`feature/system-package-manager` / `ebb217108b91aef60bb4035852fbbce4a16bf482` / 已纳入并归档；`integrate/kpanel-virus-scan-protocol-20260920` / `2b90b2d2ca56bc954c9328a51bb5571e896f713d` / 跨仓库脚本已发布并归档。
- 归档 ref 与 SHA：`archive/feature/virus-scan-system-center-20260920` = `3319d3be...`；`archive/feature/system-package-manager-20260920` = `ebb21710...`；`archive/assemble/v1.21.0-rc.4` = `0c566418...`；脚本仓库 `archive/feature/kpanel-virus-scan-protocol` = `90829558...`、`archive/integrate/kpanel-virus-scan-protocol-20260920` = `2b90b2d2...`。远端逐 ref 复核一致。
- 本次来源任务分支均已映射到冻结候选；KPanel 与脚本仓库对应本地活动分支已删除。两个脚本来源 worktree 已释放；KPanel 两个来源 worktree 已从 Git worktree 注册中释放。
- `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 含未提交内容，按规范原样保留且未纳入。`docs/security-audit-run2-local-20260919` 含受限本地材料，保持未发布、未改动。
- 未完成归档项：无。两个已释放的 KPanel worktree 路径仍有 `.vite`、`node_modules` 或可由归档提交重建的 `web` 文件残留，未构成 Git 分支或工作树；下次本地缓存维护时清理，不影响恢复证据。

归档不代表生产上线；本版仅发布预览产物，生产未部署。

## 发布画像

- 业务域：系统中心病毒扫描、系统常用软件包管理、受管脚本供应链。
- 变更面：增加全盘、重点目录和最多 8 个自定义绝对路径的 ClamAV 扫描；增加 APT、DNF、DNF5、YUM、APK、Pacman、Zypper 的固定工具查询与安装入口；更新受管 `kejilion.sh`。
- 受影响用户旅程：管理员查看病毒库与扫描报告、启动只读病毒扫描；查看 17 个固定工具的真实安装状态并安装缺失项。
- 未变化契约：扫描不会自动删除或隔离文件；稳定 GitHub Latest、Docker `latest`、生产数据和生产部署均未改变。
- 风险等级及理由：L3。新增宿主机路径读取、容器化病毒库更新、持久任务与包管理器写入，采用路径失败关闭、固定工具 ID、完整 L3、真实 ClamAV、跨仓库脚本契约和公开 OCI E2E 约束风险。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.4]`：系统中心病毒扫描、固定系统工具管理、路径校验和 ClamAV 首次运行修复。
- 精确提交清单：`04ba3c42`、`b6921e01`、`0b8b767c`、`166a3b75`、`e7b4b1d9`、`22519950`、`0c566418`。
- 明确未纳入：任意软件包名输入、自动删除/隔离感染文件、生产部署、所有发行版上的真实包安装、长期扫描 soak 和 EICAR 感染样本处置。

## 外部审计与修复交付

- Alibaba open-code-review 1.12.6 trailer 覆盖病毒扫描路径 21/21、预览与失败态 22/22、软件包管理 28/28，均为 `H0/M0/L0`；它只作为辅助证据，不替代项目门禁。
- Cloudflare `security-audit-skill` 本轮没有新增可引用的最终结构化结论；不把此前中止或不完整的专项运行改写为通过。本版安全结论来自路径失败关闭、固定操作集合、单元/契约测试、L3、Trivy、govulncheck、npm audit、真实 ClamAV 与公开镜像验收。
- 修复交付：ClamAV 镜像初始化、权限降级、日志 tmpfs、数据库卷 copy-up 和 strict-shell 问题在脚本提交 `2b90b2d2...` 修复；脚本先发布，KPanel RC 再绑定其 revision/SHA。
- 本稳定周期尚不足以单凭少量候选判断外部审计工具长期有效性或退出条件；继续按 `PROJECT_RULES.md` 5.4/5.5 观察。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`。
- 变更集编号：`kpanel-virus-scan-protocol-20260920`。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 脚本候选 commit / SHA-256：与上述基线一致；脚本仓库 `main` 已先行发布并由不可变 raw URL 复核。
- 状态判定依据与兼容性证据：根脚本和 CN 镜像 `bash -n`、同步检查、主菜单 smoke、病毒扫描 smoke、真实 ClamAV 1.5.4 干净文件扫描、KPanel managed-script-contract、L3 与公开镜像提取均通过。
- 本版发布决定：脚本先行后发布 KPanel；KPanel Dockerfile 固定 revision 和 SHA-256。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Panel/Agent/systemmanage 契约、Web 组件与完整 Go/Web 测试通过；真实 ClamAV 干净文件扫描成功。 | 未用 EICAR 验证感染报告；未在所有包管理器发行版真实安装。 |
| 网络入侵与供应链安全 | 已验证 | 路径失败关闭、最多 8 个路径、只读挂载、无自动删除；脚本 revision/SHA 固定；govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/镜像 0 阻断项。 | `clamav/clamav-debian:latest` 仍是运行期浮动上游镜像。 |
| 稳定性、失败恢复与兼容 | 已验证 | 首次病毒库更新、离线扫描、报告上限、持久任务、核心 race 和安装/更新/回滚/卸载生命周期通过。 | 无长期病毒库更新或大目录扫描 soak。 |
| 性能与资源预算 | 已验证 | 扫描按显式任务执行，无新增后台轮询；报告有界；自定义路径数受限。 | 未对超大目录扫描耗时和 ClamAV 峰值内存压测。 |
| 用户体验与可访问性 | 已验证 | Web typecheck、172 个文件 / 1466 项测试、3234 个短语 / 21 个目录和生产构建通过；成功、进度、失败和安全边界有明确文案。 | 未另做真实浏览器多缩放人工矩阵。 |
| 数据、配置与迁移 | 已验证 | 无 Panel schema 迁移；扫描报告和包状态走既有任务/接口边界；应用生命周期通过。 | ClamAV 病毒库首次下载依赖外网和上游可用性。 |

## 自动门禁

- 定向测试：病毒扫描路径/任务/报告、系统软件包管理器与对应 Web 组件全部通过；治理一致性 197/197 通过。
- L3 Runner：`kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；Go 全量、核心 race、Web 172/1466、typecheck、3234 短语、构建、govulncheck、npm audit、Trivy、双架构二进制、镜像和 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：`v1.21.0-rc.4-0c56641-l3-r1`，2026-09-20 14:52:51 至 15:09:37 +08:00，exit 0；bundle `ea3b6d67...`、plan `741d59a1...`、remote entry `d8bb2cf2...`、远端日志 `d11a357d...`。证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.4-0c56641-l3-r1` 和 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.4-0c56641-l3-r1`。
- 候选 CI：[CI 35496152801](https://github.com/kejilion/KPanel/actions/runs/35496152801) 与 [freshness 35496152800](https://github.com/kejilion/KPanel/actions/runs/35496152800) 成功。
- 主线 CI：[CI 35496539306](https://github.com/kejilion/KPanel/actions/runs/35496539306) 与 [freshness 35496539334](https://github.com/kejilion/KPanel/actions/runs/35496539334) 成功。
- annotated tag object `d11a99440b707234a0689a19ee4b0e6c4fd884cf` 指向产品提交；首次 [Release 35496921825](https://github.com/kejilion/KPanel/actions/runs/35496921825) 在 `Verify source` 瞬时失败并阻断在镜像推送前；原 SHA 的恢复运行 [Release 35497328699](https://github.com/kejilion/KPanel/actions/runs/35497328699) 与 [tag freshness 35497328711](https://github.com/kejilion/KPanel/actions/runs/35497328711) 成功。
- Release 生成 SBOM/provenance，原生镜像 Trivy 和运行时契约通过。

## 依赖与技术栈变化

- dependency policy validate-only、候选/main/tag freshness 均通过；最近安全通告作业无阻断项。
- Go 1.26.7、Node 24.20.0、Trivy 0.72.0 与固定 Action SHA 沿用项目冻结工具链。
- 本版新增的运行时外部依赖是 ClamAV 容器镜像；受管脚本代码固定为 `2b90b2d2...`，但 ClamAV 镜像标签仍浮动，稳定版前应评估固定 digest 和更新策略。
- 3 个 govulncheck 模块通告不可达；继续由每日审计跟踪，不在本 RC 擅自升级无关依赖。

## 隔离真机与浏览器验收

- 主机：`arena-154`，Linux/amd64、Docker；环境策略允许 candidate-validation，禁止 `prod-108`。
- 真实 ClamAV 证据：`arena-154:/root/kpanel-release-evidence/script-virus-scan-2b90b2d-r7`；Engine 1.5.4、病毒签名 3,628,071、1 个文件、感染 0，扫描前后文件 SHA 一致；`real-clamav.log` SHA-256 `4d2f9151...`。
- 公开 OCI 使用不可变摘要 `sha256:07b67e3b51d01017170d5ca056854e30688c5a09189cf98b8746d2733a47a899`，镜像 version/revision 精确匹配，并在临时端口 `18184` 执行项目标准 `image-e2e.sh`。
- 公开镜像证据：源码 archive SHA-256 `a6df27aa...`、执行脚本 SHA-256 `f0af95c3...`、日志 SHA-256 `234ad9bf...`，位于 `C:/GitHub/_release-evidence/v1.21.0-rc.4-public-image-e2e-r1`；输出 `image_e2e=pass`、`public_oci_e2e=pass`，临时容器、网络和远端目录已清理。
- 未执行长期 soak、EICAR 感染样本、全部包管理器发行版的真实安装和最终 SHA 人工浏览器多缩放矩阵；自动组件、失败态、L3 与公开镜像覆盖本 RC 准入。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.4](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.4) 为非 draft、prerelease、非 Latest；GitHub Latest 仍为 `v1.20.0`。
- 14 个附件均为 uploaded 且非空：MCP 六平台、Agent/Node 双架构、部署包、LICENSE、`SHA256SUMS` 和 THIRD_PARTY_NOTICES。
- Docker `1.21.0-rc.4` 与 `preview` OCI index 均为 `sha256:07b67e3b51d01017170d5ca056854e30688c5a09189cf98b8746d2733a47a899`；`linux/amd64` / `linux/arm64` 分别为 `sha256:1a036c23fa6202685d8aad2c1173b4c8b9ba41fbd7a3affce77a7bd1d6ada7e4` / `sha256:e8c316bb38892c14c97022349d05faee7261cd46b81730ed973303254f1a55d1`；另外两项为 attestations。
- 稳定 `latest` 与 `1.20.0` 均保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- `kejilion.sh` `main` 精确为 `2b90b2d2...`，公开 raw 摘要与 KPanel 镜像契约一致。

## 自更新通道验收

- 稳定来源只接受正式 GitHub Latest；预览来源接受同列车 RC，也会在无更新 RC 时接受更高稳定版本。现有更新选择测试通过。
- 加入预览只切换来源并立即检查，不自动安装；自动安装和一次性立即安装相互独立。
- 旧状态默认迁移到 `stable`，重启后通道保持；退出预览不会把较低稳定版作为降级候选。
- systemd 更新执行、更新前备份、失败恢复和失败版本隔离由 L3 生命周期覆盖；OpenRC 与轻量 Node 边界按 `docs/release-channels.md` 保持。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证环境：仅 `arena-154` 的隔离 L3、脚本验证和公开 OCI E2E。
- 正式部署环境：不适用。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 部署前后版本、备份、入口和生产数据：不适用。
- 生产已执行写操作：0。
- 仅在隔离真机执行：L3、真实 ClamAV 干净文件扫描、公开镜像 E2E。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c...`；上一预览 `v1.21.0-rc.3` / `f6f969b6...`。
- 镜像 digest：稳定 `sha256:a991b5d2...`；上一预览 `sha256:1f6fa3f2...`。
- 数据/配置备份及生产回滚：不适用，未部署生产。
- 需要回退预览通道时重新固定上一 RC digest；GitHub Latest、Docker `latest` 和稳定更新入口继续指向 `v1.20.0`。
- 公共默认更新通道决策：不适用，稳定通道未改变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-20T12:48:54+08:00
- 候选冻结时间：2026-09-20T14:51:03+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：是（产品未回滚；Release Verify source 瞬时失败后以同一源码和标签重跑）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-09-20T15:33:50+08:00；恢复时间：2026-09-20T15:46:18+08:00；逃逸门禁：未逃逸：首次 Release 在镜像推送和公开发布前阻断
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：10
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "script-validation/archive/line-ending-conversion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次脚本 archive 未禁用 core.autocrlf，远端 bash 语法检查失败；没有发布或远端业务写入。",
    "recoveryEvidence": "改用 core.autocrlf=false 的 LF archive 后，根/CN 语法、smoke 和真实 ClamAV 验证通过。",
    "permanentAction": "跨平台脚本 bundle 固定使用 core.autocrlf=false，并在上传前检查文件字节与 bash -n。",
    "historicalReleases": []
  },
  {
    "fingerprint": "script-validation/remote-shell/local-variable-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次真实 ClamAV 命令中的远端变量被本地 shell 提前解释，命令在创建资源前退出。",
    "recoveryEvidence": "改为上传固定脚本后执行，最终 r7 真实扫描与清理均通过。",
    "permanentAction": "复杂远端验证统一上传经摘要核对的脚本，不内联含远端变量的命令。",
    "historicalReleases": []
  },
  {
    "fingerprint": "governance-validation/local-command/obsolete-script-name",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次调用了仓库中不存在的旧治理脚本名，命令未启动，未形成错误通过结论。",
    "recoveryEvidence": "改用 check-governance-consistency、report-governance-health、check-release-acceptance-coverage 和 dependency policy 实际入口，全部通过。",
    "permanentAction": "治理检查只从 package.json、Makefile 或 scripts 目录解析当前入口，不凭历史命令名调用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "source-publication/github-transport/transient-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "脚本仓库首次 GitHub HTTPS 写入连接超时，SSH 备用身份无权限；没有部分 ref 更新。",
    "recoveryEvidence": "HTTPS 重试成功，并逐 ref 核对 main 与两个 archive 指向精确 SHA。",
    "permanentAction": "保留 HTTPS 精确 lease 和远端 ref 复核；连接超时只重试同一不可变 refspec。",
    "historicalReleases": []
  },
  {
    "fingerprint": "tag-release/verify-source/one-time-run-failure",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次标签 Release 的 Verify source 在已通过 L3、候选和 main CI 的同一 SHA 上退出 1，镜像推送和公开 Release 均被跳过。",
    "recoveryEvidence": "确认无 Release、无 Docker 写入后精确删除并重推同一 annotated tag；恢复运行 35497328699 的 Verify source 及全部后续步骤成功。",
    "permanentAction": "KPanel 维护者在下一稳定版复核；若未来两个版本再次发生，拆分 Verify source 子步骤并上传失败日志，退出条件为连续两个版本无复现。",
    "historicalReleases": []
  },
  {
    "fingerprint": "registry-inspection/local-runtime/docker-cli-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机没有 Docker CLI，首次镜像摘要查询未执行。",
    "recoveryEvidence": "在已批准且具备 Docker 的 arena-154 读取四个标签和平台清单，并完成公开镜像 E2E。",
    "permanentAction": "公开 OCI 清单和运行验收固定在 arena-154 执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "registry-inspection/ssh-loop/powershell-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 SSH 循环中的 $tag 被 PowerShell 提前展开，jq 未得到有效清单；没有远端写入。",
    "recoveryEvidence": "改用单引号封装远端脚本后，四个标签名称与摘要完整返回。",
    "permanentAction": "包含远端 shell 变量的 SSH 命令使用 literal 单引号或上传脚本。",
    "historicalReleases": []
  },
  {
    "fingerprint": "worktree-cleanup/ignored-cache/windows-delete-residual",
    "position": "before-production-write",
    "count": 2,
    "impact": "两个 clean 来源 worktree 因 .vite 或 node_modules 忽略缓存导致 Windows 目录删除未完全完成；Git worktree 注册和本地活动分支已安全释放。",
    "recoveryEvidence": "远端 archive ref 与精确 tip 已先复核；git worktree list 不再包含两个来源路径，活动分支列表为空，残留仅为可再生成文件。",
    "permanentAction": "下次释放 worktree 前先列出 ignored 缓存并使用项目缓存清理入口，再执行 git worktree remove。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：四条来源活动分支和两个脚本 worktree 已释放；KPanel 两个来源路径只剩可再生成缓存或可由归档提交重建的残留文件，未作为 Git worktree；L3、脚本和公开 OCI 证据保留在 `C:/GitHub/_release-evidence/` 与 `arena-154:/root/kpanel-release-evidence/`。
- 未验证风险：ClamAV 上游镜像浮动、感染样本处置、长时间大目录扫描、所有支持发行版的真实包安装、最终人工浏览器矩阵。
- 已实现待实机准入：软件包管理器在 APT/DNF/DNF5/YUM/APK/Pacman/Zypper 的真实安装覆盖应在稳定版前扩展；病毒扫描应补受控 EICAR 报告验证和资源预算。
- 不阻断本版的理由：本版为预览渠道；固定源码已通过 L3、候选/main/tag freshness、Release 安全链、真实 ClamAV 干净扫描、14 个公开附件和不可变公开 OCI E2E，稳定入口与生产均未改变。
- 后续应进入的自动门禁或专项工作流：固定或治理 ClamAV image digest；增加包管理器发行版矩阵和 EICAR 只读报告测试；继续观察外部审计有效性和 Release `Verify source` 瞬时失败。
