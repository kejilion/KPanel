<script setup lang="ts">
import { computed, inject, nextTick, ref } from 'vue'
import { useI18n } from '@/i18n'
import type { OfficeItem } from '@/types/api'
import OfficeTextEditor, { type OfficeEditorMove } from './OfficeTextEditor.vue'
import { officeEditingKey } from './officeEditing'

const props = defineProps<{ item: OfficeItem; label: string }>()
const { t } = useI18n()
const editing = inject(officeEditingKey)!
const button = ref<HTMLButtonElement>()
const caret = ref<number>()
const active = computed(() => Boolean(props.item.id) && editing.editingId.value === props.item.id)
const text = computed(() => editing.value(props.item))

type CaretDocument = Document & { caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null }
/** Keeps the caret where the user clicked when the text turns into an editor. */
function caretFromPoint(event: MouseEvent): number | undefined {
  if (event.detail === 0) return undefined
  const doc = document as CaretDocument
  const position = doc.caretPositionFromPoint?.(event.clientX, event.clientY)
  const range = position ? undefined : doc.caretRangeFromPoint?.(event.clientX, event.clientY)
  const node = position?.offsetNode ?? range?.startContainer
  const target = button.value?.querySelector('[data-office-text]')?.firstChild
  return node && node === target ? (position?.offset ?? range?.startOffset) : undefined
}
function activate(event: MouseEvent) {
  caret.value = caretFromPoint(event)
  editing.startEdit(props.item)
}
function edit() {
  caret.value = undefined
  editing.startEdit(props.item)
}
function commit(_move: OfficeEditorMove | undefined, keyboard: boolean) {
  editing.commit()
  if (keyboard) void nextTick(() => button.value?.focus({ preventScroll: true }))
}
function cancel() {
  editing.cancel()
  void nextTick(() => button.value?.focus({ preventScroll: true }))
}
</script>

<template>
  <div
    class="office-block"
    :class="{
      'is-selected': item.id && editing.selectedId.value === item.id,
      'is-modified': editing.isModified(item),
      'is-editing': active,
      'is-readonly': !item.editable,
    }"
    :data-office-id="item.id"
  >
    <OfficeTextEditor
      v-if="active"
      :model-value="text"
      :label="label"
      :caret="caret"
      @update:model-value="editing.update(item, $event)"
      @commit="commit"
      @cancel="cancel"
    />
    <button
      v-else
      ref="button"
      type="button"
      class="office-block__text"
      @click="activate"
      @keydown.f2.prevent="edit"
    ><span data-office-text data-i18n-ignore>{{ text || ' ' }}</span><span v-if="!item.editable" class="sr-only">{{ t('office.readonlyShort') }}</span
    ><span v-if="editing.isModified(item)" class="sr-only">{{ t('office.modified') }}</span></button>
  </div>
</template>
