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
- 轻量 Node 的无人值守更新继续只跟踪稳定版。Release 可以附带 RC Node 二进制供隔离验收，但设置页的
  预览版计划不隐式改变远程 Node 策略。

## 5. 发布与验收

稳定版和预览版均属于 L3 发布，必须通过同一源码、双架构、供应链、镜像运行时、安装生命周期和公开
产物门禁。额外检查至少包括：

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
