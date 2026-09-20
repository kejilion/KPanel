import { describe, expect, it } from 'vitest'
import {
  deriveDesktopGridLayout,
  desktopGridPlacementRect,
  dropDesktopGridItem,
  moveDesktopGridItemByKeyboard,
} from './desktopGridLayout'
import { desktopIconPixelsToPosition, desktopIconPositionForGridSlot, type DesktopIconBounds } from './desktopIconLayout'
import { desktopGroupItem } from './desktopGroups'
import type { DesktopGroup } from '@/types/api'

const bounds: DesktopIconBounds = { width: 1000, height: 700 }
const items = [
  { key: 'icon:one' },
  { key: 'icon:two' },
  { key: 'widget:clock', columns: 3, rows: 2, defaultSlot: { column: 7, row: 0 } },
  { key: 'widget:monitor', columns: 3, rows: 3, defaultSlot: { column: 7, row: 2 } },
]

function assertNoOverlap(
  placements: ReturnType<typeof deriveDesktopGridLayout>['placements'],
): void {
  for (let left = 0; left < placements.length; left += 1) {
    const first = desktopGridPlacementRect(placements[left]!, bounds)
    for (let right = left + 1; right < placements.length; right += 1) {
      const second = desktopGridPlacementRect(placements[right]!, bounds)
      expect(
        first.left >= second.left + second.width
          || second.left >= first.left + first.width
          || first.top >= second.top + second.height
          || second.top >= first.top + first.height,
      ).toBe(true)
    }
  }
}

describe('desktop mixed grid layout', () => {
  it('places variable-size widgets in the same collision-free grid as icons', () => {
    const layout = deriveDesktopGridLayout(items, [], bounds, false)
    expect(layout.placements).toHaveLength(4)
    expect(layout.placements.find((item) => item.key === 'widget:clock')?.columns).toBe(3)
    expect(desktopGridPlacementRect(layout.placements.find((item) => item.key === 'widget:clock')!, bounds).width)
      .toBeGreaterThan(desktopGridPlacementRect(layout.placements[0]!, bounds).width)
    assertNoOverlap(layout.placements)
  })

  it('keeps a widget out of an occupied icon target instead of overlapping it', () => {
    const layout = deriveDesktopGridLayout(items, [], bounds, false)
    const moved = dropDesktopGridItem(
      layout.placements,
      items,
      'widget:clock',
      layout.placements.find((item) => item.key === 'icon:one')!.position,
      bounds,
    )
    assertNoOverlap(moved)
  })

  it('supports keyboard movement with the same collision rules', () => {
    const layout = deriveDesktopGridLayout(items, [], bounds, false)
    const current = layout.placements.find((item) => item.key === 'icon:one')!
    const moved = moveDesktopGridItemByKeyboard(layout.placements, items, 'icon:one', 'right', bounds)
    const next = moved.find((item) => item.key === 'icon:one')!
    expect(next.position).not.toEqual(current.position)
    assertNoOverlap(moved)
  })

  it('preserves explicit normalized widget positions across derivation', () => {
    const saved = desktopIconPositionForGridSlot({ column: 5, row: 1 }, bounds)
    const layout = deriveDesktopGridLayout(items, [{ key: 'widget:clock', position: saved }], bounds, false)
    const clock = layout.placements.find((item) => item.key === 'widget:clock')!
    expect(clock.position).toEqual(saved)
  })
})

describe('pixel-accurate desktop groups', () => {
  const group = (id: string, count: number, collapsed = false): DesktopGroup => ({
    id, name: id, columns: 4, rows: 0, collapsed,
    members: Array.from({ length: count }, (_, index) => `${id}:${index}`),
  })
  const first = group('a', 8)
  const second = group('b', 9)
  const groupItems = [first, second].map(value => desktopGroupItem(value, new Set(value.members), bounds))
  const position = (left: number, top: number) => desktopIconPixelsToPosition({ left, top }, bounds)
  const initial = () => deriveDesktopGridLayout(groupItems, [
    { key: 'group:a', position: position(0, 0) },
    { key: 'group:b', position: position(475, 0) },
  ], bounds, false).placements
  const rect = (placements: ReturnType<typeof initial>, key = 'group:b') => desktopGridPlacementRect(placements.find(item => item.key === key)!, bounds)

  it('uses the painted footprint, not the old five-cell reservation', () => {
    expect(rect(initial(), 'group:a')).toMatchObject({ width: 391, height: 252 })
    expect(rect(initial())).toMatchObject({ width: 391, height: 352 })
  })

  it.each([380, 403, 424])('snaps horizontal neighbours to 12px from a %spx pointer target and survives reload', left => {
    const moved = dropDesktopGridItem(initial(), groupItems, 'group:b', position(left, 7), bounds)
    expect(rect(moved).left).toBeCloseTo(403)
    expect(rect(moved).top).toBe(0)
    const reloaded = deriveDesktopGridLayout(groupItems, moved.map(({ key, position }) => ({ key, position })), bounds, false)
    expect(rect(reloaded.placements)).toEqual(rect(moved))
    assertNoOverlap(moved)
  })

  it('snaps above/below as well as left/right, including collapsed cards', () => {
    const below = dropDesktopGridItem(initial(), groupItems, 'group:b', position(8, 254), bounds)
    expect(rect(below)).toMatchObject({ left: 0, top: 264 })
    const above = dropDesktopGridItem(below, groupItems, 'group:a', position(8, 10), bounds)
    expect(rect(above, 'group:a')).toMatchObject({ left: 0, top: 0 })
    const left = dropDesktopGridItem(initial(), groupItems, 'group:a', position(65, 0), bounds)
    expect(rect(left, 'group:a').left).toBeCloseTo(72)
    const collapsedItems = [desktopGroupItem({ ...first, collapsed: true }, new Set(), bounds), groupItems[1]!]
    const collapsed = deriveDesktopGridLayout(collapsedItems, initial(), bounds, false).placements
    const underHeader = dropDesktopGridItem(collapsed, collapsedItems, 'group:b', position(0, 60), bounds)
    expect(rect(underHeader).top).toBeCloseTo(68)
    const expanded = deriveDesktopGridLayout(groupItems, underHeader, bounds, false)
    expect(expanded.placements).toHaveLength(2)
    assertNoOverlap(expanded.placements)
  })

  it('keeps free anchors and stops invalid drops without displacing neighbours', () => {
    const free = dropDesktopGridItem(initial(), groupItems, 'group:b', position(463, 93), bounds)
    expect(rect(free)).toMatchObject({ left: 463, top: 93 })
    const blocked = dropDesktopGridItem(free, groupItems, 'group:b', position(100, 100), bounds)
    expect(blocked).toEqual(free)
    const edge = dropDesktopGridItem(free, groupItems, 'group:b', position(900, 400), bounds)
    expect(rect(edge).left + rect(edge).width).toBeCloseTo(bounds.width)
  })

  it('does not snap through standalone icons or widgets', () => {
    const mixed = [...groupItems, { key: 'icon:outside' }, { key: 'widget:outside', columns: 2, rows: 2 }]
    const layout = deriveDesktopGridLayout(mixed, [
      ...initial(), { key: 'icon:outside', position: position(0, 400) },
      { key: 'widget:outside', position: position(475, 500) },
    ], bounds, false)
    for (const destination of [position(0, 400), position(475, 500)]) {
      expect(dropDesktopGridItem(layout.placements, mixed, 'group:b', destination, bounds)).toEqual(layout.placements)
    }
    assertNoOverlap(layout.placements)
  })

  it('uses the same snap for keyboard moves and retains fractional anchors on narrow screens', () => {
    const moved = moveDesktopGridItemByKeyboard(initial(), groupItems, 'group:b', 'left', bounds)
    expect(rect(moved).left).toBeCloseTo(403)
    for (const width of [320, 390, 760, 960]) {
      const narrowBounds = { width, height: 700 }
      const narrowItems = [first, second].map(value => desktopGroupItem(value, new Set(), narrowBounds))
      const narrow = deriveDesktopGridLayout(narrowItems, moved, narrowBounds, true)
      expect(narrow.placements).toHaveLength(2)
      for (const placement of narrow.placements) {
        const r = desktopGridPlacementRect(placement, narrowBounds)
        expect(r.left + r.width).toBeLessThanOrEqual(width + 1e-7)
      }
    }
  })
})
