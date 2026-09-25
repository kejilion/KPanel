/**
 * A private wallpaper (an uploaded picture, or a 3D scene's poster) is only served to a
 * signed-in browser, so the sign-in page cannot load it. While signed in, this browser keeps
 * a reduced copy of it in its own storage; public/appearance-init.js shows that copy behind
 * the sign-in brand panel without asking the server. Nothing is made public: only browsers
 * that have signed in hold a copy, and it is replaced or removed with the wallpaper.
 */
export const AUTH_WALLPAPER_COPY_KEY = 'kpanel:auth-wallpaper:v1'
/** The brand panel is at most about 900 CSS pixels wide; 1280 keeps it sharp without much weight. */
const MAX_EDGE = 1280
/** Mirrors the boot script's limit; localStorage keeps a few megabytes per origin. */
export const AUTH_WALLPAPER_COPY_MAX_LENGTH = 400_000
const DATA_URL = /^data:image\/(webp|jpeg);base64,[A-Za-z0-9+/]+=*$/

export interface AuthWallpaperCopy {
  /** The wallpaper choice it belongs to (`custom:<id>` or `pack:<id>`). */
  id: string
  image: string
  focusX: number
  focusY: number
  bright: boolean
}

function inRange(value: unknown): value is number {
  return Number.isInteger(value) && (value as number) >= 0 && (value as number) <= 1000
}

export function readAuthWallpaperCopy(): AuthWallpaperCopy | undefined {
  try {
    const copy = JSON.parse(window.localStorage.getItem(AUTH_WALLPAPER_COPY_KEY) || 'null') as Partial<AuthWallpaperCopy> | null
    if (!copy || typeof copy.id !== 'string' || typeof copy.image !== 'string' || typeof copy.bright !== 'boolean') return undefined
    if (copy.image.length > AUTH_WALLPAPER_COPY_MAX_LENGTH || !DATA_URL.test(copy.image) || !inRange(copy.focusX) || !inRange(copy.focusY)) return undefined
    return copy as AuthWallpaperCopy
  } catch {
    return undefined
  }
}

export function forgetAuthWallpaperCopy(): void {
  try {
    window.localStorage.removeItem(AUTH_WALLPAPER_COPY_KEY)
  } catch {
    // Nothing is stored where storage is unavailable.
  }
}

function toBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, quality))
}

function toDataURL(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error)
    reader.readAsDataURL(blob)
  })
}

/** Mean perceived brightness of a tiny downscale, 0–1. */
function brightness(source: ImageBitmap): number {
  const canvas = document.createElement('canvas')
  canvas.width = 16
  canvas.height = 16
  const context = canvas.getContext('2d')
  if (!context) return 0
  context.drawImage(source, 0, 0, 16, 16)
  const pixels = context.getImageData(0, 0, 16, 16).data
  let sum = 0
  for (let index = 0; index < pixels.length; index += 4) {
    sum += 0.2126 * pixels[index]! + 0.7152 * pixels[index + 1]! + 0.0722 * pixels[index + 2]!
  }
  return sum / (pixels.length / 4) / 255
}

/**
 * Downloads the signed-in image at `url`, keeps a reduced copy for `id` and asks the boot
 * script to show it. `stillChosen` guards against the choice changing while this runs.
 * Failures leave the sign-in page on the default wallpaper.
 */
export async function rememberAuthWallpaperCopy(
  id: string,
  url: string,
  focus: { focusX: number, focusY: number },
  stillChosen: () => boolean,
): Promise<boolean> {
  try {
    const response = await fetch(url, { credentials: 'same-origin' })
    if (!response.ok) return false
    const bitmap = await createImageBitmap(await response.blob())
    try {
      const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height))
      const canvas = document.createElement('canvas')
      canvas.width = Math.max(1, Math.round(bitmap.width * scale))
      canvas.height = Math.max(1, Math.round(bitmap.height * scale))
      const context = canvas.getContext('2d')
      if (!context) return false
      context.imageSmoothingQuality = 'high'
      context.drawImage(bitmap, 0, 0, canvas.width, canvas.height)
      let image = ''
      for (const quality of [0.78, 0.62, 0.48]) {
        let blob = await toBlob(canvas, 'image/webp', quality)
        if (blob && blob.type !== 'image/webp') blob = await toBlob(canvas, 'image/jpeg', quality)
        if (!blob) continue
        const encoded = await toDataURL(blob)
        if (encoded.length <= AUTH_WALLPAPER_COPY_MAX_LENGTH && DATA_URL.test(encoded)) {
          image = encoded
          break
        }
      }
      if (!image || !stillChosen()) return false
      const copy: AuthWallpaperCopy = {
        id, image, focusX: focus.focusX, focusY: focus.focusY, bright: brightness(bitmap) >= 0.5,
      }
      window.localStorage.setItem(AUTH_WALLPAPER_COPY_KEY, JSON.stringify(copy))
      window.dispatchEvent(new Event('kpanel:cache-desktop-wallpaper'))
      return true
    } finally {
      bitmap.close()
    }
  } catch {
    return false
  }
}
