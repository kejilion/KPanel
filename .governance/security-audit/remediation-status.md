# 安全审计修复与交付追踪

本记录区分源码修复、独立复核、RC 交付、稳定版交付和部署；执行标准见 `PROJECT_RULES.md` 5.4。
本次核对日期：2026-09-19。审计历史原文不改写。

## run-1 已公开发现

- fingerprint：`hostbackup.restore.payload-root-unconfined-to-data-model`。
- 原始审计：run-1，源码基线 `6340e0783d3e57873fd2c93a964372aefb9e1a81`；该次 dirty 状态缺完整快照。
- 修复提交：`e99e6b3d225425aa0b9afd14c55c8a33d75722f9`；公开主线已包含。
- 当前源码复核基线：`23cbb9979ce0df84801b9fe614bd1b00d7ad8a99`，与 RC.3 的业务代码一致。
- 复核状态：run-2 hunter 与独立覆盖复核均确认原路径已由模块绑定和目标真实清单校验约束，
  原 fingerprint 为源码层面已修复；未重新执行动态回归，不据此宣称全部备份行为安全。
- 回归用例：`internal/hostbackup/backup_test.go` 的 `TestBackupPayloadRootScopeValidation` 和
  `TestBackupRestoreRejectsRootOutsideDestinationData`；本次只读审计未执行目标代码。
  既有发布级执行证据见 `docs/release-v1.20.0-rc.1-acceptance.md` 至 rc.3 验收记录，不能替代新正式候选 L3。
- RC 已交付：`v1.20.0-rc.1`、`v1.20.0-rc.2`、`v1.20.0-rc.3` 均包含修复，公开 Release 均为 prerelease。
- 稳定版状态：`pending-stable`；2026-09-19 GitHub Latest 为 `v1.19.0`，其 tag 不包含修复。
- 部署状态：本任务未验证、未执行生产部署；不得用 RC 发布记录推断正式部署。

## 后续审计与正式发布责任

run-2 为当前候选的 scoped 源码增量审计：16 个当前范围单元、46 个范围外单元；
4 条独立复核的待验证线索、0 条新增确认漏洞。完整记录保存在本地候选
`docs/security-audit-run2-local-20260919` 的 `.governance/security-audit/run-2/`，
待验证攻击细节不先期推送；此分支不是公开远端依赖。
待验证不等于已确认，也不等于已排除；不能用“审计运行完成”替代“候选可发布”。
本轮未建立完整 OS 执行沙箱，产品代码、测试和动态复现均未执行；每条记录附有具体离线验证计划。
对应版本唯一发布任务负责在新候选冻结时核对未决问题，运行精确候选 L3，并在正式验收中补齐
stable tag 祖先关系、公开 Release、镜像及适用的部署证据。

`release/v1.20.0-candidate` 当前仍服务预览列车，保持不变。正式版未发布时不执行归档或删除。
发布成功后的处置统一引用 `docs/release-channels.md`；分支归档改进候选
`docs/branch-archive-lifecycle-20260919` 需先完成集成，不能将未集成规则描述为已在主线生效。
