import type { SceneContext2D, SceneModule, SceneSurface } from '../types'
import { TAU, createLayer, createStars, drawStars, glowSprite, hex, random, type Layer, type Star } from './shared'

// A misty pine forest at blue hour with drifting fog and fireflies weaving between the trees.

interface Firefly {
  x: number
  y: number
  rangeX: number
  rangeY: number
  speed: number
  phase: number
  blink: number
  size: number
}

function pines(
  context: SceneContext2D, seed: number, width: number, baseline: number,
  minHeight: number, maxHeight: number, spacing: number, include: (u: number) => boolean = () => true,
): void {
  const next = random(seed)
  context.beginPath()
  for (let x = -spacing; x < width + spacing; x += spacing * (0.55 + next() * 0.9)) {
    const treeHeight = minHeight + next() * (maxHeight - minHeight)
    const half = treeHeight * (0.17 + next() * 0.08)
    const tiers = 5 + Math.floor(next() * 3)
    if (!include(x / width)) continue
    for (let tier = 0; tier < tiers; tier++) {
      const top = baseline - treeHeight + (treeHeight * tier) / (tiers + 1.4)
      const reach = half * (0.35 + (tier + 1) / tiers * 0.65)
      const bottom = top + treeHeight / (tiers - 0.5)
      context.moveTo(x, top)
      context.lineTo(x + reach, bottom)
      context.lineTo(x - reach, bottom)
      context.closePath()
    }
    context.rect(x - half * 0.08, baseline - treeHeight * 0.2, half * 0.16, treeHeight * 0.2)
  }
  context.fill()
}

export const createScene: SceneModule['createScene'] = (context, environment) => {
  const next = random(6203)
  const stars: Star[] = createStars(next, 80)
  const flies: Firefly[] = Array.from({ length: 64 }, (_, index) => ({
    x: next(),
    y: index < 54 ? 0.5 + next() * 0.38 : 0.62 + next() * 0.3,
    rangeX: 0.02 + next() * 0.05,
    rangeY: 0.015 + next() * 0.035,
    speed: 0.08 + next() * 0.18,
    phase: next() * TAU,
    blink: 0.35 + next() * 0.9,
    size: index < 54 ? 0.6 + next() * 0.6 : 1.4 + next() * 0.9,
  }))

  let width = 1
  let height = 1
  let backLayer: Layer | undefined
  let frontLayer: Layer | undefined
  let fogSprite: SceneSurface | undefined
  let flySprite: SceneSurface | undefined

  function build(): void {
    backLayer = createLayer(environment, width, height)
    const back = backLayer.context
    const sky = back.createLinearGradient(0, 0, 0, height * 0.7)
    sky.addColorStop(0, '#050b19')
    sky.addColorStop(0.55, '#0c2032')
    sky.addColorStop(1, '#23504f')
    back.fillStyle = sky
    back.fillRect(0, 0, width, height)
    drawStars(back, stars, width, height * 0.45, 1.3, 0.7, Math.max(1, width / 1800))
    const moonX = width * 0.78
    const moonY = height * 0.17
    const moonRadius = Math.min(width, height) * 0.035
    const halo = Math.min(width, height) * 0.5
    back.globalAlpha = 0.35
    back.drawImage(glowSprite(environment, halo / 2, hex('#cfe8e0'), 0.04), moonX - halo / 2, moonY - halo / 2, halo, halo)
    back.globalAlpha = 1
    back.fillStyle = '#eef6ef'
    back.beginPath()
    back.arc(moonX, moonY, moonRadius, 0, TAU)
    back.fill()
    back.fillStyle = '#1b3f4a'
    pines(back, 13, width, height * 0.72, height * 0.2, height * 0.3, width * 0.035)
    const mist = back.createLinearGradient(0, height * 0.55, 0, height * 0.78)
    mist.addColorStop(0, 'rgb(150 200 190 / 0%)')
    mist.addColorStop(1, 'rgb(150 200 190 / 22%)')
    back.fillStyle = mist
    back.fillRect(0, height * 0.55, width, height * 0.23)
    // The meadow sits behind the middle pines so their trunks stand in grass, not sky.
    const meadow = back.createLinearGradient(0, height * 0.76, 0, height)
    meadow.addColorStop(0, '#163a38')
    meadow.addColorStop(0.3, '#0e2527')
    meadow.addColorStop(1, '#050d0f')
    back.fillStyle = meadow
    back.fillRect(0, height * 0.76, width, height * 0.24)
    back.fillStyle = '#0a1f24'
    pines(back, 29, width, height * 0.84, height * 0.28, height * 0.42, width * 0.05)

    frontLayer = createLayer(environment, width, height)
    const front = frontLayer.context
    front.fillStyle = '#040b0e'
    // Foreground pines frame a clearing so the fireflies and misty layers stay visible.
    pines(front, 47, width, height * 1.02, height * 0.5, height * 0.8, width * 0.07, (u) => u < 0.16 || u > 0.84)
    front.beginPath()
    front.moveTo(0, height)
    for (let x = 0; x <= width; x += width / 90) {
      front.lineTo(x, height * (0.93 + Math.sin(x * 0.07) * 0.01 + Math.sin(x * 0.013) * 0.015))
    }
    front.lineTo(width, height)
    front.closePath()
    front.fill()

    fogSprite = glowSprite(environment, width * 0.3, hex('#b5d8d0'), 0.02)
    flySprite = glowSprite(environment, 24, hex('#e9ff9a'), 0.12)
  }

  function drawFog(time: number): void {
    const fogWidth = width * 0.9
    const fogHeight = height * 0.16
    context.globalAlpha = 0.22
    for (let band = 0; band < 3; band++) {
      const speed = width * (0.006 + band * 0.003)
      const span = width + fogWidth
      const x = ((band * 0.37 * span + time * speed) % span) - fogWidth
      const y = height * (0.66 + band * 0.07) + Math.sin(time * 0.1 + band) * height * 0.01
      context.drawImage(fogSprite!, x, y - fogHeight / 2, fogWidth, fogHeight)
    }
    context.globalAlpha = 1
  }

  function drawFlies(time: number, from: number, to: number): void {
    const base = Math.min(width, height) / 900
    context.globalCompositeOperation = 'lighter'
    for (let index = from; index < to; index++) {
      const fly = flies[index]!
      const glow = Math.sin(time * fly.blink + fly.phase)
      const brightness = glow > 0 ? glow ** 2 : 0
      if (brightness < 0.02) continue
      const t = time * fly.speed
      const x = (fly.x + Math.sin(t + fly.phase) * fly.rangeX + Math.sin(t * 2.3 + fly.phase * 1.7) * fly.rangeX * 0.35) * width
      const y = (fly.y + Math.cos(t * 1.3 + fly.phase) * fly.rangeY) * height
      const size = 30 * fly.size * base * (0.7 + brightness * 0.3)
      context.globalAlpha = brightness
      context.drawImage(flySprite!, x - size, y - size, size * 2, size * 2)
    }
    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
      backLayer = undefined
    },
    render(frame) {
      if (!backLayer || !frontLayer) build()
      context.drawImage(backLayer!.surface, 0, 0)
      drawFog(frame.time)
      drawFlies(frame.time, 0, 54)
      context.drawImage(frontLayer!.surface, 0, 0)
      drawFlies(frame.time, 54, flies.length)
    },
  }
}
