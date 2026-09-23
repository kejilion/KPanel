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
  it('removes abort listeners after each completed wait', async () => {
    let handlers: TerminalStreamHandlers = {}
    const reader = createTerminalOutputReader('s', 0, {
      stream: { subscribe: (_target, next) => { handlers = next; return { close: vi.fn() } } },
      output: vi.fn(), idleMs: 1000,
    })
    const signal = new AbortController().signal
    const added = vi.spyOn(signal, 'addEventListener')
    const removed = vi.spyOn(signal, 'removeEventListener')
    for (let index = 0; index < 3; index++) {
      const pending = reader.next(signal)
      handlers.output?.(chunk('x', index))
      await pending
    }
    expect(removed.mock.calls.filter(([type]) => type === 'abort')).toHaveLength(3)
    expect(removed.mock.calls.map(([, fn]) => fn)).toEqual(added.mock.calls.map(([, fn]) => fn))
    reader.close()
  })

  it('falls back at the consumed offset when pushed output exceeds its queue budget', async () => {
    let handlers: TerminalStreamHandlers = {}
    const close = vi.fn()
    const output = vi.fn().mockResolvedValue(chunk('recovered', 0))
    const reader = createTerminalOutputReader('s', 0, {
      stream: { subscribe: (_target, next) => { handlers = next; return { close } } },
      output, idleMs: 1000,
    })
    for (let index = 0; index < 33; index++) handlers.output?.(chunk('x', index))
    const signal = new AbortController().signal
    expect(await reader.next(signal)).toMatchObject({ data: 'recovered' })
    expect(close).toHaveBeenCalledOnce()
    expect(output).toHaveBeenCalledWith('s', 0, signal)
    reader.close()
  })

  it('rejects an already aborted wait immediately and releases waits on close', async () => {
    const reader = createTerminalOutputReader('s', 0, {
      stream: { subscribe: () => ({ close: vi.fn() }) }, output: vi.fn(), idleMs: 60_000,
    })
    const controller = new AbortController()
    controller.abort()
    await expect(reader.next(controller.signal)).rejects.toMatchObject({ name: 'AbortError' })
    const pending = reader.next(new AbortController().signal)
    reader.close()
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
  })

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
