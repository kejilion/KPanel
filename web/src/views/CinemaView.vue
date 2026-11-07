<script setup lang="ts">
import { computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { mediaLocations, setMediaLocation } from '@/lib/mediaLocations'
import { useRoute, useRouter } from 'vue-router'
import { Clapperboard, Film, Folder, FolderOpen, Image as ImageIcon, LayoutGrid, Pencil, Play, RefreshCw, Search, X } from '@lucide/vue'
import HostSwitcher from '@/components/common/HostSwitcher.vue'
import GalleryVideo from '@/components/gallery/GalleryVideo.vue'
import GalleryViewer from '@/components/gallery/GalleryViewer.vue'
import CinemaMetadataPanel from '@/components/gallery/CinemaMetadata.vue'
import { cinemaMetadataImage, cinemaSourceURL, type CinemaMetadata } from '@/lib/cinemaMetadata'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase, usePhraseCatalog } from '@/i18n/phrase'
import { api } from '@/lib/api'
import { automaticCinemaCover, CINEMA_ROOT, cinemaImage, cinemaRenameTarget, cinemaTitle, readCinemaFolder, readCinemaPreferences, writeCinemaPreferences, type CinemaFolder } from '@/lib/cinema'
import { clusterHostPanelPageURL } from '@/lib/clusterHostNavigation'
import { desktopWindowVisibleKey } from '@/lib/desktopRouteKeys'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { fileHostLabel, fileHostStatus } from '@/lib/fileHostStatus'
import { notifyFileDirectoriesChanged } from '@/lib/fileWindowTransfer'
import { formatBytes } from '@/lib/format'
import { galleryBaseName, galleryBrowserCanShow, galleryFileExtension, galleryParentPath, nextGalleryUploadName, normalizeGalleryRoot, type GalleryItem } from '@/lib/gallery'
import { galleryPosterKey, readGalleryPoster } from '@/lib/galleryPosters'
import { useMusicPlayer } from '@/stores/musicPlayer'
import type { ClusterHost } from '@/types/api'

usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/GalleryView/en-US').then(m => m.default) : import('@/i18n/pages/GalleryView/zh-TW').then(m => m.default))
usePhraseCatalog(locale => locale === 'en-US' ? import('@/i18n/pages/CinemaView/en-US').then(m => m.default) : import('@/i18n/pages/CinemaView/zh-TW').then(m => m.default))
function phrase(value: string) {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const route = useRoute()
const router = useRouter()
const i18n = useI18n()
const windowVisible = inject(desktopWindowVisibleKey, undefined)
const windowed = Boolean(windowVisible)
const explicitLocation = computed(() => typeof route.query.path === 'string' || typeof route.query.hostId === 'string')
const hostId = computed(() => explicitLocation.value ? (typeof route.query.hostId === 'string' ? route.query.hostId : '') : mediaLocations.cinemaLocation?.hostId ?? '')
const path = computed(() => typeof route.query.path === 'string' ? (route.query.path === '/' ? '/' : normalizeGalleryRoot(route.query.path) ?? CINEMA_ROOT) : explicitLocation.value ? CINEMA_ROOT : mediaLocations.cinemaLocation?.path ?? CINEMA_ROOT)
const files = computed(() => fileAPIForHost(hostId.value))
const preferences = ref(readCinemaPreferences(hostId.value, path.value))
const snapshot = ref<CinemaFolder>()
const directory = ref('')
const search = ref('')
const shown = ref(48)
const order = ref<'newest' | 'name'>('newest')
const loading = ref(false)
const error = ref('')
const notice = ref('')
const hosts = ref<ClusterHost[]>([])
const hostsLoading = ref(false)
const hostsError = ref(false)
const picker = ref<InstanceType<typeof HostSwitcher>>()
const playingPath = ref('')
const editing = ref<GalleryItem>()
const editorTitle = ref<HTMLInputElement>()
const editTitle = ref('')
const editCover = ref('')
const editFeatured = ref(false)
const editError = ref('')
const saving = ref(false)
const metadata = ref(new Map<string, CinemaMetadata>())
function filmTitle(item: GalleryItem) { return metadata.value.get(item.entry.path)?.title ?? cinemaTitle(item.entry.name) }
function filmMetadata(item: GalleryItem) { return metadata.value.get(item.entry.path) }
let listing: AbortController | undefined
let coverUpload: AbortController | undefined
let disposed = false

const selectedHost = computed(() => hosts.value.find(host => hostId.value ? host.id === hostId.value : host.isLocal))
const films = computed(() => [...(snapshot.value?.films ?? [])].sort((a, b) =>
  (order.value === 'newest' ? b.time - a.time : 0) || a.entry.name.localeCompare(b.entry.name, undefined, { numeric: true })))
const visibleFilms = computed(() => films.value.filter(item => (item.entry.name + ' ' + filmTitle(item)).toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
const featured = computed(() => films.value.find(item => item.entry.path === preferences.value.featured) ?? films.value[0])
const viewerIndex = computed(() => films.value.findIndex(item => item.entry.path === playingPath.value))
const folderCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const item of films.value) counts.set(item.folder, (counts.get(item.folder) ?? 0) + 1)
  return counts
})
const covers = computed(() => new Map(preferences.value.covers))
const failedCovers = ref(new Set<string>())
const editImages = computed(() => editing.value ? snapshot.value?.images.get(editing.value.folder) ?? [] : [])
const automatic = ''
const frame = ':frame'

function content(item: GalleryItem, disposition?: 'inline' | 'attachment') {
  return files.value.contentUrl(item.entry.path, disposition)
}
function posterKey(item: GalleryItem) {
  return galleryPosterKey(item.entry, content(item, 'inline'))
}
function coverUrl(item: GalleryItem, backdrop = false): string | undefined {
  const images = snapshot.value?.images.get(item.folder) ?? []
  const choice = covers.value.get(item.entry.path)
  if (choice === frame) return undefined
  const image = images.find(entry => entry.path === choice) ?? automaticCinemaCover(item, images, folderCounts.value.get(item.folder) === 1)
  const meta = metadata.value.get(item.entry.path)
  const url = image ? files.value.contentUrl(image.path, 'inline') : meta ? cinemaMetadataImage(meta, backdrop) : undefined
  return url && !failedCovers.value.has(url) ? url : undefined
}
function coverError(event: Event) {
  const url = (event.target as HTMLImageElement).getAttribute('src')
  if (url) failedCovers.value.add(url)
}
function viewerSources(item: GalleryItem) {
  return {
    original: content(item, 'inline'),
    preview: coverUrl(item) ?? readGalleryPoster(posterKey(item))?.poster,
  }
}
function play(item: GalleryItem) {
  const music = useMusicPlayer()
  if (music.state.playing || music.state.waiting) music.pause()
  playingPath.value = item.entry.path
}
function download(item: GalleryItem) {
  const link = document.createElement('a')
  link.href = content(item, 'attachment')
  link.download = item.entry.name
  link.click()
}

function message(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}
async function load() {
  listing?.abort()
  const controller = new AbortController()
  listing = controller
  loading.value = true
  error.value = ''
  failedCovers.value.clear()
  try {
    const result = await readCinemaFolder(path.value, hostId.value, controller.signal, partial => {
      if (!controller.signal.aborted) snapshot.value = partial
    })
    if (!controller.signal.aborted) snapshot.value = result
  } catch (cause) {
    if (!controller.signal.aborted) error.value = message(cause)
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}
async function loadHosts() {
  if (hostsLoading.value) return
  hostsLoading.value = true
  try {
    const result = await api.cluster.hosts()
    if (!disposed) {
      hosts.value = result.items
      hostsError.value = false
    }
  } catch {
    if (!disposed) hostsError.value = true
  } finally {
    if (!disposed) hostsLoading.value = false
  }
}
function location(target: string, host = hostId.value) {
  return { path: target, ...(host ? { hostId: host } : {}) }
}
function browse(target: string, host = hostId.value) {
  if (saving.value) return
  const unchanged = target === path.value && host === hostId.value
  setMediaLocation('cinemaLocation', target, host)
  if (unchanged) {
    void load()
    return
  }
  void router.push({ name: 'cinema', query: location(target, host) })
}
function openDirectory() {
  const input = directory.value.trim()
  const target = input === '/' ? '/' : normalizeGalleryRoot(input)
  if (!target) {
    error.value = phrase('请输入有效的绝对路径')
    return
  }
  browse(target)
}
function selectHost(host: ClusterHost) {
  picker.value?.close(true)
  const status = fileHostStatus(host)
  if (status.action === 'select') {
    browse(CINEMA_ROOT, host.isLocal ? '' : host.id)
    return
  }
  const url = status.action === 'open' ? clusterHostPanelPageURL(host, 'cinema') : undefined
  if (!url) {
    void router.push({ name: 'cluster' })
    return
  }
  if (host.transportSecurity !== 'e2e_http' || window.confirm(i18n.t('cluster.confirm.openHttpPanel'))) {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}
function openFiles(folder = path.value) {
  void router.push({ name: 'files', query: location(folder) })
}
async function edit(item: GalleryItem) {
  editing.value = item
  editTitle.value = cinemaTitle(item.entry.name)
  editCover.value = covers.value.get(item.entry.path) ?? automatic
  editFeatured.value = preferences.value.featured === item.entry.path
  editError.value = ''
  await nextTick()
  editorTitle.value?.focus()
}
async function save() {
  const item = editing.value
  if (!item || saving.value) return
  const target = cinemaRenameTarget(item.entry, editTitle.value)
  if (!target) {
    editError.value = phrase('请输入有效的影片名称')
    return
  }
  const old = item.entry.path
  const host = hostId.value
  const root = path.value
  const previous = preferences.value
  const selected = editCover.value
  const pin = editFeatured.value
  const autoImage = automaticCinemaCover(item, editImages.value, folderCounts.value.get(item.folder) === 1)
  saving.value = true
  editError.value = ''
  try {
    if (target !== old) {
      const result = await fileAPIForHost(host).action({ action: 'rename', sources: [old], target, expectedResourceVersion: item.entry.resourceVersion })
      if (result.failed.length) throw new Error(result.failed[0]!.detail)
      notifyFileDirectoriesChanged([item.folder], undefined, [{ source: old, destination: target }], host)
    }
    const cover = selected || (target !== old ? autoImage?.path ?? '' : '')
    const next: typeof previous = {
      layout: previous.layout,
      featured: pin ? target : previous.featured === old ? '' : previous.featured,
      covers: [...previous.covers.filter(([key]) => key !== old && key !== target), ...(cover ? [[target, cover] as [string, string]] : [])].slice(-100),
    }
    writeCinemaPreferences(host, root, next)
    if (disposed || host !== hostId.value || root !== path.value) return
    preferences.value = next
    editing.value = undefined
    notice.value = phrase('影片已更新')
    if (target !== old) await load()
  } catch (cause) {
    if (!disposed && host === hostId.value && root === path.value) editError.value = message(cause) || phrase('影片未更新')
  } finally {
    saving.value = false
  }
}
async function uploadCover(event: Event) {
  const input = event.target as HTMLInputElement
  const image = input.files?.[0]
  input.value = ''
  const item = editing.value
  if (!item || !image || saving.value) return
  if (!cinemaImage({ name: image.name, kind: 'file' }) || !image.size || image.size > 8 * 1024 * 1024) {
    editError.value = phrase('请选择不超过 8 MB 的 JPG、PNG、WebP 或 AVIF 图片')
    return
  }
  const host = hostId.value
  const extension = galleryFileExtension(image.name)
  const name = nextGalleryUploadName(`film-cover.${extension}`, new Set(editImages.value.map(entry => entry.name)))
  const controller = new AbortController()
  coverUpload = controller
  saving.value = true
  editError.value = ''
  try {
    const entry = await fileAPIForHost(host).upload(item.folder, new File([image], name, { type: image.type }), false, undefined, controller.signal)
    notifyFileDirectoriesChanged([item.folder], undefined, [], host)
    if (disposed || editing.value !== item || host !== hostId.value) return
    snapshot.value?.images.set(item.folder, [...editImages.value, entry])
    editCover.value = entry.path
  } catch (cause) {
    if (!controller.signal.aborted && editing.value === item) editError.value = message(cause)
  } finally {
    saving.value = false
    coverUpload = undefined
  }
}
function setLayout(layout: 'poster' | 'wide') {
  preferences.value = { ...preferences.value, layout }
  writeCinemaPreferences(hostId.value, path.value, preferences.value)
}

watch([path, hostId], () => {
  directory.value = path.value
  search.value = ''
  shown.value = 48
  playingPath.value = ''
  editing.value = undefined
  snapshot.value = undefined
  notice.value = ''
  preferences.value = readCinemaPreferences(hostId.value, path.value)
  void load()
}, { immediate: true })
watch([search, order], () => { shown.value = 48 })
watch(() => windowVisible?.value, visible => { if (visible === false) playingPath.value = '' })
onMounted(loadHosts)
onBeforeUnmount(() => {
  disposed = true
  listing?.abort()
  coverUpload?.abort()
})
</script>

<template>
  <main class="page cinema-page">
    <header v-if="!windowed" class="cinema-heading">
      <img class="cinema-heading__icon" :src="'/desktop-icons/cinema-kpanel-flat-v3.webp'" alt="" />
      <div>
        <h1>{{ i18n.t('route.cinema') }}</h1>
        <p>{{ phrase('自己的片库，随时开场') }}</p>
      </div>
      <button class="button button--secondary cinema-heading__files" type="button" @click="openFiles()">
        <FolderOpen :size="17" /> {{ phrase('文件管理') }}
      </button>
    </header>
    <h1 v-else class="sr-only">{{ i18n.t('route.cinema') }}</h1>

    <section class="cinema-source" :aria-label="phrase('影片来源')">
      <HostSwitcher
        ref="picker" :hosts="hosts" :active-id="selectedHost?.id ?? hostId"
        :label="fileHostLabel(hostId, hosts, phrase('本机'))" :loading="hostsLoading" :error="hostsError"
        :status-of="host => { const status = fileHostStatus(host); return { ...status, label: phrase(status.label) } }"
        :disabled="saving" @open="loadHosts" @refresh="loadHosts" @select="selectHost"
      />
      <form class="cinema-location" @submit.prevent="openDirectory">
        <input v-model="directory" :aria-label="phrase('影视目录')" :disabled="saving" />
        <button class="button button--secondary" :disabled="saving">{{ phrase('打开目录') }}</button>
      </form>
      <button class="icon-button" type="button" :aria-label="phrase('上级目录')" :title="phrase('上级目录')" :disabled="path === '/' || saving" @click="browse(galleryParentPath(path))"><FolderOpen :size="17" /></button>
      <button class="icon-button" type="button" :aria-label="phrase('刷新')" :title="phrase('刷新')" :disabled="loading || saving" @click="load"><RefreshCw :size="17" /></button>
      <button v-if="windowed" class="button button--secondary" type="button" @click="openFiles()"><FolderOpen :size="17" /> {{ phrase('文件管理') }}</button>
    </section>

    <CinemaMetadataPanel :items="films" :host="hostId" :selected="editing" :scanning="loading" :visible="windowVisible !== false" @update="metadata = $event" />
    <section v-if="featured" class="cinema-feature" :aria-label="phrase('精选影片')">
      <div class="cinema-feature__art" aria-hidden="true">
        <Film :size="84" />
        <img v-if="coverUrl(featured, true)" :key="coverUrl(featured, true)" :src="coverUrl(featured, true)" alt="" decoding="async" @error="coverError" />
        <GalleryVideo v-else-if="galleryBrowserCanShow(featured)" :src="content(featured, 'inline')" :poster-key="posterKey(featured)" :suspended="windowVisible === false" />
      </div>
      <div class="cinema-feature__body">
        <span class="cinema-feature__label"><Clapperboard :size="16" /> {{ phrase('精选影片') }}</span>
        <h2>{{ filmTitle(featured) }}</h2>
        <p v-if="filmMetadata(featured)">{{ filmMetadata(featured)?.year }} <span v-if="filmMetadata(featured)?.rating"> · {{ filmMetadata(featured)?.rating?.toFixed(1) }} / 10</span> · {{ filmMetadata(featured)?.genres?.join(' / ') }}</p>
        <p v-if="filmMetadata(featured)?.overview" class="cinema-overview">{{ filmMetadata(featured)?.overview }}</p>
        <p v-if="filmMetadata(featured)?.cast?.length">{{ filmMetadata(featured)?.cast?.join(' · ') }}</p>
        <a v-if="filmMetadata(featured)" :href="cinemaSourceURL(filmMetadata(featured)!)" target="_blank" rel="noopener noreferrer" class="cinema-credit">{{ filmMetadata(featured)?.provider }}</a>
        <p>{{ galleryFileExtension(featured.entry.name).toUpperCase() }} · {{ formatBytes(featured.entry.sizeBytes) }}</p>
        <div class="cinema-feature__actions">
          <button class="button cinema-feature__play" type="button" @click="play(featured)"><Play :size="18" /> {{ phrase('播放影片') }}</button>
          <button class="button cinema-feature__edit" type="button" @click="edit(featured)"><Pencil :size="16" /> {{ phrase('编辑影片') }}</button>
        </div>
      </div>
    </section>

    <nav v-if="snapshot?.folders.length" class="cinema-folders" :aria-label="phrase('影视目录')">
      <button v-for="folder in snapshot.folders.slice(0, 24)" :key="folder.path" class="button button--secondary button--small" type="button" :disabled="saving" @click="browse(folder.path)"><Folder :size="15" />{{ folder.name }}</button>
    </nav>

    <form v-if="editing" class="cinema-editor" :aria-label="phrase('编辑影片')" @submit.prevent="save" @keydown.esc.stop="!saving && (editing = undefined)">
      <header><h2>{{ phrase('编辑影片') }}</h2><button class="icon-button" type="button" :aria-label="phrase('关闭')" :disabled="saving" @click="editing = undefined"><X :size="18" /></button></header>
      <label>{{ phrase('影片名称') }}<input ref="editorTitle" v-model="editTitle" required maxlength="240" :disabled="saving" /></label>
      <p>{{ phrase('改名会同步修改服务器上的视频文件名，保留扩展名。') }}</p>
      <label>{{ phrase('影片封面') }}
        <select v-model="editCover" :disabled="saving">
          <option :value="automatic">{{ phrase('自动封面') }}</option>
          <option :value="frame">{{ phrase('使用视频截帧') }}</option>
          <option v-for="image in editImages" :key="image.path" :value="image.path">{{ image.name }}</option>
        </select>
      </label>
      <label>{{ phrase('上传封面图片') }}
        <input type="file" accept=".jpg,.jpeg,.png,.webp,.avif" :disabled="saving" @change="uploadCover" />
      </label>
      <label class="cinema-editor__check"><input v-model="editFeatured" type="checkbox" :disabled="saving" /> {{ phrase('设为首页主图') }}</label>
      <p>{{ phrase('封面和海报布局保存在当前浏览器。') }}</p>
      <p v-if="editError" class="cinema-error" role="alert">{{ editError }}</p>
      <div class="cinema-feature__actions">
        <button class="button button--primary" :disabled="saving">{{ phrase('保存') }}</button>
        <button class="button button--secondary" type="button" :disabled="saving" @click="editing = undefined">{{ phrase('取消') }}</button>
      </div>
    </form>
    <p v-if="notice" class="cinema-note" role="status">{{ notice }}</p>

    <section class="cinema-library" :aria-label="phrase('海报墙')">
      <div class="cinema-toolbar">
        <div><h2>{{ phrase('海报墙') }}</h2><p>{{ visibleFilms.length }} {{ phrase('部影片') }}</p></div>
        <div class="cinema-toolbar__controls">
          <label class="cinema-search"><Search :size="17" /><input v-model="search" type="search" :aria-label="phrase('搜索影片')" :placeholder="phrase('搜索影片')" /></label>
          <select v-model="order" :aria-label="phrase('排序')"><option value="newest">{{ phrase('最近更新') }}</option><option value="name">{{ phrase('按名称') }}</option></select>
          <div class="cinema-layout" role="group" :aria-label="phrase('海报墙')">
            <button class="icon-button" type="button" :aria-label="phrase('竖版海报')" :title="phrase('竖版海报')" :aria-pressed="preferences.layout === 'poster'" @click="setLayout('poster')"><LayoutGrid :size="17" /></button>
            <button class="icon-button" type="button" :aria-label="phrase('宽屏剧照')" :title="phrase('宽屏剧照')" :aria-pressed="preferences.layout === 'wide'" @click="setLayout('wide')"><ImageIcon :size="17" /></button>
          </div>
        </div>
      </div>
      <p v-if="loading" class="cinema-note" role="status">{{ phrase('正在读取影视目录') }}</p>
      <div v-if="error" class="cinema-empty" role="alert"><h3>{{ phrase('影视目录读取失败') }}</h3><p>{{ error }}</p><button class="button button--secondary" @click="load">{{ phrase('重试') }}</button></div>
      <p v-if="snapshot?.truncated || snapshot?.unreadable" class="cinema-note">{{ phrase('目录较大或有子目录无法读取，当前显示已扫描的影片。') }}</p>
      <div v-if="!loading && !error && !visibleFilms.length" class="cinema-empty">
        <Clapperboard :size="36" />
        <h3>{{ phrase(search ? '没有匹配的影片' : '你的影院还没有影片') }}</h3>
        <p v-if="!search">{{ phrase('选择存有视频的目录，或打开文件管理添加影片。') }}</p>
        <button v-if="search" class="button button--secondary" @click="search = ''">{{ phrase('清除搜索') }}</button>
        <button v-else class="button button--secondary" @click="openFiles()">{{ phrase('文件管理') }}</button>
      </div>
      <ul v-if="visibleFilms.length" class="cinema-grid" :class="{ 'cinema-grid--wide': preferences.layout === 'wide' }">
        <li v-for="item in visibleFilms.slice(0, shown)" :key="posterKey(item)" class="cinema-card">
          <button class="cinema-card__play" type="button" :aria-label="phrase('播放影片') + ' ' + cinemaTitle(item.entry.name)" @click="play(item)">
            <span class="cinema-card__poster">
              <Film :size="38" />
              <img v-if="coverUrl(item)" :key="coverUrl(item)" :src="coverUrl(item)" alt="" loading="lazy" decoding="async" @error="coverError" />
              <GalleryVideo v-else-if="galleryBrowserCanShow(item)" :src="content(item, 'inline')" :poster-key="posterKey(item)" :suspended="windowVisible === false" />
              <span class="cinema-card__format">{{ galleryFileExtension(item.entry.name).toUpperCase() }}</span>
              <span class="cinema-card__watch"><Play :size="22" /></span>
            </span>
            <strong>{{ filmTitle(item) }}</strong>
            <span v-if="filmMetadata(item)" class="cinema-card__metadata">{{ filmMetadata(item)?.year }} <span v-if="filmMetadata(item)?.rating"> · {{ filmMetadata(item)?.rating?.toFixed(1) }} / 10</span> · {{ filmMetadata(item)?.provider }}</span>
          </button>
          <div class="cinema-card__footer">
            <span>{{ item.folder === path ? formatBytes(item.entry.sizeBytes) : galleryBaseName(item.folder) }}</span>
            <button class="icon-button icon-button--small" type="button" :aria-label="phrase('编辑影片') + ' ' + cinemaTitle(item.entry.name)" :title="phrase('编辑影片')" :disabled="saving" @click="edit(item)"><Pencil :size="15" /></button>
          </div>
        </li>
      </ul>
      <button v-if="shown < visibleFilms.length" class="button button--secondary cinema-more" type="button" @click="shown += 48">{{ phrase('显示更多影片') }}</button>
    </section>

  </main>
  <GalleryViewer v-if="viewerIndex >= 0" :items="films" :index="viewerIndex" :sources="viewerSources" :contained="windowed" :filmstrip="true"
    :navigation-labels="{ previous: phrase('上一部影片'), next: phrase('下一部影片') }"
    @close="playingPath = ''" @navigate="playingPath = films[$event]?.entry.path ?? ''"
    @download="download" @reveal="playingPath = ''; openFiles($event.folder)" />
</template>

<style scoped>
.cinema-overview { max-width: 75ch; max-height: 8em; overflow: auto; margin-block: 10px; }
.cinema-credit { color: #c5e6ff; font-size: 13px; }
.cinema-card__metadata { display: block; padding: 0 12px 8px; color: var(--text-soft); font-size: 13px; }
.cinema-page { display: block; container-type: inline-size; padding: clamp(14px, 2vw, 24px); border: 1px solid var(--border); border-radius: var(--radius-lg); background: var(--surface); color: var(--text); }
.cinema-heading, .cinema-feature__actions, .cinema-source, .cinema-folders, .cinema-toolbar, .cinema-toolbar__controls, .cinema-layout, .cinema-card__footer { display: flex; align-items: center; gap: 10px; }
.cinema-heading { flex-wrap: wrap; margin-bottom: 20px; }
.cinema-heading__icon { width: 52px; height: 52px; border-radius: 22%; }
.cinema-heading__files { margin-inline-start: auto; }
h1, h2, h3, p { margin: 0; }
h1 { font-size: 26px; }
p { color: var(--text-soft); line-height: 1.55; }
.cinema-heading p, .cinema-toolbar p, .cinema-note { font-size: 14px; }
.cinema-feature { position: relative; display: grid; min-height: clamp(255px, 32cqi, 395px); align-items: end; isolation: isolate; overflow: hidden; border-radius: var(--radius-lg); background: #121925; color: #fff; }
.cinema-feature__art { position: absolute; inset: 0; display: grid; place-items: center; background: linear-gradient(120deg, #101827, #234d56 60%, #345a70); color: #b8d7dc; }
.cinema-feature__art > img, .cinema-card__poster > img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; }
.cinema-feature__art::after { content: ''; position: absolute; inset: 0; background: linear-gradient(90deg, rgb(8 14 24 / 88%), transparent 90%), linear-gradient(0deg, rgb(8 14 24 / 92%), transparent 85%); }
.cinema-feature__body { position: relative; padding: clamp(20px, 4cqi, 50px); min-width: 0; }
.cinema-feature__label { display: inline-flex; gap: 8px; align-items: center; color: #b4e4e7; font-size: 13px; font-weight: 700; }
.cinema-feature h2 { margin: 8px 0; max-width: 26ch; font-size: clamp(26px, 4.4cqi, 56px); line-height: 1.16; overflow-wrap: anywhere; }
.cinema-feature p { color: #dae4ed; }
.cinema-feature__actions { flex-wrap: wrap; margin-top: 16px; }
.cinema-feature__play { background: #d9f4f5; color: #142c36; }
.cinema-feature__edit { background: #233441; border-color: #5c707e; color: #fff; }
.cinema-source, .cinema-folders { margin-top: 14px; flex-wrap: wrap; }
.cinema-source { margin-bottom: 16px; }
.cinema-location { flex: 1 1 220px; display: flex; gap: 8px; min-width: 0; }
.cinema-location input { flex: 1 1 0; }
.cinema-location .button { flex: 0 0 auto; white-space: nowrap; }
.cinema-page input:not([type=checkbox]), .cinema-page select { min-width: 0; min-height: 40px; border: 1px solid var(--border); border-radius: var(--radius-sm); background: var(--surface); color: var(--text); font: inherit; padding: 8px 10px; }
.cinema-library { margin-top: 24px; }
.cinema-toolbar { flex-wrap: wrap; justify-content: space-between; margin-bottom: 14px; }
.cinema-toolbar h2, .cinema-editor h2 { font-size: 20px; }
.cinema-toolbar__controls { flex-wrap: wrap; }
.cinema-search { display: flex; align-items: center; gap: 7px; }
.cinema-layout { gap: 4px; }
.cinema-layout [aria-pressed=true] { background: var(--brand-soft); color: var(--text); border-color: var(--brand); }
.cinema-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(154px, 100%), 1fr)); gap: 14px; margin: 0; padding: 0; list-style: none; }
.cinema-grid--wide { grid-template-columns: repeat(auto-fill, minmax(min(235px, 100%), 1fr)); }
.cinema-card { min-width: 0; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface); }
.cinema-card__play { display: block; width: 100%; text-align: start; padding: 0; background: none; color: var(--text); border: 0; font: inherit; cursor: pointer; }
.cinema-card__poster { position: relative; display: grid; place-items: center; aspect-ratio: 2 / 3; overflow: hidden; border-radius: var(--radius-md) var(--radius-md) 0 0; background: linear-gradient(145deg, #1b3645, #172131); color: #a6cbd3; }
.cinema-grid--wide .cinema-card__poster { aspect-ratio: 16 / 9; }
.cinema-card__format, .cinema-card__watch { position: absolute; display: grid; place-items: center; background: #14202d; color: #fff; }
.cinema-card__format { inset: auto auto 8px 8px; padding: 4px 7px; border-radius: var(--radius-sm); font-size: 12px; }
.cinema-card__watch { width: 44px; height: 44px; border-radius: 50%; opacity: 0; }
.cinema-card__play:is(:hover,:focus-visible) .cinema-card__watch { opacity: 1; }
.cinema-card__play strong { display: block; padding: 11px 11px 1px; font-size: 14px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cinema-card__footer { justify-content: space-between; padding: 1px 7px 5px 11px; font-size: 13px; color: var(--text-soft); }
.cinema-card__footer span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.cinema-empty, .cinema-editor { display: grid; gap: 12px; padding: 20px; border: 1px solid var(--border); border-radius: var(--radius-lg); background: var(--surface); }
.cinema-empty { place-items: center; text-align: center; padding-block: 40px; }
.cinema-editor { margin-top: 16px; }
.cinema-editor header { display: flex; justify-content: space-between; align-items: center; }
.cinema-editor label { display: grid; gap: 6px; }
.cinema-editor .cinema-editor__check { display: flex; align-items: center; }
.cinema-editor p { font-size: 13px; }
.cinema-error { color: var(--danger); }
.cinema-note { margin: 12px 0; }
.cinema-more { display: flex; margin: 18px auto 0; }
@container (max-width: 440px) {
  .cinema-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
  .cinema-grid--wide { grid-template-columns: 1fr; }
  .cinema-search { flex: 1 1 100%; }
  .cinema-search input { flex: 1; }
}
</style>
