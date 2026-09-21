# 本地 WSL 灾备发布验证通道

- 提案状态：已采纳并完成首轮真实移交演练
- 提案日期：2026-09-21
- 负责人：本次治理候选负责人；采纳后由唯一发布任务负责
- 适用业务域：发布验证、灾备执行与证据链
- 基线提交 / 标签：`bae0e533ec985656d39e9655db1b360feb5b7337` / `v1.21.0-rc.6`
- 关联事件：`arena-154` 的 SSH、ICMP 与服务端口在 rc.7 L3 上传前持续超时；候选代码未执行
- 提交、推送、发布和生产权限：治理提交已由用户授权进入主线；本通道只执行候选验证，不授予生产写入

## 观察证据

- 2026-09-21，`arena-154` 在多次有界重试中均不可达，现有唯一 L3 入口只能使用 SSH，候选验证因此停止。
- 本机默认 `Ubuntu` WSL2 具备 Linux 6.6、Docker 29.6.2、Buildx 0.35.0、16 CPU、15 GiB 内存及充足磁盘，root 可访问 Docker socket。
- 固定 Runner 标签未缓存。按历史 Dockerfile 现场重建得到的 ID 为
  `sha256:2cee05d1e25e18bf8f3020cdaba7f76922c56a6270a13027dd9456d378bff7bf`，与已验收 ID
  `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3` 不同；错误标签已删除。
- 已确认缺口：环境策略没有传输类型；计划只保存 Runner 标签，远端记录实际 ID 但不和冻结预期值比对；没有受控的离线 Runner 导入和 WSL 证据回收。
- 后续已从固定构建源生成并冻结可信 Runner 归档；首轮独立接收端按 manifest 摘要执行同一 kit 的完整 L3 已通过。
  该演练验证的是治理提交 `2f6fb8b8...`，不能替代后续 rc.7 最终 SHA 的发布 L3。

## 原因假设

- 假设：把 L3 绑定到单一 SSH 传输且缺少 Runner 身份合同，使验证主机故障成为单点阻塞；直接重建同名镜像又会破坏证据可比性。
- 替代解释：上游网络可能恢复，默认通道仍可继续使用；这不消除单点故障和镜像漂移风险。
- 证伪条件：策略化 WSL 通道无法复用同一 bundle、同一执行脚本和同一内部门禁，或必须降低 Runner/证据门禁才能运行。
- 产品原则：不改变 KPanel 业务真源、Panel/Agent 权限、产品代码、版本或生产部署目标。

## 基线、目标与观察窗口

| 指标 | 当前基线 | 目标 / 守护阈值 | 数据源 | 观察窗口 |
| --- | --- | --- | --- | --- |
| L3 执行环境可用性 | 只有 `arena-154` SSH；本次不可达 | 默认 SSH 不可达时可明确选择 WSL，仍使用唯一 L3 入口 | 计划、终态、环境策略 | 后续 2 次灾备演练或实际使用 |
| Runner 身份完整性 | 只记录实际 ID | 计划冻结预期 ID，任何不一致均在门禁前失败 | `plan.env`、`runner.txt` | 每次 L3 |
| 灾备权限边界 | 未登记 | 只允许 `candidate-validation`，生产及其他用途 100% 拒绝 | 环境策略测试 | 每次治理 CI |
| 证据完整性 | 远端留存 | WSL 的状态、日志和摘要回收到本地 artifact 目录 | `wsl-evidence/` | 每次 WSL L3 |
| 发布责任移交 | kit 只能由生成主机立即执行 | 接收方凭独立 manifest 摘要验证并执行同一 kit/run ID | handoff kit、终态 | 每次移交 |

## 范围与规范验收合同

- 允许修改：环境策略、唯一 L3 编排/执行脚本、定向测试、发布工作流及相关规范。
- 明确不修改：产品功能、版本元数据、GitHub Release、公开镜像、生产部署、`prod-108` 或现有 rc.7 候选。
- 风险等级与验收：永久治理与发布门禁变更，按 L3 候选治理执行；实际产品发布仍需完整 L3 和后续门禁。
- 精确规范基线：上述基线提交中的 `PROJECT_RULES.md` 5.1、环境策略与 `release-kpanel` 3.1。
- 冻结允许范围：登记 `local-wsl-dr`、封闭传输配置、冻结 Runner ID、仓库内固定 Runner 构建源、可选可信离线归档、证据回收、测试与对齐文档。
- 明确非目标：把 WSL 变为生产或通用验收主机；自动回退；现场重建固定 Runner；改变质量阈值。
- 权威入口：`scripts/run-release-l3.mjs` → `scripts/run-release-l3-remote.sh` → `scripts/run-release-gate.sh` → `make verify-release`。
- 固定验收矩阵：正确性、一致性、完整性、可执行性、效率与比例性、可演进性。
- 回归集：策略用途拒绝、计划 Runner ID、归档摘要、脚本封闭解析、SSH 原路径、WSL 缺少/错误 Runner 关闭失败、工作流 stable/preview 渲染、治理检查。
- 停止条件：任一生产权限泄漏、Runner ID 可绕过、证据被覆盖、候选源码或远端引用被修改。
- 范围外发现：单独进入后续候选，不吸收到本治理变更。

## 备选方案与取舍

| 方案 | 收益 | 风险 / 成本 | 结论 |
| --- | --- | --- | --- |
| 等待 `arena-154` 恢复 | 无代码变化 | 单点阻塞保留，无法灾备 | 不采用 |
| 本地临时命令重跑 | 快 | 绕过唯一入口、Runner 与证据合同 | 拒绝 |
| 在唯一入口增加策略化 WSL 传输 | 复用相同 bundle、脚本和门禁 | 需补传输与离线镜像回归 | 采用 |

## 最小改动方案

- `environment-policy.json` 登记 `local-wsl-dr`，只允许 `candidate-validation`，固定 `Ubuntu`/`root`，SSH 环境也显式登记传输。
- L3 计划升级为 schema 2，强制 `EXPECTED_RUNNER_ID`；可选 Runner tar 必须同时冻结 SHA-256，执行脚本先验摘要、导入后再核对实际 ID。
- `packaging/release-runner/Dockerfile` 固定基础镜像摘要和直接系统包版本，修复 npm 跨阶段布局并在构建时执行 Node/npm/npx smoke。
- WSL 仅由 Windows 控制端按参数数组调用，不拼接 Shell；执行后只回收日志、状态和摘要到 `artifactDir/wsl-evidence`。
- prepare-only kit 可通过 `--execute-kit` 在另一登记控制主机继续；manifest 摘要必须独立交接，kit 不含凭据和生产授权。
- WSL/接收主机的标准代理变量只在执行时临时转发；Runner 与固定安全扫描容器使用主机网络兼容回环代理，镜像构建以 BuildKit secret 使用 HTTPS 代理，代理值不进入 kit、计划、manifest、日志、镜像历史或证据。
- Runner 为完成候选工作树构建和嵌套 Docker 生命周期测试必须挂载宿主 Docker socket；该能力本身等价于宿主 root。`AVD-DS-0002` 仅对 `packaging/release-runner/Dockerfile` 建立至 2026-12-21 的有期限例外，保留理由、缓解与退出条件，不降低产品镜像或其他 Dockerfile 的非 root 门禁。
- 默认仍为 `arena-154`，不自动静默回退；候选 CI、主线、Release、公开镜像与生产流程不变。

## 验证与证据层级

- 定向测试：环境策略与 L3 orchestrator 回归、Shell 语法、真实 WSL 的错误 Runner 关闭失败均已验证。
- `make verify-change`：治理测试 219/219 通过；同一治理 SHA 的候选 CI 与 dependency freshness 通过。
- 隔离真机或浏览器：浏览器不适用；真实 WSL 只验证执行通道和失败边界。
- 公开产物与生产：不执行，不适用。
- 真实移交演练：治理 SHA `2f6fb8b8fb093007e9750c853dfd164f3c690de4`，run ID
  `local-wsl-dr-2f6fb8b-l3-r2`，接收端 kit 位于
  `C:/GitHub/_release-evidence/handoff-inbox-local-wsl-dr-2f6fb8b-l3-r2`，独立 manifest SHA-256
  `3a6f4ecc71f43aec9ec5d0e5f484e36baa61dcc281475187be137fc30da0f7e0`，终态 `release_l3_handoff=pass`。
- 固定 Runner：ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；归档 SHA-256
  `f533e0db78d328ee5504e511a22e9769e6be55f53cbd96a3a6ae47f04a969509`。归档只作为受信输入，执行后仍核对实际 ID。
- 未验证项：尚未对 rc.7 最终 SHA 执行完整 WSL L3；RC、Release、公开 OCI 与稳定版生产链仍须各自形成精确证据。

## 独立复核

- 复核人 / 智能体：独立治理复核与批判复核
- 复核提供商 / 实现提供商：Codex（其他提供商不可用；由不同独立复核上下文完成） / Codex
- 是否独立读取原始证据：是；复核覆盖策略、脚本、测试、真实失败证据和成功移交证据
- 假设与方案评审结论：采用显式选择的 WSL 传输与独立 manifest 交接；拒绝自动回退和临时命令旁路
- 门禁是否被削弱、绕过或只对样例优化：否；Runner 身份、候选用途、manifest/plan 身份与证据摘要均关闭失败
- 复核状态：通过

## 回滚与采纳

- 源码 / 规范回滚点：`bae0e533ec985656d39e9655db1b360feb5b7337`。
- 数据或配置备份：无产品数据变化；Git 提交即完整回滚点。
- 触发回滚：生产用途可达、Runner 不一致被放行、SSH 默认路径回归、证据不能绑定精确 run ID。
- 回滚步骤：回退本候选提交，恢复 schema 1 入口并只使用 `arena-154`；重新运行治理与 SSH 路径回归。
- 决策：采纳。主线提交为 `2f6fb8b8fb093007e9750c853dfd164f3c690de4`；默认 SSH 通道保持不变，WSL 仅在显式选择时参与候选验证。
- 规范验收结论：通过。首轮 kit 从生成位置移至独立接收位置后，使用外部持有的 manifest 摘要完成 L3，证明发布验证责任可以移交且证据仍绑定原始 run ID。
- 后续事项：每次交接必须分别传递 kit 与 manifest 摘要；接收方使用登记环境、固定 Runner ID 和唯一入口执行。RC 与稳定版仍按各自精确 SHA 完成全部后续门禁。
