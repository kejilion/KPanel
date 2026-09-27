<script setup lang="ts">
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { customWallpaperFromID, desktopWallpaperImage, DESKTOP_WALLPAPERS, useDesktopWallpaper, wallpaperFocusPosition, type DesktopWallpaperID } from '@/lib/desktopWallpapers'
import { scenePackFromWallpaper } from '@/lib/scenePacks'
import { useSceneMotionPreference } from '@/lib/desktopScenes/motionPreference'
import '@/styles/desktopWallpaperSurface.css'

const props = defineProps<{ wallpaperId: DesktopWallpaperID, revision: number, covered: boolean }>()
const emit = defineEmits<{ cameras: [ids: string[]], failed: [] }>()
const ScenePack = defineAsyncComponent(() => import('./DesktopScenePack.vue'))
const motion = useSceneMotionPreference()
const wallpapers = useDesktopWallpaper()
const descriptor = computed(() => {
  const pack = scenePackFromWallpaper(props.wallpaperId)
  const customID = customWallpaperFromID(props.wallpaperId)
  const custom = customID && wallpapers.customWallpapers.value.find((wallpaper) => wallpaper.id === customID)
  return {
    id: props.wallpaperId, pack, revision: props.revision,
    key: `${props.wallpaperId}:${props.revision}:${motion.reducedMotion.value}`,
    live: Boolean(pack) && !motion.reducedMotion.value,
    ...desktopWallpaperImage(props.wallpaperId),
    position: custom ? wallpaperFocusPosition(custom) : document.documentElement.style.getPropertyValue('--desktop-wallpaper-position') || 'center',
  }
})
// Selection can change during departure; only the latest target is mounted under black.
const shown = shallowRef(descriptor.value)
const phase = ref<'idle' | 'leaving' | 'waiting' | 'entering'>('idle')
const image = ref<HTMLImageElement>()
const scene = ref<{ nextCamera: () => void }>()
const imageFailed = ref(false)
const current = computed(() => shown.value.key === descriptor.value.key && phase.value !== 'leaving')
let timer: number | undefined
let generation = 0
let disposed = false

function cancelTimer(): void {
  if (timer !== undefined) window.clearTimeout(timer)
  timer = undefined
}

function arm(callback: () => void, milliseconds: number): void {
  cancelTimer()
  timer = window.setTimeout(callback, milliseconds)
}

function reveal(): void {
  if (disposed || phase.value !== 'waiting') return
  phase.value = 'entering'
  arm(() => { phase.value = 'idle' }, 900)
}

async function showLatest(): Promise<void> {
  cancelTimer()
  const token = ++generation
  shown.value = descriptor.value
  imageFailed.value = false
  phase.value = 'waiting'
  await nextTick()
  if (disposed || token !== generation || phase.value !== 'waiting') return
  // A stalled poster must not prevent a ready scene or its loading feedback from appearing.
  arm(reveal, 2000)
  const node = image.value
  if (node?.complete && node.naturalWidth > 0) reveal()
  else if (node && typeof node.decode === 'function') {
    void node.decode().then(() => {
      if (!disposed && token === generation && node === image.value) reveal()
    }).catch(() => { /* load/error handlers provide fallback without revealing an old image. */ })
  }
}

function finishImmediately(): void {
  cancelTimer()
  generation++
  shown.value = descriptor.value
  imageFailed.value = false
  phase.value = 'idle'
}

watch([descriptor, motion.systemReducedMotion], ([target], [previousTarget]) => {
  if (motion.systemReducedMotion.value || document.visibilityState === 'hidden') {
    finishImmediately()
  } else if (target.key === previousTarget.key) {
    // A late metadata refresh is not another selection and must not restart the black handoff.
    if (phase.value !== 'leaving' && shown.value.key === target.key) {
      shown.value = { ...shown.value, position: target.position }
    }
  } else if (phase.value === 'leaving') {
    // The opaque handoff will read descriptor again, including rapid scene → still → scene.
  } else if (phase.value === 'waiting') {
    void showLatest()
  } else if (phase.value === 'entering' || shown.value.live || target.live) {
    generation++
    phase.value = 'leaving'
    arm(() => { void showLatest() }, 700)
  } else {
    shown.value = target
    imageFailed.value = false
  }
})

function onVeilEnd(event: TransitionEvent): void {
  if (event.target !== event.currentTarget || event.propertyName !== 'opacity') return
  const opacity = Number(getComputedStyle(event.currentTarget as Element).opacity)
  // An already queued end event can belong to the reveal that this selection interrupted.
  if (phase.value === 'leaving' && opacity >= .999) void showLatest()
  else if (phase.value === 'entering' && opacity <= .001) { cancelTimer(); phase.value = 'idle' }
}

function onImageLoad(event: Event): void {
  if (event.target === image.value && current.value) reveal()
}

function onImageError(event: Event): void {
  if (event.target !== image.value) return
  const fallback = shown.value.src !== shown.value.url ? shown.value.url
    : !shown.value.pack && shown.value.src !== DESKTOP_WALLPAPERS[0].src ? DESKTOP_WALLPAPERS[0].src : undefined
  if (fallback) shown.value = { ...shown.value, src: fallback }
  else { imageFailed.value = true; reveal() }
}

function onCameras(ids: string[]): void {
  if (!current.value) return
  reveal()
  emit('cameras', ids)
}

function onFailed(): void {
  if (!current.value) return
  reveal()
  emit('failed')
}

onBeforeUnmount(() => { disposed = true; generation++; cancelTimer() })
defineExpose({ nextCamera: () => { if (current.value) scene.value?.nextCamera() } })
</script>

<template>
  <div class="desktop-wallpaper-host" :data-wallpaper-phase="phase" aria-hidden="true">
    <Transition name="desktop-wallpaper-fade" :css="phase === 'idle' && !motion.systemReducedMotion.value">
      <div
        :key="shown.key"
        class="desktop-wallpaper-surface desktop__wallpaper-image"
        :class="{ 'desktop-wallpaper-surface--scene': shown.live }"
        :data-wallpaper="shown.id"
        :style="{ '--desktop-wallpaper-image': `url(&quot;${shown.src}&quot;)`, '--desktop-wallpaper-position': shown.position }"
      >
        <img ref="image" class="desktop-wallpaper-surface__image" :class="{ 'desktop-wallpaper-surface__image--failed': imageFailed }" :src="shown.src" alt="" decoding="async" fetchpriority="high" @load="onImageLoad" @error="onImageError" />
        <ScenePack v-if="shown.pack" :key="shown.key" ref="scene" :pack-id="shown.pack" :covered="covered || phase === 'leaving'" @cameras="onCameras" @failed="onFailed" />
      </div>
    </Transition>
    <div class="desktop-wallpaper-handoff" :class="{ 'desktop-wallpaper-handoff--black': phase === 'leaving' || phase === 'waiting', 'desktop-wallpaper-handoff--reveal': phase === 'entering' }" @transitionend="onVeilEnd" />
  </div>
</template>
