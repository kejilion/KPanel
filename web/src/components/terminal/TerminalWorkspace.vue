<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef, watch } from 'vue'
import { Maximize2, Minimize2, Plus, X } from '@lucide/vue'
import { useTerminalWorkspace, type TerminalLayout } from '@/composables/useTerminalWorkspace'
import { useSplitPaneGeometry } from '@/composables/useSplitPaneGeometry'
import { translatePhrase } from '@/i18n/phrase'
import { useI18n } from '@/i18n'

interface WorkspaceSession {
  id: string
  label: string
  offset: number
  state: string
  stateLabel: string
  closing?: boolean
  closeFailed?: boolean
  retained?: boolean
  hostId?: string
}
const props = defineProps<{ sessions: WorkspaceSession[]; activeId: string }>()
const emit = defineEmits<{ activate: [id: string]; close: [id: string]; chooseHost: []; resize: [] }>()
const { locale } = useI18n()
const phrase = (value: string) => { locale.value; return translatePhrase(value) }
const ids = computed(() => props.sessions.map((item) => item.id))
const { layout, panes, activePane, maximized, paneCount, selectSession, setLayout } = useTerminalWorkspace(ids, toRef(props, 'activeId'))
const root = ref<HTMLElement>()
const compact = ref(false)
let observer: ResizeObserver | undefined
const singleVisible = computed(() => compact.value || maximized.value || layout.value === 'single')
const {
  grid, columnRatio, rowRatio, minimumColumnRatio, minimumRowRatio, hasColumns, hasRows, gridStyle,
  isVisible, cellStyle, measure, startDrag, moveDrag, endDrag, resizeWithKeyboard, resetRatio,
} = useSplitPaneGeometry({ layout, singleVisible, activePane, paneCount, onResize: () => emit('resize') })
function activate(index: number): void {
  if (index < 0 || index >= paneCount.value) return
  activePane.value = index
  const id = panes.value[index]
  if ((id ?? '') !== props.activeId) emit('activate', id ?? '')
}
function chooseHost(index: number): void {
  activate(index)
  emit('chooseHost')
}
function applyLayout(next: TerminalLayout): void {
  setLayout(next)
  const id = panes.value[activePane.value]
  if (id && id !== props.activeId) emit('activate', id)
}
function refresh(): void { void nextTick(() => emit('resize')) }
watch([layout, panes, activePane, maximized, compact], refresh, { deep: true })

onMounted(() => {
  if (typeof ResizeObserver === 'undefined') return
  observer = new ResizeObserver(() => {
    if (!root.value || root.value.clientWidth <= 0 || root.value.clientHeight <= 0) return
    // Respect the real desktop-window size, including zoom and the commands panel.
    compact.value = root.value.clientWidth < 640 || root.value.clientHeight < 360
    measure()
  })
  if (root.value) observer.observe(root.value)
  if (grid.value) observer.observe(grid.value)
})
onBeforeUnmount(() => observer?.disconnect())
defineExpose({ selectSession, layout, setLayout: applyLayout, compact })
</script>

<template>
  <section ref="root" class="terminal-pane-workspace" :class="{ 'is-single-visible': singleVisible }">
    <div ref="grid" class="terminal-pane-grid" :style="gridStyle">
      <!-- Stable siblings: layout changes never remount a terminal or close its PTY. -->
      <section v-for="item in sessions" v-show="isVisible(panes.indexOf(item.id))" :key="item.id"
        class="terminal-pane" :class="{ 'is-active': item.id === activeId && panes[activePane] === item.id }"
        :style="cellStyle(panes.indexOf(item.id))" :data-session-id="item.id"
        :aria-label="item.label" @pointerdown="activate(panes.indexOf(item.id))" @focusin="activate(panes.indexOf(item.id))">
        <header v-if="layout !== 'single'" class="terminal-pane-header">
          <span class="terminal-pane-title" :title="item.label">{{ item.label }}</span>
          <small>{{ item.stateLabel }}</small>
          <button type="button" :aria-label="phrase(maximized ? '恢复分屏' : '最大化窗格')" :title="phrase(maximized ? '恢复分屏' : '最大化窗格')"
            @click="activate(panes.indexOf(item.id)); maximized = !maximized">
            <Minimize2 v-if="maximized" :size="15" /><Maximize2 v-else :size="15" />
          </button>
          <button type="button" :disabled="item.closing" :aria-label="`${phrase('关闭终端')} · ${item.label}`"
            :title="phrase('关闭终端')" @click="emit('close', item.id)"><X :size="15" /></button>
        </header>
        <slot :item="item" :visible="isVisible(panes.indexOf(item.id))" />
      </section>
      <template v-for="(id, index) in panes" :key="`empty-${index}`">
        <div v-if="!id && isVisible(index)" class="terminal-pane terminal-pane-empty" :class="{ 'is-active': activePane === index }" :style="cellStyle(index)">
          <button type="button" @click="chooseHost(index)"><Plus :size="18" />{{ phrase('选择主机或新建终端') }}</button>
          <small>{{ phrase('已有终端可从顶部标签切换到此窗格') }}</small>
        </div>
      </template>
      <div v-if="hasColumns" class="terminal-pane-divider is-column" role="separator" tabindex="0" aria-orientation="vertical"
        :aria-label="phrase('调整左右分屏比例')" :aria-valuenow="Math.round(columnRatio)" :aria-valuemin="Math.ceil(minimumColumnRatio)" :aria-valuemax="Math.floor(100 - minimumColumnRatio)"
        @pointerdown.prevent="startDrag('column', $event)" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="endDrag" @lostpointercapture="endDrag"
        @keydown="resizeWithKeyboard('column', $event)" @dblclick="resetRatio('column')" />
      <div v-if="hasRows" class="terminal-pane-divider is-row" role="separator" tabindex="0" aria-orientation="horizontal"
        :aria-label="phrase('调整上下分屏比例')" :aria-valuenow="Math.round(rowRatio)" :aria-valuemin="Math.ceil(minimumRowRatio)" :aria-valuemax="Math.floor(100 - minimumRowRatio)"
        @pointerdown.prevent="startDrag('row', $event)" @pointermove="moveDrag" @pointerup="endDrag" @pointercancel="endDrag" @lostpointercapture="endDrag"
        @keydown="resizeWithKeyboard('row', $event)" @dblclick="resetRatio('row')" />
    </div>
  </section>
</template>

<style scoped>
.terminal-pane-workspace { display:grid; grid-template-rows:minmax(0,1fr); min-width:0; min-height:0; overflow:hidden; }
.terminal-pane-grid { display:grid; min-width:0; min-height:0; overflow:hidden; }
.terminal-pane { display:flex; flex-direction:column; min-width:0; min-height:0; overflow:hidden; position:relative; }
.terminal-pane.is-active { box-shadow:inset 0 0 0 1px var(--brand); }
.terminal-pane-header { display:flex; flex:0 0 auto; align-items:center; gap:8px; min-height:34px; padding:3px 8px; border-bottom:1px solid var(--terminal-shell-border); background:var(--terminal-shell-panel); color:var(--terminal-shell-text); font-size:14px; }
.terminal-pane.is-active .terminal-pane-header { border-top:2px solid var(--brand); padding-top:1px; }
.terminal-pane-title { min-width:0; flex:1; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.terminal-pane-header small { color:var(--terminal-shell-muted); font-size:12px; }
.terminal-pane-header button { display:grid; place-items:center; flex:0 0 28px; width:28px; height:28px; padding:0; border:0; border-radius:var(--radius-sm); color:inherit; background:transparent; cursor:pointer; }
.terminal-pane-header button:hover { background:color-mix(in srgb,var(--brand) 15%,transparent); }
.terminal-pane-header button:disabled { cursor:wait; opacity:.6; }
.terminal-pane :deep(.host-terminal) { flex:1; min-width:0; min-height:0; border:0; border-radius:0; box-shadow:none; }
.terminal-pane-empty { align-items:center; justify-content:center; gap:12px; padding:16px; text-align:center; color:var(--terminal-shell-muted); }
.terminal-pane-empty button { display:flex; align-items:center; justify-content:center; flex-wrap:wrap; gap:6px; min-height:36px; padding:6px 10px; border:1px solid var(--terminal-shell-border); border-radius:var(--radius-sm); background:var(--terminal-shell-panel); color:var(--terminal-shell-text); font:inherit; font-size:14px; cursor:pointer; }
.terminal-pane-empty small { font-size:13px; }
.terminal-pane-divider { background:var(--terminal-shell-panel); touch-action:none; position:relative; z-index:1; }
.terminal-pane-divider::after { content:''; position:absolute; background:var(--terminal-shell-border); }
.terminal-pane-divider.is-column { grid-column:2; grid-row:1 / -1; cursor:col-resize; }
.terminal-pane-divider.is-column::after { width:2px; top:0; bottom:0; left:3px; }
.terminal-pane-divider.is-row { grid-column:1 / -1; grid-row:2; cursor:row-resize; }
.terminal-pane-divider.is-row::after { height:2px; left:0; right:0; top:3px; }
.terminal-pane-divider:hover::after,.terminal-pane-divider:focus-visible::after { background:var(--brand); }
button:focus-visible,select:focus-visible,.terminal-pane-divider:focus-visible { outline:2px solid var(--brand); outline-offset:-2px; }
</style>
