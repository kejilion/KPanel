// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterView from './ClusterView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), createLightEnrollment: vi.fn(), add: vi.fn() }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {},
  api: { cluster: mocks },
}))

let wrapper: ReturnType<typeof mount> | undefined
let items: Record<string, unknown>[]
const node = { id: 'enrolled-node', kind: 'light_node', name: 'Test node', state: 'unknown' }
const reportedNode = () => ({
  ...node,
  state: 'online',
  lastSuccessAt: new Date().toISOString(),
  lastSnapshot: {
    telemetry: {
      cpu: { cores: 2, usagePercent: 10 },
      memory: { usedBytes: 1024, totalBytes: 2048, usagePercent: 50 },
      disk: { usedBytes: 1024, totalBytes: 2048, usagePercent: 50 },
      network: { receivedBytes: 1024, sentBytes: 1024 },
      publicNetwork: {}, os: 'Linux', uptimeSeconds: 60,
    },
  },
})
const primary = () => document.querySelector<HTMLButtonElement>('button[form="cluster-add-form"]')!
const credential = () => document.querySelector<HTMLTextAreaElement>('[name="cluster-access-credential"]')!
const form = () => document.querySelector<HTMLFormElement>('#cluster-add-form')!

async function openEnrollment(): Promise<void> {
  wrapper = mount(ClusterView, { attachTo: document.body, global: { stubs: { RouterLink: true } } })
  await flushPromises()
  await wrapper.get('.cluster-hero__add').trigger('click')
  expect(credential().required).toBe(true)
  expect(form().checkValidity()).toBe(false)
  const generate = Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
    .find((button) => button.textContent?.includes('生成接入命令'))!
  generate.click()
  await flushPromises()
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] })
  vi.setSystemTime(new Date('2026-09-12T10:00:00Z'))
  vi.clearAllMocks()
  items = []
  mocks.hosts.mockImplementation(async () => ({
    items, total: items.length, remoteTotal: items.length, maxHosts: 100, pollIntervalSeconds: 30, nodeId: 'center',
  }))
  mocks.createLightEnrollment.mockResolvedValue({
    id: node.id, command: 'test enrollment command', expiresAt: '2026-09-12T10:00:05Z',
  })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  localStorage.clear()
  vi.useRealTimers()
})

describe('light node enrollment form', () => {
  it('submits the real form without credentials after a registered node reports past token expiry', async () => {
    await openEnrollment()
    expect(primary().disabled).toBe(true)
    items = [node]
    await vi.advanceTimersByTimeAsync(2_000)
    expect(primary().textContent).toContain('等待首次上报')
    await vi.advanceTimersByTimeAsync(4_000)
    expect(primary().textContent).toContain('等待首次上报')
    expect(primary().disabled).toBe(true)

    items = [reportedNode()]
    await vi.advanceTimersByTimeAsync(2_000)
    expect(primary().disabled).toBe(false)
    expect(credential().required).toBe(false)
    expect(credential().value).toBe('')
    expect(form().checkValidity()).toBe(true)
    primary().click()
    await flushPromises()
    expect(document.querySelector('#cluster-add-form')).toBeNull()
    expect(mocks.add).not.toHaveBeenCalled()
  })

  it('checks the latest host list before declaring a command expired', async () => {
    await openEnrollment()
    await vi.advanceTimersByTimeAsync(4_000)
    items = [reportedNode()]
    await vi.advanceTimersByTimeAsync(2_000)
    expect(primary().disabled).toBe(false)
    expect(primary().textContent).toContain('添加主机')
  })

  it('keeps an unused command expired across normal refreshes and resets when regenerated', async () => {
    await openEnrollment()
    await vi.advanceTimersByTimeAsync(6_000)
    expect(primary().textContent).toContain('命令已过期')
    expect(primary().disabled).toBe(true)
    await vi.advanceTimersByTimeAsync(10_000)
    expect(primary().textContent).toContain('命令已过期')
    mocks.createLightEnrollment.mockResolvedValueOnce({
      id: 'new-enrollment', command: 'new test command', expiresAt: '2026-09-12T10:05:00Z',
    })
    Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
      .find((button) => button.textContent?.includes('重新生成'))!.click()
    await flushPromises()
    expect(primary().textContent).toContain('等待节点执行')
    expect(primary().disabled).toBe(true)
    expect(credential().required).toBe(true)
  })
})
