# KPanel v1.11.0 发布验收与撤回记录

日期：2026-09-09。发布级别 L3。产品提交 261c03ef04913fef66664cc8301abb5464981446，标签 v1.11.0。上一稳定 v1.10.0 / a21d5bfc744b2a9316578dfca83426c001e2ee6d，发布后文档基线 ce3c15a5be3356ae3c9b2466474758b9514e8e7d。

## 最终决定与实际生产状态

用户在完成公开发布和生产备份后要求还原 1.10.0，随后明确完整撤回本轮发布、主线和默认更新通道回到 1.10.0。本次撤回依据用户决定，没有观测到 1.11.0 生产故障：1.11.0 升级命令尚未执行，生产始终运行 1.10.0。不能把公开发布或旧版健康核对记成 1.11.0 生产完成。

主线使用聚焦 revert 提交 d5fe5b367320fea281bdd9e0c89848abb404391b 撤销本轮五个提交；Git tree a43f2be07ec5d7301069481798812273f15e3f79 与 ce3c15a5 的文件树完全一致，无冲突。随后仅补入本撤回记录以满足已发布标签必须有验收记录的门禁。未重写 main 历史、未删除标签或版本镜像，未修改作者工作树。

公开通道恢复作业 [34358270935](https://github.com/kejilion/KPanel/actions/runs/34358270935) 已成功。GitHub Latest 恢复 v1.10.0；v1.11.0 标记 withdrawn/prerelease，原标签与附件保留。Docker latest 恢复原不可变 OCI index sha256:eaecfa6a156c35b106e74820619d35734c6cbdc2fd577465f08ce95bb46e964e，amd64 sha256:54917fd565b41d7f5716d17e2e1b6af873736141314e22bdce20621555457d17、arm64 sha256:c3f0706d5d12a9ec749c4ff4043c9fdab77fee3a5c24c80272cfc2dcd864601b。未重建或覆盖 1.10.0/1.11.0 版本镜像。临时操作工作流只在 ops/restore-v1100-channel，不进入 main；通过已有 GitHub Actions 的同仓库凭据执行标准镜像提升与 Release 编辑，没有复制或输出凭据。

## 原发布范围与未纳入内容

原纳入压缩包浏览和后台压缩/解压 fea65e39c34d21d8db135a9c9c3b8132e4119ea6、应用脚本管理按钮位置 c7e2f6cd6125b42a4d1501bbf3fe474d7669983f、终端选区 0563220a499fd755d768068976cb8365ba921baf；无冲突重放为 0c9e6e5、a7a5387、8af45dd。发布前另修复正常容器的 sha256 资源版本被 Panel manage 仅接受 marker:sha256 的校验拒绝问题 df83574，261c03e 准备版本。新 HTTP handler 回归修复前实际 FAIL 422，修复后透传成功和 Agent 409 保持通过。以上变更全部随本轮撤回，不宣称仍存在于恢复后的 1.10.0。

系统中心新增内容和未提交的轻节点文件稳定性修复未纳入。所有 KPanel 主机操作仅 arena-154 / 154.36.153.9；prod-108/108 未连接、未读取、未备份、未部署。

## 跨仓库联动

scriptLinkageState=not-required（无需发布脚本（不适用））。本轮没有修改脚本、部署协议或 apps 仓库。1.10.0 与本轮候选均固定脚本 9f612efc4f861459c0a525491c7cdf5eda756cf7，SHA256 9f3eaabaae32fd51511d2c3749cfb874bd600fb4c3b0b139b2f640b9eb877663。撤回无需回退脚本或应用目录。

## 原候选自动门禁与质量边界

精确 261c03e 的 L3 v1.11.0-261c03e-l3-r1 于 2026-09-09T12:27:27Z—12:42:47Z 首轮通过，exit0，11/11 证据摘要核对。Runner sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3；bundle a0c14e79aa9cab43860b64935cd7a113c7682e7b38f040d39bda56c28ff5fec7；plan d9d002571ae214904a4691378cd9e74b6d510cd5c95c11f0252fafdc3d67f7df；remote script d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c。

150 个前端测试文件/1311 tests、全量 Go、核心 race、typecheck/build、vet、双架构、镜像契约和 app-conf/rootfs 生命周期通过。govulncheck 可达漏洞 0，存在未调用依赖通告；npm audit 0，Trivy 本轮扫描无发现。额外 Archive race 在 filemanager/agent 通过，cluster 为 no tests to run，不作为新增 cluster race 覆盖。

候选 CI 34352944228/freshness 34352944256、主线 CI 34353667970/freshness 34353668055、Release 34354471852/标签 freshness 34354471507 均成功，绑定 261c03e。没有更新依赖、工具链、基础镜像或 Action 版本，继承原受管基线；依赖检查成功不是全部上游永无更新的声明。恢复分支初次 CI 34358144350 因缺少本已发布版本记录被拦截，补记录后以新提交重新核验；临时通道分支独立 freshness 34358270805 失败，不冒充完整候选通过，该操作分支不进入 main。

| 维度 | 已验证事实与边界 |
| --- | --- |
| 业务正确性 | 非 root 真 Panel HTTP → Agent Unix socket → systemd/磁盘；ZIP/TAR/TAR.GZ 压缩、包内浏览、选择解压、冲突保留原文件通过 |
| 安全 | 恶意 ../ 路径拒绝且无输出/暂存泄漏；真实鉴权、版本透传与 Agent 冲突拒绝；公开产物及摘要核验 |
| 稳定恢复 | 取消正在运行的 2GiB 稀疏文件压缩无输出；精确 Agent MainPID SIGKILL 后 systemd 恢复、任务 interrupted、日志拥有的暂存清理；历史重启恢复 |
| 性能资源 | 夹具 1536MiB/1CPU/pids1024；单次 2GiB 稀疏压缩约 17.18 秒，仅一个样本，无长期 soak/P95 结论；processedBytes 按完成文件累计 |
| 体验 | mock 390×844/1280×800、浅深主题、按钮右置、应用终端拖选/失焦；真实 Apps 详情与桌面原生菜单正常退出、HostTerminal ANSI 拖选与真实失焦、ZIP 浏览和选择解压后目录刷新及焦点返回 |
| 数据迁移 | 无数据库 schema 变更；生产没有运行新归档业务、没有恢复数据库；恢复后保护文件及 SQLite 核对通过 |

隔离夹具 kpanel-v1110-real-r2 使用 Ubuntu 24.04 systemd、nested Docker/vfs、internal network、无宿主挂载、非 privileged；FRP 是命名的容器夹具，不是实际隧道工作负载。缺 ss 与 nested cgroup 地址查询警告不算 FRP 地址验收通过。未验证原生 125%/200% 浏览器缩放、真实双节点/旧节点归档组合，不作完整互通或全设备声明。最终业务 final.json、故障 failure-final-r3.json 与浏览器证据绑定 261c03e；截图及 cmp 字节相等另有记录。容器 RestartCount=0/OOMKilled=false，容器/network/一次性凭据清理成功，15/15 实机证据摘要核对。作者预览保持，只有本轮 canonical mock preview 停止。

## 公开产物与生产核验

v1.11.0 曾公开 8 个附件，SHA256SUMS、API digest/size、许可文件内容通过；公开 OCI sha256:dd1a60a20fa89eba4b78f36d7bcc1d091f27ba8dcc51b3ed99bbf7e672411213 的 amd64/arm64、revision/version/script/User 标签及 image_e2e=pass 已核验。实机业务用 L3 构建，公开镜像单独 E2E，不宣称跨构建字节相同。公开产物现仅保留为撤回历史。

官方生产备份 /root/kpanel-backups/pre-v1.11.0-20260909T132322Z 保存 1.10.0，备份包和旧镜像摘要检查通过，停写备份后旧服务恢复。用户叫停发生在调用升级入口之前，没有运行 1.11.0 标准升级，没有数据回灌。

撤回后官方 postdeploy 只读安全核对 v1.11.0-withdrawn-v1100-check 于 2026-09-09T13:33:51Z—13:33:53Z 通过，8/8 摘要已核对。生产 Panel 1.10.0 / revision a21d5bfc744b2a9316578dfca83426c001e2ee6d / 上述旧 OCI，healthy，RestartCount=0，Agent active；保护文件无变化、SQLite quick_check、日志/资源断言通过。公网 HTTPS /api/v1/health 返回 HTTP 200、status ok、version 1.10.0。标准更新入口使用已恢复的 GitHub Latest 和 Docker latest，不在生产再执行一次无必要的更新。

证据根 C:/GitHub/_release-artifacts：v1110-preflight、v1110-l3-r1、v1110-preview、v1110-real-r2、v1.11.0-public-fixed-r1、v1110-production-preflight、v1110-production-backup、v1110-withdrawn-v1100-check、v1110-rollback。主机证据 /root/kpanel-release-evidence；本地和主机唯一证据及旧版恢复包保留。

## 交付节奏与异常

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-09T18:12:11+08:00
- 候选冻结时间：2026-09-09T20:26:26+08:00
- 生产完成时间：不适用
- 提交到生产用时：不适用
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

这里“否”仅指未发生生产产品载荷失败、生产回滚或紧急热修复。用户明确撤回了公开发布和源代码集成，生产未升级；该撤回事实不能被误读为正常完成部署。14 天/20 版本统计会保留稳定 tag，但本版无生产完成时间，不计为生产部署完成样本。

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：未记录
- 其中生产写操作开始后异常次数：未记录
<!-- kpanel-release-process-metrics:end -->

<!-- kpanel-release-process-incidents:start -->
[]
<!-- kpanel-release-process-incidents:end -->

完整流程累计未形成可保证完整的总数，因此不推断为零或首轮成功。中断前已经记录的 10 次如下原样保留；此外补充说明后续已知异常，不能用该阶段小计当最终总计。

```json
[
  {
    "fingerprint": "browser/local-feature-preview/navigation-not-ready",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次 mock Apps 完整导航后 app 为空且无控制台错误，首轮不计加载通过。",
    "recoveryEvidence": "显式 reload 后同候选 Apps 页面与终端完整可用；manual-validation.json 和截图。",
    "permanentAction": "未证明产品缺陷或永久修复；本轮发布任务负责，以真实静态构建页面再验收作为退出条件，2026-09-09复核。",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture/guest-panel/http-before-listen",
    "position": "before-production-write",
    "count": 1,
    "impact": "初始化 token 文件先于 HTTP 监听，夹具首次 bootstrap 收到连接拒绝。",
    "recoveryEvidence": "r1 guest.log 与服务启动日志；r2 final.json 全部业务通过。",
    "permanentAction": "唯一 guest-panel-body.py 在 bootstrap 前增加有界 HTTP health 等待，r2冷启动实测通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/ssh-tunnel/internal-network-port",
    "position": "before-production-write",
    "count": 1,
    "impact": "Docker internal network 未发布宿主回环端口，初始浏览器隧道不能连接。",
    "recoveryEvidence": "NetworkSettings.Ports 为 null；改为已核实容器IP的SSH转发后实机HTTP和浏览器正常。",
    "permanentAction": "隔离浏览器入口改为读取实际网络地址并预先验证 health；不开放容器外网或生产端口。当前夹具工具保留，未宣称仓库已有通用自动适配。",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture/archive-failure/completed-progress-wait",
    "position": "before-production-write",
    "count": 1,
    "impact": "processedBytes 按完成文件累计，夹具等字节非零时2GiB单文件已完成，取消被正确拒绝409。",
    "recoveryEvidence": "failure-api.jsonl complete 后 cancel409；r3改等running与真实暂存文件，cancelled且无输出。",
    "permanentAction": "唯一故障夹具使用实际running和暂存状态定位注入窗口，保留按已完成文件累计的进度语义；r3回归通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture/systemd-kill/auxiliary-signal",
    "position": "before-production-write",
    "count": 1,
    "impact": "systemctl kill 主进程后对辅助进程发信号报Invalid argument，夹具退出失败。",
    "recoveryEvidence": "r2调用错误；r3对已验证exe和MainPID精确发送SIGKILL，任务interrupted且暂存清理。",
    "permanentAction": "唯一故障夹具核实MainPID及exe后os.kill主进程，避免容器辅助进程信号差异；r3实机通过。",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture/powershell-adapter/nested-quote",
    "position": "before-production-write",
    "count": 2,
    "impact": "生成修订故障夹具的内联JavaScript被PowerShell解析拒绝，未执行命令。 后续额外内容校验内联Python再次被SSH/PowerShell引号破坏；未执行验证，不能计通过。",
    "recoveryEvidence": "原解析失败保留于执行记录；prepare-failure-r2.mjs文件入口成功生成并执行。 第二次改用直接argv的find和cmp，精确文件内容一致，exit0。",
    "permanentAction": "本轮夹具生成器已固定为外部mjs，生产入口全部采用已预检的文件；额外取证禁止内联嵌套解释器，直接argv调用find/cmp验证通过。此前通用工具纪律没有落实到所有辅助操作，不能宣称全流程永久修复；本轮发布任务负责，2026-09-09复核，后续退出条件为所有复杂跨shell操作使用文件入口。",
    "historicalReleases": []
  },
  {
    "fingerprint": "fixture/native-menu/missing-ss",
    "position": "before-production-write",
    "count": 1,
    "impact": "一次性最小系统缺少ss，真实FRP菜单有监听地址警告；该轮不作为地址查询通过证据。",
    "recoveryEvidence": "app-detail-native-menu.png；fixture-ss.py补齐实际ss及缺失依赖，ss -lnt预检和桌面菜单再验。",
    "permanentAction": "受管隔离夹具新增ss与动态依赖预检；没有修改生产或脚本业务实现。",
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/desktop-icon/role-mismatch",
    "impact": "桌面图标AX显示checkbox，但DOM角色定位未命中，首次定位超时。",
    "recoveryEvidence": "改用当前AX索引双击，真实HostTerminal与文件窗口成功打开。",
    "permanentAction": "本轮桌面入口固定使用已观察的AX操作，后续文件入口同方式通过；不推断产品缺陷。",
    "position": "before-production-write",
    "count": 1,
    "historicalReleases": []
  },
  {
    "fingerprint": "browser/in-app-cdp/focus-command-timeout",
    "impact": "tab2切换经典模式Input命令超时，随后焦点仿真命令超时，同一挂起批次。",
    "recoveryEvidence": "保留同一browser1，新建tab3后真实页面、归档浏览和选目录解压通过；backend健康。",
    "permanentAction": "根因未确定，未宣称永久修复。本轮发布任务负责，2026-09-09复核；新标签页恢复为工具故障处置，退出条件为新页真实用户旅程成功。",
    "position": "before-production-write",
    "count": 1,
    "historicalReleases": []
  }
]
```

后续已知异常：夹具收尾错误地从 guest 复制 host-only input-sha256.json，停在删除前，校正来源后清理成功；SCP 会话中断导致尾部三份文件缺失，补拉并核对 15 项摘要成功；只读路径/格式查询的 PowerShell-SSH 引号和 Git tree 花括号解析错误，改为完整 inspect JSON 和带引号的 Git 参数后完成核对；回退 L0 首次错误传 VERIFY_BASE_REF 到只支持 VERIFY_LEVEL 的适配器，改为既有位置参数；YAML 预检所选 Python/node 环境无相应包，随后复用已有 @lezer/yaml 语法树验证并逐条 bash -n；GitHub CLI 未登录且 CUA 内核超时，改为隔离操作分支的现有 Actions 凭据完成通道恢复。以上不计作产品测试通过；没有在失败后继续部署新版本。

嵌套引号与猜测路径在 v1.10.0 已有同类记录（preflight/public-artifacts/nested-python-quoting、preflight/release-diagnostics/unverified-path），不能通过本轮新名称掩盖重复。复杂命令已转文件、稳定入口已复用，但未证明所有辅助诊断永久消除同类问题；发布任务负责后续复核，原始错误与恢复证据保留。未增加永久规范或工作流，未更新记忆。
