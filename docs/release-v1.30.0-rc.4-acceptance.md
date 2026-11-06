# KPanel v1.30.0-rc.4 发布验收

日期：2026-11-06；发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.30.0`。

冻结候选 `e06c4c181c4b7dee18d0d9afe05291e28908e60b` / 标签 `v1.30.0-rc.4`；[公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.30.0-rc.4) 为 prerelease，含 19 个附件。预览 OCI index 为 `sha256:965197fd6f8d311df3de4997bda8906e6b0908bf79003c2f10ead47bc2bf3063`。稳定回滚点为 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`；发布前上一预览为 `v1.30.0-rc.3` / `sha256:fd3ddb98dfa654d7466e33f9d01f05be92aba4fc859bf522b7b8df9261c48398`。

## 发布画像

- 业务域：站点访问保护、移动终端、Agent 终端恢复、AI/MCP 文件尾读取与媒体目录浏览。
- 变更面：展示、只读、宿主机写入（访问密码通过绑定的 `kejilion.sh` 配置 Nginx）。
- 受影响用户旅程：配置站点访问密码；在手机终端输入 Ctrl 组合命令；终端断线/刷新后恢复；AI/MCP 请求读取文件尾；媒体目录扫描期间查看已发现影片。
- 未变化契约：没有 KPanel 端口、Compose 或数据库 schema 迁移；Agent、终端和脚本协议按候选绑定版本检查。网站访问密码需要配套脚本，不能混用旧版脚本。
- 风险等级：L3。涉及主机 Nginx 写入、AI/MCP 文件读取边界与公开 OCI/安装产物。

## 发布范围与未纳入内容

- 用户可见更新：网站访问密码；手机终端 Ctrl 辅助键；影视目录渐进显示；终端窗口关闭与断线恢复；AI/MCP 文件尾读取稳定性和敏感字段遮盖修复。更新说明与升级提示见公开 Release。
- 精确候选：`d5e79b70`（移动 Ctrl）、`3c489510` 至 `6a1832d8`（AI/MCP 文件读取与脱敏）、终端恢复及媒体扫描修复，最终测试隔离提交 `e06c4c18`。候选完整提交范围以 `v1.30.0-rc.3..e06c4c18` 为准。
- 未纳入：稳定版发布、生产部署、真实 AI 提供商端到端测试、真实手机设备验收、全量自定义 Nginx 错误页/响应头管理。

## 外部审计与修复交付

- 安全盲审：候选基线 `v1.29.0`；安全范围 15 组均有记录。脱敏审查原始 M 级条目 10 项（含 1 项重复），确认 9 项、修复 7 项、2 项既有基线发现继续延期并保留在外部 backlog；另有 1 项 H 级原始报告经生产调用链复核为推测项并拒绝。未声称完整安全审计证明无漏洞。证据见 `C:/GitHub/_release-evidence/v1.30.0-rc.4/security-review-acc618bc.txt`。
- `scripts/security-review-scope.mjs --base v1.29.0`：候选 security-review groups 15 组为 recorded；发布 CI 的 Go 调用链、Node 依赖、源码/机密/配置扫描均通过。
- Finding 处置：7 项脱敏边界修复在候选源码内；2 项确认的基线发现保留外部负责人、触发条件和期限；无凭据暴露或真实利用结论。最终异步清理测试竞态修复在 `e06c4c18`，只改测试隔离，不改产品 Go 行为。
- 独立复核：候选有记录，`cross_provider=false`；不将其表述为不同模型供应商复核。竞态修复 OCR：`1.12.11 range=529a790..701837d files=3/3 free-form=0 valid=H0/M0/L0 constrained-only=unreported`，详见 `C:/GitHub/_release-evidence/v1.30.0-rc.4/race-fix-review/`。
- 配套脚本修复交付：RC4 OCI 标签固定 `kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff`，根脚本 SHA-256 `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`，中文脚本 SHA-256 `c1d09c83333802c75a4c6449f988ef457afde7b7ef13aabb6ad5d999606d7c3d`。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`。
- 变更集编号：验收补记 `kpanel-v1.30.0-rc.4-website-access-password`，绑定 KPanel `e06c4c181c4b7dee18d0d9afe05291e28908e60b` 与脚本 `1faee13f3faef164efbfa899e244fc19ad58aaff`。候选原始证据未找到更早共享编号，故明确作为验收记录标识，不声称此前已建立。
- KPanel 实际内置脚本基线：RC4 image labels 中脚本 commit/SHA-256 与上方配套脚本一致；公开镜像 amd64/arm64 均相同。
- 脚本候选 commit / SHA-256：`1faee13f3faef164efbfa899e244fc19ad58aaff` / `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`；中文 SHA-256 `c1d09c83333802c75a4c6449f988ef457afde7b7ef13aabb6ad5d999606d7c3d`。
- 状态依据：容器标签、配套脚本测试与发布 CI 生命周期检查通过；完整 Panel→Agent→脚本→真实 Nginx 主机链路未在本次公开验收中执行。
- 发布决定：配套脚本先行公开，再发布 KPanel 预览版；未变更 `kejilion/apps` 默认稳定入口。
- 阻断或移除范围：未移除功能。自定义 `error_page`/认证前响应正文、`add_header ... always`、回调与证书续期、直连上游端口及 Bitwarden/Halo 创建时启用访问密码等边界按 Release 与外联配置登记的限制披露。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证（公开容器范围） | 公开产物真实隔离容器 48 项通过；包括终端、历史、发布接口与通道状态 | 网站密码真实 Nginx 主机双端闭环未测 |
| 网络入侵与供应链安全 | 已验证（门禁范围） | Go 漏洞扫描、Node audit、源码/依赖/机密/配置扫描、双架构身份与 provenance 检查通过 | 两项基线安全发现仍按 backlog 延期；不代表零漏洞 |
| 稳定性、失败恢复与兼容 | 已验证（隔离容器范围） | 终端跨刷新恢复、关闭会话拒绝输入、脚本生命周期及浏览器错误检查通过 | 长期 soak、生产宿主和旧版混用未验证 |
| 性能与资源预算 | 未验证 | 本次未做负载或长期资源测试 | 预览版不外推生产容量结论 |
| 用户体验与可访问性 | 已验证（浏览器范围） | 发布浏览器 45 项、升级浏览器 31 项；320/390/1280 视口无横向溢出 | Ctrl 辅助键已纳入移动 UI；真实手机、键盘辅助技术及原生 200% 缩放未验证 |
| 数据、配置与迁移 | 已验证（发布通道范围） | 无 schema 迁移；升级流程核验 stable/preview、自动安装关闭与退出不降级 | 自定义 Nginx 配置、证书续期和真实宿主恢复未验证 |

## 自动门禁

- 定向测试：`node --test scripts/tests/release-runner-image.test.mjs`，4/4 通过（恢复验收工具提交 `53d335ec`）；RC4 候选 race 修复由候选 CI 覆盖。
- `make verify-release`：未在本机运行；以 GitHub-hosted L3 为准。
- L3：发布运行 [42183446758](https://github.com/kejilion/KPanel/actions/runs/42183446758) job `127778202170` 通过，artifact `l3-42183446758-1`；同 SHA 候选 CI、主线 CI 均通过。
- 候选 CI：运行 `42183446758`，`candidate-ci / verify` 与 `windows-node` 通过。
- 主线 CI：发布提升后运行 `42183446758`，`main-ci / verify` 与 `windows-node` 通过；发布后 Runner 修复 `53d335ec` 的主线 CI [42192631177](https://github.com/kejilion/KPanel/actions/runs/42192631177) 通过。
- Release workflow：发布 job `127793392479` 成功并公开 RC4。原始完整 workflow 因 post-publish public-acceptance runner 包版本不匹配而失败；修复 Runner 后恢复验收运行 [42192715429](https://github.com/kejilion/KPanel/actions/runs/42192715429) 成功。不得把原始 run 改写为成功。
- 发布说明门禁：草稿和最终正文均通过；6 条摘要、1 条升级提示。
- 安全扫描、镜像契约、provenance：Go 调用链漏洞扫描、Node audit、源码/依赖/机密/配置扫描、`kejilion.sh` 生命周期、原生镜像扫描与运行契约、双架构镜像及 provenance 上传均通过。

## 依赖与技术栈变化

- `make dependency-report`：最近完整报告为 2026-10-29 GitHub run `40893882484`，必需检测源 9/9 成功；依据 `docs/dependency-lifecycle-review-2026-10-29.md`。
- 最近每日安全通告审计：2026-11-05 run `42005545987` 的 `security-advisories` job 成功；最近 EOL 复核为 2026-10-29，未超过 92 天期限。
- 直接/基座行动项：产品运行时与 RC4 基座未升级；`packaging/release-runner/Dockerfile` 的验收工具仅更新 `docker-cli` pin `29.9.2-r0`→`29.9.3-r0`，保持 `docker-cli-buildx=0.38.1-r0` 和其他 pin 不变。原因是 Alpine 已提供 29.9.3-r0，旧 pin 使首轮发布后验收容器无法构建。
- 版本/摘要：验收 Runner 修复提交 `53d335ec2bddad570afd05ff0ba4e41b504afe00`；该修改不进入 RC4 tag 或产品镜像。恢复公开验收使用新 Runner 成功构建，`productImageUnchanged=true`。
- 暂缓候选：无产品依赖更新。Alpine APK pin 会随上游包滚动失效；下次预览前需复核精确包版本并在公开发布前构建验收 Runner。
- 兼容、性能与回滚：仅影响验收工具容器，不影响 KPanel、Agent 或生产镜像；可通过回滚 `53d335ec` 恢复旧 pin，但旧 pin 当前不可构建。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时：GitHub-hosted Ubuntu 24.04.5；公开产品镜像 linux/amd64 与 linux/arm64。
- 环境策略 ID：`github-hosted`；仅用于公开 Release 与隔离容器验收。
- 精确产物：RC4 `e06c4c181c4b7dee18d0d9afe05291e28908e60b`，OCI `sha256:965197fd6f8d311df3de4997bda8906e6b0908bf79003c2f10ead47bc2bf3063`。
- 后台作业：原始 public acceptance job `127802102697` 于 `2026-11-06T08:53:04Z` 失败；修复工具后 run `42192715429` job `127808390900` 于 `2026-11-06T09:09:55Z` 成功，退出码 0。artifact `public-42192715429-1` / ID `13490094357`；本地证据 `C:/GitHub/_release-evidence/v1.30.0-rc.4/public-acceptance-recovery/`。
- 测试窗口：无 soak（预览发布验收，不代表生产稳定性）。公开真实隔离容器 48 项、published 浏览器 45 项、upgrade 浏览器 31 项均通过，错误数为 0。
- 用户旅程/视口：真实浏览器检查 stable/preview 发布接口、摘要、升级弹窗、自动安装开关、退出预览行为；320/390/1280 视口无横向溢出。真实终端 API/PTY、历史隐私提示、跨刷新恢复、会话关闭后拒绝输入均通过。
- 宿主机写入：只在一次性隔离容器执行；没有连接生产主机。访问密码真实 Nginx 配置写入、持续日志下 AI 读取及原生手机终端未在本次公验中执行。
- 未执行：真实 AI 提供商对话、物理手机、长期 soak、生产升级/回滚；不将 mock 或隔离容器结果外推为这些场景的证据。

## 发布产物与公开仓库复核

- 公开 Release、面板发布接口和更新弹窗均显示 `1.30.0-rc.4` 与 digest `sha256:965197fd6f8d311df3de4997bda8906e6b0908bf79003c2f10ead47bc2bf3063`；发布正文摘要非空，6 条更新与 1 条升级提示实际显示，浏览器检查通过。
- GitHub Release：`isDraft=false`、`isPrerelease=true`；GitHub Latest 仍为 `v1.29.0`。
- Docker OCI index：RC4/`preview` 为上方摘要；稳定 `latest` 仍为 `v1.29.0` / `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- 平台摘要：linux/amd64 `sha256:d6902c49c225a0cfc9e885166b42bb79d2a991b652ea016b0cd428737aad91f0`；linux/arm64 `sha256:7b1f04477d20df01d4a77167b6e947acd9eb7456aa384f45a2e1aec890ddcfbf`。
- 附件：19 个；`SHA256SUMS` 含 15 个校验条目；公开验收核对附件、checksum 与 OCI/source 标签均通过。
- 公开镜像 `image_e2e=pass`：真实隔离容器报告 48 项通过；published/upgrade 浏览器分别 45/31 项通过，错误数均为 0。
- `kejilion/apps` / `kejilion.sh`：未变更 apps 默认稳定入口；RC4 镜像标签与配套脚本 revision/hash 相符。完整真实 Nginx 主机链路未验证。

## 自更新通道验收

- 公开接口验证 stable 为正式 `v1.29.0`，preview 为规范 RC，digest 匹配；发布镜像为唯一官方来源。
- 加入预览只切换来源并立即检查，没有自动安装；实际更新需用户显式选择。
- 自动安装默认关闭；拒绝加入与 opt-in 状态检查通过，opt-in 不会自动请求安装。
- 旧状态默认 stable 与通道持久化：公开浏览器核验 stable/preview 和升级弹窗行为；真实跨重启的宿主系统状态未测。
- 退出预览不会产生降级候选；退出通道只恢复稳定来源。
- 真实 systemd 安装、备份/升级/失败恢复和 OpenRC 主机行为未在生产宿主上验证；公开流程无 systemd 时失败关闭。
- OpenRC 与轻量 Node 边界按 `docs/release-channels.md` 和 Release 正文披露。

## 生产部署安全核对

> `preview` 必须将本节生产动作标记为“不适用（预览版禁止生产部署）”，不得用隔离验收代替生产证据。

- 生产目标和部署授权范围：不适用（预览版禁止生产部署）。
- 验证/灰度环境：GitHub-hosted，仅运行公开验收。
- 正式部署环境：不适用；未连接 `arena-154` 或其他 KPanel 生产主机。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：不适用。
- 部署命令/入口：未运行。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：未验证（无生产部署）。
- 生产已执行写操作：否。
- 仅在隔离环境执行：公开 RC4 镜像、浏览器和一次性容器验收。

## 回滚

- 源码/tag：保留 `v1.30.0-rc.4` 指向 `e06c4c181c4b7dee18d0d9afe05291e28908e60b`；回退来源 `v1.29.0`。
- 镜像 digest：RC4 `sha256:965197fd6f8d311df3de4997bda8906e6b0908bf79003c2f10ead47bc2bf3063`；稳定回滚 `sha256:587b51cafb56da4c1d4e30b7eb48714c9af755267fa769ac2b7fa949e0625ea6`。
- 数据/配置备份：真实生产数据未触及；用户升级前应备份 Panel、Agent 和自定义宿主脚本。
- 回滚步骤：从设置页退出预览不会自动降级；如需恢复稳定版，按 `docs/release-channels.md` 与应用市场既有流程手动恢复并核对数据备份。未在生产执行回滚。
- 回滚后生产实际版本与健康状态：不适用。
- GitHub Latest、Docker `latest` 与标准更新入口：仍为 `v1.29.0` / stable。
- 公共默认更新通道决策：不适用（预览版显式加入，稳定默认未变）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-05T15:40:50+08:00
- 候选冻结时间：2026-11-06T15:35:57+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（产品产物未回滚或重复发布；仅恢复公开验收）
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
    "fingerprint": "public-acceptance/release-runner/unavailable-apk-pins",
    "position": "before-production-write",
    "count": 1,
    "impact": "RC4 Release 与预览镜像公开后，首次真实公开验收在构建验收 Runner 时因 Alpine 已提供 docker-cli 29.9.3-r0、Runner 仍固定 29.9.2-r0 而失败；浏览器和容器断言尚未开始。无生产部署或产品镜像变更。",
    "recoveryEvidence": "原始发布 run 42183446758 job 127802102697 保留失败日志/artifact 13489244042；Runner pin 修复 53d335ec 的 main CI 42192631177 通过；恢复 run 42192715429 job 127808390900 与 artifact 13490094357 报告通过，productImageUnchanged=true，稳定摘要仍为 v1.29.0。",
    "permanentAction": "KPanel Release Maintainer 在下次预览冻结前核对官方 Alpine APK 版本并先构建精确验收 Runner；跟进将 Runner 构建前移到发布前。v1.28.0-rc.2 有同指纹旧记录；对照本次之前最近五个公开版本 v1.30.0-rc.3、rc.1、v1.29.0、v1.29.0-rc.7、rc.6，未发现同指纹；下次预览前关闭此项并留存预检证据。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 本地资源回收：未清理本地分支、worktree 或 evidence；保留 RC4 与恢复验收证据用于追溯。
- 未验证风险：真实 AI 提供商对话；实时持续写入文件的稳定读取兼容性；自定义 Nginx 错误页/响应头、回调与证书续期；物理手机和原生缩放；长期负载；真实生产主机数据迁移。
- 已实现待实机准入：网站访问密码的 Panel→Agent→配套脚本→真实 Nginx 完整链路；有写入方持续修改文件时 AI 文件尾读取会安全失败关闭，按 Release 指示关闭写入或复制稳定文件后重试。
- 不阻断本版的理由：本版是显式 opt-in 预览，风险和兼容边界在公开说明中披露；公开双架构身份、附件、真实隔离容器、通道与浏览器验收均通过；生产默认仍是 v1.29.0。
- 后续准入：下次预览冻结前复核并预构建 Release Runner；访问密码功能若进入稳定版，先完成登记隔离环境的真实 Nginx 双端测试；AI 外部提供商和手机设备验收另行登记，不以本次浏览器/隔离证据代替。
