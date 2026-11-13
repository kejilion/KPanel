// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import { tracksFromEntries, type Track } from './music'
import { musicQualityKey, readMusicQuality } from './musicQuality'
import { useMusicQuality } from './useMusicQuality'
import { clearMusicInfoMemory } from './musicInfoCache'
import { api } from './api'
import type { FileEntry } from '@/types/api'

vi.mock('./api', () => ({ api: { music: { cache: vi.fn(async () => ({ items: [] })) }, files: { contentUrl: (path: string) => path } } }))
vi.mock('./musicTags', () => ({ readMusicTags: vi.fn(async () => ({})) }))
vi.mock('./musicQuality', async importOriginal => ({ ...await importOriginal<typeof import('./musicQuality')>(), readMusicQuality: vi.fn() }))
const readers = vi.mocked(readMusicQuality)
let observed: Element[] = [], callback: IntersectionObserverCallback
const disconnect = vi.fn()
vi.stubGlobal('IntersectionObserver', class {
  constructor(cb: IntersectionObserverCallback) { callback = cb; observed = [] }
  observe(element: Element) { observed.push(element) }
  disconnect = disconnect
})
afterEach(() => { clearMusicInfoMemory(); readers.mockReset(); disconnect.mockClear(); vi.useRealTimers() })
function track(index: number): Track {
  return tracksFromEntries([{ name: index + '.mp3', path: '/music/' + index + '.mp3', kind: 'file', sizeBytes: 100, resourceVersion: '1' } as FileEntry], '')[0]!
}
function visible(elements = observed) {
  callback(elements.map(target => ({ target, isIntersecting: true }) as IntersectionObserverEntry), {} as IntersectionObserver)
}
async function setup(count = 5) {
  const tracks = ref(Array.from({ length: count }, (_, i) => track(i)))
  let quality!: ReturnType<typeof useMusicQuality>
  const wrapper = mount(defineComponent({ setup() {
    const root = ref<HTMLElement>()
    quality = useMusicQuality(root, () => tracks.value, () => 0)
    return () => h('main', { ref: root }, tracks.value.map(item => h('div', { 'data-music-quality': musicQualityKey(item) })))
  } }))
  await flushPromises()
  return { wrapper, tracks, quality }
}
it('probes visible rows only, limits concurrency, reuses cache and aborts when leaving', async () => {
  const pending: { resolve: (value: {}) => void; signal: AbortSignal }[] = []
  readers.mockImplementation((_track, signal) => new Promise(resolve => pending.push({ resolve, signal })))
  const { wrapper, tracks, quality } = await setup()
  expect(readers).not.toHaveBeenCalled()
  visible(); await flushPromises(); expect(readers).toHaveBeenCalledTimes(2)
  pending[0]!.resolve({ bitrate: 320 }); await flushPromises()
  expect(readers).toHaveBeenCalledTimes(3); expect(quality(tracks.value[0]!)).toEqual({ bitrate: 320 })
  visible([observed[0]!]); expect(readers).toHaveBeenCalledTimes(3)
  wrapper.unmount()
  expect(pending[1]!.signal.aborted).toBe(true); expect(pending[2]!.signal.aborted).toBe(true)
  pending[1]!.resolve({}); pending[2]!.resolve({}); await flushPromises()
  expect(readers).toHaveBeenCalledTimes(3); expect(disconnect).toHaveBeenCalled()
})
it('invalidates by host and resource version and bounds error cache', async () => {
  readers.mockRejectedValue(new Error('Unavailable'))
  const { wrapper, tracks, quality } = await setup(1)
  visible(); await flushPromises(); expect(quality(tracks.value[0]!)).toEqual({})
  tracks.value = [{ ...track(0), hostId: 'another-host' }]; await flushPromises(); visible(); await flushPromises()
  tracks.value = [{ ...track(0), entry: { ...track(0).entry, resourceVersion: '2' } }]; await flushPromises(); visible(); await flushPromises()
  expect(readers).toHaveBeenCalledTimes(3)
  const original = tracks.value[0]!
  // Submit in small visible batches so queue eviction does not affect cache coverage.
  for (let start = 1; start <= 130; start += 10) {
    tracks.value = Array.from({ length: 10 }, (_, i) => track(start + i)); await flushPromises(); visible(); await flushPromises()
  }
  expect(quality(original)).toBeUndefined(); expect(api.music.cache).toHaveBeenCalled()
  wrapper.unmount()
})

it('retries transient quality failures and stops retrying after success', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
  readers.mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ lossless: true, bitrate: 960 })
  const { wrapper, tracks, quality } = await setup(1)
  try {
    visible(); await flushPromises(); expect(readers).toHaveBeenCalledOnce()
    await vi.advanceTimersByTimeAsync(1000); await flushPromises()
    expect(readers).toHaveBeenCalledTimes(2); expect(quality(tracks.value[0]!)?.lossless).toBe(true)
    await vi.advanceTimersByTimeAsync(60000); expect(readers).toHaveBeenCalledTimes(2)
  } finally { wrapper.unmount() }
})

it('pauses retries offscreen, bounds automatic attempts and permits a later visible retry', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] })
  readers.mockRejectedValue(new Error('offline'))
  const { wrapper } = await setup(1)
  try {
    visible(); await flushPromises()
    callback(observed.map(target => ({ target, isIntersecting: false }) as IntersectionObserverEntry), {} as IntersectionObserver)
    await vi.advanceTimersByTimeAsync(1000); expect(readers).toHaveBeenCalledOnce()
    visible(); await flushPromises(); expect(readers).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(2000); await flushPromises(); expect(readers).toHaveBeenCalledTimes(3)
    await vi.advanceTimersByTimeAsync(60000); expect(readers).toHaveBeenCalledTimes(3)
    visible(); await flushPromises(); expect(readers).toHaveBeenCalledTimes(4)
  } finally { wrapper.unmount() }
})
