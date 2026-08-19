<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Check, Globe2 } from '@lucide/vue'
import { useI18n, type SupportedLocale } from '@/i18n'
import type { MessageKey } from '@/i18n/messages/zh-CN'

withDefaults(defineProps<{ compact?: boolean }>(), { compact: false })

const i18n = useI18n()
const root = ref<HTMLElement>()
const open = ref(false)
const busy = ref(false)

function localeLabelKey(locale: SupportedLocale): MessageKey {
  if (locale === 'zh-CN') return 'common.locale.zhCN'
  if (locale === 'zh-TW') return 'common.locale.zhTW'
  return 'common.locale.enUS'
}

function closeOnOutsideClick(event: PointerEvent): void {
  if (open.value && event.target instanceof Node && !root.value?.contains(event.target)) {
    open.value = false
  }
}

function closeOnEscape(event: KeyboardEvent): void {
  if (event.key === 'Escape') open.value = false
}

onMounted(() => {
  document.addEventListener('pointerdown', closeOnOutsideClick)
  document.addEventListener('keydown', closeOnEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', closeOnOutsideClick)
  document.removeEventListener('keydown', closeOnEscape)
})

async function selectLocale(locale: SupportedLocale): Promise<void> {
  if (busy.value || locale === i18n.locale.value) {
    open.value = false
    return
  }
  busy.value = true
  await i18n.setLocale(locale)
  busy.value = false
  open.value = false
}
</script>

<template>
  <div ref="root" class="language-selector" :class="{ 'language-selector--compact': compact }">
    <button
      class="icon-button language-selector__trigger"
      type="button"
      :aria-label="i18n.t('common.language')"
      :aria-expanded="open"
      aria-haspopup="menu"
      :title="i18n.t('common.language')"
      @click="open = !open"
    >
      <Globe2 :size="18" aria-hidden="true" />
      <span>{{ i18n.localeOption.value.shortLabel }}</span>
    </button>
    <div v-if="open" class="language-selector__menu" role="menu">
      <button
        v-for="option in i18n.localeOptions"
        :key="option.id"
        type="button"
        role="menuitemradio"
        :aria-checked="option.id === i18n.locale.value"
        :disabled="busy"
        @click="selectLocale(option.id)"
      >
        <span>{{ i18n.t(localeLabelKey(option.id)) }}</span>
        <Check v-if="option.id === i18n.locale.value" :size="16" aria-hidden="true" />
      </button>
    </div>
  </div>
</template>
