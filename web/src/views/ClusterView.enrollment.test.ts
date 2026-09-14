// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ClusterView from './ClusterView.vue'

const mocks = vi.hoisted(() => ({
  hosts: vi.fn(),
  createLightEnrollment: vi.fn(),
  lightBatchEnrollments: vi.fn(),
  createLightBatchEnrollment: vi.fn(),
  revokeLightBatchEnrollment: vi.fn(),
  add: vi.fn(),
}))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {},
  api: { cluster: mocks },
}))

let wrapper: ReturnType<typeof mount> | undefined
let items: Record<string, unknown>[]
const node = { id: 'enrolled-node', kind: 'light_node', name: 'Test node', state: 'unknown' }
const reportedNode = (publicNetwork: Record<string, string> = {}) => ({
  ...node,
  state: 'online',
  lastSuccessAt: new Date().toISOString(),
  lastSnapshot: {
    telemetry: {
      cpu: { cores: 2, usagePercent: 10 },
      memory: { usedBytes: 1024, totalBytes: 2048, usagePercent: 50 },
      disk: { usedBytes: 1024, totalBytes: 2048, usagePercent: 50 },
      network: { receivedBytes: 1024, sentBytes: 1024 },
      publicNetwork, os: 'Linux', uptimeSeconds: 60,
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
  mocks.lightBatchEnrollments.mockResolvedValue({ items: [], total: 0 })
  mocks.createLightBatchEnrollment.mockResolvedValue({
    id: 'batch-enrollment',
    command: "bash <(curl -fsSL https://kejilion.sh) kpanel node join 'kpb1.batch-token'",
    maxUses: 100,
    usedCount: 0,
    remainingCount: 100,
    createdAt: '2026-09-12T10:00:00Z',
    expiresAt: '2026-09-13T10:00:00Z',
  })
  mocks.revokeLightBatchEnrollment.mockResolvedValue({ deleted: true })
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  document.body.innerHTML = ''
  localStorage.clear()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('light node enrollment form', () => {
  it('shows reported public IPs in the host address position without making them links', async () => {
    items = [reportedNode({ ipv4: '198.51.100.20', ipv6: '2001:db8::20' })]
    wrapper = mount(ClusterView, { attachTo: document.body, global: { stubs: { RouterLink: true } } })
    await flushPromises()

    const address = wrapper.get('.cluster-card__origin--static')
    expect(address.element.tagName).toBe('SPAN')
    expect(address.text()).toBe('198.51.100.20 · 2001:db8::20')
    expect(address.attributes('href')).toBeUndefined()

    const manage = wrapper.findAll('button').find((button) => button.text().includes('管理'))!
    await manage.trigger('click')
    expect(document.querySelector('.cluster-manage__identity')?.textContent).toContain('公网 IP')
    expect(document.querySelector('.cluster-manage__identity')?.textContent).toContain('198.51.100.20 · 2001:db8::20')
  })

  it('keeps a clear placeholder before a light node reports its public IP', async () => {
    items = [reportedNode()]
    wrapper = mount(ClusterView, { attachTo: document.body, global: { stubs: { RouterLink: true } } })
    await flushPromises()

    expect(wrapper.get('.cluster-card__origin--static').text()).toBe('公网 IP 未获取')
  })

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

  it('keeps the single flow as default and turns copy into done for a 100-host batch', async () => {
    const clipboardWrite = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText: clipboardWrite } })
    wrapper = mount(ClusterView, { attachTo: document.body, global: { stubs: { RouterLink: true } } })
    await flushPromises()
    await wrapper.get('.cluster-hero__add').trigger('click')

    expect(document.querySelector('#cluster-add-form')).not.toBeNull()
    const batchTab = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="tab"]'))
      .find((button) => button.textContent?.includes('批量接入'))!
    batchTab.click()
    await flushPromises()

    expect(mocks.lightBatchEnrollments).toHaveBeenCalledTimes(1)
    const maxUses = document.querySelector<HTMLInputElement>('[name="cluster-light-batch-max-uses"]')!
    expect(maxUses.value).toBe('100')
    const generate = document.querySelector<HTMLButtonElement>('button[form="cluster-batch-add-form"]')!
    expect(generate.textContent).toContain('生成批量命令')
    generate.click()
    await flushPromises()

    expect(mocks.createLightBatchEnrollment).toHaveBeenCalledWith({
      namePrefix: undefined,
      maxUses: 100,
      expiresInSeconds: 86_400,
    })
    const copy = Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
      .find((button) => button.textContent?.includes('复制命令') && !button.closest('.cluster-light-enrollment'))!
    copy.click()
    await flushPromises()

    expect(clipboardWrite).toHaveBeenCalledWith(
      "bash <(curl -fsSL https://kejilion.sh) kpanel node join 'kpb1.batch-token'",
    )
    const done = Array.from(document.querySelectorAll<HTMLButtonElement>('button'))
      .find((button) => button.textContent?.includes('完成'))!
    done.click()
    await flushPromises()
    expect(document.querySelector('#cluster-batch-add-form')).toBeNull()
  })

  it('revokes only the selected future enrollment authorization', async () => {
    const enrollment = {
      id: 'active-batch', namePrefix: 'edge', maxUses: 100, usedCount: 23, remainingCount: 77,
      createdAt: '2026-09-12T09:00:00Z', expiresAt: '2026-09-13T09:00:00Z',
    }
    mocks.lightBatchEnrollments.mockResolvedValueOnce({ items: [enrollment], total: 1 })
    const confirm = vi.fn().mockReturnValue(true)
    vi.stubGlobal('confirm', confirm)
    wrapper = mount(ClusterView, { attachTo: document.body, global: { stubs: { RouterLink: true } } })
    await flushPromises()
    await wrapper.get('.cluster-hero__add').trigger('click')
    Array.from(document.querySelectorAll<HTMLButtonElement>('[role="tab"]'))
      .find((button) => button.textContent?.includes('批量接入'))!.click()
    await flushPromises()

    const revoke = Array.from(document.querySelectorAll<HTMLButtonElement>('.cluster-light-batch__active button'))
      .find((button) => button.textContent?.includes('撤销'))!
    revoke.click()
    await flushPromises()

    expect(confirm).toHaveBeenCalledTimes(1)
    expect(mocks.revokeLightBatchEnrollment).toHaveBeenCalledWith(enrollment.id)
    expect(document.querySelector('#cluster-batch-add-form')).not.toBeNull()
    expect(document.querySelector('.cluster-light-batch__active')?.textContent).not.toContain('edge')
  })
})
