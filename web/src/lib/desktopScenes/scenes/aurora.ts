import type { SceneModule, SceneSurface } from '../types'
import { createLayer, createStars, drawStars, fillRidge, hex, random, rgba, ridge, type Layer, type RGB, type Star } from './shared'

// Northern lights over a still alpine lake. Curtains render into a quarter-size
// buffer and are upscaled, which both softens them and keeps the fill cost low.

const LAKE = 0.76

interface Ribbon {
  base: number
  height: number
  alpha: number
  phase: number
  sway: number
  drift: number
  palette: readonly [RGB, RGB, RGB]
}

const RIBBONS: readonly Ribbon[] = [
  { base: 0.74, height: 0.5, alpha: 0.95, phase: 0.4, sway: 0.11, drift: 0.05, palette: [hex('#b6ffcf'), hex('#38e09a'), hex('#2a9fd0')] },
  { base: 0.6, height: 0.42, alpha: 0.6, phase: 2.1, sway: 0.08, drift: -0.04, palette: [hex('#a8fff0'), hex('#2fc7c0'), hex('#6a5cff')] },
  { base: 0.48, height: 0.34, alpha: 0.4, phase: 4.3, sway: 0.06, drift: 0.03, palette: [hex('#ffc6f3'), hex('#b467ff'), hex('#4a3bd8')] },
]

export const createScene: SceneModule['createScene'] = (context, environment) => {
  const next = random(4471)
  const staticStars: Star[] = createStars(next, 260)
  const twinkling: Star[] = createStars(next, 70)
  const farRidge = ridge(random(5), 129, 0.6)
  const nearRidge = ridge(random(17), 129, 0.52)
  const curtains: SceneSurface[] = RIBBONS.map((ribbon) => {
    const { surface, context: sprite } = createLayer(environment, 1, 96)
    const gradient = sprite.createLinearGradient(0, 96, 0, 0)
    gradient.addColorStop(0, rgba(ribbon.palette[0], 0))
    gradient.addColorStop(0.05, rgba(ribbon.palette[0], 0.95))
    gradient.addColorStop(0.3, rgba(ribbon.palette[1], 0.55))
    gradient.addColorStop(0.7, rgba(ribbon.palette[2], 0.18))
    gradient.addColorStop(1, rgba(ribbon.palette[2], 0))
    sprite.fillStyle = gradient
    sprite.fillRect(0, 0, 1, 96)
    return surface
  })

  let width = 1
  let height = 1
  let backLayer: Layer | undefined
  let frontLayer: Layer | undefined
  let auroraLayer: Layer | undefined
  let nextMeteor = 6
  let meteor: { start: number, x: number, y: number, angle: number, length: number } | undefined

  function build(): void {
    const lake = height * LAKE
    backLayer = createLayer(environment, width, height)
    const back = backLayer.context
    const sky = back.createLinearGradient(0, 0, 0, lake)
    sky.addColorStop(0, '#02050f')
    sky.addColorStop(0.55, '#07142a')
    sky.addColorStop(1, '#0d2a3c')
    back.fillStyle = sky
    back.fillRect(0, 0, width, lake)
    drawStars(back, staticStars, width, lake * 0.85, 0, 0.75, Math.max(1, width / 1800))
    const water = back.createLinearGradient(0, lake, 0, height)
    water.addColorStop(0, '#0a2233')
    water.addColorStop(1, '#030811')
    back.fillStyle = water
    back.fillRect(0, lake, width, height - lake)

    frontLayer = createLayer(environment, width, height)
    const front = frontLayer.context
    const layers = [
      { heights: farRidge, baseline: lake - height * 0.04, amplitude: height * 0.3, color: '#101d31', rim: 0.3, snow: 0.5 },
      { heights: nearRidge, baseline: lake + 1, amplitude: height * 0.14, color: '#060c18', rim: 0.12, snow: 0 },
    ]
    for (const layer of layers) {
      front.fillStyle = layer.color
      fillRidge(front, layer.heights, width, layer.baseline, layer.amplitude, lake + 1)
      if (layer.snow > 0) {
        const peak = layer.baseline - layer.amplitude
        const snow = front.createLinearGradient(0, peak, 0, peak + layer.amplitude * 0.6)
        snow.addColorStop(0, `rgb(206 224 246 / ${layer.snow * 100}%)`)
        snow.addColorStop(0.45, `rgb(150 180 214 / ${layer.snow * 40}%)`)
        snow.addColorStop(1, 'rgb(150 180 214 / 0%)')
        front.fillStyle = snow
        fillRidge(front, layer.heights, width, layer.baseline, layer.amplitude, lake + 1)
        front.fillStyle = layer.color
      }
      front.save()
      front.translate(0, lake * 2)
      front.scale(1, -1)
      front.globalAlpha = 0.72
      fillRidge(front, layer.heights, width, layer.baseline, layer.amplitude, lake)
      front.restore()
      front.strokeStyle = `rgb(190 230 255 / ${layer.rim * 100}%)`
      front.lineWidth = Math.max(1, height / 700)
      front.beginPath()
      const step = width / (layer.heights.length - 1)
      layer.heights.forEach((value, index) => {
        const y = layer.baseline - value * layer.amplitude
        if (index === 0) front.moveTo(0, y)
        else front.lineTo(index * step, y)
      })
      front.stroke()
    }
    front.fillStyle = 'rgb(160 220 255 / 6%)'
    for (let line = 0; line < 14; line++) {
      const y = lake + (height - lake) * ((line + 1) / 15) ** 1.4
      front.fillRect(width * (0.1 + ((line * 37) % 50) / 100), y, width * (0.18 + (line % 4) * 0.06), Math.max(1, height / 900))
    }

    auroraLayer = createLayer(environment, Math.max(64, Math.round(width / 4)), Math.max(48, Math.round(lake / 4)))
  }

  function drawAurora(time: number): void {
    const buffer = auroraLayer!
    const bufferWidth = buffer.surface.width
    const bufferHeight = buffer.surface.height
    buffer.context.clearRect(0, 0, bufferWidth, bufferHeight)
    buffer.context.globalCompositeOperation = 'lighter'
    const columnWidth = 2
    const pulse = 0.82 + Math.sin(time * 0.21) * 0.18
    RIBBONS.forEach((ribbon, index) => {
      const sprite = curtains[index]!
      for (let x = 0; x < bufferWidth; x += columnWidth) {
        const u = x / bufferWidth
        const y = bufferHeight * (ribbon.base
          + Math.sin(u * 5.2 + time * ribbon.sway + ribbon.phase) * 0.09
          + Math.sin(u * 11.3 - time * ribbon.sway * 1.6 + ribbon.phase * 1.7) * 0.035
          + Math.sin(u * 2.1 + time * ribbon.drift) * 0.05)
        const fold = Math.sin(u * 17 + time * 0.35 + ribbon.phase) * Math.sin(u * 7.3 - time * 0.22 + ribbon.phase)
        const rays = 0.72 + 0.28 * Math.sin(u * 96 + time * 0.6 + ribbon.phase) * Math.sin(u * 37 - time * 0.4)
        const intensity = Math.max(0, 0.35 + fold * 0.65) * (0.35 + 0.65 * Math.sin(u * Math.PI) ** 0.6) * rays
        if (intensity < 0.03) continue
        const curtain = bufferHeight * ribbon.height * (0.65 + 0.35 * Math.sin(u * 9.1 + time * 0.28 + ribbon.phase))
        buffer.context.globalAlpha = intensity * ribbon.alpha * pulse
        buffer.context.drawImage(sprite, x, y - curtain, columnWidth + 0.5, curtain)
      }
    })
    buffer.context.globalAlpha = 1
    buffer.context.globalCompositeOperation = 'source-over'

    const lake = height * LAKE
    context.globalCompositeOperation = 'lighter'
    context.drawImage(buffer.surface, 0, 0, width, lake)
    context.save()
    context.beginPath()
    context.rect(0, lake, width, height - lake)
    context.clip()
    context.translate(0, lake * 2)
    context.scale(1, -1)
    context.globalAlpha = 0.3
    context.drawImage(buffer.surface, 0, 0, width, lake)
    context.restore()
    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
  }

  function drawMeteor(time: number): void {
    if (!meteor && time >= nextMeteor) {
      meteor = { start: time, x: 0.15 + next() * 0.7, y: 0.05 + next() * 0.2, angle: 0.35 + next() * 0.35, length: 0.12 + next() * 0.1 }
      nextMeteor = time + 9 + next() * 9
    }
    if (!meteor) return
    const progress = (time - meteor.start) / 0.9
    if (progress >= 1) {
      meteor = undefined
      return
    }
    const headX = (meteor.x + Math.cos(meteor.angle) * meteor.length * progress) * width
    const headY = (meteor.y + Math.sin(meteor.angle) * meteor.length * progress) * height
    const tail = meteor.length * width * 0.35
    const tailX = headX - Math.cos(meteor.angle) * tail
    const tailY = headY - Math.sin(meteor.angle) * tail
    const trail = context.createLinearGradient(tailX, tailY, headX, headY)
    const fade = Math.sin(progress * Math.PI)
    trail.addColorStop(0, 'rgb(255 255 255 / 0%)')
    trail.addColorStop(1, `rgb(235 245 255 / ${Math.round(fade * 90)}%)`)
    context.strokeStyle = trail
    context.lineWidth = Math.max(1, height / 600)
    context.lineCap = 'round'
    context.beginPath()
    context.moveTo(tailX, tailY)
    context.lineTo(headX, headY)
    context.stroke()
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
      backLayer = undefined
    },
    render(frame) {
      if (!backLayer || !frontLayer || !auroraLayer) build()
      context.drawImage(backLayer!.surface, 0, 0)
      drawStars(context, twinkling, width, height * LAKE * 0.8, frame.time, 0.9, Math.max(1, width / 1600))
      drawMeteor(frame.time)
      drawAurora(frame.time)
      context.drawImage(frontLayer!.surface, 0, 0)
    },
  }
}
