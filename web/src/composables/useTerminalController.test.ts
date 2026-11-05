// @vitest-environment jsdom
import { defineComponent, ref } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useTerminalController } from './useTerminalController'
import type { TerminalAdapter, TerminalChunk, TerminalOutputHandlers } from '@/lib/terminalAdapters'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'

const mocks = vi.hoisted(() => ({ writes: [] as string[], deferWrites: false, pending: [] as Array<() => void>, osc: [] as number[] }))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; rows = 24; cols = 80; buffer = { active: { viewportY: 0, baseY: 0 } }
  parser = { registerOscHandler(code: number) { mocks.osc.push(code) }, registerCsiHandler() { return { dispose() {} } } }
  loadAddon() {} attachCustomKeyEventHandler() {} onData() {} open() {} dispose() {} focus() {} scrollToBottom() {} reset() {}
  resize(columns: number, rows: number) { this.cols = columns; this.rows = rows }
  write(data: string | Uint8Array, callback?: () => void) {
    mocks.writes.push(typeof data === 'string' ? data : new TextDecoder().decode(data))
    if (callback) { if (mocks.deferWrites) mocks.pending.push(callback); else callback() }
  }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))
const idle = (): Promise<TerminalChunk> => new Promise(() => {})
const chunk = (data = '', nextOffset = data.length): TerminalChunk => ({ data: btoa(data), nextOffset, inputOpen: true, finished: false })
const wrappers: ReturnType<typeof mount>[] = []

function fixture(mode: 'session' | 'task', stream = false) {
  const id = ref('first'), visible = ref(true)
  const read = vi.fn<(id: string, offset: number, signal: AbortSignal) => Promise<TerminalChunk>>().mockImplementation(idle)
  const resize = vi.fn<(id: string, rows: number, columns: number) => Promise<void>>().mockResolvedValue(undefined)
  const legacy = vi.fn().mockResolvedValue({ accepted: true })
  const subscriptions: Array<{ id: string; offset: number; handlers: TerminalOutputHandlers; close: ReturnType<typeof vi.fn> }> = []
  let core!: ReturnType<typeof useTerminalController>
  const component = defineComponent({
    setup() {
      const host = ref<HTMLElement>(), clipboardMenu = ref()
      core = useTerminalController({ host, clipboardMenu, inputOpen: () => true, adapter: (): TerminalAdapter => {
        const target = id.value
        return {
          key: `${mode}:${target}`, mode,
          input: { negotiate: async () => ({ protocol: '' }), credentials: () => ({ url: '', csrf: '' }), legacy },
          read: (offset, _inputOpen, signal) => read(target, offset, signal),
          subscribe: (offset, _inputOpen, handlers) => {
            if (!stream) return null
            const close = vi.fn()
            subscriptions.push({ id: target, offset, handlers, close })
            return { close }
          },
          resize: (rows, columns) => resize(target, rows, columns),
          missing: code => code === 'terminal_not_found',
        }
      } })
      return { host }
    }, template: '<div ref="host" />',
  })
  const wrapper = mount(component, { global: { provide: { [desktopWindowVisibleKey as symbol]: visible } } })
  wrappers.push(wrapper)
  return { core, wrapper, id, visible, read, resize, legacy, subscriptions }
}

beforeEach(() => {
  vi.useFakeTimers()
  mocks.writes.length = mocks.pending.length = mocks.osc.length = 0
  mocks.deferWrites = false
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  vi.stubGlobal('requestAnimationFrame', () => 0)
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(800)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(400)
})
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe.each(['session', 'task'] as const)('shared %s terminal controller', mode => {
  it('stops output and resize retries while a close remains unconfirmed', async () => {
    const f = fixture(mode, true)
    const subscription = f.subscriptions[0]!
    f.core.stopForClose()
    expect(subscription.close).toHaveBeenCalledOnce()
    subscription.handlers.chunk(chunk('late'))
    f.visible.value = false; await flushPromises()
    f.visible.value = true; await flushPromises()
    await vi.advanceTimersByTimeAsync(20000)
    expect(f.subscriptions).toHaveLength(1)
    expect(f.resize).not.toHaveBeenCalled()
    expect(mocks.writes.join('')).not.toContain('late')
    expect(f.core.state.value).toBe('error')
    expect(f.core.canInput.value).toBe(false)
  })
  it.each([0, 1, 2, 3])('follows output only within two lines of the bottom (gap %s)', gap => {
    const f = fixture(mode, true)
    const terminal = f.core.terminal.value!
    Object.assign(terminal.buffer.active, { baseY: 100, viewportY: 100 - gap })
    const bottom = vi.spyOn(terminal, 'scrollToBottom')
    f.subscriptions[0]!.handlers.chunk(chunk('new output\r\n'))
    expect(bottom).toHaveBeenCalledTimes(gap <= 2 ? 1 : 0)
  })

  it('allows explicit touch focus while suppressing automatic focus and blocked input', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: true }))
    const f = fixture(mode)
    const focus = vi.spyOn(f.core.terminal.value!, 'focus')
    vi.spyOn(f.wrapper.element, 'getBoundingClientRect').mockReturnValue({
      left: 0, top: 0, right: 800, bottom: 400, width: 800, height: 400,
    } as DOMRect)
    const touch = (type: string) => ({ type, touches: { length: 1, item: () => ({ clientX: 20, clientY: 20 }) }, preventDefault() {} }) as unknown as TouchEvent
    f.core.focusTerminal()
    expect(focus).not.toHaveBeenCalled()
    f.core.terminalTouchScroll.start(touch('touchstart'))
    f.core.terminalTouchScroll.end(touch('touchend'))
    expect(focus).toHaveBeenCalledOnce()
    f.core.blockInput()
    f.core.terminalTouchScroll.start(touch('touchstart'))
    f.core.terminalTouchScroll.end(touch('touchend'))
    expect(focus).toHaveBeenCalledOnce()
  })

  it('bounds a hung output read and retries after backoff', async () => {
    const f = fixture(mode)
    await vi.advanceTimersByTimeAsync(15000)
    expect(f.read.mock.calls[0]![2].aborted).toBe(true)
    expect(f.core.state.value).toBe('reconnecting')
    await vi.advanceTimersByTimeAsync(499)
    expect(f.read).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(f.read).toHaveBeenCalledTimes(2)
  })
  it('discards old poll data after switching target and aborts the old read', async () => {
    const f = fixture(mode)
    let finish!: (value: TerminalChunk) => void
    f.read.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    // Start another read after pausing the original mount read.
    f.visible.value = false; await flushPromises()
    f.visible.value = true; await flushPromises()
    const previousSignal = f.read.mock.calls.at(-1)![2]
    f.id.value = 'second'; await flushPromises()
    expect(previousSignal.aborted).toBe(true)
    finish(chunk('stale', 999)); await flushPromises()
    expect(mocks.writes.join('')).not.toContain('stale')
    expect(f.read).toHaveBeenLastCalledWith('second', 0, expect.any(AbortSignal))
  })
  it('fences late stream callbacks while hidden and resumes from the committed offset', async () => {
    const f = fixture(mode, true)
    f.subscriptions[0]!.handlers.chunk(chunk('hi', 2))
    f.visible.value = false; await flushPromises()
    f.subscriptions[0]!.handlers.chunk(chunk('stale', 999))
    f.visible.value = true; await flushPromises()
    expect(f.subscriptions.at(-1)).toMatchObject({ id: 'first', offset: 2 })
    expect(mocks.writes.join('')).not.toContain('stale')
  })
  it('uses shared backoff and treats 429 as back-pressure without changing connected state', async () => {
    const f = fixture(mode)
    f.read.mockResolvedValueOnce(chunk()).mockRejectedValueOnce({ status: 429 }).mockImplementation(idle)
    f.visible.value = false; await flushPromises(); f.visible.value = true; await flushPromises()
    await vi.advanceTimersByTimeAsync(0)
    expect(f.core.state.value).toBe('connected')
    const count = f.read.mock.calls.length
    await vi.advanceTimersByTimeAsync(299); expect(f.read).toHaveBeenCalledTimes(count)
    await vi.advanceTimersByTimeAsync(1); expect(f.read).toHaveBeenCalledTimes(count + 1)
  })
  it.each([401, 403])('stops retrying and blocks input on HTTP %s without marking the process finished', async status => {
    const f = fixture(mode)
    f.read.mockRejectedValueOnce({ status })
    f.visible.value = false; await flushPromises(); f.visible.value = true; await flushPromises()
    const count = f.read.mock.calls.length
    await vi.advanceTimersByTimeAsync(30000)
    expect(f.read).toHaveBeenCalledTimes(count)
    expect(f.core.state.value).toBe('error')
    expect(f.core.queueInput('ignored\r')).toBe(false)
    expect(f.legacy).not.toHaveBeenCalled()
  })
  it('does not apply an old resize acknowledgment to the new target', async () => {
    const f = fixture(mode)
    let finish!: () => void
    f.resize.mockReturnValueOnce(new Promise<void>(resolve => { finish = resolve }))
    await vi.advanceTimersByTimeAsync(100)
    f.id.value = 'second'; await flushPromises()
    finish(); await flushPromises(); await vi.advanceTimersByTimeAsync(100)
    expect(f.resize).toHaveBeenLastCalledWith('second', 24, 80)
  })
  it('installs the same clipboard and hyperlink escape protections', () => {
    fixture(mode)
    expect(mocks.osc).toEqual([8, 52])
  })
})

it('settles a presented command when unmounted before xterm finishes painting, without sending it', async () => {
  const f = fixture('session', true)
  f.subscriptions[0]!.handlers.chunk(chunk())
  mocks.deferWrites = true
  const pending = f.core.executePresentedCommand('private startup', 'bash ./demo.sh')
  await flushPromises()
  f.wrapper.unmount()
  expect(await pending).toBe(false)
  expect(f.legacy).not.toHaveBeenCalled()
  for (const done of mocks.pending.splice(0)) done()
  expect(f.legacy).not.toHaveBeenCalled()
})
