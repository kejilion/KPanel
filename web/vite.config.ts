import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

// The browse shell's cache-busting version stamp used to be injected here.
// It now comes from the server, which appends it when redirecting into the
// shell (handleBrowseEnter in internal/panel/browse_origin.go) — the shell
// lives on its own origin and BrowserView.vue no longer builds that URL.

export default defineConfig({
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
