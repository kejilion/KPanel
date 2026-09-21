import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it } from 'vitest'

const script = readFileSync(new URL('../../public/appearance-init.js', import.meta.url), 'utf8')
function run(values: Record<string, string>, pathname = '/overview', blocked = false) {
  const properties = new Map<string, string>()
  const classes = new Set<string>()
  const root = { dataset: {} as Record<string, string>, style: { colorScheme: '', setProperty: (key: string, value: string) => properties.set(key, value) }, classList: { add: (name: string) => classes.add(name) } }
  runInNewContext(script, { document: { documentElement: root }, location: { pathname }, matchMedia: () => ({ matches: true }), localStorage: { getItem: (key: string) => { if (blocked) throw new Error('blocked'); return values[key] ?? null } } })
  return { root, properties, classes }
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
    expect(result.classes.size).toBe(0)
  })
  it('never interpolates an untrusted wallpaper URL', () => {
    expect(run({ 'kpanel:desktop-wallpaper:v1': 'https://example.com/image' }).properties.get('--desktop-wallpaper-image')).toBe('url("/wallpapers/kpanel-desktop.webp")')
  })
  it.each(['/login', '/setup', '/share/token', '/share/file/token'])('does not paint a desktop behind %s', pathname => {
    expect(run({ 'kejilion-panel-desktop-mode': 'desktop' }, pathname).classes.size).toBe(0)
  })
  it('leaves classic mode without a startup wallpaper surface', () => {
    expect(run({ 'kejilion-panel-desktop-mode': 'classic' }).classes.size).toBe(0)
  })
})
