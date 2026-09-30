# 集群分享主题库

与 `scene-packs/` 一样，这是仓库内开放源码的独立包目录。面板只内置默认样式；管理员在「集群 → 公开分享 → 分享主题」下载、应用或删除可选主题。删除当前主题自动恢复默认样式。已安装主题无需再次访问 GitHub。

## 设计原则

一个主题就是一个完整的网页。KPanel 只负责三件事：把**公开、经过白名单过滤的数据**送进沙箱、保证隐私与安全边界、提供「刷新 / 浅深色 / 使用默认样式」这几个外框入口。除此之外——版式、配色、字体、图表、SVG/Canvas、动效、交互（排序、筛选、展开、键盘快捷键……）——全部由主题作者自由决定。

- **不依赖任何框架**：主题不继承 KPanel 的 CSS、组件或设计令牌，也没有共享渲染器。仓库里的官方主题各自独立实现（表格终端、杂志版式、状态墙），互不复用代码，正是为了证明这一点。
- **数据是原料，不是成品**：每个指标同时给出原始数值和已格式化文本，主题可以画仪表、热力图、迷你趋势，也可以只写文字。
- **词汇由核心提供**：状态名、指标名、提示语已按访客语言本地化放在 `labels` 中，主题无需自带翻译。

## 提交自己的主题

1. 复制 `_template/` 为自己的短名称目录，例如 `my-fleet/`，更新 `manifest.json` 的 ID、名称、版本、作者及许可证。`theme` 三个颜色仅用于管理界面的配色预览，请取主题最有代表性的背景、主色和点缀色。
2. 在 `src/` 里写 `index.html`、`theme.css`、`theme.js`（或任何你喜欢的文件组织）。无需打包器；使用框架时须将完整源码、构建方法和所有运行依赖放在仓库中，禁止 CDN、统计脚本、外链字体、混淆代码。
3. 从仓库根目录运行 `node scripts/build-share-themes.mjs`，同时提交源码、生成的 `dist/` 与 `catalog.json`。校验命令为 `node scripts/build-share-themes.mjs --check` 和 `go test ./internal/scenepacks`。
4. 向本仓库提交 PR，附 390px / 768px / 1280px、浅色 / 深色、200% 缩放截图。维护者复核后目录才对用户可见。

目录来源固定为 `https://raw.githubusercontent.com/kejilion/KPanel/main/share-themes/`，自动使用现有 GitHub 镜像回退。主题文件逐个校验 SHA-256 和字节数，完整下载后原子安装。每包最多 40 个文件、5 MiB；最多安装 10 包，总存储遵循共享包引擎 150 MiB 上限。`index.html` 和 `manifest.json` 各不超过 64 KiB。文件名小写，目录最多 4 层，不允许软链接、路径穿越或任意下载地址。允许的文件类型：html、css、js、mjs、json、webp、png、jpg、avif、woff2、txt、md。

## 运行协议 `kpanel-share-theme@2`

`manifest.json` 中 `runtime` 填 `kpanel-share-theme@2`。主题运行于 `sandbox="allow-scripts"` 的独立 iframe，HTTP CSP 也强制隔离：不能访问面板 DOM、Cookie、存储、管理 API、任意网络、弹窗、表单或顶层导航。所有 JS、CSS、图片和字体使用包内相对路径；JS 不得内联。12 秒内未发出 `ready` 自动回退默认样式。

`_template/src/theme.js` 顶部的注释是完整字段参考，可直接复制。流程：

1. 监听 `message` 后立即声明协议版本：
   ```js
   parent.postMessage({ source: 'kpanel-share-theme', type: 'ready', protocol: 2 }, '*')
   ```
   不带 `protocol` 的 `ready` 视为旧协议 1（保持原有 schema 1 快照，已安装的旧主题不受影响）。
2. 父页面发送 `{ source: 'kpanel-share', type: 'snapshot', schema: 2, locale, mode, labels, data }`。先校验 `event.source === parent` 与 source/type/schema。`mode` 为 `light` / `dark`，`locale` 为 `zh-CN` / `zh-TW` / `en-US`。数据更新、语言或浅深色变化时会再次发送，无需主题自行轮询。
3. 可选发送 `{ source: 'kpanel-share-theme', type: 'resize', height }` 调整内容高度；只接受 320–32768 的整数像素，避免嵌套滚动条，超出范围保持原高度。

### `data` 字段

| 字段 | 说明 |
| --- | --- |
| `title` `description` `generatedAt` | 分享标题、一句话介绍（可能为空）、快照生成时间（ISO 8601） |
| `counts` | `total` `online` `attention` `offline` |
| `states` | 四种状态的本地化名称 |
| `value` | 剩余价值估算：`groups[]`（`currency` `text` `amount`）、`included` `excluded`；`groups` 为空时**隐藏整块**及占位 |
| `hosts[]` | 主机列表，顺序即管理员设定的公开页顺序（勾选「按当前面板顺序」时） |

每台主机：`id`（公开专用的不透明标识）、`name`、`state`（`online` `degraded` `offline` `pending`）、`stateLabel`、`os`、`architecture`、`cores`、`collected`、`location{text,country,countryCode,city,region,isp,latitude,longitude}`、`cpu{text,ratio}`、`memory{text,ratio,usedBytes,totalBytes,usedText,totalText}`、`disk{…同 memory}`、`load{one,five,fifteen}|null`、`uptime`（文本）、`uptimeSeconds`、`network{down,up}{bytesPerSecond,text}`、`traffic`、`price`、`expiresOn`、`remaining`。

- `system` 是发行版标识：`key`（如 `debian`、`ubuntu`，无法识别时为 `linux`）、`label`、品牌色 `accent`，以及图形 `path`（24×24 viewBox 的 SVG 路径数据）或 `image`（仅有位图时的 `data:` URL）。推荐用 `createElementNS` 自建 `<svg>` 并只设置 `path` 的 `d` 属性，这样可以自由改色、描边、发光或做成徽章；**不要**把它拼进 HTML 字符串。
- `location.flag` 是圆形国旗的 `data:` URL（来源 circle-flags，MIT），直接作为 `<img src>` 使用，可随意裁切、加边框或作为角标；未知为空字符串。国旗和系统图标在主题就绪后异步补发，首个快照里可能为空，主题需能在收到下一次快照时更新。
- `latitude` / `longitude` 是**国家/地区中心点**（与内置地球相同的区域锚点），用于画地图；未知为 `null`。它不是机器的真实位置，主题不得据此暗示精确坐标。
- `ratio` 是 0–1 的小数，**未知为 `null`**；文本未知为 `—`。未知永远不要画成 0。`collected` 为 `false` 表示还没有采集到数据。
- `price` / `expiresOn` / `remaining` 为空字符串表示未配置，应隐藏整行；已配置的零值必须显示。
- `traffic` 含 `monthly`（是否启用月度配额）、`percent`（文本）、`ratio`（已用比例，最大 1）、`tone`（`normal` `warning` `danger`）、`received` `sent`、`quotaGiB`、`available` `partial` `estimated` 和 `hint`。`hint` 是核心提供的本地化说明（等待数据、不完整、估算、计费方向、接近/超额），**必须展示或提供可访问详情**，不得隐藏影响解读的状态。不得把上行和下行分别除以配额再当成两个总百分比。重置日期只在主机设置中显示，协议不提供，主题不得展示。
- 剩余价值是按已公开价格、到期日估算的预付价值，不代表退款金额。

所有指标沿用 KPanel 核心计算，主题只负责展示。协议不包含分享令牌、面板地址、真实节点 ID 或管理资料。

## 主题必须遵守

- 对名称、介绍等用户数据只用 `textContent` / 文本节点，不能 `innerHTML`。
- 提供搜索或等价的查找方式、空状态、超长名称换行和可见的键盘焦点。
- 文字最小 12px，正文和操作最小 14px，辅助文字最小 13px；颜色不能独自表达状态（配合文字、符号或形状）。
- 遵守 `prefers-reduced-motion`；支持 200% 缩放和 390px 窄屏，不出现横向页面滚动。
- 深浅色都要检查对比度；`mode` 由父页面传入，用它设置主题而不是猜测系统偏好。

## 与父页面的关系

新增/删除/更新已安装主题立即影响分享设置；已经打开的公开页会在下一次轮询（通常 15 秒）更新。访客的「使用默认样式」只影响本次浏览，不修改管理员设置。关闭/重置分享仍由现有分享令牌生命周期控制。仓库中的主题包需要随候选代码合入主线后，线上目录才能下载；本地预览从当前检出读取包，不能据此声称 GitHub 已发布。

## 官方主题

三套主题彼此不共享任何代码，分别代表三种不同的阅读方式。都支持浅/深色、搜索、状态筛选和两种视图（访客的视图选择只保存在本次浏览中）。

| ID | 名称 | 适合 | 视图 | 设计要点 |
| --- | --- | --- | --- | --- |
| `minimal` | 简约看板 | 给别人看的状态页 | 卡片 / 列表 | 品牌色系统徽章 + 国旗角标、在线率环图、8px 粗条仪表、流量区块、信息标签 |
| `orbit` | 星图 | 多地区部署、展示型分享 | 卡片 / 列表 + 世界地图 | 地图上以国旗作标记、状态作光环与计数，可点击筛选；发光系统图标；拥挤时自动合并与避让标签；三色环形仪表 |
| `midnight` | 午夜终端 | 机器多、要快速扫一遍 | 热力表 / 分屏 | 单色系统字形与小国旗、htop 风格热力单元格、按列排序、行内展开详情、`/` 与 `1–4` 键盘操作 |

`orbit/src/land.js` 是 Natural Earth v5.1.2 陆地点阵（公有领域），与内置地球同源。
