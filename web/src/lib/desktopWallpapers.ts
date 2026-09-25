import { readonly, ref } from 'vue'
import { api } from '@/lib/api'
import { rememberFileBase, scenePackFromWallpaper, scenePackThemeColors, type ScenePack } from '@/lib/scenePacks'
import type { CustomWallpaper, CustomWallpaperList } from '@/types/api'
import { useTheme } from '@/stores/theme'
import { THEME_COLOR_PRESETS, type ThemeColorIntent } from '@/theme/colors'

/**
 * The one wallpaper choice shared by desktop mode, classic mode and Settings. Choosing a
 * wallpaper also applies its colour scheme. public/appearance-init.js reads the same key
 * before the app starts.
 */
export const DESKTOP_WALLPAPER_KEY = 'kpanel:desktop-wallpaper:v1'

export const DESKTOP_WALLPAPERS = [
  {
    id: 'classic',
    src: '/wallpapers/kpanel-desktop.webp',
    nameKey: 'desktop.wallpaperClassic',
    descriptionKey: 'desktop.wallpaperClassicDescription',
    themePreset: THEME_COLOR_PRESETS[0]!,
  },
  {
    id: 'orbit',
    src: '/wallpapers/kpanel-desktop-orbit.webp',
    nameKey: 'desktop.wallpaperOrbit',
    descriptionKey: 'desktop.wallpaperOrbitDescription',
    themePreset: THEME_COLOR_PRESETS[1]!,
  },
  {
    id: 'horizon',
    src: '/wallpapers/kpanel-desktop-horizon.webp',
    nameKey: 'desktop.wallpaperHorizon',
    descriptionKey: 'desktop.wallpaperHorizonDescription',
    themePreset: THEME_COLOR_PRESETS[2]!,
  },
  {
    id: 'rift',
    src: '/wallpapers/kpanel-desktop-rift.webp',
    nameKey: 'desktop.wallpaperRift',
    descriptionKey: 'desktop.wallpaperRiftDescription',
    themePreset: THEME_COLOR_PRESETS[3]!,
  },
  {
    id: 'prism',
    src: '/wallpapers/kpanel-desktop-prism.webp',
    nameKey: 'desktop.wallpaperPrism',
    descriptionKey: 'desktop.wallpaperPrismDescription',
    themePreset: THEME_COLOR_PRESETS[4]!,
  },
] as const

export type DesktopStaticWallpaperID = typeof DESKTOP_WALLPAPERS[number]['id']
// Installed 3D scene packs share the wallpaper key as `pack:<id>`, uploaded pictures as `custom:<id>`.
export type DesktopWallpaperID = DesktopStaticWallpaperID | `pack:${string}` | `custom:${string}`

const CUSTOM_WALLPAPER_PREFIX = 'custom:'
const CUSTOM_WALLPAPER_ID = /^[0-9a-f]{32}$/
/** How the chosen uploaded picture is framed; appearance-init.js applies it before the app starts. */
export const CUSTOM_WALLPAPER_DISPLAY_KEY = 'kpanel:desktop-wallpaper-custom:v1'
/** Above this mean brightness (0–100) the classic-mode veil is thickened. */
export const BRIGHT_WALLPAPER_LUMINANCE = 50

export function customWallpaperFromID(value: string | null | undefined): string | undefined {
  if (!value?.startsWith(CUSTOM_WALLPAPER_PREFIX)) return undefined
  const id = value.slice(CUSTOM_WALLPAPER_PREFIX.length)
  return CUSTOM_WALLPAPER_ID.test(id) ? id : undefined
}

export function customWallpaperID(id: string): DesktopWallpaperID {
  return `${CUSTOM_WALLPAPER_PREFIX}${id}`
}

export function isDesktopWallpaperID(value: string | null): value is DesktopWallpaperID {
  return DESKTOP_WALLPAPERS.some((wallpaper) => wallpaper.id === value)
    || Boolean(scenePackFromWallpaper(value))
    || Boolean(customWallpaperFromID(value))
}

/** CSS background-position for a focal point stored as 0–1000 per axis. */
export function wallpaperFocusPosition(wallpaper: Pick<CustomWallpaper, 'focusX' | 'focusY'>): string {
  return `${wallpaper.focusX / 10}% ${wallpaper.focusY / 10}%`
}

function applyCustomDisplay(wallpaper?: Pick<CustomWallpaper, 'id' | 'focusX' | 'focusY' | 'luminance'>): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  try {
    if (wallpaper) {
      window.localStorage.setItem(CUSTOM_WALLPAPER_DISPLAY_KEY, JSON.stringify({
        id: wallpaper.id, focusX: wallpaper.focusX, focusY: wallpaper.focusY, luminance: wallpaper.luminance,
      }))
    } else {
      window.localStorage.removeItem(CUSTOM_WALLPAPER_DISPLAY_KEY)
    }
  } catch {
    // The framing still applies to this page.
  }
  if (wallpaper) root.style.setProperty('--desktop-wallpaper-position', wallpaperFocusPosition(wallpaper))
  else root.style.removeProperty('--desktop-wallpaper-position')
  if (wallpaper && wallpaper.luminance >= BRIGHT_WALLPAPER_LUMINANCE) root.dataset.wallpaperBright = 'true'
  else delete root.dataset.wallpaperBright
}

function readDesktopWallpaperID(): DesktopWallpaperID {
  try {
    const stored = window.localStorage.getItem(DESKTOP_WALLPAPER_KEY)
    return isDesktopWallpaperID(stored) ? stored : 'classic'
  } catch {
    return 'classic'
  }
}

const current = ref<DesktopWallpaperID>(typeof window === 'undefined' ? 'classic' : readDesktopWallpaperID())
const sceneRevision = ref(0)
const customWallpapers = ref<CustomWallpaper[]>([])
const customUsage = ref<CustomWallpaperList['usage']>()

function persist(id: DesktopWallpaperID): void {
  try {
    window.localStorage.setItem(DESKTOP_WALLPAPER_KEY, id)
    window.dispatchEvent(new Event('kpanel:cache-desktop-wallpaper'))
  } catch {
    // The wallpaper still applies to this session when storage is unavailable.
  }
}

function resetToClassic(): void {
  current.value = 'classic'
  applyCustomDisplay()
  persist('classic')
}

export function useDesktopWallpaper() {
  const theme = useTheme()

  /** A scene comes with one color scheme, applied when it is chosen (like a static wallpaper's). */
  function applyScenePackTheme(colors: ThemeColorIntent): void {
    const applied = theme.colors.value
    if (theme.isCustom.value && applied.signatureLinked === colors.signatureLinked
      && [applied.brand, applied.neutral, applied.signature].join() === [colors.brand, colors.neutral, colors.signature].join()) return
    theme.setColors(colors)
  }

  return {
    id: readonly(current),
    sceneRevision: readonly(sceneRevision),
    /** Restart either wallpaper host after installing a new version of the active pack. */
    sceneInstalled(pack: ScenePack): void {
      rememberFileBase(pack.id, pack.fileBase)
      if (scenePackFromWallpaper(current.value) === pack.id) sceneRevision.value++
    },
    /** Re-reads the saved choice (another tab, or state set before this view mounted). */
    refresh(): void {
      current.value = readDesktopWallpaperID()
    },
    /**
     * Applies a wallpaper and its colors. `source` is the listed pack for a `pack:` id and
     * the uploaded wallpaper for a `custom:` id; an upload without colors keeps the current ones.
     */
    select(id: DesktopWallpaperID, source?: ScenePack | CustomWallpaper): boolean {
      const wallpaper = DESKTOP_WALLPAPERS.find((candidate) => candidate.id === id)
      const customID = customWallpaperFromID(id)
      const custom = customID && source && 'luminance' in source && source.id === customID ? source : undefined
      if (!wallpaper && !scenePackFromWallpaper(id) && !custom) return false
      current.value = id
      applyCustomDisplay(custom)
      if (wallpaper) theme.setColors(wallpaper.themePreset.colors)
      const pack = source && !('luminance' in source) ? source : undefined
      const packColors = pack && scenePackThemeColors(pack)
      if (packColors) applyScenePackTheme(packColors)
      if (custom?.theme) applyScenePackTheme({ ...custom.theme, signatureLinked: custom.theme.signature === custom.theme.brand })
      persist(id)
      return true
    },
    /** Falls back to the default wallpaper without touching the colors (a removed pack or picture). */
    resetToClassic,
    /** Uploaded wallpapers, newest first, shared by every picker. */
    customWallpapers: readonly(customWallpapers),
    customUsage: readonly(customUsage),
    /**
     * Refreshes the uploaded list. A chosen picture that is gone (deleted in another
     * browser) falls back to the default wallpaper; one still present has its framing
     * re-applied, in case it was chosen elsewhere.
     */
    async loadCustomWallpapers(signal?: AbortSignal): Promise<void> {
      const list = await api.desktop.wallpapers(signal)
      customWallpapers.value = list.wallpapers
      customUsage.value = list.usage
      const chosen = customWallpaperFromID(current.value)
      if (!chosen) return
      const wallpaper = list.wallpapers.find((candidate) => candidate.id === chosen)
      if (wallpaper) applyCustomDisplay(wallpaper)
      else resetToClassic()
    },
    /** Adds a just-uploaded wallpaper to the shared list. */
    customUploaded(wallpaper: CustomWallpaper): void {
      customWallpapers.value = [wallpaper, ...customWallpapers.value.filter((candidate) => candidate.id !== wallpaper.id)]
      const usage = customUsage.value
      if (usage) customUsage.value = { ...usage, count: usage.count + 1, bytes: usage.bytes + wallpaper.imageBytes + wallpaper.thumbBytes }
    },
    /** Removes a deleted wallpaper; if it was the chosen one, the default takes its place. */
    customDeleted(id: string): void {
      const removed = customWallpapers.value.find((candidate) => candidate.id === id)
      customWallpapers.value = customWallpapers.value.filter((candidate) => candidate.id !== id)
      const usage = customUsage.value
      if (usage && removed) customUsage.value = { ...usage, count: usage.count - 1, bytes: usage.bytes - removed.imageBytes - removed.thumbBytes }
      if (customWallpaperFromID(current.value) === id) resetToClassic()
    },
  }
}
