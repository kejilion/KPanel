# KPanel v0.81.0 上线验收记录

日期：2026-08-17（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`21261e35dd2aa8bc3fea4bed004c8b5ba896c975` / `v0.81.0`

上一生产版本 / 回滚点：`v0.80.1` / `sha256:54f44fce245d8f03991c7880a6c49a065f3662335375b9d4c4a6349d8083700f`

结论：通过

## 发布范围

- 应用市场支持可选 `addedAt`，在新增后的 60 个 UTC 日内显示“新品”，并在不破坏刚安装应用与已安装应用优先级的前提下将新品提前。
- 离线安全目录由 147 项同步到 153 项：新增内置 `#116` DeepSeek Harness，以及 AIradio、Arena Brawl、Bomb Party、Ice Climber Arena、Neon Arena 五项第三方应用；简体、繁体和英文元数据及本地图标齐全。
- 动态目录同步改为只为真正新增的应用下载图标；已存在应用的本地图标、路径、哈希和审核元数据一律保留，防止官方目录刷新覆盖 KPanel 自有定制。
- `addedAt` 只在首次纳入时写入，后续同步、远端目录缺失日期或异常未来日期均不会刷新已有日期或改变安全回退。
- 版本与 CHANGELOG 更新到 v0.81.0；无数据库、API、端口、Compose、Agent、应用市场安装契约或托管脚本迁移。
- 全面盘点 KPanel、`kejilion/sh` 与 `kejilion/apps` 后，本次仅上述 KPanel 聚焦功能未上线；壁纸、集群分享、跨 KPanel 文件互传等内容已在 v0.78.0-v0.80.1 进入主线和生产，未重复混入。

## 定制保护与目录真源

- 基线 `e2cb9ddb5c5746d01c8547a7545c1f6c9da7a59a` 到最终候选仅新增 6 个 `web/public/app-icons/*.webp`；已有图标二进制差异为 0。
- 目录同步后仍为 153 项（内置 116、第三方 37）；与线上 `app.kejilion.sh` 的 ID/token 集合一致，候选只补充项目审核的繁体元数据、离线图标及首次纳入日期。
- 同步脚本复验结果：新增 6 项、已有条目变化 0；全部 153 个本地图标与目录登记 SHA-256 一致。
- `qinglong`、`nginx-stream`、`kpanel` 等既有项目定制图标和元数据均保持主线版本，没有被上游目录内容覆盖。

## 自动门禁

- Web：98 个测试文件、741 项测试通过；i18n 2255 条文案、20 个按页加载 catalog；typecheck、生产构建通过。
- Go：全包测试、核心包 race、vet、linux/amd64 与 linux/arm64 构建通过。
- 安全与供应链：govulncheck 可达漏洞 0、npm audit 0、Trivy source/image HIGH/CRITICAL/secret/misconfiguration 0；受限容器、安装安全、版本、治理、依赖、业务上下文、托管脚本和应用生命周期门禁通过。
- 最终 bundle：`kpanel-v0.81.0-21261e3-final.bundle`，SHA-256 `58e620c07413101807c31d18b5a203984f80d03e33727edc58d3b17503fb0643`；包含业务基线所需的 v0.78.0-v0.80.1 稳定 Tag。
- arena-154 最终 L3：`L3 release verification completed`；日志 SHA-256 `e914d346e806d5a00df5d53a6366462ec4d2bc32903ac3605be32e143aa310f8`。
- 首个校正后 bundle 只携带 v0.80.1，业务事实门禁因无法从 v0.78.0 基线解析稳定 Tag 而 fail-closed；补齐不可变 Tag 引用后从同一冻结 SHA 重新完整执行，代码未因此变化。

## 隔离真机与浏览器验收

- arena-154 从精确 source/revision 构建隔离候选；随后从公开 OCI digest 重新拉取，在只读根文件系统、非 root、cap-drop、内存/PID 限额下完成启动、Bootstrap、主页和 `/paneld healthcheck` E2E。
- 真实浏览器显示 153 项；6 个新增应用均立即显示“新品”并排在普通未安装应用之前，DeepSeek Harness 显示为内置 `#116`。
- 简体中文、繁体中文、英文均显示正确名称、说明和新品徽标；1280px 桌面与 390px 手机视口无页面级横向溢出；控制台 error/warn 为 0。
- 首轮页面验收发现北京时间已到 8 月 17 日但 UTC 仍为 8 月 16 日，原 `addedAt=2026-08-17` 被未来日期保护正确隐藏；最终提交将 6 项校正为实际首次纳入的 UTC 日期 `2026-08-16` 并重新跑完全部门禁。
- 浏览器验收记录 SHA-256 `15a81e4e2e0c90e6ae974415c461fef5122560a3c39a39051cd1765fcce8aef3`；公开镜像 inspect 证据 SHA-256 `db2c040e3bb1880a2b99c2a922dee05b6bba51ae0954042027d6c1e3dab6be74`。

## GitHub、Release 与公开产物

- source tree：`69004f9f38ffbd9a4d53fe2fa42903ddca00b3c2`；annotated tag object：`569260e85071837719b457fc21669de6f0b88436`。
- 候选 CI `31961336819`、候选依赖治理 `31961336771`、主线 CI `31961545319`、主线依赖治理 `31961545295`：success，head SHA 精确匹配。
- Release workflow `31961794301`、tag 依赖治理 `31961794401`：success。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.81.0>，非 draft、非 prerelease，8 个附件完成公开校验。
- Docker `0.81.0` 与 `latest` OCI index：`sha256:541f9b2d3b4b6e17c925e6c663da162e1bc42db855f7a253e95c8b5eba82100d`。
- `linux/amd64`：`sha256:e31fc5a4a3938706c820fc21b04b2c325bb2e2819adb48619814e11e826325e8`；`linux/arm64`：`sha256:d379b9bdb6a90901eca63c68e7502a21df41031510d6a2538fa574079a630c9e`。
- 公开镜像重新拉取受限容器 E2E 通过，version=`0.81.0`、revision=`21261e35dd2aa8bc3fea4bed004c8b5ba896c975`，与正式提交一致。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；与 v0.81.0 packaging 契约一致，因此未制造空提交。
- `kejilion/sh main@70a36d736f933d78bbe682f53e37efb729d03e7e` 已公开 `deepseek_harness_manager.sh`，Raw SHA-256 `45b6efbb788c2f6aaaf181b1da62638e801dd8f62e58e42df3ae9052fc5b8a96` 且 `bash -n` 通过；按用户要求小补丁不提升 `kejilion.sh` 的 v4.5.7 版本号。

## 生产部署与健康

- 唯一 KPanel 生产目标：`arena-154`；`108` 按用户长期约束未连接、未测试、未备份、未部署。
- 部署前：v0.80.1，运行 OCI/revision 为 `sha256:54f44fce245d8f03991c7880a6c49a065f3662335375b9d4c4a6349d8083700f` / `9f515017f7a576eb4c134c116e2141b881390eb9`；Panel healthy、restart 0、OOM false，Agent active。
- 停写备份：`/root/kpanel-backups/pre-v0.81.0-20260816T174003Z`；业务目录归档 SHA-256 `e553e470279eb3efb4f9008e35264e53ad605f70c96ca8380d35c6c4387df837`，旧镜像归档 SHA-256 `a5030b6da353c5f1a19c34846889c212bbe90035c6b2de5feb7df456e3f583b8`。
- 备份在停写状态形成，已独立解包、逐文件 SHA-256 对比，并对原始/恢复副本执行 SQLite `integrity_check=ok`；Compose、`.env`、Agent unit、二进制、数据和旧镜像均包含在回滚材料中。
- 标准应用入口完成更新；部署日志 SHA-256 `bd3347ddc76b1a8658fc3a841bfdc1c36634f84e549950339b1295358e232909`。首次执行器在 source 契约前被 Bash 函数作用域保护拒绝，未拉取镜像、未停服务、未修改生产；修正执行器作用域后一次完成升级。
- 部署后：Panel/Agent 均为 v0.81.0；运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active。
- 五次健康/资源采样通过：Panel 73.56-73.57 MiB/256 MiB、7 PIDs，CPU 0.02%-0.08%；采样日志 SHA-256 `ca344d957f565998faa53491a368e4f4a0046691e5d6d0bb95136010cca1524f`，磁盘剩余约 12 GiB。
- 更新前后宿主机托管脚本 SHA-256 均为 `d73231f146f7398d7b50133695faf2116134fbfe33a7b94068e277cc7b82df55`，本机偏好和脚本定制未被镜像内原始脚本覆盖。

## 回滚

- 源码/tag：`9f515017f7a576eb4c134c116e2141b881390eb9` / `v0.80.1`。
- 镜像 digest：`sha256:54f44fce245d8f03991c7880a6c49a065f3662335375b9d4c4a6349d8083700f`；旧镜像已形成独立归档。
- 回滚材料：`/root/kpanel-backups/pre-v0.81.0-20260816T174003Z`，包含完整 `/home/docker/kpanel`、Compose、`.env`、Agent unit、二进制、业务数据、镜像归档、原始 inspect 与 SHA256SUMS。
- 回滚必须停止 Panel 与 Agent，加载旧镜像归档并恢复其 `latest` 标记，成套恢复 `/home/docker/kpanel` 及 Agent unit，执行 `systemctl daemon-reload`，再启动 Agent 与原 Compose，并核对 v0.80.1、SQLite、Panel-Agent、restart/OOM 和日志；禁止只切换浮动 `latest`。
- 未触发正式回滚；回滚点已通过独立恢复材料验证并保持可执行。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-17T00:09:23+08:00
- 候选冻结时间：2026-08-17T01:06:21+08:00
- 生产完成时间：2026-08-17T01:42:35+08:00
- 提交到生产用时：1.55 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与沉淀

- 远端动态目录未来新增条目若没有项目审核的 `addedAt`，不会自行显示为新品；这是避免远端数据擅自改变产品排序的预期边界，日期由同步入库提交冻结。
- 本轮不安装或运行 DeepSeek Harness 及五个第三方应用，只验证目录、选择器、元数据、图标和展示；其安装生命周期沿用各自脚本/Compose 门禁。
- 本轮复用 `release-kpanel` v2.4 与既有版本治理；定制保护逻辑已沉淀在 `scripts/sync-app-market.mjs` 和 `docs/application-market.md`，未新增重复工作流。
