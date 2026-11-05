<script setup lang="ts">
import { nextTick, computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import { api, ApiError } from '@/lib/api'
import { closeTerminalQuietly } from '@/lib/terminalClose'
import { closeTerminalsOnPageHide } from '@/lib/terminalPageHide'
import { hostTerminalAdapter } from '@/lib/terminalAdapters'
import { useTerminalController } from '@/composables/useTerminalController'
import { useI18n } from '@/i18n'
import { isTouchPrimary } from '@/lib/touchFocus'
import { createTerminalCommandCapture, type TerminalCommandCapture } from '@/lib/terminalCommandCapture'
import { useTerminalHistory } from '@/stores/terminalHistory'

const props = withDefaults(defineProps<{
  sessionId: string
  hostName: string
  initialOffset: number
  visible?: boolean
  autoFocus?: boolean
  retained?: boolean
  closePending?: boolean
  historyHostId?: string
}>(), { visible: true, autoFocus: true, retained: false, historyHostId: '' })
const emit = defineEmits<{ stateChange: [state: 'connecting' | 'connected' | 'reconnecting' | 'finished'] }>()
const { t } = useI18n()
const host = ref<HTMLElement>()
const clipboardMenu = ref<InstanceType<typeof TerminalContextMenu>>()
const composerInput = ref<HTMLInputElement>()
const terminalHistory = useTerminalHistory()
const mobileTerminalKeys = [
  ['Esc', '\x1b'], ['Tab', '\t'], ['↑', '\x1b[A'], ['↓', '\x1b[B'], ['←', '\x1b[D'], ['→', '\x1b[C'],
  ['Space', ' '], ['Ctrl+C', '\x03'], ['Ctrl+D', '\x04'], ['Ctrl+Z', '\x1a'], ['Ctrl+L', '\x0c'], ['Enter', '\r'],
] as const
let commandCapture: TerminalCommandCapture | undefined
let historyIndex = -1
let historyDraft = ''
const adapter = computed(() => hostTerminalAdapter(props.sessionId))
const {
  terminal, state, pendingLine, canInput,
  focusTerminal, executeCommand, executePresentedCommand, scheduleResize, stopForClose, queueInput,
  containTerminalWheel, terminalTouchScroll, submitPendingLine, handlePendingLineEnter,
} = useTerminalController({
  adapter: () => adapter.value, host, clipboardMenu,
  acceptedInput: (data, initialCommand) => {
    if (initialCommand) commandCapture?.reset()
    else commandCapture?.input(data)
    historyIndex = -1
  },
  initialOffset: () => props.initialOffset, inputOpen: () => true,
  visible: () => props.visible, autoFocus: () => props.autoFocus,
})
function bindCommandCapture(): void {
  commandCapture?.dispose()
  commandCapture = undefined
  const screen = terminal.value
  const hostId = props.historyHostId
  if (screen && hostId) {
    commandCapture = createTerminalCommandCapture(screen, command => terminalHistory.record(command, hostId))
    void terminalHistory.ensureLoaded()
  }
}
watch([terminal, () => props.sessionId, () => props.historyHostId], bindCommandCapture, { flush: 'sync' })
function moveComposerCaretToEnd(focus: boolean): void {
  void nextTick(() => {
    const input = composerInput.value
    if (!input) return
    if (focus && !isTouchPrimary()) input.focus()
    if (document.activeElement === input) input.setSelectionRange(input.value.length, input.value.length)
  })
}
function fillCommand(command: string): boolean {
  if (!command.trim() || command.includes('\n') || !canInput.value) return false
  pendingLine.value = command
  historyIndex = -1
  moveComposerCaretToEnd(true)
  return true
}
function browseHistory(event: KeyboardEvent, step: 1 | -1): void {
  if (!props.historyHostId || event.isComposing || event.keyCode === 229 || !canInput.value) return
  const commands = terminalHistory.history.value.items.map(item => item.command).filter(command => !command.includes('\n'))
  if (historyIndex === -1 && step < 0) return
  const next = historyIndex + step
  if (next >= commands.length) return
  event.preventDefault()
  if (historyIndex === -1) historyDraft = pendingLine.value
  historyIndex = next
  pendingLine.value = next === -1 ? historyDraft : commands[next]!
  moveComposerCaretToEnd(false)
}
function editComposer(): void { historyIndex = -1 }
function sendMobileTerminalKey(sequence: string): void {
  if (!canInput.value) return
  if (sequence === '\r') {
    if (queueInput(`${pendingLine.value}\r`)) pendingLine.value = ''
    return
  }
  if (sequence === '\t' && pendingLine.value) {
    if (queueInput(`${pendingLine.value}${sequence}`)) pendingLine.value = ''
    return
  }
  if (sequence === ' ' && pendingLine.value) {
    pendingLine.value += sequence
    return
  }
  queueInput(sequence)
}
let closeRequest: Promise<void> | undefined
let closeConfirmed = false
let detached = false
let stopPageHideClose: (() => void) | undefined
function markDetached(): void { detached = true }

function closeSession(): Promise<void> {
  stopForClose()
  if (closeConfirmed) return Promise.resolve()
  if (closeRequest) return closeRequest
  const id = props.sessionId
  const request = api.terminals.close(id).then(result => {
    if (!result.closed) throw new Error('Terminal close was not confirmed')
    if (id === props.sessionId) closeConfirmed = true
  }).catch((reason: unknown) => {
    if (reason instanceof ApiError && reason.code === 'terminal_not_found') {
      if (id === props.sessionId) closeConfirmed = true
      return
    }
    throw reason
  }).finally(() => { if (closeRequest === request) closeRequest = undefined })
  closeRequest = request
  return request
}
watch(state, value => emit('stateChange', value === 'error' ? 'reconnecting' : value), { immediate: true })
watch(() => props.closePending, pending => { if (pending) stopForClose() }, { immediate: true })
watch(() => props.sessionId, (_id, previous) => {
  closeConfirmed = false
  detached = false
  closeRequest = undefined
  // A reused temporary viewer must not leave its previous PTY behind.
  if (previous && !previousRetained) void closeTerminalQuietly(previous, async () => {
    if (!(await api.terminals.close(previous)).closed) throw new Error('Terminal close was not confirmed')
  })
  previousRetained = props.retained
}, { flush: 'sync' })
let previousRetained = props.retained
watch(() => props.retained, value => { previousRetained = value })
onMounted(() => {
  stopPageHideClose = closeTerminalsOnPageHide(() => closeConfirmed || detached || props.retained ? [] : [props.sessionId])
})
onBeforeUnmount(() => {
  stopPageHideClose?.()
  commandCapture?.dispose()
  if (!props.retained && !detached) void closeTerminalQuietly(props.sessionId, closeSession)
})
defineExpose({ focusTerminal, executeCommand, executePresentedCommand, fillCommand, scheduleResize, closeSession, markDetached })
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
    <div class="host-terminal__mobile-keys" role="group" :aria-label="t('terminal.mobileKeys')">
      <button
        v-for="([label, sequence]) in mobileTerminalKeys"
        :key="label"
        class="host-terminal__mobile-key"
        type="button"
        :aria-label="label"
        :disabled="!canInput"
        @pointerdown.prevent
        @click="sendMobileTerminalKey(sequence)"
      >{{ label }}</button>
    </div>
    <form class="host-terminal__composer" @submit.prevent="submitPendingLine">
      <input
        ref="composerInput"
        v-model="pendingLine"
        type="text"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        maxlength="8192"
        :placeholder="t('terminal.inputPlaceholder')"
        :disabled="!canInput"
        @keydown.enter="handlePendingLineEnter"
        @keydown.up="browseHistory($event, 1)"
        @keydown.down="browseHistory($event, -1)"
        @input="editComposer"
      />
      <button type="submit" :disabled="!canInput">{{ t('terminal.send') }}</button>
    </form>
    <TerminalContextMenu
      ref="clipboardMenu"
      :get-terminal="() => terminal"
      :can-paste="canInput"
    />
  </section>
</template>

<style scoped>
.host-terminal { display:grid; height:100%; grid-template-rows:minmax(0,1fr) auto; min-height:0; overflow:hidden; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--terminal-shell-radius,12px); background:var(--terminal-shell-background,#0b1214); box-shadow:var(--terminal-shell-shadow); }
.host-terminal__screen { position:relative; min-width:0; min-height:0; overflow:hidden; overscroll-behavior:contain; padding:0; }
.host-terminal__screen :deep(.xterm) { box-sizing:border-box; height:100%; padding:6px 8px 4px; touch-action:none; }
.host-terminal__screen :deep(.xterm-viewport) { overflow-y:scroll !important; overscroll-behavior:contain; background:transparent; }
.host-terminal__screen :deep(.xterm-scrollable-element) { overscroll-behavior:contain; }
.host-terminal__mobile-keys { display:none; }
.host-terminal__composer { position:relative; z-index:2; display:grid; grid-template-columns:minmax(0,1fr) auto; gap:8px; padding:9px 10px; border-top:1px solid var(--terminal-shell-border,#29383a); background:var(--terminal-shell-panel,#111a1d); }
.host-terminal__composer input { min-width:0; border:1px solid var(--terminal-shell-border,#29383a); border-radius:8px; padding:9px 11px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); font:14px ui-monospace,SFMono-Regular,Menlo,Consolas,monospace; }
.host-terminal__composer button { border:0; border-radius:8px; padding:0 16px; color:var(--on-brand,#05251c); background:var(--brand-action,#35cba6); font-weight: 700; }
@media (max-width: 900px) {
  .host-terminal { grid-template-rows:minmax(0,1fr) auto auto; }
  .host-terminal__mobile-keys { display:grid; grid-template-columns:repeat(6,minmax(0,1fr)); gap:6px; padding:8px 10px 0; }
  .host-terminal__mobile-key { min-width:0; min-height:40px; border:1px solid var(--terminal-shell-border,#29383a); border-radius:var(--radius-sm,8px); padding:4px 2px; color:var(--terminal-shell-text,#d8dddc); background:var(--terminal-shell-background,#0b1214); font:600 12px ui-sans-serif,system-ui,sans-serif; cursor:pointer; touch-action:manipulation; }
  .host-terminal__mobile-key:active:not(:disabled),.host-terminal__mobile-key:focus-visible { border-color:var(--brand,#35cba6); outline:2px solid var(--brand,#35cba6); outline-offset:1px; }
  .host-terminal__mobile-key:disabled { cursor:not-allowed; opacity:.6; }
}
/* Workspace dialogs run the terminal to the window edges. */
:global(.modal-panel--workspace .host-terminal) { border:0; border-radius:0; box-shadow:none; }
</style>
