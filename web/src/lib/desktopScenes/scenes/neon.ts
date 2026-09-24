import {
  clamp,
  easeOutCubic,
  imagePoint,
  type ScenePainterFactory,
  type SceneViewport,
} from '../painterKit'

const DROPS_PER_MEGAPIXEL = 200
const MAX_DROPS = 720
const STREET_TOP = 0.8
const STREET_BOTTOM = 0.99
const MAX_RIPPLES = 48

interface Drop {
  x: number
  y: number
  speed: number
  length: number
  near: boolean
}

interface Ripple {
  x: number
  y: number
  age: number
  life: number
  radius: number
}

/**
 * Neon Rain: two depths of rain streaks drawn as one path each, plus ripples on
 * the wet street. The entrance lets the shower build up from a drizzle.
 */
const createNeonPainter: ScenePainterFactory = ({ random }) => {
  const drops: Drop[] = []
  const ripples: Ripple[] = []
  let viewport: SceneViewport = { width: 0, height: 0, image: { x: 0, y: 0, width: 0, height: 0 } }
  let density = 1
  let rippleBudget = 0

  function respawn(drop: Drop, anywhere: boolean): void {
    drop.near = random() < 0.42
    drop.speed = drop.near ? 1050 + random() * 350 : 640 + random() * 220
    drop.length = drop.near ? 22 + random() * 18 : 10 + random() * 9
    drop.x = random() * viewport.width * 1.25 - viewport.width * 0.2
    drop.y = anywhere ? random() * viewport.height : -drop.length - random() * viewport.height * 0.25
  }

  function reconcile(): void {
    const target = Math.min(MAX_DROPS, Math.round((viewport.width * viewport.height) / 1e6 * DROPS_PER_MEGAPIXEL * density))
    while (drops.length < target) {
      const drop = {} as Drop
      respawn(drop, true)
      drops.push(drop)
    }
    drops.length = Math.min(drops.length, target)
  }

  function strokeLayer(context: CanvasRenderingContext2D, near: boolean, visible: number, slant: number): void {
    context.beginPath()
    for (let index = 0; index < visible; index++) {
      const drop = drops[index]!
      if (drop.near !== near) continue
      context.moveTo(drop.x, drop.y)
      context.lineTo(drop.x - slant * drop.length, drop.y - drop.length)
    }
    context.strokeStyle = near ? 'rgba(222, 232, 255, 0.52)' : 'rgba(186, 202, 255, 0.3)'
    context.lineWidth = near ? 1.5 : 1
    context.stroke()
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
      const intensity = Math.max(0.08, easeOutCubic(frame.entrance))
      const slant = 0.16 + 0.05 * Math.sin(frame.time * 0.31)
      const visible = Math.round(drops.length * intensity)
      for (const drop of drops) {
        drop.y += drop.speed * frame.dt
        drop.x += drop.speed * frame.dt * slant
        if (drop.y - drop.length > viewport.height) respawn(drop, false)
      }
      context.setTransform(frame.pixelRatio, 0, 0, frame.pixelRatio, 0, 0)
      context.lineCap = 'round'
      strokeLayer(context, false, visible, slant)
      strokeLayer(context, true, visible, slant)

      // Ripples follow street perspective: larger and flatter near the viewer.
      rippleBudget += frame.dt * 22 * density * intensity * clamp(viewport.width / 1920, 0.5, 1.6)
      while (rippleBudget >= 1 && ripples.length < MAX_RIPPLES) {
        rippleBudget -= 1
        const v = STREET_TOP + (STREET_BOTTOM - STREET_TOP) * random() ** 0.7
        const point = imagePoint(viewport, random(), v)
        const depth = (v - STREET_TOP) / (STREET_BOTTOM - STREET_TOP)
        ripples.push({ x: point.x, y: point.y, age: 0, life: 0.45 + random() * 0.45, radius: 5 + depth * 12 })
      }
      rippleBudget = Math.min(rippleBudget, 1)
      context.lineWidth = 1
      for (let index = ripples.length - 1; index >= 0; index--) {
        const ripple = ripples[index]!
        ripple.age += frame.dt
        const progress = ripple.age / ripple.life
        if (progress >= 1) {
          ripples.splice(index, 1)
          continue
        }
        const radius = ripple.radius * easeOutCubic(progress)
        context.globalAlpha = (1 - progress) * 0.55
        context.strokeStyle = 'rgba(206, 218, 255, 1)'
        context.beginPath()
        context.ellipse(ripple.x, ripple.y, radius, radius * 0.28, 0, 0, Math.PI * 2)
        context.stroke()
      }
      context.globalAlpha = 1
    },
  }
}

export default createNeonPainter
