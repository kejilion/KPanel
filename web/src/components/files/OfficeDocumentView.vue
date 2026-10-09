<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { ChevronLeft, ChevronRight, ImageOff } from '@lucide/vue'
import { useI18n } from '@/i18n'
import { documentTextStyle, officeImageSource, OFFICE_PAGE_ITEMS, type OfficeLocation } from '@/lib/officeDocument'
import type { OfficeSection } from '@/types/api'
import OfficeTextBlock from './OfficeTextBlock.vue'
import { focusOfficeTarget } from './officeEditing'

const props = defineProps<{ section: OfficeSection; locate: (id?: string) => OfficeLocation | undefined; hint: string }>()
const { t } = useI18n()
const desk = ref<HTMLElement>()
const page = ref(0)
const pages = computed(() => Math.max(1, Math.ceil(props.section.items.length / OFFICE_PAGE_ITEMS)))
const start = computed(() => page.value * OFFICE_PAGE_ITEMS)
const items = computed(() => props.section.items.slice(start.value, start.value + OFFICE_PAGE_ITEMS))
// Saving replaces the section object; keep the reader on the same page unless it no longer exists.
watch(pages, total => { if (page.value >= total) page.value = total - 1 })

function label(id?: string) {
  const location = props.locate(id)?.label
  if (location?.kind === 'tableCell') return t('office.loc.tableCell', location)
  if (location?.kind === 'paragraph') return t('office.loc.paragraph', location)
  return t('office.content')
}
function go(next: number) {
  page.value = Math.max(0, Math.min(pages.value - 1, next))
  desk.value?.scrollTo?.({ top: 0 })
}
async function reveal(location: OfficeLocation) {
  page.value = location.page
  await nextTick()
  focusOfficeTarget(desk.value, location.item.id!)
}
defineExpose({ reveal })
</script>

<template>
  <div class="office-view office-view--docx">
    <div ref="desk" class="office-desk">
      <article class="office-page">
        <template v-for="(item, index) in items" :key="item.id || `${page}:${index}`">
          <figure v-if="item.kind === 'image'" class="office-figure">
            <img v-if="officeImageSource(item)" :src="officeImageSource(item)" :alt="t('office.image')" decoding="async" />
            <span v-else class="office-figure__missing"><ImageOff :size="18" aria-hidden="true" />{{ t('office.imageUnavailable') }}</span>
          </figure>
          <table v-else-if="item.kind === 'table'" class="office-table">
            <tbody>
              <tr v-for="(row, r) in item.table" :key="r">
                <td v-for="(cell, c) in row" :key="cell.id || c">
                  <OfficeTextBlock :item="cell" :label="label(cell.id)" :style="documentTextStyle(cell)" />
                </td>
              </tr>
            </tbody>
          </table>
          <OfficeTextBlock v-else :item="item" :label="label(item.id)" class="office-paragraph" :style="documentTextStyle(item)" />
        </template>
        <p v-if="section.items.length === 0" class="office-placeholder">{{ t('office.empty') }}</p>
      </article>
    </div>
    <footer class="office-statusbar">
      <span class="office-statusbar__hint" aria-live="polite">{{ hint }}</span>
      <span v-if="pages > 1" class="office-pager">
        <button type="button" class="office-icon-button" :aria-label="t('office.previousPage')" :title="t('office.previousPage')" :disabled="page === 0" @click="go(page - 1)">
          <ChevronLeft :size="18" aria-hidden="true" />
        </button>
        <span>{{ t('office.pageOf', { page: page + 1, total: pages }) }}</span>
        <button type="button" class="office-icon-button" :aria-label="t('office.nextPage')" :title="t('office.nextPage')" :disabled="page + 1 >= pages" @click="go(page + 1)">
          <ChevronRight :size="18" aria-hidden="true" />
        </button>
      </span>
    </footer>
  </div>
</template>
