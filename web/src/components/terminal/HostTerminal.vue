<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import { api, ApiError, terminalStream } from '@/lib/api'
import { TerminalDuplexInput } from '@/lib/terminalDuplexInput'
import type { TerminalStreamSubscription } from '@/lib/terminalStream'
import type { TerminalOutput } from '@/types/api'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import {
  terminalEnterShouldSubmit,
  terminalInputFlushInterval,
  terminalInputShouldFlushImmediately,
  terminalLineSubmission,
} from '@/lib/terminalInput'
import { createTerminalTouchScroll } from '@/lib/terminalTouchScroll'
import { readTerminalTheme } from '@/lib/terminalTheme'
import { TerminalWriteFlow } from '@/lib/terminalWriteFlow'
import { useI18n } from '@/i18n'
import { useTerminalActivity } from '@/composables/useTerminalActivity'
import { useTheme } from '@/stores/theme'

const { t } = useI18n()
const { colors: themeColors, resolved: resolvedTheme } = useTheme()
// Output streams while the window is on screen, focused or not, and pauses
// without closing the server session while it is minimized or the tab hidden.
const { streaming } = useTerminalActivity()

const props = defineProps<{
  sessionId: string
  hostName: string
  initialOffset: number
}>()

const emit = defineEmits<{
  stateChange: [state: 'connecting' | 'connected' | 'reconnecting' | 'finished']
}>()

const host = ref<HTMLElement>()
const clipboardMenu = ref<InstanceType<typeof TerminalContextMenu>>()
const pendingLine = ref('')
const inputBlocked = ref(false)
let duplexInput: TerminalDuplexInput | undefined
const state = ref<'connecting' | 'connected' | 'reconnecting' | 'finished'>('connecting')
let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let observer: ResizeObserver | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let streamSubscription: TerminalStreamSubscription | null = null
let inputTimer: number | undefined
let resizeTimer: number | undefined
// A freshly opened terminal starts at offset 0. Keep the client resilient to
// older Panel responses that did not include the initial offset field.
let offset = Number.isFinite(props.initialOffset) && props.initialOffset >= 0 ? props.initialOffset : 0
let disposed = false
let mounted = false
const resizeRetryLimit = 3
let syncedRows = 0
let syncedColumns = 0
let resizeSequence = 0
let resizeFailures = 0
let reconnectAttempts = 0
let closeRequest: Promise<void> | undefined
let closeConfirmed = false
// Bumped whenever output stops so a late poll result cannot be applied twice.
let outputGeneration = 0
const writeFlow = new TerminalWriteFlow(() => startOutput())

function closeSession(): Promise<void> {
	duplexInput?.close()
	inputBlocked.value = true
  if (closeConfirmed) return Promise.resolve()
  if (closeRequest) return closeRequest
  closeRequest = api.terminals.close(props.sessionId).then((result) => {
    if (!result.closed) throw new Error('Terminal close was not confirmed')
    closeConfirmed = true
  }).catch((reason: unknown) => {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      closeConfirmed = true
      return
    }
    throw reason
  }).finally(() => { closeRequest = undefined })
  return closeRequest
}

watch(state, (value) => emit('stateChange', value), { immediate: true })

function decodeBase64(value: string): Uint8Array {
  const decoded = window.atob(value)
  return Uint8Array.from(decoded, (character) => character.charCodeAt(0))
}

function isFollowingOutput(): boolean {
  const buffer = terminal?.buffer.active
  return !buffer || buffer.viewportY >= buffer.baseY
}

function writeTerminalOutput(data: string | Uint8Array): void {
  if (!terminal || data.length === 0) return
  const follow = isFollowingOutput()
  const parsed = writeFlow.track(data.length)
  terminal.write(data, () => {
    parsed()
    if (follow) terminal?.scrollToBottom()
  })
  // The offset is kept, so output resumes exactly where it paused.
  if (writeFlow.blocked) stopOutput()
}

function focusTerminal(): void {
  terminal?.focus()
}

function containTerminalWheel(event: WheelEvent): void {
  containWheelScroll(event, host.value?.querySelector<HTMLElement>('.xterm-viewport, .xterm-scrollable-element'))
}

const terminalTouchScroll = createTerminalTouchScroll({
  getTerminal: () => terminal,
  getScreen: () => host.value?.querySelector<HTMLElement>('.xterm-screen') ?? host.value,
})

function flushInput(): void {
  if (disposed || state.value === 'finished') return
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  duplexInput?.flush()
}

function queueInput(data: string): boolean {
  if (disposed || inputBlocked.value || state.value === 'finished') return false
  if (!duplexInput?.append(data)) return false
  if (terminalInputShouldFlushImmediately(data) || duplexInput.byteLength >= 2048) {
    void flushInput()
  } else if (!inputTimer) {
    inputTimer = window.setTimeout(() => void flushInput(), terminalInputFlushInterval)
  }
  return true
}

function submitPendingLine(): void {
  if (!pendingLine.value || state.value === 'finished') return
  const value = terminalLineSubmission(pendingLine.value)
  if (queueInput(value)) pendingLine.value = ''
}

function executeCommand(command: string): boolean {
  if (!command.trim() || disposed || state.value === 'finished') return false
  if (!queueInput(terminalLineSubmission(command.replace(/\r\n?/g, '\n')))) return false
  focusTerminal()
  return true
}

function handlePendingLineEnter(event: KeyboardEvent): void {
  if (!terminalEnterShouldSubmit(event)) return
  event.preventDefault()
  submitPendingLine()
}

function applyChunk(chunk: TerminalOutput): void {
  if (chunk.truncated) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.outputTruncated')}\x1b[0m\r\n`)
  if (chunk.data) writeTerminalOutput(decodeBase64(chunk.data))
  offset = chunk.nextOffset
  const recovered = state.value === 'reconnecting'
  state.value = chunk.closed || chunk.exitedAt ? 'finished' : 'connected'
  reconnectAttempts = 0
  if (recovered && state.value === 'connected') {
    syncedRows = 0
    syncedColumns = 0
    resizeFailures = 0
    scheduleResize()
  }
  if (state.value === 'connected' && duplexInput?.byteLength) flushInput()
  if (chunk.exitError) writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${chunk.exitError}\x1b[0m\r\n`)
  if (state.value === 'finished') { stopOutput(); duplexInput?.close() }
}

// applyChunk may finish the session; read the state through a call so the
// compiler does not narrow it across that mutation.
function terminalFinished(): boolean {
  return state.value === 'finished'
}

function canReceiveOutput(): boolean {
  return mounted && !disposed && streaming.value && !writeFlow.blocked && !terminalFinished()
}

// Output prefers the tab's shared push stream and falls back to long-polling
// when the stream is unavailable; both paths feed applyChunk.
function startOutput(): void {
  if (!canReceiveOutput() || streamSubscription || pollController) return
  streamSubscription = terminalStream.subscribe({ kind: 'terminal', id: props.sessionId, offset }, {
    output: applyChunk,
    error: (code) => {
      if (code === 'terminal_not_found') {
        state.value = 'finished'
        stopOutput()
        return
      }
      state.value = 'reconnecting'
    },
    unavailable: () => {
      streamSubscription = null
      void poll()
    },
  })
  if (!streamSubscription) void poll()
}

function stopOutput(): void {
  outputGeneration++
  streamSubscription?.close()
  streamSubscription = null
  pollController?.abort()
  pollController = undefined
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = undefined
}

function schedulePoll(delay: number): void {
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(() => {
    pollTimer = undefined
    void poll()
  }, delay)
}

async function poll(): Promise<void> {
  if (!canReceiveOutput() || streamSubscription || pollController) return
  const generation = outputGeneration
  const controller = new AbortController()
  pollController = controller
  try {
    const chunk = await api.terminals.output(props.sessionId, offset, controller.signal)
    if (generation !== outputGeneration) return
    applyChunk(chunk)
    if (generation === outputGeneration && !terminalFinished()) schedulePoll(0)
  } catch (reason) {
    if (generation !== outputGeneration || (reason instanceof DOMException && reason.name === 'AbortError')) return
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      state.value = 'finished'
      return
    }
    if (reason instanceof ApiError && reason.status === 429) {
      // Rate limiting is back-pressure, not a lost connection: keep the
      // terminal marked connected and retry shortly.
      schedulePoll(300)
      return
    }
    state.value = 'reconnecting'
    reconnectAttempts++
    schedulePoll(Math.min(5000, 500 * 2 ** Math.min(reconnectAttempts - 1, 3)))
  } finally {
    if (pollController === controller) pollController = undefined
  }
}

function scheduleResize(): void {
  queueResize(100)
}

function queueResize(delay: number): void {
  if (resizeTimer) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    resizeTimer = undefined
    void syncResize()
  }, delay)
}

// The PTY size is only recorded as synced after the Panel accepts it, so a
// transient failure is retried (bounded) instead of leaving the browser and
// PTY permanently out of step. A reconnect clears the synced size and resends.
async function syncResize(): Promise<void> {
  if (disposed || state.value === 'finished') return
  fitAddon?.fit()
  const rows = terminal?.rows || 0
  const columns = terminal?.cols || 0
  if (!rows || !columns || rows > 500 || columns > 1000 || (rows === syncedRows && columns === syncedColumns)) return
  const sequence = ++resizeSequence
  try {
    await api.terminals.resize(props.sessionId, rows, columns)
    if (sequence !== resizeSequence) return
    syncedRows = rows
    syncedColumns = columns
    resizeFailures = 0
  } catch {
    if (disposed || sequence !== resizeSequence) return
    resizeFailures++
    if (resizeFailures === 1) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.resizeFailed')}\x1b[0m\r\n`)
    if (resizeFailures <= resizeRetryLimit) queueResize(500 * 2 ** (resizeFailures - 1))
  }
}

defineExpose({ focusTerminal, executeCommand, scheduleResize, closeSession })

watch(streaming, (active) => {
  if (active) startOutput()
  else stopOutput()
})

watch([themeColors, resolvedTheme], () => {
  void nextTick(() => {
    if (terminal && host.value) terminal.options.theme = readTerminalTheme(host.value)
  })
})

onMounted(() => {
  mounted = true
  duplexInput = new TerminalDuplexInput({
      negotiate: () => api.terminals.inputTransport(props.sessionId),
      credentials: () => api.terminals.inputSocket(props.sessionId),
      legacy: (data) => api.terminals.input(props.sessionId, data),
      error: (kind) => {
        if (disposed) return
        const key = kind === 'capacity' ? 'terminal.inputCapacity' : kind === 'fatal' ? 'terminal.inputUncertain' : 'terminal.inputFailed'
        writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t(key)}\x1b[0m\r\n`)
        if (kind === 'fatal') inputBlocked.value = true
        if (kind === 'retry') state.value = 'reconnecting'
      },
      recovered: () => { if (!disposed && state.value !== 'finished') state.value = 'connected' },
  })
  terminal = new Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    convertEol: false,
    fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, monospace',
    fontSize: 13,
    lineHeight: 1.25,
    scrollback: 5000,
    theme: host.value ? readTerminalTheme(host.value) : undefined,
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon((_event, uri) => void openTerminalURL(uri)))
  terminal.parser.registerOscHandler(8, () => true)
  terminal.parser.registerOscHandler(52, () => true)
  terminal.attachCustomKeyEventHandler((event) => clipboardMenu.value?.handleKeyEvent(event) ?? true)
  terminal.onData(queueInput)
  if (host.value) {
    terminal.open(host.value)
    observer = new ResizeObserver(scheduleResize)
    observer.observe(host.value)
    scheduleResize()
    window.requestAnimationFrame(focusTerminal)
  }
  startOutput()
})

onBeforeUnmount(() => {
  disposed = true
  mounted = false
  stopOutput()
  duplexInput?.close()
  if (inputTimer) window.clearTimeout(inputTimer)
  if (resizeTimer) window.clearTimeout(resizeTimer)
  observer?.disconnect()
  terminal?.dispose()
  void closeSession().catch(() => undefined)
})
</script>

<template>
  <section class="host-terminal terminal-theme-scope">
    <div
      ref="host"
      class="host-terminal__screen terminal-screen"
      @click="focusTerminal"
      @wheel="containTerminalWheel"
      @touchstart="terminalTouchScroll.start"
      @touchmove="terminalTouchScroll.move"
      @touchend="terminalTouchScroll.end"
      @touchcancel="terminalTouchScroll.end"
      @pointerdown="clipboardMenu?.handleSelectionPointerDown($event)"
      @contextmenu="clipboardMenu?.open($event)"
      @paste.capture="clipboardMenu?.handlePaste($event)"
    >
    </div>
    <form class="host-terminal__composer" @submit.prevent="submitPendingLine">
      <input
        v-model="pendingLine"
        type="text"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        maxlength="8192"
        :placeholder="t('terminal.inputPlaceholder')"
        :disabled="state === 'finished' || inputBlocked"
        @keydown.enter="handlePendingLineEnter"
      />
      <button type="submit" :disabled="state === 'finished' || inputBlocked">{{ t('terminal.send') }}</button>
    </form>
    <TerminalContextMenu
      ref="clipboardMenu"
      :get-terminal="() => terminal"
      :can-paste="state !== 'finished' && !inputBlocked"
    />
  </section>
</template>

<style scoped>
.host-terminal { display:grid; height:100%; grid-template-rows:minmax(0,1fr) auto; min-height:0; overflow:hidden; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--terminal-shell-radius,12px); background:var(--terminal-shell-background,#0b1214); box-shadow:var(--terminal-shell-shadow); }
.host-terminal__screen { position:relative; min-width:0; min-height:0; overflow:hidden; overscroll-behavior:contain; padding:0; }
.host-terminal__screen :deep(.xterm) { box-sizing:border-box; height:100%; padding:6px 8px 4px; touch-action:none; }
.host-terminal__screen :deep(.xterm-viewport) { overflow-y:scroll !important; overscroll-behavior:contain; background:var(--terminal-shell-background,#0b1214); }
.host-terminal__screen :deep(.xterm-scrollable-element) { overscroll-behavior:contain; }
.host-terminal__composer { position:relative; z-index:2; display:grid; grid-template-columns:minmax(0,1fr) auto; gap:8px; padding:9px 10px; border-top:1px solid var(--terminal-shell-border,#29383a); background:var(--terminal-shell-panel,#111a1d); }
.host-terminal__composer input { min-width:0; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; padding:9px 11px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); font:14px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; }
.host-terminal__composer button { border:0; border-radius:8px; padding:0 16px; color:var(--on-brand,#05251c); background:var(--brand-action,#35cba6); font-weight: 700; }
.host-terminal :deep(.xterm-viewport) { scrollbar-color:var(--terminal-shell-scrollbar,#35474a) var(--terminal-shell-background,#0b1214); }
</style>
