// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppInteractiveTerminal from './AppInteractiveTerminal.vue'
import type { TerminalStreamHandlers } from '@/lib/terminalStream'

const mocks = vi.hoisted(() => ({
  resize: vi.fn(), output: vi.fn(), subscribe: vi.fn(), handlers: undefined as TerminalStreamHandlers | undefined,
  observer: undefined as undefined | (() => void),
  terminal: undefined as undefined | { rows: number; cols: number }, writes: [] as string[],
}))
vi.mock('@/lib/api', () => ({ api: { apps: { terminalResize: mocks.resize, terminal: mocks.output },
    // An Agent without the acknowledged input protocol: input stays on the per-request route.
    jobTerminals: { inputTransport: () => Promise.resolve({ protocol: '' }) } },
  terminalStream: { subscribe: mocks.subscribe } }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80
  buffer = { active: { viewportY: 0, baseY: 0 } }
  constructor() { mocks.terminal = this }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {} scrollToBottom() {} reset() {}
  write(data: Uint8Array, callback?: () => void) { mocks.writes.push(new TextDecoder().decode(data)); callback?.() }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
const props = { jobId: 'job-a', inputOpen: true }
const idleChunk = { dataBase64: '', nextOffset: 0, inputOpen: true, finished: false }

describe('application script terminal geometry', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.resize.mockReset().mockResolvedValue({ accepted: true })
    mocks.output.mockReset().mockReturnValue(new Promise(() => {}))
    mocks.subscribe.mockReset().mockReturnValue(null)
    mocks.writes.length = 0
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(800)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(400)
    vi.stubGlobal('ResizeObserver', class { constructor(callback: () => void) { mocks.observer = callback }; observe() {} disconnect() {} })
    vi.stubGlobal('requestAnimationFrame', () => 0)
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it('sends actual geometry on attachment, resize/fullscreen, and reopening', async () => {
    const wrapper = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-a', 24, 80)
    mocks.terminal!.rows = 40; mocks.terminal!.cols = 140
    mocks.observer?.()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-a', 40, 140)
    mocks.observer?.()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(2)
    wrapper.unmount()
    const reopened = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(3)
    reopened.unmount()
  })

  it('keeps the latest size when resize occurs during a request', async () => {
    let finish!: (value: { accepted: boolean }) => void
    mocks.resize.mockReturnValueOnce(new Promise((resolve) => { finish = resolve }))
    const wrapper = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    mocks.terminal!.cols = 100
    mocks.observer?.()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(1)
    finish({ accepted: true }); await flushPromises()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-a', 24, 100)
    wrapper.unmount()
  })

  it('retries failures within a bound and never marks a rejected size synced', async () => {
    mocks.resize.mockRejectedValue(new Error('down'))
    const wrapper = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100 + 500 + 1000 + 2000 + 10000)
    expect(mocks.resize).toHaveBeenCalledTimes(4)
    expect(mocks.writes.filter((text) => text.includes('[KPanel]'))).toHaveLength(1)
    mocks.resize.mockResolvedValue({ accepted: true })
    mocks.observer?.()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(5)
    wrapper.unmount()
  })

  it('resynchronizes when output reconnects', async () => {
    mocks.output.mockResolvedValueOnce(idleChunk).mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(idleChunk).mockReturnValue(new Promise(() => {}))
    const wrapper = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    await vi.advanceTimersByTimeAsync(500 + 100)
    expect(mocks.resize).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('resynchronizes after native SSE reconnect even after resize retries ran out', async () => {
    mocks.subscribe.mockImplementation((_target: unknown, handlers: TerminalStreamHandlers) => {
      mocks.handlers = handlers
      return { close: vi.fn() }
    })
    const wrapper = mount(AppInteractiveTerminal, { props })
    mocks.handlers?.job?.(idleChunk)
    await vi.advanceTimersByTimeAsync(100)
    mocks.resize.mockRejectedValue(new Error('offline'))
    mocks.terminal!.cols = 100
    mocks.observer?.()
    await vi.advanceTimersByTimeAsync(100 + 500 + 1000 + 2000)
    expect(mocks.resize).toHaveBeenCalledTimes(5)
    mocks.resize.mockResolvedValue({ accepted: true })
    mocks.handlers?.connected?.()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(6)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-a', 24, 100)
    wrapper.unmount()
  })

  it('does not apply an old job acknowledgement to a replacement job', async () => {
    let finish!: (value: { accepted: boolean }) => void
    mocks.resize.mockReturnValueOnce(new Promise((resolve) => { finish = resolve }))
    const wrapper = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    await wrapper.setProps({ jobId: 'job-b' })
    finish({ accepted: true }); await flushPromises()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-b', 24, 80)
    wrapper.unmount()
  })

  it('leaves other job kinds untouched and does not retry after unmount', async () => {
    const wrapper = mount(AppInteractiveTerminal, { props: { ...props, kind: 'site' } })
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).not.toHaveBeenCalled()
    wrapper.unmount()
    mocks.resize.mockRejectedValue(new Error('down'))
    const app = mount(AppInteractiveTerminal, { props })
    await vi.advanceTimersByTimeAsync(100)
    app.unmount()
    await vi.advanceTimersByTimeAsync(5000)
    expect(mocks.resize).toHaveBeenCalledTimes(1)
  })
})
