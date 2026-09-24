import {
  clamp,
  drawSprite,
  paintSprite,
  type SceneSurface,
  type ScenePainterFactory,
  type SceneViewport,
} from '../painterKit'

interface Petal {
  x: number
  y: number
  depth: number
  vx: number
  fall: number
  rotation: number
  spin: number
  flip: number
  flipSpeed: number
  sway: number
  sprite: number
}

const PETALS_PER_MEGAPIXEL = 48
const MAX_PETALS = 130
const PETAL_TINTS = [
  ['#fff4f8', '#f6b3c9'],
  ['#ffe9f1', '#ee9cba'],
  ['#fffafc', '#f9c7d8'],
  ['#ffe2ec', '#e889aa'],
] as const

/** Sakura Slope: petals ride a breathing wind; the entrance is one strong gust from the left. */
const createSakuraPainter: ScenePainterFactory = (environment) => {
  const { random } = environment
  const sprites = PETAL_TINTS.map(([light, deep]) => paintSprite(environment, 48, 48, (context, size) => {
    context.translate(size / 2, size / 2)
    const gradient = context.createLinearGradient(0, -size * 0.42, 0, size * 0.4)
    gradient.addColorStop(0, light)
    gradient.addColorStop(1, deep)
    context.fillStyle = gradient
    context.beginPath()
    // A petal with the characteristic notch at its tip.
    context.moveTo(0, size * 0.42)
    context.bezierCurveTo(-size * 0.36, size * 0.18, -size * 0.3, -size * 0.3, -size * 0.07, -size * 0.4)
    context.lineTo(0, -size * 0.3)
    context.lineTo(size * 0.07, -size * 0.4)
    context.bezierCurveTo(size * 0.3, -size * 0.3, size * 0.36, size * 0.18, 0, size * 0.42)
    context.fill()
    context.globalAlpha = 0.45
    context.strokeStyle = '#ffffff'
    context.lineWidth = size * 0.03
    context.beginPath()
    context.moveTo(0, size * 0.34)
    context.quadraticCurveTo(-size * 0.04, 0, 0, -size * 0.24)
    context.stroke()
  })).filter((sprite): sprite is SceneSurface => Boolean(sprite))

  const petals: Petal[] = []
  let viewport: SceneViewport = { width: 0, height: 0, image: { x: 0, y: 0, width: 0, height: 0 } }
  let target = 0
  let density = 1

  function spawn(petal: Petal, entering: boolean, anywhere: boolean): void {
    petal.depth = 0.5 + random() * 0.75
    petal.fall = (26 + random() * 30) * petal.depth
    petal.rotation = random() * Math.PI * 2
    petal.spin = (random() - 0.5) * 2.4
    petal.flip = random() * Math.PI * 2
    petal.flipSpeed = 1.2 + random() * 2.6
    petal.sway = random() * Math.PI * 2
    petal.sprite = Math.floor(random() * Math.max(1, sprites.length))
    petal.vx = entering ? 260 + random() * 220 : 20 * petal.depth
    if (anywhere) {
      petal.x = random() * viewport.width
      petal.y = random() * viewport.height
    } else if (entering) {
      // A wind front: half of the petals already inside the left of the view.
      petal.x = (random() * 1.1 - 0.55) * viewport.width
      petal.y = random() * viewport.height * 0.9
    } else if (random() < 0.45) {
      petal.x = -40 - random() * viewport.width * 0.2
      petal.y = random() * viewport.height * 0.85
    } else {
      petal.x = random() * viewport.width * 1.1 - viewport.width * 0.1
      petal.y = -30 - random() * 60
    }
  }

  function reconcile(): void {
    target = Math.min(MAX_PETALS, Math.round((viewport.width * viewport.height) / 1e6 * PETALS_PER_MEGAPIXEL * density))
    while (petals.length < target) {
      const petal = {} as Petal
      spawn(petal, false, true)
      petals.push(petal)
    }
    petals.length = Math.min(petals.length, target)
  }

  let gustStarted = false

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
      if (!sprites.length) return
      if (!gustStarted && frame.entrance < 1) {
        gustStarted = true
        // The entrance gust: petals regroup along a wind front and sweep across.
        for (const petal of petals) spawn(petal, true, false)
      }
      const gust = frame.entrance < 1 ? (1 - frame.entrance) ** 1.5 * 420 : 0
      const wind = 24 + 22 * Math.sin(frame.time * 0.21) + 12 * Math.sin(frame.time * 0.057 + 1.3) + gust
      const size = clamp(viewport.width / 96, 12, 26)
      for (const petal of petals) {
        const desired = wind * petal.depth
        petal.vx += (desired - petal.vx) * Math.min(1, frame.dt * 1.4)
        petal.sway += frame.dt * 1.3
        petal.x += (petal.vx + Math.sin(petal.sway) * 14 * petal.depth) * frame.dt
        petal.y += (petal.fall + Math.cos(petal.sway * 0.7) * 6) * frame.dt
        petal.rotation += petal.spin * frame.dt
        petal.flip += petal.flipSpeed * frame.dt
        if (petal.y > viewport.height + 30 || petal.x > viewport.width + 50) spawn(petal, false, false)
        const flatten = Math.cos(petal.flip)
        context.globalAlpha = 0.5 + petal.depth * 0.38
        drawSprite(
          context,
          sprites[petal.sprite % sprites.length]!,
          petal.x,
          petal.y,
          size * petal.depth,
          frame.pixelRatio,
          petal.rotation,
          Math.abs(flatten) < 0.18 ? 0.18 * Math.sign(flatten || 1) : flatten,
        )
      }
      context.globalAlpha = 1
    },
  }
}

export default createSakuraPainter
