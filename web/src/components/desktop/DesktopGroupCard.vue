<script setup lang="ts">
import { ChevronDown, MoreHorizontal } from '@lucide/vue'
import { nextTick, ref, useId, watch } from 'vue'
import type { DesktopGroup } from '@/types/api'
import { useI18n } from '@/i18n'
import type { desktopGroupCells } from '@/lib/desktopGroups'
const props = defineProps<{ group: DesktopGroup; count: number; dropping: boolean; busy: boolean; cells: ReturnType<typeof desktopGroupCells>; dropCell?: number; rename?: (name: string) => Promise<boolean>; error?: string }>()
const emit = defineEmits<{ toggle: []; dissolve: []; drag: [event: PointerEvent]; nudge: [direction: 'left' | 'right' | 'up' | 'down'] }>()
const i18n = useI18n()
const root = ref<HTMLElement>()
const nameButton = ref<HTMLButtonElement>()
const nameInput = ref<HTMLInputElement>()
const menuButton = ref<HTMLButtonElement>()
const menu = ref<HTMLElement>()
const menuOpen = ref(false)
const menuStyle = ref<Record<string, string>>({})
const editing = ref(false)
const draft = ref('')
const submitting = ref(false)
const renameFailed = ref(false)
const menuId = useId()
const composing = ref(false)

function closeMenu(restoreFocus = false) {
  menuOpen.value = false
  if (restoreFocus) void nextTick(() => menuButton.value?.focus())
}
function showMenu() {
  if (props.busy || editing.value) return
  menuOpen.value = true
  void nextTick(() => {
    const anchor = menuButton.value?.getBoundingClientRect()
    const popup = menu.value?.getBoundingClientRect()
    if (anchor && popup) menuStyle.value = {
      left: `${Math.max(8, Math.min(anchor.right - popup.width, window.innerWidth - popup.width - 8))}px`,
      top: `${Math.max(8, Math.min(anchor.bottom + 4, window.innerHeight - popup.height - 8))}px`,
    }
    menu.value?.querySelector<HTMLButtonElement>('button')?.focus()
  })
}
watch(menuOpen, (open, _, cleanup) => {
  if (!open) return
  const outside = (event: PointerEvent) => {
    if (!root.value?.contains(event.target as Node) && !menu.value?.contains(event.target as Node)) closeMenu()
  }
  const reposition = () => closeMenu()
  document.addEventListener('pointerdown', outside, true)
  window.addEventListener('resize', reposition)
  window.addEventListener('scroll', reposition, true)
  cleanup(() => {
    document.removeEventListener('pointerdown', outside, true)
    window.removeEventListener('resize', reposition)
    window.removeEventListener('scroll', reposition, true)
  })
})
function menuKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); closeMenu(true); return }
  const buttons = [...menu.value!.querySelectorAll<HTMLButtonElement>('button')]
  const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    let next = (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = buttons.length - 1
    buttons[next]?.focus()
  }
}
function startRename() {
  if (props.busy || !props.rename) return
  closeMenu()
  draft.value = props.group.name
  renameFailed.value = false
  editing.value = true
  void nextTick(() => { nameInput.value?.focus(); nameInput.value?.select() })
}
function finishRename(restoreFocus: boolean) {
  editing.value = false
  if (restoreFocus) void nextTick(() => nameButton.value?.focus())
}
async function saveName(restoreFocus = false) {
  if (!editing.value || composing.value || submitting.value || props.busy || !props.rename) return
  const name = draft.value.trim()
  if (!name || name === props.group.name) { finishRename(restoreFocus); return }
  submitting.value = true
  renameFailed.value = false
  try {
    if (await props.rename(name)) finishRename(restoreFocus)
    else renameFailed.value = true
  } finally {
    submitting.value = false
    if (renameFailed.value) void nextTick(() => nameInput.value?.focus())
  }
}
function nameKeydown(event: KeyboardEvent) {
  // Enter confirms an IME candidate before it can submit a name.
  if (event.isComposing || composing.value || event.keyCode === 229) return
  if (event.key === 'Enter') { event.preventDefault(); void saveName(true) }
  if (event.key === 'Escape' && !submitting.value && !props.busy) { event.preventDefault(); finishRename(true) }
}
function finishComposition() {
  composing.value = false
  if (document.activeElement !== nameInput.value) void nextTick(() => saveName())
}
function contextMenu(event: MouseEvent) {
  if ((event.target as Element).closest('input')) { event.stopPropagation(); return }
  event.preventDefault(); event.stopPropagation(); showMenu()
}
function keydown(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey)) return
  const direction = ({ ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down' } as const)[event.key as 'ArrowLeft']
  if (direction) { event.preventDefault(); emit('nudge', direction) }
}
</script>

<template>
  <section ref="root" class="desktop-group" :class="{ 'desktop-group--collapsed': group.collapsed, 'desktop-group--drop': dropping, 'desktop-group--editing': editing || menuOpen }"
    :data-group-id="group.id" :aria-label="group.name" @contextmenu="contextMenu" @pointerdown.stop>
    <header class="desktop-group__header" tabindex="0" :aria-label="i18n.t('desktop.groupMove', { name: group.name })"
      @keydown="keydown" @pointerdown="($event.target as Element).closest('button, input') ? undefined : emit('drag', $event)">
      <button type="button" class="desktop-group__toggle" :aria-expanded="!group.collapsed"
        :aria-label="i18n.t(group.collapsed ? 'desktop.groupExpand' : 'desktop.groupCollapse', { name: group.name })"
        :disabled="busy" @click="emit('toggle')"><ChevronDown :size="18" /></button>
      <input v-if="editing" ref="nameInput" v-model="draft" class="desktop-group__name-input" maxlength="48"
        :aria-label="i18n.t('desktop.groupName')" :readonly="busy || submitting" :aria-invalid="renameFailed || undefined"
        :aria-describedby="renameFailed && error ? `${menuId}-error` : undefined"
        @pointerdown.stop @keydown.stop="nameKeydown" @blur="saveName()"
        @compositionstart="composing = true" @compositionend="finishComposition" />
      <button v-else ref="nameButton" type="button" class="desktop-group__name" :title="group.name"
        :aria-label="`${i18n.t('desktop.groupRename')} · ${group.name}`" :disabled="busy" @click.stop="startRename"><strong>{{ group.name }}</strong></button>
      <span class="desktop-group__count">{{ count }}</span>
      <span v-if="group.collapsed && !editing" class="desktop-group__previews" aria-hidden="true" inert><slot name="preview" /></span>
      <button ref="menuButton" type="button" class="desktop-group__menu" :aria-label="i18n.t('desktop.groupManage', { name: group.name })"
        aria-haspopup="menu" :aria-expanded="menuOpen" :aria-controls="menuId"
        :disabled="busy || editing" @click="menuOpen ? closeMenu() : showMenu()" @keydown.down.prevent.stop="showMenu"><MoreHorizontal :size="18" /></button>
    </header>
    <Teleport to="body">
    <div v-if="menuOpen" :id="menuId" ref="menu" class="desktop-group__actions" :style="menuStyle" role="menu" :aria-label="i18n.t('desktop.groupManage', { name: group.name })"
      @pointerdown.stop @contextmenu.prevent.stop @keydown.stop="menuKeydown"
      @focusout="!root?.contains($event.relatedTarget as Node) && !menu?.contains($event.relatedTarget as Node) && closeMenu()">
      <button type="button" role="menuitem" :disabled="busy" @click="startRename">{{ i18n.t('desktop.groupRename') }}</button>
      <button type="button" role="menuitem" :disabled="busy" :title="i18n.t('desktop.groupHint')" @click="closeMenu(); emit('dissolve')">{{ i18n.t('desktop.groupDissolve') }}</button>
    </div>
    </Teleport>
    <p v-if="editing && renameFailed && error" :id="`${menuId}-error`" class="sr-only" role="alert">{{ error }}</p>
    <span v-for="cell in cells" :key="cell.index" class="desktop-group__cell"
      :class="{ 'desktop-group__cell--empty': !cell.key, 'desktop-group__cell--target': dropping && dropCell === cell.index }"
      :data-group-cell="cell.index" :data-cell-empty="!cell.key || undefined" aria-hidden="true"
      :style="{ left: `${cell.left}px`, top: `${cell.top}px`, width: `${cell.width}px`, height: `${cell.height}px` }" />
    <p v-if="!count && !group.collapsed" class="desktop-group__empty">{{ i18n.t('desktop.groupEmpty') }}</p>
    <span v-if="dropping" class="desktop-group__drop-label">{{ i18n.t('desktop.groupDrop', { name: group.name }) }}</span>
  </section>
</template>

<style scoped>
.desktop-group { position: absolute; box-sizing: border-box; container-type: inline-size; border: 1px solid var(--desktop-group-border); border-radius: var(--radius); background: var(--desktop-group-surface); color: var(--desktop-group-text); box-shadow: inset 0 1px 0 var(--desktop-group-hover), 0 6px 20px rgb(0 8 18 / 10%); transition: transform 180ms cubic-bezier(.22,1,.36,1), height 200ms cubic-bezier(.22,1,.36,1), border-color 120ms; }
.desktop-group__header { height: 48px; display: flex; align-items: center; gap: 6px; padding: 0 10px; cursor: grab; touch-action: none; }
.desktop-group__cell { position: absolute; box-sizing: border-box; border: 1px dashed transparent; border-radius: var(--radius); pointer-events: none; }
.desktop-group:hover .desktop-group__cell--empty, .desktop-group--drop .desktop-group__cell--empty { border-color: var(--desktop-group-border); }
.desktop-group .desktop-group__cell--target { border: 2px solid var(--desktop-group-focus); background: var(--desktop-group-hover); }
.desktop-group__header strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 600; }
.desktop-group__header button { flex: 0 0 28px; width: 28px; height: 32px; display: grid; place-items: center; border: 0; border-radius: var(--radius-sm); background: transparent; color: inherit; cursor: pointer; }
.desktop-group__header button:hover { background: var(--desktop-group-hover); }
.desktop-group__header:focus-visible, .desktop-group__header button:focus-visible { outline: 2px solid var(--desktop-group-focus); outline-offset: 2px; }
.desktop-group__count { flex: 0 0 auto; min-width: 20px; padding: 1px 5px; box-sizing: border-box; border-radius: 999px; font-size: 12px; line-height: 1.5; text-align: center; color: var(--desktop-group-muted); background: var(--desktop-group-hover); }
.desktop-group__menu { margin-left: auto; }
.desktop-group__header .desktop-group__name { display: block; flex: 0 1 auto; width: auto; min-width: 28px; max-width: 100%; padding: 0 4px; overflow: hidden; text-align: start; }
.desktop-group__name strong { display: block; }
.desktop-group__name-input { flex: 1 1 80px; width: 80px; min-width: 0; height: 32px; box-sizing: border-box; padding: 0 4px; border: 1px solid var(--desktop-group-focus); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font: inherit; font-size: 14px; outline: 2px solid var(--desktop-group-hover); }
.desktop-group__actions { position: fixed; z-index: 3400; display: grid; min-width: 120px; max-width: calc(100vw - 16px); padding: 4px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface-raised); color: var(--text); box-shadow: var(--shadow-sm); }
.desktop-group__actions button { border: 0; border-radius: var(--radius-sm); background: transparent; color: inherit; text-align: start; padding: 8px 12px; font: inherit; font-size: 14px; cursor: pointer; }
.desktop-group__actions button:hover, .desktop-group__actions button:focus-visible { background: var(--surface-subtle); outline: 2px solid var(--brand); outline-offset: -2px; }
.desktop-group__previews { display: flex; flex: 0 0 auto; gap: 6px; margin-left: auto; pointer-events: none; }
.desktop-group__previews + .desktop-group__menu { margin-left: 0; }
.desktop-group--collapsed .desktop-group__header { height: 54px; }
@container (max-width: 280px) { .desktop-group__previews :deep(> :nth-child(n+3)) { display: none; } }
@container (max-width: 240px) { .desktop-group__previews :deep(> :nth-child(n+2)) { display: none; } }
@container (max-width: 180px) { .desktop-group__previews { display: none; } .desktop-group__previews + .desktop-group__menu { margin-left: auto; } }
.desktop-group__toggle svg { transition: transform 180ms ease; }
.desktop-group--collapsed .desktop-group__toggle svg { transform: rotate(-90deg); }
.desktop-group--drop { border-color: var(--desktop-group-focus); box-shadow: 0 0 0 2px var(--desktop-group-hover); }
.desktop-group__empty { margin: 24px 16px; text-align: center; font-size: 14px; color: var(--desktop-group-muted); }
.desktop-group__drop-label { position: absolute; bottom: 8px; left: 12px; right: 12px; padding: 4px 8px; border-radius: var(--radius-sm); background: var(--brand-action); color: var(--on-brand); font-size: 13px; text-align: center; pointer-events: none; z-index: 3; }
.desktop-group--dragging { transition: none; z-index: 22; }
@media (prefers-reduced-motion: reduce) { .desktop-group, .desktop-group__toggle svg { transition: none; } }
</style>
