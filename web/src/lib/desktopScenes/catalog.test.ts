import { existsSync, readFileSync, statSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { enUSMessages } from '@/i18n/messages/en-US'
import { zhCNMessages } from '@/i18n/messages/zh-CN'
import { zhTWMessages } from '@/i18n/messages/zh-TW'
import {
  DESKTOP_SCENE_ASSET_ROOT,
  DESKTOP_SCENE_IDS,
  DESKTOP_SCENES,
  chronoPoster,
  desktopScenePoster,
  isDesktopSceneID,
} from './catalog'
import { CHRONO_PHASES } from './chrono'

const publicRoot = new URL('../../../public', import.meta.url)
const bootScript = readFileSync(new URL('appearance-init.js', `${publicRoot.href}/`), 'utf8')
const assetPath = (url: string) => new URL(`.${url}`, `${publicRoot.href}/`)
const kilobytes = (url: string) => statSync(assetPath(url)).size / 1024

describe('desktop scene catalog', () => {
  it('offers five distinct scenes', () => {
    expect(DESKTOP_SCENES.map((scene) => scene.id)).toEqual([...DESKTOP_SCENE_IDS])
    expect(new Set(DESKTOP_SCENE_IDS).size).toBe(5)
    expect(isDesktopSceneID('tide')).toBe(true)
    expect(isDesktopSceneID('classic')).toBe(false)
    expect(isDesktopSceneID('https://example.com/x.webp')).toBe(false)
  })

  it('keeps the pre-module allow-list in step with the catalog', () => {
    const list = bootScript.match(/\[('chrono'[^\]]*)\]\.includes\(stored\)/)?.[1]
    expect(list?.split(',').map((value) => value.trim().replace(/'/g, ''))).toEqual([...DESKTOP_SCENE_IDS])
  })

  it('ships every same-origin asset inside its download budget', () => {
    const posters = [
      ...DESKTOP_SCENE_IDS.filter((id) => id !== 'chrono').map((id) => desktopScenePoster(id)),
      ...CHRONO_PHASES.map(chronoPoster),
    ]
    for (const poster of posters) {
      expect(poster.startsWith(`${DESKTOP_SCENE_ASSET_ROOT}/`)).toBe(true)
      expect(existsSync(assetPath(poster)), poster).toBe(true)
      expect(kilobytes(poster), poster).toBeLessThanOrEqual(320)
    }
    for (const scene of DESKTOP_SCENES) {
      expect(kilobytes(scene.thumb), scene.thumb).toBeLessThanOrEqual(32)
      for (const layer of scene.layers) {
        if (layer.image) expect(kilobytes(layer.image), layer.image).toBeLessThanOrEqual(100)
      }
    }
  })

  it('anchors layers near the poster and uses valid theme colors', () => {
    for (const scene of DESKTOP_SCENES) {
      expect(scene.entranceSeconds).toBeGreaterThan(1)
      expect(scene.entranceSeconds).toBeLessThanOrEqual(5)
      for (const layer of scene.layers) {
        expect(layer.x).toBeGreaterThanOrEqual(0)
        expect(layer.x).toBeLessThanOrEqual(1)
        expect(layer.w).toBeGreaterThan(0)
        expect(layer.h).toBeGreaterThan(0)
        expect(layer.id).toMatch(/^[a-z-]+$/)
      }
      for (const color of [scene.colors.brand, scene.colors.neutral, scene.colors.signature]) {
        expect(color).toMatch(/^#[0-9a-f]{6}$/)
      }
    }
  })

  it('names every scene in all three locales', () => {
    for (const messages of [zhCNMessages, zhTWMessages, enUSMessages] as Record<string, string>[]) {
      for (const scene of DESKTOP_SCENES) {
        expect(messages[scene.nameKey]).toBeTruthy()
        expect(messages[scene.descriptionKey]).toBeTruthy()
      }
      for (const key of ['desktop.wallpaperStaticTitle', 'desktop.wallpaperSceneTitle', 'desktop.wallpaperSceneHint', 'desktop.wallpaperSceneBadge']) {
        expect(messages[key]).toBeTruthy()
      }
    }
  })
})
