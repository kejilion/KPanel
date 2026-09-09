# KPanel v1.10.0 发布验收

日期：2026-09-09。发布级别 L3。最终产品提交 a21d5bfc744b2a9316578dfca83426c001e2ee6d，目标标签 v1.10.0；main 基线 973a2b75efbb717896a45a3fa2f4b67dd164827e，上一稳定 v1.9.0/9028fab55d950cedc6e59325a7a28ba6ce21bd1c，旧 OCI sha256:b6afec34cb271659d0ad937f06fa1d0e1593a606ccb0b91bdbd6da8fa3fa9db6。

## 范围与业务真源

纳入 25bcb058d26d6a738991b3e3b84c2a9610c65381、58945fbfa58ce9c3e919c18cb007ba89eec8fd98、2405eb7a3f07d8d8af8c3869cee1852d04af0e05；无冲突重放为 53b6619、90d8fd0、5d49484，d689df7 准备版本。发布任务独立审查和验证，没有召回作者或新增复核任务。

已安装的兼容脚本应用可打开原生管理终端；实际容器绑定 expectedContainerId/resourceVersion，只有安装标记时保留原恢复入口。LibreSpeed、IT-Tools 和 DOS 游戏回归原生脚本规则。Docker 的 HostConfig.PortBindings 是停止容器端口配置的真源；旧回环绑定禁止 update/direct_access/manage，仍保留启停、卸载及域名相关的兼容动作，不自动扩大网络暴露。FRP 使用其专属原生菜单，本次不声称每个菜单选项都逐次重新校验容器。

安装界面使用“开始安装”“安装”等简明表述，雷池采用官网图标转换的 128×128 WebP，SHA256 6ab76da9785f6791ad7f68ed5060357177beb4f7683901b104936fe50bace4d5。终端右键菜单恢复组件原有 7000 层级，高于 5000 的任务弹窗；文件和 Docker 菜单原有层级保留。

候选 diff 已排除系统中心新增页面、路由、API、权限和业务文档，保留既有基线。未改变数据库 schema 或配对数据；未升级 Go/Node/npm 或基础镜像。所有真机验证及生产操作仅限 arena-154/154.36.153.9；108/prod-108 没有任何连接。

## 发布前产品缺陷与配对修复

原 d689df7 在隔离真机 r2 中安装了真实官方 IT-Tools 镜像。禁外网故障注入使 update 删除旧容器后无法拉取镜像，原脚本已经打印“更新失败”，但外层 linux_panel 在 case 后用协议判断覆盖退出码，Agent 收到退出 0，错误报告 succeeded。失败任务 f740c297f54f6cbed6190d66fa200bc3 于 2026-09-09T07:51:49.535Z 完成；这是产品缺陷，不以 L3 通过或“无自动回滚”备注豁免。本轮发布前拦截，旧候选未进入 main/tag/生产；不据此断言此前所有原生脚本使用者均不受影响。

scriptLinkageState=coupled，变更集 kpanel-v1100-native-app-status。脚本基线 aac8bc8710bb559ec7ae1d6988770f64564954ca/SHA256 583aff02c2f75510edc5135addad31163a7dfeeb9ac99fcf190b064fcbadedee；修复 9f612efc4f861459c0a525491c7cdf5eda756cf7/SHA256 9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663。根/CN 都捕获原始 case 状态并将其返回 KPanel 协议；普通 SSH 菜单不变。完整分发器回归覆盖 standard/plus/custom、成功 0、失败 1/37、目录刷新失败、普通菜单继续；可拒绝原脚本。根/CN 语法、同步、应用非交互、plus 生命周期、目录刷新 smoke 全部通过。

脚本先经 SSH 推送专用修复分支，再确认 main 仍为 aac8bc8 后快进发布 9f612ef；公开 raw 字节与本地 Git blob 摘要一致。KPanel 专用修复分支 3e96e96 固定 Dockerfile 与 node source.json，并说明失败和恢复语义；节点更新模板原样核对通过。首次来源文档漏同步被本地 L3 和候选 CI 拦截，a21d5bf 补齐 active source rows，保留历史证书验收来源。没有覆盖旧 tag 或伪装旧证据为新 SHA。成对回滚为 v1.9.0 与 aac8bc8。

## 多维质量与证据边界

| 维度 | 证据与边界 |
| --- | --- |
| 业务正确性 | 实际 Agent Unix HTTP、systemd PTY、完整原脚本和嵌套 Docker Engine；IT-Tools 为缓存的官方真实镜像，FRP/LibreSpeed 仅用明确命名的 workload 夹具验证管理及绑定规则 |
| 安全与供应链 | 固定 selector、容器身份及资源版本；401/409 和旧回环绑定拒绝；脚本公开字节校验、L3 和公开 OCI 核验 |
| 恢复与稳定 | 原生失败正确报告、marker-only 恢复、Agent 重启继续 PTY、取消保持应用、卸载清理；更新不承诺自动回滚 |
| 性能与资源 | 无新增业务轮询；隔离容器 1536MiB/1CPU/pids1024，宿主内存/磁盘与 420 秒业务 watchdog；不声称长期 soak 或全站 P95 已重测 |
| 用户体验 | 真实 AppsView/Modal/AppInteractiveTerminal/xterm 的 mock UI 浏览器验收、三视口主题、键盘/复制/菜单 hit-test；原生浏览器缩放、触屏、读屏器未实测 |
| 数据与迁移 | Docker 配置和脚本 marker/port 文件为真源，停止容器不丢绑定；生产停写备份、保护文件与 SQLite 核对另行记录 |

最终浏览器均绑定 a21d5bf，canonical preview manifest v1100-final-20260909080627，模式 mock、grade acceptance、profile visual-composition。Apps 作业 7004 在 08:06:28Z—08:06:45Z 通过，terminal 作业 12152 在 08:06:28Z—08:06:35Z 通过；终态 exit0。Apps 覆盖 1280 浅色、768/390 深色，FRP 管理、旧回环提示及禁用/保留动作、雷池图片、DOS 安装入口；终端覆盖 desktop/classic、浅深色、390/768/1280、CSS zoom 1/1.25/2、copy/all、Escape 和焦点。截图已人工查看。证据 C:/GitHub/_release-artifacts/v1100-apps-fixed-r2、v1100-terminal-fixed-r2，mock 数据不当作生产账户或网络业务实测。

隔离真机 r1 缺 Git 导致原脚本刷新目录失败，属于环境异常；r2 补实际 Git 和来自 /root/apps 的同源离线镜像，apps HEAD 2d8044adec98e3eb16f47cdbb297f6be9632a66f，使用标准 Git URL rewrite，并未伪造 Git 或 Docker 成功。r2 随后检出上述产品缺陷，失败证据保留。最终 r4 使用真实 systemd 与 nested dockerd/vfs，network none、无宿主挂载、非 privileged，所需 capabilities 显式列出；fixture image sha256:9b05d7b3648fc2e72f557b0c4fe798246903c9d45d90fd9882555647fcd07e29。完整脚本只应用标准安装器的许可/统计设置，除此不改测试副本；最终 Agent 来自本次 L3 的 dist 构建。具体终态在完成段登记。

## 自动门禁与预检

旧 d689df7 L3 r1 首轮成功，07:35:00Z—07:51:07Z；原始 11 项证据摘要已核对，仅保留历史，不替代最终 SHA。3e96e96 的 fixed-l3-r1 因 active source register 漏同步终止，候选 CI 34326942285 同根因失败；a21d5bf 修复后本地 managed-script-contract 与 node 模板检查通过。最终完整 L3 使用唯一 run-release-l3.mjs、run ID v1.10.0-fixed-l3-r2，固定 Runner sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3。计划摘要 bec876adef3d40693a7f039ded6aa6c5e796efcb884e0d9c1c7852b57e3ea299，remote script d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c，bundle 03f4d9b34bcd76a21b29ef623f9f633bba91d85935f223e58d10abf185d2ce58。

最终候选 CI [34327234050](https://github.com/kejilion/KPanel/actions/runs/34327234050) 成功并绑定 a21d5bf。最后一次仅来源文档变化未触发 GitHub freshness 路径过滤，因此在同一精确 Linux snapshot、同一不可变 Runner 中执行权威 report-dependency-freshness.mjs，未放宽完整性：08:09:02.487Z 报告 8/8 源成功、25 个直接/基座候选、127 个传递归属信号，退出 0，脚本与 main 一致；证据 v1.10.0-freshness-final。EOL 最近复核 2026-07-28、下次 2026-10-28，无维护例外需要行动；采用继续依现行分类和期限，不额外升级依赖。

生产预检由 run-production-evidence.mjs 于 07:33:57Z—07:33:58Z 完成，旧 1.9.0 健康。backup/postdeploy 的 prepare-only 与真实执行目录严格分开；假的 digest 仅用于 argv-only，不能进入执行。最终公开核验脚本本地/远端 SHA256 67682fcdc7b7a4fa378636c6e9dbb7aa5ef909c2ca59154576d78e93a8a570e4 一致，bash -n 与真实 UTF-8 JSON 解析命令都通过；此前静态审查发现的嵌套引号已修复。闭环 L0 的 argv/VERIFY_BASE_REF、秒/毫秒时间与非法精度拒绝在 a21d5bf 于 08:08:55Z 重新通过。验收记录不借用旧候选 SHA。

正式生产写操作、公开资产、回滚包、最终 L3/实机终态与摘要以下均从本轮完成证据填写。

## 完成证据

最终 L3 于 2026-09-09T08:06:17Z—2026-09-09T08:20:14Z 完成，exit0；Go 全量、149 文件/1292 前端测试、核心 race（panel 169.472s、auth 3.942s、dockerx 88.696s）、typecheck/build、双架构二进制、源码和镜像扫描、rootfs/app-conf 生命周期全部通过。govulncheck 可达漏洞 0、未调用模块通告 3；npm audit 0、Trivy 本次扫描无发现。L3 原始目录 11/11 摘要核对通过，失败 fixed-r1 的 11/11 摘要也保留核对。

实机 r3 已通过失败状态修复，但测试漏回答恢复安装的端口提示而超时；r4 根据实际提示输入 18064，完整 9 组检查通过，恢复后的端口文件和 HTTP 服务均正确，卸载清除真实容器与 marker/port 文件。Agent 输入 SHA256 50ef7ad174b9b994c40e712e5a67c5de99ea548dccc9a5a4e0d247a9a54c4a8d；真实 IT-Tools archive 45fad9ddfe1fa9d7a45722c158b25a70758b19ac9dc6bf074fb03a5c3932beac。容器 RestartCount=0/OOMKilled=false，清理断言通过。r2/r3 失败证据归档和 r4 完整证据保留在 C:/GitHub/_release-artifacts/v1100-preflight 与 v1100-real-r4；没有把 FRP/LibreSpeed workload 夹具升级为全产品实测声明。

公开 Agent 附件 SHA256 为 2b523e3e4ef128ab2c8b7159010673733a694b7cd34d344081b64298b61083d9，与 L3 二进制不同；go version -m 已核对两者均为 Go1.26.7、同一依赖与 a21d5bf revision，区别包括 tag 构建的 v1.10.0+dirty 与预 tag 快照的模块/VCS 元数据。Release 在编译前生成 release/DRAFT_NOTES.md；本轮不声称跨构建环境逐字节可复现。隔离完整应用旅程使用 L3 二进制，公开镜像 E2E、正式安装的 Agent 启动版本及健康证据另行验证。

主线 CI [34328871252](https://github.com/kejilion/KPanel/actions/runs/34328871252) 与 freshness [34328871286](https://github.com/kejilion/KPanel/actions/runs/34328871286) 均成功，绑定产品 SHA。Release run [34329657557](https://github.com/kejilion/KPanel/actions/runs/34329657557) 成功；[v1.10.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.10.0) 于 2026-09-09T08:41:38Z 公开。8 个公开附件的 SHA256SUMS、API digest/size 全部核对，LICENSE/THIRD_PARTY_NOTICES 与候选一致。版本和 latest OCI 摘要相等，公开 immutable image 已实际 pull、revision/version/脚本/User 标签核对和 image-e2e 通过，完成 2026-09-09T08:43:25Z；证据 C:/GitHub/_release-artifacts/v1.10.0-public-fixed-r1。

annotated tag object 5039a56042f7b72abd66026ed547da19832b22b0 指向 a21d5bf；标签 freshness [34329657561](https://github.com/kejilion/KPanel/actions/runs/34329657561) 成功。当天定时 security-advisories [34324434578](https://github.com/kejilion/KPanel/actions/runs/34324434578) 成功，该项为主线当日安全信号，不替代最终候选扫描。

Docker Hub：[kjlion/kejilion-panel](https://hub.docker.com/r/kjlion/kejilion-panel)。1.10.0/latest：`sha256:eaecfa6a156c35b106e74820619d35734c6cbdc2fd577465f08ce95bb46e964e`。支持 linux/amd64 和 linux/arm64，完整 index 与 attestations 如下：

```text
Name:      docker.io/kjlion/kejilion-panel:1.10.0
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:eaecfa6a156c35b106e74820619d35734c6cbdc2fd577465f08ce95bb46e964e

Manifests:
  Name:        docker.io/kjlion/kejilion-panel:1.10.0@sha256:54917fd565b41d7f5716d17e2e1b6af873736141314e22bdce20621555457d17
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    linux/amd64

  Name:        docker.io/kjlion/kejilion-panel:1.10.0@sha256:c3f0706d5d12a9ec749c4ff4043c9fdab77fee3a5c24c80272cfc2dcd864601b
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    linux/arm64

  Name:        docker.io/kjlion/kejilion-panel:1.10.0@sha256:38b1e9cbb23a0b60e6ba1598e624bf32edc99348d349749f3ce1aa8ee79da70e
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    unknown/unknown
  Annotations:
    vnd.docker.reference.digest: sha256:54917fd565b41d7f5716d17e2e1b6af873736141314e22bdce20621555457d17
    vnd.docker.reference.type:   attestation-manifest

  Name:        docker.io/kjlion/kejilion-panel:1.10.0@sha256:3095f699ff31d2673bf7995475cf123eb0dbbfab94ac6f2518f24b8aa018ef3c
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    unknown/unknown
  Annotations:
    vnd.docker.reference.digest: sha256:c3f0706d5d12a9ec749c4ff4043c9fdab77fee3a5c24c80272cfc2dcd864601b
    vnd.docker.reference.type:   attestation-manifest
```

生产 backup 于 2026-09-09T08:44:22Z—2026-09-09T08:44:31Z 通过；恢复包 `/root/kpanel-backups/pre-v1.10.0-20260909T084422Z`，备份摘要核对、旧镜像 load 与旧服务恢复均成功。随后只调用标准入口 `env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，退出 0。postdeploy 于 2026-09-09T08:46:00Z—2026-09-09T08:46:02Z 通过，Panel/Agent 1.10.0，精确 revision/OCI 匹配；容器 healthy、RestartCount=0、OOMKilled=false，Agent 服务 active/running，保护文件 diff 为空，SQLite quick_check 通过，日志无 fatal/panic/OOM。预检、backup、postdeploy 顶层摘要分别 5/5、8/8、8/8 已核对。

[生产公网 HTTPS 健康](https://kpanel.154.36.153.9.sslip.io/api/v1/health) 于 2026-09-09T08:46:26Z 返回 HTTP 200、version 1.10.0、status ok。公开 apps kpanel.conf 与候选归一全文一致，无 apps 提交；源脚本 main raw 字节再次确认等于配对摘要。仅隔离环境做应用安装/更新故障/恢复/卸载，不在生产应用数据上重复破坏性验收。没有执行回滚或重复正式发布。

## 发布节奏与首轮情况

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-09T13:36:09+08:00
- 候选冻结时间：2026-09-09T08:05:59.681Z
- 生产完成时间：2026-09-09T08:46:02Z
- 提交到生产用时：3.164722 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

这里的“否/不适用”只表示本轮正式生产变更未失败；发布前产品缺陷、无效证据与工具问题已在前文和以下明细记录，本轮全流程不是首轮成功。最终产品 SHA 的完整 L3 首轮通过；不得将旧候选首轮通过归给最终候选。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：16
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "testing/apps-browser/fixture-contract",
    "position": "before-production-write",
    "count": 6,
    "impact": "原 d689 候选浏览器 r1-r5 分别缺回环 warning、残留 mock 活动任务、默认已安装过滤、选择背景按钮、错误预期按钮文案；r6 虽通过但端口数据与回环模式矛盾，不能作为最终证据。",
    "recoveryEvidence": "修正真实页面所需 mock 字段及作用域选择器、只清测试 context 的活动任务；r7 通过，最终 a21d5bf Apps/terminal 两作业重新通过。",
    "permanentAction": "本轮唯一 apps-browser.cjs 保留修正夹具与真实按钮断言，背景作业固定最终 SHA；不降低产品断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/git-ssh/identity-selection",
    "position": "before-production-write",
    "count": 1,
    "impact": "脚本仓库只读 ls-remote 未传仓库专用 SSH identity，被 publickey 拒绝，没有远端写入。",
    "recoveryEvidence": "复用 KPanel core.sshCommand 后精确查询、推送修复分支和脚本主线成功；专用适配器 inspect 复验通过。",
    "permanentAction": "本轮 script-git.ps1 固定从权威仓库加载身份、检查 exit code 与候选/main SHA，后续调用禁止裸脚本 SSH。",
    "historicalReleases": [
      "v1.9.0"
    ]
  },
  {
    "fingerprint": "preflight/public-config/node-fetch-reset",
    "position": "before-production-write",
    "count": 1,
    "impact": "Node fetch 公共 apps 配置发生 ECONNRESET，未获得有效配置证据。",
    "recoveryEvidence": "有界 Invoke-WebRequest 读取同一公开 URL 成功并与候选归一全文一致。",
    "permanentAction": "本轮外部下载使用明确 URL、有界 Invoke-WebRequest 或 curl；负责人为发布任务，2026-09-16 复核网络，退出条件为同来源完整读取与摘要核对成功，不将瞬时网络恢复宣称永久网络修复。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/tool-approval/compound-command",
    "position": "before-production-write",
    "count": 1,
    "impact": "自动审批拒绝复合 PowerShell 的夹具编辑、浏览器启动及下载比较命令，提示 blocked by policy，未给具体原因且命令未执行。",
    "recoveryEvidence": "改为 apply_patch 和单一用途的背景测试/只读下载调用后完成，不绕过审批限制。",
    "permanentAction": "本轮编辑、执行和读取使用可独立审查的结构化步骤。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/real-app-fixture/catalog-transport",
    "position": "before-production-write",
    "count": 1,
    "impact": "隔离真机 r1 缺 Git 且无外网，完整原脚本必须刷新 apps，验证未进入目标业务。",
    "recoveryEvidence": "r2/r3 提供实际 Git 和来自真实 apps checkout 的同源离线镜像，经标准 Git URL rewrite 执行真实 clone/pull；无 fake Git 成功。",
    "permanentAction": "prepare-real-r3.py 显式检查 Git/Docker 并提供实际目录传输；网络隔离仍保留，用于真实 registry 失败注入。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/managed-script/source-register-sync",
    "position": "before-production-write",
    "count": 1,
    "impact": "3e96e96 固定脚本后漏同步 active 来源文档，同一批次 L3 fixed-r1 与候选 CI 都在 managed-script-contract 失败；旧通过证据不能复用。",
    "recoveryEvidence": "a21d5bf 补齐来源登记，权威契约入口和 node 模板匹配通过，最终候选 CI 成功，并重新执行完整 L3。",
    "permanentAction": "保留既有 check-managed-script-contract 门禁；本轮同步主动行与 pin 后先执行该检查，未放宽 grep/hash 要求。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-l3/candidate-argv",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次将 HEAD 字面量传给只接受完整对象 ID 的 L3 入口，被参数校验拒绝，未执行门禁。",
    "recoveryEvidence": "使用完整 40 位候选 SHA 启动独立 run，manifest 记录精确身份。",
    "permanentAction": "本轮后续 argv 固定最终 SHA，禁止符号 ref 代替候选对象 ID。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/tool-edit/patch-protocol",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次 apply_patch 输入漏 Begin Patch，被工具拒绝，没有文件修改。",
    "recoveryEvidence": "补齐正式 patch 格式后执行，Git diff 和相关入口验证通过。",
    "permanentAction": "后续编辑使用完整结构化 patch；不把被拒绝工具调用算作验证执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/public-artifacts/nested-python-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "静态复核发现公开核验脚本 python -c 的 UTF-8 引号嵌套不正确；此前 bash -n 不足以证明实际 Python 参数有效。",
    "recoveryEvidence": "使用双引号编码参数，并真实执行同一解析命令读取中文 JSON fixture；本地/远端脚本 SHA256 一致。",
    "permanentAction": "check-final-preflight.py 保留实际解释器命令验收，不能仅以 shell 语法检查作为产物解析证据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-diagnostics/unverified-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "只读路径探索中多次猜测文件名或使用未被 PowerShell 展开的路径 glob 被拒绝；按同根因诊断批次计一项，均未执行验证或业务写入。",
    "recoveryEvidence": "使用明确已有文件与 rg 的目录加 -g 过滤，读取真实权威入口；有效验证结果单独登记。",
    "permanentAction": "后续仅使用已列出的路径和工具 schema；诊断失败不得计为产品测试失败或成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/real-app-fixture/native-recovery-input",
    "position": "before-production-write",
    "count": 1,
    "impact": "最终候选 r3 已验证更新失败正确报告；marker-only 恢复时测试只输入安装菜单编号，漏回答真实端口提示，等待 70 秒超时。",
    "recoveryEvidence": "r4 等待实际端口提示后输入 18064，完整重跑相同 SHA 并增加恢复后端口文件及真实 HTTP 校验。",
    "permanentAction": "唯一 guest-apps-r4.py 按原生提示完成输入，不以自动完成假设代替交互协议，不改变产品或放宽等待断言。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 清理与保留

本轮全部 canonical 预览已停止，r1-r4 隔离容器已清理。已确认两份修复工作树干净、无忽略原件，且提交可由 v1.10.0 或已验证脚本 main 恢复后，移除这两份 worktree，文件总计 23167720 字节。保留当前发布工作树、源码分支与 tag、旧 OCI、生产恢复包和全部唯一证据；未做全局 prune，也未清理他人或旧版本工作树。

最终验收记录形成独立文档提交；产品 tag 保持固定 a21d5bf，不把后续文档 SHA 当产品 revision。未改他人工作树或未提交内容，没有新增永久规范、工作流或记忆；本轮临时验证工具及唯一证据保留。
