/// <reference types="vite/client" />

/** Release version, injected at build time by vite.config.ts's `define`. */
declare const __APP_VERSION__: string

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
