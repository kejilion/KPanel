# KPanel v1.27.0-rc.20 发布验收记录

日期：2026-10-21。RC20 已公开，正式根 39521599271 attempt 1 的所有必需阶段成功，公开验收通过，生产未部署。首轮候选 CI 的依赖安全拦截和两项本地夹具异常均保留。

- releaseChannel=preview，releaseTrain=1.27.0，L3。
- 批准基线/上一预览 RC19 `afb41280b59ad326f84665aa74cc380be88f2db8`；最终候选 `11a9ca456a64a9439e06b07d3f6358a171c9257f`（首轮 9e0c1bc6 因 npm 安全通告拦截）。
- RC19 回滚镜像 `sha256:840c6435ef700c719918119697d60b8c5cbbe8f18b24b32f04b6288d81f214d5`；未执行回滚。超过 10 轮的新索引不能直接交给 RC19 读取，回滚须先备份并恢复对应版本的数据备份，已执行主机动作不撤销。terminal-batches 独立目录不在标准 Panel 备份中，升级前须额外冷备份该目录；缺少配套旧索引备份时不得把直接降级读失败当成功回滚。
- 独立回读确认稳定 Latest/latest 仍为 v1.26.0 / `sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`，未推进公共默认通道。

## 本地候选、精确范围与授权

| 来源 | tip | 本轮新增 |
| --- | --- | --- |
| feature/terminal-batch-background | 235e1905e5a07ce985e32ce4ef9fe6601c0a0a77 | 3 个 UI/测试提交：恢复原批量界面，去除新增说明/记录选择/删除控件，继续恢复最近任务，使用原终止按钮重试释放 |
| fix/terminal-batch-retention | 069ebb3da609bb76fe21aed65a00724603f287bc | 依赖上项，1 个提交：记录结束后保留 24 小时，取消 10 轮截断，保留完整主机与顺序 |

两项原件 clean、来源交付完成。后台调度主体 89c59877 已在 RC18/RC19；不重复声称本轮新增。源码差集相对 RC19 原为 6 文件，merge 无冲突。发布者专属修复 `bde1036c3f5c7e0753a9b258c4977eaaac61d3f4` 补齐取消轮数后输出内存预算：只保留最近 10 轮输出，全部元数据仍保留。再加版本/文档/源码身份后再追加安全依赖补丁后共 8 提交、14 文件。

用户明确授权重新整理本地新候选并发布新预览，包含唯一候选 push/main/tag/Release/镜像；稳定提升、生产部署与其他待办不在范围。主管理树 clean 但落后，保持只读；唯一列车 release/v1.27.0-candidate。

## 业务、资源、数据及失败边界

- 每轮所有目标、逐主机状态和原提交顺序保留，整轮结束后的更新时间起 24 小时；运行或释放未确认的任务不按过期清理。
- 元数据仍为 terminal-batches/jobs.json，schema 1、2 MiB 文件准入上限、原子写入、0700/0600 权限。达到字节上限拒绝新执行，不挤掉未过期记录。取消轮数不等于无限存储。
- 最多 64 台主机、4 worker、一个活动批量、整轮含排队 4 小时上限均不变。原有身份、scope、输入/输出限长、未知创建/输入不重放和关闭确认配额继续有效。
- 最近 10 轮每台最后 16 KiB 内存输出，最多约 10 MiB；仅在成功持久化准入后释放更早输出。旧记录及主机顺序不删除，OutputAvailable=false；失败准入不释放旧输出。输出不落盘，重启标记中断且不自动重放。
- 原数据格式无需迁移，但旧 RC19 读取器拒绝 >10 轮，已在 Changelog 和存储文档明确回滚备份要求。预览退出不自动降级。
- 新 UI 不引入 host/API/任意 Shell 能力；没有新增数据库、直接依赖、常驻服务或端口。API 删除能力保留，本轮 UI 按原交付移除记录切换和删除控件，默认只恢复最近一轮。
- scriptLinkageState=not-required，**无需发布脚本（不适用）**。继续复用现有认证终端/脚本契约，实际脚本 commit `160924a498d9b8d071fed72f165d23c1b9e886de`，SHA256 `39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1`。无新脚本宿主配置或应用市场默认入口，不发布脚本。

## 审阅、验证与源码身份

- 来源后台主体原 Linux L2 绑定 89c59877 的相同 tree，但该主体早已纳入；UI 后续 235e1905 的 verify-change：312 文件/3059 tests、typecheck/build/i18n 通过，精确绑定原件保存。retention 069ebb3d 的 Linux verify-change 和 terminalbatch race 通过；Windows 无 cgo 的原失败未改记成功。
- 发布者从差异及 runner 完成、清理、持久化与 UI 回读上下文独立核对，发现输出预算随取消计数被放大（MEDIUM），在专属 fix/v127-rc20-output-budget 修复。12 轮 × 14 个非字母排序主机测试同时断言最近 10 轮输出保留、更早释放、全部状态及顺序跨重启保持、零重放。补丁本地 Go 测试通过；最终 11a9ca45 在登记 local-wsl-dr 的独立 Ubuntu clone（Go 1.27.2）补做 `go test -race ./internal/terminalbatch -count=1` 通过，结束 clean。常规 race 仅含 panel/auth/dockerx，故补此实际包，不重复完整门禁；这不是 WSL 发布。完整 L3 仍由 GitHub-hosted 执行。
- 其他提供商 CLI 未找到，独立来源审查记录 provider-unavailable；输出预算补丁由发布者自查/回归，不冒充另一审查者或跨提供商验证。
- security-review-scope 相对 RC19 为 not-applicable / groups=0；未触及注册的敏感包，无新 CF 或全仓安全保证。RC19 及历史安全覆盖缺口继续保留，不篡改历史 verdict。
- 自由臂先于 OCR 圈选落盘，发现的输出预算已修复；OCR 1.12.11 逐文件 10/10 reviewed，4 个 excluded 为文档 unsupported_ext/受跟踪 lock，文档与 lock 另核对；约束臂无新增 confirmed，constrained-only=0。原 d78b458f 与首轮 9e0c1bc6 仅 trailer 不同，tree 相同。最终 11a9ca45 仅追加两项依赖清单/lock 与发布说明；第一方代码字节未变，原代码双臂审查按精确字节复用，新增清单/lock 已逐项审阅，manifest-only 补丁明确 OCR skipped，不冒充新一轮代码盲测。
- canonical firstPartySources 999 文件 SHA256 `4af28ee6821ff7892c6bfe825a490654853e2d717392b2af0375e6558e2b1173`。新增文件仅测试，无新运行时 Go 路径/import/module/build target；closure 继续 `7344df4f0fe019b97ab22062bbbec1b93c1b935943480e334e2c879270221db7`，finding dispositions、graph 与 2026-10-25 expiry 不变。精确 L3 已通过，身份与扫描结果保留原件。
- 受信辅助按 dispatch 相同 ls-tree 范围核对，与 RC19 main 相同，仅允许 scan-scope JSON 差异；未把外部模拟夹具带入发布。

## 必需安全依赖修复

CI 审计拦截后在独立 fix/v127-rc20-security-deps 形成 11a9ca45：markdown-it 15.0.2→15.0.3、Vite 8.3.2→8.3.4，锁定兼容子依赖 argparse 3.0.3、entities 8.1.0、postcss 8.5.31、nanoid 3.3.20；官方 registry tarball/integrity，未添加直接包或脚本。markdown-it 用于 AI Markdown，html:false 和 DOMPurify 不变；默认引用解析拒绝服务通告影响该版本，升级修复。Vite 用于开发/静态构建，生产由 Go 服务静态文件，不能据开发工具通告声称生产暴露编辑器执行接口。采用理由为安全维护，无提速结论。

[markdown-it 通告](https://github.com/advisories/GHSA-gfr2-rv44-r23c)、[Vite/launch-editor 通告](https://github.com/advisories/GHSA-r825-w8mm-5pxw) 与官方发布核对。npm ci 通过，新图 npm audit 0；AiMarkdown 与 BatchTerminalPanel 13 项定向测试、生产 build/typecheck/i18n 通过。Go 字节和 closure 不变；原完整 CI 证据不能替代最终精确提交 CI/L3。此安全载荷被正常门禁拦截，不伪记为环境重试或生产变更失败；流程异常仍只记两项预览夹具事件。

## L3 精确回执

正式根 39521599271 attempt 1 的 L3 receipt 绑定 11a9ca45/tree `7f145c90eaa2f13b83a687ff36758db55280ce6b`，status=passed、identityUnchanged=true；GitHub-hosted ubuntu24 `20261012.334.1`，Go 1.27.2、Node 24.21.0、Docker 28.0.4，2026-10-21T00:29:47.427Z→00:42:38.522Z。源码 Web/Go/deploy 三 lane 通过，分别 243817/496790/1886ms，jobs=2；Web 312 文件、3066 passed/6 skipped。三个原日志的本地摘要匹配 result.json，入口 run-release-gate.sh 摘要 `c286f7f2ad692ea17b8ec0537fe2a84e8d049d22adec63a2b00ceba39b165d57` 匹配。artifact 12312486692，ZIP SHA256 `67de18dc29a21ada69d449730047d630dc26595d7a61c06feabd622574ab9808` 匹配 GitHub metadata。L3 完成不代替后续主线 CI、公开产物与浏览器验收。

## 本地预览与分层证据

- 最终精确候选 11a9ca45（fresh npm ci）clean/fingerprint 的 acceptance/visual 页面在 http://127.0.0.1:4175/terminal。首轮 9e0c1bc6 预览的原生 mock 缺少 terminal-batches API，确实显示同步错误，未作为通过；随后外部夹具首次启动因相对模块导入失败，保留 stderr 与失败 manifest。
- 修复仓外夹具导入路径后，使用来源 235e1905 的 mock（只调整绝对 root/import），预检 GET 返回空列表，再由统一启动器连接回环 API。manifest mode=integration 仅表示传输方式，数据模式始终是**模拟**，不是实际 Panel 或宿主执行。
- 1280×720 检查原简洁布局、空状态禁用执行、选择模拟主机后执行按钮变终止、模拟后台完成、刷新重开恢复同一主机/最近结果、展开显示输出；无记录 select、无 document 横溢出、dev.errors=[]。最终截图 batch-final-restored.png 与 batch-final-alt-theme.png（首轮截图独立保留）。
- button 字号 14/13/16px，13px 为既有展开控件，本轮未缩小。当前浏览器未暴露 tab-local viewport/zoom 能力，窄屏、125%/200%、实体移动端、读屏未新验，不以桌面截图替代。1280×720 浅/深切换无横溢出；多语言完整矩阵未新验，源码翻译与 i18n 自动门禁另行覆盖。
- mock 不证明 24 小时留存、真正后台执行、Linux 文件权限或跨节点故障；这些分别由源码测试/L3/后续实机证据分层说明。没有新性能对照，不宣称提速或低配长时收益。

## 发布运行

- 首轮候选 CI 39519207153：failure，变更验证/核心 race/Go 漏洞调用路径通过，npm audit 报 markdown-it/Vite 两个高危依赖项；首次日志和终态保留。39519207160 freshness success 不等于安全审计通过。最终候选 CI 39520796240 / freshness 39520796183：success，精确 SHA 11a9ca45。
- 正式 [Publish preview 39521599271](https://github.com/kejilion/KPanel/actions/runs/39521599271) attempt 1 已派发，可信控制面为 RC19 afb41280，输入产品候选为 11a9ca45。inputs、candidate-ci、完整 L3、promote 已成功。远端 main/candidate/tag peel 均 11a9ca45，annotated tag object `fde05418a2bb4034be899127b29249990ad070c6`。main CI、publish/release、公开产物与浏览器验收、preview-completion 均成功。预览已有根 L3，publish/l3 为 stable 专属分支而正常 skipped，不冒充再次执行 L3。
- 仅 GitHub-hosted 发布。未连接 prod-108 或任何生产主机；生产部署、备份与部署后核对全部不适用（预览版禁止生产部署）。

## 公开产物与真实浏览器回执

- [v1.27.0-rc.20](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.20) 于 2026-10-21T09:03:56+08:00 公开，isDraft=false、isPrerelease=true；15 assets、11 checksums、3 条主要更新与 3 条升级提示通过公开回读，手机 320px 更新弹窗和重启桌面截图已查看。
- OCI index `sha256:866acb4fd993255d3ae53b7363b48ad7552259d69424e58e27e8dccf5249bf35`；amd64 `sha256:a394f368e4ebe1c5cd6a903107ddc83c7e56ecac2b02c6a5afd00cc8068151a6`；arm64 `sha256:972badec814e6f7333a3852e7c72741601ac4e88032e3bf7c7a74af7db44c74c`。双平台 version/revision/script labels 一致。独立 Docker Hub 回读确认版本/preview 同 digest，GitHub Latest 和 Docker latest 均保持 v1.26.0。
- 原 snapshot artifact 12311329568 独立摘要匹配，publication identity 的 candidate/version/controller/run/attempt 已核对；原 stable-before.json SHA256 `ad5193e32541d0bcc662f5736962da271f8915f24092f005a9110a796337abbc`，保存 RC19 previous-preview 与 v1.26.0 stable；未重采发布前状态。
- 公开验收 4 报告均 passed、mocked=false、errors=[]：初始化 44、重启 26、真实终端浏览器 45、升级浏览器 31，共 146 项，53 文件/10 PNG。包含真实 PTY 输入输出、刷新续接/历史/关闭、桌面和首选项恢复、证书故障、RC19→RC20 更新说明、预览退出及较低 stable 不降级。systemd 未运行返回是隔离更新故障用例的预期结果，不是宿主实际升级/回滚成功；无真实公网/混合旧节点或低配 soak 结论。
- artifact 12313405605 / ZIP SHA256 `149c3a754a1501a255c8014c1b79344acb8f5ad8fa3e64843c17ff542dbcc00d` 匹配 GitHub metadata。本地文件 manifest 单独保存，不冒充远端逐文件签名清单。
- 测试 runner Id `sha256:f00fb80b3ddc64cc3f631ff7eca92e4885b41b20517d0dfff53bbaf1a966245e`；可信 controller=afb41280，recipe/original SHA256 同为 `8e07c6dda1815cf80973a4311cdb60eeb037bb7d6ebbc03c8da499801c93e77f`、diff 空、productImageUnchanged=true。与冻结产品身份分开记录，不覆盖产品镜像或放宽断言；本轮不需要只读恢复重跑。

## 多维质量状态

| 维度 | 状态 | 证据及边界 |
| --- | --- | --- |
| 业务正确性/双端互通 | 已实现未实机验证 | 完整记录与顺序/恢复自动测试；真实混合旧节点未验 |
| 网络入侵/供应链 | 已验证 | 原权限契约不变，身份重新资格化；L3 精确扫描通过，历史缺口保留 |
| 稳定性/恢复/兼容 | 已实现未实机验证 | 过期保护、重启不重放、清理失败控制与旧索引回滚说明；公网/真实宿主生命周期未验 |
| 性能/资源 | 已实现未实机验证 | 2 MiB 元数据/约 10 MiB 输出限额；无真实低配长期资源曲线 |
| 体验/可访问性 | 已实现未实机验证 | 模拟刷新结果/空状态通过，窄屏与缩放/读屏未新验 |
| 数据/配置/迁移 | 已实现未实机验证 | 无格式迁移，保留字节准入/原子落盘；旧版 >10 轮读取限制明确，实际回滚未执行 |

## 分支、资源与回滚

保留唯一预览列车与来源工作树，来源分支没有推送；未知或活跃对象不清理。发布者修复分支、本轮文档分支及仓外首次失败/成功证据保留。来源 235e1905、069ebb3d 已纳入 RC20，不再计入待发布队列；原任务工作树/预览所有权未移交，原本地分支与工作树保留为本地待处置，不重命名或删除他人路径。责任为当前发布者，下次原任务明确释放本地资源时复核；无远端来源分支待删。发布者自有 `fix/v127-rc20-output-budget` bde1036c 和 `fix/v127-rc20-security-deps` 11a9ca45 已核对 clean、精确 tip 及纳入 RC20 的祖先关系，在本地分别改名为 `archive/fix/v127-rc20-output-budget`、`archive/fix/v127-rc20-security-deps`，归档 SHA 回读一致；原先未推送远端，故无远端移除动作。自有工作树与当前版本依赖保留供复核。新建前 C 盘约 66.2 GB 空闲，结束约 64.5 GB；仅清理本任务拥有的预览服务（本任务所有已启动的预览与外部 mock 均已停止），未删除其他任务内容，不虚报释放字节。验收独立 docs/release-v127-rc20-acceptance，不改变已冻结产品字节、不创建 PR。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-21T07:36:01+08:00
- 候选冻结时间：2026-10-21T08:20:23+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "local-preview/local-feature-preview/missing-batch-mock-api",
    "position": "before-production-write",
    "count": 1,
    "impact": "原生 mock 缺少批量记录接口，页面显示同步失败，不能作为成功验收。",
    "recoveryEvidence": "原 manifest 和日志保留；外部来源 mock 补齐接口后 GET 空列表预检和刷新恢复旅程通过。",
    "permanentAction": "本轮没有修改受信发布辅助。外部夹具与产品分层标注；后续模拟验收先核对受影响 API，不以 UI 启动代替功能完成。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-preview/external-mock/relative-module-resolution",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次仓外 mock 启动因相对 import 找不到模块，启动器拒绝不可达 API，未产生有效验收。",
    "recoveryEvidence": "原 stderr 保留；逐项确认模块存在后使用固定仓库绝对 import，健康预检通过并生成新 manifest。",
    "permanentAction": "只修复本任务仓外夹具 import/root；先验证必需模块和 API 再启动预览，不覆盖首轮失败，不降低正式发布断言。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->
