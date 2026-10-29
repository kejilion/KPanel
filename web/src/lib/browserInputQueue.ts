// Continuous input is lossy in position. Keep at most one sample of each;
// retry wheel distance while connected, with a bounded residual of eight
// packets. Discrete actions end the gesture and discard obsolete motion.
type Command = Record<string, unknown>
// The service accepts at most 120 commands per second. With both continuous
// samples present, a 20ms window caps this queue at 100 commands per second
// and leaves room for discrete input.
const inputBatchDelayMs = 20
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
      if (!send({ ...w, deltaX: dx, deltaY: dy })) {
        wheel = w
        timer = setTimeout(flush, 25)
      } else if (x !== dx || y !== dy) {
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
    // Two continuous message types at most every 20ms, leaving room under the
    // protocol budget for keys, clicks and lifecycle commands.
    timer ??= setTimeout(flush, inputBatchDelayMs)
    return true
  }
  return { push, flush, clear }
}
