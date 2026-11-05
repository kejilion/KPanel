<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch, watchEffect, type WatchStopHandle } from 'vue'
import { RouterView } from 'vue-router'
import { useRoute } from 'vue-router'
import ToastHost from '@/components/feedback/ToastHost.vue'
import MusicMiniPlayer from '@/components/music/MusicMiniPlayer.vue'
import { useMusicPlayer } from '@/stores/musicPlayer'
import { useI18n } from '@/i18n'
import { installPhraseLocalization, usePhraseCatalog } from '@/i18n/phrase'
import { useDesktopMode } from '@/stores/desktopMode'
import { useSession } from '@/stores/session'
import { useSiteBranding } from '@/stores/branding'
import { useLiveNotifications } from '@/composables/useLiveNotifications'
import { stopPreferencesSync } from '@/lib/preferencesSync'

const route = useRoute()
const i18n = useI18n()
const desktop = useDesktopMode()
const session = useSession()
const music = useMusicPlayer()
// Navigation does not own audio. Authentication and app teardown do.
watch(() => session.state.authenticated ? session.state.user?.id : undefined, (id, previous) => {
  if ((!id || (previous && id !== previous)) && music.state.current) music.stop()
}, { flush: 'sync', immediate: true })
onBeforeUnmount(() => { stopPreferencesSync(); music.stop() })
const branding = useSiteBranding()
const brandIcons = Array.from(document.querySelectorAll<HTMLLinkElement>('link[rel="icon"], link[rel="apple-touch-icon"]')).map((link) => ({
  link, href: link.href, type: link.type, sizes: link.getAttribute('sizes'),
}))
useLiveNotifications(computed(() => session.state.authenticated && !route.meta.public))
let stopPhraseLocalization: WatchStopHandle | null = null

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/shared/en-US').then((module) => module.default)
  : import('@/i18n/pages/shared/zh-TW').then((module) => module.default))

onMounted(() => {
  const root = document.querySelector<HTMLElement>('#app')
  if (root) stopPhraseLocalization = installPhraseLocalization(root)
})

onBeforeUnmount(() => stopPhraseLocalization?.())

watchEffect(() => {
  const title = desktop.mode.value === 'desktop' && !route.meta.public
    ? i18n.t('desktop.aboutTitle')
    : route.meta.titleKey ? i18n.t(route.meta.titleKey) : ''
  document.title = title ? `${title} · ${branding.name.value}` : branding.name.value
  document.querySelector<HTMLMetaElement>('meta[name="application-name"]')?.setAttribute('content', branding.name.value)
  brandIcons.forEach(({ link, href, type, sizes }) => {
    const custom = branding.value.value.icon
    link.href = custom || href
    link.type = custom ? 'image/png' : type
    if (custom || sizes === null) link.removeAttribute('sizes')
    else link.setAttribute('sizes', sizes)
  })
})
</script>

<template>
  <RouterView />
  <MusicMiniPlayer v-if="session.state.authenticated && !route.meta.public" />
  <ToastHost />
</template>
