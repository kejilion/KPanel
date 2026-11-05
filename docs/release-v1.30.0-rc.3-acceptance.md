# KPanel v1.30.0-rc.3 发布验收

日期：2026-11-05；发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.30.0`。

冻结候选 `a12dd7516190453a3ddb5937ab1cb2c847d0500a` / 标签 `v1.30.0-rc.3`；[公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.3) 为 prerelease，含 19 个附件。版本和 `preview` 镜像摘要 `sha256:fd3ddb98dfa654d7466e33f9d01f05be92aba4fc859bf522b7b8df9261c48398`。GitHub Latest 和稳定镜像仍为 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。发布前上一预览为 `v1.30.0-rc.1` / `sha256:931237c8c2751e058cb45d1e3ccb4f085489e3c9b4f7cc12239fc45dac926994`。

## 范围、兼容与风险

图片、音乐、影视目录和音乐播放位置随面板偏好同步，播放可手动恢复；优化音乐播放状态、系统媒体控制、元数据编码、桌面窗口、图标和滚动布局。偏好使用现有认证、乐观并发和文件接口。回退旧版前应恢复升级前备份，旧版对新增偏好字段的保留未验证；退出预览不会自动降级。

组合候选 `f7728b8d` 后接滚动条 `a6444554` 和图标 `f9d0150f`；整合来源通过精确 cherry-pick 映射和差异复核纳入。`scriptLinkageState=not-required`，无需发布脚本（不适用）：镜像内脚本 `d2156444add457973d4a84634653aa10cece712f`，SHA-256 `f60a7023ca0d6dd9ee5f3557669675b84ca7e2c0f3de9fe4b8da22e740478235`，无新脚本协议。原有媒体浏览器格式限制、真实多设备、宿主差异和物理 ARM64 未由 mock 证明。

## 门禁、产物与用户旅程

- [候选 CI 41994365403](https://github.com/kejilion/KPanel/actions/runs/41994365403) 和 [发布运行 41996328243/1](https://github.com/kejilion/KPanel/actions/runs/41996328243) 的候选 CI、GitHub-hosted 完整 L3、主线 CI、Release、双架构与公开验收均成功；可复用工作流内第二个 L3 为 skipped，因为同一次前置 L3 已通过。
- artifact `public-41996328243-1` 中公开验收 `passed`，绑定精确候选、版本、镜像摘要，核对 19 附件、15 项校验和、4 项摘要及 2 项升级提示；`snapshot-41996328243-1` 保存发布前稳定状态和发布身份。真实浏览器/隔离容器验证公开版本接口、更新弹窗和通道行为；不能外推为生产或真实 systemd 宿主验收。
- 本地 mock acceptance 绑定冻结 SHA，覆盖音乐布局、目录偏好、滚动播放栏和桌面入口；仅证明模拟数据范围。原候选本地 L2 因 Docker socket 权限未完成、Windows 本地验证有权限环境失败，未记为通过；以精确候选的 GitHub CI/L3 结果分别记录，不称本地 L2 通过。
- 随机播放 220 首跨越有界历史的测试在 rc.2 发布源码检查中耗时 5.217 秒超过默认 5 秒；原运行 `41988787529/1` 保留。rc.3 只将该测试超时设为 15 秒，保留全部 220 首及断言；本地该测试文件 18/18 通过，rc.3 完整源码组、L3、CI 与发布验证通过。慢速原因未完全确定，不推断为产品性能问题已解决。

## 分支、部署与回滚

`release/v1.30.0-candidate` 保留用于同一 RC 列车；本地来源及工作树保持原状，未执行重要删除。`v1.30.0-rc.2` 标签指向 `b8199bb7`，因发布前测试失败无公开 Release 和镜像，不覆盖或删除；`rc.3` 递增发布。源候选详细身份及本地验收证据保存在 `C:/GitHub/_release-evidence/v1.30.0-rc.2` 和 `v1.30.0-rc.3`。测试辅助分支 `fix/rc2-preview-mock-qualification` 精确提交 `3ff84f62` 已纳入 main，保留待授权归档；未删除其他工作树或远端分支。

生产部署不适用：预览版禁止生产部署；本次未连接生产或进行生产写入。人工回退需要先备份，使用上述稳定版不可变摘要及升级前备份，并核验恢复；不使用预览开关隐式降级。未清理其他本地资源。

## 发布节奏

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-05T10:11:26+08:00
- 候选冻结时间：2026-11-05T16:25:04+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {"fingerprint":"candidate-ci/business-review/stale-baseline","position":"before-production-write","count":1,"impact":"候选业务复核超出 50 提交窗口，CI 在测试前拦截。","recoveryEvidence":"41984649247 后更新复核并由 41984945033 通过。","permanentAction":"发布负责人按现有 50 提交门禁检查并更新事实基线。","historicalReleases":[]},
  {"fingerprint":"preview-dispatch/automation-check/candidate-only-mock","position":"before-production-write","count":1,"impact":"候选含尚未进入主线的 mock 脚本，发布输入检查拒绝。","recoveryEvidence":"41986032850；脚本候选 3ff84f62 的 CI 和主线 CI 通过后合入候选。","permanentAction":"发布负责人在冻结前比较受信自动化路径并先行资格化；下次发布复核，不修改现有门禁。","historicalReleases":[]},
  {"fingerprint":"release-source/music-shuffle/test-timeout","position":"before-production-write","count":1,"impact":"220 首随机循环测试在并发源码检查中耗时 5.217 秒，超过 5 秒默认时限，rc.2 未公开。","recoveryEvidence":"41988787529 原失败；rc.3 41996328243 的源码和完整发布验证通过。","permanentAction":"该测试保留全部断言并限定 15 秒；KPanel 维护者在下一次稳定版前复核并发耗时，若再次超时定位性能原因。","historicalReleases":[]}
]
<!-- kpanel-release-process-incidents:end -->
