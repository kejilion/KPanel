<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import type { Component } from 'vue'
import { Copy, Globe2, LoaderCircle } from '@lucide/vue'
import type { DesktopEntry } from '@/lib/desktopEntries'

/**
 * One desktop icon tile. A static navigation app renders a lucide icon; an
 * app-market app renders its market icon image; a site renders its favicon or
 * a globe fallback. Windows-style interaction is intentional: single-click
 * selects and double-click opens. Touch/pen opens on one tap and exposes the
 * same parent-owned context menu through a cancellable long press.
 */

const props = defineProps<{
  label: string
  navIcon?: Component
  entry?: DesktopEntry
  gradient: string
  active?: boolean
  selected?: boolean
  order?: number
  dragging?: boolean
  transferHint?: string
  transferReady?: boolean
}>()

const emit = defineEmits<{
  select: [event: MouseEvent]
  open: [event: MouseEvent | KeyboardEvent]
  context: [event: MouseEvent]
  warm: []
  nudge: [deltaX: number, deltaY: number]
}>()

const LONG_PRESS_DURATION = 520
const LONG_PRESS_MOVE_TOLERANCE = 10
const CLICK_SUPPRESSION_DURATION = 800

const iconElement = ref<HTMLButtonElement>()
const imageFailed = ref(false)
let activePointerId: number | undefined
let longPressTimer: number | undefined
let pressX = 0
let pressY = 0
let lastPointerType: string | undefined
let suppressClickUntil = 0
let suppressNativeContextUntil = 0
let touchOpenUntil = 0
let dispatchingCustomContext = false

watch(
  () => props.entry?.iconURL,
  () => {
    imageFailed.value = false
  },
)

const monogram = computed(() => props.label.trim().slice(0, 1).toLocaleUpperCase() || 'K')
const accessibleLabel = computed(() => {
  const domain = props.entry?.kind === 'site' ? props.entry.site?.primaryDomain : undefined
  const label = domain && domain !== props.label ? `${props.label} · ${domain}` : props.label
  return props.transferHint ? `${label} · ${props.transferHint}` : label
})

function isCoarseInput(): boolean {
  return window.matchMedia?.('(hover: none), (pointer: coarse)').matches ?? false
}

function isTouchActivation(): boolean {
  if (lastPointerType === 'touch' || lastPointerType === 'pen') return true
  return lastPointerType === undefined && (isCoarseInput() || window.innerWidth <= 760)
}

function removeLongPressListeners(): void {
  window.removeEventListener('pointermove', onLongPressMove)
  window.removeEventListener('pointerup', onLongPressEnd)
  window.removeEventListener('pointercancel', onLongPressCancel)
}

function clearLongPress(): void {
  if (longPressTimer !== undefined) {
    window.clearTimeout(longPressTimer)
    longPressTimer = undefined
  }
  activePointerId = undefined
  removeLongPressListeners()
}

function dispatchContextMenu(clientX: number, clientY: number): void {
  const element = iconElement.value
  if (!element) return
  dispatchingCustomContext = true
  try {
    element.dispatchEvent(new MouseEvent('contextmenu', {
      bubbles: true,
      cancelable: true,
      button: 2,
      clientX,
      clientY,
    }))
  } finally {
    dispatchingCustomContext = false
  }
}

function activateLongPress(): void {
  longPressTimer = undefined
  suppressClickUntil = Date.now() + CLICK_SUPPRESSION_DURATION
  suppressNativeContextUntil = suppressClickUntil
  dispatchContextMenu(pressX, pressY)
}

function onPointerDown(event: PointerEvent): void {
  lastPointerType = event.pointerType || undefined
  const touchLike = event.pointerType === 'touch'
    || event.pointerType === 'pen'
    || (!event.pointerType && (isCoarseInput() || window.innerWidth <= 760))
  if (!touchLike || event.button !== 0 || activePointerId !== undefined) return

  clearLongPress()
  activePointerId = event.pointerId
  pressX = event.clientX
  pressY = event.clientY
  longPressTimer = window.setTimeout(activateLongPress, LONG_PRESS_DURATION)
  window.addEventListener('pointermove', onLongPressMove, { passive: true })
  window.addEventListener('pointerup', onLongPressEnd, { passive: true })
  window.addEventListener('pointercancel', onLongPressCancel, { passive: true })
}

function onLongPressMove(event: PointerEvent): void {
  if (event.pointerId !== activePointerId) return
  if (Math.hypot(event.clientX - pressX, event.clientY - pressY) <= LONG_PRESS_MOVE_TOLERANCE) return
  suppressClickUntil = Date.now() + CLICK_SUPPRESSION_DURATION
  clearLongPress()
}

function onLongPressEnd(event: PointerEvent): void {
  if (event.pointerId !== activePointerId) return
  clearLongPress()
}

function onLongPressCancel(event: PointerEvent): void {
  if (event.pointerId !== activePointerId) return
  suppressClickUntil = Date.now() + CLICK_SUPPRESSION_DURATION
  clearLongPress()
}

function onSelect(event: MouseEvent): void {
  if (Date.now() < suppressClickUntil) {
    event.preventDefault()
    event.stopPropagation()
    return
  }
  if (isTouchActivation()) {
    event.preventDefault()
    if (Date.now() < touchOpenUntil) return
    touchOpenUntil = Date.now() + 450
    emit('open', event)
    return
  }
  emit('select', event)
}

function onOpen(event: MouseEvent | KeyboardEvent): void {
  event.preventDefault()
  if (event instanceof MouseEvent && (isTouchActivation() || Date.now() < touchOpenUntil)) {
    event.stopPropagation()
    return
  }
  emit('open', event)
}

function onContext(event: MouseEvent): void {
  event.preventDefault()
  event.stopPropagation()
  if (!dispatchingCustomContext && Date.now() < suppressNativeContextUntil) return
  if (!dispatchingCustomContext && activePointerId !== undefined) {
    suppressClickUntil = Date.now() + CLICK_SUPPRESSION_DURATION
    suppressNativeContextUntil = suppressClickUntil
    if (longPressTimer !== undefined) window.clearTimeout(longPressTimer)
    longPressTimer = undefined
  }
  emit('context', event)
}

function onKeyDown(event: KeyboardEvent): void {
  if ((event.ctrlKey || event.metaKey) && event.key.startsWith('Arrow')) {
    event.preventDefault()
    event.stopPropagation()
    const deltaX = event.key === 'ArrowLeft' ? -1 : event.key === 'ArrowRight' ? 1 : 0
    const deltaY = event.key === 'ArrowUp' ? -1 : event.key === 'ArrowDown' ? 1 : 0
    emit('nudge', deltaX, deltaY)
    return
  }
  if (event.key === 'Enter' || event.key === ' ') {
    suppressClickUntil = Date.now() + 120
    onOpen(event)
    return
  }
  if (event.key !== 'ContextMenu' && !(event.shiftKey && event.key === 'F10')) return
  event.preventDefault()
  event.stopPropagation()
  const rect = iconElement.value?.getBoundingClientRect()
  dispatchContextMenu(rect?.left ?? 12, rect?.bottom ?? 12)
}

function onImageError(): void {
  imageFailed.value = true
}

onBeforeUnmount(clearLongPress)
</script>

<template>
  <button
    ref="iconElement"
    class="desktop__icon"
    :class="{
      'desktop__icon--launching': active,
      'desktop__icon--selected': selected,
      'desktop__icon--dragging': dragging,
    }"
    :style="{ '--desktop-entry-order': String(order ?? 0) }"
    type="button"
    :aria-label="accessibleLabel"
    :aria-pressed="selected"
    :title="accessibleLabel"
    aria-keyshortcuts="Control+ArrowUp Control+ArrowDown Control+ArrowLeft Control+ArrowRight"
    @pointerdown="onPointerDown"
    @click="onSelect"
    @dblclick="onOpen"
    @keydown="onKeyDown"
    @focus="emit('warm')"
    @pointerenter="emit('warm')"
    @contextmenu="onContext"
  >
    <span
      class="desktop__icon-glyph"
      :class="{ 'desktop__icon-glyph--dynamic': Boolean(entry) }"
      :style="{ background: gradient }"
    >
      <img
        v-if="entry?.iconURL && !imageFailed"
        class="desktop__icon-img"
        :src="entry.iconURL"
        alt=""
        draggable="false"
        loading="lazy"
        decoding="async"
        referrerpolicy="no-referrer"
        width="38"
        height="38"
        @error="onImageError"
      />
      <component
        v-else-if="navIcon || entry?.icon"
        :is="navIcon || entry?.icon"
        :size="38"
        :stroke-width="1.6"
        aria-hidden="true"
      />
      <span v-else-if="entry?.kind === 'site'" class="desktop__site-fallback" aria-hidden="true">
        <span class="desktop__site-fallback-letter">{{ monogram }}</span>
        <span class="desktop__site-fallback-badge"><Globe2 :size="10" :stroke-width="2.2" /></span>
      </span>
      <span v-else class="desktop__icon-monogram" aria-hidden="true">{{ monogram }}</span>
    </span>
    <span class="desktop__icon-label">{{ label }}</span>
    <span
      v-if="transferHint"
      class="desktop__icon-transfer-badge"
      :class="{ 'desktop__icon-transfer-badge--ready': transferReady }"
      :title="transferHint"
      aria-hidden="true"
    >
      <Copy v-if="transferReady" :size="11" :stroke-width="2.4" />
      <LoaderCircle v-else :size="11" :stroke-width="2.4" />
    </span>
  </button>
</template>
