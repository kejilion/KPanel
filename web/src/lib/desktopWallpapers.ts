import { readonly, ref } from 'vue'
import { rememberFileBase, scenePackFromWallpaper, scenePackThemeColors, type ScenePack } from '@/lib/scenePacks'
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
// Installed 3D scene packs share the wallpaper key as `pack:<id>`.
export type DesktopWallpaperID = DesktopStaticWallpaperID | `pack:${string}`

export function isDesktopWallpaperID(value: string | null): value is DesktopWallpaperID {
  return DESKTOP_WALLPAPERS.some((wallpaper) => wallpaper.id === value) || Boolean(scenePackFromWallpaper(value))
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

function persist(id: DesktopWallpaperID): void {
  try {
    window.localStorage.setItem(DESKTOP_WALLPAPER_KEY, id)
    window.dispatchEvent(new Event('kpanel:cache-desktop-wallpaper'))
  } catch {
    // The wallpaper still applies to this session when storage is unavailable.
  }
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
    /** Applies a wallpaper and its colors; `pack` is the listed pack for a `pack:` id. */
    select(id: DesktopWallpaperID, pack?: ScenePack): boolean {
      const wallpaper = DESKTOP_WALLPAPERS.find((candidate) => candidate.id === id)
      if (!wallpaper && !scenePackFromWallpaper(id)) return false
      current.value = id
      if (wallpaper) theme.setColors(wallpaper.themePreset.colors)
      const packColors = pack && scenePackThemeColors(pack)
      if (packColors) applyScenePackTheme(packColors)
      persist(id)
      return true
    },
    /** Falls back to the default wallpaper without touching the colors (a removed pack). */
    resetToClassic(): void {
      current.value = 'classic'
      persist('classic')
    },
  }
}
