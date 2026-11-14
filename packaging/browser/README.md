# KPanel Browser 镜像

## Tor Browser 可选组件（本地候选）

在浏览器设置中选择 Chromium 或 Tor Browser 后安装；已安装时锁定类型，先卸载后才能切换。
Tor 首轮仅支持 Linux amd64，使用完整原生工具栏，不使用 Chromium/CDP，也不继承 Chromium 起始页设置。
Panel 只转发经过管理员会话、Origin 和首帧 CSRF 验证的有界 RFB 二进制流，不在 Panel 同源下托管第三方 HTML/JavaScript。
所有窗口共享一个 Tor 桌面；关闭窗口只断开画面，停止/卸载组件会丢失临时会话。

Agent 从 Tor 官方固定 URL 流式下载 16.0.1，单文件上限 256 MiB，固定 SHA-256
`5b36ad610c20c81faa72caf27d1cd3a11f14a327c320b4cec581400016ba78b0`；构建阶段再次检查摘要，
用随候选保存的官方签名和公钥执行 `gpgv`，限定主密钥指纹
`EF6E286DDA85EA2A4BA7DE684E2C6E8793298290`。2026-11-14 已核验安装包签名。
`tor.Dockerfile` 固定 jlesage Ubuntu 24.04 GUI 基座摘要，运行时安装 Ubuntu 签名仓库的 GTK、字体与图形依赖；
镜像在宿主机构建为 `kpanel-browser-tor:16.0.1-kpanel.1`，不依赖尚未发布的 KPanel Tor 镜像。
Docker 构建上下文只允许配方、启动脚本、安装包、签名、公钥及下述助手锁定包/构建脚本，不包含 Agent 的 token/任务状态。

两个网桥助手由 KPanel 重建，不再是 Tor 官方签名包内的原始助手字节：保留该版本上游使用的
Conjure `14e660f0a64bcbb2b55f01f69c1aafdfd5a6dcfa` 与 Lyrebird
`59ac7e400c384773f5584dbe366516ad04a33d9f` 应用源码，升级六项显式安全依赖及其必要传递依赖。
`tor-helpers/manifest.json` 记录来源、归档摘要、固定 Go 工具链与版本；各自 `go.mod`/`go.sum`
和随安装器嵌入的 `tor-helper-locks.tar` 由自动测试逐字核对。构建校验来源归档与锁文件，
采用 `-mod=readonly`，并用固定版本 govulncheck 的普通文本模式检查实际二进制，检出漏洞即失败。
最终镜像 `/opt/kpanel-tor-helpers` 保留来源说明、模块清单、许可证与二进制摘要。
这增加了 KPanel 对重建助手及其依赖升级的维护责任；网桥由原生 Tor 界面配置，不能将扫描通过解释为匿名性保证。

`tor-compose.yml` 不发布端口、不挂载宿主数据或 Docker Socket；浏览器进程 UID/GID 65532，根文件系统只读。
内存 1536 MiB、CPU 1.5、PID 512，临时配置 512 MiB、临时目录 256 MiB、共享内存 256 MiB，日志 2×5 MiB。
GUI 初始化保留所需的 CHOWN/SETUID/SETGID/FOWNER/DAC_OVERRIDE/KILL/NET_BIND_SERVICE；不使用 privileged、host network 或无沙箱参数。
GUI 基座先以 root 初始化内部用户，再以 UID/GID 65532 启动浏览器；root 初始化/监督进程仍存在，不能将它称为完全 rootless 容器。
2026-11-14 的受限启动实测中，直接强制 `--user 65532:65532` 会在 `10-init-users.sh` 的 `/etc/shadow` 所有权调整处失败；正常方式的 GUI 返回 200，Firefox 有效 capability 为零。
Trivy `AVD-DS-0002` 因此只对两份相同 Tor Dockerfile 有路径限定的临时例外，2026-11-21 到期；其他漏洞、秘密及配置规则继续扫描。
浏览器组件维护者须在到期前复核；经过原生桌面与生命周期验证的 rootless 基座可用，或初始化特权被移除时，删除该例外。
Tor 的连接/网桥设置在原生界面内完成，容器健康和画面可用不等于 Tor 已连接。
发行版兼容、ARM64、公开产物及生产验收仍须单独完成；本地原型证据不能替代发布门禁。

可选独立组件：Alpine 3.24、官方仓库 Chromium、TigerVNC、Openbox、软件渲染组件、中文字体和 `kpanel-browser`。
不包含桌面环境，不挂载 Docker Socket，不发布宿主机端口，不改变 KPanel 主镜像。
`amd64` / `arm64` 分别原生构建、运行和扫描后，才允许发布对应版本。

KPanel v1.30.0-rc.13 配套组件使用 `docker.io/kjlion/kpanel-browser:0.1.0-rc.5`，固定多架构摘要为
`sha256:9a5cd79bef4262cf675331a8ce07e9e5df9121d26fb3721440c6256ad444ea03`。两架构原生构建、运行、扫描、完整 L3 和匿名公开拉取复测均通过，见[发布记录](https://github.com/kejilion/KPanel/actions/runs/43024744979)。`.env.example` 与托管 Compose 使用这个固定摘要；旧 RC2 截图协议与本分支 VNC 协议不兼容，Panel 会拒绝旧组件，避免错误连接。
此前公开版本为 `docker.io/kjlion/kpanel-browser:0.1.0-rc.2`，源码提交
`94d88f035ee5eb899896a29d25b2d2d1c71991c1`，其[发布记录](https://github.com/kejilion/KPanel/actions/runs/40382052237)保留作历史参考。`0.1.0-rc.1` 的公开复测发现静态标签恢复缺帧问题；`rc.2` 修复了旧协议中的该问题。

启动使用 UID/GID `65532:65532`，要求只读根目录、可写临时目录、独立共享内存、
资源上限、`no-new-privileges` 和提供的 seccomp profile。组件启动前实际检查
Chromium 的 PID namespace 与 seccomp 沙箱；不满足时以 `browser_runtime_unsupported`
退出，不自动改用 `--no-sandbox`、特权容器或放宽宿主机全局安全配置。

## 本地验证

在 Linux Docker 环境、仓库根目录执行：

```sh
docker build -f packaging/browser/Dockerfile --target runtime -t kpanel-browser:check .
docker build -f packaging/browser/Dockerfile --target test -t kpanel-browser:test .
sudo bash packaging/browser/verify.sh kpanel-browser:check kpanel-browser:test /absolute/evidence/path
```

本地下载慢时可添加 `--build-arg ALPINE_MIRROR=https://mirrors.ustc.edu.cn/alpine`
或使用清华镜像。仅切换构建容器的软件包下载地址，保留 HTTPS、APK 签名校验和构建缓存。
公开 Actions 构建默认使用 Alpine 官方源，减少镜像站同步延迟对安全更新的影响。
构建代理使用 `--secret id=https_proxy,env=HTTPS_PROXY`；代理需要额外可信证书时，
可通过 `--secret id=proxy_ca,src=<可信 CA bundle>` 提供，仅在构建期间读取，不写入镜像。

测试使用临时凭据和只对本次夹具开放的 `127.0.0.1/32`。覆盖沙箱状态、实际服务启动、
认证、带禁止嵌入响应头的页面、画面、中文输入、回车、滚动、缩放、8 标签上限和进程清理。
成功后保留日志、版本、包清单和容器配置；不保留 token。

## 接入现有 Panel

`compose.yml` 是与 `deploy/compose/compose.yml` 合并的可选覆盖文件。
将本目录 `.env.example` 复制到私有环境文件，保留已验证的镜像摘要，并填写下面两个文件路径；
与原有 Panel 环境文件一起传给 Compose，不覆盖原有 Panel 配置。
先生成不少于 32 字节的独立随机 token，权限设为 `0400`、所有者 `65532:65532`，
供 Panel 与浏览器容器只读挂载。设置：

- `KPANEL_BROWSER_IMAGE=docker.io/kjlion/kpanel-browser@sha256:<已验证摘要>`
- `KPANEL_BROWSER_TOKEN_FILE=<token 的宿主机绝对路径>`
- `KPANEL_BROWSER_SECCOMP_FILE=<browser-seccomp.json 的宿主机绝对路径>`

然后将此覆盖文件追加到现有 Compose 命令中。Panel 版本必须包含 browser service 协议；
当前稳定主镜像不能因浏览器组件已运行就被视为已具备该功能。
默认仍禁止内网与元数据目标；需要访问内网应用时单独配置精确 CIDR。

## 更新与发布

镜像通过固定 Alpine 基础层与签名软件源构建；镜像内的
`/usr/share/kpanel-browser-packages.txt` 记录实际包版本。更新 Chromium 时构建新的组件版本，
重新验收后替换摘要并重建容器；宿主机系统升级不会更新镜像中的 Chromium。
会话使用临时目录，替换容器将结束当前网页会话。

`Publish browser component` 只从 `main` 手动调度，输入已通过候选 CI 的精确源码 SHA
和未使用的组件版本。运行工作流必须与候选内该文件完全相同；完整 L3 与两架构组件验收
全部通过后才使用 Docker Hub 凭据发布，再拉取公开摘要复测。没有自动发布 `latest`，
也不修改 KPanel 的 VERSION、Release 或默认更新通道。当前试验发布入口不恢复部分成功的发布：
任一版本或架构标签已占用便拒绝重新推送；修复后使用新的 RC 版本。
首次稳定组件前需补齐发布附件恢复与新 Dockerfile 的统一依赖检测覆盖，由组件维护者负责。
候选分支的相关 push 只执行双架构构建、真实浏览器测试与扫描，不运行发布 job 或读取 Docker Hub 凭据。

seccomp profile 基于 Microsoft Playwright `v1.64.1` 的
[`utils/docker/seccomp_profile.json`](https://github.com/microsoft/playwright/blob/v1.64.1/utils/docker/seccomp_profile.json)，
上游 SHA-256 `cc3e61cabda6bbc1e53e54d27ba4d55a9d3be829b6dd1a596f4a7b31b1cc7849`，
Apache-2.0；保留上游许可于 `PLAYWRIGHT-LICENSE`。本地增加 `chroot` syscall 规则，
用于外层 `cap_drop: ALL` 时 Chromium 在自身 user namespace 内完成沙箱隔离；外层依然无此能力。
本地 SHA-256 `07ea7eec42cb37c0bfb2b8c5e1e19d65ee15046d415e7c34854849a934e06b51`。
它允许 Chromium 用户命名空间，
仍默认拒绝未允许的系统调用。容器继续共享宿主机内核，不能用容器替代浏览器沙箱。
