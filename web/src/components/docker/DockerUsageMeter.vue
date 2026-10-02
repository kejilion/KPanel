<script setup lang="ts">
import { computed } from 'vue'
import { clampPercent } from '@/lib/format'

const props = defineProps<{
  /** Raw percentage; the bar is clamped to 0-100 while `value` keeps the real figure. */
  percent: number
  value: string
  label: string
  detail?: string
  stale?: boolean
}>()

const width = computed(() => `${clampPercent(props.percent)}%`)
const warningPercent = 70
const dangerPercent = 90

// Colour is only a secondary cue: the figure itself is always printed.
const level = computed(() => {
  if (props.percent >= dangerPercent) return 'danger'
  if (props.percent >= warningPercent) return 'warning'
  return 'normal'
})
</script>

<template>
  <div class="usage-meter" :class="[`usage-meter--${level}`, { 'is-stale': stale }]">
    <div class="usage-meter__head">
      <strong>{{ value }}</strong>
      <small v-if="detail">{{ detail }}</small>
    </div>
    <div
      class="usage-meter__track"
      role="meter"
      aria-valuemin="0"
      aria-valuemax="100"
      :aria-label="label"
      :aria-valuenow="Math.round(clampPercent(percent))"
      :aria-valuetext="detail ? `${value}, ${detail}` : value"
    >
      <i :style="{ width }" />
    </div>
  </div>
</template>

<style scoped>
.usage-meter { display: grid; min-width: 0; gap: 5px; }
.usage-meter__head { display: flex; min-width: 0; align-items: baseline; justify-content: space-between; gap: 8px; font-variant-numeric: tabular-nums; }
.usage-meter__head strong { font-size: .875rem; font-weight: 600; }
.usage-meter__head small { overflow: hidden; color: var(--muted); font-size: .8125rem; text-overflow: ellipsis; white-space: nowrap; }
.usage-meter__track { height: 6px; overflow: hidden; border-radius: 999px; background: var(--line-soft); }
.usage-meter__track i { display: block; height: 100%; border-radius: inherit; background: var(--success); transition: width 360ms ease, background-color 200ms ease; }
.usage-meter--warning .usage-meter__track i { background: var(--amber); }
.usage-meter--danger .usage-meter__track i { background: var(--danger); }
.usage-meter.is-stale { opacity: .6; }
@media (prefers-reduced-motion: reduce) { .usage-meter__track i { transition: none; } }
</style>
