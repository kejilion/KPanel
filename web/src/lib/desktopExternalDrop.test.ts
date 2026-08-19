// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import {
  collectExternalDrop,
  DESKTOP_UPLOAD_DIRECTORY,
  DesktopExternalDropError,
  MAX_EXTERNAL_DROP_FILES,
  uploadExternalDrop,
  type DesktopExternalTransferAPI,
} from './desktopExternalDrop'
import type { FileActionInput, FileActionResult, FileEntry } from '@/types/api'

function entry(path: string, kind: FileEntry['kind'] = 'file'): FileEntry {
  return {
    name: path.split('/').at(-1) || '/', path, kind, mime: 'text/plain', sizeBytes: 1,
    mode: '0644', owner: 'root', group: 'root', modifiedAt: '2026-08-14T00:00:00Z',
    resourceVersion: 'sha256:test', editable: kind === 'file', previewable: kind === 'file',
  }
}

function fileSystemFile(name: string, content: string) {
  return {
    name,
    isFile: true,
    isDirectory: false,
    file(success: (value: File) => void) {
      success(new File([content], name, { type: 'text/plain' }))
    },
  }
}

function fileSystemDirectory(name: string, children: unknown[]) {
  return {
    name,
    isFile: false,
    isDirectory: true,
    createReader() {
      let read = false
      return {
        readEntries(success: (values: unknown[]) => void) {
          if (read) success([])
          else {
            read = true
            success(children)
          }
        },
      }
    },
  }
}

function transfer(entries: unknown[], files: File[] = []): DataTransfer {
  return {
    items: entries.map((value) => ({ kind: 'file', webkitGetAsEntry: () => value })),
    files,
  } as unknown as DataTransfer
}

function mockAPI(existing: Record<string, FileEntry> = {}): DesktopExternalTransferAPI & {
  uploads: Array<{ path: string; name: string }>
  directories: string[]
} {
  const values = new Map(Object.entries(existing))
  const uploads: Array<{ path: string; name: string }> = []
  const directories: string[] = []
  return {
    uploads,
    directories,
    async entry(path) {
      const value = values.get(path)
      if (value) return value
      throw Object.assign(new Error('not found'), { status: 404 })
    },
    async action(input: FileActionInput): Promise<FileActionResult> {
      if (input.action !== 'mkdir' || !input.target || !input.name) throw new Error('unexpected action')
      const path = `${input.target === '/' ? '' : input.target}/${input.name}`
      directories.push(path)
      values.set(path, entry(path, 'directory'))
      return { action: 'mkdir', succeeded: [{ path }], failed: [] }
    },
    async upload(path, file, _overwrite, onProgress) {
      uploads.push({ path, name: file.name })
      onProgress?.(50)
      onProgress?.(100)
      const target = `${path}/${file.name}`
      const value = entry(target)
      values.set(target, value)
      return value
    },
  }
}

describe('desktop external drop', () => {
  it('recursively collects files, directories, empty folders, and total bytes', async () => {
    const input = transfer([
      fileSystemDirectory('project', [
        fileSystemFile('README.md', 'hello'),
        fileSystemDirectory('empty', []),
        fileSystemDirectory('src', [fileSystemFile('main.ts', 'export {}')]),
      ]),
    ])

    const manifest = await collectExternalDrop(input)

    expect(manifest.roots).toEqual([{ name: 'project', kind: 'directory' }])
    expect(manifest.directories).toEqual([['project'], ['project', 'empty'], ['project', 'src']])
    expect(manifest.files.map((value) => value.segments)).toEqual([
      ['project', 'README.md'],
      ['project', 'src', 'main.ts'],
    ])
    expect(manifest.totalBytes).toBe(14)
  })

  it('falls back to ordinary files when directory entries are unavailable', async () => {
    const first = new File(['a'], 'one.txt')
    const second = new File(['bb'], 'two.txt')
    const manifest = await collectExternalDrop(transfer([], [first, second]))

    expect(manifest.roots).toEqual([
      { name: 'one.txt', kind: 'file' },
      { name: 'two.txt', kind: 'file' },
    ])
    expect(manifest.totalBytes).toBe(3)
  })

  it('rejects invalid path components before any upload', async () => {
    await expect(collectExternalDrop(transfer([fileSystemFile('../secret', 'x')]))).rejects.toBeInstanceOf(
      DesktopExternalDropError,
    )
  })

  it('rejects an oversized fallback file list before allocating a manifest', async () => {
    const files = Array.from(
      { length: MAX_EXTERNAL_DROP_FILES + 1 },
      (_, index) => new File([], `file-${index}.txt`),
    )

    await expect(collectExternalDrop(transfer([], files))).rejects.toMatchObject({ code: 'too_many' })
  })

  it('stops directory enumeration when the transfer is cancelled', async () => {
    const controller = new AbortController()
    controller.abort()

    await expect(collectExternalDrop(transfer([fileSystemFile('notes.txt', 'x')]), controller.signal))
      .rejects.toMatchObject({ name: 'AbortError' })
  })

  it('creates the managed desktop directory, preserves hierarchy, and reports progress', async () => {
    const manifest = await collectExternalDrop(transfer([
      fileSystemDirectory('project', [
        fileSystemFile('README.md', 'hello'),
        fileSystemDirectory('src', [fileSystemFile('main.ts', 'export {}')]),
      ]),
    ]))
    const api = mockAPI({ '/home': entry('/home', 'directory') })
    const progress = vi.fn()

    const result = await uploadExternalDrop(manifest, api, new AbortController().signal, progress)

    expect(api.directories).toEqual([
      DESKTOP_UPLOAD_DIRECTORY,
      `${DESKTOP_UPLOAD_DIRECTORY}/project`,
      `${DESKTOP_UPLOAD_DIRECTORY}/project/src`,
    ])
    expect(api.uploads).toEqual(expect.arrayContaining([
      { path: `${DESKTOP_UPLOAD_DIRECTORY}/project`, name: 'README.md' },
      { path: `${DESKTOP_UPLOAD_DIRECTORY}/project/src`, name: 'main.ts' },
    ]))
    expect(result.entries).toEqual([{ name: 'project', path: `${DESKTOP_UPLOAD_DIRECTORY}/project`, kind: 'directory' }])
    expect(result.failed).toEqual([])
    expect(progress).toHaveBeenCalledWith(expect.objectContaining({ completedFiles: 2, loadedBytes: 14 }))
  })

  it('keeps both top-level files by suffixing a server-side name conflict', async () => {
    const manifest = await collectExternalDrop(transfer([], [new File(['new'], 'notes.txt')]))
    const api = mockAPI({
      '/home': entry('/home', 'directory'),
      [DESKTOP_UPLOAD_DIRECTORY]: entry(DESKTOP_UPLOAD_DIRECTORY, 'directory'),
      [`${DESKTOP_UPLOAD_DIRECTORY}/notes.txt`]: entry(`${DESKTOP_UPLOAD_DIRECTORY}/notes.txt`),
    })

    const result = await uploadExternalDrop(manifest, api, new AbortController().signal, vi.fn())

    expect(api.uploads).toEqual([{ path: DESKTOP_UPLOAD_DIRECTORY, name: 'notes (1).txt' }])
    expect(result.entries[0]?.path).toBe(`${DESKTOP_UPLOAD_DIRECTORY}/notes (1).txt`)
  })

  it('does not expose a failed top-level file as a desktop entry', async () => {
    const manifest = await collectExternalDrop(transfer([], [new File(['x'], 'broken.txt')]))
    const api = mockAPI({
      '/home': entry('/home', 'directory'),
      [DESKTOP_UPLOAD_DIRECTORY]: entry(DESKTOP_UPLOAD_DIRECTORY, 'directory'),
    })
    api.upload = vi.fn().mockRejectedValue(new Error('disk full'))

    const result = await uploadExternalDrop(manifest, api, new AbortController().signal, vi.fn())

    expect(result.entries).toEqual([])
    expect(result.failed).toEqual([{ name: 'broken.txt', detail: 'disk full' }])
  })
})
