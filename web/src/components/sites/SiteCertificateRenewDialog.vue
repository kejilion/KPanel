<script setup lang="ts">
import { LoaderCircle, ShieldCheck, TriangleAlert } from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import type { Site } from '@/types/api'

const props = defineProps<{
  open: boolean
  site?: Site
  renewing: boolean
  error?: string
  notice?: string
}>()

const emit = defineEmits<{
  close: []
  confirm: []
}>()

function close(): void {
  if (!props.renewing) emit('close')
}

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}
</script>

<template>
  <ModalDialog
    :open="open && Boolean(site)"
    :title="phrase(`手动申请证书 ${site?.primaryDomain || ''}`)"
    :description="phrase('确认后将调用 kejilion.sh 的 k ssl 为该域名重新申请证书；自动续签机制保持不变。')"
    size="small"
    @close="close"
  >
    <form id="site-certificate-renew-form" class="form-stack" @submit.prevent="emit('confirm')">
      <div v-if="error" class="inline-alert inline-alert--danger" role="alert">{{ phrase(error) }}</div>
      <div v-if="notice" class="inline-alert inline-alert--info" role="status">{{ phrase(notice) }}</div>
      <div class="inline-alert inline-alert--warning">
        <TriangleAlert :size="17" />
        <span>{{ phrase('申请期间 Nginx 会短暂停止（通常约 1 分钟），经 Nginx 访问的网站，包括通过本域名访问的面板，会暂时无法打开。') }}</span>
      </div>
      <p>{{ phrase('需要域名已解析到本机且 80 端口可从公网访问。申请失败时原证书保持不变。') }}</p>
    </form>
    <template #footer>
      <button class="button button--secondary" type="button" :disabled="renewing" @click="close">
        {{ phrase('取消') }}
      </button>
      <button class="button button--primary" type="submit" form="site-certificate-renew-form" :disabled="renewing">
        <LoaderCircle v-if="renewing" class="spin" :size="16" />
        <ShieldCheck v-else :size="16" />
        {{ phrase(renewing ? '正在申请…' : '开始申请') }}
      </button>
    </template>
  </ModalDialog>
</template>
