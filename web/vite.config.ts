import { readFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// Files under public/ are copied verbatim and never get Vite's hashed
// filenames. web/public/browser-app/app.html is an entry-point shell loaded
// into an iframe by BrowserView.vue, and an iframe subresource is not
// revalidated by a parent-page hard reload — so a stale cached copy can
// survive with no user-reachable way to clear it. Stamping the iframe URL
// with the release version gives each release its own cache key. Read from
// package.json rather than a build timestamp to keep builds reproducible
// (release images publish an SBOM and provenance).
const packageVersion = JSON.parse(
  readFileSync(fileURLToPath(new URL('./package.json', import.meta.url)), 'utf8'),
).version as string

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(packageVersion),
  },
  plugins: [vue()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
    dedupe: ['vue', 'vue-router'],
  },
  server: {
    host: '127.0.0.1',
    port: 4173,
    proxy: {
      '/api': {
        target: process.env.VITE_DEV_API_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: process.env.VITE_DEV_API_CHANGE_ORIGIN === 'true',
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
    assetsDir: 'assets',
  },
  test: {
    environment: 'node',
    css: true,
  },
})
