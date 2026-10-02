<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { Check, ChevronDown, ChevronRight, CircleAlert, ExternalLink, RefreshCw, Search, Server } from '@lucide/vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { contextMenuBounds, placeContextMenu } from '@/lib/contextMenu'
import type { HostSwitcherStatus } from '@/lib/hostSwitcher'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import type { ClusterHost } from '@/types/api'

/**
 * The one host picker used by the file manager, the gallery and history
 * monitoring: a "current host" button and a searchable, keyboard-navigable
 * list of cluster hosts. The list is teleported to <body> so no scrolling or
 * clipping ancestor (a desktop window, a split pane) can cut it. The page owns
 * the host inventory and decides what choosing a host means; this component
 * only presents and reports.
 */

const props = withDefaults(defineProps<{
  /** Hosts in display order; the picker filters them by the search text. */
  hosts: readonly ClusterHost[]
  activeId: string
  /** Name shown on the button for the current host. */
  label: string
  loading?: boolean
  error?: boolean
  /** Status line, tone and action for each host, already translated. */
  statusOf: (host: ClusterHost) => HostSwitcherStatus
}>(), {
  loading: false,
  error: false,
})

const emit = defineEmits<{
  /** The list just opened: load the inventory if it is not there yet. */
  open: []
  refresh: []
  select: [host: ClusterHost]
}>()

// The list lives outside #app, where the page-wide phrase observer does not
// reach, so every visible string is translated explicitly.
function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const trigger = ref<HTMLButtonElement>()
const menu = ref<HTMLElement>()
const searchInput = ref<HTMLInputElement>()
const isOpen = ref(false)
const search = ref('')
const menuId = `host-switcher-${useId()}`
const position = ref({ left: '0px', top: '0px' })

const filteredHosts = computed(() => {
  const needle = search.value.trim().toLocaleLowerCase()
  return props.hosts.filter((host) => !needle ||
    `${host.name} ${host.origin || ''} ${host.lastSnapshot?.telemetry?.hostname || ''} ${host.isLocal ? phrase('本机') : ''}`.toLocaleLowerCase().includes(needle))
})

const rows = computed(() => filteredHosts.value.map((host) => {
  const status = props.statusOf(host)
  const identity = detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)
  return {
    host,
    name: host.isLocal ? phrase('本机') : host.name,
    status,
    action: status.action ?? 'select',
    active: host.id === props.activeId,
    identity,
  }
}))

function close(restoreFocus = false): void {
  isOpen.value = false
  if (restoreFocus) void nextTick(() => trigger.value?.focus())
}

function toggle(): void {
  if (isOpen.value) {
    close()
    return
  }
  search.value = ''
  isOpen.value = true
  emit('open')
  void nextTick(reposition)
}

/** Open from the keyboard and put the caret in the search field, ready to type or arrow down. */
function openFromKeyboard(): void {
  if (!isOpen.value) toggle()
  void nextTick(() => searchInput.value?.focus())
}

function reposition(): void {
  const list = menu.value
  const button = trigger.value
  if (!isOpen.value || !list || !button) return
  const anchor = button.getBoundingClientRect()
  const bounds = contextMenuBounds(button)
  const below = Math.max(0, bounds.bottom - anchor.bottom - 14)
  const above = Math.max(0, anchor.top - bounds.top - 14)
  const openAbove = below < 240 && above > below
  list.style.setProperty('--host-menu-height', `${openAbove ? above : below}px`)
  const height = Math.min(list.offsetHeight, openAbove ? above : below, 440)
  const y = openAbove ? anchor.top - 6 - height : anchor.bottom + 6
  const placement = placeContextMenu(list, { x: anchor.left, y }, button)
  position.value = { left: `${placement.x}px`, top: `${placement.y}px` }
}

function onMenuKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close(true)
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  if (event.target === searchInput.value && ['Home', 'End'].includes(event.key)) return
  const options = Array.from(menu.value?.querySelectorAll<HTMLButtonElement>('[data-host-id]') || [])
  if (!options.length) return
  const current = options.indexOf(document.activeElement as HTMLButtonElement)
  let index: number
  if (event.key === 'Home') index = 0
  else if (event.key === 'End') index = options.length - 1
  else if (event.key === 'ArrowDown') index = (current + 1) % options.length
  else index = current < 0 ? options.length - 1 : (current - 1 + options.length) % options.length
  event.preventDefault()
  options[index]?.focus({ preventScroll: true })
  options[index]?.scrollIntoView?.({ block: 'nearest' })
}

function contains(target: EventTarget | null): boolean {
  return target instanceof Node && Boolean(trigger.value?.contains(target) || menu.value?.contains(target))
}

function onFocusout(event: FocusEvent): void {
  if (event.relatedTarget instanceof Node && !contains(event.relatedTarget)) close()
}

/** Pointer down closes before the click lands elsewhere; click covers synthetic and keyboard clicks. */
function onOutside(event: Event): void {
  if (isOpen.value && !contains(event.target)) close()
}

function onViewportChange(): void {
  reposition()
}

function onScroll(event: Event): void {
  // Scrolling the list itself must not move it.
  if (menu.value?.contains(event.target as Node)) return
  reposition()
}

watch([isOpen, filteredHosts, () => props.loading, () => props.error], async () => {
  await nextTick()
  reposition()
}, { flush: 'post' })

onMounted(() => {
  window.addEventListener('pointerdown', onOutside)
  window.addEventListener('click', onOutside)
  window.addEventListener('resize', onViewportChange)
  document.addEventListener('scroll', onScroll, true)
  window.visualViewport?.addEventListener?.('resize', onViewportChange)
  window.visualViewport?.addEventListener?.('scroll', onViewportChange)
})

onBeforeUnmount(() => {
  window.removeEventListener('pointerdown', onOutside)
  window.removeEventListener('click', onOutside)
  window.removeEventListener('resize', onViewportChange)
  document.removeEventListener('scroll', onScroll, true)
  window.visualViewport?.removeEventListener?.('resize', onViewportChange)
  window.visualViewport?.removeEventListener?.('scroll', onViewportChange)
})

defineExpose({ close, isOpen })
</script>

<template>
  <div class="host-switcher" data-host-switcher @focusout="onFocusout">
    <button
      ref="trigger"
      class="host-switcher__trigger"
      type="button"
      aria-haspopup="dialog"
      :aria-controls="menuId"
      :aria-expanded="isOpen"
      :title="phrase('切换主机')"
      @click.stop="toggle"
      @keydown.down.prevent="openFromKeyboard"
      @keydown.up.prevent="openFromKeyboard"
    >
      <Server class="host-switcher__glyph" :size="15" aria-hidden="true" />
      <span class="host-switcher__caption">{{ phrase('当前主机') }}</span>
      <strong>{{ label }}</strong>
      <ChevronDown class="host-switcher__chevron" :size="15" aria-hidden="true" />
    </button>
    <Teleport to="body">
      <div
        v-if="isOpen"
        :id="menuId"
        ref="menu"
        class="host-switcher__menu"
        :style="position"
        role="dialog"
        :aria-label="phrase('切换主机')"
        @click.stop
        @keydown="onMenuKeydown"
        @focusout="onFocusout"
        @contextmenu.stop.prevent
      >
        <div class="host-switcher__bar">
          <label class="host-switcher__search">
            <Search :size="15" aria-hidden="true" />
            <input ref="searchInput" v-model="search" type="search" :placeholder="phrase('搜索主机')" :aria-label="phrase('搜索主机')" />
          </label>
          <button
            class="host-switcher__refresh"
            type="button"
            :title="phrase('刷新主机列表')"
            :aria-label="phrase('刷新主机列表')"
            :disabled="loading"
            @click="emit('refresh')"
          >
            <RefreshCw :size="15" :class="{ spinning: loading }" aria-hidden="true" />
          </button>
        </div>
        <div class="host-switcher__list" :aria-busy="loading">
          <!-- A refresh keeps the current list; only the first read shows a placeholder. -->
          <div v-if="loading && !hosts.length" class="host-switcher__message" role="status" aria-live="polite">
            <RefreshCw :size="15" class="spinning" aria-hidden="true" />
            <span>{{ phrase('正在读取主机列表…') }}</span>
          </div>
          <div v-else-if="error && !hosts.length" class="host-switcher__message host-switcher__message--error" role="alert">
            <CircleAlert :size="15" aria-hidden="true" />
            <span>{{ phrase('无法读取集群主机') }}</span>
          </div>
          <template v-else-if="rows.length">
            <button
              v-for="row in rows"
              :key="row.host.id"
              class="host-switcher__item"
              :class="{ 'is-active': row.active, 'is-manage': row.action === 'manage' }"
              type="button"
              :aria-pressed="row.active"
              :title="`${row.name} · ${row.status.label}`"
              :data-host-id="row.host.id"
              @click="emit('select', row.host)"
            >
              <OperatingSystemIcon
                class="host-switcher__os"
                :distro="row.identity.key"
                :label="row.identity.label"
                :show-tooltip="false"
              />
              <span class="host-switcher__text">
                <strong>{{ row.name }}</strong>
                <small>
                  <i v-if="row.status.tone" class="host-switcher__dot" :class="`is-${row.status.tone}`" aria-hidden="true" />
                  <span>{{ row.status.label }}</span>
                </small>
              </span>
              <Check v-if="row.active" class="host-switcher__mark" :size="16" aria-hidden="true" />
              <ExternalLink v-else-if="row.action === 'open'" class="host-switcher__hint" :size="14" aria-hidden="true" />
              <ChevronRight v-else-if="row.action === 'manage'" class="host-switcher__hint" :size="15" aria-hidden="true" />
              <span v-else aria-hidden="true" />
            </button>
          </template>
          <div v-else class="host-switcher__message" role="status">
            <span>{{ search.trim() ? phrase('没有匹配的主机') : phrase('暂未发现集群主机') }}</span>
          </div>
        </div>
        <button
          v-if="error"
          class="host-switcher__retry"
          type="button"
          @click="emit('refresh')"
        >
          <CircleAlert :size="14" aria-hidden="true" />
          {{ phrase('主机列表刷新失败，点击重试') }}
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.host-switcher {
  position: relative;
  min-width: 0;
  max-width: 100%;
  flex: 0 0 auto;
}

/* ---- Button ---- */

.host-switcher__trigger {
  display: inline-flex;
  max-width: min(100%, 280px);
  min-height: 36px;
  align-items: center;
  gap: 6px;
  padding: 0 8px 0 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: border-color .16s ease, background-color .16s ease;
}

.host-switcher__trigger:hover,
.host-switcher__trigger[aria-expanded='true'] {
  border-color: color-mix(in srgb, var(--brand) 45%, var(--border));
}

.host-switcher__trigger:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: 2px;
}

.host-switcher__glyph,
.host-switcher__chevron {
  flex: 0 0 auto;
  color: var(--muted);
}

.host-switcher__chevron {
  margin-left: 2px;
  transition: transform .16s ease;
}

.host-switcher__trigger[aria-expanded='true'] .host-switcher__chevron {
  transform: rotate(180deg);
}

.host-switcher__caption {
  color: var(--muted);
  font-size: 13px;
  white-space: nowrap;
}

.host-switcher__trigger > strong {
  min-width: 0;
  overflow: hidden;
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---- Popover ---- */

.host-switcher__menu {
  position: fixed;
  z-index: 90;
  display: flex;
  flex-direction: column;
  gap: 6px;
  width: min(320px, calc(100vw - 24px), var(--context-menu-max-width, 320px));
  max-height: min(440px, var(--host-menu-height, 440px), var(--context-menu-max-height, calc(100dvh - 16px)));
  padding: 6px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-raised, var(--surface));
  box-shadow: var(--shadow-md, var(--shadow-sm));
  animation: host-switcher-in .14s ease-out both;
}

.host-switcher__bar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
}

.host-switcher__search {
  display: flex;
  min-width: 0;
  flex: 1 1 auto;
  align-items: center;
  gap: 8px;
  height: 36px;
  padding: 0 10px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  color: var(--muted);
  background: var(--surface-subtle);
  transition: border-color .16s ease, background-color .16s ease;
}

.host-switcher__search:focus-within {
  border-color: color-mix(in srgb, var(--brand) 50%, transparent);
  background: var(--surface);
}

.host-switcher__search > svg {
  flex-shrink: 0;
}

.host-switcher__search input {
  width: 100%;
  min-width: 0;
  padding: 0;
  border: 0;
  outline: none;
  color: var(--text);
  background: transparent;
  font: inherit;
  font-size: 14px;
}

.host-switcher__refresh {
  display: grid;
  flex: 0 0 36px;
  width: 36px;
  height: 36px;
  place-items: center;
  padding: 0;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--muted);
  background: transparent;
  cursor: pointer;
}

.host-switcher__refresh:hover:not(:disabled) {
  color: var(--text);
  background: var(--interaction-hover);
}

.host-switcher__refresh:disabled {
  cursor: default;
  opacity: .6;
}

.host-switcher__list {
  display: grid;
  min-height: 0;
  align-content: start;
  gap: 2px;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.host-switcher__item {
  display: grid;
  width: 100%;
  min-height: 48px;
  grid-template-columns: 30px minmax(0, 1fr) 16px;
  align-items: center;
  gap: 10px;
  padding: 7px 8px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--text);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: background-color .14s ease;
}

.host-switcher__item:hover {
  background: var(--interaction-hover);
}

.host-switcher__item.is-active {
  background: color-mix(in srgb, var(--brand) 11%, transparent);
}

.host-switcher__item:focus-visible,
.host-switcher__refresh:focus-visible,
.host-switcher__retry:focus-visible {
  outline: 2px solid var(--brand);
  outline-offset: -2px;
}

.host-switcher__item :deep(.host-switcher__os) {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  box-shadow: none;
}

.host-switcher__item :deep(.host-switcher__os svg) {
  width: 17px;
  height: 17px;
}

.host-switcher__text {
  display: grid;
  min-width: 0;
  gap: 1px;
}

.host-switcher__text strong {
  overflow: hidden;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.host-switcher__text small {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.4;
}

.host-switcher__text small > span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.host-switcher__item.is-manage .host-switcher__text strong {
  color: var(--text-soft);
}

.host-switcher__dot {
  width: 6px;
  height: 6px;
  flex: 0 0 auto;
  border-radius: 50%;
  background: var(--muted);
}

.host-switcher__dot.is-online {
  background: var(--success);
}

.host-switcher__dot.is-warning {
  background: var(--warning);
}

.host-switcher__dot.is-offline {
  background: var(--danger);
}

.host-switcher__dot.is-neutral {
  background: color-mix(in srgb, var(--muted) 70%, transparent);
}

.host-switcher__mark {
  justify-self: end;
  color: var(--brand);
}

.host-switcher__hint {
  justify-self: end;
  color: var(--muted);
}

.host-switcher__message {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 72px;
  padding: 12px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.5;
  text-align: center;
}

.host-switcher__message--error {
  color: var(--danger);
}

.host-switcher__retry {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  min-height: 36px;
  padding: 0 10px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--danger);
  background: var(--danger-soft, transparent);
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  text-align: left;
}

@keyframes host-switcher-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: none; }
}

@media (prefers-reduced-motion: reduce) {
  .host-switcher__menu,
  .host-switcher__chevron,
  .host-switcher__item {
    animation: none;
    transition: none;
  }
}
</style>
