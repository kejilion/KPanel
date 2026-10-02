// @vitest-environment jsdom
import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GalleryViewer from './GalleryViewer.vue'
import type { GalleryItem } from '@/lib/gallery'
import type { FileEntry } from '@/types/api'

function item(name: string): GalleryItem {
  const entry: FileEntry = {
    name, path: `/home/gallery/${name}`, kind: 'file', mime: 'image/jpeg', sizeBytes: 2048, mode: '-rw-r--r--',
    owner: 'root', group: 'root', modifiedAt: '2026-09-10T08:00:00Z', resourceVersion: `sha256:${'d'.repeat(64)}`,
    editable: false, previewable: true,
  }
  return { entry, kind: 'image', folder: '/home/gallery', time: Date.parse(entry.modifiedAt) }
}

let wrapper: VueWrapper | undefined

function mountViewer(props: Record<string, unknown> = {}): VueWrapper {
  wrapper = mount(GalleryViewer, {
    props: {
      items: [item('a.jpg'), item('b.jpg'), item('c.jpg')],
      index: 1,
      sources: (value: GalleryItem) => ({ original: `/content${value.entry.path}` }),
      filmstrip: true,
      ...props,
    },
    attachTo: document.body,
  })
  return wrapper
}

beforeEach(() => vi.useFakeTimers())

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
})

describe('GalleryViewer controls', () => {
  it('fades the bar, arrows and strip when idle and brings them back on movement', async () => {
    const view = mountViewer()
    expect(view.classes()).not.toContain('gallery-viewer--idle')

    vi.advanceTimersByTime(2900)
    await view.vm.$nextTick()
    expect(view.classes()).toContain('gallery-viewer--idle')

    await view.trigger('pointermove', { pointerType: 'mouse' })
    expect(view.classes()).not.toContain('gallery-viewer--idle')
  })

  it('keeps the controls while the pointer rests on them', async () => {
    const view = mountViewer()
    await view.get('.gallery-viewer__bar').trigger('pointerenter')
    vi.advanceTimersByTime(6000)
    await view.vm.$nextTick()
    expect(view.classes()).not.toContain('gallery-viewer--idle')

    await view.get('.gallery-viewer__bar').trigger('pointerleave')
    vi.advanceTimersByTime(2900)
    await view.vm.$nextTick()
    expect(view.classes()).toContain('gallery-viewer--idle')
  })

  it('pages with the arrow keys without waking the controls, and wakes them for other keys', async () => {
    const view = mountViewer()
    vi.advanceTimersByTime(2900)
    await view.vm.$nextTick()
    await view.trigger('keydown', { key: 'ArrowRight' })
    expect(view.emitted('navigate')?.[0]).toEqual([2])
    expect(view.classes()).toContain('gallery-viewer--idle')

    await view.trigger('keydown', { key: 'i' })
    expect(view.classes()).not.toContain('gallery-viewer--idle')
  })

  it('toggles the thumbnail strip through its own button', async () => {
    const view = mountViewer()
    expect(view.find('.gallery-viewer__strip').exists()).toBe(true)
    await view.get('button[aria-label="隐藏缩略图条"]').trigger('click')
    expect(view.emitted('update:filmstrip')?.[0]).toEqual([false])

    await view.setProps({ filmstrip: false })
    expect(view.find('.gallery-viewer__strip').exists()).toBe(false)
    expect(view.get('button[aria-label="显示缩略图条"]').attributes('aria-pressed')).toBe('false')
  })

  it('centres the current thumbnail by scrolling the strip only, never its ancestors', async () => {
    const intoView = vi.fn()
    const scrollTo = vi.fn()
    Element.prototype.scrollIntoView = intoView
    Element.prototype.scrollTo = scrollTo as unknown as typeof Element.prototype.scrollTo
    const view = mountViewer({ items: Array.from({ length: 30 }, (_, index) => item(`p${index}.jpg`)), index: 0 })
    await view.setProps({ index: 25 })
    await view.vm.$nextTick()
    // scrollIntoView would also scroll the overflow-hidden viewer sideways.
    expect(intoView).not.toHaveBeenCalled()
    expect(scrollTo).toHaveBeenCalled()
    expect(view.element.scrollLeft).toBe(0)
  })

  it('keeps the controls while the cover menu is open and closes it before the viewer', async () => {
    const view = mountViewer({ coverOptions: [{ id: 'own', label: '设为「图库」封面', active: false }] })
    await view.get('button[aria-label="设为封面"]').trigger('click')
    expect(view.find('.gallery-viewer__cover-menu').exists()).toBe(true)
    vi.advanceTimersByTime(6000)
    await view.vm.$nextTick()
    expect(view.classes()).not.toContain('gallery-viewer--idle')

    await view.trigger('keydown', { key: 'Escape' })
    expect(view.find('.gallery-viewer__cover-menu').exists()).toBe(false)
    expect(view.emitted('close')).toBeUndefined()
    await view.get('button[aria-label="设为封面"]').trigger('click')
    await view.get('.gallery-viewer__cover-menu button').trigger('click')
    expect(view.emitted('cover')?.[0]).toEqual([{ item: expect.objectContaining({ kind: 'image' }), option: 'own' }])
    expect(view.find('.gallery-viewer__cover-menu').exists()).toBe(false)
  })

  it('shows no cover button without options', () => {
    expect(mountViewer().find('button[aria-label="设为封面"]').exists()).toBe(false)
  })

  it('asks the page to move the open photo, and only offers it when the page can', async () => {
    expect(mountViewer().find('button[aria-label="移动到相册"]').exists()).toBe(false)
    wrapper?.unmount()
    const view = mountViewer({ canMove: true })
    await view.get('button[aria-label="移动到相册"]').trigger('click')
    expect(view.emitted('move')?.[0]).toEqual([expect.objectContaining({ entry: expect.objectContaining({ name: 'b.jpg' }) })])
  })

  it('has no strip toggle for a single photo', () => {
    const view = mountViewer({ items: [item('only.jpg')], index: 0 })
    expect(view.find('button[aria-label="隐藏缩略图条"]').exists()).toBe(false)
    expect(view.find('.gallery-viewer__strip').exists()).toBe(false)
  })
})
