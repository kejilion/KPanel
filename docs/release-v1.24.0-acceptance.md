# KPanel v1.24.0 稳定版发布验收记录

日期：2026-10-04

发布级别：L3

候选提交 / 标签：`ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `v1.24.0`

上一稳定版本 / 回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96` / `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`。

`releaseChannel`：`stable`

`releaseTrain`：`1.24.0`

候选分支与发布后处置：`release/v1.24.0-candidate` 已按同 SHA 归档至 `archive/release/v1.24.0-candidate`，活跃远端引用移除；具体远端回执与本地最终状态见 `final-alignment.json`。正式产物已发布，生产未部署。

- 产品恢复点是 annotated tag 和精确归档。RC11 五个来源 tip 已在 `archive/<sourceBranch>/v1.24.0-rc.11` 保存并复核，原作者活跃工作树和分支保留；详情沿用 RC11 `branch-disposition.json`，本轮不清理他人资源。
- 本次纯验收分支 `docs/release-v1.24.0-acceptance` 在同 SHA 主线 CI 成功后单独归档至 `archive/docs/release-v1.24.0-acceptance` 并回收；实际 SHA 与空间回执见 `acceptance-archive.json` / `acceptance-worktree-cleanup.json`。
- 自有产品候选与只读 CF 工作树仅在 clean、所有权释放、忽略文件检查和精确恢复点复核后回收；本地待处置项与理由以最终回执为准。不得重建活跃候选。

## 发布画像与范围

- 业务域：RC1–RC11 累计已验收产品、图库/经典文件管理/桌面、WebSSH/任务终端、备份/分享/监控、Linux 轻量 Node 与受管更新。
- 用户旅程：图库相册封面/移动、更多菜单可达、筛选随内容滚动/选择操作条；WebSSH 有序确认输入、同主机多标签/序号/上限、空闲保活、刷新释放已挂载会话；受管脚本更新与稳定渠道发现。
- 本次相对 RC11 的产品源基线是 `ed3299a9cf8fad494b09124cadf4fa89dc350fe3`；稳定元数据和审计治理差异独立记录。全量用户更新见 CHANGELOG，不将后续 Windows/Node26/TS7/治理草稿纳入。
- WebSSH 沿用 32 帧×2048 字节窗口、1 MiB 队列、WS/HTTP batch/legacy 回退；不新增权限或配对 scope。崩溃/断网/组件挂载前刷新仍依赖旧超时，bfcache 保留会话；创建未完成时跨主机打开的静默 no-op 是既有后续项。
- 无新增数据库迁移；API、端口、Compose、Agent 权限和更新事务保持既有契约。完整 L3 风险核验不等于生产部署授权。

## 外部审计与修复交付

- CF scoped `run-16`，源码 `b56362f7d7109b19f76d44b1162c0917f04b361a` / tree `1c42757139ce8ea4be31cf07b8c9201837f6937d`，source_dirty=false；124 路径、68 未覆盖提交，新边界 `internal/backupremote`。专用审计及其所有子代理显式 `gpt-6-luna/max`，source-only，无目标代码执行/生产探测。
- 独立 hunter、wave/final critic、Phase3/5 与记录核验按原件和最终账本完成，固定 skill pin `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`，两个固定验证器通过。审计实际结论：0 confirmed；7 needs_validation；source-only，未进行动态漏洞确认。
- 覆盖检查（稳定提交、--require）：`decision=ok`；`security-coverage-after.json` 精确绑定 `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；先前中断/无效输出不计覆盖。发现与复核的区别、待确认部署事实及 safe follow-up 均按最终审计报告，不把 source-only 推断写成实机证明。
- 治理仅公开经审查最小文件，raw scratch/失败原件保留在外层证据。稳定晋级元数据/治理 OCR 标记 skipped；已验收 RC11 产品 OCR 映射原 tree，不冒充新 SHA 盲审，也不因未满三个稳定周期判断工具有效。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集 `light-node-procd-20261002`。兼容脚本已先行公开，再发布 KPanel 正式版；本轮不改写脚本 main 或 pin。
- 内置/候选脚本 `c981fb6c8b481981ac7a006e102e111e435f6d30` / ROOT SHA `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`，CN SHA `5be9c4462664ed12833a9dbeba64c4623f7f00deb3ddd5adb8e1539615ff617a`；公开原字节核对与归一化区域差异分开记录。
- ROOT/CN bash-n、同步/轻量安装 fixture/31 项 updater tests 在固定隔离 fixture 通过。fixture 无 network、只读根与源码、受限 tmpfs/CPU/内存/PID，并非 PID1 systemd 或生产脚本执行。
- 旧稳定脚本 `779192048077c130442a64a126d7c0050776d868` / `33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04`，旧 Node 模板 `4d61f7ef123fe5ecd7419cdaa1c93483bbfda403` 为配对回滚说明，不执行回滚。
- 应用市场契约 SHA `f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，本地/远端/产品和公开镜像核对一致，apps clean、无需提交；默认来源仍 stable/latest。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 新稳定 SHA 全 L3、6 原生浏览器、6 主机样本、RC11/稳定双方向12例与 v1.23/稳定双方向12例 | 隔离 Linux TLS/Noise 配对，不代表 Windows 或生产安装业务 |
| 网络入侵与供应链安全 | 已验证 | run16 固定验证器和独立复核；L3 race/扫描；公开双架构 OCI 与 SBOM/provenance subject | source-only 审计；外部配置未知点按报告，SBOM绑定非完整内容审计 |
| 稳定性、失败恢复与兼容 | 已验证 | 65秒空闲后真实输入，刷新旧会话404、6 PTY故障、两组实际混合版本、脚本失败fixture | 有限隔离窗口；未验证 WAN/CDN/长时 soak |
| 性能与资源预算 | 已验证 | RTT0/150、128KiB粘贴、确认窗口与6主机采样 | 单轮模型和点采样，非公网丢包/资源峰值或历史倍数结论 |
| 用户体验与可访问性 | 已验证 | 16 Mock、4桌面、6原生、2字号组，新 SHA 注册作业终态 | 图库既有12px小按钮与14px操作规范仍有差距；CSS zoom非原生缩放，未验Safari/移动 |
| 数据、配置与迁移 | 已验证 | 无新增迁移；L3更新/备份/恢复；18个非自有容器在已记录比较窗口状态相同 | 不追溯为早期预检或生产数据证据 |

## 自动门禁与技术栈

- 唯一 L3 入口 `scripts/run-release-l3.mjs`，run `v1.24.0-ce27dc51-l3-r1`，候选 `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`，passed/exit0；12份 evidence checksum回收核对。证据 `C:/GitHub/_validation/kpanel-v124-stable-l3-r1/remote-evidence`。
- Runner `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Go1.27.1/Node24.21.0；plan `c0734a95fc67a7deee3c82c088b4abbc0d29ce22bbc7e292532ba5dcba061690`；remoteScript `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；bundle `cb44685f1c4057b8c9c17798abf630780005bbf600dbd6273cce2399d0577ca7`。
- Go/race/vet、前端 typecheck/tests、govulncheck/npm/固定Trivy、双架构构建、镜像受限非root、安装/更新/恢复以本轮 `l3-verify-release.log` 原始输出为准，不复用 RC11 测试总数。
- [candidate CI](https://github.com/kejilion/KPanel/actions/runs/37157438154)；[candidate Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37157438170)；[main CI](https://github.com/kejilion/KPanel/actions/runs/37158049647)；[main Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/37158049646)；[Release](https://github.com/kejilion/KPanel/actions/runs/37158469376)，均绑定精确 `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`。文档 CI 独立留存，不代替产品门禁。
- Go1.27.1、Node24.21.0、TS6.0.3 及既有锁文件/Action固定SHA沿用累计RC基线，本次晋级不升级；候选/主线 Dependency freshness 与 Release检查通过，未虚构额外 dependency-report。Node26/TS7仍待原作者准入。

## 隔离真机与浏览器验收

- `arena-154` Linux amd64/Docker，受限 Runner内真实 Panel/Agent/Node、独立TLS/Noise配对；原生浏览器为本机Chrome/Edge，通过loopback HTTPS Vite fixture代理，忽略隔离自签名证书错误，保留Cookie flags。不是公网TLS、原生systemd/procd、Windows证据。
- Mock16、桌面4、原生6与字号2组均为新稳定SHA注册作业，终态/退出码/硬超时/命令规格/清理证据已核对。Chrome/Edge原生每端6次刷新，65秒空闲后真实输出；图库390窄视口与CSS100/125/200组合由Mock覆盖。
- Mock：worker `34052`，passed/exit0；spec SHA `474d6c052870221c880b0fae3ff3ccfbf365d0d3a7be0f8b2e2d69e29d84efe1`；证据 `C:\GitHub\_release-evidence\v1.24.0\browser-mock-job-r4\state.json`。
- native：worker `24076`，passed/exit0；spec SHA `da815a0d9a7b629a8b83b4e1f1334a18896d9a977f64364f9b2992b403f6e12f`；证据 `C:\GitHub\_release-evidence\v1.24.0\browser-native-job-r3\state.json`。
- desktop：worker `30120`，passed/exit0；spec SHA `53eab7d642b67daaa8b25a7044e6360163d1988276a9e42c37ba4dc713e3a66b`；证据 `C:\GitHub\_release-evidence\v1.24.0\browser-desktop-job\state.json`。
- fonts：worker `3380`，passed/exit0；spec SHA `d8f0f1f38057105802ab901e0d3d32c4cf38af105552d7c2b6ddf1eb89eadf84`；证据 `C:\GitHub\_release-evidence\v1.24.0\browser-fonts-job\state.json`。
- 图库桌面fixture滚动仅5px，不外推长列表；字号抽样图库12px/终端16px。未验证Safari/OS剪贴板、原生browser zoom、完整路由器重启入网、arm64原生、WAN或长期soak。
- 旧稳定fixture先资格化，再以公开v1.23真实Panel/Agent/Node和当前稳定新构建二进制做双向legacy回退12例；与RC11双向12例分开。PTY回收、配对状态与唯一执行marker按原始报告，非仅标签或编译证明。

## 发布产物与自更新通道

- [v1.24.0](https://github.com/kejilion/KPanel/releases/tag/v1.24.0) published `2026-10-03T22:39:47Z`，draft=false、prerelease=false，为GitHub Latest。Docker版本与 `latest` index `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`；`preview` 保持RC11 `sha256:a1f2cabf9164001e37b6cd8d05dd7a3a2dc80ed07c0eee082cda304a7306e060`。
- amd64 `sha256:22188d4a4a7e58aa72da3d63552780f84ae03d7298a694cc0fd131352c13e659`；arm64 `sha256:f0a69d3f33f5e95f5c2b315fc2b15de56310392ff520c51841f5aeaf7a4a1770`。14附件/11唯一SHA256SUMS、实际amd64 Node下载、metadata Git blobs、双架构OCI version/revision/script标签及SPDX/SLSA subject核对。
- 公开amd64不可变镜像 `image_e2e=pass`，内置VERSION/脚本/appconf/图库icon实际bytes核对，临时资源已删除；arm64构建与manifest不等于原生执行。
- stable只选正式Latest，preview来源校验规范版本与官方digest；加入只切换来源/check，自动安装与一次立即安装分开；旧状态默认stable并持久化，退出不自动降级。同版本正式版高于RC；轻量Node无人值守继续stable。
- systemd备份/失败恢复与OpenRC/轻量Node当前边界见本轮L3和 release-channels，不冒充生产管理员实际安装。

## 生产部署安全核对与回滚

- 正式产物发布已授权；生产部署、业务备份、管理员写操作、健康/数据核对未授权，均未验证、未执行，生产写0。隔离验证不代替生产。
- `prod-108`/`108` 禁用全部KPanel操作，本轮未连接、读取、备份、部署、升级或核对。验收只使用登记 `arena-154` 隔离命名空间。
- 回滚点v1.23源码/镜像见首段，对应脚本/模板见联动段。实际生产回滚与停写备份未验证；另行授权后恢复精确镜像、配对数据/配置/脚本备份，不reset脚本main、不改历史tag。
- 公共默认更新通道决策：正式v1.24为Latest/latest；preview保留RC11；本次无失败公开版本/重复发布或生产回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-30T12:51:20Z
- 候选冻结时间：2026-10-03T21:38:16.569+00:00
- 生产完成时间：未验证
- 提交到生产用时：未验证
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：86
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

实际异常按稳定指纹计数，保留失败原文和成功重试；发生在生产写前的执行器/夹具/证据错误不计产品变更失败。滚动5份原始正式验收已复核，历史匹配见各项，名字不同的类似根因不用于否认复发。永久处置仍按真实负责人/复核期限/退出条件，不把本次外层helper恢复宣布为仓库永久修复。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 9,
    "impact": "Skill nested-path guess, nonexistent verify-release.sh operand, two Windows rg wildcard operands and nonexistent .codex directory search did not produce valid read evidence; source files were unchanged. Corrected queries use the verified repository root or literal directory with -g filename filters. A later root-only read repeated the nonexistent .codex directory assumption; rg exited 2 without changing source or audit evidence. A root-only native rg read passed a comma-suffixed first operand through PowerShell. That file read failed, although later commands made the overall shell exit 0; the invalid read is still counted. After successful stable L3, the root read assumed a separate release-gate-runner.log filename; rg exited 1 for that missing operand. The actual L3 log and all 12 authoritative checksums were valid; no source, runtime or audit evidence changed.",
    "recoveryEvidence": "Tool transcript retained; rg --files located root SKILL.md and actual release gate entries; literal directory plus -g filters used for subsequent helper reads. An additional root-only read batch guessed metadata.json and notes.json after compaction and failed before any write (tool chunk c39f40, exit 1). Literal rg --files inventory (chunk 5b6782, exit 0) identified run-metadata.json and coverage-ledger-recovery-notes.json; corrected reads succeeded. The original baseline read in that same failed batch succeeded; no audit coverage or product change is inferred. root-workflow-path-read-error-original.json retains the exact tool call/output. The rules link and literal rg --files inventory located .codex-workflows/release-kpanel.workflow.yaml; subsequent reads used verified paths. root-native-rg-comma-read-error-original.json retains exact tool call/output. Subsequent native commands use separate space-delimited literal operands; no product or audit state was changed by the failed read. root-l3-log-path-read-error-original.json retains the exact root tool call/output. A literal rg --files inventory confirms release_gate_runner=pass is inside l3-verify-release.log. CF remains closed at its original terminal aggregate 80; this later non-CF inspection event brings release aggregate to 81.",
    "permanentAction": "Release owner must resolve exact filenames with rg --files before every read. This run repeated the RC11 inspection pattern; no production is authorized. Before any next production write, qualify a single inventory/read preflight and retain regression evidence. Review date 2026-10-10; closure requires literal-path inventory checks passing without guessed operands.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/collaboration-state/unsupported-output-option",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first writer preflight used unsupported --format=json and stopped before creating the audit source or modifying product files.",
    "recoveryEvidence": "prepare.py corrected to preserve the check's text output; writer-preflight.json and source baseline PASS bind the clean D11 source.",
    "permanentAction": "Release owner will use the actual CLI usage/schema and the corrected prepared entry. Review date 2026-10-10; closure requires a clean source preflight with the supported argument set.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/ssh-runner-identity/powershell-remote-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "Inline Python SSH identity probe lost its quoting and was rejected by the remote shell; the runner probe did not run.",
    "recoveryEvidence": "preflight.py uses structured subprocess arguments and shlex.join; public-preflight.json verifies the exact Runner ID and vacancy.",
    "permanentAction": "Release owner freezes structured argument transport before execution; no inline PowerShell-to-SSH Python strings. Review date 2026-10-10; closure requires the fixed entry to verify Runner identity without fallback.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/external-script-integrity/crlf-normalization",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial root/CN raw replacement equality assertion failed because line endings differed; actual public byte SHA checks had matched.",
    "recoveryEvidence": "script-root-cn-diff.patch contains only the canshu regional difference after explicit CRLF normalization; public-preflight.json retains raw SHA values and normalized comparison.",
    "permanentAction": "Release owner must keep raw digest verification separate from documented normalized regional comparison. Review date 2026-10-10; closure requires both independent raw SHA and regional-only diff checks.",
    "historicalReleases": []
  },
  {
    "fingerprint": "appmarket-fixture/staging/crlf-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "First script smoke stopped at shell set before business tests because the initial Windows Git archive had CRLF working-tree conversions. Immutable Git object bytes are LF; the committed test itself did not have CRLF.",
    "recoveryEvidence": "script-smoke-1.log and initial archive retained. script-archive-integrity-review.json proves every selected archive entry returns to its immutable Git blob after only CRLF normalization. r3 constructs its fixture directly from Git object bytes; three commands and 31 updater tests passed.\nCanonical history alignment: v1.22.0 recorded the same Windows CRLF fixture-staging failure. The established fingerprint is reused; the current operation remains the external paired-script smoke fixture, not an appmarket product change.",
    "permanentAction": "Release owner uses an explicit normalized fixture archive while retaining original Git/public digest proof. Review date 2026-10-10; closure requires the prepared exact-pin fixture to pass all three commands.",
    "historicalReleases": [
      "v1.22.0"
    ]
  },
  {
    "fingerprint": "script-smoke/isolated-fixture/noexec-scratch",
    "position": "before-production-write",
    "count": 1,
    "impact": "Second attempt passed root/CN sync but installer fixture execution was denied by default noexec /tmp; no host service or network was changed.",
    "recoveryEvidence": "script-smoke-r2-2.log retained; r3 runs on prechecked immutable fixture with /tmp tmpfs exec and fixed limits, source/root read-only, network none; all smoke tests passed and container removed.",
    "permanentAction": "Release owner records temporary execution requirements with the command spec and prechecks Bash/Python and mount flags together. Review date 2026-10-10; closure requires exact prepared r3 fixture and cleanup proof, not an inline bypass.",
    "historicalReleases": [
      "v1.23.0"
    ]
  },
  {
    "fingerprint": "release-metrics/external-history-review/windows-esm-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "The initial read-only history reviewer used a Windows absolute path as a static ESM import; Node rejected the c: scheme before reading acceptance records. Product source and releases were unchanged.",
    "recoveryEvidence": "Tool transcript retains ERR_UNSUPPORTED_ESM_URL_SCHEME. review-process-history-failed-r1.mjs retains the failed helper; corrected import uses a valid file URL and reruns the five original acceptance blocks.",
    "permanentAction": "Release owner uses valid file URLs for Windows ESM imports. Review date 2026-10-10; closure requires the corrected history entry to parse all five records and retain canonical root-cause matches before production work.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/hunter-return/report-schema-mismatch",
    "position": "before-production-write",
    "count": 2,
    "impact": "The first hostbackup hunter returned trace and validation_plan fields incompatible with the pinned schema. A filemanager hunter returned forbidden candidate coverage_id and incompatible uncovered fields. Both results were rejected as coverage and their units returned to planned with owner, current evidence and finding fingerprints cleared. Fresh hunters must re-hunt the source; raw leads do not become validated findings.",
    "recoveryEvidence": "Both original responses are retained at security-run-16/agents/hunter_backup_local_w1/scratch/phase2-malformed-response.json and security-run-16/agents/hunter_filemanager_w2/scratch/phase2-malformed-response.json. malformed-result-retention.json independently verifies their existence, byte sizes, SHA256 and JSON syntax while preserving the invalid fields. The fresh hostbackup hunt returned a valid contract; final recovery remains pending fresh filemanager coverage and independent wave/final critics and validators.",
    "permanentAction": "Audit/release owner must qualify the pinned schema contract in hunter dispatch and retain failed attempts. Review date 2026-10-10; closure requires a fresh owner returning an accepted contract, independent wave/final critics, and both final pinned validators passing. This entry does not claim the audit is complete or the process defect is permanently fixed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "host-preflight/external-compatibility-harness/missing-module-manifest",
    "position": "before-production-write",
    "count": 1,
    "impact": "Previous-stable fixture qualification stopped at Go compilation because the adapted host driver folders omitted RC11 go.mod/go.sum, so gorilla/websocket could not be resolved. No business test process was started; the raw failure and cleanup proof are retained.",
    "recoveryEvidence": "old-compat-fixture-evidence-r1/build.log and result.json bind public v1.23.0 binaries and cleaned=true. host-driver-manifest-repair.json maps the four copied manifests to byte-identical verified RC11 originals. Qualification is repeated in a fresh r2 namespace before freezing.",
    "permanentAction": "Release owner must copy and hash the entire explicit driver dependency manifest with source files. Review date 2026-10-10; closure requires cold compilation and real previous-stable legacy cases passing in the prepared r2 fixture and fresh stable/old mixed verification. This does not upgrade product dependencies or claim permanent repository closure.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/nontransactional-update",
    "position": "before-production-write",
    "count": 1,
    "impact": "The audit parent's wave2 append used a PowerShell variable without reloading the on-disk ledger. It replaced the 18-unit canonical ledger with a null entry and five new critic units. The fixed validator rejected the damaged ledger; new hunting stopped. Product source, main/candidate refs and stable tag were unchanged.",
    "recoveryEvidence": "Damaged bytes retained in security-run-16/agents/parent_transcriptions/coverage-ledger-wave2-additions-snapshot.json. The parent's own rollout was identified from session_meta and contains the original 15-unit seed call; wave1/wave2 critics and legal hunter originals are retained separately. Reconstruction and provenance are pending, followed by a fixed validator and fresh independent recovery critic. This record does not claim lossless recovery or completed coverage.",
    "permanentAction": "Audit parent must qualify one transactional ledger writer before any further wave: reload disk on each change, preserve last-good bytes, validate a temporary file using the pinned validator, then atomically replace canonical. Enable strict undefined-variable checks when PowerShell is used. Owner: audit/release owner; review date 2026-10-10; closure requires the recovered 23-unit ledger, independent recovery review and qualified transactional updates plus final validators passing. No production operation is authorized.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/ledger-recovery/empty-scope-units",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first ledger reconstruction produced empty starting_paths for the Obvious things and Wildcard units. The pinned validator rejected that temporary recovery artifact, so reconstruction required another attempt; it did not become accepted coverage or overwrite the retained damaged canonical ledger.",
    "recoveryEvidence": "Failed bytes retained at security-run-16/agents/parent_transcriptions/coverage-ledger-recovery-first-attempt.json; SHA256 b69fca3130e1f79a95a2238f3f8c7850571b8ff444a3f92b37482c23460e6cd7. coverage-ledger-recovery-notes.json records the exact validator failure and the second attempt's explicit original path intersection with authoritative scope. Second recovery passed the fixed structural validator with 23 units, 124 paths and 68 commits; fresh independent semantic review is still pending.",
    "permanentAction": "Audit parent must reconstruct from recorded original definitions and explicitly reconcile every unit's starting_paths with scope authority before promoting a recovered ledger. Owner: audit/release owner; review date 2026-10-10; closure requires nonempty valid units, no missing authoritative paths/commits, retained failed attempts, a qualified transactional writer and fresh recovery/final critics. Structural validation alone does not close audit scope or establish permanent process repair.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/transactional-writer/invalid-backup-argument",
    "position": "before-production-write",
    "count": 1,
    "impact": "The recovery provenance/metadata transaction attempted .NET File.Replace with an empty backup argument. The API rejected the replacement before any canonical metadata/notes change, requiring a retry. Existing bytes, snapshots and validated temporary JSON were retained.",
    "recoveryEvidence": "Audit parent reports the failed parameter call and successful retry with an explicit nonempty backup path after verifying temporary hashes and unchanged targets against last-good snapshots. Original notes/meta byte snapshots and replacement backups remain in security-run-16. Exact failed/successful rollout command/output provenance must be retained and reviewed before closure.",
    "permanentAction": "Audit parent must qualify the actual platform API and explicit backup path in the sole transactional writer, retain last-good bytes and validate/read back each atomic replacement. Owner: audit/release owner; review date 2026-10-10; closure requires a successful documented transaction without an empty backup argument, retained raw failures, fresh recovery/final critics and final validators. This does not claim permanent repository repair or completed audit scope.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/recovery-critic-append-unsorted",
    "position": "before-production-write",
    "count": 1,
    "impact": "The recovery critic append produced an unsorted temporary 25-unit ledger. The pinned validator rejected it before canonical replacement. The damaged canonical and failed temporary were retained; sorted revalidation and atomic promotion subsequently passed.",
    "recoveryEvidence": "audit-recovery-event-retention.json verifies original failed bytes and sidecar; coverage-ledger-recovery-promotion.json records snapshot, fixed validation and canonical read-back for 25 units / 124 paths / 68 commits. This is structural recovery, not completed security coverage.",
    "permanentAction": "Audit owner must use ordinal coverage_id sorting and the qualified disk-read/snapshot/temp-validator/atomic-replace writer for every ledger update. Review date 2026-10-10; closure requires subsequent transactions and final critics/validators passing without reclassifying pending units.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/run-metadata/temp-self-check-invalid-field",
    "position": "before-production-write",
    "count": 1,
    "impact": "The metadata/notes temporary self-check referenced nonexistent agent_configuration.model rather than required_child_settings.model. It failed before either canonical target was replaced. Original targets and both failed temporaries were retained.",
    "recoveryEvidence": "audit-recovery-event-retention.json verifies both original failed temporaries against byte counts and SHA256. recovery-critic-metadata-selfcheck-failure.json retains the exact rollout call/output provenance. Successful actual-structure retry and final validation remain to be checked.",
    "permanentAction": "Audit owner must read the existing on-disk metadata structure and centralize self-checks in the qualified transaction entry. Review date 2026-10-10; closure requires retained failures, successful nested-model check, canonical read-back and final audit validators. No completed audit or permanent repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/run-metadata/writer-reserved-path-preflight",
    "position": "before-production-write",
    "count": 1,
    "impact": "A PowerShell array expression in the new writer included the canonical notes path in its reserved-path set. Preflight refused the write before any snapshot, temporary file or replacement. Product code and canonical audit targets were unchanged.",
    "recoveryEvidence": "metadata-writer-reserved-path-preflight.json retains rollout call/output provenance and unchanged canonical hashes. audit-recovery-event-retention.json mechanically verifies the retained original notes/metadata snapshots. The root atomic-audit-evidence.py passed a scratch-only original-byte backup, replacement and read-back qualification; canonical audit use and final record verification remain pending.",
    "permanentAction": "Audit owner will use the qualified Python evidence writer rather than handwritten PowerShell path arithmetic. Review date 2026-10-10; closure requires canonical transactions, independently checked records and final pinned validators passing. This is an outer evidence-I/O recovery, not a source security judgment.",
    "historicalReleases": []
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/evidence-writer/powershell-invocation-parser",
    "count": 5,
    "impact": "Two old PowerShell invocations were rejected before the Python evidence writer ran: receipt/backup precheck array syntax, then quoted absolute interpreter invocation without the call operator. Three resumed read-only commands, terminal-hash inventory, hunter-contract inspection and metadata-wave query, also failed to parse foreach statements followed by pipelines. These failed attempts changed neither canonical audit records nor product code.",
    "recoveryEvidence": "audit-invocation-failure-originals.json retains exact one-based rollout call/output pairs 4175/4178 and 4217/4220. The fixed complete interpreter path with call operator subsequently executed the qualified evidence helper; original-byte backups and receipts are retained in security-run-16/agents/parent_transcriptions. audit-resume-command-failures-wave4-complete.json retains resumed records exec-22230ff3-c9bb-440f-bdbf-ec48e7d41432 and exec-a2298e6e-e35d-4faa-9280-2004a04af50c; audit-resume-command-failures-final-clean.json retains exec-ceaa9f3f-3ab3-47c7-a5b5-961e23e36e3a. Complete ParserErrors remain available; corrected inspections subsequently passed. Final counters must include all original failures rather than the resumed parent's smaller subset.",
    "permanentAction": "Release/audit owner uses the exact qualified interpreter/helper command with fixed arguments and no auxiliary PowerShell expressions. Review date 2026-10-10; closure requires successful canonical writes, independent final record review and validators; root I/O recovery is not audit completion."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/evidence-writer/windowsapps-python-alias",
    "count": 1,
    "impact": "The interpreter precheck used the WindowsApps python.exe alias, which returned exit 1 with no version output. It could not execute the required evidence writer; no target code or canonical evidence was changed.",
    "recoveryEvidence": "audit-invocation-failure-originals.json preserves the actual call/output matched by call_id for ordinal 4189. The bundled absolute Python executable subsequently passed version qualification and actual canonical atomic writes.",
    "permanentAction": "Release owner supplies the verified bundled interpreter absolute path in delegation and invocation contracts. Review date 2026-10-10; closure requires exact-runtime execution and final provenance review without alias fallback."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/evidence-provenance/powershell-string-length",
    "count": 1,
    "impact": "The required failed-attempt provenance reader passed the tool-call string rather than its length into Math.Min. MethodException prevented a valid first extraction and required a corrected read. Source code and canonical audit records were not changed.",
    "recoveryEvidence": "audit-invocation-failure-originals.json retains the complete reader call/output pair at 4266 and its matching output, including the exact conversion exception. This root reader parses rollout JSON as data and preserves all four original pairs; diagnostic failure remains a process event, not product or source-coverage failure.",
    "permanentAction": "Audit/release owner uses a qualified JSON-data provenance reader with numeric ordinal and call_id matching. Review date 2026-10-10; closure requires independent record verification of raw failures and final reconciled counts, without executing historical commands."
  },
  {
    "fingerprint": "security-audit/hunter-scratch/unavailable-js-base64",
    "position": "before-production-write",
    "count": 2,
    "impact": "The hunter evidence writer referenced btoa in the isolated JS runtime. ReferenceError stopped the attempt before any file or directory write; the legal source hunt remained unaccepted pending original scratch preservation. The AI hunter separately hit the same unavailable btoa primitive before any file write; its actual original call/output is retained in audit-final-blocker-originals.json.",
    "recoveryEvidence": "wave3-scratch-failure-originals.json mechanically matches hunter session_meta and call_id, preserving both exact original calls/outputs and write-attempt-failures.json SHA. The hunter was directed to apply_patch for original JSON; accepted contract, critic review and final record verification remain pending. Failed output is not coverage. AI hunter actual one-based pair 904/906 is preserved in audit-final-blocker-originals.json; count includes both independent attempts.",
    "permanentAction": "Audit owner uses a directly structured file-writing tool for scratch JSON, with no assumed btoa/Buffer globals or shell command-line encoding. Review date 2026-10-10; closure requires exact raw artifact retention, legal-schema acceptance, independent critics and both final validators. No completed source audit or permanent repository repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/hunter-scratch/windows-command-length",
    "position": "before-production-write",
    "count": 1,
    "impact": "A retry transported the full JSON as an overlong PowerShell process argument. Windows rejected CreateProcess with os error206 before the shell started; no scratch or canonical coverage was written.",
    "recoveryEvidence": "wave3-scratch-failure-originals.json mechanically matches hunter session_meta and call_id, preserving both exact original calls/outputs and write-attempt-failures.json SHA. The hunter was directed to apply_patch for original JSON; accepted contract, critic review and final record verification remain pending. Failed output is not coverage.",
    "permanentAction": "Audit owner uses a directly structured file-writing tool for scratch JSON, with no assumed btoa/Buffer globals or shell command-line encoding. Review date 2026-10-10; closure requires exact raw artifact retention, legal-schema acceptance, independent critics and both final validators. No completed source audit or permanent repository repair is claimed.",
    "historicalReleases": []
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/hunter-scratch/unavailable-js-text-encoder",
    "count": 1,
    "impact": "AI hunter initial JSON serialization used unavailable TextEncoder in the isolated JS runtime. ReferenceError stopped the writer before a shell or file mutation; raw source hunt was retained separately.",
    "recoveryEvidence": "audit-final-blocker-originals.json preserves AI hunter call/output pair 898/900. Original legal JSON was later written directly; no source or coverage change occurred in this failed attempt.",
    "permanentAction": "Audit owner uses structured apply_patch for scratch JSON rather than assumed browser encoding globals. Review date 2026-10-10; closure requires raw failure retention, accepted contract and final independent audit checks."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/hunter-contract/failed-self-check",
    "count": 1,
    "impact": "The first AI hunter mandatory contract self-check threw a check-contract exception and required a retry. This failed check did not change product source or canonical coverage; the existing scratch JSON and failed check were retained.",
    "recoveryEvidence": "audit-final-blocker-originals.json preserves pair 921/924 and the exact check expression/output. Parent independently parsed the original nine-unit JSON, then recorded the successful final hunter return. Root does not substitute a semantic contract judgment or final coverage critic.",
    "permanentAction": "Audit owner must qualify the exact pinned return-contract checker and retain failed checks. Review date 2026-10-10; closure requires independently verified raw contract, fresh critic and final validators, without repairing a malformed final result into coverage."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [
      "v1.23.0"
    ],
    "fingerprint": "security-audit/collaboration/agent-thread-limit",
    "count": 5,
    "impact": "Three audit-parent and one release-root attempts to start a mandatory fresh gpt-6-luna/max post-wave critic were rejected by the platform agent thread limit, despite completed hunters and idle concurrency. No fresh wave3/final critic, later hunter waves or candidate Phase3/5 could run. Stable publication is blocked before freeze/tag.\nDispatch of the fourth Phase3 Wave2 verifier was rejected by agent thread limit after three other verifiers had been accepted. The rejected call created no verifier or result. Existing assignment files and metadata prepared before dispatch remain; this is not rollback or completed notification-candidate validation. The original tool output provides no numeric limit or proof whether the limit is cumulative or concurrent.",
    "recoveryEvidence": "audit-final-blocker-originals.json matches all four spawn failures by call_id to original commands/results: parent 5163/5165, 5181/5183, 5252/5254 and root 19002/19004. Existing raw hunts remain partial evidence, not clean coverage.\naudit-resume-phase3-dispatch-limit-original.json retains the exact original spawn call and output, bound by session id and call_id. Audit parent preserves phase3-wave2-thread-limit.json. A later accepted fresh verifier and independent record are required; do not mark this candidate validated on the failed dispatch.\nThe established v1.23.0 platform-dispatch fingerprint is reused for five current original rejections. Historical batch count remains unchanged. Exact quota semantics differ or are unavailable; no single numeric quota or permanent capacity repair is inferred.",
    "permanentAction": "Audit/release owner must secure fresh agent capacity before resuming required critics and independent validation. Review date 2026-10-10; closure requires actual fresh configured agents, all remaining scoped work and both final validators. Existing agents must not be relabeled fresh; no platform-limit waiver or stable release is inferred.\nAudit owner waits for existing verifiers and checks the actual accepted retry before claiming capacity recovery; preserve any repeated failure and never substitute models or old roles. Review date 2026-10-10; closure requires accepted fresh notification verifier, independent candidate result and final counter reconciliation. No permanent platform repair or completed audit is claimed."
  },
  {
    "position": "before-production-write",
    "historicalReleases": [],
    "fingerprint": "security-audit/hunter-dispatch/missing-in-progress-ownership",
    "count": 1,
    "impact": "Before grouped wave3 dispatch, the parent did not persist in_progress and owner state for the 15 assigned ledger rows. The last-good pre-consolidation ledger still held all rows as planned with null owner and empty checks. Results were subsequently consolidated from original legal hunter outputs; no historical in_progress state is fabricated.",
    "recoveryEvidence": "wave3-dispatch-gap-retention.json verifies the original pre-consolidation SHA and all 15 exact IDs against the two raw hunter outputs. The audit parent disclosed the missed required dispatch step and wrote a validated consolidation receipt; fresh critic/independent source validation remains blocked by agent capacity.",
    "permanentAction": "Audit owner must persist and fixed-validate assignment ownership before each future spawn, and verify exact IDs against dispatch contracts. Review date 2026-10-10; closure requires honest provenance, fresh independent critics and final record validation; retrospective consolidation is not evidence of prior ownership persistence."
  },
  {
    "fingerprint": "preflight/public-release-read/github-cli-unavailable-auth",
    "position": "before-production-write",
    "count": 1,
    "impact": "A root read-only batch used gh api and gh release view without an authenticated GitHub CLI session. Both commands refused before any API request or external write; no release evidence resulted from that batch. Git SSH independently verified remote main/candidate alignment and stable tag absence.",
    "recoveryEvidence": "Actual tool chunk 1f04bb retains both authentication-required errors and exit 1. The existing release method uses unauthenticated official public GitHub API reads; a bounded exact-runtime retry succeeded (chunk 1eced0, exit 0), verifying Latest v1.23.0 and public prerelease v1.24.0-rc.11. This is a read recovery, not stable release validation.",
    "permanentAction": "Release owner uses the project-prepared public API reader for unauthenticated public metadata. Review date 2026-10-10; closure requires retained failure and successful bounded public reads, and a separate fresh full public verification after authorized publication. Do not request or expose credentials for these public reads.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/evidence-summary/powershell-pipeline-join",
    "position": "before-production-write",
    "count": 4,
    "impact": "Three resumed audit commands incorrectly passed -join as an argument to ForEach-Object. The initial read-only ledger summary failed before producing valid inspection evidence. Later ledger and metadata dispatch writes passed the qualified atomic writer, but their trailing summaries failed. Product source and production were untouched; the two writes are retained successful operations with failed trailing summaries, not rollback or malformed hunter results. A final read-only report summary placed -join directly after a ForEach-Object pipeline, so PowerShell bound it as RemainingScripts and rejected the expression. The preceding hash, pinned-validator, root Git baseline and Wave4 reads succeeded; no canonical or other file was written by this inspection.",
    "recoveryEvidence": "audit-resume-command-failures.json retains exact commandExecution records exec-ecb3f20b-aec1-4f5a-a8e8-d90c5e7f87a2, exec-2e30b367-d4af-4338-b3e9-c9ef9057f219 and exec-e9163075-2985-4f6a-a679-96c1547f675f with full error outputs. Wave4 dispatch receipts retain before/after bytes and SHA256; ledger post-write SHA 13db3fffdc22e1ab6f5c330b1cb02e925de166726b53f3884a6180e7a6178fc7 passed the fixed 25-unit validator, and metadata post-write SHA 6947213d7f27a00c03d58b5f9e770c8d3c5e5503b2fa43f74414fe2179228471 was read back. These facts establish evidence I/O only; fresh hunting, independent validation and final record review remain pending. audit-rework-terminal-review-pipeline-join-error-original.json retains exact parent wrapper call/output at call_BZIMiaFDnQtdMZNjbLxwbu6K/output2927. The old 79 terminal snapshot remains retained. Root will reconcile the one late event mechanically, then request a single literal Get-Content of the actual capsule without any pipeline, parsing helper or join expression.",
    "permanentAction": "Audit/release owner must use a qualified structured JSON reader for summaries and keep successful write receipts distinct from trailing command failures. Review date 2026-10-10; closure requires original failures retained, actual final process counts reconciled and independent final audit checks. No permanent source repair or completed audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/run-metadata/powershell-object-shape",
    "position": "before-production-write",
    "count": 2,
    "impact": "One metadata update indexed the hunter_completion PSCustomObject as a dictionary. It failed before stage/canonical replacement. This dictionary-index mistake stays distinct from the two absent-property assignments mapped to the established historical fingerprint. A read-only staged metadata summary treated a ConvertFrom-Json PSCustomObject as a dictionary and invoked GetEnumerator, which that object does not provide. This repeats the existing dictionary-versus-PSCustomObject root-cause class; earlier absent-property assignments remain separately classified. The private stage already existed, and no canonical artifact or source was changed by this failed display.",
    "recoveryEvidence": "audit-resume-command-failures-wave4-complete.json retains exact record exec-cbc5b1a0-3444-465d-af50-170a18adab37 and its original TypeError. The pre-canonicalization rollup preserves the earlier combined row and all original raw records remain unchanged. audit-rework-final-summary-object-enumerator-error-original.json preserves the exact parent wrapper call and its visibly truncated output without reconstructing omitted text. The parent also retained final-stage-summary-powershell-inspection-error-original.json with the full visible command and type/method failure, leaving unavailable nested exit/call identifiers null. Subsequent summary inspection uses Python JSON keys or PSObject.Properties; terminal evidence still requires canonical atomic commit and actual disk reconciliation.",
    "permanentAction": "Audit/release owner uses an explicitly qualified JSON-object reader and stage builder with exact field shape before calling the atomic writer. Review date 2026-10-10; closure requires original failure and sidecar retention, exact accepted-agent provenance, reconciled counters and final independent record verification. No permanent repository repair or completed audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/run-metadata/read-before-stage-creation",
    "position": "before-production-write",
    "count": 1,
    "impact": "A hunter-status update read the fresh stage path before creating it. Get-Content rejected the nonexistent path, stopping before stage/canonical metadata writes; the accepted hunter was already active. This is an evidence-precondition failure, not malformed hunter output or a product defect.",
    "recoveryEvidence": "audit-resume-command-failures-wave4.json retains exact record exec-7466eaba-9e4b-4d9f-adff-f759d160d00a and its non-truncated missing-path output. Audit parent retains wave4-hunter-exposure-metadata-stage-error.json and the successful atomic retry receipt; all original failures remain visible.",
    "permanentAction": "Audit/release owner verifies existing input paths and writes a fresh stage from the current canonical JSON before any stage read. Review date 2026-10-10; closure requires retained failure, successful guarded writes and independently reconciled final records. No complete security coverage is inferred.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/evidence-writer/js-template-backticks",
    "position": "before-production-write",
    "count": 4,
    "impact": "Three resumed parent functions.exec scripts failed JavaScript parsing with unexpected identifiers n, json and gpt. The runtime rejected each whole script before any nested tool invocation. Original calls and content-block-array outputs are retained; these are orchestration failures, not malformed hunter results or product findings. A rework-parent invocation repeated this failure before canonical audit replacement; the exact original call/output is retained separately.",
    "recoveryEvidence": "audit-resume-js-template-failure-originals.json retains exact original call/output pairs, verified thread identities and the failed reader snapshot SHA256. Successful later audit dispatch is separate evidence; no final coverage is inferred. Outer process rollup must be reconciled by final audit record verification. audit-rework-tool-error-originals.json retains the new exact failure pair. Fresh validation and final audit completion remain pending.",
    "permanentAction": "Audit and release owner use qualified structured JSON transport and content-block-aware provenance readers. Review date 2026-10-10; closure requires original failures retained, actual counters reconciled and independent final record verification. No permanent repository repair or complete audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/evidence-provenance/output-shape-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The release owner first assumed raw tool outputs were strings. The provenance reader found zero pairs and failed its assertion before writing a retention artifact or changing canonical audit evidence. Original output was an array of content blocks. Failed reader bytes and its exact call/output pair are retained; the corrected reader preserves three original failures without executing historical commands.",
    "recoveryEvidence": "audit-resume-js-template-failure-originals.json retains exact original call/output pairs, verified thread identities and the failed reader snapshot SHA256. Successful later audit dispatch is separate evidence; no final coverage is inferred. Outer process rollup must be reconciled by final audit record verification.",
    "permanentAction": "Audit and release owner use qualified structured JSON transport and content-block-aware provenance readers. Review date 2026-10-10; closure requires original failures retained, actual counters reconciled and independent final record verification. No permanent repository repair or complete audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-prompt/skill-section-boundary",
    "position": "before-production-write",
    "count": 1,
    "impact": "The resumed audit parent assumed the pinned promotion-procedure section was followed by a text code fence. Python substring lookup failed before the prompt-generation loop or any directory/file write. Only existing source-governance data and pinned skill text had been read; no verifier was dispatched and no target code was executed by this attempt.",
    "recoveryEvidence": "audit-resume-phase3-prompt-error.json retains the exact commandExecution record and non-truncated original traceback. The failing line precedes all write calls. Corrected extraction and accepted independent verifier dispatch remain separate evidence; no candidate disposition is inferred from the failed attempt.",
    "permanentAction": "Audit owner extracts the exact pinned section according to its observed Markdown structure and checks all prompt parts before writing or dispatch. Review date 2026-10-10; closure requires retained original failure, independent candidate verification and final counter reconciliation. No permanent repository repair or completed audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/coverage-ledger/top-level-read-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The resumed parent read coverage-ledger.json as a dictionary with units, although the canonical contract is a top-level array. Python TypeError stopped this read-only query before a valid linked-unit summary. No filesystem write, verifier dispatch or target execution occurred in this failed attempt.",
    "recoveryEvidence": "audit-resume-phase3-ledger-query-error.json retains exact commandExecution record exec-e0e80982-b245-468e-a2ce-9bc13466aa83 and non-truncated traceback. The query is read-only. Corrected array iteration and independent candidate disposition remain separate evidence; no coverage is inferred from this failed summary.",
    "permanentAction": "Audit owner reads the already-qualified canonical top-level array and uses the fixed schema for linked-unit queries. Review date 2026-10-10; closure requires original failure retention, verified associations and final counter reconciliation. No completed audit or permanent repository repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-prompt/python-string-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "The Phase3 Wave2 prompt-generation Python program had an unterminated string literal at line16. Python rejected the complete program during parsing, before any statement, directory/file write, metadata replacement or verifier dispatch executed. The three accepted Wave1 needs_validation records remain separate evidence.",
    "recoveryEvidence": "audit-resume-phase3-wave2-string-error.json retains exact commandExecution record exec-3c79337b-a891-491d-9cec-ea532c04a028 and non-truncated original SyntaxError. Later prompt generation and actual verifier acceptance must be checked independently; the failed attempt is not coverage or candidate validation.",
    "permanentAction": "Audit owner uses a directly saved Python source file and checks its AST before data writes; avoid embedding generated quote expressions in shell here-strings. Review date 2026-10-10; closure requires original failure retention, accepted independent verifier assignments and final record/counter reconciliation. No permanent repository fix or complete audit is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-precheck/digest-case-comparison",
    "position": "before-production-write",
    "count": 1,
    "impact": "The Wave1 result precheck compared Python lowercase SHA256 text directly with uppercase verifier-declared SHA256. Its assertion failed before stage or canonical writes. This checks textual casing, not a demonstrated byte mismatch; subsequent lowercase-normalized byte verification is separate retained evidence.",
    "recoveryEvidence": "audit-resume-phase3-digest-case-error.json Original commandExecution exec-b2e71526-ad27-422b-b545-8063e057f0fc and non-truncated traceback are retained. The failing hash assertion precedes all ledger/metadata stage writes. No candidate result is accepted on this failed check.",
    "permanentAction": "Audit owner qualifies the actual JSON/hash and citation-context contract before guarded writes, retaining all failed originals and successful retries distinctly. Review date 2026-10-10; closure requires accepted results, fixed validators and independent final record/counter review. No completed audit or permanent repository repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-precheck/context-path-scope",
    "position": "before-production-write",
    "count": 1,
    "impact": "The corrected-hash Wave1 precheck then assumed every source citation had to belong to the 124 changed-path authority. It rejected the AI verifier citation internal/ai/runtime.go before any stage/canonical write. The audit parent identified this as a scope-versus-context check error; authority remains 124 paths and the exact cited context must be reviewed under pinned rules by the dedicated audit roles.",
    "recoveryEvidence": "audit-resume-phase3-context-scope-error.json Original commandExecution exec-328cfe28-add0-4c45-a4ef-f4353091ca7a and exact non-truncated assertion are retained. Correct source-context admissibility and final local-check ownership require dedicated audit review; root retention makes no source security judgment or expanded coverage claim.",
    "permanentAction": "Audit owner qualifies the actual JSON/hash and citation-context contract before guarded writes, retaining all failed originals and successful retries distinctly. Review date 2026-10-10; closure requires accepted results, fixed validators and independent final record/counter review. No completed audit or permanent repository repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-source-review/wrong-workdir",
    "position": "before-production-write",
    "count": 2,
    "impact": "A Phase3 source-review command used repository-relative Get-Content operands from the audit chat default C:/GitHub working directory. All source reads in this command failed with nonexistent-path errors; no file write or target execution occurred. This is one original command failure, separately counted from the other command in the parallel read batch.\nA Phase3 source-review command used repository-relative Get-Content operands from the audit chat default C:/GitHub working directory. All source reads in this command failed with nonexistent-path errors; no file write or target execution occurred. This is one original command failure, separately counted from the other command in the parallel read batch.",
    "recoveryEvidence": "audit-resume-phase3-notification-workdir-error.json retains exact commandExecution exec-0a89f087-fa59-4253-aa8c-25170e210de3 with cwd and non-truncated original missing-path outputs. The parent sidecar phase3-source-context-wrong-workdir.json describes the batch as one event, while the outer rollup counts both original failed commands. Subsequent absolute-source-root reads are separate evidence; these failed reads provide no source-review proof.\naudit-resume-phase3-installer-workdir-error.json retains exact commandExecution exec-db01f581-510c-4723-a5ef-27dcf0f0141f with cwd and non-truncated original missing-path outputs. The parent sidecar phase3-source-context-wrong-workdir.json describes the batch as one event, while the outer rollup counts both original failed commands. Subsequent absolute-source-root reads are separate evidence; these failed reads provide no source-review proof.",
    "permanentAction": "Audit owner uses the verified immutable target root for every source read, with explicit workdir or literal absolute operands, and reconciles individual failed commands against final process counts. Review date 2026-10-10; closure requires original retention, corrected reads and independent final record review. No source fix or completed audit is inferred.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/terminal-metadata/missing-object-property",
    "position": "before-production-write",
    "count": 3,
    "impact": "Two current metadata updates assigned absent properties directly on ConvertFrom-Json PSCustomObject: agents and post_wave_critic_id. Both failed before stage/canonical metadata replacement. The first of these had already saved a raw sidecar for a preceding failure, so it is not a zero-filesystem-side-effect claim. This reuses the same missing-property root cause recorded in v1.23.0; current stage and raw attempts remain explicit. A rework-parent invocation repeated this failure before canonical audit replacement; the exact original call/output is retained separately.",
    "recoveryEvidence": "audit-resume-command-failures-wave4-complete.json retains exact records exec-51745f6f-03ee-4e97-8c56-d4e6de44b643 and exec-465a97eb-6a12-440d-bb76-2baf32337ad5. Successful Add-Member retries and qualified atomic receipts are distinct from failed attempts. The prior complete rollup bytes are retained by this canonicalization receipt. audit-rework-tool-error-originals.json retains the new exact failure pair. Fresh validation and final audit completion remain pending.",
    "permanentAction": "Audit/release owner uses an explicitly qualified JSON-object reader and stage builder with exact field shape before calling the atomic writer. Review date 2026-10-10; closure requires original failure and sidecar retention, exact accepted-agent provenance, reconciled counters and final independent record verification. No permanent repository repair or completed audit is claimed.",
    "historicalReleases": [
      "v1.23.0"
    ]
  },
  {
    "fingerprint": "security-audit/findings-validator/trace-kind-constraint",
    "position": "before-production-write",
    "count": 1,
    "impact": "The pinned findings validator rejected four intermediate trace entries labeled sink in two records. Raw Phase3 AI and filemanager verifier records contain those intermediate sink labels, while final findings use propagation. Raw originals remain preserved. Whether parent normalization satisfies the mandated discard/fresh-verifier contract is under independent gpt-6-luna/max protocol review; a final structural PASS alone is not accepted as release approval.",
    "recoveryEvidence": "audit-final-findings-trace-kind-error.json retains the exact original commandExecution exec-8c2d7f25-d509-4be9-bff5-b8e6e62a1534 and non-truncated output. Original audit records remain untouched by this retention. Independent protocol review and final process reconciliation are required before freeze.",
    "permanentAction": "Audit/release owner checks exact original record and receipt contracts; malformed verifier output must follow pinned fresh-verification rules, and structural validity must remain separate from audit completeness. Review date 2026-10-10; closure requires retained originals, resolved protocol review and counter reconciliation. No completed compliant audit or permanent repair is claimed by this record.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/terminal-check/validator-receipt-status-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The audit parent's terminal consistency check expected receipt status passed and rejected the actual structure-only receipt status. The check threw before writing its final consistency output; it does not establish a validator failure or source change. Successful corrected read and actual pinned receipts must remain distinct.",
    "recoveryEvidence": "audit-final-validator-receipt-read-error.json retains the exact original commandExecution exec-c67c9afb-179f-4257-b23c-c83715e48e34 and non-truncated output. Original audit records remain untouched by this retention. Independent protocol review and final process reconciliation are required before freeze.",
    "permanentAction": "Audit/release owner checks exact original record and receipt contracts; malformed verifier output must follow pinned fresh-verification rules, and structural validity must remain separate from audit completeness. Review date 2026-10-10; closure requires retained originals, resolved protocol review and counter reconciliation. No completed compliant audit or permanent repair is claimed by this record.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/agent-dispatch/tool-schema-title-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "The root protocol-review dispatch was rejected because the runtime did not accept the advertised title field. No agent was created by this call; retry without title was accepted with the required model and reasoning effort.",
    "recoveryEvidence": "protocol-rework-process-retention.json retains exact raw evidence and the independent protocol-review decision. The fresh dedicated audit parent owns minimal rework; completion is pending accepted fresh independent records and final validation.",
    "permanentAction": "Audit/release owner checks the actual dispatch and pinned raw result contract before ingestion, discards malformed returns without parent repair, and obtains fresh independent validation and reporting passes. Review date 2026-10-10; closure requires complete compliant records and reconciled actual counters. No permanent tool or repository fix is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/verifier-return/report-schema-mismatch",
    "position": "before-production-write",
    "count": 3,
    "impact": "The independent protocol reviewer identified two original Phase 3 records with intermediate sink trace kinds forbidden by the pinned schema. Those two original verifier returns are malformed and cannot count as accepted Phase 3 dispositions. This counts the two returned records separately from the previously recorded final validator command failure. The first fresh FileManager rework verifier also returned an intermediate sink in a multi-step trace; the dedicated audit parent discarded it and requested a different fresh verifier. This is one additional malformed verifier return.",
    "recoveryEvidence": "protocol-rework-process-retention.json retains exact raw evidence and the independent protocol-review decision. The fresh dedicated audit parent owns minimal rework; completion is pending accepted fresh independent records and final validation. audit-filemanager-rework-malformed-r1-retention.json and its exact original copy preserve the new malformed return with SHA256 3165cabd5aaac854a9382215654e17fdd99c77f84e080512db3ebb36907a7efe. A valid fresh verifier and distinct Phase5 are still required.",
    "permanentAction": "Audit/release owner checks the actual dispatch and pinned raw result contract before ingestion, discards malformed returns without parent repair, and obtains fresh independent validation and reporting passes. Review date 2026-10-10; closure requires complete compliant records and reconciled actual counters. No permanent tool or repository fix is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase3-ingest/malformed-parent-repair",
    "position": "before-production-write",
    "count": 1,
    "impact": "The previous audit parent changed the malformed Phase 3 trace kinds in the final findings file rather than discarding the malformed results and assigning fresh Phase 3 verifiers. The independent protocol review requires fresh Phase 3 followed by separate independent Phase 5. This is the parent repair operation, separately counted from the two malformed returns and the already retained validator command failure.",
    "recoveryEvidence": "protocol-rework-process-retention.json retains exact raw evidence and the independent protocol-review decision. The fresh dedicated audit parent owns minimal rework; completion is pending accepted fresh independent records and final validation.",
    "permanentAction": "Audit/release owner checks the actual dispatch and pinned raw result contract before ingestion, discards malformed returns without parent repair, and obtains fresh independent validation and reporting passes. Review date 2026-10-10; closure requires complete compliant records and reconciled actual counters. No permanent tool or repository fix is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/evidence-retention/undefined-process-variable",
    "position": "before-production-write",
    "count": 1,
    "impact": "A rework-parent recorder command referenced the prior process variable $run without defining it in this invocation. Join-Path failed, then the command wrote a 4-byte JSON null recorder artifact. The malformed recorder is preserved; canonical audit metadata and ledger were not replaced by this command.",
    "recoveryEvidence": "audit-rework-tool-error-originals.json retains the exact original command/output and the failed null bytes. Root retains evidence directly from the verified agent rollout; no missing chunk ID is fabricated.",
    "permanentAction": "Audit/release owner uses self-contained Python dictionary staging and the qualified atomic writer; no cross-call PowerShell variables or nested JS templates. Review date 2026-10-10; closure requires compliant fresh records and exact final counter reconciliation. No permanent process repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/rework-staging/ledger-check-count-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The first Python rework staging helper assumed eight old local_checks would be removed. It found five and failed its assertion after writing only a private metadata stage. No canonical audit replacement occurred. The actual old checks were subsequently inventoried across four units; this counts the failed staging invocation once.",
    "recoveryEvidence": "audit-rework-ledger-check-count-error.json retains the exact original function call/output. The audit parent preserves the failed helper and metadata stage, then prepares a distinct corrected helper using the observed five checks. Fresh P3 and P5 remain pending; this record establishes no audit completeness.",
    "permanentAction": "Audit/release owner derives check counts from the actual retained ledger and stages all evidence before atomic promotion. Review date 2026-10-10; closure requires preserved failed attempts, accepted fresh independent records and final validators/counter reconciliation. No permanent repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/rework-inspection/inline-python-literal-newline",
    "position": "before-production-write",
    "count": 1,
    "impact": "A read-only envelope-inspection command passed literal backslash-n characters to Python -c through PowerShell. Python rejected the command at parsing; no target code or file operation ran. The command must use a saved, self-contained script instead.",
    "recoveryEvidence": "audit-rework-inline-python-newline-error.json preserves the exact original call/output. Root supplied inspect-audit-json.py as a literal-file mechanical structure inventory; its output is not a semantic audit or schema acceptance decision. Fresh source validators and Phase 5 remain responsible for those decisions.",
    "permanentAction": "Audit/release owner uses saved Python files and literal structured arguments, avoiding inline code and nested template quoting. Review date 2026-10-10; closure requires accepted fresh records and actual final validators/counter reconciliation. No permanent process repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/phase5-prompt/prefix-scratch-path-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The prompt helper assumed the old Phase5 scratch path appeared in a copied prompt prefix. Its assertion failed after creating a private FileManager retry prompt and before writing the notification Phase5 prompt. No target source or shared audit JSON was modified by this invocation.",
    "recoveryEvidence": "audit-rework-p5-prompt-prefix-error.json retains exact raw tool call/output. The failed helper and partial prompt are preserved. Root directed use of structured direct dispatch rather than guessed prefix substitution; accepted fresh roles and final records are still required.",
    "permanentAction": "Audit/release owner dispatches explicit fresh roles with actual scope and fixed contract, avoids unverified prompt-string substitutions, and preserves failed attempts. Review date 2026-10-10; closure requires valid independent records and actual final validators/counter reconciliation. No permanent process repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/verifier-preflight/model-self-description-confusion",
    "position": "before-production-write",
    "count": 1,
    "impact": "The fresh FileManager retry verifier initially declined source verification because it could not reconcile its generic model self-description with the required configuration. Its actual first and followup turn_context records both explicitly show gpt-6-luna/max. Clarification and a followup were needed before source verification continued. This is an interrupted preflight turn, not a malformed candidate record or evidence that the configured model was unavailable.",
    "recoveryEvidence": "audit-verifier-model-identity-refusal-original.json preserves the exact first-turn final, session agent identity and actual model/effort contexts. Root mechanically verified routing; the dedicated parent reports the verifier accepted this evidence and continued. Its final candidate record and independent Phase5 remain required.",
    "permanentAction": "Audit/release owner verifies accepted explicit dispatch against available actual turn metadata, distinguishes generic self-description from routing facts, and reports truly unavailable configuration without substitution. Review date 2026-10-10; closure requires accepted fresh validation and actual final record/validator reconciliation. No permanent platform fix is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/skill-pin/unavailable-git-metadata",
    "position": "before-production-write",
    "count": 1,
    "impact": "The rework audit parent attempted git rev-parse HEAD in the configured pinned skill directory, which contains qualified fixed files but no Git metadata. The read-only Git probe failed. In that same batch both pinned structural validators passed; no target code was run and no source was modified by the failed probe.",
    "recoveryEvidence": "audit-rework-skill-pin-git-probe-error-original.json preserves exact parent wrapper call and output. The parent's skill-pin-git-probe-error-original.json also retains visible command/error and honestly leaves unavailable nested exit_code/call_id null. Existing skill-pin-integrity.json binds all twenty files to the fixed commit; it does not claim the extracted directory is a Git checkout. Final audit handoff must disclose this independent provenance limit.",
    "permanentAction": "Audit/release owner verifies extracted skill files against the retained authoritative twenty-blob pin proof, and only uses git provenance commands on a verified Git repository. Review date 2026-10-10; closure requires fixed-file verification and honest final provenance disclosure. No permanent upstream/platform repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/rework-staging/patch-context-assumption",
    "position": "before-production-write",
    "count": 2,
    "impact": "A patch to the private terminal metadata staging helper assumed context that did not match the current file. apply_patch rejected it without changing the helper, source, or canonical audit evidence. A second patch to the same private terminal staging helper also failed context matching. This second original rejection is counted separately, with no source or canonical audit mutation from that rejected attempt.",
    "recoveryEvidence": "audit-rework-terminal-patch-context-r1-original.json retains the exact patch and rejected wrapper output at call_e4QDmx6fsMG67gDLwIIPVbVj/output2669; the parent also retained stage-terminal-script-patch-context-error-original.json. The parent was instructed to use a stable structured JSON projection input rather than repeatedly editing literal counter-version strings. audit-rework-terminal-patch-context-r2-original.json preserves the second actual patch and rejected wrapper output at call_hbD3GlKqDxitzqEYkfOZUj6q/output2711. Neither rejection is treated as successful evidence or hidden by later staging.",
    "permanentAction": "Audit/release owner reads the actual private staging helper before a narrow patch and passes changing process projection data as a structured input. Review date 2026-10-10; closure requires the final atomic canonical receipt and actual count readback. No permanent project or platform repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/rework-inspection/json-top-level-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "The private inspect-stage-fields.py helper called dict.get on the actual top-level list in process-incidents.json. The read-only inspection failed after printing earlier stage/projection data. It performed no canonical or source write.",
    "recoveryEvidence": "audit-rework-inspection-json-top-level-error-original.json preserves exact parent wrapper call/output at call_x8iV5pKPBePHRKvAbwXbrkGR/output2780 and its original output truncation. The failed helper remains unchanged in the parent scratch. Root requested a mechanical ownership handoff instead of further new probe helpers; findings, scope and source-security judgments remain with the dedicated gpt-6-luna/max auditor.",
    "permanentAction": "Audit/release owner validates actual top-level JSON types before selecting fields and uses a fixed mechanical counter projection input. Review date 2026-10-10; closure requires a qualified atomic commit, readback and dedicated auditor's final semantic approval. No permanent project or upstream repair is claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-acceptance/report-release-metrics/missing-coverage-line-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "The required stable acceptance writer used a coverage sentence that did not match the canonical - 覆盖检查...： line grammar. report-release-metrics rejected it before any docs commit or push. The publicly released product and passed L3/public evidence were unchanged.",
    "recoveryEvidence": "root-acceptance-coverage-format-error-original.json retains the exact tool call/output; acceptance-first-generated-r1 retains the first generated documents, writer, orchestration helper and original check log. The writer is corrected to the repository-required line grammar, the two owned generated documents are recreated from exact P and current receipts, and the same metrics gate is rerun in a new r2 log.",
    "permanentAction": "Release owner must use the canonical required acceptance field format from report-release-metrics and the repository template. Review date 2026-10-10; closure requires the prepared corrected writer passing the unchanged metrics/governance gates. This helper repair does not claim permanent repository tooling adoption.",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-acceptance/report-release-metrics/timestamp-precision-contract",
    "position": "before-production-write",
    "count": 1,
    "impact": "The second generated acceptance passed coverage-decision grammar but copied the original six-digit microsecond freeze timestamp. The canonical metrics grammar accepts at most three fractional digits and rejected it before docs commit or push. The stable product, public image and raw freeze evidence were unchanged. This is a second format-contract failure in the same external writer family, distinct from its missing coverage-line grammar.",
    "recoveryEvidence": "root-acceptance-timestamp-format-error-original.json and acceptance-first-generated-r2 preserve the exact failed return, writer, generated documents and r2 log. The writer converts the authoritative freeze timestamp to milliseconds only for the six-row metrics block; freeze.json retains the original microseconds. Current incidents are regenerated without dropping either failure, followed by the unchanged complete docs gate in a fresh r3 log.",
    "permanentAction": "Release owner must qualify the whole canonical acceptance contract, including precision, against report-release-metrics before a future production write. Review date 2026-10-10; closure requires canonical field/date/process blocks passing unchanged guards. This local writer repair does not establish permanent repository tooling adoption.",
    "historicalReleases": []
  },
  {
    "fingerprint": "resource-cleanup/powershell-file-entry/execution-policy",
    "position": "before-production-write",
    "count": 1,
    "impact": "The owned resource helper stopped when its nested powershell -File invocation returned 1. The original nested stdout/stderr were not retained. No git worktree removal was reached; both candidate and CF worktrees remained registered. A later readonly file probe reproduces the Restricted policy denial; this supports the diagnosis but does not replace the lost original nested stderr.",
    "recoveryEvidence": "root-local-cleanup-file-entry-error-original.json preserves the exact outer failure. powershell-entry-results-readonly.json preserves the subsequent separate readonly diagnostic, with zero deletions. Native PowerShell filesystem commands use the already approved precise owned path, fresh path/process/recovery checks and original execution policy. Final receipts and a separately qualified acceptance addendum record actual outcomes.",
    "permanentAction": "Release owner must qualify Windows invocation/module availability before cleanup, capture nested stdout/stderr before checking exit status, and preserve resources on failed eligibility. Review date 2026-10-10; closure requires the actual native entry and all ownership/path/process/Git recovery checks passing. This repair is not permanent repository tooling adoption.",
    "historicalReleases": []
  },
  {
    "fingerprint": "resource-cleanup/powershell-policy-probe/module-autoload",
    "position": "before-production-write",
    "count": 1,
    "impact": "The initial readonly diagnostic raised CalledProcessError while querying Get-ExecutionPolicy from Python. Its nested stderr was not retained. A later deliberately observed readonly probe preserves CouldNotAutoloadMatchingModule for Microsoft.PowerShell.Security; direct shell lookup succeeds. No deletion or product operation was performed by this diagnostic.",
    "recoveryEvidence": "root-policy-probe-module-error-original.json preserves the exact outer failure. powershell-entry-results-readonly.json preserves the subsequent separate readonly diagnostic, with zero deletions. Native PowerShell filesystem commands use the already approved precise owned path, fresh path/process/recovery checks and original execution policy. Final receipts and a separately qualified acceptance addendum record actual outcomes.",
    "permanentAction": "Release owner must qualify Windows invocation/module availability before cleanup, capture nested stdout/stderr before checking exit status, and preserve resources on failed eligibility. Review date 2026-10-10; closure requires the actual native entry and all ownership/path/process/Git recovery checks passing. This repair is not permanent repository tooling adoption.",
    "historicalReleases": []
  },
  {
    "fingerprint": "resource-cleanup/native-powershell-entry/automatic-policy-denial",
    "position": "before-production-write",
    "count": 1,
    "impact": "Automatic approval review rejected the native PowerShell recursive node_modules removal before execution with blocked by policy. No more specific reason was supplied. The candidate directory, worktree and its local branch are preserved; no alternate removal mechanism is attempted. Stable publication, verified remote archives and main are unaffected.",
    "recoveryEvidence": "native-owned-dependency-delete-policy-rejection-original.json preserves the exact rejected call/output. Final cleanup records the preserved candidate and remote recovery SHA, handles only other independently eligible clean tracked resources, and updates the required acceptance process block with this late event.",
    "permanentAction": "Release owner preserves a policy-blocked resource and reports the exact approval reason. Review date 2026-10-10; closure requires an approved cleanup route or continued documented retention. Do not change execution policy, force-remove the worktree, or hide this action inside another tool.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 资源回收与遗留风险

- Arena自有L3 work/inbox、fixture二进制与独立配对runtime已回收，逻辑字节 `1079386041`，实际净空闲变化 `1141026816`；保留全部原始证据/首轮失败、bundle、固定Runner/镜像与缓存，未global prune。
- 18个非自有容器只在 `before fresh stable L3 through final owned resource cleanup; not retroactive proof for earlier script/old-version fixture prechecks` 比较，不追溯早期脚本/旧版本预检，也不解释为生产核对。
- 自有文档/候选/审计工作树在全部恢复和Git/进程检查后收尾；实际净空间/跳过项及保留恢复点见最终回执，活跃作者/未知改动保留。
- 所有审计待确认事项按run16报告和safe跟进计划；图库14px规范差距、长期网络/arm64/router/Windows等未验证边界继续保留，不扩大本轮产品范围。

## 本地收尾实际结果

- 候选远端与恢复标签仍为产品提交 `ce27dc5171a97ed6e3d9475cddfdfac89762aad3`；本地 `release/v1.24.0-candidate` 和工作树 `C:/GitHub/_codex-tasks/kpanel-v124-rc1-assembly` 保留，含 `web/node_modules`。自动审批审查在执行前拒绝其递归删除，理由仅为 `blocked by policy`；未通过其他入口再次删除。
- 自有只读 CF 工作树已通过所有权释放、clean、无忽略文件、路径/进程及标签恢复点复核后使用 `git worktree remove` 回收；逻辑字节 `45494769`，该窗口实际净空闲变化 `51429376` 字节。其他作者的工作树登记保持一致。
- 初始嵌套 PowerShell 清理与只读诊断未保存内部 stderr；原始外层失败和后续独立只读探针分别保留，未将后续诊断冒充首次原始 stderr。全部收尾异常均发生在生产写前，实际流程累计 `86`；本次产品发布、公开镜像和生产写 0 的结论不变。
- 首次验收提交 `6ddcfda15b7cbc37756e5062c6b4b7fd6447defc` 已通过候选及主线 CI 并归档；本补充只修正本文件的晚发生流程指标与实际资源结果。最终主线/归档 SHA 和清理回执见 `final-alignment.json`、`final-local-resource-cleanup.json` 和 `cleanup-acceptance-worktree-cleanup.json`。
