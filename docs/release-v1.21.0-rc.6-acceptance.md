# KPanel v1.21.0-rc.6 发布验收记录

日期：2026-09-21

发布级别：L3

候选提交 / 标签：`d73e9c781294717cb8ce5cc552f0daf275582e1c` / `v1.21.0-rc.6`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `d73e9c781294717cb8ce5cc552f0daf275582e1c`

- 原分支 / 精确 tip / 处置分类：`fix/settings-version-route-first-paint-20260921` / `c64c1c3f40af1ddaad110b18aa78dff95260097f` / 在 rc.5 发布后形成，已纳入 rc.6 并归档。
- 归档 ref 与 SHA：`archive/fix/settings-version-route-first-paint-20260921` = `c64c1c3f...`；`archive/assemble/v1.21.0-rc.6` = `d73e9c78...`。远端复核一致。
- 本地来源 Git worktree 注册和活动分支已回收；路径只剩可再生成的 `web` 忽略缓存，不再是 Git worktree。
- `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass` 含未提交内容，继续原样保留且未纳入；`docs/security-audit-run2-local-20260919` 继续保持未发布。
- 未完成归档项：无。候选分支按预览列车规则保留。

归档不代表生产上线；本版仅发布预览产物，生产未部署。

## 发布画像

- 业务域：设置导航与版本更新入口。
- 变更面：前端首次渲染的分类初始化与回归测试，没有新增 API、数据迁移、宿主机权限或部署行为。
- 受影响用户旅程：用户点击当前版本进入设置时，首屏直接显示“系统”分类和版本更新区域。
- 未变化契约：rc.5 的可点击版本号、设置搜索、分类导航、粘滞导航、Panel/Agent、端口、Compose、受管脚本和稳定更新入口均未改变。
- 风险等级及理由：L3。代码改动小，但它直接修复预览入口的首屏状态；继续使用完整 L3、候选/main CI、Release 安全链和公开 OCI E2E 验证。

## 发布范围与未纳入内容

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.6]`：版本更新路由意图在首次渲染前选择系统分类。
- 精确提交清单：`c6dca0a6`、`d73e9c78`；基线包含 rc.5 产品提交与 `docs/release-v1.21.0-rc.5-acceptance.md`。
- 明确未纳入：生产部署、与设置入口无关的功能或依赖升级、长期浏览器 soak。

## 外部审计与修复交付

- 本 RC 没有新增可引用的 Cloudflare `security-audit-skill` 最终结构化结论；Alibaba open-code-review 按小范围规则记录 `skipped reason=L1 code change under 30 lines`，不把 skipped 改写为通过。
- 外部审计能力继续作为辅助证据，不替代项目回归、L3、安全扫描、候选/main CI 和公开镜像验收。
- 修复提交 `c6dca0a6` 绑定精确 rc.5 基线，新增首次渲染回归；独立 L3 和公开产物验证完成。
- 稳定周期观察仍按 `PROJECT_RULES.md` 5.4/5.5 延续，不根据单个小改动提前判断工具有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`2b90b2d2ca56bc954c9328a51bb5571e896f713d` / `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：本轮仅修改设置前端和测试；L3 managed-script-contract 与应用生命周期通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 新增首次渲染用例 30/30，完整 Web 174 文件 / 1528 项及完整 Go 通过。 | 无后端协议变化。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/镜像 0 阻断项，SBOM/provenance 生成。 | 3 个模块通告不可达，继续跟踪。 |
| 稳定性、失败恢复与兼容 | 已验证 | 固定 Linux L3、核心 race、应用生命周期、候选/main CI 和公开 OCI E2E 通过。 | 未做长期浏览器 soak。 |
| 性能与资源预算 | 已验证 | 仅在 setup 初始化一个枚举值，没有新增请求、轮询或持久资源。 | 不适用独立性能压测。 |
| 用户体验与可访问性 | 已验证 | 首帧即可见系统分类和版本更新区域；rc.5 搜索、分类、粘滞导航测试继续通过。 | 未另做最终 SHA 多浏览器缩放人工矩阵。 |
| 数据、配置与迁移 | 不适用 | 没有 schema、配置格式或持久数据变化。 | 不适用。 |

## 自动门禁

- 定向测试：`SettingsView.test.ts` 30/30；完整前端 174 个文件 / 1528 项、typecheck、3296 个短语与生产构建通过；治理 203/203、16 个提案、11 个依赖组通过。
- L3 Runner：`kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；完整 Go/Web、核心 race、govulncheck、npm audit、Trivy、双架构、镜像和 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：`v1.21.0-rc.6-d73e9c7-l3-r1`，2026-09-21 00:32:07 至 00:49:10 +08:00，exit 0；bundle `f1546629...`、plan `bc1ccfeb...`、remote entry `d8bb2cf2...`、远端日志 `daef361c...`。证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.6-d73e9c7-l3-r1` 和 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.6-d73e9c7-l3-r1`。
- 候选 CI：[CI 35523924669](https://github.com/kejilion/KPanel/actions/runs/35523924669) 与 [freshness 35523924699](https://github.com/kejilion/KPanel/actions/runs/35523924699) 成功。
- 主线 CI：[CI 35524278914](https://github.com/kejilion/KPanel/actions/runs/35524278914) 首次 race 因回执丢失用例的瞬时调度窗口失败；同一 SHA 的 attempt 2 成功。[freshness 35524278921](https://github.com/kejilion/KPanel/actions/runs/35524278921) 成功。
- annotated tag object `32e9aead900aba194f9bf0fc943a97245327033c` 指向产品提交；[Release 35525047110](https://github.com/kejilion/KPanel/actions/runs/35525047110) 与 [tag freshness 35525047109](https://github.com/kejilion/KPanel/actions/runs/35525047109) 成功。

## 依赖与技术栈变化

- 本版没有依赖、工具链、Action、基础镜像或受管脚本升级；dependency policy validate-only 和候选/main/tag freshness 通过。
- Go 1.26.7、Node 24.20.0、Trivy 0.72.0 与固定 Runner 沿用冻结工具链。
- 安全与兼容结论来自同一精确源码的 L3、Release 和公开镜像；回滚点保持 rc.5 / v1.20.0。

## 隔离真机与浏览器验收

- 主机：`arena-154`，Linux/amd64、Docker；环境策略允许 candidate-validation，禁止 `prod-108`。
- 公开 OCI 使用不可变摘要 `sha256:0d4cee03b47adf005f23ee6bb9589ddeae70e5696583dfe39e4161f533624d7c`，镜像 version/revision 精确匹配。
- 在临时端口 `18186` 执行 `packaging/tests/image-e2e.sh`；`image_e2e=pass`、`public_oci_e2e=pass`，临时容器、网络和远端目录已清理。
- 证据：源码 archive `4ad67ae0...`、执行脚本 `070a8de7...`、日志 `493aaa5d...`，位于 `C:/GitHub/_release-evidence/v1.21.0-rc.6-public-image-e2e-r1`。
- 首屏分类由组件 setup 回归覆盖；未执行最终 SHA 的人工多浏览器缩放矩阵和长期 soak。

## 发布产物与公开仓库复核

- [KPanel v1.21.0-rc.6](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.6) 为非 draft、prerelease、非 Latest；发布时间 2026-09-21 01:20:53 +08:00，GitHub Latest 仍为 `v1.20.0`。
- 14 个附件均为 uploaded 且非空；`SHA256SUMS` 自身摘要 `4a29b224...` 与 GitHub API digest 一致，包含 11 个二进制/部署包条目。
- Docker `1.21.0-rc.6` 与 `preview` OCI index 均为 `sha256:0d4cee03b47adf005f23ee6bb9589ddeae70e5696583dfe39e4161f533624d7c`。
- `linux/amd64` / `linux/arm64` 分别为 `sha256:ed470e2568459ade805e010f86ad39f4fa294828e2e9b82363c98ad9994842f7` / `sha256:d15e11a36e65438f0160b38a21425b144288902408910c7d34121460ed25dc1c`；另外两项为 attestations。
- 稳定 `latest` 与 `1.20.0` 均保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。

## 自更新通道验收

- 稳定来源继续只接受正式 GitHub Latest；预览来源接受同列车 RC，并可在没有更新 RC 时接受更高稳定版本。
- 加入预览只切换来源并立即检查，不自动安装；自动安装和一次性立即安装相互独立。
- 旧状态默认迁移到 `stable`，退出预览不会产生降级候选；systemd 更新、备份和失败恢复由 L3 生命周期覆盖。

## 生产部署安全核对

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证环境：仅 `arena-154` 的隔离 L3 和公开 OCI E2E。
- 正式部署环境：不适用。
- `prod-108`：本次未连接、未备份、未部署、未升级、未核对。
- 生产已执行写操作：0。

## 回滚

- 源码/tag：稳定回滚点 `v1.20.0` / `c98727c...`；上一预览 `v1.21.0-rc.5` / `573b17da...`。
- 镜像 digest：稳定 `sha256:a991b5d2...`；上一预览 `sha256:5ee9bb3c...`。
- 数据/配置备份及生产回滚：不适用，未部署生产。
- 预览回退时重新固定 rc.5 digest；GitHub Latest、Docker `latest` 和稳定更新入口继续指向 `v1.20.0`。
- 公共默认更新通道决策：不适用，稳定通道未改变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-21T00:27:09+08:00
- 候选冻结时间：2026-09-21T00:49:39+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "main-ci/race-test/receipt-loss-timing",
    "position": "before-production-write",
    "count": 1,
    "impact": "main CI 首次 race 中 MCP 回执丢失测试在 0.34 秒检查时仍为 executing；同一 SHA 的候选 CI 和固定 L3 已通过，Release 尚未开始。",
    "recoveryEvidence": "保留首次 job 日志后只重跑失败作业；run 35524278914 attempt 2 在同一 d73e9c78 SHA 上完整成功，随后 Release 成功。",
    "permanentAction": "下一稳定版前复核该测试的状态等待条件；若未来两个候选再次复现，改为有界轮询终态并保留不重复执行断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-observation/github-api/anonymous-rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "高频只读状态查询达到匿名 GitHub API 限额，一次 jobs 查询未返回；没有改变运行、标签或镜像。",
    "recoveryEvidence": "切换到本机 Git 凭据的认证 API 后读取同一 run attempt 与 job 终态，后续发布状态和附件复核完整。",
    "permanentAction": "发布观察从首个查询开始使用认证 API，并保持 55 秒退避，避免消耗匿名共享限额。",
    "historicalReleases": []
  },
  {
    "fingerprint": "worktree-cleanup/windows/ignored-cache-residual",
    "position": "before-production-write",
    "count": 1,
    "impact": "来源 worktree 因 web 忽略缓存导致 Windows 物理目录删除未完成；Git worktree 注册和本地活动分支已安全释放。",
    "recoveryEvidence": "远端 archive ref 已先核对，git worktree list 不再包含来源路径，本地来源活动分支已删除。",
    "permanentAction": "来源任务在交付前先清理 node_modules 与 .vite，再由发布流程解除 worktree 注册。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：来源 Git worktree 注册和活动分支已回收；路径只剩可再生成的 web 缓存。L3 和公开 OCI 证据保留在 `C:/GitHub/_release-evidence/` 与 `arena-154:/root/kpanel-release-evidence/`。
- 未验证风险：最终人工多浏览器缩放矩阵、长期设置页交互 soak，以及 MCP 回执丢失 race 测试的偶发调度敏感性。
- 已实现待实机准入：稳定版前人工抽查版本号跳转在主流浏览器和 100%/125%/200% 缩放下的首帧与焦点。
- 不阻断本版的理由：本版只进入预览渠道；同一源码已通过固定 L3、候选/main/tag 门禁、Release 安全链、14 个附件和公开 OCI E2E，首次 main race 失败未逃逸且复跑成功，稳定入口与生产未改变。
- 后续应进入的自动门禁或专项工作流：增强回执丢失测试的有界终态等待，并把版本跳转首帧加入浏览器验收矩阵。
