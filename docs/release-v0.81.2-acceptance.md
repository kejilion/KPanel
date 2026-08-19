# KPanel v0.81.2 上线验收记录

日期：2026-08-17（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`23b61a7c1e33ffe01e90f0fb21df10bc6d10f142` / `v0.81.2`

上一稳定版本 / 回滚点：`v0.81.1` / `sha256:83372a7c772e67ddf3682c7b28d2ca1763ed48284fe50e187d1541c81b3f1ddd`

结论：通过，生产已部署，未回滚。

## 发布画像

- 业务域：KPanel 前端可见文案与简体中文、繁体中文、英文词条。
- 变更面：展示；无后端、API、依赖、数据、宿主机权限或部署契约变化。
- 受影响用户旅程：登录、首次初始化、密码恢复，以及概览、网站、Docker、应用、AI、集群、终端、文件、审计、任务、诊断、监控和设置页面的可见提示。
- 未变化契约：API、数据、端口、Compose、Agent 权限、`kejilion.sh`、应用安装/更新入口和 `packaging/kejilion-app/kpanel.conf` 均未变化。
- 风险等级及理由：发布、镜像和生产部署属于 L3；功能差异本身仅替换既有文案槽位及一处精确测试，不新增输入、网络、权限或持久化路径。

## 发布范围与未纳入内容

- 用户可见更新：统一产品核心定位，重写登录/初始化/密码恢复说明，优化主要页面现有简中、繁中和英文提示。
- 精确提交清单：开发提交 `e1938a5124b381424c232754b288451a7fc3f8e2`；线性集成提交 `90e8dc243adc0f4d2977ccf122eebf8b39ae2782`；版本提交 `23b61a7c1e33ffe01e90f0fb21df10bc6d10f142`。
- 候选基线：`origin/main@0d39876b913f33425d16cb74845737df996fddb7`；开发提交与基线严格 0/1 线性，迁移无冲突。
- 明确未纳入：其他分支、依赖升级、后端/API 修改、文案新槽位、应用市场配置和任何 `prod-108` 操作。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 2256 条 i18n、98 文件/741 测试、初始化/登录三语双视口真实浏览器与生产登录页复核 | 本版不改变 Agent、API 或双端协议，写入闭环不适用 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck` 可达漏洞 0、npm audit 0、Trivy source/image 漏洞/secret/misconfiguration 0；正式 OCI revision 与候选一致 | 无安全边界变化 |
| 稳定性、失败恢复与兼容 | 已验证 | Go 全量、核心 race、vet、安装安全、应用生命周期、公开镜像 E2E、停写备份恢复和生产健康采样通过 | 无 Schema 或配置兼容窗口变化 |
| 性能与资源预算 | 已验证 | 生产 5 次采样 CPU 0.02%、内存 74.92–74.93 MiB / 256 MiB、7 PIDs、0 restart、无 OOM | 仅静态文案，无新增轮询或常驻任务 |
| 用户体验与可访问性 | 已验证 | 初始化和登录页简中/繁中/英文在 1280×800、390×844 共 12 组隔离验收；生产登录页另 6 组通过，均无横向溢出或非预期控制台错误 | 未逐页人工浏览所有后台页面；现有词条、类型检查和全量测试覆盖未变结构 |
| 数据、配置与迁移 | 不适用 | `.env`、Compose、托管 `kejilion.sh` 更新前后哈希一致；SQLite `integrity_check=ok` | 本版无数据、配置或迁移变更 |

## 自动门禁

- `git diff --check`、版本一致性、`scripts/verify-change.sh` 均通过。
- Linux L3：在 `arena-154` 从完整 Git bundle detached 到 `23b61a7c1e33ffe01e90f0fb21df10bc6d10f142`，Runner image ID `sha256:b593c0ffe32e80a6a6ae8fcac38ed916587dbf698a6b6a55fa33887298737148`；输出 `L3 release verification completed` 与 `release_gate_runner=pass`。
- L3 日志 SHA256：`31282759bd5c06753bacad93f33d89276db38dd9ac9f3974b1b40b5a05483af4`；bundle SHA256：`4e3233d3461d27bcf4c5cd881150fdccb435a2514638a906727749e2be33f48c`。
- 全量结果：Go `go test ./...`、`panel/auth/dockerx` race、`go vet ./...`、linux/amd64 与 linux/arm64 二进制、前端 98 文件/741 测试、i18n 2256/20、typecheck、生产 build、治理、部署与生命周期门禁均通过。
- 候选 CI：[`31999639023`](https://github.com/kejilion/KPanel/actions/runs/31999639023) success；候选依赖治理 [`31999639016`](https://github.com/kejilion/KPanel/actions/runs/31999639016) success。
- 主线 CI：[`32000711182`](https://github.com/kejilion/KPanel/actions/runs/32000711182) success；主线依赖治理 [`32000711188`](https://github.com/kejilion/KPanel/actions/runs/32000711188) success。
- Release workflow：[`32001192267`](https://github.com/kejilion/KPanel/actions/runs/32001192267) success；tag 依赖治理 [`32001192215`](https://github.com/kejilion/KPanel/actions/runs/32001192215) success。

## 依赖与技术栈变化

- 本版没有依赖、工具链、基础镜像、Action、扫描器或受管脚本版本变化。
- 候选、主线和 tag 三层依赖治理均成功；锁文件只同步项目版本号。
- `govulncheck`、npm audit 与固定 Trivy 数据库扫描均完成；Trivy 0.72.0 提示上游已有 0.74.0，扫描器升级继续由独立依赖流程评估，本版不夹带升级。

## 隔离真机与浏览器验收

- 主机/环境：`arena-154`，Linux amd64；环境策略允许 `candidate-validation`、`browser-validation`、`production-deploy` 与 `production-safety-check`。
- 精确候选镜像：`sha256:dfe1fe1797c5bba9adad5a3b7a6ff32c0da0b21fbec08f02cda7ac322dc445e9`，OCI version/revision 为 `0.81.2` / 精确候选提交。
- 独立后台 Google Chrome、随机临时 Profile、loopback-only SSH tunnel；未接管用户 Chrome。初始化页和登录页的简中/繁中/英文、桌面/390px 共 12 组全部通过。
- 浏览器验收记录 SHA256：`cf71963b0f1a71aceb69864b133cec6f384f08bef9103352686b6080584f51d8`；截图与脚本位于 `C:\GitHub\_release-artifacts\v0.81.2`。
- 首次隧道使用不同本地端口触发 Host allowlist 421，门禁在发布前拦截；改为同端口 loopback 后复验通过。未认证 `/api/v1/auth/session` 401 经精确 URL 分类为预期登录状态。
- 本版没有宿主机业务写路径；隔离测试账户、容器、网络、数据、Profile 和隧道均已清理，无 soak 必要。

## 发布产物与公开仓库复核

- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.81.2>，非 draft、非 prerelease，8 个附件公开。
- source commit：`23b61a7c1e33ffe01e90f0fb21df10bc6d10f142`；annotated tag object：`1787915646dd23994998eb9b1d74c4507887b398`。
- Docker `0.81.2` 与 `latest` OCI index：`sha256:089602f13e8a05f5d11cf809c23a030a0986770a3642ee21c8728d90b7dac850`。
- `linux/amd64`：`sha256:bd416bb74718ee80668b22d07de0829d45c1462581dc5e35b1d40054e8fef804`；`linux/arm64`：`sha256:25a0a1059d14d030e591bb421e6e81214b54d4ed90d009da18761d16408cf5d7`；另含对应 provenance/SBOM attestation。
- 附件包括双架构 Agent、双架构 lightweight node、`kejilion-panel-deploy-0.81.2.tar.gz`、`SHA256SUMS`、LICENSE 和 notices。
- 公开镜像重新拉取并运行 `packaging/tests/image-e2e.sh`，输出 `image_e2e=pass`。
- `packaging/kejilion-app/kpanel.conf` 相对 v0.81.1 无差异；KPanel 与 `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` blob 均为 `34316059d4e42f527819bc7d56e0ff14ec434c96`，无需空提交。

## 生产部署安全核对

- 生产目标和授权：仅 `arena-154`；用户明确授权完整发布和生产部署。
- `prod-108`：禁用全部 KPanel 操作；本次未连接、未备份、未部署、未升级、未核对。
- 部署前：v0.81.1，revision `e2416ae393e0e23323c0d3088b996bc8d89273d5`，OCI `sha256:83372a7c772e67ddf3682c7b28d2ca1763ed48284fe50e187d1541c81b3f1ddd`；Panel healthy、restart 0、OOM false，Agent active。
- 停写备份：`/root/kpanel-backups/pre-v0.81.2-20260817T062910Z`；应用目录归档 SHA256 `46d6ef947b59b7456e15612ee9c690442417a99b5b0f297fce1f3811e008aaa5`，旧镜像归档 SHA256 `a7aa4cc3feab4a4e593464aea4367f7677872e78b624d2fe7eebc3f1bf798fdc`。
- 备份已独立解包并逐文件对比，原始/恢复 SQLite 均 `integrity_check=ok`。
- 部署入口：从 apps 精确 commit/同 blob 契约调用标准 `docker_app_update` 事务。
- 部署后：v0.81.2，revision 与正式 OCI 一致；Panel healthy、restart 0、OOM false，Agent `0.81.2 v1alpha1` active；生产登录页三语双视口复验通过。
- 更新日志 SHA256 `d6b831a254c4f4e2511e07b03f4498bbed4de52f6f146930bcff4b64e5e2742c`；健康采样 SHA256 `c39781141bfa509456a9eaf36728ec5e6d89dcc410719de04195ff2be5a5378b`。
- `.env`、Compose 和托管 `kejilion.sh` 更新前后保持一致；脚本 SHA256 均为 `d73231f146f7398d7b50133695faf2116134fbfe33a7b94068e277cc7b82df55`。
- 生产已执行写操作：停写备份和标准 KPanel 更新事务；未修改账户、应用、站点或业务配置。

## 回滚

- 源码/tag：`e2416ae393e0e23323c0d3088b996bc8d89273d5` / `v0.81.1`。
- 镜像 digest：`sha256:83372a7c772e67ddf3682c7b28d2ca1763ed48284fe50e187d1541c81b3f1ddd`，旧镜像已独立归档。
- 数据/配置备份：`/root/kpanel-backups/pre-v0.81.2-20260817T062910Z`，包含完整 `/home/docker/kpanel`、Compose/Agent unit 记录、容器 inspect、SQLite 和旧镜像归档。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.81.1 镜像归档，成套恢复完整应用目录与匹配配置，恢复旧 Agent unit，`systemctl daemon-reload` 后启动，再核对版本、SQLite、健康、restart/OOM、脚本哈希和日志；禁止只切换浮动 `latest`。
- 本次未触发正式回滚；生产实际版本为 v0.81.2，健康正常。
- GitHub Latest、Docker `latest` 与标准更新入口均指向 v0.81.2 / `sha256:089602f13e8a05f5d11cf809c23a030a0986770a3642ee21c8728d90b7dac850`。
- 公共默认更新通道决策：不适用；生产部署安全核对成功。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-17T13:36:24+08:00
- 候选冻结时间：2026-08-17T13:41:50+08:00
- 生产完成时间：2026-08-17T14:34:37+08:00
- 提交到生产用时：0.97 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

上述 1 次为生产写操作前的隔离浏览器通道端口与 Host allowlist 不匹配；门禁拦截后以同端口 loopback
重新取证并通过。它不属于产品变更失败，未计入变更失败率。

## 遗留风险与后续准入

- 未验证风险：没有逐页人工浏览所有已认证后台页面；未变化的 DOM 结构由 typecheck、i18n 和 741 项全量测试覆盖，受影响认证页面已做真实浏览器和生产复核。
- 不阻断本版的理由：本版仅替换既有文案和精确测试，不改变业务逻辑、API、数据、权限或部署契约；最终源码、OCI、浏览器和生产 revision 完全一致。
- 本次复用 `release-kpanel` v2.4 与 `project-version-control`；隔离验证通道异常已进入独立流程指标，
  不再被迫丢失或歪曲为产品失败。
