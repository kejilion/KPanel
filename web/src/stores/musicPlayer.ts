import { computed, reactive, watch } from 'vue'
import { matchOnlineMusic } from '@/lib/musicOnline'
import { saveMusicSidecars, type MusicSidecars } from '@/lib/musicSidecars'
import { fileAPIForHost } from '@/lib/fileHostContext'
import { MUSIC_LIMIT, readMusicLyrics, readMusicTrack, isMusic, trackKey, type Track } from '@/lib/music'
import { readMusicTags, readableMusicTag } from '@/lib/musicTags'

type Repeat = 'off' | 'all' | 'one'
interface PlayerDependencies {
  audio: () => HTMLAudioElement
  tags: typeof readMusicTags
  resolve: (track: Track, signal: AbortSignal) => Promise<Track | undefined>
  text: (track: Track, signal: AbortSignal) => Promise<string>
  save: typeof saveMusicSidecars
  online: typeof matchOnlineMusic
  random: () => number
}

/** One audio owner per browser tab; views only observe and control it. */
export function createMusicPlayer(overrides: Partial<PlayerDependencies> = {}) {
  const deps: PlayerDependencies = {
    audio: () => new Audio(), tags: readMusicTags,
    resolve: async (track, signal) => {
      const entry = await fileAPIForHost(track.hostId).entry(track.entry.path, signal)
      if (entry.path !== track.entry.path || !isMusic(entry)) return undefined
      return readMusicTrack(entry, track.hostId, signal)
    },
    text: readMusicLyrics,
    save: saveMusicSidecars, online: matchOnlineMusic, random: Math.random, ...overrides,
  }
  const state = reactive({
    queue: [] as Track[], current: undefined as Track | undefined,
    playing: false, waiting: false, restored: false, position: 0, duration: 0, volume: 0.8, muted: false,
    repeat: 'off' as Repeat, shuffle: false, error: '' as '' | 'blocked' | 'network' | 'unsupported',
    lyrics: '', lyricsLoading: false, lyricsFailed: false, embeddedCover: '', onlineCover: '', onlineSource: '', onlineStatus: '' as '' | 'loading' | 'matched' | 'empty' | 'failed', album: '',
    saveStatus: '' as '' | 'saving' | 'saved' | 'kept' | 'partial' | 'failed',
    viewCount: 0, notice: '' as '' | 'added' | 'next' | 'full',
  })
  let audio: HTMLAudioElement | undefined, controller: AbortController | undefined
  let timer: ReturnType<typeof setTimeout> | undefined, sequence = 0, detachedIndex = -1
  let history: Track[] = [], remaining: string[] = []
  let pendingSave: { track: Track; assets: MusicSidecars } | undefined
  let cycle = new Set<string>(), priority = ''
  const key = (track: Track) => trackKey(track)
  const currentIndex = computed(() => state.current ? state.queue.findIndex(t => key(t) === key(state.current!)) : -1)
  const cover = computed(() => state.embeddedCover || (state.current?.cover ? fileAPIForHost(state.current.hostId).contentUrl(state.current.cover) : state.onlineCover))
  const owns = (candidate: HTMLAudioElement, id: number) => candidate === audio && id === sequence

  function releaseAudio() {
    if (!audio) return
    audio.onplaying = audio.onpause = audio.onwaiting = audio.oncanplay = audio.ontimeupdate = audio.onloadedmetadata = audio.onended = audio.onerror = null
    audio.pause(); audio.removeAttribute('src'); audio.load(); audio = undefined
  }
  function releaseMetadata() {
    controller?.abort(); controller = undefined; clearTimeout(timer)
    if (state.embeddedCover) URL.revokeObjectURL(state.embeddedCover)
    state.embeddedCover = ''; state.onlineCover = ''; state.onlineSource = ''; state.onlineStatus = ''; state.saveStatus = ''; pendingSave = undefined
  }
  function mediaSession() { return typeof navigator !== 'undefined' ? navigator.mediaSession : undefined }
  function publishMetadata() {
    const session = mediaSession()
    if (!session || typeof MediaMetadata === 'undefined' || !state.current) return
    session.metadata = new MediaMetadata({ title: state.current.title, artist: state.current.artist, album: state.album,
      artwork: cover.value ? [{ src: cover.value }] : [] })
  }
  function syncSession() {
    const session = mediaSession()
    if (!session) return
    session.playbackState = state.playing ? 'playing' : 'paused'
    if (session.setPositionState && state.duration > 0) {
      try { session.setPositionState({ duration: state.duration, position: Math.min(state.position, state.duration), playbackRate: 1 }) } catch { /* Optional browser capability. */ }
    }
  }
  function bindSession() {
    const session = mediaSession()
    if (!session) return
    // Music controls should offer track navigation; clear skip handlers left by
    // other media sessions so systems can prefer previous/next track controls.
    for (const action of ['seekbackward', 'seekforward'] as MediaSessionAction[]) {
      try { session.setActionHandler(action, null) } catch { /* Unsupported media key. */ }
    }
    const handlers: Partial<Record<MediaSessionAction, MediaSessionActionHandler>> = {
      play: () => resume(), pause, nexttrack: () => next(), previoustrack: previous,
      seekto: detail => { if (detail.seekTime !== undefined) seek(detail.seekTime) },
      stop,
    }
    for (const [action, handler] of Object.entries(handlers)) {
      try { session.setActionHandler(action as MediaSessionAction, handler) } catch { /* Unsupported media key. */ }
    }
  }
  async function resume() {
    const target = audio, id = sequence
    if (!target) return
    state.restored = false
    state.error = ''
    try { await target.play() }
    catch (error) {
      if (!owns(target, id)) return
      state.waiting = false; state.playing = false
      if (error instanceof DOMException && error.name === 'AbortError') return
      state.error = error instanceof DOMException && error.name === 'NotAllowedError' ? 'blocked' : 'unsupported'
    }
  }
  function pause() { audio?.pause(); state.playing = false; state.waiting = false; syncSession() }
  function toggle() { if (state.playing || state.waiting) pause(); else void resume() }
  function seek(value: number) {
    if (!audio || !Number.isFinite(value) || !state.duration) return
    audio.currentTime = Math.min(state.duration, Math.max(0, value)); state.position = audio.currentTime; syncSession()
    if (typeof window !== 'undefined') window.dispatchEvent(new Event('kpanel:music-seek'))
  }
  function setVolume(value: number) {
    if (!Number.isFinite(value)) return
    state.volume = Math.min(1, Math.max(0, value)); state.muted = false
    if (audio) { audio.volume = state.volume; audio.muted = false }
  }
  function mute() { state.muted = !state.muted; if (audio) audio.muted = state.muted }

  async function persistSidecars(track: Track, assets: MusicSidecars, id: number) {
    if (id !== sequence || !state.current) return
    pendingSave = { track: { ...track }, assets }
    state.saveStatus = 'saving'
    controller?.abort(); clearTimeout(timer)
    const saving = new AbortController(); controller = saving
    timer = setTimeout(() => saving.abort(), 20000)
    try {
      const result = await deps.save(track, assets, saving.signal)
      if (id !== sequence || saving.signal.aborted || !state.current) return
      const paths = { ...(result.lyrics ? { lyrics: result.lyrics } : {}), ...(result.cover ? { cover: result.cover } : {}) }
      Object.assign(state.current, paths)
      const queued = state.queue.find(item => key(item) === key(track))
      if (queued) Object.assign(queued, paths)
      state.saveStatus = result.failed ? (result.saved || result.kept ? 'partial' : 'failed') : (result.saved ? 'saved' : 'kept')
      publishMetadata()
    } catch {
      if (id === sequence) state.saveStatus = 'failed'
    } finally {
      if (id === sequence) { clearTimeout(timer); if (saving.signal.aborted && state.saveStatus === 'saving') state.saveStatus = 'failed' }
    }
  }
  async function retrySave() {
    if (state.saveStatus === 'saving' || !pendingSave || !state.current || key(pendingSave.track) !== key(state.current)) return
    await persistSidecars(pendingSave.track, pendingSave.assets, sequence)
  }

  async function metadata(track: Track, id: number, signal: AbortSignal) {
    state.lyricsLoading = true
    if (track.needsRefresh) {
      let refreshed: Track | undefined
      try { refreshed = await deps.resolve(track, signal) } catch { /* Keep offline references for a later retry. */ }
      if (id !== sequence || !state.current) return
      if (!refreshed || signal.aborted) { state.lyricsLoading = false; state.lyricsFailed = true; return }
      track = refreshed
      state.current = { ...refreshed }
      const queued = state.queue.find(item => key(item) === key(track))
      if (queued) Object.assign(queued, refreshed, { needsRefresh: false })
    }
    // Independent optional resources: a broken sidecar does not discard embedded lyrics.
    const [tags, sidecar] = await Promise.allSettled([
      deps.tags(fileAPIForHost(track.hostId).contentUrl(track.entry.path), signal),
      track.lyrics ? deps.text(track, signal) : Promise.resolve(''),
    ])
    if (id !== sequence || !state.current) return
    if (signal.aborted) { state.lyricsLoading = false; state.lyricsFailed = true; return }
    const embedded = tags.status === 'fulfilled' ? tags.value : {}
    state.current = { ...state.current, title: readableMusicTag(embedded.title, track.title), artist: readableMusicTag(embedded.artist, track.artist) }
    const queued = state.queue.find(item => key(item) === key(track))
    if (queued) { queued.title = state.current.title; queued.artist = state.current.artist }
    state.album = readableMusicTag(embedded.album, '')
    if (embedded.picture) state.embeddedCover = URL.createObjectURL(embedded.picture)
    state.lyrics = ((sidecar.status === 'fulfilled' && sidecar.value) || embedded.lyrics || '').slice(0, 256 * 1024)
    state.lyricsFailed = Boolean(track.lyrics && sidecar.status === 'rejected' && !state.lyrics)
    state.lyricsLoading = false; publishMetadata()
    clearTimeout(timer)
    if (state.lyrics && cover.value) return
    if (state.current.title.length > 256 || state.current.artist.length > 256 || state.album.length > 256) return
    state.onlineStatus = 'loading'
    if (!state.current.artist && !state.duration) {
      await new Promise<void>(resolve => {
        const finish = () => { clearTimeout(waitTimer); unwatch(); signal.removeEventListener('abort', finish); resolve() }
        const unwatch = watch(() => state.duration, value => { if (value > 0) finish() })
        const waitTimer = setTimeout(finish, 1500)
        signal.addEventListener('abort', finish, { once: true })
      })
      if (id !== sequence || signal.aborted) return
    }
    // Separate budget: slow optional metadata never holds up audio playback.
    const onlineController = new AbortController()
    controller = onlineController
    timer = setTimeout(() => onlineController.abort(), 14000)
    try {
      const result = await deps.online({ title: state.current.title, artist: state.current.artist, album: state.album, duration: state.duration }, onlineController.signal)
      if (id !== sequence || onlineController.signal.aborted) return
      const assets: MusicSidecars = { ...(!state.lyrics && result.lyrics ? { lyrics: result.lyrics } : {}), ...(!cover.value && result.cover ? { cover: result.cover } : {}) }
      if (!state.lyrics && result.lyrics) { state.lyrics = result.lyrics.slice(0, 128 * 1024); state.lyricsFailed = false }
      if (!cover.value && result.cover && /^data:image\/(jpeg|png);base64,/.test(result.cover)) state.onlineCover = result.cover
      state.onlineSource = result.source ?? ''
      state.onlineStatus = result.matched ? 'matched' : 'empty'
      publishMetadata()
      if (assets.lyrics || assets.cover) await persistSidecars(track, assets, id)
    } catch {
      if (id === sequence) state.onlineStatus = 'failed'
    } finally {
      if (id === sequence) clearTimeout(timer)
    }
  }
  function play(track: Track, remember = true, startPosition = 0, autoplay = true) {
    if (remember && state.current && key(state.current) !== key(track)) history = [...history.slice(-199), { ...state.current }]
    const id = ++sequence
    releaseAudio(); releaseMetadata()
    state.position = startPosition; state.current = { ...track }; state.restored = !autoplay; state.duration = 0; state.error = ''
    const queuedKeys = new Set(state.queue.map(key))
    cycle = new Set([...cycle].filter(k => queuedKeys.has(k)))
    cycle.add(key(track))
    state.playing = false; state.waiting = autoplay; state.lyrics = ''; state.lyricsFailed = false; state.album = ''
    const target = deps.audio(); audio = target
    let pendingSeek = startPosition
    target.preload = 'metadata'; target.volume = state.volume; target.muted = state.muted
    target.onplaying = () => { if (owns(target, id)) { state.playing = true; state.waiting = false; syncSession() } }
    target.onpause = () => { if (owns(target, id)) { state.playing = false; syncSession() } }
    target.onwaiting = () => { if (owns(target, id)) state.waiting = true }
    target.oncanplay = () => { if (owns(target, id)) state.waiting = false }
    target.ontimeupdate = () => { if (owns(target, id)) { state.position = target.currentTime; syncSession() } }
    target.onloadedmetadata = () => { if (owns(target, id)) { state.duration = Number.isFinite(target.duration) ? target.duration : 0; if (pendingSeek > 0 && state.duration) { target.currentTime = Math.min(pendingSeek, state.duration); state.position = target.currentTime; pendingSeek = 0 }; syncSession() } }
    target.onerror = () => { if (owns(target, id)) { state.waiting = false; state.playing = false; state.error = target.error?.code === 2 ? 'network' : 'unsupported'; syncSession() } }
    target.onended = () => { if (owns(target, id)) { state.playing = false; state.waiting = false; next(true); syncSession() } }
    target.src = fileAPIForHost(track.hostId).contentUrl(track.entry.path)
    bindSession(); publishMetadata(); if (autoplay) void resume()
    controller = new AbortController(); timer = setTimeout(() => controller?.abort(), 8000)
    void metadata(track, id, controller.signal)
  }
  function restore(track: Track | undefined, position: number, repeat: Repeat, shuffle: boolean, queue?: Track[]) {
    if (!track) stop()
    state.queue = queue ?? (track ? [track] : []); history = []; remaining = []; cycle.clear(); priority = ''
    state.repeat = repeat; state.shuffle = shuffle
    if (track) play(track, false, position, false)
    else state.restored = true
  }
  function retry() { if (state.current) play(state.current, false, state.position) }
  function replaceQueue(tracks: Track[], start?: Track) {
    state.queue = [...new Map(tracks.slice(0, MUSIC_LIMIT).map(track => [key(track), track])).values()]
    history = []; remaining = []; cycle.clear(); priority = ''
    const track = start || state.queue[0]
    if (track) play(track, false)
  }
  function choose(track: Track) {
    if (state.current && key(state.current) === key(track)) { toggle(); return }
    if (!state.queue.some(t => key(t) === key(track))) {
      if (state.queue.length >= MUSIC_LIMIT) { state.notice = 'full'; return }
      state.queue.push(track)
    }
    play(track)
  }
  function enqueue(track: Track, nextUp = false) {
    const existing = state.queue.findIndex(t => key(t) === key(track))
    if (state.current && key(state.current) === key(track)) return
    if (existing < 0 && state.queue.length >= MUSIC_LIMIT) { state.notice = 'full'; return }
    if (existing >= 0) state.queue.splice(existing, 1)
    const at = nextUp ? Math.max(0, currentIndex.value + 1) : state.queue.length
    state.queue.splice(at, 0, track); remaining = []; if (nextUp) priority = key(track)
    state.notice = nextUp ? 'next' : 'added'
  }
  function next(ended = false) {
    if (!state.queue.length) return
    if (ended && state.repeat === 'one' && state.current) { play(state.current, false); return }
    const priorityTrack = state.queue.find(t => key(t) === priority)
    priority = ''
    if (priorityTrack) { play(priorityTrack); return }
    let track: Track | undefined
    if (state.shuffle) {
      remaining = remaining.filter(k => state.queue.some(t => key(t) === k) && !cycle.has(k))
      if (!remaining.length) {
        remaining = state.queue.filter(t => !cycle.has(key(t))).map(key)
        if (!remaining.length) {
          if (ended && state.repeat === 'off') return
          cycle.clear()
          if (state.current) cycle.add(key(state.current))
          remaining = state.queue.filter(t => !state.current || key(t) !== key(state.current)).map(key)
        }
      }
      const chosen = remaining.splice(Math.floor(deps.random() * remaining.length), 1)[0]
      track = state.queue.find(t => key(t) === chosen)
      if (!track && state.repeat !== 'off') track = state.current
    } else {
      const index = (currentIndex.value >= 0 ? currentIndex.value : detachedIndex) + 1
      track = state.queue[index]
      if (!track && (!ended || state.repeat === 'all')) track = state.queue[0]
    }
    if (track) play(track)
  }
  function previous() {
    if (state.position > 3) { seek(0); return }
    const previousTrack = history.pop()
    if (previousTrack) { play(previousTrack, false); return }
    const track = state.queue[Math.max(0, currentIndex.value - 1)]
    if (track) play(track, false)
  }
  function remove(track: Track) {
    const index = state.queue.findIndex(t => key(t) === key(track))
    if (index < 0) return
    if (state.current && key(state.current) === key(track)) detachedIndex = index - 1
    else if (currentIndex.value < 0 && index <= detachedIndex) detachedIndex--
    state.queue.splice(index, 1); remaining = remaining.filter(k => k !== key(track))
  }
  function move(track: Track, direction: number) {
    const index = state.queue.findIndex(t => key(t) === key(track)), to = index + direction
    if (index < 0 || to < 0 || to >= state.queue.length) return
    const [item] = state.queue.splice(index, 1); if (item) state.queue.splice(to, 0, item)
  }
  function clearQueue() { state.queue = state.current ? [{ ...state.current }] : []; remaining = []; history = []; priority = ''; cycle.clear(); if (state.current) cycle.add(key(state.current)) }
  function shuffle(value = !state.shuffle) { state.shuffle = value; remaining = []; history = []; cycle.clear(); if (state.current) cycle.add(key(state.current)) }
  function stop() {
    ++sequence; releaseAudio(); releaseMetadata(); history = []; remaining = []; cycle.clear(); priority = ''
    state.current = undefined; state.queue = []; state.playing = false; state.waiting = false; state.restored = false
    state.position = 0; state.duration = 0; state.lyrics = ''; state.error = ''; state.notice = ''; state.lyricsLoading = false; state.lyricsFailed = false
    const session = mediaSession()
    if (session) {
      session.metadata = null; session.playbackState = 'none'
      for (const action of ['play', 'pause', 'nexttrack', 'previoustrack', 'seekto', 'seekbackward', 'seekforward', 'stop'] as MediaSessionAction[]) {
        try { session.setActionHandler(action, null) } catch { /* Unsupported media key. */ }
      }
    }
  }
  return { state, currentIndex, cover, retrySave, play, restore, retry, choose, replaceQueue, enqueue, next, previous, toggle, pause, seek, setVolume, mute, remove, move, clearQueue, shuffle, stop }
}
const player = createMusicPlayer()
export function useMusicPlayer() { return player }
