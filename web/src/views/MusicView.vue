<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { mediaLocations, setMediaLocation } from '@/lib/mediaLocations'
import { useRoute, useRouter } from 'vue-router'
import { Music2, Play, Pause, SkipBack, SkipForward, FolderOpen, Plus, RefreshCw, Download, X, Shuffle, Repeat, Repeat1, ListMusic, ArrowUp, ArrowDown, Volume2, VolumeX, Search, Disc3, ChevronRight, ListPlus, Gem } from '@lucide/vue'
import HostSwitcher from '@/components/common/HostSwitcher.vue'
import { api } from '@/lib/api'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { fileHostLabel, fileHostStatus } from '@/lib/fileHostStatus'
import { clusterHostPanelPageURL } from '@/lib/clusterHostNavigation'
import { withoutUnsupportedLightNodes } from '@/lib/nodeFeatureHosts'
import { galleryParentPath, normalizeGalleryRoot } from '@/lib/gallery'
import { MUSIC_ROOT, musicExtension, musicTime, parseLyrics, readMusicFolder, tracksFromEntries, trackKey, type Track } from '@/lib/music'
import { useMusicPlayer } from '@/stores/musicPlayer'
import { useMusicQuality } from '@/lib/useMusicQuality'
import { musicQualityKey } from '@/lib/musicQuality'
import { lookupMusicInfo, peekMusicInfo } from '@/lib/musicInfoCache'
import { readableMusicTag } from '@/lib/musicTags'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { formatBytes } from '@/lib/format'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import type { ClusterHost, FileEntry } from '@/types/api'

usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/MusicView/en-US').then(m => m.default) : import('@/i18n/pages/MusicView/zh-TW').then(m => m.default))
function phrase(value: string) { phraseCatalogVersion.value; return translatePhrase(value) }
const i18n = useI18n(), player = useMusicPlayer(), state = player.state
const currentKey = computed(() => state.current && trackKey(state.current))
const route = useRoute(), router = useRouter()
const windowVisible = inject(desktopWindowVisibleKey, undefined)
const windowed = Boolean(windowVisible)
const visible = windowVisible ?? ref(true)
const explicitLocation = computed(() => typeof route.query.path === 'string' || typeof route.query.hostId === 'string')
const hostId = computed(() => explicitLocation.value ? (typeof route.query.hostId === 'string' ? route.query.hostId : '') : mediaLocations.musicLocation?.hostId ?? '')
const path = computed(() => typeof route.query.path === 'string' && (route.query.path === '/' || normalizeGalleryRoot(route.query.path)) ? route.query.path : explicitLocation.value ? MUSIC_ROOT : mediaLocations.musicLocation?.path ?? MUSIC_ROOT)
const location = ref(''), search = ref(''), error = ref(''), loading = ref(false), truncated = ref(false)
const entries = ref<FileEntry[]>([]), hosts = ref<ClusterHost[]>([]), hostsError = ref(false)
const picker = ref<InstanceType<typeof HostSwitcher>>()
const locationInput = ref<HTMLInputElement>()
const activeHost = computed(() => hosts.value.find(h => hostId.value ? h.id === hostId.value : h.isLocal))
const hostLabel = computed(() => fileHostLabel(hostId.value, hosts.value, phrase('本机')))
const allTracks = computed(() => tracksFromEntries(entries.value, hostId.value))
const sort = ref('name'), tab = ref<'library' | 'listen' | 'queue'>(route.query.view === 'listen' ? 'listen' : 'library'), locationOpen = ref(false)
const tracks = shallowRef<Track[]>([])
const nameOrder = new Intl.Collator(undefined, { numeric: true })
// Only browsing controls rebuild the list. Metadata can still update each row in place.
watch([allTracks, sort, search], ([available, order, query]) => {
  const term = query.toLocaleLowerCase()
  const rows = (term ? available.filter(track => trackSearchText(track).includes(term)) : available)
    .map(track => ({ track, title: trackTitle(track), artist: order === 'artist' ? trackArtist(track) : '' }))
  rows.sort((a, b) => order === 'artist'
    ? a.artist.localeCompare(b.artist) || a.title.localeCompare(b.title)
    : nameOrder.compare(a.title, b.title))
  tracks.value = rows.map(row => row.track)
}, { immediate: true })
const folders = computed(() => entries.value.filter(e => e.kind === 'directory' && !e.name.startsWith('.')))
const volumeOpen = ref(false), titleExpanded = ref(false), musicRoot = ref<HTMLElement>()
const quality = useMusicQuality(musicRoot, () => [...tracks.value, ...state.queue, ...(state.current ? [state.current] : [])], () => tab.value)
function bitrate(track: Track) { const info = quality(track); return info?.bitrate ? info.bitrate + ' kbps' + (info.vbr ? ' VBR' : '') : '' }
function qualityTitle(track: Track) {
  const info = quality(track)
  if (!info) return ''
  const details = [info.bitsPerSample ? info.bitsPerSample + ' bit' : '', info.sampleRate ? info.sampleRate / 1000 + ' kHz' : ''].filter(Boolean).join(' / ')
  return [info.bitrateKind === 'frame' ? phrase('采样音频帧码率，可能随播放变化') : info.bitrateKind === 'average' ? phrase('按文件大小与时长计算的平均码率') : '', details].filter(Boolean).join(' · ')
}
const coverFailed = ref(false), lyricPanel = ref<HTMLElement>(), following = ref(true)
const lyrics = computed(() => parseLyrics(state.lyrics))
const activeLine = computed(() => { let active = -1; lyrics.value.forEach((line, index) => { if (line.time <= state.position) active = index }); return active })
const currentHost = computed(() => fileHostLabel(state.current?.hostId, hosts.value, phrase('本机')))
const repeatLabel = computed(() => state.repeat === 'off' ? phrase('顺序播放') : state.repeat === 'all' ? phrase('列表循环') : phrase('单曲循环'))
const notice = computed(() => state.notice === 'next' ? phrase('已设为下一首') : state.notice === 'added' ? phrase('已加入播放队列') : state.notice === 'full' ? phrase('队列已满，请先移除部分歌曲') : '')
const onlineNotice = computed(() => state.onlineStatus === 'loading' ? phrase('正在在线匹配歌词与封面…') : state.onlineStatus === 'matched' ? phrase(state.onlineSource === 'qq' ? '在线补全来自 QQ 音乐' : '在线补全来自网易云音乐') : state.onlineStatus === 'empty' ? phrase('未找到匹配的在线歌词或封面') : state.onlineStatus === 'failed' ? phrase('在线匹配暂不可用，重新播放可重试') : '')
const saveNotice = computed(() => {
  const messages = { saving: '正在保存在线内容…', saved: '已保存到歌曲目录', kept: '同名文件已存在，已保留', partial: '部分内容保存失败', failed: '保存失败，请检查目录权限或连接' }
  return state.saveStatus ? phrase(messages[state.saveStatus]) : ''
})
const sourceShort = computed(() => {
  if (state.onlineStatus === 'matched') return state.onlineSource === 'qq' ? 'QQ' : phrase('网易云')
  return state.onlineStatus === 'loading' ? phrase('匹配中') : state.onlineStatus === 'empty' ? phrase('未匹配') : state.onlineStatus === 'failed' ? phrase('暂不可用') : ''
})
const saveShort = computed(() => {
  const labels = { saving: '保存中', saved: '已保存', kept: '已保留', partial: '部分失败', failed: '保存失败' }
  return state.saveStatus ? phrase(labels[state.saveStatus]) : ''
})
const playbackError = computed(() => state.error === 'blocked' ? phrase('浏览器暂停了自动播放，点击播放继续') : state.error === 'network' ? phrase('音乐连接中断，请检查来源主机后重试') : state.error ? phrase('当前浏览器无法解码此音频，可下载原文件播放') : '')
let listing: AbortController | undefined, destroyed = false
async function loadHosts() {
  try { const result = await api.cluster.hosts(); if (!destroyed) { hosts.value = withoutUnsupportedLightNodes(result.items); hostsError.value = false } }
  catch { if (!destroyed) hostsError.value = true }
}
async function load() {
  listing?.abort(); const controller = new AbortController(); listing = controller
  entries.value = []; error.value = ''; loading.value = true; truncated.value = false
  try { const result = await readMusicFolder(path.value, hostId.value, controller.signal); if (!controller.signal.aborted) {
      const budget = new AbortController(), stop = () => budget.abort()
      controller.signal.addEventListener('abort', stop, { once: true })
      const timer = setTimeout(stop, 1200)
      try { await lookupMusicInfo(tracksFromEntries(result.entries, hostId.value), budget.signal) } finally { clearTimeout(timer); controller.signal.removeEventListener('abort', stop) }
      if (!controller.signal.aborted) { entries.value = result.entries; truncated.value = result.truncated }
    } }
  catch (e) { if (!controller.signal.aborted) error.value = e instanceof Error ? e.message : String(e) }
  finally { if (!controller.signal.aborted) loading.value = false }
}
function navigate(target: string, host = hostId.value) {
  setMediaLocation('musicLocation', target, host)
  void router.push({ name: 'music', query: { path: target, ...(host ? { hostId: host } : {}) } })
}
function openLocation() {
  const target = location.value === '/' ? '/' : normalizeGalleryRoot(location.value)
  if (!target) { error.value = phrase('请输入有效的绝对路径'); return }
  navigate(target)
}
async function focusLocation() {
  locationOpen.value = true
  await nextTick()
  locationInput.value?.focus()
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
function isCurrent(track: Track) { return currentKey.value === trackKey(track) }
function trackTitle(track: Track) { return readableMusicTag(peekMusicInfo(track)?.tags?.title, isCurrent(track) ? state.current!.title : track.title) }
function trackArtist(track: Track) { return readableMusicTag(peekMusicInfo(track)?.tags?.artist, isCurrent(track) ? state.current!.artist : track.artist) }
function trackSearchText(track: Track) {
  const tags = peekMusicInfo(track)?.tags
  return [track.entry.name, track.title, track.artist, tags?.title, tags?.artist].filter(Boolean).join(' ').toLocaleLowerCase()
}
function choose(track: Track) { if (!state.queue.length) player.replaceQueue(tracks.value, track); else player.choose(track) }
function seek(event: Event) { player.seek(Number((event.target as HTMLInputElement).value)) }
function volume(event: Event) { player.setVolume(Number((event.target as HTMLInputElement).value)) }
function artwork(track: Track) { return (isCurrent(track) && player.cover.value) || (track.cover ? fileAPIForHost(track.hostId).contentUrl(track.cover) : '') }
function trackRowInputs(track: Track, index: number) {
  const active = isCurrent(track)
  return [track, track.entry, index, peekMusicInfo(track), quality(track), phraseCatalogVersion.value, active,
    active && state.current?.title, active && state.current?.artist, active && player.cover.value,
    active && state.playing, active && state.waiting, active && visible.value]
}
function keyboard(event: KeyboardEvent) {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
  const target = event.target as HTMLElement
  if (target.closest('input, textarea, select, button, a, [contenteditable="true"]')) return
  if (event.code === 'Space') { event.preventDefault(); player.toggle() }
  else if (event.key === 'ArrowRight') { event.preventDefault(); player.seek(state.position + 5) }
  else if (event.key === 'ArrowLeft') { event.preventDefault(); player.seek(state.position - 5) }
}
let followedScrollTop = 0
let lyricResize: ResizeObserver | undefined
function lyricScrolled() {
  if (tab.value !== 'listen') { void followLyrics(); return }
  if (following.value && lyricPanel.value && Math.abs(lyricPanel.value.scrollTop - followedScrollTop) > 1) following.value = false
}
function seekLyric(time: number) { following.value = true; player.seek(time); void followLyrics() }
async function followLyrics() {
  if (tab.value === 'listen' && !following.value) return
  await nextTick()
  if (tab.value !== 'listen') {
    const sidebar = lyricPanel.value?.closest<HTMLElement>('.music-now')
    const content = sidebar?.closest<HTMLElement>('.music-content')
    if (sidebar && content) {
      const bounds = content.getBoundingClientRect()
      const height = `${Math.max(0, bounds.bottom - Math.max(bounds.top, sidebar.getBoundingClientRect().top))}px`
      if (sidebar.style.getPropertyValue('--music-sidebar-height') !== height) sidebar.style.setProperty('--music-sidebar-height', height)
    }
  }
  const panel = lyricPanel.value, line = panel?.querySelector<HTMLElement>('[aria-current="true"]')
  if (panel && line && panel.clientHeight > 0) {
    panel.scrollTop = Math.max(0, line.offsetTop - panel.clientHeight / 2 + line.clientHeight / 2)
    followedScrollTop = panel.scrollTop
  }
}
watch([lyrics, activeLine, tab, following], followLyrics)
watch(() => route.query.view, view => { if (view === 'listen') tab.value = 'listen' })
watch(() => state.current && trackKey(state.current), () => { following.value = true; followedScrollTop = 0; coverFailed.value = false; titleExpanded.value = false })
watch(() => player.cover.value, () => { coverFailed.value = false })
watch([path, hostId], () => { location.value = path.value; search.value = ''; void load() }, { immediate: true })
watch(visible, (value, previous) => { state.viewCount += Number(value) - Number(previous ?? false) }, { immediate: true })
onMounted(() => {
  void loadHosts()
  if (typeof ResizeObserver !== 'undefined' && lyricPanel.value) {
    lyricResize = new ResizeObserver(() => { void followLyrics() })
    lyricResize.observe(lyricPanel.value)
    const content = lyricPanel.value.closest('.music-content')
    if (content) lyricResize.observe(content)
  }
})
onBeforeUnmount(() => { destroyed = true; listing?.abort(); lyricResize?.disconnect(); if (visible.value) state.viewCount = Math.max(0, state.viewCount - 1) })
</script>

<template>
  <main ref="musicRoot" class="page music-page" :class="{ 'music-page--focus': tab === 'listen' }" tabindex="0" @keydown="keyboard">
    <div class="music-content" @scroll.passive="followLyrics">
    <header v-if="!windowed" class="music-header">
      <div class="music-brand"><img :src="'/desktop-icons/music-kpanel-flat-v2.webp'" alt="" /><div><h1>音乐</h1><p>把日常，交给一首好歌</p></div></div>
    </header>
    <h1 v-else class="sr-only">{{ i18n.t('route.music') }}</h1>
    <nav class="music-tabs" :aria-label="phrase('音乐导航')"><button :aria-current="tab === 'library' ? 'page' : undefined" @click="tab = 'library'"><Disc3 :size="18" /> 音乐库</button><button :aria-current="tab === 'listen' ? 'page' : undefined" @click="tab = 'listen'"><Music2 :size="18" /> 正在播放</button><button :aria-current="tab === 'queue' ? 'page' : undefined" @click="tab = 'queue'"><ListMusic :size="18" /> 播放队列 <span>{{ state.queue.length }}</span></button></nav>
    <div class="music-layout" :class="{ 'music-layout--focus': tab === 'listen' }">
      <section v-if="tab !== 'listen'" class="music-library">
        <template v-if="tab === 'library'">
          <div class="music-location"><div class="music-location__source"><HostSwitcher ref="picker" :hosts="hosts" :active-id="activeHost?.id || hostId" :label="hostLabel" :error="hostsError" :status-of="h => ({ ...fileHostStatus(h), label: phrase(fileHostStatus(h).label) })" @select="selectHost" @refresh="loadHosts" /><button class="music-text-button" :title="path" @click="locationOpen = !locationOpen" :aria-expanded="locationOpen"><FolderOpen :size="16" /><span>{{ path }}</span><ChevronRight :size="15" /></button></div><div class="music-location__actions"><button class="button button--secondary" :aria-label="phrase('刷新')" :disabled="loading" @click="load"><RefreshCw :size="17" /></button><button class="music-text-button" @click="files">文件管理</button></div></div>
          <form v-if="locationOpen" class="music-location-form" @submit.prevent="openLocation"><input ref="locationInput" v-model="location" :aria-label="phrase('音乐文件夹')" spellcheck="false" /><button class="button button--secondary" type="submit">打开</button><button class="button button--secondary" type="button" :disabled="path === '/'" @click="navigate(galleryParentPath(path))">上级文件夹</button></form>
          <div class="music-library__hero"><div class="music-library__hero-info"><h2>{{ path.split('/').filter(Boolean).pop() || phrase('音乐库') }}</h2><p>{{ allTracks.length }} {{ phrase('首歌曲') }} <span v-if="folders.length"> · {{ folders.length }} {{ phrase('个文件夹') }}</span></p></div><div class="music-library__hero-actions"><button class="button button--primary" :disabled="!tracks.length" @click="player.shuffle(false); player.replaceQueue(tracks)"><Play :size="17" /> 播放全部</button><button class="music-icon-button" :aria-label="phrase('随机播放全部')" :disabled="!tracks.length" @click="player.shuffle(true); player.replaceQueue(tracks, tracks[Math.floor(Math.random() * tracks.length)])"><Shuffle :size="19" /></button></div></div>
          <div class="music-search"><label><Search :size="18" /><input v-model="search" type="search" :placeholder="phrase('搜索歌曲或歌手')" :aria-label="phrase('搜索歌曲或歌手')" /></label><select v-model="sort" :aria-label="phrase('歌曲排序')"><option value="name">按歌名</option><option value="artist">按歌手</option></select></div>
          <p v-if="loading" role="status" class="music-empty">正在读取音乐…</p>
          <div v-else-if="error" role="alert" class="music-error"><p>{{ error }}</p><p>检查主机连接和文件夹路径后重试。也可从文件管理上传音乐。</p><button class="button button--secondary" @click="load">重试</button></div>
          <template v-else>
            <p v-if="truncated" role="status">仅显示部分文件，请进入子文件夹继续浏览。</p>
            <div v-if="folders.length && !search" class="music-folders"><button v-for="folder in folders" :key="folder.path" @click="navigate(folder.path)"><span><Disc3 :size="25" /></span><strong>{{ folder.name }}</strong><ChevronRight :size="16" /></button></div>
            <div v-if="!tracks.length" class="music-empty"><Disc3 :size="48" /><h2>{{ search ? phrase('没有匹配的歌曲') : phrase('让音乐住进这里') }}</h2><p>{{ search ? phrase('试试其他歌名或歌手') : phrase('选择音乐文件夹，或通过文件管理上传歌曲。') }}</p><button v-if="search" class="button button--secondary" @click="search = ''">清除搜索</button><button v-else class="button button--secondary" @click="focusLocation">选择文件夹</button></div>
             <ol class="music-tracks" :aria-label="phrase('歌曲列表')"><li v-for="(track, index) in tracks" :key="trackKey(track)" v-memo="trackRowInputs(track, index)" :class="{ active: isCurrent(track) }">
              <button class="music-track" :aria-label="`${phrase('播放')} ${trackTitle(track)}`" :aria-description="trackArtist(track) || phrase('未知歌手')" @click="choose(track)">
                <span class="music-track__number"><span v-if="isCurrent(track) && state.playing" class="music-wave" :class="{ 'is-playing': !state.waiting && visible }" aria-hidden="true"><i></i><i></i><i></i></span><Play v-else-if="isCurrent(track)" :size="16" /><template v-else>{{ String(index + 1).padStart(2, '0') }}</template></span>
                <span class="music-track__art"><img v-if="artwork(track)" :key="artwork(track)" :src="artwork(track)" alt="" loading="lazy" @error="($event.target as HTMLImageElement).hidden = true" /><Music2 v-else :size="21" /></span>
                <span class="music-track__name"><strong :title="trackTitle(track)">{{ trackTitle(track) }}</strong><span :title="trackArtist(track)">{{ trackArtist(track) || phrase('未知歌手') }}</span></span>
                <span class="music-track__format" :data-music-quality="musicQualityKey(track)" :title="qualityTitle(track)">
                  <span class="music-format-line">
                    <span>{{ musicExtension(track.entry.name).toUpperCase() }}</span>
                    <span class="music-bitrate" :title="bitrate(track)">{{ bitrate(track) }}</span>
                    <span v-if="quality(track)?.lossless" class="music-lossless"><Gem :size="13" aria-hidden="true" />{{ phrase('无损') }}</span>
                  </span>
                  <small>{{ formatBytes(track.entry.sizeBytes) }}</small>
                </span>
              </button>
              <button class="music-icon-button music-next" :aria-label="`${phrase('下一首播放')} ${trackTitle(track)}`" :title="phrase('下一首播放')" @click="player.enqueue(track, true)"><ListPlus :size="18" /></button><button class="music-icon-button" :aria-label="`${phrase('加入播放队列')} ${trackTitle(track)}`" :title="phrase('加入播放队列')" @click="player.enqueue(track)"><Plus :size="18" /></button>
            </li></ol>
          </template>
        </template>
        <template v-else>
          <div class="music-section-heading"><div><span class="music-eyebrow">接下来听什么</span><h2>播放队列</h2></div><button class="button button--secondary" :disabled="!state.queue.length" @click="player.clearQueue">清空待播</button></div>
          <p class="music-caption">跨主机歌曲可以放在同一队列。移除歌曲不会打断当前播放。</p>
          <div v-if="!state.queue.length" class="music-empty"><ListMusic :size="44" /><h2>队列还是空的</h2><button class="button button--secondary" @click="tab = 'library'">去音乐库选歌</button></div>
          <ol class="music-queue"><li v-for="(track, index) in state.queue" :key="trackKey(track)" :class="{ active: isCurrent(track) }"><button class="music-track" :data-music-quality="musicQualityKey(track)" :aria-label="`${phrase('播放')} ${trackTitle(track)}`" @click="player.choose(track)"><span class="music-track__number">{{ index + 1 }}</span><span class="music-track__art"><img v-if="artwork(track)" :key="artwork(track)" :src="artwork(track)" alt="" loading="lazy" @error="($event.target as HTMLImageElement).hidden = true" /><Music2 v-else :size="21" /></span><span class="music-track__name"><strong>{{ trackTitle(track) }}</strong><span>{{ trackArtist(track) || phrase('未知歌手') }} · {{ fileHostLabel(track.hostId, hosts, phrase('本机')) }}</span></span><Play v-if="isCurrent(track)" :size="16" /></button><button class="music-icon-button" :disabled="index === 0" :aria-label="`${phrase('上移')} ${trackTitle(track)}`" @click="player.move(track, -1)"><ArrowUp :size="16" /></button><button class="music-icon-button" :disabled="index === state.queue.length - 1" :aria-label="`${phrase('下移')} ${trackTitle(track)}`" @click="player.move(track, 1)"><ArrowDown :size="16" /></button><button class="music-icon-button" :aria-label="`${phrase('移出队列')} ${trackTitle(track)}`" @click="player.remove(track)"><X :size="17" /></button></li></ol>
        </template>
      </section>
      <aside class="music-now" :class="{ 'music-now--mobile-hidden': tab !== 'listen' }">
        <div class="music-now__album"><div class="music-cover"><img v-if="player.cover.value && !coverFailed" :src="player.cover.value" alt="" @error="coverFailed = true" /><div v-else class="music-cover__record"><Disc3 :size="96" /></div></div><span class="music-eyebrow"><span v-if="state.playing" class="music-wave" :class="{ 'is-playing': !state.waiting && visible }" aria-hidden="true"><i></i><i></i><i></i></span> {{ state.current ? phrase(state.playing ? '正在播放' : '暂停') : phrase('准备好，听点喜欢的') }}</span><h2>{{ (state.current ? trackTitle(state.current) : '') || phrase('你的下一首好歌') }}</h2><p>{{ (state.current ? trackArtist(state.current) : '') || phrase('从音乐库开始聆听') }}</p><p v-if="state.album" class="music-caption">{{ state.album }}</p><div v-if="state.current" class="music-provenance" :data-music-quality="musicQualityKey(state.current)" :title="qualityTitle(state.current)"><span>{{ musicExtension(state.current.entry.name).toUpperCase() }} · {{ phrase('原文件') }}</span><span v-if="bitrate(state.current)">{{ bitrate(state.current) }}</span><span v-if="quality(state.current)?.lossless" class="music-lossless"><Gem :size="13" aria-hidden="true" />{{ phrase('无损') }}</span><span>{{ currentHost }}</span></div></div>
        <section class="music-lyric-section"><div class="music-section-heading"><div class="music-lyric-title"><h3>歌词</h3><span v-if="sourceShort" class="music-source-label" :title="onlineNotice" :aria-label="onlineNotice" role="status">{{ sourceShort }}</span><span v-if="saveShort" class="music-source-label" :title="saveNotice" :aria-label="saveNotice" role="status">{{ saveShort }}</span><button v-if="state.saveStatus === 'failed' || state.saveStatus === 'partial'" class="music-save-retry" @click="player.retrySave">{{ phrase('重试保存') }}</button></div><button v-if="tab === 'listen' && lyrics.length" class="music-text-button" :aria-pressed="following" @click="following = !following">{{ following ? phrase('跟随播放') : phrase('恢复跟随') }}</button></div><div ref="lyricPanel" class="music-lyrics" @scroll.passive="lyricScrolled"><p v-if="state.lyricsLoading" role="status">正在读取歌词…</p><p v-else-if="state.lyricsFailed" role="status">歌词读取失败，可重新播放以重试</p><template v-else-if="lyrics.length"><button v-for="(line, index) in lyrics" :key="index" :aria-current="index === activeLine ? 'true' : undefined" @click="seekLyric(line.time)">{{ line.text || '♪' }}</button></template><p v-else>{{ state.lyrics || phrase('暂无歌词：优先读取本地内容，再尝试在线匹配') }}</p></div></section>
      </aside>
    </div>
    <p class="music-notice" role="status" aria-live="polite">{{ notice }}</p>
    </div>
    <section class="music-console" :aria-label="phrase('播放控制')">
      <div class="music-console__summary"><button class="music-console__cover" :aria-label="phrase('查看正在播放')" @click="tab = tab === 'listen' ? 'library' : 'listen'"><img v-if="player.cover.value && !coverFailed" :src="player.cover.value" alt="" @error="coverFailed = true" /><Music2 v-else :size="23" /></button><button class="music-console__title" :class="{ 'is-expanded': titleExpanded }" :aria-expanded="titleExpanded" :aria-label="`${phrase('查看正在播放')} ${(state.current ? trackTitle(state.current) : '') || ''}`" @click="tab = 'listen'; titleExpanded = !titleExpanded"><strong>{{ (state.current ? trackTitle(state.current) : '') || phrase('还没有播放歌曲') }}</strong><span>{{ (state.current ? trackArtist(state.current) : '') || phrase('选一首喜欢的歌') }}</span></button><a v-if="state.current" class="music-icon-button" :href="fileAPIForHost(state.current.hostId).contentUrl(state.current.entry.path, 'attachment')" :aria-label="phrase('下载原文件')"><Download :size="17" /></a></div>
      <button class="music-icon-button music-mobile-volume-toggle" :aria-label="phrase('音量设置')" :aria-expanded="volumeOpen" @click="volumeOpen = !volumeOpen"><Volume2 :size="19" /></button><div class="music-console__center"><div class="music-transport"><button class="music-icon-button" :aria-label="phrase('随机播放')" :aria-pressed="state.shuffle" @click="player.shuffle()"><Shuffle :size="18" /></button><button class="music-icon-button" :disabled="!state.current" :aria-label="phrase('上一首')" @click="player.previous"><SkipBack :size="20" /></button><button class="music-play" :disabled="!state.current" :aria-label="state.playing || state.waiting ? phrase('暂停') : phrase('播放')" @click="player.toggle"><Pause v-if="state.playing || state.waiting" :size="24" /><Play v-else :size="24" /></button><button class="music-icon-button" :disabled="!state.queue.length" :aria-label="phrase('下一首')" @click="player.next()"><SkipForward :size="20" /></button><button class="music-icon-button" :aria-label="repeatLabel" :title="repeatLabel" :aria-pressed="state.repeat !== 'off'" @click="repeat"><Repeat1 v-if="state.repeat === 'one'" :size="18" /><Repeat v-else :size="18" /></button></div><div class="music-progress"><span>{{ musicTime(state.position) }}</span><input type="range" min="0" :max="state.duration || 0" step="0.1" :value="state.position" :disabled="!state.duration" :aria-label="phrase('播放进度')" @input="seek" /><span>{{ musicTime(state.duration) }}</span></div></div>
      <div class="music-console__volume" :class="{ 'is-open': volumeOpen }"><button class="music-icon-button" :aria-label="state.muted ? phrase('取消静音') : phrase('静音')" :aria-pressed="state.muted" @click="player.mute"><VolumeX v-if="state.muted || state.volume === 0" :size="19" /><Volume2 v-else :size="19" /></button><input type="range" min="0" max="1" step="0.01" :value="state.muted ? 0 : state.volume" :aria-label="phrase('音量')" @input="volume" /><button class="music-icon-button" :aria-label="phrase('播放队列')" :aria-pressed="tab === 'queue'" @click="tab = tab === 'queue' ? 'library' : 'queue'"><ListMusic :size="20" /></button><button class="music-icon-button" :disabled="!state.current" :aria-label="phrase('停止播放')" @click="player.stop"><X :size="18" /></button></div>
      <div v-if="state.waiting || playbackError" class="music-playback-status" :role="playbackError ? 'alert' : 'status'"><span>{{ playbackError || phrase('正在缓冲…') }}</span><button v-if="playbackError && state.current" class="music-text-button" @click="player.retry()">重试</button><button v-if="playbackError && state.queue.length > 1" class="music-text-button" @click="player.next()">跳过此曲</button></div>
    </section>
  </main>
</template>

<style scoped>
.music-page { display:flex; flex-direction:column; gap:0; width:100%; height:100%; min-height:0; --music-page-padding:clamp(16px, 2vw, 24px); --music-accent: #ad315e; --music-tint: color-mix(in srgb, var(--music-accent) 8%, var(--surface)); container-type: size; color: var(--text); }
:global(.app-shell__main:has(> .page-content > .music-page)) { display:flex; flex-direction:column; height:100dvh; min-height:0; overflow:hidden; }
:global(.app-shell__main:has(> .page-content > .music-page) > .topbar) { flex-shrink:0; }
:global(.page-content:has(> .music-page)) { display:flex; flex:1 1 0; min-height:0; }
:global(.desktop-window__body:has(> .music-page)) { overflow:hidden; scrollbar-gutter:auto; }
.music-content { flex:1 1 auto; min-height:0; margin-inline-end:calc(-1 * var(--music-page-padding)); padding-inline-end:var(--music-page-padding); overflow-y:auto; overscroll-behavior:contain; scrollbar-gutter:stable; }
.music-page {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: var(--music-page-padding);
}
.music-page:focus-visible { outline: 2px solid var(--brand); outline-offset: -2px; }
.music-header,.music-brand,.music-tabs,.music-location,.music-section-heading,.music-search,.music-transport,.music-progress { display:flex; align-items:center; gap:12px; }
.music-header { justify-content:space-between; flex-wrap:wrap; margin-bottom:16px; }
.music-brand img { width:52px; height:52px; border-radius:22%; } h1 { font-size:26px; margin:0; } h2 { font-size:22px; line-height:1.35; margin:8px 0; overflow-wrap:anywhere; } h3 { font-size:16px; margin:0; }
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
.music-library { min-width:0; } .music-library__hero { display:flex; align-items:center; justify-content:space-between; gap:8px 16px; flex-wrap:wrap; padding:10px 14px; background:linear-gradient(125deg,var(--music-tint),var(--surface)); border:1px solid var(--border); border-radius:var(--radius-lg); }
.music-library__hero-info { display:flex; flex:1 1 110px; flex-wrap:wrap; align-items:baseline; gap:4px 12px; min-width:0; } .music-library__hero h2 { font-size:18px; overflow-wrap:anywhere; } .music-library__hero p { margin:0; color:var(--muted); font-size:13px; } .music-library__hero-actions { display:flex; gap:8px; }
.music-eyebrow { font-size:12px; font-weight:600; color:var(--muted); letter-spacing:.06em; }
.music-location { justify-content:space-between; flex-wrap:wrap; margin:12px 0; gap:8px; } .music-location__source { display:flex; align-items:center; flex-wrap:wrap; gap:8px; min-width:0; flex:1; } .music-location__source > button { min-width:0; } .music-location__actions { display:flex; align-items:center; gap:8px; flex-shrink:0; margin-left:auto; } .music-location span { overflow-wrap:anywhere; min-width:0; } .music-location svg { flex-shrink:0; }
.music-location-form { display:flex; flex-wrap:wrap; gap:8px; margin:12px 0; } .music-location-form input { flex:1; min-width:150px; }
input:not([type=range]),select { border:1px solid var(--border); border-radius:var(--radius-md); padding:10px; color:var(--text); background:var(--surface); min-width:0; }
.music-search { margin:20px 0; flex-wrap:wrap; } .music-search label { display:flex; align-items:center; gap:8px; min-width:160px; flex:1; color:var(--muted); } .music-search input { width:100%; }
.music-folders { display:grid; grid-template-columns:repeat(auto-fill,minmax(min(180px,100%),1fr)); gap:10px; margin-bottom:16px; }
.music-folders button { display:flex; align-items:center; gap:12px; padding:14px; border:1px solid var(--border); border-radius:var(--radius-md); background:var(--surface); color:var(--text); text-align:start; min-width:0; } .music-folders strong { overflow-wrap:anywhere; flex:1; } .music-folders button > span { color:var(--music-accent); }
.music-tracks,.music-queue { list-style:none; padding:0; margin:0; max-height:54vh; overflow-y:auto; scrollbar-gutter:stable; }
.music-tracks,.music-queue { background:var(--surface); border:1px solid var(--border); border-radius:var(--radius-lg); padding:4px; }
.music-tracks li,.music-queue li { display:flex; align-items:center; border-bottom:1px solid var(--border); border-radius:var(--radius-md); padding-inline:4px; }
.music-tracks li:hover,.music-queue li:hover { background:var(--surface-raised); } .music-tracks li.active,.music-queue li.active { background:var(--music-tint); }
.music-tracks li.active,.music-queue li.active { box-shadow:inset 3px 0 var(--music-accent); }
.music-track { display:flex; align-items:center; gap:12px; padding:12px 0; flex:1; min-width:0; text-align:start; }
.music-track__number { width:24px; flex-shrink:0; display:flex; justify-content:center; color:var(--muted); font-size:12px; font-variant-numeric:tabular-nums; }
.music-wave { display:inline-flex; align-items:end; justify-content:center; gap:2px; width:16px; height:16px; color:var(--music-accent); vertical-align:middle; }
.music-wave i { width:3px; height:13px; border-radius:999px; background:currentColor; transform:scaleY(.35); transform-origin:bottom; }
.music-wave.is-playing i { animation:music-wave calc(var(--motion-duration-ambient) * 2.4) ease-in-out infinite alternate; }
.music-wave.is-playing i:nth-child(2) { animation-delay:calc(var(--motion-duration-ambient) * -1); } .music-wave.is-playing i:nth-child(3) { animation-delay:calc(var(--motion-duration-ambient) * -1.6); }
@keyframes music-wave { from { transform:scaleY(.3); } to { transform:scaleY(1); } }
@media (prefers-reduced-motion: reduce) { .music-wave.is-playing i { animation:none; transform:scaleY(.65); } }
.active .music-track__number { color:var(--text); } .music-track__art { display:grid; place-items:center; width:44px; height:44px; flex-shrink:0; border-radius:var(--radius-sm); overflow:hidden; color:var(--music-accent); background:var(--music-tint); }
.music-track__art img { width:100%; height:100%; object-fit:cover; } .music-track__name { display:grid; min-width:0; gap:5px; flex:1; overflow-wrap:anywhere; } .music-track__name strong { font-weight:500; } .music-track__name span { font-size:13px; color:var(--muted); }
.music-track__format { font-size:12px; color:var(--muted); text-align:end; padding-inline:8px; } .music-track__format small { display:block; font-size:12px; margin-top:5px; }
.music-track__number,.music-track__name span,.music-track__format { color:var(--text-soft); }
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
.music-console { position:sticky; bottom:0; flex-shrink:0; display:grid; grid-template-columns:minmax(0,1fr) minmax(240px,1.3fr) minmax(100px,.65fr); gap:16px; align-items:center; padding:16px; background:var(--surface-raised); border:1px solid var(--border); border-radius:var(--radius-lg); z-index:2; }
.music-console__summary { display:flex; gap:12px; align-items:center; min-width:0; } .music-console__summary > div { min-width:0; flex:1; overflow-wrap:anywhere; } .music-console__summary strong { font-size:14px; display:block; } .music-console__summary span { display:block; font-size:13px; color:var(--muted); margin-top:4px; }
.music-console__cover { display:grid; place-items:center; width:48px; height:48px; flex-shrink:0; border-radius:var(--radius-md); background:var(--music-tint); overflow:hidden; } .music-console__cover img { width:100%; height:100%; object-fit:cover; }
.music-transport { justify-content:center; gap:12px; } .music-play { display:grid; place-items:center; border:0; width:46px; height:46px; border-radius:50%; color:#fff; background:var(--music-accent); flex-shrink:0; }
.music-progress { gap:10px; margin-top:8px; font-size:12px; color:var(--muted); font-variant-numeric:tabular-nums; } .music-progress input { flex:1; min-width:0; width:100%; }
input[type=range] { accent-color:var(--music-accent); cursor:pointer; min-height:24px; }
.music-mobile-volume-toggle { display:none; } .music-console__volume { display:flex; align-items:center; justify-content:end; min-width:0; } .music-console__volume input { width:70px; min-width:35px; }
.music-playback-status { position:absolute; bottom:calc(100% + 8px); left:50%; transform:translateX(-50%); max-width:calc(100% - 24px); display:flex; align-items:center; flex-wrap:wrap; justify-content:center; gap:8px; padding:8px 12px; background:var(--surface-raised); border:1px solid var(--border); border-radius:var(--radius-md); box-shadow:var(--shadow-sm); font-size:14px; } .music-playback-status[role=alert],.music-error { color:var(--danger); }
.music-empty { padding:42px 16px; text-align:center; color:var(--muted); } .music-empty h2 { font-size:20px; margin-top:16px; }
@container (max-width: 900px) { .music-layout { grid-template-columns:minmax(0,1fr) 260px; gap:16px; } .music-now { padding:16px; } .music-console { grid-template-columns:minmax(0,1fr) minmax(220px,1.2fr); } .music-console__volume { grid-column:1/-1; justify-content:center; } .music-track__format small { display:none; }  }
@container (max-width: 680px) { .music-layout { display:block; } .music-now--mobile-hidden { display:none; } .music-tabs { gap:18px; margin-bottom:18px; } .music-track { gap:8px; } .music-track__format { padding:0 4px; } .music-console { grid-template-columns:1fr; gap:12px; padding:12px; } .music-console__volume { grid-column:auto; } .music-console__cover { width:40px; height:40px; } .music-console__summary { gap:10px; } .music-console__center { min-width:0; } .music-console__volume { display:none; } .music-console__volume.is-open { display:flex; } .music-console { position:sticky; } .music-mobile-volume-toggle { display:flex; position:absolute; right:12px; top:12px; } .music-console__summary { padding-right:40px; } .music-track__number { width:18px; } .music-tracks,.music-queue { max-height:52vh; } .music-layout--focus .music-now { display:block; padding:20px; } .music-layout--focus .music-cover { width:200px; } .music-layout--focus .music-lyric-section { margin-top:24px; } .music-layout--focus .music-lyrics { max-height:300px; } .music-layout--focus .music-lyrics button { font-size:16px; } }

/* Keep the listening workspace inside its allotted window; lyrics own scrolling. */
.music-page--focus { container-type:size; }
.music-page--focus .music-content { display:flex; flex-direction:column; overflow:hidden; margin-inline-end:0; padding-inline-end:0; }
.music-page--focus .music-header,.music-page--focus .music-tabs,.music-page--focus .music-notice { flex-shrink:0; }
.music-page--focus .music-tabs { margin-bottom:16px; }
.music-page--focus .music-notice:empty { display:none; }
.music-page--focus .music-layout--focus { display:flex; flex:1 1 0; min-height:0; }
.music-page--focus .music-now { width:100%; height:100%; min-height:0; grid-template-rows:minmax(0,1fr); align-items:stretch; gap:24px; padding:clamp(12px,2cqi,24px); overflow:hidden; }
.music-page--focus .music-now__album { display:grid; grid-template-rows:minmax(0,1fr); grid-auto-rows:max-content; justify-items:center; min-height:0; overflow-y:auto; overscroll-behavior:contain; }
.music-page--focus .music-cover { width:auto; height:100%; max-width:100%; max-height:340px; aspect-ratio:1; margin:0 auto 8px; }
.music-page--focus .music-lyric-section { display:flex; flex-direction:column; min-height:0; margin:0; }
.music-page--focus .music-lyric-section > .music-caption,.music-page--focus .music-section-heading { flex-shrink:0; }
.music-page--focus .music-lyrics { flex:1 1 0; min-height:0; max-height:none; overscroll-behavior:contain; }
@container (max-width:680px) {
  .music-page--focus .music-now { display:flex; flex-direction:column; gap:12px; }
  .music-page--focus .music-now__album { display:grid; grid-template-columns:64px minmax(0,1fr); grid-template-rows:auto; justify-items:stretch; column-gap:12px; text-align:start; flex-shrink:0; max-height:35%; }
  .music-page--focus .music-now__album > :not(.music-cover) { grid-column:2; margin-block:2px; }
  .music-page--focus .music-cover { grid-column:1; grid-row:1/7; width:64px; height:64px; margin:0; }
  .music-page--focus .music-now__album h2 { font-size:18px; }
  .music-page--focus .music-lyric-section { flex:1 1 0; }
  .music-queue .music-track__art { width:32px; height:32px; }
  .music-queue > li { gap:2px; }
  .music-queue > li > .music-icon-button { min-width:32px; }
}

@container (max-height:480px) {
  .music-page--focus .music-header { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip-path:inset(50%); white-space:nowrap; }
  .music-page--focus .music-tabs { margin-bottom:8px; }
  .music-page--focus .music-tabs button { padding-block:8px; }
  .music-page--focus .music-now { display:flex; flex-direction:column; padding:8px; }
  .music-page--focus .music-now__album { display:none; }
  .music-page--focus .music-console__title.is-expanded { max-height:min(30cqh,144px); overflow-y:auto; overscroll-behavior:contain; }
  .music-page--focus .music-lyric-section { flex:1 1 0; }
  .music-page--focus .music-lyric-section > .music-caption { margin:0 0 4px; }
  .music-page--focus .music-console { gap:6px; padding:8px; }
  .music-page--focus .music-lyrics { padding-block:4px; }
}
@container (min-width:480px) and (max-height:480px) {
  .music-page--focus .music-console { grid-template-columns:minmax(100px,.7fr) minmax(280px,1.3fr); }
  .music-page--focus .music-console__cover { width:36px; height:36px; }
  .music-page--focus .music-console__summary { padding-right:0; }
  .music-page--focus .music-console__center { padding-right:32px; }
  .music-page--focus .music-mobile-volume-toggle { display:flex; position:absolute; right:6px; top:8px; }
  .music-page--focus .music-console__volume { display:none; }
  .music-page--focus .music-console__volume.is-open { display:flex; grid-column:1/-1; }
}
.music-save-retry { border:0; background:transparent; color:var(--brand); font:inherit; padding:0 4px; text-decoration:underline; }

.music-lyric-title { display:flex; flex-wrap:wrap; align-items:center; gap:8px; min-width:0; }
.music-source-label { color:var(--muted); font-size:13px; font-weight:400; }
.music-format-line { display:flex; flex-wrap:wrap; align-items:center; justify-content:flex-end; gap:4px 6px; }
.music-track__format { max-width:170px; }
.music-lossless { display:inline-flex; align-items:center; gap:3px; border:1px solid var(--brand); border-radius:var(--radius-sm); padding:2px 5px; color:var(--brand-strong); background:var(--brand-soft); font-size:12px; font-weight:600; white-space:nowrap; }
/* A browser must not anchor to text that can move within an otherwise stable row. */
.music-tracks { overflow-anchor:none; }
/* Give names and quality indicators the same space before and after metadata arrives. */
.music-tracks .music-track__name strong {
  display:-webkit-box; -webkit-box-orient:vertical; -webkit-line-clamp:2;
  line-height:1.35; block-size:2lh; overflow:hidden;
}
.music-tracks .music-track__name > span,.music-tracks .music-bitrate { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.music-tracks .music-track__format { inline-size:min(36%, 170px); flex:none; }
.music-tracks .music-format-line {
  display:grid; grid-template-columns:max-content minmax(0,1fr);
  grid-template-rows:1lh calc(1lh + 6px); justify-items:end;
}
.music-tracks .music-lossless { grid-column:1/-1; }
/* Browsing scrolls its header away; lyrics scroll independently and controls stay visible. */
.music-content { display:flex; flex-direction:column; overflow:hidden; }
.music-page:not(.music-page--focus) .music-content { display:block; overflow-y:auto; container-type:size; }
.music-header,.music-tabs,.music-location,.music-location-form,.music-notice { flex-shrink:0; }
.music-notice:empty { display:none; }
.music-layout { flex:1 1 0; min-height:0; align-items:stretch; }
.music-layout:not(.music-layout--focus) { min-height:100cqh; align-items:start; }
.music-library { display:flex; flex-direction:column; min-height:0; overflow:hidden; }
.music-layout:not(.music-layout--focus) .music-library { min-height:100cqh; overflow:visible; }
.music-library > :not(.music-tracks):not(.music-queue) { flex-shrink:0; }
.music-library:has(.music-empty),.music-library:has(.music-error) { overflow-y:auto; overscroll-behavior:contain; }
.music-tracks,.music-queue { flex:1 0 auto; min-height:0; max-height:none; overflow:visible; scrollbar-gutter:auto; }
.music-library .music-folders { max-height:30%; overflow-y:auto; }
.music-layout:not(.music-layout--focus) .music-now { position:sticky; top:0; height:var(--music-sidebar-height,100cqh); padding:16px; display:flex; flex-direction:column; min-height:0; overflow:hidden; }
.music-layout:not(.music-layout--focus) .music-now__album { display:grid; grid-template-rows:minmax(0,248px); grid-auto-rows:max-content; justify-items:center; align-content:center; flex:0 0 75%; min-height:0; overflow-y:auto; overscroll-behavior:contain; }
.music-layout:not(.music-layout--focus) .music-cover { width:auto; height:calc(100% - 8px); max-width:100%; max-height:240px; margin-bottom:8px; }
.music-layout:not(.music-layout--focus) .music-provenance { gap:4px; margin:4px 0; }
.music-layout:not(.music-layout--focus) .music-now__album > .music-eyebrow { display:none; }
.music-layout:not(.music-layout--focus) .music-now__album h2 { font-size:18px; margin-block:2px; }
.music-layout:not(.music-layout--focus) .music-now__album > p { font-size:13px; margin-block:2px; }
.music-layout:not(.music-layout--focus) .music-lyric-section { display:flex; flex-direction:column; flex:1 1 0; min-height:0; margin-top:12px; padding-top:10px; }
.music-layout:not(.music-layout--focus) .music-lyrics { flex:1 1 0; min-height:0; max-height:none; overscroll-behavior:contain; }
@container (max-width:680px) {
  .music-track__format { max-width:100px; }
  .music-layout:not(.music-layout--focus) { display:flex; flex-direction:column; }
  .music-layout:not(.music-layout--focus) .music-now.music-now--mobile-hidden { display:none; }
  .music-layout:not(.music-layout--focus) .music-library { flex:1 1 0; }
}
@container (max-height:480px) {
  .music-header { position:absolute; width:1px; height:1px; padding:0; margin:-1px; overflow:hidden; clip-path:inset(50%); white-space:nowrap; }
  .music-tabs { margin-bottom:8px; }
  .music-tabs button { padding-block:8px; }
  .music-location { margin-block:4px; }
  .music-location,.music-location__source { flex-wrap:nowrap; }
  .music-location__source > .music-text-button { flex:1; min-width:0; }
  .music-location__source > .music-text-button span { white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .music-library__hero { display:none; }
  .music-search { margin-block:8px; }
  .music-console { gap:6px; padding:8px; }
}
@container (min-width:480px) and (max-height:480px) {
  .music-console { grid-template-columns:minmax(100px,.7fr) minmax(280px,1.3fr); }
  .music-console__cover { width:36px; height:36px; }
  .music-console__summary { padding-right:0; }
  .music-console__center { padding-right:32px; }
  .music-mobile-volume-toggle { display:flex; position:absolute; right:6px; top:8px; }
  .music-console__volume { display:none; }
  .music-console__volume.is-open { display:flex; grid-column:1/-1; }
}
@media (max-height:560px) {
  .music-page { --music-page-padding:8px; padding:8px; }
  .music-tabs { margin-bottom:4px; }
  .music-search { margin-block:4px; }
  .music-search input,.music-search select { padding-block:6px; }
}
/* Phone browsers use the same bounded workspace as desktop, with readable rows and reachable controls. */
.music-console__title { flex:1; min-width:0; border:0; padding:0; color:var(--text); background:transparent; text-align:start; font:inherit; }
.music-console__title strong,.music-console__title span { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.music-console__title.is-expanded strong,.music-console__title.is-expanded span { overflow:visible; text-overflow:clip; white-space:normal; overflow-wrap:anywhere; }
@container (max-width:680px) {
  .music-header { display:none; }
  .music-tabs { position:sticky; top:0; z-index:1; background:var(--surface); gap:16px; margin-bottom:12px; }
  .music-tabs button { min-height:44px; gap:6px; padding-block:8px; }
  .music-location { display:grid; grid-template-columns:minmax(0,1fr) auto; align-items:start; margin:0 0 12px; }
  .music-location__source { display:grid; min-width:0; }
  .music-location__source > .music-text-button { max-width:100%; padding:6px 0; }
  .music-location__source > .music-text-button span { white-space:nowrap; text-overflow:ellipsis; overflow:hidden; }
  .music-location__actions { gap:0; margin:0; flex-wrap:wrap; justify-content:end; max-width:100px; }
  .music-location-form input { flex-basis:100%; font-size:16px; }
  .music-library__hero { display:flex; padding:8px; gap:8px; }
  .music-library__hero-info { flex-basis:80px; gap:0 6px; }
  .music-library__hero h2 { font-size:16px; margin:2px 0; }
  .music-library__hero-actions { gap:0; }
  .music-library__hero-actions .button { padding-inline:10px; }
  .music-search { flex-wrap:nowrap; gap:8px; margin:12px 0; }
  .music-search label { min-width:0; gap:6px; }
  .music-search input,.music-location-form input { font-size:16px; }
  .music-search select { max-width:94px; padding-inline:6px; min-height:44px; }
  .music-track { display:grid; grid-template-columns:36px minmax(0,1fr); gap:4px 8px; padding-block:10px; }
  .music-track__number { display:none; }
  .music-track__art { grid-column:1; grid-row:1; width:36px; height:36px; }
  .music-track__name { grid-column:2; gap:3px; }
  .music-track__name strong { display:-webkit-box; -webkit-box-orient:vertical; -webkit-line-clamp:2; overflow:hidden; }
  .music-track__name span { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
  .music-track__format { grid-column:1/-1; max-width:none; padding:0; text-align:start; }
  .music-format-line { justify-content:start; gap:3px 6px; min-height:24px; }
  .music-tracks .music-track__format { inline-size:auto; }
  .music-tracks .music-format-line { grid-template-columns:max-content minmax(0,max-content) max-content; grid-template-rows:calc(1lh + 6px); justify-items:start; }
  .music-tracks .music-lossless { grid-column:auto; }
  .music-icon-button,.music-queue > li > .music-icon-button { min-width:40px; min-height:44px; }
  .music-queue .music-track { grid-template-columns:32px minmax(0,1fr); }
  .music-queue .music-track > svg { display:none; }
  .music-queue > li { gap:0; padding-inline:2px; }
  .music-queue .music-track__art { width:32px; height:32px; }
  .music-console { gap:4px; padding:8px; padding-bottom:max(8px,env(safe-area-inset-bottom)); }
  .music-console__summary { gap:8px; padding-right:40px; }
  .music-console__cover { width:36px; height:36px; }
  .music-console__summary .music-icon-button { min-width:32px; }
  .music-console__title span { margin-top:2px; }
  .music-mobile-volume-toggle { right:8px; top:8px; }
  .music-transport { gap:8px; }
  .music-play { width:44px; height:44px; }
  .music-progress { gap:6px; margin-top:0; }
  .music-console__volume.is-open { display:flex; grid-column:1/-1; justify-content:center; padding-top:4px; border-top:1px solid var(--border); }
  .music-console__volume input { flex:1; max-width:160px; }
  .music-playback-status { position:static; transform:none; grid-column:1/-1; max-width:none; max-height:88px; overflow-y:auto; box-shadow:none; padding:4px 8px; font-size:14px; }
  .music-page--focus .music-now { gap:8px; padding:10px; }
  .music-page--focus .music-now__album { grid-template-columns:clamp(64px,24cqi,120px) minmax(0,1fr); max-height:42%; overflow-y:auto; }
  .music-page--focus .music-cover { width:100%; height:auto; max-height:120px; }
  .music-page--focus .music-now__album h2 { font-size:18px; }
  .music-page--focus .music-provenance { justify-content:start; gap:4px; margin:4px 0; }
  .music-page--focus .music-lyrics button { font-size:16px; padding:12px 8px; }
  .music-lyric-title { gap:4px 6px; }
}
@container (max-width:680px) and (max-height:480px) {
  .music-tabs { margin-bottom:4px; }
  .music-page--focus .music-now__album { display:none; }
}
@container (min-width:480px) and (max-height:480px) {
  .music-console__summary { padding-right:0; }
  .music-console__title { max-height:52px; }
  .music-console__volume.is-open { position:absolute; bottom:100%; right:0; padding:8px; background:var(--surface-raised); border:1px solid var(--border); border-radius:var(--radius-md); }
}
</style>
