# KPanel v1.30.1-rc.2 发布验收记录

日期：2026-11-14

发布级别：L3

候选提交 / 标签：`d07215f8a544bd47f0dace1b0ced8587a325e1cd` / `v1.30.1-rc.2`

上一稳定版本 / 回滚点：`v1.30.0@cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa` / `sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`

`releaseChannel`：`preview`；`releaseTrain`：`1.30.1`。

## 发布画像与范围

- 浏览器业务；L3。新增显式选装的 Tor Browser 原生桌面，Chromium 保持默认，Tor 首轮仅 Linux amd64。多个窗口共享同一桌面，安装后类型锁定，先卸载才能切换；停止/卸载丢弃临时会话。
- 从公开 RC1 e82c1a5d 合入来源 feature/browser-tor-component@b7cdcb4a 的四提交 89e2a30a、9b3e66ef、0f945ccc、b7cdcb4a；保留 RC1 音乐列表、切歌定位和 FLAC 修复，相关音乐产品文件未变。
- Panel 经管理员 Session、Origin、首帧 CSRF 转发有界 RFB，不托管上游浏览器 HTML/JS；独立私有控制网络和浏览器出口网络，无宿主端口、数据挂载或 Docker socket。
- 包下载、签名核验和本机构建由 Agent 后台任务完成。对 Windows/ARM64 不声称 Tor 支持；不新增脚本要求、持久化 schema 或生产部署。

## 来源、修复与安全边界

- Tor 16.0.1 Linux amd64 安装包 SHA-256 `5b36ad610c20c81faa72caf27d1cd3a11f14a327c320b4cec581400016ba78b0`，主签名指纹 `EF6E286DDA85EA2A4BA7DE684E2C6E8793298290`；先核验官方包，后覆盖两个重建助手。16.0.2 是 Android 更新，未冒用为 Linux 包。
- 用户明确授权重建升级两个网桥助手后再发布。Conjure 来源 `14e660f0a64bcbb2b55f01f69c1aafdfd5a6dcfa`、Lyrebird 来源 `59ac7e400c384773f5584dbe366516ad04a33d9f` 保持原应用源码；升级六项显式安全依赖及必要传递依赖，Go 1.27.4 固定构建。新助手不是 Tor 官方签名包内的原始字节。
- 两助手 go.mod/go.sum、来源归档摘要、manifest、嵌入锁包及构建脚本已核对。最终可选镜像保留模块清单、二进制摘要与108份许可证文本；安装镜像标签 `kpanel-browser-tor:16.0.1-kpanel.1` 避免复用旧原包缓存。运行主机未来重建时，Ubuntu 签名仓库内容可能变化，不能承诺跨时间字节相同。
- 原镜像4项 Ubuntu HIGH、助手22+2项 HIGH匹配及 govuln symbol finding 原件保留。Ubuntu 安全更新及助手重建后，本地最终镜像 `sha256:9e8cfb634187d55a0009e8c85c2b37712a5e251b84428149324666c413d892f1` 的固定 Trivy HIGH/CRITICAL、secret 均0；两实际二进制的固定 govulncheck v1.6.0 普通文本检查通过。JSON模式exit0不表示无finding，没有使用该退出码冒充通过。
- GUI 基座需 root 初始化/监督进程；两份 Tor Dockerfile 的 DS-0002 例外仅精确路径、单规则、截至2026-11-21，责任人与退出条件在 .trivyignore.yaml。正常运行 Firefox UID/GID65532、CapEff0、NNP1、Seccomp2、只读rootfs；不是完全rootless容器。内存1536MiB/CPU1.5/PID512，临时配置与日志有界。未放宽其他漏洞、秘密或配置规则。
- 来源安全盲审 recorded；本次未执行 CF scoped/full 或稳定补漏，不声称全仓无漏洞、完整 Firefox 内容沙箱或匿名性保证。已有主Go图漏洞 disposition/日期不改，嵌套助手模块另行锁定与检测。
- helper 源码/策略独立复核 PASS WITH FOLLOW-UP，唯一 LOW 文案已修正；bootstrap 独立复核 PASS，唯一 LOW 公开预览元数据已修正。提供商fallback=provider-unavailable，使用与实现者分离的干净 Codex 会话；最终验证/发布由该独立任务接管。
- OCR1.12.11 双臂原件：助手增量12/12圈选、24/24处置、H0/M0/L1、constrained-only0；bootstrap3/3圈选、4/4处置、H0/M0/L1、constrained-only0。来源及先前不盲的复核按原记录保留，未将未报告值补成0。

## 跨仓库联动

`scriptLinkageState=not-required`，无需发布脚本（不适用）。沿用 `kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff`，SHA-256 `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`；没有新脚本候选或跨仓发布。

## 多维质量与隔离验证

| 维度 | 状态 | 证据 | 未验证范围 |
| --- | --- | --- | --- |
| 业务与双端互通 | 已验证 | 来源L2、受认证原生画面、最终设置页4旅程及两助手8种managed transport启动/退出 | 本轮未新增公网网桥/Tor出口验证；来源出口证据不能替代重建助手的真实网桥验收 |
| 供应链与权限 | 已验证 | 固定源/模块锁、签名边界、最终镜像扫描与运行时字段 | 没有匿名性、完整内容沙箱或全仓无漏洞保证 |
| 稳定与恢复 | 已验证 | 类型锁定、卸载重选、409失败恢复、标准部署/网络回归及默认Chromium生命周期夹具修复 | 长期负载、各种宿主发行版和生产升级未执行 |
| 性能与资源 | 已实现未实机验证 | 下载、容器、tmpfs与日志上限，本地构建和启动成功 | 冷构建资源峰值、低配/多用户吞吐未量化 |
| 体验与可访问性 | 已验证 | 1440/390px、CSS200%等效几何、设置键盘焦点、页面错误0 | 实体手机、原生缩放、完整读屏与Tor触屏矩阵未执行 |
| 数据与迁移 | 已验证 | 无新schema，旧任务kind默认兼容，音乐文件保留 | 临时Tor会话不承诺保留，没有生产数据迁移 |

本地附加验证使用注册的 local-wsl-dr；完整发布L3仅由 GitHub-hosted canonical 工作流执行，未在本机或自托管服务器替代。最终镜像的构建输入与 d072 逐字匹配，绑定见 helper-rebuild/final-image-candidate-binding.json。无网络运行 GUI200；协议检查覆盖7种 Lyrebird transport及 Conjure的 VERSION1/CMETHOD/CMETHODS DONE/关闭stdin退出0，不等同于公网桥接。

## 自动门禁与发布事实

- 最终候选 push CI [43548162616](https://github.com/kejilion/KPanel/actions/runs/43548162616) Linux/Windows成功；同SHA依赖新鲜度43548162567。
- 最终task-preflight source identity、业务事实、clean候选及handoff通过。第一方1321文件source SHA-256 `8689424d214c6f52fd80d216fcfd12da3977b48e03118f0cbf060159a8d63ee2`；主Go模块图不变，生产闭包最终核验以canonical证据为准。
- 本地检测器34测试分别在root-only准备树及Tor登记候选通过；准备树完整治理391测试通过，最终Changelog草稿渲染及运行时selfupdate解析器通过。草稿零digest不是公开镜像摘要。
- 预dispatch发现可信scripts树不匹配后，先独立复核三脚本准备候选ee613，候选CI43547994831与主线CI43548901087成功；正常ff main并核对远端，RC候选继承准备提交。没有修改发布workflow/可信范围。
- 唯一 [canonical发布43549062064](https://github.com/kejilion/KPanel/actions/runs/43549062064) 于2026-11-14T02:57:19.666Z dispatch；GitHub-hosted L3于02:58:01Z至03:12:24Z通过，Go1.27.4/Node24.22.1/Docker28.0.4；Web363文件/3743通过/6跳过，Go与部署lane均通过，源码日志摘要及入口摘要已独立核对；隔离安装/升级/回滚及image_e2e通过。main及annotated tag已提升d072，主线Linux/Windows、公开产物/浏览器验收及preview-completion均通过，整轮workflow成功。
- 43503104515业务旧基线、43503488387 Tor DS-0002、43506292984旧hosted夹具失败原件保留；cb647的CI43509680218通过但不能排除当时助手风险。82137的43547218385因更新候选自动取消，不计为通过或产品失败。原型首轮uTLS/Brotli残余finding与后续升级通过结果分开保留。

## 公开产物与更新验收

- [Release](https://github.com/kejilion/KPanel/releases/tag/v1.30.1-rc.2)：ID444994677，公开时间2026-11-14T03:51:30Z，draft=false、prerelease=true，19附件与冻结清单的名称/大小/digest一致。实际公开正文与最终RELEASE_NOTES.md逐字相同，SHA-256 `cd39ba7409ab639b226c81d489857df24695d481bdaf0fc4773647434a58e8ef`；未重新渲染或改写已发布正文。
- 不可变版本镜像与preview均为 `sha256:2dda9c65a1defaaa77b8fbbc9c7360465f37018f1bebec7b42ccf31302856b51`，包含linux/amd64与linux/arm64，此外的unknown/unknown条目为构建证明。稳定GitHub Latest仍v1.30.0，Docker latest仍 `sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`。
- 匿名公开附件/OCI检查、真实隔离容器initial/restart、真实浏览器published/upgrade全部通过；四份report合计150项检查、0未预期错误，candidate=d072、digest匹配、mocked=false。公开SHA256SUMS列15项，元数据含6条摘要与3条升级提示。容器journey中的Docker inventory和CA trust为明确测试夹具，不外推生产Engine或CA/宿主能力。
- 当前版本/摘要/说明与更新弹窗绑定RC2；旧RC1后端发现RC2，预期no-systemd安装派发返回受控503且清理请求，未冒称生产安装成功。加入预览不启用自动安装，退出预览不自动降级；1280/390/320px弹窗与交互结果见公开升级report。
- 证据原件：仓库外 `C:/GitHub/_release-artifacts/v1.30.1-rc.2/helper-rebuild/canonical-43549062064-1/` 的l3/snapshot/publication/public，原始ZIP及API digest均保留；live-registry-channel-verification.json与publication-freeze-and-live-metadata-verification.json独立核对外部状态。
- [完整工作流](https://github.com/kejilion/KPanel/actions/runs/43549062064)所有必需阶段success；publish/l3因已完成canonical L3而按原流程复用，stable-completion因preview不适用而skipped。没有重复dispatch。

## 生产、回滚及分支处置

- 生产动作全部不适用（仅产物发布）；本次未发起服务器SSH运维连接，没有连接、备份、升级、部署或清理prod-108及其他生产主机。
- 稳定v1.30.0和上一公开RC1保留；退出预览不自动降级，人工回退需先备份并选择固定版本/digest。旧官方助手存在已知匹配，不将回滚源码等同于可发布修复。
- release/v1.30.1-candidate保留RC列车。来源作者工作树/分支所有权未知，保留为已纳入待处置；不搭车删除。
- bootstrap精确ee613已在其mainCI成功后原子保存为archive/fix/tor-release-tooling-20261114并移出active，远端readback一致；本地分支同名archive、ee613、clean、无upstream，工作树保留。没有重写远端历史。
- 旧验收草稿明确未发布；本验收记录在公开通过后保存为独立本地文档候选，不修改产品tag/镜像或制造新RC。已停止绑定d072的本任务mock API和Web进程，来源作者预览未触碰。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-14T03:20:19+08:00
- 候选冻结时间：2026-11-14T10:46:11+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：10
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/business-context/stale-review-baseline",
    "position": "before-production-write",
    "count": 1,
    "impact": "43503104515 在旧业务事实基线累计51提交处失败，未发布RC2。",
    "recoveryEvidence": "candidate-ci-first-failure.log；afd49b1e完成事实复核；final-business-context.log与最终候选CI43548162616通过。",
    "permanentAction": "沿用权威check-business-context-freshness入口，在本次contract加入冻结前带摘要检查，不降低阈值。",
    "historicalReleases": []
  },
  {
    "fingerprint": "candidate-ci/browser-fixture/stale-resource-fingerprint",
    "position": "before-production-write",
    "count": 1,
    "impact": "43506292984真实Chromium生命周期因旧四字段测试指纹而失败，生产协议已含组件类型。",
    "recoveryEvidence": "candidate-ci-third-failure.log；9b1364c7只同步夹具第五字段，保留七动作及生产并发冲突保护；独立复核及最终候选CI通过。",
    "permanentAction": "修正唯一hosted夹具internal/browsercomponent/testdata/live-component-task.mjs，继续由canonical真实生命周期验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "component-validation/runtime-probe/shell-pid-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次进程取证命令把PID丢失，读取/proc//status失败，不能作为运行边界证据。",
    "recoveryEvidence": "root-init-probe-first-collection.log保留；修正采集器后root-init-runtime-receipt.json及helper-rebuild/final-runtime/receipt.json记录真实PID安全字段。",
    "permanentAction": "本次采集改为pgrep结果校验后以argv读取固定/proc/PID/status，保留失败和成功原件；未改产品安全配置。",
    "historicalReleases": []
  },
  {
    "fingerprint": "component-validation/trivy-checks/missing-proxy-routing",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地Trivy完整规则包首次下载受网络路由阻塞，早期内置规则不能替代完整规则验证。",
    "recoveryEvidence": "trivy-check-bundle-network.log及首次检查原件保留；使用既有代理路由后trivy-path-scope-current.json正确仅对两精确路径豁免，未匹配路径继续HIGH。",
    "permanentAction": "本次扫描容器传递既有代理环境名并使用标准路由，保留TLS与完整规则检查；最终镜像扫描同样按固定扫描器执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "component-validation/build-receipt/windows-worktree-pointer",
    "position": "before-production-write",
    "count": 1,
    "impact": "Ubuntu安全更新镜像构建已成功，但收据步骤用WSL Git读取Windows linked-worktree指针失败。",
    "recoveryEvidence": "tor-security-build.log证明导出成功；recover-tor-build-receipt.mjs使用native Windows Git复核保留的配方、构建上下文和镜像ID，未重建覆盖原件。",
    "permanentAction": "本次最终助手构建以输入摘要落盘，再由native Git校验准确候选字节；final-image-candidate-binding.json绑定d072与实测镜像。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-governance/toolchain-probe/missing-go-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次本地治理前提检查未设置已安装Go路径，go-native/go-repo-bash/upstream-source-bytes失败。",
    "recoveryEvidence": "helper-rebuild/governance-first-toolchain-unavailable.log；显式设置已有Go1.27.4的GOROOT/PATH后governance.log通过，bootstrap-governance.log完整391测试通过。",
    "permanentAction": "复用现有task-preflight治理探针与run-repo-bash权威入口，后续本任务门禁显式选择已有Go安装；没有安装替代工具链或改变探针。",
    "historicalReleases": []
  },
  {
    "fingerprint": "component-validation/protocol-collector/text-buffer-readiness",
    "position": "before-production-write",
    "count": 1,
    "impact": "协议测试首轮TextIO与selectors混用漏读已缓冲的CMETHODS DONE，两个助手均退出0但采集判定false。",
    "recoveryEvidence": "helper-rebuild/helper-protocol-first-collector-failure.json；采集器改为os.read按字节分行后helper-protocol-receipt.json记录8种传输完整协议及stdin关闭退出0。",
    "permanentAction": "修正本次仓库外protocol采集脚本，仍要求VERSION1、全部CMETHOD、CMETHODS DONE和exit0，不删除断言或改产品。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview-preflight/trusted-control-tree/candidate-only-tooling",
    "position": "before-production-write",
    "count": 1,
    "impact": "82137预dispatch检查发现三个scripts仅在候选，可信控制树与main不同，尚未dispatch即停止。",
    "recoveryEvidence": "helper-rebuild/publication-preflight-initial.json；独立bootstrap ee613通过同SHA候选CI后ff main；d072继承相同脚本且pushCI通过，43549062064 inputs成功，唯一dispatch。",
    "permanentAction": "按既有主线准备纪律预集成三脚本；collector只选择策略已声明Tor源，新增root-only/缺配方失败/真实候选pin回归，不改变发布可信范围。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview-preflight/registry-occupancy/direct-node-routing",
    "position": "before-production-write",
    "count": 1,
    "impact": "唯一发布任务首次用本机Node直连DockerHub版本占用API时未继承系统代理，出现UND_ERR_CONNECT_TIMEOUT，未dispatch或解释为标签不存在。",
    "recoveryEvidence": "helper-rebuild/prepublish-registry-first-failure.json保留；只读复用既有系统代理后prepublish-registry-final-response.json返回404，publication-preflight-final.json确认版本未占用。",
    "permanentAction": "本任务后续注册表检查复用既有系统代理路由，保持TLS和失败即停止的占用判断；未改仓库发布规则或重复dispatch。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance-evidence/workflow-logs/direct-gh-routing",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开验收成功后的辅助gh格式化日志直连下载出现peer CANCEL或长时间无输出；此为取证通道问题，没有重新发布或生产写入。",
    "recoveryEvidence": "停止唯一仍等待的自有只读gh进程，既有代理取GitHub原始日志API成功；canonical-workflow-log-receipt.json记录6517214字节ZIP与SHA256，主要L3/公开产物原件此前已保留。",
    "permanentAction": "本次最终取证使用原始Actions日志ZIP并保留完整展开文件，格式化原文从ZIP提取；未修改canonical工作流或重跑发布。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

上述为有可核对原件的流程事件；首次检出真实漏洞/DS-0002、预期rootless失败和候选被后续push取消单列，不据此虚构生产变更失败。若干准备命令用错工作目录及Windows Go联网超时在会话工具记录中保留，缺少统一原始时序，不把未归集次数推断为零。

## 遗留边界

未验证公网网桥/重建助手的抗审查表现、完整内容沙箱/匿名性、ARM64 Tor、长时与低配性能、实体手机和完整可访问性。root-init例外2026-11-21到期，由组件维护者复核退出条件。保留实测镜像、构建上下文、源码/扫描/失败原件和工作树；没有磁盘清理或净释放量声明，没有新增知识、工作流或周期任务。
