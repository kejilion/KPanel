// Continuous input is lossy in position, but not in wheel distance. Keep at
// most one sample of each and leave discrete actions (keys/buttons) ordered.
type Command = Record<string, unknown>
export function browserInputQueue(send: (command: Command) => boolean) {
  let move: Command | undefined, wheel: Command | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  function clear() { clearTimeout(timer); timer = undefined; move = wheel = undefined }
  function flush() {
    clearTimeout(timer); timer = undefined
    const m = move, w = wheel; move = wheel = undefined
    if (m) send(m)
    if (w) send(w)
  }
  function push(command: Command) {
    if (command.type === 'mouse' && command.action === 'mouseMoved') move = command
    else if (command.type === 'mouse' && command.action === 'mouseWheel') {
      if (wheel && (wheel.tab !== command.tab || wheel.modifiers !== command.modifiers)) flush()
      wheel = { ...command, deltaX: Math.max(-3000, Math.min(3000, Number(wheel?.deltaX || 0) + Number(command.deltaX || 0))), deltaY: Math.max(-3000, Math.min(3000, Number(wheel?.deltaY || 0) + Number(command.deltaY || 0))) }
    } else { flush(); return send(command) }
    // Two continuous message types at most every 25ms, leaving room under the
    // protocol budget for keys, clicks and lifecycle commands.
    timer ??= setTimeout(flush, 25)
    return true
  }
  return { push, flush, clear }
}
