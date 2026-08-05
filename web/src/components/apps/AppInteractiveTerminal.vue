<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'
import { ArrowDownToLine } from '@lucide/vue'
import { api } from '@/lib/api'
import { useI18n } from '@/i18n'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import {
  takeTerminalInputChunk,
  terminalInputShouldFlushImmediately,
  terminalLineSubmission,
} from '@/lib/terminalInput'

const props = defineProps<{
  jobId: string
  inputOpen?: boolean
  kind?: 'app' | 'site' | 'diagnostic' | 'environment'
  compact?: boolean
}>()

const { t } = useI18n()

const host = ref<HTMLElement>()
const composerInput = ref<HTMLInputElement>()
const connectionState = ref<'connecting' | 'connected' | 'finished' | 'error'>('connecting')
const terminalInputOpen = ref(Boolean(props.inputOpen))
const pendingLine = ref('')
let terminal: Terminal | undefined
let fitAddon: FitAddon | undefined
let resizeObserver: ResizeObserver | undefined
let pollController: AbortController | undefined
let pollTimer: number | undefined
let inputTimer: number | undefined
let inputQueue = ''
let inputSending = false
let offset = 0
let disposed = false
let polling = false

const inputFlushInterval = 24

function terminalThemeColor(name: string, fallback: string): string {
  if (!host.value) return fallback
  return window.getComputedStyle(host.value).getPropertyValue(name).trim() || fallback
}

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

function writeTerminalOutput(data: string | Uint8Array): void {
  const follow = isFollowingOutput()
  terminal?.write(data, () => {
    if (follow) terminal?.scrollToBottom()
  })
}

function scrollToBottom(): void {
  terminal?.scrollToBottom()
  terminal?.focus()
}

function containTerminalWheel(event: WheelEvent): void {
  containWheelScroll(event, host.value?.querySelector<HTMLElement>('.xterm-viewport'))
}

async function flushInput(): Promise<void> {
  if (inputSending || !terminalInputOpen.value || disposed) return
  if (inputTimer) window.clearTimeout(inputTimer)
  inputTimer = undefined
  inputSending = true
  try {
    while (inputQueue && terminalInputOpen.value && !disposed) {
      const { chunk: data, rest } = takeTerminalInputChunk(inputQueue)
      if (props.kind === 'site') {
        await api.sites.terminalInput(props.jobId, data)
      } else if (props.kind === 'diagnostic') {
        await api.diagnostics.terminalInput(props.jobId, data)
      } else if (props.kind === 'environment') {
        await api.webEnvironment.terminalInput(props.jobId, data)
      } else {
        await api.apps.terminalInput(props.jobId, data)
      }
      inputQueue = rest
    }
  } catch {
    writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t('terminal.taskInputFailed')}\x1b[0m\r\n`)
  } finally {
    inputSending = false
  }
}

function queueInput(data: string): void {
  if (!terminalInputOpen.value || disposed) return
  inputQueue += data
  if (
    terminalInputShouldFlushImmediately(data) ||
    new TextEncoder().encode(inputQueue).byteLength >= 2048
  ) {
    void flushInput()
    return
  }
  if (!inputTimer) {
    inputTimer = window.setTimeout(() => void flushInput(), inputFlushInterval)
  }
}

function submitPendingLine(): void {
  if (!terminalInputOpen.value || disposed) return
  const data = terminalLineSubmission(pendingLine.value)
  pendingLine.value = ''
  queueInput(data)
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
    offset = chunk.nextOffset
    terminalInputOpen.value = chunk.inputOpen
    connectionState.value = chunk.finished ? 'finished' : 'connected'
    if (terminalInputOpen.value && inputQueue) void flushInput()
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
  terminal?.reset()
  pendingLine.value = ''
  terminalInputOpen.value = Boolean(props.inputOpen)
  connectionState.value = 'connecting'
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
  if (open) void nextTick(() => composerInput.value?.focus())
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
    theme: {
      background: terminalThemeColor('--terminal-background', '#0b1214'),
      foreground: terminalThemeColor('--terminal-text', '#d8dddc'),
      cursor: terminalThemeColor('--terminal-accent', '#35cba6'),
      cursorAccent: terminalThemeColor('--terminal-background', '#0b1214'),
      selectionBackground: terminalThemeColor('--terminal-selection', 'rgb(53 203 166 / 20%)'),
      black: terminalThemeColor('--terminal-ansi-black', '#1d2426'),
      red: terminalThemeColor('--terminal-ansi-red', '#d86f74'),
      green: terminalThemeColor('--terminal-ansi-green', '#91b56d'),
      yellow: terminalThemeColor('--terminal-ansi-yellow', '#d5ae62'),
      blue: terminalThemeColor('--terminal-ansi-blue', '#76a4c7'),
      magenta: terminalThemeColor('--terminal-ansi-magenta', '#ad8bb8'),
      cyan: terminalThemeColor('--terminal-ansi-cyan', '#72aaa7'),
      white: terminalThemeColor('--terminal-ansi-white', '#c9cecd'),
      brightBlack: terminalThemeColor('--terminal-ansi-bright-black', '#687376'),
      brightRed: terminalThemeColor('--terminal-ansi-bright-red', '#e68589'),
      brightGreen: terminalThemeColor('--terminal-ansi-bright-green', '#a7c982'),
      brightYellow: terminalThemeColor('--terminal-ansi-bright-yellow', '#e3c27b'),
      brightBlue: terminalThemeColor('--terminal-ansi-bright-blue', '#8bb9dc'),
      brightMagenta: terminalThemeColor('--terminal-ansi-bright-magenta', '#c19bcb'),
      brightCyan: terminalThemeColor('--terminal-ansi-bright-cyan', '#8cc2be'),
      brightWhite: terminalThemeColor('--terminal-ansi-bright-white', '#f0f2f1'),
    },
  })
  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon((_event, uri) => void openTerminalURL(uri)))
  // A remote script may print OSC 52; never allow it to write the browser clipboard.
  terminal.parser.registerOscHandler(52, () => true)
  terminal.onData(queueInput)
  if (host.value) {
    terminal.open(host.value)
    fitAddon.fit()
    resizeObserver = new ResizeObserver(() => fitAddon?.fit())
    resizeObserver.observe(host.value)
    if (terminalInputOpen.value) {
      window.requestAnimationFrame(() => composerInput.value?.focus())
    } else {
      terminal.focus()
    }
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
  <section class="interactive-terminal" :class="{ 'is-compact': props.compact }">
    <header>
      <div>
        <strong>
          kejilion.sh
          {{
            props.kind === 'site'
              ? '建站'
              : props.kind === 'diagnostic'
                ? '体检'
                : props.kind === 'environment'
                  ? '环境管理'
                  : '应用'
          }}终端
        </strong>
        <small>
          {{
            props.kind === 'site'
              ? '域名和固定参数已由面板传入；需要时按脚本提示继续输入。'
              : props.kind === 'diagnostic'
                ? '保留第三方脚本原生颜色；需要安装依赖或选择测试项时可直接输入。'
                : props.kind === 'environment'
                  ? '保留脚本原生颜色；关闭窗口不会中断后台环境任务。'
              : '直接按脚本提示输入；窗口关闭后任务仍在后台继续。'
          }}
        </small>
      </div>
      <div class="interactive-terminal__actions">
        <span :class="`is-${connectionState}`">
          {{
            connectionState === 'connected'
              ? terminalInputOpen
                ? '可输入'
                : '运行中'
              : connectionState === 'finished'
                ? '已结束'
                : connectionState === 'error'
                  ? '正在重连'
                  : '正在连接'
          }}
        </span>
        <button type="button" :title="t('terminal.scrollToBottom')" :aria-label="t('terminal.scrollToBottom')" @click="scrollToBottom"><ArrowDownToLine :size="17" /></button>
      </div>
    </header>
    <div ref="host" class="interactive-terminal__screen" @click="terminal?.focus()" @wheel="containTerminalWheel" />
    <form
      v-if="terminalInputOpen"
      class="interactive-terminal__composer"
      @submit.prevent="submitPendingLine"
    >
      <input
        ref="composerInput"
        v-model="pendingLine"
        type="text"
        aria-label="预输入终端内容"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        maxlength="8192"
        placeholder="在此预输入，按 Enter 整行发送"
        @keydown.enter.prevent="submitPendingLine"
      />
      <button type="submit">发送</button>
    </form>
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
  --terminal-selection: rgb(53 203 166 / 20%);
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
  padding: 10px;
}

.interactive-terminal__screen :deep(.xterm) {
  height: 100%;
}

.interactive-terminal__screen :deep(.xterm-viewport) {
  overflow-y: scroll !important;
  overscroll-behavior: contain;
}

.interactive-terminal__actions button {
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  border: 1px solid var(--terminal-border);
  border-radius: 8px;
  color: var(--terminal-muted);
  background: transparent;
}

.interactive-terminal__actions button:hover {
  color: var(--terminal-text);
  border-color: var(--terminal-accent);
}

.interactive-terminal.is-compact .interactive-terminal__screen {
  height: min(30vh, 260px);
  min-height: 200px;
}

.interactive-terminal__composer {
  display: flex;
  gap: 8px;
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
  color: #fff;
  background: var(--terminal-accent);
  font-weight: 700;
  cursor: pointer;
}

.interactive-terminal__composer button:hover {
  border-color: var(--brand-strong, #5adaba);
  background: var(--brand-strong, #5adaba);
}
</style>
