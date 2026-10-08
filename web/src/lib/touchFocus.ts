// 触屏设备上程序化聚焦文本输入会立即弹出软键盘并打断浏览流程，
// 这里统一判断：触屏主指针不自动聚焦输入框，由用户主动点按再唤起键盘。
export function isTouchPrimary(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false
  return window.matchMedia('(hover: none) and (pointer: coarse)').matches
}

export function isTextEntry(element: Element | null | undefined): boolean {
  if (!element) return false
  if (element instanceof HTMLTextAreaElement) return true
  if (element instanceof HTMLInputElement) {
    return !['button', 'submit', 'reset', 'checkbox', 'radio', 'file', 'range', 'color', 'image', 'hidden'].includes(element.type)
  }
  return element instanceof HTMLElement && element.isContentEditable === true
}

// 仅在非触屏设备上聚焦；返回是否实际聚焦。
export function focusTextInput(element: HTMLElement | null | undefined, options?: FocusOptions & { select?: boolean }): boolean {
  if (!element || isTouchPrimary()) return false
  element.focus(options)
  if (options?.select && (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement)) element.select()
  return true
}
