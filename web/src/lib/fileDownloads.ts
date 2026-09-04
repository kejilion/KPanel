import { api, isRemoteFileHostSelected } from '@/lib/api'
import type { FileEntry } from '@/types/api'

export type FileDownloadEntry = Pick<FileEntry, 'name' | 'path' | 'kind' | 'resourceVersion'>

function safeDownloadName(name: string): string {
  const cleaned = name.replace(/[\u0000-\u001f<>:"/\\|?*]/g, '_').replace(/[ .]+$/u, '').trim()
  const characters = [...(cleaned || 'download')]
  const bounded = characters.length <= 180 ? characters.join('') : characters.slice(0, 180).join('')
  const stem = bounded.replace(/\..*$/u, '').toUpperCase()
  return /^(?:CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])$/.test(stem) ? `_${bounded}` : bounded
}

export function archiveDownloadName(
  entries: readonly FileDownloadEntry[],
  batchName: string,
): string | undefined {
  if (!entries.length || entries.some((entry) => entry.kind !== 'file' && entry.kind !== 'directory')) {
    return undefined
  }
  const sourceName = entries.length === 1 && entries[0]!.kind === 'directory'
    ? entries[0]!.name
    : batchName
  return `${safeDownloadName(sourceName).replace(/\.zip$/iu, '') || 'download'}.zip`
}

function triggerDownload(url: string, name: string): void {
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = name
  anchor.rel = 'noopener'
  document.body.appendChild(anchor)
  try {
    anchor.click()
  } finally {
    anchor.remove()
  }
}

async function downloadSingleFile(
  entry: FileDownloadEntry,
  fileHostId?: string | null,
): Promise<void> {
  const remoteHostSelected = fileHostId === undefined ? isRemoteFileHostSelected() : Boolean(fileHostId)
  if (remoteHostSelected) {
    const url = fileHostId === undefined
      ? api.files.contentUrl(entry.path, 'attachment')
      : api.files.contentUrl(entry.path, 'attachment', fileHostId)
    triggerDownload(url, entry.name)
    return
  }
  const ticket = await api.files.createDownloadTicket(entry.path)
  triggerDownload(ticket.downloadUrl, entry.name)
}

export async function downloadFileEntries(
  entries: readonly FileDownloadEntry[],
  batchName: string,
  fileHostId?: string | null,
): Promise<void> {
  if (!entries.length || entries.some((entry) => entry.kind !== 'file' && entry.kind !== 'directory')) {
    throw new TypeError('download selection must contain only files or directories')
  }
  if (entries.length === 1 && entries[0]!.kind === 'file') {
    await downloadSingleFile(entries[0]!, fileHostId)
    return
  }

  const archiveName = archiveDownloadName(entries, batchName)
  if (!archiveName) throw new TypeError('download archive name is unavailable')

  const remoteHostSelected = fileHostId === undefined ? isRemoteFileHostSelected() : Boolean(fileHostId)
  if (remoteHostSelected) {
    const url = fileHostId === undefined
      ? api.files.archiveUrl(entries, archiveName)
      : api.files.archiveUrl(entries, archiveName, fileHostId)
    triggerDownload(url, archiveName)
    return
  }

  const ticket = await api.files.createArchiveDownloadTicket(entries, archiveName)
  triggerDownload(ticket.downloadUrl, archiveName)
}
