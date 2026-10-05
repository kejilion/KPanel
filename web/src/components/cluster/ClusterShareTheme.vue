<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from '@/i18n'
import { useTheme } from '@/stores/theme'
import { shareThemeLabels } from '@/lib/shareThemeLabels'
import { shareThemeModel, shareThemeModelV2, shareThemeProtocol, shareThemeURL } from '@/lib/shareThemes'
import type { ShareThemeAssets } from '@/lib/shareThemeAssets'
import type { PublicClusterShareSnapshot } from '@/types/api'

const props = defineProps<{ snapshot?: PublicClusterShareSnapshot; errorMessage?: string }>()
// `immersive`: the theme declared it owns the whole page, so the host hides its own header, frame and footer.
const emit = defineEmits<{ immersive: [value: boolean]; refresh: [] }>()
const { t, locale } = useI18n()
const { resolved, setTheme } = useTheme()
const frame = ref<HTMLIFrameElement>()
const ready = ref(false)
let protocol: 1 | 2 = 1
let assets: Partial<ShareThemeAssets> = {}
let assetKey: string | null = null
const fallback = ref(false)
const failed = ref(false)
const immersive = ref(false)
const height = ref(720)
const url = computed(() => shareThemeURL(props.snapshot?.theme))
const name = computed(() => props.snapshot?.theme?.name[locale.value] || props.snapshot?.theme?.name['en-US'] || '')
let timer: ReturnType<typeof setTimeout> | undefined
function clearTimer() { if (timer) clearTimeout(timer); timer = undefined }
function stop(error = false) { clearTimer(); fallback.value = true; ready.value = false; immersive.value = false; failed.value = error }
function send() {
  if (!ready.value || !props.snapshot) return
  const now = new Date(), target = frame.value?.contentWindow
  if (protocol === 2) target?.postMessage({ source: 'kpanel-share', type: 'snapshot', schema: 2, locale: locale.value, mode: resolved.value,
    labels: shareThemeLabels(locale.value), data: shareThemeModelV2(props.snapshot, locale.value, now, t, assets) }, '*')
  else target?.postMessage({ source: 'kpanel-share', type: 'snapshot', schema: 1,
    locale: locale.value, mode: resolved.value, data: shareThemeModel(props.snapshot, locale.value, now, t) }, '*')
}
// Flags, system marks and map anchors are fetched only for protocol 2 themes, and again only
// when the set of countries or systems changes; each resolution resends the snapshot.
function loadAssets() {
  const snapshot = props.snapshot
  if (!ready.value || protocol !== 2 || !snapshot) return
  const key = snapshot.items.map(host => `${host.location.countryCode || ''}:${host.os || ''}`).sort().join('|')
  if (key === assetKey) return
  assetKey = key
  void import('@/lib/shareThemeAssets').then(module => module.loadShareThemeAssets(snapshot)).then(next => { if (assetKey === key) { assets = next; send() } })
    .catch(() => { /* Themes treat missing flags, marks and coordinates as unknown. */ })
}
function message(event: MessageEvent) {
  if (!frame.value || event.source !== frame.value.contentWindow || event.origin !== 'null') return
  if (event.data?.source !== 'kpanel-share-theme') return
  if (ready.value && event.data?.type === 'resize') {
    if (Number.isInteger(event.data.height) && event.data.height >= 320 && event.data.height <= 32768) height.value = event.data.height
    return
  }
  // Controls the host would otherwise draw (refresh, light/dark, back to default) arrive as explicit, whitelisted actions.
  if (ready.value && immersive.value && event.data?.type === 'action') {
    const { action, mode } = event.data
    if (action === 'refresh') emit('refresh')
    else if (action === 'set-mode' && (mode === 'light' || mode === 'dark')) setTheme(mode)
    else if (action === 'use-default') stop()
    return
  }
  if (event.data?.type !== 'ready' || ready.value) return
  protocol = shareThemeProtocol(event.data.protocol)
  immersive.value = protocol === 2 && event.data.chrome === 'self'
  clearTimer(); ready.value = true; send()
  loadAssets()
}
watch(url, () => {
  clearTimer(); ready.value = false; immersive.value = false; protocol = 1; assetKey = null; fallback.value = false; failed.value = false; height.value = 720
  if (url.value) timer = setTimeout(() => stop(true), 12_000)
}, { immediate: true })
watch(immersive, value => emit('immersive', value))
watch([() => props.snapshot, locale, resolved], () => { send(); loadAssets() })
onMounted(() => window.addEventListener('message', message))
onBeforeUnmount(() => { clearTimer(); emit('immersive', false); window.removeEventListener('message', message) })
</script>

<template>
  <p v-if="ready && errorMessage && !immersive" class="share-theme__notice" role="alert">{{ errorMessage }}</p>
  <section v-if="url && !fallback" class="share-theme" :class="{ 'is-immersive': immersive }" :aria-label="name">
    <div v-if="!immersive" class="share-theme__bar">
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
/* The theme paints the entire viewport and scrolls inside its own frame, so no host surface is visible around it. */
.is-immersive iframe.is-ready { position: fixed; inset: 0; z-index: 10; width: 100vw; height: 100vh; height: 100dvh; border-radius: 0; }
.share-theme__notice { font-size: 0.875rem; color: var(--text-soft); }
</style>
