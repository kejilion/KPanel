// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '@/lib/api'
import type { OfferItem } from '@/types/api'
import { hasNewOffers, markOffersSeen, offerKey, refreshOffersNotice, resetOffersNoticeForTest } from './offersNotice'

const offer = (digest: string): OfferItem => ({
  id: digest, vendor: 'v', alt: 'a', featured: false, url: 'https://x.example/', host: 'x.example',
  card: `/api/v1/offers/media/${digest.repeat(64).slice(0, 64)}`,
})

beforeEach(() => {
  window.localStorage.clear()
  resetOffersNoticeForTest()
  vi.restoreAllMocks()
})

describe('offers notice', () => {
  it('stays quiet on the first run and lights up only for banners seen later', async () => {
    const list = vi.spyOn(api.offers, 'list').mockResolvedValue({ state: 'live', items: [offer('a')] })
    await refreshOffersNotice()
    expect(hasNewOffers.value).toBe(false)

    list.mockResolvedValue({ state: 'live', items: [offer('a'), offer('b')] })
    await refreshOffersNotice()
    expect(hasNewOffers.value).toBe(true)

    markOffersSeen([offer('a'), offer('b')])
    expect(hasNewOffers.value).toBe(false)
    expect(JSON.parse(window.localStorage.getItem('kpanel:offers-seen:v1')!)).toContain(offerKey(offer('b')))
  })

  it('does not record anything when no list could be fetched', async () => {
    vi.spyOn(api.offers, 'list').mockResolvedValue({ state: 'unavailable', items: [] })
    await refreshOffersNotice()
    vi.spyOn(api.offers, 'list').mockRejectedValue(new Error('offline'))
    await refreshOffersNotice()
    expect(hasNewOffers.value).toBe(false)
    expect(window.localStorage.getItem('kpanel:offers-seen:v1')).toBeNull()
  })
})
