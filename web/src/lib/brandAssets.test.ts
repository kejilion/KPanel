import { existsSync, readFileSync, statSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const publicDir = fileURLToPath(new URL('../../public/', import.meta.url))
const manifestPath = fileURLToPath(new URL('../../public/manifest.webmanifest', import.meta.url))
const indexPath = fileURLToPath(new URL('../../index.html', import.meta.url))

describe('KPanel brand assets', () => {
  it('keeps every manifest icon local and lightweight', () => {
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8')) as {
      icons: Array<{ src: string; type: string; purpose: string }>
    }

    expect(manifest.icons).toHaveLength(3)
    expect(manifest.icons.some((icon) => icon.purpose.includes('maskable'))).toBe(true)

    for (const icon of manifest.icons) {
      const assetPath = `${publicDir}${icon.src.replace(/^\//, '')}`
      expect(existsSync(assetPath), `${icon.src} should exist`).toBe(true)
      expect(statSync(assetPath).size, `${icon.src} should stay lightweight`).toBeLessThan(30_000)
      expect(icon.type).toMatch(/^image\//)
    }
  })

  it('publishes stable favicon, Apple and manifest references', () => {
    const html = readFileSync(indexPath, 'utf8')

    expect(html).toContain('href="/icons/kpanel.svg?v=20260829"')
    expect(html).toContain('href="/icons/favicon-96.png?v=20260829"')
    expect(html).toContain('href="/icons/apple-touch-icon.png"')
    expect(html).toContain('href="/manifest.webmanifest"')
  })
})
