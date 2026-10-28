import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { IDecorationOptions, Terminal } from '@xterm/xterm'
import { terminalColorTokens, TerminalKeywordColorsAddon } from './terminalKeywordColors'

function fixture(texts = ['ERROR WARNING SUCCESS']) {
  const events: Record<string, () => void> = {}
  const subscribe = (name: string) => (callback: () => void) => {
    events[name] = callback
    return { dispose: vi.fn(() => { delete events[name] }) }
  }
  const lines = texts.map(text => {
    const cells = Array.from(text).flatMap(char => {
      const width = /[\u4e00-\u9fff]/.test(char) ? 2 : 1
      const cell = { getWidth: () => width, getChars: () => char, isFgDefault: () => true, isBgDefault: () => true, isInverse: () => false }
      return width === 2 ? [cell, { ...cell, getWidth: () => 0 }] : [cell]
    })
    return { length: cells.length, getCell: (x: number) => cells[x], cells, isWrapped: false }
  })
  const buffer = { type: 'normal', viewportY: 0, baseY: 0, cursorY: 0, length: lines.length, getLine: vi.fn((row: number) => lines[row]) }
  const markers: Array<{ line: number; isDisposed: boolean; dispose: () => void }> = []
  const registerDecoration = vi.fn((_options: IDecorationOptions) => ({ dispose: vi.fn() }))
  const terminal = {
    cols: Math.max(80, ...lines.map(line => line.length)), rows: 24, buffer: { active: buffer, onBufferChange: subscribe('buffer') },
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
  it('recognizes IPv4, IPv6 and scoped addresses with separately colored ports', () => {
    const text = '127.0.0.1:8080 [2001:db8::1]:443 ::1 fe80::1%eth0 0.0.0.0:22'
    expect(terminalColorTokens(text).map(({ text, color }) => [text, color])).toEqual([
      ['127.0.0.1', 'cyan'], ['8080', 'magenta'], ['2001:db8::1', 'cyan'], ['443', 'magenta'],
      ['::1', 'cyan'], ['fe80::1%eth0', 'cyan'], ['0.0.0.0', 'cyan'], ['22', 'magenta'],
    ])
  })

  it('recognizes explicit port labels, host endpoints and protocol notation', () => {
    const text = 'port 22 PORT=8080 端口：443 localhost:3000 example.com:8443 *:53 80/tcp 53/udp'
    expect(terminalColorTokens(text).map(token => token.text)).toEqual(['22', '8080', '443', '3000', '8443', '53', '80', '53'])
    expect(terminalColorTokens(text).every(token => token.color === 'magenta')).toBe(true)
  })

  it('does not treat arbitrary numbers, timestamps, MAC addresses or invalid IP/ports as endpoints', () => {
    expect(terminalColorTokens('2026 8080 12:34:56 aa:bb:cc:dd:ee:ff aa:12:cc:34:ee:56 999.1.2.3 1.2.3.400 1.2.3.4.5 port 65536 host:99999')).toEqual([])
  })

  it('recognizes absolute, relative, home, Windows and quoted paths without nested keyword colors', () => {
    const text = String.raw`ERROR /var/log/error.log ~/app ./config ../data root@host:/etc/nginx C:\Windows\temp \\server\share\log "C:\Program Files\app.log" '/tmp/my file'`
    expect(terminalColorTokens(text).map(token => [token.text, token.color])).toEqual([
      ['ERROR', 'red'], ['/var/log/error.log', 'blue'], ['~/app', 'blue'], ['./config', 'blue'], ['../data', 'blue'],
      ['/etc/nginx', 'blue'], [String.raw`C:\Windows\temp`, 'blue'], [String.raw`\\server\share\log`, 'blue'],
      [String.raw`C:\Program Files\app.log`, 'blue'], ['/tmp/my file', 'blue'],
    ])
  })

  it('keeps offsets exact and excludes adjacent punctuation and URL path fragments', () => {
    const text = '失败：(/var/log/app.log), ip=192.168.1.1:80 https://example.com/path'
    const tokens = terminalColorTokens(text)
    expect(tokens.map(token => token.text)).toEqual(['失败', '/var/log/app.log', '192.168.1.1', '80', 'https://example.com/path'])
    expect(tokens.every(token => text.slice(token.start, token.start + token.text.length) === token.text)).toBe(true)
  })

  it('colors complete HTTP(S) URLs uniformly instead of their embedded IP, port, path or status words', () => {
    const text = 'See https://127.0.0.1:8443/ERROR?q=OK and (http://[::1]:8080/path).'
    expect(terminalColorTokens(text).map(token => [token.text, token.color])).toEqual([
      ['https://127.0.0.1:8443/ERROR?q=OK', 'blue'], ['http://[::1]:8080/path', 'blue'],
    ])
  })

  it('keeps naturally wrapped URLs uniform, including when their first row is above the viewport', () => {
    const url = 'https://host:8080/ERROR?q=OK'
    const f = fixture([url.slice(0, 20), url.slice(20)])
    f.terminal.cols = 20
    f.lines[1]!.isWrapped = true
    vi.advanceTimersByTime(100)
    expect(f.options().map(({ x, width, foregroundColor }) => [x, width, foregroundColor])).toEqual([
      [0, 20, '#76a4c7'], [0, url.length - 20, '#76a4c7'],
    ])
    f.buffer.viewportY = 1
    f.events.scroll!()
    vi.advanceTimersByTime(100)
    expect(f.options().at(-1)!.foregroundColor).toBe('#76a4c7')
    expect(f.markers.filter(marker => !marker.isDisposed).map(marker => marker.line)).toEqual([1])
    f.addon.dispose()
  })

  it('preserves native foreground colors on addresses and paths too', () => {
    const f = fixture(['127.0.0.1:80 /var/log/app.log'])
    f.lines[0]!.cells.forEach(cell => { cell.isFgDefault = () => false })
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(0)
    f.addon.dispose()
  })

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

  it('preserves explicit ANSI backgrounds instead of risking unreadable foreground colors', () => {
    const f = fixture(['ERROR 127.0.0.1:80 /tmp/log https://example.com'])
    f.lines[0]!.cells.forEach(cell => { cell.isBgDefault = () => false })
    vi.advanceTimersByTime(100)
    expect(f.options()).toHaveLength(0)
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
