<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Check, Circle, LoaderCircle, RefreshCw, Rocket, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { SystemTuningItemID, SystemTuningSnapshot } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<SystemTuningSnapshot>()
const selected = ref<SystemTuningItemID[]>([])
const loading = ref(false)
const refreshing = ref(false)
const submitting = ref(false)
const error = ref('')
let controller: AbortController | undefined
let timer: number | undefined

const definitions: Array<{ id: SystemTuningItemID; title: string; detail: string; risk?: boolean }> = [
  { id: 'system-update', title: '优化更新源并更新系统', detail: '按地区选择固定校验的镜像脚本，然后更新全部系统软件包。' },
  { id: 'system-cleanup', title: '清理系统垃圾', detail: '清理无用软件包、软件缓存和过期日志。' },
  { id: 'swap-1g', title: '设置 1 GB 虚拟内存', detail: '按 kejilion.sh 原流程重建 /swapfile 为 1 GB。', risk: true },
  { id: 'ssh-port-5522', title: '设置 SSH 端口为 5522', detail: '修改并验证 OpenSSH 配置；云安全组仍需放行 5522。', risk: true },
  { id: 'ssh-defense', title: '开启 SSH 防御', detail: '安装并启用 Fail2Ban，防止 SSH 暴力破解。' },
  { id: 'firewall-open-all', title: '开放所有端口', detail: '将主机防火墙 INPUT 与 FORWARD 策略设为允许。', risk: true },
  { id: 'bbr', title: '开启 BBR 加速', detail: '启用 BBR 拥塞控制与 fq 队列。' },
  { id: 'timezone-shanghai', title: '设置上海时区', detail: '将系统时区设置为 Asia/Shanghai。' },
  { id: 'dns-auto', title: '自动优化 DNS', detail: '国内使用国内 DNS，其他地区使用 Cloudflare 与 Google DNS。' },
  { id: 'ipv4-preferred', title: '设置 IPv4 优先', detail: '双栈网络中优先使用 IPv4，不关闭 IPv6。' },
  { id: 'basic-tools', title: '安装基础工具', detail: '安装 Docker、wget、sudo、tar、unzip、socat、btop、nano、vim。' },
  { id: 'kernel-auto', title: '自动网络参数优化', detail: '运行固定版本且通过 SHA-256 校验的 kejilion.sh 网络优化脚本。' },
]

const running = computed(() => snapshot.value?.maintenance.state === 'running' && snapshot.value.maintenance.action === 'system-tuning')
const failed = computed(() => snapshot.value?.maintenance.state === 'failed' && snapshot.value.maintenance.action === 'system-tuning')
const succeeded = computed(() => snapshot.value?.maintenance.state === 'succeeded' && snapshot.value.maintenance.action === 'system-tuning')
const currentItem = computed(() => running.value || failed.value ? (snapshot.value?.maintenance.stage || '').replace(/^system_tuning_/, '') : '')
const selectedSet = computed(() => new Set(selected.value))
const completedItems = computed(() => {
  if (succeeded.value) return new Set(selected.value)
  const currentIndex = selected.value.indexOf(currentItem.value as SystemTuningItemID)
  return new Set(currentIndex > 0 ? selected.value.slice(0, currentIndex) : [])
})

function clearTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

function selectedFromPolicy(policy: string): SystemTuningItemID[] {
  const separator = policy.indexOf('.')
  if (separator < 0) return []
  return policy.slice(separator + 1).split(',').filter((item): item is SystemTuningItemID => definitions.some((definition) => definition.id === item))
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.systemTuning(controller.signal)
    if (snapshot.value.maintenance.action === 'system-tuning' && snapshot.value.maintenance.policy) {
      const activeItems = selectedFromPolicy(snapshot.value.maintenance.policy)
      if (activeItems.length) selected.value = activeItems
    } else if (!selected.value.length) {
      selected.value = definitions.map((item) => item.id)
    }
    clearTimer()
    if (running.value) timer = window.setTimeout(() => void load(true), 1800)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取系统综合调优状态。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function toggle(item: SystemTuningItemID): void {
  if (running.value || submitting.value) return
  selected.value = selectedSet.value.has(item)
    ? selected.value.filter((value) => value !== item)
    : definitions.filter((definition) => definition.id === item || selectedSet.value.has(definition.id)).map((definition) => definition.id)
}

async function apply(): Promise<void> {
  if (!props.writable || !snapshot.value?.resourceVersion || !selected.value.length || running.value || submitting.value) return
  const risky = selected.value.filter((id) => definitions.find((item) => item.id === id)?.risk).length
  const message = `将按顺序执行 ${selected.value.length} 项调优${risky ? `，其中 ${risky} 项会修改 SSH、Swap 或防火墙` : ''}。任务在后台继续运行，并在首个失败项目停止。确认开始吗？`
  if (typeof window !== 'undefined' && !window.confirm(translatePhrase(message))) return
  submitting.value = true
  try {
    const result = await api.system.systemTuningAction({ action: 'apply', items: selected.value, expectedResourceVersion: snapshot.value.resourceVersion })
    toast.success('系统综合调优已开始', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('系统综合调优启动失败', reason instanceof ApiError ? reason.message : 'Agent 未能启动后台任务。')
    await load(true)
  } finally {
    submitting.value = false
  }
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) {
    selected.value = definitions.map((item) => item.id)
    void load()
  } else {
    controller?.abort()
    clearTimer()
  }
}, { immediate: true })

onBeforeUnmount(() => { controller?.abort(); clearTimer() })
</script>

<template>
  <ModalDialog :open="open" title="系统综合调优" description="沿用 kejilion.sh 原有 12 项流程；默认全部勾选，也可以只执行需要的项目。" size="wide" @close="emit('close')">
    <div class="tuning-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">{{ unavailableReason || '当前 Agent 的系统综合调优能力尚未就绪。' }}</div>
      <LoadingState v-else-if="loading && !snapshot" :rows="6" />
      <ErrorState v-else-if="error && !snapshot" :message="error" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="tuning-summary">
          <span v-if="running"><LoaderCircle :size="16" class="spin" /> 正在执行 · {{ snapshot.maintenance.progress }}%</span>
          <span v-else-if="snapshot.maintenance.action === 'system-tuning' && snapshot.maintenance.state === 'succeeded'"><Check :size="16" /> 上次任务已完成</span>
          <span v-else><Rocket :size="16" /> 已选择 {{ selected.length }}/12 项</span>
          <button class="icon-button" type="button" :disabled="refreshing || submitting" title="刷新调优状态" @click="load(true)"><RefreshCw :size="16" :class="{ spin: refreshing }" /></button>
        </header>
        <div v-if="snapshot.maintenance.action === 'system-tuning' && snapshot.maintenance.state === 'failed'" class="inline-alert inline-alert--danger">
          <TriangleAlert :size="16" /> {{ snapshot.maintenance.message || '任务在当前项目停止，请检查后重试。' }}
        </div>
        <div class="tuning-toolbar">
          <button type="button" :disabled="running || submitting" @click="selected = definitions.map((item) => item.id)">全部选择</button>
          <button type="button" :disabled="running || submitting" @click="selected = []">清空选择</button>
          <small>每项完成后都会回读验证；失败即停止，不会把后续项目显示为成功。</small>
        </div>
        <div class="tuning-list">
          <button
            v-for="(item, index) in definitions"
            :key="item.id"
            type="button"
            class="tuning-item"
            :class="{ 'is-selected': selectedSet.has(item.id), 'is-running': running && currentItem === item.id, 'is-failed': failed && currentItem === item.id, 'is-complete': completedItems.has(item.id) }"
            :disabled="running || submitting"
            @click="toggle(item.id)"
          >
            <span class="tuning-item__check">
              <LoaderCircle v-if="running && currentItem === item.id" :size="18" class="spin" />
              <TriangleAlert v-else-if="failed && currentItem === item.id" :size="18" />
              <Check v-else-if="completedItems.has(item.id) || selectedSet.has(item.id)" :size="18" />
              <Circle v-else :size="18" />
            </span>
            <span class="tuning-item__number">{{ index + 1 }}</span>
            <span class="tuning-item__body"><strong>{{ item.title }}</strong><small>{{ item.detail }}</small></span>
            <span v-if="item.risk" class="tuning-item__risk">有影响</span>
            <span v-else-if="snapshot.items.find((state) => state.id === item.id)?.state === 'ready'" class="tuning-item__ready">已符合</span>
          </button>
        </div>
        <footer class="tuning-footer">
          <span>{{ running ? snapshot.maintenance.message : '任务提交后可关闭窗口，重新打开仍会显示真实进度。' }}</span>
          <button class="button button--primary" type="button" :disabled="!writable || !selected.length || running || submitting" @click="apply">
            <LoaderCircle v-if="submitting" :size="16" class="spin" /><Rocket v-else :size="16" />
            {{ running ? `正在调优 ${snapshot.maintenance.progress}%` : `一键调优（${selected.length} 项）` }}
          </button>
        </footer>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.tuning-dialog { display: grid; gap: 14px; }
.tuning-summary { display: flex; align-items: center; gap: 8px; min-height: 42px; padding: 10px 12px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface-subtle); }
.tuning-summary > span { display: inline-flex; align-items: center; gap: 7px; font-weight: 700; }
.tuning-summary .icon-button { margin-left: auto; }
.tuning-toolbar { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.tuning-toolbar button { border: 1px solid var(--border); border-radius: 8px; padding: 6px 10px; color: var(--muted); background: transparent; cursor: pointer; }
.tuning-toolbar small { margin-left: auto; color: var(--muted); }
.tuning-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; padding-bottom: 74px; }
.tuning-item { display: grid; grid-template-columns: 22px 24px minmax(0, 1fr) auto; align-items: center; gap: 9px; min-height: 72px; padding: 11px 12px; border: 1px solid var(--border); border-radius: 12px; color: inherit; background: transparent; text-align: left; cursor: pointer; transition: border-color .18s ease, background .18s ease, transform .18s ease; }
.tuning-item:hover:not(:disabled) { transform: translateY(-1px); border-color: color-mix(in srgb, var(--brand) 45%, var(--border)); }
.tuning-item.is-selected { border-color: color-mix(in srgb, var(--brand) 45%, var(--border)); background: color-mix(in srgb, var(--brand) 7%, transparent); }
.tuning-item.is-running { border-color: var(--brand); animation: tuning-pulse 1.7s ease-in-out infinite; }
.tuning-item.is-complete { opacity: .72; }
.tuning-item.is-failed { border-color: var(--danger); background: color-mix(in srgb, var(--danger) 8%, transparent); }
.tuning-item.is-failed .tuning-item__check { color: var(--danger); }
.tuning-item__check { display: grid; place-items: center; color: var(--brand); }
.tuning-item__number { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 999px; background: var(--surface-subtle); font-size: 11px; font-weight: 800; }
.tuning-item__body { display: grid; gap: 4px; min-width: 0; }
.tuning-item__body small { color: var(--muted); line-height: 1.35; }
.tuning-item__risk, .tuning-item__ready { padding: 3px 7px; border-radius: 999px; font-size: 11px; white-space: nowrap; }
.tuning-item__risk { color: var(--amber); background: color-mix(in srgb, var(--amber) 12%, transparent); }
.tuning-item__ready { color: var(--brand); background: color-mix(in srgb, var(--brand) 12%, transparent); }
.tuning-footer { position: sticky; z-index: 2; bottom: 0; display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px 0 10px; background: var(--surface); box-shadow: 0 -12px 18px var(--surface); }
.tuning-footer > span { color: var(--muted); font-size: 12px; }
.tuning-footer .button { flex: 0 0 auto; display: inline-flex; align-items: center; gap: 7px; }
@keyframes tuning-pulse { 0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--brand) 0%, transparent); } 50% { box-shadow: 0 0 0 4px color-mix(in srgb, var(--brand) 12%, transparent); } }
@media (max-width: 760px) { .tuning-list { grid-template-columns: 1fr; } .tuning-toolbar small { width: 100%; margin-left: 0; } .tuning-footer { align-items: stretch; flex-direction: column; } .tuning-footer .button { justify-content: center; } }
@media (prefers-reduced-motion: reduce) { .tuning-item, .tuning-item.is-running { transition: none; animation: none; } }
</style>
