# KPanel v1.30.0-rc.13 发布验收记录

日期：2026-11-12

发布级别：L3

候选提交 / 标签：`1b6bcbf79ba2c3ef698af4d9bc06985437d29131` / `v1.30.0-rc.13`

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览序列保留

- 发布时冻结候选、`main`、远端候选分支和公开 tag 均为 `1b6bcbf79ba2c3ef698af4d9bc06985437d29131`；RC 序列候选分支继续保留。
- `fix/desktop-url-icon-auth-20261111@9c1fb8d7`、`fix/wallpaper-unified-handoff@dddf2ef7`、`feature/browser-start-links@09d33455` 的功能进入 RC13；公开验收及相应源码在 RC13 中验证。
- 三条来源分支保留在本地，未创建远端归档引用、未删除分支或工作树；按 RC 序列规则暂不归档，后续稳定版收尾时再核对精确 tip 和所有权。
- RC13 发布后新候选不在本版：桌面目录冷启动修复和图片 URL 图标导入，留待后续 RC。

预览产物已公开，生产未部署。

## 发布画像

- 业务域：桌面快捷方式图标识别、壁纸过渡、内置浏览器初始页。
- 变更面：用户界面与只读浏览器入口；自动图标识别须管理员登录并访问公网地址，保存快捷方式沿用既有工作区写入。
- 受影响用户旅程：新建 URL 快捷方式时识别网站图标；切换桌面壁纸/动态场景；浏览器初始页查看应用、网站和 URL 快捷方式。
- 未改变：数据库结构、Agent 协议、主机配置、脚本生命周期、稳定默认通道。
- 风险等级：中低；显式选择的 RC 预览版，生产保持未部署。

## 发布范围与未纳入内容

- 用户可见更新：URL 快捷方式自动读取公开站点图标；桌面静态壁纸和动态场景统一淡黑过渡；浏览器初始页展示桌面应用、网站及 URL 快捷方式并提供图标失败兜底。
- 精确发布提交：`1b6bcbf79ba2c3ef698af4d9bc06985437d29131`；公开标签：`v1.30.0-rc.13`。
- 未纳入：RC13 发布后形成的桌面目录冷启动修复、图片 URL 手动导入；全曲库/影视自动刮削；生产部署。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`；本版不修改或发布 `kejilion/sh`。
- KPanel 内置脚本沿用既有版本；无脚本候选或脚本 SHA 变化。
- 本版决定：脚本不在范围；无需跨仓库发布。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证（发布门禁） | 候选 CI、Windows Node 检查、L3、主线 CI 和公开浏览器/隔离容器验收均成功 | 不表示第三方站点均可提供可用图标 |
| 网络入侵与供应链安全 | 已验证（发布门禁） | 精确候选安全扫描、源码与依赖扫描、镜像身份、SBOM/provenance 和公开产物核对成功 | 不据此宣称整个产品无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证（发布门禁） | 完整预览发布链路成功；发布说明与 SHA256SUMS 随 release 附件发布 | 未做生产回滚或长时间 soak |
| 性能与资源预算 | 已实现未实机验证 | 不新增常驻服务或后台队列 | 未采集新增低配长时性能数据 |
| 用户体验与可访问性 | 已验证（候选与公开门禁） | 预览界面和真实浏览器公开验收通过 | 实体手机及完整键盘/缩放矩阵未覆盖 |
| 数据、配置与迁移 | 已验证 | 无数据库、Agent 协议或主机配置迁移 | 无生产迁移 |

## 自动门禁、发布产物与公开仓库复核

- 预览发布工作流：[43069124150](https://github.com/kejilion/KPanel/actions/runs/43069124150)，最终结论 `success`。
- 候选 CI（含 Windows Node job）、L3、原子提升、主线 CI、Release、公开验收及 preview completion 均成功；L3 job `130692078108` 成功。
- 公开验收 job `130726987651` 成功，执行“Verify public release in real browsers and isolated containers”；本机没有读取其逐项用例计数，不据此补写实体设备或未见的细节。
- GitHub Release：[v1.30.0-rc.13](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.13)，`isDraft=false`、`isPrerelease=true`，19 个附件；不是 GitHub Latest。
- Docker 预览 OCI index digest：`sha256:cc618400f93778272a923c349d551754ecf050e46893dec7291af4e31889e37c`；amd64：`sha256:6f96bec6be8ad08ab2b231fa8245919a7ce1f87ccccdfedf9ce4876d4aa22ae2`；arm64：`sha256:e3ce7e6a8beffd28aad0688d5635c9bae5f0e58fd80e132b50d3c35e5df216f7`。
- Docker `latest` 及 GitHub Latest 保持稳定版 `v1.29.0`；稳定 OCI index：`sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- Release 附件含 `SHA256SUMS`；公开验收与发布工作流报告成功。脚本/应用市场契约检查成功。

## 生产部署与回滚

- 生产部署：不适用（预览版禁止生产部署）；本次没有连接、备份、升级或部署生产主机。
- 回滚点：稳定 `v1.29.0` 与 Docker `latest` digest `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- RC13 tag 为不可变源码恢复点；预览用户退出预览不会自动降级。

## 首轮发布门禁拦截及恢复

首轮工作流 [43065489244](https://github.com/kejilion/KPanel/actions/runs/43065489244) 的候选验证与 L3 于 2026-11-12 01:10（+08:00）发现 RC12 验收记录缺少固定交付节奏 marker 和结构化字段。提升、主线 CI 和 Release job 均跳过，未写入 `main`、tag、GitHub Release 或公开镜像。补齐 `docs/release-v1.30.0-rc.12-acceptance.md` 后，最终候选 `1b6bcbf7` 在后续工作流通过全部门禁并仅公开一次。此事件是无效验收证据在首次发布写入前被正常门禁拦截，不是产品缺陷或生产回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-11T23:47:45+08:00
- 候选冻结时间：2026-11-12T01:22:22+08:00
- 生产完成时间：不适用
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（首轮证据门禁拦截，最终预览只公开一次）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/preview-release/acceptance-metrics-missing",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮候选 CI 与 L3 因 RC12 验收指标缺少固定 markers 和字段而失败；main/tag/Release/镜像均未写入。",
    "recoveryEvidence": "https://github.com/kejilion/KPanel/actions/runs/43065489244；修复提交 1b6bcbf79ba2c3ef698af4d9bc06985437d29131；恢复工作流 https://github.com/kejilion/KPanel/actions/runs/43069124150。",
    "permanentAction": "补齐 docs/release-v1.30.0-rc.12-acceptance.md 的固定指标与流程事件结构，并在后续候选验证继续执行 release metrics 门禁。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- RC13 本地源分支保留，未创建归档引用或删除工作树；稳定版序列收尾时按 `docs/project-management.md` 10.2 复核。
- 未验证风险：真实公网图标可用性取决于第三方站点；本版公开验收不能证明所有站点都返回兼容图像。
- 本版已公开、生产未部署；后续功能候选另按 RC14 独立验收，不回写 RC13 发布范围。