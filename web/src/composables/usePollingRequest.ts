import { onBeforeUnmount, ref } from 'vue'

interface PollingRequestOptions<T> {
  enabled: () => boolean
  intervalMs: number
  request: (signal: AbortSignal) => Promise<T>
  apply: (value: T) => void
  shouldPoll: () => boolean
  onStart: () => void
  onError: (reason: unknown) => void
}

/** Serial reads with delayed recovery, scoped to the current dialog lifecycle. */
export function usePollingRequest<T>(options: PollingRequestOptions<T>) {
  const loading = ref(false)
  const refreshing = ref(false)
  let timer: number | undefined
  let controller: AbortController | undefined
  let inFlight: Promise<void> | undefined
  let generation = 0
  let loaded = false
  let disposed = false

  function clearTimer(): void {
    if (timer !== undefined) window.clearTimeout(timer)
    timer = undefined
  }

  function current(run: number): boolean {
    return !disposed && run === generation && options.enabled()
  }

  function stop(): void {
    generation += 1
    clearTimer()
    controller?.abort()
    loading.value = false
    refreshing.value = false
  }

  function load(silent = false): Promise<void> {
    if (disposed || !options.enabled()) return Promise.resolve()
    const run = ++generation
    clearTimer()
    controller?.abort()
    if (silent && loaded) refreshing.value = true
    else loading.value = true
    options.onStart()
    if (inFlight) {
      // Even a reader that settles late after abort must finish before reopening.
      return inFlight.then(() => { if (current(run)) return load(silent) })
    }

    const requestController = new AbortController()
    controller = requestController
    // Defer execution so synchronous throws also settle the registered request.
    inFlight = Promise.resolve().then(async () => {
      let retry = false
      try {
        if (!current(run)) return
        const value = await options.request(requestController.signal)
        if (!current(run) || requestController.signal.aborted) return
        options.apply(value)
        loaded = true
      } catch (reason) {
        if (!current(run) || requestController.signal.aborted) return
        retry = true
        if (reason instanceof DOMException && reason.name === 'AbortError') return
        options.onError(reason)
      } finally {
        if (controller === requestController) controller = undefined
        inFlight = undefined
        if (current(run) && !requestController.signal.aborted) {
          loading.value = false
          refreshing.value = false
          if (retry || options.shouldPoll()) {
            timer = window.setTimeout(() => {
              timer = undefined
              if (current(run)) void load(true)
            }, options.intervalMs)
          }
        }
      }
    })
    return inFlight
  }

  onBeforeUnmount(() => { disposed = true; stop() })
  return { load, stop, loading, refreshing }
}
