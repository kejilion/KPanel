// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), open: vi.fn() }))
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
  { id: 'light-ready', name: '香港轻量机', isLocal: false, kind: 'light_node', origin: '', terminalAvailable: true },
  { id: 'light-monitor', name: '仅监控节点', isLocal: false, kind: 'light_node', origin: '', terminalAvailable: false },
  { id: 'legacy', name: '旧版节点', isLocal: false, kind: 'panel', origin: 'https://legacy.example.com', terminalAvailable: false },
]

function hostButton(wrapper: ReturnType<typeof mount>, name: string) {
  const button = wrapper.findAll<HTMLButtonElement>('.terminal-host')
    .find((item) => item.text().includes(name))
  if (!button) throw new Error(`missing host button: ${name}`)
  return button
}

describe('terminal host list semantics', () => {
  beforeEach(() => {
    resetLocaleForTest()
    window.localStorage.clear()
    mocks.hosts.mockReset().mockResolvedValue({ items: hosts })
    mocks.open.mockReset()
      .mockResolvedValueOnce({ sessionId: 'local-session', offset: 0 })
      .mockResolvedValueOnce({ sessionId: 'panel-session', offset: 0 })
  })

  it('shows two concise semantic levels and keeps addresses out of the repeated rows', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()

    expect(wrapper.get('.terminal-connections__heading').text()).toBe('主机5 台')
    expect(wrapper.get<HTMLInputElement>('.terminal-search input').attributes('placeholder')).toBe('名称或地址')
    expect(hostButton(wrapper, '本地主机').text()).toContain('本机· 已打开')
    expect(hostButton(wrapper, '东京节点').text()).toContain('KPanel· 可连接')
    expect(hostButton(wrapper, '香港轻量机').text()).toContain('轻量节点· 可连接')
    expect(hostButton(wrapper, '仅监控节点').text()).toContain('轻量节点· 仅监控')
    expect(hostButton(wrapper, '旧版节点').text()).toContain('KPanel· 需重新配对')
    expect(wrapper.text()).not.toContain('当前 KPanel')
    expect(wrapper.text()).not.toContain('加密直连')
    expect(wrapper.text()).not.toContain('https://tokyo.example.com')
    expect(hostButton(wrapper, '东京节点').attributes('title')).toContain('https://tokyo.example.com')
    expect(hostButton(wrapper, '仅监控节点').attributes('aria-disabled')).toBe('true')

    wrapper.unmount()
  })

  it('keeps hidden addresses searchable and changes a newly opened host to 已打开', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()

    await wrapper.get<HTMLInputElement>('.terminal-search input').setValue('tokyo.example.com')
    expect(wrapper.findAll('.terminal-host')).toHaveLength(1)
    const panel = hostButton(wrapper, '东京节点')
    await panel.trigger('click')
    await flushPromises()

    expect(mocks.open).toHaveBeenLastCalledWith('panel', 30, 120)
    expect(hostButton(wrapper, '东京节点').text()).toContain('KPanel· 已打开')
    wrapper.unmount()
  })
})
