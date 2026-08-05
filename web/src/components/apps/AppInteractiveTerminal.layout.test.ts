import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const terminalSource = readFileSync(
  new URL('./AppInteractiveTerminal.vue', import.meta.url),
  'utf8',
)
const diagnosticsSource = readFileSync(
  new URL('../../views/DiagnosticsView.vue', import.meta.url),
  'utf8',
)

describe('interactive task terminal layout', () => {
  it('reserves an explicit row for the input composer', () => {
    expect(terminalSource).toMatch(
      /\.interactive-terminal\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-rows:\s*auto minmax\(0, 1fr\) auto;/,
    )
  })

  it('keeps the diagnostics override aligned with the shared three-row layout', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-interactive-terminal\s*\{[^}]*grid-template-rows:\s*auto minmax\(0, 1fr\) auto;/,
    )
  })

  it('contains wheel scrolling inside the xterm viewport', () => {
    expect(terminalSource).toContain('@wheel="containTerminalWheel"')
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm-viewport\)\s*\{[^}]*overflow-y: scroll !important;[^}]*overscroll-behavior: contain;/,
    )
  })
})
