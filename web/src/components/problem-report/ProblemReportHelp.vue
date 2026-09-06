<script setup lang="ts">
import { FileText } from '@lucide/vue'
import ProblemReportButton from './ProblemReportButton.vue'
import { useProblemReportText } from './i18n'
const text = useProblemReportText()
</script>

<template>
  <section class="settings-section panel-card">
    <header class="settings-section__header">
      <span><FileText :size="19" aria-hidden="true" /></span>
      <div><h2>{{ text('helpTitle') }}</h2><p>{{ text('help') }}</p></div>
    </header>
    <ProblemReportButton source="manual" feature="settings" />
  </section>
</template>
