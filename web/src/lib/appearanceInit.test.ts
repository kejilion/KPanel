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
  it('restores matching veil tokens but rejects URL-bearing CSS', () => {
    const cached = { 'kpanel:desktop-backdrop:v1': JSON.stringify({ theme: 'dark', colors: null, tokens: { '--desktop-wallpaper-veil-dark': 'linear-gradient(145deg, rgb(0 0 0 / 26%), rgb(0 0 0 / 48%))', '--desktop-aurora-one': 'url(https://example.com/x)' } }) }
    expect(run({}, '/', false, cached).properties.get('--desktop-wallpaper-veil-dark')).toContain('26%')
    expect(run({}, '/', false, cached).properties.has('--desktop-aurora-one')).toBe(false)
    expect(run({ 'kejilion-panel-colors': 'changed' }, '/', false, cached).properties.has('--desktop-wallpaper-veil-dark')).toBe(false)
  })
})
