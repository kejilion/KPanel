<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Pencil, Plus, RefreshCw, Trash2, X } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { CronEntry, CronSnapshot } from '@/types/api'

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
const snapshot = ref<CronSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const expression = ref('')
const command = ref('')
const editingLine = ref<number>()
let controller: AbortController | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const canSubmit = computed(() =>
  props.writable && Boolean(snapshot.value?.resourceVersion && expression.value.trim() && command.value.trim()),
)

function reasonMessage(reason: unknown, fallback: string): string {
  return reason instanceof ApiError ? reason.message : fallback
}

function resetForm(): void {
  editingLine.value = undefined
  expression.value = ''
  command.value = ''
}

function editEntry(entry: CronEntry): void {
  editingLine.value = entry.line
  expression.value = entry.expression
  command.value = entry.command
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.cron(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reasonMessage(reason, '无法读取定时任务。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function submit(): Promise<void> {
  if (!canSubmit.value || running.value || !snapshot.value) return
  running.value = true
  const input = editingLine.value === undefined
    ? {
        action: 'cron-add' as const,
        expression: expression.value.trim(),
        command: command.value.trim(),
        expectedResourceVersion: snapshot.value.resourceVersion,
      }
    : {
        action: 'cron-update' as const,
        line: editingLine.value,
        expression: expression.value.trim(),
        command: command.value.trim(),
        expectedResourceVersion: snapshot.value.resourceVersion,
      }
  try {
    const result = await api.system.resourceAction(input)
    toast.success(result.changed ? '定时任务已保存' : '定时任务无需变更', result.message)
    resetForm()
    await load(true)
  } catch (reason) {
    toast.danger('保存定时任务失败', reasonMessage(reason, 'Agent 未能保存该任务。'))
    await load(true)
  } finally {
    running.value = false
  }
}

async function removeEntry(entry: CronEntry): Promise<void> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion) return
  if (typeof window !== 'undefined' && !window.confirm(translatePhrase(`确认删除定时任务第 ${entry.line} 行吗？`))) return
  running.value = true
  try {
    const result = await api.system.resourceAction({
      action: 'cron-delete',
      line: entry.line,
      expectedResourceVersion: snapshot.value.resourceVersion,
    })
    toast.success(result.changed ? '定时任务已删除' : '定时任务无需变更', result.message)
    if (editingLine.value === entry.line) resetForm()
    await load(true)
  } catch (reason) {
    toast.danger('删除定时任务失败', reasonMessage(reason, 'Agent 未能删除该任务。'))
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
    :title="phrase('定时任务管理')"
    :description="phrase('读取并管理 kejilion.sh 兼容的系统定时任务。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的定时任务适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>{{ phrase(`共 ${snapshot.total} 条定时任务记录`) }}</span>
          <span v-if="snapshot.truncated" class="text-warning">{{ phrase('列表已截断') }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新定时任务')" :aria-label="phrase('刷新定时任务')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看定时任务，写入适配器未就绪。') }}
        </div>

        <form class="system-resource-form system-resource-form--cron" @submit.prevent="submit">
          <label class="field">
            <span>{{ phrase('Cron 表达式') }}</span>
            <input v-model="expression" autocomplete="off" :placeholder="phrase('例如 0 3 * * *')" />
          </label>
          <label class="field system-resource-form__wide">
            <span>{{ phrase('执行命令') }}</span>
            <input v-model="command" autocomplete="off" :placeholder="phrase('由 kejilion.sh 兼容适配器校验并写入')" />
          </label>
          <div class="system-resource-form__actions">
            <button v-if="editingLine !== undefined" class="button button--secondary" type="button" :disabled="running" @click="resetForm">
              <X :size="16" /> {{ phrase('取消编辑') }}
            </button>
            <button class="button button--primary" type="submit" :disabled="!canSubmit || running">
              <Pencil v-if="editingLine !== undefined" :size="16" />
              <Plus v-else :size="16" />
              {{ phrase(editingLine === undefined ? '添加任务' : '保存修改') }}
            </button>
          </div>
        </form>

        <EmptyState v-if="!snapshot.entries.length" :title="phrase('暂无定时任务')" :description="phrase('可在上方添加新的 Cron 任务。')" />
        <div v-else class="system-resource-list">
          <article v-for="entry in snapshot.entries" :key="`${entry.line}-${entry.raw}`" class="system-resource-item">
            <div class="system-resource-item__main">
              <strong>{{ entry.expression || entry.kind }}</strong>
              <span>{{ entry.command || entry.raw }}</span>
              <small>{{ phrase(`${entry.kind} · 第 ${entry.line} 行`) }}</small>
            </div>
            <button class="icon-button" type="button" :disabled="!writable || running || !entry.expression || !entry.command" :title="phrase('编辑定时任务')" :aria-label="phrase('编辑定时任务')" @click="editEntry(entry)">
              <Pencil :size="16" />
            </button>
            <button class="icon-button icon-button--danger" type="button" :disabled="!writable || running" :title="phrase('删除定时任务')" :aria-label="phrase('删除定时任务')" @click="removeEntry(entry)">
              <Trash2 :size="16" />
            </button>
          </article>
        </div>
      </template>
    </div>
  </ModalDialog>
</template>
