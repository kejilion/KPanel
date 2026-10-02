// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import type { Ref } from 'vue'
import AppInteractiveTerminal from './AppInteractiveTerminal.vue'
import { desktopWindowActiveKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import type { TerminalStreamHandlers, TerminalStreamTarget } from '@/lib/terminalStream'

interface FakeTerminal {
  rows: number
  cols: number
  focus: ReturnType<typeof vi.fn>
  resize: ReturnType<typeof vi.fn>
  pending: Array<() => void>
}

const mocks = vi.hoisted(() => ({
  resize: vi.fn(), output: vi.fn(), subscribe: vi.fn(), close: vi.fn(),
  handlers: [] as TerminalStreamHandlers[], targets: [] as TerminalStreamTarget[],
  observers: [] as Array<() => void>, terminals: [] as FakeTerminal[], deferWrites: false,
}))
vi.mock('@/lib/api', () => ({ api: { apps: { terminalResize: mocks.resize, terminal: mocks.output },
    // An Agent without the acknowledged input protocol: input stays on the per-request route.
    jobTerminals: { inputTransport: () => Promise.resolve({ protocol: '' }) } },
  terminalStream: { subscribe: mocks.subscribe } }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80
  buffer = { active: { viewportY: 0, baseY: 0 } }
  focus = vi.fn()
  resize = vi.fn((cols: number, rows: number) => { this.cols = cols; this.rows = rows })
  pending: Array<() => void> = []
  constructor() { mocks.terminals.push(this as unknown as FakeTerminal) }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} scrollToBottom() {} reset() {}
  write(_data: Uint8Array, callback?: () => void) {
    if (mocks.deferWrites && callback) this.pending.push(callback)
    else callback?.()
  }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))

function mountTerminal(jobId: string, active: Ref<boolean>, visible: Ref<boolean>) {
  return mount(AppInteractiveTerminal, {
    props: { jobId, inputOpen: true },
    global: { provide: { [desktopWindowActiveKey as symbol]: active, [desktopWindowVisibleKey as symbol]: visible } },
  })
}

function chunk(bytes: number, nextOffset: number) {
  return { dataBase64: Buffer.alloc(bytes, 97).toString('base64'), nextOffset, inputOpen: true, finished: false }
}

describe('application terminals shown side by side', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.resize.mockReset().mockResolvedValue({ accepted: true })
    mocks.output.mockReset().mockReturnValue(new Promise(() => {}))
    mocks.close.mockReset()
    mocks.handlers.length = 0
    mocks.targets.length = 0
    mocks.observers.length = 0
    mocks.terminals.length = 0
    mocks.deferWrites = false
    mocks.subscribe.mockReset().mockImplementation((target: TerminalStreamTarget, handlers: TerminalStreamHandlers) => {
      mocks.targets.push({ ...target })
      mocks.handlers.push(handlers)
      return { close: mocks.close }
    })
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(800)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(400)
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { mocks.observers.push(callback) }
      observe() {} disconnect() {}
    })
    vi.stubGlobal('requestAnimationFrame', (callback: () => void) => { callback(); return 0 })
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it('keeps streaming in an unfocused visible window and resumes from its offset after minimizing', async () => {
    const active = ref(false)
    const visible = ref(true)
    const wrapper = mountTerminal('job-live', active, visible)
    expect(mocks.subscribe).toHaveBeenCalledTimes(1)
    mocks.handlers[0]!.job?.(chunk(10, 10))

    visible.value = false
    await flushPromises()
    expect(mocks.close).toHaveBeenCalledTimes(1)

    visible.value = true
    await flushPromises()
    expect(mocks.subscribe).toHaveBeenCalledTimes(2)
    expect(mocks.targets[1]).toMatchObject({ id: 'job-live', offset: 10 })
    wrapper.unmount()
  })

  it('does not take keyboard focus from the focused window', async () => {
    const wrapper = mountTerminal('job-focus', ref(false), ref(true))
    await flushPromises()
    expect(mocks.terminals[0]!.focus).not.toHaveBeenCalled()
    wrapper.unmount()

    mountTerminal('job-focus-2', ref(true), ref(true)).unmount()
    expect(mocks.terminals[1]!.focus).toHaveBeenCalled()
  })

  it('pauses while xterm is backlogged and resumes at the same offset once drained', async () => {
    mocks.deferWrites = true
    const wrapper = mountTerminal('job-flood', ref(true), ref(true))
    const terminal = mocks.terminals[0]!
    mocks.handlers[0]!.job?.(chunk(300 << 10, 300 << 10))
    expect(mocks.close).not.toHaveBeenCalled()
    mocks.handlers[0]!.job?.(chunk(300 << 10, 600 << 10))
    expect(mocks.close).toHaveBeenCalledTimes(1)
    expect(mocks.subscribe).toHaveBeenCalledTimes(1)

    for (const parsed of terminal.pending.splice(0)) parsed()
    expect(mocks.subscribe).toHaveBeenCalledTimes(2)
    expect(mocks.targets[1]).toMatchObject({ id: 'job-flood', offset: 600 << 10 })
    wrapper.unmount()
  })

  it('lets only the focused view of a shared PTY resize it; the other follows', async () => {
    const detailActive = ref(true)
    const windowActive = ref(false)
    const detail = mountTerminal('job-shared', detailActive, ref(true))
    const script = mountTerminal('job-shared', windowActive, ref(true))
    const [detailTerminal, scriptTerminal] = mocks.terminals
    detailTerminal!.rows = 40
    detailTerminal!.cols = 140
    mocks.observers[0]!()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenCalledTimes(1)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-shared', 40, 140)
    await flushPromises()
    expect(scriptTerminal!.resize).toHaveBeenLastCalledWith(140, 40)

    // Neither a resize nor a stream reconnect of the unfocused view reaches the PTY.
    mocks.observers[1]!()
    mocks.handlers[1]!.connected?.()
    await vi.advanceTimersByTimeAsync(200)
    expect(mocks.resize).toHaveBeenCalledTimes(1)

    // Focusing the script window moves ownership and sends its own size.
    detailActive.value = false
    windowActive.value = true
    await flushPromises()
    scriptTerminal!.rows = 30
    scriptTerminal!.cols = 100
    mocks.observers[1]!()
    await vi.advanceTimersByTimeAsync(100)
    expect(mocks.resize).toHaveBeenLastCalledWith('job-shared', 30, 100)
    await flushPromises()
    expect(detailTerminal!.resize).toHaveBeenLastCalledWith(100, 30)

    script.unmount()
    detail.unmount()
  })
})
