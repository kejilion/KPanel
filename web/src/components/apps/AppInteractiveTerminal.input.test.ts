// @vitest-environment jsdom
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AppInteractiveTerminal from './AppInteractiveTerminal.vue'
import type { TerminalStreamHandlers } from '@/lib/terminalStream'

const mocks = vi.hoisted(() => ({
  transport: vi.fn(), socket: vi.fn(), batch: vi.fn(), subscribe: vi.fn(),
  legacy: { app: vi.fn(), site: vi.fn(), diagnostic: vi.fn(), environment: vi.fn() },
  handlers: undefined as TerminalStreamHandlers | undefined,
  onData: undefined as ((data: string) => void) | undefined,
  writes: [] as string[],
}))
vi.mock('@/lib/api', () => ({
  api: {
    apps: { terminalResize: vi.fn().mockResolvedValue({ accepted: true }), terminal: vi.fn(), terminalInput: mocks.legacy.app },
    sites: { terminalInput: mocks.legacy.site },
    diagnostics: { terminalInput: mocks.legacy.diagnostic },
    webEnvironment: { terminalInput: mocks.legacy.environment },
    jobTerminals: { inputTransport: mocks.transport, inputSocket: mocks.socket, inputBatch: mocks.batch },
  },
  terminalStream: { subscribe: mocks.subscribe },
}))
vi.mock('@xterm/xterm', () => ({ Terminal: class {
  options = {}; parser = { registerOscHandler() {} }; rows = 24; cols = 80
  buffer = { active: { viewportY: 0, baseY: 0 } }
  loadAddon() {} attachCustomKeyEventHandler() {} open() {} dispose() {} focus() {} scrollToBottom() {} reset() {}
  onData(handler: (data: string) => void) { mocks.onData = handler }
  write(data: Uint8Array, callback?: () => void) { mocks.writes.push(new TextDecoder().decode(data)); callback?.() }
} }))
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }))
vi.mock('@xterm/addon-web-links', () => ({ WebLinksAddon: class {} }))

type Sent = { type: string; csrf?: string; stream?: string; frame?: { stream: string; seq: number; data: string } }
class FakeSocket {
  static instances: FakeSocket[] = []
  readyState = 1
  bufferedAmount = 0
  closed = false
  sent: Sent[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  constructor(public url: string, public protocol: string) { FakeSocket.instances.push(this) }
  send(data: string) { this.sent.push(JSON.parse(data)) }
  close() { this.closed = true }
  open() { this.onopen?.() }
  receive(message: unknown) { this.onmessage?.({ data: JSON.stringify(message) }) }
  frames() { return this.sent.filter((message) => message.type === 'input').map((message) => message.frame!) }
}

const text = (frame: { data: string }) => new TextDecoder().decode(Uint8Array.from(atob(frame.data), (c) => c.charCodeAt(0)))

function mountTerminal(kind: 'app' | 'site' | 'diagnostic' | 'environment', jobId = 'job-1') {
  return mount(AppInteractiveTerminal, { props: { jobId, kind, inputOpen: true } })
}

async function readySocket(index = 0, epoch = 'epoch-a'): Promise<FakeSocket> {
  await flushPromises()
  const socket = FakeSocket.instances[index]!
  socket.open()
  socket.receive({ type: 'ready', window: 32, epoch })
  return socket
}

describe('task terminal input channel', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    FakeSocket.instances.length = 0
    mocks.writes.length = 0
    mocks.onData = undefined
    mocks.handlers = undefined
    mocks.transport.mockReset().mockResolvedValue({ protocol: 'terminal-input-v1' })
    mocks.socket.mockReset().mockImplementation((kind: string, id: string) => ({ url: `wss://panel.test/api/v1/job-terminals/${kind}/${id}/input-stream`, csrf: 'csrf-token' }))
    mocks.batch.mockReset()
    for (const mock of Object.values(mocks.legacy)) mock.mockReset().mockResolvedValue({ ok: true })
    mocks.subscribe.mockReset().mockImplementation((_target: unknown, handlers: TerminalStreamHandlers) => {
      mocks.handlers = handlers
      return { close() {} }
    })
    vi.stubGlobal('WebSocket', FakeSocket)
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(800)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(400)
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
    vi.stubGlobal('requestAnimationFrame', () => 0)
  })
  afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it.each(['app', 'site', 'diagnostic', 'environment'] as const)('streams %s task input as acknowledged frames before the first key', async (kind) => {
    const wrapper = mountTerminal(kind)
    await flushPromises()
    // The connection exists before anybody typed: the first key pays no handshake.
    expect(mocks.transport).toHaveBeenCalledWith(kind, 'job-1')
    expect(mocks.socket).toHaveBeenCalledWith(kind, 'job-1')
    const socket = await readySocket()
    expect(socket.url).toContain(`/job-terminals/${kind}/job-1/input-stream`)
    expect(socket.sent[0]).toMatchObject({ type: 'auth', csrf: 'csrf-token' })
    mocks.onData!('ls\r')
    mocks.onData!('中文')
    await vi.advanceTimersByTimeAsync(12)
    expect(socket.frames().map((frame) => [frame.seq, text(frame)])).toEqual([[1, 'ls\r'], [2, '中文']])
    expect(socket.frames().every((frame) => frame.stream === socket.frames()[0]!.stream)).toBe(true)
    for (const mock of Object.values(mocks.legacy)) expect(mock).not.toHaveBeenCalled()
    wrapper.unmount()
    expect(socket.closed).toBe(true)
  })

  it('coalesces plain typing into one frame and sends control keys at once', async () => {
    const wrapper = mountTerminal('app')
    const socket = await readySocket()
    for (const key of 'echo') mocks.onData!(key)
    expect(socket.frames()).toHaveLength(0)
    await vi.advanceTimersByTimeAsync(12)
    expect(socket.frames().map(text)).toEqual(['echo'])
    mocks.onData!('\x03')
    expect(socket.frames().map(text)).toEqual(['echo', '\x03'])
    wrapper.unmount()
  })

  it('keeps a burst in order without waiting for each acknowledgement', async () => {
    const wrapper = mountTerminal('app')
    const socket = await readySocket()
    const paste = 'x'.repeat(2048 * 5)
    mocks.onData!(paste)
    await vi.advanceTimersByTimeAsync(12)
    expect(socket.frames().map((frame) => frame.seq)).toEqual([1, 2, 3, 4, 5])
    expect(socket.frames().map(text).join('')).toBe(paste)
    wrapper.unmount()
  })

  it('never sends NUL, which the per-request route refused as well', async () => {
    const wrapper = mountTerminal('app')
    const socket = await readySocket()
    mocks.onData!('a\0b\r')
    mocks.onData!('\0')
    await vi.advanceTimersByTimeAsync(12)
    expect(socket.frames().map(text)).toEqual(['ab\r'])
    wrapper.unmount()
  })

  it('uses the per-request route with the typed text when the Agent lacks the protocol', async () => {
    mocks.transport.mockResolvedValue({ protocol: '' })
    const wrapper = mountTerminal('site')
    await flushPromises()
    mocks.onData!('y\r')
    await vi.advanceTimersByTimeAsync(12)
    // The routes take the text itself, not the base64 that host terminals send.
    expect(mocks.legacy.site).toHaveBeenCalledWith('job-1', 'y\r')
    expect(FakeSocket.instances).toHaveLength(0)
    wrapper.unmount()
  })

  it('stops input when the task closes it and starts a new stream when it reopens', async () => {
    const wrapper = mountTerminal('diagnostic')
    const socket = await readySocket()
    mocks.handlers!.job!({ dataBase64: '', nextOffset: 1, inputOpen: false, finished: true })
    await flushPromises()
    expect(socket.closed).toBe(true)
    mocks.onData!('late\r')
    await vi.advanceTimersByTimeAsync(12)
    expect(socket.frames()).toHaveLength(0)
    wrapper.unmount()
  })

  it('reports a refused stream once and recovers with a new stream on the next key', async () => {
    const wrapper = mountTerminal('app')
    const first = await readySocket()
    mocks.onData!('one\r')
    first.receive({ type: 'error', code: 'terminal_input_sequence', retryable: false })
    expect(mocks.writes.filter((write) => write.includes('[KPanel]'))).toHaveLength(1)
    expect(first.closed).toBe(true)
    // The next keystroke does not die with the failed stream.
    mocks.onData!('two\r')
    const second = await readySocket(1, 'epoch-a')
    await vi.advanceTimersByTimeAsync(12)
    expect(second.sent[0]!.stream).not.toBe(first.sent[0]!.stream)
    expect(second.frames().map((frame) => [frame.seq, text(frame)])).toEqual([[1, 'two\r']])
    wrapper.unmount()
  })

  it('offers an unconfirmed frame again over the HTTP fallback, on the same stream', async () => {
    mocks.batch.mockImplementation(async (_kind: string, _id: string, frames: Array<{ seq: number }>) => ({ acked: frames.at(-1)!.seq, epoch: 'epoch-a' }))
    const wrapper = mountTerminal('app')
    const first = await readySocket()
    mocks.onData!('a\r')
    expect(first.frames().map(text)).toEqual(['a\r'])
    // The page lost the socket before the ACK: the owner may already have the
    // bytes, so only the same stream and sequence may carry them again.
    first.onclose?.()
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    const sent = mocks.batch.mock.calls.map(([, , frames]) => frames as Array<{ stream: string; seq: number }>)
    expect(sent.map((frames) => frames.map((frame) => frame.seq))).toEqual([[0], [1]])
    expect(sent.every((frames) => frames.every((frame) => frame.stream === first.sent[0]!.stream))).toBe(true)
    wrapper.unmount()
  })

  it('reopens the socket on the same stream after an idle connection drops', async () => {
    const wrapper = mountTerminal('app')
    const first = await readySocket()
    mocks.onData!('a\r')
    first.receive({ type: 'ack', seq: 1 })
    first.onclose?.()
    await vi.advanceTimersByTimeAsync(300)
    const second = await readySocket(1)
    expect(second.sent[0]!.stream).toBe(first.sent[0]!.stream)
    mocks.onData!('b\r')
    expect(second.frames().map((frame) => [frame.seq, text(frame)])).toEqual([[2, 'b\r']])
    wrapper.unmount()
  })

  it('drops the stream of the previous task when the terminal shows another one', async () => {
    const wrapper = mountTerminal('app', 'job-1')
    const first = await readySocket()
    await wrapper.setProps({ jobId: 'job-2' })
    await flushPromises()
    expect(first.closed).toBe(true)
    expect(mocks.socket).toHaveBeenLastCalledWith('app', 'job-2')
    const second = await readySocket(1)
    mocks.onData!('z\r')
    expect(second.url).toContain('/job-2/')
    expect(second.frames().map(text)).toEqual(['z\r'])
    expect(first.frames()).toHaveLength(0)
    wrapper.unmount()
  })

  it('does not connect for a task that is not waiting for input', async () => {
    const wrapper = mount(AppInteractiveTerminal, { props: { jobId: 'job-1', kind: 'app', inputOpen: false } })
    await flushPromises()
    expect(mocks.transport).not.toHaveBeenCalled()
    expect(FakeSocket.instances).toHaveLength(0)
    mocks.onData!('ignored\r')
    expect(mocks.legacy.app).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('falls back to acknowledged HTTP batches when WebSocket cannot connect', async () => {
    mocks.batch.mockImplementation(async (_kind: string, _id: string, frames: Array<{ seq: number }>) => ({ acked: frames.at(-1)!.seq, epoch: 'epoch-a' }))
    const wrapper = mountTerminal('environment')
    await flushPromises()
    FakeSocket.instances[0]!.onerror?.()
    await vi.advanceTimersByTimeAsync(300)
    mocks.onData!('go\r')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()
    expect(mocks.batch.mock.calls.map(([kind, id, frames]) => [kind, id, frames.map((frame: { seq: number }) => frame.seq)])).toEqual([
      ['environment', 'job-1', [0]],
      ['environment', 'job-1', [1]],
    ])
    wrapper.unmount()
  })
})

