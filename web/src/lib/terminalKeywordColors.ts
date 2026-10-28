import type { IDisposable, IMarker, ITerminalAddon, Terminal } from '@xterm/xterm'

const keywords = /\b(ERROR|FATAL|FAIL(?:ED|URE)?|WARN(?:ING)?|SUCCESS(?:FUL)?|OK)\b|错误|失败|警告|成功|完成/gi
const error = /^(?:ERROR|FATAL|FAIL(?:ED|URE)?|错误|失败)$/i
const warning = /^(?:WARN(?:ING)?|警告)$/i

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
      const colors = [theme?.red || '#d86f74', theme?.yellow || '#d5ae62', theme?.green || '#91b56d']
      const matches: Array<{ row: number; x: number; width: number; color: string }> = []
      // Alternate-screen TUIs own their presentation. Work never grows with scrollback.
      if (buffer.type === 'normal' && terminal.cols <= 500) {
        for (let row = buffer.viewportY; row < Math.min(buffer.length, buffer.viewportY + terminal.rows, buffer.viewportY + 120); row++) {
          const line = buffer.getLine(row)
          if (!line) continue
          let text = ''
          const columns: number[] = []
          const plain: boolean[] = []
          for (let x = 0; x < line.length; x++) {
            const cell = line.getCell(x)!
            if (cell.getWidth() === 0) continue
            const chars = cell.getChars() || ' '
            for (let i = 0; i < chars.length; i++) {
              columns.push(x)
              plain.push(cell.isFgDefault() && !cell.isInverse())
            }
            text += chars
          }
          columns.push(line.length)
          for (const match of text.matchAll(keywords)) {
            const start = match.index!, end = start + match[0].length
            if (!plain.slice(start, end).every(Boolean)) continue
            matches.push({ row, x: columns[start]!, width: columns[end]! - columns[start]!,
              color: colors[error.test(match[0]) ? 0 : warning.test(match[0]) ? 1 : 2]! })
            if (matches.length === 128) break
          }
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
