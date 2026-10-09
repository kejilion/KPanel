import type { InjectionKey, Ref } from 'vue'
import type { OfficeItem } from '@/types/api'

/** Draft editing state shared by the Word, Excel and PowerPoint views of one workspace. */
export interface OfficeEditing {
  selectedId: Readonly<Ref<string | undefined>>
  editingId: Readonly<Ref<string | undefined>>
  value(item: OfficeItem): string
  isModified(item?: OfficeItem): boolean
  canEdit(item?: OfficeItem): boolean
  select(item?: OfficeItem): void
  /** Enters edit mode; returns false and only selects read-only targets. */
  startEdit(item: OfficeItem): boolean
  update(item: OfficeItem, text: string): void
  /** Keeps the current draft and leaves edit mode. */
  commit(): void
  /** Restores the text from before this edit session and leaves edit mode. */
  cancel(): void
}

export const officeEditingKey: InjectionKey<OfficeEditing> = Symbol('office-editing')

/** Moves focus to the rendered target of a draft without scrolling the dialog chrome. */
export function focusOfficeTarget(root: HTMLElement | undefined, id: string, selector = 'button'): boolean {
  const target = root?.querySelector<HTMLElement>(`[data-office-id="${id.replace(/["\\]/g, '\\$&')}"]`)
  if (!target) return false
  target.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
  const focusable = target.matches(selector) ? target : target.querySelector<HTMLElement>(selector) ?? target
  focusable.focus({ preventScroll: true })
  return true
}
