<script setup lang="ts">
// Embed of the Scramjet-based browse app (web/public/browser-app) inside a
// desktop window.
//
// The iframe deliberately points at a *different origin* — the one configured
// in Settings → 面板访问域名 → 浏览器专用域名 — not at a path on the panel.
// Scramjet's Service Worker serves rewritten third-party pages back under
// whatever origin the shell runs on, so running it here would give browsed
// content the panel's cookies and API. See internal/panel/browse_origin.go.
//
// This component therefore does not know the URL up front: it asks the panel
// for a single-use handoff ticket, and the returned URL is on the browse
// origin. When no browse origin is configured the feature fails closed and the
// operator is told to set one, rather than silently falling back to a
// same-origin embed.
import { onMounted, onUnmounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Globe, RotateCw } from '@lucide/vue'
import EmptyState from '@/components/feedback/EmptyState.vue'
import ErrorState from '@/components/feedback/ErrorState.vue'
import { api, ApiError } from '@/lib/api'
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog(() => import('@/i18n/pages/BrowserView/en-US').then((module) => module.default))

const frameUrl = ref('')
const notConfigured = ref(false)
const error = ref('')
const loading = ref(true)
const retrying = ref(false)

// While the hostname is missing the window re-checks on its own, so filling the
// field in Settings is enough -- no closing and reopening this window. The poll
// is bounded: it only runs in the not-configured state, stops the moment a
// session opens, and gives up after maxAutoRetries so an abandoned window does
// not keep asking forever.
const autoRetryIntervalMs = 3000
const maxAutoRetries = 100
let autoRetryTimer: number | undefined
let autoRetries = 0

function stopAutoRetry(): void {
  if (autoRetryTimer !== undefined) {
    window.clearInterval(autoRetryTimer)
    autoRetryTimer = undefined
  }
}

function startAutoRetry(): void {
  if (autoRetryTimer !== undefined) return
  autoRetryTimer = window.setInterval(() => {
    if (autoRetries >= maxAutoRetries) {
      stopAutoRetry()
      return
    }
    autoRetries += 1
    void openSession({ quiet: true })
  }, autoRetryIntervalMs)
}

// quiet keeps the automatic poll from flashing the loading state on every tick;
// a click on Retry is not quiet, so the operator sees it act.
async function openSession(options: { quiet?: boolean } = {}): Promise<void> {
  if (retrying.value) return
  retrying.value = true
  if (!options.quiet) {
    loading.value = true
    error.value = ''
  }
  try {
    const { url } = await api.browse.handoff()
    stopAutoRetry()
    notConfigured.value = false
    error.value = ''
    frameUrl.value = url
  } catch (err) {
    if (err instanceof ApiError && err.code === 'browse_origin_not_configured') {
      notConfigured.value = true
      startAutoRetry()
    } else {
      error.value = err instanceof Error ? err.message : String(err)
    }
  } finally {
    retrying.value = false
    loading.value = false
  }
}

onMounted(() => {
  void openSession()
})
onUnmounted(stopAutoRetry)
</script>

<template>
  <div class="browser-view">
    <iframe v-if="frameUrl" class="browser-view__frame" :src="frameUrl" title="浏览器" />
    <!-- Everything below the iframe is a panel surface, not browser chrome, so
         it uses the shared state components and theme tokens rather than
         Chrome-coloured one-offs. -->
    <div v-else class="browser-view__states">
      <p v-if="loading" class="browser-view__loading">正在建立浏览会话…</p>
      <EmptyState
        v-else-if="notConfigured"
        title="需要先设置浏览器专用域名"
        description="浏览器功能必须运行在和面板不同的域名上。被浏览的网页会在它所在的域名下执行，和面板同域时它就能读到面板的登录状态并调用面板接口，因此这一项没填就不放行。"
      >
        <template #icon><Globe :size="24" :stroke-width="1.8" /></template>
        <p class="browser-view__hint">
          在“设置 → 面板访问域名”右侧填入一个专用域名（例如 browse.example.com），
          并把它解析到这台服务器。填好保存后本窗口会自动进入，不用关掉重开。
        </p>
        <div class="browser-view__actions">
          <RouterLink
            class="button button--primary"
            :to="{ path: '/settings', query: { focus: 'browse-origin' } }"
          >
            前往设置
          </RouterLink>
          <button
            type="button"
            class="button button--secondary"
            :disabled="retrying"
            @click="openSession()"
          >
            <RotateCw :size="15" />
            立即重试
          </button>
        </div>
      </EmptyState>
      <ErrorState
        v-else
        title="无法建立浏览会话"
        :message="error"
        retry-label="重试"
        @retry="openSession()"
      />
    </div>
  </div>
</template>

<style scoped>
.browser-view {
  position: absolute;
  inset: 0;
  background: var(--bg);
}

.browser-view__frame {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}

/* Centres whichever state card is showing, and keeps it readable in a window
   the operator may have dragged down to a narrow size. */
.browser-view__states {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  overflow: auto;
  padding: 24px;
}

.browser-view__states > * {
  width: 100%;
  max-width: 520px;
}

.browser-view__loading {
  color: var(--muted);
  font-size: 13px;
  text-align: center;
}

.browser-view__hint {
  margin: 12px 0 0;
  color: var(--muted);
  font-size: 12.5px;
  line-height: 1.7;
  text-align: left;
}

.browser-view__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 9px;
  margin-top: 14px;
}
</style>
