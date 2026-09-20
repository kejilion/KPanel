import { describe, expect, it } from 'vitest'
import { desktopGroupItem, desktopGroupMembers, groupKey, moveGroupMembers } from './desktopGroups'
import { deriveDesktopGridLayout, desktopGridPlacementRect } from './desktopGridLayout'
import type { DesktopGroup } from '@/types/api'

const a: DesktopGroup = { id: 'a', name: 'A', columns: 3, collapsed: false, members: ['nav:/overview', 'nav:/files'] }
const b: DesktopGroup = { id: 'b', name: 'B', columns: 3, collapsed: false, members: ['nav:/docker'] }
describe('desktop groups', () => {
  it('moves and reorders members without duplication or mutating the original', () => {
    const moved = moveGroupMembers([a,b], ['nav:/files'], 'b', 'nav:/docker')
    expect(moved[0]!.members).toEqual(['nav:/overview'])
    expect(moved[1]!.members).toEqual(['nav:/files', 'nav:/docker'])
    expect(a.members).toHaveLength(2)
    expect(moveGroupMembers(moved, ['nav:/files'])[1]!.members).toEqual(['nav:/docker'])
    expect(moveGroupMembers([a,b], ['nav:/files'], 'missing')).toEqual([a,b])
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
