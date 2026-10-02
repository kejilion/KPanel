import { afterEach, describe, expect, it, vi } from 'vitest'
import { TerminalDuplexInput, terminalInputQueueLimit } from './terminalDuplexInput'

class Socket {
  readyState = 1 as const
  bufferedAmount = 0
  onopen: WebSocket['onopen'] = null
  onmessage: WebSocket['onmessage'] = null
  onclose: WebSocket['onclose'] = null
  onerror: WebSocket['onerror'] = null
  sent: Array<{ type: string; frame?: { seq: number; stream: string; data: string } }> = []
  send(data: string) { this.sent.push(JSON.parse(data)) }
  close() {}
  receive(data: unknown) { this.onmessage?.call(this as unknown as WebSocket, { data: JSON.stringify(data) } as MessageEvent) }
}

function fixture(protocol = 'terminal-input-v1') {
  const sockets: Socket[] = []
  const error = vi.fn()
  const legacy = vi.fn().mockResolvedValue({ accepted: true })
  const input = new TerminalDuplexInput({
    negotiate: async () => ({ protocol }), credentials: () => ({ url: 'ws://panel.test/input', csrf: 'secret' }),
    legacy, error, stream: '00000000000000000000000000000001',
    socket: () => { const socket = new Socket(); sockets.push(socket); return socket },
  })
  return { input, sockets, legacy, error }
}

async function ready(f: ReturnType<typeof fixture>) {
  f.input.flush()
  await Promise.resolve(); await Promise.resolve()
  const socket = f.sockets.at(-1)!
  socket.onopen?.call(socket as unknown as WebSocket, new Event('open'))
  socket.receive({ type: 'ready', window: 32 })
  return socket
}

afterEach(() => vi.useRealTimers())
describe('host terminal duplex input', () => {
  it('pipelines the full window without waiting for ACK, then advances only in order', async () => {
    const f = fixture()
    f.input.append('x'.repeat(33 * 2048))
    const socket = await ready(f)
    expect(socket.sent.filter(m => m.type === 'input')).toHaveLength(32)
    expect(f.input.outstanding).toBe(32)
    socket.receive({ type: 'ack', seq: 1 })
    expect(socket.sent.at(-1)?.frame?.seq).toBe(33)
    socket.receive({ type: 'ack', seq: 3 })
    expect(f.error).toHaveBeenLastCalledWith('fatal')
    f.input.close()
  })
  it('replays exactly the same stream, sequence and UTF8 bytes after lost ACK', async () => {
    vi.useFakeTimers()
    const f = fixture()
    const value = '中文🙂'.repeat(400) + '\x03\r'
    f.input.append(value)
    const first = await ready(f)
    const frames = first.sent.filter(m => m.type === 'input')
    first.onclose?.call(first as unknown as WebSocket, {} as CloseEvent)
    await vi.advanceTimersByTimeAsync(250)
    const second = f.sockets.at(-1)!
    second.receive({ type: 'ready', window: 32 })
    expect(second.sent).toEqual(frames)
    const bytes = frames.flatMap(m => Array.from(atob(m.frame!.data), c => c.charCodeAt(0)))
    expect(new TextDecoder().decode(new Uint8Array(bytes))).toBe(value)
    for (const message of frames) second.receive({ type: 'ack', seq: message.frame!.seq })
    expect(f.input.byteLength).toBe(0)
    f.input.close()
  })
  it('rejects an over-capacity append without silently accepting a prefix', () => {
    const f=fixture()
    expect(f.input.append('a'.repeat(terminalInputQueueLimit))).toBe(true)
    expect(f.input.append('b')).toBe(false)
    expect(f.input.byteLength).toBe(terminalInputQueueLimit)
    expect(f.error).toHaveBeenCalledWith('capacity')
    f.input.close()
  })
  it('uses explicit old-target negotiation and never automatically replays an ambiguous POST', async () => {
    const f=fixture('')
    f.legacy.mockRejectedValue(new Error('response lost'))
    f.input.append('command\r');f.input.flush()
    for(let i=0;i<6;i++)await Promise.resolve()
    f.input.flush();f.input.append('more')
    expect(f.legacy).toHaveBeenCalledTimes(1)
    expect(f.sockets).toHaveLength(0)
    expect(f.error).toHaveBeenCalledWith('fatal')
  })
  it('does not downgrade after a sequenced frame may have reached the owner', async () => {
    vi.useFakeTimers()
    const f=fixture();f.input.append('command\r')
    const socket=await ready(f)
    socket.receive({ type:'error', retryable:true })
    await vi.advanceTimersByTimeAsync(250)
    expect(f.sockets).toHaveLength(2)
    expect(f.legacy).not.toHaveBeenCalled()
    f.input.close()
  })
  it('does not extend an unconfirmed frame deadline with new keystrokes', async () => {
    vi.useFakeTimers()
    const f=fixture(); f.input.append('first')
    await ready(f)
    for(let i=0;i<11;i++){ await vi.advanceTimersByTimeAsync(1000); f.input.append('x');f.input.flush() }
    await vi.advanceTimersByTimeAsync(1000)
    expect(f.error).toHaveBeenCalledWith('retry')
    f.input.close()
  })
  it('bounds reconnects even when a stalled owner lets every socket become ready', async () => {
    vi.useFakeTimers()
    const f=fixture();f.input.append('first');await ready(f)
    for(let i=0;i<9;i++) {
      f.sockets.at(-1)!.receive({ type:'error', retryable:true })
      await vi.advanceTimersByTimeAsync(4000)
      if(i<8)f.sockets.at(-1)!.receive({ type:'ready',window:32 })
    }
    expect(f.error).toHaveBeenLastCalledWith('fatal')
    f.input.close()
  })
})
