<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Plus, RefreshCw, Trash2 } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { HostsEntry, HostsSnapshot } from '@/types/api'

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
const snapshot = ref<HostsSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const address = ref('')
const hostnames = ref('')
const comment = ref('')
let controller: AbortController | undefined

const parsedHostnames = computed(() =>
  [...new Set(hostnames.value.split(/[\s,，]+/).map((value) => value.trim()).filter(Boolean))],
)
const canAdd = computed(() =>
  props.writable && Boolean(snapshot.value?.resourceVersion && address.value.trim() && parsedHostnames.value.length),
)

function reasonMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.hosts(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reasonMessage(reason, '无法读取本地 Hosts。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function addEntry(): Promise<void> {
  if (!canAdd.value || running.value || !snapshot.value) return
  running.value = true
  try {
    const result = await api.system.resourceAction({
      action: 'hosts-add',
      address: address.value.trim(),
      hostnames: parsedHostnames.value,
      ...(comment.value.trim() ? { comment: comment.value.trim() } : {}),
      expectedResourceVersion: snapshot.value.resourceVersion,
    })
    toast.success(result.changed ? 'Hosts 记录已添加' : 'Hosts 记录无需变更', result.message)
    address.value = ''
    hostnames.value = ''
    comment.value = ''
    await load(true)
  } catch (reason) {
    toast.danger('添加 Hosts 记录失败', reasonMessage(reason, 'Agent 未能添加该记录。'))
    await load(true)
  } finally {
    running.value = false
  }
}

async function removeEntry(entry: HostsEntry): Promise<void> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion) return
  if (typeof window !== 'undefined' && !window.confirm(translatePhrase(`确认删除 Hosts 第 ${entry.line} 行吗？`))) return
  running.value = true
  try {
    const result = await api.system.resourceAction({
      action: 'hosts-delete',
      line: entry.line,
      expectedResourceVersion: snapshot.value.resourceVersion,
    })
    toast.success(result.changed ? 'Hosts 记录已删除' : 'Hosts 记录无需变更', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('删除 Hosts 记录失败', reasonMessage(reason, 'Agent 未能删除该记录。'))
    await load(true)
  } finally {
    running.value = false
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
    :title="phrase('本地 Hosts')"
    :description="phrase('直接读取并管理 kejilion.sh 使用的本机 Hosts 事实。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的 Hosts 适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>{{ phrase(`共 ${snapshot.total} 条记录`) }}</span>
          <span v-if="snapshot.truncated" class="text-warning">{{ phrase('列表已截断') }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新 Hosts')" :aria-label="phrase('刷新 Hosts')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看 Hosts，写入适配器未就绪。') }}
        </div>

        <form class="system-resource-form system-resource-form--hosts" @submit.prevent="addEntry">
          <label class="field">
            <span>{{ phrase('IP 地址') }}</span>
            <input v-model.trim="address" autocomplete="off" :placeholder="phrase('例如 192.0.2.10')" />
          </label>
          <label class="field">
            <span>{{ phrase('主机名') }}</span>
            <input v-model="hostnames" autocomplete="off" :placeholder="phrase('多个主机名用空格分隔')" />
          </label>
          <label class="field">
            <span>{{ phrase('备注（可选）') }}</span>
            <input v-model="comment" autocomplete="off" maxlength="200" :placeholder="phrase('用途说明')" />
          </label>
          <button class="button button--primary" type="submit" :disabled="!canAdd || running">
            <Plus :size="16" /> {{ phrase('添加记录') }}
          </button>
        </form>

        <EmptyState v-if="!snapshot.entries.length" :title="phrase('暂无 Hosts 记录')" :description="phrase('可在上方添加本机域名映射。')" />
        <div v-else class="system-resource-list">
          <article v-for="entry in snapshot.entries" :key="`${entry.line}-${entry.raw}`" class="system-resource-item">
            <div class="system-resource-item__main">
              <strong>{{ entry.address }}</strong>
              <span>{{ entry.hostnames.join(' · ') }}</span>
              <small v-if="entry.comment">{{ entry.comment }}</small>
              <code v-else>{{ entry.raw }}</code>
            </div>
            <span class="system-resource-item__meta">{{ phrase(`第 ${entry.line} 行`) }}</span>
            <button class="icon-button icon-button--danger" type="button" :disabled="!writable || running" :title="phrase('删除 Hosts 记录')" :aria-label="phrase('删除 Hosts 记录')" @click="removeEntry(entry)">
              <Trash2 :size="16" />
            </button>
          </article>
        </div>
      </template>
    </div>
  </ModalDialog>
</template>
