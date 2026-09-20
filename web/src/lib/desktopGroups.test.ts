import { describe, expect, it } from 'vitest'
import { desktopGroupItem, desktopGroupMembers, desktopGroupSlots, desktopGroupCells, desktopGroupRect, GROUP_PADDING, normalizeDesktopGroupColumns, placeGroupMembers, groupKey, moveGroupMembers } from './desktopGroups'
import { deriveDesktopGridLayout, desktopGridPlacementRect } from './desktopGridLayout'
import type { DesktopGroup } from '@/types/api'

const a: DesktopGroup = { id: 'a', name: 'A', columns: 3, collapsed: false, members: ['nav:/overview', 'nav:/files'] }
const b: DesktopGroup = { id: 'b', name: 'B', columns: 3, collapsed: false, members: ['nav:/docker'] }
describe('desktop groups', () => {
  it('uses four columns for legacy groups without losing slots, reserved rows or members', () => {
    const legacy = { ...a, rows: 3, slots: { 'nav:/overview': 0, 'nav:/files': 7 } }
    const group = normalizeDesktopGroupColumns([legacy])[0]!
    expect(group).toEqual({ ...legacy, columns: 4 })
    expect(legacy.columns).toBe(3)
    expect(group.members).not.toBe(legacy.members)
    expect(group.slots).not.toBe(legacy.slots)
  })
  it('paints tight edges inside collision reservations without compacting reserved cells', () => {
    for (const bounds of [{ width: 1200, height: 800 }, { width: 290, height: 550 }]) {
      for (const columns of [2, 3, 4]) for (const rows of [1, 2, 3]) {
        const group = { ...a, columns, rows }
        const item = desktopGroupItem(group, new Set(group.members), bounds)
        const placement = deriveDesktopGridLayout([item], [], bounds, true).placements[0]!
        const rect = desktopGroupRect(group, placement, bounds)
        const reservation = desktopGridPlacementRect(placement, bounds)
        const cells = desktopGroupCells(group, placement, bounds)
        expect(cells).toHaveLength(columns * rows)
        expect(rect.width).toBeLessThanOrEqual(reservation.width)
        expect(rect.height).toBeLessThanOrEqual(reservation.height)
        expect(Math.min(...cells.map(cell => cell.left))).toBe(GROUP_PADDING)
        expect(rect.width - Math.max(...cells.map(cell => cell.left + cell.width))).toBe(GROUP_PADDING)
        expect(rect.height - Math.max(...cells.map(cell => cell.top + cell.height))).toBe(GROUP_PADDING)
        expect(desktopGroupRect({ ...group, collapsed: true }, placement, bounds).height).toBe(56)
      }
    }
  })
  it('retires only groups emptied by moving members out, keeping intentional empty groups', () => {
    const empty = { ...a, id: 'empty', members: [] }
    const moved = placeGroupMembers([a, b, empty], a.members)
    expect(moved.map(group => group.id)).toEqual(['b', 'empty'])
    expect(a.members).toHaveLength(2)
    const transferred = placeGroupMembers([a, b, empty], b.members, 'a')
    expect(transferred.map(group => group.id)).toEqual(['a', 'empty'])
    expect(transferred[0]!.members).toContain('nav:/docker')
    expect(placeGroupMembers([b], b.members, 'b', 4)[0]!.slots).toEqual({ 'nav:/docker': 4 })
    expect(placeGroupMembers([a], ['nav:/files'])[0]!.members).toEqual(['nav:/overview'])
  })
  it('moves and reorders members without duplication or mutating the original', () => {
    const moved = moveGroupMembers([a,b], ['nav:/files'], 'b', 'nav:/docker')
    expect(moved[0]!.members).toEqual(['nav:/overview'])
    expect(moved[1]!.members).toEqual(['nav:/files', 'nav:/docker'])
    expect(a.members).toHaveLength(2)
    expect(moveGroupMembers(moved, ['nav:/files'])[1]!.members).toEqual(['nav:/docker'])
    expect(moveGroupMembers([a,b], ['nav:/files'], 'missing')).toMatchObject([a,b])
  })
  it('preserves holes, swaps occupied cells and fills vacancies when adding members', () => {
    const sparse = placeGroupMembers([a], ['nav:/files'], 'a', 4)[0]!
    expect(sparse.slots).toEqual({ 'nav:/overview': 0, 'nav:/files': 4 })
    const added = moveGroupMembers([sparse], ['nav:/terminal', 'nav:/settings'], 'a')[0]!
    expect(added.slots).toEqual({ 'nav:/overview': 0, 'nav:/files': 4, 'nav:/terminal': 1, 'nav:/settings': 2 })
    const swapped = placeGroupMembers([added], ['nav:/overview'], 'a', 4)[0]!
    expect(swapped.slots!['nav:/overview']).toBe(4)
    expect(swapped.slots!['nav:/files']).toBe(0)
    const removed = moveGroupMembers([swapped], ['nav:/terminal'])[0]!
    expect(removed.slots!['nav:/settings']).toBe(2)
    expect(Object.values(removed.slots!)).not.toContain(1)
    expect(a.slots).toBeUndefined()
  })

  it('keeps reserved rows and deliberate gaps through narrow reflow', () => {
    const group = { ...a, rows: 3, columns: 3, slots: { 'nav:/overview': 0, 'nav:/files': 7 } }
    for (const bounds of [{ width: 1200, height: 800 }, { width: 290, height: 550 }]) {
      const item = desktopGroupItem(group, new Set(group.members), bounds)
      const placement = deriveDesktopGridLayout([item], [], bounds, true).placements[0]!
      const cells = desktopGroupCells(group, placement, bounds)
      expect(cells).toHaveLength(9)
      expect(cells.filter(cell => !cell.key)).toHaveLength(7)
      expect(desktopGroupMembers(group, placement, new Set(group.members), bounds)).toHaveLength(2)
      expect(cells[7]!.top).toBeGreaterThan(cells[0]!.top)
      expect(desktopGroupSlots(group)).toEqual(group.slots)
    }
  })

  it('keeps bounded unique slots when a multi-selection lands on the last cell', () => {
    const keys = Array.from({ length: 510 }, (_, i) => `app:item-${i}`)
    const group = { ...a, members: keys }
    const placed = placeGroupMembers([group], ['app:new-1', 'app:new-2'], 'a', 511)[0]!
    expect(placed.members).toHaveLength(512)
    expect(new Set(Object.values(placed.slots!)).size).toBe(512)
    expect(Math.min(...Object.values(placed.slots!))).toBe(0)
    expect(Math.max(...Object.values(placed.slots!))).toBe(511)
  })

  it('allows tall containers, preserves other anchors and never overlaps standalone icons', () => {
    const bounds = {width: 1200, height: 350}
    const group = {...a, members:Array.from({length:64},(_,i)=>`app:item-${i}`)}
    const visible = new Set(group.members)
    const item = desktopGroupItem(group, visible, bounds)
    const layout = deriveDesktopGridLayout([{key:'nav:/terminal'},item], [{key:'nav:/terminal',position:{x:0,y:0}}], bounds, false)
    expect(layout.overflowKeys).toEqual([])
    const placement = layout.placements.find(p=>p.key===groupKey(group.id))!
    const rect = desktopGridPlacementRect(placement,bounds)
    expect(rect.height).toBeGreaterThan(bounds.height)
    expect(desktopGroupMembers(group,placement,visible,bounds)).toHaveLength(64)
    expect(layout.placements[0]!.position).toEqual({x:0,y:0})
  })
  it('fits compact widths and hides collapsed members without changing membership', () => {
    const bounds = {width:290,height:600}
    const group = {...a, collapsed:true}
    const item = desktopGroupItem(group,new Set(a.members),bounds)
    const layout = deriveDesktopGridLayout([item],[],bounds,true)
    const placement = layout.placements[0]!
    expect(desktopGridPlacementRect(placement,bounds).width).toBeLessThanOrEqual(bounds.width)
    expect(desktopGroupMembers(group,placement,new Set(a.members),bounds)).toEqual([])
    expect(group.members).toHaveLength(2)
  })
})
