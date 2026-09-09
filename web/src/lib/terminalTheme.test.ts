// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { readTerminalTheme } from './terminalTheme'
import { contrastRatio, deriveThemeTokens, THEME_COLOR_PRESETS } from '../theme/colors'

describe('readTerminalTheme', () => {
  it('reads themed shell surfaces and keeps the ANSI palette independent', () => {
    const host = document.createElement('div')
    host.style.setProperty('--terminal-shell-background', '#071426')
    host.style.setProperty('--terminal-shell-text', '#eef5ff')
    host.style.setProperty('--brand', '#4d88ff')
    host.style.setProperty('--brand-soft', '#172c52')
    host.style.setProperty('--text', '#eef5ff')
    host.style.setProperty('--terminal-ansi-red', '#d86f74')
    document.body.append(host)

    const theme = readTerminalTheme(host)

    expect(theme).toMatchObject({
      background: '#071426',
      foreground: '#eef5ff',
      cursor: '#4d88ff',
      cursorAccent: '#071426',
      selectionBackground: '#172c52',
      selectionInactiveBackground: '#172c52',
      selectionForeground: '#eef5ff',
      red: '#d86f74',
      green: '#91b56d',
    })
    host.remove()
  })

  for (const mode of ['light', 'dark'] as const) {
    it.each(THEME_COLOR_PRESETS)(`keeps focused and inactive selections readable in ${mode} / $id`, (preset) => {
      const host = document.createElement('div')
      const tokens = deriveThemeTokens(preset.colors, mode)
      for (const [name, value] of Object.entries(tokens)) host.style.setProperty(name, value)
      // Light pages deliberately retain a dark shell with light ANSI text.
      host.style.setProperty('--terminal-shell-text', '#d8dddc')
      document.body.append(host)

      const theme = readTerminalTheme(host)

      expect(contrastRatio(theme.selectionForeground!, theme.selectionBackground!)).toBeGreaterThanOrEqual(4.5)
      expect(contrastRatio(theme.selectionForeground!, theme.selectionInactiveBackground!)).toBeGreaterThanOrEqual(4.5)
      expect(theme.foreground).toBe('#d8dddc')
      expect(theme.red).toBe('#d86f74')
      host.remove()
    })
  }

  it('keeps selection readable before theme tokens are available', () => {
    const host = document.createElement('div')
    document.body.append(host)
    const theme = readTerminalTheme(host)
    expect(contrastRatio(theme.selectionForeground!, theme.selectionBackground!)).toBeGreaterThanOrEqual(4.5)
    expect(theme.selectionInactiveBackground).toBe(theme.selectionBackground)
    host.remove()
  })
})
