// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import JobsView from './JobsView.vue'
import type { Job } from '@/types/api'
import { ref } from 'vue'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'

const mocks = vi.hoisted(() => ({ list: vi.fn(), detail: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class ApiError extends Error { constructor(message: string, public status = 0) { super(message) } }, api: { jobs: { list: mocks.list, detail: mocks.detail } } }))
import { ApiError } from '@/lib/api'

const job: Job = { id: `docker:${'a'.repeat(32)}`, action: 'docker.image_pull', status: 'queued', progress: 0, createdAt: '2026-09-05T00:00:00Z', stages: [{ name: 'queued', status: 'failed' }] }
let wrapper: ReturnType<typeof mount> | undefined
const active = ref(true)

async function openView(path = '/jobs') {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/jobs', component: JobsView }, { path: '/docker', component: { template: '<div />' } }] })
  await router.push(path)
  wrapper = mount(JobsView, { global: { plugins: [router], provide: { [desktopWindowActiveKey as symbol]: active }, stubs: {
    ModalDialog: { props: ['open'], template: '<div v-if="open" class="dialog"><slot /><slot name="footer" /></div>' },
    StatusBadge: { props: ['status'], template: '<span class="badge">{{ status }}</span>' },
  } } })
  await flushPromises()
  return wrapper
}

describe('job owner state continuity', () => {
  beforeEach(() => { active.value = true; vi.useFakeTimers(); mocks.list.mockReset(); mocks.detail.mockReset(); mocks.detail.mockResolvedValue(job); mocks.list.mockResolvedValue({ items: [job] }) })
  afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

  it('keeps an open error report and its draft when background polling recovers the list', async () => {
    mocks.list.mockRejectedValue(new ApiError('offline', 503))
    const view = await openView()
    await view.get('.error-state .problem-report-open').trigger('click')
    await view.get('[data-report-input="actual"]').setValue('Keep this draft')
    mocks.list.mockResolvedValue({ items: [job] })
    await vi.advanceTimersByTimeAsync(4000)
    await flushPromises()
    expect(view.find('[data-report-preview]').exists()).toBe(true)
    expect((view.get('[data-report-preview]').element as HTMLTextAreaElement).value).toContain('Keep this draft')
    expect(view.findAll('.job-item')).toHaveLength(1)
  })

  it('reports only the current confirmed job snapshot and never its resource or error body', async () => {
    mocks.detail.mockResolvedValue({ ...job, status: 'failed', resourceName: '/root/private-seed', errorMessage: 'private-seed', stages: [{ name: 'failed', status: 'failed', message: 'private-seed' }] })
    const view = await openView(`/jobs?job=${job.id}`)
    await view.get('.dialog .problem-report-open').trigger('click')
    const report = JSON.parse((view.get('[data-report-preview]').element as HTMLTextAreaElement).value)
    expect(report.jobId).toBe(job.id)
    expect(report.jobStatus).toBe('failed')
    expect(report.failureStage).toBe('failed')
    expect(report.requestId).toBeNull()
    expect(JSON.stringify(report)).not.toContain('private-seed')
  })

  it('keeps an unavailable task missing while reporting the actual failed lookup request', async () => {
    mocks.detail.mockRejectedValue(Object.assign(new ApiError('private-seed', 404), { code: 'job_not_found', requestId: 'c'.repeat(32) }))
    mocks.list.mockResolvedValue({ items: [] })
    const view = await openView(`/jobs?job=${job.id}`)
    await view.get('.dialog .problem-report-open').trigger('click')
    const report = JSON.parse((view.get('[data-report-preview]').element as HTMLTextAreaElement).value)
    expect(report.recordState).toBe('unavailable')
    expect(report.jobId).toBeNull()
    expect(report.jobStatus).toBeNull()
    expect(report.requestId).toBe('c'.repeat(32))
    expect(report.errorCode).toBe('job_not_found')
  })

  it('keeps the selected identity and refreshes queued, running and terminal detail', async () => {
    const view = await openView()
    await view.get('.job-item').trigger('click')
    await flushPromises()
    expect(view.get('.dialog').text()).toContain('queued')
    expect(view.get('.dialog').text()).not.toContain('failed')
    for (const status of ['running', 'succeeded', 'failed_needs_attention'] as const) {
      mocks.list.mockResolvedValue({ items: [{ ...job, status, progress: status === 'running' ? 15 : 100, errorMessage: status === 'failed_needs_attention' ? 'disk write failed' : undefined }] })
      mocks.detail.mockResolvedValue({ ...job, status })
      await vi.advanceTimersByTimeAsync(4000)
      await flushPromises()
      expect(view.get('.dialog').text()).toContain(status)
    }
    expect(view.get('.dialog a').attributes('href')).toBe('/docker')
  })

  it('recovers detail outside latest50 and clears old success when the owner read fails', async () => {
    mocks.list.mockResolvedValue({ items: [{ ...job, status: 'succeeded' }] })
    const view = await openView()
    await view.get('.job-item').trigger('click')
    await flushPromises()
    mocks.list.mockResolvedValue({ items: [] })
    mocks.detail.mockResolvedValue({ ...job, status: 'running' })
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('running')
    expect(mocks.detail).toHaveBeenCalledWith('docker', 'a'.repeat(32), expect.any(AbortSignal))
    mocks.detail.mockRejectedValue(new ApiError('missing', 404))
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('任务不存在或已超出来源保留期')
    expect(view.get('.dialog').findAll('.badge')).toHaveLength(0)
    mocks.list.mockRejectedValue(new Error('offline'))
    mocks.detail.mockRejectedValue(new Error('offline'))
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('无法确认任务详情')
    expect(view.findAll('.job-item')).toHaveLength(0)
    mocks.list.mockResolvedValue({ items: [{ ...job, status: 'running' }] })
    mocks.detail.mockResolvedValue({ ...job, status: 'running' })
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('running')
  })

  it('reopens an owner identity from the URL without the list or audit window', async () => {
    mocks.list.mockResolvedValue({ items: [] })
    const view = await openView(`/jobs?job=${job.id}`)
    expect(view.get('.dialog').text()).toContain('queued')
    await view.get('.dialog button:last-child').trigger('click')
    await flushPromises()
    const count = mocks.detail.mock.calls.length
    await vi.advanceTimersByTimeAsync(8000)
    expect(mocks.detail).toHaveBeenCalledTimes(count)
    expect(view.find('.dialog').exists()).toBe(false)
  })

  it('ignores late responses after selecting a different task and waits before polling', async () => {
    const other = { ...job, id: `app:${'b'.repeat(32)}`, action: 'app.install', status: 'cancelled' as const }
    mocks.list.mockResolvedValue({ items: [job, other] })
    const view = await openView()
    let resolveOld!: (job: Job) => void
    mocks.detail.mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    await view.findAll('.job-item')[0]!.trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(12000)
    expect(mocks.detail).toHaveBeenCalledTimes(1)
    mocks.detail.mockResolvedValue(other)
    await view.findAll('.job-item')[1]!.trigger('click')
    await flushPromises()
    resolveOld({ ...job, status: 'succeeded' })
    await flushPromises()
    expect(view.get('.dialog').text()).toContain('cancelled')
    expect(view.get('.dialog').text()).not.toContain('succeeded')
  })

  it('keeps healthy source rows with a visible partial warning and clears rows on total failure', async () => {
    mocks.list.mockResolvedValue({ items: [job], partial: true, sources: [{ source: 'app', state: 'unavailable' }, { source: 'docker', state: 'available' }] })
    const view = await openView()
    expect(view.text()).toContain('记录不完整')
    expect(view.text()).toContain('应用')
    expect(view.findAll('.job-item')).toHaveLength(1)
    mocks.list.mockRejectedValue(new Error('offline'))
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.findAll('.job-item')).toHaveLength(0)
    expect(view.text()).toContain('无法读取任务记录')
  })

  it('keeps legacy records uncertain when they leave the window and never queries an invented owner', async () => {
    mocks.list.mockResolvedValue({ items: [{ ...job, id: 'old-audit' }] })
    const view = await openView()
    await view.get('.job-item').trigger('click')
    await flushPromises()
    mocks.list.mockResolvedValue({ items: [] })
    await vi.advanceTimersByTimeAsync(4000)
    expect(view.get('.dialog').text()).toContain('该记录已不在当前查询窗口中')
    expect(mocks.detail).not.toHaveBeenCalled()
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
