<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import TerminalContextMenu from '@/components/terminal/TerminalContextMenu.vue'
import TerminalToolbar from '@/components/terminal/TerminalToolbar.vue'
import { jobTerminalAdapter } from '@/lib/terminalAdapters'
import { joinTerminalSizeGroup, type TerminalSizeMembership } from '@/lib/terminalSizeOwnership'
import { useTerminalController } from '@/composables/useTerminalController'
import { useMobileTerminalCtrl } from '@/composables/useMobileTerminalCtrl'
import { useTerminalActivity } from '@/composables/useTerminalActivity'
import { useTerminalFullscreen } from '@/composables/useTerminalFullscreen'
import { useI18n } from '@/i18n'

const props = defineProps<{
  jobId: string
  inputOpen?: boolean
  kind?: 'app' | 'site' | 'diagnostic' | 'environment'
  compact?: boolean
  headless?: boolean
}>()
const { locale, t } = useI18n()
const activity = useTerminalActivity()
const host = ref<HTMLElement>()
const clipboardMenu = ref<InstanceType<typeof TerminalContextMenu>>()
const mobileTerminalKeys = [
  ['Esc', '\x1b'], ['Tab', '\t'], ['↑', '\x1b[A'], ['↓', '\x1b[B'], ['←', '\x1b[D'], ['→', '\x1b[C'],
  ['Space', ' '], ['Ctrl+C', '\x03'], ['Ctrl+D', '\x04'], ['Ctrl+Z', '\x1a'], ['Ctrl+L', '\x0c'],
] as const
const sizeMembership = shallowRef<TerminalSizeMembership>()
const ownsSize = computed(() => sizeMembership.value?.isOwner() ?? true)
const ownerSize = computed(() => sizeMembership.value?.ownerSize())
function joinSizeGroup(): void {
  sizeMembership.value?.leave()
  sizeMembership.value = !props.kind || props.kind === 'app' ? joinTerminalSizeGroup(`app:${props.jobId}`) : undefined
  if (activity.focused.value) sizeMembership.value?.claim()
}
watch(() => `${props.kind ?? 'app'}:${props.jobId}`, joinSizeGroup, { immediate: true, flush: 'sync' })
watch(activity.focused, focused => { if (focused) sizeMembership.value?.claim() })
const adapter = computed(() => jobTerminalAdapter(props.kind ?? 'app', props.jobId))
const {
  terminal, state, inputOpen: terminalInputOpen, pendingLine, canInput,
  focusTerminal, fitTerminal, containTerminalWheel, terminalTouchScroll, queueInput, submitPendingLine, handlePendingLineEnter,
} = useTerminalController({
  adapter: () => adapter.value, host, clipboardMenu, activity,
  inputOpen: () => props.inputOpen,
  ownsSize: () => ownsSize.value, ownerSize: () => ownerSize.value,
  sizeSynced: size => sizeMembership.value?.publish(size),
})
const { fullscreen, toggleFullscreen } = useTerminalFullscreen(fitTerminal)
const { ctrlActive, toggleCtrl, captureCtrlInput, startCtrlComposition, finishCtrlComposition } = useMobileTerminalCtrl(canInput, queueInput)
function sendMobileTerminalKey(sequence: string): void {
  ctrlActive.value = false
  if (!canInput.value) return
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
watch(() => `${props.kind ?? 'app'}:${props.jobId}`, () => { ctrlActive.value = false })
const connectionState = computed(() => state.value === 'reconnecting' ? 'error' : state.value)
const taskKindLabel = computed(() => { locale.value; return t(`terminal.task.kind.${props.kind ?? 'app'}`) })
const taskDescription = computed(() => { locale.value; return t(`terminal.task.description.${props.kind ?? 'app'}`) })
const connectionStatusLabel = computed(() => {
  locale.value
  if (state.value === 'connected') return terminalInputOpen.value ? t('terminal.task.inputReady') : t('terminal.task.running')
  if (state.value === 'finished') return t('terminal.task.finished')
  if (state.value === 'error') return t('terminal.connectionFailed')
  if (state.value === 'reconnecting') return t('terminal.reconnecting')
  return t('terminal.connecting')
})
// Business jobs keep running when their viewer leaves; only release size ownership.
onBeforeUnmount(() => sizeMembership.value?.leave())
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
      <div class="interactive-terminal__mobile-keys" role="group" :aria-label="t('terminal.mobileKeys')">
        <button
          v-for="([label, sequence]) in mobileTerminalKeys"
          :key="label"
          class="interactive-terminal__mobile-key"
          type="button"
          :aria-label="label"
          :disabled="!canInput"
          @pointerdown.prevent
          @click="sendMobileTerminalKey(sequence)"
        >{{ label }}</button>
        <button
          class="interactive-terminal__mobile-key"
          type="button"
          :aria-label="t('terminal.mobileCtrlHint')"
          :aria-pressed="ctrlActive"
          :title="t('terminal.mobileCtrlHint')"
          :disabled="!canInput"
          @pointerdown.prevent
          @click="toggleCtrl"
        >Ctrl{{ ctrlActive ? ' ✓' : '' }}</button>
      </div>
      <form
        class="interactive-terminal__composer"
        @submit.prevent="ctrlActive = false; submitPendingLine()"
      >
        <input
          v-model="pendingLine"
          type="text"
          :aria-label="t('terminal.task.inputLabel')"
          autocomplete="off"
          autocapitalize="off"
          spellcheck="false"
          maxlength="8192"
          :placeholder="ctrlActive ? t('terminal.mobileCtrlHint') : t('terminal.task.inputPlaceholder')" :disabled="!canInput"
          @beforeinput="captureCtrlInput"
          @compositionstart="startCtrlComposition"
          @compositionend.capture="finishCtrlComposition"
          @keydown.enter="ctrlActive = false; handlePendingLineEnter($event)"
        />
        <button type="submit" :disabled="!canInput">{{ t('terminal.send') }}</button>
      </form>
    </div>
    <TerminalContextMenu
      ref="clipboardMenu"
      :get-terminal="() => terminal"
      :can-paste="canInput"
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

.interactive-terminal__mobile-keys {
  display: none;
}

@media (max-width: 900px) {
  .interactive-terminal__mobile-keys {
    display: grid;
    grid-template-columns: repeat(6, minmax(0, 1fr));
    gap: 6px;
  }

  .interactive-terminal__mobile-key {
    min-width: 0;
    min-height: 40px;
    border: 1px solid var(--terminal-border);
    border-radius: var(--radius-sm, 8px);
    padding: 4px 2px;
    color: var(--terminal-text);
    background: var(--terminal-background);
    font: 600 12px ui-sans-serif, system-ui, sans-serif;
    cursor: pointer;
    touch-action: manipulation;
  }

  .interactive-terminal__mobile-key:active:not(:disabled),
  .interactive-terminal__mobile-key:focus-visible {
    border-color: var(--terminal-accent);
    outline: 2px solid var(--terminal-accent);
    outline-offset: 1px;
  }

  .interactive-terminal__mobile-key[aria-pressed="true"] {
    border-color: var(--terminal-accent);
    color: var(--terminal-background);
    background: var(--terminal-accent);
  }

  .interactive-terminal__mobile-key:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }
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
