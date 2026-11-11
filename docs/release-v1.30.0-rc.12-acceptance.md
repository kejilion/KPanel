# KPanel v1.30.0-rc.12 发布验收记录

日期：2026-11-11

发布级别：L3

候选提交 / 标签：`7f70633cc72d1cb0548bdddf5d2c511edfc2a251` / `v1.30.0-rc.12`

上一稳定版本 / 回滚点：`v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.0`

候选分支与发布后处置：`release/v1.30.0-candidate` / 预览版保留

- 冻结候选、`main` 和公开 tag 均指向 `7f70633cc72d1cb0548bdddf5d2c511edfc2a251`；候选分支保留，未归档或删除来源分支。
- `fix/music-library-row-layout@34a00054` 纳入；异步标签排序与筛选稳定行为由同版音乐源码覆盖。旧分支的两行展示样式被本版紧凑单行布局取代。
- `fix/direct-browser-links@7d0128f8` 的打开行为已在 RC12 源码中等效存在（`233b2436`）；同一功能不重复合并。
- `fix/music-scroll-stability@7082c2d3` 的稳定排序、筛选与滚动稳定行为已被 RC12 的组合音乐实现覆盖；保留 RC12 单行固定行高布局，不导入旧版两行样式。
- 网站快捷方式自动图标接口、壁纸切换统一和浏览器初始页增强均不在 RC12 冻结差异中；它们在 RC12 发布后形成新候选，进入后续 RC13 审查。
- 本地候选工作树和发布证据保留；来源分支不删除。未完成归档由 Release Maintainer 在稳定版收尾时复核。

预览产物已公开，生产未部署。

## 发布画像

- 业务域：音乐库行布局、Panel 与可选浏览器组件工具链。
- 变更面：展示、版本与镜像身份；无宿主机写入、数据迁移或 Agent 协议变更。
- 受影响用户旅程：浏览曲目时异步标签和音质资料加载不改变紧凑行高；预览用户可安装 Go 1.27.4 工具链构建的 Panel 与可选浏览器组件 RC5。
- AI 助手、脚本、站点认证、历史监控和生产配置不在本版产品差异中。
- 风险等级：中低；明确 opt-in 的预览版，稳定默认和生产部署不变。

## 发布范围与未纳入内容

- 用户可见更新：曲目标题单行省略，标签和音质信息不再增加行高；可选浏览器组件固定至已验证的 RC5 多架构镜像。
- Go 工具链更新至 1.27.4，修复 GO-2026-6859；无 Go module 或 `go.sum` 依赖变化。
- 旧的未认证远端图标抓取候选因 P2 风险未纳入；RC12 不提供快捷方式自动获取站点图标。
- 全曲库音乐自动刮削、影视刮削、实体手机验收和生产升级均未包含或执行。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`；RC12 未修改或发布 `kejilion/sh`。
- KPanel 内置脚本沿用此前已发布版本；本次没有脚本候选或脚本 SHA 变化。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证（发布门禁） | 候选 CI、L3、主线 CI、公开产物和公开浏览器验收均成功 | 不声称外部音乐服务刮削或真实第三方媒体验收 |
| 网络入侵与供应链安全 | 已验证（发布门禁） | 精确候选 L3、安全扫描、镜像身份及公开产物检查成功 | 旧未认证图标候选明确排除；不声称整个产品无漏洞 |
| 稳定性、失败恢复与兼容 | 已验证（发布门禁） | 候选 CI、Windows Node job、L3 与公开验收 job 成功 | 未做生产回滚或长时间 soak |
| 性能与资源预算 | 已实现未实机验证 | 紧凑布局不增加行高；发布门禁通过 | 没有新增低配长时性能数据 |
| 用户体验与可访问性 | 已验证（候选与公开门禁） | 音乐行布局候选预览及公开浏览器验收通过 | 实体手机、完整键盘/缩放矩阵未覆盖 |
| 数据、配置与迁移 | 已验证 | 无数据库、用户文件、Agent 协议或宿主配置迁移 | 不适用生产迁移 |

## 自动门禁与发布产物

- 发布工作流：[`43039543913`](https://github.com/kejilion/KPanel/actions/runs/43039543913)，2026-11-11 14:33:04–15:35:46 UTC，结论 `success`。
- 候选 CI、L3、候选提升、主线 CI、Release、公开验收和 preview completion 均成功；公开验收 artifact 未下载到本地，不在本记录中推断其逐项计数。
- GitHub Release：[`v1.30.0-rc.12`](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.12)，`isDraft=false`、`isPrerelease=true`，19 个附件；不是 GitHub Latest。
- Docker 版本与 `preview` OCI index：`sha256:911a633490db71bea64f7e098165834907d389643ea6ac56b710fc54c29addb2`。公开注册表回读确认两个标签一致；双架构发布与公开身份 job 成功。
- Docker `latest` 保持 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`；RC12 未改变稳定默认。
- `go.mod` 声明 `go 1.27.4`；浏览器组件固定为 `kpanel-browser:0.1.0-rc.5`，多架构 digest `sha256:9a5cd79bef4262cf675331a8ce07e9e5df9121d26fb3721440c6256ad444ea03`。
- 发布前 snapshot 记录上一稳定 `v1.29.0` 和先前预览 `v1.30.0-rc.11`；RC12 后公开 `preview` 已指向 RC12，`latest` 仍为 v1.29.0。

## 生产与回滚

- 生产部署：不适用（预览版）；本次未连接、备份、升级或部署生产主机。
- 回滚点：`v1.29.0`，Docker `latest` 仍指向上述稳定 digest；预览用户可退出预览来源，但退出不会自动降级。
- 验收边界：公开验收 job 成功证明精确公开产物门禁完成；本机未获得公开验收 artifact 的逐项报告，因此不补写未见的用例数量或实体设备结果。

## 首轮 L3 失败与恢复

- 首轮 RC12 预览工作流 [43032017270](https://github.com/kejilion/KPanel/actions/runs/43032017270) 于 2026-11-11 21:54:36（+08:00）在 L3 的 TestEmbeddedDeploymentAssetsMatchCanonicalPackaging 检出嵌入式 managed-compose.yml 与规范打包文件不一致。候选 CI 与 Windows 检查已通过；L3 拒绝提升，未写入 main、tag 或 GitHub Release。
- 提交 ff62c3eeeef48f29970302232fce331e76c99b4a 同步了嵌入资源。最终 RC12 候选 7f70633cc72d1cb0548bdddf5d2c511edfc2a251 的 L3（job 130587467992）于 2026-11-11 22:52:18（+08:00）通过，之后仅公开一次。
- 该事件归类为候选产品/打包校验失败，被正常 L3 门禁拦截；不是执行器、基础设施或证据通道异常，因此流程异常计数为 0。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-11T10:08:05+08:00
- 候选冻结时间：2026-11-11T22:22:18+08:00
- 生产完成时间：不适用
- 提交到生产用时：未记录
- 是否回滚、紧急热修复或重复发布：否（首轮 L3 在公开前拦截并修复；最终预览仅公开一次，无回滚或紧急热修复）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->
