import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const diagnosticsSource = readFileSync(new URL('./DiagnosticsView.vue', import.meta.url), 'utf8')
const desktopStyles = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')

describe('diagnostics workspace layout', () => {
  it('keeps duplicate refresh and fullscreen controls out of the terminal bar', () => {
    expect(diagnosticsSource).not.toContain('aria-label="刷新体检命令"')
    expect(diagnosticsSource).not.toContain('class="diagnostic-fullscreen-toggle"')
    expect(diagnosticsSource).not.toContain('diagnostic-fullscreen-open')
  })

  it('contains scroll chaining inside the diagnostic log', () => {
    expect(diagnosticsSource).toContain('@wheel="containLogWheel"')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-log\s*\{[^}]*overflow: auto;[^}]*overscroll-behavior: contain;/,
    )
  })

  it('keeps the command list compact with a per-command run action and category color', () => {
    expect(diagnosticsSource).not.toContain('class="diagnostic-tabs"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-group"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-row"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-run"')
    expect(diagnosticsSource).toContain('@click="requestCheck(check)"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-tested">已测</small>')
    expect(diagnosticsSource).toContain("job.status === 'succeeded' || job.status === 'failed'")
    expect(diagnosticsSource).not.toContain('{{ categoryName(check.category) }} · 约')
    expect(diagnosticsSource).toMatch(/\.diagnostic-command-row\.is-category-access,[^}]*--diagnostic-category:/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-command-row\.is-category-network,[^}]*--diagnostic-category:/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-command-row\.is-category-hardware,[^}]*--diagnostic-category:/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-command-row\.is-category-comprehensive,[^}]*--diagnostic-category:/)
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-command-group \+ \.diagnostic-command-group\s*\{[^}]*border-top: 1px dashed/,
    )
    expect(diagnosticsSource).not.toMatch(/font-size:\s*(?:10|11)px;/)
  })

  it('keeps native probes on the overview instead of presenting them as terminal commands', () => {
    expect(diagnosticsSource).toContain('const optionalChecks = computed')
    expect(diagnosticsSource).toContain('items: optionalChecks.value.filter')
    expect(diagnosticsSource).toContain('v-for="check in optionalChecks"')
    expect(diagnosticsSource).toContain("if (check.provider === 'native')")
    expect(diagnosticsSource).toContain("scoreCheck?.provider !== 'native'")
  })

  it('allows overview cards to shrink inside the phone-width result grid', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-overview > \*\s*\{[^}]*min-width:\s*0;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-section__header,[\s\S]*?\.diagnostic-report-section__title > div\s*\{[^}]*min-width:\s*0;/,
    )
  })

  it('lets classic mobile overview scroll with the document while preserving bounded-window scrolling', () => {
    expect(diagnosticsSource).toMatch(
      /@media \(max-width: 680px\)[\s\S]*?\.diagnostic-result\.is-overview\s*\{[^}]*overflow: visible;[^}]*overscroll-behavior: auto;[^}]*touch-action: pan-y;/,
    )
    expect(diagnosticsSource).toMatch(
      /:global\(\.desktop-window__body \.diagnostic-result\.is-overview\)\s*\{[^}]*overflow: auto;[^}]*overscroll-behavior: contain;[^}]*scrollbar-gutter: stable;/,
    )
  })

  it('collapses commands into a persistent icon rail', () => {
    expect(diagnosticsSource).toContain("'is-command-panel-collapsed': commandsCollapsed")
    expect(diagnosticsSource).toContain('aria-controls="diagnostic-command-selector"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-rail"')
    expect(diagnosticsSource).toContain('class="diagnostic-command-rail__item"')
    expect(diagnosticsSource).toContain(':title="checkNameLabel(check.name)"')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-workbench\.is-command-panel-collapsed\s*\{[^}]*grid-template-columns:\s*52px minmax\(0, 1fr\);/,
    )
  })

  it('uses an overlay drawer for command selection on mobile', () => {
    expect(diagnosticsSource).toContain("'is-command-drawer-open': mobileCommandsOpen")
    expect(diagnosticsSource).toContain('class="diagnostic-command-overlay"')
    expect(diagnosticsSource).toContain('aria-label="打开体检项目选择"')
    expect(diagnosticsSource).toContain('aria-label="关闭体检项目选择"')
    expect(diagnosticsSource).toContain('mobileCommandsOpen.value = false')
    expect(diagnosticsSource).toContain('background: color-mix(in srgb, var(--surface-subtle) 38%, var(--surface));')
    expect(diagnosticsSource).toMatch(
      /@media \(max-width: 680px\)[\s\S]*?\.diagnostic-command-panel\s*\{[^}]*position: absolute;[^}]*transform: translateX\(-105%\);/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-workbench\.is-command-drawer-open \.diagnostic-command-panel\s*\{[^}]*transform: translateX\(0\);/,
    )
  })

  it('keeps the mobile selector on the page theme instead of the terminal palette', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-mobile-selector\s*\{[^}]*border-bottom:\s*1px solid var\(--border\);[^}]*color:\s*var\(--text\);[^}]*background:\s*var\(--surface-subtle\);/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-mobile-selector button\s*\{[^}]*color:\s*inherit;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-mobile-selector small\s*\{[^}]*color:\s*var\(--muted\);/,
    )
    expect(diagnosticsSource).not.toMatch(
      /\.diagnostic-mobile-selector\s*\{[^}]*background:\s*var\(--terminal-shell-panel/,
    )
  })

  it('keeps bounded desktop windows in drawer mode instead of stacking the result below commands', () => {
    expect(diagnosticsSource).toContain('@container desktop-window (max-width: 820px)')
    expect(diagnosticsSource).toMatch(
      /@container desktop-window \(max-width: 820px\)[\s\S]*?\.diagnostic-workbench,[\s\S]*?grid-template-columns: minmax\(0, 1fr\) !important;[\s\S]*?height: 100% !important;/,
    )
    expect(diagnosticsSource).toContain("viewMode === 'terminal' && selectedCheck?.id === check.id")
  })

  it('presents a compact total score with performance and network report groups', () => {
    expect(diagnosticsSource).toContain('class="diagnostic-score-total"')
    expect(diagnosticsSource).toContain('class="diagnostic-report-section is-performance"')
    expect(diagnosticsSource).toContain('class="diagnostic-report-section is-network"')
    expect(diagnosticsSource).toContain("summaryValue('performance', 'cpu_model')")
    expect(diagnosticsSource).toContain("summaryValue('ip', 'public_ip')")
    expect(diagnosticsSource).not.toContain('class="diagnostic-score-route"')
    expect(diagnosticsSource).not.toContain('class="diagnostic-score-dimensions"')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-score-hero--simple\s*\{[^}]*grid-template-columns:\s*minmax\(150px, 170px\) minmax\(280px, 360px\);[^}]*justify-content:\s*center;/,
    )
    expect(diagnosticsSource).toContain('class="diagnostic-report-section__body"')
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-card\s*\{[^}]*min-height:\s*96px;/)
  })

  it('keeps scores at section level instead of repeating them on metric cards', () => {
    expect(diagnosticsSource).toContain('class="diagnostic-report-section__score"')
    expect(diagnosticsSource).not.toContain('class="diagnostic-report-card__score"')
  })

  it('keeps three metrics in one scan line and gives phone cards a readable stack', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--performance,[\s\S]*?\.diagnostic-report-card-grid--network\s*\{[^}]*grid-template-columns:\s*repeat\(3, minmax\(0, 1fr\)\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card-grid--performance,[\s\S]*?\.diagnostic-report-card-grid--network\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) auto;[^}]*min-height:\s*92px;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card > header\s*\{[^}]*grid-column:\s*1 \/ -1;/,
    )
  })

  it('uses a section rail on wide reports and switches identity data to one row per item on phones', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(min-width: 1040px\)[\s\S]*?\.diagnostic-report-section\s*\{[^}]*grid-template-columns:\s*minmax\(176px, \.2fr\) minmax\(0, \.8fr\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-identity\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-identity > div \+ div\s*\{[^}]*border-left:\s*0;[^}]*border-top:\s*1px solid var\(--border\);/,
    )
  })

  it('uses three equal network identity columns aligned with the three metric cards', () => {
    expect(diagnosticsSource).not.toContain('<span>出口线路</span>')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-identity\s*\{[^}]*grid-template-columns:\s*repeat\(3, minmax\(0, 1fr\)\);/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--network\s*\{[^}]*grid-template-columns:\s*repeat\(3, minmax\(0, 1fr\)\);/,
    )
  })

  it('makes performance and network report blocks visibly distinct', () => {
    expect(diagnosticsSource).toContain('--diagnostic-section-accent: var(--brand);')
    expect(diagnosticsSource).toContain('.diagnostic-report-section.is-performance {')
    expect(diagnosticsSource).toContain('--diagnostic-section-accent: var(--amber);')
    expect(diagnosticsSource).toContain('.diagnostic-report-section.is-network {')
    expect(diagnosticsSource).toContain('--diagnostic-section-accent: var(--primary);')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-section__header\s*\{[^}]*background: color-mix\(in srgb, var\(--surface-subtle\) 56%, var\(--surface\)\);[^}]*border-bottom: 1px solid var\(--border-strong\);/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid\s*\{[^}]*background: var\(--border-strong\);[^}]*border: 1px solid var\(--border-strong\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(min-width: 1040px\)[\s\S]*?\.diagnostic-report-section__header\s*\{[^}]*border-right: 1px solid var\(--border-strong\);[^}]*border-bottom: 0;/,
    )
  })

  it('gives wide network reports equal-height identity and metric layers', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(min-width: 521px\)[\s\S]*?\.diagnostic-report-section\.is-network \.diagnostic-report-section__body\s*\{[^}]*grid-template-rows:\s*repeat\(2, minmax\(120px, auto\)\);/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(min-width: 521px\)[\s\S]*?\.diagnostic-report-section\.is-network \.diagnostic-report-identity\s*\{[^}]*height:\s*100%;[^}]*margin-bottom:\s*0;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(min-width: 521px\)[\s\S]*?\.diagnostic-report-section\.is-network \.diagnostic-report-card-grid--network\s*\{[^}]*height:\s*100%;/,
    )
  })

  it('keeps phone network identity rows and metric cards on one common height', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-section\.is-network \.diagnostic-report-identity > div,[\s\S]*?\.diagnostic-report-section\.is-network \.diagnostic-report-card-grid--network > \.diagnostic-report-card\s*\{[^}]*min-height:\s*106px;/,
    )
  })

  it('gives performance single values the same data slot as disk pairs', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--performance \.diagnostic-report-card__value,[\s\S]*?\.diagnostic-report-card-grid--performance \.diagnostic-report-pair\s*\{[^}]*min-height:\s*41px;[^}]*margin-top:\s*11px;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--performance \.diagnostic-report-card__value\s*\{[^}]*display:\s*grid;[^}]*align-content:\s*end;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card-grid--performance > \.diagnostic-report-card\s*\{[^}]*min-height:\s*106px;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card-grid--performance \.diagnostic-report-card__value,[\s\S]*?\.diagnostic-report-card-grid--performance \.diagnostic-report-pair\s*\{[^}]*margin-top:\s*0;/,
    )
  })

  it('gives network single values the same data slot as bandwidth pairs', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--network \.diagnostic-report-card__value,[\s\S]*?\.diagnostic-report-card-grid--network \.diagnostic-report-pair,[\s\S]*?\.diagnostic-report-card-grid--network \.diagnostic-report-risk\s*\{[^}]*min-height:\s*41px;[^}]*margin-top:\s*11px;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card-grid--network \.diagnostic-report-card__value\s*\{[^}]*display:\s*grid;[^}]*align-content:\s*end;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card-grid--network \.diagnostic-report-card__value,[\s\S]*?\.diagnostic-report-card-grid--network \.diagnostic-report-pair,[\s\S]*?\.diagnostic-report-card-grid--network \.diagnostic-report-risk\s*\{[^}]*margin-top:\s*0;/,
    )
  })

  it('keeps latency and IP quality data on one line', () => {
    expect(diagnosticsSource.match(/class="diagnostic-report-card__data-row"/g)?.length).toBe(2)
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card__data-row\s*\{[^}]*display:\s*flex;[^}]*min-height:\s*41px;[^}]*align-items:\s*flex-end;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card__data-row > \.diagnostic-report-card__meta\s*\{[^}]*flex-wrap:\s*nowrap;[^}]*overflow:\s*hidden;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card__data-row > \.diagnostic-report-risk\s*\{[^}]*flex:\s*1 1 auto;[^}]*flex-wrap:\s*nowrap;[^}]*overflow:\s*hidden;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-card__data-row\s*\{[^}]*grid-column:\s*1 \/ -1;[^}]*margin-top:\s*0;/,
    )
  })

  it('merges IP quality details into one group and omits risk tags', () => {
    const cardStart = diagnosticsSource.indexOf('<strong>IP 质量</strong>')
    const cardEnd = diagnosticsSource.indexOf('</article>', cardStart)
    expect(cardStart).toBeGreaterThanOrEqual(0)
    expect(cardEnd).toBeGreaterThan(cardStart)
    const ipQualityCard = diagnosticsSource.slice(cardStart, cardEnd)
    expect(ipQualityCard.match(/class="diagnostic-report-card__data-row"/g)).toHaveLength(1)
    expect(ipQualityCard.match(/class="diagnostic-report-risk"/g)).toHaveLength(1)
    expect(ipQualityCard).not.toContain('diagnostic-report-card__meta')
    expect(ipQualityCard).not.toContain('risk_tag')
    expect(diagnosticsSource).not.toContain('reportIPRiskTags')
  })

  it('uses the same icon box specification for identity data and metric cards', () => {
    expect(diagnosticsSource).toContain('<Globe2 :size="17" />')
    expect(diagnosticsSource).toContain('<Network :size="17" />')
    expect(diagnosticsSource).toContain('<MapPin :size="17" />')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-card__heading > span,[\s\S]*?\.diagnostic-report-identity__heading > span\s*\{[^}]*width:\s*30px;[^}]*height:\s*30px;[^}]*border-radius:\s*9px;/,
    )
  })

  it('keeps score and copy side by side on narrow result surfaces', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 560px\)[\s\S]*?\.diagnostic-score-hero--simple\s*\{[\s\S]*?grid-template-columns:\s*minmax\(170px, \.42fr\) minmax\(0, \.58fr\);[\s\S]*?justify-content:\s*stretch;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-score-total\s*\{[^}]*padding:\s*14px 0 14px 50px;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 560px\)[\s\S]*?\.diagnostic-score-total\s*\{[\s\S]*?padding:\s*14px 0 14px 42px;/,
    )
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 420px\)[\s\S]*?\.diagnostic-score-hero--simple\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\);/,
    )
  })

  it('keeps report metrics visually flat and gives IP risk levels semantic colors', () => {
    expect(diagnosticsSource).not.toContain('class="diagnostic-report-model"')
    expect(diagnosticsSource).toContain('<strong>带宽</strong>')
    expect(diagnosticsSource).not.toContain('<strong>测速</strong>')
    expect(diagnosticsSource).not.toContain('<strong>上传 / 下载</strong>')
    expect(diagnosticsSource).toContain('class="diagnostic-report-pair diagnostic-report-pair--speed"')
    expect(diagnosticsSource).toContain('`is-${reportIPRiskTone()}`')
    expect(diagnosticsSource).toContain("'is-isp': hasIPISP()")
    expect(diagnosticsSource).toContain("ip_type: 'IP 类型'")
    expect(diagnosticsSource).toContain('reportIPType()')
    expect(diagnosticsSource).toContain('reportIPQualityScore()')
    expect(diagnosticsSource).toContain('riskSafety * 0.8')
    expect(diagnosticsSource).toContain('reportIPTypeScore() * 0.1')
    expect(diagnosticsSource).toContain('reportIPProxyScore() * 0.05')
    expect(diagnosticsSource).toContain('reportIPMetadataScore() * 0.05')
    expect(diagnosticsSource).toContain('reportIPAttributeDetails()')
    expect(diagnosticsSource).toContain("'is-native-ip': detail.isNativeIP")
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-identity strong\.is-isp\s*\{[^}]*color:\s*var\(--brand-strong\);/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-risk \.is-native-ip\s*\{[^}]*color:\s*var\(--brand-strong\);/,
    )
    expect(diagnosticsSource).toContain('.diagnostic-report-risk__level.is-low')
    expect(diagnosticsSource).toContain('.diagnostic-report-risk__level.is-medium')
    expect(diagnosticsSource).toContain('.diagnostic-report-risk__level.is-high')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-report-identity strong\s*\{[^}]*font-size:\s*16px;[^}]*font-weight:\s*700;/,
    )
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-card__value\s*\{[^}]*font-size:\s*16px;/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-pair strong\s*\{[^}]*font-size:\s*16px;/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-pair span\s*\{[^}]*font-size: 12px;/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-card__meta\s*\{[^}]*font-size: 13px;/)
    expect(diagnosticsSource).toMatch(/\.diagnostic-report-note\s*\{[^}]*font-size: 13px;/)
  })

  it('uses one phone content guide for network identity and metric cards', () => {
    expect(diagnosticsSource).toMatch(
      /@container diagnostic-result \(max-width: 520px\)[\s\S]*?\.diagnostic-report-identity > div,[\s\S]*?\.diagnostic-report-identity > div:first-child\s*\{[^}]*padding-right:\s*12px;[^}]*padding-left:\s*12px;/,
    )
  })

  it('states that native network scores measure the server rather than the browser link', () => {
    expect(diagnosticsSource).toContain('原生探针由服务器本机执行')
    expect(diagnosticsSource).toContain('服务器至探测节点')
    expect(diagnosticsSource).toContain('服务器公网上下行带宽')
    expect(diagnosticsSource).toContain("summaryMetricLabel('download')")
    expect(diagnosticsSource).toContain("summaryMetricLabel('upload')")
    expect(diagnosticsSource).toContain('不包含当前浏览器到服务器的访问质量')
  })

  it('reserves a dedicated home row above the scrollable command list', () => {
    expect(diagnosticsSource).toContain('class="diagnostic-command-panel__toolbar"')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-command-panel\s*\{[^}]*grid-template-rows:\s*auto auto minmax\(0, 1fr\);/,
    )
    expect(diagnosticsSource).toContain('.diagnostic-command-list {')
    expect(diagnosticsSource).toContain('padding-top: 8px;')
  })

  it('uses darker category accents in light mode and brighter accents in dark mode', () => {
    expect(diagnosticsSource).toContain('--diagnostic-category: #087a72;')
    expect(diagnosticsSource).toContain('--diagnostic-category: #2563c4;')
    expect(diagnosticsSource).toContain('--diagnostic-category: #965900;')
    expect(diagnosticsSource).toContain('--diagnostic-category: #7546c8;')
    expect(diagnosticsSource).toContain(":global(:root[data-theme='dark'] .diagnostic-command-group.is-category-access)")
    expect(diagnosticsSource).toContain('--diagnostic-category: #4ecdc4;')
  })

  it('binds first-screen metrics to backend summaries and keeps the terminal as the raw-output fallback', () => {
    expect(diagnosticsSource).toContain('const scoreSummaryMetricCount = computed')
    expect(diagnosticsSource).toContain('summaryMetrics(dimension)')
    expect(diagnosticsSource).toContain('summaryReportURL')
    expect(diagnosticsSource).toContain('真实结果已汇总')
    expect(diagnosticsSource).toContain('未识别项目仍保留在终端中。')
    expect(diagnosticsSource).toContain('openSummaryTerminal')
  })

  it('keeps terminal output flexible while reserving the bottom status row', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-interactive-terminal\s*\{[^}]*min-height:\s*0;[^}]*overflow:\s*hidden;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-result footer\s*\{[^}]*flex:\s*0 0 auto;[^}]*min-height:\s*42px;/,
    )
    expect(desktopStyles).toContain('.desktop-window__body > .diagnostics-page > .diagnostic-workbench')
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.diagnostics-page > \.diagnostic-workbench\s*\{[^}]*height:\s*auto;[^}]*min-height:\s*0;/,
    )
  })

  it('removes the history block and fills the desktop window with the workbench', () => {
    expect(diagnosticsSource).not.toContain('class="diagnostic-history"')
    expect(diagnosticsSource).not.toContain('class="diagnostic-history__list"')
    expect(desktopStyles).toMatch(
      /\.desktop-window__body > \.diagnostics-page\s*\{[^}]*grid-template-rows:\s*minmax\(0, 1fr\);[^}]*overflow:\s*hidden;[^}]*align-content:\s*stretch;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body:has\(> \.terminal-page\),[\s\S]*?\.desktop-window__body:has\(> \.diagnostics-page\)\s*\{[^}]*overflow:\s*hidden;/,
    )
    expect(desktopStyles).toMatch(
      /\.desktop-window__body \.diagnostic-workbench\s*\{[^}]*height:\s*auto !important;[^}]*min-height:\s*0 !important;/,
    )
  })
})
