# KPanel v1.25.0-rc.14 发布验收记录

日期：2026-10-08

发布级别：L3。**预览产物已发布，生产未部署；生产实例版本未验证。**

产品提交 / 标签：`289c5d51c60fe55535829bb4862c95b05c3b469d` / `v1.25.0-rc.14`；tree `793c137e2e2658fe70849a8b1b8d61b91527d2e1`；公开时间 `2026-10-08T12:45:25Z`。

`releaseChannel=preview`；`releaseTrain=1.25.0`。RC 候选 `release/v1.25.0-candidate` 保留在该产品 SHA；GitHub Release 为非 draft、prerelease、非 Latest。版本与 preview OCI index `sha256:835f66c11d4bfd46676d2096587718320bdd9d8d3b6d1409d32ba7ca425b79dd`。

## 发布画像与功能范围

本版组合六组已经提交的候选，覆盖文件任务索引、文件/桌面新建与预览、目录定位、手机组件布局及触屏焦点。基础 API 动作、Agent 权限、安装/更新契约、Compose、应用市场和受管脚本不变；没有新增任意执行入口。文件和目录创建沿用已有受控写入流程。

- **新增**：桌面空白处、文件管理工具栏与空白处共用“新建”菜单，可创建文件夹及 txt、md、sh、json、yml、conf、py 文件；提供基础模板、名称和扩展名校验，创建后选中新项目，便于继续操作。
- **修复**：修复旧跨主机任务索引与统一文件接收索引格式冲突导致远程下载不可用的问题。统一任务改用版本化索引，优先迁入有效的 RC13 记录，再兼容旧下载记录；保留旧文件，不自动重放历史跨主机任务。
- **修复**：桌面文件直接进入与文件管理一致的预览窗口，加载和重试不再闪现目录或临时窗口；“打开所在目录”会分页查找、选中并定位文件，新建项目的定位限定在当前文件窗口。
- **修复**：手机桌面组件页与图标页保持相同工作区高度，小组件按内容增高，并缩小与底栏之间的留白，翻页时不跳动。
- **修复**：触屏主指针设备打开登录、弹窗、搜索、编辑器和终端时减少程序化输入聚焦，避免自动弹出键盘；电脑键盘焦点与用户主动点按输入保持可用。

公开 Release 经实际 RC14 Agent 解析的升级提示：

- 升级保留配置及旧任务索引；新通用任务保存在 file-transfers/jobs-v1.json。未知、损坏、不可读或链接索引仍关闭失败，不覆盖原件；回滚前备份状态，RC13 的旧格式冲突不随回滚消失。
- 新建 sh、py 只写入基础内容，不自动执行；新建文件和文件夹只选中、不自动打开。触屏行为按主指针能力判断，真实手机系统键盘及完整平台矩阵仍未验证。
- 本版仅用于预览，Docker 批处理未纳入；继承的 CF 审计仍未完整完成，WAN、10 GiB、突然断电未验证。统一接收的性能与约两倍目录临时空间限制继续沿用 RC13；退出预览不会自动降级。

Docker 批处理按用户“Docker 批处理 不汇入预览版”排除。既有 `refs/heads/archive/feature/docker-container-batch-actions` / `dca6305b9d631aeea5b12eebffa6b6752b731d5a` 未修改，且不是本产品祖先；不算本轮新归档。识别历史跨主机任务索引只是迁移兼容，不等于汇入 Docker 批处理。

## 来源、集成与分支处置

批准 main 为 `d67aa101b976977787e304888fb9fb33d7702267`。六个原始 source tip 均作为祖先保留，no-ff 集成无冲突，映射在 integration-map.json。发布负责人补三项最小修正：`fb63cf8ea83fc1dde66421ac5c721493197e3e6d` 复用现有有界分页和当前 FilesView DOM 范围定位新项；`db5a30c64baec994ee9631efa7c116f585e45b8b` 更新一条已有共享“新建”标签断言；`94fb8cf2a12c4e3ed550429c1c990ac1500f374d` 补英/繁各两个已有页面词条。未添加新的仓库测试、依赖或无关重构。

| 来源分支 | 原始精确 tip | 处置分类 | 归档 ref / 同 SHA | 远端与本地结果 |
| --- | --- | --- | --- | --- |
| `fix/remote-download-storage-recovery` | `decf9a67c27c0e44b69356b1c4e7eefe4d687fee` | included-original-ancestry | `refs/heads/archive/fix/remote-download-storage-recovery` | absent; original source was local only；clean detached，目录保留 |
| `codex/mobile-widget-gap` | `3d759e3a3e309dc9cc4cdcd41a4dd63fe1b9fd41` | included-original-ancestry | `refs/heads/archive/codex/mobile-widget-gap` | absent; original source was local only；clean detached，目录保留 |
| `codex/desktop-preview-loading-window` | `8d4237ef9dfb5f48c99f111c651c8a41a0061467` | included-original-ancestry | `refs/heads/archive/codex/desktop-preview-loading-window` | absent; original source was local only；clean detached，目录保留 |
| `claude/files-reveal-selection` | `039154ccce8d1229c9a2cac744c336952e302a97` | included-original-ancestry | `refs/heads/archive/claude/files-reveal-selection` | absent; original source was local only；clean detached，目录保留 |
| `claude/mobile-no-autofocus` | `d40dd09953d4c5c500bb9248343217ad3a3a8cb0` | included-original-ancestry | `refs/heads/archive/claude/mobile-no-autofocus` | absent; original source was local only；clean detached，目录保留 |
| `claude/desktop-new-menu-wt` | `3d342f73529dcf3298eb70fb68ad3b696fe96a54` | included-original-ancestry | `refs/heads/archive/claude/desktop-new-menu-wt` | absent; original source was local only；clean detached，目录保留 |
| `codex/mobile-widget-scroll-simplify` | `576d99575843cb0f1d35c44963766069a3866d58` | replaced-not-separately-merged | `refs/heads/archive/codex/mobile-widget-scroll-simplify` | absent; original source was local only；clean detached，目录保留 |
| `fix/release-rc14-ui-integration` | `94fb8cf2a12c4e3ed550429c1c990ac1500f374d` | included-release-owner-corrections | `refs/heads/archive/fix/release-rc14-ui-integration` | absent; original source was local only；clean detached，目录保留 |

旧手机滚动简化分支与替代提交 `f0914f7b9fdc8cdd66141b10ace2fc65292eab4a` stable patch ID 均为 `365b8f7eb1b6ec5b63f6fe36e7d67f85acd2facc`，标记被替代，未重复合并。原作者结束记录与交付快照已核对，接管后未召回作者；归档前复核 clean、精确 tip、所有权与远端引用，使用 expected-SHA 本地事务。原本未推送的来源不制造远端 archive。工作树均物理保留、解除 active ref，私有报告不新推送。

原管理目录 `C:/GitHub/kejilion-panel` 的触屏候选先保留；原作者 end_turn、精确提交和 clean 状态复读后只按已交付快照归档。没有 reset、stash、强制切换或覆盖未知文件。最终管理目录同步与两个文档 CI 的精确结果另存 docs-closeout-result.json/management-sync-result.json。

验收记录及当前事实入口使用独立 `docs/release-v1.25.0-rc.14-acceptance` 文档候选，只改两个 docs 路径；在同 SHA 文档候选和 main CI 成功后单独归档。Dependency freshness 是否触发由实际 push 路径和工作流 filters 推导，不声称未触发的作业已通过。产品 tag、镜像、Release 正文均保持不可变。

无关旧分支、登录 CF 进行中任务和其他作者的保留预览不纳入、不清理。本版没有新增归档待办；后续预览仍使用唯一 RC 候选。

## 多维质量结论

| 维度 | 状态 | 实际证据 | 范围与未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确 L3，实际隔离 Panel/Agent 的 17 项文件/桌面/触屏旅程，公开镜像 E2E | 多节点、外部 SFTP/WAN 和全平台矩阵未验证 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 固定供应链/镜像扫描、govulncheck、npm audit、同 SHA CI；CF 仅覆盖检查 | scoped-required；未完成 CF 不能推定无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证 | Linux 旧索引拒绝/保留回归、关闭生命周期；L3 安装/更新/备份/中断恢复契约 | 真机 systemd 生产升级回滚、断电、长期 soak 未验证 |
| 性能与资源预算 | 已验证 | Panel/Agent 1 CPU/256 MiB/128 PID，browser 2 CPU/1 GiB/256 PID，watchdog/OOM 通过；历史 RC13 对比保留 | 没有新性能改进结论；历史小文件退步与目录约两倍临时空间仍在 |
| 用户体验与可访问性 | 已验证 | 双窗口分页定位、正常文件预览、深色桌面/浅色触屏/深色横屏，真实更新弹窗桌面及窄屏双色，Escape/手动焦点 | Chromium 合成触屏；真实手机键盘、Safari、原生 125%/200% 缩放和完整语言矩阵未验证 |
| 数据、配置与迁移 | 已验证 | 原旧索引 bytes 不变、独立 jobs-v1.json；8 类新建与实际字节、目录及桌面快捷方式持久化 | 历史跨主机批量记录不信任、不导入或重放；生产存量和其他 NAS 未验证 |

## L3、源码与独立复核

- 唯一外层入口 `scripts/run-release-l3.mjs`；登记环境 arena-154，production=false。冻结 `2026-10-08T11:23:52.739Z`，base tag v1.24.0 / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`。
- R1 `v1.25.0-rc.14-2e326e1e-l3-r1`：failed/2，`2026-10-08T10:52:10Z` 至 `2026-10-08T11:01:21Z`。旧“新建目录”静态期望失效，1 failed / 2424 passed / 6 skipped；Go cancelled、deploy not-run。
- R2 `v1.25.0-rc.14-12d14a98-l3-r2`：failed/2，`2026-10-08T11:12:21Z` 至 `2026-10-08T11:21:37Z`。2425 测试全部通过，但生产 build 国际化门禁拒绝两条未翻译词；Go cancelled、deploy not-run。两个失败原件各 12 文件回收/hash 验证，未改称通过。
- R3 `v1.25.0-rc.14-289c5d51-l3-r3`：passed/0，`2026-10-08T11:26:00Z` 至 `2026-10-08T11:41:42Z`；固定 Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0 / npm 11.19.0 / Linux amd64。
- ` Test Files  264 passed (264)`；`      Tests  2425 passed | 6 skipped (2431)`；前端 typecheck、i18n/production build、全部 Go 单测、panel/auth/dockerx race、go vet、部署/安装安全/锁兼容/更新备份/中断恢复契约通过。web/go/deploy 均 passed，分别 web=585610 ms / go=545379 ms / deploy=4092 ms；source group 585664 ms，identity_unchanged=true。
- 原始日志 `C:/GitHub/_release-evidence/v1.25.0-rc.14-289c5d51-l3-r3/remote-evidence/l3-verify-release.log`，SHA-256 `ab862897c3709b0dbd06b747ba6dd0fc7abe1f4cdd70dadccbf0ed7684d3585a`。12 个权威远端证据文件逐项回收/hash 验证；manifest/plan/script/bundle 本地远端一致。
- 输入 SHA-256：plan `a7014204ac8e28e042931c64ace6aa08fc2d83f0683dda78f78735dc37700677`；script `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `b1f068b9f749454fbb1a99d0920cca2f208f5a236c3019b5f1001bb49c764d68`。
- 固定 Trivy 源码和最终镜像门禁通过；govulncheck/npm audit 的原始输出保留，不扩大成完整漏洞审计。linux/amd64、linux/arm64 二进制与发布镜像均完成构建。

40/40 选中文件经自由臂和 OCR 规则逐行复核，初次 34 产品文件、随后版本/现有断言及两份词条目录；未改变的 head blob 精确复用。一次 MEDIUM 的跨窗口/分页新项定位缺陷已在 fb63cf8e 修正，并由本轮真实两窗口旅程验证。约束臂增量 unreported、blind=false；没有把既有 OCR 线索改作盲发现。

Claude 来源由 Codex 不同供应商独立复核；Codex 来源由不同实现会话的同供应商回退复核，因为 claude/gemini CLI 不可调用。发布负责人自己的小修正是 L1 自审，不冒充独立供应商验证。精确 provider qualification 及 coverage 位于 review、review-r2、review-r3；完整 L3 不替代这些声明。

CF 冻结检查 `scoped-required`；待审提交口径 `53`，文件口径 `169`（权威细项见 security-coverage.json）。最近完整 run4 基线 `4c0694aa8e02e46145a775707b8d5a0355f7ce10`；run19/20/21 incomplete、run22 partial 不计覆盖。internal/atomicfile 与 internal/netpolicy 新边界仍需补审。本轮没有执行 CF scoped/full，预览只按 PROJECT_RULES 5.4 非阻断记录；稳定版另需准入审计。指定 CF gpt-6-luna/max 规则没有被改动。

## 隔离真机、浏览器和公开信息

- 来源下载恢复 L2 的 Linux 迁移与真实 URL/copy 夹具已核对；UID 65532、只读、1 CPU/512 MiB、无网络用于源迁移场景。源内存首轮失败及最终 L2 原件保留，不称为本次冻结头执行。
- native-feature-browser-r5 绑定 `289c5d51c60fe55535829bb4862c95b05c3b469d`，先对 L3 最终导出的固定 image/index ID 核对（当前 containerd image store 返回 manifest-list ID，同时保留 config SHA；version=RC14，本地 OCI revision 未由 Docker build 传入，不伪称公开 revision）。实际 Linux Panel/Agent 和专属可写 /home/rc14-ui、桌面目录执行 17 项：旧索引打开、8 类新建、分页与另一窗口滚动保持、真实延迟 metadata/body 的正常预览、目录定位、桌面新建快捷方式、触屏登录/新建弹窗焦点、组件页手势与横屏。
- native 真实持久化字节、JSON/脚本初始内容与非可执行权限、目录、新桌面文件均核对；两个旧索引 hash 原样保留，`file-transfers/jobs-v1.json` 存在。未预期页/console/HTTP 资源错误 0；桌面新建前恰好一次既有缺失路径 GET/404 与对应 console 按完整 URL、方法、状态及阶段单独记录，未放宽其他错误门禁。外层持久化 passed/0 与内部 receipt passed 同时成立，22 项原件 hash 验证。此前 r1/r3/r4 在 12 项后因脚本桌面定位失败，r2 完成 17 项后因只读夹具缺 GET /info 及预期 404 分类失败；四个 failed 原件、截图与资源清理结果保留。r5 等待实际图标/网格就绪，再通过物理鼠标在 NAV.desktop__icons (240,680) 右键；完整命中图记录在 blank-desktop-target.json。
- public-notes-browser-r1 使用实际公开 RC14 与前版 RC13 image digest。RC14 API 返回正确 preview/version/digest/官方 URL、5 条摘要、3 条升级提示；实际 RC13 设置页加入预览、候选详情/更新弹窗逐条与 API 一致。1280×900 dark、390×844 light/dark 均无横向溢出，字体至少 13px、末条可滚动可见、Escape 关闭。安装请求 0，不执行更新安装。
- Chromium 140.0.7339.16 / Playwright 1.55.0 / Node v24.18.0；browser runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`，Node/Playwright 文件 hash 固定。两作业各 420 秒外层硬超时、240 秒浏览器超时，Panel/Agent 各 1 CPU/256 MiB/128 PID；独立网络、只读根、cap 限制、私有状态和启动 token 不进入公共仓库。
- 只读 Docker 夹具提供实际 GET /_ping 与限定唯一空应用 label 的 GET /containers/json；native 作业另允许有 2 MiB 上限和字段约束的实际 GET /info，记录实际宿主 Docker 数量，不能称为虚构或空宿主；host Docker socket 不挂 Agent，写方法/其他路径在上游前拒绝。所有请求与 provider 范围、watchdog、OOM、容器/网络/私有状态清理均有 receipt，无真实应用部署或宿主升级结论。
- 公开镜像 `docker.io/kjlion/kejilion-panel@sha256:835f66c11d4bfd46676d2096587718320bdd9d8d3b6d1409d32ba7ca425b79dd` 显式 pull 后走仓库 packaging/tests/image-e2e.sh，passed/0；真实 public asset bytes 与 frozen code 一致、健康版本/反代/cookie/非 root 契约通过。自有容器和网络 before/after 一致。
- 本地 Mock 4179 绑定新头，用于 acceptance visual-composition；不是实际 Panel/Agent 或公开发布证据。最终停止和生成物回收在 cleanup-closeout-result.json；其他作者保留的 Mock 4178 不清理。

## CI、发布产物和跨仓库契约

- CI / `release/v1.25.0-candidate`: [37775298991](https://github.com/kejilion/KPanel/actions/runs/37775298991)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）
- Dependency freshness / `release/v1.25.0-candidate`: [37775298884](https://github.com/kejilion/KPanel/actions/runs/37775298884)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）
- CI / `main`: [37776299086](https://github.com/kejilion/KPanel/actions/runs/37776299086)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）
- Dependency freshness / `main`: [37776298963](https://github.com/kejilion/KPanel/actions/runs/37776298963)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）
- Release / `v1.25.0-rc.14`: [37777563182](https://github.com/kejilion/KPanel/actions/runs/37777563182)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）
- Dependency freshness / `v1.25.0-rc.14`: [37777563153](https://github.com/kejilion/KPanel/actions/runs/37777563153)（success，`289c5d51c60fe55535829bb4862c95b05c3b469d`）

GitHub Release：[v1.25.0-rc.14](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.14)；Docker：[kjlion/kejilion-panel](https://hub.docker.com/r/kjlion/kejilion-panel/tags)。14 个附件，SHA256SUMS 11 项对照 GitHub API digest；meta/许可证真实 bytes 已下载核验，没有下载所有二进制全部 bytes。amd64 `sha256:b6f4590319781a7c97416f04834fde1ab07cb4e9f94cd8b4727e9574b2506e70`；arm64 `sha256:543521ddbe7704d382a3f96a570f733e26b72188224892783b0bb739355ecb05`，附 provenance/SBOM attestations。

公开 body 与冻结产品由 canonical renderer、最终 digest 重建的正文一致，仅规范化 LF/末尾空白；normalized SHA-256 `0ba8979d37b39c845327c636d46f6a287c1848608398b62cd503851c97792d9e`。真实 API 和实际更新弹窗共同复核，不能只凭 CI 推断该旅程。

脚本联动 `scriptLinkageState=not-required`：受管脚本 `c3a8bd895f8878d9e4ced7592c91a20c974472a5` / `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`；本版不改内置脚本、安装或运行时契约。应用市场 `C:/GitHub/kejilion/apps/kpanel.conf`，commit `4fc985e964dbd1742143521d0a28ab652b885773`、原分支 `feat/lanqin-email` clean；归一化 LF SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089` 与 packaging conf 一致，无需 Apps 提交。当前候选未采用新的依赖/锁文件包版本/Action/扫描器候选；package/lock 仅版本字段变化。CI Dependency freshness 是检测证据，不能认作全部候选已升级。

## 性能、恢复与生产边界

继承 RC13 同机 1 CPU/768 MiB 三次中位数：64 MiB loopback 0.397→0.695 s（约 1.75×）；32×4 KiB 0.062→0.501 s（约 8.13×）。本轮没有重复 benchmark 或改善结论；20/100 ms 固定 handler 延迟不是 WAN RTT。目录临时空间约逻辑数据两倍；10 GiB/10000 项是预算上限，满额、WAN、真实 SFTP、断电、长期 soak 与 arm64 浏览器未验证。

生产全部动作 **不适用（预览版禁止生产部署）**：没有生产探测、部署、备份、升级或回滚；prod-108/108 禁用全部 KPanel 操作，本次未连接、未核对。隔离夹具写入只发生在专属临时目录，未写入真实宿主 Docker、systemd 或业务数据。生产实例版本/健康/状态/公网入口均未验证。

公共 GitHub Latest、Docker latest 与标准稳定更新源保持 v1.24.0，digest `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。稳定源码恢复点 `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；前预览 RC13 `f9e4f5e126087ec6841a0bb794f036062368d3ce` / `sha256:0e92dd33d05d4138a85e35f0d7aa0afd25f09b0cf9b4a6d90c0af2b1414cf34b`。本轮没有产品回滚/紧急热修复/重复公开发布；L3 失败只算冻结前验证通道重试。退出预览不自动降级；真实恢复需独立授权、先保存数据状态、固定旧 digest，再走现有恢复流程并复核数据与版本。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-08T16:08:09+08:00
- 候选冻结时间：2026-10-08T11:23:52.739Z
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个纳入时间按上一产品 tag 后最早 Git commit `8764dd675be4bc3f579661ad67b75c2773a0bfb4` 记录，包含 ancestry 中的前版验收文档，不当作新业务功能。预览公开时间与正式发布/生产频率分开，生产时间不适用。产品失败字段不适用；发布执行器、验证通道及无效证据事件另行完整计数，不因未生产而记零。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：25
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

全部异常保留首轮输出、影响和成功替换原件。R1 静态断言、R2 i18n 门禁、路径/执行器失误均在公开写入前拦截；不存在对生产恢复的虚构。最近五个正式版本 v1.24.0/v1.23.0/v1.22.0/v1.21.0/v1.20.0 按各记录 JSON 指纹复核；本轮重复路径读取失误仍完整计数。现场换命令不等于永久修复；责任发布任务，2026-10-14 且下一次 L3 生产写前复核，退出条件为相应唯一预检/夹具实现修复与回归证据。没有以这次预览降低生产门禁。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/management-worktree/non-main-feature-branch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Management qualification failed cleanly because another ended author left claude/mobile-no-autofocus d40dd099 checked out. Its source and worktree were preserved; this release uses its own qualified existing candidate worktree. Root chunk e9d0cb retained.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/source-intake/incorrect-management-tip-field",
    "position": "before-production-write",
    "count": 1,
    "impact": "Self-review caught an incorrect duplicated batch tip in the managementWorktree metadata field. Source entries and their identity checks were correct. Preserve the initial plan, correct the field to d40dd099 before assembly; no published or source state changed.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/check-collaboration-state/invalid-trailer-paragraphs",
    "position": "before-production-write",
    "count": 1,
    "impact": "Release-owner tiny fix used two trailer paragraphs. Corrected its own unpublished, unbound commit through an exact message file before merge; original message retained.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-notes-go-gate/missing-local-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "First draft rendering completed but required runtime parser failed because Go was not in Windows PATH. Retained original draft; selected existing pinned go1.27.1 path, verified version, passed canonical notes gate into DRAFT_NOTES-r2.md before freeze.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 10,
    "impact": "rg used docs/dependency* as a positional Windows path, rejected before any mutation; subsequent discovery uses literal directory and glob. Additional event: Three read-only rg invocations guessed Agent config, desktop composable and dashboard paths; discovery errors preserved in chunks 410f2c, 570e0a, 75f9b2. Subsequent reads use discovered paths. No source mutation. Additional event: A later read-only query again used scripts/source-checks*.mjs as a positional path (410f2c); use directory-scoped --glob. Repeated usage is not claimed as permanently fixed. Additional event: Repeated read-only scripts/source-checks*.mjs positional path failed (69bbcf). Exact rg --files discovery then literal run-source-checks.mjs succeeded (c82c49/24ed84). No source change; repeated practice still not permanently fixed. Additional event: Read-only guessed review-assembly.cjs path failed (0e4650); exact rg --files found review-r2.cjs and prepare-review.cjs, then read literals. Additional event: Read-only root package.json query failed because KPanel npm package is under web (18b870). Actual canonical script CLIs were read from existing literal scripts. No mutation. Additional event: Read-only assumed scripts/lib/release-metrics.mjs path was absent (a37c55). Required field authority read from existing acceptance template; use discovered canonical report-release-metrics.mjs. Additional event: Read-only native-r4 diagnostic lookup incorrectly guessed recovered/evidence directory (4a4cdd/07abcc). rg --files located actual remote-evidence; no mutation. Repeated discovery mistake is retained, not claimed fixed.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "validation/source-checks/stale-new-label-expectation",
    "position": "before-production-write",
    "count": 1,
    "impact": "Exact L3 r1 failed one obsolete literal-label expectation. Preserved failed/2, cancelled Go and not-run deploy; update only that existing assertion, re-freeze and repeat complete L3.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/coverage-helper/incomplete-version-field-whitelist",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial retry coverage guard rejected OCR-selected VERSION (2ae854). Preserve free-form, preview and rules; inspect actual added set (VERSION, internal version, web package and existing test), then finish coverage without rerunning or overwriting original receipts.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "validation/i18n-check/missing-new-file-phrases",
    "position": "before-production-write",
    "count": 1,
    "impact": "Exact L3 r2 failed production build on two phrases missing from English and Traditional Chinese catalogs. All 2425 tests passed. Raw failed/2 and cancelled/not-run lanes recovered; add four entries, i18n 3593 phrases passed, re-freeze full L3.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/i18n-check/missing-own-worktree-dependency",
    "position": "before-production-write",
    "count": 1,
    "impact": "Fast i18n gate lacked TypeScript in the clean own fix worktree. No package install or mutation; after exact FF, canonical checker in own release worktree passed existing dependency set (45328b).",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ocr-delegate/unsupported-help-command",
    "position": "before-production-write",
    "count": 1,
    "impact": "Canonical OCR delegate rejects --help; no OCR-managed LLM endpoint used. Existing exact helper supplies supported preview/rule commands (45328b).",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/read-only-log-inspection/premature-read",
    "position": "before-production-write",
    "count": 1,
    "impact": "Remote R3 log was queried before canonical prepare had created it (558b81). Preserved read failure; wait for prepare output, then authoritative log appeared. No verification success inferred.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/native-browser-preparation/wrong-docker-image-id-kind",
    "position": "before-production-write",
    "count": 1,
    "impact": "Preparation guard correctly rejected comparing Docker containerd image-store ID (30328b3d manifest list) to exported config (89e13423), before creating fixture or job. Original helper retained. Exact L3 export and inspect verified; bind manifest-list ID and separately record config SHA, no product or L3 changes (28854f/0e60d6/6322eb).",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/native-browser/incorrect-blank-desktop-target",
    "position": "before-production-write",
    "count": 3,
    "impact": "Native r1 completed 12 cases but timed out opening desktop New. Script picker omitted desktop-widget-slot/desktop__widgets while product onContextMenu correctly ignores that area (c060f9). Raw failed/1, 15 hashes and clean resource receipts retained. Use the actual complete guard selectors for blank point; full retry r2 uses unique port 18086 and namespace, same product SHA and L3. Additional event: Native r3 had zero resource/browser errors and completed 12 cases, then again could not locate desktop New after a coordinate click. Prior pointerdown-selector analysis did not cover all actual entry context handlers; original inference and 15 hashed files retained. Replace localStorage reset of live directory windows with real close buttons and persistence wait; choose only actual empty grid/background elements, exclude data-icon/pager/widget targets, and collect failure screenshots/DOM diagnostics. Full r4 keeps same product/L3, unique 18088 namespace. Additional event: Native r4 stopped after 12 journeys at an overstrict blank-element class guard. Failure screenshot shows no windows and restored empty preferences, but picker ran immediately after desktop root mounted. Preserve failed/1 and 17 verified evidence files; wait actual workspace/icon readiness and record complete point hit maps before the physical right click. Source confirms exclusions are onDesktopPointerDown, not onContextMenu; the r1 context-menu guard inference is withdrawn. No product change or invalidation of exact L3.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/native-browser/incomplete-readonly-fixture-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "Native r2 completed all 17 journeys but exact error gate failed: read-only fixture denied existing Docker summary GET /info; it also treated the deliberate one exact desktop-new missing-path GET/404 as unexpected. Recovered 20 hashed files and clean resources. Source Summary uses real /info; trace proves exact missing path and console URL. Extend only bounded GET /info actual upstream and classify exactly one armed-path GET/404 plus its exact console message; all other errors still fail. Full r3 uses unique 18087 namespace.",
    "recoveryEvidence": "Original failed commands and receipts are preserved in C:/GitHub/_release-evidence/v1.25.0-rc.14 and exact failed L3 kits. Successful replacement evidence is linked by each incident; no failed attempt is relabeled passed.",
    "permanentAction": "Owner: release task; review due 2026-10-14 and before any next L3 production write. Executor corrections and exact repeated validation are complete. Shared preflight/fixture regression repair is not claimed complete; exit requires an implemented correction and regression evidence for any recurrent preventable fingerprint.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 资源与剩余限制

失败 R1/R2 各在终态、12 文件摘要与一个自包含可恢复本地 bundle 资格化后，只移除对应自有远端生成目录和两个冗余 bundle，实际对象占用累计回收 914497930 bytes；原始 log/status/manifest/plan/hash 与本地 bundle 保留。R3 及本地依赖/Mock 的最后回收以 cleanup-closeout-result.json 的实际对象和 df 复测为准，不把预计值或共享 Docker 层变化当净释放。

来源工作树、历史失败截图/日志与恢复原件保留；没有共享 Docker prune、删除其他任务或新建清理/监控会话。发布文件记录已经完成的产品与来源归档事实，文档 CI/归档和终态资源清单在仓库外 final receipts，避免改写已发布 product tag 或记录自身 SHA。

本版准入限预览产物和上述验证范围。CF 完整覆盖、真实手机 Safari/键盘、WAN/大文件满额、长期稳定性、全平台及生产更新/回滚仍未验证；不授予稳定或生产准入。
