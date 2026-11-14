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
    archiveMemberUrl: (entry: Parameters<typeof files.archiveMemberUrl>[0], members: readonly string[], name: string, raw = false) => files.archiveMemberUrl(entry, members, name, raw, hostId),
    archiveContents: (input: Parameters<typeof files.archiveContents>[0], signal?: AbortSignal) => files.archiveContents(input, signal, hostId),
    archiveJobs: (signal?: AbortSignal) => files.archiveJobs(signal, hostId),
    createArchiveJob: (input: Parameters<typeof files.createArchiveJob>[0]) => files.createArchiveJob(input, hostId),
    decideArchiveJob: (id: string, decision: Parameters<typeof files.decideArchiveJob>[1]) => files.decideArchiveJob(id, decision, hostId),
    resolveName: async (directory: string, name: string, kind: 'file' | 'directory') => {
      const result = await files.action({action:'resolve',target:directory,name,sourceKind:kind,conflictPolicies:{file:'keep-both',directory:'keep-both'}}, undefined, hostId)
      if (!result.resolution?.name) throw new Error('目标主机不支持统一同名处理。')
      return result.resolution.name
    },
    changeArchiveJob: (id: string, operation: 'cancel' | 'clear') => files.changeArchiveJob(id, operation, hostId),
    previewUrl: (path: string, version: string) => files.previewUrl(path, version, hostId),
    thumbnailUrl: (path: string, version: string) => files.thumbnailUrl(path, version, hostId),
    text: (path: string, signal?: AbortSignal) => files.text(path, hostId, signal),
    office: (path: string, signal?: AbortSignal) => files.office(path, hostId, signal),
    writeOffice: (path: string, edits: Parameters<typeof files.writeOffice>[1], version: string, contentVersion: string) =>
      files.writeOffice(path, edits, version, contentVersion, hostId),
    write: (path: string, content: string, version: string) => files.write(path, content, version, hostId),
    action: (input: Parameters<typeof files.action>[0], signal?: AbortSignal) => files.action(input, signal, hostId),
    trash: () => files.trash(hostId),
    upload: (path: string, file: File, overwrite = false, onProgress?: (percent: number) => void, signal?: AbortSignal) =>
      files.upload(path, file, overwrite, onProgress, signal, hostId),
    uploadVersion: (path: string, file: File, overwrite: boolean, version?: string, onProgress?: Parameters<typeof files.upload>[3], signal?: AbortSignal) =>
      version === undefined ? files.upload(path, file, overwrite, onProgress, signal, hostId) : files.upload(path, file, overwrite, onProgress, signal, hostId, version),
    transferFromPanel: (input: Parameters<typeof files.transferFromPanel>[0], onEvent: Parameters<typeof files.transferFromPanel>[1], signal?: AbortSignal) =>
      files.transferFromPanel({ ...input, background: true }, onEvent, signal, hostId),
  }
}
