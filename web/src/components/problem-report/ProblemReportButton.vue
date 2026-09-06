<script setup lang="ts">
import { computed, ref } from 'vue'
import { ClipboardCopy, Download, FileText } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { buildReport, captureReport, MAX_DESCRIPTION_LENGTH, REPORT_FIELDS, type ReportInput, type ReportField, type ReportSnapshot } from './report'
import { useProblemReportText } from './i18n'

const props = defineProps<ReportInput>()
const text = useProblemReportText()
const snapshot = ref<ReportSnapshot>()
const expected = ref('')
const actual = ref('')
const removed = ref<ReportField[]>([])
const copying = ref(false)
const feedback = ref<'copied' | 'copyFailed' | 'downloaded' | 'downloadFailed'>()
const preview = computed(() => snapshot.value ? buildReport(snapshot.value, removed.value, expected.value, actual.value) : '')
let generation = 0

function open(): void {
  generation++
  expected.value = ''
  actual.value = ''
  removed.value = []
  feedback.value = undefined
  copying.value = false
  snapshot.value = captureReport(props)
}
function close(): void {
  generation++
  snapshot.value = undefined
  expected.value = actual.value = ''
  removed.value = []
  feedback.value = undefined
  copying.value = false
}
function toggle(field: ReportField): void {
  removed.value = removed.value.includes(field) ? removed.value.filter((value) => value !== field) : [...removed.value, field]
  feedback.value = undefined
}
async function copy(): Promise<void> {
  if (copying.value) return
  const current = generation
  const value = preview.value
  copying.value = true
  feedback.value = undefined
  try {
    await navigator.clipboard.writeText(value)
    if (generation === current && preview.value === value) feedback.value = 'copied'
  } catch {
    if (generation === current) feedback.value = 'copyFailed'
  } finally {
    if (generation === current) copying.value = false
  }
}
function download(): void {
  let url: string | undefined
  try {
    url = URL.createObjectURL(new Blob([preview.value], { type: 'application/json;charset=utf-8' }))
    const link = document.createElement('a')
    link.href = url
    link.download = 'kpanel-problem-report.json'
    link.click()
    feedback.value = 'downloaded'
  } catch {
    feedback.value = 'downloadFailed'
  } finally {
    if (url) URL.revokeObjectURL(url)
  }
}
</script>

<template>
  <button class="button button--secondary button--small problem-report-open" type="button" @click="open">
    <FileText :size="16" aria-hidden="true" />{{ text('open') }}
  </button>
  <ModalDialog :open="Boolean(snapshot)" :title="text('title')" size="medium" @close="close">
    <div v-if="snapshot" class="problem-report">
      <p class="problem-report__privacy">{{ text('privacy') }}</p>
      <label class="problem-report__input">
        <span>{{ text('expected') }}</span>
        <textarea v-model="expected" data-report-input="expected" :maxlength="MAX_DESCRIPTION_LENGTH" :placeholder="text('optional')" rows="2" @input="feedback = undefined" />
      </label>
      <label class="problem-report__input">
        <span>{{ text('actual') }}</span>
        <textarea v-model="actual" data-report-input="actual" :maxlength="MAX_DESCRIPTION_LENGTH" :placeholder="text('optional')" rows="2" @input="feedback = undefined" />
      </label>
      <details class="problem-report__details">
        <summary>{{ text('details') }}</summary>
        <p>{{ text('detailsHint') }}</p>
        <label v-for="field in REPORT_FIELDS" :key="field" class="problem-report__field">
          <input type="checkbox" :checked="!removed.includes(field)" :data-report-field="field" @change="toggle(field)" />
          <span>{{ text(field) }}</span><code data-i18n-ignore>{{ snapshot[field] ?? text('missing') }}</code>
        </label>
      </details>
      <label class="problem-report__input">
        <span>{{ text('preview') }}</span>
        <textarea class="problem-report__preview" data-report-preview data-i18n-ignore :value="preview" readonly rows="8" spellcheck="false" />
      </label>
      <p class="problem-report__note">{{ text('snapshot') }}</p>
      <p v-if="feedback" class="problem-report__feedback" role="status">{{ text(feedback) }}</p>
    </div>
    <template #footer>
      <div class="problem-report__actions">
        <button class="button button--secondary" type="button" @click="close">{{ text('close') }}</button>
        <button class="button button--secondary" type="button" data-report-download @click="download"><Download :size="16" aria-hidden="true" />{{ text('download') }}</button>
        <button class="button button--primary" type="button" data-report-copy :disabled="copying" @click="copy"><ClipboardCopy :size="16" aria-hidden="true" />{{ text(copying ? 'copying' : 'copy') }}</button>
      </div>
    </template>
  </ModalDialog>
</template>

<style scoped>
.problem-report-open { font-size: 0.875rem; }
.problem-report { display: grid; gap: 1rem; font-size: 0.875rem; line-height: 1.5; min-width: 0; }
.problem-report p { margin: 0; }
.problem-report__privacy { color: var(--text-soft); }
.problem-report__input { display: grid; gap: 0.5rem; min-width: 0; }
.problem-report textarea { width: 100%; min-width: 0; resize: vertical; font-size: 0.875rem; line-height: 1.5; padding: 0.625rem; color: var(--text); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm); }
.problem-report__details summary { cursor: pointer; padding: 0.25rem 0; }
.problem-report__details p, .problem-report__note { color: var(--text-soft); font-size: 0.8125rem; }
.problem-report__details p { margin: 0.5rem 0; }
.problem-report__field { display: grid; grid-template-columns: 1.5rem minmax(0, 1fr); align-items: start; gap: 0.375rem; padding: 0.375rem 0; }
.problem-report__field input { width: 1.25rem; height: 1.25rem; margin: 0; accent-color: var(--brand); }
.problem-report__field code { grid-column: 2; font-size: 0.8125rem; overflow-wrap: anywhere; color: var(--text-soft); }
.problem-report__feedback { color: var(--text); }
.problem-report__actions { display: flex; flex: 1; flex-wrap: wrap; justify-content: flex-end; gap: 0.5rem; min-width: 0; }
.problem-report__actions .button { flex: 0 1 auto; max-width: 100%; font-size: 0.875rem; overflow-wrap: anywhere; }
</style>
