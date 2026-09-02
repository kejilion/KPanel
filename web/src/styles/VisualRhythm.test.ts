// @vitest-environment node
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  collectVueStyleBlocks,
  collectVueStyleDeclarations,
  isAllowedComponentBlurSelector,
  isBlurFilter,
  isNonTokenRadius,
  isNonTokenShadow,
  numericCssPixels,
  visualDeclarationKey,
} from './visualContract'
import { VISUAL_CONTRACT_BASELINE, baselineKey } from './visualContractBaseline'

const main = readFileSync(new URL('./main.css', import.meta.url), 'utf8')
const desktop = readFileSync(new URL('./desktop.css', import.meta.url), 'utf8')
const themes = readFileSync(new URL('./themes.css', import.meta.url), 'utf8')
const filesView = readFileSync(new URL('../views/FilesView.vue', import.meta.url), 'utf8')
const sources = { main, desktop }
const webRoot = fileURLToPath(new URL('../../', import.meta.url))
const componentStyleRoot = join(webRoot, 'src')
const componentStyleBlocks = collectVueStyleBlocks(componentStyleRoot)
const componentDeclarations = collectVueStyleDeclarations(componentStyleRoot)
const baselineKeys = new Set(VISUAL_CONTRACT_BASELINE.map((entry) => baselineKey(entry)))

/** Hex value of a token inside one themes.css block. */
function token(block: string, name: string): string {
  const match = block.match(new RegExp(`${name}:\\s*(#[0-9a-f]{6})`, 'i'))
  expect(match, `${name} should be a literal hex in this block`).toBeTruthy()
  return match![1]!.toLowerCase()
}

/** Perceived lightness, good enough to order neutral steps. */
function lightness(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255)
  return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!
}

function themeBlock(selector: string): string {
  const start = themes.indexOf(selector)
  expect(start).toBeGreaterThan(-1)
  const rest = themes.slice(start)
  return rest.slice(0, rest.indexOf('\n}') + 1)
}

/*
 * Visual rhythm contract.
 *
 * The lightweight theme pass consolidated radii onto the three semantic
 * radius tokens, collapsed invented font weights onto the weights the
 * system font stack actually ships, and removed negative tracking that the
 * shared visual language forbids on Chinese text. These assertions keep the
 * rhythm from drifting back one ad-hoc declaration at a time.
 */

// Hairline details below the smallest radius token: active-state slivers,
// tiny swatches and favicon crops, where an 8px corner would swallow the shape.
const HAIRLINE_RADII = new Set(['2px', '3px', '4px'])

// A pill is a shape, not a step on the scale.
const PILL_RADIUS = '999px'

function radiusValues(source: string): string[] {
  return Array.from(
    source.matchAll(/border-radius:\s*([^;]+);/g),
    (match) => match[1]!.trim(),
  )
}

const LANDSCAPE_QUERY = '@media (max-height: 560px) and (orientation: landscape)'

const STANDARD_FONT_WEIGHTS = new Set(['400', '500', '600', '700'])

function isComponentVisualDebt(declaration: (typeof componentDeclarations)[number]): boolean {
  if (declaration.property === 'font-size') {
    const size = numericCssPixels(declaration.value)
    return size !== null && size < 12
  }
  if (declaration.property === 'border-radius') return isNonTokenRadius(declaration.value)
  if (declaration.property === 'box-shadow') return isNonTokenShadow(declaration.value)
  if (declaration.property === 'backdrop-filter') {
    return isBlurFilter(declaration.value) && !isAllowedComponentBlurSelector(declaration.selector)
  }
  return false
}

/** Text of the landscape block only, so a later rule cannot satisfy an assertion. */
function landscapeBlock(source: string): string {
  const start = source.indexOf(LANDSCAPE_QUERY)
  expect(start).toBeGreaterThan(-1)
  const rest = source.slice(start)
  const end = rest.indexOf('\n@media', 1)
  return end === -1 ? rest : rest.slice(0, end)
}

describe('visual rhythm contract', () => {
  it('discovers every Vue style block for the repo-wide contract', () => {
    expect(componentStyleBlocks.length).toBeGreaterThan(0)
    const files = new Set(componentStyleBlocks.map((block) => block.file))
    expect(files.has('src/views/AppsView.vue')).toBe(true)
    expect(files.has('src/components/apps/AppInteractiveTerminal.vue')).toBe(true)
  })

  it('keeps historical component visual debt explicit, current, and expiring', () => {
    const sourceKeys = new Set(componentDeclarations.map(visualDeclarationKey))
    const seen = new Set<string>()

    for (const entry of VISUAL_CONTRACT_BASELINE) {
      const key = baselineKey(entry)
      expect(seen.has(key), `duplicate baseline entry: ${key}`).toBe(false)
      seen.add(key)
      expect(sourceKeys.has(key), `stale baseline entry: ${key}`).toBe(true)
      expect(entry.reason.trim().length, `missing reason: ${key}`).toBeGreaterThan(0)
      expect(entry.expandedPath.trim().length, `missing migration path: ${key}`).toBeGreaterThan(0)
      expect(typeof entry.replaceable, `missing replaceability decision: ${key}`).toBe('boolean')
      if (entry.property === 'font-size') {
        expect(entry.calculatedPx, `font-size should have a pixel calculation: ${key}`).not.toBeNull()
        expect(entry.calculatedPx, `font-size baseline must remain below 12px: ${key}`).toBeLessThan(12)
      }
      expect(Date.parse(entry.expires), `invalid or expired baseline: ${key}`).toBeGreaterThan(Date.now())
    }
  })

  it('does not add unapproved visual debt in Vue style blocks', () => {
    const offenders = componentDeclarations
      .filter(isComponentVisualDebt)
      .filter((declaration) => !baselineKeys.has(visualDeclarationKey(declaration)))
      .map((declaration) => `${declaration.file}:${declaration.line} ${declaration.selector} ${declaration.property}: ${declaration.value}`)

    expect(offenders).toEqual([])
  })

  it('keeps component font weights on the four standard values', () => {
    const offenders = componentDeclarations
      .filter((declaration) => declaration.property === 'font-weight')
      .filter((declaration) => /^\d+$/.test(declaration.value) && !STANDARD_FONT_WEIGHTS.has(declaration.value))
      .map((declaration) => `${declaration.file}:${declaration.line} ${declaration.selector}: ${declaration.value}`)

    expect(offenders).toEqual([])
  })

  it('never uses negative letter-spacing in component styles', () => {
    const offenders = componentDeclarations
      .filter((declaration) => declaration.property === 'letter-spacing' && /^-/.test(declaration.value))
      .map((declaration) => `${declaration.file}:${declaration.line} ${declaration.selector}: ${declaration.value}`)

    expect(offenders).toEqual([])
  })

  it('gives the light theme four separable neutral steps', () => {
    // Before this pass --surface and --surface-raised were both #ffffff, so
    // dialogs and menus had no elevation at all against their parent panel.
    const light = themeBlock(':root {\n  color:')
    const steps = ['--bg', '--surface-subtle', '--surface', '--surface-raised']
      .map((name) => ({ name, level: lightness(token(light, name)) }))

    for (let index = 1; index < steps.length; index += 1) {
      const previous = steps[index - 1]!
      const current = steps[index]!
      expect(
        current.level,
        `${current.name} must sit above ${previous.name} in the light ladder`,
      ).toBeGreaterThan(previous.level)
      expect(
        current.level - previous.level,
        `${previous.name} -> ${current.name} must be a perceivable step`,
      ).toBeGreaterThan(0.004)
    }
    expect(token(light, '--surface-raised')).toBe('#ffffff')
  })

  it('keeps the dark theme recessed toward the page with lifted nested fills', () => {
    // Dark mode inverts the middle of the ladder on purpose: the page is the
    // darkest layer, and nested wells must lift off their parent to read.
    const dark = themeBlock(":root[data-theme='dark']")
    const bg = lightness(token(dark, '--bg'))
    const surface = lightness(token(dark, '--surface'))
    const subtle = lightness(token(dark, '--surface-subtle'))
    const raised = lightness(token(dark, '--surface-raised'))

    expect(bg).toBeLessThan(surface)
    expect(surface).toBeLessThan(subtle)
    expect(subtle).toBeLessThan(raised)
  })

  it('keeps the shipped palette and the runtime solver on one shadow model', () => {
    // themes.css is the default palette; theme/colors.ts recomputes the same
    // tokens when a user opts into custom colors. They drifted apart before.
    const solver = readFileSync(new URL('../theme/colors.ts', import.meta.url), 'utf8')
    for (const opacity of ['0.1', '0.16']) {
      expect(solver).toContain(`'${opacity}'`)
    }
    expect(themes).toContain('--desktop-aurora-opacity: .1;')
    expect(themes).toContain('--desktop-aurora-opacity: .16;')
    // One contact shadow, plus a diffuse layer only for overlays.
    expect(themes).toMatch(/--shadow-sm: 0 1px 2px [^;]+;/)
    expect(themes).toMatch(/--shadow-md: 0 1px 2px [^,]+, 0 1[02]px (?:28|32)px [^;]+;/)
    expect(solver).toMatch(/const shadowSm = mode === 'light'/)
    expect(solver).toMatch(/0 1px 2px \$\{cssRgb\(shadowColor, 0\.09\)\}, 0 10px 28px/)
  })

  it('describes depth with one lighting pass instead of decorative glow', () => {
    const ambient = themes.match(/--page-ambient-background:([\s\S]*?);/)?.[1] ?? ''
    expect(ambient).not.toContain('radial-gradient')
    expect(ambient).toContain('linear-gradient(180deg')

    const surfaceGradient = themes.match(/--surface-gradient:([\s\S]*?);/)?.[1] ?? ''
    // Two stops: a top light and the surface itself. Not a painted sheen.
    expect(surfaceGradient.split(',').length).toBeLessThanOrEqual(3)
    expect(surfaceGradient).toContain('var(--surface-raised)')
  })

  it('keeps every corner on a radius token, a pill, or a declared hairline', () => {
    const offenders: string[] = []
    for (const [name, source] of Object.entries(sources)) {
      for (const value of radiusValues(source)) {
        // Circles, inherited corners and token-derived inner corners are
        // shapes rather than scale steps. Only bare px lengths are graded.
        const lengths = value.match(/(?<![\w-])\d+(?:\.\d+)?px/g) ?? []
        const graded = value.includes('calc(') ? [] : lengths
        for (const length of graded) {
          if (length === PILL_RADIUS) continue
          if (HAIRLINE_RADII.has(length)) continue
          offenders.push(`${name}: border-radius: ${value}`)
          break
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('derives nested inner corners from a token instead of a new literal', () => {
    for (const source of Object.values(sources)) {
      for (const value of radiusValues(source)) {
        if (!value.includes('calc(')) continue
        expect(value).toMatch(/calc\(var\(--radius[a-z-]*\)\s*-\s*\d+px\)/)
      }
    }
  })

  it('does not reintroduce a second radius scale beside the semantic tokens', () => {
    // 99px was an inconsistent second spelling of the pill radius.
    for (const source of Object.values(sources)) {
      expect(source).not.toMatch(/border-radius:\s*99px/)
    }
    for (const token of ['--radius-sm: 8px', '--radius: 12px', '--radius-lg: 18px']) {
      expect(readFileSync(new URL('./themes.css', import.meta.url), 'utf8')).toContain(token)
    }
  })

  it('keeps font weights on the four the system font stack can actually render', () => {
    // There is no @font-face in the product, so Inter falls back to the
    // platform UI font. Weights like 650 or 780 snap to a neighbour and only
    // create the illusion of a finer scale.
    const offenders: string[] = []
    for (const [name, source] of Object.entries(sources)) {
      for (const match of source.matchAll(/font-weight:\s*(\d+)/g)) {
        if (!['400', '500', '600', '700'].includes(match[1]!)) {
          offenders.push(`${name}: font-weight: ${match[1]}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('never sets negative letter-spacing, which damages Chinese text', () => {
    for (const source of Object.values(sources)) {
      expect(source).not.toMatch(/letter-spacing:\s*-/)
    }
    // Numeric alignment comes from tabular figures, not from squeezing.
    expect(main).toMatch(/\.metric-card > strong\s*\{[^}]*font-variant-numeric:\s*tabular-nums;/)
  })

  it('limits translucency to OS chrome and modal scrims', () => {
    const allowed = [
      '.desktop__menubar',
      '.desktop__taskbar',
      '.desktop__file-drop',
      '.modal-backdrop',
      '.ai-settings-backdrop',
      '.theme-color-actions > div',
    ]
    const offenders: string[] = []
    for (const [name, source] of Object.entries(sources)) {
      const rules = source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)
      for (const rule of rules) {
        const selector = rule[1]!.trim()
        if (!/backdrop-filter:\s*blur/.test(rule[2]!)) continue
        if (allowed.some((entry) => selector.includes(entry))) continue
        offenders.push(`${name}: ${selector}`)
      }
    }
    expect(offenders).toEqual([])
  })

  it('drops painted-on highlights from neutral panels and window chrome', () => {
    // A white inset line on a neutral surface reads as fake gloss. It stays
    // only on saturated icon tiles, where it behaves like real shading.
    expect(desktop).toMatch(/\.desktop-window\s*\{[^}]*box-shadow:\s*var\(--shadow-md\);/)
    expect(desktop).not.toMatch(/\.desktop__menubar\s*\{[^}]*inset 0 1px 0 rgb\(255 255 255/)
    expect(desktop).not.toMatch(/\.desktop__taskbar\s*\{[^}]*inset 0 1px 0 rgb\(255 255 255/)
  })

  it('routes the window close affordance through the danger pair', () => {
    expect(desktop).toMatch(
      /\.desktop-window__action--close:hover\s*\{[^}]*color:\s*var\(--on-danger\);[^}]*background:\s*var\(--danger-action\);/,
    )
    expect(desktop).not.toContain('#c42b1c')
  })

  it('gives landscape short viewports their own vertical budget', () => {
    const mainBlock = landscapeBlock(main)
    // Dialogs measure against the live viewport instead of a fixed 90vh cap,
    // which left roughly 351px of usable height at 844x390.
    expect(mainBlock).toMatch(/--topbar-height:\s*56px;/)
    expect(mainBlock).toMatch(/max-height:\s*calc\(\s*100dvh - 24px - env\(safe-area-inset-top\) - env\(safe-area-inset-bottom\)\s*\)/)
    expect(mainBlock).toMatch(/\.modal-backdrop--fullscreen\s*\{[^}]*padding:\s*0;/)
    expect(mainBlock).toMatch(/\.modal-panel--fullscreen\s*\{[^}]*height:\s*100dvh;/)
    // The media viewer has a wider non-fullscreen override in its scoped CSS.
    // Keep it from winning over the shared full-screen viewport contract.
    expect(filesView).toMatch(/:global\(\.modal-panel--wide:not\(\.modal-panel--fullscreen\):has\(\.media-viewer\)\)/)

    const desktopBlock = landscapeBlock(desktop)
    // Windows and icons are re-cut against the taskbar reserve only. There is
    // no menubar markup in the app, so no top chrome height is reserved.
    expect(desktopBlock).toMatch(/\.desktop__taskbar\s*\{[^}]*height:\s*44px;/)
    expect(desktopBlock).toMatch(/top:\s*max\(8px, env\(safe-area-inset-top\)\) !important;/)
    expect(desktopBlock).toMatch(/bottom:\s*calc\(52px \+ max\(6px, env\(safe-area-inset-bottom\)\)\) !important;/)
    expect(desktopBlock).not.toMatch(/top:\s*calc\(44px \+/)
  })

  it('reserves landscape top space only for chrome that actually renders', () => {
    // .desktop__menubar is a legacy selector the theme contract still pins,
    // but no component renders it. Reserving height for it would waste the
    // scarcest axis in landscape, so the reserve is measured from the taskbar.
    const desktopBlock = landscapeBlock(desktop)
    const icons = desktopBlock.match(/\.desktop__icons\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(icons).toMatch(/inset:\s*\n?\s*max\(8px, env\(safe-area-inset-top\)\)/)
  })

  it('keeps landscape touch targets at the 40px minimum', () => {
    const desktopBlock = landscapeBlock(desktop)
    for (const selector of ['.desktop__taskbar-item', '.desktop__tray-button', '.desktop__classic-button']) {
      const rule = desktopBlock.match(new RegExp(`${selector.replace('.', '\\.')}[^{]*\\{([^}]*)\\}`))?.[1] ?? ''
      const size = rule.match(/(?:min-)?height:\s*(\d+)px/)?.[1]
      expect(Number(size), `${selector} in landscape`).toBeGreaterThanOrEqual(40)
    }
  })

  it('orders the landscape override after the portrait phone rules', () => {
    expect(main.indexOf('@media (max-width: 480px)')).toBeLessThan(main.indexOf(LANDSCAPE_QUERY))
    expect(desktop.indexOf('@media (max-width: 420px)')).toBeLessThan(desktop.indexOf(LANDSCAPE_QUERY))
  })
})
