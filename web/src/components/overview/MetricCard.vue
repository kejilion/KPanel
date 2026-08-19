<script setup lang="ts">
import type { Component } from 'vue'
import { computed } from 'vue'
import { useI18n } from '@/i18n'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { clampPercent } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    label: string
    value: string
    detail?: string
    percent?: number
    icon: Component
    tone?: 'brand' | 'blue' | 'violet' | 'amber'
  }>(),
  {
    detail: '',
    percent: undefined,
    tone: 'brand',
  },
)

const normalizedPercent = computed(() => clampPercent(props.percent))
const { locale } = useI18n()
const displayLabel = computed(() => {
  locale.value
  phraseCatalogVersion.value
  return translatePhrase(props.label)
})
</script>

<template>
  <article class="metric-card">
    <div class="metric-card__top">
      <span class="metric-card__icon" :class="`metric-card__icon--${tone}`">
        <component :is="icon" :size="19" :stroke-width="1.9" aria-hidden="true" />
      </span>
      <span>{{ displayLabel }}</span>
    </div>
    <strong>{{ value }}</strong>
    <div v-if="percent !== undefined" class="progress-track" :aria-label="`${displayLabel} ${value}`">
      <span :style="{ width: `${normalizedPercent}%` }" />
    </div>
    <small>{{ detail || '　' }}</small>
  </article>
</template>
