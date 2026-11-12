# KPanel v1.30.0 发布验收记录

日期：2026-11-12

发布级别：L3。产物已发布，公开验收已通过；生产未部署。

候选提交 / 标签：cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa / v1.30.0；annotated tag object 5ae7be919d03e50d665af6ff6cfe7436d1c24dab。

上一稳定版本 / 回滚点：v1.29.0 / sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6。

releaseChannel：stable；releaseTrain：1.30.0。

候选分支与发布后处置：release/v1.30.0-candidate / 稳定版已发布并归档。

- 原分支 / 精确 tip / 处置分类：release/v1.30.0-candidate @ cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa；已由稳定 tag、GitHub Release、稳定镜像和公开验收覆盖。
- 归档 ref 与 SHA / 远端复核结果：origin/archive/release/v1.30.0-candidate 指向 cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa；origin/release/v1.30.0-candidate 已不存在；远端 main 和 v1.30.0 peeled commit 均为同一 SHA。
- 来源任务分支：已发布的桌面目录、图标、壁纸和浏览器首页来源见 [RC14 验收记录](release-v1.30.0-rc.14-acceptance.md)。本次未写入或移除来源任务分支；远端活跃和 archive 引用查询均无结果，相关本地工作树依原记录保留，所有权尚未重新确认。
- 本地分支/upstream/worktree：本地分支已归档命名为 archive/release/v1.30.0-candidate，跟踪 origin/archive/release/v1.30.0-candidate；worktree C:/GitHub/_release-candidates/kpanel-v130 保留且 clean，HEAD 为候选 SHA。未删除工作树或回收磁盘内容。
- 未完成归档项 / 责任人 / 下次复核触发条件：来源任务本地工作树保留；由各来源任务原作者释放所有权后，在下一轮候选盘点时逐项核实，不按分支名批量清理。

本次授权为发布稳定产物，不包含生产部署；生产状态在下文独立记录。

## 发布画像

- 业务域：桌面快捷方式图标、桌面与浏览器入口、音乐/影视体验、站点访问保护、终端、Agent 与 AI/MCP 文件读取。
- 变更面：RC14 已验证的用户界面与 Agent 行为；稳定候选只改版本元数据、稳定发布说明和来源身份记录，没有新增产品行为代码。
- 受影响用户旅程：桌面和浏览器首页打开应用/网站/URL 快捷方式；导入或自动识别网站图标；媒体偏好同步和按需资料匹配；启用站点访问密码；手机终端使用 Ctrl 辅助键；Agent 与 AI/MCP 读取失败时维持受限结果。
- 用户可见更新：桌面 URL 快捷方式可读取网站公开图标并支持图片 URL 自定义图标；音乐、图片和影视目录/播放位置可同步，音乐支持在线歌词与封面，影视资料按需匹配；增加站点访问密码与手机终端 Ctrl 辅助键；音乐列表改为紧凑单行，网站入口支持内置或系统浏览器，壁纸切换统一效果，历史监控分组显示，影视扫描逐步反馈已发现内容。
- 明确边界：影视资料按需匹配，不是全库后台自动刮削。生产环境、AI 服务商真实账号以及低配长时间负载未在本次公开验收中实测。
- 未变化契约：稳定候选未新增数据库 schema、API/Agent 协议、端口、Compose 结构、主机权限或 kejilion.sh 生命周期契约。
- 风险等级：中低；正式稳定产物已公开，部署仍需管理员按更新流程自行选择并提前备份。

## 发布范围与未纳入内容

- 精确提交清单：最终稳定候选 cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa；产品实现基线为 v1.30.0-rc.14 / 16fa206dd5239ec37de5d095bffa168e5a74f9a2。稳定候选将版本固定为 1.30.0，增加稳定变更记录并更新来源身份，不改写已验证的 RC14 产品行为。
- 稳定版包含此前 RC1-RC14 已纳入且在对应记录中有证据的功能；不将 RC14 之外未集成的本地候选带入稳定版。
- 明确未纳入：生产部署、真实外部 AI 提供商端到端请求、实体手机专项/读屏矩阵、低配 soak，以及来源任务分支/工作树清理。

## 外部审计与修复交付

- 从 v1.29.0 到稳定候选运行 security-review-scope：17 个敏感包组 recorded，missing=0、stale=0；invalid_trailer=true。一个历史 Security-Review trailer 不符合当前解析器格式，未将其计作覆盖；不宣称全仓无漏洞。
- 稳定候选独立复核结果 PASS，提交范围是发布元数据和版本字面量；因非产品行为变更，OCR 与该版本字面量的独立安全行为审查按提交 trailer 标记 skipped。
- 候选和发布级 L3 的 Go 调用路径漏洞扫描、依赖审计、源码/秘密/配置扫描、原生镜像扫描及生命周期检查均通过。扫描通过不等于零漏洞结论。
- 修复交付：稳定 tag 与公开 Release 的候选 SHA 完全一致；生产部署未执行。

## 跨仓库联动判定

- scriptLinkageState：not-required；本版未修改或单独发布 kejilion/sh。
- KPanel 内置脚本基线：kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff；SHA-256 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。
- 脚本候选和变更集编号：不适用。稳定镜像 amd64/arm64 标签均绑定上述脚本 revision 和摘要；发布生命周期检查通过。
- 阻断或移除的跨仓依赖：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 CI、主线 CI、L3、稳定镜像公开浏览器与隔离容器验收 | 未对生产主机或外部 AI 服务商执行真实业务操作 |
| 网络入侵与供应链安全 | 已验证 | 安全范围核对、Go/Node/源码扫描、镜像扫描、provenance、附件摘要及双架构身份验证 | 有效 review 覆盖不构成无漏洞保证；保留一条历史 trailer 解析异常 |
| 稳定性、失败恢复与兼容 | 已验证 | Panel/Agent 重启、持久化状态、证书故障回退和发布镜像升级旅程 | 未在生产安装、未执行生产回滚、未做长时间 soak |
| 性能与资源预算 | 已实现未实机验证 | L3 与资源检查通过；用户可见旅程在 GitHub-hosted 环境完成 | arena-154 公钥尚未安装，未运行登记主机性能对照或低配长期压力测试 |
| 用户体验与可访问性 | 已验证 | 真实浏览器公开验收覆盖初始、重启、发布和升级路径；升级界面含 320/390/1280 像素截图 | 没有实体手机触控、完整读屏和 200% 缩放专项结论 |
| 数据、配置与迁移 | 已验证 | 偏好、终端历史、桌面状态和 Agent 重启后读取均通过 | 未做生产数据迁移或完整生产备份恢复 |

## 自动门禁

- 候选 CI：run [43166441085](https://github.com/kejilion/KPanel/actions/runs/43166441085)，精确 SHA cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa；Windows Node job 131009307000 与 Linux verify job 131009307319 均成功。
- 主线 CI：run [43169035994](https://github.com/kejilion/KPanel/actions/runs/43169035994)，同一 SHA；Windows Node job 131017600594 与 Linux verify job 131017600978 均成功。
- 依赖新鲜度：候选 run [43166441143](https://github.com/kejilion/KPanel/actions/runs/43166441143) 与主线 run [43169035989](https://github.com/kejilion/KPanel/actions/runs/43169035989) 均以该 SHA 成功。
- GitHub-hosted L3：Release run [43171361924](https://github.com/kejilion/KPanel/actions/runs/43171361924)，job 131025180684；2026-11-12T06:54:32.784Z 至 07:10:56.874Z，状态 passed。Runner ubuntu24 / 20261101.360.1；Go 1.27.4 linux/amd64、Node v24.22.1、Docker 28.0.4；入口 SHA-256 5bfcf66870b49edb9abd2895e818ac2bdc8323579aad06e43946ee2f073f5a7d。L3 的治理、Web、Go、部署及 release-level 验证 lane 均成功。
- Release workflow：run 43171361924，Release job 131031714040 成功；GitHub-hosted L3、Windows、发布说明、来源身份、依赖/安全扫描、生命周期、二进制、容器构建与扫描、Registry 未占用保护、多架构推送和恢复输入冻结均通过。
- 公开验收：job 131041638152 成功，状态 passed；真实浏览器与隔离 Panel/Agent 容器，mocked=false。初始 48、重启 26、公开镜像 47、升级 35，共 156 项；错误数与附件失败数均为 0。Docker inventory 与测试 CA 使用登记 fixture；未将测试环境冒充生产。
- 稳定收尾：job 131042588747 成功，远端候选分支归档；preview-completion 按稳定发布流程跳过。
- 发布说明门禁：最终正文 8 条更新摘要、2 条升级提示；notes SHA-256 c650a881434c8365ca0c859c0faa73b5f656f8c0313cf5ca2fcba73b406a281a。公开面板更新接口回读版本、摘要、Release URL 和升级提示均与产物一致。
- 源码身份：first-party source SHA-256 a91de5c61237171eede4681857bbb825bae31d9a31c0b04fdae46f8342414831，1316 个源文件；相对 RC14 的 Go 产品源差异限于版本字面量。

## 依赖与技术栈变化

- 候选与主线依赖新鲜度报告均通过；本稳定版本未引入 RC14 之后的依赖、锁文件、构建目标、Action 或基础镜像升级。
- 发布工具链：Go 1.27.4、Node v24.22.1、Docker 28.0.4；Go 1.27.4 已随本发布列车之前的 RC 集成，不是本稳定版本首次引入。
- 镜像内置脚本 revision 与 SHA-256 固定，见“跨仓库联动判定”。延期的依赖行动项仍按依赖策略处理，本次未把候选信号等同于漏洞。

## 隔离真机与浏览器验收

- 环境：GitHub-hosted Ubuntu 24 runner、真实 Docker 镜像、真实隔离 Panel/Agent 容器和浏览器；不是生产主机。L3 回执和 public-43171361924-1 artifact 保留完整报告、容器状态与清理证据。
- 使用的精确产物：v1.30.0 / cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa / OCI index sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e。
- 初始、重启、公开镜像和升级旅程共 156 项通过；包括认证图标路由、应用目录、偏好与终端历史持久化、桌面重载、稳定版本发现、浏览器 UI、升级错误回退及资源清理。浏览器错误、断言错误和附件失败均为 0。
- 失败路径包含 systemd 不可用时更新启动返回 retryable 503；安装开关保持关闭且没有生成安装请求。该项是隔离故障注入，不是生产更新失败。
- 视口截图覆盖 320、390 和 1280 像素；未宣称实体手机、读屏或所有缩放组合通过。arena-154 未连接，低配及长时资源压力未验证。

## 发布产物与公开仓库复核

- [GitHub Release v1.30.0](https://github.com/kejilion/KPanel/releases/tag/v1.30.0)，ID 442616635，2026-11-12T07:35:27Z 公开；draft=false、prerelease=false；GitHub Latest 查询返回 v1.30.0。
- Docker Hub 版本标签 1.30.0 与 latest 的 OCI index 均为 sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e。linux/amd64 为 sha256:38f8362161869b40854ecc0be189b5bd25531547ba8917342bc6a7019ef549ee；linux/arm64 为 sha256:121982dbf8e61da70f79448be4f78a5a7a8fc3c100420a867564128ce4f5bc5e。
- Docker preview 发布后仍指向 RC14：sha256:0fe00dc1800ce990f67d8882d50f49eb704732d504ca07e4516a78c3eaafd3da；amd64 sha256:790ff0c606e15d9b2177f57aa66c1da723822d26995b44c83a51e508441b329f，arm64 sha256:e3ef770ecec5829db7bbba82c0e5088d36e27ff5608bf759245bc6929816262a。
- 公开附件 19 个；SHA256SUMS 覆盖 15 个发布文件，附件身份、文件大小和摘要由发布前冻结清单与公开验收核对通过。
- image_e2e=pass；稳定镜像标签、面板更新信息接口、浏览器更新旅程、Agent revision 与公开 Release 均绑定精确候选和 digest。
- kejilion/apps 与 kejilion.sh 继续使用既有稳定入口及生命周期契约，本次未发布脚本仓库新版本；脚本生命周期 L3 检查通过。

## 自更新通道验收

- 稳定来源解析 GitHub Latest：公开镜像重启阶段读取 1.30.0 和上述精确 OCI digest；发布记录将 stable/latest 都设为正式稳定版。
- preview 仍为规范 RC14 且 digest 未变化；预览切换和退出时不自动安装/降级的边界按同代码 RC14 公开验收记录复用，见 [RC14 验收记录](release-v1.30.0-rc.14-acceptance.md)。
- 本次升级公开旅程中自动安装开关保持关闭、installRequested=false；systemd 故障注入返回可重试错误，不触发隐式安装。
- systemd 前置备份、失败恢复和通道状态迁移按 RC14 已验证路径执行；OpenRC 与轻量 Node 边界仍遵守 docs/release-channels.md。

## 生产部署安全核对

- 生产目标与部署授权：本次仅授权发布产物，不含生产部署。
- arena-154：未连接；其共享 SSH 公钥仍未安装。本次使用 GitHub-hosted 验收环境，不能替代主机实测。
- prod-108：本次未连接、未备份、未部署、未升级或核对。
- 生产备份、生产写操作、生产公网健康与生产回滚：不适用（生产未部署）。

## 回滚

- 发布源码/tag：v1.30.0 / cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa。
- 稳定回滚目标：v1.29.0 / sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6。
- 回退前按升级说明备份 Panel、Agent 数据、媒体偏好和自定义宿主脚本；媒体歌词/封面旁车文件按原数据策略保留。降级需人工选择固定目标 digest，退出 preview 不会自动降级。
- 当前 GitHub Latest、Docker latest 与标准稳定更新来源指向 v1.30.0；preview 保持 RC14。未执行生产回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-12T11:34:17+08:00
- 候选冻结时间：2026-11-12T14:03:32+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/scan-scope/stale-source-digest",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一发布列车 RC14 首轮候选 CI run 43149839953 在 release version literal 变更后发现 firstPartySourceSha256 陈旧；当时没有提升 main、创建 tag、公开 Release 或推送镜像。稳定发布流程本身无失败。",
    "recoveryEvidence": "原失败 run https://github.com/kejilion/KPanel/actions/runs/43149839953 已保留；RC14 精确候选重新通过 CI run 43151185428 和 Release run 43152246039；稳定候选 cdbdd2b 的 CI、主线 CI、L3、Release 与公开验收均成功。",
    "permanentAction": "稳定候选提交前运行 scripts/task-preflight.mjs ready/handoff 与 source-identity；对版本 Go 字面量更新先复核来源摘要，再提交并对精确候选重跑检查。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- AI/MCP 文件读取变更通过精确候选 CI、L3 与供应链扫描；本次公开容器未连接真实 AI 服务商，也未覆盖所有工具组合，不能由此推出所有 AI 请求均无失败。
- arena-154 未连接，因此没有低端硬件性能数据或长期负载结论；在管理员安装共享 SSH 公钥并按 environment-policy 登记验证后复核。
- 影视资料匹配为按需使用；没有整库后台自动刮削承诺。
- RC14 记录的来源任务本地工作树未在本次强制清理。原作者释放所有权后，在下一轮候选盘点中核对是否归档或保留。
- 本次没有生产部署、生产数据迁移或磁盘回收；不报告生产完成时间、生产回滚或磁盘净释放量。
