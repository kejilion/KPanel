<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AuditView from '@/views/AuditView.vue'
import JobsView from '@/views/JobsView.vue'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/ActivityView/en-US').then((module) => module.default)
  : import('@/i18n/pages/ActivityView/zh-TW').then((module) => module.default))

type ActivityTab = 'jobs' | 'audit'

const route = useRoute()
const router = useRouter()
const activeTab = computed<ActivityTab>(() => (route.query.tab === 'audit' ? 'audit' : 'jobs'))

function selectTab(tab: ActivityTab): void {
  void router.replace({ path: '/activity', query: { tab } })
}
</script>

<template>
  <div class="page activity-page">
    <div class="tab-bar activity-page__tabs" role="tablist" aria-label="活动记录类型">
      <button
        type="button"
        role="tab"
        :aria-selected="activeTab === 'jobs'"
        :class="{ 'is-active': activeTab === 'jobs' }"
        @click="selectTab('jobs')"
      >
        变更任务
      </button>
      <button
        type="button"
        role="tab"
        :aria-selected="activeTab === 'audit'"
        :class="{ 'is-active': activeTab === 'audit' }"
        @click="selectTab('audit')"
      >
        安全审计
      </button>
    </div>
    <component :is="activeTab === 'audit' ? AuditView : JobsView" />
  </div>
</template>

<style scoped>
.activity-page {
  align-content: start;
  gap: 18px;
}

.activity-page__tabs {
  align-self: flex-start;
}
</style>
