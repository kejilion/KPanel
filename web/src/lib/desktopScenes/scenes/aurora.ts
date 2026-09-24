import {
  clamp,
  drawSprite,
  easeOutCubic,
  glintSprite,
  glowSprite,
  type ScenePainterFactory,
  type SceneViewport,
} from '../painterKit'
import { createShootingStar, drawTwinkles, scatterStars } from './sky'

const FLAKES_PER_MEGAPIXEL = 66
const MAX_FLAKES = 240
const SKY = { left: 0.04, right: 0.96, top: 0.03, bottom: 0.44 } as const

interface Flake {
  x: number
  y: number
  depth: number
  fall: number
  sway: number
  swayRate: number
  phase: number
}

/** Aurora Snowfield: slow three-depth snowfall under a twinkling sky; the aurora itself is CSS. */
const createAuroraPainter: ScenePainterFactory = (environment) => {
  const { random } = environment
  const flakeSprite = glowSprite(environment, 32, [
    [0, 'rgba(255,255,255,1)'],
    [0.35, 'rgba(236,244,255,0.85)'],
    [1, 'rgba(220,236,255,0)'],
  ])
  const starSprite = glintSprite(environment, 32, 'rgba(206,226,255,0.85)')
  const stars = scatterStars(random, 18, SKY, 7, 15)
  const meteor = createShootingStar(random, { left: 0.18, right: 0.82, top: 0.05, bottom: 0.2 }, 2.4, [14, 28])
  const flakes: Flake[] = []
  let viewport: SceneViewport = { width: 0, height: 0, image: { x: 0, y: 0, width: 0, height: 0 } }
  let density = 1

  function respawn(flake: Flake, anywhere: boolean): void {
    flake.depth = 0.35 + random() * 0.85
    flake.fall = 10 + flake.depth * 34
    flake.sway = 6 + flake.depth * 18
    flake.swayRate = 0.25 + random() * 0.45
    flake.phase = random() * Math.PI * 2
    flake.x = random() * viewport.width
    flake.y = anywhere ? random() * viewport.height : -10 - random() * 40
  }

  function reconcile(): void {
    const target = Math.min(MAX_FLAKES, Math.round((viewport.width * viewport.height) / 1e6 * FLAKES_PER_MEGAPIXEL * density))
    while (flakes.length < target) {
      const flake = {} as Flake
      respawn(flake, true)
      flakes.push(flake)
    }
    flakes.length = Math.min(flakes.length, target)
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
    frame(context, frame) {
      const arrival = easeOutCubic(frame.entrance)
      if (starSprite) drawTwinkles(context, starSprite, stars, viewport, frame, arrival)
      meteor.update(context, viewport, frame, arrival)
      if (!flakeSprite) return
      const scale = clamp(viewport.width / 1920, 0.8, 1.4)
      const drift = 6 * Math.sin(frame.time * 0.12)
      for (const flake of flakes) {
        flake.phase += flake.swayRate * frame.dt
        flake.y += flake.fall * frame.dt
        flake.x += drift * flake.depth * frame.dt
        if (flake.y > viewport.height + 12) respawn(flake, false)
        context.globalAlpha = (0.35 + flake.depth * 0.5) * arrival
        drawSprite(
          context,
          flakeSprite,
          flake.x + Math.sin(flake.phase) * flake.sway,
          flake.y,
          (2.4 + flake.depth * 5.2) * scale,
          frame.pixelRatio,
        )
      }
      context.globalAlpha = 1
    },
  }
}

export default createAuroraPainter
