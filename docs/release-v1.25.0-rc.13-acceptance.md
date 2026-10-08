# KPanel v1.25.0-rc.13 发布验收记录

日期：2026-10-08

发布级别：L3。**预览产物已发布，生产未部署；实际生产实例版本未验证。**

产品提交 / 标签：`f9e4f5e126087ec6841a0bb794f036062368d3ce` / `v1.25.0-rc.13`；tree：`e05afe352eeef2e4c6b521ba3f0cc01b7fe817bb`。公开时间：`2026-10-08T07:46:56Z`。

`releaseChannel=preview`；`releaseTrain=1.25.0`。版本与 Docker preview index：`sha256:0e92dd33d05d4138a85e35f0d7aa0afd25f09b0cf9b4a6d90c0af2b1414cf34b`。仅提升 preview，GitHub Release 为非 draft、prerelease、非 Latest。

公共稳定入口仍为 v1.24.0 / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。Changelog 先前把“公共稳定入口保持 v1.24.0”与“生产环境”合写，不能据此认定实例版本；收尾候选更正该句并保留原产品 tag，不改公开摘要、版本或镜像。此处明确没有连接 prod-108/108，也没有执行生产用途。

历史公开 Release 的“发布边界”段仍包含该生产版本表述；面板的 8 条摘要和 3 条升级提示不采纳该非摘要分类，但原始公开正文并未排除它。这是已记录的文案缺陷，不是生产版本证据。遵守历史 Release 不回写的规则，以本记录和主线 Changelog 更正实际边界，不改 tag、镜像或历史正文。

## 本次功能

- **新增**：上传（含拖拽）、跨主机传输与 URL 下载共用持久化接收会话，提供 8 MiB 分块校验、进度确认、中断恢复、提交和主动取消；新版接收单文件上限 10 GiB，目录总量 10 GiB、最多 10,000 项，旧节点与公开分享继续遵守原限制。
- **新增**：备份中心新增基于完成回执的自动备份健康状态，可查看漏跑、最近成功开始时间、下次执行、上传与清理异常，并可选择开启备份异常及恢复通知。
- **修复**：修复命名卷与匿名卷跨 Docker 数据目录恢复时的目标标识匹配；重复上传或清理不会抹掉原备份异常，也不会制造新的成功时间。
- **修复**：网站 CLI 配置更新在失败时按当前文件身份条件回滚，并发修改发生冲突时保留恢复副本，减少旧配置覆盖新修改。
- **修复**：AI 文件写入审批识别运行和停止项目的实际 Compose 来源、嵌套项目与自定义文件；来源、链接或遍历完整性无法确认时转人工审批。
- **修复**：壁纸及升级状态区分已提交的写入与后续同步或清理警告，保持内存和磁盘状态一致；AI、备份、集群共用地址分类，保留各自已授权的私网策略。
- **修复**：软件包、系统调优和病毒查杀窗口在读取短暂失败后继续轮询，关闭后停止读取，避免过期响应覆盖重新打开的状态。
- **修复**：更新弹窗恢复读取历史组合分类的发布摘要，并保留升级提示；稳定与预览通道都在发布前校验实际生成的正文。

升级注意事项来自公开 Release 经真实 RC13 Agent 解析的 API：

- 升级保留现有配置和数据；新增备份通知默认关闭。远程下载任务迁入统一文件传输状态目录并保留旧记录用于恢复；连接旧节点时受旧节点能力与大小限制，回滚前应保存状态及数据备份。
- 持久化接收增加请求、校验和同步开销：隔离实测 32 个 4 KiB 文件约 0.062 秒变为 0.501 秒，64 MiB 文件约 0.397 秒变为 0.695 秒；目录接收需约两倍临时空间。数据来自单机三次中位数，不代表真实 WAN 或 10 GiB 实测。
- 本版用于预览体验，Docker 批处理未纳入；相关 CF 安全审计仍未完整完成，真实 WAN、10 GiB、突然断电与完整平台矩阵未验证。加入预览与退出预览遵循原通道设置，退出不会自动降级。

Docker 批处理按用户“Docker 批处理 不汇入预览版”排除。其既有本地 `refs/heads/archive/feature/docker-container-batch-actions` / `dca6305b9d631aeea5b12eebffa6b6752b731d5a` 保持原状，不算作本次归档或发布；该 tip 不是产品 HEAD 的祖先。

## 来源与归档

三个组装来源：可靠性 `587993a2`、公共能力 `a6ea5347`、统一传输 `8c17b17a`。安全代码前缀分别为 `970e7f92`、`00659e90`、`4e989fb4`；可靠性三个后续代码提交按 integration-map.json 精确重放。原最终 tip 的私有审计材料不推送；原始 tip、本地报告和映射均保留。

124 个业务/文档路径有 121 个与来源 blob 完全一致。三个合并路径 internal/agent/server.go、internal/panel/server.go、web/src/types/api.ts 已人工复核为新增路由/类型的合并；未发现非重叠文件偏差。公共能力三个子分支的 8/6/7 个路径与父候选业务 blob 全部一致，包括测试；按被父组装替代处置。发布说明修复 `5194858b` 已先经候选和主线 CI 集成，本版首次包含该修复。

| 来源分支 | 原始 tip / 归档 SHA | 归档 ref | 处置结果 |
| --- | --- | --- | --- |
| `fix/reliability-hardening` | `587993a206ccaeecba9625fb7d492dce9d0c6d4d` | `refs/heads/archive/fix/reliability-hardening` | absent; original source never published；本地 clean/detached，目录保留 |
| `fix/shared-capabilities-candidate` | `a6ea5347a07c8e18602f08cda9d553936c9fca2f` | `refs/heads/archive/fix/shared-capabilities-candidate` | absent; original source never published；本地 clean/detached，目录保留 |
| `feature/unified-file-transfer` | `8c17b17a94f53cf72a9ed879b4c99d849d6e9134` | `refs/heads/archive/feature/unified-file-transfer` | absent; original source never published；本地 clean/detached，目录保留 |
| `fix/shared-outbound-policy` | `e9588d743da89d8a5cc2e68a8beb7f6cfdc9777c` | `refs/heads/archive/fix/shared-outbound-policy` | absent; original source never published；本地 clean/detached，目录保留 |
| `fix/shared-persistence-outcomes` | `04c90e90fdf4adc9a83c318826af167ba3aff03a` | `refs/heads/archive/fix/shared-persistence-outcomes` | absent; original source never published；本地 clean/detached，目录保留 |
| `fix/system-polling-recovery` | `b97fa63d258262e2c6a00379fcd2ffa2e042aaa9` | `refs/heads/archive/fix/system-polling-recovery` | absent; original source never published；本地 clean/detached，目录保留 |
| `fix/release-notes-contract` | `5194858b302c039cbb82a5802f9e3eadb917b904` | `refs/heads/archive/fix/release-notes-contract` | 5194858b302c039cbb82a5802f9e3eadb917b904；本地 clean/detached，目录保留 |

处置时逐一重新核对所有权已释放、clean、精确 tip；本地事务 create archive/delete active 使用 expected-old-SHA。未物理删除任何来源工作树。六条原本未推送的来源不制造远端 archive；release-notes-contract 的远端通过原子 push 与两端精确 lease 保存 archive 并移除 active，随后复读验证。原件 source-archive-results.json。

`release/v1.25.0-candidate` 保留在产品提交 `f9e4f5e126087ec6841a0bb794f036062368d3ce`，供同序列后续 RC 使用。登录模块的进行中 CF 审计及无关旧分支不纳入，不催问、恢复或清理它们。

本验收记录与事实入口在独立 `docs/release-v1.25.0-rc.13-acceptance` 候选收尾；额外仅更正 Changelog 的生产版本表述。CHANGELOG.md 会触发 Dependency freshness，因此文档候选和主线均要求同 SHA 的 CI、Dependency freshness 成功后再精确归档。最终文档 SHA/CI/ref 在仓库外 docs-closeout-result.json 保留，避免对已发布 tag 或记录自身 SHA 反复改写。

## L3 与源码验证

- 冻结：`2026-10-08T06:34:47.6115371Z`，原始 main `5194858b302c039cbb82a5802f9e3eadb917b904`，稳定 base v1.24.0 / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`。
- 唯一入口：`node scripts/run-release-l3.mjs`，登记环境 arena-154，生产 false。
- R1：`v1.25.0-rc.13-f9e4f5e1-l3-r1` 因磁盘压力只停止了精确挂载的自有 Runner；终态 failed/137，`2026-10-08T06:35:20Z` 至 `2026-10-08T06:45:26Z`，原件和 SHA-256 保留，不算通过。
- R2：`v1.25.0-rc.13-f9e4f5e1-l3-r2`，`2026-10-08T06:58:00Z` 至 `2026-10-08T07:13:42Z`，passed/0。不可变 Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0 / npm 11.19.0 / Linux amd64。
- 完整日志 `C:/GitHub/_release-evidence/v1.25.0-rc.13-f9e4f5e1-l3-r2/remote-evidence/l3-verify-release.log`；SHA-256 `e6921c786608645c9835cda1c7b110d432e4428eebe76ffa13a96e1d55f20f63`；12 个远端证据文件回收到本地并逐项验证摘要。manifest/plan/执行脚本/bundle 的本地与远端摘要一致，inputDigests 在 l3-result.json。
- L3 输入 SHA-256：plan `24eef118c91a8260322240c3533d2ff7c9a16d3351e6dd1e149a5c84f8f029de`；执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `6474d4d71c238dcbb0d40dad1588972951439960efa651d2f8dc1d0eac43f476`。
- 固定 source 分组 jobs=2，web/go/deploy 全部 passed，耗时分别 590901 / 566239 / 4153 ms，整组 590935 ms，identity_unchanged=true。
- 前端：262 个测试文件，2,405 passed / 6 skipped；typecheck、production build 通过。Go 全部单测、panel/auth/dockerx race 与 go vet 通过；安装安全、工作流/治理/场景包静态契约通过。
- govulncheck：0 可调用漏洞，另有 1 项未被代码调用的模块漏洞；npm audit 0。固定 Trivy 源码和最终镜像门禁通过；其默认过滤开发/测试依赖，不能解释为完整漏洞或 CF 审计。
- linux/amd64 与 linux/arm64 原生二进制完成构建。最终候选镜像运行时、安装/升级/卸载生命周期、锁兼容与更新前数据备份/损坏拒绝/中断恢复场景通过。公开 Release 的受限运行时条件包含 256 MiB / 1 CPU / 128 PID、非 root、只读根、cap-drop ALL。

## 实际业务与浏览器证据

- 可靠性来源：原作者的 Linux L2、13 个隔离 native 场景与 UI 矩阵原件已核对；最后源提交的 native-input-equivalence.json 证明与原执行输入一致。组合头按相关 blob 与合并路由复核复用来源证据，**没有把来源 native 执行称为在 f9e4f5e1 重跑**。组合头另经上述完整 L3。
- 公共能力来源：最终组装 a6ea5347 的隔离 Linux L2 原日志通过且摘要匹配；3 个子分支由父组装覆盖。原作者 WSL 限权/无网络验证只作为来源证据，不把它当本轮灾备或完整 Docker 实机证据。
- 统一传输来源：最终源 L2、8 组 1280×900 浏览器流程、临时真实文件系统与合成 URL 传输、v1/v2/v3 配对和进程退出恢复原件已核对。源首轮内存/中断失败与后续通过原件保留，未改称首轮通过。
- 公开镜像：`docker.io/kjlion/kejilion-panel:1.25.0-rc.13` 显式 docker pull 后在 arena-154 执行仓库 packaging/tests/image-e2e.sh，image_e2e=pass/0；两项真实 public asset bytes 与候选源码一致，健康版本、反向代理/cookie/非 root 容器契约通过。临时 container/network 前后清单一致；临时目录由 canonical EXIT trap 清理，未单独枚举所有临时目录。
- 新发布说明旅程：后台作业 public-notes-browser-r3，exact candidate=`f9e4f5e126087ec6841a0bb794f036062368d3ce`。真实 RC13 Panel/Agent API 返回 preview、正确版本/digest/官方 URL、8 条非空摘要与 3 条升级注意；RC12 的实际设置页加入预览后打开 RC13 更新弹窗，逐条内容与真实 API 一致。desktop dark 与 390×844 light/dark、关闭/Escape 和布局断言通过，安装请求 0；Chromium 140.0.7339.16 / Playwright 1.55.0 / Node v24.18.0。
- 后台作业持久化终态 passed/0，公开镜像/Runner/Node/Playwright 均精确固定；Panel/Agent 各 1 CPU/256 MiB/128 PID，浏览器 2 CPU/1 GiB/256 PID；watchdog、OOM 与 container/network/私有状态精确清理证据均通过。浏览器 page/console/HTTP 资源错误为 0，4 项用例实际执行。
- 浏览器 R1 外层 failed/1：缺少 Docker 提供方使应用列表返回 503；24 个文件摘要完整回收。R2 的内部 4 项浏览器场景已通过，但外层只读提供方完整性断言因健康探针被拒绝而 failed/1，27 个文件原件保留。R3 不放宽零错误断言，以真实 arena Docker 的 GET /_ping 及按唯一空应用命名空间 label 限制的 GET /containers/json 提供只读能力；host Docker socket 没有挂进 Agent，所有写方法及其他路径在上游前拒绝。实际代理请求、资源与关闭记录在 docker-read-proxy.json；该提供方仅补齐 notes 旅程夹具，不解释为真实应用部署/恢复验证。
- 此处实测更新弹窗及读取，不执行 systemd 安装、自动更新回滚、生产、arm64 浏览器或完整平台矩阵。自有本地 acceptance Mock 4178 仅为可交互源码预览，未将它当作真实 API 或在线发布。

## 性能、恢复与兼容边界

统一传输的对比来自同一 arena-154、Go 1.27.1、1 CPU/768 MiB、各三次中位数：64 MiB loopback 0.397→0.695 s（约 1.75×）；32×4 KiB 0.062→0.501 s（约 8.13×）。20/100 ms 是 handler 固定延迟，不能称为真实 WAN RTT。中断 64 MiB 传输保留已确认 16 MiB、重传 48 MiB；目录临时空间约逻辑数据两倍。Go heap 统计包含约 84 MiB 的预加载样本，不能作 RSS 结论。

新版 10 GiB/10,000 项为预算上限；真实 WAN、满额 10 GiB、突然断电、外部 SFTP 性能和全平台矩阵未验证。旧节点与分享 512 MiB 以及同主机复制/电脑下载保留原授权与兼容路径。已测正常/小文件性能退步列入公开升级注意；本次准入限预览体验，未授予稳定或生产性能准入。

## 安全与独立复核

CF 覆盖冻结结果 decision=scoped-required，52 个边界提交/169 个文件尚未覆盖；最近完整 run4 基线 `4c0694aa8e02e46145a775707b8d5a0355f7ce10`。新增 internal/atomicfile 与 internal/netpolicy；元数据校验 14 个记录有效。三个作者各自 run20 使用 gpt-6-luna/max，但都未完整完成；为避免冲突，组合候选只把最小脱敏元数据保存为 run20/21/22，并保留原始 ID/SHA/费用/时间及 countedAsCoverage=false。run19 同样仍 incomplete，不计覆盖。

未完成的线索和协议/生命周期范围没有被父级开发测试代替，也未升级为“独立动态验证通过”。本轮没有新启动 CF scoped/full 审计，没有形成无漏洞或安全清场结论。PROJECT_RULES 5.4 的预览覆盖记录为非阻断；后续稳定版仍需补齐对应审计/实机/性能证据。所有未修复攻击细节、临时 ledger/report 原件保留在私有仓库外及原本地 source archive，未推送公开。

发布任务对来源 blob、映射、三处组合路由/类型和工具证据独立复核；原作者 OCR/独立复核原件按相同输入复用。本发布操作未执行新的完整 OCR；f9e4f5e1 连续 trailers 明确 OCR skipped 与 Security-Audit deferred。claude/gemini CLI 没有可调用入口，fallback=provider-unavailable、cross_provider=false 已如实记录；不声称跨供应商复核已完成。

## CI、公开产物与应用市场

- CI / `release/v1.25.0-candidate`: [37742395202](https://github.com/kejilion/KPanel/actions/runs/37742395202)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）
- Dependency freshness / `release/v1.25.0-candidate`: [37742395197](https://github.com/kejilion/KPanel/actions/runs/37742395197)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）
- CI / `main`: [37743707496](https://github.com/kejilion/KPanel/actions/runs/37743707496)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）
- Dependency freshness / `main`: [37743707427](https://github.com/kejilion/KPanel/actions/runs/37743707427)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）
- Release / `v1.25.0-rc.13`: [37744557563](https://github.com/kejilion/KPanel/actions/runs/37744557563)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）
- Dependency freshness / `v1.25.0-rc.13`: [37744557596](https://github.com/kejilion/KPanel/actions/runs/37744557596)（success，`f9e4f5e126087ec6841a0bb794f036062368d3ce`）

GitHub Release：[v1.25.0-rc.13](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.13)。Docker Hub：[kjlion/kejilion-panel](https://hub.docker.com/r/kjlion/kejilion-panel/tags)。版本标签及 preview 摘要相同；amd64 `sha256:58ed90c35259409ac2894a189b71971ee5f7a014b5b917e8f335c987bf466ae1`，arm64 `sha256:9e73797b92cceacac0038c9f88f3163cf74ce25a4336c43629d23a8926b29052`；另有两个 provenance/SBOM attestation 条目。

14 个附件、SHA256SUMS 11 条全部与 GitHub API digest 对照；meta 与许可证附件真实 bytes 已下载校验。没有下载每个二进制的全部字节，allBinaryBytesDownloaded=false。标签、image revision/version、script pin 和双架构已核对。公开正文在发布前由仓库统一门禁通过，发布后 body/API/弹窗共同回读；公开 body 与冻结产品使用最终 digest 经 canonical renderer 重建的正文一致（仅规范化 LF 与末尾空白，SHA-256 `4b7d41410258f350f031ec32cef16dab375f24265f08cda7ccbb143ed6caa380`），原件 published-body-result.json。不能只以 CI 推断用户旅程。

应用市场：`C:/GitHub/kejilion/apps/kpanel.conf`，`4fc985e964dbd1742143521d0a28ab652b885773`，实际分支 `feat/lanqin-email` clean；与 packaging/kejilion-app/kpanel.conf 的归一化 SHA-256 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089` 一致。无需 apps 提交，未切换其分支。scriptLinkageState=not-required，脚本 revision `c3a8bd895f8878d9e4ced7592c91a20c974472a5` / SHA-256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`；版本准备没有修改内置脚本、安装配置或默认 latest 更新源。

## 生产与回滚

本次无生产用途、生产写入、生产探测或部署；prod-108/108 未连接。自有隔离 Panel/Agent 不挂真实 Docker socket，不操作真实宿主升级。生产实例的版本、健康、数据、备份及公网入口均未验证。

稳定源码/镜像恢复点：v1.24.0 / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；上一预览 v1.25.0-rc.12 / `fd2a243c12c9f9434da409408098135c8a23b3f9` / `sha256:b9f4aa4342deb7155581a301d920b07a36e446417d85da96744cd2693a98f016`。这次未回滚、热修复或重复公开发布；R1 是失败验收重试，不是重复发布。真实回滚须另获授权，先保存状态/数据、固定旧 digest，再走现有更新恢复流程，复核版本与数据；退出预览不会自动降级。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-08T02:07:56+08:00
- 候选冻结时间：2026-10-08T06:34:47.611Z
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间按上一产品 tag 后最早纳入 Git 提交 998697fe 记录，包含上一版验收文档；不把该文档当作本版新业务功能。预览公开时间与正式发布频率、生产部署频率分别记录；生产频率及提交到生产用时不适用。
恢复时间记录本轮最后失败验证通道经 R3 浏览器外层 passed/0 的恢复，不将它当作生产恢复或主线文档 CI 完成时间。主线文案澄清候选、其 CI 与归档的完成时间单独保存在 docs-closeout-result.json。

结构化指标没有产品回滚、紧急热修复或重复发布，产品恢复字段因此不适用；流程失败另行记录：首次发现 2026-10-08T06:45:26Z，最后验证通道恢复 2026-10-08T08:00:38.303Z。公开 Release 的非摘要发布边界文案已逃逸至历史正文，本记录与主线 Changelog 澄清生产未核验，历史产物不回写；L3 R1、正文本地编码比对及浏览器夹具失败则被各自门禁拦截。结构化冻结时间按校验器使用毫秒精度，原始七位小数 `2026-10-08T06:34:47.6115371Z` 在 freeze-final.json 保留，未改变冻结输入。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：26
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

所有首轮命令失败、无效证据、R1 失败和恢复记录原样保留。最近五个正式版本 v1.24.0/v1.23.0/v1.22.0/v1.21.0/v1.20.0 已复核；路径读取指纹在 v1.24.0 有历史复发，12 次当前事件完整计入，没有靠合并明细降低总数。现场改用正确命令不等于永久修复；唯一入口/预检的永久改进仍未完成，责任发布任务，2026-10-14 复核，在下一次 L3 生产写前处理。未执行生产，不把流程重试计为产品生产变更失败。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/thread-list/unsupported-page-size",
    "position": "before-production-write",
    "count": 1,
    "impact": "list_threads limit 60 exceeded server maximum 50; corrected to 50 (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Use the advertised maximum of 50. This is an executor correction; no shared repository change is claimed. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 12,
    "impact": "rg included nonexistent .codex; corrected to tracked .codex-workflows (count=1). Additional event: Read attempted guessed RC12 result.json; discovered authoritative manifest paths instead (count=1). Additional event: rg given literal internal/panel/self* and web/src/components/settings/*.vue as positional paths on Windows; corrected to --glob or discovered tracked file paths Additional root read passed literal internal/panel/auth*.go on Windows; corrected to directory plus --glob and excluded tests to limit irrelevant results. (count=3). Additional event: Guessed scripts/source-check-groups.mjs was absent; actual grouped authority is scripts/run-source-checks.mjs (count=1). Additional event: rg given absent .workflows and docs/RELEASE_CHANNELS.md; tracked .codex-workflows and docs/release-channels.md discovered via rg --files (count=1). Additional event: Two attempted source reads used absent internal/selfupdate/release_summary.go and cmd/kejilion-panel/main.go; actual internal/agent/self_update.go and cmd/paneld/main.go read after rg discovery Additional root read attempted absent internal/agent/config.go and guessed web/src/components/settings/AutomaticUpdateSection.vue; actual Config is in internal/panel/config.go and UI is web/src/views/SettingsView.vue, discovered and read. Both failed reads retained. Owned remote generated-file cleanup first probed a guessed status.json; canonical inventory showed status.txt. Guard failed before any remote removal (root chunk a9118b). Correct authoritative key/value status input, preserve source/inputs/evidence, rerun the owned cleanup. (count=5).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Repeated path guessing is retained as 12 events, including after a prior reminder. Use rg --files before each read. Permanent shared preflight/fixture repair is not completed; release task owns follow-up by 2026-10-14, before any next L3 production write. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "preflight/task-contract/invalid-shape",
    "position": "before-production-write",
    "count": 1,
    "impact": "First ready rejected >128 literal paths, unsupported tool names and no validation entries; corrected contract only (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Keep <=128 paths, supported tools and explicit validation records; ready passed before L3. No source policy relaxation. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-notes-go-gate/missing-local-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows PATH had no go; canonical draft gate used existing portable bootstrap and its selected toolchain (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Discover a Go entry before the mandatory notes gate; portable bootstrap was reused. No installation or gate weakening. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "review/check-collaboration-state/invalid-trailer-paragraphs",
    "position": "before-production-write",
    "count": 1,
    "impact": "Separate commit message paragraphs made review trailers nonconforming; recorded contiguous trailers in a new review checkpoint (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Use one exact message file with a contiguous final trailer paragraph. New f9e4f5e1 review checkpoint records conformance; original nonconforming message retained. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "freeze/receipt/powershell-boolean",
    "position": "before-production-write",
    "count": 1,
    "impact": "Bare false in a PowerShell hashtable failed to create freeze JSON; corrected to $false without changing source or L3 plan (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Use PowerShell $false and read back freeze JSON. Candidate source and Runner identity remained fixed. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/read-only-log-inspection/premature-read",
    "position": "before-production-write",
    "count": 1,
    "impact": "Attempted Get-Content before Tee created the first L3 transport log; later canonical terminal and recovered hash-verified log used (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Check for a created log or read the current terminal; recovered canonical logs are hash verified. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/run-release-l3/insufficient-disk-space",
    "position": "before-production-write",
    "count": 1,
    "impact": "Stopped only owned Runner after disk pressure; canonical R1 status failed exit137, preserved and hash verified; qualified ended generated artifacts and duplicate bundles reclaimed, R2 uses new run ID (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Recorded R1 failed/137; qualified immutable published-source generated files and locally verified duplicate bundles reclaimed before unique R2. No new shared disk-preflight implementation or production clearance is claimed. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ssh-runner-identity/optional-field-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "Read-only browser Runner image inspection used an absent .Config.User template field; recovered via complete JSON inspect with omitted User treated as Docker default root (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Read the complete image JSON and treat absent User as the Docker root default; browser Runner remains pinned. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/lane-progress-inspection/unsupported-find-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "Read-only source lane progress probe used GNU find -printf inside Alpine BusyBox Runner; replaced with ls/tail; canonical L3 continued unchanged (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Use BusyBox-supported ls/tail for optional progress reads; do not alter the canonical L3. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/release-boundary/unverified-production-version",
    "position": "before-production-write",
    "count": 1,
    "impact": "Frozen Changelog and the published Release non-summary boundary contain an unsupported production-instance v1.24.0 phrase. The panel summary parser omits this non-summary category; the full public body includes it. Separate main documentation closeout corrects the phrase, keeps historical tag/image/Release immutable and explicitly records production version unverified. (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Correct the production-version phrase in the separate documentation closeout and require exact candidate/main CI, including Dependency freshness triggered by CHANGELOG. The full historical public body retains the phrase; the panel summary parser omits this non-summary category. Keep historical tag/image/Release immutable, mark the original phrase unsupported, and state production version explicitly unverified. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/public-release-body/local-encoding-and-image-parameter",
    "position": "before-production-write",
    "count": 1,
    "impact": "First canonical public-body comparison failed: Windows Python serialized local JSON in cp936 while Node read UTF-8, and the comparison passed kjlion instead of the actual docker.io image parameter. Fresh official UTF-8 API read showed intact public text. Original JSON/render/failed receipt preserved; explicit UTF-8 serialization and exact frozen workflow image output used for unique R2, without public writes. (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Preserve the first failed comparison, explicitly encode fresh public JSON as UTF-8, and use the exact docker.io image output from the frozen Release workflow. Compare again without changing the tag, image, public Release or repository renderer. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/background-browser/isolated-docker-provider-missing",
    "position": "before-production-write",
    "count": 2,
    "impact": "Canonical notes browser R1 ended failed/1: all four notes/API/dialog scenarios asserted, but real /api/v1/apps returned 503 because the isolated Agent had no Docker provider. Zero error assertions retained; 24 raw artifacts hash-recovered, all owned fixtures cleaned and no resource/OOM failures. Unique R2 uses a real Docker read gateway restricted to an empty owned application namespace, with no mutation access or mocked Panel/Agent/GitHub data. R2 inner browser passed all four cases with zero errors/installations, but the outer provider guard failed because two real GET /_ping health reads were denied. All 27 R2 raw artifacts hash-recovered, clean fixture shutdown and no resource failures preserved. R3 additionally permits only the real read-only health probe, keeps all assertions and uses a new namespace/job. (count=2).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Keep R1 failed/1 with 24 hashes and R2 outer failed/1 with 27 hashes despite its inner browser passing. Unique R3 allows actual Docker GET /_ping plus GET /containers/json with a forced empty application namespace filter. No host socket mounted into Agent; mutations and other paths denied. Keep zero console/page/HTTP and all permitted read assertions, and verify provider shutdown, container/network/private fixture cleanup and resources. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "documentation/report-release-metrics/unsupported-precision-and-recovery-state",
    "position": "before-production-write",
    "count": 1,
    "impact": "First docs validator rejected seven-digit fractional freeze timestamp and product recovery details alongside a no rollback/hotfix/republication state (original root chunk b97e45). Preserve draft; use millisecond ISO structured freeze with original precision retained and keep product recovery not applicable. Detailed infrastructure recovery and public wording escape remain outside product metrics; no parser weakening or production failure invented. (count=1).",
    "recoveryEvidence": "Root release transcript and process-incidents-current.json preserve each failed command. Successful replacements and canonical receipts are retained in C:/GitHub/_release-evidence/v1.25.0-rc.13.",
    "permanentAction": "Preserve the original draft and first validator failure. Use millisecond ISO in the structured freeze field while retaining the original seven fractional digits in freeze-final.json. The rollback/hotfix/republication flag remains no, so its product recovery field is not applicable; record infrastructure failure recovery and the public wording escape separately. No metric parser or policy changes. Owner: release task; review due 2026-10-14 before production L3.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 资源与剩余限制

按精确状态/恢复依据回收旧已结束验收的 web/node_modules、dist，以及有本地 SHA-256 相同、自包含可恢复 bundle 的远端重复副本；原源码、Git、状态、日志和本地 bundle 保留。累计删除文件的已核验占用约 3810121309 bytes；这是指定对象口径，宿主并发构建与 Docker 层回收会影响卷净变化，不能把该数字当全盘净释放。每次 df 复测、qualified 清单与结果位于本轮外部证据目录；未触及活跃/未知任务、无关项目或生产数据。

全部来源 worktree 物理保留，自己的 frozen release worktree 与两个 L3 源码/输入/证据保留用于恢复；本地 preview/依赖的实际最终回收另在 cleanup-closeout-result.json 记录。未新增监控会话、常驻清理任务、工作流或旁路台账。

本版只完成预览产物与上述范围验收；CF 完整覆盖、真实 WAN/10 GiB、突然断电、全平台/长期稳定性、原生触摸/缩放与真实 systemd 生产更新回滚仍未验证。已通过完整 L3、同 SHA CI、公开产物与实际 notes 用户旅程，结合透明性能/审计边界作为预览交付；这些证据没有扩展生产或稳定授权。

### 最终自有资源处置

本次本地 Mock 已由 canonical stop 停止；自有 release worktree 的 web/node_modules 回收 238387741 bytes，web/dist 原本不存在，源码/Git/日志保留。C: 实测可用空间 109011996672 → 109264179200 bytes。

两个已结束 L3 run 的 web/node_modules 与 R2 web/dist 回收 423342440 bytes；R1/R2 精确 status.txt、候选 HEAD、clean 状态及活跃 container mount/process cwd 检查通过。arena 可用空间实测 4526764032 → 5051293696 bytes。这些是回收时测量，宿主并发会影响卷净变化；源码、Git、bundle、plan、status、完整日志和固定 Runner 保留。此前状态文件路径的失败探测已经完整计入上述第26次流程事件，首次文档候选 8764dd675be4bc3f579661ad67b75c2773a0bfb4 与其两项成功 CI 保留，最终文档候选重新获得同 SHA CI，不复用旧 SHA 结论。
