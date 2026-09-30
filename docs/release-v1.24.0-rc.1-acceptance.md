# KPanel v1.24.0-rc.1 发布验收记录

日期：2026-10-01。发布级别：L3。`releaseChannel=preview`；`releaseTrain=1.24.0`。

- 产品候选和标签：`40ab5a2f5be9c23d82bee1f8de96f4e138f8f974`；发布前远端 `main` 基线为 `00a032af954330acce3b47d9db6d2a5f93aebec7`（v1.23.0 验收提交）。候选 CI 通过且基线未移动后已快进主线，主线 CI 通过并三次核对一致后推送注释标签 `v1.24.0-rc.1`，tag object `f54915c32131792bbe9c89f1c2f763bbdb4fb871`。
- 上一稳定版及回滚点：`v1.23.0`，Docker `latest` index `sha256:236759215c0f527`（Docker Hub 回读，未改变）。上一不可变预览为 `v1.23.0-rc.8`。
- 新序列候选 `release/v1.24.0-candidate` 已创建并保留，供同序列后续 RC 使用；本轮仅公开预览产物，生产未部署。

## 发布画像与精确范围

- 业务域：集群公开分享主题；应用脚本、诊断、站点安装与 LDNMP 环境任务终端。
- 变更面：分享主题协议 `kpanel-share-theme@2` 及三套官方主题包、主题管理界面；终端可见性/背压/尺寸归属、共享滚动任务日志与环境任务保留。无数据库迁移、无新的安装配置、无脚本协议变化。
- 核心旅程：公开分享页按主题渲染、搜索、筛选、浅/深色；旧 `@1` 主题继续可用；多个应用脚本窗口并排实时输出、最小化后恢复不回放、多视图共享同一 PTY 时尺寸不互相争抢；超过旧 8 MiB 单文件上限后输出持续；旧 Agent 写入的单文件日志升级后可读；环境任务保留最近 50 条。
- 分享主题只经沙箱内 postMessage 接收公开快照，协议与沙箱边界沿用既有约束；日志分段沿用旧 `<id>.log` 命名并兼容旧日志。

| 来源分支 | 精确 tip | 纳入与处置依据 |
| --- | --- | --- |
| `claude/compassionate-einstein-ou1l2i` | `50b02930157e54dfed47d6e47a02d925d3cf4713` | 六个提交，无冲突合并；集群分享主题重做；tip 已是发布标签祖先；分支保留，未归档 |
| `claude/upbeat-hamilton-u0uab4` | `f317303aa0225b7bbb96b1f76d297c1525c61f9e` | 四个提交，无冲突合并；应用终端并排实时与任务日志；tip 已是发布标签祖先；分支保留，未归档 |

以上 tip 以 `git ls-remote` 回读为准：`50b02930`、`f317303a`，组装合并提交为 `e297ca78` 与 `5c2590c0`，发布准备提交为 `40ab5a2f`。两条分支在本会话内互相及与 `main` 做 merge-tree 模拟均无冲突。

历史本地分支（自定义壁纸、3D 场景包、实时壁纸、旧动态场景等）内容已以新提交落入 `main`（以 merge-tree 模拟确认冲突仅来自分叉而非缺失功能），不是新增候选，本轮不处置。

## 外部审计与跨仓库联动

- 安全覆盖在精确候选上为 `decision=scoped-required`，45 个提交、83 个文件未由 CF 账本覆盖；新增边界包 `internal/backupremote`，最近 full 为 `run-4`。预览按规则记录；稳定版前补 scoped，L3 扫描不替代 CF 审计。
- 来源提交保留 OCR（1.12.6）trailer：分享主题最终 range `b73d626..b527440` 46/46 文件，free-form 3、valid H0/M0/L3；终端线三段 range 分别 32/32（H1/M0/L3，已由后续提交修复旧单文件日志循环）、13/13（L1）、1/1（无发现）。发布准备提交只含版本与 CHANGELOG。
- `scriptLinkageState=not-required`（无需发布脚本（不适用））：本轮不改变脚本、安装契约或镜像内脚本内容；来源提交记录的受管脚本为 `7791920` / SHA-256 `33010d54…`，L3 受管契约检查通过。
- `Dockerfile` 与 `packaging/kejilion-app/kpanel.conf` 相对 v1.23.0 无变化；配置 blob `fa4b95374ed3b186d920207537f451a5b6d839f7`。无需应用市场提交，默认入口继续稳定版。

## 多维质量与证据边界

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 分享主题协议、快照与旧主题兼容有前端测试；日志分段读写、旧单文件日志、截断回读有 Go 测试；脚本双端资源契约未变。真实多设备未测。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 主题沙箱与快照边界沿用既有测试；govulncheck、npm audit、源码及镜像 Trivy 在 L3 通过。CF scoped 待补。 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 完整 race、镜像生命周期、更新回滚窗口与备份恢复在 L3 通过；旧 Agent 单文件日志升级路径有回归。来源记录的 `TestLegacyUpdateMigrationInMountNamespace` 在无 systemd 容器内失败，在固定 Runner 的 L3 中通过。 |
| 性能与资源预算 | 已实现未实机验证 | 来源分支给出日志读取基准（64 KiB 读 47→19 µs、诊断列表尾读 1.55→0.74 ms）；背压上限 512 KiB 有测试。真实多窗口长时间流量与低配启动未实测。 |
| 用户体验与可访问性 | 已实现未实机验证 | 主题状态筛选已补辅助技术标签；浅/深色与搜索筛选由来源测试覆盖。本发布任务未重新做浏览器组合验收，真实浏览器并排终端与 PWA 待补。 |
| 数据、配置与迁移 | 已实现未实机验证 | 无数据库迁移；任务日志沿用旧命名并兼容；环境任务保留策略会删除超过 50 条的已结束任务文件，运行中任务不清理，真实旧数据升级未单独执行。 |

## 自动门禁与本地验收

- `arena-154` SSH 超时（22 端口连接超时），经用户选择使用登记的 `local-wsl-dr`。固定 Runner 为 `kpanel-release-gate:go1.26.7-node24`，image ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`。
- L3 run `v1.24.0-rc.1-40ab5a2f-l3-r1`，UTC 2026-09-30T16:09:51Z 至 16:18:53Z，`status=passed`、`exit_code=0`。完整 Go、前端测试、typecheck、i18n、构建、race、漏洞扫描、场景包复现、镜像构建、安装/更新/回滚与备份恢复生命周期通过。
- 自包含 bundle SHA-256 `1f32be6f689f526ab4fe29e32d6014fb2409d8cc759d872fbce681ba13d0bd9f`，plan `3d4c44918ed8bb456f80a44463837d63387c3dce6ba5e6dca09b22dd8f5a1e79`，执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。原件在 `C:/GitHub/_validation/v1.24.0-rc.1-40ab5a2f-l3-r1`，终态为 `wsl-evidence/status.txt`。
- 本发布任务未运行真实浏览器 E2E：`local-wsl-dr` 不允许 browser-validation，`arena-154` 不可达；界面证据来自来源分支的测试与评审记录。
- 候选 [CI 36743425077](https://github.com/kejilion/KPanel/actions/runs/36743425077) 与 [Dependency freshness 36743425093](https://github.com/kejilion/KPanel/actions/runs/36743425093)、主线 [CI 36744345608](https://github.com/kejilion/KPanel/actions/runs/36744345608) 与 [Dependency freshness 36744345414](https://github.com/kejilion/KPanel/actions/runs/36744345414) 均在精确提交 `40ab5a2f` 上成功。

## 标签与公开产物

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.1) 于 UTC 2026-09-30T16:49:54Z（北京时间 2026-10-01 00:49:54）公开，`prerelease=true`、`draft=false`；[Release workflow 36745517344](https://github.com/kejilion/KPanel/actions/runs/36745517344) 与标签 Dependency freshness 36745517171 成功。GitHub Latest 仍为 `v1.23.0`。
- API 列出 14 个发布附件，含双架构 Agent/轻量节点、`kejilion-panel-meta-1.24.0-rc.1.tar.gz`、各平台 `kpanel-mcp`、`SHA256SUMS` 等；本轮未逐一下载校验附件。
- Docker `1.24.0-rc.1` 与 `preview` 均指向 `sha256:0d08ee52ab5ad22198093756f3e5affaf92d7e19227dfd9be4f6b1e74c4e7160`，含 linux/amd64 与 linux/arm64（Docker Hub API 回读）。Docker `latest` 与 `1.23.0` 仍为 `sha256:236759215c0f527`，未被本次发布改变。
- 在 WSL 从 Docker Hub 显式拉取 `kjlion/kejilion-panel:1.24.0-rc.1`（RepoDigest 与上述摘要一致），用候选源码的 `packaging/tests/image-e2e.sh` 以 `KPANEL_EXPECTED_VERSION=1.24.0-rc.1` 运行，输出 `image_e2e=pass`，退出码 0。证据 `C:/GitHub/_validation/e2e-rc1.log`。这是公开 amd64 镜像运行证据；arm64 仅核对发布 descriptor，未执行容器，也不是真实 Nginx/PWA 浏览器验收。

## 自更新、生产与回滚

- 自更新通道契约未改；本轮未在真实宿主重测。
- 生产部署安全核对：不适用（预览版禁止生产部署）。`prod-108` 禁用全部 KPanel 操作；本次未连接、未部署、未核对。WSL 候选和公开镜像验收不是生产证据。
- 稳定源码与镜像回滚点见首段，上一不可变预览为 rc.8；实际回退前须备份和检查数据兼容，退出预览不会自动降级。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-30T20:51:20+08:00
- 候选冻结时间：2026-10-01T00:09:51+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与分支收尾

- CF scoped 审计、真实 Nginx/隔离主机浏览器（并排应用终端、分享主题）、原生浏览器 200% 缩放与 PWA 验收留待稳定版前；当前公开镜像验收不能替代这些场景。
- `arena-154` 恢复后应补在登记主机上的公开镜像 E2E 与浏览器验证。
- 来源分支 `claude/compassionate-einstein-ou1l2i`、`claude/upbeat-hamilton-u0uab4` 的 tip 均已是发布标签祖先；由于它们来自仍可能继续推送的远程会话，本轮保留原名不归档，待来源会话确认结束后按 `docs/project-management.md` 10.2 归档。`release/v1.24.0-candidate` 继续供同序列后续 RC 使用。
