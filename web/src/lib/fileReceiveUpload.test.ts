import { afterEach, describe, expect, it, vi } from 'vitest'
import type { FileEntry } from '@/types/api'
import { uploadFileInSession, type FileReceiveSession, type FileUploadReceiver } from './fileReceiveUpload'
import { FileTransferHash } from './fileTransferHash'

const id = 'a'.repeat(64)
const entry = { name: 'data.bin', path: '/home/data.bin', kind: 'file', sizeBytes: 6 } as FileEntry
function session(offset: number, data: string, state: FileReceiveSession['state'] = 'receiving'): FileReceiveSession {
  return { id, offset, state, sizeBytes: 6, prefixSha256: new FileTransferHash().update(new TextEncoder().encode(data)).hex(), chunkBytes: 8 << 20, expiresAt: '2030-01-01T00:00:00Z', ...(state === 'complete' ? { entry } : {}) }
}
function receiver(): FileUploadReceiver {
  return {
    command: vi.fn(async (input) => input.operation === 'commit' ? session(6, 'abcdef', 'complete') : session(0, '')),
    status: vi.fn(async () => session(3, 'abc')),
    chunk: vi.fn(async () => session(6, 'abcdef')),
    legacy: vi.fn(async () => entry),
  }
}
afterEach(() => { vi.useRealTimers(); if (typeof sessionStorage !== 'undefined') sessionStorage.clear() })

describe('receiving session upload', () => {
  it('replays an unacknowledged chunk and commits once before reporting 100 percent', async () => {
    vi.useFakeTimers()
    const target = receiver()
    const chunk = target.chunk as ReturnType<typeof vi.fn>
    chunk.mockRejectedValueOnce({ status: 0, code: 'network_error' })
    const progress = vi.fn()
    const operation = uploadFileInSession('/home', new File(['abcdef'], 'data.bin', { lastModified: 1234 }), false, 'host-a', target, progress)
    await vi.runAllTimersAsync()
    await expect(operation).resolves.toBe(entry)
    expect(chunk).toHaveBeenCalledTimes(2)
    expect(chunk.mock.calls[0]!.slice(0, 4)).toEqual(chunk.mock.calls[1]!.slice(0, 4))
    expect(progress).toHaveBeenLastCalledWith(100)
    expect(target.command).toHaveBeenLastCalledWith(expect.objectContaining({ operation: 'commit', sizeBytes: 6 }), undefined)
  })

  it('retains a detached view checkpoint and verifies its local prefix on reselect', async () => {
    const target = receiver()
    const file = new File(['abcdef'], 'data.bin', { lastModified: 5678 })
    const controller = new AbortController()
    target.chunk = vi.fn(async () => { controller.abort('file-transfer-detach'); throw new DOMException('Detached', 'AbortError') })
    await expect(uploadFileInSession('/home', file, false, 'host-detach', target, undefined, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    expect(target.command).toHaveBeenCalledTimes(1)
    target.chunk = vi.fn(async () => session(6, 'abcdef'))
    await expect(uploadFileInSession('/home', file, false, 'host-detach', target)).resolves.toBe(entry)
    expect(target.status).toHaveBeenCalledTimes(1)
    expect(target.chunk).toHaveBeenCalledWith(id, expect.any(String), 3, expect.any(String), expect.any(Blob), expect.any(Function), undefined)
  })

  it('rejects a changed durable prefix and never commits it', async () => {
    const target = receiver()
    const file = new File(['abcdef'], 'data.bin', { lastModified: 91011 })
    const controller = new AbortController()
    target.chunk = vi.fn(async () => { controller.abort('file-transfer-detach'); throw new DOMException('Detached', 'AbortError') })
    await expect(uploadFileInSession('/home', file, false, 'host-changed', target, undefined, controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    target.status = vi.fn(async () => session(3, 'bad'))
    await expect(uploadFileInSession('/home', file, false, 'host-changed', target)).rejects.toThrow('内容发生变化')
    expect(target.command).toHaveBeenLastCalledWith(expect.objectContaining({ operation: 'abort', id }))
  })

  it('falls back only when the receiving route is absent', async () => {
    const target = receiver()
    target.command = vi.fn(async () => { throw { status: 404 } })
    await expect(uploadFileInSession('/home', new File(['abcdef'], 'data.bin'), false, 'old-host', target)).resolves.toBe(entry)
    expect(target.legacy).toHaveBeenCalledTimes(1)
    target.command = vi.fn(async () => { throw { status: 403 } })
    await expect(uploadFileInSession('/home', new File(['abcdef'], 'data.bin'), false, 'denied-host', target)).rejects.toMatchObject({ status: 403 })
    expect(target.legacy).toHaveBeenCalledTimes(1)
  })
})
