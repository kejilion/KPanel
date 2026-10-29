# KPanel v1.28.0 稳定版发布验收记录

日期：2026-10-30（Asia/Shanghai）

发布级别：L3；releaseChannel=stable；releaseTrain=1.28.0

候选提交 / 标签：6613318a78fa19135de691372cd94fbce0e70edf / v1.28.0

上一稳定版本 / 回滚点：v1.27.2 / sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10

**稳定产物已发布，生产环境未部署。** [v1.28.0 GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.28.0) 于 2026-10-29T17:03:04Z 公开，ID 428155058，draft=false、prerelease=false，Latest=v1.28.0。正式发布工作流 [40913307488](https://github.com/kejilion/KPanel/actions/runs/40913307488) attempt 1 成功。本文为发布后的验收记录，不改变稳定 tag、镜像或 Release 产物。

精确提交：6613318a78fa19135de691372cd94fbce0e70edf；tree：4549db757960023208b18575b9d4efc32eb7aac7；annotated tag object：e1246810fa12cd38c65f7839962f869bc53f0869。稳定发布时 main 与 tag 指向同一提交。

## 候选分支与发布后处置

- 原候选分支 / 精确 tip / 处置：release/v1.28.0-candidate / 6613318a78fa19135de691372cd94fbce0e70edf / 稳定版归档。
- stable-completion job 123602924430 成功；远端 archive/release/v1.28.0-candidate 回读为上述 SHA，远端 release/v1.28.0-candidate 已不存在。
- 本地候选 worktree C:\GitHub\_release\kpanel-v128 和分支保留在精确候选 SHA，工作区干净且未设置 upstream，用于保留发布证据；未删除来源工作树或其他任务分支。
- 发布后文档提交不属于 v1.28.0 产品 tag。无未完成远端归档项。

## 发布范围与兼容性

- 精确源码范围：v1.27.2..6613318a78fa19135de691372cd94fbce0e70edf；稳定发布使用冻结候选，没有在发布后改写候选代码或重建产物。
- 公开更新包括虚拟机管理、网站流量与防护监控、可选桌面浏览器组件、Windows 轻量节点、桌面快捷方式复制与自定义网站图标，以及跨节点文件传输、桌面恢复、终端滚动和集群远程访问管理改进。
- 安全相关更新包括文件归档与恢复、节点配对撤销、跨节点传输、登录和资源限额边界，以及文件管理器复制、跨文件系统移动和回收的私有暂存与目录身份校验。
- Panel 与 Agent 应配套更新；Windows 节点是轻量节点，不提供远程桌面；安装前应核验固定版本的官方附件及 SHA256SUMS。升级前备份 Panel 和 Agent 数据并检查在途任务。
- 旧 Cloudflare 凭据不会自动迁移，升级后需在面板重新录入。虚拟机功能依赖可用的 KVM、libvirt 网络和存储。自签名 TLS 端口仍接受明文 HTTP API，应限制在可信网络并强制 HTTPS。OneDrive 真实账户往返未实测。
- 本列车没有为稳定发布另加数据库 schema 变更；公开升级说明仍要求按常规流程备份和回滚数据，不把隔离验收写成真实宿主机迁移证明。

## 内置安全审查与审计范围

- 本次使用 KPanel 自带发布门禁：精确候选 CI 和主线 CI 的代码、Go 调用路径、Node 依赖、源码/依赖/秘密/配置扫描及 kejilion.sh 生命周期检查全部成功。CI run 40911258883 和 40912272564 均绑定 6613318a78fa19135de691372cd94fbce0e70edf。
- 针对文件管理器边界的内置安全盲审覆盖 internal/filemanager 的管理器、归档、接收、传输、Linux 路径、权限、暂存、共享文件及 Office 源码；报告为“未发现可信的安全缺陷”，H0/M0/L0，confirmed=0、fixed=0、deferred=0。未审范围：其余辅助文件与测试、HTTP/RPC 鉴权调用链及前端。报告 SHA-256：7A7EF63607E8DD36970BBB34BD0A78678E017C12269229AA9F8DAF7882108CEA；本地证据位于 C:\GitHub\_release-evidence\kpanel-v128-stable-quality\security-v1.28.0-candidate-e6240307\review-report.txt。
- 按上一稳定 tag v1.27.2 运行 node scripts/security-review-scope.mjs --base v1.27.2，输出 security_review=recorded groups=68 invalid_trailer=true：68 个变更包组均为 recorded，无 missing/stale；invalid_trailer 来自该历史范围中旧式或缺少 scope 的审查 trailer，不是本次 internal/filemanager 审查 trailer。该工具明确为 advisory；保留告警，不改写历史，也不把它解释为全仓独立审计通过。
- 本候选 OCR 记录：OCR-Review 1.12.11，范围 ac65c30..5a951b1，8/8 可评审文件，free-form=0、H0/M0/L0、constrained-only=0。它只记录本次观察，不据此宣称 OCR 工具有效或退出。
- 按用户明确要求，未执行 Cloudflare/CF security-boundary-audit。内置 CI 与本地范围审查不等同于 CF 审计或全仓无漏洞保证；既有待办和未审范围仍按原记录保留。
- 文件管理器候选曾触发 Windows 普通复制目标 DACL 继承回归，作为产品缺陷由 Windows 门禁拦截；commit c51854e511996ddb2e25b5111a7be93fa771cae8 修复后，精确候选 Windows 检查通过。该产品缺陷与下方发布流程异常分开计数。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选、主线 CI 与 L3 通过；发布后浏览器及升级旅程通过。未覆盖全部外部部署组合。 |
| 网络入侵与供应链安全 | 已验证 | KPanel 内置扫描、范围审查、公开附件与双架构镜像身份核对通过；CF 审计及全仓独立盲审未执行。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 隔离容器重启、受控恢复和浏览器更新旅程通过；生产宿主机升级及回滚未实演。 |
| 性能与资源预算 | 已实现未实机验证 | 发布门禁及有界流程通过；未做真实生产负载、低配主机和长时 soak 验收。 |
| 用户体验与可访问性 | 已验证 | 公开浏览器报告通过；读屏、实体触控和所有原生缩放档位未专项验证。 |
| 数据、配置与迁移 | 已验证 | 稳定发布未追加 schema 迁移；旧 Cloudflare 凭据需手动重新录入，升级和回滚仍须配套备份。 |

## 自动门禁与依赖

- 候选 CI：run 40911258883，精确候选成功；verify、特权核心竞争检查、Go 调用路径扫描、Node audit、Trivy 扫描、脚本生命周期和 Windows native boundaries 均通过。
- 主线 CI：run 40912272564，绑定同一 SHA，全部必需检查成功。依赖 freshness run 40912272453 成功。
- L3：release workflow 40913307488，L3 job 123586771313 成功；GitHub-hosted Ubuntu 24 image 20261026.354.1，Go 1.27.3、Node 24.22.1。不可变 Runner ID 未记录。权威回执状态 passed，候选 SHA/tree 精确匹配，web/go/deploy 三 lane 均退出 0。入口 SHA-256：43e57f4fd321846204020d6cf27eb75961f0aefa1284a9187e786601704c2d40；web/go/deploy 日志 SHA-256 分别为 b2f9b80f5548673b5c0e2f6f976677bc05d3e9a9ec5d34946056ba5d962d7092、713e55356c62fa8cca0f0430b64626a9a9ad1a70fb6077e010e5bd54fd0393b4、a7d1b37b9c9bd6b45338b59a4642416c0a563b6b0f0bee119555c39b8c735748。回执没有披露单独的 plan、script、bundle SHA-256，故不补写推测值。
- Release：job 123593106608 成功；Windows node job 123586770627、public-acceptance job 123601850510 和 stable-completion job 123602924430 成功。preview-completion 按稳定版流程跳过。
- Release notes 最终版本为 1.28.0，公开摘要 6 条、升级提示 3 条。Go/Node 工具链版本如上；本轮依赖新鲜度检查通过。
- L3 与公开验收原始证据目录：C:\GitHub\_release-evidence\kpanel-v128-stable-quality\release-run-40913307488。

## 隔离浏览器与公开产物

- 发布后隔离验收只使用 GitHub-hosted runner、Panel/Agent 容器和真实浏览器；没有连接生产主机。初始阶段 48 项、重启阶段 26 项、公开浏览器旅程 47 项、升级旅程 35 项均 passed，errors=0、assetFailures=0；报告标记 mocked=false。Docker inventory 与测试 CA 使用明确标记的测试夹具。
- 发布接口和更新弹窗返回 stable 1.28.0 与预期 digest；测试覆盖稳定版信息、重启后状态、终端历史、偏好与桌面恢复。此证据不是生产安装、真实宿主 Docker/systemd 或真实 CA 验收。
- GitHub Release 为非 draft、非 prerelease 且 Latest；共 19 个附件，包含 SHA256SUMS（15 项摘要）、LICENSE、THIRD_PARTY_NOTICES.md、双平台 Agent/Node、Windows 安装器、MCP 二进制和元数据归档。面板元数据 tarball 不是可构建源码包，Release 不提供 paneld 原生部署二进制。
- Docker Hub 标签 1.28.0 和 latest 均为 sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4；linux/amd64 为 sha256:5e66f8f001db852e10f2609b491a3a7afef3567dada8f300e8ce490cb208d649，linux/arm64 为 sha256:5e9fb0881c86a6dda3fb1ed60e7643d0b36e3ea5472f3e70dfca824bac132fe1。公开 Release、镜像 digest 与 L3 候选身份一致。
- Release 身份快照 SHA-256：3159800c6bf10e11358cbec66980f04ce06365dfa8ef2230df009bb61938fb21；附件 manifest SHA-256：9c0be98b36c67844796a7ebf06fb03781fa23795bc265e526bd73806e332a077；公开 Release digest：sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4。
- scriptLinkageState=not-required；本次稳定发布没有新脚本候选或跨仓库变更集。应用市场/kejilion.sh 生命周期由内置 CI 检查通过。

## 生产部署与回滚

- 生产部署：不适用；未连接、备份、部署、升级或核对任何生产主机，未连接 prod-108。隔离真机和浏览器场景不能替代生产证据。
- 回滚点：v1.27.2 / sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10。若需从 1.28.0 回滚，应恢复相应 Panel/Agent 镜像和配套数据备份；本次未执行生产回滚，生产健康状态不适用。
- 未在本次任务中执行本地磁盘清理；源码、发布附件、验收报告及 L3/浏览器原始证据保留在上述路径。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-29T23:48:02+08:00
- 候选冻结时间：2026-10-30T00:24:14+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（稳定版公开一次，未回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/release-metrics/missing-rc12-acceptance-fields",
    "position": "before-production-write",
    "count": 1,
    "impact": "run 40906485080 的发布指标校验拒绝了缺少机器指标区块的 RC12 验收记录；该候选未晋级。",
    "recoveryEvidence": "commit e62403071fb896f6bdb7db869a163c370ca482f0 补齐 RC12 指标；最终精确候选 run 40911258883、主线 run 40912272564 和发布 run 40913307488 均成功。",
    "permanentAction": "补齐正式历史验收记录，保留 report-release-metrics 作为候选与发布验证门禁；未绕过或放宽校验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/source-inventory/stale-first-party-hash",
    "position": "before-production-write",
    "count": 1,
    "impact": "run 40910122349 的源码检查发现 first-party source hash 与候选实际文件集合不一致，未接受旧摘要作为通过证据。",
    "recoveryEvidence": "按最终六平台 canonical inventory 重新绑定身份，最终 firstPartySourceSha256=f90b5b04a0e14bdb2a070fc17e70309c484ca283fea61a7c95d8508c61f50430；source-checks 21/21 通过，精确候选 CI 40911258883 和主线 CI 40912272564 成功。",
    "permanentAction": "从精确候选重算六平台 canonical inventory 与 source identity，并维持 source-checks 和 CI 的摘要一致性检查；不复用旧候选摘要。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

上述两次均发生于稳定公开前，没有生产写操作；它们是候选门禁失败，已按流程异常明细分别记录，不属于已发布变更失败。run 40906485080 同时发现 Windows 普通复制的目标 DACL 继承缺陷；该项属于产品缺陷，由正常 Windows 门禁拦截并在 c51854e511996ddb2e25b5111a7be93fa771cae8 修复，不计入流程异常次数。最近五份稳定验收记录中未发现上述两个精确流程指纹重复。公开发布首次 attempt 即成功，没有重跑发布、调用恢复入口或覆盖既有 Release。
