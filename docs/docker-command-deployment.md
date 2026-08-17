# Docker 命令优先部署契约

## 目标

KPanel 的“新建容器”使用一个粘贴入口覆盖两类常见部署资料：

- 单条 `docker run` 命令；
- 完整 Docker Compose YAML。

默认流程不要求用户先选择部署类型、容器名、网络或重启策略。只有需要修改解析结果或没有现成部署
内容时，才展开结构化高级设置。

粘贴入口使用浏览器内已有的轻量 YAML 语法树和有位置的 Docker Run 词法分析，不调用远端服务，
也不执行粘贴文本。错误统一返回源码偏移、行、列、错误原因和修复提示；用户点击诊断可聚焦并选中
对应位置。浏览器诊断用于即时反馈，Agent 提交前的结构化校验和 `docker compose config` 仍是最终门禁。

## 业务真源与互通

- 容器、镜像、网络和卷的真实状态仍来自 Docker Engine。
- `docker run` 被解析为现有结构化 Docker Engine API 请求，不执行用户粘贴的 Shell 文本。
- Compose 项目保存为 `/home/docker/<project>/docker-compose.yml`，项目变量保存为同目录权限 `0600`
  的标准 `.env`；再使用宿主机固定的
  `docker compose` 入口启动。该目录和文件名与 `kejilion.sh` 的 Compose 发现、备份和恢复方式兼容。
- Compose 容器由标准 `com.docker.compose.*` 标签识别；Docker Run 容器继续写入
  `io.kejilion.panel.managed=true`。两类资源都可被 Docker CLI、Compose、`kejilion.sh` 和 KPanel
  继续管理。
- 容器页按 `com.docker.compose.project` 就地编组，独立容器保留在同一列表的“独立容器”组；不创建
  Compose 项目数据库或导入状态。
- Compose 管理按需读取标准 `working_dir`、`config_files` 和 `service` labels，再读取 `/home/docker`
  或 `/home/web` 中的实际配置。支持多配置文件切换、项目启动/停止/重启、修改配置并重新部署。

用户粘贴的 Compose 属于用户配置，不是 KPanel 维护的外联模板，因此不在
`docs/external-config-sources.md` 中建立第二份模板来源。

## 竞品复核（2026-08-17）

- [1Panel v2 编排文档](https://1panel.cn/docs/v2/user_manual/containers/compose/)覆盖编辑、选择现有路径、
  模板和编排内的容器详情，但其文档说明编辑和启停仅适用于 1Panel 创建的 Compose。KPanel 保留容器
  列表内就地编组，并以 Docker 标准 labels 与实际配置文件为准，支持继续管理 `kejilion.sh`、SSH 或
  KPanel 创建的项目，不引入来源护栏。
- [1Panel v2 版本记录](https://1panel.cn/docs/v2/changelog/)显示其已增加容器与 Compose 备份恢复。
  KPanel 本次沿用既有 Docker 备份链路，不扩展新的 Compose 影子状态；按项目备份/恢复属于后续独立
  需求，不与本次编组、配置编辑和重新部署混做。
- [宝塔 Docker 编排文档](https://docs.bt.cn/category/%E5%AE%B9%E5%99%A8%E7%BC%96%E6%8E%92)
  当前公开目录重点覆盖编排备份、还原和跨服务器迁移。该复核没有发现需要改变本次核心交互的证据；
  KPanel 仍以低层 Compose 校验、原子写入和可恢复任务作为差异化基础。

## 输入与资源边界

| 项目 | 边界 |
| --- | --- |
| 浏览器请求 | 保留 Panel Session、Origin、CSRF 和 JSON 严格解码 |
| Compose 源码 | UTF-8、无 NUL，最多 `24 KiB` |
| Compose 项目名 | `[a-z0-9][a-z0-9_-]{0,62}` |
| 项目目录 | 仅新建 `/home/docker/<project>`；同名目录或同名运行项目返回资源冲突 |
| Compose 命令 | 固定 root-owned `/usr/bin/docker` 或 `/bin/docker`，参数独立传递，不使用 Shell |
| 命令输出 | 单次最多保留 `256 KiB`，错误进入任务前再次脱敏和截断 |
| 并发与时间 | 复用单个 Docker 后台任务互斥；任务总超时 `30 分钟` |

Docker Run 前端解析只接受单条命令，拒绝管道、重定向、后续 Shell 命令和未支持的选项。常见名称、
端口、挂载、环境变量、网络、重启策略和启动参数转换为既有类型。没有本地镜像时按 Docker Run 默认
语义拉取缺失镜像后重试创建。

Compose YAML 可以使用 Docker Compose 支持的管理员能力，包括 host network、privileged、设备和
宿主机挂载；KPanel 不以风险为由削减底层能力。真实缺失的相对文件、环境文件、构建上下文或 Compose
插件会由固定 Compose 校验返回可纠正错误。

浏览器会从 Compose 中识别 `$VAR`、`${VAR}`、`${VAR:?message}` 与 `${VAR:-default}`。没有默认值的
变量按需展示在“项目变量”中，用户无需手工创建或引用私有环境文件；Agent 将变量写入项目目录标准
`.env`，并在 `config`、`up`、启停和重新部署时显式加载。该文件用于 Compose 插值；变量只有在服务的
`environment` 或 `env_file` 中声明后才会进入容器。任意其他相对 `env_file` 仍需用户自行提供。

## 执行、成功与回滚

Compose 部署按以下顺序执行：

1. 复核项目目录和 Docker Engine 中不存在同名 Compose 项目；
2. 以 `0750` 创建项目目录，以 `0640` 原子创建 `docker-compose.yml`，以 `0600` 创建标准 `.env`；
3. 执行 `docker compose config --services`，确认至少存在一个活动服务；
4. 执行 `docker compose up --detach`；
5. 执行 `docker compose ps --all --quiet`，至少读取到一个真实容器 ID 后才判定成功。

配置校验失败时直接删除本次新建目录。启动或产物复核失败时，使用独立的 30 秒回滚上下文执行
`docker compose down --remove-orphans`；回滚成功后删除本次项目目录。回滚失败时保留
`docker-compose.yml` 并明确标记“需要人工处理”，避免丢失恢复依据。默认 `down` 不删除命名卷。

已有项目修改配置时，Agent 在执行前重新发现项目并核对 Compose 与 `.env` 共同计算的
`resourceVersion`，然后在原配置目录中创建同属主的暂存文件。完整配置集合先使用暂存 Compose 和
暂存 `.env` 执行 `docker compose config --services`；通过后依次原子替换目标文件，再执行
`docker compose up --detach --remove-orphans` 并复核真实容器 ID。若启动或复核失败，同时恢复原
Compose 与 `.env` 并再次 `up`；配置恢复或运行态恢复失败时保留明确的“需要人工处理”状态。
容器数量、运行状态和随时间变化的状态文本不参与配置版本计算，因此手工删除某个服务容器不会制造
配置冲突；重新部署会由 `compose up` 重建缺失容器。受管项目即使已无现存容器，也会按项目名从
`/home/docker/<project>` 或 `/home/web/<project>` 的标准 Compose 文件安全恢复发现。项目任务始终
使用独立参数调用固定 Docker 可执行文件，不拼接 Shell，也不扫描受管根目录以外的路径。容器列表会
保留这类 `0` 容器 Compose 项目入口，用户可直接打开配置并重新部署恢复。

## 验收范围

- 自动测试：Docker Run 引号/换行/端口/挂载/环境变量解析、行列诊断、Shell 链拒绝、Compose YAML
  语法定位与编组；Compose 新建成功、启动失败回滚、回滚失败保留恢复文件、同名冲突；已有项目读取、
  暂存校验、原子替换、删除全部服务容器后重新发现与重建、重新部署失败恢复；缺失镜像自动拉取。
- Linux 构建：`paneld`、`kejilion-agent`、`kejilion-node`、`kpctl` 的 Linux/amd64 与 Linux/arm64
  无 CGO 二进制可编译。
- 隔离真机待验证：真实 Docker Compose 插件、镜像拉取、端口冲突回滚、`kejilion.sh` 发现/备份/恢复。
- 浏览器（2026-08-17 本地模拟 API 已验证）：桌面与 390px 窄视口无页面横向溢出；Compose 编组与
  管理弹窗可读；点击错误诊断会聚焦输入框并选中准确源码范围。真实 Agent 交互仍归入隔离真机验收。
