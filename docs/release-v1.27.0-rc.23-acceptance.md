# KPanel v1.27.0-rc.23 发布验收记录

日期：2026-10-21。发布级别：L3。releaseChannel=preview，releaseTrain=1.27.0。

候选 accb4c1208ecb3f9a6e300dbb3fc0422fd7a1165，tree=611cd8ff70de884a1acd300cc4c7beff275fc21d，annotated tag 对象 0e70eb4f4845cd5984ff95f886d570bb1d378633。用户授权本轮重新盘点候选并发布新预览，包括候选 push、main、tag、Release 和镜像；不部署生产，不提升 stable。

## 范围与用户行为

- 图库 candidate/gallery-experience-optimization@a2b3daa020a88d40fdb6d66f5c3eee716731f64e：完成后立即显示，快滑继续推进可见图片，有界冷缩略图名额，近期图片回看保留。八个变更文件与最终候选 blob 完全一致。
- 批量终端 fix/terminal-batch-recovery@27dba52c6275dd1e16791caefcc102965145a3da：管理器自动重试释放临时连接，执行与清理分开，历史失败不阻塞当前新任务或停止；只显示执行/停止，清理不重放命令。
- OneDrive feature/onedrive-backup@ffcb5de8575fd56e6b7344d74440e9b0bfb5aad6：Microsoft Graph 设备码和应用目录，复用现有上传、导入及备份计划；用户自行配置应用编号。真实微软个人/工作学校账户、云端往返、配额及恢复尚未实测。
- RC22 systemd 和终端停止/滚动已包含，不重复作为新候选。mobile-terminal-selection 的整合候选已经是主线祖先，不重复纳入。没有加入其他新功能。
- 发布修复 0efa14b6 仅刷新规范要求的累计业务事实；不改规则、阈值、测试或产品代码。原 36d7ff01 失败 CI 保留，最终 accb4c12 重新冻结并预览。

## 兼容、资源与回滚

批量自动清理每轮默认 30 秒、单次关闭 10 秒，仍受全局 16/用户 4 会话名额约束；未确认关闭保留会话身份和占用，不代表远程进程已确认结束。24 小时记录、2 MiB 元数据及最近 10 轮内存输出规则保持；历史 legacy 关闭错误不生成 live cleanup。

图库来源冷图串行 200 ms 夹具在 mobile Chromium、mobile WebKit、desktop Chromium 的 maxOutstanding=4、decodedVisibleButHidden=0。停手后首张图片分别 1498/1524/866 ms；不外推为真实服务器性能、实体手机 FPS 或硬卸载后的全局网络上限。

OneDrive 固定 Graph/授权端点，公共 IP 拨号、禁止代理及重定向，有界 JSON/会话/列表和顺序 10 MiB 分块；凭据加密落盘，刷新轮换持久化保持冲突约束。无数据库迁移。降级至不支持 OneDrive 的旧版前先移除连接并调整关联计划，保留配套数据备份。

回退基线为 RC22 / f0c039d8d3604f4fbafd4745efbc5a3cf4a3df96 / sha256:0dfc446bca4d8e20057d031f927ea8edf54df67c9e68b3643973f2d4b3b3301f。人工使用配套镜像和数据备份，跨挂载生命周期回滚需同步服务注册；退出 preview 不会自动降级。未执行生产回滚。

## 跨仓库和供应链

scriptLinkageState=not-required，**无需发布脚本（不适用）**。固定脚本 160924a498d9b8d071fed72f165d23c1b9e886de，原始字节 SHA256=39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1 已回算；无新引擎动作、协议、Agent 归档或宿主路径契约，跨仓变更集与脚本候选不适用。

保持 Go1.27.3、模块版本及生产目标，不能复用 RC22 的第一方/闭包身份。本轮第一方 Go 文件 1007，source=55ba25e4fb3ac974187f2fdf6a12b29b794225ca4591db0c7d78a9645d68bd31；closure=f5b0d0988c5f7d37ed9cd9cee453c22586efa5d9d1bed42240599df5517eee07；actual/original 六平台闭包在固定 Go1.27.3 重新观察，并与 GitHub canonical 原始 inventory 匹配。finding/expiry 沿用既有处置，不因来源新 Go 文件省略核对。

## 审阅和本地验收

发布者独立核对三份来源完整差异、来源独立审查原件及组合 i18n，先自由臂后约束臂。OCR39/39、excluded4 另核；后续仅增加的业务事实文档单独审阅。发布者新约束臂发现 0；来源原始 unreported 不改成 0，不据单轮宣称工具效果。

敏感范围四组 source-only 轻量安全盲审已有 recorded 证据，非 CF scoped/full，也不是全仓安全结论。既有未改路径中风险线索继续由备份维护者在 2026-11-04 前处置，细节仅仓外；没有新增 confirmed 候选发现。来源 OneDrive 完整 L2 因宿主污染失败保留，未改成通过；最终独立 GitHub CI/L3 另行通过。

绑定 clean accb4c12 的 acceptance/mock-ui 复核：无效 OneDrive 应用编号显示可恢复错误并保留输入，MOCK-CODE 自动模拟授权保存，目录编辑为 rc23-review 后读回；未打开真实微软登录。历史终端失败允许新任务，刷新恢复运行状态，停止恢复执行表单；fixture calls 精确一 start/一 stop、同一 id，无重放。图库可打开已解码图片并退出；滚动压力复用生产 blob 相同的来源三浏览器原件。

本地截图一次超时、两次空白，原件不作为视觉通过；配置帮助展开、原生200%缩放、读屏、实体手机和真实图库/PTY/微软云端尚未验收。无关 mock Passkey/MCP 404 保留，不宣称全页 console 为零。公开真实浏览器和隔离容器结果在下节独立记录。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 CI/L3、核心 mock UI；真实微软云端与跨服务器 PTY 未验 |
| 网络入侵与供应链安全 | 已验证 | 固定基座、四组来源盲审、原始 inventory；无全仓安全或 CF 通过结论 |
| 稳定性、失败恢复与兼容 | 已验证 | 自动清理生命周期/并发测试与公开夹具；实体服务器重启和长期故障未验 |
| 性能与资源预算 | 已实现未实机验证 | 来源冷图名额与有界批量/传输；无真实硬件吞吐/FPS/soak |
| 用户体验与可访问性 | 已验证 | 核心 AX/DOM、来源浏览器、公开真实浏览器；本地视觉截图、原生缩放、读屏未验 |
| 数据、配置与迁移 | 已验证 | 加密连接、revision、兼容测试；无数据库迁移，降级需移除 OneDrive 并调整计划 |

## 完整发布与公开产物

[正式发布 root39569322955](https://github.com/kejilion/KPanel/actions/runs/39569322955)，controller=672e5b41405aa65003607ef85b8569ef041ecdbf。attempt1 的发布源码断言失败保留；attempt2 仅有限重试失败的发布阶段，Release 成功并公开，但公开 restart 浏览器出现动态模块 fetch 失败，原 root failure、preview-completion skipped，均不倒写全绿。候选 CI、完整 L3、main/tag、主线 CI 复用同一 SHA 的 attempt1 成功证据；stable 专属任务 skipped 不视为缺少预览 L3。MCP 测试的异步时序敏感风险仍保留待修，不以重试通过宣称永久解决。

随后通过 main 的官方只读 [Verify published preview 39577465506](https://github.com/kejilion/KPanel/actions/runs/39577465506)（attempt1，controller=accb4c1208ecb3f9a6e300dbb3fc0422fd7a1165）验证原 tag/SHA/digest 和原始发布前快照，终态 success；该恢复不调用发布、不修改镜像或tag，不把当前preview重新当成发布前快照。最终交付依据为原发布成功事实加本次独立公开验收成功。原动态模块加载失败原因未唯一定位，偶发风险保留待核查。

候选 push CI39568373827 success，helper 同 SHA CI39566760414、helper 主线 CI39568297391 success。最终 main/candidate/tag peel 都为 accb4c1208ecb3f9a6e300dbb3fc0422fd7a1165。L3 runner={"image":"ubuntu24","version":"20261018.339.1"}，tools={"go":"go version go1.27.3 linux/amd64","node":"v24.21.0","docker":"28.0.4"}，entry=a2406ede4695ad76b509bad1389e543b54e6d1e9b1af17f5f6111b6e582ef6f7；源码 web/go/deploy 三 lane exit0，日志哈希独立回算，identityUnchanged=true。前端计数原文：Test Files  314 passed (314)；Tests  3096 passed | 6 skipped (3102)。L3 artifact 12332507267 / sha256:4380a354c1f62ae9adf629d9cf4aebcea94cd7a9ddb75522a5a6ab1fa166caee。

[公开预览 v1.27.0-rc.23](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.23)，公开时间 2026-10-21T09:25:32Z，draft=false/prerelease=true。OCI 版本镜像与 preview=sha256:144cf771a1043dc584b342cd2cef15e3f0b9280a632706b0c1c04576a1ac0dd7；linux/amd64=sha256:6f80455e04a136b796f1c9719ae219ccd7db6b8dc41fecb6f7788d24db974193；linux/arm64=sha256:e0006a563486114db68e46071e89956b7411544a593ca447379b8ab7ab0f55d4。

公开 146 项检查，kpanel-browser-published-39577465506-1\results\report.json: 45；kpanel-browser-upgrade-39577465506-1\results\report.json: 31；kpanel-real-39577465506-1\results\initial\report.json: 44；kpanel-real-39577465506-1\results\restart\report.json: 26。四份真实 report passed/mock=false/errors=[]，共 53 文件/10 PNG；包括公开更新信息/弹窗、PTY、桌面、证书故障及预览通道。属于 GitHub-hosted 隔离 Panel/Agent 容器夹具，不冒充实体主机 systemd 或 OpenRC 更新。

15 附件由正式门禁及发布者独立下载，API size/digest、真实磁盘 SHA256 一致；11 项 SHA256SUMS 重新计算。说明4项摘要/3项升级提示。public artifact 12334648475 / sha256:9f690314e8a59f7639773781371ac98dbf7a3f8169636e692253fabcdefa8d95；publication artifact 12333648845 / sha256:b32dbded5d6eb60ef464dd7a7e528da0fd65a8e3c23031b4974d4ce9f4c29df3；原始 snapshot artifact 12334342468 / sha256:6e5b138244346f2a5896e920c356841ad9f84bda11a3968ffe9f785ac4798524。快照身份、run/attempt 和原始字节摘要另核，previousPreview 必须为 RC22/0dfc446b，stable 为 v1.26.0/e0fc7776；公开验收所用 snapshot 与原件字节一致。

测试 Runner 实际 ID=sha256:dc598debd6ef6dbb1b5ee85b68632dd24e38160a41d9b6c5e231b28cd441ee12；recipe=d82de7272da2795ef4793a600fb35bcef7b75c948b0d24dd2298581a28f14055，原始/实际摘要一致、diff为空、productImageUnchanged=true。不预设 Runner ID 与 RC22 相同。GitHub Latest 仍 v1.26.0，Docker latest 仍 sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9，发布前后独立回读通过。

## 生产和收尾

生产部署不适用（预览版禁止生产部署）。本轮未连接任何生产主机，prod-108 未连接、未核对、未备份、未部署、未升级。普通实体服务器的 systemd/OpenRC 安装、重启、回退与低配长时表现仍未验证。

唯一 release/v1.27.0-candidate 列车保留。自有 helper 完成同 SHA CI、main 快进和主线 CI 后，原子归档 archive/fix/v127-rc23-onedrive-preview@672e5b41405aa65003607ef85b8569ef041ecdbf，远端 readback、本地同名分支与 clean 工作树保留。自有业务事实修复分支 archive/fix/v127-rc23-business-review@0efa14b6a1f7690b008ae6f1d4bf7ee04dcf49bb，远端readback一致，本地同名归档/clean工作树保留，活跃远端ref不存在。三份来源已纳入 RC23，不再作为全新候选，但原作者工作树及服务未明确移交处置，保留待释放；发布者在所有权释放时复核。

本任务最初 acceptance 和最终 acceptance-final 两组 launcher 精确停止；自有 mock batch proxy PID33372 停止。未清理作者服务；非 loopback 4181 的其他 PID 未触碰。证据根 C:/GitHub/_release-evidence/v1.27.0-rc.23 保留，结束磁盘可用字节 51261235200。自有工具链/原始证据供重现，未清理未知内容。

本验收在独立 docs 分支提交、推送并精确读回，不并入冻结产品 main，不创建 PR。复用既有版本管理、发布、OCR 和预览工作流；未新增工作流或知识沉淀。本记录为项目强制发布交付。

## 交付节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-21T14:08:26+08:00
- 候选冻结时间：2026-10-21T16:19:09+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：14
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "public-acceptance/real-browser/dynamic-module-fetch",
    "count": 1,
    "impact": "原公开验收 restart 已通过16项（包括偏好持久化），随后 OverviewView 动态模块 fetch 失败，desktop-window 等待30秒超时；root attempt2 failure，preview-completion skipped。",
    "recoveryEvidence": "原 public-39569322955-2 ZIP、两份真实 initial/restart report、Panel日志保留；使用 snapshot-39569322955-2 的原始字节与不可变公开digest，经只读 Verify published preview 39577465506 恢复，不重建或覆盖RC23。",
    "permanentAction": "发布维护者在2026-10-25或稳定发布前（取较早）核查动态模块请求与TLS/网络日志；本轮无法从原件唯一确定网络失败原因，复验成功仍保留偶发加载风险，不宣称原root全绿。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "publication/run-source-checks/mcp-synchronous-wait-assumption",
    "count": 1,
    "impact": "attempt1 Release 源码检查中，旧 TestMCPManagedLostReceiptDoesNotRetry 在合法 executing 状态上要求立即 unknown，发布在快照、draft、镜像写入前停止。",
    "recoveryEvidence": "原日志与 attempt1 root JSON 保留；生产接口等待上限100ms，其他既有测试轮询 operation_status；MCP 文件相对RC22无变更，相同SHA候选/主线/L3通过。确认Release和版本镜像404后，只在新GitHub runner重试失败阶段一次；attempt2另核原件，不改源码或tag，不覆盖发布。",
    "permanentAction": "MCP维护者在2026-10-25或稳定发布前（取较早）修正测试的异步终态等待，并补确定性回归；本预览保留时序敏感风险，重复同指纹即停止重试并形成新候选。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/task-preflight/missing-authorization-field",
    "count": 1,
    "impact": "验收文档契约首次资格化缺少 authorization 字段而被拒绝，尚未写入仓库。",
    "recoveryEvidence": "补录用户发布请求和项目强制验收要求的既有授权来源后，L0 ready passed；未新增权限。",
    "permanentAction": "复用完整契约字段，发布者在下一次文档资格化前核对。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate/business-context-freshness/stale-baseline",
    "count": 1,
    "impact": "首次候选 CI 被 60 提交刷新门槛拦截，产品测试未执行。",
    "recoveryEvidence": "39567327769 原日志；独立修复 0efa14b6 刷新累计事实，新候选 accb4c12 的 CI39568373827 和完整发布通过。",
    "permanentAction": "保持现有 50 提交阈值；发布者在后续冻结前检查累计业务基线，期限为下一次预览。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate/check-collaboration-state/invalid-review-trailer",
    "count": 1,
    "impact": "本地未推送的组合消息被提示复核 trailer 不合规。",
    "recoveryEvidence": "发布绑定前修正 provider/result/fallback，最终 writer 检查 recorded；未改写已推送候选。",
    "permanentAction": "后续组合复用已核对的规范 trailer；发布者负责，下一次冻结前复核。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-ui/cua/input-no-visible-effect",
    "count": 1,
    "impact": "首次语义点击仅获得焦点，没有打开弹窗。",
    "recoveryEvidence": "读取实际界面后通过键盘 Enter 完成设置与批量操作，AX 和模拟 calls 原件保留。",
    "permanentAction": "先检查实际状态，不把点击调用成功当作旅程通过；发布者下次 UI 验收继续核对。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-ui/cua/locator-semantic-mismatch",
    "count": 3,
    "impact": "类型标签、配置说明及刷新后的批量按钮语义定位各一次未匹配，未据此记录通过。",
    "recoveryEvidence": "fresh DOM/AX 后使用实际 combobox 和运行任务按钮；配置帮助展开不计为已验收。",
    "permanentAction": "使用当前可见角色与名称；发布者下一次浏览器验收复核，帮助展开和原生缩放仍保留未验证。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/cua/screenshot-compositor",
    "count": 3,
    "impact": "一次截图超时、两次截图仅有空白背景，均不作为视觉通过证据。",
    "recoveryEvidence": "保留本地原件和明确限制；核心交互采用 AX/DOM 与 mock calls，图库复用精确生产文件相同的来源多浏览器回归，公开浏览器 PNG 另行验收。",
    "permanentAction": "发布者在下一次本地视觉任务前核查 CUA 截图通道；未恢复时明确未验证，不降低产品门禁。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/artifact-reader/assumed-file-location",
    "count": 1,
    "impact": "一次读取 OCR coverage 时误用不存在的子目录。",
    "recoveryEvidence": "列实际证据文件后读取根目录 ocr-coverage.json；保留原件并生成精确候选绑定。",
    "permanentAction": "从实际文件清单读取；发布者下一轮取证前复核。",
    "position": "before-production-write",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-evidence/local-summary/innertext-missing-input-value",
    "count": 1,
    "impact": "本地汇总器误以为 innerText 包含输入值，首次拒绝汇总。",
    "recoveryEvidence": "核对原始 AX 输入值及错误文本，汇总器只对 innerText 中真实包含的错误和保存结果断言；原候选与 UI 不变。",
    "permanentAction": "区分控件值和 innerText；发布者下次验收使用 AX/DOM 字段值单独取证。",
    "position": "before-production-write",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->
