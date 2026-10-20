# KPanel v1.27.0-rc.16 发布验收记录

日期：2026-10-20。发布级别：L3。已验证：RC16 于北京时间 2026-10-20 12:28:25 公开为 GitHub prerelease，15 附件齐全；版本镜像与 preview 通道已提升。根流水线 attempt 2 成功，公开浏览器/隔离容器四份报告共 146 项检查通过、0 错误，10 张截图保留。原 attempt 1 的 HTTP 403 失败记录保留。稳定 Latest/latest 均为 v1.26.0；生产未部署。

候选提交 / 标签：`06c5ba2eaf328256073a26bb8347c54a472d40ea` / `v1.27.0-rc.16`。
候选 tree：`50c4d72b02b4aafdcf6803e342116b80721d5599`。
批准基线：RC14 `f5e053ae0055a5b7a3f7a169b588ba2416360c22`。
`releaseChannel=preview`，`releaseTrain=1.27.0`。
上一稳定版：`v1.26.0`，Docker index `sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`。
上一预览回滚点：`v1.27.0-rc.14`，Docker index `sha256:6bd05b5e7b6d50646fb14ab1657cdde48d744059e11a25ef292702f8c9f4b524`。

## 发布画像与范围

- 业务域：桌面 XP 开始菜单、桌面组件卡片、移动终端触摸选择。
- 变更面：Web 展示与触摸交互、版本元数据、版本变化所需的供应链源码身份重绑定。
- 用户可见更新：XP 开始菜单完整程序在单层显示并支持搜索；桌面 widget 卡片阴影不再被网格裁切；移动终端长按选择文字并在释放后显示复制菜单。
- 受影响旅程：开始菜单浏览/搜索/Enter 打开；组件共存时的阴影边界；终端长按选词、跨行拖选、快速滑动、多指与取消。
- 无数据库 schema、Panel/Agent 协议、端口、Compose、Agent 权限、安装脚本或应用市场默认入口变化。生产部署不适用。
- 风险等级 L3：实际预览发布执行完整源码、供应链、镜像运行时、公开产物与隔离浏览器门禁。
- 本轮精确提交：`ed5c729c`、`00c9f5fa`、`4d1ba850`、`ff9aa274`、`8ffeba44`、`000a277d`、`06c5ba2e`。阴影中间尝试以最终树为准，最终差异保留卡片阴影并修复裁切。
- 未纳入：其他较早候选不搭车。图库虚拟时间线与 POSIX IPC ACL 已由等价补丁进入 RC14，不重复列为本轮新增。RC15 是未公开的本地中间版本；未创建 RC15 tag/Release/版本镜像。

## 跨仓库联动

- `scriptLinkageState=not-required`：无需发布脚本（不适用）。本轮没有脚本或应用市场契约变化。
- 实际内置 `kejilion.sh`：`160924a498d9b8d071fed72f165d23c1b9e886de`，SHA-256 `39837815ce0eb23021fd4d8e8b5898000f5309b709bdc48b2713e18f78f3eaf1`，由不变的 Dockerfile 固定。
- 脚本候选、跨仓库变更集及阻断依赖：不适用。

## 自动门禁与审阅

- 精确候选 push CI：[39364509897](https://github.com/kejilion/KPanel/actions/runs/39364509897)，success。
- 正式发布根 run：[39365245104](https://github.com/kejilion/KPanel/actions/runs/39365245104)。attempt 1 的 inputs、candidate-ci、L3、promote、main CI、Release 源码验证与 Go 漏洞扫描均成功；发布前 snapshot 请求 GitHub Latest 返回 HTTP 403，Release 失败，未创建草稿或推送镜像。保留原 run/日志；attempt 2 只重跑失败的 Release 及其下游，Release、public-acceptance、preview-completion 全部 success，根 run conclusion=success。可复用 Release 内的独立 l3 job 按 workflow_call 条件 skipped，实际完整 L3 由本根 run 的 l3/l3 在 attempt 1 成功执行，不将 skipped 当作未执行 L3。
- L3 权威入口：`scripts/run-release-l3.mjs --github-hosted`，`status=passed`，2026-10-20T03:37:51.565Z 至 03:49:44.661Z；环境 github-hosted、ubuntu24/20261012.334.1、Go 1.27.2、Node 24.21.0、Docker 28.0.4；入口 SHA-256 `c286f7f2ad692ea17b8ec0537fe2a84e8d049d22adec63a2b00ceba39b165d57`。
- L3 源码检查三组 web/go/deploy 全部 passed、exitCode=0、identityUnchanged=true；下载后的三个原始日志 SHA-256 与 receipt 一致。源码检查耗时 458549 ms，Web 221260 ms、Go 458533 ms、deploy 2362 ms；不把并行分组之和当作总历时。
- Web：312 files、3037 passed、6 skipped；typecheck、build、3736 phrases / 25 catalogs 通过。Go：全量 test、核心 race、全量 vet 通过。
- L3 日志中 `app_conf_lifecycle=pass`、`image_e2e=pass`；故障注入的 Killed 日志为既有预期崩溃测试，不记为产品失败。
- OCR：`000a277d` 记录 11/11 可审文件的自由臂/约束臂结果，valid H0/M0/L0；代码 7 paths / 280 lines。`06c5ba2e` 仅修改供应链元数据，不扩大产品范围，不声称新一轮 OCR 或工具有效性结论。
- 独立发布复核：发布接手者与原实现上下文分离；本机无 claude/gemini CLI，使用 Codex 干净上下文 fallback=provider-unavailable。对照 RC14 后唯一 Go 源码差异是版本字符串，无新 imports、GoFiles、模块图或构建目标。规范化第一方源码摘要为 `4b4a32fbf718fd1deef13c95a084c82f201dc65cab91f952929e0e2e3ea62b0e`，保留既有 closure/findings/expiry，完整 L3 重验成功。
- 安全盲审：本轮相对 RC14 `not-applicable`、0 包组；未执行 CF。相对 v1.26.0 的继承台账仍有 missing 包组和 invalidTrailer=true，不能宣称全仓安全审计完成。

## 多维质量结论与证据边界

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确 L3 与应用生命周期/镜像 E2E 通过；不声称新的脚本协议或真实业务主机互通验证 |
| 网络入侵与供应链安全 | 已验证 | 本轮调用路径/源码/依赖与镜像门禁通过；继承 finding、期限及审计缺口保留，不代表零风险 |
| 稳定性、失败恢复与兼容 | 已验证 | 单元/集成、race 与故障夹具通过；混合旧节点、真实 systemd/OpenRC 更新和长期运行未新验 |
| 性能与资源预算 | 已实现未实机验证 | 无新依赖和常驻服务；实体手机、大终端缓冲长时负载及原生 ARM 未新测 |
| 用户体验与可访问性 | 已实现未实机验证 | 原候选 mock 中 XP 单层菜单、搜索 Enter 和组件阴影已检查；手机实体长按、原生缩放与读屏未验证 |
| 数据、配置与迁移 | 不适用 | 本轮无持久化格式或配置迁移，无生产数据写入 |

## 依赖与技术栈

- 本轮不升级 Go/Node、第三方依赖、基础镜像、Actions 或内置脚本。版本锁文件只调整产品版本字符串；供应链元数据只重绑定版本变化后的第一方源码身份。
- Dependency freshness run `39364362023` 对 `000a277d` 成功；之后 `06c5ba2e` 不改变依赖清单。不是所有上游版本都已最新的保证，不另造新鲜度结论。
- 精确候选的 push CI、正式候选/主线 CI、L3 和 Release 重新执行规定的漏洞/依赖/源码/镜像门禁。既有扫描 finding 及不在生产闭包等处置仍按仓库清单保留，不能将原始扫描发现或例外到期改写为零风险。
- 既有工具链维护复核与供应链观察到期分别保留 2026-10-23 / 2026-10-25；无新豁免、无延后期限。

## 本地与公开浏览器验收

- 模拟数据 acceptance 预览绑定最终 `06c5ba2e`，clean，`http://127.0.0.1:4175`；manifest 和日志在仓库外 `C:\GitHub\_release-evidence\v1.27.0-rc.16\local-preview-06c5ba2e`。
- `000a277d` 的原 UI 预览与最终候选 Web 树相同；最终只改供应链 JSON。原预览停止后重新生成最终候选 manifest，原证据保留。模拟终端不是宿主机 PTY，不提升为真实主机结论。
- 当前浏览器工具未能进行实体触摸长按；自动化测试覆盖长按选词/释放菜单、跨行拖选、快速滚动取消、多指/touchcancel、中文宽字符、空白与滚动条边界。
- 公开 Panel/Agent 浏览器与隔离容器验收：本次公开 digest 的初次启动 44 项、restart 26 项、published 45 项、RC14 升级展示/失败恢复 31 项，合计 146 项，四报告均 passed、errors=[]、mocked=false。
- 隔离环境为 GitHub-hosted runner 内真实 Panel/Agent 容器；Docker inventory 与测试 CA 为显式夹具。测试 Runner 实际 ID `sha256:7fec28c778b8661a0393b52c682c1fed9d43e9201ca84b41ccfe65af5cb1d2bb`。未挂载宿主 Docker/systemd socket，不等同实际生产/systemd 宿主验收。
- 真实公开 Agent 二进制在隔离 bash runner 内覆盖 retained PTY、刷新续接与关闭；RC14 到 RC16 的真实接口返回 3 项更新和 2 项升级提示，320/390/1280px 弹窗截图保留。独立查看 320px 更新弹窗与重启恢复桌面截图，目标版本、完整摘要和恢复界面可见。
- 加入 preview 仅检查、不自动安装；退出 preview 不降级。fixture 无 systemd，立即安装返回实际 503 后可切回 stable；这证明失败反馈与通道恢复，不证明宿主 systemd 安装成功。
- `public-39365245104-2` 已下载，53 个文件、10 PNG、4 report；本地 SHA-256 清单保存为 `public-artifact-sha256.json`，汇总为 `public-acceptance-summary.json`。三个 L3 源码日志摘要已与其 receipt 独立比对一致。

## 发布产物、通道与回滚

- [公开 Release](https://github.com/kejilion/KPanel/releases/tag/v1.27.0-rc.16)：draft=false、prerelease=true，2026-10-20T04:28:25Z 公开；15 附件齐全。实际正文含 3 项更新、2 项升级提示、完整升级与回滚说明，版本与镜像摘要一致。
- RC16 版本镜像与 preview OCI index：`sha256:1e55461560b5054302c62aa99c1427038f51a5d8efb860057b5724df46b2a128`。
- linux/amd64：`sha256:25b9cfcb62f0e37bd1af934e1c93389f78f01ff6679dc9622174071a423ce2fb`；linux/arm64：`sha256:c46bce6c3bb955b2ce3d4d541e9e369d356fc79ec708b5f9e94ef58640b40b36`。
- native 镜像扫描/运行契约、双架构构建、modified-component provenance 上传与最终发布说明门禁通过；公开附件/镜像身份门禁通过。public-release.json 记录 assets=15、checksumCount=11、notesCount=3、upgradeNotesCount=2；两架构 OCI config 的产品/source/script 标签均与精确身份一致。
- 独立官方 Docker Hub 回读 `registry-independent.json`：version/preview 摘要一致，各有唯一 linux/amd64 和 linux/arm64，latest 保持稳定摘要；GitHub API 独立 Latest 回读为 v1.26.0。
- attempt 2 的发布前快照 artifact `snapshot-39365245104-2` 已下载，candidate/version/run/attempt 身份匹配；stable-before.json SHA-256 `17c587a41bc739f0282fcde6ba4c67e6c87efd6cb77b6b60e34d465a63da4d38` 与 publication-identity.json 一致。快照 stable=1.26.0、previousPreview=RC14。
- main 与候选分支已提升到 `06c5ba2e`；annotated tag object `5551921daf9100876eaba5b03fe8bf06139e8d0b`，peeled commit 为精确候选。
- 稳定 GitHub Latest 为 `v1.26.0`；Docker latest 为 `sha256:e0fc7776d1b55754e597d73aab25c519a70c8ef0402752e1e70896397f1e96f9`，已独立回读，与发布前快照一致。
- 生产部署、生产备份、真实业务升级/回滚：不适用（预览版禁止生产部署）。本次未连接 prod-108 或其他生产主机。
- 回滚点为 RC14 immutable digest；退出 preview 不自动降级。未执行真实回滚。

## 分支处置与资源

- `release/v1.27.0-candidate` 按 RC 序列规范保留，不归档、不删除。
- 原作者来源分支/worktree 未取得所有权释放信号，全部保留；不将保留分支的已集成内容再算新候选。
- 本次供应链修复分支 `fix/v127-rc16-source-identity` 与专属 worktree 保留可恢复提交；未单独推送来源修复分支，精确修改随候选推送。
- 文档 worktree `C:\GitHub\_release-docs\kpanel-v127-rc16`，分支 `docs/release-v127-rc16-acceptance`；本记录不属于已冻结产品字节。
- 证据根目录 `C:\GitHub\_release-evidence\v1.27.0-rc.16`。本次创建的 mock 预览服务已正常停止，manifest 保留 stopped；没有删除未知、活跃或原作者资源，未统计释放字节。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-20T09:59:13+08:00
- 候选冻结时间：2026-10-20T11:28:21+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/version-check/wrong-file-extension",
    "position": "before-production-write",
    "count": 1,
    "impact": "尝试不存在的 check-version-consistency.mjs，module-not-found；未改变产品状态。",
    "recoveryEvidence": "定位实际 scripts/check-version-consistency.sh，经 run-repo-bash.mjs 执行，RC16 版本一致性通过。",
    "permanentAction": "保留唯一仓库入口；未知路径先通过 rg --files 定位，不将推测路径当作已确认。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/source-scope-digest/stale-binding",
    "position": "before-production-write",
    "count": 1,
    "impact": "原始 RC16 候选 000a277d 沿用 RC14 第一方源码摘要；发布接手者在 dispatch 前发现，旧 push CI 39364362038 作为已替代检查取消。没有失败版本被公开。",
    "recoveryEvidence": "06c5ba2e 以 canonical firstPartySources 重绑定源码身份，closure/findings/expiry 保持原值；新精确 push CI、候选 CI 与完整 L3 通过。",
    "permanentAction": "保留 fail-closed 摘要门禁；版本字符串变化也必须先做 canonical source requalification。该根因曾出现于 RC13/RC14 预览，明确保留重复事实，未发生生产写入。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-snapshot/github-public-api/http403",
    "position": "before-production-write",
    "count": 1,
    "impact": "attempt 1 在 Freeze stable channel before preview publication 请求公开 releases/latest 得到 HTTP 403；源码与调用路径扫描此前已通过。snapshot 尚未形成，未创建 draft、版本镜像或通道提升。403 的上游根因未确认，不将其直接断言为限流或权限配置错误。",
    "recoveryEvidence": "publication-failed.log 与 publication-attempt1.json 保留原失败；首次失败后 GitHub Release 不存在，Docker Hub 独立探测 RC16=404，preview=RC14、latest=1.26.0。只重试失败 Release 作业一次；attempt 2 的 snapshot、镜像发布、GitHub prerelease、公开验收和 preview-completion 均 success，根 run 成功；公开四报告 146 checks / errors=0。",
    "permanentAction": "当前仅为一次有界恢复，不宣称永久修复。发布维护者于 2026-10-23 复核公开 API 的鉴权/速率限制与错误回执；退出条件为明确根因并对唯一脚本补相应验证。相同阶段再次失败先诊断修复，不继续盲目重试。",
    "historicalReleases": []
  },
  {
    "fingerprint": "verification/registry-probe/local-fetch-network",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地 Node fetch 直连 Docker 认证端点连接超时，未取得 registry 身份证据；该探测没有镜像或生产写入。",
    "recoveryEvidence": "改用本机可用的 Invoke-RestMethod 读取官方 Docker Hub API，pre-retry-hub.json 记录实际 HTTP 状态与 preview/latest 摘要；不将失败的 Node fetch 当作版本未占用证明。发布器自己的 unused-version 强制门禁仍然执行。",
    "permanentAction": "该主机独立 Hub 回读使用已验证的系统网络通道；本地网络失败保持未验证，不据此跳过 GitHub-hosted 发布门禁。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 继承 RC14 的 HTTP API / secure-cookie 配置边界与安全审计缺口；不宣称本轮已修复。既有维护与供应链观察期限保持不变。
- 实体手机触摸、原生缩放/读屏、长时资源趋势、真实 NAS/混合旧节点与 systemd/OpenRC 宿主升级未新验。
- 稳定发布仍须重新冻结精确候选并执行完整 L3 和公开产物/用户旅程验收。
