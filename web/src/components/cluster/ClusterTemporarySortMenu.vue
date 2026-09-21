<script lang="ts">
export interface ClusterTemporarySortOption {
  value: 'custom' | 'cpu' | 'memory' | 'disk' | 'traffic'
  label: string
}
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import {
  Check,
  ChevronDown,
  Gauge,
  GripVertical,
  HardDrive,
  MemoryStick,
  Network,
} from '@lucide/vue'
import type { ClusterHostTemporarySortKey } from '@/lib/clusterHostTemporarySort'

const props = defineProps<{
  modelValue: ClusterHostTemporarySortKey
  options: ClusterTemporarySortOption[]
  label: string
  prefix: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: ClusterHostTemporarySortKey] }>()

const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const open = ref(false)
const menuId = `cluster-temporary-sort-${useId()}`

const sortIcons = {
  custom: GripVertical,
  cpu: Gauge,
  memory: MemoryStick,
  disk: HardDrive,
  traffic: Network,
} satisfies Record<ClusterHostTemporarySortKey, typeof Gauge>

const selected = computed(() => props.options.find((option) => option.value === props.modelValue)!)

function optionButtons(): HTMLButtonElement[] {
  return Array.from(root.value?.querySelectorAll<HTMLButtonElement>('.cluster-temporary-sort-menu__option') || [])
}

function focusSelected(): void {
  const buttons = optionButtons()
  const selectedButton = buttons.find((button) => button.dataset.value === props.modelValue)
  ;(selectedButton || buttons[0])?.focus()
}

function show(): void {
  if (open.value) return
  open.value = true
  void nextTick(focusSelected)
}

function close(restoreFocus = false): void {
  if (!open.value) return
  open.value = false
  if (restoreFocus) void nextTick(() => trigger.value?.focus())
}

function toggle(): void {
  if (open.value) close()
  else show()
}

function choose(value: ClusterHostTemporarySortKey): void {
  emit('update:modelValue', value)
  close(true)
}

function moveFocus(direction: number): void {
  const buttons = optionButtons()
  if (!buttons.length) return
  const current = buttons.indexOf(document.activeElement as HTMLButtonElement)
  buttons[(current + direction + buttons.length) % buttons.length]?.focus()
}

function onTriggerKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    show()
    void nextTick(() => {
      focusSelected()
      if (event.key === 'ArrowUp') moveFocus(-1)
    })
    return
  }
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    toggle()
  } else if (event.key === 'Escape') {
    close()
  }
}

function onMenuKeydown(event: KeyboardEvent): void {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    moveFocus(1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    moveFocus(-1)
  } else if (event.key === 'Home') {
    event.preventDefault()
    optionButtons()[0]?.focus()
  } else if (event.key === 'End') {
    event.preventDefault()
    optionButtons().at(-1)?.focus()
  } else if (event.key === 'Escape') {
    event.preventDefault()
    close(true)
  } else if (event.key === 'Tab') {
    close()
  }
}

function closeOnOutsidePointer(event: PointerEvent): void {
  if (open.value && event.target instanceof Node && !root.value?.contains(event.target)) close()
}

onMounted(() => document.addEventListener('pointerdown', closeOnOutsidePointer))
onBeforeUnmount(() => document.removeEventListener('pointerdown', closeOnOutsidePointer))
</script>

<template>
  <div ref="root" class="cluster-temporary-sort-menu" :class="{ 'is-open': open }">
    <button
      ref="trigger"
      class="cluster-temporary-sort-menu__trigger"
      type="button"
      aria-haspopup="listbox"
      :aria-expanded="open"
      :aria-controls="menuId"
      :aria-label="`${label}：${selected.label}`"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <span class="cluster-temporary-sort-menu__prefix">{{ prefix }}</span>
      <strong>{{ selected.label }}</strong>
      <ChevronDown :size="15" aria-hidden="true" />
    </button>

    <Transition name="cluster-temporary-sort-popover">
      <div
        v-if="open"
        :id="menuId"
        class="cluster-temporary-sort-menu__options"
        role="listbox"
        :aria-label="label"
        @keydown="onMenuKeydown"
      >
        <button
          v-for="option in options"
          :key="option.value"
          class="cluster-temporary-sort-menu__option"
          type="button"
          role="option"
          :data-value="option.value"
          :aria-selected="option.value === modelValue"
          @click="choose(option.value)"
        >
          <component :is="sortIcons[option.value]" :size="16" aria-hidden="true" />
          <span>{{ option.label }}</span>
          <Check v-if="option.value === modelValue" :size="16" aria-hidden="true" />
        </button>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.cluster-temporary-sort-menu {
  position: relative;
  min-width: 0;
  flex: 1;
}

.cluster-temporary-sort-menu__trigger {
  display: grid;
  width: 100%;
  min-width: 220px;
  min-height: 38px;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  color: var(--text);
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: calc(var(--radius-sm) - 1px) 0 0 calc(var(--radius-sm) - 1px);
  cursor: pointer;
  font: inherit;
}

.cluster-temporary-sort-menu__trigger:hover,
.cluster-temporary-sort-menu.is-open .cluster-temporary-sort-menu__trigger {
  background: var(--interaction-hover-subtle);
}

.cluster-temporary-sort-menu__trigger:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: -3px;
}

.cluster-temporary-sort-menu__trigger strong {
  min-width: 0;
  overflow: hidden;
  font-size: .875rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-temporary-sort-menu__prefix {
  color: var(--muted);
  font-size: .8125rem;
  font-weight: 600;
  white-space: nowrap;
}

.cluster-temporary-sort-menu__trigger > svg {
  color: var(--muted);
  transition: transform .14s ease;
}

.cluster-temporary-sort-menu.is-open .cluster-temporary-sort-menu__trigger > svg {
  transform: rotate(180deg);
}

.cluster-temporary-sort-menu__options {
  position: absolute;
  z-index: 50;
  top: calc(100% + 7px);
  left: 0;
  width: max(100%, 250px);
  padding: 6px;
  color: var(--text);
  background: var(--surface-raised);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius);
  box-shadow: var(--shadow-md);
}

.cluster-temporary-sort-menu__option {
  display: grid;
  width: 100%;
  min-height: 40px;
  grid-template-columns: 20px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 8px;
  padding: 7px 9px;
  color: var(--text-soft);
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: var(--radius-sm);
  cursor: pointer;
  font: inherit;
  font-size: .875rem;
}

.cluster-temporary-sort-menu__option > svg:first-child {
  color: var(--muted);
}

.cluster-temporary-sort-menu__option > svg:last-child {
  color: var(--brand);
}

.cluster-temporary-sort-menu__option:hover,
.cluster-temporary-sort-menu__option:focus-visible {
  color: var(--text);
  background: var(--interaction-hover);
}

.cluster-temporary-sort-menu__option:focus-visible {
  outline: 3px solid var(--brand);
  outline-offset: -3px;
}

.cluster-temporary-sort-menu__option[aria-selected='true'] {
  color: var(--brand-strong);
  background: var(--brand-soft);
  font-weight: 700;
}

.cluster-temporary-sort-menu__option[aria-selected='true'] > svg:first-child {
  color: var(--brand-strong);
}

.cluster-temporary-sort-popover-enter-active,
.cluster-temporary-sort-popover-leave-active {
  transition: opacity .12s ease, transform .12s ease;
  transform-origin: top left;
}

.cluster-temporary-sort-popover-enter-from,
.cluster-temporary-sort-popover-leave-to {
  opacity: 0;
  transform: translateY(-4px) scale(.985);
}

@media (max-width: 680px) {
  .cluster-temporary-sort-menu__trigger {
    min-width: 0;
  }

  .cluster-temporary-sort-menu__options {
    right: 0;
    width: auto;
  }
}
</style>
