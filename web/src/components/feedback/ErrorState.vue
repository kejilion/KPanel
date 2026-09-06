<script setup lang="ts">
import { CircleAlert, RotateCw } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from '@/i18n'
import ProblemReportButton from '@/components/problem-report/ProblemReportButton.vue'
import type { ReportError, ReportFeature } from '@/components/problem-report/report'

const props = withDefaults(
  defineProps<{
    title?: string
    message: string
    retryLabel?: string
    reportError?: ReportError
    reportFeature?: ReportFeature
  }>(),
  {
    title: undefined,
    retryLabel: undefined,
  },
)

const i18n = useI18n()
const resolvedTitle = computed(() => props.title || i18n.t('state.errorTitle'))
const resolvedRetryLabel = computed(() => props.retryLabel || i18n.t('common.retry'))

defineEmits<{
  retry: []
}>()
</script>

<template>
  <div class="error-state" role="alert">
    <div class="error-state__icon" aria-hidden="true">
      <CircleAlert :size="24" :stroke-width="1.8" />
    </div>
    <div>
      <h3>{{ resolvedTitle }}</h3>
      <p>{{ message }}</p>
      <div class="error-state__actions">
        <button class="button button--secondary button--small" type="button" @click="$emit('retry')">
          <RotateCw :size="15" />
          {{ resolvedRetryLabel }}
        </button>
        <ProblemReportButton source="error" :feature="reportFeature || 'unknown'" :error="reportError" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.error-state__actions { display: flex; flex-wrap: wrap; gap: 0.5rem; }
</style>
