import type { DesktopGroup } from '@/types/api'
import type { DesktopGridItem, DesktopGridPlacement } from '@/lib/desktopGridLayout'
import { desktopGridPlacementRect } from '@/lib/desktopGridLayout'
import { desktopIconGrid, desktopIconPixelsToPosition, type DesktopIconBounds } from '@/lib/desktopIconLayout'

export const MAX_DESKTOP_GROUPS = 32
export const GROUP_HEADER_HEIGHT = 48
export const groupKey = (id: string) => `group:${id}`
export const cloneDesktopGroups = (groups: readonly (Omit<DesktopGroup, 'members'> & { readonly members: readonly string[] })[]): DesktopGroup[] => groups.map(group => ({ ...group, members: [...group.members] }))

export function moveGroupMembers(groups: readonly DesktopGroup[], keys: readonly string[], targetId?: string, beforeKey?: string): DesktopGroup[] {
  const moving = new Set(keys)
  if (targetId && !groups.some(group => group.id === targetId)) return cloneDesktopGroups(groups)
  return groups.map(group => {
    const members = group.members.filter(key => !moving.has(key))
    if (group.id === targetId) {
      const index = beforeKey ? members.indexOf(beforeKey) : -1
      members.splice(index < 0 ? members.length : index, 0, ...moving)
    }
    return { ...group, members }
  })
}

export function desktopGroupItem(group: DesktopGroup, visibleKeys: ReadonlySet<string>, bounds: DesktopIconBounds): DesktopGridItem {
  const grid = desktopIconGrid(bounds)
  const columns = Math.min(grid.columns, group.columns + 1)
  const innerColumns = Math.max(1, Math.min(group.columns, columns - 1))
  const count = group.members.filter(key => visibleKeys.has(key)).length
  const height = group.collapsed ? 56 : GROUP_HEADER_HEIGHT + Math.max(1, Math.ceil(count / innerColumns)) * grid.stepY + 12
  return { key: groupKey(group.id), columns, rows: Math.ceil((height + grid.metrics.rowGap) / grid.stepY) }
}

export function desktopGroupMembers(group: DesktopGroup, placement: DesktopGridPlacement, visibleKeys: ReadonlySet<string>, bounds: DesktopIconBounds): DesktopGridPlacement[] {
  if (group.collapsed) return []
  const grid = desktopIconGrid(bounds)
  const rect = desktopGridPlacementRect(placement, bounds)
  const columns = Math.max(1, Math.min(group.columns, placement.columns - 1))
  const padding = Math.max(0, (rect.width - columns * grid.stepX + grid.metrics.columnGap) / 2)
  return group.members.filter(key => visibleKeys.has(key)).map((key, index) => ({
    key, columns: 1, rows: 1,
    position: desktopIconPixelsToPosition({
      left: rect.left + padding + (index % columns) * grid.stepX,
      top: rect.top + GROUP_HEADER_HEIGHT + Math.floor(index / columns) * grid.stepY,
    }, bounds),
  }))
}
