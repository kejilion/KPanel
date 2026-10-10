import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { normalizeClassicWallpaperLevel } from './classicWallpaper'

describe('classic wallpaper level', () => {
  it('accepts only the known levels and defaults to off', () => {
    expect(normalizeClassicWallpaperLevel('ambient')).toBe('ambient')
    expect(normalizeClassicWallpaperLevel('clear')).toBe('clear')
    expect(normalizeClassicWallpaperLevel('off')).toBe('off')
    expect(normalizeClassicWallpaperLevel(null)).toBe('off')
    expect(normalizeClassicWallpaperLevel('glass')).toBe('off')
  })


  it('keeps an explicitly selected wallpaper visible when the browser reduces transparency', () => {
    const css = readFileSync(resolve(__dirname, '../styles/classicWallpaper.css'), 'utf8')
    expect(css).not.toMatch(/@media[^\{]*prefers-reduced-transparency/)
    expect(css).toMatch(/@media\s*\(forced-colors:\s*active\)/)
  })
})
