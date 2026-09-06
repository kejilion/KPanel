# KPanel v1.5.0 发布验收记录

日期：2026-09-06

发布级别：L3

候选提交 / 标签：`7ba46d1ad7d3e1d55dc6451b194e75a6c78df8b2` / `v1.5.0`。

上一稳定版本 / 回滚点：`v1.4.1` / `7cb68c0303946cf9cffeb211af5d2651e52236ac`。

## 发布画像

- 业务域：任务详情恢复、终端关闭确认、网站自有证书事务更新、轻量节点更新健康观测、发布入口预检。
- 变更面：展示、只读聚合、宿主机写入、可选协议字段、部署；L3。证书涉及私钥/回滚/续签资格，终端涉及真实进程和会话配额，节点涉及身份与自动更新，采用 minor 1.5.0。
- 未变化契约：普通已配对节点不要求重新配对；不新增端口、Agent/节点服务权限、配对授权或数据库迁移；自动证书仍默认，自有证书需管理员维护；应用市场标准事务不改。

## 发布范围与未纳入内容

- 用户可见更新见 CHANGELOG 1.5.0。Jobs 从所属服务恢复不在最近列表内的任务，部分来源失败明确提示；终端须目标确认关闭后释放配额；网站详情可校验并事务替换自有证书；轻量节点区分遥测在线、实际运行版本、更新结果与辅助服务观测。
- 精确增量 `351bf2a5ebea2c1f987b4d19c2887864639eded1..7ba46d1ad7d3e1d55dc6451b194e75a6c78df8b2`：85c393a、0a34e2a、5c0ef16、59c2e94、ad99f89、c6cc775、e8b84ee、e3fa1b4、57ac0e4、7ba46d1。9 个原候选提交无冲突移入，加一个版本准备提交；原作者工作树未修改。
- 未纳入：新 System Center 页面/routes/APIs/权限/文档，未提交内容、候选冻结后新增功能、Go/npm/Action/基础镜像升级。既有共享 server.go 修改限本批 Jobs/证书/终端，不借文件名放行其他系统中心内容。

## 跨仓库联动判定

- `scriptLinkageState`：`coupled`；变更集 `post-v141-certificate-and-node-health-20260906`。
- sh 基线 `50289bfd17bae589e22267c397b5c2781bd41ead`；已先行发布 `298f6f23751e36726660d73b5c0c83aef1b404f4`，包含 b3d0d35、ae24516、298f6f2；脚本版本号不改。
- KPanel 原内置3759e10692f61fe375c0424389842ddc70a73ed7；本版内置298f6f2，root SHA256 `80365884b126b7a0cfb1bf0976ffbce7dfcc946ac05274f8da99594086d09581`，CN `455759e4012e9a0727377088b71ec41cbeac786f2f45e3220a5468c467779a1b`。
- updater generation 3，SHA256 `858c9d168fb4e13b6c38062616a12d92f0526276d0b0213524ce564bbb6b2842`；续签脚本 `3a63b9e0c1557fae9e18983a8eee284476a946a811c11b3076535aab1432e0d0`。Dockerfile/source.json/来源文档与实际公开 raw 对齐。
- 根/CN只保留区域参数差异；LF源语法、证书 OpenSSL/flock/Docker替身 smoke、23项 updater 测试、嵌入运行时检查通过；实际 Agent/TLS 与节点/systemd矩阵另列，不能把替身当真实公网证书签发。
- 发布决定：脚本兼容变更先行，再固定其来源发布 KPanel；阻断或移除的依赖范围：不适用。

## 多维质量结论

| 维度 | 状态 | 证据 | 未验证风险或边界 |
| --- | --- | --- | --- |
| 业务正确性与双端互通 | 已验证 | 精确SHA Go/前端门禁；真实Agent证书/PTY、真实node/systemd；聚合UI恢复 | Jobs浏览器数据为Mock；node enrollment为合成HTTPS peer，不是用户全部节点实测 |
| 网络入侵与供应链安全 | 已验证 | race、govulncheck、npm audit、Trivy源码/最终镜像、固定脚本源 | 3项模块不可达提示不等于所有依赖无通告；实际调用路径0 |
| 稳定性、失败恢复与兼容 | 已验证 | CAS旧版本拒绝、Nginx reload失败回滚、服务重启重发现、终端关闭失败保留、timer/身份恢复 | 不承诺和任意外部写入者原子协作；公开资产另需验收 |
| 性能与资源预算 | 已验证 | browser 1GiB/1CPU/512PID；systemd guest768MiB/1CPU/512PID，无OOM/restart，清理通过 | 非长期SLA；timer加速是确定性窗口，不是小时级soak |
| 用户体验与可访问性 | 已验证 | Jobs/Health/终端/证书390/768/1280、浅深色、中英文、键盘/恢复、缩放组合 | 仅受影响矩阵；health图中既有保存名称中文不在新文案范围，非全应用英文认证 |
| 数据、配置与迁移 | 已验证 | 无数据库schema迁移；证书配置不变、失败原材料恢复、旧节点可选字段兼容、身份不变 | Panel回滚不会撤销已替换证书，需要事务备份；生产数据核对另列 |

## 自动门禁

- L3 `v1.5.0-7ba46d1-l3-r1`，11:48:00Z→12:04:57Z，passed/exit0；固定 `run-release-l3.mjs` → `run-release-gate.sh` → `make verify-release`，无替代L3 wrapper。
- Runner `sha256:0ae41a9fb92e5a9dd5fc60cbdbcde8ef6e2d703f34c17a59b3b26f73d72495d3`，Go1.26.7/Node24.20；bundle `451312973c85e3b4bc249134688dcfcafd6747c546f0641fc20e4ae53f2ffa62`，plan `383c3e9490208b885e7b6642897a44a7ab2118098d809dce94dd52c5efa16758`，remote脚本 `d8bb2cf214fb2833ea2f410d82f848ac38307308bbf261a5354727b730eb0c5c`。
- 日志SHA256 `5e6bb32af245a9fe32c2e5b79d7b1fe4d546756ac5db95c6fe5ee4c5d124ff3c`，manifest `66de834e46af62d2af82d54276292bb030d7bb28062c9d143a316fb351b66b3a`。本地 `_release-artifacts/v150-l3-remote`，远端 `/root/kpanel-release-evidence/v1.5.0-7ba46d1-l3-r1`。
- 全量Go、142前端文件1208测试、typecheck、2208短语/21页、特权包race（Panel283.213s/auth4.063s/docker7.771s）、vet、双架构编译、安装/rootfs生命周期、最终Docker构建通过。安全实际可达govulncheck0、npm0、Trivy0；负向安装拒绝日志属于已通过断言。
- 候选 [CI34031333954](https://github.com/kejilion/KPanel/actions/runs/34031333954) / [freshness34031333953](https://github.com/kejilion/KPanel/actions/runs/34031333953) success；主线 [CI34032544406](https://github.com/kejilion/KPanel/actions/runs/34032544406) / [freshness34032544423](https://github.com/kejilion/KPanel/actions/runs/34032544423) success，均完整7ba SHA。main先快进、CI通过后才创建标签。
- 生产入口与run-repo-bash补充回归49项通过；外部隔离夹具唯一preflight补9项回归，实际Linux/82个LF shell/自有可写缓存offline包解析通过。不修改冻结产品，不将外部预检称作仓库治理已永久采纳。
- [Release34032987574](https://github.com/kejilion/KPanel/actions/runs/34032987574) success，精确7ba，含源码验证、扫描、256MiB/1CPU/128PID/非root只读根/cap-drop ALL实际镜像契约、多架构构建、SBOM/provenance、latest提升、Release公开及候选分支清理；tag freshness34032987579 success。

## 依赖与技术栈变化

- 候选dependency-report生成2026-09-06T11:49:41.742Z，8/8源，emergency-security0，compatible-patch7/minor14/major4，共25项行动候选。受管sh来源随后已推进，不把旧main502误认为需要降级。
- 最近每日安全审计34018751198（07:17:32Z，基线351bf2a，job101447305053）success，明确不是7ba同SHA每日审计；7ba自己的govulncheck/npm/Trivy全部通过。EOL元数据最近复核2026-07-28，未伪造本日人工复核。
- 本版采用受管脚本298f6f2；不升级Go/npm、Action或扫描器。候选包含x/crypto.56、x/net.58、sqlite1.58、CodeMirror系列、lucide1.41、DOMPurify3.4.15、markdown-it15.0.1、TypeScript7.0.2及基座pin等，完整清单在same-SHA freshness报告；发现不等于已接受。
- 常规行动项由项目维护协调负责，2026-09-07复核；沿最早完整检测计时，不按本次重报重置期限：Patch7/14/30天、Minor14/30/60天、Major或非SemVer30/90/90天，安全1/3/3天。冻结产品不夹带依赖升级；安全可达0允许当前批次，超期限须按策略带证据处置而非无限延期。

## 隔离真机与浏览器验收

- 仅arena-154，适用environment-policy登记用途；网络隔离、无host mounts/宿主Docker socket、非privileged、私有cgroup。Ubuntu systemd guest固定镜像be6bd104fb79562b2ea6d563047e94f5d3a04bddb4c0c258ad70d13439b2239c，Nginx扩展fixture实测TLS；允许的caps只在隔离guest，不提高生产服务权限。
- `/root/kpanel-v150-review-20260906/real-agent-r7`：正式Agent unit、7ba原生二进制、安装器标准替换后的298脚本，实际Nginx TLS旧→新指纹、CAS旧版本409、reload失败回滚、restart后重发现、真实PTY错误owner拒绝和正确close子进程回收/幂等共5项通过。Docker nginx名称解析是明确的命令适配器，非真实Docker端到端；全部容器清理。
- `/root/kpanel-v150-review-20260906/node/health-r2`：9项生命周期、5份正式unprivileged sandbox health观测。孤儿锁resume、旧公开1.4.1不覆盖generation3、实际timer升级、并发join/update/uninstall拒绝、SIGKILL恢复、网络/校验失败保留身份和binary、fresh HTTPS单次enrollment/续装、卸载无残留。旧fixture header版本1.4.1/1.4.0与实际binary1.5.0-dev/1.4.1不同，保留边界，不充作新公开资产证据。
- 公开节点最终 `/root/kpanel-v150-review-20260906/node-public/public-r1` passed：直接使用公开amd64 asset `239a53d7b51a1d28432961c96bbe005c2c16043f17bd6f2f739ea0717dce4c02`，旧公开1.4.1 `fb39dc753a23753bd03ed3f0729360f5093b05e8926d12c0a72860ca0d3d7c21`；9项真实systemd生命周期与5份health观测再次通过，runtimeVersion和新enrollment均1.5.0，transport版本已对齐1.5.0/1.4.1。OOMfalse/restart0/cleanup=true，12:35:28Z结束；本地v150-public-node-final。唯一host fixture c73167a3b875addc5e1d95d62d40744535e0384d247776e44a61865a6cb92bb4，guest d6dff874c9f5b283cb5d4dc3071d14ae4d2a5702e1b1f35598f840564ce91a66，transport 5e47544d5be527a224413a760ed9b6f3c38814202d622198e6790e727b6b3dac；不复用dev二进制证明正式资产。
- browser aggregate r7：job `arena-154-35776`，12:11:52.574Z→12:12:45.375Z，passed/exit0，timeout260，cleanup=true；本地 `v150-browser-r7` 与 `v150-browser-final`；命令spec SHA256 `82bb14bf13072980e7b0f97377ea29d13f7aa2b604be0537207c0dd1dcd9acdb`。Playwrightcore1.55.0/Chromium140.0.7339.16，镜像b27e719ecbfef153e13fd24e8341736733bf2658b229677eb21ff57ff5d7fb29；精确7ba全应用构建+Mock API，不是生产浏览器。
- Jobs33断言：390/768/1280、50项部分列表外恢复、URL/刷新/桌面生命周期、owner503/404无假成功、键盘返回、中英文浅深色、200%CSS等效；GET-only。Health25断言：5组合、100/125/200%等效缩放、current/degraded/missing/stale、打开详情后推进105秒过期、焦点返回、最小字号≥13px。Actions22断言另3观察对象：Terminal/HostTerminal失败保留与成功重试唯一close、证书空表单disabled/CAS5字段/409重试/取消清私钥，0page errors。
- 无生产证书替换/用户节点卸载/并发杀进程；本批确定性恢复窗口不宣称真实所有浏览器、所有平台或长期soak。

## 发布产物与公开仓库复核

- [GitHub Release v1.5.0](https://github.com/kejilion/KPanel/releases/tag/v1.5.0) 于12:33:23Z公开，draft=false/prerelease=false；annotated tag a8b3c340690fd078b6641a3ab101213e14a592c0，peel为7ba。
- [Docker Hub](https://hub.docker.com/r/kjlion/kejilion-panel/tags) 1.5.0/latest OCI index均 `sha256:20c3648c1fb3d64a8730a625a3ab3afc38d60643ac1f29c7d0b77c4239ee7134`；amd64 `sha256:57cec2f92ef28f24e1615bd157137615bd1a9d846bcc1ae64a742adab031c509`，arm64 `sha256:bd7983390cab41d7764e6fc57331f5f3c88796b24913ee4dc75ff570e5f68ee9`；另两个unknown平台为对应attestation。
- 八个公开附件实际下载，SHA256SUMS五项全部OK，八项逐个与GitHub asset digest/size一致。SHA256SUMS自身afb034f9608b488a2dd57e6d8c8d60414c3e0ae9f000b738ad79704e1785c3b1；Agent amd64 af16a2cf2ff40b09333acdc063deb96450cbd1b7effed9bff5ae84aa7b70722a、arm64 7aacd4546f216fa28b37fa446b08028f48b5a1245fd767cd429b34f738290d12；node amd64 239a53d7b51a1d28432961c96bbe005c2c16043f17bd6f2f739ea0717dce4c02、arm64 82cf1a487f2427fe384191fd118eba985d974551fe5d90b969defa089a685324；deploy0212ca7fb8fd976cc4bb2e48c9f62662bc7e9c4568df7aa6233cff4df2bb4089。LICENSE/NOTICES与精确源码逐字节相同。
- 显式pull公开digest，revision7ba/version1.5.0/script298+803/user65532:65532全部匹配；固定image-e2e=pass，12:34:18Z完成。证据/root/kpanel-release-evidence/v1.5.0-public-7ba-r1及本地同名目录；没有拿L3本地镜像代替公开镜像。
- apps无契约变更：当前main2d8044adec98e3eb16f47cdbb297f6be9632a66f，规范/本地blob abf0efd22876f34aa3731f5b6d8ba04e373b965e，raw HTTP200与 `packaging/kejilion-app/kpanel.conf` 换行归一字节相同；无需apps提交。

## 生产部署安全核对

- 唯一生产/验证arena-154；108/prod-108禁用全部KPanel操作，本次未连接、未备份、未部署、未核对。
- 新preflight `v1.5.0-7ba-production-r1` passed，12:15:56Z→12:15:58Z，旧1.4.1 healthy，受保护文件、OCI、SQLite和资源baseline已保存。旧pre-v1.4.1含1.4.0，不当作此次备份。
- 新备份 `/root/kpanel-backups/pre-v1.5.0-20260906T123605Z`，停写归档、压缩完整性、旧镜像实际load与SHA256SUMS全部通过；备份后旧1.4.1健康恢复。kpanel.tar.zst 0462e19df7cb4e1872ae442db102ac1938a779daef3377ba696a6b3938ebe2aa，old-image.tar.zst b9a005dd1d8d583f5c4fb39442543e947d9ed81cbd1235a112b7a9668671a442；服务/安装标记/旧inspect一并保留。
- 标准应用市场update exit0，拉取20c3648正式digest，安装器完成并恢复访问策略。日志v150-production-standard-update.log SHA256 3a321069558eb3f1910d13cc7909c417344d42dd51dfa7dcc1cb47f2b53c3dc3。
- postdeploy同run，12:37:10Z→12:37:12Z，passed/exit0；线上health1.5.0/ok/initialized，revision7ba/公开digest一致。Panel running/healthy、restart0/OOMfalse，Agent active/running/enabled、NeedDaemonReload=no；protected.diff为空，panel/ai.db quick_check=ok，顶层空ai.db显式empty，近10分钟致命日志扫描通过。
- 生产实际写操作仅标准备份停写/恢复、标准应用市场更新与其访问策略恢复；证书替换、失败注入、节点卸载/配对、并发杀进程和旧版本降级只在隔离环境执行。当前证据为生产本机health/服务/OCI/数据核对，未额外用外部域名路径替代实机结果。
- 固定生产入口 `scripts/run-production-evidence.mjs`；标准更新 `KJ_APP_NONINTERACTIVE=1 KJ_APP_ACTION=update bash /home/docker/kpanel/bin/kejilion.sh app kpanel`。preflight不携带postdeploy字段，backup/postdeploy各按phase契约，无另造生产wrapper。

## 回滚

- 保留v1.4.1源码/tag、OCI index `sha256:ae7d997f0e3dd565ffbbcd8ed63c552cb8c64e2462ec436665fb9ceebab3e9d3`；恢复时使用本版新停写备份的数据库、密钥、配置、Agent、精确旧镜像，再验health/OCI/protected/SQLite。Panel降级不自动恢复证书，不能用旧续签器覆盖自有保护。
- 未发生生产回滚，生产实际1.5.0健康；GitHub Latest、Docker latest与标准更新入口均指向1.5.0。公共默认通道恢复决策：不适用。备份不是旧pre-v1.4.1；历史产物未删除/改写。

## 交付节奏数据

<!-- kpanel-release-metrics:start -->
- 首个纳入提交时间：2026-09-06T18:14:12+08:00
- 候选冻结时间：2026-09-06T19:47:34.611+08:00
- 生产完成时间：2026-09-06T20:37:12+08:00
- 提交到生产用时：2.383333
- 是否回滚、紧急热修复或重复发布：否
- 若发生失败，发现时间、恢复时间和逃逸门禁：不适用
<!-- kpanel-release-metrics:end -->

<!-- kpanel-release-process-metrics:start -->
- 已记录发布流程异常或无效证据拦截次数：21
- 其中生产写操作开始后异常次数：1
<!-- kpanel-release-process-metrics:end -->

### 流程异常明细

<!-- kpanel-release-process-incidents:start -->
[
  {
    "fingerprint": "preflight/verify-change/windows-linux-toolchain",
    "position": "before-production-write",
    "count": 1,
    "impact": "Windows完整验证缺Go/gofmt，未形成有效L2/L3。",
    "recoveryEvidence": "原Windows失败；固定Linux完整L3 passed；25项L3外层回归passed。",
    "permanentAction": "隔离夹具唯一preflight 464192574eb16c22deada3de695b58c9ee3903447b48f2258deea778a2384038 在生产前加入并由公开node验收必调；9项正负回归及实际Linux/82个LF源/自有可写offline缓存解析通过；不是另造L3/生产wrapper，不宣称仓库治理永久采纳。",
    "historicalReleases": [
      "v1.4.1"
    ]
  },
  {
    "fingerprint": "preflight/certificate-fixture/missing-python",
    "position": "before-production-write",
    "count": 1,
    "impact": "旧测试镜像缺Python，证书夹具r1不能执行。",
    "recoveryEvidence": "v150-sh-smoke-r3及真实Agent r7通过。",
    "permanentAction": "证书唯一夹具固定已有Ubuntu systemd image，启动前检查Python/OpenSSL/Nginx，不安装宿主工具。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/run-repo-bash/checkout-crlf",
    "position": "before-production-write",
    "count": 1,
    "impact": "Git默认archive带CRLF，证书r2拒绝。",
    "recoveryEvidence": "git -c core.autocrlf=false归档，LF smoke r3和最终L3通过。",
    "permanentAction": "隔离夹具唯一preflight 464192574eb16c22deada3de695b58c9ee3903447b48f2258deea778a2384038 在生产前加入并由公开node验收必调；9项正负回归及实际Linux/82个LF源/自有可写offline缓存解析通过；不是另造L3/生产wrapper，不宣称仓库治理永久采纳。",
    "historicalReleases": [
      "v1.4.0"
    ]
  },
  {
    "fingerprint": "preflight/light-node-tests/module-cache-selection",
    "position": "before-production-write",
    "count": 1,
    "impact": "借用只读模块缓存缺依赖，原生build r1失败。",
    "recoveryEvidence": "v150-build-7ba-r1原日志保留；自有可写cache后原生构建通过，最终L3/公开node通过。",
    "permanentAction": "隔离夹具唯一preflight 464192574eb16c22deada3de695b58c9ee3903447b48f2258deea778a2384038 在生产前加入并由公开node验收必调；9项正负回归及实际Linux/82个LF源/自有可写offline缓存解析通过；不是另造L3/生产wrapper，不宣称仓库治理永久采纳。",
    "historicalReleases": [
      "v1.4.1"
    ]
  },
  {
    "fingerprint": "testing/agent-fixture/missing-token",
    "position": "before-production-write",
    "count": 2,
    "impact": "Agent r1/r2缺符合实际接口的认证token。",
    "recoveryEvidence": "原始r1/r2保留；r7真实Agent/PTy/TLS通过。",
    "permanentAction": "唯一guest检查器在安装unit前准备固定合成token，并核对实际认证路径，不放宽Agent认证。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/agent-fixture/failure-log-copy",
    "position": "before-production-write",
    "count": 1,
    "impact": "r1业务前置失败后日志复制表达式TypeError，和缺token为同次尝试两个原因。",
    "recoveryEvidence": "r1原输出保留；r7完整guest命令/结果/journal已取回。",
    "permanentAction": "修正唯一fixture日志复制表达式；finally独立清理/取证，不吞失败。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/agent-fixture/token-permissions",
    "position": "before-production-write",
    "count": 1,
    "impact": "r3 token组/模式不符合正式Agent服务读取规则。",
    "recoveryEvidence": "r7正式unit未改权限，认证请求通过。",
    "permanentAction": "guest准备root/Agent组和最小读权限，在服务启动前检查。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/nginx-fixture/missing-null-device",
    "position": "before-production-write",
    "count": 1,
    "impact": "r4隔离Nginx rootfs缺/dev/null。",
    "recoveryEvidence": "r7实际Nginx启动和TLS指纹/失败回滚通过。",
    "permanentAction": "只在owned guest rootfs补全设备，不修改宿主设备或生产权限。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/agent-fixture/installer-substitutions",
    "position": "before-production-write",
    "count": 1,
    "impact": "r5未模拟标准managed-script安装时的许可/统计替换，调用交互未完成。",
    "recoveryEvidence": "r7与真实安装契约一致后通过。",
    "permanentAction": "唯一guest安装准备按标准permission_granted/ENABLE_STATS替换；固定原始script SHA另核对，不把处理后bytes伪称raw。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/agent-fixture/restart-socket-readiness",
    "position": "before-production-write",
    "count": 1,
    "impact": "r6重启等待未容忍socket瞬时不存在。",
    "recoveryEvidence": "r7重启后实际认证API恢复并发现证书。",
    "permanentAction": "同一等待predicate仅容忍明确过渡态，最终要求健康API业务结果而非sleep成功。",
    "historicalReleases": []
  },
  {
    "fingerprint": "evidence/node-health/completed-invocation-journal",
    "position": "before-production-write",
    "count": 1,
    "impact": "node r1已完成oneshot InvocationID为空，journal取证无效。",
    "recoveryEvidence": "health-r2和新公开public-r1各5份真实health JSON。",
    "permanentAction": "唯一health fixture使用systemd StandardOutput append；保留原unit sandbox，严格JSON/runtime/state断言。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/browser-fixture/missing-catalog-assets",
    "position": "before-production-write",
    "count": 2,
    "impact": "browser r1缺catalog相对路径，r2缺legacy资产，页面准备不完整。",
    "recoveryEvidence": "r7完整应用资产、33+25+22断言和cleanup通过。",
    "permanentAction": "唯一guest保留scripts相对树与legacy文件，启动前核对完整路径，不减少用例。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/browser-fixture/locale-mismatch",
    "position": "before-production-write",
    "count": 1,
    "impact": "browser r3默认英文导致中文精确断言失配。",
    "recoveryEvidence": "r7中英文矩阵通过。",
    "permanentAction": "fixture显式locale/主题，不用宽松包含匹配掩盖翻译缺失。",
    "historicalReleases": []
  },
  {
    "fingerprint": "testing/browser-fixture/nested-route-predicate",
    "position": "before-production-write",
    "count": 2,
    "impact": "browser r4/r5 glob未拦截nested terminal URL；r5实际closeCalls0，UI安全保留会话。",
    "recoveryEvidence": "r6 actions通过；r7完整aggregate通过，原r4/r5仍failed。",
    "permanentAction": "唯一测试URL谓词按实际路径匹配并计数；failure/retry/HostTerminal全链不断言Toast代替服务端结果。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/browser-fixture/execution-policy",
    "position": "before-production-write",
    "count": 1,
    "impact": "aggregate r7首次上传/启动整条命令在执行前被策略拒绝。",
    "recoveryEvidence": "用户再次明确继续后原任务执行被接受，r7passed。",
    "permanentAction": "不绕过策略；保留原拒绝。发布负责人2026-09-07复核执行环境；退出条件为授权范围内原命令获执行与终态证据，已满足。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/apps-contract/wrong-source-path",
    "position": "before-production-write",
    "count": 1,
    "impact": "公开apps比较初读错误packaging/apps路径；PowerShell非终止异常后输出pass无效，未采信。",
    "recoveryEvidence": "实际packaging/kejilion-app/kpanel.conf经ErrorActionPreference=Stop重新比较，raw/本地blob一致。",
    "permanentAction": "同一检查先rg定位真实路径，Stop模式阻断空值/读取失败，禁止仅凭最后一行pass。",
    "historicalReleases": []
  },
  {
    "fingerprint": "preflight/source-check/ssh-identity-selection",
    "position": "before-production-write",
    "count": 1,
    "impact": "sh主线回读初次未选已登记SSH identity，publickey拒绝。",
    "recoveryEvidence": "用既有core.sshCommand显式identity，远端main298及公开raw摘要一致。",
    "permanentAction": "不改凭据或远端；沿已登记SSH参数，逐次检查退出码；未把失败的第一次查询计作main确认。",
    "historicalReleases": []
  },
  {
    "fingerprint": "cleanup/local-artifacts/execution-policy",
    "position": "after-production-write",
    "count": 1,
    "impact": "生产验收通过后，删除本任务web/node_modules与web/dist命令在执行前被策略拒绝；没有删除或释放空间。",
    "recoveryEvidence": "原始拒绝保留；继续只做独立文档收尾，未换shell/方式绕过删除限制，线上不受影响。",
    "permanentAction": "发布负责人2026-09-07复核平台限制；退出条件为平台明确允许同一已核对范围清理。当前保留可再生目录/证据，净释放0；不是生产入口错误或服务失败。",
    "historicalReleases": []
  }
]
<!-- kpanel-release-process-incidents:end -->

- 18类21次流程异常，不把六次Agent尝试中的日志取证第二原因漏记；初始缺工具、CRLF、缓存和浏览器失败均不冒称首轮通过。普通只读查找笔误未让必要步骤产生虚假有效证据者不计；apps非终止错误后的假pass明确作废并计入。
- 已比较v1.4.1、v1.4.0、v1.3.1、v1.3.0、v1.2.0原始指纹。Windows工具链/借用缓存与v1.4.1复发，CRLF与v1.4.0复发；生产前实际修正隔离夹具唯一preflight并补9项回归，公开node运行强制调用。固定L3入口额外25回归、生产入口49回归通过；不以一句“换环境”冒称仓库永久治理采纳。
- 单个产品tag/单次正式生产发布，无产品事故、生产回滚或紧急热修复。备份恢复中的短暂连接重置、标准安装TERM提示/daemon-reload过渡提示均在原入口正常收敛，postdeploy NeedDaemonReload=no，不算额外重试。
- 唯一生产写后异常是本地清理在执行前被平台拦截，未影响生产。没有删除失败版本或重写任何旧tag/验收。

## 遗留风险与后续准入

- 普通配对身份不变；旧节点没有health可选字段显示未知，不能仅凭遥测在线判断文件/终端代理可用。节点需取得1.5.0公开二进制及generation3更新器后出现新观测。
- 本地源测试不是全用户真实配对证明；合成HTTPS peer、离线资产transport、Docker名称适配器、Mock浏览器、加速timer等边界均保留，不扩大已验证范围。
- 本地已核对仅本任务release树web/node_modules（13743文件）及web/dist（1059文件），根均无reparse point；清理命令被执行策略在启动前拒绝，未绕过，未删除，净释放0。目录可由npm ci/build重建但本次保留；原始失败/成功证据、公开资产、bundle、备份和全部原作者工作树均保留，隔离测试容器已经各自正常清理。
