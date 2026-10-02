import { afterEach, describe, expect, it, vi } from 'vitest'
import { TerminalDuplexInput, terminalInputQueueLimit, type TerminalDuplexOptions } from './terminalDuplexInput'

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

function fixture(protocol = 'terminal-input-v1', batch?: TerminalDuplexOptions['batch']) {
  const sockets: Socket[] = []
  const error = vi.fn()
  const legacy = vi.fn().mockResolvedValue({ accepted: true })
  const input = new TerminalDuplexInput({
    negotiate: async () => ({ protocol }), credentials: () => ({ url: 'ws://panel.test/input', csrf: 'secret' }),
    legacy, batch, error, stream: '00000000000000000000000000000001',
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
  it('reuses the prewarmed socket while opening and awaiting the owner claim', async () => {
    const f = fixture()
    f.input.flush()
    await Promise.resolve(); await Promise.resolve()
    const socket = f.sockets[0]!
    const pieces = ['中文🙂', 'x'.repeat(2050), '\x03', '\r']
    for (const piece of pieces.slice(0, 2)) { f.input.append(piece); f.input.flush() }
    expect(f.sockets).toHaveLength(1)
    expect(socket.sent).toEqual([])
    socket.onopen?.call(socket as unknown as WebSocket, new Event('open'))
    for (const piece of pieces.slice(2)) { f.input.append(piece); f.input.flush() }
    expect(f.sockets).toHaveLength(1)
    expect(socket.sent.map(message => message.type)).toEqual(['auth'])
    socket.receive({ type: 'ready', window: 32 })
    const frames = socket.sent.filter(message => message.type === 'input')
    expect(frames.map(message => message.frame!.seq)).toEqual([1, 2])
    const bytes = frames.flatMap(message => Array.from(atob(message.frame!.data), char => char.charCodeAt(0)))
    expect(new TextDecoder().decode(new Uint8Array(bytes))).toBe(pieces.join(''))
    expect(f.error).not.toHaveBeenCalled()
    f.input.close()
  })
  it('prewarms without a keystroke and tolerates repeated healthy idle reconnects',async()=>{
    vi.useFakeTimers();const f=fixture();await ready(f)
    for(let i=0;i<10;i++){
      await vi.advanceTimersByTimeAsync(30000)
      const socket=f.sockets.at(-1)!;socket.onclose?.call(socket as unknown as WebSocket,{} as CloseEvent)
      await vi.advanceTimersByTimeAsync(250)
      f.sockets.at(-1)!.receive({type:'ready',window:32})
    }
    expect(f.error).not.toHaveBeenCalledWith('fatal');expect(f.sockets).toHaveLength(11)
    expect(f.sockets[0]!.sent[0]?.type).toBe('auth');f.input.close()
  })
  it('falls back behind an HTTP-only proxy using the same sequence batch and deduplicable replay',async()=>{
    vi.useFakeTimers()
    const bodies: Array<Array<{stream:string;seq:number;data:string}>>=[]
    let lost=false
    const batch: NonNullable<TerminalDuplexOptions['batch']>=async frames=>{
      bodies.push(structuredClone(frames))
      if(frames[0]!.seq>0&&!lost){lost=true;throw new Error('owner ACK response lost')}
      return {acked:frames.at(-1)!.seq}
    }
    const f=fixture('terminal-input-v1',batch);f.input.append('x'.repeat(34*2048));f.input.flush()
    await Promise.resolve();await Promise.resolve()
    f.sockets[0]!.onerror?.call(f.sockets[0] as unknown as WebSocket,new Event('error'))
    await vi.advanceTimersByTimeAsync(250);await vi.advanceTimersByTimeAsync(500)
    expect(bodies[0]![0]!.seq).toBe(0)
    expect(bodies[1]).toHaveLength(32);expect(bodies[2]).toEqual(bodies[1]);expect(bodies[3]).toHaveLength(2)
    expect(f.input.byteLength).toBe(0);expect(f.legacy).not.toHaveBeenCalled();f.input.close()
  })
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
