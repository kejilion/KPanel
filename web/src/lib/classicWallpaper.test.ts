import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { CLASSIC_WALLPAPER_KEY, normalizeClassicWallpaperLevel } from './classicWallpaper'

describe('classic wallpaper level', () => {
  it('accepts only the known levels and defaults to off', () => {
    expect(normalizeClassicWallpaperLevel('ambient')).toBe('ambient')
    expect(normalizeClassicWallpaperLevel('clear')).toBe('clear')
    expect(normalizeClassicWallpaperLevel('off')).toBe('off')
    expect(normalizeClassicWallpaperLevel(null)).toBe('off')
    expect(normalizeClassicWallpaperLevel('glass')).toBe('off')
  })

  it('is applied by the boot script under the same key before the app starts', () => {
    const boot = readFileSync(resolve(__dirname, '../../public/appearance-init.js'), 'utf8')
    expect(boot).toContain(`read('${CLASSIC_WALLPAPER_KEY}')`)
    expect(boot).toContain("classicLevel === 'ambient' || classicLevel === 'clear'")
  })
})
