# KPanel 图标标准

KPanel 使用同一枚多彩渐变 `K` 图标覆盖面板品牌位、浏览器标签、搜索结果、iOS 主屏幕、可安装 Web 应用和应用市场条目，避免各平台出现不同字形或临时字母占位。

## 资源与用途

| 文件 | 用途 |
|---|---|
| `web/public/icons/kpanel.svg` | 标准 SVG 品牌资源、面板品牌位和现代浏览器 favicon |
| `web/public/icons/favicon-96.png` | 不支持 SVG favicon 的浏览器及搜索引擎后备 |
| `web/public/icons/apple-touch-icon.png` | iOS/iPadOS 主屏幕，180×180 |
| `web/public/icons/kpanel-192.png` | Web App Manifest 小尺寸图标 |
| `web/public/icons/kpanel-512.png` | Web App Manifest 大尺寸及 maskable 图标 |
| `web/public/icons/kpanel-mask.svg` | Safari 固定标签页单色蒙版 |
| `web/public/app-icons/kpanel.webp` | 应用市场 KPanel 条目，128×128 WebP |
| `.github/assets/kpanel-logo.png` | README 横版品牌锁定，保留 1304×292 尺寸 |

## 约束

- 图标必须保持 1:1；白色 `K`、多彩渐变背景和半透明叠层不可改为其他含义。
- `K` 的主要轮廓必须位于画布中央 80% 安全区，适配圆形、圆角方形和 squircle 裁切。
- favicon 使用稳定的站内 URL，仅在品牌资源更新时递增显式版本参数，避免浏览器继续展示旧缓存。
- 默认优先 SVG；只保留平台明确需要的 PNG 尺寸，不批量生成重复资源。
- `manifest.webmanifest` 中必须声明正确的 `sizes`、`type` 和 `purpose`。
- 新增或替换图标后运行 `npm run test` 和 `npm run build`，并检查各资源仍满足 30 KB 单文件预算。

图标只能改善品牌识别、搜索结果 favicon 资格和安装后的系统呈现，不等同于提高搜索排名。正式域名还需允许搜索引擎抓取首页和图标 URL。
