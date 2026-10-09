<script lang="ts">
export type ClusterHostMenuAction =
  | 'open-panel'
  | 'history'
  | 'terminal'
  | 'files'
  | 'refresh'
  | 'manage'
  | 'copy-address'
  | 'remove'

export interface ClusterHostMenuRequest {
  host: ClusterHost
  /** Viewport point to open at; keyboard or button triggers pass the trigger's corner. */
  x: number
  y: number
  /** Element the menu belongs to; bounds the menu inside a desktop window. */
  anchor: Element | null
  /** Button that opened the menu, so focus can return to it. */
  opener?: HTMLElement | null
  origin: ContextMenuFocusOrigin
  /** Address shown for the host; empty hides the copy item. */
  address: string
}

export interface ClusterHostMenuItem {
  action: ClusterHostMenuAction
  label: string
  disabled?: boolean
}

/** Menu entries per group; the shape depends only on what the host can do. */
export function clusterHostMenuGroups(host: ClusterHost, address: string): ClusterHostMenuItem[][] {
  const groups: ClusterHostMenuItem[][] = []
  if (host.kind !== 'light_node') {
    groups.push([{ action: 'open-panel', label: host.isLocal ? '当前面板' : '打开面板' }])
  }
  const tools: ClusterHostMenuItem[] = [{ action: 'history', label: '历史监控' }]
  if (host.terminalAvailable) tools.push({ action: 'terminal', label: '终端' })
  if (host.fileManagementAvailable === true) tools.push({ action: 'files', label: '文件' })
  groups.push(tools)
  const manage: ClusterHostMenuItem[] = [
    { action: 'refresh', label: '刷新', disabled: host.polling },
    { action: 'manage', label: '管理' },
  ]
  if (address) manage.push({ action: 'copy-address', label: '复制地址' })
  groups.push(manage)
  if (!host.isLocal) groups.push([{ action: 'remove', label: '移除主机' }])
  return groups
}
</script>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, type Component } from 'vue'
import { Activity, ArrowUpRight, Copy, FolderOpen, Pencil, RefreshCw, SquareTerminal, Trash2 } from '@lucide/vue'
import {
  type ContextMenuFocusOrigin,
  focusFirstContextMenuItem,
  moveContextMenuFocus,
  placeContextMenu,
  showContextMenuKeyboardFocus,
  showContextMenuPointerFocus,
} from '@/lib/contextMenu'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { ClusterHost } from '@/types/api'

const emit = defineEmits<{ select: [action: ClusterHostMenuAction, host: ClusterHost] }>()

const icons: Record<ClusterHostMenuAction, Component> = {
  'open-panel': ArrowUpRight,
  history: Activity,
  terminal: SquareTerminal,
  files: FolderOpen,
  refresh: RefreshCw,
  manage: Pencil,
  'copy-address': Copy,
  remove: Trash2,
}

const menu = ref<HTMLElement>()
const request = ref<ClusterHostMenuRequest>()
const left = ref(0)
const top = ref(0)

const groups = computed(() => request.value ? clusterHostMenuGroups(request.value.host, request.value.address) : [])

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

async function open(next: ClusterHostMenuRequest): Promise<void> {
  request.value = next
  left.value = next.x
  top.value = next.y
  await nextTick()
  if (!menu.value) return
  // A button-opened menu hangs from the button's right edge instead of spilling past it.
  const openerRight = next.opener?.isConnected ? next.opener.getBoundingClientRect().right : undefined
  const pointX = openerRight === undefined ? next.x : openerRight - menu.value.offsetWidth
  const placed = placeContextMenu(menu.value, { x: pointX, y: next.y }, next.anchor)
  left.value = placed.x
  top.value = placed.y
  focusFirstContextMenuItem(menu.value, next.origin)
}

function close(restoreFocus = false): void {
  const opener = request.value?.opener
  request.value = undefined
  if (restoreFocus && opener?.isConnected) void nextTick(() => opener.focus())
}

function choose(item: ClusterHostMenuItem): void {
  const host = request.value?.host
  if (!host || item.disabled) return
  close(item.action === 'copy-address' || item.action === 'refresh')
  emit('select', item.action, host)
}

function onMenuKeydown(event: KeyboardEvent): void {
  if (!menu.value) return
  showContextMenuKeyboardFocus(menu.value)
  if (moveContextMenuFocus(menu.value, event)) return
  // Escape is handled by the capturing document listener, which runs first.
  if (event.key === 'Tab') close()
}

function onDocumentPointer(event: PointerEvent): void {
  if (!request.value || menu.value?.contains(event.target as Node)) return
  // The opener button toggles through its own click handler.
  if (request.value.opener?.contains(event.target as Node)) return
  close()
}

function onDocumentEscape(event: KeyboardEvent): void {
  if (event.key !== 'Escape' || !request.value) return
  event.preventDefault()
  event.stopImmediatePropagation()
  close(true)
}

function onViewportChange(): void {
  close()
}

function onScroll(event: Event): void {
  if (menu.value?.contains(event.target as Node)) return
  close()
}

onMounted(() => {
  document.addEventListener('pointerdown', onDocumentPointer, true)
  document.addEventListener('keydown', onDocumentEscape, true)
  document.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('blur', onViewportChange)
  window.visualViewport?.addEventListener?.('resize', onViewportChange)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocumentPointer, true)
  document.removeEventListener('keydown', onDocumentEscape, true)
  document.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('blur', onViewportChange)
  window.visualViewport?.removeEventListener?.('resize', onViewportChange)
})

defineExpose({ open, close, isOpen: computed(() => Boolean(request.value)), hostId: computed(() => request.value?.host.id) })
</script>

<template>
  <Teleport to="body">
    <div
      v-if="request"
      ref="menu"
      class="cluster-host-menu k-context-menu"
      role="menu"
      :aria-label="`${request.host.name} · ${phrase('主机操作')}`"
      :style="{ left: `${left}px`, top: `${top}px` }"
      @click.stop
      @contextmenu.prevent
      @pointermove="showContextMenuPointerFocus"
      @keydown.stop="onMenuKeydown"
    >
      <strong class="cluster-host-menu__title">{{ request.host.name }}</strong>
      <template v-for="(group, index) in groups" :key="index">
        <hr v-if="index > 0" />
        <button
          v-for="item in group"
          :key="item.action"
          type="button"
          role="menuitem"
          :class="{ 'k-context-menu__item--danger cluster-host-menu__danger': item.action === 'remove' }"
          :disabled="item.disabled"
          @click="choose(item)"
        >
          <component :is="icons[item.action]" :size="15" aria-hidden="true" />
          <span>{{ phrase(item.label) }}</span>
        </button>
      </template>
    </div>
  </Teleport>
</template>

<style scoped>
.cluster-host-menu {
  position: fixed;
  z-index: 7000;
  display: grid;
  box-sizing: border-box;
  width: 200px;
  max-width: var(--context-menu-max-width, calc(100vw - 16px));
  max-height: var(--context-menu-max-height, calc(100dvh - 16px));
  overflow-y: auto;
  overscroll-behavior: contain;
  padding: 6px;
}

.cluster-host-menu__title {
  overflow: hidden;
  padding: 8px 10px 7px;
  color: var(--text-muted);
  font-size: .78rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cluster-host-menu button {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  padding: 8px 10px;
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--text);
  text-align: left;
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 14px;
}

.cluster-host-menu button.cluster-host-menu__danger {
  color: var(--danger);
}

.cluster-host-menu hr {
  width: 100%;
  margin: 4px 0;
  border: 0;
  border-top: 1px solid var(--border);
}
</style>
