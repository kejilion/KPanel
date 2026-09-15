// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useWindowGesture } from './useWindowGesture'
import type { WindowGeometry } from '@/lib/desktopWindowGeometry'

function pointer(type: string, x: number, y: number, id = 1): PointerEvent {
  return new PointerEvent(type, {
    bubbles: true,
    clientX: x,
    clientY: y,
    pointerId: id,
    button: 0,
  })
}

describe('useWindowGesture', () => {
  let getGeometry: () => WindowGeometry
  let updateGeometry: (geometry: WindowGeometry) => void
  let onMoveStart: () => void
  let onMoveEnd: () => void
  let target: HTMLElement

  beforeEach(() => {
    getGeometry = vi.fn(() => ({ left: 100, top: 100, width: 800, height: 600 }))
    updateGeometry = vi.fn()
    onMoveStart = vi.fn()
    onMoveEnd = vi.fn()
    target = document.createElement('div')
    document.body.appendChild(target)
  })

  it('moves the window by the pointer delta', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry, {
      onStart: onMoveStart,
      onEnd: onMoveEnd,
    })
    gesture.onPointerDown(pointer('pointerdown', 100, 100, 1), null)
    window.dispatchEvent(pointer('pointermove', 140, 130, 1))
    window.dispatchEvent(pointer('pointerup', 140, 130, 1))
    await nextTick()
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 140, top: 130, width: 800, height: 600 })
    expect(onMoveStart).toHaveBeenCalledTimes(1)
    expect(onMoveEnd).toHaveBeenCalledTimes(1)
  })

  it('resizes from the south-east edge', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    gesture.onPointerDown(pointer('pointerdown', 900, 700, 1), 'se')
    window.dispatchEvent(pointer('pointermove', 950, 740, 1))
    window.dispatchEvent(pointer('pointerup', 950, 740, 1))
    await nextTick()
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 100, top: 100, width: 850, height: 640 })
  })

  it('resizes from the west edge, moving the left edge and widening', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    gesture.onPointerDown(pointer('pointerdown', 100, 300, 1), 'w')
    window.dispatchEvent(pointer('pointermove', 60, 300, 1))
    window.dispatchEvent(pointer('pointerup', 60, 300, 1))
    await nextTick()
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 60, top: 100, width: 840, height: 600 })
  })

  it('resizes from the north edge, moving the top edge and growing', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    gesture.onPointerDown(pointer('pointerdown', 400, 100, 1), 'n')
    window.dispatchEvent(pointer('pointermove', 400, 70, 1))
    window.dispatchEvent(pointer('pointerup', 400, 70, 1))
    await nextTick()
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 100, top: 70, width: 800, height: 630 })
  })

  it('clamps resize to the minimum size', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    gesture.onPointerDown(pointer('pointerdown', 900, 700, 1), 'se')
    window.dispatchEvent(pointer('pointermove', 300, 200, 1))
    window.dispatchEvent(pointer('pointerup', 300, 200, 1))
    await nextTick()
    const called = (updateGeometry as ReturnType<typeof vi.fn>).mock.calls[0]?.[0] as { width: number; height: number }
    expect(called.width).toBeGreaterThanOrEqual(420)
    expect(called.height).toBeGreaterThanOrEqual(280)
  })

  it('ignores non-primary mouse buttons', async () => {
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    const rightClick = new MouseEvent('pointerdown', { bubbles: true, button: 2, clientX: 100, clientY: 100 })
    gesture.onPointerDown(rightClick as PointerEvent, null)
    window.dispatchEvent(pointer('pointermove', 200, 200, 1))
    await nextTick()
    expect(updateGeometry).not.toHaveBeenCalled()
  })

  it('activates a move after a small threshold and can replace its restore geometry', async () => {
    const onMoveActivate = vi.fn(() => ({ left: 300, top: 40, width: 700, height: 500 }))
    const onMove = vi.fn()
    const onEnd = vi.fn()
    const gesture = useWindowGesture(getGeometry, updateGeometry, { onMoveActivate, onMove, onEnd })

    gesture.onPointerDown(pointer('pointerdown', 620, 20, 1), null)
    window.dispatchEvent(pointer('pointermove', 621, 21, 1))
    expect(onMoveActivate).not.toHaveBeenCalled()
    window.dispatchEvent(pointer('pointermove', 630, 30, 1))
    window.dispatchEvent(pointer('pointermove', 650, 45, 1))
    window.dispatchEvent(pointer('pointerup', 650, 45, 1))
    await nextTick()

    expect(onMoveActivate).toHaveBeenCalledTimes(1)
    expect(onMove).toHaveBeenCalled()
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 320, top: 55, width: 700, height: 500 })
    expect(onEnd).toHaveBeenCalledWith(expect.objectContaining({ kind: 'move', moved: true, cancelled: false }))
  })

  it('keeps the active pointer as the sole gesture owner and reports cancellation', async () => {
    const onEnd = vi.fn()
    const gesture = useWindowGesture(getGeometry, updateGeometry, { onEnd })
    gesture.onPointerDown(pointer('pointerdown', 100, 100, 1), null)
    gesture.onPointerDown(pointer('pointerdown', 200, 200, 2), null)
    window.dispatchEvent(pointer('pointermove', 260, 260, 2))
    expect(updateGeometry).not.toHaveBeenCalled()
    window.dispatchEvent(pointer('pointermove', 130, 130, 1))
    window.dispatchEvent(pointer('pointercancel', 130, 130, 1))
    await nextTick()
    expect(updateGeometry).toHaveBeenCalledTimes(1)
    expect(onEnd).toHaveBeenCalledWith(expect.objectContaining({ moved: true, cancelled: true }))
  })

  it('recovers from lost pointer capture and accepts the next gesture', async () => {
    const onStart = vi.fn()
    const onEnd = vi.fn()
    const gesture = useWindowGesture(getGeometry, updateGeometry, { onStart, onEnd })
    target.setPointerCapture = vi.fn()
    target.hasPointerCapture = vi.fn(() => false)
    target.releasePointerCapture = vi.fn()
    target.addEventListener('pointerdown', (event) => gesture.onPointerDown(event as PointerEvent, null))

    target.dispatchEvent(pointer('pointerdown', 100, 100, 1))
    window.dispatchEvent(pointer('pointermove', 140, 130, 1))
    target.dispatchEvent(pointer('lostpointercapture', 140, 130, 1))
    await nextTick()

    expect(target.setPointerCapture).toHaveBeenCalledWith(1)
    expect(onEnd).toHaveBeenLastCalledWith(expect.objectContaining({ moved: true, cancelled: true }))
    expect(gesture.active.value).toBe(false)
    target.dispatchEvent(pointer('pointerdown', 200, 200, 2))
    window.dispatchEvent(pointer('pointerup', 200, 200, 2))
    expect(onStart).toHaveBeenCalledTimes(2)
    expect(onEnd).toHaveBeenCalledTimes(2)
  })

  it('allows an owning control to cancel an active gesture explicitly', async () => {
    const onEnd = vi.fn()
    const gesture = useWindowGesture(getGeometry, updateGeometry, { onEnd })
    gesture.onPointerDown(pointer('pointerdown', 100, 100, 1), null)
    window.dispatchEvent(pointer('pointermove', 140, 130, 1))

    gesture.cancel()
    await nextTick()

    expect(gesture.active.value).toBe(false)
    expect(onEnd).toHaveBeenCalledWith(expect.objectContaining({ moved: true, cancelled: true }))
  })

  it('coalesces rapid pointer moves into one geometry update per animation frame', () => {
    let frame: FrameRequestCallback | undefined
    const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frame = callback
      return 17
    })
    const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => undefined)
    const gesture = useWindowGesture(getGeometry, updateGeometry)
    gesture.onPointerDown(pointer('pointerdown', 100, 100, 1), null)
    window.dispatchEvent(pointer('pointermove', 120, 120, 1))
    window.dispatchEvent(pointer('pointermove', 150, 145, 1))
    window.dispatchEvent(pointer('pointermove', 180, 170, 1))

    expect(updateGeometry).not.toHaveBeenCalled()
    frame?.(16)
    expect(updateGeometry).toHaveBeenCalledTimes(1)
    expect(updateGeometry).toHaveBeenLastCalledWith({ left: 180, top: 170, width: 800, height: 600 })
    window.dispatchEvent(pointer('pointerup', 180, 170, 1))
    requestFrame.mockRestore()
    cancelFrame.mockRestore()
  })
})
