# KPanel 轻量简约视觉主题重做验收记录

- 记录状态：待提交（本地验收完成）
- 记录日期：2026-09-02
- 适用业务域：Web 视觉语言、经典外壳、桌面模式外壳、主题求解器
- 工作树 / 分支：`C:/GitHub/kejilion-panel-claude-visual-refinement` / `feature/visual-refinement-pass`
- 精确基线：`06e43b7f572245165f7ed71e929f9ce1ceed7916`（`origin/main`，`clean=true ahead=0 behind=0`）
- 权限状态：仅授权本任务分支的一个聚焦本地提交；未授权推送、更新 `main`、tag、Release 或部署
- 依据规范：`docs/ui-visual-language.md` §3.1 / §7、`PROJECT_RULES.md` L1

## 本轮目标

对现有主题做一轮轻量简约重做，覆盖深浅模式、横竖屏、经典与桌面两种外壳，并把「AI 味」
从主观印象转成可自动校验的客观约束。

## 观察证据（改动前实测）

| 现象 | 基线实测 |
| --- | --- |
| 圆角失控 | `main.css` 17 种字面圆角，`desktop.css` 13 种；语义 token 仅 3 个 |
| 字重虚构 | 97 条 `font-weight` 中 77 条不在 400/500/600/700；全仓无 `@font-face`，细分字重被浏览器就近吸附 |
| 浅色无层级 | `--surface` 与 `--surface-raised` 同为 `#ffffff`，35 处抬升面消费者在浅色下无抬升 |
| 装饰性渐变 | ambient 含品牌 radial glow，`--surface-gradient` 为 155° 三段渐变，违反 §3.1 |
| 中文负字距 | 20 处负 `letter-spacing`，最低 -0.048em |
| 玻璃感堆叠 | 8 处 `backdrop-filter` 叠 `inset 0 1px 0 rgb(255 255 255 / 18%)` 假高光 |
| 横屏零支持 | 全仓 0 条 `orientation` 媒体查询 |
| 主题双份失同步 | `themes.css` 与 `theme/colors.ts` 的阴影与 aurora 取值不一致 |

## 本轮改动

- **浅色四级中性阶**：`--bg #eff3f2` < `--surface-subtle #f5f8f7` < `--surface #fbfcfc` <
  `--surface-raised #ffffff`。抬升面第一次真正浮起；`--surface-gradient` 因此从装饰变成真实顶部受光。
- **深色阶保持「向页面递暗、嵌套内容抬升」**：`--bg` < `--surface` < `--surface-subtle` < `--surface-raised`；
  `--surface-subtle` 的 43 处消费者均为面板内嵌入式填充（列表井、输入托盘、内嵌卡片），在深色下必须
  高于父面板才可分辨，属于设计意图而非顺序缺陷。
- **降噪**：ambient 去 radial glow 改单段纵向 wash；`--surface-gradient` 收为两色标；阴影改为
  「1px 接触阴影 + 仅浮层追加一层扩散」。
- **求解器镜像**：`theme/colors.ts` 的浅色基座亮度、四个阴影与 aurora 透明度与 `themes.css` 逐字对齐，
  修掉既有失同步，自定义配色继承同一层级。浅色四级阶实测与发货表**逐色号完全一致**
  （`#eff3f2 / #f5f8f7 / #fbfcfc / #ffffff`，四级 Δl 均为 0.0000）。
- **求解器钳位塌陷（本轮自身缺陷，提交前复查 diff 发现）**：原实现按「基准亮度 + 色调偏移」逐级独立
  计算，`surfaceRaised` 取 `1 + offset`，而 `setLightness` 内部 `clamp(l, 0, 1)`；偏亮的中性意图会把
  `surface`(0.982+off) 与 `raised`(1+off) 同时钳到 `#ffffff`，正好复现本轮要修的「浅色无层级」缺陷。
  实测 8 个浅色中性意图中 5 个的 `raised - surface` 为 0.00000。改为**整梯固定跨度、自顶悬挂**
  （`LIGHT_LADDER.span = 0.055`，两个中间级按发货表比例定位），偏亮意图保持顶端贴白而非拉伸。
  修复后 18 个 意图×模式 组合全部单调，最小步进 ≥0.0137。
- **顺带修复既有求解器脆弱点**：极浅中性意图（如 `#daf0ba`）会同时产出近黑侧栏与近白面，
  强调色需同时满足「对侧栏 ≥4.55」与「对两个面 ≥3.05」，可行窗口不足一个亮度搜索步。基线是靠
  `--surface` 与 `--surface-raised` 恰好同为纯白才侥幸通过（实测 3.09，仅高出 0.04）；给 `--surface`
  真实层级后窗口收窄至无解。`closestAccessibleColor` 增补降饱和回退搜索：先按原色相找最近亮度，
  无解时逐级降饱和（0.7→0），用让渡部分色相换取两个背景上的可读性，而非抛错。
- **节奏收敛**：约 110 处字面圆角归并到三个 token；字重收敛到 500/600/700；删除全部负字距，
  `.metric-card > strong` 改用 `font-variant-numeric: tabular-nums` 获得真实数字对齐。
- **玻璃收敛**：`backdrop-filter` 仅保留 5 个真实消费者（桌面菜单栏与任务栏 `blur(12px) saturate(120%)`、
  拖放蒙层、两个弹窗遮罩）；右键菜单、服务状态、传输浮层等 5 处改为实心 `--surface-raised` + 描边 + token 阴影。
  中性面与窗口 chrome 的假高光删除；仅保留 5 处饱和图标砖上的顶部高光（属真实材质受光）。
- **桌面窗口关闭键**去 `#c42b1c` 硬编码，改走 `--danger-action` / `--on-danger`。
- **首次横屏矮视口支持**：`@media (max-height: 560px) and (orientation: landscape)`，经典侧压缩
  topbar 至 56px 并把弹窗高度从 `min(90vh, 860px)` 改为按 `100dvh` 与安全区实测；桌面侧任务栏 44px、
  标题栏 38px、窗口按实际 chrome 重切工作区。两个块均置于既有竖屏/粗指针块之后，层叠不互相击穿。
- **AI 工作区**正文/标签由 12px 提到 13px 次要级，仅真元数据角标保留 12px。

## 验收字段

- **受影响页面/旅程**：概览、设置-外观、AI 工作区、文件、Docker、终端、体检、集群、应用市场；
  经典外壳与桌面外壳；弹窗（端口占用/磁盘/日志/账户/调优）；桌面图标、任务栏、窗口、右键菜单。
- **最小计算字号及用途**：32/32 个矩阵单元实测最小 12px，出现在 `.status-badge`（状态角标）、
  `.desktop__icon-label`（桌面图标标签）、`.desktop-clock__header`（时钟部件眉标）——均为角标/眉标类
  真元数据，符合 12px 绝对下限；无任何单元低于 12px。
- **正文/操作控件字号**：正文 14px 不变；次要 13px；横屏弹窗页脚按钮实测最小高度 40px；
  横屏任务栏控件实测最小 40px。
- **视口与缩放**：390×844、768×1024、1280×800，缩放 100% / 125% / 200%，另加 844×390 横屏，
  共 32 个截图单元（8 视口/缩放 × 深浅 × 经典/桌面）。200% 下重排无裁切或重叠。
- **浅色/深色主题**：均覆盖；实测浅色四级阶单调递增且每步 ≥0.004 相对亮度（发货表），
  求解器侧每步 ≥0.0137 HSL 亮度；深色按上文语义排序。
- **键盘/焦点/触摸**：焦点环与内阴影发丝线未改动，`:focus-visible` 规则保持原样；触摸目标在横屏
  下实测 ≥40px；`prefers-reduced-motion` 与 `forced-colors` 块未改动。
- **加载/空/失败/部分成功**：mock 预览覆盖「外网查询不可用」「等待下一次采样」「需要关注／2 项资源
  未处于正常状态」等空/降级态，均随新中性阶正常呈现。
- **长文本与多语言**：中文界面实测；删除负字距后中文字距回归正常。横屏 844px 宽下发现并修复
  任务栏「切换回经典模式」标签在 40px 控件内逐字换行的缺陷（改为与 ≤760px 一致隐藏标签）。
- **对比度或可访问性证据**：语义 token 对与 `--on-brand` / `--on-danger` 配对关系未改动，
  `theme/contract.test.ts` 与 `theme/colors.test.ts` 的 WCAG 约束保持绿；本轮只改中性阶与阴影，
  未改前景色与品牌色，故未引入新的对比度风险。
- **预览等级、候选提交和证据目录**：预览等级 `mock-ui`（draft，未提交前）；
  证据目录 `.codex-tmp/previews/visual-refinement-pass-20260901155945/`，含
  `landscape-probe.json`（横屏几何实测）、`shots-recheck/`（32 单元截图）、
  `shots-recheck/visual-audit.json`（逐单元最小字号与解析后的 token 值）。
- **历史小字号迁移债务与本次未处理范围**：
  - `.status-badge`、`.desktop__icon-label`、`.desktop-clock__header` 保留 12px，属规范允许的角标层，
    本次不再上调。
  - `.desktop__menubar` 是 `theme/contract.test.ts:88` 仍钉死、但全仓无任何组件渲染的遗留选择器；
    本次不删除该 CSS（会破契约），但横屏块已不再为它预留顶部高度。**建议后续单独任务**同时清理
    契约断言与 CSS。
  - `main.css` / `desktop.css` 中非抬升类的字面 `box-shadow`（发丝内阴影、焦点环）未归并，属既有债务。
  - 1024×600 等高于 560px 的横屏视口沿用原竖屏/桌面规则，本次不改（已用该视口作为对照，确认
    横屏覆盖不外溢）。

## 横屏专项实测

CDP 直连真实 Chrome 实测（`landscape-probe.json`）：

| 检查项 | 844×390 | 1024×552 | 1024×600（对照） |
| --- | --- | --- | --- |
| 横屏查询命中 | 是 | 是 | 否（正确不命中） |
| 弹窗溢出视口 | 无 | 无 | 无 |
| 页脚可达 | 是 | 是 | 是 |
| 页脚按钮最小高度 | 40px | 40px | 40px |
| 窗口被任务栏遮挡 | 否 | 否 | 否 |
| 任务栏控件最小尺寸 | 40px | 40px | 32px（基线未改） |

实测过程中发现并修复本轮自身引入的一处缺陷：横屏块原为无任何组件渲染的 `.desktop__menubar`
预留 44px 顶部空间，白白占用横屏最紧缺的纵轴。去除后 844×390 下窗口可用高度由 282px 提升到
324px（+15%）。

## 验证

- `npm run typecheck`：通过。
- `npm test`：127 个文件 / 1081 个测试全绿（含新增 `web/src/styles/VisualRhythm.test.ts` 16 条断言，
  以及 `theme/colors.test.ts` 新增的求解器层级不变量）。
- `node scripts/run-repo-bash.mjs scripts/verify-change.sh`：生态策略、业务上下文新鲜度、治理一致性、
  全量测试与生产构建均通过（`Change-aware verification completed.`，退出码 0）。
- 新增契约测试经变异验证：注入 `border-radius: 10px` / `font-weight: 650` /
  `letter-spacing: -0.02em` / 越界 `backdrop-filter` 后，对应 4 条断言均如期失败，随后完整还原。
- 求解器层级不变量并入既有 96 组随机配色 fuzz 循环（96 × 深浅两模式 = 192 次求解），断言浅色
  `bg < subtle < surface < raised`、深色 `bg < surface < subtle < raised`，每步 HSL 亮度 >0.01。
  该断言在修复前会命中真实塌陷案例，属有效约束而非恒真。

## 防回归

新增 `web/src/styles/VisualRhythm.test.ts` 作为本轮脊柱，断言：圆角只用 token/胶囊/白名单发丝值、
嵌套内角由 token 派生、字重限定四档、无负字距、玻璃只在许可选择器、中性面与窗口 chrome 无假高光、
关闭键走危险色对、浅色四级阶单调、深色阶语义排序、两份主题表达的阴影模型一致、层级只用一次受光、
横屏块存在且含预期声明、横屏触摸目标 ≥40px、横屏只按真实 chrome 预留空间、横屏块排在竖屏块之后。
