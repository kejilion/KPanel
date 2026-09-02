export const THEME_COLOR_KEYS = ['brand', 'neutral', 'signature'] as const

export type ThemeColorKey = (typeof THEME_COLOR_KEYS)[number]
export type ThemeMode = 'light' | 'dark'

export interface ThemeColorIntent {
  brand: string
  neutral: string
  signatureLinked: boolean
  signature: string
}

export const DEFAULT_THEME_COLORS = Object.freeze({
  brand: '#0c7a60',
  neutral: '#52645f',
  signatureLinked: true,
  signature: '#0c7a60',
}) satisfies Readonly<ThemeColorIntent>

export interface ThemeColorPreset {
  readonly id: string
  readonly label: string
  readonly description: string
  readonly colors: Readonly<ThemeColorIntent>
}

export const THEME_COLOR_PRESETS = Object.freeze([
  {
    id: 'flow-star-blue',
    label: '流光星蓝',
    description: '深海青蓝与微金流光，呼应经典流体壁纸',
    colors: { brand: '#1686a0', neutral: '#3d5663', signatureLinked: false, signature: '#c9973d' },
  },
  {
    id: 'quiet-orbit',
    label: '静海轨迹',
    description: '轨道蓝与冰青光点，呼应深海系统轨迹',
    colors: { brand: '#356fc0', neutral: '#34465c', signatureLinked: false, signature: '#23a6bd' },
  },
  {
    id: 'cloud-dawn',
    label: '云曦霞光',
    description: '晨曦暖霞与雾蓝云层，呼应轻盈地平线',
    colors: { brand: '#b9786b', neutral: '#8a8896', signatureLinked: false, signature: '#d8a95d' },
  },
  {
    id: 'obsidian-gold',
    label: '曜石鎏金',
    description: '曜石黑与香槟金，克制奢华、层次利落',
    colors: { brand: '#b07d22', neutral: '#25231f', signatureLinked: false, signature: '#d9b95f' },
  },
  {
    id: 'twilight-prism',
    label: '暮光棱镜',
    description: '暮光紫与玫瑰折光，呼应柔和玻璃棱面',
    colors: { brand: '#7856b6', neutral: '#54475f', signatureLinked: false, signature: '#c76586' },
  },
] satisfies readonly ThemeColorPreset[])

export const THEME_TOKEN_NAMES = [
  '--bg', '--surface', '--surface-subtle', '--surface-raised',
  '--text', '--text-soft', '--muted', '--border', '--border-strong', '--control-border',
  '--brand', '--brand-strong', '--brand-soft', '--brand-muted', '--theme-accent', '--on-brand',
  '--success', '--success-soft', '--blue', '--blue-soft', '--violet', '--violet-soft',
  '--amber', '--amber-soft', '--danger', '--danger-soft', '--on-danger', '--neutral-soft',
  '--sidebar', '--sidebar-text', '--sidebar-muted', '--sidebar-border', '--sidebar-hover',
  '--sidebar-active', '--sidebar-accent', '--desktop-glass', '--desktop-glass-strong',
  '--desktop-glass-border', '--desktop-label', '--desktop-shadow', '--desktop-wallpaper-base',
  '--desktop-wallpaper-veil-light', '--desktop-wallpaper-veil-dark', '--desktop-wallpaper-vignette',
  '--desktop-aurora-one', '--desktop-aurora-two', '--desktop-aurora-opacity', '--shadow-sm',
  '--shadow-md', '--shadow-button', '--shadow-feature', '--scrollbar-track', '--scrollbar-thumb',
  '--scrollbar-thumb-hover', '--scrollbar-thumb-active',
] as const

export type ThemeTokenName = (typeof THEME_TOKEN_NAMES)[number]
export type ThemeTokenMap = Record<ThemeTokenName, string>

interface Rgb {
  r: number
  g: number
  b: number
}

interface Hsl {
  h: number
  s: number
  l: number
}

interface AccentPair {
  color: string
  soft: string
}

const STORAGE_VERSION = 1
const MAX_STORED_THEME_LENGTH = 512
const AA_CONTRAST = 4.55
const UI_CONTRAST = 3.05
const SEARCH_STEPS = 320
const WHITE = '#ffffff'
const DARK_ON_COLOR = '#0b111b'
const DARK_FOUNDATION_LIGHTNESS = Object.freeze({
  background: 0.04,
  surface: 0.065,
  subtle: 0.09,
  raised: 0.125,
  neutralSoft: 0.105,
})
// The light foundation ladder, measured off the shipped palette in themes.css so
// custom colors land on the same rhythm: page, subtle, surface, raised. `span` is
// page -> raised in HSL lightness; the two middle values are positions inside it.
const LIGHT_LADDER = Object.freeze({
  span: 0.055,
  subtle: 0.393,
  surface: 0.75,
})

const HEX_TOKEN_NAMES = new Set<ThemeTokenName>([
  '--bg', '--surface', '--surface-subtle', '--surface-raised',
  '--text', '--text-soft', '--muted', '--border', '--border-strong', '--control-border',
  '--brand', '--brand-strong', '--brand-soft', '--brand-muted', '--theme-accent', '--on-brand',
  '--success', '--success-soft', '--blue', '--blue-soft', '--violet', '--violet-soft',
  '--amber', '--amber-soft', '--danger', '--danger-soft', '--on-danger', '--neutral-soft',
  '--sidebar', '--sidebar-text', '--sidebar-muted', '--sidebar-border', '--sidebar-hover',
  '--sidebar-active', '--sidebar-accent', '--desktop-label', '--scrollbar-track', '--scrollbar-thumb',
  '--scrollbar-thumb-hover', '--scrollbar-thumb-active',
])

const STATUS_SEEDS = {
  success: '#247149',
  blue: '#315f95',
  violet: '#6756a5',
  amber: '#875813',
  danger: '#a13f49',
} as const

export function normalizeHexColor(value: unknown): string | null {
  if (typeof value !== 'string') return null
  const normalized = value.trim().toLowerCase()
  const short = normalized.match(/^#([0-9a-f]{3})$/)
  if (short) {
    const digits = short[1] ?? ''
    const [red = '', green = '', blue = ''] = digits
    return `#${red}${red}${green}${green}${blue}${blue}`
  }
  return /^#[0-9a-f]{6}$/.test(normalized) ? normalized : null
}

export function normalizeThemeColors(
  partial: Partial<ThemeColorIntent> | null | undefined,
): ThemeColorIntent {
  const candidate = partial ?? {}
  return {
    brand: normalizeHexColor(candidate.brand) ?? DEFAULT_THEME_COLORS.brand,
    neutral: normalizeHexColor(candidate.neutral) ?? DEFAULT_THEME_COLORS.neutral,
    signatureLinked: typeof candidate.signatureLinked === 'boolean'
      ? candidate.signatureLinked
      : DEFAULT_THEME_COLORS.signatureLinked,
    signature: normalizeHexColor(candidate.signature) ?? DEFAULT_THEME_COLORS.signature,
  }
}

export function parseStoredThemeColors(raw: string | null): ThemeColorIntent | null {
  if (raw === null || raw.length > MAX_STORED_THEME_LENGTH) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return null
  }
  if (!isPlainRecord(parsed)) return null
  const allowedKeys = new Set(['version', 'brand', 'neutral', 'signatureLinked', 'signature'])
  if (Object.keys(parsed).some((key) => !allowedKeys.has(key))) return null
  if (parsed.version !== STORAGE_VERSION || typeof parsed.signatureLinked !== 'boolean') return null
  const brand = normalizeHexColor(parsed.brand)
  const neutral = normalizeHexColor(parsed.neutral)
  const signature = normalizeHexColor(parsed.signature)
  if (!brand || !neutral || !signature) return null
  return { brand, neutral, signatureLinked: parsed.signatureLinked, signature }
}

export function serializeThemeColors(colors: ThemeColorIntent): string {
  const normalized = normalizeThemeColors(colors)
  return JSON.stringify({ version: STORAGE_VERSION, ...normalized })
}

export function contrastRatio(first: string, second: string): number {
  const left = normalizeHexColor(first)
  const right = normalizeHexColor(second)
  if (!left || !right) throw new TypeError('contrastRatio expects hexadecimal colors')
  const leftLuminance = relativeLuminance(parseRgb(left))
  const rightLuminance = relativeLuminance(parseRgb(right))
  return (Math.max(leftLuminance, rightLuminance) + 0.05)
    / (Math.min(leftLuminance, rightLuminance) + 0.05)
}

/**
 * Derive a complete, trusted token map from three user-controlled color intents.
 * User strings never become CSS directly: colors are normalized first, neutral
 * chroma is capped, and every semantic foreground is searched against its real
 * backgrounds using WCAG sRGB contrast before the map is returned.
 */
export function deriveThemeTokens(colors: ThemeColorIntent, mode: ThemeMode): ThemeTokenMap {
  const normalized = normalizeThemeColors(colors)
  const neutralIntentLightness = rgbToHsl(parseRgb(normalized.neutral)).l
  const defaultNeutralLightness = rgbToHsl(parseRgb(DEFAULT_THEME_COLORS.neutral)).l
  const tonePosition = neutralIntentLightness < defaultNeutralLightness
    ? (neutralIntentLightness - defaultNeutralLightness) / defaultNeutralLightness
    : (neutralIntentLightness - defaultNeutralLightness) / (1 - defaultNeutralLightness)
  // A lighter neutral intent cannot raise the light foundation — the ladder is
  // already hung from white — so its expressive range lives entirely on the
  // darkening side and is scaled to match.
  const lightToneOffset = mode === 'light'
    ? tonePosition * (tonePosition < 0 ? 0.1 : 0.04)
    : 0
  const darkToneOffset = mode === 'dark'
    ? tonePosition * (tonePosition < 0 ? 0.05 : 0.09)
    : 0
  const foundationToneOffset = mode === 'light' ? lightToneOffset : darkToneOffset
  const darkeningStrength = mode === 'dark' ? Math.max(-tonePosition, 0) : 0
  const lighteningStrength = mode === 'dark' ? Math.max(tonePosition, 0) : 0
  const neutralSeed = expandSaturation(
    normalized.neutral,
    mode === 'light' ? 0.72 : 0.90,
    mode === 'light' ? 0.45 : 0.70,
  )
  const direction = mode === 'light' ? 'darker' : 'lighter'
  const contentContrastAnchor = mode === 'light' ? '#000000' : WHITE
  const safeBackground = (candidate: string, target = 7) => accessibleColor(
    candidate,
    [contentContrastAnchor],
    target,
    mode === 'light' ? 'lighter' : 'darker',
  )
  // Four separable light steps: page < subtle < surface < raised. The ladder
  // hangs from the raised step and keeps a fixed span, so every neutral intent
  // gets the same rhythm. Only a darkening intent moves it: pinning the top at
  // white for lighter intents both matches the shipped palette and keeps the
  // accent solver a pure-white background to find a mid-tone against, while a
  // ladder that stretched upward would push two steps into the lightness clamp
  // at 1 and merge them into the same flat white.
  const lightFoundation = mode === 'light'
    ? (() => {
        const raised = 1 + Math.min(lightToneOffset, 0)
        const base = raised - LIGHT_LADDER.span
        return {
          background: base,
          subtle: base + LIGHT_LADDER.span * LIGHT_LADDER.subtle,
          surface: base + LIGHT_LADDER.span * LIGHT_LADDER.surface,
          raised,
        }
      })()
    : null
  const bg = safeBackground(setLightness(neutralSeed, lightFoundation ? lightFoundation.background : DARK_FOUNDATION_LIGHTNESS.background + darkToneOffset))
  const surface = safeBackground(setLightness(neutralSeed, lightFoundation ? lightFoundation.surface : DARK_FOUNDATION_LIGHTNESS.surface + darkToneOffset))
  const surfaceSubtle = safeBackground(setLightness(neutralSeed, lightFoundation ? lightFoundation.subtle : DARK_FOUNDATION_LIGHTNESS.subtle + darkToneOffset))
  const surfaceRaised = safeBackground(setLightness(neutralSeed, lightFoundation ? lightFoundation.raised : DARK_FOUNDATION_LIGHTNESS.raised + darkToneOffset))
  const neutralSoft = safeBackground(
    setLightness(neutralSeed, mode === 'light' ? 0.925 + lightToneOffset : DARK_FOUNDATION_LIGHTNESS.neutralSoft + darkToneOffset),
    AA_CONTRAST,
  )
  const contentBackgrounds = [bg, surface, surfaceSubtle, surfaceRaised]

  const text = accessibleColor(
    setLightness(capSaturation(neutralSeed, 0.14), mode === 'light' ? 0.14 : 0.94),
    contentBackgrounds,
    7,
    direction,
  )
  const textSoft = accessibleColor(
    setLightness(capSaturation(neutralSeed, 0.14), mode === 'light' ? 0.31 : 0.80),
    contentBackgrounds,
    7,
    direction,
  )
  const muted = accessibleColor(
    setLightness(capSaturation(neutralSeed, 0.12), mode === 'light' ? 0.43 : 0.66),
    [...contentBackgrounds, neutralSoft],
    AA_CONTRAST,
    direction,
  )

  const border = mix(surface, text, mode === 'light' ? 0.11 : 0.12)
  const borderStrong = mix(surface, text, mode === 'light' ? 0.20 : 0.21)
  const controlBorder = accessibleColor(
    mix(surface, text, mode === 'light' ? 0.34 : 0.32),
    [surface, surfaceRaised],
    UI_CONTRAST,
    direction,
  )

  const brandSeed = capSaturation(normalized.brand, 0.94)
  const brand = deriveActionColor(brandSeed, surface, mode)
  const brandSoft = mix(surface, brand, mode === 'light' ? 0.13 : 0.23)
  const onBrand = mode === 'light' ? WHITE : DARK_ON_COLOR
  const strongSeed = shiftLightness(brand, mode === 'light' ? -0.10 : 0.10)
  const brandStrong = accessibleColor(
    strongSeed,
    [surface, brandSoft],
    AA_CONTRAST,
    direction,
    (candidate) => contrastRatio(onBrand, candidate) >= AA_CONTRAST,
  )
  const brandMuted = mix(surface, brand, mode === 'light' ? 0.30 : 0.38)

  const success = deriveAccentPair(STATUS_SEEDS.success, surface, mode)
  const blue = deriveAccentPair(STATUS_SEEDS.blue, surface, mode)
  const violet = deriveAccentPair(STATUS_SEEDS.violet, surface, mode)
  const amber = deriveAccentPair(STATUS_SEEDS.amber, surface, mode)
  const danger = deriveAccentPair(STATUS_SEEDS.danger, surface, mode, true)
  const onDanger = mode === 'light' ? WHITE : DARK_ON_COLOR

  const sidebarSeed = neutralSeed
  const sidebar = setLightness(sidebarSeed, (mode === 'light' ? 0.095 : 0.045) + foundationToneOffset / 2)
  const sidebarText = accessibleColor(
    setLightness(sidebarSeed, 0.96),
    [sidebar],
    AA_CONTRAST,
    'lighter',
  )
  const sidebarMuted = accessibleColor(
    setLightness(capSaturation(sidebarSeed, 0.16), mode === 'light' ? 0.70 : 0.66),
    [sidebar],
    AA_CONTRAST,
    'lighter',
  )
  const signatureIntent = normalized.signatureLinked ? normalized.brand : normalized.signature
  const signature = closestAccessibleColor(
    capSaturation(signatureIntent, 0.72),
    [
      { background: sidebar, minimum: AA_CONTRAST },
      { background: surface, minimum: UI_CONTRAST },
      { background: surfaceRaised, minimum: UI_CONTRAST },
    ],
  )
  const sidebarBorder = mix(sidebar, sidebarText, mode === 'light' ? 0.10 : 0.09)
  const sidebarHover = mix(sidebar, brand, mode === 'light' ? 0.15 : 0.14)
  const sidebarActive = mix(sidebar, brand, mode === 'light' ? 0.28 : 0.27)

  const glassAlpha = safeGlassAlpha(surface, text, mode === 'light' ? 0.90 : 0.82)
  const strongGlassAlpha = safeGlassAlpha(surfaceRaised, text, mode === 'light' ? 0.96 : 0.93)
  const desktopGlass = cssRgb(surface, glassAlpha)
  const desktopGlassStrong = cssRgb(surfaceRaised, strongGlassAlpha)
  const desktopGlassBorder = cssRgb(mode === 'light' ? text : sidebarText, mode === 'light' ? 0.18 : 0.12)
  const desktopShadow = cssRgb(mode === 'light' ? sidebar : '#000000', mode === 'light' ? 0.22 : 0.48)

  const wallpaperStart = setLightness(neutralSeed, (mode === 'light' ? 0.72 : 0.055) + foundationToneOffset)
  const wallpaperEnd = setLightness(neutralSeed, (mode === 'light' ? 0.92 : 0.16) + foundationToneOffset)
  const lightVeilStart = setLightness(neutralSeed, 0.94)
  const lightVeilEnd = setLightness(neutralSeed, 0.72)
  const darkVeilStart = setLightness(neutralSeed, 0.055)
  const darkVeilEnd = setLightness(neutralSeed, 0.025)
  const vignetteEdge = setLightness(neutralSeed, 0.04)
  const desktopWallpaperBase = `linear-gradient(145deg, ${wallpaperStart}, ${wallpaperEnd})`
  const desktopWallpaperVeilLight = `linear-gradient(145deg, ${cssRgb(lightVeilStart, 0.07)}, ${cssRgb(lightVeilEnd, 0.04)})`
  const desktopWallpaperVeilDark = `linear-gradient(145deg, ${cssRgb(darkVeilStart, 0.16 + darkeningStrength * 0.10 - lighteningStrength * 0.05)}, ${cssRgb(darkVeilEnd, 0.32 + darkeningStrength * 0.16 - lighteningStrength * 0.10)})`
  const desktopWallpaperVignette = [
    `linear-gradient(90deg, ${cssRgb(vignetteEdge, mode === 'light' ? 0.06 : 0.12)}, transparent 30%, transparent 80%, ${cssRgb(vignetteEdge, mode === 'light' ? 0.05 : 0.08)})`,
    `radial-gradient(circle at 50% 40%, transparent ${mode === 'light' ? 42 : 36}%, ${cssRgb(vignetteEdge, mode === 'light' ? 0.08 : 0.12)} 100%)`,
  ].join(', ')
  const desktopAuroraOne = `radial-gradient(circle, ${cssRgb(brand, 0.24)}, transparent 67%)`
  const desktopAuroraTwo = `radial-gradient(circle, ${cssRgb(signature, 0.18)}, transparent 68%)`

  const shadowColor = mode === 'light' ? sidebar : '#000000'
  // Mirrors the shadow model in themes.css: one contact shadow for small
  // elevation, a second diffuse layer only for overlays.
  const shadowSm = mode === 'light'
    ? `0 1px 2px ${cssRgb(shadowColor, 0.07)}`
    : `0 1px 2px ${cssRgb(shadowColor, 0.24)}`
  const shadowMd = mode === 'light'
    ? `0 1px 2px ${cssRgb(shadowColor, 0.09)}, 0 10px 28px ${cssRgb(shadowColor, 0.11)}`
    : `0 1px 2px ${cssRgb(shadowColor, 0.3)}, 0 12px 32px ${cssRgb(shadowColor, 0.34)}`
  const shadowButton = `0 1px 2px ${cssRgb(mode === 'light' ? brand : shadowColor, mode === 'light' ? 0.16 : 0.24)}`
  const shadowFeature = `0 2px 6px ${cssRgb(shadowColor, mode === 'light' ? 0.07 : 0.2)}`

  const scrollbarTrack = neutralSoft
  const scrollbarThumb = mix(surface, text, mode === 'light' ? 0.28 : 0.27)
  const scrollbarThumbHover = mix(surface, text, mode === 'light' ? 0.43 : 0.42)

  const tokens = {
    '--bg': bg,
    '--surface': surface,
    '--surface-subtle': surfaceSubtle,
    '--surface-raised': surfaceRaised,
    '--text': text,
    '--text-soft': textSoft,
    '--muted': muted,
    '--border': border,
    '--border-strong': borderStrong,
    '--control-border': controlBorder,
    '--brand': brand,
    '--brand-strong': brandStrong,
    '--brand-soft': brandSoft,
    '--brand-muted': brandMuted,
    '--theme-accent': signature,
    '--on-brand': onBrand,
    '--success': success.color,
    '--success-soft': success.soft,
    '--blue': blue.color,
    '--blue-soft': blue.soft,
    '--violet': violet.color,
    '--violet-soft': violet.soft,
    '--amber': amber.color,
    '--amber-soft': amber.soft,
    '--danger': danger.color,
    '--danger-soft': danger.soft,
    '--on-danger': onDanger,
    '--neutral-soft': neutralSoft,
    '--sidebar': sidebar,
    '--sidebar-text': sidebarText,
    '--sidebar-muted': sidebarMuted,
    '--sidebar-border': sidebarBorder,
    '--sidebar-hover': sidebarHover,
    '--sidebar-active': sidebarActive,
    '--sidebar-accent': signature,
    '--desktop-glass': desktopGlass,
    '--desktop-glass-strong': desktopGlassStrong,
    '--desktop-glass-border': desktopGlassBorder,
    '--desktop-label': text,
    '--desktop-shadow': desktopShadow,
    '--desktop-wallpaper-base': desktopWallpaperBase,
    '--desktop-wallpaper-veil-light': desktopWallpaperVeilLight,
    '--desktop-wallpaper-veil-dark': desktopWallpaperVeilDark,
    '--desktop-wallpaper-vignette': desktopWallpaperVignette,
    '--desktop-aurora-one': desktopAuroraOne,
    '--desktop-aurora-two': desktopAuroraTwo,
    '--desktop-aurora-opacity': mode === 'light' ? '0.1' : '0.16',
    '--shadow-sm': shadowSm,
    '--shadow-md': shadowMd,
    '--shadow-button': shadowButton,
    '--shadow-feature': shadowFeature,
    '--scrollbar-track': scrollbarTrack,
    '--scrollbar-thumb': scrollbarThumb,
    '--scrollbar-thumb-hover': scrollbarThumbHover,
    '--scrollbar-thumb-active': brand,
  } satisfies ThemeTokenMap

  assertSafeTokenMap(tokens)
  return tokens
}

function isPlainRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object'
    && value !== null
    && !Array.isArray(value)
    && Object.getPrototypeOf(value) === Object.prototype
}

function parseRgb(color: string): Rgb {
  return {
    r: Number.parseInt(color.slice(1, 3), 16),
    g: Number.parseInt(color.slice(3, 5), 16),
    b: Number.parseInt(color.slice(5, 7), 16),
  }
}

function toHex({ r, g, b }: Rgb): string {
  const channel = (value: number) => Math.round(clamp(value, 0, 255)).toString(16).padStart(2, '0')
  return `#${channel(r)}${channel(g)}${channel(b)}`
}

function rgbToHsl({ r, g, b }: Rgb): Hsl {
  const red = r / 255
  const green = g / 255
  const blue = b / 255
  const maximum = Math.max(red, green, blue)
  const minimum = Math.min(red, green, blue)
  const delta = maximum - minimum
  const lightness = (maximum + minimum) / 2
  if (delta === 0) return { h: 0, s: 0, l: lightness }
  const saturation = delta / (1 - Math.abs(2 * lightness - 1))
  const sector = maximum === red
    ? ((green - blue) / delta) % 6
    : maximum === green
      ? (blue - red) / delta + 2
      : (red - green) / delta + 4
  return { h: (sector * 60 + 360) % 360, s: saturation, l: lightness }
}

function hslToRgb({ h, s, l }: Hsl): Rgb {
  const chroma = (1 - Math.abs(2 * l - 1)) * s
  const sector = ((h % 360) + 360) % 360 / 60
  const intermediate = chroma * (1 - Math.abs((sector % 2) - 1))
  let red = 0
  let green = 0
  let blue = 0
  if (sector < 1) [red, green] = [chroma, intermediate]
  else if (sector < 2) [red, green] = [intermediate, chroma]
  else if (sector < 3) [green, blue] = [chroma, intermediate]
  else if (sector < 4) [green, blue] = [intermediate, chroma]
  else if (sector < 5) [red, blue] = [intermediate, chroma]
  else [red, blue] = [chroma, intermediate]
  const offset = l - chroma / 2
  return { r: (red + offset) * 255, g: (green + offset) * 255, b: (blue + offset) * 255 }
}

function capSaturation(color: string, maximum: number): string {
  const hsl = rgbToHsl(parseRgb(color))
  return toHex(hslToRgb({ ...hsl, s: Math.min(hsl.s, maximum) }))
}

function expandSaturation(color: string, maximum: number, boost: number): string {
  const hsl = rgbToHsl(parseRgb(color))
  const expanded = hsl.s * (1 + boost * (1 - hsl.s))
  return toHex(hslToRgb({ ...hsl, s: Math.min(expanded, maximum) }))
}

function setLightness(color: string, lightness: number): string {
  const hsl = rgbToHsl(parseRgb(color))
  return toHex(hslToRgb({ ...hsl, l: clamp(lightness, 0, 1) }))
}

function shiftLightness(color: string, amount: number): string {
  const hsl = rgbToHsl(parseRgb(color))
  return toHex(hslToRgb({ ...hsl, l: clamp(hsl.l + amount, 0, 1) }))
}

function mix(first: string, second: string, amount: number): string {
  const left = parseRgb(first)
  const right = parseRgb(second)
  const ratio = clamp(amount, 0, 1)
  return toHex({
    r: left.r + (right.r - left.r) * ratio,
    g: left.g + (right.g - left.g) * ratio,
    b: left.b + (right.b - left.b) * ratio,
  })
}

function accessibleColor(
  seed: string,
  backgrounds: readonly string[],
  minimum: number,
  direction: 'darker' | 'lighter',
  extraCheck: (candidate: string) => boolean = () => true,
): string {
  const passes = (candidate: string) => backgrounds.every(
    (background) => contrastRatio(candidate, background) >= minimum,
  ) && extraCheck(candidate)
  if (passes(seed)) return seed
  const hsl = rgbToHsl(parseRgb(seed))
  const target = direction === 'darker' ? 0 : 1
  let previous = seed
  for (let step = 1; step <= SEARCH_STEPS; step += 1) {
    const progress = step / SEARCH_STEPS
    const candidate = toHex(hslToRgb({ ...hsl, l: hsl.l + (target - hsl.l) * progress }))
    if (candidate === previous) continue
    previous = candidate
    if (passes(candidate)) return candidate
  }
  const extreme = direction === 'darker' ? '#000000' : '#ffffff'
  if (passes(extreme)) return extreme
  throw new Error('Unable to derive an accessible theme color')
}

function closestAccessibleColor(
  seed: string,
  requirements: ReadonlyArray<{ background: string; minimum: number }>,
): string {
  const passes = (candidate: string) => requirements.every(
    ({ background, minimum }) => contrastRatio(candidate, background) >= minimum,
  )
  if (passes(seed)) return seed
  const hsl = rgbToHsl(parseRgb(seed))
  let best: { color: string; distance: number } | undefined
  let previous = ''
  for (let step = 0; step <= SEARCH_STEPS; step += 1) {
    const lightness = step / SEARCH_STEPS
    const candidate = toHex(hslToRgb({ ...hsl, l: lightness }))
    if (candidate === previous) continue
    previous = candidate
    if (!passes(candidate)) continue
    const distance = Math.abs(lightness - hsl.l)
    if (!best || distance < best.distance) best = { color: candidate, distance }
  }
  if (best) return best.color
  // A very light neutral intent yields a near-black sidebar and near-white
  // surfaces at once, leaving the accent a window only a fraction of one
  // lightness step wide. Desaturating widens it: the accent gives up some of
  // the user's hue, which is the right trade against losing legibility on one
  // of the two backgrounds.
  for (const saturation of [0.7, 0.5, 0.35, 0.2, 0.1, 0]) {
    for (let step = 0; step <= SEARCH_STEPS; step += 1) {
      const candidate = toHex(hslToRgb({ ...hsl, s: hsl.s * saturation, l: step / SEARCH_STEPS }))
      if (passes(candidate)) return candidate
    }
  }
  throw new Error('Unable to derive an accessible theme accent')
}

function deriveActionColor(seed: string, surface: string, mode: ThemeMode): string {
  const direction = mode === 'light' ? 'darker' : 'lighter'
  const onColor = mode === 'light' ? WHITE : DARK_ON_COLOR
  const softAmount = mode === 'light' ? 0.13 : 0.23
  return accessibleColor(
    seed,
    [surface],
    AA_CONTRAST,
    direction,
    (candidate) => contrastRatio(candidate, mix(surface, candidate, softAmount)) >= AA_CONTRAST
      && contrastRatio(onColor, candidate) >= AA_CONTRAST,
  )
}

function deriveAccentPair(
  seed: string,
  surface: string,
  mode: ThemeMode,
  requireOnColor = false,
): AccentPair {
  const direction = mode === 'light' ? 'darker' : 'lighter'
  const softAmount = mode === 'light' ? 0.10 : 0.18
  const onColor = mode === 'light' ? WHITE : DARK_ON_COLOR
  const prepared = capSaturation(seed, 0.74)
  const color = accessibleColor(
    prepared,
    [surface],
    AA_CONTRAST,
    direction,
    (candidate) => contrastRatio(candidate, mix(surface, candidate, softAmount)) >= AA_CONTRAST
      && (!requireOnColor || contrastRatio(onColor, candidate) >= AA_CONTRAST),
  )
  return { color, soft: mix(surface, color, softAmount) }
}

function relativeLuminance({ r, g, b }: Rgb): number {
  const linear = [r, g, b].map((channel) => {
    const normalized = channel / 255
    return normalized <= 0.04045
      ? normalized / 12.92
      : ((normalized + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * (linear[0] ?? 0) + 0.7152 * (linear[1] ?? 0) + 0.0722 * (linear[2] ?? 0)
}

function composite(foreground: string, background: string, alpha: number): string {
  return mix(background, foreground, alpha)
}

function safeGlassAlpha(base: string, label: string, initial: number): number {
  for (let alpha = initial; alpha <= 1.0001; alpha += 0.01) {
    const rounded = Math.min(1, Number(alpha.toFixed(2)))
    const againstBlack = composite(base, '#000000', rounded)
    const againstWhite = composite(base, '#ffffff', rounded)
    if (contrastRatio(label, againstBlack) >= AA_CONTRAST
      && contrastRatio(label, againstWhite) >= AA_CONTRAST) return rounded
  }
  return 1
}

function cssRgb(color: string, alpha: number): string {
  const { r, g, b } = parseRgb(color)
  const percentage = Number((clamp(alpha, 0, 1) * 100).toFixed(1))
  return `rgb(${r} ${g} ${b} / ${percentage}%)`
}

function assertSafeTokenMap(tokens: ThemeTokenMap): void {
  const names = Object.keys(tokens)
  if (names.length !== THEME_TOKEN_NAMES.length
    || THEME_TOKEN_NAMES.some((name) => !Object.hasOwn(tokens, name))) {
    throw new Error('Derived theme token allowlist mismatch')
  }
  for (const [name, value] of Object.entries(tokens) as Array<[ThemeTokenName, string]>) {
    if (!value || /[;{}]|url\(|var\(|(?:^|\W)(?:NaN|undefined)(?:$|\W)/i.test(value)) {
      throw new Error(`Unsafe derived theme token: ${name}`)
    }
    if (HEX_TOKEN_NAMES.has(name) && !normalizeHexColor(value)) {
      throw new Error(`Expected hexadecimal derived theme token: ${name}`)
    }
  }

  const aaPairs: Array<[ThemeTokenName, ThemeTokenName]> = [
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
  ]
  for (const [foreground, background] of aaPairs) {
    if (contrastRatio(tokens[foreground], tokens[background]) < 4.5) {
      throw new Error(`Derived theme contrast failure: ${foreground}/${background}`)
    }
  }
  for (const background of ['--surface', '--surface-raised'] as const) {
    if (contrastRatio(tokens['--control-border'], tokens[background]) < 3) {
      throw new Error(`Derived theme control boundary failure: ${background}`)
    }
  }
  for (const background of ['--surface', '--surface-raised'] as const) {
    if (contrastRatio(tokens['--theme-accent'], tokens[background]) < 3) {
      throw new Error(`Derived theme accent boundary failure: ${background}`)
    }
  }
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value))
}
