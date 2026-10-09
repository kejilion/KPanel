# KPanel v1.26.0-rc.1 发布验收记录

日期：2026-10-09；发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.26.0`。

候选提交 / 标签：`861a3335a4e2f5263b0d32942da89c3f5becfe4b` / `v1.26.0-rc.1`；tree `f67fe9bf1569a288322c80869399a8280054e98f`。
公开时间：`2026-10-09T11:39:03Z`；[GitHub prerelease](https://github.com/kejilion/KPanel/releases/tag/v1.26.0-rc.1)。产物已发布，生产未部署。

上一稳定版本及回滚点：`v1.25.0` / `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83` / `sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`。

## 发布画像与范围

单管理员 Linux 控制面的桌面、Office 和网站证书旅程；界面展示/交互及已鉴权的宿主机续签写入，按 L3 发布。API/存储真源、Panel/Agent 权限分离、安装端口和应用市场稳定默认入口保持既有契约。

| 来源 | 精确提交 | 本轮范围 |
| --- | --- | --- |
| 桌面材质与动效 | `bb03ad3015f950a6e03330f27b3973705e9e2cc4` | 外框与短时浮层材质、活动焦点、无障碍降级、经典壁纸 |
| Office | `b50a427e4f5fb61683d8bbcaa01ccd1aa7e3ea04` | Word 编辑分页、Excel 键盘与公式只读、幻灯片比例、草稿/冲突/保存重读 |
| 手动 SSL 续签 | `f2c867b4b48ecc1eddb6ad60efef87f7a53c51dc` | 规范域名及资源版本校验、互斥、原证书保护、Nginx 恢复与有效期重读 |

来源祖先均保留。组合缺陷在发布方专属修复分支处置：`2adec96c` 限 systemd 能力，`19a44ddb` 绑定兼容脚本，`3a1f4e0e` 修复治理记录字段，`5d4be342` 复用现有动效 token，`35dcc3df` 更新安全工具链与网络库，`861a3335` 仅纠正 OCR 工具版本收据。

用户明确排除 Docker 批处理；旧候选 `dca6305b9d631aeea5b12eebffa6b6752b731d5a` 及本次复查刚形成的新候选 `9a0954e4cb61dbcd21cb0b7c3a3cb13d065fabe0` 均不是本标签祖先。新 DockerBatchBar/Dialog/dockerBatch 模块未在候选树出现，DockerView 本轮仅去除由统一材质接管的重复样式。Compose 删除已随 v1.25.0 发布；旧审计/诊断、已等价集成的历史分支和其他项目不拼入本轮。

## 多维质量与证据层级

| 维度 | 状态 | 本轮证据及实际限制 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 完整源码门禁、生产前端与 Mock 的 54 项界面用例、配套脚本 11 项资格化；真实 CA 签发与真实宿主机续签双向闭环仍未验证 |
| 网络入侵与供应链安全 | 已验证 | 可达漏洞检查、npm 审计、源码及镜像扫描通过；govulncheck 仍报告不可达模块通告，CF run31 incomplete，不计完整覆盖，不表示无安全风险 |
| 稳定性、失败恢复与兼容 | 已验证 | 草稿冲突、提交后读取失败、合成续签 503 恢复/固定失败及只读/能力关闭；未执行生产故障或完整回滚安装 |
| 性能与资源预算 | 已验证 | 24 个 GPU 原件与资源保护已核对；经典壁纸拖动 P95 中位数退步 25.485%，未达预设 20%，performanceQualified=false；根因未确认 |
| 用户体验与可访问性 | 已验证 | 390/768/1280 视口、深浅主题、键盘/IME/Escape、文案和失败状态的限定浏览器矩阵；真实手机、原生缩放、Safari/Firefox 与低端 GPU 未验证 |
| 数据、配置与迁移 | 已验证 | Mock 提交/重读、冲突与草稿反馈，脚本隔离夹具的证书保护/互斥；未执行真实 Office 文件/宿主配置的完整互通或正式数据迁移 |

## 冻结与完整 L3

冻结 `2026-10-09T10:22:28.296Z`；run `v1.26.0-rc.1-861a3335-l3-r2`；唯一入口 `scripts/run-release-l3.mjs`；登记目标 `arena-154`；Runner `sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`（Go 1.27.2 / Node 24.21.0 / npm 11.19.0）。

实际开始 / 结束：`2026-10-09T10:24:19Z` / `2026-10-09T10:40:42Z`；终态 passed，退出 0。
完整 Web/Go/部署源码组、核心 race、vet、安全扫描、场景包、Linux 双架构二进制、Docker 自包含构建、受管脚本镜像契约、最终镜像扫描和应用配置生命周期全部由该 run 完成。前端 265 文件通过 / 2460 测试通过 / 6 skipped。

计划、脚本、self-contained bundle 与 manifest 摘要：`C:/GitHub/_release-evidence/v1.26.0-rc.1-861a3335-l3-r2/manifest.json`；12 个原始文件均回收并逐项 SHA-256 核对。原始日志 `C:/GitHub/_release-evidence/v1.26.0-rc.1-861a3335-l3-r2/remote-evidence/l3-verify-release.log`，SHA-256 `3bc72568b81d46322d969e2b3d7ec4e52f737ae2845a3981eef64a7cec73df6e`。

db3825f2/8c81d9fb 的治理拒绝、3a1f4e0e 的动效断言失败、5d4be342 的 13 条可达 Go 标准库安全通告、861a3335 r1 的磁盘停止 failed/137 均保留原件，不使用这些失败状态冒充最终通过。

## 隔离浏览器与性能观察

后台作业 `arena-154-48200`；唯一目录 `affected-ui-r11`；候选 `861a3335a4e2f5263b0d32942da89c3f5becfe4b`；状态 passed，退出 0；780 秒硬限时；命令规格 `C:\GitHub\_release-evidence\v1.26.0-rc.1/affected-ui-r11-spec.json`，SHA-256 `e2ef9f3a6a95deef63a813b252e9fd90bee8a29dba202563bf5678fee5edf4fa`。Node 24.18.0 / Playwright 1.55.0 / Browser Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`。

54 项：桌面材质 12、布局 7、动效 2、分组 4；Office/SSL 29（18 个 Office 视口/主题/格式组合、编辑/IME/保存重读、冲突、复制拒绝、提交后读取失败、Excel 键盘/公式、分页/不支持格式及 5 个合成续签场景）。原件 `C:\GitHub\_release-evidence\v1.26.0-rc.1/affected-ui-r11/remote-evidence`；回收清单与摘要 `affected-ui-r11/recovery.json`。

生产前端构建配合 Mock/synthetic API；合成续签没有真实 CA 或 Nginx 操作。浏览器/Mock 有界 CPU、内存、PID、只读挂载和临时目录；清理回执及资源日志按原件核验。普通确定性交互不机械执行 soak。

GPU 原件：`gpu-final-r4/result.json`，观测提交 5d4be342；最终候选的 web tree 与其字节相同。Windows RTX4060 / Chrome149 / D3D11，24 个交错样本均保留；经典拖动结果未达预算，不归因于用户操作，也不宣称无模糊开销。

预览准入使用测量前已存在的材质试行规则（`docs/quality-improvement-2026-10-09-material-motion-system.md:91`），`gpu-preview-admission-r5.json` 仅验证证据与非阻断政策。原预设 20% 保持失败；其他 7 组中位数在预算内不能覆盖该失败。桌面与发布负责人于 2026-10-16 或稳定提升前复核，以先发生者为准；退出条件为隔离 GPU 重测及所需性能准入。用户指出前台干扰后，本次后续验收统一远端 headless 后台，无新增本地可见 GPU 探针。

## CF 与 OCR

CF run31 使用 `gpt-6-luna` / `max` 的专用审计及不同发现/验证代理；只读源码片段 `f2c867b4` / tree `99813b177086265502efd914e201d9ec1bd1f638`，配套初始脚本 f0e0c652。16 次调用 / 2520 秒封顶；token/费用未提供。

官方 findings/ledger 结构验证退出 0（1 finding / 5 units）；最终 distinct critic 在 09:29:19Z 截止前未返回，run_status=incomplete、scope_complete=false、risk_acceptance=null；保留 1 needs_validation、0 confirmed blocking findings，不计完成覆盖。systemd-only、1800d955 和最终候选不在该审计 reviewed_commits；父级开发测试不是 CF 动态验证。最小脱敏元数据入 `.governance/security-audit/run-31/run-metadata.json`，原始 findings/攻击材料保留仓库外。

覆盖观察实际退出 0：`security-coverage-final-r5.log`，decision=ok；未审计 57 commits / 166 files，最老 5 天（上限 14 天），上次 full run4 距今 18 天（上限 30 天）。这是既有覆盖阈值判断，run31 incomplete 不计覆盖；稳定提升前重新检查覆盖与必要补审，不声称全局审计通过。

OCR 适用候选按本轮现有 delegate 原件记载，先自由臂后约束臂，不扩大为全部历史任务覆盖。安全修复的实际工具 1.12.11，自由评审覆盖 13 个实际修改文件，约束臂为 10/10 可评审文件、0 findings；CHANGELOG/go.mod 为不支持、go.sum 明确排除。0.3.0 错误记录保留并由 861a3335 收据纠正。主功能来自 Claude，发布方独立完成组合复核与修复；发布方自身修复不冒称再次独立复核。

## 跨仓库脚本与应用市场

`scriptLinkageState=coupled`；变更集 `kpanel-v1.26.0-rc.1-manual-certificate-renewal`。兼容脚本先行发布于 `1800d955f216aebd2776674368a8e469a671c489`，批准上游基线 `90d1b1f9` 保留祖先；ROOT SHA-256 `5778fdc9637c8614f5246f9eb13c1e549de51522fb91b3805067cbc0475b614c`，CN `167b5d09c84c1d2eb5ebd8141b0703e5799ca9285429cf98e2fed38fd330f862`。面板 Dockerfile pin、公开双架构 OCI 配置和实际镜像内脚本摘要匹配。

精确脚本 r7 隔离资格化 11 项通过（工具、同步、ROOT/CN syntax 与 7 smokes）；原件 `script-qualification-r7/remote/result.json`。systemd 才启用手动续签；OpenRC 不宣称具备可靠中断恢复；通过 Nginx 的面板可能短暂断开，连接恢复后核对证书，无自动重复 POST。

应用市场新鲜读取与当前源码/local 归一化 LF 字节匹配：`f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；相对 v1.25.0 无契约变化，默认 latest，appsCommitRequired=false、appsWrites=false。未覆盖其他 sh/apps 作者工作树。

## 依赖与技术栈

首次 L3 的可达安全扫描拦截 13 条 Go1.27.1 标准库通告；采用官方 Go1.27.2 与 x/net v0.60.0，并同步 Dockerfile、固定 Runner、CI/Release/工作流与锁文件。Node24.21.0 保持。Go alpine index `sha256:85dc1069ac644ea3c527b177303a406eb3358192816cd7f9e5848eb658851673`；bookworm `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`。

候选/main/tag 的 Dependency freshness 结果见对应 CI 原始 API；检测成功不等于所有建议已采用。Trivy 固定 0.74.0 扫描原件提示 0.75.0 可用，本轮不夹带扫描器升级；构建维护负责人 2026-10-16 复核，退出条件为聚焦升级候选与固定规则回归。兼容/构建结论仅限本轮门禁及公开产物；旧稳定版仍是回滚点。

依赖报告实际步骤与原始 Jobs API 已回收，见 `dependency-ci-qualification-r1.json` / `dependency-ci-raw-r1.json`。候选、main、tag 的生成时间窗口依次为 `2026-10-09T11:01:01Z 至 2026-10-09T11:01:08Z`；`2026-10-09T11:15:20Z 至 2026-10-09T11:15:35Z`；`2026-10-09T11:26:46Z 至 2026-10-09T11:26:51Z`。固定 CLI 在任一源失败时退出 2，维护例外或 EOL 到期时退出 3，workflow 未使用 --allow-partial；据实际成功步骤与该退出契约，推导 10/10 检测源完成。这是可复核推导，不冒称逐行下载了报告 Markdown 或完整候选列表，也不表示全部依赖最新。

最近一次可观察每日安全通告作业为 [37912308544](https://github.com/kejilion/KPanel/actions/runs/37912308544)，基线 `29f7451816a69acad9aa999dd936b1e1021f0627`，结论 failure，结束 `2026-10-09T09:36:54Z`。未单独回收该失败日志，不推定其具体原因；本轮最终提交的当前依赖图审计由实际 L3 与候选/main CI 通过证明，不能改写旧每日作业为成功。

策略登记 EOL 最近复核 `2026-07-28`，下次 `2026-10-28T23:59:59.999Z`，当前状态 current；证据 `docs/security-performance-hardening-2026-07-28.md`。登记例外 TypeScript 6.0.3 → 7.0.2 因 Vue 工具链 API 不兼容暂缓，@types/node 24.19.0 → 26.6.3 因当前 Node24 LTS 运行时暂缓；KPanel base maintenance 于 2026-10-15 复核，缓解/退出条件/原回滚点保留在 dependency-policy.json。传递依赖 latest 作为直接依赖归属信号，不逐项强升。

采用时限复用策略（启动/决策/处置上限）：紧急安全 1/3/3 天且服从第 11 节更严格时限；兼容补丁 7/14/30 天；minor 14/30/60 天；major/工具链/基座 30/90/90 天。逐项首次检测时点和完整行动列表未下载，不臆算各组件到期日。构建维护负责人于 2026-10-16 或稳定提升前补齐报告原件及到期行动处置，以先发生者为准。

## CI 与公开产物

### 候选 CI

- CI: [37921083027](https://github.com/kejilion/KPanel/actions/runs/37921083027)（success；2026-10-09T11:12:00Z）
- Dependency freshness: [37921083092](https://github.com/kejilion/KPanel/actions/runs/37921083092)（success；2026-10-09T11:01:13Z）

### 主线 CI

- CI: [37922548896](https://github.com/kejilion/KPanel/actions/runs/37922548896)（success；2026-10-09T11:25:47Z）
- Dependency freshness: [37922548902](https://github.com/kejilion/KPanel/actions/runs/37922548902)（success；2026-10-09T11:15:43Z）

### 标签工作流

- Release: [37923717643](https://github.com/kejilion/KPanel/actions/runs/37923717643)（success；2026-10-09T11:39:18Z）
- Dependency freshness: [37923717681](https://github.com/kejilion/KPanel/actions/runs/37923717681)（success；2026-10-09T11:26:56Z）

GitHub draft=false、prerelease=true，Latest 仍 `v1.25.0`。Docker `1.26.0-rc.1` 与 `preview` index 相同：`sha256:7a628d96e42fb23898b274bde0b384c35179574d82cbab16aa360da9a8f58660`；稳定 latest 未变化：`sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`。平台 manifest/config 完整记录见 `public-after-r1/result.json`。

Linux/amd64 manifest `sha256:04a395e1e80d27f8dba9a18ec9e1f373f69acf498b0437822235c8179acc2fa3`；Linux/arm64 manifest `sha256:81e659a48c70e159f9066d80cc768e39c591846e5b662bea15dd17c32649ccc7`；各 config 的版本、完整源码 revision 与脚本 revision/摘要均核对。

14 个附件 / SHA256SUMS 11 条；附件 API 摘要、meta VERSION 与 license 原始字节验证通过；未把 API 证明描述为已下载所有二进制。SBOM/provenance attestation entries `2`，不把存在声明扩大为完整内容审计。公开镜像实际 `image_e2e=pass`：`public-image-e2e.json`，精确 digest、版本及受管脚本，夹具容器/网络前后清单一致。

发布说明：草稿与最终正文通过，6 条摘要 / 3 条升级提示，8/3/280 解析上限未改。公开 Panel/Agent 与实时官方上游的 8 项更新信息旅程通过：稳定来源保持 v1.25.0、预览来源选择新 RC、加入预览立即检查但不安装、旧 RC15 向前提示、当前 RC 切回 stable 不出现降级安装候选。桌面及窄屏深浅主题弹窗与 Escape 按原件范围验证，安装次数为 0；未执行实际自动安装、升级/备份回滚或 systemd/OpenRC 真机全矩阵。原件 `public-notes-result.json` 与其绑定作业。

公开更新信息作业 `arena-154-36508`，开始/结束 `2026-10-09T11:41:12.609Z` / `2026-10-09T11:41:49.129Z`，420 秒硬限时，规格 `C:\GitHub\_release-evidence\v1.26.0-rc.1/public-notes-browser-r1-spec.json` / SHA-256 `af29a375b08c3999be6f6c8c6a3b655064f1a1a3abf8903662967f155e2ab32a`；43 个原件逐项摘要回收。两次加入预览的立即检查均实际断言，安装请求为 0。三张实际 PNG 已查看；截图采用 fullPage，含临时成功通知，局部遮挡上方弹窗内容，升级提示可读。原始视口宽度和升级提示纵向边界由截图前 DOM 断言限定，不把 PNG 全页高度称为 844px 真实视口。

## 分支处置与资源回收

预览序列候选 `release/v1.26.0-candidate` 保留在产品标签提交，不归档为稳定完成。三个来源作者分支已纳入；保留活跃预览或未释放所有权的作者 worktree，不以分支旧/空闲推断可删除。Docker batch 的明确归档仍不进入候选队列；已等价集成的旧分支不重复作为功能发布。

冻结后最后一次非归档分支复查 `2026-10-09T10:58:45.936Z`，批准基线 `29f7451816a69acad9aa999dd936b1e1021f0627`；本地未推送候选也逐项核对。历史 `archive/` 未作为新候选队列。以下原分支与 tip 的最终归档/保留结果以 `branch-disposition.json` 的 SSH 引用复核及本地回执为准：

| 原本地分支 | 精确 tip | 处置分类 |
| --- | --- | --- |
| `claude/desktop-material-system` | `bb03ad3015f950a6e03330f27b3973705e9e2cc4` | 纳入本轮；作者资源保留 |
| `claude/docker-batch-actions` | `9a0954e4cb61dbcd21cb0b7c3a3cb13d065fabe0` | 用户排除；后续另行决定 |
| `claude/docker-monitor-mode` | `fac0f71c7552bb9c791aa7ea5e8fc15f2fb2c387` | 已在批准 main；不重复发布 |
| `claude/gallery-cover-move` | `c9506fdfe0a4a8607b77f2d31c7cfa41fc45d2ee` | 已在批准 main；不重复发布 |
| `claude/media-gallery` | `85982e5bca36a3b41df4ec881e583370bcc3ebd0` | 已在批准 main；不重复发布 |
| `claude/office-workspace-polish` | `b50a427e4f5fb61683d8bbcaa01ccd1aa7e3ea04` | 纳入本轮；作者资源保留 |
| `claude/share-theme-immersive` | `0be14dcd184868239e7ec5a783e736f7eeb1f13b` | 补丁等价已集成；不重复发布 |
| `claude/site-manual-ssl` | `f2c867b4b48ecc1eddb6ad60efef87f7a53c51dc` | 纳入本轮；作者资源保留 |
| `docs/release-v1.25.0-rc.6-acceptance` | `ce872fe95da69b34dd3089d5374bd99707880c04` | 已在批准 main；不重复发布 |
| `docs/release-v1.25.0-rc.9-acceptance` | `40a5b9299b4382eaa130c0b1044e1d67999b0076` | 已在批准 main；不重复发布 |
| `docs/security-audit-login-20261008` | `2c314d79358ccadf0042596b8b443ee74a9049c3` | 历史残余待原所有者处置；不加入本轮 |
| `docs/security-audit-login-validation-20261008` | `fa5a54b9c4f275e85712908575b89efd840c0310` | 历史残余待原所有者处置；不加入本轮 |
| `feature/latency-median-band` | `108162eba8287445cef707685bf3f830506668c7` | 补丁等价已集成；不重复发布 |
| `feature/office-light` | `b05136738461e369ba93aeb1ad2a3256735d506e` | 补丁等价已集成；不重复发布 |
| `feature/panel-login-notification` | `94d8b1b6dfc41d7f64d5137a2cdef14957654695` | 补丁等价已集成；不重复发布 |
| `fix/backup-archive-data-path` | `96d97075a449f7681c383f400094b2b1643ac930` | 历史残余待原所有者处置；不加入本轮 |
| `fix/desktop-site-status` | `10ba3d76d2fa242b6454e04d60767160b2a69323` | 已在批准 main；不重复发布 |
| `fix/v126-rc1-go-security` | `861a3335a4e2f5263b0d32942da89c3f5becfe4b` | 本轮已纳入；发布方精确归档 |
| `fix/v126-rc1-office-motion-token` | `5d4be342f5c76f78cd977612e170151bdcd7619c` | 本轮已纳入；发布方精确归档 |
| `main` | `29f7451816a69acad9aa999dd936b1e1021f0627` | 管理工作树保留 |
| `release/v1.26.0-candidate` | `861a3335a4e2f5263b0d32942da89c3f5becfe4b` | 本轮预览候选保留 |

作者资源及历史残余的责任人为原任务所有者；所有权释放、精确提交变化或下一轮候选筛选时复核。未释放所有权的分支与 worktree 标为保留/本地待处置，不能据此解释为尚有待发布功能。发布方自有修复 tip 通过不可变 v1.26.0-rc.1 标签可达，归档引用只读保留；当前候选及作者预览受保护。

本记录写入前，自有 Go/动效修复本地分支已保存至各自 `archive/fix/v126-rc1-*` 精确 tip，脚本本地引用亦精确归档；仅改 Git 引用，工作树文件保留。配套脚本远端原子归档 `archive/release/kpanel-v1.26.0-rc.1-script` / `1800d955f216aebd2776674368a8e469a671c489` 已 SSH 复核。四个自有临时工作树的删除先被 Windows -File 策略拒绝，后续原生命令又在进程创建前被自动审批复核拒绝；精确理由仅为 blocked by policy，未提供细分原因。四个目录全部保留，实际删除数 0，未改变执行策略、强删或改用其他删除后端。其资格化盘点显示 clean、无进程、无链接/嵌套仓库，不能用这些条件覆盖审批拒绝。

原始回执 `owned-worktree-cleanup-launch-r1.json` / `owned-worktree-native-policy-denial-r1.json`，完整未执行命令及摘要单独保留；原始 native stderr 不存在，因为命令未启动，不伪造原件。引用归档回执 `owned-local-archive-result.json`，保留处置 `owned-worktree-cleanup-result.json`；两个自有本地预览已停止，见 `owned-preview-stop-result.json`。验收工作树和本轮可复核 L3 源码/恢复 bundle 保留，未把资源收尾写成全部删除。发布负责人于 2026-10-10 或下一次资源收尾时复核，退出条件为获准的原生清理入口或继续明确保留。

发布方修复/脚本分支、文档候选归档及 owned 临时资源回收，以外部 `branch-disposition.json` / 最终 closeout 的实际 SSH 引用、SHA lease、Git 状态和恢复位置为准。本记录提交时文档候选/main CI 与归档尚待执行，不预写通过。文档收尾只改本文件、当前事实入口与 run31 元数据；不可变产品标签不重打。

154 磁盘深度回收经用户原话“深度清理一波”授权，只清理闲置 BuildKit 缓存。此前 24h 波次实际释放 2,660,749,312 bytes；深度波次实际释放 7783747584 bytes，结束可用 12038905856 bytes；运行/全部容器、镜像、数据卷清单均不变。冻结重试使用每 2 秒资源守护及 512 MiB 停止线。其他任务与当前/上一稳定唯一证据不清理。

## 生产与回滚

不适用（预览版禁止生产部署）。本次无生产安装/升级、正式数据修改、生产备份或部署安全核对；没有连接 prod-108/108。arena-154 仅执行登记的隔离候选验证与公共产物验收，不用这些结果替代生产证据。

回滚方案使用 v1.25.0 精确镜像和其内置脚本/原备份；必要时另行显式授权配套脚本回滚，不自动改写 sh main、不自动降级。当前 GitHub Latest、Docker latest 与应用市场默认入口仍指向稳定 v1.25.0；本轮未执行回滚。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-09T10:27:12+08:00
- 候选冻结时间：2026-10-09T10:22:28.296Z
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：26
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

记录口径为本次已留存的执行/取证异常，非全部历史任务推断。正常产品动效断言、可达安全门禁及真实 GPU 预算失败在质量证据保留，不为降低计数改成通过；这些产品结果不自动计作流程异常。所有以下事件均在生产写入前，生产写入次数为 0。

流程复核已读取当前失败原件并比较最近 5 个正式验收 v1.25.0 至 v1.21.0，摘要及路径见 `process-history-review-r5.json`（r1/r2/r3/r4 原件保留）。CRLF staging、noexec scratch 与 SSH quoting 复用历史 fingerprint，Go 预检错误由原始 docker run 命令纠正为容器 Runner 缺少 Go。两次 Windows 清理失败复用 v1.24.0 的 file-entry execution-policy 与 automatic-policy-denial fingerprint，保留 2026-10-10 复核期限；两处历史版本字段格式被指标门禁拦截后按原 tag 修正，首次错误文档和日志保留；原台账计数不降低，包含两次文档检查失败共 26 次。第二轮未报告标记语法错误按唯一入口契约修正，预览禁止生产部署的说明保留在正文。重复 CRLF/noexec 的本轮外部入口恢复不等于共享唯一入口永久修复，后者须在下一次 L3 生产写操作前完成；发布负责人于 2026-10-16 或生产准入前复核，以先发生者为准。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "script-qualification/paired-smoke/openssl-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮夹具 Runner 工具预检失败，未计有效脚本验收。",
    "recoveryEvidence": "script-qualification-r1/remote/result.json；r7 精确 1800d955 资格化 passed。",
    "permanentAction": "发布负责人；2026-10-16 复核；退出条件：下一次脚本验收先固定并资格化 openssl 等完整工具清单。",
    "historicalReleases": []
  },
  {
    "fingerprint": "appmarket-fixture/staging/crlf-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 源码归档含 CRLF，Bash syntax 门禁拒绝。",
    "recoveryEvidence": "script-qualification-r2/remote/result.json；r7 原始 Git 字节、同步与摘要验证通过。；按最近 5 个正式验收的同类根因复用既有 fingerprint，原命名及次数保留于 process-incidents-before-history-review-r1.json。",
    "permanentAction": "发布负责人；2026-10-16 复核；退出条件：归档入口从精确 Git blob 保留原始字节并先做语法预检。 当前外部入口资格化只构成本轮恢复；共享唯一入口的永久修复仍须在下次 L3 生产写操作前完成，不声称已经修复全局流程。",
    "historicalReleases": [
      "v1.24.0",
      "v1.22.0"
    ]
  },
  {
    "fingerprint": "script-smoke/isolated-fixture/noexec-scratch",
    "position": "before-production-write",
    "count": 2,
    "impact": "r3/r4 的 tmpfs noexec 导致两个必需 smoke 尝试失败。",
    "recoveryEvidence": "script-qualification-r3/remote/result.json、script-qualification-r4/remote/result.json；r7 完整通过。；按最近 5 个正式验收的同类根因复用既有 fingerprint，原命名及次数保留于 process-incidents-before-history-review-r1.json。",
    "permanentAction": "发布负责人；2026-10-16 复核；退出条件：复用明确可执行的有界夹具挂载并在全部 smoke 前检查执行语义。 当前外部入口资格化只构成本轮恢复；共享唯一入口的永久修复仍须在下次 L3 生产写操作前完成，不声称已经修复全局流程。",
    "historicalReleases": [
      "v1.24.0",
      "v1.23.0"
    ]
  },
  {
    "fingerprint": "script-qualification/paired-smoke/xxd-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "r5 网络夹具缺少 xxd，未形成有效完整结果。",
    "recoveryEvidence": "script-qualification-r5/remote/result.json；r7 全套结果 passed。",
    "permanentAction": "发布负责人；2026-10-16 复核；退出条件：固定完整工具清单，工具不足在启动全套 smoke 前拒绝。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ssh-runner-identity/powershell-remote-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "内嵌 Python 多层引号运输失败，预检需重做。",
    "recoveryEvidence": "原始失败工具输出未单独落盘，不补造日志；后续 preflight-final-r1 原始记录与冻结方案使用 SSH stdin。；按最近 5 个正式验收的同类根因复用既有 fingerprint，原命名及次数保留于 process-incidents-before-history-review-r1.json。",
    "permanentAction": "当前外部执行入口已改用 argv 与 SSH stdin；发布负责人 2026-10-16 复核，退出条件为下一轮仅使用已资格化入口并保存失败原件。 当前外部入口资格化只构成本轮恢复；共享唯一入口的永久修复仍须在下次 L3 生产写操作前完成，不声称已经修复全局流程。",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "execution-preflight/fixed-runner/go-unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次工具预检在固定容器 Runner sha256:6f1e654d… 中运行，Go 不存在而退出 127；原台账“宿主机未安装”的归因由原始命令纠正。",
    "recoveryEvidence": "preflight-remote.log 原始 CalledProcessError 明确 docker run 与 Runner ID；后续 Go1.27.2 固定 Runner 20667a9f 的工具资格化、资源预检及完整 L3 r2 通过。",
    "permanentAction": "当前冻结 Runner 提供完整工具链；发布负责人 2026-10-16 复核，退出条件为首次工具预检精确匹配发布用途及 Runner ID，不以宿主机或其他用途镜像的能力推断。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-governance/canonical-release-l3/provider-reason-field",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次 L3 被治理健康门禁拒绝；第二次本地多命令还掩盖失败退出码，随后纠正。",
    "recoveryEvidence": "l3-r1-failure.json、l3-r2-failure.json；3a1f4e0e 在规范字段补充实际不可用原因。",
    "permanentAction": "3a1f4e0e 修复字段；后续本地门禁单独捕获退出码。发布负责人 2026-10-16 复核，退出条件为固定入口保持失败传播。",
    "historicalReleases": []
  },
  {
    "fingerprint": "gpu-evidence/probe-dispatch/overlapping-probes",
    "position": "before-production-write",
    "count": 2,
    "impact": "重复分发造成两次无效探针尝试，保留原件并按独占锁重放；本轮最终 GPU 实测未达预设预算。",
    "recoveryEvidence": "gpu-final-r3 原始失败/重试与独占锁记录；gpu-final-r4/result.json、gpu-preview-admission-r5.json。",
    "permanentAction": "外部探针使用独占锁与不可覆盖输出；发布负责人 2026-10-16 复核。后续使用隔离 GPU 环境，退出条件为单作业、完整清理及真实原件核验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cf-evidence/official-findings-validator/single-object-serialization",
    "position": "before-production-write",
    "count": 1,
    "impact": "单 finding 被序列化为对象，官方 validator 首次拒绝。",
    "recoveryEvidence": "cf-ssl-run31/validator-results.json 记录首次失败与纠正后 PASS: 1 findings valid。",
    "permanentAction": "审计负责人；2026-10-16 复核；退出条件：传输前保留数组结构并先执行可信官方结构验证器。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cf-closeout/security-boundary-audit/final-critic-deadline",
    "position": "before-production-write",
    "count": 1,
    "impact": "最终独立 critic 未在 2520 秒截止前返回，run31 保持 incomplete，不计完成覆盖。",
    "recoveryEvidence": "cf-ssl-run31/REPORT.md、run-metadata.json；security-audit-admission-r5.json。",
    "permanentAction": "审计及发布负责人；2026-10-16 或稳定提升前复核，以先发生者为准；退出条件：重新满足覆盖准入和所需独立收尾，不追认越时结果。",
    "historicalReleases": []
  },
  {
    "fingerprint": "apps-contract/public-read/windows-node-fetch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows Node 公共读取失败，首次契约取证无效。",
    "recoveryEvidence": "check-app-contract 原始失败输出；apps-contract-final-r6.json 为 SSH 远端 urllib 新鲜读取通过。",
    "permanentAction": "外部公开查询入口已使用 arena-154 urllib；发布负责人 2026-10-16 复核，退出条件为复用已资格化的只读运输并保存原始 HTTP 结果。",
    "historicalReleases": []
  },
  {
    "fingerprint": "l3-resources/canonical-release-l3/disk-peak-underestimated",
    "position": "before-production-write",
    "count": 1,
    "impact": "861a3335 首次 L3 磁盘耗尽，停止精确拥有的 Runner，终态 failed/137，不计门禁通过。",
    "recoveryEvidence": "go-security-fix/l3-resource-stop-r1.json；v1.26.0-rc.1-861a3335-l3-r1/recovery-result.json；冻结 r6 和独占资源守护。",
    "permanentAction": "用户授权缓存回收后以约 12 GB 余量重试，增加每 2 秒采样及 512 MiB 停止保护；发布负责人 2026-10-16 复核，退出条件为验证冷缓存峰值并纳入启动预检。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cache-recovery/authorized-buildx-prune/reclaim-estimate-shortfall",
    "position": "before-production-write",
    "count": 1,
    "impact": "24 小时过滤的清理命令成功，但实际仅释放约 2.66 GB，未达到 8 GB 资源预留条件，资格化状态 failed。",
    "recoveryEvidence": "go-security-fix/cache-cleanup-result.json；用户追加深度清理后 deep-cache-cleanup-result.json passed，清单不变。",
    "permanentAction": "发布负责人；2026-10-16 复核；退出条件：仅以实际净释放量和最终可用空间判定资源准入，不以缓存报告逻辑大小代替。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-notes/repository-bash-adapter/windows-path-js-escape",
    "position": "before-production-write",
    "count": 1,
    "impact": "草稿 r3 将 Windows 路径直接嵌入 JS 字符串，Go 未找到，退出 127。",
    "recoveryEvidence": "final-draft-r3/failure.json、failed-DRAFT_NOTES.md；freeze-notes-supplement-r5.json 记录 r4 实际通过。",
    "permanentAction": "外部入口已规范化正斜杠路径并使用 argv 传参；发布负责人 2026-10-16 复核，退出条件为路径资格化通过后再渲染说明。",
    "historicalReleases": []
  },
  {
    "fingerprint": "ocr-evidence/ocr-delegate/tool-version-recording",
    "position": "before-production-write",
    "count": 1,
    "impact": "外部记录及 35dcc3df trailer 的工具版本错误写为 0.3.0；实际固定工具为 1.12.11。",
    "recoveryEvidence": "go-security-fix/candidate-r2.json 与 861a3335 纠正提交；树与评审结论不变，错误原件保留。",
    "permanentAction": "861a3335 仅纠正收据 trailer；发布负责人 2026-10-16 复核，退出条件为工具版本从实际 delegate 资格化结果自动取得。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-runtime/background-browser-validation/playwright-package-resolution",
    "position": "before-production-write",
    "count": 2,
    "impact": "affected-ui-r7/r8 两次作业分别在 CommonJS package 与 package.json 版本读取时失败，均未执行界面断言；原件与清理回执保留。",
    "recoveryEvidence": "affected-ui-r7/recovery.json、affected-ui-r8/recovery.json；r9 同时资格化固定版本元数据、CommonJS 和 ESM loader。",
    "permanentAction": "外部夹具加载路径已修正并增加执行前 loader 资格化；发布负责人 2026-10-16 复核，退出条件为后续复用完整固定 Runner 工具预检。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-fixture/affected-office/download-button-ambiguity",
    "position": "before-production-write",
    "count": 1,
    "impact": "affected-ui-r9 已通过 49 项；不支持格式下载用例定位器匹配 Office 面板与媒体标题栏两个同名按钮，严格模式拒绝。",
    "recoveryEvidence": "affected-ui-r9/recovery.json、office-ssl-result.json 和原始 trace；r10 将按钮限定到 .office-workspace，保留可见性断言和完整 54 项矩阵。",
    "permanentAction": "外部夹具定位器已限定到对应工作区；发布负责人 2026-10-16 复核，退出条件为同名操作始终使用语义范围且原断言不弱化。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/office-theme/server-appearance-not-qualified",
    "position": "before-production-write",
    "count": 1,
    "impact": "affected-ui-r10 脚本终态通过，但截图复核发现 Office 浅色用例仍为深色，主题覆盖无效，不计完整矩阵通过。",
    "recoveryEvidence": "affected-ui-r10/visual-qualification.json 与原始截图/trace；r11 在每项场景前写入 Mock 外观设置，并在导航及结束时断言实际 DOM theme。",
    "permanentAction": "外部 Office 夹具已以服务端外观真源同步主题并增加实际主题断言；发布负责人 2026-10-16 复核，退出条件为主题截图与 DOM 证据一致，禁止只以场景名称推断覆盖。",
    "historicalReleases": []
  },
  {
    "fingerprint": "resource-cleanup/powershell-file-entry/execution-policy",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows PowerShell -File 被本机脚本执行策略拒绝，脚本未启动，未到达任何工作树删除。既有公开产品与脚本远端归档不受影响。",
    "recoveryEvidence": "owned-worktree-cleanup-launch-r1.json 保留工具返回原文；后续原生 PowerShell 入口的实际结果单独记录，原系统执行策略保持不变。",
    "permanentAction": "发布负责人于 2026-10-10 或下一次 L3 生产写操作前复核，以先发生者为准；固定共享清理入口的 Windows 原生执行资格化。外部入口恢复不冒充共享永久修复；自动审批拒绝时保留目标并报告，不改执行策略或强删。",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "resource-cleanup/native-powershell-entry/automatic-policy-denial",
    "position": "before-production-write",
    "count": 1,
    "impact": "自动审批复核在进程创建前拒绝原生 PowerShell 的四个自有工作树清理命令，仅返回 blocked by policy，没有细分原因。所有目标保留，未继续更换后端或执行策略删除。",
    "recoveryEvidence": "owned-worktree-native-policy-denial-r1.json 与原始待执行命令/摘要；产品公开、真实镜像与 8 项更新信息验收已通过。保留工作树，仅完成安全的精确 Git 引用归档。",
    "permanentAction": "发布负责人保留策略拦截资源并报告精确原因；2026-10-10 或下一次资源收尾时复核，退出条件为获准的原生清理入口或继续明确保留。禁止改变执行策略、强删或通过其他工具隐藏同一删除操作。",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "release-acceptance/report-release-metrics/historical-tag-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮文档指标门禁拒绝两个 historicalReleases 字段，外部历史复核错误使用了验收文件路径而非稳定版本标签。检查在文档提交、推送前失败；已公开产品与镜像不受影响。",
    "recoveryEvidence": "docs-metrics.log、docs-metrics-execution.json 保留实际失败；process-incidents-before-metrics-repair-r1.json、acceptance-before-metrics-repair-r1.md 保留错误原件。按实际历史记录的 tag 字段修正为 v1.24.0，随后以新 r2 日志运行同一指标门禁。",
    "permanentAction": "发布负责人于 2026-10-10 或下一次 L3 生产写操作前复核，以先发生者为准；退出条件为外部验收生成入口明确区分版本 tag 与证据 path，并在提交前通过未改变的完整指标契约。当前记录修正不声称共享入口永久修复。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-acceptance/report-release-metrics/unreported-marker-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "第二轮文档指标检查接受已修正的历史版本字段后，拒绝“生产完成时间：不适用（预览版禁止生产部署）”；括号说明不属于唯一入口接受的显式未报告标记。文档仍未提交或推送。",
    "recoveryEvidence": "docs-metrics-r2.log、docs-metrics-r2-execution.json 和 acceptance-before-preview-marker-repair-r1.md 保留原件；依据 report-release-metrics.mjs 的 EMPTY_VALUE 契约，将指标值改为精确“不适用”，预览版禁止生产部署的事实保留在正文。随后使用独立 r3 日志执行同一门禁。",
    "permanentAction": "发布负责人于 2026-10-10 或下一次 L3 生产写操作前复核，以先发生者为准；退出条件为外部验收生成入口使用完整规范字段语法并在提交前通过未改变的指标门禁。当前字段修正不声称共享入口永久修复。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

原始证据统一保留 `C:/GitHub/_release-evidence/v1.26.0-rc.1` 与各精确 run 的目录；公共记录为脱敏结论，受控原件和首轮失败不回填或覆盖。
