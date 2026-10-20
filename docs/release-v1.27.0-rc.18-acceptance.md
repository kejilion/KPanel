# KPanel v1.27.0-rc.18 发布验收记录

日期：2026-10-20。L3 预览发布，已于北京时间 22:09:40（2026-10-20T14:09:40Z）公开，修正后正式发布根与公开验收全部成功。

- releaseChannel=preview，releaseTrain=1.27.0。
- 最终候选 `c89ee44d3d8f7552e4aa732718f4a50aaeb26474`；初次候选 `ad4d103b42def89a08a4ddd7ba03acb3af5049fc`；批准基线/上一预览 RC17 `4fa6dd81a9f400d3d335fa439829303029e69e87`。
- 回滚镜像 `sha256:5b67ae24080335684b707e4ec580104f9995c37166b679c75faa710bddaf76d0`；退出 preview 不自动降级，真实回滚未执行。
- 发布后独立回读稳定版仍为 v1.26.0 / `sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`。

## 范围与候选盘点

| 本轮新候选 | 精确 tip | 内容 |
| --- | --- | --- |
| fix/desktop-move-resize-layout | 7333dbcaf672b211c5fe50e1f2464ac5e8428a17 | 2 提交；图标/小组件手动位置、窗口尺寸与全屏网格间距 |
| feature/terminal-batch-background | 89c5987720dae328c19fd1f70e3d0f47162e518f | 6 提交；后台排队、恢复选择/进度、容量/关闭确认及有界操作记录 |

两组均以精确 RC17 为基线，接手时 worktree clean、来源任务 completed/idle；未召回旧作者或另建监工。来源文件互不重叠，octopus merge 无冲突、保留历史。合并、版本准备与专属修复后相对基线共 11 提交。RC17 内容、较早待复核分支和历史等价补丁不重复纳入。发布范围排除来源的本地 mock 批量夹具支持，产品和测试不受影响。

## 产品性质、协议与风险

- 单管理员轻量 Linux 控制面、宿主事实、Panel/Agent 分权、管理员完整能力不变。后台批量仍通过既有认证 PTY 和跨节点 scope，不增加 Agent 任意 Shell API。
- 最多一个活动批量、64 台目标、4 个 worker，含排队最长 4 小时；共享服务端终端容量，只有明确 terminal_limit 才排队重试，未知创建/输入结果不重放。关闭失败保留身份与名额，显式停止后可重试释放。
- 关闭浏览器、刷新、离开窗口仅断开查看；重开恢复活动任务、目标选择与最近记录。Panel/宿主重启标记未完成操作中断，不自动重跑。
- `terminal-batches/jobs.json` 为新增独立 schema 1 索引，最多 10 任务、7 天、2 MiB、目录 0700/文件 0600，原子替换。只保存命令名/固定 k 动作摘要、目标和状态；命令参数与 PTY 输出不落此索引，原有共享命令历史仍按既有契约处理。输出单主机最后 16 KiB，仅内存。
- 损坏/未知索引保留原件并停用新批量，首次落盘失败不发送命令；运行中写失败显式提示恢复风险。旧版本忽略新增目录，回滚不会撤销已执行主机操作；没有数据库或 Agent 协议迁移。

## 跨仓库与供应链

- scriptLinkageState=not-required，**无需发布脚本（不适用）**：复用已有认证终端协议，无脚本、安装路径、宿主外联配置或应用市场默认入口变更。
- 实际内置脚本 commit `160924a498d9b8d071fed72f165d23c1b9e886de`，SHA256 `39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1`。
- 批量来源已用固定 Go 1.27.2 核对六平台 actual/original 生产闭包，989 个第一方源文件、110 个生产模块，无新增第三方依赖。closure `7344df4f0fe019b97ab22062bbbec1b93c1b935943480e334e2c879270221db7`。
- 组合相对批量来源的唯一 Go 变化为版本字符串；canonical firstPartySources 重算为 `3849b371e9f050ac0819fa729a856d334830977d147452b756c667c4f900e6db`。图、构建目标、finding dispositions 与 2026-10-25 到期日保留；新的精确 CI/L3 必须重新验证。

## 审阅与本地证据

- 发布上下文与原实现上下文分离。未找到 Claude/Gemini/Qwen/Copilot CLI，Independent-Review 使用 provider-unavailable fallback，不冒充跨提供商复核。
- 来源安全盲审初始 H0/M1/L0、confirmed=1/fixed=1/deferred=0，已修复批量摘要保留任意凭据参数；最终独立报告未发现可信新问题。来源独立审阅初始 prompt/长输入/UTF-8/关闭身份问题及修复均保留原件。精确后端字节未被组合改变，security-review-scope 对 4 个 Panel 文件均 recorded，继承复用，不重复启动 CF。
- 组合自由臂先落盘，再 OCR 1.12.11 圈选/规则/逐文件检查：24/24 可审文件 reviewed、4 个排除为 3 个 unsupported_ext 文档及受跟踪 lock 排除，无新增 confirmed 发现。blind=false，constrained-only=unreported，不宣称工具增量效果。原 e41a4c51 到初次 ad4d103b 只追加 trailer，tree 相同；c89ee44d 仅删除未受信 mock 夹具增量。最终 23 个可审文件均与原已评审文件完全相同，coverage-recovered.json 逐文件记录复用；37 行 mock 恢复单独核对与受信主线一致，不增加产品逻辑。
- 初次本地 acceptance / visual-composition 绑定 ad4d103b clean、fingerprint，证据在 `C:\GitHub\_release-evidence\v1.27.0-rc.18\local-preview`。修正后重新生成绑定 c89ee44d clean 的 `local-preview-recovered`；mock API/Web 均已停止。后者不支持新批量 mock 旅程，不把该局限当作真实服务端不可用；原作者的完整 mock 在其来源分支保留。
- 1280×720 → 1920×1080：22 图标坐标相同，小组件 y=16/216/516、无根页面横向溢出；键盘手动移动概览后，1280×800 → 1920×1080 均保留 translate3d(95px,0px,0px)。全屏截图保存，不等于原生 F11。
- 批量组合浏览器未完成：CDP focus/mouse 超时，重新打开标签可读但模式/任务栏操作没有确认结果，原因未确认；未把按钮尝试算成功。来源模拟预览和回归可复用其精确后端/批量前端范围，正式公开门禁仍需验证，不能证明真实跨节点旅程。
- 本地 OCR rules-check 两次参数误用、handoff 的 PowerShell 七位小数时间戳被拒绝后修正为 ISO 毫秒，未修改产品代码、未重复运行已成功长门禁。原始工具结果保留在任务记录。

## CI、L3 与公开产物

- 初次 push CI [39427234067](https://github.com/kejilion/KPanel/actions/runs/39427234067) 和 Dependency freshness 39427234083 成功，仅对 ad4d103b 生效。修正后精确 CI [39429191845](https://github.com/kejilion/KPanel/actions/runs/39429191845) 已成功。
- 首次正式发布 [39428886636](https://github.com/kejilion/KPanel/actions/runs/39428886636) attempt 1 在 inputs 失败：mock-app-market-api.mjs 候选独有更新触发 Release automation must be integrated on main before dispatch；未启动候选 CI/L3、未推进 main/tag/Release/镜像。保留原失败日志。
- 由本发布者在专属修复分支 fix/v127-rc18-preview-fixture 恢复唯一 mock 文件为 RC17 受信字节，形成 c89ee44d，再快进唯一列车；产品/测试/发布检查未改。按与入口相同的 ls-tree 范围核对 main 与候选受信辅助文件完全相同（仅允许扫描身份例外），新候选需重新同 SHA CI 后再 dispatch。
- 修正后正式根 [39430030435](https://github.com/kejilion/KPanel/actions/runs/39430030435) attempt 1 conclusion=success：inputs、candidate-ci、完整 L3、promote、main CI、Release、public-acceptance 与 preview-completion 全部成功。publish/l3 按既有编排跳过，权威 L3 已在前置完成。
- L3 receipt 精确 candidate c89ee44d / tree `813959b22a09545ca31b97af0fe69e26e0e7346f`，github-hosted ubuntu24 `20261012.334.1`、Go 1.27.2、Node 24.21.0、Docker 28.0.4，status=passed；Web 312 文件、3059 passed/6 skipped。Web/Go/deploy 三 lane 原始日志 SHA256 与 result.json 全部匹配，run-release-gate.sh 入口摘要与 receipt 匹配。
- 远端 main 与 RC18 tag peel 均 c89ee44d；annotated tag object `5da2f3e87144920cf7115ed509188edf687d02f0`。
- [公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.18) isDraft=false、isPrerelease=true，15 附件，正文为 3 项更新与 3 项升级注意事项。
- OCI index `sha256:5eb0cd7ff4a469a017f3570767c148339bee460c872d7dea7b26a5cc51abdca2`；版本 tag 与 preview 相同，独立回读均匹配。amd64 `sha256:377359b2586c2192db6f85d10db596b318c928863cc9537e73464fdd0500ef62`，arm64 `sha256:80af3df0de9b9cdbfcdb4cb6826cb67ce4d194bb53260a22bf3368239b7b4009`。
- 发布前 stable-before.json SHA256 `bd55cff7d21f85137f0b735000e45a7708648a4749b744ac73aa05fd8f52998a` 与 publication-identity.json 匹配，candidate/version/run/attempt 已核对；controller 为 RC17，产品为 RC18。发布后 GitHub Latest 与 Docker latest 保持 v1.26.0。独立原始注册表响应先保存后断言，位于 registry-raw-*.json / registry-independent.json。
- 公开产物 public-release.json passed，15 附件、11 项 SHA256SUMS、3 项更新与 3 项升级提示，两架构 config 标签均绑定精确候选和脚本基线。initial 44、restart 26、browser-published 45、browser-upgrade 31，共 146 项检查，4 报告均 passed、mocked=false、errors=[]；Docker inventory/测试 CA 等受控 fixture 明确保留，不能泛化为所有真实宿主环境。
- 53 个公开文件、10 张 PNG 已下载；ZIP SHA256 `fb8bc88fff4d9c5ad95ad629405bc615d43d3d6255bd2e593b31493580165e15` 匹配 GitHub artifact 元数据。另保存本地逐文件 SHA 清单，不声称存在远端逐文件 manifest 可比。独立查看 320px 更新弹窗和桌面恢复截图：RC17 → RC18、三项更新可见、确认操作存在，恢复桌面显示 RC18（服务卡片截图仍在同步，未声称该瞬间全部健康）。
- 公开升级浏览器实际由 RC17 后端检查到 RC18，显示三项升级提示；点击安装失败用例为 503 / systemd is not running，随后可退出 preview 回 stable。这验证错误反馈/通道状态，不证明真实 systemd 更新完成，也不覆盖新批量的真实跨节点关闭浏览器旅程。
- 实际测试 runner image ID `sha256:22b1d228c47bd0fb6ed666ddd7fb8cedc87237218fbee39c4ebfc1f831c059b3`；recipe SHA256 `8e07c6dda1815cf80973a4311cdb60eeb037bb7d6ebbc03c8da499801c93e77f`，原/执行配方相同、diff 为空。产品与控制面身份分开记录。
- 只使用 github-hosted。生产部署/生产备份不适用，本次未连接 prod-108 或任何生产主机。

## 多维质量状态

| 维度 | 状态 | 边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 来源回归、组合 CI/L3/公开门禁通过；新批量真跨节点断网、旧节点仍未验 |
| 网络入侵与供应链 | 已验证 | 来源独立审阅、包组 recorded、源身份和公开扫描通过；既有审计缺口保留 |
| 稳定性与恢复 | 已实现未实机验证 | 队列/容量/停止/损坏/重启不重放已有回归；真实系统生命周期未验 |
| 性能与资源 | 已实现未实机验证 | 明确并发/输出/索引/任务时长上限，无新增依赖；真实长时低配负载未验 |
| 用户体验与可访问性 | 已实现未实机验证 | 桌面尺寸与手动位置组合已查；批量组合浏览器、原生缩放/读屏未完成 |
| 数据配置与迁移 | 已验证 | 新增独立有界索引，旧版忽略；真实磁盘故障与回滚未执行 |

## 分支、资源与回滚

- `release/v1.27.0-candidate` 按 RC 序列保留；来源分支/worktree 仍保留作者所有权，未知/dirty 内容不清理。两组纳入后不再计作未发布新候选。
- 验收记录在 `docs/release-v127-rc18-acceptance` 独立 worktree；文档提交不改变冻结产品字节，不创建 PR。
- 新建前 C 盘空闲约 81.5 GB；仅停止本次拥有的组合 mock 服务，其他预览和唯一证据保留，没有统计释放字节。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-20T18:35:48+08:00
- 候选冻结时间：2026-10-20T21:17:05+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：6
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "artifact-read/gh-download/stalled-transfer",
    "position": "before-production-write",
    "count": 1,
    "impact": "正式根已成功后，公开证据包 gh run download 约 5 分钟无返回，暂不能完成独立原件核对，网络根因未确认。",
    "recoveryEvidence": "验证进程归属后只停止该下载进程，以 PowerShell 官方 API 一次 60 秒有界读取成功，2.8 MB ZIP 的 SHA256 匹配 GitHub artifact digest，再解压核对 146 项检查和 10 截图。未重跑发布或覆盖镜像。",
    "permanentAction": "将下载设为有界操作并在客户端切换后先核对 ZIP 摘要；保留首次未完成事实，不把传输问题写成产品或验收失败。",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication-input/github-preview-release/candidate-only-mock-helper",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次根 run 39428886636 attempt 1 在 inputs 拒绝候选独有 mock 辅助脚本，后续门禁及发布写入均未开始。",
    "recoveryEvidence": "保存 publication-attempt1-failed.log，当时主线仍 RC17、RC18 tag 不存在；专属修复仅恢复 mock 为主线字节，candidate-recovery.json 记录新候选 c89ee44d，随后新精确 CI 和正式根 39430030435 attempt 1 全部成功。",
    "permanentAction": "冻结时本地核对与 dispatch 相同的完整受信辅助树范围，禁止仅检查 workflow 文件名；本次已做精确一致性核对，不放宽输入门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-review/ocr-rules-check/invalid-arguments",
    "position": "before-production-write",
    "count": 2,
    "impact": "rules-check 首次携带不允许的 format 参数，第二次缺少文件路径，均未产生有效检查证据。",
    "recoveryEvidence": "随后以受支持的逐路径调用完成全部 24 文件，ocr-rule-checks.json 保存成功原始输出；不把两次错误调用算作通过。",
    "permanentAction": "本次按委托入口已返回的实际 usage 修正调用，复用一次成功结果，不改 OCR pin、圈选策略或产品代码。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-handoff/task-preflight/timestamp-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "PowerShell o 格式时间戳含七位小数，被 handoff 的 canonical timestamp 校验拒绝，推送尚未开始。",
    "recoveryEvidence": "仅将已有回执时间规范为 JavaScript ISO 毫秒，保留原日志与候选身份后 handoff passed，再执行推送，未重复长验证。",
    "permanentAction": "回执采用 canonical ISO 毫秒时间，保留拒绝原因，不修改或放宽 task-preflight 校验。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-browser/cua-cdp/cause-unconfirmed",
    "position": "before-production-write",
    "count": 1,
    "impact": "桌面尺寸和手动移动已验证后发生 focus/mouse 超时，批量组合浏览器未完成。未将无响应操作计为成功。",
    "recoveryEvidence": "新标签可读且 console errors=[]，模式切换仍无确认结果；保存 local-browser-validation.json，重置视口并停止本任务服务。正式公开 Actions 通用浏览器验收成功；不能补充证明未完成的新批量组合旅程。",
    "permanentAction": "保留原因未确认和未完成旅程；正式公开门禁不得跳过，本轮不据工具超时修改产品或降低断言。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留边界

继承 RC17 的安全审计覆盖缺口、HTTP API / secure-cookie 配置边界和维护期限。真实跨节点断网、旧节点兼容、宿主 systemd/OpenRC 生命周期、低配长时负载、原生 F11/125%/200% 缩放及读屏均未新验。公开成功不等于生产或这些实机旅程准入。
