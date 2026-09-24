import { describe, expect, it } from 'vitest'
import { DESKTOP_SCENES } from './catalog'
import { coverImageRect, mulberry32, type ScenePainterEnvironment, type SceneFrame } from './painterKit'

/** Counts the draw calls a painter issues; gradients and paths are inert stand-ins. */
function recordingContext() {
  const counts = { drawImage: 0, stroke: 0, lineTo: 0, setTransform: 0 }
  const gradient = { addColorStop() {} }
  const context = new Proxy({} as Record<string, unknown>, {
    get(target, key: string) {
      if (key in target) return target[key]
      if (key === 'createLinearGradient' || key === 'createRadialGradient') return () => gradient
      return (...args: unknown[]) => {
        if (key in counts) counts[key as keyof typeof counts] += 1
        void args
      }
    },
    set(target, key: string, value) {
      target[key] = value
      return true
    },
  })
  return { context: context as unknown as CanvasRenderingContext2D, counts }
}

function environment(withSurfaces = true): ScenePainterEnvironment {
  return {
    random: mulberry32(7),
    createSurface: withSurfaces
      ? () => ({ getContext: () => recordingContext().context }) as unknown as OffscreenCanvas
      : () => undefined,
  }
}

function frames(count: number, dt = 1 / 30): SceneFrame[] {
  return Array.from({ length: count }, (_, index) => ({
    dt,
    time: (index + 1) * dt,
    entrance: Math.min(1, ((index + 1) * dt) / 3),
    pixelRatio: 1,
  }))
}

const viewport = (width: number, height: number) => ({ width, height, image: coverImageRect(width, height) })

describe.each(DESKTOP_SCENES.map((scene) => [scene.id, scene] as const))('%s painter', (_id, scene) => {
  it('runs a minute of frames inside a bounded draw budget', async () => {
    const painter = (await scene.loadPainter())(environment())
    painter.setParams?.({ day: 0, golden: 0.5, night: 0.5 })
    painter.resize(viewport(2560, 1440))
    const { context, counts } = recordingContext()
    let peak = 0
    for (const frame of frames(1800)) {
      const before = counts.drawImage + counts.lineTo + counts.stroke
      painter.frame(context, frame)
      peak = Math.max(peak, counts.drawImage + counts.lineTo + counts.stroke - before)
    }
    expect(peak).toBeGreaterThan(0)
    // Even at 1440p a frame stays a few hundred sprite blits or line segments.
    expect(peak).toBeLessThanOrEqual(900)
  })

  it('draws less at the governor floor', async () => {
    const measure = async (density: number) => {
      const painter = (await scene.loadPainter())(environment())
      painter.setParams?.({ day: 0, golden: 0.5, night: 0.5 })
      painter.resize(viewport(1920, 1080))
      painter.setDensity(density)
      const { context, counts } = recordingContext()
      for (const frame of frames(240)) painter.frame(context, frame)
      return counts.drawImage + counts.lineTo
    }
    expect(await measure(0.25)).toBeLessThan(await measure(1))
  })

  it('tolerates an empty viewport and missing sprite surfaces', async () => {
    const painter = (await scene.loadPainter())(environment(false))
    painter.resize(viewport(0, 0))
    const { context } = recordingContext()
    expect(() => {
      for (const frame of frames(60)) painter.frame(context, frame)
    }).not.toThrow()
  })
})
