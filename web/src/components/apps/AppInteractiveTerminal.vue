<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { api } from '@/lib/api'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import TerminalToolbar from '@/components/terminal/TerminalToolbar.vue'
import { useTerminalFullscreen } from '@/composables/useTerminalFullscreen'
import { useI18n } from '@/i18n'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import { createTerminalTouchScroll } from '@/lib/terminalTouchScroll'
import { TerminalOutputNormalizer } from '@/lib/terminalOutput'
import { readTerminalTheme } from '@/lib/terminalTheme'
import { useTheme } from '@/stores/theme'
import {
  drainTerminalInputQueue,
  TerminalInputQueue,
  terminalEnterShouldSubmit,
  terminalInputFlushInterval,
  terminalInputShouldFlushImmediately,
  terminalLineSubmission,
} from '@/lib/terminalInput'

const props = defineProps<{
  jobId: string
  inputOpen?: boolean
  kind?: 'app' | 'site' | 'diagnostic' | 'environment'
  compact?: boolean
}>()

const { locale, t } = useI18n()
const { colors: themeColors, resolved: resolvedTheme } = useTheme()

const host = ref<HTMLElement>()
const clipboardMenu = ref<InstanceType<typeof TerminalContextMenu>>()
const connectionState = ref<'connecting' | 'connected' | 'finished' | 'error'>('connecting')
const terminalInputOpen = ref(Boolean(props.inputOpen))
const pendingLine = ref('')
let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let resizeObserver: ResizeObserver | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let inputTimer: number | undefined
let inputQueue = new TerminalInputQueue()
let inputSending = false
let offset = 0
let disposed = false
let polling = false
const outputNormalizer = new TerminalOutputNormalizer()

const { fullscreen, toggleFullscreen } = useTerminalFullscreen(fitTerminal)

const taskKindLabel = computed(() => {
  locale.value
  if (props.kind === 'site') return t('terminal.task.kind.site')
  if (props.kind === 'diagnostic') return t('terminal.task.kind.diagnostic')
  if (props.kind === 'environment') return t('terminal.task.kind.environment')
  return t('terminal.task.kind.app')
})

const taskDescription = computed(() => {
  locale.value
  if (props.kind === 'site') return t('terminal.task.description.site')
  if (props.kind === 'diagnostic') return t('terminal.task.description.diagnostic')
  if (props.kind === 'environment') return t('terminal.task.description.environment')
  return t('terminal.task.description.app')
})

const connectionStatusLabel = computed(() => {
  locale.value
  if (connectionState.value === 'connected') {
    return terminalInputOpen.value ? t('terminal.task.inputReady') : t('terminal.task.running')
  }
  if (connectionState.value === 'finished') return t('terminal.task.finished')
  if (connectionState.value === 'error') return t('terminal.reconnecting')
  return t('terminal.connecting')
})

function decodeBase64(value: string): Uint8Array {
  const decoded = window.atob(value)
  const bytes = new Uint8Array(decoded.length)
  for (let index = 0; index < decoded.length; index += 1) {
    bytes[index] = decoded.charCodeAt(index)
  }
  return bytes
}

function isFollowingOutput(): boolean {
  const buffer = terminal?.buffer.active
  return !buffer || buffer.viewportY >= buffer.baseY
}

function writeNormalizedTerminalOutput(data: Uint8Array): void {
  if (data.length === 0) return
  const follow = isFollowingOutput()
  terminal?.write(data, () => {
    if (follow) terminal?.scrollToBottom()
  })
}

function writeTerminalOutput(data: string | Uint8Array): void {
  writeNormalizedTerminalOutput(outputNormalizer.transform(data))
}

function flushTerminalOutput(): void {
  writeNormalizedTerminalOutput(outputNormalizer.flush())
}

function scrollToTop(): void {
  terminal?.scrollToTop()
  if (terminalInputOpen.value) focusTerminal()
}

function focusTerminal(): void {
  terminal?.focus()
}

function focusTerminalWhenInputOpens(): void {
  void nextTick(() => {
    if (terminalInputOpen.value && !disposed) focusTerminal()
  })
}

function fitTerminal(): void {
  fitAddon?.fit()
}

function containTerminalWheel(event: WheelEvent): void {
  containWheelScroll(event, host.value?.querySelector<HTMLElement>('.xterm-viewport, .xterm-scrollable-element'))
}

const terminalTouchScroll = createTerminalTouchScroll({
  getTerminal: () => terminal,
  getScreen: () => host.value?.querySelector<HTMLElement>('.xterm-screen') ?? host.value,
})

async function flushInput(): Promise<void> {
  if (inputSending || !terminalInputOpen.value || disposed) return
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  inputSending = true
  const queue = inputQueue
  try {
    await drainTerminalInputQueue(
      queue,
      () => queue === inputQueue && terminalInputOpen.value && !disposed,
      async (data) => {
        if (props.kind === 'site') {
          await api.sites.terminalInput(props.jobId, data)
        } else if (props.kind === 'diagnostic') {
          await api.diagnostics.terminalInput(props.jobId, data)
        } else if (props.kind === 'environment') {
          await api.webEnvironment.terminalInput(props.jobId, data)
        } else {
          await api.apps.terminalInput(props.jobId, data)
        }
      },
    )
  } catch {
    if (queue === inputQueue) {
      connectionState.value = 'error'
      writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t('terminal.taskInputFailed')}\x1b[0m\r\n`)
    }
  } finally {
    inputSending = false
    if (queue !== inputQueue && !inputQueue.empty && terminalInputOpen.value && !disposed) {
      void flushInput()
    }
  }
}

function queueInput(data: string): void {
  if (!terminalInputOpen.value || disposed) return
  inputQueue.append(data)
  if (
    terminalInputShouldFlushImmediately(data) ||
    inputQueue.byteLength >= 2048
  ) {
    void flushInput()
    return
  }
  if (!inputTimer) {
    inputTimer = window.setTimeout(() => void flushInput(), terminalInputFlushInterval)
  }
}

function submitPendingLine(): void {
  if (!terminalInputOpen.value || disposed) return
  const data = terminalLineSubmission(pendingLine.value)
  pendingLine.value = ''
  queueInput(data)
}

function handlePendingLineEnter(event: KeyboardEvent): void {
  if (!terminalEnterShouldSubmit(event)) return
  event.preventDefault()
  submitPendingLine()
}

async function poll(): Promise<void> {
  if (polling || disposed) return
  polling = true
  pollController?.abort()
  pollController = new AbortController()
  try {
    const chunk = props.kind === 'site'
      ? await api.sites.terminal(
          props.jobId,
          offset,
          terminalInputOpen.value,
          pollController.signal,
        )
      : props.kind === 'diagnostic'
        ? await api.diagnostics.terminal(
            props.jobId,
            offset,
            terminalInputOpen.value,
            pollController.signal,
          )
        : props.kind === 'environment'
          ? await api.webEnvironment.terminal(
              props.jobId,
              offset,
              terminalInputOpen.value,
              pollController.signal,
            )
        : await api.apps.terminal(
            props.jobId,
            offset,
            terminalInputOpen.value,
            pollController.signal,
          )
    const data = chunk.dataBase64 ? decodeBase64(chunk.dataBase64) : undefined
    if (data) writeTerminalOutput(data)
    if (chunk.finished) flushTerminalOutput()
    offset = chunk.nextOffset
    terminalInputOpen.value = chunk.inputOpen
    connectionState.value = chunk.finished ? 'finished' : 'connected'
    if (terminalInputOpen.value && !inputQueue.empty) void flushInput()
    if (!chunk.finished && !disposed) {
      pollTimer = window.setTimeout(() => void poll(), 0)
    }
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    connectionState.value = 'error'
    if (!disposed) pollTimer = window.setTimeout(() => void poll(), 500)
  } finally {
    polling = false
  }
}

function resetTerminal(): void {
  pollController?.abort()
  polling = false
  offset = 0
  inputQueue = new TerminalInputQueue()
  outputNormalizer.reset()
  terminal?.reset()
  pendingLine.value = ''
  terminalInputOpen.value = Boolean(props.inputOpen)
  connectionState.value = 'connecting'
  if (terminalInputOpen.value) focusTerminalWhenInputOpens()
  if (pollTimer) window.clearTimeout(pollTimer)
  pollTimer = window.setTimeout(() => void poll(), 0)
}

watch(() => props.jobId, resetTerminal)
watch(
  () => props.inputOpen,
  (open) => {
    terminalInputOpen.value = Boolean(open)
  },
)
watch(terminalInputOpen, (open) => {
  if (open) focusTerminalWhenInputOpens()
})
watch([themeColors, resolvedTheme], () => {
  void nextTick(() => {
    if (terminal && host.value) terminal.options.theme = readTerminalTheme(host.value)
  })
})

onMounted(() => {
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
  // A remote script may print OSC 52; never allow it to write the browser clipboard.
  terminal.parser.registerOscHandler(52, () => true)
  terminal.attachCustomKeyEventHandler((event) => clipboardMenu.value?.handleKeyEvent(event) ?? true)
  terminal.onData(queueInput)
  if (host.value) {
    terminal.open(host.value)
    fitAddon.fit()
    resizeObserver = new ResizeObserver(() => fitAddon?.fit())
    resizeObserver.observe(host.value)
    if (terminalInputOpen.value) window.requestAnimationFrame(focusTerminal)
  }
  void poll()
})

onBeforeUnmount(() => {
  disposed = true
  pollController?.abort()
  if (pollTimer) window.clearTimeout(pollTimer)
  if (inputTimer) window.clearTimeout(inputTimer)
  resizeObserver?.disconnect()
  terminal?.dispose()
})
</script>

<template>
  <section
    class="interactive-terminal terminal-theme-scope"
    :class="{
      'is-compact': props.compact,
      'is-fullscreen': fullscreen,
    }"
  >
    <header>
      <div>
        <strong>
          kejilion.sh
          {{ t('terminal.task.title', { kind: taskKindLabel }) }}
        </strong>
        <small>{{ taskDescription }}</small>
      </div>
      <div class="interactive-terminal__actions">
        <span :class="`is-${connectionState}`">
          {{ connectionStatusLabel }}
        </span>
        <TerminalToolbar
          :fullscreen="fullscreen"
          @scroll-top="scrollToTop"
          @toggle-fullscreen="toggleFullscreen"
        />
      </div>
    </header>
    <div
      ref="host"
      class="interactive-terminal__screen"
      @click="terminalInputOpen && focusTerminal()"
      @wheel="containTerminalWheel"
      @touchstart="terminalTouchScroll.start"
      @touchmove="terminalTouchScroll.move"
      @touchend="terminalTouchScroll.end"
      @touchcancel="terminalTouchScroll.end"
      @contextmenu="clipboardMenu?.open($event)"
      @paste.capture="clipboardMenu?.handlePaste($event)"
    />
    <div v-if="terminalInputOpen" class="interactive-terminal__input-area">
      <form
        class="interactive-terminal__composer"
        @submit.prevent="submitPendingLine"
      >
        <input
          v-model="pendingLine"
          type="text"
          :aria-label="t('terminal.task.inputLabel')"
          autocomplete="off"
          autocapitalize="off"
          spellcheck="false"
          maxlength="8192"
          :placeholder="t('terminal.task.inputPlaceholder')"
          @keydown.enter="handlePendingLineEnter"
        />
        <button type="submit">{{ t('terminal.send') }}</button>
      </form>
    </div>
    <TerminalContextMenu
      ref="clipboardMenu"
      :get-terminal="() => terminal"
      :can-paste="terminalInputOpen && connectionState !== 'finished'"
      :contained="fullscreen"
    />
  </section>
</template>

<style scoped>
.interactive-terminal {
  --terminal-background: var(--terminal-shell-background, #0b1214);
  --terminal-panel: var(--terminal-shell-panel, #111a1d);
  --terminal-panel-raised: var(--terminal-shell-panel-raised, #182326);
  --terminal-text: var(--terminal-shell-text, #d8dddc);
  --terminal-muted: var(--terminal-shell-muted, #8a9695);
  --terminal-accent: var(--brand, #35cba6);
  --terminal-selection: var(--brand-soft, #153a31);
  --terminal-border: var(--terminal-shell-border, #29383a);
  --scrollbar-track: var(--terminal-shell-background, #0b1214);
  --scrollbar-thumb: var(--terminal-shell-scrollbar, #35474a);
  --scrollbar-thumb-hover: var(--terminal-shell-scrollbar-hover, #506367);
  --scrollbar-thumb-active: var(--terminal-accent);
  display: grid;
  grid-template-rows: auto minmax(0, 1fr) auto;
  min-height: 0;
  overflow: hidden;
  border: 1px solid var(--terminal-border);
  border-radius: var(--terminal-shell-radius, 12px);
  background: var(--terminal-background);
  box-shadow: var(--terminal-shell-shadow, inset 0 1px 0 rgb(255 255 255 / 3%));
}

.interactive-terminal.is-fullscreen {
  position: fixed;
  z-index: 6000;
  inset: 0;
  width: 100vw;
  height: 100dvh;
  min-height: 0;
  border: 0;
  border-radius: 0;
}

.interactive-terminal header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 11px 14px;
  border-bottom: 1px solid var(--terminal-border);
  background: var(--terminal-panel);
}

.interactive-terminal header > div:first-child {
  display: grid;
  gap: 2px;
}

.interactive-terminal header strong {
  color: #f2faf7;
  font-size: 13px;
}

.interactive-terminal header small {
  color: var(--terminal-muted);
  font-size: 11px;
}

.interactive-terminal__actions {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

.interactive-terminal__actions > span {
  flex: 0 0 auto;
  border-radius: 999px;
  padding: 4px 9px;
  color: #b5c8c2;
  background: var(--terminal-panel-raised);
  font-size: 11px;
}

.interactive-terminal__actions > span.is-connected {
  color: #72e4ae;
  background: color-mix(in srgb, var(--terminal-accent) 18%, var(--terminal-panel));
}

.interactive-terminal__actions > span.is-error {
  color: #ffaaa8;
  background: color-mix(in srgb, var(--danger, #ef7a7a) 18%, var(--terminal-panel));
}

.interactive-terminal__screen {
  position: relative;
  height: min(54vh, 520px);
  min-height: 320px;
  overflow: hidden;
  overscroll-behavior: contain;
  padding: 0;
}

.interactive-terminal__screen :deep(.xterm) {
  box-sizing: border-box;
  height: 100%;
  padding: 6px 8px 4px;
  touch-action: none;
}

.interactive-terminal__screen :deep(.xterm-viewport) {
  overflow-y: scroll !important;
  overscroll-behavior: contain;
  background: var(--terminal-background);
}

.interactive-terminal__screen :deep(.xterm-scrollable-element) {
  overscroll-behavior: contain;
}

.interactive-terminal.is-compact .interactive-terminal__screen {
  height: min(30vh, 260px);
  min-height: 200px;
}

.interactive-terminal.is-fullscreen .interactive-terminal__screen,
.interactive-terminal.is-fullscreen.is-compact .interactive-terminal__screen {
  height: auto;
  min-height: 0;
}

.interactive-terminal__composer {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.interactive-terminal__input-area {
  display: grid;
  gap: 8px;
  min-width: 0;
  padding: 10px;
  border-top: 1px solid var(--terminal-border);
  background: var(--terminal-panel);
}

.interactive-terminal__composer input {
  min-width: 0;
  flex: 1;
  border: 1px solid #315148;
  border-radius: 9px;
  padding: 9px 11px;
  color: var(--terminal-text);
  background: var(--terminal-background);
  font: 13px/1.2 "Cascadia Code", "SFMono-Regular", Consolas, monospace;
}

.interactive-terminal__composer input:focus {
  border-color: var(--terminal-accent);
  outline: 2px solid color-mix(in srgb, var(--terminal-accent) 24%, transparent);
}

.interactive-terminal__composer button {
  border: 1px solid var(--terminal-accent);
  border-radius: 9px;
  padding: 8px 16px;
  color: var(--on-brand, #05251c);
  background: var(--terminal-accent);
  font-weight: 600;
  cursor: pointer;
}

.interactive-terminal__composer button:hover {
  border-color: var(--brand-strong, #5adaba);
  background: var(--brand-strong, #5adaba);
}
</style>
