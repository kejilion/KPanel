import type { FileEntry } from '@/types/api'
import type { FileUploadProgress } from './fileReceiveUpload'
import { FileConflictBatch, isFileBatchCancelled, joinFilePath } from './fileConflicts'
import type { FileActionInput, FileActionResult } from '@/types/api'
import type { ExternalDropManifest, DesktopExternalTransferProgress, DesktopExternalTransferResult } from './desktopExternalDrop'

export interface ConflictUploadAPI {
  resolveName?: (directory: string, name: string, kind: 'file' | 'directory') => Promise<string>
  entry: (path: string, signal?: AbortSignal) => Promise<FileEntry>
  uploadVersion: (path: string, file: File, overwrite: boolean, version?: string, onProgress?: FileUploadProgress, signal?: AbortSignal, directoryIdentity?: string) => Promise<FileEntry>
}
export async function uploadConflictFile(api: ConflictUploadAPI, directory: string, file: File, batch: FileConflictBatch, onProgress?: FileUploadProgress, directoryIdentity?: string): Promise<FileEntry | undefined> {
  let changed = false
  for (let attempt = 0; attempt < 8; attempt++) {
    batch.signal.throwIfAborted()
    const choice = await batch.resolve({ name: file.name, kind: 'file', sizeBytes: file.size, modifiedAt: new Date(file.lastModified).toISOString() }, directory, (path) => api.entry(path, batch.signal), changed)
    if (choice.skipped) return undefined
    if (choice.name !== file.name && api.resolveName) choice.name = await api.resolveName(directory,file.name,'file')
    const named = choice.name === file.name ? file : new File([file], choice.name, { type: file.type, lastModified: file.lastModified })
    try { return await api.uploadVersion(directory, named, choice.overwrite, choice.version, onProgress, batch.signal, directoryIdentity) } catch (error) {
      if (error && typeof error === 'object' && 'code' in error && error.code === 'file_directory_conflict') throw error
      if (!error || typeof error !== 'object' || !('status' in error) || error.status !== 409) throw error
      changed = true
    }
  }
  throw new Error('目标持续发生变化，本项未完成，请刷新后重试。')
}

export async function uploadDirectoryWithConflicts(
  api: ConflictUploadAPI & { action: (input: FileActionInput, signal?: AbortSignal) => Promise<FileActionResult> },
  destination: string, manifest: ExternalDropManifest, batch: FileConflictBatch,
  onProgress: (progress: DesktopExternalTransferProgress) => void,
): Promise<DesktopExternalTransferResult> {
  const paths = new Map<string, string | null>([['', destination]])
  const identities = new Map<string, string>()
  if (manifest.directories.length) {
    const base = await api.entry(destination, batch.signal)
    if (!base.directoryIdentity) throw new Error('目标主机需要升级后才能安全导入文件夹。')
    identities.set(destination, base.directoryIdentity)
  }
  const entries = new Map<string, Pick<FileEntry, 'name' | 'kind' | 'path'>>()
  const failed: DesktopExternalTransferResult['failed'] = []
  let completedFiles = 0; let loadedBytes = 0
  const loaded = new Map<number, number>()
  const report = (currentName: string) => onProgress({ completedFiles, totalFiles: manifest.files.length, loadedBytes, totalBytes: manifest.totalBytes, currentName })
  for (const segments of [...manifest.directories].sort((a, b) => a.length - b.length)) {
    if (batch.signal.aborted) break
    const key = segments.join('/'); const parent = paths.get(segments.slice(0, -1).join('/'))
    if (!parent) { paths.set(key, null); continue }
    const name = segments.at(-1)!
    try {
      let created = false
      for (let attempt = 0; attempt < 8; attempt++) {
        const choice = await batch.resolve({ name, kind: 'directory' }, parent, (path) => api.entry(path, batch.signal), attempt > 0)
        if (choice.skipped) { paths.set(key, null); created = true; break }
        if (choice.name !== name && api.resolveName) choice.name = await api.resolveName(parent,name,'directory')
        const path = joinFilePath(parent, choice.name)
        if (!choice.merge) {
          let outcome: FileActionResult
          try { outcome = await api.action({ action: 'mkdir', target: parent, name: choice.name, expectedDirectoryIdentity: identities.get(parent) }, batch.signal) } catch (error) { if (error && typeof error === 'object' && 'code' in error && error.code === 'file_directory_conflict') throw error; if (error && typeof error === 'object' && 'status' in error && error.status === 409) continue; throw error }
          if (outcome.failed.some(item => item.code === 'already_exists' || item.code === 'resource_conflict')) continue
          if (outcome.failed.length) throw new Error(outcome.failed[0]!.detail)
        }
        const current = await api.entry(path, batch.signal)
        if (current.kind !== 'directory' || choice.merge && (current.resourceVersion !== choice.version || !choice.directoryIdentity || current.directoryIdentity !== choice.directoryIdentity)) continue
        if (!current.directoryIdentity) throw new Error('目标主机需要升级后才能安全导入文件夹。')
        identities.set(path, current.directoryIdentity)
        paths.set(key, path)
        if (segments.length === 1) entries.set(segments[0]!, { name: choice.name, kind: 'directory', path })
        created = true; break
      }
      if (!created) throw new Error('目标文件夹持续发生变化，请刷新后重试。')
    } catch (error) {
      if (isFileBatchCancelled(error)) break
      paths.set(key, null); failed.push({ name: key, detail: error instanceof Error ? error.message : '创建文件夹失败' })
    }
  }
  let cursor = 0
  await Promise.all(Array.from({ length: Math.min(2, manifest.files.length) }, async () => {
    while (cursor < manifest.files.length && !batch.signal.aborted) {
      const index = cursor++; const item = manifest.files[index]!
      const parent = paths.get(item.segments.slice(0, -1).join('/'))
      if (!parent) continue
      try {
        const file = item.segments.at(-1) === item.file.name ? item.file : new File([item.file], item.segments.at(-1)!, { type: item.file.type, lastModified: item.file.lastModified })
        const entry = await uploadConflictFile(api, parent, file, batch, (percent, progress) => {
          const bytes = Math.min(file.size, progress?.loadedBytes ?? Math.round(file.size * percent / 100))
          loadedBytes += bytes - (loaded.get(index) || 0); loaded.set(index, bytes); report(file.name)
        }, identities.get(parent))
        if (entry) {
          loadedBytes += file.size - (loaded.get(index) || 0); loaded.set(index, file.size); completedFiles++
          if (item.segments.length === 1) entries.set(item.segments[0]!, entry)
        }
        report(file.name)
      } catch (error) {
        if (isFileBatchCancelled(error)) break
        failed.push({ name: item.file.name, detail: error instanceof Error ? error.message : '上传失败' })
      }
    }
  }))
  return { completedFiles, loadedBytes, entries: [...entries.values()], failed, skipped: [...batch.skipped], cancelled: batch.signal.aborted }
}
