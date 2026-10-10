<script setup lang="ts">
import { computed, inject, nextTick, ref, useId, watch } from 'vue'
import { ChevronDown, ChevronLeft, ChevronRight, ChevronUp, Lock, SquareFunction } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { copyText } from '@/lib/clipboard'
import {
  cellName, columnName, isNumericText, OFFICE_SHEET_COLUMNS, OFFICE_SHEET_ROWS, parseCellReference, type OfficeLocation,
} from '@/lib/officeDocument'
import type { OfficeItem, OfficeSection } from '@/types/api'
import OfficeTextEditor, { type OfficeEditorMove } from './OfficeTextEditor.vue'
import { officeEditingKey } from './officeEditing'

const props = defineProps<{ sections: OfficeSection[]; hint: string }>()
const sectionIndex = defineModel<number>('sectionIndex', { required: true })
const { t } = useI18n()
const editing = inject(officeEditingKey)!
const uid = useId()
const grid = ref<HTMLTableElement>(), tabs = ref<HTMLElement>()
const rowStart = ref(1), columnStart = ref(1)
const active = ref({ row: 1, column: 1 })
const editor = ref<'cell' | 'bar'>()
const nameDraft = ref('A1'), nameFocused = ref(false), nameError = ref(false)
const copyNotice = ref<{ cell: string; ok: boolean }>()

const section = computed(() => props.sections[sectionIndex.value])
const cells = computed(() => new Map(section.value?.items.map(item => [`${item.row}:${item.column}`, item])))
const totalRows = computed(() => Math.max(1, section.value?.rows ?? 1, ...(section.value?.items.map(item => item.row ?? 1) ?? [])))
const totalColumns = computed(() => Math.max(1, section.value?.columns ?? 1, ...(section.value?.items.map(item => item.column ?? 1) ?? [])))
const rows = computed(() => range(rowStart.value, Math.min(totalRows.value, rowStart.value + OFFICE_SHEET_ROWS - 1)))
const columns = computed(() => range(columnStart.value, Math.min(totalColumns.value, columnStart.value + OFFICE_SHEET_COLUMNS - 1)))
const activeItem = computed(() => cells.value.get(`${active.value.row}:${active.value.column}`))
const activeName = computed(() => cellName(active.value.row, active.value.column))
const editingCell = computed(() => editor.value === 'cell' && Boolean(activeItem.value?.id) && editing.editingId.value === activeItem.value?.id)
const barValue = computed(() => {
  const item = activeItem.value
  if (!item) return ''
  return item.formula && !editing.isModified(item) ? item.formula : editing.value(item)
})
const sheetHint = computed(() => {
  const item = activeItem.value
  if (copyNotice.value) return copyNotice.value.ok ? t('office.cellCopied', { cell: copyNotice.value.cell }) : t('office.copyFailed')
  if (nameError.value) return t('office.invalidCell')
  if (!item) return t('office.emptyCell')
  if (item.formula) return t('office.formulaReadonly')
  if (!item.editable) return t('office.readonly')
  if (editor.value && editing.value(item).trimStart().startsWith('=')) return t('office.literalFormula')
  if (editor.value) return t('office.hint.editingCell')
  return props.hint
})
const rangeText = computed(() => t('office.sheetRange', {
  rows: `${rows.value[0]}–${rows.value.at(-1)}`, totalRows: totalRows.value.toLocaleString(),
  columns: `${columnName(columns.value[0]!)}–${columnName(columns.value.at(-1)!)}`, totalColumns: columnName(totalColumns.value),
}))

function range(from: number, to: number) { return Array.from({ length: Math.max(1, to - from + 1) }, (_, i) => from + i) }
function cellID(row: number, column: number) { return `${uid}-r${row}c${column}` }
function cellAt(row: number, column: number): OfficeItem | undefined { return cells.value.get(`${row}:${column}`) }

watch(sectionIndex, () => {
  editor.value = undefined
  rowStart.value = 1; columnStart.value = 1; active.value = { row: 1, column: 1 }
  editing.select(activeItem.value)
})
watch(activeName, name => { if (!nameFocused.value) nameDraft.value = name }, { immediate: true })
watch(() => editing.editingId.value, id => { if (!id) editor.value = undefined })

function scrollActive() {
  void nextTick(() => document.getElementById(cellID(active.value.row, active.value.column))?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' }))
}
function moveTo(row: number, column: number) {
  const r = Math.max(1, Math.min(totalRows.value, row)), c = Math.max(1, Math.min(totalColumns.value, column))
  active.value = { row: r, column: c }
  nameError.value = false; copyNotice.value = undefined
  if (r < rowStart.value) rowStart.value = r
  else if (r >= rowStart.value + OFFICE_SHEET_ROWS) rowStart.value = r - OFFICE_SHEET_ROWS + 1
  if (c < columnStart.value) columnStart.value = c
  else if (c >= columnStart.value + OFFICE_SHEET_COLUMNS) columnStart.value = c - OFFICE_SHEET_COLUMNS + 1
  editing.select(activeItem.value)
  scrollActive()
}
function move(rowDelta: number, columnDelta: number) { moveTo(active.value.row + rowDelta, active.value.column + columnDelta) }
/** Shifts the visible window and keeps the active cell at the same position inside it. */
function page(rowDelta: number, columnDelta: number) {
  const nextRow = Math.max(1, Math.min(totalRows.value - OFFICE_SHEET_ROWS + 1, rowStart.value + rowDelta))
  const nextColumn = Math.max(1, Math.min(totalColumns.value - OFFICE_SHEET_COLUMNS + 1, columnStart.value + columnDelta))
  const rowShift = nextRow - rowStart.value, columnShift = nextColumn - columnStart.value
  rowStart.value = nextRow; columnStart.value = nextColumn
  moveTo(active.value.row + rowShift, active.value.column + columnShift)
}
function focusGrid() { grid.value?.focus({ preventScroll: true }) }
function startCellEdit(replace?: string) {
  const item = activeItem.value
  if (!item || !editing.startEdit(item)) return
  editor.value = 'cell'
  if (replace !== undefined) editing.update(item, replace)
}
function finishCellEdit(move?: OfficeEditorMove, keyboard = true) {
  editing.commit()
  editor.value = undefined
  if (move === 'down') moveTo(active.value.row + 1, active.value.column)
  else if (move === 'up') moveTo(active.value.row - 1, active.value.column)
  else if (move === 'right') moveTo(active.value.row, active.value.column + 1)
  else if (move === 'left') moveTo(active.value.row, active.value.column - 1)
  if (keyboard) void nextTick(focusGrid)
}
function cancelCellEdit() {
  editing.cancel()
  editor.value = undefined
  void nextTick(focusGrid)
}
function selectCell(row: number, column: number) {
  if (editor.value) finishCellEdit(undefined, false)
  moveTo(row, column)
  focusGrid()
}
function gridKeydown(event: KeyboardEvent) {
  if (event.target !== grid.value || event.isComposing) return
  const item = activeItem.value, editable = editing.canEdit(item), command = event.ctrlKey || event.metaKey
  const handled = () => event.preventDefault()
  switch (event.key) {
    case 'ArrowUp': handled(); return command ? moveTo(1, active.value.column) : move(-1, 0)
    case 'ArrowDown': handled(); return command ? moveTo(totalRows.value, active.value.column) : move(1, 0)
    case 'ArrowLeft': handled(); return command ? moveTo(active.value.row, 1) : move(0, -1)
    case 'ArrowRight': handled(); return command ? moveTo(active.value.row, totalColumns.value) : move(0, 1)
    case 'PageUp': handled(); return move(-OFFICE_SHEET_ROWS, 0)
    case 'PageDown': handled(); return move(OFFICE_SHEET_ROWS, 0)
    case 'Home': handled(); return moveTo(command ? 1 : active.value.row, 1)
    case 'End': handled(); return moveTo(command ? totalRows.value : active.value.row, totalColumns.value)
    case 'Enter': case 'F2': handled(); return editable ? startCellEdit() : undefined
    case 'Delete': if (editable && item) { handled(); editing.update(item, '') } return
    case 'Backspace': if (editable) { handled(); startCellEdit('') } return
  }
  if (command && !event.altKey && event.key.toLowerCase() === 'c' && item) { handled(); void copyCell(item); return }
  if (editable && event.key.length === 1 && !command && !event.altKey) { handled(); startCellEdit(event.key) }
}

async function copyCell(item: OfficeItem) {
  const cell = activeName.value
  copyNotice.value = { cell, ok: await copyText(editing.value(item)) }
}
function barFocus() {
  const item = activeItem.value
  if (item && editing.canEdit(item) && editing.editingId.value !== item.id && editing.startEdit(item)) editor.value = 'bar'
}
function barInput(event: Event) {
  const item = activeItem.value, element = event.target as HTMLTextAreaElement
  if (item && editor.value === 'bar') editing.update(item, element.value)
}
function barKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  if (event.key === 'Escape') {
    event.preventDefault(); event.stopPropagation()
    if (editor.value === 'bar') editing.cancel()
    editor.value = undefined; focusGrid()
  } else if (editor.value !== 'bar') {
    return
  } else if (event.key === 'Enter' && event.altKey) {
    event.preventDefault()
    const element = event.target as HTMLTextAreaElement
    element.setRangeText('\n', element.selectionStart, element.selectionEnd, 'end')
    barInput(event)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    editing.commit(); editor.value = undefined
    move(event.shiftKey ? -1 : 1, 0); focusGrid()
  }
}
function barBlur() { if (editor.value === 'bar') { editing.commit(); editor.value = undefined } }

function nameKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault(); event.stopPropagation()
    nameDraft.value = activeName.value; nameError.value = false; focusGrid()
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const target = parseCellReference(nameDraft.value)
    if (!target || target.row > totalRows.value || target.column > totalColumns.value) { nameError.value = true; return }
    moveTo(target.row, target.column); focusGrid()
  }
}
function nameBlur() { nameFocused.value = false; nameDraft.value = activeName.value }

function tabKeydown(event: KeyboardEvent, index: number) {
  const targets: Record<string, number> = { ArrowRight: index + 1, ArrowLeft: index - 1, Home: 0, End: props.sections.length - 1 }
  const next = targets[event.key]
  if (next === undefined) return
  event.preventDefault()
  sectionIndex.value = (next + props.sections.length) % props.sections.length
  void nextTick(() => tabs.value?.querySelector<HTMLElement>('[aria-selected="true"]')?.focus())
}

async function reveal(location: OfficeLocation) {
  await nextTick()
  moveTo(location.item.row ?? 1, location.item.column ?? 1)
  focusGrid()
}
defineExpose({ reveal })
</script>

<template>
  <div class="office-view office-view--xlsx">
    <div class="office-formula-bar">
      <input
        v-model="nameDraft"
        class="office-name-box"
        :class="{ 'is-invalid': nameError }"
        :aria-label="t('office.nameBox')"
        :aria-invalid="nameError || undefined"
        spellcheck="false"
        autocomplete="off"
        data-i18n-ignore
        @focus="nameFocused = true; ($event.target as HTMLInputElement).select()"
        @blur="nameBlur"
        @keydown="nameKeydown"
      />
      <span class="office-fx" aria-hidden="true"><SquareFunction :size="18" /></span>
      <textarea
        class="office-formula-input"
        rows="1"
        spellcheck="false"
        maxlength="65536"
        :value="barValue"
        :aria-label="t('office.formulaBar', { cell: activeName })"
        :placeholder="activeItem ? '' : t('office.emptyCellPlaceholder')"
        :readonly="!editing.canEdit(activeItem)"
        data-i18n-ignore
        @focus="barFocus"
        @input="barInput"
        @keydown="barKeydown"
        @blur="barBlur"
      />
      <span v-if="activeItem && !editing.canEdit(activeItem)" class="office-chip">
        <Lock :size="14" aria-hidden="true" />{{ activeItem.formula ? t('office.formula') : t('office.readonlyShort') }}
      </span>
    </div>
    <div class="office-grid-scroll">
      <table
        ref="grid"
        class="office-grid"
        role="grid"
        tabindex="0"
        :aria-label="t('office.sheetGrid', { sheet: section?.name || sectionIndex + 1 })"
        :aria-rowcount="totalRows + 1"
        :aria-colcount="totalColumns + 1"
        :aria-activedescendant="cellID(active.row, active.column)"
        @keydown="gridKeydown"
      >
        <thead>
          <tr role="row" aria-rowindex="1">
            <th class="office-grid__corner" aria-hidden="true" />
            <th
              v-for="column in columns"
              :key="column"
              role="columnheader"
              :aria-colindex="column + 1"
              :class="{ 'is-active': column === active.column }"
              data-i18n-ignore
            >{{ columnName(column) }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row" role="row" :aria-rowindex="row + 1">
            <th role="rowheader" :aria-colindex="1" :class="{ 'is-active': row === active.row }" data-i18n-ignore>{{ row }}</th>
            <td
              v-for="column in columns"
              :id="cellID(row, column)"
              :key="column"
              role="gridcell"
              :aria-colindex="column + 1"
              :aria-selected="row === active.row && column === active.column"
              :aria-readonly="!editing.canEdit(cellAt(row, column)) || undefined"
              :data-office-id="cellAt(row, column)?.id"
              :class="{
                'is-active': row === active.row && column === active.column,
                'is-number': isNumericText(cellAt(row, column) ? editing.value(cellAt(row, column)!) : ''),
                'is-modified': editing.isModified(cellAt(row, column)),
                'has-formula': Boolean(cellAt(row, column)?.formula),
              }"
              @mousedown.prevent="selectCell(row, column)"
              @dblclick="startCellEdit()"
            >
              <OfficeTextEditor
                v-if="editingCell && row === active.row && column === active.column"
                class="office-cell-editor"
                multiline
                :model-value="editing.value(activeItem!)"
                :label="t('office.formulaBar', { cell: activeName })"
                @update:model-value="editing.update(activeItem!, $event)"
                @commit="finishCellEdit"
                @cancel="cancelCellEdit"
                @mousedown.stop
              />
              <span v-else-if="cellAt(row, column)" class="office-cell__text" data-i18n-ignore>{{ editing.value(cellAt(row, column)!) }}</span>
              <span v-if="editing.isModified(cellAt(row, column))" class="sr-only">{{ t('office.modified') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div class="office-sheetbar">
      <div ref="tabs" class="office-sheet-tabs" role="tablist" :aria-label="t('office.sheets')">
        <button
          v-for="(sheet, index) in sections"
          :key="index"
          type="button"
          role="tab"
          class="office-sheet-tab"
          :aria-selected="index === sectionIndex"
          :tabindex="index === sectionIndex ? 0 : -1"
          @click="sectionIndex = index"
          @keydown="tabKeydown($event, index)"
        ><span data-i18n-ignore>{{ sheet.name || index + 1 }}</span></button>
      </div>
      <div class="office-sheet-nav">
        <span class="office-sheet-nav__range">{{ rangeText }}</span>
        <button type="button" class="office-icon-button" :aria-label="t('office.previousRows')" :title="t('office.previousRows')" :disabled="rowStart === 1" @click="page(-OFFICE_SHEET_ROWS, 0)"><ChevronUp :size="18" aria-hidden="true" /></button>
        <button type="button" class="office-icon-button" :aria-label="t('office.nextRows')" :title="t('office.nextRows')" :disabled="rowStart + OFFICE_SHEET_ROWS > totalRows" @click="page(OFFICE_SHEET_ROWS, 0)"><ChevronDown :size="18" aria-hidden="true" /></button>
        <button type="button" class="office-icon-button" :aria-label="t('office.previousColumns')" :title="t('office.previousColumns')" :disabled="columnStart === 1" @click="page(0, -OFFICE_SHEET_COLUMNS)"><ChevronLeft :size="18" aria-hidden="true" /></button>
        <button type="button" class="office-icon-button" :aria-label="t('office.nextColumns')" :title="t('office.nextColumns')" :disabled="columnStart + OFFICE_SHEET_COLUMNS > totalColumns" @click="page(0, OFFICE_SHEET_COLUMNS)"><ChevronRight :size="18" aria-hidden="true" /></button>
      </div>
    </div>
    <footer class="office-statusbar">
      <span class="office-statusbar__hint" aria-live="polite">{{ sheetHint }}</span>
    </footer>
  </div>
</template>
