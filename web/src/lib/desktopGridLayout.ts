/**
 * Mixed desktop layout helpers.
 *
 * Icons and widgets share the same logical grid. Positions keep the existing
 * normalized anchor format so old icon layouts remain stable, while widgets
 * declare a rectangular footprint in grid cells.
 */

import {
  clampDesktopIconPosition,
  desktopIconGrid,
  desktopIconGridSlotForPosition,
  desktopIconPositionForGridSlot,
  desktopIconPositionToPixels,
  DEFAULT_DESKTOP_ICON_METRICS,
  MAX_DESKTOP_ICON_POSITIONS,
  type DesktopIconBounds,
  type DesktopIconGrid,
  type DesktopIconGridSlot,
  type DesktopIconMetrics,
  type DesktopIconPixelPosition,
  type DesktopIconPosition,
} from '@/lib/desktopIconLayout'

export interface DesktopGridItem {
  key: string
  /** Width in base grid cells. Icons use 1. */
  columns?: number
  /** Height in base grid cells. Icons use 1. */
  rows?: number
  /** Optional default anchor for a newly visible item. */
  defaultSlot?: DesktopIconGridSlot
}

export interface DesktopGridPlacement {
  key: string
  position: DesktopIconPosition
  columns: number
  rows: number
}

export interface DesktopGridArrangement {
  placements: DesktopGridPlacement[]
  overflowKeys: string[]
  grid: DesktopIconGrid
  contentHeight: number
}

export interface DesktopGridRect extends DesktopIconPixelPosition {
  width: number
  height: number
}

const MAX_GRID_SPAN = 8

function span(value: number | undefined): number {
  if (!Number.isFinite(value)) return 1
  return Math.min(MAX_GRID_SPAN, Math.max(1, Math.floor(value!)))
}

function normalizeItem(item: DesktopGridItem): DesktopGridItem & { columns: number; rows: number } {
  return { ...item, columns: span(item.columns), rows: item.key.startsWith('group:')
    ? Math.min(MAX_DESKTOP_ICON_POSITIONS + 1, Math.max(1, Math.floor(item.rows || 1)))
    : span(item.rows) }
}

function uniqueItems(items: readonly DesktopGridItem[]): Array<DesktopGridItem & { columns: number; rows: number }> {
  const seen = new Set<string>()
  const result: Array<DesktopGridItem & { columns: number; rows: number }> = []
  for (const item of items) {
    if (!item.key || seen.has(item.key)) continue
    seen.add(item.key)
    result.push(normalizeItem(item))
  }
  return result
}

function clampSlot(
  slot: DesktopIconGridSlot,
  item: DesktopGridItem & { columns: number; rows: number },
  grid: DesktopIconGrid,
): DesktopIconGridSlot {
  return {
    column: Math.min(Math.max(0, Math.round(slot.column)), Math.max(0, grid.columns - item.columns)),
    row: Math.min(Math.max(0, Math.round(slot.row)), Math.max(0, grid.maxRow - item.rows + 1)),
  }
}

function slotForPosition(
  position: DesktopIconPosition,
  item: DesktopGridItem & { columns: number; rows: number },
  grid: DesktopIconGrid,
): DesktopIconGridSlot {
  return clampSlot(
    desktopIconGridSlotForPosition(position, grid.bounds, grid.metrics),
    item,
    grid,
  )
}

function positionForSlot(slot: DesktopIconGridSlot, grid: DesktopIconGrid): DesktopIconPosition {
  return desktopIconPositionForGridSlot(slot, grid.bounds, grid.metrics)
}

function footprint(item: DesktopGridItem & { columns: number; rows: number }, grid: DesktopIconGrid): { width: number; height: number } {
  return {
    width: item.columns * grid.metrics.width + (item.columns - 1) * grid.metrics.columnGap,
    height: item.rows * grid.metrics.height + (item.rows - 1) * grid.metrics.rowGap,
  }
}

function rectForSlot(
  item: DesktopGridItem & { columns: number; rows: number },
  slot: DesktopIconGridSlot,
  grid: DesktopIconGrid,
): DesktopGridRect {
  const position = positionForSlot(slot, grid)
  const pixels = desktopIconPositionToPixels(position, grid.bounds, grid.metrics)
  const size = footprint(item, grid)
  return { ...pixels, ...size }
}

function overlaps(left: DesktopGridRect, right: DesktopGridRect): boolean {
  return left.left < right.left + right.width
    && right.left < left.left + left.width
    && left.top < right.top + right.height
    && right.top < left.top + left.height
}

function itemMap(items: readonly DesktopGridItem[]): Map<string, DesktopGridItem & { columns: number; rows: number }> {
  return new Map(uniqueItems(items).map((item) => [item.key, item]))
}

function occupiedRects(
  placements: readonly DesktopGridPlacement[],
  items: Map<string, DesktopGridItem & { columns: number; rows: number }>,
  grid: DesktopIconGrid,
  excludedKeys = new Set<string>(),
): DesktopGridRect[] {
  return placements.flatMap((placement) => {
    if (excludedKeys.has(placement.key)) return []
    const item = items.get(placement.key)
    if (!item) return []
    return [rectForSlot(item, slotForPosition(placement.position, item, grid), grid)]
  })
}

function canPlace(
  item: DesktopGridItem & { columns: number; rows: number },
  slot: DesktopIconGridSlot,
  grid: DesktopIconGrid,
  occupied: readonly DesktopGridRect[],
): boolean {
  const safe = clampSlot(slot, item, grid)
  if (safe.column !== Math.round(slot.column) || safe.row !== Math.round(slot.row)) return false
  const rect = rectForSlot(item, safe, grid)
  return !occupied.some((candidate) => overlaps(rect, candidate))
}

function firstFreeSlot(
  item: DesktopGridItem & { columns: number; rows: number },
  grid: DesktopIconGrid,
  occupied: readonly DesktopGridRect[],
  requested?: DesktopIconGridSlot,
): DesktopIconGridSlot | undefined {
  // Containers may extend below one viewport; icons/widgets retain their existing paging.
  if (item.key.startsWith('group:')) {
    const lastOccupiedRow = Math.ceil(Math.max(0, ...occupied.map(rect => rect.top + rect.height)) / grid.stepY)
    const lastRow = Math.min(grid.maxRow - item.rows + 1, Math.max(lastOccupiedRow + 1, requested?.row || 0))
    let best: DesktopIconGridSlot | undefined
    let distance = Infinity
    for (let row = 0; row <= lastRow; row += 1) {
      for (let column = 0; column <= grid.columns - item.columns; column += 1) {
        const candidate = { column, row }
        const score = requested ? Math.abs(column - requested.column) + Math.abs(row - requested.row) : row * grid.columns + column
        if (score < distance && canPlace(item, candidate, grid, occupied)) { best = candidate; distance = score }
      }
    }
    return best
  }
  const candidates: DesktopIconGridSlot[] = []
  const pageCount = Math.ceil(MAX_DESKTOP_ICON_POSITIONS / grid.pageCapacity)
  for (let page = 0; page < pageCount; page += 1) {
    for (let column = 0; column <= grid.columns - item.columns; column += 1) {
      for (let row = 0; row <= grid.rows - item.rows; row += 1) {
        candidates.push({ column, row: page * grid.rows + row })
      }
    }
  }
  if (requested) {
    candidates.sort((left, right) => (
      Math.abs(left.column - requested.column) + Math.abs(left.row - requested.row)
      - Math.abs(right.column - requested.column) - Math.abs(right.row - requested.row)
      || left.column - right.column
      || left.row - right.row
    ))
  }
  return candidates.find((candidate) => canPlace(item, candidate, grid, occupied))
}

function placementAt(
  item: DesktopGridItem & { columns: number; rows: number },
  slot: DesktopIconGridSlot,
  grid: DesktopIconGrid,
): DesktopGridPlacement {
  return { key: item.key, position: positionForSlot(slot, grid), columns: item.columns, rows: item.rows }
}

function contentHeight(placements: readonly DesktopGridPlacement[], items: Map<string, DesktopGridItem & { columns: number; rows: number }>, grid: DesktopIconGrid): number {
  return Math.max(
    grid.bounds.height,
    ...placements.map((placement) => {
      const item = items.get(placement.key)
      if (!item) return 0
      return rectForSlot(item, slotForPosition(placement.position, item, grid), grid).top + footprint(item, grid).height
    }),
  )
}

/**
 * Derive a mixed layout while preserving saved anchors and avoiding widget
 * rectangles. Missing items are allocated to the first available column-major
 * slot; widgets may provide a preferred default slot.
 */
export function deriveDesktopGridLayout(
  items: readonly DesktopGridItem[],
  savedPlacements: readonly Pick<DesktopGridPlacement, 'key' | 'position'>[],
  bounds: DesktopIconBounds,
  compact: boolean,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopGridArrangement {
  const normalizedItems = uniqueItems(items)
  const grid = desktopIconGrid(bounds, metrics)
  // Containers must not disappear when new dynamic icons exceed the position
  // budget. Reserve their slots while keeping the existing placement order.
  const groupKeys = new Set(normalizedItems.filter(item => item.key.startsWith('group:')).map(item => item.key))
  let iconBudget = Math.max(0, MAX_DESKTOP_ICON_POSITIONS - groupKeys.size)
  const supportedItems = normalizedItems.filter(item => groupKeys.has(item.key) || iconBudget-- > 0)
  const supportedKeys = new Set(supportedItems.map(item => item.key))
  const byKey = new Map(supportedItems.map((item) => [item.key, item]))
  const savedByKey = new Map(savedPlacements.map((placement) => [placement.key, placement]))
  const placements: DesktopGridPlacement[] = []
  const pending: Array<DesktopGridItem & { columns: number; rows: number }> = []
  const occupied: DesktopGridRect[] = []

  for (const item of supportedItems) {
    const saved = savedByKey.get(item.key)
    if (!saved) {
      pending.push(item)
      continue
    }
    const slot = slotForPosition(saved.position, item, grid)
    if (!canPlace(item, slot, grid, occupied)) {
      pending.push(item)
      continue
    }
    const placement = placementAt(item, slot, grid)
    placements.push(placement)
    occupied.push(rectForSlot(item, slot, grid))
  }

  const unplaced: typeof pending = []
  for (const item of pending) {
    const defaultSlot = !compact && item.defaultSlot
      ? clampSlot(item.defaultSlot, item, grid)
      : undefined
    const slot = (defaultSlot && canPlace(item, defaultSlot, grid, occupied))
      ? defaultSlot
      : firstFreeSlot(item, grid, occupied, item.key.startsWith('group:') && savedByKey.has(item.key)
        ? slotForPosition(savedByKey.get(item.key)!.position, item, grid) : undefined)
    if (!slot) {
      unplaced.push(item)
      continue
    }
    const placement = placementAt(item, slot, grid)
    placements.push(placement)
    occupied.push(rectForSlot(item, slot, grid))
  }

  const order = new Map(supportedItems.map((item, index) => [item.key, index]))
  placements.sort((left, right) => (order.get(left.key) ?? 0) - (order.get(right.key) ?? 0))
  return {
    placements,
    overflowKeys: [
      ...unplaced.map((item) => item.key),
      ...normalizedItems.filter(item => !supportedKeys.has(item.key)).map((item) => item.key),
    ],
    grid,
    contentHeight: contentHeight(placements, byKey, grid),
  }
}

/** Resolve a mixed placement to the visible pixel rectangle. */
export function desktopGridPlacementRect(
  placement: DesktopGridPlacement,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopGridRect {
  const grid = desktopIconGrid(bounds, metrics)
  const item = normalizeItem(placement)
  return rectForSlot(item, slotForPosition(placement.position, item, grid), grid)
}

/** Move one item to a snapped slot, exchanging only equal-size single cells. */
export function dropDesktopGridItem(
  placements: readonly DesktopGridPlacement[],
  items: readonly DesktopGridItem[],
  movingKey: string,
  destination: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopGridPlacement[] {
  const safeItems = itemMap(items)
  const grid = desktopIconGrid(bounds, metrics)
  const movingItem = safeItems.get(movingKey)
  const moving = placements.find((placement) => placement.key === movingKey)
  if (!movingItem || !moving) return [...placements]

  const targetSlot = slotForPosition(destination, movingItem, grid)
  const targetRect = rectForSlot(movingItem, targetSlot, grid)
  const otherPlacements = placements.filter((placement) => placement.key !== movingKey)
  const occupied = occupiedRects(otherPlacements, safeItems, grid)
  const occupant = otherPlacements.find((placement) => {
    const item = safeItems.get(placement.key)
    if (!item) return false
    return overlaps(targetRect, rectForSlot(item, slotForPosition(placement.position, item, grid), grid))
  })

  if (!occupant || (movingItem.columns === 1 && movingItem.rows === 1 && occupant.columns === 1 && occupant.rows === 1)) {
    const originItem = safeItems.get(moving.key)!
    const originSlot = slotForPosition(moving.position, originItem, grid)
    return placements.map((placement) => {
      if (placement.key === movingKey) return placementAt(movingItem, targetSlot, grid)
      if (placement.key === occupant?.key) {
        const occupantItem = safeItems.get(placement.key)!
        return placementAt(occupantItem, originSlot, grid)
      }
      return placement
    })
  }

  const freeSlot = firstFreeSlot(movingItem, grid, occupied, targetSlot)
  if (!freeSlot) return [...placements]
  return placements.map((placement) => placement.key === movingKey
    ? placementAt(movingItem, freeSlot, grid)
    : placement)
}

/** Move a selected group as one translated shape while respecting widgets. */
export function moveDesktopGridItemGroup(
  placements: readonly DesktopGridPlacement[],
  items: readonly DesktopGridItem[],
  movingKeys: readonly string[],
  anchorKey: string,
  destination: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopGridPlacement[] {
  const safeItems = itemMap(items)
  const grid = desktopIconGrid(bounds, metrics)
  const movingSet = new Set(movingKeys)
  const moving = placements.filter((placement) => movingSet.has(placement.key) && safeItems.has(placement.key))
  const anchor = moving.find((placement) => placement.key === anchorKey)
  if (!anchor || moving.length < 2) return [...placements]

  const anchorItem = safeItems.get(anchorKey)!
  const anchorSlot = slotForPosition(anchor.position, anchorItem, grid)
  const sourceSlots = new Map(moving.map((placement) => [
    placement.key,
    slotForPosition(placement.position, safeItems.get(placement.key)!, grid),
  ]))
  const occupied = occupiedRects(placements, safeItems, grid, movingSet)
  const targetSlot = slotForPosition(destination, anchorItem, grid)
  const desiredDeltaX = targetSlot.column - anchorSlot.column
  const desiredDeltaY = targetSlot.row - anchorSlot.row
  const fits = (deltaX: number, deltaY: number): boolean => moving.every((placement) => {
    const item = safeItems.get(placement.key)!
    const source = sourceSlots.get(placement.key)!
    const next = { column: source.column + deltaX, row: source.row + deltaY }
    return canPlace(item, next, grid, occupied)
  })

  const sourceColumns = [...sourceSlots.values()].map((slot) => slot.column)
  const sourceRows = [...sourceSlots.values()].map((slot) => slot.row)
  const minimumDeltaX = -Math.max(...sourceColumns)
  const maximumDeltaX = grid.columns - 1 - Math.min(...sourceColumns)
  const minimumDeltaY = -Math.max(...sourceRows)
  const maximumDeltaY = grid.maxRow - Math.min(...sourceRows)
  const candidates: Array<{ deltaX: number; deltaY: number; distance: number }> = []
  for (let deltaY = minimumDeltaY; deltaY <= maximumDeltaY; deltaY += 1) {
    for (let deltaX = minimumDeltaX; deltaX <= maximumDeltaX; deltaX += 1) {
      candidates.push({
        deltaX,
        deltaY,
        distance: Math.abs(deltaX - desiredDeltaX) + Math.abs(deltaY - desiredDeltaY),
      })
    }
  }
  candidates.sort((left, right) => left.distance - right.distance || left.deltaY - right.deltaY || left.deltaX - right.deltaX)
  const translation = candidates.find(({ deltaX, deltaY }) => fits(deltaX, deltaY))
  if (!translation) return [...placements]

  return placements.map((placement) => {
    const source = sourceSlots.get(placement.key)
    const item = safeItems.get(placement.key)
    if (!source || !item) return placement
    return placementAt(item, {
      column: source.column + translation.deltaX,
      row: source.row + translation.deltaY,
    }, grid)
  })
}

export function moveDesktopGridItemByKeyboard(
  placements: readonly DesktopGridPlacement[],
  items: readonly DesktopGridItem[],
  movingKey: string,
  direction: 'left' | 'right' | 'up' | 'down',
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopGridPlacement[] {
  const item = itemMap(items).get(movingKey)
  const moving = placements.find((placement) => placement.key === movingKey)
  if (!item || !moving) return [...placements]
  const grid = desktopIconGrid(bounds, metrics)
  const slot = slotForPosition(moving.position, item, grid)
  const next = { ...slot }
  if (direction === 'left') next.column -= 1
  else if (direction === 'right') next.column += 1
  else if (direction === 'up') next.row -= 1
  else next.row += 1
  return dropDesktopGridItem(
    placements,
    items,
    movingKey,
    positionForSlot(clampSlot(next, item, grid), grid),
    bounds,
    metrics,
  )
}

export function desktopGridPositionForSlot(
  slot: DesktopIconGridSlot,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPosition {
  return desktopIconPositionForGridSlot(slot, bounds, metrics)
}

export function desktopGridSlotForPosition(
  position: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconGridSlot {
  return desktopIconGridSlotForPosition(position, bounds, metrics)
}

export { clampDesktopIconPosition }
