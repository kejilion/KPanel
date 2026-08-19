import { computed, onBeforeUnmount, onMounted, readonly, ref } from 'vue'

/**
 * Browser/OS fullscreen state for the whole KPanel document.
 *
 * Requests can be rejected when the browser does not support arbitrary-element
 * fullscreen, when user activation has expired, or when a policy blocks it.
 * Callers always receive a boolean result so desktop mode can keep working as
 * a normal viewport-filling interface in those environments.
 */
export function useDocumentFullscreen() {
  const active = ref(false)

  function sync(): void {
    active.value = typeof document !== 'undefined' && Boolean(document.fullscreenElement)
  }

  const supported = computed(() => (
    typeof document !== 'undefined'
    && document.fullscreenEnabled
    && typeof document.documentElement.requestFullscreen === 'function'
  ))

  async function enter(): Promise<boolean> {
    if (typeof document === 'undefined') return false
    if (document.fullscreenElement) {
      sync()
      return true
    }
    if (!supported.value) return false
    try {
      await document.documentElement.requestFullscreen()
    } catch {
      sync()
      return false
    }
    sync()
    return active.value
  }

  async function exit(): Promise<boolean> {
    if (typeof document === 'undefined') return false
    if (!document.fullscreenElement) {
      sync()
      return true
    }
    if (typeof document.exitFullscreen !== 'function') return false
    try {
      await document.exitFullscreen()
    } catch {
      sync()
      return false
    }
    sync()
    return !active.value
  }

  function toggle(): Promise<boolean> {
    return typeof document !== 'undefined' && document.fullscreenElement ? exit() : enter()
  }

  onMounted(() => {
    sync()
    document.addEventListener('fullscreenchange', sync)
  })

  onBeforeUnmount(() => {
    document.removeEventListener('fullscreenchange', sync)
  })

  return {
    active: readonly(active),
    supported,
    enter,
    exit,
    toggle,
  }
}
