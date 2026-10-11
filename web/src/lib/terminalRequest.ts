// Bound connection setup even when an adapter does not settle after abort.
// This helper must not wrap terminal creation or ambiguous legacy input writes.
export function terminalRequest<T>(
  controller: AbortController,
  request: (signal: AbortSignal) => Promise<T>,
  timeoutMs = 5000,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const { signal } = controller
    let timer: ReturnType<typeof setTimeout> | undefined
    const cleanup = () => {
      if (timer) clearTimeout(timer)
      signal.removeEventListener('abort', abort)
    }
    const abort = () => {
      cleanup()
      reject(new DOMException('Terminal connection setup aborted', 'AbortError'))
    }
    if (signal.aborted) { abort(); return }
    signal.addEventListener('abort', abort, { once: true })
    timer = setTimeout(() => controller.abort(), timeoutMs)
    try {
      request(signal).then(
        value => { cleanup(); resolve(value) },
        error => { cleanup(); reject(error) },
      )
    } catch (error) { cleanup(); reject(error) }
  })
}
