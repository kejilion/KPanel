<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Plus, RefreshCw, Trash2 } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import type { MonitoringCheck, MonitoringCheckKind, MonitoringCheckSnapshot } from '@/types/api'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: []; saved: [snapshot: MonitoringCheckSnapshot] }>()

const snapshot = ref<MonitoringCheckSnapshot>()
const items = ref<MonitoringCheck[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const status = ref('')
let controller: AbortController | undefined

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const canAdd = computed(() => items.value.length < (snapshot.value?.maxItems || 16))
const canSave = computed(() => Boolean(snapshot.value?.available) && !saving.value && items.value.every(validCheck))

function checkPlaceholder(kind: MonitoringCheckKind): string {
  if (kind === 'tcp') return 'example.com:443'
  if (kind === 'http') return 'https://example.com/health'
  return '1.1.1.1'
}

function newID(): string {
  const random = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID().replaceAll('-', '')
    : `${Date.now().toString(36)}${Math.random().toString(36).slice(2)}`
  return `check-${random}`.slice(0, 64).replace(/-$/, '0')
}

function addCheck(): void {
  if (!canAdd.value) return
  items.value.push({ id: newID(), kind: 'ping', name: '', target: '' })
  status.value = ''
}

function removeCheck(index: number): void {
  items.value.splice(index, 1)
  status.value = ''
}

function validCheck(item: MonitoringCheck): boolean {
  if (!item.name.trim() || item.name !== item.name.trim() || [...item.name].length > 48) return false
  if (item.kind === 'ping') {
    const octets = item.target.split('.')
    return octets.length === 4 && octets.every((octet) => /^\d{1,3}$/.test(octet) && Number(octet) <= 255)
  }
  if (item.kind === 'tcp') {
    const match = item.target.match(/^(?:\[[^\]]+\]|[^:\s]+):(\d{1,5})$/)
    return Boolean(match && Number(match[1]) >= 1 && Number(match[1]) <= 65535)
  }
  try {
    const url = new URL(item.target)
    return (url.protocol === 'http:' || url.protocol === 'https:') && Boolean(url.hostname) && !url.username && !url.password
  } catch {
    return false
  }
}

function validationHint(item: MonitoringCheck): string {
  if (!item.name.trim()) return phrase('请填写检测名称。')
  if (item.name !== item.name.trim() || [...item.name].length > 48) return phrase('名称需为 1–48 个字符，首尾不能有空格。')
  if (validCheck(item)) return ''
  if (item.kind === 'ping') return phrase('Ping 目标需填写 IPv4 地址。')
  if (item.kind === 'tcp') return phrase('TCP 目标需使用 host:port 格式。')
  return phrase('HTTP 目标需使用完整的 http 或 https URL。')
}

async function load(): Promise<void> {
  if (!props.open) return
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  error.value = ''
  status.value = ''
  try {
    const result = await api.monitoring.checks(controller.signal)
    snapshot.value = result
    items.value = result.items.map((item) => ({ ...item }))
    if (!result.available) error.value = phrase('检测配置文件异常，已进入只读保护；请修复 Agent 状态目录中的 checks.json。')
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : phrase('无法读取检测项。')
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (!snapshot.value || !canSave.value) return
  saving.value = true
  error.value = ''
  status.value = ''
  try {
    const result = await api.monitoring.updateChecks({
      expectedResourceVersion: snapshot.value.resourceVersion,
      items: items.value.map((item) => ({ ...item, name: item.name.trim(), target: item.target.trim() })),
    })
    snapshot.value = result
    items.value = result.items.map((item) => ({ ...item }))
    status.value = phrase('检测项已保存，将在下一轮采样生效。')
    emit('saved', result)
  } catch (reason) {
    if (reason instanceof ApiError && reason.code === 'monitoring_checks_changed') {
      await load()
      error.value = phrase('检测项已在其他会话中更新，已重新读取，请核对后再保存。')
    } else {
      error.value = reason instanceof ApiError ? reason.message : phrase('检测项保存失败。')
    }
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) void load()
  else controller?.abort()
}, { immediate: true })

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <ModalDialog
    :open="open"
    :title="phrase('管理检测项')"
    :description="phrase('统一管理 Ping、TCP 与 HTTP 检测；保存后下一轮采样生效。')"
    size="large"
    :close-disabled="saving"
    @close="emit('close')"
  >
    <LoadingState v-if="loading" :message="phrase('正在读取检测项')" />
    <ErrorState v-else-if="error && !snapshot" :title="phrase('检测项读取失败')" :message="error" @retry="load" />
    <div v-else class="check-manager">
      <div v-if="error" class="check-manager__alert" role="alert">{{ error }}</div>
      <div v-if="status" class="check-manager__status" role="status">{{ status }}</div>
      <div class="check-manager__toolbar">
        <span>{{ phrase(`${items.length}/${snapshot?.maxItems || 16} 个检测项`) }}</span>
        <button class="button button--secondary button--small" type="button" :disabled="loading || saving" @click="load"><RefreshCw :size="15" />{{ phrase('重新读取') }}</button>
        <button class="button button--primary button--small" type="button" :disabled="!canAdd || saving" @click="addCheck"><Plus :size="15" />{{ phrase('添加检测项') }}</button>
      </div>

      <div v-if="items.length" class="check-manager__list">
        <article v-for="(item, index) in items" :key="item.id" class="check-editor">
          <label><span>{{ phrase('类型') }}</span><select v-model="item.kind"><option value="ping">Ping</option><option value="tcp">TCP</option><option value="http">HTTP</option></select></label>
          <label><span>{{ phrase('名称') }}</span><input v-model="item.name" maxlength="48" :placeholder="phrase('例如：官网首页')" /></label>
          <label class="check-editor__target"><span>{{ phrase('检测目标') }}</span><input v-model="item.target" maxlength="2048" :placeholder="checkPlaceholder(item.kind)" /></label>
          <button class="check-editor__delete" type="button" :aria-label="phrase(`删除 ${item.name || '未命名检测项'}`)" :title="phrase('删除检测项')" :disabled="saving" @click="removeCheck(index)"><Trash2 :size="17" /></button>
          <p v-if="validationHint(item)" class="check-editor__hint">{{ validationHint(item) }}</p>
        </article>
      </div>
      <div v-else class="check-manager__empty">
        <strong>{{ phrase('暂无检测项') }}</strong>
        <span>{{ phrase('可保持为空，或添加 Ping、TCP、HTTP 检测。') }}</span>
      </div>
    </div>
    <template #footer>
      <button class="button button--secondary" type="button" :disabled="saving" @click="emit('close')">{{ phrase('取消') }}</button>
      <button class="button button--primary" type="button" :disabled="!canSave" @click="save">{{ saving ? phrase('正在保存…') : phrase('保存变更') }}</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.check-manager { display: grid; gap: 14px; }
.check-manager__alert, .check-manager__status { padding: 10px 12px; border-radius: var(--radius-sm); font-size: 14px; line-height: 1.55; }
.check-manager__alert { color: var(--danger); background: var(--danger-soft); }
.check-manager__status { color: var(--brand-strong); background: var(--brand-soft); }
.check-manager__toolbar { display: flex; align-items: center; gap: 8px; }
.check-manager__toolbar > span { flex: 1; color: var(--muted); font-size: 13px; }
.check-manager__list { display: grid; gap: 9px; max-height: min(58vh, 620px); overflow-y: auto; padding-right: 3px; }
.check-editor { display: grid; grid-template-columns: 120px minmax(160px, .75fr) minmax(240px, 1.4fr) 38px; align-items: end; gap: 10px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius); background: var(--surface-subtle); }
.check-editor label { display: grid; gap: 6px; color: var(--muted); font-size: 13px; }
.check-editor input, .check-editor select { width: 100%; min-height: 38px; padding: 7px 9px; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--text); background: var(--surface); font: inherit; font-size: 14px; }
.check-editor input:focus, .check-editor select:focus { outline: 2px solid var(--brand); outline-offset: 1px; }
.check-editor__delete { display: grid; width: 38px; height: 38px; place-items: center; border: 1px solid var(--border); border-radius: var(--radius-sm); color: var(--danger); background: var(--surface); cursor: pointer; }
.check-editor__hint { grid-column: 1 / -1; margin: -2px 0 0; color: var(--amber); font-size: 13px; }
.check-manager__empty { display: grid; min-height: 180px; place-content: center; gap: 6px; color: var(--muted); text-align: center; }
.check-manager__empty strong { color: var(--text); font-size: 15px; }
.check-manager__empty span { font-size: 14px; }
@media (max-width: 760px) {
  .check-manager__toolbar { align-items: stretch; flex-wrap: wrap; }
  .check-manager__toolbar > span { width: 100%; flex-basis: 100%; }
  .check-manager__toolbar .button { flex: 1; }
  .check-editor { grid-template-columns: 1fr 42px; }
  .check-editor label, .check-editor__target { grid-column: 1 / -1; }
  .check-editor__delete { grid-column: 2; grid-row: 1; align-self: end; }
}
</style>
