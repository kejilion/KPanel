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
const environmentSource = readFileSync(
  new URL('../../views/EnvironmentView.vue', import.meta.url),
  'utf8',
)
const appScriptSource = readFileSync(
  new URL('../../views/AppScriptView.vue', import.meta.url),
  'utf8',
)

describe('interactive task terminal layout', () => {
  it('reserves an explicit row for the input composer', () => {
    expect(terminalSource).toMatch(
      /\.interactive-terminal\s*\{[\s\S]*?display:\s*grid;[\s\S]*?grid-template-rows:\s*auto minmax\(0, 1fr\) auto;/,
    )
  })

  it('keeps the outer screen edge-to-edge and uses fit-aware xterm content padding', () => {
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen\s*\{[^}]*padding:\s*0;/,
    )
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm\)\s*\{[^}]*box-sizing:\s*border-box;[^}]*height:\s*100%;[^}]*padding:\s*6px 8px 4px;[^}]*touch-action:\s*none;/,
    )
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm-viewport\)\s*\{[^}]*background:\s*var\(--terminal-background\);/,
    )
  })

  it('lets the dedicated desktop shell shrink while keeping the composer visible', () => {
    expect(appScriptSource).toMatch(
      /\.app-script-page__terminal\s*\{[^}]*height:\s*100%;[^}]*min-height:\s*0;[^}]*flex:\s*1 1 0;/,
    )
    expect(appScriptSource).toMatch(
      /\.app-script-page__terminal :deep\(\.interactive-terminal__screen\)\s*\{[^}]*height:\s*auto;[^}]*min-height:\s*0;/,
    )
  })

  it('keeps the diagnostics override aligned with the shared three-row layout', () => {
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-interactive-terminal\s*\{[^}]*grid-template-rows:\s*auto minmax\(0, 1fr\) auto;[^}]*flex:\s*1 1 0;[^}]*min-height:\s*0;/,
    )
    expect(diagnosticsSource).toMatch(
      /\.diagnostic-interactive-terminal :deep\(\.interactive-terminal__screen\)\s*\{[^}]*height:\s*auto;[^}]*min-height:\s*0;/,
    )
  })

  it('contains wheel scrolling inside the xterm viewport', () => {
    expect(terminalSource).toContain('@wheel="containTerminalWheel"')
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm-viewport\)\s*\{[^}]*overflow-y: scroll !important;[^}]*overscroll-behavior: contain;/,
    )
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm-scrollable-element\)\s*\{[^}]*overscroll-behavior: contain;/,
    )
    expect(terminalSource).toContain('@touchmove="terminalTouchScroll.move"')
    expect(terminalSource).toMatch(
      /\.interactive-terminal__screen :deep\(\.xterm\)\s*\{[^}]*touch-action: none;/,
    )
  })

  it('focuses the shell when input opens and leaves the composer user-activated', () => {
    expect(terminalSource).toContain('if (open) focusTerminalWhenInputOpens()')
    expect(terminalSource).toContain('if (terminalInputOpen.value) window.requestAnimationFrame(focusTerminal)')
    expect(terminalSource).toContain('@click="terminalInputOpen && focusTerminal()"')
    expect(terminalSource).not.toContain('composerInput')
  })

  it('keeps diagnostic input manual without preset choices', () => {
    expect(terminalSource).not.toContain('diagnosticQuickInputs')
    expect(terminalSource).not.toContain('interactive-terminal__quick-actions')
    expect(terminalSource).not.toContain('sendQuickInput')
    expect(terminalSource).toContain('class="interactive-terminal__input-area"')
  })

  it('localizes the shell chrome while leaving command output in xterm', () => {
    expect(terminalSource).toContain("t('terminal.task.title'")
    expect(terminalSource).toContain("t('terminal.task.inputLabel')")
    expect(terminalSource).toContain("t('terminal.task.inputPlaceholder')")
    expect(terminalSource).toContain("t('terminal.send')")
    expect(terminalSource).not.toContain("'域名和固定参数已由面板传入")
    expect(terminalSource).not.toContain("'保留第三方脚本原生颜色")
  })

  it('shares the terminal clipboard behavior with host terminals', () => {
    expect(terminalSource).toContain('@contextmenu="clipboardMenu?.open($event)"')
    expect(terminalSource).toContain('@paste.capture="clipboardMenu?.handlePaste($event)"')
    expect(terminalSource).toContain('terminal.attachCustomKeyEventHandler')
  })

  it('uses the shared top and fullscreen controls in every interactive task terminal', () => {
    expect(terminalSource).toContain('<TerminalToolbar')
    expect(terminalSource).toContain('@scroll-top="scrollToTop"')
    expect(terminalSource).toContain('@toggle-fullscreen="toggleFullscreen"')
    expect(terminalSource).toMatch(
      /\.interactive-terminal\.is-fullscreen\s*\{[^}]*position: fixed;[^}]*inset: 0;[^}]*height: 100dvh;/,
    )
    expect(diagnosticsSource).toContain('v-if="!activeJob?.interactive"')
    expect(environmentSource).not.toContain('allow-fullscreen')
  })
})
