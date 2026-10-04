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

### 5.1 Windows 轻量节点签名产物

纳入 Windows 完整功能后的每次 Release 必须执行 Windows 配置检查、构建、签名和验证，不再通过
可选仓库开关跳过。只有 Windows 任务成功，后续发布任务才可运行；配置缺失、签名失败、取消或跳过
均不得公开缺少三项已签名 Windows 附件的版本、镜像或通道。实际签名配置以该任务的检查和签名结果
确认；构建成功不等于具备公开安装或真实 Windows 服务生命周期的验收证据。

签名任务运行在独立 `windows-2025` Runner 和 `windows-node-signing` Environment，只有读取仓库的
权限，不持有 Release 写权限。管理员应为该 Environment 配置发布 Tag 限制和人工保护，并按
[Azure 官方签名集成](https://learn.microsoft.com/en-us/azure/artifact-signing/how-to-signing-integrations)
预先完成身份验证、Public Trust 证书配置及最小范围 Certificate Profile Signer 授权。

| 配置位置 | 名称 | 含义 |
| --- | --- | --- |
| Environment variables | `KPANEL_WINDOWS_NODE_PUBLISHER` | 证书完整 Subject，区分大小写，与中心端信任配置一致 |
| Environment variable | `KPANEL_WINDOWS_NODE_PROFILE_OID` | 可选的稳定 profile EKU OID；使用 Artifact Signing 时建议固定 |
| Environment variables | `KPANEL_SIGNING_ENDPOINT`、`KPANEL_SIGNING_ACCOUNT`、`KPANEL_SIGNING_PROFILE` | 区域 HTTPS endpoint、账号、证书 profile 名称 |
| Environment secrets | `KPANEL_AZURE_TENANT_ID`、`KPANEL_AZURE_CLIENT_ID`、`KPANEL_AZURE_CLIENT_SECRET` | 仅用于配置检查和签名步骤的应用身份；不得写入源码、产物或日志 |

Panel 端需配置 `KEJILION_PANEL_WINDOWS_NODE_PUBLISHER` 为同一实际签名 Subject，可选配置
`KEJILION_PANEL_WINDOWS_NODE_PROFILE_OID` 为同一 profile OID。发布者为空时安装入口 fail-closed；
功能源码纳入发行不代表已经默认配置信任身份或完成真实安装、RDP 验收。

工作流固定使用官方 `azure/artifact-signing-action` `v2.0.0` 提交
`c7ab2a863ab5f9a846ddb8265964877ef296ee82`。该上游版本固定 ArtifactSigning 模块 `0.1.8`、
Windows SDK BuildTools `10.0.26100.4188`、ArtifactSigning.Client `1.0.128`，内部还声明 SignCLI
`0.9.1-beta.26227.3`；本项目只签 EXE/PowerShell，不增加 MSIX 路径。升级时需同时核对这些传递依赖。
签名依赖缓存关闭，Azure CLI 及其他替代身份路径关闭；私钥保留在 Azure 服务端。
Actions 的完整 SHA/版本由现有 `dependency-policy.json` 的 `github-actions` 分组自动枚举，不另建清单。

固定发布附件为 `kejilion-node-windows-amd64.exe`、`kejilion-node-windows-arm64.exe`、
`install-windows.ps1`。源码安装器是 `deploy/windows/install.ps1`：构建时先确定最终 UTF-8 BOM/CRLF
字节，再统一 Authenticode SHA-256 签名并附 RFC3161 时间戳。验证器要求 Windows 信任链有效、
发布者完全匹配、代码签名 EKU 和已配置的 profile OID 一致；不固定短期叶证书指纹，允许同身份轮换。
所有文件通过后才生成 `SHA256SUMS.windows`，Linux 任务只下载同一次 workflow 的签名产物，复核文件
清单和摘要，然后合并到公共 `SHA256SUMS`；签名后不得再次改编码或修改 EXE。

本地可运行 `scripts/tests/windows-node-release.test.ps1`、
`node --test scripts/tests/merge-windows-release.test.mjs`；前者真实拒绝未签脚本，并用模拟 OS 验签结果
覆盖身份/轮换/时间戳/篡改边界，后者覆盖跨 Runner 文件完整性。CI 另执行 Windows 原生测试和双架构
构建，不上传未签产物。正式发布仍须验证真实 Azure 签名、下载后验签与 SHA-256、amd64/arm64 实机安装、
升级和中断恢复，不能以模拟测试或交叉编译替代。RC 附件只供主动选择的隔离验收，无人值守节点仍跟随稳定版。
