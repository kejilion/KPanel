# KPanel v1.14.0 发布验收

## 发布范围

本次发布把 v1.13.0 之后已完成的候选整理为 v1.14.0，产品提交固定为 `2bf866df47bdfef79324103e20242fade8d38dea`。主要变化包括：应用市场最多并行运行四个相互隔离的原生脚本任务；集群主机历史监控和指标导航统一；远程历史指标压缩分块传输并限制浏览器查询资源；移动端文件批量操作栏不再遮挡文件行。

没有数据库 schema、端口、Compose、节点身份或应用数据迁移。配套 `kejilion/sh` 先以纯快进发布到 `b776ae85850bd50c86b2902a904361d7a0794e39`，根脚本 SHA256 为 `766b458ac2c017021c4186dd99e508471148cd8b924c5b194e6b781ea61cc10d`，CN 脚本 SHA256 为 `6cb3fdbc34b8b15feadca147d0577c7ac9d57be153a723538d72223b687dd9d0`。`scriptLinkageState=coupled`。

候选分支 `release/v1.14.0-candidate`、远端 `main` 和标签 `v1.14.0` 都曾精确绑定产品 SHA。Release workflow 成功后自动删除候选远端分支；标签保持不可变，后续验收文档提交不是产品 revision。

## 自动门禁

最终候选 L3 run `v1.14.0-2bf866d-l3-r1` 于 2026-09-11T13:58:43Z 至 14:12:47Z 首轮完成，exit 0。固定 Runner 为 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，plan SHA256 `95927d4e15f28efb464e91559435eaa0ee4a4d6ca61af9a21f11a69ce226a65e`，remote entry SHA256 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle SHA256 `26b3c5acc678e5521c4a344f073b63d93aedf512e25b4a57c11b84ee8d6d569c`。完整本地证据为 `C:/GitHub/_release-artifacts/v1140-2bf866d-l3-r1`。

L3 覆盖 164 项治理/编排测试、Go 全量与核心 race、152 个前端文件/1348 项测试、2241 条 i18n、typecheck、production build、双架构二进制、安装安全、app-conf 生命周期、受管脚本契约和最终镜像。`govulncheck`、npm audit、Trivy 源码/配置与最终镜像扫描均无命中。

脚本候选在 arena-154 的精确 Git bundle clone 中通过根/CN Bash 语法、同步、应用目录刷新、任务分派、非交互、Docker 应用生命周期和 8 项并行测试。第一次 Windows 源归档和第二次 bundle verify 调用在执行产品测试前失效；第三次固定 clone 证据完整通过，失败证据未拼接。

候选 dependency freshness [34608843363](https://github.com/kejilion/KPanel/actions/runs/34608843363) 与 CI [34608843388](https://github.com/kejilion/KPanel/actions/runs/34608843388) 成功；main dependency freshness [34609528232](https://github.com/kejilion/KPanel/actions/runs/34609528232) 与 CI [34609528284](https://github.com/kejilion/KPanel/actions/runs/34609528284) 成功；tag dependency freshness [34610424367](https://github.com/kejilion/KPanel/actions/runs/34610424367) 与 Release [34610424410](https://github.com/kejilion/KPanel/actions/runs/34610424410) 成功，全部绑定 `2bf866d`。

## 依赖与应用市场契约

dependency freshness 覆盖 policy 要求的依赖组。业务上下文基线仍为 v1.12.0 / `0ff1e32`，候选距基线 16 个提交，未放宽 50 提交阈值。本版没有 Go module、npm package、Go 1.26.7、Node 24.20.0、基础镜像、Action 或 Trivy 0.72 升级。

`packaging/kejilion-app/kpanel.conf` 相对 v1.13.0 未变，且与干净的 `kejilion/apps` main `2d8044adec98e3eb16f47cdbb297f6be9632a66f` 归一全文一致，因此没有创建 apps 空提交。

## 公开制品验收

[v1.14.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.14.0) 于 2026-09-11T14:38:25Z 公开，为 Latest、非 draft、非 prerelease。8 个附件齐全：两种架构的 Agent、两种架构的 Node、`kejilion-panel-deploy-1.14.0.tar.gz`、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。5 个受 SHA256SUMS 管理的文件均已下载并逐项匹配；LICENSE 与 notices 的归一内容和标签源码一致。证据为 `C:/GitHub/_release-artifacts/v1140-github-release`。

Docker Hub `1.14.0` 与 `latest` 同指 OCI index `sha256:238a1f9d855325549736032ee401791e1fa12284b953233858251140487437bd`。linux/amd64 manifest 为 `sha256:a7561abca8b1d750fa377e10425aee0cf0327e8244bda886316377f82f11afbc`，linux/arm64 为 `sha256:4e49fc1c6a82c8de6eee846f8cc391aa1a798800fbb649c70336e850f930835e`；两个 unknown/unknown 条目为 attestations。

公开镜像在 arena-154 的临时端口 18114 按固定 `packaging/tests/image-e2e.sh` 运行，验证 version `1.14.0`、revision `2bf866d`、User `65532:65532`、受管脚本 SHA256、健康、首页、代理头、真实静态资源、bootstrap、Secure cookie、单网络和最终 container health，输出 `image_e2e=pass`。临时容器、网络和端口已清理；证据为 `C:/GitHub/_release-artifacts/v1.14.0-public-2bf866d-r1`。

## 生产部署安全核对

用户明确授权本次完整上线。验证和正式生产目标均为 `arena-154` / `154.36.153.9`；`prod-108` / `108` 禁止全部 KPanel 操作，本轮没有连接或修改。

生产 preflight run `v1.14.0-production-20260911` 于 14:42:24Z 至 14:42:26Z 确认原生产 v1.13.0、revision `2718726`、旧 OCI `sha256:9dc2ce1cf57bd3f56e001cdcdbca39f40fd3a7a2f6f401258e495d7a440c4043` 健康。Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite quick check 通过，63 个数据文件，磁盘可用约 23 GiB。

备份阶段于 14:43:06Z 至 14:43:15Z 使用固定入口停写并创建 `/root/kpanel-backups/pre-v1.14.0-20260911T144306Z`，包含数据/配置、旧镜像、service、inspect 和 SHA256SUMS。逐项校验后恢复 v1.13.0 健康，保护文件与 preflight 一致。恢复健康探测发生一次连接重置，固定入口在同一阶段内恢复并最终通过，未进入产品部署。证据为 `C:/GitHub/_release-artifacts/v1140-production-preflight-r1` 与 `C:/GitHub/_release-artifacts/v1140-production-backup-r1`。

生产只执行标准应用市场入口：

```text
env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel
```

命令退出 0，拉取 `latest@sha256:238a1f...` 并完成 Panel、Agent 和受管脚本事务更新。postdeploy 于 14:44:09Z 至 14:44:11Z 通过：version `1.14.0`、revision `2bf866d`、OCI digest 和脚本 SHA256 精确一致；Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite `panel/ai.db=ok`，保护文件 diff 为空，63 个数据文件保持不变。部署后资源约 74.88 MiB / 256 MiB、CPU 0.03%、PIDs 7。

公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.14.0`。生产证据为 `C:/GitHub/_release-artifacts/v1140-production-postdeploy-r1`。

生产未执行真实 light node 身份重置、文件上传写入、应用任务强制终止、故障注入或回滚演练。本轮没有生产退化、数据迁移、回滚、紧急热修复或重复正式发布。

## 回滚

- 源码/tag：v1.13.0 / `271872628c1882cd444d82829d2bf585fa8ddeb7`；v1.14.0 tag 保持不可变。
- 镜像：`docker.io/kjlion/kejilion-panel@sha256:9dc2ce1cf57bd3f56e001cdcdbca39f40fd3a7a2f6f401258e495d7a440c4043`。
- 脚本：`kejilion/sh@7369e1d66d9db304ba9956a9e137930414629ed2`。
- 数据/配置：`/root/kpanel-backups/pre-v1.14.0-20260911T144306Z`，含旧镜像和已校验 SHA256SUMS。
- 若需回滚，停止 Agent/Panel，恢复备份数据、配置、service、旧镜像和脚本 pin，再启动并重复 health、Agent、OCI、SQLite、保护文件和日志检查；本轮未实际触发。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-10T00:12:33+08:00
- 候选冻结时间：2026-09-11T21:57:44+08:00
- 生产完成时间：2026-09-11T22:44:11+08:00
- 提交到生产用时：46.53 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终 SHA 的 L3、候选 CI/freshness、main CI/freshness、tag freshness、Release、公开附件、公开镜像 E2E 和生产三阶段门禁均首轮产品通过。以下单独统计发布编排、取证或证据读取异常，不把它们写成产品失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：7
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/script-source/windows-line-endings",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows Git archive 把脚本候选转换为 CRLF，第一次 Bash 语法检查失败，候选代码测试未开始。",
    "recoveryEvidence": "改用 arena-154 中 core.autocrlf=false 的精确 Git bundle clone，根/CN 语法和全部脚本测试通过。",
    "permanentAction": "本版后续脚本验证已固定只使用 Linux 精确 bundle clone，并将该要求继续作为跨仓库发布入口约束。",
    "historicalReleases": ["v1.13.0"]
  },
  {
    "fingerprint": "preflight/script-bundle/verify-context",
    "position": "before-production-write",
    "count": 1,
    "impact": "第二次脚本验证在仓库外执行 git bundle verify，Git 拒绝该调用，候选代码测试未开始。",
    "recoveryEvidence": "先从 bundle 创建独立 clone，再在 clone 内验证精确 SHA，第三次完整脚本门禁通过。",
    "permanentAction": "bundle 证据入口先 clone，再在仓库上下文执行 verify 和 checkout，不在裸目录直接调用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-published-hash/command-escaping",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次内联 awk 校验因跨 Shell 控制字符转义失败，未形成已发布脚本哈希证据。",
    "recoveryEvidence": "改用上传的固定文件脚本重新读取公开 raw 内容，根/CN SHA256 与候选 pin 一致。",
    "permanentAction": "跨 Shell 哈希核对使用版本化或上传文件承载，不把 awk 程序嵌入多层命令字符串。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/orchestrator-cli/unsupported-help",
    "position": "before-production-write",
    "count": 1,
    "impact": "对生产证据编排器传入未支持的 --help，只返回 usage，没有创建证据或连接生产。",
    "recoveryEvidence": "读取固定入口源码与工作流示例后，preflight、backup 和 postdeploy 都以支持的参数首轮通过。",
    "permanentAction": "该指纹已连续重复；后续不再探测 --help，只以仓库工作流示例为调用真源，并在入口新增 help 契约前保持此禁用规则。",
    "historicalReleases": ["v1.13.0", "v1.12.0"]
  },
  {
    "fingerprint": "release/tag-fetch/local-clobber-conflict",
    "position": "before-production-write",
    "count": 1,
    "impact": "标签前的广泛 --tags fetch 遇到本地历史 v0.86.2 与远端对象冲突；v1.14.0 的精确 ref 检查仍有效。",
    "recoveryEvidence": "独立核对远端 main、候选 SHA 和目标标签不存在后，只创建并推送 v1.14.0；peeled tag 精确指向 2bf866d。",
    "permanentAction": "发布前只 fetch 所需分支并单独查询目标标签，禁止为单版本发布执行无范围的 --tags fetch。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/evidence-read/missing-summary-file",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次本地摘要读取假定存在 summary.env，状态文件已读到但组合命令退出 1；权威 preflight 证据未受影响。",
    "recoveryEvidence": "按实际 evidence 树读取 status.txt 与 snapshot 下的 health、Agent、OCI、SQLite 和资源文件，确认 preflight passed。",
    "permanentAction": "生产证据摘要只读取 manifest 列出的文件和已枚举的 snapshot 路径，不假定额外汇总文件存在。",
    "historicalReleases": []
  },
  {
    "fingerprint": "backup/health/transient-connection-reset",
    "position": "after-production-write",
    "count": 1,
    "impact": "停写备份恢复后的第一次健康探测遇到连接重置，固定入口继续有界探测，未开始产品更新。",
    "recoveryEvidence": "同一 backup gate 最终确认 v1.13.0、Panel、Agent、SQLite 和保护文件全部健康，随后 postdeploy 也首轮通过。",
    "permanentAction": "保留备份恢复后的有界健康等待与最终状态断言，单次连接重置不得绕过 gate 或触发直接部署。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与清理

远端候选分支已由 Release workflow 删除。公开 E2E 临时容器、网络和端口已清理；保留 L3、脚本、公开附件、公开镜像和生产三阶段证据，以及唯一生产恢复包。

未验证风险包括真实 light node 重接入/弱网/大文件、最终 SHA 的完整人工浏览器矩阵、长期 soak、生产故障注入和受控回滚。它们不阻断本版，因为最终 SHA 的协议与失败态自动测试、配套脚本精确测试、公开 OCI E2E、生产备份和 postdeploy 均已通过，且生产没有主动修改对应业务数据。
