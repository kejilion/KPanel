import type { FileActionInput, FileActionResult, FileEntry } from '@/types/api'

export const DESKTOP_UPLOAD_DIRECTORY = '/home/KPanel Desktop'
export const MAX_EXTERNAL_DROP_FILES = 500
export const MAX_EXTERNAL_DROP_DIRECTORIES = 256
export const MAX_EXTERNAL_DROP_DEPTH = 32
export const MAX_EXTERNAL_DROP_BYTES = 2 * 1024 * 1024 * 1024
export const MAX_EXTERNAL_DROP_FILE_BYTES = 512 * 1024 * 1024

export type ExternalDropRootKind = 'file' | 'directory'

export interface ExternalDropFile {
  file: File
  segments: string[]
}

export interface ExternalDropRoot {
  name: string
  kind: ExternalDropRootKind
}

export interface ExternalDropManifest {
  roots: ExternalDropRoot[]
  directories: string[][]
  files: ExternalDropFile[]
  totalBytes: number
}

export interface DesktopExternalTransferProgress {
  completedFiles: number
  totalFiles: number
  loadedBytes: number
  totalBytes: number
  currentName: string
}

export interface DesktopExternalTransferResult {
  entries: Array<Pick<FileEntry, 'name' | 'path' | 'kind'>>
  failed: Array<{ name: string; detail: string }>
}

export interface DesktopExternalTransferAPI {
  entry: (path: string, signal?: AbortSignal) => Promise<FileEntry>
  action: (input: FileActionInput, signal?: AbortSignal) => Promise<FileActionResult>
  upload: (
    path: string,
    file: File,
    overwrite: boolean,
    onProgress?: (percent: number) => void,
    signal?: AbortSignal,
  ) => Promise<FileEntry>
}

interface LegacyFileSystemEntry {
  isFile: boolean
  isDirectory: boolean
  name: string
}

interface LegacyFileSystemFileEntry extends LegacyFileSystemEntry {
  file: (success: (file: File) => void, failure?: (error: DOMException) => void) => void
}

interface LegacyFileSystemDirectoryEntry extends LegacyFileSystemEntry {
  createReader: () => {
    readEntries: (
      success: (entries: LegacyFileSystemEntry[]) => void,
      failure?: (error: DOMException) => void,
    ) => void
  }
}

type DataTransferItemWithEntry = DataTransferItem & {
  webkitGetAsEntry?: () => LegacyFileSystemEntry | null
  getAsEntry?: () => LegacyFileSystemEntry | null
}

export class DesktopExternalDropError extends Error {
  constructor(readonly code: 'unsupported' | 'invalid' | 'too_many' | 'too_large' | 'too_deep', message: string) {
    super(message)
  }
}

function validSegment(value: string): boolean {
  return Boolean(value)
    && value !== '.'
    && value !== '..'
    && new TextEncoder().encode(value).length <= 255
    && !/[\u0000-\u001f\u007f/\\]/.test(value)
    && !value.startsWith('.kpanel-edit-')
    && !value.startsWith('.kpanel-upload-')
    && !value.startsWith('.kpanel-copy-')
    && !value.startsWith('.kpanel-archive-')
    && !value.startsWith('.kpanel-extract-')
}

function checkedSegments(values: readonly string[]): string[] {
  if (!values.length || values.some((value) => !validSegment(value))) {
    throw new DesktopExternalDropError('invalid', '文件或目录名称不受支持。')
  }
  const path = `/${values.join('/')}`
  if (new TextEncoder().encode(path).length > 4096) {
    throw new DesktopExternalDropError('invalid', '文件路径过长。')
  }
  return [...values]
}

function assertManifestLimits(manifest: ExternalDropManifest, depth: number): void {
  if (depth > MAX_EXTERNAL_DROP_DEPTH) {
    throw new DesktopExternalDropError('too_deep', `目录层级不能超过 ${MAX_EXTERNAL_DROP_DEPTH} 层。`)
  }
  if (manifest.files.length > MAX_EXTERNAL_DROP_FILES || manifest.directories.length > MAX_EXTERNAL_DROP_DIRECTORIES) {
    throw new DesktopExternalDropError(
      'too_many',
      `每次最多拖入 ${MAX_EXTERNAL_DROP_FILES} 个文件和 ${MAX_EXTERNAL_DROP_DIRECTORIES} 个目录。`,
    )
  }
  if (manifest.totalBytes > MAX_EXTERNAL_DROP_BYTES) {
    throw new DesktopExternalDropError('too_large', '单次拖入的文件总量不能超过 2 GiB。')
  }
}

function readEntryFile(entry: LegacyFileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject))
}

function readDirectoryEntries(entry: LegacyFileSystemDirectoryEntry): Promise<LegacyFileSystemEntry[]> {
  const reader = entry.createReader()
  const result: LegacyFileSystemEntry[] = []
  return new Promise((resolve, reject) => {
    let settled = false
    const readNext = (): void => {
      reader.readEntries((entries) => {
        if (settled) return
        if (!entries.length) {
          settled = true
          resolve(result)
          return
        }
        if (result.length + entries.length > MAX_EXTERNAL_DROP_FILES + MAX_EXTERNAL_DROP_DIRECTORIES) {
          settled = true
          reject(new DesktopExternalDropError(
            'too_many',
            `每次最多拖入 ${MAX_EXTERNAL_DROP_FILES} 个文件和 ${MAX_EXTERNAL_DROP_DIRECTORIES} 个目录。`,
          ))
          return
        }
        result.push(...entries)
        readNext()
      }, (error) => {
        if (settled) return
        settled = true
        reject(error)
      })
    }
    readNext()
  })
}

async function walkEntry(
  entry: LegacyFileSystemEntry,
  parent: readonly string[],
  manifest: ExternalDropManifest,
  signal?: AbortSignal,
): Promise<void> {
  signal?.throwIfAborted()
  const segments = checkedSegments([...parent, entry.name])
  assertManifestLimits(manifest, segments.length)
  if (entry.isFile) {
    const file = await readEntryFile(entry as LegacyFileSystemFileEntry)
    signal?.throwIfAborted()
    if (file.size > MAX_EXTERNAL_DROP_FILE_BYTES) {
      throw new DesktopExternalDropError('too_large', `${file.name} 超过 512 MiB。`)
    }
    manifest.files.push({ file, segments })
    manifest.totalBytes += file.size
    assertManifestLimits(manifest, segments.length)
    return
  }
  if (!entry.isDirectory) {
    throw new DesktopExternalDropError('unsupported', `${entry.name} 不是可上传的文件或目录。`)
  }
  manifest.directories.push(segments)
  assertManifestLimits(manifest, segments.length)
  const children = await readDirectoryEntries(entry as LegacyFileSystemDirectoryEntry)
  signal?.throwIfAborted()
  for (const child of children) await walkEntry(child, segments, manifest, signal)
}

function addFallbackFile(file: File, manifest: ExternalDropManifest): void {
  const relativePath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name
  const segments = checkedSegments(relativePath.split('/').filter(Boolean))
  if (segments.length > 1) {
    for (let index = 1; index < segments.length; index += 1) {
      const directory = segments.slice(0, index)
      if (!manifest.directories.some((value) => value.join('\0') === directory.join('\0'))) {
        manifest.directories.push(directory)
      }
    }
  }
  if (file.size > MAX_EXTERNAL_DROP_FILE_BYTES) {
    throw new DesktopExternalDropError('too_large', `${file.name} 超过 512 MiB。`)
  }
  manifest.files.push({ file, segments })
  manifest.totalBytes += file.size
  assertManifestLimits(manifest, segments.length)
}

export function hasExternalFileDrop(event: DragEvent): boolean {
  return Array.from(event.dataTransfer?.types || []).includes('Files')
}

export async function collectExternalDrop(dataTransfer: DataTransfer, signal?: AbortSignal): Promise<ExternalDropManifest> {
  signal?.throwIfAborted()
  const manifest: ExternalDropManifest = { roots: [], directories: [], files: [], totalBytes: 0 }
  const entries = Array.from(dataTransfer.items || []).flatMap((item) => {
    if (item.kind !== 'file') return []
    const typed = item as DataTransferItemWithEntry
    const entry = typed.getAsEntry?.() || typed.webkitGetAsEntry?.()
    return entry ? [entry as unknown as LegacyFileSystemEntry] : []
  })
  if (entries.length > MAX_EXTERNAL_DROP_FILES + MAX_EXTERNAL_DROP_DIRECTORIES) {
    throw new DesktopExternalDropError(
      'too_many',
      `每次最多拖入 ${MAX_EXTERNAL_DROP_FILES} 个文件和 ${MAX_EXTERNAL_DROP_DIRECTORIES} 个目录。`,
    )
  }

  if (entries.length) {
    const rootNames = new Set<string>()
    for (const entry of entries) {
      if (rootNames.has(entry.name)) {
        throw new DesktopExternalDropError('invalid', `拖入内容包含多个名为 ${entry.name} 的顶层项目，请分开拖入。`)
      }
      rootNames.add(entry.name)
      manifest.roots.push({ name: entry.name, kind: entry.isDirectory ? 'directory' : 'file' })
      await walkEntry(entry, [], manifest, signal)
    }
  } else {
    const files = dataTransfer.files || []
    if (files.length > MAX_EXTERNAL_DROP_FILES) {
      throw new DesktopExternalDropError('too_many', `每次最多拖入 ${MAX_EXTERNAL_DROP_FILES} 个文件。`)
    }
    for (const file of Array.from(files)) {
      signal?.throwIfAborted()
      addFallbackFile(file, manifest)
    }
    const roots = new Map<string, ExternalDropRootKind>()
    for (const directory of manifest.directories) roots.set(directory[0]!, 'directory')
    for (const value of manifest.files) {
      if (!roots.has(value.segments[0]!)) roots.set(value.segments[0]!, value.segments.length > 1 ? 'directory' : 'file')
    }
    manifest.roots = [...roots].map(([name, kind]) => ({ name, kind }))
  }
  if (!manifest.roots.length) throw new DesktopExternalDropError('unsupported', '没有检测到可上传的文件或目录。')
  return manifest
}

function joinPath(parent: string, name: string): string {
  return parent === '/' ? `/${name}` : `${parent}/${name}`
}

function suffixedName(name: string, attempt: number): string {
  if (attempt === 0) return name
  const dot = name.lastIndexOf('.')
  const hasExtension = dot > 0 && dot < name.length - 1
  const stem = hasExtension ? name.slice(0, dot) : name
  const extension = hasExtension ? name.slice(dot) : ''
  const suffix = ` (${attempt})`
  const maxStemBytes = Math.max(1, 255 - new TextEncoder().encode(`${suffix}${extension}`).length)
  let value = stem
  while (new TextEncoder().encode(value).length > maxStemBytes) value = value.slice(0, -1)
  return `${value}${suffix}${extension}`
}

async function pathExists(api: DesktopExternalTransferAPI, path: string, signal: AbortSignal): Promise<boolean> {
  try {
    await api.entry(path, signal)
    return true
  } catch (error) {
    if (typeof error === 'object' && error && 'status' in error && error.status === 404) return false
    throw error
  }
}

async function uniqueName(
  api: DesktopExternalTransferAPI,
  directory: string,
  original: string,
  signal: AbortSignal,
): Promise<string> {
  for (let attempt = 0; attempt <= 999; attempt += 1) {
    const candidate = suffixedName(original, attempt)
    if (!await pathExists(api, joinPath(directory, candidate), signal)) return candidate
  }
  throw new DesktopExternalDropError('invalid', `${original} 的同名副本过多。`)
}

async function createDirectory(
  api: DesktopExternalTransferAPI,
  parent: string,
  name: string,
  signal: AbortSignal,
): Promise<string> {
  const result = await api.action({ action: 'mkdir', target: parent, name }, signal)
  if (result.failed.length) throw new Error(result.failed[0]?.detail || '目录创建失败。')
  return joinPath(parent, name)
}

async function ensureDestinationDirectory(
  api: DesktopExternalTransferAPI,
  destination: string,
  signal: AbortSignal,
): Promise<void> {
  if (await pathExists(api, destination, signal)) {
    const entry = await api.entry(destination, signal)
    if (entry.kind !== 'directory') throw new Error(`${destination} 已存在，但不是目录。`)
    return
  }
  if (destination !== DESKTOP_UPLOAD_DIRECTORY) throw new Error(`${destination} 不存在或无法访问。`)
  await createDirectory(api, '/home', 'KPanel Desktop', signal)
}

export async function uploadExternalDrop(
  manifest: ExternalDropManifest,
  api: DesktopExternalTransferAPI,
  signal: AbortSignal,
  onProgress: (progress: DesktopExternalTransferProgress) => void,
  destination = DESKTOP_UPLOAD_DIRECTORY,
): Promise<DesktopExternalTransferResult> {
  await ensureDestinationDirectory(api, destination, signal)
  const rootNames = new Map<string, string>()
  for (const root of manifest.roots) {
    rootNames.set(root.name, await uniqueName(api, destination, root.name, signal))
  }
  const mappedSegments = (segments: readonly string[]): string[] => [rootNames.get(segments[0]!)!, ...segments.slice(1)]
  const directoryPaths = new Set<string>()
  for (const directory of [...manifest.directories].sort((left, right) => left.length - right.length)) {
    const mapped = mappedSegments(directory)
    let parent = destination
    for (const name of mapped) {
      const path = joinPath(parent, name)
      if (!directoryPaths.has(path)) {
        await createDirectory(api, parent, name, signal)
        directoryPaths.add(path)
      }
      parent = path
    }
  }

  const loadedByFile = new Map<number, number>()
  const failedRootNames = new Set<string>()
  let completedFiles = 0
  let loadedBytes = 0
  const failed: DesktopExternalTransferResult['failed'] = []
  const report = (currentName: string): void => onProgress({
    completedFiles,
    totalFiles: manifest.files.length,
    loadedBytes,
    totalBytes: manifest.totalBytes,
    currentName,
  })
  report(manifest.roots[0]?.name || '')

  let cursor = 0
  const workers = Array.from({ length: Math.min(2, Math.max(1, manifest.files.length)) }, async () => {
    while (cursor < manifest.files.length) {
      const index = cursor
      cursor += 1
      const item = manifest.files[index]!
      const mapped = mappedSegments(item.segments)
      const fileName = mapped.at(-1)!
      const parent = mapped.length === 1
        ? destination
        : joinPath(destination, mapped.slice(0, -1).join('/'))
      try {
        const uploadFile = fileName === item.file.name
          ? item.file
          : new File([item.file], fileName, { type: item.file.type, lastModified: item.file.lastModified })
        await api.upload(parent, uploadFile, false, (percent) => {
          const next = Math.round(item.file.size * percent / 100)
          loadedBytes += next - (loadedByFile.get(index) || 0)
          loadedByFile.set(index, next)
          report(item.file.name)
        }, signal)
        loadedBytes += item.file.size - (loadedByFile.get(index) || 0)
        loadedByFile.set(index, item.file.size)
        completedFiles += 1
        report(item.file.name)
      } catch (error) {
        if (signal.aborted) throw error
        failedRootNames.add(item.segments[0]!)
        failed.push({ name: item.file.name, detail: error instanceof Error ? error.message : '上传失败。' })
        report(item.file.name)
      }
    }
  })
  await Promise.all(workers)

  const entries = manifest.roots.flatMap((root) => {
    if (root.kind === 'file' && failedRootNames.has(root.name)) return []
    return [{
      name: rootNames.get(root.name)!,
      path: joinPath(destination, rootNames.get(root.name)!),
      kind: root.kind,
    } satisfies Pick<FileEntry, 'name' | 'path' | 'kind'>]
  })
  return { entries, failed }
}
