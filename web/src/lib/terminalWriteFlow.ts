// xterm.js parses writes asynchronously. Without back-pressure a fast producer
// (a redrawing TUI, or several terminals shown side by side) grows the unparsed
// queue without bound and starves the main thread. Terminals stop pulling
// output once the queue passes the high watermark and resume from their own
// offset below the low one; the server keeps the bytes, so nothing is lost.
export const terminalWriteHighWatermark = 512 << 10
export const terminalWriteLowWatermark = 64 << 10

export class TerminalWriteFlow {
  private pending = 0
  private paused = false
  private generation = 0

  constructor(private readonly resume: () => void) {}

  /** True while output should stay paused until xterm drains. */
  get blocked(): boolean {
    return this.paused
  }

  /**
   * Records bytes handed to xterm. Call the returned function from the write
   * callback once xterm has parsed them.
   */
  track(bytes: number): () => void {
    const generation = this.generation
    this.pending += bytes
    if (this.pending >= terminalWriteHighWatermark) this.paused = true
    return () => {
      if (generation !== this.generation) return
      this.pending -= bytes
      if (this.paused && this.pending <= terminalWriteLowWatermark) {
        this.paused = false
        this.resume()
      }
    }
  }

  /** Forgets writes of a discarded buffer, e.g. after the terminal switched jobs. */
  reset(): void {
    this.generation++
    this.pending = 0
    this.paused = false
  }
}
