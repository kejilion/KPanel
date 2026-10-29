# KPanel 依赖生命周期复核（2026-10-29）

## 范围与证据身份

- 复核基线：`ac65c30d1f773a7da325265d7d1c8f25174b648b`（KPanel `origin/main`）。
- 完整依赖报告：GitHub Actions run [40893882484](https://github.com/kejilion/KPanel/actions/runs/40893882484)，生成于 2026-10-29 14:41:36 UTC，必需检测源 9/9 成功。报告发现 35 个直接依赖/基座候选（兼容补丁 7、次版本 21、主版本/工具链/基座 7）、310 个传递依赖归属信号，紧急安全候选为 0。该次作业失败是因为两条例外及 EOL 复核已过期，不代表检测器失败，也不表示所有候选都应升级。
- 本机报告 `C:\GitHub\_task-evidence\kpanel-v128-dependency-review-20261029\dependency-report-r2.md` 仅有 4/9 个检测源成功（缺少 Windows 原生 Go，部分上游请求失败），只用于记录环境限制，不作为完整性证据。
- 无生产主机检查；本记录是依赖维护决策，不是安全审计，也不包含 CF 审计。

## 生命周期复核结论

- **Go 构建器**：当前 `go.mod` 为 Go 1.27.3，依照 [Go 发布与支持政策](https://go.dev/doc/devel/release) 继续使用受支持分支的安全补丁；当前无须为候选数量进行无关升级。
- **Node 构建器**：Dockerfile 固定 Node 24.22.1 / Alpine 3.24，并把最终产物复制到 `scratch`，Node 不进入最终运行镜像。Node 24 仍在 LTS；Node 26.13.0 于 2026-10-28 才转为 LTS，并支持到 2029 年 4 月。[Node 发布状态](https://nodejs.org/en/about/previous-releases) [Node 26.13.0](https://nodejs.org/uk/blog/release/v26.13.0)
- **Node 24 补丁**：Node 24.22.1 已包含 2026-10-21 官方高危安全修复；24.22.2 于 2026-10-28 发布，当前完整报告同时发现原固定镜像 tag 的 digest 候选。它们是独立的补丁/基座维护信号，按既有时限继续跟踪；本次不把 24→26 主版本迁移混入稳定候选。[Node 24.22.1 安全发布](https://nodejs.org/en/blog/release/v24.22.1/) [Node 24.22.2](https://nodejs.org/fr/blog/release/v24.22.2)
- **Alpine**：构建基础为 Alpine 3.24；按 [Alpine 分支支持表](https://alpinelinux.org/releases/) 继续跟踪至其支持结束。最终镜像为 `scratch`，不携带 Alpine、Node 或 Go 用户态运行环境。
- **部署宿主机**：Docker、containerd、runc 和内核版本未读取；不能从 CI 镜像推断生产宿主机是否受支持或安全。

## 有期限例外复核

1. **TypeScript 6.0.3 / 候选 7.0.2**：继续暂缓 TypeScript 7。KPanel 现有兼容日志记录 `vue-tsc` 3.3.11 和 3.3.13 均在进入 typecheck 前因 `typescript/lib/tsc` 子路径不可导出而失败；当前 npm 最新 `vue-tsc` 仍为 3.3.13（输出保存在仓库外 `C:\GitHub\_task-evidence\kpanel-v128-dependency-review-20261029\vue-tsc-current-version.txt`）。Vue Language Tools 也有 TypeScript 7.0.2 兼容失败记录（[issue #6124](https://github.com/vuejs/language-tools/issues/6124)）。保留 6.0.3，不关闭任何类型检查；退出条件仍是完整 typecheck、测试、场景包检查和生产构建通过。负责人：KPanel base maintenance；复核截止：2026-11-28。
2. **@types/node 24.19.0 / 候选 26.6.6**：更新记录中的候选号并继续暂缓 24→26 配套迁移。Node 26 刚进入 LTS，但 KPanel 当前构建仍固定 Node 24；应在单独工具链候选中同时验证 runtime、类型声明、Web typecheck、测试、场景包、镜像和双架构构建。负责人：KPanel base maintenance；复核截止：2026-11-28。

回滚点维持原例外记录的 `9c5be9c293f98ad9bce260a4562e28743951b105`。本复核不修改运行时、依赖锁或业务代码。下一次 EOL 复核不晚于 2027-01-29。
