import {
  clamp,
  drawSprite,
  easeOutCubic,
  glintSprite,
  glowSprite,
  imagePoint,
  type ScenePainterFactory,
  type SceneViewport,
} from '../painterKit'
import { createShootingStar, drawTwinkles, scatterStars } from './sky'

const SKY = { left: 0.12, right: 0.86, top: 0.02, bottom: 0.3 } as const
const BANKS = { left: 0.04, right: 0.96, top: 0.56, bottom: 0.9 } as const
const CANAL = { left: 0.28, right: 0.74, top: 0.68, bottom: 0.98 } as const

interface Firefly {
  u: number
  v: number
  radiusU: number
  radiusV: number
  rateU: number
  rateV: number
  phase: number
  pulse: number
}

interface Ripple {
  u: number
  v: number
  age: number
  life: number
  scale: number
}

/**
 * Chrono Canal: the painter follows the phase weights supplied by the scene —
 * stars and fireflies at night, warm fireflies at dusk, light on the canal by day.
 */
const createChronoPainter: ScenePainterFactory = (environment) => {
  const { random } = environment
  const starSprite = glintSprite(environment, 24, 'rgba(214,226,255,0.8)')
  const fireflySprite = glowSprite(environment, 32, [
    [0, 'rgba(255,250,210,1)'],
    [0.3, 'rgba(255,214,120,0.75)'],
    [1, 'rgba(255,190,90,0)'],
  ])
  const glintDay = glintSprite(environment, 32, 'rgba(255,255,255,0.8)')
  const stars = scatterStars(random, 46, SKY, 4, 10)
  const meteor = createShootingStar(random, { left: 0.2, right: 0.7, top: 0.04, bottom: 0.16 }, 6, [22, 44])
  const fireflies: Firefly[] = []
  const ripples: Ripple[] = []
  let viewport: SceneViewport = { width: 0, height: 0, image: { x: 0, y: 0, width: 0, height: 0 } }
  let density = 1
  let weights = { day: 1, golden: 0, night: 0 }

  function newRipple(ripple: Ripple): void {
    ripple.u = CANAL.left + random() * (CANAL.right - CANAL.left)
    ripple.v = CANAL.top + random() ** 0.8 * (CANAL.bottom - CANAL.top)
    ripple.life = 0.5 + random() * 0.9
    ripple.age = random() * ripple.life
    ripple.scale = 0.5 + (ripple.v - CANAL.top) / (CANAL.bottom - CANAL.top)
  }

  function reconcile(): void {
    const fireflyTarget = Math.round(26 * density)
    while (fireflies.length < fireflyTarget) {
      fireflies.push({
        u: BANKS.left + random() * (BANKS.right - BANKS.left),
        v: BANKS.top + random() * (BANKS.bottom - BANKS.top),
        radiusU: 0.008 + random() * 0.02,
        radiusV: 0.006 + random() * 0.015,
        rateU: 0.15 + random() * 0.35,
        rateV: 0.2 + random() * 0.4,
        phase: random() * Math.PI * 2,
        pulse: 0.6 + random() * 1.4,
      })
    }
    fireflies.length = Math.min(fireflies.length, fireflyTarget)
    const rippleTarget = Math.round(30 * density)
    while (ripples.length < rippleTarget) {
      const ripple = {} as Ripple
      newRipple(ripple)
      ripples.push(ripple)
    }
    ripples.length = Math.min(ripples.length, rippleTarget)
  }

  return {
    resize(next) {
      viewport = next
      reconcile()
    },
    setDensity(next) {
      density = next
      reconcile()
    },
    setParams(params) {
      weights = {
        day: clamp(params.day ?? 0, 0, 1),
        golden: clamp(params.golden ?? 0, 0, 1),
        night: clamp(params.night ?? 0, 0, 1),
      }
    },
    frame(context, frame) {
      const arrival = easeOutCubic(frame.entrance)
      if (starSprite) drawTwinkles(context, starSprite, stars, viewport, frame, weights.night * arrival)
      meteor.update(context, viewport, frame, weights.night * arrival)

      const glowScale = clamp(viewport.image.width / 1920, 0.75, 1.5)
      const waterLight = weights.day * 0.75 + weights.golden * 0.55
      if (glintDay && waterLight > 0.02) {
        context.globalCompositeOperation = 'lighter'
        for (const ripple of ripples) {
          ripple.age += frame.dt
          if (ripple.age >= ripple.life) {
            newRipple(ripple)
            ripple.age = 0
          }
          const wave = Math.sin((ripple.age / ripple.life) * Math.PI)
          context.globalAlpha = wave * wave * waterLight * arrival * 0.8
          const point = imagePoint(viewport, ripple.u, ripple.v)
          drawSprite(context, glintDay, point.x, point.y, 12 * ripple.scale * glowScale, frame.pixelRatio)
        }
      }

      const fireflyLight = weights.night * 0.95 + weights.golden * 0.4
      if (fireflySprite && fireflyLight > 0.02) {
        context.globalCompositeOperation = 'lighter'
        for (const firefly of fireflies) {
          const u = firefly.u + Math.sin(frame.time * firefly.rateU + firefly.phase) * firefly.radiusU
          const v = firefly.v + Math.cos(frame.time * firefly.rateV + firefly.phase * 1.7) * firefly.radiusV
          const pulse = 0.35 + 0.65 * (0.5 + 0.5 * Math.sin(frame.time * firefly.pulse + firefly.phase)) ** 2
          context.globalAlpha = pulse * fireflyLight * arrival
          const point = imagePoint(viewport, u, v)
          drawSprite(context, fireflySprite, point.x, point.y, 13 * glowScale, frame.pixelRatio)
        }
      }
      context.globalCompositeOperation = 'source-over'
      context.globalAlpha = 1
    },
  }
}

export default createChronoPainter
