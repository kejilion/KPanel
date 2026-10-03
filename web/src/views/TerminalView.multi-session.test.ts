// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import { ApiError } from '@/lib/api'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), open: vi.fn(), next: 0 }))
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRoute: () => ({ path: '/terminal', query: {} }),
}))
vi.mock('@/lib/api', () => ({
  api: { cluster: { hosts: mocks.hosts }, terminals: { open: mocks.open } },
  ApiError: class extends Error { code = '' },
}))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(_, { expose }) {
    expose({ closeSession: () => Promise.resolve(), focusTerminal() {}, executeCommand: () => true, scheduleResize() {} })
    return () => null
  },
}) }))

const hosts = [
  { id: 'local', name: '本地主机', isLocal: true, kind: 'panel', origin: '', terminalAvailable: true },
  { id: 'tokyo', name: '东京节点', isLocal: false, kind: 'panel', origin: 'https://tokyo.example.com', terminalAvailable: true },
]

const tabNames = (wrapper: VueWrapper) => wrapper.findAll('.terminal-tab__name').map((tab) => tab.text())
const activeTab = (wrapper: VueWrapper) => wrapper.find('.terminal-tab.is-active .terminal-tab__name').text()
const hostRow = (wrapper: VueWrapper, name: string) => wrapper.findAll('.terminal-host').find((row) => row.text().includes(name))!
const newTerminalButton = (wrapper: VueWrapper) => wrapper.find('.terminal-tabs__new')

async function mountWithLocalTerminal() {
  const wrapper = mount(TerminalView, { attachTo: document.body })
  await flushPromises()
  expect(tabNames(wrapper)).toEqual(['本地主机'])
  return wrapper
}

describe('several terminals on one host', () => {
  beforeEach(() => {
    resetLocaleForTest()
    window.localStorage.clear()
    mocks.next = 0
    mocks.hosts.mockReset().mockResolvedValue({ items: hosts })
    mocks.open.mockReset().mockImplementation(async (hostId: string) => ({ sessionId: `${hostId}-${++mocks.next}`, offset: 0 }))
  })

  it('opens another terminal on the active host from the tab bar and numbers it', async () => {
    const wrapper = await mountWithLocalTerminal()
    const button = newTerminalButton(wrapper)
    expect(button.attributes('aria-label')).toBe('在 本地主机 上新建终端')
    await button.trigger('click')
    await flushPromises()
    expect(mocks.open.mock.calls.map(([hostId]) => hostId)).toEqual(['local', 'local'])
    expect(tabNames(wrapper)).toEqual(['本地主机', '本地主机 2'])
    expect(activeTab(wrapper)).toBe('本地主机 2')
    expect(hostRow(wrapper, '本地主机').text()).toContain('已打开 2 个')
    wrapper.unmount()
  })

  it('goes to the open terminal when the host is chosen in the list instead of opening another', async () => {
    const wrapper = await mountWithLocalTerminal()
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    await wrapper.findAll('.terminal-tab__select')[0]!.trigger('click')
    expect(activeTab(wrapper)).toBe('本地主机')
    await hostRow(wrapper, '本地主机').trigger('click')
    await flushPromises()
    expect(mocks.open).toHaveBeenCalledTimes(2)
    expect(activeTab(wrapper)).toBe('本地主机')
    wrapper.unmount()
  })

  it('keeps every number when another terminal on the host closes, and reuses the free one', async () => {
    const wrapper = await mountWithLocalTerminal()
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    await wrapper.findAll('.terminal-tab__close')[0]!.trigger('click')
    await flushPromises()
    expect(tabNames(wrapper)).toEqual(['本地主机 2'])
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    expect(tabNames(wrapper)).toEqual(['本地主机 2', '本地主机'])
    wrapper.unmount()
  })

  it('opens the new terminal on the host of the active tab', async () => {
    const wrapper = await mountWithLocalTerminal()
    await hostRow(wrapper, '东京节点').trigger('click')
    await flushPromises()
    expect(activeTab(wrapper)).toBe('东京节点')
    expect(newTerminalButton(wrapper).attributes('aria-label')).toBe('在 东京节点 上新建终端')
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    expect(mocks.open.mock.calls.map(([hostId]) => hostId)).toEqual(['local', 'tokyo', 'tokyo'])
    expect(tabNames(wrapper)).toEqual(['本地主机', '东京节点', '东京节点 2'])
    wrapper.unmount()
  })

  it('says that the limit counts terminals on all hosts together', async () => {
    const wrapper = await mountWithLocalTerminal()
    mocks.open.mockRejectedValueOnce(Object.assign(new ApiError('limit'), { code: 'terminal_limit' }))
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    expect(wrapper.find('.terminal-alert').text()).toBe('所有主机合计的终端数已达上限，请先关闭不用的终端。')
    expect(tabNames(wrapper)).toEqual(['本地主机'])
    wrapper.unmount()
  })

  it('renames the open tabs of a host renamed in the inventory, keeping their numbers', async () => {
    const wrapper = await mountWithLocalTerminal()
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    mocks.hosts.mockResolvedValue({ items: [{ ...hosts[0], name: '主控服务器' }, hosts[1]] })
    await wrapper.find('.terminal-connections__refresh').trigger('click')
    await flushPromises()
    expect(tabNames(wrapper)).toEqual(['主控服务器', '主控服务器 2'])
    await newTerminalButton(wrapper).trigger('click')
    await flushPromises()
    expect(tabNames(wrapper)).toEqual(['主控服务器', '主控服务器 2', '主控服务器 3'])
    wrapper.unmount()
  })

  it('cannot start a second open while one is in flight', async () => {
    const wrapper = await mountWithLocalTerminal()
    let release: (value: { sessionId: string; offset: number }) => void = () => {}
    mocks.open.mockImplementationOnce(() => new Promise((resolve) => { release = resolve }))
    await newTerminalButton(wrapper).trigger('click')
    expect(newTerminalButton(wrapper).attributes('disabled')).toBeDefined()
    await newTerminalButton(wrapper).trigger('click')
    expect(mocks.open).toHaveBeenCalledTimes(2)
    release({ sessionId: 'local-late', offset: 0 })
    await flushPromises()
    expect(tabNames(wrapper)).toEqual(['本地主机', '本地主机 2'])
    expect(newTerminalButton(wrapper).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })
})
