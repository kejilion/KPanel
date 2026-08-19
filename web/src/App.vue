<script setup lang="ts">
import { onBeforeUnmount, onMounted, watchEffect, type WatchStopHandle } from 'vue'
import { RouterView } from 'vue-router'
import { useRoute } from 'vue-router'
import ToastHost from '@/components/feedback/ToastHost.vue'
import { useI18n } from '@/i18n'
import { installPhraseLocalization, usePhraseCatalog } from '@/i18n/phrase'

const route = useRoute()
const i18n = useI18n()
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
  const title = route.meta.titleKey ? i18n.t(route.meta.titleKey) : ''
  document.title = title ? `${title} · KPanel` : 'KPanel'
})
</script>

<template>
  <RouterView />
  <ToastHost />
</template>
