# KPanel v0.85.0 上线验收记录

日期：2026-08-18（Asia/Shanghai）

发布级别：L3

发布工作流：release-kpanel v2.6

候选提交 / 标签：`f73b96ae7064d189e398c1b1b2f2003c1d308238` / `v0.85.0`

上一稳定版本 / 回滚点：`v0.84.0` / `32ca51c70bf5d3e53837fe301f495d196974427f`

结论：通过，`arena-154` 已按标准应用入口升级并完成健康核对；`prod-108` 全程未连接、未测试、未备份、未部署。

## 发布范围

- 从 `v0.84.0` 主线纳入且完成复核的三项 Docker 管理改动：管理页布局紧凑化（`b00309b`）、窄屏操作列可见性（`2f352a2`）、按真实 Docker 创建时间排序（`e9fe0eb`）。
- 版本提交 `f73b96a` 将 KPanel、Web 包和 CHANGELOG 统一为 `0.85.0`，并记录升级/回滚边界。
- 集群公开分享、一次配对双向文件互传、RHEL/WSL 调优脚本固定等内容已在既有主线/`v0.84.0`，本次不重复夹带；治理预览、未提交草稿和临时夹具均未纳入。
- 本版未修改 `kejilion.sh`、Agent 协议、端口、数据 schema 或 `packaging/kejilion-app/kpanel.conf`；应用市场无需新提交。

## 质量与安全门禁

- 候选 CI `32127611458`、候选依赖 freshness `32127611357`：success。
- 主线 CI `32128030117`、主线依赖 freshness `32128030014`：success。
- Release workflow `32128406223`、Release 依赖 freshness `32128406107`：success；source verification、Go binaries、govulncheck、npm audit、Trivy source/image、runtime contract、app lifecycle、双架构 OCI 均通过。
- Web：100 files / 767 tests、i18n 2285（20 catalogs）、typecheck、生产构建和 `git diff --check` 通过；`npm audit --audit-level=high` 为 0 vulnerabilities。
- Linux/Go/容器 L3 以 CI/release runner 为准；本机 Windows 无 Go/Docker，不将本机缺失工具表述为产品门禁失败。
- Bundle：`C:\GitHub\_release-artifacts\v0.85.0\kpanel-v0.85.0.bundle`，SHA-256 `0D5B29425EAF6CA82BBEEBE7E0C0501798739E6AD572F36BF54A338E96C522E0`。

## Release 与公开 OCI

- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.85.0>，非 draft、非 prerelease。
- 正式公开 OCI `docker.io/kjlion/kejilion-panel:0.85.0` 与 `latest` index digest：`sha256:3894a1f4dad31fa853b4bd93d561e4eaafe6e2c14cbcac49031a65602f57bf40`。
- 公开 OCI 平台摘要：amd64 `sha256:7a8887b258982a6ffc67ba1cc69cb4f0f33e7240e82dd6f9f2fbff63fb172a4d`；arm64 `sha256:ee86a853864d41f27b8dd7d35b463deeec11dc4c690ad20280f7123258397b51`。
- 154 使用正式公开 digest 做镜像回拉/E2E，标签核对为 version=`0.85.0`、revision=`f73b96ae7064d189e398c1b1b2f2003c1d308238`、脚本 revision=`fdb0ac0e1f2b98d27339937e7f8eb0c9299c56a9`、脚本摘要=`d8c06ad40c2845a2ee3f1f4c9f0780b7e30d65a58bca91a80cdca5c390222408`。
- `packaging/kejilion-app/kpanel.conf` 相对 `v0.84.0` 无差异；应用仓库无需空提交或同步写入。

## 154 备份、升级与上线后核对

- 目标仅 `arena-154`。升级前 Panel 运行镜像为 `sha256:813f7573ae9a7de9f57b1cffb78418adfdf158bbe150a08bfa03d8d38077deee`，version=`0.84.0`、revision=`32ca51c70bf5d3e53837fe301f495d196974427f`。
- 停写一致性备份：`/root/kpanel-backups/pre-v0.85.0-20260818T110304Z`；`kpanel.tar.gz` SHA-256 `10a6a02641fa48f385772c1cdb9ae86435982ecdeef453d9503053921fe225e9`；旧镜像归档 SHA-256 `baf681d94e4af99fa1707729c0ee542209de44722b8332f688349f1531e3f806`；`kpanel.conf` SHA-256 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`。归档解包、`.env`/Compose 比对和恢复后 Panel/Agent 健康核对均通过。
- 标准入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /usr/local/bin/k app kpanel`。未手改 Compose、数据或其他业务容器。
- 上线后正式镜像 digest、version、revision 与脚本标签均精确匹配；镜像内 `/release/kejilion.sh` SHA-256=`d8c06ad40c2845a2ee3f1f4c9f0780b7e30d65a58bca91a80cdca5c390222408`。
- 宿主 `/home/docker/kpanel/bin/kejilion.sh` 保留安装器运行时 `permission_granted="true"` 标记，因此原始文件摘要为 `f5ffbcc356b1a7681b700f249e58294efb62f9db9d7b4f285866929bef91461b`；将该单字节运行时标记归一化后与固定脚本摘要完全一致，不属于脚本漂移。
- `/api/v1/health` 返回 `status=ok`、`version=0.85.0`；Panel healthy、Agent active、restart=0、OOM=false。连续 3 次约 5 秒采样均通过；Panel 资源约 11.1 MiB / 256 MiB、CPU 约 0.03%。

## 回滚

- 成套回滚点为 `v0.84.0`、旧 OCI `sha256:813f7573ae9a7de9f57b1cffb78418adfdf158bbe150a08bfa03d8d38077deee` 和上述备份目录；不得只切换浮动 `latest`。
- 回滚步骤：停止 Panel/Agent，加载 `previous-image.tar.gz`，恢复 `kpanel.tar.gz`、`.env`、Compose、Agent unit/binary/script 与 `kpanel.conf`，必要时 `daemon-reload`，启动后复核旧版本、SQLite、health、restart/OOM 和日志。本次未触发正式回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-18T18:27:44+08:00
- 候选冻结时间：2026-08-18T18:36:27+08:00
- 生产完成时间：2026-08-18T19:09:55+08:00
- 提交到生产用时：0.70
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：4
- 其中生产写操作开始后异常次数：2
<!-- kpanel-release-process-metrics:end -->

四次均 fail-closed：三次备份脚本校验/断言问题（每次均恢复 Panel/Agent，最终备份恢复校验通过），以及一次上线后脚本摘要检查未考虑 `permission_granted` 运行时单字节标记；均未造成数据丢失、版本漂移、回滚或 108 操作。失败备份目录保留在 `/root/kpanel-backups/pre-v0.85.0-*` 供审计，不作为正式回滚点。

## 遗留风险

- 本版仅改变 Docker 管理前端行为，未对现有业务 Compose 执行写操作；业务容器状态未被更新入口改变。
- 未连接或测试 108，后续如需其上线必须另行授权和独立门禁。
- 未做长时间 soak；本版无新增常驻任务，以正式 OCI、健康、资源和短时连续采样作为上线后核对。
