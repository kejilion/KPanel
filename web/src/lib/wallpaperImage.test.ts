import { describe, expect, it } from 'vitest'
import { extractWallpaperTheme, fitWallpaperSize, hslToHex, rgbToHsl, wallpaperNameFromFile } from './wallpaperImage'

function pixels(...colors: Array<[number, number, number, number]>): Uint8ClampedArray {
  const data: number[] = []
  for (const [r, g, b, repeat] of colors) {
    for (let index = 0; index < repeat; index++) data.push(r, g, b, 255)
  }
  return new Uint8ClampedArray(data)
}

function hexToRgb(hex: string): [number, number, number] {
  return [1, 3, 5].map((start) => Number.parseInt(hex.slice(start, start + 2), 16)) as [number, number, number]
}

describe('wallpaper image preparation', () => {
  it('fits pictures inside 4K and 10 megapixels without upscaling', () => {
    expect(fitWallpaperSize(6000, 4000)).toEqual({ width: 3840, height: 2560 })
    expect(fitWallpaperSize(1920, 1080)).toEqual({ width: 1920, height: 1080 })
    const square = fitWallpaperSize(3840, 3840)
    expect(square.width * square.height).toBeLessThanOrEqual(10_000_000)
    expect(fitWallpaperSize(1920, 1080, 640)).toEqual({ width: 640, height: 360 })
  })

  it('derives a readable name from the file name', () => {
    expect(wallpaperNameFromFile('valley_sunset-2026.jpg')).toBe('valley sunset 2026')
    expect(wallpaperNameFromFile('.png')).toBe('Wallpaper')
    expect(Array.from(wallpaperNameFromFile(`${'长'.repeat(60)}.webp`))).toHaveLength(40)
  })

  it('round-trips colors through HSL', () => {
    const [hue, saturation, lightness] = rgbToHsl(232, 184, 106)
    expect(hslToHex(hue, saturation, lightness)).toBe('#e8b86a')
  })

  it('takes the theme from the dominant vivid hue and the accent from a distinct second hue', () => {
    const theme = extractWallpaperTheme(pixels([20, 40, 60, 600], [232, 150, 40, 250], [40, 120, 220, 120]))
    expect(theme).toBeDefined()
    const [brandHue] = rgbToHsl(...hexToRgb(theme!.brand))
    const [signatureHue] = rgbToHsl(...hexToRgb(theme!.signature))
    const [, neutralSaturation] = rgbToHsl(...hexToRgb(theme!.neutral))
    expect(brandHue).toBeGreaterThan(25)
    expect(brandHue).toBeLessThan(45)
    expect(signatureHue).toBeGreaterThan(200)
    expect(signatureHue).toBeLessThan(225)
    expect(neutralSaturation).toBeLessThanOrEqual(0.31)
    for (const color of Object.values(theme!)) expect(color).toMatch(/^#[0-9a-f]{6}$/)
  })

  it('keeps the current colors for a picture without clear color', () => {
    expect(extractWallpaperTheme(pixels([30, 30, 30, 400], [220, 220, 220, 400]))).toBeUndefined()
    expect(extractWallpaperTheme(new Uint8ClampedArray())).toBeUndefined()
  })
})
