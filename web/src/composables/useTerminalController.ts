import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch, type Ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { TerminalWebLinksAddon } from '@/lib/terminalWrappedLinks'
import { TerminalKeywordColorsAddon } from '@/lib/terminalKeywordColors'
import '@xterm/xterm/css/xterm.css'
import type TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import type { TerminalAdapter, TerminalChunk } from '@/lib/terminalAdapters'
import type { TerminalStreamSubscription } from '@/lib/terminalStream'
import { TerminalDuplexInput } from '@/lib/terminalDuplexInput'
import { terminalRequest } from '@/lib/terminalRequest'
import { TerminalWriteFlow } from '@/lib/terminalWriteFlow'
import { TerminalOutputNormalizer } from '@/lib/terminalOutput'
import { TerminalCommandPresentation } from '@/lib/terminalCommandPresentation'
import { terminalEnterShouldSubmit, terminalInputFlushInterval, terminalInputShouldFlushImmediately, terminalLineSubmission } from '@/lib/terminalInput'
import { terminalCursorOptions } from '@/lib/terminalCursor'
import { installTerminalCursorReport } from '@/lib/terminalCursorReport'
import { installTerminalScrollbar } from '@/lib/terminalScrollbar'
import { createTerminalTouchScroll } from '@/lib/terminalTouchScroll'
import { readTerminalTheme } from '@/lib/terminalTheme'
import { openTerminalURL } from '@/lib/terminalLinks'
import { containWheelScroll } from '@/lib/scroll'
import { isTouchPrimary } from '@/lib/touchFocus'
import { useTerminalActivity } from '@/composables/useTerminalActivity'
import { useTheme } from '@/stores/theme'
import { useI18n } from '@/i18n'

export type TerminalConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'finished' | 'error'
interface TerminalSize { rows: number; columns: number }
interface Options {
  adapter: () => TerminalAdapter
  host: Ref<HTMLElement | undefined>
  clipboardMenu: Ref<InstanceType<typeof TerminalContextMenu> | undefined>
  initialOffset?: () => number
  inputOpen?: () => boolean | undefined
  visible?: () => boolean | undefined
  autoFocus?: () => boolean | undefined
  ownsSize?: () => boolean
  ownerSize?: () => TerminalSize | undefined
  sizeSynced?: (size: TerminalSize) => void
  activity?: ReturnType<typeof useTerminalActivity>
  acceptedInput?: (data: string, initialCommand: boolean) => void
}

// A single viewer owns rendering, transport orchestration and bounded queues.
// It never creates, cancels or closes a backend session/job.
export function useTerminalController(options: Options) {
  const { t } = useI18n()
  const { colors: themeColors, resolved: resolvedTheme } = useTheme()
  const activity = options.activity ?? useTerminalActivity()
  const { host, clipboardMenu } = options
  const terminal = shallowRef<Terminal>()
  const state = ref<TerminalConnectionState>('connecting')
  const inputOpen = ref(Boolean(options.inputOpen?.()))
  const inputBlocked = ref(false)
  const presentingCommand = ref(false)
  const pendingLine = ref('')
  const canInput = computed(() => inputOpen.value && !inputBlocked.value && !presentingCommand.value && state.value !== 'finished' && state.value !== 'error')
  let adapter = options.adapter()
  let offset = initialOffset()
  let mounted = false
  let disposed = false
  let targetGeneration = 0
  let outputGeneration = 0
  let streamSubscription: TerminalStreamSubscription | null = null
  let pollController: AbortController | undefined
  let pollTimer: number | undefined
  let reconnectAttempts = 0
  let duplexInput: TerminalDuplexInput | undefined
  let inputTimer: number | undefined
  let inputRetrying = false
  let fitAddon: FitAddon | undefined
  let observer: ResizeObserver | undefined
  let disposeScrollbar: (() => void) | undefined
  let cursorReport: ReturnType<typeof installTerminalCursorReport> | undefined
  let resizeTimer: number | undefined
  let resizeGeneration = 0
  let resizeSending = false
  let resizeDirty = false
  let resizeFailures = 0
  let syncedRows = 0
  let syncedColumns = 0
  let commandPresentation: TerminalCommandPresentation | undefined
  let presentationTimer: number | undefined
  let presentationWake: (() => void) | undefined
  const presentationFrames = new Set<number>()
  const normalizer = new TerminalOutputNormalizer()
  const writeFlow = new TerminalWriteFlow(() => startOutput())

  function initialOffset(): number {
    const value = options.initialOffset?.() ?? 0
    return Number.isFinite(value) && value >= 0 ? value : 0
  }
  function writeRaw(data: string | Uint8Array, done?: () => void): void {
    const screen = terminal.value
    if (!screen || !data.length) { done?.(); return }
    const buffer = screen.buffer.active
    const follow = buffer.baseY - buffer.viewportY <= 2
    const parsed = writeFlow.track(data.length)
    const generation = targetGeneration
    screen.write(data, () => {
      parsed()
      if (!disposed && generation === targetGeneration) {
        if (follow) screen.scrollToBottom()
      }
      done?.()
    })
    if (writeFlow.blocked) stopOutput()
  }
  function writeTerminalOutput(data: string | Uint8Array): void {
    writeRaw(adapter.mode === 'task' ? normalizer.transform(data) : data)
  }
  function focusTerminal(): void {
    if (disposed || isTouchPrimary() || options.visible?.() === false) return
    terminal.value?.focus()
  }
  function focusWhenReady(): void {
    void nextTick(() => { if (inputOpen.value && activity.focused.value) focusTerminal() })
  }
  function containTerminalWheel(event: WheelEvent): void {
    containWheelScroll(event, host.value?.querySelector<HTMLElement>('.xterm-viewport, .xterm-scrollable-element'))
  }
  const terminalTouchScroll = createTerminalTouchScroll({
    getTerminal: () => terminal.value,
    getScreen: () => host.value?.querySelector<HTMLElement>('.xterm-screen') ?? host.value,
    getSelectionTerminal: () => terminal.value,
    onLongPress: (action, x, y) => { void clipboardMenu.value?.handleTouchHold(action, x, y) },
    onTap: () => {
      if (!disposed && canInput.value && options.visible?.() !== false) terminal.value?.focus()
    },
  })

  function ensureInput(): TerminalDuplexInput {
    if (duplexInput) return duplexInput
    const generation = targetGeneration
    const mode = adapter.mode
    const input = new TerminalDuplexInput({
      ...adapter.input,
      error: kind => {
        if (disposed || generation !== targetGeneration || duplexInput !== input) return
        const keys = { capacity: 'terminal.inputCapacity', retry: 'terminal.inputFailed', fatal: mode === 'task' ? 'terminal.taskInputFailed' : 'terminal.inputUncertain' } as const
        const key = keys[kind]
        writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t(key)}\x1b[0m\r\n`)
        if (kind === 'fatal') {
          if (mode === 'task') dropInput()
          else inputBlocked.value = true
        }
        if (kind === 'retry') { inputRetrying = true; state.value = 'reconnecting' }
      },
      recovered: () => {
        if (disposed || generation !== targetGeneration || duplexInput !== input || !inputRetrying || state.value === 'finished' || state.value === 'error') return
        inputRetrying = false
        state.value = 'connected'
      },
    })
    duplexInput = input
    return input
  }
  function dropInput(): void {
    if (inputTimer) window.clearTimeout(inputTimer)
    inputTimer = undefined
    duplexInput?.close()
    duplexInput = undefined
    inputRetrying = false
  }
  function blockInput(): void { inputBlocked.value = true; dropInput(); finishPresentation() }
  function warmInput(): void { if (mounted && !disposed && inputOpen.value && !inputBlocked.value && state.value !== 'finished' && state.value !== 'error') ensureInput().flush() }
  function flushInput(): void {
    if (disposed || !inputOpen.value || inputBlocked.value || state.value === 'finished' || state.value === 'error') return
    if (inputTimer) window.clearTimeout(inputTimer)
    inputTimer = undefined
    duplexInput?.flush()
  }
  function queueInput(data: string, initialCommand = false): boolean {
    if (disposed || !inputOpen.value || inputBlocked.value || state.value === 'finished' || state.value === 'error' || presentingCommand.value && !initialCommand) return false
    const text = adapter.mode === 'task' ? data.replaceAll('\0', '') : data
    if (!text || !ensureInput().append(text)) return false
    options.acceptedInput?.(text, initialCommand)
    if (terminalInputShouldFlushImmediately(text) || (duplexInput?.byteLength ?? 0) >= 2048) flushInput()
    else if (!inputTimer) inputTimer = window.setTimeout(flushInput, terminalInputFlushInterval)
    return true
  }
  function submitPendingLine(): void {
    if (adapter.mode === 'session' && !pendingLine.value) return
    if (queueInput(terminalLineSubmission(pendingLine.value))) pendingLine.value = ''
  }
  function handlePendingLineEnter(event: KeyboardEvent): void {
    if (!terminalEnterShouldSubmit(event)) return
    event.preventDefault()
    submitPendingLine()
  }
  function executeCommand(command: string): boolean {
    if (adapter.mode !== 'session' || !command.trim() || !queueInput(terminalLineSubmission(command.replace(/\r\n?/g, '\n')))) return false
    focusTerminal()
    return true
  }
  function finishPresentation(): void {
    if (presentationTimer) window.clearTimeout(presentationTimer)
    presentationTimer = undefined
    for (const frame of presentationFrames) window.cancelAnimationFrame(frame)
    presentationFrames.clear()
    presentationWake?.()
    presentationWake = undefined
    commandPresentation = undefined
    presentingCommand.value = false
  }
  function presentationStep(start: (done: () => void) => void): Promise<void> {
    return new Promise(resolve => {
      const done = () => { if (presentationWake === done) presentationWake = undefined; resolve() }
      presentationWake = done
      start(done)
    })
  }
  async function executePresentedCommand(command: string, displayCommand: string): Promise<boolean> {
    if (adapter.mode !== 'session' || !terminal.value || disposed || !canInput.value || state.value !== 'connected'
      || !command.trim() || !displayCommand.trim() || /[\x00-\x1f\x7f-\x9f]/.test(displayCommand)) return false
    const generation = targetGeneration
    const presentation = new TerminalCommandPresentation(command)
    presentingCommand.value = true
    commandPresentation = presentation
    const current = () => !disposed && generation === targetGeneration && !inputBlocked.value && state.value === 'connected' && commandPresentation === presentation
    await presentationStep(done => writeRaw(`\r\x1b[2K$ ${displayCommand}\r\n\x1b[?2004l`, done))
    if (!current()) return false
    await presentationStep(done => {
      const first = window.requestAnimationFrame(() => {
        presentationFrames.delete(first)
        if (!current()) { done(); return }
        const second = window.requestAnimationFrame(() => { presentationFrames.delete(second); done() })
        presentationFrames.add(second)
      })
      presentationFrames.add(first)
    })
    if (disposed || generation !== targetGeneration || inputBlocked.value || state.value !== 'connected' || commandPresentation !== presentation) return false
    if (!queueInput(terminalLineSubmission(presentation.command), true)) { finishPresentation(); return false }
    presentationTimer = window.setTimeout(() => { writeRaw(presentation.release()); finishPresentation() }, 15000)
    focusTerminal()
    return true
  }

  function applyChunk(chunk: TerminalChunk): void {
    const reconnected = state.value !== 'connected'
    // Commit the cursor before writing: a synchronous render drain may resume output.
    offset = chunk.nextOffset
    inputOpen.value = chunk.inputOpen
    state.value = chunk.finished ? 'finished' : 'connected'
    reconnectAttempts = 0
    if (chunk.truncated) {
      if (commandPresentation) { writeRaw(commandPresentation.release()); finishPresentation() }
      writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.outputTruncated')}\x1b[0m\r\n`)
    }
    if (chunk.data) {
      const data = Uint8Array.from(window.atob(chunk.data), c => c.charCodeAt(0))
      writeTerminalOutput(commandPresentation ? commandPresentation.push(data) : data)
      if (commandPresentation && !commandPresentation.pending) finishPresentation()
    }
    if (chunk.finished) {
      if (adapter.mode === 'task') writeRaw(normalizer.flush())
      if (commandPresentation) writeRaw(commandPresentation.release())
      finishPresentation()
      stopOutput()
      dropInput()
    } else {
      if (reconnected) resetResize()
      if (inputOpen.value && duplexInput?.byteLength) flushInput()
    }
    if (chunk.exitError) writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${chunk.exitError}\x1b[0m\r\n`)
  }
  function canReceiveOutput(): boolean {
    return mounted && !disposed && activity.streaming.value && !writeFlow.blocked && state.value !== 'finished' && state.value !== 'error'
  }
  function startOutput(): void {
    if (!canReceiveOutput() || streamSubscription || pollController) return
    const generation = outputGeneration
    // Reserve the slot before subscribing; transports can fail synchronously.
    const pending = { close: () => undefined }
    streamSubscription = pending
    const current = () => !disposed && generation === outputGeneration
    const subscription = adapter.subscribe(offset, inputOpen.value, {
      chunk: chunk => { if (current()) applyChunk(chunk) },
      connected: () => { if (current()) resetResize() },
      error: code => { if (current()) { stopOutput(); outputFailed({ code }) } },
      unavailable: () => { if (current()) { stopOutput(); void poll() } },
    })
    if (!current()) { subscription?.close(); return }
    streamSubscription = subscription
    if (!subscription) void poll()
  }
  function stopOutput(): void {
    outputGeneration++
    const subscription = streamSubscription
    streamSubscription = null
    subscription?.close()
    pollController?.abort()
    pollController = undefined
    if (pollTimer) window.clearTimeout(pollTimer)
    pollTimer = undefined
  }
  function schedulePoll(delay: number): void {
    if (!canReceiveOutput()) return
    if (pollTimer) window.clearTimeout(pollTimer)
    pollTimer = window.setTimeout(() => { pollTimer = undefined; void poll() }, delay)
  }
  function outputFailed(reason: unknown): void {
    const failure = reason && typeof reason === 'object' ? reason as { code?: string; status?: number } : {}
    if (adapter.missing(failure.code ?? '')) {
      state.value = 'finished'
      inputOpen.value = false
      dropInput()
      return
    }
    if (failure.status === 401 || failure.status === 403 || ['unauthorized', 'forbidden', 'session_expired'].includes(failure.code ?? '')) {
      state.value = 'error'
      blockInput()
      writeTerminalOutput(`\r\n\x1b[31m[KPanel] ${t('terminal.connectionFailed')}\x1b[0m\r\n`)
      return
    }
    if (failure.status === 429 || failure.code === 'terminal_stream_limit') { schedulePoll(300); return }
    state.value = 'reconnecting'
    reconnectAttempts++
    schedulePoll(Math.min(5000, 500 * 2 ** Math.min(reconnectAttempts - 1, 3)))
  }
  async function poll(): Promise<void> {
    if (!canReceiveOutput() || streamSubscription || pollController) return
    const generation = outputGeneration
    const controller = new AbortController()
    pollController = controller
    try {
      const chunk = await terminalRequest(controller, signal => adapter.read(offset, inputOpen.value, signal), 15000)
      if (generation !== outputGeneration || disposed) return
      applyChunk(chunk)
      if (generation === outputGeneration && !chunk.finished) schedulePoll(0)
    } catch (reason) {
      if (generation !== outputGeneration || disposed) return
      outputFailed(reason)
    } finally {
      if (pollController === controller) pollController = undefined
    }
  }

  function ownsSize(): boolean { return options.ownsSize?.() !== false }
  function fitTerminal(): void {
    if (disposed || options.visible?.() === false) return
    const size = !ownsSize() ? options.ownerSize?.() : undefined
    if (size && terminal.value) {
      if (terminal.value.rows !== size.rows || terminal.value.cols !== size.columns) terminal.value.resize(size.columns, size.rows)
    } else fitAddon?.fit()
    scheduleResize()
  }
  function queueResize(delay: number): void {
    if (!mounted || disposed || !ownsSize() || !adapter.resize) return
    resizeDirty = true
    if (resizeTimer) window.clearTimeout(resizeTimer)
    resizeTimer = window.setTimeout(() => { resizeTimer = undefined; void syncResize() }, delay)
  }
  function scheduleResize(): void { resizeFailures = 0; queueResize(100) }
  function resetResize(): void {
    resizeGeneration++
    syncedRows = syncedColumns = resizeFailures = 0
    queueResize(100)
  }
  async function syncResize(): Promise<void> {
    if (resizeSending || !mounted || disposed || !ownsSize() || !adapter.resize || !inputOpen.value || state.value === 'finished' || state.value === 'error'
      || options.visible?.() === false || !host.value?.clientWidth || !host.value.clientHeight) return
    fitAddon?.fit()
    const rows = terminal.value?.rows ?? 0, columns = terminal.value?.cols ?? 0
    if (!Number.isSafeInteger(rows) || !Number.isSafeInteger(columns) || rows <= 0 || columns <= 0 || rows > 500 || columns > 1000 || rows === syncedRows && columns === syncedColumns) { resizeDirty = false; return }
    const generation = resizeGeneration
    const target = adapter
    resizeSending = true
    resizeDirty = false
    let retry = false
    try {
      await target.resize!(rows, columns)
      if (disposed || generation !== resizeGeneration || target.key !== adapter.key || !ownsSize()) return
      syncedRows = rows; syncedColumns = columns; resizeFailures = 0
      options.sizeSynced?.({ rows, columns })
      resizeDirty = terminal.value?.rows !== rows || terminal.value?.cols !== columns
    } catch {
      if (disposed || generation !== resizeGeneration || target.key !== adapter.key) return
      resizeFailures++
      if (resizeFailures === 1) writeTerminalOutput(`\r\n\x1b[33m[KPanel] ${t('terminal.resizeFailed')}\x1b[0m\r\n`)
      retry = resizeFailures <= 3
    } finally {
      resizeSending = false
      if (!disposed) {
        if (generation !== resizeGeneration || resizeDirty) queueResize(100)
        else if (retry) queueResize(500 * 2 ** (resizeFailures - 1))
      }
    }
  }

  function resetTarget(): void {
    terminalTouchScroll.end()
    targetGeneration++
    stopOutput(); dropInput(); finishPresentation(); writeFlow.reset(); normalizer.reset()
    adapter = options.adapter()
    offset = initialOffset()
    pendingLine.value = ''
    inputOpen.value = Boolean(options.inputOpen?.())
    inputBlocked.value = false
    state.value = 'connecting'
    reconnectAttempts = 0
    terminal.value?.reset()
    resetResize()
    if (mounted) { startOutput(); warmInput(); focusWhenReady() }
  }
  watch(() => options.adapter().key, resetTarget, { flush: 'sync' })
  watch(() => options.inputOpen?.(), open => { inputOpen.value = Boolean(open) })
  watch(inputOpen, open => {
    if (open) { focusWhenReady(); resetResize(); warmInput() }
    else dropInput()
  })
  watch(activity.streaming, active => { if (active) startOutput(); else stopOutput() })
  watch(activity.focused, focused => { if (focused && adapter.mode === 'task') focusWhenReady() })
  watch(() => options.visible?.(), visible => { if (visible !== false) fitTerminal() })
  watch(() => options.ownsSize?.(), () => { resetResize(); fitTerminal() })
  watch(() => options.ownerSize?.(), () => { if (!ownsSize()) fitTerminal() })
  watch([themeColors, resolvedTheme], () => {
    void nextTick(() => { if (terminal.value && host.value && !disposed) terminal.value.options.theme = readTerminalTheme(host.value) })
  })
  onMounted(() => {
    mounted = true
    const screen = new Terminal({
      ...terminalCursorOptions, convertEol: false,
      fontFamily: '"Cascadia Code", "SFMono-Regular", Consolas, monospace', fontSize: 13,
      lineHeight: 1.25, scrollback: 5000, allowTransparency: true, allowProposedApi: true,
      theme: host.value ? readTerminalTheme(host.value) : undefined,
    })
    terminal.value = screen
    fitAddon = new FitAddon()
    screen.loadAddon(fitAddon)
    screen.loadAddon(new TerminalWebLinksAddon((_event, uri) => void openTerminalURL(uri)))
    screen.loadAddon(new TerminalKeywordColorsAddon())
    // Both viewers ignore hyperlinks/clipboard escape sequences from untrusted output.
    screen.parser.registerOscHandler(8, () => true)
    screen.parser.registerOscHandler(52, () => true)
    screen.attachCustomKeyEventHandler(event => clipboardMenu.value?.handleKeyEvent(event) ?? true)
    screen.onData(data => queueInput(data))
    cursorReport = installTerminalCursorReport(screen)
    if (host.value) {
      screen.open(host.value)
      disposeScrollbar = installTerminalScrollbar(host.value, screen)
      observer = new ResizeObserver(fitTerminal)
      observer.observe(host.value)
      fitTerminal()
      if (options.autoFocus?.() !== false && (adapter.mode === 'session' || inputOpen.value && activity.focused.value)) window.requestAnimationFrame(focusTerminal)
    }
    startOutput(); warmInput()
  })
  onBeforeUnmount(() => {
    terminalTouchScroll.end()
    disposed = true
    mounted = false
    targetGeneration++; resizeGeneration++
    stopOutput(); dropInput(); finishPresentation()
    if (resizeTimer) window.clearTimeout(resizeTimer)
    observer?.disconnect(); disposeScrollbar?.(); cursorReport?.dispose(); terminal.value?.dispose()
  })
  return {
    terminal, state, inputOpen, inputBlocked, presentingCommand, pendingLine, canInput,
    focusTerminal, fitTerminal, scheduleResize, resetResize, containTerminalWheel, terminalTouchScroll,
    queueInput, submitPendingLine, handlePendingLineEnter, executeCommand, executePresentedCommand, blockInput,
  }
}
