import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { FileEntry } from '@/types/api'
import { automaticCinemaCover, cinemaRenameTarget, readCinemaFolder } from './cinema'

const { filesForHost, list } = vi.hoisted(() => {
  const list = vi.fn()
  return { list, filesForHost: vi.fn(() => ({ list })) }
})
vi.mock('./fileHostContext', () => ({ fileAPIForHost: filesForHost }))

function entry(path: string, kind: FileEntry['kind'] = 'file'): FileEntry {
  return {
    path, name: path.split('/').pop()!, kind, sizeBytes: 42, mode: '', owner: '', group: '',
    modifiedAt: '2026-01-01T00:00:00Z', resourceVersion: 'v1', editable: false, previewable: true,
  }
}

beforeEach(() => { list.mockReset(); filesForHost.mockClear() })

describe('cinema library', () => {
  it('keeps a rename in the original directory with the original extension', () => {
    const film = entry('/films/old.cut.mkv')
    expect(cinemaRenameTarget(film, ' New Cut ')).toBe('/films/New Cut.mkv')
    expect(cinemaRenameTarget(film, '../New Cut')).toBeUndefined()
    expect(cinemaRenameTarget(film, '影'.repeat(100))).toBeUndefined()
  })

  it('uses film-specific posters before folder artwork and avoids assigning a shared poster to several films', () => {
    const film = { entry: entry('/films/A.mp4'), folder: '/films', kind: 'video' as const, time: 0 }
    const generic = entry('/films/poster.webp')
    const specific = entry('/films/A-poster.jpg')
    expect(automaticCinemaCover(film, [generic, specific], true)).toBe(specific)
    expect(automaticCinemaCover(film, [generic], false)).toBeUndefined()
    expect(automaticCinemaCover(film, [generic], true)).toBe(generic)
  })

  it('scans nested movies through the chosen host and keeps readable results after a child error', async () => {
    list.mockImplementation(async (path: string) => {
      if (path === '/films') return { entries: [entry('/films/folder', 'directory'), entry('/films/denied', 'directory'), entry('/films/.hidden', 'directory')] }
      if (path === '/films/folder') return { entries: [entry('/films/folder/title.webm'), entry('/films/folder/poster.webp')] }
      throw new Error('denied')
    })
    const signal = new AbortController().signal
    const result = await readCinemaFolder('/films', 'host-b', signal)
    expect(filesForHost).toHaveBeenCalledWith('host-b')
    expect(result.films.map(film => film.entry.name)).toEqual(['title.webm'])
    expect(result.images.get('/films/folder')?.[0]?.name).toBe('poster.webp')
    expect(result.unreadable).toBe(1)
    expect(list).toHaveBeenCalledTimes(3)
  })

  it('does not walk the host filesystem when browsing its root', async () => {
    list.mockResolvedValue({ entries: [entry('/home', 'directory'), entry('/preview.mp4')] })
    const result = await readCinemaFolder('/', '', new AbortController().signal)
    expect(list).toHaveBeenCalledTimes(1)
    expect(result.folders[0]?.path).toBe('/home')
    expect(result.films[0]?.entry.path).toBe('/preview.mp4')
  })

  it('publishes completed pages before a slow child and keeps prior snapshots immutable', async () => {
    let finishChild!: (value: unknown) => void
    list.mockImplementation((path: string) => path === '/films'
      ? Promise.resolve({ entries: [entry('/films/a.mp4'), entry('/films/sub', 'directory'), entry('/films/a.jpg')] })
      : new Promise(resolve => { finishChild = resolve }))
    const updates: import('./cinema').CinemaFolder[] = []
    const loading = readCinemaFolder('/films', '', new AbortController().signal, value => updates.push(value))
    await vi.waitFor(() => expect(finishChild).toBeDefined())
    expect(updates[0]?.films.map(film => film.entry.name)).toEqual(['a.mp4'])
    finishChild({ entries: [entry('/films/sub/b.mp4')] })
    const result = await loading
    expect(result.films).toHaveLength(2)
    expect(updates[0]?.films).toHaveLength(1)
  })
})
