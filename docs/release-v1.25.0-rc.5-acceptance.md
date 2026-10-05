# KPanel v1.25.0-rc.5 发布验收记录

日期：2026-10-05

发布级别：L3

候选提交 / 标签：55cc6f0acd172bb8892df34014f9c3e140f0e521 / v1.25.0-rc.5

上一稳定版本 / 回滚点：v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；稳定 OCI index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b

releaseChannel：preview

releaseTrain：1.25.0

候选分支与发布后处置：release/v1.25.0-candidate / 预览版保留；main 继续为主线

- 原分支 / 精确 tip / 处置分类：本轮登记 49 个来源分支及候选 ref，逐项处置见 C:/GitHub/_release-evidence/v1.25.0-rc.5/candidate-sources.json。纳入项从 claude/compact-dialogs、feature/latency-median-band、feature/traffic-interface-selection 选取或安全移植；其他来源按已整合、等价已整合、已淘汰或被后续实现取代处理，没有整支重放落后分支。
- 归档 ref 与 SHA / 远端复核结果：C:/GitHub/_release-evidence/v1.25.0-rc.5/candidate-remote-r1.json 记录 49 项远端复核；40 项精确归档 ref 与登记 SHA 相符，另 7 个来源 tip 可从现有 origin/archive/* 历史到达。远端活动分支只有 main 和 release/v1.25.0-candidate，两者均为 55cc6f0acd172bb8892df34014f9c3e140f0e521。未发现仍活动的来源候选分支。
- 本次来源任务分支：claude/compact-dialogs 纳入 ca5b57e、04b5d1d；feature/latency-median-band 纳入 8743bfc、88e6100；feature/traffic-interface-selection 只移植 d17d2d6、f25b2c1 中安全的 Linux 支持说明和英文文案，保留 RC4 的 Windows 不支持提示与防护；feature/rc4-candidate-integration 已由 main 覆盖，不重放。其余 45 项沿用既有分支盘点结论。
- 本地分支/upstream/worktree：产品候选 worktree C:/GitHub/_codex-tasks/kpanel-v125-rc5 保留，为正在运行的本地验收预览提供精确源码；本验收文档使用独立 docs/release-v1.25.0-rc.5-acceptance worktree。其他已有任务 worktree 和不属于本轮的本地分支未删除或改写。
- 未完成归档项 / 责任人 / 下次复核触发条件：远端来源分支归档核验已完成；release/v1.25.0-candidate 按预览规则保留。其他任务 worktree 的所有权未核实，交由其原任务负责人处理；待其任务完成再复核，不对无主身份作推断。

预览产物已公开；生产未部署。此 RC 不改变稳定更新入口。

## 发布画像

- 业务域：应用任务与脚本窗口、本地监控历史预览、轻量节点统计网卡文案。
- 变更面：展示和只读行为；没有宿主机写入、协议、数据库或部署变化。
- 受影响用户旅程：取消应用交互任务后关闭对应窗口；从应用继续打开脚本终端；查看监控长时间范围的中位延迟和最小至峰值色带；选择轻节点统计网卡时理解平台支持范围。
- 未变化契约：API、数据、端口、Compose、Agent 权限、kejilion.sh、应用市场默认 stable 更新入口均未改变。
- 风险等级及理由：L3。发布工作流构建 Windows 节点产物、双架构容器与多项供应链/运行时检查；本轮真实 Windows 安装、更新、回滚与 RDP 验收留待用户真机完成。

## 发布范围与未纳入内容

- 用户可见更新：已取消的应用交互任务在确认终止后关闭窗口；自然结束或失败仍保留结果；脚本管理终端可从应用继续打开，桌面不再重复显示终端标题；本地监控历史预览增加长时间范围中位延迟和最小/峰值示例；轻节点网卡选择说明 Linux 支持边界并修正文案。
- 冻结候选相对基线 f9fc80a9989c72acddaf8f3187927acef8fde54f 只改 16 个路径，列于 candidate-freeze-r2.json。冻结提交清单：ca5b57e234225ea8a424bd594bd570031d77e062、04b5d1ddac472f01906f3ea9f67035d60b33b7eb、8743bfcf9535d2ff7e35dac6b4a40759c857b8a8、88e61004d8d77ee90b460dbc33f66adb7f1358d9、d17d2d6dc7bcf76b82bfb76ec363c52e1fdc19c7、f25b2c18c18527e6e9cf035258ecca967634d39e、7c132cb2b1fe1f31ca367f62ab4458b386dca3e4、edb2a4a9e9a2795dabc8c9b582d00130697a1256、b38556bff11a9d095a590ab7266fea8b4689acc6、55cc6f0acd172bb8892df34014f9c3e140f0e521。
- 明确未纳入：不整支重放已过时的 Windows/网络候选；traffic-interface-selection 中移除 Windows 防护的改动被排除。RC5 未新增 Windows 运行时；真机安装、重启、更新、回滚、RDP 待用户后续验收。未运行 Cloudflare CF 专项审计，详见下节。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：当前候选 55cc6f0acd172bb8892df34014f9c3e140f0e521 没有完成 CF security-boundary-audit run。安全覆盖检查的 decision 为 scoped-required，原因包含 internal/desktopbridge、internal/desktopcredentials、internal/windowsnode 三个新边界包；覆盖检查报告 16 个待 scoped 比较的提交、117 个文件。预览发布只记录该状态，不代表 CF 审计通过。
- 覆盖检查：scoped-required；未审计提交数 16、文件数 117；应在稳定版准入前完成 scoped 审计和独立复核。现有未完成/中断 run 不计为覆盖证据。
- finding fingerprint / 修复 commit / 独立复核与回归证据：本轮没有由已完成 CF 审计确认的 finding；不得把未完成审计的 leads 写作已验证 finding。
- 修复交付状态：源码与 RC 包含本次 16 路径冻结变更；CF 审计未完成，稳定版与生产尚无审计交付结论。本轮 Windows 文件未签名；Release 明确说明官方 HTTPS 来源和 SHA256SUMS 只能校验来源路径与完整性，不提供证书发布者身份保证。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：本次验收索引未保存足以复算的 OCR 观察区间与适用候选计数，以上各值未记录；不据此宣称工具有效或应退出。
- 项目规则 5.4/5.5：本次不更新稳定周期结论；观察样本不足时不作长期有效性判断。
- 候选原始 tip、归档位置和远端核验：见前述候选处置字段及 candidate-sources.json、candidate-remote-r1.json。

## 跨仓库联动判定

- scriptLinkageState：not-required。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：本次未更改内置 kejilion.sh，基线为上一发布记录所登记的 c3a8bd895f8878d9e4ced7592c91a20c974472a5；本次不重算文件 SHA。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：冻结的 16 个变更路径不含脚本；版本契约保持不变。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。
- kejilion/apps 的 kpanel.conf 未变更，摘要 f03c75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089；应用市场默认更新通道继续为 stable。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 候选 CI、L3 全量、9/9 场景包、应用任务与脚本窗口定向回归；公开镜像 API 与 bootstrap E2E 通过 | 未在真实 Windows/RDP 与配对主机上互通验收 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 固定源代码/依赖/镜像扫描、govulncheck、npm audit、Trivy、OCI 双架构和证明材料检查通过 | CF scoped audit 尚未执行；Windows 二进制未签名，真机 SmartScreen/安装行为待测 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 安装/更新/备份/失败恢复检查、候选和主线 CI、公开 amd64 镜像 E2E 通过 | 没有生产部署、长时 soak、Windows 真机重启恢复或 NAS 验证 |
| 性能与资源预算 | 已实现未实机验证 | L3 双架构构建及运行资源检查通过；监控历史变化仅属本地预览数据 | 没有真实监控负载、低配硬件或长周期延迟测量 |
| 用户体验与可访问性 | 已实现未实机验证 | 本地 mock acceptance preview 显示 12 个月监控中位数和最小/峰值色带，选择器可用，浏览器 error 日志为 0 | 未验证 100%/125%/200% 真缩放、键盘/焦点、主题、语言全矩阵及新增任务窗口旅程 |
| 数据、配置与迁移 | 不适用 | 不改数据库、宿主配置或持久化格式 | 无迁移场景 |

## 自动门禁

- 定向测试及结果：L3 r2 在固定 Runner 上 Go/Web 检查、竞态、9/9 场景包、go vet、govulncheck、npm audit、Trivy、amd64/arm64 构建、容器运行和备份/恢复检查均通过。govulncheck 未发现可达调用路径漏洞；一项依赖模块有公告但本轮调用路径不涉及。npm audit 为 0 项；Trivy 未发现报告中的漏洞、secret 或 misconfiguration。
- make verify-release 环境和结果：未从另一入口单独运行；本次使用仓库唯一 L3 外层入口 scripts/run-release-l3.mjs，不能把同一 L3 结果重复计为另一项运行。
- L3 外层入口 run ID：v1.25.0-rc.5-55cc6f0a-l3-r2；候选 55cc6f0acd172bb8892df34014f9c3e140f0e521；基线 v1.24.0；目标 arena-154；Runner 镜像 kpanel-go127-prep-runner:go1.27.1-node24.21.0，不可变 ID sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c；开始 2026-10-05 18:05:55+08、结束 18:19:41+08、825.6 秒、pass。bundle SHA256 e8ba0a87dc955b8ee0cb1c758ca661afc5e4dcd1b6d9c17fb874b3cc76adfa82；plan SHA256 64e9b9c9b23186a0a52a60b53c8048e8bfc0494e7ca02c3ab21b4fc2ea048c58；remote script SHA256 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979；console SHA256 ff091943664efa852e40fd6729a14d527e4f908bd15e68078eaf87bc060e8fc9。
- 候选 CI：[37296013813，Success](https://github.com/kejilion/KPanel/actions/runs/37296013813)，源 SHA 相同。冻结前有一次 Windows 测试断言仍期待 Linux /proc 文案的正常门禁失败；b38556b 修正为平台支持说明，失败 run ID 未在当前验收索引记录，最终冻结 SHA 的 Windows job 和完整 CI 通过。该产品回归由门禁拦截，不计发布流程异常或已发布版本失败。
- 主线 CI：[37296639759，Success](https://github.com/kejilion/KPanel/actions/runs/37296639759)，源 SHA 相同。
- Release workflow：[37297870730，Success](https://github.com/kejilion/KPanel/actions/runs/37297870730)，windows-node 与 release job 成功；依赖新鲜度 main [37296639745](https://github.com/kejilion/KPanel/actions/runs/37296639745) 和 tag [37297870671](https://github.com/kejilion/KPanel/actions/runs/37297870671) 整体成功。security-advisories 子任务没有提供本轮执行证据。
- 安全扫描、镜像契约、SBOM/provenance：源码、依赖、容器扫描和镜像运行契约均为 L3/release CI 通过；OCI manifest 附有双平台 attestation。CF 审计仍为 scoped-required，不能由这些自动扫描替代。

## 依赖与技术栈变化

- make dependency-report 生成时间及检测源完整性：本轮未单独运行 make dependency-report；依赖新鲜度 workflow 的 main/tag 两次报告均成功，GitHub REST API 独立调用受公开限流，状态依据为公开 Actions 页面。
- 最近每日安全通告审计、EOL 复核状态及证据：本轮没有完成单独每日 advisory 或 EOL 复核。dependency-freshness 整体成功不代表被跳过的 security-advisories job 已审计。
- 直接/基座行动项、传递依赖归属信号及期限：L3 npm audit 0 项；Go 扫描没有可达调用路径漏洞，仍有一项未调用依赖模块公告，后续在稳定版冻结前复核升级条件。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：无依赖、工具链、基础镜像、Action 或受管脚本升级。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：web 锁文件只随依赖安装状态复核，版本没有新增运行时依赖；RC5 预览 OCI index 为 sha256:a861d37bd78e664dcb2a7c89c3346750df68286946cc27497b5ba690b473278f。
- 暂缓或拒绝候选：未把含删除 Windows unsupported 提示和平台 guard 的旧 traffic-interface-selection 树合入；无新增依赖升级候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：不涉及依赖升级；tag 与 preview 使用同一 OCI index，stable latest 保持原 digest。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：公开镜像 E2E 在登记隔离环境 arena-154 的 Linux/amd64 运行；本地浏览器为 Codex IAB，127.0.0.1:4184。
- 环境策略 ID 与允许用途：arena-154 用于 L3/隔离验收；prod-108 全程禁用。浏览器 preview 使用本地 mock，不连接生产服务。
- 使用的精确候选或公开产物：源码 55cc6f0acd172bb8892df34014f9c3e140f0e521；E2E 镜像 docker.io/kjlion/kejilion-panel@sha256:a861d37bd78e664dcb2a7c89c3346750df68286946cc27497b5ba690b473278f。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 ID v1.25.0-rc.5-55cc6f0a-l3-r2，pass/exit 0，825.6 秒；公开镜像 E2E pass/exit 0，2026-10-05 11:01:16Z 至 11:01:36Z，命令 sh packaging/tests/image-e2e.sh docker.io/kjlion/kejilion-panel@sha256:a861d37bd78e664dcb2a7c89c3346750df68286946cc27497b5ba690b473278f 18091。原始证据位于 C:/GitHub/_release-evidence/v1.25.0-rc.5/public-e2e-r1/image-e2e.json；image-e2e 脚本 SHA256 1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4，日志 SHA256 d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9。
- 测试窗口/循环数及风险依据：公开镜像单次完整 smoke；未做 soak，因为本版为预览且无长时运行需求证据。
- 受影响用户旅程、视口、缩放、最小计算字号、主题、键盘/焦点、语言和失败态：本地 mock 预览只打开历史监控并切换 12 个月；验证页面说明为区间中位延迟，最多 3 条显示最低到最高色带，浏览器 console error 0。未声称完整浏览器验收。
- 宿主机写入、失败注入、重启恢复和回滚结果：公开 E2E 仅在本地容器测试；bootstrap 返回 201 且设置 Secure cookie，版本/API、路由、公共静态文件比较和容器健康均通过。测试容器、网络和临时数据已删除；没有生产或宿主机写入。
- 未执行场景及原因：真实 Windows 节点安装、更新、回滚、重启和 RDP 由用户后续真机测试；100%/125%/200% 原生缩放、多个主题/语言、键盘/焦点全旅程及 soak 未执行。

## 发布产物与公开仓库复核

- GitHub Release 状态：[v1.25.0-rc.5](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.5) 于 2026-10-05 10:52 UTC 公开，draft=false、prerelease=true、非 Latest；页面固定在 55cc6f0。Assets 总数 19（17 个上传文件及 GitHub 自动生成的两个源码归档）。[GitHub Latest](https://github.com/kejilion/KPanel/releases/latest) 仍指向稳定版 v1.24.0。
- Docker 版本与通道 OCI index：docker.io/kjlion/kejilion-panel:1.25.0-rc.5 与 :preview 同为 sha256:a861d37bd78e664dcb2a7c89c3346750df68286946cc27497b5ba690b473278f；:latest 保持 sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b。
- linux/amd64：sha256:063f3d437e2271bf447372463f67fb7d3483c7a15d81953e1feb19c4d71a2434。
- linux/arm64：sha256:71376057ee652b1adcf8c361b3598aed413fc060b658b7a0b914d13c1fafb87b。
- 附件及 SHA256SUMS：Release 页面有 Windows amd64/arm64 节点 EXE、安装脚本和 SHA256SUMS；未逐个下载执行全部附件。Windows 节点未签名，SHA256 不证明发布者身份。
- 公开镜像 image_e2e=pass：以 digest 固定的公开 linux/amd64 镜像执行 packaging/tests/image-e2e.sh，退出码 0。该结果不等同生产部署，也不代表 arm64 主机运行测试。
- kejilion/apps / kejilion.sh 契约结论：应用市场 kpanel.conf 摘要未变；默认稳定通道保留。脚本基线不变，应用市场无需提交。

## 自更新通道验收

- 稳定来源仍只选择正式 GitHub Latest；预览发布不改变稳定来源，镜像由唯一 OCI index digest 固定。
- 加入预览只切换来源并立即检查，没有自动安装：本轮不改更新器配置或用户设置。
- 自动安装开关与一次性立即安装相互独立：代码未改。
- 旧状态默认迁移到 stable，重启后通道选择保持：代码未改，未重复跑全量状态迁移测试。
- 退出预览且稳定版较低时没有产生降级候选：该更新策略未改；退出预览不会自动降级已安装预览版。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：L3 相关门禁通过；真实主机部署和数据恢复未测试。
- OpenRC 与轻量 Node 当前边界按 docs/release-channels.md 呈现；本轮没有扩大兼容平台承诺。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：arena-154 只用于隔离 L3 和镜像 E2E；未用于生产。
- 正式部署环境：不适用（预览版禁止生产部署）。
- prod-108：禁用全部 KPanel 操作；确认本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：不适用。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：不适用。
- 生产已执行写操作：无。
- 仅在隔离真机执行、未在生产执行的场景：L3 和公开镜像 smoke；没有执行生产动作。

## 回滚

- 源码/tag：回滚准点 v1.24.0 / ce27dc5171a97ed6e3d9475cddfdfac89762aad3；不移动或覆盖 v1.25.0-rc.5 不可变 tag。
- 镜像 digest：稳定 latest index sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b。
- 数据/配置备份：无生产数据或配置写入，无需本次回滚备份。
- 回滚步骤和回滚后复核：预览用户可按现有通道流程退出预览并手动选择稳定来源；退出预览不会自动降级。
- 回滚后生产实际版本与健康状态：不适用，本轮没有生产部署或回滚。
- GitHub Latest、Docker latest 与标准更新入口实际指向：仍为 v1.24.0 / 原稳定 digest，均未改变。
- 公共默认更新通道决策：不适用；stable 继续 v1.24.0，RC5 只供主动加入预览的用户。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-05T16:00:37+08:00
- 候选冻结时间：2026-10-05T18:03:59+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

预览节奏补充：首个纳入提交到 GitHub Release 公开约 2 小时 52 分；GitHub Release 页面发布时间只精确到分钟。冻结到公开发布约 49 分钟。Release workflow 自身约 14 分 39 秒。以上为 RC 公开节奏，不计作提交到生产时间。冻结前修正的 Windows 测试断言由候选 CI 发现，未进入公开产物；没有公开版本失败、生产退化、回滚、紧急热修复或重复发布。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

本轮三个流程异常均发生在生产写操作前，没有生产写入、产品回滚或用户数据影响。两次只读发布核验命令修正后获得有效证据；一次交接预检拒绝无效时间精度，规范化后通过。流程异常不等同产品失败。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/ssh-runner-identity/powershell-remote-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 arena-154 只读端口可用性探测的 PowerShell 到 SSH 引号传递错误，远程命令未运行；没有远端写操作。",
    "recoveryEvidence": "随后使用 Base64-safe 只读入口确认 127.0.0.1:18091 可用；public-e2e-r1/image-e2e.json 记录同端口公开镜像测试通过且容器、网络和临时数据已清理。首次拒绝留在工具记录中。",
    "permanentAction": "与 v1.24.0 出现的相同指纹复发。负责人：release owner；复核日期：2026-10-10；下一次 L3 生产写前将端口/Runner 预检固化为结构化 argv helper 并加参数回归。退出条件：固定入口能首轮核对 Runner 身份和端口空闲且回归通过；关闭前不开始下一次 L3 生产写。",
    "historicalReleases": ["v1.24.0"]
  },
  {
    "fingerprint": "public-release-verification/registry-inspection/docker-version-tag-prefix",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次只读 Registry 查询错误把 Git tag 的 v 前缀拼入 Docker version tag，返回 not found；未写 Registry。",
    "recoveryEvidence": "改用 docker.io/kjlion/kejilion-panel:1.25.0-rc.5 后确认版本 tag 与 preview index 同为 sha256:a861d37bd78e664dcb2a7c89c3346750df68286946cc27497b5ba690b473278f；完整 registry-index-r1.txt SHA256 为 6e1c56ea75a1c0b3d89e45ddff21b456cc2b7ea03e4c9ab0c4b0087bbe592a8d。",
    "permanentAction": "负责人：release owner；复核日期：2026-10-10；将 Git tag 到 Docker version tag 的无 v 前缀映射加入发布校验器参数测试。退出条件：固定验证器首轮同时核对版本 tag、preview、latest 三个 index；关闭前不开始下一次 L3 生产写。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-handoff/task-preflight/fractional-seconds-precision",
    "position": "before-production-write",
    "count": 1,
    "impact": "task-preflight handoff 不接受 L3 与公开 E2E 原始凭据中超过三位的小数秒，首次交接预检拒绝两条 evidence receipt；候选、tag 与镜像未改动。",
    "recoveryEvidence": "首次拒绝结构化记录为 C:/GitHub/_release-evidence/v1.25.0-rc.5/handoff-preflight-r1-first-failure.json，SHA256 fb94ee6ddc0d56cf9e2cb5850f6312b9c30fa216c786b564801b19b55b91d0f6。只将 contract receipt 时间精度规范化到毫秒，不改原始日志；同一 handoff 预检随后 passed。",
    "permanentAction": "负责人：release workflow owner；复核日期：2026-10-10；在 receipt 生成入口统一到 task-preflight 接受的 ISO 毫秒格式，并用实际 L3/E2E 原始时间回归。退出条件：无手工二次改写时新 receipt 的 handoff 首轮通过；关闭前不开始下一次 L3 生产写。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：arena-154 公开 E2E 容器、网络和临时数据已回收；完整证据保留在 C:/GitHub/_release-evidence/v1.25.0-rc.5。RC5 产品 worktree、web/node_modules、本地 mock 预览及当前 Codex 浏览器页保留给用户检查，未登记为已释放空间。其他任务的 worktree 和分支不属于本次清理范围。
- 未验证风险：CF scoped audit、Windows 真机安装/更新/回滚/重启/RDP、arm64 实际容器运行、原生浏览器缩放矩阵、长时间监控负载。
- 已实现待实机准入：Windows 节点构建产物和安装脚本已发布但未签名；公开页面提供固定官方来源与 SHA256SUMS 校验说明。签名不提供发布者身份时不补写“可信签名”结论。
- 不阻断本版的理由：用户明确表示 Windows 真机后续测试；预览流程未改 GitHub Latest、Docker latest、stable 更新入口或生产系统。CF 覆盖项作为 RC 风险记录，稳定准入前必须完成。
- 后续应进入的自动门禁或专项工作流：完成 CF scoped audit 与独立验证；为 Windows 首次安装/更新/回滚/RDP 建立真实机验收；统一 SSH argv、Docker tag 映射和 release timestamp receipt helper，关闭本次 3 个流程异常的行动项。
