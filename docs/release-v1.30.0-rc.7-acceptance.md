# KPanel v1.30.0-rc.7 发布验收记录

日期：2026-11-07

发布级别：L3

候选提交 / 标签：1d628771d5ce3ec98b9abe70d5db1f8ea71192c5 / v1.30.0-rc.7。Annotated tag 对象：a34fbf4178622d4c1c32e3eae6d0f1454c64d407，剥离后精确指向候选提交。

上一稳定版本 / 回滚点：v1.29.0 / sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6

releaseChannel：preview

releaseTrain：1.30.0

候选分支与发布后处置：release/v1.30.0-candidate / 预览版保留，供 1.30.0 后续 RC 使用。

- 原分支 / 精确 tip / 处置分类：release/v1.30.0-candidate / 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5 / RC7 已发布，按预览规则保留；fix/rc7-scan-scope-refresh-20261107 / e506e679c0dce884fa25ec001bfa5e13667fd7c5 / 已合入 main，原远端引用保留、未删除。
- 归档 ref 与 SHA / 远端复核结果：预览版候选不归档；GitHub origin/main、origin/release/v1.30.0-candidate 和 v1.30.0-rc.7 剥离提交均为 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5。远端标签对象为 a34fbf4178622d4c1c32e3eae6d0f1454c64d407。
- 本次来源任务分支：音乐元数据实现提交为 b7d246bbe61320c6ed041dfe1024b56fc0809512、214d05c88fdbfdb105fb572f7946cb9e69bf07b7、15fb17434acdc2c705f9a7fd90b11d2d0e0a3af5；发布候选保留精确提交链。本工作树未发现额外音乐来源分支引用，未推断或执行分支删除。
- 本地分支/upstream/worktree：发布时 release/v1.30.0-candidate 工作树 C:\GitHub\_release-candidates\kpanel-v130 干净并指向冻结 SHA；证据保存在 C:\GitHub\_release-evidence\kpanel-v130-rc7-final。此验收记录为发布后的文档提交，不属于 RC7 tag。
- 未完成归档项 / 责任人 / 下次复核触发条件：KPanel Release Maintainer 在 1.30.0 稳定版公开并包含 RC7 候选后，按 project-management.md 10.2 归档候选并复核来源分支处置；在稳定版完成前保留候选。

归档不代表生产上线；RC7 产物已公开，生产未部署。

## 发布画像

- 业务域：音乐播放器的在线元数据补全，以及 Go 网络依赖安全更新。
- 变更面：Web 展示与音乐元数据只读查询；Panel 增加受认证保护的元数据查询路由；Go 依赖更新。没有宿主机写入、数据格式或部署变更。
- 受影响用户旅程：播放歌曲时优先使用本地标签、歌词和封面；缺少歌词或封面时自动尝试网易云音乐匹配，未找到适合结果时回退 QQ 音乐。该功能针对当前播放歌曲，不扫描整库、不修改音频文件、不把歌词或封面写入宿主音乐库。
- 未变化契约：没有修改持久化 schema、音乐文件、Agent 权限、应用市场默认更新入口或 kejilion.sh 执行协议。RC6 的网站访问密码设置与历史监控分组沿用；RC7 未修改 AI 助手和手机终端 Ctrl 辅助键代码。
- 风险等级及理由：中低。新增上游元数据查询依赖网络、匹配质量和网易云/QQ 可用性；请求只用于当前播放，匹配失败不影响播放。上游服务真实在线联调未作为发布验收项。

## 发布范围与未纳入内容

- 用户可见更新：RC7 新增缺失歌词/封面的在线匹配（网易云优先、QQ 回退）；更新 golang.org/x/net 至 v0.62.0 修复 CVE-2026-78661。在线查询不等于整库自动刮削或媒体文件写回。
- 精确提交清单（RC6 到 RC7，共 17 个提交）：
  - e5fbae358b4ab32ba090162296ae45bd6e16d914：记录 RC6 验收。
  - b7d246bbe61320c6ed041dfe1024b56fc0809512：音乐缺失歌词与封面在线匹配。
  - 214d05c88fdbfdb105fb572f7946cb9e69bf07b7：加入 QQ 回退并改进匹配精度。
  - 15fb17434acdc2c705f9a7fd90b11d2d0e0a3af5：限制上游响应头并添加合成预览夹具。
  - 29f268a952019850fb44b6e46305b35f9ee7e94c：准备 RC7 版本身份。
  - 43d4d15cc5fae60bb66618a3cfe7c04e24685ab2：记录 RC7 候选评审。
  - 9ec96511434d727d78c851bbba40d499fd347a13：整理 RC7 评审 trailers。
  - cb8570b64df1a3e29cdbdcd734e0d54e5584f943：避免本地音乐夹具进入 RC 源码。
  - 7ace5fdb512e47a13509919fea8eed5d69a352ec：绑定最终产品差异评审。
  - 69d1dd3d7f2da8f87af089becc9748e8843c6259：修复 HTML 解析器拒绝服务漏洞并更新 x/net。
  - be5b7e39ff6e2b297a5f15f6a83c36fceb7ee26b：记录安全依赖更新复核。
  - a80e14643cff43b56d551e2b5a770a85b2a956a8：修正扫描图身份排序。
  - f23bb99638cf50a6babfbc8701d2657bef6ea488：记录修正后的扫描身份复核。
  - 03721ec8fe6169504a6fe0065ffc4a6e81be7b29：更新 RC7 扫描策略与范围。
  - e506e679c0dce884fa25ec001bfa5e13667fd7c5：记录扫描策略 OCR 覆盖并合入 main。
  - 99ab43b50cd81c46c15f6d07deb7f82aba9a88ff：将 main 扫描策略更新合入候选并保留 RC7 来源身份。
  - 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5：记录冻结候选的最终 OCR 覆盖。
- 明确未纳入的分支、文件或后续事项：没有把在线匹配扩展为整库扫描、标签编辑或媒体文件写入；没有修改 AI 助手行为、访问密码认证逻辑或手机终端操作；未把上游实时可用性说成已验证。

## 外部审计与修复交付

安全盲审按 PROJECT_RULES.md 5.4 记录；本 RC 无 Cloudflare 安全边界改动，security-boundary-audit 不适用。

- 安全盲审补漏（quality-audit review 档）：未执行独立盲审；无 Cloudflare 安全边界改动。RC7 行级复核和自由审查为非盲，不替代盲审结论。
- node scripts/security-review-scope.mjs --base v1.29.0 输出：recorded groups=16，missing/stale=0；另有 invalid_trailer=true，源于历史 scope-less Security-Review skipped 元数据，作为治理遗留记录，未隐藏。
- finding fingerprint / 修复 commit / 独立复核与回归证据：已确认的安全问题为 CVE-2026-78661，修复提交 69d1dd3d7f2da8f87af089becc9748e8843c6259，升级 golang.org/x/net v0.60.0 至 v0.62.0；Go 漏洞扫描、候选 CI、Release CI 和 L3 通过。RC7 扫描范围更新经独立复核，精确 main CI run 42309827789 通过。RC7 本身无新增未解决安全 finding。
- 修复交付状态：依赖源码与 RC7 预览镜像已发布；未部署生产。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / 经抽查成立的 constrained-only：OCR 1.12.11；基线 e506e679 到复核提交 99ab43b5；20 个文件中 18 个可审、2 个显式排除；18/18 覆盖，0 findings；free-form findings=0；constrained-only=unreported（非盲且自由臂先运行）。排除项 CHANGELOG.md 为 unsupported_ext，web/package-lock.json 为 user_exclude，均手动检查。最终 1d628771 仅追加 OCR 记录，候选 CI/L3 针对最终 SHA 通过。
- 按 PROJECT_RULES.md 5.5 记录本稳定周期的 OCR 观察结果：这是一个 RC 候选观察，不据此判断工具长期有效或退出。
- 候选分支的原始 tip、归档位置和远端核验统一填写本模板既有候选处置字段：见本记录开头；RC 候选依规范保留。

## 跨仓库联动判定

- scriptLinkageState：not-required（无需发布脚本，不是延期）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：1faee13f3faef164efbfa899e244fc19ad58aaff / 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：RC7 没有改脚本协议、宿主机动作或内置脚本；L3 与公开镜像报告 managed_script_contract=pass，镜像标签和原始脚本摘要匹配上述基线。
- 本版发布决定：脚本不在范围，不发布配套脚本。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证（限自动匹配逻辑） | Go/Web 来源检查、音乐匹配回归测试、候选 CI 与 L3 通过。 | 未对网易云/QQ 上游服务做真实在线联调；没有整库扫描功能。 |
| 网络入侵与供应链安全 | 已验证（限精确候选和产物） | 源码、依赖、密钥/配置扫描，L3、镜像运行契约、双架构 provenance 和公开身份核验通过。 | security-review-scope 报告 invalid_trailer=true，见治理遗留；无 CF 边界改动。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3、候选/主线 CI、真实隔离容器初始与重启、更新预览/退出流程通过。 | 未执行音乐服务商故障注入以外的长时间 soak；上游 API 可能不可用。 |
| 性能与资源预算 | 不适用（未单独基准） | 在线查询设置独立超时，不阻塞音频开始播放；Release 源码与镜像门禁通过。 | 未进行长时间播放或第三方网络延迟专项基准。 |
| 用户体验与可访问性 | 已验证（限发布页/更新旅程） | 公开浏览器 published 45 项、upgrade 31 项通过；更新弹窗覆盖 320/390/1280px。 | 没有实体手机、200% 缩放或完整焦点矩阵；音乐页的第三方在线命中未作真实浏览器验证。 |
| 数据、配置与迁移 | 不适用 | 无数据库/schema 迁移、媒体文件写回、音乐库持久化或 Agent 协议变化。 | 上游匹配会发送当前歌曲检索所需元数据；依赖服务的隐私政策与可用性由用户自行评估。 |

## 自动门禁

- 定向测试及结果：精确候选的 L3 source checks：Web、Go、deploy 三个 lane 均 passed；Go 约 760 秒、Web 约 350 秒，退出码均为 0，identity_unchanged=true。候选/主线 CI 中 Windows node boundaries、变更验证、race、Go 漏洞路径、Node audit、源码/密钥/配置扫描、kejilion.sh 生命周期均通过。
- make verify-release 环境和结果：GitHub-hosted Ubuntu 24 amd64；发布入口通过 GitHub Publish preview workflow 内的 authoritative L3，最终候选 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5 通过。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：Publish preview run 42312099248 / attempt 1；L3 job 128215867855；Runner image ubuntu24 20261101.360.1，Runner ID 1000005134；Go 1.27.3、Node 24.22.1、Docker 28.0.4；状态 passed，2026-11-06T23:28:57Z 至 23:48:10Z。入口脚本 SHA-256 181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3。该 GitHub-hosted L3 入口直接运行，未生成离线 plan/bundle；相应 SHA 不适用。证据目录 C:\GitHub\_release-evidence\kpanel-v130-rc7-final\published-42312099248\l3。
- 候选 CI：run 42312099248，candidate-ci/windows-node job 128215867795 与 candidate-ci/verify job 128215867946 均 success；verify 包含变更验证、race、Go vuln、npm audit、source/secret/config、脚本生命周期。
- 主线 CI：run 42312099248，main-ci/verify job 128223006628 与 windows-node job 128223006738 均 success；主线精确 SHA 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5。
- Release workflow：run 42312099248 最终 success；promote job 128222904413 success，publish/windows-node 128228927731 success，publish/release 128229583618 success，public-acceptance/verify 128237156804 success；preview-completion success，stable-completion skipped。Release 内 L3 skipped 是预览入口已通过同候选 L3 的正常条件，不是漏跑。
- 发布说明门禁：最终正文目标版本 1.30.0-rc.7，摘要 3 条（含安全修复与两条音乐边界说明），升级提示 1 条；Release 和公开更新接口正文匹配。
- 安全扫描、镜像契约、SBOM/provenance：候选/Release 扫描和 managed-script-contract 通过；linux/amd64 与 linux/arm64 OCI index、non-root、healthcheck、版本与源 SHA labels 核验通过；修改组件 provenance 附件成功上传并验摘要。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：2026-11-07T00:41:09Z 直接执行 node scripts/report-dependency-freshness.mjs --format markdown；本机报告退出码 1，必需源完整性 4/9。go-modules 因本机找不到 go 可执行文件失败，toolchains-and-base-images、docker-base-images、security-tools、dockerfile-frontend 的在线 fetch 失败；因此这是不完整的补充报告，不当作 CI 失败或完整依赖审计。仓库 Makefile 中的 make.exe 本机入口不可启动，直接调用其底层 Node 脚本取得报告。
- 最近每日安全通告审计、EOL 复核状态及证据：RC7 Go vulnerability/source scans 通过。补充依赖报告所见 EOL review 为 current，下次截止 2027-01-29；实时源完整性不足，故不宣称覆盖全部上游审计源。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：补充报告发现 5 个 stable-channel 更新信号、0 个依赖例外、0 个传递归属信号；候选包括 Actions / build tool 的兼容或 minor 更新，本版均未采用。Go 审计固定更新如上。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：golang.org/x/net v0.60.0→v0.62.0；golang.org/x/crypto v0.57.0→v0.58.0；x/sys v0.48.0→v0.49.0；x/term v0.46.0→v0.47.0；x/sync v0.23.0→v0.24.0；x/text v0.42.0→v0.43.0。Web lock 只更新 RC 版本字段。工具链 Go 1.27.3、Node 24.22.1、Docker 28.0.4。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：Release run 42312099248 使用仓库固定 Action SHA；镜像 OCI index sha256:3e6e3fae9ea0d3e4ed4d6f4ef775d03a010fc7c9a63cdec852e6cc8988872e71；内置脚本 revision/hash 见跨仓库章节。产品 Go 与 Web 依赖变化仅限上列内容。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：5 个上游工具更新候选留给后续依赖评估；没有在 RC7 顺带升级。
- 升级后的兼容、安全、构建、性能资源和回滚结论：依赖安全修复在精确候选上通过 L3、CI、镜像构建；功能不写媒体文件，stable 与 preview 回滚点均由摘要固定。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：GitHub-hosted ubuntu24，Runner image 20261101.360.1，amd64；预览 OCI index 同时包含 Linux amd64、arm64。容器独立验收运行真实 RC7 镜像。
- 环境策略 ID 与允许用途：environment-policy.json 中的 github-hosted candidate-validation / browser-validation；仅隔离容器、真实浏览器、更新与公开页面验收。
- 使用的精确候选或公开产物：候选及公开 SHA 1d628771d5ce3ec98b9abe70d5db1f8ea71192c5；镜像 digest sha256:3e6e3fae9ea0d3e4ed4d6f4ef775d03a010fc7c9a63cdec852e6cc8988872e71。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：Release run 42312099248 / attempt 1。public-acceptance job 128237156804 success，2026-11-07T00:29:40Z 至 00:31:23Z；published 浏览器 45 项、upgrade 浏览器 31 项、隔离容器初始 48 项、重启 26 项，全部 passed / errors=0。证据目录 C:\GitHub\_release-evidence\kpanel-v130-rc7-final\published-42312099248；未触发 timeout。L3 receipt 和来源日志在同目录 l3 下。
- 测试窗口/循环数及风险依据：单轮公开验收、隔离初始与重启；未执行 soak。RC7 无持久化 schema、Agent 权限或宿主机脚本动作变化。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：公开 release/upgrade 浏览器验证了预览身份、正文、stable/preview 通道与更新提示；更新对话框 320、390、1280px 通过。音乐在线服务未在真实浏览器中请求网易云/QQ；无实体手机和全应用缩放矩阵。
- 宿主机写入、失败注入、重启恢复和回滚结果：未连接生产主机；真实隔离容器初始/重启验收通过；自动安装默认关闭，加入/退出预览没有触发安装或降级。
- 未执行场景及原因：真实音乐服务商联调、整库刮削、实体手机、全应用缩放、长期 soak 未执行；它们不属于此 RC 的受控公开验收范围，外部服务可用性不稳定。

## 发布产物与公开仓库复核

- 公开 Release 正文、面板发布信息接口与更新弹窗：版本 1.30.0-rc.7、preview 通道、digest 和 URL 一致；3 条摘要、1 条升级提示；公开浏览器断言通过。
- GitHub Release 的 draft / prerelease / Latest 状态：isDraft=false、isPrerelease=true；2026-11-07T00:29:17Z 发布；GitHub Latest 保持 v1.29.0。RC7 Release：https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.7
- Docker 版本与通道 OCI index：1.30.0-rc.7 与 preview 均为 sha256:3e6e3fae9ea0d3e4ed4d6f4ef775d03a010fc7c9a63cdec852e6cc8988872e71；Docker latest 保持稳定镜像 sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6。
- linux/amd64、linux/arm64 digest：amd64 sha256:f6fa912931b0d1f5c89e2ade0e3ff650e579c9fa792e81afc426904e2414a276；arm64 sha256:dc8a496e8991820e4b149eb24526d03e9835d9f2b80fad425a6c857202898f5c。
- 附件及 SHA256SUMS：Release 19 个附件；15 项校验；SHA256SUMS 附件 sha256:76f8bfa1b214f4910913f6be2cffca00f61c8f091200d90941af3e540e501640；KPANEL_MODIFIED_COMPONENTS.json provenance 已发布。
- 公开镜像 image_e2e=pass：发布报告确认摘要、平台架构和 image labels；隔离容器初始/重启总计 74 项通过、0 errors。
- kejilion/apps / kejilion.sh 契约结论：应用市场稳定默认入口未变；镜像内脚本 revision/hash 与登记一致；无跨仓库脚本发布。

## 自更新通道验收

- 稳定来源只选择正式 GitHub Latest，预览来源只接受规范稳定版或 RC，并校验唯一官方 digest：真实预览 API/browser 检查通过，RC7 version/digest 与 Release、Docker Hub 一致。
- 加入预览只切换来源并立即检查，没有自动安装：验收报告中加入预览未请求安装。
- 自动安装开关与一次性立即安装相互独立：公开 upgrade 流程检查通过；默认 automatic update disabled，未发安装请求。
- 旧状态默认迁移到 stable，重启后通道选择保持：公共隔离初始/重启验收通过。
- 退出预览且稳定版较低时没有产生降级候选：浏览器和 API 验收通过，退出仅切回 stable 来源。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：真实公开 update 流程确认默认关闭；RC7 未执行真实宿主机更新，无生产主机写入。
- OpenRC 与轻量 Node 当前边界：本 RC 未改变平台策略，仍按 docs/release-channels.md 的 systemd 自动更新边界和轻量 Node 稳定通道策略。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：GitHub-hosted 隔离验收；未使用 prod-108。
- 正式部署环境：不适用；未连接 arena-154。
- prod-108：确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用（预览版禁止生产部署）。
- 部署命令/入口：不适用（预览版禁止生产部署）。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用（预览版禁止生产部署）。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3、公开镜像、浏览器和容器初始/重启验收。

## 回滚

- 源码/tag：稳定回滚参考 v1.29.0；RC7 使用 annotated tag v1.30.0-rc.7，发布后未覆盖或改写该 tag。
- 镜像 digest：稳定回滚镜像 sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6；预览镜像另由 RC7 digest 固定。
- 数据/配置备份：无生产部署，无生产备份操作；在线匹配不写本地音乐数据。
- 回滚步骤和回滚后复核：退出预览仅切回 stable、不自动降级；需要人工回退时使用稳定版官方 digest 和现有更新恢复流程。
- 回滚后生产实际版本与健康状态：不适用（无生产部署）。
- GitHub Latest、Docker latest 与标准更新入口实际指向：GitHub Latest v1.29.0；Docker latest 为 v1.29.0 digest；标准稳定入口不变。
- 公共默认更新通道决策：短期保留（opt-in preview），不提升默认通道。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-07T03:13:42+08:00
- 候选冻结时间：2026-11-07T07:28:10+08:00
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

## 遗留风险与后续准入

- 本地资源回收（按 docs/project-management.md 13.1）：未删除候选 worktree、分支或发布证据；保留位置 C:\GitHub\_release-candidates\kpanel-v130 与 C:\GitHub\_release-evidence\kpanel-v130-rc7-final。
- 未验证风险：网易云/QQ 真实上游可用性与实时匹配质量；依赖 freshness 补充报告在本机只有 4/9 个源成功；security-review-scope 有 scope-less skipped trailer 格式遗留；实体手机与完整缩放/键盘矩阵未跑。
- 已实现待实机准入：当前播放缺失歌词/封面时的在线匹配已实现；整库扫描、媒体文件写回未实现。
- 不阻断本版的理由：RC7 为 opt-in 预览，候选 CI、L3、main CI、Release、多架构镜像和公开验收均通过；GitHub Latest、Docker latest、生产和媒体文件均未被改写。
- 后续应进入的自动门禁或专项工作流：确认依赖报表失败的检测源可在受信 Linux CI 完整运行；统一 Security-Review skipped trailer scope 字段；如要批量音乐刮削，另行明确产品范围与写回策略。
