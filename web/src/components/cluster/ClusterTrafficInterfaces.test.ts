// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useI18n } from '@/i18n'
import type { ClusterHost, TrafficInterfacesSnapshot } from '@/types/api'
import ClusterTrafficInterfaces from './ClusterTrafficInterfaces.vue'

const mocks = vi.hoisted(() => ({ read: vi.fn(), save: vi.fn() }))
vi.mock('@/lib/api', () => {
  class ApiError extends Error {
    constructor(message: string, readonly status = 0, readonly code = 'request_failed') { super(message) }
  }
  return { ApiError, api: { system: { trafficInterfaces: mocks.read, updateTrafficInterfaces: mocks.save } } }
})
const { ApiError } = await import('@/lib/api')

const { t } = useI18n()
const localHost = { id: 'local', isLocal: true, kind: 'kpanel' } as unknown as ClusterHost

function snapshot(overrides: Partial<TrafficInterfacesSnapshot> = {}): TrafficInterfacesSnapshot {
  return {
    selection: { include: [], exclude: [] },
    interfaces: [
      { name: 'eth0', receivedBytes: 2048, sentBytes: 4096, counted: true, reason: 'default-route' },
      { name: 'lo', receivedBytes: 1, sentBytes: 1, counted: false, reason: 'loopback' },
      { name: 'warp', receivedBytes: 10, sentBytes: 20, counted: false, reason: 'not-default-route' },
    ],
    resourceVersion: 'sha256:a',
    ...overrides,
  }
}

const wrappers: ReturnType<typeof mount>[] = []
async function render(host: ClusterHost = localHost) {
  const wrapper = mount(ClusterTrafficInterfaces, { props: { host, disabled: false } })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}
type Exposed = { dirty: boolean; validate: () => string; save: () => Promise<void> }
const exposed = (wrapper: ReturnType<typeof mount>) => wrapper.vm as unknown as Exposed

beforeEach(() => {
  vi.clearAllMocks()
  mocks.read.mockResolvedValue(snapshot())
})
afterEach(() => wrappers.splice(0).forEach((wrapper) => wrapper.unmount()))

describe('cluster counted interfaces', () => {
  it('shows the automatic choice read-only and hides loopback', async () => {
    const wrapper = await render()
    const boxes = wrapper.findAll<HTMLInputElement>('input[type="checkbox"]')
    expect(wrapper.findAll('code').map((code) => code.text())).toEqual(['eth0', 'warp'])
    expect(boxes).toHaveLength(2)
    expect(boxes.map((box) => [box.element.checked, box.element.disabled])).toEqual([[true, true], [false, true]])
    expect(wrapper.text()).toContain(t('cluster.trafficInterfaces.reason.default-route'))
    expect(exposed(wrapper).dirty).toBe(false)
  })

  it('starts a manual choice from the counted interfaces and saves an explicit list', async () => {
    mocks.save.mockResolvedValue(snapshot({ selection: { include: ['eth0', 'warp'], exclude: [] }, resourceVersion: 'sha256:b' }))
    const wrapper = await render()
    await wrapper.find('input[type="radio"][value="manual"]').setValue(true)
    const boxes = wrapper.findAll<HTMLInputElement>('input[type="checkbox"]')
    expect(boxes.map((box) => box.element.checked)).toEqual([true, false])
    await boxes[1]!.setValue(true)
    expect(exposed(wrapper).dirty).toBe(true)
    expect(exposed(wrapper).validate()).toBe('')
    await exposed(wrapper).save()
    expect(mocks.save).toHaveBeenCalledWith({ include: ['eth0', 'warp'], exclude: [], expectedResourceVersion: 'sha256:a' })
    expect(exposed(wrapper).dirty).toBe(false)
  })

  it('refuses an empty manual choice before any save starts', async () => {
    const wrapper = await render()
    await wrapper.find('input[type="radio"][value="manual"]').setValue(true)
    await wrapper.findAll('input[type="checkbox"]')[0]!.setValue(false)
    expect(exposed(wrapper).validate()).toBe(t('cluster.trafficInterfaces.chooseOne'))
  })

  it('keeps exclusions when returning to the automatic choice', async () => {
    mocks.read.mockResolvedValue(snapshot({ selection: { include: ['eth0'], exclude: [] } }))
    mocks.save.mockResolvedValue(snapshot())
    const wrapper = await render()
    expect(wrapper.find<HTMLInputElement>('input[type="radio"][value="manual"]').element.checked).toBe(true)
    await wrapper.find('input[type="radio"][value="auto"]').setValue(true)
    await exposed(wrapper).save()
    expect(mocks.save).toHaveBeenCalledWith({ include: [], exclude: [], expectedResourceVersion: 'sha256:a' })

    mocks.read.mockResolvedValue(snapshot({ selection: { include: [], exclude: ['warp'] } }))
    const excluded = await render()
    expect(excluded.text()).toContain(t('cluster.trafficInterfaces.excluded', { names: 'warp' }))
    expect(exposed(excluded).dirty).toBe(false)
  })

  it('reloads after a conflicting change and reports it to the caller', async () => {
    mocks.save.mockRejectedValue(new ApiError('changed', 409, 'traffic_interfaces_changed'))
    const wrapper = await render()
    await wrapper.find('input[type="radio"][value="manual"]').setValue(true)
    await expect(exposed(wrapper).save()).rejects.toMatchObject({ code: 'traffic_interfaces_changed' })
    expect(mocks.read).toHaveBeenCalledTimes(2)
  })

  it('explains a fallback when every chosen interface is missing', async () => {
    mocks.read.mockResolvedValue(snapshot({
      selection: { include: ['ppp0'], exclude: [] },
      interfaces: [
        { name: 'eth0', receivedBytes: 1, sentBytes: 1, counted: true, reason: 'default-route' },
        { name: 'ppp0', receivedBytes: 0, sentBytes: 0, counted: false, reason: 'missing' },
      ],
    }))
    const wrapper = await render()
    expect(wrapper.text()).toContain(t('cluster.trafficInterfaces.fallback'))
    expect(wrapper.text()).toContain(t('cluster.trafficInterfaces.reason.missing'))
  })

  it('names an Agent without the endpoint instead of a generic failure', async () => {
    mocks.read.mockRejectedValue(new ApiError('not found', 404, 'not_found'))
    const wrapper = await render()
    expect(wrapper.text()).toContain(t('cluster.trafficInterfaces.unsupported'))
    expect(exposed(wrapper).dirty).toBe(false)
  })

  it('points other hosts to where their own selection lives without reading', async () => {
    const light = await render({ id: 'n', isLocal: false, kind: 'light_node' } as unknown as ClusterHost)
    expect(light.text()).toContain('/usr/local/lib/kejilion-node/kejilion-node interfaces include eth0')
    const remote = await render({ id: 'r', isLocal: false, kind: 'kpanel' } as unknown as ClusterHost)
    expect(remote.text()).toContain(t('cluster.trafficInterfaces.remoteHint'))
    expect(mocks.read).not.toHaveBeenCalled()
  })
})
