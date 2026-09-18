// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TerminalView from './TerminalView.vue'

const mocks = vi.hoisted(() => ({ hosts: vi.fn(), open: vi.fn(), terminalMounts: vi.fn(), batchApplies: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: { cluster: { hosts: mocks.hosts }, terminals: { open: mocks.open } },
  ApiError: class extends Error {},
}))

vi.mock('@/components/terminal/HostTerminal.vue', () => ({ default: defineComponent({
  name: 'HostTerminalStub',
  props: ['sessionId', 'hostName', 'initialOffset'],
  setup(props, { expose }) {
    mocks.terminalMounts(props.sessionId)
    expose({ closeSession: vi.fn(), focusTerminal() {}, scrollToTop() {}, scheduleResize() {} })
    return () => 'interactive terminal'
  },
}) }))

vi.mock('@/components/terminal/BatchTerminalPanel.vue', () => ({ default: defineComponent({
  name: 'BatchTerminalPanelStub',
  props: { hosts: { type: Array, default: () => [] }, sessionCapacity: Number },
  emits: ['runningChange'],
  setup(_props, { expose }) {
    expose({ applyQuickCommand: mocks.batchApplies })
  },
  template: '<div class="batch-panel-stub">{{ hosts.map((host) => host.name).join(",") }}</div>',
}) }))

describe('terminal batch mode', () => {
  beforeEach(() => {
    mocks.hosts.mockReset()
    mocks.open.mockReset()
    mocks.terminalMounts.mockReset()
    mocks.batchApplies.mockReset()
    mocks.hosts.mockResolvedValue({
      items: [
        { id: 'local', name: '本机', origin: '', isLocal: true, kind: 'panel', terminalAvailable: true },
        { id: 'edge', name: '生产环境', origin: 'https://edge.example.com', isLocal: false, kind: 'panel', terminalAvailable: true },
        { id: 'offline', name: '离线节点', origin: '', isLocal: false, kind: 'light_node', terminalAvailable: false },
      ],
    })
    mocks.open.mockResolvedValue({ sessionId: 'interactive-local', offset: 0 })
  })

  it('switches the host list to multi-select and preserves the interactive session', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()

    expect(mocks.open).toHaveBeenCalledTimes(1)
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(mocks.terminalMounts).toHaveBeenCalledTimes(1)
    expect(wrapper.get('.terminal-mode-button').text()).toContain('批量执行')

    await wrapper.get('.terminal-mode-button').trigger('click')

    expect(wrapper.get('.terminal-mode-button').text()).toContain('返回交互终端')
    expect(wrapper.findAll('.terminal-host--batch')).toHaveLength(3)
    const checkboxes = wrapper.findAll('.terminal-host--batch input[type="checkbox"]')
    expect(checkboxes).toHaveLength(3)
    expect(checkboxes[2]!.attributes('disabled')).toBeDefined()
    expect(wrapper.get('.batch-panel-stub').text()).toBe('')

    await checkboxes[1]!.setValue(true)
    expect(wrapper.get('.batch-panel-stub').text()).toBe('生产环境')

    await wrapper.get('.terminal-search input').setValue('本机')
    expect(wrapper.findAll('.terminal-host--batch')).toHaveLength(1)
    expect(wrapper.get('.batch-panel-stub').text()).toBe('生产环境')

    await wrapper.get('.terminal-mode-button').trigger('click')
    expect(wrapper.findAll('.terminal-tab')).toHaveLength(1)
    expect(mocks.terminalMounts).toHaveBeenCalledTimes(1)
    expect(mocks.open).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('routes batch-mode quick commands through the panel handle and closes the drawer', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()

    await wrapper.get('.terminal-mode-button').trigger('click')
    expect(mocks.batchApplies).not.toHaveBeenCalled()

    const batchStage = wrapper.findAll('.terminal-stage--batch')[0]
    const aside = batchStage?.findComponent({ name: 'TerminalQuickCommands' })
    expect(aside?.exists()).toBe(true)
    aside?.vm.$emit('execute', 'docker ps')
    await flushPromises()

    expect(mocks.batchApplies).toHaveBeenCalledTimes(1)
    expect(mocks.batchApplies).toHaveBeenCalledWith('docker ps')
    // F1 regression guard: closing the drawer is observable on the open prop.
    expect(aside?.props('open')).toBe(false)
    wrapper.unmount()
  })

  it('ignores batch quick commands while a run is active and keeps the drawer open', async () => {
    const wrapper = mount(TerminalView)
    await flushPromises()

    await wrapper.get('.terminal-mode-button').trigger('click')
    await flushPromises()

    // Open the batch drawer via its toolbar toggle.
    const batchStage = wrapper.findAll('.terminal-stage--batch')[0]
    const toolbarButtons = batchStage?.findAll('.terminal-toolbar button') ?? []
    const expandButton = toolbarButtons.find((button) => button.attributes('aria-label') === '展开快捷命令')
    await expandButton?.trigger('click')
    await flushPromises()

    const before = batchStage?.findComponent({ name: 'TerminalQuickCommands' })
    expect(before?.props('open')).toBe(true)

    // Drive the real batchRunning ref through the stubbed panel's event.
    const panel = wrapper.findComponent({ name: 'BatchTerminalPanelStub' })
    panel.vm.$emit('runningChange', true)
    await flushPromises()

    const aside = batchStage?.findComponent({ name: 'TerminalQuickCommands' })
    expect(aside?.props('open')).toBe(true)
    aside?.vm.$emit('execute', 'uptime')
    await flushPromises()

    expect(mocks.batchApplies).not.toHaveBeenCalledWith('uptime')
    expect(aside?.props('open')).toBe(true)
    wrapper.unmount()
  })
})
