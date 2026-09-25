/**
 * Desktop 3D scene packs: separately downloadable wallpaper bundles. A pack is
 * a self-contained web page (its own WebGL engine) that the desktop runs in a
 * sandboxed, opaque-origin iframe, so pack code can never reach the panel
 * session, storage or DOM. The desktop and the pack only exchange the
 * rendering commands below via postMessage.
 */
import type { ThemeColorIntent } from '@/theme/colors'

export const SCENE_PACK_ID = /^[a-z0-9][a-z0-9-]{0,39}$/
export const SCENE_PACK_WALLPAPER_PREFIX = 'pack:'

export type LocalizedText = Partial<Record<'zh-CN' | 'zh-TW' | 'en-US', string>>

/** The panel accent colors a scene comes with: one scheme per scene, applied when it is chosen. */
export interface ScenePackTheme {
  brand: string
  neutral: string
  signature: string
}

export interface ScenePackCamera {
  id: string
  name: LocalizedText
}

/** Where the panel downloads packs from: GitHub raw, the gh.kejilion.pro mirror, or direct-then-mirror. */
export type ScenePackSource = 'auto' | 'github' | 'mirror'

export interface ScenePack {
  resourceVersion: string
  id: string
  version: string
  name: LocalizedText
  description: LocalizedText
  author?: { name: string, url?: string }
  license?: string
  tags: string[]
  theme?: ScenePackTheme
  cameras: ScenePackCamera[]
  sizeBytes: number
  installed: boolean
  installedVersion: string | null
  /** Same-origin URL prefix of the installed files; the pack page is loaded from here. */
  fileBase: string | null
}

export interface ScenePackList {
  warning?: string
  source: ScenePackSource
  sources: ScenePackSource[]
  packs: ScenePack[]
}

/** Packs published by the KPanel maintainers; everything else is community work. */
export function isOfficialScenePack(pack: Pick<ScenePack, 'author'>): boolean {
  return pack.author?.name === 'KPanel'
}

/** Only an installed scene capability path may become a frame URL. */
export function scenePackPageURL(pack: Pick<ScenePack, 'fileBase'>): string | undefined {
  const base = pack.fileBase
  if (!base || !/^\/api\/v1\/desktop\/scene-packs\/[a-z0-9][a-z0-9-]{0,39}\/files\/[a-f0-9]{32}\/$/.test(base)) return undefined
  return `${base}index.html`
}

const HEX_COLOR = /^#[0-9a-f]{6}$/i

/** The scene's theme as panel colors; only plain #rrggbb values are accepted. */
export function scenePackThemeColors(pack: Pick<ScenePack, 'theme'>): ThemeColorIntent | undefined {
  const theme = pack.theme
  if (!theme || ![theme.brand, theme.neutral, theme.signature].every((color) => typeof color === 'string' && HEX_COLOR.test(color))) return undefined
  return { brand: theme.brand.toLowerCase(), neutral: theme.neutral.toLowerCase(), signatureLinked: false, signature: theme.signature.toLowerCase() }
}

export function isScenePackID(value: unknown): value is string {
  return typeof value === 'string' && SCENE_PACK_ID.test(value)
}

/** `pack:<id>` in the desktop wallpaper key, or undefined for anything else. */
export function scenePackFromWallpaper(value: string | null | undefined): string | undefined {
  if (!value?.startsWith(SCENE_PACK_WALLPAPER_PREFIX)) return undefined
  const id = value.slice(SCENE_PACK_WALLPAPER_PREFIX.length)
  return isScenePackID(id) ? id : undefined
}

export function scenePackWallpaper(id: string): string {
  return `${SCENE_PACK_WALLPAPER_PREFIX}${id}`
}

export function localizedText(text: LocalizedText, locale: string): string {
  return text[locale as keyof LocalizedText] ?? text['zh-CN'] ?? text['en-US'] ?? Object.values(text)[0] ?? ''
}

export function formatPackSize(bytes: number): string {
  if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${Math.max(1, Math.round(bytes / 1024))} KB`
}

export type ScenePackCommand =
  | { source: 'kpanel-desktop', type: 'pause' }
  | { source: 'kpanel-desktop', type: 'resume' }
  | { source: 'kpanel-desktop', type: 'camera', index?: number }

export type ScenePackEvent =
  | { source: 'kpanel-scene-pack', type: 'ready', cameras: string[] }
  | { source: 'kpanel-scene-pack', type: 'camera', index: number }
  | { source: 'kpanel-scene-pack', type: 'error', reason: string }

/** Accepts only well-formed events; anything else from the sandbox is ignored. */
export function parseScenePackEvent(data: unknown): ScenePackEvent | undefined {
  if (!data || typeof data !== 'object') return undefined
  const event = data as Record<string, unknown>
  if (event.source !== 'kpanel-scene-pack') return undefined
  if (event.type === 'ready' && Array.isArray(event.cameras) && event.cameras.length <= 12 && event.cameras.every((camera) => typeof camera === 'string' && camera.length <= 40)) {
    return { source: 'kpanel-scene-pack', type: 'ready', cameras: event.cameras as string[] }
  }
  if (event.type === 'camera' && Number.isInteger(event.index) && (event.index as number) >= 0 && (event.index as number) < 12) {
    return { source: 'kpanel-scene-pack', type: 'camera', index: event.index as number }
  }
  if (event.type === 'error' && typeof event.reason === 'string') {
    return { source: 'kpanel-scene-pack', type: 'error', reason: event.reason.slice(0, 80) }
  }
  return undefined
}
