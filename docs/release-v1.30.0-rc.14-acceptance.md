# KPanel v1.30.0-rc.14 发布验收记录

日期：2026-11-12

发布级别：L3

候选提交 / 标签：`16fa206dd5239ec37de5d095bffa168e5a74f9a2` / `v1.30.0-rc.14`

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览序列保留

- 发布时冻结候选、`main`、远端候选分支及 tag 均指向 `16fa206dd5239ec37de5d095bffa168e5a74f9a2`；RC14 继续使用同一发布列车候选分支。
- `fix/desktop-catalog-cold-start@e232d48d` 与 `feature/desktop-icon-url-import-20261112@f0cb04ef` 纳入 RC14；来源工作树保留，未删除。
- RC13 已纳入的 `fix/desktop-url-icon-auth-20261111@9c1fb8d7`、`fix/wallpaper-unified-handoff@dddf2ef7`、`feature/browser-start-links@09d33455` 继续随本列车保留；功能范围以各自已完成审查和 RC13 记录为准。
- 稳定版候选正式公开成功后再按 `docs/project-management.md` 10.2 归档发布候选，并核对来源分支工作树的所有权与状态；未处置前保留现有本地工作树。

产物已公开，生产未部署。

## 发布画像

- 业务域：桌面应用目录冷启动、快捷方式图标、浏览器初始页和桌面壁纸。
- 变更面：界面与只读站点图标读取；图标保存复用现有快捷方式/网站图标接口。
- 受影响用户旅程：桌面冷启动时加载 Agent 脚本应用；从图片 URL 导入快捷方式或网站图标并保存；浏览器初始页展示桌面入口。
- 未变化契约：无数据库 schema、Agent 协议、主机配置或 `kejilion.sh` 改动；站点图标读取仍受管理员会话、同源、CSRF、公网目标、超时和图片大小限制。
- 风险等级：中低；自愿加入的预览通道，未部署生产。

## 发布范围与未纳入内容

- 用户可见更新：Agent 内置应用目录冷启动后有限刷新并更新桌面；网站和 URL 快捷方式支持图片 URL 图标导入，包含等待、重试与取消。
- 精确提交清单：RC14 候选 `16fa206dd5239ec37de5d095bffa168e5a74f9a2`；RC13 基线已包含 URL 站点图标自动识别、壁纸过渡和浏览器初始页。
- 未纳入：全库后台媒体自动刮削、生产部署，以及 RC14 之外仍未合入本列车的本地候选。

## 外部审计与修复交付

- 安全盲审补漏：以上一稳定 tag `v1.29.0` 检查；`security-review-scope` 共 17 个敏感包组，全部 `recorded`，`missing=0`、`stale=0`，因此 review 档无待审单元；不将覆盖记录称为全仓无漏洞。
- 该范围另有一条旧 `Security-Review` skip trailer 不符合当前解析器格式；未将它计作覆盖，也没有包组因此变为 missing/stale。
- 本轮来源审查：桌面目录候选 OCR 覆盖 4/4 文件、H0/M0/L0、`constrained-only=0`；图片 URL 导入候选 OCR 覆盖 11/11 文件、H0/M0/L0、`constrained-only=0`。图片导入候选的独立审查记录有备用提供商不可用说明。
- 发布者在同一精确候选执行 CI 与 L3；不以盲审覆盖、来源分支测试或 RC13 结果代替当前候选门禁。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`；本版不修改或发布 `kejilion.sh`。
- KPanel 实际内置脚本基线：`kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff`，SHA-256 `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选、变更集编号及阻断范围：不适用。
- 本版决定：脚本不在范围；复用 RC4 起已公开且镜像固定的兼容脚本来源。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证（发布门禁） | 精确候选 CI、Windows Node、L3、主线 CI 与公开浏览器/隔离容器验收均成功 | 不表示所有外部图标站点都可用 |
| 网络入侵与供应链安全 | 已验证（发布门禁） | 候选扫描、L3、镜像身份、SBOM/provenance 与公开摘要核对成功 | 安全范围覆盖不等于零漏洞结论 |
| 稳定性、失败恢复与兼容 | 已验证（发布门禁） | preview 通道、双架构镜像、19 附件和公开升级旅程验收成功 | 未做生产安装、回滚或长时 soak |
| 性能与资源预算 | 已实现未实机验证 | 冷启动目录刷新有界并在离开桌面时取消 | arena-154 SSH 密钥尚未安装，未执行该主机性能对照或低配长期负载 |
| 用户体验与可访问性 | 已验证（候选与公开门禁） | 图标导入进度/失败路径、浏览器和桌面入口在公开浏览器/隔离容器通过 | 未做实体手机和完整读屏、缩放矩阵 |
| 数据、配置与迁移 | 已验证 | 沿用现有图标上传和工作区存储，无 schema 或 Agent 协议变化 | 无生产迁移 |

## 自动门禁

- 候选 CI：最终精确 SHA push run `43151185428` 成功；候选验证 `130964006345`、Windows Node `130964006332` 成功。
- GitHub-hosted L3：发布 run `43152246039` 的 L3 job `130964006305` 成功。
- 主线 CI：promotion 后的 Linux `130971638985`、Windows Node `130971638984` 均成功。
- Release workflow：run `43152246039`，Release job `130978366512` 成功；preview completion `130988279397` 成功。
- 公开验收：job `130987631057` 成功；公开浏览器及隔离容器状态文件为 `passed`。
- 发布说明门禁：RC14 的正式 Release 正文及附件检查通过，19 个附件含 `SHA256SUMS`。
- 安全扫描、镜像契约、SBOM/provenance 与最终发布摘要由同一 Release workflow 成功核对。

## 依赖与技术栈变化

- `scriptLinkageState=not-required`；RC12 已把 Panel/可选浏览器工具链更新至 Go 1.27.4 并固定 `kpanel-browser 0.1.0-rc.5`；RC14 没有依赖版本变更。
- RC14 预览镜像继续固定上面的公开 `kejilion.sh` commit 与摘要；stable/latest 未受预览发布影响。
- RC14 发布前出现一次已记录的 source-identity 摘要陈旧拦截；候选摘要在后续提交更新，最终候选重新通过 CI 和 L3。该流程异常详见本文末尾，不能改写为产品失败或已验证的稳定版结果。

## 隔离真机与浏览器验收

- GitHub-hosted 浏览器 Runner 与隔离 Panel/Agent 容器；不是生产主机。
- 精确候选：`16fa206dd5239ec37de5d095bffa168e5a74f9a2`；发布镜像 index `sha256:0fe00dc1800ce990f67d8882d50f49eb704732d504ca07e4516a78c3eaafd3da`。
- 公开旅程状态为 `passed`，包含初始、重启、已发布镜像与升级路径；图标和桌面冷启动候选随精确公开镜像验收。
- 未连接 arena-154：仓库允许性能验证，但共享 SSH 公钥仍待管理员安装；未因此声称主机性能实测通过。未执行生产写入或生产回滚。

## 发布产物与公开仓库复核

- GitHub Release：[v1.30.0-rc.14](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.14)，于 `2026-11-12T05:10:58Z` 公开，`draft=false`、`prerelease=true`，不是 GitHub Latest。
- Docker 预览 OCI index：`sha256:0fe00dc1800ce990f67d8882d50f49eb704732d504ca07e4516a78c3eaafd3da`；linux/amd64：`sha256:790ff0c606e15d9b2177f57aa66c1da723822d26995b44c83a51e508441b329f`；linux/arm64：`sha256:e3ef770ecec5829db7bbba82c0e5088d36e27ff5608bf759245bc6929816262a`。
- Release 附件 19 个；`SHA256SUMS` 和公开下载摘要校验通过。预览工作流及公开验收绑定同一版本、tag 和镜像摘要。
- Docker `latest` 与 GitHub Latest 保持 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`；生产未部署。

## 自更新通道验收

- preview 只选择正式稳定版和规范 RC；RC14 版本及摘要与 GitHub Release、Docker Hub 和更新旅程一致。
- 加入预览只切换来源并检查，不自动安装；自动安装与一次性安装相互独立；退出预览不会自动降级。
- 稳定 `latest` 未变化，RC14 不成为 GitHub Latest 或标准 stable 来源。
- systemd、OpenRC 与轻量 Node 更新边界沿用 `docs/release-channels.md`；本次公开验收未外推到生产安装。

## 生产部署安全核对

- 生产部署：不适用（预览版禁止生产部署）。未连接、备份、升级或核对 `prod-108`，未执行任何生产写操作。
- 只在 GitHub-hosted 浏览器和隔离容器执行验收；隔离环境结果不代表生产部署、真实宿主 systemd 或公网业务完成。

## 回滚

- 源码/tag：RC14 不可变 tag `v1.30.0-rc.14` / `16fa206dd5239ec37de5d095bffa168e5a74f9a2`。
- 稳定回滚镜像：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- 退出 preview 不会自动降级；如需恢复较低稳定版本，按现有更新流程备份数据后人工选择固定摘要。
- GitHub Latest、Docker `latest` 和标准 stable 来源仍指向 `v1.29.0`；preview 指向 RC14。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-12T11:34:17+08:00
- 候选冻结时间：2026-11-12T11:52:24+08:00
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
    "impact": "首轮候选 CI run 43149839953 在 release version literal 变更后发现 firstPartySourceSha256 陈旧；没有提升 main、创建 tag、公开 Release 或推送镜像。",
    "recoveryEvidence": "保留失败 run https://github.com/kejilion/KPanel/actions/runs/43149839953；候选 source identity 复核提交 16fa206dd5239ec37de5d095bffa168e5a74f9a2 后，精确 CI run 43151185428 和完整预览 run https://github.com/kejilion/KPanel/actions/runs/43152246039 均成功。",
    "permanentAction": "稳定候选提交前，Release Maintainer 必须按已集成的 scripts/task-preflight.mjs ready/handoff 与 source-identity 入口，对版本 Go 字面量更新执行精确候选检查；旧摘要必须在候选推送前失败关闭，并在审查变更后更新身份。稳定版验收记录应保留本次精确检查证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

RC14 首轮失败发生在任何主线、tag、Release 或镜像写入之前，原始运行保留；不以最终重试覆盖历史失败。RC5/RC9 的同类预览指纹仍由独立 RC 指标观察，不能填入只接受稳定标签的 `historicalReleases` 字段。
