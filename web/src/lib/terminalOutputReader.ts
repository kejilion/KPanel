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
  const wake = () => {
    const resolve = waiter
    waiter = undefined
    resolve?.()
  }
  subscription = dependencies.stream.subscribe({ kind: 'terminal', id: sessionId, offset }, {
    output: (chunk) => {
      queued.push(chunk)
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
      if (!streaming) {
        const chunk = await dependencies.output(sessionId, offset, signal)
        offset = chunk.nextOffset
        return chunk
      }
      if (!queued.length && failure === undefined) {
        await new Promise<void>((resolve) => {
          const timer = setTimeout(() => { waiter = undefined; resolve() }, dependencies.idleMs)
          waiter = () => { clearTimeout(timer); resolve() }
          signal.addEventListener('abort', () => waiter?.(), { once: true })
        })
      }
      if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
      if (failure !== undefined) throw failure
      const chunk = queued.shift()
      if (chunk) {
        offset = chunk.nextOffset
        return chunk
      }
      // Fell back to polling while waiting: continue from the last offset.
      if (!streaming) return this.next(signal)
      return null
    },
    close(): void {
      subscription?.close()
      subscription = null
      streaming = false
    },
  }
}
