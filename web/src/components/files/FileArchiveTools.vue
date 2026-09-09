<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Archive, ArrowLeft, Folder, File, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError } from '@/lib/api'
import { fileAPIForHost } from '@/lib/fileHostContext'
import type { FileActionInput, FileArchiveDirectory, FileArchiveJob, FileEntry } from '@/types/api'

const props = defineProps<{ hostId: string; path: string }>()
const emit = defineEmits<{ changed: [hostId: string, path: string]; open: [hostId: string, path: string] }>()
const i18n = useI18n()
const jobs = ref<FileArchiveJob[]>([])
const available = ref(false)
const jobsError = ref('')
const expanded = ref(false)
const pending = ref(new Set<string>())
const browser = ref<FileEntry>()
const directory = ref('')
const search = ref('')
const contents = ref<FileArchiveDirectory>()
const pageOffset = ref(0)
const selection = ref(new Set<string>())
const loading = ref(false)
const browseError = ref('')
const form = ref<{ action: 'compress' | 'extract'; sources: FileEntry[]; hostId: string; members?: string[] }>()
const destination = ref('')
const name = ref('')
const format = ref<'tar.gz' | 'zip' | 'tar'>('tar.gz')
const submitting = ref(false)
const formError = ref('')
let disposed = false
let polling: ReturnType<typeof setTimeout> | undefined
let searching: ReturnType<typeof setTimeout> | undefined
let jobsController: AbortController | undefined
let contentsController: AbortController | undefined
let generation = 0
let browseGeneration = 0
let discoveryFailures = 0
const batch = computed(() => form.value?.action === 'extract' && form.value.sources.length > 1)
const entries = computed(() => contents.value?.entries || [])
const allSelected = computed(() => entries.value.length > 0 && entries.value.every(entry => selection.value.has(entry.path)))
const active = (job: FileArchiveJob) => ['queued', 'running', 'cancelling'].includes(job.state)
const baseName = (value: string) => value.replace(/\.(tar\.gz|tgz|zip|tar)$/i, '') || 'archive'
const size = (bytes: number) => bytes < 1024 ? `${bytes} B` : bytes < 1048576 ? `${(bytes / 1024).toFixed(1)} KiB` : bytes < 1073741824 ? `${(bytes / 1048576).toFixed(1)} MiB` : `${(bytes / 1073741824).toFixed(2)} GiB`
const archiveDetails = new Set([
  '文件状态已变化，请刷新后重试', '目标已存在，请修改名称后重试', '压缩包超过条目或容量上限',
  '压缩包格式或内容无效', '操作已停止，请核对目标目录', '操作超时，请核对目标目录',
  '没有目标目录的读写权限', '文件或目标目录已不存在', '文件操作未完成，请检查目标目录、可用空间及文件权限',
  'Agent 已重启，请核对目标目录后重试', 'Agent 已重启，暂存清理未完成，请核对目标目录',
  '任务状态保存失败，请核对目标目录', '任务意外停止，请核对目标目录后重试',
  '任务已停止，请核对目标目录中的已完成结果', '暂时无法确认归档状态，请刷新后重试',
])
function detail(value: string) { phraseCatalogVersion.value; return archiveDetails.has(value) ? translatePhrase(value) : value }
const errorDetail = (error: unknown) => error instanceof ApiError && [404, 405].includes(error.status)
  ? i18n.t('files.archive.unsupported')
  : error instanceof Error ? detail(error.message) : i18n.t('files.archive.error')
const stateLabel = (job: FileArchiveJob) => i18n.t(`files.archive.state.${job.state}`)

async function refreshJobs() {
  clearTimeout(polling)
  jobsController?.abort()
  const current = new AbortController(); jobsController = current
  const host = props.hostId; const token = generation
  try {
    const result = await fileAPIForHost(host).archiveJobs(current.signal)
    if (disposed || current.signal.aborted || token !== generation) return
    const old = new Map(jobs.value.map(job => [job.id, job]))
    for (const job of result.items) {
      if (old.has(job.id) && active(old.get(job.id)!) && !active(job)) emit('changed', host, job.target)
    }
    available.value = true; jobs.value = result.items; jobsError.value = ''; discoveryFailures = 0
  } catch (error) {
    if (disposed || current.signal.aborted || token !== generation) return
    const unsupported = error instanceof ApiError && [404, 405].includes(error.status)
    if (!unsupported || jobs.value.length) jobsError.value = errorDetail(error)
    if (unsupported) available.value = false
    else ++discoveryFailures
  } finally {
    if (!disposed && token === generation && !current.signal.aborted && (jobs.value.some(active) || (jobsError.value && discoveryFailures <= 3))) {
      polling = setTimeout(refreshJobs, jobsError.value ? 10_000 : 2_500)
    }
  }
}

async function loadContents(offset = 0) {
  if (!browser.value) return
  contentsController?.abort()
  const current = new AbortController(); contentsController = current
  const host = props.hostId; const source = browser.value; const token = ++browseGeneration
  loading.value = true; browseError.value = ''; selection.value = new Set()
  try {
    const result = await fileAPIForHost(host).archiveContents({ path: source.path, resourceVersion: source.resourceVersion, directory: directory.value, search: search.value, offset }, current.signal)
    if (!disposed && !current.signal.aborted && token === browseGeneration && host === props.hostId) { contents.value = result; pageOffset.value = offset }
  } catch (error) {
    if (!disposed && !current.signal.aborted && token === browseGeneration) { contents.value = undefined; browseError.value = errorDetail(error) }
  } finally { if (token === browseGeneration) loading.value = false }
}
function browse(entry: FileEntry) {
  browser.value = { ...entry }; directory.value = ''; search.value = ''; contents.value = undefined
  void loadContents()
}
function closeBrowser() { browser.value = undefined; contentsController?.abort(); ++browseGeneration; clearTimeout(searching) }
function navigate(value: string) { clearTimeout(searching); directory.value = value; search.value = ''; void loadContents() }
function searchContents() { clearTimeout(searching); contentsController?.abort(); ++browseGeneration; loading.value = true; selection.value = new Set(); searching = setTimeout(() => loadContents(), 250) }
function toggle(path: string) { const next = new Set(selection.value); next.has(path) ? next.delete(path) : next.add(path); selection.value = next }
function toggleAll() { selection.value = allSelected.value ? new Set() : new Set(entries.value.map(entry => entry.path)) }
function configure(action: 'compress' | 'extract', sources: FileEntry[], members?: string[]) {
  if (!sources.length) return
  form.value = { action, sources: sources.map(source => ({ ...source })), hostId: props.hostId, members }
  destination.value = props.path; format.value = 'tar.gz'
  name.value = action === 'compress' ? `${sources.length === 1 ? sources[0]!.name : 'archive'}.tar.gz` : baseName(sources[0]!.name)
  formError.value = ''
}
async function extractContents() {
  const source = browser.value; const token = generation
  if (!source) return
  const members = selection.value.size ? [...selection.value] : undefined
  closeBrowser()
  // Let the shared dialog restore the file opener before the next dialog captures it.
  await nextTick(); await nextTick()
  if (!disposed && token === generation) configure('extract', [source], members)
}
function changeFormat() { name.value = `${baseName(name.value)}.${format.value}` }
async function submit() {
  if (!form.value || submitting.value) return
  const snapshot = form.value
  const saveName = name.value.trim(); const target = destination.value.trim()
  if ((!batch.value && (!saveName || /[/\\\u0000]/.test(saveName) || ['.', '..'].includes(saveName))) || !target.startsWith('/')) { formError.value = i18n.t('files.archive.invalid'); return }
  const input: FileActionInput = { action: snapshot.action, sources: snapshot.sources.map(source => source.path), target, name: saveName,
    format: format.value, archiveEntries: snapshot.members, expectedResourceVersions: Object.fromEntries(snapshot.sources.map(source => [source.path, source.resourceVersion])) }
  submitting.value = true; formError.value = ''
  try {
    const job = await fileAPIForHost(snapshot.hostId).createArchiveJob(input)
    if (!disposed && snapshot.hostId === props.hostId) {
      jobs.value = [job, ...jobs.value.filter(item => item.id !== job.id)]; available.value = true; form.value = undefined
      await refreshJobs()
    }
  } catch (error) { if (!disposed && form.value === snapshot) formError.value = errorDetail(error) }
  finally { submitting.value = false }
}
async function changeJob(job: FileArchiveJob, operation: 'cancel' | 'clear') {
  const host = props.hostId; const token = generation
  pending.value = new Set(pending.value).add(job.id)
  try { await fileAPIForHost(host).changeArchiveJob(job.id, operation); if (token === generation) await refreshJobs() }
  catch (error) { if (token === generation) jobsError.value = errorDetail(error) }
  finally { if (token === generation) { const next = new Set(pending.value); next.delete(job.id); pending.value = next } }
}
async function retry(job: FileArchiveJob) {
  const host = props.hostId; const token = generation
  const completed = new Set(job.result.succeeded.map(item => item.path))
  const paths = job.action === 'extract' ? job.sources.filter(path => !completed.has(path)) : job.sources
  pending.value = new Set(pending.value).add(job.id)
  try {
    const entries: FileEntry[] = []
    for (let index = 0; index < paths.length; index += 64) {
      const sources = await fileAPIForHost(host).entries(paths.slice(index, index + 64))
      if (disposed || token !== generation) return
      if (sources.unavailable.length) { jobsError.value = i18n.t('files.archive.sourceChanged'); return }
      entries.push(...sources.entries)
    }
    configure(job.action, entries, job.archiveEntries); destination.value = job.target
    if (job.action === 'compress' || job.sources.length === 1) name.value = job.name
    if (job.format) format.value = job.format
  } catch (error) { if (token === generation) jobsError.value = errorDetail(error) }
  finally { if (token === generation) { const next = new Set(pending.value); next.delete(job.id); pending.value = next } }
}
watch(() => props.hostId, () => {
  ++generation; jobsController?.abort(); clearTimeout(polling); closeBrowser(); form.value = undefined
  jobs.value = []; jobsError.value = ''; pending.value = new Set(); available.value = false; discoveryFailures = 0; void refreshJobs()
}, { immediate: true })
onBeforeUnmount(() => { disposed = true; ++generation; jobsController?.abort(); contentsController?.abort(); clearTimeout(polling); clearTimeout(searching) })
const hasJobs = computed(() => jobs.value.length > 0 || Boolean(jobsError.value))
defineExpose({ configure, browse, available, hasJobs })
</script>

<template>
  <section v-if="hasJobs" class="archive-jobs" :aria-label="i18n.t('files.archive.jobs')">
    <header><strong>{{ i18n.t('files.archive.jobs') }}</strong><span>{{ i18n.t('files.archive.background') }}</span><button type="button" class="button button--secondary" :aria-label="i18n.t('files.archive.refresh')" @click="refreshJobs"><RefreshCw :size="15" /></button></header>
    <p v-if="jobsError" class="archive-error" role="alert">{{ jobsError }}</p>
    <div v-for="job in expanded ? jobs : jobs.slice(0, 3)" :key="job.id" class="archive-job">
      <Archive :size="18" /><div class="archive-job__body"><strong>{{ job.name }}</strong><p role="status">{{ stateLabel(job) }}<template v-if="active(job)"> · {{ size(job.processedBytes) }} · {{ i18n.t('files.archive.count', { count: job.entries }) }}</template></p>
        <details v-if="job.detail || job.result.failed.length || job.result.succeeded.length"><summary>{{ i18n.t('files.archive.details') }}</summary><p v-if="job.detail">{{ detail(job.detail) }}</p><p v-for="item in job.result.failed" :key="item.path" class="archive-error"><span data-i18n-ignore>{{ item.path }}</span> · {{ detail(item.detail) }}</p><p v-for="item in job.result.succeeded" :key="item.path" data-i18n-ignore>{{ item.destination }}</p></details>
      </div><div class="archive-job__actions"><button v-if="active(job)" type="button" class="button button--secondary" :disabled="pending.has(job.id) || job.state === 'cancelling'" @click="changeJob(job, 'cancel')">{{ i18n.t('files.archive.stop') }}</button><template v-else><button v-if="job.result.succeeded.length" type="button" class="button button--secondary" @click="emit('open', props.hostId, job.target)">{{ i18n.t('files.archive.openResult') }}</button><button v-if="job.state !== 'complete'" type="button" class="button button--secondary" :disabled="pending.has(job.id)" @click="retry(job)">{{ i18n.t('files.archive.retryFailed') }}</button><button type="button" class="button button--secondary" :disabled="pending.has(job.id)" @click="changeJob(job, 'clear')">{{ i18n.t('files.archive.clear') }}</button></template></div>
    </div>
    <button v-if="jobs.length > 3" type="button" class="button button--secondary" :aria-expanded="expanded" @click="expanded = !expanded">{{ i18n.t(expanded ? 'files.archive.collapse' : 'files.archive.showAll', { count: jobs.length }) }}</button>
  </section>

  <ModalDialog :open="Boolean(browser)" :title="browser?.name || i18n.t('files.archive.title')" :description="i18n.t('files.archive.browseDescription')" size="large" :allow-fullscreen="true" @close="closeBrowser">
    <div class="archive-browser">
      <div class="archive-toolbar"><button type="button" class="button button--secondary" :disabled="!directory || loading" @click="navigate(directory.includes('/') ? directory.slice(0, directory.lastIndexOf('/')) : '')"><ArrowLeft :size="16" />{{ i18n.t('files.archive.parent') }}</button><button type="button" class="archive-root" @click="navigate('')">{{ i18n.t('files.archive.root') }}</button><span>{{ directory }}</span><button type="button" class="button button--primary" :disabled="loading || Boolean(browseError) || selection.size > 100" @click="extractContents">{{ selection.size ? i18n.t('files.archive.extractSelected', { count: selection.size }) : i18n.t('files.archive.extractAll') }}</button></div>
      <p v-if="selection.size > 100" class="archive-error" role="alert">{{ i18n.t('files.archive.selectionLimit') }}</p>
      <input v-model="search" type="search" :placeholder="i18n.t('files.archive.search')" :aria-label="i18n.t('files.archive.search')" @input="searchContents">
      <p v-if="loading" role="status">{{ i18n.t('files.archive.loading') }}</p>
      <div v-else-if="browseError" class="archive-error" role="alert">{{ browseError }} <button type="button" class="button button--secondary" @click="loadContents()">{{ i18n.t('files.archive.refresh') }}</button></div>
      <template v-else><table><thead><tr><th><input type="checkbox" :checked="allSelected" :aria-label="i18n.t('files.archive.selectAll')" @change="toggleAll"></th><th>{{ i18n.t('files.archive.name') }}</th><th>{{ i18n.t('files.archive.size') }}</th></tr></thead><tbody><tr v-for="entry in entries" :key="entry.path" :class="{ selected: selection.has(entry.path) }"><td><input type="checkbox" :checked="selection.has(entry.path)" :aria-label="entry.path" @change="toggle(entry.path)"></td><td><div class="archive-entry"><Folder v-if="entry.kind === 'directory'" :size="18" /><File v-else :size="18" /><button v-if="entry.kind === 'directory'" type="button" class="archive-root" @click="navigate(entry.path)">{{ search ? entry.path : entry.name }}</button><span v-else>{{ search ? entry.path : entry.name }}</span></div></td><td>{{ entry.kind === 'directory' ? '—' : size(entry.sizeBytes) }}</td></tr></tbody></table><p v-if="!entries.length">{{ i18n.t('files.archive.empty') }}</p>
        <footer><span>{{ i18n.t('files.archive.count', { count: contents?.total || 0 }) }}</span><button v-if="pageOffset" type="button" class="button button--secondary" @click="loadContents(Math.max(0, pageOffset - 500))">{{ i18n.t('files.archive.previous') }}</button><button v-if="contents?.truncated" type="button" class="button button--secondary" @click="loadContents(contents.nextOffset)">{{ i18n.t('files.archive.next') }}</button></footer>
      </template>
    </div>
  </ModalDialog>

  <ModalDialog :open="Boolean(form)" :title="form?.action === 'compress' ? i18n.t('files.archive.createTitle') : i18n.t('files.archive.extractTitle')" size="small" @close="form = undefined">
    <form class="archive-form" @submit.prevent="submit">
      <p>{{ i18n.t('files.archive.sources', { count: form?.members?.length || form?.sources.length || 0 }) }}</p>
      <label><span>{{ i18n.t('files.archive.destination') }}</span><input v-model="destination" required :disabled="submitting"></label>
      <label v-if="!batch"><span>{{ form?.action === 'compress' ? i18n.t('files.archive.archiveName') : i18n.t('files.archive.folderName') }}</span><input v-model="name" required :disabled="submitting"></label>
      <p v-else>{{ i18n.t('files.archive.batchFolders') }}</p>
      <label v-if="form?.action === 'compress'"><span>{{ i18n.t('files.archive.format') }}</span><select v-model="format" :disabled="submitting" @change="changeFormat"><option value="tar.gz">TAR.GZ</option><option value="zip">ZIP</option><option value="tar">TAR</option></select></label>
      <small>{{ i18n.t('files.archive.background') }} {{ i18n.t('files.archive.noOverwrite') }}</small>
      <p v-if="formError" class="archive-error" role="alert">{{ formError }}</p>
      <footer><button class="button button--secondary" type="button" @click="form = undefined">{{ i18n.t('files.archive.close') }}</button><button class="button button--primary" type="submit" :disabled="submitting">{{ submitting ? i18n.t('files.archive.submitting') : i18n.t('files.archive.start') }}</button></footer>
    </form>
  </ModalDialog>
</template>

<style scoped>
.archive-jobs{background:var(--surface-subtle);border:1px solid var(--border);border-radius:var(--radius);padding:12px 16px;max-height:290px;overflow:auto;font-size:14px}
.archive-jobs>header,.archive-toolbar,.archive-job,.archive-job__actions,.archive-entry,footer{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.archive-jobs>header>span{font-size:13px;color:var(--text-soft);flex:1}
.archive-job{padding:12px 0;border-top:1px solid var(--border);margin-top:10px;align-items:flex-start}
.archive-job>svg,.archive-entry>svg{color:var(--brand);flex-shrink:0}
.archive-job__body{flex:1;min-width:160px;overflow-wrap:anywhere}
.archive-job__body p{margin:4px 0;font-size:13px;color:var(--text-soft)}
.archive-job__body strong{font-weight:500}
.archive-job__actions .button{font-size:14px;min-height:36px}
.archive-error,.archive-job__body .archive-error{color:var(--danger);overflow-wrap:anywhere}
.archive-browser,.archive-form{font-size:14px;line-height:1.55}
.archive-toolbar{margin-bottom:14px}
.archive-toolbar>span{flex:1;overflow-wrap:anywhere;color:var(--text-soft)}
.archive-root{padding:4px 0;background:none;border:0;color:var(--text);font:inherit;text-align:left;cursor:pointer;overflow-wrap:anywhere}
.archive-root:focus-visible{outline:2px solid var(--brand);outline-offset:3px}
.archive-browser input[type=search],.archive-form input,.archive-form select{width:100%;padding:10px 12px;background:var(--surface);color:var(--text);border:1px solid var(--border);border-radius:var(--radius-sm);font:inherit;min-width:0}
.archive-browser input[type=search]{margin-bottom:14px}
.archive-browser table{width:100%;border-collapse:collapse;table-layout:fixed}
.archive-browser th,.archive-browser td{padding:12px 8px;text-align:left;border-bottom:1px solid var(--border);overflow-wrap:anywhere}
.archive-browser th{font-size:13px;color:var(--text-soft);font-weight:500}
.archive-browser th:first-child{width:36px}.archive-browser th:last-child{width:100px}
.archive-browser td:last-child{font-size:13px;color:var(--text-soft)}
.archive-browser input[type=checkbox]{accent-color:var(--brand);width:24px;height:24px}
.archive-browser tr.selected{background:var(--brand-soft)}
.archive-entry{flex-wrap:nowrap}
.archive-browser footer{justify-content:space-between;margin-top:14px;color:var(--text-soft)}
.archive-form label{display:grid;gap:8px;margin:16px 0}.archive-form p{overflow-wrap:anywhere}
.archive-form small{font-size:13px;color:var(--text-soft)}
.archive-form footer{justify-content:flex-end;margin-top:22px}
@media(max-width:540px){.archive-toolbar>.button:last-child{margin-left:auto}.archive-job__actions{width:100%}.archive-browser th:last-child{width:84px}.archive-browser th,.archive-browser td{padding:10px 4px}}
</style>
