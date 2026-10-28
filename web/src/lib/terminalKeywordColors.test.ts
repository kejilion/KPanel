import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { IDecorationOptions, Terminal } from '@xterm/xterm'
import { TerminalKeywordColorsAddon } from './terminalKeywordColors'

function fixture(texts = ['ERROR WARNING SUCCESS']) {
  const events: Record<string, () => void> = {}
  const subscribe = (name: string) => (callback: () => void) => {
    events[name] = callback
    return { dispose: vi.fn(() => { delete events[name] }) }
  }
  const lines = texts.map(text => {
    const cells = Array.from(text).flatMap(char => {
      const width = /[\u4e00-\u9fff]/.test(char) ? 2 : 1
      const cell = { getWidth: () => width, getChars: () => char, isFgDefault: () => true, isInverse: () => false }
      return width === 2 ? [cell, { ...cell, getWidth: () => 0 }] : [cell]
    })
    return { length: cells.length, getCell: (x: number) => cells[x], cells }
  })
  const buffer = { type: 'normal', viewportY: 0, baseY: 0, cursorY: 0, length: lines.length, getLine: vi.fn((row: number) => lines[row]) }
  const markers: Array<{ line: number; isDisposed: boolean; dispose: () => void }> = []
  const registerDecoration = vi.fn((_options: IDecorationOptions) => ({ dispose: vi.fn() }))
  const terminal = {
    cols: 80, rows: 24, buffer: { active: buffer, onBufferChange: subscribe('buffer') },
    options: { theme: { red: '#ff0000', yellow: '#ffff00', green: '#00ff00' } },
    onWriteParsed: subscribe('write'), onScroll: subscribe('scroll'), onResize: subscribe('resize'), onRender: subscribe('render'),
    registerMarker: (offset: number) => {
      const marker = { line: buffer.baseY + buffer.cursorY + offset, isDisposed: false, dispose() { this.isDisposed = true } }
      markers.push(marker)
      return marker
    }, registerDecoration,
  }
  const addon = new TerminalKeywordColorsAddon()
  addon.activate(terminal as unknown as Terminal)
  return { addon, terminal, buffer, lines, markers, events, registerDecoration,
    options: () => registerDecoration.mock.calls.map(([options]) => options) }
}

beforeEach(() => vi.useFakeTimers())
afterEach(() => vi.useRealTimers())

describe('terminal keyword colors', () => {
  it('colors common English and Chinese words without coloring word fragments', () => {
    const f = fixture(['ERROR fatal Failed failure warning WARN success successful OK 错误 失败 警告 成功 完成', 'terror warnings book token'])
    vi.advanceTimersByTime(100)
    expect(f.options().map(item => item.foregroundColor)).toEqual([
      '#ff0000', '#ff0000', '#ff0000', '#ff0000', '#ffff00', '#ffff00', '#00ff00', '#00ff00', '#00ff00',
      '#ff0000', '#ff0000', '#ffff00', '#00ff00', '#00ff00',
    ])
    f.addon.dispose()
  })

  it('uses cell coordinates for wide Chinese text and leaves the buffer unchanged', () => {
    const f = fixture(['中文 错误 OK'])
    vi.advanceTimersByTime(100)
    expect(f.options().map(({ x, width }) => ({ x, width }))).toEqual([{ x: 5, width: 4 }, { x: 10, width: 2 }])
    expect(f.lines[0]!.cells.map(cell => cell.getWidth() ? cell.getChars() : '').join('')).toBe('中文 错误 OK')
    f.addon.dispose()
  })

  it('preserves explicit ANSI foreground and inverse cells', () => {
    const f = fixture()
    f.lines[0]!.cells[0]!.isFgDefault = () => false
    f.lines[0]!.cells[6]!.isInverse = () => true
    vi.advanceTimersByTime(100)
    expect(f.options().map(item => item.foregroundColor)).toEqual(['#00ff00'])
    f.addon.dispose()
  })

  it('coalesces output and reuses unchanged decorations, including split-word updates', () => {
    const f = fixture(['ERR'])
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(0)
    f.lines[0]!.cells[2]!.getChars = () => 'ROR'
    for (let i = 0; i < 1000; i++) f.events.write!()
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(1)
    f.events.write!()
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(1)
    f.addon.dispose()
  })

  it('removes colors on alternate-screen entry and restores them on exit', () => {
    const f = fixture()
    vi.advanceTimersByTime(100)
    f.buffer.type = 'alternate'
    f.events.buffer!()
    vi.advanceTimersByTime(100)
    expect(f.markers.every(marker => marker.isDisposed)).toBe(true)
    expect(f.options()).toHaveLength(3)
    f.buffer.type = 'normal'
    f.events.buffer!()
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(6)
    f.addon.dispose()
  })

  it('scans only the viewport with a hard decoration budget', () => {
    const f = fixture(Array(5000).fill('ERROR '.repeat(20)))
    f.buffer.viewportY = 4900
    f.buffer.baseY = 4976
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(128)
    expect(f.buffer.getLine.mock.calls.every(([row]) => row >= 4900 && row < 4924)).toBe(true)
    expect(f.markers[0]!.line).toBe(4900)
    f.addon.dispose()
    expect(f.markers.every(marker => marker.isDisposed)).toBe(true)
    expect(Object.keys(f.events)).toHaveLength(0)
    expect(vi.getTimerCount()).toBe(0)
  })

  it('refreshes stale markers after buffer trimming and colors after theme changes', () => {
    const f = fixture()
    vi.advanceTimersByTime(100)
    f.markers[0]!.isDisposed = true
    f.events.write!()
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(6)
    f.terminal.options.theme.red = '#cc0000'
    f.events.render!()
    vi.advanceTimersByTime(100)
    expect(f.options()[6]!.foregroundColor).toBe('#cc0000')
    f.addon.dispose()
  })

  it('disposes a pending refresh without creating decorations', () => {
    const f = fixture()
    f.addon.dispose()
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(0)
  })
})
