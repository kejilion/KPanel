# KPanel v1.28.0-rc.15 发布验收记录

日期：2026-10-29（Asia/Shanghai）

发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.28.0`

候选提交 / 标签：`536561312f658dbed2dcb0171f244ddfff067ab7` / `v1.28.0-rc.15`

上一稳定版本 / 回滚点：`v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`

候选分支与发布后处置：`release/v1.28.0-candidate` / 预览候选分支保留

- 原分支 / 精确 tip / 处置分类：`release/v1.28.0-candidate` / `536561312f658dbed2dcb0171f244ddfff067ab7` / 保留。
- 归档 ref 与 SHA / 远端复核结果：远端分支与注释 tag 均指向候选 SHA；tag object 为 `a1672e9058a4c6615add5b4e1f52dc229f61582d`。不归档预览列车。
- 本次来源任务分支：`fix/terminal-menu-path-colors`（`3a2f0496`）、`feature/desktop-drag-copy`（`8e48e2cd`）、`fix/browser-vnc-restore`（`d1f6a126`）已合入；RC14 已含网站防护状态和日志入口。`feature/browser-vnc-prototype` 不作为整支合并，兼容的远端光标隐藏行为由候选提交 `d099d6b` 覆盖。CF 模式解释任务没有代码改动。
- 本地分支/upstream/worktree：候选 worktree `C:\GitHub\_release\kpanel-v128` 干净，精确 tip 与 `origin/release/v1.28.0-candidate` 一致；本地分支未设置 upstream 跟踪配置。主线已包含候选并追加发布验收工具提交。
- 未完成归档项 / 责任人 / 下次复核触发条件：候选及来源 worktree、证据目录均保留；下次候选列车整理时由维护者复核。

归档不代表生产上线；本版为公开预览，生产未部署。

## 发布画像

- 业务域：终端输出着色、桌面快捷方式操作、浏览器 VNC 会话状态响应。
- 变更面：展示与交互；没有数据库迁移、Agent 权限扩大或主机安全策略自动变更。
- 受影响用户旅程：中文菜单斜杠和真实路径着色；按修饰键拖动复制桌面快捷方式；浏览器 VNC 状态刷新与控制输入并发。
- 未变化契约：Panel API 数据库 schema、Compose/端口、Agent 权限、`kejilion/apps` 契约不变；不更新内置 `kejilion.sh`。
- 风险等级及理由：中等。远程浏览器交互与终端着色有公开浏览器和候选门禁覆盖；真实宿主机 VNC 长时运行与真实 WAF/CF 数据源仍未实机验收。

## 发布范围与未纳入内容

- 用户可见更新：桌面快捷方式可按住 Ctrl（macOS 使用 Option）拖动复制；终端不会把中文菜单中的斜杠误判成路径；浏览器 VNC 会话元数据刷新有界异步，不占用控制输入锁，并隐藏远端光标；继续使用内置 noVNC 和固定的 `kpanel-browser 0.1.0-rc.4`。
- 精确提交范围：`a049f7ba3c6cc15aee825671dddea1cd1d5133e5..536561312f658dbed2dcb0171f244ddfff067ab7`。来源提交包括 `3a2f0496`、`8e48e2cd`、`055d07ef`、`d099d6b`、`a76b9b99` 及 RC15 评审、身份固定和版本组装提交。
- 明确未纳入的分支、文件或后续事项：`codex/waf-migration-compat-20261028` tip `b9012cf8` 未合入。盘点时该分支较当前主线落后 42 个提交；递进封禁状态没有贯通脚本协议与 KPanel 页面，且没有最终候选 CI、L3 和完整安全验收。RC15 保留 RC14 已发布的防护状态、Jail 计数及日志展示，但不包含此分支提出的最新脚本/共享包固定，也不宣称递进封禁已在页面展示。该候选后续更新主线、补齐页面和验证后再评估。

## 外部审计与修复交付

- 安全盲审补漏：本记录没有执行独立 CF `security-boundary-audit`；未将未执行的审计记为通过，也不作全仓安全保证。发布流程中的源码、依赖、秘密和镜像扫描通过。
- `security-review-scope`：本次未单独运行。
- finding fingerprint / 修复 commit / 独立复核：本轮无经独立审计确认的新 finding；公开验收自动化缺少显式点击恢复按钮的问题已由测试控制器提交 `1c25820c2d0992d45d80b0ccafb9e12dd36bebc7` 修复，不修改产品载荷。
- OCR 观察区间 / 计数：本轮没有新增 OCR 观察结论；不据此宣称工具有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：沿用 RC14 的固定来源；本轮未变更。
- 脚本候选 commit / SHA-256：不适用。
- 判定依据：RC15 不消费新脚本或共享包版本；WAF 分支因尚未完成产品适配而排除。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 CI、L3 通过；公开容器及浏览器旅程通过。 | 真实 VNC 宿主机联调未覆盖。 |
| 网络入侵与供应链安全 | 已验证 | 发布工作流源码、依赖、镜像扫描及 SBOM/provenance 门禁通过。 | 独立 CF 审计未执行；扫描通过不代表全产品无漏洞。 |
| 稳定性、失败恢复与兼容 | 已验证 | 最终公开验收的启动及重启阶段通过，错误和资源加载失败均为 0；预览更新信息与 stable/preview 版本匹配。 | 不代表长时间 VNC soak 或所有代理/网络环境均已覆盖。 |
| 性能与资源预算 | 已实现未实机验证 | VNC 元数据轮询、输入队列和终端着色均有界。 | 未测真实主机峰值、多人并发及长时负载。 |
| 用户体验与可访问性 | 已验证 | 公开真实浏览器 journey 通过；终端斜杠边界、桌面复制和 VNC 操作由候选回归覆盖。 | 读屏、实体触控及所有缩放主题未专项验收。 |
| 数据、配置与迁移 | 不适用 | 无 schema、配置格式或持久数据迁移。 | WAF 新递进封禁候选未纳入。 |

## 自动门禁

- 定向测试及结果：终端候选测试和类型检查通过；最终候选 CI 两平台通过。发布验收控制器改动的 `git diff --check`、`node --check` 和发布测试 36/36 通过。
- `make verify-release`：由 GitHub-hosted L3 入口执行并通过；本机没有单独运行。
- L3：Publish preview run `40834539339` attempt 1，L3 job `123316206984` 成功；证据目录 `C:\GitHub\_release-evidence\kpanel-v128-rc15-run-40834539339\l3`。
- 候选 CI：同一发布 run 的 verify job `123316207049` 与 Windows job `123316206931` 成功。更早失败见流程异常明细。
- 主线 CI：发布 run 内 verify `123320600620` 与 Windows `123320600683` 均成功。测试控制器修复提交 `1c25820c` 的后续主线 CI `40840470078` 也通过。
- Release workflow：run `40834539339` 的输入、候选 CI、L3、主线晋升、Release 与镜像发布均成功；原 `public-acceptance` 因重启 journey 未点击页面已有的“重新加载”而失败，preview completion 因而未完成。只读 recovery run `40841859159` 后续通过；没有重建或覆盖已发布镜像。
- 发布说明门禁：最终正文通过；版本 `1.28.0-rc.15`、4 条摘要、1 条升级提示。
- 安全扫描、镜像契约、SBOM/provenance：发布工作流门禁通过；浏览器组件 `0.1.0-rc.4` index 为 `sha256:37c0eb1c4a0e6408fe395e699687d405fe78d052669c32625e09a17a7e3165af`。

## 依赖与技术栈变化

- `make dependency-report`：本轮未单独运行；发布 workflow 依赖审计通过。
- 依赖 freshness：独立 run `40833142638` 提醒 TypeScript 与 `@types/node` 的维护复核日期逾期；没有升级这些依赖，本项仍交由后续 base maintenance 复核，不阻断 RC15。
- 工具链和镜像：沿用项目锁定工具链；RC15 面板 OCI index 为 `sha256:2db52a45d988c8376efe286ad157b266b567b6d3193f4bd5f89f296e34f2184d`。
- 暂缓候选：WAF 最新脚本适配暂缓至页面协议、候选基线和实机/安全验证补齐后复核。

## 隔离真机与浏览器验收

- 环境：GitHub-hosted Linux runner；真实 Panel/Agent 容器、隔离网络和 Playwright 浏览器。测试夹具明确标记；未挂载宿主 Docker/systemd socket。
- 使用的精确公开产物：候选 `536561312f658dbed2dcb0171f244ddfff067ab7`；image `sha256:2db52a45d988c8376efe286ad157b266b567b6d3193f4bd5f89f296e34f2184d`。
- 首次发布验收 run `40834539339` 与只读复现 run `40839715012` 均在重启后的页面恢复步骤等待 `.desktop-window` 超时；截图确认 bootstrap 恢复页和显式“重新加载”按钮存在。修复测试 runner 后，run `40841859159` 的初始阶段 48 项、重启阶段 26 项全部通过，均为 0 errors、0 assetFailures；恢复阶段确认终端历史、首选项、桌面状态及 RC15/stable 发布信息跨重启保持。
- 同次 recovery 的 published browser 45 项和 upgrade browser 31 项均通过，errors=0。
- 未执行场景：真实生产主机、真实 CF/WAF 日志源、物理 VNC 端点、长时 soak、读屏和实体触控。
- 证据目录：`C:\GitHub\_release-evidence\kpanel-v128-rc15-run-40834539339\public-acceptance-recovery-40841859159`；首次失败与复现证据仍保存在相邻 `public-acceptance` 和 `public-acceptance-recovery-40839715012` 目录。

## 发布产物与公开仓库复核

- GitHub Release：[v1.28.0-rc.15](https://github.com/kejilion/KPanel/releases/tag/v1.28.0-rc.15)，`draft=false`、`prerelease=true`，published at `2026-10-29T07:57:45Z`。GitHub Latest 仍为 `v1.27.2`。
- Docker：`docker.io/kjlion/kejilion-panel:1.28.0-rc.15` 与 `:preview` index 均为 `sha256:2db52a45d988c8376efe286ad157b266b567b6d3193f4bd5f89f296e34f2184d`；`:latest` 仍为稳定 digest `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`。
- 架构摘要：amd64 `sha256:055164fb10fdf42a0ac9326f27cebdeaa9632b5aa47cbd4f47349dab639b6021`；arm64 `sha256:955720715c5e514601bf69dca4edda6721d5c2aedf47311170c5a52e439e849e`。
- 附件与 `SHA256SUMS`：公开回读通过，15 项 checksum；公开 image e2e 与两条浏览器验收均通过。
- 发布接口：恢复验收中的 `/settings/kpanel-release` 返回 preview `1.28.0-rc.15` 与预期 digest，stable 为 `1.27.2`；正文包含 4 项摘要和 1 项升级提示。
- `kejilion/apps` / `kejilion.sh`：契约不变，未发布新脚本版本。

## 自更新通道验收

- stable 与 preview 均返回预期版本和唯一官方 digest；加入预览不会自动安装，退出预览不会自动降级。
- Panel 重启后 stable/preview 发布信息保持一致；公开浏览器 upgrade journey 通过。
- 本次未执行生产自更新；OpenRC、轻量 Node 和物理设备未专项重测。

## 生产部署安全核对

- 生产动作：不适用（预览版禁止生产部署）。本次未连接、未备份、未部署或升级生产环境。
- 真实生产目标与部署后状态：不适用；公开验收在隔离 runner 和容器中完成。

## 回滚

- 源码/tag：`v1.28.0-rc.14`，候选 `c928683951d07923d81ab206d9c5d65d058ade33`。
- 镜像 digest：RC14 `sha256:5e6fe52fd2382579879bfde1f3efaa6330ea2de2aa1a3da8eda95ca732d9ded2`；RC15 数据与配置未迁移。
- 回滚步骤：用户可退出 preview；退出不自动安装较低版本。如需手动恢复，固定 RC14 镜像 digest 并检查健康状态。
- 稳定默认：GitHub Latest 与 Docker `latest` 继续指向 `v1.27.2` / `sha256:5369dc9a2b8151314181d306e8d490939e63607d017b07c3985a0f4afb65aa10`；生产未部署，无生产回滚操作。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-29T10:17:01+08:00
- 候选冻结时间：2026-10-29T15:04:28+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（无产品回滚、紧急热修复或重复公开；公开验收重跑属于流程异常）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/business-context/stale-review",
    "position": "before-production-write",
    "count": 1,
    "impact": "Candidate CI 40829270628 被业务上下文 freshness 门禁拦截；canonical review 已达到 57 commits。",
    "recoveryEvidence": "刷新 RC15 业务上下文后，最终候选的 CI 与发布 run 40834539339 均通过。",
    "permanentAction": "在冻结候选前更新 docs/product-quality-review-current.md 并重新运行 freshness 门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/source-inventory/stale-hash",
    "position": "before-production-write",
    "count": 1,
    "impact": "Candidate CI 40829675362 的 source inventory hash 与候选实际文件摘要不一致，验证步骤失败。",
    "recoveryEvidence": "以提交 df082771 重新绑定 RC15 source identity；最终精确候选 CI 和发布 run 40834539339 通过。",
    "permanentAction": "冻结前从精确候选重新生成并核对 source identity，保留失败 artifact 与日志。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-acceptance/restart-recovery/missing-reload-click",
    "position": "before-production-write",
    "count": 2,
    "impact": "公开验收 run 40834539339 与只读复现 run 40839715012 在重启后等待 .desktop-window 超时；浏览器已显示产品提供的 bootstrap recovery 页面，但 runner 没有点击其显式重新加载按钮。",
    "recoveryEvidence": "测试 runner 修复提交 1c25820c 和主线 CI 40840470078 通过；只读验收 run 40841859159 对同一发布快照、版本和 digest 通过。",
    "permanentAction": "restart journey 在桌面未就绪时识别并点击一次现有恢复按钮，然后要求桌面加载成功；initial journey 仍保持严格检查。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

### 流程异常明细

两次候选 CI 的失败分别来自过期业务上下文与 source inventory 摘要不一致，均在发布候选冻结前拦截。公开验收的首次失败和只读复现具有相同 runner 根因；它们发生在 preview 已公开之后，但没有生产写入。修复只更新受信任的 main 测试 runner，不重建、覆盖或改写 RC15。最终公开验收在 2026-10-29T08:26:10Z（16:26:10+08:00）通过；GitHub 页面仍保留原始失败记录。

## 遗留风险与后续准入

- 本地资源回收：未执行；保留候选、来源 worktree 和完整发布/失败证据目录。
- 未验证风险：真实主机 VNC 长时稳定性、真实 WAF/Fail2Ban/CF 数据源、独立 CF 安全审计、低配资源和辅助技术尚未验证；TypeScript 与 `@types/node` 维护复核日期需后续处理。
- 后续准入：先将 WAF 兼容候选更新到最新主线，贯通递进封禁状态协议与页面展示，再完成候选 CI、L3、实机和相应安全复核；完成前不声称相关新功能已进入 RC15。
