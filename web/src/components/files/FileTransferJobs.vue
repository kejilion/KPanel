<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Copy, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { changeTransferJob, createTransferJob, newTransferJobID, observeTransferJobs, refreshTransferJobs, transferJobActive, transferJobs, transferJobsError, transferJobsLoading } from '@/lib/fileTransferJobs'
import type { ClusterHost, FileEntry, FileTransferJob, FileTransferJobInput } from '@/types/api'

const props = defineProps<{ hosts: ClusterHost[]; sourceNodeId: string; localNodeId: string; hostId: string; path: string }>()
const form = ref<FileTransferJobInput>()
const submitting = ref(false)
const formError = ref('')
const actionError = ref('')
const pending = ref(new Set<string>())
const expanded = ref(false)
const speed = ref<Record<string, number>>({})
const samples = new Map<string, { bytes: number; at: number }>()
let unsubscribe: (() => void) | undefined
const phrase = (s: string) => { phraseCatalogVersion.value; return translatePhrase(s) }
const bytes = (n: number) => n < 1024 ? `${n} B` : n < 1048576 ? `${(n / 1024).toFixed(1)} KiB` : n < 1073741824 ? `${(n / 1048576).toFixed(1)} MiB` : `${(n / 1073741824).toFixed(2)} GiB`
const current = (job: FileTransferJob) => job.items.find(item => transferJobActive(item.state) && item.state !== 'queued')
const finishedCount = (job: FileTransferJob) => job.items.filter(item => item.state === 'complete').length
const retryItems = (job: FileTransferJob) => job.items.filter(item => item.state !== 'complete' && item.retryable)
const hostName = (id: string) => props.hosts.find(host => id ? host.id === id : host.isLocal)?.name || (!id ? phrase('本机') : id)
const sourceName = (id: string) => props.hosts.find(host => host.remoteNodeId === id || host.isLocal && id === localNodeID.value)?.name || id
const localNodeID = computed(() => props.localNodeId)
const targets = computed(() => props.hosts.filter(host => (host.isLocal || host.fileManagementAvailable) && (host.isLocal ? localNodeID.value : host.remoteNodeId) !== form.value?.sourceNodeId))
const visibleJobs = computed(() => expanded.value ? transferJobs.value : transferJobs.value.slice(0, 3))
const labels: Record<string, string> = {
  queued: '等待传输', running: '正在复制', connecting: '正在连接', transferring: '正在传输', committing: '正在确认落盘',
  complete: '复制完成', partial: '部分完成', error: '复制失败', cancelled: '已取消', interrupted: '传输已中断',
}
const stage = (job: FileTransferJob) => phrase(labels[current(job)?.state || job.state] || '状态未知')
const targetAvailable = computed(() => !!form.value && targets.value.some(host => (host.isLocal ? '' : host.id) === form.value?.targetHostId))
const validPath = (p: string) => p.startsWith('/') && p.length <= 4096 && !/[\\\x00\r\n]/.test(p) && (p === '/' || !p.slice(1).split('/').some(part => !part || part === '.' || part === '..'))

watch(transferJobs, jobs => {
  const rates: Record<string, number> = {}
  const keys = new Set<string>()
  const at = performance.now()
  for (const job of jobs) {
    const item = current(job)
    if (!item) continue
    const key = `${job.id}:${item.path}`
    keys.add(key)
    const old = samples.get(key)
    rates[job.id] = old && at > old.at ? Math.max(0, item.loadedBytes - old.bytes) * 1000 / (at - old.at) : 0
    samples.set(key, { bytes: item.loadedBytes, at })
  }
  for (const key of samples.keys()) if (!keys.has(key)) samples.delete(key)
  speed.value = rates
})

function openCopy(entries: readonly FileEntry[]): void {
  const items = entries.filter(entry => ['file', 'directory'].includes(entry.kind))
  formError.value = ''
  if (!props.sourceNodeId || !items.length || items.length !== entries.length || items.length > 64) {
    actionError.value = '请选择 1 至 64 个普通文件或目录，并等待主机信息加载。'; return
  }
  form.value = { id: newTransferJobID(), sourceNodeId: props.sourceNodeId, targetHostId: '', targetDirectory: '/home', items: items.map(({ path, resourceVersion }) => ({ path, resourceVersion })) }
  const first = targets.value[0]
  if (first) form.value.targetHostId = first.isLocal ? '' : first.id
}

async function submit(): Promise<void> {
  if (!form.value || submitting.value || !targetAvailable.value || !validPath(form.value.targetDirectory)) return
  submitting.value = true
  formError.value = ''
  try { await createTransferJob({ ...form.value, items: [...form.value.items] }); form.value = undefined; expanded.value = true }
  catch (error) { formError.value = error instanceof Error ? error.message : '提交结果暂时无法确认，请刷新任务记录。' }
  finally { submitting.value = false }
}

async function change(job: FileTransferJob, operation: 'cancel' | 'clear' | 'retry'): Promise<void> {
  if (pending.value.has(job.id)) return
  pending.value = new Set(pending.value).add(job.id)
  actionError.value = ''
  try {
    if (operation === 'retry') {
      const items = retryItems(job).map(({ path, resourceVersion }) => ({ path, resourceVersion }))
      if (!items.length) return
      await createTransferJob({ id: newTransferJobID(), retryOf: job.id, sourceNodeId: job.sourceNodeId, targetHostId: job.targetHostId, targetDirectory: job.targetDirectory, items })
      await refreshTransferJobs()
    } else await changeTransferJob(job.id, operation)
  } catch (error) { actionError.value = error instanceof Error ? error.message : '任务操作失败，请刷新重试。' }
  finally { const next = new Set(pending.value); next.delete(job.id); pending.value = next }
}

onMounted(() => { unsubscribe = observeTransferJobs() })
onBeforeUnmount(() => { unsubscribe?.() })
const hasActivity = computed(() => Boolean(transferJobs.value.length || transferJobsError.value || actionError.value))
defineExpose({ openCopy, hasActivity })
</script>

<template>
  <section v-if="transferJobs.length || transferJobsError || actionError" class="transfer-jobs" aria-label="跨主机传输">
    <header>
      <strong><Copy :size="16" />{{ phrase('跨主机传输') }}</strong>
      <button type="button" class="button button--secondary button--small" :disabled="transferJobsLoading" @click="refreshTransferJobs()"><RefreshCw :size="14" />{{ phrase('刷新') }}</button>
    </header>
    <p class="transfer-jobs__hint">{{ phrase('关闭文件窗口后继续传输；面板重启后标记中断。') }}</p>
    <p v-if="transferJobsError || actionError" role="alert">{{ phrase(actionError || transferJobsError) }}</p>
    <article v-for="job in visibleJobs" :key="job.id" class="transfer-job" :data-state="job.state">
      <div class="transfer-job__summary">
        <strong>{{ stage(job) }} · {{ finishedCount(job) }}/{{ job.items.length }}</strong>
        <span>{{ sourceName(job.sourceNodeId) }} → {{ hostName(job.targetHostId) }}</span>
        <span>{{ job.targetDirectory }}</span>
      </div>
      <div v-if="current(job)" class="transfer-job__progress" role="status">
        <span>{{ current(job)?.path }}</span>
        <span>{{ bytes(current(job)?.loadedBytes || 0) }}<template v-if="current(job)?.totalBytes"> / {{ bytes(current(job)?.totalBytes || 0) }}</template><template v-if="speed[job.id]"> · {{ bytes(Math.round(speed[job.id] || 0)) }}/s</template></span>
        <progress v-if="current(job)?.totalBytes" :value="Math.min(current(job)?.loadedBytes || 0, current(job)?.totalBytes || 0)" :max="current(job)?.totalBytes" aria-label="当前文件传输进度" />
      </div>
      <div class="transfer-job__actions">
        <button v-if="transferJobActive(job.state)" type="button" class="button button--secondary button--small" :disabled="pending.has(job.id)" @click="change(job, 'cancel')">{{ phrase('取消传输') }}</button>
        <template v-else>
          <button v-if="retryItems(job).length" type="button" class="button button--secondary button--small" :disabled="pending.has(job.id)" @click="change(job, 'retry')">{{ phrase('重试可恢复项目') }}</button>
          <button type="button" class="button button--secondary button--small" :disabled="pending.has(job.id)" @click="change(job, 'clear')">{{ phrase('清除记录') }}</button>
        </template>
      </div>
      <details>
        <summary>{{ phrase('逐项结果') }}</summary>
        <p v-if="!transferJobActive(job.state) && job.items.some(item => item.state !== 'complete' && !item.retryable)" class="transfer-jobs__hint">{{ phrase('结果不确定的项目请先检查目标目录，避免重复复制。') }}</p>
        <ul><li v-for="item in job.items" :key="item.path"><span>{{ item.path }}</span><strong>{{ phrase(labels[item.state] || '状态未知') }}</strong><span v-if="item.entry">→ {{ item.entry.path }}</span><span v-if="item.detail">{{ phrase(item.detail) }}</span></li></ul>
      </details>
    </article>
    <button v-if="transferJobs.length > 3" type="button" class="button button--secondary button--small" @click="expanded = !expanded">{{ phrase(expanded ? '收起记录' : '查看全部记录') }}</button>
  </section>
  <ModalDialog :open="Boolean(form)" title="复制到其他主机" description="复制完成后保留源文件；同名目标自动创建副本。" size="compact" :close-disabled="submitting" @close="form = undefined">
    <form v-if="form" class="transfer-form" @submit.prevent="submit">
      <p>{{ sourceName(form.sourceNodeId) }} · {{ form.items.length }} {{ phrase('项') }}</p>
      <label>{{ phrase('目标主机') }}<select v-model="form.targetHostId" :disabled="submitting"><option v-for="host in targets" :key="host.id" :value="host.isLocal ? '' : host.id">{{ host.name }}</option></select></label>
      <p v-if="!targetAvailable" role="alert">{{ phrase('没有可用的目标主机，请检查节点连接与文件管理能力。') }}</p>
      <label>{{ phrase('目标目录') }}<input v-model="form.targetDirectory" :disabled="submitting" placeholder="/home" autocomplete="off" /></label>
      <p>{{ phrase('目标目录必须已存在。单文件最多 512 MiB，目录内容最多 10 GiB、10,000 项。') }}</p>
      <p v-if="formError" role="alert">{{ phrase(formError) }}</p>
      <button class="button button--primary" type="submit" :disabled="submitting || !targetAvailable || !validPath(form.targetDirectory)">{{ phrase(submitting ? '正在提交…' : '开始复制') }}</button>
    </form>
  </ModalDialog>
</template>

<style scoped>
.transfer-jobs { padding: 1rem; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface); min-width: 0; }
.transfer-jobs header, .transfer-jobs header strong, .transfer-job__actions { display: flex; align-items: center; gap: .5rem; flex-wrap: wrap; }
.transfer-jobs header { justify-content: space-between; }
.transfer-job { border-top: 1px solid var(--border); padding: .85rem 0; display: grid; gap: .6rem; }
.transfer-job__summary, .transfer-job__progress, .transfer-form, .transfer-form label { display: grid; gap: .4rem; min-width: 0; }
.transfer-job span, .transfer-job li, .transfer-form p { overflow-wrap: anywhere; }
.transfer-job__summary strong, .transfer-form, .transfer-jobs header { font-size: .875rem; line-height: 1.5; }
.transfer-job span, .transfer-jobs__hint, .transfer-job details { font-size: .8125rem; line-height: 1.5; color: var(--text-soft); }
.transfer-job progress { width: 100%; accent-color: var(--brand); }
.transfer-job li { display: grid; gap: .2rem; margin-block: .6rem; }
.transfer-job ul { padding-left: 1rem; }
.transfer-job summary { cursor: pointer; min-height: 1.5rem; color: var(--text); }
.transfer-form input, .transfer-form select { width: 100%; box-sizing: border-box; min-height: 2.75rem; font: inherit; color: var(--text); background: var(--surface); border: 1px solid var(--border-strong); border-radius: var(--radius-sm); padding: .6rem; }
.transfer-jobs [role="alert"], .transfer-form [role="alert"] { color: var(--text); font-size: .875rem; overflow-wrap: anywhere; }
</style>
