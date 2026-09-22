import { describe, expect, it, vi } from 'vitest'
import type { TerminalStreamHandlers } from './terminalStream'
import { createTerminalOutputReader } from './terminalOutputReader'
import type { TerminalOutput } from '@/types/api'

vi.mock('@/lib/api', () => ({
  api: { terminals: { output: vi.fn() } },
  ApiError: class extends Error {
    constructor(message: string, readonly status = 0, readonly code = '') { super(message) }
  },
  terminalStream: { subscribe: () => null },
}))

function chunk(data: string, offset: number): TerminalOutput {
  return { data, offset, nextOffset: offset + data.length, truncated: false, closed: false }
}

describe('terminal output reader', () => {
  it('reads pushed chunks, idles with null and resumes polling at the pushed offset', async () => {
    let handlers: TerminalStreamHandlers = {}
    const output = vi.fn().mockResolvedValue(chunk('polled', 9))
    const reader = createTerminalOutputReader('s', 0, {
      stream: { subscribe: (_target, next) => { handlers = next; return { close: vi.fn() } } },
      output,
      idleMs: 20,
    })
    const signal = new AbortController().signal
    handlers.output?.(chunk('pushed', 3))
    expect(await reader.next(signal)).toMatchObject({ data: 'pushed', nextOffset: 9 })
    expect(await reader.next(signal)).toBeNull()
    handlers.unavailable?.()
    expect(await reader.next(signal)).toMatchObject({ data: 'polled' })
    expect(output).toHaveBeenCalledWith('s', 9, signal)
  })

  it('surfaces a vanished session as terminal_not_found', async () => {
    let handlers: TerminalStreamHandlers = {}
    const reader = createTerminalOutputReader('s', 0, {
      stream: { subscribe: (_target, next) => { handlers = next; return { close: vi.fn() } } },
      output: vi.fn(),
      idleMs: 1000,
    })
    const pending = reader.next(new AbortController().signal)
    handlers.error?.('terminal_not_found')
    await expect(pending).rejects.toMatchObject({ code: 'terminal_not_found' })
  })
})
