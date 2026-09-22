// One Server-Sent Events connection per tab carries output for every open
// terminal (host, batch and task terminals). Components subscribe with their
// current offset; when the stream is unavailable (no EventSource, a proxy that
// buffers or blocks it, or repeated failures) subscribe() returns null or the
// subscription reports `unavailable`, and callers keep their polling path.
import type { AppTerminalChunk, TerminalOutput } from '@/types/api'

export type TerminalStreamTarget =
  | { kind: 'terminal'; id: string; offset: number }
  | { kind: 'job'; job: 'app' | 'site' | 'diagnostic' | 'environment'; id: string; offset: number; inputOpen: boolean }

export interface TerminalStreamHandlers {
  output?: (output: TerminalOutput) => void
  job?: (chunk: AppTerminalChunk) => void
  error?: (code: string) => void
  unavailable?: () => void
}

export interface TerminalStreamSubscription {
  close(): void
}

interface StreamEvent {
  key: string
  output?: TerminalOutput
  job?: AppTerminalChunk
  error?: string
}

export interface TerminalStreamTransport {
  url(): string
  subscribe(body: { streamId: string; add: unknown[]; remove: string[] }): Promise<unknown>
  createSource?: (url: string) => EventSource
}

interface Entry {
  target: TerminalStreamTarget
  handlers: TerminalStreamHandlers
}

const MAX_FAILURES_BEFORE_READY = 3
const READY_TIMEOUT_MS = 5000
const SUBSCRIBE_DEBOUNCE_MS = 10

export function terminalStreamKey(target: TerminalStreamTarget): string {
  return target.kind === 'job' ? `job:${target.job}:${target.id}` : `terminal:${target.id}`
}

export class TerminalStreamClient {
  private source?: EventSource
  private streamId = ''
  private failures = 0
  private disabled = false
  private readyTimer?: ReturnType<typeof setTimeout>
  private flushTimer?: ReturnType<typeof setTimeout>
  private readonly entries = new Map<string, Entry>()
  private pendingAdd = new Set<string>()
  private pendingRemove = new Set<string>()

  constructor(private readonly transport: TerminalStreamTransport) {}

  get available(): boolean {
    return !this.disabled
  }

  subscribe(target: TerminalStreamTarget, handlers: TerminalStreamHandlers): TerminalStreamSubscription | null {
    if (this.disabled || !this.canStream()) return null
    this.ensureSource()
    if (this.disabled) return null
    const key = terminalStreamKey(target)
    this.entries.set(key, { target: { ...target }, handlers })
    this.pendingRemove.delete(key)
    this.pendingAdd.add(key)
    this.scheduleFlush()
    let closed = false
    return {
      close: () => {
        if (closed) return
        closed = true
        const current = this.entries.get(key)
        if (current?.handlers !== handlers) return
        this.entries.delete(key)
        this.pendingAdd.delete(key)
        if (this.streamId) this.pendingRemove.add(key)
        this.scheduleFlush()
        if (!this.entries.size) this.closeSource()
      },
    }
  }

  private canStream(): boolean {
    return Boolean(this.transport.createSource) || typeof EventSource !== 'undefined'
  }

  private ensureSource(): void {
    if (this.source || this.disabled) return
    const create = this.transport.createSource || ((url: string) => new EventSource(url, { withCredentials: true }))
    let source: EventSource
    try {
      source = create(this.transport.url())
    } catch {
      this.disable()
      return
    }
    this.source = source
    this.armReadyTimer()
    source.addEventListener('ready', (event) => {
      try {
        const payload = JSON.parse((event as MessageEvent<string>).data) as { streamId?: string }
        if (!payload.streamId) throw new Error('missing stream id')
        this.streamId = payload.streamId
      } catch {
        this.disable()
        return
      }
      this.failures = 0
      this.clearReadyTimer()
      // A reconnect gets a new stream: resubscribe everything at its latest offset.
      this.pendingAdd = new Set(this.entries.keys())
      this.pendingRemove.clear()
      this.scheduleFlush()
    })
    source.addEventListener('output', (event) => {
      let payload: StreamEvent
      try {
        payload = JSON.parse((event as MessageEvent<string>).data) as StreamEvent
      } catch {
        return
      }
      const entry = this.entries.get(payload.key)
      if (!entry) return
      if (payload.output) {
        if (entry.target.kind === 'terminal') entry.target.offset = payload.output.nextOffset
        entry.handlers.output?.(payload.output)
      } else if (payload.job) {
        if (entry.target.kind === 'job') {
          entry.target.offset = payload.job.nextOffset
          entry.target.inputOpen = payload.job.inputOpen
        }
        entry.handlers.job?.(payload.job)
      } else if (payload.error) {
        entry.handlers.error?.(payload.error)
      }
    })
    source.addEventListener('auth.expired', () => this.disable())
    source.addEventListener('error', () => {
      this.streamId = ''
      if (source.readyState === 2 /* CLOSED */) {
        this.failures = MAX_FAILURES_BEFORE_READY
      } else {
        this.failures += 1
      }
      if (this.failures >= MAX_FAILURES_BEFORE_READY) this.disable()
      else this.armReadyTimer()
    })
  }

  private armReadyTimer(): void {
    this.clearReadyTimer()
    // A buffering proxy delivers nothing; fall back instead of waiting forever.
    this.readyTimer = setTimeout(() => {
      if (!this.streamId) this.disable()
    }, READY_TIMEOUT_MS)
  }

  private clearReadyTimer(): void {
    if (this.readyTimer) clearTimeout(this.readyTimer)
    this.readyTimer = undefined
  }

  private scheduleFlush(): void {
    if (this.flushTimer || !this.streamId) return
    this.flushTimer = setTimeout(() => {
      this.flushTimer = undefined
      void this.flush()
    }, SUBSCRIBE_DEBOUNCE_MS)
  }

  private async flush(): Promise<void> {
    if (!this.streamId || (!this.pendingAdd.size && !this.pendingRemove.size)) return
    const streamId = this.streamId
    const addKeys = [...this.pendingAdd]
    const remove = [...this.pendingRemove]
    this.pendingAdd.clear()
    this.pendingRemove.clear()
    const add = addKeys
      .map((key) => this.entries.get(key)?.target)
      .filter((target): target is TerminalStreamTarget => Boolean(target))
    try {
      await this.transport.subscribe({ streamId, add, remove })
    } catch {
      if (streamId !== this.streamId) return
      // The server could not take these subscriptions (limit or stale
      // stream): those terminals keep working over polling.
      for (const key of addKeys) {
        const entry = this.entries.get(key)
        if (!entry) continue
        this.entries.delete(key)
        entry.handlers.unavailable?.()
      }
    }
  }

  private disable(): void {
    if (this.disabled) return
    this.disabled = true
    this.closeSource()
    const entries = [...this.entries.values()]
    this.entries.clear()
    for (const entry of entries) entry.handlers.unavailable?.()
  }

  private closeSource(): void {
    this.clearReadyTimer()
    if (this.flushTimer) clearTimeout(this.flushTimer)
    this.flushTimer = undefined
    this.source?.close()
    this.source = undefined
    this.streamId = ''
    this.failures = 0
  }

  /** Allows a later subscription to retry streaming, e.g. after re-login. */
  reset(): void {
    this.closeSource()
    this.disabled = false
  }
}
