// @vitest-environment jsdom
import { flushPromises, shallowMount } from '@vue/test-utils'
import { reactive } from 'vue'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { applyMediaLocation, mediaLocationKeys, mediaLocations } from '@/lib/mediaLocations'

const navigation = vi.hoisted(() => ({ query: {} as Record<string, string>, push: vi.fn() }))
vi.mock('vue-router', async (original) => ({
  ...await original<typeof import('vue-router')>(),
  useRoute: () => navigation,
  useRouter: () => ({ push: navigation.push }),
}))
vi.mock('@/lib/music', async (original) => ({
  ...await original<typeof import('@/lib/music')>(),
  readMusicFolder: vi.fn(async () => ({ entries: [], truncated: false })),
}))
vi.mock('@/lib/cinema', async (original) => ({
  ...await original<typeof import('@/lib/cinema')>(),
  readCinemaFolder: vi.fn(async () => ({ films: [], folders: [], images: new Map(), truncated: false })),
}))

import { api } from '@/lib/api'
import MusicView from './MusicView.vue'
import CinemaView from './CinemaView.vue'

beforeEach(() => {
  navigation.query = reactive({})
  navigation.push.mockReset()
  vi.spyOn(api.cluster, 'hosts').mockResolvedValue({ nodeId: 'local', items: [] } as never)
  localStorage.clear()
})
afterEach(() => {
  for (const key of mediaLocationKeys) applyMediaLocation(key, undefined)
  vi.restoreAllMocks()
})

it.each([
  ['musicLocation', MusicView, '/music/saved', 'navigate'],
  ['cinemaLocation', CinemaView, '/films/saved', 'browse'],
] as const)('%s restores the shared location but explicit links take priority', async (key, component, saved, method) => {
  applyMediaLocation(key, { path: saved, hostId: 'edge-1' })
  const view = shallowMount(component)
  await flushPromises()
  const vm = view.vm as unknown as { path: string; hostId: string; [key: string]: any }
  expect(vm.path).toBe(saved)
  expect(vm.hostId).toBe('edge-1')
  navigation.query.path = '/linked'
  delete navigation.query.hostId
  await flushPromises()
  expect(vm.path).toBe('/linked')
  expect(vm.hostId).toBe('')
  expect(mediaLocations[key]?.path).toBe(saved)
  vm[method]('/chosen', 'edge-2')
  expect(mediaLocations[key]).toEqual({ path: '/chosen', hostId: 'edge-2' })
  expect(navigation.push).toHaveBeenCalledWith(expect.objectContaining({ query: { path: '/chosen', hostId: 'edge-2' } }))
  view.unmount()
})
