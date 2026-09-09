// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileArchiveTools from './FileArchiveTools.vue'
import { ApiError } from '@/lib/api'
import type { FileArchiveJob, FileEntry } from '@/types/api'

const mocks = vi.hoisted(() => ({ archiveJobs: vi.fn(), archiveContents: vi.fn(), createArchiveJob: vi.fn(), changeArchiveJob: vi.fn(), entries: vi.fn() }))
vi.mock('@/lib/api', () => ({
  ApiError: class extends Error {
    readonly status: number
    readonly code: string

    constructor(message: string, status = 0, code = 'request_failed') {
      super(message)
      this.status = status
      this.code = code
    }
  },
  api: { files: mocks },
}))
const source = { name: 'site.zip', path: '/site.zip', resourceVersion: 'sha256:source', kind: 'file' } as FileEntry
const job: FileArchiveJob = { id: 'a'.repeat(32), action: 'extract', name: 'site', target: '/', sources: ['/site.zip'], state: 'queued', entries: 0, processedBytes: 0, createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:00:00Z', result: { action: 'extract', succeeded: [], failed: [] } }
let wrapper: ReturnType<typeof mount<typeof FileArchiveTools>> | undefined
async function open(archiveManagementAvailable = true) {
  wrapper = mount(FileArchiveTools, {
    props: { hostId: 'host-a', path: '/destination', archiveManagementAvailable },
    global: {
      stubs: {
        ModalDialog: {
          props: ['open', 'closeDisabled'],
          emits: ['close'],
          template: '<div v-if="open" class="dialog" :data-close-disabled="String(Boolean(closeDisabled))"><button class="stub-close" type="button" @click="$emit(\'close\')">stub close</button><slot /></div>',
        },
      },
    },
  })
  await flushPromises()
  return wrapper
}
describe('archive UI host and task ownership', () => {
  beforeEach(() => {
    vi.useFakeTimers(); vi.resetAllMocks()
    mocks.archiveJobs.mockResolvedValue({ items: [] })
    mocks.archiveContents.mockResolvedValue({ path: source.path, resourceVersion: source.resourceVersion, directory: '', entries: [{ path: 'assets', name: 'assets', kind: 'directory', sizeBytes: 0 }], total: 1, truncated: false })
    mocks.createArchiveJob.mockResolvedValue(job)
    mocks.changeArchiveJob.mockResolvedValue({ ok: true })
    mocks.entries.mockResolvedValue({ entries: [source], unavailable: [] })
  })
  afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

  it('recovers from a temporary discovery error without remounting the window', async () => {
    mocks.archiveJobs.mockRejectedValueOnce(new Error('temporarily unavailable')).mockResolvedValue({ items: [job] })
    const view = await open()
    expect(view.get('[role=alert]').text()).toContain('temporarily unavailable')
    expect(view.vm.available).toBe(true)
    expect(view.vm.legacy).toBe(false)
    await vi.advanceTimersByTimeAsync(10000); await flushPromises()
    expect(view.vm.available).toBe(true)
    expect(view.find('.archive-job').exists()).toBe(true)
  })

  it('uses the directory capability as truth and never probes archive jobs on a legacy host', async () => {
    const view = await open(false)
    expect(mocks.archiveJobs).not.toHaveBeenCalled()
    expect(view.vm.available).toBe(false)
    expect(view.vm.legacy).toBe(true)
    expect(view.find('[role=alert]').exists()).toBe(false)
  })

  it('keeps advertised support after a relay failure instead of risking a synchronous duplicate', async () => {
    mocks.archiveJobs.mockRejectedValueOnce(new ApiError('远端主机文件代理未连接', 503, 'file_relay_unavailable'))
    const view = await open()
    expect(view.vm.available).toBe(true)
    expect(view.vm.legacy).toBe(false)
    expect(view.get('[role=alert]').text()).toContain('远端主机文件代理未连接')
  })

  it('keeps advertised support and reports the actual error when a supported route returns 404', async () => {
    mocks.archiveJobs.mockRejectedValueOnce(new ApiError('任务不存在或已超出来源保留期', 404, 'job_not_found'))
    const view = await open()
    expect(view.vm.available).toBe(true)
    expect(view.vm.legacy).toBe(false)
    expect(view.get('[role=alert]').text()).toContain('任务不存在或已超出来源保留期')
  })

  it('discovers support without existing tasks, and sends selected members with the source version', async () => {
    const view = await open()
    expect(view.vm.available).toBe(true)
    view.vm.browse(source); await flushPromises()
    expect(mocks.archiveContents).toHaveBeenCalledWith(expect.objectContaining({ path: '/site.zip', resourceVersion: source.resourceVersion }), expect.any(AbortSignal), 'host-a')
    await view.get('tbody input').setValue(true)
    await view.get('.archive-toolbar .button--primary').trigger('click'); await flushPromises()
    await view.get('.archive-form').trigger('submit'); await flushPromises()
    expect(mocks.createArchiveJob).toHaveBeenCalledWith(expect.objectContaining({ archiveEntries: ['assets'], sources: ['/site.zip'], target: '/destination', expectedResourceVersions: { '/site.zip': source.resourceVersion } }), 'host-a')
    expect(mocks.createArchiveJob.mock.calls[0]![0]).not.toHaveProperty('format')
  })

  it('discards a slow archive listing when its window switches hosts', async () => {
    let resolve!: (value: unknown) => void
    mocks.archiveContents.mockImplementation(() => new Promise(done => { resolve = done }))
    const view = await open(); view.vm.browse(source); await flushPromises()
    const signal = mocks.archiveContents.mock.calls[0]![1] as AbortSignal
    await view.setProps({ hostId: 'host-b' })
    resolve({ entries: [{ path: 'private-host-a' }], total: 1 }); await flushPromises()
    expect(signal.aborted).toBe(true)
    expect(view.text()).not.toContain('private-host-a')
    expect(mocks.archiveJobs).toHaveBeenLastCalledWith(expect.any(AbortSignal), 'host-b')
    expect(mocks.createArchiveJob).not.toHaveBeenCalled()
  })

  it('returns keyboard focus to the file opener after browsing and configuring extraction', async () => {
    const opener = document.createElement('button'); document.body.append(opener); opener.focus()
    wrapper = mount(FileArchiveTools, { attachTo: document.body, props: { hostId: 'host-a', path: '/destination' } })
    try {
      await flushPromises(); wrapper.vm.browse(source); await flushPromises()
      document.querySelector<HTMLButtonElement>('.archive-toolbar .button--primary')!.click()
      await flushPromises()
      expect(document.querySelector('.archive-form')).not.toBeNull()
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
      await flushPromises()
      expect(document.querySelector('[role=dialog]')).toBeNull()
      expect(document.activeElement).toBe(opener)
    } finally { wrapper.unmount(); wrapper = undefined; opener.remove() }
  })

  it('keeps an accepted background job independent of a closed window', async () => {
    let resolve!: (value: unknown) => void
    mocks.createArchiveJob.mockImplementation(() => new Promise(done => { resolve = done }))
    const view = await open(); view.vm.configure('extract', [source]); await flushPromises()
    await view.get('form').trigger('submit')
    view.unmount(); wrapper = undefined
    resolve(job); await flushPromises()
    expect(mocks.createArchiveJob).toHaveBeenCalledTimes(1)
    expect(mocks.changeArchiveJob).not.toHaveBeenCalled()
  })

  it('keeps the create form visible and locked while task acceptance is pending', async () => {
    let resolve!: (value: FileArchiveJob) => void
    mocks.createArchiveJob.mockImplementation(() => new Promise(done => { resolve = done }))
    const view = await open(); view.vm.configure('extract', [source]); await flushPromises()
    await view.get('form').trigger('submit'); await flushPromises()
    expect(view.get('.dialog').attributes('data-close-disabled')).toBe('true')
    expect(view.get('.archive-form footer .button--secondary').attributes('disabled')).toBeDefined()
    await view.get('.stub-close').trigger('click'); await flushPromises()
    expect(view.find('.archive-form').exists()).toBe(true)
    resolve(job); await flushPromises()
    expect(view.find('.archive-form').exists()).toBe(false)
  })

  it('caps manual member selection at 100 and disables oversized page selection', async () => {
    mocks.archiveContents.mockResolvedValue({
      path: source.path,
      resourceVersion: source.resourceVersion,
      directory: '',
      entries: Array.from({ length: 101 }, (_, index) => ({ path: `item-${index}`, name: `item-${index}`, kind: 'file', sizeBytes: 1 })),
      total: 101,
      truncated: false,
    })
    const view = await open(); view.vm.browse(source); await flushPromises()
    expect(view.get('thead input[type="checkbox"]').attributes('disabled')).toBeDefined()
    const rows = view.findAll('tbody input[type="checkbox"]')
    for (const row of rows.slice(0, 100)) await row.setValue(true)
    expect(rows[100]!.attributes('disabled')).toBeDefined()
    expect(view.get('.archive-toolbar .button--primary').text()).toContain('100')
    expect(view.get('.archive-selection-note').text()).toContain('100')
  })

  it('retries only unfinished sources and preserves member selection', async () => {
    mocks.archiveJobs.mockResolvedValue({ items: [{ ...job, state: 'error', archiveEntries: ['assets'], result: { action: 'extract', succeeded: [], failed: [{ path: source.path, detail: 'failed' }] } }] })
    const view = await open()
    const retry = view.findAll('.archive-job__actions button').find(button => button.text().includes('未完成'))!
    await retry.trigger('click'); await flushPromises()
    await view.get('form').trigger('submit'); await flushPromises()
    expect(mocks.entries).toHaveBeenCalledWith(['/site.zip'], undefined, 'host-a')
    expect(mocks.createArchiveJob).toHaveBeenCalledWith(expect.objectContaining({ archiveEntries: ['assets'], name: 'site', target: '/' }), 'host-a')
  })

  it('does not retry a cancelled extraction that already committed its result', async () => {
    mocks.archiveJobs.mockResolvedValue({ items: [{
      ...job,
      state: 'cancelled',
      result: { action: 'extract', succeeded: [{ path: source.path, destination: '/site' }], failed: [] },
    }] })
    const view = await open()
    expect(view.findAll('.archive-job__actions button').some(button => button.text().includes('未完成'))).toBe(false)
    expect(view.text()).toContain('打开位置')
  })

  it('stops a task on its captured host and never clears it while active', async () => {
    mocks.archiveJobs.mockResolvedValue({ items: [job] })
    const view = await open()
    expect(view.findAll('.archive-job__actions button')).toHaveLength(1)
    await view.get('.archive-job__actions button').trigger('click'); await flushPromises()
    expect(mocks.changeArchiveJob).toHaveBeenCalledWith(job.id, 'cancel', 'host-a')
  })

  it('polls active tasks and stops polling after a confirmed terminal result', async () => {
    mocks.archiveJobs.mockResolvedValueOnce({ items: [job] }).mockResolvedValue({ items: [{ ...job, state: 'complete', result: { action: 'extract', succeeded: [{ path: source.path, destination: '/site' }], failed: [] } }] })
    const view = await open()
    await vi.advanceTimersByTimeAsync(2500); await flushPromises()
    expect(view.emitted('changed')).toEqual([['host-a', '/']])
    await vi.advanceTimersByTimeAsync(15000)
    expect(mocks.archiveJobs).toHaveBeenCalledTimes(2)
  })
})
