<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import { Maximize2, Minimize2, X } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { activateModal, deactivateModal, isTopModal } from './modalStack'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    size?: 'compact' | 'small' | 'medium' | 'large' | 'wide'
    allowFullscreen?: boolean
  }>(),
  {
    description: '',
    size: 'medium',
    allowFullscreen: false,
  },
)

const emit = defineEmits<{
  close: []
}>()
const fullscreen = ref(false)
const panel = ref<HTMLElement>()
const modalID = Symbol('modal-dialog')
const ariaBaseID = `modal-dialog-${useId()}`
const titleID = `${ariaBaseID}-title`
const descriptionID = `${ariaBaseID}-description`
const i18n = useI18n()
let active = false
let activationSequence = 0
let opener: HTMLElement | null = null

function canUseDOM(): boolean {
  return typeof window !== 'undefined' && typeof document !== 'undefined'
}

const focusableSelector = [
  'a[href]',
  'area[href]',
  'button:not(:disabled)',
  'input:not(:disabled):not([type="hidden"])',
  'select:not(:disabled)',
  'textarea:not(:disabled)',
  'iframe',
  'object',
  'embed',
  'summary',
  '[contenteditable]:not([contenteditable="false"])',
  '[tabindex]',
].join(',')

function isAvailableForFocus(element: HTMLElement): boolean {
  if (!canUseDOM()) return false
  if (element.tabIndex < 0 || element.matches(':disabled') || element.closest('[inert]')) return false

  let current: HTMLElement | null = element
  while (current) {
    if (current.hidden || current.getAttribute('aria-hidden') === 'true') return false
    const style = window.getComputedStyle(current)
    if (style.display === 'none' || style.visibility === 'hidden') return false
    if (current === panel.value) break
    current = current.parentElement
  }
  return true
}

function focusableElements(): HTMLElement[] {
  if (!panel.value) return []
  return Array.from(panel.value.querySelectorAll<HTMLElement>(focusableSelector)).filter(isAvailableForFocus)
}

function focusWithoutScroll(element: HTMLElement): void {
  element.focus({ preventScroll: true })
}

async function focusInitialElement(sequence: number): Promise<void> {
  await nextTick()
  if (
    !canUseDOM() ||
    !active ||
    sequence !== activationSequence ||
    !props.open ||
    !isTopModal(modalID) ||
    !panel.value
  ) return
  focusWithoutScroll(focusableElements()[0] || panel.value)
}

async function restoreOpener(element: HTMLElement | null, sequence: number): Promise<void> {
  await nextTick()
  if (!canUseDOM() || active || sequence !== activationSequence || !element?.isConnected) return
  focusWithoutScroll(element)
}

function activate(): void {
  if (active || !canUseDOM()) return
  active = true
  activationSequence += 1
  opener = document.activeElement instanceof HTMLElement && document.activeElement !== document.body
    ? document.activeElement
    : null
  activateModal(modalID)
  window.addEventListener('keydown', onKeyDown)
  void focusInitialElement(activationSequence)
}

function deactivate(): void {
  if (!active) return
  const wasTopModal = isTopModal(modalID)
  const sequence = activationSequence
  const restoreTarget = opener
  active = false
  opener = null
  deactivateModal(modalID)
  if (typeof window !== 'undefined') window.removeEventListener('keydown', onKeyDown)
  if (wasTopModal) void restoreOpener(restoreTarget, sequence)
}

function close(): void {
  fullscreen.value = false
  emit('close')
}

function onKeyDown(event: KeyboardEvent): void {
  if (!isTopModal(modalID)) return
  if (event.key === 'Escape') {
    close()
    return
  }
  if (event.key !== 'Tab' || !panel.value) return

  const elements = focusableElements()
  if (elements.length === 0) {
    event.preventDefault()
    focusWithoutScroll(panel.value)
    return
  }

  const first = elements[0]!
  const last = elements[elements.length - 1]!
  const current = document.activeElement
  const focusIsInside = current instanceof Node && panel.value.contains(current)

  if (event.shiftKey && (!focusIsInside || current === first || current === panel.value)) {
    event.preventDefault()
    focusWithoutScroll(last)
  } else if (!event.shiftKey && (!focusIsInside || current === last || current === panel.value)) {
    event.preventDefault()
    focusWithoutScroll(first)
  }
}

watch(
  () => props.open,
  (open) => {
    if (open) activate()
    else deactivate()
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  deactivate()
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="modal-backdrop"
      :class="{ 'modal-backdrop--fullscreen': fullscreen }"
      role="presentation"
      @mousedown.self="close"
    >
      <section
        ref="panel"
        class="modal-panel"
        :class="[`modal-panel--${size}`, { 'modal-panel--fullscreen': fullscreen }]"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleID"
        :aria-describedby="description ? descriptionID : undefined"
        tabindex="-1"
      >
        <header class="modal-panel__header">
          <div>
            <h2 :id="titleID">{{ title }}</h2>
            <p v-if="description" :id="descriptionID">{{ description }}</p>
          </div>
          <div class="modal-panel__actions">
            <button
              v-if="allowFullscreen"
              class="icon-button"
              type="button"
              :aria-label="i18n.t(fullscreen ? 'common.exitFullscreen' : 'common.enterFullscreen')"
              @click="fullscreen = !fullscreen"
            >
              <Minimize2 v-if="fullscreen" :size="18" />
              <Maximize2 v-else :size="18" />
            </button>
            <button class="icon-button" type="button" :aria-label="i18n.t('common.closeDialog')" @click="close">
              <X :size="19" />
            </button>
          </div>
        </header>
        <div class="modal-panel__body">
          <slot />
        </div>
        <footer v-if="$slots.footer" class="modal-panel__footer">
          <slot name="footer" />
        </footer>
      </section>
    </div>
  </Teleport>
</template>
