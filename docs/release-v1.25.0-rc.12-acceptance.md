# KPanel v1.25.0-rc.12 发布验收记录

日期：2026-10-08

发布级别：L3。产物已发布，生产未部署。

候选提交 / 标签：`fd2a243c12c9f9434da409408098135c8a23b3f9` / `v1.25.0-rc.12`；源码 tree：`235b47c1b859677160c80d0bee16ef4950362c15`。

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。

`releaseChannel=preview`；`releaseTrain=1.25.0`；仅提升 `preview`。公开时间：`2026-10-07T18:00:47Z`。

## 候选分支与发布后处置

`release/v1.25.0-candidate` 保留在上述精确产品提交，供同序列后续预览使用。五条来源分支在公开产物和 E2E 成功后按原始 tip 归档；14 个源提交的重放映射已核对。归档使用 Git 本地事务的 expected-old-SHA，未覆盖既有归档，未物理删除工作树。原作者已交付/停止，所有来源在处置前重新核对 clean、分支和 tip。

| 原分支 | 原始 tip / 归档 SHA | 归档 ref | 本地与远端结果 |
| --- | --- | --- | --- |
| `codex/desktop-file-preview-no-directory-flash` | `ab01f5988d633bf32f937b32dfd1584d0d49cb19` | `archive/codex/desktop-file-preview-no-directory-flash` | clean、detached、原工作树保留；远端 source/archive 均不存在 |
| `codex/desktop-video-autoplay` | `50e1d14bddf5538dd2787fc528d3a33fb28d987f` | `archive/codex/desktop-video-autoplay` | clean、detached、原工作树保留；远端 source/archive 均不存在 |
| `codex/modal-close-maximized` | `5b0683320b3b5adaf8f54fb1948763a7c77de669` | `archive/codex/modal-close-maximized` | clean、detached、原工作树保留；远端 source/archive 均不存在 |
| `codex/mobile-widget-page` | `e36a519658b5699fe4b2184b2cbac1a05312b920` | `archive/codex/mobile-widget-page` | clean、detached、原工作树保留；远端 source/archive 均不存在 |
| `codex/compose-project-delete` | `1ee4102e9071a0190ee55ef9fbdda8f6a2e6dda8` | `archive/codex/compose-project-delete` | clean、detached、原工作树保留；远端 source/archive 均不存在 |

来源本来均未推送：远端 active/archive 均不存在，不制造远端来源分支。原分支本地活跃 ref 已移除，旧 branch config/upstream 按精确分支解除；保留的 detached 工作树可从归档恢复到新任务。处置原件：`source-archive-results.json`。

Docker 批处理按用户原话“Docker 批处理 不汇入预览版”排除。实际观察到既有 `archive/feature/docker-container-batch-actions` 为 `dca6305b9d631aeea5b12eebffa6b6752b731d5a`；本次收尾未更改它，也不把先前归档算成本次执行。未知归档执行者不作推断。无其他来源搭车清理。

本验收记录在独立 `docs/release-v1.25.0-rc.12-acceptance` 候选完成，仅改本记录和当前业务事实入口。其精确提交在候选 Linux CI、主线快进及同 SHA 主线 CI 成功后按 10.2 归档到 `archive/docs/release-v1.25.0-rc.12-acceptance`；最终 SHA、CI 和引用收尾原件保存在仓库外 `docs-closeout-result.json`，避免为记录自身 SHA 反复改动已公开产品。该文档候选不打产品标签、不制造新版本。

## 发布画像

- 业务域：Docker Compose 项目管理、桌面文件预览与窗口、手机桌面组件页。
- 变更面：展示、只读和管理员主动触发的宿主机写入；不变更安装、升级、公开端口、数据库格式、Agent 权限模型或标准应用市场入口。
- 受影响旅程：删除 Compose 项目与保留/恢复配置、默认数据保留/显式卷删除、文件快捷方式加载和失败重试、视频播放、最大化窗口关闭/取消、手机组件页滚动。
- 风险：L3，Compose 涉及容器/网络移除、用户选择的数据卷删除和配置归档；破坏性场景使用 arena-154 的独立 fixture 与哨兵数据。
- 单管理员控制面和宿主资源真源沿用；预览禁止生产部署，prod-108/108 禁止全部操作。

## 发布范围与未纳入内容

1. Compose 新增“删除项目”。默认停止并移除容器及非外部网络，保留数据卷、bind mount、外部卷和镜像；配置文件重命名备份。可选择保留配置，删除项目数据卷必须单独勾选。容器内未持久化数据随容器删除。
2. 桌面文件快捷方式显示打开进度或可重试的错误，加载/错误/重试不闪现目录。
3. 视频预览在浏览器政策允许时自动播放，手动播放、Space/F/Esc 等原有操作保留。
4. 最大化弹窗关闭动画和取消确认保持原尺寸，真正关闭后重置，重新打开恢复初始尺寸。
5. 手机负一屏隐藏分页控件和占位，小组件按内容增高并保留阴影留白，纵向滚动与返回图标页的手势交接。

Docker 批处理、未完成备份路径工作、其他新功能、生产部署均未纳入。依赖/工具链/Action/脚本基线未升级；版本字段和 Changelog 同步为 1.25.0-rc.12。

| 原始源提交 | 整合提交 | 重放等价性 |
| --- | --- | --- |
| `ab01f5988d633bf32f937b32dfd1584d0d49cb19` | `15b3ef22e6a4a4daeba9802ac5b037f9addf3f27` | 相同 patch-id：`f5c90e5d46df31095a1d6bc7bfb7c8b27e0ad547` |
| `50e1d14bddf5538dd2787fc528d3a33fb28d987f` | `0c68fad19c71b61de4b3da9715694931f2227994` | 相同 patch-id：`60d975e50b0429d6f6e118cc66ea053c475b0997` |
| `5b0683320b3b5adaf8f54fb1948763a7c77de669` | `e59378eec6c61d4f3d4fe61ce1c26d74ec6a8c85` | 相同 patch-id：`3e0c7c8bbaa83e49a43ac5c7253354e4cba72ec5` |
| `82215bf96fae8ae605cea0b508d29946c452fa01` | `feb6b435167a3706bf40a47d2eac99eeadeebe50` | 相同 patch-id：`1bcc9d5f6f78845a25d3bc0849824f2424f3127d` |
| `82ce246f9edaed6b6505cb4996bdc60c3f124704` | `5ac1ece085d2b6b11133a7b8a31bb8f6973f7340` | 相同 patch-id：`4e319d6a421013d9d54237b7642349aa70c8e6e6` |
| `e36a519658b5699fe4b2184b2cbac1a05312b920` | `510a02c6b1cc39dfaf6232b5a540b9155e434ac0` | 相同 patch-id：`d44e04276abe441b798c4b5eaea71cd1d439069d` |
| `a42807c306fda0fe4023a9c5d73362cae53203b5` | `659a108d8cd30c6ad35dfbe7fa97bc64fa705da7` | 相同 patch-id：`343aef121e2627866d90c4cefe814a7f8fc50c34` |
| `cf977e02628e9427ba8b3f39d80d2bd318fe2577` | `f870d58a32fb3e9f961b4bb2c24dec290332c810` | 相同 patch-id：`3328840439eceb09fb3d0488f905bf597f41a0d6` |
| `1375f4dfef18bb91766490c5bed71657754c28b6` | `24df7e45f48ed67b7ab1ca4604df0752e0baba44` | 相同 patch-id：`b4ad3ec6a68382e9cb49970af680a0218ccf5e52` |
| `c0d61b3f384981cfd0d98862a33b40e60401724e` | `c134fa88796305c2b506ff9d2e810a6c20dc1670` | 相同 patch-id：`a04eff1bbcd16d1c18f8d948eca255f367ef0beb` |
| `09d13ce9d3ee4dbcd1b8db05b4cc75e4d564d22b` | `7034aa29914bf0174d2e00f6fae5079d02017917` | 空的复核提交，保留来源 trailer |
| `10c67c8fd43adb4f8f2016ec4d24cc03f5d0c62d` | `380720acba2a9f86837b3e3aebeee262d9c5ffa5` | 相同 patch-id：`43605903a6f2d6298e836486c8ee88301f2af54f` |
| `88a640539877b5499471fba4c2a42dfc24aec12f` | `c0dbdcca2deae7ba9525d996b5c53101eaaa0a0a` | 相同 patch-id：`b084cbdb905628405dd400ea221bd08c9f105f03` |
| `1ee4102e9071a0190ee55ef9fbdda8f6a2e6dda8` | `9bce646f1a4eff5323b9dea9085e98076de1c345` | 空的复核提交，保留来源 trailer |

## 外部审计与修复交付

- CF scoped run19：源码 `9bce646f1a4eff5323b9dea9085e98076de1c345`，四条新增 Compose 边界提交；只读 source-only；专用代理及子代理显式 `gpt-6-luna/max`。固定 skill 提交 `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`，源码无改动。
- 状态：`incomplete`、`scope_complete=false`、`coverage_counted=false`。1800 秒预算耗尽、critic 受平台线程限制，限时文档收尾又解析失败。覆盖账本、正式 reports、两份 skill validator logs、独立记录核验及完整调用账目缺失；不声明整轮通过或安全洁净。
- 一条 `ai.compose-remove.hidden-destructive-option` 线索由不同独立验证者否决：未证实低信任输入链或跨项目卷影响，执行绑定管理员明确审批的完整参数，截断提示可见。它不是 surviving confirmed/needs_validation finding；真实 Docker 正常验收不是 CF 动态验证。
- 成本：开始 2026-10-07T15:42:07Z，结束 16:17:50Z，墙钟 2143 秒包含截止后的收尾；观察到至少 7 个子代理，完整 agents/tokens/cost/platform-wait/阻塞时长 unreported，不把并行墙钟当发布阻塞时间。
- 覆盖检查：`decision=ok`，未审计边界提交 36，最老 3 天；最近完整 run4 为 16 天，阈值 30 天。11 份元数据 validate 通过；run19 及其他 incomplete run 不计覆盖。RC 只记录，稳定版仍须按原规则 `--require` 重查。
- OCR：工具 1.12.11，自由臂先于本轮规则/preview；此前已读原作者 OCR，blind=false。20/20 reviewable 文件已审查，总差异 24；4 个 Markdown/锁文件按各自 unsupported 或显式 exclude 原因排除。有效 H0/M0/L0；constrained-only unreported。17 个产品/测试文件按实际 Git blob 与源交付等同，最终提交新增文档/audit 元数据另记 docs-only skipped，不捏造独立提供商覆盖。
- 公开摘要：`.governance/security-audit/run-19/run-metadata.json`；候选详情和验证对话仅保存在私有外部证据目录。本版没有宣称新的安全修复已完成。

## 跨仓库联动判定

- `scriptLinkageState=not-required`（无需发布脚本（不适用））；变更集编号和新脚本候选：不适用。
- 实际内置脚本 commit：`c3a8bd895f8878d9e4ced7592c91a20c974472a5`；SHA-256：`d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0`。根 Dockerfile 与 Node 更新 source.json 一致，公开双架构 OCI label 复核一致。
- 包内/标准 apps 配置归一化 LF 后相同：`f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`；apps 干净基线：`4fc985e964dbd1742143521d0a28ab652b885773`。
- 五项差异复用已有固定 CLI、安装、更新和脚本契约；本版不产生脚本/apps 提交，不硬编码 RC 或切换标准默认源。完整脚本双向实机闭环未执行，原有 app-conf 生命周期与 managed-script 固定门禁通过。

## 多维质量结论

状态仅描述本次范围及所列环境，不能推为全部平台或生产已通过。

| 维度 | 状态 | 证据 | 未验证边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 真实 Compose 生命周期/数据结果、公开镜像 E2E、源等价 UI 回归 | 完整 kejilion.sh 双向实机闭环、自定义 Docker socket/context 未验证 |
| 网络入侵与供应链安全 | 已实现未实机验证 | L3/Release 扫描、固定镜像与 script pins、受限非 root runtime、OCI provenance/SBOM | CF run19 incomplete；不声称整体入侵边界通过 |
| 稳定性、失败恢复与兼容 | 已验证 | missing-image 失败注入、配置/env 恢复、stale-version、取消/失败/重试、核心 race | 无长稳 soak、真实浏览器策略差异或所有 daemon context 证明 |
| 性能与资源预算 | 已验证 | 源码分组2、Go GOMAXPROCS=4；Release 256 MiB/1 CPU/128 PID contract | 不是长期资源趋势、吞吐或大型项目性能基准 |
| 用户体验与可访问性 | 已实现未实机验证 | 九组浏览器、14px 单行/边界、Tab/Space/Esc、实际 mock DOM 与源 frame 证据 | 真实 touch、native 125%/200% zoom、Safari、脏编辑 native confirm 未完整验证 |
| 数据、配置与迁移 | 已验证 | named/anonymous/external/bind 哨兵保留、显式项目卷删除、归档原字节和恢复 | 容器内未持久化数据不保留；无数据库迁移 |

## 自动门禁

- 完整 L3 唯一外层入口：`node scripts/run-release-l3.mjs`，run `v1.25.0-rc.12-fd2a243c-l3-r3`，候选 `fd2a243c12c9f9434da409408098135c8a23b3f9`，base main `9b1024acc2f9d66e611b54fdd8a6bd5e5bdc5a41` / v1.24.0。2026-10-07T16:36:14Z–16:51:10Z，passed/0。
- 固定 Runner：`kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`；Go 1.27.1、Node 24.21.0、npm 11.19.0。
- 分组耗时：Web 578466 ms、Go 536392 ms、deploy 4264 ms，源码分组总耗时 578507 ms，jobs=2、GOMAXPROCS=4；前端 259 文件、2355 passed、6 skipped（2361 total）。core privileged race、vet、场景包、构建、install safety、app-conf lifecycle、update backup parity 通过。
- bundle SHA-256：`8ac849b42f3c5a7ca343ff67801bac4d58cd3e4060cdfa361b47bb1865eca44d`；plan：`b6fe39b60445bfe8ef393164b1ca58139d50d22595a1a5996153f1bf05189a2a`；受检远端脚本：`21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。12 个远端证据文件和本地输入摘要复核一致；原完整 log：`1ae3d0456d37523c6c86cd7f2865c9496dec978c18089c7a6d4de1e624bbfe84`。
- r1/r2 失败原件保留：b9c82ac6-r1 isolated fetch timeout；r2 业务事实基线 54>=50 被拦截。更新现有业务事实后冻结 fd2，阈值未更改；r3 完整通过。未把前置失败算作产品测试失败，也未沿用旧 SHA L3。
- 候选 CI：[CI 37656540164](https://github.com/kejilion/KPanel/actions/runs/37656540164)（2026-10-07T17:06:52Z–2026-10-07T17:15:47Z，success）；[Dependency freshness 37656540274](https://github.com/kejilion/KPanel/actions/runs/37656540274)（2026-10-07T17:06:52Z–2026-10-07T17:07:27Z，success）。
- 主线 CI：[CI 37660480983](https://github.com/kejilion/KPanel/actions/runs/37660480983)（2026-10-07T17:37:17Z–2026-10-07T17:43:45Z，success）；[Dependency freshness 37660480876](https://github.com/kejilion/KPanel/actions/runs/37660480876)（2026-10-07T17:37:17Z–2026-10-07T17:37:51Z，success）。
- Release workflow：[37662285755](https://github.com/kejilion/KPanel/actions/runs/37662285755)，2026-10-07T17:51:16Z–2026-10-07T18:01:05Z，success。
- govulncheck：0 reachable vulnerabilities；另有 1 个 required module advisory，当前路径未调用，不能写成全部模块风险消除。npm audit 0，Trivy 受检 source/config/native-image 范围通过。Release 固定 Action SHA、原生 runtime 256 MiB/1 CPU/128 PID、65532:65532、read-only、cap-drop ALL、no-new-privileges 门禁通过。

## 依赖与技术栈变化

- 独立 `make dependency-report`：本次未生成；检测源完整性未验证。最近每日通告/EOL 复核未在本次另行执行，不据全绿 CI 推断所有行动项已关闭。
- 候选/main Dependency freshness 同 SHA 均通过；安全扫描与上述未调用模块 advisory 按实际范围记录。工具链、基座、Action、扫描器和 managed script 均沿用已发布 pins，本次仅 package 版本字段变化。
- 无新增依赖升级或传递依赖处置决定；不把 version lock 变更描述成依赖升级。模块 advisory 与稳定版 CF 准入仍由后续既有规则处理。

## 隔离真机与浏览器验收

- 唯一主机：arena-154，Debian 13.7 trixie、x86_64、Python 3.13.5；环境策略 candidate-validation / browser-validation。未启用 WSL 灾备，未连接 prod-108/108。
- 真 Docker：`true-compose-9bce646f-r1`，生产代码与 fd2 等价。现有 `TestComposeLifecycleAgainstDocker` + 仅 fixture/assertion 的外部 Go overlay，固定 Runner、Compose v5.3.1 插件 `f9ebc6ebdb19d769b793c245a736caaeb198c62587f13b25c660c13b4987f959`，4 CPU/4 GiB/1024 PID，timeout 300 秒，exit0、测试 72.008 秒；2026-10-07T15:53:17Z–15:55:01Z。
- deploy/stop/start/restart/delete-service/recreate、pull-policy never 的 missing-image 失败后 config/env 恢复、原生 .env、stale version、保留配置后 redeploy 和配置归档通过。默认删除 named/anonymous/external/bind 全保留；显式删除 named/当前 anonymous，external/bind/此前旧匿名卷保留。fixture 精确容器、网络、卷已清理，未批量 prune。log SHA-256 `15fb11ca8481085362372ff5fab60f66e02932eb2573786900349b43ab414142`。
- Compose browser：后台 job `arena-154-32440`，2026-10-07T16:00:43.695Z–16:01:57.351Z，passed/0；固定 browser Runner `sha256:b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29`，Node 24.18.0、Playwright 1.55.0、Chromium 140.0.7339.16，2 CPU/3 GiB/512 PID。原 harness byte-identical，spec `cb0608d7566fc46dcddb3a231ae3fd041c7b2fbaafdbd703010d0a393e812801`；log `a7b8e3de2411c13c2479b6f037d2cea5067731ca8c06b19fdeaf693492fc6a77`，result `b16d12616d7f4e04828e493af01a3ffc741ec2a16b1307449ee89024eca3a2a3`。
- 九组：1280/640/390 深浅色、390 en-US 深浅色，以及失败/保留配置/显式参数场景。Tab/Space/Esc、14px 单行、边界/无横向溢出、cancel 无 API、失败保留选项与重试、无容器但有配置项目和不相关容器通过；pageErrors=[]，无 OOM，清理完成。
- 本轮实际 mock DOM：文件 README shortcut 显示编辑器，底层目录 display:none/rect0；最大化 1280x720；390x844 组件页 pager display:none、无横向溢出、卡片自然高度与滚动。根关闭帧 sampler 因 restricted performance.now 失败，未宣称根关闭帧通过；原 5b068332 等价源码关闭帧 5/25/22 帧均维持 1280x720，重新打开 1232x672。
- 文件 ab01f598、视频 50e1d14b、窗口 5b068332 交付原件只按 blob 等价复用；有效 MP4 播放进度、Space/关闭/重开/Esc、坏视频超时与修复后 retry 原件保留。旧 RC11 mobile 1e073 证据不当作新 e36 候选证据；新组件页使用本轮实际 DOM/回归/patch 等价。
- 最终 fd2 acceptance mock 预览：`http://127.0.0.1:4178`，manifest `acceptance-final-fd2a243c`，ID `v125-rc12-final-1791392611936-6160dd`，grade=acceptance、profile=visual-composition；指纹 `e622cde43a6df00ed2bc29467836d7e8b88263dae96f67f4e50a2de124f74c29`。保留供用户验收，停止命令在 manifest；它不是线上实例。旧本任务 preview 已停止，原作者 4183 预览未代停。
- 未执行：真实手机 touch、Safari、native browser zoom 125%/200%、自定义 daemon socket/context、完整 kejilion.sh 双向旅程、长期 soak。确定性交互不机械加 soak；不以模拟鼠标拖拽代替真实 touch。

## 发布产物与公开仓库复核

- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.12)：draft=false、prerelease=true，非 Latest；Latest 仍为 v1.24.0。
- `docker.io/kjlion/kejilion-panel:1.25.0-rc.12` 与 `preview` OCI index 同为 `sha256:b9f4aa4342deb7155581a301d920b07a36e446417d85da96744cd2693a98f016`。
- linux/amd64：`sha256:eb06e98d6f6fed184c8481fb0a9d881925daeee702176b2b2d6cd7d616e98e61`；linux/arm64：`sha256:658a4b994f9af49a856157e6d7c071b380e72f96bf4abda50de336cef4dbbf64`。双架构 config version/revision 精确为本版/fd2，managed-script pins 一致；unknown/unknown attestation entries 2，不是缺失平台。Release 开启 SBOM 和 mode=max provenance。
- 14 个附件、SHA256SUMS 11 项与 GitHub asset digest 全部对应；实际下载 sums、metadata tar（内含 VERSION 校验）、LICENSE、THIRD_PARTY_NOTICES 并核对字节。其余 binary 使用官方 asset digest，`allBinaryBytesDownloaded=false`，未宣称所有 binary 下载执行过。
- public-after-r1/result.json：2026-10-07T18:02:22.302049+00:00 passed。版本 index body SHA-256 与 registry digest 一致，preview 相同；Docker latest 仍为 `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。
- arena-154 从公开 Docker Hub 明确 pull，实际 pull log 的 Digest 与上述版本 index 一致，再运行当前已验源码 `packaging/tests/image-e2e.sh`，loopback 18080、dummy Agent token、独立 fixture；`image_e2e=pass`、exit0。`public-image-e2e.json` 保存实际命令、时间和 raw log/hash；前后 E2E 容器/网络清单均为空，临时目录按 canonical EXIT trap 清理、未独立枚举。不把内部 native build 当公开拉取证据。
- apps/script 契约无需发布，稳定默认源未更改；原件 `script-apps-contract.json`。公开 E2E 非生产验收。

## 自更新通道验收

本版未改此契约。现有 source/scenario/update-backup-parity 门禁验证稳定 Latest、规范 RC、官方 digest、加入预览仅切来源不自动安装、自动安装与一次性安装分离、旧状态 stable 迁移/重启保持、退出较低稳定版不产生降级候选及失败备份/恢复。公开源的 stable/preview 身份和 digest 又在发布后复核。

未对真实用户 systemd/OpenRC/轻量 Node 做本次安装、升级、降级或生产重启；不能用自动门禁代替全旅程实机证据。OpenRC/Node 边界仍按 release-channels.md，未扩展支持。

## 生产部署安全核对

本节全部生产动作：不适用（预览版禁止生产部署）。没有生产部署授权或实际生产写操作，未连接/备份/部署/升级/核对 prod-108/108。arena-154 本次仅作登记隔离验证；不把候选容器或 fixture 当生产服务。生产前后版本、健康、正式备份和公网入口：不适用。

## 回滚

- 产品 tag 不可变，未重打、未重复公开同一版本；未实际执行生产回滚。
- 上一预览 v1.25.0-rc.11 源码 `e42afcfe7f47ea7cb4803d644c23583f844320cb` / index `sha256:e3eb789968da567b7bd1d21c25bb7b1288e7fd2e843afa69f20314da5546e93f`；上一稳定 v1.24.0 / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。版本镜像和 Git 标签均保留。
- 需要撤回预览通道时，核验上一 RC 的不可变 index 后仅恢复 preview 并复核；默认稳定 Latest/latest/apps 维持 v1.24.0。此处是恢复方案，未执行通道回退或修改生产数据。
- fixture 配置归档/哨兵恢复已实测；用户真实数据/配置备份：不适用。公共默认更新通道决策：不适用（预览不成为默认稳定源）。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-07T19:40:24+08:00
- 候选冻结时间：2026-10-07T16:34:10.960Z
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否（没有产品失败、生产回滚或重复公开版本；基础设施重试单独计流程异常）
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个纳入时间取 14 条精确映射原始源提交的最早 committer timestamp（ab01f598），不是 cherry-pick 时间；前后 UTC/上海日期如实保留。发布时刻见上文，未用发布时间替代生产完成。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：27
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/powershell/boolean-literal",
    "position": "before-production-write",
    "count": 1,
    "impact": "Initial pre-release metadata used false instead of $false; PowerShell rejected the expression and produced an unusable null receipt.",
    "recoveryEvidence": "Tool output retained in task history; pre-release-state.json was rebuilt from verified Git/remote identities.",
    "permanentAction": "Use structured Node/JSON receipts or native PowerShell boolean literals. Release owner; 2026-10-14 review before next production L3; no source or remote write occurred.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/powershell/contract-parser",
    "position": "before-production-write",
    "count": 1,
    "impact": "Inline contract hashtable had an unmatched delimiter; command did not execute.",
    "recoveryEvidence": "Tool ParserError retained in task history; prepare-release-contract.cjs then created the actual contract and ready preflight receipts.",
    "permanentAction": "Keep complex contracts in a parsed structured file. Release owner; 2026-10-14 review before next production L3; no permanent repository entry change claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/task-preflight/unsupported-tool-list",
    "position": "before-production-write",
    "count": 1,
    "impact": "requiredTools included ssh/scp unsupported by the canonical preflight schema; preflight rejected it.",
    "recoveryEvidence": "preflight-ready.json preserves failure; schema-supported tools corrected, SSH/SCP separately prechecked; preflight-ready-final-scope.json passed.",
    "permanentAction": "Use schema-supported requiredTools and declared separate transport prechecks. Release owner; 2026-10-14 review before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/read-only-file-inspection/unverified-path",
    "position": "before-production-write",
    "count": 8,
    "impact": "Six read-only calls guessed unavailable paths: old preview manifest; old CF metadata; two Compose evidence directories in one call; scripts/verify-release.sh; internal/config; packaging/Dockerfile. No failed output used as pass evidence. Additional retained event (count=1): A literal Windows wildcard was passed to rg as a file argument; it was rejected. No invalid output was used as evidence. Additional retained event (count=1): A batch worktree path from compacted context was read before authoritative worktree inventory; it did not exist.",
    "recoveryEvidence": "Actual manifests from delivery receipts, Compose _validation source inventory, integration test, root Dockerfile and cmd/kejilion-node/update_runtime/source.json were read. Both silently probed paths subsequently checked absent. Additional retained event (count=1): Exact check-security-audit-coverage.mjs and workflow paths were read, then metadata validation used the canonical checker. Additional retained event (count=1): Two Git errors in one read-only tool call retained. git worktree list and exact archive ref/reflog were read; no source write or reconstruction performed.",
    "permanentAction": "This repeats v1.24.0 and RC11. Discover all paths via rg --files or authoritative receipts before reads. Fixed repository entry preflight/regression remains due before next production L3; release owner, 2026-10-14 review. Current preview has no production write. Additional retained event (count=1): Use rg --files inventory or directory plus --glob. Owner: release task; due 2026-10-14 before next production L3. Additional retained event (count=1): Treat compacted paths as unverified until inventory confirms them; repeat of earlier path guessing. Owner release task; due 2026-10-14 before production L3.",
    "historicalReleases": [
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "evidence/integration-map/stdout-contamination",
    "position": "before-production-write",
    "count": 1,
    "impact": "PowerShell foreach captured Git stdout alongside mapping objects; first mapping JSON was unsuitable as the authoritative receipt.",
    "recoveryEvidence": "integration-map-first.json retained; integration-map.json rebuilt from exact Git cherry-pick trailers with 14 structured mappings.",
    "permanentAction": "Capture process stdout separately and validate object arrays before evidence use. Release owner; 2026-10-14 review before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/local-feature-preview/unsupported-help-flag",
    "position": "before-production-write",
    "count": 1,
    "impact": "Preview CLI does not implement --help; exploratory invocation returned usage failure.",
    "recoveryEvidence": "Actual repository usage read; scoped mock preview start/stop completed with verified manifests.",
    "permanentAction": "Read declared parser/usage before new flags. Release owner; 2026-10-14 review before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/run-repo-bash/option-as-script",
    "position": "before-production-write",
    "count": 2,
    "impact": "Two syntax probes passed -n as the required script argument; launcher rejected both before any remote execution.",
    "recoveryEvidence": "Canonical launcher parser read; uploaded remote and inner fixture scripts passed registered host bash -n, then real Compose and browser jobs passed.",
    "permanentAction": "Use run-repo-bash only with a script first; syntax prechecks use the already-declared registered Linux Bash directly. Release owner; 2026-10-14 review before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/cua-repl/unavailable-evaluate-global",
    "position": "before-production-write",
    "count": 1,
    "impact": "Closing-frame sampler used performance.now in restricted read-only evaluation; missing global caused a rejected promise and runtime reset. No closing-frame pass inferred.",
    "recoveryEvidence": "Same browser tab rebound; owner window visibly closed, actual fullscreen size and normal preview were verified; source-equivalent original closing-frame evidence and existing regression tests retained.",
    "permanentAction": "Use documented DOM evaluation without unavailable browser globals and handle async rejection. Real touch/native zoom remain unverified. Release owner; 2026-10-14 review before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/collaboration/agent-thread-limit",
    "position": "before-production-write",
    "count": 1,
    "impact": "Platform rejected coverage critic dispatch during run-19. This row counts the blocked critic stage once; the uncollected exact number of dispatch attempts is not asserted. Audit remained incomplete.",
    "recoveryEvidence": "Audit parent final message and run-19 incomplete metadata; no completed coverage claimed.",
    "permanentAction": "Keep bounded audit phases and capture every dispatch result before followup; retain prior platform-limit history. Audit owner; due 2026-10-14 before stable release.",
    "historicalReleases": [
      "v1.23.0",
      "v1.24.0"
    ]
  },
  {
    "fingerprint": "security-audit/evidence/closeout-parser",
    "position": "before-production-write",
    "count": 1,
    "impact": "Audit parent closeout JS command had a syntax error from embedded PowerShell backticks; no artifacts were written before its closeout time limit.",
    "recoveryEvidence": "Audit parent final message; root close-incomplete-audit.cjs preserved start metadata and records incomplete status plus verified unchanged end source.",
    "permanentAction": "Use file-backed structured metadata writers; never treat a planned artifact as evidence. Audit owner; due 2026-10-14 before stable release.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-l3/github-fetch-timeout",
    "position": "before-production-write",
    "count": 1,
    "impact": "Canonical r1 origin fetch timed out at isolated-source preparation. Candidate code, Runner and remote L3 were not executed.",
    "recoveryEvidence": "r1/source-prepare.json preserves failed status and cleanup=removed. Fresh SSH ls-remote passed unchanged main and tag identities; canonical r2 retries same SHA.",
    "permanentAction": "Upstream transient transport timeout: retain bounded canonical retries and immutable attempts. Owner release task; review 2026-10-14; exit when r2 canonical source preparation and L3 pass, otherwise stop remote publication.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/release-l3/stale-business-context",
    "position": "before-production-write",
    "count": 1,
    "impact": "Canonical r2 rejected stale business review (54 commits >= 50) during offline bundle verification; remote L3 and product tests were not executed.",
    "recoveryEvidence": "r2/source-prepare.json retained failed status and cleanup=removed; existing canonical business facts refreshed to b9c82ac6 with unchanged thresholds, source freshness rechecked, and r3 uses a new frozen SHA.",
    "permanentAction": "Run canonical business-context freshness before future freeze; add its required input to this task contract. A permanent shared preflight regression is not claimed. Owner release task; due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/source-archive/excluded-source-state",
    "position": "before-production-write",
    "count": 1,
    "impact": "First read-only source readiness helper incorrectly required excluded batch work to remain an active clean worktree. Actual inventory showed its existing exact archive; excluded work is not a frozen cleanup dependency.",
    "recoveryEvidence": "Internal first failed helper output retained; corrected source-preflight-final.json confirms five clean sources and all 14 patch IDs equal. Existing batch archive dca6305b was observed and retained.",
    "permanentAction": "Observe excluded work without freezing its lifecycle or recreating it. Owner release task; review 2026-10-14 before next publication.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/source-checks/ephemeral-runner-ended",
    "position": "before-production-write",
    "count": 1,
    "impact": "Optional separate lane-log copy was attempted after the canonical Runner had completed and removed itself; Docker rejected the copy. Required full L3 log remained persistent.",
    "recoveryEvidence": "Canonical r3 remote status is passed/0 and recovered evidence.sha256 matches all files. Full L3 log includes raw lane output and durations; live source result JSON was independently read before Runner removal.",
    "permanentAction": "Use canonical persistent L3 log or capture structured lane artifacts before Runner teardown. Owner release task; due 2026-10-14 before production L3.",
    "historicalReleases": []
  },
  {
    "fingerprint": "publish/git-ssh/github-internal-error",
    "position": "before-production-write",
    "count": 3,
    "impact": "Three candidate pushes were rejected by GitHub Internal Server Error. Remote refs remained unchanged, no candidate CI or public release started.",
    "recoveryEvidence": "candidate-push-failures-first-three.json preserves the three failure timestamps/request IDs; fourth immutable-SHA SSH push succeeded at 2026-10-07T17:06:44Z. candidate-ci-result.json records CI 37656540164 success at 17:15:47Z and Dependency freshness 37656540274 success at 17:07:27Z on fd2a243c12c9f9434da409408098135c8a23b3f9. No forced update or code change was used.",
    "permanentAction": "External transient GitHub write failure: preserve bounded retries, reread exact refs after rejection and require same-SHA CI. Owner release task; review 2026-10-14. Exit condition successful SSH write and candidate CI has been met; no repository repair claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/ci-receipt/field-shape",
    "position": "before-production-write",
    "count": 1,
    "impact": "First progress receipt writer checked a nonexistent top-level sha field instead of actual candidate; it exited before any file mutation. Candidate CI and main push remained successful.",
    "recoveryEvidence": "Root tool chunk 410553 retains Error: Exact candidate CI missing. Actual candidate-ci-result.json shape was read; corrected writer uses candidate and records the successful original CI runs.",
    "permanentAction": "Read the authoritative structured sample before parsing evidence fields. Owner release task; due 2026-10-14 before next production L3; no repository entry repair claimed.",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/acceptance-metrics/duplicate-fingerprint",
    "position": "before-production-write",
    "count": 1,
    "impact": "First canonical acceptance validation rejected two duplicate path-inspection fingerprint rows. Published product and public gates remained successful; no document commit/push had occurred.",
    "recoveryEvidence": "Root tool chunk 99de0f preserves duplicate fingerprint errors for items 9 and 15. acceptance-first-validation-failed.md and process-incidents-before-acceptance-normalization.json retain original inputs. Duplicate rows were aggregated without lowering any event count; final canonical result is recorded in docs-metrics-validation.log before documentation handoff.",
    "permanentAction": "Use unique canonical fingerprint rows and sum all retained per-event counts before acceptance validation. Owner release task; review 2026-10-14 before next production L3; permanent shared parser/preflight change not claimed.",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

最近五个正式版本 v1.24.0/v1.23.0/v1.22.0/v1.21.0/v1.20.0 已只读比较：路径猜测在 v1.24.0 重复，代理 thread-limit 在 v1.23.0/v1.24.0 重复。永久唯一入口/回归尚未声称完成；责任人 release/audit owner，2026-10-14、下一次生产 L3/稳定复核前处理。GitHub 三次候选 push 500 在第四次同 SHA push 和候选 CI 后恢复，未跳过门禁或改用 force push。

本次发布前 report-release-metrics 原件：生成 2026-10-07T16:06:13.895Z；滚动 14 天正式发布 3/生产部署 0，最近 20 份验收生产完成 15、中位数 4.07 h、变更失败 1/20=5%；流程数据 19/20 reported、258 异常、35 post-production、22 repeated、undeclared=0。它是发布前快照，不混为包含本版的统计。

## 遗留风险与后续准入

- CF run19 缺完整覆盖/正式证据；稳定版仍按现有 require 门禁补审/处置，不从 rejected 单条线索推断整轮无问题。真实触摸/native zoom/Safari、自定义 Docker context、完整脚本互通和长期资源趋势未验证。
- 无确认的 surviving 安全 finding 阻断本 RC；RC 审计检查仅记录，完整 L3、同 SHA 候选/main CI、Release 和公开产物门禁已执行，不降低稳定或生产要求。
- 本地资源：仅清理六个已结束 RC1–RC6 可再生 L3 workdir，实际净释放 3,172,470,784 bytes；原始日志、bundle、所有恢复 ref、最终 L3 源码和 evidence 保留。未 prune 任意 Docker cache/volume，未物理删除来源/未知工作树，最终 mock preview 保留供用户验收。固定 Runner 离线归档保留，未因磁盘压力启用灾备。
- 证据总根：`C:/GitHub/_release-evidence/v1.25.0-rc.12`；最终完整 L3：`C:/GitHub/_release-evidence/v1.25.0-rc.12-fd2a243c-l3-r3`。无新增知识库、工作流、提醒、PR 或生产动作；仅现有验收记录和事实基线的必要更新。
