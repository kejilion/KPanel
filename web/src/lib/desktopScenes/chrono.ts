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

/** The phase painted before modules load; public/appearance-init.js mirrors it. */
export function dominantChronoPhase(date: Date): ChronoPhase {
  const weights = chronoWeights(date)
  return CHRONO_PHASES.reduce((best, phase) => (weights[phase] > weights[best] ? phase : best), 'golden')
}
