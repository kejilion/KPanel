<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import {
  Clock3,
  File,
  LoaderCircle,
  RefreshCw,
  Share2,
  Trash2,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import type { FileShareListItem } from '@/types/api'

const emit = defineEmits<{
  close: []
}>()

const shares = ref<FileShareListItem[]>([])
const loading = ref(true)
const stoppingID = ref('')
const errorMessage = ref('')
const statusMessage = ref('')
let controller: AbortController | undefined
let loadSequence = 0

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

function expiryLabel(share: FileShareListItem): string {
  if (!share.expiresAt) return '永久有效'
  const expiresAt = new Date(share.expiresAt)
  if (!Number.isNaN(expiresAt.getTime()) && expiresAt.getTime() <= Date.now()) return '已过期'
  return `有效期至 ${formatDateTime(share.expiresAt)}`
}

function friendlyError(_reason: unknown, action: 'load' | 'delete'): string {
  return action === 'delete'
    ? '暂时无法停止分享，请稍后重试。'
    : '暂时无法读取分享列表，请稍后重试。'
}

async function loadShares(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await api.files.shares(controller.signal)
    if (sequence !== loadSequence) return
    shares.value = result.shares
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (sequence !== loadSequence) return
    errorMessage.value = friendlyError(reason, 'load')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function stopShare(share: FileShareListItem): Promise<void> {
  if (typeof window !== 'undefined' && !window.confirm(phrase(
    '停止这个分享后，现有链接会立即失效。确认继续吗？',
  ))) return

  stoppingID.value = share.id
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    await api.files.deleteShare(share.id)
    shares.value = shares.value.filter((item) => item.id !== share.id)
    statusMessage.value = '分享已停止。'
  } catch (reason) {
    if (reason instanceof ApiError && reason.status === 404) {
      shares.value = shares.value.filter((item) => item.id !== share.id)
      statusMessage.value = '分享已不存在。'
    } else {
      errorMessage.value = friendlyError(reason, 'delete')
    }
  } finally {
    stoppingID.value = ''
  }
}

function requestClose(): void {
  if (!stoppingID.value) emit('close')
}

onMounted(() => void loadShares())
onBeforeUnmount(() => {
  loadSequence += 1
  controller?.abort()
})
</script>

<template>
  <ModalDialog :open="true" :title="phrase('分享管理')" size="small" @close="requestClose">
    <div class="file-share-manager">
      <div class="file-share-manager__intro">
        <Share2 :size="18" />
        <p>{{ phrase('集中管理文件分享。即使源文件已经移动或删除，也可以在这里停止旧分享。') }}</p>
      </div>

      <div v-if="loading" class="file-share-manager__state" role="status" aria-live="polite">
        <LoaderCircle class="spin" :size="20" />
        <span>{{ phrase('正在读取分享列表…') }}</span>
      </div>

      <template v-else>
        <div v-if="errorMessage && !shares.length" class="file-share-manager__state file-share-manager__state--error" role="alert">
          <strong>{{ phrase('无法读取分享列表') }}</strong>
          <p>{{ phrase(errorMessage) }}</p>
          <button type="button" @click="loadShares"><RefreshCw :size="15" />{{ phrase('重试') }}</button>
        </div>

        <div v-else-if="!shares.length" class="file-share-manager__state">
          <span class="file-share-manager__empty-icon"><File :size="23" /></span>
          <strong>{{ phrase('还没有文件分享') }}</strong>
          <p>{{ phrase('从文件的更多菜单中即可创建分享。') }}</p>
        </div>

        <ul v-else class="file-share-manager__list" :aria-label="phrase('现有文件分享')">
          <li v-for="share in shares" :key="share.id">
            <span class="file-share-manager__file-icon"><File :size="18" /></span>
            <span class="file-share-manager__details">
              <strong :title="share.path">{{ share.path }}</strong>
              <small :class="{ 'is-expired': expiryLabel(share) === '已过期' }">
                <Clock3 :size="13" />{{ phrase(expiryLabel(share)) }}
              </small>
            </span>
            <button
              class="file-share-manager__stop"
              type="button"
              :disabled="Boolean(stoppingID)"
              :aria-label="phrase(`停止分享 ${share.path}`)"
              @click="stopShare(share)"
            >
              <LoaderCircle v-if="stoppingID === share.id" class="spin" :size="15" />
              <Trash2 v-else :size="15" />
              {{ phrase(stoppingID === share.id ? '正在停止…' : '停止') }}
            </button>
          </li>
        </ul>

        <p v-if="statusMessage" class="file-share-manager__message" role="status" aria-live="polite">
          {{ phrase(statusMessage) }}
        </p>
        <p v-if="errorMessage && shares.length" class="file-share-manager__error" role="alert">
          {{ phrase(errorMessage) }}
        </p>
      </template>
    </div>

    <template #footer>
      <button class="button button--secondary" type="button" :disabled="Boolean(stoppingID)" @click="requestClose">
        {{ phrase('关闭') }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.file-share-manager {
  display: grid;
  gap: 14px;
  font-size: 14px;
}

.file-share-manager__intro {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  padding: 11px 12px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 11px;
}

.file-share-manager__intro svg {
  flex: 0 0 auto;
  margin-top: 1px;
  color: var(--brand-strong);
}

.file-share-manager__intro p,
.file-share-manager__state p {
  margin: 0;
  font-size: 13px;
  line-height: 1.55;
}

.file-share-manager__state {
  display: grid;
  min-height: 128px;
  place-items: center;
  align-content: center;
  gap: 8px;
  color: var(--text-soft);
  text-align: center;
}

.file-share-manager__state p {
  color: var(--muted);
}

.file-share-manager__state button {
  display: inline-flex;
  min-height: 40px;
  align-items: center;
  gap: 7px;
  margin-top: 4px;
  padding: 0 13px;
  color: var(--brand-strong);
  background: var(--surface);
  border: 1px solid var(--brand-muted);
  border-radius: 9px;
  cursor: pointer;
  font-size: 14px;
}

.file-share-manager__state--error strong {
  color: var(--danger);
}

.file-share-manager__empty-icon {
  display: grid;
  width: 48px;
  height: 48px;
  place-items: center;
  color: var(--muted);
  background: var(--surface-subtle);
  border-radius: 14px;
}

.file-share-manager__list {
  display: grid;
  max-height: min(52vh, 430px);
  gap: 8px;
  padding: 0 3px 0 0;
  margin: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
  list-style: none;
}

.file-share-manager__list li {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  padding: 10px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 11px;
}

.file-share-manager__file-icon {
  display: grid;
  width: 38px;
  height: 38px;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: 10px;
}

.file-share-manager__details {
  display: grid;
  min-width: 0;
  gap: 4px;
}

.file-share-manager__details strong {
  overflow-wrap: anywhere;
  font-size: 14px;
  line-height: 1.4;
}

.file-share-manager__details small {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.35;
}

.file-share-manager__details small.is-expired {
  color: var(--danger);
}

.file-share-manager__stop {
  display: inline-flex;
  min-height: 38px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 0 10px;
  color: var(--danger);
  background: transparent;
  border: 1px solid color-mix(in srgb, var(--danger) 30%, var(--border));
  border-radius: 9px;
  cursor: pointer;
  font-size: 13px;
  font-weight: 700;
}

.file-share-manager__stop:hover:not(:disabled) {
  color: #fff;
  background: var(--danger);
  border-color: var(--danger);
}

.file-share-manager__stop:disabled {
  cursor: wait;
  opacity: 0.55;
}

.file-share-manager__message,
.file-share-manager__error {
  margin: 0;
  padding: 9px 11px;
  border-radius: 9px;
  font-size: 13px;
  line-height: 1.5;
}

.file-share-manager__message {
  color: var(--brand-strong);
  background: var(--brand-soft);
}

.file-share-manager__error {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 9%, transparent);
}

.spin {
  animation: file-share-manager-spin 0.85s linear infinite;
}

@keyframes file-share-manager-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 460px) {
  .file-share-manager__list li {
    grid-template-columns: 34px minmax(0, 1fr);
  }

  .file-share-manager__file-icon {
    width: 34px;
    height: 34px;
  }

  .file-share-manager__stop {
    grid-column: 1 / -1;
    min-height: 40px;
  }
}
</style>
