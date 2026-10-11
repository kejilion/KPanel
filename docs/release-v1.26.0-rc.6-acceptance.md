# KPanel v1.26.0-rc.6 发布验收记录

日期：2026-10-11

发布级别：L3

候选提交 / 标签：`0d43a08ac53d4f493e4a32ea288b3ee2450be6d5` / `v1.26.0-rc.6`

上一稳定版本 / 回滚点：`v1.25.1` / `sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`。上一预览为 `v1.26.0-rc.5` / `sha256:1cab7f607c8ddc6fd84489210caf544fbceb5888a2d836efecaa43f02bfbf242`。

`releaseChannel`：`preview`

`releaseTrain`：`1.26.0`

候选分支与发布后处置：`release/v1.26.0-candidate` 保留在本产品提交，供本发布序列继续使用。

- 纳入来源：`fix/docker-terminal-agent-capability` / `b7208ce19509bf672ee64308960b6618a4a58bab`，原始 tip 已接管，全部四个产品文件 Git blob 与交付一致。
- 已归档：`archive/fix/docker-terminal-agent-capability` / `b7208ce19509bf672ee64308960b6618a4a58bab`，SSH 远端精确读回通过，原活动远端不存在、本地已重命名归档。本轮来源工作树已移除，可由不可变标签或归档恢复。
- 冻结前全部本地和远端分支已比较 RC5 台账；只有上述一组成型新候选。远端 offers 旧 tip 已为 RC5 祖先，其他旧来源不重复列为新增功能。原始清单在本轮 intake.json。
- 其他作者工作树与活跃预览仍保留。此前自动审批以 blocked by policy 拒绝清理的六个历史工作树继续保留；本轮未重试或绕过。

产物已发布，生产未部署。

## 发布画像

- 业务域：Docker 持续终端与 KPanel 自有 Agent 生命周期。
- 变更面：有界进程身份就绪等待；安装/更新生成的 systemd capability 配置；发布元数据。
- 用户旅程：容器“进入控制台”启动、目录输入、尺寸调整、关闭与 Agent 重启；旧 RC5 服务配置迁移；实际发布信息与更新弹窗。
- API、存储、端口、Compose 和 kejilion/sh 协议未改变。应用市场 Agent 模板两行补 CAP_KILL，和既有 canonical unit 对齐；权限迁移必须显式刷新服务配置。
- L3：涉及宿主机进程控制和发布通道。无生产操作。

## 发布范围与未纳入内容

- 新增修复：补齐应用市场 Agent 的 CAP_KILL；Docker exec 返回 running 但尚未安装 nonce 时，在原调用截止内等待。错误/空/前缀 nonce 与读取失败继续拒绝，pidfd、TTY、进程出生时间、daemon 身份检查保留。
- 提交：批准 main `b78cff2b341f697ef3340e56b4655886b7dc0983`；来源 `b7208ce19509bf672ee64308960b6618a4a58bab`；集成 `32b958ab7ce3d6c4effcc69f6d84533f66b186a0`；版本与发布说明 `0d43a08ac53d4f493e4a32ea288b3ee2450be6d5`。
- RC5 全部既有功能继承，见[RC5验收](release-v1.26.0-rc.5-acceptance.md)。本轮没有其他新功能、稳定提升或生产部署。

## 外部审计与修复交付

- CF 覆盖：`decision=scoped-required`，待审 76 提交 / 196 文件，full 距今 19 天、最早待审 6 天。预览只记录，未执行新的 CF 审计，不宣称安全审计完成。run31/32/33 的 critic/verifier/Phase5/raw 证据缺口和 needs_validation 继续开放，稳定前补审。
- 来源独立复核 PASS WITH FOLLOW-UP：原 reviewed tree 与来源最终 tree 相同，最终四个集成 blob 一致；正式 systemd 实机回执随后通过。旧生命周期首次更新不能补权限的兼容性条件已进入 Changelog 和真实弹窗。
- OCR 1.12.11：来源 3/3 supported、4/4 处置；最终范围 6/6 reviewable、9/9 处置，自由臂 0、有效 H/M/L 0/0/0、blind=false、constrained-only=unreported。最终范围辅助记录晚于 L3 启动，源码 SHA 未变；发布元数据此前已人工复核。文档/conf unsupported、锁文件规则排除后逐项人工检查，未把本轮计为新的盲测收益。
- 修复交付：源码和本 RC 已发布；稳定版和生产不适用。本稳定周期工具有效性仍不足三个周期，不提前给出退出或增益结论。

## 跨仓库联动判定

- `scriptLinkageState=not-required`：无需发布 kejilion/sh 脚本（不适用）。脚本定义的协议、内置内容和安装入口不变。
- 内置脚本基线 commit：`1800d955f216aebd2776674368a8e469a671c489`；SHA-256：`5778fdc9637c8614f5246f9eb13c1e549de51522fb91b3805067cbc0475b614c`。候选脚本修订不适用；复用公开兼容基线，L3/公开镜像契约已核对。
- 变更集：`kpanel-v1.26.0-rc.6-docker-terminal-capability`。apps main `6602ac36e0c329e35b74ff621721b23583e94f5f` 仅同步两行 CAP_KILL，配置 Git blob `9d2d2efdc334db1ba014203370fdc8281f1a5870`；公开 Release/镜像通过后才推送，Bash 语法、同配置独立生命周期和远端读回通过。默认入口保持 latest。
- 旧 RC5 仅换镜像或首次面板更新不保证修复 unit；必须从刷新后的生命周期显式更新预览目标，或迁移服务配置、daemon-reload 并重启 Agent。没有在健康/版本读取时隐式迁移权限。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或范围 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确四个源码 blob；来源 80 次正式 unit root/uid1000 终端及重启、输入/cd/resize/close；本候选完整 L3 | 本版未新增公开 Docker UI 全旅程；源码实机证据按相同 blob 继承 |
| 网络入侵与供应链安全 | 已验证 | 精确 nonce 拒绝、pidfd 边界；完整供应链扫描和公开镜像身份 | CF 未完成，既有 needs_validation 不关闭 |
| 稳定性、失败恢复与兼容 | 已验证 | 完整 Go/race/vet、生命周期/回滚与正式 unit recovery；旧 unit 刷新迁移测试 | 有限重复不能保证未来无启动竞态；旧安装须显式迁移 |
| 性能与资源预算 | 已验证 | 固定并发和超时、Runner/cgroup/OOM、磁盘/内存 watchdog | 无新吞吐/GPU基准；继承经典拖动 +25.485% 超20%预算 |
| 用户体验与可访问性 | 已验证 | 154 后台 8 项真实发布/API/弹窗、8 PNG逐张检查，摘要和提示 2/2 | 瞬时通知可暂遮弹窗标题，窄屏需滚动；实体手机、Safari/Firefox、屏幕阅读器未新增验收 |
| 数据、配置与迁移 | 已验证 | 两行能力集对齐、刷新脚本恢复旧 unit、原有备份/事务/回滚 | 用户部署未检查，不能由原截图认定唯一原因 |

## 自动门禁

- 完整入口：`scripts/run-release-l3.mjs --execute-kit`，run ID `v1.26.0-rc.6-0d43a08a-l3-r1`；候选/基线/Runner 冻结，`make verify-release`、源码并发组、10 个 Agent/轻量 Node/MCP 跨平台二进制、镜像构建/契约/扫描和 app-conf 生命周期通过。公开镜像 E2E 在发布后独立执行。
- manifest SHA-256：`eb4b7fcf15611b90547bc45920694c013e44528539a0f9cfa6865bb801ba5ec7`；Runner：`sha256:20667a9f1fad6590219ba8d8acdabc09a49f8683d4a877b847df17d28b83f8fb`。plan/script/bundle 原始摘要见 manifest.json，终态 passed/exit0；本地原件 `C:/GitHub/_release-evidence/v1.26.0-rc.6-0d43a08a-l3-r1`，所有 recovered evidence.sha256 已核验。
- 候选 CI：[CI #38097095979](https://github.com/kejilion/KPanel/actions/runs/38097095979)；[Dependency freshness #38097096245](https://github.com/kejilion/KPanel/actions/runs/38097096245)；main CI：[CI #38097739580](https://github.com/kejilion/KPanel/actions/runs/38097739580)；[Dependency freshness #38097739573](https://github.com/kejilion/KPanel/actions/runs/38097739573)；Release：[Release #38098504719](https://github.com/kejilion/KPanel/actions/runs/38098504719)；[Dependency freshness #38098504871](https://github.com/kejilion/KPanel/actions/runs/38098504871)。各结果绑定同一产品 SHA，未将观察中的状态当成功。
- 发布说明：草稿真实 Go 解析与 Release 最终正文门禁通过；公开正文/运行时/API/弹窗均为 RC6、2 条摘要和 2 条升级提示，展示上限仍为 8/3/280。
- 扫描：govulncheck 符号可达 0、导入包提示 0、模块级未调用提示 1；不将其写成全依赖零告警。npm audit 0，Trivy 在门禁扫描范围内为 0；完整原始日志保留。正式 workflow 配置生成 SBOM/provenance，公开 OCI 两项 attestation 条目存在；本次未逐个读取其 payload，未声称本地完成签名密码验证。

## 依赖与技术栈变化

- 未升级 Go/npm依赖图、Action、基础镜像、扫描器或受管脚本。采用既有 Go1.27.2/Node24.21.0 固定 Runner，版本字段更新 RC6。
- 精确 candidate/main/tag Dependency freshness 通过；检测源、生成时间与行动项保留在各自动报告及原始日志。未额外声称新的每日安全通告或 EOL 人工复核。
- 继承 v1.25.1 安全补丁、RC5 BT 与 offers/GPU/CF限制。当前/上一稳定回滚点保留。

## 隔离真机与浏览器验收

- arena-154：Debian13.7/Linux6.12.107+deb13-amd64/x86_64/Docker29.6.2；用途 candidate-validation、browser-validation。未连接 prod-108。
- 源码正式 unit：来源候选完整 sandbox，80 次 root/uid1000、前后 Agent 重启；Ctrl-C/HUP resistant close/disconnect/natural exit/process recovery 证据保留。未新增 native ARM runtime 或长时 soak，ARM 仅构建。
- 浏览器后台 job `arena-154-35392`：passed/exit0，超时420s，spec摘要 `628c23f32ff4d6b412f66cefd0ba04c508408ff8a3132f5522c702e128f0715c`；当前RC6/旧RC5/稳定1.25.1公开镜像，实际固定上游。记录和原件 `C:\GitHub\_release-evidence\v1.26.0-rc.6/public-notes-browser-r1/remote-evidence`。
- 8 项场景、8 PNG，桌面暗色和窄屏明/暗、键盘/Escape、滚动和字号/overflow；真实同源会话小文件、HEAD/Range/ZIP、FilesView及匿名/退出拒绝复验。没有自动安装请求、console/resource errors或OOM，fixture/container/network已清理。窄屏 PNG 是全页截取，内容需纵向滚动，实时视口中的末条升级提示边界由浏览器断言核验；加入/检查通知可暂遮部分弹窗标题，主要修复、权限迁移警告和操作仍可读，不宣称标题始终无遮挡。125%/200%原生缩放、长时公网和真实CA未新增验收。

## 发布产物与公开仓库复核

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.26.0-rc.6)：draft=false、prerelease=true、非 Latest，公开时间 2026-10-11T00:39:14Z。
- 版本/preview OCI index 均为 `sha256:83960073e7202ad6b73db193037e208cbe8d242c96bd4919fa9cf80ca3a2e52b`；amd64 `sha256:05057ee3aebd3bf632d676d163e8ddc894a746cd8a76e8f2916092fc4de35391`，arm64 `sha256:549ff38f073222b8148bdae5d5b5e722e87452900c00a0236cac0192de3c0334`。版本/源码/脚本标签匹配。
- GitHub Latest=v1.25.1，Docker latest=`sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`，均保持不变。
- 14 个附件、11 条 SHA256SUMS，API digest全部核验；meta VERSION和许可证字节已核验。未逐个下载所有二进制字节。
- 公开镜像实际 pull 并 `image_e2e=pass`；公开 Release 正文、真实发布API与更新弹窗相符，升级提示完整。

## 自更新通道验收

- 实际 stable/preview API选择与官方唯一镜像digest匹配；稳定来源选择预览只切换来源并立即检查，installRequests=0；安装开关与一次安装继续独立。
- 旧RC5来源向前检查RC6；当前RC6退出预览且稳定较低时不生成降级候选。旧默认stable迁移和重启通道持久化继承完整既有门禁。
- systemd事务备份/失败恢复/版本隔离通过生命周期门禁；OpenRC和轻量Node边界见[发布通道](release-channels.md)，本轮未扩展其Docker进程控制支持范围。

## 生产发布与隔离写入

不适用（预览版禁止生产部署）。本次未连接、备份、部署、升级、核对或清理 prod-108；仅在 arena-154 隔离验收，生产写操作为零。

## 回滚

- 上一预览 v1.26.0-rc.5/`sha256:1cab7f607c8ddc6fd84489210caf544fbceb5888a2d836efecaa43f02bfbf242`，稳定 v1.25.1/`sha256:1894b5e3aa76edbd75efef2de1164f44d38734cb3a27dc11aa2f75c420c1b325`，历史 tag/镜像不可变。没有生产升级，实际生产回滚不适用。
- 如用户后续回滚，保留安装/更新前事务备份并按既有生命周期恢复成对 Panel/Agent/配置，重新核对版本、健康和会话；本次未操作用户数据。回滚低于当前版本不会自动执行，默认公共入口继续 stable latest。

## 交付节奏数据

节奏基线：0d43a08ac53d4f493e4a32ea288b3ee2450be6d5，生成于 2026-10-11T00:47:12.116Z；仅使用已提交验收记录，不把标签时间解释为生产完成时间。本轮 RC 的交付与流程数据单独列在下方。

- 滚动 14 天：稳定标签 4 个，分布于 4 个自然日；有生产完成证据的部署 0 次，分布于 0 个自然日。
- 最近 20 个正式版本（实际 20 个）：验收记录 20/20，生产完成时间 13/20；变更失败有明确填报 20/20，缺失不推断为成功。
- 最近正式版本已记录流程异常 319 次，其中生产写后 33 次；滚动五版本重复指纹 21 个、未声明重复 0 个。完整字段和覆盖率见本轮 release-metrics-product-baseline.json。

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-11T04:35:13+08:00
- 候选冻结时间：2026-10-10T23:39:31.043Z
- 生产完成时间：未验证
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

已读取最近五个正式版本及 RC5/RC3/RC2/RC1 的流程明细；RC4 验收记录缺失，不推断为无异常。第一条沿用 RC2 的 Docker --format 空白参数运输指纹，属于五个 RC 窗口内复发观察；最近五个正式版本没有这一具体指纹。Buffer 原始字节误序列化与旧数组/对象序列化事件根因不同。原件摘要与比较结论见 process-history-review-qualified.json；本轮只计实际三次事件，不复制旧版本次数。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "diagnostics/ssh-remote-argv/docker-format-whitespace",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial read-only disk survey passed Docker format as split remote arguments; container-name subcommand failed. It was not used as a qualified resource inventory.",
    "recoveryEvidence": "Fresh inventory-resources.py, cache-inventory-summary.json and cache-cleanup-r1-output.json supply exact IDs plus unchanged container/image/volume inventories before the frozen resource check.",
    "permanentAction": "Release maintainer; review by 2026-10-18 or before next L3 production write. Same Docker --format whitespace reconstruction root cause as v1.26.0-rc.2; retain the existing fingerprint rather than introduce a new alias. Use structured Python subprocess argv for remote inventory and require each subcommand success; external correction is recovery only. Preview recurrence remains an observation and is not a stable historicalReleases declaration.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/apps-config/buffer-json-serialization",
    "position": "before-production-write",
    "count": 1,
    "impact": "Preparation wrote two raw-config evidence files through a JSON serializer. Actual in-memory two-line comparison and plan digests were correct; the files were corrected before target use or publication.",
    "recoveryEvidence": "apps-raw-artifact-correction.json and r2 exact Git-byte files, target Git blob equality, Bash syntax, disposable lifecycle and SSH main readback. Original misserialized artifacts retained.",
    "permanentAction": "Release maintainer; review by 2026-10-18 or before next L3 production write. Treat Buffer as bytes and bind copied artifact hash to the source before consumption in the shared artifact writer; external correction is recovery only.",
    "historicalReleases": []
  },
  {
    "fingerprint": "documentation/external-writer/line-ending-assumption",
    "position": "before-production-write",
    "count": 1,
    "impact": "A preparer correction expected CRLF after an earlier apply_patch normalized the file to LF. Node rejected the anchor before any file write; the subsequent independent syntax command exited zero, which is not accepted as correction success. Product, CI and immutable source were unaffected.",
    "recoveryEvidence": "Original Node error and combined native exit retained in tool chunk b85a35; no separate original log is fabricated. Saved correct-acceptance-preparer.cjs uses an explicit CRLF-or-LF anchor and a single native invocation, with independent inspection before docs generation.",
    "permanentAction": "Release maintainer; review by 2026-10-18 or before next L3 production write. Qualify line-ending-independent writer anchors and propagate the edit exit rather than a following command exit. This saved external correction is recovery only; no permanent shared-entry repair is claimed.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地来源工作树实际回收：56643584 字节（观测净空闲变化，非逻辑大小）。当前候选仅保留源码，候选自有 node_modules 清理：263409664 字节观测净回收；可按锁文件重建。137条已授权 Docker 构建缓存实际净回收 3545067520 字节；容器/镜像/卷清单未变。原始失败、审计、截图和完整本地 L3 恢复 bundle 保留。
- 本轮文档/apps临时工作树在 main/归档精确读回后回收；154本轮自有 staging/inbox/重复bundle及草稿可再生目录在门禁结束后回收，实际闭环回执保存在 `C:\GitHub\_release-evidence\v1.26.0-rc.6`，不得将计划大小写成已释放。
- 六个此前 policy-blocked 历史工作树和其他活跃/未知所有权作者资源保留；当前 release/v1.26.0-candidate 保留。归档/本地清理不代表生产上线。
- 稳定前：补齐CF critic/verifier/Phase5、SSE撤权/轻量节点撤权/private magnet DHT needs_validation；RC3缓存清理独立关闭、offers后台刷新/TempDir竞态（2026-10-15前或稳定前）、GPU准入（2026-10-16前或稳定前）继续开放。
- BT持续载荷后停滞、原RC4 CI唯一根因、私有magnet/PEX/v2/hybrid/做种/恢复/WAN/10GiB、原生ARM/实体手机/更多浏览器/屏幕阅读器等未新增验证。有限nonce重复仅证明记录内场景；Agent停止时仍无独立清理守护，分离后台任务可能继续运行。
- 本版仅为预览修复，保留上述风险并披露旧安装权限迁移，稳定提升须重新执行所需补审与准入。
