// Adapter for the pinned noVNC decoder. RFB's public connect event is only a
// handshake, and canvas resize also blits an empty buffer. Only a completed
// pixel rectangle followed by a drained render queue qualifies as a frame.
type Decoder = {
  _FBU: { encoding: number | null }
  _handleRect(): boolean
  _framebufferUpdate(): boolean
  _display: { flush(): Promise<void> }
}
export function observeVNCFrame(client: unknown, painted: () => void): () => void {
  const decoder = client as Decoder
  if (typeof decoder._handleRect !== 'function' || typeof decoder._framebufferUpdate !== 'function' || typeof decoder._display?.flush !== 'function') throw new Error('Unsupported VNC decoder')
  const rect = decoder._handleRect, frame = decoder._framebufferUpdate
  let pixels = false, active = true
  decoder._handleRect = function () {
    const encoding = this._FBU.encoding
    const complete = rect.call(this)
    // Standard pixel encodings are nonnegative; TightPNG is -260. Negative
    // cursor, desktop-size, last-rectangle and capability pseudo-encodings
    // must never unlock input.
    if (complete && encoding !== null && (encoding >= 0 || encoding === -260)) pixels = true
    return complete
  }
  decoder._framebufferUpdate = function () {
    const complete = frame.call(this)
    if (complete && pixels) {
      pixels = false
      void this._display.flush().then(() => { if (active) painted() })
    }
    return complete
  }
  return () => { active = false; decoder._handleRect = rect; decoder._framebufferUpdate = frame }
}
