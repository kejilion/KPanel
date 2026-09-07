/**
 * Desktop window geometry helpers.
 *
 * Windows live in a viewport-sized coordinate space (CSS pixels). Geometry is
 * kept as plain data so it can be persisted to localStorage and unit-tested
 * without a DOM.
 */

export interface WindowGeometry {
  left: number
  top: number
  width: number
  height: number
}

export interface ViewportSize {
  width: number
  height: number
}

export type WindowSnap = 'left' | 'right'
export type WindowSnapTarget = WindowSnap | 'maximize'

export interface ViewportPoint {
  x: number
  y: number
}

export const MIN_WINDOW_WIDTH = 420
export const MIN_WINDOW_HEIGHT = 280
export const DEFAULT_WINDOW_WIDTH = 880
export const DEFAULT_WINDOW_HEIGHT = 600
/** Taskbar + window chrome allowance so a window never opens fully offscreen. */
const TOP_MARGIN = 16
const SIDE_MARGIN = 24
const BOTTOM_MARGIN = 72
// Leave drag space beside the three 46px title-bar action buttons.
const MIN_VISIBLE_TITLEBAR_WIDTH = 200
const TITLEBAR_HEIGHT = 42
const SNAP_INSET = 10
const SNAP_GAP = 10
const SNAP_EDGE_THRESHOLD = 18
const MIN_SIDE_SNAP_VIEWPORT_WIDTH = 760

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}

/**
 * Clamp a geometry so the title bar stays reachable and at least a slice of
 * the window remains on screen. Width and height are first bounded to the
 * viewport, then position is clamped so a visible, grabbable portion always
 * remains inside the viewport.
 */
export function clampToViewport(geometry: WindowGeometry, viewport: ViewportSize): WindowGeometry {
  const maxWidth = Math.max(viewport.width - SIDE_MARGIN * 2, 1)
  const maxHeight = Math.max(viewport.height - TOP_MARGIN - BOTTOM_MARGIN, 1)
  const width = clamp(geometry.width, Math.min(MIN_WINDOW_WIDTH, maxWidth), maxWidth)
  const height = clamp(geometry.height, Math.min(MIN_WINDOW_HEIGHT, maxHeight), maxHeight)
  const visibleWidth = Math.min(MIN_VISIBLE_TITLEBAR_WIDTH, width, viewport.width)
  const left = clamp(geometry.left, visibleWidth - width, viewport.width - visibleWidth)
  const maxTop = Math.max(viewport.height - BOTTOM_MARGIN - TITLEBAR_HEIGHT, TOP_MARGIN)
  const top = clamp(geometry.top, TOP_MARGIN, maxTop)
  return { left, top, width, height }
}

/**
 * Compute a cascade position for the next window of a given application so
 * repeated opens do not stack exactly on top of each other. Falls back to the
 * center when the viewport is too small to fit a default window.
 */
export function cascadePosition(
  index: number,
  viewport: ViewportSize,
  width = DEFAULT_WINDOW_WIDTH,
  height = DEFAULT_WINDOW_HEIGHT,
): WindowGeometry {
  const usableWidth = Math.max(viewport.width - SIDE_MARGIN * 2, 1)
  const usableHeight = Math.max(viewport.height - TOP_MARGIN - BOTTOM_MARGIN, 1)
  const w = Math.min(width, usableWidth)
  const h = Math.min(height, usableHeight)
  const offset = (index % 6) * 28
  return clampToViewport(
    {
      left: clamp((viewport.width - w) / 2 + offset, 0, Math.max(viewport.width - SIDE_MARGIN - w, 0)),
      top: clamp(TOP_MARGIN + (usableHeight - h) / 2 - offset / 2, TOP_MARGIN, TOP_MARGIN + usableHeight - h),
      width: w,
      height: h,
    },
    viewport,
  )
}

/** Normalize user-supplied geometry (e.g. from localStorage) against a viewport. */
export function normalizeGeometry(raw: Partial<WindowGeometry> | null | undefined, viewport: ViewportSize): WindowGeometry {
  if (!raw) return cascadePosition(0, viewport)
  const width = Number.isFinite(raw.width) ? raw.width! : DEFAULT_WINDOW_WIDTH
  const height = Number.isFinite(raw.height) ? raw.height! : DEFAULT_WINDOW_HEIGHT
  const left = Number.isFinite(raw.left) ? raw.left! : (viewport.width - width) / 2
  const top = Number.isFinite(raw.top) ? raw.top! : (viewport.height - height) / 2
  return clampToViewport({ left, top, width, height }, viewport)
}

/** Whether the viewport can present two useful half-width desktop windows. */
export function supportsSideWindowSnap(viewport: ViewportSize): boolean {
  return viewport.width >= MIN_SIDE_SNAP_VIEWPORT_WIDTH
}

/** Resolve the small set of Windows-style edge targets supported by KPanel. */
export function detectWindowSnapTarget(point: ViewportPoint, viewport: ViewportSize): WindowSnapTarget | null {
  if (point.y <= SNAP_EDGE_THRESHOLD) return 'maximize'
  if (!supportsSideWindowSnap(viewport)) return null
  if (point.x <= SNAP_EDGE_THRESHOLD) return 'left'
  if (point.x >= viewport.width - SNAP_EDGE_THRESHOLD) return 'right'
  return null
}

/** Geometry shared by the live snap preview and the final snapped window. */
export function geometryForWindowSnap(target: WindowSnapTarget, viewport: ViewportSize): WindowGeometry {
  const height = Math.max(viewport.height - SNAP_INSET - BOTTOM_MARGIN, 1)
  if (target === 'maximize') {
    return {
      left: SNAP_INSET,
      top: SNAP_INSET,
      width: Math.max(viewport.width - SNAP_INSET * 2, 1),
      height,
    }
  }

  const width = Math.max((viewport.width - SNAP_INSET * 2 - SNAP_GAP) / 2, 1)
  return {
    left: target === 'left' ? SNAP_INSET : SNAP_INSET + width + SNAP_GAP,
    top: SNAP_INSET,
    width,
    height,
  }
}
