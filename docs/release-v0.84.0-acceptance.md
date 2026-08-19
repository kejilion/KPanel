# KPanel v0.84.0 上线验收记录

日期：2026-08-18（Asia/Shanghai）

发布级别：L3

发布工作流：release-kpanel v2.6

候选提交 / 标签：`32ca51c70bf5d3e53837fe301f495d196974427f` / `v0.84.0`

上一稳定版本 / 回滚点：`v0.83.0` / `202a7f912d8462948657fd19fec69a89d0620338`

结论：通过，`arena-154` 已部署并完成健康核对；`prod-108` 全程未连接、未测试、未备份、未部署。

## 发布范围

- 产品提交：`4d37410`（Activity 作业弹窗阶段标记不再溢出）和 `61e2ee7`（文件/目录原生拖出，单文件使用 `DownloadURL`，目录和多选使用受限流式 ZIP）。两项均从最新主线重放后形成 `ce3338f`、`c65a63a`，未夹带旧分支差异。
- 版本提交：`32ca51c`，将 KPanel、Web 包和 CHANGELOG 统一为 `0.84.0`，补充升级/回滚说明。
- 明确不属于产品发布的内容：`local-feature-preview-standard` 仅作为治理文档和脚本普通快进并入 main（最终治理提交 `85a9388`），没有创建产品 Tag、Release、OCI 或生产变更。
- 未纳入：未提交的预览夹具、`scripts/mock-file-drag-out-api.mjs`、`web/drag-out-kpanel-preview.html`、其他工作树草稿、108 操作和任何 `kejilion.sh` 源码改动。

## 质量与安全门禁

- 候选 CI `32113694578`、候选依赖 freshness `32113694554`：success。
- 主线 CI `32115491587`、主线依赖 freshness `32115491584`：success。
- 候选第二次完整 L3：Go 全量、race/vet、Web 100 files/764 tests、typecheck/build、govulncheck、npm audit、Trivy source/image、双架构构建、安装安全和 app lifecycle 均通过；末尾 `release_gate_runner=pass commit=32ca51c`。首次 L3 的 DesktopView 既有时序测试偶发失败后，已在同一固定 Runner 内定向复跑 22/22，再从同一 SHA 完整重跑。
- L3 日志：`/root/kpanel-release-evidence/v0.84.0/l3-verify-release-r2.log`，SHA-256 `0b7724ea25c82290fbf6ea6ffb7500b8a3f980f6f62094ea810f4d212599ec6e`；bundle `C:\GitHub\_release-artifacts\v0.84.0\kpanel-v0.84.0-32ca51c.bundle`，SHA-256 `7dd196aae12b26fd2f43323b3517e4184924c616948f344f626dec5a1719e53c`。
- Web 受影响测试覆盖文件/目录拖出、DownloadURL、目录/多选归档、资源版本和权限边界，以及作业阶段标记布局；Windows 全量 100 files/764 tests、i18n 2290、生产构建和 `git diff --check` 通过。

## Release 与公开 OCI

- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.84.0>，非 draft、非 prerelease；Release workflow `32115990401` success，依赖 freshness `32115990413` success。
- 正式公开 OCI `docker.io/kjlion/kejilion-panel:0.84.0` 与 `latest` index digest：`sha256:813f7573ae9a7de9f57b1cffb78418adfdf158bbe150a08bfa03d8d38077deee`；linux/amd64 `sha256:feebb4a655476961cef40288f340acf61eb80778eb648802d6c71bddafb6b29c`，linux/arm64 `sha256:5215a56e7804864c51fc57a3d941265fbd6846183e911176447710205a16bd23`。
- 154 重新拉取正式 digest 后核对：OCI revision=`32ca51c70bf5d3e53837fe301f495d196974427f`、version=`0.84.0`、非 root 用户 `65532:65532`、healthcheck=`/paneld healthcheck`；镜像内 `kejilion.sh` SHA-256=`d8c06ad40c2845a2ee3f1f4c9f0780b7e30d65a58bca91a80cdca5c390222408`，与标签契约一致。
- `packaging/kejilion-app/kpanel.conf` 相对 `v0.83.0` 无差异；应用市场仓库无需空提交或同步写入。

## 154 备份、升级与上线后核对

- 目标仅 `arena-154`。部署前 `v0.83.0` 运行镜像为 `sha256:bfa0c98aca0d22b4fd8e6ac14507572b491c5628caf5b3924ebd230cfea4afd2`，Panel healthy、Agent active、restart=0、OOM=false。
- 停写一致性备份：`/root/kpanel-backups/pre-v0.84.0-20260818T083447Z`；`kpanel.tar.gz` SHA-256 `5fff70498c092565fe33a3f9029b06072e5bba43fc970b4cff4e2ac4864c97db`；旧镜像归档 SHA-256 `900b6dfcce9d4898362a4bfaa763af42820932ac969638ed9440fea7d0a96529`；`kpanel.conf` SHA-256 `82f06ca32ce827ef8d0c9c72e65eed9180841a23cbc507237072b58a0807ef04`。归档已独立解包逐文件比对，源/恢复 SQLite 均 `quick_check=ok`，恢复后旧服务 healthy 才执行升级。
- 标准入口：服务器现有 `/root/apps/kpanel.conf` 的 `docker_app_update`；更新日志 SHA-256 `1b2d21cf1a7b2a91f98bbd25621b55a833666da91f56317b8bc0acda9422e469`。入口保留既有 `permission_granted`、区域和统计偏好，没有覆盖宿主本地定制；`kpanel.conf` 上线前后未改变。
- 上线后：Panel image digest 精确为正式 OCI，version=`0.84.0`、revision=`32ca51c`；`/api/v1/health` 返回 `status=ok`/`version=0.84.0`，Panel healthy、Agent active、restart=0、OOM=false；近期日志仅有正常启动记录。Panel 资源快照约 74 MiB/256 MiB、CPU 约 0.05%。
- 生产外部 HTTP 从本地网络不可达（超时/503），不据此误判服务；通过 SSH 在宿主回环完成 `/login` 200、health 200 及版本核对。未使用用户日常 Chrome Profile，不启动扩展、不触碰 108。

## 回滚

- 成套回滚点为 `v0.83.0`、旧 OCI `sha256:bfa0c98aca0d22b4fd8e6ac14507572b491c5628caf5b3924ebd230cfea4afd2` 和上述备份目录；不得只切换浮动 `latest` 或只改 mode。
- 回滚步骤：停止 Panel/Agent，加载 `previous-image.tar`，恢复 `kpanel.tar.gz`、`.env`、Compose、Agent unit/binary/script 与独立 `kpanel.conf`，核对摘要和 unit target，`daemon-reload` 后启动，再复核旧版本、SQLite、health、restart/OOM 和日志。本次未触发正式回滚。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-18T15:51:20+08:00
- 候选冻结时间：2026-08-18T15:54:13+08:00
- 生产完成时间：2026-08-18T16:35:56+08:00
- 提交到生产用时：0.74 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：2
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

两次异常均已 fail-closed：首次 L3 的既有 DesktopView 时序测试偶发失败，随后定向复跑并完整重跑；备份脚本第一次使用了旧版本健康断言，在恢复旧服务后停止，修正断言并重新完成同一停写备份。未造成数据丢失、版本漂移、回滚或 108 操作。

## 遗留风险

- 未对现有业务 Compose 执行写操作，避免影响用户业务；文件拖出、目录/多选归档的真实资源版本、权限、路径和预算边界已由 L3/定向测试覆盖。
- 未做长时间 soak；本版没有新增常驻任务，生产以正式 OCI、健康、资源和日志快照作为上线后核对。
- 108 明确排除在 KPanel 测试、灰度、观察和部署之外，以节省正式服流量。
