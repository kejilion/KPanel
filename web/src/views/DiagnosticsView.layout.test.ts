import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const diagnosticsSource = readFileSync(new URL('./DiagnosticsView.vue', import.meta.url), 'utf8')

describe('diagnostics workspace layout', () => {
  it('matches the terminal workspace fullscreen behavior', () => {
    expect(diagnosticsSource).toContain(":class=\"{ 'is-fullscreen': fullscreen }\"")
    expect(diagnosticsSource).toContain("t('common.enterFullscreen')")
    expect(diagnosticsSource).toContain("t('common.exitFullscreen')")
    expect(diagnosticsSource).toContain("event.key === 'Escape'")
    expect(diagnosticsSource).toContain("classList.toggle('diagnostic-fullscreen-open', enabled)")
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-workbench\.is-fullscreen\s*\{[^}]*position: fixed;[^}]*inset: 0;[^}]*height: 100dvh;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-workbench\.is-fullscreen \.diagnostic-command-panel\s*\{[^}]*display: none;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-workbench\.is-fullscreen \.diagnostic-log,[\s\S]*?min-height: 0;/,
    )
  })

  it('contains scroll chaining inside the diagnostic log', () => {
    expect(diagnosticsSource).toContain('@wheel="containLogWheel"')
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-log\s*\{[^}]*overflow: auto;[^}]*overscroll-behavior: contain;/,
    )
  })
})
