import { readonly, ref } from 'vue'

// Classic mode can show the desktop wallpaper behind its pages. The picture is the one chosen in
// desktop mode (a 3D scene pack contributes its poster, never a live scene); only the strength is
// chosen here. appearance-init.js applies the stored value before the app starts.
export const CLASSIC_WALLPAPER_KEY = 'kpanel:classic-wallpaper:v1'

export type ClassicWallpaperLevel = 'off' | 'ambient' | 'clear'

const levels: readonly ClassicWallpaperLevel[] = ['off', 'ambient', 'clear']

export function normalizeClassicWallpaperLevel(value: string | null | undefined): ClassicWallpaperLevel {
  return levels.includes(value as ClassicWallpaperLevel) ? value as ClassicWallpaperLevel : 'off'
}

function readLevel(): ClassicWallpaperLevel {
  try {
    return normalizeClassicWallpaperLevel(window.localStorage.getItem(CLASSIC_WALLPAPER_KEY))
  } catch {
    return 'off'
  }
}

const level = ref<ClassicWallpaperLevel>(typeof window === 'undefined' ? 'off' : readLevel())

function applyToRoot(value: ClassicWallpaperLevel): void {
  if (typeof document === 'undefined') return
  const root = document.documentElement
  if (value === 'off') delete root.dataset.classicWallpaper
  else root.dataset.classicWallpaper = value
}

export function useClassicWallpaper() {
  function setLevel(value: ClassicWallpaperLevel): void {
    level.value = value
    applyToRoot(value)
    try {
      window.localStorage.setItem(CLASSIC_WALLPAPER_KEY, value)
    } catch {
      // The level still applies to this session when storage is unavailable.
    }
  }

  return { level: readonly(level), setLevel }
}
