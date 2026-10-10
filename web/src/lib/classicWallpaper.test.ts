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

  it('keeps the selected wallpaper visible while reducing topbar transparency', () => {
    const css = readFileSync(resolve(__dirname, '../styles/classicWallpaper.css'), 'utf8')
    const reduced = css.match(/@media \(prefers-reduced-transparency: reduce\), \(prefers-contrast: more\) \{([\s\S]*?)^\}/m)?.[1]
    expect(reduced).toBeDefined()
    expect(reduced).toContain(':root:has(.classic-backdrop) .topbar')
    expect(reduced).toContain('background: var(--material-chrome-fill);')
    // Explicit wallpaper remains on the page; only shell chrome becomes solid.
    expect(reduced).not.toMatch(/display:\s*none|visibility:\s*hidden|opacity:\s*0\b/)
    expect(reduced).not.toContain('.page-content')
    expect(css).toMatch(/@media\s*\(forced-colors:\s*active\)/)
  })
})
