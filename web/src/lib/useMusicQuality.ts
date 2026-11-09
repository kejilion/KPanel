import { nextTick, onBeforeUnmount, shallowReactive, watch, type Ref } from 'vue'
import { musicQualityKey, type MusicQuality } from './musicQuality'
import type { Track } from './music'
import { loadMusicInfo, peekMusicInfo } from './musicInfoCache'

/** Read only visible rows: two bounded header probes, 64 queued items, 128 cached results. */
export function useMusicQuality(root: Ref<HTMLElement | undefined>, tracks: () => Track[], revision: () => unknown) {
  const cache = shallowReactive(new Map<string, MusicQuality>())
  const pending = new Set<string>(), queue: Track[] = []
  const controller = new AbortController()
  let active = 0, observer: IntersectionObserver | undefined
  function remember(key: string, value: MusicQuality) {
    if (cache.size >= 128 && !cache.has(key)) cache.delete(cache.keys().next().value!)
    cache.set(key, value)
  }
  async function run(track: Track) {
    const key = musicQualityKey(track), probe = new AbortController()
    const stop = () => probe.abort()
    controller.signal.addEventListener('abort', stop, { once: true })
    const timer = setTimeout(stop, 6000)
    try {
      const value = (await loadMusicInfo(track, probe.signal)).quality ?? {}
      if (!controller.signal.aborted) {
        remember(key, value)
      }
    } catch { if (!controller.signal.aborted) remember(key, {}) }
    finally {
      clearTimeout(timer); controller.signal.removeEventListener('abort', stop)
      pending.delete(key); active--; pump()
    }
  }
  function pump() {
    while (!controller.signal.aborted && active < 2 && queue.length) { active++; void run(queue.shift()!) }
  }
  function enqueue(track: Track) {
    const key = musicQualityKey(track)
    if (cache.has(key) || pending.has(key) || controller.signal.aborted) return
    if (queue.length >= 64) pending.delete(musicQualityKey(queue.shift()!))
    pending.add(key); queue.push(track); pump()
  }
  watch([root, tracks, revision], async () => {
    await nextTick()
    if (controller.signal.aborted || !root.value || typeof IntersectionObserver === 'undefined') return
    observer?.disconnect()
    const available = new Map(tracks().map(track => [musicQualityKey(track), track]))
    observer = new IntersectionObserver(entries => {
      for (const entry of entries) {
        const key = (entry.target as HTMLElement).dataset.musicQuality
        const track = key ? available.get(key) : undefined
        if (entry.isIntersecting && track) enqueue(track)
      }
    }, { rootMargin: '100px' })
    root.value.querySelectorAll('[data-music-quality]').forEach(element => observer!.observe(element))
  }, { immediate: true })
  onBeforeUnmount(() => { controller.abort(); observer?.disconnect(); queue.length = 0; pending.clear() })
  return (track: Track): MusicQuality | undefined => peekMusicInfo(track)?.quality ?? cache.get(musicQualityKey(track))
}
