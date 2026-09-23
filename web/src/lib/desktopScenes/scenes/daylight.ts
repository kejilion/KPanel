import type { SceneModule } from '../types'
import {
  celestialAt, createLayer, createStars, drawStars, fillRidge, hex, mix, paintSky, random, rgba, ridge,
  skyAt, type Layer, type RGB, type SkyColors, type Star,
} from './shared'

// A mountain valley whose sky, sun, moon and haze follow the viewer's local clock.

interface Cloud {
  x: number
  y: number
  scale: number
  speed: number
  puffs: Array<{ x: number, y: number, r: number }>
}

const RIDGES: ReadonlyArray<{ day: RGB, night: RGB, haze: number, baseline: number, amplitude: number, roughness: number, seed: number }> = [
  { day: hex('#8ea9cb'), night: hex('#1e2849'), haze: 0.34, baseline: 0.64, amplitude: 0.17, roughness: 0.56, seed: 11 },
  { day: hex('#56769c'), night: hex('#151d3a'), haze: 0.16, baseline: 0.76, amplitude: 0.16, roughness: 0.52, seed: 23 },
  { day: hex('#2a4463'), night: hex('#0b1127'), haze: 0.05, baseline: 0.9, amplitude: 0.15, roughness: 0.5, seed: 37 },
]

export const createScene: SceneModule['createScene'] = (context, environment) => {
  const next = random(20260923)
  const stars: Star[] = createStars(next, 170)
  const ridges = RIDGES.map((layer) => ridge(random(layer.seed), 129, layer.roughness))
  const clouds: Cloud[] = Array.from({ length: 7 }, (_, index) => ({
    x: next(),
    y: 0.08 + next() * 0.3,
    scale: 0.55 + next() * 0.6,
    speed: 0.0035 + next() * 0.004 + index * 0.0004,
    puffs: Array.from({ length: 16 + Math.floor(next() * 9) }, () => {
      const x = 0.5 + (next() + next() - 1) * 0.42
      const crown = 1 - Math.abs(x - 0.5) * 1.7
      return { x, y: 0.62 - crown * 0.3 * next(), r: (0.3 + crown * 0.55) * (0.7 + next() * 0.3) }
    }),
  }))

  let width = 1
  let height = 1
  let skyLayer: Layer | undefined
  let landLayer: Layer | undefined
  let cloudLayers: Layer[] = []
  let cachedMinute = -1
  let sky: SkyColors = skyAt(12)

  function paintCelestial(target: Layer, horizon: number): void {
    const body = celestialAt(cachedMinute / 60)
    if (body.altitude < -0.08) return
    // Measured from the far ridge line so a low sun still rises between the peaks.
    const x = body.x * width
    const y = horizon - body.altitude * height * 0.5
    const radius = Math.min(width, height) * (body.kind === 'sun' ? 0.034 : 0.026)
    const glowRadius = Math.min(width, height) * (body.kind === 'sun' ? 0.42 : 0.24)
    const glow = target.context.createRadialGradient(x, y, 0, x, y, glowRadius)
    const strength = body.kind === 'sun' ? 0.5 + (1 - body.altitude) * 0.25 : 0.22
    glow.addColorStop(0, rgba(sky.sun, strength))
    glow.addColorStop(0.25, rgba(sky.sun, strength * 0.35))
    glow.addColorStop(1, rgba(sky.sun, 0))
    target.context.fillStyle = glow
    target.context.fillRect(0, 0, width, horizon + height * 0.1)
    target.context.fillStyle = rgba(body.kind === 'sun' ? mix(sky.sun, [255, 255, 255], 0.55) : hex('#eef2ff'))
    target.context.beginPath()
    target.context.arc(x, y, radius, 0, Math.PI * 2)
    target.context.fill()
    if (body.kind === 'moon') {
      target.context.fillStyle = rgba(mix(sky.top, sky.middle, 0.35), 0.88)
      target.context.beginPath()
      target.context.arc(x + radius * 0.42, y - radius * 0.18, radius * 0.92, 0, Math.PI * 2)
      target.context.fill()
    }
  }

  function repaint(minute: number): void {
    cachedMinute = minute
    sky = skyAt(minute / 60)
    const horizon = height * 0.8
    skyLayer ??= createLayer(environment, width, height)
    paintSky(skyLayer.context, sky, width, horizon)
    skyLayer.context.fillStyle = rgba(sky.horizon)
    skyLayer.context.fillRect(0, horizon, width, height - horizon)
    paintCelestial(skyLayer, height * 0.64)

    landLayer ??= createLayer(environment, width, height)
    landLayer.context.clearRect(0, 0, width, height)
    RIDGES.forEach((layer, index) => {
      const color = mix(mix(layer.night, layer.day, sky.light), sky.horizon, layer.haze)
      landLayer!.context.fillStyle = rgba(color)
      fillRidge(landLayer!.context, ridges[index]!, width, height * layer.baseline, height * layer.amplitude, height)
      // Valley mist settles at each ridge's foot and fades out on both sides.
      const hazeTop = height * layer.baseline
      const haze = landLayer!.context.createLinearGradient(0, hazeTop - height * 0.04, 0, hazeTop + height * 0.1)
      haze.addColorStop(0, rgba(sky.horizon, 0))
      haze.addColorStop(0.45, rgba(sky.horizon, 0.3 - index * 0.09))
      haze.addColorStop(1, rgba(sky.horizon, 0))
      landLayer!.context.fillStyle = haze
      landLayer!.context.fillRect(0, hazeTop - height * 0.04, width, height * 0.14)
    })

    // Cumulus sprites: soft puffs piled into a crown, a flattened base and a
    // shaded underside, re-tinted once a minute as the light changes.
    const cloudColor = mix(mix(hex('#2a3358'), [255, 255, 255], sky.light), sky.sun, 0.22)
    const cloudShade = mix(cloudColor, sky.middle, 0.45)
    cloudLayers = clouds.map((cloud) => {
      const cloudWidth = width * 0.28 * cloud.scale
      const cloudHeight = cloudWidth * 0.42
      const layer = createLayer(environment, cloudWidth, cloudHeight)
      const sprite = layer.context
      for (const puff of cloud.puffs) {
        const x = cloudWidth * puff.x
        const y = cloudHeight * puff.y
        const radius = cloudHeight * 0.36 * puff.r
        const gradient = sprite.createRadialGradient(x, y, 0, x, y, radius)
        gradient.addColorStop(0, rgba(cloudColor, 0.62))
        gradient.addColorStop(0.55, rgba(cloudColor, 0.36))
        gradient.addColorStop(1, rgba(cloudColor, 0))
        sprite.fillStyle = gradient
        sprite.fillRect(x - radius, y - radius, radius * 2, radius * 2)
      }
      sprite.globalCompositeOperation = 'source-atop'
      const underside = sprite.createLinearGradient(0, cloudHeight * 0.3, 0, cloudHeight * 0.8)
      underside.addColorStop(0, rgba(cloudShade, 0))
      underside.addColorStop(1, rgba(cloudShade, 0.7))
      sprite.fillStyle = underside
      sprite.fillRect(0, 0, cloudWidth, cloudHeight)
      sprite.globalCompositeOperation = 'destination-out'
      const base = sprite.createLinearGradient(0, cloudHeight * 0.68, 0, cloudHeight * 0.84)
      base.addColorStop(0, 'rgb(0 0 0 / 0%)')
      base.addColorStop(1, 'rgb(0 0 0 / 100%)')
      sprite.fillStyle = base
      sprite.fillRect(0, cloudHeight * 0.68, cloudWidth, cloudHeight * 0.32)
      sprite.globalCompositeOperation = 'source-over'
      return layer
    })
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
      skyLayer = undefined
      landLayer = undefined
      cachedMinute = -1
    },
    render(frame) {
      const minute = Math.floor(frame.hour * 60)
      if (minute !== cachedMinute || !skyLayer || !landLayer) repaint(minute)
      context.drawImage(skyLayer!.surface, 0, 0)
      drawStars(context, stars, width, height * 0.7, frame.time, sky.stars, Math.max(1, width / 1600))
      const cloudAlpha = 0.28 + sky.light * 0.67
      context.globalAlpha = cloudAlpha
      clouds.forEach((cloud, index) => {
        const layer = cloudLayers[index]
        if (!layer) return
        const span = width + layer.surface.width
        const x = ((cloud.x * span + frame.time * cloud.speed * width) % span) - layer.surface.width
        context.drawImage(layer.surface, x, cloud.y * height)
      })
      context.globalAlpha = 1
      context.drawImage(landLayer!.surface, 0, 0)
    },
  }
}
