// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/lib/api'
import { GALLERY_DRAG_TYPE, parseGalleryDrag } from '@/lib/galleryMove'
import type { FileActionInput, FileDirectory, FileEntry } from '@/types/api'

const harness = vi.hoisted(() => ({
  tree: {} as Record<string, FileEntry[]>,
  list: vi.fn(),
  action: vi.fn(),
  toast: { show: vi.fn(), success: vi.fn(), danger: vi.fn() },
}))

vi.mock('@/stores/toast', () => ({ useToast: () => harness.toast }))
vi.mock('@/lib/fileHostContext', () => ({
  fileAPIForHost: () => ({
    list: harness.list,
    text: vi.fn().mockResolvedValue(''),
    upload: vi.fn(),
    action: harness.action,
    entry: vi.fn().mockResolvedValue({}),
    contentUrl: (path: string) => `/content${path}`,
    thumbnailUrl: (path: string) => `/thumb${path}`,
  }),
}))

import GalleryView from './GalleryView.vue'

const ROOT = '/home/gallery'
const KYOTO = `${ROOT}/Kyoto`

function media(path: string, modifiedAt: string, overrides: Partial<FileEntry> = {}): FileEntry {
  return {
    name: path.slice(path.lastIndexOf('/') + 1), path, kind: 'file', mime: 'application/octet-stream',
    sizeBytes: 2048, mode: '-rw-r--r--', owner: 'root', group: 'root', modifiedAt,
    resourceVersion: `sha256:${'c'.repeat(64)}`, editable: false, previewable: true, ...overrides,
  }
}

const folder = (path: string): FileEntry => media(path, '2026-09-01T00:00:00Z', { kind: 'directory', mime: undefined, sizeBytes: 4096 })

/** The host's move/mkdir over the in-memory tree: the target is a directory and nothing is overwritten. */
function hostAction(input: FileActionInput) {
  const result = { action: input.action, succeeded: [] as Array<{ path: string; destination?: string }>, failed: [] as Array<{ path: string; detail: string }> }
  const target = input.target ?? ''
  if (input.action === 'mkdir') {
    const created = `${target}/${input.name}`
    harness.tree[created] = []
    harness.tree[target]!.push(folder(created))
    result.succeeded.push({ path: created })
  } else if (input.action === 'move') {
    for (const source of input.sources ?? []) {
      const parent = source.slice(0, source.lastIndexOf('/'))
      const entry = harness.tree[parent]?.find((candidate) => candidate.path === source)
      const destination = `${target}/${source.slice(source.lastIndexOf('/') + 1)}`
      if (!entry) result.failed.push({ path: source, detail: '文件状态已变化，请刷新后重试' })
      else if (harness.tree[target]!.some((candidate) => candidate.path === destination)) result.failed.push({ path: source, detail: '目标已存在' })
      else {
        harness.tree[parent] = harness.tree[parent]!.filter((candidate) => candidate !== entry)
        harness.tree[target]!.push({ ...entry, path: destination })
        result.succeeded.push({ path: source, destination })
      }
    }
  }
  return Promise.resolve(result)
}

let wrapper: VueWrapper | undefined

async function mountGallery(): Promise<VueWrapper> {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/gallery', name: 'gallery', component: GalleryView }] })
  await router.push('/gallery')
  await router.isReady()
  wrapper = mount({ template: '<RouterView />' }, { global: { plugins: [router] }, attachTo: document.body })
  await flushPromises()
  return wrapper
}

const rows = () => [...document.querySelectorAll<HTMLButtonElement>('.gallery-move__folder')]
const confirmButton = () => document.querySelector<HTMLButtonElement>('.gallery-move__footer .button--primary')!
const dialogOpen = () => Boolean(document.querySelector('.modal-panel'))
const rowNamed = (name: string) => rows().find((row) => row.textContent?.trim() === name)!
const tileTitles = (view: VueWrapper) => view.findAll('.gallery-tile__open').map((tile) => tile.attributes('aria-label'))
const move = (): FileActionInput[] => harness.action.mock.calls.map((call) => call[0] as FileActionInput).filter((input) => input.action === 'move')

async function click(element: HTMLElement | undefined): Promise<void> {
  element!.click()
  await flushPromises()
}

async function selectTiles(view: VueWrapper, ...indexes: number[]): Promise<void> {
  for (const index of indexes) await view.findAll('.gallery-tile__check')[index]!.trigger('click')
}

async function openMoveFromSelection(view: VueWrapper): Promise<void> {
  await view.findAll('.gallery-selection button').find((button) => button.text().includes('移动到'))!.trigger('click')
  await flushPromises()
}

function fakeTransfer() {
  const store = new Map<string, string>()
  return {
    store,
    effectAllowed: '',
    dropEffect: '',
    get types() { return [...store.keys()] },
    setData: (type: string, value: string) => { store.set(type, value) },
    getData: (type: string) => store.get(type) ?? '',
  }
}

beforeEach(() => {
  localStorage.clear()
  harness.tree = {
    [ROOT]: [
      media(`${ROOT}/sunset.jpg`, '2026-09-20T08:00:00Z'),
      media(`${ROOT}/beach.png`, '2026-08-02T08:00:00Z'),
      folder(KYOTO),
      folder(`${ROOT}/Osaka`),
    ],
    [KYOTO]: [
      media(`${KYOTO}/temple.jpg`, '2026-09-12T08:00:00Z'),
      media(`${KYOTO}/old.jpg`, '2026-07-01T08:00:00Z'),
      media(`${KYOTO}/walk.mp4`, '2026-09-14T08:00:00Z'),
    ],
    [`${ROOT}/Osaka`]: [],
  }
  harness.list.mockReset().mockImplementation(async (path: string): Promise<FileDirectory> => {
    const entries = harness.tree[path]
    if (!entries) throw new ApiError('not found', 404, 'not_found')
    return { path, entries: [...entries], offset: 0, truncated: false, readAt: '2026-09-20T08:00:00Z' }
  })
  harness.action.mockReset().mockImplementation(hostAction)
  Object.values(harness.toast).forEach((mock) => mock.mockReset())
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('moving photos to another album', () => {
  it('moves the selection into a chosen album, with version protection, and updates the page at once', async () => {
    const view = await mountGallery()
    expect(tileTitles(view)).toHaveLength(5)
    await selectTiles(view, 0, 3) // sunset.jpg and beach.png
    await openMoveFromSelection(view)

    expect(document.querySelector('.gallery-move__summary')?.textContent).toContain('2 个文件')
    expect(rows().map((row) => row.textContent?.trim())).toEqual(['Kyoto', 'Osaka'])
    // Both files already live here, so moving "here" would change nothing.
    expect(confirmButton().disabled).toBe(true)

    await click(rowNamed('Kyoto'))
    expect(confirmButton().textContent).toContain('移到「Kyoto」')
    expect(confirmButton().disabled).toBe(false)
    await click(confirmButton())

    expect(move()).toEqual([{
      action: 'move',
      sources: [`${ROOT}/sunset.jpg`, `${ROOT}/beach.png`],
      target: KYOTO,
      expectedResourceVersions: { [`${ROOT}/sunset.jpg`]: `sha256:${'c'.repeat(64)}`, [`${ROOT}/beach.png`]: `sha256:${'c'.repeat(64)}` },
    }])
    expect(harness.toast.success).toHaveBeenCalledWith('已移到「Kyoto」', '2 个文件')
    expect(dialogOpen()).toBe(false)
    // The selection is spent, the timeline still shows every photo, and the album card was recounted.
    expect(view.find('.gallery-selection').exists()).toBe(false)
    expect(tileTitles(view)).toHaveLength(5)
    expect(view.get('.album-card__meta').text()).toContain('4 张照片')
  })

  it('reports files that could not move, keeps them selected, and still closes once something moved', async () => {
    harness.tree[KYOTO]!.push(media(`${KYOTO}/beach.png`, '2026-08-01T08:00:00Z'))
    const view = await mountGallery()
    await selectTiles(view, 0, 3)
    await openMoveFromSelection(view)
    await click(rowNamed('Kyoto'))
    await click(confirmButton())

    expect(harness.toast.danger).toHaveBeenCalledWith('部分文件未移动', '1 项成功，1 项失败：目标已存在')
    expect(dialogOpen()).toBe(false)
    expect(view.get('.gallery-selection').text()).toContain('已选择 1 项')
  })

  it('keeps the dialog open when nothing moved, so another album can be chosen', async () => {
    harness.tree[KYOTO]!.push(media(`${KYOTO}/beach.png`, '2026-08-01T08:00:00Z'))
    const view = await mountGallery()
    await selectTiles(view, 3) // beach.png
    await openMoveFromSelection(view)
    await click(rowNamed('Kyoto'))
    await click(confirmButton())

    expect(harness.toast.danger).toHaveBeenCalledWith('未能移动', '0 项成功，1 项失败：目标已存在')
    expect(dialogOpen()).toBe(true)
  })

  it('moves the open photo from the viewer and stays on the one next to it', async () => {
    const view = await mountGallery()
    await view.findAll('.gallery-tile__open')[0]!.trigger('click')
    expect(view.get('.gallery-viewer__title strong').text()).toBe('sunset.jpg')
    await view.get('button[aria-label="移动到相册"]').trigger('click')
    await flushPromises()
    await click(rowNamed('Osaka'))
    await click(confirmButton())

    expect(move()[0]).toMatchObject({ sources: [`${ROOT}/sunset.jpg`], target: `${ROOT}/Osaka` })
    expect(view.get('.gallery-viewer__title strong').text()).toBe('walk.mp4')
  })

  it('does not offer to move a photo into the folder it is already in', async () => {
    const view = await mountGallery()
    await selectTiles(view, 2) // temple.jpg, in Kyoto
    await openMoveFromSelection(view)
    expect(confirmButton().disabled).toBe(false) // the dialog starts at the library, where it is not
    await click(rowNamed('Kyoto'))
    expect(confirmButton().disabled).toBe(true)
    expect(document.querySelector('.gallery-move__hint')?.textContent).toBe('文件已经在这个相册里')
    expect(move()).toHaveLength(0)
  })

  it('creates a new album from the dialog and moves into it', async () => {
    const view = await mountGallery()
    await selectTiles(view, 0)
    await openMoveFromSelection(view)
    await click([...document.querySelectorAll<HTMLButtonElement>('.gallery-move__footer button')].find((button) => button.textContent?.includes('新建相册')))
    const input = document.querySelector<HTMLInputElement>('.gallery-move__create input')!
    input.value = 'Nara'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.querySelector<HTMLFormElement>('.gallery-move__create')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()

    expect(harness.action).toHaveBeenCalledWith({ action: 'mkdir', target: ROOT, name: 'Nara' })
    // The dialog steps into the new album, ready to move there.
    expect(confirmButton().textContent).toContain('移到「Nara」')
    await click(confirmButton())
    expect(move()[0]).toMatchObject({ sources: [`${ROOT}/sunset.jpg`], target: `${ROOT}/Nara` })
  })

  it('refuses an album name that is already taken', async () => {
    const view = await mountGallery()
    await selectTiles(view, 0)
    await openMoveFromSelection(view)
    await click([...document.querySelectorAll<HTMLButtonElement>('.gallery-move__footer button')].find((button) => button.textContent?.includes('新建相册')))
    const input = document.querySelector<HTMLInputElement>('.gallery-move__create input')!
    input.value = 'kyoto'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    expect(document.querySelector('.gallery-move__error')?.textContent).toContain('已有同名相册或文件夹')
    expect(document.querySelector<HTMLButtonElement>('.gallery-move__create button[type="submit"]')!.disabled).toBe(true)
  })

  it('says so when a folder cannot be read, and does not allow moving there', async () => {
    const view = await mountGallery()
    await selectTiles(view, 0)
    await openMoveFromSelection(view)
    delete harness.tree[KYOTO]
    await click(rowNamed('Kyoto'))
    expect(document.querySelector('.gallery-move__state--error')?.textContent).toContain('文件夹读取失败')
    expect(confirmButton().disabled).toBe(true)
  })
})

describe('dragging photos onto albums', () => {
  async function dragTile(view: VueWrapper, index: number) {
    const transfer = fakeTransfer()
    await view.findAll('.gallery-tile')[index]!.trigger('dragstart', { dataTransfer: transfer })
    return transfer
  }

  it('moves a dragged photo into the album it is dropped on', async () => {
    const view = await mountGallery()
    const transfer = await dragTile(view, 0)
    expect(view.get('.gallery-tile').attributes('draggable')).toBe('true')
    expect(parseGalleryDrag(transfer.getData(GALLERY_DRAG_TYPE))).toEqual({ hostId: '', paths: [`${ROOT}/sunset.jpg`] })
    expect(view.get('.gallery-drag-hint').text()).toContain('1 项')

    const cards = view.findAll('.album-card')
    await cards[0]!.trigger('dragover', { dataTransfer: transfer })
    expect(cards[0]!.classes()).toContain('is-drop-target')
    expect(transfer.dropEffect).toBe('move')
    await cards[0]!.trigger('drop', { dataTransfer: transfer })
    await flushPromises()

    expect(move()[0]).toMatchObject({ sources: [`${ROOT}/sunset.jpg`], target: KYOTO })
    expect(view.find('.gallery-drag-hint').exists()).toBe(false)
    expect(view.find('.album-card.is-drop-target').exists()).toBe(false)
  })

  it('takes the whole selection along when one of the selected photos is dragged', async () => {
    const view = await mountGallery()
    await selectTiles(view, 0, 3)
    const transfer = await dragTile(view, 3)
    expect(parseGalleryDrag(transfer.getData(GALLERY_DRAG_TYPE))?.paths).toEqual([`${ROOT}/sunset.jpg`, `${ROOT}/beach.png`])
    // A photo outside the selection goes alone.
    const other = await dragTile(view, 1)
    expect(parseGalleryDrag(other.getData(GALLERY_DRAG_TYPE))?.paths).toEqual([`${KYOTO}/walk.mp4`])
  })

  it('tells you when a photo is dropped on the album it is already in', async () => {
    const view = await mountGallery()
    const transfer = await dragTile(view, 2) // temple.jpg
    await view.findAll('.album-card')[0]!.trigger('drop', { dataTransfer: transfer })
    await flushPromises()
    expect(harness.toast.show).toHaveBeenCalledWith('文件已经在这个相册里')
    expect(move()).toHaveLength(0)
  })

  it('ignores drags that did not start on this page', async () => {
    const view = await mountGallery()
    const foreign = fakeTransfer()
    foreign.setData(GALLERY_DRAG_TYPE, JSON.stringify({ hostId: 'other', paths: [`${ROOT}/sunset.jpg`] }))
    const card = view.get('.album-card')
    await card.trigger('dragover', { dataTransfer: foreign })
    expect(card.classes()).not.toContain('is-drop-target')
    await card.trigger('drop', { dataTransfer: foreign })
    await flushPromises()
    expect(move()).toHaveLength(0)
  })

  it('clears a drag whose tile vanished before dragend on the next press', async () => {
    const view = await mountGallery()
    await dragTile(view, 0)
    expect(view.find('.gallery-drag-hint').exists()).toBe(true)
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(view.find('.gallery-drag-hint').exists()).toBe(false)
  })

  it('does not start a drag while a move is still running', async () => {
    let finish: (value: unknown) => void = () => undefined
    const view = await mountGallery()
    harness.action.mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    await selectTiles(view, 0)
    await openMoveFromSelection(view)
    await click(rowNamed('Kyoto'))
    confirmButton().click() // the move is now in flight
    await flushPromises()
    const transfer = fakeTransfer()
    const event = new Event('dragstart', { cancelable: true, bubbles: true }) as Event & { dataTransfer?: unknown }
    event.dataTransfer = transfer
    view.findAll('.gallery-tile')[1]!.element.dispatchEvent(event)
    await flushPromises()
    expect(event.defaultPrevented).toBe(true)
    expect(view.find('.gallery-drag-hint').exists()).toBe(false)
    finish({ action: 'move', succeeded: [], failed: [] })
    await flushPromises()
  })

  it('clears the hint and highlight when the drag is abandoned', async () => {
    const view = await mountGallery()
    const transfer = await dragTile(view, 0)
    await view.get('.album-card').trigger('dragover', { dataTransfer: transfer })
    await view.findAll('.gallery-tile')[0]!.trigger('dragend', { dataTransfer: transfer })
    expect(view.find('.gallery-drag-hint').exists()).toBe(false)
    expect(view.find('.album-card.is-drop-target').exists()).toBe(false)
  })
})
