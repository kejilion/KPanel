// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api } from '@/lib/api'
import type { FileActionInput, FileDirectory, FileEntry } from '@/types/api'

const harness = vi.hoisted(() => ({
  tree: {} as Record<string, FileEntry[]>,
  list: vi.fn(),
  upload: vi.fn(),
  action: vi.fn(),
  entry: vi.fn(),
  toast: { show: vi.fn(), success: vi.fn(), danger: vi.fn() },
}))

vi.mock('@/stores/toast', () => ({ useToast: () => harness.toast }))
vi.mock('@/lib/fileHostContext', () => ({
  fileAPIForHost: () => ({
    list: harness.list,
    upload: harness.upload,
    action: harness.action,
    entry: harness.entry,
    contentUrl: (path: string) => `/content${path}`,
    thumbnailUrl: (path: string) => `/thumb${path}`,
  }),
}))

import GalleryView from './GalleryView.vue'

function media(path: string, modifiedAt = '2026-09-10T08:00:00Z', sizeBytes = 2048): FileEntry {
  return {
    name: path.slice(path.lastIndexOf('/') + 1), path, kind: 'file', mime: 'application/octet-stream',
    sizeBytes, mode: '-rw-r--r--', owner: 'root', group: 'root', modifiedAt,
    resourceVersion: `sha256:${'c'.repeat(64)}`, editable: false, previewable: true,
  }
}

function folder(path: string): FileEntry {
  return { ...media(path), kind: 'directory', mime: undefined, sizeBytes: 4096 }
}

let wrapper: VueWrapper | undefined
let router: Router

async function mountGallery(path = '/gallery'): Promise<VueWrapper> {
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/gallery', name: 'gallery', component: GalleryView },
      { path: '/files', name: 'files', component: defineComponent({ render: () => null }) },
      { path: '/cluster', name: 'cluster', component: defineComponent({ render: () => null }) },
    ],
  })
  await router.push(path)
  await router.isReady()
  wrapper = mount({ template: '<RouterView />' }, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  localStorage.clear()
  harness.tree = {
    '/home/gallery': [
      media('/home/gallery/sunset.jpg', '2026-09-20T08:00:00Z'),
      media('/home/gallery/beach.png', '2026-08-02T08:00:00Z'),
      media('/home/gallery/notes.txt'),
      folder('/home/gallery/Kyoto'),
    ],
    '/home/gallery/Kyoto': [
      media('/home/gallery/Kyoto/temple.jpg', '2026-09-12T08:00:00Z'),
      media('/home/gallery/Kyoto/walk.mp4', '2026-09-14T08:00:00Z'),
    ],
  }
  harness.list.mockReset().mockImplementation(async (path: string): Promise<FileDirectory> => {
    const entries = harness.tree[path]
    if (!entries) throw new ApiError('not found', 404, 'not_found')
    return { path, entries, offset: 0, truncated: false, readAt: '2026-09-20T08:00:00Z' }
  })
  harness.upload.mockReset().mockResolvedValue(media('/home/gallery/new.jpg'))
  harness.action.mockReset().mockResolvedValue({ action: 'mkdir', succeeded: [{ path: '/home/gallery' }], failed: [] })
  harness.entry.mockReset().mockResolvedValue(folder('/home/gallery'))
  Object.values(harness.toast).forEach((mock) => mock.mockReset())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

const clusterHosts = [
  { id: 'local', name: 'panel', isLocal: true, kind: 'panel', state: 'online' },
  { id: 'edge-1', name: 'edge-melbourne', isLocal: false, kind: 'panel', state: 'online', fileManagementAvailable: true },
  { id: 'lite-1', name: 'lite-berlin', isLocal: false, kind: 'light_node', state: 'online', fileManagementAvailable: false },
  {
    id: 'paired-1', name: 'paired-oslo', isLocal: false, kind: 'panel', state: 'online', origin: 'https://oslo.example.com',
    transportSecurity: 'https', mutualFileTransferAvailable: true,
  },
]

describe('GalleryView host switcher', () => {
  const windowsHost = { id: 'win-1', name: 'Windows', isLocal: false, kind: 'light_node', state: 'online', fileManagementAvailable: true, pathStyle: 'windows-volumes' }

  afterEach(() => {
    vi.restoreAllMocks()
    document.body.querySelectorAll('.host-switcher__menu').forEach((menu) => menu.remove())
  })

  async function mountWithHosts(path = '/gallery', items: unknown[] = clusterHosts) {
    vi.spyOn(api.cluster, 'hosts').mockResolvedValue({ items } as never)
    return mountGallery(path)
  }

  async function openPicker(view: VueWrapper): Promise<HTMLButtonElement[]> {
    await view.get('.host-switcher__trigger').trigger('click')
    await flushPromises()
    return [...document.body.querySelectorAll<HTMLButtonElement>('[data-host-id]')]
  }

  it('waits for platform discovery and lists Windows drives without scanning them or assuming C:', async () => {
    let resolveHosts!: (value: never) => void
    vi.spyOn(api.cluster, 'hosts').mockReturnValue(new Promise((resolve) => { resolveHosts = resolve }) as never)
    harness.tree['/'] = [folder('/E'), folder('/D')]
    harness.tree['/D'] = [folder('/D/Pictures')]
    harness.tree['/D/Pictures'] = [media('/D/Pictures/photo.jpg')]
    const view = await mountGallery('/gallery?hostId=win-1')
    expect(harness.list).not.toHaveBeenCalled()
    resolveHosts({ items: [...clusterHosts, windowsHost] } as never)
    await flushPromises()
    expect(harness.list.mock.calls.map(([path]) => path)).toEqual(['/'])
    expect(view.get('.gallery-volumes').text()).toContain('选择磁盘')
    expect(view.get('.gallery-hero__actions .button--primary').attributes('disabled')).toBeDefined()
    expect(view.findAll('button').some((button) => button.text() === '新建相册')).toBe(false)
    const drives = view.findAll('.gallery-volumes button')
    expect(drives.slice(0, 2).map((button) => button.text())).toEqual(['D', 'E'])
    await drives[0]!.trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ hostId: 'win-1', path: '/D' })
    expect(view.find('.gallery-volumes').exists()).toBe(false)
    expect(view.findAll('.gallery-tile')).toHaveLength(1)
  })

  it('keeps a Windows file-pane folder on its host and restores the Linux library when switching', async () => {
    harness.tree['/E/Photos'] = [media('/E/Photos/photo.jpg')]
    const view = await mountWithHosts('/gallery?hostId=win-1&path=%2FE%2FPhotos', [...clusterHosts, windowsHost])
    expect(harness.list.mock.calls.map(([path]) => path)).toEqual(['/E/Photos'])
    expect(router.currentRoute.value.query).toEqual({ hostId: 'win-1', path: '/E/Photos' })
    expect(view.findAll('.gallery-tile')).toHaveLength(1)
    harness.list.mockClear()
    ;(await openPicker(view)).find((row) => row.dataset.hostId === 'edge-1')!.click()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ hostId: 'edge-1' })
    expect(harness.list.mock.calls[0]?.[0]).toBe('/home/gallery')
  })

  it('discards a late local snapshot when Windows is opened before platform discovery completes', async () => {
    let resolveHosts!: (value: never) => void
    let resolveLocal!: (value: FileDirectory) => void
    vi.spyOn(api.cluster, 'hosts').mockReturnValue(new Promise((resolve) => { resolveHosts = resolve }) as never)
    harness.list.mockImplementationOnce(() => new Promise((resolve) => { resolveLocal = resolve }))
    harness.tree['/E/Photos'] = [media('/E/Photos/windows.jpg')]
    const view = await mountGallery()
    await router.push('/gallery?hostId=win-1&path=%2FE%2FPhotos')
    await flushPromises()
    resolveLocal({ path: '/home/gallery', entries: [media('/home/gallery/local.jpg')], offset: 0, truncated: false, readAt: '2026-10-03T00:00:00Z' })
    await flushPromises()
    expect(view.findAll('.gallery-tile')).toHaveLength(0)
    resolveHosts({ items: [...clusterHosts, windowsHost] } as never)
    await flushPromises()
    expect(view.findAll('.gallery-tile')).toHaveLength(1)
    expect(view.findAllComponents({ name: 'GalleryTile' }).map((tile) => tile.props('item').entry.path)).toEqual(['/E/Photos/windows.jpg'])
    expect(harness.list.mock.calls.some(([path]) => path === '/E/Photos')).toBe(true)
  })

  it('returns from a missing Windows folder to its drive list, not the Linux library', async () => {
    harness.tree['/'] = [folder('/E')]
    const view = await mountWithHosts('/gallery?hostId=win-1&path=%2FE%2Fmissing', [...clusterHosts, windowsHost])
    expect(view.text()).toContain('这个文件夹不存在')
    await view.findAll('button').find((button) => button.text() === '返回图库')!.trigger('click')
    await flushPromises()
    expect(view.find('.gallery-volumes').exists()).toBe(true)
    expect(harness.list.mock.calls.some(([path]) => path === '/home/gallery')).toBe(false)
  })

  it('shows a recoverable Windows drive-list error and refuses an unknown host default', async () => {
    harness.tree['/'] = [folder('/E')]
    harness.list.mockRejectedValueOnce(new ApiError('Drive listing unavailable', 503, 'unavailable'))
    const view = await mountWithHosts('/gallery?hostId=win-1', [...clusterHosts, windowsHost])
    expect(view.text()).toContain('图库读取失败')
    await view.get('.error-state button').trigger('click')
    await flushPromises()
    expect(view.find('.gallery-volumes').exists()).toBe(true)
    harness.list.mockClear()
    await router.push('/gallery?hostId=missing')
    await flushPromises()
    expect(view.text()).toContain('所选主机已移除或不存在')
    expect(harness.list).not.toHaveBeenCalled()
  })

  it('rejects dropping uploads onto the virtual drive list without invoking a file write', async () => {
    harness.tree['/'] = [folder('/E')]
    const view = await mountWithHosts('/gallery?hostId=win-1', [...clusterHosts, windowsHost])
    await view.get('.gallery-page').trigger('drop', {
      dataTransfer: { types: ['Files'], files: [new File(['photo'], 'photo.jpg', { type: 'image/jpeg' })] },
    })
    await flushPromises()
    expect(harness.toast.show).toHaveBeenCalledWith('请先选择磁盘中的文件夹，再上传照片和视频。')
    expect(harness.upload).not.toHaveBeenCalled()
    expect(harness.action).not.toHaveBeenCalled()
  })

  it('removes the previous host snapshot while the next host loads or fails', async () => {
    const view = await mountWithHosts()
    expect(view.findAll('.gallery-tile')).toHaveLength(4)
    let fail: (error: Error) => void = () => undefined
    harness.list.mockImplementationOnce(() => new Promise((_resolve, reject) => { fail = reject }))
    await router.push('/gallery?hostId=edge-1')
    await flushPromises()
    expect(view.findAll('.gallery-tile')).toHaveLength(0)
    expect(view.findAll('.album-card')).toHaveLength(0)
    expect(view.get('.gallery-hero__actions .button--primary').attributes('disabled')).toBeDefined()
    fail(new ApiError('remote unavailable', 503, 'unavailable'))
    await flushPromises()
    expect(view.text()).toContain('图库读取失败')
    expect(view.findAll('.gallery-tile')).toHaveLength(0)
  })

  it('reuses the file manager host picker and lists every host with its status', async () => {
    const view = await mountWithHosts()
    expect(view.get('.host-switcher__trigger').text()).toContain('本机')
    const rows = await openPicker(view)
    expect(rows.map((row) => row.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      '本机当前面板', 'edge-melbourne文件管理已就绪', 'lite-berlin文件代理未就绪', 'paired-oslo已配对 · 文件互传',
    ])
    expect(rows[0]!.getAttribute('aria-pressed')).toBe('true')
    expect(rows[1]!.getAttribute('aria-pressed')).toBe('false')
  })

  it('opens the chosen host library and returns to this panel without a host', async () => {
    const view = await mountWithHosts('/gallery?path=%2Fhome%2Fgallery%2FKyoto')
    ;(await openPicker(view))[1]!.click()
    await flushPromises()
    // The folder path stays behind: each host starts at its own library folder.
    expect(router.currentRoute.value.query).toEqual({ hostId: 'edge-1' })
    expect(view.get('.host-switcher__trigger').text()).toContain('edge-melbourne')
    expect(document.body.querySelector('.host-switcher__menu')).toBeNull()

    ;(await openPicker(view))[0]!.click()
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({})
  })

  it('sends a host whose file relay is not ready to the cluster page instead of a dead gallery', async () => {
    const view = await mountWithHosts()
    ;(await openPicker(view))[2]!.click()
    await flushPromises()
    expect(router.currentRoute.value.name).toBe('cluster')
  })

  it('opens a paired panel without a file relay in a new tab on its own gallery page', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue({} as Window)
    const view = await mountWithHosts()
    ;(await openPicker(view))[3]!.click()
    await flushPromises()
    expect(open).toHaveBeenCalledWith('https://oslo.example.com/gallery', '_blank', 'noopener,noreferrer')
    expect(router.currentRoute.value.name).toBe('gallery')
  })

  it('keeps a queued upload on the host it was started for when you switch', async () => {
    const view = await mountWithHosts()
    let release: (value: FileEntry) => void = () => undefined
    harness.upload.mockImplementationOnce(() => new Promise<FileEntry>((resolve) => { release = resolve }))
    const input = view.get<HTMLInputElement>('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: [new File(['x'], 'late.jpg', { type: 'image/jpeg' })], configurable: true })
    await input.trigger('change')
    ;(await openPicker(view))[1]!.click()
    await flushPromises()
    release(media('/home/gallery/late.jpg'))
    await flushPromises()
    expect(view.get('.gallery-uploads').text()).toContain('已上传 1 项')
  })

  it('hides the picker on a single-panel install', async () => {
    const view = await mountWithHosts('/gallery', [clusterHosts[0]])
    expect(view.find('.gallery-host').exists()).toBe(false)
  })
})

describe('GalleryView on a remote host', () => {
  it('treats the library folder on that host like the local one and names the host', async () => {
    vi.spyOn(api.cluster, 'hosts').mockResolvedValue({ items: [{ id: 'edge-1', name: 'edge-melbourne', isLocal: false }] } as never)
    harness.tree = {}
    const view = await mountGallery('/gallery?hostId=edge-1')
    expect(view.get('.gallery-hero__title').text()).toBe('图库')
    expect(view.get('.host-switcher__trigger').text()).toContain('edge-melbourne')
    expect(view.text()).toContain('开始建立你的图库')
    // The location setting belongs to this browser, so it is only offered on the local host.
    const buttons = view.findAll('.gallery-empty button').map((button) => button.text())
    expect(buttons).toEqual(['创建图库文件夹'])
    harness.tree['/home/gallery'] = []
    harness.entry.mockRejectedValueOnce(new ApiError('not found', 404, 'not_found'))
    await view.get('.gallery-empty button').trigger('click')
    await flushPromises()
    expect(harness.action).toHaveBeenCalledWith({ action: 'mkdir', target: '/home', name: 'gallery' })
  })
})

describe('GalleryView', () => {
  it('keeps the last snapshot and reports a failed refresh with a retry', async () => {
    const view = await mountGallery()
    expect(view.get('.gallery-hero').classes()).not.toContain('gallery-hero--menu-open')
    harness.list.mockRejectedValueOnce(new ApiError('refresh unavailable', 503, 'unavailable'))
    await view.get('.gallery-hero__more > button').trigger('click')
    expect(view.get('.gallery-hero').classes()).toContain('gallery-hero--menu-open')
    await view.findAll('.gallery-menu button').find((button) => button.text().includes('刷新'))!.trigger('click')
    await flushPromises()
    expect(view.get('.gallery-hero').classes()).not.toContain('gallery-hero--menu-open')
    expect(view.find('.gallery-hero__more .gallery-menu').exists()).toBe(false)
    expect(view.findAll('.gallery-tile')).toHaveLength(4)
    expect(view.get('.gallery-notice[role="alert"]').text()).toContain('refresh unavailable')
    await view.get('.gallery-notice[role="alert"] button').trigger('click')
    await flushPromises()
    expect(view.find('.gallery-notice[role="alert"]').exists()).toBe(false)
  })

  it('does not start queued uploads after the gallery is closed', async () => {
    const view = await mountGallery()
    harness.upload.mockImplementation((_target, _file, _overwrite, _progress, signal: AbortSignal) => new Promise((_resolve, reject) => {
      signal.addEventListener('abort', () => reject(new Error('cancelled')), { once: true })
    }))
    const input = view.get<HTMLInputElement>('input[type="file"]')
    Object.defineProperty(input.element, 'files', { value: ['one', 'two', 'three'].map((name) => new File(['x'], `${name}.jpg`, { type: 'image/jpeg' })), configurable: true })
    await input.trigger('change')
    await flushPromises()
    expect(harness.upload).toHaveBeenCalledTimes(2)
    view.unmount()
    wrapper = undefined
    await flushPromises()
    expect(harness.upload).toHaveBeenCalledTimes(2)
  })

  it('shows the library with albums, a month timeline and media counts', async () => {
    const view = await mountGallery()
    expect(view.get('.gallery-hero__title').text()).toBe('图库')
    expect(view.get('.gallery-hero__meta').text()).toContain('3 张照片 · 1 段视频 · 1 个相册')
    expect(view.findAll('.album-card__name').map((node) => node.text())).toEqual(['Kyoto'])
    expect(view.get('.album-card__meta').text()).toBe('1 张照片 · 1 段视频')
    expect(view.findAll('.gallery-month__header h3')).toHaveLength(2)
    expect(view.findAll('.gallery-tile')).toHaveLength(4)
    // Non-media files never reach the timeline.
    expect(view.text()).not.toContain('notes.txt')
    // Thumbnails come from the Agent for JPEG/PNG; videos stay lazy until visible.
    expect(view.find('img[src="/thumb/home/gallery/sunset.jpg"]').exists()).toBe(true)
  })

  it('filters to videos and opens an album through the route', async () => {
    const view = await mountGallery()
    const videoFilter = view.findAll('.gallery-segmented button').find((button) => button.text().startsWith('视频'))!
    await videoFilter.trigger('click')
    expect(view.findAll('.gallery-tile')).toHaveLength(1)

    await view.get('.album-card__open').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query.path).toBe('/home/gallery/Kyoto')
    expect(view.get('.gallery-hero__title').text()).toBe('Kyoto')
    expect(view.findAll('.gallery-trail__link').map((node) => node.text())).toEqual(['图库'])
  })

  it('offers to create a missing library folder', async () => {
    harness.tree = {}
    harness.entry.mockRejectedValueOnce(new ApiError('not found', 404, 'not_found'))
    const view = await mountGallery()
    expect(view.text()).toContain('开始建立你的图库')
    harness.tree['/home/gallery'] = []
    await view.findAll('.gallery-empty button').find((button) => button.text().includes('创建图库文件夹'))!.trigger('click')
    await flushPromises()
    expect(harness.action).toHaveBeenCalledWith({ action: 'mkdir', target: '/home', name: 'gallery' })
    expect(view.text()).toContain('图库还是空的')
  })

  it('uploads media only, never overwrites, and renames on conflict', async () => {
    const view = await mountGallery()
    harness.upload.mockRejectedValueOnce(new ApiError('exists', 409, 'file_exists'))
    const input = view.get<HTMLInputElement>('input[type="file"]')
    const photo = new File(['x'], 'sunset.jpg', { type: 'image/jpeg' })
    const fresh = new File(['y'], 'fresh.jpg', { type: 'image/jpeg' })
    const pdf = new File(['z'], 'report.pdf', { type: 'application/pdf' })
    Object.defineProperty(input.element, 'files', { value: [photo, fresh, pdf], configurable: true })
    await input.trigger('change')
    await flushPromises()

    const names = harness.upload.mock.calls.map((call) => (call[1] as File).name)
    // The known duplicate is renamed before upload; a server-side conflict picks the next name.
    expect(names).toEqual(['sunset (2).jpg', 'fresh.jpg', 'sunset (3).jpg'])
    expect(harness.upload.mock.calls.every((call) => call[2] === false)).toBe(true)
    expect(names).not.toContain('report.pdf')
    expect(harness.toast.show).toHaveBeenCalledWith('已跳过非图片或视频文件', expect.anything())
    expect(view.get('.gallery-uploads').text()).toContain('已上传 2 项')
  })

  it('opens the viewer, pages with the keyboard and closes with Escape', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[0]!.trigger('click')
    const viewer = view.get('.gallery-viewer')
    expect(viewer.get('.gallery-viewer__title strong').text()).toBe('sunset.jpg')
    expect(viewer.get('img.gallery-viewer__media:not(.gallery-viewer__media--preview)').attributes('src')).toBe('/content/home/gallery/sunset.jpg')

    await viewer.trigger('keydown', { key: 'ArrowRight' })
    expect(view.get('.gallery-viewer__title strong').text()).toBe('walk.mp4')
    expect(view.find('video.gallery-viewer__media').exists()).toBe(true)

    await view.get('.gallery-viewer').trigger('keydown', { key: 'Escape' })
    expect(view.find('.gallery-viewer').exists()).toBe(false)
  })

  it('moves on to the next photo when the open one goes to the trash', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[0]!.trigger('click')
    expect(view.get('.gallery-viewer__title strong').text()).toBe('sunset.jpg')
    harness.action.mockResolvedValueOnce({ action: 'trash', succeeded: [{ path: '/home/gallery/sunset.jpg' }], failed: [] })
    await view.get('.gallery-viewer__danger').trigger('click')
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('.modal-panel button')]
      .find((button) => button.textContent?.includes('移入回收站'))!
    confirm.click()
    await flushPromises()
    expect(view.get('.gallery-viewer__title strong').text()).toBe('walk.mp4')
  })

  it('drops an album and everything in it from the page once it is in the trash', async () => {
    const view = await mountGallery()
    expect(view.findAll('.gallery-tile')).toHaveLength(4)
    await view.get('.album-card__menu-button').trigger('click')
    await view.findAll('.gallery-menu--album button').find((button) => button.text().includes('删除相册'))!.trigger('click')
    expect(document.querySelector('.modal-panel')?.textContent).toContain('Kyoto · 2 个媒体文件')
    harness.action.mockResolvedValueOnce({ action: 'trash', succeeded: [{ path: '/home/gallery/Kyoto' }], failed: [] })
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('.modal-panel button')]
      .find((button) => button.textContent?.includes('移入回收站'))!
    confirm.click()
    await flushPromises()
    expect(view.find('.album-card').exists()).toBe(false)
    expect(view.findAll('.gallery-tile')).toHaveLength(2)
  })

  it('moves selected media to the trash with resource versions', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__check')[0]!.trigger('click')
    expect(view.get('.gallery-selection').text()).toContain('已选择 1 项')
    harness.action.mockResolvedValueOnce({ action: 'trash', succeeded: [{ path: '/home/gallery/sunset.jpg' }], failed: [] })
    await view.findAll('.gallery-selection button').find((button) => button.text().includes('移入回收站'))!.trigger('click')
    const confirm = [...document.querySelectorAll<HTMLButtonElement>('.modal-panel button')]
      .find((button) => button.textContent?.includes('移入回收站'))!
    confirm.click()
    await flushPromises()
    const input = harness.action.mock.calls.at(-1)![0] as FileActionInput
    expect(input).toMatchObject({
      action: 'trash',
      sources: ['/home/gallery/sunset.jpg'],
      expectedResourceVersions: { '/home/gallery/sunset.jpg': `sha256:${'c'.repeat(64)}` },
    })
    expect(view.findAll('.gallery-tile')).toHaveLength(3)
  })
})
