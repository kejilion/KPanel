import { readFileSync } from 'node:fs'
import { runInNewContext } from 'node:vm'
import { describe, expect, it } from 'vitest'
import { CHRONO_PHASES, chronoEntrancePhase, chronoEntranceStart, chronoWeightsAt, dominantChronoPhase, minuteOfDay } from './chrono'

const bootScript = readFileSync(new URL('../../../public/appearance-init.js', import.meta.url), 'utf8')

function at(hours: number, minutes = 0): Date {
  return new Date(2026, 8, 24, hours, minutes, 0)
}

/** Runs the pre-module boot script at a fixed local time and returns the poster it paints. */
function bootPoster(date: Date, reducedMotion = false, motionAlways = false): string | undefined {
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
    matchMedia: (query: string) => ({ matches: reducedMotion && query.includes('reduced-motion') }),
    localStorage: { getItem: (key: string) => ({ 'kpanel:desktop-wallpaper:v1': 'chrono', 'kpanel:desktop-scene-motion:v1': motionAlways ? 'always' : null })[key] ?? null },
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

  it('keeps the boot script on the entrance phase for every minute of the day', () => {
    for (let minute = 0; minute < 1440; minute++) {
      const date = at(Math.floor(minute / 60), minute % 60)
      expect(bootPoster(date), `minute ${minute}`).toBe(`url("/wallpapers/scenes/chrono-${chronoEntrancePhase(date)}.webp")`)
      expect(bootPoster(date, true), `reduced minute ${minute}`).toBe(`url("/wallpapers/scenes/chrono-${dominantChronoPhase(date)}.webp")`)
      // Opting back in from the wallpaper dialog restores the time-lapse start under reduced motion.
      expect(bootPoster(date, true, true), `opted-in minute ${minute}`).toBe(`url("/wallpapers/scenes/chrono-${chronoEntrancePhase(date)}.webp")`)
    }
  })

  it('starts the page-load time-lapse inside the previous phase and ends at now', () => {
    for (let minute = 0; minute < 1440; minute += 7) {
      const date = at(Math.floor(minute / 60), minute % 60)
      const start = chronoEntranceStart(date)
      expect(start).toBeLessThan(minuteOfDay(date))
      expect(minuteOfDay(date) - start).toBeLessThanOrEqual(12 * 60)
      expect(chronoEntrancePhase(date)).not.toBe(dominantChronoPhase(date))
      // The start is a full-weight moment, so the boot poster equals the first stage frame.
      expect(Math.max(...Object.values(chronoWeightsAt(start)))).toBe(1)
    }
    expect(chronoEntrancePhase(at(12))).toBe('golden')
    expect(chronoEntrancePhase(at(17, 40))).toBe('day')
    expect(chronoEntrancePhase(at(6, 0))).toBe('night')
    expect(chronoEntrancePhase(at(23))).toBe('golden')
    expect(chronoEntranceStart(at(2))).toBe(18 * 60 - 1440)
  })

  it('names every painted phase', () => {
    expect([...CHRONO_PHASES].sort()).toEqual(['day', 'golden', 'night'])
  })
})
