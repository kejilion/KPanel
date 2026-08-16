<script setup lang="ts">
// Thin embed of the legacy Scramjet-based browse app (web/public/browser-app)
// inside a desktop window. The app is a self-contained static page (its own
// node picker, tabs, address bar, service worker) that talks to
// /api/v1/browse/* directly — this component owns nothing of that behavior,
// it only gives it a full-bleed same-origin iframe to run in. See
// internal/panel/server.go's browserAppSecurityRelaxation for the matching
// CSP/X-Frame-Options carve-out that permits this iframe embed.
import { usePhraseCatalog } from '@/i18n/phrase'

usePhraseCatalog(() => import('@/i18n/pages/BrowserView/en-US').then((module) => module.default))

// Version-stamped so each release gets its own HTTP cache key. app.html lives
// under public/ (copied verbatim, no content hash in the filename) and an
// iframe subresource is not revalidated by a parent-page hard reload, so
// without this a stale copy can survive with no user-reachable way to clear
// it. See the matching note in vite.config.ts.
const browserAppUrl = `/browser-app/app.html?v=${__APP_VERSION__}`
</script>

<template>
  <div class="browser-view">
    <iframe class="browser-view__frame" :src="browserAppUrl" title="浏览器" />
  </div>
</template>

<style scoped>
.browser-view {
  position: absolute;
  inset: 0;
  background: #dee1e6;
}

.browser-view__frame {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  border: 0;
  display: block;
}
</style>
