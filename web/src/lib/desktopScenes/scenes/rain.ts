import type { SceneModule, SceneSurface } from '../types'
import { createLayer, glowSprite, hex, random, rgba, type Layer, type RGB } from './shared'

// A rainy neon city seen through out-of-focus glass: skyline, bokeh, rain and puddle ripples.

const STREET = 0.86
const NEON: readonly RGB[] = [hex('#ff5fa2'), hex('#4fd8ff'), hex('#ffb347'), hex('#9b7bff')]
const PANE_LIGHTS: readonly RGB[] = [hex('#ffd58a'), hex('#ffc46b'), hex('#8fdcff'), hex('#ff8ccf')]

interface Pane {
  x: number
  y: number
  w: number
  h: number
  color: RGB
  lit: boolean
}

interface Drop {
  x: number
  y: number
  speed: number
  length: number
  near: boolean
}

interface Bokeh {
  x: number
  y: number
  radius: number
  sprite: number
  phase: number
  drift: number
}

interface Ripple {
  x: number
  y: number
  start: number
}

export const createScene: SceneModule['createScene'] = (context, environment) => {
  const next = random(3303)
  const bokeh: Bokeh[] = Array.from({ length: 26 }, () => ({
    x: next(),
    y: 0.1 + next() * 0.8,
    radius: 0.025 + next() ** 2 * 0.07,
    sprite: Math.floor(next() * NEON.length),
    phase: next() * 10,
    drift: 0.2 + next() * 0.6,
  }))

  let width = 1
  let height = 1
  let unit = 1
  let cityLayer: Layer | undefined
  let panes: Pane[] = []
  let drops: Drop[] = []
  let ripples: Ripple[] = []
  let bokehSprites: SceneSurface[] = []
  let flickerAt = 0
  let rippleAt = 0

  function buildingRow(target: Layer, seed: number, baseline: number, minHeight: number, maxHeight: number, color: string, windowAlpha: number, record: boolean): void {
    const rows = random(seed)
    let x = -rows() * width * 0.03
    while (x < width) {
      const buildingWidth = width * (0.035 + rows() * 0.055)
      const buildingHeight = height * (minHeight + rows() * (maxHeight - minHeight))
      const top = baseline - buildingHeight
      target.context.fillStyle = color
      target.context.fillRect(x, top, buildingWidth + 1, buildingHeight)
      if (rows() < 0.25) target.context.fillRect(x + buildingWidth * 0.45, top - height * 0.03, Math.max(1, unit * 2), height * 0.03)
      const cell = Math.max(4, unit * 9)
      for (let row = top + cell; row < baseline - cell; row += cell * 1.6) {
        for (let column = x + cell * 0.6; column < x + buildingWidth - cell; column += cell * 1.3) {
          if (rows() > 0.32) continue
          const light = PANE_LIGHTS[Math.floor(rows() * PANE_LIGHTS.length)]!
          const lit = rows() > 0.35
          const pane = { x: column, y: row, w: cell * 0.7, h: cell * 0.8, color: light, lit }
          target.context.fillStyle = lit ? rgba(light, windowAlpha) : color
          target.context.fillRect(pane.x, pane.y, pane.w, pane.h)
          if (record) panes.push(pane)
        }
      }
      x += buildingWidth + width * rows() * 0.012
    }
  }

  function build(): void {
    cityLayer = createLayer(environment, width, height)
    panes = []
    const city = cityLayer.context
    const street = height * STREET
    const sky = city.createLinearGradient(0, 0, 0, street)
    sky.addColorStop(0, '#05050f')
    sky.addColorStop(0.55, '#150d2e')
    sky.addColorStop(1, '#3d1a47')
    city.fillStyle = sky
    city.fillRect(0, 0, width, street)
    const haze = [
      { x: 0.22, color: NEON[0]!, radius: 0.5 },
      { x: 0.7, color: NEON[1]!, radius: 0.45 },
      { x: 0.48, color: NEON[3]!, radius: 0.35 },
    ]
    city.globalCompositeOperation = 'lighter'
    for (const glow of haze) {
      city.globalAlpha = 0.28
      const size = width * glow.radius
      city.drawImage(glowSprite(environment, size / 2, glow.color, 0.05), glow.x * width - size / 2, street - size * 0.62, size, size * 0.9)
    }
    city.globalAlpha = 1
    city.globalCompositeOperation = 'source-over'
    buildingRow(cityLayer, 71, street - height * 0.08, 0.16, 0.42, '#1b1533', 0.25, false)
    buildingRow(cityLayer, 29, street, 0.14, 0.5, '#0b0916', 0.72, true)

    const ground = city.createLinearGradient(0, street, 0, height)
    ground.addColorStop(0, '#1a1024')
    ground.addColorStop(1, '#07060c')
    city.fillStyle = ground
    city.fillRect(0, street, width, height - street)
    city.globalCompositeOperation = 'lighter'
    // Wet asphalt: soft elongated glows below the neon haze instead of hard-edged bands.
    for (const glow of haze) {
      const reflection = glowSprite(environment, 48, glow.color, 0.05)
      const reflectionWidth = width * glow.radius * 0.45
      city.globalAlpha = 0.45
      city.drawImage(reflection, glow.x * width - reflectionWidth / 2, street - (height - street) * 0.4, reflectionWidth, (height - street) * 2.2)
    }
    city.globalAlpha = 1
    const streaks = random(8)
    for (let index = 0; index < 40; index++) {
      const pane = panes[Math.floor(streaks() * panes.length)]
      if (!pane) break
      city.fillStyle = rgba(pane.color, 0.07)
      city.fillRect(pane.x, street + 2, pane.w, (height - street) * (0.3 + streaks() * 0.6))
    }
    city.globalCompositeOperation = 'source-over'

    bokehSprites = NEON.map((color) => {
      const size = 128
      const { surface, context: sprite } = createLayer(environment, size, size)
      const gradient = sprite.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2)
      gradient.addColorStop(0, rgba(color, 0.3))
      gradient.addColorStop(0.72, rgba(color, 0.38))
      gradient.addColorStop(0.9, rgba(color, 0.55))
      gradient.addColorStop(1, rgba(color, 0))
      sprite.fillStyle = gradient
      sprite.fillRect(0, 0, size, size)
      return surface
    })

    const count = Math.round(Math.min(320, Math.max(120, (width * height) / 9000)))
    const rain = random(55)
    drops = Array.from({ length: count }, () => ({
      x: rain() * width * 1.2,
      y: rain() * height,
      speed: height * (0.9 + rain() * 0.7),
      length: height * (0.018 + rain() * 0.03),
      near: rain() < 0.3,
    }))
    ripples = []
  }

  function flicker(time: number): void {
    if (time < flickerAt || panes.length === 0) return
    flickerAt = time + 0.35 + next() * 0.8
    const pane = panes[Math.floor(next() * panes.length)]!
    pane.lit = !pane.lit
    cityLayer!.context.fillStyle = pane.lit ? rgba(pane.color, 0.85) : '#0b0916'
    cityLayer!.context.fillRect(pane.x, pane.y, pane.w, pane.h)
  }

  function drawRain(delta: number): void {
    const wind = 0.16
    for (const near of [false, true]) {
      context.strokeStyle = near ? 'rgb(214 226 255 / 42%)' : 'rgb(190 205 245 / 20%)'
      context.lineWidth = Math.max(1, unit * (near ? 1.6 : 0.9))
      context.beginPath()
      for (const drop of drops) {
        if (drop.near !== near) continue
        const speed = drop.speed * (near ? 1.35 : 1)
        drop.y += speed * delta
        drop.x -= speed * wind * delta
        if (drop.y - drop.length > height || drop.x < -width * 0.05) {
          drop.y = -drop.length - next() * height * 0.2
          drop.x = next() * width * 1.2
        }
        const length = drop.length * (near ? 1.6 : 1)
        context.moveTo(drop.x, drop.y)
        context.lineTo(drop.x + length * wind, drop.y - length)
      }
      context.stroke()
    }
  }

  function drawRipples(time: number): void {
    if (time >= rippleAt) {
      rippleAt = time + 0.08 + next() * 0.18
      ripples.push({ x: next() * width, y: height * (STREET + 0.015 + next() * (0.97 - STREET)), start: time })
      if (ripples.length > 18) ripples.shift()
    }
    context.lineWidth = Math.max(1, unit)
    for (const ripple of ripples) {
      const progress = (time - ripple.start) / 1.1
      if (progress >= 1) continue
      const depth = (ripple.y / height - STREET) / (1 - STREET)
      const radius = unit * (4 + progress * 26) * (0.6 + depth)
      context.strokeStyle = `rgb(200 215 255 / ${Math.round((1 - progress) * 32)}%)`
      context.beginPath()
      context.ellipse(ripple.x, ripple.y, radius, radius * 0.28, 0, 0, Math.PI * 2)
      context.stroke()
    }
  }

  function drawBokeh(time: number): void {
    context.globalCompositeOperation = 'lighter'
    const base = Math.min(width, height)
    for (const light of bokeh) {
      const radius = base * light.radius
      const x = light.x * width + Math.sin(time * 0.05 * light.drift + light.phase) * width * 0.03
      const y = light.y * height + Math.cos(time * 0.04 * light.drift + light.phase) * height * 0.02
      context.globalAlpha = 0.35 + 0.3 * Math.sin(time * 0.3 * light.drift + light.phase)
      context.drawImage(bokehSprites[light.sprite]!, x - radius, y - radius, radius * 2, radius * 2)
    }
    context.globalAlpha = 1
    context.globalCompositeOperation = 'source-over'
  }

  return {
    resize(nextWidth, nextHeight) {
      width = nextWidth
      height = nextHeight
      unit = Math.min(width, height) / 900
      cityLayer = undefined
    },
    render(frame) {
      if (!cityLayer) build()
      flicker(frame.time)
      context.drawImage(cityLayer!.surface, 0, 0)
      drawRipples(frame.time)
      drawRain(frame.delta)
      drawBokeh(frame.time)
    },
  }
}
