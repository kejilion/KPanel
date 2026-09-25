# KPanel v1.22.0-rc.5 发布验收记录

日期：2026-09-25。发布级别：L3。本记录将公开产物、自动测试、Mock 浏览器和未验证的真实集成分别记录。

- 发布级别：L3；`releaseChannel=preview`；`releaseTrain=1.22.0`。
- 当前候选：`aea1a75bc00c32172cb9c52b4a65805f86398f52`。
- 发布基线：`6468e0536b66c8162eb6e66d6431ee77beec0ade`。
- 本地组装分支：`release/v1.22.0-rc.5-assembly`；工作树 `C:/GitHub/_codex-tasks/kpanel-v122-rc5`。
- 同序列远端：`release/v1.22.0-candidate`，预览版按规范保留；精确 tip 为本次发布提交。
- 上一预览：`v1.22.0-rc.4` / `866d2aa8bd34e568fbd686d6c0aefa70f484e8f9`。
- 上一稳定版：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3`。

## 发布画像与范围

业务域为桌面外观、独立下载的 WebGL 场景和共用弹窗遮罩。新增登录态下的 Panel 应用数据写入、固定来源下载和匿名 artwork capability 文件交付；不变更 Agent 动作、宿主机管理权限、Compose/生产端口、既有业务数据格式或安装契约。

用户明确确认只纳入原来的三个场景：

| 场景 | ID | 文件数 | 发布字节数 |
| --- | --- | --- | --- |
| 星港轨道 | orbital-station | 34 | 4,086,696 |
| 霓虹都市 | neon-city | 8 | 1,150,940 |
| 海天日月 | sea-and-sky | 27 | 10,564,422 |

- 场景来源 tip：`69b7703452a4d8fb9e318e3e2beda946c49ef193`。
- 共用中性遮罩来源 tip：`ec3d2162e18666457f25b11c8477cebc42dc94a9`。
- 原候选只有前端和 mock，`8a9f0ffc` 补充真实存储、下载和沙箱文件服务；`96398f7b` 补齐更新按钮及当前场景刷新；`b7a77352` 补版权通知、字体/窄屏和 mock CSP；`5c025a9d` 修复自动下载反复尝试不可达线路导致总超时的问题。
- `e3ebb691` 统一版本、CHANGELOG 和既有业务现状文档；`f8a90ddb`、`b8a980cd` 保存 OCR 审查记录。
- 独立复核发现无效目录会阻止线路回退，`56aca05e` 补充每一路目录 JSON/schema 校验，`beb8370b` 保存窄增量 OCR 记录。该发现归于独立复核，不计入 OCR 发现数。
- `885faf81` 增加 opt-in `RejectRedirects`，仅场景 Store 启用，在发出后续请求前拒绝重定向；其他下载器默认行为不变。`aea1a75b` 保存对应 OCR 记录。该修改来自独立审计指出的固定来源边界问题，不宣称存在私网 SSRF 或凭据泄漏。
- 倒悬月宫 `e21afa1a2e2697f69fc1424c53793720758b9d6b`、深海遗迹 `f26f64e8be9d6d435402d30052072173a6bc36ee` 的误合入由 `68d03671` 撤回，净内容不在候选中。两者来源分支保留，不以祖先关系当作已发布依据。
- 其他独立/脏工作树未纳入、未回收；原任务所有权未释放，暂保留来源分支及工作树。

受影响旅程：显示三个场景、切换下载来源、下载/校验/安装、应用并持久化选择、切换镜头、遮挡和隐藏时暂停、减少动态效果、原子更新并刷新 iframe、删除当前场景后回退、失败恢复。

## 多维质量结论

| 维度 | 状态 | 现有证据 | 未验证风险 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已实现未实机验证 | Store 与真实 Panel HTTP 安装/删除/冲突回归，前端定向测试 | 浏览器与真实服务完整端到端未执行 |
| 网络入侵与供应链安全 | 已实现未实机验证 | auth/Origin/CSRF/audit/capability 路径测试，当前 L3 r4 安全扫描通过，CF scoped 源码审计和两项下载增量独立复核完成 | SSE 撤销时序契约仍 needs_validation，浏览器沙箱运行行为未验证 |
| 稳定性、失败恢复与兼容 | 已实现未实机验证 | 下载校验失败和状态提交失败保留旧版本；并发/race、重启、撤销 capability 测试 | 未做真实宿主机长期运行 |
| 性能与资源预算 | 已实现未实机验证 | 文件、包、总量、并发边界；资源独立下载，Three.js 不进入主应用运行包 | 实际 WebGL/GPU 资源和体验未验证 |
| 用户体验与可访问性 | 已实现未实机验证 | 三场景实际渲染、刷新恢复、删除回退、失败恢复、三语言、主题、宽窄屏、键盘和实际字号 | 真实集成、真实缩放、reduced-motion、更新和暂停/资源行为尚未完成 |
| 数据、配置与迁移 | 已验证 | 可选 scene cache，不迁移既有业务数据；备份既有目录白名单不包含缓存 | 下载资源可重新获取，实际生产回滚未执行 |

## 自动门禁与证据归属

- 当前 L3：`v1.22.0-rc.5-aea1a75b-l3-r4`，完整通过，exit 0；`2026-09-25T03:20:07Z` 至 `03:27:20Z`；唯一入口 `scripts/run-release-l3.mjs`，目标 `local-wsl-dr`。旧 r3 在 `beb8370b` 完整通过，但已被后续重定向修复替代。
- Runner：`kpanel-release-gate:go1.26.7-node24`；不可变 ID `sha256:a9f708891d1e81f17dd286bc7e9d6124658964716dc9a457b5c73340722f6a7d`；Ubuntu WSL / root Docker，Go 1.26.7、Node 24.20.0。
- L3 r2 已完整通过，188 个前端测试文件、1,679 个测试通过，三个场景从源码逐字节复现；执行时间 `2026-09-25T02:26:20Z` 至 `02:33:26Z`，exit 0。r2、r3 均已被后续修复替代，当前候选结果以 `l3-r4/wsl-evidence/status.txt` 为准。
- 当前 r4 bundle SHA-256：`d5f23d784c041cf4a89dd85a93a60873897a6708f666a26a6e9c2879812bc88c`。
- 当前 r4 plan SHA-256：`1adbdfa8b9417eb88d6c82aa963486e71ada8f81621150330ffb30a60fa328e9`。
- 受检外层执行脚本 SHA-256：`21c08b11be3526a0fe766aa606a81d0e4047e8a4e89d79227da4e62914164979`。
- r4 的上述三个输入已由本地 SHA-256 复核与 manifest 一致，Runner 实际 ID 与登记 ID 完全匹配；全量 Go、188 files / 1,679 frontend tests、3-pack 字节复现、核心 race/vet、双架构构建和 app_conf_lifecycle 均通过。govulncheck 为 0 reachable、0 imported-package 漏洞，另报告 1 个 required-module 中未调用的漏洞；npm audit 0，Trivy source/final-image 门禁通过。
- L3 r1 在旧候选 `f8a90ddb` 完整通过，后续线路修复使其被 r2 替代，不作为当前 SHA 最终证据。
- 线路修复 `go-route-race-r1.log` 通过；`go-route-before.log` 在旧代码负向对照中按预期失败（反复请求故障线路），证明回归用例覆盖真实差异。
- 目录修复 `go-catalog-race-r1.log` 包级 race 通过；`go-catalog-before.log` 在旧实现复现四项失败（坏 JSON/坏 schema × 官方/首选镜像）。修复后自动模式均恢复，显式线路维持不回退。
- 重定向修复 `go-redirect-race-r1.log` 中 remotedownload / scenepacks 两包 race 通过；新增内存 RoundTripper 用例确认同域/跨域跳转均只发送一个请求，既有默认重定向及敏感头剥离测试仍通过。
- 候选 CI、主线 CI、Release workflow、公开镜像 E2E：见下方公开产物复核。

## 审查与审计

- CF run-8 对不可变 `8a9f0ffc2b8a74008ed8fc44379d4f3e82751dba` 的 12 个边界文件完成 scoped 源码审计。协调代理和有效子代理明确使用 `gpt-6-luna` / `max`；最终 coverage critic 为 `stop=true` 且无遗漏，Phase 5 独立核验全部三条记录且无改变结论的遗漏，Linux/WSL 结构校验通过（28 个覆盖单元、3 条 findings），冻结审计源 HEAD/tree 未变且 clean。
- 审计结论为 1 条 informational 已确认（固定源重定向出站，已由本候选修复并独立复核）、1 条 SSE 注销响应时序契约待验证、1 条 PTY 有界清理问题不成立为安全发现。未确认高危或中危问题；浏览器 opaque-origin/导航行为、可选 artwork 的掉电耐久性仍未验证。完整记录位于 `security-run-8/REPORT.md`，不能把源码审计完成写成这些运行时边界已通过。
- 审计有效证据为源码复核和结构校验，root 的 L3 单独归属。两次代理在只读审计目标内产生临时未跟踪输出的尝试已弃用、清理并记录；其中一次调用 go test 并编译，但测试二进制因 permission denied 未执行成功。相关输出不算动态验证；最终源身份未变。缺少显式模型配置的一次 hunter 结果亦已弃用。
- 记录维护限制：协调代理在终结时间戳时曾短暂损坏 `run-metadata.json` 和 `source-baseline-end.json` 的序列化内容，随后从此前已捕获的完整内容重建。root 已直接核对恢复后的起止时间、范围、三条结论、Phase 5 状态、两项增量记录及结构校验输出；冻结源保持 clean，代码未被该操作写入。
- 固定 Cloudflare skill 提交 `c1c8a8c1471069fb0e188eeaff69b8e8db6564a8`；20 个本地文件与 GitHub 对应树的 blob 完全匹配，证据 `skill-pin-verification/verification-r2.json`。
- 线路修复 `f8a90ddb..56aca05e` 已由独立 `delta_route_review_r1`（显式 `gpt-6-luna` / `max`）完成源码和测试差异复核，无阻断项；证据 `security-run-8/agents/delta_route_review_r1/delta_route_review_r1.json`。这是 run-8 冻结源之后的独立增量，未扩展 run-8。复核者没有独立运行 Go 测试，执行证据仍归 root 的负向对照、包级 race 和精确候选 L3。
- 窄复核保留一项低影响限制：在途 auto 缩略图下载可能在手动切换来源后迟到写回临时线路偏好，只影响后续 auto 的来源顺序；显式来源、大小/哈希校验和 fallback 不变。本轮不作额外修复。
- 重定向修复 `beb8370b..885faf81` 已由独立 `delta_redirect_fix_review_r1`（显式 `gpt-6-luna` / `max`）只读复核通过；证据 `security-run-8/agents/rc5_cf_audit/artifacts/delta-review-redirect-reject-beb8370b-885faf81.json`。确认默认下载器不变、场景 Client 单独启用第二请求前拒绝、错误映射及 fallback/哈希/大小限制仍保留。该复核未执行测试；执行证据归 root 的两包 race 和最终 L3 r4。
- OCR 1.12.6 最终观察范围 `6468e053..5c025a9d`：105 个可审文件，61 已审、44 按文件注明跳过，处置覆盖 100%；不宣称全部源码逐行审查。场景 shader/几何、离线 Blender 工具及生成代码部分排除；生成 dist 另由源码构建复现验证。
- r1 自由审查先落证、blind=true，发现更新动作遗漏；约束阶段新增 mock CSP、版权通知、字体/窄屏问题，均在候选冻结前修复。r2/r3 follow-up 不是新的盲测，constrained-only=unreported。r3 自由审查发现自动线路问题并修复。
- r4 对 `b8a980cd..56aca05e` 窄增量审查 2/2 个 Go 文件，两个 Markdown 被工具排除但已人工核对；没有新增 OCR 发现。保留 r3 对全范围的既有处置，不将独立发现重记为工具收益。
- r5 对 `beb8370b..885faf81` 窄增量审查 3/3 个 Go 文件，没有新增 OCR 发现；重定向问题归独立 CF 审计发现，不归功于 OCR。冻结候选为 `aea1a75b`。
- Claude/Gemini/Qwen CLI 不可用；本轮实际 fallback 为上述独立 Codex scoped 源码审计及增量复核，已完成。运行证据仍分别归属各自执行者。

## 跨仓库联动与依赖

- `scriptLinkageState=not-required`；无需发布脚本（不适用）。实际内置 `kejilion/sh@2b90b2d2ca56bc954c9328a51bb5571e896f713d`，SHA-256 `806b4715664fad502f7faeccbc75972f2d1a24559d46208221b98f774ef56c99`。
- 未变更脚本协议、宿主机动作、镜像内脚本或 apps 安装/更新契约。apps 仓库 `39b498a0dc6b3013fda31b103c138ad0df3cc42c` clean；无需提交。
- `dependency-report-r2.json` 生成时间 `2026-09-25T02:02:26.128Z`，10/10 来源完整，31 个直接/基座候选、164 个传递依赖信号。报告分类 emergency-security=0 不代替漏洞扫描。
- Three.js / types 为 0.186.0 场景构建依赖；报告出现 0.186.1 候选，本轮冻结后不追加升级。既有依赖处置期限沿用，不因新报告重置。EOL 复核 current。
- 新包随发布资源携带 Three.js/Ashima 许可文本及相关来源说明，并被目录 SHA-256 覆盖。

## 本地预览和未完成验收

Mock 模拟数据，clean checkpoint；启动器状态 ready 不代表受影响旅程已完成。预览责任人为本发布任务。

- URL：`http://127.0.0.1:4185`，API 4186；`mode=mock`，`grade=acceptance`，`profile=visual-composition`。
- 功能入口：桌面更换壁纸中的 3D 场景选择；分支 `release/v1.22.0-rc.5-assembly`。
- preview ID：`rc5-scene-ui-1790307873155-8efb53`，源码 `aea1a75b` clean；fingerprint `83dddfd6984e7d6095d000eb888701093bf7d1a4179bcb3bdbd04556c1d3749a`。
- manifest：`preview-ui-acceptance-r5/manifest.json`。旧 r1/r2/r3/r4 已停止；本轮发现 r4 进程已退出后按原启动器创建 r5，未覆盖旧证据。r5 的核心 Mock 旅程已部分验证，真实后端端到端未通过。
- 前轮及本轮 reset 后的 CUA inventory 均报告 `nodeRepl.fetch request failed`；本轮直接调用文档支持的 `cua.createBrowserTab("iab", ...)` 成功。所有实际浏览器操作继续使用 `cua_repl`，未改用其他 UI 自动化。
- 本轮已从精确候选 `aea1a75b` 重新交叉编译 `panel-scene-preview-aea1a75b.exe`，exit 0。用户要求继续尝试后，`Start-Process -WindowStyle Hidden` 再次被自动审批以 `blocked by policy` 拒绝，实际未执行；没有更换启动机制。编译身份和二进制哈希记录在当前预览的 `browser-acceptance.json`。
- 已在应用内浏览器检查三个场景实际渲染、霓虹都市机位切换与刷新恢复、来源选择、活动场景删除回退、另一页面旧状态删除失败及重新下载恢复、浅/深主题、简繁英、1440×900 宽屏和 390×844 窄屏、键盘焦点与 Escape。控制台 error/warn 捕获为空。详见 `preview-ui-acceptance-r5/browser-acceptance.json`。
- 尚需真实后端浏览器集成、其他场景机位逐项、资源更新/remount、reduced-motion、隐藏/遮挡暂停、持续 GPU/资源与敌对 iframe 导航验证。两次 Control+plus 未改变宽高、DPR 或 viewport scale，因此 125%/200% 缩放仍未验证；不将窄视口代替真实缩放。
- 实际计算字号：最小可见文字 12px（3D/官方紧凑徽章），辅助说明/元数据 13px，操作按钮/正文 14px，场景名称 15px，分组标题 16px，弹窗标题 17px；符合现有用途基线。窄屏弹窗 clientWidth=scrollWidth=390，无横向溢出。受影响体验维度包括场景与桌面/弹窗共存、主题/语言、窄宽视口、缩放、键盘焦点、运动偏好及失败反馈。
- 预期旅程与本轮状态（均为 Mock 浏览器证据）：
  1. 进入桌面更换壁纸 => 目录只显示三个已确认场景；已检查。
  2. 下载并应用每个场景 => 沙箱画面渲染，操作反馈可见；三个场景均已检查。
  3. 切换机位并刷新 => 场景选择恢复；霓虹都市已检查，其他场景机位逐项未测。
  4. 删除活动场景 => 回到经典壁纸；海天日月已检查，iframe 数量变为 0。
  5. 切换减少动态效果 => 显示静态海报或明确开启动画；未完成浏览器验证。
  6. 窄屏与 100%/125%/200% 缩放、切换主题 => 面板可滚动，操作文字可读；默认比例的宽窄屏/主题已检查，125%/200% 真实缩放未验证。
- 停止本轮预览的命令：`node scripts/local-feature-preview.mjs stop --evidence-dir C:/GitHub/_release-evidence/v1.22.0-rc.5-20260925/preview-ui-acceptance-r5`。其他任务 4174 未触碰。

## 发布决定与验收边界

用户在获知真实后端启动被自动审批拒绝和剩余浏览器矩阵后，明确要求“请上线预览版”。本次据此继续 RC 公开发布，保留已披露的限制；不将这些项目标为通过。`publication-decision.json` 保存此次决定。后续稳定版仍需补齐准入证据。

- CF 覆盖检查在发布 SHA 返回 `decision=scoped-required`、`unaudited commits=8`、新边界包 `internal/scenepacks`。RC 按规范只记录；外部 run-8 与两个后续增量复核已完成，但未写入仓库覆盖索引，不能把该结果改写为 `ok`。
- 变更集编号、脚本候选及脚本发布：不适用；`scriptLinkageState=not-required`。
- 受影响的发布画像包含展示、登录态 Panel 应用数据写入、固定来源下载和 opaque-origin 沙箱；没有宿主机/Agent 新权限、现有数据迁移或生产部署。
- 最小视觉风险由实际三场景渲染、主要操作和错误恢复的 Mock 浏览器检查覆盖；其余场景与真实服务贯通仍为已实现未实机验证。

## 发布产物与公开仓库复核

- 主仓库发布提交 `aea1a75bc00c32172cb9c52b4a65805f86398f52`；annotated tag `v1.22.0-rc.5`，不可改写。
- 候选 CI：[run 36094028499](https://github.com/kejilion/KPanel/actions/runs/36094028499)，success。
- 主线 CI：[run 36094716749](https://github.com/kejilion/KPanel/actions/runs/36094716749)，success。
- Release workflow：[run 36095442914](https://github.com/kejilion/KPanel/actions/runs/36095442914)，success，所有必需步骤完成。
- [GitHub Release](https://github.com/kejilion/KPanel/releases/tag/v1.22.0-rc.5)：`draft=false`、`prerelease=true`、非 Latest；公开时间 `2026-09-25T04:52:40Z`。GitHub Latest 复核仍为 `v1.21.0`。
- [Docker Hub](https://hub.docker.com/r/kjlion/kejilion-panel/tags)：`docker.io/kjlion/kejilion-panel:1.22.0-rc.5` 与 `preview` 的 OCI index 均为 `sha256:7fd3ff5f9e9cc0d14df2313520f5d16a150bc744c7076b4efc313efcde9359c5`。
- `linux/amd64`：`sha256:f9def0b4365d5b73e0911c741c2b294c25e0d870089fc18721327a71aeba468e`；`linux/arm64`：`sha256:2dedc7a18b4d2cba60a84403139c82be6e7a65bc431bf22c9cae25627662155c`。
- Docker `latest` 发布前后均为 `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`，稳定通道未变化。
- 全部 14 个 Release 附件已从公开 URL 下载并核对 API 大小/摘要，其中 SHA256SUMS 覆盖的 11 项全部匹配。包括 Agent/Node 双架构、MCP 客户端与 deploy tar.gz；详情 `public-verification.json`。
- 公开 OCI index 的双架构 attestation 逐项核对；SPDX SBOM 与 SLSA provenance 均存在，见 `attestation-verification.json`。额外 `unknown/unknown` 是 attestation，不是缺架构。
- 显式从 Docker Hub 拉取版本镜像后，使用仓库原有 `packaging/tests/image-e2e.sh` 得到 `image_e2e=pass`；记录 `public-image-pull.log`、`public-image-e2e.log`，非本地构建缓存冒充公开证据。测试包含健康/版本、非 root 静态资源读取、代理/HTTPS 安全 cookie、隔离网络和 Docker health 状态。
- GitHub main 的公开目录与冻结提交逐字节相同；仅三个场景，全部 69 个资源文件大小和 SHA-256 匹配，合计 15,802,058 字节，见 `public-scene-verification.json`。这是公开资源校验，不是浏览器与真实 Panel 的完整集成验收。
- `kejilion/apps@39b498a0dc6b3013fda31b103c138ad0df3cc42c` clean 且远端同 SHA；仅既有 `app_url` 的 Docker Hub/GitHub 展示链接差异。归一化该展示字段与换行后内容一致，安装/更新契约无需提交；证据 `apps-contract.json`。
- 本文中外部证据的根目录：`C:/GitHub/_release-evidence/v1.22.0-rc.5-20260925`。

## 自更新通道验收

- 稳定来源仍读取 GitHub Latest v1.21.0，预览来源允许规范 RC；公开 `preview` 与版本 OCI index 一致。源码通道选择、持久化、退出预览不降级等既有自动回归随精确候选测试通过。
- 加入预览只切换来源和检查，安装开关与一次性立即安装独立；本轮未执行真实面板自更新、systemd 后台更新、更新前备份或失败恢复演练，不冒充实机成功。
- OpenRC、轻量 Node 的支持边界沿用 `docs/release-channels.md`；应用市场默认入口保持 `latest`。

## 生产部署安全核对与回滚

- 生产目标、部署、停写备份、生产 Panel/Agent 健康、日志和数据核对：不适用（预览版禁止生产部署）。产物已发布，生产未部署；本轮没有生产写入。
- `prod-108` / `108`：禁用全部 KPanel 操作，本次未连接、未备份、未部署、未升级、未核对。
- `arena-154` 本轮未连接。Linux 验证使用 `local-wsl-dr` / Ubuntu / root Docker；公开镜像 E2E 使用临时隔离容器和独立数据目录，不能代替真实宿主机管理验收。
- 上一预览回滚点：`v1.22.0-rc.4` / `866d2aa8bd34e568fbd686d6c0aefa70f484e8f9` / `sha256:b7401060adee40926ae690950db1d67f2d6b4462526d1ee1ed6d2b844abda4ca`。
- 上一稳定回滚点：`v1.21.0` / `396fcd62c5635c9812ee97ee509cb61b962724f3` / `sha256:e2c5d392dfcee8263ea82ebe0c8aed15276c046a3bd260b6219b634a844f3f61`。
- 未执行生产回滚。需要恢复时使用已验证的上一版本固定镜像，按标准更新/备份流程恢复并重新核对版本、健康及数据；不得改写 RC5 tag。可选 scene cache 不属于既有业务数据迁移。
- 公共默认更新通道决策：不适用；GitHub Latest / Docker `latest` 保持上一稳定版本。

## 候选与来源分支处置

- `release/v1.22.0-candidate` 保留于 `aea1a75bc00c32172cb9c52b4a65805f86398f52`；同序列预览候选不归档。产品组装分支 `release/v1.22.0-rc.5-assembly` 保留同一 SHA，可由不可变 RC5 tag 恢复。
- `feature/desktop-3d-scene-packs-20260924` 精确 tip `69b7703452a4d8fb9e318e3e2beda946c49ef193`，远端存在且同 SHA，净内容已纳入本版三个场景并补齐真实后端。来源作者所有权未释放，保留活跃引用和工作树；不重复算作下一版未发布场景。
- `fix/shared-modal-scrim-20260924` tip `ec3d2162e18666457f25b11c8477cebc42dc94a9`，包含 `fix/neutral-modal-backdrops-20260924@bd351e576dc9f17e3cb16eb3333116887c34f3b7`；对应远端原引用与默认归档引用均未发现。两者本地提交及工作树保留、由 RC5 tag 可达；本轮没有伪称远端归档完成。
- 两个额外场景的现有本地归档引用 `archive/feature/celestial-palace-scene@e21afa1a2e2697f69fc1424c53793720758b9d6b`、`archive/feature/blender-abyssal-ruins@f26f64e8be9d6d435402d30052072173a6bc36ee` 保留。远端对应归档未发现；误合入已撤回，不能用祖先关系证明它们上线。
- 原动态方案 `827b8c76bbce203adbb0296ad0fe924effc9af40`、其他壁纸方案 `075b38a164b8bec00f099d961a801fc5c2c82649`、GPU 收尾 `05ebce59a2a553b21304575cbb777b08117237d6` 及全部无关工作树原样保留。
- 未完成来源归档责任人为本发布任务/项目维护者；下次收到来源所有权释放并核对 clean、精确 tip 和恢复依据时处理，未唤醒旧任务清理。
- 本验收记录独立使用 `docs/release-v1.22.0-rc.5-acceptance` 候选，按同 SHA 候选 CI → 主线快进 → 主线 CI 执行，再保存至 `archive/docs/release-v1.22.0-rc.5-acceptance`；最终精确 SHA、run ID 与远端复核由本任务交付和 `acceptance-completion.json` 保存，不纳入或改写已公开产品标签。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-24T20:10:26+08:00
- 候选冻结时间：2026-09-25T11:19:52.776+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

本版属于预览产物，不计为稳定正式发布或生产部署。没有产品生产变更失败，流程异常另行统计。首个纳入时间按净保留功能的首个提交计算；两个被撤回场景的更早提交不计入此口径。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：13
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

按事件或同根因批次统计实际记录的 13 次异常。已对照最近五个稳定版 v1.21.0、v1.20.0、v1.19.0、v1.18.0、v1.17.0 的记录，未发现下列完全相同指纹；不把 RC4 的相似网络/浏览器局限计入稳定版复发次数。业务缺陷被正常测试发现和主动负向对照不计作发布流程异常。

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "assembly/candidate-scope/extra-scenes-included",
    "position": "before-production-write",
    "count": 1,
    "impact": "曾把两个独立场景误算为本轮新候选，用户确认只发布原三个场景。",
    "recoveryEvidence": "68d03671 撤回额外场景；最终目录和实际 Mock 浏览器只含三个场景。",
    "permanentAction": "本次按用户确认的精确 scene ID 冻结范围；后续组装前逐项核对来源清单与净内容，不以分支可达性代替范围。",
    "historicalReleases": []
  },
  {
    "fingerprint": "dependencies/report-dependency-freshness/network-source-failure",
    "position": "before-production-write",
    "count": 1,
    "impact": "首份报告四个来源 fetch/timeout 失败，只有 6/10 完整源，未作为最终完整报告。",
    "recoveryEvidence": "dependency-report-r1.json 与 dependency-report-r2.json；r2 达到 10/10。",
    "permanentAction": "保留失败原件；发布维护者于 2026-10-01 复核网络来源，退出条件为固定 Runner 报告来源完整；未以报告重置既有依赖期限。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/agent-dispatch/missing-explicit-model",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次 hunter 未显式配置指定模型，输出作废。",
    "recoveryEvidence": "security-run-8/run-metadata.json 与代理登记；有效协调、hunter、独立验证和 critic 均显式 gpt-6-luna/max。",
    "permanentAction": "后续审计每次 dispatch 显式写模型与推理强度，配置不可用即报告；不把已作废结果算作覆盖。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/read-only-source/agent-output-in-source",
    "position": "before-production-write",
    "count": 2,
    "impact": "两名代理将临时 overlay/日志或结论写入只读审计源，相关验证尝试弃用；一次 go test 编译后执行被 permission denied 拒绝。",
    "recoveryEvidence": "security-run-8/agents/rc5_cf_audit/artifacts 下两个 isolation-incident.json；精确临时文件清理后 HEAD/tree/clean 复核，独立只读验证重新完成。",
    "permanentAction": "本轮改为代理返回结构化结果、协调者写仓库外绝对证据路径；后续审计由项目维护者复核只读源和输出位置，退出条件为无源树写入且独立证据有效。",
    "historicalReleases": []
  },
  {
    "fingerprint": "security-audit/record-serialization/powershell-quoting",
    "position": "before-production-write",
    "count": 1,
    "impact": "终结时间戳时 metadata/end-baseline 内容序列化短暂损坏，不能直接作为完成证据。",
    "recoveryEvidence": "从此前捕获的完整内容重建，root 逐项复核范围、结论、时间和结构验证；candidate-checkpoint.json 固定最终哈希。",
    "permanentAction": "后续记录通过 JSON 序列化器写入、解析后复核并保存摘要，避免跨 Shell 拼接完整 JSON；不抹去本次记录维护限制。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/cua-inventory/fetch-request-failed",
    "position": "before-production-write",
    "count": 1,
    "impact": "同根因批次的 CUA inventory/reset 后请求失败，先前无法进行实际浏览器检查。",
    "recoveryEvidence": "使用文档支持的直接 IAB 创建入口恢复；preview-ui-acceptance-r5/browser-acceptance.json 和本任务工具截图。",
    "permanentAction": "继续使用 CUA 文档入口；工具维护者于 2026-10-01 复核 inventory，退出条件为该入口可正常返回；未改用其他浏览器自动化通道。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview/native-start/approval-policy-rejection",
    "position": "before-production-write",
    "count": 2,
    "impact": "两次原生真实服务预览 Start-Process 被自动审批拒绝，实际未启动，真实后端浏览器集成未验证。",
    "recoveryEvidence": "工具返回 blocked by policy；精确 aea1a75b Windows 测试程序编译成功但没有运行成功证据。",
    "permanentAction": "没有更换启动机制绕过拒绝；用户获知限制后明确要求上线 RC。后续稳定准入由项目维护者在允许的环境补真实浏览器集成，以完整旅程通过为退出条件，2026-10-01 复核。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preview/local-feature-preview/processes-exited",
    "position": "before-production-write",
    "count": 1,
    "impact": "继续任务时 r4 的 Web/API 进程已退出，旧 ready 不能作为当前可访问证明。",
    "recoveryEvidence": "原入口记录 r4 stopped；r5 独立 manifest ready 并完成实际 Mock 浏览器旅程。",
    "permanentAction": "每次续接先复核当前进程和 HTTP 状态，失效实例通过唯一启动器建立新证据目录；不覆盖旧 manifest。",
    "historicalReleases": []
  },
  {
    "fingerprint": "public-verification/docker-preflight/wrong-repository",
    "position": "before-production-write",
    "count": 1,
    "impact": "早期查询了错误 Docker 仓库，结果不作为公共镜像状态证据。",
    "recoveryEvidence": "docker-preflight-r3.json 与 public-preflight-r4.json 均核对 docker.io/kjlion/kejilion-panel。",
    "permanentAction": "从发布 workflow 的 DOCKERHUB_IMAGE 真源取仓库；后续公开校验固定正确 owner/repository，并比较 preflight 摘要。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/report-release-metrics/timestamp-fraction-precision",
    "position": "before-production-write",
    "count": 1,
    "impact": "本地验收记录校验拒绝 PowerShell 原始七位小数时间戳；尚未提交或推送文档。",
    "recoveryEvidence": "将冻结时间按毫秒精度写为 2026-09-25T11:19:52.776+08:00 后重新校验；原始七位时间仍保存在 release-profile.json。",
    "permanentAction": "机器验收记录统一使用 ISO 毫秒精度时间戳，提交前运行既有 metrics 校验；不修改或放宽门禁。",
    "historicalReleases": []
  },
  {
    "fingerprint": "acceptance/check-collaboration-state/unsupported-role-value",
    "position": "before-production-write",
    "count": 1,
    "impact": "验收文档本地提交 0130c9b6 后误用 role=task，参数校验拒绝，远端推送未执行。",
    "recoveryEvidence": "改用入口支持的 role=writer 后检查通过，clean=true、ahead=1、behind=0；补记后再验证最终候选。",
    "permanentAction": "角色参数按唯一入口的 management/writer/auto 使用，本次发布收尾属于 writer；不改变脚本校验。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 后续稳定版前需补真实服务浏览器集成、其他场景逐项机位、真实 125%/200% 缩放、reduced-motion、资源更新/remount、隐藏/遮挡暂停和持续 GPU 资源验收；敌对 iframe 导航与 SSE 会话撤销时序仍未动态验证。
- 既有数据格式未迁移；可选 artwork 的掉电耐久性未做故障注入。自动线路在途缩略图可能迟到写回临时 auto 偏好，显式来源和资源完整性校验不受影响。
- 本轮只发布 RC；这些风险如实保留，未作为稳定版或生产准入结论。
- 本地资源：公开镜像 E2E 的临时容器、网络与数据由原脚本 trap 清理。当前用户浏览器仍打开本任务 4185 Mock 预览，保留其依赖、冻结产品工作树及 r5 manifest；所有 L3/audit 成败原件、Runner 和恢复缓存继续保留。没有删除跨任务资源、没有全局 Docker prune；本轮手动删除释放字节为 0。验收文档工作树仅用于本轮收尾，来源归档仍等待所有权释放。
