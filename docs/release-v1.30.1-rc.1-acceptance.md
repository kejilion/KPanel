# KPanel v1.30.1-rc.1 发布验收记录

日期：2026-11-14

发布级别：L3

候选提交 / 标签：`e82c1a5dea2ac224450d054e886c13626c43d35e` / `v1.30.1-rc.1`

上一稳定版本 / 回滚点：`v1.30.0` / `sha256:4523345173d5e8eefcf6fccc46ff350c32491a8abf81e7f226bfe0685a8c212e`

`releaseChannel`：`preview`

`releaseTrain`：`1.30.1`

候选分支与发布后处置：`release/v1.30.1-candidate` / 预览序列保留。

- 本次来源分支呈线性包含关系，未重复 cherry-pick：`perf/music-list-rendering@57f490c2f675442e085c18566b46bdcbfb5eb0dc` → `feat/music-follow-current@9c3fb14fe952ca8a3e7eb819e26d6d5e831cf94c` → `fix/music-flac-quality@824f039df0276777eee825893ec652f855c32251`。
- 以上三个原分支及其 `C:/GitHub/_codex-tasks/kpanel-music-*` 工作树保留；没有明确释放所有权，不删除或改写。发布完成后从待发布产品候选中移除，但本地资源仍待所有权核对。
- 原 tip 可从本次不可变 tag 的祖先对象恢复；不将保留状态称为已归档。下一次稳定列车收尾或原作者明确移交时复核归档。
- RC 候选工作树 `C:/GitHub/_release-candidates/kpanel-v1301` 保留；验收记录在独立文档工作树形成，不改变产品 tag。
- 本轮未处理更早历史分支、其他会话工作树或生产资源。

产物已公开，公开验收成功；生产部署不适用（预览版禁止生产部署）。

## 发布画像

- 业务域：音乐库、播放队列、正在播放歌曲定位、FLAC 音质检测与失败恢复。
- 变更面：Web 展示和现有媒体只读 Range 请求；另有发布版本字面量及发布元数据。
- 用户旅程：浏览长音乐列表和队列、快速滚动、切歌后定位、继续手动浏览、恢复队列后的文件版本解析、FLAC 检测遇到临时失败后重试。
- 未变化契约：API、数据库 schema、持久化缓存格式、Agent 协议与权限、端口、Compose、应用市场和 `kejilion.sh`。
- 风险：局部界面与只读探测，发布等级 L3；没有新增宿主机写入口或依赖。

## 发布范围与未纳入内容

- 长音乐库及队列采用测量行高的虚拟渲染，滚动同步刷新范围并使用有界缓冲。
- 切歌时让当前歌曲进入可见范围；手动继续浏览时不会持续拉回；筛选列表没有该歌曲时保留筛选结果。
- 修复 ID3 前缀后及跨 Range 的 FLAC 识别；空音质结果不再持久化为完成；临时失败有界重试，恢复引用使用解析后的真实版本；音质头请求避免等待慢标签的浏览器缓存锁。
- 精确提交：`29b375e7`、`7f1ea5aa`、`59f87799`、`57f490c2`、`9c3fb14f`、`142dd9ec`、`b728be5c`、`824f039d`；发布元数据为 `e82c1a5d`。产品差异为 12 个 Web 文件，唯一 Go 差异为版本字面量。
- 不包含新媒体提供商、全库自动刮削、依赖升级、其他历史候选或生产部署。

## 外部审计与独立复核

- 独立复核：`PASS WITH FOLLOW-UP`，覆盖基线 `f8adf55b34a7b49688bcf58c39aa95226dff1caa` 到来源 tip `824f039df0276777eee825893ec652f855c32251` 的完整产品差异；未发现有正常路径证据的发布阻断缺陷。
- Claude/Gemini 非交互入口不可用，按规范使用干净上下文 Codex 独立复核回退；报告保存于 `C:/GitHub/_release-artifacts/v1.30.1-rc.1/independent-review.md`，不把模型回退称为跨提供商复核。
- CF 安全盲审补漏本轮未执行；没有安全敏感包代码改动，Go 仅版本字面量，不能宣称全仓无漏洞。
- 来源提交保留既有 OCR 与安全审查 trailer；发布元数据提交记录 OCR skipped（仅发布元数据）及安全 skipped（仅版本字面量）。不以记录存在替代最终 CI/L3。
- 非阻断观察点：异步标签令条目加入长筛选列表的可视区之前时，缺少专门的稳定滚动锚点验证；没有真实浏览器复现，不记为已修复或已验证。

## 跨仓库联动判定

- `scriptLinkageState`：`not-required`（无需发布脚本，不适用）。
- 内置脚本：`kejilion/sh@1faee13f3faef164efbfa899e244fc19ad58aaff`；SHA-256 `5c5acccac56bba480046f28ac0d5d34af736fcfb5ab187647684b52c6689ad23`。
- 脚本候选、跨仓变更集和阻断范围不适用；本次没有脚本发布。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或不适用依据 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确候选 43 项音乐测试及 9 项本地模拟浏览器检查 | 不等于真实外部音乐库/所有编码文件互通 |
| 网络入侵与供应链安全 | 已验证 | 候选/主线/发布扫描、双架构身份、SBOM/provenance 及公开摘要核对成功 | 不作零漏洞保证；没有本轮全仓 CF 审计 |
| 稳定性、失败恢复与兼容 | 已验证 | 受控 503 后 FLAC 可见恢复、恢复队列版本与有界重试测试 | 未执行生产安装/回滚和长期 soak |
| 性能与资源预算 | 已实现未实机验证 | 94 条模拟库只挂载 29 行、快速滚动范围检查通过；请求/缓存有界 | 没有低配真机帧率或内存对照数据 |
| 用户体验与可访问性 | 已验证 | 本地 390px、明暗主题、CSS 200% 等效几何、切歌定位/手动浏览 | 不是原生浏览器缩放；未做实体手机、完整读屏/焦点矩阵 |
| 数据、配置与迁移 | 已验证 | 沿用缓存格式，有效旧缓存保留，旧空结果可恢复 | 无生产迁移 |

## 自动门禁

- 精确候选：`e82c1a5dea2ac224450d054e886c13626c43d35e`。
- 本地：clean 候选检查、`git diff --check`、版本一致性、业务文档新鲜度、source identity 和 task-preflight handoff 均通过；前端 vue-tsc/build 及 6 个文件共 43 项音乐测试通过。
- 发布说明草稿由正式生成器及 Go 解析器验证通过，零 digest 仅用作草稿占位，不代表最终公开摘要。
- 候选 push CI：[43481793099](https://github.com/kejilion/KPanel/actions/runs/43481793099) 成功；同 SHA 依赖新鲜度 [43481793051](https://github.com/kejilion/KPanel/actions/runs/43481793051) 成功。
- 完整发布 workflow：[43484108817](https://github.com/kejilion/KPanel/actions/runs/43484108817)，attempt `1`，`completed/success`，于 `2026-11-13T19:37:06Z` 结束。
- workflow 内候选 Linux job `132087994119`、Windows Node job `132087993964` 成功；L3 job `132087994045` 成功，随后 promotion job `132098973108` 原子提升 `main` 和 annotated tag，回读两者均为候选 `e82c1a5d`。
- L3 原始回执：attempt `1`，`github-hosted`，Runner `ubuntu24 / 20261111.370.1`，Go `1.27.4`、Node `24.22.1`、Docker `28.0.4`；`2026-11-13T18:36:12.918Z` 至 `18:55:25.026Z`，`status=passed`，入口 SHA-256 `5bfcf66870b49edb9abd2895e818ac2bdc8323579aad06e43946ee2f073f5a7d`。
- GitHub L3 不使用旧 SSH/WSL plan 或源码 bundle；其回执、source 三分组和原始日志保存于 Actions artifact `l3-43484108817-1`，本地核对副本 `C:/GitHub/_release-artifacts/v1.30.1-rc.1/github-l3`。Web/Go/deploy 三分组 `passed`，源码身份未变；L3 末尾 `image_e2e=pass`、`release_gate_runner=pass`。
- 主线 Linux job `132099091245`、Windows Node job `132099091364` 均成功；主线 Linux 于 `2026-11-13T19:09:57Z` 完成。发布复用本轮已经成功的 L3，`publish / l3` 为预期 skipped，不是漏跑。
- Release job `132107918390`、发布 Windows Node `132106895042`、公开验收 `132120725603`、preview completion `132121810103` 均成功；stable completion 为预期 skipped。
- 发布 job 的 source 三分组于 `2026-11-13T19:26:19.361Z` 通过，`identity_unchanged=true`；发布说明最终正文通过正式解析器和公开 API/弹窗读取，摘要 5 条、升级提示 1 条。
- 公开产物回执 `status=passed`：19 附件下载/API digest/磁盘回读一致，`SHA256SUMS` 15 条一致；两架构 OCI、固定脚本身份和 SBOM/provenance 均通过发布门禁。发布身份记录原最终正文 SHA-256 `506a4545989dcd7db49ec95a9c0ca1f9080b58bb6ac0bed7e643471c34d96491`、附件清单 SHA-256 `3447d639fdab489310833d370df401ec90fed3a715c328979766e77edb2ac676`。

## 依赖与技术栈变化

- 无依赖、基础镜像、Action 或脚本版本升级；Web lockfile 只同步产品版本。
- CI 固定 Go `1.27.4` / Node `24.22.1`；本地前端 Node `24.17.0`，本地发布说明解析器使用现有官方 Go `1.27.4`。
- 版本 Go 字面量变更后，1316 个第一方 Go 文件的 source identity 更新为 `6572dd416e031dfcd82893284053f61a037f48c10851b6f408c7227138922189`；未更改依赖发现、闭包观察日期或扫描结论。

## 隔离真机与浏览器验收

- 本地 acceptance mock 预览：精确候选 `e82c1a5d`，Chrome `149.0.7827.55`，`http://127.0.0.1:4180/music`；证据目录 `C:/GitHub/_release-artifacts/v1.30.1-rc.1`。
- `browser-result.json`：9 项检查通过、页面错误 0。覆盖库/队列虚拟列表、快速滚动、切歌定位、手动浏览、缺失筛选结果、70KB ID3 后 FLAC 跨区间探测遇到一次 503 后恢复、390px 布局、暗色与 CSS 200% 等效几何。
- FLAC 与列表数据为合成夹具，请求拦截提供字节及一次受控 503；不是外部服务、真实宿主、实体手机、原生缩放或低配性能证据。
- 不连接任何自有 SSH 主机；未执行宿主写入、systemd/OpenRC 真机操作或生产回滚。没有长时 soak，因本轮局部 UI/有界只读探测且无常驻服务改动。
- GitHub 公开产物隔离容器/真实浏览器：公开镜像报告 45 项、升级路径报告 31 项、真实业务初始报告 48 项、重启后报告 26 项，全部 `status=passed`、各报告页面错误 0；这些是分阶段检查数，存在重复断言，不能相加宣称独立测试总数。
- 公开浏览器报告 `mocked=false`，但 Docker 库存和测试 CA 仍为明确标注的夹具；隔离容器不挂宿主 Docker/systemd socket。测试 Runner 实际 ID `sha256:80c6b37c55ea555b2b151e6afa400f30f20920ee52e5fbdb2a585a01a3c0c8fc`。原件在 Actions artifact `public-43484108817-1` 和本地 `C:/GitHub/_release-artifacts/v1.30.1-rc.1/github-public`。
- 本地预览已通过标准 stop 入口停止，mock API/Web 均 `stopped`，状态保存在 `preview/manifest.json`；停止进程不删除证据。

## 发布产物与公开仓库复核

- GitHub Release：[v1.30.1-rc.1](https://github.com/kejilion/KPanel/releases/tag/v1.30.1-rc.1)，`2026-11-13T19:34:45Z` 公开，`draft=false`、`prerelease=true`，19 附件；Release ID `444652672`。
- annotated tag 对象 `37ff21af0ea662fb7cd2d915e4d344229011910b`，解析到产品候选 `e82c1a5dea2ac224450d054e886c13626c43d35e`；发布时远端 `main`、RC 候选均同 SHA。
- Docker `1.30.1-rc.1` / `preview` OCI index：`sha256:862081f408a273cdb0e4f94de798f6d7ba99f43d3d97e19213a2fca98a2babd4`。
- `linux/amd64`：`sha256:5a8ad32a6a50e8cbdd6b07eef22182c2993bae3562b3c6a017092d7b48ca864e`；`linux/arm64`：`sha256:a6207ca9142da720d6fc5c0987fc156afbbebdce7e4312e893f095e3972b07db`。
- 公开检查于 `2026-11-13T19:35:17.894Z` 为 `passed`；收尾时再以 Docker Registry HEAD 回读版本/preview/latest，并读取 GitHub Latest，确认版本与 preview 同摘要、GitHub Latest 为 `v1.30.0`，Docker latest 为本文开头稳定摘要，未改变稳定通道。
- 冻结 stable-before 快照保留上一 preview `1.30.0-rc.14` / `sha256:0fe00dc1800ce990f67d8882d50f49eb704732d504ca07e4516a78c3eaafd3da`；快照 SHA-256 `791fb4e6d3c2f114fa116d786ab1261b8e2a5f0f29832cbc78a82e230e2a49d5`，与 publication identity 绑定。

## 自更新通道验收

- 本轮不修改更新逻辑；公开镜像的真实发布信息 API、当前版本和升级弹窗均绑定本版精确版本/digest，摘要 5 条、升级提示 1 条。
- 旧 preview 后端发现新 RC；升级弹窗在 1280/390/320px 无横向溢出，真实 no-systemd 安装派发返回受控 503 并清理队列，新前端不刷新即可继续切换通道。该预期失败注入不表示生产升级成功，也不计为发布故障。
- 加入预览只切换来源并检查，安装开关仍关闭；相同版本不重复安装，退出预览不自动降级；stable 来源仍为 `1.30.0`。
- systemd/OpenRC 与轻量 Node 真机安装、长时观察窗口和生产回滚本次未执行，边界沿用 `docs/release-channels.md`。

## 生产部署安全核对

- 全部生产动作：不适用（预览版禁止生产部署）。产物发布不代表生产部署。
- 本次没有连接、备份、部署、升级或核对 `prod-108`，也没有其他生产写操作。
- 本地 mock 及 GitHub 隔离容器结果不能外推为生产宿主验证。

## 回滚

- 上一稳定 tag：`v1.30.0` / `cdbdd2b493d3d4e9bd2e7ef2730d47326460aaaa`；固定镜像摘要见本文开头。
- 保留产品不可变 tag 与版本镜像；退出预览不会自动降级，如需人工恢复较低稳定版本，按现有更新流程先备份数据并选择固定摘要。
- 无生产部署，因此生产数据备份、回滚执行与健康复核不适用。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-11-14T00:13:35+08:00
- 候选冻结时间：2026-11-14T02:19:40+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：0
- 其中生产写操作开始后异常次数：0
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

## 遗留风险与后续准入

- 发布与公开验收已完成；本记录作为独立本地文档候选保存，不变更产品 tag、镜像或发布正文，不再制造一个产品 RC。
- 未验证：真实音乐库的格式长尾、实体手机、低配量化性能、原生缩放与完整可访问性矩阵、异步筛选插入锚点、长期 soak。
- 本地资源：来源工作树所有权未知而保留；RC 列车工作树继续用于后续 RC，证据原件保留。没有删除目录，不申报磁盘净释放量。
- 本轮不新增知识、工作流或周期任务。
