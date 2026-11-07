<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ArrowLeft, Maximize } from '@lucide/vue'
import { activateModal, deactivateModal } from '@/components/common/modalStack'

const props = defineProps<{ src: string; poster?: string; title: string; backLabel: string; fullscreenLabel: string; fallbackLabel: string; errorLabel: string }>()
const emit = defineEmits<{ close: [] }>()
const root = ref<HTMLElement>()
const video = ref<HTMLVideoElement>()
const fallback = ref(false)
const failed = ref(false)
let entered = false
let disposed = false
let opener: HTMLElement | null = null
const modalId = Symbol('cinema-player')

async function enterFullscreen() {
  const element = root.value
  try {
    if (!element?.requestFullscreen) { fallback.value = true; return }
    await element.requestFullscreen()
    if (disposed) {
      if (document.fullscreenElement === element) await document.exitFullscreen()
      return
    }
    entered = true
    fallback.value = false
  } catch { if (!disposed) fallback.value = true }
}
function fullscreenChanged() {
  if (document.fullscreenElement === root.value) entered = true
  else if (entered && !disposed) emit('close')
}
function keydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && !document.fullscreenElement) {
    event.preventDefault()
    event.stopPropagation()
    emit('close')
  }
  if (event.key === 'Tab') {
    const controls = Array.from(root.value?.querySelectorAll<HTMLElement>('button, video') ?? [])
    const first = controls[0], last = controls.at(-1)
    if (event.shiftKey && (document.activeElement === first || document.activeElement === root.value)) { event.preventDefault(); last?.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }
}
onMounted(() => {
  opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
  activateModal(modalId)
  document.addEventListener('fullscreenchange', fullscreenChanged)
  root.value?.focus()
  // Vue mounts in the originating click's microtask, retaining user activation.
  void enterFullscreen()
})
onBeforeUnmount(() => {
  disposed = true
  deactivateModal(modalId)
  document.removeEventListener('fullscreenchange', fullscreenChanged)
  video.value?.pause()
  video.value?.removeAttribute('src')
  video.value?.load()
  if (root.value && document.fullscreenElement === root.value) void document.exitFullscreen().catch(() => undefined)
  void nextTick(() => { if (opener?.isConnected) opener.focus({ preventScroll: true }) })
})
</script>

<template>
  <Teleport to="body">
    <section ref="root" class="cinema-player" role="dialog" aria-modal="true" :aria-label="title" tabindex="-1" @keydown="keydown">
      <header>
        <button type="button" @click="emit('close')"><ArrowLeft :size="20" /> {{ backLabel }}</button>
        <strong>{{ title }}</strong>
        <button type="button" :aria-label="fullscreenLabel" @click="enterFullscreen"><Maximize :size="20" /></button>
      </header>
      <video ref="video" :src="props.src" :poster="poster" controls autoplay playsinline preload="metadata" @error="failed = true" @loadeddata="failed = !video?.videoWidth; failed && video?.pause()" />
      <p v-if="failed" class="cinema-player__notice" role="alert">{{ errorLabel }}</p>
      <p v-else-if="fallback" class="cinema-player__notice" role="status">{{ fallbackLabel }}</p>
    </section>
  </Teleport>
</template>

<style scoped>
.cinema-player { position: fixed; inset: 0; z-index: 10000; display: flex; flex-direction: column; background: #050709; color: #f5f7fa; outline: none; }
.cinema-player header { display: flex; align-items: center; gap: 18px; padding: 12px 18px; background: #10151c; }
.cinema-player header strong { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
.cinema-player button { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 44px; flex-shrink: 0; padding: 8px 12px; border: 1px solid #45515f; border-radius: var(--radius-sm); background: #1c2530; color: #fff; font: inherit; font-size: 14px; cursor: pointer; }
.cinema-player button:focus-visible { outline: 2px solid #bce8f1; outline-offset: 3px; }
.cinema-player video { flex: 1; min-height: 0; width: 100%; object-fit: contain; }
.cinema-player__notice { margin: 0; padding: 10px 18px; background: #1c2530; font-size: 14px; line-height: 1.5; }
@media (max-width: 480px) { .cinema-player header { gap: 8px; padding: 8px; } }
</style>
