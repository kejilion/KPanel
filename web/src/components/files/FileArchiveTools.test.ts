// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FileArchiveTools from './FileArchiveTools.vue'
import type { FileArchiveJob, FileEntry } from '@/types/api'

const mocks = vi.hoisted(() => ({ archiveJobs: vi.fn(), archiveContents: vi.fn(), createArchiveJob: vi.fn(), changeArchiveJob: vi.fn(), entries: vi.fn() }))
vi.mock('@/lib/api', () => ({ ApiError: class extends Error {}, api: { files: mocks } }))
const source = { name: 'site.zip', path: '/site.zip', resourceVersion: 'sha256:source', kind: 'file' } as FileEntry
const job: FileArchiveJob = { id: 'a'.repeat(32), action: 'extract', name: 'site', target: '/', sources: ['/site.zip'], state: 'queued', entries: 0, processedBytes: 0, createdAt: '2026-09-09T00:00:00Z', updatedAt: '2026-09-09T00:00:00Z', result: { action: 'extract', succeeded: [], failed: [] } }
let wrapper: ReturnType<typeof mount<typeof FileArchiveTools>> | undefined
async function open() {
  wrapper = mount(FileArchiveTools, { props: { hostId: 'host-a', path: '/destination' }, global: { stubs: { ModalDialog: { props: ['open'], template: '<div v-if="open" class="dialog"><slot /></div>' } } } })
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
    await vi.advanceTimersByTimeAsync(10000); await flushPromises()
    expect(view.vm.available).toBe(true)
    expect(view.find('.archive-job').exists()).toBe(true)
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

  it('retries only unfinished sources and preserves member selection', async () => {
    mocks.archiveJobs.mockResolvedValue({ items: [{ ...job, state: 'error', archiveEntries: ['assets'], result: { action: 'extract', succeeded: [], failed: [{ path: source.path, detail: 'failed' }] } }] })
    const view = await open()
    const retry = view.findAll('.archive-job__actions button').find(button => button.text().includes('重试'))!
    await retry.trigger('click'); await flushPromises()
    await view.get('form').trigger('submit'); await flushPromises()
    expect(mocks.entries).toHaveBeenCalledWith(['/site.zip'], undefined, 'host-a')
    expect(mocks.createArchiveJob).toHaveBeenCalledWith(expect.objectContaining({ archiveEntries: ['assets'], name: 'site', target: '/' }), 'host-a')
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
