import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'
import { zhCNMessages } from '@/i18n/messages/zh-CN'
import { zhTWMessages } from '@/i18n/messages/zh-TW'
import { enUSMessages } from '@/i18n/messages/en-US'
import { DESKTOP_SCENES, DESKTOP_SCENE_IDS } from './catalog'
import { sceneModules } from './loaders'
import { celestialAt, skyAt } from './scenes/shared'
import type { SceneContext2D, SceneSurface } from './types'

interface Recorder {
  gradients: number
  drawImages: number
}

// A permissive 2D context stand-in that counts the calls with a real per-frame cost.
function fakeContext(recorder: Recorder): SceneContext2D {
  const gradient = { addColorStop() {} }
  return new Proxy({} as Record<string | symbol, unknown>, {
    get(target, property) {
      if (property in target) return target[property]
      if (property === 'createLinearGradient' || property === 'createRadialGradient') {
        return () => {
          recorder.gradients++
          return gradient
        }
      }
      if (property === 'drawImage') return () => { recorder.drawImages++ }
      return () => undefined
    },
    set(target, property, value) {
      target[property] = value
      return true
    },
  }) as unknown as SceneContext2D
}

function fakeSurface(recorder: Recorder, width: number, height: number): SceneSurface {
  const context = fakeContext(recorder)
  return { width, height, getContext: () => context } as unknown as SceneSurface
}

describe('desktop scene catalog', () => {
  it('keeps the catalog, boot script, posters and translations in sync', () => {
    expect(DESKTOP_SCENES.map((scene) => scene.id)).toEqual([...DESKTOP_SCENE_IDS])
    const boot = readFileSync(new URL('../../../public/appearance-init.js', import.meta.url), 'utf8')
    const bootIDs = /\[('daylight'[^\]]*)\]\.includes\(scene\)/.exec(boot)?.[1]
    expect(bootIDs?.split(',').map((id) => id.trim().slice(1, -1))).toEqual([...DESKTOP_SCENE_IDS])
    const styles = readFileSync(new URL('../../styles/desktopWallpaper.css', import.meta.url), 'utf8')
    for (const scene of DESKTOP_SCENES) {
      expect(styles).toMatch(new RegExp(`\\[data-desktop-scene='${scene.id}'\\] \\{\\s*--desktop-scene-poster:`))
      expect(styles.slice(styles.indexOf(`[data-desktop-scene='${scene.id}']`)).split('}')[0]).not.toMatch(/url\(/)
      for (const messages of [zhCNMessages, zhTWMessages, enUSMessages] as Array<Record<string, string>>) {
        expect(messages[scene.nameKey]).toBeTruthy()
        expect(messages[scene.descriptionKey]).toBeTruthy()
      }
    }
  })
})

describe('time of day', () => {
  it('interpolates the sky continuously around midnight', () => {
    const before = skyAt(23.99)
    const after = skyAt(0.01)
    before.top.forEach((channel, index) => expect(Math.abs(channel - after.top[index]!)).toBeLessThan(1))
    expect(skyAt(12).light).toBe(1)
    expect(skyAt(0).stars).toBe(1)
    expect(skyAt(12).stars).toBe(0)
  })

  it('places the sun by day and the moon by night', () => {
    expect(celestialAt(12.3)).toMatchObject({ kind: 'sun' })
    expect(celestialAt(12.3).altitude).toBeCloseTo(1, 1)
    expect(celestialAt(6.5).x).toBeLessThan(celestialAt(17).x)
    expect(celestialAt(0.6)).toMatchObject({ kind: 'moon' })
    expect(celestialAt(0.6).altitude).toBeGreaterThan(0.8)
    expect(celestialAt(19).altitude).toBeLessThan(0.3)
  })
})

describe.each(DESKTOP_SCENE_IDS)('%s scene', (sceneID) => {
  it('renders around the clock and keeps expensive gradients out of steady frames', async () => {
    const recorder: Recorder = { gradients: 0, drawImages: 0 }
    const module = await sceneModules.load(sceneID)
    const scene = module.createScene(fakeContext(recorder), {
      createSurface: (width, height) => fakeSurface(recorder, width, height),
    })
    scene.resize(1920, 1080)
    for (const hour of [0.5, 5.8, 7, 12, 18.2, 19.4, 22]) {
      expect(() => scene.render({ time: hour * 10, delta: 1 / 30, hour })).not.toThrow()
    }
    const warm = recorder.gradients
    for (let frame = 1; frame <= 60; frame++) scene.render({ time: 300 + frame / 30, delta: 1 / 30, hour: 22 })
    // Layers stay cached within a minute; only a passing shooting star builds a trail gradient.
    expect(recorder.gradients - warm).toBeLessThanOrEqual(sceneID === 'aurora' ? 30 : 0)
    expect(recorder.drawImages).toBeGreaterThan(0)

    scene.resize(640, 360)
    expect(() => scene.render({ time: 400, delta: 1 / 30, hour: 9 })).not.toThrow()
  })
})
