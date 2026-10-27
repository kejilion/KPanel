# KPanel v1.28.0-rc.11 发布验收记录

日期：2026-10-28（Asia/Shanghai）。发布级别：L3；`releaseChannel=preview`，`releaseTrain=1.28.0`。公开预览产物已发布，生产未部署。

候选提交／标签：`23abf8641c210ef2fc2bcff5d0f0e8f92168e92a` / `v1.28.0-rc.11`；[Release](https://github.com/kejilion/KPanel/releases/tag/v1.28.0-rc.11) 为非草稿 prerelease，19 个附件。版本与 preview 镜像摘要 `sha256:bef50883d167d8bccae3549722b0ff9dce1fc6fca7e228d9754a48d813cd030c`。上一预览回滚点 `v1.28.0-rc.10` / `sha256:184d4ba2c80db45869340bafb66239515b34f6728dae8de61bb3c1198cb561a7`；正式 Latest 保持 `v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`。退出预览不自动降级。

## 发布范围与分支

| 来源分支 | 精确 tip | 用户价值与处置 |
| --- | --- | --- |
| `fix/preview-request-recovery` | `cbcd1ad7e4060d60d1c3ca0c3282e92b68e29b88` | 缓解反向代理并发连接误拒绝和白屏；恢复入口、认证忙碌限额及诊断。本地分支与工作树保留。 |
| `fix/light-node-retry-backoff` | `a69854757b345e90a53fa688f19ae34af31db54c` | 轻量节点断连重试退避。本地分支与工作树保留。 |
| `fix/browser-compose-project` | `9cba235528af3b5469347e1d6250d7c571732d3a` | 识别实际 Compose 项目名；远端与本地分支、工作树保留，待明确归档处置。 |

以上 tip 均为候选祖先，归档未执行；RC 列车 `release/v1.28.0-candidate` 保留且远端 tip 为 `23abf864`。历史候选、探测分支和外部 PR 未纳入。发布画像：L3；影响登录/静态资源、节点通信与浏览器组件安装旅程。`scriptLinkageState=not-required`；内置 `kejilion.sh` 基线 `614d58cee228007012a2ec602cbdc8dbaeb7950d` / SHA-256 `ec6d52da028ad6c6145fbb9c6796cd1548104b288f9234551c3b3d800b43eec9`，无需发布脚本（不适用）。

## 验证与限制

[候选 CI 40596595575](https://github.com/kejilion/KPanel/actions/runs/40596595575) 成功。[预览发布 40597804483](https://github.com/kejilion/KPanel/actions/runs/40597804483) attempt 1 成功：候选复验、GitHub-hosted L3、主线及标签提升、主线 CI、Windows 附件、Release、双架构镜像和公开验收通过。L3 receipt 绑定精确候选、Go 1.27.3、Node 24.22.1；源码 Web/Go/部署三个分组通过。公开验收核对 19 附件、15 项 SHA256SUMS、amd64/arm64、正文与 stable-before；镜像正文摘要 5 项、升级提示 2 项。浏览器公开版 45 项、升级对话框 31 项，以及隔离容器初次／重启 48/26 项检查通过；覆盖面板发布信息接口、预览加入与退出、更新弹窗及精确镜像摘要。公开验收使用隔离夹具，不代表真实生产安装、系统重启或真实多主机验证。

来源安全盲审原件保留；轻量节点文件流相关既有中级线索未确认，未作为已修复漏洞。release maintainer 独立于产品实现任务复核来源；同一发布责任人组装合并、准备版本并重算六平台实际／原始依赖闭包和 1251 源文件身份，不声称对其组装另有独立复核。原扫描发现和观察期限未变化；最终候选 CI 和 L3 已重新扫描。浏览器组件真实服务器安装与 Windows 更新回滚此前“访问被拒绝”的根因仍未验证；rc.10 原始失败保留，后续通过不能证明根因消除。

[依赖时效报告 40596595375](https://github.com/kejilion/KPanel/actions/runs/40596595375) 失败：TypeScript 7 和 @types/node 26 的既有例外复核日期已到，责任人为 KPanel base maintenance；不将该独立报告记作通过。来源浏览器候选 [CI 40519146035](https://github.com/kejilion/KPanel/actions/runs/40519146035) 因源码扫描身份不匹配失败；本版重新观察并绑定身份后最终候选 CI、L3 和发行扫描通过。历史首败保留。原始证据可从 Actions artifacts 及 `C:/GitHub/_release-evidence/kpanel-v128-rc11` 读取。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-27T22:13:47+08:00
- 候选冻结时间：2026-10-28T06:40:06+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：未记录
- 其中生产写操作开始后异常次数：未记录
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->
