// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  appearance: vi.fn(),
  updateAppearance: vi.fn(),
  setTheme: vi.fn(),
  setColors: vi.fn(),
  resetColors: vi.fn(),
  setLevel: vi.fn(),
  applyWallpaper: vi.fn(),
  danger: vi.fn(),
  preference: { value: 'system' },
  colors: { value: { brand: '#2867b2', neutral: '#4b5d76', signature: '#2867b2', signatureLinked: true } },
  isCustom: { value: false },
  wallpaper: { value: 'classic' },
  classicLevel: { value: 'off' },
}))

vi.mock('@/lib/api', () => ({ api: { desktop: { appearance: mocks.appearance, updateAppearance: mocks.updateAppearance } }, ApiError: class extends Error { constructor(message: string, public status = 0) { super(message) } } }))
vi.mock('@/stores/theme', () => ({ useTheme: () => ({ preference: mocks.preference, colors: mocks.colors, isCustom: mocks.isCustom, setTheme: mocks.setTheme, setColors: mocks.setColors, resetColors: mocks.resetColors }) }))
vi.mock('@/lib/classicWallpaper', () => ({ useClassicWallpaper: () => ({ level: mocks.classicLevel, setLevel: mocks.setLevel }) }))
vi.mock('@/lib/desktopWallpapers', () => ({ useDesktopWallpaper: () => ({ id: mocks.wallpaper }), isDesktopWallpaperID: () => true, applySyncedWallpaper: mocks.applyWallpaper }))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ danger: mocks.danger }) }))
vi.mock('@/i18n', () => ({ t: (key: string) => key }))

import { appearanceReady, startAppearanceSync, stopAppearanceSync } from './appearanceSync'

const remote = {
  configured: true, resourceVersion: 'sha256:remote', theme: 'dark' as const, colors: null,
  wallpaper: 'orbit', classicLevel: 'clear' as const,
}

async function settle(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0))
}

describe('shared appearance preference', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.preference.value = 'system'
    mocks.isCustom.value = false
    mocks.wallpaper.value = 'classic'
    mocks.classicLevel.value = 'off'
  })
  afterEach(() => stopAppearanceSync())

  it('holds the first wallpaper until a fresh browser receives the server choice', async () => {
    let resolve!: (value: typeof remote) => void
    mocks.appearance.mockReturnValue(new Promise((done) => { resolve = done }))
    const sync = startAppearanceSync()
    expect(appearanceReady.value).toBe(false)
    expect(mocks.applyWallpaper).not.toHaveBeenCalled()
    resolve(remote)
    await sync
    expect(mocks.applyWallpaper).toHaveBeenCalledWith('orbit')
    expect(appearanceReady.value).toBe(true)
  })

  it('releases the local fallback on failure without leaving the wallpaper hidden', async () => {
    mocks.appearance.mockRejectedValue(new Error('offline'))
    await startAppearanceSync()
    expect(appearanceReady.value).toBe(true)
    expect(mocks.applyWallpaper).not.toHaveBeenCalled()
    expect(mocks.danger).toHaveBeenCalledWith('desktop.appearanceLoadFailed', 'desktop.appearanceLoadFallback')
  })

  it('does not reveal a new session when an old appearance request finishes after logout', async () => {
    let resolveOld!: (value: typeof remote) => void
    let resolveNew!: (value: typeof remote) => void
    mocks.appearance.mockReturnValueOnce(new Promise((done) => { resolveOld = done }))
      .mockReturnValueOnce(new Promise((done) => { resolveNew = done }))
    const old = startAppearanceSync()
    stopAppearanceSync()
    const current = startAppearanceSync()
    resolveOld(remote)
    await old
    expect(appearanceReady.value).toBe(false)
    expect(mocks.applyWallpaper).not.toHaveBeenCalled()
    resolveNew(remote)
    await current
    expect(appearanceReady.value).toBe(true)
  })

  it('applies the saved server choice in a fresh browser and writes later changes', async () => {
    mocks.appearance.mockResolvedValue({ ...remote })
    mocks.updateAppearance.mockImplementation(async (body) => ({ ...remote, ...body, configured: true, resourceVersion: 'sha256:next' }))
    await startAppearanceSync()
    expect(mocks.setTheme).toHaveBeenCalledWith('dark')
    expect(mocks.applyWallpaper).toHaveBeenCalledWith('orbit')
    expect(mocks.setLevel).toHaveBeenCalledWith('clear')
    expect(mocks.updateAppearance).not.toHaveBeenCalled()

    window.dispatchEvent(new CustomEvent('kpanel:appearance-changed', { detail: { wallpaper: 'prism' } }))
    await settle()
    expect(mocks.updateAppearance).toHaveBeenCalledWith(expect.objectContaining({
      theme: 'dark', wallpaper: 'prism', classicLevel: 'clear', expectedResourceVersion: 'sha256:remote',
    }))
  })

  it('does not let a new browser with defaults replace an unconfigured server', async () => {
    mocks.appearance.mockResolvedValue({ ...remote, configured: false, theme: 'system', wallpaper: 'classic', classicLevel: 'off' })
    mocks.updateAppearance.mockImplementation(async (body) => ({ ...remote, ...body, resourceVersion: 'sha256:next' }))
    await startAppearanceSync()
    expect(mocks.updateAppearance).not.toHaveBeenCalled()
    window.dispatchEvent(new CustomEvent('kpanel:appearance-changed', { detail: { theme: 'light' } }))
    await settle()
    expect(mocks.updateAppearance).toHaveBeenCalledWith(expect.objectContaining({ theme: 'light', expectedResourceVersion: 'sha256:remote' }))
  })

  it('rebases an explicit change when another browser saved first', async () => {
    const { ApiError } = await import('@/lib/api')
    mocks.appearance.mockResolvedValueOnce({ ...remote }).mockResolvedValueOnce({ ...remote, theme: 'light', resourceVersion: 'sha256:other' })
    mocks.updateAppearance.mockRejectedValueOnce(new ApiError('conflict', 409))
      .mockImplementationOnce(async (body) => ({ ...remote, ...body, configured: true, resourceVersion: 'sha256:next' }))
    await startAppearanceSync()
    window.dispatchEvent(new CustomEvent('kpanel:appearance-changed', { detail: { wallpaper: 'prism' } }))
    await settle()
    expect(mocks.updateAppearance).toHaveBeenNthCalledWith(2, expect.objectContaining({
      theme: 'light', wallpaper: 'prism', expectedResourceVersion: 'sha256:other',
    }))
  })
})
