# KPanel v1.29.0-rc.6 发布验收记录

日期：2026-11-02；发布级别：L3；releaseChannel=preview；releaseTrain=1.29.0。

候选提交 / 标签：dca6190fd1dc504719800396f2e24c0659e44226 / v1.29.0-rc.6。[公开预览 Release](https://github.com/kejilion/KPanel/releases/tag/v1.29.0-rc.6)，Release ID 431263032，draft=false、prerelease=true、Latest=false。产物已发布，生产未部署。

上一稳定版本 / 回滚点：v1.28.0 / sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4。发布前 preview 通道为 v1.29.0-rc.5 / sha256:14e6020e825169d1ce479ce6b155b8252dc5621f29e689643559ecad9e8facd9；RC6 不覆盖此前不可变镜像。

候选分支与发布后处置：release/v1.29.0-candidate 保留于本 1.29.0 RC 列车，精确 tip 为 dca6190fd1dc504719800396f2e24c0659e44226；预览版单次发布不归档候选分支，待该列车稳定发布时按 project-management.md 10.2 收尾。

- 纳入的原分支 / 精确 tip / 处置：
  - fix/app-domain-progress-layout / a5259d8c3033b88f01c8d4b79252e692a1a2abef / 本地归档为 archive/fix/app-domain-progress-layout。
  - fix/environment-job-dismissal / a7a6a6e4f8a2f7922b85a5a898f8185f0859be14 / 本地归档为 archive/fix/environment-job-dismissal。
  - fix/terminal-resize-silent / c2e622f043938e86702de25e113be015e995a003 / 本地归档为 archive/fix/terminal-resize-silent。
- 远端核验：origin 上没有上述三个来源分支或其 archive 对应引用；来源分支本来仅存在本地，没有远端活动引用可删。共享 Git 仓库中的 archive refs 均保存原 SHA。三个原 worktree 保留且 clean，路径分别为 C:\GitHub\_codex-tasks\kejilion-panel-codex-app-domain-progress、C:\GitHub\_codex-tasks\kejilion-panel-codex-environment-job-dismissal、C:\GitHub\_codex-tasks\kejilion-panel-codex-terminal-resize-silent。
- 发布工作树 C:\GitHub\_release\kpanel-v129 保留在候选分支，clean；没有删除工作树或清理共享依赖。
- 未完成归档项：无远端来源引用待处理。release/v1.29.0-candidate 按预览列车规则保留至稳定版发布。
- archive/host-script-latest-20261102 按用户在另一会话明确暂停的决定未纳入本版。

## 发布画像与范围

- 业务域：环境任务结果提示、应用市场域名绑定进度、交互终端尺寸同步提示。
- 变更面：Web 前端行为与展示、版本元数据和来源扫描身份；无宿主机写入或数据迁移。
- 受影响用户旅程：重开环境页时只恢复当前关联任务；完成结果可关闭且仅提示一次；域名绑定运行中和失败态保留阶段说明、成功态节省空间；终端 resize 瞬时失败仍重试但不向命令输出插入重复黄色提示。
- 未变化契约：API、数据 Schema、端口、Compose、Agent 权限、kejilion.sh、应用市场外部数据均未改变。go.mod/go.sum 未改变；package-lock 仅更新项目版本字段。
- 风险等级：L3 发布级流程；产品改动主要为前端交互，仍按预览版执行完整 CI、L3、镜像和公开验收。

精确候选提交清单（50d2059b5e9b707d6c596e07701a3e2336792c31..dca6190fd1dc504719800396f2e24c0659e44226）：

- 41ecd01a67d5a69bff11de66f4021bfbc229101b：应用市场域名绑定进度版式。
- d6aac40e67a09426fd59590178c867726a91c718：环境任务结果提示关闭与记忆。
- 7cb2fa70746b897e87495006407e7afabe869162：环境任务恢复状态与应用市场对齐。
- cdf166fe8e74872252112ae1b56e8bd364808c22：终端 resize 短暂失败不再写重复提示。
- 80ff1b4f0c4c60ed68185cafb851afd3595d0c84：准备 v1.29.0-rc.6 版本元数据与 changelog。
- aa966c18408f4aa7a057e59d30e41e5e925c9bf7：候选 OCR 覆盖记录。
- ea681a11aa9842de4da8c6a2fbe9facfe6a95611：集成复核回执。
- ef94ad0769773a19ee99f3c743b6b7cbc0f4dd79：按源码重算并绑定 first-party source identity。
- dca6190fd1dc504719800396f2e24c0659e44226：修正后最终候选 OCR 复核回执。

## 外部审计与修复交付

- OCR 最终范围从 50d2059b 到 ef94ad07；12 个可审文件已读完，另 2 个排除项按规则记录；H0/M0/L0，constrained-only 为 unreported。最终候选审计回执为 PASS。
- 独立审查 CLI 中 Claude/Gemini 等替代 provider 不可用；保留了 Codex 作者/复核同 provider 的 fallback 收据，结论 PASS WITH FOLLOW-UP，不将其描述为独立 provider 复核。
- node scripts/security-review-scope.mjs --base v1.28.0 输出 recorded=4：cmd/kejilion-agent、internal/agent、internal/ai、internal/appmarket；invalid_trailer=true，历史无 scope 的 trailer 警告保留。该输出不是本版安全审计通过证明。
- 本版未改 Cloudflare 相关边界代码，未执行 CF security-boundary-audit；候选 CI、L3 和发布工作流实际执行的依赖、源码、漏洞路径与镜像扫描分别记录。
- scriptLinkageState=not-required。未修改 kejilion/sh 或 kejilion/apps 仓库，没有发布配套脚本或改变外部应用市场契约。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 候选 CI 和 L3 Web 套件通过；公开浏览器验收覆盖环境任务、终端恢复、域名绑定发布信息和更新提示旅程。 | 未在生产账号执行；环境任务与域名绑定仍以候选 CI 和任务验收为主要证据。 |
| 网络入侵与供应链安全 | 已验证 | 候选、主线、L3、发布安全扫描通过；公开镜像、附件校验清单与 modified-components provenance 均读回成功。 | 不宣称零漏洞或全量独立安全审计；security-review-scope 的 invalid_trailer=true 作为历史警告保留。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 Web/Go/deploy lanes 通过；公开隔离容器的首次启动与重启检查通过。 | 物理 ARM64 主机、跨发行版和生产服务未实测。 |
| 性能与资源预算 | 已实现未实机验证 | 版式在 1280、390、320 CSS 像素宽度下无横向溢出；检查结果见公开验收附件。 | 未执行长时间 soak 或宿主压力基准，不宣称性能提升。 |
| 用户体验与可访问性 | 已验证 | 公布镜像真实浏览器路径、升级提示、键盘命令与终端恢复检查通过。 | 未覆盖完整主题、语言、200% 缩放矩阵。 |
| 数据、配置与迁移 | 已验证 | API/数据 Schema 不变；隔离容器状态和终端历史在 Panel 重启后读回成功。 | 没有生产数据或配置迁移。 |

## 自动门禁

- 定向测试：候选 CI 41412645419 的 Linux verify 与 Windows node 同一候选 SHA 成功；发布 run 中再次执行候选 CI、主线 Linux/Windows CI 均成功。
- 本机 make verify-release：本机 MINGW64 缺少 Go、gofmt、make，无法运行本地完整入口；未将本机状态冒充通过。相同冻结 SHA 的 GitHub-hosted candidate CI 与 L3 均通过。
- L3 外层入口：[run 41413375831 attempt 1](https://github.com/kejilion/KPanel/actions/runs/41413375831)，候选 SHA dca6190fd1dc504719800396f2e24c0659e44226；GitHub-hosted Ubuntu 24 image 20261026.354.1，Go 1.27.3、Node v24.22.1、Docker 28.0.4；2026-11-02T08:29:48.376Z 至 08:48:30.437Z。入口 SHA-256 181efee78ef2f8c197bf34c90a074550286acebc7a10630635386494b57c2fa3。source lanes web/go/deploy 全部 passed，identityUnchanged=true。
- 候选 CI、主线 CI、Release workflow、public-acceptance 和 preview-completion 均在 run 41413375831 attempt 1 成功；stable-completion 按 preview 通道跳过。
- 最终发布说明目标版本 1.29.0-rc.6，3 条修复摘要、1 条升级提示；公开正文和浏览器发布接口读回一致。
- 安全扫描、Go call-path 扫描、Node dependency audit、源码/密钥/配置扫描、kejilion.sh 生命周期、构建二进制与运行时镜像契约步骤均成功；未据此声称零安全发现。

## 依赖与技术栈变化

- go.mod、go.sum、npm 依赖树未变；web/package-lock.json 仅把根项目版本由 1.29.0-rc.5 更新为 1.29.0-rc.6。
- 本轮未重新生成 make dependency-report 或每日安全通告/EOL 报告；不宣称本版重新完成了这些独立周期报告。候选与发布 CI 的依赖和源码审计步骤成功。
- Runner 使用 Go 1.27.3、Node 24.22.1、Docker 28.0.4；没有升级工具链、基础镜像或 Actions。

## 隔离真机与浏览器验收

- 环境：GitHub-hosted Linux 隔离容器和真实浏览器验收；产品镜像固定为 sha256:bcb3fb8470324ab4514e768b2bd3a15039310509fd53a5c2f3c9452d295b8d90。
- 真实 Panel/Agent 初始与重启验收分别 48、26 项通过；published 浏览器 45 项、upgrade 浏览器 31 项通过，四份报告均 mocked=false、errors=0、assetFailures=0。
- published 与 upgrade 报告绑定预览 digest；检查发布版本/摘要/正文、自动安装关闭、主动选择与失败恢复、离开预览不降级、终端历史和真实 PTY 生命周期。页面 errors=0。
- 测试使用隔离容器中的 Docker inventory 与测试 CA fixture；不连接生产宿主、不等价于物理 ARM64 或生产账号。
- soak：不适用；本版为预览发布，没有生产暴露或持续压力测试任务。

## 发布产物与公开仓库复核

- [公开 GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.29.0-rc.6)：draft=false、prerelease=true、不是 Latest；19 个附件已公开，包含 SHA256SUMS、LICENSE、安装脚本、Agent/Node/MCP 二进制、元数据归档和 KPANEL_MODIFIED_COMPONENTS.json。
- Docker Hub preview OCI index：sha256:bcb3fb8470324ab4514e768b2bd3a15039310509fd53a5c2f3c9452d295b8d90。
- linux/amd64：sha256:1c9e00b0950fbd37d6596cf69be4461e4028a2a439fcd730aa5872a905e2503c。
- linux/arm64：sha256:3d32de929c7dd7e48c062d214c7160392b293f4e8111e91f503d7101db1ce494。
- Docker latest 和版本 1.28.0 均为 sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4；GitHub Latest 仍是 v1.28.0。
- 公共浏览器读取 releaseChannel=preview 的版本、镜像 digest、3 条摘要、1 条升级提示均匹配；隔离真实镜像 e2e=pass。预览 digest 未与生产稳定通道混淆。

## 自更新通道验收

- 稳定通道仍返回 v1.28.0；预览通道返回 v1.29.0-rc.6 和唯一官方 digest。
- 加入预览仅切换来源并检查；不会自动安装；自动安装开关关闭且未产生一次性安装请求。
- 退出预览不会产生降级候选；升级对话框显示准确版本、摘要与升级注意事项。
- 公开真实浏览器的 published 与 upgrade 用例均通过；OpenRC/无 systemd 更新失败时保持 fail-closed。完整生产宿主更新、备份恢复和系统重启不在本次预览验证范围。

## 生产部署安全核对

- 生产目标与部署授权：不适用；preview 预览版禁止生产部署。
- 验证/灰度目标与正式部署环境：不适用；本轮仅使用已隔离的 GitHub-hosted 测试环境。
- prod-108 未连接、未备份、未部署、未升级、未核对；kpanel-eu/158.179.20.115 未连接。
- 没有生产写操作；公开产物不代表生产部署。

## 回滚

- 源码/tag：保留 v1.29.0-rc.6 -> dca6190fd1dc504719800396f2e24c0659e44226。
- 稳定回滚镜像：v1.28.0 / sha256:bd90d2bf69f059b4da29e456054bfb1785b47a6b1d453e5941e299a4a7de78c4。
- 发布前上一预览镜像：v1.29.0-rc.5 / sha256:14e6020e825169d1ce479ce6b155b8252dc5621f29e689643559ecad9e8facd9。
- 未执行生产回滚或创建生产数据备份；退出预览不会自动降级。公共默认更新通道继续使用稳定版。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-02T10:33:54+08:00
- 候选冻结时间：2026-11-02T16:29:05+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

四次异常均在任何生产写操作前被拦截或恢复；无产品变更失败、生产回滚或生产重复发布。候选 source identity 与 dispatch 输入问题均保留原失败日志，归档命令解析失败未改动分支，随后用逐项预检完成本地归档。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/source-inventory/stale-first-party-hash",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮候选 CI 41411982833 发现 firstPartySourceSha256 与 internal/version/version.go 变更后的真实源码摘要不同，verify 未通过。",
    "recoveryEvidence": "C:/GitHub/_release-evidence/v129-rc6-public/process/41411982833.log 保存首轮失败；派生摘要由 1f1a8b4a581b70e655d6778f35750b8d589b34700272df69dd0778554f39f542 重算为 6065b69cc80aca77603aae0da53e1ea79d51751f0d705803dd75d103a7e40b16，ef94ad07 更新绑定，冻结候选 41412645419 和 41413375831 均通过。",
    "permanentAction": "此精确指纹也出现在 v1.28.0。责任人：KPanel release maintainer；期限：2026-11-09 且早于 v1.29.0 稳定版 L3 生产写入。退出条件：source-scope SHA 从精确源码自动派生或由提交前 fail-closed 预检验证，并有回归及同 SHA candidate CI；未满足则稳定版 L3 不得开始。",
    "historicalReleases": ["v1.28.0"]
  },
  {
    "fingerprint": "preview-dispatch/candidate-input/frozen-sha-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "preview-release run 41413315284 的 candidate 输入仍为旧 SHA，与远端 release/v1.29.0-candidate 当前 tip 不一致；inputs job 按设计拒绝，后续作业均 skipped，未写 main、tag 或 registry。",
    "recoveryEvidence": "失败原件保存在 C:/GitHub/_release-evidence/v129-rc6-public/process/41413315284.log；改用冻结 tip dca6190fd1dc504719800396f2e24c0659e44226 后，run 41413375831 attempt 1 全流程成功。",
    "permanentAction": "保留输入层精确 SHA 与远端候选 ref 的 fail-closed 比较；后续 dispatch 只从刚核对的远端候选 tip 复制完整 SHA，不能复用旧候选值。",
    "historicalReleases": []
  },
  {
    "fingerprint": "archive/source-branch/powershell-pipeline-parse",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地归档分支脚本在执行前因将 PowerShell foreach 语句直接接管道而 ParserError（An empty pipe element is not allowed），未修改任何 ref。",
    "recoveryEvidence": "修正为先收集结果再输出；三个 source worktree 均逐项核对分支、HEAD、clean 状态与目标 archive ref，归档后的 full SHA 与原 tip 完全一致。恢复收据：C:/GitHub/_release-evidence/v129-rc6-public/process/postflight-recovery.json。",
    "permanentAction": "PowerShell 多分支处理先写入独立结果集合，再执行管道格式化；每次本地 branch rename 前核验 worktree path、branch、SHA、clean 状态和目标 ref 未占用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/metrics/windows-path-invalid-json",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次验收指标校验因流程异常 JSON 中 Windows 反斜杠路径未转义而拒绝记录；未提交验收文件。",
    "recoveryEvidence": "node scripts/report-release-metrics.mjs --validate-acceptance 拒绝该草稿；恢复时将 JSON 路径统一为正斜杠并重新执行同一校验；过程收据：C:/GitHub/_release-evidence/v129-rc6-public/process/postflight-recovery.json。",
    "permanentAction": "验收 JSON 内使用正斜杠路径；提交前必须以仓库 validator 校验，不在 JSON 字符串中直接写 Windows 反斜杠路径。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 待修复的 source identity 生成入口为稳定版 1.29.0 L3 的明确阻断项；负责人、期限和退出条件见流程异常条目。
- 替代 provider CLI 本轮不可用，复核使用有记录的同 provider fallback；不宣称外部 provider 独立盲审。
- 未覆盖的生产宿主安装/升级、物理 ARM64、全主题语言与 200% 缩放、长时压力及完整数据备份恢复均须按稳定版准入计划另验。
- 本地资源回收未执行；三个归档来源 worktree、发布工作树、候选分支和外部失败/公开验收证据均保留；不报告净释放字节。
