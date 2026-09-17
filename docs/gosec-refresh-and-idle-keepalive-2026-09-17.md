# KPanel gosec 覆盖更新与静默会话保活复核（2026-09-17）

本报告衔接 [security-performance-hardening-2026-07-28.md](security-performance-hardening-2026-07-28.md)
（基线 `0e4b5da`：95 条启发式提示、其时人工复核 16 条 High），完成两件事：

1. 以 `gosec v2.28.0` 对当前候选（`03a1795c`，含 `multi-host-terminal` 全量新增代码）重跑
   启发式分析并逐类复核新进入的规则命中；
2. 复核"批量执行 4 小时上限与 30 分钟闲置回收"的交互，并用回归测试钉死结论。

## 1. gosec 重跑结果

命令：`gosec -no-fail -fmt=json -exclude-dir=web ./internal/...`（Go 1.26.7，gosec v2.28.0，与 2026-07 基线同版本规则集）。

| 指标 | 2026-07（`0e4b5da`） | 2026-09（`03a1795c`） |
| --- | --- | --- |
| 扫描文件 / 行数 | 73 / 25,320 | 278 / 96,253 |
| 提示总数 | 95 | 258 |
| High | 16 | 40 |
| `nosec` 压制 | 0 | 0 |

数量上升主因是覆盖面扩大（internal 行数 3.8 倍，新增多主机终端、轻量节点、文件归档、
通知通道包），不是质量回退：增量集中在既有分类（G304 路径变量、G302 权限位、G104
未处理错误），规则口径与 7 月一致，未出现新的可利用模式。

### 新进入规则的复核

| 规则 | 位置 | 复核结论 |
| --- | --- | --- |
| G505 引入弱原语（sha1） | `internal/auth/totp.go` | RFC 6238 规定 TOTP 默认算法为 HMAC-SHA1；此处为规范实现，非缺陷 |
| G101 疑似硬编码凭据 | `internal/notification/store.go` | `telegram-bot-token` 是凭证文件的**文件名常量**，不是凭据本体 |
| G110 解压炸弹 | `internal/backup/archive.go`、`internal/hostbackup/archive.go` | 恢复链有 `OpenRegular` 源大小预检、`LimitedReader`（`MaxBytes + 128 MiB`）读取上限、total/entries 复核三层边界 |
| G202 SQL 拼接 | `internal/ai/context.go` | 拼接对象是包内常量投影函数 `attachmentMetadataProjectionSQL()`，无用户输入路径 |
| G115 整型转换 | `filemanager`/`cluster`/`auth` 等 17 处 | 与 7 月 G115 分类一致：均位于显式边界检查之后的转换（文件大小、偏移、时间窗），输入先前已被限制 |
| G117、G305 等其余 | 各 1–4 处 | 同类既有分类；无新增可利用面 |

### 与 2026-07 基线逐规则对照

| 规则 | 07-28 | 09-17 |
| --- | ---: | ---: |
| G104 | 14 | 75 |
| G115 | 4 | 17 |
| G204 | 14 | 21 |
| G304 | 38 | 85 |
| G302 | 6 | 20 |
| G703 | 3 | 12 |
| G704 | 4 | 4 |
| 其余（G101/G110/G117/G118/G122/G124/G202/G301/G305/G306/G505） | 10 | 18 |

G104/G304 的增长与新增包数量成正比，其形态（best-effort 错误路径、业务根内变量路径）
与 7 月人工复核结论相符，未发现越界样本。G122（inode 竞态）在 07 基线唯一样本处已消除。

### 后续

- gosec 不在 CI 常规门禁（CI 为 govulncheck/npm audit/Trivy）；建议在依赖安全复核作业中
  加入"gosec 定期全量 + 新增文件提示逐项复核"，与本报告同口径。`nosec` 保持 0 是硬约束，
  任何豁免必须走治理评审。

## 2. 静默会话保活复核（4 小时上限 × 30 分钟闲置回收）

针对"批量执行把单机上限提升到 4 小时后，长时间无输出的阶段是否会被 30 分钟闲置回收腰斩"：

调用链复核结论——**现有实现已经是保活的**：

1. 批量前端对每台主机按 `wait=1000ms` 长轮询 output（`web/src/lib/api.ts` 固定携带），
   即使命令静默，每秒也有一次 output 请求；
2. Panel 侧 `handleTerminalOperation` 对每次合法 output/input/resize 刷新会话
   `UpdatedAt`（`internal/panel/terminal.go`），35 分钟索引 TTL 不会触发；
3. Agent 侧 `session.output()` 在读取时刷新 `updatedAt`（`internal/terminal/manager.go`），
   因此 30 分钟 `IdleTimeout` 回收不会命中一个持续被轮询的会话。

新增回归测试 `TestOutputPollingKeepsSilentSessionPastIdleTimeout`
（`internal/terminal/manager_test.go`）：推进时钟 35 分钟（超过 30 分钟闲置上限）并在每分钟
模拟一次 output 轮询，验证会话不被 idle reap 关闭。该测试把"轮询即活跃信号"钉进套件，
防止未来重构在无提示的情况下破坏长任务稳定性。

残余限制（如实记录，不需要改动）：

- 浏览器关闭页面会终止轮询，此时按既有 30 分钟闲置回收兜底，属预期行为（页面上已有文案）；
- 远端 Agent 不可达时关闭请求无法送达，同样由目标侧闲置回收兜底（与
  `docs/multi-host-terminal.md` §5 一致）。

## 3. 文档同步

`docs/multi-host-terminal.md` §4 限额表与 §6 验收清单同步为 4 小时上限与轮询重试语义；
§4 闲置行明确"服务端 30 分钟闲置回收仅作用于不再被轮询的会话"。

## 4. 本报告范围声明

- 只读评估 + 文档/测试补齐；未修改任何运行时代码行为；
- gosec 启发式提示不等于漏洞；本报告的"复核结论"均为人工判断，依据是上述调用链与
  上游规范（RFC 6238）；
- 未覆盖：实机容器逃逸链、宿主发行版 CVE、Trivy 镜像扫描（仅 CI 可执行，未变化时沿用既有证据）。
