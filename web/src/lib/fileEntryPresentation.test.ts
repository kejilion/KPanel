import { describe, expect, it } from 'vitest'
import { FolderOpen } from '@lucide/vue'
import { fileEntryIconKind, fileIconPalette, shortcutFileGradient, shortcutFileIcon, type FileIconKind } from '@/lib/fileEntryPresentation'

describe('fileEntryIconKind', () => {
  it.each([
    ['README.md', 'document'], ['notes.txt', 'document'], ['server.log', 'document'],
    ['app.ts', 'code'], ['compose.yaml', 'code'], ['backup.tar.gz', 'archive'],
    ['report.csv', 'spreadsheet'], ['data.sql', 'database'], ['server.pem', 'secret'],
  ])('keeps %s consistent inside archives and editable listings', (name, kind) => {
    for (const editable of [false, true]) {
      expect(fileEntryIconKind({ name, kind: 'file', editable, previewable: editable })).toBe(kind)
    }
  })

  it('retains capability fallbacks for extensionless files', () => {
    expect(fileEntryIconKind({ name: 'Dockerfile', kind: 'file', editable: true, previewable: true })).toBe('code')
    expect(fileEntryIconKind({ name: 'unknown', kind: 'file', editable: false, previewable: false })).toBe('generic')
    expect(fileEntryIconKind({ name: 'README.md', kind: 'directory', editable: false, previewable: false })).toBe('folder')
  })
})

describe('shortcutFileGradient', () => {
  it('uses an open folder glyph for directory shortcuts', () => {
    expect(shortcutFileIcon('nginx', 'directory')).toBe(FolderOpen)
  })

  it('keeps directories visually distinct from regular files', () => {
    expect(shortcutFileGradient('nginx', 'directory')).toContain(fileIconPalette.folder[0])
    expect(shortcutFileGradient('README.md', 'file')).toContain(fileIconPalette.document[0])
  })

  it.each<[string, FileIconKind]>([
    ['compose.yaml', 'code'], ['photo.webp', 'image'], ['report.xlsx', 'spreadsheet'],
    ['backup.tar.gz', 'archive'], ['data.sqlite', 'database'], ['server.pem', 'secret'],
    ['recording.mp4', 'media'], ['music.mp3', 'media'], ['installer.deb', 'package'],
    ['slides.pptx', 'presentation'], ['README.md', 'document'], ['artifact.bin', 'generic'],
  ])('uses the file manager palette for desktop %s', (name, category) => {
    const gradient = shortcutFileGradient(name, 'file')
    expect(gradient).toContain(fileIconPalette[category][0])
    expect(gradient).toContain(fileIconPalette[category][1])
  })

  it('uses the neutral fallback for unknown file types', () => {
    expect(shortcutFileGradient('artifact.unknown', 'file'))
      .toBe('linear-gradient(145deg, #dde4eb 0%, #929fad 100%)')
  })
})
