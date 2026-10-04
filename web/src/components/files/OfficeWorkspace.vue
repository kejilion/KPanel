<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import { ChevronLeft, ChevronRight, FileText, RefreshCw, Save } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { ApiError } from '@/lib/api'
import { fileAPIForHost } from '@/lib/fileHostContext'
import type { FileEntry, OfficeDocument, OfficeItem } from '@/types/api'

const props = defineProps<{ entry: FileEntry; hostId: string }>()
const emit = defineEmits<{ dirty: [boolean]; saving: [boolean]; saved: [FileEntry] }>()
const { t } = useI18n()
const inputID = useId()
const doc = ref<OfficeDocument>()
const loading = ref(false), saving = ref(false), error = ref(''), saved = ref(false), needsReload = ref(false)
const sectionIndex = ref(0), rowStart = ref(1), columnStart = ref(1), page = ref(0)
const selected = ref<OfficeItem>()
const drafts = ref<Record<string, string>>({})
const dirty = computed(() => Object.keys(drafts.value).length > 0)
const section = computed(() => doc.value?.sections[sectionIndex.value])
const pageItems = computed(() => section.value?.items.slice(page.value * 80, (page.value + 1) * 80) ?? [])
const cells = computed(() => new Map(section.value?.items.map(item => [`${item.row}:${item.column}`, item])))
const rows = computed(() => Array.from({ length: Math.min(40, Math.max(1, (section.value?.rows ?? 1) - rowStart.value + 1)) }, (_, i) => rowStart.value + i))
const columns = computed(() => Array.from({ length: Math.min(12, Math.max(1, (section.value?.columns ?? 1) - columnStart.value + 1)) }, (_, i) => columnStart.value + i))
let controller: AbortController | undefined, generation = 0
watch(dirty, value => emit('dirty', value))
watch(sectionIndex, () => { rowStart.value = 1; columnStart.value = 1; page.value = 0; selected.value = undefined })

function failure(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'file_conflict') return t('office.conflict')
    if (err.code === 'office_unsupported') return t('office.unsupported')
    if (err.code === 'file_too_large') return t('office.tooLarge')
    if (err.code === 'office_edit_invalid') return t('office.invalidEdit')
  }
  return t('office.failed')
}
async function load() {
  const own = ++generation
  controller?.abort(); controller = new AbortController()
  const files = fileAPIForHost(props.hostId), path = props.entry.path
  doc.value = undefined; selected.value = undefined; loading.value = true; error.value = ''; saved.value = false
  drafts.value = {}; sectionIndex.value = 0; needsReload.value = false
  try {
    const result = await files.office(path, controller.signal)
    if (own === generation) doc.value = result
  } catch (err) { if (own === generation) error.value = failure(err) }
  finally { if (own === generation) loading.value = false }
}
watch(() => [props.hostId, props.entry.path], load, { immediate: true })
onBeforeUnmount(() => { generation++; controller?.abort() })
function value(item: OfficeItem) { return item.id && Object.hasOwn(drafts.value, item.id) ? drafts.value[item.id]! : item.text }
function change(event: Event) {
  const item = selected.value
  if (!item?.id || !item.editable || saving.value || needsReload.value) return
  const text = (event.target as HTMLInputElement).value
  if (text === item.text) delete drafts.value[item.id]
  else drafts.value[item.id] = text
  saved.value = false; error.value = ''
}
async function save() {
  if (!doc.value || !dirty.value || saving.value || needsReload.value) return
  const edits = Object.entries(drafts.value).map(([id, text]) => ({ id, text }))
  if (edits.length > 256 || edits.some(edit => new TextEncoder().encode(edit.text).length > 65536)) { error.value = t('office.editLimit'); return }
  const own = generation, files = fileAPIForHost(props.hostId), snapshot = doc.value
  saving.value = true; emit('saving', true); error.value = ''; saved.value = false
  let committed = false
  try {
    const result = await files.writeOffice(snapshot.entry.path, edits, snapshot.entry.resourceVersion, snapshot.contentVersion)
    committed = true
    const refreshed = await files.office(snapshot.entry.path)
    if (own !== generation) return
    doc.value = refreshed; drafts.value = {}; selected.value = undefined; saved.value = true
    emit('saved', result.entry)
  } catch (err) {
    if (own === generation) { needsReload.value = committed; error.value = committed ? t('office.savedReloadFailed') : failure(err) }
  } finally { if (own === generation) { saving.value = false; emit('saving', false) } }
}
function columnName(column: number) { let name = ''; for (let n = column; n > 0; n = Math.floor((n - 1) / 26)) name = String.fromCharCode(65 + (n - 1) % 26) + name; return name }
function style(item: OfficeItem) {
  return { fontWeight: item.bold ? '700' : undefined, fontStyle: item.italic ? 'italic' : undefined,
    fontSize: `${Math.max(14, Math.min(48, item.fontSize ?? 16))}px`,
    textAlign: (['center', 'ctr'].includes(item.align ?? '') ? 'center' : ['right', 'r'].includes(item.align ?? '') ? 'right' : 'left') as 'left' | 'center' | 'right' }
}
function slideStyle(item: OfficeItem) {
  const w = section.value?.width || 960, h = section.value?.height || 540
  return { ...style(item), left: `${(item.x ?? 0) / w * 100}%`, top: `${(item.y ?? 0) / h * 100}%`,
    width: `${Math.min(100, (item.width ?? w) / w * 100)}%`, height: `${Math.min(100, (item.height ?? h) / h * 100)}%` }
}
function imageSource(item: OfficeItem) { return /^data:image\/(?:png|jpeg|gif);base64,[A-Za-z0-9+/=]+$/.test(item.image ?? '') ? item.image : undefined }
function moveRows(direction: number) { rowStart.value = Math.max(1, Math.min(section.value?.rows ?? 1, rowStart.value + direction * 40)) }
function moveColumns(direction: number) { columnStart.value = Math.max(1, Math.min(section.value?.columns ?? 1, columnStart.value + direction * 12)) }
function jumpRows(event: Event) { rowStart.value = Math.max(1, Math.min(section.value?.rows ?? 1, Number((event.target as HTMLInputElement).value) || 1)) }
function shortcut(event: KeyboardEvent) { if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') { event.preventDefault(); void save() } }
</script>

<template>
  <section class="office-workspace" :aria-label="t('office.title')" @keydown="shortcut">
    <header class="office-toolbar">
      <span class="office-label"><FileText :size="18" aria-hidden="true" />{{ t('office.title') }}</span>
      <span class="office-status" role="status">{{ saving ? t('office.saving') : dirty ? t('office.unsaved') : saved ? t('office.saved') : t('office.ready') }}</span>
      <button class="button button--secondary button--small" :disabled="saving || loading || (dirty && !needsReload)" @click="load"><RefreshCw :size="16" aria-hidden="true" />{{ t('office.reload') }}</button>
      <button class="button button--primary button--small office-save" :disabled="!dirty || saving || needsReload" @click="save"><Save :size="16" aria-hidden="true" />{{ t('office.save') }}</button>
    </header>
    <p class="office-hint">{{ t('office.basic') }}</p>
    <p v-if="doc?.kind === 'xlsx'" class="office-hint">{{ t('office.sheetHint') }}</p>
    <p v-if="doc?.notes.includes('signed_readonly')" class="office-hint">{{ t('office.signed') }}</p>
    <p v-if="error" class="office-error" role="alert">{{ error }}<button v-if="!doc && !loading" class="button button--secondary button--small" @click="load">{{ t('office.retry') }}</button></p>
    <div v-if="loading" class="office-empty" role="status">{{ t('office.loading') }}</div>
    <template v-else-if="doc && section">
      <nav v-if="doc.sections.length > 1" class="office-tabs" :aria-label="t('office.sections')">
        <button v-for="(s, i) in doc.sections" :key="i" :aria-current="sectionIndex === i ? 'true' : undefined" @click="sectionIndex = i"><span data-i18n-ignore>{{ s.name || i + 1 }}</span></button>
      </nav>
      <div class="office-body">
        <main class="office-canvas" :class="`office-canvas--${doc.kind}`">
          <div v-if="doc.kind === 'docx'" class="office-paper">
            <template v-for="(item, index) in pageItems" :key="item.id || index">
              <img v-if="item.kind === 'image'" :src="imageSource(item)" :alt="t('office.image')" />
              <table v-else-if="item.kind === 'table'" class="office-table"><tbody><tr v-for="(row, r) in item.table" :key="r"><td v-for="(cell, c) in row" :key="c"><button :class="{ 'is-selected': selected?.id === cell.id }" :style="style(cell)" @click="selected = cell"><span data-i18n-ignore>{{ value(cell) || '\u00a0' }}</span></button></td></tr></tbody></table>
              <button v-else class="office-paragraph" :class="{ 'is-selected': selected?.id === item.id }" :style="style(item)" @click="selected = item"><span data-i18n-ignore>{{ value(item) || '\u00a0' }}</span></button>
            </template>
            <p v-if="section.items.length === 0">{{ t('office.empty') }}</p>
            <div v-if="section.items.length > 80" class="office-paging"><button :disabled="page === 0" @click="page--">{{ t('office.previous') }}</button><span data-i18n-ignore>{{ page + 1 }}/{{ Math.ceil(section.items.length / 80) }}</span><button :disabled="(page + 1) * 80 >= section.items.length" @click="page++">{{ t('office.next') }}</button></div>
          </div>
          <template v-else-if="doc.kind === 'xlsx'">
            <div class="office-paging"><button :disabled="rowStart === 1" @click="moveRows(-1)">{{ t('office.previous') }}</button><label>{{ t('office.row') }}<input type="number" :value="rowStart" min="1" :max="section.rows || 1" @change="jumpRows" /></label><button :disabled="rowStart + 40 > (section.rows || 1)" @click="moveRows(1)">{{ t('office.next') }}</button><button :aria-label="t('office.previousColumns')" :disabled="columnStart === 1" @click="moveColumns(-1)"><ChevronLeft :size="18" aria-hidden="true" /></button><button :aria-label="t('office.nextColumns')" :disabled="columnStart + 12 > (section.columns || 1)" @click="moveColumns(1)"><ChevronRight :size="18" aria-hidden="true" /></button></div>
            <div class="office-grid-scroll"><table class="office-grid"><thead><tr><th></th><th v-for="column in columns" :key="column" scope="col" data-i18n-ignore>{{ columnName(column) }}</th></tr></thead><tbody><tr v-for="row in rows" :key="row"><th scope="row" data-i18n-ignore>{{ row }}</th><td v-for="column in columns" :key="column"><button :disabled="!cells.get(`${row}:${column}`)" :class="{ 'is-selected': selected?.id && selected.id === cells.get(`${row}:${column}`)?.id }" :title="cells.get(`${row}:${column}`)?.formula" @click="selected = cells.get(`${row}:${column}`)"><span data-i18n-ignore>{{ cells.get(`${row}:${column}`) ? value(cells.get(`${row}:${column}`)!) : '\u00a0' }}</span></button></td></tr></tbody></table></div>
          </template>
          <div v-else class="office-slide" :style="{ aspectRatio: `${section.width || 960}/${section.height || 540}` }">
            <template v-for="(item, index) in section.items" :key="item.id || index"><img v-if="item.kind === 'image'" :src="imageSource(item)" :alt="t('office.image')" :style="slideStyle(item)" /><button v-else :style="slideStyle(item)" :class="{ 'is-selected': selected?.id === item.id }" @click="selected = item"><span data-i18n-ignore>{{ value(item) || '\u00a0' }}</span></button></template>
          </div>
        </main>
        <aside class="office-inspector">
          <template v-if="selected"><label :for="inputID">{{ t('office.content') }}<span v-if="selected.kind === 'cell'" data-i18n-ignore> · {{ columnName(selected.column || 1) }}{{ selected.row }}</span></label>
            <textarea v-if="doc.kind === 'xlsx'" :id="inputID" :value="value(selected)" :readonly="!selected.editable || needsReload" :disabled="saving" maxlength="65536" data-i18n-ignore @input="change" />
            <input v-else :id="inputID" type="text" :value="value(selected)" :readonly="!selected.editable || needsReload" :disabled="saving" maxlength="65536" data-i18n-ignore @input="change" />
            <p v-if="!selected.editable">{{ t('office.readonly') }}</p><p v-else>{{ t('office.editHint') }}</p><code v-if="selected.formula" data-i18n-ignore>{{ selected.formula }}</code>
          </template><p v-else>{{ t('office.select') }}</p>
        </aside>
      </div>
    </template>
  </section>
</template>

<style scoped>
.office-workspace { display: flex; flex-direction: column; min-width: 0; min-height: 360px; height: 70dvh; color: var(--file-preview-text); background: var(--file-preview-background); border: 1px solid var(--file-preview-border); border-radius: var(--radius); overflow: hidden; font-size: 14px; }
.office-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; padding: 12px; border-bottom: 1px solid var(--file-preview-border); background: var(--file-preview-panel); }
.office-label { display: flex; gap: 8px; align-items: center; font-weight: 600; }
.office-status { flex: 1; font-size: 13px; color: var(--file-preview-muted); }
.office-hint { margin: 6px 12px; color: var(--file-preview-muted); font-size: 13px; line-height: 1.5; }
.office-error { margin: 8px 12px; color: var(--danger); line-height: 1.5; }
.office-error button { margin-left: 10px; }
.office-empty { margin: auto; padding: 30px; }
.office-tabs { display: flex; overflow-x: auto; flex: 0 0 auto; border-bottom: 1px solid var(--file-preview-border); }
.office-tabs button { padding: 10px 16px; white-space: nowrap; }
.office-tabs [aria-current] { box-shadow: inset 0 -2px var(--file-preview-accent); }
.office-body { display: grid; grid-template-columns: minmax(0, 1fr) 260px; min-height: 0; flex: 1; }
.office-canvas { min-width: 0; overflow: auto; padding: 24px; }
.office-paper { max-width: 740px; min-height: 100%; padding: 32px; margin: auto; border: 1px solid var(--file-preview-border); background: var(--file-preview-panel); }
.office-paper img { display: block; max-width: 100%; max-height: 360px; margin: 10px auto; object-fit: contain; }
.office-workspace button { font: inherit; color: inherit; background: transparent; cursor: pointer; border: 0; }
.office-workspace .button--primary { color: var(--file-preview-background); background: var(--file-preview-accent); }
.office-workspace button:disabled { cursor: default; opacity: .5; }
.office-paragraph { width: 100%; display: block; text-align: left; padding: 8px; line-height: 1.7; overflow-wrap: anywhere; white-space: pre-wrap; }
.office-workspace .is-selected { outline: 2px solid var(--file-preview-accent); outline-offset: -2px; background: var(--file-preview-panel-raised); }
.office-workspace button:focus-visible, .office-workspace input:focus-visible, .office-workspace textarea:focus-visible { outline: 2px solid var(--file-preview-accent); outline-offset: -2px; }
.office-table { width: 100%; table-layout: fixed; border-collapse: collapse; margin: 10px 0; }
.office-table td { border: 1px solid var(--file-preview-border); overflow-wrap: anywhere; }
.office-table button { width: 100%; padding: 8px; text-align: left; white-space: pre-wrap; }
.office-inspector { overflow: auto; padding: 16px; border-left: 1px solid var(--file-preview-border); background: var(--file-preview-panel); }
.office-inspector label { display: block; margin-bottom: 10px; font-weight: 600; }
.office-inspector p { color: var(--file-preview-muted); line-height: 1.7; }
.office-inspector input, .office-inspector textarea, .office-paging input { min-width: 0; width: 100%; padding: 10px; font: inherit; border: 1px solid var(--file-preview-border); border-radius: var(--radius-sm); background: var(--file-preview-background); color: inherit; }
.office-inspector textarea { height: 160px; resize: vertical; }
.office-inspector code { display: block; overflow-wrap: anywhere; }
.office-canvas--xlsx { padding: 0; display: flex; flex-direction: column; }
.office-paging { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 8px; padding: 8px; font-size: 13px; }
.office-paging button { min-height: 36px; padding: 6px; }
.office-paging input { width: 90px; padding: 6px; margin-left: 6px; }
.office-grid-scroll { overflow: auto; flex: 1; min-height: 0; }
.office-grid { border-collapse: separate; border-spacing: 0; width: max-content; min-width: 100%; font-size: 14px; }
.office-grid td, .office-grid th { border-right: 1px solid var(--file-preview-border); border-bottom: 1px solid var(--file-preview-border); }
.office-grid th { position: sticky; top: 0; background: var(--file-preview-panel); height: 34px; min-width: 48px; font-weight: 500; }
.office-grid td button { display: block; width: 140px; height: 36px; padding: 6px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; text-align: left; }
.office-slide { position: relative; width: 100%; overflow: hidden; background: var(--file-preview-panel); border: 1px solid var(--file-preview-border); }
.office-slide button, .office-slide img { position: absolute; padding: 8px; overflow: hidden; white-space: pre-wrap; overflow-wrap: anywhere; object-fit: contain; }
:global(.modal-panel--fullscreen .office-workspace) { height: 100%; }
@media (max-width: 800px) { .office-body { grid-template-columns: 1fr; grid-template-rows: minmax(180px, 1fr) auto; } .office-inspector { max-height: 200px; border-left: 0; border-top: 1px solid var(--file-preview-border); padding: 12px; } .office-inspector p { margin: 8px 0; } .office-inspector textarea { height: 72px; } .office-paper { padding: 16px; } .office-canvas { padding: 12px; } .office-canvas--xlsx { padding: 0; } }
</style>
