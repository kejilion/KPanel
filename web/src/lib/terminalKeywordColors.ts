import type { IDisposable, IMarker, ITerminalAddon, Terminal } from '@xterm/xterm'

const keywords = /\b(ERROR|FATAL|FAIL(?:ED|URE)?|WARN(?:ING)?|SUCCESS(?:FUL)?|OK)\b|错误|失败|警告|成功|完成/gi
const error = /^(?:ERROR|FATAL|FAIL(?:ED|URE)?|错误|失败)$/i
const warning = /^(?:WARN(?:ING)?|警告)$/i

type Token = { start: number; text: string; color: 'red' | 'yellow' | 'green' | 'cyan' | 'magenta' | 'blue' }
const pathStart = String.raw`(?:[a-z]:[\\/]|\\\\[^\\\s]+\\|~/|\.{1,2}/|/(?!/))`
const quotedPaths = new RegExp(String.raw`(["'])(${pathStart}[^\r\n]*?)\1`, 'gi')
const paths = new RegExp(String.raw`(?<![\w/\\])${pathStart}[^\s"'<>|;,，；）)\]}]*`, 'gi')
const ipv4 = /(?<![\w.])(?:\d{1,3}\.){3}\d{1,3}(?![\w.])/g
const ipv6 = /(?<![\w:.])(?:[a-f\d]*:){2,}[a-f\d:.]*(?:%[\w.-]+)?(?![\w:.])/gi
const ports = /(?:\bport\s*[:=]?\s*|端口\s*[:：=]?\s*|(?:(?<![\w:])(?:[a-z][\w.-]*|(?:\d{1,3}\.){3}\d{1,3})|\[[a-f\d:.%\w-]+\]|\*):)(\d{1,5})(?![\w:])|\b(\d{1,5})(?=\/(?:tcp|udp)\b)/gi
// Match the terminal link addon's HTTP(S) boundaries, keeping click behavior separate.
const urls = /https?:\/\/[^\s"'!*(){}|\\^<>`]*[^\s"':,.!?{}|\\^~\[\]`()<>]/gi

/** Prefer complete paths over words/addresses inside them; bare numbers are not ports. */
export function terminalColorTokens(text: string): Token[] {
  const tokens: Token[] = []
  const add = (start: number, value: string, color: Token['color']) => {
    if (!value || tokens.some(token => start < token.start + token.text.length && start + value.length > token.start)) return
    tokens.push({ start, text: value, color })
  }
  for (const match of text.matchAll(urls)) {
    try { new URL(match[0]); add(match.index!, match[0], 'blue') } catch { /* Not a URL. */ }
  }
  for (const match of text.matchAll(quotedPaths)) add(match.index! + 1, match[2]!, 'blue')
  for (const match of text.matchAll(paths)) add(match.index!, match[0].replace(/[.:!?]+$/, ''), 'blue')
  for (const match of text.matchAll(ipv4)) {
    if (match[0].split('.').every(part => Number(part) <= 255)) add(match.index!, match[0], 'cyan')
  }
  for (const match of text.matchAll(ipv6)) {
    // URL's IPv6 parser rejects timestamps, MAC addresses and malformed groups; no network I/O.
    try {
      new URL(`http://[${match[0].split('%')[0]}]/`)
      add(match.index!, match[0], 'cyan')
    } catch { /* Not an IPv6 address. */ }
  }
  for (const match of text.matchAll(ports)) {
    const value = match[1] || match[2]!
    if (Number(value) <= 65535) add(match.index! + match[0].lastIndexOf(value), value, 'magenta')
  }
  for (const match of text.matchAll(keywords)) {
    add(match.index!, match[0], error.test(match[0]) ? 'red' : warning.test(match[0]) ? 'yellow' : 'green')
  }
  return tokens.sort((a, b) => a.start - b.start)
}

/** Display-only, bounded highlighting: never rewrite the PTY byte stream. */
export class TerminalKeywordColorsAddon implements ITerminalAddon {
  private subscriptions: IDisposable[] = []
  private decorations: IDisposable[] = []
  private markers: Array<{ marker: IMarker; row: number }> = []
  private timer: ReturnType<typeof setTimeout> | undefined
  private signature = ''

  activate(terminal: Terminal): void {
    const refresh = () => {
      this.timer = undefined
      const buffer = terminal.buffer.active
      const theme = terminal.options.theme
      const colors = { red: theme?.red || '#d86f74', yellow: theme?.yellow || '#d5ae62', green: theme?.green || '#91b56d',
        cyan: theme?.cyan || '#72aaa7', magenta: theme?.magenta || '#ad8bb8', blue: theme?.blue || '#76a4c7' }
      const matches: Array<{ row: number; x: number; width: number; color: string }> = []
      // Alternate-screen TUIs own their presentation. Work never grows with scrollback.
      if (buffer.type === 'normal' && terminal.cols <= 500) {
        const viewEnd = Math.min(buffer.length, buffer.viewportY + terminal.rows, buffer.viewportY + 120)
        let firstRow = buffer.viewportY
        // A small wrapped-line window keeps ordinary long URLs one color, even after scrolling.
        while (firstRow > 0 && firstRow > buffer.viewportY - 7 && buffer.getLine(firstRow)?.isWrapped) firstRow--
        for (let row = firstRow; row < viewEnd; row++) {
          let text = ''
          const columns: number[] = []
          const plain: boolean[] = []
          let lastRow = row, endColumn = 0
          while (lastRow < row + 7 && buffer.getLine(lastRow + 1)?.isWrapped) lastRow++
          for (let sourceRow = row; sourceRow <= lastRow; sourceRow++) {
            const line = buffer.getLine(sourceRow)
            if (!line) continue
            const offset = (sourceRow - row) * terminal.cols
            for (let x = 0; x < line.length; x++) {
              const cell = line.getCell(x)!
              if (cell.getWidth() === 0) continue
              const chars = cell.getChars() || ' '
              for (let i = 0; i < chars.length; i++) {
                columns.push(offset + x)
                plain.push(cell.isFgDefault() && !cell.isInverse())
              }
              text += chars
            }
            endColumn = offset + line.length
          }
          columns.push(endColumn)
          for (const token of terminalColorTokens(text)) {
            const start = token.start, end = start + token.text.length
            if (!plain.slice(start, end).every(Boolean)) continue
            for (let position = columns[start]!; position < columns[end]!;) {
              const targetRow = row + Math.floor(position / terminal.cols), x = position % terminal.cols
              const width = Math.min(terminal.cols - x, columns[end]! - position)
              if (targetRow >= buffer.viewportY && targetRow < viewEnd) matches.push({ row: targetRow, x, width, color: colors[token.color] })
              position += width
              if (matches.length === 128) break
            }
            if (matches.length === 128) break
          }
          row = lastRow
          if (matches.length === 128) break
        }
      }
      const signature = JSON.stringify([buffer.type, buffer.baseY, buffer.viewportY, matches])
      if (signature === this.signature && this.markers.every(({ marker, row }) => !marker.isDisposed && marker.line === row)) return
      this.signature = signature
      this.clear()
      for (const match of matches) {
        const marker = terminal.registerMarker(match.row - buffer.baseY - buffer.cursorY)
        if (!marker) continue
        this.markers.push({ marker, row: match.row })
        this.decorations.push(marker)
        const decoration = terminal.registerDecoration({ marker, x: match.x, width: match.width, foregroundColor: match.color })
        if (decoration) this.decorations.push(decoration)
      }
    }
    const schedule = () => {
      // Coalesce chatty output; at most ten viewport scans per second.
      if (this.timer === undefined) this.timer = setTimeout(refresh, 100)
    }
    this.subscriptions = [terminal.onWriteParsed(schedule), terminal.onScroll(schedule), terminal.onResize(schedule), terminal.onRender(schedule),
      terminal.buffer.onBufferChange(schedule)]
    schedule()
  }

  private clear(): void {
    this.decorations.forEach(item => item.dispose())
    this.decorations = []
    this.markers = []
  }

  dispose(): void {
    if (this.timer !== undefined) clearTimeout(this.timer)
    this.timer = undefined
    this.subscriptions.forEach(item => item.dispose())
    this.subscriptions = []
    this.clear()
  }
}
