<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, Info, LoaderCircle, RefreshCw, Search } from '@lucide/vue'
import { ApiError, api } from '@/lib/api'
import { clusterRemoteAccessChanged } from '@/lib/clusterSettings'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import type { ClusterHost, ClusterRemoteAccess } from '@/types/api'

usePhraseCatalog(locale => locale === 'en-US'
  ? import('@/i18n/pages/ClusterView/en-US').then(module => module.default)
  : import('@/i18n/pages/ClusterView/zh-TW').then(module => module.default))
const props = withDefaults(defineProps<{ open: boolean; active: boolean; formId: string; hosts?: ClusterHost[] }>(), { hosts: () => [] })
const emit = defineEmits<{ busy: [value: boolean]; ready: [value: boolean]; saved: [] }>()
type Choice = Pick<ClusterRemoteAccess, 'terminalEnabled' | 'filesEnabled'>
type HostDraft = { snapshot?: ClusterRemoteAccess; form: Choice; loading: boolean; error: boolean }
type ReadQueue = { controller: AbortController; active: number; ids: string[] }
const snapshot = ref<ClusterRemoteAccess>()
const form = reactive<Choice>({ terminalEnabled: true, filesEnabled: true })
const drafts = reactive(new Map<string, HostDraft>())
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const message = ref('')
const search = ref('')
const page = ref(1)
const selected = ref<string[]>([])
const batch = reactive({ terminal: '', files: '' })
const pageSize = 8
let reads: ReadQueue | undefined
let sequence = 0
let loadedForOpen = false
let resourceVersion = ''
const remoteHosts = computed(() => props.hosts.filter(host => !host.isLocal))
const filteredHosts = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return remoteHosts.value.filter(host => !query || [host.name, host.id, host.origin].some(value => value.toLocaleLowerCase().includes(query)))
})
const pageCount = computed(() => Math.max(1, Math.ceil(filteredHosts.value.length / pageSize)))
const visibleHosts = computed(() => filteredHosts.value.slice((page.value - 1) * pageSize, page.value * pageSize))
const selectedHosts = computed(() => remoteHosts.value.filter(host => selected.value.includes(host.id)))
const selectionReady = computed(() => selectedHosts.value.every(host => drafts.get(host.id)?.snapshot?.available && !drafts.get(host.id)?.error))
const canApply = computed(() => !saving.value && !!snapshot.value?.available && selectedHosts.value.length > 0 &&
  !!(batch.terminal || batch.files) && selectionReady.value)
function changed(choice: Choice, original: Choice): boolean {
  return choice.terminalEnabled !== original.terminalEnabled || choice.filesEnabled !== original.filesEnabled
}
const dirtyHosts = computed(() => remoteHosts.value.filter(host => {
  const value = drafts.get(host.id)
  return value?.snapshot?.available && (changed(value.form, value.snapshot) || value.snapshot.pendingTerminalCleanup)
}))
const canSave = computed(() => !!snapshot.value?.available && !loading.value && !saving.value &&
  (changed(form, snapshot.value) || !!snapshot.value.pendingTerminalCleanup || dirtyHosts.value.length > 0))
watch(saving, value => emit('busy', value))
watch(canSave, value => emit('ready', value), { immediate: true })
watch(search, () => { page.value = 1 })
watch(pageCount, value => { page.value = Math.min(page.value, value) })
function phrase(value: string): string { phraseCatalogVersion.value; return translatePhrase(value) }
function selectHosts(invert = false): void {
  const ids = new Set(filteredHosts.value.map(host => host.id))
  selected.value = remoteHosts.value.filter(host => ids.has(host.id) ? !invert || !selected.value.includes(host.id) : selected.value.includes(host.id)).map(host => host.id)
}
function applyBatch(): void {
  if (!canApply.value) return
  for (const host of selectedHosts.value) {
    const draft = drafts.get(host.id)!
    if (batch.terminal) draft.form.terminalEnabled = batch.terminal === 'allow'
    if (batch.files) draft.form.filesEnabled = batch.files === 'allow'
  }
  batch.terminal = ''; batch.files = ''
  message.value = '已应用到所选主机，点击底部「保存设置」后生效。'
}

function enqueueVisible(): void {
  const queue = reads
  if (!queue || !props.open || !props.active || !snapshot.value?.available) return
  for (const host of [...visibleHosts.value, ...selectedHosts.value]) {
    if (drafts.has(host.id)) continue
    drafts.set(host.id, { form: { terminalEnabled: true, filesEnabled: true }, loading: true, error: false })
    queue.ids.push(host.id)
  }
  pump(queue)
}
function pump(queue: ReadQueue): void {
  if (reads !== queue || queue.controller.signal.aborted) return
  while (queue.active < 4 && queue.ids.length) {
    const id = queue.ids.shift()!
    queue.active++
    void api.cluster.hostRemoteAccess(id, queue.controller.signal).then(value => {
      if (reads !== queue || !props.open) return
      drafts.set(id, { snapshot: value, form: { terminalEnabled: value.terminalEnabled, filesEnabled: value.filesEnabled }, loading: false, error: !value.available })
    }).catch(() => {
      if (reads !== queue || queue.controller.signal.aborted) return
      const draft = drafts.get(id)
      if (draft) { draft.loading = false; draft.error = true }
    }).finally(() => { queue.active--; pump(queue) })
  }
}
function retryHost(id: string): void {
  if (saving.value) return
  drafts.delete(id)
  enqueueVisible()
}
async function load(): Promise<void> {
  if (saving.value) return
  const current = ++sequence
  reads?.controller.abort()
  const queue: ReadQueue = { controller: new AbortController(), active: 0, ids: [] }
  reads = queue
  drafts.clear()
  selected.value = []; batch.terminal = ''; batch.files = ''
  snapshot.value = undefined
  loading.value = true; error.value = ''; message.value = ''
  try {
    const value = await api.cluster.remoteAccess(queue.controller.signal)
    if (current !== sequence || !props.open) return
    snapshot.value = value; resourceVersion = value.resourceVersion
    form.terminalEnabled = value.terminalEnabled; form.filesEnabled = value.filesEnabled
    if (!value.available) error.value = '配置不可读取，远程操作已停用。请检查当前中心的数据目录。'
    enqueueVisible()
  } catch {
    if (current === sequence && !queue.controller.signal.aborted) error.value = '远程操作设置读取失败，请重试。'
  } finally { if (current === sequence) loading.value = false }
}
async function save(): Promise<void> {
  if (!canSave.value || !snapshot.value) return
  saving.value = true; error.value = ''; message.value = ''
  const desired = { ...form }
  const targets = [...dirtyHosts.value]
  let saved = 0
  const saveGlobal = async (choice: Choice): Promise<void> => {
    const value = await api.cluster.updateRemoteAccess({ ...choice, expectedResourceVersion: resourceVersion })
    snapshot.value = value; resourceVersion = value.resourceVersion; saved++
  }
  try {
    // Tighten global policy before host edits; postpone global enabling until all host edits succeed.
    const restricted = { terminalEnabled: snapshot.value.terminalEnabled && desired.terminalEnabled, filesEnabled: snapshot.value.filesEnabled && desired.filesEnabled }
    if (changed(restricted, snapshot.value)) await saveGlobal(restricted)
    for (const host of targets) {
      const draft = drafts.get(host.id)!
      const value = await api.cluster.updateHostRemoteAccess(host.id, { ...draft.form, expectedResourceVersion: resourceVersion })
      draft.snapshot = value; draft.form = { terminalEnabled: value.terminalEnabled, filesEnabled: value.filesEnabled }
      resourceVersion = value.resourceVersion; saved++
    }
    if (changed(desired, snapshot.value) || snapshot.value.pendingTerminalCleanup) await saveGlobal(desired)
    message.value = '远程操作设置已保存。'
  } catch (reason) {
    message.value = saved ? '部分设置已保存，尚未保存的选择已保留。' : ''
    error.value = reason instanceof ApiError && reason.status === 409
      ? '设置已在其他页面更新，请重新读取后再修改。'
      : '远程操作设置保存失败，当前选择尚未保存，请重试。'
  } finally {
    if (saved) { window.dispatchEvent(new Event(clusterRemoteAccessChanged)); emit('saved') }
    saving.value = false
  }
}
watch(() => [props.open, props.active] as const, ([open, active]) => {
  if (open && active && !loadedForOpen) { loadedForOpen = true; void load() }
  else if (!open) { loadedForOpen = false; sequence++; reads?.controller.abort(); reads = undefined; loading.value = false }
}, { immediate: true })
watch(() => [props.active, visibleHosts.value.map(host => host.id).join('\n')], enqueueVisible)
watch(selected, enqueueVisible)
watch(() => remoteHosts.value.map(host => host.id).join('\n'), () => {
  selected.value = selected.value.filter(id => remoteHosts.value.some(host => host.id === id))
})
onBeforeUnmount(() => { sequence++; reads?.controller.abort(); reads = undefined })
</script>

<template>
  <form :id="formId" class="cluster-remote-access" @submit.prevent="save">
    <div v-if="loading && !snapshot" class="cluster-remote-access__loading" role="status"><LoaderCircle class="spin" :size="18" />{{ phrase('正在读取远程操作设置…') }}</div>
    <template v-else>
      <header><h3>{{ phrase('全局总开关') }}</h3><p>{{ phrase('统一控制当前中心对所有远程节点的访问。') }}</p></header>
      <fieldset :disabled="loading || saving || !snapshot?.available">
        <label class="cluster-remote-access__row">
          <span><strong>{{ phrase('允许远程终端') }}</strong><small>{{ phrase('连接远程终端、恢复会话及批量执行命令。') }}</small></span>
          <input v-model="form.terminalEnabled" class="cluster-remote-access__switch" type="checkbox" role="switch" :aria-label="phrase('允许远程终端')" />
        </label>
        <label class="cluster-remote-access__row">
          <span><strong>{{ phrase('允许远程文件管理') }}</strong><small>{{ phrase('浏览、编辑、上传、下载及跨节点文件互传。') }}</small></span>
          <input v-model="form.filesEnabled" class="cluster-remote-access__switch" type="checkbox" role="switch" :aria-label="phrase('允许远程文件管理')" />
        </label>
      </fieldset>
      <section class="cluster-remote-access__hosts" :aria-label="phrase('主机独立设置')">
        <header><h3>{{ phrase('主机独立设置') }}</h3><p>{{ phrase('与总开关共同生效，统一保存。新增主机默认开启。') }}</p></header>
        <label v-if="remoteHosts.length" class="cluster-remote-access__search"><Search :size="16" /><input v-model="search" :disabled="saving" type="search" :aria-label="phrase('搜索主机')" :placeholder="phrase('搜索主机名称或地址')" /></label>
        <fieldset v-if="remoteHosts.length" class="cluster-remote-access__batch" :disabled="saving || !snapshot?.available">
          <div class="cluster-remote-access__selection">
            <button class="button button--secondary button--small" type="button" :disabled="!filteredHosts.length" @click="selectHosts()">{{ phrase('全选') }}</button>
            <button class="button button--secondary button--small" type="button" :disabled="!filteredHosts.length" @click="selectHosts(true)">{{ phrase('反选') }}</button>
            <button class="button button--ghost button--small" type="button" :disabled="!selected.length" @click="selected = []">{{ phrase('清空选择') }}</button>
            <span role="status">{{ phrase('已选 {0} 台').replace('{0}', String(selectedHosts.length)) }}</span>
          </div>
          <p v-if="pageCount > 1 || search" class="cluster-remote-access__hint">{{ phrase('全选与反选作用于当前搜索结果，选择跨页保留。') }}</p>
          <p v-if="selectedHosts.length && !selectionReady" class="cluster-remote-access__hint" role="status">{{ phrase('所选主机设置尚未全部读取，请等待或重试失败主机。') }}</p>
          <div v-if="selectedHosts.length" class="cluster-remote-access__batch-fields">
            <label><span>{{ phrase('终端') }}</span><select v-model="batch.terminal" :aria-label="phrase('批量设置终端')"><option value="">{{ phrase('不调整') }}</option><option value="allow">{{ phrase('允许') }}</option><option value="deny">{{ phrase('禁用') }}</option></select></label>
            <label><span>{{ phrase('文件管理') }}</span><select v-model="batch.files" :aria-label="phrase('批量设置文件管理')"><option value="">{{ phrase('不调整') }}</option><option value="allow">{{ phrase('允许') }}</option><option value="deny">{{ phrase('禁用') }}</option></select></label>
            <button class="button button--secondary button--small" type="button" :disabled="!canApply" @click="applyBatch">{{ phrase('应用到所选主机') }}</button>
          </div>
          <p v-if="batch.terminal || batch.files" class="cluster-remote-access__hint">{{ phrase('先应用到所选主机，再保存设置。') }}</p>
        </fieldset>
        <p v-if="!form.terminalEnabled || !form.filesEnabled" class="cluster-remote-access__hint">{{ phrase('总开关关闭的项目暂不生效；主机选择仍会保留。') }}</p>
        <p v-if="!remoteHosts.length" class="cluster-remote-access__hint">{{ phrase('暂无远程主机，接入后可在此统一管理。') }}</p>
        <p v-else-if="!filteredHosts.length" class="cluster-remote-access__hint">{{ phrase('未找到匹配的主机。') }}</p>
        <div v-else class="cluster-remote-access__host-list">
          <div class="cluster-remote-access__columns" aria-hidden="true"><span>{{ phrase('主机') }}</span><span>{{ phrase('终端') }}</span><span>{{ phrase('文件管理') }}</span></div>
          <div v-for="host in visibleHosts" :key="host.id" class="cluster-remote-access__host" :data-host-id="host.id" role="group" :aria-label="host.name">
            <label class="cluster-remote-access__identity"><input v-model="selected" type="checkbox" :value="host.id" :disabled="saving || !snapshot?.available" :aria-label="phrase('选择主机') + ' · ' + host.name" /><span><strong>{{ host.name }}</strong><small>{{ host.origin || host.id }}</small></span></label>
            <div v-if="drafts.get(host.id)?.error" class="cluster-remote-access__host-status" role="alert"><span>{{ phrase('主机设置读取失败') }}</span><button class="button button--ghost button--small" type="button" :disabled="saving" @click="retryHost(host.id)">{{ phrase('重试') }}</button></div>
            <div v-else-if="!drafts.get(host.id)?.snapshot" class="cluster-remote-access__host-status" role="status"><LoaderCircle class="spin" :size="16" />{{ phrase('正在读取…') }}</div>
            <template v-else>
              <label class="cluster-remote-access__host-toggle"><span>{{ phrase('终端') }}</span><input v-model="drafts.get(host.id)!.form.terminalEnabled" class="cluster-remote-access__switch" type="checkbox" role="switch" :aria-label="host.name + ' · ' + phrase('允许远程终端')" :disabled="saving || !snapshot?.available" /></label>
              <label class="cluster-remote-access__host-toggle"><span>{{ phrase('文件管理') }}</span><input v-model="drafts.get(host.id)!.form.filesEnabled" class="cluster-remote-access__switch" type="checkbox" role="switch" :aria-label="host.name + ' · ' + phrase('允许远程文件管理')" :disabled="saving || !snapshot?.available" /></label>
            </template>
          </div>
        </div>
        <nav v-if="pageCount > 1" class="cluster-remote-access__pagination" :aria-label="phrase('主机分页')">
          <button class="button button--secondary button--small" type="button" :aria-label="phrase('上一页')" :disabled="page === 1 || saving" @click="page--"><ChevronLeft :size="16" /></button>
          <span aria-live="polite">{{ page }} / {{ pageCount }}</span>
          <button class="button button--secondary button--small" type="button" :aria-label="phrase('下一页')" :disabled="page === pageCount || saving" @click="page++"><ChevronRight :size="16" /></button>
        </nav>
        <p v-if="dirtyHosts.length" class="cluster-remote-access__hint" role="status">{{ phrase('待保存主机：{0} 台（含其他页与搜索外的修改）').replace('{0}', String(dirtyHosts.length)) }}</p>
      </section>
      <aside class="cluster-remote-access__scope"><Info :size="18" /><p>{{ phrase('本机管理与集群监控不受影响，不修改系统 SSH 服务。') }}<br>{{ phrase('关闭后停止远程访问，并取消可取消的连接与传输；已完成的操作不会撤销。') }}</p></aside>
      <p class="cluster-remote-access__hint">{{ phrase('重新开启无需重新配对，不会自动恢复命令或传输。') }}</p>
      <p v-if="message" class="cluster-remote-access__message" role="status" aria-live="polite">{{ phrase(message) }}</p>
      <p v-if="error" class="cluster-remote-access__error" role="alert">{{ phrase(error) }}</p>
      <button v-if="error" type="button" class="button button--secondary button--small" :disabled="saving || loading" @click="load"><RefreshCw :size="14" />{{ phrase('重新读取设置') }}</button>
    </template>
  </form>
</template>

<style scoped>
.cluster-remote-access { display:grid; gap:16px; min-width:0; }
.cluster-remote-access header h3 { margin:0 0 6px; font-size:16px; }
.cluster-remote-access header p, .cluster-remote-access__hint { margin:0; color:var(--muted); font-size:13px; line-height:1.6; }
.cluster-remote-access fieldset { margin:0; padding:0; border:0; min-width:0; }
.cluster-remote-access__row { display:flex; align-items:center; justify-content:space-between; gap:20px; padding:14px 0; cursor:pointer; }
.cluster-remote-access__row + .cluster-remote-access__row { border-top:1px solid var(--border); }
.cluster-remote-access__row > span { min-width:0; }
.cluster-remote-access__row strong { display:block; font-size:14px; }
.cluster-remote-access__row small { display:block; margin-top:6px; color:var(--muted); font-size:13px; line-height:1.6; }
.cluster-remote-access__switch { appearance:none; position:relative; flex:0 0 40px; width:40px; height:24px; margin:0; border:1px solid var(--border-strong); border-radius:999px; background:var(--muted); cursor:pointer; }
.cluster-remote-access__switch::after { content:''; position:absolute; top:3px; left:3px; width:16px; height:16px; border-radius:50%; background:var(--surface); transition:transform var(--motion-duration-fast); }
.cluster-remote-access__switch:checked { border-color:var(--brand); background:var(--brand); }
.cluster-remote-access__switch:checked::after { transform:translateX(16px); }
.cluster-remote-access__switch:focus-visible { outline:2px solid var(--brand); outline-offset:3px; }
.cluster-remote-access__switch:disabled { opacity:.5; cursor:default; }
.cluster-remote-access__hosts { display:grid; gap:12px; padding-top:18px; border-top:1px solid var(--border); min-width:0; }
.cluster-remote-access__search { display:flex; align-items:center; gap:8px; color:var(--muted); }
.cluster-remote-access__search input { box-sizing:border-box; min-width:0; width:100%; min-height:38px; padding:8px 12px; border:1px solid var(--border); border-radius:var(--radius-sm); background:var(--surface); color:var(--text); font:inherit; font-size:14px; }
.cluster-remote-access__search svg { flex-shrink:0; }
.cluster-remote-access fieldset.cluster-remote-access__batch { display:grid; gap:10px; }
.cluster-remote-access__selection { display:flex; align-items:center; flex-wrap:wrap; gap:8px; }
.cluster-remote-access__selection > span { margin-left:auto; color:var(--muted); font-size:13px; }
.cluster-remote-access__batch-fields { display:flex; align-items:flex-end; flex-wrap:wrap; gap:10px; }
.cluster-remote-access__batch-fields label { display:grid; flex:1 1 110px; gap:6px; font-size:14px; }
.cluster-remote-access__batch-fields select { box-sizing:border-box; width:100%; min-width:0; min-height:38px; padding:8px 10px; border:1px solid var(--border); border-radius:var(--radius-sm); background:var(--surface); color:var(--text); font:inherit; font-size:14px; }
.cluster-remote-access__batch-fields > button { min-height:36px; }
.cluster-remote-access__host-list { border:1px solid var(--border); border-radius:var(--radius); background:var(--surface-subtle); }
.cluster-remote-access__columns, .cluster-remote-access__host { display:grid; grid-template-columns:minmax(0, 1fr) 72px 88px; gap:8px; align-items:center; padding:12px; }
.cluster-remote-access__columns { color:var(--muted); font-size:13px; }
.cluster-remote-access__columns > span:not(:first-child) { text-align:center; }
.cluster-remote-access__host { border-top:1px solid var(--border); }
.cluster-remote-access__identity { display:flex; align-items:center; gap:10px; min-width:0; overflow-wrap:anywhere; cursor:pointer; }
.cluster-remote-access__identity > input { flex:none; width:16px; height:16px; margin:0; accent-color:var(--brand); }
.cluster-remote-access__identity > span { min-width:0; }
.cluster-remote-access__identity strong { display:block; font-size:14px; line-height:1.5; }
.cluster-remote-access__identity small { display:block; margin-top:3px; color:var(--muted); font-size:13px; line-height:1.5; }
.cluster-remote-access__host-toggle { display:flex; align-items:center; justify-content:center; gap:8px; cursor:pointer; }
.cluster-remote-access__host-toggle > span { display:none; font-size:14px; }
.cluster-remote-access__host-status { display:flex; flex-wrap:wrap; align-items:center; gap:8px; grid-column:2 / -1; font-size:13px; color:var(--muted); }
.cluster-remote-access__pagination { display:flex; justify-content:flex-end; align-items:center; gap:12px; font-size:13px; }
.cluster-remote-access__scope { display:flex; gap:10px; padding:12px; border:1px solid var(--border); border-radius:var(--radius); background:var(--surface-subtle); color:var(--text-soft); }
.cluster-remote-access__scope svg { flex-shrink:0; margin-top:2px; color:var(--brand-strong); }
.cluster-remote-access__scope p { margin:0; font-size:13px; line-height:1.6; }
.cluster-remote-access__loading { display:flex; align-items:center; gap:8px; padding:24px 0; }
.cluster-remote-access__message { margin:0; font-size:14px; color:var(--text); }
.cluster-remote-access__error { margin:0; font-size:14px; color:var(--danger); }
@media (max-width:480px) {
  .cluster-remote-access__columns { display:none; }
  .cluster-remote-access__host { grid-template-columns:repeat(2, minmax(0, 1fr)); gap:14px; }
  .cluster-remote-access__host:first-child { border-top:0; }
  .cluster-remote-access__identity { grid-column:1 / -1; }
  .cluster-remote-access__host-toggle { justify-content:space-between; }
  .cluster-remote-access__host-toggle > span { display:inline; }
  .cluster-remote-access__host-status { grid-column:1 / -1; }
}
@media (prefers-reduced-motion:reduce) { .cluster-remote-access__switch::after { transition:none; } }
@media (forced-colors:active) { .cluster-remote-access__switch { appearance:auto; } .cluster-remote-access__switch::after { display:none; } }
</style>
