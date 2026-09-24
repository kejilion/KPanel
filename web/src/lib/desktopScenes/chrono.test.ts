import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it } from 'vitest'
import { CHRONO_PHASES, chronoWeightsAt, dominantChronoPhase } from './chrono'

const bootScript = readFileSync(new URL('../../../public/appearance-init.js', import.meta.url), 'utf8')

function at(hours: number, minutes = 0): Date {
  return new Date(2026, 8, 24, hours, minutes, 0)
}

/** Runs the pre-module boot script at a fixed local time and returns the poster it paints. */
function bootPoster(date: Date): string | undefined {
  const properties = new Map<string, string>()
  const FixedDate = class extends Date {
    constructor() {
      super(date.getTime())
    }
  }
  runInNewContext(bootScript, {
    Date: FixedDate,
    document: { documentElement: { dataset: {}, style: { setProperty: (key: string, value: string) => properties.set(key, value) }, classList: { add() {}, remove() {} } } },
    location: { pathname: '/' },
    matchMedia: () => ({ matches: false }),
    localStorage: { getItem: (key: string) => (key === 'kpanel:desktop-wallpaper:v1' ? 'chrono' : null) },
    sessionStorage: { getItem: () => null },
    Image: class { decode() { return new Promise(() => {}) } },
    fetch: async () => ({ ok: false }),
    window: { addEventListener() {} },
  })
  return properties.get('--desktop-wallpaper-image')
}

describe('chrono canal time of day', () => {
  it('mixes at most two adjacent phases and always sums to one', () => {
    for (let minute = 0; minute < 1440; minute += 5) {
      const weights = chronoWeightsAt(minute)
      expect(weights.day + weights.golden + weights.night).toBeCloseTo(1, 6)
      expect(weights.day > 0 && weights.night > 0).toBe(false)
    }
  })

  it.each([
    [at(2), 'night'],
    [at(5, 0), 'night'],
    [at(6, 0), 'golden'],
    [at(12, 0), 'day'],
    [at(17, 40), 'golden'],
    [at(22, 30), 'night'],
  ] as const)('paints %s as %s', (date, phase) => {
    expect(dominantChronoPhase(date)).toBe(phase)
  })

  it('keeps the boot script on the same phase for every minute of the day', () => {
    for (let minute = 0; minute < 1440; minute++) {
      const date = at(Math.floor(minute / 60), minute % 60)
      expect(bootPoster(date), `minute ${minute}`).toBe(`url("/wallpapers/scenes/chrono-${dominantChronoPhase(date)}.webp")`)
    }
  })

  it('names every painted phase', () => {
    expect([...CHRONO_PHASES].sort()).toEqual(['day', 'golden', 'night'])
  })
})
