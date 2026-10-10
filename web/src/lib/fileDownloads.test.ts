import { beforeEach, describe, expect, it, vi } from 'vitest'
import { archiveDownloadName, downloadFileEntries } from './fileDownloads'

const mocks = vi.hoisted(() => ({
  archiveUrl: vi.fn(),
  contentUrl: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    files: {
      archiveUrl: mocks.archiveUrl,
      contentUrl: mocks.contentUrl,
    },
  },
}))

function entry(name: string, kind: 'file' | 'directory' = 'file') {
  return {
    name,
    path: `/${name}`,
    kind,
    resourceVersion: `sha256:${name}`,
  }
}

function installDocument() {
  const anchors: Array<{
    href: string
    download: string
    rel: string
    click: ReturnType<typeof vi.fn>
    remove: ReturnType<typeof vi.fn>
  }> = []
  const appendChild = vi.fn()
  vi.stubGlobal('document', {
    body: { appendChild },
    createElement: vi.fn(() => {
      const anchor = { href: '', download: '', rel: '', click: vi.fn(), remove: vi.fn() }
      anchors.push(anchor)
      return anchor
    }),
  })
  return { anchors, appendChild }
}

beforeEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  mocks.archiveUrl.mockReset()
  mocks.contentUrl.mockReset()
  mocks.contentUrl.mockReturnValue('/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment')
  mocks.archiveUrl.mockImplementation((_entries, name) => `/api/v1/files/archive?name=${name}`)
})

describe('downloadFileEntries', () => {
  it('builds Windows-safe archive names without duplicate ZIP suffixes', () => {
    expect(archiveDownloadName([entry('photos', 'directory')], 'home')).toBe('photos.zip')
    expect(archiveDownloadName([entry('one.txt'), entry('two.txt')], 'reports.zip')).toBe('reports.zip')
    expect(archiveDownloadName([entry('one.txt'), entry('two.txt')], 'CON')).toBe('_CON.zip')
    expect(archiveDownloadName([entry('one.txt'), entry('two.txt')], 'CON.foo.bar')).toBe('_CON.foo.bar.zip')
    expect(archiveDownloadName([entry('one.txt'), entry('two.txt')], 'LPT1.backup.tar')).toBe('_LPT1.backup.tar.zip')
    expect(archiveDownloadName([entry('one.txt'), entry('two.txt')], '报表:2026?')).toBe('报表_2026_.zip')
  })

  it('starts a session download immediately for a single ordinary file', async () => {
    const { anchors } = installDocument()
    const file = entry('nginx.conf')
    const download = downloadFileEntries([file], 'etc')

    expect(mocks.contentUrl).toHaveBeenCalledWith(file.path, 'attachment', undefined)
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment',
      download: 'nginx.conf', rel: 'noopener',
    })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
    expect(anchors[0]!.remove).toHaveBeenCalledOnce()
    await download
  })

  it('uses the session archive URL for a directory ZIP', async () => {
    const { anchors } = installDocument()
    const directory = entry('photos', 'directory')

    await downloadFileEntries([directory], 'home')

    expect(mocks.contentUrl).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).toHaveBeenCalledWith([directory], 'photos.zip', undefined)
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ href: '/api/v1/files/archive?name=photos.zip', download: 'photos.zip' })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
  })

  it('uses one session archive URL for a mixed multi-selection', async () => {
    const { anchors } = installDocument()
    const entries = [entry('one.txt'), entry('logs', 'directory')]

    await downloadFileEntries(entries, 'home')

    expect(mocks.contentUrl).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).toHaveBeenCalledWith(entries, 'home.zip', undefined)
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ href: '/api/v1/files/archive?name=home.zip', download: 'home.zip' })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
  })

  it('streams a single file directly from the selected remote host', async () => {
    const { anchors } = installDocument()
    const file = entry('nginx.conf')
    mocks.contentUrl.mockReturnValue('/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment')

    await downloadFileEntries([file], 'etc', 'remote')

    expect(mocks.contentUrl).toHaveBeenCalledWith(file.path, 'attachment', 'remote')
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment',
      download: 'nginx.conf',
    })
  })

  it('streams a remote directory archive directly from the selected host', async () => {
    const { anchors } = installDocument()
    const directory = entry('photos', 'directory')
    mocks.archiveUrl.mockReturnValue('/api/v1/files/archive?selection=photos&name=photos.zip')

    await downloadFileEntries([directory], 'home', 'remote')

    expect(mocks.contentUrl).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).toHaveBeenCalledWith([directory], 'photos.zip', 'remote')
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/archive?selection=photos&name=photos.zip',
      download: 'photos.zip',
    })
  })
})
