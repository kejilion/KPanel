# KPanel 本地功能预览标准化提案

## 观察证据

- 当前只有 `web/README.md` 的单服务启动说明，Vite 固定 4173、mock API 固定 8080，多个功能会话可能冲突。
- 现有任务交付包记录浏览器证据，但没有统一预览地址、mock/真实数据声明、候选身份、体验步骤和停止方式。
- 已有后台浏览器工作流适合长测，不负责用户即时体验的本地开发预览。

## 规范验收合同

- 基线：`a26054c6592132a2e60426060360eafc53226cf1`。
- 范围：本地预览分层、启动/状态/停止入口、统一预览卡、与任务契约和工作流衔接。
- 非目标：不改变 L0-L3、产品代码、发布流程、远程环境授权、154/108 或生产验收。
- 权威入口：`scripts/local-feature-preview.mjs`、`docs/local-feature-preview-standard.md`、
  `.codex-workflows/local-feature-preview.workflow.yaml`。
- 固定矩阵：正确性、一致性、完整性、可执行性、效率与比例性、可演进性。
- 正常路径：功能会话在独立 worktree 选择 mock/integration，启动、交付预览卡、完成受影响浏览器旅程并停止。
- 回归集：启动器单测、治理一致性、全部工作流 validate、变更感知门禁、实际 mock start/status/stop。
- 停止条件：固定矩阵和回归完成；正常路径阻断为零；其余事项分级并有触发条件。

## 最小方案

1. 保留 `npm run dev`，增加一个跨会话统一启动器，不引入新 npm 依赖。
2. mock API 端口改为受约束环境参数，预览端口自动选择且仅监听回环。
3. 新增唯一规范和工作流，并在任务契约/Definition of Done 中引用。
4. dirty 仅允许 draft；acceptance 必须 clean checkpoint，避免证据绑定不明。
5. 真实主机与长时间浏览器测试继续复用现有环境门禁和后台工作流，不建立第二入口。

## 固定矩阵预期

| 维度 | 通过条件 |
| --- | --- |
| 正确性 | mock、本地集成和隔离真机结论不互相冒充，候选身份可追溯 |
| 一致性 | 规范、任务契约、工作流和启动器引用同一字段与入口 |
| 完整性 | 启动、健康、交付、浏览器旅程、停止、残留和未验证边界齐全 |
| 可执行性 | Windows 当前项目环境可完成 mock start/status/stop，端口并行且证据可读 |
| 效率与比例性 | UI 使用 mock，真实系统才升级到隔离环境；不机械执行长测 |
| 可演进性 | manifest 有 schema version，运行态不入 Git，可新增模式而不改变产品协议 |

## 权限与回滚

本轮允许在专用分支修改规范、工作流、脚本和测试；不允许推送、更新 main、Tag、Release 或部署。
回滚点为基线 `a26054c`，删除本候选差异即可恢复原开发方式。

## 采纳状态

### 固定矩阵结果

| 维度 | 结果 | 证据 |
| --- | --- | --- |
| 正确性 | PASS | dirty 自动降为 draft；acceptance 拒绝 dirty；mock/integration 输出独立证据层级 |
| 一致性 | PASS | 唯一规范、工作流、任务契约、协作流程和治理门禁均引用同一启动器 |
| 完整性 | PASS | start/status/stop、source identity、日志、预览卡、旅程、未验证边界和交接责任齐全 |
| 可执行性 | PASS | Windows 实际完成两套并行 mock 和一套 integration 的启动、健康、状态、停止 |
| 效率与比例性 | PASS | UI 默认 mock；本地 API 才用 integration；真实系统和长测复用既有工作流 |
| 可演进性 | PASS | manifest `schemaVersion: 1`，动态端口和模式可扩展，运行态全部位于忽略目录 |

### 验证证据

- 两套 Mock 同时达到 `ready`，Web 端口自动分配为 4173/4174，证据目录和 PID 独立。
- Local Integration 成功连接回环 API 8899，预览达到 `ready`；启动器拒绝非回环和带凭据 URL。
- 三套预览均由 manifest 精确停止，4173/4174/8080/8081/8899 最终无监听残留。
- 启动器定向测试、治理验证、全部工作流 validate 和 `verify-change` 通过；最终数字以交付报告为准。

实现会话结论为 `PASS`；永久采纳前仍需独立复核并获得主线集成授权。观察窗口为后续 3 个使用本地
预览的功能任务，重点记录端口冲突、用户追问次数、预览残留、mock 误判和正式验收证据缺口。
