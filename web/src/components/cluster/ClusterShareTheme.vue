<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from '@/i18n'
import { useTheme } from '@/stores/theme'
import { shareThemeLabels } from '@/lib/shareThemeLabels'
import { shareThemeModel, shareThemeModelV2, shareThemeProtocol, shareThemeURL, type RegionCenters } from '@/lib/shareThemes'
import type { PublicClusterShareSnapshot } from '@/types/api'

const props = defineProps<{ snapshot?: PublicClusterShareSnapshot; errorMessage?: string }>()
const { t, locale } = useI18n()
const { resolved } = useTheme()
const frame = ref<HTMLIFrameElement>()
const ready = ref(false)
let protocol: 1 | 2 = 1
let centers: RegionCenters = {}
const fallback = ref(false)
const failed = ref(false)
const height = ref(720)
const url = computed(() => shareThemeURL(props.snapshot?.theme))
const name = computed(() => props.snapshot?.theme?.name[locale.value] || props.snapshot?.theme?.name['en-US'] || '')
let timer: ReturnType<typeof setTimeout> | undefined
function clearTimer() { if (timer) clearTimeout(timer); timer = undefined }
function stop(error = false) { clearTimer(); fallback.value = true; ready.value = false; failed.value = error }
function send() {
  if (!ready.value || !props.snapshot) return
  const now = new Date(), target = frame.value?.contentWindow
  if (protocol === 2) target?.postMessage({ source: 'kpanel-share', type: 'snapshot', schema: 2, locale: locale.value, mode: resolved.value,
    labels: shareThemeLabels(locale.value), data: shareThemeModelV2(props.snapshot, locale.value, now, t, centers) }, '*')
  else target?.postMessage({ source: 'kpanel-share', type: 'snapshot', schema: 1,
    locale: locale.value, mode: resolved.value, data: shareThemeModel(props.snapshot, locale.value, now, t) }, '*')
}
function message(event: MessageEvent) {
  if (!frame.value || event.source !== frame.value.contentWindow || event.origin !== 'null') return
  if (event.data?.source !== 'kpanel-share-theme') return
  if (ready.value && event.data?.type === 'resize') {
    if (Number.isInteger(event.data.height) && event.data.height >= 320 && event.data.height <= 32768) height.value = event.data.height
    return
  }
  if (event.data?.type !== 'ready' || ready.value) return
  protocol = shareThemeProtocol(event.data.protocol)
  clearTimer(); ready.value = true; send()
  // Map anchors ship in the globe chunk; fetch them only for protocol 2 themes, then resend.
  if (protocol === 2 && !Object.keys(centers).length) void import('@/components/cluster/globeData').then(module => { centers = module.regionCenters; send() }).catch(() => { /* Themes treat null coordinates as unknown. */ })
}
watch(url, () => {
  clearTimer(); ready.value = false; protocol = 1; fallback.value = false; failed.value = false; height.value = 720
  if (url.value) timer = setTimeout(() => stop(true), 12_000)
}, { immediate: true })
watch([() => props.snapshot, locale, resolved], send)
onMounted(() => window.addEventListener('message', message))
onBeforeUnmount(() => { clearTimer(); window.removeEventListener('message', message) })
</script>

<template>
  <p v-if="ready && errorMessage" class="share-theme__notice" role="alert">{{ errorMessage }}</p>
  <section v-if="url && !fallback" class="share-theme" :aria-label="name">
    <div class="share-theme__bar">
      <span>{{ ready ? name : t('cluster.themes.loadingTheme') }}</span>
      <button class="button button--secondary" type="button" @click="stop()">{{ t('cluster.themes.useDefault') }}</button>
    </div>
    <iframe ref="frame" :key="url" :src="url" :title="name" sandbox="allow-scripts" referrerpolicy="no-referrer"
      :class="{ 'is-ready': ready }" :style="{ '--theme-height': `${height}px` }" :tabindex="ready ? 0 : -1" :aria-hidden="!ready" @error="stop(true)" />
  </section>
  <p v-if="failed" class="share-theme__notice" role="status">{{ t('cluster.themes.fallback') }}</p>
  <slot v-if="!ready" />
</template>

<style scoped>
.share-theme__bar { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.75rem; margin-bottom: 1rem; font-size: 0.875rem; color: var(--text-soft); }
iframe { display: block; visibility: hidden; border: 0; width: 100%; height: 0; background: var(--bg); border-radius: var(--radius-lg); }
iframe.is-ready { visibility: visible; height: var(--theme-height); }
.share-theme__notice { font-size: 0.875rem; color: var(--text-soft); }
</style>
