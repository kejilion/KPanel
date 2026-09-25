import type { CustomWallpaperTheme } from '@/types/api'

/**
 * Prepares an uploaded picture in the browser: it is decoded (respecting camera
 * orientation), scaled to at most 4K, re-encoded as WebP (JPEG where the browser
 * cannot encode WebP) and given a thumbnail. Re-encoding drops EXIF data such as
 * where a photo was taken before anything leaves the device; the panel strips
 * metadata again on arrival. Limits mirror internal/desktopwallpapers.
 */
export const WALLPAPER_SOURCE_MAX_BYTES = 20 * 1024 * 1024
export const WALLPAPER_ACCEPT = 'image/jpeg,image/png,image/webp,image/avif'
const ACCEPTED_TYPES = new Set(WALLPAPER_ACCEPT.split(','))
const MAX_LONG_EDGE = 3840
const MAX_PIXELS = 10_000_000
const MIN_EDGE = 64
const THUMB_LONG_EDGE = 640
const MAX_IMAGE_BYTES = 8 * 1024 * 1024
const MAX_THUMB_BYTES = 256 * 1024

export type WallpaperFileProblem = 'type' | 'size' | 'decode' | 'small' | 'encode'

export class WallpaperFileError extends Error {
  constructor(readonly problem: WallpaperFileProblem) {
    super(`wallpaper file ${problem}`)
  }
}

export interface PreparedWallpaperImage {
  image: Blob
  thumb: Blob
  width: number
  height: number
  /** Undefined for a picture without a clear color (black and white, for example). */
  theme?: CustomWallpaperTheme
}

/** Fits a picture inside the long-edge and pixel limits without upscaling. */
export function fitWallpaperSize(width: number, height: number, longEdge = MAX_LONG_EDGE, maxPixels = MAX_PIXELS): { width: number, height: number } {
  const scale = Math.min(1, longEdge / Math.max(width, height), Math.sqrt(maxPixels / (width * height)))
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) }
}

/** A readable default name from the file name. */
export function wallpaperNameFromFile(fileName: string): string {
  const base = fileName.replace(/\.[a-z0-9]{2,5}$/i, '').replace(/[\u0000-\u001f\u007f]/g, '').replace(/[_-]+/g, ' ').trim()
  return Array.from(base || 'Wallpaper').slice(0, 40).join('').trim()
}

export async function prepareWallpaperImage(file: File): Promise<PreparedWallpaperImage> {
  if (!ACCEPTED_TYPES.has(file.type)) throw new WallpaperFileError('type')
  if (file.size > WALLPAPER_SOURCE_MAX_BYTES) throw new WallpaperFileError('size')
  const bitmap = await decodePicture(file)
  try {
    if (Math.min(bitmap.width, bitmap.height) < MIN_EDGE) throw new WallpaperFileError('small')
    const size = fitWallpaperSize(bitmap.width, bitmap.height)
    const image = await encodeWithin(draw(bitmap, size.width, size.height), MAX_IMAGE_BYTES, [0.86, 0.74, 0.62])
    const thumbSize = fitWallpaperSize(bitmap.width, bitmap.height, THUMB_LONG_EDGE)
    const thumbCanvas = draw(bitmap, thumbSize.width, thumbSize.height)
    const thumb = await encodeWithin(thumbCanvas, MAX_THUMB_BYTES, [0.8, 0.66, 0.5])
    const pixels = thumbCanvas.getContext('2d')!.getImageData(0, 0, thumbSize.width, thumbSize.height).data
    return { image, thumb, ...size, theme: extractWallpaperTheme(pixels) }
  } finally {
    bitmap.close()
  }
}

/** Decodes with the camera orientation applied; browsers predating 'from-image' reject that option. */
async function decodePicture(file: File): Promise<ImageBitmap> {
  try {
    return await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch (error) {
    if (!(error instanceof TypeError)) throw new WallpaperFileError('decode')
  }
  try {
    return await createImageBitmap(file)
  } catch {
    throw new WallpaperFileError('decode')
  }
}

function draw(source: ImageBitmap, width: number, height: number): HTMLCanvasElement {
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const context = canvas.getContext('2d')
  if (!context) throw new WallpaperFileError('encode')
  context.imageSmoothingQuality = 'high'
  context.drawImage(source, 0, 0, width, height)
  return canvas
}

function toBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, quality))
}

async function encodeWithin(canvas: HTMLCanvasElement, maxBytes: number, qualities: number[]): Promise<Blob> {
  for (const quality of qualities) {
    let blob = await toBlob(canvas, 'image/webp', quality)
    // Browsers that cannot encode WebP hand back PNG; use JPEG there.
    if (blob && blob.type !== 'image/webp') blob = await toBlob(canvas, 'image/jpeg', quality)
    if (blob && (blob.type === 'image/webp' || blob.type === 'image/jpeg') && blob.size <= maxBytes) return blob
  }
  throw new WallpaperFileError('size')
}

interface HueBin { weight: number, r: number, g: number, b: number }

/**
 * Suggests a color scheme from RGBA pixels: the most prominent vivid hue is the
 * theme color, a clearly different second hue the accent, and the overall mean
 * (kept muted) the base tone. The theme store then derives readable light and dark
 * variants from these, as it does for hand-picked colors.
 */
export function extractWallpaperTheme(pixels: ArrayLike<number>): CustomWallpaperTheme | undefined {
  const bins: HueBin[] = Array.from({ length: 24 }, () => ({ weight: 0, r: 0, g: 0, b: 0 }))
  let totalR = 0, totalG = 0, totalB = 0, count = 0
  for (let index = 0; index + 3 < pixels.length; index += 4) {
    if (pixels[index + 3]! < 128) continue
    const r = pixels[index]!, g = pixels[index + 1]!, b = pixels[index + 2]!
    totalR += r
    totalG += g
    totalB += b
    count++
    const [hue, saturation, lightness] = rgbToHsl(r, g, b)
    if (saturation < 0.2 || lightness < 0.12 || lightness > 0.9) continue
    const weight = saturation * (1 - Math.abs(2 * lightness - 1))
    const bin = bins[Math.floor(hue / 15) % 24]!
    bin.weight += weight
    bin.r += r * weight
    bin.g += g * weight
    bin.b += b * weight
  }
  if (!count) return undefined
  const ranked = bins.map((bin, index) => ({ ...bin, hue: index * 15 + 7.5 })).filter((bin) => bin.weight > 0).sort((a, b) => b.weight - a.weight)
  const top = ranked[0]
  // Too little color to build a scheme on: keep the current colors.
  if (!top || top.weight / count < 0.01) return undefined
  const second = ranked.find((bin) => hueDistance(bin.hue, top.hue) >= 45 && bin.weight >= top.weight * 0.25)
  const vivid = (bin: HueBin) => {
    const [hue, saturation, lightness] = rgbToHsl(bin.r / bin.weight, bin.g / bin.weight, bin.b / bin.weight)
    return hslToHex(hue, Math.max(saturation, 0.45), Math.min(Math.max(lightness, 0.4), 0.6))
  }
  const [meanHue, meanSaturation, meanLightness] = rgbToHsl(totalR / count, totalG / count, totalB / count)
  const brand = vivid(top)
  return {
    brand,
    neutral: hslToHex(meanHue, Math.min(meanSaturation, 0.3), Math.min(Math.max(meanLightness, 0.3), 0.45)),
    signature: second ? vivid(second) : brand,
  }
}

function hueDistance(a: number, b: number): number {
  const distance = Math.abs(a - b) % 360
  return Math.min(distance, 360 - distance)
}

export function rgbToHsl(r: number, g: number, b: number): [number, number, number] {
  r /= 255
  g /= 255
  b /= 255
  const max = Math.max(r, g, b), min = Math.min(r, g, b)
  const lightness = (max + min) / 2
  if (max === min) return [0, 0, lightness]
  const delta = max - min
  const saturation = delta / (1 - Math.abs(2 * lightness - 1))
  let hue = (r - g) / delta + 4
  if (max === r) hue = ((g - b) / delta) % 6
  else if (max === g) hue = (b - r) / delta + 2
  hue *= 60
  if (hue < 0) hue += 360
  return [hue, Math.min(1, saturation), lightness]
}

export function hslToHex(hue: number, saturation: number, lightness: number): string {
  const chroma = (1 - Math.abs(2 * lightness - 1)) * saturation
  const x = chroma * (1 - Math.abs(((hue / 60) % 2) - 1))
  const m = lightness - chroma / 2
  // One 60° sector of the hue wheel per row: which channel carries chroma, x and 0.
  const sectors: Array<[number, number, number]> = [
    [chroma, x, 0], [x, chroma, 0], [0, chroma, x], [0, x, chroma], [x, 0, chroma], [chroma, 0, x],
  ]
  const [r, g, b] = sectors[Math.min(5, Math.max(0, Math.floor(hue / 60)))]!
  return `#${[r, g, b].map((channel) => Math.round((channel + m) * 255).toString(16).padStart(2, '0')).join('')}`
}
