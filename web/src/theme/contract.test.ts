import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { THEME_TOKEN_NAMES } from './colors'

const themeSource = readFileSync(new URL('../styles/themes.css', import.meta.url), 'utf8')
  .replace(/\/\*[\s\S]*?\*\//g, '')
const mainSource = readFileSync(new URL('../styles/main.css', import.meta.url), 'utf8')
const desktopSource = readFileSync(new URL('../styles/desktop.css', import.meta.url), 'utf8')
  + readFileSync(new URL('../styles/desktopWallpaper.css', import.meta.url), 'utf8')

const REQUIRED_DEFAULT_TOKENS = [
  ...THEME_TOKEN_NAMES,
  '--radius-sm',
  '--radius',
  '--radius-lg',
] as const

interface ThemeRule {
  selector: ':root' | ":root[data-theme='dark']"
  body: string
}

function themeRules(): ThemeRule[] {
  return Array.from(
    themeSource.matchAll(/(?:^|\n)\s*(:root(?:\[data-theme='dark'\])?)\s*\{([^}]*)\}/g),
    (match) => ({ selector: match[1] as ThemeRule['selector'], body: match[2] ?? '' }),
  )
}

function tokensFor(selector: ThemeRule['selector']): Map<string, string> {
  const tokens = new Map<string, string>()
  for (const { selector: currentSelector, body } of themeRules()) {
    if (currentSelector !== selector) continue
    for (const declaration of body.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) {
      tokens.set(declaration[1] ?? '', declaration[2]?.trim() ?? '')
    }
  }
  return tokens
}

describe('theme contract', () => {
  it('defines only the default root and dark-mode contracts without preset selectors', () => {
    expect(themeRules().map(({ selector }) => selector)).toEqual([
      ':root',
      ':root',
      ":root[data-theme='dark']",
    ])
    for (const source of [themeSource, mainSource, desktopSource]) {
      expect(source).not.toContain('data-skin')
      expect(source).not.toContain('skin-signature')
    }
  })

  for (const selector of [':root', ":root[data-theme='dark']"] as const) {
    it(`${selector} defines every safe runtime token and fixed radius`, () => {
      const tokens = tokensFor(selector)
      for (const token of REQUIRED_DEFAULT_TOKENS) {
        expect(tokens.get(token), `${selector} misses ${token}`).toBeTruthy()
      }
    })
  }

  it('keeps shared compatibility aliases on semantic tokens', () => {
    const tokens = tokensFor(':root')
    expect(tokens.get('--brand-action')).toBe('var(--brand)')
    expect(tokens.get('--danger-action')).toBe('var(--danger)')
    expect(tokens.get('--accent-contrast')).toBe('var(--on-brand)')
    expect(tokens.get('--surface-muted')).toBe('var(--surface-subtle)')
    expect(tokens.get('--text-tertiary')).toBe('var(--muted)')
    expect(tokens.get('--line')).toBe('var(--border)')
    expect(tokens.get('--interaction-hover')).toContain('var(--brand)')
    expect(tokens.get('--interaction-hover-surface')).toContain('var(--brand)')
    expect(tokens.get('--interaction-hover-subtle')).toContain('var(--brand)')
    expect(tokens.get('--interaction-pressed')).toContain('var(--brand)')
    expect(tokens.get('--page-ambient-background')).toContain('var(--brand-soft)')
    expect(tokens.get('--surface-gradient')).toContain('var(--surface-raised)')
    expect(tokens.get('--surface-gradient-muted')).toContain('var(--surface-subtle)')
    expect(tokens.get('--auth-brand-background')).toContain('var(--sidebar-accent)')
    expect(tokens.get('--auth-form-background')).toContain('var(--brand-soft)')
  })

  it('connects the default contract to production CSS and desktop surfaces', () => {
    expect(mainSource).toContain("@import './themes.css';")
    expect(mainSource).toMatch(/body\s*\{[^}]*background:\s*var\(--page-ambient-background\)/)
    expect(mainSource).toMatch(/\.panel-card,[\s\S]*?background:\s*var\(--surface-gradient\)/)
    expect(mainSource).toMatch(/\.table-card\s*\{[^}]*background:\s*var\(--surface\)/)
    expect(mainSource).toMatch(/\.button\s*\{[^}]*border-radius:\s*var\(--radius-sm\)/)
    expect(mainSource).toMatch(/\.modal-panel\s*\{[^}]*border-radius:\s*var\(--radius-lg\)/)
    expect(desktopSource).toMatch(/\.desktop__menubar\s*\{[^}]*border-radius:\s*var\(--radius-lg\)/)
    expect(desktopSource).toMatch(/\.desktop-window\s*\{[^}]*border-radius:\s*var\(--radius-lg\)/)
    expect(desktopSource).toMatch(/\.desktop__taskbar\s*\{[^}]*border-radius:\s*var\(--radius\)/)
    expect(desktopSource).toContain('background: var(--desktop-wallpaper-base)')
    expect(desktopSource).toContain('background: var(--desktop-wallpaper-veil-light)')
    expect(desktopSource).toContain('background: var(--desktop-wallpaper-veil-dark)')
    expect(desktopSource).toContain('background: var(--desktop-wallpaper-vignette)')
    expect(desktopSource).toContain('background: var(--desktop-aurora-one)')
    expect(desktopSource).toContain('background: var(--desktop-aurora-two)')
    expect(desktopSource).toContain('opacity: var(--desktop-aurora-opacity)')
    expect(desktopSource).toMatch(/\.desktop__taskbar-item--active > i\s*\{[^}]*background:\s*var\(--theme-accent\)/)
  })

  it('keeps desktop health, warning and error states on semantic status tokens', () => {
    expect(desktopSource).toMatch(/\.desktop__taskbar-agent-status > i\s*\{[^}]*background:\s*var\(--success\);/)
    expect(desktopSource).toMatch(/\.desktop__taskbar-agent-status--offline > i,[\s\S]*?\.desktop__taskbar-agent-status--incompatible > i\s*\{[^}]*background:\s*var\(--danger\);/)
    expect(desktopSource).toMatch(/\.desktop__taskbar-agent-status--read-only > i\s*\{[^}]*background:\s*var\(--amber\);/)
    expect(desktopSource).toMatch(/\.desktop-service-status__dot--healthy\s*\{[^}]*background:\s*var\(--success\);[^}]*var\(--success-soft\)/)
    expect(desktopSource).toMatch(/\.desktop-service-status__dot--attention\s*\{[^}]*background:\s*var\(--amber\);[^}]*var\(--amber-soft\)/)
  })
})
