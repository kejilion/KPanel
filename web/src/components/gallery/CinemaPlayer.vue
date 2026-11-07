<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ArrowLeft, Maximize, Minimize, Pause, Play, RotateCcw, RotateCw, Volume2, VolumeX } from '@lucide/vue'
import { activateModal, deactivateModal } from '@/components/common/modalStack'
import { formatGalleryDuration } from '@/lib/gallery'

const props = defineProps<{
  src: string; poster?: string; title: string; startPosition?: number
  backLabel: string; fullscreenLabel: string; fallbackLabel: string; errorLabel: string
  labels: { play: string; pause: string; loading: string; retry: string; blocked: string; rewind: string; forward: string; mute: string; unmute: string; volume: string; seek: string; speed: string; exitFullscreen: string }
}>()
const emit = defineEmits<{ close: []; progress: [value: { position: number; duration: number; completed: boolean }, flush: boolean] }>()
const root = ref<HTMLElement>(), video = ref<HTMLVideoElement>()
const fallback = ref(false), failed = ref(false), blocked = ref(false), waiting = ref(true)
const playing = ref(false), fullscreen = ref(false), completed = ref(false)
const position = ref(0), duration = ref(0), volume = ref(1), muted = ref(false), speed = ref(1)
const time = computed(() => formatGalleryDuration(position.value) + ' / ' + formatGalleryDuration(duration.value))
let disposed = false, ready = false, pendingSeek = props.startPosition ?? 0, lastSent = '', lastProgressAt = 0, lastFullscreenExit = -1000
let opener: HTMLElement | null = null
const modalId = Symbol('cinema-player')

function publish(flush = false) {
  const target = video.value
  if (!ready || !target || !Number.isFinite(target.duration) || target.duration <= 0 || target.duration > 31536000) return
  const value = { position: Math.min(target.duration, Math.max(0, target.currentTime)), duration: target.duration, completed: completed.value }
  const signature = JSON.stringify(value)
  if (signature === lastSent || (!flush && Date.now() - lastProgressAt < 30000)) return
  lastSent = signature; lastProgressAt = Date.now()
  emit('progress', value, flush)
}
async function toggleFullscreen() {
  const element = root.value
  try {
    if (document.fullscreenElement === element) { await document.exitFullscreen(); return }
    if (!element?.requestFullscreen) { fallback.value = true; return }
    await element.requestFullscreen()
    if (disposed) { if (document.fullscreenElement === element) await document.exitFullscreen(); return }
    fullscreen.value = true; fallback.value = false
  } catch { if (!disposed) fallback.value = true }
}
function fullscreenChanged() {
  const wasFullscreen = fullscreen.value
  fullscreen.value = Boolean(root.value && document.fullscreenElement === root.value)
  if (wasFullscreen && !fullscreen.value) lastFullscreenExit = performance.now()
}
async function play() {
  if (disposed || !video.value) return
  if (completed.value) { completed.value = false; seek(0) }
  blocked.value = false
  try { await video.value.play() }
  catch (error) {
    if (disposed || (error instanceof DOMException && error.name === 'AbortError')) return
    waiting.value = false; playing.value = false
    if (error instanceof DOMException && error.name === 'NotAllowedError') blocked.value = true
    else failed.value = true
  }
}
function toggle() { if (playing.value) video.value?.pause(); else void play() }
function seek(value: number) {
  if (!video.value || !Number.isFinite(value) || duration.value <= 0) return
  completed.value = false
  position.value = Math.min(duration.value, Math.max(0, value))
  video.value.currentTime = position.value
}
function metadata() {
  const target = video.value
  if (!target || !Number.isFinite(target.duration) || target.duration <= 0) return
  duration.value = target.duration
  if (pendingSeek > 0) target.currentTime = Math.min(pendingSeek, Math.max(0, target.duration - .1))
  pendingSeek = 0
  position.value = target.currentTime
}
function loaded() {
  if (!video.value?.videoWidth) { failed.value = true; waiting.value = false; video.value?.pause(); return }
  ready = true; waiting.value = false; position.value = video.value.currentTime
  publish(true)
}
function tick() { position.value = video.value?.currentTime ?? position.value; publish() }
function paused() { playing.value = false; publish(true) }
function ended() { completed.value = true; playing.value = false; waiting.value = false; publish(true) }
function close() { publish(true); emit('close') }
function retry() {
  // A first-request failure has not consumed the requested resume position yet.
  pendingSeek = pendingSeek > 0 ? pendingSeek : position.value
  failed.value = false; blocked.value = false; waiting.value = true; ready = false
  video.value?.load(); void play()
}
function setVolume(value: number) {
  if (!video.value) return
  volume.value = Math.max(0, Math.min(1, value)); muted.value = false
  video.value.volume = volume.value; video.value.muted = false
}
function mute() { muted.value = !muted.value; if (video.value) video.value.muted = muted.value }
function rate(value: number) { speed.value = value; if (video.value) video.value.playbackRate = value }
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault(); event.stopPropagation()
    if (document.fullscreenElement === root.value) void toggleFullscreen()
    else if (performance.now() - lastFullscreenExit > 250) close()
    return
  }
  if (event.key === 'Tab') {
    const controls = Array.from(root.value?.querySelectorAll<HTMLElement>('button:not(:disabled), input, select') ?? [])
    const first = controls[0], last = controls.at(-1)
    if (event.shiftKey && (document.activeElement === first || document.activeElement === root.value)) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
    return
  }
  if ((event.target as HTMLElement)?.closest('button,input,select')) return
  const key = event.key.toLowerCase()
  if ([' ', 'enter', 'f', 'm', 'arrowleft', 'arrowright', 'arrowup', 'arrowdown'].includes(key)) event.preventDefault()
  if (key === ' ' || key === 'enter') toggle()
  else if (key === 'f') void toggleFullscreen()
  else if (key === 'm') mute()
  else if (key === 'arrowleft') seek(position.value - 10)
  else if (key === 'arrowright') seek(position.value + 10)
  else if (key === 'arrowup') setVolume(volume.value + .1)
  else if (key === 'arrowdown') setVolume(volume.value - .1)
}
function visibility() { if (document.hidden) publish(true) }
function pagehide() { publish(true) }
onMounted(() => {
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  activateModal(modalId)
  document.addEventListener('fullscreenchange', fullscreenChanged)
  document.addEventListener('visibilitychange', visibility)
  window.addEventListener('pagehide', pagehide)
  root.value?.focus()
  void toggleFullscreen(); void play()
})
onBeforeUnmount(() => {
  publish(true); disposed = true; deactivateModal(modalId)
  document.removeEventListener('fullscreenchange', fullscreenChanged)
  document.removeEventListener('visibilitychange', visibility)
  window.removeEventListener('pagehide', pagehide)
  video.value?.pause(); video.value?.removeAttribute('src'); video.value?.load()
  if (root.value && document.fullscreenElement === root.value) void document.exitFullscreen().catch(() => undefined)
  void nextTick(() => { if (opener?.isConnected) opener.focus({ preventScroll: true }) })
})
</script>

<template>
  <Teleport to="body">
    <section ref="root" class="cinema-player" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1" @keydown="keydown">
      <header><button data-action="back" type="button" @click="close"><ArrowLeft :size="20" /> {{ backLabel }}</button><strong>{{ title }}</strong></header>
      <div class="cinema-player__stage">
        <video ref="video" :src="src" :poster="poster" playsinline preload="metadata" @loadedmetadata="metadata" @loadeddata="loaded" @timeupdate="tick" @seeked="publish(true)" @pause="paused" @playing="playing = true; waiting = false; blocked = false" @waiting="waiting = true" @canplay="waiting = false" @ended="ended" @error="failed = true; waiting = false; playing = false" @click="toggle" @dblclick="toggleFullscreen" />
        <div v-if="failed" class="cinema-player__message" role="alert"><p>{{ errorLabel }}</p><button type="button" @click="retry"><RotateCcw :size="18" />{{ labels.retry }}</button></div>
        <div v-else-if="blocked" class="cinema-player__message" role="status"><p>{{ labels.blocked }}</p><button type="button" @click="play"><Play :size="20" />{{ labels.play }}</button></div>
        <p v-else-if="waiting" class="cinema-player__loading" role="status">{{ labels.loading }}</p>
      </div>
      <p v-if="fallback" class="cinema-player__notice" role="status">{{ fallbackLabel }}</p>
      <footer class="cinema-player__controls">
        <input class="cinema-player__seek" type="range" min="0" :max="duration || 1" step=".1" :value="position" :disabled="!duration" :aria-label="labels.seek" :aria-valuetext="time" @input="seek(Number(($event.target as HTMLInputElement).value))" />
        <div class="cinema-player__buttons">
          <button data-action="toggle" type="button" :aria-label="playing ? labels.pause : labels.play" @click="toggle"><Pause v-if="playing" :size="22" /><Play v-else :size="22" /></button>
          <button type="button" :aria-label="labels.rewind" @click="seek(position - 10)"><RotateCcw :size="20" /></button>
          <button type="button" :aria-label="labels.forward" @click="seek(position + 10)"><RotateCw :size="20" /></button>
          <span class="cinema-player__time">{{ time }}</span>
          <div class="cinema-player__volume"><button type="button" :aria-label="muted ? labels.unmute : labels.mute" @click="mute"><VolumeX v-if="muted || volume === 0" :size="20" /><Volume2 v-else :size="20" /></button><input type="range" min="0" max="1" step=".05" :value="muted ? 0 : volume" :aria-label="labels.volume" @input="setVolume(Number(($event.target as HTMLInputElement).value))" /></div>
          <select :value="speed" :aria-label="labels.speed" @change="rate(Number(($event.target as HTMLSelectElement).value))"><option v-for="value in [.5, .75, 1, 1.25, 1.5, 2]" :key="value" :value="value">{{ value }}×</option></select>
          <button data-action="fullscreen" type="button" :aria-label="fullscreen ? labels.exitFullscreen : fullscreenLabel" @click="toggleFullscreen"><Minimize v-if="fullscreen" :size="20" /><Maximize v-else :size="20" /></button>
        </div>
      </footer>
    </section>
  </Teleport>
</template>

<style scoped>
.cinema-player { position: fixed; inset: 0; z-index: 10000; display: flex; flex-direction: column; background: #050709; color: #f5f7fa; outline: none; }
.cinema-player header { display: flex; align-items: center; gap: 18px; padding: 10px 18px; background: #10151c; }
.cinema-player header strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
.cinema-player button { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 44px; flex-shrink: 0; padding: 8px 12px; border: 1px solid #45515f; border-radius: var(--radius-sm); background: #1c2530; color: #fff; font: inherit; font-size: 14px; cursor: pointer; }
.cinema-player button:focus-visible, .cinema-player input:focus-visible, .cinema-player select:focus-visible { outline: 2px solid #bce8f1; outline-offset: 3px; }
.cinema-player__stage { flex: 1; position: relative; min-height: 0; display: flex; }
.cinema-player video { width: 100%; min-height: 0; object-fit: contain; }
.cinema-player__message { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 18px; background: #050709ed; padding: 24px; text-align: center; font-size: 14px; line-height: 1.5; }
.cinema-player__loading { position: absolute; inset: 50% 16px auto; margin: 0; text-align: center; background: #10151ce6; padding: 12px; font-size: 14px; }
.cinema-player__controls { padding: 10px 18px 14px; background: #10151c; }
.cinema-player__seek { display: block; width: 100%; min-height: 24px; margin: 0 0 8px; accent-color: #bce8f1; cursor: pointer; }
.cinema-player__buttons { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.cinema-player__time { font-size: 13px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.cinema-player__volume { display: flex; align-items: center; gap: 8px; margin-inline-start: auto; }
.cinema-player__volume input { width: 90px; accent-color: #bce8f1; }
.cinema-player select { min-height: 44px; padding: 8px; border: 1px solid #45515f; border-radius: var(--radius-sm); background: #1c2530; color: #fff; font: inherit; font-size: 14px; }
.cinema-player__notice { margin: 0; padding: 10px 18px; background: #1c2530; font-size: 14px; line-height: 1.5; }
@media (max-width: 560px) { .cinema-player header { gap: 8px; padding: 8px; } .cinema-player__controls { padding: 8px; } .cinema-player__volume input { display: none; } .cinema-player__time { flex: 1; } .cinema-player__volume { margin-inline-start: 0; } }
</style>
