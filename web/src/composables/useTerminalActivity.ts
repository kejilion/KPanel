import { computed, inject, onBeforeUnmount, onMounted, ref } from 'vue'
import type { ComputedRef, Ref } from 'vue'
import { desktopWindowActiveKey, desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'

export interface TerminalActivity {
  /** The hosting window is focused: it takes keyboard focus and PTY geometry. */
  focused: Readonly<Ref<boolean>>
  /** The terminal is on screen, so its output keeps streaming, focused or not. */
  streaming: ComputedRef<boolean>
}

/**
 * Terminals in unfocused but visible desktop windows keep streaming so several
 * scripts can run side by side. Output pauses, keeping its offset, only while
 * the window is minimized or closing or the browser tab is hidden.
 */
export function useTerminalActivity(): TerminalActivity {
  const focused = inject(desktopWindowActiveKey, computed(() => true))
  const windowVisible = inject(desktopWindowVisibleKey, computed(() => true))
  const documentVisible = ref(typeof document === 'undefined' || document.visibilityState !== 'hidden')
  const updateDocumentVisibility = () => {
    documentVisible.value = document.visibilityState !== 'hidden'
  }
  onMounted(() => {
    updateDocumentVisibility()
    document.addEventListener('visibilitychange', updateDocumentVisibility)
  })
  onBeforeUnmount(() => document.removeEventListener('visibilitychange', updateDocumentVisibility))
  return { focused, streaming: computed(() => windowVisible.value && documentVisible.value) }
}
