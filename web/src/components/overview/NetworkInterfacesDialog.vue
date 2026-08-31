<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { Network, RefreshCw } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { NetworkInterfaceEntry, NetworkInterfacesSnapshot } from '@/types/api'

const props = withDefaults(
  defineProps<{
    open: boolean
    readable: boolean
    writable: boolean
    unavailableReason?: string
  }>(),
  { unavailableReason: '' },
)
const emit = defineEmits<{ close: [] }>()
const toast = useToast()
const snapshot = ref<NetworkInterfacesSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const runningInterface = ref('')
const error = ref('')
let controller: AbortController | undefined

function reasonMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function isEnabled(entry: NetworkInterfaceEntry): boolean {
  return entry.state.toLowerCase() === 'up'
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.networkInterfaces(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reasonMessage(reason, '无法读取网卡状态。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function setInterfaceState(entry: NetworkInterfaceEntry): Promise<void> {
  if (!props.writable || runningInterface.value || !snapshot.value?.resourceVersion) return
  runningInterface.value = entry.name
  const enabled = !isEnabled(entry)
  if (
    !enabled &&
    typeof window !== 'undefined' &&
    !window.confirm(translatePhrase(`停用网卡 ${entry.name} 可能中断面板和网络连接，确认继续吗？`))
  ) {
    runningInterface.value = ''
    return
  }
  try {
    const result = await api.system.resourceAction({
      action: 'network-interface-state',
      interfaceName: entry.name,
      enabled,
      expectedResourceVersion: entry.resourceVersion || snapshot.value.resourceVersion,
    })
    toast.success(result.changed ? `网卡 ${entry.name} 已${enabled ? '启用' : '停用'}` : `网卡 ${entry.name} 无需变更`, result.message)
    await load(true)
  } catch (reason) {
    toast.danger('修改网卡状态失败', reasonMessage(reason, 'Agent 未能修改该网卡。'))
    await load(true)
  } finally {
    runningInterface.value = ''
  }
}

watch(
  () => [props.open, props.readable] as const,
  ([open, readable]) => {
    if (open && readable) void load()
    else controller?.abort()
  },
  { immediate: true },
)

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('网卡管理')"
    :description="phrase('查看网卡、硬件地址和本机地址，并通过固定动作启用或停用接口。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的网卡适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>{{ phrase(`共 ${snapshot.total} 个网络接口`) }}</span>
          <span v-if="snapshot.truncated" class="text-warning">{{ phrase('列表已截断') }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || Boolean(runningInterface)" :title="phrase('刷新网卡')" :aria-label="phrase('刷新网卡')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看网卡，写入适配器未就绪。') }}
        </div>

        <EmptyState v-if="!snapshot.entries.length" :title="phrase('未发现网络接口')" :description="phrase('Agent 没有返回可管理的网卡。')" />
        <div v-else class="system-resource-list system-resource-list--interfaces">
          <article v-for="entry in snapshot.entries" :key="entry.name" class="system-resource-item system-resource-item--interface">
            <span class="system-resource-item__icon"><Network :size="18" /></span>
            <div class="system-resource-item__main">
              <strong>{{ entry.name }}</strong>
              <span>{{ entry.addresses.length ? entry.addresses.join(' · ') : phrase('未配置地址') }}</span>
              <small>{{ entry.macAddress || phrase('未识别硬件地址') }}<template v-if="entry.loopback"> · {{ phrase('回环接口') }}</template></small>
            </div>
            <span class="system-resource-state" :class="{ 'is-on': isEnabled(entry) }">
              {{ isEnabled(entry) ? phrase('已启用') : entry.state || phrase('未知状态') }}
            </span>
            <button
              class="button button--secondary button--small"
              type="button"
              :disabled="!writable || Boolean(runningInterface)"
              @click="setInterfaceState(entry)"
            >
              {{ runningInterface === entry.name ? phrase('正在应用…') : isEnabled(entry) ? phrase('停用') : phrase('启用') }}
            </button>
          </article>
        </div>
      </template>
    </div>
  </ModalDialog>
</template>
