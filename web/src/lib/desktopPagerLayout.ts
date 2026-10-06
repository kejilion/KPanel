/**
 * Phone-style paged desktop layout.
 *
 * Compact viewports show the desktop like a phone home screen: icons fill
 * each page left-to-right, then top-to-bottom, and further pages continue to
 * the right. The layout is derived only; it never writes workspace positions.
 */

import type { DesktopIconBounds, DesktopIconPosition } from '@/lib/desktopIconLayout'

export interface DesktopPagerMetrics {
  /** Rendered compact icon slot size; each slot is centered inside its cell. */
  iconWidth: number
  iconHeight: number
  /** Smallest cell that keeps labels and touch targets readable. */
  minCellWidth: number
  minCellHeight: number
  minColumns: number
  maxColumns: number
}

export interface DesktopPagerGrid {
  bounds: DesktopIconBounds
  metrics: DesktopPagerMetrics
  columns: number
  rows: number
  pageCapacity: number
  pageWidth: number
  cellWidth: number
  cellHeight: number
}

export interface DesktopPagerPlacement {
  key: string
  index: number
  page: number
  row: number
  column: number
  left: number
  top: number
}

export interface DesktopPagerLayout {
  grid: DesktopPagerGrid
  placements: DesktopPagerPlacement[]
  pageCount: number
}

export const DEFAULT_DESKTOP_PAGER_METRICS: Readonly<DesktopPagerMetrics> = Object.freeze({
  iconWidth: 82,
  iconHeight: 89,
  minCellWidth: 84,
  minCellHeight: 108,
  minColumns: 1,
  maxColumns: 6,
})

/** Saved wide x values closer than this share one visual column. */
const COLUMN_TOLERANCE = 0.015

function finite(value: number, fallback = 0): number {
  return Number.isFinite(value) ? Math.max(0, value) : fallback
}

/** Describe the phone grid for the measured icon work area. */
export function desktopPagerGrid(
  bounds: DesktopIconBounds,
  metrics: DesktopPagerMetrics = DEFAULT_DESKTOP_PAGER_METRICS,
): DesktopPagerGrid {
  const width = finite(bounds.width)
  const height = finite(bounds.height)
  const columns = Math.min(metrics.maxColumns, Math.max(metrics.minColumns, Math.floor(width / metrics.minCellWidth)))
  const rows = Math.max(1, Math.floor(height / metrics.minCellHeight))
  return {
    bounds: { width, height },
    metrics,
    columns,
    rows,
    pageCapacity: columns * rows,
    pageWidth: width,
    cellWidth: width / columns,
    cellHeight: rows > 1 ? height / rows : Math.max(metrics.iconHeight, height),
  }
}

/**
 * Order keys for the phone screen. Keys with a saved wide-screen position keep
 * the desktop's column-major reading order, so the first desktop column becomes
 * the first phone row. Keys without a saved position follow afterwards in
 * their catalog order, like newly installed apps on a phone.
 */
export function desktopPagerOrder(
  keys: readonly string[],
  savedPositions: Readonly<Record<string, DesktopIconPosition | undefined>>,
): string[] {
  const seen = new Set<string>()
  const unique = keys.filter((key) => key && !seen.has(key) && seen.add(key))
  const saved = unique.flatMap((key, index) => {
    const position = savedPositions[key]
    return position && Number.isFinite(position.x) && Number.isFinite(position.y)
      ? [{ key, index, x: position.x, y: position.y }]
      : []
  })
  const columns = [...new Set(saved.map((item) => item.x))].sort((left, right) => left - right)
  const columnOf = new Map<number, number>()
  let column = -1
  let previous = -Infinity
  for (const x of columns) {
    if (x - previous > COLUMN_TOLERANCE) column += 1
    columnOf.set(x, column)
    previous = x
  }
  saved.sort((left, right) => (
    columnOf.get(left.x)! - columnOf.get(right.x)!
    || left.y - right.y
    || left.index - right.index
  ))
  const placed = new Set(saved.map((item) => item.key))
  return [...saved.map((item) => item.key), ...unique.filter((key) => !placed.has(key))]
}

/** Place ordered keys row-major on consecutive horizontal pages. */
export function layoutDesktopPager(
  orderedKeys: readonly string[],
  bounds: DesktopIconBounds,
  metrics: DesktopPagerMetrics = DEFAULT_DESKTOP_PAGER_METRICS,
): DesktopPagerLayout {
  const grid = desktopPagerGrid(bounds, metrics)
  const insetX = Math.max(0, (grid.cellWidth - metrics.iconWidth) / 2)
  const insetY = Math.max(0, (grid.cellHeight - metrics.iconHeight) / 2)
  const placements = orderedKeys.map((key, index) => {
    const page = Math.floor(index / grid.pageCapacity)
    const withinPage = index % grid.pageCapacity
    const row = Math.floor(withinPage / grid.columns)
    const column = withinPage % grid.columns
    return {
      key,
      index,
      page,
      row,
      column,
      left: page * grid.pageWidth + column * grid.cellWidth + insetX,
      top: row * grid.cellHeight + insetY,
    }
  })
  return {
    grid,
    placements,
    pageCount: Math.max(1, Math.ceil(orderedKeys.length / grid.pageCapacity)),
  }
}

/** Resolve the page nearest to a horizontal scroll offset. */
export function desktopPagerPageForScroll(scrollLeft: number, pageWidth: number, pageCount: number): number {
  if (!(pageWidth > 0) || pageCount <= 1) return 0
  return Math.min(pageCount - 1, Math.max(0, Math.round(finite(scrollLeft) / pageWidth)))
}
