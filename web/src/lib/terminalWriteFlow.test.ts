import { describe, expect, it, vi } from 'vitest'
import { TerminalWriteFlow, terminalWriteHighWatermark, terminalWriteLowWatermark } from './terminalWriteFlow'

describe('TerminalWriteFlow', () => {
  it('pauses at the high watermark and resumes once drained below the low one', () => {
    const resume = vi.fn()
    const flow = new TerminalWriteFlow(resume)
    const chunk = 64 << 10
    const parsed: Array<() => void> = []
    while (!flow.blocked) parsed.push(flow.track(chunk))
    expect(parsed.length * chunk).toBeGreaterThanOrEqual(terminalWriteHighWatermark)
    while (parsed.length * chunk > terminalWriteLowWatermark) {
      parsed.shift()!()
      if (parsed.length * chunk > terminalWriteLowWatermark) expect(resume).not.toHaveBeenCalled()
    }
    expect(flow.blocked).toBe(false)
    expect(resume).toHaveBeenCalledOnce()
  })

  it('ignores parse callbacks of a buffer discarded by reset', () => {
    const resume = vi.fn()
    const flow = new TerminalWriteFlow(resume)
    const stale = flow.track(terminalWriteHighWatermark)
    expect(flow.blocked).toBe(true)
    flow.reset()
    expect(flow.blocked).toBe(false)
    const current = flow.track(terminalWriteHighWatermark - 1)
    stale()
    expect(flow.blocked).toBe(false)
    expect(resume).not.toHaveBeenCalled()
    current()
    expect(resume).not.toHaveBeenCalled()
  })
})
