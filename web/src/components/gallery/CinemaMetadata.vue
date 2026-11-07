<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { api, ApiError } from '@/lib/api'
import { siThemoviedatabase } from 'simple-icons'
import GalleryImage from './GalleryImage.vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { automaticCinemaMatch, cinemaExactMatches, cinemaMetadataImage, cinemaMetadataKey, cinemaSearchTitle, cinemaSourceURL, readCinemaMetadataAttempts, readCinemaMetadataCache, readCinemaMetadataSettings, saveCinemaMetadataAttempts, saveCinemaMetadataCache, saveCinemaMetadataSettings, type CinemaMetadata, type CinemaProvider } from '@/lib/cinemaMetadata'
import type { GalleryItem } from '@/lib/gallery'

const props = defineProps<{ items: GalleryItem[]; host: string; selected?: GalleryItem; scanning: boolean; visible: boolean; limit?: number }>()
const emit = defineEmits<{ update: [metadata: Map<string, CinemaMetadata>] }>()
const i18n = useI18n()
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const language = computed(() => i18n.locale.value)
const settings = readCinemaMetadataSettings()
const open = ref(false)
const enabled = ref(settings.enabled)
const provider = ref<CinemaProvider | 'auto'>(settings.provider)
const kind = ref<'movie' | 'tv'>('movie')
const credential = ref('')
const title = ref('')
const searchInput = ref<HTMLInputElement>()
const results = ref<CinemaMetadata[]>([])
const cache = ref(readCinemaMetadataCache())
// Keep displayed results for the current scan's 4,000-entry file budget even
// when the smaller persistent browser cache evicts older records.
const sessionMatches = new Map<string, CinemaMetadata>()
const attempts = readCinemaMetadataAttempts()
const busy = ref(false)
const autoBusy = ref(false)
const error = ref('')
const status = ref('')
const target = computed(() => props.selected)
const selectedMetadata = computed(() => target.value ? cache.value.get(key(target.value)) ?? sessionMatches.get(key(target.value)) : undefined)
const memo = new Map<string, { items: CinemaMetadata[]; until: number }>()
let manual: AbortController | undefined
let automatic: AbortController | undefined
let scheduled: ReturnType<typeof setTimeout> | undefined
let disposed = false
let retryAt = 0
function key(item: GalleryItem) { return cinemaMetadataKey(props.host, language.value, item) }
function attemptKey(item: GalleryItem) { return JSON.stringify([key(item), provider.value]) }
function publish() { emit('update', new Map(props.items.flatMap(item => { const value = cache.value.get(key(item)) ?? sessionMatches.get(key(item)); return value ? [[item.entry.path, value] as const] : [] }))) }
function persist() {
  cache.value = new Map([...cache.value].slice(-200))
  if (!saveCinemaMetadataCache(cache.value)) error.value = phrase('浏览器未保存刮削结果，请检查存储空间。')
  publish()
}
function stopAuto() { automatic?.abort(); automatic = undefined; autoBusy.value = false; if (scheduled) clearTimeout(scheduled); scheduled = undefined }
function stopManual() { manual?.abort(); manual = undefined; busy.value = false }
function stop() { stopAuto(); stopManual() }
function errorText(cause: unknown) {
  if (cause instanceof ApiError) {
    if (cause.code === 'cinema_source_credentials') return phrase('数据源拒绝请求，请检查 TMDB 密钥或稍后重试。')
    if (cause.code === 'cinema_metadata_busy') return phrase('数据源繁忙，请稍后重试。')
  }
  return phrase('刮削失败，请检查面板网络后重试。')
}
async function lookup(name: string, type: 'movie' | 'tv', signal: AbortSignal, matchName: string) {
  const sources: CinemaProvider[] = provider.value === 'auto' ? type === 'tv' ? ['tvmaze', 'wikipedia'] : ['wikipedia'] : [provider.value]
  const items: CinemaMetadata[] = []; let failure: unknown; let succeeded = false
  for (const source of sources) {
    const query = { provider: source, title: name, kind: source === 'tvmaze' ? 'tv' as const : type, language: language.value, ...(source === 'tmdb' ? { credential: credential.value.trim() } : {}) }
    const memoKey = JSON.stringify(query)
    try {
      const hit = memo.get(memoKey)
      const found = hit && hit.until > Date.now() ? hit.items : (await api.cinema.metadata(query, signal)).items
      signal.throwIfAborted()
      if (memo.size >= 64) memo.delete(memo.keys().next().value!)
      memo.set(memoKey, { items: found, until: Date.now() + 60_000 }); succeeded = true; items.push(...found)
      // Preserve ambiguity in the primary source rather than letting fallback hide it.
      if (cinemaExactMatches(matchName, found).length > 0) break
    } catch (cause) { if (signal.aborted) throw cause; failure = cause }
  }
  if (!succeeded) throw failure ?? new Error('cinema source unavailable')
  return items
}
async function detailed(item: CinemaMetadata, signal: AbortSignal) {
  if (item.provider !== 'tmdb') return item
  const detail = await api.cinema.metadata({ provider: 'tmdb', kind: item.kind, title: item.title, language: language.value, credential: credential.value.trim(), id: item.id }, signal)
  return detail.items[0] ?? item
}
function apply(film: GalleryItem, item: CinemaMetadata) {
  if (sessionMatches.size >= 4000) sessionMatches.delete(sessionMatches.keys().next().value!)
  sessionMatches.set(key(film), item)
  cache.value.delete(key(film)); cache.value.set(key(film), { ...item, fetchedAt: Date.now() }); attempts.delete(attemptKey(film)); persist()
}
function schedule(delay = 200) {
  if (scheduled) clearTimeout(scheduled)
  if (disposed || !enabled.value || !props.visible || props.scanning || busy.value || (open.value && target.value)) return
  scheduled = setTimeout(() => { scheduled = undefined; void scan() }, Math.max(delay, retryAt - Date.now()))
}
async function scan(refresh = false) {
  if (disposed || !enabled.value || !props.visible || props.scanning || busy.value || autoBusy.value) return
  if (provider.value === 'tmdb' && !credential.value.trim()) return
  if (refresh) memo.clear()
  const run = new AbortController(); automatic = run; autoBusy.value = true
  const items = [...(props.selected ? [props.selected] : []), ...props.items.slice(0, props.limit ?? 48)]
  const seen = new Set<string>(); let count = 0
  try {
    for (const film of items) {
      const identity = key(film); if (seen.has(identity)) continue; seen.add(identity)
      if (!refresh && (cache.value.has(identity) || sessionMatches.has(identity) || (attempts.get(attemptKey(film)) ?? 0) > Date.now())) continue
      const parsed = cinemaSearchTitle(film.entry.name)
      const found = await lookup(parsed.title || film.entry.name, parsed.kind, run.signal, film.entry.name)
      let item = automaticCinemaMatch(film.entry.name, found)
      if (item) item = await detailed(item, run.signal)
      run.signal.throwIfAborted()
      if (item) { apply(film, item); count++ }
      else attempts.set(attemptKey(film), Date.now() + 24 * 60 * 60 * 1000)
    }
    if (count) status.value = phrase('影片资料已自动更新')
  } catch (cause) {
    if (!run.signal.aborted) { retryAt = Date.now() + 60_000; error.value = errorText(cause) }
  } finally {
    saveCinemaMetadataAttempts(attempts)
    if (automatic === run) { automatic = undefined; autoBusy.value = false; if (retryAt > Date.now()) schedule(retryAt - Date.now()) }
  }
}
async function search() {
  if (!title.value.trim() || !target.value) return
  stop(); const run = new AbortController(); manual = run; busy.value = true; error.value = ''; results.value = []; status.value = ''
  try { const items = await lookup(title.value.trim(), kind.value, run.signal, title.value.trim())
    if (!run.signal.aborted) { results.value = items; if (!items.length) status.value = phrase('没有找到资料，请尝试原片名或其他数据源。') }
  } catch (cause) { if (!run.signal.aborted) error.value = errorText(cause) }
  finally { if (manual === run) { busy.value = false; manual = undefined } }
}
async function choose(item: CinemaMetadata) {
  const film = target.value; if (!film) return
  stop(); const run = new AbortController(); manual = run; busy.value = true; error.value = ''
  try { const metadata = await detailed(item, run.signal); run.signal.throwIfAborted(); apply(film, metadata); results.value = []; status.value = phrase('资料已应用，原视频文件未改动。') }
  catch (cause) { if (!run.signal.aborted) error.value = errorText(cause) }
  finally { if (manual === run) { busy.value = false; manual = undefined } }
}
function remove() { if (target.value) { cache.value.delete(key(target.value)); sessionMatches.delete(key(target.value)); attempts.set(attemptKey(target.value), Date.now() + 30 * 24 * 60 * 60 * 1000); saveCinemaMetadataAttempts(attempts); persist(); status.value = phrase('已恢复本地影片信息。') } }
watch(() => props.selected, item => { stop(); results.value = []; error.value = ''; status.value = ''; if (item) { const parsed = cinemaSearchTitle(item.entry.name); title.value = parsed.title; kind.value = parsed.kind } schedule() }, { immediate: true })
watch([() => props.host, language], () => { stop(); credential.value = ''; memo.clear(); sessionMatches.clear(); retryAt = 0; results.value = []; publish(); schedule() })
watch([provider, credential], () => { stop(); memo.clear(); retryAt = 0; results.value = []; error.value = ''; saveCinemaMetadataSettings({ enabled: enabled.value, provider: provider.value }); schedule() })
watch(enabled, value => { stopAuto(); saveCinemaMetadataSettings({ enabled: value, provider: provider.value }); if (value) schedule() })
watch([() => props.items, () => props.limit, () => props.scanning, () => props.visible], () => { stopAuto(); publish(); schedule() }, { immediate: true })
watch(open, value => { if (value && target.value) stopAuto(); if (!value) { stopManual(); results.value = []; schedule() } })
onBeforeUnmount(() => { disposed = true; stop(); credential.value = ''; memo.clear() })
async function focusSearch() { open.value = true; await nextTick(); searchInput.value?.focus(); searchInput.value?.scrollIntoView?.({ block: 'center', behavior: 'auto' }) }
function openSettings() { open.value = !open.value }
defineExpose({ focusSearch, openSettings })
</script>

<template>
  <section v-if="open" class="metadata" :aria-label="phrase('影片资料设置')">
    <div class="metadata__body">
      <header><h2>{{ phrase('影片资料设置') }}</h2><button class="button button--secondary" type="button" @click="open = false">{{ phrase('关闭') }}</button></header>
      <p>{{ phrase('海报和资料会在浏览片库时自动补全，电影与剧集自动选择数据源；只查询片名，不上传视频。') }}</p>
      <div class="metadata__controls">
        <label class="metadata__check"><input v-model="enabled" type="checkbox" />{{ phrase('自动补全海报和资料') }}</label>
        <label>{{ phrase('数据源') }}<select v-model="provider"><option value="auto">{{ phrase('自动选择') }}</option><option value="wikipedia">Wikipedia</option><option value="tvmaze">TVmaze</option><option value="tmdb">TMDB</option></select></label>
        <label v-if="provider === 'tmdb'">{{ phrase('TMDB 密钥或只读令牌') }}<input v-model="credential" type="password" autocomplete="off" maxlength="2048" /></label>
      </div>
      <p>{{ phrase('已有资料直接复用；未匹配影片一天内不重复查询。结果缓存在当前浏览器，最多 200 部、30 天，可随时关闭自动补全。') }}</p>
      <p v-if="provider === 'tmdb'">{{ phrase('密钥仅在本次打开的影视窗口内使用，关闭窗口后清除。') }} <a href="https://www.themoviedb.org/settings/api" target="_blank" rel="noopener noreferrer">TMDB API</a></p>
      <form v-if="target" class="metadata__controls" @submit.prevent="search">
        <label>{{ phrase('匹配片名') }}<input ref="searchInput" v-model="title" maxlength="240" required /></label>
        <label>{{ phrase('影片类型') }}<select v-model="kind"><option value="movie">{{ phrase('电影') }}</option><option value="tv">{{ phrase('剧集') }}</option></select></label>
        <button class="button button--primary" :disabled="busy || (provider === 'tmdb' && !credential.trim())">{{ phrase('搜索资料') }}</button>
        <button class="button button--secondary" type="button" :disabled="busy" @click="remove">{{ phrase('清除匹配') }}</button>
      </form>
      <article v-if="selectedMetadata" class="metadata__selected"><strong>{{ selectedMetadata.title }} {{ selectedMetadata.year }}</strong><p>{{ selectedMetadata.overview }}</p><a :href="cinemaSourceURL(selectedMetadata)" target="_blank" rel="noopener noreferrer">{{ selectedMetadata.provider }}</a></article>
      <div class="metadata__controls"><button class="button button--secondary" type="button" :disabled="!enabled || autoBusy || scanning || busy" @click="scan(true)">{{ phrase('重新获取资料') }}</button><button v-if="autoBusy || busy" class="button button--secondary" type="button" @click="stop">{{ phrase('停止刮削') }}</button></div>
      <p v-if="busy || autoBusy" role="status">{{ phrase('正在获取影片资料') }}</p><p v-if="error" role="alert">{{ error }}</p><p v-if="status" role="status">{{ status }}</p>
      <ul v-if="results.length" class="metadata__results"><li v-for="item in results" :key="`${item.provider}:${item.id}`"><GalleryImage v-if="item.poster" :src="cinemaMetadataImage(item)!" /><div><strong>{{ item.title }} {{ item.year }}</strong><p>{{ item.overview.slice(0, 280) }}</p><div class="metadata__controls"><button class="button button--secondary" type="button" :disabled="busy" @click="choose(item)">{{ phrase('使用此资料') }}</button><a :href="cinemaSourceURL(item)" target="_blank" rel="noopener noreferrer">{{ item.provider }}</a></div></div></li></ul>
      <footer><a href="https://www.tvmaze.com/api" target="_blank" rel="noopener noreferrer">TVmaze · CC BY-SA</a> · <a href="https://www.wikipedia.org/" target="_blank" rel="noopener noreferrer">Wikipedia · CC BY-SA</a><p><a href="https://www.themoviedb.org" target="_blank" rel="noopener noreferrer"><svg viewBox="0 0 24 24" width="24" height="24" role="img" aria-label="TMDB"><path :d="siThemoviedatabase.path" /></svg> TMDB</a> · This product uses the TMDB API but is not endorsed or certified by TMDB.</p></footer>
    </div>
  </section>
</template>

<style scoped>
.metadata { margin-block: 16px; }
.metadata__body { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-subtle); font-size: 14px; line-height: 1.55; overflow-wrap: anywhere; }
.metadata__body p { margin: 0; color: var(--text-soft); }
header, .metadata__controls { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
header { justify-content: space-between; } h2 { margin: 0; font-size: 18px; }
label { display: grid; gap: 6px; min-width: 0; }
input:not([type=checkbox]), select { max-width: 100%; min-width: 0; padding: 8px; font: inherit; color: var(--text); background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-md); }
.metadata__check { display: flex; align-items: center; }
.metadata__results { display: grid; gap: 16px; list-style: none; padding: 0; margin: 0; }
.metadata__results li { display: flex; gap: 12px; align-items: start; }
.metadata__results li > div { min-width: 0; }
.metadata__results img { width: 70px; height: 105px; object-fit: cover; border-radius: var(--radius-md); }
footer { font-size: 13px; } footer svg { fill: #01b4e4; vertical-align: middle; }
.metadata__selected { display: grid; gap: 8px; padding: 12px; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-md); }
a { color: var(--brand); }
</style>
