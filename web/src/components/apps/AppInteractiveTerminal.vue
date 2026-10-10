<script setup lang="ts">
import { isTouchPrimary } from '../../lib/touchFocus'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { api, terminalStream } from '@/lib/api'
import type { TerminalStreamSubscription } from '@/lib/terminalStream'
import type { AppTerminalChunk, JobTerminalKind } from '@/types/api'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import TerminalToolbar from '@/components/terminal/TerminalToolbar.vue'
import { useTerminalActivity } from '@/composables/useTerminalActivity'
import { useTerminalFullscreen } from '@/composables/useTerminalFullscreen'
import { useI18n } from '@/i18n'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import { joinTerminalSizeGroup } from '@/lib/terminalSizeOwnership'
import type { TerminalSizeMembership } from '@/lib/terminalSizeOwnership'
import { createTerminalTouchScroll } from '@/lib/terminalTouchScroll'
import { TerminalOutputNormalizer } from '@/lib/terminalOutput'
import { TerminalDuplexInput } from '@/lib/terminalDuplexInput'
import { terminalCursorOptions } from '@/lib/terminalCursor'
import { readTerminalTheme } from '@/lib/terminalTheme'
import { TerminalWriteFlow } from '@/lib/terminalWriteFlow'
import { useTheme } from '@/stores/theme'
import {
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
  /**
   * Inside a workspace dialog the window already names the task and owns
   * full screen, so the terminal drops its own header and fills the body.
   */
  headless?: boolean
}>()

const { locale, t } = useI18n()
const { colors: themeColors, resolved: resolvedTheme } = useTheme()
const activity = useTerminalActivity()

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
let streamSubscription: TerminalStreamSubscription | null = null
let inputTimer: number | undefined
// One acknowledged stream per open task input; see ensureInput.
let duplexInput: TerminalDuplexInput | undefined
let inputRetrying = false
let offset = 0
let disposed = false
// Bumped whenever output stops so a late poll result cannot be applied twice.
let outputGeneration = 0
let resizeTimer: number | undefined
let resizeSending = false
let resizeGeneration = 0
let resizeFailures = 0
let syncedRows = 0
let syncedColumns = 0
const outputNormalizer = new TerminalOutputNormalizer()
const writeFlow = new TerminalWriteFlow(() => startOutput())
// Only task PTYs have a size; other job kinds always fit their own view.
const sizeMembership = shallowRef<TerminalSizeMembership>()
const ownsSize = computed(() => sizeMembership.value?.isOwner() ?? true)
const ownerSize = computed(() => sizeMembership.value?.ownerSize())

const { fullscreen, toggleFullscreen } = useTerminalFullscreen(fitTerminal)

function joinSizeGroup(): void {
  sizeMembership.value?.leave()
  sizeMembership.value = props.kind && props.kind !== 'app'
    ? undefined
    : joinTerminalSizeGroup(`app:${props.jobId}`)
  if (activity.focused.value) sizeMembership.value?.claim()
}

joinSizeGroup()

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
  if (data.length === 0 || !terminal) return
  const follow = isFollowingOutput()
  const parsed = writeFlow.track(data.length)
  terminal.write(data, () => {
    parsed()
    if (follow) terminal?.scrollToBottom()
  })
  // The offset is kept, so output resumes exactly where it paused.
  if (writeFlow.blocked) stopOutput()
}

function writeTerminalOutput(data: string | Uint8Array): void {
  writeNormalizedTerminalOutput(outputNormalizer.transform(data))
}

function flushTerminalOutput(): void {
  writeNormalizedTerminalOutput(outputNormalizer.flush())
}

function focusTerminal(): void {
  if (isTouchPrimary()) return
  terminal?.focus()
}

// Terminals in unfocused windows stay mounted; never take focus from the
// window the user is working in.
function focusTerminalWhenInputOpens(): void {
  void nextTick(() => {
    if (terminalInputOpen.value && !disposed && activity.focused.value) focusTerminal()
  })
}

function fitTerminal(): void {
  if (!ownsSize.value) {
    followOwnerSize()
    return
  }
  fitAddon?.fit()
  resizeFailures = 0
  scheduleResize()
}

// A view that does not drive the PTY renders at the owner's size so the
// program's layout stays intact; it fits locally until a size is known.
function followOwnerSize(): void {
  if (disposed) return
  const size = ownerSize.value
  if (!size) {
    fitAddon?.fit()
    return
  }
  if (terminal && (terminal.rows !== size.rows || terminal.cols !== size.columns)) {
    terminal.resize(size.columns, size.rows)
  }
}

function scheduleResize(delay = 100): void {
  if (disposed || !ownsSize.value || (props.kind && props.kind !== 'app')) return
  if (resizeTimer) window.clearTimeout(resizeTimer)
  resizeTimer = window.setTimeout(() => {
    resizeTimer = undefined
    void syncResize()
  }, delay)
}

async function syncResize(): Promise<void> {
  if (props.kind && props.kind !== 'app') return
  if (disposed || !ownsSize.value || resizeSending || !terminalInputOpen.value || connectionState.value === 'finished') return
  if (!host.value?.clientWidth || !host.value.clientHeight) return
  fitAddon?.fit()
  const rows = terminal?.rows ?? 0
  const columns = terminal?.cols ?? 0
  if (!rows || !columns || rows > 500 || columns > 1000 || (rows === syncedRows && columns === syncedColumns)) return
  const generation = resizeGeneration
  resizeSending = true
  try {
    const result = await api.apps.terminalResize(props.jobId, rows, columns)
    if (!result.accepted) throw new Error('terminal resize was not applied')
    if (disposed || generation !== resizeGeneration) return
    syncedRows = rows
    syncedColumns = columns
    resizeFailures = 0
    sizeMembership.value?.publish({ rows, columns })
    if (terminal?.rows !== rows || terminal?.cols !== columns) scheduleResize()
  } catch {
    if (disposed || generation !== resizeGeneration) return
    resizeFailures++
    if (resizeFailures === 1) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.resizeFailed')}\x1b[0m\r\n`)
    if (resizeFailures <= 3) scheduleResize(500 * 2 ** (resizeFailures - 1))
  } finally {
    resizeSending = false
    if (!disposed && generation !== resizeGeneration) scheduleResize()
  }
}

function resetResize(): void {
  resizeGeneration++
  syncedRows = syncedColumns = resizeFailures = 0
  scheduleResize()
}

function containTerminalWheel(event: WheelEvent): void {
  containWheelScroll(event, host.value?.querySelector<HTMLElement>('.xterm-viewport, .xterm-scrollable-element'))
}

const terminalTouchScroll = createTerminalTouchScroll({
  getTerminal: () => terminal,
  getScreen: () => host.value?.querySelector<HTMLElement>('.xterm-screen') ?? host.value,
})

const inputFailureMessages = {
  capacity: 'terminal.inputCapacity',
  fatal: 'terminal.taskInputFailed',
  retry: 'terminal.inputFailed',
} as const

function legacyInput(kind: JobTerminalKind, jobId: string, data: string): Promise<unknown> {
  if (kind === 'site') return api.sites.terminalInput(jobId, data)
  if (kind === 'diagnostic') return api.diagnostics.terminalInput(jobId, data)
  if (kind === 'environment') return api.webEnvironment.terminalInput(jobId, data)
  return api.apps.terminalInput(jobId, data)
}

// Input of the task that is open now goes through one acknowledged stream. It
// belongs to that task and to that open period: it is dropped with them, so a
// late result or ACK can never reach the next task or a later input. After a
// failure the next keystroke starts a new stream, which takes over from the
// failed one.
function ensureInput(): TerminalDuplexInput {
  if (duplexInput) return duplexInput
  const kind: JobTerminalKind = props.kind ?? 'app'
  const jobId = props.jobId
  const input: TerminalDuplexInput = new TerminalDuplexInput({
    negotiate: () => api.jobTerminals.inputTransport(kind, jobId),
    credentials: () => api.jobTerminals.inputSocket(kind, jobId),
    legacy: (data) => legacyInput(kind, jobId, data),
    legacyText: true,
    batch: (frames, signal) => api.jobTerminals.inputBatch(kind, jobId, frames, signal),
    error: (failure) => {
      if (disposed || duplexInput !== input) return
      writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t(inputFailureMessages[failure])}\x1b[0m\r\n`)
      if (failure === 'fatal') dropInput()
      if (failure === 'retry') {
        inputRetrying = true
        connectionState.value = 'error'
      }
    },
    recovered: () => {
      if (disposed || duplexInput !== input || !inputRetrying) return
      inputRetrying = false
      if (connectionState.value === 'error') connectionState.value = 'connected'
    },
  })
  duplexInput = input
  return input
}

// Connect ahead of the first key so it does not pay for negotiation and handshake.
function warmInput(): void {
  if (!disposed && terminalInputOpen.value) ensureInput().flush()
}

function dropInput(): void {
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  duplexInput?.close()
  duplexInput = undefined
  inputRetrying = false
}

function flushInput(): void {
  if (disposed || !terminalInputOpen.value) return
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  duplexInput?.flush()
}

function queueInput(data: string): void {
  if (!terminalInputOpen.value || disposed) return
  // NUL is never valid task input; the per-request route refused it as well.
  const text = data.includes('\0') ? data.replaceAll('\0', '') : data
  if (!text) return
  const input = ensureInput()
  if (!input.append(text)) return
  if (terminalInputShouldFlushImmediately(text) || input.byteLength >= 2048) {
    flushInput()
    return
  }
  if (!inputTimer) {
    inputTimer = window.setTimeout(() => flushInput(), terminalInputFlushInterval)
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

function applyJobChunk(chunk: AppTerminalChunk): void {
  const reconnected = connectionState.value !== 'connected'
  const data = chunk.dataBase64 ? decodeBase64(chunk.dataBase64) : undefined
  if (chunk.truncated) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.outputTruncated')}\x1b[0m\r\n`)
  if (data) writeTerminalOutput(data)
  if (chunk.finished) flushTerminalOutput()
  offset = chunk.nextOffset
  terminalInputOpen.value = chunk.inputOpen
  connectionState.value = chunk.finished ? 'finished' : 'connected'
  if (reconnected && !chunk.finished) resetResize()
  if (terminalInputOpen.value && duplexInput?.byteLength) flushInput()
  if (chunk.finished) stopOutput()
}

// Output flows while the terminal is on screen and xterm keeps up; it pauses
// when the window is minimized, the tab is hidden or xterm is backlogged, and
// resumes from the same offset.
function canReceiveOutput(): boolean {
  return !disposed && activity.streaming.value && !writeFlow.blocked && connectionState.value !== 'finished'
}

// Task output prefers the tab's shared push stream; polling remains the
// fallback and is also used after a stream-level failure for this task.
function startOutput(): void {
  if (!canReceiveOutput() || streamSubscription || pollController) return
  streamSubscription = terminalStream.subscribe(
    { kind: 'job', job: props.kind ?? 'app', id: props.jobId, offset, inputOpen: terminalInputOpen.value },
    {
      connected: resetResize,
      job: applyJobChunk,
      error: () => {
        streamSubscription?.close()
        streamSubscription = null
        connectionState.value = 'error'
        schedulePoll(500)
      },
      unavailable: () => {
        streamSubscription = null
        connectionState.value = 'error'
        void poll()
      },
    },
  )
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

function readJobChunk(signal: AbortSignal): Promise<AppTerminalChunk> {
  const inputOpen = terminalInputOpen.value
  if (props.kind === 'site') return api.sites.terminal(props.jobId, offset, inputOpen, signal)
  if (props.kind === 'diagnostic') return api.diagnostics.terminal(props.jobId, offset, inputOpen, signal)
  if (props.kind === 'environment') return api.webEnvironment.terminal(props.jobId, offset, inputOpen, signal)
  return api.apps.terminal(props.jobId, offset, inputOpen, signal)
}

async function poll(): Promise<void> {
  if (!canReceiveOutput() || streamSubscription || pollController) return
  const generation = outputGeneration
  const controller = new AbortController()
  pollController = controller
  try {
    const chunk = await readJobChunk(controller.signal)
    if (generation !== outputGeneration) return
    applyJobChunk(chunk)
    if (!chunk.finished && generation === outputGeneration) schedulePoll(0)
  } catch (reason) {
    if (generation !== outputGeneration || (reason instanceof DOMException && reason.name === 'AbortError')) return
    connectionState.value = 'error'
    schedulePoll(500)
  } finally {
    if (pollController === controller) pollController = undefined
  }
}

function resetTerminal(): void {
  stopOutput()
  writeFlow.reset()
  joinSizeGroup()
  resetResize()
  offset = 0
  dropInput()
  outputNormalizer.reset()
  terminal?.reset()
  pendingLine.value = ''
  terminalInputOpen.value = Boolean(props.inputOpen)
  connectionState.value = 'connecting'
  if (terminalInputOpen.value) {
    focusTerminalWhenInputOpens()
    warmInput()
  }
  pollTimer = window.setTimeout(() => {
    pollTimer = undefined
    startOutput()
  }, 0)
}

watch(() => props.jobId, resetTerminal)
watch(activity.streaming, (streaming) => {
  if (streaming) startOutput()
  else stopOutput()
})
watch(activity.focused, (focused) => {
  if (!focused) return
  sizeMembership.value?.claim()
  if (terminalInputOpen.value) focusTerminalWhenInputOpens()
})
watch(ownsSize, (owner) => {
  if (disposed) return
  if (!owner) {
    followOwnerSize()
    return
  }
  // The previous owner resized the PTY; send this view's size again.
  fitAddon?.fit()
  resetResize()
})
watch(ownerSize, () => {
  if (!ownsSize.value) followOwnerSize()
})
watch(
  () => props.inputOpen,
  (open) => {
    terminalInputOpen.value = Boolean(open)
  },
)
watch(terminalInputOpen, (open) => {
  if (open) focusTerminalWhenInputOpens()
  if (open) resetResize()
  if (open) warmInput()
  else dropInput()
})
watch([themeColors, resolvedTheme], () => {
  void nextTick(() => {
    if (terminal && host.value) terminal.options.theme = readTerminalTheme(host.value)
  })
})

onMounted(() => {
  terminal = new Terminal({
    ...terminalCursorOptions,
    convertEol: false,
    fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, monospace',
    fontSize: 13,
    lineHeight: 1.25,
    scrollback: 5000,
    allowTransparency: true,
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
    fitTerminal()
    resizeObserver = new ResizeObserver(fitTerminal)
    resizeObserver.observe(host.value)
    if (terminalInputOpen.value && activity.focused.value) window.requestAnimationFrame(focusTerminal)
  }
  startOutput()
  warmInput()
})

onBeforeUnmount(() => {
  disposed = true
  stopOutput()
  dropInput()
  resizeGeneration++
  if (resizeTimer) window.clearTimeout(resizeTimer)
  resizeObserver?.disconnect()
  sizeMembership.value?.leave()
  terminal?.dispose()
})
</script>

<template>
  <section
    class="interactive-terminal terminal-theme-scope"
    :class="{
      'is-compact': props.compact,
      'is-fullscreen': fullscreen,
      'is-headless': props.headless,
    }"
  >
    <header v-if="!props.headless">
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
      @pointerdown="clipboardMenu?.handleSelectionPointerDown($event)"
      @contextmenu="clipboardMenu?.open($event)"
      @paste.capture="clipboardMenu?.handlePaste($event)"
    />
    <span
      v-if="props.headless && connectionState !== 'connected'"
      class="interactive-terminal__connection"
      :class="`is-${connectionState}`"
      role="status"
    >
      {{ connectionStatusLabel }}
    </span>
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

.interactive-terminal.is-headless {
  position: relative;
  flex: 1 1 auto;
  grid-template-rows: minmax(0, 1fr) auto;
  min-height: 0;
  border: 0;
  border-radius: 0;
  box-shadow: none;
}

.interactive-terminal.is-headless .interactive-terminal__screen {
  height: auto;
  min-height: 0;
}

/* Headless terminals only surface the connection while it needs attention. */
.interactive-terminal__connection {
  position: absolute;
  z-index: 2;
  top: 10px;
  right: 20px;
  padding: 4px 10px;
  border-radius: 999px;
  color: #b5c8c2;
  background: var(--terminal-panel-raised);
  font-size: 12px;
  line-height: 1.4;
  pointer-events: none;
}

.interactive-terminal__connection.is-error {
  color: #ffaaa8;
  background: color-mix(in srgb, var(--danger, #ef7a7a) 18%, var(--terminal-panel));
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

.interactive-terminal__screen { --terminal-canvas-background:#00000000; --terminal-wallpaper-veil:color-mix(in srgb,var(--terminal-shell-background,#0b1214) 84%,transparent); background:linear-gradient(var(--terminal-wallpaper-veil),var(--terminal-wallpaper-veil)),var(--classic-wallpaper-image,url('/wallpapers/kpanel-desktop.webp')) var(--desktop-wallpaper-position,center) / cover no-repeat; }
:global(:root[data-wallpaper-bright] .interactive-terminal__screen) { --terminal-wallpaper-veil:color-mix(in srgb,var(--terminal-shell-background,#0b1214) 94%,transparent); }

.interactive-terminal__screen :deep(.xterm) {
  box-sizing: border-box;
  height: 100%;
  padding: 6px 8px 4px;
  touch-action: none;
}

.interactive-terminal__screen :deep(.xterm-viewport) {
  overflow-y: scroll !important;
  overscroll-behavior: contain;
  background: transparent;
}

.interactive-terminal__screen :deep(.xterm-scrollable-element) {
  overscroll-behavior: contain;
}

@media (prefers-reduced-transparency: reduce), (prefers-contrast: more), (forced-colors: active) {
  .interactive-terminal__screen { background:var(--terminal-shell-background,#0b1214); }
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
