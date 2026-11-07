// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, expect, it, vi } from 'vitest'
import type { FileEntry } from '@/types/api'

const harness = vi.hoisted(() => ({ list: vi.fn(), pause: vi.fn(), metadata: vi.fn(async () => ({ items: [] })) }))
vi.mock('@/lib/fileHostContext', () => ({ fileAPIForHost: () => ({ list: harness.list, contentUrl: (path: string) => `/content${path}` }) }))
vi.mock('@/stores/musicPlayer', () => ({ useMusicPlayer: () => ({ state: { playing: true }, pause: harness.pause }) }))
vi.mock('@/lib/api', async () => { const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api'); return { ...actual, api: { ...actual.api, cinema: { metadata: harness.metadata }, cluster: { hosts: async () => ({ items: [] }) } } } })
import CinemaView from './CinemaView.vue'
import { cinemaPlayback, setCinemaPlaybackOwner } from '@/lib/cinemaPlayback'

let view: VueWrapper | undefined
afterEach(() => { view?.unmount(); setCinemaPlaybackOwner(''); localStorage.clear(); vi.clearAllMocks() })
it('opens details before playback, returns to the same film, and preserves library search', async () => {
  setCinemaPlaybackOwner('admin')
  const entry: FileEntry = { name: 'Journey.mp4', path: '/home/videos/Journey.mp4', kind: 'file', sizeBytes: 2048, modifiedAt: '2026-11-06T00:00:00Z', mode: '-rw-r--r--', owner: 'root', group: 'root', resourceVersion: 'v1', editable: false, previewable: true }
  harness.list.mockResolvedValue({ entries: [entry] })
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/cinema', name: 'cinema', component: CinemaView }] })
  await router.push('/cinema'); await router.isReady()
  view = mount(CinemaView, { global: { plugins: [router], stubs: { HostSwitcher: true, GalleryVideo: true, CinemaPlayer: { inheritAttrs: false, props: ['labels'], emits: ['progress', 'close'], template: '<button class="test-player" @click="$emit(\'progress\', { position: 3, duration: 10, completed: false }, true); $emit(\'close\')">exit</button>' } } }, attachTo: document.body })
  await flushPromises()
  expect(view.find('.cinema-feature').exists()).toBe(false)
  expect(view.find('.cinema-detail').exists()).toBe(false)
  await view.get('.cinema-search input').setValue('Journey')
  await view.get('.cinema-card__play').trigger('click'); await flushPromises()
  expect(view.find('.cinema-detail').exists()).toBe(true)
  expect(view.find('.test-player').exists()).toBe(false)
  expect(harness.pause).not.toHaveBeenCalled()
  await view.get('.cinema-feature__play').trigger('click')
  expect(harness.pause).toHaveBeenCalledOnce()
  expect(view.find('.test-player').exists()).toBe(true)
  await router.back(); await new Promise(resolve => setTimeout(resolve, 10)); await flushPromises()
  expect(view.find('.test-player').exists()).toBe(false)
  expect(view.find('.cinema-detail').exists()).toBe(false)
  await router.forward(); await new Promise(resolve => setTimeout(resolve, 10)); await flushPromises()
  expect(view.get('.cinema-detail h2').text()).toBe('Journey')
  await view.get('.cinema-feature__play').trigger('click')
  await view.get('.test-player').trigger('click')
  expect(view.find('.test-player').exists()).toBe(false)
  expect(cinemaPlayback.entries[0]?.position).toBe(3)
  expect(view.get('.cinema-detail h2').text()).toBe('Journey')
  await view.get('.cinema-detail__back').trigger('click'); await new Promise(resolve => setTimeout(resolve, 10)); await flushPromises()
  expect(view.find('.cinema-detail').exists()).toBe(false)
  expect((view.get('.cinema-search input').element as HTMLInputElement).value).toBe('Journey')
  expect(document.activeElement).toBe(view.get('.cinema-card__play').element)
})
