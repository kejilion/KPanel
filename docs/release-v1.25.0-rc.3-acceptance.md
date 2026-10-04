# KPanel v1.25.0-rc.3 发布验收记录

日期：2026-10-05（北京时间）；发布级别：L3。

候选提交 / 标签：`5214c36a42db1a6c7aafbeb875fda7b987c7376a` / `v1.25.0-rc.3`；tree `a12df14da0a86a503aa2e22f82059ca66c2d7a6f`；annotated tag object `5eedba22e01e4cafbe1cb987ed97c4be7ecda4cb`，peeled SHA 精确相同。`releaseChannel=preview`；`releaseTrain=1.25.0`。

上一稳定版本 / 回滚点：`v1.24.0` / `ce27dc5171a97ed6e3d9475cddfdfac89762aad3` / `sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`。产物已发布，生产未部署。

## 发布画像与范围

- 用户授权继续发布 Windows 完整功能预览，继而明确要求下一 RC 删除 Windows Azure/Authenticode/Publisher/Profile 签名机制；采用递增 RC3，不改写失败 RC2。真实 Windows 生命周期/RDP 后续由用户测试，`owner-deferred/not verified`；CF 预览覆盖只记录。
- 纳入 Windows 平台/runtime、native 轻量节点、文件/多卷图库、终端、RDP bridge/准入及界面；展示、宿主机安装/更新、配置、协议和发行链均属 L3 风险。主机写入路径已实现，实际安装、更新、回滚、卸载与 RDP 真机矩阵未验证。
- Windows 产物为未签名测试发行。固定官方 GitHub tag HTTPS、受限官方跳转、执行前脚本 SHA、EXE SHA、大小限额、保护目录 owner/ACL/reparse、版本/协议核对、原子替换与失败恢复保留；SHA 清单同源校验不提供证书发布者身份证明。
- 本轮相对旧冻结6f为33文件514+/629-：`c91372c4958edede4c2fd85f6780d9fc930c1374`必要发布修复，`5214c36a42db1a6c7aafbeb875fda7b987c7376a`同tree OCR空提交。VERSION/package为1.25.0-rc.3、Go开发身份1.25.0-rc.3-dev。Windows来源6a是正常祖先，未squash。其此前小范围测试helper/字号/有界Vitest worker修复沿用。
- 只移除 Windows 外部代码签名链，不改 Noise、节点/联邦认证签名、desktopbridge/RDP凭证核心。`web/src`与6a全blob相同；现有API、数据、端口、Compose、Linux受管脚本、应用市场契约维持。没有新收费资源。

## 候选及来源分支处置

| 原分支 / 精确 tip | 处置与恢复证据 |
| --- | --- |
| `release/v1.25.0-candidate` / `5214c36a42db1a6c7aafbeb875fda7b987c7376a` | 本RC列车保留，精确已公开产品源码，不混入本验收纯记录提交。 |
| `feature/windows-node-rc2` / `6a2c97974491d45884fbdc33c2d878dcf652cc57` | 作者clean/所有权释放、源码为RC3祖先；保存`archive/feature/windows-node-rc2`精确tip、受双lease原子移除active remote，逐ref复核PASS。原作者local branch/upstream/worktree按用户要求保留，不把远端归档冒充本地回收。 |
| `fix/windows-unsigned-preview-release` / `5214c36a42db1a6c7aafbeb875fda7b987c7376a` | 本publisher必要修复，保存`archive/fix/windows-unsigned-preview-release`精确tip；原active remote未建立，归档点和不可变Tag恢复，远端复核PASS。本地修复分支到收尾前保留，worktree转纯记录候选。 |
| 本验收纯记录候选 | 单独候选CI→main快进→同SHA mainCI后归档`archive/docs/release-v1.25.0-rc.3-acceptance`；实际tip、lease/远端终态记录在仓库外`final-closeout.json`，不改已公开Tag/source。 |

原件`C:/GitHub/_release-evidence/v1.25.0-rc.3/source-archives-r1.json`。旧候选筛选及八项README/Claude/TS7归档沿用RC1真实记录；Node26、TS7、未公开脚本pin等未搭车纳入。原作者、CF detached b5和其它未知归属树不改不删；遗留native原树由owner在明确释放/恢复核验后按§13.1另行处置。

## 外部审计、独立复核与 OCR

- 精确RC3覆盖检查：`decision=scoped-required`，未审计5commits/92files；完整run4年龄13d，含desktopbridge/desktopcredentials/windowsnode新边界。按PROJECT_RULES5.4，preview只记录，不以未完成审计阻止本次预览，不改变未来stable规则。
- CF run18仍以b5为基线、scoped incomplete。93登记path到RC3只有83unchanged、10changed/deleted；新增`integrity_windows.go`在旧allowlist外。当前源码已变化，旧scope不能赋予RC3审计信用；未验证线索不公开拼接为已确认漏洞。未声称run18全结项或CF PASS，账本未修改。
- root对6f→RC3必要修复做独立只读消费：PASS WITH FOLLOW-UP、无新增source blocker；原件`C:/GitHub/_task-evidence/windows-node-rc2-20261004/rc3-signature-removal-review-r1.json`。其他provider CLI及tool route不可用，按provider-unavailable回退codex/codex；不新增第三个监工或召回原writer。该复核不是CF结论。
- OCR1.12.11实际范围6f→c913，26/26适用代码review、33dispositioned、7excluded（四doc unsupported_ext、两deleted签名文件、lock user_exclude）；自由臂在本次约束臂之前，但publisher已知规则，`blind=false`、`constrained-only=unreported`，H0/M0/L0。publisher自审属辅助，不冒充独立复核；5214同tree trailer，不重写已冻结source。纯Markdown验收OCR不适用。观察不足，不声称工具收益。

## 跨仓库联动与依赖

- `scriptLinkageState=not-required`：无需发布脚本（不适用）。变更集/脚本候选不适用，Windows native独立实现，没有变更Linux消费pin或写sh/apps。
- 内置脚本`c981fb6c8b481981ac7a006e102e111e435f6d30` / `0eb9a82860e6cf6cf76d4f946782a02fd90bef8e7be5a3fa724b93920d8e35cb`保持；标准L3静态/生命周期与公开镜像实际字节核对。
- apps local/packaged/public规范化SHA均`f03cb75911b7491cb009fee0bfe3500f4ae874327bb16ef1006894a232bcc089`，clean apps HEAD`4fc985e964dbd1742143521d0a28ab652b885773`；同契约无需新commit、默认latest。公开镜像实际kpanel.conf字节另验。
- Go1.27.1、Node24.21.0、TypeScript6.0.3及其余lock/Action SHA/image pins沿用既有基线；本轮未采用Node26/TS7等延期候选，删除Azure签名Action/配置是依赖移除。候选/main Dependency freshness均SUCCESS（report在push执行；每日security-advisories该条件SKIP），现行完整Go/npm/Trivy等扫描重新执行。不把历史每日通告/EOL或每一行报告当新人工审核；延期项责任及退出条件沿用现有依赖维护入口，不新增队列。

## 多维质量结论

| 维度 | 状态 | 证据与边界 |
| --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | 完整Linux L3、nativeWindows定向单位/PowerShell AST/两架构构建及Mock旅程通过；真实Windows安装/更新/卸载/RDP owner-deferred。 |
| 网络入侵与供应链 | 已验证 | 固定HTTPS/SHA、ACL/owner/reparse失败关闭、source/image扫描、附件/OCI/SBOM绑定；移除证书发布者证明，CF当前scope未完成，不是完整安全审计。 |
| 稳定性、恢复与兼容 | 已验证 | 原测试deadline/断言不变，source jobs2/maxWorkers2；事务/故障恢复fixture及Linux生命周期PASS。旧92b失败/RC2Check失败保留，真实Windows断电/重启恢复未执行。 |
| 性能与资源预算 | 已验证 | 本轮source wall571669ms、L3889s及一次资源采样，均为执行观测；无产品内存/长期负载/WAN收益结论，不声称整体大幅提速。 |
| 用户体验与可访问性 | 已验证 | sixcase actual100/125/200zoom Mock浏览器和91PNG原件按583UI blob+vite.config精确复用；不称新head重跑、nativeRDP或完整真实OS验收。 |
| 数据、配置与迁移 | 已验证 | 只兼容忽略两retired Publisher/Profile JSON键，其它unknown保持strict；旧trust.json不读取为策略，版本/协议/原子恢复保留；生产数据迁移未执行。 |

## 自动门禁与执行环境

- 定向开发5日志摘要核对：14release contracts、PS release/installer完整性、cluster bootstrap、native checksum/update边界PASS；追加reparse fixture实跑非skip、Windowsopt-in PASS、Go vet。开发dirty结果不替代最终资格。
- standard `scripts/run-release-l3.mjs` run `v1.25.0-rc.3-5214c36a-l3-r1`，arena-154 candidate-validation；固定Runner`kpanel-go127-prep-runner:go1.27.1-node24.21.0` / `sha256:6f1e654d1cc38727d85f1f8ff4d58441c11035013ffd455d9923705c76b22c8c`，Linuxamd64 Go1.27.1/Node24.21.0，无并发browser。WindowsBash经run-repo-bash GitBash。
- `2026-10-04T17:10:41Z`→`2026-10-04T17:25:30Z`，889s，passed/exit0，inner release_gate_runner与outer release_l3_gate PASS；frontend254files/2294PASS+6SKIP；Go tests/race/vet、漏洞/镜像扫描、双架构、安装/应用生命周期/备份恢复通过。
- source wall571669ms，web571603/go541808/deploy1994ms，identity_unchanged=true；原deadline、测试断言和用例保留。一次资源采样CPU459.84%、1.601GiB/7.76GiB，不是峰值或产品预算。与历史778/886s不同source/cache不做整体提速断言。
- bundle`20dfe3abc97d428721b88a4d5393c6d4cfc6beb674f403c37369c90e8786a6a0`；plan`88ea2551e19aaba3a15291df8aaeddb19690e6e2d1eaa79f7b039a99a4bb59e7`；remoteScript`21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`；12远端原件全部摘要核对。ready/environment/candidate/handoff exactHEAD/log/env/tools/argv PASS；初始DoR失败保留，不追认预实施DoR通过。
- [候选CI](https://github.com/kejilion/KPanel/actions/runs/37219452171) exact5214SUCCESS，Linux635s/Windows76s；[mainCI](https://github.com/kejilion/KPanel/actions/runs/37220946145) exact5214SUCCESS，Windows75s/Linux623s。当前Windowscluster AST/native checksum实际执行，部分其它包cached，非真实服务安装。
- [Release workflow](https://github.com/kejilion/KPanel/actions/runs/37221664953) exact5214SUCCESS；Windowsjob73s，actualCheck→Build→Verify三附件→transfer全部SUCCESS，失败/取消/skipped均禁止下游公开。Linuxreleasejob667s，Verify source364s、Linux二进制77s、native image65s；均为单次实测，不声称总体提速，公开不代表生产部署。

## 隔离真机与浏览器验收

- 正式浏览器Mock证据实际执行source6a，fixture `1158a4f418beb9040b9ce28f41c2877d2a0181d3e67ebe3ebba3bdb495ecb127`，preview-r9 fingerprint `60126899fffce85e17dedd47de10ecbf8e83bdee7e07f39bba37618b24e8516a`；登记background-browser workflow/actualChrome zoom，runner已停。新5214复用583web/src blob+vite.config与91PNG rehash，不重复昂贵未变UI层。
- 六组390/768/1280、100/125/200真实zoom、light/dark、zh-CN/zh-TW/en-US，验证enrollment/history、RDP失败策略、多卷图库、选中操作、Move→New Album焦点/空值disabled/dummy enabled/Cancel及Escape无提交、Windows+Linux第二session。正文按钮实际最小14px，icon-only aria label不当可见文字；截图可见关键盘符/操作。
- [公开镜像E2E](#发布产物与公开仓库复核)只在arena-154隔离Linuxamd64、loopback18089，标准`packaging/tests/image-e2e.sh`执行。Windows11/Server2022/2025真实install/update/rollback/uninstall、SAM/SCM/RDP、重启/WAN/arm64真机与长期soak均未执行；由用户后续验收，不阻断本次明确未签名测试preview。

## 发布产物与公开仓库复核

- [GitHub prerelease](https://github.com/kejilion/KPanel/releases/tag/v1.25.0-rc.3)公开于`2026-10-04T17:56:45Z`（北京时间`2026-10-05T01:56:45.000+08:00`），draft=false/prerelease=true；GitHubLatest仍v1.24.0。RC2Release未生成，失败tag不可变保留。
- 版本与preview OCI index `sha256:b0db852ee288ee68ae66ea36994e3c608d753fd6438f6a6ee1ae86c9f5f45dc5`；linux/amd64 `sha256:2d17e7bddc4ba5d42278e1e2223f97360ef68618670f2b9a33833fb74b3449ff`；linux/arm64 `sha256:2336af1abf1d7c4cefd763aaf4a954dab4c1741d56a3c76f48156e1c63671615`。实际raw body digest、各manifest/config、VERSION/revision/source/scriptlabels、User65532:65532/Entrypoint均核对；两个attestation manifest/layer bytehash、provenance/SPDX predicate和subject绑定PASS，不是SBOM内容全审计。
- Dockerlatest/1.24.0保留`sha256:e4417146d8db9cd59e15d62656438503345f84cea9ae7aba2c42ba5926f3935b`，GitHubLatest`v1.24.0`；preview从RC1前移RC3。官方registry baseline/after实际字节校验，无生产或stable/latest写入。
- 17公开附件，SHA256SUMS14唯一产品项均与GitHubasset digest匹配；实际下载校验sums、metadata、Linuxamd64Node、两WindowsEXE和安装脚本、LICENSE/THIRD_PARTY_NOTICES。WindowsPE架构/零证书目录、脚本与精确candidate BOM/CRLF规范字节匹配；未执行下载的Windows文件。其它二进制只核API摘要元数据，不声称逐一下载/运行。metadata12关键文件与candidate blob相同；没有独立JSON SBOM附件，SBOM/provenance在OCIattestation提供，其原始bytes/JSON/predicate/subject绑定已核对，不是完整内容审计。

| Windows文件 | 字节数 | 实际下载SHA256 |
| --- | ---: | --- |
| `kejilion-node-windows-amd64.exe` | 10634240 | `9d73bcb3660d7907dd93bcd219505a5a32682686876b9597af7da29c4e867d3e` |
| `kejilion-node-windows-arm64.exe` | 9541120 | `23092d69266bf99027b065a1eb88fbc1fd13376178b6bb67155490bd81711e62` |
| `install-windows.ps1` | 11220 | `b29699ecf6740c9bbecdb2b4867c9bf4c3290a0d58b099c9e56736e4414b0ace` |

- 公开镜像`image_e2e=pass`，run `2026-10-04T18:00:52.900573+00:00`→`2026-10-04T18:01:12.639803+00:00`，标准脚本/实际公开digest/loopback18089，exit0；内置script/apps/两gallery图标/VERSION实际字节PASS。owned container/network/tmp回收，原件保留`C:/GitHub/_release-evidence/v1.25.0-rc.3/public-remote-evidence`，不触及生产服务。

## 自更新通道验收

- Panel stable来源只选正式Latest，preview选规范RC并核digest；加入preview只切来源/检查，不自动安装；自动安装开关和立即安装独立、旧状态默认stable、退出preview不自动降级，既有Go/L3回归保持。
- Windows节点原有自动更新保留：官方Latest稳定release规范版本解析为固定tag，下载受限官方HTTPS与SHA清单，然后版本/协议核对及原子事务。移除Authenticode不新增双轨模式、手工版本锁或Azure变量。空/畸形checksum、越界文件、不可信ACL/reparse、篡改/不兼容版本失败关闭。旧trust状态不恢复Publisher依赖；生产实机更新未声明PASS。
- systemd/OpenRC/轻量Node边界见现行release-channels；实际Windows服务生命周期由用户后续测试，Linux自动门禁和Mock不替代该层。

## 生产部署安全核对与回滚

- 生产目标/授权、备份、部署命令、灰度、升级、健康/Panel/Agent/重启/日志/公网/数据完整性：不适用（预览版禁止生产部署）。生产已执行写操作0。
- `prod-108`/108：禁用全部KPanel操作，本次未连接、未备份、未部署、未升级、未核对；不作为验收缺口或覆盖。arena-154仅owned隔离L3/publicE2E，未操作其生产实例。
- 稳定回滚点见首段；GitHubLatest、Dockerlatest和apps默认更新仍v1.24.0，无公共默认通道回退需求。旧previewRC1 digest`sha256:e3d0b3543f014e84abed356ac590fb36c161d9dad6524eb4d96f7d6b3585f316`保留恢复；源码tag不等于任何版本都有可用OCI，RC2没有公开镜像。
- 用户退出preview不自动降级；需要回滚时须另按授权配对数据/配置/版本备份及健康核验，不改历史Tag。本次真实生产回滚及健康未验证。

## 交付节奏与已知异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-10-04T11:58:24+08:00
- 候选冻结时间：2026-10-05T01:08:51.647+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

首个时间为Windows聚合f1d2原author/commit时间。RC3是用户改变Windows签名发行机制后的新不可变预览版本，不计生产吞吐或生产变更失败；原RC2Check失败被公开前拦截。仅报告889s及各门禁实际观测，不以成功轮掩盖多turn准备/返工，不声称整体大幅效率提升。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：未记录
- 其中生产写操作开始后异常次数：未记录
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

跨多turn整体异常次数无完整计数原件，机器两项均未记录，不硬造0或部分总数。实际生产写为0，以下已知事件逐项保留，未记录总数不抹去失败：

- RC2Release37215609499 WindowsCheck在2026-10-04T16:07:39Z→16:07:55Z因`An exact trusted publisher Subject is required.`失败；Build/Sign/Verify和下游公开skipped，非Environment待审批。不可变tagobject53f043eacf5ff45e1e2c1ad8f77f8a3fb74be86b/peel6f及原件`v1.25.0-rc.2/publisher-signing-blocker-r5.json`保留。用户新要求后修复RC3，不空重试/重写RC2。
- 旧92b L3 Desktop overflow10s FAIL、race/vet cancelled和图库12px/Move按钮browser失败归属旧source，作者最小修复后254/6a证据各自绑定，未扩大timeout或减少断言。本RC3新完整L3独立通过。
- RC3初始task-preflight requiredTools误含pwsh被拒，随后dirty9files ready拒绝；实际预实施DoR未PASS，授权修改已发生事实不追认。最终clean5214 ready/environment/candidate/handoff PASS，初始日志保留。
- 已知准备/取证错误包括：猜测路径/工具名/缺失旧JSON、PowerShell glob与cwd、Go未在PATH；原子patch拒绝、JS模板backtick与替换导致gofmt失败、CRLF marker问题；JSON字段假设、helper旧l3.json输入在执行前修正。正确当前路径/注册runtime、原literal替换、py_compile及实际最终测试/摘要核验恢复；编译/产品开发错误与流程诊断区分，不当成已通过证据。此阶段读取缺失旧RC2handoff和误名doc checker同样保留tool chunks87270a/150498，没有源码或外部写入。
- public verifier r1在三Windows下载字节通过后，因误期待2独立JSON SBOM附件FAIL；实际不可变工作流只有17已核附件，SBOM/provenance在OCI。原脚本/hash/rawlog/阶段原件保存`public-verifier-r1-failed`，修正外部取证器、已下载不可变字节重hash复用后r2完整通过；没有产品source/Tag改动或重跑L3。原日志`public-validation.log`保留，成功原件`public-validation-r2.log`及实际OCI原始bytes另存，不能以unknown平台存在或sbom:true冒充内容已审。
- 收尾首次docs增量检查误把VERIFY_BASE_REF交给launcher的受限`--env`，被明确拒绝；原件`docs-change-r1.log`保留，修正为标准脚本已有的base positional参数后r2通过，最终docs exactcommit再绑定适用验证。另一次只读猜测不存在的build-linux-release-binaries.sh路径被rg拒绝（chunk3b1c77）；现有workflow实际步骤/日志已直接读取，不改变source、Tag或公开产物。
- 原件在本版`focused-r1`、`final-*-r1.log`、`public-helper-qualification-r2/r3.json`及会话tool outputs，RC1/RC2旧失败不删。最近五stable v1.20-v1.24指纹已对照历史`v1.25.0-rc.1/incident-history-five-stable.json`；read-only-file-inspection/unverified-path与v1.24重复。责任人为publisher，下次L3生产写前须修复唯一读取/预检入口并补回归；2026-10-12前复核，退出条件为真实入口/字段资格化成功且原错误保留。现场绕行不称永久修复。本RC禁止生产。

## 遗留风险与收尾

- Windows未签名失去证书发布者证明；官方同源SHA只提供约定完整性，必须保留固定来源/ACL/限额/事务边界。Windows11/Server2022/2025真实安装/更新/rollback/uninstall/RDP、长时连接/WAN/arm64均尚未验证，由用户后续验收；公开产物不标成这些场景PASS。
- CF当前源码变更/未覆盖，run18仍incomplete；未覆盖新integrity实现和修改路径明确保留。稳定发布按当时PROJECT_RULES重新判定，不借本preview豁免。
- 本地当前RC3/上一稳定恢复kit、失败原件、PNG/公开下载原件及author/CFtree保留。没有安全可回收的已释放本地可再生资源时不强删，实际本地文件净释放0字节；owned L3/E2E container/network/tmp已清理，磁盘采样及最终docsCI/归档终态保存final-closeout，不将预计释放当实际结果。
- 证据根`C:/GitHub/_release-evidence/v1.25.0-rc.3`；标准L3/CI/main/tag/Windowsjob/公开字节/OCI/SBOM/E2E/分支恢复均有真实原件。纯验收docs提交用适用增量验证/同SHA CI收尾，不重复产品L3或改已公开Tag。
