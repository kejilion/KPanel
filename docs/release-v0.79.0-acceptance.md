# KPanel v0.79.0 上线验收记录

日期：2026-08-16（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`8ad781a2a6ef4d34adfbb09ba6092518fc13c9af` / `v0.79.0`

上一生产版本 / 回滚点：`v0.77.0` / `sha256:072ede148231f4233d9fe849c0882340b1fcbed4bd327a13da830cacc058f378`

结论：通过

## 发布范围

- 纳入 v0.78.0 已冻结但未部署到生产的完整能力：一次配对双向文件传输，以及文件管理器与桌面之间四种组合的文件/目录快捷方式互传。
- 集群公开分享优化：公开分享入口前置，增强列表/卡片展示，压缩主机布局并提高信息可读性；补齐简体、繁体及其他语言目录文案。
- 桌面壁纸：新增壁纸选择器、持久化、轻量遮罩和切换动画；主题菜单在动画前确定性关闭。
- 版本、CHANGELOG 和当前业务上下文已更新到 v0.79.0；未纳入旧工作树、未提交草稿、重复补丁或不完整实验。
- `v0.78.0` 保持不可变公开历史，但没有部署到生产；生产由 v0.77.0 直接升级到 v0.79.0。

## 自动门禁

- Web：98 个测试文件、734 项测试通过；壁纸/集群分享定向 3 个文件、44 项测试通过。
- i18n：2254 条文案、20 个按页加载 catalog；typecheck、生产构建、npm audit 0 漏洞通过。
- Go：全包测试、核心包 race、vet、linux/amd64 与 linux/arm64 构建通过。
- 安全与供应链：govulncheck 可达漏洞 0，Trivy source/image HIGH/CRITICAL 0，受限容器、安装安全、版本、治理、依赖和业务上下文门禁通过。
- 最终 bundle：`kpanel-v0.79.0-8ad781a.bundle`，SHA-256 `953d25a79356a0e59fa017d092c6643c87da4fc92308936803ff3adffab65269`。
- arena-154 完整 L3：`L3 release verification completed`；日志 SHA-256 `241c32923d74b3c4eb009d83f3498623f685697826b68db7954a05d2867fca5f`。
- 两次预门禁失败均在生产前 fail closed：bundle 缺少基线 tag、业务上下文基线陈旧；修复后从最终 SHA 完整重跑通过。

## 隔离真机与浏览器验收

- arena-154 两个隔离 Panel、真实 Agent 与受控文件树验证双向配对、四种文件传输组合、冲突/撤销和 fail-closed；汇总 SHA-256 `7d266af4f1d9e26b3bf07340236a1741fab4c9168ca6129aa847e5336dcd2c99`。
- 撤销后新请求拒绝，汇总 SHA-256 `1c922c5c6c6ca4470c4ac8e741c88d51c009dbaccc74a0b70f8170b9a2ba30fd`。
- 正式 Google Chrome 151.0.7922.138 使用独立临时 Profile 验证壁纸选择/持久化、主题切换、四种文件传输路径、集群公开分享列表/卡片与 390px 布局；无意外控制台或 HTTP 错误，结果 SHA-256 `c77d7b2ad339d45d8abdd7eaea30feb762694cc7913cfeb776cceda212a38d7f`。
- 候选镜像 `sha256:d6db57f59e9777984c3f19b1e40946cf432b4183b0b35793443ee822bd3c1133` 的 version/revision 与冻结提交一致；全部隔离容器、Agent、网络、隧道和临时浏览器 Profile 已清理。

## GitHub、Release 与公开产物

- source tree：`1ae323528e61739c3a2f5685abcacfc7e05cfa4d`；annotated tag object：`f52ca2eb35af2bd0b4a6321210fddeaf4ee3035f`。
- 候选 CI `31921870922`、候选依赖治理 `31921870929`、主线 CI `31921976598`、主线依赖治理 `31921976608`：success，head SHA 精确匹配。
- Release workflow `31922164699`、tag 依赖治理 `31922164639`：success。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.79.0>，非 draft、非 prerelease，8 个附件完成公开校验。
- Docker `0.79.0` 与 `latest` OCI index：`sha256:a3902416e16df80c49cb94dc145b5b67f1fff9cbc37d014efa83acc406589518`。
- `linux/amd64`：`sha256:7a7308496e1aa2ddecfc91c17a157dda52504dfeab500c92f15341942b09e4a0`；`linux/arm64`：`sha256:896005850b30819ed8a63bb6bbb31ddb4624940a35b8f998ecf864a0901742d2`。
- 公开镜像重新拉取 E2E 通过，version=`0.79.0`、revision=`8ad781a2a6ef4d34adfbb09ba6092518fc13c9af`。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；与本版 packaging 契约一致，因此未制造空提交。

## 生产部署与健康

- 唯一 KPanel 生产目标：`arena-154`；`108` 按用户长期约束未连接、未测试、未备份、未部署。
- 部署前：v0.77.0，Panel healthy、restart 0、OOM false，Agent active；运行 OCI/revision 为 `sha256:072ede148231f4233d9fe849c0882340b1fcbed4bd327a13da830cacc058f378` / `dc0082a3493e257267f896d58860613aa75571bb`。
- 停写备份：`/root/kpanel-backups/v0.79.0-preupgrade-arena154-20260816T024731Z.tar.gz`，SHA-256 `fcf62f5fc0dc59be3a3f8ed8b4a27b27ba93268e064cf417e76350bb921b7cef`。
- 旧镜像归档：同名 `.image.tar`，SHA-256 `2ed6332f4d3b9eba5742f6e6e7995e8e9f58c7b3fafcb323c30d0ee654439dfc`；备份已独立解包并核对数据树、`.env` 权限、Compose、SQLite 和镜像可加载性。
- 标准更新入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`；更新日志 SHA-256 `66b721eb75643fd81a35eac65071f5ce95984691743612a1ad0c4d3823a844e3`。
- 部署后：Panel/Agent 均为 v0.79.0，运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active，systemd 无待 reload。
- Compose、Panel-Agent、SQLite、错误日志、回滚镜像加载和五次健康采样通过；采样时 Panel 约 75.31 MiB/256 MiB、7 PIDs。
- 备份恢复阶段曾发现本地 `latest` 标签已被公开镜像 E2E 更新，Compose 尝试将旧 Panel 重建为 v0.79.0，而 Agent 仍为 v0.77.0；版本门禁立即阻止启动。生产随即用精确 v0.77.0 镜像恢复 healthy，并改为启动原容器后重新完成备份。无数据损失，正式更新未绕过任何门禁。

## 回滚

- 源码/tag：`dc0082a3493e257267f896d58860613aa75571bb` / `v0.77.0`。
- 镜像 digest：`sha256:072ede148231f4233d9fe849c0882340b1fcbed4bd327a13da830cacc058f378`。
- 成套材料：上述停写备份、镜像归档和 metadata；恢复校验已通过，旧镜像已实际 `docker load`。
- 回滚步骤：停止 Panel 与 Agent，加载 v0.77.0 镜像归档，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `systemctl daemon-reload`，启动 Agent 与 Compose，再核对版本、Panel-Agent、SQLite、restart/OOM 和日志。
- 未触发正式回滚；回滚点保持可执行。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-16T10:02:45+08:00
- 候选冻结时间：2026-08-16T10:11:23+08:00
- 生产完成时间：2026-08-16T10:48:31+08:00
- 提交到生产用时：0.76 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与沉淀

- 跨 KPanel 传输依赖双方网络可达和权限有效；地址变化、撤销、旧版本兼容及冲突按现有 fail-closed 机制处理。
- 桌面跨窗口仍使用浏览器原生 DnD 拖影，视觉无法与同窗口 pointer drag 像素级一致；功能和落点已验证。
- 本轮复用 `release-kpanel` v2.4，没有新增重复工作流；新增的“备份恢复禁止依赖浮动镜像标签”经验记录在本验收文件，后续应在治理流程中单独评估后再固化。
