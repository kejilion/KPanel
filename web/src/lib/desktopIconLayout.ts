/**
 * Pure desktop-icon layout helpers.
 *
 * Persisted positions use normalized coordinates so a wide-screen layout can
 * survive viewport changes. Pixel coordinates are derived from the current
 * icon work-area bounds. Compact layouts are intentionally derived from icon
 * order and never mutate the saved wide-screen placements.
 */

export interface DesktopIconBounds {
  width: number
  height: number
}

export interface DesktopIconMetrics {
  width: number
  height: number
  columnGap: number
  rowGap: number
}

export interface DesktopIconPosition {
  /** Horizontal position within the available travel, from 0 to 1. */
  x: number
  /** Paged vertical position from 0 to 512; 0..1 is the first view and larger values continue below it. */
  y: number
}

export interface DesktopIconPixelPosition {
  left: number
  top: number
}

export interface DesktopIconPlacement {
  key: string
  position: DesktopIconPosition
}

export interface DesktopIconGridSlot {
  column: number
  row: number
}

export interface DesktopIconGrid {
  bounds: DesktopIconBounds
  metrics: DesktopIconMetrics
  stepX: number
  stepY: number
  maxLeft: number
  maxTop: number
  /** Pixel travel represented by one normalized vertical page. */
  verticalUnit: number
  columns: number
  /** Rows that fit in the visible work area. */
  rows: number
  /** Number of slots in one visible page. */
  pageCapacity: number
  /** Maximum number of positions accepted by the workspace API. */
  capacity: number
  /** Largest global row whose normalized y does not exceed the API limit. */
  maxRow: number
}

export interface DesktopIconArrangement {
  placements: DesktopIconPlacement[]
  /** Keys beyond the workspace API's persisted-position limit. */
  overflowKeys: string[]
  grid: DesktopIconGrid
  /** Minimum scroll-surface height required to reveal every placement. */
  contentHeight: number
}

export type DesktopIconDirection = 'left' | 'right' | 'up' | 'down'

export const DEFAULT_DESKTOP_ICON_METRICS: Readonly<DesktopIconMetrics> = Object.freeze({
  width: 90,
  height: 96,
  columnGap: 5,
  rowGap: 4,
})

/** Mirrors internal/desktopworkspace.MaxPositions. */
export const MAX_DESKTOP_ICON_POSITIONS = 512
/** Mirrors the workspace API's inclusive maximum paged y coordinate. */
export const MAX_DESKTOP_ICON_Y = 512

const MAX_DESKTOP_ICON_DIMENSION = 1_000_000

function finiteNonNegative(value: number): number {
  return Number.isFinite(value) ? Math.min(MAX_DESKTOP_ICON_DIMENSION, Math.max(0, value)) : 0
}

function finitePositive(value: number, fallback: number): number {
  return Number.isFinite(value) && value > 0
    ? Math.min(MAX_DESKTOP_ICON_DIMENSION, Math.max(1, value))
    : fallback
}

function clamp(value: number, minimum: number, maximum: number): number {
  const finite = Number.isFinite(value) ? value : minimum
  return Math.min(Math.max(finite, minimum), maximum)
}

function normalizeBounds(bounds: DesktopIconBounds): DesktopIconBounds {
  return {
    width: finiteNonNegative(bounds.width),
    height: finiteNonNegative(bounds.height),
  }
}

function normalizeMetrics(metrics: DesktopIconMetrics): DesktopIconMetrics {
  return {
    width: finitePositive(metrics.width, DEFAULT_DESKTOP_ICON_METRICS.width),
    height: finitePositive(metrics.height, DEFAULT_DESKTOP_ICON_METRICS.height),
    columnGap: finiteNonNegative(metrics.columnGap),
    rowGap: finiteNonNegative(metrics.rowGap),
  }
}

function uniqueKeys(keys: readonly string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const key of keys) {
    if (!key || seen.has(key)) continue
    seen.add(key)
    result.push(key)
  }
  return result
}

/** Clamp an untrusted persisted position to finite normalized coordinates. */
export function clampDesktopIconPosition(position: DesktopIconPosition): DesktopIconPosition {
  return {
    x: clamp(position.x, 0, 1),
    y: clamp(position.y, 0, MAX_DESKTOP_ICON_Y),
  }
}

/** Describe the finite invisible grid inside a desktop icon work area. */
export function desktopIconGrid(
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconGrid {
  const safeBounds = normalizeBounds(bounds)
  const safeMetrics = normalizeMetrics(metrics)
  const stepX = safeMetrics.width + safeMetrics.columnGap
  const stepY = safeMetrics.height + safeMetrics.rowGap
  const maxLeft = Math.max(0, safeBounds.width - safeMetrics.width)
  const maxTop = Math.max(0, safeBounds.height - safeMetrics.height)
  const verticalUnit = Math.max(maxTop, stepY)
  const columns = Math.max(1, Math.floor(maxLeft / stepX) + 1)
  const rows = Math.max(1, Math.floor(maxTop / stepY) + 1)
  const pageCapacity = columns * rows
  const maxRow = Math.floor((MAX_DESKTOP_ICON_Y * verticalUnit) / stepY)

  return {
    bounds: safeBounds,
    metrics: safeMetrics,
    stepX,
    stepY,
    maxLeft,
    maxTop,
    verticalUnit,
    columns,
    rows,
    pageCapacity,
    capacity: MAX_DESKTOP_ICON_POSITIONS,
    maxRow,
  }
}

/** Convert normalized persisted coordinates into bounded CSS pixel offsets. */
export function desktopIconPositionToPixels(
  position: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPixelPosition {
  const grid = desktopIconGrid(bounds, metrics)
  const safe = clampDesktopIconPosition(position)
  return {
    left: safe.x * grid.maxLeft,
    top: safe.y * grid.verticalUnit,
  }
}

/** Convert CSS pixel offsets back into finite normalized persisted coordinates. */
export function desktopIconPixelsToPosition(
  pixels: DesktopIconPixelPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPosition {
  const grid = desktopIconGrid(bounds, metrics)
  return {
    x: grid.maxLeft > 0 ? clamp(pixels.left / grid.maxLeft, 0, 1) : 0,
    y: clamp(pixels.top / grid.verticalUnit, 0, MAX_DESKTOP_ICON_Y),
  }
}

/** Resolve the nearest bounded invisible-grid slot for a normalized position. */
export function desktopIconGridSlotForPosition(
  position: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconGridSlot {
  const grid = desktopIconGrid(bounds, metrics)
  const pixels = desktopIconPositionToPixels(position, grid.bounds, grid.metrics)
  return {
    column: clamp(Math.round(pixels.left / grid.stepX), 0, grid.columns - 1),
    row: clamp(Math.round(pixels.top / grid.stepY), 0, grid.maxRow),
  }
}

/** Convert a possibly invalid slot to a bounded normalized grid position. */
export function desktopIconPositionForGridSlot(
  slot: DesktopIconGridSlot,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPosition {
  const grid = desktopIconGrid(bounds, metrics)
  const column = clamp(Math.round(slot.column), 0, grid.columns - 1)
  const row = clamp(Math.round(slot.row), 0, grid.maxRow)
  return desktopIconPixelsToPosition(
    { left: column * grid.stepX, top: row * grid.stepY },
    grid.bounds,
    grid.metrics,
  )
}

/** Snap a normalized drag destination to the nearest bounded grid position. */
export function snapDesktopIconPosition(
  position: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPosition {
  return desktopIconPositionForGridSlot(
    desktopIconGridSlotForPosition(position, bounds, metrics),
    bounds,
    metrics,
  )
}

function sanitizePlacements(placements: readonly DesktopIconPlacement[]): DesktopIconPlacement[] {
  const seen = new Set<string>()
  const result: DesktopIconPlacement[] = []
  for (const placement of placements) {
    if (!placement.key || seen.has(placement.key)) continue
    seen.add(placement.key)
    result.push({
      key: placement.key,
      position: clampDesktopIconPosition(placement.position),
    })
    if (result.length >= MAX_DESKTOP_ICON_POSITIONS) break
  }
  return result
}

function gridSlotForOrderedIndex(index: number, grid: DesktopIconGrid): DesktopIconGridSlot {
  const safeIndex = Math.max(0, Math.floor(index))
  const page = Math.floor(safeIndex / grid.pageCapacity)
  const withinPage = safeIndex % grid.pageCapacity
  return {
    column: Math.floor(withinPage / grid.rows),
    row: page * grid.rows + (withinPage % grid.rows),
  }
}

function arrangementContentHeight(
  placements: readonly DesktopIconPlacement[],
  grid: DesktopIconGrid,
): number {
  let bottom = 0
  for (const placement of placements) {
    const pixels = desktopIconPositionToPixels(placement.position, grid.bounds, grid.metrics)
    bottom = Math.max(bottom, pixels.top + grid.metrics.height)
  }
  return Math.max(grid.bounds.height, bottom)
}

function sameGridSlot(left: DesktopIconGridSlot, right: DesktopIconGridSlot): boolean {
  return left.column === right.column && left.row === right.row
}

/**
 * Commit a drag destination. The moving icon snaps to the nearest grid slot;
 * when that slot is occupied, the two icons exchange their grid positions.
 */
export function dropDesktopIcon(
  placements: readonly DesktopIconPlacement[],
  movingKey: string,
  destination: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPlacement[] {
  const safe = sanitizePlacements(placements)
  if (!movingKey) return safe

  const moving = safe.find((placement) => placement.key === movingKey)
  const targetSlot = desktopIconGridSlotForPosition(destination, bounds, metrics)
  const targetPosition = desktopIconPositionForGridSlot(targetSlot, bounds, metrics)
  if (!moving) return [...safe, { key: movingKey, position: targetPosition }]

  const originSlot = desktopIconGridSlotForPosition(moving.position, bounds, metrics)
  const originPosition = desktopIconPositionForGridSlot(originSlot, bounds, metrics)
  const occupant = safe.find((placement) => (
    placement.key !== movingKey
    && sameGridSlot(
      desktopIconGridSlotForPosition(placement.position, bounds, metrics),
      targetSlot,
    )
  ))

  return safe.map((placement) => {
    if (placement.key === movingKey) return { ...placement, position: targetPosition }
    if (placement.key === occupant?.key) return { ...placement, position: originPosition }
    return placement
  })
}

/**
 * Move a selected group as one shape. The anchor snaps to the requested slot;
 * if that translation overlaps an unselected icon, the nearest free
 * translation is used. Unselected placements never move.
 */
export function moveDesktopIconGroup(
  placements: readonly DesktopIconPlacement[],
  movingKeys: readonly string[],
  anchorKey: string,
  destination: DesktopIconPosition,
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPlacement[] {
  const safe = sanitizePlacements(placements)
  const movingSet = new Set(uniqueKeys(movingKeys))
  const moving = safe.filter((placement) => movingSet.has(placement.key))
  const anchor = moving.find((placement) => placement.key === anchorKey)
  if (!anchor || moving.length < 2) return safe

  const grid = desktopIconGrid(bounds, metrics)
  const slots = new Map(moving.map((placement) => [
    placement.key,
    desktopIconGridSlotForPosition(placement.position, grid.bounds, grid.metrics),
  ]))
  const anchorSlot = slots.get(anchorKey)
  if (!anchorSlot) return safe

  const movingSlots = [...slots.values()]
  const columns = movingSlots.map((slot) => slot.column)
  const rows = movingSlots.map((slot) => slot.row)
  const minimumDeltaX = -Math.min(...columns)
  const maximumDeltaX = grid.columns - 1 - Math.max(...columns)
  const minimumDeltaY = -Math.min(...rows)
  const maximumDeltaY = grid.maxRow - Math.max(...rows)
  if (minimumDeltaX > maximumDeltaX || minimumDeltaY > maximumDeltaY) return safe

  const destinationSlot = desktopIconGridSlotForPosition(destination, grid.bounds, grid.metrics)
  const desiredDeltaX = clamp(destinationSlot.column - anchorSlot.column, minimumDeltaX, maximumDeltaX)
  const desiredDeltaY = clamp(destinationSlot.row - anchorSlot.row, minimumDeltaY, maximumDeltaY)
  const occupied = new Set(
    safe
      .filter((placement) => !movingSet.has(placement.key))
      .map((placement) => {
        const slot = desktopIconGridSlotForPosition(placement.position, grid.bounds, grid.metrics)
        return `${slot.column}:${slot.row}`
      }),
  )

  const translationFits = (deltaX: number, deltaY: number): boolean => (
    movingSlots.every((slot) => !occupied.has(`${slot.column + deltaX}:${slot.row + deltaY}`))
  )
  const desiredTranslation = translationFits(desiredDeltaX, desiredDeltaY)
    ? { deltaX: desiredDeltaX, deltaY: desiredDeltaY }
    : undefined

  const candidates: Array<{ deltaX: number; deltaY: number; distance: number }> = []
  if (!desiredTranslation) {
    for (let deltaY = minimumDeltaY; deltaY <= maximumDeltaY; deltaY += 1) {
      for (let deltaX = minimumDeltaX; deltaX <= maximumDeltaX; deltaX += 1) {
        candidates.push({
          deltaX,
          deltaY,
          distance: Math.abs(deltaX - desiredDeltaX) + Math.abs(deltaY - desiredDeltaY),
        })
      }
    }
    candidates.sort((left, right) => (
      left.distance - right.distance
      || Math.abs(left.deltaY - desiredDeltaY) - Math.abs(right.deltaY - desiredDeltaY)
      || Math.abs(left.deltaX - desiredDeltaX) - Math.abs(right.deltaX - desiredDeltaX)
      || left.deltaY - right.deltaY
      || left.deltaX - right.deltaX
    ))
  }

  const translation = desiredTranslation
    || candidates.find(({ deltaX, deltaY }) => translationFits(deltaX, deltaY))
  if (!translation) return safe

  return safe.map((placement) => {
    const slot = slots.get(placement.key)
    if (!slot) return placement
    return {
      ...placement,
      position: desktopIconPositionForGridSlot({
        column: slot.column + translation.deltaX,
        row: slot.row + translation.deltaY,
      }, grid.bounds, grid.metrics),
    }
  })
}

/**
 * Arrange keys in KPanel's existing column-major order (top-to-bottom first).
 * Each full visible grid becomes the next vertical page. The workspace API
 * persists at most 512 positions; additional keys are reported explicitly.
 */
export function autoArrangeDesktopIcons(
  keys: readonly string[],
  bounds: DesktopIconBounds,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconArrangement {
  const orderedKeys = uniqueKeys(keys)
  const grid = desktopIconGrid(bounds, metrics)
  const visibleKeys = orderedKeys.slice(0, grid.capacity)
  const placements = visibleKeys.map((key, index) => ({
    key,
    position: desktopIconPositionForGridSlot(
      gridSlotForOrderedIndex(index, grid),
      grid.bounds,
      grid.metrics,
    ),
  }))

  return {
    placements,
    overflowKeys: orderedKeys.slice(grid.capacity),
    grid,
    contentHeight: arrangementContentHeight(placements, grid),
  }
}

/**
 * Derive the current rendered layout without mutating persisted placements.
 * Compact screens always use a temporary column-major arrangement. Wide
 * screens retain saved gaps and allocate missing icons into free grid slots.
 */
export function deriveDesktopIconLayout(
  keys: readonly string[],
  savedWidePlacements: readonly DesktopIconPlacement[],
  bounds: DesktopIconBounds,
  compact: boolean,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconArrangement {
  const orderedKeys = uniqueKeys(keys)
  if (compact) return autoArrangeDesktopIcons(orderedKeys, bounds, metrics)

  const grid = desktopIconGrid(bounds, metrics)
  const supportedKeys = orderedKeys.slice(0, grid.capacity)
  const savedByKey = new Map(sanitizePlacements(savedWidePlacements).map((placement) => [placement.key, placement]))
  const usedSlots = new Set<string>()
  const placements: DesktopIconPlacement[] = []
  const pending: string[] = []

  for (const key of supportedKeys) {
    const saved = savedByKey.get(key)
    if (!saved) {
      pending.push(key)
      continue
    }
    const slot = desktopIconGridSlotForPosition(saved.position, grid.bounds, grid.metrics)
    const slotKey = `${slot.column}:${slot.row}`
    if (usedSlots.has(slotKey)) {
      pending.push(key)
      continue
    }
    usedSlots.add(slotKey)
    placements.push({
      key,
      position: desktopIconPositionForGridSlot(slot, grid.bounds, grid.metrics),
    })
  }

  const freeSlots: DesktopIconGridSlot[] = []
  for (let index = 0; index < grid.capacity && freeSlots.length < pending.length; index += 1) {
    const slot = gridSlotForOrderedIndex(index, grid)
    if (!usedSlots.has(`${slot.column}:${slot.row}`)) freeSlots.push(slot)
  }

  const placeable = pending.slice(0, freeSlots.length)
  placeable.forEach((key, index) => {
    const slot = freeSlots[index]
    if (!slot) return
    placements.push({
      key,
      position: desktopIconPositionForGridSlot(slot, grid.bounds, grid.metrics),
    })
  })

  const order = new Map(supportedKeys.map((key, index) => [key, index]))
  placements.sort((left, right) => (order.get(left.key) ?? 0) - (order.get(right.key) ?? 0))
  return {
    placements,
    overflowKeys: [
      ...pending.slice(freeSlots.length),
      ...orderedKeys.slice(grid.capacity),
    ],
    grid,
    contentHeight: arrangementContentHeight(placements, grid),
  }
}

/** Move a position by one or more bounded grid steps for keyboard control. */
export function nudgeDesktopIconPosition(
  position: DesktopIconPosition,
  direction: DesktopIconDirection,
  bounds: DesktopIconBounds,
  steps = 1,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPosition {
  const grid = desktopIconGrid(bounds, metrics)
  const slot = desktopIconGridSlotForPosition(position, grid.bounds, grid.metrics)
  const safeSteps = Number.isFinite(steps)
    ? Math.min(10_000, Math.max(1, Math.floor(Math.abs(steps))))
    : 1
  const next = { ...slot }

  if (direction === 'left') next.column -= safeSteps
  else if (direction === 'right') next.column += safeSteps
  else if (direction === 'up') next.row -= safeSteps
  else next.row += safeSteps

  return desktopIconPositionForGridSlot(next, grid.bounds, grid.metrics)
}

/** Keyboard movement shares the same occupied-target exchange as pointer drag. */
export function moveDesktopIconByKeyboard(
  placements: readonly DesktopIconPlacement[],
  movingKey: string,
  direction: DesktopIconDirection,
  bounds: DesktopIconBounds,
  steps = 1,
  metrics: DesktopIconMetrics = DEFAULT_DESKTOP_ICON_METRICS,
): DesktopIconPlacement[] {
  const moving = placements.find((placement) => placement.key === movingKey)
  if (!moving) return sanitizePlacements(placements)
  return dropDesktopIcon(
    placements,
    movingKey,
    nudgeDesktopIconPosition(moving.position, direction, bounds, steps, metrics),
    bounds,
    metrics,
  )
}
