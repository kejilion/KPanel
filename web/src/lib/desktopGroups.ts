import type { DesktopGroup } from '@/types/api'
import type { DesktopGridItem, DesktopGridPlacement } from '@/lib/desktopGridLayout'
import { desktopGridPlacementRect } from '@/lib/desktopGridLayout'
import { desktopIconGrid, desktopIconPixelsToPosition, type DesktopIconBounds } from '@/lib/desktopIconLayout'

export const MAX_DESKTOP_GROUPS = 32
export const MAX_GROUP_CELLS = 512
export const GROUP_HEADER_HEIGHT = 48
export const GROUP_PADDING = 8
export const DESKTOP_GROUP_COLUMNS = 4
export const GROUP_DWELL_MS = 450
export const groupKey = (id: string) => 'group:' + id
type ReadonlyGroup = Omit<DesktopGroup, 'members'> & { readonly members: readonly string[] }

/** Fill missing legacy assignments; never compact explicit empty cells. */
export function desktopGroupSlots(group: ReadonlyGroup): Record<string, number> {
  const slots: Record<string, number> = {}
  const used = new Set<number>()
  for (const key of group.members) {
    const slot = group.slots?.[key]
    if (slot !== undefined && Number.isInteger(slot) && slot >= 0 && slot < MAX_GROUP_CELLS && !used.has(slot)) {
      slots[key] = slot; used.add(slot)
    }
  }
  let cursor = 0
  for (const key of group.members) {
    if (slots[key] !== undefined) continue
    while (used.has(cursor)) cursor++
    slots[key] = cursor; used.add(cursor)
  }
  return slots
}

export const cloneDesktopGroups = (groups: readonly ReadonlyGroup[]): DesktopGroup[] => groups.map(group => ({
  ...group, members: [...group.members], slots: desktopGroupSlots(group),
}))

/** Legacy column preferences become four-column rows without deleting deliberate holes. */
export const normalizeDesktopGroupColumns = (groups: readonly ReadonlyGroup[]): DesktopGroup[] =>
  cloneDesktopGroups(groups).map(group => ({ ...group, columns: DESKTOP_GROUP_COLUMNS, rows: 0 }))

/** Exact cell placement: a single internal move swaps; cross-group inserts preserve gaps. */
export function placeGroupMembers(groups: readonly DesktopGroup[], keys: readonly string[], targetId?: string, targetSlot?: number): DesktopGroup[] {
  if (targetId && !groups.some(group => group.id === targetId)) return cloneDesktopGroups(groups)
  const moving = [...new Set(keys)]
  // Only retire groups emptied by this move, not intentionally created empty groups.
  const populated = new Set(groups.filter(group => group.members.length).map(group => group.id))
  const keepGroup = (group: DesktopGroup) => group.members.length > 0 || !populated.has(group.id) || group.id === targetId
  const source = groups.find(group => group.id === targetId)
  const origin = source && moving.length === 1 ? desktopGroupSlots(source)[moving[0]!] : undefined
  const result = cloneDesktopGroups(groups).map(group => {
    group.members = group.members.filter(key => !moving.includes(key))
    for (const key of moving) delete group.slots![key]
    return group
  })
  const target = result.find(group => group.id === targetId)
  if (!target) return result.filter(keepGroup)
  if (target.members.length + moving.length > MAX_GROUP_CELLS) return cloneDesktopGroups(groups)
  const slots = target.slots!
  const free = (from = 0) => {
    const occupied = new Set(Object.values(slots))
    for (let index = from; index < MAX_GROUP_CELLS; index++) if (!occupied.has(index)) return index
    for (let index = 0; index < from; index++) if (!occupied.has(index)) return index
    return -1
  }
  const start = targetSlot === undefined ? free() : Math.max(0, Math.min(MAX_GROUP_CELLS - 1, targetSlot))
  const displaced: string[] = []
  for (const [index, key] of moving.entries()) {
    const cell = targetSlot === undefined ? free() : start + index < MAX_GROUP_CELLS ? start + index : free()
    const occupant = Object.keys(slots).find(member => slots[member] === cell)
    if (occupant) { displaced.push(occupant); delete slots[occupant] }
    slots[key] = cell
  }
  for (const key of displaced) {
    slots[key] = origin !== undefined && !Object.values(slots).includes(origin) ? origin : free(Math.min(MAX_GROUP_CELLS, start + moving.length))
  }
  target.members = [...target.members, ...moving].sort((a, b) => slots[a]! - slots[b]!)
  return result.filter(keepGroup)
}

export function moveGroupMembers(groups: readonly DesktopGroup[], keys: readonly string[], targetId?: string, beforeKey?: string): DesktopGroup[] {
  const target = groups.find(group => group.id === targetId)
  return placeGroupMembers(groups, keys, targetId, target && beforeKey ? desktopGroupSlots(target)[beforeKey] : undefined)
}

function geometry(group: DesktopGroup, bounds: DesktopIconBounds, span?: number) {
  const grid = desktopIconGrid(bounds)
  const columns = span ?? Math.min(grid.columns, group.columns + 1)
  const innerColumns = Math.max(1, Math.min(group.columns, columns - 1))
  const rowSegments = Math.ceil(group.columns / innerColumns)
  const slots = desktopGroupSlots(group)
  const logicalRows = Math.max(1, Math.ceil((Math.max(-1, ...Object.values(slots)) + 1) / group.columns))
  const contentHeight = (logicalRows * rowSegments - 1) * grid.stepY + grid.metrics.height
  const width = group.collapsed
    ? innerColumns * grid.stepX - grid.metrics.columnGap + GROUP_PADDING * 2
    : columns * grid.stepX - grid.metrics.columnGap
  const rows = Math.ceil((GROUP_HEADER_HEIGHT + contentHeight + GROUP_PADDING + grid.metrics.rowGap) / grid.stepY)
  const height = group.collapsed ? 56 : rows * grid.stepY - grid.metrics.rowGap
  const top = Math.max(GROUP_HEADER_HEIGHT, (height - contentHeight) / 2)
  return { grid, columns, innerColumns, rowSegments, logicalRows, slots, width, height, top }
}

export function desktopGroupItem(group: DesktopGroup, _visibleKeys: ReadonlySet<string>, bounds: DesktopIconBounds): DesktopGridItem {
  const { grid, columns, width, height } = geometry(group, bounds)
  return { key: groupKey(group.id), columns, rows: Math.ceil((height + grid.metrics.rowGap) / grid.stepY),
    ...(group.collapsed ? { pixelSize: { width, height } } : {}) }
}

/** Paint, hit-test and collision use the same tight content bounds. */
export function desktopGroupRect(group: DesktopGroup, placement: DesktopGridPlacement, bounds: DesktopIconBounds) {
  const rect = desktopGridPlacementRect(placement, bounds)
  const { width, height } = geometry(group, bounds, placement.columns)
  return { ...rect, width: Math.min(rect.width, width), height }
}

export function desktopGroupCells(group: DesktopGroup, placement: DesktopGridPlacement, bounds: DesktopIconBounds) {
  const { grid, innerColumns, rowSegments, logicalRows, slots, top } = geometry(group, bounds, placement.columns)
  const rect = desktopGroupRect(group, placement, bounds)
  const padding = Math.max(0, (rect.width - innerColumns * grid.stepX + grid.metrics.columnGap) / 2)
  const byCell = new Map(Object.entries(slots).map(([key, cell]) => [cell, key]))
  return Array.from({ length: Math.min(MAX_GROUP_CELLS, logicalRows * group.columns) }, (_, index) => ({
    index, key: byCell.get(index),
    left: padding + (index % group.columns % innerColumns) * grid.stepX,
    top: top + (Math.floor(index / group.columns) * rowSegments + Math.floor(index % group.columns / innerColumns)) * grid.stepY,
    width: grid.metrics.width, height: grid.metrics.height,
  }))
}

export function desktopGroupCellAtPoint(group: DesktopGroup, placement: DesktopGridPlacement, bounds: DesktopIconBounds, x: number, y: number): number | undefined {
  return desktopGroupCells(group, placement, bounds).find(cell =>
    x >= cell.left - 3 && x <= cell.left + cell.width + 3 && y >= cell.top - 3 && y <= cell.top + cell.height + 3,
  )?.index
}

export function desktopGroupMembers(group: DesktopGroup, placement: DesktopGridPlacement, visibleKeys: ReadonlySet<string>, bounds: DesktopIconBounds): DesktopGridPlacement[] {
  if (group.collapsed) return []
  const rect = desktopGridPlacementRect(placement, bounds)
  return desktopGroupCells(group, placement, bounds).flatMap(cell => cell.key && visibleKeys.has(cell.key) ? [{
    key: cell.key, columns: 1, rows: 1,
    position: desktopIconPixelsToPosition({ left: rect.left + cell.left, top: rect.top + cell.top }, bounds),
  }] : [])
}
