import { describe, expect, it } from 'vitest'
import { joinTerminalSizeGroup } from './terminalSizeOwnership'

describe('terminal size ownership', () => {
  it('lets exactly one view drive a PTY and hands ownership to the latest claimant', () => {
    const detail = joinTerminalSizeGroup('app:job-owner')
    const window = joinTerminalSizeGroup('app:job-owner')
    expect(detail.isOwner()).toBe(true)
    expect(window.isOwner()).toBe(false)

    window.claim()
    expect(window.isOwner()).toBe(true)
    expect(detail.isOwner()).toBe(false)

    window.publish({ rows: 40, columns: 140 })
    detail.publish({ rows: 10, columns: 20 })
    expect(detail.ownerSize()).toEqual({ rows: 40, columns: 140 })

    window.leave()
    expect(detail.isOwner()).toBe(true)
    detail.leave()
    expect(joinTerminalSizeGroup('app:job-owner').isOwner()).toBe(true)
  })

  it('keeps separate PTYs independent', () => {
    const first = joinTerminalSizeGroup('app:job-one')
    const second = joinTerminalSizeGroup('app:job-two')
    expect(first.isOwner()).toBe(true)
    expect(second.isOwner()).toBe(true)
    first.leave()
    first.leave()
    expect(second.isOwner()).toBe(true)
    second.leave()
  })
})
