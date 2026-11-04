<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Music2, Play, Pause, SkipBack, SkipForward, FolderOpen, Plus, RefreshCw, Download, X, Shuffle, Repeat, Repeat1, ListMusic, ArrowUp, ArrowDown, Volume2, VolumeX, Search, Disc3, ChevronRight, ListPlus } from '@lucide/vue'
import HostSwitcher from '@/components/common/HostSwitcher.vue'
import { api } from '@/lib/api'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { fileHostStatus } from '@/lib/fileHostStatus'
import { clusterHostPanelPageURL } from '@/lib/clusterHostNavigation'
import { withoutUnsupportedLightNodes } from '@/lib/nodeFeatureHosts'
import { galleryParentPath, normalizeGalleryRoot } from '@/lib/gallery'
import { MUSIC_ROOT, musicExtension, musicTime, parseLyrics, readMusicFolder, tracksFromEntries, trackKey, type Track } from '@/lib/music'
import { useMusicPlayer } from '@/stores/musicPlayer'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { formatBytes } from '@/lib/format'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import type { ClusterHost, FileEntry } from '@/types/api'

usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/MusicView/en-US').then(m => m.default) : import('@/i18n/pages/MusicView/zh-TW').then(m => m.default))
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const i18n = useI18n(), player = useMusicPlayer(), state = player.state
const route = useRoute(), router = useRouter()
const visible = inject(desktopWindowVisibleKey, ref(true))
const hostId = computed(() => typeof route.query.hostId === 'string' ? route.query.hostId : '')
const path = computed(() => typeof route.query.path === 'string' && (route.query.path === '/' || normalizeGalleryRoot(route.query.path)) ? route.query.path : MUSIC_ROOT)
const location = ref(''), search = ref(''), error = ref(''), loading = ref(false), truncated = ref(false)
const entries = ref<FileEntry[]>([]), hosts = ref<ClusterHost[]>([]), hostsError = ref(false)
const picker = ref<InstanceType<typeof HostSwitcher>>()
const activeHost = computed(() => hosts.value.find(h => hostId.value ? h.id === hostId.value : h.isLocal))
const hostLabel = computed(() => activeHost.value?.name || hostId.value || phrase('本机'))
const allTracks = computed(() => tracksFromEntries(entries.value, hostId.value))
const sort = ref('name'), tab = ref<'library' | 'listen' | 'queue'>(route.query.view === 'listen' ? 'listen' : 'library'), locationOpen = ref(false)
const tracks = computed(() => allTracks.value.filter(t => `${t.title} ${t.artist}`.toLocaleLowerCase().includes(search.value.toLocaleLowerCase())).sort((a,b) => sort.value === 'artist' ? a.artist.localeCompare(b.artist) || a.title.localeCompare(b.title) : a.title.localeCompare(b.title, undefined, {numeric:true})))
const folders = computed(() => entries.value.filter(e => e.kind === 'directory' && !e.name.startsWith('.')))
const volumeOpen = ref(false)
const coverFailed = ref(false), lyricPanel = ref<HTMLElement>(), following = ref(true)
const lyrics = computed(() => parseLyrics(state.lyrics))
const activeLine = computed(() => { let active = -1; lyrics.value.forEach((line, index) => { if (line.time <= state.position) active = index }); return active })
const currentHost = computed(() => hosts.value.find(h => state.current?.hostId ? h.id === state.current.hostId : h.isLocal)?.name || state.current?.hostId || phrase('本机'))
const repeatLabel = computed(() => state.repeat === 'off' ? phrase('顺序播放') : state.repeat === 'all' ? phrase('列表循环') : phrase('单曲循环'))
const notice = computed(() => state.notice === 'next' ? phrase('已设为下一首') : state.notice === 'added' ? phrase('已加入播放队列') : state.notice === 'full' ? phrase('队列已满，请先移除部分歌曲') : '')
const playbackError = computed(() => state.error === 'blocked' ? phrase('浏览器暂停了自动播放，点击播放继续') : state.error === 'network' ? phrase('音乐连接中断，请检查来源主机后重试') : state.error ? phrase('当前浏览器无法解码此音频，可下载原文件播放') : '')
let listing: AbortController | undefined, destroyed = false
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

function files() { void router.push({ name: 'files', query: { path: path.value, ...(hostId.value ? { hostId: hostId.value } : {}) } }) }
function repeat() { state.repeat = state.repeat === 'off' ? 'all' : state.repeat === 'all' ? 'one' : 'off' }
function isCurrent(track: Track) { return state.current && trackKey(state.current) === trackKey(track) }
function choose(track: Track) { if (!state.queue.length) player.replaceQueue(tracks.value, track); else player.choose(track) }
function seek(event: Event) { player.seek(Number((event.target as HTMLInputElement).value)) }
function volume(event: Event) { player.setVolume(Number((event.target as HTMLInputElement).value)) }
function artwork(track: Track) { return track.cover ? fileAPIForHost(track.hostId).contentUrl(track.cover) : '' }
function keyboard(event: KeyboardEvent) {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
  const target = event.target as HTMLElement
  if (target.closest('input, textarea, select, button, a, [contenteditable="true"]')) return
  if (event.code === 'Space') { event.preventDefault(); player.toggle() }
  else if (event.key === 'ArrowRight') { event.preventDefault(); player.seek(state.position + 5) }
  else if (event.key === 'ArrowLeft') { event.preventDefault(); player.seek(state.position - 5) }
}
async function followLyrics() {
  if (!following.value) return
  await nextTick()
  const panel = lyricPanel.value, line = panel?.querySelector<HTMLElement>('[aria-current="true"]')
  if (panel && line) panel.scrollTop = Math.max(0, line.offsetTop - panel.clientHeight / 2 + line.clientHeight / 2)
}
watch([activeLine, tab, following], followLyrics)
watch(() => route.query.view, view => { if (view === 'listen') tab.value = 'listen' })
watch(() => state.current && trackKey(state.current), () => { following.value = true; coverFailed.value = false })
watch([path, hostId], () => { location.value = path.value; search.value = ''; void load() }, { immediate: true })
watch(visible, (value, previous) => { state.viewCount += Number(value) - Number(previous ?? false) }, { immediate: true })
onMounted(() => { void loadHosts() })
onBeforeUnmount(() => { destroyed = true; listing?.abort(); if (visible.value) state.viewCount = Math.max(0, state.viewCount - 1) })
</script>

<template>
  <main class="page music-page" tabindex="0" @keydown="keyboard">
    <header class="music-header">
      <div class="music-brand"><img src="/desktop-icons/music-kpanel-flat-v1.webp" alt="" /><div><h1>音乐</h1><p>把日常，交给一首好歌</p></div></div>
    </header>
    <nav class="music-tabs" :aria-label="phrase('音乐导航')"><button :aria-current="tab === 'library' ? 'page' : undefined" @click="tab = 'library'"><Disc3 :size="18" /> 音乐库</button><button :aria-current="tab === 'listen' ? 'page' : undefined" @click="tab = 'listen'"><Music2 :size="18" /> 正在播放</button><button :aria-current="tab === 'queue' ? 'page' : undefined" @click="tab = 'queue'"><ListMusic :size="18" /> 播放队列 <span>{{ state.queue.length }}</span></button></nav>
    <div class="music-layout" :class="{ 'music-layout--focus': tab === 'listen' }">
      <section v-if="tab !== 'listen'" class="music-library">
        <template v-if="tab === 'library'">
          <div class="music-library__hero"><div><span class="music-eyebrow">私人音乐收藏</span><h2>{{ path.split('/').filter(Boolean).pop() || phrase('音乐库') }}</h2><p>{{ allTracks.length }} {{ phrase('首歌曲') }} <span v-if="folders.length"> · {{ folders.length }} {{ phrase('个文件夹') }}</span></p></div><div class="music-library__hero-actions"><button class="button button--primary" :disabled="!tracks.length" @click="player.shuffle(false); player.replaceQueue(tracks)"><Play :size="17" /> 播放全部</button><button class="music-icon-button" :aria-label="phrase('随机播放全部')" :disabled="!tracks.length" @click="player.shuffle(true); player.replaceQueue(tracks, tracks[Math.floor(Math.random() * tracks.length)])"><Shuffle :size="19" /></button></div></div>
          <div class="music-location"><div class="music-location__source"><HostSwitcher ref="picker" :hosts="hosts" :active-id="activeHost?.id || hostId" :label="hostLabel" :error="hostsError" :status-of="h => ({ ...fileHostStatus(h), label: phrase(fileHostStatus(h).label) })" @select="selectHost" @refresh="loadHosts" /><button class="music-text-button" @click="locationOpen = !locationOpen" :aria-expanded="locationOpen"><FolderOpen :size="16" /><span>{{ path }}</span><ChevronRight :size="15" /></button></div><div class="music-location__actions"><button class="button button--secondary" :aria-label="phrase('刷新')" :disabled="loading" @click="load"><RefreshCw :size="17" /></button><button class="music-text-button" @click="files">文件管理</button></div></div>
          <form v-if="locationOpen" class="music-location-form" @submit.prevent="openLocation"><input v-model="location" :aria-label="phrase('音乐文件夹')" spellcheck="false" /><button class="button button--secondary" type="submit">打开</button><button class="button button--secondary" type="button" :disabled="path === '/'" @click="navigate(galleryParentPath(path))">上级文件夹</button></form>
          <div class="music-search"><label><Search :size="18" /><input v-model="search" type="search" :placeholder="phrase('搜索歌曲或歌手')" :aria-label="phrase('搜索歌曲或歌手')" /></label><select v-model="sort" :aria-label="phrase('歌曲排序')"><option value="name">按歌名</option><option value="artist">按歌手</option></select></div>
          <p v-if="loading" role="status" class="music-empty">正在读取音乐…</p>
          <div v-else-if="error" role="alert" class="music-error"><p>{{ error }}</p><p>检查主机连接和文件夹路径后重试。也可从文件管理上传音乐。</p><button class="button button--secondary" @click="load">重试</button></div>
          <template v-else>
            <p v-if="truncated" role="status">仅显示部分文件，请进入子文件夹继续浏览。</p>
            <div v-if="folders.length && !search" class="music-folders"><button v-for="folder in folders" :key="folder.path" @click="navigate(folder.path)"><span><Disc3 :size="25" /></span><strong>{{ folder.name }}</strong><ChevronRight :size="16" /></button></div>
            <div v-if="!tracks.length" class="music-empty"><Disc3 :size="48" /><h2>{{ search ? phrase('没有匹配的歌曲') : phrase('让音乐住进这里') }}</h2><p>{{ search ? phrase('试试其他歌名或歌手') : phrase('选择音乐文件夹，或通过文件管理上传歌曲。') }}</p><button v-if="search" class="button button--secondary" @click="search = ''">清除搜索</button><button v-else class="button button--secondary" @click="locationOpen = true">选择文件夹</button></div>
            <ol class="music-tracks" :aria-label="phrase('歌曲列表')"><li v-for="(track, index) in tracks" :key="trackKey(track)" :class="{ active: isCurrent(track) }">
              <button class="music-track" :aria-label="`${phrase('播放')} ${track.title}`" @click="choose(track)"><span class="music-track__number"><Pause v-if="isCurrent(track) && state.playing" :size="16" /><Play v-else-if="isCurrent(track)" :size="16" /><template v-else>{{ String(index + 1).padStart(2, '0') }}</template></span><span class="music-track__art"><img v-if="track.cover" :src="artwork(track)" alt="" loading="lazy" @error="($event.target as HTMLImageElement).hidden = true" /><Music2 v-else :size="21" /></span><span class="music-track__name"><strong>{{ track.title }}</strong><span>{{ track.artist || phrase('未知歌手') }}</span></span><span class="music-track__format">{{ musicExtension(track.entry.name).toUpperCase() }}<small>{{ formatBytes(track.entry.sizeBytes) }}</small></span></button>
              <button class="music-icon-button music-next" :aria-label="`${phrase('下一首播放')} ${track.title}`" :title="phrase('下一首播放')" @click="player.enqueue(track, true)"><ListPlus :size="18" /></button><button class="music-icon-button" :aria-label="`${phrase('加入播放队列')} ${track.title}`" :title="phrase('加入播放队列')" @click="player.enqueue(track)"><Plus :size="18" /></button>
            </li></ol>
          </template>
        </template>
        <template v-else>
          <div class="music-section-heading"><div><span class="music-eyebrow">接下来听什么</span><h2>播放队列</h2></div><button class="button button--secondary" :disabled="!state.queue.length" @click="player.clearQueue">清空待播</button></div>
          <p class="music-caption">跨主机歌曲可以放在同一队列。移除歌曲不会打断当前播放。</p>
          <div v-if="!state.queue.length" class="music-empty"><ListMusic :size="44" /><h2>队列还是空的</h2><button class="button button--secondary" @click="tab = 'library'">去音乐库选歌</button></div>
          <ol class="music-queue"><li v-for="(track, index) in state.queue" :key="trackKey(track)" :class="{ active: isCurrent(track) }"><button class="music-track" @click="player.choose(track)"><span class="music-track__number">{{ index + 1 }}</span><span class="music-track__name"><strong>{{ track.title }}</strong><span>{{ track.artist || phrase('未知歌手') }} · {{ hosts.find(h => track.hostId ? h.id === track.hostId : h.isLocal)?.name || track.hostId || phrase('本机') }}</span></span><Play v-if="isCurrent(track)" :size="16" /></button><button class="music-icon-button" :disabled="index === 0" :aria-label="`${phrase('上移')} ${track.title}`" @click="player.move(track, -1)"><ArrowUp :size="16" /></button><button class="music-icon-button" :disabled="index === state.queue.length - 1" :aria-label="`${phrase('下移')} ${track.title}`" @click="player.move(track, 1)"><ArrowDown :size="16" /></button><button class="music-icon-button" :aria-label="`${phrase('移出队列')} ${track.title}`" @click="player.remove(track)"><X :size="17" /></button></li></ol>
        </template>
      </section>
      <aside class="music-now" :class="{ 'music-now--mobile-hidden': tab !== 'listen' }">
        <div class="music-now__album"><div class="music-cover"><img v-if="player.cover.value && !coverFailed" :src="player.cover.value" alt="" @error="coverFailed = true" /><div v-else class="music-cover__record"><Disc3 :size="96" /></div></div><span class="music-eyebrow">{{ state.current ? phrase('正在播放') : phrase('准备好，听点喜欢的') }}</span><h2>{{ state.current?.title || phrase('你的下一首好歌') }}</h2><p>{{ state.current?.artist || phrase('从音乐库开始聆听') }}</p><p v-if="state.album" class="music-caption">{{ state.album }}</p><div v-if="state.current" class="music-provenance"><span>{{ musicExtension(state.current.entry.name).toUpperCase() }} · {{ phrase('原文件') }}</span><span>{{ currentHost }}</span></div></div>
        <section class="music-lyric-section"><div class="music-section-heading"><h3>歌词</h3><button v-if="lyrics.length" class="music-text-button" :aria-pressed="following" @click="following = !following">{{ following ? phrase('跟随播放') : phrase('恢复跟随') }}</button></div><div ref="lyricPanel" class="music-lyrics" @wheel.passive="following = false" @touchmove.passive="following = false"><p v-if="state.lyricsLoading" role="status">正在读取歌词…</p><p v-else-if="state.lyricsFailed" role="status">歌词读取失败，可重新播放以重试</p><template v-else-if="lyrics.length"><button v-for="(line, index) in lyrics" :key="index" :aria-current="index === activeLine ? 'true' : undefined" @click="player.seek(line.time)">{{ line.text || '♪' }}</button></template><p v-else>{{ state.lyrics || phrase('暂无歌词：自动读取同名 LRC 文件') }}</p></div></section>
      </aside>
    </div>
    <p class="music-notice" role="status" aria-live="polite">{{ notice }}</p>
    <section class="music-console" :aria-label="phrase('播放控制')">
      <div class="music-console__summary"><button class="music-console__cover" :aria-label="phrase('查看正在播放')" @click="tab = tab === 'listen' ? 'library' : 'listen'"><img v-if="player.cover.value && !coverFailed" :src="player.cover.value" alt="" @error="coverFailed = true" /><Music2 v-else :size="23" /></button><div><strong>{{ state.current?.title || phrase('还没有播放歌曲') }}</strong><span>{{ state.current?.artist || phrase('选一首喜欢的歌') }}</span></div><a v-if="state.current" class="music-icon-button" :href="fileAPIForHost(state.current.hostId).contentUrl(state.current.entry.path, 'attachment')" :aria-label="phrase('下载原文件')"><Download :size="17" /></a></div>
      <button class="music-icon-button music-mobile-volume-toggle" :aria-label="phrase('音量设置')" :aria-expanded="volumeOpen" @click="volumeOpen = !volumeOpen"><Volume2 :size="19" /></button><div class="music-console__center"><div class="music-transport"><button class="music-icon-button" :aria-label="phrase('随机播放')" :aria-pressed="state.shuffle" @click="player.shuffle()"><Shuffle :size="18" /></button><button class="music-icon-button" :disabled="!state.current" :aria-label="phrase('上一首')" @click="player.previous"><SkipBack :size="20" /></button><button class="music-play" :disabled="!state.current" :aria-label="state.playing || state.waiting ? phrase('暂停') : phrase('播放')" @click="player.toggle"><Pause v-if="state.playing || state.waiting" :size="24" /><Play v-else :size="24" /></button><button class="music-icon-button" :disabled="!state.queue.length" :aria-label="phrase('下一首')" @click="player.next()"><SkipForward :size="20" /></button><button class="music-icon-button" :aria-label="repeatLabel" :title="repeatLabel" :aria-pressed="state.repeat !== 'off'" @click="repeat"><Repeat1 v-if="state.repeat === 'one'" :size="18" /><Repeat v-else :size="18" /></button></div><div class="music-progress"><span>{{ musicTime(state.position) }}</span><input type="range" min="0" :max="state.duration || 0" step="0.1" :value="state.position" :disabled="!state.duration" :aria-label="phrase('播放进度')" @input="seek" /><span>{{ musicTime(state.duration) }}</span></div></div>
      <div class="music-console__volume" :class="{ 'is-open': volumeOpen }"><button class="music-icon-button" :aria-label="state.muted ? phrase('取消静音') : phrase('静音')" :aria-pressed="state.muted" @click="player.mute"><VolumeX v-if="state.muted || state.volume === 0" :size="19" /><Volume2 v-else :size="19" /></button><input type="range" min="0" max="1" step="0.01" :value="state.muted ? 0 : state.volume" :aria-label="phrase('音量')" @input="volume" /><button class="music-icon-button" :aria-label="phrase('播放队列')" :aria-pressed="tab === 'queue'" @click="tab = tab === 'queue' ? 'library' : 'queue'"><ListMusic :size="20" /></button><button class="music-icon-button" :disabled="!state.current" :aria-label="phrase('停止播放')" @click="player.stop"><X :size="18" /></button></div>
      <div v-if="state.waiting || playbackError" class="music-playback-status" :role="playbackError ? 'alert' : 'status'"><span>{{ playbackError || phrase('正在缓冲…') }}</span><button v-if="playbackError && state.current" class="music-text-button" @click="player.play(state.current)">重试</button><button v-if="playbackError && state.queue.length > 1" class="music-text-button" @click="player.next()">跳过此曲</button></div>
    </section>
    <details class="music-help"><summary>播放说明与快捷键</summary><p>焦点在音乐页面空白区域时：空格播放或暂停，左右方向键快退或快进 5 秒。输入框和按钮保留原有键盘操作。</p><p>音乐在当前标签页内持续播放，关闭音乐窗口或切换页面不会中断；退出登录、关闭标签页或停止播放会结束聆听。</p><p>原文件播放 · FLAC / WAV 等格式取决于浏览器解码能力；不支持时可下载原文件。自动读取同目录 cover、folder 或同名图片。</p></details>
  </main>
</template>

<style scoped>
.music-page { display:block; --music-accent: #ad315e; --music-tint: color-mix(in srgb, var(--music-accent) 8%, var(--surface)); container-type: inline-size; color: var(--text); }
.music-page:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
.music-header,.music-brand,.music-tabs,.music-location,.music-section-heading,.music-search,.music-transport,.music-progress { display:flex; align-items:center; gap:12px; }
.music-header { justify-content:space-between; flex-wrap:wrap; margin-bottom:16px; }
.music-brand img { width:52px; height:52px; } h1 { font-size:26px; margin:0; } h2 { font-size:22px; line-height:1.35; margin:8px 0; overflow-wrap:anywhere; } h3 { font-size:16px; margin:0; }
p { font-size:14px; line-height:1.65; overflow-wrap:anywhere; } .music-brand p { margin:4px 0 0; color:var(--muted); }
button,input,select { font:inherit; font-size:14px; } button { cursor:pointer; } button:disabled { opacity:.45; cursor:default; }
button:focus-visible,a:focus-visible,input:focus-visible,select:focus-visible,summary:focus-visible { outline:3px solid var(--brand); outline-offset:3px; }
.music-icon-button,.music-text-button,.music-track,.music-tabs button,.music-console__cover,.music-lyrics button { border:0; background:transparent; color:var(--text); }
.music-icon-button { display:inline-flex; align-items:center; justify-content:center; min-width:36px; min-height:40px; border-radius:var(--radius-md); text-decoration:none; flex-shrink:0; }
.music-icon-button:hover,.music-text-button:hover { background:var(--music-tint); } [aria-pressed=true] { color:var(--text); background:var(--music-tint); }
.music-text-button { display:flex; align-items:center; gap:8px; padding:8px; border-radius:var(--radius-md); text-align:start; }
.music-tabs { border-bottom:1px solid var(--border); margin-bottom:24px; gap:24px; overflow-x:auto; }
.music-tabs button { display:flex; align-items:center; gap:8px; flex-shrink:0; padding:12px 0; border-bottom:3px solid transparent; }
.music-tabs [aria-current=page] { border-bottom-color:var(--music-accent); font-weight:600; } .music-tabs span { font-size:12px; color:var(--muted); }
.music-layout { display:grid; grid-template-columns:minmax(0,1fr) minmax(260px,310px); gap:28px; align-items:start; }
.music-library { min-width:0; } .music-library__hero { display:flex; align-items:center; justify-content:space-between; gap:16px; flex-wrap:wrap; padding:20px; background:linear-gradient(125deg,var(--music-tint),var(--surface)); border:1px solid var(--border); border-radius:var(--radius-lg); }
.music-library__hero h2 { font-size:28px; } .music-library__hero p { margin:8px 0 0; color:var(--muted); } .music-library__hero-actions { display:flex; gap:8px; }
.music-eyebrow { font-size:12px; font-weight:600; color:var(--muted); letter-spacing:.06em; }
.music-location { justify-content:space-between; flex-wrap:wrap; margin:12px 0; gap:8px; } .music-location__source { display:flex; align-items:center; flex-wrap:wrap; gap:8px; min-width:0; flex:1; } .music-location__source > button { min-width:0; } .music-location__actions { display:flex; align-items:center; gap:8px; flex-shrink:0; margin-left:auto; } .music-location span { overflow-wrap:anywhere; min-width:0; } .music-location svg { flex-shrink:0; }
.music-location-form { display:flex; flex-wrap:wrap; gap:8px; margin:12px 0; } .music-location-form input { flex:1; min-width:150px; }
input:not([type=range]),select { border:1px solid var(--border); border-radius:var(--radius-md); padding:10px; color:var(--text); background:var(--surface); min-width:0; }
.music-search { margin:20px 0; flex-wrap:wrap; } .music-search label { display:flex; align-items:center; gap:8px; min-width:160px; flex:1; color:var(--muted); } .music-search input { width:100%; }
.music-folders { display:grid; grid-template-columns:repeat(auto-fill,minmax(min(180px,100%),1fr)); gap:10px; margin-bottom:16px; }
.music-folders button { display:flex; align-items:center; gap:12px; padding:14px; border:1px solid var(--border); border-radius:var(--radius-md); background:var(--surface); color:var(--text); text-align:start; min-width:0; } .music-folders strong { overflow-wrap:anywhere; flex:1; } .music-folders button > span { color:var(--music-accent); }
.music-tracks,.music-queue { list-style:none; padding:0; margin:0; max-height:54vh; overflow-y:auto; scrollbar-gutter:stable; }
.music-tracks li,.music-queue li { display:flex; align-items:center; border-bottom:1px solid var(--border); border-radius:var(--radius-md); padding-inline:4px; }
.music-tracks li:hover,.music-queue li:hover { background:var(--surface-raised); } .music-tracks li.active,.music-queue li.active { background:var(--music-tint); }
.music-track { display:flex; align-items:center; gap:12px; padding:12px 0; flex:1; min-width:0; text-align:start; }
.music-track__number { width:24px; flex-shrink:0; display:flex; justify-content:center; color:var(--muted); font-size:12px; font-variant-numeric:tabular-nums; }
.active .music-track__number { color:var(--text); } .music-track__art { display:grid; place-items:center; width:44px; height:44px; flex-shrink:0; border-radius:var(--radius-sm); overflow:hidden; color:var(--music-accent); background:var(--music-tint); }
.music-track__art img { width:100%; height:100%; object-fit:cover; } .music-track__name { display:grid; min-width:0; gap:5px; flex:1; overflow-wrap:anywhere; } .music-track__name strong { font-weight:500; } .music-track__name span { font-size:13px; color:var(--muted); }
.music-track__format { font-size:12px; color:var(--muted); text-align:end; padding-inline:8px; } .music-track__format small { display:block; font-size:12px; margin-top:5px; }
.music-now { min-width:0; padding:22px; border:1px solid var(--border); border-radius:var(--radius-lg); background:var(--surface); }
.music-now__album { text-align:center; } .music-cover { width:min(100%,240px); aspect-ratio:1; margin:0 auto 22px; overflow:hidden; border-radius:var(--radius-lg); background:linear-gradient(145deg,#ecd8d7,#d6c8dc); }
.music-cover img { width:100%; height:100%; object-fit:cover; } .music-cover__record { width:100%; height:100%; display:grid; place-items:center; background:radial-gradient(circle at 50% 50%,#e9b7a3 0 12%,#523d52 12.5% 40%,transparent 40.5%); color:#f9e5dd; }
.music-now__album > p { color:var(--muted); margin:8px 0; } .music-provenance { display:flex; justify-content:center; flex-wrap:wrap; gap:8px; margin:14px 0; } .music-provenance span { font-size:12px; padding:4px 8px; border-radius:var(--radius-sm); background:var(--surface-raised); overflow-wrap:anywhere; }
.music-section-heading { justify-content:space-between; flex-wrap:wrap; } .music-caption { font-size:13px; color:var(--muted); }
.music-lyric-section { margin-top:24px; border-top:1px solid var(--border); padding-top:14px; }
.music-lyrics { position:relative; max-height:230px; overflow-y:auto; padding:14px 0; scrollbar-width:thin; }
.music-lyrics button { display:block; width:100%; text-align:center; padding:12px 8px; line-height:1.7; overflow-wrap:anywhere; color:var(--muted); border-radius:var(--radius-md); }
.music-lyrics button[aria-current=true] { color:var(--text); background:var(--music-tint); font-weight:700; } .music-lyrics p { white-space:pre-wrap; color:var(--muted); text-align:center; }
.music-layout--focus { display:block; } .music-layout--focus .music-now { display:grid; grid-template-columns:minmax(0,1fr) minmax(0,1fr); gap:40px; padding:32px; align-items:center; } .music-layout--focus .music-cover { width:min(100%,340px); } .music-layout--focus .music-lyric-section { border:0; padding:0; margin:0; } .music-layout--focus .music-lyrics { max-height:420px; } .music-layout--focus .music-lyrics button { font-size:18px; padding:18px 12px; }
.music-notice { min-height:22px; margin:12px 0 6px; color:var(--muted); font-size:13px; }
.music-console { position:sticky; bottom:0; display:grid; grid-template-columns:minmax(0,1fr) minmax(240px,1.3fr) minmax(100px,.65fr); gap:16px; align-items:center; padding:16px; background:var(--surface-raised); border:1px solid var(--border); border-radius:var(--radius-lg); z-index:2; }
.music-console__summary { display:flex; gap:12px; align-items:center; min-width:0; } .music-console__summary > div { min-width:0; flex:1; overflow-wrap:anywhere; } .music-console__summary strong { font-size:14px; display:block; } .music-console__summary span { display:block; font-size:13px; color:var(--muted); margin-top:4px; }
.music-console__cover { display:grid; place-items:center; width:48px; height:48px; flex-shrink:0; border-radius:var(--radius-md); background:var(--music-tint); overflow:hidden; } .music-console__cover img { width:100%; height:100%; object-fit:cover; }
.music-transport { justify-content:center; gap:12px; } .music-play { display:grid; place-items:center; border:0; width:46px; height:46px; border-radius:50%; color:#fff; background:var(--music-accent); flex-shrink:0; }
.music-progress { gap:10px; margin-top:8px; font-size:12px; color:var(--muted); font-variant-numeric:tabular-nums; } .music-progress input { flex:1; min-width:0; width:100%; }
input[type=range] { accent-color:var(--music-accent); cursor:pointer; min-height:24px; }
.music-mobile-volume-toggle { display:none; } .music-console__volume { display:flex; align-items:center; justify-content:end; min-width:0; } .music-console__volume input { width:70px; min-width:35px; }
.music-playback-status { grid-column:1/-1; display:flex; align-items:center; flex-wrap:wrap; justify-content:center; gap:8px; font-size:14px; } .music-playback-status[role=alert],.music-error { color:var(--danger); }
.music-help { margin-top:16px; font-size:13px; color:var(--muted); } .music-help summary { cursor:pointer; font-size:14px; } .music-help p { font-size:13px; }
.music-empty { padding:42px 16px; text-align:center; color:var(--muted); } .music-empty h2 { font-size:20px; margin-top:16px; }
@container (max-width: 900px) { .music-layout { grid-template-columns:minmax(0,1fr) 260px; gap:16px; } .music-now { padding:16px; } .music-console { grid-template-columns:minmax(0,1fr) minmax(220px,1.2fr); } .music-console__volume { grid-column:1/-1; justify-content:center; } .music-track__format small { display:none; }  }
@container (max-width: 680px) { .music-layout { display:block; } .music-now--mobile-hidden { display:none; } .music-tabs { gap:18px; margin-bottom:18px; } .music-library__hero { padding:20px; } .music-library__hero h2 { font-size:24px; } .music-track { gap:8px; } .music-track__format { padding:0 4px; } .music-console { grid-template-columns:1fr; gap:12px; padding:12px; } .music-console__volume { grid-column:auto; } .music-console__cover { width:40px; height:40px; } .music-console__summary { gap:10px; } .music-console__center { min-width:0; } .music-console__volume { display:none; } .music-console__volume.is-open { display:flex; } .music-console { position:sticky; } .music-mobile-volume-toggle { display:flex; position:absolute; right:12px; top:12px; } .music-console__summary { padding-right:40px; } .music-track__number { width:18px; } .music-tracks,.music-queue { max-height:52vh; } .music-layout--focus .music-now { display:block; padding:20px; } .music-layout--focus .music-cover { width:200px; } .music-layout--focus .music-lyric-section { margin-top:24px; } .music-layout--focus .music-lyrics { max-height:300px; } .music-layout--focus .music-lyrics button { font-size:16px; } }
</style>
