import { onBeforeUnmount, ref } from 'vue'
import type { WindowGeometry } from '@/lib/desktopWindowGeometry'
import { MIN_WINDOW_WIDTH, MIN_WINDOW_HEIGHT } from '@/lib/desktopWindowGeometry'

/**
 * Drag-to-move and edge-resize behaviour for a desktop window.
 *
 * Pointer events drive a move/resize gesture; the gesture updates the window
 * geometry through the store's `updateGeometry`, which clamps against the
 * viewport. The component remains responsive during the gesture because state
 * is applied on every pointermove (throttled by the browser frame rate) rather
 * than only on pointerup.
 */

export type ResizeEdge = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

type Gesture =
  | { kind: 'move'; startX: number; startY: number; startGeometry: WindowGeometry; moved: boolean }
  | { kind: 'resize'; edge: ResizeEdge; startX: number; startY: number; startGeometry: WindowGeometry; moved: boolean }

export type WindowGestureKind = Gesture['kind']

export interface WindowGestureEnd {
  kind: WindowGestureKind
  event?: PointerEvent
  moved: boolean
  cancelled: boolean
}

export interface WindowGestureCallbacks {
  onStart?: (kind: WindowGestureKind, event: PointerEvent) => void
  /** Called once after a short move threshold; may replace the drag's restore geometry. */
  onMoveActivate?: (event: PointerEvent) => WindowGeometry | undefined
  onMove?: (kind: WindowGestureKind, event: PointerEvent) => void
  onEnd?: (result: WindowGestureEnd) => void
}

const RESIZE_MARGIN = 6

function edgeForTarget(target: HTMLElement): ResizeEdge | null {
  const edge = target.dataset.resizeEdge
  if (!edge) return null
  const valid: ResizeEdge[] = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw']
  return valid.includes(edge as ResizeEdge) ? (edge as ResizeEdge) : null
}

function beginGesture(event: PointerEvent, startGeometry: WindowGeometry, edge: ResizeEdge | null): Gesture {
  if (edge) return { kind: 'resize', edge, startX: event.clientX, startY: event.clientY, startGeometry, moved: false }
  return { kind: 'move', startX: event.clientX, startY: event.clientY, startGeometry, moved: false }
}

export function useWindowGesture(
  getGeometry: () => WindowGeometry,
  updateGeometry: (geometry: WindowGeometry) => void,
  callbacks: WindowGestureCallbacks = {},
) {
  const active = ref(false)
  let gesture: Gesture | null = null
  let pointerId: number | undefined
  let captureTarget: HTMLElement | null = null
  let pendingGeometry: WindowGeometry | undefined
  let updateFrame: number | undefined

  function flushGeometry(): void {
    if (updateFrame !== undefined) {
      window.cancelAnimationFrame(updateFrame)
      updateFrame = undefined
    }
    if (!pendingGeometry) return
    const next = pendingGeometry
    pendingGeometry = undefined
    updateGeometry(next)
  }

  function scheduleGeometry(geometry: WindowGeometry): void {
    pendingGeometry = geometry
    if (updateFrame !== undefined) return
    updateFrame = window.requestAnimationFrame(() => {
      updateFrame = undefined
      flushGeometry()
    })
  }

  function onPointerDown(event: PointerEvent, edge: ResizeEdge | null): void {
    if (event.button !== 0) return
    if (gesture) return
    const target = event.currentTarget as HTMLElement | null
    if (target?.hasAttribute('data-no-drag')) return
    gesture = beginGesture(event, getGeometry(), edge)
    active.value = true
    pointerId = event.pointerId
    if (target && typeof target.setPointerCapture === 'function') {
      try {
        target.setPointerCapture(event.pointerId)
        captureTarget = target
        target.addEventListener('lostpointercapture', onLostPointerCapture)
      } catch {
        captureTarget = null
      }
    }
    callbacks.onStart?.(gesture.kind, event)
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerCancel)
    window.addEventListener('blur', onWindowBlur)
    event.preventDefault()
  }

  function onPointerMove(event: PointerEvent): void {
    if (!gesture || pointerId !== event.pointerId) return
    let dx = event.clientX - gesture.startX
    let dy = event.clientY - gesture.startY
    if (!gesture.moved) {
      if (gesture.kind === 'move' && Math.hypot(dx, dy) < 3) {
        event.preventDefault()
        return
      }
      gesture.moved = dx !== 0 || dy !== 0
      if (gesture.kind === 'move') {
        const restored = callbacks.onMoveActivate?.(event)
        if (restored) {
          gesture.startGeometry = restored
          gesture.startX = event.clientX
          gesture.startY = event.clientY
          dx = 0
          dy = 0
        }
      }
    }
    callbacks.onMove?.(gesture.kind, event)
    const next: WindowGeometry = { ...gesture.startGeometry }
    if (gesture.kind === 'move') {
      next.left = gesture.startGeometry.left + dx
      next.top = gesture.startGeometry.top + dy
    } else {
      const { edge } = gesture
      if (edge.includes('e')) next.width = Math.max(MIN_WINDOW_WIDTH, gesture.startGeometry.width + dx)
      if (edge.includes('s')) next.height = Math.max(MIN_WINDOW_HEIGHT, gesture.startGeometry.height + dy)
      if (edge.includes('w')) {
        next.width = Math.max(MIN_WINDOW_WIDTH, gesture.startGeometry.width - dx)
        next.left = gesture.startGeometry.left + (gesture.startGeometry.width - next.width)
      }
      if (edge.includes('n')) {
        next.height = Math.max(MIN_WINDOW_HEIGHT, gesture.startGeometry.height - dy)
        next.top = gesture.startGeometry.top + (gesture.startGeometry.height - next.height)
      }
    }
    if (dx !== 0 || dy !== 0) scheduleGeometry(next)
    event.preventDefault()
  }

  function onPointerUp(event: PointerEvent): void {
    if (pointerId !== event.pointerId) return
    finishGesture(event, false)
  }

  function onPointerCancel(event: PointerEvent): void {
    if (pointerId !== event.pointerId) return
    finishGesture(event, true)
  }

  function onLostPointerCapture(event: PointerEvent): void {
    if (pointerId !== event.pointerId) return
    finishGesture(event, true)
  }

  function onWindowBlur(): void {
    if (!gesture) return
    finishGesture(undefined, true)
  }

  function releasePointerCapture(): void {
    const target = captureTarget
    const activePointerId = pointerId
    captureTarget = null
    if (!target) return
    target.removeEventListener('lostpointercapture', onLostPointerCapture)
    if (activePointerId === undefined || typeof target.releasePointerCapture !== 'function') return
    try {
      if (typeof target.hasPointerCapture !== 'function' || target.hasPointerCapture(activePointerId)) {
        target.releasePointerCapture(activePointerId)
      }
    } catch {
      // The browser may already have released capture after pointerup.
    }
  }

  function finishGesture(event?: PointerEvent, cancelled = true): void {
    const result: WindowGestureEnd | undefined = gesture
      ? { kind: gesture.kind, event, moved: gesture.moved, cancelled }
      : undefined
    flushGeometry()
    releasePointerCapture()
    gesture = null
    active.value = false
    pointerId = undefined
    if (result) callbacks.onEnd?.(result)
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerCancel)
    window.removeEventListener('blur', onWindowBlur)
  }

  onBeforeUnmount(() => finishGesture(undefined, true))

  return {
    onPointerDown,
    edgeForTarget,
    cancel: () => finishGesture(undefined, true),
    active,
    RESIZE_MARGIN,
  }
}
