<script setup lang="ts">
import { computed, inject, nextTick, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight } from '@lucide/vue'
import { useI18n } from '@/i18n'
import {
  officeImageSource, slideBoxStyle, slideFontRatio, slideMetrics, textAlign, thumbnailBars, type OfficeLocation,
} from '@/lib/officeDocument'
import type { OfficeItem, OfficeSection } from '@/types/api'
import OfficeTextBlock from './OfficeTextBlock.vue'
import { focusOfficeTarget, officeEditingKey } from './officeEditing'

const props = defineProps<{ sections: OfficeSection[]; locate: (id?: string) => OfficeLocation | undefined; hint: string }>()
const sectionIndex = defineModel<number>('sectionIndex', { required: true })
const { t } = useI18n()
const editing = inject(officeEditingKey)!
const rail = ref<HTMLElement>(), stage = ref<HTMLElement>()
const section = computed(() => props.sections[sectionIndex.value]!)
const metrics = computed(() => slideMetrics(section.value))
const total = computed(() => props.sections.length)

function ratio(s: OfficeSection) { const m = slideMetrics(s); return `${m.width} / ${m.height}` }
function title(s: OfficeSection) { return s.items.find(item => item.kind === 'text' && item.text.trim())?.text.trim() ?? '' }
function textStyle(item: OfficeItem, s: OfficeSection) {
  const m = slideMetrics(s)
  return { ...slideBoxStyle(item, m), '--office-font': slideFontRatio(item, m), fontWeight: item.bold ? 700 : undefined,
    fontStyle: item.italic ? 'italic' : undefined, textAlign: textAlign(item.align) }
}
function label(id?: string) {
  const location = props.locate(id)?.label
  return location?.kind === 'slideText' ? t('office.loc.slideText', location) : t('office.content')
}
function go(index: number) {
  if (index < 0 || index >= total.value || index === sectionIndex.value) return
  editing.commit()
  sectionIndex.value = index
}
watch(sectionIndex, () => {
  editing.select(undefined)
  void nextTick(() => rail.value?.querySelector('[aria-current="true"]')?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' }))
})
function deckKeydown(event: KeyboardEvent) {
  const target = event.target as HTMLElement
  if (event.isComposing || event.altKey || event.ctrlKey || event.metaKey || target.closest('textarea, input')) return
  const inRail = Boolean(target.closest('.office-rail'))
  const steps: Record<string, number> = { PageUp: -1, PageDown: 1, ArrowLeft: -1, ArrowRight: 1, ArrowUp: inRail ? -1 : 0, ArrowDown: inRail ? 1 : 0 }
  let next: number | undefined
  if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = total.value - 1
  else if (steps[event.key]) next = sectionIndex.value + steps[event.key]!
  if (next === undefined || next < 0 || next >= total.value) return
  event.preventDefault()
  go(next)
  if (inRail) void nextTick(() => rail.value?.querySelector<HTMLElement>('[aria-current="true"]')?.focus())
}
async function reveal(location: OfficeLocation) {
  await nextTick()
  focusOfficeTarget(stage.value, location.item.id!)
}
defineExpose({ reveal })
</script>

<template>
  <div class="office-view office-view--pptx" @keydown="deckKeydown">
    <div class="office-deck">
      <nav ref="rail" class="office-rail" :aria-label="t('office.slides')">
        <button
          v-for="(slide, index) in sections"
          :key="index"
          type="button"
          class="office-thumb"
          :aria-current="index === sectionIndex ? 'true' : undefined"
          :aria-label="t('office.slideLabel', { n: index + 1, title: title(slide) || t('office.untitled') })"
          @click="go(index)"
        >
          <span class="office-thumb__number" aria-hidden="true">{{ index + 1 }}</span>
          <span class="office-thumb__slide" :style="{ aspectRatio: ratio(slide) }" aria-hidden="true">
            <template v-for="(item, i) in slide.items" :key="item.id || i">
              <img v-if="item.kind === 'image' && officeImageSource(item)" :src="officeImageSource(item)" alt="" :style="slideBoxStyle(item, slideMetrics(slide))" />
              <span v-else-if="item.kind === 'text'" class="office-thumb__text" :class="{ 'is-bold': item.bold }" :style="textStyle(item, slide)">
                <i v-for="(width, line) in thumbnailBars(editing.value(item), item, slideMetrics(slide))" :key="line" :style="{ width: `${width}%` }" />
              </span>
            </template>
          </span>
        </button>
      </nav>
      <div ref="stage" class="office-stage">
        <div
          class="office-slide"
          role="group"
          :aria-label="t('office.slideOf', { n: sectionIndex + 1, total })"
          :style="{ aspectRatio: ratio(section), '--office-ratio': metrics.width / metrics.height }"
        >
          <template v-for="(item, index) in section.items" :key="item.id || index">
            <img v-if="item.kind === 'image' && officeImageSource(item)" class="office-slide__image" :src="officeImageSource(item)" :alt="t('office.image')" :style="slideBoxStyle(item, metrics)" />
            <OfficeTextBlock v-else-if="item.kind === 'text'" class="office-textbox" :item="item" :label="label(item.id)" :style="textStyle(item, section)" />
          </template>
          <p v-if="!section.items.length" class="office-placeholder">{{ t('office.emptySlide') }}</p>
        </div>
      </div>
    </div>
    <footer class="office-statusbar">
      <span class="office-statusbar__hint" aria-live="polite">{{ hint }}</span>
      <span class="office-pager">
        <button type="button" class="office-icon-button" :aria-label="t('office.previousSlide')" :title="t('office.previousSlide')" :disabled="sectionIndex === 0" @click="go(sectionIndex - 1)">
          <ChevronLeft :size="18" aria-hidden="true" />
        </button>
        <span>{{ t('office.slideOf', { n: sectionIndex + 1, total }) }}</span>
        <button type="button" class="office-icon-button" :aria-label="t('office.nextSlide')" :title="t('office.nextSlide')" :disabled="sectionIndex + 1 >= total" @click="go(sectionIndex + 1)">
          <ChevronRight :size="18" aria-hidden="true" />
        </button>
      </span>
    </footer>
  </div>
</template>
