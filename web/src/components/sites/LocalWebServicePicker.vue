<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ArrowRight, ChevronDown, ChevronUp, LoaderCircle, RefreshCw, Server } from '@lucide/vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import LoadingState from '@/components/feedback/LoadingState.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import {
  discoverLocalWebServiceCandidates,
  collectLocalWebServiceReverseProxyPorts,
  LOCAL_WEB_SERVICE_MAX_CANDIDATES,
  localWebServiceAddressLabel,
  localWebServiceOrigin,
  type LocalWebServiceProxySite,
  type LocalWebServiceCandidate,
} from '@/lib/localWebServices'
import type { PortUsageSnapshot } from '@/types/api'

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const props = withDefaults(
  defineProps<{
    readable?: boolean
    unavailableReason?: string
    existingSites?: ReadonlyArray<LocalWebServiceProxySite>
  }>(),
  {
    readable: true,
    unavailableReason: '',
  },
)

const emit = defineEmits<{ select: [origin: string] }>()

const open = ref(false)
const loading = ref(false)
const error = ref('')
const snapshot = ref<PortUsageSnapshot>()
const candidates = ref<LocalWebServiceCandidate[]>([])
const reverseProxyPorts = computed(() => collectLocalWebServiceReverseProxyPorts(props.existingSites || []))
let controller: AbortController | undefined

function isAbortError(reason: unknown): boolean {
  return reason instanceof DOMException && reason.name === 'AbortError'
}

async function scan(): Promise<void> {
  if (!props.readable || loading.value) return

  controller?.abort()
  const requestController = new AbortController()
  controller = requestController
  open.value = true
  loading.value = true
  error.value = ''

  try {
    const nextSnapshot = await api.system.portUsage(requestController.signal)
    if (requestController.signal.aborted) return
    snapshot.value = nextSnapshot
    candidates.value = discoverLocalWebServiceCandidates(nextSnapshot.entries)
  } catch (reason) {
    if (isAbortError(reason)) return
    error.value = reason instanceof ApiError ? reason.message : '无法读取端口占用状态。'
  } finally {
    if (controller === requestController) {
      controller = undefined
      loading.value = false
    }
  }
}

function toggleCandidates(): void {
  if (snapshot.value) open.value = !open.value
  else void scan()
}

function selectCandidate(candidate: LocalWebServiceCandidate): void {
  emit('select', localWebServiceOrigin(candidate))
  open.value = false
}

function addressSummary(candidate: LocalWebServiceCandidate): string {
  return candidate.addresses.length
    ? candidate.addresses.map(localWebServiceAddressLabel).join('、')
    : phrase('仅本机')
}

function processSummary(candidate: LocalWebServiceCandidate): string {
  const process = candidate.processes.length ? candidate.processes.join('、') : phrase('系统未返回占用程序')
  return candidate.pids.length ? `${process} · PID ${candidate.pids.join('、')}` : process
}

function containerSummary(candidate: LocalWebServiceCandidate): string {
  return candidate.containers.map((container) => {
    const name = container.name.trim() || container.id.slice(0, 12)
    const port = container.containerPort ? phrase(` · 容器端口 ${container.containerPort}`) : ''
    return `${name || phrase('未命名容器')}${port}`
  }).join('、')
}

function isReverseProxied(candidate: LocalWebServiceCandidate): boolean {
  return reverseProxyPorts.value.has(candidate.port)
}

watch(() => props.readable, (readable) => {
  if (!readable) controller?.abort()
})

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="local-web-service-picker" :aria-label="phrase('本机 Web 服务选择')">
    <div class="local-web-service-picker__toolbar">
      <div class="local-web-service-picker__intro">
        <span class="local-web-service-picker__icon" aria-hidden="true"><Server :size="18" /></span>
        <span>
          <strong>{{ phrase('本机 Web 服务') }}</strong>
          <small>{{ phrase('读取 TCP 监听端口，选择后自动填入上游地址。') }}</small>
        </span>
      </div>
      <div class="local-web-service-picker__actions">
        <button
          v-if="snapshot"
          class="local-web-service-picker__view"
          type="button"
          :aria-expanded="open"
          @click="toggleCandidates"
        >
          <ChevronUp v-if="open" :size="15" />
          <ChevronDown v-else :size="15" />
          {{ phrase(open ? '收起候选' : '查看候选') }}
        </button>
        <button
          class="local-web-service-picker__scan"
          type="button"
          :disabled="!readable || loading"
          :aria-label="phrase(snapshot ? '重新扫描本机服务' : '扫描本机服务')"
          @click="scan"
        >
          <LoaderCircle v-if="loading" class="spin" :size="15" />
          <RefreshCw v-else :size="15" />
          {{ phrase(snapshot ? '重新扫描' : '扫描本机服务') }}
        </button>
      </div>
    </div>

    <div v-if="!readable" class="local-web-service-picker__alert local-web-service-picker__alert--warning" role="status">
      {{ phrase(unavailableReason || '当前 Agent 的端口占用适配器未就绪。') }}{{ phrase('仍可手动填写上游地址。') }}
    </div>

    <div v-else-if="open" class="local-web-service-picker__results">
      <header class="local-web-service-picker__results-header">
        <div>
          <strong>{{ phrase('可选择的 TCP 监听端口') }}</strong>
          <small>{{ phrase('仅按监听状态整理，不主动访问服务；已有反代的端口会标记，默认填入 HTTP。') }}</small>
        </div>
        <span v-if="loading" class="local-web-service-picker__updating" role="status">
          <LoaderCircle class="spin" :size="14" /> {{ phrase('更新中') }}
        </span>
      </header>

      <div v-if="error && snapshot" class="local-web-service-picker__alert local-web-service-picker__alert--danger" role="alert">
        {{ phrase(error) }} <button type="button" @click="scan">{{ phrase('重试') }}</button>
      </div>
      <ErrorState v-if="error && !snapshot" :message="error" @retry="scan" />
      <LoadingState v-else-if="loading && !snapshot" :rows="2" :label="phrase('正在扫描本机服务')" />
      <template v-else>
        <div class="local-web-service-picker__meta" role="status">
          <span>{{ phrase(`发现 ${candidates.length} 个端口候选`) }}</span>
          <span v-if="snapshot?.truncated || candidates.length >= LOCAL_WEB_SERVICE_MAX_CANDIDATES">
            {{ phrase(`端口较多，仅显示前 ${LOCAL_WEB_SERVICE_MAX_CANDIDATES} 个`) }}
          </span>
        </div>

        <div v-if="candidates.length" class="local-web-service-picker__grid">
          <button
            v-for="candidate in candidates"
            :key="candidate.port"
            class="local-web-service-picker__candidate"
            type="button"
            :aria-label="phrase(`使用本机 TCP 端口 ${candidate.port}${isReverseProxied(candidate) ? '，已反代' : ''}`)"
            @click="selectCandidate(candidate)"
          >
            <span class="local-web-service-picker__candidate-top">
              <span class="local-web-service-picker__port">
                <Server :size="16" aria-hidden="true" />
                <strong>{{ candidate.port }}</strong>
                <span>TCP</span>
              </span>
              <span v-if="isReverseProxied(candidate)" class="local-web-service-picker__proxy-status" :title="phrase('已有 k fd 反代配置')">
                {{ phrase('已反代') }}
              </span>
              <span class="local-web-service-picker__use">{{ phrase('填入') }} <ArrowRight :size="15" /></span>
            </span>
            <span class="local-web-service-picker__detail">{{ phrase(`监听：${addressSummary(candidate)}`) }}</span>
            <span class="local-web-service-picker__detail">{{ phrase(`服务：${processSummary(candidate)}`) }}</span>
            <span v-if="candidate.containers.length" class="local-web-service-picker__detail local-web-service-picker__container">
              {{ phrase(`Docker 容器：${containerSummary(candidate)}`) }}
            </span>
          </button>
        </div>
        <div v-else class="local-web-service-picker__empty" role="status">
          <strong>{{ phrase('未发现 TCP 监听端口') }}</strong>
          <span>{{ phrase('请确认本机 Web 服务已启动，或直接手动填写上游地址。') }}</span>
        </div>
      </template>
    </div>
  </section>
</template>

<style scoped>
.local-web-service-picker {
  display: grid;
  gap: 10px;
  margin-top: -2px;
}

.local-web-service-picker__toolbar,
.local-web-service-picker__results {
  min-width: 0;
  padding: 12px 14px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 12px;
}

.local-web-service-picker__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.local-web-service-picker__intro,
.local-web-service-picker__actions,
.local-web-service-picker__candidate-top,
.local-web-service-picker__port,
.local-web-service-picker__use,
.local-web-service-picker__updating {
  display: flex;
  align-items: center;
}

.local-web-service-picker__intro {
  min-width: 0;
  gap: 9px;
}

.local-web-service-picker__intro > span:last-child,
.local-web-service-picker__results-header > div {
  display: grid;
  min-width: 0;
  gap: 3px;
}

.local-web-service-picker__intro strong,
.local-web-service-picker__results-header strong {
  color: var(--text);
  font-size: 14px;
}

.local-web-service-picker__intro small,
.local-web-service-picker__results-header small {
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.local-web-service-picker__icon {
  display: grid;
  width: 32px;
  height: 32px;
  flex: 0 0 auto;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border: 1px solid var(--brand-muted);
  border-radius: 9px;
}

.local-web-service-picker__actions {
  flex: 0 0 auto;
  gap: 7px;
}

.local-web-service-picker__scan,
.local-web-service-picker__view {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 11px;
  border-radius: 9px;
  cursor: pointer;
  font-size: 14px;
  white-space: nowrap;
}

.local-web-service-picker__scan {
  color: var(--on-brand);
  background: var(--brand-action);
  border: 1px solid var(--brand-action);
}

.local-web-service-picker__scan:hover:not(:disabled) {
  background: var(--brand-strong);
  border-color: var(--brand-strong);
}

.local-web-service-picker__view {
  color: var(--text-soft);
  background: var(--surface);
  border: 1px solid var(--border);
}

.local-web-service-picker__view:hover,
.local-web-service-picker__view:focus-visible {
  color: var(--text);
  border-color: var(--border-strong);
}

.local-web-service-picker__scan:disabled {
  cursor: not-allowed;
  opacity: .55;
}

.local-web-service-picker__scan:focus-visible,
.local-web-service-picker__candidate:focus-visible,
.local-web-service-picker__alert button:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: 2px;
}

.local-web-service-picker__alert {
  padding: 10px 12px;
  border: 1px solid;
  border-radius: 9px;
  font-size: 13px;
  line-height: 1.5;
}

.local-web-service-picker__alert--warning {
  color: var(--amber);
  background: var(--amber-soft);
  border-color: color-mix(in srgb, var(--amber) 28%, var(--border));
}

.local-web-service-picker__alert--danger {
  color: var(--danger);
  background: var(--danger-soft);
  border-color: color-mix(in srgb, var(--danger) 25%, var(--border));
}

.local-web-service-picker__alert button {
  padding: 0;
  color: inherit;
  background: transparent;
  border: 0;
  cursor: pointer;
  font: inherit;
  text-decoration: underline;
}

.local-web-service-picker__results {
  display: grid;
  gap: 11px;
  background: var(--surface);
}

.local-web-service-picker__results-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.local-web-service-picker__updating {
  flex: 0 0 auto;
  gap: 5px;
  color: var(--muted);
  font-size: 13px;
}

.local-web-service-picker__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 12px;
  color: var(--muted);
  font-size: 13px;
}

.local-web-service-picker__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 8px;
}

.local-web-service-picker__candidate {
  display: grid;
  min-width: 0;
  min-height: 112px;
  gap: 10px;
  padding: 12px;
  color: var(--text-soft);
  text-align: left;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 10px;
  cursor: pointer;
  transition: border-color .14s ease, background .14s ease, transform .14s ease;
}

.local-web-service-picker__candidate:hover {
  background: var(--interaction-hover-subtle);
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border-strong));
  transform: translateY(-1px);
}

.local-web-service-picker__candidate-top {
  justify-content: space-between;
  gap: 8px;
}

.local-web-service-picker__port {
  min-width: 0;
  gap: 7px;
  color: var(--brand-strong);
}

.local-web-service-picker__port strong {
  color: var(--text);
  font-size: 20px;
  line-height: 1;
}

.local-web-service-picker__port > span {
  padding: 3px 6px;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: 999px;
  font-size: 12px;
}

.local-web-service-picker__use {
  flex: 0 0 auto;
  gap: 3px;
  color: var(--brand-strong);
  font-size: 13px;
}

.local-web-service-picker__proxy-status {
  flex: 0 0 auto;
  padding: 3px 7px;
  color: var(--success);
  background: var(--success-soft);
  border: 1px solid color-mix(in srgb, var(--success) 28%, var(--border));
  border-radius: 999px;
  font-size: 12px;
  font-weight: 680;
  white-space: nowrap;
}

.local-web-service-picker__detail {
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
  overflow-wrap: anywhere;
  text-overflow: ellipsis;
}

.local-web-service-picker__container {
  color: var(--brand-strong);
  font-weight: 600;
}

.local-web-service-picker__empty {
  display: grid;
  gap: 4px;
  padding: 18px 12px;
  color: var(--muted);
  text-align: center;
  font-size: 13px;
  line-height: 1.5;
}

.local-web-service-picker__empty strong {
  color: var(--text-soft);
  font-size: 14px;
}

@media (max-width: 560px) {
  .local-web-service-picker__toolbar,
  .local-web-service-picker__results-header {
    align-items: stretch;
    flex-direction: column;
  }

  .local-web-service-picker__actions {
    width: 100%;
  }

  .local-web-service-picker__scan,
  .local-web-service-picker__view {
    flex: 1;
  }

  .local-web-service-picker__grid {
    grid-template-columns: 1fr;
  }
}
</style>
