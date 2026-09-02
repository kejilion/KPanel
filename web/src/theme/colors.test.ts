import { describe, expect, it } from 'vitest'
import {
  DEFAULT_THEME_COLORS,
  THEME_COLOR_KEYS,
  THEME_COLOR_PRESETS,
  THEME_TOKEN_NAMES,
  contrastRatio,
  deriveThemeTokens,
  normalizeHexColor,
  normalizeThemeColors,
  parseStoredThemeColors,
  serializeThemeColors,
  type ThemeColorIntent,
  type ThemeMode,
  type ThemeTokenMap,
} from './colors'

const HEX_TOKENS = [
  '--bg', '--surface', '--surface-subtle', '--surface-raised',
  '--text', '--text-soft', '--muted', '--border', '--border-strong', '--control-border',
  '--brand', '--brand-strong', '--brand-soft', '--brand-muted', '--theme-accent', '--on-brand',
  '--success', '--success-soft', '--blue', '--blue-soft', '--violet', '--violet-soft',
  '--amber', '--amber-soft', '--danger', '--danger-soft', '--on-danger', '--neutral-soft',
  '--sidebar', '--sidebar-text', '--sidebar-muted', '--sidebar-border', '--sidebar-hover',
  '--sidebar-active', '--sidebar-accent', '--desktop-label', '--scrollbar-track', '--scrollbar-thumb',
  '--scrollbar-thumb-hover', '--scrollbar-thumb-active',
] as const

const AA_PAIRS = [
  ['--text', '--surface'], ['--text-soft', '--surface'], ['--muted', '--surface'],
  ['--muted', '--surface-subtle'], ['--muted', '--neutral-soft'],
  ['--brand', '--surface'], ['--brand', '--brand-soft'], ['--brand-strong', '--surface'],
  ['--brand-strong', '--brand-soft'], ['--success', '--surface'], ['--success', '--success-soft'],
  ['--blue', '--surface'], ['--blue', '--blue-soft'], ['--violet', '--surface'],
  ['--violet', '--violet-soft'], ['--amber', '--surface'], ['--amber', '--amber-soft'],
  ['--danger', '--surface'], ['--danger', '--danger-soft'], ['--sidebar-text', '--sidebar'],
  ['--sidebar-muted', '--sidebar'], ['--theme-accent', '--sidebar'],
  ['--sidebar-accent', '--sidebar'],
  ['--on-brand', '--brand'], ['--on-brand', '--brand-strong'], ['--on-danger', '--danger'],
] as const

function expectAccessible(tokens: ThemeTokenMap): void {
  expect(Object.keys(tokens)).toEqual([...THEME_TOKEN_NAMES])
  for (const token of HEX_TOKENS) expect(tokens[token]).toMatch(/^#[0-9a-f]{6}$/)
  for (const [foreground, background] of AA_PAIRS) {
    expect(
      contrastRatio(tokens[foreground], tokens[background]),
      `${foreground}/${background}`,
    ).toBeGreaterThanOrEqual(4.5)
  }
  expect(contrastRatio(tokens['--control-border'], tokens['--surface'])).toBeGreaterThanOrEqual(3)
  expect(contrastRatio(tokens['--control-border'], tokens['--surface-raised'])).toBeGreaterThanOrEqual(3)
  expect(contrastRatio(tokens['--theme-accent'], tokens['--surface'])).toBeGreaterThanOrEqual(3)
  expect(contrastRatio(tokens['--theme-accent'], tokens['--surface-raised'])).toBeGreaterThanOrEqual(3)
  for (const value of Object.values(tokens)) {
    expect(value).not.toMatch(/[;{}]|url\(|var\(|NaN|undefined/i)
  }
}

/** HSL lightness, the axis the foundation ladder is actually built on. */
function foundationLightness(hex: string): number {
  const channels = [1, 3, 5].map((offset) => Number.parseInt(hex.slice(offset, offset + 2), 16) / 255)
  return (Math.max(...channels) + Math.min(...channels)) / 2
}

/*
 * The neutral ladder has to stay separable for every color intent, not just the
 * shipped palette. Light mode runs page < subtle < surface < raised; dark mode
 * recesses toward the page and lifts nested fills, so subtle sits above surface.
 * Anchoring the light ladder below pure white is what keeps a lightening tone
 * offset from folding two steps into the same clamped white.
 */
function expectSeparableLadder(tokens: ThemeTokenMap, mode: ThemeMode, neutral: string): void {
  const ladder = mode === 'light'
    ? (['--bg', '--surface-subtle', '--surface', '--surface-raised'] as const)
    : (['--bg', '--surface', '--surface-subtle', '--surface-raised'] as const)

  for (let index = 1; index < ladder.length; index += 1) {
    const lower = ladder[index - 1]!
    const upper = ladder[index]!
    const step = foundationLightness(tokens[upper]) - foundationLightness(tokens[lower])
    expect(step, `${mode} neutral ${neutral}: ${lower} -> ${upper}`).toBeGreaterThan(0.01)
  }
}

function channelSpread(color: string): number {
  const channels = [
    Number.parseInt(color.slice(1, 3), 16),
    Number.parseInt(color.slice(3, 5), 16),
    Number.parseInt(color.slice(5, 7), 16),
  ]
  return Math.max(...channels) - Math.min(...channels)
}

describe('theme color input contract', () => {
  it('publishes a fixed three-intent color key list', () => {
    expect(THEME_COLOR_KEYS).toEqual(['brand', 'neutral', 'signature'])
    expect(DEFAULT_THEME_COLORS).toEqual({
      brand: '#0c7a60',
      neutral: '#52645f',
      signatureLinked: true,
      signature: '#0c7a60',
    })
  })

  it('offers five distinct editable presets backed only by color intents', () => {
    expect(THEME_COLOR_PRESETS).toHaveLength(5)
    expect(new Set(THEME_COLOR_PRESETS.map((preset) => preset.id))).toHaveProperty('size', 5)
    for (const preset of THEME_COLOR_PRESETS) {
      expect(preset.label).not.toBe('')
      expect(preset.description).not.toBe('')
      expect(normalizeThemeColors(preset.colors)).toEqual(preset.colors)
      expect(preset.colors.signatureLinked).toBe(false)
      expect(preset.colors.signature).not.toBe(preset.colors.brand)
    }
    expect(THEME_COLOR_PRESETS.map((preset) => preset.colors.neutral)).toEqual([
      '#3d5663', '#34465c', '#8a8896', '#25231f', '#54475f',
    ])
  })

  it('normalizes safe short and long hexadecimal colors only', () => {
    expect(normalizeHexColor('#ABC')).toBe('#aabbcc')
    expect(normalizeHexColor('  #Aa10Ff  ')).toBe('#aa10ff')
    for (const invalid of [null, 42, '', 'fff', '#ffff', '#ffffffff', 'red', 'var(--brand)', 'url(x)', '#12;bad']) {
      expect(normalizeHexColor(invalid)).toBeNull()
    }
  })

  it('fills partial or invalid live input from the safe defaults', () => {
    expect(normalizeThemeColors({ brand: '#123', neutral: 'bad', signatureLinked: false })).toEqual({
      brand: '#112233',
      neutral: DEFAULT_THEME_COLORS.neutral,
      signatureLinked: false,
      signature: DEFAULT_THEME_COLORS.signature,
    })
    expect(normalizeThemeColors(null)).toEqual(DEFAULT_THEME_COLORS)
  })

  it('round-trips the explicit version-one storage shape', () => {
    const colors: ThemeColorIntent = {
      brand: '#123456',
      neutral: '#657483',
      signatureLinked: false,
      signature: '#d1a35f',
    }
    const serialized = serializeThemeColors(colors)
    expect(JSON.parse(serialized)).toEqual({ version: 1, ...colors })
    expect(parseStoredThemeColors(serialized)).toEqual(colors)
  })

  it('rejects malformed, oversized, future, incomplete, and injected storage', () => {
    const invalid = [
      null,
      '',
      'null',
      '[]',
      '{',
      JSON.stringify({ version: 2, ...DEFAULT_THEME_COLORS }),
      JSON.stringify({ version: 1, brand: '#fff', neutral: '#000', signatureLinked: 'yes', signature: '#fff' }),
      JSON.stringify({ version: 1, brand: 'var(--x)', neutral: '#000', signatureLinked: true, signature: '#fff' }),
      JSON.stringify({ version: 1, ...DEFAULT_THEME_COLORS, extra: '#ffffff' }),
      ' '.repeat(513),
    ]
    for (const value of invalid) expect(parseStoredThemeColors(value)).toBeNull()
  })

  it('calculates canonical WCAG sRGB contrast and rejects non-colors', () => {
    expect(contrastRatio('#000', '#fff')).toBe(21)
    expect(contrastRatio('#777777', '#ffffff')).toBeCloseTo(4.478, 3)
    expect(() => contrastRatio('red', '#fff')).toThrow(TypeError)
  })
})

describe('derived custom theme', () => {
  it.each(['light', 'dark'] as const)('derives an exact, safe token allowlist in %s mode', (mode) => {
    expectAccessible(deriveThemeTokens(DEFAULT_THEME_COLORS, mode))
  })

  it('keeps every recommended preset accessible in light and dark modes', () => {
    for (const preset of THEME_COLOR_PRESETS) {
      const light = deriveThemeTokens({ ...preset.colors }, 'light')
      const dark = deriveThemeTokens({ ...preset.colors }, 'dark')
      expectAccessible(light)
      expectAccessible(dark)
      expect(light['--theme-accent']).not.toBe(light['--brand'])
      expect(dark['--theme-accent']).not.toBe(dark['--brand'])
    }
  })

  it('keeps recommended dark presets close to black through the dark foundation branch', () => {
    for (const preset of THEME_COLOR_PRESETS) {
      const dark = deriveThemeTokens({ ...preset.colors }, 'dark')
      expect(contrastRatio(dark['--bg'], '#000000')).toBeLessThan(1.12)
      expect(contrastRatio(dark['--surface'], '#000000')).toBeLessThan(1.23)
      expect(contrastRatio(dark['--surface-raised'], '#000000')).toBeLessThan(1.45)
    }
  })

  it('uses one neutral intent for visibly different light and dark foundations', () => {
    const light = deriveThemeTokens(DEFAULT_THEME_COLORS, 'light')
    const dark = deriveThemeTokens(DEFAULT_THEME_COLORS, 'dark')
    expect(light['--bg']).not.toBe(dark['--bg'])
    expect(light['--surface']).not.toBe(dark['--surface'])
    expect(light['--sidebar']).not.toBe(dark['--sidebar'])
    expect(light['--on-brand']).toBe('#ffffff')
    expect(dark['--on-brand']).toBe('#0b111b')
  })

  it('keeps dark foundations close to black without washing out chromatic neutral intent', () => {
    const defaults = deriveThemeTokens(DEFAULT_THEME_COLORS, 'dark')
    const black = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#000000' }, 'dark')
    const blue = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#315d7d' }, 'dark')

    expect(contrastRatio(black['--bg'], '#000000')).toBeLessThan(1.05)
    expect(contrastRatio(black['--surface'], '#000000')).toBeLessThan(1.1)
    expect(black['--desktop-wallpaper-veil-dark']).toBe('linear-gradient(145deg, rgb(14 14 14 / 26%), rgb(6 6 6 / 48%))')
    expect(defaults['--desktop-wallpaper-veil-dark']).toContain('/ 16%')
    expect(defaults['--desktop-wallpaper-veil-dark']).toContain('/ 32%')
    expect(contrastRatio(defaults['--bg'], '#000000')).toBeGreaterThan(contrastRatio(black['--bg'], '#000000'))
    expect(contrastRatio(black['--bg'], '#000000')).toBeLessThan(1.1)
    expect(channelSpread(blue['--bg'])).toBeGreaterThanOrEqual(6)
    expectAccessible(black)
    expectAccessible(blue)
  })

  it('reflects a broader brand and foundation range while preserving safe contrast', () => {
    const blue = { ...DEFAULT_THEME_COLORS, brand: '#0055ff', neutral: '#0055ff' }
    const red = { ...DEFAULT_THEME_COLORS, brand: '#ff2200', neutral: '#ff2200' }
    const blueLight = deriveThemeTokens(blue, 'light')
    const redLight = deriveThemeTokens(red, 'light')
    const blueDark = deriveThemeTokens(blue, 'dark')
    const redDark = deriveThemeTokens(red, 'dark')
    const blackLight = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#000000' }, 'light')
    const whiteLight = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#ffffff' }, 'light')

    expect(channelSpread(blueLight['--sidebar-active'])).toBeGreaterThanOrEqual(60)
    expect(channelSpread(redLight['--sidebar-active'])).toBeGreaterThanOrEqual(60)
    expect(channelSpread(blueDark['--bg'])).toBeGreaterThanOrEqual(24)
    expect(channelSpread(redDark['--bg'])).toBeGreaterThanOrEqual(24)
    expect(blueLight['--brand-soft']).not.toBe(redLight['--brand-soft'])
    expect(contrastRatio(whiteLight['--bg'], '#000000')).toBeGreaterThan(contrastRatio(blackLight['--bg'], '#000000'))
    for (const tokens of [blueLight, redLight, blueDark, redDark, blackLight, whiteLight]) expectAccessible(tokens)
  })

  it('maps foundation brightness and saturation across visibly broad independent ranges', () => {
    const darkLow = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#000000' }, 'dark')
    const darkHigh = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#ffffff' }, 'dark')
    const lightLow = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#000000' }, 'light')
    const lightHigh = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, neutral: '#ffffff' }, 'light')
    const mutedGreen = { ...DEFAULT_THEME_COLORS, neutral: '#50635d' }
    const vividGreen = { ...DEFAULT_THEME_COLORS, neutral: '#00b377' }
    const mutedDark = deriveThemeTokens(mutedGreen, 'dark')
    const vividDark = deriveThemeTokens(vividGreen, 'dark')
    const mutedLight = deriveThemeTokens(mutedGreen, 'light')
    const vividLight = deriveThemeTokens(vividGreen, 'light')

    expect(contrastRatio(darkHigh['--bg'], '#000000') - contrastRatio(darkLow['--bg'], '#000000')).toBeGreaterThan(0.3)
    expect(contrastRatio(lightHigh['--bg'], '#000000') - contrastRatio(lightLow['--bg'], '#000000')).toBeGreaterThan(3)
    expect(channelSpread(vividDark['--bg']) - channelSpread(mutedDark['--bg'])).toBeGreaterThan(12)
    expect(channelSpread(vividDark['--surface-raised']) - channelSpread(mutedDark['--surface-raised'])).toBeGreaterThan(20)
    expect(channelSpread(vividLight['--bg']) - channelSpread(mutedLight['--bg'])).toBeGreaterThan(10)
    for (const tokens of [darkLow, darkHigh, lightLow, lightHigh, mutedDark, vividDark, mutedLight, vividLight]) {
      expectAccessible(tokens)
    }
  })

  it('derives sidebar hover feedback from brand intent instead of neutral text', () => {
    const blue = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, brand: '#315dbe' }, 'dark')
    const coral = deriveThemeTokens({ ...DEFAULT_THEME_COLORS, brand: '#b64f3f' }, 'dark')
    expect(blue['--sidebar']).toBe(coral['--sidebar'])
    expect(blue['--sidebar-text']).toBe(coral['--sidebar-text'])
    expect(blue['--sidebar-hover']).not.toBe(coral['--sidebar-hover'])
  })

  it('links signature to brand without allowing the dormant signature value to leak', () => {
    const first = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      brand: '#315d7d',
      signatureLinked: true,
      signature: '#ff0000',
    }, 'light')
    const second = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      brand: '#315d7d',
      signatureLinked: true,
      signature: '#00ffff',
    }, 'light')
    expect(first).toEqual(second)
    expect(first['--theme-accent']).toBe(first['--sidebar-accent'])
  })

  it('uses an independent signature only for signature surfaces', () => {
    const linked = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      brand: '#315d7d',
      signatureLinked: true,
      signature: '#d0aa65',
    }, 'dark')
    const independent = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      brand: '#315d7d',
      signatureLinked: false,
      signature: '#d0aa65',
    }, 'dark')
    expect(independent['--brand']).toBe(linked['--brand'])
    expect(independent['--theme-accent']).not.toBe(linked['--theme-accent'])
    expect(independent['--theme-accent']).toBe(independent['--sidebar-accent'])
  })

  it('searches accent lightness in both directions for sidebar text and surface marks', () => {
    const light = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      signatureLinked: false,
      signature: '#ffffff',
    }, 'light')
    const dark = deriveThemeTokens({
      ...DEFAULT_THEME_COLORS,
      signatureLinked: false,
      signature: '#000000',
    }, 'dark')

    expect(light['--theme-accent']).not.toBe('#ffffff')
    expect(dark['--theme-accent']).not.toBe('#000000')
    for (const tokens of [light, dark]) {
      expect(contrastRatio(tokens['--theme-accent'], tokens['--sidebar'])).toBeGreaterThanOrEqual(4.5)
      expect(contrastRatio(tokens['--theme-accent'], tokens['--surface'])).toBeGreaterThanOrEqual(3)
      expect(contrastRatio(tokens['--theme-accent'], tokens['--surface-raised'])).toBeGreaterThanOrEqual(3)
    }
  })

  const extremes: ThemeColorIntent[] = [
    { brand: '#000000', neutral: '#000000', signatureLinked: true, signature: '#000000' },
    { brand: '#ffffff', neutral: '#ffffff', signatureLinked: true, signature: '#ffffff' },
    { brand: '#000000', neutral: '#ffffff', signatureLinked: false, signature: '#ffffff' },
    { brand: '#ffffff', neutral: '#000000', signatureLinked: false, signature: '#000000' },
    { brand: '#ff0000', neutral: '#00ff00', signatureLinked: false, signature: '#0000ff' },
    { brand: '#00ffff', neutral: '#ff00ff', signatureLinked: false, signature: '#ffff00' },
    { brand: '#808080', neutral: '#808080', signatureLinked: true, signature: '#808080' },
  ]

  it.each(extremes.flatMap((colors) => [
    { colors, mode: 'light' as const },
    { colors, mode: 'dark' as const },
  ]))('keeps extreme input accessible in $mode mode: $colors', ({ colors, mode }) => {
    expectAccessible(deriveThemeTokens(colors, mode))
  })

  it('stays deterministic and accessible across a seeded random color sample', () => {
    let state = 0x6d2b79f5
    const randomHex = () => {
      state = (Math.imul(state ^ (state >>> 15), 1 | state) + 0x6d2b79f5) | 0
      const value = (state ^ (state >>> 14)) >>> 0
      return `#${(value & 0xffffff).toString(16).padStart(6, '0')}`
    }
    for (let index = 0; index < 96; index += 1) {
      const colors: ThemeColorIntent = {
        brand: randomHex(),
        neutral: randomHex(),
        signatureLinked: index % 2 === 0,
        signature: randomHex(),
      }
      for (const mode of ['light', 'dark'] as const satisfies readonly ThemeMode[]) {
        const first = deriveThemeTokens(colors, mode)
        expect(first).toEqual(deriveThemeTokens(colors, mode))
        expectAccessible(first)
        expectSeparableLadder(first, mode, colors.neutral)
      }
    }
  })
})
