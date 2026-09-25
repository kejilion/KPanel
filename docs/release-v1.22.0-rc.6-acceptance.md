# KPanel v1.22.0-rc.6 发布验收记录

日期：2026-09-25。发布级别：L3；发布通道为预览。生产未部署。

- 候选提交 / 标签：245caaa9e85d9e2432b4c7743aa97212d1f5d044 / v1.22.0-rc.6。
- annotated tag object：8f4d034a4adc2832b34a6fc8e3df7fbf2397566e；peeled commit 与候选一致。
- 发布基线：main 245caaa9e85d9e2432b4c7743aa97212d1f5d044；L3 业务基线 v1.21.0 / b7a773526b9c11f64d4dfbe125c68e91fa45cc71。
- 上一稳定版 / 回滚点：v1.21.0 / 396fcd62c5635c9812ee97ee509cb61b962724f3；Docker index sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。
- 上一预览版 / 回滚点：v1.22.0-rc.5 / aea1a75bc00c32172cb9c52b4a65805f86398f52；Docker index sha256:7fd3ff5f9e9cc0d14df2313520f5d16a150bc744c7076b4efc313efcde9359c5。
- releaseChannel：preview；releaseTrain：1.22.0。
- 本地组装分支：release/v1.22.0-rc.6-assembly；工作树 C:/GitHub/_codex-tasks/kpanel-v122-rc6。
- 候选分支与发布后处置：预览版候选保留，不归档；远端 release/v1.22.0-candidate 精确指向 245caaa9e85d9e2432b4c7743aa97212d1f5d044。
- 本次修复来源：fix/rc5-scene-playback，精确 tip 4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c，已纳入 RC6；本地来源工作树保留，未回收。
- 三个场景包的既有来源引用和工作树依 RC5 验收记录保留；本次不清理或重新归属它们。
- 验收记录分支：docs/release-v1.22.0-rc.6-acceptance；按同 SHA 候选 CI、主线快进、主线 CI 后归档。归档 ref 和最终 SHA 在本文后续更新，旧产品 tag 不改写。
- 场景范围在初次整理时曾把两个独立分支误算入候选；用户纠正后冻结为原来的三个场景。最终构建与公开目录只包含下列三个 ID，没有第五个场景。

## 发布画像与变更范围

业务域为桌面动态壁纸的 3D 场景下载与播放。RC5 中用户报告“海天日月”无法播放，而另外两个场景正常。场景文件请求会并发到达，场景下载流上限为 4；超过上限的合法请求此前会直接得到 409。RC6 将进入中的请求放入有界等待队列，并在请求取消时释放队列位置；队列满时仍按边界返回 busy。该修复解决的是并发请求被拒，不是资源包过大。

本次实际场景目录只有三个包，共 69 个文件、15,802,058 字节（约 15.1 MiB）：

| 场景 | ID | 文件数 | 包大小 |
| --- | --- | ---: | ---: |
| 霓虹都市 | neon-city | 8 | 1,150,940 字节 |
| 星港轨道 | orbital-station | 34 | 4,086,696 字节 |
| 海天日月 | sea-and-sky | 27 | 10,564,422 字节 |

公开 GitHub main catalog 的 69 项资源均已逐文件核对 SHA-256 和字节数，与冻结候选一致；证据为 C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/public-verification/public-scene-verification.json。场景包在源码与 dist 间可重复生成，定向场景测试 9/9 通过。

用户旅程为选择场景、下载、校验、安装、应用到桌面，以及并发资源文件请求等待、取消和恢复。未改变既有 API/数据格式、端口、Compose、Agent 权限或安装契约；packaging/kejilion-app/kpanel.conf 未修改。

明确不在本次范围：除 neon-city、orbital-station、sea-and-sky 外的独立场景、稳定版发布、生产部署和主通道更新。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 请求排队、请求取消和队列容量测试；9/9 场景定向测试 | RC6 在用户实际面板上的海天日月播放仍需更新后确认；未完成真实后端浏览器端到端 |
| 网络入侵与供应链安全 | 已实现未实机验证 | CF scoped run-9；L3、Trivy、npm audit、govulncheck 结果见下节 | terminal-sse-output-after-session-revocation 为 needs_validation；浏览器沙箱运行边界及 artwork 掉电耐久性未动态验证 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 有界队列、取消清理测试；Race 检查及全量 L3 通过 | 未在用户真实宿主机做长时间运行和资源观察 |
| 性能与资源预算 | 已实现未实机验证 | 每次最多 4 个响应体并发，等待队列有界；场景包总量和单包大小明确 | 实际网络时延、浏览器 GPU/显存和弱网行为未在用户设备测量 |
| 用户体验与可访问性 | 已实现未实机验证 | 目录和公开场景文件均只有三个 ID；RC5 Mock 浏览器证据沿用 | RC6 真实后端播放、各场景镜头、缩放、reduced-motion、遮挡暂停尚未完整验收 |
| 数据、配置与迁移 | 已验证 | 不迁移既有业务数据；场景缓存可重新下载；无 schema 变化 | 没有生产回滚或真实宿主机恢复演练 |

## 安全审计与修复

- CF scoped audit：run-9，源提交 4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c，tree 444f26132591951256e18b471e1473b430f2dbfb；13 个代码路径、9 个提交的声明范围。源工作树开始和结束均 clean。
- 审计结果：0 confirmed，1 needs_validation，1 rejected；coverage ledger 有 16 个单元（13 covered、1 candidate、2 blocked）。redirect 旧主张因当前 Scene Store 在 follow-up 请求前拒绝重定向而被驳回。两个 blocked 单元为浏览器 opaque-origin/navigation 动态执行和可选 artwork 崩溃耐久性。
- SSE 注销时序需产品负责人先明确 logout 截止承诺，之后在符合隔离要求的环境动态验证。本轮审计没有执行目标代码、测试、浏览器或网络行为；WSL 仅运行受信结构校验，结果为 16 coverage units valid、2 findings valid。
- run-9 的协调者及 15 个子代理均显式配置 gpt-6-luna / max。记录位于 .governance/security-audit/run-9/ 和 C:/GitHub/_codex-evidence/kpanel-scene-queue-security-audit-run-9。
- check-security-audit-coverage.mjs 在候选 SHA 返回 decision=ok，未审计提交数为 0；这是覆盖状态检查，不代表全仓动态安全验收。
- 场景并发修复提交：4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c。独立回归包含排队请求恢复、队列满边界和取消清理；完整 L3 的 Go / race 检查通过。

## 自动门禁与依赖

- L3 唯一外层入口：scripts/run-release-l3.mjs；run ID v1.22.0-rc.6-245caaa9-l3-r2；2026-09-25T09:21:19Z 至 09:28:51Z；status=passed，exit 0。
- 固定 Runner：kpanel-release-gate:go1.26.7-node24；不可变 ID sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d。
- 证据目录：C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/l3-r2。候选 SHA 与 Runner ID 精确匹配。bundle SHA-256 af16b270cf35eb4d6249507953cedd6aac654ed8dcb2f499efb1b82101b96264；plan SHA-256 4d959d8b12c986813e98656e1d38718b723eadc15c2887a8b96ef37f9dacb53d；执行脚本 SHA-256 21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979。
- L3 全量 Go、188 个前端测试文件 / 1,679 个测试、三包源码与 dist 复现、核心 race/vet、Linux amd64/arm64 构建、app_conf_lifecycle 均通过。npm audit 为 0；govulncheck 为 0 reachable / 0 imported-package 漏洞，另有 1 项未调用的 required-module 漏洞；Trivy source/image 门禁通过。Node 构建仅有 glob@10.5.0 deprecated 警告。
- 候选 CI：[36118723316](https://github.com/kejilion/KPanel/actions/runs/36118723316)，success；Dependency freshness：[36118723457](https://github.com/kejilion/KPanel/actions/runs/36118723457)，success。
- 主线 CI：[36119651224](https://github.com/kejilion/KPanel/actions/runs/36119651224)，success；Dependency freshness：[36119651210](https://github.com/kejilion/KPanel/actions/runs/36119651210)，success。上述工作流均针对精确提交 245caaa9e85d9e2432b4c7743aa97212d1f5d044。
- Release workflow：[36120566814](https://github.com/kejilion/KPanel/actions/runs/36120566814)，success。
- 依赖新鲜度采用同日 RC5 完整报告：2026-09-25T02:02:26.128Z、检测源 10/10、失败源 0、SHA-256 C81DBD1A7E23A91CA6341D5191E67845BEE75188C1A97F9E6666A77174FE4C47。RC6 对比范围未改依赖声明、版本、Action pin、基础镜像或脚本 pin。RC6 四次重跑尝试分别遇到 Windows 检测源不完整、Runner 未挂载 Git 元数据、工作目录错误、Go 依赖查询超过 4 分钟；均未冒充为完整报告，故复用当日 10/10 报告。原始证据在 C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/dependency-evidence.md。
- scriptLinkageState：not-required。无 KPanel 与 kejilion.sh 的联动提交；面板应用配置契约未变化。

## 隔离真机与浏览器验收

本次未执行符合治理要求的真实 Panel 后端浏览器验收。公开镜像的端到端脚本 packaging/tests/image-e2e.sh 也未运行：WSL Docker daemon 在拉取公开 RC6 镜像时连接 Docker Hub registry 超时并尝试直连 IPv6。日志为 C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/public-verification/public-image-pull.log。没有更改或重启共享 Docker daemon。不得把公开 OCI digest、L3 本地构建镜像契约或 Mock 浏览器检查写成公开镜像 image_e2e=pass。

因此，源码修复和发布门禁已通过，但用户机器上的 RC6 播放结果尚未验证。安装 RC6 预览版后若海天日月仍无法播放，应记录浏览器控制台和 Network 中失败资源的状态码，以区分旧版未更新、网络失败和新的运行时问题。

## 发布产物与公开仓库复核

- GitHub Release：[v1.22.0-rc.6](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.6)，draft=false，prerelease=true，公开时间 2026-09-25T10:01:17Z。GitHub Latest 仍为 v1.21.0。
- Docker Hub：docker.io/kjlion/kejilion-panel:1.22.0-rc.6 与 :preview 的 OCI index 均为 sha256:9f69a7752aa108eebfa88e3b3e638758e166eb7856d4f068453bb587ea81702b。
- linux/amd64 digest：sha256:648c8f67e6f87ccde8dcff07d7605741f96a6ccebc25f43f6658fa6841690b07；linux/arm64 digest：sha256:be97ee6e81168fcf86cf65f75a69394d71f60eb23ccbe45403eac093a91cad89。额外的 unknown/unknown 为 attestation，不是缺失架构。
- Docker latest 仍为稳定版 index sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61；稳定通道未变化。
- 14 个公开 Release 附件已下载并逐项匹配 GitHub API 报告的字节数和 SHA-256；SHA256SUMS 覆盖的 11 项全部匹配。证据：C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/public-verification/github-release-assets-verified.json、sha256sums-verified.json。
- 公开场景目录为三个场景、69 个文件、15,802,058 字节；逐文件大小和 SHA-256 均与候选一致。
- 公开镜像的架构 digest 和 attestation 可读，但由于上述 daemon 网络超时，image_e2e=未验证。
- Release 记录及频道复核 JSON 位于 C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/public-verification。

## 自更新、生产状态与回滚

- 预览来源已发布为规范 RC；加入预览会切换来源并立即检查，不自动安装。发布本身不代表用户设备已更新。
- 生产部署安全核对：不适用（预览版禁止生产部署）。产物已发布，生产未部署；没有生产写入。
- prod-108：禁用全部 KPanel 操作，本轮未连接、未备份、未部署、未升级、未核对；arena-154 本轮未连接。
- 回滚源码/Tag：v1.22.0-rc.5 / aea1a75bc00c32172cb9c52b4a65805f86398f52。回滚预览镜像 index：sha256:7fd3ff5f9e9cc0d14df2313520f5d16a150bc744c7076b4efc313efcde9359c5。
- 上一稳定回滚镜像：v1.21.0 / sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61。未执行任何生产回滚。
- GitHub Latest、Docker latest 和标准稳定更新入口均保持 v1.21.0；公共默认更新通道决策：不适用（预览版）。

## 候选与来源分支处置

- release/v1.22.0-candidate：远端精确 SHA 245caaa9e85d9e2432b4c7743aa97212d1f5d044；按预览规范保留，不归档。
- release/v1.22.0-rc.6-assembly：本地分支及组装工作树保留于 245caaa9e85d9e2432b4c7743aa97212d1f5d044，恢复 tag 为 v1.22.0-rc.6。
- fix/rc5-scene-playback：本地来源分支 tip 4fb9c16f3b97eea1342d87a48e454e3d0ba37d8c，产品修复已纳入；来源工作树未删除，所有权释放后再独立核验和处置。
- scene pack 源分支及工作树依 RC5 记录保留；本轮只修复请求队列，没有增加或删除既有场景。
- 本验收纯文档分支在精确 SHA 的候选 CI、main 快进和 main CI 成功后，归档至 archive/docs/release-v1.22.0-rc.6-acceptance。验收分支提交 SHA、归档 SHA 和远端核验写入发布证据目录 acceptance-completion.json，不回写已通过 CI 的文档提交。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-25T14:02:12+08:00
- 候选冻结时间：2026-09-25T17:19:34+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

共记录 9 次发布流程异常，均发生在生产写入前；不含正常产品测试发现，也不把未运行的测试记成通过。已逐项核对最近五个正式版本验收记录 v1.21.0 至 v1.17.0，未发现下列相同指纹。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "assembly/scene-scope/initial-miscount",
    "position": "before-production-write",
    "count": 1,
    "impact": "初次整理时把两个独立分支误算入候选；用户纠正后将范围冻结为原三个场景，未发布范围外资源。",
    "recoveryEvidence": "RC6 精确候选与 GitHub main catalog 均只有 neon-city、orbital-station、sea-and-sky；公开69项资源逐字节复核通过。",
    "permanentAction": "候选冻结前按用户确认的 scene ID 对照目录、manifest 和公开 payload；任何 pack_count 不为3即停止发布。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/report-windows/incomplete-sources",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 主机报告仅5/10检测源完整；缺少Go工具链且多个网络源失败，不能作为完整报告。",
    "recoveryEvidence": "RC6 dependency-evidence.md 保留原失败；采用同日 10/10 完整报告并核对依赖输入未变化。",
    "permanentAction": "在 Go 可用且网络源可达的登记Runner上运行报告；所有10个源成功前不得将报告标记完整。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/runner/missing-git-metadata",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次固定Runner重跑没有挂载linked worktree的.git元数据，报告入口在执行前失败。",
    "recoveryEvidence": "dependency-evidence.md 记录该次启动失败；后续挂载Git元数据后继续验证。",
    "permanentAction": "Runner启动预检必须从/repo成功读取Git根目录和HEAD后才调用依赖报告。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/runner/incorrect-working-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "后续Runner启动停留在/go，无法解析仓库脚本路径，未生成报告。",
    "recoveryEvidence": "dependency-evidence.md 记录错误路径；纠正到/repo后重试。",
    "permanentAction": "依赖报告命令固定绝对仓库工作目录，并在执行前断言脚本路径存在。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/go-list/timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "正确挂载和路径下go list -m -u -json all超过4分钟无输出；停止后未将部分结果作为报告。",
    "recoveryEvidence": "dependency-evidence.md 记录停止条件；使用同日RC5完整报告，RC6依赖输入核对未变化。",
    "permanentAction": "Runner为Go模块查询提供可复用缓存与明确超时；超时只记失败并阻止不完整报告晋升。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/version-check/wrong-shell-runtime",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次将bash脚本交给Node执行，产生SyntaxError而未完成版本校验。",
    "recoveryEvidence": "最终通过node scripts/run-repo-bash.mjs scripts/check-version-consistency.sh得到Version metadata is consistent: 1.22.0-rc.6。",
    "permanentAction": "Shell脚本统一经run-repo-bash.mjs入口调用；发布前记录脚本终态，不直接以Node执行.sh文件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/l3-preflight/non-stable-base-tag",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3首次预检误用前一预览RC作为base tag；参数预检拒绝，未生成L3产物。",
    "recoveryEvidence": "有效L3 run v1.22.0-rc.6-245caaa9-l3-r2 使用稳定基线v1.21.0并完整通过。",
    "permanentAction": "L3发布计划从当前稳定tag读取base并校验为稳定版本后启动Runner。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-verification/docker-pull/daemon-network-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "WSL Docker daemon直连Docker Hub registry的IPv6请求超时，公开镜像image-e2e未能启动。",
    "recoveryEvidence": "public-image-pull.log保留timeout；Docker Hub API确认RC6双架构index和preview tag一致，但不替代E2E。",
    "permanentAction": "后续在登记的隔离Runner配置可验证的registry代理/出口后重跑公开镜像拉取和image-e2e；不在共享Docker daemon上临时改配置。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 用户设备上的RC6实际播放未验证；这是本次海天日月修复的最后一项用户侧确认。
- 后续稳定版前完成真实服务浏览器集成、所有场景镜头、真实125%/200%缩放、reduced-motion、资源更新/remount、隐藏/遮挡暂停、持续GPU资源、敌对iframe导航和SSE撤销时序验证。
- 可选 artwork 崩溃耐久性缺少故障注入；不属于已确认安全漏洞。
- 本地资源回收：本轮未删除文件，手动释放 0 字节。14个Release附件和校验JSON作为公开核验证据保留在 C:/GitHub/_release-evidence/v1.22.0-rc.6-20260925/public-verification；候选、来源和预览工作树保留以支持恢复与原作者后续处置。
