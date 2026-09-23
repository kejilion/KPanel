import type { SceneContext2D, SceneModule } from '../types'
import {
  celestialAt, createLayer, createStars, drawStars, fillRidge, hex, mix, paintSky, random, rgba, ridge,
  skyAt, type Layer, type SkyColors, type Star,
} from './shared'

// A quiet beach: layered swells, a glittering sun or moon path and a breathing shoreline.

const HORIZON = 0.52
const SHORE = 0.82

const SEA_FAR_DAY = hex('#86b9d9')
const SEA_NEAR_DAY = hex('#177f98')
const SEA_FAR_NIGHT = hex('#1c2b50')
const SEA_NEAR_NIGHT = hex('#08142c')
const SAND_DAY = hex('#ead3a6')
const SAND_NIGHT = hex('#2b2a3d')

interface Swell {
  depth: number
  amplitude: number
  frequency: number
  speed: number
  phase: number
}

interface Bird {
  x: number
  y: number
  speed: number
  size: number
  flap: number
}

export const createScene: SceneModule['createScene'] = (context, environment) => {
  const next = random(8812)
  const stars: Star[] = createStars(next, 120)
  const island = ridge(random(91), 65, 0.62)
  const swells: Swell[] = Array.from({ length: 5 }, (_, index) => ({
    depth: 0.06 + (index / 4) ** 1.35 * 0.94,
    amplitude: 0.0035 + index * 0.0028,
    frequency: 7 - index * 1.1,
    speed: 0.18 + index * 0.09,
    phase: next() * 10,
  }))
  const birds: Bird[] = Array.from({ length: 4 }, () => ({
    x: next(),
    y: 0.16 + next() * 0.2,
    speed: 0.006 + next() * 0.006,
    size: 0.7 + next() * 0.5,
    flap: next() * 10,
  }))

  let width = 1
  let height = 1
  let unit = 1
  let skyLayer: Layer | undefined
  let sandLayer: Layer | undefined
  let lightPath: Layer | undefined
  let cachedMinute = -1
  let sky: SkyColors = skyAt(12)
  let bodyX = 0.5
  let glitter = 0
  let glitterColor = hex('#ffffff')
  let seaFar = SEA_FAR_DAY
  let seaNear = SEA_NEAR_DAY

  function wave(x: number, swell: Swell, time: number): number {
    const u = x / width
    return Math.sin(u * swell.frequency * 6.283 + time * swell.speed + swell.phase) * 0.62
      + Math.sin(u * swell.frequency * 13.7 - time * swell.speed * 1.7 + swell.phase * 2) * 0.38
  }

  function repaint(minute: number): void {
    cachedMinute = minute
    sky = skyAt(minute / 60)
    const horizon = height * HORIZON
    const shore = height * SHORE
    skyLayer ??= createLayer(environment, width, height)
    const sea = skyLayer.context
    paintSky(sea, sky, width, horizon)

    const body = celestialAt(minute / 60)
    bodyX = body.x
    glitter = body.altitude > -0.05 ? (body.kind === 'sun' ? 0.55 + (1 - body.altitude) * 0.45 : 0.4) : 0
    glitterColor = body.kind === 'sun' ? mix(sky.sun, [255, 255, 255], 0.35) : hex('#dfe6ff')
    if (body.altitude > -0.08) {
      const x = body.x * width
      const y = horizon - body.altitude * height * 0.42
      const glowRadius = Math.min(width, height) * (body.kind === 'sun' ? 0.4 : 0.22)
      const glow = sea.createRadialGradient(x, y, 0, x, y, glowRadius)
      glow.addColorStop(0, rgba(sky.sun, body.kind === 'sun' ? 0.7 : 0.25))
      glow.addColorStop(0.3, rgba(sky.sun, body.kind === 'sun' ? 0.22 : 0.08))
      glow.addColorStop(1, rgba(sky.sun, 0))
      sea.fillStyle = glow
      sea.fillRect(0, 0, width, horizon)
      sea.fillStyle = rgba(body.kind === 'sun' ? mix(sky.sun, [255, 255, 255], 0.6) : hex('#f0f3ff'))
      sea.beginPath()
      sea.arc(x, y, Math.min(width, height) * (body.kind === 'sun' ? 0.032 : 0.024), 0, Math.PI * 2)
      sea.fill()
    }

    sea.fillStyle = rgba(mix(mix(hex('#101a36'), hex('#5d7593'), sky.light), sky.horizon, 0.35))
    sea.save()
    sea.beginPath()
    sea.rect(0, 0, width, horizon)
    sea.clip()
    fillRidge(sea, island, width * 0.34, horizon + 1, height * 0.035, horizon + 1)
    sea.restore()

    seaFar = mix(mix(SEA_FAR_NIGHT, SEA_FAR_DAY, sky.light), sky.middle, 0.35 - sky.light * 0.15)
    seaNear = mix(mix(SEA_NEAR_NIGHT, SEA_NEAR_DAY, sky.light), sky.top, 0.18)

    // The light path widens towards the viewer; built per minute, blended per frame.
    lightPath = undefined
    if (glitter > 0) {
      const pathHeight = Math.max(1, Math.round(shore - horizon))
      const pathWidth = Math.max(2, Math.round(width * 0.24))
      lightPath = createLayer(environment, pathWidth, pathHeight)
      for (let row = 0; row < pathHeight; row += 3) {
        const depth = row / pathHeight
        const half = pathWidth * (0.06 + depth * 0.44)
        const strip = lightPath.context.createLinearGradient(pathWidth / 2 - half, 0, pathWidth / 2 + half, 0)
        const alpha = glitter * 0.34 * (1 - depth * 0.55)
        strip.addColorStop(0, rgba(glitterColor, 0))
        strip.addColorStop(0.5, rgba(glitterColor, alpha))
        strip.addColorStop(1, rgba(glitterColor, 0))
        lightPath.context.fillStyle = strip
        lightPath.context.fillRect(pathWidth / 2 - half, row, half * 2, 3)
      }
    }

    const water = sea.createLinearGradient(0, horizon, 0, shore)
    water.addColorStop(0, rgba(seaFar))
    water.addColorStop(1, rgba(seaNear))
    sea.fillStyle = water
    sea.fillRect(0, horizon, width, height - horizon)

    sandLayer ??= createLayer(environment, width, height)
    const sand = sandLayer.context
    sand.clearRect(0, 0, width, height)
    const sandColor = mix(mix(SAND_NIGHT, SAND_DAY, sky.light), sky.sun, 0.12)
    const beach = sand.createLinearGradient(0, shore, 0, height)
    beach.addColorStop(0, rgba(mix(sandColor, seaNear, 0.35)))
    beach.addColorStop(0.3, rgba(sandColor))
    beach.addColorStop(1, rgba(mix(sandColor, hex('#000000'), 0.12)))
    sand.fillStyle = beach
    sand.beginPath()
    sand.moveTo(0, shore + height * 0.012)
    for (let x = 0; x <= width; x += width / 48) {
      sand.lineTo(x, shore + height * (0.012 - 0.018 * (x / width)) + Math.sin(x / width * 5.1) * height * 0.006)
    }
    sand.lineTo(width, height)
    sand.lineTo(0, height)
    sand.closePath()
    sand.fill()
  }

  function shoreline(x: number): number {
    return height * SHORE + height * (0.012 - 0.018 * (x / width)) + Math.sin(x / width * 5.1) * height * 0.006
  }

  function drawSwells(time: number, step: number): void {
    const horizon = height * HORIZON
    const shore = height * SHORE
    const reflection = mix(sky.middle, sky.horizon, 0.45)
    swells.forEach((swell, index) => {
      const base = horizon + (shore - horizon) * swell.depth
      const amplitude = height * swell.amplitude
      // Far swells mirror the sky, strongest at dawn and dusk when the horizon glows.
      const tone = mix(seaFar, seaNear, swell.depth)
      context.fillStyle = rgba(mix(tone, reflection, (1 - swell.depth) * (0.6 - sky.light * 0.4)))
      // Each band only reaches under the next one, which keeps overdraw near one screen.
      const following = swells[index + 1]
      const bottom = following
        ? horizon + (shore - horizon) * following.depth + height * following.amplitude * 1.5
        : shore + height * 0.04
      context.beginPath()
      context.moveTo(0, bottom)
      for (let x = 0; x <= width + step; x += step) context.lineTo(x, base + wave(x, swell, time) * amplitude)
      context.lineTo(width + step, bottom)
      context.closePath()
      context.fill()
      context.strokeStyle = rgba([255, 255, 255], 0.05 + index * 0.025 + sky.light * 0.05)
      context.lineWidth = Math.max(1, unit * (0.8 + index * 0.35))
      context.beginPath()
      for (let x = 0; x <= width + step; x += step) {
        const y = base + wave(x, swell, time) * amplitude
        if (x === 0) context.moveTo(x, y)
        else context.lineTo(x, y)
      }
      context.stroke()
    })
  }

  function drawGlitter(context2d: SceneContext2D, time: number): void {
    if (glitter <= 0) return
    const horizon = height * HORIZON
    const shore = height * SHORE
    context2d.globalCompositeOperation = 'lighter'
    if (lightPath) context2d.drawImage(lightPath.surface, bodyX * width - lightPath.surface.width / 2, horizon)
    context2d.fillStyle = rgba(glitterColor)
    for (let index = 0; index < 130; index++) {
      const depth = (index / 130) ** 1.5
      const y = horizon + 2 + depth * (shore - horizon - 4)
      const spread = width * (0.008 + depth * 0.075)
      const x = bodyX * width + Math.sin(time * (0.9 + (index % 7) * 0.13) + index * 12.9898) * spread
      const shimmer = Math.abs(Math.sin(time * 1.9 + index * 3.17))
      context2d.globalAlpha = glitter * shimmer * (0.85 - depth * 0.5)
      context2d.fillRect(x, y, unit * (3 + depth * 34) * (0.4 + shimmer * 0.6), Math.max(1, unit * (1 + depth * 1.5)))
    }
    context2d.globalAlpha = 1
    context2d.globalCompositeOperation = 'source-over'
  }

  function drawWash(time: number, step: number): void {
    const reach = (Math.sin(time * 0.42) * 0.5 + 0.5) * height * 0.03
    context.beginPath()
    context.moveTo(0, height * SHORE - height * 0.02)
    const edge: number[] = []
    for (let x = 0; x <= width + step; x += step) {
      const y = shoreline(x) + reach + Math.sin(x / width * 23 + time * 0.8) * unit * 2.4
      edge.push(y)
      context.lineTo(x, y)
    }
    context.lineTo(width, height * SHORE - height * 0.02)
    context.closePath()
    context.fillStyle = rgba(mix(seaNear, [255, 255, 255], 0.28), 0.55)
    context.fill()
    context.strokeStyle = rgba([255, 255, 255], 0.35 + sky.light * 0.35)
    context.lineWidth = Math.max(1, unit * 2.2)
    context.beginPath()
    edge.forEach((y, index) => (index === 0 ? context.moveTo(0, y) : context.lineTo(index * step, y)))
    context.stroke()
  }

  function drawBirds(time: number): void {
    if (sky.light < 0.3) return
    context.strokeStyle = rgba(mix(sky.top, hex('#0b1020'), 0.6), 0.75)
    context.lineWidth = Math.max(1, unit * 1.6)
    context.beginPath()
    for (const bird of birds) {
      const span = width * 1.2
      const x = ((bird.x * span + time * bird.speed * width) % span) - width * 0.1
      const y = bird.y * height + Math.sin(time * 0.4 + bird.flap) * height * 0.01
      const size = unit * 11 * bird.size
      const lift = Math.sin(time * 5 + bird.flap) * 0.5
      context.moveTo(x - size, y - size * (0.25 + lift * 0.5))
      context.quadraticCurveTo(x - size * 0.45, y - size * (0.45 + lift), x, y)
      context.quadraticCurveTo(x + size * 0.45, y - size * (0.45 + lift), x + size, y - size * (0.25 + lift * 0.5))
    }
    context.stroke()
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
      unit = Math.min(width, height) / 900
      skyLayer = undefined
      sandLayer = undefined
      cachedMinute = -1
    },
    render(frame) {
      const minute = Math.floor(frame.hour * 60)
      if (minute !== cachedMinute || !skyLayer || !sandLayer) repaint(minute)
      const step = Math.max(8, width / 150)
      context.drawImage(skyLayer!.surface, 0, 0)
      drawStars(context, stars, width, height * HORIZON * 0.9, frame.time, sky.stars, Math.max(1, width / 1600))
      drawSwells(frame.time, step)
      drawGlitter(context, frame.time)
      context.drawImage(sandLayer!.surface, 0, 0)
      drawWash(frame.time, step)
      drawBirds(frame.time)
    },
  }
}
