// @vitest-environment jsdom
import { mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HostTerminal from './HostTerminal.vue'

const mocks = vi.hoisted(() => ({ output: vi.fn(), resize: vi.fn(), writes: [] as string[] }))
vi.mock('@/lib/api', async (original) => ({ ...await original<typeof import('@/lib/api')>(), api: { terminals: {
  close: () => Promise.resolve({ closed: true }), output: mocks.output, resize: mocks.resize,
} } }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80; buffer = { active: { viewportY: 0, baseY: 0 } }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {} scrollToBottom() {}
  write(data: string) { mocks.writes.push(String(data)) }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
const props = { sessionId: 'resize-id', hostName: 'Local', initialOffset: 0 }
const idleChunk = { data: '', offset: 0, nextOffset: 0, truncated: false, exitedAt: '', exitError: '', closed: false }

describe('HostTerminal resize sync', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.output.mockReset()
    mocks.resize.mockReset()
    mocks.writes.length = 0
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.stubGlobal('requestAnimationFrame', () => 0)
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('retries a failed resize instead of recording the unsent size as synced', async () => {
    mocks.output.mockReturnValue(new Promise(() => {}))
    mocks.resize.mockRejectedValueOnce(new Error('timeout')).mockResolvedValue({ accepted: true })
    const wrapper = mount(HostTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(1)
    expect(mocks.writes.some((line) => line.includes('[KPanel]'))).toBe(true)
    await vi.advanceTimersByTimeAsync(500)
    expect(mocks.resize).toHaveBeenCalledTimes(2)
    expect(mocks.resize).toHaveBeenLastCalledWith('resize-id', 24, 80)
    // Once synced, an unchanged size is not resent.
    ;(wrapper.vm as unknown as { scheduleResize(): void }).scheduleResize()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('stops retrying after the bounded limit', async () => {
    mocks.output.mockReturnValue(new Promise(() => {}))
    mocks.resize.mockRejectedValue(new Error('down'))
    const wrapper = mount(HostTerminal, { props })
    await vi.advanceTimersByTimeAsync(100 + 500 + 1000 + 2000 + 10000)
    expect(mocks.resize).toHaveBeenCalledTimes(4)
    expect(mocks.writes.filter((line) => line.includes('[KPanel]'))).toHaveLength(1)
    wrapper.unmount()
  })

  it('resends the last size after reconnecting', async () => {
    mocks.resize.mockResolvedValue({ accepted: true })
    mocks.output
      .mockResolvedValueOnce(idleChunk)
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(idleChunk)
      .mockReturnValue(new Promise(() => {}))
    const wrapper = mount(HostTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(500 + 100)
    expect(mocks.output).toHaveBeenCalledTimes(4)
    expect(mocks.resize).toHaveBeenCalledTimes(2)
    expect(mocks.resize).toHaveBeenLastCalledWith('resize-id', 24, 80)
    wrapper.unmount()
  })
})
