import { nextTick, onBeforeUnmount, shallowReactive, watch, type Ref } from 'vue'
import { hasMusicQuality, musicQualityKey, type MusicQuality } from './musicQuality'
import type { Track } from './music'
import { loadMusicInfo, peekMusicInfo } from './musicInfoCache'

/** Read only visible rows: two bounded header probes, 64 queued items, 128 cached results. */
export function useMusicQuality(root: Ref<HTMLElement | undefined>, tracks: () => Track[], revision: () => unknown) {
  const cache = shallowReactive(new Map<string, { quality: MusicQuality; attempts: number; retryAt: number }>())
  const pending = new Set<string>(), queue: { track: Track; key: string }[] = []
  const visible = new Map<Element, Track>()
  const controller = new AbortController()
  let active = 0, observer: IntersectionObserver | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  function remember(key: string, quality: MusicQuality, attempts = 0, retryAt = 0) {
    if (cache.size >= 128 && !cache.has(key)) cache.delete(cache.keys().next().value!)
    cache.set(key, { quality, attempts, retryAt })
  }
  function failed(key: string) {
    const previous = cache.get(key)?.attempts ?? 0, attempts = previous >= 3 ? 1 : previous + 1
    remember(key, {}, attempts, Date.now() + (attempts < 3 ? attempts * 1000 : 30000))
  }
  function scheduleRetry() {
    clearTimeout(retryTimer); retryTimer = undefined
    if (controller.signal.aborted) return
    let next = Infinity
    for (const track of visible.values()) {
      const key = musicQualityKey(track), entry = cache.get(key)
      if (entry && entry.attempts < 3 && !hasMusicQuality(entry.quality) && !hasMusicQuality(peekMusicInfo(track)?.quality) && !pending.has(key)) next = Math.min(next, entry.retryAt)
    }
    if (Number.isFinite(next)) retryTimer = setTimeout(() => {
      for (const track of visible.values()) enqueue(track)
      scheduleRetry()
    }, Math.max(0, next - Date.now()))
  }
  async function run(track: Track, key: string) {
    const probe = new AbortController()
    const stop = () => probe.abort()
    controller.signal.addEventListener('abort', stop, { once: true })
    const timer = setTimeout(stop, 6000)
    try {
      const info = await loadMusicInfo(track, probe.signal)
      const currentKey = musicQualityKey(track)
      const value = currentKey === key && hasMusicQuality(info.quality) ? info.quality : peekMusicInfo(track)?.quality
      if (!controller.signal.aborted) {
        if (hasMusicQuality(value)) remember(currentKey, value)
        else failed(currentKey)
      }
    } catch { if (!controller.signal.aborted) failed(musicQualityKey(track)) }
    finally {
      clearTimeout(timer); controller.signal.removeEventListener('abort', stop)
      pending.delete(key); active--; pump(); scheduleRetry()
    }
  }
  function pump() {
    while (!controller.signal.aborted && active < 2 && queue.length) {
      const item = queue.shift()!
      if (![...visible.values()].some(track => musicQualityKey(track) === item.key)) { pending.delete(item.key); continue }
      active++; void run(item.track, item.key)
    }
  }
  function enqueue(track: Track) {
    const key = musicQualityKey(track)
    const entry = cache.get(key)
    if (hasMusicQuality(entry?.quality) || (entry && Date.now() < entry.retryAt) || pending.has(key) || controller.signal.aborted) return
    if (queue.length >= 64) pending.delete(queue.shift()!.key)
    pending.add(key); queue.push({ track, key }); pump()
  }
  watch([root, tracks, revision], async () => {
    await nextTick()
    if (controller.signal.aborted || !root.value || typeof IntersectionObserver === 'undefined') return
    observer?.disconnect()
    visible.clear(); scheduleRetry()
    const available = new Map(tracks().map(track => [musicQualityKey(track), track]))
    const next = new IntersectionObserver(entries => {
      if (controller.signal.aborted || observer !== next) return
      for (const entry of entries) {
        const key = (entry.target as HTMLElement).dataset.musicQuality
        const track = key ? available.get(key) : undefined
        if (entry.isIntersecting && track) { visible.set(entry.target, track); enqueue(track) }
        else visible.delete(entry.target)
      }
      scheduleRetry()
    }, { rootMargin: '100px' })
    observer = next
    root.value.querySelectorAll('[data-music-quality]').forEach(element => observer!.observe(element))
  }, { immediate: true })
  onBeforeUnmount(() => { controller.abort(); observer?.disconnect(); clearTimeout(retryTimer); visible.clear(); queue.length = 0; pending.clear() })
  return (track: Track): MusicQuality | undefined => {
    const saved = peekMusicInfo(track)?.quality
    return hasMusicQuality(saved) ? saved : cache.get(musicQualityKey(track))?.quality
  }
}
