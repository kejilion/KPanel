// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import GalleryMoveDialog, { type GalleryMoveFolder } from './GalleryMoveDialog.vue'

let wrapper: VueWrapper | undefined

function mountDialog(props: Record<string, unknown> = {}): VueWrapper {
  wrapper = mount(GalleryMoveDialog, {
    props: {
      open: true,
      summary: 'a.jpg',
      start: '/home/gallery',
      root: '/home/gallery',
      libraryRoot: '/home/gallery',
      sourceFolders: ['/home/gallery'],
      listFolders: vi.fn().mockResolvedValue([]),
      createFolder: vi.fn().mockResolvedValue(undefined),
      ...props,
    },
    attachTo: document.body,
  })
  return wrapper
}

const rows = () => [...document.querySelectorAll<HTMLButtonElement>('.gallery-move__folder')]
const crumbs = () => [...document.querySelectorAll('.gallery-move__crumb')].map((crumb) => crumb.textContent?.trim())
const confirmButton = () => document.querySelector<HTMLButtonElement>('.gallery-move__footer .button--primary')!

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('GalleryMoveDialog', () => {
  it('ignores a delayed creation after the dialog closes and reopens elsewhere', async () => {
    let finish: () => void = () => undefined
    const createFolder = vi.fn(() => new Promise<void>((resolve) => { finish = resolve }))
    const listFolders = vi.fn().mockResolvedValue([])
    const view = mountDialog({ createFolder, listFolders })
    await flushPromises()
    ;[...document.querySelectorAll<HTMLButtonElement>('.gallery-move__footer button')].find((button) => button.textContent?.includes('新建相册'))!.click()
    await flushPromises()
    const input = document.querySelector<HTMLInputElement>('.gallery-move__create input')!
    input.value = 'Nara'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.querySelector<HTMLFormElement>('.gallery-move__create')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(createFolder).toHaveBeenCalledWith('/home/gallery', 'Nara')
    await view.setProps({ open: false })
    await view.setProps({ open: true, start: '/home/gallery/Osaka' })
    await flushPromises()
    finish()
    await flushPromises()
    expect(crumbs()).toEqual(['图库', 'Osaka'])
    expect(listFolders).toHaveBeenLastCalledWith('/home/gallery/Osaka')
  })

  it('starts at the given folder, shows the library by name, and climbs back with the trail', async () => {
    const listFolders = vi.fn(async (path: string): Promise<GalleryMoveFolder[]> => (
      path === '/home/gallery' ? [{ name: 'Kyoto', path: '/home/gallery/Kyoto' }] : [{ name: 'Temples', path: '/home/gallery/Kyoto/Temples' }]
    ))
    mountDialog({ listFolders })
    await flushPromises()
    expect(crumbs()).toEqual(['图库'])
    rows()[0]!.click()
    await flushPromises()
    expect(crumbs()).toEqual(['图库', 'Kyoto'])
    expect(rows().map((row) => row.textContent?.trim())).toEqual(['Temples'])

    document.querySelector<HTMLButtonElement>('button.gallery-move__crumb')!.click()
    await flushPromises()
    expect(listFolders).toHaveBeenLastCalledWith('/home/gallery')
    expect(crumbs()).toEqual(['图库'])
  })

  it('confirms the folder it is showing, not a row', async () => {
    const view = mountDialog({
      sourceFolders: ['/elsewhere'],
      listFolders: vi.fn().mockResolvedValue([{ name: 'Kyoto', path: '/home/gallery/Kyoto' }]),
    })
    await flushPromises()
    confirmButton().click()
    expect(view.emitted('confirm')).toEqual([['/home/gallery']])
  })

  it('lets only the newest listing through when folders are opened quickly', async () => {
    const pending = new Map<string, (folders: GalleryMoveFolder[]) => void>()
    const listFolders = vi.fn((path: string) => new Promise<GalleryMoveFolder[]>((resolve) => {
      if (path === '/home/gallery') resolve([{ name: 'Kyoto', path: '/home/gallery/Kyoto' }, { name: 'Osaka', path: '/home/gallery/Osaka' }])
      else pending.set(path, resolve)
    }))
    mountDialog({ listFolders })
    await flushPromises()
    rows()[0]!.click() // Kyoto, slow
    await flushPromises()
    document.querySelector<HTMLButtonElement>('button.gallery-move__crumb')?.click()
    await flushPromises()
    pending.get('/home/gallery/Kyoto')?.([{ name: 'Stale', path: '/home/gallery/Kyoto/Stale' }])
    await flushPromises()
    expect(rows().map((row) => row.textContent?.trim())).toEqual(['Kyoto', 'Osaka'])
  })

  it('does not let a move proceed while it is busy', async () => {
    const view = mountDialog({ busy: true, sourceFolders: ['/elsewhere'] })
    await flushPromises()
    expect(confirmButton().disabled).toBe(true)
    expect(confirmButton().textContent).toContain('正在移动')
    expect(view.emitted('confirm')).toBeUndefined()
  })

  it('leaves the new-album field with Escape without closing the dialog', async () => {
    const view = mountDialog()
    await flushPromises()
    ;[...document.querySelectorAll<HTMLButtonElement>('.gallery-move__footer button')].find((button) => button.textContent?.includes('新建相册'))!.click()
    await flushPromises()
    const input = document.querySelector<HTMLInputElement>('.gallery-move__create input')!
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await flushPromises()
    expect(document.querySelector('.gallery-move__create')).toBeNull()
    expect(view.emitted('close')).toBeUndefined()
  })

  it('shows why creating failed and stays on the same folder', async () => {
    const createFolder = vi.fn().mockRejectedValue(new Error('权限不足'))
    mountDialog({ createFolder })
    await flushPromises()
    ;[...document.querySelectorAll<HTMLButtonElement>('.gallery-move__footer button')].find((button) => button.textContent?.includes('新建相册'))!.click()
    await flushPromises()
    const input = document.querySelector<HTMLInputElement>('.gallery-move__create input')!
    input.value = 'Nara'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await flushPromises()
    document.querySelector<HTMLFormElement>('.gallery-move__create')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    await flushPromises()
    expect(createFolder).toHaveBeenCalledWith('/home/gallery', 'Nara')
    expect(document.querySelector('.gallery-move__error')?.textContent).toContain('权限不足')
    expect(crumbs()).toEqual(['图库'])
  })
})
