// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError, api } from '@/lib/api'
import type { OfferItem, OffersSnapshot } from '@/types/api'
import OffersView from './OffersView.vue'

function offer(id: string, featured = false): OfferItem {
  return {
    id,
    vendor: `厂商 ${id}`,
    alt: `${id} 的优惠活动`,
    featured,
    url: `https://${id}.example.com/aff.php?aff=1`,
    host: `${id}.example.com`,
    card: `/api/v1/offers/media/${id.padEnd(64, 'a')}`,
    ...(featured ? { wide: `/api/v1/offers/media/${id.padEnd(64, 'b')}` } : {}),
  }
}

function snapshot(items: OfferItem[], state: OffersSnapshot['state'] = 'live'): OffersSnapshot {
  return { state, updatedAt: '2026-10-08T00:00:00Z', fetchedAt: '2026-10-09T02:00:00Z', items }
}

let wrapper: VueWrapper | undefined
let list: ReturnType<typeof vi.spyOn>

async function mountOffers(value: OffersSnapshot | Error): Promise<VueWrapper> {
  if (value instanceof Error) list.mockRejectedValue(value)
  else list.mockResolvedValue(value)
  wrapper = mount(OffersView, { attachTo: document.body })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  list = vi.spyOn(api.offers, 'list')
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
  Reflect.deleteProperty(window, 'matchMedia')
})

describe('OffersView', () => {
  it('shows featured artwork in the carousel and the rest on the wall, each labelled as an ad', async () => {
    const view = await mountOffers(snapshot([offer('a', true), offer('b', true), offer('c'), offer('d')]))

    const slides = view.findAll('.offers-showcase__slide')
    expect(slides).toHaveLength(2)
    expect(slides[0]!.find('img').attributes('src')).toBe(offer('a', true).wide)
    expect(slides[1]!.attributes('aria-hidden')).toBe('true')
    expect(slides[1]!.attributes('tabindex')).toBe('-1')

    const tiles = view.findAll('.offers-tile:not(.offers-tile--more)')
    expect(tiles.map((tile) => tile.find('img').attributes('src'))).toEqual([offer('c').card, offer('d').card])
    for (const link of [...slides, ...tiles]) {
      expect(link.attributes('target')).toBe('_blank')
      expect(link.attributes('rel')).toBe('sponsored noopener noreferrer')
      expect(link.attributes('aria-label')).toContain('（广告，在新标签页打开 ')
    }
    // One disclosure under the carousel and one under every wall banner.
    expect(view.findAll('.offers-disclosure')).toHaveLength(1 + tiles.length)
    expect(view.find('.offers-showcase__caption').text()).toContain('厂商 a')
    expect(view.find('.offers-showcase .offers-host').text()).toContain('a.example.com')
    expect(view.find('.offers-footnote').text()).toContain('链接含推广返利（AFF）')
    expect(view.find('.offers-footnote').text()).toContain('也不记录你点了哪些横幅')
  })

  it('rotates the carousel every six seconds and stops while hovered or paused', async () => {
    vi.useFakeTimers()
    const view = await mountOffers(snapshot([offer('a', true), offer('b', true), offer('c', true)]))
    const active = () => view.findAll('.offers-showcase__slide').findIndex((slide) => slide.classes('is-active'))

    expect(active()).toBe(0)
    await vi.advanceTimersByTimeAsync(6000)
    expect(active()).toBe(1)

    await view.find('.offers-showcase').trigger('pointerenter')
    await vi.advanceTimersByTimeAsync(12000)
    expect(active()).toBe(1)
    await view.find('.offers-showcase').trigger('pointerleave')

    await view.find('.offers-controls__pause').trigger('click')
    expect(view.find('.offers-controls__pause').attributes('aria-pressed')).toBe('true')
    await vi.advanceTimersByTimeAsync(12000)
    expect(active()).toBe(1)

    await view.findAll('.offers-dots button')[2]!.trigger('click')
    expect(active()).toBe(2)
    await view.find('[aria-label="下一张"]').trigger('click')
    expect(active()).toBe(0)
  })

  it('never rotates on its own when the visitor prefers reduced motion', async () => {
    vi.useFakeTimers()
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() }),
    })
    const view = await mountOffers(snapshot([offer('a', true), offer('b', true)]))
    await vi.advanceTimersByTimeAsync(30000)
    expect(view.findAll('.offers-showcase__slide')[0]!.classes()).toContain('is-active')
  })

  it('lays out the wall by page width and swaps the carousel to 2:1 art when narrow', async () => {
    let report: ((width: number) => void) | undefined
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) {
        report = (width) => callback([{ contentRect: { width } } as ResizeObserverEntry], this as unknown as ResizeObserver)
      }
      observe() {}
      disconnect() {}
    })
    const view = await mountOffers(snapshot([offer('a', true), offer('c'), offer('d')]))
    const wall = () => view.get('.offers-wall').classes()

    report?.(1100)
    await flushPromises()
    expect(wall()).toContain('offers-wall--cols-3')
    report?.(1000)
    await flushPromises()
    expect(wall()).toContain('offers-wall--cols-2')
    report?.(600)
    await flushPromises()
    expect(wall()).toContain('offers-wall--cols-1')
    expect(view.get('.offers-showcase__slide img').attributes('src')).toBe(offer('a', true).card)
    vi.unstubAllGlobals()
  })

  it('hides carousel controls for a single featured banner', async () => {
    const view = await mountOffers(snapshot([offer('a', true), offer('c')]))
    expect(view.find('.offers-controls').exists()).toBe(false)
    expect(view.find('.offers-showcase__meta .offers-disclosure').text()).toBe('广告')
  })

  it.each([
    { wall: 5, layout: 'offers-tile--tile' },
    { wall: 4, layout: 'offers-tile--span' },
    { wall: 3, layout: 'offers-tile--bar' },
  ])('fills the last wall row with the more-offers link ($wall wall banners)', async ({ wall, layout }) => {
    const view = await mountOffers(snapshot(Array.from({ length: wall }, (_, index) => offer(`w${index}`))))
    const more = view.get('.offers-tile--more')
    expect(more.classes()).toContain(layout)
    expect(more.attributes('href')).toBe('https://kejilion.pro/topvps/')
    expect(more.attributes('rel')).toBe('noopener noreferrer')
  })

  it('keeps the last publication visible and says so when a refresh failed', async () => {
    const view = await mountOffers(snapshot([offer('c')], 'stale'))
    expect(view.find('.offers-stale').text()).toContain('未能刷新')
    expect(view.find('.offers-updated').exists()).toBe(false)
    expect(view.findAll('.offers-tile:not(.offers-tile--more)')).toHaveLength(1)

    await view.find('.offers-section-head button').trigger('click')
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ refresh: true }))
  })

  it('explains where the content comes from when nothing has been fetched yet', async () => {
    const view = await mountOffers(snapshot([], 'unavailable'))
    expect(view.text()).toContain('暂时无法获取广告内容')
    expect(view.text()).toContain('app.kejilion.sh')
    expect(view.find('.offers-showcase').exists()).toBe(false)
  })

  it('shows an empty state with the external list when the manifest has no active banners', async () => {
    const view = await mountOffers(snapshot([]))
    expect(view.text()).toContain('暂时没有可展示的广告')
    expect(view.find('.offers-empty a').attributes('href')).toBe('https://kejilion.pro/topvps/')
  })

  it('reports a failed request and retries it', async () => {
    const view = await mountOffers(new ApiError('会话已过期', 401, 'unauthorized'))
    expect(view.find('[role="alert"]').text()).toContain('会话已过期')
    list.mockResolvedValue(snapshot([offer('c')]))
    await view.find('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(view.find('[role="alert"]').exists()).toBe(false)
    expect(view.findAll('.offers-tile:not(.offers-tile--more)')).toHaveLength(1)
  })

  it('refreshes on demand and replaces a broken image with the vendor text', async () => {
    const view = await mountOffers(snapshot([offer('c')]))
    expect(view.find('.offers-updated').text()).toContain('更新于')
    await view.find('.offers-section-head button').trigger('click')
    await flushPromises()
    expect(list).toHaveBeenLastCalledWith(expect.objectContaining({ refresh: true }))

    await view.find('.offers-tile img').trigger('error')
    const fallback = view.find('.offers-tile .offers-fallback')
    expect(fallback.text()).toContain('厂商 c')
    expect(fallback.attributes('data-i18n-ignore')).toBeDefined()
  })
})
