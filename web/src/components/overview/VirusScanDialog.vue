<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Bug, CheckCircle2, FileWarning, FolderSearch, LoaderCircle, RefreshCw, ScanSearch, ShieldCheck, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type { VirusScanMode, VirusScanSnapshot } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<VirusScanSnapshot>()
const mode = ref<VirusScanMode>('important')
const customPaths = ref('/home')
const loading = ref(false)
const refreshing = ref(false)
const submitting = ref(false)
const error = ref('')
let controller: AbortController | undefined
let timer: number | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const running = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action === 'virus-scan')
const maintenanceBusy = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action !== 'virus-scan')
const parsedPaths = computed(() => customPaths.value.split(/\r?\n/).map((value) => value.trim()).filter(Boolean))
const customValid = computed(() => {
  const paths = parsedPaths.value
  return paths.length >= 1 && paths.length <= 8 && new Set(paths).size === paths.length && paths.every((value) =>
    value.startsWith('/') && value.length <= 512 && !value.includes(',') && !value.includes('\0'))
})
const canSubmit = computed(() => props.writable && !running.value && !maintenanceBusy.value && !submitting.value && (mode.value !== 'custom' || customValid.value))
const statusLabel = computed(() => {
  if (running.value) return phrase(`扫描进行中 · ${snapshot.value?.maintenance.progress || 0}%`)
  switch (snapshot.value?.status) {
    case 'clean': return phrase('未发现病毒')
    case 'infected': return phrase(`发现 ${snapshot.value.infectedFiles} 个威胁`)
    case 'completed-with-errors': return phrase('扫描完成，但存在读取错误')
    case 'unknown': return phrase('扫描报告不完整')
    default: return phrase('尚未扫描')
  }
})

function clearTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.virusScan(controller.signal)
    clearTimer()
    if (running.value) timer = window.setTimeout(() => void load(true), 2_000)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取病毒扫描状态。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function startScan(): Promise<void> {
  if (!canSubmit.value) return
  const labels: Record<VirusScanMode, string> = {
    full: '整个系统', important: '重要系统目录', custom: `${parsedPaths.value.length} 个自定义目录`,
  }
  const confirmation = `将更新 ClamAV 病毒库并扫描${labels[mode.value]}。扫描可能持续较长时间并占用 CPU、磁盘 I/O 和网络流量；只生成报告，不会自动删除或隔离文件。确认开始吗？`
  if (typeof window !== 'undefined' && !window.confirm(translatePhrase(confirmation))) return
  submitting.value = true
  try {
    const result = await api.system.virusScanAction({
      mode: mode.value,
      paths: mode.value === 'custom' ? parsedPaths.value : undefined,
    })
    toast.success('病毒扫描已开始', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('病毒扫描启动失败', reason instanceof ApiError ? reason.message : 'Agent 未能启动后台扫描任务。')
    await load(true)
  } finally {
    submitting.value = false
  }
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) void load()
  else {
    controller?.abort()
    clearTimer()
  }
}, { immediate: true })

onBeforeUnmount(() => { controller?.abort(); clearTimer() })
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('病毒查杀')"
    :description="phrase('复用 kejilion.sh 的 Docker ClamAV 流程，更新病毒库后扫描并保留有界报告。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="virus-scan-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的病毒扫描适配器尚未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="5" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="virus-scan-summary" :class="{ 'is-danger': snapshot.status === 'infected' }">
          <span class="virus-scan-summary__icon">
            <LoaderCircle v-if="running" :size="22" class="spin" />
            <Bug v-else-if="snapshot.status === 'infected'" :size="22" />
            <ShieldCheck v-else :size="22" />
          </span>
          <span class="virus-scan-summary__body">
            <strong>{{ statusLabel }}</strong>
            <small v-if="running">{{ phrase(snapshot.maintenance.message || '病毒扫描正在后台运行。') }}</small>
            <small v-else-if="snapshot.completedAt">{{ phrase('上次完成') }} · {{ formatDateTime(snapshot.completedAt) }}</small>
            <small v-else>{{ phrase('扫描记录来自宿主机真实 ClamAV 报告。') }}</small>
          </span>
          <button class="icon-button" type="button" :disabled="refreshing || submitting" :title="phrase('刷新病毒扫描状态')" :aria-label="phrase('刷新病毒扫描状态')" @click="load(true)">
            <RefreshCw :size="17" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="maintenanceBusy" class="inline-alert inline-alert--warning">
          <TriangleAlert :size="17" /> {{ phrase('另一项系统维护任务正在运行，完成后才能开始病毒扫描。') }}
        </div>
        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看报告；Docker、后台任务执行器或新版 kejilion.sh 协议尚未就绪。') }}
        </div>

        <section class="virus-scan-options" :aria-label="phrase('扫描范围')">
          <button type="button" :class="{ 'is-selected': mode === 'full' }" :disabled="running || submitting" @click="mode = 'full'">
            <ScanSearch :size="20" /><span><strong>{{ phrase('全盘扫描') }}</strong><small>{{ phrase('扫描整个 /，耗时和资源占用最高。') }}</small></span>
          </button>
          <button type="button" :class="{ 'is-selected': mode === 'important' }" :disabled="running || submitting" @click="mode = 'important'">
            <ShieldCheck :size="20" /><span><strong>{{ phrase('重要目录扫描') }}</strong><small>/etc · /var · /usr · /home · /root</small></span>
          </button>
          <button type="button" :class="{ 'is-selected': mode === 'custom' }" :disabled="running || submitting" @click="mode = 'custom'">
            <FolderSearch :size="20" /><span><strong>{{ phrase('自定义目录') }}</strong><small>{{ phrase('每行一个绝对路径，最多 8 个。') }}</small></span>
          </button>
        </section>

        <label v-if="mode === 'custom'" class="field virus-scan-paths">
          <span>{{ phrase('扫描目录') }}</span>
          <textarea v-model="customPaths" rows="4" spellcheck="false" placeholder="/home&#10;/srv/sites" />
          <small :class="{ 'is-invalid': !customValid }">{{ phrase('目录必须是规范的 Linux 绝对路径，不能重复或包含逗号。') }}</small>
        </label>

        <div class="virus-scan-metrics" aria-live="polite">
          <span><CheckCircle2 :size="17" /><small>{{ phrase('已扫描文件') }}</small><strong>{{ snapshot.scannedFiles }}</strong></span>
          <span :class="{ 'is-danger': snapshot.infectedFiles > 0 }"><Bug :size="17" /><small>{{ phrase('发现威胁') }}</small><strong>{{ snapshot.infectedFiles }}</strong></span>
          <span :class="{ 'is-warning': snapshot.errors > 0 }"><FileWarning :size="17" /><small>{{ phrase('读取错误') }}</small><strong>{{ snapshot.errors }}</strong></span>
        </div>

        <section v-if="snapshot.findings.length" class="virus-scan-findings">
          <header><strong>{{ phrase('威胁位置') }}</strong><small v-if="snapshot.truncated">{{ phrase('结果过多，仅显示有界片段。') }}</small></header>
          <pre data-i18n-ignore>{{ snapshot.findings.join('\n') }}</pre>
        </section>
        <div v-else-if="snapshot.reportAvailable && !running" class="inline-alert inline-alert--success">
          <ShieldCheck :size="17" /> {{ phrase('当前报告未发现威胁。') }}
        </div>

        <footer class="virus-scan-footer">
          <span>{{ phrase('报告保存在 /home/docker/clamav/log/scan.log；只生成报告，不会自动删除或隔离文件。发现威胁后请先核对文件来源。') }}</span>
          <button class="button button--primary" type="button" :disabled="!canSubmit" @click="startScan">
            <LoaderCircle v-if="submitting || running" :size="17" class="spin" /><ScanSearch v-else :size="17" />
            {{ phrase(running ? `正在扫描 ${snapshot.maintenance.progress}%` : '开始扫描') }}
          </button>
        </footer>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.virus-scan-dialog { display: grid; gap: 14px; }
.virus-scan-summary { display: flex; align-items: center; gap: 11px; min-height: 56px; padding: 11px 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); }
.virus-scan-summary.is-danger { border-color: color-mix(in srgb, var(--danger) 45%, var(--border)); }
.virus-scan-summary__icon { display: grid; place-items: center; width: 38px; height: 38px; color: var(--brand); border-radius: var(--radius); background: color-mix(in srgb, var(--brand) 10%, transparent); }
.virus-scan-summary.is-danger .virus-scan-summary__icon { color: var(--danger); background: color-mix(in srgb, var(--danger) 10%, transparent); }
.virus-scan-summary__body { display: grid; gap: 3px; min-width: 0; }
.virus-scan-summary__body strong { font-size: 15px; }
.virus-scan-summary__body small { color: var(--muted); font-size: 13px; line-height: 1.45; }
.virus-scan-summary .icon-button { margin-left: auto; }
.virus-scan-options { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.virus-scan-options button { display: flex; align-items: flex-start; gap: 9px; min-height: 78px; padding: 12px; color: inherit; text-align: left; border: 1px solid var(--border); border-radius: var(--radius); background: transparent; cursor: pointer; }
.virus-scan-options button:hover:not(:disabled), .virus-scan-options button.is-selected { border-color: color-mix(in srgb, var(--brand) 50%, var(--border)); background: color-mix(in srgb, var(--brand) 7%, transparent); }
.virus-scan-options button > span { display: grid; gap: 5px; }
.virus-scan-options strong { font-size: 14px; }
.virus-scan-options small, .virus-scan-paths small { color: var(--muted); font-size: 13px; line-height: 1.45; }
.virus-scan-paths textarea { min-height: 96px; resize: vertical; font: 14px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; }
.virus-scan-paths small.is-invalid { color: var(--danger); }
.virus-scan-metrics { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.virus-scan-metrics > span { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 8px; padding: 10px 12px; border: 1px solid var(--border); border-radius: var(--radius); }
.virus-scan-metrics small { color: var(--muted); font-size: 13px; }
.virus-scan-metrics strong { font-size: 16px; }
.virus-scan-metrics .is-danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 38%, var(--border)); }
.virus-scan-metrics .is-warning { color: var(--amber); }
.virus-scan-findings { display: grid; gap: 8px; }
.virus-scan-findings header { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; }
.virus-scan-findings header strong { font-size: 14px; }
.virus-scan-findings header small { color: var(--muted); font-size: 13px; }
.virus-scan-findings pre { max-height: 240px; margin: 0; padding: 12px; overflow: auto; color: var(--text); font-size: 12px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); }
.virus-scan-footer { position: sticky; bottom: 0; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px 0 8px; background: var(--surface); box-shadow: 0 -12px 18px var(--surface); }
.virus-scan-footer > span { max-width: 68ch; color: var(--muted); font-size: 13px; line-height: 1.5; }
.virus-scan-footer .button { flex: 0 0 auto; display: inline-flex; align-items: center; gap: 7px; }
@media (max-width: 760px) { .virus-scan-options, .virus-scan-metrics { grid-template-columns: 1fr; } .virus-scan-footer { align-items: stretch; flex-direction: column; } .virus-scan-footer .button { justify-content: center; min-height: 42px; } }
</style>
