<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  nearestTimestamp,
  normalizeTrendChartWidth,
  svgClientXToViewBox,
  svgViewBoxXToClient,
  trendLegendLabel,
  uniqueSeriesTimes,
} from '@/lib/monitoringPresentation'

export interface TrendSeries {
  label: string
  color: string
  latestLabel?: string
  points: Array<{ at: string; value: number }>
}

interface NormalizedPoint {
  at: string
  time: number
  value: number
}

const props = withDefaults(defineProps<{
  series: TrendSeries[]
  formatter?: (value: number) => string
  zeroBased?: boolean
  maxValue?: number
}>(), {
  formatter: (value: number) => value.toFixed(1),
  zeroBased: true,
  maxValue: undefined,
})

const defaultWidth = 720
const height = 210
const padding = { top: 16, right: 12, bottom: 28, left: 64 }
const canvas = ref<HTMLDivElement>()
const width = ref(defaultWidth)
const hoveredTime = ref<number>()
const tooltipX = ref<number>()
const tooltipOnLeft = ref(false)
let observer: ResizeObserver | undefined

const normalizedSeries = computed(() => props.series.map((series) => ({
  ...series,
  points: series.points.reduce<NormalizedPoint[]>((result, point) => {
    const time = Date.parse(point.at)
    if (Number.isFinite(time) && Number.isFinite(point.value)) {
      result.push({ at: point.at, time, value: point.value })
    }
    return result
  }, []),
})))

const bounds = computed(() => {
  let minimumTime = Number.POSITIVE_INFINITY
  let maximumTime = Number.NEGATIVE_INFINITY
  let minimumValue = props.zeroBased ? 0 : Number.POSITIVE_INFINITY
  let maximumValue = Number.NEGATIVE_INFINITY
  for (const series of normalizedSeries.value) {
    for (const point of series.points) {
      minimumTime = Math.min(minimumTime, point.time)
      maximumTime = Math.max(maximumTime, point.time)
      minimumValue = Math.min(minimumValue, point.value)
      maximumValue = Math.max(maximumValue, point.value)
    }
  }
  if (Number.isFinite(props.maxValue)) maximumValue = Math.max(maximumValue, props.maxValue as number)
  const hasData = Number.isFinite(minimumTime) && Number.isFinite(maximumTime) && Number.isFinite(maximumValue)
  if (!hasData) {
    return { hasData: false, minimumTime: 0, maximumTime: 1, minimumValue: 0, maximumValue: 1 }
  }
  if (!Number.isFinite(minimumValue)) minimumValue = 0
  if (maximumValue <= minimumValue) maximumValue = minimumValue + 1
  return { hasData, minimumTime, maximumTime, minimumValue, maximumValue }
})

const yTicks = computed(() => Array.from({ length: 4 }, (_, index) => {
  const ratio = index / 3
  const value = bounds.value.maximumValue -
    ratio * (bounds.value.maximumValue - bounds.value.minimumValue)
  return { value, y: padding.top + ratio * plotHeight() }
}))

const hoveredPoints = computed(() => {
  if (hoveredTime.value === undefined) return []
  return normalizedSeries.value.flatMap((series) => {
    const point = nearestPoint(series.points, hoveredTime.value as number)
    return point ? [{ ...point, label: series.label, color: series.color }] : []
  })
})

const interactionTimes = computed(() => uniqueSeriesTimes(normalizedSeries.value))

const hoverX = computed(() => {
  return hoveredTime.value === undefined ? 0 : xFor(hoveredTime.value)
})

const tooltipTime = computed(() => hoveredTime.value ?? 0)

const tooltipStyle = computed(() => ({
  left: tooltipX.value === undefined
    ? `${(hoverX.value / width.value) * 100}%`
    : `${tooltipX.value}px`,
}))

function plotWidth(): number {
  return width.value - padding.left - padding.right
}

function plotHeight(): number {
  return height - padding.top - padding.bottom
}

function xFor(time: number): number {
  const span = Math.max(1, bounds.value.maximumTime - bounds.value.minimumTime)
  return padding.left + ((time - bounds.value.minimumTime) / span) * plotWidth()
}

function yFor(value: number): number {
  const span = Math.max(1, bounds.value.maximumValue - bounds.value.minimumValue)
  return padding.top + (1 - (value - bounds.value.minimumValue) / span) * plotHeight()
}

function linePath(points: NormalizedPoint[]): string {
  if (!bounds.value.hasData) return ''
  return points.map((point, index) =>
    `${index === 0 ? 'M' : 'L'}${xFor(point.time).toFixed(2)},${yFor(point.value).toFixed(2)}`,
  ).join(' ')
}

function nearestPoint(points: NormalizedPoint[], target: number): NormalizedPoint | undefined {
  if (!points.length) return undefined
  let low = 0
  let high = points.length - 1
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (points[middle]!.time < target) low = middle + 1
    else high = middle
  }
  const current = points[low]
  const previous = low > 0 ? points[low - 1] : undefined
  if (!current) return previous
  if (!previous) return current
  return Math.abs(previous.time - target) <= Math.abs(current.time - target) ? previous : current
}

function onPointerMove(event: PointerEvent): void {
  if (!bounds.value.hasData) return
  const svg = event.currentTarget as SVGSVGElement
  const svgX = svgClientXToViewBox(svg, event.clientX, event.clientY, width.value)
  if (svgX === undefined) return
  const clamped = Math.min(width.value - padding.right, Math.max(padding.left, svgX))
  const ratio = (clamped - padding.left) / plotWidth()
  const target = bounds.value.minimumTime + ratio * (bounds.value.maximumTime - bounds.value.minimumTime)
  const nearest = nearestTimestamp(interactionTimes.value, target)
  hoveredTime.value = nearest
  if (nearest === undefined) return

  const canvasRect = svg.parentElement?.getBoundingClientRect()
  const clientX = svgViewBoxXToClient(svg, xFor(nearest), width.value)
  if (!canvasRect?.width || clientX === undefined) {
    tooltipX.value = undefined
    tooltipOnLeft.value = hoverX.value > width.value / 2
    return
  }

  const localX = Math.min(canvasRect.width, Math.max(0, clientX - canvasRect.left))
  tooltipX.value = localX
  tooltipOnLeft.value = localX > canvasRect.width / 2
}

function clearHover(): void {
  hoveredTime.value = undefined
  tooltipX.value = undefined
  tooltipOnLeft.value = false
}

function lastValue(series: (typeof normalizedSeries.value)[number]): number {
  return series.points.at(-1)?.value ?? 0
}

function timeLabel(value: number, detailed = false): string {
  if (!Number.isFinite(value)) return '—'
  return new Intl.DateTimeFormat('zh-CN', detailed ? {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit',
  } : {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(new Date(value))
}

function updateWidth(): void {
  const measured = canvas.value?.clientWidth || 0
  width.value = normalizeTrendChartWidth(measured, defaultWidth)
}

onMounted(() => {
  updateWidth()
  if (canvas.value) {
    if (typeof ResizeObserver !== 'undefined') {
      observer = new ResizeObserver(updateWidth)
      observer.observe(canvas.value)
    }
  }
})

onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <div class="trend-chart">
    <div class="trend-chart__legend">
      <span v-for="item in normalizedSeries" :key="item.label">
        <i :style="{ backgroundColor: item.color }" />
        {{ item.label }}
        <strong>{{ trendLegendLabel(item.latestLabel, lastValue(item), formatter) }}</strong>
      </span>
    </div>
    <div v-if="bounds.hasData" ref="canvas" class="trend-chart__canvas">
      <svg
        :viewBox="`0 0 ${width} ${height}`"
        role="img"
        aria-label="资源历史趋势，移动鼠标查看准确刻度"
        @pointermove="onPointerMove"
        @pointerleave="clearHover"
      >
        <g v-for="tick in yTicks" :key="tick.y">
          <line
            :x1="padding.left"
            :x2="width - padding.right"
            :y1="tick.y"
            :y2="tick.y"
            class="trend-chart__grid"
          />
          <text :x="padding.left - 9" :y="tick.y + 4" text-anchor="end" class="trend-chart__tick">
            {{ formatter(tick.value) }}
          </text>
        </g>
        <path
          v-for="item in normalizedSeries"
          :key="item.label"
          :d="linePath(item.points)"
          :stroke="item.color"
          class="trend-chart__line"
        />
        <template v-if="hoveredPoints.length">
          <line
            :x1="hoverX"
            :x2="hoverX"
            :y1="padding.top"
            :y2="height - padding.bottom"
            class="trend-chart__cursor"
          />
          <circle
            v-for="point in hoveredPoints"
            :key="point.label"
            :cx="hoverX"
            :cy="yFor(point.value)"
            r="4"
            :stroke="point.color"
            class="trend-chart__point"
          />
        </template>
      </svg>
      <div
        v-if="hoveredPoints.length"
        class="trend-chart__tooltip"
        :class="{
          'is-left': tooltipOnLeft,
          'is-dense': hoveredPoints.length > 6,
        }"
        :style="tooltipStyle"
      >
        <time>{{ timeLabel(tooltipTime, true) }}</time>
        <span v-for="point in hoveredPoints" :key="point.label">
          <i :style="{ backgroundColor: point.color }" />
          {{ point.label }}
          <strong>{{ formatter(point.value) }}</strong>
        </span>
      </div>
      <div class="trend-chart__axis">
        <span>{{ timeLabel(bounds.minimumTime) }}</span>
        <span>{{ timeLabel(bounds.maximumTime) }}</span>
      </div>
    </div>
    <div v-else class="trend-chart__empty">等待采样数据</div>
  </div>
</template>

<style scoped>
.trend-chart { min-width: 0; }
.trend-chart__legend {
  display: flex; align-items: center; flex-wrap: wrap; gap: 8px 18px; min-height: 28px;
  color: var(--muted); font-size: .8rem;
}
.trend-chart__legend span { display: inline-flex; align-items: center; gap: 6px; }
.trend-chart__legend i, .trend-chart__tooltip i { width: 8px; height: 8px; border-radius: 999px; }
.trend-chart__legend strong { color: var(--text); font-size: .86rem; }
.trend-chart__canvas { position: relative; overflow: hidden; margin-top: 4px; }
.trend-chart svg { display: block; width: 100%; height: 184px; touch-action: pan-y; }
.trend-chart__grid { stroke: var(--border); stroke-width: 1; stroke-dasharray: 4 6; }
.trend-chart__tick { fill: var(--muted); font-size: 10.5px; }
.trend-chart__line {
  fill: none; stroke-width: 2.25; stroke-linecap: round; stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}
.trend-chart__cursor { stroke: var(--muted); stroke-width: 1; stroke-dasharray: 3 4; vector-effect: non-scaling-stroke; }
.trend-chart__point { fill: var(--surface); stroke-width: 2.5; vector-effect: non-scaling-stroke; }
.trend-chart__tooltip {
  position: absolute; z-index: 2; top: 12px; display: grid; min-width: 150px; gap: 7px;
  padding: 10px 11px; border: 1px solid var(--border); border-radius: 10px;
  background: var(--surface); box-shadow: var(--shadow-sm);
  pointer-events: none; transform: translateX(10px);
}
.trend-chart__tooltip.is-left { transform: translateX(calc(-100% - 10px)); }
.trend-chart__tooltip.is-dense {
  width: min(360px, calc(100% - 20px)); grid-template-columns: repeat(2, minmax(0, 1fr));
}
.trend-chart__tooltip.is-dense time { grid-column: 1 / -1; }
.trend-chart__tooltip.is-dense span { min-width: 0; white-space: nowrap; }
.trend-chart__tooltip time { color: var(--muted); font-size: .72rem; }
.trend-chart__tooltip span { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 7px; font-size: .76rem; }
.trend-chart__tooltip strong { color: var(--text); }
.trend-chart__axis { display: flex; justify-content: space-between; padding-left: 64px; color: var(--muted); font-size: .72rem; }
.trend-chart__empty { min-height: 212px; display: grid; place-items: center; color: var(--muted); font-size: .86rem; }
@media (max-width: 560px) {
  .trend-chart__tooltip.is-dense:not(.is-left) { left: 10px !important; transform: none; }
  .trend-chart__tooltip.is-dense.is-left { right: 10px; left: auto !important; transform: none; }
}
</style>
