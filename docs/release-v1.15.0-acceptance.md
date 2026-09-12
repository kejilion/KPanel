# KPanel v1.15.0 发布验收

## 发布范围

本次发布把 v1.14.1 之后完成的 5 个候选提交发布为 v1.15.0，产品提交固定为 `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`：

- `9a088c4`：改善轻节点文件中继吞吐、断线恢复和慢连接清理。
- `e2afbc3`：中心、面板节点和轻量节点统一使用带认证的流式文件传输。
- `c8d5b9b`：新建网站支持 `域名:端口` 与 `IPv4:端口` HTTP 地址，并按真实监听生成访问地址。
- `b4211c0`：新增加密、可选择模块、可预览且带回滚的备份与恢复中心。
- `5bbbb50`：固定 1.15.0 版本、发布说明和配套脚本来源。

备份中心支持 Panel、应用、网站和 Docker 模块的全部或选择性 `.kpb` 导出；导入先完成密码、认证标签、结尾、摘要、容量和依赖检查，预览后再明确恢复。AI 只迁移 API 接入配置，目标端既有会话、消息和附件保留。Panel、Agent 与 `kejilion.sh` 三个业务菜单共用同一适配器和加密包格式，恢复过程保留持久任务日志、中断恢复和失败回滚。

`scriptLinkageState=coupled`。配套组合脚本已先发布到 `kejilion/sh@5ef0201947dfb80062d54a0ba8f11009e871cf04`；该提交同时包含 HTTP 自定义端口适配与备份中心入口。根脚本 SHA256 为 `4adc9e163a6db31a180e3a16489dcec3bf1f1a48a95253a140aaf11100eae048`，CN 脚本 SHA256 为 `62b01b5b1ba736fafe1a167733d64eaba64606f9c5a4b77fa1cd136adbf843cc`，公开 raw 内容与 Git blob 一致。

无数据库 schema、Compose、现有站点、应用、节点身份或配对密钥迁移。已有站点保持原监听；只有新建时显式填写端口才启用 HTTP 自定义端口。集群恢复会恢复实际身份与配对密钥；同域名替换实例仍要求协议和端口不变、密钥完整且旧实例停止。

候选分支 `release/v1.15.0-candidate`、远端 `main` 和标签 `v1.15.0` 都曾精确绑定产品 SHA。Release workflow 成功后自动删除候选远端分支；标签保持不可变，后续验收文档提交不是产品 revision。

## 自动门禁

最终候选 L3 run `v1.15.0-5bbbb50-l3-r1` 于 2026-09-13T01:00:41+08:00 至 01:15:12+08:00 完成，exit 0。固定 Runner 为 `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，plan SHA256 `5549fb91ffb6cc0372d2fdaf037fa0f22a6a02eb47e61c90eb0f2539dd8d756d`，remote entry SHA256 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，bundle SHA256 `4b68f03ec0726da4d5f4e9b578cdfba57e67bbc4abb68dbbd3ab4db1dd7ace1f`。完整本地输入与远端证据为 `C:/GitHub/_release-artifacts/v1150-5bbbb50-l3-r1` 和 `C:/GitHub/_release-artifacts/v1150-5bbbb50-l3-r1-remote`，远端证据 11 个受校验文件均匹配。

L3 覆盖 164 项治理/编排测试、Go 全量与核心 race、155 个前端文件/1359 项测试、2294 条 i18n、typecheck、production build、双架构二进制、安装安全、app-conf 生命周期、受管脚本契约和最终镜像。`govulncheck`、npm audit、Trivy 源码/配置与最终镜像扫描均无命中。合并后的备份关闭顺序与健康流连接回归连续运行 3 次通过；备份功能另有 13 个隔离真实 Docker 场景和三语言、明暗主题、桌面/移动视口的浏览器 mock 验证。

候选 dependency freshness [34707686471](https://github.com/kejilion/KPanel/actions/runs/34707686471) 与 CI [34707686484](https://github.com/kejilion/KPanel/actions/runs/34707686484) 成功；main dependency freshness [34708044529](https://github.com/kejilion/KPanel/actions/runs/34708044529) 与 CI [34708044552](https://github.com/kejilion/KPanel/actions/runs/34708044552) 成功；tag dependency freshness [34708413971](https://github.com/kejilion/KPanel/actions/runs/34708413971) 与 Release [34708413924](https://github.com/kejilion/KPanel/actions/runs/34708413924) 成功，全部绑定 `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`。

## 依赖与应用市场契约

dependency freshness 覆盖 policy 要求的依赖组。业务上下文基线为 v1.12.0 / `0ff1e32`，候选距基线 27 个提交，未放宽 50 提交阈值。新增 `coder/websocket` 依赖已纳入许可证与第三方声明；Go 1.26.7、Node 24.20.0、基础镜像、Action 和 Trivy 0.72 固定版本未变化。

`packaging/kejilion-app/kpanel.conf` 相对 v1.14.1 未变，并与干净的 `kejilion/apps` main `2d8044adec98e3eb16f47cdbb297f6be9632a66f` 归一全文一致，因此没有创建 apps 空提交。

## 公开制品验收

[v1.15.0 Release](https://github.com/kejilion/KPanel/releases/tag/v1.15.0) 于 2026-09-13T01:40:21+08:00 公开，为 Latest、非 draft、非 prerelease。8 个附件齐全：两种架构的 Agent、两种架构的 Node、`kejilion-panel-deploy-1.15.0.tar.gz`、LICENSE、SHA256SUMS 和 THIRD_PARTY_NOTICES.md。5 个受 SHA256SUMS 管理的文件均已下载并逐项匹配，证据为 `C:/GitHub/_release-artifacts/v1150-5bbbb50-release-assets-r1`。

Docker Hub `1.15.0` 与 `latest` 同指 OCI index `sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9`。linux/amd64 manifest 为 `sha256:8816eb460f5126c90029a84dd53c67d9884d70ddb3a570f1e0e63c4de6cd8ecb`，linux/arm64 为 `sha256:6949564e6aec8f8794408bf5adcc6adcbf255e96da694755524e263b7146e616`；两个 unknown/unknown 条目为 attestations。

公开镜像在 arena-154 的临时端口 18115 按固定 `packaging/tests/image-e2e.sh` 运行，验证 version `1.15.0`、revision `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`、User `65532:65532`、受管脚本 SHA256、健康、首页、代理头、真实静态资源、bootstrap、Secure cookie、单网络和最终 container health，输出 `image_e2e=pass`。临时容器、网络和端口已清理；证据为 `C:/GitHub/_release-artifacts/v1.15.0-public-5bbbb50-r1`。

## 生产部署安全核对

用户明确授权本次完整上线。验证和正式生产目标均为 `arena-154` / `154.36.153.9`；`prod-108` / `108` 禁止全部 KPanel 操作，本轮没有连接或修改。

生产 preflight run `v1.15.0-production-20260913` 于 01:43:48+08:00 至 01:43:50+08:00 确认原生产 v1.14.1、revision `50f6602d831c812daf35446127bd5c5341586c7b`、旧 OCI `sha256:893f0fe328c61017d344debb3d9abefe70139c82373bf12eafc04fd373701f6a` 健康。Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite quick check 通过，63 行数据清单正常。

备份阶段于 01:44:12+08:00 至 01:44:21+08:00 使用固定入口停写并创建 `/root/kpanel-backups/pre-v1.15.0-20260912T174412Z`，包含数据/配置、旧镜像、service、inspect 和 SHA256SUMS。逐项校验后恢复 v1.14.1 健康，保护文件与 preflight 一致。恢复健康探测发生一次连接重置，固定入口在同一阶段内继续有界探测并最终通过，之后才进入产品部署。

生产只执行标准应用市场入口：

```text
env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel
```

命令退出 0，拉取 `latest@sha256:80a9013765c56c4bd0efb1b0c19fb76a73a6467f318a70496b070c39c3cd40e9` 并完成 Panel、Agent 和受管脚本事务更新。postdeploy 于 01:45:08+08:00 至 01:45:10+08:00 通过：version `1.15.0`、revision `5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92`、OCI digest、脚本 revision `5ef0201947dfb80062d54a0ba8f11009e871cf04` 与脚本 SHA256 精确一致；Panel running/healthy、restart 0、OOM false；Agent active/running/enabled、NeedDaemonReload=no；SQLite `panel/ai.db=ok`，保护文件 diff 为空，63 行数据清单保持一致，变化仅为运行中的监控 JSONL 文件继续增长。部署后资源约 74.87 MiB / 256 MiB、CPU 0.02%、PIDs 7。

公网 `https://kpanel.154.36.153.9.sslip.io/api/v1/health` 返回 HTTP 200、`status=ok`、`initialized=true`、`version=1.15.0`。生产证据为 `C:/GitHub/_release-artifacts/v1150-production-preflight-r1`、`C:/GitHub/_release-artifacts/v1150-production-backup-r1` 与 `C:/GitHub/_release-artifacts/v1150-production-postdeploy-r1`。

生产没有导入或恢复真实 `.kpb`、切换 DNS、替换集群身份、修改现有站点端口、执行故障注入或回滚演练。本轮没有生产退化、数据迁移、回滚、紧急热修复或重复正式发布。

## 回滚

- 源码/tag：v1.14.1 / `50f6602d831c812daf35446127bd5c5341586c7b`；v1.15.0 tag 保持不可变。
- 镜像：`docker.io/kjlion/kejilion-panel@sha256:893f0fe328c61017d344debb3d9abefe70139c82373bf12eafc04fd373701f6a`。
- 脚本：`kejilion/sh@b776ae85850bd50c86b2902a904361d7a0794e39`。
- 数据/配置：`/root/kpanel-backups/pre-v1.15.0-20260912T174412Z`，含旧镜像和已校验 SHA256SUMS。
- 若需回滚，等待备份/恢复任务结束，停止 Agent/Panel，恢复备份数据、配置、service、旧镜像和脚本 pin，再启动并重复 health、Agent、OCI、SQLite、保护文件和日志检查；本轮未实际触发。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-12T21:41:09+08:00
- 候选冻结时间：2026-09-13T01:15:33+08:00
- 生产完成时间：2026-09-13T01:45:10+08:00
- 提交到生产用时：4.07 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

最终 SHA 的 L3、候选 CI/freshness、main CI/freshness、tag freshness、Release、公开附件、公开镜像 E2E 和生产三阶段门禁均首轮产品通过。以下单独统计发布编排、取证或证据读取异常，不把它们写成产品失败。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：13
- 其中生产写操作开始后异常次数：5
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/script-source/windows-line-endings",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows git archive 应用 checkout 换行过滤后，脚本测试把 CRLF 产物误判为源文件失败；没有发布错误内容。",
    "recoveryEvidence": "直接校验 Git blob 为 LF，并在 WSL 对原始 blob 运行 bash -n、根/CN 一致性及 smoke 测试全部通过。",
    "permanentAction": "脚本发布校验固定读取 Git blob 或禁用归档 checkout 过滤，不用 Windows 工作树归档代表远端字节。",
    "historicalReleases": ["v1.14.0", "v1.13.0"]
  },
  {
    "fingerprint": "preflight/script-git/ssh-identity-unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次按显式 SSH URL 推送 kejilion/sh 候选时，本机没有该仓库可用的 SSH identity，远端没有变化。",
    "recoveryEvidence": "改用仓库已经配置并可认证的 HTTPS origin 后，候选分支和 main 精确推送到 5ef0201947dfb80062d54a0ba8f11009e871cf04。",
    "permanentAction": "发布前从目标仓库现有 origin 和认证探测确定传输方式，不为相邻仓库推断 SSH 身份可复用。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/script-git/https-fetch-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "kejilion/sh main 首次 HTTPS 推送连接 GitHub 443 超时，API 确认远端 main 仍为 b776ae8。",
    "recoveryEvidence": "在确认远端未变化后重试，main 精确更新到 5ef0201947dfb80062d54a0ba8f11009e871cf04，公开 raw 内容和 Git blob 哈希一致。",
    "permanentAction": "保留超时后的远端精确 SHA 核对，只在确认未写入或幂等时重试同一提交。",
    "historicalReleases": ["v1.13.0"]
  },
  {
    "fingerprint": "preflight/orchestrator-cli/unsupported-help",
    "position": "before-production-write",
    "count": 1,
    "impact": "用 --help 查询生产证据编排器时，该固定入口按未知参数退出 2，没有创建证据目录或连接生产。",
    "recoveryEvidence": "读取 tracked 入口源码及 v1.14.1 三阶段 plan 后，按正式参数执行 preflight、backup 和 postdeploy 均通过。",
    "permanentAction": "编排器参数从仓库文档或源码 usage 读取，不再对不声明 help 支持的入口调用 --help。",
    "historicalReleases": ["v1.14.0", "v1.13.0", "v1.12.0"]
  },
  {
    "fingerprint": "release/tag-fetch/local-clobber-conflict",
    "position": "before-production-write",
    "count": 1,
    "impact": "标签发布前执行全量 fetch --tags 时，本地历史 v0.86.2 与远端同名标签不同，Git 拒绝覆盖该旧本地标签；v1.15.0 当时尚不存在。",
    "recoveryEvidence": "HEAD 与 origin/main 均精确为产品 SHA，目标 v1.15.0 不存在；随后只创建并推送目标标签，远端 peeled ref 精确匹配产品 SHA。",
    "permanentAction": "发布前只 fetch main 和目标标签所需引用，不执行会受无关历史本地标签影响的全量 tag 更新。",
    "historicalReleases": ["v1.14.0"]
  },
  {
    "fingerprint": "release/tag-verification/powershell-peeled-ref",
    "position": "before-production-write",
    "count": 1,
    "impact": "首次同时读取标签对象与 peeled ref 时，未引用的 ^{} 被 PowerShell 解析，结果只显示 annotated tag 对象。",
    "recoveryEvidence": "用单引号保护 refs/tags/v1.15.0^{} 后，远端 peeled ref 精确返回 5bbbb50db1f5e958cc27bfc6f457478c4f5b0c92。",
    "permanentAction": "Windows 标签核验固定把完整 peeled ref 作为单引号参数传给 git ls-remote。",
    "historicalReleases": ["v1.14.1", "v1.13.0"]
  },
  {
    "fingerprint": "public-image/docker-cli/unavailable",
    "position": "before-production-write",
    "count": 1,
    "impact": "本机没有 Docker CLI，第一次 imagetools 查询未执行。",
    "recoveryEvidence": "使用 Docker Hub Registry API 核对 1.15.0/latest 摘要与双架构清单，并在允许的 arena-154 完成固定公开镜像 E2E。",
    "permanentAction": "公开镜像清单优先使用 Registry API，运行验收固定在已注册且具备 Docker 的 arena-154 执行。",
    "historicalReleases": ["v1.14.1"]
  },
  {
    "fingerprint": "public-artifacts/oci-parser/powershell-pipeline-syntax",
    "position": "before-production-write",
    "count": 1,
    "impact": "第一次 Registry API 解析脚本把对象构造直接接到管道，PowerShell 报空管道元素，没有形成镜像平台结论。",
    "recoveryEvidence": "先累积对象再 ConvertTo-Json，确认 1.15.0 与 latest 同摘要，并列出 amd64、arm64 和 attestations。",
    "permanentAction": "Registry 查询使用固定 PowerShell 对象数组模板，避免在 foreach 语句块尾部直接拼接管道。",
    "historicalReleases": []
  },
  {
    "fingerprint": "backup/health/transient-connection-reset",
    "position": "after-production-write",
    "count": 1,
    "impact": "停写备份恢复后的第一次健康探测遇到连接重置，固定入口继续有界探测，未开始产品更新。",
    "recoveryEvidence": "同一 backup gate 最终确认 v1.14.1、Panel、Agent、SQLite 和保护文件全部健康，随后 postdeploy 首轮通过。",
    "permanentAction": "保留备份恢复后的有界健康等待与最终状态断言，单次连接重置不得绕过 gate 或触发直接部署。",
    "historicalReleases": ["v1.14.1", "v1.14.0"]
  },
  {
    "fingerprint": "postrelease/l3-evidence/absolute-path-checksum",
    "position": "after-production-write",
    "count": 1,
    "impact": "L3 远端证据复制到 Windows 后直接运行 sha256sum -c，清单中的远端绝对路径不可读取，首次本地复核失败；远端 L3 与生产均未受影响。",
    "recoveryEvidence": "按清单路径 basename 映射到只读复制目录后，11 个文件 SHA256 全部匹配。",
    "permanentAction": "远端绝对路径证据清单回收后使用固定前缀映射验证，同时保留原清单字节不修改。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postrelease/script-git/https-read-timeout",
    "position": "after-production-write",
    "count": 1,
    "impact": "终态只读核对 kejilion/sh main 时，GitHub HTTPS 443 再次超时；没有远端写入。",
    "recoveryEvidence": "脚本发布阶段已经确认远端 main、公开 raw 与 Git blob；终态本地候选 HEAD 和 origin/main 也都精确为 5ef0201947dfb80062d54a0ba8f11009e871cf04。",
    "permanentAction": "脚本终态优先复用发布阶段保存的远端精确 SHA 证据，额外网络复核失败时只按幂等只读查询重试或使用独立出口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "postrelease/script-evidence/cn-path-case",
    "position": "after-production-write",
    "count": 2,
    "impact": "终态脚本复核先后使用了错误的 CN/kejilion.sh 和 CN/kejilion_CN.sh 路径，分别得到 404 和不存在结果，没有形成 CN 哈希结论。",
    "recoveryEvidence": "从目标提交树读取真实路径 cn/kejilion.sh，Git blob SHA256 为 62b01b5b1ba736fafe1a167733d64eaba64606f9c5a4b77fa1cd136adbf843cc，与发布证据一致。",
    "permanentAction": "多语言脚本路径从目标提交 git ls-tree 输出选择，保持仓库真实大小写，不凭历史目录命名拼接。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与清理

远端候选分支已由 Release workflow 删除。公开 E2E 临时容器、网络和端口已清理；保留 L3、公开附件、公开镜像和生产三阶段证据，以及唯一生产恢复包。

未验证边界包括跨发行版真实 LDNMP/数据库迁移、真实 DNS 与外部集群对端切换、原生 arm64 运行时、200% 缩放、长期 soak、生产故障注入和 systemd 沙箱画像。rootless/userns-remap、远程或带选项卷、符号链接、特殊文件、独立挂载点按设计由预检拒绝；SELinux 标签、ACL 和 xattrs 尚未适配。这些边界不阻断本版，因为最终 SHA 的自动门禁、隔离 Docker 回归、公开 OCI E2E、生产备份和 postdeploy 均已通过，且生产没有执行真实恢复或基础设施迁移。
