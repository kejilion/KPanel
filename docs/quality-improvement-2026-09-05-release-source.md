# L3 隔离源准备

- 状态：已实现，待候选 CI 与实现独立复核；设计独立复核 `PASS WITH FOLLOW-UP` 不代表实现验收。
- 基线与回滚点：`2e2a310532a488a9d55eb05e079483cf64b1ef50`。
- 触发证据：`release-v1.2.0-acceptance.md`、`release-v1.3.0-acceptance.md`、
  `release-v1.3.1-acceptance.md` 均记录 `l3/run-release-l3/local-tag-mismatch` 首轮拦截和 r2 恢复。
- 原因假设：共享 worktree 的 tag 冲突仍由操作者手工换 clone 解决；独立 clone 不携带源仓库
  本机 `core.sshCommand`。历史 tag 冲突如何产生、历史 SSH 故障次数未确认。
- 目标：唯一入口自动准备隔离源，源 refs/config/文件不变；保留真实失败与恢复记录。
- 替代方案：人工 clone 已重复失败；新 wrapper 或共享缓存增加平行入口与状态，不采用。

## 冻结验收合同

- 允许：`scripts/run-release-l3.mjs`、现有 `scripts/tests/release-l3-orchestrator.test.mjs`、本提案。
  现有工作流只在引用实际失效时调整，不新增入口。
- 非目标：产品版本、依赖、Runner、远端 L3 脚本、产品业务、System Center、历史验收记录。
- 权威：`PROJECT_RULES.md` 5.3；入口仍为 `run-release-l3.mjs`，复用 required tags 规则、
  `COVERAGE_BASELINE`、离线 bundle/freshness 和固定远端门禁。
- 正常路径：Windows Git for Windows/Node 24 或 Linux Git/Node 24；clean linked worktree 或
  standalone clone；精确候选可以尚未推送。原候选 HEAD/dirty/main 先检查，base/business tag
  在隔离源精确抓取后检查。非候选祖先的远端 tag 不额外阻断。
- 传输：canonical SSH origin；优先级为 `GIT_SSH_COMMAND`、effective `core.sshCommand`、
  `GIT_SSH`/默认 SSH；`GIT_SSH_VARIANT` 优先于 `ssh.variant`。仅子进程环境沿用，不写配置、
  私钥或完整命令；敏感 Git 错误只输出阶段、稳定代码和退出状态，不持久化原始 stderr。
- 资源：每次增加一个独立源 repo，保留原离线验证 clone；单 Git 子进程最多 120 秒、输出
  最多 8 MiB；源准备总预算 10 分钟，最多 10,000 个远端稳定 tag，不自动重试。
- 恢复：新 run ID/证据路径重试；失败证据保留，仅删除本轮持有且身份未变的临时目录。
  SIGKILL 遗留 `running` 表示不完整，不是成功；未知目录不清理，不增加守护进程。

| 固定维度 | 验收依据 |
| --- | --- |
| 正确性 | 原候选不变，精确 SHA/main/tag/freshness/dirty 与 fail-closed 不削弱 |
| 一致性 | 唯一 CLI 与既有覆盖规则/离线/远端验证，不建第二真源 |
| 完整性 | 正向、失败、中断、秘密和清理边界明确 |
| 可执行性 | 以下有限回归、正常 Windows/Linux 源准备、同 SHA CI 和独立复核 |
| 效率与比例性 | 单个额外源 repo、有界运行、不执行额外产品发布链 |
| 可演进性 | 可证伪、聚焦 revert、有样本边界的观察 |

## 有限回归与证据

R1 正常 clean clone/未推送候选，精确 kit 与源 refs/config/status 前后比对。
R2 linked worktree 冲突 annotated/lightweight tag；源错误 tag 保留，隔离源使用远端身份。
R3 本地缺 tag 自动获取；远端缺 base/business tag 或 shallow/partial 历史不完整失败。
R4 main/tag 在准备前后移动、required tag 删除或新增；不相关别分支 tag 不误阻断。
R5 HEAD 不符、tracked/untracked dirty 或准备中变化失败，源不 reset/stash/clean。
R6 SSH 唯一 repo-local 身份、环境优先级、含空格路径、缺身份、秘密 sentinel 扫描。
R7 超时中断、已有证据路径、临时目录替换、清理失败；未完成不报告成功。
R8 保留已有五项测试及原离线 freshness/固定远端脚本契约。

验证入口：`node --test scripts/tests/release-l3-orchestrator.test.mjs`、治理一致性和
`make verify-change`（Windows 通过仓库 `run-repo-bash.mjs` 适配）。候选提交后检查同 SHA
Linux CI/freshness，独立复核只复查固定矩阵与受影响层。真实远端 L3/生产不属于本次验证。

## 权限、停止与采纳

实现任务允许独立分支聚焦提交及 SSH 推送；只有唯一集成任务能更新 main，随后核对 main CI。
不创建产品 tag/Release/OCI。源码、HEAD、所有权异常先停；不得修改冻结发布树。
矩阵完成、阻断为零、独立复核及同 SHA CI 通过才准入；出现秘密泄漏、源污染或错误放行则拒绝，
必要时以聚焦 revert 回滚，不覆写历史 tag。集成使 main 前移时，旧候选证据按现有冻结规则重验。

未来 5 次正常准备或 14 天观察：手工修源目标 0、源污染 0、错误放行 0；不足样本明确报告。
实际失败/重试沿用原指纹完整记录，不改写过去事故、不为指标漏记。观察不是额外发布等待门槛。
本提案的候选与独立复核结果待精确提交证据补齐，不能将设计通过称为实现通过。

## 本地试行结果

Windows 首轮有限回归 24/24；追加超时项后 24/25，暴露同步终止 Git 后 SSH 子进程仍占用
临时目录的问题。改为有界进程树终止后，PowerShell 定向复验曾报 `termination-failed`，而 Git Bash
门禁通过；进一步定位到 Git for Windows 的 `cmd/git.exe` 启动器与实际 Git 程序差异，入口现在
按当前 Git 安装定位真实进程。修复后 PowerShell 正常准备/超时定向 2/2，最终 Windows
`verify-change` 治理集 120/120，通过相关源码、覆盖、新鲜度和依赖策略检查。

该结果不是首轮全通过。SSH 配置仅在子进程环境沿用，回归扫描 stdout/stderr 与全部 kit 文件，
秘密 sentinel 未泄漏；源 refs/config/status 在临时仓库前后比对保持一致。真实远端 L3、生产、
长期观察和 Linux 候选 CI 尚未由这些本地结果证明。
