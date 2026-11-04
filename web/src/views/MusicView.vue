<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Music2, Play, Pause, SkipBack, SkipForward, FolderOpen, Plus, RefreshCw, Download, X } from '@lucide/vue'
import HostSwitcher from '@/components/common/HostSwitcher.vue'
import { api } from '@/lib/api'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { fileHostStatus } from '@/lib/fileHostStatus'
import { clusterHostPanelPageURL } from '@/lib/clusterHostNavigation'
import { withoutUnsupportedLightNodes } from '@/lib/nodeFeatureHosts'
import { galleryParentPath, normalizeGalleryRoot } from '@/lib/gallery'
import { MUSIC_ROOT, MUSIC_LIMIT, musicExtension, musicTime, parseLyrics, readMusicFolder, tracksFromEntries, trackKey, type Track } from '@/lib/music'
import { readMusicTags } from '@/lib/musicTags'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import type { ClusterHost, FileEntry } from '@/types/api'

usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/MusicView/en-US').then(m => m.default) : import('@/i18n/pages/MusicView/zh-TW').then(m => m.default))
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const i18n = useI18n()
const route = useRoute(), router = useRouter()
const hostId = computed(() => typeof route.query.hostId === 'string' ? route.query.hostId : '')
const path = computed(() => typeof route.query.path === 'string' && (route.query.path === '/' || normalizeGalleryRoot(route.query.path)) ? route.query.path : MUSIC_ROOT)
const location = ref(''), search = ref(''), error = ref(''), loading = ref(false), truncated = ref(false)
const entries = ref<FileEntry[]>([]), hosts = ref<ClusterHost[]>([]), hostsError = ref(false)
const picker = ref<InstanceType<typeof HostSwitcher>>()
const activeHost = computed(() => hosts.value.find(h => hostId.value ? h.id === hostId.value : h.isLocal))
const hostLabel = computed(() => activeHost.value?.name || hostId.value || phrase('本机'))
const tracks = computed(() => tracksFromEntries(entries.value, hostId.value).filter(t => `${t.title} ${t.artist}`.toLocaleLowerCase().includes(search.value.toLocaleLowerCase())))
const folders = computed(() => entries.value.filter(e => e.kind === 'directory' && !e.name.startsWith('.')))
const queue = ref<Track[]>([]), current = ref<Track>(), audio = ref<HTMLAudioElement>()
const playing = ref(false), waiting = ref(false), playbackError = ref(''), position = ref(0), duration = ref(0), volume = ref(0.8)
const mode = ref<'order' | 'repeat' | 'one' | 'shuffle'>('order')
const lyricText = ref(''), lyricError = ref(''), showQueue = ref(false), coverFailed = ref(false)
const lyrics = computed(() => parseLyrics(lyricText.value))
const activeLine = computed(() => { let active = -1; lyrics.value.forEach((line, index) => { if (line.time <= position.value) active = index }); return active })
const embeddedCover = ref('')
const source = computed(() => current.value ? fileAPIForHost(current.value.hostId).contentUrl(current.value.entry.path) : undefined)
const cover = computed(() => embeddedCover.value || (current.value?.cover ? fileAPIForHost(current.value.hostId).contentUrl(current.value.cover) : undefined))
const currentHost = computed(() => hosts.value.find(h => current.value?.hostId ? h.id === current.value.hostId : h.isLocal)?.name || current.value?.hostId || phrase('本机'))
let listing: AbortController | undefined, metadata: AbortController | undefined
let destroyed = false, playSequence = 0
async function loadHosts() {
  try { const result = await api.cluster.hosts(); if (!destroyed) { hosts.value = withoutUnsupportedLightNodes(result.items); hostsError.value = false } }
  catch { if (!destroyed) hostsError.value = true }
}
async function load() {
  listing?.abort(); const controller = new AbortController(); listing = controller
  entries.value = []; error.value = ''; loading.value = true; truncated.value = false
  try { const result = await readMusicFolder(path.value, hostId.value, controller.signal); if (!controller.signal.aborted) { entries.value = result.entries; truncated.value = result.truncated } }
  catch (e) { if (!controller.signal.aborted) error.value = e instanceof Error ? e.message : String(e) }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function navigate(target: string, host = hostId.value) { void router.push({ name: 'music', query: { path: target, ...(host ? { hostId: host } : {}) } }) }
function openLocation() {
  const target = location.value === '/' ? '/' : normalizeGalleryRoot(location.value)
  if (!target) { error.value = phrase('请输入有效的绝对路径'); return }
  navigate(target)
}
function selectHost(host: ClusterHost) {
  picker.value?.close(true)
  const status = fileHostStatus(host)
  if (status.action === 'manage') { void router.push({ name: 'cluster' }); return }
  if (status.action === 'open') {
    const target = clusterHostPanelPageURL(host, 'music')
    if (!target) { void router.push({ name: 'cluster' }); return }
    if (host.transportSecurity === 'e2e_http' && !window.confirm(i18n.t('cluster.confirm.openHttpPanel'))) return
    window.open(target, '_blank', 'noopener,noreferrer'); return
  }
  navigate(MUSIC_ROOT, host.isLocal ? '' : host.id)
}
async function play(track: Track) {
  const sequence = ++playSequence
  metadata?.abort(); metadata = new AbortController()
  const controller = metadata
  if (embeddedCover.value) URL.revokeObjectURL(embeddedCover.value)
  embeddedCover.value = ''
  audio.value?.pause(); current.value = { ...track }; lyricText.value = ''; lyricError.value = ''; coverFailed.value = false
  position.value = 0; duration.value = 0; playbackError.value = ''; waiting.value = true
  await nextTick()
  if (sequence !== playSequence || destroyed) return
  audio.value?.load()
  void audio.value?.play().catch(() => { if (sequence === playSequence) { waiting.value = false; playbackError.value = phrase('无法播放：请重试，或下载后使用支持该编码的播放器。') } })
  const tagTimeout = window.setTimeout(() => controller.abort(), 8000)
  void readMusicTags(fileAPIForHost(track.hostId).contentUrl(track.entry.path), controller.signal).then(tags => {
    if (sequence !== playSequence || destroyed || !current.value) return
    if (tags.title) current.value.title = tags.title
    if (tags.artist) current.value.artist = tags.artist
    if (tags.picture) embeddedCover.value = URL.createObjectURL(tags.picture)
    if (!track.lyrics && tags.lyrics) lyricText.value = tags.lyrics
  }).catch(() => {}).finally(() => clearTimeout(tagTimeout))
  if (track.lyrics) {
    const timeout = window.setTimeout(() => controller.abort(), 8000)
    try { const text = await fileAPIForHost(track.hostId).text(track.lyrics, controller.signal); if (sequence === playSequence) lyricText.value = text.slice(0, 256 * 1024) }
    catch { if (sequence === playSequence) lyricError.value = phrase('歌词读取失败，可重新播放以重试') }
    finally { clearTimeout(timeout) }
  }
}
function playList(track = tracks.value[0]) { if (track) { queue.value = [...tracks.value]; void play(track) } }
function enqueue(track: Track) {
  if (queue.value.length >= MUSIC_LIMIT || queue.value.some(t => trackKey(t) === trackKey(track))) return
  queue.value.push(track)
}
function step(direction: number, ended = false) {
  if (!queue.value.length) return
  if (ended && mode.value === 'one' && current.value) { void play(current.value); return }
  let index = queue.value.findIndex(t => current.value && trackKey(t) === trackKey(current.value)) + direction
  if (mode.value === 'shuffle') {
    const choices = queue.value.filter(t => !current.value || trackKey(t) !== trackKey(current.value))
    void play(choices[Math.floor(Math.random() * choices.length)] || queue.value[0]!); return
  }
  if (ended && index >= queue.value.length && mode.value === 'order') return
  index = (index + queue.value.length) % queue.value.length
  const target = queue.value[index]
  if (target) void play(target)
}
async function toggle() {
  if (!audio.value || !current.value) return
  if (!audio.value.paused) audio.value.pause()
  else { playbackError.value = ''; try { await audio.value.play() } catch { playbackError.value = phrase('无法播放：请重试，或下载后使用支持该编码的播放器。') } }
}
function seek(event: Event) { if (audio.value) audio.value.currentTime = Number((event.target as HTMLInputElement).value) }
function files() { void router.push({ name: 'files', query: { path: path.value, ...(hostId.value ? { hostId: hostId.value } : {}) } }) }
watch([path, hostId], () => { location.value = path.value; search.value = ''; void load() }, { immediate: true })
watch(volume, value => { if (audio.value) audio.value.volume = value })
onMounted(() => { if (audio.value) audio.value.volume = volume.value; void loadHosts() })
onBeforeUnmount(() => { destroyed = true; ++playSequence; if (embeddedCover.value) URL.revokeObjectURL(embeddedCover.value); listing?.abort(); metadata?.abort(); audio.value?.pause(); audio.value?.removeAttribute('src'); audio.value?.load() })
</script>

<template>
  <main class="page music-page">
    <header class="music-heading">
      <div class="music-brand"><img src="/desktop-icons/music-kpanel-flat-v1.webp" alt="" /><div><h1>音乐</h1><p>你的音乐，随处聆听</p></div></div>
      <HostSwitcher ref="picker" :hosts="hosts" :active-id="activeHost?.id || hostId" :label="hostLabel" :error="hostsError" :status-of="h => ({ ...fileHostStatus(h), label: phrase(fileHostStatus(h).label) })" @select="selectHost" @refresh="loadHosts" />
    </header>
    <form class="music-toolbar" @submit.prevent="openLocation">
      <button type="button" class="button button--secondary" :disabled="path === '/'" :aria-label="phrase('上级文件夹')" @click="navigate(galleryParentPath(path))"><FolderOpen :size="18" /></button>
      <input v-model="location" :aria-label="phrase('音乐文件夹')" spellcheck="false" />
      <button class="button button--secondary" type="submit">打开</button>
      <button class="button button--secondary" type="button" :aria-label="phrase('刷新')" :disabled="loading" @click="load"><RefreshCw :size="18" /></button>
      <button class="button button--secondary" type="button" @click="files">文件管理</button>
    </form>
    <div class="music-layout">
      <section class="music-library">
        <div class="music-library__bar"><h2>音乐库</h2><input v-model="search" type="search" :placeholder="phrase('搜索歌曲或歌手')" :aria-label="phrase('搜索歌曲或歌手')" /><button class="button button--primary" :disabled="!tracks.length" @click="playList()"><Play :size="16" /> 播放全部</button></div>
        <p v-if="loading" role="status">正在读取音乐…</p>
        <div v-else-if="error" role="alert" class="music-error"><p>{{ error }}</p><p>检查主机连接和文件夹路径后重试。也可从文件管理上传音乐。</p><button class="button button--secondary" @click="load">重试</button></div>
        <template v-else>
          <p v-if="truncated" role="status">仅显示部分文件，请进入子文件夹继续浏览。</p>
          <div v-if="folders.length" class="music-folders"><button v-for="folder in folders" :key="folder.path" @click="navigate(folder.path)"><FolderOpen :size="20" /><span>{{ folder.name }}</span></button></div>
          <div v-if="!tracks.length" class="music-empty"><Music2 :size="48" /><h2>让音乐住进这里</h2><p>选择音乐文件夹，或通过文件管理上传歌曲。</p></div>
          <ol class="music-tracks">
            <li v-for="track in tracks" :key="trackKey(track)" :class="{ active: current && trackKey(current) === trackKey(track) }">
              <button class="music-track" @click="playList(track)"><span class="music-track__icon"><Music2 :size="22" /></span><span class="music-track__name"><strong>{{ track.title }}</strong><span>{{ track.artist || phrase('未知歌手') }}</span></span><span class="music-format">{{ musicExtension(track.entry.name).toUpperCase() }}</span></button>
              <button class="music-add" :disabled="queue.length >= MUSIC_LIMIT || queue.some(t => trackKey(t) === trackKey(track))" :aria-label="phrase('加入播放队列')" @click="enqueue(track)"><Plus :size="18" /></button>
            </li>
          </ol>
        </template>
      </section>
      <aside class="music-player">
        <div class="music-cover"><img v-if="cover && !coverFailed" :src="cover" alt="" @error="coverFailed = true" /><Music2 v-else :size="76" /></div>
        <h2>{{ current?.title || phrase('现在播放') }}</h2><p>{{ current?.artist || phrase('选一首喜欢的歌') }}<template v-if="current"> · {{ currentHost }}</template></p>
        <p v-if="waiting" role="status">正在缓冲…</p>
        <p v-if="playbackError" role="alert" class="music-error">{{ playbackError }}</p>
        <div class="music-transport"><button :disabled="!queue.length" :aria-label="phrase('上一首')" @click="step(-1)"><SkipBack /></button><button class="music-play" :disabled="!current" :aria-label="playing ? phrase('暂停') : phrase('播放')" @click="toggle"><Pause v-if="playing" /><Play v-else /></button><button :disabled="!queue.length" :aria-label="phrase('下一首')" @click="step(1)"><SkipForward /></button></div>
        <input class="music-seek" type="range" min="0" :max="duration || 0" step="0.1" :value="position" :disabled="!duration" :aria-label="phrase('播放进度')" @input="seek" />
        <div class="music-time"><span>{{ musicTime(position) }}</span><span>{{ musicTime(duration) }}</span></div>
        <div class="music-options"><select v-model="mode" :aria-label="phrase('播放模式')"><option value="order">顺序播放</option><option value="repeat">列表循环</option><option value="one">单曲循环</option><option value="shuffle">随机播放</option></select><label>{{ phrase('音量') }}<input v-model.number="volume" type="range" min="0" max="1" step="0.01" /></label></div>
        <div class="music-actions"><button class="button button--secondary" @click="showQueue = !showQueue">{{ phrase('播放队列') }} · {{ queue.length }}</button><a v-if="current" class="button button--secondary" :href="fileAPIForHost(current.hostId).contentUrl(current.entry.path, 'attachment')" :aria-label="phrase('下载原文件')"><Download :size="18" /></a></div>
        <section v-if="showQueue" class="music-queue"><div v-for="track in queue" :key="trackKey(track)"><button @click="play(track)">{{ track.title }} <small>{{ track.hostId || phrase('本机') }}</small></button><button :aria-label="phrase('移出队列')" @click="queue = queue.filter(t => trackKey(t) !== trackKey(track))"><X :size="16" /></button></div></section>
        <section class="music-lyrics" :aria-label="phrase('歌词')"><p v-if="lyricError" role="status">{{ lyricError }}</p><template v-else-if="lyrics.length"><button v-for="(line, index) in lyrics" :key="index" :class="{ 'is-current': index === activeLine }" @click="audio && (audio.currentTime = Math.min(line.time, duration))">{{ line.text || '♪' }}</button></template><p v-else>{{ lyricText || phrase('暂无歌词：自动读取同名 LRC 文件') }}</p></section>
        <p class="music-note">原文件播放 · FLAC / WAV 等格式取决于浏览器解码能力；不支持时可下载原文件。自动读取同目录 cover、folder 或同名图片。</p>
      </aside>
    </div>
    <audio ref="audio" :src="source" preload="metadata" @playing="playing = true; waiting = false" @pause="playing = false" @waiting="waiting = true" @canplay="waiting = false" @timeupdate="position = audio?.currentTime || 0" @loadedmetadata="duration = Number.isFinite(audio?.duration) ? audio!.duration : 0" @ended="playing = false; step(1, true)" @error="waiting = false; playing = false; playbackError = phrase('无法播放：请重试，或下载后使用支持该编码的播放器。')" />
  </main>
</template>

<style scoped>
.music-page { container-type: inline-size; color: var(--text); }
.music-heading,.music-brand,.music-toolbar,.music-library__bar,.music-actions,.music-options { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
.music-heading { justify-content: space-between; margin-bottom: 20px; }
.music-brand img { width: 64px; height: 64px; }
h1 { font-size: 26px; margin: 0; } h2 { font-size: 18px; margin: 0; overflow-wrap: anywhere; }
p { font-size: 14px; line-height: 1.6; overflow-wrap: anywhere; } .music-brand p { margin: 4px 0 0; color: var(--muted); }
button,input,select { font: inherit; font-size: 14px; } button { cursor: pointer; } button:disabled { cursor: default; opacity: .5; }
button:focus-visible,a:focus-visible,input:focus-visible,select:focus-visible { outline: 3px solid var(--primary); outline-offset: 3px; }
.music-toolbar { margin-bottom: 20px; } .music-toolbar input { flex: 1; min-width: 150px; }
input:not([type=range]),select { background: var(--surface); color: var(--text); border: 1px solid var(--border); border-radius: var(--radius-md); padding: 10px; min-width: 0; }
.music-layout { display: grid; grid-template-columns: minmax(0,1fr) minmax(260px,340px); gap: 24px; align-items: start; }
.music-library { min-width: 0; } .music-library__bar { margin-bottom: 16px; } .music-library__bar input { flex: 1; width: 150px; }
.music-folders { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; } .music-folders button { display: flex; align-items: center; gap: 8px; max-width: 100%; overflow-wrap: anywhere; background: var(--surface); border: 1px solid var(--border); color: var(--text); padding: 12px; border-radius: var(--radius-md); }
.music-tracks { list-style: none; margin: 0; padding: 0; } .music-tracks li { display: flex; border-bottom: 1px solid var(--border); border-radius: var(--radius-md); } .music-tracks li:hover,.music-tracks li.active { background: var(--surface-raised); }
.music-track { display: flex; gap: 12px; align-items: center; padding: 12px 8px; flex: 1; min-width: 0; text-align: start; } .music-track,.music-add,.music-transport button,.music-queue button,.music-lyrics button { border: 0; background: transparent; color: var(--text); }
.music-track__icon { background: color-mix(in srgb, #e65789 12%, var(--surface)); color: #c4376c; padding: 12px; border-radius: var(--radius-md); }
.music-track__name { display: grid; gap: 4px; min-width: 0; flex: 1; overflow-wrap: anywhere; } .music-track__name span { font-size: 13px; color: var(--muted); } .music-format { font-size: 12px; color: var(--muted); } .music-add { min-width: 40px; }
.music-player { border: 1px solid var(--border); border-radius: var(--radius-lg); background: var(--surface); padding: 20px; min-width: 0; text-align: center; }
.music-cover { width: min(100%,240px); aspect-ratio: 1; display: grid; place-items: center; margin: 0 auto 20px; background: linear-gradient(145deg,#fbe4ed,#e9def9); color: #a63775; border-radius: var(--radius-lg); overflow: hidden; } .music-cover img { width: 100%; height: 100%; object-fit: cover; }
.music-transport { display: flex; align-items: center; justify-content: center; gap: 20px; margin: 18px 0; } .music-transport button { width: 44px; height: 44px; display: grid; place-items: center; border-radius: 50%; } .music-transport .music-play { width: 56px; height: 56px; background: #ab3167; color: white; }
.music-seek { width: 100%; accent-color: #ab3167; } .music-time { display: flex; justify-content: space-between; font-size: 13px; color: var(--muted); } .music-options { justify-content: space-between; margin: 16px 0; } .music-options label { display: flex; gap: 8px; align-items: center; font-size: 14px; } .music-options input { width: 80px; } .music-actions { justify-content: center; }
.music-lyrics,.music-queue { max-height: 240px; overflow-y: auto; margin-top: 20px; text-align: start; } .music-lyrics button { display: block; padding: 10px 8px; width: 100%; text-align: center; line-height: 1.6; } .music-lyrics .is-current { background: var(--surface-raised); font-weight: 700; border-left: 3px solid #ab3167; } .music-lyrics p { white-space: pre-wrap; text-align: center; }
.music-queue > div { display: flex; } .music-queue button { padding: 10px; overflow-wrap: anywhere; text-align: start; } .music-queue button:first-child { flex: 1; min-width: 0; } .music-queue small { font-size: 12px; color: var(--muted); } .music-note { font-size: 13px; color: var(--muted); margin-bottom: 0; } .music-empty { padding: 48px 16px; text-align: center; color: var(--muted); } .music-empty h2 { margin-top: 16px; } .music-error { color: var(--danger); }
@container (max-width: 720px) { .music-layout { grid-template-columns: minmax(0,1fr); } .music-player { grid-row: 1; } .music-cover { width: 160px; } }
</style>
