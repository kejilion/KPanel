# KPanel v0.80.0 上线验收记录

日期：2026-08-16（Asia/Shanghai）

发布级别：L3

发布工作流：`release-kpanel` v2.4

候选提交 / 标签：`88bdb3cf13e125acaba53ba5e2ff80a3af2cece7` / `v0.80.0`

上一生产版本 / 回滚点：`v0.79.0` / `sha256:a3902416e16df80c49cb94dc145b5b67f1fff9cbc37d014efa83acc406589518`

结论：通过

## 发布范围

- 桌面图标连续快速拖动时，以最新一次落点为准；旧请求迟到成功或失败均不得覆盖较新的乐观位置。
- 集群主机同步目标 KPanel 的安全入口路径；目标启用、禁用或重新启用入口后，来源端刷新会同步更新或清空入口。
- 集群“打开面板”使用目标安全入口、普通 HTTP 确认、`noopener,noreferrer` 和独立新窗口；目标端仍执行原有入口认证和限流。
- 修复跨站打开目标入口时 `SameSite=Strict` Cookie 被浏览器沿 303 跨站重定向链拒绝、最终显示 404 的问题。目标端先返回无脚本、禁止联网、`no-store` 的同源过渡文档，再进入 `/login`，未放宽 Cookie 或入口安全边界。
- 版本、CHANGELOG 和当前业务上下文更新到 v0.80.0；未纳入旧工作树、未提交草稿、重复补丁或不完整实验。

## 自动门禁

- Web：98 个测试文件、738 项测试通过；i18n 2254 条文案、20 个按页加载 catalog；typecheck、生产构建通过。
- Go：全包测试、核心包 race、vet、linux/amd64 与 linux/arm64 构建通过。
- 安全与供应链：govulncheck 可达漏洞 0、npm audit 0、Trivy source/image HIGH/CRITICAL/secret/misconfiguration 0；受限容器、安装安全、版本、治理、依赖、业务上下文、托管脚本和应用生命周期门禁通过。
- 最终 bundle：`kpanel-v0.80.0-88bdb3c.bundle`，SHA-256 `e4224cfeabb212e2dbf81254bd08a998f95586a8bbc310a609cfb52d3220490b`。
- arena-154 最终 L3：`L3 release verification completed`；日志 SHA-256 `baef81f6ed1ad109ca65c7a592571aca47a6c4e8c01d2c17dae21df24cc8c662`。
- 初次浏览器门禁发现真实跨站 Cookie/303 问题并在生产前 fail closed；提交 `88bdb3c` 修复后，全部自动门禁和真机门禁从最终 SHA 重新执行。

## 隔离真机与浏览器验收

- arena-154 两个隔离 Panel 与两个隔离 Agent 验证 v2 配对、安全入口路径同步、禁用清空及重新启用恢复；API 汇总 SHA-256 `277bd226d90b5e20e2dfd2342181f71e07a428896fafe13436930703c92b22ed`。
- 候选镜像 `sha256:b310a3c5f176a0f48b3a677bdd64262c41997d3befcfef9258a093e054fd4492` 的 version=`0.80.0`、revision=`88bdb3cf13e125acaba53ba5e2ff80a3af2cece7`。
- 正式 Google Chrome 151.0.7922.138 使用独立临时 Profile 验证两次快速拖动产生两次持久化写入且保留最新落点；集群入口只确认一次，精确请求 `/panel-v0800-test`，最终到目标 `/login`；`_blank`、`noopener,noreferrer`、`window.opener=null` 均成立，产品控制台错误 0。
- 浏览器结果 SHA-256 `7d3194edcce291cb8bea81630bdba2b78ee7996790f4c48b4835b1b0f6e2a07b`；桌面与目标登录截图 SHA-256 分别为 `97580dc43bbe3756b294897efefbb5d95cfe1fc48bc90a0e73ebd2c68fecd7da`、`8f2e948d42c76ccd414b9fce1463da39e1a7fac092aef9a2926e8fefaffd34d3`。
- 隔离 HTTP、初始未登录和无宿主 Agent 产生的 COOP/401/503 被单列为验收环境消息，不计作产品缺陷；业务旅程、API 门禁和目标登录页均独立断言通过。隔离容器、Agent、网络、隧道和临时 Chrome Profile已清理。

## GitHub、Release 与公开产物

- source tree：`46ffc8f4c28cf7bf25e38abeecbfee1560ac0ea9`；annotated tag object：`6e40d953ca57f604219664aad18644f482895ce0`。
- 候选 CI `31928373004`、候选依赖治理 `31928372980`、主线 CI `31928499907`、主线依赖治理 `31928499891`：success，head SHA 精确匹配。
- Release workflow `31928729174`、tag 依赖治理 `31928729308`：success。
- GitHub Release：<https://github.com/kejilion/KPanel/releases/tag/v0.80.0>，非 draft、非 prerelease，8 个附件完成公开校验。
- Docker `0.80.0` 与 `latest` OCI index：`sha256:206ebd3571432cd91ec15b6cd8285a199fcfa0f5f5d3a18e5edd217e430383ef`。
- `linux/amd64`：`sha256:a29247c9aa07822cb0e40dbb7ddb594ab9303bb153a51a994adaffeb50b21254`；`linux/arm64`：`sha256:acb4a62a0b41f80bc772cbd3c8196566192246aa12ea43f11a8f1b887dc2a861`。
- 公开镜像重新拉取受限容器 E2E 通过，version/revision 与正式提交一致；E2E 日志 SHA-256 `72b6d4ffd1e065c825a3334b5cb0c01598b0d7c4a6f688cd0a1229068bd7cef0`。
- `kejilion/apps main@6d86eee24a477320f4d8ffb32d9e85b785cf3c2c` 的 `kpanel.conf` blob 为 `34316059d4e42f527819bc7d56e0ff14ec434c96`；与本版 packaging 契约归一化后一致，因此未制造空提交。

## 生产部署与健康

- 唯一 KPanel 生产目标：`arena-154`；`108` 按用户长期约束未连接、未测试、未备份、未部署。
- 部署前：v0.79.0，Panel healthy、restart 0、OOM false，Agent active；运行 OCI/revision 为 `sha256:a3902416e16df80c49cb94dc145b5b67f1fff9cbc37d014efa83acc406589518` / `8ad781a2a6ef4d34adfbb09ba6092518fc13c9af`。
- 停写备份：`/root/kpanel-backups/v0.80.0-preupgrade-arena154-20260816T053021Z.tar.gz`，SHA-256 `8a4bed2cc1cd827c1fc3448ac2359805e8547e39eb379b3c1016c9df552503bd`。
- 旧镜像归档：同名 `.image.tar`，SHA-256 `e848736bb3505c794440804be2c92631a85548dc4085562374be8d842a950da4`；备份已独立解包并核对文件清单、`.env` 权限、Compose、两个 SQLite 库和镜像可加载性。
- 标准更新入口：`KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update /home/docker/kpanel/bin/kejilion.sh app kpanel`；更新日志 SHA-256 `ebdbf97f3da2104958cc0ea667f9ae8075b17c7fb03dd598ab22998e5c6a3fa8`。
- 部署后：Panel/Agent 均为 v0.80.0；运行 OCI/revision 与正式产物一致；Panel healthy、restart 0、OOM false，Agent active，systemd 无待 reload。
- Compose、Panel-Agent、SQLite、错误日志及五次健康/资源采样通过；Panel 稳定约 72.88 MiB/256 MiB、8 PIDs。

## 回滚

- 源码/tag：`8ad781a2a6ef4d34adfbb09ba6092518fc13c9af` / `v0.79.0`。
- 镜像 digest：`sha256:a3902416e16df80c49cb94dc145b5b67f1fff9cbc37d014efa83acc406589518`；本地恢复标记为 `kjlion/kejilion-panel:rollback-v0.79.0`，同时重新确认 `latest` 仍为 v0.80.0。
- 成套材料：上述停写备份、旧镜像归档、文件摘要和 metadata；恢复材料、SQLite 及镜像加载已独立验证。
- 回滚必须停止 Panel 与 Agent，加载并使用精确 v0.79.0 镜像，成套恢复 `/home/docker/kpanel`、`/etc/kejilion-panel` 和 Agent unit，执行 `systemctl daemon-reload`，再启动 Agent 与原 Compose，并核对版本、Panel-Agent、SQLite、restart/OOM 和日志；禁止只切换浮动 `latest`。
- 未触发正式回滚；回滚点保持可执行。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-08-16T11:52:46+08:00
- 候选冻结时间：2026-08-16T13:00:20+08:00
- 生产完成时间：2026-08-16T13:33:29+08:00
- 提交到生产用时：1.68 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

## 遗留风险与沉淀

- 集群入口仍依赖双方网络可达和目标安全入口有效；普通 HTTP 会继续要求明确确认，路径缺失时回退到目标根地址。
- HTTP 隔离环境无法应用浏览器对可信 HTTPS Origin 的 COOP 行为；该限制不影响本轮入口、Cookie、重定向和 `noopener` 的独立断言。
- 本轮复用 `release-kpanel` v2.4，没有新增重复工作流；跨站 `SameSite=Strict` 入口过渡与完整回滚材料记录在本验收文件中。
