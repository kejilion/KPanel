// @vitest-environment jsdom
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HostTerminal from './HostTerminal.vue'
import { resetTerminalHistoryForTest, useTerminalHistory } from '@/stores/terminalHistory'

const mocks = vi.hoisted(() => ({
  input: vi.fn(), captured: [] as string[], createCapture: vi.fn(), onCommand: undefined as ((command: string) => void) | undefined,
  reset: vi.fn(), dispose: vi.fn(), history: vi.fn(), appendHistory: vi.fn(),
}))
vi.mock('@/lib/api', async (original) => ({ ...await original<typeof import('@/lib/api')>(),
  terminalStream: { subscribe: () => ({ close() {} }) },
  api: { terminals: {
    inputTransport: () => Promise.resolve({ protocol: '' }), input: mocks.input,
    close: () => Promise.resolve({ closed: true }), resize: () => Promise.resolve({ accepted: true }),
    history: mocks.history, appendHistory: mocks.appendHistory,
  } },
}))
vi.mock('@/lib/terminalCommandCapture', () => ({
  createTerminalCommandCapture: (terminal: unknown, onCommand: (command: string) => void) => {
    mocks.createCapture(terminal)
    mocks.onCommand = onCommand
    return { input: (data: string) => mocks.captured.push(data), reset: mocks.reset, dispose: mocks.dispose }
  },
}))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {}, registerCsiHandler() { return { dispose() {} } } }
  rows = 24; cols = 80; buffer = { active: { viewportY: 0, baseY: 0 } }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {} scrollToBottom() {}
  write(_data: unknown, callback?: () => void) { callback?.() }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))

type Exposed = { fillCommand(command: string): boolean; executeCommand(command: string): boolean }
let wrapper: VueWrapper | undefined

function mountTerminal(historyHostId?: string) {
  wrapper = mount(HostTerminal, {
    attachTo: document.body,
    props: { sessionId: 'session', hostName: 'Local', initialOffset: 0, ...(historyHostId ? { historyHostId } : {}) },
  })
  return wrapper
}

describe('HostTerminal command history', () => {
  beforeEach(() => {
    resetTerminalHistoryForTest()
    vi.clearAllMocks()
    mocks.captured.length = 0
    mocks.onCommand = undefined
    mocks.input.mockResolvedValue({ accepted: true })
    mocks.history.mockResolvedValue({ schemaVersion: 1, available: true, items: [
      { command: 'df -h', hostId: 'host-a', at: 3 },
      { command: 'for i in 1\ndo echo\ndone', hostId: '', at: 2 },
      { command: 'uptime', hostId: 'host-b', at: 1 },
    ] })
    mocks.appendHistory.mockImplementation(async (body: { items: Array<{ command: string; hostId: string }> }) => ({
      schemaVersion: 1, available: true, items: body.items.map((item) => ({ ...item, at: 9 })),
    }))
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.stubGlobal('requestAnimationFrame', () => 0)
  })
  afterEach(() => {
    wrapper?.unmount()
    wrapper = undefined
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  it('leaves other terminals out of the shared history', async () => {
    mountTerminal()
    await flushPromises()
    expect(mocks.createCapture).not.toHaveBeenCalled()
    expect(mocks.history).not.toHaveBeenCalled()
  })

  it('observes typed and composed input and records commands for its host', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    expect(mocks.createCapture).toHaveBeenCalledTimes(1)
    expect(mocks.history).toHaveBeenCalledTimes(1)

    await view.get('.host-terminal__composer input').setValue('ls -la')
    await view.get('.host-terminal__composer').trigger('submit')
    ;(view.vm as unknown as Exposed).executeCommand('docker ps')
    expect(mocks.captured).toEqual(['ls -la\r', 'docker ps\r'])

    mocks.onCommand?.('ls -la')
    await flushPromises()
    expect(mocks.appendHistory).toHaveBeenCalledWith({ items: [{ command: 'ls -la', hostId: 'host-a' }] })
    expect(useTerminalHistory().history.value.items[0]?.command).toBe('ls -la')

    view.unmount()
    wrapper = undefined
    expect(mocks.dispose).toHaveBeenCalledTimes(1)
  })

  it('adds the mobile Space key to a composed host command before Enter submits it', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    const input = view.get<HTMLInputElement>('.host-terminal__composer input')
    await input.setValue('ls')
    await view.findAll('.host-terminal__mobile-key').find((button) => button.text() === 'Space')!.trigger('click')
    expect(input.element.value).toBe('ls ')
    await view.get('.host-terminal__composer').trigger('submit')
    expect(input.element.value).toBe('')
    expect(mocks.captured).toEqual(['ls \r'])
  })

  it('sends an empty return using the composer and its Enter key', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    await view.get('.host-terminal__composer').trigger('submit')
    await view.get('.host-terminal__composer input').trigger('keydown.enter')
    expect(mocks.captured).toEqual(['\r', '\r'])
  })

  it('sends mobile Ctrl+X immediately and preserves the draft', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    const input = view.get<HTMLInputElement>('.host-terminal__composer input')
    await input.setValue('draft')
    const ctrl = view.findAll('.host-terminal__mobile-key').find(button => button.text() === 'Ctrl')!
    await ctrl.trigger('click')
    expect(document.activeElement).toBe(input.element)
    const event = new InputEvent('beforeinput', { inputType: 'insertText', data: 'X', bubbles: true, cancelable: true })
    input.element.dispatchEvent(event)
    await view.vm.$nextTick()
    expect(event.defaultPrevented).toBe(true)
    expect(mocks.captured).toEqual(['\x18'])
    expect(input.element.value).toBe('draft')
    expect(ctrl.attributes('aria-pressed')).toBe('false')
  })

  it('fills the composer for review and refuses commands it cannot hold', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    const exposed = view.vm as unknown as Exposed
    expect(exposed.fillCommand('systemctl restart nginx')).toBe(true)
    await flushPromises()
    const input = view.get<HTMLInputElement>('.host-terminal__composer input')
    expect(input.element.value).toBe('systemctl restart nginx')
    expect(document.activeElement).toBe(input.element)
    expect(mocks.input).not.toHaveBeenCalled()
    expect(exposed.fillCommand('for i in 1\ndo echo\ndone')).toBe(false)
    expect(input.element.value).toBe('systemctl restart nginx')
  })

  it('does not raise the soft keyboard on touch devices', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: query.includes('pointer: coarse'), addEventListener() {}, removeEventListener() {} }))
    const view = mountTerminal('host-a')
    await flushPromises()
    expect((view.vm as unknown as Exposed).fillCommand('uptime')).toBe(true)
    await flushPromises()
    const input = view.get<HTMLInputElement>('.host-terminal__composer input')
    expect(input.element.value).toBe('uptime')
    expect(document.activeElement).not.toBe(input.element)
  })

  it('browses single-line history with arrow keys and returns to the draft', async () => {
    const view = mountTerminal('host-a')
    await flushPromises()
    const input = view.get<HTMLInputElement>('.host-terminal__composer input')
    await input.setValue('draft')
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(input.element.value).toBe('df -h')
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(input.element.value).toBe('uptime')
    await input.trigger('keydown', { key: 'ArrowUp' })
    expect(input.element.value).toBe('uptime')
    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(input.element.value).toBe('df -h')
    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(input.element.value).toBe('draft')
    await input.trigger('keydown', { key: 'ArrowUp', isComposing: true })
    expect(input.element.value).toBe('draft')
  })
})
