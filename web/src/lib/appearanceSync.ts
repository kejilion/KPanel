import { ApiError, api } from '@/lib/api'
import { useClassicWallpaper } from '@/lib/classicWallpaper'
import { applySyncedWallpaper, isDesktopWallpaperID, useDesktopWallpaper } from '@/lib/desktopWallpapers'
import { useTheme } from '@/stores/theme'
import { useToast } from '@/stores/toast'
import { t } from '@/i18n'
import type { AppearanceSettings } from '@/types/api'

type AppearanceValue = Pick<AppearanceSettings, 'theme' | 'colors' | 'wallpaper' | 'classicLevel'>

const theme = useTheme()
const wallpaper = useDesktopWallpaper()
const classic = useClassicWallpaper()
const toast = useToast()
let server: AppearanceSettings | undefined
let pending: Partial<AppearanceValue> = {}
let active = false
let applying = false
let saving = false
let generation = 0

function localValue(): AppearanceValue {
  return {
    theme: theme.preference.value,
    colors: theme.isCustom.value ? { ...theme.colors.value } : null,
    wallpaper: wallpaper.id.value,
    classicLevel: classic.level.value,
  }
}

function hasLocalCustomization(value: AppearanceValue): boolean {
  return value.theme !== 'system' || value.colors !== null || value.wallpaper !== 'classic' || value.classicLevel !== 'off'
}

function apply(value: AppearanceValue): void {
  applying = true
  try {
    theme.setTheme(value.theme)
    if (value.colors) theme.setColors(value.colors)
    else theme.resetColors()
    if (isDesktopWallpaperID(value.wallpaper)) applySyncedWallpaper(value.wallpaper)
    classic.setLevel(value.classicLevel)
  } finally {
    applying = false
  }
}

function changed(event: Event): void {
  if (!active || applying) return
  Object.assign(pending, (event as CustomEvent<Partial<AppearanceValue>>).detail)
  // Wallpaper selection and its matching color change are emitted together in one turn.
  queueMicrotask(() => { void save() })
}

async function save(): Promise<void> {
  if (!active || !server || saving || !Object.keys(pending).length) return
  saving = true
  const run = generation
  let failed = false
  let conflicts = 0
  try {
    while (active && run === generation && Object.keys(pending).length) {
      const patch = pending
      pending = {}
      const next = { ...server, ...patch }
      try {
        const updated = await api.desktop.updateAppearance({
          theme: next.theme, colors: next.colors, wallpaper: next.wallpaper,
          classicLevel: next.classicLevel, expectedResourceVersion: server.resourceVersion,
        })
        if (!active || run !== generation) return
        server = updated
      } catch (error) {
        if (!active || run !== generation) return
        if (error instanceof ApiError && error.status === 409 && ++conflicts <= 3) {
          try {
            const current = await api.desktop.appearance()
            if (!active || run !== generation) return
            server = current
            pending = { ...patch, ...pending }
            continue
          } catch { /* Report the original failed save below. */ }
        }
        pending = { ...patch, ...pending }
        toast.danger(t('desktop.appearanceSyncFailed'), t('desktop.appearanceSyncRetry'))
        failed = true
        break
      }
    }
  } finally {
    saving = false
    if (!failed && active && server && Object.keys(pending).length) queueMicrotask(() => { void save() })
  }
}

/** Called only after an authenticated route mounts. Local storage supplies the first paint. */
export async function startAppearanceSync(): Promise<void> {
  if (active) return
  active = true
  const run = ++generation
  window.addEventListener('kpanel:appearance-changed', changed)
  const initial = localValue()
  try {
    let fetched = await api.desktop.appearance()
    if (!active || run !== generation) return
    if (!fetched.configured && hasLocalCustomization(initial)) {
      try {
        fetched = await api.desktop.updateAppearance({
          ...initial, expectedResourceVersion: fetched.resourceVersion,
        })
      } catch (error) {
        if (!(error instanceof ApiError && error.status === 409)) throw error
        fetched = await api.desktop.appearance()
      }
    }
    if (!active || run !== generation) return
    server = fetched
    apply({ ...fetched, ...pending })
    void save()
  } catch {
    if (active && run === generation) toast.danger(t('desktop.appearanceLoadFailed'), t('desktop.appearanceLoadFallback'))
  }
}

export function stopAppearanceSync(): void {
  active = false
  generation++
  window.removeEventListener('kpanel:appearance-changed', changed)
  server = undefined
  pending = {}
}
