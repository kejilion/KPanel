import { TerminalInputQueue } from './terminalInput'
import { terminalRequest } from './terminalRequest'

export const terminalInputProtocol = 'terminal-input-v1'
export const terminalInputWindow = 32
export const terminalInputQueueLimit = 1 << 20
const encoder = new TextEncoder()
type Frame = { stream: string; seq: number; data: string; bytes: number; sent: boolean }
type Socket = Pick<WebSocket, 'send' | 'close' | 'readyState' | 'bufferedAmount' | 'onopen' | 'onmessage' | 'onclose' | 'onerror'>
export type TerminalDuplexOptions = {
  negotiate: (signal: AbortSignal) => Promise<{ protocol: string }>
  credentials: () => { url: string; csrf: string }
  legacy: (data: string) => Promise<unknown>
  /** Hand the per-request fallback the typed text instead of its base64 form. */
  legacyText?: boolean
  batch?: (frames: Array<{ stream: string; seq: number; data: string }>, signal: AbortSignal) => Promise<{ acked: number; epoch?: string }>
  error: (kind: 'retry' | 'fatal' | 'capacity') => void
  recovered?: () => void
  socket?: (url: string) => Socket
  stream?: string
}

function base64(value: string): string {
  let binary = ''
  for (const byte of encoder.encode(value)) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function newStream(): string {
  return Array.from(crypto.getRandomValues(new Uint8Array(16)), n => n.toString(16).padStart(2, '0')).join('')
}

// Input of host terminals and of task terminals (application, website,
// diagnostic and environment). A frame is removed exclusively after an ordered
// owner ACK. The owner may announce an epoch; a different one means the state
// holding this stream was lost, which is not the same as a lost connection.
export class TerminalDuplexInput {
  private queue = new TerminalInputQueue()
  private pending: Frame[] = []
  private sequence = 0
  private stream: string
  private epoch?: string
  private mode: 'unknown' | 'legacy' | 'duplex' | 'post' = 'unknown'
  private socket?: Socket
  private ready = false
  private stopped = false
  private connecting = false
  private legacySending = false
  private postSending = false
  private postClaimed = false
  private postController?: AbortController
  private negotiationController?: AbortController
  private everReady = false
  private retries = 0
  private failureSince = 0
  private retryTimer?: ReturnType<typeof setTimeout>
  private deadline?: ReturnType<typeof setTimeout>
  constructor(private options: TerminalDuplexOptions) {
    this.stream = options.stream ?? newStream()
  }
  get byteLength(): number { return this.queue.byteLength + this.pending.reduce((n, frame) => n + frame.bytes, 0) }
  get outstanding(): number { return this.pending.length }
  append(value: string): boolean {
    if (this.stopped) return false
    if (this.byteLength + encoder.encode(value).byteLength > terminalInputQueueLimit) { this.options.error('capacity'); return false }
    this.queue.append(value)
    return true
  }
  flush(): void {
    if (this.stopped) return
    if (this.mode === 'legacy') { void this.flushLegacy(); return }
    if (this.mode === 'post') { void this.flushBatch(); return }
    if (this.ready) { this.pump(); return }
    // Socket construction finishes before open/auth/owner claim. Keep newly
    // typed bytes queued on that socket until ready or disconnect clears it.
    if (!this.socket && !this.connecting && !this.retryTimer) void this.connect()
  }
  private async connect(): Promise<void> {
    this.connecting = true
    try {
      if (this.mode === 'unknown') {
        const controller = new AbortController()
        this.negotiationController = controller
        const result = await terminalRequest(controller, signal => this.options.negotiate(signal))
        if (this.stopped) return
        if (result.protocol === '') { this.mode = 'legacy'; void this.flushLegacy(); return }
        if (result.protocol !== terminalInputProtocol) { this.fail(); return }
        this.mode = 'duplex'
      }
      const { url, csrf } = this.options.credentials()
      const socket = this.options.socket?.(url) ?? new WebSocket(url, 'kpanel-terminal-input-v1')
      this.socket = socket
      this.armDeadline()
      socket.onopen = () => { if (this.socket === socket && !this.stopped) socket.send(JSON.stringify({ type: 'auth', csrf, stream: this.stream })) }
      socket.onmessage = (event) => {
        if (this.socket !== socket || this.stopped) return
        try {
          const message = JSON.parse(String(event.data)) as { type: string; seq?: number; retryable?: boolean; window?: number; epoch?: string }
          if (message.type === 'ready' && !this.ready && message.window === terminalInputWindow) {
            if (!this.acceptEpoch(message.epoch)) { this.ownerLost(); return }
            this.ready = true
            this.everReady = true
            // An idle, successfully reattached writer is healthy. Unconfirmed
            // bytes still retain their retry budget until an owner ACK arrives.
            if (!this.pending.length) { this.retries = 0; this.failureSince = 0 }
            for (const frame of this.pending) frame.sent = false
            this.options.recovered?.()
            this.pump()
            return
          }
          if (message.type === 'ack' && this.ready && this.pending[0]?.seq === message.seq) {
            this.pending.shift()
            this.retries = 0
            this.failureSince = 0
            if (this.deadline) clearTimeout(this.deadline)
            this.deadline = undefined
            this.armDeadline()
            this.pump()
            return
          }
          if (message.type === 'error' && message.retryable) { this.disconnect(); return }
          this.fail()
        } catch { this.fail() }
      }
      socket.onclose = () => { if (this.socket === socket) this.disconnect() }
      socket.onerror = () => { if (this.socket === socket) this.disconnect() }
    } catch { this.disconnect() }
    finally { this.negotiationController = undefined; this.connecting = false }
  }
  private pump(): void {
    const socket = this.socket
    if (!socket || !this.ready || this.stopped) return
    this.fillWindow()
    try {
      for (const frame of this.pending) {
        if (frame.sent) continue
        if (socket.bufferedAmount > 128 << 10) { this.disconnect(); return }
        socket.send(JSON.stringify({ type: 'input', frame: { stream: frame.stream, seq: frame.seq, data: frame.data } }))
        frame.sent = true
      }
      this.armDeadline()
    } catch { this.disconnect() }
  }
  private armDeadline(): void {
    if (this.ready && !this.pending.length) {
      if (this.deadline) clearTimeout(this.deadline)
      this.deadline = undefined
    } else if (!this.deadline) {
      // Sending another key must not postpone the oldest unconfirmed frame.
      this.deadline = setTimeout(() => { this.deadline = undefined; this.disconnect() }, 12000)
    }
  }
  private disconnect(): void {
    if (this.stopped || this.retryTimer) return
    const socket = this.socket
    this.socket = undefined
    this.ready = false
    socket?.close()
    if (this.mode === 'duplex' && this.options.batch && (this.pending.length > 0 || !this.everReady)) this.mode = 'post'
    if (this.deadline) clearTimeout(this.deadline)
    this.deadline = undefined
    this.failureSince ||= Date.now()
    if (++this.retries > 8 || Date.now() - this.failureSince >= 120000) { this.fail(); return }
    // A dropped idle connection is not failed input: reconnect quietly and only
    // tell the user when typed input is waiting for the connection.
    if (this.byteLength > 0) this.options.error('retry')
    this.retryTimer = setTimeout(() => { this.retryTimer = undefined; this.flush() }, Math.min(4000, 250 * 2 ** (this.retries - 1)))
  }
  private acceptEpoch(epoch: string | undefined): boolean {
    if (epoch === undefined) return true
    if (this.epoch !== undefined && this.epoch !== epoch) return false
    this.epoch = epoch
    return true
  }
  // The owner restarted and no longer knows this stream. Frames it may already
  // have applied must never be replayed; with none outstanding the writer just
  // starts over as a new stream.
  private ownerLost(): void {
    if (this.pending.length) { this.fail(); return }
    this.stream = newStream()
    this.sequence = 0
    this.epoch = undefined
    this.postClaimed = false
    this.disconnect()
  }
  private fillWindow(): void {
    while (this.pending.length < terminalInputWindow && !this.queue.empty) {
      const chunk = this.queue.take(2048)
      this.pending.push({ stream: this.stream, seq: ++this.sequence, data: base64(chunk), bytes: encoder.encode(chunk).byteLength, sent: false })
    }
  }
  private async flushBatch(): Promise<void> {
    if (this.postSending || this.stopped || this.retryTimer || !this.options.batch) return
    this.postSending = true
    try {
      if (!this.postClaimed) {
        const reply = await this.sendBatch([{ stream: this.stream, seq: 0, data: '' }])
        if (this.stopped) return
        if (reply.acked !== 0) { this.fail(); return }
        if (!this.acceptEpoch(reply.epoch)) { this.ownerLost(); return }
        this.postClaimed = true
      }
      this.fillWindow()
      while (this.pending.length && !this.stopped) {
        const last = this.pending.at(-1)!.seq
        const reply = await this.sendBatch(this.pending.map(({ stream, seq, data }) => ({ stream, seq, data })))
        if (this.stopped) return
        if (reply.acked !== last) { this.fail(); return }
        this.pending = []
        this.retries = 0; this.failureSince = 0
        this.fillWindow()
      }
      this.options.recovered?.()
    } catch (error) {
      const status = (error as { status?: number })?.status
      if (status && status >= 400 && status < 500 && status !== 408 && status !== 429) this.fail()
      else this.disconnect()
    } finally { this.postSending = false }
  }
  private async sendBatch(frames: Array<{ stream: string; seq: number; data: string }>): Promise<{ acked: number; epoch?: string }> {
    const controller = new AbortController()
    this.postController = controller
    const timer = setTimeout(() => controller.abort(), 23000)
    try { return await this.options.batch!(frames, controller.signal) }
    finally { clearTimeout(timer); if (this.postController === controller) this.postController = undefined }
  }
  private async flushLegacy(): Promise<void> {
    if (this.legacySending || this.stopped) return
    this.legacySending = true
    try {
      while (!this.queue.empty && !this.stopped) {
        const chunk = this.queue.take()
        // A lost POST response is ambiguous. Never restore and automatically
        // replay these bytes on an old target that cannot deduplicate them.
        await this.options.legacy(this.options.legacyText ? chunk : base64(chunk))
      }
    } catch { this.fail() }
    finally { this.legacySending = false }
  }
  private fail(): void { this.close(); this.options.error('fatal') }
  close(): void {
    this.stopped = true
    this.ready = false
    this.negotiationController?.abort()
    this.postController?.abort()
    if (this.retryTimer) clearTimeout(this.retryTimer)
    if (this.deadline) clearTimeout(this.deadline)
    this.socket?.close()
    this.socket = undefined
    this.pending = []
    this.queue.clear()
  }
}
