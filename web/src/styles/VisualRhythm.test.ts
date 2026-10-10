// @vitest-environment node
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { contrastRatio } from '../theme/colors'
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
const classicWallpaper = readFileSync(new URL('./classicWallpaper.css', import.meta.url), 'utf8')
const desktopWallpaper = readFileSync(new URL('./desktopWallpaper.css', import.meta.url), 'utf8')
const filesView = readFileSync(new URL('../views/FilesView.vue', import.meta.url), 'utf8')
const sources = { main, desktop }
const webRoot = fileURLToPath(new URL('../../', import.meta.url))
const componentStyleRoot = join(webRoot, 'src')
const componentStyleBlocks = collectVueStyleBlocks(componentStyleRoot)
const componentDeclarations = collectVueStyleDeclarations(componentStyleRoot)
const baselineKeys = new Set(VISUAL_CONTRACT_BASELINE.map((entry) => baselineKey(entry)))

/** A selector or literal, escaped for use inside a RegExp source. */
function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

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
  if (/^(?:-webkit-)?backdrop-filter$/.test(declaration.property)) {
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

  it('uses the same outer radius token for desktop groups and widgets', () => {
    const group = readFileSync(new URL('../components/desktop/DesktopGroupCard.vue', import.meta.url), 'utf8')
    expect(group.match(/\.desktop-group\s*\{([^}]+)\}/)?.[1]).toContain('border-radius: var(--radius-lg)')
    expect(desktop.match(/\.desktop-widget-slot\s*\{([^}]+)\}/)?.[1]).toContain('border-radius: var(--radius-lg)')
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

  /*
   * Materials (docs/ui-visual-language.md 3.5). Live blur belongs to resident
   * shell chrome and short-lived overlays, with explicitly listed fixed filters.
   * Classic wallpaper may tune page fills but never adds blur to content cards.
   * The selector lists are the machine side of the spec's layer table.
   */
  const MATERIAL_SURFACES = {
    chrome: ['.desktop__menubar', '.desktop__taskbar'],
    overlay: ['.k-context-menu', '.desktop-start-menu'],
  } as const
  // Panels rest on the wallpaper alone: translucent fill, no resident live blur.
  const PANEL_SURFACES = ['.desktop-clock,\n.desktop-monitor,\n.desktop-service-status']
  // Chrome whose fill is tuned elsewhere (the classic wallpaper level) still takes
  // the chrome material's blur, so no literal radius escapes the tokens.
  const CHROME_FILTER_ONLY = [':root:has(.classic-backdrop) .topbar']
  // Scrims and drag feedback keep their own light, fixed blur.
  const FIXED_BLUR_SURFACES = ['.desktop__file-drop', '.modal-scrim']

  function backdropRules(source: string): Array<{ selector: string, value: string }> {
    const rules = source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)
    const found: Array<{ selector: string, value: string }> = []
    for (const rule of rules) {
      for (const declaration of rule[2]!.matchAll(/(?:^|;)\s*(?:-webkit-)?backdrop-filter:\s*([^;}]+)/g)) {
        found.push({ selector: rule[1]!.trim(), value: declaration[1]!.trim() })
      }
    }
    return found
  }

  it('finds prefixed and unterminated filters and treats variable filters as possible blur', () => {
    expect(backdropRules('.bad { -webkit-backdrop-filter: blur(30px) }')).toEqual([
      { selector: '.bad', value: 'blur(30px)' },
    ])
    expect(backdropRules('.bad { color: red; backdrop-filter: blur(30px) }')).toEqual([
      { selector: '.bad', value: 'blur(30px)' },
    ])
    for (const property of ['backdrop-filter', '-webkit-backdrop-filter']) {
      expect(isComponentVisualDebt({ file: 'fixture.vue', line: 1, selector: '.bad', property, value: 'var(--x-blur)' })).toBe(true)
    }
  })

  it('caps every literal backdrop blur, including fixed filters and component variables', () => {
    for (const source of [main, desktop, desktopWallpaper, classicWallpaper, themes, ...componentStyleBlocks.map((block) => block.content)]) {
      const clean = source.replace(/\/\*[\s\S]*?\*\//g, '')
      const filters = [
        ...backdropRules(clean).map((rule) => rule.value),
        ...Array.from(clean.matchAll(/--[\w-]+\s*:\s*([^;{}]+)/g), (match) => match[1]!),
      ]
      for (const filter of filters) {
        for (const blur of filter.matchAll(/\bblur\(\s*(\d*\.?\d+)(px|rem)\s*\)/g)) {
          expect(Number(blur[1]) * (blur[2] === 'rem' ? 16 : 1), blur[0]).toBeLessThanOrEqual(24)
        }
      }
    }
  })

  it('limits translucency to shell chrome, short-lived overlays and scrims', () => {
    const materialSelectors = [...MATERIAL_SURFACES.chrome, ...MATERIAL_SURFACES.overlay, ...CHROME_FILTER_ONLY]
    const offenders: string[] = []
    for (const [name, source] of Object.entries({ ...sources, classicWallpaper, desktopWallpaper })) {
      for (const { selector, value } of backdropRules(source)) {
        if (value === 'none') continue
        const isMaterial = (materialSelectors as readonly string[]).includes(selector)
        const isFixed = FIXED_BLUR_SURFACES.includes(selector)
        if (isMaterial && /^var\(--material-(chrome|overlay)-filter\)$/.test(value)) continue
        if (isFixed && /^blur\(\d*\.?\d+px\)(?:\s+saturate\(\d+%\))?$/.test(value)) continue
        offenders.push(`${name}: ${selector} { backdrop-filter: ${value} }`)
      }
    }
    expect(offenders).toEqual([])
  })

  it('keeps static material text readable over black and white without running the theme solver', () => {
    for (const block of [themeBlock(':root {\n  color:'), themeBlock(":root[data-theme='dark']")]) {
      const rgba = block.match(/--desktop-glass-strong:\s*rgb\((\d+) (\d+) (\d+) \/ ([\d.]+)%\)/)!
      expect(rgba).toBeTruthy()
      const alpha = Number(rgba[4]) / 100
      for (const backdrop of [0, 255]) {
        const background = '#' + rgba.slice(1, 4).map((channel) => Math.round(
          Number(channel) * alpha + backdrop * (1 - alpha),
        ).toString(16).padStart(2, '0')).join('')
        for (const role of ['panel', 'chrome', 'overlay']) {
          expect(themes).toContain(`--material-${role}-fill: var(--desktop-glass-strong);`)
          for (const label of ['--text', '--text-soft', '--muted']) {
            expect(contrastRatio(token(block, label), background), `${role}/${label} over ${backdrop}`)
              .toBeGreaterThanOrEqual(4.5)
          }
        }
      }
    }
  })

  it('applies each material through its tokens, never a literal blur', () => {
    for (const [role, selectors] of Object.entries(MATERIAL_SURFACES)) {
      for (const selector of selectors) {
        const escaped = escapeRegExp(selector)
        const rule = `${main}\n${desktop}`.match(new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? ''
        expect(rule, `${selector} must be a ${role} material`).toContain(`background: var(--material-${role}-fill);`)
        expect(rule, selector).toContain(`border: 1px solid var(--material-${role}-edge);`)
        expect(rule, selector).toContain(`backdrop-filter: var(--material-${role}-filter);`)
        expect(rule, selector).toContain(`-webkit-backdrop-filter: var(--material-${role}-filter);`)
      }
    }
    for (const selector of PANEL_SURFACES) {
      const escaped = escapeRegExp(selector)
      const rule = desktop.match(new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? ''
      expect(rule, selector).toContain('background: var(--material-panel-fill);')
      expect(rule, selector).toContain('border: 1px solid var(--material-panel-edge);')
      expect(rule, selector).not.toContain('backdrop-filter')
    }
    expect(desktop).toContain('--desktop-group-surface: var(--material-panel-fill);')
    // Translucent surfaces consume material tokens so they turn solid together;
    // raw glass survives only inside two brand-tinted state indicators.
    const rawGlass = desktop.split('\n').filter((line) => /var\(--desktop-glass/.test(line) && !/^\s*--/.test(line)).map((line) => line.trim())
    expect(rawGlass).toEqual([
      'border-color: color-mix(in srgb, var(--brand) 20%, var(--desktop-glass-border));',
      'background: color-mix(in srgb, var(--brand) 18%, var(--desktop-glass));',
    ])
    // Material blur radii live in two tokens; fixed exceptions share the 24px cap.
    const filters = Array.from(themes.matchAll(/--material-(chrome|overlay)-filter:\s*blur\((\d+)px\)[^;]*;/g))
    expect(filters.map((match) => match[1])).toEqual(['chrome', 'overlay'])
    for (const match of filters) expect(Number(match[2])).toBeLessThanOrEqual(24)
    expect(themes).not.toMatch(/--material-panel-filter/)
    // Text sits on materials, so their fills reuse the solver's contrast-safe glass.
    for (const role of ['panel', 'chrome', 'overlay']) {
      expect(themes).toContain(`--material-${role}-fill: var(--desktop-glass-strong);`)
    }
  })

  it('keeps desktop windows solid and content panels free of live blur', () => {
    for (const selector of ['.desktop-window', '.desktop-window__titlebar', '.desktop-window__body', '.modal-panel']) {
      for (const { selector: ruleSelector, value } of [...backdropRules(main), ...backdropRules(desktop)]) {
        if (value === 'none') continue
        expect(ruleSelector.split(',').map((part) => part.trim()), `${selector} must not blur`).not.toContain(selector)
      }
    }
    expect(desktop).toMatch(/\.desktop-window\s*\{[^}]*background:\s*var\(--surface\);/)
    // A blurred scrim already softens what lies under the phone folder sheet.
    expect(desktop.match(/\.desktop-folder__sheet\s*\{([^}]*)\}/)?.[1]).not.toContain('backdrop-filter')
  })

  it('makes materials solid for accessibility and blurred surfaces solid without blur support', () => {
    const degrade = main.match(/@media \(prefers-reduced-transparency: reduce\), \(prefers-contrast: more\) \{\s*:root \{([^}]*)\}/)?.[1] ?? ''
    for (const role of ['panel', 'chrome', 'overlay']) {
      expect(degrade).toContain(`--material-${role}-fill: var(--surface-raised);`)
      expect(degrade).toContain(`--material-${role}-edge: var(--border-strong);`)
    }
    for (const role of ['chrome', 'overlay']) expect(degrade).toContain(`--material-${role}-filter: none;`)
    const unsupported = main.match(/@supports not \(\(backdrop-filter: blur\(1px\)\) or \(-webkit-backdrop-filter: blur\(1px\)\)\) \{\s*:root \{([^}]*)\}/)?.[1] ?? ''
    // Panels never blur, so only the blurred materials need a solid fallback.
    for (const role of ['chrome', 'overlay']) {
      expect(unsupported).toContain(`--material-${role}-fill: var(--surface-raised);`)
    }
    for (const condition of [
      '@media (prefers-reduced-transparency: reduce), (prefers-contrast: more)',
      '@supports not ((backdrop-filter: blur(1px)) or (-webkit-backdrop-filter: blur(1px)))',
    ]) {
      const fallback = classicWallpaper.slice(classicWallpaper.indexOf(condition)).match(/\.topbar\s*\{([^}]*)\}/)?.[1] ?? ''
      expect(fallback, condition).toContain('background: var(--material-chrome-fill);')
    }
  })

  it('tells the focused window apart by more than its shadow', () => {
    const inactive = desktop.match(/\n\.desktop-window__titlebar\s*\{([^}]*)\}/)?.[1] ?? ''
    const active = desktop.match(/\.desktop-window--focused \.desktop-window__titlebar\s*\{([^}]*)\}/)?.[1] ?? ''
    expect(inactive).toContain('background: var(--desktop-titlebar-inactive);')
    expect(inactive).toContain('color: var(--muted);')
    expect(active).toContain('background: var(--desktop-titlebar-active);')
    expect(active).toContain('color: var(--text);')
    expect(desktop).toMatch(/\.desktop-window--focused\s*\{[^}]*border-color:[^;]*var\(--brand\)[^;]*;[^}]*box-shadow:\s*var\(--desktop-window-shadow-active\);/)
    expect(desktop).toMatch(/--desktop-titlebar-active:\s*color-mix\(in srgb, var\(--surface-raised\) \d+%, var\(--brand\) \d+%\);/)
  })

  it('moves on the shared motion tokens and never animates a blur', () => {
    for (const token of [
      '--motion-duration-instant: 100ms;',
      '--motion-duration-fast: 160ms;',
      '--motion-duration-base: 220ms;',
      '--motion-duration-layout: 260ms;',
      '--motion-duration-ambient: 420ms;',
      '--motion-ease-standard: cubic-bezier(.22, 1, .36, 1);',
      '--motion-ease-exit: cubic-bezier(.4, 0, 1, 1);',
      '--motion-ease-fade: ease;',
    ]) expect(themes).toContain(token)
    // Surfaces migrated to the motion system stay on it.
    for (const selector of ['.desktop-window', '.desktop-window--closing', '.desktop-start-menu-enter-active', '.desktop-menu-enter-active', '.desktop-menu-leave-active']) {
      const escaped = escapeRegExp(selector)
      const rule = desktop.match(new RegExp(`\\n${escaped}\\s*\\{([^}]*)\\}`))?.[1] ?? ''
      const transition = rule.match(/transition:\s*([^;]+);/)?.[1] ?? ''
      expect(transition, selector).toContain('var(--motion-')
      expect(transition, selector).not.toMatch(/\d+(?:\.\d+)?m?s\b/)
    }
    // Exits are quicker than entrances and accelerate away.
    expect(desktop).toMatch(/\.desktop-menu-leave-active\s*\{[^}]*var\(--motion-duration-instant\) var\(--motion-ease-exit\)/)
    // A backdrop blur re-samples everything behind it on every frame it changes;
    // materials appear by fading their surface, never by interpolating the blur.
    for (const source of Object.values(sources)) {
      expect(source).not.toMatch(/transition(?:-property)?:[^;]*\bbackdrop-filter\b/)
    }
  })

  it('moves the classic shell on the same motion tokens', () => {
    const rule = (selector: string) => main.match(new RegExp(`\\n${escapeRegExp(selector)}\\s*\\{([^}]*)\\}`))?.[1] ?? ''
    // Sidebar geometry moves together on the base step; the collapsed labels hand
    // off visibility only after that step, so text never shows mid-collapse.
    expect(rule('.sidebar')).toMatch(/width var\(--motion-duration-base\) var\(--motion-ease-standard\),\s*transform var\(--motion-duration-base\)/)
    expect(main).toMatch(/\.sidebar--collapsed \.sidebar__user > svg\s*\{[^}]*opacity var\(--motion-duration-instant\)[^}]*visibility 0s linear var\(--motion-duration-base\);/)
    // Controls keep their 160ms feedback, now named.
    expect(rule('.button')).toMatch(/transition:\s*border-color var\(--motion-duration-fast\) var\(--motion-ease-fade\)/)
    // Dialogs settle on the base step; toasts and fades leave quicker than they came.
    expect(rule('.modal-panel')).toContain('animation: modal-panel-in var(--motion-duration-base) var(--motion-ease-standard) both;')
    expect(rule('.modal-backdrop')).toContain('animation: modal-backdrop-in var(--motion-duration-fast) var(--motion-ease-fade) both;')
    expect(rule('.toast-leave-active')).toContain('var(--motion-duration-instant) var(--motion-ease-exit)')
    expect(rule('.fade-leave-active')).toContain('var(--motion-duration-instant) var(--motion-ease-exit)')
  })

  it('keeps overshoot to the app icon launch and hover feedback', () => {
    // ui-visual-language 3.6.2: nothing bounces, except the icon you just pressed.
    const allowed = ['.desktop__icon-glyph', '.desktop__icon--launching .desktop__icon-glyph']
    const offenders: string[] = []
    for (const [name, source] of Object.entries({ ...sources, desktopWallpaper, classicWallpaper })) {
      for (const rule of source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
        for (const curve of rule[2]!.matchAll(/cubic-bezier\(([^)]+)\)/g)) {
          const [, y1, , y2] = curve[1]!.split(',').map(Number)
          if (y1! >= 0 && y1! <= 1 && y2! >= 0 && y2! <= 1) continue
          if (allowed.includes(rule[1]!.trim()) && y1! >= 0 && y1! <= 1.3 && y2! >= 0 && y2! <= 1.3) continue
          offenders.push(`${name}: ${rule[1]!.trim()} ${curve[0]}`)
        }
      }
    }
    expect(offenders).toEqual([])
  })

  it('keeps migrated entry motion bounded and each exit property one step shorter', () => {
    const rule = (source: string, selector: string) => source.match(new RegExp(`${escapeRegExp(selector)}\\s*\\{([^}]*)\\}`))?.[1] ?? ''
    const closing = rule(desktop, '.desktop-window--closing')
    expect(closing).toContain('opacity var(--motion-duration-instant) var(--motion-ease-exit)')
    expect(closing).toContain('transform var(--motion-duration-fast) var(--motion-ease-exit)')
    expect(rule(desktop, '.desktop-folder-enter-active')).toContain('opacity var(--motion-duration-base) var(--motion-ease-fade)')
    expect(rule(desktop, '.desktop-folder-leave-active')).toContain('opacity var(--motion-duration-fast) var(--motion-ease-exit)')
    expect(rule(desktop, '.desktop-folder-leave-active .desktop-folder__sheet')).toContain('opacity var(--motion-duration-instant) var(--motion-ease-exit)')
    const folder = rule(desktop, '.desktop-folder-leave-to .desktop-folder__sheet')
    const toast = rule(main, '.toast-leave-to')
    expect(folder).toMatch(/scale\([\d.]+\)/)
    expect(toast).toMatch(/translateX\([\d.]+px\)/)
    expect(Number(folder.match(/scale\(([\d.]+)\)/)?.[1])).toBeGreaterThanOrEqual(.96)
    expect(Math.abs(Number(toast.match(/translateX\(([\d.]+)px\)/)?.[1]))).toBeLessThanOrEqual(12)
    expect(main).not.toMatch(/transform\s+var\(--motion-duration-[\w-]+\)\s+var\(--motion-ease-fade\)/)
  })

  it('only lets literal motion durations shrink', () => {
    // Ratchet for 3.6.4: literal durations still owed to the motion tokens. Lower
    // a ceiling when a feature migrates; never raise one to fit new code.
    const ceilings: Record<string, number> = {
      main: 28, desktop: 78, desktopWallpaper: 2, classicWallpaper: 0,
      'src/components/cluster/ClusterGlobe.vue': 1,
      'src/components/cluster/ClusterNotificationsDialog.vue': 6,
      'src/components/cluster/ClusterTemporarySortMenu.vue': 3,
      'src/components/common/HostSwitcher.vue': 7,
      'src/components/docker/DockerDeploymentEditor.vue': 5,
      'src/components/docker/DockerUsageMeter.vue': 2,
      'src/components/files/FileShareDialog.vue': 1,
      'src/components/files/FileShareManagerDialog.vue': 1,
      'src/components/files/FilesSplitWorkspace.vue': 2,
      'src/components/gallery/GalleryMoveDialog.vue': 1,
      'src/components/gallery/GalleryTile.vue': 5,
      'src/components/gallery/GalleryViewer.vue': 12,
      'src/components/monitoring/TrendChart.vue': 2,
      'src/components/overview/DiskPartitionDialog.vue': 1,
      'src/components/overview/FirewallManagerDialog.vue': 1,
      'src/components/overview/SystemTuningDialog.vue': 4,
      'src/components/sites/LocalWebServicePicker.vue': 3,
      'src/components/terminal/BatchTerminalPanel.vue': 3,
      'src/components/terminal/TerminalQuickCommands.vue': 1,
      'src/views/AppsView.vue': 5,
      'src/views/ClusterShareView.vue': 5,
      'src/views/ClusterView.vue': 1,
      'src/views/DiagnosticsView.vue': 1,
      'src/views/DockerView.vue': 15,
      'src/views/EnvironmentView.vue': 1,
      'src/views/FileShareView.vue': 1,
      'src/views/FilesView.vue': 8,
      'src/views/GalleryView.vue': 5,
      'src/views/MonitoringView.vue': 8,
      'src/views/ProcessManagerView.vue': 5,
      'src/views/SettingsView.vue': 9,
      'src/views/TerminalView.vue': 12,
    }
    const literalDurations = (source: string) => {
      let total = 0
      for (const match of source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/(?:(?:transition|animation)(?:-duration|-delay)?|--[\w-]+)\s*:\s*([^;{}]+)/g)) {
        total += (match[1]!.match(/(?<![\w-])\d*\.?\d+m?s\b/g) ?? []).filter((value) => !['0s', '0ms', '.01ms', '0.01ms'].includes(value)).length
      }
      return total
    }
    expect(literalDurations('.x { --x-duration: 300ms; transition: opacity var(--x-duration) }')).toBe(1)
    expect(literalDurations('.x { animation: fade 300ms }')).toBe(1)
    const counts: Record<string, number> = {
      main: literalDurations(main),
      desktop: literalDurations(desktop),
      desktopWallpaper: literalDurations(desktopWallpaper),
      classicWallpaper: literalDurations(classicWallpaper),
    }
    for (const block of componentStyleBlocks) {
      counts[block.file] = (counts[block.file] ?? 0) + literalDurations(block.content)
    }
    for (const [name, count] of Object.entries(counts)) {
      expect(count, `${name} literal motion durations`).toBeLessThanOrEqual(ceilings[name] ?? 0)
    }
  })

  it('drops painted-on highlights from neutral panels and window chrome', () => {
    // A white inset line on a neutral surface reads as fake gloss. It stays
    // only on saturated icon tiles, where it behaves like real shading.
    expect(desktop).toMatch(/\.desktop-window\s*\{[^}]*box-shadow:\s*var\(--shadow-md\);/)
    expect(desktop).not.toMatch(/\.desktop__menubar\s*\{[^}]*inset 0 1px 0 rgb\(255 255 255/)
    expect(desktop).not.toMatch(/\.desktop__taskbar\s*\{[^}]*inset 0 1px 0 rgb\(255 255 255/)
    const group = readFileSync(new URL('../components/desktop/DesktopGroupCard.vue', import.meta.url), 'utf8')
    expect(group.match(/\.desktop-group\s*\{([^}]+)\}/)?.[1]).toContain('box-shadow: var(--shadow-md);')
  })

  it('routes the window close affordance through the danger pair', () => {
    expect(desktop).toMatch(
      /\.desktop-window__action--close:hover\s*\{[^}]*color:\s*var\(--on-danger\);[^}]*background:\s*var\(--danger-action\);/,
    )
    expect(desktop).not.toContain('#c42b1c')
  })

  it('gives workspace dialogs the desktop window chrome and flush content', () => {
    // One 42px title bar with flat window buttons, borrowed from desktop windows.
    expect(main).toMatch(/\.modal-panel--workspace \.modal-panel__titlebar\s*\{[^}]*min-height:\s*42px;[^}]*padding:\s*0 0 0 14px;/)
    expect(main).toMatch(/\.modal-panel__window-action\s*\{[^}]*width:\s*46px;[^}]*border:\s*0;[^}]*border-radius:\s*0;/)
    expect(main).toMatch(
      /\.modal-panel__window-action--close:hover:not\(:disabled\)\s*\{[^}]*color:\s*var\(--on-danger\);[^}]*background:\s*var\(--danger-action\);/,
    )
    // Content runs to the panel edges and fills the window height.
    expect(main).toMatch(/\.modal-panel--workspace \.modal-panel__body\s*\{[^}]*flex:\s*1 1 auto;[^}]*min-height:\s*0;[^}]*padding:\s*0;/)
    expect(main).toMatch(/\.modal-panel--workspace\s*\{[^}]*height:\s*min\(900px, calc\(100dvh - 48px\)\);/)
    // Full screen is the whole viewport, like a maximized desktop window.
    expect(main).toMatch(/\.modal-backdrop--fullscreen\s*\{[^}]*padding:\s*0;/)
    expect(main).toMatch(/\.modal-panel--fullscreen\s*\{[^}]*width:\s*100vw;[^}]*height:\s*100dvh;[^}]*border-radius:\s*0;/)
    // The workspace size rule comes later in the file, so full screen needs a
    // compound selector to win; otherwise the window stays at 1360x900.
    expect(main).toMatch(/\.modal-panel--workspace\.modal-panel--fullscreen\s*\{[^}]*width:\s*100vw;[^}]*height:\s*100dvh;/)
  })

  it('gives landscape short viewports their own vertical budget', () => {
    const mainBlock = landscapeBlock(main)
    // Dialogs measure against the live viewport instead of a fixed 90vh cap,
    // which left roughly 351px of usable height at 844x390.
    expect(mainBlock).toMatch(/--topbar-height:\s*56px;/)
    expect(mainBlock).toMatch(/max-height:\s*calc\(\s*100dvh - 24px - env\(safe-area-inset-top\) - env\(safe-area-inset-bottom\)\s*\)/)
    expect(mainBlock).toMatch(/\.modal-backdrop--fullscreen\s*\{[^}]*padding:\s*0;/)
    expect(mainBlock).toMatch(/\.modal-panel--fullscreen\s*\{[^}]*height:\s*100dvh;/)
    // Workspace windows (editors, viewers, task terminals) keep the full
    // landscape width instead of the 680px form-dialog cap.
    expect(mainBlock).toMatch(/\.modal-panel--workspace\s*\{[^}]*width:\s*100%;[^}]*100dvh - 24px/)
    // The media viewer sizes itself through the workspace contract only, so no
    // scoped dialog override can win over the shared full-screen viewport.
    expect(filesView).not.toContain('.modal-panel--wide')

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
