# 构建、发布与部署

## 部署边界

KPanel 使用两个独立进程：

- `paneld` 以非 root Docker 容器运行，入口使用专用 `internal` 网络；联邦监控与 AI Provider
  复用独立受控出站网络，默认不发布宿主端口。
- `kejilion-agent` 以受限 systemd 服务运行，只接受本机 Unix Socket 上的类型化请求。

宿主机必须使用 systemd。当前发行版代码路径覆盖 Debian/Ubuntu、
RHEL/Fedora、Arch/Manjaro 和 openSUSE/SLES；具体实机验收层级见
[宿主机系统兼容矩阵](platform-support.md)。Alpine/OpenRC 尚不属于正式部署目标。

安装器只管理 `/etc/kejilion-panel`、`/opt/kejilion-panel`、
`/var/lib/kejilion-panel`、`/run/kejilion-panel`、
`/usr/local/libexec/kejilion-agent`、对应 systemd unit、专用
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

仓库的 `Release` 工作流仅接受精确的 `v<semver>` 标签。默认发布到
`docker.io/kjlion/kejilion-panel`，如需改用其他 Docker Hub 仓库，可覆盖：

- Repository variable `DOCKERHUB_IMAGE`：`owner/repository`；
- Repository variable `DOCKERHUB_USERNAME`：Docker Hub 用户名。

发布前必须配置 Repository secret `DOCKERHUB_TOKEN`，且令牌应仅具备目标仓库写权限。

工作流会先执行前后端验证，再构建双架构 Agent、带 SBOM/Provenance 的双架构
镜像，并把固定镜像 digest 写入 GitHub Release。生产部署使用 Release 中的
digest 与校验和，不直接使用 `latest`。

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

推送 Docker Hub：

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

### 面板访问域名白名单

`publicUrl` 只能写一个来源，但面板常常同时通过多个名字访问（反代域名、CDN 域名、内网地址、
隧道主机名）。设置页的「面板访问域名」（`GET`/`PUT /api/v1/settings/allowed-hosts`）用于登记
额外允许的 Host，持久化在面板存储里，改动会写审计。

该白名单**同时作用于 Host 校验和 Origin 校验**。只放开 Host 会得到一个半残状态：页面能打开，
但所有写操作都会因为 Origin 不匹配而 403——因为 `expected` 只能是 `publicUrl` 那一个来源。

两条刻意的设计约束：

- **只做精确匹配**，不支持 `*.example.com` 这类通配或后缀写法。Host/Origin 校验正是阻挡 DNS
  重绑定与 Host 头伪造的防线，通配等于把它拆掉。
- **Origin 匹配忽略协议（http/https）**，只比对主机部分。在 CDN/反代后面，除非代理位于
  `trustedProxyCidrs`，服务端无法可信地得知浏览器侧的协议（来自不可信来源的
  `X-Forwarded-Proto` 会被正确忽略），强制要求 https 会让白名单在它最该生效的场景里失效。
  这是可接受的取舍：Origin 校验的职责是拒绝**其他站点**，而能以 `http://<你自己的域名>` 提供
  服务的攻击者已经控制了该域名的 DNS，那时失守的并不是 Origin 校验这一环。

### 桌面浏览器功能与 HTTPS

桌面"浏览器"应用（`/api/v1/browse/*`）的页面改写完全依赖 Service Worker，而浏览器只在
secure context（HTTPS，或 `localhost` 等 loopback 来源）下允许注册 Service Worker。因此该功能
在非 secure context 下物理上无法工作。

**这条限制由浏览器执行，服务端不做对应校验。** 服务端看不到可靠的浏览器侧协议——放在 CDN 或
反向代理后面时它收到的是明文 HTTP，而来自不可信对端的 `X-Forwarded-Proto` 会被正确忽略——
任何服务端猜测都会误判，把本来能用的部署挡在门外。前端用 `window.isSecureContext`
（唯一权威判据）自检，并区分"浏览器不支持 Service Worker"与"当前来源不是 secure context"
两种情况给出准确提示，见 `web/public/browser-app/app.js`。

放在 Cloudflare 等 CDN 后面时，浏览器地址栏是 `https://`，浏览器自身的判定就会通过，功能可用；
面板不需要为此做任何配置。

#### 何时仍应把 CDN 加入 `trustedProxyCidrs`

与浏览器功能无关，但影响两件正确性问题：CDN 回源通常是明文 HTTP，若 CDN 段不在
`trustedProxyCidrs` 里，Cookie 不会带 `Secure`，且登录限速与审计记录的是 **CDN 边缘 IP** 而不是
真实客户端 IP——不同攻击者互相触发对方的锁定，同一攻击者换边缘节点又能重置计数。

诊断（在源站执行，确认 CDN 实际以什么地址、什么端口回源）：

```sh
ss -tn state established "( sport = :8080 )"
```

修复需要**同时**做两件事，缺一不可：

1. 把 CDN 的官方回源段写入 `trustedProxyCidrs`（Cloudflare：`https://www.cloudflare.com/ips-v4`
   与 `ips-v6`，共 22 段；保留原有的 `127.0.0.0/8`、`::1/128`）。可信对端的客户端 IP 依次取
   `CF-Connecting-IP`、`X-Real-IP`、`X-Forwarded-For` 中最右侧的非可信地址。
2. 把源站端口的入站限制为只接受这些 CDN 段。

第 2 步不是可选的加固。信任一个共享 CDN 的整个地址段，等于信任**任何人**经该 CDN 转发到你源站的
请求——攻击者可以把自己的 CDN 域名回源指向你的源站 IP，其请求就从一个"可信代理"地址到达。届时
`CF-Connecting-IP` / `X-Forwarded-For` 可被任意伪造，登录失败锁定和审计里的客户端 IP 全部失效
（开启 `allowIpHosts` 时 Host 校验也拦不住，因为字面 IP 本身就是允许的 Host）。源站端口对公网
开放的前提下，第 1 步是净损失。

同理，**"Host 命中白名单"不能替代对端 CIDR 判断**：Host 头由客户端提供，域名也不是秘密（知道
域名的人可以直接连源站 IP 并带上该 Host），因此它不能证明请求经由 CDN 到达。可信代理的判定只
能基于对端地址——那是请求里唯一无法伪造的部分。

Cloudflare 回源默认走 80/443；如果源站监听在 8080 之类的端口，那是 CDN 侧的 Origin Rules 或
DNS 记录端口决定的，以 `ss` 实测为准，不要照抄本文档的端口号。

若只是在没有证书的机器上做验证，可用 SSH 端口转发把
面板映射到本地 loopback，从而获得真正的 secure context：

```sh
ssh -N -L 8080:127.0.0.1:8080 root@<host>
# 然后浏览器访问 http://localhost:8080
```

此时 `publicUrl` 需要设为 `http://localhost:8080`；如仍需保留直接用 IP 访问面板其余功能的
能力，同时打开 `allowIpHosts`（浏览器功能在 IP 来源下会被浏览器自己拒绝）。
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

## 验收

```sh
systemctl is-active kejilion-agent
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
标签同时匹配时停止 Panel 容器；随后复核容器运行态、Agent `ActiveState` 和
`UnitFileState`。无法确认时会输出 `CRITICAL`，此时不得重试或启动相关服务。
安装器不会自动删除数据、日志或镜像。先保留现场并检查：

```sh
journalctl -u kejilion-agent --no-pager
docker --host unix:///var/run/docker.sock logs kejilion-panel
systemctl show kejilion-agent.service -p LoadState -p FragmentPath -p DropInPaths
docker --host unix:///var/run/docker.sock inspect kejilion-panel \
  --format '{{json .Config.Labels}}'
```

只有确认 unit 的 `FragmentPath` 是
`/etc/systemd/system/kejilion-agent.service`，且容器标签同时包含
`com.docker.compose.project=kejilion-panel` 与
`com.docker.compose.service=panel` 后，才可执行以下全新安装恢复步骤：

```sh
docker --host unix:///var/run/docker.sock compose \
  --project-name kejilion-panel \
  --env-file /opt/kejilion-panel/.env \
  -f /opt/kejilion-panel/compose.yml down
systemctl disable --now kejilion-agent.service
rm -f -- /etc/systemd/system/kejilion-agent.service \
  /usr/local/libexec/kejilion-agent
rm -rf -- /etc/kejilion-panel /opt/kejilion-panel /var/lib/kejilion-panel
systemctl daemon-reload
```

`kejilion-panel` 组默认保留；只有能证明它由本次失败安装创建、没有显式成员且
没有用户以其为主 GID 时，才可单独删除。恢复完成后重新执行只读 preflight，
不得绕过 fresh-install 检查直接重试。

任何回滚都不得删除、恢复或覆盖 `/home/web`、`kejilion.sh`、站点、数据库、
证书和其他容器。生产部署前应记录 `kejilion.sh` 相关文件哈希和现有 Docker
资源清单，回滚后再次比对，证明现有业务未变化。
