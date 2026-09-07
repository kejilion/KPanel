import { describe, expect, it } from 'vitest'
import {
  clampToViewport,
  cascadePosition,
  detectWindowSnapTarget,
  geometryForWindowSnap,
  normalizeGeometry,
  supportsSideWindowSnap,
  MIN_WINDOW_WIDTH,
  MIN_WINDOW_HEIGHT,
} from './desktopWindowGeometry'

describe('desktop window geometry', () => {
  it('clamps a window to minimum size', () => {
    const result = clampToViewport({ left: 0, top: 0, width: 100, height: 100 }, { width: 1280, height: 800 })
    expect(result.width).toBe(MIN_WINDOW_WIDTH)
    expect(result.height).toBe(MIN_WINDOW_HEIGHT)
  })

  it('clamps a window that exceeds the viewport', () => {
    const result = clampToViewport(
      { left: -1000, top: -1000, width: 5000, height: 5000 },
      { width: 1280, height: 800 },
    )
    expect(result.width).toBeLessThanOrEqual(1280 - 24 * 2)
    expect(result.height).toBeLessThanOrEqual(800 - 16 - 72)
    expect(result.left + result.width).toBeGreaterThanOrEqual(200)
    expect(result.top).toBeGreaterThanOrEqual(16)
  })

  it('keeps the title bar reachable when a window is far off the top edge', () => {
    const result = clampToViewport({ left: 40, top: -2000, width: 900, height: 600 }, { width: 1280, height: 800 })
    expect(result.top).toBeGreaterThanOrEqual(16)
  })

  it('clamps left so a visible slice remains on screen', () => {
    const result = clampToViewport({ left: 2000, top: 50, width: 900, height: 600 }, { width: 1280, height: 800 })
    expect(result.left).toBeLessThanOrEqual(1280 - 24)
    expect(result.left + result.width).toBeGreaterThan(24)
  })

  it('cascades repeated windows without stacking exactly', () => {
    const viewport = { width: 1280, height: 800 }
    const first = cascadePosition(0, viewport)
    const second = cascadePosition(1, viewport)
    const third = cascadePosition(2, viewport)
    expect(second.left).not.toBe(first.left)
    expect(third.left).not.toBe(second.left)
    expect(third.top).not.toBe(second.top)
  })

  it.each([
    { left: 1000, top: 460, width: 880, height: 600 },
    { left: -500, top: 250, width: 880, height: 600 },
  ])('preserves partially offscreen floating geometry: $left, $top', (geometry) => {
    const viewport = { width: 1280, height: 800 }
    expect(clampToViewport(geometry, viewport)).toEqual(geometry)
    expect(normalizeGeometry(geometry, viewport)).toEqual(geometry)
  })

  it.each([{ width: 1920, height: 1080 }, { width: 768, height: 600 }, { width: 390, height: 844 }])(
    'keeps a usable title bar above the taskbar after extreme moves in $width px',
    (viewport) => {
      for (const left of [-10000, 10000]) {
        const geometry = clampToViewport({ left, top: 10000, width: 880, height: 600 }, viewport)
        const visibleWidth = Math.min(geometry.left + geometry.width, viewport.width) - Math.max(geometry.left, 0)
        expect(visibleWidth).toBeGreaterThanOrEqual(200)
        expect(geometry.top + 42).toBeLessThanOrEqual(viewport.height - 72)
      }
    },
  )

  it('keeps every cascade position fully inside the viewport', () => {
    const viewport = { width: 1280, height: 800 }
    for (let index = 0; index < 12; index += 1) {
      const position = cascadePosition(index, viewport)
      expect(position.left).toBeGreaterThanOrEqual(0)
      expect(position.top).toBeGreaterThanOrEqual(16)
      expect(position.left + position.width).toBeLessThanOrEqual(viewport.width)
      expect(position.top + position.height).toBeLessThanOrEqual(viewport.height - 72)
    }
  })

  it('falls back to a centered position on a small viewport', () => {
    const result = cascadePosition(0, { width: 500, height: 400 })
    expect(result.width).toBeLessThanOrEqual(500 - 24 * 2)
    expect(result.left).toBeGreaterThanOrEqual(0)
  })

  it('normalizes missing geometry to a cascade position', () => {
    const geometry = normalizeGeometry(null, { width: 1280, height: 800 })
    expect(geometry.width).toBeGreaterThan(0)
    expect(geometry.height).toBeGreaterThan(0)
  })

  it('normalizes invalid persisted geometry against the viewport', () => {
    const geometry = normalizeGeometry(
      { left: -5000, top: -5000, width: 20000, height: 20 },
      { width: 1280, height: 800 },
    )
    expect(geometry.width).toBeLessThanOrEqual(1280 - 24 * 2)
    expect(geometry.height).toBeGreaterThanOrEqual(MIN_WINDOW_HEIGHT)
    expect(geometry.left + geometry.width).toBeGreaterThanOrEqual(200)
    expect(geometry.top).toBeGreaterThanOrEqual(16)
  })

  it('detects only the lightweight top and side snap targets', () => {
    const viewport = { width: 1280, height: 800 }
    expect(detectWindowSnapTarget({ x: 640, y: 8 }, viewport)).toBe('maximize')
    expect(detectWindowSnapTarget({ x: 8, y: 300 }, viewport)).toBe('left')
    expect(detectWindowSnapTarget({ x: 1272, y: 300 }, viewport)).toBe('right')
    expect(detectWindowSnapTarget({ x: 640, y: 300 }, viewport)).toBeNull()
  })

  it('prioritizes top maximize at a corner and disables side snap on narrow viewports', () => {
    expect(detectWindowSnapTarget({ x: 2, y: 2 }, { width: 1280, height: 800 })).toBe('maximize')
    expect(supportsSideWindowSnap({ width: 759, height: 800 })).toBe(false)
    expect(detectWindowSnapTarget({ x: 2, y: 300 }, { width: 759, height: 800 })).toBeNull()
  })

  it('lays out symmetric half windows above the taskbar safety area', () => {
    const viewport = { width: 1280, height: 800 }
    const left = geometryForWindowSnap('left', viewport)
    const right = geometryForWindowSnap('right', viewport)
    const maximized = geometryForWindowSnap('maximize', viewport)
    expect(left).toEqual({ left: 10, top: 10, width: 625, height: 718 })
    expect(right).toEqual({ left: 645, top: 10, width: 625, height: 718 })
    expect(right.left - (left.left + left.width)).toBe(10)
    expect(maximized).toEqual({ left: 10, top: 10, width: 1260, height: 718 })
  })
})
