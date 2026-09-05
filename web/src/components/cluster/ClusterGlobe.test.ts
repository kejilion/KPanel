// @vitest-environment jsdom
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterGlobe from './ClusterGlobe.vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { registerPhraseCatalog } from '@/i18n/phrase'
import english from '@/i18n/pages/shared/en-US'
import type { ClusterHost, PublicClusterShareHost } from '@/types/api'
import type { GlobeHost } from './globeRenderer'

const render = vi.hoisted(() => ({ draw: vi.fn(), focus: vi.fn(), move: vi.fn(), setHosts: vi.fn(), disconnect: vi.fn() }))
vi.mock('./globeRenderer', async importOriginal => ({
  ...await importOriginal<typeof import('./globeRenderer')>(),
  GlobeRenderer: class {
    draw = render.draw
    focus = render.focus
    move = render.move
    setHosts = render.setHosts
    setFlag = vi.fn()
    resize = vi.fn()
    setPalette = vi.fn()
    setZoom = vi.fn()
  },
}))

function host(id: string, code?: string): ClusterHost {
  return {
    id, name: id, state: 'online', kind: 'light_node',
    lastSnapshot: { receivedAt: '2026-09-05T00:00:00Z', telemetry: {
      publicNetwork: { countryCode: code }, os: 'Debian',
      cpu: { usagePercent: 5 }, memory: { usagePercent: 25 }, disk: { usagePercent: 10 },
    } },
  } as ClusterHost
}

let wrapper: VueWrapper | undefined
let frames: Map<number, FrameRequestCallback>
let nextFrame: number
let intersect: (entries: { isIntersecting: boolean }[]) => void
let reduced = false
function flushFrame(time = 40) {
  const pending = [...frames.values()]
  frames.clear()
  pending.forEach(callback => callback(time))
}

beforeEach(() => {
  vi.clearAllMocks()
  frames = new Map(); nextFrame = 0; reduced = false
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({} as CanvasRenderingContext2D)
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => { frames.set(++nextFrame, callback); return nextFrame })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => frames.delete(id))
  vi.stubGlobal('ResizeObserver', class { observe = vi.fn(); disconnect = render.disconnect })
  vi.stubGlobal('IntersectionObserver', class {
    constructor(callback: typeof intersect) { intersect = callback }
    observe = vi.fn(); disconnect = render.disconnect
  })
  vi.stubGlobal('matchMedia', () => ({ matches: reduced, addEventListener: vi.fn(), removeEventListener: vi.fn() }))
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks(); vi.unstubAllGlobals() })

function setup(hosts: GlobeHost[] = [host('alpha', 'CN'), host('beta')], active = ref(true)) {
  wrapper = mount(ClusterGlobe, { props: { hosts }, global: { provide: { [desktopWindowActiveKey as symbol]: active }, stubs: { CountryFlagIcon: true } } })
  return wrapper
}

describe('cluster globe interaction and lifecycle', () => {
  it.each(['unknown', 'pending'] as const)('does not count waiting state %s as needing attention', async state => {
    const waiting: GlobeHost = state === 'unknown'
      ? { ...host('waiting', 'SG'), state, lastSnapshot: undefined }
      : { id: 'waiting', name: 'waiting', state, location: {} } as PublicClusterShareHost
    const failed = { ...host('failed', 'US'), state: 'offline' as const }
    const view = setup([waiting, failed, host('healthy', 'DE')])
    const attentionButton = view.findAll('.cluster-globe__filters button').find(button => button.text().startsWith('需关注'))!
    expect(attentionButton.text()).toBe('需关注 1')
    await attentionButton.trigger('click')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(1)
    expect(view.get('.cluster-globe__node').text()).toContain('failed')
    await view.setProps({ hosts: [waiting] })
    expect(attentionButton.text()).toBe('需关注 0')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(0)
  })

  it('uses only public samples for a shared globe and never offers management actions', async () => {
    const shared: PublicClusterShareHost = {
      id: 'public-id', name: 'Public Singapore', state: 'degraded', os: 'Debian', architecture: 'arm64',
      location: { countryCode: 'SG', country: 'Singapore', city: 'Singapore', isp: 'Public Network' },
      collectedAt: '2026-09-05T00:00:00Z', uptimeSeconds: 3600,
      cpu: { cores: 2, usagePercent: 12.5 }, load: { one: 0, five: 0, fifteen: 0 },
      memory: { totalBytes: 1024 ** 3, usedBytes: 1024 ** 2, usagePercent: 1 },
      disk: { totalBytes: 1024 ** 3, usedBytes: 1024 ** 2, usagePercent: 1 },
      network: { receivedBytes: 1024, sentBytes: 2048, receiveBytesPerSecond: 100, transmitBytesPerSecond: 200 },
    }
    const view = setup([shared])
    await view.get('.cluster-globe__node').trigger('click')
    expect(view.get('h3').text()).toBe(shared.name)
    expect(view.get('.cluster-globe__location').text()).toBe('Singapore')
    expect(view.get('.cluster-globe__detail-heading').text()).toContain('需关注')
    expect(view.get('.cluster-globe__metrics').text()).toContain('12.5%')
    expect(view.get('.cluster-globe__metrics').text()).toContain('2 核')
    expect(view.get('.cluster-globe__metrics').text()).toContain('1.0 MB / 1.0 GB')
    expect(view.get('.cluster-globe__host-details').text()).toContain('arm64')
    expect(view.get('.cluster-globe__host-details').text()).toContain('Public Network')
    expect(view.get('.cluster-globe__host-details').text()).toContain('100 B/s')
    expect(view.get('.cluster-globe__host-details').text()).toContain('200 B/s')
    expect(view.get('.cluster-globe__host-details').text()).toContain('累计传送2.0 KB')
    expect(view.get('.cluster-globe__host-details').text()).toContain('运行时间1 小时 0 分钟')
    expect(view.get('.cluster-globe__host-details').text()).not.toContain('延迟')
    expect(view.get('.cluster-globe__seen').text()).toContain('采集于')
    expect(view.get('.cluster-globe__seen').text()).not.toContain('最近在线')
    expect(view.find('.cluster-globe__actions').exists()).toBe(false)
    expect(render.focus).toHaveBeenCalledWith(expect.objectContaining({ code: 'SG' }))
    await view.setProps({ hosts: [{ ...shared, state: 'pending', location: {}, collectedAt: undefined }] })
    expect(view.get('.cluster-globe__detail-heading').text()).toContain('等待数据')
    expect(view.text()).toContain('地区未公开')
    expect(view.get('.cluster-globe__metrics').text()).not.toContain('%')
    expect(view.find('.cluster-globe__host-details').exists()).toBe(false)
    expect(view.text()).toContain('等待首次主机摘要')
    expect(view.find('.cluster-globe__actions').exists()).toBe(false)
    expect(view.emitted('manage')).toBeUndefined()
    expect(view.emitted('openPanel')).toBeUndefined()
  })

  it('retains card monitoring fields for managed nodes and follows refreshed samples', async () => {
    const node = host('full-node', 'DE')
    const snapshot = node.lastSnapshot!
    Object.assign(snapshot, { latencyMilliseconds: 42, receiveBytesPerSecond: 1024, transmitBytesPerSecond: 2048 })
    Object.assign(snapshot.telemetry, {
      os: 'Debian GNU/Linux 13', architecture: 'arm64', kernel: '6.12-test', uptimeSeconds: 90000,
      cpu: { cores: 8, usagePercent: 12.5 },
      memory: { usedBytes: 2 * 1024 ** 3, totalBytes: 8 * 1024 ** 3, usagePercent: 25 },
      disk: { usedBytes: 20 * 1024 ** 3, totalBytes: 80 * 1024 ** 3, usagePercent: 25 },
      network: { receivedBytes: 123 * 1024 ** 3, sentBytes: 45 * 1024 ** 3 },
      publicNetwork: { countryCode: 'DE', country: 'Germany', city: 'Frankfurt', isp: 'Test Network' },
    })
    const view = setup([node])
    expect(view.get('.cluster-globe__node-metrics').text()).toContain('磁盘 25%')
    await view.get('.cluster-globe__node').trigger('click')
    const metrics = view.get('.cluster-globe__metrics').text()
    for (const value of ['12.5%', '8 核', '2.0 GB / 8.0 GB', '20.0 GB / 80.0 GB']) expect(metrics).toContain(value)
    const details = view.get('.cluster-globe__host-details').text()
    for (const value of ['Debian GNU/Linux 13', 'arm64 · 6.12-test', 'Test Network', '实时下行1.0 KB/s', '实时上行2.0 KB/s', '累计接收123.0 GB', '累计传送45.0 GB', '运行时间1 天 1 小时', '延迟42 ms']) expect(details).toContain(value)

    await view.setProps({ hosts: [{ ...node, lastSnapshot: { ...snapshot, receiveBytesPerSecond: 0, latencyMilliseconds: 0, telemetry: { ...snapshot.telemetry, uptimeSeconds: 0, network: { ...snapshot.telemetry.network, sentBytes: 0 } } } }] })
    expect(view.get('.cluster-globe__host-details').text()).toContain('实时下行0 B/s')
    expect(view.get('.cluster-globe__host-details').text()).toContain('累计传送0 B')
    expect(view.get('.cluster-globe__host-details').text()).toContain('运行时间0 秒')
    expect(view.get('.cluster-globe__host-details').text()).toContain('延迟--')

    await view.setProps({ hosts: [{ ...node, lastSnapshot: undefined }] })
    expect(view.find('.cluster-globe__host-details').exists()).toBe(false)
    expect(view.get('.cluster-globe__metrics').text()).not.toContain('%')
    expect(view.text()).toContain('等待首次主机摘要')
  })

  it('translates dynamic location counts, timestamps and management labels', async () => {
    const unregister = registerPhraseCatalog(english)
    try {
      const node = host('alpha', 'CN')
      node.kind = 'panel'; node.isLocal = true
      const view = setup([node])
      await view.get('.cluster-globe__node').trigger('click')
      expect(view.get('.cluster-globe__legend-items').text()).toContain('1 / 1 located')
      expect(view.get('.cluster-globe__seen').text()).toContain('Last online')
      expect(view.get('.cluster-globe__seen').text()).toContain('Collected at')
      expect(view.get('.cluster-globe__actions').text()).toContain('Manage')
      expect(view.get('.cluster-globe__actions').text()).toContain('Current panel')
    } finally { unregister() }
  })

  it('stops animation when the desktop window loses focus, and fully cancels on unmount', async () => {
    const active = ref(true)
    const view = setup(undefined, active)
    flushFrame()
    expect(render.draw).toHaveBeenCalledTimes(1)
    expect(frames.size).toBe(1)
    active.value = false; await nextTick()
    expect(frames.size).toBe(0)
    active.value = true; await nextTick()
    expect(frames.size).toBe(1)
    view.unmount(); wrapper = undefined
    expect(frames.size).toBe(0)
    expect(render.disconnect).toHaveBeenCalledTimes(2)
  })

  it('stops offscreen and while the browser document is hidden', () => {
    setup(); flushFrame()
    intersect([{ isIntersecting: false }]); expect(frames.size).toBe(0)
    intersect([{ isIntersecting: true }]); expect(frames.size).toBe(1)
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(frames.size).toBe(0)
  })

  it('draws one static frame under reduced motion, and redraws when telemetry changes', async () => {
    reduced = true
    const view = setup()
    await nextTick(); flushFrame()
    expect(frames.size).toBe(0)
    expect(view.text()).toContain('自动旋转')
    await view.setProps({ hosts: [host('updated', 'SG')] })
    flushFrame(80)
    expect(render.draw).toHaveBeenCalledTimes(2)
    expect(view.get('.cluster-globe__node-name').text()).toBe('updated')
    expect(frames.size).toBe(0)
  })

  it('keeps unknown and stale nodes selectable and uses the current refreshed object for actions', async () => {
    const view = setup()
    await view.findAll('.cluster-globe__node')[1]!.trigger('click')
    expect(view.get('h3').text()).toBe('beta')
    expect(view.text()).toContain('地理信息不足')
    expect(render.focus).not.toHaveBeenCalled()
    const updated = host('beta')
    updated.state = 'offline'; updated.lastError = 'network disconnected'
    await view.setProps({ hosts: [updated] })
    expect(view.text()).toContain('离线')
    expect(view.text()).toContain('network disconnected')
    await view.get('.cluster-globe__actions button').trigger('click')
    expect(view.emitted('manage')?.[0]?.[0]).toEqual(updated)
    expect(view.text()).not.toContain('打开面板')
  })

  it('supports keyboard rotation and selection without requiring canvas pointer input', async () => {
    const view = setup()
    await view.get('canvas').trigger('keydown', { key: 'ArrowLeft' })
    expect(render.move).toHaveBeenCalledWith(-12, 0)
    expect(view.text()).toContain('自动旋转')
    await view.findAll('.cluster-globe__node')[0]!.trigger('click')
    expect(render.focus).toHaveBeenCalledWith(expect.objectContaining({ code: 'CN' }))
  })

  it('filters many nodes and returns from detail without losing the active filters', async () => {
    const nodes = Array.from({ length: 30 }, (_, i) => host(`node-${i}`, i < 15 ? 'CN' : 'SG'))
    nodes[4]!.state = 'offline'
    nodes[7]!.state = 'offline'
    nodes[20]!.state = 'offline'
    const view = setup(nodes)
    expect(view.findAll('.cluster-globe__node')).toHaveLength(30)
    expect(view.find('.cluster-globe__detail').exists()).toBe(false)
    await view.get('select').setValue('CN')
    await view.findAll('.cluster-globe__status-filters button')[2]!.trigger('click')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(2)
    await view.findAll('.cluster-globe__node')[0]!.trigger('click')
    expect(view.get('h3').text()).toBe('node-4')
    await view.get('[aria-label="下一个节点"]').trigger('click')
    expect(view.get('h3').text()).toBe('node-7')
    await view.get('.cluster-globe__back').trigger('click')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(2)
    await view.get('input[type="search"]').setValue('no-such-node')
    expect(view.text()).toContain('没有匹配的节点')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(0)
    await view.get('.cluster-globe__empty button').trigger('click')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(30)
  })

  it('retains host details and management if Canvas 2D is unavailable', async () => {
    vi.mocked(HTMLCanvasElement.prototype.getContext).mockReturnValue(null)
    const view = setup()
    await nextTick()
    expect(view.text()).toContain('地球绘制不可用')
    expect(view.findAll('.cluster-globe__node')).toHaveLength(2)
    expect(view.get('.cluster-globe__controls button').attributes('disabled')).toBeDefined()
    expect(frames.size).toBe(0)
  })
})
