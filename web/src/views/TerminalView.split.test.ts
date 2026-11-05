// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, onUnmounted } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { resetLocaleForTest } from '@/i18n'
import TerminalView from './TerminalView.vue'
import TerminalQuickCommands from '@/components/terminal/TerminalQuickCommands.vue'

const mocks = vi.hoisted(() => ({ open: vi.fn(), close: vi.fn(), detach: vi.fn(), execute: vi.fn(), mounted: [] as string[], unmounted: [] as string[] }))
vi.mock('vue-router', async (original) => ({ ...await original<typeof import('vue-router')>(),
  useRoute: () => ({ path: '/terminal', query: {} }),
}))
vi.mock('@/lib/api', () => ({ api: { cluster: { hosts: async () => ({ items: [
  { id: 'local', name: '本地主机', isLocal: true, terminalAvailable: true },
] }) }, terminals: { list: () => Promise.resolve({ items: [] }), open: mocks.open, detach: mocks.detach } }, ApiError: class extends Error {} }))
vi.mock('@/components/terminal/TerminalQuickCommands.vue', () => ({ default: defineComponent({
  props: ['open', 'disabled'], emits: ['execute'], setup() { return () => null },
}) }))
vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  props: ['sessionId', 'visible', 'autoFocus'],
  setup(props, { expose }) {
    const id = String(props.sessionId)
    mocks.mounted.push(id)
    onUnmounted(() => mocks.unmounted.push(id))
    expose({ closeSession: () => mocks.close(id), focusTerminal() {}, scheduleResize() {},
      executeCommand: (command: string) => { mocks.execute(id, command); return true } })
    return () => h('input', { class: 'fake-composer' })
  },
}) }))

let resize: ResizeObserverCallback
let wrapper: VueWrapper
const visibleIds = () => wrapper.findAll('.terminal-pane[data-session-id]').filter((pane) => pane.isVisible()).map((pane) => pane.attributes('data-session-id'))
const layout = async (value: string) => {
  await wrapper.get('.terminal-layout-trigger').trigger('click')
  await wrapper.get(`[data-layout="${value}"]`).trigger('click')
  await flushPromises()
}
const add = async () => { await wrapper.get('.terminal-host__new').trigger('click'); await flushPromises() }

describe('terminal split workspace integration', () => {
  beforeEach(async () => {
    resetLocaleForTest()
    localStorage.clear()
    mocks.mounted.length = 0
    mocks.unmounted.length = 0
    mocks.execute.mockReset()
    mocks.close.mockReset().mockResolvedValue(undefined)
    mocks.detach.mockReset().mockResolvedValue({ detached: true })
    let id = 0
    mocks.open.mockReset().mockImplementation(async () => ({ sessionId: `s${++id}`, offset: 0 }))
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) { resize = callback }
      observe() {} disconnect() {}
    })
    wrapper = mount(TerminalView, { attachTo: document.body })
    await flushPromises()
  })
  afterEach(() => { wrapper.unmount(); vi.unstubAllGlobals() })

  it('orders panes like tabs when expanding and appends the next terminal', async () => {
    await add()
    await layout('columns')
    expect(wrapper.get('[data-session-id="s1"]').attributes('style')).toContain('grid-column: 1')
    expect(wrapper.get('[data-session-id="s2"]').attributes('style')).toContain('grid-column: 3')
    expect(wrapper.get('[data-session-id="s2"]').classes()).toContain('is-active')
    await layout('grid')
    await add()
    expect(wrapper.get('[data-session-id="s1"]').attributes('style')).toContain('grid-row: 1')
    expect(wrapper.get('[data-session-id="s3"]').attributes('style')).toContain('grid-row: 3')
    expect(wrapper.get('[data-session-id="s3"]').classes()).toContain('is-active')
  })

  it('synchronizes the command target when the active single pane is outside the new layout', async () => {
    await add(); await add(); await add()
    expect(visibleIds()).toEqual(['s4'])
    await layout('columns')
    expect(visibleIds()).toEqual(['s1', 's2'])
    expect(wrapper.get('[data-session-id="s1"]').classes()).toContain('is-active')
    wrapper.findComponent(TerminalQuickCommands).vm.$emit('execute', 'pwd')
    expect(mocks.execute).toHaveBeenLastCalledWith('s1', 'pwd')
  })

  it('keeps every terminal instance across layouts, maximize and batch round trips', async () => {
    await add(); await add(); await add()
    await layout('grid')
    expect(visibleIds()).toHaveLength(4)
    const pane = wrapper.get('[data-session-id="s1"]')
    await pane.get('[aria-label="最大化窗格"]').trigger('click')
    expect(visibleIds()).toEqual(['s1'])
    await wrapper.get('[data-session-id="s1"] .terminal-pane-header button').trigger('click')
    await wrapper.get('.terminal-mode-button').trigger('click')
    await wrapper.get('.terminal-mode-button').trigger('click')
    await layout('rows'); await layout('single'); await layout('grid')
    expect(visibleIds()).toHaveLength(4)
    expect(mocks.mounted).toHaveLength(4)
    expect(mocks.unmounted).toEqual([])
    expect(mocks.close).not.toHaveBeenCalled()
  })

  it('targets commands to the focused composer and disables commands in an empty pane', async () => {
    await add(); await layout('columns')
    await wrapper.get('[data-session-id="s1"] .fake-composer').trigger('focusin')
    wrapper.findComponent(TerminalQuickCommands).vm.$emit('execute', 'pwd')
    expect(mocks.execute).toHaveBeenLastCalledWith('s1', 'pwd')
    await layout('grid')
    await wrapper.findAll('.terminal-pane-empty button')[0]!.trigger('click')
    expect(wrapper.findComponent(TerminalQuickCommands).props('disabled')).toBe(true)
    wrapper.findComponent(TerminalQuickCommands).vm.$emit('execute', 'danger')
    expect(mocks.execute).toHaveBeenCalledTimes(1)
    await add()
    expect(visibleIds()).toHaveLength(3)
  })

  it('automatically queues a failed close and removes the pane without a prompt', async () => {
    await add(); await layout('columns')
    mocks.close.mockRejectedValueOnce(new Error('offline'))
    await wrapper.get('[data-session-id="s2"] [aria-label^="关闭终端"]').trigger('click')
    await flushPromises()
    expect(mocks.detach).toHaveBeenCalledWith('s2')
    expect(visibleIds()).toEqual(['s1'])
    expect(mocks.unmounted).toEqual(['s2'])
  })

  it('adjusts the separator by keyboard and preserves assignments in small windows', async () => {
    await add(); await layout('columns')
    await wrapper.get('[role="separator"]').trigger('keydown', { key: 'ArrowRight' })
    expect(wrapper.get('[role="separator"]').attributes('aria-valuenow')).toBe('55')
    const element = wrapper.get('.terminal-pane-workspace').element as HTMLElement
    vi.spyOn(element, 'clientWidth', 'get').mockReturnValue(500)
    vi.spyOn(element, 'clientHeight', 'get').mockReturnValue(500)
    resize([], {} as ResizeObserver)
    await flushPromises()
    expect(visibleIds()).toEqual(['s2'])
    expect(wrapper.find('[role="separator"]').exists()).toBe(false)
    vi.spyOn(element, 'clientWidth', 'get').mockReturnValue(1000)
    resize([], {} as ResizeObserver)
    await flushPromises()
    expect(new Set(visibleIds())).toEqual(new Set(['s1', 's2']))
    expect(wrapper.get('[role="separator"]').attributes('aria-valuenow')).toBe('55')
  })
})
