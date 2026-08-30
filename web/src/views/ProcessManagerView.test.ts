import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const source = readFileSync(new URL('./ProcessManagerView.vue', import.meta.url), 'utf8')

describe('ProcessManagerView performance contract', () => {
  it('uses the standard page container for consistent desktop window spacing', () => {
    expect(source).toContain('<div class="page process-page">')
  })

  it('keeps collection bounded and uses completion-based polling', () => {
    expect(source).toContain('const processLimit = 200')
    expect(source).toContain('const refreshIntervalMilliseconds = 2_000')
    expect(source).toContain('window.setTimeout(() => void load(true), refreshIntervalMilliseconds)')
    expect(source).not.toContain('setInterval(')
  })

  it('stops work when hidden and guards termination with process identity', () => {
    expect(source).toContain("document.visibilityState === 'visible'")
    expect(source).toContain('desktopWindowActive.value')
    expect(source).toContain('controller?.abort()')
    expect(source).toContain('startTimeTicks: process.startTimeTicks')
    expect(source).toContain("signal: pendingSignal.value")
  })

  it('renders relative CPU and memory heatmap cells with the theme color', () => {
    expect(source).toContain('const cpuPeak = computed(() => items.value.reduce')
    expect(source).toContain('const memoryPeak = computed(() => items.value.reduce')
    expect(source).toContain("'--process-metric-strength': metricHeatmapStrength(process.cpuPercent, cpuPeak)")
    expect(source).toContain("'--process-metric-strength': metricHeatmapStrength(process.memoryBytes, memoryPeak)")
    expect(source).toContain('background-color: color-mix(in srgb, var(--brand) var(--process-metric-strength), var(--surface))')
    expect(source).not.toContain('percentWidth(process.cpuPercent) } /></i>')
  })
})
