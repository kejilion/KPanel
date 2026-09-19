# 窗口页面按钮完整性契约（设计与规划）

- 状态：设计草案，未实施
- 分支：`fix/window-button-integrity-20260903`
- 工作树：`C:/GitHub/kpanel-claude-button-integrity-20260903`
- 精确基线：`origin/main` = `bfd8a5cedbe81fdb0dba6c37f5ed392a1cc253a8`（v1.0.1）
- 上级规范：`docs/ui-visual-language.md` §2.1 §2.2 §2.3 §4 §5，`PROJECT_RULES.md`
- 相关前序修复：`a6f9ef7 fix(web): reclaim vertical space in narrow desktop windows`

本文件定义"按钮在任何窗口尺寸下保持完整"的可判定契约、断点策略、自动化门禁和分期实施计划。
它不重复 `docs/ui-visual-language.md` 的字号与视觉数值，只在按钮这一控件类上把该规范落成可测断言。

---

## 1. 问题陈述

桌面窗口可被拖到 `MIN_WINDOW_WIDTH = 420`（`web/src/lib/desktopWindowGeometry.ts:29`），默认 880。
在窄窗口与部分经典模式宽度下，带文字的按钮出现三类完整性损失：

| 损失类型 | 现象 | 用户后果 |
| --- | --- | --- |
| 换行挤压 | 标签折成两行，`line-height: 1` 使字形相接 | 文案可读性下降，按钮视觉破形 |
| 省略截断 | `text-overflow: ellipsis` 把标签截成 `C..`、`M..` | 无法判断操作是什么，且无完整查看路径 |
| 越界裁切 | 按钮被父容器 `overflow: hidden` 切边 | 点击目标不完整甚至不可达 |

用户提供的三张实机截图分别复现了：窄窗口下 Docker 分区导航截断为 `C.. 2 / M.. 2 / N.. 5 / St.. 4`、
`更新环境` 与 `查看实现状态` 折行、以及经典模式宽视口下 `查看实现状态` 仍然折行。

按用户要求，**优先级是保证按钮完整性**：宁可让容器换行、横向滚动或整块堆叠，也不接受按钮自身破形。
这与 `docs/ui-visual-language.md` §5.2 "在窄屏下先换布局，再考虑减少次要字段，最后才缩短文案"一致，
也与 §2.2 明令禁止的"只显示省略号而没有完整查看路径"一致。

---

## 2. 现状盘点（静态度量，基线 bfd8a5c）

| 指标 | 数值 |
| --- | --- |
| `<button>` 元素总数 | 642 |
| 使用 `.button` 类的实例 | 392 |
| `.icon-button` 使用 | 106 |
| `.button--small`（12px 字号）使用 | 97 |
| `.button--block` 使用 | 2 |
| 含按钮且为"不换行 flex 行"或"固定多列栅格"的容器 | 144 处，分布在 38 个文件 |
| 手机 `@media (max-width)` 适配到的选择器 | 499 |
| 其中在窄桌面窗口内**不生效**的选择器 | **372** |
| `@container desktop-window` 桥接覆盖的选择器 | 121 |
| 图标+文字按钮标签缺失 i18n 词条 | 27 条（10 个视图，100% 为图标+文字混排按钮） |

盘点方法为源码静态分析（模板中按钮的最近祖先类 → 该类的 CSS 声明），属启发式：
144 处中包含纯图标行等本身安全的情况，该数字用于确定范围与优先级，不等于 144 个确认缺陷。
372/499 与 121 两项是选择器集合的精确差集，不是估算。

---

## 3. 根因

### 根因 1：`.button` 基类没有完整性不变量

`web/src/styles/main.css:775` 的 `.button` 只声明 `display: inline-flex`、`min-height: 40px`、
`line-height: 1`，**没有** `white-space` 也**没有** `flex-shrink: 0`。因此：

- 作为 flex 子项时按内容宽度以下被压缩，标签随即折行；
- `line-height: 1` 下的两行文字上下字形相接，CJK 尤其明显；
- 全仓唯一的补丁是 `web/src/views/DockerView.vue:2207` 的
  `.card-actions .button { flex: 0 0 auto; white-space: nowrap; }` —— 局部、易漏。

这正好解释截图差异：`创建备份 / 刷新`（在 `.card-actions` 内）不折行，
而 `更新环境 / 查看实现状态`（在 `.action-card` 内，未被该补丁覆盖）折行。

### 根因 2：窄窗口是容器查询上下文，手机断点在其中全部失效

`web/src/styles/desktop.css:2292` 对窗口内容区声明 `container-name: desktop-window; container-type: inline-size;`。
窄窗口位于宽视口内，视图里的 `@media (max-width: …)` 因此完全不生效——这是 `a6f9ef7` 已记录的机制，
但当时只修了 hero 与资源区，未处理按钮类。全仓仍有 **372 个选择器**的窄屏适配无法到达窗口内。

补偿手段是 `desktop.css` 中一份手写的 `@container desktop-window` 桥接清单（覆盖 121 个选择器）。
手写重复清单必然漂移，已产生两处**完全失效的规则**：

- `web/src/styles/desktop.css:3593` 和 `:3981` 对 `.docker-nav` 设 `grid-template-columns`，
  但 `.docker-nav` 是 `display: flex`（`DockerView.vue:2178`），该属性无效；
- 视图里**已经存在**正确修复：`DockerView.vue:2404`
  `.docker-nav button { min-width: 104px; flex: 0 0 auto; }` 让导航转为横向滚动，
  但它在 `@media (max-width)` 内，窗口里读不到。

于是窗口内 `.docker-nav button` 保持 `flex: 1 1 0`（`:2180`），五个分区均分 380px，
`strong` 的 `text-overflow: ellipsis`（`:2184`）把标签截成 `C..`。
同时 `.docker-nav` 的 `overflow-x: auto` 永不触发——`flex-basis: 0` 的子项不会溢出容器。
这是一条完整、已验证的因果链，也说明该缺陷类不是"再补几条桥接规则"能收敛的。

### 根因 3：容器行为没有统一约定

144 处按钮容器各自选择 `flex` 不换行、`repeat(N, 1fr)` 固定栅格或 `overflow-x: auto`，
没有统一的"空间不足时该怎么退让"约定。`.action-grid`（`DockerView.vue:2200`）是典型：
三等分栅格在 ≤820px 容器时被折成一列，但列内 `.action-card` 是不换行 flex 行，
说明文本块 `.action-card > div` 缺 `min-width: 0`，按钮又没有 `flex: 0 0 auto`，
于是无论窗口还是经典宽视口，按钮都被挤到折行。

### 根因 4：文案宽度未纳入设计（i18n 与字号迁移债）

- 27 条按钮标签未进入 `web/src/i18n/pages/*` 词条，全部是 `<Icon /> 文字` 混排按钮；
  纯文本按钮（如 `应用镜像源`）已被抽取。抽取管线漏掉与元素同级的文本节点。
- 一旦补齐翻译，英文标签宽度约为中文的 2–3 倍（`查看实现状态` → `View implementation status`），
  **完整性预算必须按 en-US 而非 zh-CN 计算**，否则修复完当场回归。
- `.button--small` 字号 12px（`main.css:846`，97 处使用）低于 §2.1 的"操作按钮不得低于 14px"；
  `.docker-nav button small` 为 `.7rem` = 11.2px，低于 12px 绝对下限。
  提字号会再增加约 15–17% 宽度需求，必须与布局退让能力同批考虑。

---

## 4. 设计

### 4.1 按钮自身不变量（Layer 0）

`.button`、`.icon-button` 以及所有承担操作的 `button` 元素，在任意容器与任意尺寸下满足：

1. **单行且不截断**：`white-space: nowrap`，且**不得**同时设置宽度上限或 `overflow: hidden`。
   nowrap 只有与"按内容定宽"配对才合法；与宽度上限配对即违反 §2.3 的禁止裁切条款。
2. **不被压缩**：`flex: 0 0 auto`（栅格中为 `justify-self` 按内容，不用 `1fr` 承载按钮）。
3. **不越界**：`max-width: 100%` 仅作兜底；触发即视为容器契约失败，由 Layer 1 处理，而非靠按钮截断吸收。
4. **行高安全**：`line-height` 由 `1` 提升到 `1.2`（§2.3 标题下限），配合既有 `min-height` 不改变按钮外观高度。
5. **点击目标**：常规按钮 `min-height: 40px`，紧凑变体不低于 32px 且点击盒不小于 24×24（§4）。
6. **字号**：迁移目标 14px（§2.1）；迁移期内低于 14px 的必须在
   `web/src/styles/visualContractBaseline.ts` 有显式到期条目。

例外：允许换行的只有 `.button--block` 这类整行按钮，且必须显式声明 `white-space: normal` 与 ≥1.35 行高。

### 4.2 容器退让行为（Layer 1）

任何含带文字按钮的容器必须显式选择四种行为之一，不允许"无行为"：

| 行为 | 声明 | 适用 |
| --- | --- | --- |
| `wrap` 换行 | `flex-wrap: wrap` + 子项 `flex: 0 0 auto` | 2–4 个短标签按钮的操作行（默认选择） |
| `scroll-x` 横滚 | 容器 `overflow-x: auto` + 子项 `flex: 0 0 auto` + `min-width` | 分段/分区导航等顺序有意义、不宜换行的按钮组 |
| `stack` 堆叠 | 容器转单列 + 按钮 `width: 100%` | 卡片内的 1–2 个主操作 |
| `collapse` 收纳 | 保留 1 个主操作，其余进溢出菜单（带可访问名称与键盘可达） | 单容器超过 4 个操作 |

`scroll-x` 必须满足：子项 `flex-basis` 不为 0（否则滚动永不触发，即根因 2 的失效形态），
且提供滚动可发现性（渐隐或滚动条），避免出现"看不出还有内容"的隐藏操作。

`collapse` 必须满足 §2.2：不得退化为无可访问名称的纯图标按钮。

### 4.3 决策规则

按容器可用宽度 `W`、按钮数 `N`、最长标签（**以 en-US 计**）自然宽度 `Lmax` 选择：

1. `Σ按钮自然宽度 + 间距 ≤ W` → 保持单行，无需退让。
2. 否则若 `N ≤ 4` 且 `Lmax ≤ W` → `wrap`。
3. 否则若容器语义为导航/分段 → `scroll-x`。
4. 否则若 `N ≤ 2` → `stack`。
5. 否则 → `collapse`。

`Lmax > W` 时禁止用缩小字号或截断解决，按 §2.2 顺序处理：先删装饰文案 → 再改布局 → 最后才动文案。

### 4.4 断点与容器策略（Layer 2）

目标是**取消手写桥接清单**，让同一套窄屏规则同时服务经典模式与窗口模式：

1. 在 `.page-content`（`main.css:2270`）增加 `container-name: page-surface; container-type: inline-size;`；
   `.desktop-window__body` 的 `container-name` 扩为 `desktop-window page-surface`。
2. 视图内的 `@media (max-width: N)` 逐个改写为 `@container page-surface (max-width: N)`；
   真正与设备/输入方式相关的条件（`hover: none`、`pointer: coarse`、`prefers-reduced-motion`）
   保留为 `@media`。
3. 每迁移一个视图，同步删除 `desktop.css` 中对应的桥接规则，桥接清单单调收缩到 0。

必须先验证的风险：`container-type: inline-size` 建立包含块，会让内部 `position: fixed` 相对 `.page-content` 定位。
全仓仅 6 处 `position: fixed`（`DockerView.vue:2266`、`FilesView.vue:3498/4349`、
`TerminalContextMenu.vue:245`、`AppInteractiveTerminal.vue:417`、`ClusterShareView.vue:332`），
均为上下文菜单/浮层，且在窗口内已经处于容器上下文中，风险已知且有界——但仍须逐处实测确认，
不得凭"窗口里本来就这样"推断经典模式也没问题。

### 4.5 与既有规范的一致性

- §2.1：不新增 12px 以下文字；`.button--small` 与 `.docker-nav button small` 进入迁移清单。
- §2.2：不引入"只有省略号没有完整路径"的按钮；不把操作退化为无名图标。
- §2.3：nowrap 只与按内容定宽配对；不用固定高度或 `overflow: hidden` 吸收溢出；200% 缩放下靠容器退让。
- §4：点击目标与可访问名称不因退让行为退化。
- §5.1：验收覆盖 390/768/1280 视口 + 420/680/880 窗口宽度、100/125/200% 缩放、双主题、中英文案。

---

## 5. 自动化门禁

把"按钮完整性"从人工看图变成可判定断言。两层：

### 5.1 静态契约测试（新增，复用现有基础设施）

放入 `web/src/styles/ButtonIntegrity.test.ts`，复用 `web/src/styles/visualContract.ts` 的声明收集器
与 `visualContractBaseline.ts` 的到期债务模式：

1. `.button` 基类必须声明 `white-space: nowrap`、`flex: 0 0 auto`、`line-height >= 1.2`。
2. 任何规则不得对带文字按钮设 `flex-basis: 0`（`flex: 1 1 0` / `flex: 1`）——直接捕获 `.docker-nav` 类缺陷。
3. 带文字按钮不得同时出现 `text-overflow: ellipsis` 与缺少完整查看路径。
4. 不得对 `display: flex` 容器设 `grid-template-columns`——直接捕获 `desktop.css:3593/:3981` 类死规则。
5. 含按钮容器必须匹配 4.2 四种行为之一，未匹配者需在基线中显式登记并带到期日。
6. 迁移进度门禁：视图内 `@media (max-width)` 的选择器集合必须或已改写为 `@container page-surface`，
   或存在于收缩中的桥接白名单；白名单只允许变小。

### 5.2 实测浏览器检查（新增，复用后台浏览器入口）

通过 `scripts/background-browser-test.mjs` 的既有入口与环境策略，遍历 15 个窗口页面
（overview / system / monitoring / processes / cluster / ai / sites / sites-environment /
apps / files / terminal / diagnostics / docker / activity / settings），
在 **窗口 420 / 680 / 880 px × 主题 明/暗 × 语言 zh-CN/zh-TW/en-US** 下对每个可见按钮断言：

| 断言 | 判据 |
| --- | --- |
| 不换行 | 标签文本节点 `Range.getClientRects().length === 1` |
| 不截断 | `scrollWidth <= clientWidth + 1` |
| 不越界 | 按钮矩形完全落在最近滚动祖先的裁切矩形内 |
| 目标尺寸 | 高 ≥32px（主操作 ≥40px），点击盒 ≥24×24 |
| 可达 | 键盘 Tab 可聚焦且焦点环可见 |

`en-US` 是宽度最坏情况，作为通过条件而非补充抽查。产物为按页面/宽度/语言维度的失败清单，
可直接作为分期实施的工作队列，并在 `docs/ui-visual-language.md` §7 的验收记录中引用。

---

## 6. 分期实施计划

每期一个独立提交，独立验收；未获授权不提交、不推送、不动 `main`。

| 期 | 范围 | 改动路径 | 风险 | 证据要求 |
| --- | --- | --- | --- | --- |
| **P0** 度量先行 | 落地 5.1 静态契约（先只报告不失败）与 5.2 实测检查，产出全量失败清单 | `web/src/styles/ButtonIntegrity.test.ts`、`scripts/` 检查入口、本文件 | 无产品代码改动 | 全量清单 + `make verify-change` |
| **P1** 基类不变量 | 4.1 的 `.button` / `.icon-button` 不变量；`line-height` 1→1.2 | `web/src/styles/main.css` | 中——影响全部 392 处实例；可能挤出新的容器溢出 | 15 页面 × 3 宽度 × 2 主题实测；P0 清单对比必须单调收敛 |
| **P2** 截图确认缺陷 | Docker 分区导航（`scroll-x`）、`.action-card` / `.action-grid`（`stack`）、删除两条死桥接规则 | `web/src/views/DockerView.vue`、`web/src/styles/desktop.css` | 低——范围明确，修复方式已存在于视图内 | 三张截图场景逐一复现并对比；420px 双主题 |
| **P3** 容器行为归一 | 按 P0 清单排序，逐视图落实 4.2/4.3；同步 4.4 的 `page-surface` 容器迁移并收缩桥接清单 | 38 个文件，按视图分批 | 中高——触及 372 个未桥接选择器；须逐视图验收 | 每视图独立验收；`position: fixed` 6 处逐处实测 |
| **P4** 文案宽度与字号 | 补 27 条 i18n 词条并修抽取管线（混排文本节点）；`.button--small` 12px→14px 迁移及基线清理 | `web/src/i18n/pages/*`、`web/scripts/check-page-i18n.mjs`、`main.css`、`visualContractBaseline.ts` | 高——先补翻译会立刻放大宽度压力 | 必须在 P1–P3 完成后执行；en-US 全量实测 |

排序理由：P4 会显著加宽标签，必须先具备 P1 的按钮不变量与 P3 的容器退让能力，否则补翻译即制造回归。
P2 插在 P3 之前是因为它对应用户已提供的实机证据，可最快形成可见收益并验证方法论。

关闭 P0 报告模式（契约转为失败）安排在 P3 完成、P4 开始前。

---

## 7. 明确不做

- 不做全仓机械 CSS 替换（`PROJECT_RULES.md` / §6.3：未经视觉验收的机械替换不允许）。
- 不用缩小字号、固定高度或纯截断换取"看起来紧凑"（§2.2 明令禁止）。
- 不提高 `MIN_WINDOW_WIDTH` 来回避问题——420px 是产品已承诺的可用宽度。
- 不新增 Claude 专属命令或平行门禁，全部接入 `make verify-change` / `verify-l2` / `verify-release`。
- 不在本期改动按钮的颜色、圆角、阴影等与完整性无关的视觉属性。

---

## 8. 待确认项

1. `collapse` 行为需要一个共享溢出菜单组件；现有 `web/src/components/common/` 无此组件，
   需确认是新建还是复用 `DockerView` 的上下文菜单实现。
2. `.button--small` 12px→14px 属产品级视觉变更，涉及 97 处；需确认是否与 P4 同批，
   或单独作为一次视觉验收发布。
3. 5.2 的实测检查需要可登录的本地预览环境；需确认复用
   `scripts/local-feature-preview.mjs` 还是 `mock-app-market-api.mjs` 路径。
