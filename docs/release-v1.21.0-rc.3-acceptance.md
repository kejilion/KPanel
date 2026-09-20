# KPanel v1.21.0-rc.3 发布验收记录

日期：2026-09-20

发布级别：L3

候选提交 / 标签：`f6f969b65e385865269f8bf089092a50e0bd81fe` / `v1.21.0-rc.3`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `f6f969b65e385865269f8bf089092a50e0bd81fe`

- 原分支 / 精确 tip / 候选映射：`fix/mcp-access-alignment-20260920` / `82f950f4e1710a4e08cb56f49912c30daf0f5310` / `aab67875c9de453afcd4e2165921fbaa8a74d374`；`fix/light-node-tls-fallback` / `0fa5ad44e4efcf069a80ed9e4f926da6d6cb2d3f`（含父提交 `8915a0b7a7a5e07d3fb649b6b66a7f119051a8dc`）/ `a9dc31951c1d879cd0684f97eacfcac784648b1e`、`57a2b8f654b458b38609239906f2c8f0cf17bd1d`。
- 归档 ref 与 SHA：`archive/fix/mcp-access-alignment-20260920` = `82f950f4e1710a4e08cb56f49912c30daf0f5310`；`archive/fix/light-node-tls-fallback` = `0fa5ad44e4efcf069a80ed9e4f926da6d6cb2d3f`；`archive/assemble/v1.21.0-rc.3` = `f6f969b65e385865269f8bf089092a50e0bd81fe`；远端逐 ref 复核一致，两个 clean 来源工作树已释放。
- 旧分支 `fix/file-host-switch-context-20260913` 与 `feature/visual-refinement-pass` 的已提交 tip 分别保存为 `archive/fix/file-host-switch-context-20260913@1cc5a842` 和 `archive/feature/visual-refinement-pass@c2106884`；其中仍有未提交或未跟踪内容的本地工作树按规范原样保留并列为“本地待处置”，没有纳入或宣称发布。
- `docs/security-audit-run2-local-20260919` 含受限本地材料，保持未发布、未改动。`release/v1.21.0-candidate` 继续承载后续 RC，不在本次预览发布后归档。

归档不代表生产上线；本版只发布预览产物，禁止生产部署。

## 发布画像

- 业务域：MCP 接入界面一致性；轻量节点与集群客户端 TLS 握手兼容。
- 变更面：MCP 接入卡片统一内容内边距、状态反馈和刷新入口；HTTP 客户端保留 Go 默认曲线优先级，仅在明确 `TLS handshake timeout` 且请求可重放时使用经典曲线重试，并按主机共享成功缓存。
- 受影响用户旅程：管理员查看和操作 MCP 接入状态；轻量节点、集群普通请求、流请求和历史请求连接不兼容默认曲线的旧 TLS 端点。
- 未变化契约：证书校验、TLS 1.2 最低版本、请求超时、禁止重定向、稳定 GitHub Latest、Docker `latest`、应用市场稳定入口、`kejilion.sh` 和生产数据均未改变。
- 风险等级及理由：L3；TLS transport 属网络基础路径，但回退触发条件和请求重放均受限，新增独立包及客户端接线测试，并通过精确候选 L3、双层 CI、Release 安全链和公开 OCI 验收。

## 发布范围与外部审计

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.3]`；默认 TLS 路径仍使用 Go 当前安全曲线选择，不主动降级。
- 主要提交：`aab67875` MCP 内容与状态对齐；`a9dc3195` 轻量节点 TLS 回退；`57a2b8f6` 集群共享回退缓存；`f6f969b6` 版本冻结。
- 两条代码来源均带 open-code-review 1.12.6 trailer，适用文件覆盖为 1/1、3/3 和 7/7，均为 `H0/M0/L0`；该结果是辅助证据，不替代项目门禁。
- Cloudflare `security-audit-skill` run-3 仍没有最终结构化结论；本次没有把中止的专项审计改写为通过，也没有把 TLS 兼容修复扩张成完整安全审计声明。
- 稳定版状态：尚未交付；生产部署：未执行。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与互通 | 已验证 | `internal/tlsfallback`、node、cluster 定向及全量 Go 测试通过；MCP 接入 4 项旅程与全量 Web 测试通过。 | 未覆盖所有第三方 TLS 设备型号。 |
| 网络与供应链安全 | 已验证 | 回退只接受明确握手超时并保留 TLS 配置；govulncheck 可达漏洞 0、npm audit 0、Trivy 源码/配置/最终镜像 0 阻断项。 | 3 个未调用模块通告继续跟踪。 |
| 稳定性与失败恢复 | 已验证 | 可重放/不可重放、非握手超时、共享缓存、关闭空闲连接、race 与应用安装/更新/回滚/卸载生命周期通过。 | 未执行长期弱网 soak。 |
| 性能与资源预算 | 已验证 | 成功回退按目标 host 缓存，避免每次等待握手超时；没有新增后台轮询。 | 未在大规模动态 host 集合压测缓存上限。 |
| 用户体验与可访问性 | 已验证 | Web typecheck、169 个文件 / 1456 项测试、3176 个短语 / 21 个目录及生产构建通过。 | 未另做真机视觉走查。 |
| 数据、配置与迁移 | 已验证 | 无 schema、持久数据或受管脚本变化；回退缓存仅进程内有效。 | 重启后会重新探测一次不兼容端点。 |

## 自动门禁

- 本机版本元数据与治理一致性检查通过；新组装工作树没有 `web/node_modules`，本地 exact-worktree 的测试/typecheck 入口未启动，未记为通过。MCP 来源工作树 4 项定向测试通过，最终以冻结 SHA 的固定远程 L3 为准。
- L3：`kpanel-release-gate:go1.26.7-node24`，Runner digest `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；197/197 治理测试、Go 全量、核心 race、Web 169/1456、typecheck、3176 短语 / 21 目录、构建、govulncheck、npm audit、Trivy、双架构二进制、镜像和 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：run ID `v1.21.0-rc.3-f6f969b-l3-r1`，2026-09-20 11:33:50 至 11:49:51 +08:00，exit 0；bundle `d9b2b5b93c05c6deef183c116d5f3ce1f2f4697e6bc1f794a007223e1665784e`、plan `b509b05ca67dd3639ed75cf38c5c4e64566b65dfb26d850113c45fcc272ad613`、remote entry `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`、远端日志 `233db75eef36ad411d069bf9c6d29d0a27e02f7b26cae973a66240f93d90695f`；证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.3-f6f969b-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.3-f6f969b-l3-r1`。
- 候选 CI：[CI 35487597674](https://github.com/kejilion/KPanel/actions/runs/35487597674) 与 [freshness 35487597684](https://github.com/kejilion/KPanel/actions/runs/35487597684) 成功，绑定精确 SHA。
- 主线 CI：[CI 35487837397](https://github.com/kejilion/KPanel/actions/runs/35487837397) 与 [freshness 35487837390](https://github.com/kejilion/KPanel/actions/runs/35487837390) 成功，绑定精确 SHA。
- annotated tag object `1d666a6b863374995b29fe8a5dae34f240ccf21b` 指向产品提交；[Release 35488150206](https://github.com/kejilion/KPanel/actions/runs/35488150206) 与 [tag freshness 35488150139](https://github.com/kejilion/KPanel/actions/runs/35488150139) 成功，2026-09-20 12:13:55 +08:00 完成。

## 发布产物与隔离验收

- [KPanel v1.21.0-rc.3](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.3) 为非 draft、prerelease、非 Latest；GitHub Latest 仍为 `v1.20.0`。
- 14 个非空附件完整：MCP 六个平台二进制、Agent/Node 双架构、部署包、LICENSE、`SHA256SUMS` 和 THIRD_PARTY_NOTICES。
- Docker `1.21.0-rc.3` 与 `preview` OCI index 均为 `sha256:1f6fa3f208a7742e11a078f82c5f6060a63df21f872d05d9ed93e3827c13c6cf`；`linux/amd64` / `linux/arm64` 分别为 `sha256:13a56148416914439b3a7769d572f98113a07018677ffcd801fd25bc5b5fd0a7` / `sha256:17beb67965efd8e569a200a90a6f0d00923085a9b88e25d3f9b128ee84669bb7`；另两项 `unknown/unknown` 为 attestation。
- 稳定 `latest` 与 `1.20.0` 均保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- `arena-154` 公开镜像 E2E 使用精确源码 archive SHA-256 `ec62c80e0c80d01c4eb8dd58b460844765d45c5fab0b6e136fecf29682b2cb67`、执行脚本 SHA-256 `9523050a4e0c5f76559f2b1d9df269a67049a1db9ccb414bf7b65b33be298502`、日志 SHA-256 `eb5c3824cc5c3e0ec0910148cf3d9802e0fb8bdab1b50f021c6f50abda945175`；镜像标签 revision=`f6f969b65e385865269f8bf089092a50e0bd81fe`、version=`1.21.0-rc.3`，输出 `image_e2e=pass` 与 `public_oci_e2e=pass`，本次容器、网络和远端临时目录已清理。

## 自更新、生产与回滚

- 稳定通道继续只接受正式 Latest；预览通道可接收规范 RC，并在没有新 RC 时接收更新的稳定版。本次只提升 `preview`，没有改变通道选择协议。
- 生产目标和授权：不适用；预览版禁止生产部署。`prod-108` 未连接、未备份、未部署、未升级、未核对。
- 生产写操作：0。`arena-154` 仅用于候选 L3 与公开 OCI 隔离验证。
- 稳定回滚点：`v1.20.0` / `c98727c...` / `sha256:a991b5d2...`；上一预览为 `v1.21.0-rc.2` / `sha256:1836a60f...`。需要回退预览通道时可重新固定上一 RC 摘要，稳定入口不受影响。
- `scriptLinkageState=not-required`：内置脚本仍为 `6ebb945f6d5cb69fdb41e3761de23566acbaf762`，SHA-256 `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`；应用市场默认 `latest` 不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-20T09:55:00+08:00
- 候选冻结时间：2026-09-20T11:32:45+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成生产回滚、紧急热修复或重复发布；本次预览版没有生产写操作。下列流程异常均在远端发布前或只读验收阶段被发现，没有逃逸发布门禁。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：6
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "release-assembly/worktree/incorrect-command-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "创建 rc.3 worktree 后的 cherry-pick 命令仍在主工作树执行，三个本地提交临时落到本地 main；远端和未提交内容均未变化。",
    "recoveryEvidence": "先把组装 worktree 精确移动到 57a2b8f6，再把 clean 的本地 main 恢复到 origin/main@010fad1d；随后版本冻结、L3 和远端引用均基于正确组装分支。",
    "permanentAction": "worktree 创建与后续写操作拆成两个命令，下一次组装在首个 cherry-pick 前同时打印 cwd、branch 和 HEAD。",
    "historicalReleases": []
  },
  {
    "fingerprint": "local-web-validation/npm-scripts/missing-worktree-node-modules",
    "position": "before-production-write",
    "count": 1,
    "impact": "新组装 worktree 的本地 vitest/typecheck 因没有 node_modules 而未启动，没有形成产品测试结论。",
    "recoveryEvidence": "MCP 来源 worktree 的 4 项定向测试已通过；冻结 SHA 在固定 L3 的 typecheck 和 169/1456 全量 Web 测试全部通过。",
    "permanentAction": "本地新 worktree 验证前先检查依赖目录；缺失时明确交由固定 L3，不把命令未启动记录成测试失败或通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-monitor/docker-hub-api/connect-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机到 hub.docker.com 与 auth.docker.io 的公开查询连接超时，未形成镜像通道结论。",
    "recoveryEvidence": "改从已批准 arena-154 使用 Docker registry 客户端读取四个标签，确认 rc.3=preview 且 latest=1.20.0，并按精确摘要完成 E2E。",
    "permanentAction": "公开镜像核验保留 Docker Hub API 与受控验证机 registry inspect 两条只读路径；任一路径失败时只采用另一条实际成功证据。",
    "historicalReleases": []
  },
  {
    "fingerprint": "remote-registry-inspect/ssh-loop/powershell-variable-expansion",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 SSH 循环中的远端 $tag 被 PowerShell 提前展开为空，只输出空标签命令，没有查询到镜像，也没有远端写入。",
    "recoveryEvidence": "改用 PowerShell 单引号封装远端脚本后，四个标签名称与摘要完整返回并交叉核对。",
    "permanentAction": "包含远端 shell 变量的 SSH 命令固定使用单引号或上传脚本，执行前避免由本地 shell 插值。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-shell/powershell-interpolation/variable-colon-adjacency",
    "position": "before-production-write",
    "count": 2,
    "impact": "来源 worktree 清理脚本和首次文档分支推送命令各有一处变量紧邻冒号，PowerShell 均在解析阶段拒绝；未执行清理或远端写入。",
    "recoveryEvidence": "变量改为 ${name} 后，先逐项核对安全路径、clean 状态、精确 HEAD 和远端归档并释放两个来源 worktree；文档更新后重新校验，再以空 ref lease 推送。",
    "permanentAction": "本轮后续 PowerShell 命令不再动态拼接含冒号的 refspec/lease，改用 Git 的独立参数数组或显式 ${name}；同类异常必须在下次发布前复核未再出现。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- Cloudflare run-3 专项审计仍未完成；旧 TLS 设备全矩阵、真实大集群、低配主机和长期弱网 soak 未执行。
- `v1.21.0-rc.3` 主要验证轻量节点和集群 TLS 兼容回退。稳定版前应结合 RC 反馈复核回退触发、缓存生命周期、证书错误不降级及 MCP 接入状态可读性，并重新运行精确候选 L3。
- 不阻断本版的理由：回退只在明确握手超时后触发，默认安全路径不变；同一 SHA 已通过完整 L3、候选/main/tag 门禁、Release 安全链、14 个公开附件和不可变公开 OCI E2E；稳定入口与生产未改变。
