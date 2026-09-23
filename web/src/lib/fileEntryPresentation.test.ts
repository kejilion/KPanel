import { describe, expect, it } from 'vitest'
import { FolderOpen } from '@lucide/vue'
import { fileEntryIconKind, shortcutFileGradient, shortcutFileIcon } from '@/lib/fileEntryPresentation'

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
    expect(shortcutFileGradient('nginx', 'directory')).toContain('#facc15')
    expect(shortcutFileGradient('README.md', 'file')).toContain('#94a3b8')
  })

  it('uses restrained category colors for common file types', () => {
    expect(shortcutFileGradient('compose.yaml', 'file')).toContain('#38bdf8')
    expect(shortcutFileGradient('photo.webp', 'file')).toContain('#a78bfa')
    expect(shortcutFileGradient('report.xlsx', 'file')).toContain('#34d399')
    expect(shortcutFileGradient('backup.tar.gz', 'file')).toContain('#fb923c')
    expect(shortcutFileGradient('data.sqlite', 'file')).toContain('#2dd4bf')
    expect(shortcutFileGradient('server.pem', 'file')).toContain('#f87171')
  })

  it('uses the neutral fallback for unknown file types', () => {
    expect(shortcutFileGradient('artifact.unknown', 'file'))
      .toBe('linear-gradient(145deg, #94a3b8 0%, #475569 100%)')
  })
})
