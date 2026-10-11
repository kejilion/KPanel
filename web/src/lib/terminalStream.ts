// One Server-Sent Events connection per tab carries output for every open
// terminal (host, batch and task terminals). Components subscribe with their
// current offset; when the stream is unavailable (no EventSource, a proxy that
// buffers or blocks it, or repeated failures) subscribe() returns null or the
// subscription reports `unavailable`, and callers keep their polling path.
import type { AppTerminalChunk, TerminalOutput } from '@/types/api'
import { terminalRequest } from './terminalRequest'

export type TerminalStreamTarget =
  | { kind: 'terminal'; id: string; offset: number }
  | { kind: 'job'; job: 'app' | 'site' | 'diagnostic' | 'environment'; id: string; offset: number; inputOpen: boolean }

export interface TerminalStreamHandlers {
  connected?: () => void
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
  subscribe(body: { streamId: string; add: unknown[]; remove: string[] }, signal: AbortSignal): Promise<unknown>
  createSource?: (url: string) => EventSource
}

interface Entry {
  target: TerminalStreamTarget
  handlers: TerminalStreamHandlers
}

const MAX_FAILURES_BEFORE_READY = 3
const READY_TIMEOUT_MS = 5000
const SUBSCRIBE_DEBOUNCE_MS = 10
// Terminals briefly unsubscribe when minimized or while xterm drains a burst;
// keep the idle connection so resuming does not reopen the stream each time.
const IDLE_CLOSE_MS = 5000

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
  private idleTimer?: ReturnType<typeof setTimeout>
  private readonly entries = new Map<string, Entry>()
  private pendingAdd = new Set<string>()
  private pendingRemove = new Set<string>()
  private subscriptionRequest?: AbortController

  constructor(
    private readonly transport: TerminalStreamTransport,
    private readonly idleCloseMs = IDLE_CLOSE_MS,
  ) {}

  get available(): boolean {
    return !this.disabled
  }

  subscribe(target: TerminalStreamTarget, handlers: TerminalStreamHandlers): TerminalStreamSubscription | null {
    if (this.disabled || !this.canStream()) return null
    this.clearIdleTimer()
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
        if (!this.entries.size) this.scheduleIdleClose()
      },
    }
  }

  private scheduleIdleClose(): void {
    this.clearIdleTimer()
    this.idleTimer = setTimeout(() => {
      this.idleTimer = undefined
      if (!this.entries.size) this.closeSource()
    }, this.idleCloseMs)
  }

  private clearIdleTimer(): void {
    if (this.idleTimer) clearTimeout(this.idleTimer)
    this.idleTimer = undefined
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
      if (this.source !== source) return
      this.cancelSubscriptionRequest()
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
      for (const entry of this.entries.values()) entry.handlers.connected?.()
    })
    source.addEventListener('output', (event) => {
      if (this.source !== source) return
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
    source.addEventListener('auth.expired', () => { if (this.source === source) this.disable() })
    source.addEventListener('error', () => {
      if (this.source !== source) return
      this.streamId = ''
      this.cancelSubscriptionRequest()
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
    if (this.subscriptionRequest || !this.streamId || (!this.pendingAdd.size && !this.pendingRemove.size)) return
    const streamId = this.streamId
    const addKeys = [...this.pendingAdd]
    const entries = new Map(addKeys.map(key => [key, this.entries.get(key)]))
    const remove = [...this.pendingRemove]
    this.pendingAdd.clear()
    this.pendingRemove.clear()
    const add = addKeys
      .map((key) => this.entries.get(key)?.target)
      .filter((target): target is TerminalStreamTarget => Boolean(target))
    const controller = new AbortController()
    this.subscriptionRequest = controller
    try {
      await terminalRequest(controller, signal => this.transport.subscribe({ streamId, add, remove }, signal))
    } catch {
      if (this.subscriptionRequest !== controller || streamId !== this.streamId) return
      if (controller.signal.aborted) {
        // A timed-out add may still reach the server. Closing this stream
        // cancels every pump; a racing remove alone cannot guarantee cleanup.
        this.disable()
        return
      }
      // The server could not take these subscriptions (limit or stale
      // stream): those terminals keep working over polling.
      for (const key of addKeys) {
        const entry = this.entries.get(key)
        if (!entry || entry !== entries.get(key)) continue
        this.entries.delete(key)
        // The server may have started some of this batch before rejecting
        // the rest; release those pumps instead of leaving them orphaned.
        this.pendingRemove.add(key)
        entry.handlers.unavailable?.()
      }
      this.scheduleFlush()
    } finally {
      if (this.subscriptionRequest === controller) {
        this.subscriptionRequest = undefined
        if (this.pendingAdd.size || this.pendingRemove.size) this.scheduleFlush()
      }
    }
  }

  private cancelSubscriptionRequest(): void {
    const controller = this.subscriptionRequest
    this.subscriptionRequest = undefined
    controller?.abort()
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
    this.cancelSubscriptionRequest()
    this.clearReadyTimer()
    this.clearIdleTimer()
    if (this.flushTimer) clearTimeout(this.flushTimer)
    this.flushTimer = undefined
    this.source?.close()
    this.source = undefined
    this.streamId = ''
    this.failures = 0
    this.pendingAdd.clear()
    this.pendingRemove.clear()
  }

  /** Allows a later subscription to retry streaming, e.g. after re-login. */
  reset(): void {
    this.closeSource()
    this.disabled = false
  }
}
