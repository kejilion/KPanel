import { beforeEach, describe, expect, it, vi } from 'vitest'
import { archiveDownloadName, downloadFileEntries } from './fileDownloads'

const mocks = vi.hoisted(() => ({
  archiveUrl: vi.fn(),
  contentUrl: vi.fn(),
  createArchiveDownloadTicket: vi.fn(),
  createDownloadTicket: vi.fn(),
  isRemoteFileHostSelected: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    files: {
      archiveUrl: mocks.archiveUrl,
      contentUrl: mocks.contentUrl,
      createArchiveDownloadTicket: mocks.createArchiveDownloadTicket,
      createDownloadTicket: mocks.createDownloadTicket,
    },
  },
  isRemoteFileHostSelected: mocks.isRemoteFileHostSelected,
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
  mocks.createArchiveDownloadTicket.mockReset()
  mocks.createDownloadTicket.mockReset()
  mocks.isRemoteFileHostSelected.mockReset()
  mocks.isRemoteFileHostSelected.mockReturnValue(false)
  mocks.createArchiveDownloadTicket.mockResolvedValue({
    downloadUrl: '/downloads/archive-ticket',
    expiresAt: '2026-08-20T00:05:00Z',
  })
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

  it('uses one ticket for a single ordinary file', async () => {
    const { anchors } = installDocument()
    const file = entry('nginx.conf')
    mocks.createDownloadTicket.mockResolvedValue({
      downloadUrl: '/downloads/ticket',
      expiresAt: '2026-08-20T00:05:00Z',
    })

    await downloadFileEntries([file], 'etc')

    expect(mocks.createDownloadTicket).toHaveBeenCalledWith(file.path)
    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ href: '/downloads/ticket', download: 'nginx.conf', rel: 'noopener' })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
    expect(anchors[0]!.remove).toHaveBeenCalledOnce()
  })

  it('uses one short-lived archive ticket for a directory ZIP', async () => {
    const { anchors } = installDocument()
    const directory = entry('photos', 'directory')

    await downloadFileEntries([directory], 'home')

    expect(mocks.createDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.createArchiveDownloadTicket).toHaveBeenCalledWith([directory], 'photos.zip')
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ href: '/downloads/archive-ticket', download: 'photos.zip' })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
  })

  it('uses one short-lived archive ticket for a mixed multi-selection', async () => {
    const { anchors } = installDocument()
    const entries = [entry('one.txt'), entry('logs', 'directory')]

    await downloadFileEntries(entries, 'home')

    expect(mocks.createDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.createArchiveDownloadTicket).toHaveBeenCalledWith(entries, 'home.zip')
    expect(mocks.archiveUrl).not.toHaveBeenCalled()
    expect(anchors).toHaveLength(1)
    expect(anchors[0]).toMatchObject({ href: '/downloads/archive-ticket', download: 'home.zip' })
    expect(anchors[0]!.click).toHaveBeenCalledOnce()
  })

  it('streams a single file directly from the selected remote host', async () => {
    const { anchors } = installDocument()
    const file = entry('nginx.conf')
    mocks.isRemoteFileHostSelected.mockReturnValue(true)
    mocks.contentUrl.mockReturnValue('/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment')

    await downloadFileEntries([file], 'etc')

    expect(mocks.createDownloadTicket).not.toHaveBeenCalled()
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/content?path=%2Fnginx.conf&disposition=attachment',
      download: 'nginx.conf',
    })
  })

  it('streams a remote directory archive directly from the selected host', async () => {
    const { anchors } = installDocument()
    const directory = entry('photos', 'directory')
    mocks.isRemoteFileHostSelected.mockReturnValue(true)
    mocks.archiveUrl.mockReturnValue('/api/v1/files/archive?selection=photos&name=photos.zip')

    await downloadFileEntries([directory], 'home')

    expect(mocks.createArchiveDownloadTicket).not.toHaveBeenCalled()
    expect(mocks.archiveUrl).toHaveBeenCalledWith([directory], 'photos.zip')
    expect(anchors[0]).toMatchObject({
      href: '/api/v1/files/archive?selection=photos&name=photos.zip',
      download: 'photos.zip',
    })
  })
})
