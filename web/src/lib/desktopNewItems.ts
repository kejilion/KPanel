import type { ExternalDropManifest } from '@/lib/desktopExternalDrop'

export type DesktopNewItemID = 'folder' | 'txt' | 'md'

export interface DesktopNewItemType {
  id: DesktopNewItemID
  kind: 'file' | 'directory'
  extension: string
  mime: string
  /** Seeded into the file body; empty for plain text. */
  template: string
}

// Add a row here to offer another type in the desktop "New" menu.
export const DESKTOP_NEW_ITEM_TYPES: readonly DesktopNewItemType[] = [
  { id: 'folder', kind: 'directory', extension: '', mime: '', template: '' },
  { id: 'txt', kind: 'file', extension: '.txt', mime: 'text/plain', template: '' },
  { id: 'md', kind: 'file', extension: '.md', mime: 'text/markdown', template: '' },
]

export function desktopNewItemType(id: DesktopNewItemID): DesktopNewItemType {
  return DESKTOP_NEW_ITEM_TYPES.find((type) => type.id === id) ?? DESKTOP_NEW_ITEM_TYPES[0]!
}

/** Returns the final file name, or undefined when the typed name is unusable. */
export function desktopNewItemName(type: DesktopNewItemType, input: string): string | undefined {
  let name = input.trim()
  if (!name || name === '.' || name === '..') return undefined
  if (/[\u0000-\u001f\u007f\/\\]/.test(name) || name.startsWith('.kpanel-')) return undefined
  if (type.extension && !name.toLowerCase().endsWith(type.extension)) name += type.extension
  return new TextEncoder().encode(name).length <= 255 ? name : undefined
}

/** One-root manifest so creation reuses the drop pipeline (dedupe, mkdir, upload). */
export function desktopNewItemManifest(type: DesktopNewItemType, name: string): ExternalDropManifest {
  if (type.kind === 'directory') {
    return { roots: [{ name, kind: 'directory' }], directories: [[name]], files: [], totalBytes: 0 }
  }
  const file = new File([type.template], name, { type: type.mime })
  return {
    roots: [{ name, kind: 'file' }],
    directories: [],
    files: [{ file, segments: [name] }],
    totalBytes: file.size,
  }
}
