<script setup lang="ts">
import { LoaderCircle, Trash2, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { Site } from '@/types/api'

const props = defineProps<{
  open: boolean
  site?: Site
  deleting: boolean
  error?: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()

function close(): void {
  if (!props.deleting) emit('close')
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}
</script>

<template>
  <ModalDialog
    :open="open && Boolean(site)"
    :title="phrase(`删除 ${site?.primaryDomain || ''}`)"
    :description="phrase('确认后将严格调用 kejilion.sh 的 k web del 删除该域名站点。')"
    size="small"
    @close="close"
  >
    <form id="site-delete-form" class="form-stack" @submit.prevent="emit('confirm')">
      <div v-if="error" class="inline-alert inline-alert--danger" role="alert">{{ phrase(error) }}</div>
      <div class="inline-alert inline-alert--danger">
        <TriangleAlert :size="17" />
        <span>{{ phrase('将删除该域名的网站目录、Nginx 配置、证书和同名数据库（若存在），此操作无法从面板撤销。') }}</span>
      </div>
    </form>
    <template #footer>
      <button class="button button--secondary" type="button" :disabled="deleting" @click="close">
        {{ phrase('取消') }}
      </button>
      <button
        class="button button--danger"
        type="submit"
        form="site-delete-form"
        :disabled="deleting"
      >
        <LoaderCircle v-if="deleting" class="spin" :size="16" />
        <Trash2 v-else :size="16" />
        {{ phrase(deleting ? '正在删除…' : '确认删除站点') }}
      </button>
    </template>
  </ModalDialog>
</template>
