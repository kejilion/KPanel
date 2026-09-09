import { api } from '@/lib/api'

/** A snapshot for one window/operation. Omitted host IDs always mean this Panel. */
export function fileAPIForHost(hostId = '') {
  const files = api.files
  return {
    entry: (path: string, signal?: AbortSignal) => files.entry(path, signal, hostId),
    entries: (paths: readonly string[], signal?: AbortSignal) => files.entries(paths, signal, hostId),
    list: (path: string, options?: Parameters<typeof files.list>[1], signal?: AbortSignal) =>
      files.list(path, options, signal, hostId),
    contentUrl: (path: string, disposition?: 'inline' | 'attachment') => files.contentUrl(path, disposition, hostId),
    archiveUrl: (entries: Parameters<typeof files.archiveUrl>[0], name: string) => files.archiveUrl(entries, name, hostId),
    thumbnailUrl: (path: string, version: string) => files.thumbnailUrl(path, version, hostId),
    text: (path: string) => files.text(path, hostId),
    write: (path: string, content: string, version: string) => files.write(path, content, version, hostId),
    action: (input: Parameters<typeof files.action>[0], signal?: AbortSignal) => files.action(input, signal, hostId),
    trash: () => files.trash(hostId),
    upload: (path: string, file: File, overwrite = false, onProgress?: (percent: number) => void, signal?: AbortSignal) =>
      files.upload(path, file, overwrite, onProgress, signal, hostId),
    transferFromPanel: (input: Parameters<typeof files.transferFromPanel>[0], onEvent: Parameters<typeof files.transferFromPanel>[1], signal?: AbortSignal) =>
      files.transferFromPanel(input, onEvent, signal, hostId),
  }
}
