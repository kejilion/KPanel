# KPanel v1.12.0 发布验收

日期：2026-09-10。发布级别 L3。最终产品提交 `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`，annotated tag `v1.12.0`，tag object `1542f940d7ccf9d6bff6a718c4e15334e92ee16b`。候选基线 main 为 `4f4e6ab7d049e66c71b685e87b0133a8cd9d685a`；上一稳定版本为 v1.10.0，产品提交 `a21d5bfc744b2a9316578dfca83426c001e2ee6d`，旧 OCI 为 `sha256:eaecfa6a156c35b106e74820619d35734c6cbdc2fd577465f08ce95bb46e964e`。v1.11.0 的历史内容和记录保留，其主线、默认更新通道和生产发布已撤回，本轮用新候选重新发布为 v1.12.0。

## 范围与业务真源

本次候选无冲突纳入以下七组已完成变更，并由 `0ff1e32` 统一版本元数据：

- `b1b3478`、`1413f9e`：文件管理器的 ZIP/TAR/TAR.GZ 创建、压缩包浏览、选中解压、后台任务、历史/取消/重试/清理，以及路径穿越、符号链接、条目数、展开大小和压缩比限制；不覆盖已有文件，并沿用 Panel、Agent、light node 的现有能力边界。
- `8d4b3d6`：light node 文件读取响应丢失后的有界恢复；仅重放 GET/HEAD，不重放写操作。已知在线节点的瞬时 404/405/426 使用 5 秒恢复窗口，未知旧状态仍为 5 分钟；目录读取保留最后一次成功结果，按原路径和 offset 重试，并在主机或窗口切换时取消。
- `908a0c7`、`dda2458`、`210d6eb`：应用脚本兼容容器版本标记、操作按钮右置，以及关闭运行中脚本时先取消并等待；停止失败时保留窗口和错误，systemd control group 使用有界等待。
- `551afeb`：终端选区对比度修复。

候选 diff 共涉及 54 个功能或测试文件，约 3907 行新增、141 行删除。已逐项检查并排除系统中心页面、路由、API、权限和业务文档；既有系统中心基线未被本轮改动。数据库 schema、配对格式和生产业务数据没有迁移。

`scriptLinkageState=not-required`。本轮没有修改 `kejilion.sh`；权威脚本仍为提交 `9f612efc4f861459c0a525491c7cdf5eda756cf7`，公开字节 SHA256 为 `9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663`。`packaging/kejilion-app/kpanel.conf` 未改，公开 apps 仓库的对应配置与候选归一全文一致，因此无需脚本或 apps 配对发布。

所有实机与生产操作只连接 `arena-154` / `154.36.153.9`。没有连接 `108` 或 `prod-108`。

## 质量、测试与证据边界

最终候选 L3 run `v1.12.0-0ff1e32-l3-r1` 于 2026-09-09T15:33:44Z 至 15:47:43Z 首轮完成，exit 0。固定 Runner 摘要为 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，计划摘要 `0a31835c9c79bb6dc6a8ad7aba421122e8aea52e77a7073ed60bf9b7681dd6f9`，远端脚本摘要 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，输入 bundle 摘要 `b0e31e56d3f5316a142f1b2e06451f489032ecd331b13b7aaa2d115913c4b4d8`。原始证据保存在 `C:/GitHub/_release-artifacts/v1120-0ff1e32-l3-r1`。

L3 覆盖 Go 全量测试、核心 race、typecheck/build、双架构构建、版本一致性、安装安全与 app-conf 生命周期。前端 151 个文件、1332 个测试全部通过。`govulncheck` 可达漏洞为 0，`npm audit` 为 0，Trivy 源码、配置和最终镜像扫描无发现。候选、main 和 tag 的 dependency freshness 均成功；本轮没有依赖升级，沿用 Go 1.26.7、Node 24.20.0 和固定 Trivy 0.72 环境。

| 维度 | 状态、证据与边界 |
| --- | --- |
| 业务正确性 | 最终候选组件测试覆盖压缩任务、能力协商、目录恢复、应用脚本关闭和终端选区。公开镜像在隔离临时容器中完成启动、健康和静态资源 E2E。 |
| 安全与供应链 | 压缩包路径、链接、条目数、展开大小和压缩比限制已进入门禁；GET/HEAD 恢复与写操作隔离；Release 附件、OCI revision/version、平台和摘要均核对。 |
| 恢复与稳定 | light node 重试有时间、次数和生命周期边界；应用脚本取消/停止失败保留真实错误；生产更新前完成可恢复备份并验证旧服务可恢复。 |
| 性能与资源 | L3 与生产资源快照通过；未执行长期 soak、全站 P95 或大规模并发压缩基准，不作相关性能承诺。 |
| 用户体验 | 最终候选前端测试覆盖新增交互。开发分支曾执行浏览器验收，但发生在最终候选形成前，因此只作为开发过程证据，不作为 `0ff1e32` 的发布门禁。 |
| 数据与迁移 | 无 schema 迁移；生产 SQLite quick check 通过，保护文件 diff 为空。生产未在用户文件上执行压缩或解压写操作。 |

精确候选没有执行原生生产浏览器中的压缩创建、选中解压、应用脚本终止或触屏/读屏器实测。压缩功能依靠最终候选自动测试、隔离构建和安全边界验证；生产只验证公开构建、服务、版本、数据完整性和更新链路。此前 archive 和 light-file 开发任务的浏览器或实机结果对应中间提交，不升级为最终 SHA 证据。

## 候选、主线与公开发布

候选分支 `release/v1.12.0-candidate` 推送后，dependency freshness run [34372638294](https://github.com/kejilion/KPanel/actions/runs/34372638294) 和 CI run [34372638328](https://github.com/kejilion/KPanel/actions/runs/34372638328) 均成功并绑定 `0ff1e32`。main 随后从 `4f4e6ab` 纯快进到同一 SHA，无冲突；main dependency freshness [34373378056](https://github.com/kejilion/KPanel/actions/runs/34373378056) 和 CI [34373378157](https://github.com/kejilion/KPanel/actions/runs/34373378157) 均成功。

Release run [34374207367](https://github.com/kejilion/KPanel/actions/runs/34374207367) 首轮成功，tag dependency freshness [34374207322](https://github.com/kejilion/KPanel/actions/runs/34374207322) 成功。[v1.12.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.12.0) 于 2026-09-09T16:11:52Z 公开，状态为非 draft、非 prerelease。附件完整：两种架构的 Agent、两种架构的 Node、`kejilion-panel-deploy-1.12.0.tar.gz`、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。

Docker Hub 的 `1.12.0` 和 `latest` 均指向 OCI index `sha256:229b81f9d31741f0bb75a1eea470566116951cec791eb70ea7bf2b046a23c34a`。linux/amd64 manifest 为 `sha256:0332501faea2ada15f5cdc13dfaf486b04348345a3c1bf8eb30fde822cae9074`，linux/arm64 manifest 为 `sha256:904ef96ae819c6772dc832478723600804c861e35cd581e9dd42f35dcdcee261`；另外两个 unknown/unknown manifest 是对应平台的 attestation。公开镜像已在 arena-154 实际 pull，并在隔离临时容器的 18120 端口完成 `image_e2e=pass`；临时容器和目录已删除，证据保存在 `C:/GitHub/_release-artifacts/v1120-public` 与 `C:/GitHub/_release-artifacts/v1120-github`。

## 备份、生产更新与回滚点

生产预检 run `v1.12.0-production` 于 2026-09-09T15:32:21Z 至 15:32:22Z 完成，确认原生产 v1.10.0 健康。正式写入前于 16:16:47Z 至 16:16:57Z 创建并验证恢复包 `/root/kpanel-backups/pre-v1.12.0-20260909T161647Z`，包含 Panel inspect、`kpanel` 数据 tar.zst、旧镜像 tar.zst、service、config 和 SHA256SUMS。备份过程停止后恢复原生产，健康门禁通过。证据保存在 `C:/GitHub/_release-artifacts/v1120-production-preflight-r1` 与 `C:/GitHub/_release-artifacts/v1120-production-backup-r1`。

生产只执行标准业务真源入口：

```text
env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel
```

命令退出 0，拉取 digest `sha256:229b81f9d31741f0bb75a1eea470566116951cec791eb70ea7bf2b046a23c34a` 并重建 Panel。postdeploy 于 2026-09-09T16:17:50Z 至 16:17:52Z 完成，精确核对 version `1.12.0`、revision `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63` 和 OCI digest。公网健康返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.12.0`。Panel 容器 healthy，Agent active/running/enabled，无 daemon reload；SQLite `panel/ai.db` 为 ok，保护文件 diff 为空，日志无 fatal/panic/OOM。生产样本约 72.38 MiB / 256 MiB、CPU 0.03%、PIDs 8。light node 状态 schema 为 1，登记主机数为 0，因此没有需要等待升级的已配对 light node。证据保存在 `C:/GitHub/_release-artifacts/v1120-production-postdeploy-r1`。

产品回滚点为 v1.10.0 / `a21d5bfc744b2a9316578dfca83426c001e2ee6d` / OCI `sha256:eaecfa6a156c35b106e74820619d35734c6cbdc2fd577465f08ce95bb46e964e`，生产数据与旧镜像恢复点为 `/root/kpanel-backups/pre-v1.12.0-20260909T161647Z`。本轮没有触发回滚、紧急热修复或重复正式发布。

## 发布节奏与流程异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-09T18:12:11+08:00
- 候选冻结时间：2026-09-09T23:29:58+08:00
- 生产完成时间：2026-09-10T00:17:52+08:00
- 提交到生产用时：6.09 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终候选 L3、候选 CI、候选 freshness、main CI、main freshness 和 Release 均为各自首轮成功。以下次数只统计发布编排、诊断或证据读取异常，不等同于产品测试失败；其中一项发生在生产写入完成后的只读复核阶段，生产始终健康。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：8
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/git-fetch/local-tag-collision",
    "position": "before-production-write",
    "count": 1,
    "impact": "git fetch --tags 因本地与远端 v0.86.2 标签冲突被拒绝；PowerShell 原生命令失败未立即停止，随后候选路径已存在检查触发。没有远端写入。",
    "recoveryEvidence": "改用仅抓取 heads 的命令，核对既有候选 worktree 的精确基线与状态后继续。",
    "permanentAction": "后续发布预检沿用工作流的 --no-tags 抓取方式，并显式检查每个原生命令退出码。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-metadata/incomplete-version-sync",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次准备只更新 VERSION 和 CHANGELOG，遗漏 internal/version、package.json 与 lockfile；候选尚未冻结。",
    "recoveryEvidence": "按发布工作流补齐全部版本真源，amend 后 check-version-consistency 和最终 L3 均通过。",
    "permanentAction": "候选冻结前固定执行版本一致性检查，并以其通过结果作为后续 L3 输入条件。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/orchestrator-cli/unsupported-help",
    "position": "before-production-write",
    "count": 1,
    "impact": "对两个发布编排入口传入不支持的 --help，入口打印 usage 后退出，未生成或执行发布证据。",
    "recoveryEvidence": "读取仓库工作流中的权威调用方式，以受支持参数完成 L3 和生产证据流程。",
    "permanentAction": "直接读取工作流和入口参数定义，不用未声明的探测参数判断编排器能力。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/repository-search/windows-path-glob",
    "position": "before-production-write",
    "count": 1,
    "impact": "同一只读诊断批次中的两个 rg 调用把 Windows 路径通配符当作实际路径，系统拒绝该路径语法；没有形成验证结论。",
    "recoveryEvidence": "改用明确文件或目录加 -g 过滤，读取到真实 Dockerfile 和生产证据入口。",
    "permanentAction": "Windows 下路径参数只传已存在目录，文件模式统一使用 rg -g。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/dockerhub-api/connection-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "Docker Hub Web API 请求连接超时，未取得可用的 tag 证据。",
    "recoveryEvidence": "改用 registry-1 API；发布前得到预期 404，发布后取得真实 OCI index、平台和摘要，并完成公开镜像 pull。",
    "permanentAction": "公开 OCI 核验以 registry API 为主；发布任务于 2026-09-10 复核，取得一次成功的完整 manifest 响应后退出重试。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/local-runtime/docker-cli-unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows 发布环境没有可用 Docker CLI，无法在本地执行公开镜像 E2E。",
    "recoveryEvidence": "在唯一允许的 arena-154 上 pull 同一公开不可变镜像，并以隔离临时容器完成 image_e2e=pass 和清理。",
    "permanentAction": "公开镜像门禁先检查本地 CLI；不可用时按工作流直接路由到批准的固定实机目标。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/oci-parser/byte-array-content",
    "position": "before-production-write",
    "count": 1,
    "impact": "Invoke-WebRequest 返回 byte array，直接写出后形成十进制行，首次平台解析失败；无效输出未计为 OCI 证据。",
    "recoveryEvidence": "按响应类型进行 UTF-8 解码，真实解析 index 后确认 amd64、arm64 和 attestations，1.12.0 与 latest 摘要相同。",
    "permanentAction": "OCI 取证脚本对 byte array 与字符串分支显式解码，再进行 JSON schema 和平台断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postdeploy/evidence-read/ad-hoc-path-and-quoting",
    "position": "after-production-write",
    "count": 1,
    "impact": "只读复核先猜测不存在的 container.txt 和 agent.txt，随后 SSH 内联 Python 引号被 shell 改写；两次均未改变生产，按同一诊断批次计一项。",
    "recoveryEvidence": "枚举实际证据文件，并用 PowerShell here-string 经 stdin 传给 python3；确认 postdeploy gate、资源、日志和 light_hosts=0，生产持续健康。",
    "permanentAction": "读取证据前先枚举 artifact schema；复杂远端解析统一通过 stdin 脚本传输，不嵌套 shell 引号。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 清理与保留

候选远端分支已由发布流程删除。公开镜像 E2E 的临时容器和目录已清理。保留产品 tag、GitHub Release、OCI、生产恢复包和本轮唯一证据；不删除 v1.11.0 的历史内容或历史发布记录，也不清理其他任务拥有的功能工作树。

本文件作为产品发布后的独立验收记录提交；`v1.12.0` 始终固定指向 `0ff1e32ec8a84099659e5e0bebcea07b4b7aad63`，后续文档提交不是产品 revision。
