// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ closeOnUnload: vi.fn() }))
vi.mock('@/lib/api', () => ({ api: { terminals: { closeOnUnload: mocks.closeOnUnload } } }))

import { closeTerminalsOnPageHide } from './terminalPageHide'

function pageHide(persisted: boolean): void {
  const event = new Event('pagehide') as PageTransitionEvent
  Object.defineProperty(event, 'persisted', { value: persisted })
  window.dispatchEvent(event)
}

afterEach(() => mocks.closeOnUnload.mockReset())

describe('closing terminals when the page goes away', () => {
  it('closes every session it was given when the page is reloaded or closed', () => {
    const stop = closeTerminalsOnPageHide(() => ['a', 'b'])
    pageHide(false)
    expect(mocks.closeOnUnload.mock.calls).toEqual([['a'], ['b']])
    stop()
  })

  it('leaves terminals open for a page that may come back from the back/forward cache', () => {
    const stop = closeTerminalsOnPageHide(() => ['a'])
    pageHide(true)
    expect(mocks.closeOnUnload).not.toHaveBeenCalled()
    stop()
  })

  it('asks for the sessions at the moment the page goes away, not when it registered', () => {
    const open = new Set(['a'])
    const stop = closeTerminalsOnPageHide(() => open)
    open.add('b')
    open.delete('a')
    pageHide(false)
    expect(mocks.closeOnUnload.mock.calls).toEqual([['b']])
    stop()
  })

  it('stops listening once removed', () => {
    const stop = closeTerminalsOnPageHide(() => ['a'])
    stop()
    pageHide(false)
    expect(mocks.closeOnUnload).not.toHaveBeenCalled()
  })

  it('does nothing when no terminal is open', () => {
    const stop = closeTerminalsOnPageHide(() => [])
    pageHide(false)
    expect(mocks.closeOnUnload).not.toHaveBeenCalled()
    stop()
  })
})
