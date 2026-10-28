// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { installTerminalScrollbar } from './terminalScrollbar'

function fixture() {
  let frame: FrameRequestCallback | undefined
  vi.stubGlobal('requestAnimationFrame', vi.fn((callback: FrameRequestCallback) => { frame = callback; return 1 }))
  vi.stubGlobal('cancelAnimationFrame', vi.fn(() => { frame = undefined }))
  const flush = () => { const callback = frame; frame = undefined; callback?.(0) }
  const host = document.createElement('div')
  host.innerHTML = '<div class="xterm-viewport"></div><div class="xterm-screen"></div>'
  const viewport = host.firstElementChild as HTMLElement
  Object.defineProperty(viewport, 'clientHeight', { value: 400 })
  Object.defineProperty(host.lastElementChild, 'clientHeight', { value: 400, configurable: true })
  const events = { scroll: () => {}, write: () => {}, resize: () => {} }
  const disposals = [vi.fn(), vi.fn(), vi.fn()]
  const buffer = { baseY: 100, viewportY: 100 }
  const terminal = {
    rows: 20, buffer: { active: buffer },
    onScroll: (fn: () => void) => { events.scroll = fn; return { dispose: disposals[0] } },
    onWriteParsed: (fn: () => void) => { events.write = fn; return { dispose: disposals[1] } },
    onResize: (fn: () => void) => { events.resize = fn; return { dispose: disposals[2] } },
    scrollToLine: vi.fn((line: number) => { buffer.viewportY = line; events.scroll() }),
  }
  const disconnect = vi.fn()
  vi.stubGlobal('ResizeObserver', class { observe() {} disconnect = disconnect })
  const dispose = installTerminalScrollbar(host, terminal as unknown as Terminal)
  return { host, viewport, terminal, buffer, events, dispose, disposals, disconnect, flush }
}

afterEach(() => vi.unstubAllGlobals())
describe('native terminal scrollbar synchronization', () => {
  it('maps buffer history to native scroll range and follows xterm wheel scrolling', () => {
    const f = fixture()
    expect((f.viewport.firstElementChild as HTMLElement).style.height).toBe('2400px')
    expect(f.viewport.scrollTop).toBe(2000)
    f.buffer.viewportY = 40
    f.events.scroll()
    f.flush()
    expect(f.viewport.scrollTop).toBe(800)
    f.viewport.dispatchEvent(new Event('scroll'))
    expect(f.terminal.scrollToLine).not.toHaveBeenCalled()
    f.dispose()
  })
  it('moves xterm when native arrows or dragging change the scroll position', () => {
    const f = fixture()
    f.viewport.scrollTop = 300
    f.viewport.dispatchEvent(new Event('scroll'))
    expect(f.terminal.scrollToLine).toHaveBeenCalledExactlyOnceWith(15)
    expect(f.viewport.scrollTop).toBe(300)
    f.dispose()
  })
  it.each([0, 1, 2])('preserves a pending return to the bottom before new output is parsed (gap %s)', gap => {
    const f = fixture()
    f.buffer.viewportY = 30
    f.events.scroll()
    f.flush()
    // Native scroll events are asynchronous: output can arrive before the event.
    f.viewport.scrollTop = (100 - gap) * 20
    f.buffer.baseY = 150
    f.events.write()
    f.flush()
    f.viewport.dispatchEvent(new Event('scroll'))
    expect(f.buffer.viewportY).toBe(150)
    expect(f.viewport.scrollTop).toBe(3000)
    f.dispose()
  })
  it('preserves a pending move into history before output synchronization', () => {
    const f = fixture()
    f.viewport.scrollTop = 600
    f.buffer.baseY = f.buffer.viewportY = 150
    f.events.write()
    f.flush()
    f.viewport.dispatchEvent(new Event('scroll'))
    expect(f.buffer.viewportY).toBe(30)
    expect(f.viewport.scrollTop).toBe(600)
    f.dispose()
  })
  it('preserves scrolled history during output and clears the range in the alternate buffer', () => {
    const f = fixture()
    f.buffer.viewportY = 30
    f.buffer.baseY = 200
    f.events.write()
    f.flush()
    expect(f.viewport.scrollTop).toBe(600)
    expect((f.viewport.firstElementChild as HTMLElement).style.height).toBe('4400px')
    f.buffer.baseY = f.buffer.viewportY = 0
    f.events.write()
    f.flush()
    expect(f.viewport.scrollTop).toBe(0)
    expect((f.viewport.firstElementChild as HTMLElement).style.height).toBe('400px')
    f.dispose()
  })
  it('coalesces output sync until xterm has updated its own range and cancels on disposal', () => {
    const f = fixture()
    f.buffer.viewportY = 30
    f.events.scroll(); f.flush()
    vi.mocked(requestAnimationFrame).mockClear()
    f.viewport.scrollTop = 2000
    f.buffer.baseY = 150
    for (let i = 0; i < 50; i++) f.events.scroll()
    f.events.write()
    expect(requestAnimationFrame).toHaveBeenCalledOnce()
    expect(f.terminal.scrollToLine).not.toHaveBeenCalled()
    f.flush()
    expect(f.buffer.viewportY).toBe(150)
    f.events.write()
    f.dispose(); f.flush()
    expect(cancelAnimationFrame).toHaveBeenCalledWith(1)
    expect(f.viewport.children).toHaveLength(0)
  })
  it('does not mistake geometry clamping or a buffer switch for a native scroll', () => {
    const f = fixture()
    f.viewport.scrollTop = 0
    Object.defineProperty(f.host.lastElementChild, 'clientHeight', { value: 200 })
    f.events.resize(); f.flush()
    expect(f.buffer.viewportY).toBe(100)
    f.viewport.scrollTop = 0
    f.terminal.buffer.active = { baseY: 0, viewportY: 0 }
    f.events.write(); f.flush()
    expect(f.terminal.scrollToLine).not.toHaveBeenCalled()
    f.dispose()
  })
  it('recalculates row height on resize and releases listeners on disposal', () => {
    const f = fixture()
    f.terminal.rows = 40
    f.events.resize()
    f.flush()
    expect(f.viewport.scrollTop).toBe(1000)
    f.dispose()
    f.viewport.scrollTop = 200
    f.viewport.dispatchEvent(new Event('scroll'))
    f.events.write()
    expect(f.terminal.scrollToLine).not.toHaveBeenCalled()
    expect(f.viewport.children).toHaveLength(0)
    f.disposals.forEach(dispose => expect(dispose).toHaveBeenCalledOnce())
    expect(f.disconnect).toHaveBeenCalledOnce()
  })
})
