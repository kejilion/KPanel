import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const terminalSource = readFileSync(
  new URL('./apps/AppInteractiveTerminal.vue', import.meta.url),
  'utf8',
)
const editorSource = readFileSync(
  new URL('./files/CodeEditor.vue', import.meta.url),
  'utf8',
)
const diagnosticsSource = readFileSync(
  new URL('../views/DiagnosticsView.vue', import.meta.url),
  'utf8',
)
const terminalViewSource = readFileSync(
  new URL('../views/TerminalView.vue', import.meta.url),
  'utf8',
)
const hostTerminalSource = readFileSync(
  new URL('./terminal/HostTerminal.vue', import.meta.url),
  'utf8',
)
const filesSource = readFileSync(
  new URL('../views/FilesView.vue', import.meta.url),
  'utf8',
)
const dockerSource = readFileSync(
  new URL('../views/DockerView.vue', import.meta.url),
  'utf8',
)
const appsSource = readFileSync(
  new URL('../views/AppsView.vue', import.meta.url),
  'utf8',
)
const globalThemeSource = readFileSync(
  new URL('../styles/main.css', import.meta.url),
  'utf8',
)
const terminalThemeSource = readFileSync(
  new URL('../lib/terminalTheme.ts', import.meta.url),
  'utf8',
)
const semanticThemeSource = readFileSync(
  new URL('../styles/themes.css', import.meta.url),
  'utf8',
)

describe('terminal and editor workspace theme', () => {
  it('themes interactive terminal surfaces while keeping the ANSI palette independent', () => {
    expect(globalThemeSource).toContain('--terminal-shell-background: #0b1214')
    expect(globalThemeSource).toContain('--terminal-shell-radius: 12px')
    expect(globalThemeSource).toMatch(/:root\[data-theme='dark'\] \.terminal-theme-scope\s*\{[^}]*--terminal-shell-background: var\(--bg\);[^}]*--terminal-shell-panel: var\(--surface\);/)
    expect(terminalSource).toContain('--terminal-background: var(--terminal-shell-background, #0b1214)')
    expect(terminalSource).toContain('--terminal-accent: var(--brand, #35cba6)')
    expect(terminalThemeSource).toContain("terminalColor(style, '--brand'")
    expect(terminalThemeSource).toContain("terminalColor(style, '--terminal-ansi-green'")
    expect(terminalThemeSource).toContain("terminalColor(style, '--terminal-ansi-cyan'")
    for (const source of [terminalSource, hostTerminalSource]) {
      expect(source).toContain('terminal-theme-scope')
      expect(source).toContain('terminal.options.theme = readTerminalTheme(host.value)')
    }
    expect(terminalSource).not.toContain("green: '#35cba6'")
    expect(terminalSource).not.toContain("cyan: '#5adaba'")
    expect(terminalSource).not.toContain('#6d5dfc')
    expect(terminalSource).not.toContain('#8b7cff')
  })

  it('uses the same terminal surface before and after an interactive diagnostic starts', () => {
    expect(diagnosticsSource).toContain('background: var(--terminal-shell-background, #0b1214)')
    expect(diagnosticsSource).toContain('background: var(--terminal-shell-panel, #111a1d)')
    expect(terminalViewSource).toContain('background:var(--terminal-shell-background,#0b1214)')
    expect(hostTerminalSource).toContain('background:var(--terminal-shell-background,#0b1214)')
    expect(terminalSource).toContain('--terminal-background: var(--terminal-shell-background, #0b1214)')
  })

  it('uses one classic-mode height for terminal and diagnostics workspaces', () => {
    expect(globalThemeSource).toContain('--terminal-workspace-height: clamp(620px, calc(100dvh - 190px), 760px)')
    expect(globalThemeSource).toContain('--terminal-workspace-min-height: 620px')
    expect(globalThemeSource).toContain('--terminal-workspace-radius: var(--radius-lg)')
    expect(terminalViewSource).toContain('height:var(--terminal-workspace-height)')
    expect(terminalViewSource).toContain('min-height:var(--terminal-workspace-min-height)')
    expect(terminalViewSource).toContain('border-radius:var(--terminal-workspace-radius)')
    expect(diagnosticsSource).toContain('height: var(--terminal-workspace-height)')
    expect(diagnosticsSource).toContain('min-height: var(--terminal-workspace-min-height)')
    expect(diagnosticsSource).toContain('border-radius: var(--terminal-workspace-radius)')
  })

  it('keeps the file editor on the preview workbench while using KPanel semantic tokens', () => {
    expect(editorSource).toContain('--code-background: var(--file-preview-background, var(--terminal-shell-background, #0b1214))')
    expect(editorSource).toContain('--code-caret: var(--file-preview-accent, var(--brand, #35cba6))')
    expect(editorSource).toContain('--code-active-line: var(--file-preview-active-line, rgb(53 203 166 / 8%))')
    expect(editorSource).toContain('--code-keyword: var(--violet)')
    expect(editorSource).toContain('color: var(--danger, #ef7a7a)')
    expect(editorSource).not.toContain('#409be8')
    expect(editorSource).not.toContain('#31415b')
  })

  it('gives file previews a derived palette while terminal and log workspaces keep theirs', () => {
    for (const token of [
      '--file-preview-background',
      '--file-preview-panel',
      '--file-preview-text',
      '--file-preview-border',
      '--file-preview-accent',
    ]) {
      expect(semanticThemeSource).toContain(`${token}:`)
    }
    expect(filesSource).toContain('background: var(--file-preview-background);')
    expect(filesSource).toContain('color: var(--file-preview-text);')
    expect(filesSource).not.toContain('var(--terminal-shell-background')
    for (const source of [dockerSource, appsSource]) {
      expect(source).toContain('var(--terminal-shell-background, #0b1214)')
      expect(source).toContain('var(--terminal-shell-text, #d8dddc)')
      expect(source).not.toContain('terminal-theme-scope')
    }
    expect(editorSource).not.toContain('terminal-theme-scope')
    expect(diagnosticsSource).not.toContain('terminal-theme-scope')
    expect(filesSource).not.toContain('background: #111a2c')
    expect(dockerSource).not.toContain('background: #0b1020')
    expect(appsSource).not.toContain('background: #111827')
  })

  it('uses one border radius and edge treatment across dark workspaces', () => {
    for (const source of [terminalSource, dockerSource, appsSource]) {
      expect(source).toContain('var(--terminal-shell-radius, 12px)')
      expect(source).toContain('var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%))')
    }
    expect(filesSource).toContain('border-radius: var(--radius, 12px);')
    expect(filesSource).toContain('box-shadow: var(--file-preview-shadow);')
    expect(globalThemeSource).toContain('border-radius: var(--terminal-shell-radius)')
    expect(globalThemeSource).toContain('box-shadow: var(--terminal-shell-shadow)')
  })
})
