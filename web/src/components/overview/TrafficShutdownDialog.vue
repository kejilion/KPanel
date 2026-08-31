<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Power, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatBytes } from '@/lib/format'
import { useToast } from '@/stores/toast'
import type { TrafficShutdownSnapshot } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; writable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<TrafficShutdownSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const rxThresholdGiB = ref(100)
const txThresholdGiB = ref(100)
const resetDay = ref(1)
let controller: AbortController | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const formValid = computed(() =>
  Number.isInteger(rxThresholdGiB.value) && rxThresholdGiB.value >= 1 && rxThresholdGiB.value <= 8_388_607 &&
  Number.isInteger(txThresholdGiB.value) && txThresholdGiB.value >= 1 && txThresholdGiB.value <= 8_388_607 &&
  Number.isInteger(resetDay.value) && resetDay.value >= 1 && resetDay.value <= 31,
)

const healthLabel = computed(() => ({
  ready: phrase('运行正常'), disabled: phrase('未启用'), inconsistent: phrase('配置不完整'),
}[snapshot.value?.health || 'disabled']))

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.trafficShutdown(controller.signal)
    if (snapshot.value.rxThresholdGiB > 0) rxThresholdGiB.value = snapshot.value.rxThresholdGiB
    if (snapshot.value.txThresholdGiB > 0) txThresholdGiB.value = snapshot.value.txThresholdGiB
    if (snapshot.value.resetDay > 0) resetDay.value = snapshot.value.resetDay
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取限流关机状态。'
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function execute(action: 'enable' | 'disable'): Promise<void> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion || (action === 'enable' && !formValid.value)) return
  const message = action === 'enable'
    ? `启用后，累计接收或发送流量达到阈值会立即关闭服务器；每月 ${resetDay.value} 日 01:00 会重启服务器以重置开机累计流量。确认继续吗？`
    : '确认停用限流关机吗？只会删除该功能自己的脚本和定时任务，不会删除其他 reboot 定时任务。'
  if (typeof window !== 'undefined' && !window.confirm(translatePhrase(message))) return
  running.value = true
  try {
    const result = action === 'enable'
      ? await api.system.trafficShutdownAction({
          action,
          expectedResourceVersion: snapshot.value.resourceVersion,
          rxThresholdGiB: rxThresholdGiB.value,
          txThresholdGiB: txThresholdGiB.value,
          resetDay: resetDay.value,
        })
      : await api.system.trafficShutdownAction({ action, expectedResourceVersion: snapshot.value.resourceVersion })
    toast.success(result.changed ? '限流关机配置已更新' : '限流关机配置无需变更', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('限流关机操作失败', reason instanceof ApiError ? reason.message : 'Agent 未能完成限流关机操作。')
    await load(true)
  } finally {
    running.value = false
  }
}

watch(() => [props.open, props.readable] as const, ([open, readable]) => {
  if (open && readable) void load()
  else controller?.abort()
}, { immediate: true })

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('限流自动关机')"
    :description="phrase('按 kejilion.sh 的累计流量目的，在达到接收或发送阈值时关闭服务器。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的限流关机适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>{{ healthLabel }}</span>
          <span>{{ phrase('累计接收') }} {{ formatBytes(snapshot.rxBytes) }}</span>
          <span>{{ phrase('累计发送') }} {{ formatBytes(snapshot.txBytes) }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新限流关机')" :aria-label="phrase('刷新限流关机')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>
        <div v-if="snapshot.health === 'inconsistent'" class="inline-alert inline-alert--warning">
          {{ phrase('检测到脚本或定时任务不完整。重新启用可由 kejilion.sh 修复为托管配置。') }}
        </div>
        <div class="inline-alert inline-alert--warning">
          {{ phrase('流量从本次开机开始累计；达到任一阈值会立即关机。重置日会在当日 01:00 重启服务器，重启后计数归零。') }}
        </div>
        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看，写入适配器未就绪。') }}
        </div>
        <form class="system-resource-form system-resource-form--traffic" @submit.prevent="execute('enable')">
          <label class="field"><span>{{ phrase('接收阈值（GiB）') }}</span><input v-model.number="rxThresholdGiB" type="number" min="1" max="8388607" /></label>
          <label class="field"><span>{{ phrase('发送阈值（GiB）') }}</span><input v-model.number="txThresholdGiB" type="number" min="1" max="8388607" /></label>
          <label class="field"><span>{{ phrase('每月重置日') }}</span><input v-model.number="resetDay" type="number" min="1" max="31" /></label>
          <div class="system-resource-form__actions">
            <button class="button button--primary" type="submit" :disabled="!writable || running || !formValid">
              <Power :size="16" /> {{ phrase(running ? '正在应用…' : snapshot.enabled ? '更新配置' : '启用') }}
            </button>
            <button class="button button--secondary" type="button" :disabled="!writable || running || snapshot.health === 'disabled'" @click="execute('disable')">{{ phrase('停用') }}</button>
          </div>
        </form>
      </template>
    </div>
  </ModalDialog>
</template>
