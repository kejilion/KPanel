# KPanel v1.27.0-rc.17 发布验收记录

日期：2026-10-20。发布级别：L3。RC17 已于北京时间 18:15:59（2026-10-20T10:15:59Z）公开，正式发布及公开验收均完成。

候选提交 / 标签：`4fa6dd81a9f400d3d335fa439829303029e69e87` / `v1.27.0-rc.17`。
候选 tree：`27c8e1c3895126506e88cbf8e4977f44fa80e61c`。
批准基线与上一预览：RC16 `06c5ba2eaf328256073a26bb8347c54a472d40ea`。
`releaseChannel=preview`，`releaseTrain=1.27.0`。
上一预览回滚镜像：`sha256:1e55461560b5054302c62aa99c1427038f51a5d8efb860057b5724df46b2a128`。
上一稳定版与默认通道：`v1.26.0`，`sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`。

## 发布画像与范围

- 业务域：Windows XP / Windows 98 桌面外观、移动终端输入和剪贴板手势、历史命令提示。
- 变更面：Web 展示、触摸交互、浏览器本地偏好；产品版本元数据及其供应链身份。
- 用户可见更新：Win 主题菜单纯色高亮、浅黄色内容底色、紧凑顶栏、滚动条与分组一致性；切回原生外观恢复原生壁纸；手机终端轻点输入、长按文字自动复制、长按空白自动粘贴；历史命令提示可关闭并在当前浏览器记住。
- 风险等级 L3：实际版本/镜像发布完整执行源码、双架构、供应链、镜像运行时和公开浏览器门禁。
- 无数据库、Panel/Agent 协议、端口、Compose、Agent 权限或安装脚本契约变化。剪贴板操作继续受浏览器权限与现有可输入状态限制，拒绝时保留菜单/键盘回退。
- 本次只组合三组 RC16 后的独立候选；不搭车发布较早的未准入功能，不重复纳入 RC16 内容。

| 来源分支 | 冻结 tip | 与基线关系及内容 |
| --- | --- | --- |
| candidate/win-theme-unified | 59bce6c2bec47313939dbfe04b7f9cefcf563580 | 以精确 RC16 为基线，11 个主题微调提交 |
| candidate/mobile-terminal-gestures | 3d935d48ffa1e9ce80580dc803a28fee8157ee8a | 以精确 RC16 为基线，单个聚焦交互提交；旧 feature/mobile-terminal-selection 为来源回溯，不重复合入 |
| fix/terminal-history-notice-dismiss | aa40754a29eea78fbb1e73edff5b501a14a4cabb | 以精确 RC16 为基线，提示关闭与浏览器记忆 |

三 tip 均为最终候选祖先。三组文件不重叠，octopus merge 无冲突，保留原提交历史；合并与版本准备之后相对 RC16 共 15 个提交。版本准备提交仅调整 VERSION、Go/Web 版本、CHANGELOG 与第一方源码扫描身份。

## 跨仓库联动与依赖

- `scriptLinkageState=not-required`：无需发布脚本（不适用）。实际内置脚本仍为 `160924a498d9b8d071fed72f165d23c1b9e886de`，SHA-256 `39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1`。
- 不改变脚本、应用市场默认更新入口、第三方依赖、基础镜像、Actions 或工具链；跨仓库变更集/脚本候选不适用。
- Dependency freshness run `39400241885` 对精确候选成功，不等于所有上游都已采用最新版。既有工具链复核与供应链观察期限保持不变，不增加豁免。
- 相对 RC16，唯一 Go 源码变化为版本字符串；无新 production imports、GoFiles、模块图或构建目标。canonical firstPartySources 对 984 个文件重算为 `fb8ca680ed41412b395cb1e31f35735224c456b52387339af651867ac5b702ea`，closure/findings/expiry 不变，完整 L3 必须重新验证。

## 审阅与自动门禁

- 接手发布者与三组原实现上下文分离；本机未找到可用的其他提供商 CLI，独立复核 trailer 使用 codex fallback=provider-unavailable。来源任务均已停止/完成，未召回或另建监工。
- 自由臂先落盘，再执行 OCR preview/rule/rules-check。17/17 可审文件 reviewed，CHANGELOG 为 unsupported_ext、package-lock 为受跟踪排除规则，另外核对版本内容。free-form 与约束臂均无 confirmed 发现；blind=false、constrained-only=unreported，不宣称工具增量效果。
- OCR 覆盖原 head `df78f924` 与最终提交 `4fa6dd81` 的 Git tree 完全相同，只追加评审 trailer，源码未变化。
- 安全盲审相对 RC16 为 not-applicable、0 安全敏感包组，未运行 CF。来源中 invalid trailer 仍保留为格式/继承缺口，不将本轮前端审查冒充稳定周期完整安全覆盖。
- 精确 push CI：[39400241922](https://github.com/kejilion/KPanel/actions/runs/39400241922)，success，变更验证、核心 race、Go 调用路径、Node 审计、源码与应用生命周期通过。
- 正式根 run：[39401702335](https://github.com/kejilion/KPanel/actions/runs/39401702335)，attempt 1，conclusion=success；inputs、candidate-ci、完整 L3、promote、main CI、Release、公开产物/浏览器与 preview-completion 均成功。publish/l3 按既有编排跳过，权威 L3 已在前置任务完成，未省略质量门禁。

- L3 receipt 精确绑定候选/tree，github-hosted ubuntu24 `20261018.339.1`，Go 1.27.2、Node v24.21.0、Docker 28.0.4。Web 312 个测试文件通过，3060 passed / 6 skipped；Web/Go/deploy 三 lane 均 passed，下载日志 SHA256 全部匹配 result.json，入口 `scripts/run-release-gate.sh` SHA256 与 receipt 匹配。
- 远端 main 与 RC17 tag peel 均为 `4fa6dd81`；annotated tag object `e773916acf63d22399b33a56bc8cedff26a78b44`。

## 本地功能预览

- 模拟数据 acceptance、visual-composition，绑定精确 `4fa6dd81`、clean；manifest/日志在 `C:\GitHub\_release-evidence\v1.27.0-rc.17\local-preview`，URL `http://127.0.0.1:4178`。
- 独立浏览器验收：1280×800 下 XP 对话框背景 rgb(236,233,216)、顶栏 34px；Win98 顶栏 28px，均无根页面横向溢出；返回原生时 style 属性移除、壁纸选中“经典流光”。
- 历史提示初始有关闭按钮；关闭后提示元素为 0、历史条目仍为 5；刷新并重新打开仍为 0 提示/5 条目。搜索空结果为 0 条，清空搜索恢复 5 条，浏览器 error 日志为空。
- 390×844 下终端辅助按键和历史区域可见，无根页面横向溢出；截图与 `local-browser-validation.json` 保存于证据根目录。临时视口覆盖已 reset。
- 手机手势源码/回归覆盖轻点、文字/空白长按、滚动、多指、取消、只读状态与异步剪贴板权限的关闭/替换/卸载边界。实体手机、原生 125%/200% 缩放、读屏和实际宿主机不由模拟预览证明。

## 多维质量与证据边界

| 维度 | 状态 | 已有事实与限制 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确 push CI、正式 L3 与公开 146 项检查通过；无新脚本协议 |
| 网络入侵与供应链安全 | 已验证 | 精确 push CI 门禁与源码身份复核；既有 finding、期限和审计缺口保留，不保证零风险 |
| 稳定性、恢复与兼容 | 已验证 | 来源回归、组合 UI 预览与 push CI；真实 systemd/OpenRC、混合旧节点未新验 |
| 性能与资源预算 | 已实现未实机验证 | 无新增常驻服务/依赖；触摸和 CSS 变化尚无实体手机长时负载结论 |
| 用户体验与可访问性 | 已实现未实机验证 | 桌面主题/壁纸、历史提示与窄屏组合已检查；实体触摸、读屏与原生缩放未验证 |
| 数据、配置与迁移 | 不适用 | 仅当前浏览器记忆提示选择，无后端持久化格式迁移，无生产数据写入 |

## 发布产物与公开验收

- [公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.17)：isDraft=false、isPrerelease=true，15 附件、11 个 SHA256SUMS 条目、3 项更新摘要、3 项升级注意事项。发布 provenance 精确绑定 `4fa6dd81` 与 RC17。
- OCI index：`sha256:5b67ae24080335684b707e4ec580104f9995c37166b679c75faa710bddaf76d0`；版本 tag 与 preview 相同。amd64 `sha256:7f82d5d1be0fd76e5e39affb81b542c4cb2dc79868560588db10e275e3d7d738`，arm64 `sha256:4cb75e475b84de7bf763117d6dd37e8adadd13198cff68761f75c8951053fdb6`，两架构 config 标签均为候选/RC17，内置脚本身份未变。
- `public-release.json` passed；initial 44、restart 26、browser-published 45、browser-upgrade 31，共 146 项检查，4 报告均 passed、mocked=false、errors=[]。存在 Docker inventory/测试 CA 等受控 fixture，不据此声称覆盖所有真实宿主环境。
- 53 个公开验收文件、10 张 PNG 已下载，保留本地 SHA256 清单；不声称该清单与远端逐文件 manifest 比对。另已独立查看 320px 升级弹窗和重启恢复桌面截图：摘要可见、确认操作存在、桌面恢复且显示 RC17。
- 公开浏览器升级路径从 RC16 检查到 RC17，展示三项摘要及三项升级提示；安装失败用例实际返回 503 / systemd is not running，随后可退出 preview 回到 stable。这验证错误反馈和通道状态，不证明真实宿主升级成功。
- 公开测试 runner 实际镜像 ID `sha256:a57fe851ead8642985a8b3c326dab6cd6fd24249930bb9a53575c56268766095`；recipe SHA256 `8e07c6dda1815cf80973a4311cdb60eeb037bb7d6ebbc03c8da499801c93e77f`，原配方/执行配方相同、diff 为空。controller 为 RC16，产品 candidate 为 RC17，不混同两者身份。
- 受信执行环境为 github-hosted；不使用自有服务器或 WSL 发布。
- 发布前 stable-before.json SHA256 `88ffc7e9a745731a0983b81196627cef504178230423058817358f68cf660135` 与 publication-identity 匹配，run/attempt/candidate/version 已独立核对。发布后 GitHub Latest 与 Docker latest 仍为 v1.26.0 及原稳定摘要；独立 Docker Hub 回读保存于 registry-independent.json。
- 生产部署、生产备份与真实宿主升级/回滚不适用（预览版禁止生产部署）。本次未连接 prod-108 或其他生产主机。
- 回滚点为 RC16 immutable digest；退出 preview 不自动降级。本轮不执行真实回滚。

## 分支处置与本地资源

- `release/v1.27.0-candidate` 按 RC 序列保留，不归档或删除；来源 tip 与 Git 恢复位置保留。
- 三组原作者分支/worktree 仍保留所有权，没有收到本轮删除/释放授权，不擅自清理。其精确已纳入内容不再算未发布新候选。
- 较早后台候选、历史验收及 archive 不混入本版；未知/dirty 内容均保留，没有批量清理。
- 验收记录在独立 `docs/release-v127-rc17-acceptance` worktree `C:\GitHub\_release-docs\kpanel-v127-rc17`；文档提交不是已冻结产品字节。
- 证据根目录 `C:\GitHub\_release-evidence\v1.27.0-rc.17`，含 intake、任务资格、源码身份、OCR、预览与 CI 原件；本任务临时预览的 mock API/Web 服务均已停止，manifest state=stopped。
- 本次未统计释放字节。只回收本任务拥有且具有恢复点的临时服务，其他资源保留。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-20T14:32:23+08:00
- 候选冻结时间：2026-10-20T17:22:04+08:00
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
    "fingerprint": "public-verification/docker-hub-read/cause-unconfirmed",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次独立读取 preview 摘要未匹配预期，阻断本地验收断言；首次实际 digest 未保留，不能证明缓存或传播是根因。",
    "recoveryEvidence": "带新查询参数的只读请求连续两次匹配版本与 preview 摘要，latest 仍为稳定摘要；registry-independent.json 保存成功响应。发布 attempt 仍为 1，未重新发布或覆盖镜像。",
    "permanentAction": "本次保留拦截事实、成功响应和未确认原因；后续回读应先保存原始响应再断言，避免失败时丢失实际摘要。未据此变更产品发布脚本。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 正式发布根 attempt 1 全部成功；独立 Docker Hub 首次 preview 摘要不一致的拦截与恢复单独保留，原因未确认，未重跑发布或覆盖版本镜像。
- 继承 RC16 的安全审计覆盖缺口、HTTP API / secure-cookie 配置边界及维护期限，不宣称本轮修复。
- 实体手机触摸/剪贴板授权、原生缩放/读屏、长时资源趋势、真实 NAS/混合旧节点与宿主 systemd/OpenRC 更新未新验。
- 稳定版仍须重新冻结精确候选，执行完整 L3 与公开产物/受影响用户旅程验收。
