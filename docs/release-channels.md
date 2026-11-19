# KPanel 稳定版与预览版规范

- 状态：长期强制规范
- 适用范围：版本号、候选分支、GitHub Release、Docker 镜像、应用市场、自更新和发布验收
- 默认原则：稳定版是公共默认；预览版必须由用户明确加入，且加入预览版不等于启用自动安装

## 1. 版本与发布身份

KPanel 只接受以下两种规范版本，所有数字段禁止前导零且不得大于 `999999`：

| 通道 | 版本 | Git tag | GitHub Release | Docker 通道标签 | Release 镜像字段 |
| --- | --- | --- | --- | --- | --- |
| `stable` | `X.Y.Z` | `vX.Y.Z` | 已发布、非 prerelease、GitHub Latest | `latest` | `生产镜像` |
| `preview` | `X.Y.Z-rc.N`，`N >= 1` | `vX.Y.Z-rc.N` | 已发布、prerelease、不得成为 GitHub Latest | `preview` | `预览镜像` |

`alpha`、`beta`、`nightly`、`canary`、其他后缀及非规范大小写均不属于发布通道。运行中的开发版本可以在
上述规范版本后附加 `-dev`，但不得作为 Tag、Release 或镜像版本发布。

稳定版与预览版都必须拥有独立的不可变版本镜像
`docker.io/kjlion/kejilion-panel:<version>` 和 manifest digest。`latest`、`preview` 只作为通道发现入口，
安装和升级最终必须固定为 `@sha256:<digest>`。稳定版发布会重新构建并验证正式产物，不把 RC 的可变
通道标签当作正式版本证据。

## 2. 候选分支与提升

同一发布序列 `X.Y.Z` 共用 `release/vX.Y.Z-candidate`：

1. RC 按 `rc.1`、`rc.2` 递增；不得覆盖旧 Tag、Release 或版本镜像。
2. 预览版发布后保留候选分支，继续承载同一序列的修复和下一 RC。
3. 准备稳定版时，重新冻结精确提交并完整执行 L3；稳定版公开成功且候选提交已包含于正式 Tag 后，
   按 [`project-management.md` 10.2](project-management.md#102-分支归档与下一轮候选筛选) 保存精确 tip 到
   `archive/release/vX.Y.Z-candidate`，再移除活跃候选。归档与生产上线分别验收；预检或事务拒绝时保留原引用，
   推送后核验失败时按项目管理 10.2 核对实际引用与恢复证据，不推断远端未变化。
4. Docker `preview` 只指向最新已验证 RC，Docker `latest` 只指向最新已验证稳定版；两个标签不得互相替代。
5. 预览版不进入正式生产部署，不计入稳定标签形成的正式发布频率；可在登记的隔离验收环境中验证。

## 3. 用户更新策略

设置页中的两个开关是相互独立的策略：

- **加入预览版计划**：仅把更新来源从 `stable` 切换为 `preview`，持久化后立即检查一次；必须先展示
  风险确认，不会自动安装任何版本。
- **自动安装更新**：默认关闭。开启后，宿主机定时任务只安装当前通道中经过观察期的候选。
- **立即安装**：只允许安装本机已经检查并展示的精确版本和 digest；这是一次性请求，不会顺带开启
  后续自动安装。

稳定通道只读取 GitHub Latest 的正式稳定版。预览通道读取已发布的正式稳定版和规范 RC，并按语义版本
选择更高版本；同一 `X.Y.Z` 的正式版高于它的所有 RC。通道变化必须清空旧通道候选、失败隔离和未执行
请求，防止跨通道安装陈旧产物。

退出预览版计划只切回稳定来源，**不得自动降级**。如果当前 RC 高于 GitHub Latest，保持当前版本并等待
更高稳定版；当同版本正式版或更高稳定版出现时，它属于向前升级。人工回滚属于独立高风险操作，必须
明确选择目标 digest、备份并验证恢复，不复用通道开关。

## 4. 安装安全与平台边界

- Release 来源只信任 `kejilion/KPanel` 的规范 Tag、官方 Release URL、发布状态和唯一且标签正确的
  `生产镜像` / `预览镜像` digest；响应限长、拒绝重定向，不接受可变镜像引用。
- 策略更新和立即安装使用 `resourceVersion` 乐观并发控制。检查只发现候选，不获得安装授权。
- 立即安装由 Agent 记录一次性请求后，通过受信的 systemd unit 异步执行；浏览器关闭不影响任务。
- 更新前冷备份 Panel 与 Agent 数据；失败按既有事务恢复原版本和数据，并隔离失败的版本/digest。
- 完整 KPanel 自动更新当前只在 systemd 宿主机可用。OpenRC 继续保持“不支持自动更新”的显式状态，
  未完成等价 unit、恢复和真实 PID 1 验收前不得静默放宽。
- Linux 轻量 Node 的无人值守更新继续只跟踪稳定版。预览版可附带 Linux RC Node 二进制供隔离验收，但设置页的
  预览版计划不隐式改变远程 Node 策略。

## 5. 发布与验收

稳定版和预览版均属于 L3 发布，必须通过同一源码、双架构、供应链、镜像运行时、安装生命周期和公开
产物门禁。

发布说明格式、真实正文解析门禁及发布后的更新弹窗验收统一遵守 `PROJECT_RULES.md` 6.1；两种通道
都使用 `scripts/check-release-notes.sh`，不因 RC 身份放宽或跳过摘要与升级提示校验。

额外检查至少包括：

- 版本解析和排序：稳定版高于同版本 RC，RC 按数字排序，非法后缀 fail-closed；
- Release 选择：稳定来源排除 prerelease，预览来源只接受稳定版与 RC，并校验唯一官方 digest；
- 策略迁移：旧状态默认迁移到 `stable`，重启后保留选择；
- 行为分离：切换预览、检查、自动安装和立即安装不能互相越权；
- 退出预览：稳定版较低时不产生降级候选；
- 应用市场：默认入口继续使用 `latest`，但显式升级目标允许规范稳定版或 RC，并始终固定 digest；
- 发布工作流：稳定版提升 `latest`/GitHub Latest，预览版提升 `preview`/prerelease，候选分支清理符合
  本文第 2 节。

每次发布在 `docs/release-v<version>-acceptance.md` 记录 `releaseChannel`、`releaseTrain`、GitHub
Latest/prerelease 状态、版本镜像和通道标签 digest、候选分支处置、自更新通道用例及生产适用性。
可执行发布步骤以 `.codex-workflows/release-kpanel.workflow.yaml` 为准，验收结构以
`docs/release-acceptance-template.md` 为准。

## GitHub 发布执行与恢复

预览版入口为 main 的 `Publish preview`，候选 CI 与完整 L3 并行，均成功后才原子提升 main/tag；
主线 CI 通过后才运行 Release、Docker Hub 与公开浏览器验收。稳定版先通过同 SHA 候选/主线 CI，
再由 annotated 稳定 tag 的 `push` 触发 `release.yml`，执行完整 L3、Release 与公开验收；
预览版 dispatch 不接受稳定版本。两条路线均须先把已审查的发布自动化集成 main。
发布前的 stable/previous-preview 快照和源码身份先保存为 `snapshot-<run ID>-<attempt>` artifact。
公开后的验收恢复使用 main 的 `Verify published release` 只读入口，输入原发布 run ID/attempt；入口从该
run 下载 `snapshot-<run ID>-<attempt>` artifact，并核对候选、版本、run/attempt 与快照 SHA256。禁止手工
提交快照 JSON，不重建或覆盖已发布的版本镜像。新业务变更必须递增 RC 序号，不能复用已公开的版本/tag。

稳定版若在 Docker `latest` 提升成功后、GitHub Release 发布步骤失败，从 main 的
`Recover stable release publication` 输入原发布 run ID/attempt、精确 candidate/version/digest，
选择 `publish-draft` 或 `restore-latest`。入口只接受原稳定 tag `push`，核验两份原始 artifact、
19 个附件（含两个 Windows Node 二进制和两个安装脚本）的名称/大小/SHA-256、原最终正文SHA-256、不可变版本镜像与未改变的公开通道状态；正文只读验证，写入前再次核验。
许可证按原上传入口从仓库根目录读取；冻结阶段通过草稿可见查询解析精确 Release ID，并写入清单与身份工件。
恢复按该冻结 ID 读取和发布同一对象；前一预览允许来自更早补丁列车，但版本必须低于目标正式版，摘要仍精确绑定原快照。
恢复仍受 GitHub 的工作流权限检查约束：若目标 tag 的 `.github/workflows` 与后续默认分支不兼容，
`GITHUB_TOKEN` 可能无法更新该 Release，入口应停止；不能宣称任意后续主线状态下都可恢复，也不自动引入额外凭据。
`publish-draft` 只公开原冻结草稿；`restore-latest` 只在该 Release 仍为草稿且 GitHub Latest 未变化时，
把 Docker `latest` 恢复到原快照摘要。任何不确定状态均停止，不重建镜像、不改写版本标签或预览通道。
Release 已公开而验收失败时，继续使用上述只读 `Verify published release`，不使用发布恢复重发产物。
受限恢复工作流在公开验收后终止，不自动执行稳定候选归档；恢复负责人还须按发布工作流手册调用既有唯一归档入口，
核对原tag/candidate和公开验收成功后保存精确tip并移除活跃候选。不能将恢复验收成功直接称为全部归档收尾完成。

公开验收的产品始终绑定原 tag/SHA/digest；测试资源取当前可信控制面 `GITHUB_SHA` 的固定路径，
要求该提交已纳入 main。白名单仅含 `packaging/release-runner/Dockerfile` 和
`packaging/tests/release-browser/` 下的 `real-feature-public.sh`、`real-feature-browser-public.cjs`、
`run.sh`，不改变版本、HEAD 或产品镜像；四份 JSON 回执分别保存产品/控制面/run/attempt、
原新文件全文、摘要与差异。`run.sh` 仅以显式白名单将必需验收变量传给 `sudo env -i`，缺项拒绝执行。
这允许在 Alpine 修订版退出仓库后更新固定验收环境并只读恢复历史产物，不放宽 APK 固定版本、
公开断言或隔离限制。恢复记录必须区分原产品与新测试环境，保留原失败，不能声称冻结源树 clean
或原发布流水线全绿；未来发布也使用其已审查控制面的测试配方。
