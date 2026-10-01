// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), open: vi.fn(), route: { current: undefined as { path: string; query: Record<string, string> } | undefined } }))
vi.mock('vue-router', async (importOriginal) => {
  const { reactive } = await import('vue')
  mocks.route.current = reactive({ path: '/terminal', query: {} as Record<string, string> })
  return { ...(await importOriginal<typeof import('vue-router')>()), useRoute: () => mocks.route.current }
})
vi.mock('@/lib/api', () => ({
  api: { cluster: { hosts: mocks.hosts }, terminals: { open: mocks.open } },
  ApiError: class extends Error {},
}))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(_, { expose }) {
    expose({ closeSession: vi.fn(), focusTerminal() {}, scrollToTop() {}, scheduleResize() {} })
    return () => null
  },
}) }))

const hosts = [
  { id: 'local', name: '本地主机', isLocal: true, kind: 'panel', origin: '', terminalAvailable: true },
  { id: 'panel', name: '东京节点', isLocal: false, kind: 'panel', origin: 'https://tokyo.example.com', terminalAvailable: true },
  { id: 'legacy', name: '旧版节点', isLocal: false, kind: 'panel', origin: 'https://legacy.example.com', terminalAvailable: false },
]

describe('terminal host requested by the cluster page', () => {
  beforeEach(() => {
    resetLocaleForTest()
    window.localStorage.clear()
    mocks.route.current!.path = '/terminal'
    mocks.route.current!.query = {}
    mocks.hosts.mockReset().mockResolvedValue({ items: hosts })
    mocks.open.mockReset().mockImplementation(async (hostId: string) => ({ sessionId: `${hostId}-session`, offset: 0 }))
  })

  it('opens the requested host instead of the local host on first load', async () => {
    mocks.route.current!.query = { hostId: 'panel' }
    const wrapper = mount(TerminalView)
    await flushPromises()

    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(mocks.open).toHaveBeenCalledWith('panel', 30, 120)
    wrapper.unmount()
  })

  it('falls back to the local host when the requested host has no terminal', async () => {
    mocks.route.current!.query = { hostId: 'legacy' }
    const wrapper = mount(TerminalView)
    await flushPromises()

    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(mocks.open).toHaveBeenCalledWith('local', 30, 120)
    wrapper.unmount()
  })

  it('ignores another page host query while the shared route is leaving the terminal', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)

    mocks.route.current!.path = '/monitoring'
    mocks.route.current!.query = { hostId: 'panel' }
    await flushPromises()

    expect(mocks.open).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('opens a newly requested host in an already mounted view without duplicating its session', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(1)

    mocks.route.current!.query = { hostId: 'panel' }
    await flushPromises()
    expect(mocks.open).toHaveBeenLastCalledWith('panel', 30, 120)
    expect(mocks.open).toHaveBeenCalledTimes(2)

    mocks.route.current!.query = { hostId: 'local' }
    await flushPromises()
    mocks.route.current!.query = { hostId: 'panel' }
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
