<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { Check, ChevronDown, ChevronRight, CircleAlert, ExternalLink, RefreshCw, Search, Server } from '@lucide/vue'
import OperatingSystemIcon from '@/components/overview/OperatingSystemIcon.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { contextMenuBounds, placeContextMenu } from '@/lib/contextMenu'
import type { FileHostStatus } from '@/lib/fileHostStatus'
import { detectOperatingSystemIdentity } from '@/lib/operatingSystem'
import type { ClusterHost } from '@/types/api'

/**
 * Host picker shared by the file manager and the gallery: a "current host"
 * button and a searchable, keyboard-navigable list of cluster hosts. The list
 * is teleported to <body> so no scrolling or clipping ancestor can cut it.
 * The parent owns the host inventory and decides what choosing a host means;
 * this component only presents and reports.
 */

const props = withDefaults(defineProps<{
  /** Hosts in display order; the picker filters them by the search text. */
  hosts: readonly ClusterHost[]
  activeId: string
  /** Name shown on the button for the current host. */
  label: string
  loading?: boolean
  error?: boolean
  /** What each host can do and why, already translated. */
  statusOf: (host: ClusterHost) => FileHostStatus
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
const menuId = `file-host-switcher-${useId()}`
const position = ref({ left: '0px', top: '0px' })

const filteredHosts = computed(() => {
  const needle = search.value.trim().toLocaleLowerCase()
  return props.hosts.filter((host) => !needle ||
    `${host.name} ${host.origin || ''} ${host.lastSnapshot?.telemetry.hostname || ''} ${host.isLocal ? phrase('本机') : ''}`.toLocaleLowerCase().includes(needle))
})

const identityOf = (host: ClusterHost) => detectOperatingSystemIdentity(host.lastSnapshot?.telemetry)

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

function reposition(): void {
  const list = menu.value
  const button = trigger.value
  if (!isOpen.value || !list || !button) return
  const anchor = button.getBoundingClientRect()
  const bounds = contextMenuBounds(button)
  const below = Math.max(0, bounds.bottom - anchor.bottom - 14)
  const above = Math.max(0, anchor.top - bounds.top - 14)
  const openAbove = below < 240 && above > below
  list.style.setProperty('--file-host-menu-height', `${openAbove ? above : below}px`)
  const height = Math.min(list.offsetHeight, openAbove ? above : below, 480)
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
  const options = Array.from(menu.value?.querySelectorAll<HTMLButtonElement>('[data-file-host-id]') || [])
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

function onWindowClick(event: MouseEvent): void {
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
  window.addEventListener('click', onWindowClick)
  window.addEventListener('resize', onViewportChange)
  document.addEventListener('scroll', onScroll, true)
  window.visualViewport?.addEventListener?.('resize', onViewportChange)
  window.visualViewport?.addEventListener?.('scroll', onViewportChange)
})

onBeforeUnmount(() => {
  window.removeEventListener('click', onWindowClick)
  window.removeEventListener('resize', onViewportChange)
  document.removeEventListener('scroll', onScroll, true)
  window.visualViewport?.removeEventListener?.('resize', onViewportChange)
  window.visualViewport?.removeEventListener?.('scroll', onViewportChange)
})

defineExpose({ close, isOpen })
</script>

<template>
  <div class="file-host-switcher" data-file-host-switcher @focusout="onFocusout">
    <button
      ref="trigger"
      class="file-host-switcher__trigger"
      type="button"
      aria-haspopup="dialog"
      :aria-controls="menuId"
      :aria-expanded="isOpen"
      :title="phrase('切换主机')"
      @click.stop="toggle"
      @keydown.down.prevent="isOpen ? searchInput?.focus() : toggle()"
    >
      <Server :size="15" aria-hidden="true" />
      <span>{{ phrase('当前主机') }}</span>
      <strong>{{ label }}</strong>
      <ChevronDown :size="15" aria-hidden="true" />
    </button>
    <Teleport to="body">
      <div
        v-if="isOpen"
        :id="menuId"
        ref="menu"
        class="file-host-switcher__menu"
        :style="position"
        role="dialog"
        :aria-label="phrase('切换主机')"
        @click.stop
        @keydown="onMenuKeydown"
        @focusout="onFocusout"
        @contextmenu.stop.prevent
      >
        <header class="file-host-switcher__heading">
          <strong>{{ phrase('切换主机') }}</strong>
          <button class="icon-button" type="button" :title="phrase('刷新主机列表')" :aria-label="phrase('刷新主机列表')" :disabled="loading" @click="emit('refresh')">
            <RefreshCw :size="15" :class="{ spinning: loading }" aria-hidden="true" />
          </button>
        </header>
        <label class="file-host-switcher__search">
          <Search :size="15" aria-hidden="true" />
          <input ref="searchInput" v-model="search" type="search" :placeholder="phrase('搜索主机')" :aria-label="phrase('搜索主机')" />
        </label>
        <div class="file-host-switcher__list" :aria-busy="loading">
          <div v-if="loading" class="file-host-switcher__message" role="status" aria-live="polite">
            <RefreshCw :size="15" class="spinning" aria-hidden="true" />
            <span>{{ phrase('正在读取主机列表…') }}</span>
          </div>
          <div v-else-if="error && !hosts.length" class="file-host-switcher__message file-host-switcher__message--error" role="alert">
            <CircleAlert :size="15" aria-hidden="true" />
            <span>{{ phrase('无法读取集群主机') }}</span>
          </div>
          <template v-else-if="filteredHosts.length">
            <button
              v-for="host in filteredHosts"
              :key="host.id"
              class="file-host-switcher__item"
              :class="{
                'is-active': host.id === activeId,
                'is-manage': statusOf(host).action === 'manage',
              }"
              type="button"
              :aria-pressed="host.id === activeId"
              :title="`${host.isLocal ? phrase('本机') : host.name} · ${statusOf(host).label}`"
              :data-file-host-id="host.id"
              @click="emit('select', host)"
            >
              <OperatingSystemIcon
                class="file-host-switcher__os"
                :distro="identityOf(host).key"
                :label="identityOf(host).label"
              />
              <span>
                <strong>{{ host.isLocal ? phrase('本机') : host.name }}</strong>
                <small>{{ statusOf(host).label }}</small>
              </span>
              <Check v-if="host.id === activeId" :size="16" aria-hidden="true" />
              <ExternalLink v-else-if="statusOf(host).action === 'open'" :size="15" aria-hidden="true" />
              <ChevronRight v-else :size="15" aria-hidden="true" />
            </button>
          </template>
          <div v-else class="file-host-switcher__message" role="status">
            <span>{{ search.trim() ? phrase('没有匹配的主机') : phrase('暂未发现集群主机') }}</span>
          </div>
        </div>
        <button
          v-if="error"
          class="file-host-switcher__retry"
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
.file-host-switcher {
  position: relative;
  flex: 0 0 auto;
}

.file-host-switcher__trigger {
  display: inline-flex;
  max-width: 220px;
  min-height: 38px;
  align-items: center;
  gap: 7px;
  padding: 7px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--text);
  background: var(--surface);
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: border-color .16s ease, color .16s ease, background-color .16s ease;
}

.file-host-switcher__trigger:hover,
.file-host-switcher__trigger:focus-visible,
.file-host-switcher__trigger[aria-expanded='true'] {
  border-color: color-mix(in srgb, var(--brand) 55%, var(--border));
  color: var(--brand);
  outline: none;
}

.file-host-switcher__trigger > span {
  color: var(--muted);
  font-size: 13px;
  white-space: nowrap;
}

.file-host-switcher__trigger > strong {
  min-width: 0;
  overflow: hidden;
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__menu {
  position: fixed;
  z-index: 90;
  display: flex;
  flex-direction: column;
  width: min(340px, calc(100vw - 32px), var(--context-menu-max-width, 340px));
  max-height: min(480px, var(--file-host-menu-height, 480px), var(--context-menu-max-height, calc(100dvh - 16px)));
  overflow: hidden;
  border: 1px solid var(--border-strong, var(--border));
  border-radius: var(--radius);
  background: var(--surface-raised, var(--surface));
  box-shadow: var(--shadow-md, var(--shadow-sm));
}

.file-host-switcher__heading {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 12px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
}

.file-host-switcher__search {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  margin: 10px 12px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  color: var(--muted);
  background: var(--surface);
}

.file-host-switcher__search > svg { flex-shrink: 0; }
.file-host-switcher__search input {
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
.file-host-switcher__search:focus-within { outline: 2px solid var(--brand); outline-offset: 1px; }
.file-host-switcher__list {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.file-host-switcher__item {
  display: grid;
  width: 100%;
  min-height: 54px;
  grid-template-columns: 28px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 9px;
  padding: 9px 12px;
  border: 0;
  color: var(--text);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
  transition: background-color .16s ease, color .16s ease;
}

.file-host-switcher__item :deep(.file-host-switcher__os) {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-sm);
  box-shadow: none;
}

.file-host-switcher__item :deep(.file-host-switcher__os svg) {
  width: 17px;
  height: 17px;
}

.file-host-switcher__item:hover,
.file-host-switcher__item:focus-visible {
  background: var(--interaction-hover);
  outline: none;
}

.file-host-switcher__item.is-active {
  color: var(--brand-strong, var(--brand));
  background: color-mix(in srgb, var(--brand) 9%, var(--surface-raised, var(--surface)));
}

.file-host-switcher__item.is-manage {
  color: var(--muted);
}

.file-host-switcher__item > span {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.file-host-switcher__item strong {
  overflow: hidden;
  color: var(--text);
  font-size: 14px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__item small {
  overflow: hidden;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-host-switcher__item > svg:last-child {
  justify-self: end;
}

.file-host-switcher__message,
.file-host-switcher__retry {
  flex: 0 0 auto;
  display: flex;
  min-height: 48px;
  align-items: center;
  gap: 8px;
  padding: 10px 12px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.file-host-switcher__message--error {
  color: var(--danger);
}

.file-host-switcher__retry {
  width: 100%;
  border: 0;
  border-top: 1px solid var(--border);
  color: var(--danger);
  background: transparent;
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.file-host-switcher__retry:hover,
.file-host-switcher__retry:focus-visible {
  background: var(--interaction-hover);
  outline: none;
}

@media (max-width: 720px) {
  .file-host-switcher__trigger {
    max-width: min(100%, 260px);
  }
}
</style>
