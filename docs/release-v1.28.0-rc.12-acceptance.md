# KPanel v1.28.0-rc.12 公开预览验收补录

日期：2026-10-29（对 2026-10-28 公开预览证据作历史补录）

> 本记录补齐 RC12 缺失的仓库内逐列车验收文件。只采用原始 GitHub run、发布身份快照、公开 Release 与隔离验收结果；它不把预览结果当作稳定版证据。

## 版本与公开身份

- 发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.28.0`
- 候选提交 / 标签：`9c6f9211c2cb37b085263f967a2b0581b818329b` / `v1.28.0-rc.12`
- Annotated tag object：`37826ad310ff0594bfd27ac14c31a77a5bf1fd2b`，指向上述候选提交。
- GitHub Release：[v1.28.0-rc.12](https://github.com/kejilion/KPanel/releases/tag/v1.28.0-rc.12)，`draft=false`、`prerelease=true`，公开时间 `2026-10-28T08:08:51Z`。
- 发布工作流：run `40650239905`，controller `004b789e186b1dee78c53a406b94290991d3c2f0`，终态成功。输入校验、L3、候选 CI（verify 与 Windows node）、提升、主线 CI（verify 与 Windows node）、Release 与公开验收均成功；`stable-completion` 按预览版流程跳过。
- 公开面板 OCI digest：`sha256:773f5dd758cd46b5969fb8748b2dfd96adf2607a5cdf9f406421d20595d61257`。
- 发布身份快照 SHA-256：`6814b859f33c699ea8968c050e5cf1de85ab822e6207c3c8e2ba47e618aa34aa`；冻结资产清单列出 19 个附件，公开 Release API 同样返回 19 个附件。`SHA256SUMS` 在该清单中。

## 用户可见范围

- 可选浏览器组件增加 VNC 画面传输并在连接恢复后继续显示现有会话。
- 修复终端原生滚动与新输出并发时的底部跟随，以及浏览器活动视口同步、重连初始状态和显示同步失败恢复。
- 浏览器 VNC 需要配套 `kpanel-browser 0.1.0-rc.3`；旧截图协议镜像不兼容，Panel、Agent 与浏览器组件需配套更新。
- 本列车没有数据库迁移或生产部署；退出预览不自动降级。RC12 的升级说明要求操作前备份并在回退时恢复 `v1.28.0-rc.11` 配套版本。

## 自动与公开验收

- GitHub-hosted L3、候选 CI 与发布后主线 CI：成功，见 run `40650239905`。
- 发布快照的初始阶段：48 项通过，0 errors、0 assetFailures；重启阶段：26 项通过，0 errors、0 assetFailures。
- 真实浏览器公开发布旅程：45 项通过、0 errors；升级旅程：31 项通过、0 errors。两项报告状态均为 `passed`；这两份浏览器报告未单列 `assetFailures` 字段。
- 公开验收使用隔离 Panel/Agent 容器和浏览器；`mocked=false`。Docker inventory 与测试 CA 使用明确标记的测试夹具，因此不等于生产安装、宿主机服务或真实 CA 验收。
- 面板发布信息接口返回 RC12 与上述 digest；稳定通道仍为 `v1.27.2`。真实生产主机、长时 VNC、物理触控、读屏和真实 WAF/CF 数据源未在这份验收中证明。

## 生产与审计边界

- 生产动作：不适用；未连接、备份、部署或升级生产环境。
- 本记录证明的是该 RC 的 L3 与公开浏览器验收，不追溯声称完成独立 Cloudflare 审计或把后续稳定版内置安全复核提前套用到 RC12。
- 原始证据目录：`C:\GitHub\_release-evidence\kpanel-v128-rc12\final-20261028-preview-run406502`。发布身份为 `publication\_temp\kpanel-public-40650239905-1\release-assets.json`；阶段报告位于 `public\kpanel-real-40650239905-1\results` 和对应 `kpanel-browser-*` 目录。

## 交付节奏与流程异常补录

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-28T11:45:22+08:00
- 候选冻结时间：2026-10-28T15:17:00+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（无回滚或紧急热修复；未发现创建第二个 RC12 Release）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preview-release/inputs/automation-not-on-main",
    "position": "before-production-write",
    "count": 1,
    "impact": "Publish preview run 40669866603 在输入预检失败；候选 SHA fbf2f5bd4907540354b19f1f26c07b6a61a14998 的发布自动化与 main 不一致，后续阶段未运行。",
    "recoveryEvidence": "run 40650239905 已对最终 RC12 候选 9c6f9211c2cb37b085263f967a2b0581b818329b 完成公开发布与验收；后续 run 40671168205 对同一候选全链路成功。GitHub Release 仍为原 ID 426397268，公开时间 2026-10-28T08:08:51Z。",
    "permanentAction": "发布前确认 `scripts/github-preview-release.mjs` 与相关自动化已集成 main，核对精确候选 SHA；仅在该门禁通过后派发，并保留输入失败原件。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->
