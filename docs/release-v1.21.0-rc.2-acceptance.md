# KPanel v1.21.0-rc.2 发布验收记录

日期：2026-09-20

发布级别：L3

候选提交 / 标签：`8a1998b39e07908f278e6abbb8f934f2722ac0d2` / `v1.21.0-rc.2`

上一稳定版本 / 回滚点：`v1.20.0` / `c98727c898f8b446cceea5d7b10f3205340926a2`

`releaseChannel`：`preview`

`releaseTrain`：`1.21.0`

候选分支与发布后处置：`release/v1.21.0-candidate` / 预览列车保留，远端精确指向 `8a1998b39e07908f278e6abbb8f934f2722ac0d2`

- 原分支 / 精确 tip / 处置分类：`feature/mcp-complete-20260919` / `0dceeaf30454d5f57e9f51aca8aa8c94d7544304` / 已纳入后归档；`feature/service-monitor-design-20260919` / `cf804dae5ec961597f2243474dda673b445d1e15` / 已纳入后归档。
- 归档 ref 与 SHA：`archive/feature/mcp-complete-20260919` = `0dceeaf30454d5f57e9f51aca8aa8c94d7544304`；`archive/feature/service-monitor-design-20260919` = `cf804dae5ec961597f2243474dda673b445d1e15`；远端逐 ref 复核一致。
- 来源提交已合入发布祖先；源分支的审查空提交不重复引入。`release/v1.21.0-candidate` 继续承载后续 RC，不在预览发布后归档。
- 未纳入且未改动：带未提交修改的 `fix/file-host-switch-context-20260913`、`feature/visual-refinement-pass`，以及含受限材料的 `docs/security-audit-run2-local-20260919`。

归档不代表生产上线；本版只发布预览产物，禁止生产部署。

## 发布画像

- 业务域：MCP 完整管理能力、OAuth PKCE 与 stdio 客户端配置；统一 Ping/TCP/HTTP 服务监控检查及状态矩阵。
- 变更面：在 rc.1 只读能力上增加结构化受控写操作、审批记录、目标授权、资源版本校验、持久任务与恢复边界；新增 `kpanel-mcp` 六个平台二进制；增加监控检查的 Panel/Agent 契约、存储、API 和界面。
- 受影响用户旅程：管理员授权主机与集群范围、审批管理操作、下载或配置 MCP 客户端、维护服务监控检查并查看统一状态；备份恢复后重新授权。
- 未变化契约：稳定 GitHub Latest、Docker `latest`、应用市场默认稳定入口、Panel/Agent 端口、`kejilion.sh` 和生产数据均未改变。
- 风险等级及理由：高；新增外部管理写路径与网络检查，但动作集合固定，使用登录态审批、目标授权、源版本、限额、审计和失败关闭，并通过精确候选 L3、双层 CI、Release 安全链与公开 OCI 验收。

## 发布范围与外部审计

- 用户可见更新见 `CHANGELOG.md` `[1.21.0-rc.2]`。MCP 默认关闭；管理能力必须显式授权，模型提供的 `approved` 或 `confirmed` 不构成批准。
- 主要提交：`40f29fda` MCP 管理与客户端适配、`438879c5` 文件授权交集和源版本、`da28ff69` 归档 owner 版本；`ca7924bf` 统一服务检查、`224861e0` 状态矩阵及其 UI 修正；`87bbfc22` 业务上下文刷新、`8a1998b3` 版本冻结。
- Cloudflare `security-audit-skill` scoped run-3 仍因平台中止而未完成，没有最终结构化结论；本次发布不把它改写为通过，也不宣称完整专项安全审计覆盖。
- open-code-review 1.12.6 的来源候选记录为 59/59 个适用文件已观察，提交 trailer 完整，独立复核为 PASS；该能力作为辅助证据，不替代项目门禁。
- 稳定版状态：尚未交付；生产部署：未执行。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与互通 | 已验证 | Go 全量测试、MCP SDK/stdio/OAuth/管理动作、Panel/Agent 监控契约和 Web 组件测试通过。 | 未逐一安装第三方 MCP 产品。 |
| 网络与供应链安全 | 已验证 | Bearer、Origin/TLS、PKCE、目标授权、版本冲突、审批与输出预算回归通过；govulncheck、npm audit、Trivy 源码和镜像均为 0 阻断项。 | run-3 专项审计未完成。 |
| 稳定性与失败恢复 | 已验证 | race、操作容量、幂等/冲突、备份 owner、应用安装/更新/回滚/卸载生命周期及公开 OCI 冷启动通过。 | 未执行长期高并发 soak。 |
| 性能与资源预算 | 已验证 | MCP 请求、响应、队列、持久记录和并发有固定上限；监控检查使用统一有界列表。 | 未在真实大规模 Agent 集群压测。 |
| 用户体验与可访问性 | 已验证 | Web 类型检查与 169 个文件、1456 项测试通过，覆盖监控布局、窄屏状态矩阵、MCP 授权/OAuth/配置。 | 本次未另做手工第三方客户端和真机浏览器全旅程。 |
| 数据、配置与迁移 | 已验证 | 新状态独立持久化；资源版本防止陈旧写入；恢复撤销 MCP/OAuth/集群授权；旧版本忽略新增文件。 | 未在生产数据目录执行恢复。 |

## 自动门禁

- 本机 `VERIFY_LEVEL=l2` 的 197 项治理测试通过，但代码阶段在 preflight 因缺少 `go/gofmt/make` 失败；未把该结果记为 L2 通过，改用固定远程 L3 对同一 SHA 完整执行。
- L3：`kpanel-release-gate:go1.26.7-node24`，Runner digest `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；Go 全量、race、Web 169/1456、构建、govulncheck、npm audit、Trivy、镜像与 `app_conf_lifecycle=pass` 全部通过。
- L3 外层入口：run ID `v1.21.0-rc.2-8a1998b-l3-r1`，2026-09-20 07:38:37 至约 07:54 +08:00，exit 0；plan `4277da8f...`、remote entry `d8bb2cf2...`、bundle `ddfb93a6...`、远端日志 `c70a7ce5...`；证据位于 `C:/GitHub/_release-evidence/v1.21.0-rc.2-8a1998b-l3-r1` 与 `arena-154:/root/kpanel-release-evidence/v1.21.0-rc.2-8a1998b-l3-r1`。
- 候选 CI：[CI 35477358621](https://github.com/kejilion/KPanel/actions/runs/35477358621) 与 [freshness 35477358591](https://github.com/kejilion/KPanel/actions/runs/35477358591) 成功，绑定精确 SHA。
- 主线 CI：[CI 35477741173](https://github.com/kejilion/KPanel/actions/runs/35477741173) 与 [freshness 35477741192](https://github.com/kejilion/KPanel/actions/runs/35477741192) 成功，绑定精确 SHA。
- Release：[workflow 35478139425](https://github.com/kejilion/KPanel/actions/runs/35478139425) 与 tag freshness `35478139427` 成功；2026-09-20 08:22:48 +08:00 完成。

## 发布产物与隔离验收

- [KPanel v1.21.0-rc.2](https://github.com/kejilion/KPanel/releases/tag/v1.21.0-rc.2) 为非 draft、prerelease、非 Latest；GitHub Latest 仍为 `v1.20.0`。
- Docker `1.21.0-rc.2` 与 `preview` OCI index 均为 `sha256:1836a60ffd9a39379ff70f43202a4006cbaead272d8571b36067bdb5cf57f4fd`；稳定 `latest` 保持 `sha256:a991b5d27d7d6519c694b6cbc88c037bbaa0fcef418a43b0ff94cb638dee575f`。
- `linux/amd64` / `linux/arm64` digest：`sha256:5f287a0ba5508d89dcf5482a95421f124644e60941f6342882c2e25bfaf555c9` / `sha256:daae1bd9fdfa0947e35a86f8af41541e42464669a92052889c7a6ae7e562493f`；另两项 `unknown/unknown` 为 attestation。
- 14 个附件逐项返回 200：MCP 六个平台二进制、Agent/Node 双架构、部署包、LICENSE、`SHA256SUMS` 和 THIRD_PARTY_NOTICES。
- `arena-154` / `candidate-validation` 公开镜像 E2E：精确源码 archive SHA-256 `e85315906f993cfe7bb43d0a66f85b240536fc48c7756c8b13d5d4e6edd5ba3d`，固定入口 `packaging/tests/image-e2e.sh` SHA-256 `1378218f9d4ac0fdd82d66ac502c5f7e0d82a13fca8d60429079936ed527edf4`，日志 SHA-256 `4c12a1ca59f82896c2d21326b760e6f557d9f5858a5ff831491db3090194b812`，结果 `image_e2e=pass`。
- E2E 按不可变摘要运行只读根文件系统、drop all capabilities、no-new-privileges 和独立网络，验证版本健康、静态资源、bootstrap、安全 Cookie 与容器健康，结束后自动清理。

## 自更新、生产与回滚

- 稳定通道继续只接受正式 Latest；预览通道可接收规范 RC，并在没有新 RC 时接收更新的稳定版。发布本版只提升 `preview`，没有改变通道选择协议。
- 生产目标和授权：不适用；预览版禁止生产部署。`prod-108` 未连接、未备份、未部署、未升级、未核对。
- 生产写操作：0。`arena-154` 仅用于候选 L3 与公开 OCI 隔离验证。
- 稳定回滚点：`v1.20.0` / `c98727c...` / `sha256:a991b5d2...`；上一预览为 `v1.21.0-rc.1` / `sha256:3add580b...`。回滚前关闭 MCP，旧版本忽略新增目录，恢复/回滚会撤销委派授权。
- `scriptLinkageState=not-required`：内置脚本仍为 `6ebb945f6d5cb69fdb41e3761de23566acbaf762`，SHA-256 `28cf3934c01fe79a19c51fac520f11a2bdd7656d762f37d6fc4153140a6df549`；应用市场默认 `latest` 不变。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-20T07:31:03+08:00
- 候选冻结时间：2026-09-20T07:36:23+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

产品载荷未造成生产回滚、紧急热修复或重复发布；本次预览版没有生产写操作。下列流程异常均在发布前或只读验收阶段被发现，没有逃逸发布门禁。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "local-l2/toolchain-preflight/missing-runtime",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机 L2 在治理测试通过后因缺少 go、gofmt、make 被 preflight 阻断，未执行产品代码阶段。",
    "recoveryEvidence": "固定 arena-154 L3 对同一 SHA 完整执行并通过测试、race、扫描、构建和生命周期；本机结果未记为 L2 通过。",
    "permanentAction": "发布维护者继续以固定 L3 作为缺少本机工具链时的发布门禁，并在下次本机环境维护时复核 Go 与 make；不得用治理测试替代代码验证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-l3/base-tag/stable-only-input",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次 L3 调用误把 v1.21.0-rc.1 作为 base tag，入口按稳定标签规则立即拒绝，远程验证未启动。",
    "recoveryEvidence": "改用当前稳定基线 v1.20.0 后，准备清单和完整 L3 均通过，候选 SHA 未变化。",
    "permanentAction": "后续 RC 的 L3 base tag 固定取当前稳定版，前置命令生成时校验必须匹配 vX.Y.Z；下次 RC 发布复核。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-monitor/github-api/anonymous-rate-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "标签 Release 运行期间匿名 GitHub REST 额度耗尽，结构化状态轮询短时不可用；Actions 本身未失败。",
    "recoveryEvidence": "改用公开 Actions HTML 持续核验至 Success，并由 Release 页面、附件和 Docker Hub 公开状态完成交叉复核。",
    "permanentAction": "无认证发布监控优先使用低频公开页面查询，REST 仅用于关键节点；后续发布继续复核并避免高频轮询。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release-artifact/github-download/transient-connect-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次用 curl 核对附件时到 github.com:443 连接超时，单次探针没有形成有效资产证据。",
    "recoveryEvidence": "使用 PowerShell HTTP 客户端并带有限重试后，14 个预期附件逐项返回 200。",
    "permanentAction": "附件验收使用有限重试并区分网络不可达与 404；只有逐项成功才记录附件完整。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- run-3 Cloudflare 专项审计仍未完成；第三方 MCP 客户端、真实大集群、低配主机和长期并发 soak 未执行。
- `v1.21.0-rc.2` 是完整管理扩展的首个公开 RC。稳定版前应结合 RC 反馈复核授权、审批、资源版本、恢复撤权和监控检查负载，并重新运行精确候选 L3。
- 不阻断本版的理由：能力默认关闭且受限；同一 SHA 已通过完整 L3、候选/main/tag 门禁、Release 安全链、14 个公开附件和不可变公开 OCI E2E；稳定入口与生产未改变。
