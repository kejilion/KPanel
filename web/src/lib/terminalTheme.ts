import type { ITheme } from '@xterm/xterm'

const terminalColorFallbacks = {
  background: '#0b1214',
  foreground: '#d8dddc',
  cursor: '#35cba6',
  selection: '#153a31',
  black: '#1d2426',
  red: '#d86f74',
  green: '#91b56d',
  yellow: '#d5ae62',
  blue: '#76a4c7',
  magenta: '#ad8bb8',
  cyan: '#72aaa7',
  white: '#c9cecd',
  brightBlack: '#687376',
  brightRed: '#e68589',
  brightGreen: '#a7c982',
  brightYellow: '#e3c27b',
  brightBlue: '#8bb9dc',
  brightMagenta: '#c19bcb',
  brightCyan: '#8cc2be',
  brightWhite: '#f0f2f1',
} as const

function terminalColor(style: CSSStyleDeclaration, name: string, fallback: string): string {
  return style.getPropertyValue(name).trim() || fallback
}

export function readTerminalTheme(element: Element): ITheme {
  const style = window.getComputedStyle(element)
  const background = terminalColor(style, '--terminal-shell-background', terminalColorFallbacks.background)

  return {
    background,
    foreground: terminalColor(style, '--terminal-shell-text', terminalColorFallbacks.foreground),
    cursor: terminalColor(style, '--brand', terminalColorFallbacks.cursor),
    cursorAccent: background,
    selectionBackground: terminalColor(style, '--brand-soft', terminalColorFallbacks.selection),
    black: terminalColor(style, '--terminal-ansi-black', terminalColorFallbacks.black),
    red: terminalColor(style, '--terminal-ansi-red', terminalColorFallbacks.red),
    green: terminalColor(style, '--terminal-ansi-green', terminalColorFallbacks.green),
    yellow: terminalColor(style, '--terminal-ansi-yellow', terminalColorFallbacks.yellow),
    blue: terminalColor(style, '--terminal-ansi-blue', terminalColorFallbacks.blue),
    magenta: terminalColor(style, '--terminal-ansi-magenta', terminalColorFallbacks.magenta),
    cyan: terminalColor(style, '--terminal-ansi-cyan', terminalColorFallbacks.cyan),
    white: terminalColor(style, '--terminal-ansi-white', terminalColorFallbacks.white),
    brightBlack: terminalColor(style, '--terminal-ansi-bright-black', terminalColorFallbacks.brightBlack),
    brightRed: terminalColor(style, '--terminal-ansi-bright-red', terminalColorFallbacks.brightRed),
    brightGreen: terminalColor(style, '--terminal-ansi-bright-green', terminalColorFallbacks.brightGreen),
    brightYellow: terminalColor(style, '--terminal-ansi-bright-yellow', terminalColorFallbacks.brightYellow),
    brightBlue: terminalColor(style, '--terminal-ansi-bright-blue', terminalColorFallbacks.brightBlue),
    brightMagenta: terminalColor(style, '--terminal-ansi-bright-magenta', terminalColorFallbacks.brightMagenta),
    brightCyan: terminalColor(style, '--terminal-ansi-bright-cyan', terminalColorFallbacks.brightCyan),
    brightWhite: terminalColor(style, '--terminal-ansi-bright-white', terminalColorFallbacks.brightWhite),
  }
}
