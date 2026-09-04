# KPanel v1.1.0 发布验收记录

日期：2026-09-04

发布级别：L3

候选提交 / 标签：`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27` / `v1.1.0`

上一稳定版本 / 回滚点：`v1.0.2` / `52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`

## 发布画像

- 业务域：集群主机文件管理、轻量节点文件代理、集群 SSH 登录通知和轻量节点监控延迟。
- 变更面：Panel、Agent、轻量节点和集群协议的兼容性功能；同步更新镜像内受管 `kejilion.sh` 到 revision `2ee9856c9916b7ede8bbc19edc97e22872e86203`。
- 受影响用户旅程：从集群主机入口进入文件管理、在集群中查看文件/跨面板传输、接收轻量节点 SSH 登录通知，以及查看轻量节点真实上报延迟。
- 未变化契约：无数据库迁移、无 Compose 端口变更、无 System Center 页面/API/写入范围；旧节点通过能力协商继续兼容运行。
- 风险等级及理由：中；本版涉及跨节点文件权限、传输协议、SSH 事件签名和 Agent 生命周期，但固定 L3、候选/main CI、Release/OCI、镜像 E2E 以及 `arena-154` 生产证据均通过。

## 发布范围与未纳入内容

- 用户可见更新：集群配对主机支持直接进入文件管理；轻量节点提供受限文件代理；集群通知增加轻量节点 SSH 登录采集、签名上报、能力协商和通知中心展示；轻量节点监控展示成功上报后的真实毫秒级延迟，异常或零值不伪造质量。
- 产品提交清单：`d41e5d3`（集群主机文件入口）、`421458d`（集群安全入口）、`7783785`（配对 Panel 直达）、`d91a9c5`（轻量节点文件管理）、`93892a0`（轻量节点延迟）、`548230f`（统一 SSH 登录遥测）。
- 发布流程提交：`011b820`（v1.1.0 版本准备）、`d39cf0b`（瞬时审计失败后的候选重试）、`ed8a15a`（CI/Release 审计重试）、`83aebcf`（Makefile 发布门禁审计重试）。后三项只改善有限重试和发布可靠性，不绕过安全门禁。
- 明确未纳入：System Center 及其页面、路由、API、数据和写入；未连接 `108` 或 `prod-108`；未修改 Apps 仓库应用契约；未纳入其他未通过本次精确 SHA 门禁的工作树内容。
- System Center 排除证据：`git diff v1.0.2..83aebcf --name-only` 的路径扫描结果为 `system_center=excluded`。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | Go/Web 全量测试、固定 L3、镜像 E2E 和生产精确版本验收通过；旧节点有能力协商路径。 | 未做长期多节点 soak。 |
| 网络入侵与供应链安全 | 已验证 | `govulncheck` 无可达代码漏洞，`npm audit` 通过，Trivy 源码/镜像扫描为 0，非 root、只读根文件系统和运行契约通过。 | 未做独立第三方渗透测试。 |
| 稳定性、失败恢复与兼容 | 已验证 | 全量 Go/Web、核心 race、双架构构建、应用生命周期、镜像 E2E、生产 backup/postdeploy 通过；旧版回滚点已备份。 | 未执行受控生产回滚演练。 |
| 性能与资源预算 | 已验证 | 轻量节点延迟为实际测量值；容器资源约束、健康检查、Panel/Agent 运行状态通过。 | 未做长期带宽和多节点压力测试。 |
| 用户体验与可访问性 | 已验证 | 文件管理入口、通知展示和 i18n 相关测试通过，发布镜像 E2E 通过。 | 未执行完整浏览器缩放、键盘焦点和三语人工矩阵。 |
| 数据、配置与迁移 | 已验证 | 无数据库迁移；生产受保护文件 diff 为空，数据清单和 SQLite 检查通过，备份校验通过。 | 不适用新增迁移。 |

## 自动门禁

- 固定 Linux L3：run=`v1.1.0-83aebcf-l3-r2`，candidate=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，base main=`011b820941af0a9333d75c0d6dac742c16a64ff0`，base tag=`v1.0.2`，状态 `passed`、exit `0`；远端目标仅 `arena-154`，证据为 `/root/kpanel-release-evidence/v1.1.0-83aebcf-l3-r2`。
- L3 Runner：`kpanel-release-gate:go1.26.7-node24`，不可变 digest=`sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`；bundle SHA-256=`3884883b0ef36702a8544e6da03ee8689a26f1b9d85805a7ee308566af773349`；plan SHA-256=`a06f36050207c37ec9f863c233d7d28ac15190d84e65dc5a7bea2a6fcc7dbd5c`；远端脚本 SHA-256=`d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- L3 结果：全量前端 `npm test` 为 131 个测试文件、1121 个测试通过；`typecheck`、`i18n:check`（2129 个本地化短语、21 个 lazy catalogs）、`build`、Go 全量测试、核心 race、`go vet`、`govulncheck`、`npm audit`、Trivy、应用配置生命周期和 runtime contract 均通过。
- 候选 CI：run `33851781926`，精确 SHA=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，`completed successfully`；[候选 CI](https://github.com/kejilion/KPanel/actions/runs/33851781926)。
- 候选 Dependency freshness：run `33851781875`，精确 SHA=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，`completed successfully`；[候选 Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/33851781875)。
- 主线 CI：run `33853288045`，精确 SHA=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，分支 `main`，`completed successfully`；[main CI](https://github.com/kejilion/KPanel/actions/runs/33853288045)。
- 主线 Dependency freshness：run `33853288086`，精确 SHA=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，`completed successfully`；[main Dependency freshness](https://github.com/kejilion/KPanel/actions/runs/33853288086)。
- Release workflow：run `33853685491`，精确 SHA=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`，Release job 全部阶段成功，包含测试、安全扫描、8 个发布附件、多架构 OCI、`latest` promotion、公开 Release 和候选分支删除；[Release workflow](https://github.com/kejilion/KPanel/actions/runs/33853685491)。

## 依赖与技术栈变化

- `web/package.json`、`web/package-lock.json` 仅同步版本号到 `1.1.0`，未新增运行时依赖；`Go 1.26.7`、Node `24.20.0`、固定 Actions/基础镜像版本保持发布门禁约束。
- `govulncheck`：未发现代码可达漏洞；输出中仅有不被代码调用的传递模块提示。`npm audit --audit-level=high` 通过；Trivy 源码、依赖、配置、secret 和镜像扫描均无阻断项。
- 受管脚本：revision=`2ee9856c9916b7ede8bbc19edc97e22872e86203`，SHA-256=`77258027f934ffe6a583300f8350249978eace0ddc838e2d35f26cc5c21ae35c`；Dockerfile 的固定 `ADD --checksum`、镜像 label 和发布运行契约与其一致。
- `kejilion/apps` 契约：`origin/main:kpanel.conf` 仍使用 `docker.io/kjlion/kejilion-panel:latest`；本版未修改 Apps 仓库，标准应用市场入口完成同步和更新。

## 隔离真机与镜像验收

- 主机/用途：唯一真实目标为 `arena-154`；环境策略 `hybrid` 的 `candidate-validation`、`production-safety-check`、`production-deploy` 均通过；未连接 `108` 或 `prod-108`。
- 独立公开镜像 E2E：使用候选精确 SHA 对应的 `packaging/tests/image-e2e.sh`，在 `arena-154` 以临时本地端口 `18081` 运行 `docker.io/kjlion/kejilion-panel:1.1.0`，输出 `image_e2e=pass`；脚本自动清理测试容器、网络和临时目录。
- GitHub Release：[`v1.1.0`](https://github.com/kejilion/KPanel/releases/tag/v1.1.0)，`draft=false`、`prerelease=false`，附件为 Agent amd64/arm64、轻量节点 amd64/arm64、部署归档、`SHA256SUMS`、`LICENSE` 和 `THIRD_PARTY_NOTICES.md`，共 8 个。
- OCI 产物：`docker.io/kjlion/kejilion-panel:1.1.0` 与 `:latest` 的 OCI index digest 均为 `sha256:a4d1c9faac7b576a75f77522409c981105f817ccd0c8f90b355edd106670113f`；amd64=`sha256:b851dddbd34e34febbae1463d854242b9aa0782504b535a92ab1aaf0381bceb7`；arm64=`sha256:0d95cc7d628ae37b712630fe1f1496286a0177126f1738b59acf8d4054e97933`。

## 生产证据与部署

- 生产证据统一 run=`v1.1.0-production`，三阶段本地记录目录分别为 `C:\GitHub\_release-evidence\v1.1.0-production-preflight`、`C:\GitHub\_release-evidence\v1.1.0-production-backup`、`C:\GitHub\_release-evidence\v1.1.0-production-postdeploy`；远端对应 `/root/kpanel-release-evidence/v1.1.0-production/production-{preflight,backup,postdeploy}`。
- preflight：`2026-09-04T08:28:05Z` 至 `08:28:07Z`，expected version=`1.0.2`，`passed/0`；plan SHA-256=`186f96b0e42425d0c03e1a7fea6a2932a3bf4405a66365f287895f91c15f464d`，manifest SHA-256=`30f6e03d99c668e0e7ab41b90a970388854118eae55018aa403a57ff5e27283e`。
- backup：`2026-09-04T08:45:40Z` 至 `08:45:49Z`，expected version=`1.1.0`，`passed/0`；plan SHA-256=`bb164aef239cf4d6cb575111ea384153928b0e06bd5ef41af058d8d8b10fa13b`，manifest SHA-256=`e172a6b0666bfa6f41bff3e8d58c7de9e7c5bdb95d41ff66312f79f45e558a88`。
- 备份位置：`/root/kpanel-backups/pre-v1.1.0-20260904T084540Z`；`image-load-verify.txt`、Agent unit、`kpanel.conf`、Panel 数据归档、旧镜像归档和 `panel-inspect.json` 均通过 SHA-256 校验。
- 标准部署入口：`ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`；应用市场同步成功，拉取 `latest` 的 digest 为本版 OCI index，Panel 重建并启动成功。
- postdeploy：`2026-09-04T08:46:31Z` 至 `08:46:33Z`，expected version=`1.1.0`、revision=`83aebcfb94fa387f8ea52c6c48f1ddb161e1af27`、image digest=`sha256:a4d1c9faac7b576a75f77522409c981105f817ccd0c8f90b355edd106670113f`，`passed/0`；plan SHA-256=`1b4b60c5fd260c31df7c5b43bf4c525b55c436808800851879e8741f2b753140`，manifest SHA-256=`dd6cdf643ac3cff652858e554eabb57c03613f79102ab360203623dbda0bcca4`。
- 生产状态：Panel `running/healthy`，health=`status=ok version=1.1.0`，Agent `active/running/enabled`，`RestartCount=0`，`OOMKilled=false`，Panel revision 和 OCI digest 精确匹配，protected data/config diff 为空，fatal logs=`none`。
- 本次生产写入仅包括 `arena-154` 的标准 evidence、停写备份和标准应用市场更新；未执行 System Center、其他主机或非标准绕行命令。

## 回滚

- 源码/tag：`v1.0.2` / `52f9f4dd8ae04049ccc4b36e252af8b43d35bf8d`。
- 上一稳定 OCI index：`sha256:d6dec696d803632f3d791af9860e72648e24e2f957b428f63207bbc1d8017b0a`；amd64=`sha256:ca0df96fff12af629355f4bcd316b4b52f958f34b686906122d6a18e69bf1aa5`；arm64=`sha256:0ec7f6890527129a73ad1d55455518aee35aa4755c0aa2720b75072b7125f95c`。
- 数据/配置备份：`/root/kpanel-backups/pre-v1.1.0-20260904T084540Z`，包含 Panel 数据、Compose/配置、Agent unit、旧镜像和校验清单。
- 回滚步骤：按应用市场回滚规范停 Panel/Agent，恢复备份的 Compose、`.env`、数据、Agent 文件和旧镜像，恢复 `v1.0.2` 受管脚本后使用标准入口启动；再以 production evidence 核对旧版 health、Agent、digest、restart/OOM、数据和日志。禁止只切换浮动 `latest`。
- 回滚状态：未执行回滚；当前 `arena-154` 为 `1.1.0` healthy。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-04T13:58:59+08:00
- 候选冻结时间：2026-09-04T14:23:08+08:00
- 生产完成时间：2026-09-04T16:46:33+08:00
- 提交到生产用时：2.79 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：1
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "candidate-ci/npm-audit/transient-endpoint-502",
    "position": "before-production-write",
    "count": 1,
    "impact": "候选 CI 的 Audit Node dependencies 阶段因 registry.npmjs.org 审计 endpoint 瞬时 502 失败；未进入主线推进或生产写入。",
    "recoveryEvidence": "候选 CI run 33849450437 的精确审计失败被定位为外部 endpoint 502；随后 d39cf0b、ed8a15a 和 83aebcf 仅增加有限三次重试与退避，当前候选 CI、L3、main CI 和 Release 均以 83aebcf 成功。",
    "permanentAction": "所有候选 CI、Release workflow 和 Makefile security-audit 入口统一使用有限重试，仍在三次失败后退出；不降低 audit-level、不吞掉非零状态，并由同 SHA L3 与 Release 门禁持续验证。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 未验证风险：长期多节点 soak、真实浏览器完整缩放/键盘焦点/三语人工矩阵、生产故障注入和受控回滚尚未执行；这些不阻断本版，因为本次固定 L3、候选/main CI、Dependency freshness、Release/OCI、镜像 E2E 和 `arena-154` 三阶段证据均通过。
- 后续维护：继续观察轻量节点文件代理的多节点并发、跨架构 Agent 升级和 SSH 通知长时间运行；若发现问题，按 `v1.0.2` 的完整 OCI、Compose、数据、Agent 和受管脚本备份回滚。
- 生产结论：`v1.1.0` 已在唯一授权目标 `arena-154` 上完成上线，System Center 保持排除，未连接 `108`/`prod-108`。
