import { describe, expect, it, vi } from 'vitest'
import type { FileEntry, FileRemoteDownloadJob } from '@/types/api'
import { watchFileTransferJob } from './fileTransferJob'

const job = { id: 'a'.repeat(32), state: 'transferring', sourceKind: 'cross-host', source: 'kpanel://source', targetDirectory: '/home', targetHostId: 'target' } as FileRemoteDownloadJob
describe('background transfer observation', () => {
  it('detaches on close without cancelling the accepted transfer', async () => {
    const controller = new AbortController()
    controller.abort('file-transfer-detach')
    const watcher = { get: vi.fn(), cancel: vi.fn() }
    await expect(watchFileTransferJob(job, watcher, vi.fn(), controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    expect(watcher.cancel).not.toHaveBeenCalled()
  })
  it('cancels on explicit user cancellation and preserves a racing completed result', async () => {
    const entry = { name: 'app', path: '/home/app', kind: 'directory' } as FileEntry
    const controller = new AbortController()
    controller.abort()
    const watcher = { get: vi.fn(async () => ({ ...job, state: 'complete', entry } as FileRemoteDownloadJob)), cancel: vi.fn(async () => undefined) }
    await expect(watchFileTransferJob(job, watcher, vi.fn(), controller.signal)).resolves.toBe(entry)
    expect(watcher.cancel).toHaveBeenCalledWith(job.id)
  })
  it('rejects a substituted target before reporting any completed entry', async () => {
    const replaced = { ...job, targetHostId: 'different' }
    vi.useFakeTimers()
    const operation = watchFileTransferJob(job, { get: vi.fn(async () => replaced), cancel: vi.fn() }, vi.fn())
    const assertion = expect(operation).rejects.toThrow('无效的传输任务')
    await vi.runAllTimersAsync()
    await assertion
    vi.useRealTimers()
  })
})
