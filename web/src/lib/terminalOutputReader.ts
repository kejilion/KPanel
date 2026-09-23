// Sequential reader over a terminal session's output for loops that process
// chunks one at a time (batch execution). It uses the shared push stream when
// available and long-polling otherwise; next() resolves with null when no
// output arrived within the idle window, like an empty long-poll.
import { api, ApiError, terminalStream } from '@/lib/api'
import type { TerminalStreamClient, TerminalStreamSubscription } from '@/lib/terminalStream'
import type { TerminalOutput } from '@/types/api'

export interface TerminalOutputReader {
  next(signal: AbortSignal): Promise<TerminalOutput | null>
  close(): void
}

interface ReaderDependencies {
  stream: Pick<TerminalStreamClient, 'subscribe'>
  output: (sessionId: string, offset: number, signal?: AbortSignal) => Promise<TerminalOutput>
  idleMs: number
}

const defaultDependencies = (): ReaderDependencies => ({
  stream: terminalStream,
  output: api.terminals.output,
  idleMs: 1000,
})

export function createTerminalOutputReader(
  sessionId: string,
  initialOffset: number,
  dependencies: ReaderDependencies = defaultDependencies(),
): TerminalOutputReader {
  let offset = initialOffset
  const queued: TerminalOutput[] = []
  let failure: unknown
  let waiter: (() => void) | undefined
  let subscription: TerminalStreamSubscription | null = null
  let streaming = false
  let closed = false
  let queuedBytes = 0
  const wake = () => {
    const resolve = waiter
    waiter = undefined
    resolve?.()
  }
  subscription = dependencies.stream.subscribe({ kind: 'terminal', id: sessionId, offset }, {
    output: (chunk) => {
      if (closed || !streaming) return
      // Resume from the last consumed offset if a slow consumer fills the
      // budget; the backend ring reports any truncation explicitly.
      if (queued.length >= 32 || queuedBytes + chunk.data.length > 1024 * 1024) {
        subscription?.close()
        subscription = null
        streaming = false
        queued.length = 0
        queuedBytes = 0
        wake()
        return
      }
      queued.push(chunk)
      queuedBytes += chunk.data.length
      wake()
    },
    error: (code) => {
      if (code === 'terminal_not_found') {
        failure = new ApiError('Terminal session not found', 404, 'terminal_not_found')
        wake()
      }
    },
    unavailable: () => {
      streaming = false
      subscription = null
      wake()
    },
  })
  streaming = subscription !== null

  return {
    async next(signal: AbortSignal): Promise<TerminalOutput | null> {
      if (signal.aborted || closed) throw new DOMException('Aborted', 'AbortError')
      if (!streaming) {
        const chunk = await dependencies.output(sessionId, offset, signal)
        offset = chunk.nextOffset
        return chunk
      }
      if (!queued.length && failure === undefined) {
        await new Promise<void>((resolve) => {
          const finish = () => {
            clearTimeout(timer)
            signal.removeEventListener('abort', finish)
            if (waiter === finish) waiter = undefined
            resolve()
          }
          const timer = setTimeout(finish, dependencies.idleMs)
          waiter = finish
          signal.addEventListener('abort', finish, { once: true })
          if (signal.aborted) finish()
        })
      }
      if (signal.aborted || closed) throw new DOMException('Aborted', 'AbortError')
      if (failure !== undefined) throw failure
      const chunk = queued.shift()
      if (chunk) {
        queuedBytes -= chunk.data.length
        offset = chunk.nextOffset
        return chunk
      }
      // Fell back to polling while waiting: continue from the last offset.
      if (!streaming) return this.next(signal)
      return null
    },
    close(): void {
      closed = true
      subscription?.close()
      subscription = null
      streaming = false
      queued.length = 0
      queuedBytes = 0
      wake()
    },
  }
}
