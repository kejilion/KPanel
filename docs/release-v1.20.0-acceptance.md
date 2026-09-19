# KPanel v1.20.0 发布验收记录

日期：2026-09-19

发布级别：L3

候选提交 / 标签：`c98727c898f8b446cceea5d7b10f3205340926a2` / `v1.20.0`

上一稳定版本 / 回滚点：`v1.19.0` / `sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`

`releaseChannel`：`stable`

`releaseTrain`：`1.20.0`

候选分支与发布后处置：`release/v1.20.0-candidate` / 稳定版归档

- 原分支 / 精确 tip / 处置分类：`release/v1.20.0-candidate` / `c98727c898f8b446cceea5d7b10f3205340926a2` / absorbed。
- 归档 ref 与 SHA / 远端复核结果：`archive/release/v1.20.0-candidate` / `c98727c898f8b446cceea5d7b10f3205340926a2`；活动候选远端已不存在，归档 ref、stable tag 与发布提交一致。
- 本次来源任务分支：审计规范与交付系列 `76af5a1b..cda7dd3e`、候选归档系列 `2774212b..fbe41d4a`、安全修复 `7836ae8e` 和审查证据 `ea995703` 全部纳入；原治理分支已分别归档为 `archive/docs/audit-evidence-lifecycle-20260919`（`f1119fbc`）和 `archive/docs/branch-archive-lifecycle-20260919`（`43a29fb6`），活动引用已删除；与本版无关的 `fix/release-metrics-chronology-20260919` 未纳入。
- 本地分支/upstream/worktree：发布工作树和本地发布分支在验收提交完成后回收；L3、OCR、公开 OCI 与生产证据保留在仓库外证据目录和 arena-154。
- 未完成归档项 / 责任人 / 下次复核触发条件：无；下一候选从发布后的 `main` 新建，不复用本分支。

归档不代表生产上线；本版生产状态在“生产部署安全核对”独立记录。

## 发布画像

- 业务域：审计存储、终端与文件/站点运维、备份恢复、安全审计与发布治理。
- 变更面：展示、只读、宿主机写入、协议或数据、部署。
- 受影响用户旅程：审计查询与迁移、登录限流、备份保存/恢复、终端批量操作、文件与站点管理、稳定版自更新、候选分支归档。
- 未变化契约：既有端口、Compose 入口、Agent 权限和 `kejilion.sh` 契约；`scriptLinkageState=not-required`。
- 风险等级及理由：中等偏上；包含 SQLite 数据迁移、宿主机写入和生产升级，由 rc.1-rc.3 渐进验收、稳定候选 L3、公开 OCI E2E、停写备份与生产 postdeploy 证据覆盖。

## 发布范围与未纳入内容

- 用户可见更新：审计历史独立 SQLite 存储；备份、终端、文件、站点、应用、初始化与国际化修复；稳定版候选归档和安全审计闭环。
- 精确提交清单：以 `git log v1.19.0..c98727c898f8b446cceea5d7b10f3205340926a2` 为完整可复现清单；关键修复和治理提交在上方来源任务字段及 `CHANGELOG.md` 中列出。
- 明确未纳入的分支、文件或后续事项：本地含待验证攻击细节的 run-2 原始审计材料未推送；无关的发布指标时序分支未纳入；没有应用市场或受管脚本业务变更。

## 外部审计与修复交付

- 安全审计 run / 精确源码基线 / 范围与未覆盖项：run-2 scoped 审计基于 `23cbb9979ce0df84801b9fe614bd1b00d7ad8a99`，覆盖 16 个当前范围单元，46 个范围外单元明确留档；原始细节保存在本地候选，未提前公开攻击材料。
- finding / 修复 commit / 独立复核与回归证据：独立复核提出 4 条待验证线索、0 条新增确认漏洞；`7836ae8ef968a618bd950b8fd46f03bef29fa6a2` 对脱敏、审计保留、登录容量和备份实际编码上限补充最小修复与回归测试；OCR 精确范围复核为 H0/M0/L1，LOW 在冻结前修正。
- 修复交付状态：源码已修复；RC 交付含 run-1 修复；stable tag `v1.20.0` 包含 run-1 与 run-2 后续修复；GitHub Release、正式 OCI 和 arena-154 部署均已核对同一 revision。
- OCR 观察区间 / 适用候选计数及口径 / 有效、skipped、unreported 数 / constrained-only：按 `v1.19.0..v1.20.0` 中含 `OCR-Review:` trailer 的候选检查点计 10 个，有效 5、skipped 5、unreported 0；三个完整记录 constrained-only=0，两个早期记录该字段为 unreported，未把缺失解释为零。
- 本稳定周期观察结果：open-code-review 能发现可复核的局部问题，但只作为评审辅助；security-audit-skill 用于范围与证据结构。两者的结论均经过源码核对、回归测试、L3 和发布门禁，不直接替代验收；观察周期不足三个稳定版，不提前宣称长期有效或退出。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本）。
- 变更集编号：不适用。
- KPanel 实际内置脚本基线 commit / SHA-256：`6ebb945f6d5cb69fdb41e3761de23566acbaf762` / `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`。
- 脚本候选 commit / SHA-256：不适用。
- 状态判定依据与兼容性证据：未改 KPanel 与 `kejilion.sh` 的接口；L3 `managed_script_contract=pass`、应用生命周期和公开镜像 E2E 通过。
- 本版发布决定：脚本不在范围。
- 阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 全量 Go、前端 1436 项、应用生命周期、公开 OCI E2E、生产 postdeploy 均通过。 | 未执行跨公网大规模多节点长期运行。 |
| 网络入侵与供应链安全 | 已验证 | govulncheck 0、npm audit 0、Trivy 源码/配置/镜像 0；OCI revision/digest、受管脚本摘要和 Release 附件均核对。 | 范围外 46 个审计单元未在 run-2 重新全量审计。 |
| 稳定性、失败恢复与兼容 | 已验证 | privileged core race 通过，应用安装/更新/中断回滚测试通过；生产更新前停写备份可校验，更新无回滚。 | 未做 4 小时生产 soak 与真实断电。 |
| 性能与资源预算 | 已验证 | 审计满量基准及 L3 构建无超时/OOM；生产采集 `resources.json`、`df.txt`。 | 未采集真实规模节点长期 P95 曲线。 |
| 用户体验与可访问性 | 已验证 | 162 个前端测试文件、类型检查、3060 条 i18n phrase 和受影响布局测试通过；本版稳定候选相对 rc.3 的用户界面只含已验收源码。 | 未重复全部真机浏览器视口；沿用 rc.1-rc.3 浏览器验收。 |
| 数据、配置与迁移 | 已验证 | 审计迁移/重复回放回归、备份边界与实际大小回归、生产 SQLite quick_check、受保护配置 diff=0。 | 未执行真实断电中的 SQLite 拷贝。 |

## 自动门禁

- 定向测试及结果：`internal/redact`、`internal/auth`、`internal/store`、`internal/backup` 新回归随全量 Go 测试通过；治理脚本 194 项通过。
- `make verify-release` 环境和结果：arena-154 `kpanel-release-gate:go1.26.7-node24`，完整 L3 通过；Go 全量、前端 162 文件/1436 项、race、漏洞扫描、双架构构建、镜像和应用生命周期均成功。
- L3 外层入口：run `v1200-stable-c98727c-l3-r1`；plan SHA `5352073100b700a6c78db43f426f5bed7ab99fc08c328eb181e2e373cbaa5971`，remote script SHA `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle SHA `508b41850cb46f4d9e709e1157cc7181d2e01084b9ad5296234f6cbbe7d4a245`，Runner ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，终态 pass；本地证据 `C:\GitHub\_release-artifacts\v1200-stable-c98727c-l3-r1`。
- 候选 CI：[run 35436283334](https://github.com/kejilion/KPanel/actions/runs/35436283334) success；依赖新鲜度 run 35436283321 success。
- 主线 CI：[run 35436618922](https://github.com/kejilion/KPanel/actions/runs/35436618922) success；依赖新鲜度 run 35436618917 success。
- Release workflow：[run 35436961485](https://github.com/kejilion/KPanel/actions/runs/35436961485) success。
- 安全扫描、镜像契约、SBOM/provenance：源码和镜像 Trivy 0，govulncheck/npm audit 0；OCI index 含 amd64/arm64 与 attestations，revision/version 标签通过；Release 附件含校验清单。

## 依赖与技术栈变化

- `make dependency-report` 与检测源完整性：tag、候选和主线的 Dependency freshness 工作流均成功；11 个依赖组策略检查通过。
- 最近每日安全通告审计、EOL 复核状态：治理严格检查 `proposals=15`、overdue=0、unclassified=0；依赖策略与 EOL 例外无本版阻断项。
- 直接/基座行动项：本版没有新增运行时直接依赖；open-code-review pin 1.12.6 仅为本地评审工具，security-audit-skill 上游只作为 scoped comparison 候选信号。
- 本版采用的工具链：Go 1.26.7、Node 24.20.0、固定 digest 基础镜像与 Actions SHA、Trivy 0.72.0、govulncheck 1.6.0、open-code-review 1.12.6。
- 暂缓或拒绝候选：无阻断本版的依赖升级；范围外审计单元按后续 full audit 触发条件处理。
- 升级结论：锁文件、构建、扫描、双架构镜像、应用生命周期和生产回滚入口均通过。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：arena-154，Linux/amd64，Docker，固定 L3 runner Go 1.26.7 / Node 24.20.0。
- 环境策略 ID 与允许用途：`arena-154` / hybrid；`candidate-validation`、`production-deploy`、`production-safety-check` 分别通过策略检查。
- 使用的精确候选或公开产物：`c98727c898f8b446cceea5d7b10f3205340926a2`；`docker.io/kjlion/kejilion-panel@sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- 后台作业和证据：L3 run `v1200-stable-c98727c-l3-r1` exit 0；公开 OCI E2E r1 exit 0，日志 SHA `d6918030f72af42d7c376ceb7a1f902705a8473b11ff4ccacb39706e81322eb9`，固定入口 SHA `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，远端证据 `/root/kpanel-release-evidence/v1.20.0/public-oci-e2e-r1`。
- 测试窗口/循环数及风险依据：一次稳定候选完整 L3、一次公开 OCI 冷启动 E2E、一次生产更新；长期 soak 不适用，风险由三个 RC、竞态测试与生产监控覆盖。
- 用户旅程与视觉：沿用 rc.1-rc.3 已记录的桌面/移动端、缩放、主题、键盘、语言与失败态验收；稳定候选新增内容为安全修复和治理，不引入新 UI。
- 宿主机写入、失败注入、重启恢复和回滚结果：L3 应用生命周期覆盖安装、更新、中断恢复与卸载；生产更新前停写备份，更新后容器 restart=0、OOM=false。
- 未执行场景及原因：未做真实断电和 4 小时生产 soak；未在禁用主机 `prod-108` 执行任何动作。

## 发布产物与公开仓库复核

- GitHub Release 的 draft / prerelease / Latest 状态：`v1.20.0` draft=false、prerelease=false、Latest；发布时间 2026-09-19T18:25:55+08:00。
- Docker 版本与通道 OCI index：`1.20.0` 与 `latest` 均为 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`；`preview` 保持 `sha256:76b54033a5f58a614ae121700539b3891f3a8d2554939cd24fa63c501506a2ba`。
- `linux/amd64`、`linux/arm64` digest：`sha256:f66d66e88033fbb698d4b413b8f3b069ff4bf23b037cf8610887dec0d4f7f5de` / `sha256:ec3fb019dbf3d8ae0b669b86e74efdfe68ec5caec653cfb0a956c2d3a34d32df`。
- 附件及 `SHA256SUMS`：Agent amd64/arm64、Node amd64/arm64、部署包、LICENSE、THIRD_PARTY_NOTICES.md、SHA256SUMS 均存在。
- 公开镜像 `image_e2e=pass`：arena-154 显式 pull `1.20.0` 后以仓库固定脚本通过，无残留测试容器。
- `kejilion/apps` / `kejilion.sh` 契约结论：`C:\GitHub\kejilion\apps` clean 且与 `origin/main` 同步，`kpanel.conf` 与发布包逐字业务内容一致，无需提交；受管脚本摘要匹配。

## 自更新通道验收

- 稳定来源选择正式 GitHub Latest，预览来源选择规范稳定版或 RC，并校验唯一官方镜像 digest：已验证。
- 加入预览只切换来源并立即检查，没有自动安装：rc 系列已验证；稳定版未改变契约。
- 自动安装开关与一次性立即安装相互独立：已验证。
- 旧状态默认迁移到 `stable`，重启后通道选择保持：已验证。
- 退出预览且稳定版较低时没有产生降级候选：已验证。
- systemd 后台执行、更新前备份、失败恢复和失败版本隔离：L3 与生产停写备份已验证。
- OpenRC 与轻量 Node 的当前边界按 `docs/release-channels.md` 呈现：已验证。

## 生产部署安全核对

- 生产目标和部署授权范围：用户明确授权稳定版上线；仅 arena-154。
- 验证/灰度环境：arena-154（hybrid）。
- 正式部署环境：arena-154。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight 2026-09-19T18:03:52+08:00 至 18:03:54+08:00，版本 1.19.0、健康通过；backup 18:29:19 至 18:29:30 通过，路径 `/root/kpanel-backups/pre-v1.20.0-20260919T102919Z`，数据、旧镜像、Agent unit、kpanel.conf 和 SHA256SUMS 均校验成功，恢复后配置 diff=0。
- 部署命令/入口：`env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy 18:30:26 至 18:30:28 通过；version=1.20.0、revision=`c98727c8`、RepoDigest 含正式 index、Agent active、容器 healthy/restart=0/OOM=false、近 10 分钟无 panic/fatal、配置 diff=0；`panel/ai.db` 与 `panel/panel-state-audit.db` quick_check ok；公网健康 18:30:55 返回 status=ok/version=1.20.0。
- 生产已执行写操作：是；标准应用市场更新成功，无回滚、无紧急热修复、无重复发布。
- 仅在隔离真机执行、未在生产执行的场景：L3 故障注入和公开 OCI 冷启动 E2E。

## 回滚

- 源码/tag：`v1.19.0` / `031e7b2616c383f006a583e44426f7948a53c8c7`。
- 镜像 digest：`sha256:351a236f4a246affd9d4cc045808ac1f8625de4482c74abfc5756cb89077b9ce`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.20.0-20260919T102919Z`，含 `old-image.tar.zst`、数据、服务配置与校验清单。
- 回滚步骤和回滚后复核：由标准应用市场事务恢复备份中的旧镜像和数据，再按 postdeploy 同口径核对 version/revision/digest、Agent、容器、配置和 SQLite。
- 回滚后生产实际版本与健康状态：不适用，本次未触发回滚；当前 1.20.0 健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：均指向 v1.20.0 / `sha256:a991b5d2…`。
- 公共默认更新通道决策：不适用；本版生产成功并保留为稳定默认。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-18T00:14:07+08:00
- 候选冻结时间：2026-09-19T17:44:35+08:00
- 生产完成时间：2026-09-19T18:30:28+08:00
- 提交到生产用时：42.27 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "version-check/authoritative-entry/wrong-filename",
    "position": "before-production-write",
    "count": 1,
    "impact": "冻结前误调用不存在的 scripts/check-version-consistency.mjs，Node 立即失败；未改变候选、远端或环境状态。",
    "recoveryEvidence": "改用仓库固定入口 scripts/check-version-consistency.sh 后输出 Version metadata is consistent: 1.20.0；同一候选 L3 再次通过该检查。",
    "permanentAction": "版本一致性只调用 verify-change.sh 所引用的 scripts/check-version-consistency.sh；发布执行以 release-kpanel workflow 渲染的固定入口为准。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：公开 E2E 临时容器由固定脚本清理；发布工作树和本地发布分支在验收提交推送后回收。保留 `C:\GitHub\_release-artifacts\v1200-*`、OCR 外部证据、arena-154 L3/生产/公开 OCI 证据与可恢复生产备份；不删除历史 tag、Release、归档 ref 或不可变镜像。
- 未验证风险：46 个 run-2 范围外单元未做本轮 full audit；未执行真实断电、长时生产 soak 和全量稳定候选浏览器矩阵。
- 已实现待实机准入：无阻断项；上述长期/范围外项目按触发条件进入后续专项。
- 不阻断本版的理由：三个 RC 渐进验收、稳定候选完整 L3、候选/main/tag 三层 CI、公开 OCI E2E、stable Release、候选归档与生产三阶段证据全部通过；生产公网入口返回 1.20.0 healthy。
- 后续应进入的自动门禁或专项工作流：继续累计 security-audit-skill 与 open-code-review 至至少三个稳定周期再评价保留/调整；下一候选必须从当前 `main` 新建并沿用归档门禁。
