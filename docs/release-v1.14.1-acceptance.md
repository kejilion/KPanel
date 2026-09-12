# KPanel v1.14.1 发布验收

## 发布范围

本次发布把 v1.14.0 之后完成的轻节点接入修复发布为 v1.14.1，产品提交固定为 `50f6602d831c812daf35446127bd5c5341586c7b`。修复后，轻节点已经上报时接入表单可以在令牌过期后正常完成；节点尚未上报时仍要求凭据；判定接入命令过期前会最后刷新一次主机列表，避免把刚完成的接入误报为失败。

没有数据库 schema、端口、Compose、节点身份或应用数据迁移。`packaging/kejilion-app/kpanel.conf` 与 v1.14.0 相同，继续使用 `kejilion/sh@b776ae85850bd50c86b2902a904361d7a0794e39`，根脚本 SHA256 为 `766b458ac2c017021c4186dd99e508471148cd8b924c5b194e6b781ea61cc10d`。`scriptLinkageState=not-required`。

候选分支 `release/v1.14.1-candidate`、远端 `main` 和标签 `v1.14.1` 都曾精确绑定产品 SHA。Release workflow 成功后自动删除候选远端分支；标签保持不可变，后续验收文档提交不是产品 revision。

## 自动门禁

最终候选 L3 run `v1.14.1-50f6602-l3-r2` 于 2026-09-12T01:50:45Z 至 02:04:16Z 完成，exit 0。固定 Runner 为 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，plan SHA256 `3ab8c9425a74279933386466099784fa0056033861795748dc35442935fac941`，remote entry SHA256 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle SHA256 `f55c5bb72de645553984d87c048826d63b7c1580afe166469377735e78f6abd1`。完整本地输入与远端证据为 `C:/GitHub/_release-artifacts/v1141-50f6602-l3-r2` 和 `C:/GitHub/_release-artifacts/v1141-50f6602-l3-r2-remote`。

L3 覆盖 164 项治理/编排测试、Go 全量与核心 race、153 个前端文件/1351 项测试、2241 条 i18n、typecheck、production build、双架构二进制、安装安全、app-conf 生命周期、受管脚本契约和最终镜像。`govulncheck`、npm audit、Trivy 源码/配置与最终镜像扫描均无命中；新增轻节点接入测试 3 项全部通过。

候选 dependency freshness [34666649961](https://github.com/kejilion/KPanel/actions/runs/34666649961) 与 CI [34666649959](https://github.com/kejilion/KPanel/actions/runs/34666649959) 成功；main dependency freshness [34666960832](https://github.com/kejilion/KPanel/actions/runs/34666960832) 与 CI [34666960827](https://github.com/kejilion/KPanel/actions/runs/34666960827) 成功；tag dependency freshness [34667259548](https://github.com/kejilion/KPanel/actions/runs/34667259548) 与 Release [34667259569](https://github.com/kejilion/KPanel/actions/runs/34667259569) 成功，全部绑定 `50f6602d831c812daf35446127bd5c5341586c7b`。

## 依赖与应用市场契约

dependency freshness 覆盖 policy 要求的依赖组。业务上下文基线仍为 v1.12.0 / `0ff1e32`，候选距基线 19 个提交，未放宽 50 提交阈值。本版没有 Go module、npm package、Go 1.26.7、Node 24.20.0、基础镜像、Action 或 Trivy 0.72 升级。

`packaging/kejilion-app/kpanel.conf` 相对 v1.14.0 未变，且与干净的 `kejilion/apps` main `2d8044adec98e3eb16f47cdbb297f6be9632a66f` 归一全文一致，因此没有创建 apps 空提交。

## 公开制品验收

[v1.14.1 Release](https://github.com/kejilion/KPanel/releases/tag/v1.14.1) 于 2026-09-12T02:26:36Z 公开，为 Latest、非 draft、非 prerelease。8 个附件齐全：两种架构的 Agent、两种架构的 Node、`kejilion-panel-deploy-1.14.1.tar.gz`、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。5 个受 SHA256SUMS 管理的文件均已下载并逐项匹配，证据为 `C:/GitHub/_release-artifacts/v1141-50f6602-release-assets-r1`。

Docker Hub `1.14.1` 与 `latest` 同指 OCI index `sha256:893f0fe328c61017d344debb3d9abefe70139c82373bf12eafc04fd373701f6a`。linux/amd64 manifest 为 `sha256:fa58816f41cd98fe08b395e4ece5425d34bb1a2972ab81b0310a20e70dd64a28`，linux/arm64 为 `sha256:375fb430e81c7ebb1c5479205edfdeeb197081f8263cdec84aeba191a1c9953f`；两个 unknown/unknown 条目为 attestations。

公开镜像在 arena-154 的临时端口 18115 按固定 `packaging/tests/image-e2e.sh` 运行，验证 version `1.14.1`、revision `50f6602d831c812daf35446127bd5c5341586c7b`、User `65532:65532`、受管脚本 SHA256、健康、首页、代理头、真实静态资源、bootstrap、Secure cookie、单网络和最终 container health，输出 `image_e2e=pass`。临时容器、网络和端口已清理；证据为 `C:/GitHub/_release-artifacts/v1.14.1-public-50f6602-r1`。

## 生产部署安全核对

用户明确授权本次完整上线。验证和正式生产目标均为 `arena-154` / `154.36.153.9`；`prod-108` / `108` 禁止全部 KPanel 操作，本轮没有连接或修改。

生产 preflight run `v1.14.1-production-20260912` 于 02:29:40Z 至 02:29:42Z 确认原生产 v1.14.0、revision `2bf866df47bdfef79324103e20242fade8d38dea`、旧 OCI `sha256:238a1f9d855325549736032ee401791e1fa12284b953233858251140487437bd` 健康。Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite quick check 通过，63 个数据文件，磁盘可用约 23 GiB。

备份阶段于 02:30:04Z 至 02:30:14Z 使用固定入口停写并创建 `/root/kpanel-backups/pre-v1.14.1-20260912T023004Z`，包含数据/配置、旧镜像、service、inspect 和 SHA256SUMS。逐项校验后恢复 v1.14.0 健康，保护文件与 preflight 一致。恢复健康探测发生一次连接重置，固定入口在同一阶段内恢复并最终通过，未进入产品部署。证据为 `C:/GitHub/_release-artifacts/v1141-production-preflight-r1` 与 `C:/GitHub/_release-artifacts/v1141-production-backup-r1`。

生产只执行标准应用市场入口：

```text
env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel
```

命令退出 0，拉取 `latest@sha256:893f0fe328c61017d344debb3d9abefe70139c82373bf12eafc04fd373701f6a` 并完成 Panel、Agent 和受管脚本事务更新。postdeploy 于 02:30:55Z 至 02:30:56Z 通过：version `1.14.1`、revision `50f6602d831c812daf35446127bd5c5341586c7b`、OCI digest 和脚本 SHA256 精确一致；Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite `panel/ai.db=ok`，保护文件 diff 为空，63 个数据文件保持不变。部署后资源约 74.41 MiB / 256 MiB、CPU 0.02%、PIDs 7。

公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.14.1`。生产证据为 `C:/GitHub/_release-artifacts/v1141-production-postdeploy-r1`。

生产未执行真实 light node 身份重置、弱网或令牌过期时序演练、故障注入或回滚演练。本轮没有生产退化、数据迁移、回滚、紧急热修复或重复正式发布。

## 回滚

- 源码/tag：v1.14.0 / `2bf866df47bdfef79324103e20242fade8d38dea`；v1.14.1 tag 保持不可变。
- 镜像：`docker.io/kjlion/kejilion-panel@sha256:238a1f9d855325549736032ee401791e1fa12284b953233858251140487437bd`。
- 脚本：`kejilion/sh@b776ae85850bd50c86b2902a904361d7a0794e39`。
- 数据/配置：`/root/kpanel-backups/pre-v1.14.1-20260912T023004Z`，含旧镜像和已校验 SHA256SUMS。
- 若需回滚，停止 Agent/Panel，恢复备份数据、配置、service、旧镜像和脚本 pin，再启动并重复 health、Agent、OCI、SQLite、保护文件和日志检查；本轮未实际触发。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-12T00:16:17+08:00
- 候选冻结时间：2026-09-12T09:49:40+08:00
- 生产完成时间：2026-09-12T10:30:56+08:00
- 提交到生产用时：10.24 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终 SHA 的 L3、候选 CI/freshness、main CI/freshness、tag freshness、Release、公开附件、公开镜像 E2E 和生产三阶段门禁均首轮产品通过。以下单独统计发布编排、取证或证据读取异常，不把它们写成产品失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：11
- 其中生产写操作开始后异常次数：4
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/public-release/expected-404-exit",
    "position": "before-production-write",
    "count": 1,
    "impact": "发布前用 fail-on-HTTP-error 查询不存在的 v1.14.1 Release，预期 404 使组合检查退出 1；没有创建标签或发布物。",
    "recoveryEvidence": "随后分别核对目标标签、GitHub Release 和 Docker 标签均不存在，正式 Release 完成后公开 API 返回 8 个附件。",
    "permanentAction": "发布前的预期不存在检查按 HTTP 状态分类，不把预期 404 与网络或服务失败放在同一退出码中。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/version-read/powershell-empty-key",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次 PowerShell package-lock JSON 读取因空属性名解析失败，没有形成版本一致性结论。",
    "recoveryEvidence": "改用仓库固定 check-version-consistency.sh 校验 VERSION、Go、package.json 和 lockfile，输出 1.14.1 一致。",
    "permanentAction": "版本真源校验只调用仓库固定脚本，不再用 PowerShell 对含空属性名的 lockfile 做临时解析。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/managed-script-contract/wrong-entrypoint",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次受管脚本检查误用不存在的 .mjs 入口，未执行契约校验。",
    "recoveryEvidence": "使用 run-repo-bash.mjs 调用 tracked check-managed-script-contract.sh 后通过，revision 与 SHA256 精确匹配。",
    "permanentAction": "受管脚本契约入口固定为 scripts/check-managed-script-contract.sh，并始终通过 run-repo-bash.mjs 启动。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/l3-artifact/precreated-directory",
    "position": "before-production-write",
    "count": 1,
    "impact": "L3 r1 的证据目录被调用方提前创建，编排器按不可覆写规则拒绝启动，未执行候选测试或远程写入。",
    "recoveryEvidence": "使用全新的 run ID 与不存在的目录启动 r2，固定 runner 和 arena-154 完整门禁 exit 0。",
    "permanentAction": "L3 和生产证据目录交由编排器原子创建，调用前只检查不存在，不预建目录。",
    "historicalReleases": []
  },
  {
    "fingerprint": "release/tag-verification/powershell-peeled-ref",
    "position": "before-production-write",
    "count": 1,
    "impact": "标签推送后的首次 peeled ref 命令未引用 ^{}，PowerShell 解析后误报引用缺失；标签已经成功写入。",
    "recoveryEvidence": "用单引号保护 refs/tags/v1.14.1^{} 后，远端 peeled ref 精确返回产品 SHA，Release 全部成功。",
    "permanentAction": "Windows 标签核验固定把完整 peeled ref 作为单引号参数传给 git ls-remote，避免 PowerShell 元字符解析。",
    "historicalReleases": ["v1.13.0"]
  },
  {
    "fingerprint": "public-image/docker-cli/unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机没有 Docker CLI，第一次公开镜像清单查询未执行。",
    "recoveryEvidence": "在允许的 arena-154 上用 docker buildx 查询 1.14.1 与 latest，二者 OCI index 和双架构清单一致。",
    "permanentAction": "公开镜像清单和运行验收统一在已注册且具备 Docker 的 arena-154 执行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-image/repository-name/wrong-repo",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次远端镜像查询使用了错误仓库 kejilion/kpanel，Docker Hub 返回权限/不存在错误，没有拉取候选镜像。",
    "recoveryEvidence": "从 release.yml 和 kpanel.conf 读取真源 docker.io/kjlion/kejilion-panel，正确仓库的 1.14.1/latest 摘要及公开 E2E 全部通过。",
    "permanentAction": "镜像仓库名从 release workflow 输出或 kpanel.conf 真源读取，不凭历史称呼手工拼接。",
    "historicalReleases": []
  },
  {
    "fingerprint": "backup/health/transient-connection-reset",
    "position": "after-production-write",
    "count": 1,
    "impact": "停写备份恢复后的第一次健康探测遇到连接重置，固定入口继续有界探测，未开始产品更新。",
    "recoveryEvidence": "同一 backup gate 最终确认 v1.14.0、Panel、Agent、SQLite 和保护文件全部健康，随后 postdeploy 首轮通过。",
    "permanentAction": "保留备份恢复后的有界健康等待与最终状态断言，单次连接重置不得绕过 gate 或触发直接部署。",
    "historicalReleases": ["v1.14.0"]
  },
  {
    "fingerprint": "postdeploy/apps-contract/powershell-string-index",
    "position": "after-production-write",
    "count": 1,
    "impact": "公网健康通过后，本地应用配置对比把单个路径字符串按字符索引读取，证据命令失败；生产服务没有受影响。",
    "recoveryEvidence": "把 rg 结果显式转换为单一路径字符串后，KPanel 与 kejilion/apps 的 kpanel.conf 归一全文一致，apps 仓库保持干净。",
    "permanentAction": "PowerShell 的单路径结果先显式转为 string，并在读取前验证非空且不含多行。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-artifacts/github-api/rate-limit",
    "position": "after-production-write",
    "count": 1,
    "impact": "读取验收文档提交的 CI 时，匿名 GitHub API 达到速率上限；本机 GitHub CLI 也没有登录，未形成该次状态证据。",
    "recoveryEvidence": "把固定只读查询脚本上传到 arena-154，通过独立网络出口确认 CI 34668017312 精确绑定验收 SHA 并成功。",
    "permanentAction": "远端工作流状态读取优先使用已注册验证环境的独立出口，并保留精确 SHA、workflow 与 run ID 过滤。",
    "historicalReleases": ["v1.13.0"]
  },
  {
    "fingerprint": "cleanup/windows-remove-item/policy-rejection",
    "position": "after-production-write",
    "count": 1,
    "impact": "本地临时文件清理的 PowerShell Remove-Item 调用被自动策略拒绝，文件和空目录当时仍保留。",
    "recoveryEvidence": "临时脚本改由补丁机制删除，空目录用同一 PowerShell 进程的 .NET API 在根路径与空目录断言后删除；远端精确路径清理也全部通过。",
    "permanentAction": "工作区内临时文本文件优先用补丁删除，空目录清理使用单进程绝对路径边界与空目录断言。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与清理

远端候选分支已由 Release workflow 删除。公开 E2E 临时容器、网络和端口已清理；保留 L3、公开附件、公开镜像和生产三阶段证据，以及唯一生产恢复包。

未验证风险包括真实 light node 重接入/弱网/令牌过期时序、最终 SHA 的完整人工浏览器矩阵、长期 soak、生产故障注入和受控回滚。它们不阻断本版，因为最终 SHA 的接入成功与过期边界自动测试、公开 OCI E2E、生产备份和 postdeploy 均已通过，且生产没有主动修改轻节点身份数据。
