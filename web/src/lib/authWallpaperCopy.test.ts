// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { AUTH_WALLPAPER_COPY_KEY, readAuthWallpaperCopy, rememberAuthWallpaperCopy } from './authWallpaperCopy'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  window.localStorage.clear()
})

it('reduces a detailed private wallpaper until its sign-in copy fits storage', async () => {
  const widths: number[] = []
  const bitmap = { width: 2400, height: 1600, close: vi.fn() }
  vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, blob: async () => new Blob(['image']) })))
  vi.stubGlobal('createImageBitmap', vi.fn(async () => bitmap))
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation(() => ({
    drawImage: vi.fn(),
    getImageData: () => ({ data: new Uint8ClampedArray(16 * 16 * 4) }),
    imageSmoothingQuality: 'high',
  }) as unknown as CanvasRenderingContext2D)
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (this: HTMLCanvasElement, callback) {
    widths.push(this.width)
    // The 1280-pixel copy remains too large after base64 encoding at every quality.
    // The 960-pixel copy fits, as a real encoder would with a smaller canvas.
    const size = this.width > 960 ? 310_000 : 210_000
    callback(new Blob([new Uint8Array(size)], { type: 'image/webp' }))
  })
  const changed = vi.fn()
  window.addEventListener('kpanel:cache-desktop-wallpaper', changed)

  const ok = await rememberAuthWallpaperCopy('pack:neon-city', '/api/v1/desktop/scene-packs/neon-city/poster', { focusX: 500, focusY: 500 }, () => true)

  expect(ok).toBe(true)
  expect(widths).toEqual([1280, 1280, 1280, 960])
  expect(readAuthWallpaperCopy()).toMatchObject({ id: 'pack:neon-city', focusX: 500, focusY: 500 })
  expect(window.localStorage.getItem(AUTH_WALLPAPER_COPY_KEY)).toContain('data:image/webp;base64,')
  expect(bitmap.close).toHaveBeenCalledOnce()
  expect(changed).toHaveBeenCalledOnce()
  window.removeEventListener('kpanel:cache-desktop-wallpaper', changed)
})
