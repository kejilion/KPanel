// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import type { FileDirectory, FileEntry } from '@/types/api'

const harness = vi.hoisted(() => ({
  tree: {} as Record<string, FileEntry[]>,
  markers: {} as Record<string, string>,
  list: vi.fn(),
  text: vi.fn(),
  upload: vi.fn(),
  toast: { show: vi.fn(), success: vi.fn(), danger: vi.fn() },
}))

vi.mock('@/stores/toast', () => ({ useToast: () => harness.toast }))
vi.mock('@/lib/fileHostContext', () => ({
  fileAPIForHost: () => ({
    list: harness.list,
    text: harness.text,
    upload: harness.upload,
    action: vi.fn(),
    entry: vi.fn().mockResolvedValue({}),
    contentUrl: (path: string) => `/content${path}`,
    thumbnailUrl: (path: string) => `/thumb${path}`,
  }),
}))

import GalleryView from './GalleryView.vue'

const ROOT = '/home/gallery'

function media(path: string, modifiedAt: string, overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name: path.slice(path.lastIndexOf('/') + 1), path, kind: 'file', mime: 'application/octet-stream',
    sizeBytes: 2048, mode: '-rw-r--r--', owner: 'root', group: 'root', modifiedAt,
    resourceVersion: `sha256:${'c'.repeat(64)}`, editable: false, previewable: true, ...overrides,
  }
}

const marker = (folderPath: string) => media(`${folderPath}/.kpanel-cover.json`, '2026-09-01T00:00:00Z', { sizeBytes: 40 })
const folder = (path: string): FileEntry => media(path, '2026-09-01T00:00:00Z', { kind: 'directory', mime: undefined, sizeBytes: 4096 })

const readFile = (file: File) => new Promise<string>((resolve) => {
  const reader = new FileReader()
  reader.onload = () => resolve(String(reader.result))
  reader.readAsText(file)
})

let wrapper: VueWrapper | undefined

async function mountGallery(path = '/gallery'): Promise<VueWrapper> {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/gallery', name: 'gallery', component: GalleryView }] })
  await router.push(path)
  await router.isReady()
  wrapper = mount({ template: '<RouterView />' }, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return wrapper
}

const heroSrc = (view: VueWrapper) => view.find('.gallery-hero__backdrop img').attributes('src')

beforeEach(() => {
  localStorage.clear()
  harness.markers = {}
  harness.tree = {
    [ROOT]: [
      media(`${ROOT}/sunset.jpg`, '2026-09-20T08:00:00Z'),
      media(`${ROOT}/beach.png`, '2026-08-02T08:00:00Z'),
      folder(`${ROOT}/Kyoto`),
    ],
    [`${ROOT}/Kyoto`]: [
      media(`${ROOT}/Kyoto/temple.jpg`, '2026-09-12T08:00:00Z'),
      media(`${ROOT}/Kyoto/old.jpg`, '2026-07-01T08:00:00Z'),
      media(`${ROOT}/Kyoto/walk.mp4`, '2026-09-14T08:00:00Z'),
    ],
  }
  harness.list.mockReset().mockImplementation(async (path: string): Promise<FileDirectory> => {
    const entries = harness.tree[path]
    if (!entries) throw new ApiError('not found', 404, 'not_found')
    return { path, entries, offset: 0, truncated: false, readAt: '2026-09-20T08:00:00Z' }
  })
  harness.text.mockReset().mockImplementation(async (path: string) => harness.markers[path] ?? '')
  harness.upload.mockReset().mockResolvedValue({})
  Object.values(harness.toast).forEach((mock) => mock.mockReset())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('gallery covers', () => {
  it('shows the newest photo until a cover is chosen, then the hand-picked ones', async () => {
    const plain = await mountGallery()
    expect(heroSrc(plain)).toBe(`/content${ROOT}/sunset.jpg`)
    plain.unmount()

    harness.tree[ROOT]!.push(marker(ROOT))
    harness.tree[`${ROOT}/Kyoto`]!.push(marker(`${ROOT}/Kyoto`))
    harness.markers[`${ROOT}/.kpanel-cover.json`] = '{"version":1,"cover":"beach.png"}'
    harness.markers[`${ROOT}/Kyoto/.kpanel-cover.json`] = '{"version":1,"cover":"old.jpg"}'
    const pinned = await mountGallery()
    expect(heroSrc(pinned)).toBe(`/content${ROOT}/beach.png`)
    expect(pinned.get('.album-card__cover img').attributes('src')).toBe(`/thumb${ROOT}/Kyoto/old.jpg`)
    // The marker is a hidden helper file, not a photo.
    expect(pinned.findAll('.gallery-tile')).toHaveLength(5)
  })

  it('falls back to the newest photo when the named cover is gone or the marker is damaged', async () => {
    harness.tree[ROOT]!.push(marker(ROOT))
    harness.markers[`${ROOT}/.kpanel-cover.json`] = '{"version":1,"cover":"deleted.jpg"}'
    expect(heroSrc(await mountGallery())).toBe(`/content${ROOT}/sunset.jpg`)
    wrapper?.unmount()
    harness.markers[`${ROOT}/.kpanel-cover.json`] = '{{ not json'
    expect(heroSrc(await mountGallery())).toBe(`/content${ROOT}/sunset.jpg`)
  })

  it('sets the cover from the viewer by writing the marker into the photo\'s folder', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[3]!.trigger('click') // beach.png, the oldest
    expect(view.get('.gallery-viewer__title strong').text()).toBe('beach.png')
    await view.get('button[aria-label="设为封面"]').trigger('click')
    const rows = view.findAll('.gallery-viewer__cover-menu button')
    expect(rows.map((row) => row.text())).toEqual(['设为「图库」封面'])
    await rows[0]!.trigger('click')
    await flushPromises()

    expect(harness.upload).toHaveBeenCalledTimes(1)
    const [folderPath, file, overwrite] = harness.upload.mock.calls[0]!
    expect(folderPath).toBe(ROOT)
    expect((file as File).name).toBe('.kpanel-cover.json')
    expect(JSON.parse(await readFile(file as File))).toEqual({ version: 1, cover: 'beach.png' })
    expect(overwrite).toBe(true)
    expect(harness.toast.success).toHaveBeenCalledWith('封面已更新')
    expect(heroSrc(view)).toBe(`/content${ROOT}/beach.png`)
  })

  it('offers both the album and the page for a photo one album down', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[2]!.trigger('click') // Kyoto/temple.jpg
    expect(view.get('.gallery-viewer__title strong').text()).toBe('temple.jpg')
    await view.get('button[aria-label="设为封面"]').trigger('click')
    const rows = view.findAll('.gallery-viewer__cover-menu button')
    expect(rows.map((row) => row.text())).toEqual(['设为「Kyoto」封面', '设为「图库」封面'])

    await rows[0]!.trigger('click')
    await flushPromises()
    expect(harness.upload.mock.calls[0]![0]).toBe(`${ROOT}/Kyoto`)
    expect(JSON.parse(await readFile(harness.upload.mock.calls[0]![1] as File))).toEqual({ version: 1, cover: 'temple.jpg' })

    await view.get('button[aria-label="设为封面"]').trigger('click')
    await view.findAll('.gallery-viewer__cover-menu button')[1]!.trigger('click')
    await flushPromises()
    expect(harness.upload.mock.calls[1]![0]).toBe(ROOT)
    expect(JSON.parse(await readFile(harness.upload.mock.calls[1]![1] as File))).toEqual({ version: 1, cover: 'Kyoto/temple.jpg' })
  })

  it('goes back to the automatic cover when the current cover is chosen again', async () => {
    harness.tree[ROOT]!.push(marker(ROOT))
    harness.markers[`${ROOT}/.kpanel-cover.json`] = '{"version":1,"cover":"beach.png"}'
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[3]!.trigger('click')
    await view.get('button[aria-label="设为封面"]').trigger('click')
    const row = view.get('.gallery-viewer__cover-menu button')
    expect(row.attributes('aria-checked')).toBe('true')
    expect(row.text()).toContain('当前封面')
    await row.trigger('click')
    await flushPromises()
    expect(JSON.parse(await readFile(harness.upload.mock.calls[0]![1] as File))).toEqual({ version: 1 })
    expect(harness.toast.success).toHaveBeenCalledWith('已恢复自动封面')
  })

  it('offers no cover for videos', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[1]!.trigger('click') // Kyoto/walk.mp4
    expect(view.get('.gallery-viewer__title strong').text()).toBe('walk.mp4')
    expect(view.find('button[aria-label="设为封面"]').exists()).toBe(false)
  })

  it('lets you pick the page cover from the cover menu, and cancel with Escape', async () => {
    const view = await mountGallery()
    const openMenu = async () => {
      await view.get('.gallery-hero__more button').trigger('click')
      return view.findAll('.gallery-hero__more .gallery-menu button').find((button) => button.text().includes('更换封面'))!
    }
    await (await openMenu()).trigger('click')
    expect(view.get('.gallery-selection--pick').text()).toContain('点选一张照片作为「图库」的封面')
    await view.get('.gallery-page').trigger('keydown', { key: 'Escape' })
    expect(view.find('.gallery-selection--pick').exists()).toBe(false)
    expect(harness.upload).not.toHaveBeenCalled()

    await (await openMenu()).trigger('click')
    await view.findAll('.gallery-tile__open')[3]!.trigger('click')
    await flushPromises()
    expect(view.find('.gallery-viewer').exists()).toBe(false)
    expect(view.find('.gallery-selection--pick').exists()).toBe(false)
    expect(harness.upload.mock.calls[0]![0]).toBe(ROOT)
    expect(JSON.parse(await readFile(harness.upload.mock.calls[0]![1] as File))).toEqual({ version: 1, cover: 'beach.png' })
  })

  it('stops waiting for a cover once you start selecting photos instead', async () => {
    const view = await mountGallery()
    await view.get('.gallery-hero__more button').trigger('click')
    await view.findAll('.gallery-hero__more .gallery-menu button').find((button) => button.text().includes('更换封面'))!.trigger('click')
    expect(view.find('.gallery-selection--pick').exists()).toBe(true)
    await view.findAll('.gallery-tile__check')[0]!.trigger('click')
    expect(view.find('.gallery-selection--pick').exists()).toBe(false)
    expect(view.get('.gallery-selection').text()).toContain('已选择 1 项')
    expect(harness.upload).not.toHaveBeenCalled()
  })

  it('refuses a video as the page cover and keeps picking', async () => {
    const view = await mountGallery()
    await view.get('.gallery-hero__more button').trigger('click')
    await view.findAll('.gallery-hero__more .gallery-menu button').find((button) => button.text().includes('更换封面'))!.trigger('click')
    await view.findAll('.gallery-tile__open')[1]!.trigger('click') // walk.mp4
    await flushPromises()
    expect(harness.upload).not.toHaveBeenCalled()
    expect(harness.toast.show).toHaveBeenCalledWith('这张不能作为封面', expect.anything())
  })

  it('offers "use the automatic cover" only while a cover is pinned', async () => {
    const plain = await mountGallery()
    await plain.get('.gallery-hero__more button').trigger('click')
    expect(plain.get('.gallery-hero__more .gallery-menu').text()).not.toContain('恢复自动封面')
    plain.unmount()

    harness.tree[ROOT]!.push(marker(ROOT))
    harness.markers[`${ROOT}/.kpanel-cover.json`] = '{"version":1,"cover":"beach.png"}'
    const pinned = await mountGallery()
    await pinned.get('.gallery-hero__more button').trigger('click')
    await pinned.findAll('.gallery-hero__more .gallery-menu button').find((button) => button.text().includes('恢复自动封面'))!.trigger('click')
    await flushPromises()
    expect(JSON.parse(await readFile(harness.upload.mock.calls[0]![1] as File))).toEqual({ version: 1 })
  })

  it('says so and keeps the page unchanged when the marker cannot be written', async () => {
    harness.upload.mockRejectedValueOnce(new ApiError('read-only', 403, 'forbidden'))
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[3]!.trigger('click')
    await view.get('button[aria-label="设为封面"]').trigger('click')
    await view.get('.gallery-viewer__cover-menu button').trigger('click')
    await flushPromises()
    expect(harness.toast.danger).toHaveBeenCalledWith('封面未更改', 'read-only')
    expect(heroSrc(view)).toBe(`/content${ROOT}/sunset.jpg`)
  })
})
