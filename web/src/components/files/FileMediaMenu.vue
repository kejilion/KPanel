<script setup lang="ts">
import { inject, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { ChevronDown, Clapperboard, Images, Music2 } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { focusFirstContextMenuItem, moveContextMenuFocus, placeContextMenu, showContextMenuKeyboardFocus, showContextMenuPointerFocus } from '@/lib/contextMenu'

const emit = defineEmits<{ gallery: []; music: []; cinema: [] }>()
const i18n = useI18n(), id = useId()
const trigger = ref<HTMLButtonElement>(), menu = ref<HTMLElement>(), open = ref(false)
const position = ref({ left: '0px', top: '0px' })
const visible = inject(desktopWindowVisibleKey, ref(true))
function phrase(text: string) { phraseCatalogVersion.value; return translatePhrase(text) }
function close(restoreFocus = false) { open.value = false; if (restoreFocus) trigger.value?.focus() }
function place() {
  if (!open.value || !trigger.value || !menu.value) return
  const anchor = trigger.value.getBoundingClientRect()
  const point = placeContextMenu(menu.value, { x: anchor.left, y: anchor.bottom + 6 }, trigger.value)
  position.value = { left: `${point.x}px`, top: `${point.y}px` }
}
async function toggle(event: MouseEvent | KeyboardEvent) {
  if (open.value) { close(true); return }
  open.value = true
  await nextTick()
  place()
  if (menu.value) focusFirstContextMenuItem(menu.value, event instanceof KeyboardEvent || event.detail === 0 ? 'keyboard' : 'pointer')
}
function select(target: 'gallery' | 'music' | 'cinema') {
  close(true)
  if (target === 'gallery') emit('gallery')
  else if (target === 'music') emit('music')
  else emit('cinema')
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true) }
  else if (event.key === 'Tab') close(true)
  else if (menu.value) { showContextMenuKeyboardFocus(menu.value); moveContextMenuFocus(menu.value, event) }
}
function outside(event: Event) {
  const target = event.target
  if (open.value && target instanceof Node && !trigger.value?.contains(target) && !menu.value?.contains(target)) close()
}
watch(visible, value => { if (!value) close() })
onMounted(() => {
  document.addEventListener('pointerdown', outside)
  document.addEventListener('focusin', outside)
  window.addEventListener('resize', place)
  window.addEventListener('scroll', place, true)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', outside)
  document.removeEventListener('focusin', outside)
  window.removeEventListener('resize', place)
  window.removeEventListener('scroll', place, true)
})
</script>

<template>
  <button ref="trigger" class="button button--secondary button--small" type="button" :aria-label="phrase('媒体')" aria-haspopup="menu" :aria-expanded="open" :aria-controls="open ? id : undefined" @click="toggle" @keydown.down.prevent="toggle">
    <Images :size="15" /><span class="file-command-bar__label">{{ phrase('媒体') }}</span><ChevronDown class="file-command-bar__label" :size="13" />
  </button>
  <Teleport to="body">
    <div v-if="open" :id="id" ref="menu" class="file-context-menu file-media-menu k-context-menu" role="menu" :aria-label="phrase('媒体')" :style="position" @keydown="keydown" @pointermove="showContextMenuPointerFocus">
      <button type="button" role="menuitem" @click="select('gallery')"><Images :size="17" />{{ phrase('图库') }}</button>
      <button type="button" role="menuitem" @click="select('music')"><Music2 :size="17" />{{ i18n.t('route.music') }}</button>
      <button type="button" role="menuitem" @click="select('cinema')"><Clapperboard :size="17" />{{ i18n.t('route.cinema') }}</button>
    </div>
  </Teleport>
</template>

<style scoped>
.file-media-menu { position:fixed; z-index:90; display:grid; box-sizing:border-box; width:176px; max-width:var(--context-menu-max-width,calc(100vw - 16px)); max-height:var(--context-menu-max-height,calc(100dvh - 16px)); overflow-y:auto; padding:6px; }
.file-media-menu button { display:flex; align-items:center; gap:10px; min-height:40px; padding:8px 10px; border:0; border-radius:var(--radius-sm); color:var(--text); background:transparent; text-align:start; font:inherit; font-size:14px; cursor:pointer; }
</style>
