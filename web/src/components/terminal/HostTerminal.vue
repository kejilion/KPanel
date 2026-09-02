<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import { api, ApiError } from '@/lib/api'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import {
  drainTerminalInputQueue,
  TerminalInputQueue,
  terminalEnterShouldSubmit,
  terminalInputFlushInterval,
  terminalInputShouldFlushImmediately,
  terminalLineSubmission,
} from '@/lib/terminalInput'
import { createTerminalTouchScroll } from '@/lib/terminalTouchScroll'
import { readTerminalTheme } from '@/lib/terminalTheme'
import { useI18n } from '@/i18n'
import { desktopWindowActiveKey } from '@/lib/desktopRouteKeys'
import { useTheme } from '@/stores/theme'

const { t } = useI18n()
const { colors: themeColors, resolved: resolvedTheme } = useTheme()
const desktopWindowActive = inject(desktopWindowActiveKey, computed(() => true))

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
const state = ref<'connecting' | 'connected' | 'reconnecting' | 'finished'>('connecting')
let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let observer: ResizeObserver | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let inputTimer: number | undefined
let resizeTimer: number | undefined
const inputQueue = new TerminalInputQueue()
let inputSending = false
// A freshly opened terminal starts at offset 0. Keep the client resilient to
// older Panel responses that did not include the initial offset field.
let offset = Number.isFinite(props.initialOffset) && props.initialOffset >= 0 ? props.initialOffset : 0
let disposed = false
let mounted = false
let lastRows = 0
let lastColumns = 0
let reconnectAttempts = 0

watch(state, (value) => emit('stateChange', value), { immediate: true })

function decodeBase64(value: string): Uint8Array {
  const decoded = window.atob(value)
  return Uint8Array.from(decoded, (character) => character.charCodeAt(0))
}

function encodeBase64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return window.btoa(binary).replace(/=+$/, '')
}

function isFollowingOutput(): boolean {
  const buffer = terminal?.buffer.active
  return !buffer || buffer.viewportY >= buffer.baseY
}

function writeTerminalOutput(data: string | Uint8Array): void {
  const follow = isFollowingOutput()
  terminal?.write(data, () => {
    if (follow) terminal?.scrollToBottom()
  })
}

function scrollToTop(): void {
  terminal?.scrollToTop()
  focusTerminal()
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

async function flushInput(): Promise<void> {
  if (inputSending || disposed || state.value === 'finished') return
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  inputSending = true
  try {
    await drainTerminalInputQueue(
      inputQueue,
      () => !disposed,
      (chunk) => api.terminals.input(props.sessionId, encodeBase64(chunk)).then(() => undefined),
    )
  } catch {
    writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t('terminal.inputFailed')}\x1b[0m\r\n`)
    state.value = 'reconnecting'
  } finally {
    inputSending = false
  }
}

function queueInput(data: string): void {
  if (disposed || state.value === 'finished') return
  inputQueue.append(data)
  if (terminalInputShouldFlushImmediately(data) || inputQueue.byteLength >= 2048) {
    void flushInput()
  } else if (!inputTimer) {
    inputTimer = window.setTimeout(() => void flushInput(), terminalInputFlushInterval)
  }
}

function submitPendingLine(): void {
  if (!pendingLine.value || state.value === 'finished') return
  const value = terminalLineSubmission(pendingLine.value)
  pendingLine.value = ''
  queueInput(value)
}

function handlePendingLineEnter(event: KeyboardEvent): void {
  if (!terminalEnterShouldSubmit(event)) return
  event.preventDefault()
  submitPendingLine()
}

async function poll(): Promise<void> {
  pollTimer = undefined
  if (disposed || !desktopWindowActive.value || state.value === 'finished') return
  pollController?.abort()
  pollController = new AbortController()
  try {
    const chunk = await api.terminals.output(props.sessionId, offset, pollController.signal)
    if (chunk.truncated) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.outputTruncated')}\x1b[0m\r\n`)
    if (chunk.data) writeTerminalOutput(decodeBase64(chunk.data))
    offset = chunk.nextOffset
    state.value = chunk.closed || chunk.exitedAt ? 'finished' : 'connected'
    reconnectAttempts = 0
    if (state.value === 'connected' && !inputQueue.empty) void flushInput()
    if (chunk.exitError) writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${chunk.exitError}\x1b[0m\r\n`)
    if (desktopWindowActive.value && state.value !== 'finished') pollTimer = window.setTimeout(() => void poll(), 0)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      state.value = 'finished'
      return
    }
    state.value = 'reconnecting'
    reconnectAttempts++
    const retryDelay = Math.min(5000, 500 * 2 ** Math.min(reconnectAttempts - 1, 3))
    if (desktopWindowActive.value) pollTimer = window.setTimeout(() => void poll(), retryDelay)
  }
}

function scheduleResize(): void {
  if (resizeTimer) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    fitAddon?.fit()
    const rows = terminal?.rows || 0
    const columns = terminal?.cols || 0
    if (!rows || !columns || rows > 500 || columns > 1000 || (rows === lastRows && columns === lastColumns)) return
    lastRows = rows
    lastColumns = columns
    void api.terminals.resize(props.sessionId, rows, columns).catch(() => undefined)
  }, 100)
}

defineExpose({ focusTerminal, scrollToTop, scheduleResize })

watch(desktopWindowActive, (active) => {
  if (active) {
    if (mounted && state.value !== 'finished') void poll()
    return
  }
  pollController?.abort()
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = undefined
})

watch([themeColors, resolvedTheme], () => {
  void nextTick(() => {
    if (terminal && host.value) terminal.options.theme = readTerminalTheme(host.value)
  })
})

onMounted(() => {
  mounted = true
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
  if (desktopWindowActive.value) void poll()
})

onBeforeUnmount(() => {
  disposed = true
  mounted = false
  pollController?.abort()
  if (pollTimer) window.clearTimeout(pollTimer)
  if (inputTimer) window.clearTimeout(inputTimer)
  if (resizeTimer) window.clearTimeout(resizeTimer)
  observer?.disconnect()
  terminal?.dispose()
  void api.terminals.close(props.sessionId).catch(() => undefined)
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
        :disabled="state === 'finished'"
        @keydown.enter="handlePendingLineEnter"
      />
      <button type="submit" :disabled="state === 'finished'">{{ t('terminal.send') }}</button>
    </form>
    <TerminalContextMenu
      ref="clipboardMenu"
      :get-terminal="() => terminal"
      :can-paste="state !== 'finished'"
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
.host-terminal__composer input { min-width:0; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; padding:9px 11px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); font:12px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; }
.host-terminal__composer button { border:0; border-radius:8px; padding:0 16px; color:var(--on-brand,#05251c); background:var(--brand-action,#35cba6); font-weight: 700; }
.host-terminal :deep(.xterm-viewport) { scrollbar-color:var(--terminal-shell-scrollbar,#35474a) var(--terminal-shell-background,#0b1214); }
</style>
