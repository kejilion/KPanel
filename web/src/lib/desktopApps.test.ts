import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { DEFAULT_WINDOW_GRADIENT, desktopApps, findDesktopApp } from './desktopApps'

function webpDimensions(image: Buffer): { width: number; height: number } {
  expect(image.toString('ascii', 0, 4)).toBe('RIFF')
  expect(image.toString('ascii', 8, 12)).toBe('WEBP')
  let offset = 12
  while (offset + 8 <= image.length) {
    const type = image.toString('ascii', offset, offset + 4)
    const size = image.readUInt32LE(offset + 4)
    const data = offset + 8
    if (type === 'VP8 ') {
      expect(image.subarray(data + 3, data + 6)).toEqual(Buffer.from([0x9d, 0x01, 0x2a]))
      return {
        width: image.readUInt16LE(data + 6) & 0x3fff,
        height: image.readUInt16LE(data + 8) & 0x3fff,
      }
    }
    if (type === 'VP8X') {
      return {
        width: 1 + image.readUIntLE(data + 4, 3),
        height: 1 + image.readUIntLE(data + 7, 3),
      }
    }
    offset = data + size + (size % 2)
  }
  throw new Error('unsupported WebP payload')
}

describe('desktop app catalogue', () => {
  it('mirrors the classic navigation set', () => {
    const paths = desktopApps.map((app) => app.path)
    expect(paths).toEqual(
      expect.arrayContaining([
        '/overview',
        '/system',
        '/ai',
        '/sites',
        '/apps',
        '/docker',
        '/files',
        '/terminal',
        '/diagnostics',
        '/cluster',
        '/activity',
        '/settings',
      ]),
    )
    expect(desktopApps).toHaveLength(12)
  })

  it('gives every app a distinct gradient', () => {
    const gradients = desktopApps.map((app) => app.gradient.join('→'))
    const unique = new Set(gradients)
    expect(unique.size).toBe(desktopApps.length)
  })

  it('ships one unique, budgeted 512px artwork file per desktop app', () => {
    const iconURLs = desktopApps.map((app) => app.desktopIconURL)
    expect(iconURLs.every(Boolean)).toBe(true)
    expect(new Set(iconURLs).size).toBe(desktopApps.length)

    for (const iconURL of iconURLs) {
      expect(iconURL).toMatch(/^\/desktop-icons\/[a-z]+-kpanel-flat-v1\.webp$/)
      const image = readFileSync(new URL(`../../public${iconURL}`, import.meta.url))
      expect(image.byteLength).toBeLessThanOrEqual(30 * 1024)
      expect(webpDimensions(image)).toEqual({ width: 512, height: 512 })
    }
  })

  it('ships a budgeted 512px open-folder artwork for directory shortcuts', () => {
    const image = readFileSync(
      new URL('../../public/desktop-icons/folder-open-shortcut-kpanel-flat-v1.webp', import.meta.url),
    )
    expect(image.byteLength).toBeLessThanOrEqual(30 * 1024)
    expect(webpDimensions(image)).toEqual({ width: 512, height: 512 })
  })

  it('marks the terminal as single-instance', () => {
    expect(findDesktopApp('/terminal')?.allowMultiple).toBe(false)
    expect(findDesktopApp('/overview')?.allowMultiple).toBe(false)
  })

  it('maps safe dynamic script routes to a terminal window without adding a desktop launcher', () => {
    expect(findDesktopApp('/app-script/openclaw')?.labelKey).toBe('desktop.scriptWindowTitle')
    expect(findDesktopApp('/app-script/openclaw')?.allowMultiple).toBe(false)
    expect(findDesktopApp('/app-script/bad/path')).toBeUndefined()
    expect(desktopApps.map((app) => app.path)).not.toContain('/app-script')
  })

  it('exposes the system center launcher while keeping the process utility route internal', () => {
    expect(findDesktopApp('/system')?.labelKey).toBe('route.systemCenter')
    expect(findDesktopApp('/system')?.allowMultiple).toBe(false)
    expect(findDesktopApp('/processes')).toBeUndefined()
    const paths = desktopApps.map((app) => app.path)
    expect(paths.indexOf('/system')).toBe(paths.indexOf('/cluster') + 1)
  })

  it('returns undefined for unknown paths', () => {
    expect(findDesktopApp('/nope')).toBeUndefined()
  })

  it('provides a default gradient fallback', () => {
    expect(DEFAULT_WINDOW_GRADIENT).toHaveLength(2)
    expect(DEFAULT_WINDOW_GRADIENT[0]).toMatch(/^#/)
  })
})
