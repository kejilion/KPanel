import { shallowRef } from 'vue'
import type { FileEntry } from '@/types/api'

export type FileConflictPolicy = 'overwrite' | 'keep-both' | 'skip'
export type ConflictSource = Pick<FileEntry, 'name' | 'kind'> & Partial<Pick<FileEntry, 'path' | 'sizeBytes' | 'modifiedAt'>>
export interface FileConflictPrompt {
  source: ConflictSource
  target: FileEntry
  host: string
  operation: string
  canOverwrite: boolean
  changed: boolean
  decide: (policy: FileConflictPolicy, applyToRest: boolean) => void
  cancel: () => void
}
export const pendingFileConflict = shallowRef<FileConflictPrompt>()
const prompts: Array<() => void> = []
let prompting = false
const batches = new Set<FileConflictBatch>()
export function cancelFileConflicts(): void { for (const batch of batches) batch.cancel() }
function nextPrompt(): void {
  if (prompting) return
  const next = prompts.shift()
  if (next) { prompting = true; next() }
}
export function cancelledFileBatch(): DOMException { return new DOMException('已取消本批后续文件操作。', 'AbortError') }
export function isFileBatchCancelled(error: unknown): boolean { return error instanceof DOMException && error.name === 'AbortError' }
export function joinFilePath(parent: string, name: string): string { return parent === '/' ? `/${name}` : `${parent}/${name}` }

export function copyFileName(name: string, attempt: number, kind = 'file'): string {
  if (!attempt) return name
  const compound = kind === 'file' ? name.match(/\.(tar\.(gz|bz2|xz|zst))$/i)?.[0] : undefined
  const dot = kind === 'directory' ? -1 : compound ? name.length - compound.length : name.lastIndexOf('.')
  const extension = dot > 0 ? name.slice(dot) : ''
  let stem = dot > 0 ? name.slice(0, dot) : name
  const suffix = ` (${attempt})${extension}`
  while (new TextEncoder().encode(stem + suffix).length > 255 && stem) stem = Array.from(stem).slice(0, -1).join('')
  if (!stem) throw new Error('文件扩展名过长，无法生成副本名称。')
  return stem + suffix
}

export async function existingFile(lookup: (path: string) => Promise<FileEntry>, path: string): Promise<FileEntry | undefined> {
  try { return await lookup(path) } catch (error) {
    if (error && typeof error === 'object' && 'status' in error && error.status === 404) return undefined
    throw error
  }
}

// A batch never shares its remembered choice with another window or operation.
// Only the prompt presentation is serialized, so concurrent batches cannot race
// to resolve a single dialog with each other's choices.
export class FileConflictBatch {
  readonly controller = new AbortController()
  readonly signal = this.controller.signal
  private policies: Partial<Record<string, FileConflictPolicy>> = {}
  private lastApply = false
  userCancelled = false
  readonly skipped: string[] = []
  constructor(readonly operation: string, readonly host: string, parentSignal?: AbortSignal) {
    batches.add(this)
    if (parentSignal?.aborted) this.cancel()
    else parentSignal?.addEventListener('abort', () => this.cancel(), { once: true, signal: this.signal })
  }
  cancel(byUser = false): void { this.userCancelled ||= byUser; this.controller.abort(); batches.delete(this) }
  close(): void { this.controller.abort(); batches.delete(this) }

  private ask(source: ConflictSource, target: FileEntry, canOverwrite: boolean, changed: boolean, force = false): Promise<FileConflictPolicy> {
    this.signal.throwIfAborted()
    if (prompts.length >= 64) return Promise.reject(new Error('等待处理的文件冲突过多，请先完成当前操作。'))
    return new Promise((resolve, reject) => {
      let settled = false
      let active = false
      const finish = (choice?: FileConflictPolicy, apply = false) => {
        if (settled) return
        settled = true
        this.signal.removeEventListener('abort', abort)
        if (active) { pendingFileConflict.value = undefined; prompting = false }
        else { const index = prompts.indexOf(show); if (index >= 0) prompts.splice(index, 1) }
        if (choice) { this.lastApply = apply; if (apply && (source.kind === target.kind || target.kind === ('missing' as string))) this.policies[source.kind] = choice; resolve(choice) } else reject(cancelledFileBatch())
        queueMicrotask(nextPrompt)
      }
      const abort = () => finish()
      const show = () => {
        active = true
        if (this.signal.aborted) { finish(); return }
        const remembered = this.policies[source.kind]
        if (!force && !changed && source.kind === target.kind && remembered && (remembered !== 'overwrite' || canOverwrite)) { finish(remembered); return }
        pendingFileConflict.value = { source, target, host: this.host, operation: this.operation, canOverwrite, changed,
          decide: (choice, apply) => { if (choice !== 'overwrite' || canOverwrite) finish(choice, apply) },
          cancel: () => this.cancel(true) }
      }
      this.signal.addEventListener('abort', abort, { once: true })
      prompts.push(show); nextPrompt()
    })
  }

  async prompt(source: ConflictSource, target: FileEntry, changed = false) {
    const policy = await this.ask(source, target, (target.kind === ('missing' as string) || source.kind === target.kind && (!source.path || source.path !== target.path)), changed, true)
    return { policy: policy === 'overwrite' && source.kind === 'directory' ? 'merge' as const : policy, applyToRest: this.lastApply }
  }

  async resolve(source: ConflictSource, directory: string, lookup: (path: string) => Promise<FileEntry>, changed = false) {
    this.signal.throwIfAborted()
    let target = await existingFile(lookup, joinFilePath(directory, source.name))
    if (!target && changed) target = {path:joinFilePath(directory,source.name),name:source.name,kind:'missing' as FileEntry['kind']} as FileEntry
    if (!target) return { name: source.name, overwrite: false, skipped: false, merge: false, version: undefined as string | undefined }
    const canOverwrite = target.kind === ('missing' as string) || (source.kind === 'file' && target.kind === 'file' || source.kind === 'directory' && target.kind === 'directory')
      && source.path !== target.path
    const remembered = this.policies[source.kind]
    const choice = !changed && source.kind === target.kind && remembered && (remembered !== 'overwrite' || canOverwrite)
      ? remembered : await this.ask(source, target, canOverwrite, changed)
    this.signal.throwIfAborted()
    if (choice === 'skip') {
      this.skipped.push(target.path)
      return { name: source.name, overwrite: false, skipped: true, merge: false, version: undefined as string | undefined }
    }
    if (choice === 'keep-both') {
      for (let attempt = 1; attempt <= 999; attempt++) {
        this.signal.throwIfAborted()
        const name = copyFileName(source.name, attempt, source.kind)
        if (!await existingFile(lookup, joinFilePath(directory, name))) return { name, overwrite: false, skipped: false, merge: false, version: undefined as string | undefined }
      }
      throw new Error('同名副本过多，请更换目标目录。')
    }
    return { name: source.name, overwrite: target.kind !== ('missing' as string), skipped: false, merge: target.kind === 'directory', version: target.resourceVersion, directoryIdentity: target.directoryIdentity }
  }
}
