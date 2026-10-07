# KPanel v1.25.0-rc.11 发布验收记录

日期：2026-10-07

发布级别：L3

候选提交 / 标签：`e42afcfe7f47ea7cb4803d644c23583f844320cb` / `v1.25.0-rc.11`

上一稳定版本 / 回滚点：`v1.24.0` / tag target `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；Docker `latest` OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。

`releaseChannel`：`preview`

`releaseTrain`：`1.25.0`

候选分支与发布后处置：`release/v1.25.0-candidate` / 预览版保留

- 原分支 / 精确 tip / 处置分类：候选 tip `e42afcfe7f47ea7cb4803d644c23583f844320cb`；预览序列唯一候选，已推送，保留。候选与 main 的同 SHA CI 均成功；annotated tag object `1e687b0131b89575369c12f308e82f216968b5f2` 的 target 为该 SHA。
- 归档 ref 与 SHA / 远端复核结果：下表五个来源均已保存到本地 `archive/<原分支全名>`，归档 SHA 为表中完整原始 tip；活动本地引用已按 expected-SHA 删除。远端逐项查询成功，五个活动来源及其 archive 均不存在；本次原来源只在本地，不创建远端来源归档。公开 RC 候选保留。
- 本次来源任务分支及纳入依据：

| 原分支 | 原始 tip | 纳入候选的精确映射 |
| --- | --- | --- |
| `claude/desktop-mobile-widget-page` | `1e0733e1a20f0a8d088991232dafccddbf65d124` | `c4c81d85`→`e24faecf`、`9b832178`→`65d47967`、`1e0733e1`→`f7f7229a`；range-diff 全部相等 |
| `claude/docker-run-command` | `f4cf40852a77bb98772ed9174b4f8cf4da070303` | `f4cf4085`→`38b16566`；range-diff 相等，候选另含敏感值和运行配置修复 |
| `codex/desktop-shortcut-open-directory` | `131d03d294f5e7c5a68ea776cbf41fa5a7b97ca6` | `abf6f7c7`→`c27de676`、`eb844088`→`fd9b6af7`；原空 OCR 提交由候选聚合审查替代 |
| `codex/desktop-browser-open-label` | `ca19403aeb9a6574bb9f75d86580a300d3ea96a2` | `ca19403a`→`a10eec9f`；range-diff 相等 |
| `feature/files-video-f-fullscreen` | `783c8b8e53605d07eaa036df5cef68c0057de7d6` | `783c8b8e`→`143d2b1e`；相邻 closePreview 上下文因快捷方式改动调整，F 全屏逻辑保留 |

- 本地分支/upstream/worktree：五个来源转为原 tip 的 detached/clean 工作树，三个原有 `origin/main` upstream 已解除；文件和忽略原件保留，分支归档不附带跨任务工作树删除。当前候选及 `127.0.0.1:4179` 预览保留。验收记录在独立 `C:\GitHub\_codex-tasks\kpanel-v125-rc11-acceptance` 工作树、`docs/release-v1.25.0-rc.11-acceptance` 分支形成，不改冻结候选。
- 纯验收候选处置契约：本文件形成独立 docs-only 提交，在同 SHA 文档候选 CI、main 快进及 main CI 成功后保存到 `archive/docs/release-v1.25.0-rc.11-acceptance` 并移除活动引用；精确提交、两次 CI 和归档结果在仓库外最终回执中核对，不反写公开代码标签。来源工作树物理删除没有本次明确授权，保留恢复位置，后续由已释放所有权资源的统一盘点处理。

产物已发布，生产未部署；预览版禁止生产部署。

## 发布画像

- 业务域：手机桌面、文件快捷方式与媒体预览、Docker 配置只读展示。
- 变更面：展示、交互及 Docker Inspect 数据到等效 `docker run` 的转换；没有执行生成命令的自动写入动作。
- 受影响用户旅程：手机左侧组件页与滚动/翻页边界；从桌面直接打开文件预览及独立所在目录；视频 F 全屏；查看容器创建命令与确认复制完整敏感值。
- 未变化契约：数据模型、端口、Compose、Agent 权限、内置 `kejilion.sh` 和应用市场默认入口不变。Docker 增加只读创建命令 API 与类型；不增加宿主机写入协议。
- 风险等级及理由：完整发布按 L3；敏感值遮蔽和命令重建需要边界回归，手机/窗口组合需要界面验收。

## 发布范围与未纳入内容

- 用户可见更新：手机桌面新增位于图标首屏左侧的组件页，首个图标页仍默认；Docker“查看创建命令”保留运行配置并报告额外网络，默认遮蔽敏感值、复制完整值需确认；桌面文件快捷方式直达预览，所在目录可独立打开；视频预览按 F 切换全屏，输入框聚焦时不触发；系统浏览器入口文案改为“浏览器打开”。
- 精确提交清单：基线 `f148b665f51769acfe6f4c953a8f61c4d0a8b634` 之后的 15 项提交，依次为 `38b16566`、`e24faecf`、`65d47967`、`f7f7229a`、`c27de676`、`fd9b6af7`、`143d2b1e`、`a10eec9f`、`1e6962e6`、`215a56e2`、`cc7358dc`、`7303ceef`、`3ef0d1bd`、`b5de6daa`、`e42afcfe`；精确完整 SHA 可从冻结候选和 L3 bundle 读取。
- 明确未纳入：其他功能候选、治理修改、生产部署和稳定通道提升。没有修改 `deploy/` 或 `packaging/`。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：本轮没有启动 CF scoped/full；沿用既有 full `run-4`（source `4c0694aa8e02`）及已登记 scoped 覆盖。
- 覆盖检查：`check-security-audit-coverage.mjs --target e42afcfe7f47ea7cb4803d644c23583f844320cb` 返回 `decision=ok`；未审计提交数 `32`、文件 `128`、最早 3 天；full 年龄 16 天/上限 30 天，`new_boundary_packages=none`。RC 仅记录，不将该输出解释为新专项审计通过。中断的 run-3/run-5 不作为覆盖。
- finding / 修复 / 独立复核：聚合代码复核期间修复 dash-leading 秘密参数、passphrase 字段遮蔽和运行配置缺失，分别为 `215a56e2`、`cc7358dc`、`7303ceef`；定向测试及完整 L3 通过。原功能实现由其他任务交付，发布任务承担聚合复核；没有新 CF finding 验证结论。
- 修复交付状态：冻结源码、候选/main CI、RC 公开 Release、preview 镜像及公开镜像 E2E 已完成；没有稳定版或生产交付。
- OCR：`1.12.11`，`f148b66..b5de6da`，39/39 可审文件已复核，4 项按规则排除；有效 H0/M0/L0，free-form=0，constrained-only=unreported。最终 `e42afcfe` 只增加 OCR trailer 空提交，代码树与 b5de6da 相同；gofmt 前后仅五字段对齐。
- 观察口径：只记录本次候选，不依据单次零发现宣称工具有效或应退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本（不适用））。
- 变更集编号、脚本候选、依赖阻断：不适用。
- 实际内置脚本：commit `c3a8bd895f8878d9e4ced7592c91a20c974472a5` / SHA-256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`。
- 状态依据：候选没有修改 Dockerfile、安装/更新配置或 Node 更新运行时来源；复用现有固定脚本契约。
- 本版发布决定：脚本不在范围；`kejilion/apps` 无需提交。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 定向回归、完整 Go/Web 检查及 mock 交互 | 真实容器创建命令与跨节点 UI 未操作 |
| 网络入侵与供应链安全 | 已验证 | govulncheck、npm audit、Trivy source/config/native image 通过 | 自动扫描范围内结论；新 CF scoped/full 未执行；1 项未见调用的模块通告待复核 |
| 稳定性、失败恢复与兼容 | 已验证 | 核心 race、安装安全、app-conf lifecycle 和 backup parity 通过 | 真实主机升级/重启、长稳未执行 |
| 性能与资源预算 | 已实现未实机验证 | Release 原生镜像在 256 MiB/1 CPU/128 PID、非 root/只读根/cap-drop ALL 下运行契约通过 | UI 性能/触摸设备资源与 arm64 实机运行未测 |
| 用户体验与可访问性 | 已实现未实机验证 | 手机 390px 组件页、Docker 弹窗、图片快捷方式和独立目录窗口预览 | 真机触摸、真实 125%/200% 缩放、视频全屏及 Markdown 内容没有完整实机证据 |
| 数据、配置与迁移 | 不适用 | 无 schema/数据迁移；应用市场契约相同 | 生产数据没有操作 |

## 自动门禁

- 定向测试：Docker 命令转换、命令弹窗、容器菜单 3 文件/20 测试通过；typecheck 通过。
- 完整 L3：固定 Runner Go `1.27.1` / Node `24.21.0` / npm `11.19.0`；r8 通过，Go lane `559595 ms`、Web `587122 ms`、deploy `4315 ms`、source 总计 `587178 ms`；Web 2343 passed/6 skipped；核心 race、场景包复现、二进制/镜像、扫描、安装和更新生命周期全部完成。
- L3 外层入口：`run-release-l3.mjs`，run `v1.25.0-rc.11-e42afcf-l3-r8`，`arena-154`；UTC `2026-10-07T08:32:38Z` 至 `2026-10-07T08:46:20Z`，status=passed/exit=0；Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`。
- 输入 SHA-256：plan `a989c6ae04c507493b8cd9b247e281320bafce48e5b100a15025f87842142ec5`；remote script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `2384bdab3c144b1aa13ec41137540c5d0bcf56c3b64cb0f3625e2dc66daa1154`。
- 证据目录：`C:\GitHub\_release-evidence\v1.25.0-rc.11-e42afcf-l3-r8`，remote `/root/kpanel-release-evidence/v1.25.0-rc.11-e42afcf-l3-r8`；下载日志 SHA-256 `0229ef06803eb02745fff4fce22b55c88084f0880a8708fdfa777df819e4da22` 与远端 evidence 清单匹配。
- 候选 CI：[37596089634](https://github.com/kejilion/KPanel/actions/runs/37596089634) success，精确 SHA `e42afcfe`；Dependency freshness [37596089985](https://github.com/kejilion/KPanel/actions/runs/37596089985) success。
- 主线 CI：[37597728069](https://github.com/kejilion/KPanel/actions/runs/37597728069) success，同 SHA；Dependency freshness [37597728185](https://github.com/kejilion/KPanel/actions/runs/37597728185) success。
- Release workflow：[37599049525](https://github.com/kejilion/KPanel/actions/runs/37599049525) success，`v1.25.0-rc.11`，精确 SHA `e42afcfe`；UTC `2026-10-07T09:12:52Z` 至 `2026-10-07T09:25:23Z`，源码、扫描、生命周期、二进制、原生运行契约、多架构推送、preview 提升及公开 Release 均成功；preview 候选归档步骤按通道 skipped。
- 安全扫描：0 可达 Go 漏洞，0 npm 漏洞，Trivy source/config/native image 0 findings；govulncheck 有 1 项 required module 通告但无调用证据；SBOM/provenance 由 Release 生成，公开 index 有对应双架构的 attestation manifests。

## 依赖与技术栈变化

- `make dependency-report`：未单独生成；Dependency freshness 自动任务 success。
- 每日安全通告/EOL：本次未另行执行每日审计；使用发布自动扫描，结论仅限其范围。
- 本版采用项：无依赖、基座、Actions 或脚本版本升级；package/lockfile 仅同步 RC 版本号。
- 版本、摘要、兼容与回滚：版本 `1.25.0-rc.11`，工具链及固定脚本摘要见上；回滚点 `v1.24.0`。
- 暂缓项及期限：未见调用的模块通告交依赖维护者在稳定版准入前复核；未声称已经修复或排除全部风险。

## 隔离真机与浏览器验收

- 环境：Windows 本地 loopback mock UI；Linux/amd64 `arena-154` 固定 L3 Runner。环境用途为 `candidate-validation`。
- 精确候选：`e42afcfe7f47ea7cb4803d644c23583f844320cb`，最终 preview ID `v125-rc11-candidate-1791357744548-7cf7a2`，URL `http://127.0.0.1:4179`；manifest `C:\GitHub\_release-evidence\v1.25.0-rc.11\acceptance-final\manifest.json`，clean/ready，`visual-composition`。
- 交互证据：gofmt 前同代码旅程的手机组件页与桌面组合、Docker 遮蔽/确认、图片文件快捷方式直接预览、独立所在目录窗口、浏览器标签；最终代码只作字段对齐并刷新 OCR，最终预览重新启动。
- 窗口/循环：短时确定性交互，不适用长稳 soak；本版没有引入生命周期、重连或流式新路径。
- 未执行：真实手机、实际 Docker UI 多配置容器、100%/125%/200% 原生缩放、视频 F 真全屏及 Markdown 内容完整显示；mock 媒体/Markdown 夹具不足，不据此声称通过。
- 宿主机写入/恢复：隔离 rootfs 夹具执行更新和失败注入；公开镜像在独立临时数据、自动分配 Docker 网络与 loopback `18080` 执行初始化/健康/静态资源/Cookie E2E，并由脚本清理容器、网络和测试目录；没有生产管理员写操作。

## 发布产物与公开仓库复核

- GitHub Release：[v1.25.0-rc.11](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.11)，draft=false、prerelease=true；公开于 `2026-10-07T17:25:10+08:00`；发布后 Latest 仍为 `v1.24.0`。
- 版本 `kjlion/kejilion-panel:1.25.0-rc.11` 与 `:preview` OCI index 均为 `sha256:e3eb789968da567b7bd1d21c25bb7b1288e7fd2e843afa69f20314da5546e93f`。
- 平台 digest：linux/amd64 `sha256:1df40b0d756568b4fb641c9286f0a4a605f43fd07e33918295988261960c57ce`；linux/arm64 `sha256:185d8c479f3808e34dff4c726af1c291bf290dbad185d43bb2749263b035cd88`。另两项 unknown/unknown 是对应 attestation，不作为运行平台。
- 稳定 `latest` 发布后 OCI index 仍为 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`，与发布前相同。
- Release 有 14 个附件，包括 Agent amd64/arm64、metadata 与 SHA256SUMS。已下载公开 SHA256SUMS，其 SHA-256 `7d35b2966c90fcf27be4cccc2e566940eefd4c3d48ec2f886916fe34c9a00315` 与 GitHub asset digest 一致；11 项 checksum 与附件 API digest 均一致。未逐件下载二进制重算，也未在 arm64 实机运行。
- 发布前 preview RC10 OCI index：`sha256:8f92695039d4842356d24a967c3414cca501af1dea57426b67be0f178e73d472`。
- 公开镜像 E2E：显式 `docker pull` 得到上述公共 index，`KPANEL_EXPECTED_VERSION=1.25.0-rc.11 sh packaging/tests/image-e2e.sh docker.io/kjlion/kejilion-panel:1.25.0-rc.11 18080` 在 `arena-154` 输出 `image_e2e=pass` / exit=0；仅 Linux/amd64。版本、健康、静态图片字节、初始化、两种 Secure Cookie 情况及容器健康检查通过。下载后的 OCI version/revision/script 标签逐项匹配冻结源码和固定脚本。
- `kejilion/apps`：工作树 clean，HEAD `4fc985e964dbd1742143521d0a28ab652b885773`；配置归一化 SHA-256 两仓库同为 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`（原始字节因 CRLF 不同）；无内容差异，无提交。

## 自更新通道验收

- stable/preview 来源：沿用既有来源选择和 digest 合同；L3 全套自更新测试覆盖。本版没有改变自更新代码。
- 加入预览、立即检查、自动安装独立开关、旧状态默认 stable、重启保持、退出预览不自动降级：由现有 Go 测试覆盖，未在真实实例逐项操作。
- systemd 更新前备份、失败恢复与版本隔离：L3 lifecycle/backup parity 通过；生产实例未执行。
- OpenRC/轻量 Node：沿用 `docs/release-channels.md` 的现有边界。

## 生产部署安全核对

- 生产目标、授权、部署前版本/备份、命令、部署后健康/数据/公网入口：不适用（预览版禁止生产部署）。
- 验证/灰度环境：`arena-154 candidate-validation`，隔离容器/rootfs；没有部署生产。
- 正式部署环境：不适用。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：否。
- 仅在隔离环境执行：源码/race/构建/扫描、更新备份与故障恢复夹具。

## 回滚

- 源码：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`。
- 镜像：稳定 OCI index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。
- 数据/配置：没有生产操作或数据迁移，无新增生产备份。
- 预览退出：按现有稳定来源/应用市场回滚流程选择稳定版本；没有自动降级，实际数据回滚未执行。
- 回滚后生产版本/健康：不适用（未部署）。
- GitHub Latest、Docker latest 与默认更新入口：发布后 GitHub Latest=`v1.24.0`，Docker latest 为原稳定 index，应用市场仍默认 latest；没有变更。
- 公共默认更新通道决策：维持 stable 默认，preview 供主动选择。

## 交付节奏数据

冻结源码的 `report-release-metrics.mjs --days 14 --releases 20 --format json --ref e42afcfe7f47ea7cb4803d644c23583f844320cb`（生成于 `2026-10-07T09:13:40.097Z`）显示：近 14 天稳定标签发布 3 次，有生产完成证据的部署 0 次；最近 20 个稳定版本验收覆盖 20/20，生产完成/用时报告 15/20，提交到生产中位数 4.07 小时，变更失败 1/20（5%）。缺失的 5 项生产事实不推断为零。RC 单独统计，不进入稳定发布频率、部署频率或变更失败率。本版未执行生产写操作。

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-07T08:54:05+08:00
- 候选冻结时间：2026-10-07T15:21:37+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（本轮 RC 一次公开；没有生产写操作）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：13
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/run-release-l3/missing-runner-alias",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1 固定别名镜像不存在，preflight 停止，候选测试未执行。",
    "recoveryEvidence": "以历史 tag 解析到同一冻结 immutable image ID；r2-r6/r8 均先核对 6f1e654d，r8 全门禁 passed。",
    "permanentAction": "冻结前核对实际 Runner tag 和 ID；仓库别名配置尚未修改，发布维护者在下一次生产 L3 前复核，2026-10-14 前决定统一别名或可靠交接。",
    "historicalReleases": []
  },
  {
    "fingerprint": "lifecycle/run-release-l3/unresolved-test-exit",
    "position": "before-production-write",
    "count": 1,
    "impact": "r3 完整源码/扫描/构建通过，但 app-conf lifecycle 非零结束。末尾磁盘提示来自测试 fake df，不能据此认定真实磁盘为根因。",
    "recoveryEvidence": "同 SHA/Runner 的同脚本独立 trace 复跑 passed，随后完整 r8 app_conf_lifecycle 和 app_conf_update_backup_parity passed；r3 原始失败保留。",
    "permanentAction": "根因尚未证明，不声称永久修复。发布维护者在 2026-10-14、稳定版准入前复核原始失败和已保留 trace；退出条件为可复现原因及唯一测试入口回归。",
    "historicalReleases": []
  },
  {
    "fingerprint": "source-checks/run-release-l3/insufficient-staging-space",
    "position": "before-production-write",
    "count": 3,
    "impact": "r4-r6 在 backupremote/hostbackup/panel 的 staging 空间保护处失败，Web 被取消；当时根文件系统仅约 2 GiB 空闲，测试瞬时输出消耗余量。",
    "recoveryEvidence": "固定 cleanup 脚本验证旧 L3 干净源码副本及 bundle 后释放根文件系统 2591797248 bytes；r8 源码、备份和完整门禁 passed。/tmp 是 tmpfs，其清理不算 root 释放。",
    "permanentAction": "未降低空间保护或测试强度。未来冻结前由发布维护者复核瞬时空间预算；在下一次生产 L3 前完成唯一入口预算预检/回归，2026-10-14 复核，现场清理不等于永久修复。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3/run-release-l3/runner-id-store-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "r7 试用 local-wsl-dr 时，同归档在 legacy overlay2 和 containerd image store 报告不同 ID；精确身份检查拦截，候选代码未执行。默认环境并未不可达，不把该尝试当合格灾备证据。",
    "recoveryEvidence": "归档 SHA-256 77126ed939467fb9114637d7a0daba547307ce3700f7f93beec696850612b705；原 runner ID cdf09f 与 manifest 6f1e 不符。回到 arena-154、原冻结 ID 完成 r8；没有放宽身份检查或切换本地 Docker 后端。",
    "permanentAction": "保持 arena-154 固定环境；仅默认不可达且明确选择灾备时才准备 DR kit，先核对存储后端的 ID 表示；发布维护者在下次 DR 前完成预检，不声称跨后端已兼容。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 4,
    "impact": "4 次只读证据定位调用猜测了不存在的 L3 日志/脚本锁文件/检查入口；其中一次包含两个错误脚本名。未使用失败输出作通过证据。",
    "recoveryEvidence": "改读 l3-verify-release.log、Dockerfile 与 cmd/kejilion-node/update_runtime/source.json，以及 AGENTS.md 中的实际入口；契约、固定摘要和 r8 日志均已核对。",
    "permanentAction": "此指纹在 v1.24.0 已出现，属于复发。路径必须先从 rg --files/权威脚本发现；当前没有生产写操作，发布维护者须在下一次生产 L3 前完成固定读取入口预检及回归，2026-10-14 复核。",
    "historicalReleases": ["v1.24.0"]
  },
  {
    "fingerprint": "preflight/rg/windows-path-glob",
    "position": "before-production-write",
    "count": 1,
    "impact": "把 scripts/bundle* 作为 rg 文件参数，Windows 未展开，读取失败。",
    "recoveryEvidence": "改用 rg --files 的 -g 过滤并读取实际路径；真实固定脚本来源从 Dockerfile 核对。",
    "permanentAction": "Windows 文件枚举统一使用 rg --files -g，避免把 shell glob 当路径。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/powershell/path-parameter-set",
    "position": "before-production-write",
    "count": 1,
    "impact": "重复 Runner 归档盘点使用 Split-Path -LiteralPath 与 -Parent 的无效参数组合，预检失败；没有删除。",
    "recoveryEvidence": "使用 Get-Item 的 Directory.FullName 和祖先属性复核，两个归档 SHA/长度相同，精确目录内无 reparse point。",
    "permanentAction": "文件操作使用 FileInfo/DirectoryInfo 已解析对象和 LiteralPath；删除仍需逐项恢复及路径检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/powershell/foreach-pipeline-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "最近五个稳定验收记录的只读 JSON 汇总将 foreach 语句直接接管道，PowerShell 解析拒绝，未产生历史核验结果。",
    "recoveryEvidence": "先将 foreach 输出收集到变量再 ConvertTo-Json，完成历史指纹比较并保留 process-history-five-stable.json。",
    "permanentAction": "结构化读取采用集合变量输出，不把语句块直接串入 Shell 管道；不使用失败调用作历史复发证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

同根因批次按实际尝试次数计数；所有异常都在生产写前，不计生产变更失败。r2 的 gofmt 格式失败属于正常产品源码门禁拦截，由 `b5de6daa` 修复并对新 SHA 完整重跑，不自动计流程异常。最近五个稳定版本 v1.20.0-v1.24.0 已复核，确认路径猜测指纹复发；未把一次恢复当作永久治理修复。

## 遗留风险与后续准入

- 资源回收：已核对、回收 10 项本任务或历史已结束 L3 的可再生干净源码/输入副本，保留原始 bundle 与全部失败证据；arena 根实际释放 `2591797248 bytes`，清理脚本 `C:\GitHub\_release-evidence\v1.25.0-rc.11\cleanup-l3-work-r1.sh`，SHA-256 `347b5b61853a32aa3620edc1b01b779d2a75cf32888be8519fd4b80770153a8d`。本地已删除 r7 的重复 Runner tar，C: 空闲量即时测得增加 `822333440 bytes`（文件逻辑大小 `822330880`），保留 `C:\GitHub\_release-evidence\kpanel-runner-go1.27.1-node24.21.0-6f1e654d.tar`，SHA-256 `77126ed939467fb9114637d7a0daba547307ce3700f7f93beec696850612b705`；恢复 r7 kit 时可复制回原文件名，原 manifest/失败证据保留。当前候选/4179 预览、来源及验收记录工作树、WSL 排错 kit、唯一 Runner 原件和本轮原始失败证据保留；未回收未知归属或无关路径。
- 未验证风险：实际容器配置 UI、真实手机/全屏/缩放/性能、Markdown 夹具不足、新 CF 专项、未见调用的模块通告，以及 r3 未证明的退出根因。
- 已实现待实机准入：本版五组用户功能；仅自动回归和 mock UI 的项目不得写成真机通过。
- 不阻断本版的理由：用户明确要求预览；完整 L3、候选/main CI、Release 在精确最终 SHA 成功，双架构公开摘要一致，公开 Linux/amd64 E2E 通过；真机功能限制明确，无生产或 stable/latest 操作。
- 后续准入：稳定版前复核边界审计及依赖通告，补真机和资源验收，按复发指纹修复固定预检，不降低门禁。
