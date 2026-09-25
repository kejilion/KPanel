// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ScenePack } from '@/lib/scenePacks'

const theme = vi.hoisted(() => ({
  setColors: vi.fn(),
  colors: { value: { brand: '#0c7a60', neutral: '#52645f', signatureLinked: true, signature: '#0c7a60' } },
  isCustom: { value: false },
}))
vi.mock('@/stores/theme', () => ({ useTheme: () => theme }))
const wallpapersAPI = vi.hoisted(() => ({
  wallpapers: vi.fn(),
  wallpaperImageURL: (id: string) => `/api/v1/desktop/wallpapers/${id}/image`,
  scenePackPosterURL: (id: string) => `/api/v1/desktop/scene-packs/${id}/poster`,
}))
vi.mock('@/lib/api', () => ({ api: { desktop: wallpapersAPI } }))
const authCopy = vi.hoisted(() => ({
  stored: undefined as { id: string, focusX: number, focusY: number } | undefined,
  remember: vi.fn(async (_id: string, _url: string, _focus: { focusX: number, focusY: number }, _stillChosen: () => boolean) => true),
  forget: vi.fn(),
}))
vi.mock('@/lib/authWallpaperCopy', () => ({
  readAuthWallpaperCopy: () => authCopy.stored,
  rememberAuthWallpaperCopy: authCopy.remember,
  forgetAuthWallpaperCopy: authCopy.forget,
}))

import type { CustomWallpaper } from '@/types/api'
import {
  CUSTOM_WALLPAPER_DISPLAY_KEY,
  customWallpaperFromID,
  customWallpaperID,
  DESKTOP_WALLPAPER_KEY,
  DESKTOP_WALLPAPERS,
  isDesktopWallpaperID,
  useDesktopWallpaper,
  wallpaperFocusPosition,
} from './desktopWallpapers'

const uploaded: CustomWallpaper = {
  id: '0123456789abcdef0123456789abcdef', name: '山谷日落', format: 'webp', width: 3840, height: 2160,
  imageBytes: 2_000_000, thumbBytes: 40_000, focusX: 700, focusY: 320, luminance: 72,
  theme: { brand: '#e8b86a', neutral: '#1c3a4a', signature: '#4f8fa8' },
  createdAt: '2026-09-25T00:00:00Z', imageDigest: 'a'.repeat(64),
}

describe('shared desktop wallpaper', () => {
  beforeEach(() => {
    window.localStorage.clear()
    theme.setColors.mockClear()
    authCopy.stored = undefined
    authCopy.remember.mockClear()
    authCopy.forget.mockClear()
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

  it('recognizes uploaded wallpaper ids and frames them at their focal point', () => {
    expect(customWallpaperFromID(customWallpaperID(uploaded.id))).toBe(uploaded.id)
    expect(isDesktopWallpaperID(`custom:${uploaded.id}`)).toBe(true)
    for (const value of ['custom:', 'custom:xyz', `custom:${uploaded.id.toUpperCase()}`, `custom:${uploaded.id}/../x`]) {
      expect(isDesktopWallpaperID(value)).toBe(false)
    }
    expect(wallpaperFocusPosition(uploaded)).toBe('70% 32%')
  })

  it('applies an uploaded wallpaper with its framing, brightness and colors', () => {
    const wallpaper = useDesktopWallpaper()
    expect(wallpaper.select(customWallpaperID(uploaded.id))).toBe(false)

    expect(wallpaper.select(customWallpaperID(uploaded.id), uploaded)).toBe(true)
    expect(window.localStorage.getItem(DESKTOP_WALLPAPER_KEY)).toBe(`custom:${uploaded.id}`)
    expect(JSON.parse(window.localStorage.getItem(CUSTOM_WALLPAPER_DISPLAY_KEY)!)).toEqual({ id: uploaded.id, focusX: 700, focusY: 320, luminance: 72 })
    expect(document.documentElement.style.getPropertyValue('--desktop-wallpaper-position')).toBe('70% 32%')
    expect(document.documentElement.dataset.wallpaperBright).toBe('true')
    expect(theme.setColors).toHaveBeenCalledWith({ ...uploaded.theme, signatureLinked: false })

    theme.setColors.mockClear()
    wallpaper.select(customWallpaperID(uploaded.id), { ...uploaded, theme: undefined, luminance: 20 })
    expect(theme.setColors).not.toHaveBeenCalled()
    expect(document.documentElement.dataset.wallpaperBright).toBeUndefined()

    wallpaper.select('orbit')
    expect(window.localStorage.getItem(CUSTOM_WALLPAPER_DISPLAY_KEY)).toBeNull()
    expect(document.documentElement.style.getPropertyValue('--desktop-wallpaper-position')).toBe('')
  })

  it('falls back to the default when the chosen upload is deleted here or elsewhere', async () => {
    const wallpaper = useDesktopWallpaper()
    wallpapersAPI.wallpapers.mockResolvedValue({ wallpapers: [uploaded], usage: { count: 1, bytes: 2_040_000, maxCount: 12, maxBytes: 48 << 20 } })
    await wallpaper.loadCustomWallpapers()
    wallpaper.select(customWallpaperID(uploaded.id), uploaded)
    wallpaper.customDeleted(uploaded.id)
    expect(wallpaper.id.value).toBe('classic')
    expect(wallpaper.customWallpapers.value).toEqual([])
    expect(wallpaper.customUsage.value?.count).toBe(0)

    wallpaper.customUploaded(uploaded)
    wallpaper.select(customWallpaperID(uploaded.id), uploaded)
    wallpapersAPI.wallpapers.mockResolvedValue({ wallpapers: [], usage: { count: 0, bytes: 0, maxCount: 12, maxBytes: 48 << 20 } })
    await wallpaper.loadCustomWallpapers()
    expect(wallpaper.id.value).toBe('classic')
    expect(window.localStorage.getItem(CUSTOM_WALLPAPER_DISPLAY_KEY)).toBeNull()
  })

  it('keeps a sign-in copy for private wallpapers only, framed at the focal point', () => {
    const wallpaper = useDesktopWallpaper()
    wallpaper.select(customWallpaperID(uploaded.id), uploaded)
    expect(authCopy.remember).toHaveBeenCalledWith(
      `custom:${uploaded.id}`, `/api/v1/desktop/wallpapers/${uploaded.id}/image`, { focusX: 700, focusY: 320 }, expect.any(Function),
    )
    const stillChosen = authCopy.remember.mock.calls[0]![3]
    expect(stillChosen()).toBe(true)

    authCopy.remember.mockClear()
    wallpaper.select('pack:orbital-station')
    expect(authCopy.remember).toHaveBeenCalledWith(
      'pack:orbital-station', '/api/v1/desktop/scene-packs/orbital-station/poster', { focusX: 500, focusY: 500 }, expect.any(Function),
    )
    expect(stillChosen()).toBe(false)

    authCopy.stored = { id: 'pack:orbital-station', focusX: 500, focusY: 500 }
    authCopy.remember.mockClear()
    wallpaper.ensureAuthWallpaperCopy()
    expect(authCopy.remember).not.toHaveBeenCalled()

    wallpaper.select('rift')
    expect(authCopy.forget).toHaveBeenCalledTimes(1)
    wallpaper.resetToClassic()
    expect(authCopy.forget).toHaveBeenCalledTimes(2)
  })

  it('reframes a centred sign-in copy once the upload list confirms the focal point', async () => {
    window.localStorage.setItem('kpanel:desktop-wallpaper:v1', `custom:${uploaded.id}`)
    const wallpaper = useDesktopWallpaper()
    wallpaper.refresh()
    authCopy.stored = { id: `custom:${uploaded.id}`, focusX: 500, focusY: 500 }
    wallpapersAPI.wallpapers.mockResolvedValue({ wallpapers: [uploaded], usage: { count: 1, bytes: 2_040_000, maxCount: 12, maxBytes: 48 << 20 } })
    await wallpaper.loadCustomWallpapers()
    expect(authCopy.remember).toHaveBeenCalledWith(
      `custom:${uploaded.id}`, `/api/v1/desktop/wallpapers/${uploaded.id}/image`, { focusX: 700, focusY: 320 }, expect.any(Function),
    )
  })
})
