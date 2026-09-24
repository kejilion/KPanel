import {
  drawSprite,
  imagePoint,
  type SceneFrame,
  type SceneSurface,
  type SceneViewport,
} from '../painterKit'

/** Night-sky pieces shared by Aurora Snowfield and Chrono Canal. */

export interface TwinkleStar {
  u: number
  v: number
  size: number
  rate: number
  phase: number
}

export interface SkyRegion {
  left: number
  right: number
  top: number
  bottom: number
}

export function scatterStars(random: () => number, count: number, region: SkyRegion, minSize: number, maxSize: number): TwinkleStar[] {
  return Array.from({ length: count }, () => ({
    u: region.left + random() * (region.right - region.left),
    v: region.top + random() * (region.bottom - region.top),
    size: minSize + random() ** 2 * (maxSize - minSize),
    rate: 0.6 + random() * 1.8,
    phase: random() * Math.PI * 2,
  }))
}

export function drawTwinkles(
  context: CanvasRenderingContext2D,
  sprite: SceneSurface,
  stars: readonly TwinkleStar[],
  viewport: SceneViewport,
  frame: SceneFrame,
  strength: number,
): void {
  if (strength <= 0.01) return
  context.globalCompositeOperation = 'lighter'
  for (const star of stars) {
    const pulse = 0.5 + 0.5 * Math.sin(frame.time * star.rate + star.phase)
    context.globalAlpha = (0.12 + 0.88 * pulse ** 4) * strength
    const point = imagePoint(viewport, star.u, star.v)
    drawSprite(context, sprite, point.x, point.y, star.size, frame.pixelRatio)
  }
  context.globalCompositeOperation = 'source-over'
  context.globalAlpha = 1
}

export interface ShootingStar {
  update(context: CanvasRenderingContext2D, viewport: SceneViewport, frame: SceneFrame, strength: number): void
}

/** A rare meteor: one gradient stroke for under a second, then a long quiet gap. */
export function createShootingStar(random: () => number, region: SkyRegion, firstDelay: number, gap: readonly [number, number]): ShootingStar {
  let wait = firstDelay
  let age = -1
  let fromU = 0
  let fromV = 0
  let direction = 1
  const duration = 0.85

  return {
    update(context, viewport, frame, strength) {
      if (age < 0) {
        wait -= frame.dt
        if (wait > 0 || strength < 0.4) return
        age = 0
        fromU = region.left + random() * (region.right - region.left)
        fromV = region.top + random() * (region.bottom - region.top)
        direction = random() < 0.5 ? -1 : 1
      }
      age += frame.dt
      const progress = age / duration
      if (progress >= 1) {
        age = -1
        wait = gap[0] + random() * (gap[1] - gap[0])
        return
      }
      const start = imagePoint(viewport, fromU, fromV)
      const travel = Math.min(viewport.width, viewport.height) * 0.22
      const headX = start.x + direction * travel * progress
      const headY = start.y + travel * 0.42 * progress
      const tail = travel * 0.55
      const tailX = headX - direction * tail
      const tailY = headY - tail * 0.42
      context.setTransform(frame.pixelRatio, 0, 0, frame.pixelRatio, 0, 0)
      const gradient = context.createLinearGradient(tailX, tailY, headX, headY)
      gradient.addColorStop(0, 'rgba(255,255,255,0)')
      gradient.addColorStop(1, 'rgba(255,255,255,0.95)')
      context.globalAlpha = Math.sin(progress * Math.PI) * strength
      context.strokeStyle = gradient
      context.lineWidth = 1.6
      context.lineCap = 'round'
      context.beginPath()
      context.moveTo(tailX, tailY)
      context.lineTo(headX, headY)
      context.stroke()
      context.globalAlpha = 1
    },
  }
}
