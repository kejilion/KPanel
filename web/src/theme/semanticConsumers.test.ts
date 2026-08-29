import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const sources = {
  main: readFileSync(new URL('../styles/main.css', import.meta.url), 'utf8'),
  themes: readFileSync(new URL('../styles/themes.css', import.meta.url), 'utf8'),
  desktop: readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8'),
  fileShare: readFileSync(new URL('../views/FileShareView.vue', import.meta.url), 'utf8'),
  clusterShare: readFileSync(new URL('../views/ClusterShareView.vue', import.meta.url), 'utf8'),
  files: readFileSync(new URL('../views/FilesView.vue', import.meta.url), 'utf8'),
  hostTerminal: readFileSync(new URL('../components/terminal/HostTerminal.vue', import.meta.url), 'utf8'),
  interactiveTerminal: readFileSync(new URL('../components/apps/AppInteractiveTerminal.vue', import.meta.url), 'utf8'),
  shareManager: readFileSync(new URL('../components/files/FileShareManagerDialog.vue', import.meta.url), 'utf8'),
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function expectRule(source: string, selector: string, declarations: Record<string, RegExp>): void {
  const match = source.match(new RegExp(`(?:^|\\n\\s*|,\\s*)${escapeRegExp(selector)}\\s*\\{([^}]*)\\}`, 'm'))
  expect(match, `missing ${selector}`).not.toBeNull()
  const actual = new Map<string, string>()
  for (const declaration of match?.[1]?.matchAll(/([\w-]+)\s*:\s*([^;]+);/g) ?? []) {
    actual.set(declaration[1] ?? '', declaration[2]?.trim() ?? '')
  }
  for (const [property, value] of Object.entries(declarations)) {
    expect(actual.get(property), `${selector} has invalid ${property}`).toMatch(value)
  }
}

describe('semantic action consumers', () => {
  it('keeps shared filled actions on readable semantic pairs', () => {
    expectRule(sources.main, '.button--primary', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
    expectRule(sources.main, '.button--danger', { color: /^var\(--on-danger\)$/, background: /^var\(--danger-action\)$/ })
    expectRule(sources.files, '.clipboard-bar button:first-of-type', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
    expectRule(sources.shareManager, '.file-share-manager__stop:hover:not(:disabled)', { color: /^var\(--on-danger\)$/, background: /^var\(--danger-action\)$/ })
  })

  it('keeps public share and terminal actions on the same brand pair', () => {
    expectRule(sources.fileShare, '.file-share-brand > span', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
    expectRule(sources.fileShare, '.file-share-download', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
    expectRule(sources.clusterShare, '.share-brand > span', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
    expectRule(sources.hostTerminal, '.host-terminal__composer button', { color: /^var\(--on-brand,\s*#05251c\)$/, background: /^var\(--brand-action,\s*#35cba6\)$/ })
    expectRule(sources.interactiveTerminal, '.interactive-terminal__composer button', { color: /^var\(--on-brand,\s*#05251c\)$/, background: /^var\(--terminal-accent\)$/ })
  })

  it('keeps ordinary hover feedback on the shared theme interaction layer', () => {
    expectRule(sources.main, ".k-context-menu > [role='menuitem']:hover:not(:disabled)", { background: /^var\(--interaction-hover\)$/ })
    expectRule(sources.main, '.button--secondary:hover:not(:disabled)', { background: /^var\(--interaction-hover-surface\)$/ })
    expectRule(sources.main, '.icon-button:hover:not(:disabled)', { background: /^var\(--interaction-hover-surface\)$/ })
    expectRule(sources.main, '.sidebar__link:hover', { background: /^var\(--sidebar-hover\)$/ })
    expectRule(sources.main, '.data-table tbody tr:hover', { background: /^var\(--interaction-hover\)$/ })
    expectRule(sources.desktop, '.desktop-window__action:hover', { background: /^var\(--interaction-hover\)$/ })
    expect(sources.files).toMatch(/\.file-grid-card:hover,\s*\.file-grid-card:focus-visible\s*\{[^}]*background:\s*var\(--interaction-hover\);/)
  })

  it('keeps every file preview branch on one theme-derived workbench palette', () => {
    for (const token of [
      '--file-preview-background',
      '--file-preview-panel',
      '--file-preview-panel-raised',
      '--file-preview-text',
      '--file-preview-muted',
      '--file-preview-border',
      '--file-preview-accent',
      '--file-preview-selection',
      '--file-preview-glow',
    ]) {
      expect(sources.themes).toContain(`${token}:`)
    }
    for (const consumer of ['var(--file-preview-background)', 'var(--file-preview-panel)', 'var(--file-preview-text)', 'var(--file-preview-muted)', 'var(--file-preview-accent)']) {
      expect(sources.files).toContain(consumer)
    }
    expect(sources.files).not.toContain('rgb(53 203 166 / 15%)')
    expect(sources.files).not.toContain('linear-gradient(180deg, #111c1d')
  })

  it('keeps the authentication shell on the runtime theme contract', () => {
    expectRule(sources.main, '.auth-layout__brand', {
      color: /^var\(--sidebar-text\)$/,
      background: /^var\(--auth-brand-background\)$/,
    })
    expectRule(sources.main, '.auth-layout__brand .brand__text small', { color: /^var\(--sidebar-accent\)$/ })
    expectRule(sources.main, '.auth-layout__message > p', { color: /^var\(--sidebar-muted\)$/ })
    expectRule(sources.main, '.auth-layout__message li', { color: /^var\(--sidebar-text\)$/ })
    expectRule(sources.main, '.auth-layout__message li svg', { color: /^var\(--sidebar-accent\)$/ })
    expectRule(sources.main, '.auth-layout__brand .eyebrow', { color: /^var\(--sidebar-accent\)$/ })
    expectRule(sources.main, '.auth-layout__footnote', { color: /^var\(--sidebar-muted\)$/ })
    expectRule(sources.main, '.auth-layout__form', { background: /^var\(--auth-form-background\)$/ })

    const authStyles = sources.main.match(/\/\* Authentication \*\/([\s\S]*?)\.form-stack\s*\{/)?.[1] ?? ''
    expect(authStyles).not.toMatch(/#[\da-f]{3,8}/i)
    expect(authStyles).toContain('var(--auth-brand-background)')
    expect(authStyles).toContain('var(--brand-soft)')
  })

  it('keeps desktop transfer failures on the danger pair', () => {
    expectRule(sources.desktop, '.desktop-transfer--error .desktop-transfer__glyph', { color: /^var\(--on-danger\)$/, background: /^var\(--danger-action\)$/ })
    expectRule(sources.desktop, '.desktop__icon--selected .desktop__icon-label', { color: /^var\(--on-brand\)$/, background: /^var\(--brand-action\)$/ })
  })

  it('keeps desktop progress, status and selection details on semantic theme colors', () => {
    expect(sources.desktop).toMatch(/\.desktop-monitor__track span\s*\{[^}]*background:\s*linear-gradient\(90deg, var\(--brand\), var\(--brand-strong\)\);/)
    expect(sources.desktop).toMatch(/\.desktop__taskbar-agent-status > i\s*\{[^}]*background:\s*var\(--success\);/)
    expect(sources.desktop).toMatch(/\.desktop__taskbar-agent-status--offline > i,[\s\S]*?background:\s*var\(--danger\);/)
    expect(sources.desktop).toMatch(/\.desktop__taskbar-agent-status--read-only > i\s*\{[^}]*background:\s*var\(--amber\);/)
    for (const legacyColor of ['#2dd4bf', '#5eead4', '#34d399', '#fb7185', '#fbbf24', '#17312c']) {
      expect(sources.desktop).not.toContain(legacyColor)
    }
  })
})
