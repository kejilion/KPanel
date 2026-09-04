# KPanel v1.2.0 发布验收记录

日期：2026-09-04

发布级别：L3

产品提交 / 标签：a163cff124a94d7f5efa07486f576d3998cf0ba3 / v1.2.0

标签对象：3dcd4374546de61a539b8130e80d0c39b607fdb5；剥离后的产品提交精确为 a163cff124a94d7f5efa07486f576d3998cf0ba3。

候选基线：origin/main 090ec251fc1b2e25558f109eb148e92a5ea36b75，基线标签 v1.1.0。

上一稳定版本 / 回滚点：v1.1.0 / 83aebcfb94fa387f8ea52c6c48f1ddb161e1af27。

## 发布画像

- 业务域：集群文件管理互传、轻量节点配对恢复，以及桌面模式文件选择交互。
- 用户可见更新：
  - e310d6d04e5910e44331e0294c1a1c1c945e6e80：配对 KPanel 节点支持在现有文件管理页面内切换远端文件主机，沿用既有路径、权限、下载、预览和 Panel-Agent 中继边界；最终只 cherry-pick 该提交的有效补丁，候选本地提交为 ac73f1c。
  - 387d57525050486fa78016b55a4fc943a0aec75f：轻量节点删除后支持重新配对，候选本地提交为 6d13d0c。
  - c41d557deb55c93ff01794a95df635f3e494e41e：稳定桌面模式文件选择布局，候选本地提交为 eb64ccb。
  - 88b198403e4939f0804e0cf1604c1f773277bcea：移除文件列表行的选中聚焦框，候选本地提交为 ea14114。
- 版本准备提交 a163cff 将版本元数据、CHANGELOG 和发布构建版本统一为 1.2.0。
- 相对候选基线的最终差异为 28 个文件、1909 行新增、85 行删除；e310d6d 分支中的平行历史提交没有整体合入，避免重复引入旧版本内容。
- 未变化契约：无数据库迁移、无 Compose 端口变化、无 Apps 仓库应用契约变化；旧节点通过既有能力协商和中继边界继续兼容。
- System Center 排除：候选产品差异路径没有 System Center 页面、路由、API、数据、文档或写入；本次未执行 System Center 操作。
- 未连接 108 或 prod-108；唯一真实验证和生产目标为 arena-154。

## 明确未纳入的工作树内容

本次只纳入在 v1.1.0 之后形成、已提交、可冻结并通过精确 SHA 门禁的四组产品变更。以下内容仍保持原状，没有被删除、重置或发布：

- feature/visual-refinement-pass 的 Claude 界面优化仍是未冻结的工作树内容。
- 轻量节点 SSH 终端扩展仍有未提交改动。
- 进程管理热力图、云存储接入、原生图标等工作树含未提交或未跟踪内容。
- 其他旧任务工作树没有形成可独立验收的 v1.2.0 候选提交。

## 自动门禁

- 固定 Linux L3：run=v1.2.0-a163cff-l3-r2，candidate=a163cff124a94d7f5efa07486f576d3998cf0ba3，base main=090ec251fc1b2e25558f109eb148e92a5ea36b75，base tag=v1.1.0，状态 passed、exit 0；远端目标仅 arena-154，证据目录为 /root/kpanel-release-evidence/v1.2.0-a163cff-l3-r2。
- L3 Runner：kpanel-release-gate:go1.26.7-node24，不可变 digest=sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3。
- L3 证据包：bundle SHA-256=1d48e6fc043a6a513cfdfa710022b1472653d71019855a8db75cf7b9e2c627c8；plan SHA-256=422289ba46e535d2f80d224ec7a5f149581a3a1d17299c746d696c3773dc9b10；远端脚本 SHA-256=d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c。
- L3 结果：Go 全量测试通过；前端 132 个测试文件、1125 项测试通过；typecheck、i18n:check（2129 个本地化短语、21 个 lazy catalogs）、build、govulncheck、npm audit、Trivy 源码/依赖/secret/镜像扫描、应用配置生命周期和 runtime contract 均通过。
- 候选 CI：run 33864447906，精确 SHA=a163cff124a94d7f5efa07486f576d3998cf0ba3，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33864447906
- 候选 Dependency freshness：run 33864446934，精确 SHA=a163cff124a94d7f5efa07486f576d3998cf0ba3，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33864446934
- 主线 CI：run 33868118336，精确 SHA=a163cff124a94d7f5efa07486f576d3998cf0ba3，分支 main，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33868118336
- 主线 Dependency freshness：run 33868118329，精确 SHA=a163cff124a94d7f5efa07486f576d3998cf0ba3，分支 main，completed successfully；https://github.com/kejilion/KPanel/actions/runs/33868118329
- Release workflow：run 33869452628，精确 SHA=a163cff124a94d7f5efa07486f576d3998cf0ba3，全部阶段 completed successfully；https://github.com/kejilion/KPanel/actions/runs/33869452628
- Windows 本地 verify-change 只因环境缺少 go/gofmt 在 preflight 停止；Linux L3、候选 CI、主线 CI 和 Release 均使用完整工具链并通过，这不是候选代码失败。

## Release 与 OCI

- GitHub Release：KPanel 1.2.0，公开、非 draft、非 prerelease；https://github.com/kejilion/KPanel/releases/tag/v1.2.0
- 发布附件 8 个：Agent amd64/arm64、轻量节点 amd64/arm64、部署归档、SHA256SUMS、LICENSE、THIRD_PARTY_NOTICES.md。
- OCI index：docker.io/kjlion/kejilion-panel:1.2.0 与 latest 的 digest 均为 sha256:ebadf5361a24b8d580da873190ae43f493ab2fefdf1ba9f41f4ffdab1f3e5f5e。
- OCI 平台 manifest：linux/amd64=sha256:e393e5419d478b51ebb853b40a4506a66aa3736f261f5a3d67671586446c033f；linux/arm64=sha256:2cf6dd48994639da3918a334c7a626f88b8eecf0b248b66cbfd80216def70979。
- 镜像内受管 kejilion.sh：revision=2ee9856c9916b7ede8bbc19edc97e22872e86203，SHA-256=77258027f934ffe6a583300f8350249978eace0ddc838e2d35f26cc5c21ae35c；固定 ADD checksum 和镜像标签校验通过。

## 生产证据与部署

- 环境策略：preflight、production-deploy、postdeploy 三种用途均通过；目标仅 arena-154。
- 当前线上基线 preflight：run=v1.2.0-a163cff-baseline-110，expected version=1.1.0，状态 passed、exit 0；远端证据为 /root/kpanel-release-evidence/v1.2.0-a163cff-baseline-110/production-preflight；本地记录目录为 C:\GitHub\_release-evidence\kpanel-v1.2.0-a163cff-baseline-110-preflight-20260904。
- backup：run=v1.2.0-a163cff-production-r3，expected version=1.1.0，baseline run=v1.2.0-a163cff-baseline-110，状态 passed、exit 0；远端证据为 /root/kpanel-release-evidence/v1.2.0-a163cff-production-r3/production-backup；有效备份为 /root/kpanel-backups/pre-v1.1.0-20260904T120500Z；备份归档、旧镜像、Agent unit、kpanel.conf、Panel inspect 和 SHA256SUMS 均通过校验。
- 标准生产入口：ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel。应用市场同步成功，拉取 latest 的 digest 为本版 OCI index，Panel 重建并启动成功。
- postdeploy：run=v1.2.0-a163cff-production-r3，expected version=1.2.0，expected revision=a163cff124a94d7f5efa07486f576d3998cf0ba3，expected image digest=sha256:ebadf5361a24b8d580da873190ae43f493ab2fefdf1ba9f41f4ffdab1f3e5f5e，状态 passed、exit 0；远端证据为 /root/kpanel-release-evidence/v1.2.0-a163cff-production-r3/production-postdeploy。
- 生产状态：Panel running/healthy，health status=ok、version=1.2.0；Agent active/running/enabled；RestartCount=0；OOMKilled=false；Panel revision、版本和 OCI digest 精确匹配；protected config/data diff 为空；近 10 分钟 Agent/Panel 日志无 panic、fatal、OOM signature。
- 本地生产证据 manifest：baseline preflight=08a69cdbf274688545e989d6b1e3f83da5075964db1fba0f5b872d2cb5a2a574；backup=B88947e7fbc44f8b1f8a79334caf92c698fbd381eaf7dc3525da83f791eba34d；postdeploy=B793f9acc42cb822e70966074634117af99ff047cadd36a401a85368150e4c88。

## 回滚

- 源码/tag：v1.1.0 / 83aebcfb94fa387f8ea52c6c48f1ddb161e1af27。
- 上一稳定 OCI index：sha256:a4d1c9faac7b576a75f77522409c981105f817ccd0c8f90b355edd106670113f。
- 数据/配置/旧镜像备份：/root/kpanel-backups/pre-v1.1.0-20260904T120500Z。
- 回滚按应用市场规范恢复 Compose、.env、Panel 数据、Agent unit、旧镜像和受管脚本，再以 production evidence 核对 health、Agent、digest、restart/OOM、数据和日志；禁止只切换浮动 latest。
- 回滚状态：未执行；当前 arena-154 为 v1.2.0 healthy。

## 流程异常与证据修正

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-04T17:19:58+08:00
- 候选冻结时间：2026-09-04T18:41:32+08:00
- 生产完成时间：2026-09-04T20:06:07+08:00
- 提交到生产用时：2.77 小时
- 是否回滚、紧急热修复或重复发布：否（仅重建证据，未重复发布产品）
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：3
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/local-release-clone-tag-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次 L3 在共享本地管理树 preflight 发现 v0.86.2 本地 tag 与 origin 不一致，按规范停止；没有上传或生产写入。",
    "recoveryEvidence": "改用全新 HTTPS release clone，验证 v0.86.2 与 origin 一致后，以相同候选 SHA 执行 v1.2.0-a163cff-l3-r2 并通过。",
    "permanentAction": "L3 使用干净发布 clone，禁止覆盖共享管理树的历史 tag。",
    "historicalReleases": []
  },
  {
    "fingerprint": "production-baseline/stale-v1.1.0-preflight",
    "position": "before-production-write",
    "count": 2,
    "impact": "前两次 backup 使用旧 v1.1.0-production preflight；该历史快照 expected version 实际为 1.0.2，与当前线上 1.1.0 不匹配，因此两次证据收尾失败。",
    "recoveryEvidence": "两次备份归档和 SHA256 校验完成，服务均自动恢复为 1.1.0 healthy；随后重建当前线上 1.1.0 的 baseline preflight，并以 production-r3 backup 通过。",
    "permanentAction": "每次发布前为当前线上版本重新建立精确 baseline preflight，不把旧发布前快照当作当前基线。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未执行长期多节点 soak、独立第三方渗透测试、完整浏览器人工矩阵和受控生产回滚演练；这些不阻断本版，因为固定 L3、候选/main CI、Dependency freshness、Release/OCI 和 arena-154 三阶段证据均通过。
- 继续观察跨集群文件互传的多节点并发、轻量节点重新配对和桌面文件选择布局；若发现问题，按 v1.1.0 的 OCI、Compose、数据、Agent 和受管脚本备份回滚。
- 生产结论：v1.2.0 已在唯一授权目标 arena-154 完成上线；System Center 保持排除，未连接 108 或 prod-108。
