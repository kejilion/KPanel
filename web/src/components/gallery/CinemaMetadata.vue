<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { api, ApiError } from '@/lib/api'
import { siThemoviedatabase } from 'simple-icons'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { automaticCinemaMatch, cinemaMetadataImage, cinemaMetadataKey, cinemaSearchTitle, cinemaSourceURL, readCinemaMetadataCache, saveCinemaMetadataCache, type CinemaMetadata, type CinemaProvider } from '@/lib/cinemaMetadata'
import type { GalleryItem } from '@/lib/gallery'

const props = defineProps<{ items: GalleryItem[]; host: string; selected?: GalleryItem; scanning: boolean; visible: boolean }>()
const emit = defineEmits<{ update: [metadata: Map<string, CinemaMetadata>] }>()
const i18n = useI18n()
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const language = computed(() => i18n.locale.value)
const open = ref(false)
const enabled = ref(false)
const provider = ref<CinemaProvider>('wikipedia')
const kind = ref<'movie' | 'tv'>('movie')
const credential = ref('')
const title = ref('')
const searchInput = ref<HTMLInputElement>()
const results = ref<CinemaMetadata[]>([])
const cache = ref(readCinemaMetadataCache())
const busy = ref(false)
const error = ref('')
const status = ref('')
const target = ref<GalleryItem>()
const selectedMetadata = computed(() => target.value ? cache.value.get(key(target.value)) : undefined)
let controller: AbortController | undefined
function key(item: GalleryItem) { return cinemaMetadataKey(props.host, language.value, item) }
function publish() { emit('update', new Map(props.items.flatMap(item => { const value = cache.value.get(key(item)); return value ? [[item.entry.path, value] as const] : [] }))) }
function persist() {
  cache.value = new Map([...cache.value].slice(-200))
  if (!saveCinemaMetadataCache(cache.value)) error.value = phrase('浏览器未保存刮削结果，请检查存储空间。')
  publish()
}
function stop() { controller?.abort(); controller = undefined; busy.value = false }
function errorText(cause: unknown) {
  if (cause instanceof ApiError) {
    if (cause.code === 'cinema_source_credentials') return phrase('数据源拒绝请求，请检查 TMDB 密钥或稍后重试。')
    if (cause.code === 'cinema_metadata_busy') return phrase('数据源繁忙，请稍后重试。')
  }
  return phrase('刮削失败，请检查面板网络后重试。')
}
function query(itemTitle: string, id?: number) {
  return { provider: provider.value, kind: kind.value, title: itemTitle, language: language.value,
    ...(provider.value === 'tmdb' ? { credential: credential.value.trim() } : {}), ...(id ? { id } : {}) }
}
async function search() {
  if (!title.value.trim() || !target.value) return
  stop(); const run = new AbortController(); controller = run
  busy.value = true; error.value = ''; results.value = []; status.value = ''
  try { const result = await api.cinema.metadata(query(title.value.trim()), run.signal)
    if (!run.signal.aborted) { results.value = result.items; if (!result.items.length) status.value = phrase('没有找到资料，请尝试原片名或其他数据源。') }
  } catch (cause) { if (!run.signal.aborted) error.value = errorText(cause) }
  finally { if (controller === run) { busy.value = false; controller = undefined } }
}
async function choose(item: CinemaMetadata) {
  const film = target.value; if (!film) return
  stop(); const run = new AbortController(); controller = run; busy.value = true; error.value = ''
  try {
    let metadata = item
    if (item.provider === 'tmdb') { const detail = await api.cinema.metadata({ ...query(title.value), provider: 'tmdb', kind: item.kind, id: item.id }, run.signal); metadata = detail.items[0] ?? item }
    if (run.signal.aborted) return
    cache.value.delete(key(film)); cache.value.set(key(film), { ...metadata, fetchedAt: Date.now() }); persist(); results.value = []; status.value = phrase('资料已应用，原视频文件未改动。')
  } catch (cause) { if (!run.signal.aborted) error.value = errorText(cause) }
  finally { if (controller === run) { busy.value = false; controller = undefined } }
}
function remove() { if (target.value) { cache.value.delete(key(target.value)); persist(); status.value = phrase('已恢复本地影片信息。') } }
async function scan(refresh = false) {
  if (!enabled.value || !props.visible || props.scanning || busy.value) return
  stop(); const run = new AbortController(); controller = run; busy.value = true; error.value = ''; status.value = ''
  let matched = 0; let uncertain = 0
  const source = provider.value; const savedKind = kind.value; const auth = credential.value.trim(); const lang = language.value; const host = props.host
  const candidates = props.items.slice(0, 48).filter(item => refresh || !cache.value.has(key(item)))
  try {
    for (const film of candidates) {
      const parsed = cinemaSearchTitle(film.entry.name)
      const result = await api.cinema.metadata({ provider: source, kind: source === 'tvmaze' ? 'tv' : parsed.kind === 'tv' ? 'tv' : savedKind, title: parsed.title || film.entry.name, language: lang, ...(source === 'tmdb' ? { credential: auth } : {}) }, run.signal)
      if (run.signal.aborted) return
      let item = automaticCinemaMatch(film.entry.name, result.items)
      if (item && source === 'tmdb') { const detail = await api.cinema.metadata({ provider: source, kind: item.kind, title: parsed.title, language: lang, credential: auth, id: item.id }, run.signal); item = detail.items[0] ?? item }
      if (run.signal.aborted) return
      if (item) { const cacheKey = cinemaMetadataKey(host, lang, film); cache.value.delete(cacheKey); cache.value.set(cacheKey, { ...item, fetchedAt: Date.now() }); matched++; persist() } else uncertain++
    }
    status.value = phrase('本轮刮削完成') + ` · ${matched} / ${candidates.length}` + (uncertain ? ' · ' + phrase('部分影片需要手动匹配') : '')
  } catch (cause) { if (!run.signal.aborted) error.value = errorText(cause) }
  finally { if (controller === run) { busy.value = false; controller = undefined } }
}
watch(() => props.selected, item => {
  stop(); results.value = []; error.value = ''; status.value = ''; target.value = item
  if (item) { open.value = true; const parsed = cinemaSearchTitle(item.entry.name); title.value = parsed.title; kind.value = parsed.kind; if (provider.value === 'tvmaze') kind.value = 'tv' }
}, { immediate: true })
watch([() => props.host, language], () => { stop(); enabled.value = false; credential.value = ''; target.value = undefined; results.value = []; publish() })
watch([provider, kind], () => { stop(); results.value = []; error.value = ''; if (provider.value === 'tvmaze') kind.value = 'tv' })
watch(() => props.items, () => { stop(); results.value = []; publish() })
watch(() => props.visible, value => { if (!value) stop() })
watch(() => props.scanning, value => { if (!value && enabled.value) void scan() })
watch(enabled, value => { if (value) void scan(); else stop() })
watch(credential, () => { stop(); results.value = [] })
onBeforeUnmount(() => { stop(); credential.value = '' })
async function focusSearch() { open.value = true; await nextTick(); searchInput.value?.focus(); searchInput.value?.scrollIntoView?.({ block: 'center', behavior: 'auto' }) }
defineExpose({ focusSearch })
publish()
</script>

<template>
  <section class="metadata" :aria-label="phrase('影视刮削')">
    <button class="button button--secondary" type="button" :aria-expanded="open" @click="open = !open">{{ phrase('影视刮削') }}</button>
    <div v-if="open" class="metadata__body">
      <p>{{ phrase('联网查询会发送片名，不发送视频或文件路径。结果保存在当前浏览器，最多保留 200 部、30 天。') }}</p>
      <div class="metadata__controls">
        <label>{{ phrase('数据源') }}<select v-model="provider"><option value="wikipedia">Wikipedia</option><option value="tvmaze">TVmaze</option><option value="tmdb">TMDB</option></select></label>
        <label>{{ phrase('影片类型') }}<select v-model="kind" :disabled="provider === 'tvmaze'"><option value="movie">{{ phrase('电影') }}</option><option value="tv">{{ phrase('剧集') }}</option></select></label>
        <label v-if="provider === 'tmdb'">{{ phrase('TMDB 密钥或只读令牌') }}<input v-model="credential" type="password" autocomplete="off" maxlength="2048" /></label>
      </div>
      <p v-if="provider === 'tmdb'">{{ phrase('密钥仅在本次打开的影视窗口内使用，关闭窗口后清除。') }} <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noopener noreferrer">TMDB API</a></p>
      <p v-else>{{ phrase('无需密钥。Wikipedia 提供百科简介与可用配图；TVmaze 提供剧集海报、年份、评分和类型。中文及冷门影片可能需要手动搜索。') }}</p>
      <div class="metadata__controls">
        <label class="metadata__check"><input v-model="enabled" type="checkbox" :disabled="provider === 'tmdb' && !credential.trim()" />{{ phrase('自动匹配当前列表') }}</label>
        <button class="button button--secondary" type="button" :disabled="!enabled || busy || scanning" @click="scan(true)">{{ phrase('重新刮削') }}</button>
        <button v-if="busy" class="button button--secondary" type="button" @click="stop">{{ phrase('停止刮削') }}</button>
      </div>
      <p>{{ phrase('每轮最多处理前 48 部影片，仅自动应用片名和年份明确匹配的结果。编辑影片可手动选择或清除资料。') }}</p>
      <form v-if="target" class="metadata__controls" @submit.prevent="search">
        <label>{{ phrase('匹配片名') }}<input ref="searchInput" v-model="title" maxlength="240" required /></label>
        <button class="button button--primary" :disabled="busy || (provider === 'tmdb' && !credential.trim())">{{ phrase('搜索资料') }}</button>
        <button class="button button--secondary" type="button" :disabled="busy" @click="remove">{{ phrase('清除匹配') }}</button>
      </form>
      <p v-else>{{ phrase('点击影片的编辑按钮，可搜索并选择正确的资料。') }}</p>
      <article v-if="selectedMetadata" class="metadata__selected">
        <strong>{{ selectedMetadata.title }} {{ selectedMetadata.year }}</strong>
        <p v-if="selectedMetadata.rating">{{ selectedMetadata.rating.toFixed(1) }} / 10 · {{ selectedMetadata.genres?.join(' / ') }}</p>
        <p>{{ selectedMetadata.overview }}</p><p v-if="selectedMetadata.cast?.length">{{ selectedMetadata.cast.join(' · ') }}</p>
        <a :href="cinemaSourceURL(selectedMetadata)" target="_blank" rel="noopener noreferrer">{{ selectedMetadata.provider }}</a>
      </article>
      <p v-if="busy" role="status">{{ phrase('正在获取影片资料') }}</p>
      <p v-if="error" role="alert">{{ error }}</p><p v-if="status" role="status">{{ status }}</p>
      <ul v-if="results.length" class="metadata__results">
        <li v-for="item in results" :key="`${item.provider}:${item.id}`">
          <img v-if="item.poster" :src="cinemaMetadataImage(item)" alt="" loading="lazy" @error="($event.target as HTMLImageElement).hidden = true" />
          <div><strong>{{ item.title }} {{ item.year }}</strong><p>{{ item.overview.slice(0, 280) }}</p>
            <div class="metadata__controls"><button class="button button--secondary" type="button" :disabled="busy" @click="choose(item)">{{ phrase('使用此资料') }}</button><a :href="cinemaSourceURL(item)" target="_blank" rel="noopener noreferrer">{{ item.provider }}</a></div>
          </div>
        </li>
      </ul>
      <footer>
        <a href="https://www.tvmaze.com/api" target="_blank" rel="noopener noreferrer">TVmaze · CC BY-SA</a> · <a href="https://www.wikipedia.org/" target="_blank" rel="noopener noreferrer">Wikipedia · CC BY-SA</a>
        <p v-if="provider === 'tmdb'"><a href="https://www.themoviedb.org" target="_blank" rel="noopener noreferrer"><svg viewBox="0 0 24 24" width="24" height="24" role="img" aria-label="TMDB"><path :d="siThemoviedatabase.path" /></svg> TMDB</a> · This product uses the TMDB API but is not endorsed or certified by TMDB.</p>
      </footer>
    </div>
  </section>
</template>

<style scoped>
.metadata { margin-block: 16px; }
.metadata__body { display: grid; gap: 12px; margin-top: 12px; padding: 16px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-subtle); font-size: 14px; line-height: 1.55; overflow-wrap: anywhere; }
.metadata__body p { margin: 0; color: var(--text-soft); }
.metadata__controls { display: flex; align-items: end; gap: 12px; flex-wrap: wrap; }
label { display: grid; gap: 6px; min-width: 0; }
input:not([type=checkbox]), select { max-width: 100%; min-width: 0; padding: 8px; font: inherit; color: var(--text); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-md); }
.metadata__check { display: flex; align-items: center; align-self: center; }
.metadata__results { display: grid; gap: 16px; list-style: none; padding: 0; margin: 0; }
.metadata__results li { display: flex; gap: 12px; align-items: start; }
.metadata__results li > div { min-width: 0; }
.metadata__results img { width: 70px; height: 105px; object-fit: cover; border-radius: var(--radius-md); }
footer { font-size: 13px; }
footer svg { fill: #01b4e4; vertical-align: middle; }
.metadata__selected { display: grid; gap: 8px; padding: 12px; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-md); }
a { color: var(--brand); }
</style>
