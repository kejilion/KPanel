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
