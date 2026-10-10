<script lang="ts">
export interface ClusterHoverInfoRow {
  label: string
  value: string
}

// Only one card is open at a time; opening another closes the previous one.
let closeActiveCard: (() => void) | undefined
</script>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'

/**
 * Icon trigger with a read-only info card. The card opens on hover, keyboard
 * focus or tap, and is teleported so card overflow and the list's horizontal
 * scroller never clip it. Teleported content sits outside the phrase observer
 * root, so callers pass already translated text. The card takes the pointer
 * itself, so rows underneath never react through it, and it stays open while
 * the pointer moves onto it.
 */
defineOptions({ inheritAttrs: false })

const props = withDefaults(defineProps<{
  /** Accessible name; repeats the card summary because tooltips are not reliably announced. */
  label: string
  title: string
  rows?: ClusterHoverInfoRow[]
  note?: string
}>(), { rows: () => [], note: '' })

const trigger = ref<HTMLButtonElement>()
const panel = ref<HTMLElement>()
const open = ref(false)
const pinned = ref(false)
const position = ref({ left: 0, top: 0 })
const placement = ref<'below' | 'above'>('below')
const id = `cluster-hover-info-${useId()}`
let hoverTimer: number | undefined
let leaveTimer: number | undefined
let cardHovered = false

function place(): void {
  const anchor = trigger.value?.getBoundingClientRect()
  const card = panel.value
  if (!anchor || !card) return
  const margin = 8
  const width = card.offsetWidth
  const height = card.offsetHeight
  const fitsBelow = anchor.bottom + margin + height <= window.innerHeight - margin
  const fitsAbove = anchor.top - margin - height >= margin
  placement.value = fitsBelow || !fitsAbove ? 'below' : 'above'
  const top = placement.value === 'below' ? anchor.bottom + margin : anchor.top - margin - height
  const left = Math.min(Math.max(margin, anchor.left - 6), Math.max(margin, window.innerWidth - margin - width))
  position.value = { left, top: Math.max(margin, top) }
}

async function show(): Promise<void> {
  window.clearTimeout(hoverTimer)
  window.clearTimeout(leaveTimer)
  if (open.value) return
  if (closeActiveCard !== close) closeActiveCard?.()
  closeActiveCard = close
  open.value = true
  await nextTick()
  place()
}

function close(): void {
  window.clearTimeout(hoverTimer)
  window.clearTimeout(leaveTimer)
  cardHovered = false
  if (closeActiveCard === close) closeActiveCard = undefined
  pinned.value = false
  open.value = false
}

function onPointerEnter(event: PointerEvent): void {
  if (event.pointerType === 'touch') return
  window.clearTimeout(hoverTimer)
  window.clearTimeout(leaveTimer)
  hoverTimer = window.setTimeout(() => void show(), 120)
}

/** The grace period lets the pointer cross the gap between trigger and card. */
function onPointerLeave(event: PointerEvent): void {
  if (event.pointerType === 'touch') return
  window.clearTimeout(hoverTimer)
  window.clearTimeout(leaveTimer)
  leaveTimer = window.setTimeout(() => {
    // A pinned card, or one opened from the keyboard, stays with its trigger.
    if (!pinned.value && !trigger.value?.matches(':focus-visible')) close()
  }, 160)
}

function onCardPointerEnter(): void {
  cardHovered = true
  window.clearTimeout(leaveTimer)
}

function onCardPointerLeave(event: PointerEvent): void {
  cardHovered = false
  onPointerLeave(event)
}

/**
 * Pressing inside the card (to select an ASN, say) drops focus to nothing;
 * focus moving to another control still closes it.
 */
function onBlur(event: FocusEvent): void {
  if (cardHovered && !event.relatedTarget) return
  close()
}

function onClick(): void {
  if (open.value && pinned.value) {
    close()
    return
  }
  pinned.value = true
  void show()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape' || !open.value) return
  event.stopPropagation()
  close()
}

function onDocumentPointer(event: PointerEvent): void {
  if (event.target instanceof Node && (trigger.value?.contains(event.target) || panel.value?.contains(event.target))) return
  close()
}

/**
 * Focusing an off-screen trigger scrolls it into view after the card opened,
 * so scrolling follows the trigger and only closes once it leaves the viewport.
 */
function onScroll(): void {
  const anchor = trigger.value?.getBoundingClientRect()
  if (!anchor || anchor.bottom < 0 || anchor.top > window.innerHeight) close()
  else place()
}

function listen(active: boolean): void {
  if (active) {
    document.addEventListener('pointerdown', onDocumentPointer, true)
    document.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', close)
    window.addEventListener('blur', close)
    return
  }
  document.removeEventListener('pointerdown', onDocumentPointer, true)
  document.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', close)
  window.removeEventListener('blur', close)
}

watch(open, listen)

onBeforeUnmount(() => {
  close()
  listen(false)
})
</script>

<template>
  <button
    v-bind="$attrs"
    ref="trigger"
    class="cluster-hover-info"
    type="button"
    :aria-label="props.label"
    :aria-describedby="open ? id : undefined"
    :aria-expanded="open"
    @pointerenter="onPointerEnter"
    @pointerleave="onPointerLeave"
    @focus="show"
    @blur="onBlur"
    @click="onClick"
    @keydown="onKeydown"
  >
    <slot name="trigger" />
  </button>
  <Teleport to="body">
    <div
      v-if="open"
      :id="id"
      ref="panel"
      class="cluster-hover-info__card"
      :class="`is-${placement}`"
      role="tooltip"
      :style="{ left: `${position.left}px`, top: `${position.top}px` }"
      @pointerenter="onCardPointerEnter"
      @pointerleave="onCardPointerLeave"
    >
      <header class="cluster-hover-info__header">
        <span class="cluster-hover-info__icon" aria-hidden="true"><slot name="trigger" /></span>
        <strong data-i18n-ignore>{{ props.title }}</strong>
      </header>
      <dl v-if="props.rows.length" class="cluster-hover-info__rows">
        <div v-for="row in props.rows" :key="row.label">
          <dt>{{ row.label }}</dt>
          <dd data-i18n-ignore>{{ row.value }}</dd>
        </div>
      </dl>
      <p v-if="props.note" class="cluster-hover-info__note">{{ props.note }}</p>
    </div>
  </Teleport>
</template>

<style scoped>
.cluster-hover-info {
  display: inline-grid;
  flex: 0 0 auto;
  min-width: 24px;
  min-height: 24px;
  place-items: center;
  padding: 0;
  color: inherit;
  cursor: help;
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
  transition: transform 150ms ease, box-shadow 150ms ease;
}

.cluster-hover-info:hover,
.cluster-hover-info[aria-expanded='true'] {
  transform: translateY(-1px);
}

.cluster-hover-info:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.cluster-hover-info__card {
  position: fixed;
  z-index: 7000;
  display: grid;
  box-sizing: border-box;
  width: max-content;
  min-width: 13rem;
  max-width: min(22rem, calc(100vw - 16px));
  gap: 10px;
  padding: 12px 14px;
  color: var(--text);
  background: var(--surface-raised, var(--surface));
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow-md);
  font-size: 14px;
  line-height: 1.5;
  cursor: default;
  animation: cluster-hover-info-in 140ms ease-out;
}

.cluster-hover-info__header {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 10px;
}

.cluster-hover-info__icon {
  display: inline-flex;
  flex: 0 0 auto;
}

.cluster-hover-info__header strong {
  min-width: 0;
  font-size: 14px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.cluster-hover-info__rows {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 6px 14px;
  margin: 0;
  padding-top: 10px;
  border-top: 1px solid var(--border);
}

.cluster-hover-info__rows > div {
  display: contents;
}

.cluster-hover-info__rows dt {
  color: var(--text-soft);
  font-size: 13px;
}

.cluster-hover-info__rows dd {
  min-width: 0;
  margin: 0;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}

.cluster-hover-info__note {
  margin: 0;
  color: var(--text-soft);
  font-size: 13px;
}

.cluster-hover-info__card.is-above {
  animation-name: cluster-hover-info-in-above;
}

@keyframes cluster-hover-info-in {
  from { opacity: 0; transform: translateY(-4px); }
}

@keyframes cluster-hover-info-in-above {
  from { opacity: 0; transform: translateY(4px); }
}

@media (prefers-reduced-motion: reduce) {
  .cluster-hover-info,
  .cluster-hover-info__card {
    transition: none;
    animation: none;
  }

  .cluster-hover-info:hover,
  .cluster-hover-info[aria-expanded='true'] {
    transform: none;
  }
}
</style>
