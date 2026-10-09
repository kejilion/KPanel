<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, provide, reactive, ref, watch } from 'vue'
import {
  Check, CircleAlert, Copy, Download, FileSpreadsheet, FileText, FileWarning, Info, ListChecks, LoaderCircle, Lock,
  Presentation, RefreshCw, Save, Undo2, X,
} from '@lucide/vue'
import { useI18n } from '@/i18n'
import { ApiError } from '@/lib/api'
import { copyText } from '@/lib/clipboard'
import { fileIconPalette } from '@/lib/fileEntryPresentation'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { indexOfficeDocument, OFFICE_MAX_EDIT_BYTES, OFFICE_MAX_EDITS, type OfficeLocation } from '@/lib/officeDocument'
import type { FileEntry, OfficeDocument, OfficeItem } from '@/types/api'
import OfficeDocumentView from './OfficeDocumentView.vue'
import OfficeSheetView from './OfficeSheetView.vue'
import OfficeSlideView from './OfficeSlideView.vue'
import { officeEditingKey, type OfficeEditing } from './officeEditing'

type OfficeKind = OfficeDocument['kind']
const NOTICE_KEY = 'kpanel:office:notice:v1'
const props = defineProps<{ entry: FileEntry; hostId: string }>()
const emit = defineEmits<{ dirty: [boolean]; saving: [boolean]; saved: [FileEntry]; download: [] }>()
const { t } = useI18n()
const doc = ref<OfficeDocument>()
const loading = ref(false), saving = ref(false), saved = ref(false), needsReload = ref(false)
const error = ref<{ message: string; conflict?: boolean }>()
const sectionIndex = ref(0)
const selectedId = ref<string>(), editingId = ref<string>()
// A reactive Map tracks per-id has/get, so views refresh when a draft is first added.
const drafts = reactive(new Map<string, string>())
const changesOpen = ref(false), noticeOpen = ref(readNotice()), copied = ref<{ id: string; ok: boolean }>()
const view = ref<{ reveal(location: OfficeLocation): Promise<void> }>()
const changesPanel = ref<HTMLElement>(), changesToggle = ref<HTMLButtonElement>(), reloadButton = ref<HTMLButtonElement>()
let editOrigin: { id: string; draft?: string } | undefined
let controller: AbortController | undefined, generation = 0, copiedTimer: ReturnType<typeof setTimeout> | undefined

const index = computed(() => doc.value ? indexOfficeDocument(doc.value) : new Map<string, OfficeLocation>())
const dirtyCount = computed(() => drafts.size)
const dirty = computed(() => dirtyCount.value > 0)
const locked = computed(() => saving.value || needsReload.value)
const signed = computed(() => doc.value?.notes.includes('signed_readonly') ?? false)
const editable = computed(() => !signed.value && [...index.value.values()].some(location => location.item.editable))
const kind = computed<OfficeKind>(() => doc.value?.kind ?? (/\.(xlsx|pptx)$/i.exec(props.entry.name)?.[1]?.toLowerCase() as OfficeKind | undefined) ?? 'docx')
const kindIcon = computed(() => ({ docx: FileText, xlsx: FileSpreadsheet, pptx: Presentation })[kind.value])
// Same hue as the file browser glyph for this format.
const kindColor = computed(() => fileIconPalette[({ docx: 'document', xlsx: 'spreadsheet', pptx: 'presentation' } as const)[kind.value]][0])
const kindLabel = computed(() => t(({ docx: 'office.kind.docx', xlsx: 'office.kind.xlsx', pptx: 'office.kind.pptx' } as const)[kind.value]))
const changes = computed(() => [...drafts].map(([id, text]) => ({ id, text, location: index.value.get(id) })))
const status = computed(() => {
  if (loading.value) return { tone: 'busy', text: t('office.loading') }
  if (saving.value) return { tone: 'busy', text: t('office.saving') }
  if (needsReload.value) return { tone: 'warning', text: t('office.needsReload') }
  if (dirty.value) return { tone: 'dirty', text: t('office.unsavedCount', { count: dirtyCount.value }) }
  if (saved.value) return { tone: 'success', text: t('office.saved') }
  if (!doc.value) return { tone: 'idle', text: '' }
  return editable.value ? { tone: 'ready', text: t('office.editable') } : { tone: 'readonly', text: t('office.previewOnly') }
})
const hint = computed(() => {
  if (!doc.value) return ''
  if (!editable.value) return t('office.previewOnlyHint')
  if (editingId.value) return t('office.hint.editing')
  const selected = selectedId.value ? index.value.get(selectedId.value)?.item : undefined
  if (selected && !selected.editable) return t('office.readonly')
  return t(({ docx: 'office.hint.docx', xlsx: 'office.hint.xlsx', pptx: 'office.hint.pptx' } as const)[doc.value.kind])
})
watch(dirty, value => emit('dirty', value))
watch(dirty, value => { if (!value) changesOpen.value = false })

function draftValue(item: OfficeItem) { return item.id && drafts.has(item.id) ? drafts.get(item.id)! : item.text }
const editing: OfficeEditing = {
  selectedId, editingId,
  value: draftValue,
  isModified: item => Boolean(item?.id && drafts.has(item.id)),
  canEdit: item => Boolean(item?.id && item.editable && !locked.value && !signed.value),
  select(item) { selectedId.value = item?.id },
  startEdit(item) {
    selectedId.value = item.id
    if (!editing.canEdit(item)) return false
    if (editingId.value === item.id) return true
    editing.commit()
    editingId.value = item.id
    editOrigin = { id: item.id!, draft: drafts.get(item.id!) }
    return true
  },
  update(item, text) {
    if (!item.id || !editing.canEdit(item)) return
    if (text === item.text) drafts.delete(item.id)
    else drafts.set(item.id, text)
    saved.value = false
    if (!needsReload.value) error.value = undefined
  },
  commit() { editingId.value = undefined; editOrigin = undefined },
  cancel() {
    const origin = editOrigin
    if (origin && origin.draft === undefined) drafts.delete(origin.id)
    else if (origin) drafts.set(origin.id, origin.draft!)
    editingId.value = undefined; editOrigin = undefined
  },
}
provide(officeEditingKey, editing)

function failure(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'file_conflict') return t('office.conflict')
    if (err.code === 'office_unsupported') return t('office.unsupported')
    if (err.code === 'file_too_large') return t('office.tooLarge')
    if (err.code === 'office_edit_invalid') return t('office.invalidEdit')
  }
  return t('office.failed')
}
async function load(keepPosition = false) {
  const own = ++generation
  controller?.abort(); controller = new AbortController()
  const files = fileAPIForHost(props.hostId), path = props.entry.path, section = keepPosition ? sectionIndex.value : 0
  doc.value = undefined; loading.value = true; error.value = undefined; saved.value = false; needsReload.value = false
  drafts.clear(); selectedId.value = undefined; editingId.value = undefined; editOrigin = undefined; changesOpen.value = false
  try {
    const result = await files.office(path, controller.signal)
    if (own !== generation) return
    sectionIndex.value = Math.min(section, Math.max(0, result.sections.length - 1))
    doc.value = result
  } catch (err) { if (own === generation) error.value = { message: failure(err) } }
  finally { if (own === generation) loading.value = false }
}
watch(() => [props.hostId, props.entry.path], () => load(), { immediate: true })
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value || saving.value) { event.preventDefault(); event.returnValue = '' } }
function outsidePointer(event: PointerEvent) {
  const target = event.target as Node
  if (changesOpen.value && !changesPanel.value?.contains(target) && !changesToggle.value?.contains(target)) changesOpen.value = false
}
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); document.addEventListener('pointerdown', outsidePointer) })
onBeforeUnmount(() => {
  generation++; controller?.abort(); clearTimeout(copiedTimer)
  window.removeEventListener('beforeunload', beforeUnload); document.removeEventListener('pointerdown', outsidePointer)
})

async function save() {
  editing.commit()
  if (!doc.value || !dirty.value || saving.value || needsReload.value) return
  const edits = [...drafts].map(([id, text]) => ({ id, text }))
  if (edits.length > OFFICE_MAX_EDITS || edits.some(edit => new TextEncoder().encode(edit.text).length > OFFICE_MAX_EDIT_BYTES)) {
    error.value = { message: t('office.editLimit') }; return
  }
  const own = generation, files = fileAPIForHost(props.hostId), snapshot = doc.value
  saving.value = true; emit('saving', true); error.value = undefined; saved.value = false
  let committed = false
  try {
    const result = await files.writeOffice(snapshot.entry.path, edits, snapshot.entry.resourceVersion, snapshot.contentVersion)
    committed = true
    const refreshed = await files.office(snapshot.entry.path)
    if (own !== generation) return
    sectionIndex.value = Math.min(sectionIndex.value, Math.max(0, refreshed.sections.length - 1))
    doc.value = refreshed; drafts.clear(); selectedId.value = undefined; saved.value = true
    emit('saved', result.entry)
  } catch (err) {
    if (own !== generation) return
    needsReload.value = committed
    error.value = committed ? { message: t('office.savedReloadFailed') }
      : { message: failure(err), conflict: err instanceof ApiError && err.code === 'file_conflict' }
  } finally { if (own === generation) { saving.value = false; emit('saving', false) } }
}
function shortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === 's') { event.preventDefault(); event.stopPropagation(); void save() }
}

function readNotice() { try { return localStorage.getItem(NOTICE_KEY) !== 'hidden' } catch { return true } }
function setNotice(open: boolean) {
  noticeOpen.value = open
  try { if (open) localStorage.removeItem(NOTICE_KEY); else localStorage.setItem(NOTICE_KEY, 'hidden') } catch { /* preference is optional */ }
}

function locationLabel(location?: OfficeLocation) {
  const label = location?.label
  switch (label?.kind) {
    case 'paragraph': return t('office.loc.paragraph', label)
    case 'tableCell': return t('office.loc.tableCell', label)
    case 'cell': return t('office.loc.cell', label)
    case 'slideText': return t('office.loc.slideText', label)
    default: return t('office.content')
  }
}
function locate(id?: string) { return id ? index.value.get(id) : undefined }
async function closeChanges(restoreFocus: boolean) {
  changesOpen.value = false
  if (restoreFocus) { await nextTick(); changesToggle.value?.focus() }
}
async function toggleChanges() {
  changesOpen.value = !changesOpen.value
  if (changesOpen.value) { await nextTick(); changesPanel.value?.focus() }
}
async function reveal(id: string) {
  const location = index.value.get(id)
  if (!location) return
  changesOpen.value = false
  editing.commit()
  sectionIndex.value = location.section
  await nextTick()
  selectedId.value = id
  await view.value?.reveal(location)
}
/** The changes toggle disappears with the last draft; keep keyboard focus on a nearby toolbar control. */
function settleFocus() {
  void nextTick(() => reloadButton.value?.focus())
}
function revert(id: string) {
  if (editingId.value === id) editing.commit()
  drafts.delete(id)
  if (!dirty.value) settleFocus()
}
async function copy(id: string, text: string) {
  copied.value = { id, ok: await copyText(text) }
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => { copied.value = undefined }, 2400)
}
function copyLabel(id: string) {
  if (copied.value?.id !== id) return t('office.copyChange')
  return copied.value.ok ? t('office.copied') : t('office.copyFailed')
}
function discardAll() {
  editing.commit()
  drafts.clear(); saved.value = false
  if (!needsReload.value) error.value = undefined
  settleFocus()
}
function discardAndReload() { drafts.clear(); void load(true) }
</script>

<template>
  <section class="office-workspace" :data-kind="kind" :aria-label="t('office.title')" @keydown="shortcut">
    <header class="office-toolbar">
      <div class="office-toolbar__start">
        <span class="office-kind"><component :is="kindIcon" :size="18" :color="kindColor" aria-hidden="true" />{{ kindLabel }}</span>
        <span v-if="status.text" class="office-status" :data-tone="status.tone" role="status">
          <LoaderCircle v-if="status.tone === 'busy'" :size="15" class="spin" aria-hidden="true" />
          <Check v-else-if="status.tone === 'success'" :size="15" aria-hidden="true" />
          <Lock v-else-if="status.tone === 'readonly'" :size="14" aria-hidden="true" />
          <CircleAlert v-else-if="status.tone === 'warning'" :size="15" aria-hidden="true" />
          <span v-else class="office-status__dot" aria-hidden="true" />
          {{ status.text }}
        </span>
      </div>
      <div class="office-toolbar__end">
        <button
          type="button"
          class="office-tool office-tool--icon"
          :aria-pressed="noticeOpen"
          :aria-label="t('office.notice.show')"
          :title="t('office.notice.show')"
          :disabled="!doc"
          @click="setNotice(!noticeOpen)"
        ><Info :size="18" aria-hidden="true" /></button>
        <button
          v-if="dirty"
          ref="changesToggle"
          type="button"
          class="office-tool office-changes-toggle"
          aria-haspopup="dialog"
          :aria-expanded="changesOpen"
          :aria-label="t('office.changesCount', { count: dirtyCount })"
          @click="toggleChanges"
        ><ListChecks :size="18" aria-hidden="true" /><span class="office-tool__text">{{ t('office.changes') }}</span><span class="office-count" aria-hidden="true">{{ dirtyCount }}</span></button>
        <button
          ref="reloadButton"
          type="button"
          class="office-tool office-reload"
          :aria-label="t('office.reload')"
          :title="dirty && !needsReload ? t('office.reloadBlocked') : t('office.reload')"
          :disabled="saving || loading || (dirty && !needsReload)"
          @click="load(true)"
        ><RefreshCw :size="17" aria-hidden="true" /><span class="office-tool__text">{{ t('office.reload') }}</span></button>
        <button
          type="button"
          class="button button--primary office-save"
          aria-keyshortcuts="Control+S Meta+S"
          :title="t('office.saveShortcut')"
          :disabled="!dirty || saving || needsReload"
          @click="save"
        ><LoaderCircle v-if="saving" :size="16" class="spin" aria-hidden="true" /><Save v-else :size="16" aria-hidden="true" />{{ t('office.save') }}</button>
      </div>
      <div
        v-if="changesOpen"
        ref="changesPanel"
        class="office-changes"
        role="dialog"
        tabindex="-1"
        :aria-label="t('office.changesTitle')"
        @keydown.esc.stop.prevent="closeChanges(true)"
      >
        <div class="office-changes__header">
          <div>
            <strong>{{ t('office.changesTitle') }}</strong>
            <span>{{ t('office.changesHint') }}</span>
          </div>
          <button type="button" class="office-icon-button" :aria-label="t('office.close')" @click="closeChanges(true)"><X :size="18" aria-hidden="true" /></button>
        </div>
        <ol class="office-changes__list">
          <li v-for="change in changes" :key="change.id" class="office-change">
            <button type="button" class="office-change__locate" :title="t('office.locate')" @click="reveal(change.id)">
              <span class="office-change__where">{{ locationLabel(change.location) }}</span>
              <span class="office-change__after" data-i18n-ignore>{{ change.text || t('office.emptyText') }}</span>
              <span class="office-change__before"><span class="sr-only">{{ t('office.original') }}</span><span data-i18n-ignore>{{ change.location?.item.text || t('office.emptyText') }}</span></span>
            </button>
            <button type="button" class="office-icon-button" :aria-label="copyLabel(change.id)" :title="copyLabel(change.id)" @click="copy(change.id, change.text)">
              <Check v-if="copied?.id === change.id && copied.ok" :size="16" aria-hidden="true" />
              <CircleAlert v-else-if="copied?.id === change.id" :size="16" class="office-copy-failed" aria-hidden="true" />
              <Copy v-else :size="16" aria-hidden="true" />
            </button>
            <button type="button" class="office-icon-button" :aria-label="t('office.revertChange')" :title="t('office.revertChange')" :disabled="locked" @click="revert(change.id)"><Undo2 :size="16" aria-hidden="true" /></button>
          </li>
        </ol>
        <div class="office-changes__footer">
          <button type="button" class="office-text-button office-text-button--danger" :disabled="locked" @click="discardAll">{{ t('office.discardAll') }}</button>
        </div>
      </div>
    </header>

    <div v-if="doc && signed" class="office-notice office-notice--locked" role="note">
      <Lock :size="16" aria-hidden="true" /><p>{{ t('office.signed') }}</p>
    </div>
    <div v-if="doc && noticeOpen" class="office-notice" role="note">
      <Info :size="16" aria-hidden="true" />
      <p>{{ t('office.basic') }}<template v-if="kind === 'xlsx'"> {{ t('office.sheetHint') }}</template></p>
      <button type="button" class="office-text-button" @click="setNotice(false)">{{ t('office.notice.dismiss') }}</button>
    </div>
    <div v-if="doc && error" class="office-alert" role="alert">
      <CircleAlert :size="18" aria-hidden="true" />
      <p>{{ error.message }}</p>
      <div class="office-alert__actions">
        <button v-if="error.conflict && dirty" type="button" class="office-text-button" @click="toggleChanges">{{ t('office.reviewChanges') }}</button>
        <button v-if="error.conflict" type="button" class="office-text-button office-text-button--danger" @click="discardAndReload">{{ t('office.discardAndReload') }}</button>
        <button v-if="needsReload" type="button" class="office-text-button" @click="load(true)">{{ t('office.reload') }}</button>
        <button v-else type="button" class="office-icon-button" :aria-label="t('office.close')" @click="error = undefined"><X :size="18" aria-hidden="true" /></button>
      </div>
    </div>

    <div v-if="loading" class="office-state" role="status">
      <LoaderCircle :size="28" class="spin" aria-hidden="true" /><span>{{ t('office.loading') }}</span>
    </div>
    <div v-else-if="!doc && error" class="office-state office-state--error" role="alert">
      <FileWarning :size="40" aria-hidden="true" />
      <strong>{{ t('office.openFailed') }}</strong>
      <span>{{ error.message }}</span>
      <div class="office-state__actions">
        <button type="button" class="office-tool office-tool--outlined" @click="load()"><RefreshCw :size="17" aria-hidden="true" />{{ t('office.retry') }}</button>
        <button type="button" class="button button--primary office-save" @click="emit('download')"><Download :size="16" aria-hidden="true" />{{ t('office.downloadOriginal') }}</button>
      </div>
    </div>
    <template v-else-if="doc">
      <OfficeDocumentView v-if="doc.kind === 'docx'" ref="view" :section="doc.sections[sectionIndex]!" :locate="locate" :hint="hint" />
      <OfficeSheetView v-else-if="doc.kind === 'xlsx'" ref="view" v-model:section-index="sectionIndex" :sections="doc.sections" :hint="hint" />
      <OfficeSlideView v-else ref="view" v-model:section-index="sectionIndex" :sections="doc.sections" :locate="locate" :hint="hint" />
    </template>
  </section>
</template>

<style>
/* Unscoped so the per-format views share one visual system; everything stays under .office-workspace. */
.office-workspace {
  --office-paper: #fff;
  --office-ink: #1f2329;
  --office-ink-muted: #5f6a79;
  --office-paper-line: #d3d8df;
  --office-paper-accent: color-mix(in srgb, var(--brand) 78%, #000);
  --office-paper-hover: color-mix(in srgb, var(--brand) 6%, #fff);
  --office-paper-active: color-mix(in srgb, var(--brand) 10%, #fff);
  --office-danger: color-mix(in srgb, var(--danger) 72%, var(--file-preview-text));
  --office-success: color-mix(in srgb, var(--success) 72%, var(--file-preview-text));
  --office-warning: color-mix(in srgb, var(--amber) 72%, var(--file-preview-text));
  container: office-workspace / inline-size;
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 360px;
  height: 70dvh;
  overflow: hidden;
  color: var(--file-preview-text);
  background: var(--file-preview-background);
  border: 1px solid var(--file-preview-border);
  border-radius: var(--radius);
  font-size: 14px;
  line-height: 1.5;
}
.modal-panel--workspace .office-workspace { flex: 1 1 auto; height: auto; min-height: 0; border: 0; border-radius: 0; }
.modal-panel--fullscreen .office-workspace { height: 100%; }
.office-workspace :where(button, input, textarea) { font: inherit; }
.office-workspace :where(button, input, textarea, [tabindex]):focus-visible { outline: 2px solid var(--file-preview-accent); outline-offset: -2px; }

/* Toolbar ------------------------------------------------------------------ */
.office-toolbar { position: relative; z-index: 5; display: flex; flex: 0 0 auto; flex-wrap: wrap; align-items: center; gap: 8px 12px; min-height: 52px; padding: 8px 12px; background: var(--file-preview-panel); border-bottom: 1px solid var(--file-preview-border); }
.office-toolbar__start { display: flex; flex: 1 1 auto; flex-wrap: wrap; align-items: center; gap: 8px 12px; min-width: 0; }
.office-toolbar__end { display: flex; align-items: center; gap: 4px; margin-left: auto; }
.office-kind { display: inline-flex; align-items: center; gap: 8px; font-weight: 600; white-space: nowrap; }
.office-status { display: inline-flex; align-items: center; gap: 6px; min-height: 28px; padding: 2px 10px; border-radius: 999px; color: var(--file-preview-muted); background: color-mix(in srgb, var(--file-preview-text) 7%, transparent); font-size: 13px; white-space: nowrap; }
.office-status[data-tone='dirty'] { color: var(--file-preview-text); background: color-mix(in srgb, var(--file-preview-accent) 18%, transparent); }
.office-status[data-tone='success'] svg { color: var(--office-success); }
.office-status[data-tone='warning'] { color: var(--file-preview-text); }
.office-status[data-tone='warning'] svg { color: var(--office-warning); }
.office-status__dot { width: 8px; height: 8px; border-radius: 50%; background: var(--office-success); }
.office-status[data-tone='dirty'] .office-status__dot { background: var(--file-preview-accent); }
.office-tool { display: inline-flex; align-items: center; justify-content: center; gap: 6px; min-width: 36px; height: 36px; padding: 0 10px; border: 1px solid transparent; border-radius: var(--radius-sm); color: var(--file-preview-text); background: transparent; font-size: 14px; font-weight: 500; white-space: nowrap; cursor: pointer; }
.office-tool--icon { width: 36px; padding: 0; color: var(--file-preview-muted); }
.office-tool--outlined { border-color: var(--file-preview-border); }
.office-tool:hover:not(:disabled), .office-tool[aria-pressed='true'], .office-tool[aria-expanded='true'] { color: var(--file-preview-text); background: var(--file-preview-panel-raised); }
.office-tool:disabled, .office-icon-button:disabled, .office-text-button:disabled { cursor: default; opacity: .45; }
.office-count { min-width: 20px; height: 20px; padding: 0 6px; border-radius: 999px; color: var(--on-brand); background: var(--file-preview-accent); font-size: 12px; font-weight: 700; line-height: 20px; text-align: center; }
.office-workspace .office-save { min-height: 36px; padding: 0 14px; font-size: 14px; }
.office-icon-button { display: inline-grid; flex: 0 0 auto; place-items: center; width: 32px; height: 32px; padding: 0; border: 0; border-radius: var(--radius-sm); color: var(--file-preview-text); background: transparent; cursor: pointer; }
.office-icon-button:hover:not(:disabled) { background: var(--file-preview-panel-raised); }
.office-text-button { min-height: 32px; padding: 4px 10px; border: 0; border-radius: var(--radius-sm); color: var(--file-preview-text); background: color-mix(in srgb, var(--file-preview-text) 8%, transparent); font-size: 14px; font-weight: 600; white-space: nowrap; cursor: pointer; }
.office-text-button:hover:not(:disabled) { background: var(--file-preview-panel-raised); }
.office-text-button--danger, .office-copy-failed { color: var(--office-danger); }

/* Pending changes popover -------------------------------------------------- */
.office-changes { position: absolute; top: calc(100% + 6px); right: 12px; display: flex; flex-direction: column; width: min(420px, calc(100% - 24px)); max-height: min(480px, 60dvh); overflow: hidden; color: var(--file-preview-text); background: var(--file-preview-panel); border: 1px solid var(--file-preview-border); border-radius: var(--radius); box-shadow: var(--shadow-md); font-weight: 400; }
.office-changes:focus-visible { outline: 2px solid var(--file-preview-accent); }
.office-changes__header { display: flex; align-items: flex-start; gap: 8px; padding: 12px 10px 10px 16px; border-bottom: 1px solid var(--file-preview-border); }
.office-changes__header div { display: grid; flex: 1; gap: 2px; }
.office-changes__header strong { font-size: 15px; }
.office-changes__header span { color: var(--file-preview-muted); font-size: 13px; }
.office-changes__list { flex: 1; min-height: 0; margin: 0; padding: 6px; overflow: auto; list-style: none; }
.office-change { display: flex; align-items: flex-start; gap: 2px; padding: 2px; border-radius: var(--radius-sm); }
.office-change:hover { background: color-mix(in srgb, var(--file-preview-text) 5%, transparent); }
.office-change__locate { display: grid; flex: 1; gap: 2px; min-width: 0; padding: 6px 8px; border: 0; border-radius: var(--radius-sm); color: inherit; background: transparent; text-align: left; cursor: pointer; }
.office-change__where { color: var(--file-preview-muted); font-size: 13px; }
.office-change__after, .office-change__before { display: -webkit-box; overflow: hidden; overflow-wrap: anywhere; white-space: pre-wrap; -webkit-box-orient: vertical; }
.office-change__after { -webkit-line-clamp: 3; }
.office-change__before { color: var(--file-preview-muted); font-size: 13px; text-decoration: line-through; -webkit-line-clamp: 2; }
.office-changes__footer { display: flex; justify-content: flex-end; padding: 10px 12px; border-top: 1px solid var(--file-preview-border); }

/* Notices, alerts and full-area states ------------------------------------ */
.office-notice, .office-alert { display: flex; flex: 0 0 auto; align-items: flex-start; gap: 10px; padding: 10px 14px; border-bottom: 1px solid var(--file-preview-border); line-height: 1.6; }
.office-notice { background: color-mix(in srgb, var(--file-preview-accent) 8%, var(--file-preview-panel)); font-size: 13px; }
.office-notice > svg, .office-alert > svg { flex: 0 0 auto; margin-top: 3px; color: var(--file-preview-accent); }
.office-notice--locked > svg { color: var(--office-warning); }
.office-notice p, .office-alert p { flex: 1 1 240px; margin: 0; }
.office-notice p { color: var(--file-preview-muted); }
.office-notice--locked p { color: var(--file-preview-text); }
.office-notice .office-text-button { min-height: 28px; margin: -2px 0; }
.office-alert { flex-wrap: wrap; align-items: center; background: color-mix(in srgb, var(--danger) 14%, var(--file-preview-panel)); }
.office-alert > svg { margin-top: 0; color: var(--office-danger); }
.office-alert__actions { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; margin-left: auto; }
.office-state { display: flex; flex: 1; flex-direction: column; align-items: center; justify-content: center; gap: 10px; padding: 32px 24px; color: var(--file-preview-muted); text-align: center; }
.office-state strong { color: var(--file-preview-text); font-size: 17px; }
.office-state > span { max-width: 460px; line-height: 1.6; }
.office-state--error > svg { color: var(--office-danger); }
.office-state__actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 10px; margin-top: 8px; }

/* Shared view frame -------------------------------------------------------- */
.office-view { display: flex; flex: 1; flex-direction: column; min-height: 0; }
.office-view--pptx { container: office-deck / size; }
.office-statusbar { display: flex; flex: 0 0 auto; flex-wrap: wrap; align-items: center; gap: 4px 12px; min-height: 38px; padding: 3px 8px 3px 14px; color: var(--file-preview-muted); background: var(--file-preview-panel); border-top: 1px solid var(--file-preview-border); font-size: 13px; }
.office-statusbar__hint { flex: 1 1 220px; min-width: 0; padding: 4px 0; }
.office-pager { display: inline-flex; align-items: center; gap: 2px; margin-left: auto; color: var(--file-preview-text); white-space: nowrap; }
.office-pager > span { padding: 0 6px; }
.office-placeholder { margin: 48px 0; color: var(--office-ink-muted); text-align: center; }

/* Paper and inline text blocks shared by Word pages and slides ------------ */
.office-block { position: relative; border-radius: var(--radius-sm); }
.office-block__text, .office-text-editor { display: block; box-sizing: border-box; width: 100%; margin: 0; padding: 3px 10px; border: 0; border-radius: var(--radius-sm); color: inherit; background: transparent; font: inherit; line-height: inherit; text-align: inherit; white-space: pre-wrap; overflow-wrap: anywhere; }
.office-block__text { display: flex; flex-direction: column; justify-content: flex-start; cursor: text; }
.office-block__text > span[data-office-text] { display: block; width: 100%; }
.office-block.is-readonly .office-block__text { cursor: default; }
.office-text-editor { overflow: hidden; resize: none; caret-color: var(--office-paper-accent); }
:is(.office-page, .office-slide) .office-block:not(.is-readonly):hover .office-block__text { background: var(--office-paper-hover); }
:is(.office-page, .office-slide) .office-block.is-selected .office-block__text { background: var(--office-paper-active); }
:is(.office-page, .office-slide) .office-text-editor { background: var(--office-paper); outline: 2px solid var(--office-paper-accent); outline-offset: 0; box-shadow: 0 0 0 5px color-mix(in srgb, var(--office-paper-accent) 16%, transparent); }
:is(.office-page, .office-slide) .office-block__text:focus-visible { outline: 2px solid var(--office-paper-accent); outline-offset: 0; }

/* Word -------------------------------------------------------------------- */
.office-desk { container-type: inline-size; flex: 1; min-height: 0; padding: 32px 24px 56px; overflow: auto; background: var(--file-preview-background); }
.office-page { box-sizing: border-box; width: 100%; max-width: 794px; min-height: min(1123px, calc((100cqw - 48px) * 1.414)); margin: 0 auto; padding: 72px 86px; color: var(--office-ink); background: var(--office-paper); border-radius: 0; box-shadow: var(--shadow-md); line-height: 1.65; }
.office-paragraph { margin: 0 -10px 4px; }
.office-page .office-block.is-modified::before { position: absolute; top: 5px; bottom: 5px; left: -10px; width: 3px; border-radius: 999px; background: var(--office-paper-accent); content: ''; }
.office-table { width: 100%; margin: 12px 0 16px; border-collapse: collapse; table-layout: fixed; }
.office-table td { padding: 2px; border: 1px solid var(--office-paper-line); vertical-align: top; }
.office-table .office-block.is-modified::before { left: 0; }
.office-figure { margin: 16px 0; text-align: center; }
.office-figure img { max-width: 100%; max-height: 420px; object-fit: contain; }
.office-figure__missing { display: inline-flex; align-items: center; gap: 8px; padding: 12px 16px; border: 1px dashed var(--office-paper-line); border-radius: var(--radius-sm); color: var(--office-ink-muted); font-size: 13px; }

/* Excel ------------------------------------------------------------------- */
.office-formula-bar { display: flex; flex: 0 0 auto; align-items: flex-start; gap: 8px; padding: 8px 12px; background: var(--file-preview-panel); border-bottom: 1px solid var(--file-preview-border); }
.office-name-box, .office-formula-input { box-sizing: border-box; min-height: 36px; border: 1px solid var(--file-preview-border); border-radius: var(--radius-sm); color: var(--file-preview-text); background: var(--file-preview-background); }
.office-name-box { flex: 0 0 96px; width: 96px; padding: 0 10px; font-family: var(--font-mono); font-weight: 600; text-transform: uppercase; }
.office-name-box.is-invalid { border-color: var(--office-danger); }
.office-fx { display: grid; flex: 0 0 auto; place-items: center; width: 24px; height: 36px; color: var(--file-preview-muted); }
.office-formula-input { flex: 1; min-width: 0; max-height: 132px; padding: 6px 10px; overflow: auto; line-height: 1.55; resize: none; field-sizing: content; }
.office-formula-input[readonly] { color: var(--file-preview-muted); }
.office-formula-input::placeholder { color: var(--file-preview-muted); opacity: 1; }
.office-chip { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 6px; height: 36px; padding: 0 10px; border-radius: 999px; color: var(--file-preview-muted); background: color-mix(in srgb, var(--file-preview-text) 7%, transparent); font-size: 13px; white-space: nowrap; }
.office-grid-scroll { flex: 1; min-height: 0; overflow: auto; background: var(--file-preview-background); }
.office-grid { border-collapse: separate; border-spacing: 0; font-size: 14px; }
.office-grid:focus-visible { outline: none; }
.office-grid th { position: sticky; top: 0; z-index: 2; box-sizing: border-box; min-width: 56px; height: 34px; padding: 0 8px; color: var(--file-preview-muted); background: var(--file-preview-panel); border-right: 1px solid var(--file-preview-border); border-bottom: 1px solid var(--file-preview-border); font-size: 13px; font-weight: 500; text-align: center; }
.office-grid tbody th { left: 0; z-index: 1; }
.office-grid .office-grid__corner { left: 0; z-index: 3; }
.office-grid thead th.is-active { color: var(--file-preview-text); background: color-mix(in srgb, var(--file-preview-accent) 20%, var(--file-preview-panel)); box-shadow: inset 0 -2px var(--file-preview-accent); }
.office-grid tbody th.is-active { color: var(--file-preview-text); background: color-mix(in srgb, var(--file-preview-accent) 20%, var(--file-preview-panel)); box-shadow: inset -2px 0 var(--file-preview-accent); }
.office-grid td { position: relative; box-sizing: border-box; width: 128px; min-width: 128px; max-width: 128px; height: 34px; padding: 0 8px; border-right: 1px solid color-mix(in srgb, var(--file-preview-border) 65%, transparent); border-bottom: 1px solid color-mix(in srgb, var(--file-preview-border) 65%, transparent); cursor: cell; scroll-margin: 36px 0 0 58px; user-select: none; }
.office-grid td.is-number { text-align: right; }
.office-grid td.is-active { background: var(--file-preview-active-line); box-shadow: inset 0 0 0 2px color-mix(in srgb, var(--file-preview-accent) 55%, transparent); }
.office-grid:focus-visible td.is-active { box-shadow: inset 0 0 0 2px var(--file-preview-accent); }
.office-grid td.is-modified::after { position: absolute; top: 0; right: 0; border-style: solid; border-width: 0 9px 9px 0; border-color: transparent var(--file-preview-accent) transparent transparent; content: ''; }
.office-cell__text { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.office-cell-editor { position: absolute; top: 0; left: 0; z-index: 4; box-sizing: border-box; width: auto; min-width: 100%; max-width: 420px; min-height: 100%; field-sizing: content; padding: 6px 8px; border: 0; color: var(--file-preview-text); background: var(--file-preview-panel-raised); outline: 2px solid var(--file-preview-accent); outline-offset: 0; box-shadow: var(--shadow-md); font: inherit; line-height: 1.5; text-align: left; white-space: pre-wrap; overflow: hidden; resize: none; user-select: text; }
.office-sheetbar { display: flex; flex: 0 0 auto; flex-wrap: wrap; align-items: stretch; min-height: 40px; background: var(--file-preview-panel); border-top: 1px solid var(--file-preview-border); }
.office-sheet-tabs { display: flex; flex: 1 1 200px; min-width: 0; padding: 0 6px; overflow-x: auto; }
.office-sheet-tab { position: relative; min-height: 40px; padding: 0 16px; border: 0; color: var(--file-preview-muted); background: transparent; white-space: nowrap; cursor: pointer; }
.office-sheet-tab:hover { color: var(--file-preview-text); }
.office-sheet-tab[aria-selected='true'] { color: var(--file-preview-text); background: var(--file-preview-background); font-weight: 600; box-shadow: inset 0 -3px var(--file-preview-accent); }
.office-sheet-nav { display: flex; align-items: center; gap: 2px; margin-left: auto; padding: 0 8px; color: var(--file-preview-muted); font-size: 13px; white-space: nowrap; }
.office-sheet-nav__range { margin-right: 6px; }

/* PowerPoint -------------------------------------------------------------- */
.office-deck { display: grid; flex: 1; grid-template-columns: 184px minmax(0, 1fr); min-height: 0; }
.office-rail { display: flex; flex-direction: column; gap: 12px; min-height: 0; padding: 16px 12px 16px 8px; overflow: auto; background: var(--file-preview-panel); border-right: 1px solid var(--file-preview-border); }
.office-thumb { display: grid; flex: 0 0 auto; grid-template-columns: 22px minmax(0, 1fr); align-items: start; gap: 6px; padding: 4px; border: 0; border-radius: var(--radius-sm); color: var(--file-preview-muted); background: transparent; font-size: 13px; text-align: right; cursor: pointer; }
.office-thumb[aria-current='true'] { color: var(--file-preview-text); font-weight: 600; }
.office-thumb__slide { position: relative; display: block; overflow: hidden; container-type: inline-size; background: var(--office-paper); border-radius: 0; box-shadow: 0 0 0 1px var(--file-preview-border); transition: box-shadow 160ms ease; }
.office-thumb:hover .office-thumb__slide { box-shadow: 0 0 0 2px color-mix(in srgb, var(--file-preview-accent) 55%, transparent); }
.office-thumb[aria-current='true'] .office-thumb__slide { box-shadow: 0 0 0 2px var(--file-preview-accent); }
.office-thumb__slide img { position: absolute; object-fit: contain; }
.office-thumb__text { position: absolute; box-sizing: border-box; padding: .75cqw; font-size: 0; line-height: 0; }
.office-thumb__text i { display: inline-block; height: max(2px, calc(var(--office-font) * 58cqw)); margin-top: calc(var(--office-font) * 32cqw); vertical-align: top; border-radius: 999px; background: color-mix(in srgb, var(--office-ink) 26%, transparent); }
.office-thumb__text.is-bold i { background: color-mix(in srgb, var(--office-ink) 52%, transparent); }
/* Auto margins centre the slide but keep oversized slides scrollable instead of overflowing upwards. */
.office-stage { display: grid; container-type: size; min-width: 0; min-height: 0; overflow: auto; background: var(--file-preview-background); }
.office-slide { position: relative; container-type: inline-size; box-sizing: border-box; width: max(280px, min(calc(100cqw - 56px), calc((100cqh - 56px) * var(--office-ratio)))); overflow: hidden; color: var(--office-ink); background: var(--office-paper); margin: auto; border-radius: 0; box-shadow: var(--shadow-md); }
.office-slide__image { position: absolute; object-fit: contain; }
.office-textbox { position: absolute; font-size: max(14px, calc(var(--office-font) * 100cqw)); line-height: 1.2; }
.office-textbox .office-block__text, .office-textbox .office-text-editor { min-height: 100%; padding: .2em .4em; }
.office-slide .office-block:not(.is-readonly):hover .office-block__text { outline: 1px dashed color-mix(in srgb, var(--office-paper-accent) 70%, transparent); outline-offset: 0; }
.office-slide .office-block.is-modified::before { position: absolute; top: -4px; left: -4px; z-index: 1; width: 9px; height: 9px; border-radius: 50%; background: var(--office-paper-accent); box-shadow: 0 0 0 2px var(--office-paper); content: ''; }
.office-slide .office-placeholder { position: absolute; inset: 0; display: grid; margin: 0; place-items: center; }

/* Narrow containers (phones, split desktop windows) ----------------------- */
@container office-workspace (max-width: 680px) {
  .office-tool__text { display: none; }
  .office-toolbar { padding: 8px 10px; }
  .office-desk { padding: 12px 8px 32px; }
  .office-page { min-height: 0; padding: 28px 22px; }
  .office-deck { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr) auto; }
  .office-rail { flex-direction: row; order: 2; padding: 10px; border-top: 1px solid var(--file-preview-border); border-right: 0; }
  .office-thumb { width: 124px; }
  .office-formula-bar { padding: 8px; }
  .office-name-box { flex-basis: 76px; width: 76px; }
  .office-chip { display: none; }
  .office-sheet-nav__range { display: none; }
}
/* Short windows (200% zoom, landscape phones) keep the slide; the pager and keyboard still switch slides. */
@container office-deck (max-height: 420px) {
  .office-rail { display: none; }
}
@media (prefers-reduced-motion: reduce) {
  .office-thumb__slide { transition: none; }
}
</style>
