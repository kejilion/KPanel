import { afterEach, describe, expect, it, vi } from 'vitest'
import { TerminalDuplexInput, type TerminalDuplexOptions } from './terminalDuplexInput'

type Sent = { type: string; stream?: string; frame?: { stream: string; seq: number; data: string } }
class Socket {
  readyState = 1 as const
  bufferedAmount = 0
  onopen: WebSocket['onopen'] = null
  onmessage: WebSocket['onmessage'] = null
  onclose: WebSocket['onclose'] = null
  onerror: WebSocket['onerror'] = null
  sent: Sent[] = []
  send(data: string) { this.sent.push(JSON.parse(data)) }
  close() {}
  receive(data: unknown) { this.onmessage?.call(this as unknown as WebSocket, { data: JSON.stringify(data) } as MessageEvent) }
  open() { this.onopen?.call(this as unknown as WebSocket, new Event('open')) }
  frames() { return this.sent.filter((message) => message.type === 'input').map((message) => message.frame!) }
  streams() { return this.sent.filter((message) => message.type === 'auth').map((message) => message.stream!) }
}

function fixture(overrides: Partial<TerminalDuplexOptions> = {}) {
  const sockets: Socket[] = []
  const error = vi.fn()
  const input = new TerminalDuplexInput({
    negotiate: async () => ({ protocol: 'terminal-input-v1' }),
    credentials: () => ({ url: 'ws://panel.test/input', csrf: 'secret' }),
    legacy: vi.fn().mockResolvedValue({}),
    error,
    stream: '00000000000000000000000000000001',
    socket: () => { const socket = new Socket(); sockets.push(socket); return socket },
    ...overrides,
  })
  return { input, sockets, error }
}

async function connect(f: ReturnType<typeof fixture>, index: number, epoch?: string) {
  f.input.flush()
  await vi.advanceTimersByTimeAsync(0)
  const socket = f.sockets[index]!
  socket.open()
  socket.receive({ type: 'ready', window: 32, ...(epoch === undefined ? {} : { epoch }) })
  return socket
}

afterEach(() => vi.useRealTimers())

describe('terminal input owner epoch', () => {
  it('keeps the stream across a reconnect to the same owner state', async () => {
    vi.useFakeTimers()
    const f = fixture()
    const first = await connect(f, 0, 'epoch-a')
    f.input.append('a')
    f.input.flush()
    first.receive({ type: 'ack', seq: 1 })
    first.receive({ type: 'error', retryable: true })
    await vi.advanceTimersByTimeAsync(300)
    const second = await connect(f, 1, 'epoch-a')
    expect(second.streams()).toEqual(first.streams())
    f.input.append('b')
    f.input.flush()
    expect(second.frames().map((frame) => frame.seq)).toEqual([2])
    expect(f.error).not.toHaveBeenCalledWith('fatal')
    f.input.close()
  })

  it('never replays frames the previous owner state may have applied', async () => {
    vi.useFakeTimers()
    const f = fixture()
    const first = await connect(f, 0, 'epoch-a')
    f.input.append('rm -rf build\r')
    f.input.flush()
    expect(first.frames()).toHaveLength(1)
    // The owner restarted before the ACK arrived: it has forgotten whether the
    // frame was applied, so the browser cannot tell either.
    first.receive({ type: 'error', retryable: true })
    await vi.advanceTimersByTimeAsync(300)
    const second = await connect(f, 1, 'epoch-b')
    expect(f.error).toHaveBeenLastCalledWith('fatal')
    expect(second.frames()).toHaveLength(0)
    f.input.close()
  })

  it('starts over as a new stream when nothing was outstanding', async () => {
    vi.useFakeTimers()
    const f = fixture()
    const first = await connect(f, 0, 'epoch-a')
    f.input.append('a')
    f.input.flush()
    first.receive({ type: 'ack', seq: 1 })
    first.receive({ type: 'error', retryable: true })
    await vi.advanceTimersByTimeAsync(300)
    const second = await connect(f, 1, 'epoch-b')
    expect(f.error).not.toHaveBeenCalledWith('fatal')
    // The lost state is dropped and the writer reconnects under a new identity.
    await vi.advanceTimersByTimeAsync(600)
    const third = await connect(f, 2, 'epoch-b')
    expect(third.streams()[0]).not.toBe(first.streams()[0])
    f.input.append('b')
    f.input.flush()
    expect(second.frames()).toHaveLength(0)
    expect(third.frames().map((frame) => [frame.seq, frame.stream])).toEqual([[1, third.streams()[0]]])
    f.input.close()
  })

  it('ignores owners that announce no epoch, as host terminals do', async () => {
    vi.useFakeTimers()
    const f = fixture()
    const first = await connect(f, 0)
    f.input.append('a')
    f.input.flush()
    first.receive({ type: 'error', retryable: true })
    await vi.advanceTimersByTimeAsync(300)
    const second = await connect(f, 1)
    expect(second.streams()).toEqual(first.streams())
    expect(second.frames().map((frame) => frame.seq)).toEqual([1])
    f.input.close()
  })

  it('checks the epoch of the HTTP claim as well', async () => {
    vi.useFakeTimers()
    // The owner behind the HTTP claim is not the one that took frame 1 over the socket.
    const batch = vi.fn(async (frames: Array<{ stream: string; seq: number }>) => ({ acked: frames.at(-1)!.seq, epoch: 'epoch-b' }))
    const f = fixture({ batch })
    const first = await connect(f, 0, 'epoch-a')
    // WebSocket refused after the first frame: continue over HTTP batches.
    f.input.append('a')
    f.input.flush()
    first.onerror?.call(first as unknown as WebSocket, new Event('error'))
    await vi.advanceTimersByTimeAsync(300)
    await vi.advanceTimersByTimeAsync(0)
    expect(batch).toHaveBeenCalledTimes(1)
    expect(batch.mock.calls[0]![0]).toEqual([{ stream: first.streams()[0], seq: 0, data: '' }])
    expect(f.error).toHaveBeenLastCalledWith('fatal')
    f.input.close()
  })
})

describe('terminal input per-request fallback', () => {
  it('can hand over the typed text instead of its base64 form', async () => {
    const legacy = vi.fn().mockResolvedValue({})
    const f = fixture({ negotiate: async () => ({ protocol: '' }), legacy, legacyText: true })
    f.input.append('中文\r')
    f.input.flush()
    await Promise.resolve()
    await Promise.resolve()
    expect(legacy).toHaveBeenCalledWith('中文\r')
    f.input.close()
  })

  it('keeps base64 for callers that expect it', async () => {
    const legacy = vi.fn().mockResolvedValue({})
    const f = fixture({ negotiate: async () => ({ protocol: '' }), legacy })
    f.input.append('ls\r')
    f.input.flush()
    await Promise.resolve()
    await Promise.resolve()
    expect(legacy).toHaveBeenCalledWith(btoa('ls\r'))
    f.input.close()
  })
})
