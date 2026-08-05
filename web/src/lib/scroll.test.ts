import { describe, expect, it, vi } from 'vitest'
import { containWheelScroll } from './scroll'

function wheel(deltaY: number) {
  return {
    deltaY,
    preventDefault: vi.fn(),
    stopPropagation: vi.fn(),
  } as unknown as WheelEvent
}

function scroller(scrollTop: number, clientHeight = 100, scrollHeight = 300) {
  return { scrollTop, clientHeight, scrollHeight } as HTMLElement
}

describe('containWheelScroll', () => {
  it('keeps normal wheel movement inside the scroller', () => {
    const event = wheel(40)
    containWheelScroll(event, scroller(80))

    expect(event.preventDefault).not.toHaveBeenCalled()
    expect(event.stopPropagation).toHaveBeenCalledOnce()
  })

  it('prevents downward scroll chaining at the bottom boundary', () => {
    const event = wheel(40)
    containWheelScroll(event, scroller(200))

    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(event.stopPropagation).toHaveBeenCalledOnce()
  })

  it('prevents upward scroll chaining at the top boundary', () => {
    const event = wheel(-40)
    containWheelScroll(event, scroller(0))

    expect(event.preventDefault).toHaveBeenCalledOnce()
    expect(event.stopPropagation).toHaveBeenCalledOnce()
  })
})
