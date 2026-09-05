// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import JobsView from './JobsView.vue'
import type { Job } from '@/types/api'
import { ref } from 'vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'

const mocks = vi.hoisted(() => ({ list: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class ApiError extends Error {}, api: { jobs: { list: mocks.list } } }))

const job: Job = { id: 'docker:abc', action: 'docker.image_pull', status: 'queued', progress: 0, createdAt: '2026-09-05T00:00:00Z', stages: [{ name: 'queued', status: 'failed' }] }
let wrapper: ReturnType<typeof mount> | undefined
const active = ref(true)

async function openView() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/jobs', component: JobsView }, { path: '/docker', component: { template: '<div />' } }] })
  await router.push('/jobs')
  wrapper = mount(JobsView, { global: { plugins: [router], provide: { [desktopWindowActiveKey as symbol]: active }, stubs: {
    ModalDialog: { props: ['open'], template: '<div v-if="open" class="dialog"><slot /><slot name="footer" /></div>' },
    StatusBadge: { props: ['status'], template: '<span class="badge">{{ status }}</span>' },
  } } })
  await flushPromises()
  return wrapper
}

describe('job owner state continuity', () => {
  beforeEach(() => { active.value = true; vi.useFakeTimers(); mocks.list.mockReset(); mocks.list.mockResolvedValue({ items: [job] }) })
  afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

  it('keeps the selected identity and refreshes queued, running and terminal detail', async () => {
    const view = await openView()
    await view.get('.job-item').trigger('click')
    expect(view.get('.dialog').text()).toContain('queued')
    expect(view.get('.dialog').text()).not.toContain('failed')
    for (const status of ['running', 'succeeded', 'failed_needs_attention'] as const) {
      mocks.list.mockResolvedValue({ items: [{ ...job, status, progress: status === 'running' ? 15 : 100, errorMessage: status === 'failed_needs_attention' ? 'disk write failed' : undefined }] })
      await vi.advanceTimersByTimeAsync(4000)
      await flushPromises()
      expect(view.get('.dialog').text()).toContain(status)
    }
    expect(view.get('.dialog a').attributes('href')).toBe('/docker')
  })

  it('does not show the old success when the row leaves the query window or reads fail', async () => {
    mocks.list.mockResolvedValue({ items: [{ ...job, status: 'succeeded' }] })
    const view = await openView()
    await view.get('.job-item').trigger('click')
    mocks.list.mockResolvedValue({ items: [] })
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('该记录已不在当前查询窗口中')
    expect(view.get('.dialog').findAll('.badge')).toHaveLength(0)
    mocks.list.mockRejectedValue(new Error('offline'))
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('无法读取任务记录')
    expect(view.findAll('.job-item')).toHaveLength(0)
    mocks.list.mockResolvedValue({ items: [{ ...job, status: 'running' }] })
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('running')
  })

  it('waits for completion before polling and ignores an aborted stale response', async () => {
    const view = await openView()
    let resolveOld!: (result: { items: Job[] }) => void
    mocks.list.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    await vi.advanceTimersByTimeAsync(4000)
    const calls = mocks.list.mock.calls.length
    await vi.advanceTimersByTimeAsync(12000)
    expect(mocks.list).toHaveBeenCalledTimes(calls)
    view.unmount(); wrapper = undefined
    resolveOld({ items: [{ ...job, status: 'succeeded' }] })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(8000)
    expect(mocks.list).toHaveBeenCalledTimes(calls)
  })

  it('stops polling when the desktop window is inactive and refreshes on return', async () => {
    await openView()
    active.value = false
    await flushPromises()
    const count = mocks.list.mock.calls.length
    await vi.advanceTimersByTimeAsync(12000)
    expect(mocks.list).toHaveBeenCalledTimes(count)
    active.value = true
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(count + 1)
  })
})
