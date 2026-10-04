import type { ComputedRef, InjectionKey, Ref } from 'vue'

/**
 * Window controls of the surrounding workspace dialog. A headerless workspace
 * dialog hands them to content that already owns a toolbar (the file editor),
 * so the window keeps one bar instead of a title bar stacked on a toolbar.
 */
export interface ModalWindowContext {
  fullscreen: Ref<boolean>
  allowFullscreen: ComputedRef<boolean>
  closeDisabled: ComputedRef<boolean>
  toggleFullscreen: () => void
  close: () => void
}

export const modalWindowKey: InjectionKey<ModalWindowContext> = Symbol('modal-window')
