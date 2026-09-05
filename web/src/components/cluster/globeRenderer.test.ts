import { describe, expect, it, vi } from 'vitest'
import { GlobeRenderer, groupGlobeHosts, layoutGlobeLabels, projectGlobePoint } from './globeRenderer'
import { landDots, regionCenters } from './globeData'
import type { ClusterHost } from '@/types/api'

function host(id: string, code?: string): ClusterHost {
  return { id, state: 'online', lastSnapshot: { telemetry: { publicNetwork: { countryCode: code } } } } as ClusterHost
}

describe('cluster globe geography', () => {
  it('groups colocated nodes without changing inventory order or state', () => {
    const hosts = [host('a', ' cn '), host('b', 'CN'), host('c', 'HK')]
    hosts[1]!.state = 'offline'
    const regions = groupGlobeHosts(hosts)
    expect(regions.map(region => region.code)).toEqual(['CN', 'HK'])
    expect(regions[0]!.hosts).toEqual(hosts.slice(0, 2))
    expect(regions[0]!.hosts[1]!.state).toBe('offline')
  })

  it('does not invent positions from a name, city, missing data or an invalid code', () => {
    const unknown = host('Singapore', undefined)
    unknown.lastSnapshot!.telemetry.publicNetwork.city = 'Singapore'
    expect(groupGlobeHosts([unknown, host('invalid', 'XX'), host('prototype', 'toString'), { id: 'waiting' } as ClusterHost])).toEqual([])
  })

  it('includes small hosting regions and bounds all baked-in coordinates and geometry', () => {
    expect(groupGlobeHosts(['HK', 'SG', 'MO', 'TW', 'LU', 'MT', 'IS'].map(code => host(code, code)))).toHaveLength(7)
    expect(landDots.length).toBeLessThan(3000)
    for (const [lat, lon] of [...landDots, ...Object.values(regionCenters)]) {
      expect(Number.isFinite(lat) && Math.abs(lat) <= 90).toBe(true)
      expect(Number.isFinite(lon) && Math.abs(lon) <= 180).toBe(true)
    }
  })

  it('projects the camera location to the center and hides its antipode', () => {
    const [x, y, z] = projectGlobePoint(37, 179, 37, 179)
    expect(x).toBeCloseTo(0); expect(y).toBeCloseTo(0); expect(z).toBeCloseTo(1)
    expect(projectGlobePoint(-37, -1, 37, 179)[2]).toBeCloseTo(-1)
    expect(projectGlobePoint(0, -179, 0, 179)[0]).toBeCloseTo(Math.sin(2 * Math.PI / 180))
  })

  it('caps pixel density and renders clickable flags only on the visible hemisphere', () => {
    const drawImage = vi.fn()
    const ctx = new Proxy({ canvas: { width: 0, height: 0 } }, {
      get(target, key) {
        if (key === 'canvas') return target.canvas
        if (key === 'drawImage') return drawImage
        if (key === 'createRadialGradient') return () => ({ addColorStop: vi.fn() })
        return vi.fn()
      },
    }) as unknown as CanvasRenderingContext2D
    const globe = new GlobeRenderer(ctx, { brand: '#087', land: '#064', light: true, text: '#123', surface: '#fff', border: '#def', success: '#087', warning: '#975' })
    const region = groupGlobeHosts([host('a', 'CN')])[0]!
    globe.resize(800, 400, 4)
    expect(ctx.canvas.width).toBe(1400)
    expect(ctx.canvas.height).toBe(700)
    globe.setHosts([region], 'a')
    globe.focus(region); globe.draw()
    expect(globe.hitTest(400, 200)?.hosts[0]!.id).toBe('a')
    expect(drawImage).not.toHaveBeenCalled()
    const flag = {} as CanvasImageSource
    globe.setFlag(' cn ', flag)
    globe.draw()
    expect(drawImage).toHaveBeenCalledWith(flag, expect.any(Number), expect.any(Number), 18, 18)
    const [, flagX, flagY] = drawImage.mock.calls[0]!
    expect(globe.hitTest(flagX + 9, flagY + 9)?.code).toBe('CN')
    drawImage.mockClear()
    globe.move(180, 0); globe.draw()
    expect(globe.hitTest(400, 200)).toBeUndefined()
    expect(globe.hitTest(flagX + 9, flagY + 9)).toBeUndefined()
    expect(drawImage).not.toHaveBeenCalled()
    globe.setHosts([], '')
    globe.setHosts([region], 'a')
    globe.focus(region); globe.draw()
    expect(drawImage).not.toHaveBeenCalled()
  })

  it.each([340, 700, 1000])('keeps dense labels bounded and disjoint at %s px, preserving the selected node', (width) => {
    const regions = groupGlobeHosts(Object.keys(regionCenters).map(code => host(code, code)))
    for (const longitude of [0, 105, 180, 280]) {
      const anchors = regions.map(region => {
        const [x, y, z] = projectGlobePoint(region.latitude, region.longitude, 18, longitude)
        return { region, x: width / 2 + x * 120, y: 200 - y * 120, selected: false, width: 78, text: region.code, z }
      }).filter(point => point.z > .06)
      anchors[anchors.length - 1]!.selected = true
      const labels = layoutGlobeLabels(anchors, width, 400)
      expect(labels.some(label => label.selected)).toBe(true)
      expect(labels.length).toBeLessThanOrEqual(16)
      for (let i = 0; i < labels.length; i++) {
        const a = labels[i]!
        expect(a.left).toBeGreaterThanOrEqual(10)
        expect(a.top).toBeGreaterThanOrEqual(10)
        expect(a.left + a.width).toBeLessThanOrEqual(width - 10)
        expect(a.top + a.height).toBeLessThanOrEqual(390)
        for (const b of labels.slice(i + 1)) {
          expect(a.left < b.left + b.width && a.left + a.width > b.left && a.top < b.top + b.height && a.top + a.height > b.top).toBe(false)
        }
      }
    }
  })
})
