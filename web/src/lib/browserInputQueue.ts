// Continuous input is lossy in position, but not in wheel distance. Keep at
// most one sample of each. A bounded residual spans at most eight packets;
// discrete actions end the gesture and discard any obsolete remaining motion.
type Command = Record<string, unknown>
const inputBatchDelayMs = 8
export function browserInputQueue(send: (command: Command) => boolean) {
  let move: Command | undefined, wheel: Command | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  function clear() { clearTimeout(timer); timer = undefined; move = wheel = undefined }
  function flush() {
    clearTimeout(timer); timer = undefined
    const m = move, w = wheel; move = wheel = undefined
    if (m) send(m)
    if (w) {
      const x = Number(w.deltaX || 0), y = Number(w.deltaY || 0)
      const dx = Math.max(-3000, Math.min(3000, x)), dy = Math.max(-3000, Math.min(3000, y))
      if (send({ ...w, deltaX: dx, deltaY: dy }) && (x !== dx || y !== dy)) {
        wheel = { ...w, deltaX: x - dx, deltaY: y - dy }
        timer = setTimeout(flush, 25)
      }
    }
  }
  function push(command: Command) {
    if (command.type === 'mouse' && command.action === 'mouseMoved') move = command
    else if (command.type === 'mouse' && command.action === 'mouseWheel') {
      if (wheel && (wheel.tab !== command.tab || wheel.modifiers !== command.modifiers)) { flush(); clear() }
      wheel = { ...command, deltaX: Math.max(-24000, Math.min(24000, Number(wheel?.deltaX || 0) + Number(command.deltaX || 0))), deltaY: Math.max(-24000, Math.min(24000, Number(wheel?.deltaY || 0) + Number(command.deltaY || 0))) }
    } else { flush(); clear(); return send(command) }
    // Two continuous message types at most every 8ms, leaving room under the
    // protocol budget for keys, clicks and lifecycle commands.
    timer ??= setTimeout(flush, inputBatchDelayMs)
    return true
  }
  return { push, flush, clear }
}
