import type { Terminal } from '@xterm/xterm'

/** Use one native rail for arrows and dragging, synchronized with xterm's buffer. */
export function installTerminalScrollbar(host: HTMLElement, terminal: Terminal): () => void {
  const viewport = host.querySelector<HTMLElement>('.xterm-viewport')
  const screen = host.querySelector<HTMLElement>('.xterm-screen')
  if (!viewport || !screen) return () => {}

  const spacer = document.createElement('div')
  spacer.setAttribute('aria-hidden', 'true')
  spacer.style.width = '1px'
  viewport.appendChild(spacer)
  let lineHeight = 1
  let disposed = false
  let syncing = false
  let frame: number | undefined
  let syncedTop = viewport.scrollTop
  let syncedBase = 0
  let syncedHeight = 0
  let syncedBuffer = terminal.buffer.active
  const sync = () => {
    if (disposed || syncing) return
    syncing = true
    const buffer = terminal.buffer.active
    const nextLineHeight = Math.max(1, screen.clientHeight / Math.max(1, terminal.rows))
    // A native scroll can precede its event. Consume that intent before output
    // overwrites the rail, using the range the user actually scrolled through.
    const pendingScroll = viewport.scrollTop !== syncedTop && buffer === syncedBuffer
      && viewport.clientHeight === syncedHeight && nextLineHeight === lineHeight
    const line = Math.max(0, Math.round(viewport.scrollTop / lineHeight))
    const target = syncedBase - line <= 2 ? buffer.baseY : Math.min(buffer.baseY, line)
    lineHeight = nextLineHeight
    spacer.style.height = `${viewport.clientHeight + buffer.baseY * lineHeight}px`
    if (pendingScroll && target !== buffer.viewportY) terminal.scrollToLine(target)
    viewport.scrollTop = buffer.viewportY * lineHeight
    syncedTop = viewport.scrollTop
    syncedBase = buffer.baseY
    syncedHeight = viewport.clientHeight
    syncedBuffer = buffer
    syncing = false
  }
  const scroll = () => sync()
  const scheduleSync = () => {
    if (disposed || frame !== undefined) return
    frame = requestAnimationFrame(() => { frame = undefined; sync() })
  }
  viewport.addEventListener('scroll', scroll)
  // xterm emits onScroll before updating its own scroll range. Wait until that
  // update completes before translating a native scroll back into buffer rows.
  const subscriptions = [terminal.onScroll(scheduleSync), terminal.onWriteParsed(scheduleSync), terminal.onResize(scheduleSync)]
  const observer = new ResizeObserver(scheduleSync)
  observer.observe(viewport)
  observer.observe(screen)
  sync()
  return () => {
    disposed = true
    if (frame !== undefined) cancelAnimationFrame(frame)
    viewport.removeEventListener('scroll', scroll)
    subscriptions.forEach(subscription => subscription.dispose())
    observer.disconnect()
    spacer.remove()
  }
}
