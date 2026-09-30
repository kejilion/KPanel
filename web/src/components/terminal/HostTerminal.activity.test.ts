// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import HostTerminal from './HostTerminal.vue'
import { terminalStream } from '@/lib/api'
import { desktopWindowActiveKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import type { TerminalStreamHandlers, TerminalStreamTarget } from '@/lib/terminalStream'

const mocks = vi.hoisted(() => ({ output: vi.fn(), signals: [] as AbortSignal[], deferWrites: false, pending: [] as Array<() => void> }))
vi.mock('@/lib/api', async (original) => ({ ...await original<typeof import('@/lib/api')>(), api: { terminals: {
  close: () => Promise.resolve({ closed: true }), output: mocks.output, resize: () => Promise.resolve({ accepted: true }),
} } }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80; buffer = { active: { viewportY: 0, baseY: 0 } }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {} scrollToBottom() {}
  write(_data: unknown, callback?: () => void) {
    if (mocks.deferWrites && callback) mocks.pending.push(callback)
    else callback?.()
  }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))

describe('HostTerminal in a background desktop window', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.signals.length = 0
    mocks.pending.length = 0
    mocks.deferWrites = false
    let served = false
    mocks.output.mockReset().mockImplementation((_id: string, _offset: number, signal: AbortSignal) => {
      mocks.signals.push(signal)
      if (served) return new Promise(() => {})
      served = true
      return Promise.resolve({ data: 'aGk=', offset: 0, nextOffset: 2, truncated: false, exitedAt: '', exitError: '', closed: false })
    })
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.stubGlobal('requestAnimationFrame', () => 0)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('streams while visible but unfocused and pauses only while minimized', async () => {
    const visible = ref(true)
    const wrapper = mount(HostTerminal, {
      props: { sessionId: 'background-id', hostName: 'Local', initialOffset: 0 },
      global: { provide: { [desktopWindowActiveKey as symbol]: ref(false), [desktopWindowVisibleKey as symbol]: visible } },
    })
    await vi.advanceTimersByTimeAsync(0)
    expect(mocks.output).toHaveBeenCalledTimes(2)
    expect(mocks.output).toHaveBeenLastCalledWith('background-id', 2, expect.any(AbortSignal))

    visible.value = false
    await flushPromises()
    expect(mocks.signals.at(-1)!.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(mocks.output).toHaveBeenCalledTimes(2)

    visible.value = true
    await flushPromises()
    expect(mocks.output).toHaveBeenCalledTimes(3)
    expect(mocks.output).toHaveBeenLastCalledWith('background-id', 2, expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('stops reading while xterm is backlogged and resumes from the same offset', async () => {
    mocks.deferWrites = true
    const flood = Buffer.alloc(600 << 10, 97).toString('base64')
    let served = false
    mocks.output.mockReset().mockImplementation(() => {
      if (served) return new Promise(() => {})
      served = true
      return Promise.resolve({ data: flood, offset: 0, nextOffset: 600 << 10, truncated: false, exitedAt: '', exitError: '', closed: false })
    })
    const wrapper = mount(HostTerminal, { props: { sessionId: 'flood-id', hostName: 'Local', initialOffset: 0 } })
    await vi.advanceTimersByTimeAsync(1000)
    expect(mocks.output).toHaveBeenCalledTimes(1)

    for (const parsed of mocks.pending.splice(0)) parsed()
    await flushPromises()
    expect(mocks.output).toHaveBeenCalledTimes(2)
    expect(mocks.output).toHaveBeenLastCalledWith('flood-id', 600 << 10, expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('closes the push subscription while backlogged and resubscribes at the same offset', async () => {
    mocks.deferWrites = true
    const targets: TerminalStreamTarget[] = []
    const handlers: TerminalStreamHandlers[] = []
    const close = vi.fn()
    const subscribe = vi.spyOn(terminalStream, 'subscribe').mockImplementation((target, handler) => {
      targets.push({ ...target })
      handlers.push(handler)
      return { close }
    })
    const wrapper = mount(HostTerminal, { props: { sessionId: 'push-id', hostName: 'Local', initialOffset: 0 } })
    const flood = Buffer.alloc(600 << 10, 97).toString('base64')
    handlers[0]!.output?.({ data: flood, offset: 0, nextOffset: 600 << 10, truncated: false, exitedAt: '', exitError: '', closed: false })
    expect(close).toHaveBeenCalledTimes(1)

    for (const parsed of mocks.pending.splice(0)) parsed()
    expect(subscribe).toHaveBeenCalledTimes(2)
    expect(targets[1]).toMatchObject({ id: 'push-id', offset: 600 << 10 })
    wrapper.unmount()
    subscribe.mockRestore()
  })
})
