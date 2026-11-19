# KPanel v1.30.1-rc.3 发布验收记录

日期：2026-11-14

发布级别：L3。候选 / 标签：`f656291688401a7e5232f97659810d98ff0c95c8` / `v1.30.1-rc.3`。

本轮批准基线与上一预览：`d07215f8a544bd47f0dace1b0ced8587a325e1cd` / v1.30.1-rc.2。

上一稳定版本 / 回滚点：v1.30.0@cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa / `sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`。

releaseChannel=preview；releaseTrain=1.30.1。用户授权新的本地提交发布预览版；生产部署不在本次范围。

## 新增与变更

- 原生系统剪贴板中的图片和文件可以粘贴到桌面、文件管理器及桌面内的文件窗口。非模态桌面窗口不阻止上传；输入框、编辑器和终端保留原生粘贴。
- 上传、粘贴、复制、移动、回收站恢复、归档解压、跨主机接收与 HTTP/BT 后台下载统一处理同名冲突。文件提供覆盖、保留两份、跳过；文件夹提供合并、保留两份、跳过；同一任务内文件与文件夹的选择分别记忆。
- 后台任务可停在等待冲突决策状态并恢复。合并保留目标独有项；部分移动不删除跳过、失败或未完成的源。用户批准后再次核对对象版本和目录身份，变更后重新确认，不静默覆盖。
- 修复排队重命名批准对象版本、跨主机接收取消/保留清理/终态顺序、旧节点能力捕获与窄屏冲突按钮及多语言文案。保留 RC2 Tor 组件/网桥助手修复及 RC1 音乐布局与 FLAC 修复。
- 不要求配套脚本新发布。scriptLinkageState=not-required；沿用 kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff，SHA-256 5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23。

## 精确来源与集成

| 原分支 | 固定 tip | 本轮处置 | 保留/责任边界 |
| --- | --- | --- | --- |
| feature/system-clipboard-upload | 31efb62cb8edbd0e8f36050490d67996d9a73de3 | 已被文件冲突候选包含，随 RC3 纳入，未重复合并 | 原作者工作树/所有权释放未再次确认，本地保留待处置，已发布内容不再算待发布候选 |
| feature/file-conflict-policy | a4b31d1edd53592f795127a2f9b9b8a2ab1e8a78 | 完整候选非快进合入，RC3 tag 可达 | 当前发布任务负责集成；源作者分支/工作树保留，未删除未知归属内容 |
| docs/file-conflict-design | 2fa838d03c265b35c49867dd7620a03f04f09fa1 | 与候选内7e624ee0的设计文档逐字相同 | 保留原分支，避免重复合并；归档待明确所有权 |
| docs/v1301rc2-acceptance | ae1ddd7f9f5c1f0e8bc31120b48d470b0002b05b | 非快进合入，验收文档原样保留 | 历史验收工作树保留；远端归档未执行 |
| release/v1.30.1-candidate | f656291688401a7e5232f97659810d98ff0c95c8 | RC3发布候选 | 同一预览序列按项目规范保留，稳定版后归档 |

本地新候选盘点已覆盖分支与 detached HEAD；其余旧候选早于 RC2，不搭车合并。本次未扩大到新远端候选。源产品实现与 a4b31d1e 比较，仅版本三个文件及发布文档/供应链身份元数据不同；未在发布任务改写功能实现。main 与 annotated tag 由 canonical 发布流程提升；管理工作树未重置。

## 独立复核与来源证据

- 来源证据目录：`C:/GitHub/_validation/kpanel-file-conflict-20261114`。交付、contract、handoff、原始失败及修复记录保留。a4b31d1e 的隔离 WSL2 ext4 L2 于2026-11-14T09:16:47.713Z通过，Go1.27.4/Node24.22.1；原始日志 SHA-256 `1844425702c0f6459f5e2d643f93eb13b941a0f08e75a6563322839c34de95a7`已核验。
- 来源独立复核最终 PASS；实现者与复核者为分离任务，提供商 fallback=provider-unavailable。发布任务独立核对原始报告、修复链与测试，未仅凭提交 trailer 宣布通过。来源反例含目录同意/身份、创建收据、接收清理顺序、排队重命名版本，均保留首次失败与后续验证。
- 安全盲审覆盖声明的文件管理、核心与 Panel/Agent Linux 路径，原始报告摘要已与交付核对；没有未解决的已确认问题。本次没有 CF scoped/full 审计，不外推全仓安全、Windows 动态语义或完整分享/回收站业务。
- OCR1.12.11最终圈选79/79代码文件（83总文件中4文档unsupported_ext），19项测试；历史 free-form=1、valid=H0/M1/L0 的排队重命名对象版本问题已修复，当前未解决0。后续阶段不盲及 constrained-only=unreported 按原件保留，未补成0。
- 本任务最终 f6562916 的 acceptance mock 预览完成9项旅程，覆盖剪贴板图片、覆盖/保留两份/跳过、对象变化重问、文件夹合并、取消、430px暗色/CSS200%/焦点及搜索框原生粘贴；页面错误0。此为内存元数据 mock，不是宿主写入证明。来源真实文件系统/Panel-Agent隔离集成与公开容器验收分别记录。

## 多维质量与边界

| 维度 | 已验证事实 | 未验证或限制 |
| --- | --- | --- |
| 业务与互通 | 来源精确L2、文件操作边界、9项UI旅程、GitHub-hosted L3及公开容器/浏览器验收 | 本轮没有双实体服务器全链路实机验收 |
| 数据与并发安全 | 对象版本/目录身份复核、目标独有项保留、部分移动源保留、取消/清理顺序回归 | 实际覆盖、移动、文件夹合并会改变数据，代码回滚不会撤销；重要文件应先备份 |
| 兼容与恢复 | Windows节点CI、旧节点能力兼容、任务待决策恢复、取消与部分完成状态 | 完整Windows动态文件系统语义与所有跨版本混合矩阵未验收；Panel/Agent/节点宜一起升级 |
| 体验与可访问性 | 窄屏、暗色、CSS200%等效几何、焦点与原生文本粘贴 | 实体手机、原生浏览器缩放、完整读屏未执行 |
| 稳定与性能 | 来源竞态测试、限制与失败边界、隔离升级/重启通过 | 24小时等待、崩溃/磁盘/网络故障组合与长期吞吐未穷尽 |
| 供应链 | 主Go图及6目标闭包与original配对一致，候选CI/L3和发布镜像准入执行 | 不承诺无未知漏洞；RC2 Tor既有受限例外/范围继续有效 |

## 自动门禁与供应链身份

- 候选 push CI [43581372970](https://github.com/kejilion/KPanel/actions/runs/43581372970) Linux/Windows成功；依赖新鲜度43581372964成功。最终 source identity、业务上下文与clean候选 handoff 已通过；首次本地缺Go路径异常及恢复原件保留于下方流程指标。
- 第一方1344个Go/proto源文件摘要 `cb94ad56053c18df7ca173d6a33218ba7bba4023930ad1f4b231f8378b542288`；生产闭包摘要 `9018045d7dda52d2fe280bd2ab58b4809466807724913b5aaa9841b506367a35`。Go1.27.4下 Linux/Darwin/Windows × amd64/arm64 的 actual/original 12份闭包原件一致；go.mod/go.sum及模块图未变。
- 沿用已有38项漏洞 disposition、2026-11-06观察时间与2026-11-20有效期；未重置日期或压制新finding。仅重新绑定source/closure身份，scanner通过以实际CI/L3为准。
- 冻结前发布说明草稿使用真实selfupdate解析器通过，草稿零digest不被用作公开摘要。可信workflow/scripts树与 dispatch main 一致，不需要 bootstrap。
- 唯一 [canonical发布43582877568](https://github.com/kejilion/KPanel/actions/runs/43582877568) 全部必需阶段 success；controller=`d07215f8a544bd47f0dace1b0ced8587a325e1cd`，candidate=`f656291688401a7e5232f97659810d98ff0c95c8`。未重复dispatch。L3=2026-11-14T09:57:00.043Z至2026-11-14T10:15:52.986Z，环境github-hosted，go version go1.27.4 linux/amd64 / Nodev24.22.1 / Docker28.0.4；源码身份、入口及web/go/deploy三lane日志摘要逐一核验。主线Linux/Windows、隔离安装/升级/回滚、镜像安全准入与公开验收通过。
- publish内重复L3按canonical复用而skipped；stable-completion因preview不适用而skipped，不把跳过写作新测试通过。

## 公开产物验收

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.30.1-rc.3)：ID445227666，公开于2026-11-14T11:00:18Z，draft=false、prerelease=true；annotated tag对象`35837de3658d889eafa590653a2feca20e67d4d1`指向`f656291688401a7e5232f97659810d98ff0c95c8`。
- 19附件的名称/大小/摘要与冻结清单一致。实际公开正文与冻结RELEASE_NOTES.md逐字一致，SHA-256 `199b9c8aa35534cd09889d8f67b1e620413a00aeb0bfd4d21eb9a26dc1d1e185`；没有重新渲染或修改已发布正文。
- 版本镜像与 preview 均为 `sha256:4e7cfce82b4421acb11a9e574504aa430f5dbebe6b1a655f1887df0c8d4e9d93`；linux/amd64和linux/arm64的平台摘要及revision/version标签匹配候选。其余unknown平台条目为构建证明。独立实时注册表查询通过。
- GitHub Latest仍v1.30.0，Docker latest仍`sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`；上一RC2版本digest保留于冻结snapshot。
- 匿名公开附件/OCI校验、真实隔离容器initial/restart与浏览器published/upgrade共4份report通过，合计150项检查、0未预期错误，均绑定candidate/digest且mocked=false。容器的Docker inventory及CA trust为明确夹具，不外推真实生产Engine/CA。
- 当前版本、说明及更新弹窗绑定RC3，旧RC2后端发现RC3；no-systemd环境的受控503安装派发为预期失败，未冒称实际生产安装成功。加入预览不会启用自动安装；退出预览不会自动降级。
- 原始ZIP与GitHub API digest已校验并保留在`C:/GitHub/_release-artifacts/v1.30.1-rc.3/canonical-43582877568-1/`。完整L3、publication snapshot、附件冻结及public原件在各自artifact目录；本任务额外live核对见publication-freeze-and-live-metadata-verification.json、live-registry-channel-verification.json。

## 生产、回滚与本地收尾

生产连接、备份、部署和运维全部不适用。本次没有连接或操作生产服务器；稳定频道未变。回退可选择上一固定RC2或稳定v1.30.0，但不会恢复已经完成的文件覆盖/移动/合并，重要数据先备份。

仅停止本任务4183 acceptance mock预览；来源作者4182预览及未知所有权工作树保留。未执行磁盘删除或声称净释放空间。发布候选列车保留，来源分支按上表列为已纳入/待所有权处置，后续明确释放且clean后再按项目规范归档，不因收尾重复发布。验收记录在公开通过后形成独立本地文档候选，未制造额外RC或推送记录分支。未新增知识、工作流或定时任务。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-14T13:06:56+08:00
- 候选冻结时间：2026-11-14T17:33:34+08:00
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
    "fingerprint": "local-governance/toolchain-probe/missing-go-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地handoff首轮Shell未设置已有Go路径，缺少工具检查失败；串行命令未在此处停止并继续推送候选。尚未执行正式dispatch/main/tag/Release或生产写入。",
    "recoveryEvidence": "handoff-first-missing-go-path.log与实际候选推送记录保留；显式选择已安装Go1.27.4后，同一f6562916的handoff.log通过。此前六目标闭包和发布说明解析已使用该Go安装通过，未把失败改记成功。",
    "permanentAction": "本任务后续明确GOROOT/PATH；watch-publication执行器核对命令退出码、精确CI和远端身份后才允许唯一dispatch，不修改仓库门禁或源代码。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-evidence/l3-entry-identity/wrong-hash-target",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3已通过，辅助本地核验器误对launcher计算摘要，发生entry hash比对失败；未作为产品失败，也未重新发布。",
    "recoveryEvidence": "l3-entry-first-collector-mismatch.json保留错误目标与正确门禁摘要；依据github-release-l3.mjs:50改为精确Git blob scripts/run-release-gate.sh；最终L3/公开证据核对结果另行保留。",
    "permanentAction": "修正本任务仓库外采集器的唯一摘要目标，不更改产品、门禁或任何已发布原件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-evidence/local-collector/windows-esm-import",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地采集器首次导入已有验证函数使用Windows路径而非fileURL，Node载入阶段失败；未读取为通过或改变发布。",
    "recoveryEvidence": "collector-first-esm-import-failure.json保留；改为file:///C:/绝对URL后执行L3原件核对。",
    "permanentAction": "本任务仓库外ESM采集器使用fileURL导入；后续组合Shell显式检查原生命令退出码。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-evidence/artifact-collector/dockerbuild-archive-format",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开发布后辅助取证将Docker build record的gzip原件按ZIP解包失败；原始下载与API digest匹配，未重新发布。",
    "recoveryEvidence": "artifact-first-dockerbuild-extraction-failure.json、原始下载与artifacts-api.json保留；改为按opaque记录保留dockerbuild，仅展开canonical ZIP证据。",
    "permanentAction": "本任务采集器区分Docker build record与canonical ZIP格式，全部下载继续强制核对GitHub API digest。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

上述为当前发布任务有原件的流程事件；来源作者首轮失败/后续修复在源交付链保留，不改写为当前通过前不存在的问题，也不据未归集历史推断所有工具错误为0。
