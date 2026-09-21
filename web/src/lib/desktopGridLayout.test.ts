import { describe, expect, it } from 'vitest'
import {
  deriveDesktopGridLayout,
  reflowDesktopGridLayout,
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
  area = bounds,
): void {
  for (let left = 0; left < placements.length; left += 1) {
    const first = desktopGridPlacementRect(placements[left]!, area)
    for (let right = left + 1; right < placements.length; right += 1) {
      const second = desktopGridPlacementRect(placements[right]!, area)
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

describe('viewport reflow', () => {
  const icons = Array.from({ length: 24 }, (_, index) => ({ key: `icon:${index}` }))
  const widgets = [
    { key: 'widget:clock', columns: 4, rows: 2 },
    { key: 'widget:monitor', columns: 4, rows: 3 },
    { key: 'widget:services', columns: 4, rows: 3 },
  ]
  it.each([{ width: 1895, height: 995 }, { width: 1100, height: 700 }])('fills icon holes and stacks widgets at $width x $height', area => {
    const order = [...icons].reverse().map(item => item.key)
    const layout = reflowDesktopGridLayout([...icons, ...widgets], order, area, false)
    expect(layout.placements).toHaveLength(27)
    assertNoOverlap(layout.placements, area)
    const rects = new Map(layout.placements.map(item => [item.key, desktopGridPlacementRect(item, area)]))
    order.slice(0, layout.grid.rows).forEach((key, row) => {
      expect(rects.get(key)?.left).toBeCloseTo(0)
      expect(rects.get(key)?.top).toBeCloseTo(row * 100)
    })
    expect(rects.get('widget:clock')?.top).toBeCloseTo(0)
    expect(rects.get('widget:monitor')?.top).toBeCloseTo(200)
    expect(rects.get('widget:services')?.top).toBeCloseTo(500)
    expect(rects.get('widget:clock')?.left).toBeCloseTo(rects.get('widget:services')!.left)
  })
  it.each([390, 768, 1280])('keeps groups intact and reachable at %spx without mutating source data', width => {
    const area = { width, height: 600 }
    const group: DesktopGroup = { id: 'a', name: 'a', columns: 4, rows: 0, collapsed: false, members: ['a', 'b', 'c'] }
    const source = [desktopGroupItem(group, new Set(group.members), area), ...icons]
    const copy = structuredClone(source)
    const layout = reflowDesktopGridLayout(source, source.map(item => item.key), area, width <= 760)
    expect(layout.placements).toHaveLength(source.length)
    expect(layout.placements.filter(item => item.key === 'group:a')).toHaveLength(1)
    for (const item of layout.placements) {
      const rect = desktopGridPlacementRect(item, area)
      expect(rect.left).toBeGreaterThanOrEqual(0)
      expect(rect.left + rect.width).toBeLessThanOrEqual(width + 0.001)
      expect(rect.top + rect.height).toBeLessThanOrEqual(layout.contentHeight + 0.001)
    }
    assertNoOverlap(layout.placements, area)
    expect(source).toEqual(copy)
    expect(reflowDesktopGridLayout(source, source.map(item => item.key), area, width <= 760)).toEqual(layout)
  })
})

describe('grid-aligned expanded desktop groups', () => {
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
    { key: 'group:b', position: position(380, 0) },
  ], bounds, false).placements
  const rect = (placements: ReturnType<typeof initial>, key = 'group:b') => desktopGridPlacementRect(placements.find(item => item.key === key)!, bounds)

  it('fits four compact columns inside a four-column desktop footprint', () => {
    expect(rect(initial(), 'group:a')).toMatchObject({ width: 375, height: 196 })
    expect(rect(initial())).toMatchObject({ width: 375, height: 296 })
  })

  it.each([355, 380, 404])('snaps horizontal neighbours to the 5px grid gap from %spx and survives reload', left => {
    const moved = dropDesktopGridItem(initial(), groupItems, 'group:b', position(left, 7), bounds)
    expect(rect(moved).left).toBeCloseTo(380)
    expect(rect(moved).top).toBe(0)
    const reloaded = deriveDesktopGridLayout(groupItems, moved.map(({ key, position }) => ({ key, position })), bounds, false)
    expect(rect(reloaded.placements)).toEqual(rect(moved))
    assertNoOverlap(moved)
  })

  it('snaps above/below as well as left/right, including collapsed cards', () => {
    const below = dropDesktopGridItem(initial(), groupItems, 'group:b', position(8, 204), bounds)
    expect(rect(below).left).toBe(0)
    expect(rect(below).top).toBeCloseTo(200)
    const above = dropDesktopGridItem(below, groupItems, 'group:a', position(8, 10), bounds)
    expect(rect(above, 'group:a')).toMatchObject({ left: 0, top: 0 })
    const left = dropDesktopGridItem(initial(), groupItems, 'group:a', position(65, 0), bounds)
    expect(rect(left, 'group:a').left).toBe(0)
    const collapsedItems = [desktopGroupItem({ ...first, collapsed: true }, new Set(), bounds), groupItems[1]!]
    const collapsed = deriveDesktopGridLayout(collapsedItems, initial(), bounds, false).placements
    const underHeader = dropDesktopGridItem(collapsed, collapsedItems, 'group:b', position(0, 60), bounds)
    expect(rect(underHeader).top).toBeCloseTo(100)
    const expanded = deriveDesktopGridLayout(groupItems, underHeader, bounds, false)
    expect(expanded.placements).toHaveLength(2)
    assertNoOverlap(expanded.placements)
  })

  it('quantizes anchors and stops invalid drops without displacing neighbours', () => {
    const free = dropDesktopGridItem(initial(), groupItems, 'group:b', position(463, 93), bounds)
    expect(rect(free).left).toBeCloseTo(475)
    expect(rect(free).top).toBeCloseTo(100)
    const blocked = dropDesktopGridItem(free, groupItems, 'group:b', position(100, 100), bounds)
    expect(blocked).toEqual(free)
    const edge = dropDesktopGridItem(free, groupItems, 'group:b', position(900, 400), bounds)
    expect(rect(edge).left + rect(edge).width).toBeLessThanOrEqual(bounds.width)
    expect(rect(edge).left / 95).toBeCloseTo(Math.round(rect(edge).left / 95))
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

  it('uses the same collision rule for keyboard moves and fits narrow screens', () => {
    const moved = moveDesktopGridItemByKeyboard(initial(), groupItems, 'group:b', 'left', bounds)
    expect(rect(moved).left).toBeCloseTo(380)
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

  it('aligns with loose icons and the clock in both directions without moving neighbours', () => {
    const mixed = [groupItems[0]!, { key: 'icon:left' }, { key: 'icon:right' }, { key: 'widget:clock', columns: 4, rows: 2 }]
    const saved = [
      { key: 'group:a', position: position(101, 6) },
      { key: 'icon:left', position: position(0, 100) },
      { key: 'icon:right', position: position(475, 100) },
      { key: 'widget:clock', position: position(95, 200) },
    ]
    const layout = deriveDesktopGridLayout(mixed, saved, bounds, false).placements
    const groupRect = rect(layout, 'group:a')
    const clock = rect(layout, 'widget:clock')
    expect(groupRect.left).toBe(clock.left)
    expect(groupRect.width).toBe(clock.width)
    expect(groupRect.top + groupRect.height + 4).toBeCloseTo(clock.top)
    expect(groupRect.left - rect(layout, 'icon:left').left - 90).toBeCloseTo(5)
    expect(rect(layout, 'icon:right').left - groupRect.left - groupRect.width).toBeCloseTo(5)
    const movedIcon = dropDesktopGridItem(layout, mixed, 'icon:left', position(0, 0), bounds)
    expect(rect(movedIcon, 'group:a')).toEqual(groupRect)
    const movedGroup = dropDesktopGridItem(movedIcon, mixed, 'group:a', position(96, 2), bounds)
    expect(rect(movedGroup, 'group:a')).toEqual(groupRect)
    assertNoOverlap(movedGroup)
  })
})
