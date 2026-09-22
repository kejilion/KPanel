# 构建、发布与部署

## 部署边界

KPanel 使用两个独立进程：

- `paneld` 以非 root Docker 容器运行，入口使用专用 `internal` 网络；联邦监控与 AI Provider
  复用独立受控出站网络，默认不发布宿主端口。
- `kejilion-agent` 以受限 systemd 或 OpenRC 服务运行，只接受本机 Unix Socket 上的类型化请求。

宿主机必须运行 systemd 或 OpenRC。当前发行版代码路径覆盖 Debian/Ubuntu、
RHEL/Fedora、Arch/Manjaro、openSUSE/SLES，以及 Alpine Linux 3.24 `sys` mode；
具体实机验收层级见[宿主机系统兼容矩阵](platform-support.md)。Alpine/OpenRC
已有自动化契约证据，但尚未完成真实 PID 1 VPS/VM 的 L3 准入，不能写成已实机验证。

安装器只管理 `/etc/kejilion-panel`、`/opt/kejilion-panel`、
`/var/lib/kejilion-panel`、`/run/kejilion-panel`、
`/usr/local/libexec/kejilion-agent`、对应 systemd unit 或
`/etc/init.d` 与 `/etc/conf.d` OpenRC 定义、专用
`kejilion-panel` 容器，以及 `kejilion-panel-internal`、`kejilion-panel-egress`
两张网络。安装器还会把固定
digest 的 Panel 镜像拉入本机缓存。安装器不会执行或修改 `kejilion.sh`，也不会
改动 `/home/web`、现有 Nginx 配置、防火墙和站点。

v0.1 安装器只支持全新安装。发现任何既有 Panel 文件、同名容器或任一同名网络时
会拒绝继续，不会把未知资源当作可升级对象。后续版本必须在具备事务化升级和自动
回滚后再开放原地升级。

## 发布产物

版本发布应包含：

- `docker.io/<owner>/kejilion-panel:<version>` 的 `linux/amd64`、`linux/arm64`
  多架构镜像；
- `kejilion-agent-linux-amd64`；
- `kejilion-agent-linux-arm64`；
- `kejilion-node-linux-amd64`；
- `kejilion-node-linux-arm64`；
- `kejilion-panel-deploy-<version>.tar.gz`；
- 上述文件的 `SHA256SUMS`；
- 镜像 manifest digest。生产部署只使用
  `docker.io/<owner>/kejilion-panel@sha256:<digest>`，不使用可漂移标签。

仓库的 `Release` 工作流只接受精确的稳定标签 `vX.Y.Z` 或预览标签 `vX.Y.Z-rc.N`。默认发布到
`docker.io/kjlion/kejilion-panel`，如需改用其他 Docker Hub 仓库，可覆盖：

- Repository variable `DOCKERHUB_IMAGE`：`owner/repository`；
- Repository variable `DOCKERHUB_USERNAME`：Docker Hub 用户名。

发布前必须配置 Repository secret `DOCKERHUB_TOKEN`，且令牌应仅具备目标仓库写权限。

工作流会先执行前后端验证，再构建双架构 Agent、带 SBOM/Provenance 的双架构
镜像，并把固定镜像 digest 写入 GitHub Release。生产部署使用 Release 中的
digest 与校验和，不直接使用 `latest`。

稳定版会成为 GitHub Latest 并提升 Docker `latest`；预览版会标记为 GitHub prerelease，只提升
Docker `preview`，不得改变公共稳定入口或进入正式生产部署。完整版本、候选分支和验收契约见
[稳定版与预览版规范](release-channels.md)。

每次发布必须先在 `CHANGELOG.md` 增加与 `VERSION` 完全一致的版本章节。Release
工作流会从该章节生成 GitHub Release 的“版本更新内容”，并补充升级方式、兼容性与
迁移提示、产物校验、测试结论和回滚说明；缺少版本章节或明确更新条目时，流水线会在
镜像构建前失败，禁止发布只有镜像摘要而没有更新内容的版本。

本地验证和交叉编译：

```sh
make test
make build-linux
sha256sum dist/linux-amd64/kejilion-agent dist/linux-arm64/kejilion-agent
```

手工构建示例仅展示稳定版标签；正式发布仍必须使用仓库 `Release` 工作流。预览版不得把同类命令中的
`latest` 作为目标，应由工作流提升 `preview`：

```sh
VERSION=0.16.0
IMAGE=docker.io/<owner>/kejilion-panel

docker login
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg "VERSION=$VERSION" \
  --provenance=mode=max \
  --sbom=true \
  --tag "$IMAGE:$VERSION" \
  --tag "$IMAGE:latest" \
  --push .

docker buildx imagetools inspect "$IMAGE:$VERSION"
```

发布前应把输出的 manifest digest 记录到发布说明；部署时必须使用该 digest。

## 更新通道

设置页的“加入预览版计划”只切换更新来源并立即检查，不会自动安装。用户还可以独立选择是否开启
“自动安装更新”；默认关闭。检查到更高版本后，“立即安装”只排队本次已展示的精确版本和镜像 digest，
不会开启以后自动安装。

退出预览版计划只恢复稳定来源，不会自动降级当前 RC。稳定版尚未追上时保持现状；出现同版本正式版
或更高稳定版后才向前升级。完整 KPanel 自动更新当前仅支持 systemd 宿主机，OpenRC 会明确显示不可用。
轻量 Node 的无人值守更新仍只跟踪稳定版。

## 宿主机预检

先在目标机执行只读预检。该命令不会连接 Docker Socket，避免意外启动已停止
的 Docker：

```sh
./deploy/preflight.sh \
  --public-url https://panel.example.com \
  --network-subnet 172.29.255.240/28
```

预检不会调用 `docker info`、不会连接 Docker Socket，也不会启动 Docker。
Docker 服务必须由运维人员在评估现有容器后提前启动。预检会拒绝重叠路由、
非专用 Agent 组和既有 Panel 资源。`/home/web` 不是安装前置条件：目标机尚未
初始化 Kejilion 网站环境时，预检只给出警告，安全登录、主机监控、Docker 和应用
市场仍可正常工作；网站列表返回空列表，可信 `kejilion.sh` 提供的 WordPress、
反向代理和一键建站入口仍可初始化 LDNMP 并创建站点。只有部分网站受管目录缺失时
仍按环境异常处理，避免把已有站点损坏误报为空列表。其他失败项必须处理后再部署。

Alpine 3.24 必须是持久化 `sys` mode，并启用 `community` 仓库。当前候选的基础准备为：

```sh
apk add bash curl coreutils e2fsprogs-extra iproute2 musl-utils openssl util-linux tzdata \
  docker docker-cli-compose
rc-update add docker default
rc-service docker start
```

只支持本机 rootful Docker Socket；rootless Docker、远程 Docker Context、Alpine
`diskless`/`data` mode 均不在当前部署契约内。账户管理、SSH 防御等可选功能还需按
[兼容矩阵](platform-support.md)安装 `shadow`、`sudo`、`openssh`、`fail2ban`、`python3` 等工具。

## 安装

根据目标机架构选择 Agent，并先在目标机核对摘要：

```sh
sha256sum kejilion-agent

sudo ./deploy/install.sh \
  --agent-binary ./kejilion-agent \
  --agent-sha256 <agent-sha256> \
  --image docker.io/<owner>/kejilion-panel@sha256:<manifest-digest> \
  --public-url https://panel.example.com \
  --network-subnet 172.29.255.240/28 \
  --dry-run
```

`--network-subnet` 只接受对齐的 RFC1918 IPv4 `/28`。安装器会用同一 CIDR
自动生成网关（网段基址加 1）、Panel 私网地址（网段基址加 2）和可信代理范围，
不提供可独立放宽的参数。`--dry-run` 会在连接 Docker Socket 前返回；确认后
去掉 `--dry-run`。正式安装只连接
`unix:///var/run/docker.sock`，并拒绝继承 `DOCKER_HOST` 或
`DOCKER_CONTEXT`。

安装成功后，一次性初始化 Token 仅保存在：

```text
/var/lib/kejilion-panel/panel/bootstrap.token
```

Token 不会写入日志或安装器输出。首次初始化成功后文件会被删除。

## HTTPS 入口

Panel 容器不发布宿主端口。需要在宿主机或 host-network Nginx 中把 Panel
域名反向代理到固定私网地址，例如默认网段对应
`http://172.29.255.242:8080`，并设置 `Host`、`X-Real-IP` 和
`X-Forwarded-Proto`。反向代理必须覆盖设置 `X-Real-IP`，不能沿用客户端提交
的同名请求头。反向代理来源必须显式加入 Panel 的可信代理 CIDR；不要信任整个
公网或所有私网。

默认 Compose 同时使用两张职责分离的网络：

- `kejilion-panel-internal`：固定 `/28` 内部网段，只承载宿主机反向代理到 Panel
  的入口。
- `kejilion-panel-egress`：普通 bridge，只供 Panel 主动访问经过校验的 HTTPS 或 Noise
  加密 `IP + 端口` 集群节点及固定外部服务。

Panel 仍只信任 loopback 与内部 `/28` 的代理头，egress 网络不加入可信代理范围。
如果预检发现冲突，通过 `--network-subnet` 选择另一个对齐的私网 `/28`；安装器会
同时写入内部网络、网关、Panel 私网地址和可信代理 CIDR，不能在安装后手工改其中一项。
Nginx 必须与 Panel 同处一台宿主机或能安全路由到该内部网段；不要把 Panel 私网地址
暴露到公网路由。

集群主机接受公网 HTTPS 根地址；无域名时，v2 也接受 `http://字面量IP:非80端口`，联邦
正文由 Noise 端到端加密。该能力不加密浏览器打开的管理页面，公网日常管理仍应配置 HTTPS。
确需访问私有管理网时，在
`/opt/kejilion-panel/.env` 的 `KEJILION_PANEL_CLUSTER_PRIVATE_CIDRS` 中填写精确
CIDR（逗号分隔）后重建 Panel 容器。loopback、link-local、组播和云元数据地址始终
拒绝；不要填写覆盖范围过大的网段。

反向代理配置属于目标机业务配置，安装器不会自动写入。上线时应单独备份、新增
独立域名配置、执行 `nginx -t`，成功后才 reload；验证失败时不得 reload。

## 直接 IP + 端口

测试主机可以不依赖 Nginx，使用附加 Compose 文件直接发布端口：

```sh
KEJILION_PANEL_PUBLIC_URL=http://154.36.153.9:8080
KEJILION_PANEL_SECURE_COOKIE=false
KEJILION_PANEL_BIND_ADDRESS=0.0.0.0
KEJILION_PANEL_PORT=8080

docker --host unix:///var/run/docker.sock compose \
  --project-name kejilion-panel \
  --env-file /opt/kejilion-panel/.env \
  -f /opt/kejilion-panel/compose.yml \
  -f /opt/kejilion-panel/direct-port.yml up -d
```

默认情况下，`KEJILION_PANEL_PUBLIC_URL` 必须与浏览器访问的来源完全一致。应用市场的
直连端口安装会设置 `KEJILION_PANEL_ALLOW_IP_HOSTS=true`，允许浏览器通过任意合法的
IPv4/IPv6 字面地址访问，以兼容内网、NAT 和端口映射；写请求的 `Origin` 仍必须与当前
IP Host 完全同源，普通域名也仍受 `KEJILION_PANEL_PUBLIC_URL` 限制。直接 HTTP 会禁用
Secure Cookie，仅建议用于受控环境。覆盖文件会把入口网络改为可发布端口的普通
bridge；独立 egress 网络仍只承担受限出站访问。正式公网环境仍建议使用 HTTPS。

直接端口部署完成后，可以使用 `kejilion.sh` 的标准反代入口：

```sh
k fd panel.example.com 127.0.0.1 8080
```

来自显式可信代理 CIDR 的请求可以使用代理传递的 HTTPS `Host` 和
`X-Forwarded-Proto`，无需把 `KEJILION_PANEL_PUBLIC_URL` 从直连地址改成域名。
应用市场安装和更新会自动信任 KPanel 内部网络以及出口网络的宿主机网关单地址
（IPv4 `/32`、IPv6 `/128`），从而支持宿主机 Nginx 转发，但不会信任整个出口网段。
该路径会自动启用 Secure Cookie，并从 `X-Real-IP` 或安全解析后的
`X-Forwarded-For` 恢复客户端地址；非可信来源不能利用这些请求头绕过 Host/Origin 校验。
Passkey 设置页会把当前可信 HTTPS 域名作为候选入口显示；管理员确认密码后即可保存固定的
`passkeyOrigin`，无需为直连 IP 安装额外编辑配置文件。若入口不是有效 HTTPS 域名，Passkey 仍保持禁用。

## 验收

```sh
# systemd 主机
systemctl is-active kejilion-agent

# OpenRC 主机改用
rc-service kejilion-agent status

sudo /usr/local/libexec/kejilion-agent healthcheck
docker --host unix:///var/run/docker.sock compose \
  --project-name kejilion-panel \
  --env-file /opt/kejilion-panel/.env \
  -f /opt/kejilion-panel/compose.yml ps
docker --host unix:///var/run/docker.sock compose \
  --project-name kejilion-panel \
  --env-file /opt/kejilion-panel/.env \
  -f /opt/kejilion-panel/compose.yml \
  exec -T panel /paneld agent-healthcheck
curl --noproxy '*' --fail --silent http://172.29.255.242:8080/api/v1/health
curl --fail --silent --show-error https://panel.example.com/api/v1/health
```

还需人工确认：

- 登录、注销、失效 Session 和 CSRF 拒绝；
- `kejilion.sh` 已有站点与容器可发现并能执行对应管理动作；
- Web 创建的测试站点产物可被脚本侧列表识别；
- 脚本侧新增测试站点后，刷新 Web 能显示实际配置；
- 无 KPanel label、带特权参数或人工创建的容器仍可按实时状态管理；
- Agent 离线时 Web 降级且所有宿主机写操作禁用。

## 回滚

v0.1 不执行原地升级，因此没有“恢复旧版 Panel”的路径。全新安装在启动阶段
失败时，安装器会尝试停止并禁用本次 Agent，并只在 Compose project/service
标签同时匹配时停止 Panel 容器；随后按实际 init backend 复核容器运行态、Agent
运行状态和开机启动状态。无法确认时会输出 `CRITICAL`，此时不得重试或启动相关服务。
安装器不会自动删除数据、日志或镜像。先保留现场并检查：

```sh
# systemd
journalctl -u kejilion-agent --no-pager
systemctl show kejilion-agent.service -p LoadState -p FragmentPath -p DropInPaths

# OpenRC
rc-service kejilion-agent status
grep -F 'kejilion-agent' /var/log/messages | tail -n 200
readlink -f /etc/runlevels/default/kejilion-agent

docker --host unix:///var/run/docker.sock logs kejilion-panel
docker --host unix:///var/run/docker.sock inspect kejilion-panel \
  --format '{{json .Config.Labels}}'
```

只有确认 systemd unit 的 `FragmentPath` 是
`/etc/systemd/system/kejilion-agent.service`，或确认 OpenRC service 是本次安装的
`/etc/init.d/kejilion-agent` 且 default runlevel 链接指向它，并且容器标签同时包含
`com.docker.compose.project=kejilion-panel` 与 `com.docker.compose.service=panel` 后，
才可执行以下全新安装恢复步骤。先执行共用的 Compose 清理：

```sh
docker --host unix:///var/run/docker.sock compose \
  --project-name kejilion-panel \
  --env-file /opt/kejilion-panel/.env \
  -f /opt/kejilion-panel/compose.yml down
```

systemd 主机：

```sh
systemctl disable --now kejilion-agent.service
rm -f -- /etc/systemd/system/kejilion-agent.service \
  /usr/local/libexec/kejilion-agent
rm -rf -- /etc/kejilion-panel /opt/kejilion-panel /var/lib/kejilion-panel
systemctl daemon-reload
```

OpenRC 主机：

```sh
rc-service kejilion-agent stop
rc-update del kejilion-agent default
rm -f -- /etc/init.d/kejilion-agent /etc/conf.d/kejilion-agent \
  /usr/local/libexec/kejilion-agent
rm -rf -- /etc/kejilion-panel /opt/kejilion-panel /var/lib/kejilion-panel
```

`kejilion-panel` 组默认保留；只有能证明它由本次失败安装创建、没有显式成员且
没有用户以其为主 GID 时，才可单独删除。恢复完成后重新执行只读 preflight，
不得绕过 fresh-install 检查直接重试。

任何回滚都不得删除、恢复或覆盖 `/home/web`、`kejilion.sh`、站点、数据库、
证书和其他容器。生产部署前应记录 `kejilion.sh` 相关文件哈希和现有 Docker
资源清单，回滚后再次比对，证明现有业务未变化。
