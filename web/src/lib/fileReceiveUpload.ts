import type { FileEntry } from '@/types/api'
import { FileTransferHash } from './fileTransferHash'

export interface FileReceiveSession {
  id: string
  state: 'receiving' | 'committing' | 'complete'
  offset: number
  sizeBytes: number
  prefixSha256: string
  chunkBytes: number
  expiresAt: string
  entry?: FileEntry
}

export interface FileUploadReceiver {
  command: (body: Record<string, unknown>, signal?: AbortSignal) => Promise<FileReceiveSession>
  status: (id: string, sourceKey: string, signal?: AbortSignal) => Promise<FileReceiveSession>
  chunk: (id: string, sourceKey: string, offset: number, digest: string, blob: Blob, progress: (percent: number) => void, signal?: AbortSignal) => Promise<FileReceiveSession>
  legacy: () => Promise<FileEntry>
}

const chunkBytes = 8 << 20
const cache = new Map<string, string>()
const cachePrefix = 'kpanel:file-receive:'
function remember(key: string, id?: string): void {
  if (id) {
    if (cache.size >= 32 && !cache.has(key)) cache.delete(cache.keys().next().value!)
    cache.set(key, id)
  }
  else cache.delete(key)
  try {
    if (id) {
      const keys = Object.keys(sessionStorage).filter((item) => item.startsWith(cachePrefix))
      if (keys.length >= 32 && !keys.includes(cachePrefix + key)) sessionStorage.removeItem(keys[0]!)
      sessionStorage.setItem(cachePrefix + key, id)
    } else sessionStorage.removeItem(cachePrefix + key)
  } catch { /* Browsers may disable private session storage. */ }
}
function remembered(key: string): string | undefined {
  try { return sessionStorage.getItem(cachePrefix + key) || cache.get(key) } catch { return cache.get(key) }
}
function aborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw new DOMException('文件上传已取消。', 'AbortError')
}
async function readBlob(blob: Blob, signal?: AbortSignal): Promise<Uint8Array> {
  aborted(signal)
  if (typeof blob.arrayBuffer === 'function') return new Uint8Array(await blob.arrayBuffer())
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    const stop = (): void => reader.abort()
    const clean = (): void => signal?.removeEventListener('abort', stop)
    reader.onload = () => { clean(); resolve(new Uint8Array(reader.result as ArrayBuffer)) }
    reader.onerror = () => { clean(); reject(reader.error || new Error('无法读取本地文件。')) }
    reader.onabort = () => { clean(); reject(new DOMException('文件上传已取消。', 'AbortError')) }
    signal?.addEventListener('abort', stop, { once: true })
    reader.readAsArrayBuffer(blob)
  })
}
function statusOf(error: unknown): number | undefined {
  return error && typeof error === 'object' && 'status' in error ? Number(error.status) : undefined
}
function transient(error: unknown): boolean {
  return error instanceof TypeError || [0, 429, 502, 503, 504].includes(statusOf(error) ?? -1)
}
async function pause(attempt: number, signal?: AbortSignal): Promise<void> {
  aborted(signal)
  await new Promise<void>((resolve, reject) => {
    const stop = (): void => { clearTimeout(timer); reject(new DOMException('文件上传已取消。', 'AbortError')) }
    const timer = setTimeout(() => { signal?.removeEventListener('abort', stop); resolve() }, (attempt + 1) * 250)
    signal?.addEventListener('abort', stop, { once: true })
  })
}
async function retry<T>(operation: () => Promise<T>, signal?: AbortSignal): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    aborted(signal)
    try { return await operation() } catch (error) {
      if (!transient(error) || attempt === 2) throw error
      await pause(attempt, signal)
    }
  }
}

function validate(session: FileReceiveSession, file: File): void {
  if (!session || !/^[a-f0-9]{64}$/.test(session.id) || !/^[a-f0-9]{64}$/.test(session.prefixSha256)
    || !Number.isSafeInteger(session.offset) || session.offset < 0 || session.offset > file.size
    || session.sizeBytes !== file.size || session.chunkBytes !== chunkBytes
    || !['receiving', 'committing', 'complete'].includes(session.state)) throw new Error('面板返回了无效的传输进度。')
}

export async function uploadFileInSession(
  directory: string, file: File, overwrite: boolean, hostId: string,
  receiver: FileUploadReceiver, onProgress?: (percent: number) => void, signal?: AbortSignal,
): Promise<FileEntry> {
  aborted(signal)
  const identity = new FileTransferHash().update(new TextEncoder().encode(JSON.stringify([hostId, directory, file.name, file.size, file.lastModified, overwrite])))
  identity.update(await readBlob(file.slice(0, chunkBytes), signal))
  if (file.size > chunkBytes) identity.update(await readBlob(file.slice(Math.max(chunkBytes, file.size - chunkBytes)), signal))
  const sourceKey = identity.hex()
  aborted(signal)
  let id = remembered(sourceKey)
  let session: FileReceiveSession | undefined
  try {
    if (id) {
      try { session = await retry(() => receiver.status(id!, sourceKey, signal), signal) } catch (error) {
        if (statusOf(error) !== 404) throw error
        remember(sourceKey); id = undefined
      }
    }
    if (!session) {
      try {
        session = await receiver.command({ operation: 'create', input: {
          directory, name: file.name, kind: 'file', sizeBytes: file.size, sourceKey, overwrite,
          modifiedAt: new Date(file.lastModified).toISOString(),
        } }, signal)
      } catch (error) {
        if (statusOf(error) === 404 || statusOf(error) === 405) return receiver.legacy()
        throw error
      }
      validate(session, file)
      id = session.id
      remember(sourceKey, id)
    }
    validate(session, file)
    let current: FileReceiveSession = session
    const hasher = new FileTransferHash()
    for (let offset = 0; offset < session.offset; offset += chunkBytes) {
      aborted(signal)
      hasher.update(await readBlob(file.slice(offset, Math.min(offset + chunkBytes, session.offset)), signal))
    }
    if (hasher.hex() !== session.prefixSha256) throw new Error('已接收的文件内容发生变化，请重新上传。')
    onProgress?.(file.size ? Math.min(99, Math.round(session.offset / file.size * 100)) : 0)
    while (current.offset < file.size) {
      aborted(signal)
      const offset: number = current.offset
      const blob = file.slice(offset, Math.min(offset + chunkBytes, file.size))
      const data = await readBlob(blob, signal)
      const digest = new FileTransferHash().update(data).hex()
      const next: FileReceiveSession = await retry(() => receiver.chunk(id!, sourceKey, offset, digest, blob, (percent) => {
        onProgress?.(Math.min(99, Math.round((offset + blob.size * percent / 100) / file.size * 100)))
      }, signal), signal)
      hasher.update(data)
      validate(next, file)
      if (next.id !== id || next.offset !== offset + blob.size || next.prefixSha256 !== hasher.hex() || next.state !== 'receiving') throw new Error('文件分块校验失败，请重试。')
      current = next
    }
    aborted(signal)
    const complete = await retry(() => receiver.command({ operation: 'commit', id, sourceKey, sizeBytes: file.size, sha256: hasher.hex() }, signal), signal)
    validate(complete, file)
    const entry = complete.entry
    if (complete.id !== id || complete.state !== 'complete' || !entry || entry.kind !== 'file'
      || entry.path !== `${directory === '/' ? '' : directory}/${file.name}` || entry.sizeBytes !== file.size) throw new Error('文件提交结果无效，请刷新目录确认。')
    remember(sourceKey)
    onProgress?.(100)
    return entry
  } catch (error) {
    const detach = signal?.aborted && signal.reason === 'file-transfer-detach'
    if (id && !detach && !transient(error)) {
      remember(sourceKey)
      void receiver.command({ operation: 'abort', id, sourceKey }).catch(() => {})
    }
    if (signal?.aborted) throw new DOMException('文件上传已取消。', 'AbortError')
    throw error
  }
}
