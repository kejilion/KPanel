import type { Component } from 'vue'
import {
  Database,
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileKey,
  FileSpreadsheet,
  FileText,
  FileVideo,
  Folder,
  FolderOpen,
  Package,
  Presentation,
} from '@lucide/vue'
import type { FileEntry, FileKind } from '@/types/api'

export type FileIconKind =
  | 'folder'
  | 'image'
  | 'media'
  | 'archive'
  | 'spreadsheet'
  | 'database'
  | 'presentation'
  | 'package'
  | 'secret'
  | 'code'
  | 'document'
  | 'generic'

type FilePresentationInput = Pick<FileEntry, 'name' | 'kind' | 'mime' | 'editable' | 'previewable'>

const imageExtensions = ['avif', 'bmp', 'gif', 'heic', 'ico', 'jpeg', 'jpg', 'png', 'svg', 'webp']
const audioExtensions = ['aac', 'flac', 'm4a', 'mp3', 'ogg', 'opus', 'wav']
const videoExtensions = ['avi', 'm4v', 'mkv', 'mov', 'mp4', 'webm']

export function fileExtension(name: string): string {
  const normalized = name.toLocaleLowerCase()
  if (normalized.endsWith('.tar.gz')) return 'tar.gz'
  const separator = normalized.lastIndexOf('.')
  return separator >= 0 ? normalized.slice(separator + 1) : ''
}

export function fileEntryIconKind(entry: FilePresentationInput): FileIconKind {
  if (entry.kind === 'directory') return 'folder'
  const mime = (entry.mime || '').toLocaleLowerCase()
  const extension = fileExtension(entry.name)
  const normalizedName = entry.name.toLocaleLowerCase()
  if (mime.startsWith('image/') || imageExtensions.includes(extension)) return 'image'
  if (
    mime.startsWith('audio/')
    || mime.startsWith('video/')
    || audioExtensions.includes(extension)
    || videoExtensions.includes(extension)
  ) return 'media'
  if (
    ['tar.gz', 'tgz', 'zip', 'tar', '7z', 'rar', 'gz', 'bz2', 'xz'].includes(extension)
    || ['application/gzip', 'application/zip', 'application/x-7z-compressed', 'application/x-rar-compressed'].includes(mime)
  ) return 'archive'
  if (['csv', 'tsv', 'xls', 'xlsx', 'ods'].includes(extension)) return 'spreadsheet'
  if (['db', 'sqlite', 'sqlite3', 'mdb', 'sql'].includes(extension)) return 'database'
  if (['ppt', 'pptx', 'odp'].includes(extension)) return 'presentation'
  if (['deb', 'rpm', 'apk', 'pkg', 'msi'].includes(extension)) return 'package'
  if (
    ['env', 'pem', 'key', 'pfx', 'p12', 'crt', 'cer'].includes(extension)
    || /^(?:id_(?:rsa|ed25519|ecdsa)|authorized_keys|known_hosts)$/.test(normalizedName)
  ) return 'secret'
  if (
    ['json', 'yaml', 'yml', 'toml', 'xml', 'ini', 'conf', 'sh', 'bash', 'zsh', 'ps1', 'js', 'ts', 'vue', 'css', 'scss', 'html', 'go', 'py', 'php', 'java', 'c', 'h', 'cpp', 'rs'].includes(extension)
  ) return 'code'
  if (
    mime === 'application/pdf'
    || ['txt', 'md', 'log', 'pdf', 'doc', 'docx', 'odt', 'rtf'].includes(extension)
  ) return 'document'
  if (entry.editable) return 'code'
  if (entry.previewable) return 'document'
  return 'generic'
}

export function fileEntryIcon(entry: FilePresentationInput): Component {
  switch (fileEntryIconKind(entry)) {
    case 'folder': return Folder
    case 'image': return FileImage
    case 'media': return entry.mime?.startsWith('audio/') || audioExtensions.includes(fileExtension(entry.name))
      ? FileAudio
      : FileVideo
    case 'archive': return FileArchive
    case 'spreadsheet': return FileSpreadsheet
    case 'database': return Database
    case 'presentation': return Presentation
    case 'package': return Package
    case 'secret': return FileKey
    case 'code': return FileCode
    case 'document': return FileText
    default: return File
  }
}

export function shortcutFileIcon(name: string, kind: Extract<FileKind, 'file' | 'directory'>): Component {
  if (kind === 'directory') return FolderOpen
  return fileEntryIcon({ name, kind, editable: false, previewable: false })
}

// Shared body and highlight colors for file browser glyphs and desktop shortcut tiles.
export const fileIconPalette: Record<FileIconKind, readonly [body: string, highlight: string]> = {
  folder: ['#e6af42', '#f7d77c'],
  image: ['#9683c6', '#d2c6ed'],
  media: ['#bf7998', '#ecc0d3'],
  archive: ['#ce9955', '#efd0a2'],
  spreadsheet: ['#589c7c', '#b5d9c4'],
  database: ['#579eaa', '#b8dce0'],
  presentation: ['#ce8466', '#f1c8b5'],
  package: ['#bb905e', '#e6c59c'],
  secret: ['#658d91', '#c1dbdc'],
  code: ['#648fbe', '#bfd6ed'],
  document: ['#7d9cbb', '#d0e0ee'],
  generic: ['#929fad', '#dde4eb'],
}

export function shortcutFileGradient(
  name: string,
  kind: Extract<FileKind, 'file' | 'directory'>,
): string {
  const iconKind = fileEntryIconKind({ name, kind, editable: false, previewable: false })
  const [body, highlight] = fileIconPalette[iconKind]
  return `linear-gradient(145deg, ${highlight} 0%, ${body} 100%)`
}
