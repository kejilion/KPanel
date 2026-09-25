# KPanel 桌面 3D 场景包开发规范

KPanel 桌面模式可以把一个 **3D 场景包** 当作动态壁纸运行：星球与空间站、霓虹都市、海边公路、云上天宫……
只要浏览器能画出来，就可以成为桌面。本目录就是场景仓库：面板从这里读取目录、按需下载场景包，用户
可以随时删除。

规范只约束三件事：**宿主协议、安全边界、体积上限**。用什么引擎、什么美术风格、什么镜头语言，全部由作者
决定。

## 1. 工作方式

```text
scene-packs/
  catalog.json            ← 仓库目录（构建脚本生成，记录每个文件的大小与 SHA-256）
  _template/              ← 起步模板（原生 WebGL2，无依赖；不会被发布）
  orbital-station/        ← 一个场景包，目录名即包 ID
    manifest.json
    index.html            ← 场景页面入口
    src/                  ← 可读源码（审核用，必需）
    poster.webp  thumb.webp  [preview.webm]
    dist/                 ← 面板实际下载的发布文件
```

1. 面板读取 `catalog.json`，在「更换壁纸 → 3D 场景」中展示每个场景；
2. 用户点「下载」后，**面板服务端** 按目录逐个下载 `dist/` 中的文件并校验大小与 SHA-256，任何一个不符就整体放弃；
   下载地址为 `https://raw.githubusercontent.com/kejilion/KPanel/main/scene-packs/<id>/dist/<file>`，
   GitHub 访问不佳时改用 `https://gh.kejilion.pro/` 前缀的加速镜像（线路可在界面中选择：自动 / GitHub 直连 / 加速镜像）；
3. 应用后，桌面在一个 **只允许脚本的沙箱 iframe** 中打开 `index.html`，场景就是桌面背景，面板换成该场景的主题色；
4. 用户删除后，本地文件被清除，桌面回到经典壁纸。

已安装的文件由面板服务端提供，地址带有每次安装新生成的 128 位随机 capability；更新或删除会撤销旧地址。
文件响应带基于 SHA-256 的弱 `ETag`，浏览器可保留缓存，但每次使用前都向服务端重新验证；已删除或已替换的 capability 不会因本地缓存继续运行。
脚本、JSON、glTF、`.bin`、`.glb`、`.wasm` 等文件在 gzip 明显缩小时按浏览器请求压缩传输。场景用相对路径加载自己的文件即可，
**不要**自己加版本查询参数或预先压缩文件。

## 2. 创作自由（明确允许）

- 任意渲染技术：Three.js、Babylon.js、PlayCanvas、原生 WebGL/WebGL2、WebGPU、Canvas 2D、SVG、CSS 3D、WASM 引擎；
- 任意着色器、后期、粒子、物理、程序化生成；可以自带模型（glTF/GLB）、纹理（KTX2/WebP/AVIF）、HDR 环境图和字体；
- 任意风格：写实、二次元、低多边形、赛博朋克、水墨、抽象；
- 1–6 个机位，进场动画、待机动画、机位转场都自由设计；可以按本地时间、季节变化；
- 任意构建工具：可以使用仓库自带的标准构建（见第 7 节），也可以用自己的工具链，只要提交可读源码和构建说明。

## 3. 目录与文件

| 文件 | 必需 | 说明 |
| --- | --- | --- |
| `manifest.json` | 是 | 元数据，见第 4 节；`dist/` 中也要有一份相同副本 |
| `index.html` | 是 | 入口页面；脚本必须是外部文件（`<script src="./scene.js">`），CSP 不允许内联脚本 |
| `poster.webp` | 是 | 场景静止时的一帧，16:9，建议 1664×936，≤ 200 KB；只在“减少动态效果”或场景运行失败时显示 |
| `thumb.webp` | 是 | 选择器缩略图，16:9，400×225，≤ 40 KB |
| `preview.webm` | 否 | 选择器悬停预览，≤ 8 秒、≤ 3 MB、无声 |
| `src/` | 是 | 可读源码；压缩或第三方库请在 manifest `dependencies` 中声明来源与版本 |
| `assets/` | 否 | 运行时加载的模型、贴图与数据（如 `.gltf`、`.glb`、`.webp`、`.bin`），构建时原样复制到 `dist/assets/`；场景用相对路径 `fetch` 或加载器读取。带贴图的模型请把贴图存成单独文件（glTF 分离格式）：嵌入模型的图片会经 `blob:` 地址加载，被沙箱 CSP 拦截 |
| 其他目录 | 否 | 生成素材的脚本等（如 Blender 脚本），随源码提交供审核与复现，不发布；生成步骤写进包目录的 `BUILD.md` |
| `dist/` | 是 | 发布文件，面板只下载这里的内容 |

**允许的扩展名**：`html js mjs css json webp png jpg jpeg avif ktx2 basis glb gltf bin hdr exr wasm woff2 webm mp4 txt md`。
**限制**：每个包 ≤ 200 个文件、总计 ≤ 30 MB、单个文件 ≤ 16 MB、路径深度 ≤ 4 层；文件名只用小写字母、数字、`-`、`_`、`.`。

## 4. manifest.json

```json
{
  "schema": 1,
  "id": "orbital-station",
  "version": "1.0.0",
  "runtime": "kpanel-scene-pack@1",
  "name": { "zh-CN": "星港轨道", "zh-TW": "星港軌道", "en-US": "Orbital Station" },
  "description": { "zh-CN": "……", "zh-TW": "……", "en-US": "……" },
  "author": { "name": "你的名字", "url": "https://github.com/you" },
  "license": "MIT",
  "tags": ["space", "sci-fi"],
  "entry": "index.html",
  "poster": "poster.webp",
  "thumb": "thumb.webp",
  "theme": { "brand": "#2f6fd0", "neutral": "#33415a", "signature": "#35b8d6" },
  "cameras": [
    { "id": "panorama", "name": { "zh-CN": "全景", "en-US": "Panorama" } },
    { "id": "sunrise", "name": { "zh-CN": "日出", "en-US": "Sunrise" } }
  ],
  "dependencies": [{ "name": "three", "version": "0.186.0", "license": "MIT" }]
}
```

- `id`：小写字母数字与 `-`，1–40 个字符，与目录名一致，发布后不可更改；
- `version`：语义化版本；内容有任何变化都要升版本，面板据此提示更新；
- `name` / `description`：至少提供 `zh-CN` 与 `en-US`；中文名称 ≤ 20 字、描述 ≤ 40 字，英文名称 ≤ 40 字符、描述 ≤ 90 字符；
- `author.name`：`KPanel` 保留给官方场景；
- `license`：SPDX 标识，场景与素材必须可以被 KPanel 用户自由使用；
- `theme`：场景的面板主题色方案，`brand`（主色）、`neutral`（中性色）、`signature`（点缀色）三项，均为 `#rrggbb`。
  每个场景只有这一套：用户应用场景时面板换成这组颜色，就像静态壁纸配套的主题一样，切换机位时不变；
- `cameras`：1–6 个，顺序即机位编号；机位名中文 ≤ 12 字、英文 ≤ 30 字符。机位不能单独设置主题色。

## 5. 宿主协议

场景页面与桌面之间只有 `postMessage`。

**场景 → 桌面**（发给 `window.parent`，`targetOrigin` 使用 `'*'`，消息不含任何敏感数据）：

| 消息 | 何时发送 |
| --- | --- |
| `{ source: 'kpanel-scene-pack', type: 'progress', value }` | 加载中（可选）：进度 0–1，只增不减，建议按整百分比发送。每条消息也告诉桌面场景仍在加载 |
| `{ source: 'kpanel-scene-pack', type: 'ready', cameras: ['panorama', …] }` | 第一帧画出后立即发送（**必需**；从场景最后一条消息算起 20 秒内没有新消息时，桌面改用海报） |
| `{ source: 'kpanel-scene-pack', type: 'camera', index }` | 机位切换开始时（可选） |
| `{ source: 'kpanel-scene-pack', type: 'error', reason }` | WebGL 不可用或上下文丢失时（建议），桌面会改用海报 |

**桌面 → 场景**（只接受 `event.source === window.parent` 的消息）：

| 消息 | 场景应当 |
| --- | --- |
| `{ source: 'kpanel-desktop', type: 'pause' }` | 停止渲染循环（窗口铺满桌面、页面隐藏时） |
| `{ source: 'kpanel-desktop', type: 'resume' }` | 恢复渲染，时间不要跳变 |
| `{ source: 'kpanel-desktop', type: 'camera', index? }` | 转到指定机位，未给 `index` 时切到下一个 |

**从黑场开始**：动态场景运行时，面板从启动到场景就绪一直显示纯黑，不显示海报；加载超过约 1 秒时，黑场中央出现一条细进度线，
由场景的 `progress` 消息驱动（不发送时停在起点）。因此场景的第一帧应当是黑的，进场从黑场淡入，这样启动 → 进场 → 待机是一个
连贯的镜头。建议先编译着色器、画出第一帧再发送 `ready`。

**加载**：桌面记得每个场景上次的文件地址，打开时立即开始加载，不等场景列表。黑场的时长就是场景自己的加载时间，建议：

- 所有文件请求一开始就同时发出，不要等一个文件到了再请求下一个；浏览器命中缓存时用 `ETag` 验证，不重复传输文件内容；
- 等待下载的同时构建场景、编译着色器：Three.js 用 `renderer.compileAsync(scene, camera)`（浏览器支持时不阻塞页面），
  还没到的贴图先用 `null` 占位，到了再设置；贴图到达时用 `renderer.initTexture(texture)` 立即上传显卡，而不是等第一帧；
- 发送 `progress`：Three.js 的加载器都经过 `THREE.DefaultLoadingManager`，在它的 `onProgress` 里换算成进度即可；
- 大模型用 Draco 压缩（`KHR_draco_mesh_compression`）：把解码器（如 `three/examples/jsm/libs/draco/gltf/` 的
  `draco_wasm_wrapper.js` 与 `draco_decoder.wasm`）放进 `assets/`，用 `setDecoderPath` 指向它，并在 `dependencies` 中声明。
  标准构建会去掉 Three.js 加载器自带的默认解码器地址（否则所有版本的解码器都会被内联进 `scene.js`）；
- 贴图分辨率以场景中实际用到的最大 mip 为准：远处物体用不到的高分辨率只会拖慢首次下载。

场景每次加载都应播放进场，然后进入待机；建议每 20–40 秒自动轮换机位。页面尺寸随桌面变化，要处理 `resize`。
场景不接收鼠标和键盘事件（桌面图标在它上面），不要依赖交互。

## 6. 安全边界（硬性）

场景代码是第三方代码，KPanel 用浏览器沙箱把它和面板完全隔开：

- iframe 使用 `sandbox="allow-scripts"`：**不透明源**，读不到面板 Cookie、登录会话、localStorage、IndexedDB 和页面 DOM；
  不能打开弹窗、提交表单、跳转顶层页面、下载文件或申请摄像头、麦克风、定位、全屏等权限；
- 场景文件响应带 `sandbox allow-scripts; default-src 'none'`。脚本、样式、图片、媒体、字体、连接和 worker
  的网络来源限定到当前包的完整资源 URL 前缀，不能用同源整站的 `'self'` 放行 Panel API；另允许 WASM、
  内联样式及图片/媒体/worker 所需的 `data:`、`blob:`。子框架、表单、base 和 object 禁用。
  场景依赖必须打包进 `dist/`，受 CSP 控制的资源请求不能访问包外路径或第三方服务；
- 管理目录、缩略图、海报要求登录，安装/删除/线路修改继续校验 Origin、CSRF 并写审计。
  沙箱没有登录凭据，资源地址使用每次安装生成的 128 位随机 capability，只开放该包清单中的已校验文件，
  并仅对这些文件开放无凭据 CORS。更新/删除即撤销旧地址；地址不包含账户、会话或任何宿主机数据；
- 场景没有声音：自动播放在沙箱中不可用，也不应尝试；
- 面板只安装仓库目录中列出的包，逐文件校验 SHA-256，不执行下载内容以外的任何东西。

服务端存储与恢复：`DataDir/scene-packs/state.json` 是 schema 1 的有界 JSON 索引（≤ 2 MiB），目录 `0700`、
文件 `0600`；每个 Panel 最多安装 10 个包、总计 150 MiB，更新暂存额外最多一个 30 MiB 包。
下载总时限 3 分钟、单请求 20 秒，最多 3 个上游请求、1 个安装/删除操作；文件响应最多并发 4 个。
只从固定官方仓库和固定镜像取资源，在发送重定向请求前拒绝跳转，并拒绝非公网地址，所有文件校验完成后原子替换索引。
失败保留旧版本，重启清理未提交的暂存对象；索引损坏时只禁用场景服务并保留现场，不影响面板启动。
已安装包保留到用户删除；目录缓存 5 分钟、失败退避 30 秒，离线时仍提供已安装包并显示目录不可用。
自动线路在镜像成功后 5 分钟内优先复用镜像，避免每个资源都重复等待直连超时；镜像失败仍尝试固定直连来源，
目录必须通过 JSON/schema 校验后才算线路成功，所有资源继续校验大小和哈希。5 分钟后重新优先直连，手动切换线路会清除这项临时偏好。
这些是可重新下载的可选资源，不进入账户/主机备份；恢复备份后重新下载所需场景。
回退 RC4 时旧服务忽略这个独立目录，回到 RC5 后继续读取 schema 1；无需迁移账户或 Agent 状态。

**禁止的内容**（审核会拒绝）：伪造系统弹窗、登录框、密码提示或 KPanel 界面；仿冒其他产品或品牌；广告与推广文字；
成人、暴力、仇恨内容；挖矿或任何与画面无关的计算；刻意耗尽 CPU、GPU 或内存；尝试探测或突破沙箱。

## 7. 本地开发

标准工具链（Three.js 由仓库 `web/` 提供，无需自己安装）：

```bash
# 1. 复制 _template/（或任意一个包）为 scene-packs/<你的 ID>/，修改 manifest.json 与 src/
# 2. 构建：打包 src/main.ts 为 dist/scene.js，复制入口、图片与 assets/，并更新 catalog.json
npm --prefix web run scene-packs:build
# 3. 校验：manifest、文件类型与体积、catalog 与 dist 是否一致
node scripts/check-scene-packs.mjs
```

预览：用任意静态服务器打开 `scene-packs/<id>/dist/index.html` 即可（URL 加 `?entrance=off&shot=1` 可跳过进场、直接看第 2 个机位，
这是示例包的约定，不是协议要求）。桌面模式整体预览使用仓库的本地预览流程。

自带工具链：只需让 `dist/` 满足第 3 节，提交 `src/` 与一段构建说明（写在包目录的 `BUILD.md`），然后运行
`npm --prefix web run scene-packs:build` 刷新 `catalog.json`（没有 `src/main.*` 的包只会被重新计算哈希，不会被改写）。

## 8. 质量建议（审核会参考，不强制）

- 3 秒内发送 `ready`（文件已缓存时 1 秒内）；首次下载尽量不超过 10 MB；1080p、中端集显上待机不低于 30 fps；
- 设备像素比封顶 1.5–2，暂停时完全停止 `requestAnimationFrame`；
- 构图避开左侧图标列（约 18% 宽）与右侧小部件区（约 22% 宽），主体放在中间；
- 画面不要大面积高频闪烁（每秒闪烁不超过 3 次）；
- 进场 3–6 秒，**简洁**：从黑场淡入，配合一个连贯的镜头运动即可，不要闪白、爆闪或堆叠多段特效；
- 待机动作缓慢、有“呼吸感”，可长时间观看；
- 用 `requestAnimationFrame` 的时间戳推进时间时，把单帧步长限制在 0 以上、0.05–0.1 秒以内，避免首帧或恢复后时间倒流、跳变。

## 9. 提交与发布

1. 在 KPanel 仓库新建 `scene-packs/<id>/`，包含源码、`dist/`、图片与许可证信息，运行构建与校验；
2. 提交 Pull Request，说明创作思路、使用的引擎与素材来源；
3. 维护者审阅源码、在隔离环境运行场景并对照第 6 节检查，合并后面板即可在场景列表中看到它；
4. 更新场景时升级 `version` 并重新构建，已下载的用户会看到新版本。
