# KPanel Browser 镜像

可选独立组件：Alpine 3.24、官方仓库 Chromium、TigerVNC、Openbox、软件渲染组件、中文字体和 `kpanel-browser`。
不包含桌面环境，不挂载 Docker Socket，不发布宿主机端口，不改变 KPanel 主镜像。
`amd64` / `arm64` 分别原生构建、运行和扫描后，才允许发布对应版本。

此前公开版本为 `docker.io/kjlion/kpanel-browser:0.1.0-rc.2`，源码提交
`94d88f035ee5eb899896a29d25b2d2d1c71991c1`。两架构检查、完整 L3 及公开镜像拉取复测
见[发布记录](https://github.com/kejilion/KPanel/actions/runs/40382052237)。接入时使用
`.env.example` 中固定的镜像摘要对应旧的截图协议，与此分支的 VNC 协议不兼容。
本分支发布前必须先按本文件完成新组件两架构验收及发布，再将托管安装器和示例中
所有固定摘要更新为新组件的已验证摘要；Panel 会拒绝旧组件，避免错误连接。
`0.1.0-rc.1` 的公开复测发现静态标签恢复缺帧问题；`rc.2` 修复了旧协议中的该问题，两者均保留作历史记录。

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
