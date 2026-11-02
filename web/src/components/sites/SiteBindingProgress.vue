<script setup lang="ts">
import { useI18n } from '@/i18n'
import type { SiteInstallationProgress } from '@/types/api'

defineProps<{ progress: SiteInstallationProgress; compactSuccess?: boolean }>()
const i18n = useI18n()
</script>

<template>
  <section class="site-binding-progress" role="status" aria-live="polite">
    <p>{{ progress.message }}</p>
    <progress v-if="!compactSuccess || progress.status !== 'succeeded'" :value="progress.progress" max="100" :aria-label="progress.message" />
    <ol v-if="(!compactSuccess || progress.status !== 'succeeded') && progress.events?.length">
      <li v-for="(event, index) in progress.events.slice(-5)" :key="`${event.at}-${index}`">{{ event.message }}</li>
    </ol>
    <p><span v-if="!compactSuccess || progress.status !== 'succeeded'">{{ i18n.t('panelAccess.taskNotice') }} </span><RouterLink to="/sites">{{ i18n.t('panelAccess.viewTask') }}</RouterLink></p>
  </section>
</template>

<style scoped>
.site-binding-progress { padding: 12px; border: 1px solid var(--border); border-radius: var(--radius); font-size: 14px; line-height: 1.5; overflow-wrap: anywhere; }
.site-binding-progress p { margin: 4px 0; }
.site-binding-progress progress { width: 100%; margin: 8px 0; accent-color: var(--brand); }
.site-binding-progress ol { margin: 8px 0; padding-left: 24px; }
</style>
