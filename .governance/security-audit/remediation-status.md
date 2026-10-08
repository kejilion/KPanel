# 安全审计修复与交付追踪

本记录区分源码修复、独立复核、RC 交付、稳定版交付和部署；执行标准见 `PROJECT_RULES.md` 5.4。
本次核对日期：2026-09-20。审计历史原文不改写。

## run-1 已公开发现

- fingerprint：`hostbackup.restore.payload-root-unconfined-to-data-model`。
- 原始审计：run-1，源码基线 `6340e0783d3e57873fd2c93a964372aefb9e1a81`；该次 dirty 状态缺完整快照。
- 修复提交：`e99e6b3d225425aa0b9afd14c55c8a33d75722f9`；公开主线已包含。
- 当前源码复核基线：`c98727c898f8b446cceea5d7b10f3205340926a2`（`v1.20.0`），包含稳定版候选对后续 4 条待验证线索的修复。
- 复核状态：run-2 hunter 与独立覆盖复核均确认原路径已由模块绑定和目标真实清单校验约束，
  原 fingerprint 为源码层面已修复；未重新执行动态回归，不据此宣称全部备份行为安全。
- 回归用例：`internal/hostbackup/backup_test.go` 的 `TestBackupPayloadRootScopeValidation` 和
  `TestBackupRestoreRejectsRootOutsideDestinationData`；本次只读审计未执行目标代码。
  既有发布级执行证据见 `docs/release-v1.20.0-rc.1-acceptance.md` 至 rc.3 验收记录，不能替代新正式候选 L3。
- RC 已交付：`v1.20.0-rc.1`、`v1.20.0-rc.2`、`v1.20.0-rc.3` 均包含修复，公开 Release 均为 prerelease。
- 稳定版状态：`delivered`；`v1.20.0` 为非 prerelease 的 GitHub Latest，tag 指向上述复核基线，
  正式 OCI index 为 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- 部署状态：`arena-154` 已通过受控更新入口部署 `v1.20.0`，postdeploy 核对版本、revision、digest、
  Panel/Agent 健康、配置哈希、SQLite quick_check 和致命日志均通过；`prod-108` 未连接或操作。

## 后续审计与正式发布责任

run-2 为当前候选的 scoped 源码增量审计：16 个当前范围单元、46 个范围外单元；
独立复核提出 4 条待验证线索、0 条新增确认漏洞。4 条线索已在稳定版候选
`7836ae8ef968a618bd950b8fd46f03bef29fa6a2` 中分别补充最小修复和回归测试；同一稳定候选
`c98727c898f8b446cceea5d7b10f3205340926a2` 已通过 arena-154 L3，定向包、全量测试、竞态测试、
前端检查、漏洞扫描和发布构建均通过，因此这 4 条线索转为已关闭。完整审计记录保存在本地候选
`docs/security-audit-run2-local-20260919` 的 `.governance/security-audit/run-2/`，
待验证攻击细节不先期推送；此分支不是公开远端依赖。
待验证不等于已确认，也不等于已排除；不能用“审计运行完成”或源码修复替代“候选可发布”。
审计阶段未建立完整 OS 执行沙箱；每条记录附有具体离线验证计划，发布任务将通过 arena-154 L3
执行定向测试、全量测试、竞态测试、前端检查和发布构建。
对应版本唯一发布任务负责在新候选冻结时核对未决问题，运行精确候选 L3，并在正式验收中补齐
stable tag 祖先关系、公开 Release、镜像及适用的部署证据。

`release/v1.20.0-candidate` 已在稳定版发布成功后归档到
`archive/release/v1.20.0-candidate`（`c98727c898f8b446cceea5d7b10f3205340926a2`），活动候选分支已删除。
处置统一引用 `docs/release-channels.md`；归档 ref 与稳定 tag 共同提供恢复入口。

## MCP 本地候选（run-3）

新增 MCP 范围的只读审计在独立验证阶段被平台中止；仅保存
[`run-3/REPORT.md`](run-3/REPORT.md) 与运行身份，不宣称审计通过或完整覆盖。
产品代码的普通开发验证与该中止审计分开记录，见 [`docs/mcp-access.md`](../../docs/mcp-access.md)。
MCP 产品源码已随 `v1.21.0-rc.1` 交付为公开 RC：tag 指向
`679b39489824bf281fd8570d6d95d42d10c54085`，GitHub Release 为 prerelease，公开 OCI index 为
`sha256:3add580bfd52fe25224de54782ddc52eeb5b87ba34a9af275e51cb131a3a5c21`，并通过
`arena-154` L3、候选/主线/Release 门禁和公开镜像 E2E。稳定版尚未交付，生产未部署。
来源分支精确 tip `291a646387a61f271f6f3c774a8d7980c55d0813` 已保存到
`archive/feature/mcp-access-20260919`；run-3 仍保持“未完成、无审计结论”，不能因 RC 发布改写为通过。

完整管理扩展已随 `v1.21.0-rc.2` 交付为公开 RC：tag 指向
`8a1998b39e07908f278e6abbb8f934f2722ac0d2`，公开 OCI index 为
`sha256:1836a60ffd9a39379ff70f43202a4006cbaead272d8571b36067bdb5cf57f4fd`。该候选通过新的
`arena-154` L3、候选/主线/Release 门禁和公开镜像 E2E；新增结构化写操作继续受服务端授权、
审批、资源版本、容量和并发预算约束。来源 tip `0dceeaf30454d5f57e9f51aca8aa8c94d7544304`
已保存到 `archive/feature/mcp-complete-20260919`。稳定版仍未交付，生产未部署，run-3 状态不变。

## v1.25.0 发车前未决项核对（2026-10-09）

发布负责人在 `50d3d75bd15dc0e152694adcf7f62276d238871f` 核对了公开 run-9/10/16、
仓库外 run-23（`5194858b`，source-only complete）及另一任务的 run-24
（`5eeecd4b`，incomplete）。后两者未作为当前候选的完成覆盖入库；原结论和原件不改写。
run-16 的候选账本和 run-23 的 12 条 `needs_validation` 均不等于已确认漏洞，也不等于风险关闭。
逐条源码比较、缺失事实和原件摘要保存在本版本外部验收证据。

| 未决项或相关 fingerprint | 当前处置及缺失事实 | 负责人 / 下次复核 |
| --- | --- | --- |
| `auth-state-dir-fsync-error-revives-revoked-session` | `open`。优先独立复核当前逻辑提交与持久性契约；现有逻辑以 rename 为提交点，尚无支持文件系统上的崩溃恢复证据。不能将未执行故障注入写成排除，也不宣称立即提交包含断电保证。后续仅用离线临时存储和合成会话验证。 | 发布负责人接手证据，认证维护者接手离线验证 / 2026-10-12 |
| `terminal-sse-output-after-session-revocation` | `open`。概述现已对齐既有详细协议：活跃复核最多缓存 1 秒，空闲在下一次心跳复核，到期定时结束，已发送字节不可撤回。源码覆盖与文档对齐不证明真实浏览器接收时序；若需要更强撤销契约，先独立立项并验证，不能暗改现有契约。 | 终端维护者 / 2026-10-12 |
| `kpanel:rollback-restores-revoked-light-node`；历史恢复与撤销线索 | `open`。回滚快照与后续撤销的交错尚未用合成节点观察；现有通用备份恢复用例不能代替鉴权结果。升级或恢复期间避免并发管理凭据，恢复后重新核对有效节点；在隔离临时状态验证后再处置原 fingerprint。 | 更新与集群维护者 / 2026-10-12 |
| `filemanager.protected-parent-symlink-toctou`；`hostbackup.restore.mutable-parent-symlink-race` | `open`。低信任本地身份是否有父目录替换权限、Go/OS 根目录约束及具体交错仍未验证。维持受保护路径、宿主机 ACL 与既有审批，不外推为任意宿主机写入。最小计划为离线合成目录、固定同步屏障与越界拒绝观察。 | 文件与备份维护者 / 2026-10-12 |
| `appmarket.update-drops-host-ip-binding` | `open`。当前相关源码与原记录一致；需要离线合成 Compose 配置及 Docker 实际绑定观察，不能假定部署端防火墙存在或不存在。自定义绑定的用户在更新前后核对有效绑定，生产未执行此验证。 | 应用市场维护者 / 2026-10-12 |
| TOTP、登录尝试、Passkey 与 MCP 容量类线索 | `open`。各入口的攻击者先决权限、速率与真实共享服务影响尚缺有界动态证据；保留已有认证、CSRF、速率和容量上限，不以容量存在即定性为鉴权绕过。最小计划为禁外网、合成会话和有限请求数的隔离验证。 | 认证与 MCP 维护者 / 2026-10-15 |
| `backupremote.credentials.allow-http`；HTTP 初始化、WebDAV 及 MCP localhost 解析类线索 | `open`。run-30 / `5eeecd4b` 保留 WebDAV 线索为 `needs_validation`；部署网络可达性、TLS/端点选择或本机解析控制权是缺失事实。继续遵守 HTTPS、可信回环与明确私网配置。源码不能代替部署网络证据，且本次没有生产检查授权。最小计划为离线假端点与合成凭据，或后续经授权的非生产拓扑核对。 | 安装、备份与 MCP 维护者 / 2026-10-15 |
| AI 响应聚合、分享缓存、通知轮换与 Telegram 发现类线索 | `open`。需要提供方字节上限、既有缓存时效合同或合成接收端交错证据；缺事实不推断为泄漏或无影响。维持现有资源限额和接收端显式配置，再用有限离线输入验证。 | AI、分享与通知维护者 / 2026-10-15 |

本次处置是发车前按可达性、权限影响与缺失事实的分级接手，未将任何未决项改为 `accepted-risk`、
`refuted` 或 `fixed-and-tested`。当前仍无独立验证的新增高风险结论；明确的当前边界违规或高风险结果一经验证，
按质量标准阻断发布或进入修复；覆盖检查 `ok` 不能替代该决定。上述日期是下一次证据复核期限，
不是审计豁免、漏洞风险接受或未来自动执行授权。

本次在干净的 `5eeecd4bac16ae4fccec5039863e476ad2054004` 源码完成两份限定范围的 source-only
审查记录：run-29 仅登记 `3507b50826f1e49b1ff5079c9557a93fb5bb02e5` 的六条改动路径，run-30
仅登记 `14dac0f7a3d9f48e38331cc3770bc5cd9a36e23a` 的八条改动路径；两者均保留
`scope_complete=false`，不覆盖周边提交或未完成的历史 run。最新官方结构校验及独立记录核验通过，
三条 `needs_validation` 仍未关闭，壁纸记录保持 `rejected`。入库件只含覆盖账本、来源和安全摘要；
完整发现记录及 P3/P5 原件仍保存在仓库外。上述核验没有运行目标程序或进行动态故障注入，
不构成整个 KPanel 的安全通过结论；后续处置继续按上表的负责人、复核日期和验证条件执行。
