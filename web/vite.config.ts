import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'
import { ironRdpWasm } from './scripts/ironrdp-wasm-plugin'

export default defineConfig({
  plugins: [ironRdpWasm(), vue()],
  optimizeDeps: { exclude: ['@devolutions/iron-remote-desktop-rdp'] },
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
      '^/mcp$': {
        target: process.env.VITE_DEV_API_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: process.env.VITE_DEV_API_CHANGE_ORIGIN === 'true',
      },
      '/api': {
        target: process.env.VITE_DEV_API_TARGET || 'http://127.0.0.1:8080',
        changeOrigin: process.env.VITE_DEV_API_CHANGE_ORIGIN === 'true',
      },
      '^/f/': {
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
    // Keep jsdom resource use bounded while release checks also run Go.
    maxWorkers: 2,
  },
})
