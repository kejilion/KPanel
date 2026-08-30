<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { RefreshCw, Search } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { ApiError, api } from '@/lib/api'
import type { PortUsageEntry, PortUsageSnapshot } from '@/types/api'

const props = withDefaults(defineProps<{ open: boolean; readable: boolean; unavailableReason?: string }>(), {
  unavailableReason: '',
})
const emit = defineEmits<{ close: [] }>()
const snapshot = ref<PortUsageSnapshot>()
const loading = ref(false)
const refreshing = ref(false)
const error = ref('')
const search = ref('')
let controller: AbortController | undefined

interface ProcessGroup {
  key: string
  name: string
  identified: boolean
  pids: number[]
  entries: PortUsageEntry[]
}

const processGroups = computed<ProcessGroup[]>(() => {
  const keyword = search.value.trim().toLowerCase()
  const values = (snapshot.value?.entries || []).filter((entry) => !keyword ||
    [entry.protocol, entry.state, entry.localAddress, entry.localPort, entry.process, String(entry.pid || ''), entry.raw]
      .some((value) => String(value || '').toLowerCase().includes(keyword)),
  )
  const groups = new Map<string, { name: string; identified: boolean; pids: Set<number>; entries: Map<string, PortUsageEntry> }>()
  for (const entry of values) {
    const name = processName(entry.process)
    const key = name || '__unknown__'
    const group = groups.get(key) || { name: name || '系统未返回占用程序', identified: Boolean(name), pids: new Set<number>(), entries: new Map<string, PortUsageEntry>() }
    if (entry.pid) group.pids.add(entry.pid)
    group.entries.set(`${entry.protocol}:${entry.localAddress}:${entry.localPort}`, entry)
    groups.set(key, group)
  }
  return [...groups.entries()]
    .map(([key, group]) => ({
      key,
      name: group.name,
      identified: group.identified,
      pids: [...group.pids].sort((left, right) => left - right),
      entries: [...group.entries.values()].sort(comparePortEntries),
    }))
    .sort((left, right) => {
      if (left.identified !== right.identified) return left.identified ? -1 : 1
      return left.name.localeCompare(right.name)
    })
})

const identifiedProcessCount = computed(() => new Set(
  (snapshot.value?.entries || [])
    .filter((entry) => processName(entry.process))
    .map((entry) => processName(entry.process)),
).size)

function comparePortEntries(left: PortUsageEntry, right: PortUsageEntry): number {
  const leftPort = Number(left.localPort)
  const rightPort = Number(right.localPort)
  if (Number.isFinite(leftPort) && Number.isFinite(rightPort) && leftPort !== rightPort) return leftPort - rightPort
  return `${left.protocol}:${left.localAddress}`.localeCompare(`${right.protocol}:${right.localAddress}`)
}

function endpoint(address: string, port: string): string {
  const host = address.includes(':') && !address.startsWith('[') ? `[${address}]` : address
  return `${host || '*'}:${port || '*'}`
}

function processName(value?: string): string {
  if (!value) return ''
  return /users:\(\("([^"]+)"/.exec(value)?.[1] || value
}

function protocolLabel(protocol: string): string {
  return protocol.toLowerCase() === 'udp' ? 'UDP 监听' : 'TCP 监听'
}

function listenScope(address: string): string {
  if (address === '0.0.0.0' || address === '*') return '所有 IPv4 地址'
  if (address === '::' || address === '[::]') return '所有 IPv6 地址'
  if (address === '127.0.0.1' || address === '::1') return '仅本机'
  return '指定地址'
}

async function load(silent = false): Promise<void> {
  if (!props.open || !props.readable) return
  controller?.abort()
  controller = new AbortController()
  if (silent) refreshing.value = true
  else loading.value = true
  error.value = ''
  try {
    snapshot.value = await api.system.portUsage(controller.signal)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    error.value = reason instanceof ApiError ? reason.message : '无法读取端口占用状态。'
  } finally {
    loading.value = false
    refreshing.value = false
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
    title="端口占用查看"
    description="通过 kejilion.sh 读取当前 TCP / UDP 监听端口、进程和 PID。"
    size="wide"
    @close="emit('close')"
  >
    <div class="system-resource-dialog">
      <div v-if="!readable" class="inline-alert inline-alert--warning">
        {{ unavailableReason || '当前 Agent 的端口占用适配器未就绪。' }}
      </div>
      <LoadingState v-else-if="loading && !snapshot" :rows="5" />
      <ErrorState v-else-if="error && !snapshot" :message="error" @retry="load()" />
      <template v-else-if="snapshot">
        <header class="system-resource-dialog__summary">
          <span>共 {{ snapshot.total }} 条监听记录</span>
          <span>已识别 {{ identifiedProcessCount }} 个占用程序</span>
          <span v-if="snapshot.truncated" class="text-warning">仅显示前 512 条</span>
          <button class="icon-button" type="button" :disabled="refreshing" title="刷新端口占用" aria-label="刷新端口占用" @click="load(true)">
            <RefreshCw :size="16" :class="{ spin: refreshing }" />
          </button>
        </header>
        <label class="field system-resource-search">
          <span>筛选</span>
          <span class="system-resource-search__input"><Search :size="15" /><input v-model="search" autocomplete="off" placeholder="端口、地址、协议、进程或 PID" /></span>
        </label>
        <div v-if="snapshot.entries.length && identifiedProcessCount === 0" class="inline-alert inline-alert--warning">
          当前主机未返回进程信息。常见于内核转发、容器网络或进程信息受限；端口与监听范围仍是实时结果。
        </div>
        <EmptyState v-if="!processGroups.length" title="没有匹配的监听端口" description="调整筛选条件或刷新后重试。" />
        <div v-else class="port-usage-list">
          <article v-for="group in processGroups" :key="group.key" class="port-usage-group">
            <header class="port-usage-group__header" :class="{ 'is-unknown': !group.identified }">
              <div>
                <span>占用程序</span>
                <strong class="port-usage-owner__name">{{ group.name }}</strong>
              </div>
              <small v-if="group.pids.length">PID {{ group.pids.join(', ') }}</small>
              <span class="port-usage-group__count">{{ group.entries.length }} 个监听端口</span>
            </header>
            <div class="port-usage-group__ports">
              <section v-for="entry in group.entries" :key="`${entry.protocol}:${entry.localAddress}:${entry.localPort}`" class="port-usage-item">
                <header class="port-usage-item__header">
                  <span class="port-usage-item__port-label">端口</span>
                  <strong>{{ entry.localPort || '未知' }}</strong>
                  <span class="system-resource-state is-on">{{ protocolLabel(entry.protocol) }}</span>
                </header>
                <dl class="port-usage-item__facts">
                  <div><dt>监听范围</dt><dd>{{ listenScope(entry.localAddress) }}</dd></div>
                  <div><dt>监听地址</dt><dd><code>{{ endpoint(entry.localAddress, entry.localPort) }}</code></dd></div>
                </dl>
                <details class="port-usage-item__details">
                  <summary>技术详情</summary>
                  <code>{{ entry.raw }}</code>
                </details>
              </section>
            </div>
          </article>
        </div>
      </template>
    </div>
  </ModalDialog>
</template>

<style scoped>
.port-usage-list {
  display: grid;
  gap: 10px;
}

.port-usage-group {
  display: grid;
  min-width: 0;
  gap: 12px;
  padding: 14px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
}

.port-usage-group__header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--brand-soft);
  border-radius: 9px;
}

.port-usage-group__header.is-unknown {
  background: var(--neutral-soft);
}

.port-usage-group__header > div {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.port-usage-group__header span,
.port-usage-group__header small {
  color: var(--muted);
  font-size: 10px;
}

.port-usage-owner__name {
  overflow: hidden;
  font-size: 14px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.port-usage-group__count {
  padding: 5px 8px;
  color: var(--text) !important;
  background: var(--surface);
  border-radius: 999px;
}

.port-usage-group__ports {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.port-usage-item {
  display: grid;
  min-width: 0;
  gap: 10px;
  padding: 10px;
  background: var(--surface-subtle);
  border-radius: 9px;
}

.port-usage-item__header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.port-usage-item__header strong {
  font-size: 20px;
  line-height: 1;
}

.port-usage-item__header .system-resource-state {
  margin-left: auto;
}

.port-usage-item__port-label,
.port-usage-item__facts dt {
  color: var(--muted);
  font-size: 10px;
  font-weight: 650;
}

.port-usage-item__facts {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin: 0;
}

.port-usage-item__facts div {
  min-width: 0;
}

.port-usage-item__facts dd {
  overflow: hidden;
  margin: 3px 0 0;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.port-usage-item__details {
  color: var(--muted);
  font-size: 10px;
}

.port-usage-item__details summary {
  width: fit-content;
  cursor: pointer;
  user-select: none;
}

.port-usage-item__details code {
  display: block;
  overflow: auto;
  margin-top: 8px;
  padding: 8px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border-radius: 7px;
  white-space: pre;
}

@media (max-width: 760px) {
  .port-usage-group__ports {
    grid-template-columns: 1fr;
  }

  .port-usage-group__header {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .port-usage-group__count {
    grid-column: 1 / -1;
    width: fit-content;
  }
}
</style>
