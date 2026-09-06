// @vitest-environment jsdom
import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ close: vi.fn(), hosts: vi.fn(), open: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: mocks.hosts }, terminals: { open: mocks.open } }, ApiError: class extends Error {} }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(_, { expose }) {
    expose({ closeSession: mocks.close, focusTerminal() {}, scrollToTop() {}, scheduleResize() {} })
    return () => null
  },
}) }))

describe('explicit terminal tab close', () => {
  beforeEach(() => {
    mocks.close.mockReset()
    mocks.hosts.mockResolvedValue({ items: [{ id: 'local', name: 'Local host', isLocal: true, terminalAvailable: true }] })
    mocks.open.mockResolvedValue({ sessionId: 'same-session', offset: 0 })
  })

  it('keeps the tab on failure, deduplicates clicks and removes only after retry confirms close', async () => {
    let reject!: (reason: Error) => void
    mocks.close.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = mount(TerminalView)
    await flushPromises()
    const close = wrapper.get('[aria-label="关闭终端"]')
    await close.trigger('click')
    await close.trigger('click')
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(wrapper.find('[aria-label="正在关闭终端"]').exists()).toBe(true)
    reject(new Error('Agent disconnected'))
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('会话已保留')
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    mocks.close.mockResolvedValueOnce(undefined)
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('allows closing with the keyboard', async () => {
    mocks.close.mockResolvedValue(undefined)
    const wrapper = mount(TerminalView)
    await flushPromises()
    await wrapper.get('[aria-label="关闭终端"]').trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(0)
    wrapper.unmount()
  })
})
