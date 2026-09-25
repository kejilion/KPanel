// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ScenePack } from '@/lib/scenePacks'

const theme = vi.hoisted(() => ({
  setColors: vi.fn(),
  colors: { value: { brand: '#0c7a60', neutral: '#52645f', signatureLinked: true, signature: '#0c7a60' } },
  isCustom: { value: false },
}))
vi.mock('@/stores/theme', () => ({ useTheme: () => theme }))

import { DESKTOP_WALLPAPER_KEY, DESKTOP_WALLPAPERS, useDesktopWallpaper } from './desktopWallpapers'

describe('shared desktop wallpaper', () => {
  beforeEach(() => {
    window.localStorage.clear()
    theme.setColors.mockClear()
  })

  it('applies a static wallpaper with its colors, saves it and asks the boot script to refresh', () => {
    const cache = vi.fn()
    window.addEventListener('kpanel:cache-desktop-wallpaper', cache)
    const wallpaper = useDesktopWallpaper()

    expect(wallpaper.select('orbit')).toBe(true)

    expect(wallpaper.id.value).toBe('orbit')
    expect(window.localStorage.getItem(DESKTOP_WALLPAPER_KEY)).toBe('orbit')
    expect(theme.setColors).toHaveBeenCalledWith(DESKTOP_WALLPAPERS[1].themePreset.colors)
    expect(cache).toHaveBeenCalledTimes(1)
    window.removeEventListener('kpanel:cache-desktop-wallpaper', cache)
  })

  it('applies a scene pack with the colors of the listed pack and resets without touching colors', () => {
    const wallpaper = useDesktopWallpaper()
    const pack = { theme: { brand: '#c2357f', neutral: '#3e3346', signature: '#2ec4d6' } } as ScenePack

    wallpaper.select('pack:neon-city', pack)
    expect(wallpaper.id.value).toBe('pack:neon-city')
    expect(theme.setColors).toHaveBeenCalledTimes(1)

    theme.setColors.mockClear()
    wallpaper.resetToClassic()
    expect(wallpaper.id.value).toBe('classic')
    expect(window.localStorage.getItem(DESKTOP_WALLPAPER_KEY)).toBe('classic')
    expect(theme.setColors).not.toHaveBeenCalled()
  })

  it('refreshes from storage and ignores unknown ids', () => {
    const wallpaper = useDesktopWallpaper()
    window.localStorage.setItem(DESKTOP_WALLPAPER_KEY, 'rift')
    wallpaper.refresh()
    expect(wallpaper.id.value).toBe('rift')

    window.localStorage.setItem(DESKTOP_WALLPAPER_KEY, 'not-a-wallpaper')
    wallpaper.refresh()
    expect(wallpaper.id.value).toBe('classic')
    expect(wallpaper.select('nope' as 'classic')).toBe(false)
  })
})
