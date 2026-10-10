# KPanel v1.26.0-rc.2 发布验收记录

日期：2026-10-10
发布级别：L3

候选提交 / 标签：`bcc80ec5fcf752eb10e5e5392e60a824d85ff0db` / `v1.26.0-rc.2`。产品标签不可变；本记录为发布后的独立文档提交。

上一稳定版本 / 回滚点：`v1.25.0` / `sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`。

`releaseChannel`：`preview`；`releaseTrain`：`1.26.0`。产物已发布，生产未部署。

## 发布画像与范围

批准 main `e4f7e19bf8e1440d262e23355263e844e9a3dd63`；新增来源为 `feature/file-transfer-acceleration@d0bb17c877e86eac6981fa44d65642f4a8e8ae27` 与 `claude/docker-batch-actions@395163be0278bdb2ec3f27df15109dae2a6aac7e`。保留两者祖先，补充版本说明、OpenTelemetry安全依赖修复、Docker工具栏与操作栏共享动效变量兼容修复，以及既有MCP异步完成等待与BT正确性夹具时限/进度诊断修正，冻结本产品提交。精确范围见branch-inventory.json、docker-integration-result.json、motion-fix-result.json、dock-motion-fix-result.json、mcp-terminal-fix-result.json与bt-fixture-fix-result.json；旧归档Docker持久批处理方案未纳入。

- 文件远程下载新增默认开启、可关闭的智能加速。HTTP/HTTPS 按真实接收收益在 1→2 连接间试探，无收益或来源条件不满足时回退单流；全局额外缓冲上限 8 MiB。固定 4 路第一阶段原型仍默认关闭，不作为当前默认能力。
- BT 首版支持 v1 `.torrent` 文件/URL 与公开 btih magnet，只落到当前 Panel 主机。与 HTTP 共用两个任务槽，BT 暂存并发 1、任务上限 10 GiB、元数据和路径有界；验证片段后复用既有 Agent 接收和原子发布。
- 不支持 v2/hybrid、私有 magnet、做种、跨重启自动续传、远端节点 BT 或完整 BT 管理器。导入私有 `.torrent` 在发现前关闭 DHT/PEX；公开 magnet 的私有标志在元数据返回前未知，可能已参与 DHT，不能承诺私有磁力隐私。PEX 全局关闭。
- BT 在 Panel 暂存后再写入目标，文件路径约需两份空间，目录归档路径约需三份；重启后任务中断。实测不覆盖 10 GiB、真实公网长稳、极端磁盘或独立控制面响应。
- Docker新批处理覆盖容器、镜像、网络、存储卷：多选、可见范围Shift选择、组选择和右键；复用既有单资源API/CSRF/resourceVersion保护，逐项执行，失败继续，可停止剩余项与仅重试失败项。已有网络、被使用卷等按适用性跳过，删除卷数据不可恢复。KPanel自身容器提示影响并排最后；离开Docker路由停止后续派发，已发请求继续。本版没有后台持久批队列或跨刷新/重启恢复。
- 继承RC.1桌面材质、Office和手动证书续签。真实CA签发、Docker Engine多项操作全旅程与完整跨平台矩阵本次未新增准入。

未变化的 Agent 接收协议、权限、端口、Compose、安装更新契约与应用市场继续沿用；API 增加有界 BT 来源适配，新增网络/元数据与磁盘暂存边界，按 L3 发布验收。

## 外部审计、OCR 与独立复核

- 覆盖检查：decision=scoped-required；64未审计提交、179文件，最老6天；上次full run4距今18天。预览按release-kpanel v3.6记录观察；稳定提升前重新执行带--require的检查并完成必要补审。
- run32 仅针对初始 0b808e7b，状态 incomplete，后续私有种子隔离等修正不在已审覆盖内；继承 run31 也保持 incomplete。两者均不计完成覆盖，不冒称最终源码安全审计通过。
- 下载来源43/43文件与治理跟进已按精确blob核对；新Claude Docker候选由Codex发布方独立复核，合并保留批准基线API/测试与共享菜单CSS。最终OCR1.12.11覆盖63/63可评审文件，0 findings；自由臂接触来源评审，blind=false，constrained-only=unreported。bt-fixture-ocr-coverage.json逐项核对63个可评审文件，保留MCP终态等待复核，补充既有BT测试文件的12行时限与诊断差异自查。
- 下载实现与来源验证来自不同Codex任务，其他供应商CLI不可用时按provider-unavailable回退，PASS WITH FOLLOW-UP。Docker实现为Claude，Codex发布方独立核对；Otel、动效兼容、MCP与BT测试修正由发布方自查，不冒称自有修复经过独立审查。原件source-evidence-qualified.json、docker-release-free-form.json、motion-fix-review.json、dock-motion-review.json、mcp-terminal-review.json、bt-fixture-review.json与candidate-final.json。

## 跨仓库联动判定

`scriptLinkageState=not-required`；变更集编号、脚本候选与脚本发布均不适用。Dockerfile、packaging 与 Agent 接收契约相对 RC.1 不变，继续内置脚本 `1800d955f216aebd2776674368a8e469a671c489` / SHA-256 `5778fdc9637c8614f5246f9eb13c1e549de51522fb91b3805067cbc0475b614c`。
应用市场 origin/main `1809cbb09bc812ad2796ca7cec1b3f8f9a7afa1e`，归一化 kpanel.conf SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089` 与源码及本地一致；默认 latest，appsCommitRequired=false，没有 apps/sh 写入。其他作者的 feat/lanqin-email 工作树保持原状态。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 来源最终 L2/race、冻结 L3、公开镜像 E2E | BT 仅本地主机，未覆盖真实公网全矩阵 |
| 网络入侵与供应链安全 | 已验证 | L3 漏洞/Secret/配置/镜像扫描、依赖补丁、四份新增 license 字节核对 | CF run31/32 incomplete，源码边界补审未完成 |
| 稳定性、失败恢复与兼容 | 已验证 | 来源边界/取消/失败测试、最终 L3 | BT 跨重启自动恢复未实现，Safari/Firefox/真实手机未验证 |
| 性能与资源预算 | 未验证 | 原始依赖图102正式样本、34 warmups及四份数据集已核对；最终图未重测 | 历史条件性提速证据；独立控制面/WAN未验证；继承GPU预算失败 |
| 用户体验与可访问性 | 已验证 | Files与Docker各4组、合计24截图/8轨迹；实际公开更新信息8项 | Mock界面不代表真实BT网络或Docker Engine多项旅程；真实手机/完整读屏未验证 |
| 数据、配置与迁移 | 已验证 | 既有接收与原子发布测试、安装生命周期、公开镜像 | 未执行生产迁移、实际自动安装或备份回滚 |

状态仅描述表中已列证据；未验证项保留，不由部分通过推导全域准入。

## 自动门禁与固定执行身份

完整 L3 原始 run `v1.26.0-rc.2-bcc80ec5-l3-r8` 在 arena-154 的规范远端入口通过，exit_code=0，开始 `2026-10-10T05:48:15Z`，结束 `2026-10-10T06:10:24Z`。源码分组耗时：web 613713 ms；go 924857 ms；deploy 1688 ms。最终 make verify-release、双架构、场景包、镜像、供应链及 app-conf 生命周期完成。

Runner `sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`；Go1.27.2 / Node24.21.0。manifest SHA-256 `e14f2ffb83cbe3b13abecbc510b9c92aedc6cf0e256a82b824ddb3e98fc952f5`；plan `619252d4ed0d84c6a86c71a32fdaf37b8f41fd1581a723ffd448b6dac17e6ba1`；bundle `4079b90bbc8098592201ff3e1d36da399cf177f1a7ecb8f952f835f2733ad539`；远端执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。十二个原始文件全部回收并逐项摘要核对，目录 `C:/GitHub/_release-evidence/v1.26.0-rc.2-bcc80ec5-l3-r8/remote-evidence`，主日志 SHA-256 `5f0a9c636430b5b8603455dbfec4a7db9cb5644b3b5adfae2dc955378c9e1f48`。

r1外部150秒超时后远端failed/141；r2完整源码分组通过后被Trivy HIGH CVE-2026-29181拦截failed/2；修复依赖后r3因用户新增Docker范围主动取消failed/137；r4被DockerBatchBar新增硬编码动效时长拦截failed/2；r5继续被DockerView操作栏新增硬编码动效时长拦截failed/2。上述终态和各12份原件保留，均不改写为通过。两处均改用共享动效变量后r6完整L3通过；随后候选CI中既有TestMCPTrashScopeAndPartialFailure在100ms同步等待结束后仍为executing，立即断言终态导致失败，无DATA RACE报告。按同文件既有模式补充最长8秒轮询operation_status，保留失败状态、私有零派发及允许项恰好一次断言，运行代码不变。r7前端267文件/2490测试与部署检查通过，但BT目录正确性夹具在Race下31.15秒时触发其30秒上下文，终态failed/2；无DATA RACE或资源停机。原日志没有最后来源/接收阶段，具体停滞原因未确认。固定Runner、单CPU/3GiB隔离复现：原30秒源码三轮六子例全部通过（3.80至4.43秒），单独120秒诊断覆盖三轮六子例也通过（3.68至4.33秒）；诊断不是发布准入，也未证明原超时确切原因。现有12MiB正确性夹具改为有界120秒并保留最后非错误进度/耗时，字节、ACK重试、原子发布、清理断言和产品性能预算不变；[Go官方Race说明](https://go.dev/doc/articles/race_detector#Runtime_Overhead)仅支持一般开销背景。r8随后从规范入口完整重跑，本地CLI与远端终态均0；原r7失败、旧CI失败和成功r6原件保留。资源守护每两秒检查，最低磁盘1581359104 bytes、最低可用内存2282098688 bytes，未触发停止。

### 候选 CI

- [CI 38030120719](https://github.com/kejilion/KPanel/actions/runs/38030120719)：success，精确提交 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`。

### 主线 CI

- [CI 38030637736](https://github.com/kejilion/KPanel/actions/runs/38030637736)：success，精确提交 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`。
- [Dependency freshness 38030637735](https://github.com/kejilion/KPanel/actions/runs/38030637735)：success，精确提交 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`。

### 标签工作流

- [Release 38031372848](https://github.com/kejilion/KPanel/actions/runs/38031372848)：success，精确提交 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`。
- [Dependency freshness 38031372890](https://github.com/kejilion/KPanel/actions/runs/38031372890)：success，精确提交 `bcc80ec5fcf752eb10e5e5392e60a824d85ff0db`。

发布说明草稿与最终正文通过；目标 1.26.0-rc.2，8 条摘要、3 条升级提示，8/3/280 解析上限未修改。

## 依赖与技术栈

采用torrent1.61.0、WebSocket1.5.3、DTLS3.1.4、STUN3.1.5，实际go.mod/go.sum锁定；OpenTelemetry系列由1.38.0修复至1.47.0，包含otel/log/metric/trace及必要logr1.4.4，auto/sdk1.2.1不变。[上游安全公告](https://github.com/open-telemetry/opentelemetry-go/security/advisories/GHSA-mh2q-q3fh-2475)与[1.47.0发布](https://github.com/open-telemetry/opentelemetry-go/releases/tag/v1.47.0)说明修复背景。Go1.27.2/Node24.21.0、基础镜像、Action、固定扫描器与受管脚本继承RC.1。新增dht/log/torrent MPL-2.0和x/time BSD-3-Clause原文，公开meta内四份license逐字节一致；THIRD_PARTY_NOTICES不冒称完整传递清单。

当前最终L3与候选/main的依赖安全门禁通过；Dependency freshness成功为候选检测，不表示全部建议已采用。最后候选推送仅改两个精确既有Go测试文件，按实际路径过滤没有新Dependency freshness作业；原候选937f73ec成功报告通过完全相同的检测器、工作流、策略和依赖输入明确资格化复用，原作业38024380985仍绑定原SHA，未冒称最终候选触发了该工作流。最终main/tag的实际报告绑定新完整SHA。原候选/main/tag的实际report生成窗口依次为 `2026-10-10T04:32:09Z 至 2026-10-10T04:32:31Z`；`2026-10-10T06:21:40Z 至 2026-10-10T06:21:52Z`；`2026-10-10T06:34:15Z 至 2026-10-10T06:34:23Z`。按实际成功步骤与固定CLI任一检测源失败退出2、到期维护退出3且未使用allow-partial的契约，推导10/10检测源完成；报告Markdown与完整候选行动清单未下载，不冒称逐行读取或臆算首次检测/逐组件到期。最近每日安全通告为 [37912308544](https://github.com/kejilion/KPanel/actions/runs/37912308544)，failure，结束`2026-10-09T09:36:54Z`；未回收其失败日志时不推断原因。EOL状态 current，最近/下次复核 2026-07-28 / 2026-10-28T23:59:59.999Z，原件dependency-ci-qualification.json。

继承 TypeScript6.0.3→7.0.2 因 Vue 工具链 API 不兼容、@types/node24.19.0→26.6.3 因 Node24 LTS 暂缓；构建维护负责人 2026-10-15 复核。Trivy0.74.0→0.75.0 继承专项评估，2026-10-16 或稳定提升前复核。采用时限按策略：紧急安全1/3/3天、补丁7/14/30天、minor14/30/60天、major/基座30/90/90天；精确行动项沿用 dependency-policy.json，不机械强升传递依赖。

## 隔离浏览器与性能

arena-154登记候选与浏览器用途。Files作业arena-154-37992与Docker作业arena-154-20172原始作业均绑定937f73ec，各通过4组、12PNG和4轨迹；最终bcc80ec5相对937f73ec仅改internal/panel/mcp_completion_test.go和internal/panel/file_torrent_downloads_test.go，完整运行代码/前端/依赖与工作流blob相同，按精确两测试文件差异资格化复用。合计24PNG均已在原作业中实际查看，保留原查看时间，未冒称新提交重新生成或查看截图。原件final-ui-qualification-r4.json。Chromium140.0.7339.16/Playwright1.55.0，固定Browser Runner b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29，桌面深色、真实200%浅色、390px英文深色与繁体浅色。Docker多选右键、四类确认及跳过保护、停止当前项后不再派发、部分失败和失败项重试通过。长列表内滚动、宽表横向滚动；Files200%需纵向滚动，case-1-failure PNG只有页顶，不据此宣称错误行可读。错误态由DOM/轨迹及其他截图证明。Mock版本/页脚为夹具，实际RC版本另由公开镜像/API核对。未证明真实BT流量或Docker Engine多项全旅程，不推导125%/真实手机/Safari/Firefox/读屏准入。所有浏览器容器与隧道已清理，无本地前台浏览器/GPU。

性能原始四份samples.jsonl已核对：102正式样本、34 warmups、每组n=3，大小/哈希/退出成功；固定1CPU/256MiB、GOMEMLIMIT192MiB、128PID、network=none。RSS包含Panel/Agent/种子，memory.events和独立控制面响应未采。HTTP2MiB/s/64MiB中位32.065→20.413秒、RSS73.6→89.2MiB；8种子512KiB/s BT32.552→20.589秒、RSS159.5→164.5MiB。16活跃种子未实测，其他来源不保证提速。下载产品逻辑保持，但Otel依赖图已改变，最终二进制未重测；这些为历史条件性收益证据，不能宣称最终图性能准入。原件performance-final-r8-qualified.json与source-evidence-qualified.json。

桌面 GPU 经典拖动 +25.485% 相对20%预算仍失败，沿用 RC.1 测量前非阻断试行政策，本次未重测或宣称性能准入。桌面负责人于2026-10-16或稳定提升前复核；真实CA签发、Safari/Firefox、真实手机、低端GPU继续未验证。普通确定性交互没有额外soak，长稳网络与磁盘专项未执行。

## 公开产物与自更新通道

[GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.26.0-rc.2) 于 `2026-10-10T06:48:59Z` 公开，draft=false、prerelease=true，Latest仍v1.25.0。Docker版本与preview index均为 `sha256:46ee78d7e322adf15f7a9664d225d6134311ecc4f2090191da3a25f8c02f1f84`，latest仍为 `sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`。

linux/amd64 `sha256:ad14acc277c0209a7319e9ec2a87545a6d7e06a8b3d782e20f53df74e2812210`；linux/arm64 `sha256:9e1cf24529587e911bcbca51d448f6ad4312225eb0b5086d0bd3dcb3d4d87bfe`。双架构config版本/完整源码revision/脚本revision及摘要已核对。14个附件、SHA256SUMS11条、meta VERSION及新增license字节通过；API摘要不描述为下载全部二进制，attestation存在不描述为完整内容审计。

实际公开镜像 `image_e2e=pass`；环境arena-154，精确不可变digest `sha256:46ee78d7e322adf15f7a9664d225d6134311ecc4f2090191da3a25f8c02f1f84`，夹具容器和网络前后清单一致，原件 public-image-e2e.json。

公开更新信息作业 `arena-154-41708`，终态passed、退出0，420秒硬超时，命令规格 `C:\GitHub\_release-evidence\v1.26.0-rc.2/public-notes-browser-r1-spec.json` / SHA-256 `6197e317e9a798fcf096cd8fc46e9201288fa56bbc184191f9d82fa04860f6ea`；开始/结束 `2026-10-10T06:51:38.966Z` / `2026-10-10T06:52:15.458Z`。八项实际公开Panel/Agent旅程使用新RC.2、稳定v1.25.0与旧RC.1；实时官方GitHub/Docker只读上游，两次加入预览立即检查并保持自动安装关闭，旧RC.1向前提示，当前RC.2切回stable不产生降级安装候选。安装请求0，三个实际弹窗截图已查看，43个原件逐项摘要回收，清理通过。

截图为fullPage且保留临时通知；原始视口宽度、字体和升级提示纵向边界由截图前DOM断言验证，不把PNG高度冒称真实844px视口。旧状态跨重启迁移、systemd实际自动安装/备份/失败隔离/OpenRC及完整轻节点更新矩阵本次未重新实机验证，沿用既有实现边界。

## 分支处置与资源回收

唯一序列候选release/v1.26.0-candidate保留在bcc80ec5fcf752eb10e5e5392e60a824d85ff0db。新增下载源d0bb17c877e86eac6981fa44d65642f4a8e8ae27和Docker源395163be0278bdb2ec3f27df15109dae2a6aac7e均为不可变产品标签祖先。下载原作者4179验收预览仍持有，Docker作者所有权也未明确释放；两者分支/工作树保留，列为本地待处置，不再重复算待发布功能。恢复依据为本产品标签可达的精确来源tip。

其余非归档引用按冻结清单逐项核对；新Docker排除已按用户最新授权撤销，旧归档dca6305b持久批处理设计未纳入。本轮处置见branch-disposition.json，所有权释放、来源tip变化或下一轮筛选时再复核，不以空闲推断可删除，不唤醒旧任务。

冻结后观察到其他作者工作树引用变化：`claude/offers-column@e28ca9f935f4ad130439ea8318c2edad20d85d0e`、`feat/offers-posters@4884b5afa78a311b76dc243986292a58ddba3ff8`、`fix/file-shortcut-artwork@c341ced0f1019024462bac5f49b8be4144ad6d12`、`fix/offers-desktop-icon@e3261e8af5952add5e2ff4c9ecc7512fcf43c22c`。这里只登记实际tip变化，未核验其成型与交接状态；未纳入本次不可变产品标签，留待下一轮按来源所有权和发布准入核对。远端origin/HEAD控制别名不计为功能候选。

154此前用户授权的约15小时构建缓存清理释放1,379,340,288 bytes，回执cache-cleanup-result.json；用户后续明确授权盘点出的42项约26分钟可回收缓存，按精确ID过滤清理，实际释放1,989,713,920 bytes，回执cache-cleanup-r7-result.json。两次前后容器、镜像、数据卷清单相同。再回收上述本轮重复bundle后，r8冻结时磁盘8,063,500,288 bytes、可用内存5,342,425,088 bytes，满足发布门槛。此前本地四工作树删除被自动审批拒绝，本次未重试或绕过。

发布方4191预览均经规范stop停止；最终回执local-preview-dock-motion-r1-stop.log，旧轮回执也保留。已回收失败r1/r2生成物与r1至r7无所有者的候选检出和七轮已完成上传inbox，实际净释放3,337,031,680 bytes；另删除已完成r1/r2的远端重复source bundle共释放135,061,504 bytes，两者各两份本地副本与全部原始日志/终态保留并核对摘要。自有临时资源累计3,472,093,184 bytes；本地冻结kit、其余远端bundle与全部原始终态证据保留。对应cleanup-*-result.json记录各次实际净字节。未清理共享缓存以外的未知资源，原作者4179及Docker工作树保留。后续自有可再生资源在closeout按精确所有权处理；文档候选CI/main CI/归档在本记录提交后执行，实际终态记外部closeout，不预写通过。本记录与当前事实入口为唯一文档改动。

## 生产部署安全核对与回滚

不适用（预览版禁止生产部署）。没有生产安装、更新、正式数据修改、备份或部署安全核对；arena-154仅隔离验证和公共产物验收。prod-108/108：禁用全部KPanel操作，本次未连接、未备份、未部署、未升级、未核对。

回滚点v1.25.0 / `sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`，使用该镜像内置脚本与原备份，另行获得部署或脚本回滚授权后才执行；本次未实际回滚。GitHub Latest、Docker latest和应用市场默认仍稳定，preview指向RC.2。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-09T10:49:22.000Z
- 候选冻结时间：2026-10-10T05:47:16.911Z
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：16
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

流程异常计数覆盖本次发布编排与取证，产品门禁阻断分列：r2漏洞、r4工具栏动效兼容、r5操作栏动效兼容，首次候选CI的既有MCP异步测试立即终态假设，以及r7的BT目录夹具30秒超时各一次，均在公开发布前修复并重新完成相应校验。r3为用户扩展范围的主动取消。生产写入0，没有重复产品发布。已读批准main上最近五份正式验收v1.25.0至v1.21.0并比根因；历史SSH事件阶段不同，Windows shell/parser属于相关大类但未证明同根因复发，不虚构或压低计数。原源码试验和未完成CF仍在其证据保留。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "browser-validation/background-browser-test/windows-path-serialization",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first final feature UI controller embedded Windows backslashes into JavaScript literals and resolved a malformed helper path. It failed before SSH, browser or product assertions; that attempt supplies no UI success evidence.",
    "recoveryEvidence": "feature-browser-r1 raw state, execution-result.json and browser-test.log plus feature-browser-r1-failure-qualified.json retain the pre-SSH path failure. The fixed helper normalizes paths and serializes JavaScript data. Final Files UI r4 and Docker UI r8 both passed at 937f73ec; the exact final delta changes only the existing MCP and BT test files and qualifies identical runtime/frontend/dependency/workflow blobs, retaining the original image review time; final-ui-qualification-r4.json binds 24 actually viewed PNGs and eight traces, terminal exit 0 and cleanup receipts.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-validation/run-release-l3/outer-controller-deadline",
    "position": "before-production-write",
    "count": 1,
    "impact": "An external observer applied a 150-second spawnSync deadline to the foreground canonical L3 CLI and timed out. The original remote process continued until its output chain closed, then ended failed/141, consistent with SIGPIPE. Both the local timeout and remote failure remain unchanged and supply no L3 pass.",
    "recoveryEvidence": "l3-launch-receipt.json, l3-launch.log and recovered r1 status/raw twelve files retain the timeout and failed/141. r2 correctly failed on Trivy HIGH CVE-2026-29181; r3 was intentionally superseded when the user added Docker; r4 correctly failed on toolbar literal durations and r5 on the dock literal duration. After focused dependency and both motion repairs, r6 full L3 passed but candidate CI failed an existing immediate-terminal assumption. The six-line MCP polling repair was followed by r7 BT fixture timeout/2. Original and diagnostic Race repetitions passed, with original stall cause unconfirmed; existing BT test context and diagnostics changed while all correctness assertions stayed exact. Fresh r8 ran the entire canonical CLI directly in a persistent exec session. l3-execute-r8-receipt.json and l3-result.json require native exit 0, remote passed/0, exact Runner, twelve checksummed files and resource watchdog. No failed run is rewritten or partially reused.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-metrics/external-history-review/acceptance-before-product-tag",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first required history reviewer attempted to read the post-release v1.25.0 acceptance document from the earlier immutable product tag. git show exited 128 and Node aborted before any valid history result; a subsequent independent syntax check made the combined shell exit zero, which is not accepted as history evidence.",
    "recoveryEvidence": "review-process-history.cjs preserves the failed helper. Original stderr is retained in Codex tool output chunk d1ea84; no separate original raw log was captured and none is fabricated. review-process-history-r2.cjs reads all five historical acceptance records from approved main e4f7e19bf8e1440d262e23355263e844e9a3dd63; process-history-review-r2.json records exact file hashes and reviewed incident details.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "inputs/task-preflight/validation-argv-omitted",
    "position": "before-production-write",
    "count": 1,
    "impact": "The focused Otel repair contract omitted the required bounded string-array argv. Canonical ready rejected it before source changes.",
    "recoveryEvidence": "repair-otel.cjs and otel-input-ready.log retained; corrected otel-task-contract-ready-r2.json and otel-input-ready-r2.log passed. Final r8 ready and full L3 separately qualified the release.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependency-fetch/qualified-go/windows-golang-proxy-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "Qualified Windows Go get failed on a TCP timeout to the module proxy; no dependency file changes resulted.",
    "recoveryEvidence": "otel-upgrade.log/receipt retain native Go failure and fixed tool SHA. The immutable remote Runner performed the minimal explicit module upgrade; otel-remote-r3 receipt and raw go.mod/go.sum hashes retain recovery. Final r8 supply-chain gates qualify the resulting graph.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review-qualification/docker-integration/whole-file-source-equality",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first integration qualifier wrongly required source-file equality for api.test.ts although the approved baseline retained certificate-renewal tests. It stopped the qualification; no evidence was accepted as a pass.",
    "recoveryEvidence": "finish-docker.cjs and its preview retained. Original stderr exists only in Codex tool output a5acca; no separate raw log is fabricated. docker-merge-review-r2.json qualifies retained baseline API/test, shared menu CSS and combined mock routes; docker-ocr-coverage-r2.json and final bt-fixture-ocr-coverage.json bind all 63 exact blobs.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "shell-preflight/external-browser-helper/powershell-js-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "Nested double quotes in an external node -e patch split JavaScript into PowerShell commands and produced parser errors. Subsequent commands made the combined shell exit zero, which is not accepted for that patch.",
    "recoveryEvidence": "Original stderr retained only in Codex tool output e2a584. External saved .cjs helpers and apply_patch replaced the inline command. Canonical Docker r8 passed and final L3 independently binds actual committed source.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/docker-batch/fixture-action-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Docker r1 selected mock-current/mock-available fixtures whose permitted actions were logs/stats. Waiting for disabled restart was an invalid journey, not a product restart failure.",
    "recoveryEvidence": "docker-browser-r1 raw failure retained; docker-browser-r2-fixture-qualification.json selects action-enabled nginx/mysql. r8 completes selection, confirmation, stop-after-current and partial-failure retry at the final source.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/docker-batch/native-target-scroll-race",
    "position": "before-production-write",
    "count": 4,
    "impact": "Four mobile context-target controller attempts did not qualify UI: r2 used a wide row locator; r4/r5 used locator right-click and the menu disappeared; r6 rejected a point that was not exposed. r5 diagnostics prove contextmenu then a captured document scroll 41.2 ms later while the menu existed. The source closes menus on scroll. The exact cause of r2 is not established; these are grouped by the same target/scroll failure class, not asserted to have identical causes.",
    "recoveryEvidence": "docker-browser-r2/r4/r5/r6 original states, results and traces retained; r4/r5 dispositions document evidence and inference. r8 explicitly centers/focuses, waits for scroll to settle, verifies elementFromPoint at the visible checkbox, uses native page.mouse right-click with no locator auto-scroll, then passes the complete four-case matrix, twelve PNGs and four traces. No synthetic menu event, hidden-menu bypass or retry loop.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-validation/docker-batch/traditional-catalog-spelling",
    "position": "before-production-write",
    "count": 1,
    "impact": "Docker r3 reached the final image action but its locator omitted the actual Traditional Chinese catalog spelling 映象. Earlier visual and stop-after-current assertions do not make that full run passed.",
    "recoveryEvidence": "docker-browser-r3 raw failure/disposition retained; source catalog maps 本地镜像 to 本地映象. The corrected r8 locator uses the existing spelling and completes sequential image jobs, partial failure and failed-only retry.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "resource-cleanup/completed-l3-inboxes/manifest-inventory-omitted",
    "position": "before-production-write",
    "count": 1,
    "impact": "The exact pre-delete inbox guard expected only three kit files, omitting the canonical uploaded manifest.json. It rejected the first inventory before the deletion loop; no path was removed.",
    "recoveryEvidence": "cleanup-completed-inboxes.cjs/.py/.log and failure-qualified.json retained. Actual SSH listing showed four files. r2 added the exact local manifest SHA without weakening ownership, process, status, inventory or hash checks; cleanup-completed-inboxes-r2-result.json records four completed upload-copy removals and actual 270,241,792 bytes freed, retaining local kits and remote bundles/raw evidence.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "diagnostics/ssh-remote-argv/docker-format-whitespace",
    "position": "before-production-write",
    "count": 1,
    "impact": "A read-only progress inspection passed a Docker --format template containing spaces through SSH command reconstruction. Docker rejected the split arguments; this inspection supplied no status or release evidence and changed no resource.",
    "recoveryEvidence": "Original command and exit-1 stderr are retained only in Codex tool output a078cc; no standalone raw log is fabricated. observe-l3-progress.cjs sends Python through SSH stdin, uses structured subprocess argv and matches the exact immutable Runner plus candidate bind. Its actual successful output in tool chunk 940a37 reported owned container 462e33e60a07, OOMKilled=false and 5,789,929,472 disk bytes free. It does not replace canonical L3 terminal evidence.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  },
  {
    "fingerprint": "diagnostics/background-browser-test/unsupported-help-mode",
    "position": "before-production-write",
    "count": 1,
    "impact": "A read-only CLI capability inspection assumed --help was implemented. The entry rejected this mode with usage before any browser job or product execution. A subsequent file read made the combined shell exit zero; this is not accepted as evidence that the CLI inspection succeeded.",
    "recoveryEvidence": "Original exact invocation and usage stderr remain only in Codex tool output at2026-10-10T05:53:24Z; no standalone original log was captured or fabricated. The controller source was then read directly: supported commands start/status, actual fixed option schema and source/environment/spec/timeout binding were inspected before the later published-notes job. This inspection supplies no browser pass.",
    "permanentAction": "Owner: KPanel release tooling task. Review by 2026-10-16 or before the next L3 production write, whichever comes first. Current external helper recovery does not claim a permanent shared entry repair. Exit condition: canonical entry/fixture preflight covers the exact failure with retained regression evidence.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

CF边界补审、最终依赖图性能重测、真实公网BT/大任务磁盘/独立控制面长稳、Docker Engine多项全旅程、真实手机与多浏览器、真实CA签发和继承GPU预算失败仍有限制。预览按已有非阻断政策公开，不代替稳定审核。安全与发布负责人2026-10-16或稳定提升前复核，以先发生者为准；退出条件为精确范围完成审计、必要隔离实测与稳定门禁。

原始证据与恢复bundle统一保留 `C:\GitHub\_release-evidence\v1.26.0-rc.2`、`C:/GitHub/_release-evidence/v1.26.0-rc.2-bcc80ec5-l3-r8` 与来源 `C:/GitHub/_validation/kpanel-smart-download-20261009`。只记录实际证据，不回填失败为首轮通过。
