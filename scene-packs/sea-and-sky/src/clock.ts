import { dayOfYear, localHour, moonAge, SUNRISE, SUNSET } from './daylight'

/**
 * The scene's clock. By default a whole day passes in ten minutes, starting
 * from the local time when the scene opens; it slows down about fourfold around
 * sunrise and sunset so the golden hour and the afterglow last, runs faster
 * through midday and the dead of night, and also slows while the moon rises or
 * sets (so a day with a moonrise and a moonset in it runs a little longer). Each turn is a new day, so the moon
 * rises later from one turn to the next, as it does. ?timelapse=off keeps to
 * the real clock instead (with ?hour= and ?moon= freezing it, for previews).
 */
export const DAY_SECONDS = 600
const LINGER = 3
const LINGER_HOURS = 0.9

const weight = (hour: number) => {
  let near = 0
  for (const event of [SUNRISE, SUNSET]) {
    const offset = ((hour - event + 36) % 24) - 12
    near += Math.exp(-((offset / LINGER_HOURS) ** 2))
  }
  return 1 + LINGER * near
}
/** Real seconds per simulated hour at the base rate, so the weighted day adds up to DAY_SECONDS. */
const SECONDS_PER_HOUR = DAY_SECONDS / (24 + LINGER * 2 * LINGER_HOURS * Math.sqrt(Math.PI))

export interface SceneTime {
  hour: number
  moonAge: number
  day: number
}

export function createClock(options: { timelapse: boolean, hour?: number, moon?: number }) {
  const now = new Date()
  const startHour = localHour(now, options.hour)
  const startAge = moonAge(now, options.moon)
  const startDay = dayOfYear(now)
  let hours = 0
  const time: SceneTime = { hour: startHour, moonAge: startAge, day: startDay }
  return {
    time,
    /** extraWeight: further slowing, e.g. while the moon is at the horizon (0 for none). */
    advance(dt: number, extraWeight = 0): SceneTime {
      if (!options.timelapse) {
        const date = new Date()
        time.hour = localHour(date, options.hour)
        time.moonAge = moonAge(date, options.moon)
        time.day = dayOfYear(date)
        return time
      }
      hours += dt / (SECONDS_PER_HOUR * (weight(time.hour) + extraWeight))
      time.hour = (startHour + hours) % 24
      time.moonAge = startAge + hours / 24
      time.day = startDay + hours / 24
      return time
    },
  }
}
