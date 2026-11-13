<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, shallowReactive, shallowRef, watch } from 'vue'
import { trackKey, type Track } from '@/lib/music'

const props = defineProps<{ tracks: Track[]; scrollRoot?: HTMLElement }>()
const emit = defineEmits<{ rangeChange: [] }>()
const list = shallowRef<HTMLOListElement>()
const heights = shallowReactive(new Map<string, number>())
const viewport = shallowRef({ top: 0, height: 600, padding: 1600 })
const offsets = computed(() => {
  const values = [0]
  for (const track of props.tracks) values.push(values[values.length - 1]! + (heights.get(trackKey(track)) ?? 76))
  return values
})
function rowAt(position: number) {
  let low = 0, high = props.tracks.length
  while (low < high) {
    const middle = (low + high) >>> 1
    if (offsets.value[middle + 1]! <= position) low = middle + 1
    else high = middle
  }
  return low
}
const range = computed(() => {
  const count = props.tracks.length
  const start = Math.max(0, Math.min(count - 1, rowAt(Math.max(0, viewport.value.top - viewport.value.padding))))
  const end = Math.min(count, Math.max(start + 1, rowAt(viewport.value.top + viewport.value.height + viewport.value.padding) + 1))
  return { start, end }
})
const rows = computed(() => props.tracks.slice(range.value.start, range.value.end).map((track, index) => ({ track, index: range.value.start + index })))
const before = computed(() => offsets.value[range.value.start] ?? 0)
const after = computed(() => offsets.value[props.tracks.length]! - (offsets.value[range.value.end] ?? 0))
let frame = 0, destroyed = false, observer: ResizeObserver | undefined
let elements = new Map<Element, { key: string; index: number }>()
function refresh() {
  frame = 0
  const root = props.scrollRoot, target = list.value
  if (!root || !target) return
  const padding = Number.parseFloat(getComputedStyle(target).paddingTop) || 0
  const top = root.getBoundingClientRect().top + root.clientTop - target.getBoundingClientRect().top - target.clientTop - padding
  const next = { top: Math.max(0, top), height: root.clientHeight, padding: viewport.value.padding }
  if (next.top !== viewport.value.top || next.height !== viewport.value.height) viewport.value = next
}
function schedule() { if (!frame && !destroyed) frame = requestAnimationFrame(refresh) }
function scrolled() {
  if (frame) cancelAnimationFrame(frame)
  refresh()
}
async function observeRows() {
  await nextTick()
  if (destroyed || !list.value) return
  observer?.disconnect()
  elements = new Map()
  if (props.scrollRoot) observer?.observe(props.scrollRoot)
  observer?.observe(list.value)
  const children = [...list.value.children].filter(element => !element.hasAttribute('data-music-spacer'))
  children.forEach((element, index) => {
    const row = rows.value[index]
    if (!row) return
    elements.set(element, { key: trackKey(row.track), index: row.index })
    observer?.observe(element)
  })
  emit('rangeChange')
  schedule()
}
watch(() => props.scrollRoot, (root, previous) => {
  previous?.removeEventListener('scroll', scrolled)
  root?.addEventListener('scroll', scrolled, { passive: true })
  schedule()
}, { immediate: true })
watch(() => props.tracks.map(trackKey), keys => {
  const available = new Set(keys)
  for (const key of heights.keys()) if (!available.has(key)) heights.delete(key)
}, { immediate: true })
watch([list, rows], () => {
  if (!observer && typeof ResizeObserver !== 'undefined') observer = new ResizeObserver(entries => {
    const previousOffsets = offsets.value
    const changes: [string, number][] = []
    let adjustment = 0
    for (const entry of entries) {
      const row = elements.get(entry.target)
      if (!row) continue
      const height = entry.borderBoxSize?.[0]?.blockSize ?? entry.target.getBoundingClientRect().height
      const difference = height - (heights.get(row.key) ?? 76)
      if (height > 0 && Math.abs(difference) > .5) {
        changes.push([row.key, height])
        if (previousOffsets[row.index + 1]! <= viewport.value.top) adjustment += difference
      }
    }
    for (const [key, height] of changes) heights.set(key, height)
    if (adjustment && props.scrollRoot) props.scrollRoot.scrollTop += adjustment
    schedule()
  })
  void observeRows()
}, { flush: 'post' })
onBeforeUnmount(() => {
  destroyed = true
  props.scrollRoot?.removeEventListener('scroll', scrolled)
  observer?.disconnect()
  if (frame) cancelAnimationFrame(frame)
})
</script>

<template>
  <ol ref="list">
    <li v-if="before > 0" data-music-spacer role="presentation" aria-hidden="true" :style="{ blockSize: before + 'px' }" />
    <slot v-for="row in rows" :key="trackKey(row.track)" :track="row.track" :index="row.index" />
    <li v-if="after > 0" data-music-spacer role="presentation" aria-hidden="true" :style="{ blockSize: after + 'px' }" />
  </ol>
</template>

<style scoped>
ol { overflow-anchor:none; }
[data-music-spacer] { list-style:none; margin:0; padding:0; border:0; pointer-events:none; overflow-anchor:none; }
</style>
