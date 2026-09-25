import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it } from 'vitest'

const script = readFileSync(new URL('../../public/appearance-init.js', import.meta.url), 'utf8')
function run(values: Record<string, string>, pathname = '/overview', blocked = false, cached: Record<string, string> = {}) {
  const properties = new Map<string, string>()
  const classes = new Set<string>()
  let resolveImage!: () => void
  const imageReady = new Promise<void>(resolve => { resolveImage = resolve })
  const root = { dataset: {} as Record<string, string>, style: { colorScheme: '', setProperty: (key: string, value: string) => properties.set(key, value) }, classList: { add: (name: string) => classes.add(name), remove: (name: string) => classes.delete(name) } }
  runInNewContext(script, { document: { documentElement: root }, location: { pathname }, matchMedia: () => ({ matches: true }), localStorage: { getItem: (key: string) => { if (blocked) throw new Error('blocked'); return values[key] ?? null } }, sessionStorage: { getItem: (key: string) => cached[key] ?? null }, Image: class { decode() { return imageReady } }, fetch: async () => ({ ok: false }), window: { addEventListener() {} } })
  return { root, properties, classes, resolveImage }
}

describe('appearance before application startup', () => {
  it.each(['light', 'dark'])('restores %s desktop and saved wallpaper', theme => {
    const result = run({ 'kejilion-panel-theme': theme, 'kejilion-panel-desktop-mode': 'desktop', 'kpanel:desktop-wallpaper:v1': 'prism' })
    expect(result.root.dataset.theme).toBe(theme)
    expect(result.root.style.colorScheme).toBe(theme)
    expect(result.classes.has('desktop-boot')).toBe(true)
    expect(result.properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop-prism.webp")')
  })
  it('falls back to the system theme when storage is unavailable', () => {
    const result = run({}, '/overview', true)
    expect(result.root.dataset.theme).toBe('dark')
    expect(result.classes.has('desktop-boot')).toBe(false)
  })
  it('never interpolates an untrusted wallpaper URL', () => {
    expect(run({ 'kpanel:desktop-wallpaper:v1': 'https://example.com/image' }).properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
  })
  it.each(['/login', '/setup', '/share/token', '/share/file/token'])('does not paint a desktop behind %s', pathname => {
    expect(run({ 'kejilion-panel-desktop-mode': 'desktop' }, pathname).classes.has('desktop-boot')).toBe(false)
  })
  it('leaves classic mode without a startup wallpaper surface', () => {
    expect(run({ 'kejilion-panel-desktop-mode': 'classic' }).classes.has('desktop-boot')).toBe(false)
  })
  it('holds the image and veil together until decoding finishes', async () => {
    const result = run({ 'kejilion-panel-desktop-mode': 'desktop' })
    expect(result.classes.has('desktop-wallpaper-loading')).toBe(true)
    result.resolveImage()
    for (let tick = 0; tick < 5; tick++) await Promise.resolve()
    expect(result.classes.has('desktop-wallpaper-loading')).toBe(false)
  })
  it('reuses a bounded per-tab bitmap without interpolating cache URLs', () => {
    const key = 'kpanel:desktop-wallpaper-cache:v1:classic'
    expect(run({}, '/', false, { [key]: 'data:image/webp;base64,UklGRg==' }).properties.get('--desktop-wallpaper-image')).toContain('data:image/webp;')
    expect(run({}, '/', false, { [key]: 'https://example.com/x.webp' }).properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
  })
  function bootPack(stored: string, { reduced, always = false }: { reduced: boolean, always?: boolean }) {
    const properties = new Map<string, string>()
    const classes = new Set<string>()
    const root = { dataset: {} as Record<string, string>, style: { colorScheme: '', setProperty: (key: string, value: string) => properties.set(key, value) }, classList: { add: (name: string) => classes.add(name), remove: (name: string) => classes.delete(name) } }
    const sources: string[] = []
    const values: Record<string, string> = { 'kpanel:desktop-wallpaper:v1': stored, ...(always ? { 'kpanel:desktop-scene-motion:v1': 'always' } : {}) }
    runInNewContext(script, {
      document: { documentElement: root },
      location: { pathname: '/' },
      matchMedia: (query: string) => ({ matches: query.includes('reduced-motion') && reduced }),
      localStorage: { getItem: (key: string) => values[key] ?? null },
      sessionStorage: { getItem: () => null },
      Image: class {
        set src(value: string) { sources.push(value) }
        decode() { return sources.at(-1)?.includes('/scene-packs/') ? Promise.reject(new Error('404')) : Promise.resolve() }
      },
      fetch: async () => ({ ok: false }),
      window: { addEventListener() {} },
    })
    return { root, properties, classes, sources }
  }
  it('boots a live scene pack to black so its entrance is the first thing seen', () => {
    for (const result of [bootPack('pack:orbital-station', { reduced: false }), bootPack('pack:orbital-station', { reduced: true, always: true })]) {
      expect(result.root.dataset.desktopWallpaper).toBe('pack:orbital-station')
      expect(result.root.dataset.desktopWallpaperScene).toBe('live')
      expect(result.properties.has('--desktop-wallpaper-image')).toBe(false)
      expect(result.classes.has('desktop-wallpaper-loading')).toBe(false)
      expect(result.sources).toEqual([])
    }
  })
  it('paints the scene pack poster for reduced motion and never trusts other pack keys', () => {
    const result = run({ 'kpanel:desktop-wallpaper:v1': 'pack:orbital-station' })
    expect(result.root.dataset.desktopWallpaperScene).toBeUndefined()
    expect(result.properties.get('--desktop-wallpaper-image')).toBe('url("/api/v1/desktop/scene-packs/orbital-station/poster")')
    for (const stored of ['pack:../../logout', 'pack:Orbital', 'pack:']) {
      const forged = bootPack(stored, { reduced: false })
      expect(forged.root.dataset.desktopWallpaperScene).toBeUndefined()
      expect(forged.properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
    }
  })
  it('falls back to the classic wallpaper when a still pack poster is gone', async () => {
    const result = bootPack('pack:orbital-station', { reduced: true })
    for (let tick = 0; tick < 8; tick++) await Promise.resolve()
    expect(result.sources).toEqual(['/api/v1/desktop/scene-packs/orbital-station/poster', '/wallpapers/kpanel-desktop.webp'])
    expect(result.properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
    expect(result.root.dataset.desktopWallpaperFailed).toBeUndefined()
  })
  it('restores matching veil tokens but rejects URL-bearing CSS', () => {
    const cached = { 'kpanel:desktop-backdrop:v1': JSON.stringify({ theme: 'dark', colors: null, tokens: { '--desktop-wallpaper-veil-dark': 'linear-gradient(145deg, rgb(0 0 0 / 26%), rgb(0 0 0 / 48%))', '--desktop-aurora-one': 'url(https://example.com/x)' } }) }
    expect(run({}, '/', false, cached).properties.get('--desktop-wallpaper-veil-dark')).toContain('26%')
    expect(run({}, '/', false, cached).properties.has('--desktop-aurora-one')).toBe(false)
    expect(run({ 'kejilion-panel-colors': 'changed' }, '/', false, cached).properties.has('--desktop-wallpaper-veil-dark')).toBe(false)
  })
  it('boots an uploaded picture with its saved framing and never trusts other ids', () => {
    const id = '0123456789abcdef0123456789abcdef'
    const display = JSON.stringify({ id, focusX: 700, focusY: 320, luminance: 72 })
    const result = run({ 'kpanel:desktop-wallpaper:v1': `custom:${id}`, 'kpanel:desktop-wallpaper-custom:v1': display })
    expect(result.properties.get('--desktop-wallpaper-image')).toBe(`url("/api/v1/desktop/wallpapers/${id}/image")`)
    expect(result.properties.get('--classic-wallpaper-image')).toBe(`url("/api/v1/desktop/wallpapers/${id}/image")`)
    expect(result.properties.get('--desktop-wallpaper-position')).toBe('70% 32%')
    expect(result.root.dataset.wallpaperBright).toBe('true')

    const mismatched = run({ 'kpanel:desktop-wallpaper:v1': `custom:${id}`, 'kpanel:desktop-wallpaper-custom:v1': JSON.stringify({ id: 'f'.repeat(32), focusX: 1, focusY: 1, luminance: 90 }) })
    expect(mismatched.properties.has('--desktop-wallpaper-position')).toBe(false)
    expect(mismatched.root.dataset.wallpaperBright).toBeUndefined()
    const outOfRange = run({ 'kpanel:desktop-wallpaper:v1': `custom:${id}`, 'kpanel:desktop-wallpaper-custom:v1': JSON.stringify({ id, focusX: '50%;x', focusY: 2000, luminance: 10 }) })
    expect(outOfRange.properties.has('--desktop-wallpaper-position')).toBe(false)
    for (const stored of ['custom:../../etc', `custom:${id.toUpperCase()}`, `custom:${id}x`]) {
      expect(run({ 'kpanel:desktop-wallpaper:v1': stored }).properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
    }
  })
})
