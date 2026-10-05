# KPanel v1.25.0-rc.4 发布验收记录

日期：2026-10-05（北京时间）

发布级别：L3

候选提交 / 标签：`5cabb45dec7d422f3f3c76ef9c695480f6c42d4e` / `v1.25.0-rc.4`；tree `8385acec15ef349aca991603743648e2a06cf3c7`；annotated tag object `6810795930792286827c0db51eec36a9a1d041d7`，peeled SHA 精确相同。

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。

`releaseChannel`：`preview`

`releaseTrain`：`1.25.0`

候选分支与发布后处置：`release/v1.25.0-candidate` / 预览版保留，已正常 FF 至产品 `5cabb45dec7d422f3f3c76ef9c695480f6c42d4e`。产物已发布，生产未部署。

- 原分支 / 精确 tip / 处置分类：本轮组装分支 `feature/rc4-candidate-integration` / `a7991d6826868affb3cc26183d5e7043d6a912a5` / 已纳入 RC4，远端已归档并移出活跃区；独立 publisher `release/rc4-independent-publisher` / `5cabb45dec7d422f3f3c76ef9c695480f6c42d4e` / 已发布，原分支为本地-only且已回收；RC train 候选 tip 为上述产品 SHA，按规范继续保留。
- 归档 ref 与 SHA / 远端复核：`origin/archive/feature/rc4-candidate-integration` 精确指向 `a7991d6826868affb3cc26183d5e7043d6a912a5`；对原远端 tip 和归档名预检后，使用一笔 `--atomic` push 与 exact-SHA lease 同时创建归档并移除 `origin/feature/rc4-candidate-integration`，写后 `git ls-remote` 只返回归档 ref。publisher 原分支无远端对应项，精确提交由不可变 RC4 tag `v1.25.0-rc.4` / L3 bundle SHA `40f448307546f794534dff44a13a9fc8dabb74a0d5bc959fe15610e9189ef0eb` 恢复。公开 tag/main/candidate 的产物核验见外部 `final-public-verified.json` 与发布引用原件；49项来源候选逐项见下表。
- 本地分支/upstream/worktree：本轮自有 assembly (`C:/GitHub/_codex-tasks/kpanel-v125-rc4-assembly`) 与 publisher (`C:/GitHub/_codex-tasks/kpanel-v125-rc4-release`) worktree 均 clean；除可从各自 lockfile 重建的 ignored `web/node_modules` 外无脏文件、无嵌套仓库，也无命令行指向这两路径的遗留进程。两个 worktree 均以非强制 `git worktree remove` 回收；assembly 本地 branch 在远端 archive 精确核对后删除，publisher 本地 branch 在 `origin/main` 与 RC4 tag 精确指向5c后删除。原作者/未知归属 local refs、worktree 和配套 sh worktree原件保持。
- 未完成归档项 / 责任人 / 下次复核触发：本轮自有 assembly 远端归档和 assembly/publisher 本地 refs、worktree 已完成收尾；`release/v1.25.0-candidate` 保留为 RC 序列唯一候选。其余49项中的原作者/未知归属 refs 不在本轮移除；责任人为各原作者，触发条件为所有权明确释放后，由下一协调任务逐项复核精确 tip、纳入/替代依据与恢复点，再按 §10.2 处理。

证据根：`C:/GitHub/_release-evidence/v1.25.0-rc.4`；下述外部文件均相对此目录。本文验收结论绑定产品发布点5c；之后的纯文档提交是独立记录，不改变不可变RC4源码、tag、Release或OCI digest，纯文档校验结果不混入产品L3。

## 发布画像

- 业务域：Panel 文件/图库/桌面窗口、集群指标、登录通知及轻量 Node 脚本更新；另有 CF 审计范围机器治理和 fallback 历史提案。
- 变更面：展示、只读指标、文件保存与任务启动等既有授权宿主机写入口、受管脚本依赖与下载行为、发布版本/契约；不是生产部署。风险级别 L3：跨 Windows/Linux、原子文件权限、任务恢复和跨仓库脚本来源需要精确候选完整门禁。
- 受影响用户旅程：Office 预览/编辑/保存/失败留草稿，窗口键盘/焦点/取消，Windows 多卷图库与终端选择，登录成功通知，网卡统计，脚本窗口 reload idle/reattach，轻量 Node 下载与依赖补齐。
- 未变化契约：既有 API 认证/CSRF与乐观版本、数据目录/备份事务、端口、Compose 和 Agent 权限边界保持；新增 Office/流量能力按对应类型接口实现。`kejilion.sh` 内容与 pin 因 coupled 变更而更新，不称不变；应用市场 `kpanel.conf` 的现有契约字节不变，公共默认仍 `latest`。
- 授权与所有权：用户“整合所有候选分支发布新预览版”，后续“授权发布配套脚本并继续 RC4”。主要组装交付 a799，独立 publisher 复核后冻结5c；production=false。

## 发布范围与未纳入内容

- 纳入 Office 基础 DOCX/XLSX/PPTX 预览与文本编辑、延迟统计、成功登录通知、Linux 网卡选择、统一弹窗/窗口控制、脚本窗口刷新恢复、依赖补齐与下载镜像回退，以及独立复核后的治理候选。138 文件、7700+/469-，基线为 RC3 验收 main `7621e52d87b5b7f8879cfe4bff87946dde0fb2f1`。
- 组合修复 Office Windows 原子保存创建临时文件时继承源 DACL；流量命令沿用既有 token，并明确 Windows 不支持依赖 `/proc` 的 Linux 手动网卡选择。Office 与新窗口控制均保留；Windows monitoring 不引入 Linux Docker collector。
- Office 为有界基础预览和已有文本编辑，复杂排版、公式、对象、签名文档有明确限制；ZIP/XML/图片预算、授权、版本冲突与原子保存保持。登录通知不含密码或会话凭据，认证失败不作为成功。连续流量偏移在内存，不增加 router flash 热路径写入。
- 延续 RC3 的未签名 Windows 测试发行；Azure/Authenticode/Publisher gate 不恢复。固定官方 HTTPS、SHA、限额、ACL/reparse、协议与原子事务仍保留。同源 SHA 不提供证书发布者身份证明。

精确提交清单：RC3 验收 main `7621e52d87b5b7f8879cfe4bff87946dde0fb2f1` → RC4 产品 `5cabb45dec7d422f3f3c76ef9c695480f6c42d4e` 共26提交；最后5c为空 review receipt，tree与f693一致。

| 精确提交 | 内容 |
| --- | --- |
| `507ef0570f720d3f22d18e054ca8190143f1dcdd` | feat(monitoring): chart the median latency with a lowest-to-highest band |
| `b451f69b664636287a8756cae471d76192e04c4f` | test(monitoring): budget latency medians in the size limits |
| `cf7f60e4c6f6a8afaf1183f8eb1841a7dc15bf0d` | feat(files): add self-written lightweight Office preview and text editing |
| `ff6c9ae23b1091ea7de8496b62260a4b063e891d` | fix(files): bound Office expansion and guard calculated content |
| `be172698d02ba953765ba3e8c59917917bab9f8e` | fix(files): keep Office controls readable on narrow screens |
| `b4e6e9edbb9da1844b29d9c5ba93195680c844dd` | fix(files): validate the final Office edit budget independent of order |
| `a75cbfa67b480641860be7a4be494f106684bea8` | feat(notifications): notify on successful panel sign-in |
| `7c23d8ce095eb55e00b985d1d79ac09e415c4fd5` | fix(notifications): use the panel's zh-TW two-factor term |
| `9b198ef3296afebcc2a74fefe2f51e274197fdbf` | feat(cluster): choose counted traffic interfaces and keep counters continuous |
| `fafd8a0c37f62cd28cd6f8a7c76d86301d1c0a45` | fix(cluster): group virtual interfaces and keep continuity writes off the disk path |
| `1e00063962c4f7f16275f472a3d9caed228f6a0e` | fix(desktop): resume script windows without restarting ended jobs |
| `27a8b64e8771f02d29d463b113b424fd09799b41` | fix(i18n): remove obsolete script startup phrases |
| `22c921b0067aa99be6c2e7f3bccd3c978bb6239e` | feat(web): borderless workspace windows for editors, viewers and task terminals |
| `2cb4411951f25ffba76eb9df4b1ffdff87aa5ed7` | fix(governance): support exact scoped audit commit slices |
| `d3ac7ed99ef657dee3b964ea6b22bb16f6963d81` | docs(governance): use CF audits by risk and verify costs and findings |
| `8a040c4879dec339b4bce4bbaf22f5df76834ebb` | fix(governance): reject audit slices that expand under rollback |
| `2d4ca6c90c494913274e96837c5c8315cde2ba5b` | test(governance): exercise slice identity with valid compatibility flags |
| `fbdc5d83defd230b6d28e9cb5413e12baceb098e` | docs(governance): record CF audit candidate acceptance |
| `667bb9e7f9c250b250e2ef312b7e9de0f1be2c3c` | fix: preserve Windows Office access and explain traffic selection support |
| `e98d181eb71f874c64fb9e8fce21594040babccb` | docs: refresh published RC3 and integrated candidate product facts |
| `908b30a7edf0119e68116c7afb54113b8dd8f5e1` | docs: retain stable ancestor in product review metadata |
| `f48df857422b3e2f354861421c21f4aa2d023596` | feat(node): integrate paired dependency bootstrap and mirror-aware updates |
| `26da4ed6bef4081b2ab5d70d2683ef4b2a750778` | docs: close the equivalent governance fallback proposal review |
| `a7991d6826868affb3cc26183d5e7043d6a912a5` | chore: prepare v1.25.0-rc.4 consolidated preview |
| `f693ceb06b644f508507890b77bdc67ba64f7249` | fix(release): synchronize RC4 version and paired script publication |
| `5cabb45dec7d422f3f3c76ef9c695480f6c42d4e` | chore(release): record independent RC4 source review |

### 本次来源任务分支与全部候选处置

初始本地 inventory 49 项全部覆盖：already-integrated 24、integrated 10、equivalent-already-integrated 3、excluded/superseded 11、assembled-superseded-inventory-baseline 1（下表使用原始 disposition）。原件 `C:/GitHub/_release-evidence/v1.25.0-rc.4/candidate-dispositions-r1.json`。git cherry 只是筛选，最终判断核对源码、交付、等价与当前资格；不把旧分支所有 cherry+ 盲合。

| 分支 / 精确 tip | 处置及原因 |
| --- | --- |
| `claude/awesome-clarke-pdzdcz` / `32013251c36c5738b04070897897662878328642` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `claude/compact-dialogs` / `4ed8eda42d1edf4a2b9d998c74298338fa6bec35` | integrated；重放 compact workspace 实现；组合保留 Office 分支与新控件。 |
| `claude/docker-monitor-mode` / `fac0f71c7552bb9c791aa7ea5e8fc15f2fb2c387` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `claude/gallery-cover-move` / `c9506fdfe0a4a8607b77f2d31c7cfa41fc45d2ee` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `claude/media-gallery` / `85982e5bca36a3b41df4ec881e583370bcc3ebd0` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `claude/upbeat-curie-x2ei28` / `f3899ad7f39d31fa9d71794544055225c01bc326` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/business-base-iteration-20261002` / `dbf58e9fb7b1cf84e10f98d5b7891768f8723e1a` | equivalent-already-integrated；patch-id minus-only，进一步RC1验收映射确认已采纳；不因patch-id单独宣称就绪。 |
| `docs/cf-audit-economics-20261005` / `3f0104c4c7993818e6335aaf7d8c4085a78356bb` | integrated；重放2550/d2af/6ce7/e4d7/3f010；包括slices替代，六矩阵非作者复核，未来成本收益未观察。 |
| `docs/cf-audit-slices-20261005` / `bf3aeef2a2b71f6defe382c40b164be5661ef46a` | excluded-or-superseded；被economics首提交2550完全等价替代，git diff为空；不重复重放。 |
| `docs/development-flow-governance-20261004` / `df79f1a2cb87aa2e222e18e42a4a666fb8e26f5c` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/governance-status-and-fallback-20261002` / `eead4d24b472a8aa518dcde65d010ee13050994c` | integrated；5 behavior blobs与main等价不重复；仅独有历史提案与准确采纳/六矩阵回执进入26da。 |
| `docs/readme-deduplicate-20261002` / `c1e0aa103cb8512460cc232ad3658bec93634be4` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/readme-desktop-hero-ai-20261002` / `887a44023b9c8949d80d9f3a38fb257d1385f923` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/readme-dual-mode-hero-20261002` / `386ae5791f10d565e2174f566553a964c4936fc1` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/readme-screenshots-20261002` / `642cd059b6d070f1496a07c96040683f2e265974` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/readme-tech-hero-no-logo-20261002` / `dc5c29f18873a848c5e99aa31af09cb126d8bc71` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/release-v1.25.0-rc.1-acceptance` / `5fecefc0539311ffe3f88721ce061209b019cabb` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `docs/security-audit-run2-local-20260919` / `bbc6cf920f5d0a659aa475c3d06528a4e98e858f` | excluded-or-superseded；metadata已同main；11独有攻击详情原授权local-only，未完成公开资格；不搭车发布未验证漏洞材料。 |
| `docs/windows-light-node-design` / `b4e84aac23da213bee4dd731521c4fc17b66777b` | excluded-or-superseded；被RC3真实Windows实现替代；旧Authenticode/直接IRM提案不复活。 |
| `feature/base-202610-go127` / `ba5ecf1fbd9e91e74544871b9d79ce3145f89ff3` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/base-202610-node26` / `fab9ff336e485400718cce43356f945829a70ea6` | excluded-or-superseded；Current实验候选，非LTS采用，旧Go1.26.7会回退当前Go1.27.1固定Runner；保留实验原件，复核2026-10-15。 |
| `feature/base-202610-ocr` / `3b1bc710324d508c634c3098b5564c4384ad2afc` | equivalent-already-integrated；patch-id minus-only，进一步RC1验收映射确认已采纳；不因patch-id单独宣称就绪。 |
| `feature/base-202610-ts7` / `f124abf49e0003bb2bffd5ea4c930b43a4543d13` | excluded-or-superseded；TS7 native与TS6 API companion双编译试行；Vue API支持及组合Node26资格不足，保留原候选，不冒充全面原生/性能收益，复核2026-10-15。 |
| `feature/latency-median-band` / `8d2bd74236a0b5b051e72bb0eb4e28b6f277c6bb` | integrated；重放2提交；旧L2通过仅历史，final L3新跑；小时median是权重估计保留峰值。 |
| `feature/light-node-dependencies` / `40c7ca56c7af1d09db7104097e9ffc3f5633354d` | integrated；同源生成runtime；sh deps/logger/mirror配套39a→c3a已先发布与真实Linux/root验收。 |
| `feature/light-node-procd` / `7d084570bda3a9f7af81841ea15485f3ee3e59c3` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/office-light` / `b05136738461e369ba93aeb1ad2a3256735d506e` | integrated；重放4提交；旧handoff failed未复用；组合修复Windows temp DACL并新增native回归。 |
| `feature/openwrt-telemetry` / `21240263519fc00f52bfab29ee78729c7beceb28` | equivalent-already-integrated；patch-id minus-only，进一步RC1验收映射确认已采纳；不因patch-id单独宣称就绪。 |
| `feature/panel-login-notification` / `94d8b1b6dfc41d7f64d5137a2cdef14957654695` | integrated；重放2提交；成功认证才入队，非阻塞、默认关闭，无凭据消息。 |
| `feature/rc4-candidate-integration` / `7621e52d87b5b7f8879cfe4bff87946dde0fb2f1` | assembled-superseded-inventory-baseline；盘点时7621，实际移交clean a799；final f693+same-tree5cabb继承其全部源码。 |
| `feature/rdp-oneclick-ui` / `d2b28dda114aeaee1e0d2a118ccd06875adc3143` | excluded-or-superseded；旧聚合重放差异被6a/RC3后继源码替代；盲合将带回旧Windows签名/UI/版本。 |
| `feature/terminal-duplex-input` / `8440de317a8d18814304da7d9fc5146cc969bfd1` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/terminal-job-duplex-input` / `e3f16efca68a163805e59295561331600c7a0e4c` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/terminal-multi-session-host` / `d8e19d2b9535a7a37861513a842f61eb81ca0668` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/traffic-interface-selection` / `540f64871c474d85de5a312be4770f8b2e87956c` | integrated；重放2提交；旧L2 failed未复用，radius token和Windows CLI/UI边界已修复。 |
| `feature/ts7-dual-benchmark-20261002` / `de377f9da4c55be52815dca7329389f235341906` | excluded-or-superseded；用户此前已明确归档；本地ref历史保留，不重启。旧冷构建23.786→22.173s、+29MiB不能代表完整发布。 |
| `feature/windows-light-node` / `8554b904916f7c19c787a748e836f63369d79e2b` | excluded-or-superseded；旧36plus/3minus不是就绪证明；RC3聚合与真实差异交付已替代，禁止重引签名gate。 |
| `feature/windows-node-platform` / `34169bd3f5f0211c3a8ebf013d0f475a379beb6b` | excluded-or-superseded；被RC3 Windows平台实现替代；旧分叉补丁不重复回放。 |
| `feature/windows-node-rc2` / `6a2c97974491d45884fbdc33c2d878dcf652cc57` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `feature/windows-node-runtime` / `98b080399213c9ed527943975ac40532a30f7407` | excluded-or-superseded；被RC3 runtime实现替代；旧签名链/旧契约不回放。 |
| `feature/windows-rdp-bridge` / `235fb0debf9b4b534587ce7e4d5c54886d6bb9b9` | excluded-or-superseded；被RC3 bridge聚合替代；旧架构分叉不恢复。 |
| `fix/app-script-refresh-resume` / `3ecf5ec4f0d7836ffeaecca8a0d26aa3c3c5eb66` | integrated；重放2独有提交；复用活动任务，恢复空闲窗口不自动重跑。 |
| `fix/desktop-site-status` / `10ba3d76d2fa242b6454e04d60767160b2a69323` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `fix/light-node-download-mirror` / `893a831dcfe9e3626c2ebb291a889d89296ccea6` | integrated；消费合成c3a published pin/rootSHA；原6b63不含61b230，未盲用原pin。 |
| `fix/terminal-cursor-block` / `f228f5535b8e4c3ee250a33979ff73b3c4495d0d` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `fix/terminal-reload-session-leak` / `585cc8cc174ac82ee444e81a54ca4e59a38ebc81` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `main` / `5fecefc0539311ffe3f88721ce061209b019cabb` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `release/v1.24.0-candidate` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |
| `release/v1.25.0-candidate` / `5214c36a42db1a6c7aafbeb875fda7b987c7376a` | already-integrated；精确tip是fresh7621祖先，已集成；本轮继承原件，不重放。 |


未纳入事项：Node26 Current 与 TS7/Vue 双编译实验不具备当前 LTS/完整 Vue 发布资格；旧 Windows 分叉已由 RC3 替代，禁止重引签名 gate；CF slices 被 economics 等价覆盖；run2 独有攻击链原授权 local-only，不随产品公开。它们的历史分支与原件保留，TS7 benchmark 继承此前已归档处置，不重启任务。真实 Windows 生命周期/RDP、WAN/长期 soak、当前 CF scoped/full、生产部署和自动更新实机旅程不作为本轮完成项。

## 外部审计与修复交付

- 其他 provider 非交互 CLI/tool route 不可用，fallback=provider-unavailable；独立干净 Codex publisher 执行源码复核，未参与主要组装。未召回旧 completed writer、未新建监工。
- 自由评审先落盘，再 OCR1.12.11 约束臂：123/123适用文件 reviewed、15规范排除，138逐文件处置；blind=true，H/M/L0、constrained-only0。精确自由源码f693，5c为空receipt且tree相同；4子证据摘要由 root 独立读取核对。纯验收Markdown OCR不适用。
- CF治理2550/d2af/6ce7/e4d7/3f010与fallback eead仅独有历史提案完成非作者固定六矩阵：正确性、一致性、完整性、可执行性、效率与比例性、可演进性。等价代码不重放、slices被economics替代。精确最终L3治理255/255，列表scope_complete=false失败关闭回归21项包含其中；candidate/main同5cCI通过。真实审计成本、fallback可用性与长期效率收益仍待观察，不伪造观察窗口结束。
- 当前CF覆盖 `scoped-required`：15未审提交、117文件、3新边界包；最后full run4年龄13天。历史 Windows run18为incomplete，基线不同，不能给RC4覆盖信用。本轮未执行CF scoped/full、不声称CF安全审计PASS；preview record-only，未来stable仍按当时规范判定。

- 安全审计 run / 精确源码 / 范围：历史 Windows CF run18 已结束且 incomplete，原件记录所用基线与本产品5c不同；没有该 run 对5c完成覆盖的证据。当前精确产品5c的覆盖检查原件为 `cf-preview-coverage-r1.json`，本轮未开展新 CF 审计；源码扫描与普通独立审查不能替代 CF 安全审计。
- finding fingerprint / CF 修复 commit / 独立回归：不适用（本轮没有新 CF 已确认 finding 或审计修复交付）。Office DACL与Windows流量边界普通组合缺陷修复为 `667bb9e7f9c250b250e2ef312b7e9de0f1be2c3c`，由独立源码复核、最终L3和同5c native Windows CI 验证，不给它们虚构 CF fingerprint。
- 修复交付状态：上述普通修复源码已包含在5c、已公开RC4；稳定版未发布、生产未部署。不存在本轮 stable tag 包含修复的证据。CF run18/current scope 保持未完成，RC只记录；稳定版必须按届时覆盖 decision 与规范另行准入，本文不构成免审决定或期限授权。
- OCR 观察区间与口径：本候选一次独立自由臂→约束臂对照，证据 `ocr-f693-r1/` 与 `independent-review-receipt-r1.json`；123适用文件有效review、15排除，skipped=0、unreported=0、经抽查成立的constrained-only=0，H/M/L=0。此为一个候选的文件口径，不等于123个发布周期或发现工具总有效性。
- 稳定周期观察：不适用（RC4非稳定周期）；PROJECT_RULES.md 5.4/5.5要求的多稳定周期及成本/收益观察本轮尚未开始，无三个周期结论，不宣称OCR/CF治理/fallback已经带来效率提升或应退出。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集编号 `rc4-light-node-integration-20261005`。
- KPanel 实际内置脚本基线 commit / SHA-256：RC3内置 `c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`。
- 脚本候选 commit / SHA-256：`c3a8bd895f8878d9e4ced7592c91a20c974472a5` / ROOT `d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`；CN raw `d92c6643df6acfe15e1f4556855f42eb2ada1e831ddd9f055e85a5619102cf4f`，CN归一化与ROOT一致。
- 状态依据：sh main从 `39a19cd7c1a2ef1193d07a1c40d7d897cc8f5547` 正常FF至c3a，同源feature tip；公开 exact-revision ROOT/CN真实下载与Git blob/pin逐字节核对，原件 `sh-public-c3a8bd8-r1/`。Docker ADD、生成runtime、dependency policy与updater pin同步，不能沿用RC1旧422拒绝结论。
- 兼容性证据：`paired-script-r6-verified.json`；arena-154真实Linux/root，ROOT/CN各22 dependency +35 update +smoke，无skip、18原件摘要独立复核。Windows Python skip不是PASS；历史curl35、独立reader网络/MCP大小限制照实保留。
- 本版发布决定：脚本先行公开且验证字节后，再冻结/发布KPanel RC4；未改 apps 仓库。apps公开 `kpanel.conf` 与产品实际字节SHA `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089` 一致，公共默认latest。
- 阻断或移除依赖范围：不适用（原待公开的c3a阻断已由用户授权的先行脚本发布和公开字节证据解除）；未经资格的Node26/TS7及旧签名候选按上节排除，没有将原6b63不完整pin直接纳入。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确5c Linux L3；native Windows选择用例；R7六组mock交互 | 结论限上述实测。真实Windows安装/服务/RDP/跨端PTY未验证。 |
| 网络入侵与供应链安全 | 已实现未实机验证 | 固定HTTPS/SHA/ACL/失败关闭、L3/CI扫描、公开附件/OCI绑定已验证 | CF run18 incomplete，5c覆盖scoped-required；不是渗透或CF审计通过。unsigned Windows没有证书发布者证明。 |
| 稳定性、失败恢复与兼容 | 已验证 | Office DACL/草稿/403/409/422、同job reload、安装备份恢复夹具 | 断电/WAN/systemd真实升级/Windows生命周期及长期soak未验证。 |
| 性能与资源预算 | 已验证 | L3用时902s、样本预算和浏览器cgroup观测 | 只证明门禁与测试资源边界；不证明产品峰值、吞吐或升级收益。2GiB为测试容器预算。 |
| 用户体验与可访问性 | 已验证 | R7真实theme/locale/zoom、92groups、116PNG/6trace、代表图 | 覆盖限定受影响mock旅程，非完整WCAG审计；更新设置界面本轮未做浏览器旅程。 |
| 数据、配置与迁移 | 已验证 | 既有乐观版本、权限/原子事务及安装配置恢复夹具 | 无生产数据迁移；真实宿主重启、旧库升级及用户备份恢复未验证。 |

## 自动门禁

- 标准 scripts/run-release-l3.mjs run `v1.25.0-rc.4-5cabb45d-l3-r1`；arena-154固定Runner `kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，L3 candidate-validation不并发browser。`2026-10-05T01:33:24Z`→`2026-10-05T01:48:26Z`，902s，exit0；inner/outer标准门禁PASS。
- frontend256files/2337PASS+6SKIP/2343total；Go tests/race/vet、source/npm/govuln/Trivy/source+image、amd64/arm64、安装与应用生命周期/配置备份恢复PASS。npm0、无called Go vuln；Trivy source/image0。source wall582568ms，web582532/go559619/deploy3397ms，identity unchanged；均单次执行观测，不声称产品性能或整体效率收益。
- 12远端原件全hash核对；rawlog `640f6eb2705e2f9499397d464d1718fec5cea1d9668adf1d2a494fd992af8a8a`，bundle `40f448307546f794534dff44a13a9fc8dabb74a0d5bc959fe15610e9189ef0eb`。最终ready/environment/candidate/handoff精确source/argv/log/digest通过；真实拒绝与准备失败保留。
- [候选 CI](https://github.com/kejilion/KPanel/actions/runs/37258932483) run37258932483 与 [main CI](https://github.com/kejilion/KPanel/actions/runs/37259716404) run37259716404 同精确5c、Linux/Windows两job SUCCESS；native Windows TestWindowsOfficeSavePreservesPrivateAccess 与 TestWindowsTrafficInterfacesExplainsMissingLinuxCounters 实际执行PASS，ConPTY/native counters/DACL/PowerShell发布边界/双Windows架构构建通过，部分其它包可能cached。它们不替代真实服务安装/RDP。
- [Release workflow](https://github.com/kejilion/KPanel/actions/runs/37260418435) run37260418435 同精确5c SUCCESS；Windows Check→Build→Verify三附件→transfer必须成功，下游才公开；Linux源码扫描/双架构/镜像native E2E与publish均通过，非生产部署。


- 定向测试：组装阶段7文件154/154、前端typecheck、覆盖检查21回归及governance255/255仅过程反馈；先前dirty/过时Office L2不复用为最终证据。最终精确产品L3、CI与R7承担发布门禁。
- `make verify-release`：由标准L3入口在arena-154固定Linux Runner执行，精确5c，退出码0；原件 `final-l3-r1-remote/l3-verify-release.log` 与 `final-l3-r1-verified.json`。plan SHA `3d9709a5bda43b506a4cf32147e5be63810f3023142ed0e82ccc5cdf1f819ab7`，remote脚本 SHA `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。不另跑全量L3填本文。
- SBOM/provenance：公开OCI两个SPDX/SLSAv1 attestation原始层、subject与架构绑定已验证，11 OCI原始文件digest核对；不等于完整SBOM内容审计、证书签名验证或独立新CF审计。公开镜像标准E2E见下节。
- 验收记录格式：14个H2章节与冻结模板同名同序；`report-release-metrics.mjs --validate-acceptance` 对本文 exit0。此纯文档检查不替代产品L3、候选/main CI或公开产物验证。

## 依赖与技术栈变化

- `make dependency-report`生成时间/完整性：Makefile目标为 `node scripts/report-dependency-freshness.mjs --format markdown`；本轮沿用同一报告入口的候选push run37258932352报告输出 `2026-10-05T03:19:13Z`，main push run37259716422为 `2026-10-05T03:31:10Z`，同5c、report job成功。原日志 `candidate-freshness-job-original.json` / `main-freshness-job-original.json`；10/10检测源均ok：Go、npm、toolchain/base、Docker base、Actions、security tools、Docker frontend、managed script、OCR、security-audit-skill。未为本文再次实时检测，也不把候选发现等同采用。
- 每日安全通告 / EOL：本次push下security-advisories job按条件SKIP；新的每日定时通告人工复核未验证。报告EOL状态current、下次截止2026-10-28，沿用最近复核日期2026-07-28和 `docs/security-performance-hardening-2026-07-28.md`，本轮未开展新人工EOL复核。当前锁图npm/govuln/Trivy由L3/CI/Release实际扫描，扫描通过不代表所有上游生命周期人工资格均已重审。
- 直接/基座行动项与传递归属：按 `dependency-policy.json#adoptionLifecycle`，直接和基座单独分级；跨范围传递latest交由拥有它的直接依赖/锁文件/可达安全路径处理，不逐项强升。首次完整检测后的启动/决策/处置最大天数：compatible-patch为7/14/30，minor为14/30/60，major-toolchain-base为30/90/90，emergency-security为1/3/3并服从漏洞24/72小时更严时限；采用、证据拒绝或有期限例外才算处置。未逐行追溯首次发现时间，不能宣称所有候选时限均已重新人工核验。
- 本版采用：配套sh c3a依赖补齐、logger兼容与镜像回退，同源生成runtime和精确SHA消费；Go1.27.1、Node24.21.0 LTS、TypeScript6.0.3及既有Action/基础镜像/扫描器保持，版本字段rc3→rc4同步，不修改Go/npm依赖图。
- 版本与锁定：go.mod SHA `19541abd4371ef032781ee2d35a750513a23880d9586fd41427f591754fe57b3`，go.sum `d4718070511ef8f93fa69bbef2ef65c04893bedd42e522282d2cbb13fa09073a`，web/package-lock.json `4a9db25cbc3cac0639eaf84b57123f10dd7e314d55ea396355b0db3e7b457401`，dependency-policy.json `3d8ca5c48056dc9a2255f0ec7cd3773d3203d1dce0a16ad563d3aeb88e38bbd0`；均为冻结5c文件摘要。
- 基础镜像/Action：node24.21.0-alpine固定 `sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1`，golang1.27.1-alpine固定 `sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414`，runtime为scratch。checkout `3d3c42e5aac5ba805825da76410c181273ba90b1`、setup-go `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e`、setup-node `820762786026740c76f36085b0efc47a31fe5020`；其余Action精确SHA见冻结 `.github/workflows/release.yml`（文件SHA `7c88825c99604073df1148ec55c003bd2aec5429e5a2284b0bda511a48de2402`），不追认浮动发现入口为发布pin。脚本commit/SHA见跨仓库章节，发布OCI见产物章节。
- 暂缓/拒绝：TS6.0.3→7.0.2、@types/node24.19.0→26.6.3的既有active例外owner为KPanel base maintenance，复核2026-10-15。报告另发现@types/node26.6.4，不能将它与例外候选26.6.3混称。TS退出条件为受支持Vue路径在不关闭检查情况下通过typecheck/tests/scene-pack/prodbuild；Node退出条件为批准LTS runtime转Node26且matching types通过完整门禁。Node26实验会回退Go基座，TS7不具备完整Vue采用证据，保留原候选不强合；本轮未宣称对全部其他候选做了采用决定。
- 升级结论：sh当前ROOT/CN/Linux/root、完整KPanel构建/扫描/镜像契约通过；Windows真实执行与长期资源收益未验证。受管脚本及嵌入内容恢复点分别为39a/c981/RC3，现行稳定源/锁图/镜像恢复点保持。回滚是另行授权动作，未经实机数据/配置核对不宣称已执行。

## 隔离真机与浏览器验收

- 主机/发行版/架构：arena-154登记SSH Linux主机，验收容器Linux/amd64；主机发行版名称未在本轮receipt中核验，记未验证。源码固定Go1.27.1/Node24.21.0 Runner；浏览器Chromium `140.0.7339.16` / Playwright1.55.0。公开linux/arm64为构建/digest绑定，未执行实际arm64宿主生命周期。
- 环境策略：`.governance/environment-policy.json` 的 `arena-154`；本轮仅candidate-validation、browser-validation及隔离failure-injection/public-image E2E用途，production=false。浏览器与L3错开，未借用prod-108。
- 后台命令规格：`browser-product-r7/command-spec.json`，SHA `f20905de1fb9340517b54057dee96a3db3304caa3a4ef2b5eee4503aea40f8a8`；job终态passed、exit0、900s outer，证据 `browser-product-r7/`、`browser-r7-verified.json`、`browser-r7-visual-review.json`。

- R7 job `arena-154-32356`，`2026-10-05T03:06:40.788Z`→`2026-10-05T03:15:29.044Z`；source5c/tree一致，preview-r2/fixture SHA `1158a4f418beb9040b9ce28f41c2877d2a0181d3e67ebe3ebba3bdb495ecb127`，immutable browser image `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`，Playwright1.55.0/Chrome1187。
- 六组390/100/light/zhCN、390/100/dark/zhTW、768/125/dark/enUS、768/100/light/zhCN、1280/200/light/zhCN、1280/100/dark/enUS。真实 tabs.setZoom/getZoom/CDP窗口，不以CSS/DPR替代；每组fresh context/profile，mock API GET/PUT/GET初始化共享appearance，每次navigate/reload及每张PNG前后核对真实data-theme/lang。
- 6/6、15×5+17=92 groups；116PNG/6trace逐hash/尺寸/source核验，root另独立122 artifact+8原件=130摘要与116实际appearance记录，0mismatch。发布前八张代表图视觉复核；R6机械PASS但假dark覆盖已拦截，原件不可改名为最终全矩阵PASS。
- Office保存/dirty-close/403保留/409冲突/422只读、Windows D/E图库/移动/新建/取消/Escape/focus/键盘/无API写、PowerShell/RDP chooser、certunready RDP禁用、注册optout/mock命令、Windows缺失load/swap及Windows/Linux双mock session均通过。
- scriptRestore：2idle reload+forged launch ignored、实际mock manage POST202仅1次、running reload同一server-owned job `000001a10a0ca1abbbbbbbbbbbbbbbbb` 重连/starts仍1、仅取消ownedjob POST200/cancelled/inputOpenfalse、poll200、finishedreloadidle；nativeScriptExecuted=false。mock WebSocket stream404使用已有HTTP poll fallback，不代表真实PTY/Windows/宿主Shell执行。
- errors/resourceEvents0、trace外部HTTP/credential headers0；expected console403/409/422各6、40423保留解释，不声称console0。second office-conflict-unsupported-passed错名marker实际位于Terminal focus之后、coexistence之前，不能单凭marker推定额外Office或共存断言。
- 1CPU/2GiB memory及swap/256pids/256MBshm，browser540s/SSH570s/outer900s；cgroup观察max `1762963456` bytes非峰值，OOMfalse、ownedcontainer/tunnel清理true。预算上调源于R3 OOM/R4压力，并非放宽业务断言或产品预算。
- Windows11/Server2022/2025 install/update/rollback/uninstall/reboot/RDP、WAN/真实断网/arm64/router/长期soak仍owner-deferred/尚未验证，由用户后续完成；不强加preview gate。

- 测试窗口/循环与风险：最终完整R7一次六组，script idle reload两次、显式start一次、running重连一次、ownedcancel后finished idle；源码L3一次902s，公开镜像E2E约20s。长期soak不适用本轮有限交互/事务预览资格，未提供长期稳定或真机负载承诺。
- 最小计算字号：R7在有风险的图库控制上实际断言14px可达/不裁切、键盘/焦点/取消，并保留截图；不是全站最小字号统计，不把未遍历字号写成已验证。
- 宿主写入/失败注入：R7使用内存mock任务与mock文件API，不执行真实shell、Windows PTY/RDP或宿主安装；403/409/422用于草稿保留与只读边界。L3安装/应用生命周期/备份恢复夹具包含失败注入，限其隔离rootfs/事务；没有实际生产备份恢复、系统重启或systemd升级旅程。

## 发布产物与公开仓库复核

- [GitHub prerelease](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.4) 公开于 `2026-10-05T03:53:06Z`，draft=false/prerelease=true；GitHubLatest仍v1.24.0。RC2/RC3标签和历史未改写。
- OCI version/preview index `sha256:246699dd4ec922cb8f88d89a7ff098c5bd6602207df67a15e04984690180644c`；linux/amd64 `sha256:e284a66c0314b63f569030c3bcede0613109f641fe0fee9b0b7a5a9fb597a41d`、linux/arm64 `sha256:a9348281c51cae7d9b87d9a4da76d141d3037ba9992473dda7e9827420d412b6`。manifest/config原始字节digest、revision/source/version/script labels、User65532:65532/Entrypoint、两个SPDX/provenance attestation原始layer/subject绑定通过；不是完整SBOM内容审计。
- latest/1.24.0保留 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；preview从RC3 b0db前移RC4。实际通道来源核验与用户端更新旅程的证据边界见“自更新通道验收”。
- 17附件、14唯一SHA256SUMS项均与GitHubasset digest一致。实际下载sums、metadata、Linuxamd64Node、两WindowsEXE/installer、LICENSE/notices；Windowsunsigned PE架构/零certificate与installer精确Gitblob+BOM/CRLF匹配，12 metadata blobs相同。其他binary仅核API摘要，不声称全部独立下载/执行；没有独立JSON SBOM附件，OCI提供绑定证据。

| Windows文件 | 字节数 | 实际下载SHA256 |
| --- | ---: | --- |
| `kejilion-node-windows-amd64.exe` | 10796544 | `a9360e0be51cab32952c40ad067838c343cc5ea981da89a10b7331c227f52f9d` |
| `kejilion-node-windows-arm64.exe` | 9684480 | `4811e32f6f569a4e94a1b00bbcdc1f1c82ce0021f3fb5774d3b69be345aac7e0` |
| `install-windows.ps1` | 11220 | `b29699ecf6740c9bbecdb2b4867c9bf4c3290a0d58b099c9e56736e4414b0ace` |

- 公开实际digest在arena-154/loopback18089执行标准 packaging/tests/image-e2e.sh PASS，内置ROOTscript/appsconf/VERSION/图库图标真实字节核对PASS。ownedcontainer/network/tmp清理，原件 public-remote-evidence；非生产实例验收。

- 最终原件：`final-public-verified.json`（source5c，公开时间03:53:06Z）及 `public-release.json`、`public-oci-originals-r1/receipt.json`、`public-remote-evidence/`；22原件摘要、11OCI原始文件、实际Windows三文件size/SHA核对完成。其他二进制只核API digest，不扩大成所有架构实际运行。

## 自更新通道验收

本轮L3的 `internal/selfupdate` Go包结果为ok（0.202s），源码测试对应下列策略；非verbose日志没有逐用例执行/skip清单，且R7没有更新设置页旅程，故不得把源码存在或包成功推断为以下完整实机端到端PASS。下列“已实现未实机验证”明确包括本轮对应用户旅程未验证。

| 模板场景 | 状态 | 已有证据与本轮未验证边界 |
| --- | --- | --- |
| 稳定来源只选择正式GitHub Latest；预览只接受规范stable/RC且唯一官方digest | 已实现未实机验证 | `internal/selfupdate/source_test.go` 的Stable/Preview选择、排序、歧义/来源拒绝测试；L3 selfupdate包ok。公开复核另已验证实际Latest=v1.24.0、RC4 prerelease及version/preview digest，未从真实运行Panel执行两来源检查并捕获结果。 |
| 加入preview只切来源并立即检查，不自动安装 | 已实现未实机验证 | `TestPreviewPolicyPersistsAndReturningStableNeverDowngrades`、Panel typed-policy/check mutation测试与 `docs/release-channels.md` §3契约；本轮未验证实际设置页确认→切换→立即检查、无安装旅程。 |
| 自动安装开关与一次性立即安装独立 | 已实现未实机验证 | `TestQueuedInstallBypassesHoldWithoutEnablingFutureUpdates` / `TestDisablingAutomaticInstallPreservesManualCandidate`，Panel explicit audited install；本轮没有真实Agent/systemd一次性安装及后续timer行为证据。 |
| 旧状态默认迁移stable，重启后选择保持 | 已实现未实机验证 | `TestStateSchemaOneMigratesToStableChannel` 和preview policy重新New服务的持久化测试；重新加载服务对象不等于真实宿主重启，本轮重启与旧安装状态升级未验证。 |
| 退出preview且stable较低，不产生降级候选 | 已实现未实机验证 | preview policy测试以2.0.0-rc.2→stable1.9.0检查无candidate；本轮未在真实当前RC4 Panel上切回v1.24.0来源验证。 |
| systemd后台执行、更新前备份、失败恢复、失败版本隔离 | 已实现未实机验证 | selfupdate failure/quarantine/reconcile测试、固定executor环境与L3 app-conf备份恢复夹具已存在/包通过；未在PID1=systemd真实宿主从旧公开版本执行RC4更新、断线/失败恢复/失败digest隔离。隔离镜像启动PASS不能代替这项。 |
| OpenRC与轻量Node边界显式呈现 | 已实现未实机验证 | `docs/release-channels.md` §4：OpenRC完整自动更新不支持，Node无人值守仅stable/latest；文档/代码契约可核对。真实OpenRC PID1、Windows/Linux Node unattended当前来源和UI显示本轮未完整实机验证，不把它们当preview隐式启用。 |

这些能力继承现有实现，RC4未变更自更新通道策略；公开通道保持stable默认已独立复核。尚未验证的实机旅程明确保留给后续准入，不由mock/script恢复矩阵代替、不进行生产更新。

## 生产部署安全核对

- 生产目标和部署授权：不适用（预览版禁止生产部署）；用户授权仅配套脚本与RC4公开产物/preview，production=false。
- 验证/灰度环境：登记arena-154仅隔离验收，非生产灰度；来源 `.governance/environment-policy.json`，不包含prod-108。
- 正式部署环境：不适用（预览版禁止生产部署）。
- prod-108：禁用全部KPanel操作，本次未连接、未备份、未部署、未升级、未做健康或公网核对。
- 部署前版本、健康、生产备份位置和摘要：不适用（预览版禁止生产部署）。
- 部署命令/入口：不适用（预览版禁止生产部署）；Release workflow是产物发布，不是生产部署。
- 部署后版本、Panel/Agent、重启、日志、数据完整性与公网入口：不适用（预览版禁止生产部署）。
- 生产已执行写操作：0；不适用（预览版禁止生产部署）。
- 仅隔离执行：源码/原生选择测试、rootfs安装/配置备份恢复夹具、内存mock UI任务、公开digest loopback镜像E2E；这些不是生产证据。

## 回滚

- 源码/tag：现公开不可变RC4=5c；稳定恢复点v1.24.0/ce27，前一preview RC3=5214c36a42db1a6c7aafbeb875fda7b987c7376a。RC2/RC3历史未改写；RC2没有公开镜像，不以tag推断每RC都有运行产物。
- 镜像digest：稳定 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；前一previewRC3 `sha256:b0db852ee288ee68ae66ea36994e3c608d753fd6438f6a6ee1ae86c9f5f45dc5`。sh公开旧main39a、内置c981/SHA0eb9及原件保留。
- 数据/配置备份：生产备份不适用（未部署）；L3仅隔离夹具，未制作或验证用户生产备份。
- 回滚步骤与复核：真实回滚未执行；另行明确授权后先备份数据/配置，选择精确旧digest及配套脚本/契约，按标准恢复事务，再核对版本、健康、权限与数据。不可覆盖既有tag/版本镜像，不能把退出preview开关当回滚。
- 回滚后生产实际版本/健康：不适用（无生产部署或回滚），生产当前状态本轮未核对。
- 公共指向：实际GitHub Latest=v1.24.0，Docker latest/1.24.0均为上列稳定digest；应用市场与Node公共默认stable/latest保持，Docker preview前移RC4是本轮授权范围。
- 公共默认更新通道决策：不适用（本轮未改变稳定默认，也未发生生产恢复）；不是短期保留有问题稳定版的决定。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-04T21:58:54+08:00
- 候选冻结时间：2026-10-05T09:31:39.064+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间为本轮重放latency首提交author时间；freeze后产品tree不变。RC4为新不可变预览，不计生产吞吐/生产变更失败。只报告实际L3902s与各job观测，不以成功轮掩盖返工或声称总体提速。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：未记录
- 其中生产写操作开始后异常次数：未记录
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

跨多turn没有完整总计，机器两项均未记录，不编造0或部分总数；实际生产写为0。已知失败原件保留：

- 组装governance早期两轮真实失败、dirty ready拒绝及开发Office过时L2不能复用PASS；最终精确L3/CI取代过时资格。
- paired-script R1–R5失败/准备问题、Windows skip不能算PASS，R6真实Linux/root通过；curl35、parentNode fetch/MCP过大限制独立保留，公开IWR exact bytes提供真实证明。
- 浏览器R1缺WindowsGallery fixture/错误terminal selector与超时，R2外层390s预算未到case5、missing result failclosed；R3 case5 reload超时且OOMKilled=true；R4 nativeFetch response.status()属性错误被独立发现后中断、清理；R5页面catchall continue跳过context计数，实际POST202但starts0!==1，source不改。R6最小fallback修复使机械断言通过，但真实dark被共享appearance覆盖成light，发布前视觉复核拦截。R7真实GET/PUT/GET+逐导航/截图actual-theme/lang完整重跑，所有业务断言保持。
- 外部helper/取证错误包含GBK读中文、缺失/猜测路径、PowerShell glob、JSON字段/命令引用、合同expiry六位小数被TIMESTAMP规则拒绝，及CI未完成时joblog404。原拒绝保留；只修外部入口/格式、不追认失败为PASS，不修改冻结源或扩大测试断言。最终contract-r3 handoff通过。
- 整体异常总数未完整记录，不能以成功run或空incident机器区块推定没有失败。重复的只读路径/字段核实问题责任人为publisher，后续生产写前按原唯一资格入口核实；恢复标准为正确已验证路径/格式、原失败保留和适用回归通过。

## 遗留风险与后续准入

- 本地资源回收（project-management.md §13.1）：本轮自有 assembly/publisher 两个 clean worktree 用非强制方式移除，唯一 ignored `web/node_modules` 均可由 lockfile 重建，无嵌套仓库/遗留进程；C: 空闲空间从 `96120410112` 增至 `96765976576` bytes，本次移除期间实测净增 `645566464` bytes。两分支分别由远端 archive 和不可变tag/main恢复。原作者/未知归属 worktree与local ref不回收；证据根、L3 bundle/远端原件、失败trace/截图和公开raw保留用于恢复。
- 已执行隔离清理：R7 owned容器/tunnel、公开E2E owned容器/network/tmp已清理，owned localpreview-r2按标准停止；证据分别为browser-r7-verified/public-image-e2e和preview停止记录。这些不是本地磁盘净释放字节测量，不用来宣称源树回收完成。
- 未验证风险：CF run18 incomplete/current scoped-required；unsigned Windows无证书发布者证明；真实Windows11/Server2022/2025安装/服务/升级/回滚/卸载/重启/RDP、WAN、arm64/router长期资源、完整WCAG及实际自更新通道旅程未验证。
- 已实现待实机准入：Windows能力与现有systemd自更新/恢复/Node unattended策略；测试失败原件与mock边界必须继续保留。Windows由用户后续真机验收，不强加为本次preview gate。
- 不阻断本版理由：本版仅明确授权的隔离预览，完整精确L3/native选择CI/R7真实视觉矩阵/公开产物E2E已过；Windows真实生命周期由用户owner-deferred，CF按preview record-only记录，production=false且stable/latest未变。此理由不是安全通过或未来stable豁免。
- 后续准入：稳定版另冻结并完整L3，按当时CF覆盖decision补审/明确规范允许的决定；Windows/sysupdate/OpenRC/真实arm64专项由相应owner准备可复核隔离计划。CF专用任务须遵守用户指定gpt-6-luna/max及发现/验证分离；本轮未启动新审计或自动任务。Node/TS例外于2026-10-15按退出条件复核，真实成本/收益与fallback/OCR长期观察尚未开始。
- 文档与归档收尾：本稿结构/metrics资格已独立复核；assembly archive的远端引用及本轮自有 worktree 回收有精确证据。产品不可变tag及其metadata不因后续纯文档提交而改写。
