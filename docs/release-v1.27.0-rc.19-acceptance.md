# KPanel v1.27.0-rc.19 发布验收记录

日期：2026-10-21。RC19 已公开；原发布根运行在公开浏览器步骤失败，随后同一不可变产物只读恢复验收通过。原失败保留，根因未确认。

- releaseChannel=preview，releaseTrain=1.27.0。
- 精确候选 `afb41280b59ad326f84665aa74cc380be88f2db8`，tree `276d3828d8215ebb294ab916a8517256a879fc55`；批准基线/上一预览 RC18 `c89ee44d3d8f7552e4aa732718f4a50aaeb26474`。
- 上一预览回滚镜像 `sha256:5eb0cd7ff4a469a017f3570767c148339bee460c872d7dea7b26a5cc51abdca2`；回滚未执行，退出 preview 不自动降级。
- 独立回读确认稳定版仍为 v1.26.0 / `sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`，GitHub Latest 与 Docker latest 均未推进。

## 新候选、业务范围与授权

| 新候选 | 冻结 tip | 内容 |
| --- | --- | --- |
| perf/login-entry | eed74c710ed03293248e4f2a151bdd4635c499a9 | 3 提交：登录/已有会话初始化、异步偏好写回、成功预算和 Session 原子落盘 |
| candidate/terminal-global-optimization | 1b2bc25503b866ee0b56a466aea5409090d76cfe | 完整 12 提交：本机/跨节点能力协商、恢复和身份边界、共享输出与空读限速 |

两组精确基线均为 RC18，接手时工作树 clean，来源交付已完成。登录来源聊天随后进行另一个线上只读检查，不修改本轮冻结代码；发布者不参与该线上检查、不召回作者。来源文件不重叠，octopus merge 无冲突，保留历史；加合并与版本准备后共 17 提交。RC18 已发布内容、旧候选和未知工作区不重复纳入。

用户明确授权重新整理候选并发布新预览，包含本轮唯一候选推送、main/tag、Release 和镜像；生产部署不在范围内。主管理工作树干净但落后，保持只读；唯一列车为 release/v1.27.0-candidate。

## 契约、安全与资源边界

- 仍为单管理员轻量 Linux Panel/Agent 控制面，宿主机原生资源和脚本为业务真源，无新端口、依赖或常驻服务。
- bootstrap 状态携带可选已认证 Session，保留旧端 /auth/session 回退；继续验证会话撤销/期限，缺失或失效 CSRF Cookie 返回空写入 token，no-store 不缓存身份。Argon2id、两步验证、审计、限流与会话期限不降低。
- 成功登录预算与 Session 一次持锁落盘，失败整体回滚；偏好读取/恢复仍先于挂载，保存与冲突重试不再阻塞导航，继续串行、版本冲突和账号隔离。
- 新终端能力提示仅帮助选择角色，Noise、scope 与实际 PTY owner 继续授权；一次协商复用当前请求能力，不跨 Agent 重启缓存。旧目标回退在发送 open 前决定，未知创建/输入结果不重放。
- 接收期限 45 秒独立于发送心跳，随机退避有上限，明确身份拒绝停止重拨；本地寿命使用本地时间，远端不能将临时请求变为保留会话。重复句柄和迟到注册不覆盖既有身份。
- 首块/安静后的输出即时发送，连续突发仍合并并限长；无变化且过快返回的兼容后端至少 100ms 再读，正常长轮询不另加等待。订阅拒绝保持原订阅，不启动部分新增 pump。可能增加小突发事件或最多约 100ms 空读发现等待，不宣称所有资源指标改善。
- 不改变数据库格式、安装脚本或配对存储，无迁移/双写。旧版回滚仍不保证活动终端跨重启恢复，已执行主机动作不撤销。

## 性能证据与不可外推范围

- 登录来源同机受控浏览器每场景每臂 3 次，中位数：已有 Session、每 API 注入 250ms 时 1444.1→964.9ms；偏好首次迁移另注入 1500ms 写入时 1938.8→333.5ms。普通密码登录 100ms 条件 314.4→317.2ms，250ms 条件 629.4→615.3ms，未证明显著改善。真实认证接口/构建，宿主观测为模拟，非公网 RTT 或 P95。
- 登录来源 ABBA 微基准保持生产 Argon2id 参数，成功登录落盘次数 2→1；仅用于剥离存储成本的快速 hasher 不用于产品。未将密码计算测量波动算作确定提速。
- 终端来源固定相同夹具/受控延迟：50/150ms RTT 首连 p50 259.489/758.832→206.693/609.415ms；持续回显未证明改善。模拟后端每读注入 25ms 时主机公共输出段 p50 55.253→25.514ms；含批量调度/执行/清理的 8 目标测量 81.955→81.316ms，未证明批量加速。
- 以上数据只引用来源原件与测量 SHA，不冒充最终组合重新测量或真实公网/低配主机结论。模拟 Agent/PTY 的忙循环边界实验不等于常态 CPU 使用率；真实浏览器端到端终端、公网、丢包矩阵、混合旧节点和长期 soak 未验。

## 来源审阅、源码身份与组合审阅

- 两组来源完整 Linux L2 成功；终端 l2-exit.json 精确绑定 1b2bc255，登录 handoff-final.json 绑定 eed74c71。原始性能、故障、复验与安全报告留在各自仓外证据中，不抹去来源的环境或夹具失败。
- security-review-scope 相对 RC18 为 recorded 11 组：auth、store、Panel server/cluster/terminal/terminal_stream 及 5 个 Cluster 文件。代码未在组合中冲突修改，复用精确来源盲审：登录 H0/M1/L0、confirmed=0（线索追溯后排除），终端最终报告无可信新发现；不重新编排 CF、不作全仓安全保证。
- 来源与发布上下文分离，其他提供商 CLI 未找到，Independent-Review 使用 provider-unavailable fallback，不冒充跨提供商或明确模型身份。
- 自由臂先落盘，再 OCR 1.12.11 规则/逐文件检查；30/30 可审文件覆盖，4 个 unsupported_ext 文档和受跟踪 lock 排除，另读发布/协议文档。组合生产数据流独立复核，未变化的测试/基准复用精确来源独立证据，无新增 confirmed 发现。blind=false、constrained-only=unreported，不宣称 OCR 增量效果。
- 初次 2d8950ae 与最终 afb41280 仅增加 trailer，Git tree 相同；没有改写已推送/预览绑定的 SHA。
- canonical firstPartySources 998 文件重算为 `460bbf24e1a57e8a371fab5af5ee2c900b5c55e7578ae67e57411b3d884d7fec`；新增 Go 路径均为测试/基准，运行时为既有文件。六平台 closure 继续 `7344df4f0fe019b97ab22062bbbec1b93c1b935943480e334e2c879270221db7`，模块图/生产 GoFiles/构建目标不变，finding dispositions 和 2026-10-25 expiry 保留，精确 L3/扫描已通过，见本轮 receipt。
- 受信辅助文件按 dispatch 同一 ls-tree 范围核对，与 origin/main 完全相同（仅允许源码扫描身份 JSON 例外），避免重复 RC18 的候选独有 mock 夹具拦截。

## 跨仓库联动

- scriptLinkageState=not-required，**无需发布脚本（不适用）**：不增加脚本运行时动作、安装/卸载、宿主外联配置或应用市场默认入口，终端原生协议兼容扩展不要求新脚本。
- 实际内置脚本 commit `160924a498d9b8d071fed72f165d23c1b9e886de`，SHA256 `39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1`；完整门禁继续核验。

## 本地预览与正式发布

- acceptance/interaction 模拟预览 clean、fingerprint，绑定 afb41280，URL http://127.0.0.1:4174/overview。打开后显示控制台；切换经典模式，刷新仍恢复经典模式；终端入口显示模拟已连接，console errors=[]。未输入或执行真实主机命令，未调整全局浏览器视口。
- mock 使用旧认证响应，仅证明旧协议回退和 UI 初始化兼容，不证明新组合 Session 实现安全或速度；新接口依据来源真实 API/生产前端对照与正式组合门禁。终端后端不修改界面，本地页面不证明实际传输延迟。截图/JSON/manifest 在证据根目录，本任务 mock API/Web 已停止。
- 精确候选 push CI [39457695955](https://github.com/kejilion/KPanel/actions/runs/39457695955) 与 dependency freshness 39457696305 已成功。正式根 [39459947645](https://github.com/kejilion/KPanel/actions/runs/39459947645) attempt 1：inputs、candidate-ci、完整 L3、promote、main-ci、publish-release 成功；public-acceptance 失败，因此原根 conclusion=failure，preview-completion 未成功。
- L3 receipt 精确绑定候选/tree，status=passed；github-hosted ubuntu24 `20261018.339.1`，Go 1.27.2、Node 24.21.0、Docker 28.0.4。Web 312 测试文件，3065 passed/6 skipped；Web/Go/deploy 三 lane 均 passed，下载日志 SHA256 与 result.json 全部匹配，run-release-gate.sh 入口摘要匹配。L3 ZIP 亦匹配 GitHub artifact digest。
- 远端 main 与 RC19 tag peel 均 afb41280，annotated tag object `c8b6ba1bdac28af74b389b7c8cc3be81a995bf10`。

- [v1.27.0-rc.19](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.19) 于 2026-10-21T01:27:16+08:00 公开，isDraft=false、isPrerelease=true；15 assets、11 checksums、3 条主要变化和 3 条升级注意事项均通过公开回读。
- OCI index `sha256:840c6435ef700c719918119697d60b8c5cbbe8f18b24b32f04b6288d81f214d5`；amd64 `sha256:a4621e2a1fc49ac55284b7584f141c67040ab83cd47ab49940b863298d763f95`；arm64 `sha256:be9f00e8c52aa8e4d0a1489487c7bcfd3587159203d75d36b304d4df510de129`。双平台 revision/version/script labels 与契约一致，独立 Docker Hub 回读确认 version/preview 指向同一 index。
- 首次公开验收真实初始化 44 项、重启 26 项通过；浏览器完成 8 项后等待 `.terminal-tab__close` 30 秒超时，升级浏览器未执行。真实 retained PTY 已创建并回显标记，errors=[]，无失败画面截图；不能据此定位原因或判定为偶发。原 artifact 12286120026/ZIP SHA256 `a33f8884e956d6c0b0977fd363dd8ef83614b198b97d0e99b65e74b06e4c9315` 完整保留。
- [只读恢复验收 39466828645](https://github.com/kejilion/KPanel/actions/runs/39466828645) attempt 1 于 2026-10-21T01:33:14+08:00 success。输入精确 candidate/version/digest 与原发布前 stable-before snapshot（SHA256 `918166d7a219d8ef5d49231ae0642abbc5ec0b2a15e027f96d12930e66a2b8fc`）；不重发、不覆盖产品、不放宽断言。初始化/重启/真实终端浏览器/升级浏览器分别 44/26/45/31 项，共 146 项通过，均 mocked=false、errors=[]。终端刷新恢复和关闭、历史、升级说明及退出预览均在隔离容器验证；升级故障用例的 systemd 未运行返回为预期，不等于真实宿主升级。
- 恢复测试 controller=afb41280，runner Id `sha256:77bd63c4c746e9a0463f785910b88af15858d48a2447290b0b287dbd133dbdad`。recipe/original SHA256 同为 `8e07c6dda1815cf80973a4311cdb60eeb037bb7d6ebbc03c8da499801c93e77f`，diff 为空，productImageUnchanged=true。恢复 artifact 12284314187，ZIP SHA256 `d567c48881de9a8796e489d428d44215913ca484193bae9e71d34d1f42845eaf` 与 GitHub artifact digest 独立核对，53 个文件/10 张 PNG；已查看 320px 升级弹窗及重启桌面截图。通过复验不证明原超时根因已消除。
- 发布执行只用 github-hosted，无自有服务器/WSL 发布；未连接 prod-108 或任何生产主机，生产部署/备份不适用。

## 多维质量状态

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性/双端互通 | 已实现未实机验证 | 来源 L2、组合审阅与模拟初始化；隔离真实 PTY/浏览器恢复验收通过；真实混合旧节点未验 |
| 网络入侵/供应链 | 已验证 | 来源盲审 recorded、身份重新资格化；精确源码/镜像门禁通过，继承缺口保留 |
| 稳定性/恢复/兼容 | 已实现未实机验证 | 来源恢复/取消/半断连/账号隔离回归；真实公网故障及宿主生命周期未验 |
| 性能/资源预算 | 已实现未实机验证 | 有明确受控前后数据与字节/读取/并发限额；非真实低配或长时结论 |
| 用户体验/可访问性 | 已实现未实机验证 | 旧认证回退、刷新偏好和终端入口组合通过；原生缩放/读屏、实体手机未新验 |
| 数据/配置/迁移 | 已验证 | 无格式迁移，原子落盘失败整体回滚/原有兼容回归；实际生产回滚未执行 |

## 分支、磁盘与回滚

唯一 release/v1.27.0-candidate 按 RC 序列保留；两组来源分支/worktree 与归档证据保留作者所有权，未知/dirty 内容不清理。来源此前自动审批拒绝删除的 bare 恢复副本继续保留，本轮不重试清理。新建前 C 盘空闲约 67.3 GB；只停止本任务创建的模拟服务，不统计释放字节。验收记录在独立 docs/release-v127-rc19-acceptance，文档不改变已冻结产品字节，不创建 PR。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-20T22:28:49+08:00
- 候选冻结时间：2026-10-21T00:27:33+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：是（原公开验收失败后只读恢复；无回滚、产品热修复或重复发布）
- 若发生失败，发现时间、恢复时间和逃逸门禁：发现时间：2026-10-21T01:29:35+08:00；恢复时间：2026-10-21T01:33:14+08:00；逃逸门禁：未逃逸：原公开浏览器验收拦截终端页签超时，同镜像只读复验通过，未操作生产
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "public-acceptance/public-browser-complete/unresolved-terminal-tab-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "原根 39459947645 的终端页签恢复等待超时，根运行 failure；已公开 RC19 未重发，无生产写操作。",
    "recoveryEvidence": "只读 39466828645 使用原 snapshot 与相同候选/digest，146 项通过；原失败 artifact 和日志保留。",
    "permanentAction": "本轮未修改产品或断言，未确认根因或完成永久修复；保留首次失败和恢复双份证据，今后同处复现须先诊断，不能以覆盖发布抹去失败。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留边界

继承 RC18 的安全审计覆盖缺口、HTTP API/secure-cookie 配置边界及维护期限。无新生产、真实跨地域/旧节点、低配长时、原生缩放/读屏或完整宿主升级/回滚证据；不将受控性能、公开产物或通用门禁扩大为这些旅程全部通过。
