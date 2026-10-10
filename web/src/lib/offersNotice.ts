import { computed, ref } from 'vue'
import { api } from '@/lib/api'
import type { OfferItem } from '@/types/api'

/**
 * "New banner" dot for the classic topbar. A banner is identified by its
 * content-addressed card image, so a replaced image under the same vendor
 * counts as new. Seen keys live only in this browser. The first run records
 * the current banners silently, so a fresh install never starts with a dot.
 */
const STORAGE_KEY = 'kpanel:offers-seen:v1'
const MAX_SEEN = 200

const current = ref<string[]>([])
const seen = ref<string[] | undefined>(readSeen())
let loading = false

function readSeen(): string[] | undefined {
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(STORAGE_KEY) || 'null')
    if (Array.isArray(parsed) && parsed.every((value) => typeof value === 'string')) return parsed.slice(0, MAX_SEEN)
  } catch {
    // Storage unavailable or damaged: behave like a first run.
  }
  return undefined
}

function writeSeen(keys: string[]): void {
  seen.value = keys.slice(0, MAX_SEEN)
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(seen.value))
  } catch {
    // The dot simply returns on the next load.
  }
}

export function offerKey(item: OfferItem): string {
  return item.card.slice(item.card.lastIndexOf('/') + 1)
}

/** Record what the visitor is looking at; called by the page itself. */
export function markOffersSeen(items: readonly OfferItem[]): void {
  const keys = items.map(offerKey)
  current.value = keys
  writeSeen([...new Set([...keys, ...(seen.value || [])])])
}

/** One background read per page load; failures never surface in the topbar. */
export async function refreshOffersNotice(): Promise<void> {
  if (loading) return
  loading = true
  try {
    const snapshot = await api.offers.list()
    const keys = snapshot.items.map(offerKey)
    current.value = keys
    if (seen.value === undefined && snapshot.state !== 'unavailable') writeSeen(keys)
  } catch {
    // Optional hint: stay quiet when the list cannot be read.
  } finally {
    loading = false
  }
}

export const hasNewOffers = computed(() => seen.value !== undefined && current.value.some((key) => !seen.value!.includes(key)))

export function resetOffersNoticeForTest(): void {
  current.value = []
  seen.value = readSeen()
  loading = false
}
