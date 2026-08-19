import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const monitoringSource = readFileSync(new URL('./MonitoringView.vue', import.meta.url), 'utf8')

describe('monitoring container comparison layout', () => {
  it('uses the injected router history for zoom navigation in every shell', () => {
    expect(monitoringSource).toContain('router.options.history.state.monitoringZoomDepth')
    expect(monitoringSource).toContain('router.go(-depth)')
    expect(monitoringSource).not.toContain('window.history')
  })

  it('hides the idle zoom instruction strip to keep the history view compact', () => {
    expect(monitoringSource).toContain(
      '<div v-if="activeWindow || updating" class="monitoring-zoom-strip"',
    )
    expect(monitoringSource).not.toContain('在任意趋势图上横向拖动，即可同步框选放大全部图表。')
  })

  it('links selected container rows to chart highlighting for pointer and keyboard users', () => {
    expect(monitoringSource).toContain(
      '@mouseenter="containerSelected(container.containerId) && (highlightedContainerId = container.containerId)"',
    )
    expect(monitoringSource).toContain(
      '@focusin="containerSelected(container.containerId) && (highlightedContainerId = container.containerId)"',
    )
    expect(monitoringSource).toContain('@mouseleave="highlightedContainerId = \'\'"')
    expect(monitoringSource).toContain('@focusout="highlightedContainerId = \'\'"')
  })

  it('exposes the compact selected-container strip as a keyboard-focusable list', () => {
    expect(monitoringSource).toContain('class="selected-container-strip" role="list"')
    expect(monitoringSource).toContain('role="listitem"')
    expect(monitoringSource).toContain(':aria-label="`聚焦 ${containerSeriesName(container)} 曲线`"')
    expect(monitoringSource).toContain('tabindex="0"')
  })

  it('keeps comparison cards compact and horizontally scrollable', () => {
    expect(monitoringSource).toMatch(
      /\.selected-container-strip\s*\{[^}]*display:\s*flex;[^}]*overflow-x:\s*auto;/,
    )
    expect(monitoringSource).toMatch(
      /\.selected-container-card\s*\{[^}]*min-width:\s*120px;[^}]*min-height:\s*34px;/,
    )
    expect(monitoringSource).toMatch(
      /\.container-row\s*\{[^}]*display:\s*grid;[^}]*grid-template-columns:\s*16px 6px minmax\(0, 1fr\) minmax\(64px, auto\);/,
    )
  })
})
