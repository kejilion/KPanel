# KPanel v0.100.0 发布验收记录

日期：2026-09-02

发布级别：L3

候选提交 / 标签：`89a384c7d65c42b14222dcace8843ff23602dc11` / `v0.100.0`

上一稳定版本 / 回滚点：`v0.99.6` / `06e43b7f572245165f7ed71e929f9ce1ceed7916`，生产回滚镜像为 `sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`

> 本记录为补记：`v0.100.0` 已于 2026-09-01 完成 L3、公开发布和 `arena-154` 生产部署，但当时未同步生成验收记录。
> 本文全部结论均从 `_release-artifacts` 与 `arena-154` 上该版本的原始证据重建，未回填或改写其他历史记录；
> 当时未执行的场景按实际状态标注，不追认为已验证。

## 发布画像

- 业务域：集群通知渠道、网站 TLS 证书材料、系统中心防火墙、公开分享页视觉、三级页面运行时多语言、构建供应链依赖。
- 变更面：展示 / 只读 / 宿主机写入 / 部署；不新增数据库迁移，不改变 Panel/Agent 协议版本与端口契约。
- 受影响用户旅程：在集群页配置并接收 Telegram 通知；新建或管理网站时提交自定义 TLS 证书材料并复用有效证书；在系统中心使用简化防火墙界面执行国家/地区允许、阻止和清除；访问文件分享与集群分享页；在三级页面、弹窗和门户操作中切换 `en-US`、`zh-CN`、`zh-TW`。
- 未变化契约：Panel/Agent API 协议版本 `v1alpha1`、数据库、端口、Compose、Agent 权限边界与应用市场安装契约均未变化；`packaging/kejilion-app/kpanel.conf` 相对 `v0.99.6` 无差异。受管 `kejilion.sh` 固定提交由 `d58079304a92936bf8e3d90467eea484c5b63d6f` 升级到 `6e65c0cd7028cb198efb0c88a57726713ee1b23b`，以承载 TLS 材料与国家防火墙协议，仍走同一受管脚本调用协议。
- 风险等级及理由：中高风险。含新增宿主机写入路径（TLS 证书材料、`iptables`/`ipset` 国家规则）、新增外发网络渠道（Telegram）、受管脚本升级和一个 CRITICAL 依赖安全修复；但无数据迁移与协议破坏，候选/主线/Release CI、L3 与生产 backup/postdeploy 均通过。

## 发布范围与未纳入内容

- 用户可见更新：
  1. 集群通知新增 Telegram 渠道与通知配置管理，沿用既有节点能力协商、签名和后台任务边界。
  2. 网站新建与管理支持自定义 TLS 证书材料并复用有效证书；证书格式、密钥匹配和来源由 Panel 侧校验与受管 `kejilion.sh` 共同约束。
  3. 系统中心防火墙界面简化，新增受控的国家/地区入站允许、阻止和清除规则。
  4. 文件分享与集群分享页统一使用新版彩色 KPanel Logo。
  5. 补齐三级页面、弹窗和门户操作的英文运行时多语言资源及回归检查。
- 精确提交清单：基线 `06e43b7f572245165f7ed71e929f9ce1ceed7916`（`v0.99.6`）；纳入 `c4b6d59914491a61c1c9505ef22ff2936da841a9`（三级英文弹窗 i18n）、`0083be4e56fdc3f97037bbe05949bb7d3ce90208`（门户弹窗 i18n）、`72cdd6d5f2127cfc94e568c0e3aaf8e07eab62ef`（分享页 Logo）、`a1c0a0ad3ee90d47d0cb0096c48e7a5a0e06b803` 与 `07ef85a1d010342e12ec01449104305e0bbd424b`（集群 Telegram 通知与渠道选择）、`59c038075866f398f087af65b5849762f5726a41` 与 `ea1b33925eac4b93a27f6b4b4582be5c3c19f6ab`（防火墙简化与国家控制）、`5d9ba94aff47d081db2a6adedc90c7c07a5e6885`（防火墙候选锁定受管脚本）、`72433c3d766007603a7060560b1c7fee5c3bc405`（自定义 TLS 证书）、`30ab1d85156d74b39c16afde7e09733f5ed029a5`（固定合并版受管脚本）、`234eb4921eb35632a0208d241073606487f83c17`、`7cc1533bb86d30a97df6a5764d688eb05132822b`、`456587385ebf22c05dd4401a65841a67a827958a`（版本准备）、`ff072823cba50d4def83e39ada7c01b76e9d82f4`（清理失效防火墙翻译键）、`ca9f96266ec26e111224f7b3316ff7c03a333bc1`（`golang.org/x/crypto` 安全升级）、`144a12b0e7bb8352b862a34ce155f67c71e4e0c5`（修正受管脚本校验和）和 `89a384c7d65c42b14222dcace8843ff23602dc11`（业务上下文刷新）。候选相对 `v0.99.6` 为 109 个文件、9009 行新增、1101 行删除。
- 明确未纳入的分支、文件或后续事项：未纳入其余候选与集成分支的功能；没有复用旧版本 L3、公开镜像或生产证据；没有连接、备份、部署、升级或核对 `108`/`prod-108`。本版未执行真实浏览器人工视觉验收、长期 soak、生产故障注入和受控回滚演练。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | L3 `v0.100.0-89a384c-l3-r1` 的 `make verify-release` 通过：Go 全量 28 包测试 `ok`、前端 128 文件 / 1075 测试通过、`app_conf_lifecycle=pass`、`managed_script_contract=pass`；生产 postdeploy health `status=ok`、`version=0.100.0`、`initialized=true`、`protocolVersion=v1alpha1`。 | 新增 TLS 证书材料与国家防火墙规则的真实外部证书链和真实跨境流量效果未在生产实测；Telegram 真实投递未在生产触发。 |
| 网络入侵与供应链安全 | 已验证 | `ff07282-l3-r2` 的 Trivy 门禁检出 `golang.org/x/crypto v0.54.0` 的 CVE-2026-56854（CRITICAL，SSH 源地址限制未强制导致认证绕过）并 fail-closed 拦截；升级到 `v0.55.0` 后 `89a384c-l3-r1` 源码/依赖/镜像扫描全部通过。受管脚本三处摘要一致：Dockerfile 固定值、镜像 label `io.kejilion.script.sha256` 与远端 `kejilion/sh@6e65c0cd` 实测 SHA-256 均为 `48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。Release workflow `#33537862190` 成功，含 SBOM/provenance 证明。 | 新增 Telegram 出站渠道与自定义私钥入口的对抗性渗透测试未执行；两者分别受 Panel 侧格式/配对校验和受管脚本协议约束。 |
| 稳定性、失败恢复与兼容 | 已验证 | L3 核心 race 检测 `internal/panel`、`internal/auth`、`internal/dockerx` 全部 `ok`；双架构构建与镜像契约通过。生产 postdeploy：容器 `running`、`health=healthy`、`RestartCount=0`、`OOMKilled=false`，Agent `LoadState=loaded`/`ActiveState=active`/`SubState=running`/`UnitFileState=enabled`/`NeedDaemonReload=no`，`panel.log` 无 panic/fatal/OOM 签名。 | 未执行长期 soak、生产故障注入和实际回滚演练；`arena-154` 根分区在 postdeploy 时为 97% 使用率、可用 2.9G（已于 2026-09-02 完成深度清理，详见遗留风险）。 |
| 性能与资源预算 | 已验证 | 生产 postdeploy 资源快照：Panel `73.89MiB / 256MiB`（`MemPerc=28.86%`）、`CPUPerc=0.03%`、`PIDs=7`、`NetIO=4.45kB / 1.39kB`、`BlockIO=0B / 32.8kB`；preflight 基线为 `20.62MiB / 256MiB`、8 PIDs。前端构建产物按路由懒加载分块。 | 未执行长期压力或 soak；`GOMEMLIMIT` 未针对 256MiB 容器上限显式设置，当前余量充足且未发生 OOM，列入后续准入。 |
| 用户体验与可访问性 | 已实现未实机验证 | 精确 SHA 的前端 typecheck、128 文件 / 1075 测试、构建以及新增 `remaining-portal-i18n`、`third-level-i18n` 回归在 L3 通过；防火墙、集群通知、站点 TLS 表单的组件级测试随全量前端测试通过。 | 本轮未执行真实浏览器的浅/深色主题、键盘/焦点、100%/125%/200% 缩放、最小计算字号和人工三语巡检；未把旧版本 preview 或其他版本证据当作本版证据。 |
| 数据、配置与迁移 | 已验证 | `packaging/kejilion-app/kpanel.conf` 相对 `v0.99.6` 无差异，且与 `C:\GitHub\kejilion\apps\kpanel.conf` 忽略行尾空白后内容一致。生产 backup 与 postdeploy 两阶段 `protected.diff` 均为 0 字节；SQLite 检查 `ai.db empty` / `panel/ai.db ok`；备份归档 `SHA256SUMS` 六文件校验通过。 | 不适用数据库迁移；通知配置与证书材料通过既有任务与脚本协议写入，不引入新的持久化模式。 |

## 自动门禁

- 定向测试及结果：L3 中前端 typecheck、前端 128 文件 / 1075 测试、前端构建、Go 全量 28 包测试、核心 race（`internal/panel` 16.003s、`internal/auth` 4.017s、`internal/dockerx` 2.342s）、`go vet`、双架构构建和应用配置生命周期均通过。
- `make verify-release` 环境和结果：目标仅 `arena-154`。L3 run=`v0.100.0-89a384c-l3-r1`，candidate=`89a384c7d65c42b14222dcace8843ff23602dc11`，base main=`06e43b7f572245165f7ed71e929f9ce1ceed7916`，base tag=`v0.99.6`，`verification_preflight=pass platform=Linux level=release tools=docker,go,gofmt,make,npm`，`release_gate_runner=pass`，`L3 release verification completed.`。
- L3 外层入口 run ID、计划/脚本/bundle SHA-256、不可变 Runner ID、终态与证据目录：run=`v0.100.0-89a384c-l3-r1`；Runner=`kpanel-release-gate:go1.26.7-node24`，不可变 Runner ID=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle SHA-256=`fbea45ed20b577849b6f3971b2d624335f86c09062a79f016aaa08230015d22b`，plan SHA-256=`ca453f75120ed1b661bb8fb211da0010a2beab7be1e513187897883bfdd90b3e`，远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`，远端 `l3-verify-release.log` SHA-256=`b3997127088851d003b1c30e9a3e0a2234c77b51b1738f17ed9323505ea475db`；`status.txt` 为 `passed`、exit code `0`，`started_at=2026-09-01T17:05:03Z`、`finished_at=2026-09-01T17:16:07Z`；远端证据目录=`/root/kpanel-release-evidence/v0.100.0-89a384c-l3-r1`，本地目录=`C:\GitHub\_release-artifacts\v0.100.0-89a384c-l3-r1`。
- 候选 CI：`CI #33535594405` 与 `CI #33536951375` 成功，`Dependency freshness #33536951314` 成功，均在 `release/v0.100.0-candidate`。
- 主线 CI：`origin/main` 快进到 `89a384c7d65c42b14222dcace8843ff23602dc11` 后，`CI #33537237438` 成功，`Dependency freshness #33537237739` 成功。
- Release workflow：`Release #33537862190` 成功，`Dependency freshness #33537862176` 成功；GitHub Release 已公开且非 draft、非 prerelease，`published_at=2026-09-01T17:35:05Z`。
- 安全扫描、镜像契约、SBOM/provenance：Release 的源代码、依赖、镜像扫描与非 root/healthcheck/label/运行时契约均成功；公开 OCI index 含 `linux/amd64`、`linux/arm64` 及两个 attestation manifest。L3 内 `managed_script_contract=pass revision=6e65c0cd7028cb198efb0c88a57726713ee1b23b sha256=48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。

## 依赖与技术栈变化

- `make dependency-report` 生成时间及检测源完整性：候选 `Dependency freshness #33536951314`、主线 `#33537237739`、tag `#33537862176` 均为同 SHA 成功；检测源完整。
- 最近每日安全通告审计、EOL 复核状态及证据：本版由 L3 Trivy 门禁主动检出并阻断 CVE-2026-56854，修复后复跑通过；未产生剩余阻断性 EOL 行动项。
- 直接/基座行动项、传递依赖归属信号及首次完整检测后的启动/决策/处置期限：唯一直接行动项为 `golang.org/x/crypto` 由 `v0.54.0` 升级到 `v0.55.0`，在本版发布前的同一发布窗口内完成决策与处置；基座 Go `1.26.7`、Node.js `24` 沿用 `v0.99.6` 已验证版本，未变更。
- 本版采用的依赖、工具链、基础镜像、Action、扫描器或受管脚本候选：`golang.org/x/crypto v0.55.0`；Go `1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468`；Runner=`kpanel-release-gate:go1.26.7-node24@sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；扫描器 Trivy `0.72.0`；受管 `kejilion.sh` 候选提交 `6e65c0cd7028cb198efb0c88a57726713ee1b23b`。
- 版本/锁文件/Action SHA/镜像 digest/脚本提交与摘要：产品版本 `0.100.0`（`VERSION` 与 `web/package.json` 一致）；公开 OCI index=`sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`；`linux/amd64`=`sha256:68486e5db1c3a210a9a0fdf5290087bd2da3f62adf0d9e674d214484ea53497a`；`linux/arm64`=`sha256:93f0acb824a78dae7865bdab0acceb1353b4a9fe179787314513484bc0a679de`；受管脚本 revision=`6e65c0cd7028cb198efb0c88a57726713ee1b23b`、SHA-256=`48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9`。
- 暂缓或拒绝候选、证据、负责人、复核日期和退出条件：`ff072823cba50d4def83e39ada7c01b76e9d82f4` 因携带含 CVE-2026-56854 的 `golang.org/x/crypto v0.54.0` 被 L3 拒绝进入生产，由 `ca9f96266ec26e111224f7b3316ff7c03a333bc1` 升级修复；Trivy 提示可升级到 `0.74.0`（当前 `0.72.0`），不阻断本版，列入后续基础设施维护。当前无其他剩余阻断性候选。
- 升级后的兼容、安全、构建、性能资源和回滚结论：公开 Release、双架构 OCI、生产 backup/更新/postdeploy 均通过；可回滚至 `v0.99.6` 或本次备份 `/root/kpanel-backups/pre-v0.100.0-20260901T173810Z`。

## 隔离真机与浏览器验收

- 主机/发行版/架构/运行时版本：`arena-154`（Linux/amd64）；L3 Runner 为 Go `1.26.7`、Node.js `24`；生产 Panel 运行于 Docker，Agent 运行于 systemd。
- 环境策略 ID 与允许用途：`environment-policy.json` 中 `arena-154`（role=hybrid）；本次使用 `candidate-validation`、`production-safety-check`、`production-deploy`。未使用 `prod-108`/`108`。
- 使用的精确候选或公开产物：源码候选 `89a384c7d65c42b14222dcace8843ff23602dc11`、tag `v0.100.0`（annotated tag object `57ced474c5ccd87e65910fe8acf2d34d205197e5`）、公开镜像 `docker.io/kjlion/kejilion-panel@sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`。
- 后台作业 ID、终态、退出码、超时、证据目录、命令规格路径及 SHA-256：L3 run=`v0.100.0-89a384c-l3-r1` / passed / exit 0；生产 preflight run=`v0.100.0-89a384c-prod-preflight-r2` / passed / exit 0、backup run=`v0.100.0-89a384c-prod-backup-r1` / passed / exit 0、postdeploy run=`v0.100.0-89a384c-prod-postdeploy-r1` / passed / exit 0；入口为 `scripts/run-release-l3.mjs` 与 `scripts/run-production-evidence.mjs`，生产远端脚本 SHA-256=`129d3fbab5cd4e5ad966a59f3e174c586a26578e5fac262d7af7bc9699012eaf`，三阶段 plan SHA-256 分别为 `cd201a8b182e1dd3ececd7027b0bc31398272f783d06a4bc5d00232079e1e510`、`7fa3a5b3c412ae31428a8778becaf9bd32a42b22a8b8403cacaf469274c8a4a1`、`0de9a69f2ed07ce58e3fc6ca1a25b43deb76ce76fe06b35389623336dfe5af7f`；未创建浏览器后台作业。
- 测试窗口/循环数及风险依据（无 soak 时写不适用依据）：未执行长期 soak。L3 单轮完整门禁窗口为 `2026-09-01T17:05:03Z` 至 `17:16:07Z`（约 11 分钟），生产三阶段窗口为 `17:37:45Z` 至 `17:39:19Z`；本版新增的通知与防火墙能力沿用既有后台任务边界，未新增常驻轮询任务，因此以单轮门禁加生产健康证据替代 soak，风险列入后续准入。
- 受影响用户旅程、视口、100%/125%/200% 缩放、最小计算字号、主题、键盘/焦点、语言和失败态：自动回归覆盖本版新增 i18n 键、弹窗与页面布局及失败状态；真实浏览器的视口、100%/125%/200% 缩放、最小计算字号、浅/深色主题、键盘/焦点和人工三语巡检本轮未执行，标记为已实现未实机验证。
- 宿主机写入、失败注入、重启恢复和回滚结果：backup 阶段归档旧容器 inspect、旧镜像、应用目录、Agent unit 和 `kpanel.conf` 并通过 `SHA256SUMS` 校验，备份路径为 `/root/kpanel-backups/pre-v0.100.0-20260901T173810Z`；更新后容器于 `2026-09-01T17:38:52Z` 重启并恢复 `healthy`，`RestartCount=0`。未执行故障注入或实际回滚。
- 未执行场景及原因：长期 soak、真实浏览器人工巡检、125%/200% 缩放、生产故障注入、受控回滚演练、真实外部 TLS 证书链导入和真实跨境流量的国家规则效果验证未执行；它们不替代已完成的 L3 与生产安全门禁，列入后续专项。

## 发布产物与公开仓库复核

- GitHub Release：[`v0.100.0`](https://github.com/kejilion/KPanel/releases/tag/v0.100.0) 已公开，`draft=false`、`prerelease=false`、`target_commitish=main`，`created_at=2026-09-01T17:27:14Z`、`published_at=2026-09-01T17:35:05Z`；annotated tag object=`57ced474c5ccd87e65910fe8acf2d34d205197e5`，peeled product commit=`89a384c7d65c42b14222dcace8843ff23602dc11`。
- Docker 版本与 `latest` OCI index：`docker.io/kjlion/kejilion-panel:0.100.0` 与 `:latest` 的 index digest 均为 `sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`，与生产运行镜像 ID 一致。
- `linux/amd64`、`linux/arm64` digest：amd64=`sha256:68486e5db1c3a210a9a0fdf5290087bd2da3f62adf0d9e674d214484ea53497a`；arm64=`sha256:93f0acb824a78dae7865bdab0acceb1353b4a9fe179787314513484bc0a679de`；另含两个 attestation manifest（SBOM/provenance）。
- 附件及 `SHA256SUMS`：Release 含 8 个附件——`kejilion-agent-linux-amd64`、`kejilion-agent-linux-arm64`、`kejilion-node-linux-amd64`、`kejilion-node-linux-arm64`、`kejilion-panel-deploy-0.100.0.tar.gz`、`SHA256SUMS`、`LICENSE`、`THIRD_PARTY_NOTICES.md`。
- 公开镜像 `image_e2e=pass`：未实现未实机验证。本版未单独执行 `packaging/tests/image-e2e.sh`；替代证据为生产通过受管标准入口拉取公开 `latest` 并运行同一 index digest，postdeploy health、容器 healthcheck、Agent 状态和数据完整性全部通过。缺口列入后续准入。
- `kejilion/apps` / `kejilion.sh` 契约结论：候选 `packaging/kejilion-app/kpanel.conf` 相对 `v0.99.6` 无差异；与 `C:\GitHub\kejilion\apps\kpanel.conf` 忽略行尾空白后内容一致，无需制造应用市场提交。受管 `kejilion.sh` 升级到 `6e65c0cd7028cb198efb0c88a57726713ee1b23b`，远端实测摘要与 Dockerfile 固定值、镜像 label 三处一致。

## 生产部署安全核对

- 生产目标和部署授权范围：用户已明确授权本次完整上线；正式写入仅执行 `arena-154`。
- 验证/灰度环境（必须来自 `environment-policy.json`，不得包含 `prod-108`）：`arena-154`，用途为 candidate-validation、production-safety-check、production-deploy。
- 正式部署环境（默认 `arena-154`；不得包含 `prod-108`）：`arena-154`。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前版本、健康、备份位置及摘要：preflight run=`v0.100.0-89a384c-prod-preflight-r2` / phase=`preflight` / passed，health `status=ok`、`version=0.99.6`、`initialized=true`，Panel `20.62MiB / 256MiB`、8 PIDs；backup run=`v0.100.0-89a384c-prod-backup-r1` / passed，备份=`/root/kpanel-backups/pre-v0.100.0-20260901T173810Z`，`backup-SHA256SUMS` 覆盖 `image-load-verify.txt`、`kejilion-agent.service`、`kpanel.conf`、`kpanel.tar.zst`、`old-image.tar.zst`、`panel-inspect.json` 六个文件，`protected.diff` 为 0 字节。
- 部署命令/入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。
- 部署后版本、Panel/Agent 状态、重启、日志、数据完整性和公网入口：postdeploy run=`v0.100.0-89a384c-prod-postdeploy-r1` / phase=`postdeploy` / passed，`expected_revision=89a384c7d65c42b14222dcace8843ff23602dc11`、`expected_image_digest=sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`、`baseline_run_id=v0.100.0-89a384c-prod-preflight-r2`；health `status=ok`、`version=0.100.0`、`protocolVersion=v1alpha1`；容器 `running`、`healthy`、`RestartCount=0`、`OOMKilled=false`，`StartedAt=2026-09-01T17:38:52Z`；镜像 label `org.opencontainers.image.revision=89a384c7d65c42b14222dcace8843ff23602dc11`、`version=0.100.0`、`io.kejilion.script.revision=6e65c0cd7028cb198efb0c88a57726713ee1b23b`；Agent `active/running/enabled`、`NeedDaemonReload=no`；`panel.log` 无 panic/fatal/OOM 签名；`protected.diff` 为 0 字节，SQLite `ai.db empty` / `panel/ai.db ok`；Panel `73.89MiB / 256MiB`、`CPUPerc=0.03%`、7 PIDs。
- 生产已执行写操作：通过标准应用市场 / `kejilion.sh` 入口更新；Docker 拉取公开 `latest`、重建 `kejilion-panel` 容器；写入生产证据和本次备份。首次生产写操作为 backup 阶段 `2026-09-01T17:38:10Z`。
- 仅在隔离真机执行、未在生产执行的场景：L3 门禁的候选构建、race 检测、Trivy 扫描和应用配置生命周期负例断言；前端人工视觉验收、故障注入、回滚演练和 `image-e2e.sh` 未执行。

## 回滚

- 源码/tag：回滚点 `v0.99.6` / `06e43b7f572245165f7ed71e929f9ce1ceed7916`。
- 镜像 digest：`docker.io/kjlion/kejilion-panel:0.99.6` 的已知稳定 index=`sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`；本次备份内 `old-image.tar.zst` 自包含该镜像，`image-load-verify.txt` 记录 `Loaded image ID: sha256:37607dc67a5b8b011d5335cda9f1dbfba8abf4d97ce9c3b475ced8fd7e78fb38`，不依赖宿主机镜像缓存。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.100.0-20260901T173810Z`，含旧容器 inspect、旧镜像归档、应用目录归档、Agent unit、`kpanel.conf` 和 `SHA256SUMS`；2026-09-02 磁盘清理后复验六文件 `sha256sum -c` 全部 OK。
- 回滚步骤和回滚后复核：恢复备份的 Compose、`.env`、数据、旧镜像和 Agent 文件，并成套恢复旧版受管 `kejilion.sh`（`d58079304a92936bf8e3d90467eea484c5b63d6f`），使用标准入口恢复 `v0.99.6`，再执行同一 `production-evidence` postdeploy 复核。本次未触发回滚。
- 回滚后生产实际版本与健康状态：未执行回滚，因此不适用；当前 `v0.100.0` postdeploy 已验证健康。
- GitHub Latest、Docker `latest` 与标准更新入口实际指向：GitHub Latest 为 `v0.100.0`；Docker `latest` 与 `0.100.0` 同 index digest `sha256:ae31b3f821b19393eef3fda0480b57c3fdf211b7363b06173fab58431cdaa601`；标准入口已在生产拉取并运行该 digest。
- 公共默认更新通道决策：保留 `v0.100.0` 为默认稳定通道；未发现需要恢复旧默认版本的产品问题。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-01T19:58:07+08:00
- 候选冻结时间：2026-09-02T01:04:11+08:00
- 生产完成时间：2026-09-02T01:39:19+08:00
- 提交到生产用时：5.69
- 是否回滚、紧急热修复或重复发布：否（两次发布执行缺陷与一次依赖 CVE 均在首次生产写操作前被门禁拦截并修复重跑，未造成生产变更失败）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "l3/dockerfile-managed-script/stale-checksum-pin",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 ca9f96266ec26e111224f7b3316ff7c03a333bc1 的 L3 在镜像构建阶段失败：Dockerfile 固定的受管脚本 SHA-256 与 kejilion/sh@6e65c0cd 当时的实际内容不一致，ADD --checksum 校验拒绝，门禁在执行候选业务验证和任何生产写入前中止。",
    "recoveryEvidence": "arena-154 的 v0.100.0-ca9f962-l3-r1 status.txt 为 failed/exit_code=2，日志记录 digest mismatch sha256:48f04709...: sha256:7f2a0d78...；提交 144a12b0e7bb8352b862a34ce155f67c71e4e0c5 修正校验和后，v0.100.0-89a384c-l3-r1 的 status.txt 为 passed/exit_code=0 且 managed_script_contract=pass sha256=48f04709eb369040b46886e12e77a862e145273a9fe97244794af692903358b9。",
    "permanentAction": "受管脚本固定值改为在候选冻结前从上游提交实测摘要取值，并由 scripts/check-managed-script-contract.sh 在 L3 内对 Dockerfile 固定值、镜像 label 和远端实际内容三处比对；后续发布必须先通过该一致性校验才能进入镜像构建。",
    "historicalReleases": []
  },
  {
    "fingerprint": "production-evidence/preflight-plan/expected-version-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "首轮生产 preflight 的 plan.env 把 EXPECTED_VERSION 误填为待部署的 0.100.0，而 preflight 阶段生产实际仍运行 0.99.6，取证入口 fail-closed 中止，未产生备份或部署写入。",
    "recoveryEvidence": "arena-154 的 v0.100.0-89a384c-prod-20260902/production-preflight/status.txt 为 failed/exit_code=1，同目录 snapshot/health.json 记录 version=0.99.6；改以 EXPECTED_VERSION=0.99.6 重跑后 v0.100.0-89a384c-prod-preflight-r2 的 status.txt 为 passed/exit_code=0。",
    "permanentAction": "生产取证的 preflight 阶段固定以部署前基线版本作为 EXPECTED_VERSION，postdeploy 才使用目标版本并附带 expectedRevision 与 expectedImageDigest；该阶段语义已由 scripts/run-production-evidence.mjs 的 baselineRunId 链路固定，后续发布按三阶段模板生成计划，不手工改写期望版本。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：真实浏览器人工验收（浅/深色主题、键盘/焦点、100%/125%/200% 缩放、最小计算字号、三语巡检）、长期 soak、生产故障注入和受控回滚演练未执行；本版新增的自定义 TLS 证书链导入与国家防火墙规则未用真实外部证书和真实跨境流量在生产实测；Telegram 通知未在生产触发真实投递；本版未单独执行 `packaging/tests/image-e2e.sh`；`GOMEMLIMIT` 未针对 256MiB 容器上限显式设置。
- 已实现待实机准入：集群 Telegram 通知与渠道管理、网站自定义 TLS 证书材料、防火墙简化与国家控制、分享页 Logo 和三级页面 i18n 均已由自动回归、L3 门禁和生产健康证据覆盖；真实浏览器视觉/交互证据、外部证书与跨境流量实测应在后续专项补齐。
- 不阻断本版的理由：候选 CI、主线 CI、Release workflow、公开双架构 OCI 与 attestation、L3 全量门禁、生产 preflight/backup/postdeploy 均以精确 SHA 和 immutable digest 通过；唯一 CRITICAL 依赖漏洞已在进入生产前被门禁拦截并修复；未验证项不涉及数据迁移或新增常驻后台任务，且回滚点已校验可用。
- 后续应进入的自动门禁或专项工作流：补充真实浏览器视觉/交互矩阵、长期 soak 和受控回滚演练；为本版新增的 TLS 证书与国家防火墙路径补真实外部材料的隔离真机验收；把公开镜像 `image-e2e` 恢复为每版必需门禁；评估为 Panel 容器显式设置 `GOMEMLIMIT`；升级 Trivy 到 `0.74.0`。`arena-154` 磁盘压力已于 2026-09-02 处置：根分区从 98%（可用 2.4G）降至 44%（可用 54G），清理范围为 Docker 构建缓存 20.15GB、v0.43–v0.89 历史发布工作目录与过期构建缓存 108 项、110 个陈旧 KPanel 备份（保留回滚点与最近 10 个）、19 个未使用 Docker 卷和 journal 870MB；15 个业务容器与 KPanel 自身 104MB 数据未受影响，需保持磁盘水位监控为常态门禁。
