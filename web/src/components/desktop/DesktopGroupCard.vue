<script setup lang="ts">
import { ChevronDown, MoreHorizontal } from '@lucide/vue'
import type { DesktopGroup } from '@/types/api'
import { useI18n } from '@/i18n'
import type { desktopGroupCells } from '@/lib/desktopGroups'
defineProps<{ group: DesktopGroup; count: number; dropping: boolean; busy: boolean; cells: ReturnType<typeof desktopGroupCells>; dropCell?: number }>()
const emit = defineEmits<{ toggle: []; menu: []; drag: [event: PointerEvent]; nudge: [direction: 'left' | 'right' | 'up' | 'down'] }>()
const i18n = useI18n()
function keydown(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey)) return
  const direction = ({ ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' } as const)[event.key as 'ArrowLeft']
  if (direction) { event.preventDefault(); emit('nudge', direction) }
}
</script>

<template>
  <section class="desktop-group" :class="{ 'desktop-group--collapsed': group.collapsed, 'desktop-group--drop': dropping }"
    :data-group-id="group.id" :aria-label="group.name" @contextmenu.prevent.stop="emit('menu')" @pointerdown.stop>
    <header class="desktop-group__header" tabindex="0" :aria-label="i18n.t('desktop.groupMove', { name: group.name })"
      @keydown="keydown" @pointerdown="($event.target as Element).closest('button') ? undefined : emit('drag', $event)">
      <button type="button" class="desktop-group__toggle" :aria-expanded="!group.collapsed"
        :aria-label="i18n.t(group.collapsed ? 'desktop.groupExpand' : 'desktop.groupCollapse', { name: group.name })"
        :disabled="busy" @click="emit('toggle')"><ChevronDown :size="18" /></button>
      <strong :title="group.name">{{ group.name }}</strong><span class="desktop-group__count">{{ count }}</span>
      <button type="button" class="desktop-group__menu" :aria-label="i18n.t('desktop.groupManage', { name: group.name })"
        :disabled="busy" @click="emit('menu')"><MoreHorizontal :size="18" /></button>
    </header>
    <span v-for="cell in cells" :key="cell.index" class="desktop-group__cell"
      :class="{ 'desktop-group__cell--empty': !cell.key, 'desktop-group__cell--target': dropping && dropCell === cell.index }"
      :data-group-cell="cell.index" :data-cell-empty="!cell.key || undefined" aria-hidden="true"
      :style="{ left: `${cell.left}px`, top: `${cell.top}px`, width: `${cell.width}px`, height: `${cell.height}px` }" />
    <p v-if="!count && !group.collapsed" class="desktop-group__empty">{{ i18n.t('desktop.groupEmpty') }}</p>
    <span v-if="dropping" class="desktop-group__drop-label">{{ i18n.t('desktop.groupDrop', { name: group.name }) }}</span>
  </section>
</template>

<style scoped>
.desktop-group { position: absolute; box-sizing: border-box; border: 1px solid var(--desktop-glass-border); border-radius: var(--radius-lg); background: var(--desktop-glass-strong); color: var(--text); transition: transform 180ms cubic-bezier(.22,1,.36,1), height 200ms cubic-bezier(.22,1,.36,1), border-color 120ms; }
.desktop-group__header { height: 48px; display: flex; align-items: center; gap: 8px; padding: 0 10px; cursor: grab; touch-action: none; }
.desktop-group__cell { position: absolute; box-sizing: border-box; border: 1px dashed transparent; border-radius: var(--radius); pointer-events: none; }
.desktop-group:hover .desktop-group__cell--empty, .desktop-group--drop .desktop-group__cell--empty { border-color: var(--desktop-glass-border); }
.desktop-group .desktop-group__cell--target { border: 2px solid var(--brand); background: var(--brand-soft); }
.desktop-group__header strong { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
.desktop-group__header button { flex: 0 0 28px; width: 28px; height: 32px; display: grid; place-items: center; border: 0; border-radius: var(--radius-sm); background: transparent; color: inherit; cursor: pointer; }
.desktop-group__header button:hover { background: var(--interaction-hover); }
.desktop-group__header:focus-visible, .desktop-group__header button:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
.desktop-group__count { font-size: 13px; color: var(--text-soft); }
.desktop-group__toggle svg { transition: transform 180ms ease; }
.desktop-group--collapsed .desktop-group__toggle svg { transform: rotate(-90deg); }
.desktop-group--drop { border-color: var(--brand); box-shadow: 0 0 0 2px var(--brand-soft); }
.desktop-group__empty { margin: 24px 16px; text-align: center; font-size: 14px; color: var(--text-soft); }
.desktop-group__drop-label { position: absolute; bottom: 8px; left: 12px; right: 12px; padding: 4px 8px; border-radius: var(--radius-sm); background: var(--brand-action); color: var(--on-brand); font-size: 13px; text-align: center; pointer-events: none; z-index: 3; }
.desktop-group--dragging { transition: none; z-index: 22; }
@media (prefers-reduced-motion: reduce) { .desktop-group, .desktop-group__toggle svg { transition: none; } }
</style>
