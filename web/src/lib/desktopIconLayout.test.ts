import { describe, expect, it } from 'vitest'
import {
  autoArrangeDesktopIcons,
  clampDesktopIconPosition,
  deriveDesktopIconLayout,
  desktopIconGrid,
  desktopIconGridSlotForPosition,
  desktopIconPixelsToPosition,
  desktopIconPositionToPixels,
  dropDesktopIcon,
  moveDesktopIconGroup,
  moveDesktopIconByKeyboard,
  nudgeDesktopIconPosition,
  snapDesktopIconPosition,
  type DesktopIconPlacement,
} from './desktopIconLayout'

const bounds = { width: 380, height: 296 }

function placement(key: string, x: number, y: number): DesktopIconPlacement {
  return { key, position: { x, y } }
}

describe('desktop icon layout', () => {
  it('clamps invalid and out-of-range normalized positions to finite values', () => {
    expect(clampDesktopIconPosition({ x: -0.4, y: 1.8 })).toEqual({ x: 0, y: 1.8 })
    expect(clampDesktopIconPosition({ x: 1.4, y: 700 })).toEqual({ x: 1, y: 512 })
    expect(clampDesktopIconPosition({ x: Number.NaN, y: Number.POSITIVE_INFINITY })).toEqual({ x: 0, y: 0 })
  })

  it('round-trips normalized coordinates through bounded pixels', () => {
    const original = { x: 0.25, y: 0.75 }
    const pixels = desktopIconPositionToPixels(original, bounds)
    expect(pixels).toEqual({ left: 72.5, top: 150 })
    expect(desktopIconPixelsToPosition(pixels, bounds)).toEqual(original)
  })

  it('snaps a drag destination to the nearest invisible grid slot', () => {
    const snapped = snapDesktopIconPosition({ x: 0.31, y: 0.58 }, bounds)
    expect(desktopIconPositionToPixels(snapped, bounds)).toEqual({ left: 95, top: 100 })
    expect(desktopIconGridSlotForPosition(snapped, bounds)).toEqual({ column: 1, row: 1 })
  })

  it('exchanges positions when a dropped icon targets an occupied slot', () => {
    const current = [
      placement('alpha', 0, 0),
      placement('beta', 95 / 290, 0),
      placement('gamma', 190 / 290, 0.5),
    ]
    const next = dropDesktopIcon(current, 'alpha', { x: 0.34, y: 0 }, bounds)

    expect(desktopIconGridSlotForPosition(next.find((item) => item.key === 'alpha')!.position, bounds))
      .toEqual({ column: 1, row: 0 })
    expect(desktopIconGridSlotForPosition(next.find((item) => item.key === 'beta')!.position, bounds))
      .toEqual({ column: 0, row: 0 })
    expect(next.find((item) => item.key === 'gamma')).toEqual(current[2])
    expect(current[0]!.position).toEqual({ x: 0, y: 0 })
  })

  it('moves a selected group together without moving unselected icons', () => {
    const current = [
      placement('alpha', 0, 0),
      placement('beta', 0, 0.5),
      placement('fixed', 190 / 290, 0),
    ]
    const next = moveDesktopIconGroup(current, ['alpha', 'beta'], 'alpha', { x: 0.34, y: 0 }, bounds)

    expect(next.map((item) => ({
      key: item.key,
      slot: desktopIconGridSlotForPosition(item.position, bounds),
    }))).toEqual([
      { key: 'alpha', slot: { column: 1, row: 0 } },
      { key: 'beta', slot: { column: 1, row: 1 } },
      { key: 'fixed', slot: { column: 2, row: 0 } },
    ])
    expect(current[0]!.position).toEqual({ x: 0, y: 0 })
  })

  it('places a selected group at the nearest free translation when the target is occupied', () => {
    const current = [
      placement('alpha', 0, 0),
      placement('beta', 0, 0.5),
      placement('fixed', 95 / 290, 0),
    ]
    const next = moveDesktopIconGroup(current, ['alpha', 'beta'], 'alpha', { x: 0.34, y: 0 }, bounds)
    const slots = Object.fromEntries(next.map((item) => [
      item.key,
      desktopIconGridSlotForPosition(item.position, bounds),
    ]))

    expect(slots.fixed).toEqual({ column: 1, row: 0 })
    expect(slots.alpha).toBeDefined()
    expect(slots.beta).toBeDefined()
    expect(slots.alpha).not.toEqual(slots.fixed)
    expect(slots.beta!.column).toBe(slots.alpha!.column)
    expect(slots.beta!.row - slots.alpha!.row).toBe(1)
  })

  it('auto-arranges top-to-bottom before moving to the next column', () => {
    const arranged = autoArrangeDesktopIcons(['one', 'two', 'three', 'four', 'five'], bounds)
    expect(arranged.grid).toMatchObject({ columns: 4, rows: 3, pageCapacity: 12, capacity: 512 })
    expect(arranged.overflowKeys).toEqual([])
    expect(arranged.placements.map((item) => ({
      key: item.key,
      slot: desktopIconGridSlotForPosition(item.position, bounds),
    }))).toEqual([
      { key: 'one', slot: { column: 0, row: 0 } },
      { key: 'two', slot: { column: 0, row: 1 } },
      { key: 'three', slot: { column: 0, row: 2 } },
      { key: 'four', slot: { column: 1, row: 0 } },
      { key: 'five', slot: { column: 1, row: 1 } },
    ])
  })

  it('extends a one-slot work area vertically instead of overlapping icons', () => {
    const arranged = autoArrangeDesktopIcons(['one', 'two', 'three'], { width: 10, height: 20 })
    expect(arranged.grid).toMatchObject({ columns: 1, rows: 1, pageCapacity: 1, capacity: 512, maxLeft: 0, maxTop: 0 })
    expect(arranged.placements).toEqual([
      placement('one', 0, 0),
      placement('two', 0, 1),
      placement('three', 0, 2),
    ])
    expect(arranged.overflowKeys).toEqual([])
    expect(arranged.contentHeight).toBe(296)
  })

  it('handles empty and non-finite bounds without producing invalid numbers', () => {
    const grid = desktopIconGrid({ width: Number.NaN, height: Number.NEGATIVE_INFINITY })
    expect(grid).toMatchObject({ columns: 1, rows: 1, pageCapacity: 1, capacity: 512, maxLeft: 0, maxTop: 0 })
    expect(autoArrangeDesktopIcons([], { width: 0, height: 0 }).placements).toEqual([])
    const pixels = desktopIconPositionToPixels({ x: 0.7, y: 0.8 }, { width: 0, height: 0 })
    expect(pixels).toEqual({ left: 0, top: 80 })
    expect(desktopIconPixelsToPosition(pixels, { width: 0, height: 0 })).toEqual({ x: 0, y: 0.8 })
  })

  it('paginates beyond the visible grid with unique slots and y greater than one', () => {
    const keys = Array.from({ length: 13 }, (_, index) => `icon-${index}`)
    const arranged = autoArrangeDesktopIcons(keys, bounds)
    const slots = arranged.placements.map((item) => desktopIconGridSlotForPosition(item.position, bounds))

    expect(arranged.grid.pageCapacity).toBe(12)
    expect(new Set(slots.map((slot) => `${slot.column}:${slot.row}`)).size).toBe(13)
    expect(arranged.placements[12]?.position.y).toBeGreaterThan(1)
    expect(arranged.contentHeight).toBeGreaterThan(bounds.height)
    expect(arranged.overflowKeys).toEqual([])
  })

  it('allocates 512 persisted positions and reports additional keys explicitly', () => {
    const keys = Array.from({ length: 513 }, (_, index) => `icon-${index}`)
    const arranged = autoArrangeDesktopIcons(keys, { width: 90, height: 96 })
    const slots = arranged.placements.map((item) => desktopIconGridSlotForPosition(item.position, { width: 90, height: 96 }))

    expect(arranged.placements).toHaveLength(512)
    expect(new Set(slots.map((slot) => `${slot.column}:${slot.row}`)).size).toBe(512)
    expect(Math.max(...arranged.placements.map((item) => item.position.y))).toBeLessThanOrEqual(512)
    expect(arranged.overflowKeys).toEqual(['icon-512'])
  })

  it('keeps every grid number finite for extreme finite inputs', () => {
    const grid = desktopIconGrid(
      { width: Number.MAX_VALUE, height: Number.MAX_VALUE },
      { width: Number.MIN_VALUE, height: Number.MIN_VALUE, columnGap: Number.MAX_VALUE, rowGap: 0 },
    )
    expect(Object.values(grid).filter((value) => typeof value === 'number').every(Number.isFinite)).toBe(true)
    expect(Object.values(grid.bounds).every(Number.isFinite)).toBe(true)
    expect(Object.values(grid.metrics).every(Number.isFinite)).toBe(true)
  })

  it('derives a compact auto layout without modifying saved wide-screen gaps', () => {
    const saved = [placement('one', 1, 1), placement('two', 0.5, 0)]
    const snapshot = structuredClone(saved)
    const compact = deriveDesktopIconLayout(['one', 'two'], saved, bounds, true)

    expect(compact.placements.map((item) => desktopIconGridSlotForPosition(item.position, bounds)))
      .toEqual([{ column: 0, row: 0 }, { column: 0, row: 1 }])
    expect(saved).toEqual(snapshot)

    const wide = deriveDesktopIconLayout(['one', 'two'], saved, bounds, false)
    expect(wide.placements.map((item) => desktopIconGridSlotForPosition(item.position, bounds)))
      .toEqual([{ column: 3, row: 2 }, { column: 2, row: 0 }])
    expect(saved).toEqual(snapshot)
  })

  it('allocates new wide-screen icons into the first free column-major slot', () => {
    const wide = deriveDesktopIconLayout(
      ['saved', 'new'],
      [placement('saved', 0, 0)],
      bounds,
      false,
    )
    expect(wide.placements.map((item) => ({
      key: item.key,
      slot: desktopIconGridSlotForPosition(item.position, bounds),
    }))).toEqual([
      { key: 'saved', slot: { column: 0, row: 0 } },
      { key: 'new', slot: { column: 0, row: 1 } },
    ])
  })

  it('derives every supported wide-screen icon across vertical pages', () => {
    const keys = Array.from({ length: 13 }, (_, index) => `icon-${index}`)
    const wide = deriveDesktopIconLayout(keys, [], bounds, false)
    const slots = wide.placements.map((item) => desktopIconGridSlotForPosition(item.position, bounds))

    expect(wide.placements).toHaveLength(13)
    expect(new Set(slots.map((slot) => `${slot.column}:${slot.row}`)).size).toBe(13)
    expect(wide.placements[12]?.position.y).toBeGreaterThan(1)
    expect(wide.overflowKeys).toEqual([])
  })

  it('nudges by bounded keyboard grid steps and exchanges occupied targets', () => {
    const current = [placement('alpha', 0, 0), placement('beta', 95 / 290, 0)]
    const nudged = nudgeDesktopIconPosition(current[0]!.position, 'right', bounds, 50)
    expect(desktopIconGridSlotForPosition(nudged, bounds)).toEqual({ column: 3, row: 0 })

    const exchanged = moveDesktopIconByKeyboard(current, 'alpha', 'right', bounds)
    expect(exchanged.map((item) => ({
      key: item.key,
      slot: desktopIconGridSlotForPosition(item.position, bounds),
    }))).toEqual([
      { key: 'alpha', slot: { column: 1, row: 0 } },
      { key: 'beta', slot: { column: 0, row: 0 } },
    ])
  })
})
