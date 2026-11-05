import { ref, watch, type Ref } from 'vue'

// A one-shot modifier works with mobile keyboards, which cannot hold Ctrl.
export function useMobileTerminalCtrl(canInput: Ref<boolean>, queueInput: (data: string) => boolean) {
  const ctrlActive = ref(false)
  let compositionDraft = ''
  watch(canInput, value => { if (!value) ctrlActive.value = false }, { flush: 'sync' })
  function toggleCtrl(event: MouseEvent): void {
    if (!canInput.value) return
    ctrlActive.value = !ctrlActive.value
    if (ctrlActive.value) (event.currentTarget as HTMLElement).closest('section')?.querySelector<HTMLInputElement>('form input')?.focus()
  }
  function captureCtrlInput(event: InputEvent): void {
    if (!ctrlActive.value) return
    if (event.isComposing || event.inputType === 'insertCompositionText') return
    if (event.inputType === 'insertText' && event.cancelable && /^[a-z]$/i.test(event.data ?? '')) {
      event.preventDefault()
      if (queueInput(String.fromCharCode(event.data!.toUpperCase().charCodeAt(0) - 64))) ctrlActive.value = false
    } else {
      ctrlActive.value = false
    }
  }
  function startCtrlComposition(event: CompositionEvent): void {
    if (ctrlActive.value) compositionDraft = (event.target as HTMLInputElement).value
  }
  function finishCtrlComposition(event: CompositionEvent): void {
    if (!ctrlActive.value) return
    if (/^[a-z]$/i.test(event.data)) {
      // Restore the local draft before Vue processes compositionend and its input event.
      ;(event.target as HTMLInputElement).value = compositionDraft
      if (queueInput(String.fromCharCode(event.data.toUpperCase().charCodeAt(0) - 64))) ctrlActive.value = false
    } else {
      ctrlActive.value = false
    }
  }
  return { ctrlActive, toggleCtrl, captureCtrlInput, startCtrlComposition, finishCtrlComposition }
}
