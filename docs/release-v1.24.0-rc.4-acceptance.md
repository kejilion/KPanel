# KPanel v1.24.0-rc.4 发布验收记录

日期：2026-10-01。发布级别：L3。`releaseChannel=preview`；`releaseTrain=1.24.0`。

- 冻结产品候选：`0cbee81494991166f9b32b495494a4616463a73d`；发布前 `main` 基线 `95bcca2b102683e3916eac9b923faceb52053c56`。注释标签 `v1.24.0-rc.4`，tag object `d1cf2d956925d3ba1cd6110fbfe9c58900512e36`。候选 CI → 同 SHA 主线 CI → 标签 → Release，验收文档另走独立候选与主线 CI。
- 上一稳定版及回滚点：`v1.23.0` / `b73d62628ed77e12ce03715fafc1aa490b884f96`，Docker index `sha256:236759215c0f527ac8e35bb66d3bd7a3f9f3d3c7a9110df3110b3048d11f46e5`；上一不可变预览 `v1.24.0-rc.3` / `086fcbcc8d41cb8634cb556464f06e376bd94e60` / `sha256:4491b866b5453cb32b9e12104901761067e92cd52e76809220a265d598a6c2a2`。
- `release/v1.24.0-candidate` 本地与远端对齐到产品 SHA，保留供下一 RC；预览版禁止生产部署。

## 发布画像与精确范围

- 业务域：经典文件双栏、文件编辑器布局、集群主机菜单与终端路由。变更面为 UI 与浏览器本地偏好；没有新后端入口、宿主机协议或数据库迁移。
- 核心旅程：宽屏打开两个独立文件栏，各自选择目录和主机，窄化后堆叠且保持挂载；关闭时核对进行中操作、未保存内容和导航成功；集群菜单按主机实际能力呈现，选中终端/文件入口携带正确主机身份；编辑器侧栏、标签与工具栏保持对齐。
- 未变化契约：Panel/Agent 权限、文件 API 与资源版本、配对能力、宿主脚本、端口、Compose、安装与应用市场默认入口。双栏偏好 `kpanel:files:split:v1` 仅保存第二栏目录/主机和开关，不保存打开文件或凭据。

| 来源分支 | 精确 tip | 内容与处置 |
| --- | --- | --- |
| `fix/file-editor-alignment` | `664270051b9b934cfc0f6b9178ab8a3a0f8ab876` | 9 行新增、2 行删除的 L1 CSS 对齐，保留本地来源 |
| `claude/files-dual-pane` | `64422127f2fc74fd27858c799a106c1a6081a2f0` | 经典双栏、内存路由、密度/堆叠、关闭及焦点，保留原本地/远端来源 |
| `claude/cluster-context-menu` | `7151e90b4c4ccf8c501451fed7c6b05e3492031a` | 主机菜单、能力呈现与目标主机终端路由，保留本地来源 |

合并后仅共享翻译文件的末尾追加发生冲突，保留双方条目并通过 i18n 校验。独立复核主要实现者 Claude 与最终验证/发布者 Codex 不同，结果 PASS WITH FOLLOW-UP；Codex 来源的编辑器是小于 30 行 L1 修改，不宣称为跨提供商 L2 复核。

发布任务在冻结前复现两条关闭回归：主栏未注册关闭守卫、路由取消后第二栏仍被移除。修正 `42ef6a80e90925c14e506118b44477bd20706c6a` 后 97 项定向测试通过；这是源码候选发现并修复，不是已发布版本的产品变更失败。三个来源所有权未释放，不改写或删除其工作树、upstream 与原分支；它们已被本次产品包含，不再归类为待发布增量。历史审计文档、未知工作树和冻结后的新功能不纳入。

## 外部审计与跨仓库联动

- CF 产品覆盖检查 `decision=scoped-required`，52 个提交、97 个文件未覆盖，新增边界包 `internal/backupremote`；最近 full `run-4`，9 天。原件 `C:/GitHub/_release-evidence/v1.24.0-rc.4/security-coverage.json`。RC 只记录，稳定版前补 scoped；本轮没有执行 CF 审计。
- OCR 1.12.6 本轮精确区间 `95bcca2..3e50476 files=27/27 free-form=1 valid=H0/M1/L0 constrained-only=0`；自由臂先完成，规则臂逐文件检查。`3e50476` 追加 trailer 后成为 `42ef6a80`，两者 tree 相同，版本准备只改发行元数据及业务事实；冻结检查的 stale 提醒以该精确 tree 证据和 metadata skipped 原因解释，不把提醒等同审计通过。来源已有 OCR trailer 原样保留，不据单轮样本推断长期有效性。
- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集编号、脚本候选不适用。实际内置脚本 commit `779192048077c130442a64a126d7c0050776d868`，SHA-256 `33010d547355f9bde4067c189a0457f44dc00ec3a72eaa25e5b7101e1fcb9c04`。安装、更新、运行时协议和镜像脚本没有变化。
- 应用市场本地与公共 `kpanel.conf` 归一化内容均一致，blob `fa4b95374ed3b186d920207537f451a5b6d839f7`；无需应用市场提交，不改变默认 `latest` 来源。跨仓库没有写入。

## 多维质量结论

| 维度 | 状态 | 证据与实际边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 全量与 97 项定向、模拟 UI 主机路由；真实配对主机复制/移动及终端生命周期仍未重测 |
| 网络入侵与供应链安全 | 已验证 | 固定工具源码/镜像扫描与 govulncheck；CF 未覆盖，LOW 依赖风险见下文 |
| 稳定性、失败恢复与兼容 | 已验证 | 主栏守卫/取消回归、既有安装/更新/回滚/备份自动用例；真实 NAS、长时间负载未验证 |
| 性能与资源预算 | 已验证 | 固定发布镜像的低资源运行约束；双栏多一份可见界面状态，不新增后台轮询协议，无长期 soak 结论 |
| 用户体验与可访问性 | 已验证 | 14 组模拟数据 UI 检查，浅/深、中/英、宽/窄、键盘/焦点、空/错误恢复；原生浏览器缩放未验证 |
| 数据、配置与迁移 | 不适用 | 无数据库/宿主配置迁移；损坏的浏览器双栏偏好安全回退，旧单栏状态默认关闭 |

## 自动门禁与浏览器证据

- 定向测试 5 文件、97 测试通过；两条补充关闭断言在修复前失败、修复后通过。原件 `pane-close-reproduction.log`、`targeted-tests.log`。
- L3 唯一外层入口 `scripts/run-release-l3.mjs`，run ID `v1.24.0-rc.4-0cbee814-l3-r1`，目标 `arena-154`；Runner `kpanel-release-gate:go1.26.7-node24`，不可变 ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`。本轮 SSH 恢复连通，未沿用上一轮 WSL 结果。
- bundle `c9ad910e2bbf669e7f7dc1dcb2a2dadcb435b8c75330e92ca60ec0dd03607628`，plan `023c8e6cf3ea059da19ead32174fb084af7425b50721808fe1aa60fff428208e`，执行脚本 `21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；终态 `passed / exit_code=0`。回收目录 `C:/GitHub/_validation/v1.24.0-rc.4-0cbee814-l3-r1`，回收摘要 12 项复核。
- L3 覆盖 Go 全量、前端 224 个测试文件、2020 项测试通过、核心特权包 race、go vet、govulncheck、固定 Trivy 源码及镜像、场景资源重现、双架构构建、镜像/脚本与安装/更新/回滚/备份生命周期。YAML 与固定 Action、版本、通道及产物契约由既有门禁核对。
- [候选 CI 36815557980](https://github.com/kejilion/KPanel/actions/runs/36815557980)、[产品主线 CI 36816283029](https://github.com/kejilion/KPanel/actions/runs/36816283029)、[Release 36816928968](https://github.com/kejilion/KPanel/actions/runs/36816928968) 均为上述产品 SHA、成功；验收提交 CI 单独记录于 closeout.json。
- 浏览器为 Windows 本机 Chrome `154.0.8037.59`，模拟数据，`visual-composition`，编辑器 9 组、双栏/菜单 5 组通过，未处理页面异常 0。最小 CSS 字号编辑器 13px、双栏操作与路径 14px；100%/125%/200% 为视口/DPR 模拟，不冒充原生缩放。
- 双栏实际验证两份文件栏共存、独立目录导航、窄屏保持挂载、活动栏与空目录、关闭后的焦点；主机菜单验证本地/远端/轻节点能力、键盘/Escape、当前视口内浮层与目标主机路由。集群列表在较窄宽度保留原有内部横向滚动，不宣称全部指标无滚动可见。
- 普通短时本地 UI 预览检查不创建远程长测作业；没有登记主机浏览器、soak、真实双端文件写入、真实宿主终端或 NAS 验证。首轮及第二轮截图/自动滚动失败与成功 r3 分别保留，见流程异常。
- acceptance 模拟预览保留 ready 供用户体验，绑定产品 SHA：`http://127.0.0.1:4175/files` 与 `/cluster`；manifest `C:/GitHub/_preview-evidence/kpanel-v124-rc4-0cbee814/manifest.json`，启动任务承担停止责任。精确停止入口：`node scripts/local-feature-preview.mjs stop --evidence-dir "C:\GitHub\_preview-evidence\kpanel-v124-rc4-0cbee814"`。旧来源编辑器预览所有权未移交，不擅自停止。

## 公开产物与通道

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.24.0-rc.4) 于 UTC `10/01/2026 05:03:30` 公开，`draft=false`、`prerelease=true`、非 Latest；GitHub Latest 仍为 `v1.23.0`。
- [Docker 版本镜像](https://hub.docker.com/r/kjlion/kejilion-panel/tags?name=1.24.0-rc.4) 与 `preview` 的 OCI index 均为 `sha256:80578043ae505d6b2f34d3a3cd1b168dab78d89824c68c648846458c5ad991f0`；linux/amd64 `sha256:5741d53c99b2febc16e56fbceefe36a316bf151cfb99654b39c73adb482cb3bb`，linux/arm64 `sha256:c18b853f8a88e85f47746c068de5225ae295c7f7e614a56685b6cd202d40bf0e`，另有 provenance/SBOM attestation。支持平台不等同运行验证，arm64 本轮未运行容器。
- Docker `latest` 与 `1.23.0` 均保持首段稳定 index；发布前后 JSON 回读与默认入口比较留在同一证据目录。
- Release 14 附件，下载 `SHA256SUMS` 自身摘要 `sha256:f138f7d2341f941cc9f7ecc3d0bd353b22f603648e5b27bae2b1889937de51bc` 与 API 一致，清单 11 项与各资产 API digest 相同；未逐一下载并运行各平台二进制。
- 公共版本镜像明确拉取后以冻结源码 `packaging/tests/image-e2e.sh` 运行，`image_e2e=pass`，退出码 0。OCI version/revision、RepoDigest、非 root 用户与发布 index 一致；这是 linux/amd64 的公开产物证据，不代表生产部署。

## 依赖、自更新、生产与回滚

- 本版没有升级依赖、工具链、基础镜像、Action、扫描器或受管脚本，只同步应用版本字段。当前 Go `1.26.7`、Node.js `24.20.0`；govulncheck 可达漏洞与 npm LOW 的原始结果见本轮 L3 日志。
- DOMPurify `3.4.14` LOW `GHSA-p98j-92pf-mc4p`，修补 `3.4.16` 在本次冻结范围外；AiMarkdown 采用返回字符串模式，不使用相关 IN_PLACE/hook。由依赖维护或稳定发布任务在稳定版冻结前复核兼容补丁，LOW 未越过现有 HIGH/CRITICAL 阻断线；不宣称所有依赖无漏洞。
- 自更新仍区分 stable/preview；加入预览只选择来源并检查，自动安装开关独立；退出预览不会自动降级。安装/迁移/备份/更新/失败恢复自动门禁与 OpenRC 边界不外推为真实 NAS systemd 旅程。
- 生产部署安全核对：不适用（预览版禁止生产部署）。`arena-154` 只执行隔离候选验证，未执行生产用途、正式部署或正式数据修改；`prod-108` 禁用全部 KPanel 操作，本次未连接、未备份、未部署、未升级、未核对。
- 回滚源码与镜像见首段，不移动公开 tag 或覆盖不可变版本。数据/生产备份和实际回退不适用；将来手动回退先备份、核对兼容性并单独获得高风险操作授权。

## 交付节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-01T10:34:23+08:00
- 候选冻结时间：2026-10-01T12:09:01+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本轮未发生已公开版本的产品失败、生产退化、回滚、紧急热修复或同版本重复发布。预览不计入正式部署频率；冻结前的候选关闭问题由回归明确发现并修复。流程异常独立统计，不能推断为产品变更失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-metadata/git-diff-check/trailing-blank",
    "position": "before-production-write",
    "count": 1,
    "impact": "版本准备文档末尾多余空行被 diff 检查拒绝，未提交无效文档。",
    "recoveryEvidence": "去除空行后同一检查通过，冻结候选 0cbee814。",
    "permanentAction": "追加发布事实时只保留一个终止换行，提交前继续执行 diff --check。",
    "historicalReleases": []
  },
  {
    "fingerprint": "apps-contract/powershell-parser/string-overload",
    "position": "before-production-write",
    "count": 1,
    "impact": "换行归一化使用 Char 重载配空字符串，证据解析拒绝；没有应用市场写入。",
    "recoveryEvidence": "显式 String 重载后本地及公共 kpanel.conf 完全一致，apps-contract.json 留存。",
    "permanentAction": "比较内容用明确 String 参数，不以命令退出推断契约一致。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser-preview/native-fixture/viewport-scroll",
    "position": "before-production-write",
    "count": 2,
    "impact": "整页截图及自动滚动触发菜单按设计关闭，两次未完成的浏览器记录均保留。",
    "recoveryEvidence": "只截当前菜单视口，先完成明确滚动并等待两帧，再操作；split-menu-r3 五组全部通过，候选与产品代码不变。",
    "permanentAction": "临时命令规格固定菜单操作的滚动顺序，整页文件布局截图和浮层视口截图分开；同类旅程后续复用本次原件或沉淀稳定仓库入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "supplemental-runner-query/ssh/format-argument",
    "position": "before-production-write",
    "count": 1,
    "impact": "可选 Docker stats 读取的带空格 format 跨 SSH 参数被拆开，查询拒绝；L3 固定入口及产品未受影响。",
    "recoveryEvidence": "继续以已声明的 L3 持久日志及外层终态提供验证证据，不将查询错误写成产品失败。",
    "permanentAction": "辅助 SSH 读取避免带空格的 format 参数，只使用固定文件与无歧义参数；L3 继续由唯一仓库入口执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence-aggregation/powershell-array/concatenation",
    "position": "before-production-write",
    "count": 1,
    "impact": "汇总 JSON 将两条失败证据路径合并成一个字符串，读回发现结构错误；原始文件均保留。",
    "recoveryEvidence": "每个数组表达式明确加括号，读回 failureEvidence.Count=2。",
    "permanentAction": "结构化聚合明确数组元素边界并核对类型、数量，不以 JSON 可解析代替 schema 正确。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-gate/run-repo-bash/unsupported-env",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地验收入口只允许 VERIFY_LEVEL，传入 VERIFY_BASE_REF 被拒绝；门禁尚未执行，未改变已公开产品。",
    "recoveryEvidence": "原始 acceptance-l0-r1-failed.log 保留；改用 verify-change.sh 的精确基线位置参数，成功结果见 acceptance-l0-r2.log。",
    "permanentAction": "调用前核对 run-repo-bash.mjs 的封闭参数 schema，基线通过受支持的脚本位置参数传递。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 分支与资源收尾、未完成项

- 产品候选本地/远端保持产品 SHA 和既有 upstream；`main` 验收后只增加验收及当前公开事实文档。纯验收分支 `docs/release-v1.24.0-rc.4-acceptance` 经同 SHA 候选 CI 和主线 CI 后保存到 `archive/docs/release-v1.24.0-rc.4-acceptance`，再移除活跃引用。
- 验收 CI、最终归档 ref/SHA、远端复核、管理树同步与可再生资源处置记录在 `C:/GitHub/_release-evidence/v1.24.0-rc.4/closeout.json`。不以产品 CI 代替文档 CI，不改变产品候选与标签。
- 来源工作树、历史本地审计分支、应用市场的独立工作树与未知资源保留。产品 worktree 与 node_modules 因 ready 用户预览保留；唯一 bundle、kit、成功 L3、原始失败、截图和 trace 不删除。自己生成的验证副本与纯验收 worktree 只有在完成、无进程引用且恢复证据确认后回收，实际字节口径见 closeout。
- 已回收本轮 `/root/kpanel-release-work/v1.24.0-rc.4-0cbee814-l3-r1` 临时副本，删除前核对真实路径、冻结 HEAD、clean 状态、无候选容器挂载、公开 E2E 及 12 项回收摘要；逻辑释放 503058432 字节，不等同 Windows 卷物理释放量。原始远端 evidence 与本机 kit 保留，原件 `remote-cleanup.json`。纯验收记录本地 L0 及 227 项治理测试通过，参数拦截及恢复日志分别保留。
- 遗留风险：CF scoped、真实配对主机文件写入/终端生命周期、原生浏览器缩放、公开 arm64 运行、真实低配/多卷长时间负载，以及 rc.3 的 fnOS/NAS 安装重启与恢复。预览门禁不代表全部实机或生产准入；由下一次稳定发车或对应专项验证触发复核。
