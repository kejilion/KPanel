import { describe, expect, it } from 'vitest'
import { desktopNewItemManifest, desktopNewItemName, desktopNewItemType } from '@/lib/desktopNewItems'

describe('desktop new items', () => {
  it('appends the extension once and rejects unusable names', () => {
    const md = desktopNewItemType('md')
    expect(desktopNewItemName(md, ' notes ')).toBe('notes.md')
    expect(desktopNewItemName(md, 'notes.MD')).toBe('notes.MD')
    expect(desktopNewItemName(md, 'a/b')).toBeUndefined()
    expect(desktopNewItemName(md, '..')).toBeUndefined()
    expect(desktopNewItemName(desktopNewItemType('folder'), 'docs')).toBe('docs')
  })

  it('builds single-root manifests', () => {
    const dir = desktopNewItemManifest(desktopNewItemType('folder'), 'docs')
    expect(dir.roots).toEqual([{ name: 'docs', kind: 'directory' }])
    expect(dir.directories).toEqual([['docs']])
    const file = desktopNewItemManifest(desktopNewItemType('txt'), 'a.txt')
    expect(file.files[0]?.segments).toEqual(['a.txt'])
    expect(file.totalBytes).toBe(0)
  })
})
