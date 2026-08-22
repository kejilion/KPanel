<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import {
  Check,
  CheckCircle2,
  Copy,
  ExternalLink,
  File,
  Link2,
  LoaderCircle,
  RefreshCw,
  Trash2,
  TriangleAlert,
} from '@lucide/vue'
import ModalDialog from '@/components/common/ModalDialog.vue'
import { phraseCatalogVersion, translatePhrase } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatBytes, formatDateTime } from '@/lib/format'
import type { FileEntry, FileShareAdminView, FileShareExpiry } from '@/types/api'

const props = defineProps<{
  entry: FileEntry
}>()

const emit = defineEmits<{
  close: []
}>()

const loading = ref(true)
const mutation = ref<'create' | 'delete'>()
const share = ref<FileShareAdminView | null>(null)
const expiresIn = ref<FileShareExpiry>('7d')
const errorMessage = ref('')
const statusMessage = ref('')
const copiedLink = ref<'page' | 'direct'>()
let controller: AbortController | undefined
let loadSequence = 0

function phrase(value: string): string {
  phraseCatalogVersion.value
  return translatePhrase(value)
}

const linksAvailable = computed(() => Boolean(
  share.value?.linksAvailable && share.value.sharePath && share.value.directPath,
))
const safeImageMimes = new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif'])
const canOpenDirect = computed(() => Boolean(
  props.entry.mime && safeImageMimes.has(props.entry.mime.toLowerCase()),
))
const sharePageURL = computed(() => linksAvailable.value ? absoluteURL(share.value?.sharePath || '') : '')
const directURL = computed(() => linksAvailable.value ? absoluteURL(share.value?.directPath || '') : '')
const expiryDescription = computed(() => share.value?.expiresAt
  ? phrase(`有效期至 ${formatDateTime(share.value.expiresAt)}`)
  : phrase('永久有效'))

function absoluteURL(path: string): string {
  if (!path) return ''
  try {
    const origin = typeof window === 'undefined' ? 'http://localhost' : window.location.origin
    return new URL(path, origin).toString()
  } catch {
    return path
  }
}

function inferredExpiry(expiresAt?: string): FileShareExpiry {
  if (!expiresAt) return 'never'
  const remaining = new Date(expiresAt).getTime() - Date.now()
  return remaining > 14 * 24 * 60 * 60 * 1000 ? '30d' : '7d'
}

function friendlyError(reason: unknown, action: 'load' | 'create' | 'delete'): string {
  if (reason instanceof ApiError && reason.code === 'file_share_limit_reached') {
    return '文件分享数量已达上限，请在分享管理中停止不再使用的分享后重试。'
  }
  if (reason instanceof ApiError && (reason.code === 'file_share_changed' || reason.status === 409)) {
    return '分享状态或文件已发生变化，请刷新目录后重新打开分享。'
  }
  if (reason instanceof ApiError && reason.status === 429) {
    return '文件分享操作较频繁，请稍后重试。'
  }
  if (action === 'load') return '暂时无法读取分享状态，请稍后重试。'
  if (action === 'delete') return '暂时无法停止分享，请稍后重试。'
  return '暂时无法生成分享链接，请稍后重试。'
}

async function loadShare(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  statusMessage.value = ''
  copiedLink.value = undefined
  try {
    const result = await api.files.share(
      props.entry.path,
      props.entry.resourceVersion,
      controller.signal,
    )
    if (sequence !== loadSequence) return
    share.value = result.share
    if (result.share) expiresIn.value = inferredExpiry(result.share.expiresAt)
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (sequence !== loadSequence) return
    share.value = null
    errorMessage.value = friendlyError(reason, 'load')
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

async function createShare(): Promise<void> {
  if (share.value && typeof window !== 'undefined' && !window.confirm(phrase(
    '重新生成会立即停用旧链接。确认继续吗？',
  ))) return

  mutation.value = 'create'
  errorMessage.value = ''
  statusMessage.value = ''
  copiedLink.value = undefined
  try {
    share.value = await api.files.createShare({
      path: props.entry.path,
      expectedResourceVersion: props.entry.resourceVersion,
      expectedShareID: share.value?.id || '',
      expiresIn: expiresIn.value,
    })
  } catch (reason) {
    errorMessage.value = friendlyError(reason, 'create')
  } finally {
    mutation.value = undefined
  }
}

async function deleteShare(): Promise<void> {
  const current = share.value
  if (!current) return
  if (typeof window !== 'undefined' && !window.confirm(phrase(
    '停止后，分享页、文件直链和正在使用这些链接的网站引用都会失效。确认停止吗？',
  ))) return

  mutation.value = 'delete'
  errorMessage.value = ''
  statusMessage.value = ''
  try {
    await api.files.deleteShare(current.id)
    share.value = null
    expiresIn.value = '7d'
    copiedLink.value = undefined
    statusMessage.value = '分享已停止。'
  } catch (reason) {
    errorMessage.value = friendlyError(reason, 'delete')
  } finally {
    mutation.value = undefined
  }
}

async function copyLink(kind: 'page' | 'direct', value: string): Promise<void> {
  if (!value) return
  errorMessage.value = ''
  try {
    if (!navigator.clipboard?.writeText) throw new Error('clipboard_unavailable')
    await navigator.clipboard.writeText(value)
    copiedLink.value = kind
    return
  } catch {
    const input = document.getElementById(`file-share-${kind}-link`)
    if (input instanceof HTMLInputElement) {
      input.focus()
      input.select()
      try {
        if (document.execCommand?.('copy')) {
          copiedLink.value = kind
          return
        }
      } catch {
        // Keep the visible link selected for manual copying.
      }
    }
    errorMessage.value = '浏览器未允许自动复制，链接已选中，请手动复制。'
  }
}

function requestClose(): void {
  if (!mutation.value) emit('close')
}

watch(
  () => [props.entry.path, props.entry.resourceVersion] as const,
  () => void loadShare(),
  { immediate: true },
)

onBeforeUnmount(() => {
  loadSequence += 1
  controller?.abort()
})
</script>

<template>
  <ModalDialog :open="true" :title="phrase('文件分享')" size="small" @close="requestClose">
    <div class="file-share-dialog">
      <section class="file-share-dialog__file" :aria-label="phrase('分享文件')">
        <span class="file-share-dialog__file-icon"><File :size="21" /></span>
        <span>
          <strong>{{ entry.name }}</strong>
          <small>{{ formatBytes(entry.sizeBytes) }}<template v-if="entry.mime"> · {{ entry.mime }}</template></small>
        </span>
      </section>

      <div v-if="loading" class="file-share-dialog__state" role="status" aria-live="polite">
        <LoaderCircle class="spin" :size="20" />
        <span>{{ phrase('正在读取分享状态…') }}</span>
      </div>

      <template v-else>
        <section v-if="share && !linksAvailable" class="file-share-dialog__active" role="status">
          <CheckCircle2 :size="19" />
          <span>
            <strong>{{ phrase('文件正在分享') }}</strong>
            <small>{{ expiryDescription }}</small>
          </span>
        </section>

        <div v-if="share && !linksAvailable" class="file-share-dialog__notice">
          <Link2 :size="18" />
          <p>
            <strong>{{ phrase('出于安全考虑，旧链接不会保存在 KPanel 中。') }}</strong>
            {{ phrase('如需再次复制，请重新生成；旧链接会立即失效。') }}
          </p>
        </div>

        <div v-else-if="!share" class="file-share-dialog__notice">
          <TriangleAlert :size="18" />
          <p>
            <strong>{{ phrase('任何持有链接的人都可以访问此文件。') }}</strong>
            {{ phrase('文件被修改、移动或删除后，链接将无法访问；分享记录仍可在“分享管理”中停止。') }}
          </p>
        </div>

        <fieldset v-if="!share || !linksAvailable" class="file-share-dialog__expiry" :disabled="Boolean(mutation)">
          <legend>{{ phrase(share ? '新链接有效期' : '有效期') }}</legend>
          <label :class="{ 'is-selected': expiresIn === '7d' }">
            <input v-model="expiresIn" type="radio" value="7d" />
            <span><strong>{{ phrase('7 天') }}</strong><small>{{ phrase('推荐') }}</small></span>
          </label>
          <label :class="{ 'is-selected': expiresIn === '30d' }">
            <input v-model="expiresIn" type="radio" value="30d" />
            <span><strong>{{ phrase('30 天') }}</strong><small>{{ phrase('长期使用') }}</small></span>
          </label>
          <label :class="{ 'is-selected': expiresIn === 'never' }">
            <input v-model="expiresIn" type="radio" value="never" />
            <span><strong>{{ phrase('永久') }}</strong><small>{{ phrase('手动停止') }}</small></span>
          </label>
        </fieldset>

        <section v-if="linksAvailable" class="file-share-dialog__links" :aria-label="phrase('新生成的分享链接')">
          <div class="file-share-dialog__one-time" role="status">
            <CheckCircle2 :size="18" />
            <p><strong>{{ phrase('分享已创建') }}</strong>{{ phrase('链接仅在本次显示，请现在复制保存。') }}</p>
          </div>

          <label for="file-share-page-link">{{ phrase('分享页面') }}</label>
          <div class="file-share-dialog__link-row">
            <input id="file-share-page-link" :value="sharePageURL" readonly spellcheck="false" />
            <button
              class="button button--secondary"
              type="button"
              :aria-label="phrase(copiedLink === 'page' ? '分享页面链接已复制' : '复制分享页面链接')"
              @click="copyLink('page', sharePageURL)"
            >
              <Check v-if="copiedLink === 'page'" :size="15" />
              <Copy v-else :size="15" />
              {{ phrase(copiedLink === 'page' ? '已复制' : '复制') }}
            </button>
            <a class="button button--secondary" :href="sharePageURL" target="_blank" rel="noopener noreferrer">
              <ExternalLink :size="15" />{{ phrase('打开') }}
            </a>
          </div>

          <label for="file-share-direct-link">{{ phrase('文件直链') }}</label>
          <div class="file-share-dialog__link-row">
            <input id="file-share-direct-link" :value="directURL" readonly spellcheck="false" />
            <button
              class="button button--secondary"
              type="button"
              :aria-label="phrase(copiedLink === 'direct' ? '文件直链已复制' : '复制文件直链')"
              @click="copyLink('direct', directURL)"
            >
              <Check v-if="copiedLink === 'direct'" :size="15" />
              <Copy v-else :size="15" />
              {{ phrase(copiedLink === 'direct' ? '已复制' : '复制') }}
            </button>
            <a v-if="canOpenDirect" class="button button--secondary" :href="directURL" target="_blank" rel="noopener noreferrer">
              <ExternalLink :size="15" />{{ phrase('打开') }}
            </a>
          </div>
          <small v-if="canOpenDirect" class="file-share-dialog__hint">{{ phrase('图片直链可直接用于网站、Markdown 或其他支持外部图片的地方。') }}</small>
          <small v-else class="file-share-dialog__hint">{{ phrase('非图片文件的直链会直接下载文件。') }}</small>
        </section>

        <p v-if="statusMessage" class="file-share-dialog__message" role="status" aria-live="polite">
          {{ phrase(statusMessage) }}
        </p>
        <p v-if="errorMessage" class="file-share-dialog__error" role="alert">
          {{ phrase(errorMessage) }}
        </p>
        <button
          v-if="errorMessage && !share"
          class="file-share-dialog__retry"
          type="button"
          :disabled="loading"
          @click="loadShare"
        >
          <RefreshCw :size="15" />{{ phrase('重试') }}
        </button>
      </template>
    </div>

    <template #footer>
      <div class="file-share-dialog__footer">
        <button
          v-if="share"
          class="button button--danger"
          type="button"
          :disabled="Boolean(mutation)"
          @click="deleteShare"
        >
          <LoaderCircle v-if="mutation === 'delete'" class="spin" :size="16" />
          <Trash2 v-else :size="16" />
          {{ phrase(mutation === 'delete' ? '正在停止…' : '停止分享') }}
        </button>
        <span />
        <button class="button button--secondary" type="button" :disabled="Boolean(mutation)" @click="requestClose">
          {{ phrase(linksAvailable ? '完成' : '取消') }}
        </button>
        <button
          v-if="!loading && (!share || !linksAvailable)"
          class="button button--primary"
          type="button"
          :disabled="Boolean(mutation) || Boolean(errorMessage && !share)"
          @click="createShare"
        >
          <LoaderCircle v-if="mutation === 'create'" class="spin" :size="16" />
          <RefreshCw v-else-if="share" :size="16" />
          <Link2 v-else :size="16" />
          {{ phrase(mutation === 'create' ? '正在生成…' : share ? '重新生成链接' : '创建分享') }}
        </button>
      </div>
    </template>
  </ModalDialog>
</template>

<style scoped>
.file-share-dialog {
  display: grid;
  gap: 16px;
  font-size: 14px;
}

.file-share-dialog__file {
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr);
  align-items: center;
  gap: 11px;
  padding: 12px;
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 12px;
}

.file-share-dialog__file-icon {
  display: grid;
  width: 42px;
  height: 42px;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: 11px;
}

.file-share-dialog__file strong,
.file-share-dialog__file small,
.file-share-dialog__active strong,
.file-share-dialog__active small {
  display: block;
}

.file-share-dialog__file strong {
  overflow-wrap: anywhere;
  color: var(--text);
  line-height: 1.4;
}

.file-share-dialog__file small,
.file-share-dialog__active small,
.file-share-dialog__hint {
  margin-top: 3px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.45;
}

.file-share-dialog__state {
  display: flex;
  min-height: 100px;
  align-items: center;
  justify-content: center;
  gap: 9px;
  color: var(--text-soft);
}

.file-share-dialog__notice,
.file-share-dialog__one-time,
.file-share-dialog__active {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 11px;
}

.file-share-dialog__notice > svg,
.file-share-dialog__one-time > svg,
.file-share-dialog__active > svg {
  flex: 0 0 auto;
  margin-top: 1px;
  color: var(--brand-strong);
}

.file-share-dialog__notice p,
.file-share-dialog__one-time p {
  margin: 0;
  line-height: 1.55;
}

.file-share-dialog__notice strong,
.file-share-dialog__one-time strong {
  display: block;
  margin-bottom: 2px;
  color: var(--text);
}

.file-share-dialog__active {
  align-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-color: var(--brand-muted);
}

.file-share-dialog__expiry {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  padding: 0;
  margin: 0;
  border: 0;
}

.file-share-dialog__expiry legend {
  grid-column: 1 / -1;
  margin-bottom: 1px;
  color: var(--text-soft);
  font-size: 13px;
  font-weight: 700;
}

.file-share-dialog__expiry label {
  position: relative;
  min-width: 0;
  cursor: pointer;
}

.file-share-dialog__expiry input {
  position: absolute;
  opacity: 0;
}

.file-share-dialog__expiry span {
  display: grid;
  min-height: 58px;
  align-content: center;
  gap: 2px;
  padding: 9px;
  text-align: center;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
}

.file-share-dialog__expiry label:hover span,
.file-share-dialog__expiry input:focus-visible + span {
  border-color: var(--brand-muted);
}

.file-share-dialog__expiry input:focus-visible + span {
  outline: 2px solid var(--brand-muted);
  outline-offset: 2px;
}

.file-share-dialog__expiry label.is-selected span {
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-color: var(--brand-muted);
}

.file-share-dialog__expiry small {
  color: var(--muted);
  font-size: 13px;
}

.file-share-dialog__links {
  display: grid;
  gap: 8px;
}

.file-share-dialog__links > label {
  margin-top: 3px;
  color: var(--text-soft);
  font-size: 13px;
  font-weight: 700;
}

.file-share-dialog__link-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 7px;
}

.file-share-dialog__link-row input {
  width: 100%;
  min-width: 0;
  height: 40px;
  padding: 0 10px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 9px;
  font: 13px/1.4 ui-monospace, SFMono-Regular, Consolas, monospace;
}

.file-share-dialog__link-row .button {
  min-height: 40px;
  padding-inline: 10px;
  text-decoration: none;
}

.file-share-dialog__message,
.file-share-dialog__error {
  margin: 0;
  padding: 10px 11px;
  border-radius: 9px;
  font-size: 13px;
  line-height: 1.5;
}

.file-share-dialog__message {
  color: var(--brand-strong);
  background: var(--brand-soft);
}

.file-share-dialog__error {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 9%, transparent);
}

.file-share-dialog__retry {
  display: inline-flex;
  min-height: 38px;
  align-items: center;
  justify-content: center;
  gap: 7px;
  color: var(--brand-strong);
  background: transparent;
  border: 1px solid var(--brand-muted);
  border-radius: 9px;
  cursor: pointer;
}

.file-share-dialog__footer {
  display: contents;
}

.file-share-dialog__footer > span {
  flex: 1;
}

.spin {
  animation: file-share-spin 0.85s linear infinite;
}

@keyframes file-share-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 480px) {
  .file-share-dialog__link-row {
    grid-template-columns: minmax(0, 1fr) 1fr 1fr;
  }

  .file-share-dialog__link-row input {
    grid-column: 1 / -1;
  }

  .file-share-dialog__link-row .button {
    justify-content: center;
  }
}
</style>
