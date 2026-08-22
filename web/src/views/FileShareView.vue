<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  Clock3,
  Download,
  ExternalLink,
  File,
  Image,
  LoaderCircle,
  Moon,
  RefreshCw,
  Server,
  Sun,
  TriangleAlert,
} from '@lucide/vue'
import { usePhraseCatalog } from '@/i18n/phrase'
import { ApiError, api } from '@/lib/api'
import { formatBytes, formatDateTime } from '@/lib/format'
import { useTheme } from '@/stores/theme'
import type { PublicFileShareView } from '@/types/api'

usePhraseCatalog((locale) => locale === 'en-US'
  ? import('@/i18n/pages/FileShareView/en-US').then((module) => module.default)
  : import('@/i18n/pages/FileShareView/zh-TW').then((module) => module.default))

const route = useRoute()
const snapshot = ref<PublicFileShareView>()
const loading = ref(true)
const errorMessage = ref('')
const previewFailed = ref(false)
const { resolved: resolvedTheme, setTheme } = useTheme()
let controller: AbortController | undefined
let loadSequence = 0

const token = computed(() => String(route.params.token || ''))
const tokenIsValid = computed(() => /^[A-Za-z0-9_-]{43}$/.test(token.value))
const safeImageMimes = new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif'])
const canPreviewImage = computed(() => Boolean(
  snapshot.value?.mime && safeImageMimes.has(snapshot.value.mime.toLowerCase()) && !previewFailed.value,
))
const expiryDescription = computed(() => snapshot.value?.expiresAt
  ? `有效期至 ${formatDateTime(snapshot.value.expiresAt)}`
  : '永久有效')

function toggleTheme(): void {
  setTheme(resolvedTheme.value === 'dark' ? 'light' : 'dark')
}

function friendlyError(reason: unknown): string {
  if (reason instanceof ApiError && reason.status === 429) {
    return '当前下载较多，请稍后重试。'
  }
  if (reason instanceof ApiError && reason.status === 404) {
    return '链接无效、已过期，或文件已发生变化。'
  }
  return '暂时无法读取分享文件，请检查网络后重试。'
}

async function load(): Promise<void> {
  controller?.abort()
  controller = new AbortController()
  const sequence = ++loadSequence
  loading.value = true
  errorMessage.value = ''
  snapshot.value = undefined
  previewFailed.value = false

  if (!tokenIsValid.value) {
    errorMessage.value = '分享链接格式无效。'
    loading.value = false
    return
  }

  try {
    const result = await api.files.publicShare(token.value, controller.signal)
    if (sequence !== loadSequence) return
    snapshot.value = result
  } catch (reason) {
    if (reason instanceof DOMException && reason.name === 'AbortError') return
    if (sequence !== loadSequence) return
    errorMessage.value = friendlyError(reason)
  } finally {
    if (sequence === loadSequence) loading.value = false
  }
}

watch(token, () => void load())
onMounted(() => void load())
onBeforeUnmount(() => {
  loadSequence += 1
  controller?.abort()
})
</script>

<template>
  <main class="file-share-page">
    <div class="file-share-shell">
      <header class="file-share-header">
        <a class="file-share-brand" href="https://github.com/kejilion/KPanel" target="_blank" rel="noopener noreferrer">
          <span><Server :size="18" /></span>
          <strong>KPanel</strong>
        </a>
        <button
          class="file-share-theme"
          type="button"
          :title="resolvedTheme === 'dark' ? '切换浅色模式' : '切换深色模式'"
          :aria-label="resolvedTheme === 'dark' ? '切换浅色模式' : '切换深色模式'"
          @click="toggleTheme"
        >
          <Sun v-if="resolvedTheme === 'dark'" :size="17" />
          <Moon v-else :size="17" />
        </button>
      </header>

      <section v-if="loading" class="file-share-state" role="status" aria-live="polite">
        <span class="file-share-state__icon"><LoaderCircle class="spin" :size="25" /></span>
        <h1>正在读取分享文件…</h1>
        <p>请稍候，这通常只需要几秒。</p>
      </section>

      <section v-else-if="errorMessage" class="file-share-state file-share-state--error" role="alert">
        <span class="file-share-state__icon"><TriangleAlert :size="25" /></span>
        <h1>无法打开分享文件</h1>
        <p>{{ errorMessage }}</p>
        <button type="button" @click="load">
          <RefreshCw :size="16" />重试
        </button>
      </section>

      <article v-else-if="snapshot" class="file-share-card">
        <div v-if="canPreviewImage" class="file-share-preview">
          <img :src="snapshot.directPath" :alt="snapshot.name" @error="previewFailed = true" />
        </div>

        <div class="file-share-file">
          <span class="file-share-file__icon">
            <Image v-if="canPreviewImage" :size="26" />
            <File v-else :size="26" />
          </span>
          <div>
            <span class="file-share-kicker">KPanel 文件分享</span>
            <h1>{{ snapshot.name }}</h1>
            <p>
              <span>{{ formatBytes(snapshot.sizeBytes) }}</span>
              <span>{{ snapshot.mime || '未知文件类型' }}</span>
            </p>
          </div>
        </div>

        <div class="file-share-expiry">
          <Clock3 :size="17" />
          <span>{{ expiryDescription }}</span>
        </div>

        <div class="file-share-actions">
          <a class="file-share-download" :href="snapshot.downloadPath">
            <Download :size="18" />下载文件
          </a>
          <a class="file-share-open" :href="snapshot.directPath" target="_blank" rel="noopener noreferrer">
            <ExternalLink :size="17" />在浏览器中打开
          </a>
        </div>

        <p class="file-share-note">文件发生修改、移动或删除后，此链接将无法访问。</p>
      </article>

      <footer class="file-share-footer">
        <span>Powered by <strong>KPanel</strong></span>
        <span>公开页不会显示服务器路径或管理信息</span>
      </footer>
    </div>
  </main>
</template>

<style scoped>
.file-share-page {
  min-height: 100vh;
  color: var(--text);
  background:
    radial-gradient(circle at 50% 0%, color-mix(in srgb, var(--brand) 10%, transparent), transparent 34%),
    var(--bg);
}

.file-share-shell {
  display: grid;
  width: min(760px, calc(100% - 40px));
  min-height: 100vh;
  grid-template-rows: auto 1fr auto;
  margin: 0 auto;
  padding: 18px 0 22px;
}

.file-share-header,
.file-share-brand,
.file-share-theme,
.file-share-actions,
.file-share-download,
.file-share-open,
.file-share-expiry,
.file-share-footer,
.file-share-state button {
  display: flex;
  align-items: center;
}

.file-share-header {
  justify-content: space-between;
  gap: 16px;
}

.file-share-brand {
  gap: 10px;
  color: inherit;
  text-decoration: none;
}

.file-share-brand > span {
  display: grid;
  width: 36px;
  height: 36px;
  place-items: center;
  color: #06271f;
  background: var(--brand);
  border-radius: 10px;
  box-shadow: 0 8px 22px color-mix(in srgb, var(--brand) 20%, transparent);
}

.file-share-brand strong {
  font-size: 16px;
  letter-spacing: 0.01em;
}

.file-share-theme {
  width: 40px;
  height: 40px;
  justify-content: center;
  color: var(--text-soft);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 10px;
  box-shadow: var(--shadow-sm);
  cursor: pointer;
}

.file-share-theme:hover {
  color: var(--brand-strong);
  border-color: var(--brand-muted);
}

.file-share-card,
.file-share-state {
  align-self: center;
  margin: 34px 0;
  background: var(--surface-raised);
  border: 1px solid var(--border);
  border-radius: 22px;
  box-shadow: var(--shadow-md);
}

.file-share-card {
  overflow: hidden;
}

.file-share-preview {
  display: grid;
  max-height: min(48vh, 420px);
  padding: 16px;
  place-items: center;
  background:
    linear-gradient(45deg, var(--surface-subtle) 25%, transparent 25%) 0 0 / 18px 18px,
    linear-gradient(-45deg, var(--surface-subtle) 25%, transparent 25%) 0 9px / 18px 18px,
    var(--surface);
  border-bottom: 1px solid var(--border);
}

.file-share-preview img {
  display: block;
  max-width: 100%;
  max-height: calc(min(48vh, 420px) - 32px);
  object-fit: contain;
  border-radius: 10px;
}

.file-share-file {
  display: grid;
  grid-template-columns: 54px minmax(0, 1fr);
  align-items: center;
  gap: 15px;
  padding: 24px 24px 18px;
}

.file-share-file__icon {
  display: grid;
  width: 54px;
  height: 54px;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: 15px;
}

.file-share-kicker {
  color: var(--brand-strong);
  font-size: 13px;
  font-weight: 750;
  letter-spacing: 0.08em;
}

.file-share-file h1 {
  margin: 4px 0 7px;
  overflow-wrap: anywhere;
  font-size: clamp(22px, 4vw, 30px);
  line-height: 1.18;
  letter-spacing: -0.035em;
}

.file-share-file p {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  margin: 0;
  color: var(--muted);
  font-size: 13px;
}

.file-share-expiry {
  gap: 8px;
  margin: 0 24px;
  padding: 12px 14px;
  color: var(--text-soft);
  background: var(--surface-subtle);
  border: 1px solid var(--border);
  border-radius: 11px;
  font-size: 13px;
}

.file-share-expiry svg {
  flex: 0 0 auto;
  color: var(--brand-strong);
}

.file-share-actions {
  gap: 10px;
  padding: 20px 24px 12px;
}

.file-share-download,
.file-share-open,
.file-share-state button {
  min-height: 44px;
  justify-content: center;
  gap: 8px;
  padding: 10px 16px;
  border-radius: 11px;
  font-size: 14px;
  font-weight: 700;
  text-decoration: none;
  cursor: pointer;
}

.file-share-download {
  flex: 1;
  color: #06271f;
  background: var(--brand);
  border: 1px solid var(--brand);
}

.file-share-open,
.file-share-state button {
  color: var(--text-soft);
  background: var(--surface);
  border: 1px solid var(--border);
}

.file-share-download:hover {
  filter: brightness(1.04);
}

.file-share-open:hover,
.file-share-state button:hover {
  color: var(--brand-strong);
  border-color: var(--brand-muted);
}

.file-share-note {
  margin: 0;
  padding: 0 24px 22px;
  color: var(--muted);
  font-size: 13px;
  line-height: 1.5;
  text-align: center;
}

.file-share-state {
  display: grid;
  min-height: 270px;
  padding: 34px;
  place-items: center;
  align-content: center;
  text-align: center;
}

.file-share-state__icon {
  display: grid;
  width: 54px;
  height: 54px;
  margin-bottom: 13px;
  place-items: center;
  color: var(--brand-strong);
  background: var(--brand-soft);
  border-radius: 16px;
}

.file-share-state h1 {
  margin: 0 0 7px;
  font-size: 21px;
}

.file-share-state p {
  margin: 0;
  color: var(--muted);
  font-size: 14px;
  line-height: 1.55;
}

.file-share-state button {
  margin-top: 18px;
}

.file-share-state--error .file-share-state__icon {
  color: var(--danger);
  background: color-mix(in srgb, var(--danger) 10%, transparent);
}

.file-share-footer {
  justify-content: space-between;
  gap: 14px;
  color: var(--muted);
  font-size: 13px;
}

.spin {
  animation: file-share-page-spin 0.85s linear infinite;
}

@keyframes file-share-page-spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 560px) {
  .file-share-shell {
    width: min(100% - 24px, 760px);
    padding-top: 12px;
  }

  .file-share-card,
  .file-share-state {
    margin: 24px 0;
    border-radius: 17px;
  }

  .file-share-file {
    grid-template-columns: 46px minmax(0, 1fr);
    gap: 12px;
    padding: 20px 18px 16px;
  }

  .file-share-file__icon {
    width: 46px;
    height: 46px;
    border-radius: 13px;
  }

  .file-share-expiry {
    margin-inline: 18px;
  }

  .file-share-actions {
    flex-direction: column;
    padding: 17px 18px 11px;
  }

  .file-share-actions a {
    width: 100%;
  }

  .file-share-note {
    padding: 0 18px 19px;
  }

  .file-share-footer {
    align-items: flex-start;
    flex-direction: column;
  }
}

@media (prefers-reduced-motion: reduce) {
  .spin { animation: none; }
}
</style>
