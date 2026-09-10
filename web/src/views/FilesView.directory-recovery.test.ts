// @vitest-environment jsdom
import { shallowMount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import FilesView from './FilesView.vue'
import { createWindowRouter, reactiveRouteFor, synchronizeWindowRoute } from '@/lib/desktopWindowRoute'
import { windowRouterKey, windowRouteKey } from '@/lib/desktopRouteKeys'
import { resetApiSecurityState } from '@/lib/api'

const toast = vi.hoisted(() => ({ show: vi.fn(), success: vi.fn(), danger: vi.fn() }))
vi.mock('@/stores/toast', () => ({ useToast: () => toast }))

let wrapper: ReturnType<typeof shallowMount> | undefined
let listReply: (url: URL) => Response
const reads: URL[] = []
const hosts = ['local', 'a', 'b'].map(id => ({
  id, name: id, isLocal: id === 'local', kind: id === 'local' ? 'panel' : 'light-node',
  state: 'online', fileManagementAvailable: true,
}))
function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), { status, headers: { 'content-type': 'application/json' } })
}
function unavailable() {
  return json({ code: 'file_relay_unavailable', detail: '远端主机文件代理未连接' }, 503)
}
function directory(url: URL, entries: unknown[] = []) {
  return json({ path: url.searchParams.get('path'), entries, offset: 0, truncated: false })
}
async function mountAt(path = '/files?path=/target&hostId=a') {
  const router = createWindowRouter(path)
  await router.isReady()
  wrapper = shallowMount(FilesView, { global: { provide: {
    [windowRouterKey as symbol]: router, [windowRouteKey as symbol]: reactiveRouteFor(router),
  } } })
  await flushPromises()
  return router
}
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  vi.clearAllMocks()
  resetApiSecurityState()
  reads.length = 0
  window.scrollTo = vi.fn()
  listReply = unavailable
  vi.stubGlobal('fetch', vi.fn(async (input: string) => {
    const url = new URL(input, 'http://localhost')
    if (url.pathname.endsWith('/cluster/hosts')) return json({ items: hosts })
    if (url.pathname.endsWith('/files')) { reads.push(url); return listReply(url) }
    return json({ items: [] })
  }))
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

it('surfaces a remote directory failure without scheduling duplicate reads', async () => {
  await mountAt()
  expect(reads).toHaveLength(1)
  expect(wrapper!.find('.file-directory-error').text()).toContain('/target')
  expect(wrapper!.text()).not.toContain('这个文件夹是空的')
  expect(toast.danger).toHaveBeenCalledTimes(1)
  await vi.advanceTimersByTimeAsync(60000)
  expect(reads).toHaveLength(1)
})

it('keeps a manual retry tied to the requested path', async () => {
  const router = await mountAt()
  listReply = url => directory(url)
  await wrapper!.get('.file-directory-error button').trigger('click')
  await flushPromises()
  expect(reads).toHaveLength(2)
  expect(reads.at(-1)!.searchParams.get('path')).toBe('/target')
  expect(router.currentRoute.value.query.path).toBe('/target')
  expect(wrapper!.find('.file-directory-error').exists()).toBe(false)
  expect(wrapper!.text()).toContain('这个文件夹是空的')
})

it.each([
  { path: '/files?path=/target&hostId=a', status: 403, code: 'file_permission_denied' },
  { path: '/files?path=/target', status: 503, code: 'file_relay_unavailable' },
  { path: '/files?path=/target&hostId=a', status: 429, code: 'file_relay_rate_limited' },
])('does not retry unrelated failures: $code at $path', async ({ path, status, code }) => {
  listReply = () => json({ code, detail: 'cannot list directory' }, status)
  await mountAt(path)
  await vi.advanceTimersByTimeAsync(10000)
  expect(reads).toHaveLength(1)
  expect(wrapper!.find('.file-directory-error').exists()).toBe(true)
})

it('switches hosts without replaying the failed host request', async () => {
  const router = await mountAt()
  listReply = url => directory(url, [{ name: 'b.txt', path: '/b.txt', kind: 'file' }])
  synchronizeWindowRoute(router, '/files?path=/&hostId=b')
  await flushPromises()
  await vi.advanceTimersByTimeAsync(10000)
  expect(reads.map(url => url.searchParams.get('hostId'))).toEqual(['a', 'b'])
  expect(wrapper!.text()).toContain('b.txt')
  expect(wrapper!.find('.file-directory-error').exists()).toBe(false)
  expect(toast.danger).toHaveBeenCalledTimes(1)
})

it('keeps the last successful listing and retries a failed next page at its original offset', async () => {
  listReply = url => json({ path: url.searchParams.get('path'), entries: [
    { name: 'first.txt', path: '/target/first.txt', kind: 'file' },
  ], offset: 0, nextOffset: 100, truncated: true })
  await mountAt()
  listReply = unavailable
  await wrapper!.get('.file-limit button').trigger('click')
  await flushPromises()
  expect(wrapper!.text()).toContain('first.txt')
  expect(wrapper!.find('.file-directory-error').exists()).toBe(true)
  listReply = url => directory(url, [{ name: 'second.txt', path: '/target/second.txt', kind: 'file' }])
  await wrapper!.get('.file-directory-error button').trigger('click')
  await flushPromises()
  expect(reads.at(-1)!.searchParams.get('offset')).toBe('100')
  expect(wrapper!.text()).toContain('first.txt')
  expect(wrapper!.text()).toContain('second.txt')
})

it('does not issue another read after the file window closes', async () => {
  await mountAt()
  wrapper!.unmount()
  wrapper = undefined
  await vi.advanceTimersByTimeAsync(10000)
  expect(reads).toHaveLength(1)
})
