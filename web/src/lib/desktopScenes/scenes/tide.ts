import {
  clamp,
  drawSprite,
  easeOutCubic,
  gaussian,
  glintSprite,
  imagePoint,
  lerp,
  type ScenePainterFactory,
  type SceneViewport,
} from '../painterKit'

/** Poster anchors in normalized image coordinates (tide.webp). */
const TIDE_SUN = { u: 0.68, v: 0.387 } as const
const HORIZON = 0.426
const SAND_EDGE = 0.93
const OPEN_SEA_EDGE = 0.7
const GLINTS_PER_MEGAPIXEL = 150
const MAX_GLINTS = 380

interface Glint {
  u: number
  v: number
  age: number
  life: number
  scale: number
  column: boolean
}

interface Gull {
  x: number
  y: number
  vx: number
  size: number
  phase: number
  flapRate: number
  bob: number
}

/**
 * Tide Shore: sun glitter blooms outward from the reflection path; on entrance the
 * path reaches from the horizon down to the beach. Gulls cross now and then.
 */
const createTidePainter: ScenePainterFactory = (environment) => {
  const { random } = environment
  const sprite = glintSprite(environment, 48, 'rgba(255,236,190,1)')
  const glints: Glint[] = []
  const gulls: Gull[] = []
  let viewport: SceneViewport = { width: 0, height: 0, image: { x: 0, y: 0, width: 0, height: 0 } }
  let density = 1
  let nextFlock = 1.1

  function respawn(glint: Glint, spread: number, reach = 1): void {
    const depth = random() ** 1.5 * reach
    glint.v = HORIZON + 0.004 + (SAND_EDGE - HORIZON) * depth
    glint.column = random() < 0.74 || glint.v > OPEN_SEA_EDGE
    if (glint.column) {
      const sigma = (0.012 + 0.075 * depth) * spread
      glint.u = TIDE_SUN.u - 0.012 * depth + gaussian(random) * sigma
    } else {
      glint.u = random()
    }
    glint.scale = lerp(0.35, 1, depth) * (0.7 + random() * 0.6)
    glint.life = 0.35 + random() * 0.8
    glint.age = random() * glint.life
  }

  function reconcile(): void {
    const target = Math.min(MAX_GLINTS, Math.round((viewport.width * viewport.height) / 1e6 * GLINTS_PER_MEGAPIXEL * density))
    while (glints.length < target) {
      const glint = {} as Glint
      respawn(glint, 1)
      glints.push(glint)
    }
    glints.length = Math.min(glints.length, target)
  }

  function launchFlock(): void {
    const leftward = random() < 0.5
    const count = 1 + Math.floor(random() * 3)
    const base = imagePoint(viewport, 0, 0.1 + random() * 0.2)
    const depth = 0.6 + random() * 0.5
    for (let index = 0; index < count; index++) {
      const size = clamp(viewport.width / 95, 12, 26) * depth * (0.85 + random() * 0.3)
      gulls.push({
        x: leftward ? viewport.width + 40 + index * size * 2.4 : -40 - index * size * 2.4,
        y: base.y + (random() - 0.5) * size * 3,
        vx: (leftward ? -1 : 1) * (48 + random() * 30) * depth,
        size,
        phase: random() * Math.PI * 2,
        flapRate: 7 + random() * 3,
        bob: random() * Math.PI * 2,
      })
    }
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
      const { image } = viewport
      const arrival = easeOutCubic(frame.entrance)
      const spread = lerp(0.18, 1, arrival)
      const reach = lerp(0.1, 1, arrival)
      const glitterSize = clamp(image.width / 100, 10, 28)
      if (sprite) {
        context.globalCompositeOperation = 'lighter'
        for (const glint of glints) {
          glint.age += frame.dt
          if (glint.age >= glint.life) {
            respawn(glint, spread, reach)
            glint.age = 0
          }
          const wave = Math.sin((glint.age / glint.life) * Math.PI)
          const x = image.x + glint.u * image.width
          const y = image.y + glint.v * image.height
          if (x < -20 || x > viewport.width + 20 || y > viewport.height + 20) continue
          context.globalAlpha = wave * wave * (glint.column ? 1 : 0.7) * spread
          drawSprite(context, sprite, x, y, glitterSize * glint.scale, frame.pixelRatio)
        }
        context.globalCompositeOperation = 'source-over'
        context.globalAlpha = 1
      }

      nextFlock -= frame.dt
      if (nextFlock <= 0) {
        launchFlock()
        nextFlock = 11 + random() * 16
      }
      if (!gulls.length) return
      context.setTransform(frame.pixelRatio, 0, 0, frame.pixelRatio, 0, 0)
      context.strokeStyle = 'rgba(62, 38, 58, 0.72)'
      context.lineCap = 'round'
      for (let index = gulls.length - 1; index >= 0; index--) {
        const gull = gulls[index]!
        gull.x += gull.vx * frame.dt
        gull.phase += gull.flapRate * frame.dt
        gull.bob += frame.dt * 1.4
        const departed = gull.vx < 0 ? gull.x < -80 - gull.size * 8 : gull.x > viewport.width + 80 + gull.size * 8
        if (departed) {
          gulls.splice(index, 1)
          continue
        }
        // Flap in bursts and glide between them, like gulls riding the sea breeze.
        const flapping = Math.sin(gull.phase * 0.18) > -0.2
        const lift = flapping ? Math.sin(gull.phase) : 0.35
        const y = gull.y + Math.sin(gull.bob) * gull.size * 0.25
        const span = gull.size
        context.lineWidth = Math.max(1.1, span * 0.09)
        context.beginPath()
        context.moveTo(gull.x - span, y - span * 0.18 * lift)
        context.quadraticCurveTo(gull.x - span * 0.45, y - span * 0.5 * lift - span * 0.12, gull.x, y)
        context.quadraticCurveTo(gull.x + span * 0.45, y - span * 0.5 * lift - span * 0.12, gull.x + span, y - span * 0.18 * lift)
        context.stroke()
      }
    },
  }
}

export default createTidePainter
