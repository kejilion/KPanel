<script setup lang="ts">
import { computed, ref, useId, watch } from 'vue'
import { Star } from '@lucide/vue'
import { cinemaSourceURL, type CinemaMetadata } from '@/lib/cinemaMetadata'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
const props = defineProps<{ metadata: CinemaMetadata }>()
const expanded = ref(false)
const descriptionId = useId()
const rating = computed(() => typeof props.metadata.rating === 'number' && props.metadata.rating > 0 && props.metadata.rating <= 10 ? props.metadata.rating.toFixed(1) : '')
const genres = computed(() => props.metadata.genres?.filter(Boolean).slice(0, 8) ?? [])
const cast = computed(() => props.metadata.cast?.filter(Boolean).slice(0, 8) ?? [])
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
watch(() => props.metadata, () => { expanded.value = false })
</script>
<template>
  <section class="cinema-synopsis" :aria-label="phrase('影片资料')">
    <p v-if="metadata.originalTitle && metadata.originalTitle !== metadata.title" class="cinema-synopsis__original">{{ metadata.originalTitle }}</p>
    <div v-if="metadata.year || rating || genres.length" class="cinema-synopsis__tags">
      <span v-if="metadata.year" class="cinema-synopsis__year">{{ metadata.year }}</span>
      <span v-if="rating" class="cinema-synopsis__rating"><Star :size="15" aria-hidden="true" /><strong>{{ rating }}</strong><span>/ 10</span></span>
      <span v-for="genre in genres" :key="genre" class="cinema-synopsis__genre">{{ genre }}</span>
    </div>
    <div v-if="metadata.overview" class="cinema-synopsis__overview">
      <p :id="descriptionId" :class="{ 'cinema-synopsis__clamped': !expanded && metadata.overview.length > 120 }">{{ metadata.overview }}</p>
      <button v-if="metadata.overview.length > 120" class="cinema-synopsis__expand" type="button" :aria-expanded="expanded" :aria-controls="descriptionId" @click="expanded = !expanded">{{ phrase(expanded ? '收起简介' : '展开简介') }}</button>
    </div>
    <dl v-if="cast.length" class="cinema-synopsis__cast"><dt>{{ phrase('主演') }}</dt><dd>{{ cast.join(' · ') }}</dd></dl>
    <a v-if="cinemaSourceURL(metadata)" class="cinema-synopsis__credit" :href="cinemaSourceURL(metadata)" target="_blank" rel="noopener noreferrer">{{ phrase('资料来源') }} · {{ metadata.provider === 'tmdb' ? 'TMDB' : metadata.provider === 'tvmaze' ? 'TVmaze' : 'Wikipedia' }}</a>
  </section>
</template>
<style scoped>
.cinema-synopsis { display: grid; gap: 16px; max-width: 70ch; margin-block: 18px 24px; min-width: 0; }
.cinema-synopsis__original { margin: 0; color: var(--text-soft); font-size: 14px; overflow-wrap: anywhere; }
.cinema-synopsis__tags { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; font-size: 14px; }
.cinema-synopsis__year { color: var(--text-soft); }
.cinema-synopsis__rating { display: inline-flex; align-items: center; gap: 5px; color: #f6d77a; }
.cinema-synopsis__rating > span { color: var(--text-soft); font-size: 13px; }
.cinema-synopsis__genre { border: 1px solid var(--border); padding: 3px 9px; border-radius: var(--radius-md); font-size: 13px; color: var(--text-soft); }
.cinema-synopsis__overview p { margin: 0; line-height: 1.8; font-size: 15px; color: var(--text); white-space: pre-line; overflow-wrap: anywhere; }
.cinema-synopsis__clamped { display: -webkit-box; -webkit-line-clamp: 5; -webkit-box-orient: vertical; overflow: hidden; }
.cinema-synopsis__expand { padding: 6px 0; background: transparent; border: 0; font: inherit; font-size: 14px; color: #c5e6ff; cursor: pointer; text-decoration: underline; text-underline-offset: 3px; }
.cinema-synopsis__cast { display: grid; grid-template-columns: auto 1fr; gap: 10px; margin: 0; font-size: 14px; line-height: 1.7; }
.cinema-synopsis__cast dt { color: var(--text-soft); }.cinema-synopsis__cast dd { margin: 0; overflow-wrap: anywhere; color: var(--text); }
.cinema-synopsis__credit { width: fit-content; font-size: 13px; color: #c5e6ff; text-underline-offset: 3px; }
</style>
