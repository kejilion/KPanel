// @vitest-environment jsdom
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { resetApiSecurityState } from '@/lib/api'
import { resetFileWindowTransferForTest } from '@/lib/fileWindowTransfer'

const router = vi.hoisted(() => ({ push: vi.fn() }))

vi.mock('vue-router', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-router')>(),
  useRoute: () => ({ query: {} }), useRouter: () => router,
}))
vi.mock('@/stores/toast', () => ({ useToast: () => ({ show: vi.fn(), success: vi.fn(), danger: vi.fn() }) }))

const hosts = ['local', 'a', 'b'].map((id) => ({
  id, name: id, isLocal: id === 'local', kind: 'panel', state: 'online',
  fileManagementAvailable: true, remoteNodeId: id.repeat(32),
}))
const wrappers: ReturnType<typeof shallowMount>[] = []

beforeEach(() => {
  router.push.mockReset()
  resetApiSecurityState()
  resetFileWindowTransferForTest()
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost')
    let body: unknown = {}
    if (url.pathname.endsWith('/cluster/hosts')) body = { nodeId: 'c'.repeat(32), items: hosts }
    if (url.pathname.endsWith('/files')) body = { path: url.searchParams.get('path'), entries: [] }
    if (url.pathname.endsWith('/remote-downloads')) body = { items: [] }
    return new Response(JSON.stringify(body), { headers: { 'content-type': 'application/json' } })
  }))
})

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount()
  vi.unstubAllGlobals()
})

async function pane(hostId: string, path: string) {
  const wrapper = shallowMount(FilesView)
  wrappers.push(wrapper)
  await flushPromises()
  const vm = wrapper.vm as unknown as Record<string, any>
  vm.handleFileHostSelection(hosts.find((host) => host.id === hostId))
  await flushPromises()
  await vm.loadDirectory(path)
  return vm
}

describe('file pane gallery entry', () => {
  it('opens the gallery for the pane\'s own host and folder, independently per pane', async () => {
    const left = await pane('a', '/srv/photos')
    const right = await pane('b', '/home/me/trips')

    router.push.mockReset()
    right.openGallery()
    left.openGallery()

    expect(router.push.mock.calls).toEqual([
      [{ name: 'gallery', query: { path: '/home/me/trips', hostId: 'b' } }],
      [{ name: 'gallery', query: { path: '/srv/photos', hostId: 'a' } }],
    ])
  })

  it('sends a remote pane at / to that host\'s library and the local pane without a host', async () => {
    const remote = await pane('a', '/')
    const local = await pane('local', '/home/gallery/Kyoto')

    router.push.mockReset()
    remote.openGallery()
    local.openGallery()

    // The root would sweep every system folder, so the gallery picks the library folder on that host.
    expect(router.push.mock.calls[0]).toEqual([{ name: 'gallery', query: { hostId: 'a' } }])
    const localQuery = router.push.mock.calls[1]![0].query
    expect(localQuery.path).toBe('/home/gallery/Kyoto')
    expect(localQuery.hostId ?? '').toBe('')
  })
})
