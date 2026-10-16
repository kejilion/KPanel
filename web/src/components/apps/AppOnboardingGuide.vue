<script setup lang="ts">
import { ArrowUpRight } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import type { AppMarketItem } from '@/types/api'
import LocalAppManager from './LocalAppManager.vue'

defineProps<{ open: boolean; localApps: AppMarketItem[] }>()
const emit = defineEmits<{ close: []; changed: []; manage: [item: AppMarketItem] }>()
</script>

<template>
  <ModalDialog
    :open="open"
    title="应用入驻"
    description="在本机添加自己的 Docker 应用，安装验证后一键提交到官方市场，让所有用户都能使用。"
    size="large"
    @close="emit('close')"
  >
    <ol class="onboarding-flow" aria-label="入驻流程">
      <li><strong>添加配置</strong><small>粘贴或按模板编写 .conf</small></li>
      <li><strong>安装验证</strong><small>在应用市场完整走一遍安装、更新、卸载</small></li>
      <li><strong>提交官方</strong><small>一键在 GitHub 发起 PR，审核后上架</small></li>
    </ol>

    <LocalAppManager :apps="localApps" @changed="emit('changed')" @manage="emit('manage', $event)" />

    <template #footer>
      <a class="button button--secondary" href="https://dev.kejilion.sh/" target="_blank" rel="noopener noreferrer">
        <ArrowUpRight :size="15" /> 开发者指南
      </a>
      <button class="button button--primary" type="button" @click="emit('close')">关闭</button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.onboarding-flow {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin: 0 0 16px;
  padding: 0;
  list-style: none;
  counter-reset: onboarding-step;
}

.onboarding-flow li {
  display: grid;
  gap: 2px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  counter-increment: onboarding-step;
}

.onboarding-flow strong::before {
  content: counter(onboarding-step) ". ";
  color: var(--brand);
}

.onboarding-flow small {
  color: var(--text-secondary);
}

@media (max-width: 560px) {
  .onboarding-flow {
    grid-template-columns: 1fr;
  }
}
</style>
