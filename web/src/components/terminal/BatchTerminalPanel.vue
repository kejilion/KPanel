<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CheckCircle2, ChevronDown, LoaderCircle, Play, TerminalSquare, XCircle } from '@lucide/vue'
import StatusBadge from '@/components/feedback/StatusBadge.vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import { drainTerminalInputQueue, TerminalInputQueue } from '@/lib/terminalInput'
import type { ClusterHost } from '@/types/api'

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const props = defineProps<{
  hosts: ClusterHost[]
  sessionCapacity: number
}>()
const emit = defineEmits<{ runningChange: [running: boolean] }>()

type ExecutionState = 'queued' | 'connecting' | 'running' | 'succeeded' | 'failed' | 'timed_out'

interface BatchExecutionResult {
  host: ClusterHost
  state: ExecutionState
  output: string
  rawOutput: string
  truncated: boolean
  error: string
}

const MAX_CONCURRENCY = 4
const MAX_COMMAND_BYTES = 16 * 1024
const MAX_CAPTURE_CHARACTERS = 128 * 1024
const EXECUTION_TIMEOUT_MS = 30 * 60 * 1000
const encoder = new TextEncoder()

const command = ref('')
const results = ref<BatchExecutionResult[]>([])
const executing = ref(false)
const expandedHosts = ref<Set<string>>(new Set())
const activeSessions = new Map<string, string>()
const outputDecoders = new Map<string, TextDecoder>()
let runController: AbortController | undefined
let runIdentity = 0

const commandBytes = computed(() => encoder.encode(command.value).byteLength)
const commandError = computed(() => commandBytes.value > MAX_COMMAND_BYTES
  ? phrase('命令不能超过 16 KiB。')
  : '')
const canExecute = computed(() => !executing.value
  && command.value.trim().length > 0
  && !commandError.value
  && props.hosts.length > 0
  && props.sessionCapacity > 0)
const completedCount = computed(() => results.value.filter((item) =>
  item.state === 'succeeded' || item.state === 'failed' || item.state === 'timed_out').length)
const succeededCount = computed(() => results.value.filter((item) => item.state === 'succeeded').length)
const failedCount = computed(() => results.value.filter((item) => item.state === 'failed' || item.state === 'timed_out').length)

watch(executing, (value) => emit('runningChange', value), { immediate: true })

function operatingSystemIdentity(host: ClusterHost) {
  return detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)
}

function toggleExpanded(hostID: string): void {
  const next = new Set(expandedHosts.value)
  if (next.has(hostID)) next.delete(hostID)
  else next.add(hostID)
  expandedHosts.value = next
}

function expandAll(): void {
  expandedHosts.value = new Set(results.value.map((item) => item.host.id))
}

function collapseAll(): void {
  expandedHosts.value = new Set()
}

const allExpanded = computed(() => results.value.length > 0 && results.value.every((item) => expandedHosts.value.has(item.host.id)))

function executionLabel(state: ExecutionState): string {
  return phrase({
    queued: '等待执行', connecting: '正在连接', running: '命令执行中', succeeded: '执行成功',
    failed: '执行失败', timed_out: '执行超时',
  }[state])
}

function badgeStatus(state: ExecutionState): string {
  if (state === 'succeeded') return 'succeeded'
  if (state === 'failed') return 'failed'
  if (state === 'timed_out') return 'expired'
  if (state === 'running') return 'running_job'
  return 'queued'
}

function encodeBase64(value: string): string {
  const bytes = encoder.encode(value)
  let binary = ''
  for (let offset = 0; offset < bytes.length; offset += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + 0x8000))
  }
  return btoa(binary)
}

function decodeBase64(value: string): Uint8Array {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=')
  const binary = atob(padded)
  return Uint8Array.from(binary, (character) => character.charCodeAt(0))
}

function plainTerminalOutput(value: string): string {
  return value
    .replace(/\x1B\][^\x07]*(?:\x07|\x1B\\)/g, '')
    .replace(/\x1B(?:\[[0-?]*[ -/]*[@-~]|[@-_][0-?]*[ -/]*[@-~])/g, '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, '')
    .trimEnd()
}

function appendOutput(result: BatchExecutionResult, hostID: string, data: string, final = false): void {
  const decoder = outputDecoders.get(hostID) || new TextDecoder()
  outputDecoders.set(hostID, decoder)
  if (data) result.rawOutput += decoder.decode(decodeBase64(data), { stream: !final })
  if (final) result.rawOutput += decoder.decode()
  if (result.rawOutput.length > MAX_CAPTURE_CHARACTERS) {
    result.rawOutput = result.rawOutput.slice(-MAX_CAPTURE_CHARACTERS)
    result.truncated = true
  }
  result.output = plainTerminalOutput(result.rawOutput)
}

async function sendTerminalText(sessionID: string, value: string, signal: AbortSignal): Promise<void> {
  const queue = new TerminalInputQueue()
  queue.append(value)
  await drainTerminalInputQueue(
    queue,
    () => !signal.aborted,
    (chunk) => api.terminals.input(sessionID, encodeBase64(chunk)).then(() => undefined),
  )
  if (signal.aborted) throw new Error('batch terminal execution aborted')
}

function friendlyExecutionError(reason: unknown): string {
  if (reason instanceof ApiError) {
    if (reason.code === 'terminal_limit') return phrase('终端会话已满，请关闭其他终端后重试。')
    if (reason.code === 'terminal_unavailable') return phrase('目标主机的终端当前不可用。')
    if (reason.code === 'terminal_not_found') return phrase('目标终端连接已中断。')
  }
  return phrase('连接或执行失败，请重试。')
}

async function closeSession(hostID: string, sessionID: string): Promise<void> {
  if (activeSessions.get(hostID) !== sessionID) return
  activeSessions.delete(hostID)
  await api.terminals.close(sessionID).catch(() => undefined)
}

async function executeHost(hostID: string, submittedCommand: string, identity: number, signal: AbortSignal): Promise<void> {
  const result = results.value.find((item) => item.host.id === hostID)
  if (!result || identity !== runIdentity || signal.aborted) return
  result.state = 'connecting'
  let sessionID = ''
  let exited = false
  try {
    const opened = await api.terminals.open(hostID, 30, 120)
    sessionID = opened.sessionId
    activeSessions.set(hostID, sessionID)
    if (identity !== runIdentity || signal.aborted) {
      await closeSession(hostID, sessionID)
      return
    }
    result.state = 'running'
    const lineEnding = /[\r\n]$/.test(submittedCommand) ? '' : '\r'
    await sendTerminalText(sessionID, `${submittedCommand}${lineEnding}exit\r`, signal)

    let offset = opened.offset
    const deadline = Date.now() + EXECUTION_TIMEOUT_MS
    while (!signal.aborted && identity === runIdentity) {
      if (Date.now() >= deadline) {
        result.state = 'timed_out'
        result.error = phrase('超过 30 分钟，终端会话已停止。')
        await closeSession(hostID, sessionID)
        return
      }
      const chunk = await api.terminals.output(sessionID, offset, signal)
      offset = chunk.nextOffset
      result.truncated = result.truncated || chunk.truncated
      const finished = Boolean(chunk.exitedAt || chunk.closed)
      appendOutput(result, hostID, chunk.data, finished)
      if (!finished) continue
      exited = true
      activeSessions.delete(hostID)
      if (chunk.exitError) {
        result.state = 'failed'
        result.error = chunk.exitError
      } else {
        result.state = 'succeeded'
      }
      return
    }
  } catch (reason) {
    if (identity !== runIdentity || signal.aborted) return
    result.state = 'failed'
    result.error = friendlyExecutionError(reason)
  } finally {
    outputDecoders.delete(hostID)
    if (sessionID && !exited && activeSessions.get(hostID) === sessionID) {
      await closeSession(hostID, sessionID)
    }
  }
}

async function execute(): Promise<void> {
  if (!canExecute.value) return
  const targets = [...props.hosts]
  const submittedCommand = command.value
  const identity = ++runIdentity
  runController?.abort()
  const current = new AbortController()
  runController = current
  executing.value = true
  results.value = targets.map((host) => ({
    host, state: 'queued', output: '', rawOutput: '', truncated: false, error: '',
  }))
  expandedHosts.value = new Set()
  const queue = targets.map((host) => host.id)
  const worker = async () => {
    while (!current.signal.aborted && identity === runIdentity) {
      const hostID = queue.shift()
      if (!hostID) return
      await executeHost(hostID, submittedCommand, identity, current.signal)
    }
  }
  try {
    const concurrency = Math.min(MAX_CONCURRENCY, props.sessionCapacity, queue.length)
    await Promise.all(Array.from({ length: concurrency }, () => worker()))
  } finally {
    if (identity === runIdentity) executing.value = false
  }
}

onBeforeUnmount(() => {
  runController?.abort()
  runIdentity += 1
  for (const [hostID, sessionID] of activeSessions) void closeSession(hostID, sessionID)
  emit('runningChange', false)
})
</script>

<template>
  <section class="batch-terminal-panel" aria-label="批量执行">
    <div class="batch-command">
      <header>
        <div><TerminalSquare :size="19" aria-hidden="true" /><span><strong>{{ phrase('自定义命令') }}</strong><small>{{ phrase(`已选择 ${hosts.length} 台主机`) }}</small></span></div>
      </header>
      <label>
        <span class="sr-only">{{ phrase('输入自定义命令') }}</span>
        <textarea v-model="command" :disabled="executing" rows="4" spellcheck="false" :placeholder="phrase('例如：uname -a')" />
      </label>
      <footer>
        <p v-if="commandError" class="batch-command__error" role="alert">{{ commandError }}</p>
        <p v-else-if="sessionCapacity < 1" class="batch-command__error" role="alert">{{ phrase('请先关闭至少一个交互终端。') }}</p>
        <p v-else>{{ phrase('页面关闭会终止仍在执行的命令。') }}</p>
        <button class="button button--primary" type="button" :disabled="!canExecute" @click="execute">
          <LoaderCircle v-if="executing" class="spin" :size="18" aria-hidden="true" />
          <Play v-else :size="18" aria-hidden="true" />
          {{ executing ? phrase('正在执行…') : phrase('执行命令') }}
        </button>
      </footer>
    </div>

    <div class="batch-status">
      <header>
        <div><strong>{{ phrase('执行情况') }}</strong><small v-if="results.length">{{ phrase(`已完成 ${completedCount} / ${results.length} 台`) }}</small></div>
        <div v-if="results.length" class="batch-status__actions">
          <div class="batch-status__counts">
            <span><CheckCircle2 :size="15" aria-hidden="true" /> {{ phrase(`成功 ${succeededCount}`) }}</span>
            <span><XCircle :size="15" aria-hidden="true" /> {{ phrase(`失败 ${failedCount}`) }}</span>
          </div>
          <button type="button" class="batch-status__toggle" @click="allExpanded ? collapseAll() : expandAll()">
            {{ phrase(allExpanded ? '全部收起' : '全部展开') }}
          </button>
        </div>
      </header>
      <div v-if="!results.length" class="batch-status__empty">
        <span><TerminalSquare :size="28" aria-hidden="true" /></span>
        <strong>{{ phrase('等待执行') }}</strong>
        <p>{{ phrase('在左侧选择主机，输入命令后开始执行。') }}</p>
      </div>
      <div v-else class="batch-result-list">
        <article v-for="result in results" :key="result.host.id" class="batch-result" :class="{ 'is-expanded': expandedHosts.has(result.host.id) }">
          <button
            type="button"
            class="batch-result__summary"
            :aria-expanded="expandedHosts.has(result.host.id)"
            :aria-controls="`batch-result-detail-${result.host.id}`"
            :aria-label="phrase('展开输出')"
            @click="toggleExpanded(result.host.id)"
          >
            <OperatingSystemIcon class="batch-result__os" :distro="operatingSystemIdentity(result.host).key" :label="operatingSystemIdentity(result.host).label" :show-tooltip="false" />
            <strong data-i18n-ignore>{{ result.host.name }}</strong>
            <StatusBadge :status="badgeStatus(result.state)" :label="executionLabel(result.state)" />
            <ChevronDown :size="16" class="batch-result__chevron" aria-hidden="true" />
          </button>
          <div :id="`batch-result-detail-${result.host.id}`" v-show="expandedHosts.has(result.host.id)" class="batch-result__detail">
            <pre v-if="result.output" data-i18n-ignore>{{ result.output }}</pre>
            <div v-else-if="result.state === 'queued' || result.state === 'connecting' || result.state === 'running'" class="batch-result__pending"><LoaderCircle class="spin" :size="16" aria-hidden="true" /> {{ executionLabel(result.state) }}</div>
            <p v-if="result.truncated" class="batch-result__notice">{{ phrase('输出已截断，仅保留最后一部分。') }}</p>
            <p v-if="result.error" class="batch-result__error" role="alert"><span data-i18n-ignore>{{ result.error }}</span></p>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.batch-terminal-panel { display: grid; min-width: 0; min-height: 0; grid-template-rows: auto minmax(0, 1fr); color: var(--terminal-shell-text, #d8dddc); background: var(--terminal-shell-background, #0b1214); }
.batch-command { padding: 18px; border-bottom: 1px solid var(--terminal-shell-border, #29383a); background: var(--terminal-shell-panel, #111a1d); }
.batch-command > header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 12px; }
.batch-command > header > div { display: flex; align-items: center; gap: 9px; }
.batch-command > header svg { color: var(--brand); }
.batch-command > header span { display: grid; gap: 2px; }
.batch-command strong { font-size: 15px; }
.batch-command small { color: var(--terminal-shell-muted, #8a9695); font-size: 13px; }
.batch-command label { display: block; }
.batch-command textarea { width: 100%; min-height: 104px; resize: vertical; padding: 12px 14px; color: var(--terminal-shell-text, #d8dddc); background: var(--terminal-shell-background, #0b1214); border: 1px solid var(--terminal-shell-border, #29383a); border-radius: var(--radius-sm); outline: none; font: 14px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; tab-size: 2; }
.batch-command textarea:focus { border-color: var(--brand); box-shadow: 0 0 0 2px color-mix(in srgb, var(--brand) 18%, transparent); }
.batch-command textarea:disabled { cursor: wait; opacity: .72; }
.batch-command textarea::placeholder { color: var(--terminal-shell-muted, #8a9695); }
.batch-command footer { display: flex; min-height: 42px; align-items: end; justify-content: space-between; gap: 16px; margin-top: 10px; }
.batch-command footer p { margin: 0; color: var(--terminal-shell-muted, #8a9695); font-size: 13px; line-height: 1.45; }
.batch-command footer p.batch-command__error { color: var(--danger); }
.batch-status { display: grid; min-height: 0; grid-template-rows: auto minmax(0, 1fr); }
.batch-status > header { display: flex; min-height: 58px; align-items: center; justify-content: space-between; gap: 14px; padding: 10px 18px; border-bottom: 1px solid var(--terminal-shell-border, #29383a); }
.batch-status > header > div:first-child { display: grid; gap: 2px; }
.batch-status > header strong { font-size: 14px; }
.batch-status > header small { color: var(--terminal-shell-muted, #8a9695); font-size: 13px; }
.batch-status__counts { display: flex; flex-wrap: wrap; gap: 12px; }
.batch-status__counts span { display: inline-flex; align-items: center; gap: 5px; color: var(--terminal-shell-muted, #8a9695); font-size: 13px; }
.batch-status__counts span:first-child svg { color: var(--success); }
.batch-status__counts span:last-child svg { color: var(--danger); }
.batch-status__actions { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.batch-status__toggle { padding: 5px 12px; border: 1px solid var(--terminal-shell-border, #29383a); border-radius: var(--radius-sm); color: var(--terminal-shell-text, #d8dddc); background: transparent; font: inherit; font-size: 13px; cursor: pointer; transition: border-color .16s ease, color .16s ease; }
.batch-status__toggle:hover, .batch-status__toggle:focus-visible { border-color: var(--brand); color: var(--brand); outline: none; }
.batch-status__empty { display: grid; place-content: center; justify-items: center; gap: 7px; padding: 24px; text-align: center; }
.batch-status__empty > span { display: grid; width: 52px; height: 52px; place-items: center; color: var(--brand); background: color-mix(in srgb, var(--brand) 12%, transparent); border-radius: var(--radius); }
.batch-status__empty strong { font-size: 16px; }
.batch-status__empty p { margin: 0; color: var(--terminal-shell-muted, #8a9695); font-size: 14px; }
.batch-result-list { min-height: 0; overflow-y: auto; overscroll-behavior: contain; padding: 12px 16px 18px; }
.batch-result { min-width: 0; margin-bottom: 10px; overflow: hidden; border: 1px solid var(--terminal-shell-border, #29383a); border-radius: var(--radius-sm); background: var(--terminal-shell-panel, #111a1d); }
.batch-result:last-child { margin-bottom: 0; }
.batch-result > header { display: flex; min-height: 58px; align-items: center; gap: 10px; padding: 10px 12px; border-bottom: 1px solid var(--terminal-shell-border, #29383a); }
.batch-result__summary { display: flex; width: 100%; min-height: 58px; align-items: center; gap: 10px; padding: 10px 12px; border: 0; color: inherit; background: transparent; font: inherit; text-align: left; cursor: pointer; }
.batch-result__summary:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
.batch-result__summary strong { min-width: 0; flex: 1; overflow-wrap: anywhere; font-size: 14px; }
.batch-result__chevron { flex: 0 0 auto; color: var(--terminal-shell-muted, #8a9695); transition: transform .16s ease; }
.batch-result.is-expanded > .batch-result__summary { border-bottom: 1px solid var(--terminal-shell-border, #29383a); }
.batch-result.is-expanded > .batch-result__summary .batch-result__chevron { transform: rotate(180deg); color: var(--brand); }
.batch-result__detail { display: grid; min-width: 0; }
.batch-result :deep(.batch-result__os) { width: 34px; height: 34px; flex: 0 0 auto; border-radius: var(--radius-sm); box-shadow: none; }
.batch-result pre { max-height: 190px; margin: 0; overflow: auto; padding: 12px 14px; color: var(--terminal-shell-text, #d8dddc); background: var(--terminal-shell-background, #0b1214); font: 13px/1.55 ui-monospace, SFMono-Regular, Consolas, monospace; white-space: pre-wrap; overflow-wrap: anywhere; }
.batch-result__pending { display: flex; min-height: 64px; align-items: center; justify-content: center; gap: 7px; color: var(--terminal-shell-muted, #8a9695); font-size: 14px; }
.batch-result__notice, .batch-result__error { margin: 0; padding: 9px 13px; font-size: 13px; line-height: 1.45; }
.batch-result__notice { color: var(--warning); }
.batch-result__error { color: var(--danger); }

@media (max-width: 720px) {
  .batch-command { padding: 14px; }
  .batch-command footer, .batch-status > header { align-items: stretch; flex-direction: column; }
  .batch-command footer .button { width: 100%; justify-content: center; }
}
</style>
