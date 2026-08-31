<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RefreshCw, ShieldCheck } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { useToast } from '@/stores/toast'
import type { FirewallSnapshot, SystemResourceActionInput } from '@/types/api'

type ResourceActionWithoutVersion = SystemResourceActionInput extends infer Action
  ? Action extends { expectedResourceVersion: string }
    ? Omit<Action, 'expectedResourceVersion'>
    : never
  : never

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
const snapshot = ref<FirewallSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const running = ref(false)
const error = ref('')
const port = ref(443)
const address = ref('')
let controller: AbortController | undefined

const rules = computed(() => snapshot.value?.rules || [])
const validPort = computed(() => Number.isInteger(port.value) && port.value >= 1 && port.value <= 65535)

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
    snapshot.value = await api.system.firewall(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reasonMessage(reason, '无法读取防火墙状态。')
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

async function execute(input: ResourceActionWithoutVersion, confirmation = ''): Promise<void> {
  if (!props.writable || running.value || !snapshot.value?.resourceVersion) return
  if (confirmation && typeof window !== 'undefined' && !window.confirm(translatePhrase(confirmation))) return
  running.value = true
  try {
    const result = await api.system.resourceAction({
      ...input,
      expectedResourceVersion: snapshot.value.resourceVersion,
    } as SystemResourceActionInput)
    toast.success(result.changed ? '防火墙配置已更新' : '防火墙配置无需变更', result.message)
    await load(true)
  } catch (reason) {
    toast.danger('防火墙操作失败', reasonMessage(reason, 'Agent 未能完成该防火墙操作。'))
    await load(true)
  } finally {
    running.value = false
  }
}

function portAction(action: 'firewall-open-port' | 'firewall-close-port'): void {
  if (!validPort.value) return
  void execute(
    { action, port: Number(port.value) },
    `确认${action === 'firewall-open-port' ? '开放' : '关闭'}端口 ${port.value} 的 TCP 与 UDP 访问吗？`,
  )
}

function addressAction(action: 'firewall-allow-ip' | 'firewall-block-ip' | 'firewall-remove-ip'): void {
  if (!address.value.trim()) return
  const labels = {
    'firewall-allow-ip': '允许',
    'firewall-block-ip': '阻止',
    'firewall-remove-ip': '移除',
  }
  void execute({ action, address: address.value.trim() }, `确认${labels[action]} IP 规则 ${address.value.trim()} 吗？`)
}

function allPortsConfirmation(): string {
  if (snapshot.value?.inputPolicy === 'ACCEPT') {
    return '确认关闭全部端口吗？此操作会清空现有 iptables filter 规则与自定义链（包括 Docker 链），仅恢复基础规则，并将 INPUT/FORWARD 策略设为 DROP。'
  }
  return '确认开放全部端口吗？此操作会清空现有 iptables filter 规则与自定义链（包括 Docker 链），仅恢复基础规则，并将 INPUT/FORWARD 策略设为 ACCEPT。'
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
    :title="phrase('防火墙')"
    :description="phrase('按 kejilion.sh 固定动作管理端口、IP、Ping 与 DDoS 防护。')"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ phrase(unavailableReason || '当前 Agent 的防火墙适配器未就绪。') }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="4" />
      <ErrorState v-else-if="error && !snapshot" :message="phrase(error)" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>{{ snapshot.backend || phrase('未知后端') }} · INPUT {{ snapshot.inputPolicy || phrase('未识别') }}</span>
          <span>{{ phrase(`${snapshot.total} 条规则`) }}</span>
          <span v-if="snapshot.truncated" class="text-warning">{{ phrase('列表已截断') }}</span>
          <button class="icon-button" type="button" :disabled="refreshing || running" :title="phrase('刷新防火墙')" :aria-label="phrase('刷新防火墙')" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>

        <div v-if="!writable" class="inline-alert inline-alert--warning">
          {{ phrase('当前 Agent 仅支持查看防火墙，写入适配器未就绪。') }}
        </div>

        <div class="firewall-status-grid">
          <button class="system-resource-toggle" type="button" :disabled="!writable || running" @click="execute({ action: snapshot.pingAllowed ? 'firewall-disable-ping' : 'firewall-enable-ping' }, `确认${snapshot.pingAllowed ? '禁止' : '允许'} Ping 响应吗？`)">
            <ShieldCheck :size="18" />
            <span><strong>{{ phrase('Ping 响应') }}</strong><small>{{ phrase(snapshot.pingAllowed ? '已允许' : '已禁止') }}</small></span>
          </button>
          <button class="system-resource-toggle" type="button" :disabled="!writable || running" @click="execute({ action: snapshot.ddosEnabled ? 'firewall-disable-ddos' : 'firewall-enable-ddos' }, `确认${snapshot.ddosEnabled ? '停用' : '启用'} DDoS 防护吗？`)">
            <ShieldCheck :size="18" />
            <span><strong>{{ phrase('DDoS 防护') }}</strong><small>{{ phrase(snapshot.ddosEnabled ? '已启用' : '未启用') }}</small></span>
          </button>
          <button class="system-resource-toggle" type="button" :disabled="!writable || running" @click="execute({ action: snapshot.inputPolicy === 'ACCEPT' ? 'firewall-close-all' : 'firewall-open-all' }, allPortsConfirmation())">
            <ShieldCheck :size="18" />
            <span><strong>{{ phrase('全部端口') }}</strong><small>{{ phrase(snapshot.inputPolicy === 'ACCEPT' ? '当前开放，点击关闭' : '当前受限，点击开放') }}</small></span>
          </button>
        </div>

        <div class="system-resource-form firewall-action-form firewall-action-form--port">
          <label class="field">
            <span>{{ phrase('端口') }}</span>
            <input v-model.number="port" type="number" min="1" max="65535" inputmode="numeric" />
            <small>{{ phrase('开放或关闭时同时处理 TCP 与 UDP。') }}</small>
          </label>
          <div class="system-resource-form__actions">
            <button class="button button--secondary" type="button" :disabled="!writable || running || !validPort" @click="portAction('firewall-close-port')">{{ phrase('关闭端口') }}</button>
            <button class="button button--primary" type="button" :disabled="!writable || running || !validPort" @click="portAction('firewall-open-port')">{{ phrase('开放端口') }}</button>
          </div>
        </div>

        <div class="system-resource-form firewall-action-form">
          <label class="field system-resource-form__wide">
            <span>{{ phrase('IP 地址或网段') }}</span>
            <input v-model.trim="address" autocomplete="off" :placeholder="phrase('例如 198.51.100.20 或 198.51.100.0/24')" />
          </label>
          <div class="system-resource-form__actions system-resource-form__actions--three">
            <button class="button button--secondary" type="button" :disabled="!writable || running || !address" @click="addressAction('firewall-remove-ip')">{{ phrase('移除规则') }}</button>
            <button class="button button--danger" type="button" :disabled="!writable || running || !address" @click="addressAction('firewall-block-ip')">{{ phrase('阻止 IP') }}</button>
            <button class="button button--primary" type="button" :disabled="!writable || running || !address" @click="addressAction('firewall-allow-ip')">{{ phrase('允许 IP') }}</button>
          </div>
        </div>

        <EmptyState v-if="!rules.length" :title="phrase('暂无防火墙规则')" :description="phrase('当前后端没有返回可展示的规则。')" />
        <div v-else class="system-resource-list">
          <article v-for="(rule, index) in rules" :key="`${rule.line ?? index}-${rule.raw}`" class="system-resource-item">
            <div class="system-resource-item__main">
              <strong>{{ rule.chain }} · {{ rule.target }} · {{ rule.protocol }}</strong>
              <span>{{ rule.source }} → {{ rule.destination }}</span>
              <small>{{ rule.options.join(' ') || rule.raw }} · {{ phrase(`第 ${rule.line} 行`) }}</small>
            </div>
          </article>
        </div>
      </template>
    </div>
  </ModalDialog>
</template>
