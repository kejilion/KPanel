# KPanel v1.25.0 发布验收记录

日期：2026-10-09

发布级别：L3

候选提交 / 标签：`a1e8b4a67d964dc2d0b1e630e3de1aee67277f83` / `v1.25.0`。业务源码继承 RC15 `5eeecd4bac16ae4fccec5039863e476ad2054004`；稳定提升仅修改版本、发布说明与必要治理/性能文档。

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。

`releaseChannel`：`stable`；`releaseTrain`：`1.25.0`。正式产物已发布，生产未部署。

## 发布画像与范围

- 业务域：Docker Compose、文件接收/传输与 Office 文件、桌面/移动交互、备份和监控、AI Compose、Linux 轻量节点。
- 变更面：展示、只读、受控文件/配置写入、传输协议与持久化、安装/更新分发。整体 L3；单元/Mock、隔离实机、公开产物和生产证据分层。
- 用户旅程：Compose 项目删除并选择卷策略；多来源接收、断点恢复、取消及结果选择定位；Office 基础预览/编辑与新建；移动桌面文件夹、分页与部件；分享主题；轻量 Linux 节点恢复；稳定/预览通道发现与升级提示。
- 汇入 RC1–RC15 的已批准内容；Docker 批处理 `dca6305b9d631aeea5b12eebffa6b6752b731d5a` 已验证不是正式提交祖先，明确排除。Windows 节点、PowerShell、RDP 已在 RC7–RC9 移除。本次不汇入 RC15 后无关新分支。
- Compose 删除默认备份配置、保留数据；仅显式选择时删除项目具名/匿名卷，外部卷、bind mount 和镜像保留。其他宿主机危险写入只使用既有隔离验收证据，未执行生产管理员操作。
- 文件接收上限 10 GiB / 10,000 项属于实现限额；本轮实测没有达到上限，不宣称极限容量、WAN 或掉电恢复已验证。
- 自动安装保持默认关闭。稳定版成为 GitHub Latest 与 Docker `latest`；`preview` 保持 RC15。预览更新来源按语义版本识别同序列正式版为向前升级，切换通道不自动安装或降级。

精确提升提交：

- `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83 docs: preserve exact audit proof bytes across Git checkouts`
- `7ee6ae74402d7f82af0592ed951a5322a7a3eb47 docs: preserve exact audit proof bytes across Git checkouts`
- `77fd6471240831ae1e488da7674242fef5ee96b8 docs: close stable audit records and runtime admission`
- `50d3d75bd15dc0e152694adcf7f62276d238871f docs: record stable receive budgets and terminal recheck timing`
- `6ed8f0c69af11d498c1ecdf7dc2e38058baa3c6f chore: prepare KPanel 1.25.0 stable release`

## 多维质量与证据层级

| 维度 | 状态与证据 | 实际限制 |
| --- | --- | --- |
| 业务正确性 | 已验证：冻结提交完整 L3；11 个原生文件结果旅程与持久字节/归档/目录结果 | 沿用 RC1–RC15 各功能隔离证据；未扩展为全平台实机 |
| 安全 | 已验证：固定扫描与核心 race 门禁；CF 两个精确提交的源码记录包复核 | CF 全局范围未闭合，3 个 needs_validation 仍开放，无风险接受 |
| 稳定与恢复 | 已验证：隔离容器限制、旧传输索引字节不变、后台清理与无 OOM | 未做生产、实体路由器全矩阵、长时 soak 或完整回滚安装 |
| 性能与资源 | 已验证：固定场景实测与限定预算准入 | 默认相对 20% / 全局 32 MiB 空闲预算的失败记录保留 |
| 体验 | 已验证：本轮 11 原生交互；公开 3 组通道 API 和 4 组升级弹窗，桌面/窄屏、浅/深色、Escape | 本轮实际浏览器为 Linux/AMD64 Chromium；未宣称 ARM64 浏览器执行 |
| 数据与迁移 | 已验证：真实文件 SHA-256、历史索引字节不变、私有状态隔离与清理 | 未执行正式数据迁移、生产恢复或最大容量/断电实测 |

## 冻结与完整 L3

- 唯一入口：`scripts/run-release-l3.mjs`；登记环境 `arena-154`；run `v1.25.0-a1e8b4a6-l3-r1`。
- 冻结时间：`2026-10-08T19:32:05.205Z`；tree `c08192d4a63c5cd635eaf3cf52c22a28dc0804cc`。Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go 1.27.1 / Node 24.21.0；退出 0、完整终态 passed。
- 实际开始 / 结束：`2026-10-08T19:33:30Z` / `2026-10-08T19:49:19Z`。计划、执行脚本、self-contained bundle 和 manifest 的摘要见 `C:/GitHub/_release-evidence/v1.25.0-a1e8b4a6-l3-r1/manifest.json`；12 个原始文件均按权威校验表回收核对。
- 原始日志：`C:/GitHub/_release-evidence/v1.25.0-a1e8b4a6-l3-r1/remote-evidence/l3-verify-release.log`，SHA-256 `1500f7c392ffc01df368c003d53edd1d5e360819448d9aa46dd2882bd4d034ac`。本轮前端汇总：Test Files  264 passed (264)；Tests  2428 passed | 6 skipped (2434)。
- 固定 Web/Go/部署源码组、核心 race、全量 vet、供给链/源码/最终镜像扫描、双架构二进制构建、镜像契约与应用配置生命周期实际完成。扫描 PASS 只代表声明的规则和范围。
- 11 原生用例：native-upload-multiple-results-across-pages；native-rename-authoritative-destination-selected；native-compress-result-selected；native-extract-result-selected；native-paste-authoritative-destination-selected；native-remote-download-actual-public-HTTPS-result-selected；native-narrow-light-selected-result-no-overflow；native-desktop-200-percent-selected-result；native-keyboard-escape-keeps-result-selection；native-late-upload-keeps-current-directory-selection；native-visible-file-controls-min-font。后台终态、浏览器/HTTP 错误为 0、持久结果和旧索引字节相同均核验；fixture/network/container 精确清理与资源检查 passed。
- Browser Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`；外部 Node 24.18.0 / Playwright 1.55.0，420 秒硬限时；没有把旧 RC15 的结果冒充新提交结果。

- 本地 L3 镜像 `sha256:23743d89541b0f879488d901fc24a3fd73477afa747498a2d854e84489023a39` 的 OCI revision 实际为 `unknown`，通过精确 bundle/checkout/Runner 和构建日志导出 ID 绑定冻结提交；未重写标签冒充完整 revision。公开镜像的完整 revision 则另行实际核验。
- 原生验收 r1–r3 的菜单关闭失败与首轮截图/事件/trace 保留；r4 等待上传提示按现有 1,800 ms 生命周期收起后通过全部旅程，实际任务数/高度与滚动记录支持布局时序分析。它修正验收操作时序，没有更改产品源码。窄屏和 200% 截图只证明当前选择状态与文档宽度断言，侧栏/标题及滚动位置的截断保留，不扩称全页面视觉无瑕疵。

## CF 专项记录与覆盖

- 覆盖检查：decision=ok；未审计提交数=52；完整 full 距目标 17 天；无新增信任边界包；冻结前 `--require` 退出 0。机器准入不表示所有待审计提交均审完。
- 审计源码：RC15 `5eeecd4bac16ae4fccec5039863e476ad2054004` / tree `02faf8fcad8d79172bfe24bc25fa4761cad295f1`；受检期间 clean。固定 skill pin `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`；专用审计、发现、独立验证与 critic 均按 `gpt-6-luna` / `max` 配置。
- run-29 只给 `3507b50826f1e49b1ff5079c9557a93fb5bb02e5` 的 6 条声明路径记覆盖；run-30 只给 `14dac0f7a3d9f48e38331cc3770bc5cd9a36e23a` 的 8 条声明路径记覆盖。上下文、补充路径和 run-28 精确 Cluster 单元不扩为其他提交或整轮覆盖。
- 最终独立源码记录包检查 PASS（SHA-256 `89406dae669a7abe2ce91ad6054434a9bc43cb082cea90a03815ee265f01b4cf`）；run-29 r6、run-30 r5 的官方 findings/ledger 双验证均退出 0。发布入库衍生记录经字段、原始索引和 Git blob 字节核对。
- 3 个开放源码线索：auth-state-dir-fsync-error-revives-revoked-session、terminal-sse-output-after-session-revocation、backupremote.credentials.allow-http；均 `needs_validation`，不是已证实高危或已修复。wallpaper.add-delete-durability-divergence 维持 rejected。
- 原外部 canonical metadata 仍为 in_progress / scope_complete=false；仅经独立复核的精确提交片段以完成的注册衍生包计入覆盖。全局范围仍未闭合，risk_acceptance=null。run-23/24/26/28 及原 merged window 的未完成/越时记录保留，不补写全局通过。
- 审计只读源码和证据、执行可信结构验证器；没有运行目标代码、测试、服务、攻击 harness 或动态流量。原始 findings/P3/P5/攻击路径在外部受控证据目录保留，公开安全摘要不伪造空 findings。
- 原 17:22:17Z 窗口保持 partial。新 18:23:08Z 收尾累计实际 8 次模型调用在两轮间共享，不按每轮重复计费；token/费用及早期批准精确时间未知。最终 critic 原件 19:17:52Z 已封存；最后 intake-only 批准 19:18:14Z 晚于前窗 6 秒，不倒推追认，不新增审计工作。
- 旧 run-29 bookkeeping SHA `4a321f3db90e91356919d753ac987db1270ecdf73adac1ef15a6bc0b820202c6` 原字节未保留，明确不重建；当前 provenance 独立核验两个正确 path/hash。遗漏、格式拒绝、预算越时和原始保留缺口列入流程事件。公开索引 42 个证明 blob 经局部 .gitattributes 保留原始 CRLF 字节及正常空白检查。
- 后续源码线索、范围和工具修复见 `.governance/security-audit/remediation-status.md`；责任人各领域及发布工具负责人，2026-10-12 复核，动态验证或真实修复前维持开放。

## 性能与资源准入

- 原始记录：`performance-admission-r1.json`、`runtime-summary-r1.json`、`runtime-admission-r1.json`，外部 `C:/GitHub/_release-evidence/v1.25.0`；业务代码与 RC15 相同，稳定版版本字段不改变此结论。
- 接收：两批，每 variant/数据集 54 次实测与 36 次 warmup，368 个原始证明；同机 client+handler 1 CPU / 512 MiB。64 MiB wall P95 326.608→711.112 ms / CPU .26→.64 s / peak RSS 174,952,448→259,911,680 bytes；32×4 KiB 85.572→534.794 ms / .04→.28 s；目录 16 MiB+32 小文件 125.536→291.104 ms / .09→.25 s。
- 默认相对 20% 预算未通过，global budget unchanged。按分块确认、持久化与中断恢复的既有语义，以明确替代方案/绝对预算/低端实测/回滚点记录场景准入：64 MiB ≤1,000 ms / .8 CPU s / 272 MiB；小文件 ≤1,000 ms / .4 CPU s / 192 MiB；目录 ≤500 ms / .35 CPU s / 192 MiB。
- 独立 FileHandler 1 CPU / 256 MiB 三轮/数据集，peak RSS 50.6 MiB、OOM kill 0、文件与磁盘 SHA 相同；memory.events.max 0→251 是真实内存压力，不能写成无压力。不代表完整 Agent、WAN、10 GiB/10,000 项或掉电恢复。接收目录需约两倍目标空间与额外索引开销。
- 运行时：公开 v1.24.0 与 RC15，相同 1 CPU / 256 MiB / 128 PID Panel、私有 Unix Agent；六批交错、20 次 cold start、20,000 warmup、180 秒 quiet / 37 样本。cold P95 855.744→815.352 ms；health/session <10 ms；overview 所有批 P95 <250 ms；CPU median-max +4.08%、Panel RSS median-max +9.83%，相对 20% 项通过，不宣称整体更快。
- 并发登录 8 请求为 1 成功/7×429，Panel peak 98.7 MiB <192 MiB，内存事件未增加；Agent read peak 24.1 MiB <128 MiB；原 Argon2 64 MiB 单槽不变。
- 全局暖态 32 MiB 空闲预算未通过；旧稳定版同样超出。仅当前 1 账号/只读 API/180 秒 quiet 场景按 ≤40 MiB、quiet 增量 ≤8 MiB 准入（当前 quiet max 36.6 MiB）。全局预算不改，未试验降低 GOMEMLIMIT/GOGC；不是 256 MiB 总内存宿主机、ARM、任意状态体积准入。
- 责任人文件传输/运行时领域与发布负责人，2026-10-15 复核；退出条件为真实低端与代表数据集证明恢复默认预算或以批准的产品方案重新准入。失败原件保留，回滚使用 v1.24.0 精确 digest 和恢复备份。

## 脚本联动与应用市场

- `scriptLinkageState=coupled`，同一继承变更集 `rc4-light-node-integration-20261005`；兼容脚本已在 RC4 前公开，本轮没有新脚本提交或变更 pin。
- 双方精确依据：KPanel 正式提交 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`；kejilion/sh `c3a8bd895f8878d9e4ced7592c91a20c974472a5`。ROOT SHA-256 `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`；CN `d92c6643df6acfe15e1f4556855f42eb2ada1e831ddd9f055e85a5619102cf4f`。
- 本轮 fresh 公开 ROOT/CN 字节、Git blob 与 canonical 同步检查 passed；公开镜像实际 `/release/kejilion.sh` 由仓库唯一检查器提取并核对上述摘要。双架构 OCI pin 同样吻合。
- 22 依赖/35 更新兼容 smoke 为 RC4 原始成对证据（`v1.25.0-rc.4/paired-script-r6-verified.json`），没有冒称本轮重跑；本轮 L3、应用生命周期与公开镜像 E2E 为新证据。
- `kejilion/apps` 基线 `4fc985e964dbd1742143521d0a28ab652b885773` clean；实际 kpanel.conf 与本包 blob/归一化摘要相同 `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，公共默认仍 latest，无需应用市场提交。保留无关 sh/apps 工作树。
- 成对回滚：KPanel v1.24.0/e441…f3935b，先前内置脚本 `c981fb6c8b481981ac7a006e102e111e435f6d30` 与数据/配置冷备份。仅记录方案，本轮未执行回滚。
- 初始 Windows 依赖查询仅 4/10 源成功，6 源不可达原件保留；不能据此称全部最新。本轮精确候选/main/tag 的 Dependency freshness 实际成功。已有兼容更新建议按既有治理条目推迟至 2026-10-15，不在稳定提升中夹带升级。

## CI、Release 与公开产物

- 候选 CI：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37836889290](https://github.com/kejilion/KPanel/actions/runs/37836889290)，2026-10-08T20:05:54Z → 2026-10-08T20:14:56Z。
- 候选 Dependency freshness：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37836889548](https://github.com/kejilion/KPanel/actions/runs/37836889548)，2026-10-08T20:05:55Z → 2026-10-08T20:06:37Z。
- main CI：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37838102545](https://github.com/kejilion/KPanel/actions/runs/37838102545)，2026-10-08T20:15:39Z → 2026-10-08T20:22:33Z。
- main Dependency freshness：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37838102553](https://github.com/kejilion/KPanel/actions/runs/37838102553)，2026-10-08T20:15:39Z → 2026-10-08T20:16:27Z。
- tag Release：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37839118504](https://github.com/kejilion/KPanel/actions/runs/37839118504)，2026-10-08T20:23:45Z → 2026-10-08T20:37:19Z。
- tag Dependency freshness：已验证 success，精确 `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83`，[37839118462](https://github.com/kejilion/KPanel/actions/runs/37839118462)，2026-10-08T20:23:45Z → 2026-10-08T20:24:15Z。
- GitHub Release：[https://github.com/kejilion/KPanel/releases/tag/v1.25.0](https://github.com/kejilion/KPanel/releases/tag/v1.25.0)，非 draft、非 prerelease、Latest；实际 published_at `2026-10-08T20:36:50Z`。
- Docker 版本 `1.25.0` 与 `latest` manifest digest：`sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9`；`preview` 保持 `sha256:90713c54f2eda99b1cb47601846f51dc08b5c022ee4675e1f5f61beaa9ca99ae`（RC15）。
- Linux/AMD64 manifest `sha256:0b5462ebd96fb96f3f02c7d0eadfeb515b35fb9452e87c8b0ec4eb6de35f3532`；Linux/ARM64 `sha256:f0f85a1e182217925086e6ce6289c6994a9bc9ca3f6afc96acbb25b111061d58`。两个 config 的版本、完整 revision 与内置脚本 pin 一致；2 个 unknown/unknown 项为 attestation 条目，不当作执行架构。
- 14 个附件、11 个 SHA256SUMS 项已核对 GitHub asset API digest；meta tar 内 VERSION 及 LICENSE/THIRD_PARTY_NOTICES 原字节实际下载核对。没有下载所有二进制，不把 API digest 核对称作全部附件字节下载。
- 公开镜像按 immutable digest 拉取，唯一 `packaging/tests/image-e2e.sh` 输出 image_e2e=pass；私有资源 before/after inventory 相同，不使用本地构建缓存充当发布证据。
- 完整公开 Release body 有 11 条摘要、3 条升级提示；三个精确版本的既有运行时解析契约均为最多 8 条摘要、3 条升级提示和每条 280 rune。API/弹窗显示完整正文的前 8 条摘要及全部 3 条升级提示，没有把截断误报成正文丢失或擅自修改已公开源码。
- 公开说明验收 r2 在当前/旧稳定 API 两项通过后等待候选状态超时：旧稳定初次加载只读取已有状态，验收未先执行立即检查。r3 通过实际 UI 按钮触发真实 check POST 并核对响应后验收弹窗；不触发 install。r2 的 24 个原件 hash 回收、资源/清理通过，但没有失败截图/trace，保留该缺口且不冒称整轮通过。
- r3 的 8 张实际截图已逐张查看；摘要容器需滚动查看全部内容，升级提示滚动后的实际几何断言通过。检查完成/加入预览通知在部分截图暂时遮挡弹窗标题，不将这些截图或 fullPage 捕获扩称全页视觉无瑕疵。
- 7 个公开浏览器用例：current-actual-stable-and-preview-release-API；previous-actual-stable-and-preview-release-API；previous-desktop-dark；previous-narrow-light；previous-narrow-dark；preview-actual-stable-and-preview-release-API；preview-narrow-dark。当前稳定、v1.24.0 和 RC15 实例的 stable/preview release API 实际返回同一正式版本/digest/8 摘要/3 升级提示；旧稳定桌面深色和窄屏浅/深色、RC15 窄屏深色弹窗均实测，无溢出、正文至少 13 px、升级提示可读、Escape 关闭、错误 0，自动安装关闭且 install POST 数 0。首轮错误的 11 条展示断言与外部重试生成器拒绝保留，复跑按三个版本共同的实际契约执行，全部 7 项与原有其他断言保持。
- 最后公开复核时间 `2026-10-08T20:43:21.434737+00:00`；原始 API/OCI/附件摘要与后台截图、trace、资源和清理收据均在外部证据目录保留。

## 分支归档与本地回收

- `release/v1.25.0-candidate` 精确 tip `a1e8b4a67d964dc2d0b1e630e3de1aee67277f83` 已由正式 Release workflow 的 canonical archive 入口保存到 `archive/release/v1.25.0-candidate`；SSH 实际复核归档 SHA 与正式 tag peeled SHA 相同，活跃候选已不存在。
- 来源为已公开 RC15 候选 `5eeecd4…004` 和已批准 main `8b825c7…2d`；RC1–RC15 不可变 tag 与各轮验收记录继续保留。已归档的 Docker 批处理仍标为未纳入，不将归档等同已发布。

| 本轮来源/排除分支 | 精确原 tip | 处置及恢复依据 |
| --- | --- | --- |
| `release/v1.25.0-candidate` | `5eeecd4bac16ae4fccec5039863e476ad2054004` | 已纳入；正式候选归档保存新精确 tip，原 tip 由 RC15 不可变 tag 及祖先历史恢复 |
| `main` | `8b825c70ee0b97c4abc908f0a3404ff50eaf6b2d` | 已纳入；main 通过候选快进保留历史，基线不单独删除 |
| `archive/feature/docker-container-batch-actions` | `dca6305b9d631aeea5b12eebffa6b6752b731d5a` | 既有归档保留；明确未纳入稳定版，不删除他人工作树 |

- 本次纯验收候选 `docs/release-v1.25.0-acceptance` 独立于正式不可变 tag；须在本记录提交的候选/main Linux CI 同 SHA 成功后原子归档至 `archive/docs/release-v1.25.0-acceptance`。实际完成收据为外部 docs-candidate-ci-result、docs-main-ci-result、docs-archive-result，记录成文时不虚构未来 CI。
- 截至本文成文阶段已回收逻辑 bytes：597681663；磁盘自由空间实测变化见 cleanup-closeout-result.json。逻辑用量不直接等同可释放物理空间，共享 Docker cache/image 和他人工作树保留。验收归档后的本地工作树实际清理另留本次外部 closeout 收据，不提前声称完成。
- 回收对象与保留/跳过原因：已回收本次结束的 arena-154 L3 生成 clone 和两份 hash 相同的远端 bundle；保留已核对的本地 self-contained bundle、所有失败原件、当前和上一稳定镜像、共享缓存、性能/浏览器原始证据。当前 Windows 产品候选、已封存 CF source 和纯验收 worktree 在本记录 CI/归档完成前保留；完成后仅移除 clean 且可恢复的本任务工作树，独有或归属不明的内容跳过保留。。可恢复依据为正式 tag、精确远端归档和已经 hash 核对的 self-contained bundle。独有失败原件、CF findings/P3/P5、截图/trace、原日志和当前/上一稳定恢复依据保留。
- 未完成或保留项由发布工具负责人 2026-10-12 复核；只处理明确归属且无进程/未提交/唯一数据的可再生输入，不以目录名字判断。验收工作树在其 CI/归档完成前继续保留，之后按相同规范回收。

## 生产、回滚与剩余风险

- 正式产物已发布，生产未部署；本次没有 production-deploy/backup/postdeploy 操作或正式数据修改。隔离 arena-154 fixture 验收不代表生产部署。
- `prod-108` / `108` 禁用全部 KPanel 操作；本次未连接、未部署、未核对。
- 生产备份、健康采样、SQLite quick_check、实际升级/回滚、生产完成时间均未验证。回滚前需独立授权，停写冷备份并使用标准应用市场事务恢复 v1.24.0 指定 digest 与匹配数据/脚本，再按固定生产入口复核；本轮未执行。
- 当前公共默认来源为 GitHub v1.25.0、Docker latest=sha256:a183a818ac94446e19078087cee0b7d9d7505bd8ecae8e6fdac1e5c8efc326d9、应用市场 latest；没有失败生产版本或生产回滚引发的公共默认恢复操作。
- 实体路由器、WAN、ARM64 浏览器、最大传输容量、掉电与长时 soak 未验证；认证 fsync、终端已发送字节、HTTP 凭据拓扑、节点回滚、软链 TOCTOU、应用绑定与运行时资源后续项保持公开记录状态，不将源码线索标成修复或接受风险。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-04T06:50:11+08:00
- 候选冻结时间：2026-10-08T19:32:05.205Z
- 生产完成时间：未验证
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（本次未部署生产；仅发布正式产物）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间来自上述精确包含区间首个 Git 提交 `6ddcfda15b7cbc37756e5062c6b4b7fd6447defc`；稳定标签时间用于发布频率，不冒充生产完成时间。未通过的本地准备和无效证据另计流程异常，不计生产变更失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：58
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/workflow-cli/python-alias-unavailable",
    "position": "before-production-write",
    "count": 2,
    "impact": "Required preflight attempt workflow-python-r1 failed before valid evidence; exit code 1. Required preflight attempt workflow-python-r2 failed before valid evidence; exit code 1.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[0] (workflow-python-r1): py -3 workflow.py list succeeded process-incidents-preparation-r22.json#incidents[1] (workflow-python-r2): py -3 workflow.py list succeeded",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "preflight/task-preflight/unsupported-tool-list",
    "position": "before-production-write",
    "count": 1,
    "impact": "Required preflight attempt ready-r1 failed before valid evidence; exit code 1.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[2] (ready-r1): Original failed output retained in initial-preflight-evidence.json; requiredTools corrected to authoritative whitelist. No separate original contract-r1 file was retained. SSH/SCP/Python qualified separately.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-notes/go-not-on-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "Canonical release-notes parser did not run because Go was absent from the child PATH. Generated r1 text is not validated evidence.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[3] (notes-r1): \"notes-r1-tool-result.json\"; Qualified Go 1.27.1 path supplied only to child environment; canonical notes-r2 parser passed. Final frozen source must pass again.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/dependency-report/upstream-network-unreachable",
    "position": "before-production-write",
    "count": 1,
    "impact": "Canonical freshness report had 6 of 10 sources unavailable; incomplete report is not evidence that dependencies are latest.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[4] (dependency-r1): [\"dependency-r1.json\",\"dependency-r1.log\",\"dependency-freshness-r1.json\"]; pending; retain incomplete output and use qualified reachable environment for the same canonical detector. No suppress/allow-partial pass.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-coupling/pinned-commit-absent",
    "position": "before-production-write",
    "count": 1,
    "impact": "Foreign sh checkout lacks inherited pinned commit; first git show failed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[5] (coupling-r1): []; Qualified separate owned source retrieval; coupling-r5 canonical read-only sync passed.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-coupling/ssh-identity-unqualified",
    "position": "before-production-write",
    "count": 1,
    "impact": "Default SSH identity failed on pinned sh fetch.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[6] (coupling-r2): [{\"path\":\"coupling-preflight-r2/git-fetch.stdout\",\"retained\":true,\"sha256\":\"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\"},{\"path\":\"coupling-preflight-r2/git-fetch.stderr\",\"retained\":true,\"sha256\":\"cec2649e6cb8db7b056111a36363ad16acb581d948d3dc4012a1d12873b683db\"}]; Use existing configured task SSH identity in child environment; no foreign checkout changes.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": [
      "v1.23.0"
    ]
  },
  {
    "fingerprint": "preflight/script-coupling/https-connection-reset",
    "position": "before-production-write",
    "count": 1,
    "impact": "Fallback HTTPS source retrieval reset connection.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[7] (coupling-https): [{\"path\":\"coupling-https-preflight-raw.json\",\"retained\":true,\"sha256\":\"4aac1fce12c7f79488afebf32b6d85027661f8a2edddd9c64266a99b59e4324c\"}]; Pinned SSH retrieval and public ROOT/CN hash equality subsequently passed.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-coupling/wrong-canonical-script-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "Assumed sync script name does not exist in pinned sh source.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[8] (coupling-r4): [{\"path\":\"coupling-preflight-r4\",\"retained\":true,\"sha256\":null}]; Use actual tests/test_cn_script_sync.sh in check mode.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": [
      "v1.20.0",
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "audit/source-identity/invalid-scout-head-tree",
    "position": "before-production-write",
    "count": 1,
    "impact": "Scout source identity was invalid; no coverage accepted.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[9] (cf-b1-identity): [{\"path\":\"cf-audit-preflight/B1-identity-recovery.json\",\"retained\":true,\"sha256\":\"b729f9c4b213f81d7a09b78cbbbeeb0deba3cd66001dd8a9e55160044155c9b2\"}]; Re-read exact source HEAD/tree/clean and preserve original unsupported evidence.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/budget/wall-clock-overrun",
    "position": "before-production-write",
    "count": 2,
    "impact": "Original 1200s B1 attempt finished at 1207s; partial remains partial. B6 close attempt used 636s against 600s; remaining validators/critic absent.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[10] (cf-original-b1-budget): [{\"path\":\"cf-audit-preflight/B1-original-budget-close.json\",\"retained\":true,\"sha256\":\"fe1ae434b77f3803b1b03dd45d2629cbe4f4d3c9e164f17f2eb0a17a4f0a958b\"}]; Separate explicitly budgeted continuation, no retrospective completion. process-incidents-preparation-r22.json#incidents[11] (cf-b6-close-budget): [{\"path\":\"cf-audit/run-28/run-metadata.json\",\"retained\":false,\"sha256\":null}]; Preserve run28 partial, independently close in separate approved combined batch.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/verifier/malformed-mandatory-json",
    "position": "before-production-write",
    "count": 2,
    "impact": "Original auth Phase3 and Phase5 mandatory verifier responses were malformed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[12] (cf-auth-malformed): [{\"path\":\"cf-audit-preflight/B1-auth-revocation-phase3.json\",\"retained\":true,\"sha256\":\"3d092f55db8066ebb35853b1105fd164b2bd8adf0206e683ee247de644d72046\"},{\"path\":\"cf-audit-preflight/B1-auth-revocation-phase5.json\",\"retained\":true,\"sha256\":\"98297c625c4b1c8d8ef10c832d9766cf3bd586b46f865a5c270299f0a7d84fc2\"}]; Discard without parent repair; fresh independent verification with preserved original results.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/record-promotion/invalid-parent-transcription",
    "position": "before-production-write",
    "count": 3,
    "impact": "Parent first transcription of actual valid auth/terminal/B6 P3 JSON introduced syntax errors.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[13] (cf-transcription): [{\"path\":\"cf-audit/run-26/agents/p3_auth_revocation_fresh/result-transcription-error.txt\",\"retained\":true,\"sha256\":\"8683d4ec2fdeab7bfd1a98f1104d73ddb803c38266df1e894c538454268eb27f\"},{\"path\":\"cf-audit/run-26/agents/p3_terminal_revoke_fresh/result-transcription-error.txt\",\"retained\":true,\"sha256\":\"e0c5bbd25789fb6ab0257e52fa5bfc4019fa049fd675fe88b52760ea67a17111\"},{\"path\":\"cf-audit/run-28/agents/p3_backupremote_http_credentials/result-transcription-error.txt\",\"retained\":true,\"sha256\":\"79b12c1dddba528b022378200b30764be404c7a35b636af57c1692f109f9ec74\"}]; Preserve failed transcription adjacent to faithful valid original transcription; not counted as malformed verifier responses.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/reconnaissance/mis-scoped-prompt",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial B6 reconnaissance incorrectly scoped two paths instead of complete eight-path change.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[14] (cf-b6-scout-scope): [{\"path\":\"cf-audit/run-27/run-metadata.json\",\"retained\":true,\"sha256\":\"b112e58b0f622778a1d277792be3c137b5432b4d29adf19db522058d69056033\"}]; Discard scope credit, corrected complete source review.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/coverage/noncanonical-unit-id",
    "position": "before-production-write",
    "count": 1,
    "impact": "Run28 hunter classifier unit ID differs from assigned canonical ID.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[15] (cf-b6-unit-id): [{\"path\":\"cf-audit/run-30/unmapped-hunter-unit.json\",\"retained\":true,\"sha256\":\"294cb2adfb4cc41d094a46689ab8616ed9ebdfdb12eab1cbed2991646e50a7c5\"}]; No parent rekeying or coverage; fresh classifier/AI/composition hunter.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/coverage/missing-owner-path-mapping",
    "position": "before-production-write",
    "count": 1,
    "impact": "Original B1 summary lacked actual independent owner/path mapping required by ledger.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[16] (cf-b1-owner-map): [{\"path\":\"cf-audit/run-29/merge-window-plan-amendment.json\",\"retained\":true,\"sha256\":\"6aaa8415212f7149d2df5f35fd3f6d028fc38e28b583813ee1e3d82a742edadd\"}]; Do not invent IDs; fresh hunter with explicit mapped checks.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/performance/unknown-case-name",
    "position": "before-production-write",
    "count": 1,
    "impact": "Evidence recorder assumed small-case name rather than reading actual JSON dataset key.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[17] (file-performance-parser): [{\"path\":\"performance-record-failed-r1.json\",\"retained\":true,\"sha256\":\"09c703bde8e1f65ea76dc83d244f4854d61508ad1da67baf6e0b08c45e5d6a44\"}]; Read actual case keys and recheck all 368 retained artifacts.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/task-ready/dirty-writer",
    "position": "before-production-write",
    "count": 1,
    "impact": "Ready gate was accidentally run while release-owned remediation document was dirty; JSON failed, later independent shell command masked shell exit.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[18] (ready-dirty): []; Retain actual failed result in conversation; require a new ready gate on clean final source. Original standalone receipt was not retained.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/runtime-fixture/wrong-session-cookie",
    "position": "before-production-write",
    "count": 1,
    "impact": "Runtime fixture chose sid instead of actual kejilion_session after bootstrap.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[19] (runtime-r1): [{\"path\":\"runtime-performance-r1-receipt.json\",\"retained\":true,\"sha256\":\"97f1d3eb8fbff2598826193c4bca89d5a62499c723a06bc11546d13f24f46bd6\"},{\"path\":\"runtime-performance-r1\",\"retained\":true,\"sha256\":null}]; Read actual Set-Cookie header and use separate headers correctly.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/runtime-fixture/agent-startup-prerequisites",
    "position": "before-production-write",
    "count": 1,
    "impact": "Actual fixture Agent exited before Unix socket readiness; original stderr not captured.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[20] (runtime-r2): [{\"path\":\"runtime-performance-r2-receipt.json\",\"retained\":true,\"sha256\":\"341a2debae7f8ea77132f553668fea54106fcfc90dce737dbad6b82ab773a559\"},{\"path\":\"runtime-performance-r2\",\"retained\":true,\"sha256\":null}]; Qualify private named group, token root:65532 mode0640 and required temporary roots; new attempt.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/runtime-parser/memory-events-unpack",
    "position": "before-production-write",
    "count": 1,
    "impact": "Controller attempted to unpack unsplit cgroup memory.events lines.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[21] (runtime-r3): [{\"path\":\"runtime-performance-r3-receipt.json\",\"retained\":true,\"sha256\":\"4247ab6c59d61b2fa0a9baf1d5e22a12a84eaf524f2d0ec91c2b59b38947e837\"},{\"path\":\"runtime-performance-r3\",\"retained\":true,\"sha256\":null}]; Fix parent measurement parser, retain failure, new attempt.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/runtime-fixture/premature-warmed-idle",
    "position": "before-production-write",
    "count": 1,
    "impact": "Controller asserted warmed idle before fixed adequate warmup/quiet window and failed before preserving actual RSS.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[22] (runtime-r4): [{\"path\":\"runtime-performance-r4-receipt.json\",\"retained\":true,\"sha256\":\"deee2089b7d7becbe253ec3c760b77ae0ca470434e0f7f9c7a7fd51edadf3237\"},{\"path\":\"runtime-performance-r4\",\"retained\":true,\"sha256\":null}]; No invented failed RSS; separately retain startup vs fixed warmed idle, then r5/r6 actual measurement.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/phase4/invalid-trace-kind-order",
    "position": "before-production-write",
    "count": 2,
    "impact": "Official Linux findings validator rejected fresh wallpaper P3 trace ordering; ledger validator passed. Second independent wallpaper P3 returned multiple branch entrypoints and intermediate sinks; run29 r2 findings validator failed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[23] (cf-wallpaper-trace): [{\"path\":\"cf-audit/run-29/validators/linux-r1-receipt.json\",\"retained\":true,\"sha256\":\"661c0b98cacca96c3580018150dfc2c3efd9c1e215e82159739f86034a77df79\"},{\"path\":\"cf-audit/run-29/validators/linux-r1/validate-findings.stderr.txt\",\"retained\":true,\"sha256\":\"d6988ff21115ccbe28408a35eb7b66a11425cf9d3258dcb95d762b777131ece5\"}]; Discard malformed independent P3 without parent semantic repair; approved fresh P3. process-incidents-preparation-r22.json#incidents[26] (cf-wallpaper-trace-r2): [{\"path\":\"cf-audit/run-29/validators/linux-r2/result.json\",\"retained\":true,\"sha256\":\"992323a70bf110521ee4b1c5d15f58beae281062ecca8c1b2d9f240609a7bf8e\"},{\"path\":\"cf-audit/run-29/validators/linux-r2/validate-findings.stderr.txt\",\"retained\":true,\"sha256\":\"c2921d035e7167115f2ffbf9a8a5c0c1fbdc6d4a760c5ea6c022075f31179ced\"}]; Preserve invalid response; fresh independent P3 produced a valid trace with the original fingerprint. Final r4 validates the actual sealed record.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": [
      "v1.23.0",
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "audit/verifier/unstable-root-cause-fingerprint",
    "position": "before-production-write",
    "count": 1,
    "impact": "Fresh wallpaper schema recheck changed the same-root-cause fingerprint; no remap or credit accepted.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[24] (cf-wallpaper-fingerprint): [{\"path\":\"cf-audit/run-29/agents/p3_wallpaper_schema_recheck_run29/result.json\",\"retained\":true,\"sha256\":\"f4f2fd90aff05d1d6e59dec5ce69c29e271d18ab4cfc76e1665eb76a99024e82\"}]; Retain original and explicitly budget one fresh P3 with original candidate fingerprint.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/phase5/overrestrictive-source-read-prompt",
    "position": "before-production-write",
    "count": 1,
    "impact": "P5 initially refused source reads after overly broad ban on shell commands.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[25] (cf-b6-p5-prompt): []; Clarify trusted read-only source viewing in same running invocation by send_message; no fresh dispatch or changed model.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/coverage/covered-unit-nonempty-fingerprints",
    "position": "before-production-write",
    "count": 1,
    "impact": "Parent ledger aggregation retained a rejected fingerprint on a covered unit; official run29 r2 ledger validator failed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[27] (cf-covered-fingerprint-r2): [{\"path\":\"cf-audit/run-29/validators/linux-r2/result.json\",\"retained\":true,\"sha256\":\"992323a70bf110521ee4b1c5d15f58beae281062ecca8c1b2d9f240609a7bf8e\"},{\"path\":\"cf-audit/run-29/validators/linux-r2/validate-coverage-ledger.stderr.txt\",\"retained\":true,\"sha256\":\"5556e011193300865f793b177999f7fa886e669d47ef459fb1258b3ccfe4672d\"}]; Keep failed input/output; retain the rejected finding in findings but use the canonical empty result_fingerprints on the covered unit. Final r4 passes.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/record-promotion/nested-ledger-root",
    "position": "before-production-write",
    "count": 1,
    "impact": "Parent attempted a nested ledger root rather than the required flat array. The invalid attempt was preserved before official validation.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[28] (cf-ledger-nested-format): [{\"path\":\"cf-audit/run-29/validators/coverage-ledger-nested-format-attempt.json\",\"retained\":true,\"sha256\":\"4d089d16dc32875796dd411f77027fe89e9b058b9647db12e7819996f55dc8f8\"}]; Restore the faithful flat ledger representation and validate the exact final bytes; no hunter identity or result is rewritten.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/phase5/invalid-source-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "Independent material record proposal contained an internal/backupremote/ client.go path with an extra space.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[29] (cf-backupremote-path): [{\"path\":\"cf-audit/run-30/agents/p5_backupremote_http_record_run30/result.json\",\"retained\":true,\"sha256\":\"d85e08ecab33dd3eb766217a5196d7ae47dca45342ab8b14cdc64f76d5e548c9\"}]; Preserve the proposer raw output; a separate independent material verifier supplied the actual path as a nonmaterial correction and retained needs_validation.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/record-promotion/nonarray-findings-root",
    "position": "before-production-write",
    "count": 1,
    "impact": "Parent PowerShell serialization unrolled the one-element findings array to an object; run30 r2 findings validator failed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[30] (cf-findings-object-r2): [{\"path\":\"cf-audit/run-30/validators/linux-r2/result.json\",\"retained\":true,\"sha256\":\"0c32bcac14cb5f5917f89e05346488f6d0ccf013ae1f634aec6d322637101cdd\"},{\"path\":\"cf-audit/run-30/validators/linux-r2/validate-findings.stderr.txt\",\"retained\":true,\"sha256\":\"1b5f4ae53b507ced343e419c5f0b5bf8e34454332b7928ba3af8660834ba88e7\"}]; Retain the invalid object input; restore only the outer array. Run30 r3 validates the actual final findings and ledger.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/dispatch/thread-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "Wallpaper Phase5 dispatch failed at the platform thread limit; no model output was produced.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[31] (cf-wallpaper-spawn-limit): []; Preserve the actual dispatch failure in the audit history. Reuse a distinct verifier role with the original explicit gpt-6-luna/max binding; count the failed dispatch separately from model invocations.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": [
      "v1.23.0",
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "audit/evidence/missing-independent-phase5-original",
    "position": "before-production-write",
    "count": 1,
    "impact": "Ledger stated an independent B6 material verification but its precise raw result was not persisted. Final clean refused completion.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[32] (cf-b6-p5-original-missing): [{\"path\":\"cf-audit/missing-independent-p5-original.json\",\"retained\":true}]; Do not recreate from a summary. A separately authorized fresh independent source-only Phase5 is required; actual result and subsequent final clean will be retained.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/phase5/missing-verifier-contract-fields",
    "position": "before-production-write",
    "count": 4,
    "impact": "Three verified records lacked the required top-level fingerprint; one replacement producer lacked reason. These retained objects are not accepted as final Phase5 proof.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[33] (cf-p5-missing-contract-fields): [{\"path\":\"cf-audit/run-29/agents/p5_auth_record_run29/result.json\",\"sha256\":\"841ab7a590026e305a802ace14b6d7f0944aa10ace928347e65e3690ee525a04\",\"keys\":[\"decision\",\"reason\",\"record\"],\"decision\":\"verified\",\"missingRequiredField\":\"fingerprint\",\"fingerprint\":\"auth-state-dir-fsync-error-revives-revoked-session\"},{\"path\":\"cf-audit/run-29/agents/p5_terminal_replacement_verifier_run29/result.json\",\"sha256\":\"a24aaa7a43026778618716cd218f0d259dfc74d969af644f80d097dc2ad5e857\",\"keys\":[\"decision\",\"reason\",\"record\"],\"decision\":\"verified\",\"missingRequiredField\":\"fingerprint\",\"fingerprint\":\"terminal-sse-output-after-session-revocation\"},{\"path\":\"cf-audit/run-29/agents/p5_wallpaper_record_run29/result.json\",\"sha256\":\"027abe3fe49fe55b9709324539a8eebb07b72b458f49fa322cbf2dbc4ea7c2d0\",\"keys\":[\"decision\",\"reason\",\"record\"],\"decision\":\"verified\",\"missingRequiredField\":\"fingerprint\",\"fingerprint\":\"wallpaper.add-delete-durability-divergence\"},{\"path\":\"cf-audit/run-30/agents/p5_backupremote_http_record_recheck_final_run30/result.json\",\"sha256\":\"5d63efbbb3034f70caedaa585f556bc4331a853707d3f4d4e89433beb9b1fc68\",\"keys\":[\"decision\",\"record\",\"source_identity_start\",\"source_identity_end\",\"verification\",\"role_configuration\",\"inputs\",\"source_execution\",\"original_output_provenance\",\"created_at_utc\"],\"decision\":\"replace\",\"missingRequiredField\":\"reason\",\"fingerprint\":\"backupremote.credentials.allow-http\"}]; Keep all original bytes. Three fresh independent Phase5 outputs are required for run29; the valid separate run30 replacement verifier supplies its independent final proof. Official validators and final clean must bind the final bytes.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before any next L3 production write. Current qualified invocation or scoped record correction is recovery only. Exit condition: canonical entry/Runner/fixture preflight and regression evidence prevent this root cause, or an uncontrollable upstream fault has an owner, expiry and retry limit. No permanent shared entry repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/final-clean/unsupported-agent-attribution",
    "position": "before-production-write",
    "count": 1,
    "impact": "The independent final clean rejected the run29 ledger attribution because its optional actor id had no persisted dispatch proof and did not match the cited wallpaper record.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[34] (cf-ledger-p5-attribution): [{\"path\":\"cf-audit/final-clean-critic-run29-standalone.json\",\"sha256\":\"0b8a988b952cf34f6b44fe108e702ce8cad9f576eceed6b53346938b3374071f\"}]; Remove the unsupported optional historical agent_id, retain the historical raw path and hash without invented attribution, and link the newly dispatched independent P5 raw records with their explicit model and role proof. Final r5 and distinct final-clean are still pending at this preparation snapshot.",
    "permanentAction": "Release tooling owner; review 2026-10-12; exit condition: canonical audit promotion validates role provenance and exact P5 output contracts before scoped run credit. This release changes governance records only and does not claim a permanent entry-point repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/coverage/missing-required-agent-attribution",
    "position": "before-production-write",
    "count": 1,
    "impact": "Run29 r5 findings passed but its ledger failed because four superseded historical local_checks lacked required agent_id. The root controller had incorrectly treated the field as optional.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[35] (cf-ledger-required-agent-id-r5): [{\"path\":\"cf-audit/run-29/validators/linux-r5/result.json\",\"sha256\":\"474be1766920b9dbe837d92e5f86a8ea3635cba6da4f9a7565872310ff004aa0\"},{\"path\":\"cf-audit/run-29/validators/linux-r5/validate-coverage-ledger.stderr.txt\",\"sha256\":\"afbc1079f29a2459df6f757e5a4152c0846b37f33afaf8ae5675e1ad0c68095a\"},{\"path\":\"cf-audit/run-29/validators/linux-r5-receipt.json\",\"sha256\":\"e9b53a661a528ce30c478b9143d0491b48c29e3010f7efb29e3724002ba70dbc\"}]; Preserve failed r5 inputs and outputs. Remove unsigned superseded checks from the current canonical local_checks without inventing actors; retain their exact originals in historical snapshots/provenance and retain fresh independently attributed P5 records. Final r6 and distinct final-clean remain pending at this snapshot.",
    "permanentAction": "Release tooling owner; review 2026-10-12; exit condition: canonical audit record promotion preflights pinned validator-required fields and isolates unsigned historical records. No permanent repository entry-point repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/provenance/superseded-input-hash-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "The current independent final-clean intercepted a superseded_ledger reference whose path identified the earlier 607e snapshot while its claimed hash was the later 075e r5 validator input. Findings and canonical ledger bytes were unaffected. The overwritten parent bookkeeping version was not backed up.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[36] (cf-provenance-historical-map-r1): [{\"path\":\"cf-audit/run-29/coverage-provenance.json\",\"sha256\":\"d71bb1133cdd07011f259bfe296e91798835c3578795b9d6da7b59ae99c6fd62\",\"originalClaimedSHA256\":\"4a321f3db90e91356919d753ac987db1270ecdf73adac1ef15a6bc0b820202c6\",\"originalBytesRetained\":false,\"limitation\":\"Old bookkeeping hash and mismatch are retained from the trusted runtime audit-parent report. Original bytes cannot be independently rehashed and are not reconstructed.\"}]; Current provenance identifies the actual 075e ledger at validators/linux-r5-inputs/coverage-ledger.json and separately the actual 607e pre-r5 snapshot; both now independently rehashed by the root controller. Final-clean recheck is pending at this preparation snapshot.",
    "permanentAction": "Release tooling owner; review 2026-10-12; exit condition: canonical record promotion verifies every referenced path/hash pair and snapshots prior bookkeeping before replacement. This release does not claim permanent repository entry-point adoption.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/public-intake/template-literal-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "The audit parent reported its first export generator failed at JavaScript parsing before execution and before creating an intake directory. The temporary generator was deleted after repair.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[37] (cf-intake-generator-parser-r1): {\"source\":\"Dedicated audit parent runtime message and final answer\",\"originalScriptRetained\":false,\"originalStderrProvidedToRoot\":false,\"limitation\":\"Exact original exit code, compiler output and original script bytes are unavailable to the root and are not reconstructed.\"}; The parent produced the two preserved original 21-file safe intake groups after correcting its generator; no target code or product files ran.",
    "permanentAction": "Release tooling owner; review 2026-10-12; exit condition: canonical projection helper preserves failed source/output and passes syntax qualification before an export attempt. No permanent repository helper adoption is claimed.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "audit/public-intake/incomplete-derivative-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "Root admission intercepted the first generated package: required scope_mode and comparison_base were omitted, SUMMARY.md contained literal newline escapes, and process-history mislabeled 1800 seconds as the initial closure budget rather than the preserved initial 600 seconds. Ledger, findings and independent review proofs were unchanged.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[38] (cf-public-intake-contract-r1): [{\"metadata\":{\"path\":\"cf-audit/run-29/public-intake/run-metadata.json\",\"sha256\":\"8dc395e32b53ae256d16fb05a531a9c577cdfb8842da2bb873703dd9d5ee6992\"},\"summary\":{\"path\":\"cf-audit/run-29/public-intake/SUMMARY.md\",\"sha256\":\"ece9f4a5208e8379caf9c21ab24cbaa4ee4c21e53050640cc90e84d5dc70f92e\"},\"history\":{\"path\":\"cf-audit/run-29/public-intake/process-history.json\",\"sha256\":\"b103c1813216185f99ec2df82dbab90e82eac3aed3a67ae1101bcda31bbab24b\"}},{\"metadata\":{\"path\":\"cf-audit/run-30/public-intake/run-metadata.json\",\"sha256\":\"baf4309ffbbe3bf5c58cc78750a81ead1fe0baa1c75db74ef01b91b91ea799a7\"},\"summary\":{\"path\":\"cf-audit/run-30/public-intake/SUMMARY.md\",\"sha256\":\"066005a69d336e02a2766ca953201703d9db26ece685e71b38d0dc3ed158d20f\"},\"history\":{\"path\":\"cf-audit/run-30/public-intake/process-history.json\",\"sha256\":\"b103c1813216185f99ec2df82dbab90e82eac3aed3a67ae1101bcda31bbab24b\"}}]; Preserve both parent groups unchanged. Create separate r2 registry derivatives with factual canonical mode/base, readable summary newlines and initial budget from the unchanged approval r1; validate the metadata with the canonical registry validator and regenerate file hash indices.",
    "permanentAction": "Release tooling owner; review 2026-10-12; exit condition: canonical projection validates registry fields, summary representation, original budget and every indexed file hash before admission. No permanent repository helper adoption is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/git-transport/newline-normalization",
    "position": "before-production-write",
    "count": 1,
    "impact": "Git text normalization changed one indexed provenance JSON blob in the first local checkpoint despite its working-copy proof hash remaining exact. The root intercepted the original checkpoint mismatch before freeze, push or L3.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[39] (cf-proof-git-transport-r1): {\"path\":\"C:\\\\GitHub\\\\_release-evidence\\\\v1.25.0/git-proof-transport-interception.json\",\"sha256\":\"8c17841019fc90c0c209f384794251b16f7c6b61ab0896fd4f25556ff022174c\"}; Scoped per-run .gitattributes disable text conversion only for the selected evidence trees. Re-stage exact raw proof bytes; all 42 indexed files match their staged Git blobs.",
    "permanentAction": "Scoped .governance/security-audit/run-29/.gitattributes and run-30/.gitattributes preserve proof bytes on each platform; the staged blob equality receipt verifies all indexed files. No product file or global repository text policy changed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "audit/git-transport/crlf-whitespace-policy",
    "position": "before-production-write",
    "count": 1,
    "impact": "Preserving exact CRLF provenance bytes exposed Git whitespace diagnostics because the scoped proof attributes lacked cr-at-eol. The earlier chained shell command committed the local checkpoint after the failed check; no freeze, push or release occurred.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[40] (cf-proof-whitespace-r1): {\"path\":\"C:\\\\GitHub\\\\_release-evidence\\\\v1.25.0/proof-whitespace-original-state.log\",\"sha256\":\"1c52b8af2a5a8c76bcca9e865b19d4ce3fb669033b42b468352cfbb1893bce4f\"}; Enable cr-at-eol only in the scoped audit proof directories while retaining all three default whitespace checks. Require a separately captured successful Git diff check and exact Git-blob proof hashes before final qualification.",
    "permanentAction": "Per-run .gitattributes explicitly preserve raw bytes and set blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol; global text and whitespace policy remains unchanged.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-metrics/external-history-review/powershell-foreach-pipeline",
    "position": "before-production-write",
    "count": 1,
    "impact": "Read-only historical incident review failed at PowerShell parsing before any filesystem or product mutation.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[41] (historical-review-foreach-pipeline-r1): \"history-read-failed-r1.json\"; Use saved inspect-stable-history.cjs to read the retained exact canonical report without cross-shell quoting. This incident repeats the foreach-then-pipeline parser class described in v1.24.0 security-audit/evidence-writer/powershell-invocation-parser; stage/entry differ, and the semantic alias is explicitly declared.",
    "permanentAction": "Release tooling owner; review 2026-10-12 and before any next L3 production write; exit condition: historical review uses qualified saved parser and retains original errors. No permanent repository entry adoption is claimed.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "browser-acceptance/native-image/unjustified-oci-revision-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "Native fixture preflight incorrectly required a full OCI revision on the canonical local L3 image, whose actual revision is unknown. It stopped before fixture creation or product/browser execution; image digest and completed L3 source proof match.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[42] (native-prepare-oci-revision-r1): \"native-prepare-failed-r1.json; native-image-inspect-r2.json; prepare-native-stable.cjs; L3 l3-verify-release.log\"; Prepared a new saved controller that explicitly admits only the observed unknown local OCI revision and exact immutable manifest from the completed frozen L3 log. It never relabels the image revision and still requires full revision labels for public Release artifacts.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before next L3 production write. Exit condition: canonical browser handoff distinguishes local L3 source provenance from published OCI provenance with regression coverage. Current external helper is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/native-files/context-menu-after-scroll",
    "position": "before-production-write",
    "count": 3,
    "impact": "Fresh stable native r1 passed multi-result cross-page upload, then rename menuitem click timed out after an observed unstable/detached menu. Page/API errors 0 and resource/cleanup passed. Menu closure from scrolling/placement is an analysis, not a proved product heap or business error. Native r2 again completed cross-page upload, but the rename menuitem disappeared while its geometry was being qualified; no rename request was sent. Zero page/API errors, OOM and cleanup failures. Exact closing event was not recorded, so a product defect or scrolling cause is not established. R3 recorded an actual document scroll while the menu was open, 249.5ms after row pointer click; scrollY 7001 to 6854 and the menu closed. Cross-page upload passed, no rename request or page/API error. Failure trace recovered among 17 hash-verified files.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[43] (native-menu-click-r1): \"native-feature-browser-r1/remote-evidence/browser-failed.json; failed-page.png; browser.log; recovery.json (16 files hash verified)\"; A new native attempt retains all 11 actual operations and durable assertions. It waits for actual menuitem geometry and uses an observed unobscured real pointer click instead of implicit scroll during locator action. No synthetic handlers, forced click or product source edit. process-incidents-preparation-r22.json#incidents[44] (native-menu-click-r2): \"native-feature-browser-r2/job/state.json; remote-evidence/browser-failed.json; browser.log; failed-page.png; resources.json; cleanup.json; recovery.json\"; A separate full attempt records actual scroll/viewport/click events and failure tracing, waits for a quiet viewport before opening, then uses the unobscured real pointer on the row button. All 11 journeys and durable assertions remain. No product edits or force/synthetic event action. process-incidents-preparation-r22.json#incidents[45] (native-menu-click-r3): \"native-feature-browser-r3/remote-evidence/browser-failed.json; native-file-results-failed-trace.zip; resources.json; cleanup.json; recovery.json\"; Source inspection shows completed upload task rows auto-dismiss after 1800ms and their stack participates in normal layout. New attempt waits for actual upload-task rows to disappear after recording the selected-results/status screenshot, then waits for real viewport geometry before dependent operations. Records task count and height; all 11 journeys and durable results remain unchanged.",
    "permanentAction": "Owner: release tooling task; review 2026-10-12 and before next L3 production write. Exit condition: canonical native journey entry covers long-directory menu placement, viewport scroll and actual stable pointer interaction with regression evidence. Current saved retry is recovery only. Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: canonical journey covers asynchronous scroll settling, actual menu geometry and trace recovery; this external retry is recovery only. Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: canonical native journey awaits task lifecycle/layout completion and records scroll/menu/failure traces. Actual closing scroll is proven; task timer as its source is an analysis pending fresh evidence. No product code was changed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/public-release-notes/unqualified-runtime-summary-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "Published stable, previous stable and preview fixtures started, but the first stable release API assertion expected all 11 public-body notes. Actual API returned the first 8 exactly. Existing parser constants in all three exact source commits bound summaries to 8 and upgrade notes to 3. No product regression, API/page error, OOM or cleanup failure was established; no browser cases had completed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[46] (public-notes-runtime-cap-r1): \"public-notes-browser-r1/remote-evidence/browser-failed.json; browser.log; resources.json; cleanup.json; recovery.json (20 files hash verified); public-notes-canonical-parser-contract-r2.json\"; New attempt derives exact displayed notes from the unchanged full 11-note public body and the qualified 8-note/3-upgrade/280-rune parser contract in all three cohorts. It retains every 7 API/modal journey, exact text/digest/version, font/geometry/Escape and zero-install assertion; full public body is separately retained and checked.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: canonical public modal fixture derives runtime limits and normalized text from the actual parser, with a public body exceeding the display limit as a regression. This external retry is recovery only; immutable release source/images are not rewritten.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/fixture-generator/unqualified-global-text-replacement",
    "position": "before-production-write",
    "count": 1,
    "impact": "Retry generator rejected its own transformed content before a retry fixture/controller was written or started. A dependent syntax check then reported the same absent module; this cascade is grouped with its originating generator error.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[47] (public-notes-retry-generator-r1): \"prepare-public-notes-retry-r2-failed-r1.cjs; public-notes-generator-failed-r1.json; original tool chunks 2565c7 / 5f2657. Rejected in-memory intermediate unavailable.\"; Copy the known qualified controller once, then apply exact reviewed source edits for the 8-note parser contract and the three explicit retry ports/names. Inspect and syntax-qualify generated Python/JavaScript before starting the same 7 journeys.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: canonical typed fixture generator binds cohort name/version/port/SHA and parser contract without unqualified global source-string substitution; content qualification and regression precede uploads.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-acceptance/public-release-notes/missing-explicit-stable-update-check",
    "position": "before-production-write",
    "count": 1,
    "impact": "Fresh r2 completed the current and previous stable release API checks with exact digest and 8/3 notes, then timed out waiting for candidate details before any explicit update check. Existing settings load reads persisted status only; changing the preview channel invokes a check, but remaining on stable does not. No product regression, page/API error or cleanup failure established.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[48] (public-notes-stable-status-r2): \"public-notes-browser-r2/job/state.json; remote-evidence/browser-failed.json; browser.log; cleanup.json; resources.json; recovery.json (24 files hash verified). No failure screenshot or trace was recovered in this attempt.\"; New attempt clicks the actual check-release-update UI button and awaits its real POST /automatic-update/check response before reading candidate state. Exact version/digest/notes, all 7 journeys, geometry/fonts/Escape and zero install checks remain. Adds failure observation/screenshot/trace recovery; no immutable product changes.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: canonical stable and preview journeys each exercise explicit status discovery and recover terminal failure traces; current external retry is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-evidence/recovery/dependent-receipt-before-session-completion",
    "position": "before-production-write",
    "count": 1,
    "impact": "Receipt finalization stopped with ENOENT because the SCP/hash recovery exec session was still running. The browser itself had already passed all 7 cases; no product or fixture failure. No final receipt was written by the failed call.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[49] (public-notes-receipt-ordering-r1): \"Original tool chunk b4074c; public-notes-receipt-ordering-failed-r1.json; recovery completion baa6bd; public-notes-browser-r3/recovery.json (40 files hash verified); final receipt public-notes-result.json\"; Poll the actual recovery session to exit 0, then finalize the same immutable recovered 40-file evidence and verify all 7 journeys, resource/cleanup and zero install results. No browser rerun or product changes.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: orchestration treats a returned session ID as running and awaits terminal success before any dependent evidence consumer. Current task correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-docs/canonical-bash-launcher/unsupported-env-assignment",
    "position": "before-production-write",
    "count": 1,
    "impact": "Acceptance metrics validation passed, but the canonical Windows Bash adapter rejected --env VERIFY_BASE_REF before running verify-change. The adapter only permits VERIFY_LEVEL via --env; verify-change supports the exact baseline as its positional argument. No product verification result or source change was produced.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[50] (docs-base-ref-launcher-r1): \"docs-precommit-qualification.json; docs-precommit-metrics.log; docs-precommit-l0.log; qualify-stable-docs.cjs; original tool chunk aa0336\"; Preserve the original contract, draft and failed receipts. Update only the docs validation argv to node scripts/run-repo-bash.mjs -- scripts/verify-change.sh <exact stable SHA>, re-run Ready and qualification with distinct r2 output names. Update draft process count before its first commit.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: task-contract generation uses the supported canonical positional base-ref interface and rejects unsupported env keys before a gate invocation. External correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-docs/writer-ready/dirty-draft-checkpoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "Updated draft and supported validation argv were saved, but Ready was invoked while the untracked acceptance draft existed. The canonical writer checkpoint correctly requires a clean worktree at Ready and rejected the single draft path; no product or qualification pass was claimed.",
    "recoveryEvidence": "process-incidents-preparation-r22.json#incidents[51] (docs-ready-after-draft-r2): \"docs-ready-r2-result.json; docs-ready-r2.log; refresh-docs-draft-r2.cjs; docs-write-result.json; original tool chunk fdea26\"; Retain the exact draft externally, temporarily move only that hash-qualified untracked file out, qualify the same clean baseline and corrected contract with a fresh r3 Ready receipt, then restore the draft and add this incident before its first commit. Validation remains required on the final document bytes.",
    "permanentAction": "Owner: release tooling; review 2026-10-12 and before next L3 production write. Exit condition: corrected task contracts are qualified on a clean writer baseline before authoring; draft retention and restoration are explicit when correcting a prepared draft. Current external recovery is not shared-entry repair.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

最近五个正式记录已按 canonical report 读取：v1.24.0=86；v1.23.0=24；v1.22.0=4；v1.21.0=0；v1.20.0=1。同指纹合并不减次数；语义同类指纹跨写法的对应关系如下，历史记录不回写。

| 本轮指纹 | 历史正式记录 / 指纹 | 复发判断 |
| --- | --- | --- |
| preflight/workflow-cli/python-alias-unavailable | v1.24.0 / security-audit/evidence-writer/windowsapps-python-alias | Same unavailable WindowsApps Python alias; different task entry. |
| preflight/script-coupling/ssh-identity-unqualified | v1.23.0 / script-linkage/ssh-remote/missing-identity-adapter | Same SSH source read omitted the established identity adapter. |
| preflight/script-coupling/wrong-canonical-script-path | v1.20.0 / version-check/authoritative-entry/wrong-filename | Same unqualified canonical filename assumption; different script. |
| preflight/script-coupling/wrong-canonical-script-path | v1.24.0 / preflight/read-only-file-inspection/unverified-path | Same guessed path class; scope and count remain separate. |
| audit/phase4/invalid-trace-kind-order | v1.24.0 / security-audit/findings-validator/trace-kind-constraint | Same pinned findings validator trace-kind contract. |
| audit/phase4/invalid-trace-kind-order | v1.23.0 / security-audit/findings/invalid-multientry-trace-and-order | Repeated intermediate entrypoint/sink class; historical batch also included ordering. |
| audit/dispatch/thread-limit | v1.24.0 / security-audit/collaboration/agent-thread-limit | Same platform refusal; no agent/result created. |
| audit/dispatch/thread-limit | v1.23.0 / security-audit/collaboration/agent-thread-limit | Same platform refusal; no claim about its numeric limit or concurrency cause. |
| audit/public-intake/template-literal-syntax | v1.24.0 / security-audit/evidence-writer/js-template-backticks | Same JavaScript evidence generator parse failure class; original current generator was not retained, so exact token cause stays unknown. |
| release-metrics/external-history-review/powershell-foreach-pipeline | v1.24.0 / security-audit/evidence-writer/powershell-invocation-parser | Historical record explicitly includes foreach statements followed by pipelines. |

未确认上游故障的具体根因不假定相同。当前局部参数纠正、外部 helper 或重新取证不称已修复共享入口；滚动五稳定版本重复的可预检根因须在下一次 L3 生产写前落地 canonical 入口/Runner/夹具修复与回归，或对不可控故障形成有负责人、复核日期和退出条件的期限例外。本轮没有生产写操作。
