// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import { readTerminalTheme } from './terminalTheme'

describe('readTerminalTheme', () => {
  it('reads themed shell surfaces and keeps the ANSI palette independent', () => {
    const host = document.createElement('div')
    host.style.setProperty('--terminal-shell-background', '#071426')
    host.style.setProperty('--terminal-shell-text', '#eef5ff')
    host.style.setProperty('--brand', '#4d88ff')
    host.style.setProperty('--brand-soft', '#172c52')
    host.style.setProperty('--terminal-ansi-red', '#d86f74')
    document.body.append(host)

    const theme = readTerminalTheme(host)

    expect(theme).toMatchObject({
      background: '#071426',
      foreground: '#eef5ff',
      cursor: '#4d88ff',
      cursorAccent: '#071426',
      selectionBackground: '#172c52',
      red: '#d86f74',
      green: '#91b56d',
    })
    host.remove()
  })
})
