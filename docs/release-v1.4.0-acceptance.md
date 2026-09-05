# KPanel v1.4.0 发布验收记录

日期：2026-09-05

发布级别：L3

候选提交 / 标签：`bfbd1bf0ff2bc0f6854fb4a522236911a6166615` / `v1.4.0`

上一稳定版本 / 回滚点：`v1.3.1` / `650f4db432d09f2251f8a03b6ac554d84e31d23b`

## 发布画像

- 业务域：集群管理/公开分享地球视图；轻量节点自动更新及旧运行时迁移；发布来源治理。
- 变更面：展示、只读状态聚合、轻量节点受管更新与 systemd 迁移、部署供应链固定来源。
- 受影响旅程：管理和匿名分享的地球/列表/卡片切换、地区与状态过滤、详情和未知数据；既有节点通过原定时更新器完成首轮迁移、后续升级及失败恢复。
- 未变化契约：数据库 schema、Panel/Agent API 和权限协议、Compose 端口、公开分享字段白名单、应用市场 `kpanel.conf`；没有扩大文件授权或新增 System Center。
- 风险等级：L3。节点更新会写受管二进制、权限与服务，要求真实 systemd、失败注入、双架构供应链及生产备份；不是仅凭 UI 测试发布。

## 发布范围与未纳入内容

- 新增集群和公开分享地球视图、筛选与详情，保留列表/卡片和绘图失败回退；未知指标/位置不伪造，待采样不计成异常关注。
- 修复旧更新器首轮 `root:root 0600` 凭据导致服务失败、旧文件 unit 网络族/条件迁移遗漏；保留节点身份、自定义 unit/drop-in 和最小权限。同步配套脚本，不要求正常可运行更新器的节点重新配对。
- 验收发现后修复窄屏英文按钮裁切、词典遗漏及 200% 字号指标布局溢出。
- 治理基线：`732eac2d11569d25b10f1914bfdb08549bce9a93`，只有 L3 来源准备入口、回归和提案三个文件；独立审查与候选/main CI 分别 `33948122793` / `33949783576` 首轮成功。独立 Dependency freshness 不适用，权威规则与触发范围已复核，不是虚报 PASS；必需 business-context freshness 保留。
- 产品精确提交（相对 `732eac2`，依序）：`80178a7331f9337f08a6807bf1503fa39590e6d9`、`e105ab3a7b35d51ccb9c0e4bbd0d6713dcf6572b`、`dbcdbdec95891a02568714f4879b7bb9cf1c4d55`、`b6741d9e1cc1a8d2756b75c40763a9f2d05465ac`、`a962e857a7024a575392bf2cf5d2c1b1a8ddbfff`、`61d815258501a6a2cd4108e725fee43c6842faf3`、`f2f21d82b261cef3628d0b59e2862d6abdb35b59`、`876ff14dffea904010ac35cc178eb30575144c0d`、`9c9965ee8d523dd64ba00af893115f958e196654`、`792d6d03d462a3b40da61ec292ef2c10c6bd3e8d`、`bfbd1bf0ff2bc0f6854fb4a522236911a6166615`。
- 原功能 `de79a4c` + `25ec8d8`、`46702d6` 及后续节点 `a2a66c9` + `ad164e5`、UI `4555da4` + `171d9e7` 完整重放，10 提交 range-diff 全为 `=`。组合独立只读终审 PASS；39 路径中 37 个与原修复逐字节一致，2 个仅发布说明，主发布复核一致。
- 排除：System Center 页面/路由/API/文档新增、所有未提交改动、后续 File/jobs/Compose/AI 候选、旧版等价残留分支。相对 v1.3.1 的 43 路径还包含上一版验收文档及上述治理 3 文件，不夹带其他业务。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 集群管理/匿名分享 12 场景、节点真实 systemd 三场景与两轮更新 | UI 为 mock API；不是所有真实配对节点已更新的证明 |
| 网络入侵与供应链安全 | 已验证 | L3/Release Go 可达漏洞、npm/Trivy、权限回归、公开资产摘要与 OCI revision | 不替代第三方渗透或长期安全审计 |
| 稳定性、失败恢复与兼容 | 已验证 | 旧更新器桥接、故障回滚/下轮恢复、旧 inode 恢复、绘图失败回退 | 停用 timer、节点失联、卡死旧锁不承诺自动恢复 |
| 性能与资源预算 | 已验证 | 浏览器峰 540.9MiB/106PID/OOM=false；6 次装卸、隐藏停止/显示恢复 | 有限窗口，无长期 soak/SLA 结论 |
| 用户体验与可访问性 | 已验证 | 中/繁/英、浅深、390/768/1280、100/125/200% 字号、键盘/焦点/长名 | 200% 为计算字号放大，不是原生浏览器页面缩放 |
| 数据、配置与迁移 | 已验证 | 合成节点身份/权限/inode 保留，生产配置摘要不变、SQLite ok、备份可读 | 自定义 unit 保留为源码与 shell 回归证据，未单独做自定义 unit 真 systemd 场景 |

## 自动门禁

- 最终 SHA 本地版本一致性 1.4.0、根/CN updater 模板检查、业务上下文 freshness、生产证据入口 4/4 和 diff-check 通过。
- 固定 Linux L3：`v1.4.0-bfbd1bf-l3-r1`，2026-09-05T06:52:34Z 至 07:04:29Z，首次完整运行 `passed/exit0`；Go 全量、135 文件 1165 前端测试、120 治理、root race、i18n 2176 短语/21 词典、双架构二进制、安装安全和 `app_conf_lifecycle` 全部通过。
- Runner `kpanel-release-gate:go1.26.7-node24`，immutable ID `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，Go1.26.7/Node24.20.0。唯一入口 `scripts/run-release-l3.mjs`，没有临时替代 wrapper。
- bundle SHA256 `70b636d9c2da7a924edea4c81f6c02faf36a9ad99d38dcf641bb6cbc9ca6b044`；plan `faf2d1a71585be36c88231568b49344d711ecf2f62e278c1ab42210392f7c287`；remote script `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 证据：`C:/GitHub/_release-artifacts/v1.4.0-bfbd1bf-l3-r1/remote-evidence`，对应 `/root/kpanel-release-evidence/v1.4.0-bfbd1bf-l3-r1`；远端 `evidence.sha256` 11 项全部 OK 后完整取回。
- 新治理首次真实产品来源准备 PASS/cleanup=removed；52 权威 tags、原共享 217 个 tags 与已有快照不变、HEAD/clean 不变。长期 5 次/14 天观察未完成，不以首例代替长期稳定结论。
- 候选 [CI 33951644190](https://github.com/kejilion/KPanel/actions/runs/33951644190) / [freshness 33951644151](https://github.com/kejilion/KPanel/actions/runs/33951644151)，main [CI 33951804575](https://github.com/kejilion/KPanel/actions/runs/33951804575) / [freshness 33951804528](https://github.com/kejilion/KPanel/actions/runs/33951804528)，均 exact bfbd、attempt1 success。
- [Release 33952076652](https://github.com/kejilion/KPanel/actions/runs/33952076652) / [tag freshness 33952076681](https://github.com/kejilion/KPanel/actions/runs/33952076681) 同 SHA 首轮成功。Release 源码/原生镜像扫描、runtime contract、多架构推送、latest 提升和发布均通过；发布工作流正常删除远端候选分支，本地候选与历史树仍保留。
- `govulncheck` 为 0 可达漏洞、3 个未调用模块层提示；npm audit 0，Trivy 源码/最终镜像策略通过。保留 npm glob 弃用和现有 jsdom 单测提示，不宣称所有上游信号为零。正式多架构构建 `provenance: mode=max` / `sbom: true`，两个架构的 attestation manifests 已公开可见。

## 依赖与技术栈变化

- 候选在线报告时间 2026-09-05T07:07:01.303Z，检测源 8/8 完整；日志 `C:/GitHub/_release-artifacts/v1.4.0-candidate-freshness-33951644151.log`。23 直接/基座行动项（6 patch/14 minor/3 major）、121 传递信号，emergency=0、需处理例外=0。
- 最近实际每日安全通告：[33951609449](https://github.com/kejilion/KPanel/actions/runs/33951609449)，2026-09-05T07:05:53Z，head `732eac2`，`security-advisories` job `101267347933` 实际 success，report 按条件 skipped；它不是 bfbd 的替代证明。bfbd push 的 security-advisories 按条件 skipped，最终源码另有 CI/L3/Release 安全证据。
- EOL 复核 current，最近登记 2026-07-28，下一到期 2026-10-28。行动项沿既有维护队列以首次完整发现计时：patch 7/14/30 天、minor 14/30/60 天、major/非 SemVer 基座 30/90/90 天（启动/决策/处置）；重复报告不重置期限。首次发现逐项历史不在本记录重造。
- 本版采用配套受管脚本 `a74495cc3ea4bac1b0b42bf572f0f122ba9e2cf1`；Dockerfile pin、来源文档、内嵌 updater 同源。Go/Node/基础镜像/Action/扫描器没有借本版机械升级；其余版本信号并非本批拒绝或无限暂缓，继续既有分级维护，由架构协调/依赖维护任务处理；2026-09-06 复核队列，超过原期限必须采用、证据拒绝或建立有期限例外，不能以本版重新起算。
- sh `linkage=coupled`；根脚本 SHA256 `d9e2a75d97e5914c2fb3d163122e025dee8e1c757ae8515cc34ba6991b319974`，CN `110d7e51080a5e07f05431e555006cefb5473899ec971b92ffd90a311d5d9cfd`，两仓 main 与公开原文核对，不改脚本版本号。其真实迁移/失败恢复、模板一致性、10 项 shell 回归和安装 smoke 通过；sh 无 push CI，未虚报 GitHub CI。

## 隔离真机与浏览器验收

- 策略：`environment-policy.json` schemaVersion1、`arena-154`，只使用其允许的隔离候选/浏览器/性能用途；测试与发布串行。Linux amd64，真实 systemd 255/Ubuntu24.04；测试下载为受控离线夹具、合成身份，不接真实用户节点。
- 最终 systemd：`retest-bfbd1bf-r1`，2026-09-05T06:49:16.248Z 完成，normal/permissions/rollback 三场景 PASS；证据位于当前工具用户证据区的 `light-systemd-d2d7c238-20260905T125052/retest-bfbd1bf-r1`，远端父目录 `/var/tmp/light-systemd-d2d7c238-20260905T125052`；保留原夹具与断言及原始失败日志，不在仓库硬编码本机用户目录。
- 最终源码归档 `8f530b70ef2ef3609e218941dd536d4e5c3e24b9bc6c8297d3928954cc23311a`，专项 binary `de455734359ef2d47f68917c436e4676dda69c408ab9e710e46637a65e897803`，summary `ea879e39d56743e66f8badfbfac17e552ffb743cca6514e00582a527bd7fd77b`，cleanup `94f0d8c981c73ecf646890eac0e3ce9ea2751f3815ba1a7894cd15a67e19254e`。代码与 ad164e5 相同所以 binary 同摘要，实际重新执行，不复用旧终态。
- 验证首轮重启前 root:node0640、身份/配置字节/inode不变、真实父 PID/timer、两轮升级、精确历史 file unit 修复与 AF_UNIX/INET/INET6 实际 socket、失败返回并回滚后下轮恢复、旧 inode 恢复。Go23 测试/子测试通过，namespace 真执行，helper 顶层 skip 为设计用途。runtime 限 768MiB/1CPU、build 1GiB/1CPU；四个专用宿主 PID/容器/镜像清理，保留证据和共享 BuildKit cache。
- 最终浏览器 job `arena-154-1114523`，2026-09-05T06:34:46.318Z 至 06:38:00.960Z，12/12/exit0。证据 `C:/GitHub/_codex-tmp/kpanel-v140-browser-20260905/final-bfbd1bf-r1`；36 PNG、12 trace；主发布重算摘要并检查窄屏英文和 200% 中文截图。
- Chromium140.0.7339.16/Playwright1.55.0，固定镜像 `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`；source 同上、dist `be509c0191a2816b99af12b98c8ac1a0cbd7bb76d557183897fd78c9ba617535`；command spec `f9ac1e96ef6f912cfc5f3424e899238385f60e8efd5cb34a06d6506a647da7e5`，result `0b0c3df863bde7b5df523cbba78dae06ee21db9d41142d3053673ca5769248c4`，identity `c8b8267fccb1d7bee10ce654ff4d35d5bf603277b6187254529ca94564ee66a5`。
- 管理/分享各 1280 dark zh-CN100、390 light en100、768 dark zh-TW125、390 light zh-CN200，另两端绘图失败回退及生命周期，共12场景；筛选/长名/未知值/详情刷新/键盘/实际 Canvas draw 停止与恢复均验证。英文 Access authorization 计算14px、高40px、client=scroll=195；200% 两端 metrics client=scroll=332。匿名无管理字段/cookie0，console/pageError 共同收集器均0。
- 页面来自精确候选构建，`finalProductEvidence=true` 仍明确 **UI MOCK ONLY**；200% 是计算字体放大，不是 native zoom。硬 1536MiB/1CPU/256PID、network none/mounts0，600秒 inner/660秒 outer watchdog；峰540.9MiB/106PID、OOM=false、容器清理确认不存在。
- 首败保留：旧候选真实 systemd 暴露 0600 与 AF_UNIX，第一修复仍漏 terminal.json 历史条件，完整修复 ad164e5+a744 后通过并在 bfbd 重跑；早期 UI 有9通过/3真实布局失败，修复后12通过并在 bfbd 重跑。产品缺陷被正常门禁挡住，不写成“全过程首轮通过”，也不算生产回滚。
- 未执行：真实用户全节点迁移、完整浏览器人工/native zoom 笛卡尔矩阵、长时 soak、第三方渗透；本版风险用上述精确边界覆盖，不虚构双节点/长期证据。

## 发布产物与公开仓库复核

- [GitHub Release v1.4.0](https://github.com/kejilion/KPanel/releases/tag/v1.4.0)，ID383168080，2026-09-05T07:23:43Z 公开，draft=false/prerelease=false，并为 GitHub Latest。annotated tag 对象 `3e1f00514631a5d48d2fa5a1bd884b12acc2dcd5`，peeled exact bfbd。
- Docker `1.4.0` / `latest` OCI index 同为 `sha256:9f70d341db62e16376f281b301c4031fc1bfb92e94b289b2c20facd9a764137b`。
- amd64 `sha256:a84ba028ddf3ed1ef72df8d9619bbd48ed4f7b552b0ba426e40bb4c494b946a5`；arm64 `sha256:29d13d0008b82ae2caa52c689dbca59b40730395f53b11594974aeb25afc83fd`。公开镜像实际拉取，标签 version1.4.0/revision bfbd/script a744/d9e 均一致。
- 8 个附件完整下载到 `C:/GitHub/_release-artifacts/v1.4.0-public-assets-20260905`，`SHA256SUMS` 列出的5件全部核对：agent amd64 `d0cc5437bdd545be1255a686d246f62f4fce2dba0cc10bb3e2af904a24dc162b`、agent arm64 `a94835499facbc015cfd06e18d8f1fcce9840b1d664e0e7705b1bbb12cf28b00`、node amd64 `6c91acadf01396268734faa8f0f8be5c4d6989aa8d0d75dcd5df36e2b1cba4f6`、node arm64 `299dc68611921c3c23ef5dd8b2b453f3c5809baa2fa62dabc04223ccc86af962`、deploy tar `3692b87394e49ffc162d04fd76760f813b65e259d8e3dbd0593a4dfc37acccd0`；LICENSE/NOTICES 不在该5件清单内，不能声称8件都有清单哈希。
- 公开 node amd64 在非root65532、network none、readonly、无生产挂载、128MiB/1CPU/64PID 的固定 Runner 容器实际输出 `1.4.0 light-v1`，退出即清理；未在宿主 root 直接运行潜在迁移入口。arm64 实际运行未验证，仅构建/公开摘要通过。部署 tar VERSION=1.4.0。
- 公开镜像 `packaging/tests/image-e2e.sh` 首轮 `image_e2e=pass`/exit0，先核对18080未占用；健康/首页/代理头/bootstrap201/Secure cookie与单网络通过，专用容器/网络已确认清理。日志 `C:/GitHub/_release-artifacts/v1.4.0-public-image-e2e-20260905.log`。
- apps 契约对 v1.3.1 不变，远端 `kejilion/apps` main `2d8044adec98e3eb16f47cdbb297f6be9632a66f`，kpanel.conf blob `abf0efd22876f34aa3731f5b6d8ba04e373b965e`；无需新增 apps 提交。sh 联动已发布 a744、无版本号修改；默认 raw CDN 曾短暂旧缓存，待原文收敛后再发布镜像，没有改源码绕过缓存。

## 生产部署安全核对

- 用户授权本轮候选上线；唯一正式目标 `arena-154`。本次未连接、备份、部署、升级或只读核对 `108`/`prod-108`；后者所有 KPanel 用途 disabled。
- 固定入口 `scripts/run-production-evidence.mjs`，共同 run ID `v1.4.0-production-20260905`；本地 `C:/GitHub/_release-artifacts/v1.4.0-production-20260905-{preflight,backup,postdeploy}/remote-evidence`，远端 `/root/kpanel-release-evidence/v1.4.0-production-20260905/production-{preflight,backup,postdeploy}`。
- preflight 07:17:00Z→01Z 首轮 passed/exit0，实际旧版1.3.1/revision650f、running/healthy、Agentactive、restart0/OOMfalse，SQLite panel/ai.db ok，根ai.db为既有empty。
- backup 07:56:35Z→45Z 首轮 passed/exit0；已短暂停写并恢复旧服务，protected SHA 不变。备份 `/root/kpanel-backups/pre-v1.4.0-20260905T075635Z`，tar可读、旧镜像可load、SHA256SUMS全OK。备份清单 SHA256 `e7f0b2aed2a7969cdf57c1743542a2758000d9962bbea2c17ef929af271c61c2`，应用归档 `d5962195fd701353f54630c38ae6de61df74479dd68ec4053960517b441affba`，旧镜像归档 `17e8bda4cf35d630117105db8e108c16c44069434b8d92666c4c65d827cf9d19`。
- 部署仅 `ssh arena-154 env KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`，exit0/Update Complete；拉取上述 latest digest、标准容器重建和访问策略恢复。
- postdeploy 07:59:19Z→20Z 首轮 passed/exit0，Panel1.4.0/running/healthy、OCI revision bfbd/index9f70匹配，restart0、OOM=false；Agent loaded/active/running/enabled、NeedDaemonReload=no；protected.diff为空，SQLite同前ok/既有empty；近10分钟日志无fatal/panic/OOM签名。
- 公网 HTTPS `https://kpanel.154.36.153.9.sslip.io/` 返回200，`/api/v1/health` 实测statusok/version1.4.0/protocolv1alpha1。没有清缓存掩盖版本。
- 生产写操作仅标准停写备份/恢复、应用市场更新和取证。备份恢复的健康轮询曾短暂curl56，正常有界就绪循环内收敛；更新期间unit changed提示在标准流程内完成reload，postdeploy为no。两者没有造成步骤失败或重跑。
- 故障注入、回滚验证、node迁移实验仅在隔离环境，不对真实配对节点额外执行重配对或强制更新。

## 回滚

- 固定源码/tag：v1.3.1 / `650f4db432d09f2251f8a03b6ac554d84e31d23b`；旧 OCI index `sha256:3cf4692374f8f09da3bffe0f5e88688e866ebf205bef8de9180d02af424ca87f`。
- 当前备份含旧镜像、数据/Compose/.env/agent.env/受管脚本、Agent unit 和 kpanel.conf；恢复采用上述精确旧镜像及本版停写备份，再固定入口核对健康、Agent、配置摘要、SQLite、日志和公网，禁止只切浮动 latest。
- 未执行生产回滚；实际生产1.4.0健康。GitHub Latest/Docker latest/标准应用更新入口均对齐1.4.0，无退化，公共默认通道回退不适用。若以后有退化，分别明确真实生产回退与公共默认通道决策，不以只恢复一台主机冒充撤回公开版本。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-05T11:28:18+08:00
- 候选冻结时间：2026-09-05T14:29:55+08:00
- 生产完成时间：2026-09-05T15:59:20+08:00
- 提交到生产用时：4.52 小时
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间取实际纳入的原功能46702d6提交时间；冻结取最后bfbd提交时间，之后产品源码未变化。此处产品变更失败不包含上线前正常门禁拦截或下列执行流程问题。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：9
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/verify-change/role-mismatch",
    "position": "before-production-write",
    "count": 2,
    "impact": "初始管理树误传require-clean、独立验证clone误用writer，正确门禁拒绝两次，不构成候选成功证据。",
    "recoveryEvidence": "既有发布任务执行记录及v1.4.0-plan-20260905历史段；linked候选与auto角色分别正确验证，最终bfbd L3/CI通过。",
    "permanentAction": "发布负责人使用既有auto角色入口，不绕过角色门禁；架构协调于2026-09-06复核本轮参数记录，退出条件为后续一次正确角色预检且不误传选项。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/dependency-report/invalid-entry",
    "position": "before-production-write",
    "count": 1,
    "impact": "初始误用不存在的dependency checker，该尝试无效。",
    "recoveryEvidence": "原发布任务保留错误输出，改用仓库真实入口；最终候选33951644151完整在线报告8/8。",
    "permanentAction": "发布负责人先从Makefile/rg-files解析唯一入口再执行；架构协调2026-09-06复核，退出条件为下一次依赖检测使用已存在入口并有完整报告。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/run-repo-bash/checkout-crlf",
    "position": "before-production-write",
    "count": 1,
    "impact": "早期根/CN检验因Windows checkout CRLF产生Bash语法失败，不是源业务断言通过。",
    "recoveryEvidence": "机械LF规范化后原根/CN syntax、安装smoke及后续L3通过，源码及公开脚本摘要保持一致。",
    "permanentAction": "由发布负责人使用仓库run-repo-bash/归档Linux源路径，架构协调2026-09-06复核受管脚本行尾合同，退出条件为后续根/CN原断言首次通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/background-browser-test/scale-fixture-loop",
    "position": "before-production-write",
    "count": 2,
    "impact": "初始r1/r2持续MutationObserver字体夹具造成点击超时，均不采纳为有效通过证据。",
    "recoveryEvidence": "r3使用显式放大检查点并保留原断言，真实9pass3fail后修复171d9e7；最终bfbd原12矩阵12/12通过，原失败包保留。",
    "permanentAction": "浏览器验证负责人保留显式检查点夹具及断言，禁止连续DOM放大观察器；退出条件已由最终bfbd实际执行和无超时证明，2026-09-06复核复用边界。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/light-node-runtime/sandbox-template-rewrite",
    "position": "before-production-write",
    "count": 1,
    "impact": "新增shell回归首遍沙箱路径替换不适配转义正则，夹具构建失败。",
    "recoveryEvidence": "a744修为只处理受管canonical模板，原断言10项通过；历史unit fixture哈希未变，最终独立systemd原三场景重新通过。",
    "permanentAction": "a74495cc3ea4bac1b0b42bf572f0f122ba9e2cf1固定canonical-only夹具构建，保留失败及原断言，不改实际历史unit来适配结果。",
    "historicalReleases": []
  },
  {
    "fingerprint": "governance/candidate-admission/inapplicable-freshness",
    "position": "before-production-write",
    "count": 1,
    "impact": "把纯治理732的未触发独立Dependency freshness误列阻断，额外浏览器dispatch尝试受控失败且未点击最终Run，延误准入。",
    "recoveryEvidence": "PROJECT_RULES295-299、project-management215-218/314-315、机器门禁和路径触发经协调及独立复核裁定not-required，撤回手动请求；实际必要CI/business freshness通过，最终产品适用freshness完整执行。",
    "permanentAction": "架构协调与发布负责人已消歧本次准入记录，禁止用泛化措辞新增不适用门禁或虚报PASS；2026-09-06检查下一批画像，退出条件为按文件范围明确required/not-required。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/verify-change/machine-specific-evidence-path",
    "position": "after-production-write",
    "count": 1,
    "impact": "生产已健康完成后的验收文档首轮verify-change发现硬编码本机Windows用户证据目录，未提交或推送，不影响生产或正式产物。",
    "recoveryEvidence": "原首轮verify-change输出保留于发布任务；改为可移植证据区相对定位及同一远端父目录，重新运行原入口和指标/覆盖校验。",
    "permanentAction": "不改或豁免现有check-governance-consistency的machine-specific路径禁止规则；发布负责人在生成验收证据引用时使用相对证据区/远端run目录，并在提交前运行现有唯一入口。架构协调2026-09-06复核，后续L3前必须确认未复发。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

计数基于本次任务已记录的无效必需验证/误阻断事件：角色两次、错误入口一次、行尾一次、浏览器夹具两次、shell夹具一次、误扩大治理要求一次，以及生产结束后验收引用不合规一次；普通只读搜索/补丁定位失误不计，业务缺陷被有效门禁发现不自动计。备份/部署/postdeploy没有失败或重跑，生产后的文档拦截不等于产品退化或回滚。

过去 v1.2.0/v1.3.0/v1.3.1 重复 `l3/run-release-l3/local-tag-mismatch` 本轮未再次触发：在生产写前由独立治理732修复唯一来源准备入口、25/25独立回归与120治理/真实Windows准备通过，最终bfbd第一次完整L3实际通过。旧失败fixture被系统阻止清理后仍保留，不宣称整机零残留，也未绕过删除限制。

## 遗留风险与后续准入

- 已实现未实机验证：所有真实用户节点的迁移完成率、离线/禁用timer/旧锁卡死自动恢复、原生浏览器zoom全矩阵、自定义unit专项真systemd；它们没有被本版证据泛化覆盖。
- 已验证：在声明的旧版本/原更新器/合成身份/真实systemd条件下自动迁移、权限/身份保留和回滚；正常已配对节点不因本次发布要求重配对。公开资产下载已独立完成，不把离线curl夹具当公开下载。
- 不阻断本版：独立精确组合复核、最终SHA浏览器/systemd/L3/CI、公开产物/E2E及生产三阶段证据全部通过，实际生产健康，无System Center/未批准业务夹带。
- 下一批File/jobs/Compose/AI仅进入各自候选准入与新组合验证，不因本版成功自动发布；资源由协调按顺序移交。本次不修改其工作树，不执行电脑睡眠。
