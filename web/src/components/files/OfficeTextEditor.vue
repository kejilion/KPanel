<script setup lang="ts">
import { nextTick, onMounted, ref } from 'vue'
import { singleLineText } from '@/lib/officeDocument'

export type OfficeEditorMove = 'up' | 'down' | 'left' | 'right'

const props = withDefaults(defineProps<{
  modelValue: string
  label: string
  /** Spreadsheet cells keep line breaks (Alt+Enter); Word and slide text stay single-line. */
  multiline?: boolean
  /** Caret offset to restore after switching from display to edit mode; defaults to the end. */
  caret?: number
  readonly?: boolean
}>(), { multiline: false, caret: undefined, readonly: false })
const emit = defineEmits<{ 'update:modelValue': [string]; commit: [move: OfficeEditorMove | undefined, keyboard: boolean]; cancel: [] }>()
const field = ref<HTMLTextAreaElement>()
let finished = false

function resize() {
  const element = field.value
  if (!element) return
  element.style.height = 'auto'
  element.style.height = `${element.scrollHeight}px`
}
function input(event: Event) {
  const element = event.target as HTMLTextAreaElement
  if (!props.multiline && /[\r\n\t]/.test(element.value)) {
    const caret = singleLineText(element.value.slice(0, element.selectionStart)).length
    element.value = singleLineText(element.value)
    element.setSelectionRange(caret, caret)
  }
  emit('update:modelValue', element.value)
  resize()
}
function finish(kind: 'commit' | 'cancel', move?: OfficeEditorMove, keyboard = true) {
  if (finished) return
  finished = true
  if (kind === 'cancel') emit('cancel')
  else emit('commit', move, keyboard)
}
function insertLineBreak(element: HTMLTextAreaElement) {
  const start = element.selectionStart, end = element.selectionEnd
  element.setRangeText('\n', start, end, 'end')
  emit('update:modelValue', element.value)
  resize()
}
function keydown(event: KeyboardEvent) {
  // Enter confirms an IME candidate while composing; it must not finish the edit.
  if (event.isComposing || event.keyCode === 229) return
  const element = event.target as HTMLTextAreaElement
  if (event.key === 'Escape') {
    event.preventDefault(); event.stopPropagation(); finish('cancel')
  } else if (event.key === 'Enter') {
    event.preventDefault(); event.stopPropagation()
    if (props.multiline && event.altKey) insertLineBreak(element)
    else finish('commit', props.multiline ? (event.shiftKey ? 'up' : 'down') : undefined)
  } else if (event.key === 'Tab' && props.multiline) {
    event.preventDefault(); event.stopPropagation(); finish('commit', event.shiftKey ? 'left' : 'right')
  }
}
onMounted(async () => {
  await nextTick()
  const element = field.value
  if (!element) return
  resize()
  element.focus({ preventScroll: true })
  const caret = Math.max(0, Math.min(element.value.length, props.caret ?? element.value.length))
  element.setSelectionRange(caret, caret)
})
</script>

<template>
  <textarea
    ref="field"
    class="office-text-editor"
    rows="1"
    spellcheck="false"
    maxlength="65536"
    :value="modelValue"
    :aria-label="label"
    :readonly="readonly"
    data-i18n-ignore
    @input="input"
    @keydown="keydown"
    @blur="finish('commit', undefined, false)"
  />
</template>
