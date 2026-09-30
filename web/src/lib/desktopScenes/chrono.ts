/**
 * Local time of day for the Chrono Canal scene. Three painted phases share one
 * composition; at most two adjacent phases are ever mixed, so the stage can
 * keep `golden` opaque at the bottom and fade `day` or `night` above it.
 */
export const CHRONO_PHASES = ['day', 'golden', 'night'] as const
export type ChronoPhase = typeof CHRONO_PHASES[number]
export type ChronoWeights = Record<ChronoPhase, number>

const MINUTES_PER_DAY = 24 * 60
const NIGHT: ChronoWeights = { day: 0, golden: 0, night: 1 }
const GOLDEN: ChronoWeights = { day: 0, golden: 1, night: 0 }
const DAY: ChronoWeights = { day: 1, golden: 0, night: 0 }

// Minute of day → phase. Dawn and dusk both use the golden painting.
const KEYFRAMES: readonly (readonly [number, ChronoWeights])[] = [
  [0, NIGHT],
  [4 * 60 + 30, NIGHT],
  [6 * 60, GOLDEN],
  [7 * 60 + 30, DAY],
  [16 * 60 + 30, DAY],
  [18 * 60, GOLDEN],
  [19 * 60 + 30, NIGHT],
  [MINUTES_PER_DAY, NIGHT],
]

function smoothstep(value: number): number {
  return value * value * (3 - 2 * value)
}

export function minuteOfDay(date: Date): number {
  return date.getHours() * 60 + date.getMinutes() + date.getSeconds() / 60
}

export function chronoWeightsAt(minute: number): ChronoWeights {
  const wrapped = ((minute % MINUTES_PER_DAY) + MINUTES_PER_DAY) % MINUTES_PER_DAY
  for (let index = 1; index < KEYFRAMES.length; index++) {
    const [end, to] = KEYFRAMES[index]!
    if (wrapped > end) continue
    const [start, from] = KEYFRAMES[index - 1]!
    const progress = end === start ? 1 : smoothstep((wrapped - start) / (end - start))
    return {
      day: from.day + (to.day - from.day) * progress,
      golden: from.golden + (to.golden - from.golden) * progress,
      night: from.night + (to.night - from.night) * progress,
    }
  }
  return { ...NIGHT }
}

export function chronoWeights(date: Date): ChronoWeights {
  return chronoWeightsAt(minuteOfDay(date))
}

function dominantPhaseAt(minute: number): ChronoPhase {
  const weights = chronoWeightsAt(minute)
  return CHRONO_PHASES.reduce((best, phase) => (weights[phase] > weights[best] ? phase : best), 'golden')
}

/** The phase that is on screen now, and the only one painted for reduced motion. */
export function dominantChronoPhase(date: Date): ChronoPhase {
  return dominantPhaseAt(minuteOfDay(date))
}

/**
 * Minute where the page-load time-lapse starts: the middle of the previous
 * phase, so arriving at "now" is visible at any hour (dawn gold before day,
 * night before dawn, day before dusk, dusk gold before night). It can be
 * negative, meaning the evening before.
 */
export function chronoEntranceStart(date: Date): number {
  const minute = minuteOfDay(date)
  const phase = dominantPhaseAt(minute)
  if (phase === 'day') return 6 * 60
  if (phase === 'golden') return minute < 12 * 60 ? 4 * 60 + 30 : 16 * 60 + 30
  return minute >= 12 * 60 ? 18 * 60 : 18 * 60 - MINUTES_PER_DAY
}

/** The phase painted before modules load when motion is allowed; public/appearance-init.js mirrors it. */
export function chronoEntrancePhase(date: Date): ChronoPhase {
  return dominantPhaseAt(chronoEntranceStart(date))
}
